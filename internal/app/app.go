// Package app is the QQ Music desktop client: the state of the app, its
// pages and the player, the login, and the views of the native window.
package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo/ui"

	"qqmusic/internal/player"
	"qqmusic/internal/qqmusic"
)

// pageKind is a page of the main area.
type pageKind int

const (
	pageHome pageKind = iota
	pageSquare
	pageToplists
	pageSearch
	pagePlaylist // a playlist, 我喜欢 (dirid 201), or a chart
	pageToplist
	pageSinger
	pageRecent // the songs played last, kept on this computer
	pageAlbum
)

// page is where the main area is; pages form a history.
type page struct {
	kind  pageKind
	id    int64 // playlist id, or chart id
	dirid int
	mid   string // singer or album mid
	title string
}

func (p page) key() page { return page{kind: p.kind, id: p.id, dirid: p.dirid, mid: p.mid} }

// songList is the content of a page of songs: a playlist, a chart, an
// album or a singer's songs. A singer's come a page at a time, total in
// all.
type songList struct {
	info    qqmusic.Playlist
	period  string
	isChart bool
	album   qqmusic.AlbumInfo // of an album's page
	songs   []qqmusic.Song
	total   int
	loading bool
	err     string
	list    ui.ListState

	// A singer's page shows their songs or their albums, in order, below
	// its header; gen changes with the order, for the loads of the one
	// before to be dropped. What a tab shows comes in with motion.
	tab    singerTab
	order  qqmusic.Order
	gen    int
	albums albumList
	motion [2]tabMotion
}

// singerTab is what a singer's page shows.
type singerTab int

const (
	tabSongs singerTab = iota
	tabAlbums
)

// albumList is the albums of a singer, loaded a page at a time.
type albumList struct {
	items   []qqmusic.AlbumInfo
	total   int
	loaded  bool // the first page; total is kept as the order changes
	loading bool
	err     string
}

// searchTab is what the search page shows.
type searchTab int

const (
	foundSongs searchTab = iota
	foundSingers
	foundPlaylists
)

// found is the singers or the playlists a search found, loaded a page at
// a time; page is the last one loaded. Each tab scrolls by itself.
type found[T any] struct {
	items   []T
	total   int
	page    int
	loading bool
	err     string
	gen     int
	list    ui.ListState
}

// playMode is how the queue goes on.
type playMode int

const (
	modeLoop playMode = iota
	modeOne
	modeShuffle
	modeOrder
)

func (m playMode) String() string {
	return [...]string{"列表循环", "单曲循环", "随机播放", "顺序播放"}[m]
}

// settings are what the app remembers between runs, beside the login.
type settings struct {
	Volume  float64         `json:"volume"`
	Quality qqmusic.Quality `json:"quality"`
	Mode    playMode        `json:"mode"`
	// The playlist sections of the sidebar that are folded.
	MineClosed bool `json:"mine_closed"`
	FavClosed  bool `json:"fav_closed"`
}

// recentMax is how many songs 最近播放 keeps.
const recentMax = 100

// savedQueue is the play queue as queue.json keeps it between runs.
type savedQueue struct {
	Songs []qqmusic.Song `json:"songs"`
	Cur   int            `json:"cur"`
	Radio bool           `json:"radio"`
}

// platform is what the app needs of the window, so tests run without one.
type platform interface {
	// Post runs fn on the main thread and draws a frame, from any goroutine.
	Post(fn func())
	// OpenQQLogin opens the QQ login page in a webview and calls done with
	// the cookies of graph.qq.com once logged in, on any goroutine.
	OpenQQLogin(done func(cookies []*cookie, err error))
	// WebViewAvailable reports whether OpenQQLogin can work.
	WebViewAvailable() bool
	// Minimize, ToggleMaximize, Maximized and Close are the window's
	// controls, which the view draws itself.
	Minimize()
	ToggleMaximize()
	Maximized() bool
	Close()
}

type cookie struct{ Name, Value, Domain, Path string }

