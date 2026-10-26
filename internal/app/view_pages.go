package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"qqmusic/internal/qqmusic"
)

func (a *app) content(c *ui.Context) {
	p := a.current()
	switch p.kind {
	case pageHome:
		a.homePage(c)
	case pageSquare:
		a.squarePage(c)
	case pageToplists:
		a.chartsPage(c)
	case pageSearch:
		a.searchPage(c)
	case pagePlaylist, pageToplist, pageAlbum:
		a.listPage(c, p)
	case pageSinger:
		a.singerPage(c, p)
	case pageRecent:
		a.recentPage(c, p)
	}
}

// columns is how many cards of at least min wide fit the content area.
func (a *app) columns(c *ui.Context, min float32) int {
	w, _ := c.Size()
	w -= sidebarWidth + 24 + 48
	if a.sidePanel() {
		w -= queueWidth
	}
	return max(2, int(w/min))
}

// pageScroll scrolls a page, behind the top of which, and of the title
// bar, is col, made of its picture by pageTint.
func (a *app) pageScroll(c *ui.Context, s *ui.ScrollState, col ui.Color, fn func()) {
	a.tint.aim(c, col)
	ui.Scroll(c).Grow(1).TrackScroll(s).Children(func() {
		ui.Column(c).Padding(8, 24, 28, 24).Gap(14).Children(fn)
	})
}

// heading is the title of a section of a page, as shadcn's text-lg
// font-semibold.
func heading(c *ui.Context, title string) {
	ui.Text(c, title).FontSize(18).FontWeight(600).LetterSpacing(-0.2).MarginY(6)
}

// errorRow is shadcn's Empty: what went wrong, and a way to try again.
func errorRow(c *ui.Context, msg string, retry func()) {
	ui.Column(c).Gap(12).Padding(24).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Size(40, 40).Radius(radiusLg).Background(colHover).Center().Children(func() {
			ui.Icon(c, icAlert).FontSize(20).TextColor(colText)
		})
		ui.Text(c, msg).FontSize(14).TextColor(colMuted).TextAlign(ui.Center).MaxLines(3)
		if retry != nil {
			if button(c, outline, icRefresh, "重试").Clicked() {
				retry()
			}
		}
	})
}

func greeting() string {
	switch h := time.Now().Hour(); {
	case h < 5:
		return "夜深了"
	case h < 11:
		return "早上好"
	case h < 13:
		return "中午好"
	case h < 18:
		return "下午好"
	default:
		return "晚上好"
	}
}

func (a *app) homePage(c *ui.Context) {
	h := &a.home
	a.pageScroll(c, &a.main, a.pageTint(a.homeCover()), func() {
		ui.Text(c, greeting()).FontSize(26).FontWeight(600).LetterSpacing(-0.6).Margin(6, 0, 4, 0)
		if h.loading && len(h.recs) == 0 {
			skeleton(c, func() {
				if a.api.LoggedIn() {
					headingSkeleton(c)
					forYouSkeleton(c)
				}
				headingSkeleton(c)
				playlistSkeleton(c, a.columns(c, 190), 2)
			})
			return
		}
		if h.err != "" && len(h.recs) == 0 {
			errorRow(c, h.err, func() { a.loadHome(true) })
			return
		}
		if len(h.guess) > 0 || len(h.daily) > 0 {
			heading(c, "为你打造")
			a.forYou(c)
		}
		heading(c, "为你推荐歌单")
		a.playlistGrid(c, h.recs)
		if len(h.newSongs) > 0 {
			ui.Row(c).AlignItems(ui.Center).Children(func() {
				heading(c, "新歌首发")
				ui.Spacer(c)
				if button(c, ghost, icPlay, "播放全部").Clicked() {
					a.playList(h.newSongs, 0)
				}
			})
			a.songCards(c, h.newSongs[:min(len(h.newSongs), 12)])
		}
	})
}

