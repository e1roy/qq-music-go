# QQ 音乐「你的私荐歌单」信息流分析指南

> 一次对 QQ 音乐客户端推荐流（音乐盲盒 / 碎片时间 / 你的私荐歌单）的逆向分析记录：这些内容从哪里来、怎么验证、涉及哪些文件、接口和字段长什么样，以及本仓库已经怎么用。
>
> 分析日期：2026-10-26。接口为非公开接口，随时可能变化；文中不记录账号 uin、`qm_keyst`、cookie 等凭证。

## 0. 结论速览

| 问题 | 结论 |
|---|---|
| 这些文案/歌单存在本地吗 | **不存在**。客户端 DLL/资源包里搜不到「私荐」「盲盒」「碎片时间」；本地也没有缓存推荐流 JSON |
| 数据从哪个接口来 | `POST https://u.y.qq.com/cgi-bin/musicu.fcg`，`module=music.recommend.RecommendFeed`，`method=get_recommend_feed` |
| 页面是什么 | 客户端内嵌的 H5 推荐页（`https://i2.y.qq.com/n3/wk_v20/entry/index/recommend`），页面只负责按 `v_shelf` 渲染 |
| 标题为什么每个号不一样 | 服务端下发 `title_template`（带 `{String}` 占位）+ `title_content`（昵称 / 歌名 / 推荐理由） |
| 「你的私荐歌单」具体是哪一条 | 同一接口下的歌单类 shelf（卡片 `type=500`）。标题由服务端按账号 / AB 实验下发，实测同类位置还会出现「你的歌单宝藏库」「歌单遨游指南」「为你精选的AI歌单」 |
| 需要登录吗 | 个性化内容需要。登录态（uin + `qm_keyst`）决定推荐结果；未登录拿到的是大众内容 |
| 本仓库怎么用 | `internal/qqmusic/api.go` 的 `RecommendFeed` 已实现（app comm + 两页），首页 `home.feed` 直接渲染 |

## 1. 分析对象与环境

- 客户端安装目录：`C:\Program Files (x86)\Tencent\QQMusic\QQMusic2261.20.52.10`（v22.61.20.52）
- 本仓库：`qqmusic` Go 客户端（纯 Go / 无 cgo），模块名 `qqmusic`
- 登录态：`%AppData%\QQMusicGo\credential.json`（本仓库自己保存的凭证，只用于只读探针）
- 请求纪律：全程只做了个位数信息流请求，没有遍历歌单 / 排行榜，没有批量取播放地址（见 `AGENTS.md` §2 第 11 条）

## 2. 分析流程（可复现）

### 2.1 定位客户端与版本

```powershell
Get-ChildItem 'C:\Program Files (x86)\Tencent\QQMusic' -Directory
# -> QQMusic2261.20.52.10
```

目录里没有 `*.js`、没有前端工程，只有 DLL / `.rdb` / `.pak` / `.exe`，所以第一步先判断：**这些文案到底是本地的，还是网络下发的？**

### 2.2 判断文案是不是本地的（关键一步）

在客户端整目录里按 UTF-8 和 UTF-16LE 两种编码搜中文关键词（Windows 程序常见 UTF-16）：

```python
import os

root = r'C:\Program Files (x86)\Tencent\QQMusic\QQMusic2261.20.52.10'
needles = ['歌单', '推荐', '私荐', '盲盒', '沉浸', '碎片时间', '猜你喜欢']
for dp, dn, fn in os.walk(root):
    for f in fn:
        p = os.path.join(dp, f)
        try:
            d = open(p, 'rb').read()
        except Exception:
            continue
        hits = {n: d.count(n.encode('utf-8')) + d.count(n.encode('utf-16-le'))
                for n in needles}
        hits = {k: v for k, v in hits.items() if v}
        if hits:
            print(os.path.basename(p), hits)
```

实测结果（只列非零）：

