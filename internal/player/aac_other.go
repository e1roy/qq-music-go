//go:build !windows || !(amd64 || arm64)

package player

import (
	"errors"
	"time"

	"github.com/egoist/mygo/audio"
)

// newAACDecoder fails: AAC decodes with Windows' Media Foundation only.
func newAACDecoder(b *buffer, duration time.Duration) (audio.Decoder, error) {
	return nil, errors.New("player: 此系统不支持播放 AAC")
}
