# 代码约定

注释命名、状态与加载、视图、接口、测试的约定，以及 MyGo 维护要点。硬性约束见 [`AGENTS.md`](../../AGENTS.md) §2，常见改动的步骤见 [`howto.md`](howto.md)。

## 1. 注释与命名

- **文档注释用英文**，解释“为什么”而不是“做了什么”；每个包首行有 package 注释。
- **界面文案用中文**（简体），错误提示也通过 `errText` 转成中文。
- 文件名用小写 + 下划线分组：`view_pages.go`、`view_player.go`；视图函数以 `xxxPage`/`xxxBar`/`xxxRow` 命名。
- 颜色变量 `col*`，圆角 `radius*`，尺寸常量放 `view.go` 顶部。
- 新增状态字段放在 `app` 结构体对应分组（Navigation / Pages / Playback / Login）下。

## 2. 状态与加载

- 每个可加载区域通常有 `loading`、`err`、`loaded`（或 `loaded bool`）。
- **失败保留旧数据**：重新加载失败时不要清空已展示内容；用 `if err == nil` 保护，或参考 `loadList` 里的 `kept` 闭包（它不是公共 helper，只在该函数内定义）。
- 空结果与失败要分开：`len(items) == 0` 且无错误才显示“暂无…”。
- 所有返回 UI 的异步结果都要检查代际号，避免旧请求覆盖新状态。
- 用户能感知的失败用 `a.toast(...)`；页面级失败用 `errorRow(c, msg, retry)`。toast 目前是**有意关闭**的：`view` 每帧丢弃 `a.toasts`、不画出来（会压住播放栏），测试仍读 `a.toasts` 断言文案。不要把它当 bug 恢复；必须让用户看到的失败放在页面里（`errorRow` 等）。

## 3. 视图

- **做功能必须考虑动效**：出现、消失、切换、刷新、加载都要有明确的动效决定（按本文加，或明确不加并说明），按 [`motion.md`](motion.md) 的三条原则与检查清单来；参数集中在 `motion.go` 顶部，不要散在视图里。
- 复用共享 helper：`view.go` 的 `button`、`toggle`、`tabs`、`badge`、`ghostButton`、`cover`/`coverOr`、`section`、`navItem`；`view_player.go` 的 `iconButton`；`skeleton.go` 的 `skeleton`。
- 列表用 `ui.List` + 稳定 key；行内不要保存元素。
- **带菜单的元素必须每帧都建**：`Element.Menu` / `ContextMenu` 的选择是在“选中之后的下一帧、该元素再次构建时”投递的（`buildMenu` 里比对 `chosen`），元素那一刻不存在，选择就永远丢了。悬停才出现的图标按钮要挂菜单，就让它一直建着、用 `Opacity(0)` 藏起来（见 `songCards` 的「更多」）。
- 滚动容器用 `ui.Scroll(...).TrackScroll(&state)`；页面滚动状态是 `a.main`。
- 页面顶部渐变色由 `a.pageTint(coverURL)` 计算，经 `a.tint.band(tintHeight)` 绘制。

### 3.1 动效实现

- 动画用 `appear`/`presence`/`tabMotion`；只改变绘制时用 `AnimatePose` + `AnimationFrame`，需要下一帧时用 `c.After`。
- 页面的进场用 `motion.go` 的 `pageTransition`（`ui.ElementTransition` 的 `Enter`），不要用 `appear`：点击改页面发生在某一帧内，一次事件会再建一遍视图，而一帧只保留最后一次 `AnimatePose`，入场姿势会被同帧那次重建吃掉；Transition 在整帧布局后采样。`Enter` 要写 `Opacity: 1`（`ui.Motion` 的 `Opacity` 默认是 0，不写就成了淡入），也不要加 `Exit`：上一页留下的拷贝与新页同时画，两页的文字和卡片对不齐，看着像重影（`TestPageSwitchAnimates`）。
- 歌曲列表的行另有逐行入场（`rowsMotion`：`songTable` 按页面 key、搜索按查询词换一份，行间 8ms、最多错开 12 行）。页面入场不要把行一起淡掉，否则看不出逐行；`TestRowsComeInOneByOne` 守着这一点。
- `ui.ListState.ScrollTo` 是瞬跳，没有动画可设：歌词换行的上下滚动是 `motion.go` 的 `lyricScroll` —— 跳转前量出下一句离中间多远，跳转那一帧把各行 `Translate` 回原位再滑回去。行内的动画状态靠 `c.Key(i)` 的身份跨帧保留；测量用 `ui.Handle` + `.Bind`（`Handle.Bounds` 给上一帧的布局位置）。

