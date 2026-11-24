package downloader

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"path/filepath"
	"regexp"
	"unicode"
	"unicode/utf8"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/gcottom/audiometa/v3"
	"github.com/gcottom/echodaemon/config"
	"github.com/gcottom/echodaemon/internal"
	"github.com/gcottom/echodaemon/internal/ump_parser"
	"github.com/gcottom/echodaemon/logger"
	"golang.org/x/text/unicode/norm"
)

func (s *Service) ConvertFile(ctx context.Context, id string, data []byte) error {
	convertedData, err := internal.ConvertFile(ctx, data)
	if err != nil {
		logger.ErrorC(ctx, "failed to convert file", slog.String("id", id), slog.Any("error", err))
		return fmt.Errorf("failed to convert file: %w", err)
	}
	if err = os.MkdirAll(config.AppConfig.TempDir, 0755); err != nil {
		logger.ErrorC(ctx, "failed to create temp dir", slog.Any("error", err))
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	savePath := fmt.Sprintf("%s/%s.%s", config.AppConfig.TempDir, id, internal.FILEFORMAT)
	if err = os.WriteFile(savePath, convertedData, 0644); err != nil {
		logger.ErrorC(ctx, "failed to write file", slog.String("id", id), slog.Any("error", err))
		return fmt.Errorf("failed to write file: %w", err)
	}
	return nil
}

// Use a separate temporary file for each job so simultaneous requests cannot
// replace or clean up another job's input.
func convertToTemporaryFile(ctx context.Context, data []byte) (string, error) {
	converted, err := internal.ConvertFile(ctx, data)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(config.AppConfig.TempDir, 0755); err != nil {
		return "", err
	}
	f, err := os.CreateTemp(config.AppConfig.TempDir, "track-*.mp3")
	if err != nil {
		return "", err
	}
	_, writeErr := f.Write(converted)
	closeErr := f.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		_ = os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}

func (s *Service) GetMeta(ctx context.Context, id string) ([]byte, error) {
	return s.MetaServiceClient.AddMeta(ctx, id, fmt.Sprintf("%s/%s.%s", config.AppConfig.TempDir, id, internal.FILEFORMAT))
}

func (s *Service) SaveFile(ctx context.Context, id string, data []byte) error {
	reader := bytes.NewReader(data)
	tag, err := audiometa.OpenTag(reader)
	if err != nil {
		logger.ErrorC(ctx, "failed to open tag", slog.String("id", id), slog.Any("error", err))
		return fmt.Errorf("failed to open tag: %w", err)
	}
	if err = os.MkdirAll(config.AppConfig.SaveDir, 0755); err != nil {
		logger.ErrorC(ctx, "failed to create save dir", slog.Any("error", err))
		return fmt.Errorf("failed to create save dir: %w", err)
	}
	title, artist := strings.TrimSpace(tag.GetTitle()), strings.TrimSpace(tag.GetArtist())
	if title == "" || artist == "" {
		return errors.New("output requires a title and artist")
	}
	key := title + " - " + artist
	s.saveMu.Lock()
	defer s.saveMu.Unlock()
	if s.LibraryMap != nil {
		if _, ok := s.LibraryMap.Load(key); ok {
			return nil
		}
	}
	name := SanitizeFilename(fmt.Sprintf("%s - %s.%s", artist, title, internal.FILEFORMAT))
	savePath := filepath.Join(config.AppConfig.SaveDir, name)
	if err := writeNewFile(savePath, data); err != nil {
		return fmt.Errorf("save track: %w", err)
	}
	if s.LibraryMap != nil {
		s.LibraryMap.Store(key, true)
	}
	logger.InfoC(ctx, "file saved successfully", slog.String("path", savePath), slog.String("id", id))
	return nil
}

// Publish a complete output without overwriting an existing file. Staging in
// the destination directory keeps both paths on the same filesystem.
func writeNewFile(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".echo-partial-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Link(f.Name(), path)
}

func SanitizeFilename(name string) string {
	if name == "" || name == "." || name == ".." {
		return "_"
	}

	// Separate extension so we can truncate the base safely.
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)

	// Normalize to NFD so it matches macOS on-disk normalization.
	base = norm.NFD.String(base)
	ext = norm.NFD.String(ext)

	// Replace path separators and other problem chars.
	// (We’re conservative: slash is illegal on POSIX; others are common troublemakers.)
	replacer := strings.NewReplacer(
		"/", "-",
		"\\", "-",
		"\x00", "", // NUL never allowed
		":", "-", // safer across tools
		"*", "-",
		"?", "-",
		"\"", "'",
		"<", "(",
		">", ")",
		"|", "-",
	)
	base = replacer.Replace(base)

	// Remove control chars and trim weird spacing.
	var b strings.Builder
	b.Grow(len(base))
	prevSpace := false
	for _, r := range base {
		if r == utf8.RuneError {
			continue
		}
		if unicode.IsControl(r) {
			continue
		}
		// collapse whitespace runs to single space
		if unicode.IsSpace(r) {
			if !prevSpace {
				b.WriteRune(' ')
			}
			prevSpace = true
			continue
		}
		prevSpace = false
		b.WriteRune(r)
	}
	base = strings.TrimSpace(b.String())

	// If the base becomes empty, use a placeholder.
	if base == "" {
		base = "_"
	}

	// Optional: collapse runs of dashes/spaces.
	reDash := regexp.MustCompile(`[ \-]{2,}`)
	base = reDash.ReplaceAllString(base, "-")

	// Final name then truncate to 255 bytes (keep extension intact).
	const maxBytes = 255
	fn := base + ext
	if len(fn) > maxBytes {
		// Shrink base portion to fit.
		target := maxBytes - len(ext)
		if target < 1 {
			target = maxBytes // worst-case: no ext space; just hard cut
		}
		base = truncateBytes(base, target)
		fn = base + ext
	}

	// Disallow dot-only and leading/trailing dots/spaces (some tools hate these).
	fn = strings.Trim(fn, " .")
	if fn == "" {
		fn = "_"
	}
	reg := regexp.MustCompile(`[^a-zA-Z0-9_.\-()&]`)
	fn = reg.ReplaceAllString(fn, "_") // replace any remaining illegal chars with underscore
	return fn
}

