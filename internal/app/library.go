package app

import (
	"context"
	"errors"

	"github.com/egoist/mygo/ui"

	"qqmusic/internal/qqmusic"
)

// The pages of QQ Music's songs: the home page, 音乐馆, and the pages of a
// playlist, a chart, an album or a podcast, and a singer.

// songList is the content of a page of songs: a playlist, a chart, an
// album or a singer's songs. A singer's come a page at a time, total in
// all.
type songList struct {
	info    qqmusic.Playlist
	period  string
	isChart bool
	album   qqmusic.AlbumInfo // of an album's page
	eps     []qqmusic.Episode // of an album's page, songs with their numbers and dates
	songs   []qqmusic.Song
	total   int
	loading bool
	err     string
	list    ui.ListState
	rows    rowsMotion // the rows of the list as the page opens
	// gen changes for the loads on the way to be dropped: as a singer's
	// order changes, or a like changes 我喜欢.
	gen int

	// A singer's page shows their songs or their albums, in order, below
	// its header. What a tab shows comes in with motion.
	tab    singerTab
	order  qqmusic.Order
	albums albumList
	motion [2]tabMotion
}

// rowGone tells which row of a list went as it loaded again, as 取消喜欢
// takes one out: the first index the two differ at, for a list one shorter.
func rowGone(old, now []qqmusic.Song) (int, bool) {
	if len(now)+1 != len(old) {
		return 0, false
	}
	for i := range now {
		if keyOf(old[i]) != keyOf(now[i]) {
			return i, true
		}
	}
	return len(now), true // the last one went
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

func (a *app) loadHome(again bool) {
	h := &a.home
	if h.loading || h.loaded && !again {
		return
	}
	h.loading, h.err = true, ""
	logged := a.api.LoggedIn()
	a.do(func(ctx context.Context) func() {
		recs, more, err1 := a.api.RecommendPlaylists(ctx, 0, recSize)
		songs, err2 := a.api.NewSongs(ctx)
		var guess, daily, radar []qqmusic.Song
		var dailyList qqmusic.Playlist
		var feed []qqmusic.FeedShelf
		var err3, err4, err5, err6 error
		radarMore := false
		if logged {
			guess, err3 = a.api.GuessYouLike(ctx)
			dailyList, daily, err4 = a.api.DailySongs(ctx)
			radar, radarMore, err5 = a.api.RadarSongs(ctx, 1)
			feed, err6 = a.api.RecommendFeed(ctx)
		}
		return func() {
			h.loading, h.loaded = false, true
			// Loaded again, a section that failed keeps what it showed.
			if err1 == nil || len(h.recs) == 0 {
				h.recs = addPlaylists(nil, recs)
				h.recFrom = 0
				if more {
					h.recFrom = recSize
				}
			}
			if err5 == nil {
				h.radar, h.radarPage = radar, 1
				if !radarMore {
					h.radarPage = 0
				}
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
			if err6 == nil {
				h.feed, h.feedMotion = feed, make([]refreshMotion, len(feed))
			}
			h.err = errText(errors.Join(err1, err2))
		}
	})
}

// recSize is how many recommended playlists the home page shows at once.
const recSize = 20

// moreRecs shows the next batch of recommended playlists, and the first
// again after those picked for the user.
func (a *app) moreRecs() {
	h := &a.home
	if h.loading || h.recLoading {
		return
	}
	h.recLoading = true
	from := h.recFrom
	a.do(func(ctx context.Context) func() {
		pls, more, err := a.api.RecommendPlaylists(ctx, from, recSize)
		return func() {
			h.recLoading = false
			if err != nil {
				a.toast("换一批失败：" + errText(err))
				return
			}
			if len(pls) > 0 {
				h.recs = addPlaylists(nil, pls)
				h.recMotion.arm()
			}
			h.recFrom = from + recSize
			if !more || len(pls) == 0 {
				h.recFrom = 0
			}
		}
	})
}

// moreRadar shows the next page of 精选好歌, and the first again after the
// last.
func (a *app) moreRadar() {
	h := &a.home
	if h.loading || h.radarLoading {
		return
	}
	h.radarLoading = true
	pg := h.radarPage + 1
	a.do(func(ctx context.Context) func() {
		songs, more, err := a.api.RadarSongs(ctx, pg)
		return func() {
			h.radarLoading = false
			if err != nil {
				a.toast("换一批失败：" + errText(err))
				return
			}
			if len(songs) > 0 {
				h.radar = songs
				h.radarMotion.arm()
			}
			h.radarPage = pg
			if !more || len(songs) == 0 {
				h.radarPage = 0
			}
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
			// Loaded again, the playlists shown stay if it failed.
			if pg == 1 && err == nil {
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
			ch.loading, ch.err = false, errText(err)
			// Loaded again, the charts that failed keep what they showed.
			if err == nil {
				ch.loaded, ch.groups = true, groups
			}
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
	switch p.kind {
	case pageSinger:
		a.loadSinger(p, l)
		return
	case pageAlbum:
		a.loadAlbum(p, l)
		return
	}
	// Loaded again, as 我喜欢 after a like, a list that failed keeps what
	// it showed, and loads again the next time it is shown.
	kept := func(err error) bool {
		l.loading, l.err = false, errText(err)
		return err != nil && len(l.songs) > 0
	}
	gen := l.gen
	a.do(func(ctx context.Context) func() {
		info, period, songs, err := a.listSongs(ctx, p)
		return func() {
			// A like since changed 我喜欢: a load after it shows it.
			if gen != l.gen {
				return
			}
			if p.kind == pageToplist {
				l.isChart = true
			}
			if kept(err) {
				return
			}
			// 取消喜欢 让列表少了一首：它下面的行从原来的位置滑上来。
			if from, gone := rowGone(l.songs, songs); gone {
				l.rows.shifted(from, songRowHeight)
			}
			l.info, l.period, l.songs = info, period, songs
			if p.dirid == 201 {
				l.info.Title = "我喜欢"
				if err == nil {
					a.setLiked(songs)
				}
			}
		}
	})
}

// listSongs gets a chart or a playlist with all its songs, as its page
// shows them and its play button plays them; a chart's period is when it
// was last updated.
func (a *app) listSongs(ctx context.Context, p page) (info qqmusic.Playlist, period string, songs []qqmusic.Song, err error) {
	if p.kind == pageToplist {
		tl, songs, err := a.api.ToplistDetail(ctx, int(p.id), 0, 300)
		info := qqmusic.Playlist{ID: int64(tl.TopID), Title: tl.Title, Desc: tl.Intro, Cover: tl.Cover, PlayCount: tl.ListenNum, SongCount: len(songs)}
		return info, tl.Period, songs, err
	}
	info, songs, err = a.api.AllPlaylistSongs(ctx, p.id, p.dirid)
	return info, "", songs, err
}

// playPage loads the songs of a playlist or chart and plays them.
func (a *app) playPage(pg page) {
	if l := a.lists[pg.key()]; l != nil && len(l.songs) > 0 {
		a.playList(l.songs, 0)
		return
	}
	if !a.api.LoggedIn() {
		a.playList([]qqmusic.Song{{}}, 0) // asks to log in
		return
	}
	a.toast("正在加载歌单…")
	a.do(func(ctx context.Context) func() {
		var songs []qqmusic.Song
		var err error
		var al qqmusic.AlbumInfo
		switch pg.kind {
		case pageAlbum:
			// A podcast's newest episodes.
			var eps []qqmusic.Episode
			al, eps, _, err = a.api.AlbumDetail(ctx, pg.mid, pg.id)
			songs = qqmusic.EpisodeSongs(eps)
		default:
			_, _, songs, err = a.listSongs(ctx, pg)
		}
		return func() {
			if err != nil {
				a.toast("加载失败：" + errText(err))
				return
			}
			if al.IsPodcast() {
				a.podcast.known[al.Mid] = true
			}
			a.playList(songs, 0)
		}
	})
}

// loadAlbum loads what an album is with its songs, or the next episodes
// of a podcast once its first are loaded: a podcast may have hundreds.
func (a *app) loadAlbum(p page, l *songList) {
	begin, mid := len(l.eps), l.album.Mid
	next := l.album.IsPodcast() && begin > 0
	a.do(func(ctx context.Context) func() {
		var al qqmusic.AlbumInfo
		var eps []qqmusic.Episode
		var total int
		var err error
		if next {
			eps, total, err = a.api.AlbumSongs(ctx, mid, begin, qqmusic.PodcastPage)
		} else {
			al, eps, total, err = a.api.AlbumDetail(ctx, p.mid, p.id)
		}
		return func() {
			l.loading, l.err = false, errText(err)
			// Loaded again, an album that failed keeps what it showed.
			if err != nil || len(l.eps) != begin {
				return
			}
			if !next {
				l.album, l.eps, l.songs = al, nil, nil
				l.info = qqmusic.Playlist{Title: al.Name, Desc: al.Desc, Cover: qqmusic.AlbumCover(al.Mid, 300)}
				if al.IsPodcast() {
					a.podcast.known[al.Mid] = true
				}
			}
			l.eps = append(l.eps, eps...)
			l.songs = append(l.songs, qqmusic.EpisodeSongs(eps)...)
			l.total = total
			if len(eps) == 0 {
				l.total = len(l.eps)
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
