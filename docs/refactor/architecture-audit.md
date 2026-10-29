# 架构审计（阶段一）

> 状态：重构前的基线审计（提交 `f9f87bc`）。文中行号与文件均指重构前的代码；各问题的处理结果见 `refactor-roadmap.md` 进度表。
> 所有行号均指该提交。

## 1. 项目概况

| 项 | 实际情况 |
|---|---|
| 语言 / 运行时 | Go 1.27.3，`CGO_ENABLED=0`，模块名 `qqmusic` |
| UI 框架 | MyGo v0.6.4 原生 UI（`ui.View(a.view)` 即时模式：每帧调用 `a.view(c)` 重建元素树） |
| 构建 | `go build` / `go tool mygo build`（NSIS 打包），`mygo.json` |
| 入口 | `cmd/qqmusic/main.go` → `app.Run()`（`internal/app/desktop.go:273`） |
| 外部依赖 | QQ 音乐非公开 HTTP 接口、小宇宙 App 接口、WebView2（仅登录）、Windows Media Foundation（AAC，syscall） |
| 持久化 | `%AppData%\QQMusicGo\*.json`，共 6 个文件（见 §3.4） |
| 规模 | 非测试 Go 代码约 1.1 万行；其中 `internal/app` 约 7,700 行，占 70% |

## 2. 当前模块职责图

```text
cmd/qqmusic ──► internal/app ─────────────┬──► internal/qqmusic   QQ 音乐协议、模型、LRC（干净，纯数据层）
                │                          ├──► internal/xyz       小宇宙协议、模型（干净，纯数据层）
                │                          ├──► internal/player    下载 + 解码 + 播放（干净，自带锁）
                │                          ├──► internal/diag      内存诊断
                │                          └──► resources          图标
                │
                └ internal/app 是一个包、一个 `app` 结构体，同时承担：
                    · 根状态容器（app.go:192-347，约 120 个字段）
                    · 导航与历史（app.go:512-672）
                    · 各页面数据加载编排（app.go:674-1336，podcast.go，xyz.go，comments.go）
                    · 播放队列规则 + 播放编排（playback.go）
                    · 会话 / 登录 / 我喜欢（app.go:1338-1459，login.go）
                    · 持久化读写（app.go:409-510，podcast.go:177-188，xyz.go:161-183）
                    · 平台实现：窗口、托盘、WebView 登录窗（desktop.go）
                    · 视图（view*.go，约 4,000 行）、动画、骨架屏、图片缓存、取色
```

**结论**：下层三个包（`qqmusic`、`xyz`、`player`）边界清楚，API 小而稳定，内部复杂度（签名、Cookie、COM、解码 seek）都藏得很好，**应当保留，不在本次重构范围内**。复杂度和耦合都集中在 `internal/app`。

## 3. 当前架构细节

### 3.1 依赖关系

```text
view_*.go ──读写──► app 字段（所有状态）
    │  ├─调用──► app 意图方法（navigate / playList / loadXxx / toggleLike …）
    │  ├─直接调用──► a.api.*（LoggedIn / Credential；view_pages.go:467 playPage 里直接请求接口）
    │  ├─直接调用──► a.player.*（查询状态、Seek）
    │  ├─直接调用──► a.saveSettings()（音质、模式、音量、侧栏折叠）
    │  └─每帧调用──► a.tick()（自动切歌、内存回收、歌词行）
app 方法 ──► a.do(work) ──goroutine──► qqmusic / xyz / player ──► Post(apply) ──► 主线程改状态
app 方法 ──► os.ReadFile / os.WriteFile（同步，在主线程）
```

包级依赖没有环；问题是**包内**没有分层：视图、编排、规则、持久化都在同一个包里，彼此可以任意访问字段和方法，依赖方向全靠自觉。

### 3.2 典型流程调用链

**A. 点击歌单卡片上的播放按钮**（数据访问在视图层）