// truncateBytes cuts a string to at most n bytes without splitting runes.
func truncateBytes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	var buf bytes.Buffer
	buf.Grow(n)
	for _, r := range s {
		rb := make([]byte, 4)
		nb := utf8.EncodeRune(rb, r)
		if buf.Len()+nb > n {
			break
		}
		buf.Write(rb[:nb])
	}
	return buf.String()
}

func (s *Service) Cleanup(ctx context.Context, id string) {
	_ = os.Remove(fmt.Sprintf("%s/%s.%s", config.AppConfig.TempDir, id, internal.FILEFORMAT))
}

var ErrInvalidID = errors.New("invalid track or playlist ID")

var validDownloadID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

func (s *Service) DownloadByID(ctx context.Context, id string) error {
	logger.InfoC(ctx, "downloading by ID", slog.String("id", id))
	if !validDownloadID.MatchString(id) {
		return ErrInvalidID
	}
	if len(id) != 11 {
		logger.InfoC(ctx, "playlist ID detected, attempting to download all tracks in playlist")
		go func(id string) {
			ctx := context.WithValue(context.Background(), "id", id)
			ids, err := s.MetaServiceClient.GetPlaylistEntries(ctx, id)
			if err != nil {
				logger.ErrorC(ctx, "failed to get playlist entries", slog.String("id", id), slog.Any("error", err))
				return
			}
			logger.InfoC(ctx, "found playlist entries", slog.Int("count", len(ids)))
			for _, i := range ids {
				if err := s.DownloadByID(ctx, i); err != nil {
					logger.ErrorC(ctx, "failed to download track", slog.String("id", i), slog.Any("error", err))
				}
			}
		}(id)
		return nil
	}
	captureRequest, err := s.FindDownloadTarget(ctx, id)
	if err != nil {
		logger.ErrorC(ctx, "failed to find download target", slog.String("id", id), slog.Any("error", err))
		return err
	}
	bod, err := ReplayCapture(ctx, *captureRequest, id)
	if err != nil {
		logger.ErrorC(ctx, "error replaying request", slog.Any("error", err))
		return fmt.Errorf("audio fetch failed: %w", err)
	}
	if len(bod) == 0 {
		return errors.New("audio response contained no usable data")
	}
	logger.InfoC(ctx, "captured data length", slog.Int("length", len(bod)))

	go func(id string, data []byte) {
		ctx := context.WithValue(context.Background(), "id", id)
		procCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
		path, err := convertToTemporaryFile(procCtx, data)
		if err != nil {
			logger.ErrorC(ctx, "error converting captured audio", slog.Any("error", err))
			return
		}
		defer os.Remove(path)
		enrichedData, err := s.MetaServiceClient.AddMeta(procCtx, id, path)
		if err != nil {
			logger.ErrorC(ctx, "error getting meta", slog.Any("error", err))
			return
		}
		if err := s.SaveFile(ctx, id, enrichedData); err != nil {
			logger.ErrorC(ctx, "error saving file", slog.Any("error", err))
			return
		}
		logger.InfoC(ctx, "download complete", slog.String("id", id))
	}(id, bod)
	return nil
}

