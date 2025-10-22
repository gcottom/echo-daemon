//go:build onnx

package genreengine

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"os/exec"
)

// DSP parameters mirrored from python/genre-service/musicnn/configuration.py
const (
	SampleRate = 16000
	FFTHop     = 256
	FFTSize    = 512
	NumMels    = 96
)

// decodePCMFloat32 uses ffmpeg to decode any supported audio file into raw float32 PCM at 16k mono.
// It returns a slice of samples in range [-1,1].
func decodePCMFloat32(ctx context.Context, filePath string) ([]float32, error) {
	// ffmpeg -v error -i input -f f32le -acodec pcm_f32le -ac 1 -ar 16000 -
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-v", "error",
		"-i", filePath,
		"-t", "30",
		"-f", "f32le",
		"-acodec", "pcm_f32le",
		"-ac", "1",
		"-ar", fmt.Sprintf("%d", SampleRate),
		"-")
	var out bytes.Buffer
	var errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffmpeg decode failed: %v, %s", err, errBuf.String())
	}
	b := out.Bytes()
	if len(b)%4 != 0 {
		return nil, errors.New("decoded pcm byte length not multiple of 4")
	}
	n := len(b) / 4
	res := make([]float32, n)
	for i := 0; i < n; i++ {
		bits := uint32(b[4*i]) | uint32(b[4*i+1])<<8 | uint32(b[4*i+2])<<16 | uint32(b[4*i+3])<<24
		res[i] = math.Float32frombits(bits)
	}
	return res, nil
}

// hannWindow returns Hann window of given size.
func hannWindow(n int) []float32 {
	w := make([]float32, n)
	if n <= 1 {
		for i := range w {
			w[i] = 1
		}
		return w
	}
	for i := 0; i < n; i++ {
		w[i] = float32(0.5 * (1 - math.Cos(2*math.Pi*float64(i)/float64(n-1))))
	}
	return w
}

// stftMagnitude computes magnitude spectrogram (power) using a correct naive DFT.
// This is not the fastest approach but is acceptable for short clips and small FFTSize.
func stftMagnitude(x []float32, nFFT, hop int) [][]float32 {
	win := hannWindow(nFFT)
	// number of frames
	frames := 0
	if len(x) >= nFFT {
		frames = 1 + (len(x)-nFFT)/hop
	}
	if frames <= 0 {
		return [][]float32{}
	}
	bins := nFFT/2 + 1
	S := make([][]float32, frames)
	for f := 0; f < frames; f++ {
		start := f * hop
		// Windowed time-domain samples (real input)
		xw := make([]float64, nFFT)
		for i := 0; i < nFFT; i++ {
			v := 0.0
			if start+i < len(x) {
				v = float64(x[start+i])
			}
			xw[i] = v * float64(win[i])
		}
		// naive DFT for simplicity (O(N^2)); acceptable for nFFT=512
		reSpec := make([]float64, nFFT)
		imSpec := make([]float64, nFFT)
		for k := 0; k < nFFT; k++ {
			var sumRe, sumIm float64
			for n := 0; n < nFFT; n++ {
				angle := -2 * math.Pi * float64(k*n) / float64(nFFT)
				c := math.Cos(angle)
				s := math.Sin(angle)
				// Imaginary part of input is zero
				sumRe += xw[n] * c
				sumIm += xw[n] * s
			}
			reSpec[k] = sumRe
			imSpec[k] = sumIm
		}
		row := make([]float32, bins)
		for k := 0; k < bins; k++ {
			p := reSpec[k]*reSpec[k] + imSpec[k]*imSpec[k]
			row[k] = float32(p)
		}
		S[f] = row
	}
	return S
}

// melFilterbank returns a [NumMels][nFFT/2+1] triangular filterbank.
func melFilterbank(sr, nFFT, nMels int) [][]float32 {
	fMin := 0.0
	fMax := float64(sr) / 2.0
	nBins := nFFT/2 + 1
	melMin := hzToMel(fMin)
	melMax := hzToMel(fMax)
	melPoints := make([]float64, nMels+2)
	for i := 0; i < nMels+2; i++ {
		melPoints[i] = melMin + (melMax-melMin)*float64(i)/float64(nMels+1)
	}
	hzPoints := make([]float64, nMels+2)
	for i := range hzPoints {
		hzPoints[i] = melToHz(melPoints[i])
	}
	bin := make([]int, nMels+2)
	for i := range bin {
		bin[i] = int(math.Floor((float64(nFFT) + 1.0) * hzPoints[i] / float64(sr)))
	}
	fb := make([][]float32, nMels)
	for m := 1; m <= nMels; m++ {
		fb[m-1] = make([]float32, nBins)
		for k := bin[m-1]; k < bin[m]; k++ {
			if k >= 0 && k < nBins {
				fb[m-1][k] = float32(float64(k-bin[m-1]) / float64(max(1, bin[m]-bin[m-1])))
			}
		}
		for k := bin[m]; k < bin[m+1]; k++ {
			if k >= 0 && k < nBins {
				fb[m-1][k] = float32(float64(bin[m+1]-k) / float64(max(1, bin[m+1]-bin[m])))
			}
		}
	}
	return fb
}

func hzToMel(hz float64) float64 {
	return 2595.0 * math.Log10(1.0+hz/700.0)
}

func melToHz(mel float64) float64 {
	return 700.0 * (math.Pow(10.0, mel/2595.0) - 1.0)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// melSpectrogram computes mel spectrogram with log scaling matching the Python pipeline.
// Returns [frames][NumMels].
func melSpectrogram(x []float32) [][]float32 {
	S := stftMagnitude(x, FFTSize, FFTHop)
	if len(S) == 0 {
		return [][]float32{}
	}
	fb := melFilterbank(SampleRate, FFTSize, NumMels)
	frames := len(S)
	mel := make([][]float32, frames)
	for t := 0; t < frames; t++ {
		mel[t] = make([]float32, NumMels)
		for m := 0; m < NumMels; m++ {
			var sum float64
			for k := 0; k < len(S[t]); k++ {
				sum += float64(S[t][k]) * float64(fb[m][k])
			}
			// log scaling: log10(1 + 10000 * x)
			mel[t][m] = float32(math.Log10(1.0 + 10000.0*sum))
		}
	}
	return mel
}

// threeSecondFrames computes the number of spectrogram frames for 3.0s according to librosa.time_to_frames + 1.
func threeSecondFrames() int {
	// librosa: n = 1 + int(round(t*sr/hop))
	return 1 + int(math.Round(3.0*float64(SampleRate)/float64(FFTHop)))
}
