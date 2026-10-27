package app

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"qqmusic/internal/qqmusic"
	"qqmusic/internal/xiaoyuzhou"
)

// xiaoyuzhouSource is the Source of the songs that are episodes of 小宇宙;
// v1.0.1 wrote oldXiaoyuzhouSource, which reads as it.
const (
	xiaoyuzhouSource    = "xiaoyuzhou"
	oldXiaoyuzhouSource = "xyz"
)

// fromQQ reports whether s is QQ Music's, whose account it needs to play,
// to like and to have lyrics: an episode of 小宇宙 needs none of them.
func fromQQ(s qqmusic.Song) bool { return s.Source == "" }

// isXiaoyuzhou reports whether s is an episode of 小宇宙, whose sound and
// page are 小宇宙's.
func isXiaoyuzhou(s qqmusic.Song) bool { return s.Source == xiaoyuzhouSource }

// xiaoyuzhouState is 小宇宙 官方: 小宇宙's own podcasts, through the API of
// its app, rather than those QQ Music has. Its login is its own; a
// podcast's page shows its newest episodes without one. Its episodes play
// as songs of the queue, of Source xiaoyuzhouSource.
type xiaoyuzhouState struct {
	api      *xiaoyuzhou.Client
	user     xiaoyuzhou.User
	checking bool // the saved login is being checked, at start

	// The QR code shown while no one is logged in, polled while the page
	// is; qrGen changes to stop the polling.
	qr        *xiaoyuzhou.QR
	qrBmp     *ui.Bitmap
	qrState   xiaoyuzhou.QRState
	qrErr     string
	qrGen     int
	phoneOpen bool // the window of the login by phone is open

	tab    int // xiaoyuzhouTabFeed…
	chart  int // what 发现 shows: one of xiaoyuzhouCharts, or xiaoyuzhouPicked
	charts [len(xiaoyuzhouCharts)]xiaoyuzhouChart
	picks  pager[xiaoyuzhou.DayPicks]
	feed   xiaoyuzhouFeed
	cats   map[string]*xiaoyuzhouCategory // the pages of categories, by id
	inbox  pager[xiaoyuzhou.Episode]
	subs   pager[xiaoyuzhou.Podcast]
	search struct {
		query string
		tab   int // 0 节目, 1 单集
		shows pager[xiaoyuzhou.Podcast]
		eps   pager[xiaoyuzhou.Episode]
	}
	shows map[string]*xiaoyuzhouShow // the pages of podcasts, by pid
}

// The tabs of 小宇宙 官方.
const (
	xiaoyuzhouTabFeed     = iota // 推荐
	xiaoyuzhouTabDiscover        // 发现
	xiaoyuzhouTabInbox           // 订阅更新
	xiaoyuzhouTabSubs            // 我的订阅
)

// xiaoyuzhouCharts are the charts of 发现; xiaoyuzhouPicked, after them, is
// 编辑精选.
var xiaoyuzhouCharts = [...]xiaoyuzhou.ChartKind{xiaoyuzhou.ChartHot, xiaoyuzhou.ChartRising, xiaoyuzhou.ChartNewStars}

const xiaoyuzhouPicked = len(xiaoyuzhouCharts)

type xiaoyuzhouChart struct {
	chart           xiaoyuzhou.Chart
	loaded, loading bool
	err             string
}

// xiaoyuzhouFeed is 推荐: the sections of the app's front page, and the
// categories above them.
type xiaoyuzhouFeed struct {
	sections        []xiaoyuzhou.Section
	cats            []xiaoyuzhou.Category
	loaded, loading bool
	err             string
}

// xiaoyuzhouCategory is the page of a category: its tabs, a list each.
type xiaoyuzhouCategory struct {
	tabs    []xiaoyuzhou.Category
	tab     int
	lists   []pager[xiaoyuzhou.Entry]
	loading bool // the tabs
	err     string
}

// xiaoyuzhouShow is the page of a podcast. web says it came from the web
// site rather than the API, which gives its newest episodes only.
type xiaoyuzhouShow struct {
	info            xiaoyuzhou.Podcast
	eps             []xiaoyuzhou.Episode
	next            xiaoyuzhou.Cursor
	web             bool
	loaded, loading bool
	err             string
	list            ui.ListState
}

// savedXiaoyuzhou is what xiaoyuzhou.json keeps between runs.
type savedXiaoyuzhou struct {
	Credential xiaoyuzhou.Credential `json:"credential"`
	User       xiaoyuzhou.User       `json:"user"`
}

