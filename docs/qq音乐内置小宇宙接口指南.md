# QQ 音乐内置「小宇宙」接口指南

> 目标：给「左侧新增小宇宙模块」做接口准备，把 QQ 音乐曲库里那批播客（小宇宙内容）的**入口、数据结构、播放链路、能拿到与拿不到的东西**记录清楚。
>
> 分析日期：2026-10-26。全部结论来自本仓库 `internal/qqmusic` 的实测（游客 + 已登录各一轮），除特别标注外都已在线上跑通。
>
> 请求纪律：全程个位数到十几次请求，串行；没有遍历榜单/歌单，没有批量取播放地址。文中不记录 uin、`qm_keyst`、cookie、vkey。

## 0. 结论速览

| 问题 | 结论 |
|---|---|
| 小宇宙在 QQ 音乐里是什么 | **长音频内容**：节目 = 专辑（`albumType="人声音频"`、`type=10025`），单集 = 歌曲（`type=0`），主播 = 歌手（个人主播与节目账号都可能是歌手） |
| 有没有独立的小宇宙接口 | 没有。客户端首页那张「小宇宙」卡片是 RN 页面 `TodayFmAudio`，数据仍然走曲库 |
| 浏览入口（推荐） | `music.area.AreaHome / getAreaHomePage`，`encArea=004Z03m934o6En`（**播客专区**，area id 206） |
| 这个入口要登录吗 | **不要**。游客可直接拿全部楼层 |
| 推荐/换一批 | `music.longRadio.recommend / getRadioList`，每次 6 个，带主播与简介，**每次调用结果不同** |
| 节目详情 | `music.musichallAlbum.AlbumInfoServer / GetAlbumDetail`，`albumMid` 或 `albumID` 都行 |
| 单集列表 | `music.musichallAlbum.AlbumSongList / GetAlbumSongList`，`begin/num` 分页，`totalNum` 是总集数 |
| 播放 | `SongURL`，**需要登录**；实测小宇宙节目只有 128k MP3，QQ 自制播客可能带 320k |
| 字幕 | 单集有 **AI 智能字幕**，就是普通 LRC，`[by:AI智能字幕]`，可直接进现有歌词页 |
| 单集简介 / shownotes | **没有**。`GetPlayLyricInfo` 只有字幕；`get_song_detail` 的 `info` 只有演唱者/语种/公司/歌词 |
| 订阅 / 我的播客 | 未找到对应接口 |
| 数据新鲜度 | 专区楼层 `modifyTime=2021-11-25`，偏旧；`getRadioList` 明显更新，两者要混用 |
| 内存 | 单集 60–115 分钟，128k 约 56–111 MB，`player.buffer` 会整集驻留 |

## 1. 数据模型

| 小宇宙概念 | QQ 音乐形态 | 关键字段 | 怎么认出来 |
|---|---|---|---|
| 节目 / 播客 | 专辑 | `albumID` / `albumMid` | `basicInfo.albumType == "人声音频"`，`basicInfo.type == 10025` |
| 单集 | 歌曲 | `songID` / `songMid` / `interval`（秒） | `type == 0`，`album.mid` 是节目 |
| 主播 | 歌手 | `singerMID` / `singerID` | `SingerAlbums` 里全是 `albumType="人声音频"`；`SingerDetail.basic_info.type` 不固定（节目账号 3、个人主播 0/1） |
| 节目分类 | 专区楼层 | `shelf.id` / `moduleType` / `card.type` | 见第 3 节 |
| 单集字幕 | LRC 歌词 | `GetPlayLyricInfo` | `[by:AI智能字幕]` |

补充：

- 专辑 `miscellany.area`（专区卡片里）区分长音频子类：`"0"` = 播客，`"1"` = 有声书。
- 有声书专辑的 `miscellany.fpaymid` 是付费商品 mid；播客通常为空。
- 单集没有独立 id 空间，就是普通 `songid`；`CgiGetTrackInfo`、`SongURL`、评论、我喜欢都能直接用。

## 2. 接口约定

### 2.1 网关与信封

```text
POST https://u.y.qq.com/cgi-bin/musicu.fcg
Content-Type: application/json
Referer: https://i.y.qq.com/
```

专区接口**不是**常见的 `{comm, req_0}` 形状，而是把业务节点直接放在顶层，节点名任意（H5 用 `PoolApiServer`）：

```json
{
  "PoolApiServer": {
    "module": "music.area.AreaHome",
    "method": "getAreaHomePage",
    "param": { "cmd": 0, "encArea": "004Z03m934o6En" }
  }
}
```

响应会原样带回这个节点名：

```json
{
  "code": 0,
  "ts": 1793030547492,
  "start_ts": 1793030547376,
  "traceid": "2e00a14a539deea3",
  "PoolApiServer": { "code": 0, "data": { "id": 206, "title": "播客专区", "v_shelf": [] } }
}
```

- `comm` 可带可不带，H5 不带；本仓库 `CallMany` 会带，实测无影响。
- `PoolApiServer.code != 0` 时不要用 `data`。
- `cmd` 只认 `0`；`1` / `2` 返回 `code 7000` + 空壳。

### 2.2 游客与登录

| 能力 | 游客 | 登录 |
|---|---|---|
| 专区楼层 | ✅ | ✅ |
| `getRadioList` | ✅ | ✅ |
| 节目详情 / 单集列表 | ✅ | ✅ |
| 搜索 | ✅ | ✅ |
| 字幕 / 评论 | ✅ | ✅ |
| `SongURL` 播放地址 | ❌ `ErrNotLoggedIn` | ✅（VIP 单集还要会员） |

