package player

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/mewkiz/flac/frame"
)

// flacDecoder is an audio.Decoder of a FLAC file as it downloads. Seeking
// guesses the byte offset from the file's size, as mp3Decoder does, then
// corrects it by the sample numbers frame headers carry: it needs no seek
// table, nor the whole file, which mewkiz/flac's Seek reads to make one.
type flacDecoder struct {
	b         *buffer
	dataStart int64 // the first frame
	rate      int
	channels  int
	bits      int     // per sample
	block     int64   // samples per frame, for fixed-size frames
	scale     float32 // from samples of bits to [-1, 1]
	frames    int64   // length in frames, -1 if unknown

	r       *cursor
	buf     []float32
	pending []float32 // decoded, not read yet
}

// newFLACDecoder opens the FLAC stream of b at start, where "fLaC" is.
func newFLACDecoder(b *buffer, start int64, duration time.Duration) (*flacDecoder, error) {
	off := start + 4
	head := make([]byte, 4)
	var info []byte
	for {
		if _, err := b.ReadAt(head, off); err != nil {
			return nil, err
		}
		last, kind := head[0]&0x80 != 0, head[0]&0x7f
		n := int64(head[1])<<16 | int64(head[2])<<8 | int64(head[3])
		if kind == 0 && n >= 18 {
			info = make([]byte, 18)
			if _, err := b.ReadAt(info, off+4); err != nil {
				return nil, err
			}
		}
		off += 4 + n
		if last {
			break
		}
	}
	if info == nil {
		return nil, errors.New("player: FLAC without STREAMINFO")
	}
	m := &flacDecoder{
		b:         b,
		dataStart: off,
		rate:      int(info[10])<<12 | int(info[11])<<4 | int(info[12])>>4,
		channels:  int(info[12]>>1&7) + 1,
		bits:      int(info[12]&1)<<4 | int(info[13]>>4) + 1,
		frames:    int64(info[13]&0xf)<<32 | int64(binary.BigEndian.Uint32(info[14:])),
		block:     int64(binary.BigEndian.Uint16(info[2:])),
		r:         &cursor{b: b, at: off},
	}
	if m.rate == 0 {
		return nil, errors.New("player: FLAC of no sample rate")
	}
	m.scale = 1 / float32(int64(1)<<(m.bits-1))
	if m.frames == 0 {
		m.frames = -1
		if duration > 0 {
			m.frames = int64(duration.Seconds() * float64(m.rate))
		}
	}
	return m, nil
}

// parseFrame parses the next frame of r; broken data must not take the
// app down with a panic in the parser.
func parseFrame(r io.Reader) (f *frame.Frame, err error) {
	defer func() {
		if p := recover(); p != nil {
			f, err = nil, fmt.Errorf("player: broken FLAC frame: %v", p)
		}
	}()
	return frame.Parse(r)
}

// fits reports whether f is a frame of the stream, not a sync code in
// the middle of one that its checksums happened to pass.
func (m *flacDecoder) fits(f *frame.Frame) bool {
	return f.Channels.Count() == m.channels && len(f.Subframes) == m.channels &&
		(f.BitsPerSample == 0 || int(f.BitsPerSample) == m.bits) &&
		(f.SampleRate == 0 || int(f.SampleRate) == m.rate)
}

// sync parses the first good frame at or after off, leaving the cursor
// past it.
func (m *flacDecoder) sync(off int64) (*frame.Frame, error) {
	const window = 64 << 10
	buf := make([]byte, window)
	for tries := 0; tries < 32; tries++ {
		n, err := m.b.ReadAt(buf, off)
		if n < 2 {
			if err == nil {
				err = io.EOF
			}
			return nil, err
		}
		for i := 0; i+2 <= n; i++ {
			if buf[i] != 0xff || buf[i+1]&0xfe != 0xf8 {
				continue
			}
			m.r.seek(off + int64(i))
			f, err := parseFrame(m.r)
			if errors.Is(m.r.err, errClosed) {
				return nil, errClosed
			}
			if err == nil && m.fits(f) {
				return f, nil
			}
		}
		off += int64(n - 1)
	}
	return nil, errors.New("player: no FLAC frames")
}

// first returns the number of f's first sample. Frame.SampleNumber is
// wrong for the last frame of fixed-size ones, which is shorter.
func (m *flacDecoder) first(f *frame.Frame) int64 {
	if f.HasFixedBlockSize {
		return int64(f.Num) * m.block
	}
	return int64(f.Num)
}

