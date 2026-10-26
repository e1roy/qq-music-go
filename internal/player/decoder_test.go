package player

import (
	"os"
	"testing"
	"time"
)

// TestStreamDecoder decodes and seeks an MP3 from the network; set
// PLAYER_TEST_URL to one.
func TestStreamDecoder(t *testing.T) {
	url := os.Getenv("PLAYER_TEST_URL")
	if url == "" {
		t.Skip("PLAYER_TEST_URL not set")
	}
	b, err := openBuffer(url)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	d, err := newMP3Decoder(b, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("rate %d, frames %d (%v), %.0f bytes/s", d.rate, d.frames, time.Duration(float64(d.frames)/float64(d.rate)*float64(time.Second)), d.bytesPerSec)
	buf := make([]float32, 4096)
	total := 0
	for total < d.rate*2*2 {
		n, err := d.Read(buf)
		if err != nil {
			t.Fatal(err)
		}
		total += n
	}
	if err := d.SetPosition(int64(d.rate) * 60); err != nil {
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
	t.Logf("peak after seek %.3f, downloaded %.0f%%", peak, b.Progress()*100)
}
