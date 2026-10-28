package app

import (
	"image"
	"math"
	"slices"
	"time"

	"github.com/egoist/mygo/ui"
)

// The colors of pages come from their pictures, as Android's Palette
// (androidx.palette) finds them: the picture's pixels are cut into a few
// boxes of like colors (median cut, in RGB of 5 bits a channel), and of
// their average colors the one nearest a target of saturation and
// lightness, weighted by how much of the picture it covers, wins.

// swatch is a color of a picture and how many of its pixels have it.
type swatch struct {
	r, g, b int // 0 to 255
	pop     int
	h, s, l float64 // HSL, h in degrees
}

func newSwatch(r, g, b, pop int) swatch {
	h, s, l := hsl(r, g, b)
	return swatch{r: r, g: g, b: b, pop: pop, h: h, s: s, l: l}
}

// target is what a swatch is chosen for, as Palette's Target: ranges of
// saturation and lightness it must lie in, and the values it is best at.
type target struct {
	minS, s, maxS float64
	minL, l, maxL float64
}

// The targets in the order they are tried. A vivid color in the middle of
// lightness makes the best background for white text; a muted one comes
// next, and the light ones, made darker, last.
var paletteTargets = []target{
	{0.35, 1, 1, 0.3, 0.5, 0.7},  // vibrant
	{0.35, 1, 1, 0, 0.26, 0.45},  // dark vibrant
	{0, 0.3, 0.4, 0.3, 0.5, 0.7}, // muted
	{0, 0.3, 0.4, 0, 0.26, 0.45}, // dark muted
	{0.35, 1, 1, 0.55, 0.74, 1},  // light vibrant
	{0, 0.3, 0.4, 0.55, 0.74, 1}, // light muted
}

// Palette's weights of how near a swatch is in saturation and lightness,
// and of how many pixels it has.
const (
	weightS   = 0.24
	weightL   = 0.52
	weightPop = 0.24
	// A swatch of fewer of the pixels is a detail, not the picture's color.
	minShare = 0.03
)

// dominantColor is the color a picture is seen as, or false if it has
// none, as one all black or white.
func dominantColor(img image.Image) (ui.Color, bool) {
	sw, total := quantize(img, 16)
	if len(sw) == 0 {
		return ui.Color{}, false
	}
	maxPop := 0
	for _, s := range sw {
		maxPop = max(maxPop, s.pop)
	}
	for _, t := range paletteTargets {
		best, score := -1, 0.0
		for i, s := range sw {
			if s.s < t.minS || s.s > t.maxS || s.l < t.minL || s.l > t.maxL || float64(s.pop) < minShare*float64(total) {
				continue
			}
			v := weightS*(1-math.Abs(s.s-t.s)) + weightL*(1-math.Abs(s.l-t.l)) + weightPop*float64(s.pop)/float64(maxPop)
			if best < 0 || v > score {
				best, score = i, v
			}
		}
		if best >= 0 {
			s := sw[best]
			return ui.RGB(uint8(s.r), uint8(s.g), uint8(s.b)), true
		}
	}
	// None fits a target: the color most pixels have.
	s := slices.MaxFunc(sw, func(x, y swatch) int { return x.pop - y.pop })
	return ui.RGB(uint8(s.r), uint8(s.g), uint8(s.b)), true
}

// quantize cuts the colors of img into at most n swatches, as Palette's
// ColorCutQuantizer, and tells how many pixels it counted.
func quantize(img image.Image, n int) ([]swatch, int) {
	b := img.Bounds()
	// Some 100 by 100 pixels are plenty.
	step := max(1, max(b.Dx(), b.Dy())/100)
	hist := make([]int, 1<<15)
	total := 0
	for y := b.Min.Y; y < b.Max.Y; y += step {
		for x := b.Min.X; x < b.Max.X; x += step {
			r, g, bl, a := img.At(x, y).RGBA()
			if a < 0x8000 {
				continue
			}
			hist[(r>>11)<<10|(g>>11)<<5|bl>>11]++
		}
	}
	var colors []int
	for c, k := range hist {
		if k == 0 {
			continue
		}
		r, g, bl := expand(c)
		if ignored(newSwatch(r, g, bl, 0)) {
			hist[c] = 0
			continue
		}
		colors = append(colors, c)
		total += k
	}
	if len(colors) <= n {
		sw := make([]swatch, len(colors))
		for i, c := range colors {
			r, g, bl := expand(c)
			sw[i] = newSwatch(r, g, bl, hist[c])
		}
		return sw, total
	}
	// Split the box of the largest volume until there are n.
	boxes := []box{newBox(colors)}
	for len(boxes) < n {
		i := -1
		for j, bx := range boxes {
			if bx.splittable() && (i < 0 || bx.volume() > boxes[i].volume()) {
				i = j
			}
		}
		if i < 0 {
			break
		}
		lo, hi := boxes[i].split(hist)
		boxes[i] = lo
		boxes = append(boxes, hi)
	}
	sw := make([]swatch, len(boxes))
	for i, bx := range boxes {
		sw[i] = bx.average(hist)
	}
	return sw, total
}

