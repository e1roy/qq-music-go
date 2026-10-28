# AGENTS.md

面向在本仓库工作的 AI 代理与开发者：项目地图（codemap）、硬性约束、开发约定、常用命令与已知坑。改动代码前先读本文件；新增/删除文件后同步更新 `CODEMAP.md`。

快速入口：动手前看 §2 硬性约束；找文件/符号看 `CODEMAP.md` 的「按任务快速定位」；理解数据流看 §5；排查问题看 §8 已知坑；提交前过一遍 §10 检查清单，提交与发布按 §10.1 的顺序执行。

## 1. 项目是什么

用 **纯 Go** 写的 Windows 桌面 QQ 音乐客户端。界面使用 [MyGo](https://github.com/egoist/mygo) 原生 UI（`github.com/egoist/mygo/ui`），**没有 WebView、没有 cgo、没有前端工程**。视觉上是 shadcn/ui 的 Nova 深色风格，主色为 QQ 音乐绿（`#31c27c`）。

- 模块名：`qqmusic`；Go 版本：`1.27.3`（`GOTOOLCHAIN=auto`）
- 平台重点：Windows（托盘、`%AppData%`、`.syso` 图标、NSIS 安装包）
- 数据来源：QQ 音乐网页端**非公开接口** `https://u.y.qq.com/cgi-bin/musicu.fcg` 及若干 `c.y.qq.com` / `c6.y.qq.com` 接口

## 2. 硬性约束

改代码时必须遵守：

1. **纯 Go / 无 cgo**：`CGO_ENABLED=0`。不要引入需要 cgo 或本地动态库的依赖。
2. **主界面只用 MyGo**：不要引入前端工程、Electron、Wails 或 HTML 界面。例外只有两个登录窗口：「QQ 账号」登录（`desktop.OpenQQLogin`，QQ 官方登录页）和小宇宙「手机号登录」（`desktop.OpenXiaoyuzhouLogin`，`accounts.xiaoyuzhoufm.com` 官方登录页，因为短信验证码要过阿里云滑块验证）。只用于登录，不要把它们扩展成应用主界面。
3. **主线程模型**：`app` 的状态只在 UI 线程读写；网络/解码等耗时操作走 `a.do`，结果通过 `platform.Post` 回到主线程。
4. **只在事件回调里做副作用**：`a.view(c)` 可能一帧重建多次，禁止在构建视图时发请求、写文件或改播放队列。只有两个约定的例外，都必须幂等：滚动到列表末尾时调用带 `loading` 防重入的加载方法（`loadList`、`loadComments`、`moreRow` 等），以及每帧的 `a.tick()`（播放结束时切歌；切歌后 `player.Ended()` 立即变为 false）。
5. **播放格式**：MP3 标准 128k（`M500`）/ HQ 320k（`M800`）、无损 FLAC（`F000`，纯 Go `mewkiz/flac`）、AAC m4a（`C600`/`C400`，仅在没有 MP3 时兜底；小宇宙单集大多是 m4a。经 syscall 调 Windows Media Foundation 解码，不用 cgo；文件通过 Go 实现的 `IStream` 边下边读）。格式按文件头识别，不按请求的音质。
6. **播放地址必须登录**：`SongURL` 未登录返回 `ErrNotLoggedIn`；VIP 歌曲还要求账号本身是会员。
7. **接口字段要容错**：数值可能是数字或字符串（用 `flexInt`），简介含 HTML（用 `cleanIntro`），封面可能为空（要回退）。
8. **不要提交隐私与生成物**：`cred.json`、`build/`、`.mygo/`、`mygo_windows_*.syso` 已在 `.gitignore` 中。`build/windows-amd64/resources.go` 是 `mygo build` 生成的，不要手改。
9. **UI 身份**：不要把 `ui.Element` 存进 `app` 状态；元素每次构建都会失效。持久控件用 `.Bind(&handle)`，列表行用稳定的 `c.Key(...)`。
10. **MyGo 版本**：钉在 `v0.7.1`。动 UI 代码前先查本地源码：`go list -m -f '{{.Dir}}' github.com/egoist/mygo`，读 `ui/doc.go` 与相关实现注释。本仓库另有 `.agents/skills/mygo-maintenance/SKILL.md`，其生命周期/身份/线程/测试规则优先。做重构时按 `.agents/skills/local-refactor/SKILL.md` 的流程与验证命令执行，方法论在其 `references/refactor-playbook.md`（原 `REFACTOR.md`）。
11. **测试不能大量请求线上接口**：尤其是需要登录的接口（`SongURL` 的 `music.vkey.GetVkey`、用户歌单等），它们用的是真实账号。批量请求会触发限流（返回 `code 104009`），之后连正常歌曲都拿不到播放地址，客户端也跟着播不了，恢复时间未知。规则：
    - 一次验证只取几首（≤ 10）代表性歌曲，串行请求，不要并发，不要遍历排行榜、歌单或「我喜欢」逐首取播放地址。
    - 想统计音源格式（有没有 FLAC / 只有 AAC 等），只看歌曲信息里的 `File.Size*` 字段，不要逐首调 `SongURL`。
    - 解码/播放测试先把文件下载到本地一次，之后用本地文件反复测（`PLAYER_TEST_URL` 支持本地路径），不要每次重新取地址。
    - 一旦出现 104009，立刻停止请求，告诉用户，不要重试。
12. **视图层不联网、不读写文件**：`view*.go` 中不出现 `a.do(`、`os.`、`json.`，也不调用接口客户端的请求方法；需要时在编排层（`library.go`、`settings.go`、`session.go` 等）加方法。读取登录状态（`a.api.LoggedIn()`、`Credential()`）和在点击回调里 `a.saveSettings()` 是允许的。这是指导原则，§10 的 grep 只是辅助：不要为了通过它而加只转发一次的包装方法。
13. **`xyz` 是参考项目，不是本项目的名字**：它的代码只能参考，不能复制；小宇宙相关的包、文件、标识符统一叫 `xiaoyuzhou`（`internal/xiaoyuzhou`、`xiaoyuzhou.go`、`pageXiaoyuzhou`…），不要再用 `xyz` 命名。`xyz` 只允许出现在兼容旧数据的常量（`fileOldXiaoyuzhou`、`oldXiaoyuzhouSource`、`settings.OldOfficial`）和服务端协议参数（`clientId: "xyz-web"`、CDN 域名 `xyzcdn.net`）里。

## 3. 常用命令

在模块根目录执行（PowerShell）：

| 目的 | 命令 |
|---|---|
| 直接运行 | `go run ./cmd/qqmusic` |
| 热重编译运行 | `go tool mygo dev` |
| 跑全部测试（**会访问线上接口**） | `go test ./...` |
| 只跑不联网的测试 | `go test -short ./...` |
| 一次性探针（`cmd/zz_*`、`internal/app/zz_feed_test.go`，带构建标签） | `go run -tags probe ./cmd/zz_feeddump <cred.json>`；`go test -tags probe -run TestZZFeed ./internal/app`（**用真实账号联网**） |
| 持久化兼容测试（旧版本写的文件仍可读、保存后可读回） | `go test ./internal/app -run TestPersistedFiles`；`testdata/v1.0.1/` 是 v1.0.1 写出的文件，不要重新生成 |
| 静态检查（MyGo 专用） | `go tool mygo vet .` |
| 生成图标（`icon.png` / `icon.ico` / `icon-256.png` / `tray.png`） | `go run ./cmd/genicon` |
| 把 `icon.ico` 写进 exe 资源 | `go run ./cmd/genicon -syso` |
| 本地 GUI 构建 | `go build -ldflags "-H=windowsgui" -o build/QQMusic.exe ./cmd/qqmusic` |
| 正式打包（exe + NSIS 安装包） | `go tool mygo build` |
| 开发模式一键脚本 | `.\启动run.bat`（即 `go tool mygo dev`：改代码保存后自动重编译重启，产物在 `.mygo/dev/`） |
| 打包脚本 | `.\打包build.bat [x.y.z]`（传版本号会先写入 `mygo.json`；不传则用现有版本） |
| macOS 打包并安装 | `./打包build.sh [x.y.z] [--universal] [--dmg] [--no-install] [--no-launch]`（打包 `build/darwin-<arch>/QQMusic.app`，替换 `/Applications/QQMusic.app` 并启动；`--universal` 出通用二进制，`--dmg` 另外生成 dmg） |
| 发布 GitHub Release（先打包，见 §10.1） | `gh release create v<version> build/release/QQMusic_v<version>_Portable.exe build/release/QQMusic_v<version>_Setup.exe --title "QQ音乐桌面版 v<version>" --notes "…"` |
| 接口冒烟测试（公开接口） | `go run ./cmd/probe` |
| 扫码登录后测用户接口/播放地址 | `go run ./cmd/probe -login qq -cred cred.json` |
| 内存诊断页 | `$env:QQMUSIC_PPROF="127.0.0.1:6060"; go run ./cmd/qqmusic`，然后打开 `http://127.0.0.1:6060/mem` |
| UI 截图（测试用） | `$env:SHOTS="C:\shots"; go test ./internal/app -run TestPages -v` |
| 解码器联网测试 | `$env:PLAYER_TEST_URL="<mp3/flac/m4a 的 url 或本地路径>"; go test ./internal/player -run TestStreamDecoder -v` |

打包产物：

- `build/windows-amd64/QQMusic.exe`
- `build/release/QQMusic_v<version>_Portable.exe`
- `build/release/QQMusic_v<version>_Setup.exe`
- `build/darwin-<arch>/QQMusic.app`（macOS，`--dmg` 时同一目录还有 `<slug>-<version>-<arch>.dmg`）

## 4. Code Map

项目地图（目录总览、各包文件职责与关键符号、按任务快速定位）已拆到单独文件维护：[`CODEMAP.md`](CODEMAP.md)。

新增/删除文件、新增页面或接口后，同步更新 `CODEMAP.md`。

## 5. 架构与数据流

### 5.1 分层

```text
cmd/qqmusic (main)
  └── internal/app                       一个包，按文件分层（详见 docs/refactor/target-architecture.md）
        展示层  view_*.go skeleton.go motion.go icons.go palette.go images.go
                只读状态、收集点击、调用编排方法；不联网、不读写文件（§2.12）
        编排层  app.go        app 结构体、platform、do / toast / errText
                nav.go        页面、历史、enter / refresh / refreshing
                library.go    首页、音乐馆、歌单/榜单/专辑/歌手加载、playPage
                search.go     搜索、联想、历史、搜索框作用域
                session.go    QQ 会话、我的歌单、我喜欢；login.go 扫码与 WebView 登录
                playback.go   播放编排：音源解析、失败跳过、歌词、tick
                settings.go   持久化记录与读写、设置项意图方法
                podcast.go xiaoyuzhou.go comments.go notes.go   各区域状态与加载
                desktop.go    platform 实现（窗口、托盘、WebView）
  ├── internal/playqueue   播放队列规则（纯数据，无 IO）
  ├── internal/store       数据目录中的 JSON 文件
  ├── internal/qqmusic     HTTP 接口 + 登录协议 + LRC
  ├── internal/xiaoyuzhou  小宇宙官方接口
  └── internal/player      流式播放（MP3 / FLAC / AAC）
```

依赖方向：视图 → 编排 → `playqueue` / `store` / `qqmusic` / `xiaoyuzhou` / `player`；`playqueue`、`store` 不依赖 UI 与 `app`。

### 5.2 线程与异步

```text
UI 线程：view(c) → 点击/快捷键 → app 方法 → a.do(work)
                                        │
                         goroutine：ctx 30s 超时，work 返回 apply func
                                        │
                         platform.Post(apply) → UI 线程改状态 → 重绘
```

- `a.do` 是唯一推荐的后台入口：`func(ctx context.Context) func()`。
- `a.plat.Post` 把闭包送回主线程；测试里由 `fakePlatform` 收集后手动执行。
- 过期结果用代际号丢弃：`search.gen`、`songList.gen`、`playGen`、`login.gen`。
- 图片下载由 `images.get` 自己起 goroutine（并发上限 6），完成后 `Post`。

### 5.3 导航与页面

- `page{kind, id, dirid, mid, title}` 描述一个页面；`key()` 去掉标题，用于比较与 `lists` 缓存。
- `app.history` / `app.future` 实现前进后退；`navigate` 入栈并清空前进栈。
- `enter(p)` 只在需要时触发首次加载。
- 歌单/榜单/专辑/歌手页的数据放在 `app.lists[page.key()]`；最近播放用 `recentView`。
- 我喜欢是 `page{kind: pagePlaylist, dirid: 201}`，自己的歌单 `id` 可能为 0、用 `dirid` 打开。
- 侧边栏只有一个「小宇宙」入口，按 `settings.XiaoyuzhouOfficial` 打开 `pagePodcast`（QQ 音乐内置）或 `pageXiaoyuzhou`（官方，新装的默认值，见 `newApp`）；还没选过（`settings.XiaoyuzhouChosen` 为假）时打开 `pagePodcastStart`，即 `podcastStartPage` 的来源选择页，选一个后 `choosePodcastSource` 写入设置并原地替换成对应页面（旧版本写的 `xyz_official` 视为已选，见 `load`）。两页标题都由 `podcastTitle` 画：右上角徽章显示「QQ音乐」/「官方」，右侧按钮调 `switchPodcasts`（同样走 `choosePodcastSource`），它原地替换当前页（不入历史栈）并保存设置。
- 设置是 `pageSettings`（左下角用户行右侧的齿轮进入）：左侧分区菜单 `settingsSections`、右侧按 `a.settingsTab` 显示分区，目前只有「缓存」。新增设置分区时往 `settingsSections` 加一项并在 `settingsPage` 里分发，不要再往音质弹窗之类的地方塞设置项。
- 小宇宙官方（原 beta）是 `pageXiaoyuzhou`（页签：推荐/发现/订阅更新/我的订阅），节目页是 `pageXiaoyuzhouShow`（`mid` 为小宇宙 pid），分类页是 `pageXiaoyuzhouCategory`（`mid` 为分类 id）。它直连小宇宙官方接口（`internal/xiaoyuzhou`），和 QQ 音乐内置的 `pagePodcast` 无关；单集作为 `qqmusic.Song` 进队列，`Source == "xiaoyuzhou"`（v1.0.1 写的 `"xyz"` 读取时自动转换）、`URL` 为音频直链（付费单集 `URL` 为空，播放时调 `PrivateMediaURL`），播放不需要登录 QQ 音乐、不走 `SongURL`，也不能加入「我喜欢」。
- 小宇宙是 `pagePodcast`；播客节目复用 `pageAlbum`（有 mid 用 mid，只有 id 时用 `id`），`AlbumInfo.IsPodcast()` 时按页加载单集（`PodcastPage`=30，滚到底再取），不要改回全量循环（有的节目上万集）。
- 小宇宙单集播放时，歌词页没有歌词，改显示单集简介（`notes.go`：`/v1/episode/get` 的 `shownotes`，未登录回退 `WebEpisode` 的网页数据）；评论复用歌词页旁的评论面板（`/v1/comment/list-primary`，需要登录）。
- `app.podcast.known` 里的专辑（播客）单集固定用 128k 播放，见 `playAt`：320k 一小时就是 144 MB；单集不进歌曲缓存。

### 5.4 一次播放的链路

```text
playList / playAt
  → player.Cache.Lookup(mid-quality)  // 歌曲缓存命中（且已登录）就不取播放地址
  → api.SongURL(song, quality)        // 需要登录；FLAC → 320k → 128k → AAC 逐级降级
  → player.Load(Source, duration)     // goroutine 中阻塞到文件开始下载（AAC 要等到索引）
      → openBuffer / openCached       // 边下边写进缓存目录的文件（无缓存或写盘失败时进内存）；命中时直接读文件
      → openDecoder(buffer, duration) // 按文件头：fLaC → flacDecoder，ftyp → aacDecoder，否则 mp3Decoder
      → audio.NewPlayer(decoder)
  → loadLyrics(song)                  // LRC + 翻译
  → 每帧 view → tick()
      → 播放结束自动 next(true)
      → 更新 lyricLine / Buffered / downloaded
```

失败处理：连续失败 `< 3` 次且队列多于 1 首时自动跳过并 toast；否则只提示错误。

### 5.5 登录链路

- QQ 扫码：`newQR` → `api.NewQR(QRQQ)` → 每 1.5s `PollQR` → `QRDone` → `loggedIn`。
- 微信扫码：同上，`QRWeChat`，轮询是长轮询。
- QQ 账号：`loginWithWebView` → `desktop.OpenQQLogin`（WebView2）→ 导航到 `LoginDoneURLPrefix` 后取 `graph.qq.com` Cookie → `LoginQQCookies`。
- `loggedIn()`：保存凭证 → 拉 `Profile` → `loadMine` → `loadHome(true)`。
- 小宇宙扫码（小宇宙官方页内，与 QQ 登录无关）：`newXiaoyuzhouQR` → `xiaoyuzhou.NewQR` → `pollXiaoyuzhouQR` 每 1.5s `PollQR`（离开页面即停）→ `xiaoyuzhouLoggedIn`。
- 小宇宙手机号：`xiaoyuzhouPhoneLogin` → `desktop.OpenXiaoyuzhouLogin`（WebView2 打开 `xiaoyuzhou.LoginPage`）→ 每秒读 `xiaoyuzhoufm.com` 的 `x-jike-access-token`/`x-jike-refresh-token` Cookie → `SetTokens` → `xiaoyuzhouLoggedIn`。不要改成直接调短信接口：`send-code` 必须带阿里云验证码（`captcha`），只有浏览器能过。
- 启动：`startSession()` 检查/刷新凭证 → `Profile` → `loadMine`；过期则清凭证并 toast。

### 5.6 持久化

目录：`os.UserConfigDir()/QQMusicGo`（Windows 即 `%AppData%\QQMusicGo`）。读写都经 `internal/store`，文件名常量与记录类型在 `internal/app/settings.go`；`TestPersistedFiles` 用 `testdata/v1.0.1/`（旧版本写出的文件）确认仍可读；写入先写 `.tmp` 再改名，崩溃不会留下截断的文件。

| 文件 | 内容 | 写入时机 |
|---|---|---|
| `settings.json` | 音量、音质、播放模式、侧边栏折叠、小宇宙来源（`xiaoyuzhou_official`；v1.0.1 写的 `xyz_official` 读取时转换） | 设置变化、窗口关闭 |
| `credential.json` | `qqmusic.Credential` | 登录成功、刷新、退出 |
| `recent.json` | 最近播放（最多 100 首，按 Mid 去重） | 歌曲开始播放 |
| `podcasts.json` | 小宇宙「我听过的节目」（最多 30 个，按节目 mid 去重；也用来在启动时认出哪些专辑是播客） | 播客单集开始播放、清空 |
| `xiaoyuzhou.json` | 小宇宙官方的登录（access/refresh token、设备 ID）与昵称；退出只清 token，设备 ID 保留；v1.0.1 写的 `xyz.json` 启动时改名迁移 | 扫码登录、token 刷新、退出 |
| `queue.json` | 播放队列、当前位置、是否猜你喜欢电台（启动时恢复；没有它时回退为最近播放第一首） | 切歌、队列增删、清空（清空即删除文件） |
| `crash.log` | 崩溃栈 | `debug.SetCrashOutput` |

歌曲缓存不在上面的目录里，而在 `os.UserCacheDir()/QQMusicGo/songs`（即 `%LocalAppData%\QQMusicGo\songs`），由 `internal/player/cache.go` 管理：每次下载写一个 `<随机数>-<歌名>-<实际音质>-<类型>.audio`（如 `2701297910-医生-HQ 320k-mp3.audio`；类型取播放地址的扩展名，QQ 音乐的地址以 `M800….mp3`/`F000….flac`/`C400….m4a` 结尾；歌名经 `fileLabel` 去掉 Windows 不允许的字符并截到 60 字；小宇宙单集没有音质一段；文件名只在开始下载时定，之后不能改），下载完整后再写 `<mid>-<请求音质>.json`（记录文件名、大小和实际音质）。JSON 最后写，所以有 JSON 就说明文件完整；不靠改名，因为 Windows 上打开的文件不能改名。上限由 `settings.CacheGB` 决定（默认 2，0 为不保留；在「设置 → 缓存」里改，入口是左下角的齿轮），单首超过上限 1/4 不保留；超出上限时按最近使用（JSON 的修改时间）淘汰，正在打开的文件不删。播客单集（QQ 内置与小宇宙）也下到文件，但播完即删。

文件权限：目录 `0700`，文件 `0600`。

## 6. 代码约定

### 6.1 注释与命名

- **文档注释用英文**，解释“为什么”而不是“做了什么”；每个包首行有 package 注释。
- **界面文案用中文**（简体），错误提示也通过 `errText` 转成中文。
- 文件名用小写 + 下划线分组：`view_pages.go`、`view_player.go`；视图函数以 `xxxPage`/`xxxBar`/`xxxRow` 命名。
- 颜色变量 `col*`，圆角 `radius*`，尺寸常量放 `view.go` 顶部。
- 新增状态字段放在 `app` 结构体对应分组（Navigation / Pages / Playback / Login）下。

### 6.2 状态与加载

- 每个可加载区域通常有 `loading`、`err`、`loaded`（或 `loaded bool`）。
- **失败保留旧数据**：重新加载失败时不要清空已展示内容；用 `if err == nil` 保护，或参考 `loadList` 里的 `kept` 闭包（它不是公共 helper，只在该函数内定义）。
- 空结果与失败要分开：`len(items) == 0` 且无错误才显示“暂无…”。
- 所有返回 UI 的异步结果都要检查代际号，避免旧请求覆盖新状态。
- 用户能感知的失败用 `a.toast(...)`；页面级失败用 `errorRow(c, msg, retry)`。toast 目前是**有意关闭**的：`view` 每帧丢弃 `a.toasts`、不画出来（会压住播放栏），测试仍读 `a.toasts` 断言文案。不要把它当 bug 恢复；必须让用户看到的失败放在页面里（`errorRow` 等）。

### 6.3 视图

- 复用共享 helper：`view.go` 的 `button`、`toggle`、`tabs`、`badge`、`ghostButton`、`cover`/`coverOr`、`section`、`navItem`；`view_player.go` 的 `iconButton`；`skeleton.go` 的 `skeleton`。
- 列表用 `ui.List` + 稳定 key；行内不要保存元素。
- 滚动容器用 `ui.Scroll(...).TrackScroll(&state)`；页面滚动状态是 `a.main`。
- 动画用 `appear`/`presence`/`tabMotion`；只改变绘制时用 `AnimatePose` + `AnimationFrame`，需要下一帧时用 `c.After`。
- 页面的进场用 `motion.go` 的 `pageTransition`（`ui.ElementTransition` 的 `Enter`），不要用 `appear`：点击改页面发生在某一帧内，一次事件会再建一遍视图，而一帧只保留最后一次 `AnimatePose`，入场姿势会被同帧那次重建吃掉；Transition 在整帧布局后采样。`Enter` 要写 `Opacity: 1`（`ui.Motion` 的 `Opacity` 默认是 0，不写就成了淡入），也不要加 `Exit`：上一页留下的拷贝与新页同时画，两页的文字和卡片对不齐，看着像重影（`TestPageSwitchAnimates`）。
- 歌曲列表的行另有逐行入场（`rowsMotion`：`songTable` 按页面 key、搜索按查询词 arm，行按 0–200ms 依次进场）。页面入场不要把行一起淡掉，否则看不出逐行；`TestRowsComeInOneByOne` 守着这一点。
- `ui.ListState.ScrollTo` 是瞬跳，没有动画可设：歌词换行的上下滚动是 `motion.go` 的 `lyricScroll` —— 跳转前量出下一句离中间多远，跳转那一帧把各行 `Translate` 回原位再滑回去。行内的动画状态靠 `c.Key(i)` 的身份跨帧保留；测量用 `ui.Handle` + `.Bind`（`Handle.Bounds` 给上一帧的布局位置）。
- 页面顶部渐变色由 `a.pageTint(coverURL)` 计算，经 `a.tint.band(tintHeight)` 绘制。

### 6.4 接口

- 新接口写在 `internal/qqmusic/api.go`，返回模型放 `types.go`。
- 单调用用 `c.Call`，多模块合并用 `c.CallMany`；未登录错误统一 `ErrNotLoggedIn`。
- 需要登录的接口先检查 `c.LoggedIn()` 或 `cr.MusicKey`。
- 新接口先加到 `cmd/probe` 做线上验证，再接入 UI。
- 接口随时可能变：解析失败要有兜底，不要因为单个字段缺失就 panic。

### 6.5 测试

- UI 测试用 `ui.NewTester(a.view, 1280, 820)` + `fakePlatform` + `settle`（见 `app_test.go`）。
- 需要触发加载但不能联网时，用 `behavior_test.go` 的 `newOfflineApp`：两个接口客户端的 `HTTP` 换成立即失败的 `offlineTransport`，加载走失败分支。播放失败路径用 `episodes(...)`（指向 `127.0.0.1:1` 的小宇宙单集），不调 `SongURL`。需要接口返回数据时，参考 `TestLikeRace` 的 `fakeLikes`：实现 `http.RoundTripper`，按请求的 `req.method` 返回 `{"req":{"code":0,"data":…}}`。
- 联网测试必须 `if testing.Short() { t.Skip("uses the network") }`。
- 截图通过 `SHOTS` 环境变量输出，`shot()` 会推进一帧再保存。
- 纯逻辑优先单测：`internal/playqueue`、`internal/store`、`TestQualityLabel`、`TestLRC`、`TestDisplayName`、`TestRecent`、`TestDominantColor`、`TestTint`。
- 改共享状态时加 `-race`；改布局/平台行为时实际运行窗口确认。

## 7. 常见改动指引

### 7.1 新增一个页面

1. 在 `nav.go` 的 `pageKind` 增加枚举。
2. 在 `page` 中确认需要的字段（`id`/`dirid`/`mid`）。
3. 在 `app` 中增加该页面的状态（如需加载）。
4. 在 `nav.go` 的 `enter(p)` 中按需触发首次加载，并在相邻的 `refresh` / `refreshing` 中加分支（漏掉 `refreshing` 只会让刷新按钮不转，不易发现）。
5. 在 `view_pages.go` 的 `content` 中分发到新的 `xxxPage(c)`。
6. 如需入口，在 `view.go` 的 `sidebar` 增加 `navItem`。
7. 更新 `CODEMAP.md` 的 codemap。

### 7.2 新增一个接口

1. `internal/qqmusic/api.go` 增加方法；模型放 `types.go`。
2. 用 `c.Call`/`c.CallMany`，注意 `Web` 与 `Desktop` 平台差异。
3. 错误映射：未登录 → `ErrNotLoggedIn`；无音源 → `ErrNoSource`。
4. 在 `cmd/probe/main.go` 增加调用并打印关键字段。
5. 在对应的编排文件（`library.go`、`search.go`、`session.go`、`playback.go` 等）中通过 `a.do` 调用，结果回到主线程再落状态；不要在 `view*.go` 中调用。

### 7.3 新增图标

- 通用图标：在 `internal/app/icons.go` 用 `icon(...)` 或 `filledIcon(...)` 添加 Lucide 风格路径。
- 应用/托盘图标：改 `resources/logo.svg`、`resources/tray.svg`，执行 `go run ./cmd/genicon`；需要 exe 图标再执行 `go run ./cmd/genicon -syso`。

### 7.4 调整主题

- 颜色、圆角、间距集中在 `view.go` 顶部的 `col*`、`radius*` 与 `theme()`。
- 页面顶部渐变与封面取色在 `palette.go`（`tint` 的 Oklch 夹取范围、`tintFade` 时长）。
- 改完用 `SHOTS` 截图走查 `TestPages`，确认深色对比度。

### 7.5 新增一个设置项

1. `settings.go` 的 `settings` 增加字段（带 `json` tag）；旧文件没有该字段时读出零值，确认零值是合理的默认值。
2. 默认值放 `newApp`。
3. 修改处调用 `a.saveSettings()`；有附带规则（提示、去重等）时在 `settings.go` 加方法，参考 `setQuality`。
4. 如需在启动时生效，在 `newApp` 或 `Run` 中应用。
5. 折叠状态这类纯 UI 设置可直接存 `settings`（参考 `MineClosed`/`FavClosed`）。

### 7.6 调试内存/卡顿

- `QQMUSIC_PPROF=127.0.0.1:6060` 启动后访问 `/mem`；`/mem.json` 可取原始采样；`POST /mem/gc` 主动回收。
- 有歌曲缓存时（正式运行都有），`buffer` 把文件写到磁盘再读回，歌曲本身不占 Go 堆，由系统文件缓存承担；只有没有缓存（测试里 `Player.Cache == nil`）或写盘失败时才整首放进内存（MP3 约 4–12 MB，FLAC 约 25–60 MB），按 `Content-Length` 预分配最多 `maxPrealloc`（32 MB）。播放完成后 `trimmer` 延迟 3s 归还。
- 图片缓存上限 48 MiB，按最近使用帧淘汰；封面尺寸由 `cover` 的 `side` 决定（≤64 用 120px，否则 300px）。
- 卡顿优先看是否在 `view` 中做了同步 IO，或列表 key 不稳定导致全量重建。

## 8. 已知坑

1. **搜索接口会随机返回 2001**：`search()` 已内置重试（2 次，间隔 400ms），不要删。同时发起的搜索越多越容易 2001，所以搜索页的歌手/歌单只在切到对应页签时才加载（`loadSearchTab`），不要改回一次全发。唯一例外是「播客」页签：按用户要求，新查询时在单曲之后立即并行发起（`loadSearchPodcasts`），单曲先显示；该页签在登录后还会并行搜索小宇宙官方（`moreXiaoyuzhouPodcastsFound`），结果排在 QQ 音乐的前面。不要再给其他页签加并行。
2. **我喜欢（dirid 201）写入会被网页端参数拒绝（80105）**：`LikeSong` 特意使用 `ct=11, cv=13020508` 的 app comm，不要“简化”成 `Web`。
3. **`Fetch` 会把 `http://` 升级为 `https://`**：封面/音频地址都依赖这一点，不要绕过。
4. **`player.Load` 会阻塞到文件开始下载**（AAC 要等 MP4 的索引 `moov` 下完：在文件头时几百 KB，在文件尾时等于整个文件）：必须在 `a.do` 的 goroutine 中调用，不能在 UI 线程直接调用。
5. **播放地址约一天后失效**：不要长期缓存 `SongURL` 结果。
6. **请求太多会被限流（104009）**：短时间内大量调用 `music.vkey.GetVkey` 后，所有歌曲的播放地址都返回 `code 104009`，包括之前能播的歌。测试时的请求量限制见 §2 第 11 条。
7. **音质降级是预期行为**：`File.files` 只在 `SizeFlac`/`Size320 > 0` 时请求无损/HQ，并总是带上 128k 与 AAC 兜底；UI 会显示“无损不可用”“无高品质”或“无 MP3 音源”；选“最高音质”（`Best`，默认）时只显示实际拿到的音质，不提示降级。音质菜单里的文件大小来自歌曲信息的 `File.Size*`，不额外请求接口。无损通常要求会员。
8. **AAC 走 Media Foundation**：只在 Windows amd64/arm64 上可用（`aac_windows.go`），Windows N 版缺媒体功能包时报 `errNoMF`。文件经 `aac_stream_windows.go` 的 Go `IStream`（`syscall.NewCallback`）交给它，读到未下载的部分会阻塞等待；`Close` 必须先关下载再释放 reader，否则阻塞中的读会让解码线程卡死。Media Foundation 不裁掉 AAC 的编码器前置延迟（约 2112 个样本，≈48ms），可以忽略。不要为此引入 cgo 或纯 Go 的 AAC 解码库。
9. **`mewkiz/flac` 的 `Frame.SampleNumber()` 对定长分块流的最后一帧是错的**（用的是该帧自己的 BlockSize）；`flacDecoder.first` 用 STREAMINFO 的块大小计算，不要换回去。也不要用 `flac.Stream.Seek`：没有 seek table 时它会读完整个文件。
10. **`go test ./...` 会访问线上接口**：日常用 `go test -short ./...`；`TestPages`/`TestSearchSinger` 依赖网络与接口现状。
11. **exe 图标**：`go build` 只会链接 `cmd/qqmusic/mygo_windows_*.syso`；先跑 `go run ./cmd/genicon -syso`。`mygo build` 会自己处理图标与版本信息。
12. **`-H=windowsgui`**：不带它会在 GUI 程序旁弹出控制台窗口。
13. **不要手改生成物**：`build/`、`build/windows-amd64/resources.go`、`cmd/qqmusic/mygo_windows_*.syso`。
14. **Windows 中文脚本**：`.bat` 使用 UTF-8 + `chcp 65001`，编辑时保持编码。
15. **接口是非公开的**：字段可能随时变动；新增解析要允许空值，失败时走 `errText` 而不是 panic。
16. **小宇宙官方的 m4a 边下边播**：实测 1h16m 的单集 0.15s 开播（索引在文件头）。整集写到缓存目录的临时文件（60–120 MB），不占 Go 堆，播完删除。小宇宙的 `/v1/episode-transcript/get` 对本客户端返回的是与节目无关的占位文字稿，不要接成歌词。
17. **歌曲缓存命中不取播放地址**：键是 `mid-请求音质`（不是实际音质），实际音质存在 JSON 的 `note` 里，用来显示音质徽章。命中同样要求已登录（与 `SongURL` 一致）。不要缓存播放地址（约一天失效），只缓存文件。改 `buffer` 时保留两点：读到未下载处阻塞等待（AAC 的 `IStream` 依赖它）；下载完成时先写 JSON 再标记 `done`，否则刚下完就重播会查不到。
18. **MP3 跳转要 preroll**：MP3 帧会借用前面帧的数据，从目标帧直接解码，开头几帧是很响的杂音。`mp3Decoder.SetPosition` 先退 `preroll`（4×1152 个样本）再解码，丢掉多出的部分；不要删，`TestMP3Seek` 会拦住。

## 9. MyGo 维护要点（摘自 `.agents/skills/mygo-maintenance/SKILL.md`）

- 视图的 `*ui.Context` 生命周期与窗口一致；不要把 Context 存进 app 状态或后台 goroutine。
- 元素与部件在下次构建前失效；只存 app 数据、控件状态与 `ui.Handle`，不要存 `ui.Element`。
- 持久控件用 `.Bind(&handle)`；焦点用 `handle.Focused(c)` / `FocusWithin(c)` 读取；需要主动聚焦用 `.FocusBind`。
- 有状态控件与列表项在构造前给稳定 `c.Key`；容器 `.Key` 必须在子元素之前。
- `Changed`/`Submitted` 在下一帧才可见；绑定值直接绑到持久字段，派生值在 `OnClick` 中回写。
- 慢 IO 与耗时计算放后台，通过 `Window.Update` 发布；丢弃过期任务结果。
- 绘制回调只做绘制，可能重复执行。
- 验证：`ui.NewTester` 复现 UI 问题；`go tool mygo vet .`；`CGO_ENABLED=0 go test ./...`；改共享状态加 `-race`。
- MyGo 大版本/次版本升级（`0.x` 到 `0.y`）前读 `docs/ui/migration.md`，先 `go tool mygo migrate-ui .` 预览，再 `-write` 应用并人工复核。

## 10. 提交前检查清单

- [ ] `go test -short ./...` 通过
- [ ] `go vet ./...` 与 `go tool mygo vet .` 通过
- [ ] 视图层不联网、不读写文件（§2.12），辅助检查无输出：`grep -nE 'a\.do\(|\bos\.|\bjson\.|\b(a|x)\.(xiaoyuzhou\.)?api\.[A-Za-z]+\(' internal/app/view*.go | grep -vE 'api\.(LoggedIn|Credential)\(\)'`
- [ ] 涉及网络/播放/登录时，手动运行确认
- [ ] 没有在 `view` 构建期引入副作用
- [ ] 新页面/接口已更新 `CODEMAP.md`
- [ ] 未提交 `cred.json`、`build/`、`*.syso` 等生成物
- [ ] 用户可见文案为中文，代码注释为英文
- [ ] 已用 `.\打包build.bat` 生成 `build/release/` 下的两个 exe（§10.1）
- [ ] 已 push 代码与 tag，并创建 GitHub Release、上传两个 exe（§10.1）

### 10.1 提交与发布（每次提交都按这个顺序）

用户要求：**提交时必须先打包 exe，再推送远程，最后发一个带附件的 GitHub Release**。只 `git push`、甚至只推 tag 都不算完成。

1. **定版本**：改 `mygo.json` 的 `version`，或直接 `.\打包build.bat x.y.z`（会先把版本写进 `mygo.json` 再打包）。版本号 `x.y.z`，tag 为 `vx.y.z`，两者必须一致。
2. **打包**：`.\打包build.bat`（不带参数则沿用 `mygo.json` 的版本）。打包前先关掉正在运行的 QQMusic，否则复制 exe 会失败。产物：
   - `build/release/QQMusic_v<version>_Portable.exe`（免安装，单文件即应用）
   - `build/release/QQMusic_v<version>_Setup.exe`（NSIS 安装包）
3. **验证**：`go test -short ./...`、`go vet ./...`、`go tool mygo vet .`，并按 §10 清单过一遍。
4. **提交并推送**：

   ```powershell
   git add -A; git commit -m "..."
   git tag v<version>
   git push; git push origin v<version>
   ```

   已有 `v<version>` tag（例如只是补发 Release）就不要重建或移动它，直接推送并发布。
5. **发 Release**（`gh` 已登录，发布前 `gh auth status` 确认账号是 `e1roy`）：

   ```powershell
   gh release create v<version> build/release/QQMusic_v<version>_Portable.exe build/release/QQMusic_v<version>_Setup.exe --verify-tag --title "QQ音乐桌面版 v<version>" --notes "中文说明"
   ```

   - 标题固定 `QQ音乐桌面版 v<version>`，两条附件缺一不可。
   - `build/` 已被 git 忽略，exe 只能通过 `gh release create/upload` 上传，不要为了让它们入库而改 `.gitignore`。
   - 说明用中文概括本次改动，末句沿用「Portable 免安装，Setup 为 NSIS 安装包；旧数据可直接沿用。」。
   - Release 已存在、只是补传附件时用 `gh release upload v<version> <两个 exe 路径> --clobber`，不要删了重建。

## 11. 代码规范
1、要为一个推测出来的、几乎不会发生的并发或安全边界加复杂度之前，先跟用户讲清楚：具体会出什么问题、概率多大、要多写多少，用户点头了再加。不许为了「理论上更稳」悄悄扩大范围。

2、外面进来的数据只在入口校验一次，往里传就用定好类型的值，不要每一层再判断一遍。

3、只用一次的逻辑不要提前抽成函数，直接写在调用的地方，除非它会复用，或者真藏着一块复杂的东西。

4、能不用 try/catch 就不用，能一个函数写完就别拆。