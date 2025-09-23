package internal

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/VoidObscura/echodaemon/logger"
)

func ConvertFile(ctx context.Context, b []byte) ([]byte, error) {
	// Decode and transcode while regenerating linear audio timestamps to avoid gaps at joins.
	var args = []string{
		"-hide_banner", "-loglevel", "error",
		"-fflags", "+genpts+igndts", // ignore/don't trust DTS, generate PTS
		"-i", "pipe:0", // read input from stdin
		"-vn", "-sn", // drop video/subtitles
		"-avoid_negative_ts", "make_zero", // normalize timestamps at splice points
		"-map", "0:a:0?", // select first audio stream if present
		"-af", "aresample=async=1:first_pts=0", // linearize PTS by sample index; minor resync only
		"-c:a", "libmp3lame", "-b:a", "256k",
		"-f", "mp3", "-", // output MP3 to stdout (pipe)
	}
	// Use CommandContext so cancellation/timeouts propagate to ffmpeg
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)

	// Pipe input via an in-memory reader to avoid manual StdinPipe writes (prevents EPIPE on early-exit)
	cmd.Stdin = bytes.NewReader(b)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// Include stderr to help diagnose codec/container issues instead of surfacing generic EPIPE
		errWrap := fmt.Errorf("ffmpeg failed: %w; stderr: %s", err, stderr.String())
		logger.ErrorC(ctx, "conversion error", slog.Any("error", errWrap))
		return nil, errWrap
	}
	return stdout.Bytes(), nil
}

func OSExecuteFindJSONStart(ctx context.Context, command string, args ...string) ([]byte, error) {
	cmd := exec.Command(command, args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		logger.ErrorC(ctx, "failed to execute command", slog.Any("error", err))
		return nil, err
	}
	i := bytes.LastIndex(out.Bytes(), []byte("{"))
	return out.Bytes()[i:], nil
}

func SanitizePath(path string) string {
	invalidChars := regexp.MustCompile(`[<>:"/\\|?*\x00-\x1F]`)
	components := strings.Split(filepath.ToSlash(path), "/")
	for i, component := range components {
		if component == "" {
			continue
		}
		safeComponent := invalidChars.ReplaceAllString(component, "_")
		safeComponent = strings.Trim(safeComponent, " .")
		const maxLength = 255
		if len(safeComponent) > maxLength {
			safeComponent = safeComponent[:maxLength]
		}
		components[i] = safeComponent
	}
	sanitizedPath := filepath.Join(components...)
	sanitizedPath = strings.Replace(sanitizedPath, fmt.Sprintf(" .%s", FILEFORMAT), fmt.Sprintf(".%s", FILEFORMAT), -1)
	return sanitizedPath
}