```text
view_pages.go:458 playlistCard → play.Clicked()
  → view_pages.go:467 playPage(pg)              ← 定义在 view 文件中
      → a.do: a.api.ToplistDetail / AlbumDetail / AllPlaylistSongs   ← 与 app.go:882-907 loadList 的分派重复
      → 主线程：a.podcast.known[al.Mid] = true   ← 业务状态在 view 文件里修改
      → playList(songs, 0)
```

**B. 播放一首歌**

```text
playList (playback.go:21)        登录校验（按 Source 区分）、替换队列、重新洗牌
 → playAt (playback.go:126-196)  一个函数里做了 9 件事：
     1 写 cur + saveQueue（磁盘 IO）
     2 电台续杯 moreRadio
     3 diag 事件
     4 代际号 + 停止播放器
     5 音质策略：已知播客强制 128k
     6 音源解析：xyz 直链 / xyz 付费 PrivateMedia / QQ SongURL（按 Source 分派）
     7 player.Load（阻塞，在 goroutine）
     8 失败策略：连续失败 <3 且队列 >1 自动跳过
     9 成功后：记录音质、addRecent（磁盘 IO）、hearShow（磁盘 IO）
   → loadLyrics、syncComments
 每帧 view → tick (playback.go:295) → player.Ended() → next(true) → playAt …
```

**C. 搜索**：顶栏输入框 `searchBox`（view.go:316）根据当前页面决定占位文字、是否联想、菜单文案、清空行为；提交走 `chooseSearch`（app.go:1180）再按页面分派到 `runSearch` / `searchPodcasts` / `searchXyz`。「当前搜索作用域」这个概念在 6 处用 `a.current().kind == pagePodcast / pageXyz` 各自判断（见 P-07）。

**D. 启动**：`Run` → `newApp`（同步读 5 个文件 + `loadXyz` 读第 6 个）→ 窗口 → `loadLyrics` / `enter` / `startSession` / `startXyz`。

### 3.3 数据从输入到展示的路径

```text
HTTP JSON ──qqmusic/xyz 解析(flexInt/cleanIntro)──► 模型(Song/Playlist/Episode…)
   ──a.do 回主线程──► app 内各区域状态(home/lists/search/podcast/xyz…) + 代际号校验
   ──view 每帧读取──► 元素树
用户操作 ──view 回调──► app 方法 ──► 可能：改状态 / 发请求 / 写盘 / 控制播放器
```

小宇宙单集通过 `episodeSong`（xyz.go:123）转成 `qqmusic.Song{Source:"xyz", URL, Pic}`，从此和 QQ 歌曲共用队列、最近播放、`queue.json`。`qqmusic.Song` 实际上是全应用的「曲目」类型，但定义在 QQ 协议包里（见 P-06）。

### 3.4 状态与共享可变状态

| 状态 | 所在 | 权威来源 | 持久化 |
|---|---|---|---|
| 设置（音量、音质、模式、窗口、搜索历史、侧栏、小宇宙来源） | `a.set` | 内存；`a.volume` 与 `a.set.Volume` 双份（saveSettings 时同步，app.go:461） | settings.json |
| QQ 凭证 | `a.api` 内部（带锁） | `qqmusic.Client` | credential.json |
| 小宇宙凭证 + 昵称 | `a.xyz.api` + `a.xyz.user` | `xyz.Client`（刷新回调 Post 保存） | xyz.json |
| 最近播放 | `a.recent` | 内存 | recent.json |
| 队列 | `a.queue/cur/radio/shuffled` | 内存 | queue.json |
| 听过的节目 / 已知播客 | `a.podcast.heard / known` | known 由 4 处写入（P-05） | podcasts.json（仅 heard） |
| 页面数据 | `a.home/square/charts/search/lists/podcast/xyz/comments` | 内存缓存，无失效策略，靠刷新按钮 | 否 |
| 播放器状态 | `player.Player`（自带锁） | player | 否 |
| 图片缓存 | `a.images`（主线程独占，48 MiB 按帧淘汰） | images | 否 |

全局变量：除常量与 `errLoginClosed` 外基本没有；共享可变状态都集中在 `app` 单例里。跨 goroutine 的只有 `qqmusic.Client`、`xyz.Client`、`player.Player`、`trimmer`、`login.gen`（atomic），均有锁或原子操作。`-race` 下现有测试通过。

