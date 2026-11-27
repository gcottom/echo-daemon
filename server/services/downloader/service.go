package downloader

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"path/filepath"
	"regexp"
	"unicode"
	"unicode/utf8"

	"github.com/gcottom/audiometa/v4"
	"github.com/gcottom/echodaemon/config"
	"github.com/gcottom/echodaemon/internal"
	"github.com/gcottom/echodaemon/logger"
	"github.com/gcottom/youtube-go/youtube"
	"golang.org/x/text/unicode/norm"
)

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
	// (WeΓÇÖre conservative: slash is illegal on POSIX; others are common troublemakers.)
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
	fetchCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	jobDir, path, err := fetchTrack(fetchCtx, s.source, config.AppConfig.TempDir, id)
	if err != nil {
		logger.ErrorC(ctx, "failed to fetch public audio", slog.Any("error", err))
		return errors.New("could not download direct public audio for this track")
	}

	go func() {
		defer os.RemoveAll(jobDir)
		procCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Minute)
		defer cancel()
		converted := filepath.Join(jobDir, "converted.mp3")
		if err := internal.ConvertPath(procCtx, path, converted); err != nil {
			logger.ErrorC(procCtx, "error converting downloaded audio", slog.Any("error", err))
			return
		}
		enrichedData, err := s.MetaServiceClient.AddMeta(procCtx, id, converted)
		if err != nil {
			logger.ErrorC(procCtx, "error getting meta", slog.Any("error", err))
			return
		}
		if err := s.SaveFile(procCtx, id, enrichedData); err != nil {
			logger.ErrorC(procCtx, "error saving file", slog.Any("error", err))
			return
		}
		logger.InfoC(procCtx, "download complete", slog.String("id", id))
	}()
	return nil
}

type trackSource interface {
	Video(context.Context, string) (*youtube.Video, error)
	Download(context.Context, *youtube.Video, youtube.DownloadOptions) (string, error)
}

var publicYouTube = youtube.NewClient(youtube.Options{SkipJavaScript: true})

// fetchTrack returns a private directory owned by the caller on success.
func fetchTrack(ctx context.Context, source trackSource, tempDir, id string) (string, string, error) {
	if source == nil {
		source = publicYouTube
	}
	video, err := source.Video(ctx, id)
	if err != nil {
		return "", "", fmt.Errorf("find video: %w", err)
	}
	if video.Live {
		return "", "", errors.New("live streams are not supported")
	}
	publicVideo := *video
	publicVideo.Formats = nil
	for _, f := range video.Formats {
		if f.HasAudio && f.Protocol == "https" && strings.HasPrefix(f.URL, "https://") {
			publicVideo.Formats = append(publicVideo.Formats, f)
		}
	}
	if len(publicVideo.Formats) == 0 {
		return "", "", errors.New("no direct public audio or combined format is available")
	}
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return "", "", fmt.Errorf("create temp directory: %w", err)
	}
	jobDir, err := os.MkdirTemp(tempDir, "track-*")
	if err != nil {
		return "", "", fmt.Errorf("create track directory: %w", err)
	}
	path, err := source.Download(ctx, &publicVideo, youtube.DownloadOptions{
		Output: filepath.Join(jobDir, "source"), AppendExtension: true, Format: "bestaudio/best",
	})
	if err != nil {
		_ = os.RemoveAll(jobDir)
		return "", "", fmt.Errorf("download audio: %w", err)
	}
	return jobDir, path, nil
}
