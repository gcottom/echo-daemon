package genreengine

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/gcottom/echodaemon/logger"
	onnx "github.com/yalue/onnxruntime_go"
)

type onnxSession struct {
	sess     *onnx.Session[float32]
	inName   string
	outName  string
	inT      *onnx.Tensor[float32]
	outT     *onnx.Tensor[float32]
	frames   int
	nClasses int
}

// ONNXEngine loads ONNX models and performs native inference using yalue's onnxruntime binding.
type ONNXEngine struct {
	opt      Options
	sessions map[string]*onnxSession
	so       *onnx.SessionOptions
}

// NewONNXEngine attempts to initialize ONNX Runtime and load any available models
// from opt.ModelDir. Callers should be prepared to fall back if initialization fails.
func NewONNXEngine(ctx context.Context, opt Options) (*ONNXEngine, error) {
	// Ensure model dir exists
	if stat, err := os.Stat(opt.ModelDir); err != nil || !stat.IsDir() {
		return nil, fmt.Errorf("model dir not found: %s", opt.ModelDir)
	}

	// Ensure the shared library can be located; best-effort path set for Docker image layout.
	const ortLibPath = "/usr/local/lib/libonnxruntime.so"
	onnx.SetSharedLibraryPath(ortLibPath)
	if err := onnx.InitializeEnvironment(); err != nil {
		return nil, fmt.Errorf("failed to init onnxruntime env: %w (ensure libonnxruntime.so is installed and compatible with the onnxruntime_go binding; see documentation for version requirements)", err)
	}

	e := &ONNXEngine{opt: opt, sessions: map[string]*onnxSession{}}
	models := []string{"MSD_musicnn.onnx", "MSD_vgg.onnx", "MTT_musicnn.onnx", "MTT_vgg.onnx"}
	for _, m := range models {
		p := filepath.Join(opt.ModelDir, m)
		if _, err := os.Stat(p); err == nil {
			// Determine the number of classes for the model
			nClasses := len(MsdLabels)
			if strings.Contains(m, "MTT") {
				nClasses = len(MttLabels)
			}
			// Choose candidate frame lengths to handle exporter/library differences
			base := threeSecondFrames()
			var frameCandidates []int
			if strings.Contains(strings.ToLower(m), "vgg") {
				frameCandidates = []int{base - 2, base - 1, base}
			} else {
				frameCandidates = []int{base, base - 1, base - 2}
			}
			tryPairs := [][2]string{{"model/input:0", "model/prob:0"}, {"model_input", "model_prob"}}
			var sess *onnx.Session[float32]
			var inName, outName string
			var inEx, outEx *onnx.Tensor[float32]
			var usedFrames int
			for _, nf := range frameCandidates {
				if nf <= 0 {
					continue
				}
				// Create example tensors for this candidate frame length
				inTmp, err := onnx.NewTensor[float32]([]int64{1, int64(nf), int64(NumMels)}, make([]float32, nf*NumMels))
				if err != nil {
					logger.WarnC(ctx, "onnx failed to create example input tensor", slog.String("model", p), slog.Int("nFrames", nf), slog.Any("error", err))
					continue
				}
				outTmp, err := onnx.NewTensor[float32]([]int64{1, int64(nClasses)}, make([]float32, nClasses))
				if err != nil {
					logger.WarnC(ctx, "onnx failed to create example output tensor", slog.String("model", p), slog.Int("nFrames", nf), slog.Any("error", err))
					_ = inTmp.Destroy()
					continue
				}
				// Try IO name pairs for this frame length
				for _, pair := range tryPairs {
					s, sErr := onnx.NewSession[float32](p, []string{pair[0]}, []string{pair[1]}, []*onnx.Tensor[float32]{inTmp}, []*onnx.Tensor[float32]{outTmp})
					if sErr != nil {
						logger.WarnC(ctx, "onnx session create failed for IO names", slog.String("model", p), slog.String("input", pair[0]), slog.String("output", pair[1]), slog.Int("nFrames", nf), slog.Any("error", sErr))
						continue
					}
					// Validate by attempting a dry run with zeroed tensors
					if err := s.Run(); err != nil {
						logger.WarnC(ctx, "onnx session validation run failed", slog.String("model", p), slog.String("input", pair[0]), slog.String("output", pair[1]), slog.Int("nFrames", nf), slog.Any("error", err))
						_ = s.Destroy()
						continue
					}
					// Success for this nf and IO pair
					sess = s
					inName, outName = pair[0], pair[1]
					inEx, outEx = inTmp, outTmp
					usedFrames = nf
					break
				}
				if sess != nil {
					break
				}
				// Cleanup and try next nf
				_ = inTmp.Destroy()
				_ = outTmp.Destroy()
			}
			if sess == nil {
				logger.WarnC(ctx, "failed to load model with any known IO names; skipping", slog.String("model", p))
				continue
			}
			e.sessions[m] = &onnxSession{sess: sess, inName: inName, outName: outName, inT: inEx, outT: outEx, frames: usedFrames, nClasses: nClasses}
		} else {
			logger.WarnC(ctx, "onnx model file not found", slog.String("model", p))
		}
	}
	if len(e.sessions) == 0 {
		_ = onnx.DestroyEnvironment()
		return nil, fmt.Errorf("no ONNX models found in %s", opt.ModelDir)
	}
	return e, nil
}

