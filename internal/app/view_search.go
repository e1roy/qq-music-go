package app

import (
	"fmt"

	"github.com/egoist/mygo/ui"

	"qqmusic/internal/qqmusic"
)

// searchPage shows what people search for now, or what the query found:
// its songs, singers or playlists, a tab each.
func (a *app) searchPage(c *ui.Context) {
	s := &a.search
	if s.query == "" {
		a.pageScroll(c, &a.main, tintNone, func() {
			heading(c, "热门搜索")
			if s.hot == nil {
				skeleton(c, func() { chipsSkeleton(c) })
				return
			}
			ui.Row(c).Wrap().Gap(8).Children(func() {
				for i, k := range s.hot {
					if toggle(c.Key(i), k, false).Clicked() {
						s.input = k
						a.runSearch(k, 1)
					}
				}
			})
		})
		return
	}
	switch s.tab {
	case foundSingers:
		a.foundSingers(c)
	case foundPlaylists:
		a.foundPlaylists(c)
	default:
		a.foundSongs(c)
	}
}

// searchHeader is the top of every tab of the search page: what was
// searched, how much the tab found, and the tabs.
func (a *app) searchHeader(c *ui.Context) {
	s := &a.search
	ui.Column(c).Padding(8, 24, 8, 24).Gap(4).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(12).Children(func() {
			ui.Text(c, "“"+s.query+"” 的搜索结果").FontSize(22).FontWeight(600).LetterSpacing(-0.4).Grow(1).SingleLine()
			if s.tab == foundSongs && len(s.songs) > 0 {
				if button(c, primary, icPlay, "播放全部").Clicked() {
					a.playList(s.songs, 0)
				}
			}
		})
		if s.direct != nil {
			a.directCard(c, s.direct)
		}
		ui.Row(c).MarginY(10).Gap(12).AlignItems(ui.Center).Children(func() {
			tabs(c, int(s.tab), false, func(i int) { a.showSearchTab(searchTab(i)) }, "单曲", "歌手", "歌单")
			sum := ""
			switch {
			case s.tab == foundSongs && s.total > 0:
				sum = fmt.Sprintf("共找到 %d 首单曲", s.total)
			case s.tab == foundSingers && s.singers.total > 0:
				sum = fmt.Sprintf("共找到 %d 位歌手", s.singers.total)
			case s.tab == foundPlaylists && s.pls.total > 0:
				sum = fmt.Sprintf("共找到 %d 个歌单", s.pls.total)
			}
			if sum != "" {
				ui.Text(c, sum).FontSize(13).TextColor(colMuted)
			}
		})
	})
}

// directCard is what the search goes straight to (直达), as the clients
// show it above the tabs: a singer's photo, or an album's cover, and what
// they have. It opens their page.
func (a *app) directCard(c *ui.Context, d *qqmusic.Direct) {
	card := ui.Row(c.Key("direct")).MarginY(10).Padding(16, 20).Gap(20).Radius(radiusXl).AlignItems(ui.Center).Background(colGhost).Cursor(ui.CursorPointer)
	if card.Hovered() {
		card.Background(ui.RGBA(255, 255, 255, 0.11))
	}
	card.Children(func() {
		const side = 88
		radius := float32(side / 2)
		if d.Kind == qqmusic.DirectAlbum {
			radius = radiusLg
		}
		a.cover(c, d.Pic, side, radius)
		ui.Column(c).Grow(1).MinWidth(0).Gap(8).Children(func() {
			ui.Text(c, d.Label+": "+d.Name).FontSize(20).FontWeight(600).LetterSpacing(-0.2).SingleLine()
			ui.Row(c).Gap(24).MinWidth(0).Children(func() {
				muted := func(s string) ui.Element {
					return ui.Text(c, s).FontSize(14).TextColor(colMuted).FontFeatures("tnum").SingleLine()
				}
				if d.Kind == qqmusic.DirectSinger {
					muted(fmt.Sprintf("单曲：%d", d.SongCount))
					if d.AlbumCount > 0 {
						muted(fmt.Sprintf("专辑：%d", d.AlbumCount))
					}
					if d.MVCount > 0 {
						muted(fmt.Sprintf("视频：%d", d.MVCount))
					}
				}
				if d.Desc != "" {
					muted(d.Desc).MinWidth(0).Shrink(1)
				}
			})
		})
		ui.Icon(c, icForward).FontSize(20).TextColor(colMuted)
	})
	if card.Clicked() {
		if d.Kind == qqmusic.DirectAlbum {
			a.navigate(page{kind: pageAlbum, mid: d.Mid, title: d.Name})
		} else {
			a.navigate(page{kind: pageSinger, mid: d.Mid, title: d.Name})
		}
	}
}

// searchCover is the picture the top of the search page takes its color
// from: what it goes straight to, or else the first that tab found.
func (a *app) searchCover(first string) string {
	if d := a.search.direct; d != nil {
		return d.Pic
	}
	return first
}

