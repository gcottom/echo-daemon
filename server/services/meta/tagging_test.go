package meta

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/gcottom/audiometa/v4"
)

func TestTagTrackM4A(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("FFmpeg is not installed")
	}
	run := func(args ...string) []byte {
		t.Helper()
		cmd := exec.Command(ffmpeg, append([]string{"-nostdin", "-v", "error"}, args...)...)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("ffmpeg %v: %v: %s", args, err, &stderr)
		}
		return out
	}
	art := image.NewNRGBA(image.Rect(0, 0, 4, 3))
	for y := range 3 {
		for x := range 4 {
			art.SetNRGBA(x, y, color.NRGBA{R: 255, A: 255})
		}
	}
	var cover bytes.Buffer
	if err := png.Encode(&cover, art); err != nil {
		t.Fatal(err)
	}
	coverServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/cover.png" {
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(cover.Bytes())
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(coverServer.Close)

	for _, path := range []string{"/cover.png", "/missing.png"} {
		t.Run(path[1:], func(t *testing.T) {
			root := t.TempDir()
			input, output := filepath.Join(root, "input.m4a"), filepath.Join(root, "output.m4a")
			run("-f", "lavfi", "-i", "sine=frequency=997:sample_rate=44100:duration=0.3",
				"-c:a", "aac", "-movflags", "+faststart", "-metadata", "title=Old title", input)
			before, err := os.ReadFile(input)
			if err != nil {
				t.Fatal(err)
			}
			result, err := tagTrack(context.Background(), "generated", input, &TrackMeta{
				Artist: " Echo Test ", Title: " Generated Tone ", Album: " Fixture Album ",
				Genre: " Electronic ", CoverArtURL: coverServer.URL + path,
			})
			if err != nil {
				t.Fatal(err)
			}
			tag, err := audiometa.OpenTag(bytes.NewReader(result))
			if err != nil {
				t.Fatal(err)
			}
			if tag.GetArtist() != "Echo Test" || tag.GetTitle() != "Generated Tone" ||
				tag.GetAlbum() != "Fixture Album" || tag.GetGenre() != "Electronic" {
				t.Fatalf("wrong tags: %q / %q / %q / %q", tag.GetArtist(), tag.GetTitle(), tag.GetAlbum(), tag.GetGenre())
			}
			got := tag.GetCoverArt()
			if path == "/cover.png" {
				if got == nil || (*got).Bounds().Size() != art.Bounds().Size() {
					t.Fatal("cover art did not round-trip")
				}
			} else if got != nil {
				t.Fatal("unexpected cover art after failed fetch")
			}
			if after, err := os.ReadFile(input); err != nil || !bytes.Equal(after, before) {
				t.Fatalf("source audio changed: %v", err)
			}
			if err := os.WriteFile(output, result, 0600); err != nil {
				t.Fatal(err)
			}
			pcm := func(file string) []byte {
				return run("-xerror", "-i", file, "-map", "0:a:0", "-f", "s16le", "-acodec", "pcm_s16le", "-")
			}
			if want := pcm(input); len(want) == 0 || !bytes.Equal(pcm(output), want) {
				t.Fatal("decoded PCM changed during metadata rewrite")
			}
		})
	}

	t.Run("fragmented write rejected", func(t *testing.T) {
		input := filepath.Join(t.TempDir(), "fragmented.m4a")
		run("-f", "lavfi", "-i", "sine=frequency=997:duration=0.3",
			"-c:a", "aac", "-movflags", "+empty_moov+frag_keyframe", input)
		before, err := os.ReadFile(input)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := audiometa.OpenTag(bytes.NewReader(before)); err != nil {
			t.Fatalf("fixture must reach the save step: %v", err)
		}
		if out, err := tagTrack(context.Background(), "fragmented", input, &TrackMeta{
			Artist: "Echo Test", Title: "Rejected title", Genre: "Electronic",
		}); err == nil || out != nil {
			t.Fatalf("unsupported rewrite returned %d bytes and %v", len(out), err)
		}
		if after, err := os.ReadFile(input); err != nil || !bytes.Equal(after, before) {
			t.Fatalf("source audio changed after failed rewrite: %v", err)
		}
		if err := os.Remove(input); err != nil {
			t.Fatalf("could not remove input after failed rewrite: %v", err)
		}
	})
}

func TestTagTrackInvalidMedia(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invalid.m4a")
	if err := os.WriteFile(path, []byte("not an audio file"), 0600); err != nil {
		t.Fatal(err)
	}
	if out, err := tagTrack(context.Background(), "invalid", path, &TrackMeta{}); err == nil || out != nil {
		t.Fatalf("invalid input returned %d bytes and %v", len(out), err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatalf("could not remove input after failed tagging: %v", err)
	}
}
