package app

import (
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/shader"

	"qqmusic/internal/qqmusic"
)

// Pages, the lyrics and the queue come in and go, in frames that only paint,
// without building the view: a large page moves as smoothly as a small one.
// Springs keep their velocity, so the lyrics closed on the way up turn back
// smoothly. The lyrics and the panels are posed; a page is a Transition, and
// comes on its own: the one it replaces is gone at once.

var (
	// A page rises a little as it comes in. It is a Transition, not a pose:
	// a click that changes the page is handled halfway through a frame,
	// which builds the view again, and the last pose of a frame is the one
	// kept, which would show the page at its place at once. A Transition is
	// laid out once the frame is built, how many passes it took.
	//
	// Nothing of the page before is drawn under it, as an Exit would: two
	// pages of cards and headings, which rarely line up, read as one over
	// the other for as long as both show. A page changes at once, and comes
	// in on its own.
	pageTransition = ui.ElementTransition{
		// Only the position animates: the page itself is laid out by every
		// frame, its width following the room the queue takes.
		Position: true,
		// Opacity 1, not the default 0: it comes in without fading, for a
		// page of a list brings its rows in one after the other (rowsMotion),
		// which a fade would hide, showing them all at once as it cleared.
		Enter:     &ui.Motion{Y: 10, Opacity: 1},
		Animation: &ui.Animation{Spring: ui.SpringOptions{Duration: 210 * time.Millisecond}},
	}

	// The lyrics come out of the cover in the player bar as macOS's genie
	// effect brings a window out of the Dock (genieShader), and go back into
	// it; the bar's card fades meanwhile, for the bar to sit on them as part
	// of the page.
	lyricsSpring = ui.Animation{Spring: ui.SpringOptions{Duration: 560 * time.Millisecond}}

	// The queue fades as its room opens and closes, which slides it. Over
	// the lyrics, where the room opens at once, a panel slides across the
	// room by itself instead: out of its far edge, the way the room opens
	// elsewhere. It comes in opaque, for the fade would leave only its end
	// to be seen, and goes faded, so that it is not seen to be cut at the
	// edge it leaves by. Both are quicker than pages.
	queueOut   = ui.Pose{Fade: 1}
	panelIn    = ui.Pose{X: queueWidth}
	panelOut   = ui.Pose{X: queueWidth, Fade: 1}
	sideSpring = ui.Animation{Spring: ui.SpringOptions{Duration: 300 * time.Millisecond}}

	// The raised box of a tab crosses the track from the label chosen before
	// to the one chosen now.
	pillSpring = ui.Animation{Spring: ui.SpringOptions{Duration: 260 * time.Millisecond}}

	// The menu below the search box drops in from just above the anchor as
	// it fades in, from its top-left corner, and goes back the same way:
	// quicker than a page, as a menu is.
	menuIn     = ui.Pose{Y: -6, Scale: 0.96}
	menuOut    = ui.Pose{Y: -6, Scale: 0.96, Fade: 1}
	menuMotion = ui.Animation{Spring: ui.SpringOptions{Duration: 160 * time.Millisecond}}

	// The menus raised above their anchors — the qualities, the play modes,
	// the account — rise out of the anchor as they fade in, and sink back
	// into it as they go, as the search box's drops in from above: as far
	// as popupRise, in as long as popupMotion.
	popupRise   = float32(6)
	popupMotion = 140 * time.Millisecond

	// The song the bar shows slides in the way the change goes — from the
	// right for the next one, from the left for the one before — as the
	// song it replaces fades out the other way. They travel far enough to
	// be seen as a slide over the fade, and bounce a little so that they
	// are.
	songSlide  = float32(96)
	songSpring = ui.Animation{Spring: ui.SpringOptions{Duration: 300 * time.Millisecond, Bounce: 0.1}}

	// The lyrics move from line to line as the song does: the one it
	// reaches grows and brightens, and the one it leaves settles back. The
	// color goes a little quicker than the size, so that the line is lit
	// before it has finished growing, as in the players it is taken from.
	// The wash under the pointer comes and goes quicker still, as a hover.
	lyricGrow = ui.Animation{Spring: ui.SpringOptions{Duration: 320 * time.Millisecond}}
	lyricFade = ui.Animation{Spring: ui.SpringOptions{Duration: 240 * time.Millisecond}}
	lyricWash = ui.Animation{Spring: ui.SpringOptions{Duration: 150 * time.Millisecond}}

	// The lines slide up to the one the song is at, rather than the list
	// jumping there: the same time as they take to grow and light up, so
	// that the whole move reads as one.
	lyricSlide = ui.Animation{Spring: ui.SpringOptions{Duration: 320 * time.Millisecond}}
)

