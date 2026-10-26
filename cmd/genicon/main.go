// Command genicon renders resources/logo.svg into the icons of the app:
// the 1024 px icon of mygo build, the window icon, the tray icon and
// icon.ico, the sizes Windows shows the executable at. Run it from the
// module root after changing the logo:
//
//	go run ./cmd/genicon
//
// With -syso it renders nothing and puts icon.ico into
// cmd/qqmusic/mygo_windows_<arch>.syso, which a plain go build links into
// the executable (mygo build and mygo dev write their own in its place).
package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"image"
	"image/color"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"runtime"

	"github.com/egoist/mygo/ui"
)

func main() {
	dir := flag.String("dir", "resources", "the directory of logo.svg and the PNGs")
	preview := flag.String("preview", "", "also write a preview sheet to this PNG")
	syso := flag.Bool("syso", false, "only write icon.ico into mygo_windows_"+runtime.GOARCH+".syso of -main")
	mainDir := flag.String("main", filepath.Join("cmd", "qqmusic"), "the directory of the app's main package, for -syso")
	flag.Parse()

	if *syso {
		ico, err := os.ReadFile(filepath.Join(*dir, "icon.ico"))
		if err != nil {
			log.Fatal(err)
		}
		out, err := iconSyso(ico, runtime.GOARCH)
		if err != nil {
			log.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(*mainDir, "mygo_windows_"+runtime.GOARCH+".syso"), out, 0o644); err != nil {
			log.Fatal(err)
		}
		return
	}

	logo := mustSVG(filepath.Join(*dir, "logo.svg"))
	tray := mustSVG(filepath.Join(*dir, "tray.svg"))
	outputs := []struct {
		name string
		svg  *ui.SVG
		size int
	}{
		{"icon.png", logo, 1024},
		{"icon-256.png", logo, 256},
		{"tray.png", tray, 32},
	}
	for _, o := range outputs {
		writePNG(filepath.Join(*dir, o.name), render(o.svg, o.size))
	}
	// The small sizes, whose ring would blur, are the tray's design.
	var images [][]byte
	for _, size := range icoSizes {
		svg := logo
		if size <= 24 {
			svg = tray
		}
		var b bytes.Buffer
		if err := png.Encode(&b, render(svg, size)); err != nil {
			log.Fatal(err)
		}
		images = append(images, b.Bytes())
	}
	if err := os.WriteFile(filepath.Join(*dir, "icon.ico"), ico(images), 0o644); err != nil {
		log.Fatal(err)
	}
	if *preview != "" {
		writePNG(*preview, sheet(logo, tray))
	}
}

// icoSizes are the sizes Windows shows icons at, from 100% to 250% scale.
var icoSizes = []int{16, 20, 24, 32, 40, 48, 64, 96, 128, 256}

// ico makes an .ico file of PNG images, in the order of icoSizes.
func ico(images [][]byte) []byte {
	var b bytes.Buffer
	le := binary.LittleEndian
	_ = binary.Write(&b, le, [3]uint16{0, 1, uint16(len(images))})
	at := 6 + 16*len(images)
	for i, img := range images {
		dim := byte(icoSizes[i] % 256) // 0 is 256
		b.Write([]byte{dim, dim, 0, 0})
		_ = binary.Write(&b, le, [2]uint16{1, 32})
		_ = binary.Write(&b, le, [2]uint32{uint32(len(img)), uint32(at)})
		at += len(img)
	}
	for _, img := range images {
		b.Write(img)
	}
	return b.Bytes()
}

func mustSVG(path string) *ui.SVG {
	data, err := os.ReadFile(path)
	if err != nil {
		log.Fatal(err)
	}
	svg, err := ui.ParseSVG(data)
	if err != nil {
		log.Fatalf("%s: %v", path, err)
	}
	return svg
}

// render draws svg at size×size pixels with a transparent background. ui.Render
// draws over an opaque window, so it draws twice, on black and on white,
// and takes the alpha from the difference.
func render(svg *ui.SVG, size int) *image.NRGBA {
	on := func(bg ui.Color) *image.RGBA {
		return ui.Render(func(c *ui.Context) {
			ui.Box(c).Size(float32(size), float32(size)).Background(bg).Children(func() {
				ui.Image(c, svg).Size(float32(size), float32(size))
			})
		}, size, size, 1)
	}
	black, white := on(ui.Hex("#000000")), on(ui.Hex("#ffffff"))
	out := image.NewNRGBA(image.Rect(0, 0, size, size))
	for y := range size {
		for x := range size {
			b, w := black.RGBAAt(x, y), white.RGBAAt(x, y)
			a := 255 - (int(w.G) - int(b.G))
			a = min(max(a, 0), 255)
			if a == 0 {
				continue
			}
			un := func(v uint8) uint8 { return uint8(min(int(v)*255/a, 255)) }
			out.SetNRGBA(x, y, color.NRGBA{un(b.R), un(b.G), un(b.B), uint8(a)})
		}
	}
	return out
}

// sheet shows the icons at the sizes Windows uses, on a dark and a light
// background.
func sheet(logo, tray *ui.SVG) *image.RGBA {
	return ui.Render(func(c *ui.Context) {
		for _, bg := range []string{"#202020", "#f3f3f3"} {
			ui.Row(c).Background(ui.Hex(bg)).Padding(16).Gap(20).AlignItems(ui.End).Children(func() {
				for _, s := range []float32{256, 64, 48, 32, 24, 16} {
					ui.Image(c, logo).Size(s, s)
				}
				ui.Image(c, tray).Size(16, 16)
				ui.Image(c, tray).Size(24, 24)
				ui.Image(c, tray).Size(32, 32)
			})
		}
	}, 720, 600, 1)
}

func writePNG(path string, img image.Image) {
	f, err := os.Create(path)
	if err != nil {
		log.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		log.Fatal(err)
	}
	if err := f.Close(); err != nil {
		log.Fatal(err)
	}
}