也就是说：**小宇宙页面可以完全在未登录时浏览，点播放才要求登录**，和现在「推荐/搜索」页的行为一致。

### 2.3 字段容错

沿用仓库现有约定：数字可能是字符串（`flexInt`），文案可能带 HTML（`cleanIntro`），封面可能为空，列表可能为空。下面所有「字段表」都只列实测有值或结构稳定的。

## 3. 播客专区（浏览入口）

### 3.1 请求

```go
req := qqmusic.Req{
    Module: "music.area.AreaHome",
    Method: "getAreaHomePage",
    Param:  map[string]any{"cmd": 0, "encArea": "004Z03m934o6En"},
}
```

`encArea` 是专区短码，不是专辑 mid。已知的：

| 专区 | encArea | area id | 说明 |
|---|---|---|---|
| 播客专区 | `004Z03m934o6En` | 206 | 本次主入口 |
| 免费有声书 | `003jmP4E20VguJ` | 226 | 有声书，卡片结构与播客相同 |
| VIP 专属 | `0028kDzu07cFkV` | — | 搜索结果里的页面，未单独实测 |

`encArea` 没有列表接口可查，只能从 H5 链接或客户端卡片拿到；**建议在代码里写成常量**，并在失败时降级到 `getRadioList`。

### 3.2 返回结构

```text
data
├── id, title, desc, cover
├── color { background, font, highlight, header, headerbtn, foreColor }
├── share { title, desc, image }
├── button { type, title, playId, jumpUrl, func, funcParams, v_song[], schemaType, schemaNewParams, FheaderBtnScheme }
├── modifyTime
├── btnTjreport
├── miscellany { Farea_headImgType, Farea_headType }
└── v_shelf[]                    ← 楼层
    └── v_niche[]                ← 一个楼层通常只有一个 niche
        └── v_card[]             ← 真正的卡片
```

| 字段 | 类型 | 说明 |
|---|---|---|
| `id` | int | 专区 id，播客 = 206 |
| `title` | string | 「播客专区」 |
| `desc` | string | 实测为空 |
| `cover` | string | 相对路径，前缀 `https://y.qq.com/music/common` |
| `share.desc` | string | 「听见闪闪发光的灵魂」，适合做副标题 |
| `button.title` | string | 「随便听听，遇见惊喜」 |
| `button.playId` | int | 699，播放按钮的参数（像电台 id，未验证），**不是**歌曲/专辑 id |
| `modifyTime` | int | Unix 秒；播客专区 1637825998（2021-11-25） |
| `v_shelf` | array | 楼层，见 3.3 |

封面拼接：

```go
cover := "https://y.qq.com/music/common" + data.Cover // "/upload/t_area/4090933.png"
```

### 3.3 shelf

| 字段 | 说明 |
|---|---|
| `id` | 楼层 id（如 5494） |
| `title_template` / `title_content` | 标题；模板里的 `{String}` 用 content 填（与信息流一致） |
| `style` | 布局样式，0/1/2，只影响排版 |
| `miscellany.moduleType` | 楼层类型，见 3.5 |
| `more` | 「更多」入口：`{jumptype, id, scheme, title, ...}`；实测播客楼层 `id` 为空、`scheme` 为空，**不可用** |
| `v_niche[]` | 子块；每个 niche 有自己的 `v_card[]` |
| `v_niche[].style` | 1 = 横排卡片，2 = 网格，10 = 分类网格 |

取卡片时要遍历所有 niche，不要只看第一个。

### 3.4 card

所有卡片共享的字段：

| 字段 | 类型 | 说明 |
|---|---|---|
| `type` | int | 卡片类型，见下表 |
| `subtype` | int | 实测 0 |
| `id` | string | 类型相关：单集 = songid，节目 = albumID，主播/运营位 = `card_index_<shelf>_<i>` |
| `subid` | string | **节目卡（400）= albumMid**，其它类型为空 |
| `title` | string | 节目名 / 单集名 / 主播名 |
| `subtitle` | string | 400 卡 = 主播名（可多人，`/` 分隔） |
| `cover` | string | 完整 URL；400 卡是 `T002` 专辑封面 |
| `cnt` | int | 400 卡为 0；电台卡是热度值（口径未验证） |
| `time` | int | 400 卡 = 发布时间戳（秒） |
| `v_user[]` | array | 400 卡 = 主播列表 `{uin, encrypt_uin, nick, avatar, ...}` |
| `miscellany` | object | 按类型不同，见下表 |
| `tjreport` / `trace` / `abt` | string | 埋点，不用 |

卡片类型（播客专区实测计数）：

| type | 含义 | 关键取值 | 数量 |
|---|---|---|---|
| `2000` | 运营位 / Banner | `miscellany.schemaType` + `schemaNewParams` 决定跳转；`cover` 是运营图 | 9 |
| `200` | 单集 | `id` = songid，`title` = 单集标题，`miscellany.albumname/singername` | 7 |
| `2100` | 主播 / 明星播客 | `title` = 人名，`miscellany.schemaType="2"`，`schemaNewParams` = **albumID** | 34（33 个不同） |
| `400` | 节目（专辑） | `id` = albumID，**`subid` = albumMid**，`title` = 节目名，`subtitle` = 主播 | 88（84 个不同） |
| `2500` | 话题 / 讨论占位 | `id` = 20，所在楼层 `moduleType=1019` | 1 |

三种主要卡片的实测样例（截断）：