// foundSongs is the tab of the songs found, a page of 30 at a time.
func (a *app) foundSongs(c *ui.Context) {
	s := &a.search
	cover := ""
	if len(s.songs) > 0 {
		cover = s.songs[0].Cover(300)
	}
	a.tint.aim(c, a.pageTint(a.searchCover(cover)))
	header := func() { a.searchHeader(c) }
	footer := func() {
		if s.total <= 30 {
			return
		}
		ui.Row(c).Justify(ui.Center).AlignItems(ui.Center).Gap(12).Padding(16).Children(func() {
			if ui.Button(c, "上一页").Disabled(s.page <= 1 || s.loading).Clicked() {
				a.runSearch(s.query, s.page-1)
			}
			ui.Text(c, fmt.Sprintf("第 %d 页", s.page)).FontSize(13).TextColor(colMuted).FontFeatures("tnum")
			if ui.Button(c, "下一页").Disabled(s.page*30 >= s.total || s.loading).Clicked() {
				a.runSearch(s.query, s.page+1)
			}
		})
	}
	if s.loading {
		header()
		ui.Column(c).Grow(1).Clip().Children(func() {
			a.columnTitles(c)
			skeleton(c, func() { songRowsSkeleton(c, 12) }).Gap(0)
		})
		return
	}
	if s.err != "" {
		header()
		errorRow(c, s.err, func() { a.runSearch(s.query, s.page) })
		return
	}
	a.songTable(c, &s.list, s.songs, header, footer, false)
}

// foundSingers is the tab of the singers found.
func (a *app) foundSingers(c *ui.Context) {
	f := &a.search.singers
	cover := ""
	if len(f.items) > 0 {
		cover = f.items[0].Photo
	}
	a.tint.aim(c, a.pageTint(a.searchCover(cover)))
	cols := a.columns(c, 160)
	card := func(sg qqmusic.FoundSinger) { a.singerCard(c, sg) }
	foundGrid(a, c, f, cols, card, func() {
		foundEnd(c, f, "歌手", a.searchSingers, func(rows int) { singerSkeleton(c, cols, rows) })
	})
}

// foundPlaylists is the tab of the playlists found.
func (a *app) foundPlaylists(c *ui.Context) {
	f := &a.search.pls
	cover := ""
	if len(f.items) > 0 {
		cover = f.items[0].Cover
	}
	a.tint.aim(c, a.pageTint(a.searchCover(cover)))
	cols := a.columns(c, 190)
	card := func(p qqmusic.Playlist) {
		a.playlistCard(c, p, page{kind: pagePlaylist, id: p.ID, title: p.Title}, p.Title, p.Creator, p.Cover, p.PlayCount)
	}
	foundGrid(a, c, f, cols, card, func() {
		foundEnd(c, f, "歌单", a.searchPlaylists, func(rows int) { playlistSkeleton(c, cols, rows) })
	})
}

// foundGrid shows what a tab found as rows of cards, cols in a row, below
// the header, in one virtual list, and then end.
func foundGrid[T any](a *app, c *ui.Context, f *found[T], cols int, card func(T), end func()) {
	rows := (len(f.items) + cols - 1) / cols
	ui.List(c, &f.list, 1+rows+1, func(i int) {
		switch {
		case i == 0:
			a.searchHeader(c)
		case i <= rows:
			r := i - 1
			ui.Grid(c).Columns(cols).Gap(12).Padding(0, 24, 12, 24).Children(func() {
				for _, it := range f.items[r*cols : min(len(f.items), (r+1)*cols)] {
					card(it)
				}
			})
		default:
			ui.Column(c).Padding(0, 24, 0, 24).Children(end)
		}
	}).Grow(1).Padding(0, 0, 24, 0).Children(func() {})
}

// foundEnd is the end of what a tab found: its bones as it loads, what
// went wrong, nothing found, or more to load.
func foundEnd[T any](c *ui.Context, f *found[T], what string, more func(), bones func(rows int)) {
	switch {
	case f.loading:
		rows := 1
		if len(f.items) == 0 {
			rows = 3
		}
		skeleton(c, func() { bones(rows) })
	case f.err != "":
		errorRow(c, f.err, more)
	case len(f.items) == 0:
		ui.Text(c, "暂无"+what).TextColor(colMuted).Padding(24, 0)
	case len(f.items) < f.total:
		ui.Row(c).Justify(ui.Center).Padding(8).Children(func() {
			if button(c, outline, nil, "加载更多"+what).Clicked() {
				more()
			}
		})
	}
}

// singerSide is the side of a singer's round photo on their card.
const singerSide = 120

// singerCard is a singer found: their photo, name and how many songs and
// albums they have. It opens their page.
func (a *app) singerCard(c *ui.Context, sg qqmusic.FoundSinger) {
	card := ui.Column(c.Key(sg.Mid)).Padding(12, 8).Gap(10).Radius(radiusXl).AlignItems(ui.Center).Cursor(ui.CursorPointer).MinWidth(0)
	if card.Hovered() {
		card.Background(colGhost)
	}
	card.Children(func() {
		ui.Box(c).Size(singerSide, singerSide).Radius(singerSide / 2).Clip().Children(func() {
			if bmp := a.images.get(sg.Photo, 300); bmp != nil {
				ui.Image(c, bmp).Fill().Fit(ui.Cover)
			} else {
				ui.Box(c).Fill().Background(colHover).Center().Children(func() {
					ui.Icon(c, icUser).FontSize(singerSide * 0.35).TextColor(colDim)
				})
			}
			ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).Radius(singerSide/2).Border(1, colLine)
		})
		ui.Column(c).FillWidth().Gap(2).Children(func() {
			ui.Text(c, sg.Name).FontSize(14).FontWeight(500).SingleLine().TextAlign(ui.Center)
			ui.Text(c, fmt.Sprintf("单曲 %d · 专辑 %d", sg.SongCount, sg.AlbumCount)).FontSize(12).TextColor(colMuted).SingleLine().TextAlign(ui.Center).FontFeatures("tnum")
		})
	})
	if card.Clicked() {
		a.navigate(page{kind: pageSinger, mid: sg.Mid, title: sg.Name})
	}
}