// decode makes f's samples the ones to read next.
func (m *flacDecoder) decode(f *frame.Frame) {
	n := int(f.BlockSize)
	if cap(m.buf) < n*m.channels {
		m.buf = make([]float32, n*m.channels)
	}
	out := m.buf[:n*m.channels]
	for ch, sf := range f.Subframes {
		for i, s := range sf.Samples[:n] {
			out[i*m.channels+ch] = float32(s) * m.scale
		}
	}
	m.pending = out
}

func (m *flacDecoder) SampleRate() int { return m.rate }
func (m *flacDecoder) Channels() int   { return m.channels }
func (m *flacDecoder) Len() int64      { return m.frames }

func (m *flacDecoder) Read(dst []float32) (int, error) {
	for len(m.pending) == 0 {
		at := m.r.pos()
		f, err := parseFrame(m.r)
		if err == nil && m.fits(f) {
			m.decode(f)
			break
		}
		if err == io.EOF {
			return 0, io.EOF
		}
		if errors.Is(m.r.err, errClosed) {
			return 0, errClosed
		}
		// A broken frame, or what follows the last one: go on from the
		// next good one, if any.
		f, serr := m.sync(at + 1)
		if serr != nil {
			if serr == io.EOF {
				return 0, io.EOF
			}
			return 0, serr
		}
		m.decode(f)
	}
	n := min(len(dst), len(m.pending))
	n -= n % m.channels
	copy(dst, m.pending[:n])
	m.pending = m.pending[n:]
	return n, nil
}

func (m *flacDecoder) SetPosition(target int64) error {
	m.pending = nil
	size := m.b.Size()
	perFrame := 0.0 // bytes per frame, on average
	if m.frames > 0 {
		perFrame = float64(size-m.dataStart) / float64(m.frames)
	}
	guess := m.dataStart + int64(float64(target)*perFrame)
	for tries := 0; ; tries++ {
		guess = max(min(guess, size-1), m.dataStart)
		f, err := m.sync(guess)
		if err != nil {
			if err == io.EOF && guess > m.dataStart && tries < 8 {
				// Past the last frame.
				guess -= 256 << 10
				continue
			}
			return err
		}
		first := m.first(f)
		last := first + int64(f.BlockSize)
		switch {
		case tries >= 8:
		case first > target && guess > m.dataStart:
			// Past it: back by about as much, and some more.
			guess -= int64(float64(first-target)*perFrame) + 64<<10
			continue
		case target-last > int64(m.rate)*5 && perFrame > 0:
			// Far before it: decoding there would take long.
			guess = m.r.pos() + int64(float64(target-last)*perFrame) - 64<<10
			continue
		}
		return m.decodeTo(f, target)
	}
}

// decodeTo decodes from f, a frame before target, to the frame of target.
func (m *flacDecoder) decodeTo(f *frame.Frame, target int64) error {
	for {
		first := m.first(f)
		if first+int64(f.BlockSize) > target {
			m.decode(f)
			m.pending = m.pending[max(target-first, 0)*int64(m.channels):]
			return nil
		}
		var err error
		if f, err = parseFrame(m.r); err != nil {
			if err == io.EOF {
				return nil // reads end
			}
			return err
		}
	}
}

func (m *flacDecoder) Close() error {
	m.b.Close()
	return nil
}

// cursor reads a buffer from an offset through a chunk of its own, for
// parsers that read a byte at a time not to lock the buffer for each.
type cursor struct {
	b     *buffer
	at    int64 // of chunk[0]
	chunk []byte
	i     int
	err   error // of the last read of the buffer
}

func (c *cursor) seek(off int64) {
	c.at, c.chunk, c.i = off, c.chunk[:0], 0
}

// pos is where the next byte read is.
func (c *cursor) pos() int64 { return c.at + int64(c.i) }

func (c *cursor) Read(p []byte) (int, error) {
	if c.i == len(c.chunk) {
		c.at += int64(c.i)
		c.i = 0
		if c.chunk == nil {
			c.chunk = make([]byte, 0, 32<<10)
		}
		n, err := c.b.ReadAt(c.chunk[:cap(c.chunk)], c.at)
		c.chunk, c.err = c.chunk[:n], err
		if n == 0 {
			if err == nil {
				err = io.EOF
			}
			return 0, err
		}
	}
	n := copy(p, c.chunk[c.i:])
	c.i += n
	return n, nil
}