type app struct {
	plat   platform
	api    *qqmusic.Client
	player *player.Player
	images *images
	tint   tintFade // the color of the top of the page, from its picture
	trim   trimmer
	dir    string // where the login and settings are saved
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
		newSongs        []qqmusic.Song
		guess           []qqmusic.Song
		dailyList       qqmusic.Playlist // 每日30首
		daily           []qqmusic.Song
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
		singers found[qqmusic.FoundSinger]
		pls     found[qqmusic.Playlist]
	}
	lists map[page]*songList
	main  ui.ScrollState

	// The user.
	user      qqmusic.User
	loggingIn bool // checking a saved login at start
	myLists   []qqmusic.Playlist
	favLists  []qqmusic.Playlist
	liked     map[int64]bool
	userMenu  bool

	// 最近播放, the song played last first.
	recent     []qqmusic.Song
	recentView songList

	// Playback.
	queue            []qqmusic.Song
	cur              int  // in queue, -1 for none
	radio            bool // the queue is 猜你喜欢's, which grows as it plays
	radioLoading     bool
	playGen          int
	playErr          string
	gotQuality       qqmusic.Quality // of the song last loaded, which asked for askedQuality
	askedQuality     qqmusic.Quality
	qualityKnown     bool
	lyricsBg         *ui.Bitmap // the cover behind the lyrics, kept as the next comes
	lyricsTint       tintFade   // the color behind it
	seekValue        float64
	seeking          bool
	volume           float64
	muted            bool
	shuffled         []int // the order of modeShuffle
	lyrics           []qqmusic.LyricLine
	lyricsFor        string
	lyricLine        int
	lyricShown       int
	volumeWasPressed bool
	lyricList        ui.ListState
	lyricHold        time.Time // the user scrolled the lyrics: follow again after
	showLyrics       bool
	showQueue        bool
	modeOpen         bool // the popup of play modes
	queueList        ui.ListState
	failStreak       int
	downloaded       bool // the song playing finished downloading
	comments         comments

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

	toasts []string // shown by the view at its next build
}

func newApp(plat platform, dir string) *app {
	a := &app{
		plat:   plat,
		api:    qqmusic.New(),
		player: player.New(),
		dir:    dir,
		lists:  map[page]*songList{},
		liked:  map[int64]bool{},
		cur:    -1,
		set:    settings{Volume: 0.8, Quality: qqmusic.High},
	}
	a.images = newImages(a.api.Fetch, plat.Post, a.trim.soon)
	a.history = []page{{kind: pageHome}}
	a.square.cat = qqmusic.PlaylistCategories[0].ID
	a.lyricLine = -1
	a.load()
	a.volume = a.set.Volume
	a.player.SetVolume(a.volume)
	// The queue of the last run waits in the player bar, at the song played
	// last, for play to resume it; before queue.json there was only that
	// song. Its lyrics load in Run: nothing may be posted before the window
	// exists.
	if len(a.queue) == 0 && len(a.recent) > 0 {
		a.queue, a.cur = []qqmusic.Song{a.recent[0]}, 0
	}
	a.reshuffle()
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
	if errors.Is(err, context.DeadlineExceeded) {
		return "网络请求超时"
	}
	return err.Error()
}

// Persistence.

func (a *app) path(name string) string { return filepath.Join(a.dir, name) }

func (a *app) load() {
	if a.dir == "" {
		return
	}
	if b, err := os.ReadFile(a.path("settings.json")); err == nil {
		json.Unmarshal(b, &a.set)
	}
	if b, err := os.ReadFile(a.path("credential.json")); err == nil {
		var cr qqmusic.Credential
		if json.Unmarshal(b, &cr) == nil && cr.MusicKey != "" {
			a.api.SetCredential(cr)
		}
	}
	if b, err := os.ReadFile(a.path("recent.json")); err == nil {
		json.Unmarshal(b, &a.recent)
	}
	var q savedQueue
	if b, err := os.ReadFile(a.path("queue.json")); err == nil && json.Unmarshal(b, &q) == nil &&
		q.Cur >= 0 && q.Cur < len(q.Songs) {
		a.queue, a.cur, a.radio = q.Songs, q.Cur, q.Radio
	}
}

// saveQueue keeps the queue and the song in it for the next run; an empty
// queue removes the file.
func (a *app) saveQueue() {
	if a.dir == "" {
		return
	}
	if len(a.queue) == 0 {
		os.Remove(a.path("queue.json"))
		return
	}
	os.MkdirAll(a.dir, 0o700)
	b, _ := json.Marshal(savedQueue{Songs: a.queue, Cur: a.cur, Radio: a.radio})
	os.WriteFile(a.path("queue.json"), b, 0o600)
}

func (a *app) saveSettings() {
	if a.dir == "" {
		return
	}
	a.set.Volume = a.volume
	os.MkdirAll(a.dir, 0o700)
	b, _ := json.MarshalIndent(a.set, "", "  ")
	os.WriteFile(a.path("settings.json"), b, 0o600)
}

