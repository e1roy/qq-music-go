package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"qqmusic/internal/qqmusic"
	"qqmusic/internal/xiaoyuzhou"
)

// xiaoyuzhouPage is 小宇宙 官方: the QR code to log in, or its four tabs, or
// what the title bar's box found.
func (a *app) xiaoyuzhouPage(c *ui.Context) {
	x := &a.xiaoyuzhou
	a.pageScroll(c, &a.main, a.pageTint(a.xiaoyuzhouCover()), func() {
		a.xiaoyuzhouHeader(c)
		switch {
		case !x.api.LoggedIn():
			a.xiaoyuzhouLogin(c)
		case x.search.query != "":
			a.xiaoyuzhouResults(c)
		default:
			ui.Row(c).MarginY(6).Children(func() {
				tabs(c, x.tab, false, a.showXiaoyuzhouTab, "推荐", "发现", "订阅更新", "我的订阅")
			})
			switch x.tab {
			case xiaoyuzhouTabFeed:
				a.xiaoyuzhouFeedTab(c)
			case xiaoyuzhouTabDiscover:
				a.xiaoyuzhouDiscoverTab(c)
			case xiaoyuzhouTabInbox:
				a.xiaoyuzhouEpisodes(c, x.inbox.items, nil, false)
				pagerEnd(c, &x.inbox, "订阅的节目还没有更新", a.moreXiaoyuzhouInbox)
			default:
				a.xiaoyuzhouShowGrid(c, x.subs.items, nil)
				pagerEnd(c, &x.subs, "还没有订阅节目", a.moreXiaoyuzhouSubs)
			}
		}
	})
}

// xiaoyuzhouCover is the picture the top of 小宇宙 官方 takes its color
// from: that of the first episode, or podcast, the tab shows.
func (a *app) xiaoyuzhouCover() string {
	x := &a.xiaoyuzhou
	var first *xiaoyuzhou.Episode
	switch {
	case !x.api.LoggedIn() || x.search.query != "":
	case x.tab == xiaoyuzhouTabFeed:
		for _, s := range x.feed.sections {
			if first = s.Entries[0].Episode; first != nil {
				break
			}
		}
	case x.tab == xiaoyuzhouTabDiscover && x.chart == xiaoyuzhouPicked:
		if days := x.picks.items; len(days) > 0 && len(days[0].Picks) > 0 {
			first = &days[0].Picks[0].Episode
		}
	case x.tab == xiaoyuzhouTabDiscover:
		if eps := x.charts[x.chart].chart.Episodes; len(eps) > 0 {
			first = &eps[0]
		}
	case x.tab == xiaoyuzhouTabInbox:
		if len(x.inbox.items) > 0 {
			first = &x.inbox.items[0]
		}
	case len(x.subs.items) > 0:
		return x.subs.items[0].Image.URL(300)
	}
	if first == nil {
		return ""
	}
	return episodeSong(*first, nil).Pic
}

// xiaoyuzhouHeader is the title of 小宇宙 官方, and who is logged in.
func (a *app) xiaoyuzhouHeader(c *ui.Context) {
	x := &a.xiaoyuzhou
	ui.Row(c).Margin(6, 0, 4, 0).Gap(12).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
			a.podcastTitle(c)
			ui.Text(c, "直连小宇宙：推荐、分类、榜单、订阅与搜索").FontSize(13).TextColor(colMuted).SingleLine()
		})
		if !x.api.LoggedIn() {
			return
		}
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			name := x.user.Nickname
			if name == "" {
				name = "小宇宙用户"
			}
			a.avatar(c, x.user.Avatar.Picture.URL(150), x.user.Nickname, 28)
			ui.Text(c, name).FontSize(14).FontWeight(500).SingleLine().MaxWidth(160)
			if x.checking {
				ui.Spinner(c).Size(14, 14)
			}
			if button(c, ghost, icLogout, "退出").Clicked() {
				a.logoutXiaoyuzhou()
				a.toast("已退出小宇宙")
			}
		})
	})
}