| 文件 | 命中 |
|---|---|
| `QQMusic.dll` | 歌单 122、推荐 19、沉浸 1、猜你喜欢 4 |
| `QQMusic_Protocol.dll` | 歌单 108、推荐 17、猜你喜欢 13 |
| `QQMusicResource.dll` | 歌单 16、推荐 1 |
| `Resource.rdb` | 歌单 15、推荐 1 |
| `recommendbase.json` / `ugc.json` | 沉浸 2 / 5（音效相关） |
| **`私荐`、`盲盒`、`碎片时间`** | **全目录 0 命中** |

结论：客户端二进制里确实存在中文（所以 0 命中不是编码问题），但这三个词不存在 → 它们只能来自服务端响应。

### 2.3 从资源 DLL 找页面入口

`QQMusicResource.dll` 里有一张 URL 表（JSON 片段）。用正则把所有 `http(s)://` 拉出来：

```python
import re

d = open(r'...\QQMusic2261.20.52.10\QQMusicResource.dll', 'rb').read().decode('latin-1')
for u in sorted(set(re.findall(r'https?://[^\s"\'<>\\]{5,300}', d))):
    print(u)
```

关键命中：

```json
"recommend_v20_url": { "url": "https://i2.y.qq.com/n3/wk_v20/entry/index/recommend" }
```

同时还命中旧入口（`wk_v17`、`wk_v15`、`i.y.qq.com/pcmusic/...`），说明推荐页是「客户端打开的一个 H5」。

### 2.4 抓 H5 入口，找 webpack chunk

```powershell
$r = Invoke-WebRequest 'https://i2.y.qq.com/n3/wk_v20/entry/index/recommend' -UseBasicParsing
$r.Content.Length   # 23188：几乎是个空壳，页面由 JS 渲染
$r.Content | Out-File -Encoding utf8 page.html
Select-String -Path page.html -Pattern 'src="[^"]+"'
```

拿到的资源（节选）：

```text
//y.qq.com/n3/wk_v20/entry/runtime.2bf7fe8a00d0306ebbaf.bundle.js
//y.qq.com/n3/wk_v20/entry/index.chunk.6f097f4bb.js
//y.qq.com/n3/wk_v20/entry/react-vendor.chunk.2c08016f9.js
//y.qq.com/n3/wk_v20/entry/vendors.chunk.382204458.js
//y.qq.com/lib/commercial/h5/react-18.2.0.min.js
```

`runtime` 里是 chunk 映射，`d.p = "//y.qq.com/n3/wk_v20/entry//"`，chunk 32 名为 `recommend`：

```text
recommend.chunk.bdf2a22fe.js
```

下载业务 chunk 后，用正则列出它用了哪些接口：

```python
import re

d = open('recommend.chunk.bdf2a22fe.js', encoding='utf-8', errors='ignore').read()
print(sorted(set(re.findall(r'module:\s*"([^"]+)"', d))))
print(sorted(set(re.findall(r'method:\s*"([^"]+)"', d))))
```

结果：

```text
module: music.trackInfo.UniformRuleCtrl / music.recommend.TrackRelationServer /
        music.pf_song_detail_svr / music.recommend.RecommendFeed /
        music.srfDissInfo.aiDissInfo / music.vipsalecgi.PriceOfferCgi
method: CgiGetTrackInfo / GetSimilarSongs / get_song_detail /
        get_recommend_feed / uniform_get_Dissinfo / GetBriefPriceOfferGroups
```

整页信息流只有一个接口：

```js
Object(i.p)().request({
  module: "music.recommend.RecommendFeed",
  method: "get_recommend_feed",
  param: { direction: 1 === e ? 0 : 1, page: e, v_cache: [], v_uniq: [], s_num: r }
})
```

渲染侧按 shelf id 分发（页面组件里的 `v_shelf.map`）：

```text
t.id === g.Banner           -> Nt   横幅
t.id === g.PersonalRadio    -> nt   个性电台（大卡 + 播放键）
t.id === g.DoubleRowSongMenu-> ft   两行歌单
t.id === g.RecommendRadio   -> mt   电台
t.id === g.RecommendSongs   -> kt   歌曲块
t.id === g.SongMenu         -> ft   一行歌单
```

