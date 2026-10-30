# 架构与数据流

分层、线程模型、导航、播放与登录链路、持久化。硬性约束见 [`AGENTS.md`](../../AGENTS.md) §2；文件与符号见 [`codemap.md`](codemap.md)。

## 1. 分层

```text
cmd/qqmusic (main)
  └── internal/app                       一个包，按文件分层（详见 docs/refactor/target-architecture.md）
        展示层  view_*.go skeleton.go motion.go icons.go palette.go images.go
                只读状态、收集点击、调用编排方法；不联网、不读写文件（AGENTS.md §2.12）
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

## 2. 线程与异步

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

## 3. 导航与页面

- `page{kind, id, dirid, mid, title}` 描述一个页面；`key()` 去掉标题，用于比较与 `lists` 缓存。
- `app.history` / `app.future` 实现前进后退；`navigate` 入栈并清空前进栈。
- `enter(p)` 只在需要时触发首次加载。
- 歌单/榜单/专辑/歌手页的数据放在 `app.lists[page.key()]`；最近播放用 `recentView`。
- 我喜欢是 `page{kind: pagePlaylist, dirid: 201}`，自己的歌单 `id` 可能为 0、用 `dirid` 打开。
- 设置是 `pageSettings`（左下角用户行右侧的齿轮进入）：左侧分区菜单 `settingsSections`、右侧按 `a.settingsTab` 显示分区，目前只有「缓存」。新增设置分区时往 `settingsSections` 加一项并在 `settingsPage` 里分发，不要再往音质弹窗之类的地方塞设置项。

### 3.1 小宇宙

- 侧边栏只有一个「小宇宙」入口，按 `settings.XiaoyuzhouOfficial` 打开 `pagePodcast`（QQ 音乐内置）或 `pageXiaoyuzhou`（官方，新装的默认值，见 `newApp`）；还没选过（`settings.XiaoyuzhouChosen` 为假）时打开 `pagePodcastStart`，即 `podcastStartPage` 的来源选择页，选一个后 `choosePodcastSource` 写入设置并原地替换成对应页面（旧版本写的 `xyz_official` 视为已选，见 `load`）。两页标题都由 `podcastTitle` 画：右上角徽章显示「QQ音乐」/「官方」，右侧按钮调 `switchPodcasts`（同样走 `choosePodcastSource`），它原地替换当前页（不入历史栈）并保存设置。
- **官方**（原 beta）是 `pageXiaoyuzhou`（页签：推荐/发现/订阅更新/我的订阅），节目页是 `pageXiaoyuzhouShow`（`mid` 为小宇宙 pid），分类页是 `pageXiaoyuzhouCategory`（`mid` 为分类 id）。它直连小宇宙官方接口（`internal/xiaoyuzhou`），和 QQ 音乐内置的 `pagePodcast` 无关；单集作为 `qqmusic.Song` 进队列，`Source == "xiaoyuzhou"`（v1.0.1 写的 `"xyz"` 读取时自动转换）、`URL` 为音频直链（付费单集 `URL` 为空，播放时调 `PrivateMediaURL`），播放不需要登录 QQ 音乐、不走 `SongURL`，也不能加入「我喜欢」。
- **QQ 音乐内置**是 `pagePodcast`；播客节目复用 `pageAlbum`（有 mid 用 mid，只有 id 时用 `id`），`AlbumInfo.IsPodcast()` 时按页加载单集（`PodcastPage`=30，滚到底再取），不要改回全量循环（有的节目上万集）。
- 小宇宙单集播放时，歌词页没有歌词，改显示单集简介（`notes.go`：`/v1/episode/get` 的 `shownotes`，未登录回退 `WebEpisode` 的网页数据）；评论复用歌词页旁的评论面板（`/v1/comment/list-primary`，需要登录）。
- `app.podcast.known` 里的专辑（播客）单集固定用 128k 播放，见 `playAt`：320k 一小时就是 144 MB；单集不进歌曲缓存。

## 4. 一次播放的链路

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

## 5. 登录链路

- QQ 扫码：`newQR` → `api.NewQR(QRQQ)` → 每 1.5s `PollQR` → `QRDone` → `loggedIn`。
- 微信扫码：同上，`QRWeChat`，轮询是长轮询。
- QQ 账号：`loginWithWebView` → `desktop.OpenQQLogin`（WebView2）→ 导航到 `LoginDoneURLPrefix` 后取 `graph.qq.com` Cookie → `LoginQQCookies`。
- `loggedIn()`：保存凭证 → 拉 `Profile` → `loadMine` → `loadHome(true)`。
- 小宇宙扫码（小宇宙官方页内，与 QQ 登录无关）：`newXiaoyuzhouQR` → `xiaoyuzhou.NewQR` → `pollXiaoyuzhouQR` 每 1.5s `PollQR`（离开页面即停）→ `xiaoyuzhouLoggedIn`。
- 小宇宙手机号：`xiaoyuzhouPhoneLogin` → `desktop.OpenXiaoyuzhouLogin`（WebView2 打开 `xiaoyuzhou.LoginPage`）→ 每秒读 `xiaoyuzhoufm.com` 的 `x-jike-access-token`/`x-jike-refresh-token` Cookie → `SetTokens` → `xiaoyuzhouLoggedIn`。不要改成直接调短信接口：`send-code` 必须带阿里云验证码（`captcha`），只有浏览器能过。
- 启动：`startSession()` 每次启动都续期一次凭证（`NeedsRefresh()` 只给探针用，不参与启动判断）→ `Profile` → `loadMine`；过期则清凭证并 toast。续期走 `music.login.LoginServer.Login` + `loginMode: 2`，`refresh_key` 本身也会过期，所以每次启动都换一次是延长免扫码时间的关键。

## 6. 持久化

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

文件权限：目录 `0700`，文件 `0600`。

### 6.1 歌曲缓存

歌曲缓存不在上面的目录里，而在 `os.UserCacheDir()/QQMusicGo/songs`（即 `%LocalAppData%\QQMusicGo\songs`），由 `internal/player/cache.go` 管理：

- 每次下载写一个 `<随机数>-<歌名>-<实际音质>-<类型>.audio`（如 `2701297910-医生-HQ 320k-mp3.audio`）。类型取播放地址的扩展名，QQ 音乐的地址以 `M800….mp3`/`F000….flac`/`C400….m4a` 结尾；歌名经 `fileLabel` 去掉 Windows 不允许的字符并截到 60 字；小宇宙单集没有音质一段；文件名只在开始下载时定，之后不能改。
- 下载完整后再写 `<mid>-<请求音质>.json`（记录文件名、大小和实际音质）。JSON 最后写，所以有 JSON 就说明文件完整；不靠改名，因为 Windows 上打开的文件不能改名。
- 上限由 `settings.CacheGB` 决定（默认 2，0 为不保留；在「设置 → 缓存」里改，入口是左下角的齿轮），单首超过上限 1/4 不保留；超出上限时按最近使用（JSON 的修改时间）淘汰，正在打开的文件不删。
- 播客单集（QQ 内置与小宇宙）也下到文件，但播完即删。
