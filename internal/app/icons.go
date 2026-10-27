package app

import (
	"github.com/egoist/mygo/ui"

	"qqmusic/resources"
)

// The logo of the app.
var icLogo = ui.MustParseSVG(resources.LogoSVG)

// Icons in the style of Lucide (ISC license), drawn in the text color.
func icon(body string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">` + body + `</svg>`))
}

func filledIcon(body string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="currentColor" stroke="none">` + body + `</svg>`))
}

var (
	icHome     = icon(`<path d="M3 10.5 12 3l9 7.5"/><path d="M5 9.5V20a1 1 0 0 0 1 1h4v-6h4v6h4a1 1 0 0 0 1-1V9.5"/>`)
	icCompass  = icon(`<circle cx="12" cy="12" r="10"/><path d="m16.24 7.76-2.12 6.36-6.36 2.12 2.12-6.36 6.36-2.12z"/>`)
	icSearch   = icon(`<circle cx="11" cy="11" r="7"/><path d="m21 21-4.3-4.3"/>`)
	icPodcast  = icon(`<path d="M16.85 18.58a9 9 0 1 0-9.7 0"/><path d="M8 14a5 5 0 1 1 8 0"/><circle cx="12" cy="11" r="1"/><path d="M13 17a1 1 0 1 0-2 0l.5 4.5a.5.5 0 1 0 1 0Z"/>`)
	icHeart    = icon(`<path d="M19 14c1.49-1.46 3-3.21 3-5.5A5.5 5.5 0 0 0 16.5 3c-1.76 0-3 .5-4.5 2-1.5-1.5-2.74-2-4.5-2A5.5 5.5 0 0 0 2 8.5c0 2.3 1.5 4.05 3 5.5l7 7Z"/>`)
	icHeartOn  = filledIcon(`<path d="M19 14c1.49-1.46 3-3.21 3-5.5A5.5 5.5 0 0 0 16.5 3c-1.76 0-3 .5-4.5 2-1.5-1.5-2.74-2-4.5-2A5.5 5.5 0 0 0 2 8.5c0 2.3 1.5 4.05 3 5.5l7 7Z"/>`)
	icList     = icon(`<path d="M3 6h13"/><path d="M3 12h9"/><path d="M3 18h9"/><path d="M16 12v9"/><circle cx="14" cy="19" r="2"/><path d="M16 12l4-1.5"/>`)
	icQueue    = icon(`<path d="M3 6h18"/><path d="M3 12h18"/><path d="M3 18h12"/><path d="m17 16 4 2-4 2z"/>`)
	icPlay     = filledIcon(`<path d="M7 4.5v15a1 1 0 0 0 1.5.86l12-7.5a1 1 0 0 0 0-1.72l-12-7.5A1 1 0 0 0 7 4.5Z"/>`)
	icPause    = filledIcon(`<rect x="6" y="4" width="4" height="16" rx="1"/><rect x="14" y="4" width="4" height="16" rx="1"/>`)
	icPrev     = filledIcon(`<path d="M18 5.2v13.6a.8.8 0 0 1-1.25.66L7.5 13.1a1.3 1.3 0 0 1 0-2.2l9.25-6.36A.8.8 0 0 1 18 5.2Z"/><rect x="4" y="4.5" width="2.5" height="15" rx="1"/>`)
	icNext     = filledIcon(`<path d="M6 5.2v13.6a.8.8 0 0 0 1.25.66l9.25-6.36a1.3 1.3 0 0 0 0-2.2L7.25 4.54A.8.8 0 0 0 6 5.2Z"/><rect x="17.5" y="4.5" width="2.5" height="15" rx="1"/>`)
	icRepeat   = icon(`<path d="m17 2 4 4-4 4"/><path d="M3 11v-1a4 4 0 0 1 4-4h14"/><path d="m7 22-4-4 4-4"/><path d="M21 13v1a4 4 0 0 1-4 4H3"/>`)
	icRepeat1  = icon(`<path d="m17 2 4 4-4 4"/><path d="M3 11v-1a4 4 0 0 1 4-4h14"/><path d="m7 22-4-4 4-4"/><path d="M21 13v1a4 4 0 0 1-4 4H3"/><path d="M11 10h1v4"/>`)
	icShuffle  = icon(`<path d="M2 18h1.4c1.3 0 2.5-.6 3.3-1.7l6.1-8.6c.7-1.1 2-1.7 3.3-1.7H22"/><path d="m18 2 4 4-4 4"/><path d="M2 6h1.9c1.5 0 2.9.9 3.6 2.2"/><path d="M22 18h-5.9c-1.3 0-2.6-.7-3.3-1.8l-.5-.8"/><path d="m18 14 4 4-4 4"/>`)
	icOrder    = icon(`<path d="M10 6h11"/><path d="M10 12h11"/><path d="M10 18h11"/><path d="M4 6h1v4"/><path d="M4 10h2"/><path d="M6 18H4c0-1 2-2 2-3s-1-1.5-2-1"/>`)
	icVolume   = icon(`<path d="M11 4.7a.7.7 0 0 0-1.2-.5L6.4 7.6A1.4 1.4 0 0 1 5.4 8H3a1 1 0 0 0-1 1v6a1 1 0 0 0 1 1h2.4a1.4 1.4 0 0 1 1 .4l3.4 3.4a.7.7 0 0 0 1.2-.5z"/><path d="M16 9a5 5 0 0 1 0 6"/><path d="M19.4 18.4a9 9 0 0 0 0-12.8"/>`)
	icVolumeX  = icon(`<path d="M11 4.7a.7.7 0 0 0-1.2-.5L6.4 7.6A1.4 1.4 0 0 1 5.4 8H3a1 1 0 0 0-1 1v6a1 1 0 0 0 1 1h2.4a1.4 1.4 0 0 1 1 .4l3.4 3.4a.7.7 0 0 0 1.2-.5z"/><path d="m22 9-6 6"/><path d="m16 9 6 6"/>`)
	icLyric    = icon(`<path d="M4 6h12"/><path d="M4 12h9"/><path d="M4 18h6"/><circle cx="17" cy="17" r="3"/><path d="M20 17V8l2-1"/>`)
	icBack     = icon(`<path d="m15 18-6-6 6-6"/>`)
	icForward  = icon(`<path d="m9 18 6-6-6-6"/>`)
	icDown     = icon(`<path d="m6 9 6 6 6-6"/>`)
	icUp       = icon(`<path d="m18 15-6-6-6 6"/>`)
	icCheck    = icon(`<path d="M20 6 9 17l-5-5"/>`)
	icUpDown   = icon(`<path d="m7 15 5 5 5-5"/><path d="m7 9 5-5 5 5"/>`)
	icAlert    = icon(`<circle cx="12" cy="12" r="10"/><path d="M12 8v4"/><path d="M12 16h.01"/>`)
	icMinimize = icon(`<path d="M5 12h14"/>`)
	icMaximize = icon(`<rect x="5" y="5" width="14" height="14" rx="2"/>`)
	icRestore  = icon(`<rect x="4" y="8" width="12" height="12" rx="2"/><path d="M8 8V6a2 2 0 0 1 2-2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2h-2"/>`)
	icClose    = icon(`<path d="M18 6 6 18"/><path d="m6 6 12 12"/>`)
	icUser     = icon(`<circle cx="12" cy="8" r="4"/><path d="M4 21a8 8 0 0 1 16 0"/>`)
	icLogout   = icon(`<path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"/><path d="m16 17 5-5-5-5"/><path d="M21 12H9"/>`)
	icClock    = icon(`<circle cx="12" cy="12" r="10"/><path d="M12 6v6l4 2"/>`)
	icDisc     = icon(`<circle cx="12" cy="12" r="10"/><circle cx="12" cy="12" r="3"/>`)
	icTrash    = icon(`<path d="M3 6h18"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6"/><path d="M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/>`)
	icPlus     = icon(`<path d="M12 5v14"/><path d="M5 12h14"/>`)
	icMore     = icon(`<circle cx="12" cy="12" r="1"/><circle cx="19" cy="12" r="1"/><circle cx="5" cy="12" r="1"/>`)
	icRefresh  = icon(`<path d="M21 12a9 9 0 1 1-3-6.7L21 8"/><path d="M21 3v5h-5"/>`)
	icSparkles = icon(`<path d="M12 3l1.9 5.8L20 11l-6.1 2.2L12 19l-1.9-5.8L4 11l6.1-2.2z"/><path d="M19 3v4"/><path d="M21 5h-4"/>`)
	icFlame    = icon(`<path d="M8.5 14.5A2.5 2.5 0 0 0 11 12c0-1.38-.5-2-1-3-1.07-2.14-.22-4.05 2-6 .5 2.5 2 4.9 4 6.5 2 1.6 3 3.5 3 5.5a7 7 0 1 1-14 0c0-1.15.43-2.29 1-3a2.5 2.5 0 0 0 2.5 2.5z"/>`)
	icTrending = icon(`<path d="M22 7l-8.5 8.5-5-5L2 17"/><path d="M16 7h6v6"/>`)
	icStar     = icon(`<path d="M12 2.5l2.9 5.9 6.5.9-4.7 4.6 1.1 6.5L12 17.3l-5.8 3.1 1.1-6.5-4.7-4.6 6.5-.9z"/>`)
	icMusic    = icon(`<path d="M9 18V5l12-2v13"/><circle cx="6" cy="18" r="3"/><circle cx="18" cy="16" r="3"/>`)
	icSwap     = icon(`<path d="M8 3 4 7l4 4"/><path d="M4 7h16"/><path d="m16 21 4-4-4-4"/><path d="M20 17H4"/>`)
	icHeadset  = icon(`<path d="M3 14h3a2 2 0 0 1 2 2v3a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-7a9 9 0 0 1 18 0v7a2 2 0 0 1-2 2h-1a2 2 0 0 1-2-2v-3a2 2 0 0 1 2-2h3"/>`)
	icComment  = icon(`<path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"/>`)
	icThumb    = icon(`<path d="M7 10v12"/><path d="M15 5.88 14 10h5.83a2 2 0 0 1 1.92 2.56l-2.33 8A2 2 0 0 1 17.5 22H4a2 2 0 0 1-2-2v-8a2 2 0 0 1 2-2h2.76a2 2 0 0 0 1.79-1.11L12 2a3.13 3.13 0 0 1 3 3.88Z"/>`)
	icQR       = icon(`<rect x="3" y="3" width="7" height="7" rx="1"/><rect x="14" y="3" width="7" height="7" rx="1"/><rect x="3" y="14" width="7" height="7" rx="1"/><path d="M14 14h3v3h-3z"/><path d="M21 14v.01"/><path d="M21 21h-4v-4"/><path d="M14 21v.01"/>`)
	icPenguin  = icon(`<path d="M12 2c3.3 0 5.5 2.6 5.5 6.2 0 1.3.4 2.3 1.2 3.4 1 1.4 1.8 3 1.8 4.9 0 1.2-.5 1.8-1.3 1.6-.5-.1-.9-.6-1.2-1.3-.4 1.6-1.4 2.9-2.8 3.7.8.3 1.3.8 1.3 1.3 0 .7-1.6 1.2-4.5 1.2s-4.5-.5-4.5-1.2c0-.5.5-1 1.3-1.3-1.4-.8-2.4-2.1-2.8-3.7-.3.7-.7 1.2-1.2 1.3-.8.2-1.3-.4-1.3-1.6 0-1.9.8-3.5 1.8-4.9.8-1.1 1.2-2.1 1.2-3.4C6.5 4.6 8.7 2 12 2Z"/>`)
	icWeChat   = icon(`<path d="M9 4C5.1 4 2 6.6 2 9.8c0 1.8 1 3.4 2.6 4.5L4 16.5l2.6-1.3c.8.2 1.6.4 2.4.4"/><path d="M15.5 9c-3.6 0-6.5 2.4-6.5 5.3s2.9 5.3 6.5 5.3c.7 0 1.4-.1 2-.3l2.3 1.2-.6-2c1.6-1 2.8-2.5 2.8-4.2C22 11.4 19.1 9 15.5 9Z"/><path d="M7 8.5v.01"/><path d="M11 8.5v.01"/><path d="M13.5 13.5v.01"/><path d="M17.5 13.5v.01"/>`)
)