// What a tab of a page shows comes in as another does, but only fading: the
// tabs above it stay put and the page is already there, so nothing moves
// under the pointer, which a slide would.
var paneTransition = ui.ElementTransition{
	Position:  true,
	Enter:     &ui.Motion{Opacity: 0},
	Animation: &ui.Animation{Duration: 160 * time.Millisecond},
}

// pane builds the content of a tab as an element of its own, keyed by the tab
// shown: another tab brings it in, while the tabs above it stay put. gap is
// that of the column the content of the page is in.
func pane(c *ui.Context, key any, gap float32, fn func()) ui.Element {
	el := ui.Column(c.Key(key)).Gap(gap)
	el.Transition(paneTransition)
	return el.Children(fn)
}

// genieShader draws the lyrics on their way into the cover in the player
// bar, or out of it, as macOS's genie effect: first the sides bend in, down
// to the cover's width at the bottom, then what they hold slides down
// through the neck into the cover. Progress is 0 open and 1 gone; Target is
// the cover's left, its top from the bottom of the box, its right, and its
// bottom from the bottom of the box, in DIPs.
var genieShader = shader.MustCompile(`package main

var Progress float
var Target vec4

func LayerEffect(pos vec2) vec4 {
	size := rectSize()
	bend := smoothstep(0.0, 0.45, Progress)
	slide := smoothstep(0.3, 1.0, Progress)
	bottom := mix(size.y, size.y-Target.w, bend)
	top := slide * (bottom - (Target.y - Target.w))
	f := bend * smoothstep(0.0, bottom, pos.y)
	left := mix(0.0, Target.x, f)
	right := mix(size.x, Target.z, f)
	// Soft by a DIP along the edges, for the curves not to be jagged.
	cover := clamp(min(pos.x-left, right-pos.x)+0.5, 0.0, 1.0) * clamp(min(pos.y-top, bottom-pos.y)+0.5, 0.0, 1.0)
	if cover <= 0.0 {
		return vec4(0)
	}
	u := (pos.x - left) / max(right-left, 0.001) * size.x
	v := (pos.y - top) / max(bottom-top, 0.001) * size.y
	return layerSample(vec2(u, v)) * cover
}
`)

// genieCover is where the cover in the player bar is, as genieShader's
// Target, from the panel's left and bottom: the bar's inset and padding,
// and the cover centered in the bar.
var genieCover = [4]float32{
	barInset + barPadX,
	barInset + (barHeight+barCover)/2,
	barInset + barPadX + barCover,
	barInset + (barHeight-barCover)/2,
}

// The shader's values and the variables they are set by, looked up once: the
// lyrics set them in every frame of the genie, with no map to build and
// nothing to convert. The UI is one thread, so they are safe to reuse.
var (
	genieValues   = genieShader.NewValues()
	genieProgress = genieShader.Uniform("Progress")
	genieTarget   = genieShader.Uniform("Target")
)

// lyricScroll slides the lines of the lyrics over the jump the list makes to
// center the line the song is at. A List moves to a row at once — ListState
// has no animated scroll to give — so the lines are drawn where the frame
// before left them and go on to their place from there, rather than jumping.
//
// How far they slide is the coming line's own: it is measured before the
// list moves, by where it shows from the middle of the lyrics. It is the
// line after the one the song is at that is measured, as that is the one the
// song comes to; a jump over lines, as a seek makes, snaps instead.
type lyricScroll struct {
	next  ui.Handle // the line after the one the song is at
	row   int       // the line next was bound to in the frame before, -1 for none
	found int       // the line next was bound to in this build, plus one, 0 for none
	from  float32   // how far the lines show from their place, from the jump
	armed bool      // the lines are drawn from there in this build
}

// start arms the slide with where the coming line shows, before the list
// moves to center it.
func (s *lyricScroll) start(c *ui.Context, list *ui.ListState, was, cur int) {
	s.armed = false
	if was < 0 || cur != was+1 || s.row != cur {
		return
	}
	// Where it showed, and the middle of the lyrics, in the same frame.
	line, view := s.next.Bounds(c), list.Bounds(c)
	if line.H <= 0 || view.H <= 0 {
		return
	}
	s.from, s.armed = line.Y+line.H/2-view.Y-view.H/2, true
}

