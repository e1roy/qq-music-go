# 目标架构（阶段二）

> 状态：已实施（U6 除外），与方案的出入见 `decisions.md` D-13；本文已按实际代码更新。问题编号 P-xx 见 `architecture-audit.md`，决策编号 D-xx 见 `decisions.md`。

## 1. 设计原则（针对本项目）

1. **不动下层**：`qqmusic`、`xyz`、`player`、`diag` 已经是合格的数据 / 基础设施层，本次只在 `internal/app` 内部和其上方动刀。
2. **`internal/app` 仍是一个包**。MyGo 的视图是 `func (a *app) xxx(c *ui.Context)`，需要直接读状态；把视图和编排拆成两个 Go 包会迫使大量状态导出，或引入 ViewModel 层，代价大、收益不明显。包内用**文件职责 + 可 grep 的依赖规则**来分层。
3. **只把有不变量、可独立测试的规则提成独立包**：播放队列（`internal/playqueue`）和本地存储（`internal/store`）。它们不依赖 UI，由编译器保证依赖方向。
4. **不引入**：DI 框架、事件总线、Repository 大接口、ViewModel、页面注册表。理由见 `decisions.md`。

## 2. 目标模块图

```text
cmd/qqmusic
   │
   ▼
internal/app ───────────────────────────────────────────────────────────────
 │ Presentation（展示层）
 │   view.go view_pages.go view_player.go view_search.go view_singer.go
 │   view_podcast.go view_xyz.go view_comments.go view_login.go
 │   skeleton.go motion.go icons.go palette.go images.go
 │        │ 只读状态；用户意图 → 调用下方的 app 方法
 │        ▼
 │ Application（编排层，均为 *app 方法，运行在主线程）
 │   app.go       组装、a.do / toast / errText
 │   nav.go       page、历史、enter / refresh / refreshing（三个 switch 相邻）
 │   library.go   首页、音乐馆、歌单/榜单/专辑/歌手加载、listSongs、playPage
 │   search.go    搜索、联想、历史、搜索作用域
 │   session.go   启动会话、登录成功、我的歌单、退出、我喜欢；login.go（扫码轮询）
 │   playback.go  播放编排：音源解析、音质策略、失败跳过、歌词、tick
 │   settings.go  设置意图方法（音质、模式、音量、侧栏）+ 持久化记录类型与读写
 │   pager.go     分页列表的统一状态与加载（搜索、播客搜索、小宇宙列表）
 │   podcast.go xyz.go comments.go   各自区域的状态与加载
 │   desktop.go   platform 真实实现
 │        │                     │
 │        ▼                     ▼
 │ Domain（纯规则，无 IO）       Data / Infrastructure
 │   internal/playqueue          internal/store     本地 JSON 文件
 │   app 包内纯函数：             internal/qqmusic   QQ 音乐接口（不变）
 │     qualityLabel、pushFront    internal/xyz       小宇宙接口（不变）
 │                                 internal/player    播放器（不变）
 │     fromQQ / isXyz / qqEpisode
 └──────────────────────────────────────────────────────────────────────────
```

## 3. 各模块职责与公开接口

### 3.1 `internal/playqueue`（新，Domain）——解决 P-09、P-08 的一部分

**代表的概念**：播放队列——一组曲目、当前位置、随机顺序、是否为「猜你喜欢」电台，以及「下一首 / 上一首」在四种播放模式下的规则。

**归入的现有实现**：`playback.go` 的 `playList` 中的替换部分、`playNext`、`enqueue`、`removeFromQueue`、`clearQueue` 的数据部分、`reshuffle`、`next` 的选曲部分、`prev` 的选曲部分、`shuffleStep`、`moreRadio` 的去重追加；`playMode` 类型及其 `String()`。

**最小接口（草案）**：

