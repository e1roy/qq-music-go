package app

import (
	"fmt"

	"github.com/egoist/mygo/ui"

	"qqmusic/internal/qqmusic"
)

// podcastsPage is 小宇宙: what the user listened to, the podcasts picked for
// them and the shelves of QQ Music's podcasts, or what the title bar's box
// found of them.
func (a *app) podcastsPage(c *ui.Context) {
	pc := &a.podcast
	a.pageScroll(c, &a.main, a.pageTint(a.podcastCover()), func() {
		ui.Column(c).Margin(6, 0, 4, 0).Gap(4).Children(func() {
			a.podcastTitle(c)
			desc := pc.area.Desc
			if desc == "" {
				desc = "听见闪闪发光的灵魂"
			}
			ui.Text(c, desc).FontSize(13).TextColor(colMuted).SingleLine()
		})
		if pc.search.query != "" {
			a.podcastResults(c)
			return
		}
		empty := len(pc.area.Shelves) == 0 && len(pc.picks) == 0
		if pc.loading && empty {
			skeleton(c, func() {
				headingSkeleton(c)
				playlistSkeleton(c, a.columns(c, 170), 2)
			})
			return
		}
		if pc.err != "" && empty {
			errorRow(c, pc.err, func() { a.loadPodcasts(true) })
			return
		}
		ui.Column(c).Gap(34).Children(func() {
			if eps := a.recentEpisodes(8); len(eps) > 0 {
				ui.Column(c.Key("recent")).Gap(6).Children(func() {
					ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
						heading(c, "最近收听")
						playAllButton(c, func() { a.playList(eps, 0) })
					})
					a.songCards(c, eps, nil)
				})
			}
			if len(pc.picks) > 0 {
				ui.Column(c.Key("picks")).Gap(6).Children(func() {
					ui.Row(c).AlignItems(ui.Center).Children(func() {
						title := "推荐节目"
						if a.api.LoggedIn() {
							title = "为你推荐"
						}
						heading(c, title)
						ui.Spacer(c)
						refreshButton(c, pc.picksLoading, false, func() { a.morePicks() })
					})
					a.picks(c)
				})
			}
			if len(pc.heard) > 0 {
				ui.Column(c.Key("heard")).Gap(6).Children(func() {
					ui.Row(c).AlignItems(ui.Center).Children(func() {
						heading(c, "我听过的节目")
						ui.Spacer(c)
						sectionAction(c, icTrash, "清空", false, func() { a.clearHeard() })
					})
					a.showGrid(c, pc.heard, 2)
				})
			}
			for i := range pc.area.Shelves {
				sh := &pc.area.Shelves[i]
				ui.Column(c.Key(i)).Gap(6).Children(func() {
					ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
						heading(c, sh.Title)
						if len(sh.Episodes) > 0 {
							playAllButton(c, func() { a.playList(sh.Episodes, 0) })
						}
					})
					switch {
					case len(sh.Episodes) > 0:
						a.songCards(c, sh.Episodes, nil)
					case sh.Hosts:
						a.hostGrid(c, sh.Shows)
					default:
						a.showGrid(c, sh.Shows, 0)
					}
				})
			}
		})
	})
}

// podcastCover is the picture 小宇宙 takes its color from: the first
// podcast picked, or else the first show of its shelves.
func (a *app) podcastCover() string {
	pc := &a.podcast
	if len(pc.picks) > 0 {
		return pc.picks[0].Cover
	}
	for _, sh := range pc.area.Shelves {
		if len(sh.Shows) > 0 && !sh.Hosts {
			return sh.Shows[0].Cover
		}
	}
	return ""
}

// podcastResults is what the search of 小宇宙 found: shows or episodes, a
// tab each, a page at a time.
func (a *app) podcastResults(c *ui.Context) {
	s := &a.podcast.search
	ui.Row(c).MarginY(6).Gap(12).AlignItems(ui.Center).Children(func() {
		tabs(c, s.tab, false, func(i int) { a.showPodcastTab(i) }, "节目", "单集")
		ui.Text(c, "“"+s.query+"” 的搜索结果").FontSize(13).TextColor(colMuted).SingleLine()
		ui.Spacer(c)
		sectionAction(c, icClose, "清除搜索", false, func() {
			a.search.input = ""
			a.searchPodcasts("")
		})
	})
	if s.tab == 0 {
		f := &s.shows
		if len(f.items) > 0 {
			a.showGrid(c, f.items, 0)
		}
		cols := a.columns(c, 170)
		podcastEnd(c, f, "节目", a.moreShows, func(rows int) { playlistSkeleton(c, cols, rows) })
		return
	}
	f := &s.episodes
	if len(f.items) > 0 {
		ui.Row(c).AlignItems(ui.Center).Children(func() {
			playAllButton(c, func() { a.playList(f.items, 0) })
		})
		a.songCards(c, f.items, nil)
	}
	podcastEnd(c, f, "单集", a.moreEpisodes, func(rows int) { a.songCardsSkeleton(c, rows) })
}

