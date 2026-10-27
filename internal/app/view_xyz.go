package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"qqmusic/internal/qqmusic"
	"qqmusic/internal/xyz"
)

// xyzPage is 小宇宙 beta: the QR code to log in, or 小宇宙's charts, what the
// podcasts subscribed to put out and those podcasts, or what the title
// bar's box found.
func (a *app) xyzPage(c *ui.Context) {
	x := &a.xyz
	a.pageScroll(c, &a.main, a.pageTint(a.xyzCover()), func() {
		a.xyzHeader(c)
		switch {
		case !x.api.LoggedIn():
			a.xyzLogin(c)
		case x.search.query != "":
			a.xyzResults(c)
		default:
			ui.Row(c).MarginY(6).Children(func() {
				tabs(c, x.tab, false, a.showXyzTab, "推荐", "发现", "订阅更新", "我的订阅")
			})
			switch x.tab {
			case xyzTabFeed:
				a.xyzFeed(c)
			case xyzTabCharts:
				a.xyzCharts(c)
			case xyzTabInbox:
				f := &x.inbox
				a.xyzEpisodes(c, f.items, nil, false)
				xyzEnd(c, f, "订阅的节目还没有更新", a.moreXyzInbox)
			default:
				f := &x.subs
				a.xyzShowGrid(c, f.items, nil)
				xyzEnd(c, f, "还没有订阅节目", a.moreXyzSubs)
			}
		}
	})
}

// xyzCover is the picture 小宇宙 beta takes its color from: the first
// episode it shows.
func (a *app) xyzCover() string {
	x := &a.xyz
	var eps []xyz.Episode
	switch {
	case !x.api.LoggedIn() || x.search.query != "":
	case x.tab == xyzTabFeed:
		for _, s := range x.feed.sections {
			if it := s.Items[0]; it.Episode != nil {
				eps = []xyz.Episode{*it.Episode}
				break
			}
		}
	case x.tab == xyzTabCharts && x.top == xyzPicked:
		if len(x.picks.items) > 0 && len(x.picks.items[0].Picks) > 0 {
			eps = []xyz.Episode{x.picks.items[0].Picks[0].Episode}
		}
	case x.tab == xyzTabCharts:
		eps = x.tops[x.top].list.Episodes
	case x.tab == xyzTabInbox:
		eps = x.inbox.items
	case len(x.subs.items) > 0:
		return x.subs.items[0].Image.URL(300)
	}
	if len(eps) > 0 {
		return episodeSong(eps[0], nil).Pic
	}
	return ""
}

// xyzHeader is the title of 小宇宙 beta, and who is logged in.
func (a *app) xyzHeader(c *ui.Context) {
	x := &a.xyz
	ui.Row(c).Margin(6, 0, 4, 0).Gap(12).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
			a.podcastTitle(c)
			ui.Text(c, "直连小宇宙：推荐、分类、榜单、订阅与搜索").FontSize(13).TextColor(colMuted).SingleLine()
		})
		if !x.api.LoggedIn() {
			return
		}
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			a.avatar(c, x.user.Avatar.Picture.URL(150), x.user.Nickname, 28)
			name := x.user.Nickname
			if name == "" {
				name = "小宇宙用户"
			}
			ui.Text(c, name).FontSize(14).FontWeight(500).SingleLine().MaxWidth(160)
			if x.checking {
				ui.Spinner(c).Size(14, 14)
			}
			if button(c, ghost, icLogout, "退出").Clicked() {
				a.xyzLogout()
				a.toast("已退出小宇宙")
			}
		})
	})
}

