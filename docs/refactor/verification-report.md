# 验证报告

## 1. 环境

| 项 | 值 |
|---|---|
| 日期 | 2026-10-27 |
| 系统 | Windows 11 Pro 10.0.26200，amd64 |
| Go | go1.27.3 windows/amd64 |
| 基线提交 | `f9f87bc`（工作区干净） |
| 竞态检测 | 需要 cgo；本机 llvm-mingw gcc，`CGO_ENABLED=1` 运行 |

## 2. 基线结果（重构前，实际执行）

| 命令 | 结果 |
|---|---|
| `go build ./...` | 通过 |
| `go vet ./...` / `go tool mygo vet .` | **失败**：`cmd/zz_podcast/main.go` 17 处无键结构体字面量 |
| `go test -short -count=1 -skip TestZZFeed ./...` | 通过 |
| `-race`（internal/app） | 通过 |
| 视图层依赖检查 | 18 处命中 |
| 覆盖率 | app 47.0%、player 25.5%、qqmusic 8.5%、xyz 7.5% |

跳过 `TestZZFeed` 的原因：它用 `%AppData%` 中的真实凭证联网，且没有 Short 保护（BUG-1）。

## 3. 重构后结果（实际执行，含代码审查修复）

| 命令 | 结果 |
|---|---|
| `go build ./...` | 通过 |
| `go build -ldflags "-H=windowsgui" ./cmd/qqmusic` | 通过 |
| `go vet ./...` | **通过** |
| `go tool mygo vet .` | **通过** |
| `go vet -tags probe ...` | 仍报 `zz_podcast` 的 17 处告警（探针代码本身未改，只在显式 `-tags probe` 时编译） |
| `go test -short -count=1 ./...` | 全部通过（不再需要 `-skip`） |
| `CGO_ENABLED=1 go test -race -short ./internal/app ./internal/playqueue ./internal/store` | 通过 |
| `gofmt -l internal cmd` | 无输出 |
| 视图层辅助检查（D-14 收窄后的规则） | 无输出 |
| `go list -deps ./internal/playqueue ./internal/store \| grep -E 'mygo\|internal/app'` | 无输出 |
| 覆盖率 | 见下方最终运行 |

`testdata/v1.0.1/` 中的 6 个文件由重构前的代码写出。U7 之后它们曾作为黄金文件按字节比对并通过，即重构本身没有改变写出的格式；代码审查后改为 `TestPersistedFiles`：这些旧文件仍可读、保存后可读回（D-14 #5）。

## 4. 特征测试与验证缺口

| 行为 | 测试 | 说明 |
|---|---|---|
| 队列增删、当前位置、空队列删除 queue.json、入空队列直接播放 | `TestQueueEdits`（app）、`TestEdits`（playqueue） | Phase 0 写于旧代码上，U2 后只改了取状态的辅助函数，断言未改 |
| 四种模式下 next / prev，空队列 | `TestNextPrev`、`TestNext`、`TestShuffle` | 同上 |
| 连续失败 <3 且队列 >1 跳过并提示 | `TestPlayFailures` | 用指向 `127.0.0.1:1` 的小宇宙单集，不调 `SongURL` |
| 音质徽章文字 | `TestQualityLabel` | **缺口**：徽章 `Label("音质")` 覆盖了可见文字，Tester 读不到，无法在旧代码上写 UI 级特征测试。`qualityLabel` 是逐分支搬移（diff 可核对），单测覆盖 6 种分支 + 小宇宙 |
| 旧版本文件可读、往返、空即删除、未知播放模式回退 | `TestPersistedFiles`、`TestDir`（store，含原子写不留临时文件） | |
| 猜你喜欢在需要登录时不改动原队列 | `TestPlayRadio` | 审查发现 #1 |
| 队列零值、不改动传入切片 | `TestZero`、`TestOwnSongs`（playqueue） | `TestOwnSongs` 去掉复制后实测失败 |
| 搜索作用域：标签、菜单文案、是否联想、提交到哪个搜索 | `TestSearchScope` | |
| 加载编排（失败保留旧数据、过期丢弃） | 无离线测试 | D-06；本次未改加载函数体（U10 为纯移动） |

**启动冒烟测试（已执行）**：先把 `%AppData%\QQMusicGo` 备份到会话临时目录，然后用重构后构建的 `QQMusic.exe`（`-H=windowsgui`）加载你的真实数据启动，运行 12 秒后用关闭窗口的方式退出。结果：
- 进程正常运行，`crash.log` 大小不变（70529 字节，无新崩溃）。
- 启动时经新的原子写入路径重写了 `credential.json`、`xyz.json`，关闭时保存了 `settings.json`；没有残留 `.tmp` 文件。
- 6 个文件都能解析；QQ 音乐与小宇宙的登录仍然有效；`settings.json` 内容与启动前相同。
- 只发出正常启动的请求（会话校验、首页等），没有调用 `SongURL`。

**仍需人操作的手动走查**：

- 登录后播放 MP3 / FLAC / AAC，四种模式下自动与手动切歌，删除队列中正在播放的歌，猜你喜欢续杯（遵守 AGENTS.md §2.11：≤10 首、串行）。
- 首页 / 榜单 / 歌单 / 专辑卡片上的播放按钮（`playPage` 已移动），未登录时点击应提示登录。
- 修改音质、模式、音量、侧栏折叠后重启，设置保持。
- 顶栏搜索框在音乐、小宇宙（QQ 音乐）、小宇宙官方三种页面的行为。
- 托盘菜单、小宇宙单集播放、窗口位置记忆。
- 快速连续点「喜欢」/取消（BUG-4 的修复用模拟服务端测过，真实服务未验证）；QQ 内置播客单集的歌词页显示「节目：」。
- 小宇宙官方各列表与搜索的滚动加载（U6 合并了分页实现）。
- **未确认**：窗口最小化时 MyGo 是否继续构建帧（自动切歌依赖每帧 `tick`）。这是原有行为，本次未改。

