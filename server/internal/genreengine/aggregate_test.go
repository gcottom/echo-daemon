package genreengine

import (
	"context"
	"testing"
)

func TestDescriptorVotesCannotOverruleClearDirectEvidence(t *testing.T) {
	raw := map[string]float64{"rock": 8, "electronic": 4, "synth": 60, "beat": 60, "drums": 60, "bass": 60}
	scored := enhanceGenreScores(context.Background(), raw)
	if scored["electronic"] > 5 {
		t.Fatalf("descriptor-only boost exceeded its cap: %v", scored)
	}
	if got := pickGenre(scored, "Unknown"); got != "Rock" {
		t.Fatalf("got %q, want Rock", got)
	}
}

func TestDirectElectronicEvidenceStillWins(t *testing.T) {
	if got := pickGenre(map[string]float64{"electronic": 10, "rock": 9.5}, "Unknown"); got != "Electronic" {
		t.Fatalf("got %q, want Electronic", got)
	}
}

func TestRockElectronicNearTieDoesNotOverrideBetterGenre(t *testing.T) {
	if got := pickGenre(map[string]float64{"electronic": 10, "rock": 9.5, "classical": 15}, "Unknown"); got != "Classical" {
		t.Fatalf("got %q, want Classical", got)
	}
}

func TestCanonicalHeavyMetalEvidenceConsolidates(t *testing.T) {
	if got := pickGenre(map[string]float64{"rock": 6, "metal": 5, "heavy metal": 4}, "Unknown"); got != "Metal" {
		t.Fatalf("got %q, want Metal", got)
	}
}

func TestNoGenreEvidenceUsesConfiguredFallback(t *testing.T) {
	if got := pickGenre(map[string]float64{"drums": 10, "guitar": 7, "synth": 6}, "Unknown"); got != "Unknown" {
		t.Fatalf("got %q, want Unknown", got)
	}
}