// xyzLogin is the QR code to scan with the 小宇宙 app, as the login dialog
// shows QQ's.
func (a *app) xyzLogin(c *ui.Context) {
	x := &a.xyz
	ui.Row(c).Margin(12, 0).Padding(24).Gap(32).Radius(radiusXl).Background(colCard).Border(1, colLine).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Size(212, 212).Radius(radiusXl).Background(ui.Hex("#ffffff")).Border(1, colLine).Center().Padding(8).Children(func() {
			switch {
			case x.qrBmp != nil:
				ui.Image(c, x.qrBmp).Size(196, 196).Fit(ui.Contain)
			case x.qrErr == "":
				ui.Spinner(c).Size(28, 28).TextColor(ui.Hex("#333"))
			}
			if x.qrState == xyz.QRScanned || x.qrState == xyz.QRExpired || x.qrErr != "" && x.qr == nil {
				ui.Column(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).Radius(radiusXl).Background(ui.RGBA(255, 255, 255, 0.94)).Center().Gap(10).Children(func() {
					if x.qrState == xyz.QRScanned {
						ui.Icon(c, icCheck).FontSize(40).TextColor(colAccent)
						ui.Text(c, "扫码成功").FontWeight(600).TextColor(ui.Hex("#0a0a0a"))
						return
					}
					msg := x.qrState.String()
					if x.qr == nil {
						msg = "二维码获取失败"
					}
					ui.Text(c, msg).FontSize(14).FontWeight(500).TextColor(ui.Hex("#0a0a0a"))
					if button(c, primary, icRefresh, "刷新二维码").Clicked() {
						a.newXyzQR()
					}
				})
			}
		})
		ui.Column(c).Grow(1).MinWidth(0).Gap(10).Children(func() {
			ui.Text(c, "登录小宇宙").FontSize(20).FontWeight(600)
			switch x.qrState {
			case xyz.QRScanned:
				ui.Text(c, "已扫码，请在小宇宙 App 上确认登录").FontSize(14).TextColor(colAccent)
			case xyz.QRDone:
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Spinner(c).Size(14, 14)
					ui.Text(c, "登录成功，正在加载…").FontSize(14)
				})
			default:
				ui.Text(c, "打开小宇宙 App，用「扫一扫」扫描左侧二维码并确认登录").FontSize(14).LineHeight(1.5)
			}
			ui.Text(c, "登录后可浏览榜单、订阅更新与我的订阅，并搜索节目和单集；单集直接播放小宇宙的音频，不需要登录 QQ 音乐。").
				FontSize(13).TextColor(colMuted).LineHeight(1.5)
			if x.qrErr != "" {
				ui.Text(c, x.qrErr).FontSize(13).TextColor(colDanger).MaxLines(3)
			}
			// By phone, on 小宇宙's own page: its SMS code needs a captcha.
			ui.Row(c).Margin(4, 0).Gap(10).AlignItems(ui.Center).Children(func() {
				switch {
				case !a.plat.WebViewAvailable():
					ui.Text(c, "手机号登录需要 Microsoft Edge WebView2 运行时").FontSize(13).TextColor(colDanger)
				case x.webBusy:
					ui.Spinner(c).Size(14, 14)
					ui.Text(c, "请在弹出的小宇宙登录窗口中输入手机号和验证码…").FontSize(13)
				default:
					if button(c, outline, icUser, "手机号登录").Clicked() {
						a.xyzPhoneLogin()
					}
					ui.Text(c, "在小宇宙官方登录页用手机号和短信验证码登录").FontSize(13).TextColor(colMuted)
				}
			})
			ui.Text(c, "登录信息仅保存在本机。小宇宙接口为非公开接口，仅供学习研究，可能随时失效。").FontSize(12).TextColor(colDim).LineHeight(1.5)
		})
	})
}

// xyzCharts is 发现: a chart of the day's episodes, of three, or the
// editors' picks.
func (a *app) xyzCharts(c *ui.Context) {
	x := &a.xyz
	a.xyzChartTabs(c)
	if x.top == xyzPicked {
		a.xyzPicks(c)
		return
	}
	t := &x.tops[x.top]
	// How the chart is made, and playing it.
	ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
		ui.Text(c, t.list.Information).FontSize(12).TextColor(colMuted).Grow(1).MinWidth(0).MaxLines(2)
		if eps := t.list.Episodes; len(eps) > 0 {
			playAllButton(c, func() { a.playList(episodeSongs(eps, nil), 0) })
		}
	})
	switch {
	case len(t.list.Episodes) > 0:
		a.xyzEpisodes(c, t.list.Episodes, nil, true)
	case t.loading:
		skeleton(c, func() { songRowsSkeleton(c, 10) }).Gap(0)
	case t.err != "":
		errorRow(c, t.err, func() { a.loadXyzTop(x.top, true) })
	case t.loaded:
		ui.Text(c, "榜单暂无单集").TextColor(colMuted).Padding(24, 0)
	}
}