## 4. 复杂度集中在哪里、为什么会扩散

1. **`app` 结构体是唯一的状态 + 行为容器**。任何新功能（小宇宙官方就是例子：一次加了 3 个页面、~1,400 行）都要往 `app` 加字段，往 `enter`/`refresh`/`refreshing`/`content` 四个 switch 各加分支。
2. **业务规则没有自己的落脚点**，于是写在第一个需要它的地方：音质降级文案写在播放栏视图里，「是不是播客单集」写在各个调用点，「播放一个页面」写在卡片视图文件里。
3. **持久化没有边界**：每个需要保存的功能自己 `os.MkdirAll + json.Marshal + os.WriteFile(0600)`，并各自判断 `a.dir == ""`（8 处）。
4. **分页加载被实现了 6 次**，每次略有不同（页码 / 游标、total / more、是否去重、是否带副作用），调用方必须了解每种的细节（P-04）。

## 5. 问题清单

置信度：高 = 已读代码并可复现推理；中 = 结论依赖未验证的运行时行为。成本/风险：S/M/L。

### P-01 视图文件中直接访问远程接口并修改业务状态
- **位置**：`internal/app/view_pages.go:466-503`（`playPage`）
- **现状**：定义在视图文件里，内部 `a.do` 调 `a.api.ToplistDetail` / `AlbumDetail` / `AllPlaylistSongs`，并写 `a.podcast.known`（497-499）。未登录时用 `a.playList([]qqmusic.Song{{}}, 0)` 借空歌曲触发登录提示（473）。
- **证据**：其接口分派与 `app.go:882-907`（`loadList`）、`app.go:921-923`（`loadAlbum`）重复；三种页面类型的取歌逻辑有两份。
- **影响**：改动取歌方式（例如歌单接口变化）要同时改两处；视图测试无法不联网地覆盖播放按钮。
- **方向**：把「按页面取歌」收敛为一个应用层函数，`loadList` 与 `playPage` 共用；`playPage` 移出视图文件。
- **成本/风险**：S / 低。**置信度**：高。

### P-02 视图回调中直接修改并持久化设置
- **位置**：`view_player.go:198-205`（音质）、`view_player.go:237-239`（音量拖动结束保存）、`view_player.go:286-292`（播放模式）、`view.go:721-724`（侧栏折叠）
- **证据**：`a.set.Quality = qq; a.saveSettings(); a.toast(...)` 等直接写在 `OnClick` / `Clicked()` 分支里。
- **影响**：设置变更的规则（是否提示、何时保存）散在视图里；同一设置要在别处修改（如托盘、快捷键）只能复制。
- **方向**：增加应用层意图方法（`setQuality`、`setMode`、`commitVolume`、`toggleSidebarSection`），视图只调用。
- **成本/风险**：S / 低。**置信度**：高。

### P-03 业务规则写在视图中：音质展示规则
- **位置**：`view_player.go:119-137`
- **证据**：AAC→「（无 MP3 音源）」、`Best` 或达到请求→只显示实际音质、请求无损未得→「（无损不可用）」、否则「（无高品质）」、小宇宙→「小宇宙」。这是 docs/dev/pitfalls.md 第 7 条 写明的业务规则，但只能通过 UI 测试验证。
- **方向**：提取为纯函数，单测覆盖 5 个分支；视图只显示结果。
- **成本/风险**：S / 低。**置信度**：高。

### P-04 「分页加载列表」被实现 6 次
- **位置与差异**：

| 实现 | 位置 | 翻页方式 | 结束判定 | 去重 | 代际号 |
|---|---|---|---|---|---|
| `found[T]` + `searchMore` | app.go:106-114, 1282-1307 | 页码 | total / 空页 | 调用方传 `add` | 有 |
| `podcastFound[T]` + `podcastMore` | podcast.go:50-58, 245-276 | 页码 | `more` | 内置（按 key） | 有，且顺带写 `podcast.known` |
| `xyzList[T]` + `xyzMore` | xyz.go:86-92, 508-530 | 游标 `xyz.More` | `next == nil` | 无 | 有 |
| `commentList` + `loadComments` | comments.go:23-29, 93-135 | 页码 + 末条 SeqNo | `more && len>0` | 内置 | 共享 `cm.gen` |
| 歌手歌曲/专辑 | app.go:954-1022 | offset | total / 空页 | 无，靠 `len != begin` 丢弃 | `l.gen` |
| 歌单广场 | app.go:791-820 | 页码 | total / 空页 | `addPlaylists` | 无（用分类号判断过期） |

