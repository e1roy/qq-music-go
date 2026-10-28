package app

import (
	"fmt"
	"slices"

	"github.com/egoist/mygo/ui"
)

// appVersion and appRepo are shown in 关于. appVersion is kept in sync by
// hand with mygo.json's version when releasing (打包build.bat / .sh writes
// mygo.json; nothing reads it back into the binary).
const (
	appVersion = "1.0.9"
	appRepo    = "https://github.com/e1roy/qq-music-go"
)

// settingsSections are the sections of 设置, a.settingsTab indexing them:
// a menu on the left, as an admin's pages have, and the section chosen on
// the right.
var settingsSections = []struct {
	icon  *ui.SVG
	label string
}{
	{icDrive, "缓存"},
	{icInfo, "关于"},
}

func (a *app) settingsPage(c *ui.Context) {
	a.pageScroll(c, &a.main, tintNone, func() {
		ui.Text(c, "设置").FontSize(26).FontWeight(600).LetterSpacing(-0.6).Margin(8, 0, 6, 0)
		ui.Row(c).Gap(32).AlignItems(ui.Start).Children(func() {
			ui.Column(c).Width(180).Children(func() {
				for i, s := range settingsSections {
					if navItem(c.Key(i), s.icon, s.label, a.settingsTab == i).Clicked() {
						a.settingsTab = i
					}
				}
			})
			pane(c, a.settingsTab, 16, func() {
				switch a.settingsTab {
				case 0:
					a.cacheSettings(c)
				case 1:
					aboutSettings(c)
				}
			}).Grow(1).MinWidth(0).MaxWidth(720)
		})
	})
}

// settingsCard is a card of a section of 设置: a title, what it is for,
// and its controls.
func settingsCard(c *ui.Context, title, about string, body func()) {
	ui.Column(c).Padding(20).Gap(14).Radius(radiusXl).Background(colCard).Border(1, colLine).Children(func() {
		ui.Column(c).Gap(4).Children(func() {
			ui.Text(c, title).FontSize(15).FontWeight(600)
			ui.Text(c, about).FontSize(13).TextColor(colMuted).LineHeight(1.5)
		})
		body()
	})
}

// cacheSettings is the song cache: what it takes, where, and how much it
// may keep.
func (a *app) cacheSettings(c *ui.Context) {
	bytes := func(n int64) string {
		if n >= 1<<30 {
			return fmt.Sprintf("%.2f GB", float64(n)/(1<<30))
		}
		return formatSize(n)
	}
	limit := int64(a.set.CacheGB) << 30
	settingsCard(c, "存储空间", "播放完整的歌曲保存在本机，再次播放时直接读取，不用重新下载。", func() {
		if a.player.Cache == nil {
			ui.Text(c, "缓存目录不可用").FontSize(13).TextColor(colDanger)
			return
		}
		ui.Row(c).Gap(8).AlignItems(ui.End).Children(func() {
			used := "计算中…"
			if a.cacheUsed >= 0 {
				used = bytes(a.cacheUsed)
			}
			ui.Text(c, used).FontSize(24).FontWeight(600).FontFeatures("tnum")
			of := "不保存歌曲"
			if limit > 0 {
				of = fmt.Sprintf("/ %d GB", a.set.CacheGB)
			}
			ui.Text(c, of).FontSize(13).TextColor(colMuted).MarginY(4)
		})
		if limit > 0 {
			ui.Box(c).FillWidth().Height(6).Radius(3).Background(colHover).Children(func() {
				// The bar grows into what the cache takes rather than jumping
				// there as it is counted.
				bar := ui.Box(c).FillHeight().Radius(3).Background(colAccent)
				used := float32(min(1, float64(max(a.cacheUsed, 0))/float64(limit))) * 100
				bar.WidthPercent(bar.AnimateTo("used", used, sideSpring))
			})
		}
		ui.Text(c, a.player.Cache.Dir()).FontSize(12).TextColor(colMuted).SingleLine()
		ui.Row(c).Gap(8).Children(func() {
			if button(c, outline, icFolder, "打开文件夹").Clicked() {
				a.openCacheFolder()
			}
			clear := button(c, outline, icTrash, "清空缓存")
			if a.cacheBusy {
				clear.Disabled(true)
			}
			if clear.Clicked() && !a.cacheBusy {
				a.clearCache()
			}
		})
	})
	settingsCard(c, "空间上限", "超出上限时，先删除最久没播放的歌曲；正在播放的不会删除。", func() {
		ui.Row(c).Children(func() {
			tabs(c, slices.Index(cacheTiers, a.set.CacheGB), false, func(i int) { a.setCacheGB(cacheTiers[i]) }, "关闭", "1 GB", "2 GB", "5 GB", "10 GB")
		})
		for i, note := range []string{
			"单首超过上限 1/4 的歌曲不保存；没下载完就切走的歌曲不保存。",
			"播客单集也会临时写到这里，播完即删除，不占用上限。",
			"选「关闭」后不再保存新歌曲，已保存的会被删除。",
		} {
			ui.Text(c.Key(i), "· "+note).FontSize(12).TextColor(colMuted).LineHeight(1.5)
		}
	})
}

// aboutSettings is 关于: what the project is, where its source is, the
// version running, and the disclaimer about data and accounts.
func aboutSettings(c *ui.Context) {
	settingsCard(c, "QQ音乐桌面版", "纯 Go 编写的 Windows / macOS 桌面 QQ 音乐客户端，原生绘制界面，无 WebView、无 cgo、无前端工程。", func() {
		ui.Row(c).Gap(14).AlignItems(ui.Center).Children(func() {
			ui.Box(c).Size(40, 40).Radius(radiusMd).Center().Background(colHover).Children(func() {
				ui.Image(c, icLogo).Size(24, 24)
			})
			ui.Column(c).Gap(2).Children(func() {
				ui.Text(c, "版本 "+appVersion).FontSize(14).FontWeight(600).FontFeatures("tnum")
				ui.Text(c, "界面：MyGo · 音源：QQ 音乐网页端接口").FontSize(12).TextColor(colMuted)
			})
		})
		ui.Row(c).Gap(8).Children(func() {
			if button(c, outline, icExternal, "GitHub 仓库").Clicked() {
				c.OpenURL(appRepo)
			}
		})
		ui.Text(c, appRepo).FontSize(12).TextColor(colMuted).SingleLine()
	})
	settingsCard(c, "免责声明", "本项目为个人开发的开源学习项目，请阅读以下说明后使用。", func() {
		for i, note := range []string{
			"本项目与腾讯 / QQ 音乐无任何关联，未获得官方授权，仅供学习交流使用，请勿用于商业用途。",
			"所用数据与接口来自 QQ 音乐网页端，接口随时可能变化或失效，不保证长期可用。",
			"播放依赖用户自己登录的 QQ 账号，VIP / 付费内容仍需账号本身具备相应权限，本项目不提供也不协助绕过付费限制。",
			"请支持正版音乐，购买正版会员；由此产生的一切纠纷与法律责任由使用者自行承担，与作者无关。",
			"如本项目侵犯你的权益，请联系作者或提交 Issue，将第一时间处理（下架、修改等）。",
		} {
			ui.Text(c.Key(i), "· "+note).FontSize(12).TextColor(colMuted).LineHeight(1.5)
		}
	})
}
