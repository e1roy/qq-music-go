---
name: local-refactor
description: 对本仓库的 Go + MyGo 桌面客户端执行分阶段、可验证的架构重构：先只读审计真实代码，再设计目标分层、制定路线图，然后按可逆单元逐个实施并验证。当用户要求重构、拆分文件、分离视图与业务逻辑、继续执行 docs/refactor 中的计划，或审查/验收一次重构时使用。
---

# 本地重构（local-refactor）

## 概览

用可审查的增量代替整体重写：每个重构单元都要有代码证据、验证命令和回滚点。完整方法论在 `references/refactor-playbook.md`（保留 §1–§10 编号，仓库文档按该编号引用），本文件只写执行流程与本项目的既定约束。

## 开工前

1. 读 `AGENTS.md`（§2 硬性约束、§3 代码规范）、`docs/dev/pitfalls.md`（已知坑）、`docs/dev/codemap.md` 与 `references/refactor-playbook.md`。
2. 跑 `git status`。工作区已有未提交改动时，先确认是否在其上重构；不得覆盖、回滚或提交用户的改动。
3. 读 `docs/refactor/`。已有审计、决策、路线图时直接复用，不重复调查，也不推翻已记录的决策（D-xx）。
4. 明确授权边界：默认只做结构重构、不改行为；方案获得确认前不修改业务代码。

## 阶段与产物

| 阶段 | 是否改代码 | 产物 | 完成标准 |
|---|---|---|---|
| 一 审计 | 只读 | `docs/refactor/architecture-audit.md` | 问题带编号、文件位置、代码证据、影响与风险 |
| 二 目标架构 | 只读 | `docs/refactor/target-architecture.md` | 职责、依赖方向、最小接口、迁移映射、不选其他方案的理由 |
| 三 路线图 | 只读 | `docs/refactor/refactor-roadmap.md` | 阶段拆分，每阶段有前置条件、验证命令、回滚策略 |
| 四 执行 | 改代码 | 每个单元一个可审查 diff | 一个单元验证通过后才进入下一个 |
| 五 收尾 | 改代码 | `docs/refactor/verification-report.md`、`decisions.md` | 基线、真实测试输出、兼容性证据、遗留风险 |

## 本项目已确认的架构边界

参见 `docs/refactor/decisions.md` 与 `target-architecture.md`，新方案与它们冲突时要先说明理由：

- `internal/app` 保持**一个 Go 包**，包内按文件分层：视图（`view_*.go`）→ 编排（`app` 方法）→ 纯规则包与接口层。MyGo 视图需要直接读 `*app` 状态，拆包会迫使大量状态导出。
- 视图只读状态、收集用户意图；不联网、不读写文件、不调用 `a.do(`（AGENTS.md §2.12）。
- 编排在主线程改状态；耗时操作走 `a.do`，结果经 `platform.Post` 回到主线程，用代际号丢弃过期结果（`docs/dev/architecture.md` §2）。
- 只有「有不变量、可脱离 UI 测试」的规则才提成独立包（现有 `internal/playqueue`、`internal/store`）。`qqmusic`、`xiaoyuzhou`、`player` 已合格，不在重构范围内。
- 不引入 UseCase 类型、DI 框架、事件总线、Repository 大接口、页面注册表、ViewModel；不为假想需求建通用框架。
- 删除或新建文件后同步更新 `docs/dev/codemap.md`；架构约定变化同步更新 `docs/dev/architecture.md` / `conventions.md`，硬性约束变化才改 `AGENTS.md`。

## 执行循环（单个重构单元）

1. 读该单元涉及的完整实现与**全部**调用方（含测试、`cmd/`、构建标签文件），不要只看最近修改的部分。
2. 写下旧职责、目标职责，以及必须保持不变的行为：异步顺序、默认值、错误文案、持久化格式。
3. 先补测试或确认已有测试覆盖；遗留逻辑用特征测试记录现状（playbook §3.4）。
4. 做最小的可逆结构变换：移动、提取、加薄适配层。不要同时改数据结构、业务规则和 UI 行为。
5. 把调用方切到新接口；删除旧实现前确认没有动态调用、配置或注册表引用。
6. 跑验证命令，逐行检查 `git diff` 确认没有混入无关修改。
7. 在 `docs/refactor/refactor-roadmap.md` 记录进度、未完成项和剩余风险。

## 验证

在模块根目录执行（完整命令表见 `docs/dev/commands.md`）：

```powershell
$env:CGO_ENABLED = "0"
go build ./...
go vet ./...
go tool mygo vet .
go test -short -count=1 ./...
$env:CGO_ENABLED = "1"; go test -race -short -count=1 ./internal/app ./internal/playqueue ./internal/store
```

- 视图层依赖检查（辅助手段，不是门禁）：`grep -nE 'a\.do\(|\bos\.|\bjson\.|\.api\.[A-Za-z]+\(' internal/app/view*.go`。不要为了通过它新增只转发一次的包装方法。
- 新增测试必须离线：用 `offlineTransport`（`behavior_test.go`）、`episodes(...)`、`fakeLikes`（`docs/dev/conventions.md` §5）。联网测试加 `testing.Short()` 跳过。
- 布局/交互改动：`$env:SHOTS="C:\shots"; go test ./internal/app -run TestPages -v` 截图走查，并实际启动一次窗口。
- 不要用 `go test ./...` 做回归（会联网）；不要批量请求 `SongURL` 等登录接口，触发 `104009` 后立即停止（AGENTS.md §2.11）。
- 持久化格式改动必须过 `TestPersistedFiles`（`testdata/v1.0.1/` 是旧版本写出、已脱敏的固件，不要重新生成）。

## 硬约束

- 保持用户可观察行为一致；发现既有 Bug 单独记录，不与结构重构混在同一改动里。
- 不删除无法证明无用的代码，不批量重命名或格式化无关代码，不改变持久化格式与配置契约。
- `cmd/zz_feeddump`、`cmd/zz_podcast`、`internal/app/zz_feed_test.go` 是保留的探针，只在 `-tags probe` 下运行；不要删除，也不要去掉构建标签。
- 不新增依赖、不引入 cgo、不改 MyGo 版本（`mygo-maintenance` skill 优先于本文件）。
- 不提交、不推送、不合并，除非用户明确要求。

## 停止点与报告

- 未获明确执行授权时，完成审计与目标架构后停止，等待确认（playbook §10）。
- 每个单元结束报告：改动范围、代码证据、真实验证输出、未覆盖的验证缺口。
- 无法证明行为等价时如实说明缺口，不虚构测试通过，也不以「测试全绿」代替兼容性论证。

## 参考

- `references/refactor-playbook.md`：完整方法论（审计清单、抽象判断标准、阶段划分、可量化验收标准）。
- `docs/refactor/*.md`：上一次重构的真实产物，可作为文档格式范例。
- `AGENTS.md` §2、§3：硬性约束与代码规范；`docs/dev/commands.md`、`conventions.md`、`pitfalls.md`：命令、代码约定与已知坑。
