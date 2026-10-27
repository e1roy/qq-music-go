# Code Map

本仓库的项目地图（codemap）：目录总览、各包文件职责与关键符号、按任务快速定位。从 `AGENTS.md` 第 4 节拆出，单独维护；**新增/删除文件、新增页面或接口后同步更新本文件**。

找「要改什么」看第 6 节，找某个文件或符号看对应包的表格。

## 1. 目录总览

```text
.
├── cmd/
│   ├── qqmusic/      # 可执行入口 main：crash.log、诊断开关、app.Run
│   ├── genicon/      # SVG → PNG/ICO/.syso 图标生成
│   ├── probe/        # 命令行接口验证（线上冒烟测试）
│   └── zz_feeddump/、zz_podcast/  # 一次性接口探针，带 `//go:build probe`，平时不编译
├── internal/
│   ├── app/          # 应用状态、页面加载编排、播放编排、登录流程、界面（展示层 + 编排层）
│   ├── playqueue/    # 播放队列规则（纯数据结构，无 IO）
│   ├── store/        # 应用数据目录中的 JSON 文件读写
│   ├── qqmusic/      # QQ 音乐接口客户端（HTTP、登录、LRC、类型）
│   ├── xiaoyuzhou/   # 小宇宙官方接口客户端（扫码登录、发现、榜单、订阅、搜索、节目网页）
│   ├── player/       # 边下载边解码的播放器（MP3 / FLAC / AAC）
│   └── diag/         # 内存采样页 + net/http/pprof
├── resources/        # 图标源文件与嵌入资源
├── mygo.json         # 应用名、identifier、版本号、入口、输出目录
├── 启动run.bat       # 开发模式启动（go tool mygo dev）
└── 打包build.bat     # 打包 Portable + Setup
```

## 2. `internal/app`（界面与状态，改动最频繁）

| 文件 | 职责 | 关键符号 |
|---|---|---|
| `app.go` | `app` 结构体（各区域状态的所有者）、`platform` 接口（含 `OpenFolder`）、`newApp`、后台任务入口与错误文案 | `app`、`platform`、`cookie`、`newApp`、`do`、`toast`、`errText` |
| `nav.go` | 页面与历史：`page`/`pageKind`，前进后退，`enter` / `refresh` / `refreshing` 三个按页面分派的 switch（相邻排列，新增页面时一起改） | `page`/`pageKind`、`current`、`navigate`、`podcastHome`/`switchPodcasts`、`back`/`forward`、`enter`、`refresh`、`refreshing` |
| `library.go` | QQ 音乐歌曲类页面的加载编排：首页、音乐馆（榜单/广场）、歌单/榜单/专辑（播客单集按页）/歌手，以及卡片上的「播放」 | `songList`、`albumList`、`singerTab`、`loadHome`/`moreRecs`/`moreRadar`、`loadSquare`/`addPlaylists`、`loadCharts`、`loadList`、`listSongs`（歌单/榜单取歌，`loadList` 与 `playPage` 共用）、`playPage`、`loadAlbum`、`loadSinger`/`loadAlbums`/`sortSinger`/`showTab` |
| `search.go` | 搜索：单曲分页、歌手/歌单/播客页签、联想、搜索历史、搜索框作用域 | `searchTab`、`runSearch`、`suggestAsTyped`/`rest`、`searchWords`/`searchStep`/`searchKeys`、`chooseSearch`、`searchScope`（`scopeMusic`/`scopePodcasts`/`scopeXiaoyuzhou`）、`rememberSearch`/`clearSearches`、`showSearchTab`/`loadSearchTab`/`loadSearchPodcasts`、`searchMore`/`searchSingers`/`searchPlaylists`/`addSingers`、`moreXiaoyuzhouPodcastsFound`/`moreFoundPodcasts`（播客页签同时搜小宇宙官方） |
| `session.go` | QQ 音乐会话：启动校验、登录成功、我的歌单、退出、我喜欢 | `startSession`、`loggedIn`、`loadMine`、`setLiked`（重读我喜欢时保留进行中的红心，`app.liking`）、`logout`、`toggleLike` |
| `pager.go` | 分页列表的统一状态与加载：搜索的歌手/歌单/播客、小宇宙（QQ 音乐）的节目/单集搜索、小宇宙官方的各列表。各列表的翻页方式（页码或游标）、结束判定、去重与错误文案留在各自的包装函数里 | `pager`（`items`/`pages`/`more`/`total`/`after`/`list`）、`reset`、`loadPage`；包装：`searchMore`（search.go）、`podcastMore`（podcast.go）、`xiaoyuzhouMore`（xiaoyuzhou.go） |
| `settings.go` | 持久化记录与读写（经 `internal/store`）、有附带规则的设置项方法、「最近使用」列表；读取时把 v1.0.1 的 `xyz_official` 与单集 `Source: "xyz"` 转成新名 | 文件名常量 `file*`（含旧名 `fileOldXiaoyuzhou`）、`settings`（`OldOfficial` 只读不写）、`savedQueue`、`recentMax`、`load`、`saveQueue`/`saveSettings`/`saveCredential`、`setQuality`（模式 `Mode` 在 `load` 中校验范围）、`cacheTiers`/`setCacheGB`/`clearCache`/`measureCache`/`openCacheFolder`（歌曲缓存上限、用量与目录，在 `a.do` 中操作磁盘）、`rememberWindow`、`pushFront`、`addRecent` |
| `playback.go` | 播放编排：队列操作（规则在 `internal/playqueue`）、音源解析、失败跳过、歌词、自动切歌、音量；音质徽章文字 | `nowPlaying`、`playList`、`playRadio`、`moreRadio`、`playNext`、`enqueue`、`removeFromQueue`、`clearQueue`、`playAt`（含按来源取音源）、`qualityLabel`、`loadLyrics`、`togglePlay`、`next`、`prev`、`tick`、`setVolume`、`toggleMute` |
| `podcast.go` | 小宇宙（QQ 音乐内置播客）：状态、专区与推荐加载、换一批、我听过的节目（`podcasts.json`）与已知播客集合（决定 128k 播放）、节目/单集搜索分页 | `podcasts`、`qqEpisode`/`isEpisode`（「节目」的判定，右键菜单与歌词页共用）、`podcastSearch`、`heardMax`、`pageOf`、`loadPodcasts`、`morePicks`、`recentEpisodes`、`hearShow`/`clearHeard`/`saveHeard`、`searchPodcasts`、`showPodcastTab`/`loadPodcastTab`、`moreShows`/`moreEpisodes`/`morePodcastsFound`（搜索页的播客页签）、`podcastMore` |
| `comments.go` | 歌词页旁的歌曲评论：状态、热门/最新分页加载、与播放队列互斥的开关；歌词页打开或切歌时同步当前歌曲的评论 | `comments`、`commentList`、`commentPage`、`commentsShown`、`sidePanel`、`toggleLyrics`/`toggleQueue`/`toggleComments`、`showCommentOrder`、`syncComments`、`loadComments` |
| `login.go` | 登录对话框的流程层：二维码轮询、WebView 登录回调 | `openLogin`、`closeLogin`、`newQR`、`loginWithWebView`、`errLoginClosed` |
| `desktop.go` | `platform` 的真实实现；主窗口、QQ 登录窗口、小宇宙手机号登录窗口、托盘；程序入口（`Run` 里创建歌曲缓存 `player.NewCache`） | `desktop`、`Post`、`OpenQQLogin`、`OpenXiaoyuzhouLogin`、`newTray`、`Run` |
| `view.go` | 根布局、主题与颜色、通用控件、侧边栏（左下角用户行 + 设置齿轮按钮）、顶栏、窗口按钮、封面/格式化 | `col*`、`radius*`、`sidebarWidth`/`barHeight`/`topHeight`/`tintHeight`/`queueWidth`、`theme`、`view`、`fadingPanelCorners`、`topbar`、`windowControls`/`windowButton`、`ghostButton`、`searchBox`/`searchMenu`、`userArea`/`userRow`、`avatar`、`variant`/`button`/`toggle`/`tabs`/`badge`、`sidebar`、`playlistSection`、`navItem`、`cover`/`coverOr`、`formatCount`/`formatSize`/`formatTime` |
| `view_pages.go` | 页面分发与主内容区：首页（含服务端下发标题的推荐信息流模块）、音乐馆（排行榜 / 歌单广场两个页签）、歌单/榜单/专辑/最近播放、歌曲表格、右键菜单 | `content`、`columns`、`pageScroll`、`heading`、`errorRow`、`greeting`、`homePage`、`refreshButton`、`forYou`/`forYouCard`/`homeCover`、`songCards`/`playCircle`/`playlistGrid`/`playlistCard`/`playPage`、`discoverTabs`/`squarePage`、`chartsPage`/`topCharts`、`listPage`/`recentPage`/`listHeader`/`listTint`、`songTable`/`columnTitles`/`songRow`、`singerLinks`/`albumLink`、`equalizer`、`isPlaying`、`songMenu` |
| `view_player.go` | 底部播放栏、播放模式菜单、音质弹窗、进度/音量滑块、歌词页、播放队列面板 | `iconButton`、`playerBar`、`modeIcons`/`modeButton`、`timeText`、`thinSlider`、`progress`、`nowPlayingPage`、`lyricPad`/`lyricsView`、`queuePanel` |
| `view_comments.go` | 歌词页的「评论」页签与右侧评论面板（与队列共用右侧槽位）：热门/最新两个页签、评论行、滚到底加载下一页 | `commentsTab`、`commentsPanel`、`commentsCard`、`emptyComments`、`commentsEnd`、`commentRow`、`commentStat`、`formatAgo` |
| `view_podcast.go` | 小宇宙页：最近收听、为你推荐（换一批）、我听过的节目、专区楼层（单集/主播/节目）、搜索结果（由顶栏搜索框发起，见 `chooseSearch`）；节目页（`pageAlbum` 且 `IsPodcast`）的单集表格与滚到底分页 | `podcastsPage`、`podcastCover`、`podcastResults`、`podcastEnd`、`picks`、`showGrid`/`showCard`、`hostGrid`、`showPage`、`episodeTitles`、`episodeRow`、`episodesEnd` |
| `xiaoyuzhou.go` | 小宇宙官方（直连小宇宙官方接口，不是 QQ 音乐内置的小宇宙）：状态、`xiaoyuzhou.json` 持久化（启动时把 v1.0.1 的 `xyz.json` 改名迁移）、扫码登录与轮询、推荐/发现/订阅更新/我的订阅/搜索的加载、分类页、节目页（未登录或接口失败时回退网页版）；单集转成 `Song`（`Source=xiaoyuzhou`，`URL` 为音频直链） | `xiaoyuzhouState`、`xiaoyuzhouSource`/`oldXiaoyuzhouSource`、`fromQQ`/`isXiaoyuzhou`（曲目来源谓词）、`xiaoyuzhouTabFeed`…`xiaoyuzhouTabSubs`、`xiaoyuzhouCharts`/`xiaoyuzhouPicked`、`xiaoyuzhouChart`、`xiaoyuzhouFeed`、`xiaoyuzhouCategory`、`xiaoyuzhouShow`、`savedXiaoyuzhou`、`episodeSong`/`episodeSongs`、`albumOf`、`loadXiaoyuzhou`/`saveXiaoyuzhou`/`startXiaoyuzhou`、`xiaoyuzhouErr`/`logoutXiaoyuzhou`、`enterXiaoyuzhou`、`newXiaoyuzhouQR`/`pollXiaoyuzhouQR`/`xiaoyuzhouPhoneLogin`/`xiaoyuzhouLoggedIn`、`showXiaoyuzhouTab`/`showXiaoyuzhouChart`/`loadXiaoyuzhouTab`、`refreshXiaoyuzhou`（重置当前页签后重载，旧内容保留到新数据到达）/`xiaoyuzhouLoading`、`loadXiaoyuzhouChart`、`loadXiaoyuzhouFeed`、`loadXiaoyuzhouCategory`/`showXiaoyuzhouCategoryTab`/`moreXiaoyuzhouCategory`、`xiaoyuzhouMore`、`moreXiaoyuzhouPicks`/`moreXiaoyuzhouInbox`/`moreXiaoyuzhouSubs`、`searchXiaoyuzhou`/`showXiaoyuzhouSearchTab`/`moreXiaoyuzhouShows`/`moreXiaoyuzhouEpisodes`、`loadXiaoyuzhouShow`/`moreXiaoyuzhouShowEps` |
| `view_settings.go` | 设置页（`pageSettings`）：左侧分区菜单、右侧内容（后台管理式布局）；「缓存」分区：已用空间与上限进度条、缓存目录、打开文件夹、清空、上限档位 | `settingsSections`、`settingsPage`、`settingsCard`、`cacheSettings` |
| `view_xiaoyuzhou.go` | 小宇宙官方页：标题与账号、扫码/手机号登录卡片、推荐（分类探索 + 首页信息流各模块，单集为卡片）/发现（最热/锋芒/新星榜、编辑精选）/订阅更新/我的订阅四个页签、搜索结果（节目/单集）、单集行、节目卡片、分类页、节目页；分页列表滚到底自动加载下一页（`pagerEnd`/`moreRow`，用 `InView`） | `xiaoyuzhouPage`、`xiaoyuzhouCover`、`xiaoyuzhouHeader`、`xiaoyuzhouLogin`、`xiaoyuzhouFeedTab`/`entrySongs`、`chartPills`/`xiaoyuzhouDiscoverTab`、`xiaoyuzhouPicks`、`xiaoyuzhouEpisodeCards`、`xiaoyuzhouCategoryPage`、`xiaoyuzhouResults`、`pagerEnd`/`moreRow`、`xiaoyuzhouEpisodes`/`xiaoyuzhouEpisodeRow`、`xiaoyuzhouShowGrid`/`xiaoyuzhouShowCard`（按需带来源徽章）、`xiaoyuzhouShowPage`/`xiaoyuzhouShowHeader` |
| `view_login.go` | 登录对话框视图：QQ 扫码 / 微信扫码 / QQ 账号三个页签 | `loginDialog`、`qrPane`、`accountPane` |
| `view_search.go` | 搜索页：热门搜索、直达卡片（歌手/专辑）+ 单曲/歌手/歌单/播客四个页签；单曲分页，歌手/歌单切到页签时才加载、按页追加；播客在新查询时与单曲并行搜索，登录后小宇宙官方的结果排在前（封面带「小宇宙」徽章） | `searchPage`、`searchHeader`、`directCard`、`searchCover`、`foundSongs`/`foundSingers`/`foundPlaylists`/`foundPodcasts`、`foundGrid`/`foundEnd`/`foundPodcastsEnd`、`singerCard`/`singerSide` |
| `view_singer.go` | 歌手页：头部 + 歌曲/专辑两个页签 + 排序 + 分页加载 | `singerPage`、`singerBar`、`singerSongsEnd`、`albumRow`、`singerAlbumsEnd` |
| `icons.go` | Lucide 风格 SVG 图标集合与应用 Logo | `icLogo`、`icon`/`filledIcon`、`icHome`…`icWeChat`、`icComment`/`icThumb`、`icSettings`/`icDrive`/`icFolder`、`icFlame`/`icTrending`/`icStar` |
| `images.go` | 图片异步下载、缩放、缓存、按帧淘汰；同时缓存图片主色 | `images`、`imageEntry`、`imageBudget`（48 MiB）、`newImages`、`frame`、`get`、`peek`、`color`、`shrink` |
| `palette.go` | 从封面提取主色（median cut + Palette 目标），生成页面顶部渐变色；颜色过渡 | `swatch`、`target`、`paletteTargets`、`dominantColor`、`quantize`、`box`、`hsl`、`tint`、`oklch`、`tintNone`、`tintFade`、`pageTint` |
| `motion.go` | 页面/歌词/队列的进入退出动画、歌手页页签行级动画、首页「换一批」卡片刷新动画 | `pageOut`/`pageSpring`、`lyricsOut`/`lyricsSpring`、`queueOut`/`panelOut`/`sideSpring`、`appear`、`presence`、`tabMotion`、`cardOut`/`cardSpring`、`refreshMotion` |
| `skeleton.go` | 各种加载骨架屏 | `skeleton`、`bone`、`bar`、`headingSkeleton`、`songCardsSkeleton`、`forYouSkeleton`、`playlistSkeleton`、`singerSkeleton`、`topChartsSkeleton`、`chipsSkeleton`、`songRowsSkeleton`、`listSkeleton`、`lyricsSkeleton`、`commentsSkeleton` |
| `memory.go` | 延迟把 Go 堆内存归还系统 | `trimmer`、`trimDelay`（3s）、`soon` |
| `app_test.go` | 无窗口 UI 测试与联网页面走查 | `fakePlatform`、`newTestApp`、`settle`、`shot`、`TestPages`、`TestLRC`、`TestModeMenu`、`TestQualityMenu`、`TestSearchSinger`、`TestDisplayName`、`TestRecent`、`TestSearchHistory`、`TestComments` |
| `behavior_test.go` | 离线特征测试（`offlineTransport` 让所有请求立即失败，不访问任何服务）：队列增删、四种模式切歌、失败跳过、音质徽章文字、持久化兼容（`testdata/v1.0.1/` 是 v1.0.1 写出的文件：仍可读、保存后可读回、`xyz.json` 迁移为 `xiaoyuzhou.json`、旧 `xyz_official` 生效、未知播放模式回退为默认）、猜你喜欢未登录时不改动原队列、搜索框作用域 | `newOfflineApp`、`drain`、`episodes`、`TestQueueEdits`、`TestNextPrev`、`TestPlayFailures`、`TestQualityLabel`、`TestPersistedFiles`、`TestPlayRadio`、`TestSearchScope`、`TestLogoutRadio`、`TestLikeRace`（`fakeLikes`：模拟服务端的「我喜欢」，控制读取的返回顺序）、`TestShowLabel`、`TestFrameBudget`（静止页不重建、播放中每秒约 4 次、歌词页约 10 次）、`TestPlayFromCache`（命中缓存不取播放地址、显示当时的实际音质、未登录不命中）、`TestSettings`（齿轮进入设置页而不弹用户菜单、改上限并保存、清空缓存）、`TestDownloadName`（下载文件名带歌名与地址的扩展名） |
| `zz_feed_test.go` | 一次性探针：用 `%AppData%` 中的真实凭证请求首页信息流；带 `//go:build probe`，只在 `-tags probe` 时编译 | `TestZZFeed` |
| `podcast_test.go` | 小宇宙页、节目页、搜索结果的离线 UI 测试；我听过的节目跨次启动保存 | `TestPodcasts`、`TestSearchPodcastsTab` |
| `xiaoyuzhou_test.go` | 小宇宙官方的离线 UI 测试：未登录、来源切换、推荐/分类/榜单/编辑精选/订阅、不登录 QQ 音乐播放单集、节目页滚到底加载、`xiaoyuzhou.json` 保存与退出 | `TestXiaoyuzhou` |
| `palette_test.go` | 主色提取与 tint 的单元测试 | `TestDominantColor`、`TestTint` |

