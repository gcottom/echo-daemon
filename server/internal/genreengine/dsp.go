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

// The bundled musicnn models expect centered power spectra, periodic Hann
// windows, and Slaney mel weights with area normalization.
func hannWindow(n int) []float32 {
	w := make([]float32, n)
	for i := range w {
		if n <= 1 {
			w[i] = 1
		} else {
			w[i] = float32(.5 - .5*math.Cos(2*math.Pi*float64(i)/float64(n)))
		}
	}
	return w
}

func reflectIndex(i, length int) int {
	if length <= 1 {
		return 0
	}
	period := 2 * (length - 1)
	i = (i%period + period) % period
	if i >= length {
		i = period - i
	}
	return i
}

func stftMagnitude(x []float32, nFFT, hop int) [][]float32 {
	if len(x) == 0 || hop <= 0 || nFFT < 2 || nFFT&(nFFT-1) != 0 {
		return nil
	}
	win := hannWindow(nFFT)
	frames := 1 + len(x)/hop
	spectra := make([][]float32, frames)
	work := make([]complex128, nFFT)
	twiddle := make([]complex128, nFFT/2)
	for k := range twiddle {
		sn, cs := math.Sincos(-2 * math.Pi * float64(k) / float64(nFFT))
		twiddle[k] = complex(cs, sn)
	}
	for frame := 0; frame < frames; frame++ {
		start := frame*hop - nFFT/2
		for i, j := 0, 0; i < nFFT; i++ {
			work[j] = complex(float64(x[reflectIndex(start+i, len(x))])*float64(win[i]), 0)
			for bit := nFFT >> 1; bit > 0; bit >>= 1 {
				j ^= bit
				if j&bit != 0 {
					break
				}
			}
		}
		for span := 2; span <= nFFT; span *= 2 {
			half := span / 2
			for at := 0; at < nFFT; at += span {
				for k := 0; k < half; k++ {
					a, b := work[at+k], work[at+k+half]*twiddle[k*(nFFT/span)]
					work[at+k], work[at+k+half] = a+b, a-b
				}
			}
		}
		row := make([]float32, nFFT/2+1)
		for i := range row {
			re, im := real(work[i]), imag(work[i])
			row[i] = float32(re*re + im*im)
		}
		spectra[frame] = row
	}
	return spectra
}

func hzToMel(hz float64) float64 {
	if hz < 1000 {
		return hz / (200.0 / 3)
	}
	return 15 + math.Log(hz/1000)/(math.Log(6.4)/27)
}

func melToHz(mel float64) float64 {
	if mel < 15 {
		return mel * (200.0 / 3)
	}
	return 1000 * math.Exp((mel-15)*(math.Log(6.4)/27))
}

func melFilterbank(sr, nFFT, nMels int) [][]float32 {
	points := make([]float64, nMels+2)
	maxMel := hzToMel(float64(sr) / 2)
	for i := range points {
		points[i] = melToHz(maxMel * float64(i) / float64(nMels+1))
	}
	weights := make([][]float32, nMels)
	for m := range weights {
		lo, mid, hi := points[m], points[m+1], points[m+2]
		areaNorm := 2 / (hi - lo)
		row := make([]float32, nFFT/2+1)
		for k := range row {
			hz := float64(k*sr) / float64(nFFT)
			weight := math.Min((hz-lo)/(mid-lo), (hi-hz)/(hi-mid))
			if weight > 0 {
				row[k] = float32(weight * areaNorm)
			}
		}
		weights[m] = row
	}
	return weights
}

func melSpectrogram(x []float32) [][]float32 {
	spectra := stftMagnitude(x, FFTSize, FFTHop)
	if len(spectra) == 0 {
		return nil
	}
	bank := melFilterbank(SampleRate, FFTSize, NumMels)
	mel := make([][]float32, len(spectra))
	for t, s := range spectra {
		row := make([]float32, NumMels)
		for m, weights := range bank {
			var sum float64
			for k, w := range weights {
				sum += float64(s[k]) * float64(w)
			}
			row[m] = float32(math.Log1p(10000*sum) / math.Ln10)
		}
		mel[t] = row
	}
	return mel
}

func threeSecondFrames() int {
	return (3*SampleRate-FFTSize/2)/FFTHop + 1
}
