// Package player plays MP3, FLAC and AAC (m4a) files from URLs as they
// download, with MyGo's audio package.
package player

import (
	"sync"
	"time"

	"github.com/egoist/mygo/audio"
)

// Player plays one track at a time. Its methods are safe from any
// goroutine.
type Player struct {
	mu      sync.Mutex
	p       *audio.Player
	buf     *buffer
	gen     int
	want    bool // playing, as the user asked: false when paused
	volume  float64
	dur     time.Duration
	loading bool
}

// New returns a player at full volume.
func New() *Player { return &Player{volume: 1} }

// Load stops what plays and opens url, a track of duration (0 if
// unknown), and plays it. It blocks while the file starts to download,
// for an MP4 of AAC until its index came: call it from a goroutine. A later Load or Stop makes it return false
// and drop what it opened.
func (pl *Player) Load(url string, duration time.Duration) (bool, error) {
	pl.mu.Lock()
	pl.gen++
	gen := pl.gen
	pl.stopLocked()
	pl.loading, pl.want = true, true
	pl.mu.Unlock()

	fail := func(err error) (bool, error) {
		pl.mu.Lock()
		defer pl.mu.Unlock()
		if gen != pl.gen {
			return false, nil
		}
		pl.loading, pl.want, pl.buf = false, false, nil
		return true, err
	}
	b, err := openBuffer(url)
	if err != nil {
		return fail(err)
	}
	// Kept while the decoder opens, which for AAC waits for the index of
	// the file: for Buffered, and for a Stop to cancel the download.
	pl.mu.Lock()
	if gen != pl.gen {
		pl.mu.Unlock()
		b.Close()
		return false, nil
	}
	pl.buf = b
	pl.mu.Unlock()
	d, err := openDecoder(b, duration)
	if err != nil {
		b.Close()
		return fail(err)
	}
	p := audio.NewPlayer(d)

	pl.mu.Lock()
	defer pl.mu.Unlock()
	if gen != pl.gen {
		p.Close()
		return false, nil
	}
	pl.p, pl.buf, pl.loading = p, b, false
	pl.dur = p.Duration()
	p.SetVolume(pl.volume * pl.volume) // a gentler curve than linear
	if pl.want {
		p.Play()
	}
	return true, nil
}

// openDecoder returns a decoder of b by the format its start tells, not
// by what was asked for: what a URL serves is what plays.
func openDecoder(b *buffer, duration time.Duration) (audio.Decoder, error) {
	head := make([]byte, 10)
	if _, err := b.ReadAt(head, 0); err != nil {
		return nil, err
	}
	if string(head[4:8]) == "ftyp" {
		return newAACDecoder(b, duration)
	}
	start := id3End(head)
	if start > 0 {
		if _, err := b.ReadAt(head[:4], start); err != nil {
			return nil, err
		}
	}
	if string(head[:4]) == "fLaC" {
		return newFLACDecoder(b, start, duration)
	}
	return newMP3Decoder(b, duration)
}

func (pl *Player) stopLocked() {
	if pl.p != nil {
		pl.p.Close()
	} else if pl.buf != nil {
		pl.buf.Close() // still opening
	}
	pl.p, pl.buf = nil, nil
	pl.dur = 0
}

// Stop stops and forgets the track.
func (pl *Player) Stop() {
	pl.mu.Lock()
	defer pl.mu.Unlock()
	pl.gen++
	pl.stopLocked()
	pl.want, pl.loading = false, false
}

// Play resumes the track.
func (pl *Player) Play() {
	pl.mu.Lock()
	defer pl.mu.Unlock()
	pl.want = true
	if pl.p != nil {
		pl.p.Play()
	}
}

// Pause pauses the track.
func (pl *Player) Pause() {
	pl.mu.Lock()
	defer pl.mu.Unlock()
	pl.want = false
	if pl.p != nil {
		pl.p.Pause()
	}
}

// Playing reports whether the track plays or is about to, once loaded.
func (pl *Player) Playing() bool {
	pl.mu.Lock()
	defer pl.mu.Unlock()
	return pl.want
}

// Loading reports whether a track is being opened.
func (pl *Player) Loading() bool {
	pl.mu.Lock()
	defer pl.mu.Unlock()
	return pl.loading
}

// Loaded reports whether there is a track.
func (pl *Player) Loaded() bool {
	pl.mu.Lock()
	defer pl.mu.Unlock()
	return pl.p != nil
}

// Ended reports whether the track played to its end.
func (pl *Player) Ended() bool {
	pl.mu.Lock()
	defer pl.mu.Unlock()
	return pl.p != nil && pl.want && !pl.p.Playing()
}

// Seek moves to t.
func (pl *Player) Seek(t time.Duration) {
	pl.mu.Lock()
	defer pl.mu.Unlock()
	if pl.p != nil {
		pl.p.Seek(t)
	}
}

// Position is where the track is.
func (pl *Player) Position() time.Duration {
	pl.mu.Lock()
	defer pl.mu.Unlock()
	if pl.p == nil {
		return 0
	}
	return pl.p.Position()
}

// Duration is the length of the track.
func (pl *Player) Duration() time.Duration {
	pl.mu.Lock()
	defer pl.mu.Unlock()
	return pl.dur
}

// Buffered is how much of the track downloaded, from 0 to 1.
func (pl *Player) Buffered() float64 {
	pl.mu.Lock()
	defer pl.mu.Unlock()
	if pl.buf == nil {
		return 0
	}
	return pl.buf.Progress()
}

// Size is the length of the file in bytes, 0 if there is none.
func (pl *Player) Size() int64 {
	pl.mu.Lock()
	defer pl.mu.Unlock()
	if pl.buf == nil {
		return 0
	}
	return pl.buf.Size()
}

// SetVolume sets the volume from 0 to 1.
func (pl *Player) SetVolume(v float64) {
	pl.mu.Lock()
	defer pl.mu.Unlock()
	pl.volume = min(max(v, 0), 1)
	if pl.p != nil {
		pl.p.SetVolume(pl.volume * pl.volume)
	}
}

// Volume returns the volume from 0 to 1.
func (pl *Player) Volume() float64 {
	pl.mu.Lock()
	defer pl.mu.Unlock()
	return pl.volume
}
