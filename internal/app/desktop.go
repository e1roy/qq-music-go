package app

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"qqmusic/internal/player"
	"qqmusic/internal/qqmusic"
	"qqmusic/internal/xiaoyuzhou"
	"qqmusic/resources"
)

// desktop is the platform of the real window.
type desktop struct {
	win *mygo.Window

	// bounds settles before the window's bounds are kept in the settings;
	// main thread only.
	bounds *time.Timer

	mu            sync.Mutex
	loginWin      *mygo.Window
	xiaoyuzhouWin *mygo.Window
}

func (d *desktop) Post(fn func()) { d.win.Update(fn) }

func (d *desktop) WebViewAvailable() bool { return mygo.WebViewAvailable() }

func (d *desktop) Minimize()       { d.win.Minimize() }
func (d *desktop) Maximized() bool { return d.win.IsMaximized() }
func (d *desktop) Close()          { d.win.Close() }

func (d *desktop) OpenFolder(path string) { mygo.Shell.OpenPath(path) }

func (d *desktop) ToggleMaximize() {
	if d.win.IsMaximized() {
		d.win.Unmaximize()
	} else {
		d.win.Maximize()
	}
}

// OpenQQLogin shows QQ's login page in a window of its own; once it goes
// to the page that follows a login, it reads the cookies the login set.
func (d *desktop) OpenQQLogin(done func([]*cookie, error)) {
	d.mu.Lock()
	if d.loginWin != nil {
		d.loginWin.Focus()
		d.mu.Unlock()
		return
	}
	d.mu.Unlock()
	go func() {
		// Start from no QQ session, so the page asks who logs in.
		for _, domain := range []string{"qq.com", "graph.qq.com", "ptlogin2.qq.com"} {
			if cs, err := mygo.Cookies.List(mygo.CookieFilter{Domain: domain}); err == nil {
				for _, c := range cs {
					mygo.Cookies.Delete(c)
				}
			}
		}
		w := mygo.NewWindow(mygo.WindowOptions{
			Title: "QQ 登录", Width: 420, Height: 520, Parent: d.win, DisableMaximize: true,
			BackgroundColor: "#ffffff",
		})
		d.mu.Lock()
		d.loginWin = w
		d.mu.Unlock()
		var once sync.Once
		finish := func(cs []*cookie, err error) {
			once.Do(func() {
				d.mu.Lock()
				d.loginWin = nil
				d.mu.Unlock()
				done(cs, err)
			})
		}
		w.OnClosed(func() { finish(nil, errLoginClosed) })
		w.Page().OnDidNavigate(func(url string) {
			if !strings.HasPrefix(url, qqmusic.LoginDoneURLPrefix) {
				return
			}
			go func() {
				var out []*cookie
				for _, u := range []string{"https://graph.qq.com/", "https://ssl.ptlogin2.graph.qq.com/", "https://qq.com/"} {
					cs, err := mygo.Cookies.List(mygo.CookieFilter{URL: u})
					if err != nil {
						continue
					}
					for _, c := range cs {
						out = append(out, &cookie{Name: c.Name, Value: c.Value, Domain: c.Domain, Path: c.Path})
					}
				}
				finish(out, nil)
				w.Close()
			}()
		})
		w.Page().LoadURL(qqmusic.QQLoginPage)
	}()
}

