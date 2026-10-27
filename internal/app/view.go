package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"qqmusic/internal/qqmusic"
)

// The colors: shadcn/ui's dark theme of neutral grays, in its Nova style,
// with QQ Music's green as the primary color.
var (
	colWindow    = ui.Hex("#171717")             // --sidebar: the frame around the pages
	colPanel     = ui.Hex("#0a0a0a")             // --background: the pages
	colCard      = ui.Hex("#171717")             // --card
	colCardHover = ui.Hex("#1f1f1f")             // a card under the pointer
	colHover     = ui.Hex("#262626")             // --accent, --muted, --secondary
	colPressed   = ui.Hex("#2e2e2e")             // --secondary pressed
	colGhost     = ui.RGBA(255, 255, 255, 0.07)  // a ghost button under the pointer, over any page
	colAccent    = ui.Hex("#31c27c")             // --primary
	colAccentH   = ui.Hex("#2cae6f")             // --primary/90
	colAccentFg  = ui.Hex("#052e16")             // --primary-foreground
	colText      = ui.Hex("#fafafa")             // --foreground
	colMuted     = ui.Hex("#a1a1a1")             // --muted-foreground
	colSubtle    = ui.RGBA(255, 255, 255, 0.55)  // what is less than muted: singers under titles, a section's actions
	colDim       = ui.Hex("#737373")             // --ring
	colLine      = ui.RGBA(255, 255, 255, 0.10)  // --border
	colFaint     = ui.RGBA(255, 255, 255, 0.05)  // the edge of cards in a grid, there but not looked at
	colInput     = ui.RGBA(255, 255, 255, 0.15)  // --input
	colInputBg   = ui.RGBA(255, 255, 255, 0.045) // --input/30, the face of inputs and outline buttons
	colInputBgH  = ui.RGBA(255, 255, 255, 0.075) // --input/50
	colRing      = ui.RGBA(115, 115, 115, 0.5)   // --ring/50, around what has the focus
	colDanger    = ui.Hex("#ff6467")             // --destructive
	colVIP       = ui.Hex("#f5c242")
	colShadow    = ui.RGBA(0, 0, 0, 0.35)
)

// The radii, as shadcn's of --radius 0.625rem.
const (
	radiusSm = 6  // rounded-sm: badges
	radiusMd = 8  // rounded-md: buttons, inputs, menu items
	radiusLg = 10 // rounded-lg: covers
	radiusXl = 14 // rounded-xl: cards, panels, dialogs
)

const (
	sidebarWidth = 240
	barHeight    = 76
	topHeight    = 52  // the title bar at the top of the page
	tintHeight   = 360 // how far down the page's color fades out
	queueWidth   = 340 // the queue, a column on the right of the panel
)

func theme() *ui.Theme {
	t := *ui.DarkTheme()
	t.Background, t.Surface, t.SurfaceHover, t.SurfacePressed = colPanel, colInputBg, colInputBgH, ui.RGBA(255, 255, 255, 0.11)
	t.Border, t.Text, t.TextMuted = colInput, colText, colMuted
	t.Accent, t.AccentHover, t.AccentPressed, t.AccentText = colAccent, colAccentH, ui.Hex("#279c63"), colAccentFg
	t.Danger = colDanger
	t.Selection = ui.RGBA(49, 194, 124, 0.3)
	t.Focus = colRing
	// Tooltips are the foreground turned over, as shadcn's.
	t.Inverse, t.InverseText = colText, colPanel
	t.Scrollbar = ui.RGBA(255, 255, 255, 0.16)
	t.Radius = radiusMd
	// Nova is shadcn's compact style: less room inside and between.
	t.Spacing = 3
	t.FontSize = 14
	t.Font = "Segoe UI, Microsoft YaHei UI, PingFang SC, Noto Sans CJK SC"
	return &t
}

