package genreengine

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// aggregateTagScores combines ranked tag lists into a single score map.
// input: list of tag slices in ranked order (best first). higher rank -> higher score.
func aggregateTagScores(ctx context.Context, lists ...[]string) map[string]float64 {
	scores := make(map[string]float64)
	for _, l := range lists {
		w := float64(len(l))
		for _, tag := range l {
			tag = strings.ToLower(strings.TrimSpace(tag))
			if tag == "" {
				continue
			}
			scores[tag] += w
			w -= 1
			if w < 0 {
				w = 0
			}
		}
	}
	// Apply enhancement to redistribute descriptive tags to genres
	return enhanceGenreScores(ctx, scores)
}

// enhanceGenreScores redistributes scores from descriptive tags (instruments, tempo, mood)
// to their most likely genre counterparts. For example, if "loud" and "rock" both appear,
// rock inherits the loud score since loud rock music is common.
func enhanceGenreScores(ctx context.Context, scores map[string]float64) map[string]float64 {
	// Define enhancement rules: descriptor -> list of genres it should enhance
	enhancements := map[string][]string{
		// Tempo/Energy descriptors
		"loud":  {"rock", "metal", "heavy metal", "hard rock", "punk", "techno", "electronic"},
		"fast":  {"rock", "metal", "heavy metal", "punk", "techno", "electronic", "dance", "drum and bass"},
		"slow":  {"ambient", "classical", "jazz", "blues", "soul", "chillout", "new age"},
		"soft":  {"ambient", "classical", "jazz", "acoustic", "folk", "new age", "chillout"},
		"quiet": {"ambient", "classical", "jazz", "acoustic", "folk", "new age"},

		// Instrumentation -> Genre associations
		"guitar":     {"rock", "metal", "country", "folk", "blues", "alternative", "indie", "punk", "hard rock"},
		"drums":      {"rock", "metal", "punk", "jazz", "electronic", "techno", "dance", "hip-hop"},
		"bass":       {"rock", "metal", "electronic", "hip-hop", "funk", "jazz"},
		"synth":      {"electronic", "techno", "ambient", "new age", "electronica", "house", "dance", "pop"},
		"electronic": {"techno", "house", "ambient", "electronica", "dance", "electro"},
		"beat":       {"electronic", "hip-hop", "dance", "techno", "house", "pop"},
		"beats":      {"electronic", "hip-hop", "dance", "techno", "house", "pop"},

		// Classical instruments
		"violin":      {"classical", "folk", "country"},
		"strings":     {"classical", "ambient", "new age"},
		"piano":       {"classical", "jazz", "ambient", "new age", "pop"},
		"cello":       {"classical", "ambient", "new age"},
		"harpsichord": {"classical"},
		"harp":        {"classical", "new age", "ambient"},
		"flute":       {"classical", "jazz", "new age"},
		"choir":       {"classical", "choral", "opera"},
		"choral":      {"classical", "opera"},
		"opera":       {"classical"},

		// World/Regional
		"indian": {"indian"},
		"sitar":  {"indian"},

		// Vocal descriptors
		"vocal":            {"pop", "rock", "soul", "rnb", "hip-hop", "country"},
		"vocals":           {"pop", "rock", "soul", "rnb", "hip-hop", "country"},
		"singing":          {"pop", "rock", "soul", "rnb", "opera", "country"},
		"voice":            {"pop", "rock", "soul", "rnb", "hip-hop"},
		"male vocal":       {"rock", "country", "soul", "hip-hop"},
		"female vocal":     {"pop", "soul", "rnb"},
		"male vocalists":   {"rock", "country", "soul", "hip-hop"},
		"female vocalists": {"pop", "soul", "rnb"},

		// Mood/Style descriptors
		"experimental": {"electronic", "ambient", "jazz", "indie", "alternative"},
		"instrumental": {"jazz", "classical", "ambient", "electronic"},
		"acoustic":     {"folk", "country", "acoustic", "indie"},
		"beautiful":    {"classical", "ambient", "new age", "pop"},
		"sexy":         {"rnb", "soul", "pop"},
		"catchy":       {"pop", "dance", "indie pop"},
		"happy":        {"pop", "dance", "indie pop"},
		"sad":          {"blues", "soul", "indie", "alternative"},
		"chill":        {"chillout", "ambient", "electronic", "jazz"},
		"chillout":     {"ambient", "electronic", "jazz"},
		"mellow":       {"jazz", "soul", "acoustic", "indie"},
		"party":        {"dance", "house", "hip-hop", "pop", "electronic"},
		"dance":        {"house", "electronic", "techno", "pop"},
		"weird":        {"experimental", "electronic", "indie"},

		// Era descriptors
		"60s":    {"rock", "soul", "folk", "blues"},
		"70s":    {"rock", "disco", "funk", "soul"},
		"80s":    {"pop", "rock", "electronic", "new wave"},
		"90s":    {"rock", "alternative", "hip-hop", "electronic"},
		"00s":    {"indie", "alternative", "hip-hop", "electronic"},
		"oldies": {"rock", "soul", "blues"},

		// Sub-genre consolidation
		"classic":          {"classical"},
		"classic rock":     {"rock"},
		"hard rock":        {"rock", "metal"},
		"alternative":      {"rock", "indie"},
		"alternative rock": {"rock"},
		"indie rock":       {"rock", "indie"},
		"progressive rock": {"rock"},
		"indie pop":        {"pop", "indie"},
		"heavy metal":      {"metal"},
		"electronica":      {"electronic"},
		"electro":          {"electronic"},
		"easy listening":   {"pop", "jazz"},
	}

	enhanced := make(map[string]float64)

	// First, copy all scores to enhanced
	for k, v := range scores {
		enhanced[k] = v
	}

	// Apply enhancement rules
	var enhancements_applied []string
	for descriptor, genreList := range enhancements {
		descriptorScore, hasDescriptor := scores[descriptor]
		if !hasDescriptor || descriptorScore <= 0 {
			continue
		}

		// Find which genres from this descriptor's list are present in scores
		var matchedGenres []string
		for _, genre := range genreList {
			if _, hasGenre := scores[genre]; hasGenre {
				matchedGenres = append(matchedGenres, genre)
			}
		}

		if len(matchedGenres) > 0 {
			// Distribute descriptor score to matched genres
			boost := descriptorScore / float64(len(matchedGenres))
			for _, genre := range matchedGenres {
				enhanced[genre] += boost
			}

			enhancements_applied = append(enhancements_applied,
				fmt.Sprintf("%s(%.1f)→%s", descriptor, descriptorScore, strings.Join(matchedGenres, "+")))

			// Remove the descriptor score if it's not itself a primary genre
			if !isPrimaryGenre(descriptor) {
				enhanced[descriptor] = 0
			}
		}
	}

	// Remove zero/negative scores
	cleaned := make(map[string]float64)
	for k, v := range enhanced {
		if v > 0 {
			cleaned[k] = v
		}
	}

	return cleaned
}

