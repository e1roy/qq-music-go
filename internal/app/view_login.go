package app

import (
	"github.com/egoist/mygo/ui"

	"qqmusic/internal/qqmusic"
)

func (a *app) loginDialog(c *ui.Context) {
	l := &a.login
	wasOpen := l.open
	ui.DialogBase(c, &l.open, func(backdrop, panel ui.Element) {
		// shadcn's Dialog: a dim backdrop, the panel of the page's color with
		// a border, a title and a line below it.
		backdrop.Background(ui.RGBA(0, 0, 0, 0.5))
		panel.Width(384).Padding(20).Gap(16).Radius(radiusXl).Background(colPanel).AlignItems(ui.Center).
			Shadow(0, 16, 48, 0, ui.RGBA(0, 0, 0, 0.5)).Border(1, colLine)
		ui.Row(c).FillWidth().AlignItems(ui.Start).Gap(8).Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
				ui.Text(c, "登录 QQ 音乐").FontSize(16).FontWeight(600)
				ui.Text(c, "选择一种方式登录，播放歌曲并同步你的歌单").FontSize(13).TextColor(colMuted)
			})
			if iconButton(c, icClose, 16, colMuted, "关闭").Margin(-6, -6, 0, 0).Clicked() {
				a.closeLogin()
			}
		})
		tabs(c, l.tab, true, func(i int) {
			if i == l.tab {
				return
			}
			l.tab = i
			if i < 2 {
				a.newQR()
			} else {
				l.gen.Add(1)
				l.err = ""
			}
		}, "QQ 扫码", "微信扫码", "QQ 账号")
		if l.tab < 2 {
			a.qrPane(c)
		} else {
			a.accountPane(c)
		}
		if l.err != "" {
			ui.Text(c, l.err).FontSize(13).TextColor(colDanger).TextAlign(ui.Center).MaxLines(3)
		}
		ui.Text(c, "登录信息仅保存在本机，用于访问 QQ 音乐接口").FontSize(12).TextColor(colMuted)
	})
	if wasOpen && !l.open {
		l.gen.Add(1) // closed by Escape or a click outside: stop polling
	}
}

func (a *app) qrPane(c *ui.Context) {
	l := &a.login
	app := "手机 QQ"
	if l.tab == 1 {
		app = "微信"
	}
	ui.Box(c).Size(212, 212).Radius(radiusXl).Background(ui.Hex("#ffffff")).Border(1, colLine).Center().Padding(8).Children(func() {
		switch {
		case l.bmp != nil:
			ui.Image(c, l.bmp).Size(196, 196).Fit(ui.Contain)
		case l.err == "":
			ui.Spinner(c).Size(28, 28).TextColor(ui.Hex("#333"))
		}
		if l.state == qqmusic.QRExpired || l.state == qqmusic.QRCancelled || l.state == qqmusic.QRScanned {
			ui.Column(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).Radius(radiusXl).Background(ui.RGBA(255, 255, 255, 0.94)).Center().Gap(10).Children(func() {
				if l.state == qqmusic.QRScanned {
					ui.Icon(c, icPenguin).FontSize(40).TextColor(colAccent)
					ui.Text(c, "扫码成功").FontWeight(600).TextColor(ui.Hex("#0a0a0a"))
					return
				}
				ui.Text(c, l.state.String()).FontSize(14).FontWeight(500).TextColor(ui.Hex("#0a0a0a"))
				if button(c, primary, icRefresh, "刷新二维码").Clicked() {
					a.newQR()
				}
			})
		}
	})
	switch l.state {
	case qqmusic.QRScanned:
		ui.Text(c, "请在"+app+"上确认登录").FontSize(14).TextColor(colAccent)
	case qqmusic.QRDone:
		ui.Row(c).Gap(8).Children(func() {
			ui.Spinner(c).Size(14, 14)
			ui.Text(c, "登录成功，正在同步…").FontSize(14)
		})
	default:
		ui.Text(c, "打开"+app+"，扫一扫登录").FontSize(14)
	}
	if l.err != "" && l.qr == nil {
		if button(c, outline, icRefresh, "重新获取").Clicked() {
			a.newQR()
		}
	}
}

func (a *app) accountPane(c *ui.Context) {
	l := &a.login
	ui.Column(c).FillWidth().Gap(14).Padding(12, 0).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Size(56, 56).Radius(radiusXl).Background(colHover).Center().Children(func() {
			ui.Icon(c, icPenguin).FontSize(28).TextColor(colAccent)
		})
		ui.Text(c, "在 QQ 官方登录页中输入账号密码，\n或使用已登录的 QQ 客户端一键登录").FontSize(13).TextColor(colMuted).TextAlign(ui.Center).LineHeight(1.5)
		if !a.plat.WebViewAvailable() {
			ui.Text(c, "此功能需要 Microsoft Edge WebView2 运行时").FontSize(13).TextColor(colDanger)
			return
		}
		if l.webBusy {
			ui.Row(c).Gap(8).Children(func() {
				ui.Spinner(c).Size(14, 14)
				ui.Text(c, "请在弹出的窗口中完成登录…").FontSize(13)
			})
			return
		}
		if button(c, primary, nil, "打开 QQ 登录窗口").Clicked() {
			a.loginWithWebView()
		}
	})
}
