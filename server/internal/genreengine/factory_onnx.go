//go:build onnx

package genreengine

// NewPreferredEngine returns the best available engine for the current build.
// ONNX build: try to create the ONNX engine; if it fails, fall back to heuristic.
func NewPreferredEngine(opt Options) (Engine, error) {
	eng, err := NewONNXEngine(opt)
	if err != nil {
		return NewHeuristicEngine(opt), err
	}
	return eng, nil
}