`ft` 组件把 `v_niche -> v_card` 铺成卡片行，`kt` 铺成歌曲卡片 —— 这解释了为什么同一份接口既能画「歌单行」，也能画「歌曲九宫格」。

> 顺带一个容易误会的点：页面里的字符串 `猜你喜欢-沉浸刷歌` 是沉浸播放器的兜底歌单名（`c.songname || "猜你喜欢-沉浸刷歌"`），不是接口参数，也不是页面路由。

### 2.5 实测接口：三种 comm 对比

用本仓库的 `internal/qqmusic` 写一次性探针：读 `credential.json` → `SetCredential` → 调 `get_recommend_feed`。同一个接口换三套公共参数（comm），返回风格完全不同：

| comm | 参数 | 返回风格 |
|---|---|---|
| Web | `ct=24, cv=4747474, platform=yqq.json` | PC 推荐页：`为你打造`、`最近常听` |
| Desktop | `ct=19, cv=2201` | 与 Web 类似；沉浸模式只剩单歌单卡 |
| App | `ct=11, cv=13020508, tmeAppID=qqmusic` | 移动端信息流：`碎片时间，积累竞争力`、歌曲块、节目卡 |

探针核心：

```go
c := qqmusic.New()
c.SetCredential(cr)

reqs := map[string]qqmusic.Req{"req": {
    "music.recommend.RecommendFeed", "get_recommend_feed",
    map[string]any{"direction": 0, "page": 1, "v_cache": []any{}, "v_uniq": []any{}, "s_num": 0},
}}
data, errs, err := c.CallMany(ctx, qqmusic.Web,
    map[string]any{"ct": 11, "cv": 13020508, "tmeAppID": "qqmusic"}, reqs)
```

`CallMany` 的第三个参数会并进 `comm`，所以不用改 `Platform` 枚举也能模拟移动端。

## 3. 接口规格

### 3.1 请求

```text
POST https://u.y.qq.com/cgi-bin/musicu.fcg
Content-Type: application/json
Referer: https://y.qq.com/
Cookie: uin=...; qm_keyst=...        （登录后才有个性化）
```

```json
{
  "comm": {
    "ct": 11,
    "cv": 13020508,
    "tmeAppID": "qqmusic",
    "uin": 0,
    "qq": "...",
    "authst": "...",
    "g_tk": 0,
    "g_tk_new_20200303": 0
  },
  "req": {
    "module": "music.recommend.RecommendFeed",
    "method": "get_recommend_feed",
    "param": {
      "direction": 0,
      "page": 1,
      "v_cache": [],
      "v_uniq": [],
      "s_num": 0
    }
  }
}
```

参数说明：

| 参数 | 含义 | 实测 |
|---|---|---|
| `direction` | 0 首屏 / ≥1 沉浸续刷 | 0 与 1 返回不同模块集；2 与 1 基本一致 |
| `page` | 页码，从 1 开始 | 1~4 有效；`load_mark=0` 时还能继续翻 |
| `v_cache` / `v_uniq` | 客户端去重缓存 | 传空数组即可 |
| `s_num` | 客户端已加载的 shelf 数（H5 里传 `v_shelf.length`） | 传 0 也能拿到完整首屏 |

### 3.2 返回结构

```text
{
  load_mark: 0,
  retcode: 0,
  v_shelf: [{
    id, title_template, title_content,
    v_niche: [{
      v_card: [{ type, id, title, subtitle, cover, cnt, jumptype, scheme, tags, ... }]
    }]
  }]
}
```

- `title_template` + `title_content`：标题模板与填充值，模板里的 `{String}` 会被替换。
- `v_card[].type`：卡片形态，决定这一行怎么渲染。
- `v_card[].id`：歌曲 / 歌单是数字 ID，其它类型可能是链接字符串，建议先按 `json.RawMessage` 收，再按 type 解析（本仓库 `RecommendFeed` 就是这么做的）。
- `v_card[].jumptype` / `scheme`：客户端跳转用，本仓库不依赖。

