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

func TestCountColorsMatchesImageAt(t *testing.T) {
	rect := image.Rect(0, 0, 7, 5)
	rgba := image.NewRGBA(rect)
	nrgba := image.NewNRGBA(rect)
	ycbcr := image.NewYCbCr(rect, image.YCbCrSubsampleRatio420)
	gray := image.NewGray(rect)
	for y := range rect.Dy() {
		for x := range rect.Dx() {
			c := color.RGBA{uint8(17 * x), uint8(31 * y), uint8(19 * (x + y)), uint8(61 + 13*(x+y))}
			rgba.SetRGBA(x, y, c)
			nrgba.SetNRGBA(x, y, color.NRGBA{c.R, c.G, c.B, c.A})
			gray.SetGray(x, y, color.Gray{uint8(23 * (x + y))})
			yi, ci := ycbcr.YOffset(x, y), ycbcr.COffset(x, y)
			ycbcr.Y[yi], ycbcr.Cb[ci], ycbcr.Cr[ci] = c.R, 128, 128
		}
	}
	for name, img := range map[string]image.Image{"RGBA": rgba, "NRGBA": nrgba, "YCbCr": ycbcr, "Gray": gray} {
		t.Run(name, func(t *testing.T) {
			want := new([1 << 15]uint16)
			for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
				for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
					r, g, b, a := img.At(x, y).RGBA()
					if a >= 0x8000 {
						want[(r>>11)<<10|(g>>11)<<5|b>>11]++
					}
				}
			}
			got := new([1 << 15]uint16)
			countColors(img, img.Bounds(), 1, got)
			if *got != *want {
				t.Fatal("direct pixel scan differs from image.At")
			}
		})
	}
}
