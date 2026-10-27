// Package app is the QQ Music desktop client: the state of the app, its
// pages and the player, the login, and the views of the native window.
package app

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo/ui"

	"qqmusic/internal/player"
	"qqmusic/internal/playqueue"
	"qqmusic/internal/qqmusic"
	"qqmusic/internal/store"
	"qqmusic/internal/xiaoyuzhou"
)

// platform is what the app needs of the window, so tests run without one.
type platform interface {
	// Post runs fn on the main thread and draws a frame, from any goroutine.
	Post(fn func())
	// OpenQQLogin opens the QQ login page in a webview and calls done with
	// the cookies of graph.qq.com once logged in, on any goroutine.
	OpenQQLogin(done func(cookies []*cookie, err error))
	// OpenXiaoyuzhouLogin opens 小宇宙's login page in a webview and calls done with
	// the tokens it set once logged in, on any goroutine.
	OpenXiaoyuzhouLogin(done func(access, refresh string, err error))
	// WebViewAvailable reports whether OpenQQLogin and OpenXiaoyuzhouLogin can
	// work.
	WebViewAvailable() bool
	// Minimize, ToggleMaximize, Maximized and Close are the window's
	// controls, which the view draws itself.
	Minimize()
	ToggleMaximize()
	Maximized() bool
	Close()
	// OpenFolder shows a folder in the system's file manager.
	OpenFolder(path string)
}

type cookie struct{ Name, Value, Domain, Path string }

type app struct {
	plat   platform
	api    *qqmusic.Client
	player *player.Player
	images *images
	tint   tintFade // the color of the top of the page, from its picture
	trim   trimmer
	store  store.Dir // where the login and settings are saved
	set    settings

	// Navigation.
	history []page
	future  []page
	// Motion: the page shown, whether it came in, and the lyrics, the
	// queue and the comments coming and going.
	pageShown    page
	pageIn       bool
	lyricsShow   presence
	queueShow    presence
	commentsShow presence
	// queueBtnHover is whether the pointer is on the queue's toggle, whose
	// press is not one outside the queue.
	queueBtnHover bool

	// Pages.
	home struct {
		loaded, loading bool
		recs            []qqmusic.Playlist
		recFrom         int // where the next batch of recs starts
		recLoading      bool
		recMotion       refreshMotion  // the playlist cards coming in again after 换一批
		radar           []qqmusic.Song // 精选好歌
		radarPage       int
		radarLoading    bool
		radarMotion     refreshMotion // the cards coming in again after 换一批
		newSongs        []qqmusic.Song
		guess           []qqmusic.Song
		dailyList       qqmusic.Playlist // 每日30首
		daily           []qqmusic.Song
		feed            []qqmusic.FeedShelf // the sections the app's front page has
		feedMotion      []refreshMotion     // one for each shelf's own 换一批
		err             string
	}
	square struct {
		cat     int
		pls     []qqmusic.Playlist
		page    int
		total   int
		loading bool
		err     string
	}
	charts struct {
		loaded, loading bool
		groups          []qqmusic.ToplistGroup
		err             string
	}
	// discoverTab is the tab of 音乐馆 shown: 0 排行榜, 1 歌单广场. It is
	// not the page's, so back from a chart it is the one left.
	discoverTab int

	search struct {
		input   string
		query   string
		songs   []qqmusic.Song
		total   int
		page    int
		loading bool
		err     string
		hot     []string
		list    ui.ListState
		gen     int
		tab     searchTab
		direct  *qqmusic.Direct // 直达, from the first page of the songs
		singers pager[qqmusic.FoundSinger]
		pls     pager[qqmusic.Playlist]
		// 播客, searched beside the songs as a query is new: QQ Music's,
		// and 小宇宙's when logged in, which the tab shows first.
		podcasts        pager[qqmusic.Show]
		xiaoyuzhouShows pager[xiaoyuzhou.Podcast]
		// What hangs below the box: the searches made before it is typed
		// in, and what is typed may become. open stays as the box is
		// focused. typing is closed when the text changes, dropping the
		// rest it waited for; sugGen drops what the text before asked the
		// service for. sel is the suggestion the arrows are on, -1 for none.
		// focus is the box's FocusBind, to let the focus go.
		open    bool
		focused bool
		focus   bool
		typing  chan struct{}
		suggest []string
		sugFor  string // the text the suggestions are for
		sugGen  int
		sel     int
	}
	lists      map[page]*songList
	main       ui.ScrollState
	podcast    podcasts        // 小宇宙
	xiaoyuzhou xiaoyuzhouState // 小宇宙 官方

	// 设置: the section shown, of settingsSections, and the song cache.
	settingsTab int
	cacheUsed   int64 // the bytes the song cache takes, as last measured; -1 until then
	cacheBusy   bool  // the cache is being cleared, trimmed or measured

	// The user.
	user      qqmusic.User
	loggingIn bool // checking a saved login at start
	myLists   []qqmusic.Playlist
	favLists  []qqmusic.Playlist
	liked     map[int64]bool
	liking    map[int64]bool // the likes sent and not yet done, as liked has them
	userMenu  bool

	// 最近播放, the song played last first.
	recent     []qqmusic.Song
	recentView songList

	// Playback, which playback.go runs: the queue, the song loading or
	// playing and how that went, and the volume.
	queue        playqueue.Queue
	radioLoading bool
	playGen      int // changes as another song loads, to drop what the one before got
	songTrack    int // how far the bar's track of songs went: up one for the next song, down one for the one before
	playErr      string
	failStreak   int             // the songs that failed in a row
	downloaded   bool            // the song playing finished downloading
	gotQuality   qqmusic.Quality // of the song last loaded, which asked for askedQuality
	askedQuality qqmusic.Quality
	qualityKnown bool
	volume       float64
	muted        bool

	// The lyrics of the song playing, the line it is at, its comments,
	// and the notes of it when it is an episode of 小宇宙.
	lyrics    []qqmusic.LyricLine
	lyricsFor string
	lyricLine int
	comments  comments
	notes     episodeNotes

	// The view's own state of the player bar and the panels: what is
	// shown, the popups, the slider dragged, the scrolls.
	showLyrics       bool
	showQueue        bool
	modeOpen         bool // the popup of play modes
	qualityOpen      bool // the popup of qualities
	seekValue        float64
	seeking          bool
	volumeWasPressed bool
	lyricShown       int
	lyricList        ui.ListState
	lyricHold        time.Time // the user scrolled the lyrics: follow again after
	queueList        ui.ListState
	lyricsBg         *ui.Bitmap // the cover behind the lyrics, kept as the next comes
	lyricsTint       tintFade   // the color behind it

	// Login.
	login struct {
		open    bool
		tab     int // 0 QQ QR, 1 WeChat QR, 2 QQ account
		qr      *qqmusic.QR
		bmp     *ui.Bitmap
		state   qqmusic.QRState
		err     string
		gen     atomic.Int64
		webBusy bool
	}

	toasts []string // the messages toasted, which the view drops: toasts are off
}

