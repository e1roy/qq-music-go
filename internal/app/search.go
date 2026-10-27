package app

import (
	"context"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"qqmusic/internal/qqmusic"
	"qqmusic/internal/xiaoyuzhou"
)

// searchTab is what the search page shows.
type searchTab int

const (
	foundSongs searchTab = iota
	foundSingers
	foundPlaylists
	foundPodcasts
)

// searchHistoryMax is how many searches the box remembers; suggestMax how
// many ways the text may go on it lists, asked for once typing rests
// suggestPause.
const (
	searchHistoryMax = 10
	suggestMax       = 6
	suggestPause     = 250 * time.Millisecond
)

func (a *app) runSearch(q string, pg int) {
	if q == "" {
		return
	}
	s := &a.search
	// Turning the pages of the songs keeps the singers, the playlists and
	// the podcasts found, and how many songs there are; a new query knows
	// none yet.
	if q != s.query {
		s.total, s.direct = 0, nil
		s.singers.reset()
		s.pls.reset()
		s.podcasts.reset()
		s.xiaoyuzhouShows.reset()
	}
	s.query, s.page, s.loading, s.err = q, pg, true, ""
	s.gen++
	gen := s.gen
	a.do(func(ctx context.Context) func() {
		songs, total, direct, err := a.api.Search(ctx, q, pg, 30)
		return func() {
			if gen != s.gen {
				return
			}
			s.loading, s.songs, s.total, s.err = false, songs, total, errText(err)
			// The other pages may go straight elsewhere, or nowhere.
			if pg == 1 && err == nil {
				s.direct = direct
			}
			s.list.ScrollTo(0, ui.Start)
		}
	})
	// The songs first, which the page shows first; the podcasts go beside
	// them, ready by the time their tab is shown. The search retries what
	// the service turns away as they come together (code 2001).
	a.loadSearchTab()
	a.loadSearchPodcasts()
	a.navigate(page{kind: pageSearch, title: "搜索"})
}

// suggestAsTyped asks the service, once typing rests, what the text in the
// box may become: a later key drops what came before it.
func (a *app) suggestAsTyped(q string) {
	s := &a.search
	// A key closes what the text before it waited for: one question per
	// rest, however fast the keys come.
	if s.typing != nil {
		close(s.typing)
	}
	s.typing = make(chan struct{})
	stop := s.typing
	s.sugGen++
	q = strings.TrimSpace(q)
	if q == "" {
		s.suggest, s.sugFor = nil, ""
		return
	}
	gen := s.sugGen
	a.do(func(ctx context.Context) func() {
		if !rest(ctx, stop, suggestPause) {
			return nil
		}
		words, _ := a.api.Suggest(ctx, q)
		return func() {
			if gen != s.sugGen {
				return
			}
			// Empty, repeated or past the few shown, they are no use.
			var out []string
			seen := make(map[string]bool, len(words))
			for _, w := range words {
				if w == "" || seen[w] || len(out) == suggestMax {
					continue
				}
				seen[w] = true
				out = append(out, w)
			}
			s.suggest, s.sugFor = out, q
			if s.sel >= len(out) {
				s.sel = len(out) - 1
			}
		}
	})
}

// searchWords is what the service says the text in the box may become, as
// the text stands now.
func (a *app) searchWords() []string {
	s := &a.search
	// What the service suggests is songs and singers, no podcasts.
	if s.sugFor != strings.TrimSpace(s.input) || a.searchScope() != scopeMusic {
		return nil
	}
	return s.suggest
}

// searchStep moves the choice below the box d suggestions down or up.
func (a *app) searchStep(d int) {
	s := &a.search
	words := a.searchWords()
	if len(words) == 0 {
		return
	}
	if s.sel < 0 {
		s.sel = 0
		if d < 0 {
			s.sel = len(words) - 1
		}
		return
	}
	if s.sel += d; s.sel < 0 {
		s.sel = 0
	} else if s.sel >= len(words) {
		s.sel = len(words) - 1
	}
}

// searchKeys walks the suggestions below the box as the arrows are pressed
// in it, which have nothing else to do in a line of text.
func (a *app) searchKeys(k ui.TextKey) bool {
	if k.Mods != 0 || k.Key != ui.KeyDown && k.Key != ui.KeyUp {
		return false
	}
	d := 1
	if k.Key == ui.KeyUp {
		d = -1
	}
	a.searchStep(d)
	return true
}

// chooseSearch searches what a row of the menu offers, and closes it.
func (a *app) chooseSearch(v string) {
	v = strings.TrimSpace(v)
	if v == "" {
		return
	}
	s := &a.search
	s.input, s.open, s.sel = v, false, -1
	a.rememberSearch(v)
	switch a.searchScope() {
	case scopePodcasts:
		a.searchPodcasts(v)
	case scopeXiaoyuzhou:
		a.searchXiaoyuzhou(v)
	default:
		a.runSearch(v, 1)
	}
}

// searchScope is what the title bar's box searches: QQ Music's songs, or
// on 小宇宙 its podcasts only, QQ Music's or 小宇宙's own.
type searchScope int

