//go:build !onnx || !cgo

package genreengine

import "context"

func NewPreferredEngine(_ context.Context, opt Options) (Engine, error) {
	return NewHeuristicEngine(opt), nil
}