## 2.1 `internal/playqueue`（播放队列规则）

| 文件 | 职责 | 关键符号 |
|---|---|---|
| `queue.go` | 队列、当前位置、随机顺序、电台标记；四种模式下的下一首/上一首；增删时维护当前位置。无 IO，由 `app/playback.go` 编排播放器与保存 | `Mode`（`Loop`/`One`/`Shuffle`/`Order`，数值即 settings.json 的 `mode`）、`Step`（`None`/`Play`/`Replay`/`Stop`）、`Queue`（零值即空队列）、`Restore`（复制传入的切片）、`Songs`/`Cur`/`Current`、`Radio`/`SetRadio`、`Replace`、`Select`、`InsertNext`/`Append`/`AppendMissing`、`Remove`、`Clear`、`NearEnd`、`Next`、`Prev` |
| `queue_test.go` | 表驱动单测：增删、续杯去重、四种模式、随机环绕、零值、不改动传入的切片 | `TestEdits`、`TestZero`、`TestOwnSongs`、`TestNearEnd`、`TestNext`、`TestShuffle` |

## 2.2 `internal/store`（本地文件）

| 文件 | 职责 | 关键符号 |
|---|---|---|
| `store.go` | 数据目录中的 JSON 文件：读、写（目录 0700、文件 0600，缩进或紧凑；先写 `.tmp` 再改名，崩溃不留截断文件；不 Sync，因为在 UI 线程上每次切歌都写）、删；路径为空时什么都不做（测试用）；写失败静默 | `Dir`、`Open`、`Load`、`Save`、`Remove` |