### 3.3 卡片 type（实测）

| type | 实测内容 | 例子 |
|---|---|---|
| 200 | 歌曲 | `听「{String}」也会喜欢💗` 下的 48 首歌 |
| 400 | 节目 / 有声书 | `碎片时间，积累竞争力` |
| 500 | 歌单 | `你的歌单宝藏库`、`歌单遨游指南`、`每日30首`、`百万收藏` |
| 700 | 功能入口 | `猜你喜欢` |
| 800 | 排行 / 功能入口 | `一周听歌排行`、`9月听歌排行` |
| 900 | 功能入口 | `雷达模式`、`小宇宙`（subtype 994，`id` 是播客单集的歌曲 id，见第 10 节）、`星光农场` |
| 1000 | 榜单 | `排行榜` |
| 1100 | 直播 | `推荐今日专属直播` |
| -1 | 占位 / 更多 | `更多为你推荐` |

> 200 / 500 是列表真正需要的两类；其余类型本仓库目前直接丢弃。

### 3.4 标题模板实测汇总

| `title_template` | `title_content` 填充 | shelf |
|---|---|---|
| `Hi {String}  今日为你推荐` | 账号昵称 | 301 |
| `听「{String}」也会喜欢💗` | 歌名 | 207 |
| `{String}` | `从你的红心歌曲开始探索` / `打破音乐舒适圈，惊喜不断` / `VIP专属歌曲推荐` | 207 |
| `听「{String}」的也在听` | 歌手名 | 207 |
| `今晚，吹吹「{String}」的风` / `深夜，「{String}」旋律陪伴` | 歌手名 | 207 |
| `碎片时间，积累竞争力` | — | 272 |
| `今日专属精彩节目` | — | 274 |
| `为你精选的AI歌单` | — | 276 |
| `歌单遨游指南` | — | 205 |
| `你的歌单宝藏库` | — | 271 |
| `排行榜` | — | 114 |
| `「数字专辑」你的专属音乐故事` | — | 295 |
| `更多为你推荐` | — | 279 |
| `为你打造` / `最近常听` | —（Web / Desktop comm） | 201 / 202 |

## 4. 实测结果（app comm）

### 4.1 `direction=0, page=1`

| shelf | 标题 | 卡片 |
|---|---|---|
| 301 | `Hi {String}  今日为你推荐`（填充昵称） | 12 张：猜你喜欢(700)、每日30首(500)、雷达模式 / 小宇宙 / 星光农场(900) 等 |
| 207 | `听「{String}」也会喜欢💗`（填充歌名） | 16 组 × 3 首，共 48 首（200） |
| 272 | `碎片时间，积累竞争力` | 12 个节目 / 有声书（400） |

### 4.2 `direction=0, page=2`

| shelf | 标题 | 卡片 |
|---|---|---|
| 301 | 同上 | 12 |
| 207 | `{String}` / `从你的红心歌曲开始探索` | 48 首（200） |
| 274 | `今日专属精彩节目` | 24 个节目（400） |

### 4.3 `direction=1, page=1`（沉浸续刷）

| shelf | 标题 | 卡片 |
|---|---|---|
| 207 | `{String}` / `打破音乐舒适圈，惊喜不断` | 48 首（200） |
| 273 | `推荐今日专属直播` | 11（1100 + 1 张 -1） |
| 207 | `听「{String}」的也在听` | 44 首（200） |
| 276 | `为你精选的AI歌单` | 9 个歌单（500） |
| 207 | `{String}` / `VIP专属歌曲推荐` | 48 首（200） |
| 114 | `排行榜` | 6（1000） |
| 205 | `歌单遨游指南` | 12 个歌单（500） |
| 207 | `今晚，吹吹「{String}」的风` | 46 首（200） |
| 271 | `你的歌单宝藏库` | 12 个歌单（500） |

