package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"qqmusic/internal/qqmusic"
	"qqmusic/internal/xyz"
)

// xyzSource is the Source of the songs that are episodes of 小宇宙.
const xyzSource = "xyz"

// xyzState is 小宇宙 beta: 小宇宙's own podcasts, through the API of its
// app, rather than those QQ Music has. Its login is its own, by a QR code
// the 小宇宙 app scans; a podcast's page shows its newest episodes without
// one. Its episodes play as songs of the queue, of Source xyzSource.
type xyzState struct {
	api      *xyz.Client
	user     xyz.User
	checking bool // the saved login is being checked, at start

	// The QR code shown while no one is logged in, polled while the page
	// is shown; qrGen changes to stop the polling.
	qr      *xyz.QR
	qrBmp   *ui.Bitmap
	qrState xyz.QRState
	qrErr   string
	qrGen   int
	webBusy bool // the window of the login by phone is open

	tab   int // xyzTabFeed…
	top   int // what 发现 shows: a chart of xyzTops, or xyzPicked
	tops  [len(xyzTops)]xyzTop
	picks xyzList[xyz.PickDay] // 编辑精选
	// 推荐: the sections of the app's front page, and the categories.
	feed struct {
		sections        []xyz.FeedSection
		cats            []xyz.Category
		loaded, loading bool
		err             string
	}
	cats   map[string]*xyzCategory // the pages of categories, by id
	inbox  xyzList[xyz.Episode]
	subs   xyzList[xyz.Podcast]
	search struct {
		query string
		tab   int // 0 节目, 1 单集
		shows xyzList[xyz.Podcast]
		eps   xyzList[xyz.Episode]
	}
	shows map[string]*xyzShow // the pages of podcasts, by pid
}

// The tabs of 小宇宙 beta.
const (
	xyzTabFeed   = iota // 推荐
	xyzTabCharts        // 发现
	xyzTabInbox         // 订阅更新
	xyzTabSubs          // 我的订阅
)

// xyzTops are the charts of 发现.
var xyzTops = [...]struct {
	cat  xyz.TopCategory
	name string
}{{xyz.TopHot, "最热榜"}, {xyz.TopRising, "锋芒榜"}, {xyz.TopNewStars, "新星榜"}}

// xyzPicked is the top of 发现 that shows 编辑精选, after the charts.
const xyzPicked = len(xyzTops)

type xyzTop struct {
	list            xyz.TopList
	loaded, loading bool
	err             string
}

// xyzList is a list loaded a page at a time: next is where it goes on,
// nil at its end; loaded, that its first page came.
type xyzList[T any] struct {
	items           []T
	next            xyz.More
	loaded, loading bool
	err             string
	gen             int
}

// xyzCategory is the page of a category: its tabs, a list each.
type xyzCategory struct {
	tabs    []xyz.CategoryTab
	tab     int
	lists   []xyzList[xyz.FeedItem]
	loading bool // the tabs
	err     string
}

// xyzShow is the page of a podcast. web says it came from the web page
// rather than the API, which has its newest episodes only.
type xyzShow struct {
	info            xyz.Podcast
	eps             []xyz.Episode
	next            xyz.More
	web             bool
	loaded, loading bool
	err             string
	list            ui.ListState
}

// savedXyz is what xyz.json keeps of 小宇宙 between runs.
type savedXyz struct {
	Credential xyz.Credential `json:"credential"`
	User       xyz.User       `json:"user"`
}