```go
type Mode int // 列表循环 / 单曲循环 / 随机播放 / 顺序播放；数值与 settings.json 中的 "mode" 一致

type Queue struct{ /* songs, cur, radio, order —— 不导出 */ }

func Restore(songs []qqmusic.Song, cur int, radio bool) Queue // 复制 songs；零值 Queue 即空队列
func (q *Queue) Songs() []qqmusic.Song
func (q *Queue) Cur() int
func (q *Queue) Current() (qqmusic.Song, bool)
func (q *Queue) Radio() bool
func (q *Queue) SetRadio(on bool)
func (q *Queue) Replace(songs []qqmusic.Song)         // 复制、radio=false、重新洗牌；cur 不变，由 Select 设定
func (q *Queue) Select(i int) bool                     // 越界返回 false
func (q *Queue) InsertNext(s qqmusic.Song)
func (q *Queue) Append(s qqmusic.Song)
func (q *Queue) AppendMissing(songs []qqmusic.Song)    // 按 Mid 去重，电台续杯用
func (q *Queue) Remove(i int) (wasCurrent bool)        // 维护 cur
func (q *Queue) Clear()
func (q *Queue) NearEnd() bool                         // radio && cur >= len-2
func (q *Queue) Next(m Mode, auto bool) Step           // Step{Kind: Play|Replay|Stop|None, Index}
func (q *Queue) Prev(m Mode) int
```

**必须保持一致的行为**（特征测试先固定）：`cur` 在删除前一首时减一、删除当前曲后播放 `min(i, len-1)`、删光等同清空；单曲循环只在自动切歌时重播；顺序播放到尾自动切歌时停止，手动「下一首」仍回到开头；随机模式按洗牌顺序环绕，洗牌长度不符时先重洗。

**隐藏的细节**：索引调整、洗牌数组与队列长度同步、环绕取模。

**禁止承担**：播放器控制、写 `queue.json`、toast、网络请求。这些由 `playback.go` 编排。

**为何集中**：这些规则目前和 IO 交织，只能通过 UI 或真播放器验证；`TestPlayModes` 甚至复制了一份规则来测。集中后可以对每条规则做表驱动单测，`playback.go` 只剩「选曲 → 控制播放器 → 保存」。

**兼容层**：无。调用方直接改用 `playqueue.Mode`（D-13）。

### 3.2 `internal/store`（新，Data）——解决 P-10

**代表的概念**：应用数据目录中的 JSON 文件。

**最小接口**：

```go
type Dir struct{ /* path；空字符串表示仅内存（测试用）*/ }

func Open(path string) Dir
func (d Dir) Load(name string, v any) bool          // 文件不存在或解析失败返回 false，v 不变
func (d Dir) Save(name string, v any, indent bool)   // MkdirAll 0700 + WriteFile 0600
func (d Dir) Remove(name string)
```

**隐藏的细节**：目录创建、权限、空路径即不落盘、缩进格式。文件名常量与记录类型（`settings`、`savedQueue`、`savedXyz`）**留在 app 包**（`settings.go`），因为它们引用 app 的类型。

**必须保持一致**：6 个文件的文件名、JSON 字段、缩进方式（settings/credential/xyz 缩进，recent/queue/podcasts 紧凑）、「空即删除」规则、写失败静默（是否改变见 D-05）。

### 3.3 Application 层（`internal/app` 非视图文件）

**职责**：把一次用户意图编排成完整操作：校验前置条件（登录）、调用 Domain 规则、经 `a.do` 访问数据层、回到主线程后校验代际号、更新状态、保存、提示。

**规则**：
- 所有网络与磁盘访问只出现在这里（及 `desktop.go`）。
- 对外（对视图）只暴露意图方法和只读查询，例如 `playPage(pg)`、`setQuality(q)`、`searchScope()`、`qqEpisode(s)`。
- 不新建 `UseCase` 类型：大多数操作只有 10-30 行，用 `*app` 方法即可（D-03）。

**新增 / 迁移的关键函数**：

