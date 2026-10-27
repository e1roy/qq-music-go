package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"

	"github.com/egoist/mygo/ui"

	"qqmusic/internal/qqmusic"
)

// podcasts is 小宇宙: QQ Music's podcasts, which need no login to browse,
// those picked for the user, those they listened to, and a search of
// them. A podcast (节目) is an album, and its episodes songs: a show opens
// as an album's page.
type podcasts struct {
	loaded, loading bool
	area            qqmusic.PodcastArea
	err             string
	picks           []qqmusic.Show // 为你推荐, others at each 换一批
	picksLoading    bool
	picksMotion     refreshMotion
	// heard is the shows listened to, the last first, kept in
	// podcasts.json. known is the albums known to be podcasts: the
	// episodes of those play at 128k, and make them heard.
	heard  []qqmusic.Show
	known  map[string]bool
	search podcastSearch
}

// heardMax is how many shows 我听过的节目 keeps.
const heardMax = 30

// podcastSearch is what a search of 小宇宙 found: shows or episodes, a tab
// each, loaded a page at a time as their tab shows. The title bar's box
// makes it while 小宇宙 is shown.
type podcastSearch struct {
	query    string
	tab      int // 0 shows, 1 episodes
	shows    podcastFound[qqmusic.Show]
	episodes podcastFound[qqmusic.Song]
}

// podcastFound is shows or episodes found, page the last page loaded. Of
// what the search finds, only the podcasts' are kept, so that a page may
// add none: more says whether pages follow.
type podcastFound[T any] struct {
	items   []T
	page    int
	more    bool
	loading bool
	err     string
	gen     int
	list    ui.ListState // as the search page's tab, which scrolls by itself
}

// The pages of a search of podcasts: albums are fewer than songs, and
// each page of them asks whether they are podcasts.
const (
	showsPage    = 20
	episodesPage = 30
)

// pageOf is the page of a show: its mid, or its id when that is all a
// card knows. A show has one page for either: its mid when known.
func pageOf(sh qqmusic.Show) page {
	if sh.Mid != "" {
		return page{kind: pageAlbum, mid: sh.Mid, title: sh.Title}
	}
	return page{kind: pageAlbum, id: sh.ID, title: sh.Title}
}

// loadPodcasts loads the shelves of 小宇宙 and the shows picked, the
// first time, or again.
func (a *app) loadPodcasts(again bool) {
	pc := &a.podcast
	if pc.loading || pc.loaded && !again {
		return
	}
	pc.loading, pc.err = true, ""
	a.do(func(ctx context.Context) func() {
		area, err := a.api.PodcastArea(ctx)
		picks, err2 := a.api.PodcastPicks(ctx)
		return func() {
			pc.loading, pc.loaded = false, true
			// Loaded again, what failed keeps what it showed. The area is
			// fixed: should it fail, the picks still show.
			if err == nil {
				pc.area = area
				for _, sh := range area.Shelves {
					for _, s := range sh.Shows {
						if s.Mid != "" {
							pc.known[s.Mid] = true
						}
					}
					for _, s := range sh.Episodes {
						pc.known[s.Album.Mid] = true
					}
				}
			}
			if err2 == nil && len(picks) > 0 {
				pc.picks = picks
			}
			pc.err = errText(errors.Join(err, err2))
		}
	})
}

// morePicks shows other podcasts picked: each call gives others.
func (a *app) morePicks() {
	pc := &a.podcast
	if pc.loading || pc.picksLoading {
		return
	}
	pc.picksLoading = true
	a.do(func(ctx context.Context) func() {
		picks, err := a.api.PodcastPicks(ctx)
		return func() {
			pc.picksLoading = false
			if err != nil {
				a.toast("换一批失败：" + errText(err))
				return
			}
			if len(picks) > 0 {
				pc.picks = picks
				pc.picksMotion.arm()
			}
		}
	})
}

// recentEpisodes is the episodes of 最近播放 that are podcasts', the last
// played first.
func (a *app) recentEpisodes(n int) []qqmusic.Song {
	var eps []qqmusic.Song
	for _, s := range a.recent {
		if a.podcast.known[s.Album.Mid] && len(eps) < n {
			eps = append(eps, s)
		}
	}
	return eps
}

