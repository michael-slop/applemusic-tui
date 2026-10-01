package visualizer

import (
	"math"
	"testing"
)

func TestConverterPassesEqualFormatsThrough(t *testing.T) {
	f := Format{SampleRate: 48_000, Channels: 2}
	in := []float32{0.1, 0.2, 0.3, 0.4}
	if out := newConverter(f, f).convert(in); &out[0] != &in[0] || len(out) != len(in) {
		t.Fatal("equal formats must return the input unchanged")
	}
}

func TestConverterMapsChannels(t *testing.T) {
	mono := newConverter(Format{48_000, 1}, Format{48_000, 2}).convert([]float32{0.5, -0.25})
	if want := []float32{0.5, 0.5, -0.25, -0.25}; !equalF(mono, want) {
		t.Fatalf("mono -> stereo = %v, want %v", mono, want)
	}
	quad := newConverter(Format{48_000, 4}, Format{48_000, 2}).convert([]float32{1, 2, 3, 4, 5, 6, 7, 8})
	if want := []float32{1, 2, 5, 6}; !equalF(quad, want) {
		t.Fatalf("4 ch -> stereo = %v, want the front pair %v", quad, want)
	}
}

// The carry-over between chunks is where resamplers click: converting in one
// piece and in many uneven pieces must give the same samples.
func TestConverterResamplesTheSameWhateverTheChunking(t *testing.T) {
	in, out := Format{44_100, 2}, Format{48_000, 2}
	signal := sinePCM(44_100, 2, 997, 0.5, 44_100, false)
	whole := append([]float32(nil), newConverter(in, out).convert(signal)...)
	c := newConverter(in, out)
	var pieces []float32
	sizes := []int{1, 7, 64, 333, 1024, 5}
	for at, i := 0, 0; at < len(signal)/2; i++ {
		n := min(sizes[i%len(sizes)], len(signal)/2-at)
		pieces = append(pieces, c.convert(signal[2*at:2*(at+n)])...)
		at += n
	}
	if len(pieces) != len(whole) {
		t.Fatalf("chunked gave %d samples, whole gave %d", len(pieces), len(whole))
	}
	for i := range whole {
		if math.Abs(float64(whole[i]-pieces[i])) > 1e-5 {
			t.Fatalf("sample %d: whole %v, chunked %v", i, whole[i], pieces[i])
		}
	}
	// One second in gives one second out, give or take a frame.
	if frames := len(whole) / 2; frames < 47_999 || frames > 48_001 {
		t.Fatalf("1 s at 44.1 kHz became %d frames at 48 kHz", frames)
	}
}

// A tone keeps its pitch: the analyzer, built for the out rate, finds it in
// the band it belongs to.
func TestConverterKeepsATonesPitch(t *testing.T) {
	in, out := Format{44_100, 2}, Format{48_000, 2}
	const freq = 1_000.0
	converted := newConverter(in, out).convert(sinePCM(44_100, 2, freq, 0.5, 44_100, false))
	an, err := NewAnalyzer(out)
	if err != nil {
		t.Fatal(err)
	}
	var bands [32]float64
	for i := 0; i+1600*2 <= len(converted); i += 1600 * 2 {
		bands, _ = an.Process(converted[i : i+1600*2])
	}
	if got, want := peakBand(bands), expectedBand(freq); got != want {
		t.Fatalf("1 kHz at 44.1 kHz peaks in band %d after conversion, want %d", got, want)
	}
}

func equalF(a, b []float32) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