// xyzChartLooks is how 发现 shows its charts, and 编辑精选 after them: an
// icon, and what each is, for a tooltip.
var xyzChartLooks = [xyzPicked + 1]struct {
	icon *ui.SVG
	desc string
}{{icFlame, "24 小时最受欢迎"}, {icTrending, "热度上升最快"}, {icStar, "新节目的高光单集"}, {icSparkles, "编辑每天挑选"}}

// xyzChartTabs are the charts of 发现 to choose from: pills smaller than
// the tabs above them, each an icon and a name, the one shown in the
// primary color; what each is shows as its tooltip.
func (a *app) xyzChartTabs(c *ui.Context) {
	x := &a.xyz
	ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
		for i, look := range xyzChartLooks {
			name := "编辑精选"
			if i < xyzPicked {
				name = xyzTops[i].name
			}
			on := i == x.top
			pill := ui.Row(c.Key(i)).Height(30).PaddingX(12).Gap(6).Radius(15).AlignItems(ui.Center).Cursor(ui.CursorPointer).Tooltip(look.desc)
			fg := colMuted
			switch {
			case on:
				pill.Background(colAccent.Alpha(0.14))
				fg = colAccent
			case pill.Hovered():
				pill.Background(colHover)
				fg = colText
			}
			pill.Children(func() {
				ui.Icon(c, look.icon).FontSize(14).TextColor(fg)
				t := ui.Text(c, name).FontSize(13).TextColor(fg).NoWrap()
				if on {
					t.FontWeight(600)
				}
			})
			if pill.Clicked() {
				a.showXyzTop(i)
			}
		}
	})
}

// xyzPicks is 编辑精选: the episodes picked each day, each with what a
// listener said of it, the next days loading as their end shows.
func (a *app) xyzPicks(c *ui.Context) {
	f := &a.xyz.picks
	for _, day := range f.items {
		items := make([]xyz.FeedItem, len(day.Picks))
		for i, p := range day.Picks {
			items[i] = xyz.FeedItem{Episode: &p.Episode, Note: p.Comment.Text, By: p.Comment.Author.Nickname}
		}
		date := day.Date
		if t, err := time.Parse("2006-01-02", day.Date); err == nil {
			date = t.Format("1月2日")
		}
		ui.Column(c.Key(day.Date)).Gap(10).Children(func() {
			ui.Text(c, date).FontSize(15).FontWeight(600).MarginY(4)
			a.xyzEpisodeCards(c, items)
		})
	}
	xyzEnd(c, f, "暂无编辑精选", a.moreXyzPicks)
}

// xyzFeed is 推荐: the categories, then the sections of the app's front
// page, in its order: podcasts as cards, episodes as cards with a line of
// why. They are the page's own children, held to its width.
func (a *app) xyzFeed(c *ui.Context) {
	f := &a.xyz.feed
	switch {
	case f.loading && len(f.sections) == 0:
		skeleton(c, func() {
			headingSkeleton(c)
			playlistSkeleton(c, a.columns(c, 170), 2)
		})
		return
	case f.err != "" && len(f.sections) == 0:
		errorRow(c, f.err, func() { a.loadXyzFeed(true) })
		return
	case f.loaded && len(f.sections) == 0:
		ui.Text(c, "暂无推荐").TextColor(colMuted).Padding(24, 0)
		return
	}
	if len(f.cats) > 0 {
		heading(c, "分类探索")
		ui.Row(c.Key("cats")).Wrap().Gap(8).Children(func() {
			for _, cat := range f.cats {
				if toggle(c.Key(cat.ID), cat.Name, false).Clicked() {
					a.navigate(page{kind: pageXyzCategory, mid: cat.ID, title: cat.Name})
				}
			}
		})
	}
	for i, s := range f.sections {
		var eps []xyz.FeedItem
		var pods []xyz.Podcast
		var notes []string
		for _, it := range s.Items {
			if it.Podcast != nil {
				pods, notes = append(pods, *it.Podcast), append(notes, it.Note)
			} else {
				eps = append(eps, it)
			}
		}
		ui.Column(c.Key(i)).Margin(18, 0, 0, 0).Gap(10).Children(func() {
			ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
				heading(c, s.Title)
				if len(eps) > 0 {
					playAllButton(c, func() { a.playList(feedSongs(eps), 0) })
				}
			})
			a.xyzShowGrid(c, pods, notes)
			a.xyzEpisodeCards(c, eps)
		})
	}
}