// Close releases all ONNX sessions and environment resources.
func (e *ONNXEngine) Close() {
	for _, s := range e.sessions {
		if s == nil {
			continue
		}
		if s.inT != nil {
			_ = s.inT.Destroy()
		}
		if s.outT != nil {
			_ = s.outT.Destroy()
		}
		if s.sess != nil {
			_ = s.sess.Destroy()
		}
	}
	_ = onnx.DestroyEnvironment()
}

// Classify decodes audio, computes a mel-spectrogram, runs ONNX inference across available
// models (MTT/MSD musicnn and VGG), aggregates tags, and returns a single genre.
func (e *ONNXEngine) Classify(ctx context.Context, filePath string, topN int) (Result, error) {
	pcm, err := decodePCMFloat32(ctx, filePath)
	if err != nil {
		return Result{}, fmt.Errorf("audio decode failed: %w", err)
	}
	mel := melSpectrogram(pcm)
	if len(mel) == 0 {
		return Result{}, fmt.Errorf("no spectrogram frames produced")
	}

	// Build 3-second patches (non-overlapping) to match training window
	nFrames := threeSecondFrames()
	patches := makePatches(mel, nFrames)
	if len(patches) == 0 {
		return Result{}, fmt.Errorf("insufficient audio for inference")
	}
	// Ensure we actually have sessions to run
	if len(e.sessions) == 0 {
		return Result{}, fmt.Errorf("no ONNX sessions loaded")
	}

	var lists [][]string
	var anyOK bool
	var modelErrs []string

	// Helper to run a model if present
	runModel := func(name string, labels []string) {
		s, ok := e.sessions[name]
		if !ok {
			// Always log when a model wasn't loaded during init
			logger.WarnC(ctx, "onnx model not loaded; skipping", slog.String("model", name))
			modelErrs = append(modelErrs, fmt.Sprintf("%s:not_loaded", name))
			return
		}
		probs, err := averageProbsOverPatches(ctx, s, patches)
		if err != nil {
			// Log per-model inference failure
			logger.ErrorC(ctx, "onnx model inference failed", slog.String("model", name), slog.Any("error", err))
			modelErrs = append(modelErrs, fmt.Sprintf("%s:%v", name, err))
			return
		}
		anyOK = true
		topTags := topTagsFromProbs(labels, probs, topN)

		// Log the model's top predictions with their probabilities
		topTagsWithProbs := make([]string, len(topTags))
		for i, tag := range topTags {
			// Find the probability for this tag
			var prob float32
			for j, label := range labels {
				if label == tag && j < len(probs) {
					prob = probs[j]
					break
				}
			}
			topTagsWithProbs[i] = fmt.Sprintf("%s(%.3f)", tag, prob)
		}
		lists = append(lists, topTags)
	}

	// Prefer musicnn models; include VGG when available
	runModel("MTT_musicnn.onnx", MttLabels)
	runModel("MSD_musicnn.onnx", MsdLabels)
	runModel("MTT_vgg.onnx", MttLabels)
	runModel("MSD_vgg.onnx", MsdLabels)

	if !anyOK || len(lists) == 0 {
		// Return an error so callers can log and fall back to default
		return Result{}, fmt.Errorf("onnx inference failed for all models: %s", strings.Join(modelErrs, "; "))
	}
	// Aggregate ranked tags and pick a final genre
	scores := aggregateTagScores(ctx, lists...)
	genre := pickGenre(scores, e.opt.DefaultGenre)
	return Result{Genre: genre}, nil
}