// OpenXiaoyuzhouLogin shows 小宇宙's login page in a window of its own, which logs
// in by phone number past a captcha only a browser solves. The page keeps
// the tokens in cookies of xiaoyuzhoufm.com: once they are there, the
// window closes with them.
func (d *desktop) OpenXiaoyuzhouLogin(done func(access, refresh string, err error)) {
	d.mu.Lock()
	if d.xiaoyuzhouWin != nil {
		d.xiaoyuzhouWin.Focus()
		d.mu.Unlock()
		return
	}
	d.mu.Unlock()
	go func() {
		// Start from no session, so the page asks who logs in.
		if cs, err := mygo.Cookies.List(mygo.CookieFilter{Domain: "xiaoyuzhoufm.com"}); err == nil {
			for _, c := range cs {
				mygo.Cookies.Delete(c)
			}
		}
		w := mygo.NewWindow(mygo.WindowOptions{
			Title: "小宇宙 登录", Width: 460, Height: 720, Parent: d.win, DisableMaximize: true,
			BackgroundColor: "#ffffff",
		})
		d.mu.Lock()
		d.xiaoyuzhouWin = w
		d.mu.Unlock()
		stop := make(chan struct{})
		var once sync.Once
		finish := func(access, refresh string, err error) {
			once.Do(func() {
				close(stop)
				d.mu.Lock()
				d.xiaoyuzhouWin = nil
				d.mu.Unlock()
				done(access, refresh, err)
			})
		}
		w.OnClosed(func() { finish("", "", errLoginClosed) })
		w.Page().LoadURL(xiaoyuzhou.LoginPage)
		for {
			select {
			case <-stop:
				return
			case <-time.After(time.Second):
			}
			cs, err := mygo.Cookies.List(mygo.CookieFilter{Domain: "xiaoyuzhoufm.com"})
			if err != nil {
				continue
			}
			var access, refresh string
			for _, c := range cs {
				switch c.Name {
				case xiaoyuzhou.TokenCookie:
					access = c.Value
				case xiaoyuzhou.RefreshCookie:
					refresh = c.Value
				}
			}
			if access != "" {
				finish(access, refresh, nil)
				w.Close()
				return
			}
		}
	}()
}

// newTray shows the app's icon in the notification area: a click brings
// the window back, a right click opens a menu of the player.
func (d *desktop) newTray(a *app) {
	tray, err := mygo.NewTray(mygo.TrayOptions{ID: "main", Icon: resources.TrayIcon, ToolTip: "QQ音乐"})
	if err != nil {
		log.Println("tray:", err)
		return
	}
	show := func() {
		if d.win.IsMinimized() {
			d.win.Restore()
		}
		d.win.Show()
		d.win.Focus()
	}
	// The menu's clicks run on the main thread; Update redraws after them.
	item := func(label string, fn func()) *mygo.MenuItem {
		return &mygo.MenuItem{Label: label, Click: func(*mygo.MenuItem, *mygo.Window) { d.win.Update(fn) }}
	}
	menu := mygo.NewMenu([]*mygo.MenuItem{
		{Label: "显示主窗口", Click: func(*mygo.MenuItem, *mygo.Window) { show() }},
		mygo.Separator(),
		item("播放 / 暂停", a.togglePlay),
		item("上一首", a.prev),
		item("下一首", func() { a.next(false) }),
		mygo.Separator(),
		{Label: "退出", Role: mygo.RoleQuit},
	})
	tray.OnClick(show)
	// Without a menu set, a left click reaches OnClick; set it only to
	// pop it up.
	tray.OnRightClick(func() {
		tray.SetMenu(menu)
		tray.PopUpMenu()
		tray.SetMenu(nil)
	})
	d.win.OnClosed(tray.Destroy)
}

// watchBounds keeps the bounds of the window in the settings once it
// settles: a run that is killed rather than closed, as `mygo dev` does to
// the build it replaces, keeps the size it was left at.
func (d *desktop) watchBounds(a *app) {
	settle := func() {
		if d.bounds == nil {
			d.bounds = time.AfterFunc(300*time.Millisecond, func() {
				mygo.RunOnMain(func() { d.rememberBounds(a) })
			})
			return
		}
		d.bounds.Reset(300 * time.Millisecond)
	}
	d.win.OnResize(settle)
	d.win.OnMove(settle)
	d.win.OnMaximize(settle)
	d.win.OnUnmaximize(settle)
	d.win.OnRestore(settle)
	d.win.OnClose(func(*mygo.CloseEvent) {
		if d.bounds != nil {
			d.bounds.Stop()
		}
		d.rememberBounds(a)
	})
}

