package visualizer

// Onsets: the moments a drum hits, found per frequency group.
//
// The level bands cannot carry them. They are measured over an 85 ms Hann
// window once per 1/30 s, so a hit is spread over three analyses and the
// window's taper nearly silences the newest samples; the old kick (orb.go's
// bassKick) watches the bass LEVEL rise above its average, so it hears kicks
// only, and a bassline that sits loud and steady under them fills the gap it
// is looking for.
//
// This detector runs its own short window (1024 frames, ~21 ms at 48 kHz) every
// ~10 ms and measures SPECTRAL FLUX: for every FFT bin, how much its log
// magnitude rose since the last look, summed over a frequency group. A steady
// note, however loud, rises by nothing; a hit rises everywhere in its group at
// once. Each group's flux is compared with its own recent mean and spread, an
// onset is the local peak that clears that threshold, and a refractory gap
// stops one hit from counting twice.
//
// The UI samples ~30 times a second, so each group is exposed as a pulse that
// jumps on an onset and decays, not as a one-hop event it could miss.

import (
	"math"

	"gonum.org/v1/gonum/dsp/fourier"
)

// Onsets are the per-group hit pulses, each 0..1: 1 on the hop an onset is
// found, decaying over ~150 ms.
type Onsets struct {
	Kick  float64 // 40-130 Hz
	Snare float64 // 1-4 kHz, the snare's noise
	Hat   float64 // 7-16 kHz
}

const (
	onsetWindow     = 1_024
	onsetHopSeconds = 0.010
	onsetGroups     = 3

	onsetLogScale   = 1_000 // log(1 + scale*|X|): compresses loudness so flux follows change, not level
	onsetMemory     = 1.0   // seconds the threshold's mean and spread follow
	onsetThreshold  = 2.0   // spreads above the mean a peak must reach
	onsetFloor      = 0.02  // minimum flux per bin: silence and dither never fire
	onsetRefractory = 0.090 // seconds: no second onset in a group sooner
	onsetDecay      = 0.150 // seconds for a pulse to fall to 1/e
	onsetKickLag    = 15    // hops (~150 ms) of kick-band energy a new kick must rise above
)

var onsetBands = [onsetGroups][2]float64{{40, 130}, {1_000, 4_000}, {7_000, 16_000}}

type onsetDetector struct {
	sampleRate int
	channels   int

	ring    []float64 // mono mix
	write   int
	hann    []float64
	scratch []float64
	coeff   []complex128
	fft     *fourier.FFT
	group   []int // bin -> group, -1 outside every group
	bins    [onsetGroups]int

	prevLog []float64
	primed  bool
	silent  int // trailing all-zero frames; a full window of them skips the FFT, as the Analyzer does

	hop, toHop int
	kickLog    [onsetKickLag]float64 // the kick band's log energy over the last onsetKickLag hops (ring)
	kickAt     int
	flux       [onsetGroups][3]float64 // last three flux values, newest last
	mean, dev  [onsetGroups]float64
	since      [onsetGroups]int // hops since the group's last onset
	pulse      [onsetGroups]float64
}

func newOnsetDetector(format Format) *onsetDetector {
	d := &onsetDetector{
		sampleRate: format.SampleRate,
		channels:   format.Channels,
		ring:       make([]float64, onsetWindow),
		hann:       make([]float64, onsetWindow),
		scratch:    make([]float64, onsetWindow),
		coeff:      make([]complex128, onsetWindow/2+1),
		fft:        fourier.NewFFT(onsetWindow),
		group:      make([]int, onsetWindow/2+1),
		prevLog:    make([]float64, onsetWindow/2+1),
		hop:        max(1, int(onsetHopSeconds*float64(format.SampleRate))),
	}
	d.toHop = d.hop
	for i := range d.hann {
		d.hann[i] = 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/float64(onsetWindow-1))
	}
	for bin := range d.group {
		d.group[bin] = -1
		f := float64(bin) * float64(format.SampleRate) / onsetWindow
		for g, band := range onsetBands {
			if bin > 0 && f >= band[0] && f < band[1] {
				d.group[bin] = g
				d.bins[g]++
			}
		}
	}
	for g := range d.since {
		d.since[g] = math.MaxInt32
	}
	return d
}

// process folds interleaved PCM in; frames must be complete.
func (d *onsetDetector) process(interleaved []float32) {
	frames := len(interleaved) / d.channels
	for f := 0; f < frames; f++ {
		var sum float64
		for c := 0; c < d.channels; c++ {
			sum += float64(interleaved[f*d.channels+c])
		}
		if sum != 0 {
			d.silent = 0
		} else if d.silent < onsetWindow {
			d.silent++
		}
		d.ring[d.write] = sum / float64(d.channels)
		d.write++
		if d.write == onsetWindow {
			d.write = 0
		}
		d.toHop--
		if d.toHop == 0 {
			d.analyze()
			d.toHop = d.hop
		}
	}
}

