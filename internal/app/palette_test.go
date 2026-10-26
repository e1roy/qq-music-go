package app

import (
	"image"
	"image/color"
	"testing"

	"github.com/egoist/mygo/ui"
)

func fill(img *image.RGBA, r image.Rectangle, c color.RGBA) {
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			img.SetRGBA(x, y, c)
		}
	}
}

func TestDominantColor(t *testing.T) {
	// Mostly a deep blue, with a little red and a lot of black.
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	fill(img, img.Rect, color.RGBA{0, 0, 0, 255})
	fill(img, image.Rect(0, 0, 100, 50), color.RGBA{30, 60, 200, 255})
	fill(img, image.Rect(0, 90, 5, 100), color.RGBA{230, 20, 20, 255})
	col, ok := dominantColor(img)
	if !ok || col.B < 150 || col.R > 80 {
		t.Fatalf("blue picture: got %v, %v", col, ok)
	}

	// Gray stays gray.
	fill(img, img.Rect, color.RGBA{120, 120, 120, 255})
	col, ok = dominantColor(img)
	if !ok {
		t.Fatal("gray picture: no color")
	}
	if _, ch, _ := oklch(tint(col)); ch > 0.01 {
		t.Fatalf("gray picture: tint %v has chroma %v", tint(col), ch)
	}

	// Black has none.
	fill(img, img.Rect, color.RGBA{0, 0, 0, 255})
	if col, ok := dominantColor(img); ok {
		t.Fatalf("black picture: got %v", col)
	}
}

func TestTint(t *testing.T) {
	for _, c := range []color.RGBA{{255, 255, 0, 255}, {10, 0, 40, 255}, {250, 250, 250, 255}, {200, 30, 30, 255}} {
		l, ch, _ := oklch(tint(rgb(c)))
		if l < 0.355 || l > 0.505 || ch > 0.135 {
			t.Errorf("tint of %v: lightness %.3f, chroma %.3f", c, l, ch)
		}
	}
}

func rgb(c color.RGBA) ui.Color { return ui.RGB(c.R, c.G, c.B) }
