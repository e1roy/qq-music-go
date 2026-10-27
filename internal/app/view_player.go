package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"qqmusic/internal/playqueue"
	"qqmusic/internal/qqmusic"
)

// iconButton is shadcn's ghost icon button: a bare icon that gets a face
// and brightens under the pointer.
func iconButton(c *ui.Context, ic *ui.SVG, size float32, col ui.Color, tip string) ui.Element {
	b := ui.Box(c).Padding(7).Radius(radiusMd).Cursor(ui.CursorPointer).Center()
	if tip != "" {
		b.Tooltip(tip)
	}
	if b.Hovered() {
		b.Background(colGhost)
		if col == colMuted {
			col = colText
		}
	}
	if b.Pressed() {
		b.Translate(0, 1)
	}
	b.Children(func() { ui.Icon(c, ic).FontSize(size).TextColor(col) })
	return b
}

// barFace poses the bar's card or the frost around it: faded out over the
// lyrics, where the bar is part of the page, as they come out of the cover.
func (a *app) barFace(el ui.Element) {
	fade := float32(0)
	if a.showLyrics {
		fade = 1
	}
	el.AnimatePose(ui.Pose{Fade: fade}, lyricsSpring)
}

func (a *app) playerBar(c *ui.Context) {
	s, ok := a.nowPlaying()
	cover := ""
	if ok {
		cover = s.Cover(300)
	}
	a.barTint.aim(c, a.pageTint(cover))
	// A card floating over the bottom of the panel (barRoom), with a wash
	// of the cover's color fading out from the left, under the song. The
	// card is a layer of its own under the controls, for it to fade, in
	// frames that only paint, as the lyrics open.
	a.barFace(ui.Box(c.Key("barFace")).Absolute().Left(barInset).Right(barInset).Bottom(barInset).Height(barHeight).PassThrough().
		Radius(radiusXl).Background(colCard).Border(1, colLine).Shadow(0, 8, 24, 0, ui.RGBA(0, 0, 0, 0.5)).
		Draw(func(p *ui.Painter, r ui.Rect) {
			col := a.barTint.color(p)
			p.FillGradient(r, ui.LinearGradient{From: col.Alpha(0.35), To: col.Alpha(0), Angle: 90, End: 0.5}, radiusXl)
		}))
	bar := ui.Row(c).Absolute().Left(barInset).Right(barInset).Bottom(barInset).Height(barHeight).Padding(0, 16, 0, barPadX).AlignItems(ui.Center).Gap(16)
	bar.Children(func() {
		// The song, in a room of its own: the one coming slides in over that
		// edge rather than over the controls.
		ui.Row(c).WidthPercent(28).MinWidth(200).Gap(12).AlignItems(ui.Center).ClipX().Children(func() {
			if !ok {
				return
			}
			a.songCell(c, s)
			// An episode of 小宇宙 is not QQ Music's to like.
			if fromQQ(s) {
				ic, col := icHeart, colMuted
				if a.liked[int64(s.ID)] {
					ic, col = icHeartOn, colAccent
				}
				if iconButton(c, ic, 16, col, "喜欢").Clicked() {
					a.toggleLike(s)
				}
			}
		})
		// The controls.
		ui.Column(c).Grow(1).MaxWidth(680).Gap(4).AlignItems(ui.Center).Children(func() {
			ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
				a.modeButton(c)
				if iconButton(c, icPrev, 22, colText, "上一首").Clicked() {
					a.prev()
				}
				// shadcn's primary icon button, made round.
				play := ui.Box(c).Size(36, 36).Radius(18).Background(colAccent).Center().Cursor(ui.CursorPointer).Shadow(0, 1, 3, 0, colShadow)
				if play.Hovered() {
					play.Background(colAccentH)
				}
				if play.Pressed() {
					play.Scale(0.96, 0.96)
				}
				play.Children(func() {
					if a.player.Loading() {
						ui.Spinner(c).Size(16, 16).TextColor(colAccentFg)
						return
					}
					ic := icPlay
					if a.player.Playing() {
						ic = icPause
					}
					ui.Icon(c, ic).FontSize(16).TextColor(colAccentFg)
				})
				if play.Clicked() {
					a.togglePlay()
				}
				if iconButton(c, icNext, 22, colText, "下一首").Clicked() {
					a.next(false)
				}
				col := colMuted
				if a.showLyrics {
					col = colAccent
				}
				if iconButton(c, icLyric, 16, col, "歌词").Clicked() {
					a.toggleLyrics()
				}
			})
			ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
				pos, dur := a.player.Position(), a.player.Duration()
				if dur == 0 && ok {
					dur = time.Duration(s.Interval) * time.Second
				}
				if a.seeking {
					pos = time.Duration(a.seekValue * float64(time.Second))
				}
				timeText(c, formatTime(pos))
				a.progress(c, pos, dur)
				timeText(c, formatTime(dur))
			})
		})
		// The rest.
		ui.Row(c).WidthPercent(28).MinWidth(200).Gap(2).Justify(ui.End).AlignItems(ui.Center).Children(func() {
			if a.playErr != "" {
				ui.Icon(c, icAlert).FontSize(16).TextColor(colDanger).Tooltip(a.playErr).Padding(0, 6)
			}
			q := qualityLabel(a.set.Quality, a.qualityKnown, a.askedQuality, a.gotQuality, ok && isXiaoyuzhou(s))
			tip := "音质"
			if n := a.player.Size(); n > 0 {
				tip = "音质 · 当前文件 " + formatSize(n)
			}
			if a.qualityOpen {
				tip = ""
			}
			qb := badge(c, q, colAccent).Cursor(ui.CursorPointer).MarginEnd(6).Label("音质")
			if tip != "" {
				qb.Tooltip(tip)
			}
			if qb.Hovered() || a.qualityOpen {
				qb.Background(colAccent.Alpha(0.12))
			}
			if qb.Clicked() {
				a.qualityOpen = !a.qualityOpen
			}
			// The qualities, pulled up from the badge, the one in use
			// checked, each with the size of the song's file.
			file, hasSong := qqmusic.File{}, false
			if s, ok := a.nowPlaying(); ok {
				file, hasSong = s.File, true
			}
			popupUp(c, &a.qualityPopup, qb, &a.qualityOpen, func(panel ui.Element) {
				panel.AttachTo(qb, ui.AnchorTop, ui.AnchorBottom).Margin(0, 0, 8, 0).
					Padding(4).Gap(1).Width(236).Radius(radiusLg).Background(colCard).Border(1, colLine).
					Shadow(0, 8, 24, 0, colShadow)
				for i, qq := range []qqmusic.Quality{qqmusic.Best, qqmusic.Lossless, qqmusic.High, qqmusic.Standard} {
					if i == 1 {
						ui.Box(c).Height(1).Margin(3, 2).Background(colLine)
					}
					on := a.set.Quality == qq
					row := ui.Row(c.Key(int(qq))).Padding(6, 8).Gap(8).Radius(radiusMd).AlignItems(ui.Center).Cursor(ui.CursorPointer)
					if row.Hovered() {
						row.Background(colHover)
					}
					fg := colText
					if on {
						fg = colAccent
					}
					size := "无"
					switch n := file.Size(qq); {
					case !hasSong:
						size = ""
					case n == 0:
					case qq == qqmusic.Best:
						size = file.Best().String() + " · " + formatSize(n)
					default:
						size = formatSize(n)
					}
					row.Children(func() {
						// The check keeps its column, so the names line up.
						ui.Box(c).Size(14, 14).Center().Children(func() {
							if on {
								ui.Icon(c, icCheck).FontSize(14).TextColor(colAccent)
							}
						})
						ui.Text(c, qq.String()).FontSize(13).TextColor(fg)
						ui.Text(c, size).FontSize(12).TextColor(colMuted).TextAlign(ui.End).Grow(1).FontFeatures("tnum")
					})
					if row.Clicked() {
						a.qualityOpen = false
						a.setQuality(qq)
					}
				}
			})
			col := colMuted
			if a.showQueue {
				col = colAccent
			}
			qbtn := iconButton(c, icQueue, 16, col, "播放队列")
			// Pressing the toggle is not pressing outside the queue: the
			// panel must stay open for the click to close it.
			a.queueBtnHover = qbtn.Hovered()
			if qbtn.Clicked() {
				a.toggleQueue()
			}
			vic := icVolume
			if a.muted || a.volume == 0 {
				vic = icVolumeX
			}
			if iconButton(c, vic, 16, colMuted, "静音").Clicked() {
				a.toggleMute()
			}
			v := a.volume
			if a.muted {
				v = 0
			}
			sl := a.thinSlider(c, &v, 0, 1, 0)
			sl.Width(64)
			if sl.Changed() {
				a.setVolume(v)
			}
			if a.volumeWasPressed && !sl.Pressed() {
				a.saveSettings()
			}
			a.volumeWasPressed = sl.Pressed()
		})
	})
}