```json
// 400：节目
{
  "type": 400,
  "id": "20102232",
  "subid": "001N8TFz49WZol",
  "title": "无乐不谈",
  "subtitle": "王东/谭维维/梁博/李健/郁可唯/苏运莹",
  "cover": "https://y.gtimg.cn/music/photo_new/T002R300x300M000001N8TFz49WZol_2.jpg",
  "time": 1622649600,
  "v_user": [{ "nick": "王东", "encrypt_uin": "001gtDqC0j3hox" }],
  "miscellany": {
    "area": "0",
    "num": "38",
    "release_time": "2021-06-03",
    "fpaymid": "",
    "singerCountry": "3"
  }
}
```

```json
// 2100：主播卡（schemaNewParams 是节目 albumID）
{
  "type": 2100,
  "id": "card_index_5494_0",
  "title": "王晰",
  "cover": "https://y.qq.com/music/common/upload/t_area_panel/4145812.jpg",
  "miscellany": { "schemaType": "2", "schemaNewParams": "17291185", "type": "3003" }
}
```

```json
// 200：单集
{
  "type": 200,
  "id": "335642933",
  "title": "073成功减肥你可能需要这个？风起洛阳此类作品是否太过模式化？警惕网咯健身房骗局！",
  "cover": "https://y.gtimg.cn/music/photo_new/T002R300x300M000002ESSxl21Ptcv_3.jpg",
  "miscellany": {
    "albumname": "打个招呼",
    "singername": "打个招呼",
    "payplay": "0",
    "paystatus": "0",
    "paytimefree": "0",
    "isonly": "0"
  }
}
```

字段缺失情况：

| 卡片 | 可能的缺值 | 处理 |
|---|---|---|
| 400 | `subid`（albumMid）为空 | 实测播客/有声书专区全部有值；为空时用 `AlbumDetail(albumID)` 反查 |
| 2100 | `schemaNewParams` 为空 | 主播卡降级为纯展示 |
| 200 | `miscellany` 缺字段 | 只用 `id` + `title` |

### 3.5 播客专区实测楼层

| # | shelf | moduleType | 标题 | 卡片 | 用途 |
|---|---|---|---|---|---|
| 0 | 4579 | 1002 | — | 6 × 2000 | 顶部 Banner |
| 1 | 5493 | 1004 | 近期热点 | 7 × 200 | 最新单集 |
| 2 | 4466 | 1002 | — | 3 × 2000 | Banner |
| 3 | 5494 | 1003 | 明星播客 | 12 × 2100 | 主播/节目 |
| 4 | 5495 | 1005 | 热门节目 | 12 × 400 | 节目网格 |
| 5 | 5496 | 1003 | 知名媒体 | 10 × 2100 | 主播/节目 |
| 6 | 5499 | 1005 | 音乐漫谈 | 7 × 400 | 节目 |
| 7 | 5500 | 1005 | 影视综艺 | 12 × 400 | 节目 |
| 8 | 5501 | 1005 | 开心一刻 | 9 × 400 | 节目 |
| 9 | 5504 | 1005 | 讲个故事 | 10 × 400 | 节目 |
| 10 | 4578 | 1005 | 体育赛事 | 12 × 400 | 节目 |
| 11 | 5502 | 1005 | 人文科技 | 10 × 400 | 节目 |
| 12 | 5503 | 1003 | 优质节目 | 12 × 2100 | 主播/节目 |
| 13 | 5498 | 1005 | 播客新星 | 16 × 400 | 节目 |
| 14 | 3952 | 1019 | 这里有态度：一起头脑风暴吧~ | 1 × 2500 | 话题，暂不解析 |

`moduleType` 只是服务端给的楼层类型标签；**渲染按 `card.type` 分发即可**，不要依赖 `moduleType`。

### 3.6 有声书专区（同一套结构）

`encArea=003jmP4E20VguJ`（免费有声书，area 226）：11 个楼层、92 张 `400` 卡（全部 `miscellany.area="1"`）、4 张 Banner，最后一个楼层同样是 `1019` + `2500` 话题卡。

差异点：

| 字段 | 播客 | 有声书 |
|---|---|---|
| `miscellany.area` | `"0"` | `"1"` |
| `miscellany.fpaymid` | 多为空 | 付费商品 mid，可能非空 |
| 单集数量 | 几十到几百 | 常见上千（实测有 3332 集） |
| 音源 | 通常 128k MP3 | 可能有 320k / 更多格式 |

结论：**一套 `getAreaHomePage` 同时能服务小宇宙和有声书两个入口**，只需换 `encArea`。

## 4. 播客推荐 `getRadioList`

```go
req := qqmusic.Req{
    Module: "music.longRadio.recommend",
    Method: "getRadioList",
    Param:  map[string]any{}, // page/num 实测被忽略
}
```

返回：

```text
data
├── radioList[6]    ← 每次都是 6 个，内容随机
├── ext { moreJumpUrl, moreTitle }
└── title           ← 实测为空
```

`radioList[]` 就是 3.4 的卡片结构，只是类型固定：

```json
{
  "type": 0,
  "jumptype": 10025,
  "id": "19012469",
  "title": "喷嚏",
  "subtitle": "喷嚏官方",
  "cover": "https://y.gtimg.cn/music/photo_new/T002R500x500M000002N4K0D19jigu.jpg",
  "cnt": 173540,
  "miscellany": {
    "anchor": "喷嚏官方",
    "badgeImageUrl": "",
    "fpaymid": "",
    "recommend": "《喷嚏》的主播是两位女创作者，竹子和韩夏……"
  }
}
```