// bind keeps row, the line after the one the song is at, as the one to
// measure where the list moves.
func (s *lyricScroll) bind(i int, row ui.Element) {
	row.Bind(&s.next)
	s.found = i + 1 // zero for none, so that a row 0 counts
}

// glide draws row where the list left it, and takes it on to its place.
func (s *lyricScroll) glide(row ui.Element) {
	y := row.AnimateTo("lyric-slide", 0, lyricSlide)
	if s.armed {
		y = row.Jump("lyric-slide", y+s.from)
	}
	if y != 0 {
		row.Translate(0, y)
	}
}

// done keeps what this build measured, and forgets the slide.
func (s *lyricScroll) done() {
	s.row, s.armed, s.found = s.found-1, false, 0
}

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

// animate poses el, built as shown said: from in to its place as it comes,
// and back to out as it goes, until it is gone. They are two poses, for an
// element that comes from beyond the window, which need not fade, and goes
// over what it covered.
func (p *presence) animate(c *ui.Context, el ui.Element, in, out ui.Pose, a ui.Animation) {
	if p.going {
		el.AnimatePose(out, a)
	} else {
		appear(c, el, &p.in, in, a)
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

// popupUp shows the panel of a popup while *open, raised above anchor: fn
// styles the panel and builds its content, as PopoverBase's fn does. The
// panel rises out of the anchor as it comes, and sinks back into it as it
// goes, which p times: it stays built while it goes, so that it can leave
// as it came. A press outside the panel and the anchor, or Escape, sets
// *open to false.
func popupUp(c *ui.Context, p *presence, anchor ui.Element, open *bool, fn func(panel ui.Element)) {
	if !p.shown(c, *open) {
		return
	}
	// Where it is on the way, 0 at its place and 1 where it starts and
	// ends. It is placed and faded as the panel is built, rather than with
	// AnimatePose, whose frames the frame keeps for no overlay's panel.
	far := float32(0)
	after := time.Duration(0)
	if p.going || !p.rest {
		x := min(max(float32(c.Now().Sub(p.at)), 0), float32(popupMotion)) / float32(popupMotion)
		// It comes at once and settles in, and goes the way it came.
		ease := 1 - (1-x)*(1-x)*(1-x)
		if p.going {
			far = ease
		} else {
			far = 1 - ease
		}
		if x < 1 {
			after = 16 * time.Millisecond
		}
	}
	// The panel is open while it is built, which leaves *open as it is: a
	// press outside it closes the panel and reports it, and the frames
	// after play it out.
	shown := true
	panel := ui.PopoverBase(c, anchor, &shown, func(panel ui.Element) {
		panel.Translate(0, popupRise*far).Opacity(1 - far)
		fn(panel)
	})
	if panel.Dismissed() {
		*open = false
	}
	if after > 0 {
		c.After(after)
		return
	}
	if p.rest {
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
	tabSpring = ui.Animation{Spring: ui.SpringOptions{Duration: 300 * time.Millisecond}}
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
	a.Delay = time.Duration(min(i, 8)) * 8 * time.Millisecond
	el.AnimatePose(ui.Pose{}, a)
}

// The rows of a song list rise a little as they fade in, one after the
// other, as a singer's tab does: a list shows its top rows first, and the
// page they are in comes in meanwhile, which alone would bring them all in
// at once.
var (
	// A little of the way there already: the rows brighten into place rather
	// than appearing out of nothing, which reads as one wave of the page
	// coming in, not a row after another as a machine would.
	rowsOut    = ui.Pose{Y: 4, Fade: 0.8}
	rowsSpring = ui.Animation{Spring: ui.SpringOptions{Duration: 170 * time.Millisecond, Bounce: 0.1}}
)

// rowsMotion brings in the rows of a song list, as the page or the query
// that shows them is another: those built as it starts come in, one after
// the other, and those scrolled to later are just there.
type rowsMotion struct {
	key any          // the page or the query shown; pages and strings compare
	at  time.Time    // when it started
	in  map[int]bool // rows built
	// A row went as the list loaded again (取消喜欢): the rows from slideFrom
	// on come from slide further down, and slide up into their places. A List
	// lays out its rows itself and moves none of them, so they are posed.
	slideFrom int
	slide     float32
	slid      map[int]bool
}

// show takes the page or the query of the list built: another one brings
// its rows in again.
func (m *rowsMotion) show(key any) {
	if m.in != nil && m.key == key {
		return
	}
	m.key, m.at, m.in = key, time.Time{}, map[int]bool{}
}

// shifted tells the rows from i on that the list lost the one it held there,
// so that they come from as far down as it was: they slide up into their
// places rather than jumping there.
func (m *rowsMotion) shifted(from int, h float32) {
	m.slideFrom, m.slide, m.slid = from, h, map[int]bool{}
}

// row poses row i of the list, which comes in, or slides up as a row above it
// went.
func (m *rowsMotion) row(c *ui.Context, i int, el ui.Element) {
	if m.slid[i] {
		el.AnimatePose(ui.Pose{}, rowsSpring) // up with the others, not one by one
		return
	}
	if m.slide != 0 && i >= m.slideFrom {
		m.slid[i] = true
		el.AnimatePose(ui.Pose{Y: m.slide}, rowsSpring)
		c.AnimationFrame() // the frame that heads them up
		return
	}
	if m.at.IsZero() {
		m.at = c.Now()
	}
	if !m.in[i] {
		m.in[i] = true
		if c.Now().Sub(m.at) < 250*time.Millisecond {
			el.AnimatePose(rowsOut, rowsSpring)
			c.AnimationFrame()
			return
		}
	}
	// Under the frames of the display, so that rows start a few to a frame
	// and the wave runs rather than steps.
	a := rowsSpring
	a.Delay = time.Duration(min(i, 12)) * 8 * time.Millisecond
	el.AnimatePose(ui.Pose{}, a)
}

// A row of the queue takes its room as it comes and gives it back as it
// goes, so that the rows below slide rather than jump when one is added or
// removed.
var queueRow = ui.ElementTransition{
	Animation: &ui.Animation{Duration: 200 * time.Millisecond},
	Enter:     &ui.Motion{Collapse: true},
	Exit:      &ui.Motion{Collapse: true},
}

// A dialog comes in as it opens: the backdrop fades, the panel rises a little
// as it fades, quicker than a page as a modal is.
var (
	dialogFade   = ui.Animation{Spring: ui.SpringOptions{Duration: 160 * time.Millisecond}}
	dialogSpring = ui.Animation{Spring: ui.SpringOptions{Duration: 200 * time.Millisecond, Bounce: 0.1}}
)

// The cards of 小宇宙's sources come forward under the pointer, as the one
// about to be taken: quicker than a page, as a hover is.
var (
	sourceCardUp = ui.Pose{Y: -5, Scale: 1.025}
	hoverSpring  = ui.Animation{Spring: ui.SpringOptions{Duration: 200 * time.Millisecond, Bounce: 0.15}}
)

// 换一批 replaces the cards of a section: the new ones come up as they
// fade in, one after the other, so the swap reads as a refresh.
var (
	cardOut    = ui.Pose{Y: 12, Fade: 1}
	cardSpring = ui.Animation{Spring: ui.SpringOptions{Duration: 300 * time.Millisecond}}
)

// refreshSpin is how long 换一批 turns its icon for a refresh whose data was
// already there: one turn, so that it ends where it started.
const refreshSpin = time.Second

// What is still loading says so by breathing, slower than anything that
// shows a change: the bones of a page pulse, and the bars of the equalizer
// rise and fall at their own paces, so that they do not read as one.
const (
	pulseLoop = 1400 * time.Millisecond
	eqBeat    = 500 * time.Millisecond
	eqStep    = 170 * time.Millisecond
)

// refreshMotion brings in the cards of a section again once a new batch
// replaces them, and turns the icon of 换一批 meanwhile.
type refreshMotion struct {
	key any          // the page or the category shown; pages and strings compare
	in  map[int]bool // cards already posed out for this batch
	at  time.Time    // when they started coming in
}

// arm makes the cards come in at the next build.
func (m *refreshMotion) arm() {
	m.in, m.at = map[int]bool{}, time.Time{}
}

// show arms the cards as the list they show is another, as a page or a
// category is: the first ones come in one after the other, as they do after
// 换一批, and a page added to them does not.
func (m *refreshMotion) show(key any) {
	if m.in != nil && m.key == key {
		return
	}
	m.key = key
	m.arm()
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
	a.Delay = time.Duration(min(i, 8)) * 8 * time.Millisecond
	card.AnimatePose(ui.Pose{}, a)
}

// songKey tells a song from the others, as the bar tells them apart.
type songKey struct {
	source, mid string
	id          int64
}

func keyOf(s qqmusic.Song) songKey {
	return songKey{s.Source, s.Mid, int64(s.ID)}
}
