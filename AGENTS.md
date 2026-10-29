# AGENTS.md

面向在本仓库工作的 AI 代理与开发者。本文件只放每次都要遵守的规则和文档索引，细节在 `docs/dev/`。改动代码前先读完本文件，再按 §5 的索引读与任务相关的文档。

## 1. 项目是什么

用 **纯 Go** 写的 Windows 桌面 QQ 音乐客户端。界面使用 [MyGo](https://github.com/egoist/mygo) 原生 UI（`github.com/egoist/mygo/ui`），**没有 WebView、没有 cgo、没有前端工程**。视觉上是 shadcn/ui 的 Nova 深色风格，主色为 QQ 音乐绿（`#31c27c`）。

- 模块名：`qqmusic`；Go 版本：`1.27.3`（`GOTOOLCHAIN=auto`）
- 平台重点：Windows（托盘、`%AppData%`、`.syso` 图标、NSIS 安装包）
- 数据来源：QQ 音乐网页端**非公开接口** `https://u.y.qq.com/cgi-bin/musicu.fcg` 及若干 `c.y.qq.com` / `c6.y.qq.com` 接口

## 2. 硬性约束

改代码时必须遵守（其他文档按 §2.N 引用，编号不要重排）：

1. **纯 Go / 无 cgo**：`CGO_ENABLED=0`。不要引入需要 cgo 或本地动态库的依赖。
2. **主界面只用 MyGo**：不要引入前端工程、Electron、Wails 或 HTML 界面。例外只有两个登录窗口：「QQ 账号」登录（`desktop.OpenQQLogin`，QQ 官方登录页）和小宇宙「手机号登录」（`desktop.OpenXiaoyuzhouLogin`，`accounts.xiaoyuzhoufm.com` 官方登录页，因为短信验证码要过阿里云滑块验证）。只用于登录，不要把它们扩展成应用主界面。
3. **主线程模型**：`app` 的状态只在 UI 线程读写；网络/解码等耗时操作走 `a.do`，结果通过 `platform.Post` 回到主线程（见 [`architecture.md`](docs/dev/architecture.md) §2）。
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
12. **视图层不联网、不读写文件**：`view*.go` 中不出现 `a.do(`、`os.`、`json.`，也不调用接口客户端的请求方法；需要时在编排层（`library.go`、`settings.go`、`session.go` 等）加方法。读取登录状态（`a.api.LoggedIn()`、`Credential()`）和在点击回调里 `a.saveSettings()` 是允许的。这是指导原则，[`release.md`](docs/dev/release.md) 清单里的 grep 只是辅助：不要为了通过它而加只转发一次的包装方法。
13. **`xyz` 是参考项目，不是本项目的名字**：它的代码只能参考，不能复制；小宇宙相关的包、文件、标识符统一叫 `xiaoyuzhou`（`internal/xiaoyuzhou`、`xiaoyuzhou.go`、`pageXiaoyuzhou`…），不要再用 `xyz` 命名。`xyz` 只允许出现在兼容旧数据的常量（`fileOldXiaoyuzhou`、`oldXiaoyuzhouSource`、`settings.OldOfficial`）和服务端协议参数（`clientId: "xyz-web"`、CDN 域名 `xyzcdn.net`）里。

## 3. 代码规范

1. 要为一个推测出来的、几乎不会发生的并发或安全边界加复杂度之前，先跟用户讲清楚：具体会出什么问题、概率多大、要多写多少，用户点头了再加。不许为了「理论上更稳」悄悄扩大范围。
2. 外面进来的数据只在入口校验一次，往里传就用定好类型的值，不要每一层再判断一遍。
3. 只用一次的逻辑不要提前抽成函数，直接写在调用的地方，除非它会复用，或者真藏着一块复杂的东西。
4. 能不用 try/catch 就不用，能一个函数写完就别拆。
5. 文档注释用英文、解释“为什么”；界面文案与错误提示用简体中文（经 `errText`）。其余约定见 [`conventions.md`](docs/dev/conventions.md)。
6. 做功能必须考虑动效：出现、消失、切换、刷新、加载都要有明确决定（加或明确不加），按 [`motion.md`](docs/dev/motion.md)。

## 4. 常用命令

在模块根目录执行（PowerShell），完整命令表见 [`commands.md`](docs/dev/commands.md)：

| 目的 | 命令 |
|---|---|
| 运行 / 热重编译运行 | `go run ./cmd/qqmusic` / `go tool mygo dev` |
| 测试（不联网，日常用这个；`go test ./...` 会访问线上接口） | `go test -short ./...` |
| 静态检查 | `go vet ./...`；`go tool mygo vet .` |
| 打包（Portable + Setup） | `.\打包build.bat [x.y.z]` |

## 5. 文档索引

| 想做什么 | 看哪里 |
|---|---|
| 找文件/符号、「要改什么先看哪」 | [`docs/dev/codemap.md`](docs/dev/codemap.md) |
| 理解分层、线程与异步、导航、播放/登录链路、持久化与歌曲缓存 | [`docs/dev/architecture.md`](docs/dev/architecture.md) |
| 命名、状态与加载、视图、接口、测试约定；MyGo 维护要点 | [`docs/dev/conventions.md`](docs/dev/conventions.md) |
| 新增页面 / 接口 / 图标 / 设置项，调主题，查内存与卡顿 | [`docs/dev/howto.md`](docs/dev/howto.md) |
| 动效原则、参数、检查清单 | [`docs/dev/motion.md`](docs/dev/motion.md) |
| 排查问题（已知坑） | [`docs/dev/pitfalls.md`](docs/dev/pitfalls.md) |
| 全部命令与打包产物 | [`docs/dev/commands.md`](docs/dev/commands.md) |
| 提交前检查清单、打包与发 Release | [`docs/dev/release.md`](docs/dev/release.md) |
| QQ 音乐推荐信息流接口 | [`docs/api/qqmusic-feed.md`](docs/api/qqmusic-feed.md) |
| QQ 音乐内置播客（小宇宙内容）接口 | [`docs/api/qqmusic-podcast.md`](docs/api/qqmusic-podcast.md) |
| 小宇宙官方接口 | [`docs/api/xiaoyuzhou.md`](docs/api/xiaoyuzhou.md) |
| 上一次重构的审计、目标架构、决策 | [`docs/refactor/`](docs/refactor/) |

`docs/` 的完整目录见 [`docs/README.md`](docs/README.md)。文档维护：新增/删除文件、页面、接口后更新 `codemap.md`；架构或约定变了更新对应文档；新踩的坑追加到 `pitfalls.md` 末尾（编号不重排）；新文档按 `docs/README.md` 的规则放进 `dev/`、`api/` 或 `images/`。只有每次都要遵守的规则才写进本文件。

## 6. 提交与发布

- 提交前先过 [`release.md`](docs/dev/release.md) 的检查清单。
- **打包与发 Release 必须由用户明确要求**。用户只说「提交」「推送」时，只提交、推送代码，不打包、不打 tag、不发 Release；如果不确定用户是否需要，先询问。**禁止擅自打包或发布 Release。**
- 用户要求发布时，按 `release.md` 第 2 节的顺序执行：打包 exe → 推送代码与 tag → 发带两个 exe 附件的 GitHub Release。