// rememberBounds writes the window's normal bounds, or that it was
// maximized, to the settings.
func (d *desktop) rememberBounds(a *app) {
	if d.win == nil || d.win.IsMinimized() {
		return
	}
	if d.win.IsMaximized() {
		// Its bounds are the maximized ones: only its state changes.
		a.rememberWindow(a.set.WinX, a.set.WinY, a.set.WinWidth, a.set.WinHeight, true)
		return
	}
	b := d.win.Bounds()
	if b.Width <= 0 || b.Height <= 0 {
		return
	}
	a.rememberWindow(b.X, b.Y, b.Width, b.Height, false)
}

// onDisplay reports whether enough of r is on a display to reach it, as a
// window left on a display that is gone is not.
func onDisplay(r mygo.Rectangle) bool {
	for _, d := range mygo.Screen.Displays() {
		a := d.WorkArea
		w := min(a.X+a.Width, r.X+r.Width) - max(a.X, r.X)
		h := min(a.Y+a.Height, r.Y+r.Height) - max(a.Y, r.Y)
		if w >= min(100, r.Width) && h >= min(50, r.Height) {
			return true
		}
	}
	return false
}

// Run opens the main window and runs the app until it quits. It must be
// called on the main goroutine.
func Run() error {
	dir := ""
	if base, err := os.UserConfigDir(); err == nil {
		dir = filepath.Join(base, "QQMusicGo")
	}
	d := &desktop{}
	a := newApp(d, dir)
	// Beside the settings, the songs would go with a roaming profile.
	if base, err := os.UserCacheDir(); err == nil {
		a.player.Cache = player.NewCache(filepath.Join(base, "QQMusicGo", "songs"), int64(a.set.CacheGB)<<30)
	}
	// The UI is dark whatever the system's appearance: the menus must be
	// dark too, or their glyphs are dark on black.
	mygo.Theme.SetSource(mygo.ThemeDark)
	mygo.App.WhenReady(func() {
		// The bounds live in the settings rather than in WindowOptions.StateKey:
		// MyGo saves a keyed window only when it closes, and `mygo dev` kills
		// the build it replaces instead of closing it.
		opts := mygo.WindowOptions{
			Title:             "QQ音乐",
			Width:             1280,
			Height:            820,
			MinWidth:          960,
			MinHeight:         600,
			Maximized:         a.set.WinMax,
			TitleBarStyle:     mygo.TitleBarHidden,
			HideWindowButtons: true,
			Content:           ui.View(a.view),
			// Shown once d.win is set: showing builds the first frame at
			// once, which asks the window whether it is maximized.
			Hidden: true,
		}
		if a.set.WinWidth > 0 && a.set.WinHeight > 0 {
			opts.Width, opts.Height = a.set.WinWidth, a.set.WinHeight
			// A place on a display that is gone would show the window out of
			// reach; without X and Y it opens on the center of one.
			if onDisplay(mygo.Rectangle{X: a.set.WinX, Y: a.set.WinY, Width: opts.Width, Height: opts.Height}) {
				opts.X, opts.Y = a.set.WinX, a.set.WinY
			}
		}
		d.win = mygo.NewWindow(opts)
		if err := d.win.SetIcon(resources.WindowIcon); err != nil {
			log.Println("icon:", err)
		}
		d.win.Show()
		d.watchBounds(a)
		d.win.OnClosed(a.saveSettings)
		d.newTray(a)
		// Background work starts only now: Post goes through d.win.
		if s, ok := a.nowPlaying(); ok {
			a.loadLyrics(s)
		}
		a.enter(a.current())
		a.startSession()
		a.startXiaoyuzhou()
	})
	return mygo.App.Run()
}
