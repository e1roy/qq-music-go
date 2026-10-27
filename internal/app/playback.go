package app

import (
	"context"
	"math/rand/v2"
	"slices"
	"time"

	"qqmusic/internal/diag"
	"qqmusic/internal/qqmusic"
)

func (a *app) nowPlaying() (qqmusic.Song, bool) {
	if a.cur < 0 || a.cur >= len(a.queue) {
		return qqmusic.Song{}, false
	}
	return a.queue[a.cur], true
}

// playList replaces the queue with songs and plays song i of them.
func (a *app) playList(songs []qqmusic.Song, i int) {
	if len(songs) == 0 {
		return
	}
	// The episodes of 小宇宙 need no login of QQ Music.
	if !a.api.LoggedIn() && songs[i].Source == "" {
		a.toast("登录后即可播放歌曲")
		a.openLogin()
		return
	}
	a.queue = slices.Clone(songs)
	a.radio = false
	a.reshuffle()
	a.failStreak = 0
	a.playAt(i)
}

// playRadio plays 猜你喜欢, which gets more songs as it nears its end.
func (a *app) playRadio() {
	a.playList(a.home.guess, 0)
	a.radio = a.cur >= 0
	a.saveQueue()
}

// moreRadio puts more songs of 猜你喜欢 at the end of the queue.
func (a *app) moreRadio() {
	if a.radioLoading {
		return
	}
	a.radioLoading = true
	a.do(func(ctx context.Context) func() {
		songs, err := a.api.GuessYouLike(ctx)
		return func() {
			a.radioLoading = false
			if err != nil || !a.radio {
				return
			}
			for _, s := range songs {
				if !slices.ContainsFunc(a.queue, func(q qqmusic.Song) bool { return q.Mid == s.Mid }) {
					a.queue = append(a.queue, s)
				}
			}
			a.reshuffle()
			a.saveQueue()
		}
	})
}

// playNext puts a song after the one playing.
func (a *app) playNext(s qqmusic.Song) {
	if len(a.queue) == 0 {
		a.playList([]qqmusic.Song{s}, 0)
		return
	}
	a.queue = slices.Insert(a.queue, a.cur+1, s)
	a.reshuffle()
	a.saveQueue()
	a.toast("已添加到下一首播放")
}

// enqueue puts a song at the end of the queue.
func (a *app) enqueue(s qqmusic.Song) {
	if len(a.queue) == 0 {
		a.playList([]qqmusic.Song{s}, 0)
		return
	}
	a.queue = append(a.queue, s)
	a.reshuffle()
	a.saveQueue()
	a.toast("已添加到播放队列")
}

func (a *app) removeFromQueue(i int) {
	if i < 0 || i >= len(a.queue) {
		return
	}
	a.queue = slices.Delete(a.queue, i, i+1)
	switch {
	case len(a.queue) == 0:
		a.clearQueue()
		return
	case i < a.cur:
		a.cur--
	case i == a.cur:
		a.playAt(min(i, len(a.queue)-1))
	}
	a.reshuffle()
	a.saveQueue()
}

func (a *app) clearQueue() {
	a.queue, a.cur = nil, -1
	a.radio = false
	a.playGen++
	a.player.Stop()
	a.lyrics, a.lyricsFor = nil, ""
	a.trim.soon()
	a.saveQueue()
}

func (a *app) reshuffle() {
	a.shuffled = rand.Perm(len(a.queue))
}

// playAt plays song i of the queue.
func (a *app) playAt(i int) {
	if i < 0 || i >= len(a.queue) {
		return
	}
	a.cur = i
	a.saveQueue()
	if a.radio && i >= len(a.queue)-2 {
		a.moreRadio()
	}
	s := a.queue[i]
	diag.Event("song", s.DisplayName()+" - "+s.Artists())
	a.playErr = ""
	a.playGen++
	gen := a.playGen
	a.seekValue, a.downloaded = 0, false
	a.player.Stop()
	q := a.set.Quality
	// An episode is long, and the player keeps the whole file: one of an
	// hour at 320k would hold 144 MB.
	podcast := a.podcast.known[s.Album.Mid]
	if podcast {
		q = qqmusic.Standard
	}
	a.do(func(ctx context.Context) func() {
		var url string
		var got qqmusic.Quality
		var err error
		switch {
		case s.Source == xyzSource && s.URL != "":
			url = s.URL
		case s.Source == xyzSource:
			// Bought, its sound has a URL of a few hours.
			url, err = a.xyz.api.PrivateMedia(ctx, s.Mid)
		default:
			url, got, err = a.api.SongURL(ctx, s, q)
		}
		if err == nil {
			var ok bool
			ok, err = a.player.Load(url, time.Duration(s.Interval)*time.Second)
			if !ok {
				return nil
			}
		}
		return func() {
			if gen != a.playGen {
				return
			}
			if err != nil {
				a.playErr = errText(err)
				a.failStreak++
				if a.failStreak < 3 && len(a.queue) > 1 {
					a.toast("《" + s.Name + "》无法播放：" + a.playErr + "，已跳过")
					a.next(true)
				} else {
					a.toast("《" + s.Name + "》无法播放：" + a.playErr)
				}
				return
			}
			a.failStreak = 0
			a.gotQuality, a.askedQuality, a.qualityKnown = got, q, s.Source == ""
			a.addRecent(s)
			if podcast {
				a.hearShow(s)
			}
		}
	})
	a.loadLyrics(s)
	if a.showLyrics {
		a.syncComments()
	}
}