## 5. 调查与审查中发现的现有 Bug（已全部修复）

| 编号 | 位置（重构后） | 描述 | 修复是否改变行为 | 状态 |
|---|---|---|---|---|
| BUG-1 | `internal/app/zz_feed_test.go` | 用真实账号联网的测试没有 Short 保护 | 否 | **已缓解**：加了 `//go:build probe`，`go test ./...` 不再编译它 |
| BUG-2 | `cmd/zz_podcast/main.go` | 无键结构体字面量导致 vet 失败 | 否 | **已缓解**：同上，默认构建不再包含 |
| BUG-3 | `internal/app/session.go` `logout` | 退出登录把电台标记设为 false，但没有 `saveQueue`，`queue.json` 仍是 `radio: true` | 是（轻微） | **已修**（D-15，`TestLogoutRadio`） |
| BUG-4 | `session.go` `toggleLike` + `library.go` `loadList` | 强制打断「我喜欢」的加载；歌单加载无代际号且只增不减已喜欢集合，快速「喜欢→取消」可能把已取消的歌重新显示为红心 | 是 | **已修**（D-15，`TestLikeRace`） |
| BUG-5 | `view_pages.go` `songMenu` 与 `view_player.go` `nowPlayingPage` | QQ 内置播客单集：右键菜单叫「查看节目」，歌词页显示「专辑：」 | 是（文案） | **已修**（D-15，`TestShowLabel`） |
| BUG-6 | `playback.go` `playRadio` | `playList` 因未登录返回时，旧队列被标为电台并开始续杯（代码审查发现，重构前就有） | 是 | **已修**（D-14 #1） |
| BUG-7 | `settings.go` `load` | settings.json 中越界的 `mode` 导致每次启动崩溃（代码审查发现，重构前就有） | 是（只影响非法文件） | **已修**（D-14 #2） |

## 6. 实际改动

| 类别 | 文件 |
|---|---|
| 新包 | `internal/playqueue/{queue,queue_test}.go`、`internal/store/{store,store_test}.go` |
| 新文件（移动 / 新职责） | `internal/app/{nav,library,search,session,settings}.go` |
| 新测试 | `internal/app/behavior_test.go`、`internal/app/testdata/persist/*.json` |
| 修改 | `app.go`（只剩结构体、构造、`do`/`toast`/`errText`）、`playback.go`、`podcast.go`、`xyz.go`、`view.go`、`view_pages.go`、`view_player.go`、`view_podcast.go`、`view_xyz.go`、`app_test.go` |
| 构建标签 | `cmd/zz_feeddump/main.go`、`cmd/zz_podcast/main.go`、`internal/app/zz_feed_test.go` |
| 文档 | `CODEMAP.md`、`AGENTS.md`、`docs/refactor/*` |

`internal/app/app.go` 从 1,459 行降到 260 行。U10 的拆分用行级多重集比对确认：拆分前后非 import 行完全相同，只有三个分节注释（`// Navigation.`、`// Loading.`、`// The user.`）被文件名取代，另加 `library.go` 的一段文件说明。

## 7. 兼容性证据与已知的细微差异

**保持不变**：持久化文件名、字段、缩进（黄金文件）；所有 toast 文案；队列规则（特征测试在旧代码上写成，重构后断言未改）；搜索分派与联想屏蔽；音质降级文案。

**代码审查后有意的行为变化**（你要求「fix all」）：BUG-6、BUG-7 已修；写盘改为原子写（多一个短暂存在的 `*.tmp` 文件）；删除正在播放的歌时只写一次 `queue.json`。

**修复剩余 Bug 后有意的行为变化**（D-15）：退出登录会写 `queue.json`；「我喜欢」重新加载后按完整列表重建红心（不再只增不减），点喜欢时正在进行的「我喜欢」加载被作废并在完成后重载；QQ 内置播客单集的歌词页显示「节目：」；小宇宙列表与播客搜索在重置时保留滚动状态并滚回顶部（原为换成零值，同样回到顶部）。

**可观察性极低的差异**（逐一列出，均判断为无用户影响）：

1. `saveSettings` 现在无论有无数据目录都把 `a.volume` 同步到 `a.set.Volume`；旧代码在没有数据目录时（仅测试或 `UserConfigDir` 失败）不同步。该字段只用于写盘。
2. `saveCredential` 删除凭证文件时不再先 `MkdirAll` 数据目录；目录不存在时旧代码会建一个空目录。
3. `playqueue.Queue.Replace` 把当前位置清空，随后立即由 `playAt(i)` 选中；旧代码在这两步之间保留旧的 `cur`，期间没有读取。
4. `removeFromQueue` 删除当前曲时，重新洗牌发生在 `playAt` 之前而非之后；洗牌本身是随机的，顺序不可观测。
5. 清空队列时随机顺序也一并清空（旧代码保留旧长度的数组，在下次使用前由 `shuffleStep` 懒修复）；队列为空时不会使用它。

## 8. 遗留风险

- 加载编排仍无离线测试（D-06）；本次未改其函数体，风险集中在手动走查。
- 已跑 `/code-review`（high），10 项发现全部处理（D-14）；D-14、D-15 的改动经第二次 `/code-review` 得到 9 项发现，已处理（D-16）；D-16 的改动尚未再次审查。
- GUI 只做了启动冒烟测试；交互项（播放、切歌、点赞、搜索）未走查（§4）。
