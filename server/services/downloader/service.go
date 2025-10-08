package downloader

import (
	"bytes"
	"context"
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
	if err = os.Mkdir(config.AppConfig.TempDir, 0755); err != nil && !os.IsExist(err) {
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
	if err = os.Mkdir(config.AppConfig.SaveDir, 0755); err != nil && !os.IsExist(err) {
		logger.ErrorC(ctx, "failed to create save dir", slog.Any("error", err))
		return fmt.Errorf("failed to create save dir: %w", err)
	}
	logger.InfoC(ctx, "checking if file already exists in library map", slog.String("key", tag.GetTitle()+" - "+tag.GetArtist()))
	if _, ok := s.LibraryMap.Load(tag.GetTitle() + " - " + tag.GetArtist()); ok {
		logger.InfoC(ctx, "file already exists in library, skipping", slog.String("id", id), slog.String("key", tag.GetTitle()+" - "+tag.GetArtist()))
		return nil // File already exists in library map, skip saving
	}
	s.LibraryMap.Store(tag.GetTitle()+" - "+tag.GetArtist(), true)
	savePath := fmt.Sprintf("%s - %s.%s", tag.GetArtist(), tag.GetTitle(), internal.FILEFORMAT)
	savePath = SanitizeFilename(savePath)
	savePath = filepath.Join(config.AppConfig.SaveDir, savePath)
	logger.InfoC(ctx, "Saving file", slog.String("path", savePath), slog.String("id", id))
	savePath = internal.SanitizePath(savePath)
	if err = os.WriteFile(savePath, data, 0644); err != nil {
		logger.ErrorC(ctx, "failed to write file", slog.String("id", id), slog.Any("error", err))
		return fmt.Errorf("failed to write file: %w", err)
	}
	logger.InfoC(ctx, "File saved successfully", slog.String("path", savePath), slog.String("id", id))
	return nil
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
	_ = os.Remove(fmt.Sprintf("%s/%s.%s", config.AppConfig.TempDir, id, internal.FILEFORMAT))
}

func (s *Service) NewCapture(ctx context.Context, id string) {
	msg := CaptureChanData{
		TrackID: id,
		IsStart: true,
	}
	s.CaptureChannel <- msg
}

func (s *Service) ContinueCapture(ctx context.Context, req CaptureRequest) {
	capData := CaptureChanData{
		CaptureRequest: &req,
	}
	s.CaptureChannel <- capData
}

func (s *Service) CaptureProcessor(ctx context.Context) {
	id := ""
	replaySuccess := false
	for {
		select {
		case req := <-s.CaptureChannel:
			if req.IsStart {
				logger.InfoC(ctx, "new capture started")
				id = req.TrackID
				logger.InfoC(ctx, "Capture track id", slog.String("id", id))
				s.CurrentCapture = &CurrentCapture{ID: id, Data: make([]byte, 0)}
				replaySuccess = false
			} else {
				if !replaySuccess {
					if id == "" {
						logger.ErrorC(ctx, "no current capture ID found")
						continue
					}
					s.CurrentCapture.Requests = append(s.CurrentCapture.Requests, *req.CaptureRequest)
					if len(s.CurrentCapture.Requests)%2 == 0 {
						logger.InfoC(ctx, "skipping even numbered request")
						continue
					}
					logger.InfoC(ctx, "attempting to replay request", slog.Int("requestNumber", len(s.CurrentCapture.Requests)))
					bod, err := ReplayCapture(ctx, *req.CaptureRequest, s.CurrentCapture.ID)
					if err != nil {
						logger.ErrorC(ctx, "error replaying request", slog.Any("error", err))
						continue
					}
					if len(bod) < 1_000_000 {
						logger.InfoC(ctx, "replayed data too small, attempting to retry download with next request", slog.Int("length", len(bod)))
						continue
					}
					replaySuccess = true
					// Only use the single replayed buffer, no overlap merging
					s.CurrentCapture.Data = bod
					logger.InfoC(ctx, "Captured data length", slog.Int("length", len(bod)))

					go func(id string, data []byte) {
						joinedData, err := internal.ConvertFile(ctx, data)
						if err != nil {
							logger.ErrorC(ctx, "error joining data for captured audio", slog.Any("error", err))
							return
						}
						savePath := fmt.Sprintf("%s/%s.%s", config.AppConfig.TempDir, id, internal.FILEFORMAT)
						if err = os.WriteFile(savePath, joinedData, 0644); err != nil {
							logger.ErrorC(ctx, "failed to write file", slog.String("id", id), slog.Any("error", err))
							return
						}
						metaedData, err := s.GetMeta(ctx, id)
						if err != nil {
							logger.ErrorC(ctx, "error getting meta", slog.Any("error", err))
							return
						}
						if err := s.SaveFile(ctx, id, metaedData); err != nil {
							logger.ErrorC(ctx, "error saving file", slog.Any("error", err))
							return
						}
					}(s.CurrentCapture.ID, s.CurrentCapture.Data)
				}
			}
		default:
			time.Sleep(100 * time.Millisecond)
		}
	}
}

func ReplayCapture(ctx context.Context, capReq CaptureRequest, id string) ([]byte, error) {
	logger.InfoC(ctx, "replaying capture request", slog.String("url", capReq.URL))
	if u, err := url.Parse(capReq.URL); err == nil && u.Scheme != "" && u.Host != "" {
		if strings.Contains(u.Host, "googlevideo.com") {
			q := u.Query()
			// Many sabr/ump links include transient range params for partial fetches; remove to fetch full object
			q.Del("range")
			q.Del("rn")
			q.Del("rbuf")

			expireStr := q.Get("expire")
			if expireStr == "" {
				logger.ErrorC(ctx, "missing expire param")
				return nil, fmt.Errorf("missing expire param")
			}
			expireUnix, err := strconv.ParseInt(expireStr, 10, 64)
			if err != nil {
				logger.ErrorC(ctx, "failed to parse expire param", slog.Any("error", err), slog.String("expire", expireStr))
				return nil, fmt.Errorf("invalid expire param: %w", err)
			}
			totalLength := q.Get("dur")
			totalLengthVal, err := strconv.ParseFloat(totalLength, 32)
			if err != nil {
				logger.ErrorC(ctx, "failed to parse total length", slog.Any("error", err), slog.String("totalLength", totalLength))
				return nil, fmt.Errorf("invalid total length: %w", err)
			}
			estDownloadTimeRemaining := int(totalLengthVal / 2)

			for {
				if estDownloadTimeRemaining%5 == 0 {
					break
				}
				estDownloadTimeRemaining++
			}

			expiryTime := time.Unix(expireUnix, 0)
			remaining := time.Until(expiryTime)
			const minLifetime = 30 * time.Second
			if remaining <= minLifetime {
				logger.ErrorC(ctx, "insufficient token life remaining", slog.Int64("expire_unix", expireUnix), slog.Time("expiry_time", expiryTime), slog.Duration("remaining", remaining))
				return nil, fmt.Errorf("insufficient token life remaining (%s)", remaining)
			}
			done := false
			defer func(done *bool) {
				*done = true
			}(&done)
			// Update query (after cleaning transient params)
			u.RawQuery = q.Encode()
			go func(done *bool) {
				startTime := time.Now()
				for {
					if *done {
						return
					}
					time.Sleep(5 * time.Second)
					if *done {
						return
					}
					elapsed := time.Since(startTime).Truncate(time.Second).Seconds()
					logger.InfoC(ctx, "downloading UMP-encoded data", slog.String("id", id), slog.Float64("time elapsed (seconds)", elapsed), slog.Int("eta (seconds)", estDownloadTimeRemaining-int(elapsed)-5))
				}
			}(&done)
			logger.InfoC(ctx, "token life OK", slog.Int64("expire_unix", expireUnix), slog.Time("expiry_time", expiryTime), slog.Int("remaining_seconds", int(remaining.Seconds())))
			logger.InfoC(ctx, fmt.Sprintf("Downloading UMP-encoded data from: %s", u.String()))
			out, err := DownloadWithHeaders(u.String(), map[string]string{
				"Accept":     "application/vnd.yt-ump",
				"User-Agent": "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome Safari",
			})
			if err != nil {
				logger.ErrorC(ctx, "failed to download UMP data", slog.Any("error", err))
				return nil, err
			}
			done = true
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