// box is a range of colors of 5 bits a channel, as Palette's Vbox.
type box struct {
	colors   []int // sorted along the box's longest side by split
	min, max [3]int
}

func newBox(colors []int) box {
	bx := box{colors: colors}
	bx.fit()
	return bx
}

func channels(c int) [3]int { return [3]int{c >> 10 & 31, c >> 5 & 31, c & 31} }

func (bx *box) fit() {
	bx.min, bx.max = [3]int{31, 31, 31}, [3]int{}
	for _, c := range bx.colors {
		ch := channels(c)
		for k := range 3 {
			bx.min[k], bx.max[k] = min(bx.min[k], ch[k]), max(bx.max[k], ch[k])
		}
	}
}

func (bx box) volume() int {
	return (bx.max[0] - bx.min[0] + 1) * (bx.max[1] - bx.min[1] + 1) * (bx.max[2] - bx.min[2] + 1)
}

func (bx box) splittable() bool { return len(bx.colors) > 1 }

// split cuts the box across its longest side where half its pixels are on
// each side.
func (bx box) split(hist []int) (box, box) {
	side := 0
	for k := 1; k < 3; k++ {
		if bx.max[k]-bx.min[k] > bx.max[side]-bx.min[side] {
			side = k
		}
	}
	slices.SortFunc(bx.colors, func(x, y int) int { return channels(x)[side] - channels(y)[side] })
	pop := 0
	for _, c := range bx.colors {
		pop += hist[c]
	}
	at, sum := 1, 0
	for i, c := range bx.colors[:len(bx.colors)-1] {
		sum += hist[c]
		if sum >= pop/2 {
			at = i + 1
			break
		}
	}
	lo, hi := box{colors: bx.colors[:at:at]}, box{colors: bx.colors[at:]}
	lo.fit()
	hi.fit()
	return lo, hi
}

// average is the box's color, weighted by its pixels.
func (bx box) average(hist []int) swatch {
	var r, g, b, pop int
	for _, c := range bx.colors {
		k := hist[c]
		cr, cg, cb := expand(c)
		r, g, b, pop = r+cr*k, g+cg*k, b+cb*k, pop+k
	}
	return newSwatch(r/pop, g/pop, b/pop, pop)
}

// expand turns a color of 5 bits a channel into one of 8.
func expand(c int) (r, g, b int) {
	ch := channels(c)
	return ch[0]<<3 | ch[0]>>2, ch[1]<<3 | ch[1]>>2, ch[2]<<3 | ch[2]>>2
}

// ignored is Palette's default filter: almost black, almost white, and
// the hues of skin, which make no color of their own.
func ignored(s swatch) bool {
	return s.l <= 0.05 || s.l >= 0.95 || (s.h >= 10 && s.h <= 37 && s.s <= 0.82)
}

func hsl(r, g, b int) (h, s, l float64) {
	rf, gf, bf := float64(r)/255, float64(g)/255, float64(b)/255
	mx, mn := max(rf, gf, bf), min(rf, gf, bf)
	l = (mx + mn) / 2
	d := mx - mn
	if d == 0 {
		return 0, 0, l
	}
	s = d / (1 - math.Abs(2*l-1))
	switch mx {
	case rf:
		h = math.Mod((gf-bf)/d, 6)
	case gf:
		h = (bf-rf)/d + 2
	default:
		h = (rf-gf)/d + 4
	}
	h *= 60
	if h < 0 {
		h += 360
	}
	return h, min(s, 1), l
}

// tint makes a picture's color a background white text reads well on: in
// Oklch, of a middle lightness and no louder than a cover's colors look.
func tint(c ui.Color) ui.Color {
	l, ch, h := oklch(c)
	return ui.Oklch(float32(min(max(l, 0.36), 0.5)), float32(min(ch, 0.13)), float32(h))
}