// episodeSong is an episode as a song of the queue; pod is its podcast,
// for the lists of one podcast, whose episodes do not say.
func episodeSong(e xiaoyuzhou.Episode, pod *xiaoyuzhou.Podcast) qqmusic.Song {
	if e.Podcast != nil {
		pod = e.Podcast
	}
	s := qqmusic.Song{
		Mid: e.Eid, Name: e.Title, Title: e.Title, Interval: e.Duration,
		Album:  qqmusic.Album{Mid: e.Pid},
		Source: xiaoyuzhouSource, URL: e.AudioURL(), Pic: e.Image.URL(300),
	}
	if pod != nil {
		s.Album = qqmusic.Album{Mid: pod.Pid, Name: pod.Title}
		if pod.Author != "" {
			s.Singers = []qqmusic.Singer{{Name: pod.Author}}
		}
		if s.Pic == "" {
			s.Pic = pod.Image.URL(300)
		}
	}
	return s
}

func episodeSongs(eps []xiaoyuzhou.Episode, pod *xiaoyuzhou.Podcast) []qqmusic.Song {
	songs := make([]qqmusic.Song, len(eps))
	for i, e := range eps {
		songs[i] = episodeSong(e, pod)
	}
	return songs
}

// albumOf is the page of a song's album: for an episode of 小宇宙, its
// podcast's.
func albumOf(s qqmusic.Song) page {
	if isXiaoyuzhou(s) {
		return page{kind: pageXiaoyuzhouShow, mid: s.Album.Mid, title: s.Album.Name}
	}
	return page{kind: pageAlbum, mid: s.Album.Mid, title: s.Album.Name}
}

// loadXiaoyuzhou reads the login of 小宇宙, from the file v1.0.1 wrote if
// it is the one there, which moves to its new name; renewed tokens are
// saved.
func (a *app) loadXiaoyuzhou() {
	x := &a.xiaoyuzhou
	var sv savedXiaoyuzhou
	if !a.store.Load(fileXiaoyuzhou, &sv) && a.store.Load(fileOldXiaoyuzhou, &sv) {
		a.store.Save(fileXiaoyuzhou, sv, true)
		a.store.Remove(fileOldXiaoyuzhou)
	}
	x.api = xiaoyuzhou.New(sv.Credential)
	x.api.OnRefresh = func(xiaoyuzhou.Credential) { a.plat.Post(a.saveXiaoyuzhou) }
	x.user = sv.User
	x.shows = map[string]*xiaoyuzhouShow{}
	x.cats = map[string]*xiaoyuzhouCategory{}
}

func (a *app) saveXiaoyuzhou() {
	x := &a.xiaoyuzhou
	a.store.Save(fileXiaoyuzhou, savedXiaoyuzhou{Credential: x.api.Credential(), User: x.user}, true)
}

// startXiaoyuzhou checks the saved login of 小宇宙, at start, and after a
// login gets who it is.
func (a *app) startXiaoyuzhou() {
	x := &a.xiaoyuzhou
	if !x.api.LoggedIn() {
		return
	}
	x.checking = true
	a.do(func(ctx context.Context) func() {
		u, err := x.api.Profile(ctx)
		return func() {
			x.checking = false
			if err != nil {
				a.xiaoyuzhouErr(err)
				return
			}
			x.user = u
			a.saveXiaoyuzhou()
		}
	})
}

// xiaoyuzhouErr is what to show of an error of 小宇宙; a login it turns
// away is forgotten, for the QR code to log in again.
func (a *app) xiaoyuzhouErr(err error) string {
	if errors.Is(err, xiaoyuzhou.ErrNotLoggedIn) && a.xiaoyuzhou.api.LoggedIn() {
		a.logoutXiaoyuzhou()
		a.toast("小宇宙登录已失效，请重新扫码登录")
	}
	return errText(err)
}

// logoutXiaoyuzhou forgets the login of 小宇宙, and what it loaded.
func (a *app) logoutXiaoyuzhou() {
	x := &a.xiaoyuzhou
	x.api.Logout()
	x.user = xiaoyuzhou.User{}
	x.charts = [len(xiaoyuzhouCharts)]xiaoyuzhouChart{}
	x.feed = xiaoyuzhouFeed{}
	x.cats = map[string]*xiaoyuzhouCategory{}
	x.picks.reset()
	x.inbox.reset()
	x.subs.reset()
	a.searchXiaoyuzhou("")
	a.search.xiaoyuzhouShows.reset()
	a.saveXiaoyuzhou()
	if a.current().kind == pageXiaoyuzhou {
		a.newXiaoyuzhouQR()
	}
}

