package genreengine

import (
	"context"
	"path/filepath"
	"regexp"
	"strings"
)

var wordSplit = regexp.MustCompile(`[\s._\-\[\]\(\)/]+`)

var preferred = []string{"classical", "techno", "strings", "drums", "electronic", "rock", "piano", "ambient", "violin", "vocal", "synth", "indian", "opera", "harpsichord", "flute", "pop", "sitar", "classic", "choir", "new age", "dance", "harp", "cello", "country", "metal", "choral", "alternative", "indie", "00s", "alternative rock", "jazz", "chillout", "classic rock", "soul", "indie rock", "mellow", "electronica", "80s", "folk", "90s", "chill", "instrumental", "punk", "oldies", "blues", "hard rock", "acoustic", "experimental", "hip-hop", "70s", "party", "easy listening", "funk", "electro", "heavy metal", "progressive rock", "60s", "rnb", "indie pop", "sad", "house"}

// HeuristicEngine is a temporary fallback that guesses from path components.
type HeuristicEngine struct {
	opt Options
}

func NewHeuristicEngine(opt Options) *HeuristicEngine {
	return &HeuristicEngine{opt: opt}
}

func (e *HeuristicEngine) Classify(ctx context.Context, filePath string, topN int) (Result, error) {
	g := guessFromPath(filePath)
	/*if g == "" {
		g = e.opt.DefaultGenre
	}*/
	return Result{Genre: titleCase(g)}, nil
}

func guessFromPath(path string) string {
	base := strings.ToLower(filepath.Base(path))
	dir := strings.ToLower(filepath.Base(filepath.Dir(path)))
	candidates := append(wordSplit.Split(base, -1), wordSplit.Split(dir, -1)...)
	seen := map[string]struct{}{}
	for _, c := range candidates {
		if c == "" {
			continue
		}
		seen[c] = struct{}{}
	}
	for _, p := range preferred {
		if _, ok := seen[p]; ok {
			return p
		}
	}
	for _, p := range preferred {
		pp := strings.ReplaceAll(p, "-", "")
		for w := range seen {
			ww := strings.ReplaceAll(w, "-", "")
			if strings.Contains(ww, pp) || strings.Contains(pp, ww) {
				return p
			}
		}
	}
	return ""
}

func titleCase(s string) string {
	return strings.Title(strings.ToLower(s))
}