func (a *app) view(c *ui.Context) {
	c.SetTheme(theme())
	for _, m := range a.toasts {
		c.Toast(m)
	}
	a.toasts = nil
	a.images.frame()
	a.tick()
	if a.player.Playing() || a.player.Loading() {
		if a.showLyrics {
			c.After(100 * time.Millisecond)
		} else {
			c.After(250 * time.Millisecond)
		}
	}
	if c.Shortcut(0, ui.KeySpace) {
		a.togglePlay()
	}
	if c.Shortcut(ui.Alt, ui.KeyLeft) {
		a.back()
	}
	if c.Shortcut(ui.Alt, ui.KeyRight) {
		a.forward()
	}

	// As shadcn's inset sidebar: the sidebar and the player bar are the
	// frame, the page an inset panel within it, up to the top of the
	// window, with the title bar on its color.
	ui.Row(c).Fill().Background(colWindow).Gap(8).PaddingX(8).AlignItems(ui.Stretch).Children(func() {
		a.sidebar(c)
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			ui.Box(c).Height(8).DragWindow()
			ui.Row(c).Grow(1).AlignItems(ui.Stretch).MinHeight(0).Children(func() {
				// The queue is a column of the panel, on its right, on the
				// same color as the page: one panel, not two.
				ui.Row(c).Grow(1).MinWidth(0).AlignItems(ui.Stretch).Radius(radiusXl).Background(colPanel).Clip().Draw(a.tint.band(tintHeight)).DrawOver(fadingPanelCorners).Children(func() {
					ui.Column(c).Grow(1).MinWidth(0).Children(func() {
						// Each page is an element of its own, which comes in as it
						// is first shown. The lyrics cover it, and the title bar,
						// coming and going; once they do, neither needs building.
						if p := a.current().key(); p != a.pageShown {
							a.pageShown, a.pageIn = p, false
						}
						if !a.showLyrics || !a.lyricsShow.rest {
							a.topbar(c)
							pg := ui.Column(c.Key(a.pageShown)).Grow(1).MinHeight(0)
							appear(c, pg, &a.pageIn, pageOut, pageSpring)
							pg.Children(func() { a.content(c) })
						}
					})
					// The room of the queue: its own, kept built while it
					// slides out. It opens and closes with the spring it fades
					// with, for the page to give way and take it back in step;
					// the queue, at the room's left edge, slides with it. The
					// comments take the same room, in place of the queue.
					// Over the lyrics, the room opens at once, and stays open
					// while a panel goes, which slides out by itself: laying
					// out the lyrics again in every frame is slow, and they
					// are behind the room anyway.
					qslot := ui.Box(c.Key("queueSlot"))
					qw := float32(0)
					if a.sidePanel() {
						qw = queueWidth
					}
					if a.showLyrics && a.lyricsShow.rest {
						if a.queueShow.built || a.commentsShow.built {
							qw = queueWidth
						}
						qw = qslot.Jump("width", qw)
					} else {
						qw = qslot.AnimateTo("width", qw, sideSpring)
					}
					qslot.Width(qw)
					// The lyrics cover the panel whole, the room too.
					if a.lyricsShow.shown(c, a.showLyrics) {
						lyrics := ui.Column(c.Key("lyrics")).Absolute().Top(0).Left(0).Right(0).Bottom(0).Background(colPanel)
						a.lyricsShow.animate(c, lyrics, lyricsOut, lyricsSpring)
						lyrics.Children(func() {
							a.nowPlayingPage(c)
							// Where it leaves room, it moves the window as the title bar.
							ui.Box(c).Absolute().Top(0).Left(56).Right(0).Height(topHeight).DragWindow()
						})
					}
					// The panels go over the room, and over the lyrics, after
					// which they paint: absolute children paint above those in
					// the flow, the room among them.
					ui.Box(c.Key("sidePanels")).Absolute().Top(0).Right(0).Bottom(0).Width(qw).Children(func() {
						if a.queueShow.shown(c, a.showQueue) {
							a.queuePanel(c)
						}
						if a.commentsShow.shown(c, a.commentsShown()) {
							a.commentsPanel(c)
						}
					})
				})
				a.windowControls(c)
			})
			a.playerBar(c)
		})
	})
	a.loginDialog(c)
}

