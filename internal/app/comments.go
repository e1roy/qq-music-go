package app

import (
	"context"

	"github.com/egoist/mygo/ui"

	"qqmusic/internal/qqmusic"
)

// comments are those of the song playing, in the panel beside its lyrics:
// the hot and the latest, each loaded a page at a time.
type comments struct {
	open   bool
	song   int64 // whose they are
	order  qqmusic.CommentOrder
	lists  [2]commentList // by order
	scroll [2]ui.ListState
	total  int // of the song, 0 until known
	gen    int
}

type commentList struct {
	items           []qqmusic.Comment
	page            int // the next to load
	more            bool
	loaded, loading bool
	err             string
}

const commentPage = 20

// commentsShown is whether the comments are beside the lyrics: they go
// with them, and come back with them.
func (a *app) commentsShown() bool { return a.showLyrics && a.comments.open }

// sidePanel is whether a panel, the queue or the comments, takes the
// right of the page.
func (a *app) sidePanel() bool { return a.showQueue || a.commentsShown() }

// toggleLyrics shows or hides the page of the song playing. Its comments
// load with it, for their number to show on it.
func (a *app) toggleLyrics() {
	a.showLyrics = !a.showLyrics
	if a.showLyrics {
		a.syncComments()
	}
}

// toggleQueue shows or hides the queue, in place of the comments.
func (a *app) toggleQueue() {
	a.showQueue = !a.showQueue
	if a.showQueue {
		a.comments.open = false
	}
}

// toggleComments shows or hides the comments, in place of the queue.
func (a *app) toggleComments() {
	a.comments.open = !a.comments.open
	if a.comments.open {
		a.showQueue = false
		a.syncComments()
	}
}

func (a *app) showCommentOrder(o qqmusic.CommentOrder) {
	a.comments.order = o
	a.syncComments()
}

// syncComments makes the comments those of the song playing, and loads
// the first page of the order shown.
func (a *app) syncComments() {
	s, ok := a.nowPlaying()
	if !ok || s.ID == 0 {
		return
	}
	cm := &a.comments
	if id := int64(s.ID); cm.song != id {
		cm.song, cm.total, cm.lists = id, 0, [2]commentList{}
		cm.gen++
		for i := range cm.scroll {
			cm.scroll[i].ScrollTo(0, ui.Start)
		}
	}
	if l := &cm.lists[cm.order]; !l.loaded && !l.loading {
		a.loadComments()
	}
}

// loadComments loads the next page of the order shown.
func (a *app) loadComments() {
	cm := &a.comments
	order, l := cm.order, &cm.lists[cm.order]
	if l.loading {
		return
	}
	l.loading, l.err = true, ""
	gen, song, page, after := cm.gen, cm.song, l.page, ""
	if n := len(l.items); n > 0 {
		after = l.items[n-1].SeqNo
	}
	a.do(func(ctx context.Context) func() {
		items, more, total, err := a.api.Comments(ctx, song, order, page, commentPage, after)
		return func() {
			if gen != cm.gen {
				return
			}
			l := &cm.lists[order]
			l.loading = false
			if err != nil {
				l.err = errText(err)
				return
			}
			// Pages overlap as comments come and go: each once, for rows
			// to keep their keys.
			seen := make(map[string]bool, len(l.items))
			for _, it := range l.items {
				seen[it.ID] = true
			}
			for _, it := range items {
				if !seen[it.ID] {
					seen[it.ID] = true
					l.items = append(l.items, it)
				}
			}
			l.page++
			l.loaded, l.more = true, more && len(items) > 0
			if total > 0 {
				cm.total = total
			}
		}
	})
}