| 字段 | 说明 |
|---|---|
| `id` | albumID（不是 mid）。已验证：`19012469` → 专辑「喷嚏」，`albumMid=002N4K0D19jigu` |
| `title` / `subtitle` | 节目名 / 主播 |
| `cnt` | 热度值（播放或订阅口径未验证） |
| `miscellany.anchor` | 主播名，和 `subtitle` 基本重复 |
| `miscellany.recommend` | **节目简介**，几十到上百字，适合做推荐卡副文案 |
| `ext.moreJumpUrl` | `qqmusic://qq.com/ui/categoryChannel?p={"categoryId":"42800344"}`，是客户端原生页，网页端没有对应接口 |

特点与用法：

- 实测连续调用 3 次，返回的 6 个节目完全不同 → 天然适合「换一批」。
- 只给 albumID，不带 albumMid；点开前要 `GetAlbumDetail(albumID)` 反查。
- `getRadioList` 不能分页，也不能按分类取；分类靠专区楼层。

## 5. 节目（专辑）

### 5.1 `GetAlbumDetail`

```go
req := qqmusic.Req{
    Module: "music.musichallAlbum.AlbumInfoServer",
    Method: "GetAlbumDetail",
    Param:  map[string]any{"albumMid": "001N8TFz49WZol"}, // 或 {"albumID": 20102232}
}
```

两种参数都实测可用：`albumMid` 用于 400 卡，`albumID` 用于 2100 主播卡和 `getRadioList` 卡（实测 `albumID=17291185` → 「王晰 | 城市漫行」，`albumMid=003tbIDl16Y0XE`）。

返回：

```text
data
├── basicInfo { albumID, albumMid, albumName, publishDate, desc, albumType, type, language, albumDuration, ... }
├── singer { singerList[] { mid, name, singerID, type, singerType, indentity, ... } }
├── company { ID, name, headPic, isShow, brief }
└── extra
```

| 字段 | 实测值（无乐不谈） | 用途 |
|---|---|---|
| `albumID` | 20102232 | 数字 id |
| `albumMid` | 001N8TFz49WZol | 后续 `GetAlbumSongList` 用 |
| `albumName` | 无乐不谈 | 节目名 |
| `albumType` | **人声音频** | 判断是不是播客/有声书 |
| `type` | **10025** | 长音频专辑类型码 |
| `publishDate` | 2021-06-03 | 节目开播日期 |
| `desc` | 世界喧哗，还好我们有音乐…… | **节目简介**，带 `\n`，是唯一的 shownotes 来源 |
| `language` | 国语 | |
| `albumDuration` | 1217 | 口径不明，不要当总时长 |
| `modifyTime` | 1642664468 | |
| `singer.singerList[]` | 王东 / 谭维维 / … | 主播；实测主播王东 `singerType=14`，但不能当判定条件 |
| `company.name` | 独立发行 | |

识别播客：

```go
func (a AlbumInfo) IsPodcast() bool { return a.Type == "人声音频" }
```

### 5.2 `GetAlbumSongList`（单集分页）

```go
req := qqmusic.Req{
    Module: "music.musichallAlbum.AlbumSongList",
    Method: "GetAlbumSongList",
    Param: map[string]any{
        "albumMid": "002JAF3D0nJ33K",
        "begin":    0,
        "num":      30,
        "order":    2,
    },
}
```

返回：

```text
data
├── albumMid
├── totalNum          ← 总集数
├── curBegin
├── sort
├── songList[] {
│     songInfo { ... },   ← 见第 6 节
│     listenCount,        ← 单集播放量
│     uploadTime,         ← "2022-01-21"
│     isThemeSong,
│     teamStr
│   }
└── ...
```

分页与排序：

- `begin` 从 0 开始，`num` 实测 3/100 都行；现有 `AlbumDetail` 用 100。
- `order=2` 是客户端在用的值：`time_public` 不同时**新的在前**；老节目被整批导入、`time_public` 全相同（如日谈公园全是 2022-06-14），此时按服务端索引顺序（`index_album` 1,2,3…）。
- 播客动辄几百集：日谈公园 `totalNum=839`，一次 100 条要 9 次请求。**页面必须按需分页，不要一进节目页就拉全量**（现有 `AlbumDetail` 是全量循环，直接拿来打开 839 集的节目会连发 9 个请求）。
- `index_album` 是集号（从 1 开始），`listenCount` 是单集播放量，`uploadTime` 是单集日期 —— 这三个字段现有 `Song` 结构没有，做节目页时要补。

## 6. 单集（Song）

### 6.1 结构

单集就是标准 `Song`，实测字段：

```text
id, type(0), mid, name, title, subtitle,
singer[] { id, mid, name, title, type, uin, pmid },
album  { id, mid, name, title, subtitle, time_public, pmid },
mv { id, vid, name, title, vt },
interval,            ← 秒，就是单集时长
isonly, language, genre,
index_cd, index_album, time_public,
file { media_mid, size_48aac, size_96aac, size_128mp3, size_320mp3, size_192aac, size_192ogg, size_96ogg, size_flac, ... },
pay { pay_month, price_track, price_album, pay_play, pay_down, pay_status, time_free }
```

补充：

- `name` 是单集标题，通常已经带 `E255` / `vol.01` 这类编号；`title` 与 `name` 相同。
- `subtitle` 实测为空。
- 没有单集简介、没有 shownotes、没有时长以外的元数据。
- `album.mid` 是节目 mid，`singer[0]` 是主播；`singer[].type` 不固定（节目账号 3、个人主播 0/1），不要用它判断。