// forYou shows the cards made for the user, as QQ Music's 为你打造:
// 猜你喜欢, a radio that plays on, and 每日30首.
func (a *app) forYou(c *ui.Context) {
	h := &a.home
	ui.Grid(c).Columns(2).Gap(12).Children(func() {
		if len(h.guess) > 0 {
			s := h.guess[0]
			if cur, ok := a.nowPlaying(); ok && a.radio {
				s = cur
			}
			playing := a.radio && a.player.Playing()
			card, play := a.forYouCard(c, "guess", s.Cover(300), icSparkles, "私人电台", "猜你喜欢", a.radio, playing, func() {
				t := ui.RichText(c).FontSize(13).SingleLine().Children(func() {
					ui.Text(c, s.DisplayName()+" - ")
					a.singerLinks(c, s.Singers, 13)
				})
				if a.radio {
					t.TextColor(colAccent)
				}
				ui.Text(c, "越听越懂你，一直播下去").FontSize(12).TextColor(colMuted).SingleLine()
			})
			if card.Clicked() || play.Valid() && play.Clicked() {
				if a.radio {
					a.togglePlay()
				} else {
					a.playRadio()
				}
			}
		}
		if len(h.daily) > 0 {
			p := h.dailyList
			pg := page{kind: pagePlaylist, id: p.ID, title: "每日30首"}
			card, play := a.forYouCard(c, "daily", p.Cover, icClock, time.Now().Format("1月2日")+" 更新", "每日30首", false, false, func() {
				for i, s := range h.daily[:min(2, len(h.daily))] {
					ui.Row(c.Key(i)).Gap(8).MinWidth(0).Children(func() {
						ui.Text(c, fmt.Sprint(i+1)).FontSize(12).FontWeight(500).FontFeatures("tnum").Width(10)
						ui.RichText(c).FontSize(12).TextColor(colMuted).SingleLine().MinWidth(0).Shrink(1).Children(func() {
							ui.Text(c, s.DisplayName()+" - ")
							a.singerLinks(c, s.Singers, 12)
						})
					})
				}
			})
			if play.Valid() && play.Clicked() {
				a.playList(h.daily, 0)
			} else if card.Clicked() {
				a.navigate(pg)
			}
		}
	})
}

// forYouCard is a card of forYou: a cover with a play button, a label, a
// title and lines of body. The play button shows while hovered or on.
func (a *app) forYouCard(c *ui.Context, key, cover string, ic *ui.SVG, label, title string, on, pause bool, body func()) (card, play ui.Element) {
	card = ui.Row(c.Key(key)).Padding(14).Gap(16).Radius(radiusXl).Background(colCard).Border(1, colLine).Cursor(ui.CursorPointer).AlignItems(ui.Center).MinWidth(0)
	hover := card.Hovered()
	if hover {
		card.Background(colCardHover)
	}
	card.Children(func() {
		ui.Box(c).Size(112, 112).Radius(radiusLg).Clip().Children(func() {
			a.cover(c, cover, 112, radiusLg)
			if hover || on {
				play = a.playCircle(c, 40, pause).Absolute().Right(8).Bottom(8)
			}
		})
		ui.Column(c).Grow(1).MinWidth(0).Gap(6).Children(func() {
			ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, ic).FontSize(12).TextColor(colAccent)
				ui.Text(c, label).FontSize(12).FontWeight(500).TextColor(colAccent)
			})
			ui.Text(c, title).FontSize(20).FontWeight(600).LetterSpacing(-0.3).SingleLine()
			body()
		})
	})
	return card, play
}

// homeCover is the picture the home page takes its color from: the song
// playing, or the first one it shows.
func (a *app) homeCover() string {
	h := &a.home
	switch s, ok := a.nowPlaying(); {
	case ok:
		return s.Cover(300)
	case len(h.guess) > 0:
		return h.guess[0].Cover(300)
	case len(h.recs) > 0:
		return h.recs[0].Cover
	}
	return ""
}

// songCards shows songs in a grid of small cards, as shadcn's Item.
func (a *app) songCards(c *ui.Context, songs []qqmusic.Song) {
	cols := min(a.columns(c, 300), 4)
	ui.Grid(c).Columns(cols).Gap(8).Children(func() {
		for i, s := range songs {
			card := ui.Row(c.Key(s.Mid)).Padding(6).Gap(10).Radius(radiusLg).Background(colCard).Border(1, colLine).AlignItems(ui.Center).Cursor(ui.CursorPointer)
			hover := card.Hovered()
			if hover {
				card.Background(colCardHover)
			}
			playing := a.isPlaying(s)
			card.Children(func() {
				a.cover(c, s.Cover(150), 44, radiusSm)
				ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
					t := ui.Text(c, s.DisplayName()).FontSize(14).FontWeight(500).SingleLine()
					if playing {
						t.TextColor(colAccent)
					}
					a.singerLinks(c, s.Singers, 12)
				})
				if hover || playing {
					a.playCircle(c, 32, playing && a.player.Playing()).MarginEnd(4)
				}
			})
			if card.Clicked() {
				a.playList(songs, i)
			}
			a.songMenu(card, s)
		}
	})
}