// songCell is the song the bar shows: its cover, name and singers, which
// slide in from the way the change goes, as the song it replaces slides
// out the other way. The heart stays put, as it is the bar's rather than
// the song's. A Transition's Exit is the one of the last frame that built
// the element, so a copy leaving cannot take the way of the change that
// removed it. The track the cell is on moves instead, one songSlide
// against the way of each change, and the cell stands where the track has
// gone to: the copy of the song before stays where it stood on the track,
// and so leaves with it, whichever way it goes.
func (a *app) songCell(c *ui.Context, s qqmusic.Song) {
	x := songSlide * float32(a.songTrack)
	track := ui.Row(c).FillWidth().AnimatePose(ui.Pose{X: -x}, songSpring)
	track.Children(func() {
		cell := ui.Row(c.Key(keyOf(s))).Gap(12).AlignItems(ui.Center).FillWidth().Translate(x, 0)
		cell.Transition(ui.ElementTransition{
			Animation: &songSpring,
			Position:  true,
			Enter:     &ui.Motion{Opacity: 0.3},
			Exit:      &ui.Motion{Opacity: 0},
		})
		cell.Children(func() { a.songCellBody(c, s) })
	})
}

// songCellBody is the cover, name and singers of a song of songCell.
func (a *app) songCellBody(c *ui.Context, s qqmusic.Song) {
	cov := ui.Box(c).Cursor(ui.CursorPointer).Tooltip("歌词").Children(func() {
		a.cover(c, s.Cover(300), barCover, radiusMd)
	})
	if cov.Clicked() {
		a.toggleLyrics()
	}
	ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
		ui.Text(c, s.DisplayName()).FontSize(14).FontWeight(500).SingleLine().Cursor(ui.CursorPointer).OnClick(a.toggleLyrics)
		a.singerLinks(c, s.Singers, 12)
	})
}