// fadingPanelCorners paints the frame's color over the corners of the inset
// panel, outside its rounding, and its border again. On the GPU only the
// innermost clip is rounded, the others cut square: what a scroll view in
// the panel paints, as the tint at the top of a page, would show in its
// corners. The border fades out towards the top, where the page's tint and
// the title bar already mark the panel's edge: a full border there makes
// the top look heavy.
//
// Only the corners are paths, each its own, and the sides are rectangles:
// the mask of a path is as large as its bounds, and two around the panel
// of a large window overflow the glyph atlas, which is then repacked in
// every frame that paints, and every motion stutters.
func fadingPanelCorners(p *ui.Painter, r ui.Rect) {
	// Angle 0 runs from the bottom up: the bottom quarter is at From, and
	// the top all but at To.
	g := ui.LinearGradient{From: colLine.Alpha(0.7), To: colLine.Alpha(0.1), Angle: 0, Start: 0.25, End: 1}
	// The handles of a quarter circle drawn with a cubic Bézier are 0.5523
	// of the radius from its ends. The border is 1 wide, inside r.
	const rad, in, k = radiusXl, radiusXl - 1, 0.5523
	for _, cn := range [4][4]float32{
		{r.X, r.Y, 1, 1}, {r.X + r.W, r.Y, -1, 1}, {r.X + r.W, r.Y + r.H, -1, -1}, {r.X, r.Y + r.H, 1, -1},
	} {
		// The corner, and the center of its rounding.
		x, y, dx, dy := cn[0], cn[1], cn[2], cn[3]
		cx, cy := x+dx*rad, y+dy*rad
		var mask, ring ui.Path
		mask.MoveTo(x, y).LineTo(cx, y).
			CubeTo(cx-dx*rad*k, y, x, cy-dy*rad*k, x, cy).Close()
		p.FillPath(&mask, colWindow)
		ring.MoveTo(x, cy).CubeTo(x, cy-dy*rad*k, cx-dx*rad*k, y, cx, y).
			LineTo(cx, cy-dy*in).CubeTo(cx-dx*in*k, cy-dy*in, cx-dx*in, cy-dy*in*k, cx-dx*in, cy).Close()
		col := g.From
		if dy > 0 {
			col = g.To
		}
		p.FillPath(&ring, col)
	}
	p.Fill(ui.Rect{X: r.X + rad, Y: r.Y, W: r.W - 2*rad, H: 1}, g.To, 0)
	p.Fill(ui.Rect{X: r.X + rad, Y: r.Y + r.H - 1, W: r.W - 2*rad, H: 1}, g.From, 0)
	p.FillGradient(ui.Rect{X: r.X, Y: r.Y + rad, W: 1, H: r.H - 2*rad}, g, 0)
	p.FillGradient(ui.Rect{X: r.X + r.W - 1, Y: r.Y + rad, W: 1, H: r.H - 2*rad}, g, 0)
}

// windowControlsWidth is the room the window controls take from the right
// of the title bar.
const windowControlsWidth = 3*32 + 2*2 + 10

// topbar is the title bar at the top of the page, on its color: history,
// and search in the middle.
func (a *app) topbar(c *ui.Context) {
	// Room for the window controls, unless a panel is beside the page,
	// below them.
	right := float32(windowControlsWidth)
	if a.sidePanel() {
		right = 0
	}
	ui.Row(c).Height(topHeight).PaddingX(16).AlignItems(ui.Center).DragWindow().Children(func() {
		// As wide as each other, so that search is in the middle.
		ui.Row(c).Grow(1).Gap(4).AlignItems(ui.Center).Children(func() {
			if ghostButton(c, icBack, len(a.history) > 1, "后退", false).Clicked() {
				a.back()
			}
			if ghostButton(c, icForward, len(a.future) > 0, "前进", false).Clicked() {
				a.forward()
			}
			if ghostButton(c, icRefresh, a.current().kind != pageRecent, "刷新", a.refreshing()).Clicked() {
				a.refresh()
			}
		})
		a.searchBox(c)
		ui.Box(c).Grow(1).MinWidth(right)
	})
}

// windowControls are the window's minimize, maximize and close buttons,
// in the top right corner of the window.
func (a *app) windowControls(c *ui.Context) {
	ui.Row(c).Absolute().Top((topHeight - 32) / 2).Right(10).Gap(2).Children(func() {
		if windowButton(c, icMinimize, "最小化", false).Clicked() {
			a.plat.Minimize()
		}
		ic, tip := icMaximize, "最大化"
		if a.plat.Maximized() {
			ic, tip = icRestore, "还原"
		}
		if windowButton(c, ic, tip, false).Clicked() {
			a.plat.ToggleMaximize()
		}
		if windowButton(c, icClose, "关闭", true).Clicked() {
			a.plat.Close()
		}
	})
}