// episodeSong is an episode as a song of the queue; pod is its podcast,
// for the lists of one podcast, whose episodes do not say.
func episodeSong(e xyz.Episode, pod *xyz.Podcast) qqmusic.Song {
	if e.Podcast != nil {
		pod = e.Podcast
	}
	s := qqmusic.Song{
		Mid: e.Eid, Name: e.Title, Title: e.Title, Interval: e.Duration,
		Album:  qqmusic.Album{Mid: e.Pid},
		Source: xyzSource, URL: e.Audio(), Pic: e.Image.URL(300),
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

func episodeSongs(eps []xyz.Episode, pod *xyz.Podcast) []qqmusic.Song {
	songs := make([]qqmusic.Song, len(eps))
	for i, e := range eps {
		songs[i] = episodeSong(e, pod)
	}
	return songs
}

// albumOf is the page of a song's album: for an episode of 小宇宙, its
// podcast's.
func albumOf(s qqmusic.Song) page {
	if s.Source == xyzSource {
		return page{kind: pageXyzShow, mid: s.Album.Mid, title: s.Album.Name}
	}
	return page{kind: pageAlbum, mid: s.Album.Mid, title: s.Album.Name}
}

// loadXyz reads the login of 小宇宙; its refreshed tokens are saved.
func (a *app) loadXyz() {
	var sv savedXyz
	if a.dir != "" {
		if b, err := os.ReadFile(a.path("xyz.json")); err == nil {
			json.Unmarshal(b, &sv)
		}
	}
	a.xyz.api = xyz.New(sv.Credential)
	a.xyz.user = sv.User
	a.xyz.shows = map[string]*xyzShow{}
	a.xyz.cats = map[string]*xyzCategory{}
	a.xyz.api.Refreshed = func(xyz.Credential) { a.plat.Post(a.saveXyz) }
}

func (a *app) saveXyz() {
	if a.dir == "" {
		return
	}
	os.MkdirAll(a.dir, 0o700)
	b, _ := json.MarshalIndent(savedXyz{Credential: a.xyz.api.Credential(), User: a.xyz.user}, "", "  ")
	os.WriteFile(a.path("xyz.json"), b, 0o600)
}

// startXyz checks the saved login of 小宇宙, at start.
func (a *app) startXyz() {
	x := &a.xyz
	if !x.api.LoggedIn() {
		return
	}
	x.checking = true
	a.do(func(ctx context.Context) func() {
		u, err := x.api.Profile(ctx)
		return func() {
			x.checking = false
			if err != nil {
				a.xyzErr(err)
				return
			}
			x.user = u
			a.saveXyz()
		}
	})
}

// xyzErr is what to show of an error of 小宇宙; a login it turns away is
// forgotten, for a QR code to log in again.
func (a *app) xyzErr(err error) string {
	x := &a.xyz
	if errors.Is(err, xyz.ErrNotLoggedIn) && x.api.LoggedIn() {
		a.xyzLogout()
		a.toast("小宇宙登录已失效，请重新扫码登录")
	}
	return errText(err)
}

// xyzLogout forgets the login of 小宇宙, and what it loaded.
func (a *app) xyzLogout() {
	x := &a.xyz
	x.api.Logout()
	x.user = xyz.User{}
	x.tops = [len(xyzTops)]xyzTop{}
	x.picks = xyzList[xyz.PickDay]{gen: x.picks.gen + 1}
	x.feed.sections, x.feed.cats, x.feed.loaded, x.feed.loading, x.feed.err = nil, nil, false, false, ""
	x.cats = map[string]*xyzCategory{}
	x.inbox = xyzList[xyz.Episode]{gen: x.inbox.gen + 1}
	x.subs = xyzList[xyz.Podcast]{gen: x.subs.gen + 1}
	a.searchXyz("")
	a.saveXyz()
	if a.current().kind == pageXyz {
		a.newXyzQR()
	}
}

// enterXyz shows 小宇宙 beta: the QR code to log in, or the tab shown.
func (a *app) enterXyz() {
	x := &a.xyz
	if !x.api.LoggedIn() {
		if x.qr == nil && x.qrErr == "" {
			a.newXyzQR()
		}
		return
	}
	a.loadXyzTab()
}

// newXyzQR gets a QR code to log in with and polls it until it is done or
// expires, or 小宇宙 beta is left: back on it, a new code comes.
func (a *app) newXyzQR() {
	x := &a.xyz
	x.qrGen++
	gen := x.qrGen
	x.qr, x.qrBmp, x.qrErr, x.qrState = nil, nil, "", xyz.QRWaiting
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		q, err := x.api.NewQR(ctx)
		cancel()
		var bmp *ui.Bitmap
		if err == nil {
			if bmp, err = ui.DecodeBitmap(q.Image); err == nil {
				bmp.Pixelated()
			}
		}
		alive := make(chan bool, 1)
		a.plat.Post(func() {
			if gen != x.qrGen {
				alive <- false
				return
			}
			if err != nil {
				x.qrErr = "获取二维码失败：" + errText(err)
			} else {
				x.qr, x.qrBmp = q, bmp
			}
			alive <- err == nil
		})
		for <-alive {
			time.Sleep(1500 * time.Millisecond)
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			st, err := x.api.PollQR(ctx, q)
			cancel()
			a.plat.Post(func() {
				if gen != x.qrGen {
					alive <- false
					return
				}
				if a.current().kind != pageXyz {
					x.qrGen++
					x.qr, x.qrBmp = nil, nil
					alive <- false
					return
				}
				// A poll that failed on the way is tried again; a login
				// confirmed that failed needs a new code.
				x.qrErr = ""
				if err != nil {
					x.qrErr = errText(err)
					if st != xyz.QRDone {
						alive <- true
						return
					}
					st = xyz.QRExpired
				}
				x.qrState = st
				switch st {
				case xyz.QRDone:
					a.xyzLoggedIn()
					alive <- false
				case xyz.QRExpired:
					alive <- false
				default:
					alive <- true
				}
			})
		}
	}()
}

// xyzPhoneLogin opens 小宇宙's own login page, by phone number: its SMS
// code needs a captcha that only a browser solves.
func (a *app) xyzPhoneLogin() {
	x := &a.xyz
	if x.webBusy {
		return
	}
	x.webBusy, x.qrErr = true, ""
	a.plat.OpenXyzLogin(func(access, refresh string, err error) {
		a.plat.Post(func() {
			x.webBusy = false
			switch {
			case errors.Is(err, errLoginClosed):
			case err != nil:
				x.qrErr = "手机号登录失败：" + errText(err)
			default:
				x.api.SetTokens(access, refresh)
				a.xyzLoggedIn()
			}
		})
	})
}

// xyzLoggedIn is called on the main thread once the QR code, or the login
// page, logged in.
func (a *app) xyzLoggedIn() {
	x := &a.xyz
	x.qrGen++
	x.qr, x.qrBmp = nil, nil
	a.saveXyz()
	a.toast("小宇宙登录成功")
	a.startXyz()
	a.loadXyzTab()
	// The podcasts shown came from their web pages: they load again with
	// all their episodes.
	for pid, sh := range x.shows {
		if sh.web {
			delete(x.shows, pid)
		}
	}
	if p := a.current(); p.kind == pageXyzShow {
		a.loadXyzShow(p.mid, false)
	}
}

// showXyzTab switches 小宇宙 beta to a tab, which loads.
func (a *app) showXyzTab(tab int) {
	a.xyz.tab = tab
	a.loadXyzTab()
}

// loadXyzTab loads the first page of the tab shown, if it has none.
func (a *app) loadXyzTab() {
	x := &a.xyz
	switch {
	case !x.api.LoggedIn():
	case x.search.query != "":
		a.loadXyzSearch()
	case x.tab == xyzTabFeed:
		a.loadXyzFeed(false)
	case x.tab == xyzTabCharts && x.top == xyzPicked:
		if !x.picks.loaded && !x.picks.loading {
			a.moreXyzPicks()
		}
	case x.tab == xyzTabCharts:
		a.loadXyzTop(x.top, false)
	case x.tab == xyzTabInbox && !x.inbox.loaded && !x.inbox.loading:
		a.moreXyzInbox()
	case x.tab == xyzTabSubs && !x.subs.loaded && !x.subs.loading:
		a.moreXyzSubs()
	}
}

// showXyzTop shows a chart of 发现, or 编辑精选.
func (a *app) showXyzTop(i int) {
	a.xyz.top = i
	a.loadXyzTab()
}

// loadXyzTop loads a chart, the first time, or again.
func (a *app) loadXyzTop(i int, again bool) {
	t := &a.xyz.tops[i]
	if t.loading || t.loaded && !again {
		return
	}
	t.loading, t.err = true, ""
	cat := xyzTops[i].cat
	a.do(func(ctx context.Context) func() {
		l, err := a.xyz.api.Top(ctx, cat)
		return func() {
			// Logged out meanwhile, the charts were dropped.
			if !t.loading {
				return
			}
			t.loading = false
			if err != nil {
				t.err = a.xyzErr(err)
				return
			}
			t.list, t.loaded = l, true
		}
	})
}

// loadXyzFeed loads 推荐: the sections of the app's front page, read a
// page after another, and the categories, the first time, or again.
func (a *app) loadXyzFeed(again bool) {
	f := &a.xyz.feed
	if f.loading || f.loaded && !again {
		return
	}
	f.loading, f.err = true, ""
	a.do(func(ctx context.Context) func() {
		sections, err := a.xyz.api.Feed(ctx)
		var cats []xyz.Category
		var err2 error
		if err == nil {
			cats, err2 = a.xyz.api.Categories(ctx)
		}
		return func() {
			if !f.loading {
				return // logged out meanwhile
			}
			f.loading = false
			// Loaded again, what failed keeps what it showed.
			if err != nil {
				f.err = a.xyzErr(err)
				return
			}
			f.sections, f.loaded = sections, true
			if err2 == nil {
				f.cats = cats
			}
		}
	})
}

// loadXyzCategory loads the page of a category, its tabs and the first
// page of its first, the first time, or again.
func (a *app) loadXyzCategory(id string, again bool) {
	x := &a.xyz
	cat := x.cats[id]
	if cat == nil || again && !cat.loading {
		cat = &xyzCategory{}
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
				cat.err = a.xyzErr(err)
				return
			}
			if len(tabs) == 0 {
				tabs = []xyz.CategoryTab{{ID: "ALL", Name: "全部"}}
			}
			cat.tabs, cat.lists = tabs, make([]xyzList[xyz.FeedItem], len(tabs))
			a.moreXyzCategory(id)
		}
	})
}