// xiaoyuzhouLogin is the QR code to scan with the 小宇宙 app, laid out as
// the login dialog of QQ, and the way to log in by phone.
func (a *app) xiaoyuzhouLogin(c *ui.Context) {
	x := &a.xiaoyuzhou
	ink := ui.Hex("#0a0a0a")
	ui.Row(c).Margin(12, 0).Padding(24).Gap(32).Radius(radiusXl).Background(colCard).Border(1, colLine).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Size(212, 212).Radius(radiusXl).Background(ui.Hex("#ffffff")).Border(1, colLine).Center().Padding(8).Children(func() {
			switch {
			case x.qrBmp != nil:
				ui.Image(c, x.qrBmp).Size(196, 196).Fit(ui.Contain)
			case x.qrErr == "":
				ui.Spinner(c).Size(28, 28).TextColor(ui.Hex("#333"))
			}
			failed := x.qr == nil && x.qrErr != ""
			if x.qrState != xiaoyuzhou.QRScanned && x.qrState != xiaoyuzhou.QRExpired && !failed {
				return
			}
			// Over the code: scanned, or a button for a new one.
			ui.Column(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).Radius(radiusXl).Background(ui.RGBA(255, 255, 255, 0.94)).Center().Gap(10).Children(func() {
				if x.qrState == xiaoyuzhou.QRScanned {
					ui.Icon(c, icCheck).FontSize(40).TextColor(colAccent)
					ui.Text(c, "扫码成功").FontWeight(600).TextColor(ink)
					return
				}
				msg := x.qrState.String()
				if failed {
					msg = "二维码获取失败"
				}
				ui.Text(c, msg).FontSize(14).FontWeight(500).TextColor(ink)
				if button(c, primary, icRefresh, "刷新二维码").Clicked() {
					a.newXiaoyuzhouQR()
				}
			})
		})
		ui.Column(c).Grow(1).MinWidth(0).Gap(10).Children(func() {
			ui.Text(c, "登录小宇宙").FontSize(20).FontWeight(600)
			switch x.qrState {
			case xiaoyuzhou.QRScanned:
				ui.Text(c, "已扫码，请在小宇宙 App 上确认登录").FontSize(14).TextColor(colAccent)
			case xiaoyuzhou.QRDone:
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
			ui.Row(c).Margin(4, 0).Gap(10).AlignItems(ui.Center).Children(func() {
				switch {
				case !a.plat.WebViewAvailable():
					ui.Text(c, "手机号登录需要 Microsoft Edge WebView2 运行时").FontSize(13).TextColor(colDanger)
				case x.phoneOpen:
					ui.Spinner(c).Size(14, 14)
					ui.Text(c, "请在弹出的小宇宙登录窗口中输入手机号和验证码…").FontSize(13)
				default:
					if button(c, outline, icUser, "手机号登录").Clicked() {
						a.xiaoyuzhouPhoneLogin()
					}
					ui.Text(c, "在小宇宙官方登录页用手机号和短信验证码登录").FontSize(13).TextColor(colMuted)
				}
			})
			ui.Text(c, "登录信息仅保存在本机。小宇宙接口为非公开接口，仅供学习研究，可能随时失效。").FontSize(12).TextColor(colDim).LineHeight(1.5)
		})
	})
}

