package genreengine

import (
	"sort"
	"strings"
)

// aggregateTagScores combines ranked tag lists into a single score map.
// input: list of tag slices in ranked order (best first). higher rank -> higher score.
func aggregateTagScores(lists ...[]string) map[string]float64 {
	scores := make(map[string]float64)
	for _, l := range lists {
		w := float64(len(l))
		for _, tag := range l {
			tag = strings.ToLower(strings.TrimSpace(tag))
			if tag == "" { continue }
			scores[tag] += w
			w -= 1
			if w < 0 { w = 0 }
		}
	}
	return scores
}

// topTagsFromProbs converts probabilities per label to the topN tag names.
func topTagsFromProbs(labels []string, probs []float32, topN int) []string {
	idx := make([]int, len(labels))
	for i := range idx { idx[i] = i }
	sort.Slice(idx, func(i, j int) bool { return probs[idx[i]] > probs[idx[j]] })
	n := topN
	if n > len(idx) { n = len(idx) }
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
			if mapped { break }
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
	type kv struct{ k string; v float64 }
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
