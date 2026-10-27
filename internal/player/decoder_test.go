package player

import (
	"bytes"
	"io"
	"math"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/audio"
	"github.com/mewkiz/flac"
	"github.com/mewkiz/flac/frame"
	"github.com/mewkiz/flac/meta"
)

// serve opens a buffer of data, downloaded from a local server.
func serve(t *testing.T, data []byte) *buffer {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(data))
	}))
	t.Cleanup(srv.Close)
	b, err := openBuffer(Source{URL: srv.URL}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(b.Close)
	return b
}

// TestStreamDecoder decodes and seeks a file from the network, or a local
// one; set PLAYER_TEST_URL to either. MP3, FLAC and AAC all do.
func TestStreamDecoder(t *testing.T) {
	url := os.Getenv("PLAYER_TEST_URL")
	if url == "" {
		t.Skip("PLAYER_TEST_URL not set")
	}
	var b *buffer
	if strings.HasPrefix(url, "http") {
		var err error
		if b, err = openBuffer(Source{URL: url}, nil); err != nil {
			t.Fatal(err)
		}
		defer b.Close()
	} else {
		data, err := os.ReadFile(url)
		if err != nil {
			t.Fatal(err)
		}
		b = serve(t, data)
	}
	start := time.Now()
	d, err := openDecoder(b, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer d.(io.Closer).Close()
	rate, ch := d.SampleRate(), d.Channels()
	t.Logf("%T: rate %d, %d channels, frames %d (%v), opened in %v", d, rate, ch, d.Len(),
		time.Duration(float64(d.Len())/float64(rate)*float64(time.Second)), time.Since(start))
	buf := make([]float32, 4096*ch)
	total := 0
	for total < rate*ch*2 {
		n, err := d.Read(buf)
		if err != nil {
			t.Fatal(err)
		}
		total += n
	}
	start = time.Now()
	if err := d.SetPosition(int64(rate) * 60); err != nil {
		t.Fatal(err)
	}
	var peak float32
	for i := 0; i < 20; i++ {
		n, err := d.Read(buf)
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range buf[:n] {
			peak = max(peak, s, -s)
		}
	}
	if peak == 0 {
		t.Error("silence after seeking to 1:00")
	}
	t.Logf("peak after seek %.3f in %v, downloaded %.0f%%", peak, time.Since(start), b.Progress()*100)
}

// TestMP3Seek seeks into two steady sines: the frames there borrow bits
// from those before and overlap their sound, so decoding from the frame
// itself starts with noise louder than the music (testdata/sine.mp3 is 5 s
// of 440 Hz and 3001 Hz, 128 kbps, made with ffmpeg).
func TestMP3Seek(t *testing.T) {
	data, err := os.ReadFile("testdata/sine.mp3")
	if err != nil {
		t.Fatal(err)
	}
	d, err := openDecoder(serve(t, data), 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]float32, 4*1152*2)
	peak := func() float32 {
		n, err := d.Read(buf)
		if err != nil {
			t.Fatal(err)
		}
		var p float32
		for _, s := range buf[:n] {
			p = max(p, s, -s)
		}
		return p
	}
	if err := d.SetPosition(44100 / 2); err != nil {
		t.Fatal(err)
	}
	steady := peak()
	for _, at := range []float64{1, 2.37, 3.9} {
		if err := d.SetPosition(int64(at * 44100)); err != nil {
			t.Fatal(err)
		}
		if p := peak(); p > steady*1.1 {
			t.Errorf("at %vs: peak %.2f after the seek, the music's %.2f", at, p, steady)
		}
	}
}

// TestFLAC decodes a FLAC file made here, and checks that seeks land on
// the exact sample.
func TestFLAC(t *testing.T) {
	const (
		rate     = 44100
		channels = 2
		block    = 4096
		frames   = rate * 30
	)
	rng := rand.New(rand.NewPCG(1, 2))
	want := make([]int32, frames*channels)
	for i := range frames {
		for c := range channels {
			v := 12000*math.Sin(float64(i)*float64(c+1)*0.01) + float64(rng.IntN(2000)-1000)
			want[i*channels+c] = int32(v)
		}
	}
	var file bytes.Buffer
	enc, err := flac.NewEncoder(&file, &meta.StreamInfo{
		BlockSizeMin: block, BlockSizeMax: block, SampleRate: rate,
		NChannels: channels, BitsPerSample: 16, NSamples: frames,
	})
	if err != nil {
		t.Fatal(err)
	}
	for off := 0; off < frames; off += block {
		n := min(block, frames-off)
		f := &frame.Frame{Header: frame.Header{
			HasFixedBlockSize: true, BlockSize: uint16(n), SampleRate: rate,
			Channels: frame.ChannelsLR, BitsPerSample: 16, Num: uint64(off / block),
		}}
		for c := range channels {
			s := make([]int32, n)
			for i := range s {
				s[i] = want[(off+i)*channels+c]
			}
			f.Subframes = append(f.Subframes, &frame.Subframe{SubHeader: frame.SubHeader{Pred: frame.PredVerbatim}, Samples: s, NSamples: n})
		}
		if err := enc.WriteFrame(f); err != nil {
			t.Fatal(err)
		}
	}
	if err := enc.Close(); err != nil {
		t.Fatal(err)
	}

	d, err := openDecoder(serve(t, file.Bytes()), 0)
	if err != nil {
		t.Fatal(err)
	}
	if d.SampleRate() != rate || d.Channels() != channels || d.Len() != frames {
		t.Fatalf("rate %d, %d channels, %d frames", d.SampleRate(), d.Channels(), d.Len())
	}
	check := func(at int64, n int) {
		t.Helper()
		buf := make([]float32, 1000*channels)
		for got := 0; got < n*channels; {
			k, err := d.Read(buf)
			if err != nil {
				t.Fatalf("at %d: %v", at, err)
			}
			for i, s := range buf[:k] {
				if w := float32(want[int(at)*channels+got+i]) / (1 << 15); s != w {
					t.Fatalf("frame %d: %v, want %v", at+int64((got+i)/channels), s, w)
				}
			}
			got += k
		}
	}
	check(0, 10000)
	for _, at := range []int64{rate * 20, 7, rate*10 + 12345, frames - 2000, block, block - 1, 0} {
		if err := d.SetPosition(at); err != nil {
			t.Fatalf("seek to %d: %v", at, err)
		}
		check(at, 1500)
	}
	// To the end.
	if err := d.SetPosition(frames - 500); err != nil {
		t.Fatal(err)
	}
	n, err := audio.ReadSound(d)
	if err != nil {
		t.Fatal(err)
	}
	if got := n.Duration(); got != 500*time.Second/rate {
		t.Errorf("after the last seek, %v of sound", got)
	}
}
