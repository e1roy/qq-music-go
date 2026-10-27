# 重构路线图

> 状态：Phase 0 – Phase 4 全部执行，含 U6；审查发现与现有 Bug 已修复。全部改动**未提交**，等待审查。按 `/goal` 指令不停下来确认，待确认的决策采用了建议的默认值，见 decisions.md D-12。

## 通用约定

**每个单元的验证命令**（模块根目录）：

```powershell
$env:CGO_ENABLED = "0"
go build ./...
go vet ./...
go tool mygo vet .
go test -short -count=1 ./...
$env:CGO_ENABLED = "1"; go test -race -short -count=1 ./internal/app ./internal/playqueue ./internal/store
```

**依赖检查**（target-architecture §4，辅助而非门禁，见 D-14）：

```sh
grep -nE 'a\.do\(|\bos\.|\bjson\.|\b(a|x)\.(xyz\.)?api\.[A-Za-z]+\(' internal/app/view*.go | grep -vE 'api\.(LoggedIn|Credential)\(\)'
go list -deps ./internal/playqueue ./internal/store | grep -E 'mygo|internal/app'
```

**回滚**：未提交。整体回滚：`git restore .`，再删除新增文件（`git status` 中的 `??` 项）。如需按单元审查，建议按下表顺序分批暂存、提交。

**联网约束**：所有新增测试离线（`offlineTransport`）；没有调用 `SongURL`。

---

## 进度

| 单元 | 解决 | 状态 | 主要改动 |
|---|---|---|---|
| T0.1 探针文件 | P-14 | ✅ | `cmd/zz_feeddump`、`cmd/zz_podcast`、`internal/app/zz_feed_test.go` 加 `//go:build probe`（D-09 b） |
| T0.2 特征测试 | — | ✅ | `internal/app/behavior_test.go`：`TestQueueEdits`、`TestNextPrev`、`TestPlayFailures`、`TestPersistedFiles`（`testdata/v1.0.1/`，审查后由字节比对改为兼容读取）、`TestSearchScope`；音质徽章改在 U1 后用 `TestQualityLabel` 覆盖（UI 层无法观测，见 verification-report §4） |
| U1 视图层去数据访问 | P-01/02/03 | ✅ | `playPage` 移出视图并与 `loadList` 共用 `listSongs`；`setQuality`；`qualityLabel`（审查后内联了只转发一次的包装，见 D-14） |
| U2 播放队列 | P-09/08 | ✅ | 新包 `internal/playqueue`；`playback.go` 只做编排；删除复制规则的 `TestPlayModes`；审查后：零值即空队列、`Restore` 复制切片 |
| U3 来源谓词 | P-05 | ✅ | `fromQQ`、`isXyz`、`qqEpisode`；`.Source` 只剩谓词定义与 `episodeSong` 构造；BUG-5 的不一致**保留** |
| U4 搜索作用域 | P-07 | ✅ | `searchScope()` + `scopeMusic/scopePodcasts/scopeXyz` |
| U5 最近使用列表 | — | ✅ | `pushFront`，`addRecent`/`rememberSearch`/`hearShow` 共用 |
| U6 合并分页 | P-04 | ✅ | `pager[T]` + `loadPage`（pager.go），见 D-15 |
| U7 存储模块 | P-10 | ✅ | 新包 `internal/store`（审查后改为原子写）；`settings.go` 集中文件名、记录类型与读写；`podcast.go`/`xyz.go` 改用 store |
| U8 状态分组 | P-13 | ✅（调整） | 未改字段名，按所有者重排 `app` 的播放字段并注释（播放 / 歌词 / 视图自身状态），避免数百处改名 |
| U9 构建期加载约定 | P-11 | ✅（仅文档） | AGENTS.md §2.4 写明两个幂等例外；代码不改 |
| U10 拆分 `app.go` | P-13/D-08 | ✅ | 纯移动到 `nav.go`、`library.go`、`search.go`、`session.go`（行级比对确认只移动，见 verification-report §6） |
| U11 清理与文档 | — | ✅ | CODEMAP.md、AGENTS.md（§2.4、§2.12、§3、§5.1、§5.6、§6.5、§7.1、§7.2、§7.5、§10）更新；未引入兼容别名，无需移除 |
| 审查修复 | — | ✅ | `/code-review` 的 10 项发现全部处理，见 decisions.md D-14 |
| Bug 修复 | — | ✅ | BUG-3/4/5，见 D-15 |
| 启动冒烟测试 | — | ✅ | 用真实数据启动 GUI 并关闭，见 verification-report §4 |

## 尚未做

- 需要人操作的手动走查（verification-report §4 列出的交互项）。