// playCircle is the round play button of cards, shadcn's primary icon
// button made round.
func (a *app) playCircle(c *ui.Context, size float32, pause bool) ui.Element {
	b := ui.Box(c).Size(size, size).Radius(size/2).Background(colAccent).Center().Cursor(ui.CursorPointer).Shadow(0, 2, 8, 0, colShadow)
	if b.Hovered() {
		b.Background(colAccentH)
	}
	if b.Pressed() {
		b.Scale(0.96, 0.96)
	}
	ic := icPlay
	if pause {
		ic = icPause
	}
	b.Children(func() { ui.Icon(c, ic).FontSize(size * 0.42).TextColor(colAccentFg) })
	return b
}

func (a *app) playlistGrid(c *ui.Context, pls []qqmusic.Playlist) {
	cols := a.columns(c, 190)
	ui.Grid(c).Columns(cols).Gap(12).Children(func() {
		for _, p := range pls {
			a.playlistCard(c, p, page{kind: pagePlaylist, id: p.ID, title: p.Title}, p.Title, p.Creator, p.Cover, p.PlayCount)
		}
	})
}

func (a *app) playlistCard(c *ui.Context, p qqmusic.Playlist, pg page, title, sub, cover string, plays int64) {
	card := ui.Column(c.Key(pg.key())).Padding(8).Gap(8).Radius(radiusXl).Cursor(ui.CursorPointer).MinWidth(0)
	hover := card.Hovered()
	if hover {
		card.Background(colGhost)
	}
	var play ui.Element
	card.Children(func() {
		ui.Box(c).AspectRatio(1).FillWidth().Radius(radiusLg).Clip().Children(func() {
			if bmp := a.images.get(cover, 300); bmp != nil {
				ui.Image(c, bmp).Fill().Fit(ui.Cover)
			} else {
				ui.Box(c).Fill().Background(colHover)
			}
			// A hairline inside the edge, as shadcn's ring-1 ring-foreground/10.
			ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).Radius(radiusLg).Border(1, colLine)
			if plays > 0 {
				ui.Row(c).Absolute().Top(6).Right(6).Padding(1, 6).Radius(radiusSm).Background(ui.RGBA(10, 10, 10, 0.7)).Gap(3).AlignItems(ui.Center).Children(func() {
					ui.Icon(c, icHeadset).FontSize(11).TextColor(colText)
					ui.Text(c, formatCount(plays)).FontSize(11).FontWeight(500)
				})
			}
			if hover {
				play = a.playCircle(c, 40, false).Absolute().Right(8).Bottom(8)
			}
		})
		ui.Column(c).Gap(2).Children(func() {
			ui.Text(c, title).FontSize(14).FontWeight(500).MaxLines(2).LineHeight(1.35)
			if sub != "" {
				ui.Text(c, sub).FontSize(12).TextColor(colMuted).SingleLine()
			}
		})
	})
	if play.Valid() && play.Clicked() {
		a.playPage(pg)
	} else if card.Clicked() {
		a.navigate(pg)
	}
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
		switch pg.kind {
		case pageToplist:
			_, songs, err = a.api.ToplistDetail(ctx, int(pg.id), 0, 300)
		case pageAlbum:
			_, songs, err = a.api.AlbumDetail(ctx, pg.mid)
		default:
			_, songs, err = a.api.AllPlaylistSongs(ctx, pg.id, pg.dirid)
		}
		return func() {
			if err != nil {
				a.toast("加载失败：" + errText(err))
				return
			}
			a.playList(songs, 0)
		}
	})
}

