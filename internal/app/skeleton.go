package app

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// Skeletons stand for what is loading, in its shape, as shadcn's
// Skeleton: bones of the accent color where the covers and the lines of
// text will be, which pulse together.

var (
	colBone      = ui.RGBA(255, 255, 255, 0.1)
	colBoneLight = ui.RGBA(255, 255, 255, 0.16) // over the lyrics' cover
)

// skeleton builds fn's bones in a column that pulses.
func skeleton(c *ui.Context, fn func()) ui.Element {
	col := ui.Column(c).Gap(16)
	col.Opacity(0.45 + 0.55*col.Loop("pulse", 1400*time.Millisecond, ui.Bounce(ui.EaseInOut)))
	return col.Children(fn)
}

func bone(c *ui.Context) ui.Element {
	return ui.Box(c).Background(colBone).Radius(radiusMd)
}

// bar is a bone for a line of text, of a percentage of its parent's width.
func bar(c *ui.Context, h, pct float32) ui.Element {
	return bone(c).Height(h).WidthPercent(pct).Radius(h / 2)
}

func headingSkeleton(c *ui.Context) {
	bone(c).Size(140, 20).MarginY(6)
}

// songCardsSkeleton stands for songCards.
func (a *app) songCardsSkeleton(c *ui.Context, rows int) {
	cols := min(a.columns(c, 300), 4)
	ui.Grid(c).Columns(cols).Gap(8).Children(func() {
		for range rows * cols {
			ui.Row(c).Padding(6).Gap(10).Radius(radiusLg).Border(1, colLine).AlignItems(ui.Center).Children(func() {
				bone(c).Size(44, 44).Radius(radiusSm)
				ui.Column(c).Grow(1).MinWidth(0).Gap(8).Children(func() {
					bar(c, 12, 60)
					bar(c, 10, 35)
				})
			})
		}
	})
}

// forYouSkeleton stands for forYou.
func forYouSkeleton(c *ui.Context) {
	ui.Grid(c).Columns(2).Gap(12).Children(func() {
		for range 2 {
			ui.Row(c).Padding(14).Gap(16).Radius(radiusXl).Border(1, colLine).AlignItems(ui.Center).MinWidth(0).Children(func() {
				bone(c).Size(112, 112).Radius(radiusLg)
				ui.Column(c).Grow(1).MinWidth(0).Gap(10).Children(func() {
					bar(c, 10, 30)
					bar(c, 18, 50)
					bar(c, 12, 80)
				})
			})
		}
	})
}

// playlistSkeleton stands for rows of playlistCard, cols in a row.
func playlistSkeleton(c *ui.Context, cols, rows int) {
	ui.Grid(c).Columns(cols).Gap(12).Children(func() {
		for range rows * cols {
			ui.Column(c).Padding(8).Gap(10).MinWidth(0).Children(func() {
				bone(c).AspectRatio(1).FillWidth().Radius(radiusLg)
				bar(c, 12, 85)
				bar(c, 10, 50)
			})
		}
	})
}

// singerSkeleton stands for rows of singerCard, cols in a row.
func singerSkeleton(c *ui.Context, cols, rows int) {
	ui.Grid(c).Columns(cols).Gap(12).Children(func() {
		for range rows * cols {
			ui.Column(c).Padding(12, 8).Gap(10).AlignItems(ui.Center).MinWidth(0).Children(func() {
				bone(c).Size(singerSide, singerSide).Radius(singerSide / 2)
				bar(c, 12, 50)
				bar(c, 10, 70)
			})
		}
	})
}

// topChartsSkeleton stands for topCharts.
func (a *app) topChartsSkeleton(c *ui.Context) {
	cols := min(a.columns(c, 320), 3)
	ui.Grid(c).Columns(cols).Gap(12).Children(func() {
		for range cols {
			ui.Row(c).Padding(12).Gap(14).Radius(radiusXl).Background(colCard).Border(1, colLine).AlignItems(ui.Center).MinWidth(0).Children(func() {
				bone(c).Size(96, 96).Radius(radiusLg)
				ui.Column(c).Grow(1).MinWidth(0).Gap(10).Children(func() {
					bar(c, 16, 55)
					for _, w := range []float32{90, 75, 82} {
						bar(c, 10, w)
					}
				})
			})
		}
	})
}