var modeIcons = [...]*ui.SVG{icRepeat, icRepeat1, icShuffle, icOrder}

// modeButton shows the play mode, and above it, as it is clicked, a menu
// of the modes, each with its icon, the one in use checked.
func (a *app) modeButton(c *ui.Context) {
	col := colMuted
	if a.set.Mode != playqueue.Order {
		col = colAccent
	}
	tip := a.set.Mode.String()
	if a.modeOpen {
		tip = ""
	}
	b := iconButton(c, modeIcons[a.set.Mode], 16, col, tip).Label("播放模式")
	if a.modeOpen {
		b.Background(colGhost)
	}
	if b.Clicked() {
		a.modeOpen = !a.modeOpen
	}
	popupUp(c, &a.modePopup, b, &a.modeOpen, func(panel ui.Element) {
		panel.AttachTo(b, ui.AnchorTop, ui.AnchorBottom).Margin(0, 0, 8, 0).
			Padding(4).Gap(2).Width(136).Radius(radiusLg).Background(colCard).Border(1, colLine).
			Shadow(0, 8, 24, 0, colShadow)
		for _, m := range []playqueue.Mode{playqueue.Order, playqueue.Loop, playqueue.One, playqueue.Shuffle} {
			on := m == a.set.Mode
			row := ui.Row(c.Key(int(m))).Padding(7, 8).Gap(8).Radius(radiusMd).AlignItems(ui.Center).Cursor(ui.CursorPointer)
			if row.Hovered() {
				row.Background(colHover)
			}
			fg := colText
			if on {
				fg = colAccent
			}
			row.Children(func() {
				ui.Icon(c, modeIcons[m]).FontSize(16).TextColor(fg)
				ui.Text(c, m.String()).FontSize(13).TextColor(fg).Grow(1)
				if on {
					ui.Icon(c, icCheck).FontSize(14).TextColor(colAccent)
				}
			})
			if row.Clicked() {
				a.modeOpen = false
				if !on {
					a.set.Mode = m
					a.saveSettings()
				}
			}
		}
	})
}

func timeText(c *ui.Context, s string) {
	ui.Text(c, s).FontSize(12).TextColor(colMuted).FontFeatures("tnum").Width(40).TextAlign(ui.Center)
}