func (a *app) squarePage(c *ui.Context) {
	s := &a.square
	cover := ""
	if len(s.pls) > 0 {
		cover = s.pls[0].Cover
	}
	a.pageScroll(c, &a.main, a.pageTint(cover), func() {
		heading(c, "歌单广场")
		ui.Row(c).Wrap().Gap(8).Children(func() {
			for _, cat := range qqmusic.PlaylistCategories {
				if toggle(c.Key(cat.ID), cat.Name, cat.ID == s.cat).Clicked() && cat.ID != s.cat {
					s.cat = cat.ID
					s.pls = nil
					a.loadSquare(1)
				}
			}
		})
		if s.err != "" && len(s.pls) == 0 {
			errorRow(c, s.err, func() { a.loadSquare(1) })
			return
		}
		if len(s.pls) > 0 {
			a.playlistGrid(c, s.pls)
		}
		switch {
		case s.loading:
			rows := 1
			if len(s.pls) == 0 {
				rows = 3
			}
			skeleton(c, func() { playlistSkeleton(c, a.columns(c, 190), rows) })
		case len(s.pls) < s.total:
			ui.Row(c).Justify(ui.Center).Padding(8).Children(func() {
				if button(c, outline, nil, "加载更多").Clicked() {
					a.loadSquare(s.page + 1)
				}
			})
		}
	})
	// Load more on reaching the end.
	if !s.loading && len(s.pls) > 0 && len(s.pls) < s.total && a.main.MaxY > 0 && a.main.Y >= a.main.MaxY-200 {
		a.loadSquare(s.page + 1)
	}
}

func (a *app) chartsPage(c *ui.Context) {
	ch := &a.charts
	cover := ""
	if len(ch.groups) > 0 && len(ch.groups[0].Lists) > 0 {
		cover = ch.groups[0].Lists[0].Cover
	}
	a.pageScroll(c, &a.main, a.pageTint(cover), func() {
		heading(c, "排行榜")
		if ch.loading {
			skeleton(c, func() {
				a.topChartsSkeleton(c)
				bone(c).Size(120, 18).Radius(radiusSm).Margin(8, 0, 0, 0)
				playlistSkeleton(c, a.columns(c, 170), 2)
			})
			return
		}
		if ch.err != "" {
			errorRow(c, ch.err, func() { a.loadCharts() })
			return
		}
		for gi, g := range ch.groups {
			ui.Text(c.Key(gi), g.Name).FontSize(16).FontWeight(600).Margin(8, 0, 0, 0)
			if gi == 0 {
				a.topCharts(c, g.Lists)
				continue
			}
			cols := a.columns(c, 170)
			ui.Grid(c.Key(g.Name)).Columns(cols).Gap(12).Children(func() {
				for _, t := range g.Lists {
					pg := page{kind: pageToplist, id: int64(t.TopID), title: t.Title}
					a.playlistCard(c, qqmusic.Playlist{}, pg, t.Title, t.Period, t.Cover, t.ListenNum)
				}
			})
		}
	})
}

// topCharts shows the main charts with their first songs, as QQ Music's
// 巅峰榜.
func (a *app) topCharts(c *ui.Context, lists []qqmusic.Toplist) {
	cols := min(a.columns(c, 320), 3)
	ui.Grid(c).Columns(cols).Gap(12).Children(func() {
		for _, t := range lists {
			pg := page{kind: pageToplist, id: int64(t.TopID), title: t.Title}
			card := ui.Row(c.Key(t.TopID)).Padding(12).Gap(14).Radius(radiusXl).Background(colCard).Border(1, colLine).Cursor(ui.CursorPointer).AlignItems(ui.Center).MinWidth(0)
			hover := card.Hovered()
			if hover {
				card.Background(colCardHover)
			}
			var play ui.Element
			card.Children(func() {
				ui.Box(c).Size(96, 96).Radius(radiusLg).Clip().Children(func() {
					a.cover(c, t.Cover, 96, radiusLg)
					if hover {
						play = a.playCircle(c, 36, false).Absolute().Right(6).Bottom(6)
					}
				})
				ui.Column(c).Grow(1).MinWidth(0).Gap(6).Children(func() {
					ui.Text(c, t.Title).FontSize(15).FontWeight(600).SingleLine()
					for i, line := range t.Preview[:min(3, len(t.Preview))] {
						ui.Row(c.Key(i)).Gap(8).MinWidth(0).Children(func() {
							ui.Text(c, fmt.Sprint(i+1)).FontSize(12).FontWeight(500).TextColor(colText).FontFeatures("tnum").Width(10)
							ui.Text(c, line).FontSize(12).TextColor(colMuted).SingleLine().MinWidth(0).Shrink(1)
						})
					}
				})
			})
			if play.Valid() && play.Clicked() {
				a.playPage(pg)
			} else if card.Clicked() {
				a.navigate(pg)
			}
		}
	})
}

