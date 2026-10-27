package app

import (
	"context"

	"github.com/egoist/mygo/ui"

	"qqmusic/internal/xiaoyuzhou"
)

// pager is a list loaded a page at a time: what a search finds, or what
// 小宇宙 lists. pages is how many came; more, whether another follows;
// total, how many there are in all, for the lists whose service says.
// QQ Music numbers its pages, 小宇宙 gives after, a cursor to the next.
// A list that scrolls by itself, as a tab of the search page, keeps its
// place in list.
type pager[T any] struct {
	items   []T
	pages   int
	more    bool
	total   int
	after   xiaoyuzhou.Cursor
	loading bool
	err     string
	gen     int
	list    ui.ListState
}

// reset forgets what the list had, for another query or a login gone,
// dropping the page loading.
func (p *pager[T]) reset() {
	*p = pager[T]{gen: p.gen + 1, list: p.list}
	p.list.ScrollTo(0, ui.Start)
}

// loadPage loads the next page of p: load gets it, off the main thread,
// by its number or after the cursor, and returns how to add it to p, on
// the main thread, where an error is shown as fail has it. A page of a
// list reset meanwhile is dropped.
func loadPage[T any](a *app, p *pager[T], fail func(error) string, load func(ctx context.Context, page int, after xiaoyuzhou.Cursor) (add func(), err error)) {
	p.loading, p.err = true, ""
	p.gen++
	gen, page, after := p.gen, p.pages+1, p.after
	a.do(func(ctx context.Context) func() {
		add, err := load(ctx, page, after)
		return func() {
			if gen != p.gen {
				return
			}
			p.loading = false
			if err != nil {
				// fail may reset p, as xiaoyuzhouErr does as the login goes: the
				// error is then of what p no longer lists.
				if msg := fail(err); gen == p.gen {
					p.err = msg
				}
				return
			}
			p.pages = page
			add()
		}
	})
}