// thinSlider is shadcn's Slider made thin: a muted track, the range in
// the primary color, and under the pointer its thumb, white in a ring of
// the primary color. buffered shows the downloaded part.
func (a *app) thinSlider(c *ui.Context, v *float64, lo, hi, buffered float64) ui.Element {
	s := ui.SliderBase(c, v, lo, hi).Height(16).PaddingX(7).Cursor(ui.CursorPointer)
	hot := s.Hovered() || s.Pressed()
	at := float32(0)
	if hi > lo {
		at = float32((*v - lo) / (hi - lo))
	}
	at = min(max(at, 0), 1)
	s.Draw(func(p *ui.Painter, r ui.Rect) {
		x0, w := r.X+7, r.W-14
		y := r.Y + r.H/2 - 2
		p.Fill(ui.Rect{X: x0, Y: y, W: w, H: 4}, ui.RGBA(255, 255, 255, 0.12), 2)
		if buffered > 0 {
			p.Fill(ui.Rect{X: x0, Y: y, W: w * float32(min(buffered, 1)), H: 4}, ui.RGBA(255, 255, 255, 0.2), 2)
		}
		p.Fill(ui.Rect{X: x0, Y: y, W: w * at, H: 4}, colAccent, 2)
		if hot {
			knob := ui.Rect{X: x0 + w*at - 7, Y: r.Y + r.H/2 - 7, W: 14, H: 14}
			p.Shadow(knob, 7, 0, 1, 3, 0, colShadow)
			p.Fill(knob, colText, 7)
			p.Stroke(knob, colAccent, 7, 1.5)
		}
	})
	return s
}

func (a *app) progress(c *ui.Context, pos, dur time.Duration) {
	if !a.seeking {
		a.seekValue = pos.Seconds()
	}
	hi := max(dur.Seconds(), 1)
	s := a.thinSlider(c, &a.seekValue, 0, hi, a.player.Buffered())
	s.Grow(1).Label("播放进度")
	pressed := s.Pressed()
	switch {
	case pressed:
		a.seeking = true
	case a.seeking:
		a.seeking = false
		a.player.Seek(time.Duration(a.seekValue * float64(time.Second)))
	case s.Changed():
		// Arrows, or a click shorter than a frame.
		a.player.Seek(time.Duration(a.seekValue * float64(time.Second)))
	}
}

// nowPlaying is the page of the song playing: its cover and lyrics.
func (a *app) nowPlayingPage(c *ui.Context) {
	s, ok := a.nowPlaying()
	if !ok {
		ui.Column(c).Fill().Center().Gap(12).Children(func() {
			ui.Icon(c, icDisc).FontSize(64).TextColor(colDim)
			ui.Text(c, "还没有在播放的歌曲").TextColor(colMuted)
		})
		return
	}
	a.lyricsTint.aim(c, a.pageTint(s.Cover(300)))
	ui.Box(c).Fill().Clip().Draw(a.lyricsTint.fill).Children(func() {
		// A blurred cover behind it all, over its color.
		// The cover of the song before stays as the next one downloads.
		bmp := a.images.get(s.Cover(300), 300)
		if bmp != nil {
			a.lyricsBg = bmp
		} else {
			bmp = a.lyricsBg
		}
		if bmp != nil {
			ui.Image(c, bmp).Absolute().Top(-80).Left(-80).Right(-80).Bottom(-80).Fit(ui.Cover).Blur(60).Opacity(0.55)
		}
		ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).Gradient(ui.RGBA(0, 0, 0, 0.35), ui.RGBA(0, 0, 0, 0.75), 180)
		w, _ := c.Size()
		side := min(max((w-sidebarWidth-24)*0.34, 0), 340)
		// A panel on the right, the queue or the comments, takes the
		// lyrics' place, not the cover's: it stays as it is, where it is.
		panel := a.sidePanel()
		ui.Row(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).Padding(topHeight, 48, barRoom+24, 48).Gap(48).AlignItems(ui.Center).Children(func() {
			if side >= 180 {
				ui.Column(c).Width(side).Gap(14).Children(func() {
					ui.Box(c).Size(side, side).Radius(radiusXl).Clip().Shadow(0, 16, 48, 0, ui.RGBA(0, 0, 0, 0.5)).Children(func() {
						a.coverOr(c, s.Cover(500), s.Cover(300), side, radiusXl)
						ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).Radius(radiusXl).Border(1, colLine)
					})
					// The name and the singers are one block: the gap within
					// it is smaller than the one that sets it apart from the
					// cover and from what follows.
					ui.Column(c).Gap(6).Children(func() {
						// Selectable, so that the name can be copied as it is
						// shown, by selecting it or from the menu.
						ui.Text(c, s.DisplayName()).FontSize(26).FontWeight(600).LetterSpacing(-0.6).LineHeight(1.25).MaxLines(2).
							Selectable().ContextMenu(func(m *ui.Menu) {
							if m.Item("复制歌名").Chosen() {
								c.WriteClipboard(s.DisplayName())
							}
							m.Separator()
							m.EditItems()
						})
						a.singerLinks(c, s.Singers, 15)
					})
					// The album and the comments are a block of two rows of
					// the same shape, so that they line up.
					ui.Column(c).Gap(2).Children(func() {
						a.albumLine(c, s)
						a.commentsTab(c, s)
					})
				})
			}
			if !panel {
				a.lyricsView(c)
			}
		})
		// On the left: the window controls are on the right.
		if iconButton(c, icDown, 18, colMuted, "收起").Absolute().Top(12).Left(12).Clicked() {
			a.closeLyrics()
		}
	})
}