// listPage shows a playlist, a chart or an album.
func (a *app) listPage(c *ui.Context, p page) {
	l := a.lists[p.key()]
	if l == nil {
		a.loadList(p)
		l = a.lists[p.key()]
	}
	if l.loading && len(l.songs) == 0 {
		a.listSkeleton(c, p)
		return
	}
	if l.err != "" && len(l.songs) == 0 {
		errorRow(c, l.err, func() { a.loadList(p) })
		return
	}
	a.songTable(c, &l.list, l.songs, func() { a.listHeader(c, p, l) }, nil, true)
}

// recentPage shows 最近播放 as a playlist.
func (a *app) recentPage(c *ui.Context, p page) {
	l := &a.recentView
	l.info.Title, l.songs = "最近播放", a.recent
	a.songTable(c, &l.list, l.songs, func() { a.listHeader(c, p, l) }, nil, true)
}

func (a *app) listHeader(c *ui.Context, p page, l *songList) {
	info := l.info
	a.tint.aim(c, a.listTint(p, l))
	ui.Column(c).Gap(16).Padding(12, 24, 16, 24).Children(func() {
		ui.Row(c).Gap(20).AlignItems(ui.End).Children(func() {
			cover := info.Cover
			// 我喜欢 and 最近播放 show the cover of their first song.
			ownList := p.dirid == 201 || p.kind == pageRecent
			const side = 168
			switch {
			case p.kind == pageSinger:
				a.cover(c, cover, side, side/2).Shadow(0, 8, 24, 0, colShadow)
			case ownList && len(l.songs) > 0:
				a.cover(c, l.songs[0].Cover(300), side, radiusXl).Shadow(0, 8, 24, 0, colShadow)
			case p.dirid == 201 || p.kind == pageRecent:
				ic, col := icHeartOn, colAccent
				if p.kind == pageRecent {
					ic, col = icClock, colText
				}
				ui.Box(c).Size(side, side).Radius(radiusXl).Background(colCard).Border(1, colLine).Center().Children(func() {
					ui.Icon(c, ic).FontSize(56).TextColor(col)
				})
			default:
				a.cover(c, cover, side, radiusXl).Shadow(0, 8, 24, 0, colShadow)
			}
			ui.Column(c).Grow(1).MinWidth(0).Gap(8).Children(func() {
				kind := "歌单"
				switch {
				case l.isChart:
					kind = "排行榜"
				case p.kind == pageSinger:
					kind = "歌手"
				case p.kind == pageAlbum:
					kind = "专辑"
					if l.album.Type != "" && l.album.Type != "录音室专辑" {
						kind = l.album.Type
					}
				case p.kind == pageRecent:
					kind = fmt.Sprintf("最近 %d 首", recentMax)
				}
				ui.Text(c, kind).FontSize(12).FontWeight(500).Padding(2, 8).Radius(radiusSm).Background(colHover).AlignSelf(ui.Start).NoWrap()
				title := info.Title
				if title == "" {
					title = p.title
				}
				ui.Text(c, title).FontSize(32).FontWeight(700).LetterSpacing(-0.8).MaxLines(2).LineHeight(1.15)
				if info.Desc != "" {
					ui.Text(c, strings.ReplaceAll(info.Desc, "\n", " ")).FontSize(13).TextColor(colMuted).MaxLines(2).Tooltip(info.Desc)
				}
				var meta []string
				if p.kind == pageAlbum {
					for _, m := range []string{l.album.PublishDate, l.album.Company} {
						if m != "" {
							meta = append(meta, m)
						}
					}
				}
				if info.Creator != "" {
					meta = append(meta, info.Creator)
				}
				if l.period != "" {
					meta = append(meta, "更新于 "+l.period)
				}
				meta = append(meta, fmt.Sprintf("%d 首歌曲", max(len(l.songs), l.total)))
				if info.PlayCount > 0 {
					meta = append(meta, formatCount(info.PlayCount)+" 次播放")
				}
				line := strings.Join(meta, " · ")
				if p.kind == pageAlbum && len(l.album.Singers) > 0 {
					ui.Row(c).MinWidth(0).AlignItems(ui.Center).Children(func() {
						a.singerLinks(c, l.album.Singers, 13).Shrink(1)
						ui.Text(c, " · "+line).FontSize(13).TextColor(colMuted).NoWrap()
					})
					return
				}
				ui.Text(c, line).FontSize(13).TextColor(colMuted)
			})
		})
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			if button(c, primary, icPlay, "播放全部").Clicked() {
				a.playList(l.songs, 0)
			}
			if l.loading {
				ui.Spinner(c).Size(16, 16)
			}
		})
	})
}