// podcastEnd is the end of what a tab of the search of 小宇宙 found: its
// bones as it loads, what went wrong, nothing found, or more to load. A
// page may find none of the podcasts' while pages follow.
func podcastEnd[T any](c *ui.Context, f *pager[T], what string, more func(), bones func(rows int)) {
	switch {
	case f.loading:
		rows := 1
		if len(f.items) == 0 {
			rows = 3
		}
		skeleton(c, func() { bones(rows) })
	case f.err != "":
		errorRow(c, f.err, more)
	case f.more:
		ui.Column(c).Padding(8).Gap(8).AlignItems(ui.Center).Children(func() {
			label := "加载更多" + what
			if len(f.items) == 0 {
				ui.Text(c, "前 "+fmt.Sprint(f.pages)+" 页没有找到相关"+what).FontSize(13).TextColor(colMuted)
				label = "继续查找"
			}
			if button(c, outline, nil, label).Clicked() {
				more()
			}
		})
	case len(f.items) == 0:
		ui.Text(c, "没有找到相关"+what).TextColor(colMuted).Padding(24, 0)
	}
}

// picks shows the podcasts picked, each a wide card with what it is about.
// They come in again after 换一批.
func (a *app) picks(c *ui.Context) {
	pc := &a.podcast
	cols := min(a.columns(c, 340), 3)
	ui.Grid(c).Columns(cols).Gap(12).Children(func() {
		for i, sh := range pc.picks {
			pg := pageOf(sh)
			card := ui.Row(c.Key(pg.key())).Padding(12).Gap(14).Radius(radiusXl).Background(colCard).Border(1, colLine).Cursor(ui.CursorPointer).AlignItems(ui.Center).MinWidth(0)
			hover := card.Hovered()
			if hover {
				card.Background(colCardHover)
			}
			var play ui.Element
			card.Children(func() {
				ui.Box(c).Size(96, 96).Radius(radiusLg).Clip().Children(func() {
					a.cover(c, sh.Cover, 96, radiusLg)
					if hover {
						play = a.playCircle(c, 36, false).Absolute().Right(6).Bottom(6)
					}
				})
				ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
					ui.Text(c, sh.Title).FontSize(15).FontWeight(600).SingleLine()
					ui.Row(c).Gap(8).AlignItems(ui.Center).MinWidth(0).Children(func() {
						ui.Text(c, sh.Host).FontSize(12).TextColor(colSubtle).SingleLine().MinWidth(0).Shrink(1)
						if sh.PlayCount > 0 {
							ui.Row(c).Gap(3).AlignItems(ui.Center).Children(func() {
								ui.Icon(c, icHeadset).FontSize(11).TextColor(colMuted)
								ui.Text(c, formatCount(sh.PlayCount)).FontSize(12).TextColor(colMuted).NoWrap()
							})
						}
					})
					if sh.Intro != "" {
						ui.Text(c, sh.Intro).FontSize(12).TextColor(colMuted).MaxLines(2).LineHeight(1.4).Tooltip(sh.Intro)
					}
				})
			})
			if play.Valid() && play.Clicked() {
				a.playPage(pg)
			} else if card.Clicked() {
				a.navigate(pg)
			}
			pc.picksMotion.card(c, i, card)
		}
	})
}

// showGrid shows podcasts as cards, in rows rows, all of them if 0: the
// play button of each plays its newest episodes.
func (a *app) showGrid(c *ui.Context, shows []qqmusic.Show, rows int) {
	cols := a.columns(c, 170)
	if rows > 0 {
		shows = shows[:min(len(shows), rows*cols)]
	}
	ui.Grid(c).Columns(cols).Gap(12).Children(func() {
		for _, sh := range shows {
			a.showCard(c, sh)
		}
	})
}

// showCard is a podcast as a playlist's card: its cover, name, episodes
// and host. Its play button plays its newest episodes.
func (a *app) showCard(c *ui.Context, sh qqmusic.Show) {
	sub := sh.Host
	switch {
	case sh.Episodes > 0 && sh.Host != "":
		sub = fmt.Sprintf("%d 集 · %s", sh.Episodes, sh.Host)
	case sh.Episodes > 0:
		sub = fmt.Sprintf("%d 集", sh.Episodes)
	}
	a.playlistCard(c, qqmusic.Playlist{}, pageOf(sh), sh.Title, sub, sh.Cover, sh.PlayCount)
}