// albumLine is the album of the song playing, the page of it on a click.
// It has no frame of its own, only the ghost the comments below it have, so
// that the two sit as one block under the name.
func (a *app) albumLine(c *ui.Context, s qqmusic.Song) {
	if s.Album.Name == "" {
		return
	}
	label := "专辑"
	if a.isEpisode(s) {
		label = "节目"
	}
	row := rowUnderName(c)
	link := s.Album.Mid != ""
	fg := colMuted
	if link {
		row.Cursor(ui.CursorPointer)
		if row.Hovered() {
			row.Background(colGhost)
			fg = colText
		}
	}
	row.Children(func() {
		ui.Icon(c, icDisc).FontSize(14).TextColor(fg)
		ui.Text(c, label).FontSize(12).TextColor(fg).NoWrap()
		ui.Text(c, s.Album.Name).FontSize(13).TextColor(colText).SingleLine().MinWidth(0).Shrink(1)
	})
	if link && row.Clicked() {
		a.navigate(albumOf(s))
	}
}

// rowUnderName is a line of the block under the name of the song playing:
// the album's and the comments' are both this, so that they line up. Its
// padding is put outside its left edge, so that its text lines up with the
// name above it.
func rowUnderName(c *ui.Context) ui.Element {
	return ui.Row(c).Height(30).PaddingX(8).MarginX(-8).Gap(8).Radius(radiusMd).AlignItems(ui.Center)
}

// lyricPad is the empty rows above and below the lyrics, so that the
// first and last lines can scroll to the center.
const lyricPad = 6

// The type of a lyric line: the one the song is at is larger and bold, and
// the lines it has passed are dimmer to it than the ones still to come.
// These are the ends the transitions between lines move between.
const (
	lyricSize     = 19
	lyricSizeOn   = 24
	lyricWeight   = 400
	lyricWeightOn = 700
)

var (
	lyricPast = ui.RGBA(255, 255, 255, 0.45)
	lyricNext = ui.RGBA(255, 255, 255, 0.62)
)

