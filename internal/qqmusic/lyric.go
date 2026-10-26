package qqmusic

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// LyricLine is a timed line of lyrics with its translation.
type LyricLine struct {
	At    time.Duration
	Text  string
	Trans string
}

var lrcTime = regexp.MustCompile(`\[(\d+):(\d+)(?:[.:](\d+))?\]`)

func parseLRCLines(lrc string) []LyricLine {
	var lines []LyricLine
	for _, raw := range strings.Split(strings.ReplaceAll(lrc, "\r", ""), "\n") {
		stamps := lrcTime.FindAllStringSubmatchIndex(raw, -1)
		if len(stamps) == 0 || stamps[0][0] != 0 {
			continue
		}
		text := strings.TrimSpace(raw[stamps[len(stamps)-1][1]:])
		for _, m := range stamps {
			min, _ := strconv.Atoi(raw[m[2]:m[3]])
			sec, _ := strconv.Atoi(raw[m[4]:m[5]])
			var frac time.Duration
			if m[6] >= 0 {
				f := raw[m[6]:m[7]]
				n, _ := strconv.Atoi(f)
				switch len(f) {
				case 1:
					frac = time.Duration(n) * 100 * time.Millisecond
				case 2:
					frac = time.Duration(n) * 10 * time.Millisecond
				default:
					frac = time.Duration(n) * time.Millisecond
				}
			}
			lines = append(lines, LyricLine{At: time.Duration(min)*time.Minute + time.Duration(sec)*time.Second + frac, Text: text})
		}
	}
	sort.SliceStable(lines, func(i, j int) bool { return lines[i].At < lines[j].At })
	return lines
}

// ParseLRC parses LRC lyrics and pairs their lines with those of a
// translation of the same times. Lines without text and the "//" QQ
// Music puts in translations are dropped. Lyrics without time stamps
// become untimed lines.
func ParseLRC(lrc, trans string) []LyricLine {
	lines := parseLRCLines(lrc)
	if len(lines) == 0 && strings.TrimSpace(lrc) != "" {
		for _, l := range strings.Split(strings.ReplaceAll(lrc, "\r", ""), "\n") {
			if l = strings.TrimSpace(l); l != "" {
				lines = append(lines, LyricLine{At: -1, Text: l})
			}
		}
		return lines
	}
	tr := map[time.Duration]string{}
	for _, l := range parseLRCLines(trans) {
		if l.Text != "" && l.Text != "//" {
			tr[l.At] = l.Text
		}
	}
	out := lines[:0]
	for _, l := range lines {
		if l.Text == "" {
			continue
		}
		l.Trans = tr[l.At]
		out = append(out, l)
	}
	return out
}

// LineAt returns the index of the line playing at t, or -1 before the first.
func LineAt(lines []LyricLine, t time.Duration) int {
	i := sort.Search(len(lines), func(i int) bool { return lines[i].At > t })
	return i - 1
}
