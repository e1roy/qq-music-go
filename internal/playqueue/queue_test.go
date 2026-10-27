package playqueue

import (
	"slices"
	"strings"
	"testing"

	"qqmusic/internal/qqmusic"
)

func songs(mids string) []qqmusic.Song {
	var out []qqmusic.Song
	for _, m := range strings.Fields(mids) {
		out = append(out, qqmusic.Song{Mid: m})
	}
	return out
}

func (q *Queue) mids() string {
	var out []string
	for _, s := range q.songs {
		out = append(out, s.Mid)
	}
	return strings.Join(out, " ")
}

func (q *Queue) check(t *testing.T, step, mids string, cur int) {
	t.Helper()
	if q.mids() != mids || q.Cur() != cur {
		t.Fatalf("%s: %q at %d, want %q at %d", step, q.mids(), q.Cur(), mids, cur)
	}
	sorted := slices.Sorted(slices.Values(q.order))
	for i, v := range sorted {
		if v != i {
			t.Fatalf("%s: order %v is not a permutation of %d", step, q.order, len(q.songs))
		}
	}
	if len(q.order) != len(q.songs) {
		t.Fatalf("%s: order of %d for %d songs", step, len(q.order), len(q.songs))
	}
}

func TestEdits(t *testing.T) {
	q := Restore(songs("a b c d"), 1, false)
	q.check(t, "restore", "a b c d", 1)
	q.InsertNext(qqmusic.Song{Mid: "x"})
	q.check(t, "insert next", "a b x c d", 1)
	q.Append(qqmusic.Song{Mid: "y"})
	q.check(t, "append", "a b x c d y", 1)
	if q.Remove(0) {
		t.Fatal("removing a song before reported the current")
	}
	q.check(t, "remove before", "b x c d y", 0)
	q.Remove(2)
	q.check(t, "remove after", "b x d y", 0)
	if !q.Remove(0) {
		t.Fatal("removing the current did not say so")
	}
	q.check(t, "remove the current", "x d y", 0)
	q.Remove(-1)
	q.Remove(3)
	q.check(t, "remove out of range", "x d y", 0)
	q.Select(2)
	q.Remove(2)
	q.check(t, "remove the current last", "x d", 1)
	q.Remove(0)
	q.Remove(0)
	q.check(t, "remove all", "", -1)

	q = Restore(songs("a b"), 0, true)
	q.AppendMissing(songs("b c a d"))
	q.check(t, "append missing", "a b c d", 0)
	if !q.Radio() {
		t.Fatal("append missing ended the radio")
	}
	q.Replace(songs("e f"))
	q.check(t, "replace", "e f", -1)
	if q.Radio() {
		t.Fatal("replace kept the radio")
	}
	if q.Select(2) || !q.Select(1) {
		t.Fatal("select")
	}
	q.Clear()
	q.check(t, "clear", "", -1)
	if _, ok := q.Current(); ok {
		t.Fatal("an empty queue has a song")
	}
}

// TestZero uses a queue never made: it is empty, and takes songs.
func TestZero(t *testing.T) {
	var q Queue
	q.check(t, "zero", "", -1)
	if _, ok := q.Current(); ok {
		t.Fatal("the zero queue has a song")
	}
	q.InsertNext(qqmusic.Song{Mid: "a"})
	q.check(t, "insert next into zero", "a", -1)
	if step, i := q.Next(Loop, false); step != Play || i != 0 {
		t.Fatalf("next from none: %v %d", step, i)
	}
}

// TestOwnSongs: a queue changes its own copy, never the list it came from.
func TestOwnSongs(t *testing.T) {
	// Room to grow in place, as a list that was appended to has.
	list := slices.Grow(songs("a b c"), 4)
	q := Restore(list, 0, false)
	q.InsertNext(qqmusic.Song{Mid: "x"})
	q.Remove(0)
	var r Queue
	r.Replace(list)
	r.InsertNext(qqmusic.Song{Mid: "y"})
	r.Remove(1)
	if got := (&Queue{songs: list}).mids(); got != "a b c" {
		t.Fatalf("the list changed: %q", got)
	}
}

func TestNearEnd(t *testing.T) {
	q := Restore(songs("a b c d"), 1, true)
	if q.NearEnd() {
		t.Fatal("second of four is near the end")
	}
	q.Select(2)
	if !q.NearEnd() {
		t.Fatal("third of four is not near the end")
	}
	q.SetRadio(false)
	if q.NearEnd() {
		t.Fatal("a queue not the radio's is near its end")
	}
}

func TestNext(t *testing.T) {
	for _, tc := range []struct {
		mode Mode
		cur  int
		auto bool
		step Step
		want int
	}{
		{Loop, 0, false, Play, 1},
		{Loop, 2, true, Play, 0},
		{Loop, 2, false, Play, 0},
		{One, 1, true, Replay, 1},
		{One, 1, false, Play, 2},
		{One, 2, false, Play, 0},
		{Order, 0, true, Play, 1},
		{Order, 2, true, Stop, -1},
		{Order, 2, false, Play, 0},
	} {
		q := Restore(songs("a b c"), tc.cur, false)
		if step, i := q.Next(tc.mode, tc.auto); step != tc.step || i != tc.want {
			t.Errorf("%v from %d (auto %v): %v %d, want %v %d", tc.mode, tc.cur, tc.auto, step, i, tc.step, tc.want)
		}
	}
	for _, tc := range []struct {
		mode      Mode
		cur, want int
	}{{Loop, 0, 2}, {Loop, 2, 1}, {One, 0, 2}, {Order, 1, 0}} {
		q := Restore(songs("a b c"), tc.cur, false)
		if i := q.Prev(tc.mode); i != tc.want {
			t.Errorf("prev %v from %d: %d, want %d", tc.mode, tc.cur, i, tc.want)
		}
	}
	var empty Queue
	if step, _ := empty.Next(Loop, true); step != None || empty.Prev(Loop) != -1 {
		t.Error("an empty queue goes on")
	}
}

func TestShuffle(t *testing.T) {
	q := Restore(songs("a b c d"), 0, false)
	q.order = []int{2, 0, 3, 1}
	for _, tc := range []struct{ cur, next, prev int }{{0, 3, 2}, {1, 2, 3}, {2, 0, 1}, {3, 1, 0}} {
		q.at = tc.cur + 1
		if _, i := q.Next(Shuffle, true); i != tc.next {
			t.Errorf("next from %d: %d, want %d", tc.cur, i, tc.next)
		}
		if i := q.Prev(Shuffle); i != tc.prev {
			t.Errorf("prev from %d: %d, want %d", tc.cur, i, tc.prev)
		}
	}
	// None playing, it starts at the first of the order.
	q.at = 0
	if _, i := q.Next(Shuffle, false); i != 2 {
		t.Errorf("next from none: %d", i)
	}
}