// windowButton is a ghost icon button of the window controls; close turns
// red under the pointer, as Windows' own.
func windowButton(c *ui.Context, ic *ui.SVG, tip string, close bool) ui.Element {
	b := ui.Box(c).Size(32, 32).Radius(radiusMd).Center().Tooltip(tip)
	col := colMuted
	if b.Hovered() {
		col = colText
		if close {
			b.Background(ui.Hex("#c42b1c"))
		} else {
			b.Background(colGhost)
		}
	}
	b.Children(func() { ui.Icon(c, ic).FontSize(16).TextColor(col) })
	return b
}

// ghostButton is shadcn's ghost icon button, 32 square, which may be
// disabled; spin turns its icon while what it starts goes on.
func ghostButton(c *ui.Context, ic *ui.SVG, enabled bool, tip string, spin bool) ui.Element {
	b := ui.Box(c).Size(32, 32).Radius(radiusMd).Center().Tooltip(tip)
	col := colDim
	if enabled {
		b.Cursor(ui.CursorPointer)
		col = colMuted
		if b.Hovered() {
			b.Background(colHover)
			col = colText
		}
	}
	turn := float32(0)
	if spin {
		turn = b.Loop("spin", time.Second, ui.Linear) * 360
	}
	b.Children(func() { ui.Icon(c, ic).FontSize(16).TextColor(col).Rotate(turn) })
	if !enabled {
		return ui.Element{}
	}
	return b
}

// searchBox is shadcn's input, with an icon before the text, and hanging
// below it what was searched before, or what the text may become.
func (a *app) searchBox(c *ui.Context) {
	s := &a.search
	podcasts := a.current().kind == pagePodcast
	xyzShown := a.current().kind == pageXyz
	box := ui.Row(c).Width(360).MinWidth(140).Height(32).Radius(radiusMd).Background(colInputBg).Border(1, colInput).PaddingX(10).Gap(8).AlignItems(ui.Center)
	box.Children(func() {
		ui.Icon(c, icSearch).FontSize(16).TextColor(colMuted)
		placeholder, label := "搜索歌曲、歌手、专辑…", "搜索歌曲"
		if podcasts {
			placeholder, label = "搜索小宇宙的节目、单集…", "搜索播客"
		}
		if xyzShown {
			placeholder, label = "搜索小宇宙官方的节目、单集…", "搜索小宇宙"
		}
		in := ui.TextInputBase(c, &s.input).Placeholder(placeholder).Label(label).Grow(1).FontSize(14).FocusBind(&s.focus, true)
		in.OnTextKey(a.searchKeys)
		// Typing opens the menu, and asks for what the text may become
		// once it rests: what was chosen before is not what is typed.
		if in.Changed() {
			s.open, s.sel = true, -1
			if !podcasts && !xyzShown {
				a.suggestAsTyped(s.input)
			}
		}
		if in.Submitted() && s.input != "" {
			if words := a.searchWords(); s.sel >= 0 && s.sel < len(words) {
				a.chooseSearch(words[s.sel])
			} else {
				a.chooseSearch(s.input)
			}
		}
		// A press opens it again once Escape closed it, or a row took it.
		if in.Clicked() {
			s.open = true
		}
		// FocusWithin stays true while another window is in front, Focused
		// does not: switching away closes the menu, and coming back, the
		// focus never having left the box, does not open it again.
		focused := in.FocusWithin()
		// A press on nothing that takes the focus leaves it nowhere, and
		// MyGo gives it back to the box, which had it as the menu opened,
		// once the menu is gone. Coming so, neither by a press nor by the
		// keyboard, it is let go.
		if focused && !s.focused && in.Focused() && !in.FocusVisible() && !in.Pressed() && !in.Clicked() {
			s.focus, focused = false, false
			c.Invalidate()
		}
		if focused && in.Focused() {
			box.Border(1, colDim).Shadow(0, 0, 0, 3, colRing)
			if !s.focused {
				s.open = true
			}
		} else {
			if box.Hovered() {
				box.Background(colInputBgH)
			}
			s.open = false
		}
		s.focused = focused
		if !s.open {
			s.sel = -1
		}
		if s.input != "" {
			clear := ui.Box(c).Padding(2).Cursor(ui.CursorPointer).KeepFocus()
			clear.Children(func() { ui.Icon(c, icClose).FontSize(14).TextColor(colMuted) })
			if clear.Clicked() {
				s.input, s.open, s.sel = "", true, -1
				a.suggestAsTyped("")
				// Cleared on 小宇宙, it goes back to the shelves.
				if podcasts && a.podcast.search.query != "" {
					a.searchPodcasts("")
				}
				if xyzShown && a.xyz.search.query != "" {
					a.searchXyz("")
				}
			}
		}
	})
	a.searchMenu(c, box)
}