| 函数 | 来自 | 作用 |
|---|---|---|
| `listSongs(ctx, p)` | `loadList` 与 `playPage` 中重复的歌单 / 榜单分派 | 取歌单或榜单的全部歌曲，供加载与卡片上的「播放」共用（P-01）；专辑分支只在 `playPage` 中使用，未并入 |
| `playPage(pg)` | `view_pages.go:467` | 移到 `library.go`，内部用 `listSongs` |
| `qualityLabel(...)` | `view_player.go:119-137` | 纯函数，返回播放栏音质徽章文字（P-03） |
| `setQuality` | 视图回调 | 设置音质，附带「下一首起使用」提示（P-02）；其余设置项在点击回调中直接改并 `saveSettings()`（D-14） |
| `searchScope()` | 6 处 `current().kind` 判断 | 返回 `scopeMusic` / `scopeQQPodcast` / `scopeXyz`（P-07） |
| `fromQQ(s)` / `isXyz(s)` / `a.qqEpisode(s)` | 14 处 `Source` / `known` 判断 | 曲目来源谓词（P-05） |
| `pushFront(list, item, max, same)` | `addRecent`、`rememberSearch`、`hearShow` | 「最近使用」列表：置顶、去重、截断（3 处同义实现） |

### 3.4 Presentation 层（`view_*.go` 等）

**职责**：布局、样式、动画、展示加载 / 空 / 错误三态、收集点击并调用意图方法、维护纯界面临时状态（弹窗开关、搜索框焦点、滚动位置）。

**允许**：读取 `app` 状态；调用 `a.player` 的只读查询与 `Seek`（纯播放控制，无业务规则）；`a.images.get`（图片缓存本身负责异步）；在构建期调用幂等的「确保已加载」方法实现滚动到底加载（P-11，MyGo 惯用法，保留）。

**禁止**：`a.do(`、`os.`、`json.`、调用接口客户端的请求方法；直接修改队列、`podcast.known` 等业务状态。**允许**读取登录状态（`LoggedIn()`、`Credential()`），以及在点击回调里修改设置字段并 `saveSettings()`（D-14）。

## 4. 依赖方向（可校验）

```text
view_*.go  ──►  app 编排方法 / 只读状态
app 编排   ──►  playqueue, store, qqmusic, xyz, player
playqueue  ──►  qqmusic（仅模型类型 Song）
store      ──►  标准库
qqmusic / xyz / player ──► 标准库与第三方解码库（不变）
```

辅助检查（指导原则而非门禁，见 D-14）：

```sh
# 视图层不联网、不读写文件
grep -nE 'a\.do\(|\bos\.|\bjson\.|\b(a|x)\.(xyz\.)?api\.[A-Za-z]+\(' internal/app/view*.go | grep -vE 'api\.(LoggedIn|Credential)\(\)'
# 纯包不得依赖 UI 或 app
go list -deps ./internal/playqueue ./internal/store | grep -E 'mygo|internal/app'
```

两条命令输出都应为空。

## 5. 状态与数据流

| 状态类别 | 例子 | 唯一所有者 | 更新入口 | 生命周期 |
|---|---|---|---|---|
| 持久化数据 | 设置、凭证、最近播放、队列、听过的节目、小宇宙登录 | app（内存为准），`store` 负责落盘 | `settings.go` 意图方法、`addRecent`、`playback` 编排、`hearShow`、会话方法 | 进程启动时读一次；变化时写 |
| 业务临时状态 | 队列位置、失败计数、代际号、`podcast.known` | `playqueue.Queue` / app | 编排方法 | 进程内 |
| 页面数据缓存 | `home`、`lists[page]`、`search`、`xyz.shows` | app 各区域结构 | `loadXxx` / `moreXxx`（主线程回调） | 进程内，无过期；刷新按钮重载；失败保留旧数据 |
| 展示状态 | 弹窗开关、动画、滚动 | 视图（存在 app 字段中，因 MyGo 需要持久） | 视图回调 | 进程内 |
| 派生数据 | 页面主色、`searchWords()`、`recentEpisodes()`、音质文字 | 不存储，每帧计算 | — | — |

