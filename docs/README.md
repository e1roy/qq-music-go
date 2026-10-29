# 文档目录

入口是根目录的 [`AGENTS.md`](../AGENTS.md)（硬性约束与代码规范）；用户说明见根目录 [`README.md`](../README.md)。

```text
docs/
├── dev/        开发文档：怎么改这个项目
├── api/        接口资料：非公开接口的分析与实测记录
├── refactor/   上一次架构重构的产物（历史记录，local-refactor skill 按此路径读写）
└── images/     README 用的截图
```

## dev/ 开发文档

| 文件 | 内容 |
|---|---|
| [`codemap.md`](dev/codemap.md) | 目录总览、各包文件职责与关键符号、按任务快速定位 |
| [`architecture.md`](dev/architecture.md) | 分层、线程与异步、导航、播放/登录链路、持久化与歌曲缓存 |
| [`conventions.md`](dev/conventions.md) | 命名、状态与加载、视图、接口、测试约定；MyGo 维护要点 |
| [`howto.md`](dev/howto.md) | 新增页面 / 接口 / 图标 / 设置项，调主题，查内存与卡顿 |
| [`motion.md`](dev/motion.md) | 动效原则、参数总表、检查清单、明确不加动效的地方 |
| [`pitfalls.md`](dev/pitfalls.md) | 已知坑（编号只追加不重排） |
| [`commands.md`](dev/commands.md) | 全部命令与打包产物 |
| [`release.md`](dev/release.md) | 提交前检查清单、打包与发 GitHub Release |

## api/ 接口资料

都是非公开接口，字段随时可能变；文中不记录任何凭证。请求量遵守 `AGENTS.md` §2.11。

| 文件 | 内容 |
|---|---|
| [`qqmusic-feed.md`](api/qqmusic-feed.md) | QQ 音乐推荐信息流（音乐盲盒 / 碎片时间 / 你的私荐歌单）：分析流程、接口规格、卡片类型与实测 |
| [`qqmusic-podcast.md`](api/qqmusic-podcast.md) | QQ 音乐内置「小宇宙」：播客专区、推荐、节目、单集、主播、播放与字幕接口 |
| [`xiaoyuzhou.md`](api/xiaoyuzhou.md) | 小宇宙官方接口：三条通道、鉴权与请求头、接口清单、返回字段、网页通道 |

## refactor/ 重构记录

| 文件 | 内容 |
|---|---|
| [`architecture-audit.md`](refactor/architecture-audit.md) | 重构前的只读审计（问题编号 P-xx） |
| [`target-architecture.md`](refactor/target-architecture.md) | 目标分层与依赖方向（`internal/app` 按文件分层的依据） |
| [`refactor-roadmap.md`](refactor/refactor-roadmap.md) | 重构单元与进度 |
| [`decisions.md`](refactor/decisions.md) | 决策记录（D-xx） |
| [`verification-report.md`](refactor/verification-report.md) | 验证报告 |

## 放新文档

- 讲「怎么改代码」的放 `dev/`，并在本页和 `AGENTS.md` §5 的索引里加一行。
- 接口分析、抓包实测放 `api/`，文件名用英文短横线（`<来源>-<主题>.md`），标题和正文用中文。
- 图片放 `images/`。
- 不要再往 `docs/` 根目录直接放文档。
