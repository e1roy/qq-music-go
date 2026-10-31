//go:build darwin

package app

import "github.com/egoist/mygo"

// systemControls reports that the system draws the window's buttons: macOS
// puts its traffic lights over the top left of the sidebar, which the view
// leaves clear instead of drawing buttons there (view.go: windowControls).
const systemControls = true

const trafficLightSize = 14

// windowChrome is the window's title bar: hidden, with the traffic lights
// inset over the brand row, level with the logo, as a native macOS app's.
// Plain Hidden avoids the toolbar of TitleBarHiddenInset, which AppKit can
// lay out again after showing and move the lights away from the brand row.
func windowChrome() chrome {
	// The close button starts 8 in from the frame's edge, and the lights
	// are as high as the logo: the row has 8 above what it centers, and a
	// light is 14 across.
	return chrome{
		style:  mygo.TitleBarHidden,
		lights: &mygo.Point{X: 16, Y: (8+brandHeight)/2 - trafficLightSize/2},
	}
}
