package app

import (
	"context"
	"time"

	"qqmusic/internal/diag"
	"qqmusic/internal/playqueue"
	"qqmusic/internal/qqmusic"
)

func (a *app) nowPlaying() (qqmusic.Song, bool) { return a.queue.Current() }

// playList replaces the queue with songs and plays song i of them,
// reporting whether it did: a song of QQ Music asks to log in first.
func (a *app) playList(songs []qqmusic.Song, i int) bool {
	if len(songs) == 0 {
		return false
	}
	// The episodes of 小宇宙 need no login of QQ Music.
	if !a.api.LoggedIn() && fromQQ(songs[i]) {
		a.toast("登录后即可播放歌曲")
		a.openLogin()
		return false
	}
	a.queue.Replace(songs)
	a.failStreak = 0
	a.playAt(i)
	return true
}

// playRadio plays 猜你喜欢, which gets more songs as it nears its end. The
// queue left as it was, should it not play, is not the radio's.
func (a *app) playRadio() {
	if a.playList(a.home.guess, 0) {
		a.queue.SetRadio(true)
		a.saveQueue()
	}
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
			if err != nil || !a.queue.Radio() {
				return
			}
			a.queue.AppendMissing(songs)
			a.saveQueue()
		}
	})
}

// playNext puts a song after the one playing.
func (a *app) playNext(s qqmusic.Song) {
	if len(a.queue.Songs()) == 0 {
		a.playList([]qqmusic.Song{s}, 0)
		return
	}
	a.queue.InsertNext(s)
	a.saveQueue()
	a.toast("已添加到下一首播放")
}

// enqueue puts a song at the end of the queue.
func (a *app) enqueue(s qqmusic.Song) {
	if len(a.queue.Songs()) == 0 {
		a.playList([]qqmusic.Song{s}, 0)
		return
	}
	a.queue.Append(s)
	a.saveQueue()
	a.toast("已添加到播放队列")
}

func (a *app) removeFromQueue(i int) {
	n := len(a.queue.Songs())
	wasCurrent := a.queue.Remove(i)
	switch {
	case len(a.queue.Songs()) == n: // i was out of range
	case n == 1:
		a.clearQueue()
	case wasCurrent:
		a.playAt(a.queue.Cur()) // which saves the queue
	default:
		a.saveQueue()
	}
}

func (a *app) clearQueue() {
	a.queue.Clear()
	a.playGen++
	a.player.Stop()
	a.lyrics, a.lyricsFor = nil, ""
	a.trim.soon()
	a.saveQueue()
}

// playAt plays song i of the queue.
func (a *app) playAt(i int) {
	if !a.queue.Select(i) {
		return
	}
	a.saveQueue()
	if a.queue.NearEnd() {
		a.moreRadio()
	}
	s, _ := a.queue.Current()
	diag.Event("song", s.DisplayName()+" - "+s.Artists())
	a.playErr = ""
	a.playGen++
	gen := a.playGen
	a.seekValue, a.downloaded = 0, false
	a.player.Stop()
	q := a.set.Quality
	// An episode is long, and the player keeps the whole file: one of an
	// hour at 320k would hold 144 MB.
	podcast := a.qqEpisode(s)
	if podcast {
		q = qqmusic.Standard
	}
	a.do(func(ctx context.Context) func() {
		var url string
		var got qqmusic.Quality
		var err error
		switch {
		case isXiaoyuzhou(s) && s.URL != "":
			url = s.URL
		case isXiaoyuzhou(s):
			// Bought, its sound has a URL of a few hours.
			url, err = a.xiaoyuzhou.api.PrivateMediaURL(ctx, s.Mid)
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
				if a.failStreak < 3 && len(a.queue.Songs()) > 1 {
					a.toast("《" + s.Name + "》无法播放：" + a.playErr + "，已跳过")
					a.next(true)
				} else {
					a.toast("《" + s.Name + "》无法播放：" + a.playErr)
				}
				return
			}
			a.failStreak = 0
			a.gotQuality, a.askedQuality, a.qualityKnown = got, q, fromQQ(s)
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

// qualityLabel is what the player bar says of the quality: the one set,
// until a song loaded, and then what the song got, with why it is less
// than what was asked for. As the next song loads, it shows what the last
// one got, for it not to change back and forth. An episode of 小宇宙 has
// the one file it has.
func qualityLabel(set qqmusic.Quality, known bool, asked, got qqmusic.Quality, episode bool) string {
	switch {
	case episode:
		return "小宇宙"
	case !known:
		return set.String()
	case got == qqmusic.AAC:
		return got.String() + "（无 MP3 音源）"
	case asked == qqmusic.Best || got >= asked:
		return got.String()
	case asked == qqmusic.Lossless:
		return got.String() + "（无损不可用）"
	}
	return got.String() + "（无高品质）"
}

func (a *app) loadLyrics(s qqmusic.Song) {
	if a.lyricsFor == s.Mid {
		return
	}
	if !fromQQ(s) {
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
		a.playAt(a.queue.Cur())
	}
}

// next goes to the next song; auto when the last one ended or failed.
func (a *app) next(auto bool) {
	switch step, i := a.queue.Next(a.set.Mode, auto); step {
	case playqueue.Replay:
		a.player.Seek(0)
		a.player.Play()
	case playqueue.Stop:
		a.player.Pause()
	case playqueue.Play:
		a.playAt(i)
	}
}

func (a *app) prev() {
	if len(a.queue.Songs()) == 0 {
		return
	}
	if a.player.Position() > 5*time.Second {
		a.player.Seek(0)
		return
	}
	a.playAt(a.queue.Prev(a.set.Mode))
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