// hostGrid shows the people of a shelf of hosts, each their photo and
// name, as singers: a card opens their show.
func (a *app) hostGrid(c *ui.Context, hosts []qqmusic.Show) {
	cols := a.columns(c, 150)
	ui.Grid(c).Columns(cols).Gap(12).Children(func() {
		for _, sh := range hosts {
			pg := pageOf(sh)
			card := ui.Column(c.Key(pg.key())).Padding(12, 8).Gap(10).Radius(radiusXl).AlignItems(ui.Center).Cursor(ui.CursorPointer).MinWidth(0)
			if card.Hovered() {
				card.Background(colGhost)
			}
			card.Children(func() {
				const side = 112
				ui.Box(c).Size(side, side).Radius(side / 2).Clip().Children(func() {
					if bmp := a.images.get(sh.Cover, 300); bmp != nil {
						ui.Image(c, bmp).Fill().Fit(ui.Cover)
					} else {
						ui.Box(c).Fill().Background(colHover).Center().Children(func() {
							ui.Icon(c, icPodcast).FontSize(side * 0.35).TextColor(colDim)
						})
					}
					ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).Radius(side/2).Border(1, colLine)
				})
				ui.Text(c, sh.Title).FontSize(14).FontWeight(500).SingleLine().TextAlign(ui.Center).FillWidth()
			})
			if card.Clicked() {
				a.navigate(pg)
			}
		}
	})
}

// showPage is the page of a podcast: who hosts it and what it is about
// above its episodes, the newest first, the next ones loading as their end
// comes into view.
func (a *app) showPage(c *ui.Context, p page, l *songList) {
	n := len(l.eps)
	ui.List(c, &l.list, 2+n+1, func(i int) {
		switch {
		case i == 0:
			a.listHeader(c, p, l)
		case i == 1:
			a.episodeTitles(c)
		case i-2 < n:
			a.episodeRow(c, l, i-2)
		default:
			a.episodesEnd(c, p, l)
		}
	}).Grow(1).Padding(0, 0, 24, 0).Children(func() {})
}

// episodeTitles is the head of a podcast's episodes, as columnTitles.
func (a *app) episodeTitles(c *ui.Context) {
	ui.Row(c).Height(40).Margin(0, 24).Padding(0, 16).Gap(16).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(colLine).Children(func() {
		muted := func(s string) ui.Element { return ui.Text(c, s).FontSize(13).FontWeight(500).TextColor(colMuted) }
		muted("#").Width(40).TextAlign(ui.End)
		muted("单集").Grow(1).Basis(0)
		muted("发布日期").Width(88)
		muted("播放").Width(64).TextAlign(ui.End)
		ui.Box(c).Width(32)
		ui.Box(c).Width(64).AlignItems(ui.End).Children(func() { ui.Icon(c, icClock).FontSize(14).TextColor(colMuted) })
	})
}

// episodeRow is episode i of a podcast's page: its number, title, date
// and plays. A double click plays the episodes from it.
func (a *app) episodeRow(c *ui.Context, l *songList, i int) {
	e := l.eps[i]
	s := e.Song
	playing := a.isPlaying(s)
	row := ui.Row(c.Key(s.Mid+fmt.Sprint(i))).Margin(0, 24).Padding(10, 16).Gap(16).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(colLine)
	hover := row.Hovered()
	switch {
	case hover:
		row.Background(colGhost)
	case playing:
		row.Background(ui.RGBA(255, 255, 255, 0.035))
	}
	row.Children(func() {
		ui.Box(c).Width(40).AlignItems(ui.End).Children(func() {
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
						a.playList(l.songs, i)
					}
				}
			case playing:
				a.equalizer(c)
			default:
				n := e.Index
				if n == 0 {
					n = i + 1
				}
				ui.Text(c, fmt.Sprint(n)).FontSize(13).TextColor(colMuted).FontFeatures("tnum")
			}
		})
		ui.Row(c).Grow(1).Basis(0).MinWidth(0).Gap(6).AlignItems(ui.Center).Children(func() {
			t := ui.Text(c, s.DisplayName()).FontSize(14).FontWeight(500).SingleLine().MinWidth(0).Shrink(1).Tooltip(s.DisplayName())
			if playing {
				t.TextColor(colAccent)
			}
			if s.VIP() && !a.user.VIP {
				badge(c, "VIP", colVIP).FontSize(10)
			}
		})
		ui.Text(c, e.UploadTime).Width(88).FontSize(13).TextColor(colMuted).FontFeatures("tnum").SingleLine()
		plays := ""
		if e.ListenCount > 0 {
			plays = formatCount(e.ListenCount)
		}
		ui.Text(c, plays).Width(64).TextAlign(ui.End).FontSize(13).TextColor(colMuted).FontFeatures("tnum").SingleLine()
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
		ui.Text(c, s.Duration()).Width(64).TextAlign(ui.End).FontSize(13).TextColor(colMuted).FontFeatures("tnum")
	})
	if row.DoubleClicked() {
		a.playList(l.songs, i)
	}
	row.ContextMenu(a.songMenu(s))
}

// episodesEnd is the end of a podcast's episodes: the next ones load as it
// comes into view.
func (a *app) episodesEnd(c *ui.Context, p page, l *songList) {
	switch {
	case l.err != "":
		errorRow(c, l.err, func() { a.loadList(p) })
	case l.loading || len(l.eps) < l.total:
		if !l.loading {
			a.loadList(p)
		}
		ui.Row(c).Justify(ui.Center).Padding(16).Children(func() { ui.Spinner(c).Size(18, 18) })
	case len(l.eps) == 0:
		ui.Text(c, "暂无单集").TextColor(colMuted).Padding(24)
	}
}
