---
name: qqmusic-performance-audit
description: 对 QQMusic Go + MyGo 桌面客户端执行逐页面、可重复的 CPU、堆、RSS 与 goroutine 性能审计；用于全量页面走查、回归复测、优化前后对比，以及要求用 pprof 证明优化效果的任务。
---

# QQMusic 性能审计

目标不是“点几下看看”，而是让页面矩阵中的每一项都有同构的采样、原始 pprof、记录和结论。默认跑完整矩阵；用户明确缩小时才允许只跑子集，并在汇总中列出未跑项。

## 开工前

1. 读 `AGENTS.md` 的 §2、§3、§5、§8、§10，以及本 skill 的 `references/page-matrix.md` 和 `references/recording-schema.md`。
2. 跑 `git status --short`。保留现有未提交改动，不覆盖、不回退用户文件。
3. 确认当前是 GUI 可用的本机会话，并记录 OS、架构、构建版本、登录状态、窗口尺寸。涉及登录态时不要退出或重新登录，除非用户明确要求。
4. 用 `scripts/perf_record.sh init <label>` 创建独立 run 目录。后续每个页面都必须写入该目录，不把结果只留在对话里。

## 启动与基线

构建本次要测的真实二进制，不要用 `go test` 的 tester 截图代替窗口性能：

```bash
./打包build.sh --no-install
QQMUSIC_PPROF=127.0.0.1:6060 \
  ./build/darwin-arm64/QQMusic.app/Contents/MacOS/QQMusic
```

Windows 或其它宿主按仓库对应构建脚本调整二进制路径，但必须保留 `QQMUSIC_PPROF`。程序启动后：

1. 等首页和登录态稳定，执行一次 `snapshot BASE-01 settled <pid>`。
2. 执行 `heap BASE-01 settled <binary>`。
3. 用 `ps` 和 `/mem.json` 交叉核对 RSS、Go live heap、free heap、goroutine；记录任何已有异常。
4. 把 app 的 CUA 绑定保留在单个会话变量中，后续只用 CUA 操作 UI。

采样命令：

```bash
skill=.agents/skills/qqmusic-performance-audit/scripts/perf_record.sh
run=$("$skill" init "full-audit")
QQMUSIC_PERF_RUN="$run" "$skill" snapshot BASE-01 settled "$pid" "logged in, idle"
QQMUSIC_PERF_RUN="$run" "$skill" heap BASE-01 settled "$binary"
```

## 每个页面怎么跑

严格按 `references/page-matrix.md` 的顺序执行。每个 case ID 至少完成以下循环：

1. 从稳定页面进入该页面，等网络、图片和动画停止。
2. 在开始点击前启动 CPU profile；默认 10 秒；分页、无限滚动、搜索和播放页用 15 秒。命令会阻塞，所以放在可并行操作的 exec session 中。
3. profile 期间执行矩阵列出的全部只读交互；需要回到顶部、切换页签、滚到底加载一页时都算在该 case 内。
4. profile 结束后立即 `snapshot` 记录 `settled`，再请求 `/mem/gc`，然后记录 `after_gc` 和 heap profile。
5. 在同一 case 的 `notes.md` 写：实际到达页面、执行过的动作、加载成功/失败、明显的卡顿、AAC/平台限制、未执行的有副作用操作。
6. 检查 `metrics.jsonl`、`.cpu.pb.gz`、`.heap.pb.gz` 和 pprof top 文本均已生成；缺任何一项都不能把该 case 标为完成。

```bash
QQMUSIC_PERF_RUN="$run" "$skill" cpu P01-HOME 10 "$binary"
QQMUSIC_PERF_RUN="$run" "$skill" snapshot P01-HOME settled "$pid" "feed fully scrolled"
curl -fsS -X POST http://127.0.0.1:6060/mem/gc -o /dev/null
QQMUSIC_PERF_RUN="$run" "$skill" snapshot P01-HOME after_gc "$pid"
QQMUSIC_PERF_RUN="$run" "$skill" heap P01-HOME after_gc "$binary"
```

CPU profile 必须先启动，再操作 UI；不要先点完再采样。每页都用相同采样秒数；若某页需要更长，记录原因。页面加载失败时仍要保存样本并记为 `failed`，不要静默换页。

## 记录与判定

记录规范和目录结构见 `references/recording-schema.md`。原始 `metrics.jsonl` 和 pprof 是事实来源，`summary.md` 是分析，不得互相替代。

每一轮至少产出：

- `run.json`：构建、环境、分支和脏工作树标记。
- `metrics.jsonl`：基线、每页 settled/after_gc、每次 profile 元数据。
- `profiles/*.cpu.pb.gz` 与 top 文本。
- `profiles/*.heap.pb.gz`、inuse top 与 alloc top 文本。
- `notes/<case>.md`：只记录该页真实操作和观察。
- `summary.md`：页面覆盖、前后对比、热点、结论、未测项。

重点区分：

- Go 活跃堆、Go free heap、RSS、GPU/原生纹理和系统文件缓存不是同一个指标。
- 累计分配大但 CPU 低、GC 后不保留的框架临时缓冲，不应误报为泄漏。
- 只有应用代码在 CPU profile 中占比明显、与帧率/输入延迟对应，或跨页面持续增长时，才提出优化。
- 优化前后必须使用同一页面、同一动作、同一采样时长和同一构建配置；只给 benchmark 而没有页面 pprof 不足以证明用户体验收益。

## 副作用与请求限制

- 默认只读：不订阅/退订、不关注/取关、不点赞、不评论、不删除、不清缓存、不修改设置。若产品页面只有切换式交互，记录“视觉路径可见，未触发写操作”。
- 播放地址只用于少量代表性播放测试，禁止遍历歌曲或节目逐首取 `SongURL`。出现 `104009` 立即停止所有播放地址请求并记录。
- 非 Windows 构建不支持 AAC 是已知限制，记录为 skipped/platform-limited，不要反复重试。
- 登录窗口属于认证面；已登录时不要为了覆盖它而退出。可在隔离的临时配置目录另起进程测试，必须先得到用户允许并确认不会动当前会话。

## 收尾

1. 跑 `scripts/perf_record.sh report "$run"` 生成可读表格。
2. 在 `summary.md` 列出所有 case 的 `passed/failed/skipped`，并解释每个 skipped。
3. 若做了优化，补跑同一矩阵或明确证明受影响的全部 case，并把 before/after 放进同一份 summary。
4. 运行必要的离线测试、race、`go vet ./...`、`go tool mygo vet .` 和视图副作用检查。
5. 保持 GUI 程序运行，最终回复给出 run 目录、覆盖数量、主要数据、剩余风险和程序的 `http://127.0.0.1:6060/mem` 地址。

## 参考

- `references/page-matrix.md`：所有主页面、页签和条件面的完整测试清单。
- `references/recording-schema.md`：run 目录、JSONL 字段、每页记录模板和判定规则。
- `scripts/perf_record.sh`：初始化 run、抓取 snapshot、CPU/heap profile、生成报告。