func (a *app) lyricsView(c *ui.Context) {
	if s, ok := a.nowPlaying(); ok && isXiaoyuzhou(s) {
		// No line is built here: what the slide measured is over.
		a.lyricScroll.done()
		a.episodeNotesView(c, s)
		return
	}
	lines := a.lyrics
	if lines == nil {
		a.lyricScroll.done()
		lyricsSkeleton(c)
		return
	}
	cur := a.lyricLine
	if cur != a.lyricShown && cur >= 0 && time.Since(a.lyricHold) > 4*time.Second {
		// Where the line the song comes to shows before the list centers it:
		// the lines are drawn from there, and slide on to their place.
		a.lyricScroll.start(c, &a.lyricList, a.lyricShown, cur)
		a.lyricList.ScrollTo(cur+lyricPad, ui.Center)
	}
	a.lyricShown = cur
	timed := len(lines) > 0 && lines[0].At >= 0
	ui.List(c, &a.lyricList, len(lines)+2*lyricPad, func(i int) {
		i -= lyricPad
		if i < 0 || i >= len(lines) {
			ui.Box(c).Height(40)
			return
		}
		l := lines[i]
		on := i == cur
		row := ui.Column(c.Key(i)).Padding(10, 16).Gap(4).Radius(radiusLg)
		if i == cur+1 {
			a.lyricScroll.bind(i, row)
		}
		a.lyricScroll.glide(row)
		// The line the song reaches grows and brightens as it comes, and the
		// line it left shrinks and dims back, so that a change of line reads
		// as the song moving on rather than as a jump.
		size, weight, fg, tr := float32(lyricSize), float32(lyricWeight), lyricNext, lyricPast
		switch {
		case on:
			size, weight, fg, tr = lyricSizeOn, lyricWeightOn, colText, colText
		case i < cur:
			fg = lyricPast
		}
		size = row.AnimateTo("lyric-size", size, lyricGrow)
		weight = row.AnimateTo("lyric-weight", weight, lyricGrow)
		fg = row.AnimateColor("lyric-fg", fg, lyricFade)
		tr = row.AnimateColor("lyric-trans", tr, lyricFade)
		hover := timed && row.Hovered()
		wash := ui.Color{}
		if hover {
			wash = ui.RGBA(255, 255, 255, 0.06)
			row.Cursor(ui.CursorPointer)
		}
		row.Background(row.AnimateColor("lyric-wash", wash, lyricWash))
		// A press on the text seeks, so it cannot also drag a selection:
		// the copy is the row's menu.
		if l.Text != "" {
			row.ContextMenu(func(m *ui.Menu) {
				if m.Item("复制此句").Chosen() {
					c.WriteClipboard(lyricText(l))
				}
				if m.Item("复制全部歌词").Chosen() {
					c.WriteClipboard(lyricsText(lines))
				}
			})
		}
		row.Children(func() {
			ui.Text(c, l.Text).LineHeight(1.35).FontSize(size).FontWeight(int(weight + 0.5)).TextColor(fg)
			if l.Trans != "" {
				ui.Text(c, l.Trans).FontSize(14).TextColor(tr)
			}
			if hover {
				ui.Text(c, formatTime(l.At)).FontSize(11).TextColor(colAccent)
			}
		})
		if timed && row.Clicked() {
			a.player.Seek(l.At)
			a.lyricHold = time.Time{}
		}
	}).Grow(1).Height(0).FillHeight().HandleInput(func(ev ui.InputEvent) bool {
		if ev.Kind == ui.InputScroll {
			a.lyricHold = time.Now()
		}
		return false
	})
	a.lyricScroll.done()
}

// lyricText is a line as the page shows it: the translation under it, if
// there is one.
func lyricText(l qqmusic.LyricLine) string {
	if l.Trans == "" {
		return l.Text
	}
	return l.Text + "\n" + l.Trans
}

// lyricsText is all the lyrics as the page shows them, for the clipboard.
func lyricsText(lines []qqmusic.LyricLine) string {
	var b strings.Builder
	for _, l := range lines {
		if l.Text == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(lyricText(l))
	}
	return b.String()
}

// episodeNotesView is what a 小宇宙 episode shows in place of the lyrics:
// its notes, the description the podcast wrote for it.
func (a *app) episodeNotesView(c *ui.Context, s qqmusic.Song) {
	n := &a.notes
	if n.eid != s.Mid || (n.paragraphs == nil && !n.loaded && n.err == "" && !n.loading) {
		lyricsSkeleton(c)
		return
	}
	ui.Column(c).Grow(1).Height(0).FillHeight().Children(func() {
		ui.Row(c).Padding(0, 16, 10, 16).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "单集简介").FontSize(12).FontWeight(600).LetterSpacing(0.5).
				TextColor(ui.RGBA(255, 255, 255, 0.5))
		})
		switch {
		case n.loading && len(n.paragraphs) == 0:
			lyricsSkeleton(c)
		case n.err != "" && len(n.paragraphs) == 0:
			ui.Column(c).Grow(1).Height(0).FillHeight().Justify(ui.Center).Children(func() {
				errorRow(c, n.err, a.retryNotes)
			})
		case len(n.paragraphs) == 0:
			ui.Column(c).Grow(1).Height(0).FillHeight().Justify(ui.Center).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "播客单集暂无简介").FontSize(15).TextColor(ui.RGBA(255, 255, 255, 0.5))
			})
		default:
			ui.List(c, &n.scroll, len(n.paragraphs), func(i int) {
				ui.Box(c.Key(i)).Padding(8, 16).Children(func() {
					ui.Text(c, n.paragraphs[i]).FontSize(15).LineHeight(1.75).
						TextColor(ui.RGBA(255, 255, 255, 0.78)).Selectable()
				})
			}).Grow(1).Height(0).FillHeight()
		}
	})
}

