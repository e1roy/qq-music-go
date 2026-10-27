package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"qqmusic/internal/qqmusic"
)

// TestLyricScroll checks the slide that covers the jump a list makes to
// center another line: on the frame the list moves, the lines are drawn
// where the frame before left them, and they go on to their place from
// there.
func TestLyricScroll(t *testing.T) {
	const rows, rowHeight = 40, 40
	var list ui.ListState
	var slide lyricScroll
	cur, shown := 5, -1
	drawnY := float32(-1) // row 3, as the frame before drew it
	view := func(c *ui.Context) {
		if cur != shown {
			slide.start(c, &list, shown, cur)
			list.ScrollTo(cur, ui.Center)
		}
		shown = cur
		ui.List(c, &list, rows, func(i int) {
			row := ui.Column(c.Key(i)).Height(rowHeight)
			if i == cur+1 {
				slide.bind(i, row)
			}
			slide.glide(row)
			if i == 3 {
				drawnY = row.Bounds().Y
			}
			row.Children(func() { ui.Text(c, "line") })
		}).Grow(1).Height(0).FillHeight()
		slide.done()
	}
	tt := ui.NewTester(view, 400, 300)
	tt.Frame()
	tt.Frame()
	before := drawnY

	// The song comes to the line after the one it was at: the list centers
	// it, and the lines slide up a row from where they showed.
	cur++
	tt.Frame()
	if moved := drawnY - before; moved > 1 || moved < -1 {
		t.Errorf("the lines moved %v on the frame the list did, want they did not", moved)
	}
	for range 30 {
		tt.Advance(16 * time.Millisecond)
		tt.Frame()
	}
	if moved := before - drawnY; moved < rowHeight-5 || moved > rowHeight+16 {
		t.Errorf("the lines settled %v from where they were, want a row", moved)
	}
}

// TestLyricScrollMeasuresTheComingLine checks that the lyrics page measures
// the line after the one the song is at, as the list numbers the lines
// below the empty rows above them.
func TestLyricScrollMeasuresTheComingLine(t *testing.T) {
	a, _ := newOfflineApp(t, t.TempDir())
	setQueue(a, []qqmusic.Song{{ID: 100, Mid: "1", Name: "期待爱"}}, 0)
	a.lyrics, a.lyricsFor = []qqmusic.LyricLine{
		{At: 0, Text: "第一句"},
		{At: 3 * time.Second, Text: "第二句"},
		{At: 7 * time.Second, Text: "第三句"},
	}, "1"
	a.lyricLine = 0
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Size(600, 400).Children(func() { a.lyricsView(c) })
	}, 700, 500)
	tt.Frame()
	if a.lyricScroll.row != 1 {
		t.Fatalf("measuring line %d, want the one after the line the song is at", a.lyricScroll.row)
	}

	// The song comes to that line: the list centers it, and the slide is how
	// far the line showed from the middle before.
	a.lyricLine = 1
	tt.Frame()
	if a.lyricScroll.from == 0 {
		t.Error("the lines have nowhere to slide from")
	}
}