// enterXiaoyuzhou shows 小宇宙 官方: the QR code to log in, or the tab shown.
func (a *app) enterXiaoyuzhou() {
	x := &a.xiaoyuzhou
	if !x.api.LoggedIn() {
		if x.qr == nil && x.qrErr == "" {
			a.newXiaoyuzhouQR()
		}
		return
	}
	a.loadXiaoyuzhouTab()
}

// newXiaoyuzhouQR gets a login QR code, which is polled while 小宇宙 官方
// is shown.
func (a *app) newXiaoyuzhouQR() {
	x := &a.xiaoyuzhou
	x.qrGen++
	gen := x.qrGen
	x.qr, x.qrBmp, x.qrErr, x.qrState = nil, nil, "", xiaoyuzhou.QRWaiting
	a.do(func(ctx context.Context) func() {
		q, err := x.api.NewQR(ctx)
		var bmp *ui.Bitmap
		if err == nil {
			if bmp, err = ui.DecodeBitmap(q.Image); err == nil {
				bmp.Pixelated()
			}
		}
		return func() {
			if gen != x.qrGen {
				return
			}
			if err != nil {
				x.qrErr = "获取二维码失败：" + errText(err)
				return
			}
			x.qr, x.qrBmp = q, bmp
			a.pollXiaoyuzhouQR(gen)
		}
	})
}

// pollXiaoyuzhouQR asks, after a pause, where the login of the QR code is,
// and again until it is done or expired. Once 小宇宙 官方 is left the code
// is dropped: back on it, a new one comes.
func (a *app) pollXiaoyuzhouQR(gen int) {
	x := &a.xiaoyuzhou
	q := x.qr
	a.do(func(ctx context.Context) func() {
		time.Sleep(1500 * time.Millisecond)
		st, err := x.api.PollQR(ctx, q)
		return func() {
			if gen != x.qrGen {
				return
			}
			if a.current().kind != pageXiaoyuzhou {
				x.qrGen++
				x.qr, x.qrBmp = nil, nil
				return
			}
			// A poll that failed on the way is made again; a login
			// confirmed that failed needs a new code.
			x.qrErr = ""
			if err != nil {
				x.qrErr = errText(err)
				if st != xiaoyuzhou.QRDone {
					a.pollXiaoyuzhouQR(gen)
					return
				}
				st = xiaoyuzhou.QRExpired
			}
			x.qrState = st
			switch st {
			case xiaoyuzhou.QRDone:
				a.xiaoyuzhouLoggedIn()
			case xiaoyuzhou.QRWaiting, xiaoyuzhou.QRScanned:
				a.pollXiaoyuzhouQR(gen)
			}
		}
	})
}

// xiaoyuzhouPhoneLogin opens 小宇宙's own login page, by phone number: its
// SMS code needs a captcha only a browser solves.
func (a *app) xiaoyuzhouPhoneLogin() {
	x := &a.xiaoyuzhou
	if x.phoneOpen {
		return
	}
	x.phoneOpen, x.qrErr = true, ""
	a.plat.OpenXiaoyuzhouLogin(func(access, refresh string, err error) {
		a.plat.Post(func() {
			x.phoneOpen = false
			switch {
			case errors.Is(err, errLoginClosed):
			case err != nil:
				x.qrErr = "手机号登录失败：" + errText(err)
			default:
				x.api.SetTokens(access, refresh)
				a.xiaoyuzhouLoggedIn()
			}
		})
	})
}

// xiaoyuzhouLoggedIn is called on the main thread once the QR code, or the
// login page, logged in.
func (a *app) xiaoyuzhouLoggedIn() {
	x := &a.xiaoyuzhou
	x.qrGen++
	x.qr, x.qrBmp = nil, nil
	a.saveXiaoyuzhou()
	a.toast("小宇宙登录成功")
	a.startXiaoyuzhou()
	a.loadXiaoyuzhouTab()
	// The podcasts that came from the web site load again, with all their
	// episodes.
	for pid, sh := range x.shows {
		if sh.web {
			delete(x.shows, pid)
		}
	}
	if p := a.current(); p.kind == pageXiaoyuzhouShow {
		a.loadXiaoyuzhouShow(p.mid, false)
	}
}

