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
	"strings"
	"time"

	"path/filepath"
	"regexp"
	"unicode"
	"unicode/utf8"

	"github.com/VoidObscura/echodaemon/config"
	"github.com/VoidObscura/echodaemon/internal"
	"github.com/VoidObscura/echodaemon/internal/ump_parser"
	"github.com/VoidObscura/echodaemon/logger"
	"github.com/gcottom/audiometa/v3"

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
				s.CurrentCapture.Requests = append(s.CurrentCapture.Requests, *req.CaptureRequest)
				if !replaySuccess {
					bod, err := ReplayCapture(ctx, *req.CaptureRequest)
					if err != nil {
						logger.ErrorC(ctx, "error replaying request", slog.Any("error", err))
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

func ReplayCapture(ctx context.Context, capReq CaptureRequest) ([]byte, error) {
	logger.InfoC(ctx, "replaying capture request", slog.String("url", capReq.URL))
	if u, err := url.Parse(capReq.URL); err == nil && u.Scheme != "" && u.Host != "" {
		if strings.Contains(u.Host, "googlevideo.com") {
			q := u.Query()
			// Many sabr/ump links include transient range params for partial fetches; remove to fetch full object
			q.Del("range")
			q.Del("rn")
			q.Del("rbuf")
			u.RawQuery = q.Encode()

			logger.InfoC(ctx, fmt.Sprintf("Downloading UMP-encoded data from: %s", u.String()))
			out, err := DownloadWithHeaders(u.String(), map[string]string{
				"Accept":     "application/vnd.yt-ump",
				"User-Agent": "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome Safari",
			})
			if err != nil {
				logger.ErrorC(ctx, "failed to download UMP data", slog.Any("error", err))
				return nil, err
			}

			logger.InfoC(ctx, "Decoding UMP data")
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
