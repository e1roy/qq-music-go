//go:build windows && (amd64 || arm64)

package player

import (
	"errors"
	"fmt"
	"io"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

// aacDecoder is an audio.Decoder of an MP4 file of AAC, with the Media
// Foundation Windows comes with: there is no AAC decoder in pure Go to
// trust, and the system's is a COM call away, no cgo needed. It reads the
// file through a stream as it downloads, which plays once the index of the
// MP4 came: at its start, at once, at its end, once all of it did. Every
// call runs on a thread of its own, locked, in COM's multithreaded
// apartment.
type aacDecoder struct {
	b      *buffer
	s      *stream // held by Media Foundation until reader is released
	calls  chan func()
	closed sync.Once

	// On the thread only.
	com, mf  bool    // CoInitializeEx and MFStartup succeeded
	reader   uintptr // IMFSourceReader
	rate     int
	channels int
	frames   int64
	pending  []float32
	buf      []float32
	eof      bool
	seekTo   int64 // the frame of the last seek, until a sample after it; -1 if none
}

type guid struct {
	data1 uint32
	data2 uint16
	data3 uint16
	data4 [8]byte
}

// propVariant is a PROPVARIANT of a 64-bit integer, the only kind used.
type propVariant struct {
	vt       uint16
	reserved [3]uint16
	val      int64
	_        int64
}

var (
	mfMTMajorType            = guid{0x48eba18e, 0xf8c9, 0x4687, [8]byte{0xbf, 0x11, 0x0a, 0x74, 0xc9, 0xf9, 0x6a, 0x8f}}
	mfMTSubtype              = guid{0xf7e34c9a, 0x42e8, 0x4714, [8]byte{0xb7, 0x4b, 0xcb, 0x29, 0xd7, 0x2c, 0x35, 0xe5}}
	mfMediaTypeAudio         = guid{0x73647561, 0x0000, 0x0010, [8]byte{0x80, 0x00, 0x00, 0xaa, 0x00, 0x38, 0x9b, 0x71}}
	mfAudioFormatFloat       = guid{0x00000003, 0x0000, 0x0010, [8]byte{0x80, 0x00, 0x00, 0xaa, 0x00, 0x38, 0x9b, 0x71}}
	mfMTAudioNumChannels     = guid{0x37e48bf5, 0x645e, 0x4c5b, [8]byte{0x89, 0xde, 0xad, 0xa9, 0xe2, 0x9b, 0x69, 0x6a}}
	mfMTAudioSamplesPerSec   = guid{0x5faeeae7, 0x0290, 0x4c31, [8]byte{0x9e, 0x8a, 0xc5, 0x34, 0xf6, 0x8d, 0x9d, 0xba}}
	mfPDDuration             = guid{0x6c990d33, 0xbb8e, 0x477a, [8]byte{0x85, 0x98, 0x0d, 0x5d, 0x96, 0xfc, 0xd8, 0x8a}}
	mfByteStreamContentType  = guid{0xfc358289, 0x3cb6, 0x460c, [8]byte{0xa4, 0x24, 0xb6, 0x68, 0x12, 0x60, 0x37, 0x5a}}
	iidIMFAttributes         = guid{0x2cd2d921, 0xc447, 0x44a7, [8]byte{0xa1, 0x3c, 0x4a, 0xda, 0xbf, 0xc2, 0x47, 0xe3}}
	guidNull                 guid
	ole32                    = syscall.NewLazyDLL("ole32.dll")
	mfplat                   = syscall.NewLazyDLL("mfplat.dll")
	mfreadwrite              = syscall.NewLazyDLL("mfreadwrite.dll")
	procCoInitializeEx       = ole32.NewProc("CoInitializeEx")
	procCoUninitialize       = ole32.NewProc("CoUninitialize")
	procMFStartup            = mfplat.NewProc("MFStartup")
	procMFShutdown           = mfplat.NewProc("MFShutdown")
	procMFCreateMediaType    = mfplat.NewProc("MFCreateMediaType")
	procMFCreateByteStream   = mfplat.NewProc("MFCreateMFByteStreamOnStream")
	procMFCreateSourceReader = mfreadwrite.NewProc("MFCreateSourceReaderFromByteStream")
)

const (
	mfVersion          = 0x00020070
	mfStartupNoSocket  = 1
	readerAllStreams   = 0xfffffffe
	readerFirstAudio   = 0xfffffffd
	readerMediaSource  = 0xffffffff
	readerFlagError    = 0x1
	readerFlagEnd      = 0x2
	vtI8               = 20
	vtUI8              = 21
	hundredNanoseconds = int64(time.Second / 100)
)

// Vtable indices of the methods called.
const (
	unkQueryInterface = 0
	unkRelease        = 2

	attrGetUINT32 = 7
	attrSetGUID   = 24
	attrSetString = 25

	readerSetStreamSelection       = 4
	readerGetCurrentMediaType      = 6
	readerSetCurrentMediaType      = 7
	readerSetCurrentPosition       = 8
	readerReadSample               = 9
	readerGetPresentationAttribute = 12

	sampleConvertToContiguousBuffer = 41

	bufferLock   = 3
	bufferUnlock = 4
)

// comCall calls method i of the COM interface obj. Pointers converted to
// uintptr in the call stay valid until it returns (go:uintptrescapes).
//
//go:uintptrescapes
func comCall(obj uintptr, i int, args ...uintptr) uintptr {
	vtbl := *(*uintptr)(native(obj))
	fn := *(*uintptr)(unsafe.Add(native(vtbl), i*int(unsafe.Sizeof(uintptr(0)))))
	var a [16]uintptr
	a[0] = obj
	n := copy(a[1:], args)
	r, _, _ := syscall.SyscallN(fn, a[:n+1]...)
	return r
}

// native converts a native address to a pointer.
func native(addr uintptr) unsafe.Pointer { return *(*unsafe.Pointer)(unsafe.Pointer(&addr)) }

func failed(hr uintptr) bool { return int32(uint32(hr)) < 0 }

func hresult(what string, hr uintptr) error {
	return fmt.Errorf("player: 系统无法解码 AAC：%s 失败（HRESULT %#08x）", what, uint32(hr))
}

func release(obj uintptr) {
	if obj != 0 {
		comCall(obj, unkRelease)
	}
}

var errNoMF = errors.New("player: 系统缺少 Media Foundation，无法播放 AAC（Windows N 版需安装媒体功能包）")

func newAACDecoder(b *buffer, duration time.Duration) (*aacDecoder, error) {
	for _, p := range []*syscall.LazyProc{procCoInitializeEx, procCoUninitialize,
		procMFStartup, procMFShutdown, procMFCreateMediaType, procMFCreateByteStream, procMFCreateSourceReader} {
		if p.Find() != nil {
			return nil, errNoMF
		}
	}
	// The stream tells its size: of a file served without one, once all
	// of it came.
	size := b.known()
	if size <= 0 {
		data, err := b.all()
		if err != nil {
			return nil, err
		}
		size = int64(len(data))
	}
	if size == 0 {
		return nil, errors.New("player: empty file")
	}
	d := &aacDecoder{b: b, s: newStream(b, size), calls: make(chan func()), seekTo: -1}
	go d.serve()
	var err error
	d.run(func() { err = d.open(duration) })
	if err != nil {
		d.Close()
		return nil, err
	}
	return d, nil
}

// serve runs the calls on its thread until Close. The thread stays locked,
// so that it ends with the goroutine, COM state and all.
func (d *aacDecoder) serve() {
	runtime.LockOSThread()
	for f := range d.calls {
		f()
	}
}

func (d *aacDecoder) run(f func()) {
	done := make(chan struct{})
	d.calls <- func() {
		defer close(done)
		f()
	}
	<-done
}

func (d *aacDecoder) open(duration time.Duration) error {
	if hr, _, _ := procCoInitializeEx.Call(0, 0); failed(hr) { // multithreaded
		return hresult("CoInitializeEx", hr)
	}
	d.com = true
	if hr, _, _ := procMFStartup.Call(mfVersion, mfStartupNoSocket); failed(hr) {
		return errNoMF
	}
	d.mf = true
	var bs uintptr
	if hr, _, _ := procMFCreateByteStream.Call(uintptr(unsafe.Pointer(d.s)), uintptr(unsafe.Pointer(&bs))); failed(hr) {
		return hresult("MFCreateMFByteStreamOnStream", hr)
	}
	defer release(bs)
	// The type, for the source resolver not to have to guess it.
	var attrs uintptr
	if hr := comCall(bs, unkQueryInterface, uintptr(unsafe.Pointer(&iidIMFAttributes)), uintptr(unsafe.Pointer(&attrs))); !failed(hr) {
		mime, _ := syscall.UTF16PtrFromString("audio/mp4")
		comCall(attrs, attrSetString, uintptr(unsafe.Pointer(&mfByteStreamContentType)), uintptr(unsafe.Pointer(mime)))
		release(attrs)
	}
	if hr, _, _ := procMFCreateSourceReader.Call(bs, 0, uintptr(unsafe.Pointer(&d.reader))); failed(hr) {
		return hresult("MFCreateSourceReaderFromByteStream", hr)
	}
	comCall(d.reader, readerSetStreamSelection, readerAllStreams, 0)
	if hr := comCall(d.reader, readerSetStreamSelection, readerFirstAudio, 1); failed(hr) {
		return hresult("SetStreamSelection", hr)
	}
	// Decoded to float samples.
	var mt uintptr
	if hr, _, _ := procMFCreateMediaType.Call(uintptr(unsafe.Pointer(&mt))); failed(hr) {
		return hresult("MFCreateMediaType", hr)
	}
	comCall(mt, attrSetGUID, uintptr(unsafe.Pointer(&mfMTMajorType)), uintptr(unsafe.Pointer(&mfMediaTypeAudio)))
	comCall(mt, attrSetGUID, uintptr(unsafe.Pointer(&mfMTSubtype)), uintptr(unsafe.Pointer(&mfAudioFormatFloat)))
	hr := comCall(d.reader, readerSetCurrentMediaType, readerFirstAudio, 0, mt)
	release(mt)
	if failed(hr) {
		return hresult("SetCurrentMediaType", hr)
	}
	var cur uintptr
	if hr := comCall(d.reader, readerGetCurrentMediaType, readerFirstAudio, uintptr(unsafe.Pointer(&cur))); failed(hr) {
		return hresult("GetCurrentMediaType", hr)
	}
	var ch, rate uint32
	comCall(cur, attrGetUINT32, uintptr(unsafe.Pointer(&mfMTAudioNumChannels)), uintptr(unsafe.Pointer(&ch)))
	comCall(cur, attrGetUINT32, uintptr(unsafe.Pointer(&mfMTAudioSamplesPerSec)), uintptr(unsafe.Pointer(&rate)))
	release(cur)
	if ch == 0 || rate == 0 {
		return errors.New("player: AAC of no channels or sample rate")
	}
	d.channels, d.rate = int(ch), int(rate)
	d.frames = -1
	var pv propVariant
	if hr := comCall(d.reader, readerGetPresentationAttribute, readerMediaSource, uintptr(unsafe.Pointer(&mfPDDuration)), uintptr(unsafe.Pointer(&pv))); !failed(hr) && pv.vt == vtUI8 {
		d.frames = pv.val * int64(d.rate) / hundredNanoseconds
	} else if duration > 0 {
		d.frames = int64(duration.Seconds() * float64(d.rate))
	}
	return nil
}

func (d *aacDecoder) SampleRate() int { return d.rate }
func (d *aacDecoder) Channels() int   { return d.channels }
func (d *aacDecoder) Len() int64      { return d.frames }

func (d *aacDecoder) Read(dst []float32) (n int, err error) {
	d.run(func() { n, err = d.read(dst) })
	return n, err
}

func (d *aacDecoder) read(dst []float32) (int, error) {
	for len(d.pending) == 0 {
		if d.eof {
			return 0, io.EOF
		}
		var index, flags uint32
		var at int64
		var sample uintptr
		hr := comCall(d.reader, readerReadSample, readerFirstAudio, 0,
			uintptr(unsafe.Pointer(&index)), uintptr(unsafe.Pointer(&flags)), uintptr(unsafe.Pointer(&at)), uintptr(unsafe.Pointer(&sample)))
		if failed(hr) {
			return 0, hresult("ReadSample", hr)
		}
		if flags&readerFlagError != 0 {
			release(sample)
			return 0, errors.New("player: AAC 解码出错")
		}
		if flags&readerFlagEnd != 0 {
			d.eof = true
		}
		if sample == 0 {
			continue
		}
		err := d.take(sample)
		release(sample)
		if err != nil {
			return 0, err
		}
		if d.seekTo >= 0 {
			// Seeks land on a sample at or before the frame; its time is
			// in 100 ns, rounded.
			first := (at*int64(d.rate) + hundredNanoseconds/2) / hundredNanoseconds
			skip := (d.seekTo - first) * int64(d.channels)
			d.pending = d.pending[min(max(skip, 0), int64(len(d.pending))):]
			d.seekTo = -1
		}
	}
	n := min(len(dst), len(d.pending))
	n -= n % d.channels
	copy(dst, d.pending[:n])
	d.pending = d.pending[n:]
	return n, nil
}

// take makes the samples of sample, an IMFSample, the ones to read next.
func (d *aacDecoder) take(sample uintptr) error {
	var mb uintptr
	if hr := comCall(sample, sampleConvertToContiguousBuffer, uintptr(unsafe.Pointer(&mb))); failed(hr) {
		return hresult("ConvertToContiguousBuffer", hr)
	}
	defer release(mb)
	var p uintptr
	var size uint32
	if hr := comCall(mb, bufferLock, uintptr(unsafe.Pointer(&p)), 0, uintptr(unsafe.Pointer(&size))); failed(hr) {
		return hresult("IMFMediaBuffer.Lock", hr)
	}
	n := int(size) / 4
	n -= n % d.channels
	if cap(d.buf) < n {
		d.buf = make([]float32, n)
	}
	d.pending = d.buf[:n]
	if n > 0 {
		copy(d.pending, unsafe.Slice((*float32)(native(p)), n))
	}
	comCall(mb, bufferUnlock)
	return nil
}

func (d *aacDecoder) SetPosition(frame int64) (err error) {
	d.run(func() {
		pv := propVariant{vt: vtI8, val: frame * hundredNanoseconds / int64(d.rate)}
		if hr := comCall(d.reader, readerSetCurrentPosition, uintptr(unsafe.Pointer(&guidNull)), uintptr(unsafe.Pointer(&pv))); failed(hr) {
			err = hresult("SetCurrentPosition", hr)
			return
		}
		d.pending, d.eof, d.seekTo = nil, false, frame
	})
	return err
}

func (d *aacDecoder) Close() error {
	d.closed.Do(func() {
		// First the download: a read waiting for bytes ends, and with it
		// the call on the thread.
		d.b.Close()
		d.run(func() {
			release(d.reader)
			d.reader = 0
			if d.mf {
				procMFShutdown.Call()
			}
			if d.com {
				procCoUninitialize.Call()
			}
		})
		close(d.calls)
	})
	return nil
}