func (s *Service) FindDownloadTarget(ctx context.Context, id string) (*CaptureRequest, error) {
	logger.InfoC(ctx, "finding download target", slog.String("id", id))
	if id == "" {
		return nil, fmt.Errorf("no track ID found")
	}
	home, _ := os.UserHomeDir()
	profileDir := filepath.Join(home, ".cache", "echodaemon-chrome")
	_ = os.MkdirAll(profileDir, 0o700)
	// Allocator + single context (tab auto-created)
	allocOpts := []chromedp.ExecAllocatorOption{
		chromedp.UserDataDir(profileDir),
		chromedp.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/115.0.0.0 Safari/537.36"),
		chromedp.Flag("no-first-run", true),
		chromedp.Flag("no-default-browser-check", true),
		chromedp.Flag("disable-default-apps", true),
		chromedp.Flag("disable-extensions", true),
		chromedp.Flag("disable-features", "TranslateUI"),
		chromedp.Flag("force-color-profile", "srgb"),
		chromedp.Flag("disable-infobars", true),
		chromedp.Flag("autoplay-policy", "no-user-gesture-required"),
		chromedp.Flag("lang", "en-US"),
		chromedp.Flag("mute-audio", true),
		chromedp.Flag("headless", true),
	}
	allocCtx, allocCancel := chromedp.NewExecAllocator(ctx, allocOpts...)
	defer allocCancel()
	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()
	ch := make(chan CaptureRequest, 10)
	chromedp.ListenTarget(ctx, func(ev interface{}) {
		switch e := ev.(type) {
		case *runtime.EventConsoleAPICalled:
			var args []string
			for _, a := range e.Args {
				if a.Value != nil {
					args = append(args, fmt.Sprint(a.Value))
				}
			}
		case *network.EventRequestWillBeSent:
			if _, ok := e.Request.Headers["Range"]; (ok && strings.Contains(e.Request.URL, "videoplayback") ||
				strings.Contains(e.Request.URL, "googlevideo.com")) &&
				(strings.Contains(e.Request.URL, "itag=140") ||
					strings.Contains(e.Request.URL, "itag=141") ||
					strings.Contains(e.Request.URL, "itag=249") ||
					strings.Contains(e.Request.URL, "itag=250") ||
					strings.Contains(e.Request.URL, "itag=251") ||
					strings.Contains(e.Request.URL, "itag=599") ||
					strings.Contains(e.Request.URL, "itag=600")) {
				headers := make(map[string]string)
				for k, v := range e.Request.Headers {
					headers[k] = fmt.Sprint(v)
				}
				ch <- CaptureRequest{
					URL:     e.Request.URL,
					Method:  e.Request.Method,
					Headers: headers,
					Cookies: "",
					Body:    "",
				}
				return
			}
		}
	})

	// Enable network and set consent cookies (best-effort)
	if err := chromedp.Run(ctx, network.Enable(), network.SetCacheDisabled(true)); err != nil {
		logger.ErrorC(ctx, "failed to enable network when trying to find download target", slog.String("id", id), slog.Any("error", err))
		return nil, err
	}
	consentVal := "YES+cb.20210328-17-p0.en+FX+123"
	_ = network.SetCookie("CONSENT", consentVal).WithDomain(".youtube.com").WithPath("/").WithSecure(true).Do(ctx)
	_ = network.SetCookie("CONSENT", consentVal).WithDomain(".music.youtube.com").WithPath("/").WithSecure(true).Do(ctx)

	// Navigate with timeout + fallback
	targetURL := fmt.Sprintf("https://music.youtube.com/watch?v=%s", id)
	navCtx, navCancel := context.WithTimeout(ctx, 40*time.Second)
	defer navCancel()
	if err := chromedp.Run(navCtx,
		chromedp.Navigate(targetURL),
		chromedp.WaitReady("body", chromedp.ByQuery),
		chromedp.Sleep(1200*time.Millisecond),
		chromedp.EvaluateAsDevTools("Array.from(document.querySelectorAll('video,audio')).forEach(m=>{try{m.muted=true;m.volume=0;}catch(e){}})", nil),
	); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		return nil, err
	}
	var out CaptureRequest
	select {
	case out = <-ch:
		logger.InfoC(ctx, "found download target", slog.String("id", id))
		return &out, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("context cancelled or deadline exceeded while waiting for download target: %w", ctx.Err())
	case <-time.After(15 * time.Second):
		return nil, fmt.Errorf("timeout waiting for download target")
	}
}

