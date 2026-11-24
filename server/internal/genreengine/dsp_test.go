package genreengine

import (
	"math"
	"testing"
)

func TestMusicNNPatchFrames(t *testing.T) {
	if got := threeSecondFrames(); got != 187 {
		t.Fatalf("got %d frames, want 187", got)
	}
}

func TestSTFTPeriodicWindowAndDC(t *testing.T) {
	got := stftMagnitude([]float32{1, 1, 1, 1, 1, 1, 1, 1}, 8, 4)
	if len(got) != 3 {
		t.Fatalf("centered frame count = %d, want 3", len(got))
	}
	want := []float32{16, 4, 0, 0, 0}
	for _, frame := range got {
		for i, x := range want {
			if math.Abs(float64(frame[i]-x)) > 1e-5 {
				t.Fatalf("bin %d got %v want %v", i, frame[i], x)
			}
		}
	}
}

func TestMelBankSlaneyArea(t *testing.T) {
	f := melFilterbank(16000, 512, 96)
	if len(f) != 96 || len(f[0]) != 257 {
		t.Fatalf("unexpected shape")
	}
	// First peak lands near 31.096 Hz; the 31.25 Hz FFT bin lies on its falling edge.
	want := (2.0*hzToMel(8000)/97*(200.0/3) - 31.25) / (math.Pow(hzToMel(8000)/97*(200.0/3), 2))
	if math.Abs(float64(f[0][1])-want) > 1e-6 {
		t.Fatalf("wrong area-normalized first-band coefficient: %v", f[0][1])
	}
	for i, r := range f {
		var sum float32
		for _, v := range r {
			sum += v
		}
		if sum <= 0 {
			t.Fatalf("empty mel band %d", i)
		}
	}
}