// xiaoyuzhouFeedTab is 推荐: the categories, then the sections of the
// app's front page in its order, podcasts as cards and episodes as cards
// with a line of why.
func (a *app) xiaoyuzhouFeedTab(c *ui.Context) {
	f := &a.xiaoyuzhou.feed
	switch {
	case f.loading && len(f.sections) == 0:
		skeleton(c, func() {
			headingSkeleton(c)
			playlistSkeleton(c, a.columns(c, 170), 2)
		})
		return
	case f.err != "" && len(f.sections) == 0:
		errorRow(c, f.err, a.refreshXiaoyuzhou)
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
					a.navigate(page{kind: pageXiaoyuzhouCategory, mid: cat.ID, title: cat.Name})
				}
			}
		})
	}
	for i, s := range f.sections {
		var pods []xiaoyuzhou.Podcast
		var notes []string
		var eps []xiaoyuzhou.Entry
		for _, e := range s.Entries {
			if e.Podcast != nil {
				pods, notes = append(pods, *e.Podcast), append(notes, e.Note)
			} else {
				eps = append(eps, e)
			}
		}
		ui.Column(c.Key(i)).Margin(18, 0, 0, 0).Gap(10).Children(func() {
			ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
				heading(c, s.Title)
				if len(eps) > 0 {
					playAllButton(c, func() { a.playList(entrySongs(eps), 0) })
				}
			})
			a.xiaoyuzhouShowGrid(c, pods, notes)
			a.xiaoyuzhouEpisodeCards(c, eps)
		})
	}
}

// entrySongs is the episodes of entries as songs of the queue.
func entrySongs(entries []xiaoyuzhou.Entry) []qqmusic.Song {
	songs := make([]qqmusic.Song, len(entries))
	for i, e := range entries {
		songs[i] = episodeSong(*e.Episode, nil)
	}
	return songs
}

// chartPills are the pills of 发现, the charts and 编辑精选 after them: a
// name, an icon, and what it is for a tooltip.
var chartPills = [xiaoyuzhouPicked + 1]struct {
	name string
	icon *ui.SVG
	desc string
}{
	{"最热榜", icFlame, "24 小时最受欢迎"},
	{"锋芒榜", icTrending, "热度上升最快"},
	{"新星榜", icStar, "新节目的高光单集"},
	{"编辑精选", icSparkles, "编辑每天挑选"},
}

// xiaoyuzhouDiscoverTab is 发现: pills smaller than the tabs above them, one
// shown in the primary color, and below them a chart of the day's
// episodes, or the editors' picks.
func (a *app) xiaoyuzhouDiscoverTab(c *ui.Context) {
	x := &a.xiaoyuzhou
	ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
		for i, p := range chartPills {
			on := i == x.chart
			pill := ui.Row(c.Key(i)).Height(30).PaddingX(12).Gap(6).Radius(15).AlignItems(ui.Center).Cursor(ui.CursorPointer).Tooltip(p.desc)
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
				ui.Icon(c, p.icon).FontSize(14).TextColor(fg)
				t := ui.Text(c, p.name).FontSize(13).TextColor(fg).NoWrap()
				if on {
					t.FontWeight(600)
				}
			})
			if pill.Clicked() {
				a.showXiaoyuzhouChart(i)
			}
		}
	})
	if x.chart == xiaoyuzhouPicked {
		a.xiaoyuzhouPicks(c)
		return
	}
	ch := &x.charts[x.chart]
	eps := ch.chart.Episodes
	ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
		ui.Text(c, ch.chart.About).FontSize(12).TextColor(colMuted).Grow(1).MinWidth(0).MaxLines(2)
		if len(eps) > 0 {
			playAllButton(c, func() { a.playList(episodeSongs(eps, nil), 0) })
		}
	})
	switch {
	case len(eps) > 0:
		a.xiaoyuzhouEpisodes(c, eps, nil, true)
	case ch.loading:
		skeleton(c, func() { songRowsSkeleton(c, 10) }).Gap(0)
	case ch.err != "":
		errorRow(c, ch.err, a.refreshXiaoyuzhou)
	case ch.loaded:
		ui.Text(c, "榜单暂无单集").TextColor(colMuted).Padding(24, 0)
	}
}