// feedSongs is the episodes of items as songs of the queue.
func feedSongs(items []xyz.FeedItem) []qqmusic.Song {
	songs := make([]qqmusic.Song, len(items))
	for i, it := range items {
		songs[i] = episodeSong(*it.Episode, nil)
	}
	return songs
}

// xyzEpisodeCards shows episodes as the app does, cards of three a row at
// most: the cover, the title, the podcast and length, and a line of why,
// a quote when a listener said it. A card plays them from it.
func (a *app) xyzEpisodeCards(c *ui.Context, items []xyz.FeedItem) {
	if len(items) == 0 {
		return
	}
	ui.Grid(c).Columns(min(a.columns(c, 340), 3)).Gap(12).Children(func() {
		for i, it := range items {
			e := it.Episode
			s := episodeSong(*e, nil)
			playing := a.isPlaying(s)
			card := ui.Row(c.Key(e.Eid+fmt.Sprint(i))).Padding(12).Gap(14).Radius(radiusXl).Background(colCard).Border(0.5, colFaint).Cursor(ui.CursorPointer).AlignItems(ui.Start).MinWidth(0)
			hover := card.Hovered()
			if hover {
				card.Background(colCardHover)
			}
			card.Children(func() {
				ui.Box(c).Size(80, 80).Radius(radiusLg).Clip().Children(func() {
					a.cover(c, s.Pic, 80, radiusLg)
					if hover || playing {
						a.playCircle(c, 30, playing && a.player.Playing()).Absolute().Right(6).Bottom(6)
					}
				})
				ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(4).Children(func() {
					t := ui.Text(c, e.Title).FontSize(14).FontWeight(600).MaxLines(2).LineHeight(1.35).Tooltip(e.Title)
					if playing {
						t.TextColor(colAccent)
					}
					ui.Row(c).Gap(6).AlignItems(ui.Center).MinWidth(0).Children(func() {
						ui.Text(c, s.Album.Name).FontSize(12).TextColor(colMuted).SingleLine().MinWidth(0).Shrink(1)
						ui.Text(c, "· "+s.Duration()).FontSize(12).TextColor(colDim).FontFeatures("tnum").NoWrap()
						if e.PayType != "" && e.PayType != "FREE" {
							badge(c, "付费", colVIP).FontSize(10)
						}
					})
					if note := strings.Join(strings.Fields(it.Note), " "); note != "" {
						if it.By != "" {
							note = "“" + note + "” —— " + it.By
						}
						ui.Text(c, note).FontSize(12).TextColor(colSubtle).MaxLines(2).LineHeight(1.45).Tooltip(it.Note)
					}
				})
			})
			if card.Clicked() {
				if playing {
					a.togglePlay()
				} else {
					a.playList(feedSongs(items), i)
				}
			}
			card.ContextMenu(a.songMenu(s))
		}
	})
}

// xyzCategoryPage is the page of a category: its tabs, 全部, 近期热门, 为你
// 推荐, each its podcasts, the next loading as their end shows.
func (a *app) xyzCategoryPage(c *ui.Context, p page) {
	cat := a.xyz.cats[p.mid]
	var l *xyzList[xyz.FeedItem]
	cover := ""
	if cat != nil && len(cat.lists) > 0 {
		l = &cat.lists[cat.tab]
		if len(l.items) > 0 {
			cover = l.items[0].Podcast.Image.URL(300)
		}
	}
	a.pageScroll(c, &a.main, a.pageTint(cover), func() {
		ui.Row(c).Margin(6, 0, 4, 0).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Text(c, p.title).FontSize(26).FontWeight(600).LetterSpacing(-0.6)
			badge(c, "分类", colAccent)
		})
		switch {
		case cat == nil || cat.loading:
			skeleton(c, func() { playlistSkeleton(c, a.columns(c, 170), 2) })
			return
		case cat.err != "":
			errorRow(c, cat.err, func() { a.loadXyzCategory(p.mid, true) })
			return
		}
		names := make([]string, len(cat.tabs))
		for i, t := range cat.tabs {
			names[i] = t.Name
		}
		ui.Row(c).MarginY(6).Children(func() {
			tabs(c, cat.tab, false, func(i int) { a.showXyzCategoryTab(p.mid, i) }, names...)
		})
		pods := make([]xyz.Podcast, len(l.items))
		notes := make([]string, len(l.items))
		for i, it := range l.items {
			pods[i] = *it.Podcast
			if it.Episode != nil {
				notes[i] = "最新：" + it.Episode.Title
			}
		}
		a.xyzShowGrid(c, pods, notes)
		xyzEnd(c, l, "这个分类暂无节目", func() { a.moreXyzCategory(p.mid) })
	})
}