**重复副本要消除的**：`a.volume` 与 `a.set.Volume`（目前在 `saveSettings` 时同步）——保留现状，记录为已知双写，在 `settings.go` 中集中处理即可，不改语义。

**异步语义（保持不变）**：
- 加载：`loading` 防重入；发起时清 `err`。
- 成功：校验代际号或「对象仍是同一个」（`x.shows[pid] == sh`）后写入。
- 失败：保留已有数据，只写 `err`；用户可感知的操作失败用 toast。
- 取消：不取消请求本身，丢弃结果（30 秒超时）；扫码轮询用代际号停止。
- 重复请求：由 `loading` 标志合并；`toggleLike` 是唯一主动打断 `loading` 的地方（见 verification-report §5 BUG-4）。

## 6. 现有代码 → 目标位置映射

| 现在 | 目标 |
|---|---|
| app.go:22-50 page/pageKind、512-672 导航与三个 switch | nav.go |
| app.go:52-128 类型（songList、found、playMode…） | 各自使用者的文件；playMode → playqueue |
| app.go:130-168 settings、savedQueue | settings.go |
| app.go:409-510 持久化 | settings.go（记录类型 + 读写调用）+ internal/store |
| app.go:674-1047 首页、广场、榜单、歌单、专辑、歌手 | library.go |
| app.go:1049-1336 搜索 | search.go |
| app.go:1338-1459 会话、我喜欢 | session.go |
| view_pages.go:466-503 playPage | library.go |
| view_player.go:119-137 音质文字规则 | playback.go `qualityLabel` |
| playback.go 队列操作 | internal/playqueue |
| podcast.go:177-188、xyz.go:161-183 存取 | 改用 store |

## 7. 为什么选这个方案

- **它直接对应审计中的问题**：P-01/02/03 是「视图里有数据访问和规则」，P-05/07 是「同一概念散落」，P-09/10 是「规则与 IO 交织」。方案中的每个新模块都能指回至少一个问题。
- **复杂度被收进模块而不是摊到调用方**：`playqueue` 隐藏索引维护；`store` 隐藏路径、权限、空目录模式；`listSongs` 隐藏歌单与榜单两种接口的分派。删掉它们，复杂度会回到 3-14 个调用点——符合 refactor-playbook.md §2.2 的判断标准。
- **浅**：只新增 2 个包、约 10 个函数；没有新的接口类型（`platform` 已有）。

## 8. 没有选择的方案

| 方案 | 不选的原因 |
|---|---|
| MVVM：每个页面一个 ViewModel 结构体 | MyGo 即时模式视图直接读状态即可，ViewModel 只会把字段复制一遍；12 个页面 × 若干字段的样板代码 |
| Clean / Hexagonal：为 qqmusic、xyz 定义 Repository 接口 | `qqmusic.Client` 有约 50 个方法，只有一个真实实现；为测试而加接口要等真有加载编排测试需求再按需定义小接口（D-06） |
| 页面控制器接口 + 注册表（替代 4 个 switch） | 多数分支 1-3 行；接口化后新增页面仍要写同样多的代码，只是换了地方，还多一层间接（D-08） |
| 统一的通用分页框架 | 6 种分页有真实差异（审计 P-04 表）；只考虑合并其中 3 种，且先试点（D-07） |
| 应用层自有 `track` 类型替代 `qqmusic.Song` | 会改变 queue.json / recent.json 格式，几乎触及所有视图（P-06） |
| 把视图拆到单独 Go 包 | 需要导出大量状态或做适配层，与 MyGo 的用法相悖 |

## 9. 代价与缺点

- 包内分层靠约定与 grep 检查，编译器不强制（只有 `playqueue`、`store` 是编译期隔离）。
- 按职责拆分 `app.go` 会产生大 diff（纯移动），影响 `git blame`；放在最后单独提交以便审查。
- 加载编排（`loadXxx` 的失败保留、过期丢弃）在本方案中仍主要靠 UI 测试覆盖，没有离线单测（D-06）。
