package app

import (
	"bytes"
	"cmp"
	"context"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"slices"

	"github.com/egoist/mygo/ui"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

// images downloads pictures on goroutines and keeps their bitmaps, by
// URL. It is used on the main thread only.
type images struct {
	fetch  func(ctx context.Context, url string) ([]byte, error)
	post   func(func())
	loaded func() // after a picture comes in or the cache drops some
	sem    chan struct{}
	m      map[string]*imageEntry
	// colors are the pictures' colors (dominantColor), kept as their
	// bitmaps are dropped: they are small.
	colors map[string]ui.Color
	tick   int
	bytes  int // of the pixels of the bitmaps in m
}

type imageEntry struct {
	bmp    *ui.Bitmap
	failed bool
	used   int
	bytes  int
}

// imageBudget is how many bytes of pixels the cache keeps. Past it, it
// drops the pictures shown least lately, but none the last frame showed:
// a cover of 300 pixels takes 360 KB, a page of them some 15 MB.
const imageBudget = 48 << 20

func newImages(fetch func(ctx context.Context, url string) ([]byte, error), post func(func()), loaded func()) *images {
	return &images{fetch: fetch, post: post, loaded: loaded, sem: make(chan struct{}, 6), m: map[string]*imageEntry{}, colors: map[string]ui.Color{}}
}

// frame counts a frame, for the cache to drop what was not shown lately.
func (im *images) frame() {
	im.tick++
	if im.bytes <= imageBudget {
		return
	}
	type old struct {
		url string
		e   *imageEntry
	}
	var olds []old
	for url, e := range im.m {
		if e.bmp != nil && e.used < im.tick-1 {
			olds = append(olds, old{url, e})
		}
	}
	slices.SortFunc(olds, func(x, y old) int { return cmp.Compare(x.e.used, y.e.used) })
	for _, o := range olds {
		if im.bytes <= imageBudget {
			break
		}
		delete(im.m, o.url)
		im.bytes -= o.e.bytes
	}
	im.loaded()
}

// get returns the bitmap of url scaled to at most side pixels, or nil
// while it downloads.
func (im *images) get(url string, side int) *ui.Bitmap {
	if url == "" {
		return nil
	}
	e := im.m[url]
	if e != nil {
		e.used = im.tick
		return e.bmp
	}
	e = &imageEntry{used: im.tick}
	im.m[url] = e
	go func() {
		im.sem <- struct{}{}
		defer func() { <-im.sem }()
		data, err := im.fetch(context.Background(), url)
		var img image.Image
		if err == nil {
			img, _, err = image.Decode(bytes.NewReader(data))
		}
		if err != nil {
			im.post(func() { e.failed = true })
			return
		}
		img = shrink(img, side)
		col, hasCol := dominantColor(img)
		im.post(func() {
			if hasCol {
				im.colors[url] = col
			}
			e.bmp = ui.NewBitmap(img)
			w, h := e.bmp.Size()
			e.bytes = 4 * w * h
			if im.m[url] == e {
				im.bytes += e.bytes
			}
			im.loaded()
		})
	}()
	return nil
}

// peek returns the bitmap of url if it came, without downloading it.
func (im *images) peek(url string) *ui.Bitmap {
	if e := im.m[url]; e != nil && e.bmp != nil {
		e.used = im.tick
		return e.bmp
	}
	return nil
}

// color returns the color of the picture at url, downloading it if it did
// not come yet.
func (im *images) color(url string) (ui.Color, bool) {
	if col, ok := im.colors[url]; ok {
		return col, true
	}
	im.get(url, 300)
	return ui.Color{}, false
}

func shrink(img image.Image, side int) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if side <= 0 || (w <= side && h <= side) {
		return img
	}
	if w >= h {
		h, w = h*side/w, side
	} else {
		w, h = w*side/h, side
	}
	dst := image.NewRGBA(image.Rect(0, 0, max(w, 1), max(h, 1)))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, b, draw.Src, nil)
	return dst
}