## 3. `internal/qqmusic`（接口层）

| 文件 | 职责 | 关键符号 |
|---|---|---|
| `client.go` | HTTP 传输与 `musicu.fcg` 协议：公共参数、签名、Cookie、批量调用、错误码映射、图片下载 | `Client`、`New`、`Credential`/`SetCredential`/`LoggedIn`、`hash33`、`Req`、`comm`、`CallMany`、`Call`、`get`/`Fetch`、`flexInt`、`APIError`、`ErrNotLoggedIn`、`Platform`（`Web`/`Desktop`） |
| `api.go` | 所有业务接口与标签常量 | `Search`（含直达 `parseDirect`）/`SearchSingers`/`SearchPlaylists`/`Suggest`/`HotKeys`、`Toplists`/`ToplistDetail`、`RecommendPlaylists`/`PlaylistsByCategory`/`PlaylistDetail`/`AllPlaylistSongs`、`SingerDetail`/`SingerSongs`/`SingerAlbums`/`Album`/`AlbumSongs`/`AlbumDetail`（按 mid 或 id；播客只取最新 `PodcastPage` 集）、`PodcastArea`（播客专区楼层，`rawArea`/`rawCard`）/`PodcastPicks`（getRadioList 推荐）/`SearchPodcasts`/`SearchEpisodes`（搜索结果经一次批量 `GetAlbumDetail` 过滤出播客，`podcasts`）、`NewSongs`、`Lyric`、`Comments`/`CommentOrder`/`cleanComment`、`SongURL`/`ErrNoSource`、`Profile`、`CreatedPlaylists`/`FavPlaylists`/`LikeSong`、`GuessYouLike`/`DailySongs`/`RadarSongs`、`RecommendFeed`（手机端首页信息流的歌曲/歌单模块，`appComm` 请求，歌曲经 `CgiGetTrackInfo` 补全）、`PlaylistCategories`、`Order` |
| `login.go` | 凭证模型、二维码登录（QQ/微信）、WebView Cookie 登录、刷新与登出 | `Credential`、`NeedsRefresh`、`QR`、`QRKind`、`QRState`、`NewQR`、`PollQR`、`loginQQSig`、`LoginQQCookies`、`loginWeChat`、`acceptLogin`、`Refresh`、`Logout`、`QQLoginPage`、`LoginDoneURLPrefix` |
| `api_test.go` | 直达（zhida）解析、评论文本清理、播客专区楼层解析的单元测试 | `TestParseDirect`、`TestCleanComment`、`TestPodcastArea` |
| `lyric.go` | LRC 解析、翻译合并、按时间取行 | `LyricLine`、`ParseLRC`、`parseLRCLines`、`LineAt` |
| `types.go` | 数据模型与展示辅助 | `Song`/`Singer`/`Album`/`File`/`Pay`、`Playlist`、`FeedShelf`、`Toplist`/`ToplistGroup`、`SingerInfo`、`FoundSinger`、`Direct`/`DirectKind`、`AlbumInfo`（`IsPodcast`）、`Episode`/`EpisodeSongs`、`Show`、`PodcastArea`/`PodcastShelf`、`User`、`Comment`/`CommentReply`、`Quality`（`AAC`/`Standard`/`High`/`Lossless`/`Best`）、`File.Best`/`File.Size`（各音质文件大小）、`songFile`/`File.files`（音源降级顺序）、`AlbumCover`/`SingerPhoto`、`Artists`/`DisplayName`/`VIP`/`Cover`/`Duration` |

