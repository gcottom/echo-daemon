package internal

import (
	"bytes"
	"context"
	"os/exec"
	"testing"
)

func TestConvertDropsFiveSecondTimestampHole(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("FFmpeg is not installed")
	}
	// Two seconds of actual audio, with a five second forward PTS jump at the midpoint.
	gen := exec.Command("ffmpeg", "-nostdin", "-v", "error", "-f", "lavfi", "-i", "sine=frequency=997:sample_rate=48000:duration=2", "-af", `asetpts=PTS+5/TB*gte(T\,1)`, "-c:a", "pcm_s16le", "-f", "matroska", "pipe:1")
	data, err := gen.Output()
	if err != nil {
		t.Fatal(err)
	}
	mp3, err := ConvertFile(context.Background(), data)
	if err != nil {
		t.Fatal(err)
	}
	dec := exec.Command("ffmpeg", "-nostdin", "-v", "error", "-xerror", "-i", "pipe:0", "-ac", "1", "-ar", "48000", "-f", "s16le", "pipe:1")
	dec.Stdin = bytes.NewReader(mp3)
	pcm, err := dec.Output()
	if err != nil {
		t.Fatal(err)
	}
	// Allow the MP3 encoder's frame padding, but not a multi-second silent seam.
	duration := float64(len(pcm)) / (48000 * 2)
	if duration < 1.99 || duration > 2.12 {
		t.Fatalf("expected ~2 seconds of decoded audio, got %.3f seconds", duration)
	}
}