// showXiaoyuzhouTab switches 小宇宙 官方 to a tab, which loads.
func (a *app) showXiaoyuzhouTab(tab int) {
	a.xiaoyuzhou.tab = tab
	a.loadXiaoyuzhouTab()
}

// showXiaoyuzhouChart shows a chart of 发现, or 编辑精选, which loads.
func (a *app) showXiaoyuzhouChart(i int) {
	a.xiaoyuzhou.chart = i
	a.loadXiaoyuzhouTab()
}

// loadXiaoyuzhouTab loads what the tab shown shows, or what the search
// found, if it has nothing yet.
func (a *app) loadXiaoyuzhouTab() {
	x := &a.xiaoyuzhou
	switch {
	case !x.api.LoggedIn():
	case x.search.query != "":
		a.loadXiaoyuzhouSearch()
	case x.tab == xiaoyuzhouTabFeed:
		a.loadXiaoyuzhouFeed()
	case x.tab == xiaoyuzhouTabDiscover && x.chart == xiaoyuzhouPicked:
		if x.picks.pages == 0 && !x.picks.loading {
			a.moreXiaoyuzhouPicks()
		}
	case x.tab == xiaoyuzhouTabDiscover:
		a.loadXiaoyuzhouChart(x.chart)
	case x.tab == xiaoyuzhouTabInbox && x.inbox.pages == 0 && !x.inbox.loading:
		a.moreXiaoyuzhouInbox()
	case x.tab == xiaoyuzhouTabSubs && x.subs.pages == 0 && !x.subs.loading:
		a.moreXiaoyuzhouSubs()
	}
}

// refreshXiaoyuzhou loads again what 小宇宙 官方 shows. What was loaded
// stays shown until what replaces it comes.
func (a *app) refreshXiaoyuzhou() {
	x := &a.xiaoyuzhou
	switch {
	case !x.api.LoggedIn():
		a.newXiaoyuzhouQR()
		return
	case x.search.query != "":
		a.searchXiaoyuzhou(x.search.query)
		return
	case x.tab == xiaoyuzhouTabFeed:
		x.feed.loaded = false
	case x.tab == xiaoyuzhouTabDiscover && x.chart == xiaoyuzhouPicked:
		x.picks.reset()
	case x.tab == xiaoyuzhouTabDiscover:
		x.charts[x.chart].loaded = false
	case x.tab == xiaoyuzhouTabInbox:
		x.inbox.reset()
	default:
		x.subs.reset()
	}
	a.loadXiaoyuzhouTab()
}

// xiaoyuzhouLoading reports whether 小宇宙 官方 loads what it shows.
func (a *app) xiaoyuzhouLoading() bool {
	x := &a.xiaoyuzhou
	switch {
	case !x.api.LoggedIn():
		return x.qr == nil && x.qrErr == ""
	case x.search.query != "":
		return x.search.shows.loading || x.search.eps.loading
	case x.tab == xiaoyuzhouTabFeed:
		return x.feed.loading
	case x.tab == xiaoyuzhouTabDiscover && x.chart == xiaoyuzhouPicked:
		return x.picks.loading
	case x.tab == xiaoyuzhouTabDiscover:
		return x.charts[x.chart].loading
	case x.tab == xiaoyuzhouTabInbox:
		return x.inbox.loading
	}
	return x.subs.loading
}

// loadXiaoyuzhouChart loads a chart of 发现, unless it has.
func (a *app) loadXiaoyuzhouChart(i int) {
	ch := &a.xiaoyuzhou.charts[i]
	if ch.loading || ch.loaded {
		return
	}
	ch.loading, ch.err = true, ""
	kind := xiaoyuzhouCharts[i]
	a.do(func(ctx context.Context) func() {
		chart, err := a.xiaoyuzhou.api.Chart(ctx, kind)
		return func() {
			if !ch.loading {
				return // logged out meanwhile
			}
			ch.loading = false
			if err != nil {
				ch.err = a.xiaoyuzhouErr(err)
				return
			}
			ch.chart, ch.loaded = chart, true
		}
	})
}