## 4. 接口

- 新接口写在 `internal/qqmusic/api.go`，返回模型放 `types.go`。
- 单调用用 `c.Call`，多模块合并用 `c.CallMany`；未登录错误统一 `ErrNotLoggedIn`。
- 需要登录的接口先检查 `c.LoggedIn()` 或 `cr.MusicKey`。
- 新接口先加到 `cmd/probe` 做线上验证，再接入 UI。
- 接口随时可能变：解析失败要有兜底，不要因为单个字段缺失就 panic。

## 5. 测试

- UI 测试用 `ui.NewTester(a.view, 1280, 820)` + `fakePlatform` + `settle`（见 `app_test.go`）。
- 需要触发加载但不能联网时，用 `behavior_test.go` 的 `newOfflineApp`：两个接口客户端的 `HTTP` 换成立即失败的 `offlineTransport`，加载走失败分支。播放失败路径用 `episodes(...)`（指向 `127.0.0.1:1` 的小宇宙单集），不调 `SongURL`。需要接口返回数据时，参考 `TestLikeRace` 的 `fakeLikes`：实现 `http.RoundTripper`，按请求的 `req.method` 返回 `{"req":{"code":0,"data":…}}`。
- 联网测试必须 `if testing.Short() { t.Skip("uses the network") }`，请求量遵守 `AGENTS.md` §2.11。
- 截图通过 `SHOTS` 环境变量输出，`shot()` 会推进一帧再保存。
- 纯逻辑优先单测：`internal/playqueue`、`internal/store`、`TestQualityLabel`、`TestLRC`、`TestDisplayName`、`TestRecent`、`TestDominantColor`、`TestTint`。
- 改共享状态时加 `-race`；改布局/平台行为时实际运行窗口确认。

## 6. MyGo 维护要点

完整规则在 [`.agents/skills/mygo-maintenance/SKILL.md`](../../.agents/skills/mygo-maintenance/SKILL.md)，与本文冲突时以它为准。

- 视图的 `*ui.Context` 生命周期与窗口一致；不要把 Context 存进 app 状态或后台 goroutine。
- 元素与部件在下次构建前失效；只存 app 数据、控件状态与 `ui.Handle`，不要存 `ui.Element`。
- 持久控件用 `.Bind(&handle)`；焦点用 `handle.Focused(c)` / `FocusWithin(c)` 读取；需要主动聚焦用 `.FocusBind`。
- 有状态控件与列表项在构造前给稳定 `c.Key`；容器 `.Key` 必须在子元素之前。
- `Changed`/`Submitted` 在下一帧才可见；绑定值直接绑到持久字段，派生值在 `OnClick` 中回写。
- 慢 IO 与耗时计算放后台，通过 `Window.Update` 发布；丢弃过期任务结果。
- 绘制回调只做绘制，可能重复执行。
- 验证：`ui.NewTester` 复现 UI 问题；`go tool mygo vet .`；`CGO_ENABLED=0 go test -short ./...`；改共享状态加 `-race`。
- MyGo 大版本/次版本升级（`0.x` 到 `0.y`）前读 MyGo 源码里的 `docs/ui/migration.md`，先 `go tool mygo migrate-ui .` 预览，再 `-write` 应用并人工复核。
