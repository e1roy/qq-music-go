package player

import (
	"encoding/binary"
	"errors"
	"io"
	"time"

	gomp3 "github.com/hajimehoshi/go-mp3"
)

// mp3Decoder is an audio.Decoder of an MP3 file as it downloads. Seeking
// estimates the byte offset from the bit rate, as streaming players do,
// and starts a new go-mp3 decoder at the next frame there.
type mp3Decoder struct {
	b           *buffer
	audioStart  int64 // past the ID3v2 tag
	rate        int
	frames      int64 // length in frames
	bytesPerSec float64

	r   *reader
	d   *gomp3.Decoder
	buf []byte
}

const frameBytes = 4 // go-mp3 decodes to 16-bit stereo

func newMP3Decoder(b *buffer, duration time.Duration) (*mp3Decoder, error) {
	head := make([]byte, 10)
	if _, err := b.ReadAt(head, 0); err != nil {
		return nil, err
	}
	var start int64
	if string(head[:3]) == "ID3" {
		size := int64(head[6]&0x7f)<<21 | int64(head[7]&0x7f)<<14 | int64(head[8]&0x7f)<<7 | int64(head[9]&0x7f)
		start = 10 + size
		if head[5]&0x10 != 0 {
			start += 10
		}
	}
	m := &mp3Decoder{b: b, audioStart: start}
	off, bitrate, err := m.sync(start)
	if err != nil {
		return nil, err
	}
	if err := m.openAt(off); err != nil {
		return nil, err
	}
	m.rate = m.d.SampleRate()
	audioBytes := float64(b.Size() - start)
	if duration <= 0 && bitrate > 0 {
		duration = time.Duration(audioBytes * 8 / float64(bitrate) * float64(time.Second))
	}
	if duration > 0 {
		m.frames = int64(duration.Seconds() * float64(m.rate))
		m.bytesPerSec = audioBytes / duration.Seconds()
	} else {
		m.frames = -1
		m.bytesPerSec = float64(bitrate) / 8
	}
	return m, nil
}

func (m *mp3Decoder) openAt(off int64) error {
	m.r = &reader{b: m.b, off: off}
	d, err := gomp3.NewDecoder(m.r)
	if err != nil {
		return err
	}
	m.d = d
	return nil
}

var (
	bitratesV1L3 = [16]int{0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 0}
	bitratesV2L3 = [16]int{0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160, 0}
	sampleRates  = [4][3]int{{11025, 12000, 8000}, {0, 0, 0}, {22050, 24000, 16000}, {44100, 48000, 32000}}
)

// frameLen returns the length of the layer III frame of header h, or 0
// if h is not one.
func frameLen(h uint32) (n, bitrate int) {
	if h>>21 != 0x7ff {
		return 0, 0
	}
	version := (h >> 19) & 3
	layer := (h >> 17) & 3
	bi := (h >> 12) & 15
	si := (h >> 10) & 3
	if version == 1 || layer != 1 || bi == 0 || bi == 15 || si == 3 {
		return 0, 0
	}
	sr := sampleRates[version][si]
	pad := int((h >> 9) & 1)
	if version == 3 {
		bitrate = bitratesV1L3[bi] * 1000
		return 144*bitrate/sr + pad, bitrate
	}
	bitrate = bitratesV2L3[bi] * 1000
	return 72*bitrate/sr + pad, bitrate
}

// sync finds the first frame at or after off that two more frames follow.
func (m *mp3Decoder) sync(off int64) (int64, int, error) {
	const window = 64 << 10
	buf := make([]byte, window)
	for tries := 0; tries < 32; tries++ {
		n, err := m.b.ReadAt(buf, off)
		if n < 4 {
			if err == nil {
				err = io.EOF
			}
			return 0, 0, err
		}
		for i := 0; i+4 <= n; i++ {
			l, br := frameLen(binary.BigEndian.Uint32(buf[i:]))
			if l == 0 {
				continue
			}
			ok := true
			j := i
			for k := 0; k < 2 && ok; k++ {
				j += l
				if j+4 > n {
					break // past what was read: take the frame
				}
				l, _ = frameLen(binary.BigEndian.Uint32(buf[j:]))
				ok = l != 0
			}
			if ok {
				return off + int64(i), br, nil
			}
		}
		off += int64(n - 3)
	}
	return 0, 0, errors.New("player: no MP3 frames")
}

func (m *mp3Decoder) SampleRate() int { return m.rate }
func (m *mp3Decoder) Channels() int   { return 2 }
func (m *mp3Decoder) Len() int64      { return m.frames }

func (m *mp3Decoder) Read(dst []float32) (int, error) {
	n := len(dst) / 2 * frameBytes
	if cap(m.buf) < n {
		m.buf = make([]byte, n)
	}
	b := m.buf[:n]
	for tries := 0; ; tries++ {
		got, err := io.ReadFull(m.d, b)
		got -= got % frameBytes
		for i := 0; i < got/2; i++ {
			dst[i] = float32(int16(binary.LittleEndian.Uint16(b[2*i:]))) / (1 << 15)
		}
		if got > 0 {
			return got / 2, nil
		}
		if err == nil || err == io.ErrUnexpectedEOF || err == io.EOF {
			return 0, io.EOF
		}
		if errors.Is(err, errClosed) || tries >= 8 {
			return 0, err
		}
		// A broken frame: go on from the next good one.
		off, _, serr := m.sync(m.r.off + 1)
		if serr != nil {
			return 0, err
		}
		if m.openAt(off) != nil {
			return 0, err
		}
	}
}

func (m *mp3Decoder) SetPosition(frame int64) error {
	if m.rate == 0 {
		return errors.New("player: not open")
	}
	sec := float64(frame) / float64(m.rate)
	off := m.audioStart + int64(sec*m.bytesPerSec)
	if size := m.b.Size(); off >= size {
		off = max(size-1, m.audioStart)
	}
	at, _, err := m.sync(off)
	if err != nil {
		return err
	}
	return m.openAt(at)
}

func (m *mp3Decoder) Close() error {
	m.b.Close()
	return nil
}