// xiaoyuzhouPicks is 编辑精选: the episodes picked each day, each with what
// a listener said of it; the days before load as their end shows.
func (a *app) xiaoyuzhouPicks(c *ui.Context) {
	p := &a.xiaoyuzhou.picks
	for _, day := range p.items {
		entries := make([]xiaoyuzhou.Entry, len(day.Picks))
		for i, pk := range day.Picks {
			entries[i] = xiaoyuzhou.Entry{Episode: &pk.Episode, Note: pk.Comment.Text, By: pk.Comment.Author.Nickname}
		}
		date := day.Date
		if t, err := time.Parse("2006-01-02", day.Date); err == nil {
			date = t.Format("1月2日")
		}
		ui.Column(c.Key(day.Date)).Gap(10).Children(func() {
			ui.Text(c, date).FontSize(15).FontWeight(600).MarginY(4)
			a.xiaoyuzhouEpisodeCards(c, entries)
		})
	}
	pagerEnd(c, p, "暂无编辑精选", a.moreXiaoyuzhouPicks)
}

// xiaoyuzhouEpisodeCards shows episodes as the app does, cards of three a
// row at most: the cover, the title, the podcast and length, and a line
// of why, quoted when a listener said it. A card plays them from it.
func (a *app) xiaoyuzhouEpisodeCards(c *ui.Context, entries []xiaoyuzhou.Entry) {
	if len(entries) == 0 {
		return
	}
	ui.Grid(c).Columns(min(a.columns(c, 340), 3)).Gap(12).Children(func() {
		for i, en := range entries {
			e := en.Episode
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
						if e.Paid() {
							badge(c, "付费", colVIP).FontSize(10)
						}
					})
					if note := strings.Join(strings.Fields(en.Note), " "); note != "" {
						if en.By != "" {
							note = "“" + note + "” —— " + en.By
						}
						ui.Text(c, note).FontSize(12).TextColor(colSubtle).MaxLines(2).LineHeight(1.45).Tooltip(en.Note)
					}
				})
			})
			if card.Clicked() {
				if playing {
					a.togglePlay()
				} else {
					a.playList(entrySongs(entries), i)
				}
			}
			card.ContextMenu(a.songMenu(s))
		}
	})
}

// xiaoyuzhouCategoryPage is the page of a category: a tab of each of its
// lists, 全部, 近期热门, 为你推荐, its podcasts with their newest episode.
func (a *app) xiaoyuzhouCategoryPage(c *ui.Context, p page) {
	cat := a.xiaoyuzhou.cats[p.mid]
	var list *pager[xiaoyuzhou.Entry]
	cover := ""
	if cat != nil && len(cat.lists) > 0 {
		list = &cat.lists[cat.tab]
		if len(list.items) > 0 {
			cover = list.items[0].Podcast.Image.URL(300)
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
			errorRow(c, cat.err, func() { a.loadXiaoyuzhouCategory(p.mid, true) })
			return
		}
		names := make([]string, len(cat.tabs))
		for i, t := range cat.tabs {
			names[i] = t.Name
		}
		ui.Row(c).MarginY(6).Children(func() {
			tabs(c, cat.tab, false, func(i int) { a.showXiaoyuzhouCategoryTab(p.mid, i) }, names...)
		})
		pods := make([]xiaoyuzhou.Podcast, len(list.items))
		notes := make([]string, len(list.items))
		for i, e := range list.items {
			pods[i] = *e.Podcast
			if e.Episode != nil {
				notes[i] = "最新：" + e.Episode.Title
			}
		}
		a.xiaoyuzhouShowGrid(c, pods, notes)
		pagerEnd(c, list, "这个分类暂无节目", func() { a.moreXiaoyuzhouCategory(p.mid) })
	})
}