// oklch converts an sRGB color to Oklch, h in degrees.
func oklch(c ui.Color) (l, ch, h float64) {
	lin := func(v uint8) float64 {
		f := float64(v) / 255
		if f <= 0.04045 {
			return f / 12.92
		}
		return math.Pow((f+0.055)/1.055, 2.4)
	}
	r, g, b := lin(c.R), lin(c.G), lin(c.B)
	lc := math.Cbrt(0.4122214708*r + 0.5363325363*g + 0.0514459929*b)
	mc := math.Cbrt(0.2119034982*r + 0.6806995451*g + 0.1073969566*b)
	sc := math.Cbrt(0.0883024619*r + 0.2817188376*g + 0.6299787005*b)
	l = 0.2104542553*lc + 0.7936177850*mc - 0.0040720468*sc
	A := 1.9779984951*lc - 2.4285922050*mc + 0.4505937099*sc
	B := 0.0259040371*lc + 0.7827717662*mc - 0.8086757660*sc
	h = math.Atan2(B, A) * 180 / math.Pi
	if h < 0 {
		h += 360
	}
	return l, math.Hypot(A, B), h
}

// tintNone is the color of a page with no picture, or before it comes.
var tintNone = ui.Hex("#3e3e3e")

const tintDuration = 600 * time.Millisecond

// tintFade is the color of the top of the page, which goes from the one of
// the page before to the one of this page, as its picture comes. It moves
// in frames that only paint.
type tintFade struct {
	from, to ui.Color
	at       time.Time
	set      bool
}

// pageTint is the background made of the picture at url, or tintNone.
func (a *app) pageTint(url string) ui.Color {
	if url == "" {
		return tintNone
	}
	if col, ok := a.images.color(url); ok {
		return tint(col)
	}
	return tintNone
}

// aim makes col the color to go to.
func (f *tintFade) aim(c *ui.Context, col ui.Color) {
	if !f.set {
		*f = tintFade{from: col, to: col, set: true}
		return
	}
	if col.R == f.to.R && col.G == f.to.G && col.B == f.to.B {
		return
	}
	now := c.Now()
	f.from, f.to, f.at = f.now(now), col, now
}

func (f *tintFade) now(t time.Time) ui.Color {
	k := float32(t.Sub(f.at)) / float32(tintDuration)
	if k >= 1 {
		return f.to
	}
	k = 1 - (1-k)*(1-k)*(1-k) // ease out
	return f.from.Mix(f.to, k)
}

func (f *tintFade) color(p *ui.Painter) ui.Color {
	if p.Now().Sub(f.at) < tintDuration {
		p.AnimationFrame()
	}
	return f.now(p.Now())
}

// The wash's alphas down the page: heavy at the top, lighter past halfway,
// gone at tintHeight. band and the title bar, which is the wash painted
// opaque, read the same numbers, so the one meets the other in its color.
const (
	tintTop  = 0.45
	tintMid  = 0.18
	tintKnee = 0.55
)

// tintWash is the wash's alpha at at, the part of the way down to
// tintHeight.
func tintWash(at float32) float32 {
	switch {
	case at <= 0:
		return tintTop
	case at <= tintKnee:
		return tintTop + (tintMid-tintTop)*(at/tintKnee)
	case at < 1:
		return tintMid * (1 - (at-tintKnee)/(1-tintKnee))
	}
	return 0
}

// band paints the color at the top of r, faint as a wash under the
// page's neutral grays, fading out down to height, or to r's bottom if it
// is lower.
func (f *tintFade) band(height float32) func(p *ui.Painter, r ui.Rect) {
	return func(p *ui.Painter, r ui.Rect) {
		col := f.color(p)
		r.H = min(r.H, height)
		p.FillGradient(r, ui.LinearGradient{Angle: 180, Stops: []ui.GradientStop{
			{Color: col.Alpha(tintWash(0)), At: 0},
			{Color: col.Alpha(tintWash(tintKnee)), At: tintKnee},
			{Color: col.Alpha(0), At: 1},
		}}, 0)
	}
}

// opaque is the wash at at as a color of its own, over the page's gray:
// what a solid bar over the page takes, to be the wash where the page
// shows it, without the page showing through.
func (f *tintFade) opaque(p *ui.Painter, at float32) ui.Color {
	return colPanel.Mix(f.color(p), tintWash(at))
}

// fill paints the color over all of r.
func (f *tintFade) fill(p *ui.Painter, r ui.Rect) {
	p.Fill(r, f.color(p), 0)
}