// showXyzCategoryTab shows a tab of a category, which loads.
func (a *app) showXyzCategoryTab(id string, i int) {
	cat := a.xyz.cats[id]
	cat.tab = i
	if l := &cat.lists[i]; !l.loaded && !l.loading {
		a.moreXyzCategory(id)
	}
}

// moreXyzCategory loads the next page of the tab of a category shown.
func (a *app) moreXyzCategory(id string) {
	cat := a.xyz.cats[id]
	tab := cat.tabs[cat.tab].ID
	xyzMore(a, &cat.lists[cat.tab], func(ctx context.Context, after xyz.More) ([]xyz.FeedItem, xyz.More, error) {
		return a.xyz.api.CategoryPodcasts(ctx, id, tab, after)
	})
}

// xyzMore loads the next page of f, from where it stopped.
func xyzMore[T any](a *app, f *xyzList[T], load func(ctx context.Context, after xyz.More) ([]T, xyz.More, error)) {
	if f.loading {
		return
	}
	f.loading, f.err = true, ""
	f.gen++
	gen, after := f.gen, f.next
	a.do(func(ctx context.Context) func() {
		items, next, err := load(ctx, after)
		return func() {
			if gen != f.gen {
				return
			}
			f.loading = false
			if err != nil {
				f.err = a.xyzErr(err)
				return
			}
			f.items = append(f.items, items...)
			f.next, f.loaded = next, true
		}
	})
}

