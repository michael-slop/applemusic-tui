package fx

import (
	"math"
	"testing"
)

func builtWaves(cols, rows int) *waves {
	w := newWaves().(*waves)
	w.SetPalette(testPalette)
	w.Resize(cols, rows)
	return w
}

// wavFlat is a playing frame with every band at v.
func wavFlat(v float64) Audio {
	a := silence()
	a.Playing = true
	for i := range a.React {
		a.React[i] = v
	}
	return a
}

// peak is the column where ridge j rises highest (smallest y) and that y.
func (w *waves) peak(j int) (col int, y float64) {
	y = math.Inf(1)
	for c := range w.cols {
		if v := w.ys[j*w.cols+c]; v < y {
			col, y = c, v
		}
	}
	return col, y
}

// A spike in band k raises the nearest ridge at column ~k/31 of the panel:
// bass at the left, treble at the right, not mirrored.
func TestWavesBandRaisesItsColumn(t *testing.T) {
	const cols = 93
	for _, k := range []int{2, 12, 29} {
		w := builtWaves(cols, 24)
		for range 40 {
			w.Step(wavFlat(0.2))
		}
		a := wavFlat(0.2)
		a.React[k] = 1
		w.Step(a)
		c, _ := w.peak(0)
		want := float64(k) / 31 * cols
		if math.Abs(float64(c)+0.5-want) > 2.5 {
			t.Errorf("band %d peaked at column %d, want ~%.0f", k, c, want)
		}
	}
}

// Depth is time: a peak moves up the stack onto farther, smaller ridges as
// the music plays on.
func TestWavesThePastRecedes(t *testing.T) {
	w := builtWaves(64, 30)
	for range 40 {
		w.Step(wavFlat(0.1))
	}
	a := wavFlat(0.1)
	a.React[10] = 1
	for range 3 {
		w.Step(a)
	}
	// Which ridge carries the peak, and how tall it is above its own line.
	carrier := func() (ridge int, height float64) {
		for j := range w.n {
			lo, hi := math.Inf(1), math.Inf(-1)
			for c := range w.cols {
				if y := w.ys[j*w.cols+c]; !math.IsNaN(y) {
					lo, hi = min(lo, y), max(hi, y)
				}
			}
			if hi-lo > height {
				ridge, height = j, hi-lo
			}
		}
		return ridge, height
	}
	prevJ, prevH := carrier()
	if prevJ > 1 {
		t.Fatalf("the peak should start on the nearest ridges, got ridge %d", prevJ)
	}
	for step := range 4 {
		for range 6 { // 0.2 s
			w.Step(wavFlat(0.1))
		}
		j, h := carrier()
		if j <= prevJ || h >= prevH {
			t.Fatalf("step %d: the peak should recede (ridge %d -> %d, height %.2f -> %.2f)", step, prevJ, j, prevH, h)
		}
		prevJ, prevH = j, h
	}
}

// Painter's order: a tall nearer ridge hides the farther ridges behind it,
// and only there.
func TestWavesNearerRidgesOccludeFartherOnes(t *testing.T) {
	const cols, rows = 64, 30
	w := builtWaves(cols, rows)
	for range 40 {
		w.Step(wavFlat(0))
	}
	a := wavFlat(0)
	for k := 12; k <= 20; k++ { // a broad, flat-topped mountain mid-panel
		a.React[k] = 1
	}
	w.Step(a)
	c, y := w.peak(0)
	top := int(math.Floor(y))
	for r := top + 1; r < rows; r++ {
		if _, _, ok := w.Cell(c, r); ok {
			t.Fatalf("row %d under the nearest ridge's peak (row %d) should be hidden", r, top)
		}
	}
	// Away from the mountain the farther ridges are all still there.
	lit := 0
	for r := range rows {
		if _, _, ok := w.Cell(cols/5, r); ok {
			lit++
		}
	}
	if lit < w.n/2 {
		t.Fatalf("a quiet column should show the stack of ridges: %d lit of %d ridges", lit, w.n)
	}
}

// Silence is a stack of flat lines: every ridge lies level, one row each.
func TestWavesSilenceIsFlatLines(t *testing.T) {
	w := builtWaves(50, 20)
	for range 60 {
		w.Step(wavFlat(0))
	}
	for j := range w.n {
		first := math.NaN()
		for c := range w.cols {
			y := w.ys[j*w.cols+c]
			if math.IsNaN(y) {
				continue
			}
			if math.IsNaN(first) {
				first = y
			} else if y != first {
				t.Fatalf("ridge %d is not flat in silence: %v vs %v", j, y, first)
			}
		}
	}
	if litWeight(w, 50, 20) == 0 {
		t.Fatal("silence should leave the resting lines, not a blank panel")
	}
}

// Kick flares the nearest ridge only.
func TestWavesKickFlaresTheNearestRidge(t *testing.T) {
	calm, hit := builtWaves(60, 24), builtWaves(60, 24)
	for range 40 {
		calm.Step(wavFlat(0.6))
		hit.Step(wavFlat(0.6))
	}
	a := wavFlat(0.6)
	a.Kick = 1
	for range 3 {
		calm.Step(wavFlat(0.6))
		hit.Step(a)
	}
	_, yc := calm.peak(0)
	_, yh := hit.peak(0)
	if yh >= yc {
		t.Fatalf("a kick should lift the nearest ridge: y %.2f vs %.2f", yh, yc)
	}
	for j := 1; j < calm.n; j++ {
		_, a := calm.peak(j)
		_, b := hit.peak(j)
		if a != b {
			t.Fatalf("a kick moved ridge %d", j)
		}
	}
}

// The landscape rolls back on music time, not frames.
func TestWavesIsPacedByTimeNotFrameCount(t *testing.T) {
	fast, slow := builtWaves(30, 12), builtWaves(30, 12)
	for i := range 50 {
		a := loud(i / 5)
		a.Kick, a.DT = 0, 0.02
		fast.Step(a)
		if i%5 == 4 {
			a.DT = 0.1
			slow.Step(a)
		}
	}
	if frame(fast, 30, 12) != frame(slow, 30, 12) {
		t.Fatal("the same second of music drew different landscapes at different frame rates")
	}
}

func TestWavesStepDoesNotAllocate(t *testing.T) {
	w := builtWaves(80, 24)
	i := 0
	if n := testing.AllocsPerRun(50, func() { w.Step(loud(i)); i++ }); n != 0 {
		t.Fatalf("Step allocates %v times", n)
	}
}
