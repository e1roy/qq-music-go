# 提交与发布

**打包与发 Release 只在用户明确要求时做**。用户只说「提交」「推送」时，过完第 1 节清单就提交、推送，不打包、不打 tag、不发 Release；不确定用户要不要发布时先问，禁止擅自打包或发布。

用户要求发布时，必须完整走完第 2 节：先打包 exe，再推送代码与 tag，最后发带两个附件的 GitHub Release。只推 tag 不算完成。

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
- [ ] 仅当用户要求发布时：已用 `.\打包build.bat` 生成 `build/release/` 下的两个 exe，已 push 代码与 tag，并创建 GitHub Release、上传两个 exe（第 2 节）

## 2. 发布步骤（仅在用户要求发布时，按这个顺序）

1. **定版本**：改 `mygo.json` 的 `version`，或直接 `.\打包build.bat x.y.z`（会先把版本写进 `mygo.json` 再打包）。版本号 `x.y.z`，tag 为 `vx.y.z`，两者必须一致。
2. **打包**：`.\打包build.bat`（不带参数则沿用 `mygo.json` 的版本）。打包前先关掉正在运行的 QQMusic，否则复制 exe 会失败。产物：
   - `build/release/QQMusic_v<version>_Portable.exe`（免安装，单文件即应用）
   - `build/release/QQMusic_v<version>_Setup.exe`（NSIS 安装包）
3. **验证**：`go test -short ./...`、`go vet ./...`、`go tool mygo vet .`，并按第 1 节清单过一遍。
4. **提交并推送**：

   ```powershell
   git add -A; git commit -m "..."
   git tag v<version>
   git push; git push origin v<version>
   ```

   已有 `v<version>` tag（例如只是补发 Release）就不要重建或移动它，直接推送并发布。
5. **发 Release**（`gh` 已登录，发布前 `gh auth status` 确认账号是 `e1roy`）：

   ```powershell
   gh release create v<version> build/release/QQMusic_v<version>_Portable.exe build/release/QQMusic_v<version>_Setup.exe --verify-tag --title "QQ音乐桌面版 v<version>" --notes "中文说明"
   ```

   - 标题固定 `QQ音乐桌面版 v<version>`，两条附件缺一不可。
   - `build/` 已被 git 忽略，exe 只能通过 `gh release create/upload` 上传，不要为了让它们入库而改 `.gitignore`。
   - 说明按第 3 节写。
   - Release 已存在、只是补传附件时用 `gh release upload v<version> <两个 exe 路径> --clobber`，不要删了重建。

## 3. Release 说明怎么写

写给用户看，只说**加了什么功能、修了什么问题**，一条一句话：

- 按「新增」「修复」分组，没有的组不写。
- 从用户角度描述现象或功能，比如「歌曲卡片的「⋯」菜单点了没反应」，不写原因、实现方式、函数名、测试名和参数。
- 不重复以前版本的内容，也不列附件清单。
- 末尾固定一句：「Portable 免安装，Setup 为 NSIS 安装包；旧数据可直接沿用。」

示例：

```text
新增
- 设置里可以调整歌曲缓存上限

修复
- 歌曲卡片的「⋯」菜单点了没反应
- 「我喜欢」里取消喜欢后，下面的歌曲直接跳上去而不是滑上来

Portable 免安装，Setup 为 NSIS 安装包；旧数据可直接沿用。
```