// loadXiaoyuzhouFeed loads 推荐, the front page read a page after another,
// and the categories, unless it has.
func (a *app) loadXiaoyuzhouFeed() {
	f := &a.xiaoyuzhou.feed
	if f.loading || f.loaded {
		return
	}
	f.loading, f.err = true, ""
	a.do(func(ctx context.Context) func() {
		sections, err := a.xiaoyuzhou.api.Discovery(ctx)
		var cats []xiaoyuzhou.Category
		var catsErr error
		if err == nil {
			cats, catsErr = a.xiaoyuzhou.api.Categories(ctx)
		}
		return func() {
			if !f.loading {
				return // logged out meanwhile
			}
			f.loading = false
			if err != nil {
				f.err = a.xiaoyuzhouErr(err)
				return
			}
			f.sections, f.loaded = sections, true
			if catsErr == nil {
				f.cats = cats
			}
		}
	})
}

// loadXiaoyuzhouCategory loads the page of a category, its tabs and the
// first page of its first, the first time, or again.
func (a *app) loadXiaoyuzhouCategory(id string, again bool) {
	x := &a.xiaoyuzhou
	cat := x.cats[id]
	if cat == nil || again && !cat.loading {
		cat = &xiaoyuzhouCategory{}
		x.cats[id] = cat
	}
	if cat.loading || len(cat.tabs) > 0 {
		return
	}
	cat.loading, cat.err = true, ""
	a.do(func(ctx context.Context) func() {
		tabs, err := x.api.CategoryTabs(ctx, id)
		return func() {
			if x.cats[id] != cat {
				return
			}
			cat.loading = false
			if err != nil {
				cat.err = a.xiaoyuzhouErr(err)
				return
			}
			if len(tabs) == 0 {
				tabs = []xiaoyuzhou.Category{{ID: "ALL", Name: "全部"}}
			}
			cat.tabs, cat.lists = tabs, make([]pager[xiaoyuzhou.Entry], len(tabs))
			a.moreXiaoyuzhouCategory(id)
		}
	})
}

// showXiaoyuzhouCategoryTab shows a tab of a category, which loads.
func (a *app) showXiaoyuzhouCategoryTab(id string, i int) {
	cat := a.xiaoyuzhou.cats[id]
	cat.tab = i
	if l := &cat.lists[i]; l.pages == 0 && !l.loading {
		a.moreXiaoyuzhouCategory(id)
	}
}

// moreXiaoyuzhouCategory loads the next page of the tab of a category
// shown.
func (a *app) moreXiaoyuzhouCategory(id string) {
	cat := a.xiaoyuzhou.cats[id]
	tab := cat.tabs[cat.tab].ID
	xiaoyuzhouMore(a, &cat.lists[cat.tab], func(ctx context.Context, after xiaoyuzhou.Cursor) ([]xiaoyuzhou.Entry, xiaoyuzhou.Cursor, error) {
		return a.xiaoyuzhou.api.CategoryPodcasts(ctx, id, tab, after)
	})
}

// xiaoyuzhouMore loads the next page of p, from the cursor where it
// stopped: none follows once 小宇宙 gives no cursor.
func xiaoyuzhouMore[T any](a *app, p *pager[T], load func(ctx context.Context, after xiaoyuzhou.Cursor) ([]T, xiaoyuzhou.Cursor, error)) {
	if p.loading {
		return
	}
	loadPage(a, p, a.xiaoyuzhouErr, func(ctx context.Context, _ int, after xiaoyuzhou.Cursor) (func(), error) {
		items, next, err := load(ctx, after)
		return func() {
			p.items = append(p.items, items...)
			p.after, p.more = next, next != nil
		}, err
	})
}

func (a *app) moreXiaoyuzhouPicks() {
	xiaoyuzhouMore(a, &a.xiaoyuzhou.picks, a.xiaoyuzhou.api.EditorPicks)
}

func (a *app) moreXiaoyuzhouInbox() {
	xiaoyuzhouMore(a, &a.xiaoyuzhou.inbox, a.xiaoyuzhou.api.Inbox)
}

func (a *app) moreXiaoyuzhouSubs() {
	xiaoyuzhouMore(a, &a.xiaoyuzhou.subs, a.xiaoyuzhou.api.Subscriptions)
}

// searchXiaoyuzhou searches 小宇宙 for q, loading the tab shown; an empty q
// goes back to the tabs of 小宇宙 官方.
func (a *app) searchXiaoyuzhou(q string) {
	x := &a.xiaoyuzhou
	s := &x.search
	s.query = strings.TrimSpace(q)
	s.shows.reset()
	s.eps.reset()
	a.main.Y = 0
	if s.query != "" && !x.api.LoggedIn() {
		a.toast("登录小宇宙后才能搜索")
	}
	a.loadXiaoyuzhouSearch()
}

