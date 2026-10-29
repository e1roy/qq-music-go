# 常见改动指引

按步骤做的常见改动。约定见 [`conventions.md`](conventions.md)，文件位置见 [`codemap.md`](codemap.md)。

## 1. 新增一个页面

1. 在 `nav.go` 的 `pageKind` 增加枚举。
2. 在 `page` 中确认需要的字段（`id`/`dirid`/`mid`）。
3. 在 `app` 中增加该页面的状态（如需加载）。
4. 在 `nav.go` 的 `enter(p)` 中按需触发首次加载，并在相邻的 `refresh` / `refreshing` 中加分支（漏掉 `refreshing` 只会让刷新按钮不转，不易发现）。
5. 在 `view_pages.go` 的 `content` 中分发到新的 `xxxPage(c)`。
6. 如需入口，在 `view.go` 的 `sidebar` 增加 `navItem`。
7. 按 [`motion.md`](motion.md) 定页面与列表的动效。
8. 更新 [`codemap.md`](codemap.md)。

## 2. 新增一个接口

1. `internal/qqmusic/api.go` 增加方法；模型放 `types.go`。
2. 用 `c.Call`/`c.CallMany`，注意 `Web` 与 `Desktop` 平台差异。
3. 错误映射：未登录 → `ErrNotLoggedIn`；无音源 → `ErrNoSource`。
4. 在 `cmd/probe/main.go` 增加调用并打印关键字段。
5. 在对应的编排文件（`library.go`、`search.go`、`session.go`、`playback.go` 等）中通过 `a.do` 调用，结果回到主线程再落状态；不要在 `view*.go` 中调用。

## 3. 新增图标

- 通用图标：在 `internal/app/icons.go` 用 `icon(...)` 或 `filledIcon(...)` 添加 Lucide 风格路径。
- 应用/托盘图标：改 `resources/logo.svg`、`resources/tray.svg`，执行 `go run ./cmd/genicon`；需要 exe 图标再执行 `go run ./cmd/genicon -syso`。

## 4. 调整主题

- 颜色、圆角、间距集中在 `view.go` 顶部的 `col*`、`radius*` 与 `theme()`。
- 页面顶部渐变与封面取色在 `palette.go`（`tint` 的 Oklch 夹取范围、`tintFade` 时长）。
- 改完用 `SHOTS` 截图走查 `TestPages`，确认深色对比度。

## 5. 新增一个设置项

1. `settings.go` 的 `settings` 增加字段（带 `json` tag）；旧文件没有该字段时读出零值，确认零值是合理的默认值。
2. 默认值放 `newApp`。
3. 修改处调用 `a.saveSettings()`；有附带规则（提示、去重等）时在 `settings.go` 加方法，参考 `setQuality`。
4. 如需在启动时生效，在 `newApp` 或 `Run` 中应用。
5. 折叠状态这类纯 UI 设置可直接存 `settings`（参考 `MineClosed`/`FavClosed`）。
6. 需要界面入口时放进设置页的分区（`settingsSections`，见 [`architecture.md`](architecture.md) §3）。

## 6. 调试内存/卡顿

- `QQMUSIC_PPROF=127.0.0.1:6060` 启动后访问 `/mem`；`/mem.json` 可取原始采样；`POST /mem/gc` 主动回收。
- 有歌曲缓存时（正式运行都有），`buffer` 把文件写到磁盘再读回，歌曲本身不占 Go 堆，由系统文件缓存承担；只有没有缓存（测试里 `Player.Cache == nil`）或写盘失败时才整首放进内存（MP3 约 4–12 MB，FLAC 约 25–60 MB），按 `Content-Length` 预分配最多 `maxPrealloc`（32 MB）。播放完成后 `trimmer` 延迟 3s 归还。
- 图片缓存上限 48 MiB，按最近使用帧淘汰；封面尺寸由 `cover` 的 `side` 决定（≤64 用 120px，否则 300px）。
- 卡顿优先看是否在 `view` 中做了同步 IO，或列表 key 不稳定导致全量重建。