// xyzResults is what the search of 小宇宙 found: podcasts or episodes, a
// tab each, a page at a time.
func (a *app) xyzResults(c *ui.Context) {
	s := &a.xyz.search
	ui.Row(c).MarginY(6).Gap(12).AlignItems(ui.Center).Children(func() {
		tabs(c, s.tab, false, a.showXyzSearchTab, "节目", "单集")
		ui.Text(c, "“"+s.query+"” 的搜索结果").FontSize(13).TextColor(colMuted).SingleLine()
		ui.Spacer(c)
		sectionAction(c, icClose, "清除搜索", false, func() {
			a.search.input = ""
			a.searchXyz("")
		})
	})
	if s.tab == 0 {
		a.xyzShowGrid(c, s.shows.items, nil)
		xyzEnd(c, &s.shows, "没有找到相关节目", a.moreXyzShows)
		return
	}
	if eps := s.eps.items; len(eps) > 0 {
		ui.Row(c).Children(func() {
			playAllButton(c, func() { a.playList(episodeSongs(eps, nil), 0) })
		})
	}
	a.xyzEpisodes(c, s.eps.items, nil, false)
	xyzEnd(c, &s.eps, "没有找到相关单集", a.moreXyzEpisodes)
}

// xyzEnd is the end of a list of 小宇宙 beta: its bones as it loads, what
// went wrong, or empty, the first page loaded. With more to load, the next
// page loads as the end comes into view, as a feed's.
func xyzEnd[T any](c *ui.Context, f *xyzList[T], empty string, more func()) {
	switch {
	case f.loading:
		skeleton(c, func() { songRowsSkeleton(c, 3) }).Gap(0)
	case f.err != "":
		errorRow(c, f.err, more)
	case f.next != nil:
		xyzMoreRow(c, more)
	case f.loaded && len(f.items) == 0:
		ui.Text(c, empty).TextColor(colMuted).Padding(24, 0)
	}
}

// xyzMoreRow is a spinner at the end of a list, which loads more once it
// showed: InView is of the frame before, and more sets loading at once.
func xyzMoreRow(c *ui.Context, more func()) {
	end := ui.Row(c.Key("more")).Padding(16).Justify(ui.Center).Children(func() { ui.Spinner(c).Size(18, 18) })
	if end.InView() {
		more()
	}
}

// xyzEpisodes shows episodes as rows; pod is their podcast when they do
// not say. ranked numbers them as a chart.
func (a *app) xyzEpisodes(c *ui.Context, eps []xyz.Episode, pod *xyz.Podcast, ranked bool) {
	if len(eps) == 0 {
		return
	}
	ui.Column(c).Margin(0, -16).Children(func() {
		for i := range eps {
			a.xyzEpisodeRow(c, eps, i, pod, ranked)
		}
	})
}