// xiaoyuzhouResults is what the search of 小宇宙 found: podcasts or
// episodes, a tab each.
func (a *app) xiaoyuzhouResults(c *ui.Context) {
	s := &a.xiaoyuzhou.search
	ui.Row(c).MarginY(6).Gap(12).AlignItems(ui.Center).Children(func() {
		tabs(c, s.tab, false, a.showXiaoyuzhouSearchTab, "节目", "单集")
		ui.Text(c, "“"+s.query+"” 的搜索结果").FontSize(13).TextColor(colMuted).SingleLine()
		ui.Spacer(c)
		sectionAction(c, icClose, "清除搜索", false, func() {
			a.search.input = ""
			a.searchXiaoyuzhou("")
		})
	})
	if s.tab == 0 {
		a.xiaoyuzhouShowGrid(c, s.shows.items, nil)
		pagerEnd(c, &s.shows, "没有找到相关节目", func() { a.moreXiaoyuzhouShows(&s.shows, s.query) })
		return
	}
	if eps := s.eps.items; len(eps) > 0 {
		ui.Row(c).Children(func() {
			playAllButton(c, func() { a.playList(episodeSongs(eps, nil), 0) })
		})
	}
	a.xiaoyuzhouEpisodes(c, s.eps.items, nil, false)
	pagerEnd(c, &s.eps, "没有找到相关单集", a.moreXiaoyuzhouEpisodes)
}

// pagerEnd is the end of a list loaded a page at a time: its bones as it
// loads, what went wrong, or that it is empty. With more to load, the next
// page loads as the end comes into view, as a feed's.
func pagerEnd[T any](c *ui.Context, p *pager[T], empty string, more func()) {
	switch {
	case p.loading:
		skeleton(c, func() { songRowsSkeleton(c, 3) }).Gap(0)
	case p.err != "":
		errorRow(c, p.err, more)
	case p.more:
		moreRow(c, more)
	case p.pages > 0 && len(p.items) == 0:
		ui.Text(c, empty).TextColor(colMuted).Padding(24, 0)
	}
}

// moreRow is a spinner at the end of a list, which loads more once it is
// shown: InView is of the frame before, and more sets loading at once, so
// a frame built twice asks once.
func moreRow(c *ui.Context, more func()) {
	end := ui.Row(c.Key("more")).Padding(16).Justify(ui.Center).Children(func() { ui.Spinner(c).Size(18, 18) })
	if end.InView() {
		more()
	}
}

// xiaoyuzhouEpisodes shows episodes as a table inside a scrolled page; pod
// is their podcast when they do not say. ranked numbers them as a chart.
// The page's padding is undone, as the rows are inset like those of
// songTable, which pages without a scroll draw from the panel's edge.
func (a *app) xiaoyuzhouEpisodes(c *ui.Context, eps []xiaoyuzhou.Episode, pod *xiaoyuzhou.Podcast, ranked bool) {
	if len(eps) == 0 {
		return
	}
	ui.Column(c).Margin(0, -pagePadX).Children(func() {
		a.xiaoyuzhouEpisodeTitles(c)
		for i := range eps {
			a.xiaoyuzhouEpisodeRow(c, eps, i, pod, ranked)
		}
	})
}

// xiaoyuzhouEpisodeTitles is the head of the episodes, as columnTitles.
func (a *app) xiaoyuzhouEpisodeTitles(c *ui.Context) {
	tableHead(c, func() {
		headText(c, "#").Width(32).TextAlign(ui.End)
		headText(c, "单集").Grow(3).Basis(0)
		headText(c, "节目").Grow(2).Basis(0)
		headText(c, "发布日期").Width(88)
		headText(c, "播放").Width(64).TextAlign(ui.End)
		headClock(c)
	})
}

