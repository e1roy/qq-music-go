package app

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// Pages, the lyrics and the queue come in and go with poses, which move
// them in frames that only paint, without building the view: a large page
// moves as smoothly as a small one. Springs keep their velocity, so the
// lyrics closed on the way up turn back smoothly.

var (
	// A page rises a little as it fades in.
	pageOut    = ui.Pose{Y: 16, Fade: 1}
	pageSpring = ui.Animation{Spring: ui.SpringOptions{Duration: 420 * time.Millisecond}}

	// The lyrics come up from below, over the page, and go down again.
	lyricsOut    = ui.Pose{Y: 48, Fade: 1}
	lyricsSpring = ui.Animation{Spring: ui.SpringOptions{Duration: 460 * time.Millisecond}}

	// The queue fades as its room opens and closes, which slides it. Over
	// the lyrics, where the room opens at once, it slides by itself, as the
	// comments do. Both are quicker than pages.
	queueOut   = ui.Pose{Fade: 1}
	panelOut   = ui.Pose{X: 40, Fade: 1}
	sideSpring = ui.Animation{Spring: ui.SpringOptions{Duration: 300 * time.Millisecond}}
)

// appear poses el at out the first time it is built since *in was reset,
// and at its place after, which a goes to.
func appear(c *ui.Context, el ui.Element, in *bool, out ui.Pose, a ui.Animation) {
	pose := ui.Pose{}
	if !*in {
		*in, pose = true, out
		c.AnimationFrame()
	}
	el.AnimatePose(pose, a)
}

// presence keeps an element built while it goes, as Motion's
// AnimatePresence: it comes in from a pose, and goes back to it before it
// is no longer built.
type presence struct {
	built, in, going bool
	// rest is whether it came in and stays: what it covers needs no
	// building.
	rest bool
	at   time.Time // when it started coming or going
}

// shown reports whether to build the element, which is to show if show.
func (p *presence) shown(c *ui.Context, show bool) bool {
	switch {
	case show && !p.built:
		*p = presence{built: true, at: c.Now()}
	case show && p.going:
		p.going, p.at = false, c.Now()
	case !show && p.built && !p.going:
		p.going, p.rest, p.at = true, false, c.Now()
	}
	return p.built
}

// animate poses el, built as shown said: from out to its place as it
// comes, and back as it goes, until it is gone.
func (p *presence) animate(c *ui.Context, el ui.Element, out ui.Pose, a ui.Animation) {
	if p.going {
		el.AnimatePose(out, a)
	} else {
		appear(c, el, &p.in, out, a)
	}
	if p.rest {
		return
	}
	if left := a.Spring.Duration - c.Now().Sub(p.at); left > 0 || el.Transitioning() {
		c.After(max(left, 30*time.Millisecond))
		return
	}
	if p.going {
		p.built = false
	} else {
		p.rest = true
	}
	c.AnimationFrame()
}

// What a singer's tab shows rises a little as it fades in, row after row.
var (
	tabOut    = ui.Pose{Y: 14, Fade: 1}
	tabSpring = ui.Animation{Spring: ui.SpringOptions{Duration: 380 * time.Millisecond}}
)

// tabMotion brings in the rows of a list below its header, as a tab
// switched to or loaded again: the rows built as it starts come in, one
// after the other, and those scrolled to later are just there. Rows are
// built under keys of the tab and the epoch, for those that came in before to be
// new and come in again.
type tabMotion struct {
	epoch   int
	pending bool      // armed, to start at the next build
	at      time.Time // when it started
	in      map[int]bool
}

// arm makes the rows come in at the next build.
func (m *tabMotion) arm() {
	m.epoch++
	m.pending, m.in = true, map[int]bool{}
}

// row builds row i of the list of tab in a column, which comes in.
func (m *tabMotion) row(c *ui.Context, tab singerTab, i int, fn func()) {
	el := ui.Column(c.Key([2]int{int(tab), m.epoch})).Children(fn)
	if m.in == nil {
		return // never armed: the page came in itself
	}
	if m.pending {
		m.pending, m.at = false, c.Now()
	}
	if !m.in[i] {
		m.in[i] = true
		if c.Now().Sub(m.at) < 250*time.Millisecond {
			el.AnimatePose(tabOut, tabSpring)
			c.AnimationFrame()
			return
		}
	}
	a := tabSpring
	a.Delay = time.Duration(min(i, 8)) * 30 * time.Millisecond
	el.AnimatePose(ui.Pose{}, a)
}

// 换一批 replaces the cards of a section: the new ones come up as they
// fade in, one after the other, so the swap reads as a refresh.
var (
	cardOut    = ui.Pose{Y: 12, Fade: 1}
	cardSpring = ui.Animation{Spring: ui.SpringOptions{Duration: 380 * time.Millisecond}}
)

// refreshSpin is how long 换一批 turns its icon for a refresh whose data was
// already there: one turn, so that it ends where it started.
const refreshSpin = time.Second

// refreshMotion brings in the cards of a section again once a new batch
// replaces them, and turns the icon of 换一批 meanwhile.
type refreshMotion struct {
	in map[int]bool // cards already posed out for this batch
	at time.Time    // when they started coming in
}

// arm makes the cards come in at the next build.
func (m *refreshMotion) arm() {
	m.in, m.at = map[int]bool{}, time.Time{}
}

// busy reports whether the cards are still coming in, so that the icon
// turns for a refresh that had no request to wait for.
func (m *refreshMotion) busy(c *ui.Context) bool {
	if m.in == nil {
		return false
	}
	if m.at.IsZero() {
		m.at = c.Now()
	}
	return c.Now().Sub(m.at) < refreshSpin
}

// card poses card i of the batch, which comes in unless it was already
// there when armed.
func (m *refreshMotion) card(c *ui.Context, i int, card ui.Element) {
	if m.in == nil {
		return // never armed: the cards came in with the page
	}
	if !m.in[i] {
		m.in[i] = true
		card.AnimatePose(cardOut, cardSpring)
		c.AnimationFrame()
		return
	}
	a := cardSpring
	a.Delay = time.Duration(min(i, 8)) * 25 * time.Millisecond
	card.AnimatePose(ui.Pose{}, a)
}