// hearShow puts the show of an episode that played at the top of 我听过的
// 节目, which keeps what was known of it.
func (a *app) hearShow(s qqmusic.Song) {
	sh := qqmusic.Show{Mid: s.Album.Mid, Title: s.Album.Name, Host: s.Artists(), Cover: qqmusic.AlbumCover(s.Album.Mid, 300)}
	heard := make([]qqmusic.Show, 1, heardMax)
	for _, h := range a.podcast.heard {
		switch {
		case h.Mid == sh.Mid:
			if h.Episodes > 0 {
				sh.Episodes = h.Episodes
			}
		case len(heard) < heardMax:
			heard = append(heard, h)
		}
	}
	// What a show's page knows of it.
	if l := a.lists[page{kind: pageAlbum, mid: sh.Mid}]; l != nil && l.total > 0 {
		sh.Episodes = l.total
	}
	heard[0] = sh
	a.podcast.heard = heard
	a.saveHeard()
}

// clearHeard forgets the shows listened to.
func (a *app) clearHeard() {
	a.podcast.heard = nil
	a.saveHeard()
}

func (a *app) saveHeard() {
	if a.dir == "" {
		return
	}
	if len(a.podcast.heard) == 0 {
		os.Remove(a.path("podcasts.json"))
		return
	}
	os.MkdirAll(a.dir, 0o700)
	b, _ := json.Marshal(a.podcast.heard)
	os.WriteFile(a.path("podcasts.json"), b, 0o600)
}

// searchPodcasts searches 小宇宙 for q, loading the tab shown; an empty q
// goes back to its shelves.
func (a *app) searchPodcasts(q string) {
	s := &a.podcast.search
	s.query = strings.TrimSpace(q)
	s.shows = podcastFound[qqmusic.Show]{gen: s.shows.gen + 1}
	s.episodes = podcastFound[qqmusic.Song]{gen: s.episodes.gen + 1}
	a.main.Y = 0
	a.loadPodcastTab()
}

// showPodcastTab switches the search of 小宇宙 to a tab, which loads.
func (a *app) showPodcastTab(tab int) {
	a.podcast.search.tab = tab
	a.loadPodcastTab()
}

// loadPodcastTab loads the first page of the tab shown, if it has none:
// the service turns away searches that come together (code 2001).
func (a *app) loadPodcastTab() {
	s := &a.podcast.search
	switch {
	case s.query == "":
	case s.tab == 0 && s.shows.page == 0 && !s.shows.loading:
		a.moreShows()
	case s.tab == 1 && s.episodes.page == 0 && !s.episodes.loading:
		a.moreEpisodes()
	}
}

// moreShows loads the next page of the shows found.
func (a *app) moreShows() {
	podcastMore(a, &a.podcast.search.shows, a.podcast.search.query, func(ctx context.Context, q string, pg int) ([]qqmusic.Show, bool, error) {
		return a.api.SearchPodcasts(ctx, q, pg, showsPage)
	}, func(sh qqmusic.Show) (string, string) { return sh.Mid, sh.Mid })
}

// morePodcastsFound loads the next page of the podcasts the search page
// found, its 播客 tab.
func (a *app) morePodcastsFound() {
	podcastMore(a, &a.search.podcasts, a.search.query, func(ctx context.Context, q string, pg int) ([]qqmusic.Show, bool, error) {
		return a.api.SearchPodcasts(ctx, q, pg, showsPage)
	}, func(sh qqmusic.Show) (string, string) { return sh.Mid, sh.Mid })
}

// moreEpisodes loads the next page of the episodes found.
func (a *app) moreEpisodes() {
	podcastMore(a, &a.podcast.search.episodes, a.podcast.search.query, func(ctx context.Context, q string, pg int) ([]qqmusic.Song, bool, error) {
		return a.api.SearchEpisodes(ctx, q, pg, episodesPage)
	}, func(s qqmusic.Song) (string, string) { return s.Mid, s.Album.Mid })
}

// podcastMore loads the next page of what f found for q, adding what it
// has not yet: pages may overlap. ids gives an item's key, and the mid of
// its podcast, which it makes known.
func podcastMore[T any](a *app, f *podcastFound[T], q string, load func(ctx context.Context, q string, pg int) ([]T, bool, error), ids func(T) (key, album string)) {
	pg := f.page + 1
	f.loading, f.err = true, ""
	f.gen++
	gen := f.gen
	a.do(func(ctx context.Context) func() {
		items, more, err := load(ctx, q, pg)
		return func() {
			if gen != f.gen {
				return
			}
			f.loading, f.err = false, errText(err)
			if err != nil {
				return
			}
			seen := make(map[string]bool, len(f.items))
			for _, it := range f.items {
				k, _ := ids(it)
				seen[k] = true
			}
			for _, it := range items {
				k, album := ids(it)
				a.podcast.known[album] = true
				if !seen[k] {
					seen[k] = true
					f.items = append(f.items, it)
				}
			}
			f.page, f.more = pg, more
		}
	})
}