// xiaoyuzhouEpisodeRow is episode i of eps: its cover, title, podcast, date,
// plays and length. A click plays eps from it.
func (a *app) xiaoyuzhouEpisodeRow(c *ui.Context, eps []xiaoyuzhou.Episode, i int, pod *xiaoyuzhou.Podcast, ranked bool) {
	e := eps[i]
	s := episodeSong(e, pod)
	playing := a.isPlaying(s)
	play := func() { a.playList(episodeSongs(eps, pod), i) }
	row := ui.Row(c.Key(e.Eid+fmt.Sprint(i))).Margin(0, pagePadX).Padding(6, 16).Gap(16).AlignItems(ui.Center).Cursor(ui.CursorPointer)
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
		ui.Row(c).Grow(3).Basis(0).MinWidth(0).Gap(12).AlignItems(ui.Center).Children(func() {
			a.cover(c, s.Pic, 36, radiusSm)
			ui.Row(c).Grow(1).MinWidth(0).Gap(6).AlignItems(ui.Center).Children(func() {
				t := ui.Text(c, e.Title).FontSize(14).FontWeight(500).SingleLine().MinWidth(0).Shrink(1).Tooltip(e.Title)
				if playing {
					t.TextColor(colAccent)
				}
				if e.Paid() {
					badge(c, "付费", colVIP).FontSize(10)
				}
			})
		})
		a.albumLink(c, s, 13).Grow(2).Basis(0)
		date := ""
		if !e.PubDate.IsZero() {
			date = e.PubDate.Local().Format("2006-01-02")
		}
		ui.Text(c, date).Width(88).FontSize(13).TextColor(colMuted).FontFeatures("tnum").SingleLine()
		plays := ""
		if e.PlayCount > 0 {
			plays = formatCount(e.PlayCount)
		}
		ui.Text(c, plays).Width(64).TextAlign(ui.End).FontSize(13).TextColor(colMuted).FontFeatures("tnum").SingleLine()
		ui.Text(c, s.Duration()).Width(48).TextAlign(ui.End).FontSize(13).TextColor(colMuted).FontFeatures("tnum")
	})
	if row.Clicked() {
		play()
	}
	row.ContextMenu(a.songMenu(s))
}

// xiaoyuzhouShowGrid shows podcasts as cards, which open their pages;
// notes, if any, is a line of each: why it is recommended.
func (a *app) xiaoyuzhouShowGrid(c *ui.Context, pods []xiaoyuzhou.Podcast, notes []string) {
	if len(pods) == 0 {
		return
	}
	ui.Grid(c).Columns(a.columns(c, 170)).Gap(12).Children(func() {
		for i, p := range pods {
			note := ""
			if i < len(notes) {
				note = notes[i]
			}
			a.xiaoyuzhouShowCard(c, p, note, false)
		}
	})
}

// xiaoyuzhouShowCard is a podcast as a card which opens its page; note, if
// not empty, is a line of why it is recommended. marked shows it is
// 小宇宙's where it is among QQ Music's.
func (a *app) xiaoyuzhouShowCard(c *ui.Context, p xiaoyuzhou.Podcast, note string, marked bool) {
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
			if marked {
				ui.Row(c).Absolute().Top(6).Left(6).Padding(1, 6).Radius(radiusSm).Background(ui.RGBA(10, 10, 10, 0.7)).Children(func() {
					ui.Text(c, "小宇宙").FontSize(11).FontWeight(500).TextColor(colAccent)
				})
			}
		})
		ui.Column(c).Gap(2).Children(func() {
			ui.Text(c, p.Title).FontSize(14).FontWeight(500).MaxLines(2).LineHeight(1.35)
			sub := p.Author
			if p.EpisodeCount > 0 {
				sub = strings.TrimPrefix(sub+fmt.Sprintf(" · %d 集", p.EpisodeCount), " · ")
			}
			ui.Text(c, sub).FontSize(12).TextColor(colMuted).SingleLine()
			if note != "" {
				ui.Text(c, note).FontSize(12).TextColor(colSubtle).MaxLines(2).LineHeight(1.4).Tooltip(note)
			}
		})
	})
	if card.Clicked() {
		a.navigate(page{kind: pageXiaoyuzhouShow, mid: p.Pid, title: p.Title})
	}
}

