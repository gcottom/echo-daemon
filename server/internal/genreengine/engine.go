package genreengine

import (
	"context"
)

// Result represents the genre classification result.
type Result struct {
	Genre string
}

// Engine is the interface for a genre classification engine.
type Engine interface {
	// Classify runs genre classification for the given audio file path and returns a Result.
	Classify(ctx context.Context, filePath string, topN int) (Result, error)
}

// Options holds configuration for the engine.
type Options struct {
	// ModelDir is the directory where ONNX models and label files are vendored.
	ModelDir string
	// DefaultGenre is used only as a last resort fallback when inference cannot produce any tag.
	DefaultGenre string
	// WorkDir is used for any temporary artifacts (decoded audio, etc.). Optional.
	WorkDir string
}