// isPrimaryGenre returns true if the tag is a primary genre we want to keep
func isPrimaryGenre(tag string) bool {
	primaryGenres := map[string]bool{
		"rock": true, "pop": true, "electronic": true, "metal": true, "jazz": true,
		"classical": true, "hip-hop": true, "country": true, "folk": true, "blues": true,
		"soul": true, "rnb": true, "punk": true, "reggae": true, "latin": true,
		"world": true, "ambient": true, "house": true, "techno": true, "dance": true,
		"indie": true, "alternative": true, "funk": true, "disco": true, "new age": true,
		"chillout": true, "acoustic": true, "opera": true, "experimental": true,
		"drum and bass": true, "dubstep": true, "trance": true, "indian": true,
	}
	return primaryGenres[strings.ToLower(strings.TrimSpace(tag))]
}

// topTagsFromProbs converts probabilities per label to the topN tag names.
func topTagsFromProbs(labels []string, probs []float32, topN int) []string {
	idx := make([]int, len(labels))
	for i := range idx {
		idx[i] = i
	}
	sort.Slice(idx, func(i, j int) bool { return probs[idx[i]] > probs[idx[j]] })
	n := topN
	if n > len(idx) {
		n = len(idx)
	}
	res := make([]string, 0, n)
	for k := 0; k < n; k++ {
		res = append(res, labels[idx[k]])
	}
	return res
}

// consolidateScores folds synonym tags into a canonical target to avoid diluting votes.
// - Rock family: {classic rock, hard rock, alternative rock, indie rock, progressive rock} -> rock
// - Electronic near-synonyms: {electronica, electro} -> electronic
// Distinct genres like techno/house/dance remain separate.
func consolidateScores(scores map[string]float64) map[string]float64 {
	aliases := map[string][]string{
		"rock":       {"classic rock", "hard rock", "alternative rock", "indie rock", "progressive rock"},
		"electronic": {"electronica", "electro"},
	}
	out := make(map[string]float64)
	for k, v := range scores {
		kk := strings.ToLower(strings.TrimSpace(k))
		mapped := false
		for canon, list := range aliases {
			if kk == canon {
				out[canon] += v
				mapped = true
				break
			}
			for _, a := range list {
				if kk == a {
					out[canon] += v
					mapped = true
					break
				}
			}
			if mapped {
				break
			}
		}
		if !mapped {
			out[kk] += v
		}
	}
	return out
}

// pickGenre chooses a single genre from scored tags by selecting the highest-scoring
// tag that exists in the allowed (preferred) whitelist. If none are allowed, fall back
// to the configured default genre.
func pickGenre(scores map[string]float64, defaultGenre string) string {
	if len(scores) == 0 {
		return titleCase(defaultGenre)
	}
	// Consolidate synonyms to avoid splitting votes across near-duplicates
	scores = consolidateScores(scores)
	// Build allowed set and preferred index map (for tie-breaking)
	allowed := make(map[string]struct{}, len(preferred))
	prefIdx := make(map[string]int, len(preferred))
	for i, g := range preferred {
		allowed[g] = struct{}{}
		prefIdx[g] = i
	}
	// Collect items and sort by score desc; tie-break by preferred order when both are allowed
	type kv struct {
		k string
		v float64
	}
	items := make([]kv, 0, len(scores))
	for k, v := range scores {
		kk := strings.ToLower(strings.TrimSpace(k))
		items = append(items, kv{k: kk, v: v})
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].v == items[j].v {
			ii, iok := prefIdx[items[i].k]
			jj, jok := prefIdx[items[j].k]
			if iok && jok {
				return ii < jj
			}
		}
		return items[i].v > items[j].v
	})
	// If electronic narrowly beats rock, prefer rock when within a small margin
	if e, eok := scores["electronic"]; eok {
		if r, rok := scores["rock"]; rok {
			if r >= 0.9*e { // within 10%
				return titleCase("rock")
			}
		}
	}
	// Pick the highest-scoring allowed tag
	for _, it := range items {
		if _, ok := allowed[it.k]; ok {
			return titleCase(it.k)
		}
	}
	// No allowed tag present: fall back to default.
	return titleCase(defaultGenre)
}