`direction=1, page=2` 还会多出：`「数字专辑」你的专属音乐故事`（295，3 个 400）等。

### 4.4 Web / Desktop comm

| comm | 结果 |
|---|---|
| Web `direction=0` | 201 `为你打造`（每日30首 500 + 听歌排行 800）、202 `最近常听`（5 个歌单）、204 `更多为你推荐`（-1）、若干 203 单歌单 |
| Desktop `direction=0` | 与 Web 相同 |
| Desktop `direction=1 / 2` | 只剩 6 个 203 单歌单卡，无标题 |

## 5. 与截图的对照

| 截图 | 对应数据 |
|---|---|
| `「<昵称>」，这是你的音乐盲盒` + 3×3 歌曲 | 歌曲类 shelf（type=200）+ 服务端标题。标题模板带 `{String}`、用昵称填充（301 的 `Hi {String}` 已证明这套机制） |
| `碎片时间，累积竞争力` | shelf 272，原样返回 `碎片时间，积累竞争力`（服务端文案会变，`积累 / 累积` 是版本差异） |
| `你的私荐歌单` | 歌单类 shelf（type=500）。同一位置实测到的是 `你的歌单宝藏库`、`歌单遨游指南`、`为你精选的AI歌单`；标题按账号 / AB 变，没有固定的「私荐歌单」接口 |
| 顶部 `猜你喜欢-沉浸刷歌` | 沉浸播放器的兜底歌单名（H5 里的字符串），不是接口字段 |

> 说明：截图里的模块组合与 **app comm** 的返回一一对应（尤其 `碎片时间` shelf 272），而 Web / Desktop comm 返回的是另一套（为你打造 / 最近常听）。所以这个页面用的是移动端那套信息流；PC 上具体是哪一层容器渲染的（H5 / Hippy / 内嵌页），本次没有继续深挖。

## 6. 涉及的文件

### 6.1 客户端（只读，未修改）

| 路径 | 作用 | 与本分析的关系 |
|---|---|---|
| `...\QQMusic2261.20.52.10\QQMusicResource.dll` | 资源与 URL 表 | 找到 `recommend_v20_url` |
| `...\QQMusic_Protocol.dll` | 原生协议模块名 | 确认 `music.recommend.*`、`music.radioProxy.*`、`music.smartboxCgi.SmartBoxCgi.GetSmartBoxResultForPc` |
| `...\QQMusic.dll` | 主程序 | 含 `RadioPageData::GetPersonalRadioData` 等原生电台逻辑（与截图页无关） |
| `...\Resource.rdb` | 资源库（XML / 图片） | 无目标文案 |
| `...\qmbrowser\*.pak` | CEF 资源 | 无目标文案 |
| `%AppData%\Tencent\QQMusic\` | `qmlist64.db`、`mmkv`、`log`、`gs_webview_data` | 扫不到推荐流 JSON；内嵌浏览器缓存里只有登录 / 小游戏相关页面 |
| `%LocalAppData%\qqmusic\WebView2\` | 登录用 WebView2 | 缓存里只有 ptlogin / captcha |
| `%AppData%\QQMusicGo\credential.json` | 本仓库保存的登录态 | 只用于只读探针，未写入本文 |

### 6.2 本仓库

| 文件 | 与本文的关系 |
|---|---|
| `internal/qqmusic/client.go` | `comm` / `CallMany` / `extraComm`：三套 comm 的实现（Web 24、Desktop 19、App 通过 extra 覆盖） |
| `internal/qqmusic/api.go` | `RecommendFeed`（947-1032，app comm、两页、`CgiGetTrackInfo` 补歌）、`appComm`（939）、`DailySongs`（903，同接口找每日30首） |
| `internal/qqmusic/types.go` | `FeedShelf`（117-124） |
| `internal/app/app.go` | `home.feed`（198）、`loadHome` 登录后拉 feed（555）、退出清空（1077） |
| `internal/app/view_pages.go` | 首页渲染 feed：歌曲 9 个 + 本地「换一批」轮转，歌单网格（133-155） |
| `cmd/probe/main.go` | 线上冒烟：`RecommendFeed` 打印 shelf 标题 / 数量（160-164） |
| `cmd/zz_feeddump/main.go` | 本地一次性 dump 工具（未纳入 git） |
| `internal/app/zz_feed_test.go` | UI 侧 feed 走查（未纳入 git） |

### 6.3 当前实现的关键逻辑

```go
// internal/qqmusic/api.go:947
func (c *Client) RecommendFeed(ctx context.Context) ([]FeedShelf, error)
```

1. 用 `appComm`（`ct=11, cv=13020508`）调两页：第 1 页 `direction=0`，第 2 页 `direction=1`（注释说「歌单在第 2 页」）。
2. 把 `title_template` 里的 `{String}` 替换成 `title_content`；模板为空时直接用 `title_content`。
3. 只保留「第一张卡是 200 或 500」的 shelf；其它（入口、节目、直播）整块丢弃。
4. 歌曲 shelf 只存 ID，最后一次性用 `music.trackInfo.UniformRuleCtrl / CgiGetTrackInfo` 批量换成完整 `Song`。
5. 删掉既没歌也没歌单的 shelf。

与本文实测的对应：第 1 页留下歌曲 shelf（207），第 2 页留下 AI 歌单（276）和歌单遨游指南（205）；`碎片时间`（400）、`排行榜`（1000）、直播（1100）会被过滤。

## 7. 复现脚本

### 7.1 一次性 dump（Go）

```go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"qqmusic/internal/qqmusic"
)