// listTint is the color of a list's header, made of the picture it shows.
func (a *app) listTint(p page, l *songList) ui.Color {
	ownList := p.dirid == 201 || p.kind == pageRecent
	switch {
	case p.kind == pageSinger:
		return a.pageTint(l.info.Cover)
	case ownList && len(l.songs) > 0:
		return a.pageTint(l.songs[0].Cover(300))
	case p.dirid == 201:
		return tint(colAccent)
	case p.kind == pageRecent:
		return tintNone
	}
	return a.pageTint(l.info.Cover)
}

func (a *app) isPlaying(s qqmusic.Song) bool {
	cur, ok := a.nowPlaying()
	return ok && cur.Mid == s.Mid
}

// songTable shows songs as rows below a header, in one virtual list.
func (a *app) songTable(c *ui.Context, ls *ui.ListState, songs []qqmusic.Song, header, footer func(), album bool) {
	extra := 2 // the header and the column titles
	n := len(songs) + extra
	if footer != nil {
		n++
	}
	ui.List(c, ls, n, func(i int) {
		switch {
		case i == 0:
			header()
		case i == 1:
			a.columnTitles(c)
		case i-extra < len(songs):
			a.songRow(c, songs, i-extra)
		default:
			footer()
		}
	}).Grow(1).Padding(0, 0, 24, 0).Children(func() {})
	if len(songs) == 0 {
		ui.Text(c, "暂无歌曲").TextColor(colMuted).Padding(24)
	}
}

// columnTitles is the head of the songs, as shadcn's TableHeader: a line
// below it as below every row.
func (a *app) columnTitles(c *ui.Context) {
	ui.Row(c).Height(40).Margin(0, 24).Padding(0, 16).Gap(16).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(colLine).Children(func() {
		muted := func(s string) ui.Element { return ui.Text(c, s).FontSize(13).FontWeight(500).TextColor(colMuted) }
		muted("#").Width(32).TextAlign(ui.End)
		muted("标题").Grow(3).Basis(0)
		muted("歌手").Grow(2).Basis(0)
		muted("专辑").Grow(2).Basis(0)
		ui.Box(c).Width(32)
		ui.Box(c).Width(48).AlignItems(ui.End).Children(func() { ui.Icon(c, icClock).FontSize(14).TextColor(colMuted) })
	})
}

func (a *app) songRow(c *ui.Context, songs []qqmusic.Song, i int) {
	s := songs[i]
	playing := a.isPlaying(s)
	row := ui.Row(c.Key(s.Mid+fmt.Sprint(i))).Margin(0, 24).Padding(6, 16).Gap(16).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(colLine)
	hover := row.Hovered()
	switch {
	case hover:
		row.Background(colGhost)
	case playing:
		row.Background(ui.RGBA(255, 255, 255, 0.035))
	}
	row.Children(func() {
		ui.Box(c).Width(32).AlignItems(ui.End).Children(func() {
			switch {
			case hover:
				ic := icPlay
				if playing && a.player.Playing() {
					ic = icPause
				}
				if ui.Box(c).Cursor(ui.CursorPointer).Children(func() { ui.Icon(c, ic).FontSize(14) }).Clicked() {
					if playing {
						a.togglePlay()
					} else {
						a.playList(songs, i)
					}
				}
			case playing:
				a.equalizer(c)
			default:
				ui.Text(c, fmt.Sprint(i+1)).FontSize(13).TextColor(colMuted).FontFeatures("tnum")
			}
		})
		ui.Row(c).Grow(3).Basis(0).MinWidth(0).Gap(12).AlignItems(ui.Center).Children(func() {
			a.cover(c, s.Cover(150), 36, radiusSm)
			ui.Row(c).Grow(1).MinWidth(0).Gap(6).AlignItems(ui.Center).Children(func() {
				t := ui.Text(c, s.DisplayName()).FontSize(14).FontWeight(500).SingleLine().MinWidth(0).Shrink(1)
				if playing {
					t.TextColor(colAccent)
				}
				// Members play VIP songs as any other: they need no mark.
				if s.VIP() && !a.user.VIP {
					badge(c, "VIP", colVIP).FontSize(10)
				}
			})
		})
		a.singerLinks(c, s.Singers, 13).Grow(2).Basis(0)
		a.albumLink(c, s.Album, 13).Grow(2).Basis(0)
		liked := a.liked[int64(s.ID)]
		ui.Box(c).Width(32).Center().Children(func() {
			if liked || hover {
				ic, col := icHeart, colMuted
				if liked {
					ic, col = icHeartOn, colAccent
				}
				if ui.Box(c).Cursor(ui.CursorPointer).Tooltip("喜欢").Children(func() { ui.Icon(c, ic).FontSize(15).TextColor(col) }).Clicked() {
					a.toggleLike(s)
				}
			}
		})
		ui.Text(c, s.Duration()).Width(48).TextAlign(ui.End).FontSize(13).TextColor(colMuted).FontFeatures("tnum")
	})
	if row.DoubleClicked() {
		a.playList(songs, i)
	}
	a.songMenu(row, s)
}

