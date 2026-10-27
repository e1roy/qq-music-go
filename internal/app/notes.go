package app

import (
	"context"
	"errors"
	"html"
	"regexp"
	"strings"

	"github.com/egoist/mygo/ui"

	"qqmusic/internal/qqmusic"
	"qqmusic/internal/xiaoyuzhou"
)

// episodeNotes is the description of the 小宇宙 episode playing, as the
// page of the song shows it in place of the lyrics. It is loaded for the
// episode in the queue, and dropped when another one plays.
type episodeNotes struct {
	eid        string
	paragraphs []string
	comments   int // the episode's, for the comments tab
	scroll     ui.ListState
	loaded     bool
	loading    bool
	err        string
	gen        int
}

// syncNotes loads the notes of the episode playing, unless they are its
// own already, or are on the way.
func (a *app) syncNotes() { a.loadNotes(false) }

// retryNotes loads the notes again after a failure.
func (a *app) retryNotes() { a.loadNotes(true) }

func (a *app) loadNotes(force bool) {
	s, ok := a.nowPlaying()
	if !ok || !isXiaoyuzhou(s) || s.Mid == "" {
		return
	}
	n := &a.notes
	if n.eid == s.Mid && !force && (n.loading || n.loaded || n.err != "") {
		return
	}
	n.eid, n.paragraphs, n.comments, n.loading, n.loaded, n.err = s.Mid, nil, 0, true, false, ""
	n.gen++
	n.scroll.ScrollTo(0, ui.Start)
	gen, eid := n.gen, s.Mid
	a.do(func(ctx context.Context) func() {
		e, err := a.episodeDetail(ctx, eid)
		var paragraphs []string
		if err == nil {
			paragraphs = episodeParagraphs(e)
		}
		return func() {
			if gen != a.notes.gen || a.notes.eid != eid {
				return
			}
			a.notes.loading = false
			if err != nil {
				a.notes.err = errText(err)
				return
			}
			a.notes.paragraphs, a.notes.loaded = paragraphs, true
			if e.CommentCount > 0 {
				a.notes.comments = int(e.CommentCount)
				if a.comments.eid == eid {
					a.comments.total = a.notes.comments
				}
			}
		}
	})
}

// episodeDetail asks the API for an episode, and its web page when there
// is no login: the page has the shownotes too.
func (a *app) episodeDetail(ctx context.Context, eid string) (xiaoyuzhou.Episode, error) {
	x := &a.xiaoyuzhou
	if x.api.LoggedIn() {
		e, err := x.api.Episode(ctx, eid)
		if err == nil {
			return e, nil
		}
		if !errors.Is(err, xiaoyuzhou.ErrNotLoggedIn) {
			return xiaoyuzhou.Episode{}, err
		}
	}
	return x.api.WebEpisode(ctx, eid)
}

// episodeParagraphs is what the page shows of an episode: its shownotes,
// the description when it has none, split into paragraphs.
func episodeParagraphs(e xiaoyuzhou.Episode) []string {
	text := cleanShownotes(e.Shownotes)
	if text == "" {
		text = cleanShownotes(e.Description)
	}
	var paragraphs []string
	for _, p := range strings.Split(text, "\n\n") {
		if p = strings.TrimSpace(p); p != "" {
			paragraphs = append(paragraphs, p)
		}
	}
	return paragraphs
}

var (
	notesScript   = regexp.MustCompile(`(?is)<(script|style)\b.*?</(script|style)>`)
	notesBreak    = regexp.MustCompile(`(?i)<br\s*/?>`)
	notesItem     = regexp.MustCompile(`(?i)<li\b[^>]*>`)
	notesBlockEnd = regexp.MustCompile(`(?i)</(p|div|h[1-6]|ul|ol|blockquote|section|tr|td|table)\s*>`)
	notesTag      = regexp.MustCompile(`(?s)<[^>]*>`)
	notesSpaces   = regexp.MustCompile(`[ \t\f\v\x{00a0}]+`)
	notesBlanks   = regexp.MustCompile(`\n{3,}`)
)

// cleanShownotes turns the HTML of an episode's shownotes into text, with
// a blank line between blocks and each item of a list on a line of its own.
func cleanShownotes(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = notesScript.ReplaceAllString(s, "")
	s = notesBreak.ReplaceAllString(s, "\n")
	s = notesItem.ReplaceAllString(s, "\n· ")
	s = notesBlockEnd.ReplaceAllString(s, "\n\n")
	s = notesTag.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	s = notesSpaces.ReplaceAllString(s, " ")
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = strings.TrimSpace(lines[i])
	}
	return strings.TrimSpace(notesBlanks.ReplaceAllString(strings.Join(lines, "\n"), "\n\n"))
}

// episodeComment is a comment of 小宇宙 as one of QQ Music's, which the
// panel beside the lyrics draws.
func episodeComment(c xiaoyuzhou.Comment) qqmusic.Comment {
	out := qqmusic.Comment{
		ID:       c.ID,
		Nick:     c.Author.Nickname,
		Avatar:   c.Author.Avatar.Picture.URL(64),
		Content:  strings.TrimSpace(c.Text),
		Time:     c.CreatedAt,
		Location: c.IPLoc,
		Likes:    c.LikeCount,
		Replies:  int(c.ReplyCount),
	}
	if len(c.Replies) > 0 {
		r := c.Replies[0]
		out.Reply = &qqmusic.CommentReply{Nick: r.Author.Nickname, Content: strings.TrimSpace(r.Text)}
	}
	return out
}

// episodeCommentOrder is the order of the comments panel for 小宇宙's.
func episodeCommentOrder(order qqmusic.CommentOrder) xiaoyuzhou.CommentOrder {
	if order == qqmusic.LatestComments {
		return xiaoyuzhou.LatestComments
	}
	return xiaoyuzhou.HotComments
}
