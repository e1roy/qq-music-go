# 已知坑

排查问题先看这里。**编号只追加、不重排**：其他文档按「pitfalls.md 第 N 条」引用。

1. **搜索接口会随机返回 2001**：`search()` 已内置重试（2 次，间隔 400ms），不要删。同时发起的搜索越多越容易 2001，所以搜索页的歌手/歌单只在切到对应页签时才加载（`loadSearchTab`），不要改回一次全发。唯一例外是「播客」页签：按用户要求，新查询时在单曲之后立即并行发起（`loadSearchPodcasts`），单曲先显示；该页签在登录后还会并行搜索小宇宙官方（`moreXiaoyuzhouPodcastsFound`），结果排在 QQ 音乐的前面。不要再给其他页签加并行。
2. **我喜欢（dirid 201）写入会被网页端参数拒绝（80105）**：`LikeSong` 特意使用 `ct=11, cv=13020508` 的 app comm，不要“简化”成 `Web`。
3. **`Fetch` 会把 `http://` 升级为 `https://`**：封面/音频地址都依赖这一点，不要绕过。
4. **`player.Load` 会阻塞到文件开始下载**（AAC 要等 MP4 的索引 `moov` 下完：在文件头时几百 KB，在文件尾时等于整个文件）：必须在 `a.do` 的 goroutine 中调用，不能在 UI 线程直接调用。
5. **播放地址约一天后失效**：不要长期缓存 `SongURL` 结果。
6. **请求太多会被限流（104009）**：短时间内大量调用 `music.vkey.GetVkey` 后，所有歌曲的播放地址都返回 `code 104009`，包括之前能播的歌。测试时的请求量限制见 `AGENTS.md` §2.11。
7. **音质降级是预期行为**：`File.files` 只在 `SizeFlac`/`Size320 > 0` 时请求无损/HQ，并总是带上 128k 与 AAC 兜底；UI 会显示“无损不可用”“无高品质”或“无 MP3 音源”；选“最高音质”（`Best`，默认）时只显示实际拿到的音质，不提示降级。音质菜单里的文件大小来自歌曲信息的 `File.Size*`，不额外请求接口。无损通常要求会员。
8. **AAC 走 Media Foundation**：只在 Windows amd64/arm64 上可用（`aac_windows.go`），Windows N 版缺媒体功能包时报 `errNoMF`。文件经 `aac_stream_windows.go` 的 Go `IStream`（`syscall.NewCallback`）交给它，读到未下载的部分会阻塞等待；`Close` 必须先关下载再释放 reader，否则阻塞中的读会让解码线程卡死。Media Foundation 不裁掉 AAC 的编码器前置延迟（约 2112 个样本，≈48ms），可以忽略。不要为此引入 cgo 或纯 Go 的 AAC 解码库。
9. **`mewkiz/flac` 的 `Frame.SampleNumber()` 对定长分块流的最后一帧是错的**（用的是该帧自己的 BlockSize）；`flacDecoder.first` 用 STREAMINFO 的块大小计算，不要换回去。也不要用 `flac.Stream.Seek`：没有 seek table 时它会读完整个文件。
10. **`go test ./...` 会访问线上接口**：日常用 `go test -short ./...`；`TestPages`/`TestSearchSinger` 依赖网络与接口现状。
11. **exe 图标**：`go build` 只会链接 `cmd/qqmusic/mygo_windows_*.syso`；先跑 `go run ./cmd/genicon -syso`。`mygo build` 会自己处理图标与版本信息。
12. **`-H=windowsgui`**：不带它会在 GUI 程序旁弹出控制台窗口。
13. **不要手改生成物**：`build/`、`build/windows-amd64/resources.go`、`cmd/qqmusic/mygo_windows_*.syso`。
14. **Windows 中文脚本**：`.bat` 使用 UTF-8 + `chcp 65001`，编辑时保持编码。
15. **接口是非公开的**：字段可能随时变动；新增解析要允许空值，失败时走 `errText` 而不是 panic。
16. **小宇宙官方的 m4a 边下边播**：实测 1h16m 的单集 0.15s 开播（索引在文件头）。整集写到缓存目录的临时文件（60–120 MB），不占 Go 堆，播完删除。小宇宙的 `/v1/episode-transcript/get` 对本客户端返回的是与节目无关的占位文字稿，不要接成歌词。
17. **歌曲缓存命中不取播放地址**：键是 `mid-请求音质`（不是实际音质），实际音质存在 JSON 的 `note` 里，用来显示音质徽章。命中同样要求已登录（与 `SongURL` 一致）。不要缓存播放地址（约一天失效），只缓存文件。改 `buffer` 时保留两点：读到未下载处阻塞等待（AAC 的 `IStream` 依赖它）；下载完成时先写 JSON 再标记 `done`，否则刚下完就重播会查不到。
18. **MP3 跳转要 preroll**：MP3 帧会借用前面帧的数据，从目标帧直接解码，开头几帧是很响的杂音。`mp3Decoder.SetPosition` 先退 `preroll`（4×1152 个样本）再解码，丢掉多出的部分；不要删，`TestMP3Seek` 会拦住。