func (a *app) loadLyrics(s qqmusic.Song) {
	if a.lyricsFor == s.Mid {
		return
	}
	if s.Source != "" {
		a.lyricsFor, a.lyrics, a.lyricLine = s.Mid, []qqmusic.LyricLine{{At: -1, Text: "播客单集暂无歌词"}}, -1
		return
	}
	a.lyricsFor, a.lyrics, a.lyricLine = s.Mid, nil, -1
	a.do(func(ctx context.Context) func() {
		lrc, trans, err := a.api.Lyric(ctx, s)
		lines := qqmusic.ParseLRC(lrc, trans)
		return func() {
			if a.lyricsFor != s.Mid {
				return
			}
			if err != nil || len(lines) == 0 {
				lines = []qqmusic.LyricLine{{At: -1, Text: "暂无歌词"}}
			}
			a.lyrics = lines
			a.lyricList.ScrollTo(0, 0)
		}
	})
}

func (a *app) togglePlay() {
	if _, ok := a.nowPlaying(); !ok {
		if len(a.home.newSongs) > 0 {
			a.playList(a.home.newSongs, 0)
		}
		return
	}
	switch {
	case a.player.Playing():
		a.player.Pause()
	case a.player.Loaded():
		a.player.Play()
	case !a.player.Loading():
		a.playAt(a.cur)
	}
}

// next goes to the next song; auto when the last one ended or failed.
func (a *app) next(auto bool) {
	n := len(a.queue)
	if n == 0 {
		return
	}
	mode := a.set.Mode
	if auto && mode == modeOne {
		a.player.Seek(0)
		a.player.Play()
		return
	}
	switch mode {
	case modeShuffle:
		a.playAt(a.shuffleStep(1))
	default:
		i := a.cur + 1
		if i >= n {
			if auto && mode == modeOrder {
				a.player.Pause()
				return
			}
			i = 0
		}
		a.playAt(i)
	}
}

func (a *app) prev() {
	n := len(a.queue)
	if n == 0 {
		return
	}
	if a.player.Position() > 5*time.Second {
		a.player.Seek(0)
		return
	}
	if a.set.Mode == modeShuffle {
		a.playAt(a.shuffleStep(-1))
		return
	}
	a.playAt((a.cur - 1 + n) % n)
}

func (a *app) shuffleStep(d int) int {
	if len(a.shuffled) != len(a.queue) {
		a.reshuffle()
	}
	at := slices.Index(a.shuffled, a.cur)
	n := len(a.shuffled)
	return a.shuffled[((at+d)%n+n)%n]
}

// tick follows the player: the next song at the end of one, and the
// lyric line. The view calls it in each frame.
func (a *app) tick() {
	if a.player.Ended() {
		a.next(true)
	}
	// The song before is gone and this one is whole: the heap is as small
	// as it gets until the next.
	if !a.downloaded && a.player.Buffered() == 1 {
		a.downloaded = true
		a.trim.soon()
	}
	if len(a.lyrics) > 0 && a.lyrics[0].At >= 0 {
		a.lyricLine = qqmusic.LineAt(a.lyrics, a.player.Position())
	} else {
		a.lyricLine = -1
	}
}

func (a *app) setVolume(v float64) {
	a.volume = v
	a.muted = v == 0
	a.player.SetVolume(v)
}

func (a *app) toggleMute() {
	if a.muted || a.volume == 0 {
		a.muted = false
		if a.volume == 0 {
			a.volume = 0.5
		}
		a.player.SetVolume(a.volume)
	} else {
		a.muted = true
		a.player.SetVolume(0)
	}
}