## 3.1 `internal/xiaoyuzhou`（小宇宙官方接口）

| 文件 | 职责 | 关键符号 |
|---|---|---|
| `client.go` | `api.xiaoyuzhoufm.com` 传输：iOS App 请求头（`iPhone` 表，版本号在其中）、`x-jike-access-token`、401 时用 refresh token 换一次新 token 并重试；`request` 把 `data` 解码进调用方的类型并返回下一页游标；网页抓取 | `Client`、`New`、`Credential`、`LoggedIn`/`SetTokens`/`Logout`、`OnRefresh`、`Cursor`（`into`）、`APIError`、`ErrNotLoggedIn`、`request`/`send`/`setAppHeaders`/`renew`、`webPage` |
| `login.go` | 扫码登录（与 `accounts.xiaoyuzhoufm.com` 相同，走 `web-api.xiaoyuzhoufm.com`，无需验证码）：生成二维码 PNG、轮询；`CONFIRMED`/`USED` 都算登录成功，token 从响应头或 Cookie 取。手机号登录页地址与 token Cookie 名 | `QR`、`QRState`、`NewQR`、`PollQR`、`postAccounts`、`LoginPage`、`TokenCookie`/`RefreshCookie` |
| `api.go` | 资料、榜单、编辑精选、订阅更新、我的订阅、节目、单集分页、搜索（按 `type` 过滤 HEADER/FOOTER）、付费音频、节目网页 `__NEXT_DATA__`（免登录，约 15 集） | `Profile`、`Chart`、`EditorPicks`、`Inbox`、`Subscriptions`、`Podcast`、`Episodes`、`SearchPodcasts`/`SearchEpisodes`（`search`）、`PrivateMediaURL`、`WebPodcast` |
| `discover.go` | App 首页信息流（按 `loadMoreKey` 逐页读：首页 → `topList` → `discoveryTopic` → `pick`，各模块单独解析，丢掉榜单/横幅/跳转与空模块）与分类（全部分类、分类页签、分类下节目分页） | `Discovery`、`recommended`、`moduleSections`、`Categories`、`CategoryTabs`、`CategoryPodcasts` |
| `types.go` | 数据模型 | `Picture`（`URL(side)`：150/300/800）、`User`、`Podcast`、`Episode`（`AudioURL`、`Paid`）、`ChartKind`/`Chart`、`Pick`/`DayPicks`、`Entry`/`Section`、`Category`（分类与分类页签共用） |
| `xiaoyuzhou_test.go` / `live_test.go` | 离线测试（`service` 假装服务端：搜索过滤与游标、token 续期、首页信息流解析、模型）；联网测试（网页 + 二维码，`-short` 跳过）；`XIAOYUZHOU_CRED=<xiaoyuzhou.json>` 时用真实登录逐个调一次接口 | `TestSearch`、`TestRenew`、`TestDiscovery`、`TestTypes`、`TestWeb`、`TestLive` |