### 6.2 音源大小实测

| 单集 | 时长 | 128k MP3 | 320k MP3 | 96k AAC | 48k AAC | 96k OGG | 192k AAC |
|---|---|---|---|---|---|---|---|
| 日谈公园 vol.01 | 6938s | 111,018,542 | 0 | 84,516,445 | 42,259,136 | 63,696,542 | 0 |
| 无乐不谈 ep38 | 946s | 15,137,861 | 37,844,330 | 11,522,163 | 5,760,440 | 10,039,813 | 22,866,465 |

结论：

- **小宇宙节目普遍只有 128k MP3**（日谈公园没有 320k / FLAC）。
- QQ 音乐自制播客可能有 320k（无乐不谈有）。
- `File.files` 的现有降级顺序（FLAC → 320 → 128 → AAC）可以直接用；对播客实际会落到 128k。

## 7. 播放

### 7.1 `SongURL` 实测

| 单集 | 请求音质 | 实际返回 |
|---|---|---|
| 无乐不谈 ep38（有 320k） | `Best` | **HQ 320k**，`M800001z32CP01Zc09.mp3` |
| 日谈公园 vol.01（只有 128k） | `Best` | **标准 128k**，`M5000004Xnzp48B0ox.mp3` |

- 需要登录；未登录返回 `ErrNotLoggedIn`。
- 播放地址约一天失效，不要缓存。
- 批量取地址会触发 `104009` 限流（见 `AGENTS.md` §2 第 11 条）。

### 7.2 内存

`player.buffer` 会把**整集**留在内存里：

| 时长 | 128k MP3 | 320k MP3 | AAC 96k |
|---|---|---|---|
| 30 分钟 | 28 MB | 72 MB | 21 MB |
| 60 分钟 | 56 MB | 144 MB | 43 MB |
| 116 分钟（日谈公园 vol.01） | 111 MB | — | 84 MB（且 AAC 要整文件下完才开播） |

建议：

- **小宇宙播放固定用 `Standard`（128k）**，不要跟随全局「最高音质」，否则遇到有 320k 的自制播客会直接吃掉 100 MB+。
- 如果以后要支持 2 小时以上的有声书，需要给 `buffer` 加窗口/落盘，否则单集就能把内存顶到 200 MB。
- AAC 方案对播客不合适：要等整集下完才开始播。

## 8. 主播

### 8.1 `SingerDetail`

```go
Param: map[string]any{
    "singer_mids": []string{"000TPT0w1Ocqxl"},
    "ex_singer": 1, "wiki_singer": 1, "group_singer": 0, "pic": 1, "photos": 0,
}
```

`wiki_singer=1` 才会带 `wiki`。实测（日谈公园）：

```json
{
  "basic_info": { "singer_mid": "000TPT0w1Ocqxl", "name": "日谈公园", "type": 3, "singer_id": 1497044 },
  "ex_info": { "area": 1, "desc": "这个人很神秘，什么都没有留下~", "birthday": "2016-09-26" },
  "wiki": "<info><desc><![CDATA[“日谈公园”是由文娱领域资深人士李叔（李志明）和小伙子（冯广健）共同主持的……]]></desc>…</info>"
}
```

| 字段 | 说明 |
|---|---|
| `basic_info.type` | **不能当判定条件**：节目账号（日谈公园）= 3，个人主播（王东）= 0 |
| `ex_info.desc` | 多数是占位文案，不能当简介 |
| `ex_info.birthday` | 节目账号常是开播日，个人主播可能是真生日 |
| `wiki` | XML 字符串，`<desc><![CDATA[...]]>` 里是真正的简介；现有 `SingerDetail` 传的是 `wiki_singer:0`，要改成 1 |

### 8.2 `SingerAlbums`

```go
Param: map[string]any{"singerMid": "000TPT0w1Ocqxl", "order": 1, "begin": 0, "num": 30}
```

实测返回 12 个节目，全部 `albumType="人声音频"`：

```json
{
  "albumID": 26316727,
  "albumMid": "002JAF3D0nJ33K",
  "albumName": "日谈公园",
  "albumType": "人声音频",
  "publishDate": "2022-06-14",
  "totalNum": 839,
  "singerName": "日谈公园",
  "tags": [], "specIcon": "", "pubText": ""
}
```

- `albumType` 可直接过滤播客。
- `totalNum` 是集数 —— 现有 `AlbumInfo` 没有这个字段，做主播页要加。

## 9. 搜索

### 9.1 按专辑搜（推荐）

```go
Param: map[string]any{"query": "日谈公园", "num_per_page": 5, "page_num": 1, "search_type": 2}
```

返回 `body.album.list[]`：

```json
{
  "albumID": 26316727,
  "albumMID": "002JAF3D0nJ33K",
  "albumName": "日谈公园",
  "albumPic": "http://y.gtimg.cn/music/photo_new/T002R180x180M000002JAF3D0nJ33K_3.jpg",
  "publicTime": "2022-06-14",
  "singerID": 1497044,
  "singerMID": "000TPT0w1Ocqxl",
  "singerName": "日谈公园"
}
```

注意：