func (a *app) moreXyzPicks() { xyzMore(a, &a.xyz.picks, a.xyz.api.EditorPicks) }

func (a *app) moreXyzInbox() { xyzMore(a, &a.xyz.inbox, a.xyz.api.Inbox) }

func (a *app) moreXyzSubs() { xyzMore(a, &a.xyz.subs, a.xyz.api.Subscriptions) }

// searchXyz searches 小宇宙 for q, loading the tab shown; an empty q goes
// back to the tabs of 小宇宙 beta.
func (a *app) searchXyz(q string) {
	s := &a.xyz.search
	s.query = strings.TrimSpace(q)
	s.shows = xyzList[xyz.Podcast]{gen: s.shows.gen + 1}
	s.eps = xyzList[xyz.Episode]{gen: s.eps.gen + 1}
	a.main.Y = 0
	if s.query != "" && !a.xyz.api.LoggedIn() {
		a.toast("登录小宇宙后才能搜索")
	}
	a.loadXyzSearch()
}

// showXyzSearchTab switches the search of 小宇宙 to a tab, which loads.
func (a *app) showXyzSearchTab(tab int) {
	a.xyz.search.tab = tab
	a.loadXyzSearch()
}

func (a *app) loadXyzSearch() {
	s := &a.xyz.search
	switch {
	case s.query == "" || !a.xyz.api.LoggedIn():
	case s.tab == 0 && !s.shows.loaded && !s.shows.loading:
		a.moreXyzShows()
	case s.tab == 1 && !s.eps.loaded && !s.eps.loading:
		a.moreXyzEpisodes()
	}
}

