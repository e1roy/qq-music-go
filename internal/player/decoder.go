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

	r    *reader
	d    *gomp3.Decoder
	buf  []byte
	skip int // bytes of decoded sound to drop: the preroll of a seek
}

const frameBytes = 4 // go-mp3 decodes to 16-bit stereo

func newMP3Decoder(b *buffer, duration time.Duration) (*mp3Decoder, error) {
	head := make([]byte, 10)
	if _, err := b.ReadAt(head, 0); err != nil {
		return nil, err
	}
	start := id3End(head)
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

// id3End returns where the ID3v2 tag that head, a file's first 10
// bytes, starts ends: 0 if there is none.
func id3End(head []byte) int64 {
	if len(head) < 10 || string(head[:3]) != "ID3" {
		return 0
	}
	end := 10 + (int64(head[6]&0x7f)<<21 | int64(head[7]&0x7f)<<14 | int64(head[8]&0x7f)<<7 | int64(head[9]&0x7f))
	if head[5]&0x10 != 0 {
		end += 10 // a footer
	}
	return end
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
	for tries := 0; ; {
		got, err := io.ReadFull(m.d, b)
		got -= got % frameBytes
		drop := min(m.skip, got)
		m.skip -= drop
		pcm := b[drop:got]
		for i := range len(pcm) / 2 {
			dst[i] = float32(int16(binary.LittleEndian.Uint16(pcm[2*i:]))) / (1 << 15)
		}
		if len(pcm) > 0 {
			return len(pcm) / 2, nil
		}
		if got > 0 {
			continue // all of it was preroll
		}
		if err == nil || err == io.ErrUnexpectedEOF || err == io.EOF {
			return 0, io.EOF
		}
		if errors.Is(err, errClosed) || tries >= 8 {
			return 0, err
		}
		tries++
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

// preroll is how many frames of sound a seek decodes before the one asked
// for, and drops. An MP3 frame borrows bits from the frames before it, up
// to 511 bytes back, and its sound overlaps theirs: the first few decoded
// from the middle of a file are noise, as loud as the loudest sound.
const preroll = 4 * 1152

func (m *mp3Decoder) SetPosition(frame int64) error {
	if m.rate == 0 {
		return errors.New("player: not open")
	}
	from := max(frame-preroll, 0)
	sec := float64(from) / float64(m.rate)
	off := m.audioStart + int64(sec*m.bytesPerSec)
	if size := m.b.Size(); off >= size {
		off = max(size-1, m.audioStart)
	}
	at, _, err := m.sync(off)
	if err != nil {
		return err
	}
	if err := m.openAt(at); err != nil {
		return err
	}
	m.skip = int(frame-from) * frameBytes
	return nil
}

func (m *mp3Decoder) Close() error {
	m.b.Close()
	return nil
}