// searchMenu hangs below the search box: the searches made before while it
// is empty, and as the text is typed, the search of it and what the text
// may become, which the arrows choose and Enter searches.
func (a *app) searchMenu(c *ui.Context, anchor ui.Element) {
	s := &a.search
	q := strings.TrimSpace(s.input)
	if !s.open || q == "" && len(a.set.Searches) == 0 {
		return
	}
	words := a.searchWords()
	ui.PopoverBase(c, anchor, &s.open, func(panel ui.Element) {
		panel.AttachTo(anchor, ui.AnchorBottomStart, ui.AnchorTopStart).Margin(6, 0, 0, 0).
			Padding(4).Gap(1).Width(360).Radius(radiusLg).Background(colCard).Border(1, colLine).
			Shadow(0, 8, 24, 0, colShadow).KeepFocus()
		// A row picks its text, which the box takes and searches; i is the
		// suggestion it is, -1 for a row the arrows cannot reach.
		row := func(key string, i int, label string, ic *ui.SVG) ui.Element {
			r := ui.Row(c.Key(key)).Padding(6, 8).Gap(8).Radius(radiusMd).AlignItems(ui.Center).Cursor(ui.CursorPointer)
			if r.Hovered() || i >= 0 && i == s.sel {
				r.Background(colHover)
			}
			r.Children(func() {
				ui.Icon(c, ic).FontSize(14).TextColor(colMuted)
				ui.Text(c, label).FontSize(13).Grow(1).SingleLine()
			})
			return r
		}
		if q == "" {
			ui.Row(c).Padding(4, 4, 4, 8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "搜索历史").FontSize(12).TextColor(colMuted).Grow(1)
				if iconButton(c, icTrash, 13, colMuted, "清空搜索历史").Clicked() {
					a.clearSearches()
				}
			})
			for _, h := range a.set.Searches {
				if row("hist "+h, -1, h, icClock).Clicked() {
					a.chooseSearch(h)
				}
			}
			return
		}
		label := "搜索 “" + q + "”"
		switch a.current().kind {
		case pagePodcast:
			label = "在小宇宙中搜索 “" + q + "”"
		case pageXyz:
			label = "在小宇宙官方中搜索 “" + q + "”"
		}
		if row("query "+q, -1, label, icSearch).Clicked() {
			a.chooseSearch(q)
		}
		for i, w := range words {
			if row("sug "+w, i, w, icSearch).Clicked() {
				a.chooseSearch(w)
			}
		}
	})
}

// userArea is the user at the foot of the sidebar, with a menu, or a way
// in when no one is logged in, as shadcn's NavUser.
func (a *app) userArea(c *ui.Context) {
	row := ui.Row(c).Margin(0, 8, 8, 8).Padding(10, 16).Gap(12).Radius(radiusMd).AlignItems(ui.Center).Cursor(ui.CursorPointer)
	if row.Hovered() {
		row.Background(colHover)
	}
	if !a.api.LoggedIn() {
		row.Children(func() {
			a.avatar(c, "", "", 32)
			ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
				ui.Text(c, "点击登录").FontSize(14).FontWeight(500).SingleLine()
				ui.Text(c, "登录后同步歌单与红心").FontSize(12).TextColor(colMuted).SingleLine()
			})
		})
		if row.Clicked() {
			a.openLogin()
		}
		return
	}
	nick := a.user.Nick
	if nick == "" {
		nick = fmt.Sprint(a.api.Credential().MusicID)
	}
	sub := "QQ 音乐"
	if a.user.VIP {
		sub = "绿钻会员"
	}
	row.Menu(func(m *ui.Menu) {
		if m.Item("刷新我的歌单").Chosen() {
			a.loadMine()
		}
		m.Separator()
		if m.Item("退出登录").Chosen() {
			a.logout()
		}
	}).Children(func() {
		a.avatar(c, a.user.Avatar, nick, 32)
		ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
			ui.Text(c, nick).FontSize(14).FontWeight(500).SingleLine()
			ui.Text(c, sub).FontSize(12).TextColor(colMuted).SingleLine()
		})
		ui.Icon(c, icUpDown).FontSize(16).TextColor(colMuted)
	})
}

