package downloader

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"

	"github.com/gcottom/audiometa/v4"
	"github.com/gcottom/echodaemon/config"
	"github.com/gcottom/echodaemon/internal"
)

func TestLocalAudioRoundTrip(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("FFmpeg is not installed")
	}
	cfg := &config.Config{TempDir: filepath.Join(t.TempDir(), "nested", "temp"), SaveDir: filepath.Join(t.TempDir(), "nested", "data")}
	original := config.AppConfig
	t.Cleanup(func() { config.AppConfig = original })
	config.AppConfig = cfg
	source := filepath.Join(t.TempDir(), "generated.m4a")
	// FFmpeg needs a seekable destination to finalize the WAV size header.
	if out, err := exec.Command("ffmpeg", "-nostdin", "-v", "error", "-f", "lavfi", "-i", "sine=frequency=997:duration=0.3", "-c:a", "aac", source).CombinedOutput(); err != nil {
		t.Fatalf("generate WAV fixture: %v: %s", err, out)
	}
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "converted.mp3")
	if err := internal.ConvertPath(ctx, source, path); err != nil {
		t.Fatal(err)
	}
	input, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	tag, err := audiometa.OpenTag(bytes.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	tag.SetArtist("Echo Test")
	tag.SetTitle("Generated Tone")
	var out bytes.Buffer
	if err := tag.Save(&out); err != nil {
		t.Fatal(err)
	}
	s := &Service{LibraryMap: new(sync.Map)}
	name := filepath.Join(cfg.SaveDir, SanitizeFilename("Echo Test - Generated Tone.mp3"))
	if err := os.MkdirAll(cfg.SaveDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte("keep existing"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveFile(ctx, "Example1234", out.Bytes()); !errors.Is(err, os.ErrExist) {
		t.Fatalf("expected no-clobber error, got %v", err)
	}
	preserved, err := os.ReadFile(name)
	if err != nil || string(preserved) != "keep existing" {
		t.Fatalf("existing output changed: %q %v", preserved, err)
	}
	if err := os.Remove(name); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveFile(ctx, "Example1234", out.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveFile(ctx, "Example1234", out.Bytes()); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(cfg.SaveDir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("want one completed file: %v %v", entries, err)
	}
	got, err := os.ReadFile(name)
	if err != nil || !bytes.Equal(got, out.Bytes()) {
		t.Fatal("complete tagged MP3 differs")
	}
	if b, err := exec.Command("ffmpeg", "-v", "error", "-xerror", "-i", name, "-f", "null", "-").CombinedOutput(); err != nil {
		t.Fatalf("output did not decode: %v %s", err, b)
	}
}
