package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"qqmusic/internal/qqmusic"
)

// commentsTab opens the comments beside the lyrics; on it, how many there
// are, once known.
func (a *app) commentsTab(c *ui.Context, s qqmusic.Song) {
	cm := &a.comments
	label := "评论"
	if cm.song == int64(s.ID) && cm.total > 0 {
		label += " " + formatCount(int64(cm.total))
	}
	// Over the blurred cover: white, a little.
	b := ui.Row(c).Height(30).PaddingX(12).Gap(6).Radius(15).AlignItems(ui.Center).Cursor(ui.CursorPointer).
		Background(ui.RGBA(255, 255, 255, 0.06)).Border(1, ui.RGBA(255, 255, 255, 0.16))
	fg := colText
	switch {
	case cm.open:
		b.Background(colAccent.Alpha(0.16)).Border(1, colAccent.Alpha(0.5))
		fg = colAccent
	case b.Hovered():
		b.Background(ui.RGBA(255, 255, 255, 0.12))
	}
	b.Children(func() {
		ui.Icon(c, icComment).FontSize(14).TextColor(fg)
		ui.Text(c, label).FontSize(13).FontWeight(500).TextColor(fg).NoWrap()
	})
	if b.Clicked() {
		a.toggleComments()
	}
}

// commentsPanel is the comments of the song playing, beside its lyrics,
// where the queue is.
func (a *app) commentsPanel(c *ui.Context) {
	cm := &a.comments
	s, ok := a.nowPlaying()
	mine := ok && s.ID != 0 && cm.song == int64(s.ID)
	sub := "暂无评论"
	if mine && cm.total > 0 {
		sub = formatCount(int64(cm.total)) + " 条评论"
	}
	// A card as the queue's, in the same room. Over the lyrics, it slides
	// by itself.
	col := ui.Column(c.Key("comments")).Absolute().Top(0).Left(0).Bottom(0).Width(queueWidth)
	a.commentsShow.animate(c, col, panelOut, sideSpring)
	col.Children(func() {
		ui.Box(c).Height(topHeight).DragWindow()
		ui.Column(c).Grow(1).MinHeight(0).Margin(0, 8, 8, 0).Radius(radiusXl).Background(colCard).Border(1, colLine).Clip().Children(func() {
			a.commentsCard(c, sub, mine)
		})
	})
}

// commentsCard is what the comments panel holds: its header, the orders
// and the comments of the one chosen.
func (a *app) commentsCard(c *ui.Context, sub string, mine bool) {
	cm := &a.comments
	ui.Row(c).Padding(12, 10, 12, 16).AlignItems(ui.Center).Gap(8).BorderWidth(0, 0, 1, 0).BorderColor(colLine).Children(func() {
		ui.Column(c).Grow(1).Gap(1).Children(func() {
			ui.Text(c, "评论").FontSize(14).FontWeight(600)
			ui.Text(c, sub).FontSize(12).TextColor(colMuted)
		})
		if iconButton(c, icClose, 16, colMuted, "关闭").Clicked() {
			cm.open = false
		}
	})
	ui.Box(c).Padding(10, 12, 4, 12).Children(func() {
		tabs(c, int(cm.order), true, func(i int) { a.showCommentOrder(qqmusic.CommentOrder(i)) }, "热门", "最新")
	})
	l := &cm.lists[cm.order]
	switch {
	case !mine:
		emptyComments(c)
	case l.err != "" && len(l.items) == 0:
		errorRow(c, l.err, a.loadComments)
	case !l.loaded:
		commentsSkeleton(c)
	case len(l.items) == 0:
		emptyComments(c)
	default:
		items := l.items
		// Each order scrolls on its own.
		ui.List(c.Key(int(cm.order)), &cm.scroll[cm.order], len(items)+1, func(i int) {
			if i == len(items) {
				a.commentsEnd(c, l)
				return
			}
			a.commentRow(c, items[i])
		}).Grow(1)
	}
}

func emptyComments(c *ui.Context) {
	ui.Column(c).Padding(32, 16).Gap(10).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Size(40, 40).Radius(radiusLg).Background(colHover).Center().Children(func() {
			ui.Icon(c, icComment).FontSize(20).TextColor(colText)
		})
		ui.Text(c, "还没有评论").FontSize(14).FontWeight(500)
	})
}

// commentsEnd is the row after the comments: as it shows, the next page
// loads.
func (a *app) commentsEnd(c *ui.Context, l *commentList) {
	end := ui.Column(c.Key("end")).Padding(16).AlignItems(ui.Center)
	end.Children(func() {
		switch {
		case l.err != "":
			errorRow(c, l.err, a.loadComments)
		case l.more:
			if !l.loading {
				a.loadComments()
			}
			ui.Spinner(c).Size(18, 18)
		default:
			ui.Text(c, "没有更多评论了").FontSize(12).TextColor(colDim)
		}
	})
}

// commentRow is a comment: who wrote it, what, the first reply to it, and
// when, where from, its likes and replies.
func (a *app) commentRow(c *ui.Context, cm qqmusic.Comment) {
	ui.Row(c.Key(cm.ID)).Padding(12, 16).Gap(10).AlignItems(ui.Start).BorderWidth(0, 0, 1, 0).BorderColor(colLine).Children(func() {
		if bmp := a.images.get(cm.Avatar, 64); bmp != nil {
			ui.Image(c, bmp).Size(32, 32).Radius(16).Fit(ui.Cover)
		} else {
			ui.Box(c).Size(32, 32).Radius(16).Background(colHover).Center().Children(func() {
				ui.Icon(c, icUser).FontSize(16).TextColor(colMuted)
			})
		}
		ui.Column(c).Grow(1).MinWidth(0).Gap(6).Children(func() {
			ui.Text(c, cm.Nick).FontSize(12).FontWeight(500).TextColor(colMuted).SingleLine()
			if cm.Content != "" {
				ui.Text(c, cm.Content).FontSize(13).LineHeight(1.5).Selectable()
			}
			if r := cm.Reply; r != nil && r.Content != "" {
				ui.Box(c).Padding(6, 8).Radius(radiusMd).Background(colHover).Children(func() {
					ui.Text(c, r.Nick+"："+r.Content).FontSize(12).LineHeight(1.45).TextColor(colMuted).MaxLines(3)
				})
			}
			ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
				var meta []string
				if ago := formatAgo(cm.Time); ago != "" {
					meta = append(meta, ago)
				}
				if cm.Location != "" {
					meta = append(meta, cm.Location)
				}
				ui.Text(c, strings.Join(meta, " · ")).FontSize(11).TextColor(colDim).SingleLine().Grow(1)
				commentStat(c, icThumb, cm.Likes)
				commentStat(c, icComment, int64(cm.Replies))
			})
		})
	})
}

func commentStat(c *ui.Context, ic *ui.SVG, n int64) {
	ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
		ui.Icon(c, ic).FontSize(12).TextColor(colDim)
		ui.Text(c, formatCount(n)).FontSize(11).TextColor(colDim).FontFeatures("tnum")
	})
}

// formatAgo is how long ago t was, as comments say it.
func formatAgo(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	switch d := time.Since(t); {
	case d < time.Minute:
		return "刚刚"
	case d < time.Hour:
		return fmt.Sprintf("%d分钟前", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%d小时前", int(d.Hours()))
	case t.Year() == time.Now().Year():
		return t.Format("01-02")
	default:
		return t.Format("2006-01-02")
	}
}