// avatar is a user's picture, rounded as shadcn's NavUser shows it.
func (a *app) avatar(c *ui.Context, url, name string, size float32) {
	if bmp := a.images.get(url, int(size*2)); bmp != nil {
		ui.Image(c, bmp).Size(size, size).Radius(radiusMd).Fit(ui.Cover)
		return
	}
	ui.Box(c).Size(size, size).Radius(radiusMd).Background(colHover).Center().Children(func() {
		ui.Icon(c, icUser).FontSize(size * 0.5).TextColor(colMuted)
	})
}

// variant is the look of a button, as shadcn's.
type variant int

const (
	primary   variant = iota // the primary color
	secondary                // a gray face
	outline                  // a border, on the face of inputs
	ghost                    // no face until hovered
)

// button is shadcn's button, 32 high: an icon, a label, or both.
func button(c *ui.Context, v variant, ic *ui.SVG, label string) ui.Element {
	b := ui.Row(c).Height(32).Radius(radiusMd).AlignItems(ui.Center).Justify(ui.Center).Gap(6).Cursor(ui.CursorPointer)
	switch {
	case label == "":
		b.Width(32)
	case ic != nil:
		b.Padding(0, 12, 0, 10)
	default:
		b.PaddingX(12)
	}
	hover := b.Hovered()
	bg, fg := ui.Color{}, colText
	switch v {
	case primary:
		bg, fg = colAccent, colAccentFg
		if hover {
			bg = colAccentH
		}
	case secondary:
		bg = colHover
		if hover {
			bg = colPressed
		}
	case outline:
		bg = colInputBg
		b.Border(1, colInput)
		if hover {
			bg = colInputBgH
		}
	case ghost:
		if hover {
			bg = colGhost
		}
	}
	if b.Pressed() {
		b.Translate(0, 1)
	}
	b.Background(bg).Children(func() {
		if ic != nil {
			ui.Icon(c, ic).FontSize(16).TextColor(fg)
		}
		if label != "" {
			ui.Text(c, label).FontSize(14).FontWeight(500).TextColor(fg).SingleLine()
		}
	})
	return b
}

// toggle is a small button of a group of choices, as shadcn's outline
// ToggleGroup: on, it has the accent's face.
func toggle(c *ui.Context, label string, on bool) ui.Element {
	b := ui.Box(c).Height(28).PaddingX(10).Radius(radiusMd).Center().Border(1, colInput).Cursor(ui.CursorPointer)
	fg := colMuted
	switch {
	case on:
		b.Background(colHover)
		fg = colText
	case b.Hovered():
		b.Background(colGhost)
		fg = colText
	}
	b.Children(func() {
		t := ui.Text(c, label).FontSize(13).TextColor(fg).SingleLine()
		if on {
			t.FontWeight(500)
		}
	})
	return b
}

// tabs is shadcn's Tabs list: the labels in a gray track, the one chosen
// raised; filling its parent's width, the labels share it. A click calls
// choose after the build.
func tabs(c *ui.Context, selected int, fill bool, choose func(int), labels ...string) {
	list := ui.Row(c).Height(32).Padding(3).Radius(radiusLg).Background(colHover).Gap(2).AlignItems(ui.Stretch)
	if fill {
		list.FillWidth()
	}
	list.Children(func() {
		for i, name := range labels {
			seg := ui.Row(c.Key(i)).PaddingX(12).Radius(radiusMd).Center().Cursor(ui.CursorPointer)
			if fill {
				seg.Grow(1).Basis(0)
			}
			fg := colMuted
			switch {
			case i == selected:
				seg.Background(ui.Hex("#323232")).Border(1, colInput).Shadow(0, 1, 2, 0, colShadow)
				fg = colText
			case seg.Hovered():
				fg = colText
			}
			seg.Children(func() { ui.Text(c, name).FontSize(13).FontWeight(500).TextColor(fg).SingleLine() })
			seg.OnClick(func() { choose(i) })
		}
	})
}

// badge is shadcn's outline Badge.
func badge(c *ui.Context, label string, col ui.Color) ui.Element {
	return ui.Text(c, label).FontSize(11).FontWeight(500).TextColor(col).Padding(1, 6).Radius(radiusSm).Border(1, col.Alpha(0.45)).NoWrap()
}