// xiaoyuzhouShowPage is the page of a podcast of 小宇宙: who hosts it and
// what it is about above its episodes, the newest first, the next ones
// loading as their end comes into view.
func (a *app) xiaoyuzhouShowPage(c *ui.Context, p page) {
	sh := a.xiaoyuzhou.shows[p.mid]
	if sh == nil || !sh.loaded {
		a.tint.aim(c, tintNone)
		if sh != nil && sh.err != "" {
			a.pageScroll(c, &ui.ScrollState{}, tintNone, func() {
				errorRow(c, sh.err, func() { a.loadXiaoyuzhouShow(p.mid, true) })
			})
		} else {
			a.listSkeleton(c, page{kind: pageAlbum})
		}
		return
	}
	n := len(sh.eps)
	ui.List(c, &sh.list, 2+n+1, func(i int) {
		switch {
		case i == 0:
			a.xiaoyuzhouShowHeader(c, p, sh)
		case i == 1:
			a.xiaoyuzhouEpisodeTitles(c)
		case i-2 < n:
			a.xiaoyuzhouEpisodeRow(c, sh.eps, i-2, &sh.info, false)
		default:
			ui.Column(c).Padding(8, 24).Children(func() {
				switch {
				case sh.err != "":
					errorRow(c, sh.err, func() { a.moreXiaoyuzhouShowEps(p.mid) })
				case sh.loading || sh.next != nil:
					moreRow(c, func() { a.moreXiaoyuzhouShowEps(p.mid) })
				case sh.web:
					ui.Text(c, fmt.Sprintf("网页版只提供最新 %d 集，登录小宇宙后可查看全部单集", n)).FontSize(13).TextColor(colMuted).Padding(16, 0)
				case n == 0:
					ui.Text(c, "暂无单集").TextColor(colMuted).Padding(24, 0)
				}
			})
		}
	}).Grow(1).Padding(topHeight, 0, barRoom+16, 0).Children(func() {})
}

func (a *app) xiaoyuzhouShowHeader(c *ui.Context, p page, sh *xiaoyuzhouShow) {
	info := &sh.info
	a.tint.aim(c, a.pageTint(info.Image.URL(300)))
	ui.Column(c).Gap(16).Padding(12, pagePadX, 16, pagePadX).Children(func() {
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
				var hosts []xiaoyuzhou.User
				for _, u := range info.Podcasters {
					if u.UID != "" {
						hosts = append(hosts, u)
					}
				}
				if !sh.web && len(hosts) > 0 && a.xiaoyuzhou.api.LoggedIn() {
					ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
						ui.Text(c, "主播").FontSize(12).TextColor(colSubtle)
						for _, u := range hosts {
							a.xiaoyuzhouUserChip(c, u)
						}
					})
				}
			})
		})
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			if button(c, primary, icPlay, "播放全部").Clicked() && len(sh.eps) > 0 {
				a.playList(episodeSongs(sh.eps, info), 0)
			}
			// A page of the web site knows nothing of a subscription, and
			// there is no login to make one with.
			if !sh.web && a.xiaoyuzhou.api.LoggedIn() {
				if info.Subscribed() {
					if button(c, secondary, icCheck, "已订阅").Tooltip("取消订阅").Clicked() && !sh.subscribing {
						a.setXiaoyuzhouSubscription(p.mid, false)
					}
				} else if button(c, outline, icPlus, "订阅").Clicked() && !sh.subscribing {
					a.setXiaoyuzhouSubscription(p.mid, true)
				}
			}
			if sh.loading || sh.subscribing {
				ui.Spinner(c).Size(16, 16)
			}
		})
		if sh.subErr != "" {
			ui.Text(c, sh.subErr).FontSize(13).TextColor(colDanger)
		}
	})
}

// xiaoyuzhouUserChip is a host of a podcast, which opens their page.
func (a *app) xiaoyuzhouUserChip(c *ui.Context, u xiaoyuzhou.User) {
	chip := ui.Row(c.Key(u.UID)).Gap(6).Padding(2, 8, 2, 4).Radius(radiusMd).AlignItems(ui.Center).Cursor(ui.CursorPointer)
	if chip.Hovered() {
		chip.Background(colGhost)
	}
	chip.Children(func() {
		a.cover(c, u.Avatar.Picture.URL(150), 22, 11)
		ui.Text(c, u.Nickname).FontSize(13).MaxLines(1)
	})
	if chip.Clicked() {
		a.navigate(page{kind: pageXiaoyuzhouUser, mid: u.UID, title: u.Nickname})
	}
}

