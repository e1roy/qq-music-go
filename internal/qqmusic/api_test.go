package qqmusic

import (
	"encoding/json"
	"testing"
)

// TestParseDirect reads zhida as the search sends it.
func TestParseDirect(t *testing.T) {
	singer := `{"list":[{"type":1,"title":"歌手: 毛不易","desciption":"歌曲:536 专辑:101 视频:1703","pic":"http://y.gtimg.cn/x.jpg",
		"custom_info":{"mid":"001BHDR33FZVZ0","song_num":"536","album_num":"101","mv_num":"1703","extra_desc":"916.3万人关注"}}]}`
	d := parseDirect([]byte(singer))
	if d == nil || d.Kind != DirectSinger || d.Label != "歌手" || d.Name != "毛不易" || d.Mid != "001BHDR33FZVZ0" ||
		d.SongCount != 536 || d.AlbumCount != 101 || d.MVCount != 1703 || d.Desc != "916.3万人关注" {
		t.Errorf("singer: %+v", d)
	}

	album := `{"list":[{"type":2,"title":"专辑: 范特西","desciption":"周杰伦  10首  2001-09-14",
		"custom_info":{"mid":"000I5jJB3blWeN","track_num":"10"}}]}`
	d = parseDirect([]byte(album))
	if d == nil || d.Kind != DirectAlbum || d.Name != "范特西" || d.Desc != "周杰伦 · 10首 · 2001-09-14" || d.Pic != AlbumCover("000I5jJB3blWeN", 300) {
		t.Errorf("album: %+v", d)
	}

	show := `{"list":[{"type":4,"title":"综艺：明日创作计划","custom_info":{"mid":"001fXbpZ0Nl6l8","song_num":"204","album_num":"10","mv_num":"0"}}]}`
	d = parseDirect([]byte(show))
	if d == nil || d.Kind != DirectSinger || d.Label != "综艺" || d.Name != "明日创作计划" {
		t.Errorf("show: %+v", d)
	}

	for _, s := range []string{``, `{"list":[]}`, `{"list":[{"type":9,"title":"x","custom_info":{"mid":"y"}}]}`, `{"list":[{"type":1,"custom_info":{"mid":3}}]}`} {
		if d := parseDirect([]byte(s)); d != nil {
			t.Errorf("%s: %+v", s, d)
		}
	}
}

// TestCleanComment makes comments plain text as they come.
func TestCleanComment(t *testing.T) {
	for in, want := range map[string]string{
		`我没有忘记你\n如果幸福的话`:         "我没有忘记你\n如果幸福的话",
		"还有双截棍\uFFFC":            "还有双截棍",
		" 好听[em]e400846[/em] ":   "好听",
		"&quot;晴天&quot; &amp; 雨": `"晴天" & 雨`,
	} {
		if got := cleanComment(in); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
}

// TestPodcastArea reads the shelves of 播客专区 as getAreaHomePage sends
// them: shows, hosts and episodes, the others left out.
func TestPodcastArea(t *testing.T) {
	const raw = `{"title":"播客专区","share":{"desc":"听见闪闪发光的灵魂"},"v_shelf":[
		{"title_template":"","v_niche":[{"v_card":[{"type":2000,"id":"card_index_4579_0","miscellany":{"schemaType":"7","schemaNewParams":"331892789"}}]}]},
		{"title_template":"近期热点","v_niche":[{"v_card":[{"type":200,"id":"335642933","title":"073"},{"type":200,"id":"336316446"}]}]},
		{"title_template":"明星播客","v_niche":[{"v_card":[
			{"type":2100,"id":"card_index_5494_0","title":"王晰","cover":"x.jpg","miscellany":{"schemaType":"2","schemaNewParams":"17291185"}},
			{"type":2100,"id":"card_index_5494_1","title":"空","miscellany":{"schemaType":"2","schemaNewParams":""}}]}]},
		{"title_template":"热门节目","v_niche":[{"v_card":[
			{"type":400,"id":"20102232","subid":"001N8TFz49WZol","title":"无乐不谈","subtitle":"王东/谭维维","miscellany":{"num":"38"}},
			{"type":400,"id":"20102232","subid":"001N8TFz49WZol","title":"无乐不谈"}]},
			{"v_card":[{"type":400,"id":20102233,"subid":"","title":"&amp;仅有 id"}]}]},
		{"title_template":"话题","v_niche":[{"v_card":[{"type":2500,"id":20}]}]}]}`
	var r rawArea
	if err := json.Unmarshal([]byte(raw), &r); err != nil {
		t.Fatal(err)
	}
	a, ids := r.area()
	if a.Title != "播客专区" || a.Desc != "听见闪闪发光的灵魂" || len(a.Shelves) != 3 {
		t.Fatalf("area: %+v", a)
	}
	if got := ids[0]; len(got) != 2 || got[0] != 335642933 || a.Shelves[0].Title != "近期热点" {
		t.Errorf("episodes: %v %+v", ids, a.Shelves[0])
	}
	if h := a.Shelves[1]; !h.Hosts || len(h.Shows) != 1 || h.Shows[0].ID != 17291185 || h.Shows[0].Title != "王晰" {
		t.Errorf("hosts: %+v", h)
	}
	s := a.Shelves[2]
	if s.Hosts || len(s.Shows) != 2 || s.Shows[0].Mid != "001N8TFz49WZol" || s.Shows[0].Episodes != 38 ||
		s.Shows[0].Host != "王东/谭维维" || s.Shows[1].ID != 20102233 || s.Shows[1].Title != "&仅有 id" {
		t.Errorf("shows: %+v", s)
	}
}

// TestDirectPodcasts reads the podcasts a search goes straight to, beside a
// singer, as the search of 来都来了 sends them.
func TestDirectPodcasts(t *testing.T) {
	const zhida = `{"list":[
		{"type":1,"title":"歌手: 黄诗扶","custom_info":{"mid":"0000Z1MN2gkvI6","song_num":"313","album_num":"78"}},
		{"type":2,"title":"播客: 来都来了 | 听了再走","desciption":"来都来了_  283集","pic":"http://y.gtimg.cn/x.jpg",
			"custom_info":{"area":"1","channel_type":"10025","mid":"003DKRD82GxqiR","track_num":"283","play_count":"89万"}},
		{"type":2,"title":"专辑: 范特西","custom_info":{"mid":"000I5jJB3blWeN","track_num":"10"}}]}`
	shows := directPodcasts([]byte(zhida))
	if len(shows) != 1 || shows[0].Mid != "003DKRD82GxqiR" || shows[0].Title != "来都来了 | 听了再走" ||
		shows[0].Episodes != 283 || shows[0].Cover != AlbumCover("003DKRD82GxqiR", 300) {
		t.Fatalf("shows: %+v", shows)
	}
	for _, s := range []string{``, `{"list":[]}`, `{"list":3}`} {
		if shows := directPodcasts([]byte(s)); shows != nil {
			t.Errorf("%s: %+v", s, shows)
		}
	}
}
