# 提交与发布

**打包与发 Release 只在用户明确要求时做**。用户只说「提交」「推送」时，过完第 1 节清单就提交、推送，不打包、不打 tag、不发 Release；不确定用户要不要发布时先问，禁止擅自打包或发布。

用户要求发布时，必须完整走完第 2 节，确认同一个 GitHub Release 已带上对应平台的附件。只推 tag 不算完成。跨平台发布优先用 `.github/workflows/release.yml`；本机 Windows 打包留在本机产物或 Actions 不可用时使用。

## 1. 提交前检查清单

- [ ] `go test -short ./...` 通过
- [ ] `go vet ./...` 与 `go tool mygo vet .` 通过
- [ ] 视图层不联网、不读写文件（`AGENTS.md` §2.12），辅助检查无输出：`grep -nE 'a\.do\(|\bos\.|\bjson\.|\b(a|x)\.(xiaoyuzhou\.)?api\.[A-Za-z]+\(' internal/app/view*.go | grep -vE 'api\.(LoggedIn|Credential)\(\)'`
- [ ] 涉及网络/播放/登录时，手动运行确认
- [ ] 没有在 `view` 构建期引入副作用
- [ ] 新文件/页面/接口已更新 [`codemap.md`](codemap.md)
- [ ] 未提交 `cred.json`、`build/`、`*.syso` 等生成物
- [ ] 用户可见文案为中文，代码注释为英文
- [ ] 动效：新的出现/消失/切换/刷新按 [`motion.md`](motion.md) 的检查清单过了一遍
- [ ] 仅当用户要求发布时：已 push 代码与 tag；GitHub Actions 已生成对应平台附件，或本机已生成 `build/release/` 下的两个 exe，并确认 GitHub Release 附件完整（第 2 节）

## 2. 发布步骤（仅在用户要求发布时）

### 2.1 GitHub Actions 跨平台发布（推荐）

1. **定版本**：改 `mygo.json` 的 `version`。版本号 `x.y.z`，tag 为 `vx.y.z`，两者必须一致；「关于」页通过 `mygo.App.Version()` 读取这个版本，不需要再单独改代码。
2. **验证**：`go test -short ./...`、`go vet ./...`、`go tool mygo vet .`，并按第 1 节清单过一遍。
3. **提交并推送**：

   ```sh
   git add .
   git commit -m "..."
   git push origin main
   git tag v<version>
   git push origin v<version>
   ```

4. **等待工作流**：`.github/workflows/release.yml` 会在 macOS runner 上构建 `darwin/universal`，在 Ubuntu runner 上构建 `windows/amd64` 并安装 NSIS。两端都成功后，最终 job 把三个附件发布到同一个 Release：
   - `QQMusic_v<version>_Portable.exe`
   - `QQMusic_v<version>_Setup.exe`
   - `QQMusic_v<version>_macOS.dmg`
5. **检查 Release**：用 `gh run watch` / `gh run view` 确认工作流成功，再用 `gh release view v<version>` 确认三个附件、标题和固定的 macOS 解隔离命令。自动生成的说明按第 3 节改写。

已有 `v<version>` tag（例如只是补发 Release）时不要重建或移动它；修正工作流后重跑失败 job 即可。

### 2.2 本机 Windows 发布（备用）

1. **定版本**：`.\打包build.bat x.y.z` 会先把版本写进 `mygo.json` 再打包。
2. **打包**：`.\打包build.bat`（不带参数则沿用 `mygo.json` 的版本）。打包前先关掉正在运行的 QQMusicGo，否则复制 exe 会失败。产物：
   - `build/release/QQMusic_v<version>_Portable.exe`
   - `build/release/QQMusic_v<version>_Setup.exe`
3. **提交并推送代码与 tag**，然后发 Release：

   ```powershell
   gh release create v<version> build/release/QQMusic_v<version>_Portable.exe build/release/QQMusic_v<version>_Setup.exe --verify-tag --title "QQ音乐桌面版 v<version>" --notes "中文说明"
   ```

   - 标题固定 `QQ音乐桌面版 v<version>`。
   - `build/` 已被 git 忽略，exe 只能通过 `gh release create/upload` 上传，不要为了让它们入库而改 `.gitignore`。
   - Release 已存在、只是补传附件时用 `gh release upload v<version> <文件路径> --clobber`，不要删了重建。

## 3. Release 说明怎么写

写给用户看，只说**加了什么功能、修了什么问题**，一条一句话：

- 按「新增」「修复」分组，没有的组不写。
- 从用户角度描述现象或功能，比如「歌曲卡片的「⋯」菜单点了没反应」，不写原因、实现方式、函数名、测试名和参数。
- 动效、过渡这类纯视觉的小调整不写；一个版本只有这类改动时写一句「界面细节优化」。
- 不重复以前版本的内容，也不列附件清单。
- 每条 Release 都必须包含以下 macOS 解隔离命令，工作流会自动写入；手工发布时也要原样加上：

  ````text
  macOS 包需在终端执行

  ```sh
  xattr -dr com.apple.quarantine /Applications/QQMusicGo.app
  ```
  ````

- 末尾固定一句：「Portable 免安装，Setup 为 NSIS 安装包；旧数据可直接沿用。」

示例：

```text
新增
- 设置里可以调整歌曲缓存上限

修复
- 歌曲卡片的「⋯」菜单点了没反应

macOS 包需在终端执行

```sh
xattr -dr com.apple.quarantine /Applications/QQMusicGo.app
```

Portable 免安装，Setup 为 NSIS 安装包；旧数据可直接沿用。
```
