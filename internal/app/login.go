package app

import (
	"context"
	"errors"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"time"

	"github.com/egoist/mygo/ui"

	"qqmusic/internal/qqmusic"
)

func (a *app) openLogin() {
	a.login.open = true
	if a.login.tab < 2 {
		a.newQR()
	}
}

func (a *app) closeLogin() {
	a.login.open = false
	a.login.gen.Add(1) // stops polling
}

// newQR gets a QR code for the tab and polls it until it is done, expires,
// or the dialog closes or switches tabs.
func (a *app) newQR() {
	l := &a.login
	gen := l.gen.Add(1)
	l.qr, l.bmp, l.err, l.state = nil, nil, "", qqmusic.QRWaiting
	kind := qqmusic.QRQQ
	if l.tab == 1 {
		kind = qqmusic.QRWeChat
	}
	alive := func() bool { return gen == l.gen.Load() }
	go func() {
		ctx := context.Background()
		qr, err := a.api.NewQR(ctx, kind)
		var bmp *ui.Bitmap
		if err == nil {
			bmp, err = ui.DecodeBitmap(qr.Image)
			if err == nil && kind == qqmusic.QRQQ {
				bmp.Pixelated() // a small PNG of square modules
			}
		}
		a.plat.Post(func() {
			if !alive() {
				return
			}
			if err != nil {
				l.err = "获取二维码失败：" + errText(err)
				return
			}
			l.qr, l.bmp = qr, bmp
		})
		if err != nil {
			return
		}
		for {
			time.Sleep(1500 * time.Millisecond)
			if !alive() {
				return
			}
			pctx, cancel := context.WithTimeout(ctx, 40*time.Second)
			st, err := a.api.PollQR(pctx, qr)
			cancel()
			done := make(chan bool, 1)
			a.plat.Post(func() {
				if !alive() {
					done <- true
					return
				}
				if err != nil {
					l.err = errText(err)
					if st == qqmusic.QRDone {
						// Scanned but the login failed: start over.
						done <- true
						return
					}
				} else {
					l.err = ""
				}
				l.state = st
				switch st {
				case qqmusic.QRDone:
					if err == nil {
						a.loggedIn()
					}
					done <- true
				case qqmusic.QRExpired, qqmusic.QRCancelled:
					done <- true
				default:
					done <- false
				}
			})
			if <-done {
				return
			}
		}
	}()
}

// loginWithWebView opens the QQ login page and finishes the login with
// its cookies.
func (a *app) loginWithWebView() {
	if a.login.webBusy {
		return
	}
	a.login.webBusy, a.login.err = true, ""
	a.plat.OpenQQLogin(func(cookies []*cookie, err error) {
		if err == nil {
			jar, _ := cookiejar.New(nil)
			byHost := map[string][]*http.Cookie{}
			for _, c := range cookies {
				host := c.Domain
				for len(host) > 0 && host[0] == '.' {
					host = host[1:]
				}
				byHost[host] = append(byHost[host], &http.Cookie{Name: c.Name, Value: c.Value, Path: "/"})
			}
			for host, cs := range byHost {
				u, _ := url.Parse("https://" + host + "/")
				jar.SetCookies(u, cs)
			}
			// graph.qq.com receives the cookies of qq.com too.
			if cs := byHost["qq.com"]; len(cs) > 0 {
				u, _ := url.Parse("https://graph.qq.com/")
				jar.SetCookies(u, cs)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			err = a.api.LoginQQCookies(ctx, jar)
			cancel()
		}
		a.plat.Post(func() {
			a.login.webBusy = false
			if errors.Is(err, errLoginClosed) {
				return
			}
			if err != nil {
				a.login.err = "QQ 登录失败：" + errText(err)
				return
			}
			a.loggedIn()
		})
	})
}

var errLoginClosed = errors.New("login window closed")