## 4. `internal/player`（播放器）

| 文件 | 职责 | 关键符号 |
|---|---|---|
| `player.go` | 对外播放器：加载（`Source`：URL，或空 URL 从缓存读；`Key`/`Note` 决定下载完是否保留；`Name` 进文件名）、播放/暂停、跳转、音量、缓冲进度；一把锁保护全部状态；按文件头选解码器 | `Player`（`Cache` 字段）、`Source`、`New`、`Load`、`openDecoder`、`Stop`、`Play`、`Pause`、`Playing`、`Loading`、`Loaded`、`Ended`、`Seek`、`Position`、`Duration`、`Buffered`、`Size`、`SetVolume`、`Volume` |
| `buffer.go` | 把 HTTP 响应边下边写进缓存目录的文件并读回（无缓存或写盘失败时进内存，预分配上限 `maxPrealloc`）；`ReadAt` 等待所需字节；下完且该保留时先写 JSON 再标记完成；关闭即取消下载，未保留的文件删除 | `buffer`、`openBuffer`、`openCached`、`fill`、`toMemory`、`dropFile`、`ReadAt`、`whole`、`known`、`Size`、`Progress`、`Close`、`reader`、`maxPrealloc` |
| `cache.go` | 歌曲缓存：`<随机数>-<歌名>-<音质>-<类型>.audio` 加最后写入的 `<key>.json`（不改名，Windows 上打开的文件不能改名）；按最近使用淘汰到上限，单首超过上限 1/4 不保留；跳过正在打开的文件；顺带清理没有 JSON 引用的残留文件 | `Cache`、`NewCache`、`Dir`、`fileLabel`、`SetLimit`、`Clear`、`Size`、`Lookup`、`validKey`、`entry`、`open`、`create`、`release`、`fits`、`keep`、`trim` |
| `decoder.go` | 跳过 ID3、找 MP3 帧、go-mp3 解码、按码率估算偏移实现 seek（先退 `preroll` 再解码并丢弃，避免开头杂音）、坏帧重同步 | `mp3Decoder`、`newMP3Decoder`、`id3End`、`sync`、`frameLen`、`Read`、`SetPosition`、`preroll`、`SampleRate`/`Channels`/`Len` |
| `flac.go` | 边下边解 FLAC（`mewkiz/flac/frame`）：自己读 STREAMINFO；seek 先按文件大小估偏移，再用帧头样本号校正（不依赖 seek table、不等整个文件）；坏帧重同步 | `flacDecoder`、`newFLACDecoder`、`parseFrame`、`fits`、`first`、`sync`、`decodeTo`、`cursor` |
| `aac_windows.go` | AAC（m4a）经 Media Foundation 解码：syscall 调 COM，`stream` → `MFCreateMFByteStreamOnStream` → Source Reader 输出 float，索引到了就开播；所有调用在一个锁定线程（MTA）上执行 | `aacDecoder`、`newAACDecoder`、`serve`/`run`、`open`、`read`/`take`、`comCall`、`errNoMF` |
| `aac_stream_windows.go` | 把下载中的 `buffer` 包成 COM `IStream`（Go 函数经 `syscall.NewCallback` 成为虚表），供 Media Foundation 边下边读；读到未下载处阻塞等待 | `stream`、`newStream`、`streamRead`/`streamSeek`/`streamStat`/`streamQueryInterface`、`statstg` |
| `aac_other.go` | 非 Windows 的 AAC 占位实现（返回不支持） | `newAACDecoder` |
| `decoder_test.go` | 本地生成 FLAC 的逐样本解码/跳转测试；MP3 跳转后无杂音（`testdata/sine.mp3`，ffmpeg 生成的 5 秒双正弦）；联网或本地文件的解码/跳转测试（需 `PLAYER_TEST_URL`，MP3/FLAC/AAC 均可） | `TestFLAC`、`TestMP3Seek`、`TestStreamDecoder`、`serve` |
| `cache_test.go` | 缓存：下完保留并能从文件读回解码；不保留/中途关闭不留文件；按最近使用淘汰、超过 1/4 不保留、清空跳过正在打开的 | `TestCacheKeeps`、`TestCacheDrops`、`TestCacheLimit`、`TestCacheNames`（文件名带歌名、非法字符替换）、`download` |