// singerLinks shows the singers of a song, each a link to their page, in
// text of size. They are one line of text, cut once at its end where it does
// not fit: as texts of their own, each would shrink to its ellipsis.
func (a *app) singerLinks(c *ui.Context, singers []qqmusic.Singer, size float32) ui.Element {
	return ui.RichText(c).FontSize(size).TextColor(colMuted).SingleLine().MinWidth(0).Children(func() {
		// Past the first two, the others are only marked.
		for k, sg := range singers {
			if k > 0 {
				ui.Text(c.Key(k), " / ")
			}
			if k == 2 {
				ui.Text(c, "…")
				break
			}
			t := ui.Text(c.Key(sg.Mid+fmt.Sprint(k)), sg.Name)
			if sg.Mid == "" {
				continue
			}
			t.Cursor(ui.CursorPointer)
			if t.Hovered() {
				t.TextColor(colText).Underline()
			}
			if t.Clicked() {
				a.navigate(page{kind: pageSinger, mid: sg.Mid, title: sg.Name})
			}
		}
	})
}

// albumLink shows the album of a song, a link to its page, in text of size.
func (a *app) albumLink(c *ui.Context, al qqmusic.Album, size float32) ui.Element {
	return ui.Row(c).MinWidth(0).AlignItems(ui.Center).Children(func() {
		t := ui.Text(c, al.Name).FontSize(size).TextColor(colMuted).SingleLine().MinWidth(0).Shrink(1)
		if al.Mid == "" {
			return
		}
		t.Cursor(ui.CursorPointer)
		if t.Hovered() {
			t.TextColor(colText).Underline()
		}
		if t.Clicked() {
			a.navigate(page{kind: pageAlbum, mid: al.Mid, title: al.Name})
		}
	})
}

// equalizer is the animated bars of the song playing.
func (a *app) equalizer(c *ui.Context) {
	bars := ui.Row(c).Size(14, 14).Gap(2).AlignItems(ui.End)
	on := a.player.Playing()
	bars.Children(func() {
		for k := 0; k < 3; k++ {
			b := ui.Box(c.Key(k)).Width(3).Background(colAccent).Radius(1)
			h := float32(5)
			if on {
				v := b.Loop(k, time.Duration(500+k*170)*time.Millisecond, ui.Bounce(ui.EaseInOut))
				h = 3 + 11*v
			}
			b.Height(h)
		}
	})
}

func (a *app) songMenu(e ui.Element, s qqmusic.Song) {
	e.ContextMenu(func(m *ui.Menu) {
		if m.Item("播放").Chosen() {
			a.playList([]qqmusic.Song{s}, 0)
		}
		if m.Item("下一首播放").Chosen() {
			a.playNext(s)
		}
		if m.Item("添加到播放队列").Chosen() {
			a.enqueue(s)
		}
		m.Separator()
		label := "喜欢"
		if a.liked[int64(s.ID)] {
			label = "取消喜欢"
		}
		if m.Item(label).Chosen() {
			a.toggleLike(s)
		}
		for _, sg := range s.Singers {
			if sg.Mid != "" && m.Item("查看歌手："+sg.Name).Chosen() {
				a.navigate(page{kind: pageSinger, mid: sg.Mid, title: sg.Name})
			}
		}
		if s.Album.Mid != "" && m.Item("查看专辑："+s.Album.Name).Chosen() {
			a.navigate(page{kind: pageAlbum, mid: s.Album.Mid, title: s.Album.Name})
		}
	})
}