// xiaoyuzhouUserPage is a host: who they are, and the podcasts they make.
func (a *app) xiaoyuzhouUserPage(c *ui.Context, p page) {
	if !a.xiaoyuzhou.api.LoggedIn() {
		a.pageScroll(c, &ui.ScrollState{}, tintNone, func() {
			ui.Text(c, "登录小宇宙后可查看主播").FontSize(14).TextColor(colMuted).Padding(24, 8)
		})
		return
	}
	us := a.xiaoyuzhou.users[p.mid]
	if us == nil || !us.loaded {
		if us != nil && us.err != "" {
			a.pageScroll(c, &us.scroll, tintNone, func() {
				errorRow(c, us.err, func() { a.loadXiaoyuzhouUser(p.mid, true) })
			})
		} else {
			a.listSkeleton(c, page{kind: pageSinger})
		}
		return
	}
	name := us.user.Nickname
	if name == "" {
		name = p.title
	}
	a.pageScroll(c, &us.scroll, a.pageTint(us.user.Avatar.Picture.URL(300)), func() {
		ui.Row(c).Gap(24).AlignItems(ui.Center).Children(func() {
			a.cover(c, us.user.Avatar.Picture.URL(300), 120, 60)
			ui.Column(c).Grow(1).MinWidth(0).Gap(8).Children(func() {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Text(c, name).FontSize(28).FontWeight(700).LetterSpacing(-0.6).MaxLines(1)
					if us.user.IPLoc != "" {
						badge(c, "IP 属地 "+us.user.IPLoc, colMuted)
					}
				})
				if us.user.Bio != "" {
					ui.Text(c, strings.ReplaceAll(us.user.Bio, "\n", " ")).FontSize(13).TextColor(colMuted).MaxLines(3).Tooltip(us.user.Bio)
				}
				var meta []string
				if us.stats.FollowingCount > 0 {
					meta = append(meta, formatCount(us.stats.FollowingCount)+" 关注")
				}
				if us.stats.FollowerCount > 0 {
					meta = append(meta, formatCount(us.stats.FollowerCount)+" 粉丝")
				}
				if us.stats.SubscriptionCount > 0 {
					meta = append(meta, formatCount(us.stats.SubscriptionCount)+" 订阅")
				}
				if len(meta) > 0 {
					ui.Text(c, strings.Join(meta, " · ")).FontSize(13).TextColor(colMuted)
				}
				a.xiaoyuzhouFollowRow(c, us)
			})
		})
		heading(c, "TA 的节目")
		if len(us.shows) == 0 {
			ui.Text(c, "暂无节目").FontSize(13).TextColor(colMuted)
		} else {
			a.xiaoyuzhouShowGrid(c, us.shows, nil)
		}
	})
}

// xiaoyuzhouFollowRow follows, or unfollows, the host of the page. A page
// without a uid is no host's, and the account's own page is not followed.
func (a *app) xiaoyuzhouFollowRow(c *ui.Context, us *xiaoyuzhouUser) {
	x := &a.xiaoyuzhou
	if !x.api.LoggedIn() || us.uid == "" || us.uid == x.user.UID {
		return
	}
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		if us.following {
			if button(c, secondary, icCheck, "已关注").Tooltip("取消关注").Clicked() && !us.followBusy {
				a.setXiaoyuzhouFollowing(us.uid, false)
			}
		} else if button(c, outline, icPlus, "关注").Clicked() && !us.followBusy {
			a.setXiaoyuzhouFollowing(us.uid, true)
		}
		if us.followBusy {
			ui.Spinner(c).Size(16, 16)
		}
	})
	if us.followErr != "" {
		ui.Text(c, us.followErr).FontSize(13).TextColor(colDanger)
	}
}