// addRecent puts a song that played at the top of 最近播放.
func (a *app) addRecent(s qqmusic.Song) {
	recent := make([]qqmusic.Song, 0, recentMax)
	recent = append(recent, s)
	for _, r := range a.recent {
		if r.Mid != s.Mid && len(recent) < recentMax {
			recent = append(recent, r)
		}
	}
	a.recent = recent
	if a.dir == "" {
		return
	}
	os.MkdirAll(a.dir, 0o700)
	b, _ := json.Marshal(a.recent)
	os.WriteFile(a.path("recent.json"), b, 0o600)
}

func (a *app) saveCredential() {
	if a.dir == "" {
		return
	}
	os.MkdirAll(a.dir, 0o700)
	cr := a.api.Credential()
	if cr.MusicKey == "" {
		os.Remove(a.path("credential.json"))
		return
	}
	b, _ := json.MarshalIndent(cr, "", "  ")
	os.WriteFile(a.path("credential.json"), b, 0o600)
}

// Navigation.

func (a *app) current() page { return a.history[len(a.history)-1] }

func (a *app) navigate(p page) {
	// Over the page it goes to, the lyrics would hide it, even if it is
	// the page already shown.
	a.showLyrics = false
	if a.current().key() == p.key() {
		return
	}
	a.history = append(a.history, p)
	a.future = nil
	a.main.Y = 0
	a.enter(p)
}

func (a *app) back() {
	if len(a.history) > 1 {
		a.future = append(a.future, a.current())
		a.history = a.history[:len(a.history)-1]
		a.showLyrics = false
		a.enter(a.current())
	}
}

func (a *app) forward() {
	if n := len(a.future); n > 0 {
		a.history = append(a.history, a.future[n-1])
		a.future = a.future[:n-1]
		a.showLyrics = false
		a.enter(a.current())
	}
}

// enter loads what a page shows, the first time.
func (a *app) enter(p page) {
	switch p.kind {
	case pageHome:
		a.loadHome(false)
	case pageSquare:
		if len(a.square.pls) == 0 {
			a.loadSquare(1)
		}
	case pageToplists:
		a.loadCharts()
	case pageSearch:
		if a.search.hot == nil {
			a.do(func(ctx context.Context) func() {
				hot, _ := a.api.HotKeys(ctx)
				return func() { a.search.hot = hot }
			})
		}
	case pagePlaylist, pageToplist, pageSinger, pageAlbum:
		if l := a.lists[p.key()]; l == nil || l.err != "" {
			a.loadList(p)
		}
	}
}

// Loading.

func (a *app) loadHome(again bool) {
	h := &a.home
	if h.loading || h.loaded && !again {
		return
	}
	h.loading, h.err = true, ""
	logged := a.api.LoggedIn()
	a.do(func(ctx context.Context) func() {
		recs, err1 := a.api.RecommendPlaylists(ctx)
		songs, err2 := a.api.NewSongs(ctx)
		var guess, daily []qqmusic.Song
		var dailyList qqmusic.Playlist
		var err3, err4 error
		if logged {
			guess, err3 = a.api.GuessYouLike(ctx)
			dailyList, daily, err4 = a.api.DailySongs(ctx)
		}
		return func() {
			h.loading, h.loaded = false, true
			// Loaded again, a section that failed keeps what it showed.
			if err1 == nil || len(h.recs) == 0 {
				h.recs = addPlaylists(nil, recs)
			}
			if err2 == nil || len(h.newSongs) == 0 {
				h.newSongs = songs
			}
			if err3 == nil {
				h.guess = guess
			}
			if err4 == nil {
				h.dailyList, h.daily = dailyList, daily
			}
			h.err = errText(errors.Join(err1, err2))
		}
	})
}

func (a *app) loadSquare(pg int) {
	s := &a.square
	if s.loading {
		return
	}
	s.loading, s.err = true, ""
	cat := s.cat
	a.do(func(ctx context.Context) func() {
		pls, total, err := a.api.PlaylistsByCategory(ctx, cat, pg, 40)
		return func() {
			if cat != s.cat {
				s.loading = false
				a.loadSquare(1)
				return
			}
			s.loading, s.err = false, errText(err)
			if pg == 1 {
				s.pls = nil
			}
			s.pls = addPlaylists(s.pls, pls)
			s.page, s.total = pg, total
			// Without the repeated ones, there may be fewer than total: the
			// last page, or an empty one, is the end.
			if err == nil && (len(pls) == 0 || pg*40 >= total) {
				s.total = len(s.pls)
			}
		}
	})
}

