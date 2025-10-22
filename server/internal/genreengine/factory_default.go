//go:build !onnx

package genreengine

// NewPreferredEngine returns the best available engine for the current build.
// Default build (no `onnx` tag): return heuristic engine.
func NewPreferredEngine(opt Options) (Engine, error) {
	return NewHeuristicEngine(opt), nil
}