- **共同不变量**：`loading` 防重入；发起前清 `err`；过期结果丢弃；失败保留已有条目；追加而非替换。
- **影响**：每加一个列表就复制一份状态机；视图端的「滚到底加载」也随之有 3 种写法（`xyzEnd`、`singerSongsEnd`、`commentsEnd`）。
- **方向**：只有前 3 个是同一种东西（结果列表 + 下一页）。可以把它们统一成一个以「游标」为类型参数的 `pager[T]`（页码也是一种游标）。评论、歌手、广场都有特殊规则，**不建议强行合并**。
- **成本/风险**：M / 中（涉及 3 个页面的视图与测试）。**置信度**：高（差异表已逐个核对）。**未确认**：统一后是否真的比现在更易读，需要先在一个调用点试写再决定（见 decisions D-07）。

### P-05 「歌曲来源 / 是否播客单集」规则分散，且已出现不一致
- **位置**（`Source` 与 `podcast.known` 判断共 14 处）：
  - `s.Source == ""` 表示 QQ 歌曲：playback.go:26、185；view_pages.go:971；view_player.go:51
  - `s.Source != ""`：app.go:1429；playback.go:202
  - `s.Source == xyzSource`：playback.go:154、156；view_player.go:135、397；xyz.go:155
  - `podcast.known[...]`：playback.go:145；podcast.go:140；view_pages.go:980
  - `known` 写入：app.go:437、935；podcast.go:96-100、267；view_pages.go:498
- **不一致的证据**：右键菜单把「已知 QQ 播客 或 小宇宙单集」都叫「查看节目」（view_pages.go:980），歌词页只对小宇宙单集显示「节目：」（view_player.go:397），QQ 内置播客单集在歌词页仍显示「专辑：」。
- **影响**：新增第三种来源（或修改播客判定）要改十几处，而且已经漏改。
- **方向**：集中为少量命名谓词（如 `isXyz(s)`、`a.isEpisode(s)`、`needsQQLogin(s)`），调用点改用谓词。**修复不一致属于行为变更，单独记录，不混入重构**（见 verification-report §5）。
- **成本/风险**：S / 低。**置信度**：高。

### P-06 `qqmusic.Song` 兼任全应用的「曲目」类型
- **位置**：`internal/qqmusic/types.go:38-58`（`Source`/`URL`/`Pic` 字段注释写明是给外部来源用的）
- **影响**：QQ 协议包知道「别的来源」的存在；但 `queue.json` / `recent.json` 正是用这个类型序列化的。
- **方向**：**保留**。新建应用层 `track` 类型要改两个持久化格式和几乎所有视图，收益不抵风险。把它记为有意的设计，并在 P-05 的谓词中封装对 `Source` 的判断。
- **置信度**：高。

### P-07 「搜索作用域」概念在 6 处各自推导
- **位置**：view.go:318-329（占位与标签）、view.go:334-337（是否联想）、view.go:383-390（清空回退）、view.go:418-424（菜单文案）、app.go:1138（`searchWords` 屏蔽联想）、app.go:1189-1196（`chooseSearch` 分派）
- **影响**：再加一个可搜索的页面要改 6 处。
- **方向**：一个返回作用域枚举的方法 + 各处 switch 改为基于枚举；分派逻辑留在 `chooseSearch`。
- **成本/风险**：S / 低。**置信度**：高。

