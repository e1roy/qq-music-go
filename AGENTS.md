# AGENTS.md

面向在本仓库工作的 AI 代理与开发者：项目地图（codemap）、硬性约束、开发约定、常用命令与已知坑。改动代码前先读本文件；新增/删除文件后同步更新第 4 节。

快速入口：动手前看 §2 硬性约束；找文件/符号看 §4.6 快速定位；理解数据流看 §5；排查问题看 §8 已知坑；提交前过一遍 §10 检查清单。

## 1. 项目是什么

用 **纯 Go** 写的 Windows 桌面 QQ 音乐客户端。界面使用 [MyGo](https://github.com/egoist/mygo) 原生 UI（`github.com/egoist/mygo/ui`），**没有 WebView、没有 cgo、没有前端工程**。视觉上是 shadcn/ui 的 Nova 深色风格，主色为 QQ 音乐绿（`#31c27c`）。

- 模块名：`qqmusic`；Go 版本：`1.27.3`（`GOTOOLCHAIN=auto`）
- 平台重点：Windows（托盘、`%AppData%`、`.syso` 图标、NSIS 安装包）
- 数据来源：QQ 音乐网页端**非公开接口** `https://u.y.qq.com/cgi-bin/musicu.fcg` 及若干 `c.y.qq.com` / `c6.y.qq.com` 接口

## 2. 硬性约束

改代码时必须遵守：

1. **纯 Go / 无 cgo**：`CGO_ENABLED=0`。不要引入需要 cgo 或本地动态库的依赖。
2. **主界面只用 MyGo**：不要引入前端工程、Electron、Wails 或 HTML 界面。唯一例外是「QQ 账号」登录：通过 MyGo 的 WebView2 窗口打开 QQ 官方登录页（`desktop.OpenQQLogin`），不要把它扩展成应用主界面。
3. **主线程模型**：`app` 的状态只在 UI 线程读写；网络/解码等耗时操作走 `a.do`，结果通过 `platform.Post` 回到主线程。
4. **只在事件回调里做副作用**：`a.view(c)` 可能一帧重建多次，禁止在构建视图时发请求、写文件或改播放队列。
5. **播放只支持 MP3**：标准 128k（`M500`）与 HQ 320k（`M800`）。只有 AAC（m4a）音源的歌曲会提示无法播放并自动跳过。
6. **播放地址必须登录**：`SongURL` 未登录返回 `ErrNotLoggedIn`；VIP 歌曲还要求账号本身是会员。
7. **接口字段要容错**：数值可能是数字或字符串（用 `flexInt`），简介含 HTML（用 `cleanIntro`），封面可能为空（要回退）。
8. **不要提交隐私与生成物**：`cred.json`、`build/`、`.mygo/`、`mygo_windows_*.syso` 已在 `.gitignore` 中。`build/windows-amd64/resources.go` 是 `mygo build` 生成的，不要手改。
9. **UI 身份**：不要把 `ui.Element` 存进 `app` 状态；元素每次构建都会失效。持久控件用 `.Bind(&handle)`，列表行用稳定的 `c.Key(...)`。
10. **MyGo 版本**：钉在 `v0.6.4`。动 UI 代码前先查本地源码：`go list -m -f '{{.Dir}}' github.com/egoist/mygo`，读 `ui/doc.go` 与相关实现注释。本仓库另有 `.agents/skills/mygo-maintenance/SKILL.md`，其生命周期/身份/线程/测试规则优先。

## 3. 常用命令

在模块根目录执行（PowerShell）：

| 目的 | 命令 |
|---|---|
| 直接运行 | `go run ./cmd/qqmusic` |
| 热重编译运行 | `go tool mygo dev` |
| 跑全部测试（**会访问线上接口**） | `go test ./...` |
| 只跑不联网的测试 | `go test -short ./...` |
| 静态检查（MyGo 专用） | `go tool mygo vet .` |
| 生成图标（`icon.png` / `icon.ico` / `icon-256.png` / `tray.png`） | `go run ./cmd/genicon` |
| 把 `icon.ico` 写进 exe 资源 | `go run ./cmd/genicon -syso` |
| 本地 GUI 构建 | `go build -ldflags "-H=windowsgui" -o build/QQMusic.exe ./cmd/qqmusic` |
| 正式打包（exe + NSIS 安装包） | `go tool mygo build` |
| 运行版一键脚本 | `.\启动run.bat`（会先 `genicon -syso`，再编译启动） |
| 打包脚本 | `.\打包build.bat [x.y.z]`（传版本号会先写入 `mygo.json`；不传则用现有版本） |
| 接口冒烟测试（公开接口） | `go run ./cmd/probe` |
| 扫码登录后测用户接口/播放地址 | `go run ./cmd/probe -login qq -cred cred.json` |
| 内存诊断页 | `$env:QQMUSIC_PPROF="127.0.0.1:6060"; go run ./cmd/qqmusic`，然后打开 `http://127.0.0.1:6060/mem` |
| UI 截图（测试用） | `$env:SHOTS="C:\shots"; go test ./internal/app -run TestPages -v` |
| 解码器联网测试 | `$env:PLAYER_TEST_URL="<mp3 url>"; go test ./internal/player -run TestStreamDecoder -v` |

打包产物：

- `build/windows-amd64/QQMusic.exe`
- `build/release/QQMusic_v<version>_Portable.exe`
- `build/release/QQMusic_v<version>_Setup.exe`

## 4. Code Map

### 4.1 目录总览

```text
.
├── cmd/
│   ├── qqmusic/      # 可执行入口 main：crash.log、诊断开关、app.Run
│   ├── genicon/      # SVG → PNG/ICO/.syso 图标生成
│   └── probe/        # 命令行接口验证（线上冒烟测试）
├── internal/
│   ├── app/          # 全部应用状态、页面、播放队列、登录流程、界面
│   ├── qqmusic/      # QQ 音乐接口客户端（HTTP、登录、LRC、类型）
│   ├── player/       # 边下载边解码的 MP3 播放器
│   └── diag/         # 内存采样页 + net/http/pprof
├── resources/        # 图标源文件与嵌入资源
├── mygo.json         # 应用名、identifier、版本号、入口、输出目录
├── 启动run.bat       # 编译并启动
└── 打包build.bat     # 打包 Portable + Setup
```

### 4.2 `internal/app`（界面与状态，改动最频繁）

| 文件 | 职责 | 关键符号 |
|---|---|---|
| `app.go` | 应用状态、页面与历史、异步任务调度、持久化、各页面数据加载、搜索、会话、我喜欢 | `app`、`page`/`pageKind`、`songList`、`albumList`、`singerTab`、`playMode`、`settings`、`platform`、`newApp`、`do`、`toast`、`errText`、`load`/`saveSettings`/`saveCredential`/`saveQueue`、`savedQueue`、`navigate`/`back`/`forward`/`enter`、`loadHome`/`loadSquare`/`loadCharts`/`loadList`/`loadSinger`/`loadAlbums`/`sortSinger`/`showTab`、`runSearch`/`showSearchTab`/`loadSearchTab`/`searchSingers`/`searchPlaylists`/`searchMore`、`found`/`searchTab`、`startSession`/`loggedIn`/`loadMine`/`logout`/`toggleLike`、`addRecent` |
| `playback.go` | 播放队列与播放控制、歌词加载、自动切歌、音量 | `nowPlaying`、`playList`、`playRadio`、`moreRadio`、`playNext`、`enqueue`、`removeFromQueue`、`clearQueue`、`reshuffle`、`playAt`、`loadLyrics`、`togglePlay`、`next`、`prev`、`shuffleStep`、`tick`、`setVolume`、`toggleMute` |
| `comments.go` | 歌词页旁的歌曲评论：状态、热门/最新分页加载、与播放队列互斥的开关；歌词页打开或切歌时同步当前歌曲的评论 | `comments`、`commentList`、`commentPage`、`commentsShown`、`sidePanel`、`toggleLyrics`/`toggleQueue`/`toggleComments`、`showCommentOrder`、`syncComments`、`loadComments` |
| `login.go` | 登录对话框的流程层：二维码轮询、WebView 登录回调 | `openLogin`、`closeLogin`、`newQR`、`loginWithWebView`、`errLoginClosed` |
| `desktop.go` | `platform` 的真实实现；主窗口、QQ 登录窗口、托盘；程序入口 | `desktop`、`Post`、`OpenQQLogin`、`newTray`、`Run` |
| `view.go` | 根布局、主题与颜色、通用控件、侧边栏、顶栏、窗口按钮、封面/格式化 | `col*`、`radius*`、`sidebarWidth`/`barHeight`/`topHeight`/`tintHeight`/`queueWidth`、`theme`、`view`、`fadingPanelCorners`、`topbar`、`windowControls`/`windowButton`、`ghostButton`、`searchBox`、`userArea`、`avatar`、`variant`/`button`/`toggle`/`tabs`/`badge`、`sidebar`、`playlistSection`、`navItem`、`cover`/`coverOr`、`formatCount`/`formatTime` |
| `view_pages.go` | 页面分发与主内容区：首页、歌单广场、排行榜、歌单/榜单/专辑/最近播放、歌曲表格、右键菜单 | `content`、`columns`、`pageScroll`、`heading`、`errorRow`、`greeting`、`homePage`、`forYou`/`forYouCard`/`homeCover`、`songCards`/`playCircle`/`playlistGrid`/`playlistCard`/`playPage`、`squarePage`、`chartsPage`/`topCharts`、`listPage`/`recentPage`/`listHeader`/`listTint`、`songTable`/`columnTitles`/`songRow`、`singerLinks`/`albumLink`、`equalizer`、`isPlaying`、`songMenu` |
| `view_player.go` | 底部播放栏、播放模式菜单、进度/音量滑块、歌词页、播放队列面板 | `iconButton`、`playerBar`、`modeIcons`/`modeButton`、`timeText`、`thinSlider`、`progress`、`nowPlayingPage`、`lyricPad`/`lyricsView`、`queuePanel` |
| `view_comments.go` | 歌词页的「评论」页签与右侧评论面板（与队列共用右侧槽位）：热门/最新两个页签、评论行、滚到底加载下一页 | `commentsTab`、`commentsPanel`、`commentsCard`、`emptyComments`、`commentsEnd`、`commentRow`、`commentStat`、`formatAgo` |
| `view_login.go` | 登录对话框视图：QQ 扫码 / 微信扫码 / QQ 账号三个页签 | `loginDialog`、`qrPane`、`accountPane` |
| `view_search.go` | 搜索页：热门搜索、直达卡片（歌手/专辑）+ 单曲/歌手/歌单三个页签；单曲分页，歌手/歌单切到页签时才加载、按页追加 | `searchPage`、`searchHeader`、`directCard`、`searchCover`、`foundSongs`/`foundSingers`/`foundPlaylists`、`foundGrid`/`foundEnd`、`singerCard`/`singerSide` |
| `view_singer.go` | 歌手页：头部 + 歌曲/专辑两个页签 + 排序 + 分页加载 | `singerPage`、`singerBar`、`singerSongsEnd`、`albumRow`、`singerAlbumsEnd` |
| `icons.go` | Lucide 风格 SVG 图标集合与应用 Logo | `icLogo`、`icon`/`filledIcon`、`icHome`…`icWeChat`、`icComment`/`icThumb` |
| `images.go` | 图片异步下载、缩放、缓存、按帧淘汰；同时缓存图片主色 | `images`、`imageEntry`、`imageBudget`（48 MiB）、`newImages`、`frame`、`get`、`peek`、`color`、`shrink` |
| `palette.go` | 从封面提取主色（median cut + Palette 目标），生成页面顶部渐变色；颜色过渡 | `swatch`、`target`、`paletteTargets`、`dominantColor`、`quantize`、`box`、`hsl`、`tint`、`oklch`、`tintNone`、`tintFade`、`pageTint` |
| `motion.go` | 页面/歌词/队列的进入退出动画、歌手页页签行级动画 | `pageOut`/`pageSpring`、`lyricsOut`/`lyricsSpring`、`queueOut`/`panelOut`/`sideSpring`、`appear`、`presence`、`tabMotion` |
| `skeleton.go` | 各种加载骨架屏 | `skeleton`、`bone`、`bar`、`headingSkeleton`、`songCardsSkeleton`、`forYouSkeleton`、`playlistSkeleton`、`singerSkeleton`、`topChartsSkeleton`、`chipsSkeleton`、`songRowsSkeleton`、`listSkeleton`、`lyricsSkeleton`、`commentsSkeleton` |
| `memory.go` | 延迟把 Go 堆内存归还系统 | `trimmer`、`trimDelay`（3s）、`soon` |
| `app_test.go` | 无窗口 UI 测试与联网页面走查 | `fakePlatform`、`newTestApp`、`settle`、`shot`、`TestPages`、`TestLRC`、`TestModeMenu`、`TestPlayModes`、`TestSearchSinger`、`TestDisplayName`、`TestRecent`、`TestComments` |
| `palette_test.go` | 主色提取与 tint 的单元测试 | `TestDominantColor`、`TestTint` |

### 4.3 `internal/qqmusic`（接口层）

| 文件 | 职责 | 关键符号 |
|---|---|---|
| `client.go` | HTTP 传输与 `musicu.fcg` 协议：公共参数、签名、Cookie、批量调用、错误码映射、图片下载 | `Client`、`New`、`Credential`/`SetCredential`/`LoggedIn`、`hash33`、`Req`、`comm`、`CallMany`、`Call`、`get`/`Fetch`、`flexInt`、`APIError`、`ErrNotLoggedIn`、`Platform`（`Web`/`Desktop`） |
| `api.go` | 所有业务接口与标签常量 | `Search`（含直达 `parseDirect`）/`SearchSingers`/`SearchPlaylists`/`Suggest`/`HotKeys`、`Toplists`/`ToplistDetail`、`RecommendPlaylists`/`PlaylistsByCategory`/`PlaylistDetail`/`AllPlaylistSongs`、`SingerDetail`/`SingerSongs`/`SingerAlbums`/`AlbumDetail`、`NewSongs`、`Lyric`、`Comments`/`CommentOrder`/`cleanComment`、`SongURL`/`ErrNoSource`、`Profile`、`CreatedPlaylists`/`FavPlaylists`/`LikeSong`、`GuessYouLike`/`DailySongs`、`PlaylistCategories`、`Order` |
| `login.go` | 凭证模型、二维码登录（QQ/微信）、WebView Cookie 登录、刷新与登出 | `Credential`、`NeedsRefresh`、`QR`、`QRKind`、`QRState`、`NewQR`、`PollQR`、`loginQQSig`、`LoginQQCookies`、`loginWeChat`、`acceptLogin`、`Refresh`、`Logout`、`QQLoginPage`、`LoginDoneURLPrefix` |
| `api_test.go` | 直达（zhida）解析、评论文本清理的单元测试 | `TestParseDirect`、`TestCleanComment` |
| `lyric.go` | LRC 解析、翻译合并、按时间取行 | `LyricLine`、`ParseLRC`、`parseLRCLines`、`LineAt` |
| `types.go` | 数据模型与展示辅助 | `Song`/`Singer`/`Album`/`File`/`Pay`、`Playlist`、`Toplist`/`ToplistGroup`、`SingerInfo`、`FoundSinger`、`Direct`/`DirectKind`、`AlbumInfo`、`User`、`Comment`/`CommentReply`、`Quality`、`AlbumCover`/`SingerPhoto`、`Artists`/`DisplayName`/`VIP`/`Cover`/`Duration` |

### 4.4 `internal/player`（播放器）

| 文件 | 职责 | 关键符号 |
|---|---|---|
| `player.go` | 对外播放器：加载、播放/暂停、跳转、音量、缓冲进度；一把锁保护全部状态 | `Player`、`New`、`Load`、`Stop`、`Play`、`Pause`、`Playing`、`Loading`、`Loaded`、`Ended`、`Seek`、`Position`、`Duration`、`Buffered`、`SetVolume`、`Volume` |
| `buffer.go` | 把 HTTP 响应边下边存进内存；`ReadAt` 等待所需字节；关闭即取消下载 | `buffer`、`openBuffer`、`fill`、`ReadAt`、`Size`、`Progress`、`Close`、`reader` |
| `decoder.go` | 跳过 ID3、找 MP3 帧、go-mp3 解码、按码率估算偏移实现 seek、坏帧重同步 | `mp3Decoder`、`newMP3Decoder`、`sync`、`frameLen`、`Read`、`SetPosition`、`SampleRate`/`Channels`/`Len` |
| `decoder_test.go` | 联网解码/跳转测试（需 `PLAYER_TEST_URL`） | `TestStreamDecoder` |

### 4.5 其他

| 路径 | 职责 | 关键符号 |
|---|---|---|
| `internal/diag/diag.go` | 内存采样、事件、`/mem` 页面、`/mem.json`、`POST /mem/gc`、pprof | `Start`、`Event`、`FreeOSMemory`、`goMemory`、`take`、`serveData` |
| `internal/diag/mem.html` | 内存曲线前端（被 `go:embed` 嵌入） | — |
| `internal/diag/procmem_windows.go` / `procmem_other.go` | 读取进程私有内存/提交量（平台实现） | `processMemory` |
| `cmd/qqmusic/main.go` | 把崩溃栈写入 `crash.log`；按 `QQMUSIC_PPROF` 启动诊断；调用 `app.Run()` | `main` |
| `cmd/genicon/main.go` | 从 `logo.svg`/`tray.svg` 渲染 PNG、ICO 与预览图 | `main`、`ico`、`render`、`sheet` |
| `cmd/genicon/syso.go` | 把 `.ico` 编成 Windows COFF 资源对象，供 `go build` 链接 | `iconSyso` |
| `cmd/probe/main.go` | 对线上接口做冒烟验证；支持扫码登录、测播放地址 | `main`、`check` |
| `resources/resources.go` | 嵌入 `logo.svg`、`icon-256.png`、`tray.png` | `LogoSVG`、`WindowIcon`、`TrayIcon` |
| `resources/logo.svg`、`tray.svg` | 图标源文件；改完跑 `go run ./cmd/genicon` | — |
| `mygo.json` | `name` / `identifier` / `version` / `main` / `out`，打包脚本读取版本号 | — |

### 4.6 按任务快速定位

| 要改什么 | 先看 |
|---|---|
| 整体布局、侧边栏、顶栏、窗口按钮 | `internal/app/view.go` |
| 首页、广场、排行榜、歌单表格 | `internal/app/view_pages.go` |
| 搜索页的页签与结果 | `internal/app/view_search.go` + `app.go`（`runSearch`） |
| 播放栏、进度/音量、歌词页、队列面板 | `internal/app/view_player.go` |
| 歌曲评论（歌词页页签 + 右侧面板） | `internal/app/view_comments.go` + `internal/app/comments.go` |
| 登录弹窗的显示与三个页签 | `internal/app/view_login.go` + `internal/app/login.go` |
| 歌手页的页签/排序/分页 | `internal/app/view_singer.go` |
| 队列行为、切歌、播放模式、音量 | `internal/app/playback.go` |
| 页面数据加载、导航、搜索、我喜欢 | `internal/app/app.go` |
| 接口地址/参数/返回字段 | `internal/qqmusic/api.go` + `types.go` |
| 登录协议、扫码轮询、凭证刷新 | `internal/qqmusic/login.go` |
| 缓冲、解码、seek | `internal/player/{buffer,decoder,player}.go` |
| 主题色、圆角、间距 | `internal/app/view.go`（`col*`、`radius*`、`theme`） |
| 封面取色与顶部渐变 | `internal/app/palette.go` |
| 图标 | `internal/app/icons.go`；应用图标改 `resources/*.svg` 后重跑 genicon |
| 卡顿/内存 | `internal/app/images.go`、`memory.go`、`internal/diag` |
| 托盘/窗口/QQ 登录窗口 | `internal/app/desktop.go` |

## 5. 架构与数据流

### 5.1 分层

```text
cmd/qqmusic (main)
  └── internal/app
        ├── view_*.go        只构建 UI、收集点击
        ├── app.go           状态、导航、加载、持久化
        ├── playback.go      播放队列与播放控制
        ├── login.go         登录流程
        ├── desktop.go       platform 实现（窗口、托盘、WebView）
        ├── internal/qqmusic HTTP 接口 + 登录协议 + LRC
        └── internal/player  流式 MP3 播放
```

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

### 5.4 一次播放的链路

```text
playList / playAt
  → api.SongURL(song, quality)        // 需要登录；HQ 不可用时降级
  → player.Load(url, duration)        // goroutine 中阻塞到文件开始下载
      → openBuffer(url)               // 内存缓冲 + 后台下载
      → newMP3Decoder(buffer, duration) // 找帧、go-mp3 解码
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
- 启动：`startSession()` 检查/刷新凭证 → `Profile` → `loadMine`；过期则清凭证并 toast。

### 5.6 持久化

目录：`os.UserConfigDir()/QQMusicGo`（Windows 即 `%AppData%\QQMusicGo`）。

| 文件 | 内容 | 写入时机 |
|---|---|---|
| `settings.json` | 音量、音质、播放模式、侧边栏折叠 | 设置变化、窗口关闭 |
| `credential.json` | `qqmusic.Credential` | 登录成功、刷新、退出 |
| `recent.json` | 最近播放（最多 100 首，按 Mid 去重） | 歌曲开始播放 |
| `queue.json` | 播放队列、当前位置、是否猜你喜欢电台（启动时恢复；没有它时回退为最近播放第一首） | 切歌、队列增删、清空（清空即删除文件） |
| `crash.log` | 崩溃栈 | `debug.SetCrashOutput` |

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
- 用户能感知的失败用 `a.toast(...)`；页面级失败用 `errorRow(c, msg, retry)`。

### 6.3 视图

- 复用共享 helper：`view.go` 的 `button`、`toggle`、`tabs`、`badge`、`ghostButton`、`cover`/`coverOr`、`section`、`navItem`；`view_player.go` 的 `iconButton`；`skeleton.go` 的 `skeleton`。
- 列表用 `ui.List` + 稳定 key；行内不要保存元素。
- 滚动容器用 `ui.Scroll(...).TrackScroll(&state)`；页面滚动状态是 `a.main`。
- 动画用 `appear`/`presence`/`tabMotion`；只改变绘制时用 `AnimatePose` + `AnimationFrame`，需要下一帧时用 `c.After`。
- 页面顶部渐变色由 `a.pageTint(coverURL)` 计算，经 `a.tint.band(tintHeight)` 绘制。

### 6.4 接口

- 新接口写在 `internal/qqmusic/api.go`，返回模型放 `types.go`。
- 单调用用 `c.Call`，多模块合并用 `c.CallMany`；未登录错误统一 `ErrNotLoggedIn`。
- 需要登录的接口先检查 `c.LoggedIn()` 或 `cr.MusicKey`。
- 新接口先加到 `cmd/probe` 做线上验证，再接入 UI。
- 接口随时可能变：解析失败要有兜底，不要因为单个字段缺失就 panic。

### 6.5 测试

- UI 测试用 `ui.NewTester(a.view, 1280, 820)` + `fakePlatform` + `settle`（见 `app_test.go`）。
- 联网测试必须 `if testing.Short() { t.Skip("uses the network") }`。
- 截图通过 `SHOTS` 环境变量输出，`shot()` 会推进一帧再保存。
- 纯逻辑优先单测：`TestLRC`、`TestDisplayName`、`TestRecent`、`TestDominantColor`、`TestTint`。
- 改共享状态时加 `-race`；改布局/平台行为时实际运行窗口确认。

## 7. 常见改动指引

### 7.1 新增一个页面

1. 在 `app.go` 的 `pageKind` 增加枚举。
2. 在 `page` 中确认需要的字段（`id`/`dirid`/`mid`）。
3. 在 `app` 中增加该页面的状态（如需加载）。
4. 在 `enter(p)` 中按需触发首次加载。
5. 在 `view_pages.go` 的 `content` 中分发到新的 `xxxPage(c)`。
6. 如需入口，在 `view.go` 的 `sidebar` 增加 `navItem`。
7. 更新第 4 节 codemap。

### 7.2 新增一个接口

1. `internal/qqmusic/api.go` 增加方法；模型放 `types.go`。
2. 用 `c.Call`/`c.CallMany`，注意 `Web` 与 `Desktop` 平台差异。
3. 错误映射：未登录 → `ErrNotLoggedIn`；无音源 → `ErrNoSource`。
4. 在 `cmd/probe/main.go` 增加调用并打印关键字段。
5. 在 `app.go`/`playback.go` 中通过 `a.do` 调用，结果回到主线程再落状态。

### 7.3 新增图标

- 通用图标：在 `internal/app/icons.go` 用 `icon(...)` 或 `filledIcon(...)` 添加 Lucide 风格路径。
- 应用/托盘图标：改 `resources/logo.svg`、`resources/tray.svg`，执行 `go run ./cmd/genicon`；需要 exe 图标再执行 `go run ./cmd/genicon -syso`。

### 7.4 调整主题

- 颜色、圆角、间距集中在 `view.go` 顶部的 `col*`、`radius*` 与 `theme()`。
- 页面顶部渐变与封面取色在 `palette.go`（`tint` 的 Oklch 夹取范围、`tintFade` 时长）。
- 改完用 `SHOTS` 截图走查 `TestPages`，确认深色对比度。

### 7.5 新增一个设置项

1. `app.go` 的 `settings` 增加字段（带 `json` tag）。
2. 默认值放 `newApp`。
3. 修改处调用 `a.saveSettings()`。
4. 如需在启动时生效，在 `newApp` 或 `Run` 中应用。
5. 折叠状态这类纯 UI 设置可直接存 `settings`（参考 `MineClosed`/`FavClosed`）。

### 7.6 调试内存/卡顿

- `QQMUSIC_PPROF=127.0.0.1:6060` 启动后访问 `/mem`；`/mem.json` 可取原始采样；`POST /mem/gc` 主动回收。
- 播放一首歌会占用约歌曲大小的内存（`buffer` 全量保存）；播放完成后 `trimmer` 延迟 3s 归还。
- 图片缓存上限 48 MiB，按最近使用帧淘汰；封面尺寸由 `cover` 的 `side` 决定（≤64 用 120px，否则 300px）。
- 卡顿优先看是否在 `view` 中做了同步 IO，或列表 key 不稳定导致全量重建。

## 8. 已知坑

1. **搜索接口会随机返回 2001**：`search()` 已内置重试（2 次，间隔 400ms），不要删。同时发起的搜索越多越容易 2001，所以搜索页的歌手/歌单只在切到对应页签时才加载（`loadSearchTab`），不要改回一次全发。
2. **我喜欢（dirid 201）写入会被网页端参数拒绝（80105）**：`LikeSong` 特意使用 `ct=11, cv=13020508` 的 app comm，不要“简化”成 `Web`。
3. **`Fetch` 会把 `http://` 升级为 `https://`**：封面/音频地址都依赖这一点，不要绕过。
4. **`player.Load` 会阻塞到文件开始下载**：必须在 `a.do` 的 goroutine 中调用，不能在 UI 线程直接调用。
5. **播放地址约一天后失效**：不要长期缓存 `SongURL` 结果。
6. **HQ 降级是预期行为**：`SongURL` 在 `Size320 == 0` 时只请求标准音质；UI 会显示“无高品质”。
7. **AAC-only 歌曲不可播放**：会走 `ErrNoSource` 并自动跳过，不要试图用 m4a 解码。
8. **`go test ./...` 会访问线上接口**：日常用 `go test -short ./...`；`TestPages`/`TestSearchSinger` 依赖网络与接口现状。
9. **exe 图标**：`go build` 只会链接 `cmd/qqmusic/mygo_windows_*.syso`；先跑 `go run ./cmd/genicon -syso`。`mygo build` 会自己处理图标与版本信息。
10. **`-H=windowsgui`**：不带它会在 GUI 程序旁弹出控制台窗口。
11. **不要手改生成物**：`build/`、`build/windows-amd64/resources.go`、`cmd/qqmusic/mygo_windows_*.syso`。
12. **Windows 中文脚本**：`.bat` 使用 UTF-8 + `chcp 65001`，编辑时保持编码。
13. **接口是非公开的**：字段可能随时变动；新增解析要允许空值，失败时走 `errText` 而不是 panic。

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
- [ ] `go tool mygo vet .` 无新增告警
- [ ] 涉及网络/播放/登录时，手动运行确认
- [ ] 没有在 `view` 构建期引入副作用
- [ ] 新页面/接口已更新第 4 节 codemap
- [ ] 未提交 `cred.json`、`build/`、`*.syso` 等生成物
- [ ] 用户可见文案为中文，代码注释为英文
