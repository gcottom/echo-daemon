package internal

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gcottom/echodaemon/logger"
)

func ConvertFile(ctx context.Context, b []byte) ([]byte, error) {
	var out bytes.Buffer
	if err := convertAudio(ctx, "pipe:0", "-", bytes.NewReader(b), &out); err != nil {
		return nil, err
	}
	if out.Len() == 0 {
		return nil, fmt.Errorf("ffmpeg produced no MP3 audio")
	}
	return out.Bytes(), nil
}

// ConvertPath transcodes a seekable input, including MP4s with trailing metadata.
func ConvertPath(ctx context.Context, input, output string) error {
	if err := convertAudio(ctx, input, output, nil, nil); err != nil {
		return err
	}
	info, err := os.Stat(output)
	if err != nil {
		return fmt.Errorf("check MP3 output: %w", err)
	}
	if info.Size() == 0 {
		return fmt.Errorf("ffmpeg produced no MP3 audio")
	}
	return nil
}

func convertAudio(ctx context.Context, input, output string, reader *bytes.Reader, writer *bytes.Buffer) error {
	logger.InfoC(ctx, "converting file")
	// Build the timeline from decoded samples, not packet timestamp gaps.
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-nostdin", "-n", "-hide_banner", "-loglevel", "error", "-xerror",
		"-fflags", "+genpts+igndts", "-i", input, "-vn", "-sn",
		"-avoid_negative_ts", "make_zero", "-map", "0:a:0", "-af", "asetpts=N/SR/TB",
		"-c:a", "libmp3lame", "-b:a", "256k", "-f", "mp3", output)
	if reader != nil {
		cmd.Stdin = reader
	}
	if writer != nil {
		cmd.Stdout = writer
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("ffmpeg failed: %w; stderr: %s", err, stderr.String())
	}
	return nil
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