// addPlaylists appends to dst the playlists of src it does not have yet.
// A page may repeat one of the page before, as the lists move between
// the two loads; a grid keys its cards by playlist, and must not have
// one twice.
func addPlaylists(dst, src []qqmusic.Playlist) []qqmusic.Playlist {
	seen := make(map[int64]bool, len(dst))
	for _, it := range dst {
		seen[it.ID] = true
	}
	for _, it := range src {
		if !seen[it.ID] {
			seen[it.ID] = true
			dst = append(dst, it)
		}
	}
	return dst
}

func (a *app) loadCharts() {
	ch := &a.charts
	if ch.loading || ch.loaded {
		return
	}
	ch.loading, ch.err = true, ""
	a.do(func(ctx context.Context) func() {
		groups, err := a.api.Toplists(ctx)
		return func() {
			ch.loading, ch.loaded, ch.groups, ch.err = false, err == nil, groups, errText(err)
		}
	})
}

func (a *app) loadList(p page) {
	l := a.lists[p.key()]
	if l == nil {
		l = &songList{}
		a.lists[p.key()] = l
	}
	if l.loading {
		return
	}
	l.loading, l.err = true, ""
	if p.kind == pageSinger {
		a.loadSinger(p, l)
		return
	}
	// Loaded again, as 我喜欢 after a like, a list that failed keeps what
	// it showed, and loads again the next time it is shown.
	kept := func(err error) bool {
		l.loading, l.err = false, errText(err)
		return err != nil && len(l.songs) > 0
	}
	a.do(func(ctx context.Context) func() {
		if p.kind == pageAlbum {
			al, songs, err := a.api.AlbumDetail(ctx, p.mid)
			return func() {
				if kept(err) {
					return
				}
				l.album, l.songs = al, songs
				l.info = qqmusic.Playlist{Title: al.Name, Desc: al.Desc, Cover: qqmusic.AlbumCover(p.mid, 300), SongCount: len(songs)}
			}
		}
		if p.kind == pageToplist {
			tl, songs, err := a.api.ToplistDetail(ctx, int(p.id), 0, 300)
			return func() {
				l.isChart = true
				if kept(err) {
					return
				}
				l.info = qqmusic.Playlist{ID: int64(tl.TopID), Title: tl.Title, Desc: tl.Intro, Cover: tl.Cover, PlayCount: tl.ListenNum, SongCount: len(songs)}
				l.period, l.songs = tl.Period, songs
			}
		}
		info, songs, err := a.api.AllPlaylistSongs(ctx, p.id, p.dirid)
		return func() {
			if kept(err) {
				return
			}
			l.info, l.songs = info, songs
			if p.dirid == 201 {
				l.info.Title = "我喜欢"
				for _, s := range songs {
					a.liked[int64(s.ID)] = true
				}
			}
		}
	})
}

// singerPage is how many songs of a singer a request gets, as many as
// the service gives.
const singerPage = 30

// loadSinger loads who a singer is with their first songs and albums, or
// their next songs once those are loaded.
func (a *app) loadSinger(p page, l *songList) {
	begin, order, gen := len(l.songs), l.order, l.gen
	needInfo := l.info.Title == ""
	if begin == 0 && !l.albums.loaded {
		a.loadAlbums(p, l)
	}
	a.do(func(ctx context.Context) func() {
		var info qqmusic.SingerInfo
		var err error
		if needInfo {
			info, err = a.api.SingerDetail(ctx, p.mid)
		}
		songs, total, err2 := a.api.SingerSongs(ctx, p.mid, order, begin, singerPage)
		return func() {
			if gen != l.gen {
				return
			}
			l.loading, l.err = false, errText(errors.Join(err, err2))
			if needInfo && err == nil {
				l.info = qqmusic.Playlist{Title: info.Name, Desc: info.Desc, Cover: qqmusic.SingerPhoto(p.mid, 300), Creator: info.ForeignName}
			}
			if err2 != nil || len(l.songs) != begin {
				return
			}
			if begin == 0 {
				l.motion[tabSongs].arm()
			}
			l.songs = append(l.songs, songs...)
			l.total, l.info.SongCount = total, total
			if len(songs) == 0 {
				l.total = len(l.songs)
			}
		}
	})
}

