package app

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/image/draw"
)

func TestScalerFastPath(t *testing.T) {
	im := newImages(nil, nil, func() {})
	if got := im.scaler(imageSize{300, 300}, imageSize{500, 500}); got != draw.ApproxBiLinear {
		t.Fatal("standard covers should use ApproxBiLinear")
	}
	if len(im.scalers) != 0 {
		t.Fatal("the fast path should not retain a CatmullRom scaler")
	}
	if got := im.scaler(imageSize{301, 301}, imageSize{500, 500}); got == draw.ApproxBiLinear {
		t.Fatal("large covers should keep CatmullRom")
	}
}

func TestShrinkScaled(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 300, 200))
	im := newImages(nil, nil, func() {})
	for _, side := range []int{36, 120, 300} {
		got := shrinkScaled(src, side, im.scaler)
		if b := got.Bounds(); max(b.Dx(), b.Dy()) != side {
			t.Errorf("side %d: got %dx%d", side, b.Dx(), b.Dy())
		}
	}
	if len(im.scalers) != 0 {
		t.Fatalf("scalers: got %d, want none for standard covers", len(im.scalers))
	}
	first := im.scaler(imageSize{320, 240}, imageSize{960, 720})
	second := im.scaler(imageSize{320, 240}, imageSize{960, 720})
	if first != second {
		t.Fatal("the same size should reuse its scaler")
	}
	if len(im.scalers) != 1 {
		t.Fatalf("scalers: got %d, want 1 after a large cover", len(im.scalers))
	}
	for i := range maxImageScalers + 2 {
		im.scaler(imageSize{400 + i, 400}, imageSize{40, 40})
	}
	if got := len(im.scalers); got != maxImageScalers {
		t.Fatalf("scalers: got %d, want at most %d", got, maxImageScalers)
	}
}

func TestImagesUpgradeToRequestedSize(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 300, 200))
	for y := range src.Bounds().Dy() {
		for x := range src.Bounds().Dx() {
			src.SetRGBA(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 80, A: 255})
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, src); err != nil {
		t.Fatal(err)
	}

	var calls atomic.Int32
	posts := make(chan func(), 2)
	im := newImages(
		func(context.Context, string) ([]byte, error) {
			calls.Add(1)
			return encoded.Bytes(), nil
		},
		func(apply func()) { posts <- apply },
		func() {},
	)
	waitPost := func() func() {
		t.Helper()
		select {
		case apply := <-posts:
			return apply
		case <-time.After(5 * time.Second):
			t.Fatal("image did not load")
			return nil
		}
	}
	size := func(bmp interface{ Size() (int, int) }) int {
		w, h := bmp.Size()
		return max(w, h)
	}

	if bmp := im.get("cover", 120); bmp != nil {
		t.Fatal("first request should load asynchronously")
	}
	waitPost()()
	small := im.get("cover", 120)
	if small == nil || size(small) != 120 {
		t.Fatalf("small bitmap: got %v, want side 120", small)
	}
	if bmp := im.get("cover", 64); bmp != small {
		t.Fatal("a smaller request should reuse the loaded bitmap")
	}

	if bmp := im.get("cover", 300); bmp != nil {
		t.Fatal("an upgrade should wait for the larger bitmap")
	}
	if bmp := im.get("cover", 300); bmp != nil {
		t.Fatal("repeated requests should not expose the undersized bitmap")
	}
	upgrade := waitPost()
	if got := calls.Load(); got != 2 {
		t.Fatalf("fetches while upgrading: got %d, want 2", got)
	}
	upgrade()
	large := im.get("cover", 300)
	if large == nil || size(large) != 300 {
		t.Fatalf("large bitmap: got %v, want side 300", large)
	}
	if bmp := im.get("cover", 120); bmp != large {
		t.Fatal("a larger loaded bitmap should serve smaller requests")
	}
}
