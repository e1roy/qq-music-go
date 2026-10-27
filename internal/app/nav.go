package app

import "context"

// pageKind is a page of the main area.
type pageKind int

const (
	pageHome pageKind = iota
	// 音乐馆: 排行榜 and 歌单广场, a tab each.
	pageDiscover
	pageSearch
	pagePlaylist // a playlist, 我喜欢 (dirid 201), or a chart
	pageToplist
	pageSinger
	pageRecent // the songs played last, kept on this computer
	pageAlbum  // an album, or a podcast (节目), by its mid or else its id
	pagePodcast
	pageXiaoyuzhou         // 小宇宙 官方: 小宇宙's own podcasts
	pageXiaoyuzhouShow     // a podcast of 小宇宙, by its pid as mid
	pageXiaoyuzhouCategory // a category of 小宇宙's podcasts, by its id as mid
	pageSettings           // 设置, its section by settingsTab
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

// podcastHome is 小宇宙 as the user left it: QQ Music's podcasts, or
// 小宇宙's own.
func (a *app) podcastHome() page {
	if a.set.XiaoyuzhouOfficial {
		return page{kind: pageXiaoyuzhou, title: "小宇宙"}
	}
	return page{kind: pagePodcast, title: "小宇宙"}
}

// switchPodcasts shows 小宇宙 from the other source in place of the page
// shown, as a tab does: back does not switch it again.
func (a *app) switchPodcasts() {
	a.set.XiaoyuzhouOfficial = !a.set.XiaoyuzhouOfficial
	a.saveSettings()
	p := a.podcastHome()
	a.history[len(a.history)-1] = p
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
	case pageDiscover:
		if a.discoverTab == 0 {
			a.loadCharts()
		} else if len(a.square.pls) == 0 {
			a.loadSquare(1)
		}
	case pageSearch:
		if a.search.hot == nil {
			a.do(func(ctx context.Context) func() {
				hot, _ := a.api.HotKeys(ctx)
				return func() { a.search.hot = hot }
			})
		}
		// The tab of what was found, as the 播客 tab of a search made before
		// logging in to 小宇宙, which can look for its podcasts now.
		if a.search.query != "" {
			a.loadSearchTab()
		}
	case pagePlaylist, pageToplist, pageSinger, pageAlbum:
		if l := a.lists[p.key()]; l == nil || l.err != "" {
			a.loadList(p)
		}
	case pagePodcast:
		a.loadPodcasts(false)
	case pageXiaoyuzhou:
		a.enterXiaoyuzhou()
	case pageXiaoyuzhouShow:
		a.loadXiaoyuzhouShow(p.mid, false)
	case pageXiaoyuzhouCategory:
		a.loadXiaoyuzhouCategory(p.mid, false)
	case pageSettings:
		a.measureCache()
	}
}

// refresh loads again what the page shown shows.
func (a *app) refresh() {
	p := a.current()
	switch p.kind {
	case pageHome:
		a.loadHome(true)
	case pageDiscover:
		if a.discoverTab == 0 {
			a.charts.loaded = false
			a.loadCharts()
		} else {
			a.loadSquare(1)
		}
	case pageSearch:
		a.runSearch(a.search.query, a.search.page)
	case pageSinger:
		// Its songs and albums come a page at a time: they start again.
		delete(a.lists, p.key())
		a.loadList(p)
	case pageAlbum:
		// So do the episodes of a podcast.
		if l := a.lists[p.key()]; l != nil && l.album.IsPodcast() && !l.loading {
			delete(a.lists, p.key())
		}
		a.loadList(p)
	case pagePlaylist, pageToplist:
		a.loadList(p)
	case pagePodcast:
		if q := a.podcast.search.query; q != "" {
			a.searchPodcasts(q)
		} else {
			a.loadPodcasts(true)
		}
	case pageXiaoyuzhou:
		a.refreshXiaoyuzhou()
	case pageXiaoyuzhouShow:
		a.loadXiaoyuzhouShow(p.mid, true)
	case pageXiaoyuzhouCategory:
		a.loadXiaoyuzhouCategory(p.mid, true)
	case pageSettings:
		a.measureCache()
	}
}

// refreshing reports whether the page shown is loading: its refresh button
// turns while it does, as a browser's.
func (a *app) refreshing() bool {
	p := a.current()
	switch p.kind {
	case pageHome:
		return a.home.loading
	case pageDiscover:
		if a.discoverTab == 0 {
			return a.charts.loading
		}
		return a.square.loading
	case pageSearch:
		return a.search.loading
	case pagePlaylist, pageToplist, pageSinger, pageAlbum:
		l := a.lists[p.key()]
		return l != nil && l.loading
	case pagePodcast:
		return a.podcast.loading || a.podcast.search.shows.loading || a.podcast.search.episodes.loading
	case pageXiaoyuzhou:
		return a.xiaoyuzhouLoading()
	case pageXiaoyuzhouShow:
		sh := a.xiaoyuzhou.shows[p.mid]
		return sh != nil && sh.loading
	case pageXiaoyuzhouCategory:
		cat := a.xiaoyuzhou.cats[p.mid]
		return cat != nil && (cat.loading || len(cat.lists) > 0 && cat.lists[cat.tab].loading)
	case pageSettings:
		return a.cacheBusy
	}
	return false
}