- 结果里**没有 `albumType`**；虽然有个 `type` 字段，但播客和普通专辑实测都是 `0`，不能用来判断。要确认只能 `GetAlbumDetail`，或查 `SingerAlbums`。
- `song_count` 就是集数（日谈公园 839、日谈物语 31），可以直接显示「N 集」。
- `singer_list[]` 是主播，结构和歌曲里的 `singer` 一样。
- `meta.sum=12`，正好是主播名下的 12 个节目。
- 搜索接口会随机返回 `2001`，沿用现有 2 次重试。

### 9.2 按歌手搜

`search_type=1` 返回歌手。播客主播会混在普通歌手里，字段里的 `type` / `singerType` 不能稳定区分（节目账号和个人主播取值不同），要判断只能看 `SingerAlbums` 的 `albumType`。

### 9.3 直达卡片

搜播客名时，`zhida` 通常给的是**歌手直达卡**（`from:"singer"`）：

```json
{
  "custom_info": {
    "mid": "000TPT0w1Ocqxl",
    "song_num": "1492",
    "album_num": "12",
    "extra_desc": "1.8万人关注",
    "source_d": "3002"
  },
  "pic": "http://y.gtimg.cn/music/photo_new/T001R150x150M000000TPT0w1Ocqxl_4.jpg"
}
```

现有 `parseDirect` 已经能解析成 `DirectSinger`，但 label 会是「歌手」。小宇宙页面里可以把它文案改成「播客」，或直接跳过直达卡、用专辑搜索。

**播客直达卡（重要，2026-10-27 实测）**：按播客名搜索时，`zhida.list` 可能有多条，播客常在**第二条**（第一条是同名歌手）。例如搜「来都来了」：`list[0]` 是「歌手: 黄诗扶」，`list[1]` 才是播客：

```json
{
  "type": 2,
  "title": "播客: 来都来了 | 听了再走",
  "desciption": "来都来了_  283集",
  "custom_info": { "channel_type": "10025", "mid": "003DKRD82GxqiR", "track_num": "283", "play_count": "89万", "singer_mid": "0009jMpE3DeoTR" }
}
```

- 识别：`custom_info.channel_type == "10025"`（长音频），`mid` 是节目 albumMid，`track_num` 是集数。
- 这个节目**不在** `search_type=2`（专辑）的结果里，只能从直达拿到，所以 `SearchPodcasts` 第 1 页会先做一次 `search_type=0` 读直达（`directPodcasts`），再合并专辑搜索的结果。
- 现有 `parseDirect` 只看 `list[0]`，主搜索页因此也看不到这张播客卡。

### 9.4 长音频搜索 `search_type=15`

`search_type=15` 返回 `body.album.list[]`，全是长音频专辑（结构同 9.1，多一个 `catch_song` 表示命中的单集标题）。但它**按单集标题匹配排序**：搜「来都来了」返回 113 个，前 20 个全是标题含「来都来了」章节的有声书，目标播客不在其中。不适合按节目名搜索，暂未使用。

连续快速搜索容易触发 `2001`（重试后仍失败），探测时每次搜索之间要留间隔。

## 10. 字幕与评论

### 10.1 单集字幕（重要）

`music.musichallSong.PlayLyricInfo / GetPlayLyricInfo` 对小宇宙单集返回 **AI 智能字幕**：

```text
[ti:]
[ar:]
[al:]
[by:AI智能字幕]
[offset:0]
[00:00.00]本字幕由天琴实验室独家AI技术生成
[00:00.00]大家好
[00:00.45]欢迎来到吾悦不谈的花絮环节
[00:02.43]让我们再次有请吴月
...
```

- 是标准 LRC，现有 `ParseLRC` + 歌词页可以直接用。
- 实测单集 946 秒、字幕约 12 KB；长节目会更大（按 1 小时估 40–60 KB）。
- 不是每集都有；没有时接口返回空串，按「无歌词」处理。
- `GetPlayLyricInfo` 的 `trans` 对播客通常为空。
- 响应里还有 `introduceTitle` / `introduceText`（base64 的「介绍」「背景」），实测内容为空，不要指望它是 shownotes。
- 游客也能读，不需要登录。

### 10.2 评论

现有 `Comments(BizType=1, BizId=songID)` 对单集可用：

```text
TotalCmNum: 38
CommentList.Comments[] { CmId, Nick, Avatar, Content, PubTime, PraiseNum, ReplyCnt, SubComments[] }
```

- 单集评论量比歌曲少，`PageSize=10` 够用。
- 评论里的 `Content` 仍要过 `cleanComment`。

### 10.3 没有的东西

`music.pf_song_detail_svr / get_song_detail` 也查过：

```text
info[] = 演唱者 / 歌曲语种 / 唱片公司 / 歌词
extras = { name, transname, subtitle, from, wikiurl }
```

**没有单集简介、没有 shownotes、没有时间戳章节**。想要 shownotes 只能回小宇宙官方网页/接口（见 `小宇宙播客接口指南.md`），不建议为它再开一条通道。

## 11. 首页信息流里的小宇宙卡片

`music.recommend.RecommendFeed / get_recommend_feed`（app comm）的 shelf 301 里有：

| 字段 | 值 |
|---|---|
| `type` | 900 |
| `subtype` | 994 |
| `jumptype` | 10046 |
| `id` | 736382210（单集 songid） |
| `title` | 小宇宙 |
| `cover` | `https://y.gtimg.cn/music/photo_new/T002R500x500M000000omnsg344lUQ_5.jpg` |
| `scheme` | `qqmusic://qq.com/ui/openRN?p={"key":"TodayFmAudio",...}` |

