// Package playqueue is the queue the player goes through: its songs, the
// one playing, the order of shuffle, and where next and previous lead in
// each play mode. It does no IO: the app plays and saves what it says.
package playqueue

import (
	"math/rand/v2"
	"slices"

	"qqmusic/internal/qqmusic"
)

// Mode is how the queue goes on. Its numbers are those settings.json
// keeps.
type Mode int

const (
	Loop Mode = iota
	One
	Shuffle
	Order
)

func (m Mode) String() string {
	return [...]string{"列表循环", "单曲循环", "随机播放", "顺序播放"}[m]
}

// Step is what the player does next.
type Step int

const (
	None   Step = iota // the queue is empty
	Play               // play the song at the index given
	Replay             // play the song playing again, from its start
	Stop               // the queue is over
)

// Queue is the songs to play and the one playing; radio says it is
// 猜你喜欢's, which grows as it nears its end. at is the index of the song
// playing plus one, 0 for none, for the zero Queue to be an empty one.
// order is the order of Shuffle, a permutation of the songs: every change
// of the songs makes another.
type Queue struct {
	songs []qqmusic.Song
	at    int
	radio bool
	order []int
}

// Restore is a queue of songs, copied, at song cur of them, which must be
// in range: the queue a run left.
func Restore(songs []qqmusic.Song, cur int, radio bool) Queue {
	q := Queue{songs: slices.Clone(songs), at: cur + 1, radio: radio}
	q.reshuffle()
	return q
}

// Songs are the songs of the queue, in order; they are the queue's own,
// not to be changed.
func (q *Queue) Songs() []qqmusic.Song { return q.songs }

// Cur is the index of the song playing, -1 for none.
func (q *Queue) Cur() int { return q.at - 1 }

// Current is the song playing, if there is one.
func (q *Queue) Current() (qqmusic.Song, bool) {
	if q.at == 0 {
		return qqmusic.Song{}, false
	}
	return q.songs[q.at-1], true
}

func (q *Queue) Radio() bool { return q.radio }

func (q *Queue) SetRadio(on bool) { q.radio = on }

// Replace makes songs, copied, the queue, no song of it playing yet, and
// not the radio's.
func (q *Queue) Replace(songs []qqmusic.Song) {
	*q = Queue{songs: slices.Clone(songs)}
	q.reshuffle()
}

// Select makes song i the one playing, reporting whether there is one.
func (q *Queue) Select(i int) bool {
	if i < 0 || i >= len(q.songs) {
		return false
	}
	q.at = i + 1
	return true
}

// InsertNext puts s after the song playing, or first when none is.
func (q *Queue) InsertNext(s qqmusic.Song) {
	q.songs = slices.Insert(q.songs, q.at, s)
	q.reshuffle()
}

// Append puts s at the end.
func (q *Queue) Append(s qqmusic.Song) {
	q.songs = append(q.songs, s)
	q.reshuffle()
}

// AppendMissing puts at the end the songs it does not have yet, by mid:
// the radio's next songs may repeat some it gave.
func (q *Queue) AppendMissing(songs []qqmusic.Song) {
	for _, s := range songs {
		if !slices.ContainsFunc(q.songs, func(o qqmusic.Song) bool { return o.Mid == s.Mid }) {
			q.songs = append(q.songs, s)
		}
	}
	q.reshuffle()
}

// Remove takes song i out, reporting whether it was the one playing: the
// one after it, or the last, is then. The songs before it move up. An i
// out of range removes nothing.
func (q *Queue) Remove(i int) (wasCurrent bool) {
	if i < 0 || i >= len(q.songs) {
		return false
	}
	cur := q.Cur()
	q.songs = slices.Delete(q.songs, i, i+1)
	switch {
	case len(q.songs) == 0:
		q.at = 0
	case i < cur:
		q.at--
	case i == cur:
		q.at = min(i, len(q.songs)-1) + 1
		wasCurrent = true
	}
	q.reshuffle()
	return wasCurrent
}

// Clear empties the queue, which is no longer the radio's.
func (q *Queue) Clear() { *q = Queue{} }

// NearEnd reports whether the radio should get more songs: the one
// playing is among its last two.
func (q *Queue) NearEnd() bool { return q.radio && q.Cur() >= len(q.songs)-2 }

// Next is what follows the song playing in mode m: auto when it ended or
// failed, rather than being skipped. Played to the end, one song plays
// again in One, and the queue stops in Order; skipped, it goes on.
func (q *Queue) Next(m Mode, auto bool) (Step, int) {
	n := len(q.songs)
	switch {
	case n == 0:
		return None, -1
	case auto && m == One:
		return Replay, q.Cur()
	case m == Shuffle:
		return Play, q.shuffleStep(1)
	}
	i := q.at
	if i >= n {
		if auto && m == Order {
			return Stop, -1
		}
		i = 0
	}
	return Play, i
}

// Prev is the song before the one playing in mode m, -1 in an empty
// queue.
func (q *Queue) Prev(m Mode) int {
	n := len(q.songs)
	switch {
	case n == 0:
		return -1
	case m == Shuffle:
		return q.shuffleStep(-1)
	}
	return (q.Cur() - 1 + n) % n
}

func (q *Queue) shuffleStep(d int) int {
	at := slices.Index(q.order, q.Cur())
	n := len(q.order)
	return q.order[((at+d)%n+n)%n]
}

func (q *Queue) reshuffle() { q.order = rand.Perm(len(q.songs)) }
