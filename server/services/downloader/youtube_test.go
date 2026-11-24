package downloader

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gcottom/youtube-go/youtube"
)

type testTransport func(*http.Request) (*http.Response, error)

func (f testTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestYouTubeGoFetch(t *testing.T) {
	const id = "Example1234"
	for _, tc := range []struct {
		name, formatKey, mime, ext string
		itag                       int
		truncated                  bool
	}{
		{"audio ranges", "adaptiveFormats", "audio/mp4; codecs=\"mp4a.40.2\"", ".m4a", 140, false},
		{"combined fallback", "formats", "video/mp4; codecs=\"avc1.42001E, mp4a.40.2\"", ".mp4", 18, false},
		{"truncated second part", "adaptiveFormats", "audio/mp4; codecs=\"mp4a.40.2\"", ".m4a", 140, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			player := fmt.Sprintf(`{"playabilityStatus":{"status":"OK"},"videoDetails":{"videoId":"%s","title":"Fixture"},"streamingData":{"%s":[{"itag":%d,"mimeType":%q,"contentLength":"8","url":"https://r1.googlevideo.com/videoplayback"}]}}`, id, tc.formatKey, tc.itag, tc.mime)
			n := 0
			transport := testTransport(func(r *http.Request) (*http.Response, error) {
				header := make(http.Header)
				status, content := http.StatusOK, player
				switch {
				case r.URL.Host == "www.youtube.com" && r.URL.Path == "/watch":
					content = "<script>var ytInitialPlayerResponse = " + player + ";</script>"
				case r.URL.Host == "www.youtube.com" && r.URL.Path == "/youtubei/v1/player":
				case r.URL.Host == "r1.googlevideo.com" && r.URL.Path == "/videoplayback":
					status = http.StatusPartialContent
					header.Set("Content-Type", "audio/mp4")
					switch n {
					case 0:
						if r.Header.Get("Range") != "bytes=0-7" {
							t.Fatalf("first range: %q", r.Header.Get("Range"))
						}
						header.Set("Content-Range", "bytes 0-3/8")
						content = "abcd"
					case 1:
						if r.Header.Get("Range") != "bytes=4-7" {
							t.Fatalf("next range: %q", r.Header.Get("Range"))
						}
						header.Set("Content-Range", "bytes 4-7/8")
						content = "efgh"
						if tc.truncated {
							content = "ef"
						}
					default:
						t.Fatal("unexpected extra media request")
					}
					n++
				default:
					return nil, fmt.Errorf("unexpected test request: %s %s", r.Method, r.URL.Path)
				}
				return &http.Response{StatusCode: status, Header: header, ContentLength: int64(len(content)), Body: io.NopCloser(strings.NewReader(content)), Request: r}, nil
			})
			src := youtube.NewClient(youtube.Options{SkipJavaScript: true, HTTPClient: &http.Client{Transport: transport}})
			root := t.TempDir()
			job, path, err := fetchTrack(context.Background(), src, root, id)
			if n != 2 {
				t.Fatalf("want two media requests, got %d; fetch error: %v", n, err)
			}
			if tc.truncated {
				if err == nil || job != "" || path != "" {
					t.Fatalf("a truncated download escaped: %q %q %v", job, path, err)
				}
				if entries, e := os.ReadDir(root); e != nil || len(entries) != 0 {
					t.Fatalf("staging not cleaned: %v %v", entries, e)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if filepath.Ext(path) != tc.ext || filepath.Dir(path) != job {
				t.Fatalf("unexpected source path %q", path)
			}
			b, err := os.ReadFile(path)
			if err != nil || string(b) != "abcdefgh" {
				t.Fatalf("joined output %q: %v", b, err)
			}
		})
	}
}