- `id` 当普通 songid 用 `CgiGetTrackInfo` 就能取到单集，再顺出 `album.mid`。
- 现有 `RecommendFeed` 只保留首卡是 200/500 的 shelf，会把它整块丢掉。小宇宙模块**不需要**依赖它，最多做成首页的一个入口。

## 12. 后续开发建议

### 12.1 API 层（`internal/qqmusic`）

新增（都走游客可用路径）：

```go
// LongAudioArea returns the shelves of a long audio area, such as 播客专区.
func (c *Client) LongAudioArea(ctx context.Context, encArea string) (LongAudioArea, error)

// LongAudioRadio returns the podcasts the 电台 tab recommends.
func (c *Client) LongAudioRadio(ctx context.Context) ([]LongAudioCard, error)

// AlbumDetailByID is AlbumDetail for callers that only have the numeric id.
func (c *Client) AlbumDetailByID(ctx context.Context, id int64) (AlbumInfo, error)

// AlbumSongs returns one page of an album's songs, newest first, and the total.
func (c *Client) AlbumSongs(ctx context.Context, albumMid string, begin, num int) ([]Episode, int, error)
```

模型建议：

```go
type LongAudioArea struct {
    ID      int
    Title   string
    Desc    string
    Cover   string
    Shelves []LongAudioShelf
}

type LongAudioShelf struct {
    ID    int
    Title string
    Cards []LongAudioCard
}

type LongAudioCardKind int

const (
    LongAudioAlbum LongAudioCardKind = iota // card type 400
    LongAudioEpisode                        // card type 200
    LongAudioHost                           // card type 2100
    LongAudioBanner                         // card type 2000
)

type LongAudioCard struct {
    Kind         LongAudioCardKind
    AlbumID      int64  // 400, 2100, and the radio cards
    AlbumMid     string // 400 only
    SongID       int64  // 200 only
    Title        string
    Subtitle     string
    Cover        string
    EpisodeCount int    // 400: miscellany.num
    ReleaseDate  string // 400: miscellany.release_time
    Area         int    // 400: 0 播客, 1 有声书
    PlayCount    int64  // radio: cnt
    Intro        string // radio: miscellany.recommend
}

type Episode struct {
    Song
    IndexAlbum  int    // 集号
    ListenCount int64  // 单集播放量
    UploadTime  string // "2022-01-21"
}
```

注意：

- `Song` 现有字段够播，但要显示集号/播放量/日期，需要 `Episode` 包装。
- `AlbumInfo` 建议加 `EpisodeCount int`（`SingerAlbums.totalNum`）和 `IsPodcast()`。
- `SingerDetail` 的 `wiki_singer` 改成 1，并解析 `wiki` 里的 `<desc><![CDATA[...]]>`。

### 12.2 页面与状态

```
pageKind += pagePodcast     // 小宇宙首页
pageKind += pagePodcastShow // 可选：直接复用 pageAlbum
```

- `page.mid` 存 `encArea`（播客专区），这样刷新/回退都能重建。
- `app` 新增 `podcast` 分组：

```go
type podcastPage struct {
    area    qqmusic.LongAudioArea
    radio   []qqmusic.LongAudioCard
    loading bool
    loaded  bool
    err     string
    gen     int
}
```

- 加载走 `a.do`，结果回主线程再落状态；失败保留旧数据。
- 节目页复用 `pageAlbum` + `a.lists[key]`，但 **单集分页**：首次取 30 条，滚到底再取下一页，用 `begin` 而不是现有 `AlbumDetail` 的全量循环。

### 12.3 UI

| 区域 | 数据 | 复用 |
|---|---|---|
| 侧边栏 | 新增 `navItem(icPodcast, "小宇宙")`，放在「排行榜」和「搜索」之间 | `view.go` |
| 页面头部 | `area.title` / `area.share.desc` / `area.cover` + tint | `heading` + `pageTint` |
| 推荐行 | `getRadioList` 6 张，右上角「换一批」再拉一次 | `songCards` / `forYouCard` |
| 节目楼层 | 400 卡 → 封面 + 节目名 + 主播 + 「N 集」 | `playlistGrid` / `songCards` |
| 主播楼层 | 2100 卡 → 头像 + 名字，点击进节目 | `singerCard` |
| 最新单集 | 200 卡 → 单集列表，点击播放 | `songTable` 的行 |
| 节目页 | `AlbumDetail` 简介 + `AlbumSongList` 单集，文案用「集」不用「首」 | `pageAlbum` 改造 |
| 歌词页 | AI 字幕直接当歌词 | 现成 |

文案要点：

- 播客不要显示「专辑」，显示「节目」；单集不要显示「歌曲」，显示「单集」；集数用「N 集」。
- 播客页隐藏「无损/HQ」音质选项，或只显示实际拿到的音质。
- 播放前如果未登录，走现有登录弹窗。

### 12.4 播放与内存

- 进入小宇宙页面后，`playList` 传 `Quality(Standard)`，或给 `playAt` 加一个「长音频固定 128k」的分支。
- 单集时长动辄 1 小时，`buffer` 全量驻留；先按 128k 上线，后续再考虑窗口缓冲。
- 连续失败跳过逻辑、`playGen` 代际号、`trimmer` 都沿用现有播放链路。

## 13. 限制与风险