// xyzEpisodeRow is episode i of eps: its cover, title, podcast and date,
// plays and length. A double click plays eps from it.
func (a *app) xyzEpisodeRow(c *ui.Context, eps []xyz.Episode, i int, pod *xyz.Podcast, ranked bool) {
	e := eps[i]
	s := episodeSong(e, pod)
	playing := a.isPlaying(s)
	play := func() { a.playList(episodeSongs(eps, pod), i) }
	row := ui.Row(c.Key(e.Eid+fmt.Sprint(i))).Padding(8, 16).Gap(14).Radius(radiusMd).AlignItems(ui.Center)
	hover := row.Hovered()
	switch {
	case hover:
		row.Background(colGhost)
	case playing:
		row.Background(ui.RGBA(255, 255, 255, 0.035))
	}
	row.Children(func() {
		ui.Box(c).Width(28).AlignItems(ui.End).Children(func() {
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
						play()
					}
				}
			case playing:
				a.equalizer(c)
			default:
				t := ui.Text(c, fmt.Sprint(i+1)).FontSize(13).TextColor(colMuted).FontFeatures("tnum")
				if ranked && i < 3 {
					t.TextColor(colAccent).FontWeight(600)
				}
			}
		})
		a.cover(c, s.Pic, 44, radiusSm)
		ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(3).Children(func() {
			ui.Row(c).Gap(6).AlignItems(ui.Center).MinWidth(0).Children(func() {
				t := ui.Text(c, e.Title).FontSize(14).FontWeight(500).SingleLine().MinWidth(0).Shrink(1).Tooltip(e.Title)
				if playing {
					t.TextColor(colAccent)
				}
				if e.PayType != "" && e.PayType != "FREE" {
					badge(c, "付费", colVIP).FontSize(10)
				}
			})
			ui.Row(c).Gap(6).AlignItems(ui.Center).MinWidth(0).Children(func() {
				if s.Album.Name != "" {
					a.albumLink(c, s, 12).Shrink(1)
				}
				if !e.PubDate.IsZero() {
					ui.Text(c, e.PubDate.Local().Format("2006-01-02")).FontSize(12).TextColor(colDim).FontFeatures("tnum").NoWrap()
				}
			})
		})
		plays := ""
		if e.PlayCount > 0 {
			plays = formatCount(e.PlayCount)
		}
		ui.Row(c).Width(72).Gap(4).Justify(ui.End).AlignItems(ui.Center).Children(func() {
			if plays != "" {
				ui.Icon(c, icHeadset).FontSize(12).TextColor(colMuted)
				ui.Text(c, plays).FontSize(13).TextColor(colMuted).FontFeatures("tnum").NoWrap()
			}
		})
		ui.Text(c, s.Duration()).Width(64).TextAlign(ui.End).FontSize(13).TextColor(colMuted).FontFeatures("tnum")
	})
	if row.DoubleClicked() {
		play()
	}
	row.ContextMenu(a.songMenu(s))
}

// xyzShowGrid shows podcasts as cards, which open their pages; notes, if
// any, is a line of each: why it is recommended.
func (a *app) xyzShowGrid(c *ui.Context, pods []xyz.Podcast, notes []string) {
	if len(pods) == 0 {
		return
	}
	ui.Grid(c).Columns(a.columns(c, 170)).Gap(12).Children(func() {
		for i, p := range pods {
			pg := page{kind: pageXyzShow, mid: p.Pid, title: p.Title}
			card := ui.Column(c.Key(p.Pid)).Padding(8).Gap(8).Radius(radiusXl).Cursor(ui.CursorPointer).MinWidth(0)
			if card.Hovered() {
				card.Background(colGhost)
			}
			card.Children(func() {
				ui.Box(c).AspectRatio(1).FillWidth().Radius(radiusLg).Clip().Children(func() {
					if bmp := a.images.get(p.Image.URL(300), 300); bmp != nil {
						ui.Image(c, bmp).Fill().Fit(ui.Cover)
					} else {
						ui.Box(c).Fill().Background(colHover).Center().Children(func() {
							ui.Icon(c, icPodcast).FontSize(36).TextColor(colDim)
						})
					}
					ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).Radius(radiusLg).Border(1, colLine)
				})
				ui.Column(c).Gap(2).Children(func() {
					ui.Text(c, p.Title).FontSize(14).FontWeight(500).MaxLines(2).LineHeight(1.35)
					sub := p.Author
					if p.EpisodeCount > 0 {
						sub = strings.TrimPrefix(sub+fmt.Sprintf(" · %d 集", p.EpisodeCount), " · ")
					}
					ui.Text(c, sub).FontSize(12).TextColor(colMuted).SingleLine()
					if i < len(notes) && notes[i] != "" {
						ui.Text(c, notes[i]).FontSize(12).TextColor(colSubtle).MaxLines(2).LineHeight(1.4).Tooltip(notes[i])
					}
				})
			})
			if card.Clicked() {
				a.navigate(pg)
			}
		}
	})
}