func main() {
	b, _ := os.ReadFile(os.Args[1]) // credential.json
	var cr qqmusic.Credential
	json.Unmarshal(b, &cr)
	c := qqmusic.New()
	c.SetCredential(cr)

	for _, q := range []struct{ dir, page int }{{0, 1}, {0, 2}, {1, 1}} {
		var out struct {
			Shelves []struct {
				ID      json.Number `json:"id"`
				Title   string      `json:"title_template"`
				Content string      `json:"title_content"`
				Niches  []struct {
					Cards []json.RawMessage `json:"v_card"`
				} `json:"v_niche"`
			} `json:"v_shelf"`
		}
		reqs := map[string]qqmusic.Req{"req": {"music.recommend.RecommendFeed", "get_recommend_feed", map[string]any{
			"direction": q.dir, "page": q.page, "v_cache": []any{}, "v_uniq": []any{}, "s_num": 0,
		}}}
		data, errs, err := c.CallMany(context.Background(), qqmusic.Web,
			map[string]any{"ct": 11, "cv": 13020508, "tmeAppID": "qqmusic"}, reqs)
		fmt.Println("==", q, err, errs["req"])
		if err != nil || errs["req"] != nil {
			continue
		}
		json.Unmarshal(data["req"], &out)
		for _, s := range out.Shelves {
			n := 0
			for _, it := range s.Niches {
				n += len(it.Cards)
			}
			fmt.Printf("  shelf %s %q + %q cards=%d\n", s.ID, s.Title, s.Content, n)
		}
	}
}
```

> 等价的现成工具：`go run ./cmd/zz_feeddump "$env:AppData\QQMusicGo\credential.json"`（本地文件，未纳入 git）。

### 7.2 跑仓库自带的冒烟

```powershell
go run ./cmd/probe -login qq -cred cred.json   # 会跑 RecommendFeed 并打印 shelf
```

## 8. 风险与限制

1. **非公开接口**：`musicu.fcg` 随时可能改字段；解析要允许缺字段（本仓库用 `json.RawMessage` + `ParseInt` 容错）。
2. **限流**：`music.vkey.GetVkey` 短时间大量请求会触发 `104009`，之后连正常歌曲都取不到地址。信息流请求相对轻，但也不要循环翻页；本分析全程个位数请求。
3. **AB / 个性化**：同一个接口不同账号、不同时间返回的模块与标题不同；「你的私荐歌单」不一定每次都出现，不要把它当固定字段。
4. **文案会变**：`积累 / 累积` 这类差异来自服务端，客户端不做本地文案。
5. **登录态**：不登录只能拿到大众内容；`RecommendFeed` 需要 `SetCredential`。
6. **隐私**：探针会打印昵称 / 歌单；文档与提交物里不要放 uin、`qm_keyst`、cookie。`internal/qqmusic/types.go:118` 的注释用了实测账号昵称做例子，开源前建议换成占位符。
7. **工作区**：本次分析的临时脚本与 JSON 已删除；`cmd/zz_feeddump/`、`internal/app/zz_feed_test.go`、`tmp_feed.png` 是本地验证产物（未纳入 git），按需保留或删除。

## 9. 一页速查

```text
页面：https://i2.y.qq.com/n3/wk_v20/entry/index/recommend
接口：POST https://u.y.qq.com/cgi-bin/musicu.fcg
      music.recommend.RecommendFeed / get_recommend_feed