// chipsSkeleton stands for a row of chips.
func chipsSkeleton(c *ui.Context) {
	ui.Row(c).Wrap().Gap(8).Children(func() {
		for _, w := range []float32{88, 64, 112, 76, 96, 58, 120, 70, 84, 104, 62, 92} {
			bone(c).Size(w, 28)
		}
	})
}

// songRowsSkeleton stands for songRow, below columnTitles.
func songRowsSkeleton(c *ui.Context, n int) {
	for i := range n {
		ui.Row(c).Margin(0, 24).Padding(6, 16).Gap(16).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(colLine).Children(func() {
			ui.Box(c).Width(32).AlignItems(ui.End).Children(func() { bone(c).Size(14, 12).Radius(3) })
			ui.Row(c).Grow(3).Basis(0).MinWidth(0).Gap(12).AlignItems(ui.Center).Children(func() {
				bone(c).Size(36, 36).Radius(radiusSm)
				bar(c, 12, float32(40+i*17%35))
			})
			ui.Box(c).Grow(2).Basis(0).MinWidth(0).Children(func() { bar(c, 10, float32(35+i*11%30)) })
			ui.Box(c).Grow(2).Basis(0).MinWidth(0).Children(func() { bar(c, 10, float32(45+i*13%35)) })
			ui.Box(c).Width(32)
			ui.Box(c).Width(48).AlignItems(ui.End).Children(func() { bone(c).Size(34, 10).Radius(5) })
		})
	}
}

// listSkeleton stands for a list page: its header and its first songs.
func (a *app) listSkeleton(c *ui.Context, p page) {
	ui.Column(c).Grow(1).Clip().Children(func() {
		skeleton(c, func() {
			ui.Row(c).Gap(20).Padding(12, 24, 16, 24).AlignItems(ui.End).Children(func() {
				r := float32(radiusXl)
				if p.kind == pageSinger {
					r = 84
				}
				bone(c).Size(168, 168).Radius(r)
				ui.Column(c).Grow(1).MinWidth(0).Gap(12).Children(func() {
					bone(c).Size(44, 20).Radius(radiusSm)
					bone(c).Height(34).WidthPercent(55)
					bar(c, 12, 40)
					bar(c, 12, 30)
				})
			})
			ui.Row(c).PaddingX(24).Margin(0, 0, 16, 0).Children(func() {
				bone(c).Size(112, 32)
			})
		}).Gap(0)
		a.columnTitles(c)
		skeleton(c, func() { songRowsSkeleton(c, 12) }).Gap(0)
	})
}

// lyricsSkeleton stands for the lyrics, over the blurred cover.
func lyricsSkeleton(c *ui.Context) {
	ui.Column(c).Grow(1).Height(0).FillHeight().Justify(ui.Center).Children(func() {
		skeleton(c, func() {
			for i, w := range []float32{55, 70, 45, 80, 60, 50, 72, 40} {
				h := float32(18)
				if i == 3 {
					h = 24
				}
				ui.Box(c).Padding(10, 16).Children(func() {
					ui.Box(c).Height(h).WidthPercent(w).Radius(h / 2).Background(colBoneLight)
				})
			}
		}).Gap(4)
	})
}

// commentsSkeleton stands for the comments.
func commentsSkeleton(c *ui.Context) {
	skeleton(c, func() {
		for range 6 {
			ui.Row(c).Padding(12, 16).Gap(10).Children(func() {
				bone(c).Size(32, 32).Radius(16)
				ui.Column(c).Grow(1).MinWidth(0).Gap(8).Children(func() {
					bar(c, 10, 30)
					bar(c, 12, 90)
					bar(c, 12, 60)
				})
			})
		}
	}).Gap(4)
}