### P-08 播放编排 `playAt` 职责过多
- **位置**：`playback.go:126-196`（见 §3.2-B 的 9 个步骤）
- **影响**：音源解析（2 种真实实现）、音质策略、失败跳过策略都无法单测；`TestPlayModes`（app_test.go:325）因此只能复制一份 `(cur+1)%n` 自己断言，没测到真正的 `next()`。
- **方向**：拆出「音源解析」（按来源分派，已有两种真实实现，符合 §4.5 的抽象条件）、「播放音质」策略、「失败后是否跳过」策略；队列规则见 P-09。
- **成本/风险**：M / 中（播放是核心路径，需先补特征测试）。**置信度**：高。

### P-09 播放队列规则与播放器 IO、持久化交织
- **位置**：`playback.go:20-123, 240-291`；字段 `app.go:302-317`（与歌词、音量、弹窗开关等平铺在一起）
- **证据**：`removeFromQueue` 的 cur 调整、`next(auto)` 的四种模式（单曲循环自动时 seek 0、顺序播放到尾暂停）、`shuffleStep` 的环绕、`moreRadio` 的按 Mid 去重，都直接操作 `a.queue/a.cur` 并穿插 `a.player.*` 与 `a.saveQueue()`。
- **不变量**：`cur ∈ [-1, len)`；`len(shuffled) == len(queue)`（`shuffleStep` 懒修复）；空队列 ⇔ `cur == -1` 且删除 `queue.json`。
- **方向**：提取纯数据结构「播放队列」（无 IO），由 `playback.go` 编排播放器与保存。这是本项目最有价值的业务抽象。
- **成本/风险**：M / 中。**置信度**：高。

### P-10 持久化无边界，错误被静默忽略
- **位置**：app.go:409-510、podcast.go:177-188、xyz.go:161-183；`a.dir == ""` 判断 8 处；`os.WriteFile` 的返回值全部丢弃
- **影响**：文件名、权限、「空即删除」规则（queue.json、podcasts.json、credential.json）分散；写盘失败用户无感知（这是现状行为，是否提示需决策）；写入非原子，崩溃时可能留下截断文件（读取端解析失败会退回默认值，所以不会崩溃）。
- **方向**：集中到一个小的存储模块（文件名常量 + 读/写/删 + 权限 + 空目录模式），持久化格式保持逐字节兼容。是否改为原子写、是否上报错误另行决策（D-05）。
- **成本/风险**：S-M / 低。**置信度**：高。

### P-11 构建视图时的副作用
- **位置**：`view.go:83`（每帧 `a.tick()`，可能触发 `next(true)` → `playAt` → 写 `queue.json` + 发请求）；滚动到底自动加载：view_pages.go:559-561、view_pages.go:644、view_singer.go:110、141-149、view_podcast.go:393、view_comments.go:120、view_xyz.go:479；`recentPage` 每帧写 `a.recentView`（view_pages.go:665）
- **现状评估**：这些调用都由 `loading` 标志或 `player.Ended()` 在 `Stop` 后变 false 保证幂等，一帧多次构建不会重复请求；它们是 MyGo 即时模式下「看到末尾就加载」的惯用法，`view_xyz.go:476` 的注释也说明了理由。但它与 AGENTS.md §2.4「禁止在构建视图时发请求」字面冲突。
- **方向**：滚动加载保留（改为统一的「确保已加载」意图方法即可）；`tick` 是播放推进的唯一驱动，**暂不改**。**未确认**：窗口最小化或隐藏时 MyGo 是否继续构建帧——若不构建，自动切歌会停在当前曲尾（需人工验证，见 verification-report §4）。
- **成本/风险**：S / 低（仅整理）；改驱动方式为 L / 高，不建议。**置信度**：中。

### P-12 页面类型分派在 4 个 switch 中重复
- **位置**：`app.go:569-599 enter`、`602-641 refresh`、`645-672 refreshing`、`view_pages.go:15-43 content`
- **影响**：docs/dev/howto.md §1 新增页面需要改 7 个地方；漏改 `refreshing` 只会让刷新按钮不转，不易被发现。
- **方向**：考虑过「页面控制器接口 + 注册表」，但 12 种页面多数只有 1-3 行分支，引入接口会增加间接层且 Go 的 switch 本身可 grep。**建议保留**，只把三个 switch 放到同一个文件里相邻排列（见 D-08）。
- **置信度**：高。

