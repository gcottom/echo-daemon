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
	"strings"
	"time"

	"github.com/VoidObscura/echodaemon/config"
	"github.com/VoidObscura/echodaemon/internal"
	"github.com/VoidObscura/echodaemon/logger"
	"github.com/gcottom/audiometa/v3"
	"github.com/gcottom/retry"

	"io/fs"
	"path/filepath"
	"regexp"
	"unicode"
	"unicode/utf8"

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
	reg := regexp.MustCompile(`[^a-zA-Z0-9_.\-\(\)&]`)
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

// SanitizeTree walks a directory (non-recursive or recursive; here recursive)
// and renames files/dirs in-place to sanitized names. It skips collisions.
func SanitizeTree(root string) error {
	// Walk from deepest paths first so we rename children before parents.
	type item struct{ oldPath, newPath string }
	var items []item

	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == root {
			return nil
		}
		dir := filepath.Dir(p)
		sanitized := SanitizeFilename(d.Name())
		if sanitized == d.Name() {
			return nil
		}
		items = append(items, item{
			oldPath: p,
			newPath: filepath.Join(dir, sanitized),
		})
		return nil
	})
	if err != nil {
		return err
	}

	// Rename children before parents: sort by path depth descending.
	// (Simple stable sort by length works for most cases.)
	for i := len(items) - 1; i >= 0; i-- {
		it := items[i]
		if _, err := os.Stat(it.newPath); err == nil {
			// Collision: append a short suffix.
			base := it.newPath
			ext := filepath.Ext(base)
			stem := strings.TrimSuffix(base, ext)
			it.newPath = fmt.Sprintf("%s_%d%s", stem, i, ext)
		}
		if err := os.Rename(it.oldPath, it.newPath); err != nil {
			return fmt.Errorf("rename %q -> %q: %w", it.oldPath, it.newPath, err)
		}
	}
	return nil
}
func (s *Service) Cleanup(ctx context.Context, id string) {
	_ = os.Remove(fmt.Sprintf("%s/%s", config.AppConfig.TempDir, id))
	_ = os.Remove(fmt.Sprintf("%s/%s.%s", config.AppConfig.TempDir, id, internal.FILEFORMAT))
}

func (s *Service) NewCapture(ctx context.Context, id string) {
	cap := CaptureChanData{
		TrackID: id,
		IsStart: true,
	}
	s.CaptureChannel <- cap
}

func (s *Service) ContinueCapture(ctx context.Context, cap CaptureRequest) {
	capData := CaptureChanData{
		CaptureRequest: &cap,
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
				s.CurrentCapture = &CurrentCapture{ID: id, Data: make([][]byte, 0)}
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
					s.CurrentCapture.Data = append(s.CurrentCapture.Data, bod)
					logger.InfoC(ctx, "Captured data length", slog.Int("length", len(bod)))
					go func(id string, data [][]byte) {
						if id == "" {
							return
						}
						ctx := context.Background()
						if err := os.Mkdir(config.AppConfig.TempDir, 0755); err != nil && !os.IsExist(err) {
							logger.ErrorC(ctx, "failed to create temp dir", slog.Any("error", err))
							return
						}
						logger.InfoC(ctx, "processing captured audio")
						dat, err := internal.ParseUMPs(data)
						if err != nil {
							logger.ErrorC(ctx, "error parsing UMP data", slog.Any("error", err))
							return
						}
						joinedData, err := internal.ConvertFile(ctx, dat)
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
	logger.InfoC(ctx, "replaying capture")
	req, err := http.NewRequest(capReq.Method, capReq.URL, strings.NewReader(capReq.Body))
	if err != nil {
		return nil, err
	}
	u, err := url.Parse(capReq.URL)
	if err != nil {
		panic(err)
	}

	// Get and edit query parameters
	q := u.Query()
	q.Set("range", "0-"+q.Get("clen")) // set or replace param

	// Re-encode query params
	u.RawQuery = q.Encode()

	for key, value := range capReq.Headers {
		req.Header.Set(key, value)
	}
	if capReq.Cookies != "" {
		req.Header.Set("Cookie", capReq.Cookies) // Send cookies properly formatted
	}
	req.URL = u
	res, err := retry.Retry(retry.NewAlgExpDefault(), 3, func() (*http.Response, error) {
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}
		if res.StatusCode > 299 {
			_ = res.Body.Close()
			return nil, errors.New("replay capture returned code: " + res.Status)
		}
		return res, nil
	})
	if err != nil {
		return nil, err
	}
	defer res[0].(*http.Response).Body.Close()

	bod, err := io.ReadAll(res[0].(*http.Response).Body)
	if err != nil {
		return nil, err
	}
	return bod, nil

}