参数：{direction, page, v_cache:[], v_uniq:[], s_num}
comm：{ct:11, cv:13020508, tmeAppID:"qqmusic"}   // 移动端信息流
返回：v_shelf[].v_niche[].v_card[]
卡片：200 歌曲 / 400 节目 / 500 歌单 / 700 入口 / 800・900 入口 / 1000 榜单 / 1100 直播
标题：title_template 的 {String} ← title_content
代码：internal/qqmusic/api.go:947 RecommendFeed
```

## 10. 附：内置「小宇宙」播客模块（2026-10-26 补）

### 10.1 入口卡片

同一份 `direction=0, page=1` 的 app comm 信息流里，shelf 301 有一张：

| 字段 | 值 |
|---|---|
| `type` | `900` |
| `subtype` | `994` |
| `jumptype` | `10046` |
| `id` | `736382210` |
| `title` | `小宇宙` |
| `cover` | `https://y.gtimg.cn/music/photo_new/T002R500x500M000000omnsg344lUQ_5.jpg` |
| `scheme` | `qqmusic://qq.com/ui/openRN?p={"key":"TodayFmAudio","params":{"id":"736382210","iType":"0","title":"小宇宙"}}` |

同一 shelf 的 `extra_info.layout_seq` 里带 `podcast@7@0`，说明「播客」是信息流布局中的一个模块。
`id` 不是歌单 id（`PlaylistDetail(736382210)` 返回的是用户歌单「我最爱听」），而是**播客单集的歌曲 id**。

### 10.2 数据模型：节目=专辑，单集=歌曲，主播=歌手

`CgiGetTrackInfo` 查 `736382210`：

```text
name    E255 从「大结果」到「大红果」：你还相信越努力越幸运吗？
mid     003cq5oI3dfvx1
album   知行小酒馆 (albummid 000omnsg344lUQ)
interval 4167 秒
file    Size128=66,685,223  Size320=0  SizeFlac=0  SizeAAC=0
pay     PayPlay=0（非会员可播）
```

`AlbumDetail(000omnsg344lUQ)`：

```text
album  知行小酒馆  type=人声音频  singer=有知有行  date=2021-01-29
songs  253 集（按发布时间倒序）
```

`SingerAlbums(主播 mid)` 返回该主播名下的 2 个节目：`知行小酒馆`、`投资ABC`。
搜索 `知行小酒馆` 的直达卡片是 `label="播客"`、`desc="有知有行 · 253集"`。

### 10.3 实测播放地址

`SongURL(song, Best)` 逐级降级后拿到 128k MP3：

```text
https://isure.stream.qqmusic.qq.com/M500003cq5oI3dfvx1.mp3?guid=...&vkey=...
quality=标准 128k
```

说明播客单集在曲库里只有 128k MP3（至少这一集），现有 `internal/player` 的 MP3 流式解码可直接播，不需要 AAC / Media Foundation。

