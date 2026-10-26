// Package resources embeds the pictures of the app. logo.svg and tray.svg
// are the sources; go run ./cmd/genicon renders the PNGs from them
// (icon.png is the icon mygo build puts in the exe).
package resources

import _ "embed"

var (
	//go:embed logo.svg
	LogoSVG []byte
	//go:embed icon-256.png
	WindowIcon []byte
	//go:embed tray.png
	TrayIcon []byte
)