func (d *onsetDetector) analyze() {
	if d.silent >= onsetWindow {
		// A paused player streams digital silence: nothing can rise, so skip
		// the FFT and let the pulses fade. The next sound compares against
		// silence, which is what it followed.
		dt := float64(d.hop) / float64(d.sampleRate)
		fade := math.Exp(-dt / onsetDecay)
		for g := range onsetGroups {
			d.pulse[g] *= fade
			d.since[g]++
			d.flux[g] = [3]float64{}
		}
		for bin := range d.prevLog {
			d.prevLog[bin] = 0
		}
		return
	}
	for i := range onsetWindow {
		j := d.write + i
		if j >= onsetWindow {
			j -= onsetWindow
		}
		d.scratch[i] = d.ring[j] * d.hann[i]
	}
	d.fft.Coefficients(d.coeff, d.scratch)

	var flux [onsetGroups]float64
	var kickEnergy float64
	for bin := 1; bin < len(d.coeff); bin++ {
		c := d.coeff[bin]
		mag := math.Hypot(real(c), imag(c)) / onsetWindow
		logMag := math.Log1p(onsetLogScale * mag)
		switch g := d.group[bin]; {
		case g == 0:
			kickEnergy += mag * mag
		case g > 0 && d.primed:
			if rise := logMag - d.prevLog[bin]; rise > 0 {
				flux[g] += rise
			}
		}
		d.prevLog[bin] = logMag
	}
	// The kick band is two or three bins wide at this window: a kick's falling
	// pitch walks between them and a held bass note's per-bin level swings with
	// its phase, and per-bin flux read both as hits. The band's TOTAL energy
	// ignores which bin holds it. And a loud master ducks the bassline under
	// each kick, so the bass swelling back afterwards is a rise too: the kick is
	// measured against the MOST energy of the last ~150 ms, which that recovery
	// never exceeds but the next real kick does.
	logKick := math.Log1p(onsetLogScale * onsetLogScale * kickEnergy)
	if d.primed {
		recent := d.kickLog[0]
		for _, v := range d.kickLog[1:] {
			recent = max(recent, v)
		}
		flux[0] = max(0, logKick-recent)
	}
	d.kickLog[d.kickAt] = logKick
	d.kickAt = (d.kickAt + 1) % onsetKickLag
	d.primed = true

	dt := float64(d.hop) / float64(d.sampleRate)
	follow := -math.Expm1(-dt / onsetMemory)
	fade := math.Exp(-dt / onsetDecay)
	refractory := int(math.Ceil(onsetRefractory / dt))
	var peak [onsetGroups]bool
	var z [onsetGroups]float64
	for g := range onsetGroups {
		f := flux[g]
		if g > 0 {
			f /= float64(max(1, d.bins[g])) // per bin, so groups compare
		}
		h := &d.flux[g]
		h[0], h[1], h[2] = h[1], h[2], f
		// The middle value is a peak if it beats both neighbours and clears the
		// threshold learned BEFORE it; one hop of latency buys the peak test.
		limit := max(d.mean[g]+onsetThreshold*d.dev[g], onsetFloor)
		peak[g] = h[1] > h[0] && h[1] >= h[2] && h[1] > limit
		z[g] = (h[1] - d.mean[g]) / max(d.dev[g], onsetFloor)
		d.mean[g] += (f - d.mean[g]) * follow
		d.dev[g] += (math.Abs(f-d.mean[g]) - d.dev[g]) * follow
	}
	// Hats and a kick's beater click reach into the snare's 1-4 kHz, and there
	// the snare group's quiet baseline makes them look big. A snare counts only
	// when its group jumped the most, each measured against its own history.
	// A kick's beater click is broadband too, so no snare on a kick's hop.
	if peak[1] && (z[1] < z[0] || z[1] < z[2] || peak[0] || d.since[0] < 2) {
		peak[1] = false
	}
	for g := range onsetGroups {
		d.pulse[g] *= fade
		d.since[g]++
		if peak[g] && d.since[g] > refractory {
			d.pulse[g] = 1
			d.since[g] = 0
		}
	}
}

func (d *onsetDetector) onsets() Onsets {
	return Onsets{Kick: d.pulse[0], Snare: d.pulse[1], Hat: d.pulse[2]}
}
