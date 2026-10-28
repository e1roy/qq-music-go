//go:build !darwin

package app

import "github.com/egoist/mygo"

// systemControls reports that the view draws the window's buttons itself,
// in the top right corner of the window (view.go: windowControls).
const systemControls = false

// windowChrome is the window's title bar: hidden, without the system's
// buttons, whose room the page takes.
func windowChrome() chrome {
	return chrome{style: mygo.TitleBarHidden, hideButtons: true}
}
