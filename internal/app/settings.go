package app

import (
	"context"
	"os"

	"qqmusic/internal/playqueue"
	"qqmusic/internal/qqmusic"
)

// The files of the app's folder. Their names and fields are what earlier
// runs wrote: they must stay readable. fileOldXiaoyuzhou is what v1.0.1
// named fileXiaoyuzhou, the login of 小宇宙.
const (
	fileSettings      = "settings.json"
	fileCredential    = "credential.json"
	fileRecent        = "recent.json"
	fileQueue         = "queue.json"
	fileHeard         = "podcasts.json"
	fileXiaoyuzhou    = "xiaoyuzhou.json"
	fileOldXiaoyuzhou = "xyz.json"
)

// settings are what the app remembers between runs, beside the login.
type settings struct {
	Volume  float64         `json:"volume"`
	Quality qqmusic.Quality `json:"quality"`
	Mode    playqueue.Mode  `json:"mode"`
	// The playlist sections of the sidebar that are folded.
	MineClosed bool `json:"mine_closed"`
	FavClosed  bool `json:"fav_closed"`
	// The searches made last, the newest first, as the search box lists them.
	Searches []string `json:"searches"`
	// The window's normal bounds, and whether it was maximized.
	WinX      int  `json:"win_x"`
	WinY      int  `json:"win_y"`
	WinWidth  int  `json:"win_width"`
	WinHeight int  `json:"win_height"`
	WinMax    bool `json:"win_max"`
	// Whether 小宇宙 shows 小宇宙's own podcasts rather than QQ Music's.
	XiaoyuzhouOfficial bool `json:"xiaoyuzhou_official"`
	// Whether the user has picked one of the two: until they have, 小宇宙
	// opens the page that asks, not either of them.
	XiaoyuzhouChosen bool `json:"xiaoyuzhou_chosen"`
	// OldOfficial is what v1.0.1 named XiaoyuzhouOfficial: read, never
	// written. It was written whenever any setting was saved, so a file
	// that has it is one whose source was already shown: it counts as
	// chosen, not as a run to ask.
	OldOfficial *bool `json:"xyz_official,omitempty"`
	// How many GB the songs played whole may take on disk: 0 keeps none.
	CacheGB int `json:"cache_gb"`
}

// recentMax is how many songs 最近播放 keeps.
const recentMax = 100

// savedQueue is the play queue as queue.json keeps it between runs.
type savedQueue struct {
	Songs []qqmusic.Song `json:"songs"`
	Cur   int            `json:"cur"`
	Radio bool           `json:"radio"`
}

// load reads what the runs before kept. A file cut short sets what it
// could; a queue whose song is not in it is dropped.
func (a *app) load() {
	a.store.Load(fileSettings, &a.set)
	// The view indexes its icons by the mode: one it does not know, as a
	// later version may write, is the default.
	if a.set.Mode < playqueue.Loop || a.set.Mode > playqueue.Order {
		a.set.Mode = playqueue.Loop
	}
	if a.set.OldOfficial != nil {
		a.set.XiaoyuzhouOfficial, a.set.XiaoyuzhouChosen = *a.set.OldOfficial, true
		a.set.OldOfficial = nil
	}
	var cr qqmusic.Credential
	if a.store.Load(fileCredential, &cr) && cr.MusicKey != "" {
		a.api.SetCredential(cr)
	}
	a.store.Load(fileRecent, &a.recent)
	var q savedQueue
	queued := a.store.Load(fileQueue, &q)
	for _, songs := range [][]qqmusic.Song{a.recent, q.Songs} {
		for i := range songs {
			if songs[i].Source == oldXiaoyuzhouSource {
				songs[i].Source = xiaoyuzhouSource
			}
		}
	}
	if queued && q.Cur >= 0 && q.Cur < len(q.Songs) {
		a.queue = playqueue.Restore(q.Songs, q.Cur, q.Radio)
	}
	a.store.Load(fileHeard, &a.podcast.heard)
	for _, sh := range a.podcast.heard {
		a.podcast.known[sh.Mid] = true
	}
}