// xyzShowPage is the page of a podcast of 小宇宙: who hosts it and what it
// is about above its episodes, the newest first, the next ones loading as
// their end comes into view.
func (a *app) xyzShowPage(c *ui.Context, p page) {
	sh := a.xyz.shows[p.mid]
	if sh == nil || !sh.loaded {
		a.tint.aim(c, tintNone)
		switch {
		case sh != nil && sh.err != "":
			errorRow(c, sh.err, func() { a.loadXyzShow(p.mid, true) })
		default:
			a.listSkeleton(c, page{kind: pageAlbum})
		}
		return
	}
	info := &sh.info
	n := len(sh.eps)
	ui.List(c, &sh.list, 1+n+1, func(i int) {
		switch {
		case i == 0:
			a.xyzShowHeader(c, p, sh)
		case i-1 < n:
			ui.Column(c.Key(i)).Padding(0, 8).Children(func() {
				a.xyzEpisodeRow(c, sh.eps, i-1, info, false)
			})
		default:
			ui.Column(c).Padding(8, 24).Children(func() {
				switch {
				case sh.err != "":
					errorRow(c, sh.err, func() { a.moreXyzShowEps(p.mid) })
				case sh.loading || sh.next != nil:
					xyzMoreRow(c, func() { a.moreXyzShowEps(p.mid) })
				case sh.web:
					ui.Text(c, fmt.Sprintf("网页版只提供最新 %d 集，登录小宇宙后可查看全部单集", n)).FontSize(13).TextColor(colMuted).Padding(16, 0)
				case n == 0:
					ui.Text(c, "暂无单集").TextColor(colMuted).Padding(24, 0)
				}
			})
		}
	}).Grow(1).Padding(0, 0, 24, 0).Children(func() {})
}

func (a *app) xyzShowHeader(c *ui.Context, p page, sh *xyzShow) {
	info := &sh.info
	a.tint.aim(c, a.pageTint(info.Image.URL(300)))
	ui.Column(c).Gap(16).Padding(12, 24, 16, 24).Children(func() {
		ui.Row(c).Gap(20).AlignItems(ui.End).Children(func() {
			a.cover(c, info.Image.URL(300), 168, radiusXl).Shadow(0, 8, 24, 0, colShadow)
			ui.Column(c).Grow(1).MinWidth(0).Gap(8).Children(func() {
				ui.Row(c).Gap(6).Children(func() {
					ui.Text(c, "节目").FontSize(12).FontWeight(500).Padding(2, 8).Radius(radiusSm).Background(colHover).NoWrap()
					badge(c, "小宇宙", colAccent)
				})
				title := info.Title
				if title == "" {
					title = p.title
				}
				ui.Text(c, title).FontSize(32).FontWeight(700).LetterSpacing(-0.8).MaxLines(2).LineHeight(1.15)
				desc := info.Brief
				if desc == "" {
					desc = info.Description
				}
				if desc != "" {
					ui.Text(c, strings.ReplaceAll(desc, "\n", " ")).FontSize(13).TextColor(colMuted).MaxLines(2).Tooltip(info.Description)
				}
				var meta []string
				if info.Author != "" {
					meta = append(meta, info.Author)
				}
				if info.EpisodeCount > 0 {
					meta = append(meta, fmt.Sprintf("%d 集", info.EpisodeCount))
				}
				if info.SubscriptionCount > 0 {
					meta = append(meta, formatCount(info.SubscriptionCount)+" 人订阅")
				}
				ui.Text(c, strings.Join(meta, " · ")).FontSize(13).TextColor(colMuted)
			})
		})
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			if button(c, primary, icPlay, "播放全部").Clicked() && len(sh.eps) > 0 {
				a.playList(episodeSongs(sh.eps, info), 0)
			}
			if sh.loading {
				ui.Spinner(c).Size(16, 16)
			}
		})
	})
}