const (
	scopeMusic searchScope = iota
	scopePodcasts
	scopeXiaoyuzhou
)

func (a *app) searchScope() searchScope {
	switch a.current().kind {
	case pagePodcast:
		return scopePodcasts
	case pageXiaoyuzhou:
		return scopeXiaoyuzhou
	}
	return scopeMusic
}

// rest waits d for the text typed in the search box to rest, reporting
// whether it did: it does not once the text changes, which closes stop, or
// the request is over.
func rest(ctx context.Context, stop <-chan struct{}, d time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-stop:
		return false
	case <-time.After(d):
	}
	// Both may be ready at once; the text is what the wait is for.
	select {
	case <-stop:
		return false
	default:
		return true
	}
}

// rememberSearch puts a search at the top of the history the box shows.
func (a *app) rememberSearch(q string) {
	q = strings.TrimSpace(q)
	if q == "" {
		return
	}
	a.set.Searches = pushFront(a.set.Searches, q, searchHistoryMax, strings.EqualFold)
	a.saveSettings()
}

// clearSearches forgets the history the box shows.
func (a *app) clearSearches() {
	a.set.Searches = nil
	a.saveSettings()
}

// showSearchTab switches the search page to a tab, which loads.
func (a *app) showSearchTab(tab searchTab) {
	a.search.tab = tab
	a.loadSearchTab()
}

// loadSearchTab loads the first page of the singers or the playlists
// found, as their tab shows, if it has none: not every search wants them,
// and the service turns away searches that come together (code 2001).
func (a *app) loadSearchTab() {
	s := &a.search
	switch {
	case s.tab == foundSingers && s.singers.pages == 0 && !s.singers.loading:
		a.searchSingers()
	case s.tab == foundPlaylists && s.pls.pages == 0 && !s.pls.loading:
		a.searchPlaylists()
	case s.tab == foundPodcasts:
		a.loadSearchPodcasts()
	}
}

// loadSearchPodcasts loads the first page of the podcasts found, of QQ
// Music and, when logged in, of 小宇宙 beside it, unless either has, or is
// loading it.
func (a *app) loadSearchPodcasts() {
	if a.search.query == "" {
		return
	}
	if f := &a.search.podcasts; f.pages == 0 && !f.loading {
		a.morePodcastsFound()
	}
	if f := &a.search.xiaoyuzhouShows; a.xiaoyuzhou.api.LoggedIn() && f.pages == 0 && !f.loading {
		a.moreXiaoyuzhouPodcastsFound()
	}
}

// moreXiaoyuzhouPodcastsFound loads the next page of the podcasts of 小宇宙 the
// 播客 tab of a search found, beside those of QQ Music.
func (a *app) moreXiaoyuzhouPodcastsFound() {
	a.moreXiaoyuzhouShows(&a.search.xiaoyuzhouShows, a.search.query)
}

// moreFoundPodcasts loads the next page of the podcasts found, of QQ Music
// and of 小宇宙, of each that has one.
func (a *app) moreFoundPodcasts() {
	if a.search.podcasts.more {
		a.morePodcastsFound()
	}
	if a.search.xiaoyuzhouShows.more {
		a.moreXiaoyuzhouPodcastsFound()
	}
}

// foundPage is how many singers or playlists a page of them has.
const foundPage = 30

// searchMore loads the next page of what f found for the query, adding
// what add finds new: pages may be short, or overlap. There are as many
// as the service says, until a page is empty.
func searchMore[T any](a *app, f *pager[T], load func(ctx context.Context, q string, pg int) ([]T, int, error), add func(dst, src []T) []T) {
	q := a.search.query
	loadPage(a, f, errText, func(ctx context.Context, pg int, _ xiaoyuzhou.Cursor) (func(), error) {
		items, total, err := load(ctx, q, pg)
		return func() {
			f.items = add(f.items, items)
			f.total = total
			if len(items) == 0 {
				f.total = len(f.items)
			}
			f.more = len(f.items) < f.total
		}, err
	})
}

// searchSingers loads the next page of the singers found.
func (a *app) searchSingers() {
	searchMore(a, &a.search.singers, func(ctx context.Context, q string, pg int) ([]qqmusic.FoundSinger, int, error) {
		return a.api.SearchSingers(ctx, q, pg, foundPage)
	}, addSingers)
}

// searchPlaylists loads the next page of the playlists found.
func (a *app) searchPlaylists() {
	searchMore(a, &a.search.pls, func(ctx context.Context, q string, pg int) ([]qqmusic.Playlist, int, error) {
		return a.api.SearchPlaylists(ctx, q, pg, foundPage)
	}, addPlaylists)
}

// addSingers appends to dst the singers of src it does not have yet.
func addSingers(dst, src []qqmusic.FoundSinger) []qqmusic.FoundSinger {
	seen := make(map[string]bool, len(dst))
	for _, it := range dst {
		seen[it.Mid] = true
	}
	for _, it := range src {
		if !seen[it.Mid] {
			seen[it.Mid] = true
			dst = append(dst, it)
		}
	}
	return dst
}
