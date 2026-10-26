// Package player plays MP3 files from URLs as they download, with MyGo's
// audio package.
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
// unknown), and plays it. It blocks while the file starts to download:
// call it from a goroutine. A later Load or Stop makes it return false
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
		pl.loading, pl.want = false, false
		return true, err
	}
	b, err := openBuffer(url)
	if err != nil {
		return fail(err)
	}
	d, err := newMP3Decoder(b, duration)
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

func (pl *Player) stopLocked() {
	if pl.p != nil {
		pl.p.Close()
		pl.p, pl.buf = nil, nil
	}
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