// podcastTitle is the title of 小宇宙, its source raised at its top right
// as a footnote mark, and beside it a quiet button to the other one.
func (a *app) podcastTitle(c *ui.Context) {
	source, other := "QQ音乐", "切换到小宇宙官方"
	if a.set.XyzOfficial {
		source, other = "官方", "切换到 QQ 音乐的小宇宙"
	}
	ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
		ui.Row(c).Gap(3).AlignItems(ui.Start).Children(func() {
			ui.Text(c, "小宇宙").FontSize(26).FontWeight(600).LetterSpacing(-0.6)
			ui.Text(c, source).FontSize(10).FontWeight(600).TextColor(colAccent).Background(colAccent.Alpha(0.14)).
				Padding(1, 6).Radius(99).MarginY(2).NoWrap()
		})
		if iconButton(c, icSwap, 15, colMuted, other).Label(other).Clicked() {
			a.switchPodcasts()
		}
	})
}

// sidebar is the library on the left, as shadcn's Sidebar: groups of
// menu items under small labels, the user at the foot.
func (a *app) sidebar(c *ui.Context) {
	ui.Column(c).Width(sidebarWidth).Children(func() {
		// The logo, level with the page's title bar; with it, the top of
		// the sidebar moves the window.
		ui.Row(c).Height(8+topHeight).Padding(8, 24, 0, 24).AlignItems(ui.Center).Gap(10).DragWindow().Children(func() {
			ui.Image(c, icLogo).Size(24, 24)
			ui.Text(c, "QQ音乐").FontSize(15).FontWeight(600)
		})
		ui.Scroll(c).Grow(1).MinHeight(0).Padding(4, 8).Children(func() {
			cur := a.current()
			if navItem(c, icHome, "推荐", cur.kind == pageHome).Clicked() {
				a.navigate(page{kind: pageHome})
			}
			if navItem(c, icCompass, "音乐馆", cur.kind == pageDiscover || cur.kind == pageToplist).Clicked() {
				a.navigate(page{kind: pageDiscover})
			}
			if navItem(c, icPodcast, "小宇宙", cur.kind == pagePodcast || cur.kind == pageXyz || cur.kind == pageXyzShow || cur.kind == pageXyzCategory).Clicked() {
				a.navigate(a.podcastHome())
			}
			if navItem(c, icSearch, "搜索", cur.kind == pageSearch).Clicked() {
				a.navigate(page{kind: pageSearch})
			}
			section(c, "我的音乐")
			if !a.api.LoggedIn() {
				ui.Column(c).Margin(4, 0).Padding(16).Gap(12).Radius(radiusXl).Background(colPanel).Border(1, colLine).Children(func() {
					ui.Column(c).Gap(4).Children(func() {
						ui.Text(c, "登录 QQ 音乐").FontWeight(600).FontSize(14)
						ui.Text(c, "登录后可播放歌曲，同步你的歌单与红心").FontSize(12).TextColor(colMuted).LineHeight(1.45)
					})
					if button(c, primary, nil, "立即登录").FillWidth().Clicked() {
						a.openLogin()
					}
				})
				return
			}
			liked := page{kind: pagePlaylist, dirid: 201, title: "我喜欢"}
			if navItem(c, icHeart, "我喜欢", cur.key() == liked.key()).Clicked() {
				a.navigate(liked)
			}
			if navItem(c, icClock, "最近播放", cur.kind == pageRecent).Clicked() {
				a.navigate(page{kind: pageRecent, title: "最近播放"})
			}
			if a.loggingIn {
				ui.Row(c).Height(44).PaddingX(16).Gap(12).AlignItems(ui.Center).Children(func() {
					ui.Spinner(c).Size(14, 14)
					ui.Text(c, "正在同步…").FontSize(13).TextColor(colMuted)
				})
			}
			a.playlistSection(c, "创建的歌单", a.myLists, &a.set.MineClosed, true)
			a.playlistSection(c, "收藏的歌单", a.favLists, &a.set.FavClosed, false)
		})
		a.userArea(c)
	})
}