func (a *app) queuePanel(c *ui.Context) {
	// A card on the right of the panel, below the title bar: the window
	// controls go on over its top, which moves the window.
	// Out of the layout of its room, which is narrower as it comes and goes.
	col := ui.Column(c.Key("queue")).Absolute().Top(0).Left(0).Bottom(0).Width(queueWidth)
	in, out := queueOut, queueOut
	if a.showLyrics {
		in, out = panelIn, panelOut
	}
	a.queueShow.animate(c, col, in, out, sideSpring)
	// A press anywhere else closes it, as a popover: the queue has no
	// focus of its own to lose. Nothing to do while it is already going.
	if a.showQueue && !a.queueShow.going && !a.queueBtnHover && col.PressedOutside() {
		a.showQueue = false
	}
	col.Children(func() {
		ui.Box(c).Height(topHeight).DragWindow()
		ui.Column(c).Grow(1).MinHeight(0).Margin(0, 8, barRoom, 0).Radius(radiusXl).Background(colCard).Border(1, colLine).Clip().Children(func() {
			ui.Row(c).Padding(12, 10, 12, 16).AlignItems(ui.Center).Gap(8).BorderWidth(0, 0, 1, 0).BorderColor(colLine).Children(func() {
				ui.Column(c).Grow(1).Gap(1).Children(func() {
					ui.Text(c, "播放队列").FontSize(14).FontWeight(600)
					ui.Text(c, fmt.Sprintf("%d 首歌曲", len(a.queue.Songs()))).FontSize(12).TextColor(colMuted)
				})
				if len(a.queue.Songs()) > 0 && iconButton(c, icTrash, 16, colMuted, "清空").Clicked() {
					a.clearQueue()
				}
				if iconButton(c, icClose, 16, colMuted, "关闭").Clicked() {
					a.showQueue = false
				}
			})
			if len(a.queue.Songs()) == 0 {
				ui.Column(c).Padding(32, 16).Gap(10).AlignItems(ui.Center).Children(func() {
					ui.Box(c).Size(40, 40).Radius(radiusLg).Background(colHover).Center().Children(func() {
						ui.Icon(c, icQueue).FontSize(20).TextColor(colText)
					})
					ui.Text(c, "队列是空的").FontSize(14).FontWeight(500)
					ui.Text(c, "去找些好听的歌吧").FontSize(13).TextColor(colMuted)
				})
				return
			}
			// The numbers are as wide as the longest, right aligned.
			numWidth := float32(7*len(fmt.Sprint(len(a.queue.Songs()))) + 2)
			ui.List(c, &a.queueList, len(a.queue.Songs()), func(i int) {
				s := a.queue.Songs()[i]
				on := i == a.queue.Cur()
				// Room above the first row and below the last, off the
				// card's edges.
				var top, bottom float32
				if i == 0 {
					top = 6
				}
				if i == len(a.queue.Songs())-1 {
					bottom = 6
				}
				row := ui.Row(c.Key(i)).Margin(top, 6, bottom, 6).Padding(6, 6, 6, 8).Gap(10).Radius(radiusMd).AlignItems(ui.Center)
				hover := row.Hovered()
				if hover || on {
					row.Background(colHover)
				}
				row.Children(func() {
					num := ui.Text(c, fmt.Sprint(i+1)).Width(numWidth).TextAlign(ui.End).FontSize(12).FontFeatures("tnum").TextColor(colDim)
					if on {
						num.TextColor(colAccent)
					}
					a.cover(c, s.Cover(150), 36, radiusSm)
					ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
						t := ui.Text(c, s.DisplayName()).FontSize(13).FontWeight(500).SingleLine()
						if on {
							t.TextColor(colAccent)
						}
						a.singerLinks(c, s.Singers, 12)
					})
					if hover {
						if iconButton(c, icClose, 14, colMuted, "移除").OnClick(func() { a.removeFromQueue(i) }).Valid() {
						}
					} else if on {
						a.equalizer(c)
					}
				})
				if row.DoubleClicked() {
					a.playAt(i)
				}
			}).Grow(1)
		})
	})
}
