# 性能记录规范

## 目录

每轮写到一个独立目录：

```text
docs/performance/runs/<UTC timestamp>-<label>/
├── run.json
├── metrics.jsonl
├── summary.md
├── metrics.md
├── notes/
│   └── P01-HOME.md
├── profiles/
│   ├── P01-HOME-settled.cpu.pb.gz
│   ├── P01-HOME-settled.cpu-top.txt
│   ├── P01-HOME-after_gc.heap.pb.gz
│   ├── P01-HOME-after_gc.heap-inuse-top.txt
│   └── P01-HOME-after_gc.heap-alloc-top.txt
└── screenshots/
```

`run.json`、`metrics.jsonl` 和 pprof 是原始记录，不随后续总结改写。允许重跑同一个 case；用 `phase` 或同名文件的时间/序号区分，不能覆盖旧样本。

## run.json

由 `scripts/perf_record.sh init` 创建：

- `schema_version`
- `run_id`
- `created_at`
- `git_commit`
- `git_dirty`
- `host_os`
- `host_arch`
- `pprof_url`

实际测试的二进制、窗口尺寸、账号状态和用户批准范围写入 `summary.md`。

## metrics.jsonl

一行一个不可变事件。snapshot 至少有：

```json
{"schema_version":1,"run_id":"20261030T020000Z-full","case_id":"P01-HOME","phase":"settled","captured_at":"2026-10-30T02:10:00Z","pid":50292,"rss_bytes":324160000,"heap_live_bytes":54111984,"heap_free_bytes":4849664,"goroutines":38,"process_cpu_percent":0.0,"note":"feed fully scrolled"}
```

profile 至少记录：

- `kind`: `cpu` / `heap`
- `case_id`, `phase`
- `seconds`（CPU）
- `profile_file`
- `top_file`
- `duration_line`
- `samples_line`

不要把“0 CPU”当成页面未运行；短样本只是说明该页大多数时间在等待或渲染线程未被 Go CPU profile 采到。必须和截图、AX 状态、RSS/heap、交互完成情况一起判断。

## notes/<case>.md

每个 case 一个文件，模板：

```markdown
# P01-HOME 首页信息流

- 状态：passed / failed / skipped
- 实际页面：
- 数据来源：
- 窗口尺寸：
- 账号态：
- 前置状态：
- 执行动作：
- 加载结果：
- CPU profile：
- Settled：
- After GC：
- 明显卡顿：
- 可疑热点：
- 未执行的有副作用动作：
- 备注：
```

## summary.md

必须包含：

1. 环境、构建、提交、脏工作树、账号与平台限制。
2. 覆盖表：每个 case 的状态和记录链接。
3. 全矩阵关键指标：基线、峰值 RSS、峰值 Go live heap、结束 after-GC、goroutine 范围。
4. CPU profile 热点：应用代码、MyGo 框架、网络、图片解码、GC/运行时分别归因。
5. 页面异常与失败。
6. 优化前后对比（若有）：同 case、同动作、同采样时长。
7. 结论与下一轮建议。

## 判定

- `passed`：页面成功加载，矩阵动作完成，记录齐全，未发现持续增长或不可接受卡顿。
- `failed`：页面加载失败、动作无法完成、记录缺失，或存在需要修复的性能回归。
- `skipped`：条件不满足；必须写具体原因和已经完成的替代检查。

需要继续调查的自动候选条件：

- 同一页面 after-GC Go live heap 比前一个稳定点增加超过 `16 MiB`，且下一页仍不回落。
- 页面稳定后 goroutine 比基线多 `10` 个以上，或每个同类页面都持续上升。
- CPU profile 中 QQ Music 业务代码占应用采样明显，或 `images.load`/`view`/序列化/文件 IO 出现新的高累计热点。
- 用户可感知的输入延迟、滚动掉帧、动画停顿，即使 CPU profile 样本不高，也要记录并考虑用更长采样复现。

不要仅凭 RSS 增长判定 Go 泄漏；MyGo/Metal/系统缓存会保留内存。必须同时看 Go live heap、free heap、GC 后变化和 profile。