### P-13 `app` 结构体与 `app.go` 过大
- **位置**：`app.go:192-347`（约 120 个字段），`app.go` 1,459 行混合 6 类职责（模型类型、持久化、导航、首页/音乐馆/歌单/歌手加载、搜索、会话）
- **影响**：定位困难；播放相关的 30 个字段与界面开关（`modeOpen`、`qualityOpen`、`volumeWasPressed`）平铺混排，所有权不清。
- **方向**：按职责拆文件（纯移动）；播放相关字段归入一个子结构，界面临时状态与业务状态分开。
- **成本/风险**：M / 低（机械，但 diff 大，应放在后期单独提交）。**置信度**：高。

### P-14 测试与检查基线的卫生问题
- `internal/app/zz_feed_test.go:15-36`：读取真实 `%AppData%\QQMusicGo\credential.json` 访问线上接口，**没有** `testing.Short()` 跳过，`go test -short ./...` 也会用真实账号联网。违反 AGENTS.md §2.11 / docs/dev/conventions.md §5。
- `cmd/zz_podcast/main.go`：17 处无键结构体字面量，导致 `go vet ./...` 与 `go tool mygo vet .` 在基线上就失败（docs/dev/release.md 检查清单无法通过）。
- `cmd/zz_feeddump`、`cmd/zz_podcast`、`zz_feed_test.go` 未出现在 CODEMAP.md 中，自述为一次性探针（「throwaway probe」）。
- **方向**：需要你决定删除、加构建标签，还是给测试加 Short 跳过（D-09）。
- **置信度**：高。

### P-15 数据访问无法替换，加载编排难以测试
- **位置**：`app.api *qqmusic.Client`、`app.xyz.api *xyz.Client` 均为具体类型（app.go:194、xyz.go:25）
- **现状**：离线 UI 测试靠直接给状态字段赋值绕过加载（xyz_test.go:46-60、podcast_test.go）；各 `loadXxx` 的「失败保留旧数据」「过期丢弃」规则没有测试。
- **方向**：**不建议**现在为 ~50 个接口方法引入一个大接口。先把纯规则（队列、音质、MRU、作用域）提出来单测；若之后确实需要测试加载编排，再按调用方需要定义小接口（D-06）。
- **置信度**：高。

## 6. 应当保留的现有设计

- `qqmusic` / `xyz` / `player` 三个包：接口小、内部复杂度隐藏得好，有针对性单测（`TestParseDirect`、`TestFLAC` 等）。
- `a.do(work) → Post(apply)` 的主线程模型 + 代际号丢弃过期结果：简单、统一、经过 `-race` 验证。
- `platform` 接口（app.go:170-188）：已有两个真实实现（`desktop`、`fakePlatform`），正是测试所需的隔离点。
- `images` 缓存通过构造参数注入 `fetch` / `post`（images.go:45），测试已利用这一点离线运行。
- 视图中的界面临时状态（搜索框 `open/sel/focused`、弹窗开关）属于展示层，不需要迁出。
- 「失败保留旧数据」「空结果与失败分开」等约定（docs/dev/conventions.md §2）在加载函数中执行得很一致。
- `qqmusic.Song` 作为跨来源曲目类型（P-06）。

## 7. 与现有文档的出入

| 文档说法 | 实际 |
|---|---|
| AGENTS.md §2.4「禁止在构建视图时发请求」 | 滚动加载与 `tick` 都在构建期触发（P-11），属于事实上的惯例 |
| docs/dev/release.md「`go tool mygo vet .` 无新增告警」 | 基线已失败（P-14） |
| docs/dev/conventions.md §5「联网测试必须 Short 跳过」 | `TestZZFeed` 未遵守 |
| CODEMAP.md 文件清单 | 缺少 `cmd/zz_feeddump`、`cmd/zz_podcast`、`internal/app/zz_feed_test.go` |
| CODEMAP.md「小宇宙 beta」 | 界面上已称「官方」（`podcastTitle`），代码与文档仍混用 beta |
