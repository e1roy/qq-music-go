package app

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"qqmusic/internal/qqmusic"
	"qqmusic/resources"
)

// desktop is the platform of the real window.
type desktop struct {
	win *mygo.Window

	mu       sync.Mutex
	loginWin *mygo.Window
}

func (d *desktop) Post(fn func()) { d.win.Update(fn) }

func (d *desktop) WebViewAvailable() bool { return mygo.WebViewAvailable() }

func (d *desktop) Minimize()       { d.win.Minimize() }
func (d *desktop) Maximized() bool { return d.win.IsMaximized() }
func (d *desktop) Close()          { d.win.Close() }

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

// Run opens the main window and runs the app until it quits. It must be
// called on the main goroutine.
func Run() error {
	dir := ""
	if base, err := os.UserConfigDir(); err == nil {
		dir = filepath.Join(base, "QQMusicGo")
	}
	d := &desktop{}
	a := newApp(d, dir)
	// The UI is dark whatever the system's appearance: the menus must be
	// dark too, or their glyphs are dark on black.
	mygo.Theme.SetSource(mygo.ThemeDark)
	mygo.App.WhenReady(func() {
		d.win = mygo.NewWindow(mygo.WindowOptions{
			Title:     "QQ音乐",
			Width:     1280,
			Height:    820,
			MinWidth:  960,
			MinHeight: 600,
			StateKey:  "main",
			// The view draws the window controls, in the page's corner.
			TitleBarStyle:     mygo.TitleBarHidden,
			HideWindowButtons: true,
			Content:           ui.View(a.view),
		})
		if err := d.win.SetIcon(resources.WindowIcon); err != nil {
			log.Println("icon:", err)
		}
		d.win.OnClosed(a.saveSettings)
		d.newTray(a)
		// Background work starts only now: Post goes through d.win.
		if s, ok := a.nowPlaying(); ok {
			a.loadLyrics(s)
		}
		a.enter(a.current())
		a.startSession()
	})
	return mygo.App.Run()
}