func newApp(plat platform, dir string) *app {
	a := &app{
		plat:    plat,
		api:     qqmusic.New(),
		player:  player.New(),
		store:   store.Open(dir),
		lists:   map[page]*songList{},
		liked:   map[int64]bool{},
		liking:  map[int64]bool{},
		podcast: podcasts{known: map[string]bool{}},
		// 小宇宙's own podcasts, rather than QQ Music's.
		set:       settings{Volume: 0.8, Quality: qqmusic.Best, XiaoyuzhouOfficial: true, CacheGB: 2},
		cacheUsed: -1,
	}
	a.images = newImages(a.api.Fetch, plat.Post, a.trim.soon)
	a.history = []page{{kind: pageHome}}
	a.square.cat = qqmusic.PlaylistCategories[0].ID
	a.lyricLine = -1
	a.load()
	a.loadXiaoyuzhou()
	a.volume = a.set.Volume
	a.player.SetVolume(a.volume)
	// The queue of the last run waits in the player bar, at the song played
	// last, for play to resume it; before queue.json there was only that
	// song. Its lyrics load in Run: nothing may be posted before the window
	// exists.
	if len(a.queue.Songs()) == 0 && len(a.recent) > 0 {
		a.queue = playqueue.Restore([]qqmusic.Song{a.recent[0]}, 0, false)
	}
	return a
}

// go runs work on a goroutine and its result on the main thread.
func (a *app) do(work func(ctx context.Context) func()) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if done := work(ctx); done != nil {
			a.plat.Post(done)
		}
	}()
}

func (a *app) toast(msg string) { a.toasts = append(a.toasts, msg) }

func errText(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, qqmusic.ErrNotLoggedIn) {
		return "需要登录 QQ 音乐账号"
	}
	if errors.Is(err, xiaoyuzhou.ErrNotLoggedIn) {
		return "需要登录小宇宙账号"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "网络请求超时"
	}
	return err.Error()
}