### 10.4 本地客户端分析结论

对 `QQMusic2261.20.52.10` 全目录 314 个文件按 UTF-8 与 UTF-16LE 搜「小宇宙」：**0 命中**；
`podcast` 只命中 `avformat-58.dll` / `avformat-lav-58.dll` / `qbcore.dll` 里的 MP4 元数据字段名。
`QQMusic_Protocol.dll` 里与电台相关的只有：

```text
music.radioProxy.MbTrackRadioSvr.get_radio_track
music.recommend.SingerRadioServer
```

`%AppData%\Tencent\QQMusic` 与 `%LocalAppData%\qqmusic` 里也搜不到 `TodayFmAudio`。
结论：客户端只负责渲染服务端下发的卡片，点开交给按需下载的 RN 页；**播客内容本身是 QQ 音乐曲库数据**，所以能直接用现有接口拿到。

### 10.5 接入可行性

| 能力 | 现状 |
|---|---|
| 从首页进播客 | `RecommendFeed` 里筛 `type=900 && subtype=994`，`id` 当歌曲 id |
| 搜索节目 | 现有 `Search` 的直达卡片已经返回 `label="播客"` + albummid |
| 节目详情 / 全部单集 | 现有 `AlbumDetail`（实测 253 集一次返回） |
| 主播的其他节目 | 现有 `SingerAlbums` |
| 播放 | 现有 `SongURL`（128k MP3） |
| 小宇宙自身的推荐排序 | 未找到对应接口；只能依赖信息流卡片与搜索 |

需要注意的两点：

1. 单集时长动辄 1 小时以上，`buffer` 会把整集（约 64 MB）留在内存里；播放完才由 `trimmer` 归还。
2. 播客专辑的 `type` 是 `人声音频`、直达卡片 `label` 是 `播客`，UI 上要和普通专辑区分，否则「专辑」文案会显得奇怪。

### 10.6 小宇宙的 pid / eid 能不能直接查 QQ 音乐

**不能按 id 查**。实测（以「知行小酒馆」为例）：

| 对象 | 小宇宙 | QQ 音乐 |
|---|---|---|
| 节目 | pid `6013f9f58e2f7ee375cf4216` | albummid `000omnsg344lUQ` |
| 单集 | eid `6adb068cc96c8942ec110ee1` | songmid `003cq5oI3dfvx1` |
| 音频 | `media.xyzcdn.net/.../lmrfBKmfdmUn7-CjfvaeogEO6FQI.m4a` | `M500003cq5oI3dfvx1.mp3` |

- 用 pid / eid 当关键词搜索：没有直达卡片，返回的是「圆周率」「押韵歌」这类模糊结果。
- `CgiGetTrackInfo` 与 `GetAlbumDetail` 的原始 JSON 里没有 pid / eid，也没有 xyz / podcast 字段。
- 音频文件名（`lmrf...` 与 `003cq5oI3dfvx1`）无对应关系，两个 CDN 不同源。

能对齐的只有**名称 + 时长**。实测最近 8 集，标题逐字相同、时长逐秒相同：

| 单集 | 小宇宙 eid | 时长 | QQ 音乐 songid | interval |
|---|---|---|---|---|
| E255 | `6adb068cc96c8942ec110ee1` | 4167 | `736382210` | 4167 |
| E254 | `6ad1c92a0e4e410a05bdc907` | 7466 | `734396142` | 7466 |
| E253 | `6ac89152195d838e2aef2e5f` | 4734 | `732454516` | 4734 |
| E252 | `6aace1189d326477816b0a46` | 3882 | `727014583` | 3882 |

落地时只能：

```text
pid → 节目名 → Search(节目名) → 直达卡片 albummid
    → AlbumDetail → 按 E编号 + 标题 + duration(±2s) 匹配 → songid → SongURL
```

如果要做「小宇宙导入到 QQ 音乐播放」，就需要维护这样一张按节目缓存的匹配表；不要指望接口层能按 id 解析。
