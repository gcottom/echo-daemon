package internal

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/VoidObscura/echodaemon/config"
	"github.com/VoidObscura/echodaemon/logger"
	"github.com/VoidObscura/umpparser"
)

func ConvertFile(ctx context.Context, b []byte) ([]byte, error) {
	var args = []string{"-i", "pipe:0", "-c:a", "libmp3lame", "-b:a", "256k", "-f", "mp3", "-"}
	cmd := exec.Command("ffmpeg", args...)
	resultBuffer := bytes.NewBuffer(make([]byte, 0)) // pre allocate 5MiB buffer

	cmd.Stdout = resultBuffer // stdout result will be written here

	stdin, err := cmd.StdinPipe() // Open stdin pipe
	if err != nil {
		logger.ErrorC(ctx, "conversion error", slog.Any("error", err))
		return nil, err
	}

	err = cmd.Start() // Start a process on another goroutine
	if err != nil {
		logger.ErrorC(ctx, "conversion error", slog.Any("error", err))
		return nil, err
	}

	_, err = stdin.Write(b) // pump audio data to stdin pipe
	if err != nil {
		logger.ErrorC(ctx, "conversion error", slog.Any("error", err))
		return nil, err
	}
	err = stdin.Close() // close the stdin, or ffmpeg will wait forever
	if err != nil {
		logger.ErrorC(ctx, "conversion error", slog.Any("error", err))
		return nil, err
	}
	err = cmd.Wait() // wait until ffmpeg finish
	if err != nil {
		logger.ErrorC(ctx, "conversion error", slog.Any("error", err))
		return nil, err
	}
	return resultBuffer.Bytes(), nil
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

func ParseUMPs(chunks [][]byte) ([]byte, error) {
	data, err := umpparser.ParseUMPChunks(chunks)
	if err != nil {
		return nil, err
	}
	return data.Media, nil
}

// CreateConcatFile generates a text file listing all audio segments
func CreateConcatFile(files []string, id string) (string, error) {
	concatFile := fmt.Sprintf("%s/concat_list_%s.txt", config.AppConfig.TempDir, id)
	f, err := os.Create(concatFile)
	if err != nil {
		return "", err
	}
	defer f.Close()

	for _, file := range files {
		_, err := f.WriteString(fmt.Sprintf("file '%s'\n", file))
		if err != nil {
			return "", err
		}
	}

	return concatFile, nil
}

// MergeAudioFiles uses FFmpeg to join segments and return MP3 as []byte
func MergeAudioFiles(concatFile string) ([]byte, error) {
	cmd := exec.Command("ffmpeg",
		"-f", "concat", "-safe", "0", "-i", concatFile, // Read list of files
		"-c", "copy", // No transcoding
		"-f", "mp3", // Output format MP3
		"pipe:1", // Send output to stdout
	)

	var outputBuffer bytes.Buffer
	cmd.Stdout = &outputBuffer // Capture stdout
	var stderr bytes.Buffer
	cmd.Stderr = &stderr // Capture stderr for debugging

	// Run FFmpeg process
	err := cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("ffmpeg failed: %v, stderr: %s", err, stderr.String())
	}

	return outputBuffer.Bytes(), nil
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