// saveQueue keeps the queue and the song in it for the next run; an empty
// queue removes the file.
func (a *app) saveQueue() {
	if len(a.queue.Songs()) == 0 {
		a.store.Remove(fileQueue)
		return
	}
	a.store.Save(fileQueue, savedQueue{Songs: a.queue.Songs(), Cur: a.queue.Cur(), Radio: a.queue.Radio()}, false)
}

func (a *app) saveSettings() {
	a.set.Volume = a.volume
	a.store.Save(fileSettings, a.set, true)
}

// cacheTiers are the limits of the song cache to choose from, in GB.
var cacheTiers = []int{0, 1, 2, 5, 10}

// setCacheGB sets how much the song cache keeps, dropping what is past it.
func (a *app) setCacheGB(gb int) {
	if gb == a.set.CacheGB {
		return
	}
	a.set.CacheGB = gb
	a.saveSettings()
	a.onCache(func() { a.player.Cache.SetLimit(int64(gb) << 30) })
}

// clearCache drops the songs the cache keeps.
func (a *app) clearCache() { a.onCache(a.player.Cache.Clear) }

// measureCache finds how much the song cache takes, for 设置.
func (a *app) measureCache() { a.onCache(func() {}) }

// openCacheFolder shows the folder of the song cache, made if no song
// downloaded yet.
func (a *app) openCacheFolder() {
	dir := a.player.Cache.Dir()
	a.do(func(context.Context) func() {
		if os.MkdirAll(dir, 0o700) == nil {
			a.plat.OpenFolder(dir)
		}
		return nil
	})
}

// onCache runs fn on a goroutine, as the cache works on the disk, and
// then measures the cache.
func (a *app) onCache(fn func()) {
	a.cacheBusy = true
	a.do(func(context.Context) func() {
		fn()
		used := a.player.Cache.Size()
		return func() { a.cacheUsed, a.cacheBusy = used, false }
	})
}

// setQuality is the quality the songs play at from the next one on.
func (a *app) setQuality(q qqmusic.Quality) {
	if q == a.set.Quality {
		return
	}
	a.set.Quality = q
	a.saveSettings()
	if _, ok := a.nowPlaying(); ok {
		a.toast("下一首起使用 " + q.String())
	}
}

// rememberWindow keeps the window's bounds, and whether it was maximized,
// for the next run.
func (a *app) rememberWindow(x, y, width, height int, maximized bool) {
	if a.set.WinX == x && a.set.WinY == y && a.set.WinWidth == width &&
		a.set.WinHeight == height && a.set.WinMax == maximized {
		return
	}
	a.set.WinX, a.set.WinY = x, y
	a.set.WinWidth, a.set.WinHeight = width, height
	a.set.WinMax = maximized
	a.saveSettings()
}

// pushFront puts item first in list, dropping the one the same as it and
// those past max: a list of what was used, the last first, as 最近播放, the
// searches made and the shows listened to are.
func pushFront[T any](list []T, item T, max int, same func(a, b T) bool) []T {
	out := make([]T, 1, max)
	out[0] = item
	for _, it := range list {
		if !same(it, item) && len(out) < max {
			out = append(out, it)
		}
	}
	return out
}

// addRecent puts a song that played at the top of 最近播放.
func (a *app) addRecent(s qqmusic.Song) {
	a.recent = pushFront(a.recent, s, recentMax, func(a, b qqmusic.Song) bool { return a.Mid == b.Mid })
	a.store.Save(fileRecent, a.recent, false)
}

func (a *app) saveCredential() {
	cr := a.api.Credential()
	if cr.MusicKey == "" {
		a.store.Remove(fileCredential)
		return
	}
	a.store.Save(fileCredential, cr, true)
}
