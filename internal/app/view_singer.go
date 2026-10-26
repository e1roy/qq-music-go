package app

import (
	"fmt"

	"github.com/egoist/mygo/ui"

	"qqmusic/internal/qqmusic"
)

// singerPage shows who a singer is above their songs or their albums, in
// one list: switching the tab or the order changes only the rows below the
// header, which come in.
func (a *app) singerPage(c *ui.Context, p page) {
	l := a.lists[p.key()]
	if l == nil {
		a.loadList(p)
		l = a.lists[p.key()]
	}
	if l.info.Title == "" && len(l.songs) == 0 {
		if l.loading {
			a.listSkeleton(c, p)
			return
		}
		if l.err != "" {
			errorRow(c, l.err, func() { a.loadList(p) })
			return
		}
	}
	cols := a.columns(c, 190)
	// The rows below the header, the last one their end.
	n := 1 + len(l.songs) + 1 // the column titles, the songs
	if l.tab == tabAlbums {
		n = (len(l.albums.items)+cols-1)/cols + 1
	}
	tab := l.tab
	m := &l.motion[tab]
	ui.List(c, &l.list, 1+n, func(i int) {
		if i == 0 {
			ui.Column(c).Children(func() {
				a.listHeader(c, p, l)
				a.singerBar(c, p, l)
			})
			return
		}
		r := i - 1
		m.row(c, tab, r, func() {
			switch {
			case r == n-1 && tab == tabSongs:
				a.singerSongsEnd(c, p, l)
			case r == n-1:
				a.singerAlbumsEnd(c, p, l, cols)
			case tab == tabSongs && r == 0:
				a.columnTitles(c)
			case tab == tabSongs:
				a.songRow(c, l.songs, r-1)
			default:
				a.albumRow(c, l.albums.items, r, cols)
			}
		})
	}).Grow(1).Padding(0, 0, 24, 0).Children(func() {})
}

// singerBar switches a singer's page between their songs and their
// albums, and their order. The rows of the list change after the build.
func (a *app) singerBar(c *ui.Context, p page, l *songList) {
	ui.Row(c).Padding(0, 24, 12, 24).Gap(4).AlignItems(ui.Center).Children(func() {
		songs, albums := "歌曲", "专辑"
		if l.total > 0 {
			songs += fmt.Sprintf(" %d", l.total)
		}
		if l.albums.loaded || l.albums.total > 0 {
			albums += fmt.Sprintf(" %d", l.albums.total)
		}
		tabs(c, int(l.tab), false, func(i int) {
			if i == 0 {
				a.showTab(l, tabSongs)
			} else {
				a.showTab(l, tabAlbums)
			}
		}, songs, albums)
		ui.Spacer(c)
		for _, o := range []struct {
			order qqmusic.Order
			label string
		}{{qqmusic.Popular, "热门"}, {qqmusic.Latest, "最新"}} {
			t := ui.Text(c.Key(o.label), o.label).FontSize(13).FontWeight(500).Padding(5, 10).Radius(radiusMd).Cursor(ui.CursorPointer)
			switch {
			case o.order == l.order:
				t.Background(colHover).TextColor(colText)
			case t.Hovered():
				t.Background(colGhost).TextColor(colText)
			default:
				t.TextColor(colMuted)
			}
			t.OnClick(func() { a.sortSinger(p, l, o.order) })
		}
	})
}

// singerSongsEnd is the end of a singer's songs: the next ones load as it
// comes into view.
func (a *app) singerSongsEnd(c *ui.Context, p page, l *songList) {
	switch {
	case l.err != "":
		errorRow(c, l.err, func() { a.loadList(p) })
	case l.loading && len(l.songs) == 0:
		skeleton(c, func() { songRowsSkeleton(c, 12) }).Gap(0)
	case l.loading || len(l.songs) < l.total:
		if !l.loading {
			a.loadList(p)
		}
		ui.Row(c).Justify(ui.Center).Padding(16).Children(func() { ui.Spinner(c).Size(18, 18) })
	case len(l.songs) == 0:
		ui.Text(c, "暂无歌曲").TextColor(colMuted).Padding(24)
	}
}

// albumRow is row r of the cards of a singer's albums.
func (a *app) albumRow(c *ui.Context, items []qqmusic.AlbumInfo, r, cols int) {
	ui.Grid(c).Columns(cols).Gap(12).Padding(0, 24, 12, 24).Children(func() {
		for _, it := range items[r*cols : min(len(items), (r+1)*cols)] {
			pg := page{kind: pageAlbum, mid: it.Mid, title: it.Name}
			sub := it.PublishDate
			if it.Type != "" {
				sub += " · " + it.Type
			}
			a.playlistCard(c, qqmusic.Playlist{}, pg, it.Name, sub, qqmusic.AlbumCover(it.Mid, 300), 0)
		}
	})
}

// singerAlbumsEnd is the end of a singer's albums: the first ones load as
// the tab shows, the next ones as it comes into view.
func (a *app) singerAlbumsEnd(c *ui.Context, p page, l *songList, cols int) {
	al := &l.albums
	ui.Column(c).PaddingX(24).Children(func() {
		switch {
		case al.err != "":
			errorRow(c, al.err, func() { a.loadAlbums(p, l) })
		case !al.loaded:
			if !al.loading {
				a.loadAlbums(p, l)
			}
			skeleton(c, func() { playlistSkeleton(c, cols, 2) })
		case al.loading || len(al.items) < al.total:
			if !al.loading {
				a.loadAlbums(p, l)
			}
			ui.Row(c).Justify(ui.Center).Padding(16).Children(func() { ui.Spinner(c).Size(18, 18) })
		case len(al.items) == 0:
			ui.Text(c, "暂无专辑").TextColor(colMuted).Padding(24, 0)
		}
	})
}