1. **专区码是硬编码的**：`encArea` 没有列表接口，只能从 H5/客户端拿。服务端换码就整页空，必须准备 `getRadioList` 兜底。
2. **专区数据旧**：播客专区 `modifyTime` 停在 2021-11-25，部分节目已停更；`getRadioList` 才是新的。两者混用可缓解。
3. **没有找到订阅/收藏播客的接口**：只能收藏单集（`LikeSong`，songid + type 0）；专辑收藏接口本次未验证。
4. **没有单集简介**：只有节目简介（`basicInfo.desc`）和 AI 字幕；需要 shownotes 就得回小宇宙官方通道。
5. **付费单集**：`pay.pay_play=1` 的要有会员；有声书的 `fpaymid` 是付费商品，别当普通专辑处理。
6. **话题楼层（1019 / 2500）未解析**：播客专区最后一个楼层是讨论模块，需要另外的接口，先跳过。
7. **`getRadioList` 无分页/无分类**：每次 6 个随机；不要循环调用刷数据。
8. **`AlbumDetail` 全量循环的坑**：现有实现对 839 集的节目会连发 9 个 `GetAlbumSongList`；播客页要改成按需分页。
9. **320k 会放大内存**：自制播客有 320k，2 小时就是 288 MB；固定 128k。
10. **接口随时变**：`moduleType` / `card.type` / `miscellany` 字段都是服务端下发，解析要允许缺字段，按 `card.type` 分支时要有 default 丢弃。
11. **限流**：`music.vkey.GetVkey` 仍然是唯一的高危接口；浏览专区、节目、单集都轻，播放地址要按 `AGENTS.md` §2 第 11 条控制。

## 14. 复现方式

本次用的临时探针：`cmd/zz_podcast`（本地工具，未纳入 git）。

```powershell
go run ./cmd/zz_podcast "$env:AppData\QQMusicGo\credential.json"
```

它会把原始 JSON 写到 `%TEMP%\qqscan\out\`：

| 文件 | 内容 |
|---|---|
| `area.json` | 播客专区完整楼层 |
| `longradio.json` | `getRadioList` |
| `album.json` | `GetAlbumDetail(albumMid)` |
| `album_by_id.json` | `GetAlbumDetail(albumID)` |
| `albumsongs.json` / `podcastsongs.json` | 单集分页（含 `listenCount` / `uploadTime`） |
| `track.json` / `song_detail.json` | 单集详情 |
| `singer.json` / `singer_albums.json` | 主播与主播的节目 |
| `search.json` / `search_album.json` | 搜索（单曲 / 专辑） |
| `comments.json` / `lyric.json` | 评论 / AI 字幕 |

跑完一轮 19 个请求（其中 2 个取播放地址），都是串行、不遍历列表，符合 `AGENTS.md` §2 第 11 条。要换专区只改 `area` 那一步的 `encArea`。

## 15. 一页速查

```text
网关:    POST https://u.y.qq.com/cgi-bin/musicu.fcg
信封:    { "PoolApiServer": { module, method, param } } -> { PoolApiServer: { code, data } }
鉴权:    浏览不需要登录；SongURL 需要

专区:    music.area.AreaHome / getAreaHomePage
         param { cmd:0, encArea:"004Z03m934o6En" }   // 播客专区 area 206
         data { id, title, desc, cover, color, share, button, v_shelf[] }
shelf:   { id, title_template, miscellany.moduleType, v_niche[].v_card[] }

卡片:    400  节目    id=albumID, subid=albumMid, subtitle=主播, misc.num=集数, misc.area=0/1
         200  单集    id=songID
         2100 主播    title=人名, misc.schemaNewParams=albumID
         2000 Banner  misc.schemaType/schemaNewParams
         2500 话题    未解析

推荐:    music.longRadio.recommend / getRadioList   // 6 个随机，带简介
节目:    music.musichallAlbum.AlbumInfoServer / GetAlbumDetail   // albumMid 或 albumID
         basicInfo.albumType = "人声音频", type = 10025, desc = 节目简介
单集:    music.musichallAlbum.AlbumSongList / GetAlbumSongList
         { albumMid, begin, num, order:2 } -> totalNum, songList[].songInfo
主播:    musichall.singer_info_server / GetSingerDetail   // wiki_singer=1 拿简介
         music.musichallAlbum.AlbumListServer / GetAlbumList -> albumType/totalNum

播放:    music.vkey.GetVkey / UrlGetVkey  （经 SongURL）
         小宇宙多为 M500 128k；自制播客可能 M800 320k
字幕:    music.musichallSong.PlayLyricInfo / GetPlayLyricInfo   // AI 智能字幕 LRC
评论:    music.globalComment.CommentRead / GetHotCommentList   // BizType 1, BizId=songID

实测:    播客专区 15 楼层 / 84 张不同节目卡（400）/ 33 个主播卡 / 7 个单集卡
         日谈公园 839 集，6938s，111 MB@128k，无 320k
         无乐不谈 38 集，946s，有 320k，SongURL(Best)=HQ 320k
```

## 参考

| 来源 | 用途 |
|---|---|
| 本仓库 `docs/小宇宙播客接口指南.md` | 小宇宙官方通道（API / 网页 `__NEXT_DATA__`），与本文的「QQ 音乐内置」互为补充 |
| 本仓库 `docs/qq音乐分析指南.md` §10 | 小宇宙卡片、`CgiGetTrackInfo`、`AlbumDetail` 的首次实测 |
| `github.com/YAQMC/YAQMC` `docs/zh-CN/discover.md` | 指出 `getAreaHomePage`、`getRadioList`、`getCategoryAreaInCategoryPlaylist` 的存在，本文已逐个实测 |
| QQ 音乐开放平台 `ILongAudioAPI` | 长音频的产品概念（分类 / 榜单 / 推荐 / 收藏），对应的是同一批曲库数据 |