// makePatches returns non-overlapping 3s patches of size [nFrames][NumMels]. If input is shorter
// than one patch, it returns a single zero-padded patch.
func makePatches(mel [][]float32, nFrames int) [][][]float32 {
	var patches [][][]float32
	if len(mel) < nFrames {
		p := make([][]float32, nFrames)
		for i := 0; i < nFrames; i++ {
			p[i] = make([]float32, NumMels)
			if i < len(mel) {
				copy(p[i], mel[i])
			}
		}
		return [][][]float32{p}
	}
	for i := 0; i+nFrames <= len(mel); i += nFrames {
		p := make([][]float32, nFrames)
		for t := 0; t < nFrames; t++ {
			row := make([]float32, NumMels)
			copy(row, mel[i+t])
			p[t] = row
		}
		patches = append(patches, p)
	}
	return patches
}

// averageProbsOverPatches runs inference per-patch (batch size 1) and averages probabilities.
func averageProbsOverPatches(ctx context.Context, s *onnxSession, patches [][][]float32) ([]float32, error) {
	nClasses := s.nClasses
	sum := make([]float32, nClasses)
	cnt := 0
	for i, p := range patches {
		out, err := runOnce(ctx, s, p)
		if err != nil {
			logger.ErrorC(ctx, "onnx patch inference failed", slog.Int("patch_index", i), slog.Any("error", err))
			continue
		}
		// out length may differ; accumulate up to min length
		m := nClasses
		if len(out) < m {
			m = len(out)
		}
		for j := 0; j < m; j++ {
			sum[j] += out[j]
		}
		cnt++
	}
	if cnt == 0 {
		return nil, fmt.Errorf("no successful inferences")
	}
	for i := range sum {
		sum[i] /= float32(cnt)
	}
	return sum, nil
}

// runOnce flattens a single patch to the model's expected [frames][NumMels], copies it into the bound input tensor,
// runs the session, and returns the output probabilities.
func runOnce(ctx context.Context, s *onnxSession, patch [][]float32) ([]float32, error) {
	frames := s.frames
	if frames <= 0 {
		return nil, fmt.Errorf("invalid session frames: %d", frames)
	}
	// Prepare flat input [1, frames, NumMels] with crop/pad behavior per model.
	flat := make([]float32, frames*NumMels)
	for t := 0; t < frames; t++ {
		if t < len(patch) {
			copy(flat[t*NumMels:(t+1)*NumMels], patch[t])
		}
	}
	// Copy into pre-bound input tensor
	inData := s.inT.GetData()
	if len(inData) < len(flat) {
		return nil, fmt.Errorf("input tensor too small: have %d want %d", len(inData), len(flat))
	}
	copy(inData, flat)
	// Run the session (uses bound tensors)
	if err := s.sess.Run(); err != nil {
		return nil, fmt.Errorf("session run failed: %w", err)
	}
	outData := s.outT.GetData()
	// Normalize length to session class count if needed
	nClasses := s.nClasses
	if len(outData) > nClasses {
		outData = outData[:nClasses]
	} else if len(outData) < nClasses {
		tmp := make([]float32, nClasses)
		copy(tmp, outData)
		outData = tmp
	}
	return outData, nil
}