// moreXyzShows loads the next page of the podcasts found.
func (a *app) moreXyzShows() {
	q := a.xyz.search.query
	xyzMore(a, &a.xyz.search.shows, func(ctx context.Context, after xyz.More) ([]xyz.Podcast, xyz.More, error) {
		pods, _, next, err := a.xyz.api.Search(ctx, xyz.SearchPodcasts, q, after)
		return pods, next, err
	})
}

// moreXyzEpisodes loads the next page of the episodes found.
func (a *app) moreXyzEpisodes() {
	q := a.xyz.search.query
	xyzMore(a, &a.xyz.search.eps, func(ctx context.Context, after xyz.More) ([]xyz.Episode, xyz.More, error) {
		_, eps, next, err := a.xyz.api.Search(ctx, xyz.SearchEpisodes, q, after)
		return eps, next, err
	})
}

// loadXyzShow loads the page of a podcast, the first time, or again from
// its start. Without a login, or should the API fail, its web page gives
// its newest episodes.
func (a *app) loadXyzShow(pid string, again bool) {
	x := &a.xyz
	sh := x.shows[pid]
	if sh == nil || again && !sh.loading {
		sh = &xyzShow{}
		x.shows[pid] = sh
	}
	if sh.loading || sh.loaded {
		return
	}
	sh.loading, sh.err = true, ""
	logged := x.api.LoggedIn()
	a.do(func(ctx context.Context) func() {
		var info xyz.Podcast
		var eps []xyz.Episode
		var next xyz.More
		var err error
		if logged {
			info, err = x.api.Podcast(ctx, pid)
			if err == nil {
				eps, next, err = x.api.Episodes(ctx, pid, nil)
			}
		}
		web := !logged || err != nil && !errors.Is(err, xyz.ErrNotLoggedIn)
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
				sh.err = a.xyzErr(err)
				return
			}
			sh.info, sh.eps, sh.next, sh.web, sh.loaded = info, eps, next, web, true
		}
	})
}

// moreXyzShowEps loads the next page of a podcast's episodes.
func (a *app) moreXyzShowEps(pid string) {
	sh := a.xyz.shows[pid]
	if sh == nil || sh.loading || sh.next == nil {
		return
	}
	sh.loading, sh.err = true, ""
	after := sh.next
	a.do(func(ctx context.Context) func() {
		eps, next, err := a.xyz.api.Episodes(ctx, pid, after)
		return func() {
			if a.xyz.shows[pid] != sh {
				return
			}
			sh.loading = false
			if err != nil {
				sh.err = a.xyzErr(err)
				return
			}
			sh.eps = append(sh.eps, eps...)
			sh.next = next
		}
	})
}

// refreshXyz loads again what 小宇宙 beta shows.
func (a *app) refreshXyz() {
	x := &a.xyz
	switch {
	case !x.api.LoggedIn():
		a.newXyzQR()
	case x.search.query != "":
		a.searchXyz(x.search.query)
	case x.tab == xyzTabFeed:
		a.loadXyzFeed(true)
	case x.tab == xyzTabCharts && x.top == xyzPicked:
		x.picks = xyzList[xyz.PickDay]{gen: x.picks.gen + 1}
		a.moreXyzPicks()
	case x.tab == xyzTabCharts:
		a.loadXyzTop(x.top, true)
	case x.tab == xyzTabInbox:
		x.inbox = xyzList[xyz.Episode]{gen: x.inbox.gen + 1}
		a.moreXyzInbox()
	default:
		x.subs = xyzList[xyz.Podcast]{gen: x.subs.gen + 1}
		a.moreXyzSubs()
	}
}

// xyzLoading reports whether 小宇宙 beta loads what it shows.
func (a *app) xyzLoading() bool {
	x := &a.xyz
	switch {
	case !x.api.LoggedIn():
		return x.qr == nil && x.qrErr == ""
	case x.search.query != "":
		return x.search.shows.loading || x.search.eps.loading
	case x.tab == xyzTabFeed:
		return x.feed.loading
	case x.tab == xyzTabCharts && x.top == xyzPicked:
		return x.picks.loading
	case x.tab == xyzTabCharts:
		return x.tops[x.top].loading
	case x.tab == xyzTabInbox:
		return x.inbox.loading
	}
	return x.subs.loading
}