// albumsPage is how many albums of a singer a request gets.
const albumsPage = 40

// loadAlbums loads the first albums of a singer, or the next ones.
func (a *app) loadAlbums(p page, l *songList) {
	al := &l.albums
	if al.loading {
		return
	}
	al.loading, al.err = true, ""
	begin, order, gen := len(al.items), l.order, l.gen
	a.do(func(ctx context.Context) func() {
		items, total, err := a.api.SingerAlbums(ctx, p.mid, order, begin, albumsPage)
		return func() {
			if gen != l.gen {
				return
			}
			al.loading, al.err = false, errText(err)
			if err != nil || len(al.items) != begin {
				return
			}
			if begin == 0 {
				l.motion[tabAlbums].arm()
			}
			al.loaded = true
			al.items = append(al.items, items...)
			al.total = total
			if len(items) == 0 {
				al.total = len(al.items)
			}
		}
	})
}

// sortSinger shows the songs and albums of a singer in another order,
// loading them again. It changes what the page's list shows: it is called
// after the build, never in it.
func (a *app) sortSinger(p page, l *songList, order qqmusic.Order) {
	if order == l.order {
		return
	}
	l.order = order
	l.gen++
	// How many there are does not change with the order: the header
	// and the tabs keep showing it.
	l.songs, l.loading, l.err = nil, false, ""
	l.albums = albumList{total: l.albums.total}
	l.motion[l.tab].arm()
	a.loadList(p)
}

// showTab switches a singer's page to a tab, which comes in.
func (a *app) showTab(l *songList, tab singerTab) {
	if tab != l.tab {
		l.tab = tab
		l.motion[tab].arm()
	}
}

func (a *app) runSearch(q string, pg int) {
	if q == "" {
		return
	}
	s := &a.search
	// Turning the pages of the songs keeps the singers and the playlists
	// found, and how many songs there are; a new query knows none yet.
	if q != s.query {
		s.total, s.direct = 0, nil
		s.singers.reset()
		s.pls.reset()
	}
	s.query, s.page, s.loading, s.err = q, pg, true, ""
	a.loadSearchTab()
	s.gen++
	gen := s.gen
	a.do(func(ctx context.Context) func() {
		songs, total, direct, err := a.api.Search(ctx, q, pg, 30)
		return func() {
			if gen != s.gen {
				return
			}
			s.loading, s.songs, s.total, s.err = false, songs, total, errText(err)
			// The other pages may go straight elsewhere, or nowhere.
			if pg == 1 && err == nil {
				s.direct = direct
			}
			s.list.ScrollTo(0, ui.Start)
		}
	})
	a.navigate(page{kind: pageSearch, title: "搜索"})
}

// showSearchTab switches the search page to a tab, which loads.
func (a *app) showSearchTab(tab searchTab) {
	a.search.tab = tab
	a.loadSearchTab()
}

// loadSearchTab loads the first page of the singers or the playlists
// found, as their tab shows, if it has none: not every search wants them,
// and the service turns away searches that come together (code 2001).
func (a *app) loadSearchTab() {
	s := &a.search
	switch {
	case s.tab == foundSingers && s.singers.page == 0 && !s.singers.loading:
		a.searchSingers()
	case s.tab == foundPlaylists && s.pls.page == 0 && !s.pls.loading:
		a.searchPlaylists()
	}
}

// foundPage is how many singers or playlists a page of them has.
const foundPage = 30

// reset forgets what was found, for another query.
func (f *found[T]) reset() {
	f.items, f.total, f.page, f.loading, f.err = nil, 0, 0, false, ""
	f.gen++
	f.list.ScrollTo(0, ui.Start)
}

// searchMore loads the next page of what f found for the query, adding
// what add finds new: pages may be short, or overlap.
func searchMore[T any](a *app, f *found[T], load func(ctx context.Context, q string, pg int) ([]T, int, error), add func(dst, src []T) []T) {
	q, pg := a.search.query, f.page+1
	f.loading, f.err = true, ""
	f.gen++
	gen := f.gen
	a.do(func(ctx context.Context) func() {
		items, total, err := load(ctx, q, pg)
		return func() {
			if gen != f.gen {
				return
			}
			f.loading, f.err = false, errText(err)
			if err != nil {
				return
			}
			f.items = add(f.items, items)
			f.page, f.total = pg, total
			// An empty page is the end.
			if len(items) == 0 {
				f.total = len(f.items)
			}
		}
	})
}