func ReplayCapture(ctx context.Context, capReq CaptureRequest, id string) ([]byte, error) {
	logger.InfoC(ctx, "replaying capture request", slog.String("id", id))
	if u, err := url.Parse(capReq.URL); err == nil && u.Scheme != "" && u.Host != "" {
		if strings.Contains(u.Host, "googlevideo.com") {
			q := u.Query()
			q.Del("range")
			q.Del("rn")
			q.Del("rbuf")
			u.RawQuery = q.Encode()
			clen, err := strconv.Atoi(q.Get("clen"))
			if err != nil {
				logger.ErrorC(ctx, "failed to parse clen param", slog.Any("error", err), slog.String("clen", q.Get("clen")))
				return nil, fmt.Errorf("invalid clen param: %w", err)
			}

			numSegs := (clen + MB - 1) / MB
			ch := make(chan SegData, numSegs)

			for start, idx := 0, 0; start < clen; start, idx = start+MB, idx+1 {
				end := start + MB - 1
				if end >= clen {
					end = clen - 1
				}

				i, c := start, idx
				segURL := u.String()
				logger.InfoC(ctx, "downloading UMP-encoded data segment", slog.Int("start_byte", i), slog.Int("end_byte", end))
				go func(start, end, segIndex int, rawURL string) {
					qv := make(map[string][]string)
					for k, v := range q {
						qv[k] = append(qv[k], v...)
					}
					qx := url.Values(qv)
					ur := *u
					qx.Set("range", fmt.Sprintf("%d-%d", start, end))
					ur.RawQuery = qx.Encode()
					var d []byte
					var err error
					for attempt := 1; attempt <= 3; attempt++ {
						d, err = DownloadWithHeaders(ur.String(), map[string]string{
							"Range":      fmt.Sprintf("bytes=%d-%d", start, end),
							"Accept":     "application/vnd.yt-ump",
							"User-Agent": "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome Safari",
						})
						if err == nil {
							break
						}
						time.Sleep(time.Duration(attempt) * 300 * time.Millisecond)
					}
					if err != nil {
						logger.ErrorC(ctx, "failed to download UMP data segment", slog.Any("error", err), slog.Int("segment", segIndex))
						ch <- SegData{C: segIndex, Data: nil}
						return
					}
					logger.InfoC(ctx, "downloaded UMP data segment", slog.Int("segment", segIndex), slog.Int("length", len(d)))
					ch <- SegData{C: segIndex, Data: d}
				}(i, end, c, segURL)
			}

			datSlices := make([][]byte, numSegs)
			received := 0
			for received < numSegs {
				seg := <-ch
				if seg.Data == nil {
					return nil, fmt.Errorf("failed to download one or more UMP data segments")
				}
				datSlices[seg.C] = seg.Data
				received++
			}

			// Concatenate segments in order.
			total := 0
			for _, d := range datSlices {
				total += len(d)
			}
			out := make([]byte, 0, total)
			for _, d := range datSlices {
				out = append(out, d...)
			}

			logger.InfoC(ctx, "Decoding UMP data", slog.Int("bytes", len(out)))
			return ump_parser.DecodeUMPFile(out)
		}
	}
	logger.ErrorC(ctx, "unsupported URL scheme")
	return nil, fmt.Errorf("unsupported URL scheme")
}

func DownloadWithHeaders(rawURL string, headers map[string]string) ([]byte, error) {
	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	return io.ReadAll(resp.Body)
}