func (a *app) playlistSection(c *ui.Context, title string, pls []qqmusic.Playlist, closed *bool, skipLiked bool) {
	if len(pls) == 0 {
		return
	}
	head := ui.Row(c).Height(40).Margin(12, 0, 4, 0).PaddingX(16).Gap(6).Radius(radiusMd).AlignItems(ui.Center).Cursor(ui.CursorPointer)
	fg := colMuted
	if head.Hovered() {
		head.Background(colHover)
		fg = colText
	}
	head.Children(func() {
		ui.Text(c, title).FontSize(13).FontWeight(500).TextColor(fg).Grow(1)
		ui.Text(c, fmt.Sprint(len(pls))).FontSize(13).TextColor(colDim).FontFeatures("tnum")
		ic := icDown
		if *closed {
			ic = icForward
		}
		ui.Icon(c, ic).FontSize(14).TextColor(fg)
	}).OnClick(func() {
		*closed = !*closed
		a.saveSettings()
	})
	if *closed {
		return
	}
	cur := a.current()
	for _, p := range pls {
		if skipLiked && p.DirID == 201 {
			continue
		}
		pg := page{kind: pagePlaylist, id: p.ID, title: p.Title}
		if p.ID == 0 {
			pg.dirid = p.DirID
		}
		row := ui.Row(c.Key(p.ID)).Padding(6, 16).Gap(12).Radius(radiusMd).AlignItems(ui.Center).Cursor(ui.CursorPointer)
		if cur.key() == pg.key() || row.Hovered() {
			row.Background(colHover)
		}
		row.Children(func() {
			a.cover(c, p.Cover, 40, radiusSm)
			ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
				ui.Text(c, p.Title).FontSize(14).FontWeight(500).SingleLine()
				ui.Text(c, fmt.Sprintf("%d 首", p.SongCount)).FontSize(12).TextColor(colMuted).SingleLine()
			})
		})
		if row.Clicked() {
			a.navigate(pg)
		}
	}
}

// section is the label of a group of the sidebar.
func section(c *ui.Context, title string) {
	ui.Text(c, title).FontSize(13).FontWeight(500).TextColor(colMuted).Padding(18, 16, 8, 16)
}

// navItem is a menu item of the sidebar, with QQ Music's room: 44 high,
// the accent's face when active or hovered.
func navItem(c *ui.Context, ic *ui.SVG, label string, active bool) ui.Element {
	row := ui.Row(c).Height(44).Margin(1, 0).PaddingX(16).Gap(12).Radius(radiusMd).AlignItems(ui.Center).Cursor(ui.CursorPointer)
	if active || row.Hovered() {
		row.Background(colHover)
	}
	row.Children(func() {
		ui.Icon(c, ic).FontSize(18).TextColor(colText)
		t := ui.Text(c, label).FontSize(15).TextColor(colText)
		if active {
			t.FontWeight(500)
		}
	})
	return row
}

// coverOr shows a picture, or as it downloads the one at fallback if that
// came already, as a smaller cover of the same album.
func (a *app) coverOr(c *ui.Context, url, fallback string, size, radius float32) ui.Element {
	if bmp := a.images.get(url, 300); bmp == nil {
		if bmp := a.images.peek(fallback); bmp != nil {
			return ui.Image(c, bmp).Size(size, size).Radius(radius).Fit(ui.Cover)
		}
	}
	return a.cover(c, url, size, radius)
}

// cover shows a picture, or a placeholder while it downloads.
func (a *app) cover(c *ui.Context, url string, size, radius float32) ui.Element {
	px := 300
	if size <= 64 {
		px = 120
	}
	if bmp := a.images.get(url, px); bmp != nil {
		return ui.Image(c, bmp).Size(size, size).Radius(radius).Fit(ui.Cover)
	}
	return ui.Box(c).Size(size, size).Radius(radius).Background(colHover).Center().Children(func() {
		ui.Icon(c, icMusic).FontSize(size * 0.35).TextColor(colDim)
	})
}

func formatCount(n int64) string {
	switch {
	case n >= 100000000:
		return fmt.Sprintf("%.1f亿", float64(n)/1e8)
	case n >= 10000:
		return fmt.Sprintf("%.1f万", float64(n)/1e4)
	default:
		return fmt.Sprint(n)
	}
}

// formatSize formats bytes as Windows does, in MiB called MB.
func formatSize(n int64) string {
	return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
}

// formatTime formats a time of a song as m:ss, or h:mm:ss in a long
// episode.
func formatTime(d time.Duration) string {
	s := int(d.Seconds())
	if s >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", s/3600, s/60%60, s%60)
	}
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}