// searchSingers loads the next page of the singers found.
func (a *app) searchSingers() {
	searchMore(a, &a.search.singers, func(ctx context.Context, q string, pg int) ([]qqmusic.FoundSinger, int, error) {
		return a.api.SearchSingers(ctx, q, pg, foundPage)
	}, addSingers)
}

// searchPlaylists loads the next page of the playlists found.
func (a *app) searchPlaylists() {
	searchMore(a, &a.search.pls, func(ctx context.Context, q string, pg int) ([]qqmusic.Playlist, int, error) {
		return a.api.SearchPlaylists(ctx, q, pg, foundPage)
	}, addPlaylists)
}

// addSingers appends to dst the singers of src it does not have yet.
func addSingers(dst, src []qqmusic.FoundSinger) []qqmusic.FoundSinger {
	seen := make(map[string]bool, len(dst))
	for _, it := range dst {
		seen[it.Mid] = true
	}
	for _, it := range src {
		if !seen[it.Mid] {
			seen[it.Mid] = true
			dst = append(dst, it)
		}
	}
	return dst
}

// The user.

// startSession checks a saved login, refreshing it when it is old.
func (a *app) startSession() {
	if !a.api.LoggedIn() {
		return
	}
	a.loggingIn = true
	a.do(func(ctx context.Context) func() {
		if a.api.Credential().NeedsRefresh() {
			a.api.Refresh(ctx)
		}
		u, err := a.api.Profile(ctx)
		if errors.Is(err, qqmusic.ErrNotLoggedIn) {
			if a.api.Refresh(ctx) == nil {
				u, err = a.api.Profile(ctx)
			}
		}
		return func() {
			a.loggingIn = false
			if errors.Is(err, qqmusic.ErrNotLoggedIn) {
				a.api.Logout()
				a.saveCredential()
				a.toast("登录已过期，请重新登录")
				return
			}
			a.saveCredential()
			a.user = u
			a.loadMine()
		}
	})
}

// loggedIn is called on the main thread once a login succeeded.
func (a *app) loggedIn() {
	a.saveCredential()
	a.login.open = false
	a.login.gen.Add(1)
	a.toast("登录成功")
	a.do(func(ctx context.Context) func() {
		u, err := a.api.Profile(ctx)
		return func() {
			if err == nil {
				a.user = u
			}
			a.saveCredential() // with the encrypted uin Profile learned
			a.loadMine()
			a.loadHome(true)
		}
	})
}

func (a *app) loadMine() {
	a.do(func(ctx context.Context) func() {
		mine, err := a.api.CreatedPlaylists(ctx)
		fav, err2 := a.api.FavPlaylists(ctx)
		_, liked, err3 := a.api.AllPlaylistSongs(ctx, 0, 201)
		return func() {
			// What failed keeps what it showed: the playlists in the
			// sidebar, the hearts of the songs liked.
			if err != nil && len(mine) == 0 {
				a.toast("获取歌单失败：" + errText(err))
			} else {
				a.myLists = mine
			}
			if err2 == nil {
				a.favLists = fav
			}
			if err3 == nil {
				a.liked = map[int64]bool{}
				for _, s := range liked {
					a.liked[int64(s.ID)] = true
				}
			}
		}
	})
}

func (a *app) logout() {
	a.api.Logout()
	a.saveCredential()
	a.user = qqmusic.User{}
	a.myLists, a.favLists = nil, nil
	a.liked = map[int64]bool{}
	a.home.guess, a.home.daily = nil, nil
	a.radio = false
	delete(a.lists, page{kind: pagePlaylist, dirid: 201})
	a.toast("已退出登录")
}

func (a *app) toggleLike(s qqmusic.Song) {
	if !a.api.LoggedIn() {
		a.openLogin()
		return
	}
	id := int64(s.ID)
	like := !a.liked[id]
	a.liked[id] = like
	a.do(func(ctx context.Context) func() {
		err := a.api.LikeSong(ctx, s, like)
		return func() {
			if err != nil {
				a.liked[id] = !like
				a.toast("操作失败：" + errText(err))
				return
			}
			if l := a.lists[page{kind: pagePlaylist, dirid: 201}]; l != nil {
				l.loading = false
				a.loadList(page{kind: pagePlaylist, dirid: 201})
			}
			if like {
				a.toast("已添加到我喜欢")
			} else {
				a.toast("已取消喜欢")
			}
		}
	})
}