## 5. 其他

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
| `.agents/skills/local-refactor/` | 重构 skill：开工前检查、阶段产物、本项目分层边界、单元执行循环与验证命令；方法论在 `references/refactor-playbook.md`（原 `REFACTOR.md`） | `local-refactor` |
| `docs/refactor/` | 上一次重构的产物：审计、目标架构、路线图、决策记录、验证报告 | — |
| `mygo.json` | `name` / `identifier` / `version` / `main` / `out`，打包脚本读取版本号 | — |
| `qq音乐分析指南.md` | 推荐信息流（音乐盲盒 / 碎片时间 / 私荐歌单）的数据来源、分析方法、接口字段与实测结果 | — |
| `小宇宙播客接口指南.md` | 小宇宙（Xiaoyuzhou FM）三条通道、鉴权与请求头、接口清单、返回字段、免登录网页通道与对接评估 | — |
| `qq音乐内置小宇宙接口指南.md` | QQ 音乐内置播客（小宇宙内容）的专区/推荐/节目/单集/主播/播放/字幕接口与数据结构，含后续小宇宙模块的实现建议 | — |

## 6. 按任务快速定位

| 要改什么 | 先看 |
|---|---|
| 整体布局、侧边栏、顶栏、窗口按钮 | `internal/app/view.go` |
| 首页、音乐馆（排行榜/广场）、歌单表格 | `internal/app/view_pages.go` |
| 搜索页的页签与结果 | `internal/app/view_search.go` + `search.go`（`runSearch`） |
| 搜索框在哪个页面搜什么（作用域） | `internal/app/search.go` 的 `searchScope` / `chooseSearch` |
| 播放栏、进度/音量、歌词页、队列面板 | `internal/app/view_player.go` |
| 歌曲评论（歌词页页签 + 右侧面板） | `internal/app/view_comments.go` + `internal/app/comments.go` |
| 登录弹窗的显示与三个页签 | `internal/app/view_login.go` + `internal/app/login.go` |
| 歌手页的页签/排序/分页 | `internal/app/view_singer.go` |
| 小宇宙（播客）页、节目页、播客搜索（在小宇宙页时顶栏搜索框只搜播客、不给歌曲联想） | `internal/app/view_podcast.go` + `internal/app/podcast.go`；接口见 `docs/qq音乐内置小宇宙接口指南.md` |
| 小宇宙页的来源切换（QQ音乐/官方徽章与切换按钮） | `internal/app/view.go` 的 `podcastTitle`；`internal/app/nav.go` 的 `podcastHome`/`switchPodcasts`、`settings.XiaoyuzhouOfficial` |
| 小宇宙官方（官方接口、扫码登录、节目页） | `internal/app/view_xiaoyuzhou.go` + `internal/app/xiaoyuzhou.go`；接口在 `internal/xiaoyuzhou`，资料见 `docs/小宇宙播客接口指南.md` |
| 队列规则、切歌、播放模式 | `internal/playqueue/queue.go` |
| 播放编排、音源、失败跳过、音量、音质徽章 | `internal/app/playback.go` |
| 导航、新增页面的分派 | `internal/app/nav.go` |
| 首页/音乐馆/歌单/专辑/歌手的加载 | `internal/app/library.go` |
| 登录后的会话、我喜欢 | `internal/app/session.go` |
| 设置项、持久化文件 | `internal/app/settings.go` + `internal/store` |
| 接口地址/参数/返回字段 | `internal/qqmusic/api.go` + `types.go` |
| 登录协议、扫码轮询、凭证刷新 | `internal/qqmusic/login.go` |
| 缓冲、解码、seek | `internal/player/{buffer,decoder,flac,aac_windows,player}.go` |
| 歌曲缓存（磁盘）、上限与清空 | `internal/player/cache.go`；编排在 `internal/app/settings.go`（`setCacheGB`/`clearCache`/`measureCache`/`openCacheFolder`）；界面在设置页 `view_settings.go`（左下角齿轮进入） |
| 设置页、新增设置分区 | `internal/app/view_settings.go`（往 `settingsSections` 加一项，在 `settingsPage` 的 switch 里分发） |
| 主题色、圆角、间距 | `internal/app/view.go`（`col*`、`radius*`、`theme`） |
| 封面取色与顶部渐变 | `internal/app/palette.go` |
| 图标 | `internal/app/icons.go`；应用图标改 `resources/*.svg` 后重跑 genicon |
| 卡顿/内存 | `internal/app/images.go`、`memory.go`、`internal/diag` |
| 托盘/窗口/QQ 登录窗口 | `internal/app/desktop.go` |
