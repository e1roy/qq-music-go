//go:build windows && (amd64 || arm64)

package player

import (
	"io"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"
)

// stream is a COM IStream of a buffer, for Media Foundation to read the
// file as it downloads: a read waits for its bytes, as the MP3 decoder's
// do. An MP4 with its index first plays once that came, as a browser's
// <audio> does; one with it last waits for the whole file. Its methods are
// Go functions made callable by syscall.NewCallback, no cgo needed; Media
// Foundation may call them on threads of its own.
type stream struct {
	vtbl *[14]uintptr // first: the layout of a COM object
	refs atomic.Int32
	b    *buffer
	size int64

	mu  sync.Mutex // the position, and a read at it
	pos int64
}

// statstg is a STATSTG, the answer of IStream::Stat.
type statstg struct {
	name                 uintptr
	typ                  uint32
	size                 uint64
	mtime, ctime, atime  uint64
	mode, locksSupported uint32
	clsid                guid
	stateBits, reserved  uint32
}

const (
	sOK              = 0
	sFalse           = 1
	eNotImpl         = 0x80004001
	eNoInterface     = 0x80004002
	stgEInvalidFunc  = 0x80030001
	stgEAccessDenied = 0x80030005
	stgEReadFault    = 0x8003001e
	stgtyStream      = 2
)

var (
	iidIUnknown          = guid{0x00000000, 0x0000, 0x0000, [8]byte{0xc0, 0, 0, 0, 0, 0, 0, 0x46}}
	iidISequentialStream = guid{0x0c733a30, 0x2a1c, 0x11ce, [8]byte{0xad, 0xe5, 0x00, 0xaa, 0x00, 0x44, 0x77, 0x3d}}
	iidIStream           = guid{0x0000000c, 0x0000, 0x0000, [8]byte{0xc0, 0, 0, 0, 0, 0, 0, 0x46}}

	// The methods, made once: callbacks are never freed.
	streamVtbl     [14]uintptr
	streamVtblOnce sync.Once
)

// newStream returns a stream of b, of size bytes. The caller keeps it
// alive while Media Foundation holds it.
func newStream(b *buffer, size int64) *stream {
	streamVtblOnce.Do(func() {
		notImpl := syscall.NewCallback(func(this uintptr) uintptr { return eNotImpl })
		streamVtbl = [14]uintptr{
			syscall.NewCallback(streamQueryInterface),
			syscall.NewCallback(func(this uintptr) uintptr { return uintptr(streamOf(this).refs.Add(1)) }),
			syscall.NewCallback(func(this uintptr) uintptr { return uintptr(streamOf(this).refs.Add(-1)) }),
			syscall.NewCallback(streamRead),
			syscall.NewCallback(func(this, pv, cb, written uintptr) uintptr { return stgEAccessDenied }), // Write
			syscall.NewCallback(streamSeek),
			notImpl,                                                               // SetSize
			notImpl,                                                               // CopyTo
			syscall.NewCallback(func(this, flags uintptr) uintptr { return sOK }), // Commit
			notImpl, // Revert
			notImpl, // LockRegion
			notImpl, // UnlockRegion
			syscall.NewCallback(streamStat),
			notImpl, // Clone
		}
	})
	s := &stream{vtbl: &streamVtbl, b: b, size: size}
	s.refs.Store(1)
	return s
}

func streamOf(this uintptr) *stream { return (*stream)(native(this)) }

func streamQueryInterface(this, riid, ppv uintptr) uintptr {
	id := *(*guid)(native(riid))
	if id != iidIUnknown && id != iidISequentialStream && id != iidIStream {
		*(*uintptr)(native(ppv)) = 0
		return eNoInterface
	}
	streamOf(this).refs.Add(1)
	*(*uintptr)(native(ppv)) = this
	return sOK
}

func streamRead(this, pv, cb, read uintptr) uintptr {
	s := streamOf(this)
	s.mu.Lock()
	defer s.mu.Unlock()
	n, err := 0, error(nil)
	if cb > 0 {
		n, err = s.b.ReadAt(unsafe.Slice((*byte)(native(pv)), uint32(cb)), s.pos)
	}
	s.pos += int64(n)
	if read != 0 {
		*(*uint32)(native(read)) = uint32(n)
	}
	switch {
	case err == nil:
		return sOK
	case err == io.EOF:
		return sFalse
	}
	return stgEReadFault
}

func streamSeek(this, move, origin, newPos uintptr) uintptr {
	s := streamOf(this)
	s.mu.Lock()
	defer s.mu.Unlock()
	pos := int64(move)
	switch origin {
	case 0: // STREAM_SEEK_SET
	case 1: // STREAM_SEEK_CUR
		pos += s.pos
	case 2: // STREAM_SEEK_END
		pos += s.size
	default:
		return stgEInvalidFunc
	}
	if pos < 0 {
		return stgEInvalidFunc
	}
	s.pos = pos
	if newPos != 0 {
		*(*uint64)(native(newPos)) = uint64(pos)
	}
	return sOK
}

func streamStat(this, stat, flags uintptr) uintptr {
	*(*statstg)(native(stat)) = statstg{typ: stgtyStream, size: uint64(streamOf(this).size)}
	return sOK
}