// showXiaoyuzhouSearchTab switches the search of 小宇宙 to a tab, which
// loads.
func (a *app) showXiaoyuzhouSearchTab(tab int) {
	a.xiaoyuzhou.search.tab = tab
	a.loadXiaoyuzhouSearch()
}

func (a *app) loadXiaoyuzhouSearch() {
	s := &a.xiaoyuzhou.search
	switch {
	case s.query == "" || !a.xiaoyuzhou.api.LoggedIn():
	case s.tab == 0 && s.shows.pages == 0 && !s.shows.loading:
		a.moreXiaoyuzhouShows(&s.shows, s.query)
	case s.tab == 1 && s.eps.pages == 0 && !s.eps.loading:
		a.moreXiaoyuzhouEpisodes()
	}
}

// moreXiaoyuzhouShows loads the next page of the podcasts p found for q:
// those of 小宇宙 官方's search, or those of the 播客 tab of the search page.
func (a *app) moreXiaoyuzhouShows(p *pager[xiaoyuzhou.Podcast], q string) {
	xiaoyuzhouMore(a, p, func(ctx context.Context, after xiaoyuzhou.Cursor) ([]xiaoyuzhou.Podcast, xiaoyuzhou.Cursor, error) {
		return a.xiaoyuzhou.api.SearchPodcasts(ctx, q, after)
	})
}

// moreXiaoyuzhouEpisodes loads the next page of the episodes found.
func (a *app) moreXiaoyuzhouEpisodes() {
	q := a.xiaoyuzhou.search.query
	xiaoyuzhouMore(a, &a.xiaoyuzhou.search.eps, func(ctx context.Context, after xiaoyuzhou.Cursor) ([]xiaoyuzhou.Episode, xiaoyuzhou.Cursor, error) {
		return a.xiaoyuzhou.api.SearchEpisodes(ctx, q, after)
	})
}

// loadXiaoyuzhouShow loads the page of a podcast, the first time, or again
// from its start. Without a login, or should the API fail, its page of the
// web site gives its newest episodes.
func (a *app) loadXiaoyuzhouShow(pid string, again bool) {
	x := &a.xiaoyuzhou
	sh := x.shows[pid]
	if sh == nil || again && !sh.loading {
		sh = &xiaoyuzhouShow{}
		x.shows[pid] = sh
	}
	if sh.loading || sh.loaded {
		return
	}
	sh.loading, sh.err = true, ""
	loggedIn := x.api.LoggedIn()
	a.do(func(ctx context.Context) func() {
		var info xiaoyuzhou.Podcast
		var eps []xiaoyuzhou.Episode
		var next xiaoyuzhou.Cursor
		var err error
		if loggedIn {
			if info, err = x.api.Podcast(ctx, pid); err == nil {
				eps, next, err = x.api.Episodes(ctx, pid, nil)
			}
		}
		web := !loggedIn || err != nil && !errors.Is(err, xiaoyuzhou.ErrNotLoggedIn)
		if web {
			info, eps, err = x.api.WebPodcast(ctx, pid)
			next = nil
		}
		return func() {
			if x.shows[pid] != sh {
				return
			}
			sh.loading = false
			if err != nil {
				sh.err = a.xiaoyuzhouErr(err)
				return
			}
			sh.info, sh.eps, sh.next, sh.web, sh.loaded = info, eps, next, web, true
		}
	})
}

// moreXiaoyuzhouShowEps loads the next page of a podcast's episodes.
func (a *app) moreXiaoyuzhouShowEps(pid string) {
	x := &a.xiaoyuzhou
	sh := x.shows[pid]
	if sh == nil || sh.loading || sh.next == nil {
		return
	}
	sh.loading, sh.err = true, ""
	after := sh.next
	a.do(func(ctx context.Context) func() {
		eps, next, err := x.api.Episodes(ctx, pid, after)
		return func() {
			if x.shows[pid] != sh {
				return
			}
			sh.loading = false
			if err != nil {
				sh.err = a.xiaoyuzhouErr(err)
				return
			}
			sh.eps, sh.next = append(sh.eps, eps...), next
		}
	})
}
