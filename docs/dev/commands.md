# 命令

在模块根目录执行（PowerShell）。日常最常用的几条也列在 [`AGENTS.md`](../../AGENTS.md) §4。

## 1. 运行与开发

| 目的 | 命令 |
|---|---|
| 直接运行 | `go run ./cmd/qqmusic` |
| 热重编译运行 | `go tool mygo dev`，或 `.\启动run.bat`（改代码保存后自动重编译重启，产物在 `.mygo/dev/`） |
| 本地 GUI 构建 | `go build -ldflags "-H=windowsgui" -o build/QQMusic.exe ./cmd/qqmusic` |
| 内存诊断页 | `$env:QQMUSIC_PPROF="127.0.0.1:6060"; go run ./cmd/qqmusic`，然后打开 `http://127.0.0.1:6060/mem` |

## 2. 测试与检查

| 目的 | 命令 |
|---|---|
| 只跑不联网的测试（日常用这个） | `go test -short ./...` |
| 跑全部测试（**会访问线上接口**） | `go test ./...` |
| 持久化兼容测试（旧版本写的文件仍可读、保存后可读回） | `go test ./internal/app -run TestPersistedFiles`；`testdata/v1.0.1/` 是 v1.0.1 写出的文件，不要重新生成 |
| 静态检查 | `go vet ./...`；MyGo 专用 `go tool mygo vet .` |
| UI 截图（测试用，会联网） | `$env:SHOTS="C:\shots"; go test ./internal/app -run TestPages -v` |
| 解码器测试 | `$env:PLAYER_TEST_URL="<mp3/flac/m4a 的 url 或本地路径>"; go test ./internal/player -run TestStreamDecoder -v`（先下载到本地反复测，见 `AGENTS.md` §2.11） |

## 3. 接口探针（用真实账号联网，遵守 `AGENTS.md` §2.11）

| 目的 | 命令 |
|---|---|
| 接口冒烟测试（公开接口） | `go run ./cmd/probe` |
| 扫码登录后测用户接口/播放地址 | `go run ./cmd/probe -login qq -cred cred.json` |
| 一次性探针（`cmd/zz_*`、`internal/app/zz_feed_test.go`，带构建标签） | `go run -tags probe ./cmd/zz_feeddump <cred.json>`；`go test -tags probe -run TestZZFeed ./internal/app` |

## 4. 图标

| 目的 | 命令 |
|---|---|
| 生成图标（`icon.png` / `icon.ico` / `icon-256.png` / `tray.png`） | `go run ./cmd/genicon` |
| 把 `icon.ico` 写进 exe 资源 | `go run ./cmd/genicon -syso` |

## 5. 打包与发布

发布流程见 [`release.md`](release.md)。

| 目的 | 命令 |
|---|---|
| 打包脚本（Windows） | `.\打包build.bat [x.y.z]`（传版本号会先写入 `mygo.json`；不传则用现有版本） |
| 正式打包（exe + NSIS 安装包） | `go tool mygo build` |
| macOS 打包并安装 | `./打包build.sh [x.y.z] [--universal] [--dmg] [--no-install] [--no-launch]`（打包 `build/darwin-<arch>/QQMusic.app`，替换 `/Applications/QQMusic.app` 并启动；`--universal` 出通用二进制，`--dmg` 另外生成 dmg） |
| GitHub Actions 跨平台发布 | 改 `mygo.json` 的版本号后提交，推 `main` 和对应的 `v<version>` tag；工作流同时生成 exe、NSIS 安装包与 macOS 通用 dmg，并发布到同一个 GitHub Release |
| 发布 GitHub Release | `gh release create v<version> build/release/QQMusic_v<version>_Portable.exe build/release/QQMusic_v<version>_Setup.exe --verify-tag --title "QQ音乐桌面版 v<version>" --notes "…"` |

打包产物：

- `build/windows-amd64/QQMusic.exe`
- `build/release/QQMusic_v<version>_Portable.exe`
- `build/release/QQMusic_v<version>_Setup.exe`
- `build/darwin-<arch>/QQMusic.app`（macOS，`--dmg` 时同一目录还有 `<slug>-<version>-<arch>.dmg`）
