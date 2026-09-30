package main

// Measures the onset detector (visualizer/onset.go) against the old kick
// (orb.go's bassKick over bassLevel) on a synthetic drum track whose every hit
// time is known. Run with -v to see the table:
//
//	go test -run TestOnsetMeasure -v .

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
	"testing"

	"github.com/k1y0miiii/applemusic-tui/visualizer"
)

const (
	measRate = 48_000
	measBPM  = 120.0
	measSecs = 32.0
)

type hit struct {
	at   float64 // seconds
	kind int     // 0 kick, 1 snare, 2 hat
	vel  float64
}

// drumTrack builds the score: kicks on 1 and 3 (plus the "and" of 3 every
// other bar), snares on 2 and 4, closed hats on every eighth with a ghost
// accent pattern. Starts at 1 s so the detectors can settle.
func drumTrack(rng *rand.Rand) []hit {
	var hits []hit
	beat := 60 / measBPM
	for b := 0; ; b++ {
		t := 1 + float64(b)*beat
		if t > measSecs-1 {
			break
		}
		bar, pos := b/4, b%4
		if pos == 0 || pos == 2 {
			hits = append(hits, hit{t, 0, 0.8 + 0.2*rng.Float64()})
		}
		if pos == 2 && bar%2 == 1 {
			hits = append(hits, hit{t + beat/2, 0, 0.7 + 0.2*rng.Float64()})
		}
		if pos == 1 || pos == 3 {
			hits = append(hits, hit{t, 1, 0.8 + 0.2*rng.Float64()})
		}
		for e := 0; e < 2; e++ {
			v := 0.9
			if e == 1 {
				v = 0.45 // offbeat ghost
			}
			hits = append(hits, hit{t + float64(e)*beat/2, 2, v * (0.85 + 0.15*rng.Float64())})
		}
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].at < hits[j].at })
	return hits
}

// render mixes the drums over a loud sustained bassline (which drops in at
// 8 s: a level change, not a hit), a pad, and a soft clip like a loud master.
func render(hits []hit, rng *rand.Rand) []float32 {
	n := int(measSecs * measRate)
	mix := make([]float64, n)
	for _, h := range hits {
		start := int(h.at * measRate)
		var hpPrev, hpOut float64
		phase := 0.0
		for i := 0; i < int(0.4*measRate) && start+i < n; i++ {
			t := float64(i) / measRate
			var s float64
			switch h.kind {
			case 0: // pitch-swept sine plus a 2 ms click
				f := 50 + 100*math.Exp(-t/0.03)
				phase += 2 * math.Pi * f / measRate
				s = 0.8 * math.Sin(phase) * math.Exp(-t/0.12)
				if t < 0.002 {
					s += 0.3 * (rng.Float64()*2 - 1)
				}
			case 1: // 190 Hz body plus high-passed noise
				noise := rng.Float64()*2 - 1
				hpOut = 0.9 * (hpOut + noise - hpPrev)
				hpPrev = noise
				s = 0.25*math.Sin(2*math.Pi*190*t)*math.Exp(-t/0.06) + 0.45*hpOut*math.Exp(-t/0.09)
			case 2: // strongly high-passed noise
				noise := rng.Float64()*2 - 1
				hpOut = 0.25 * (hpOut + noise - hpPrev)
				hpPrev = noise
				s = 0.9 * hpOut * math.Exp(-t/0.03)
			}
			mix[start+i] += s * h.vel
		}
	}
	notes := []float64{55, 49, 65.4, 58.3}
	for i := range mix {
		t := float64(i) / measRate
		if t >= 8 { // the bass drops in: loud and sustained
			note := notes[int((t-8)/2)%len(notes)]
			mix[i] += 0.45 * math.Min(1, (t-8)/0.02) * math.Sin(2*math.Pi*note*t)
		}
		pad := 0.05 * (math.Sin(2*math.Pi*220*t) + math.Sin(2*math.Pi*277.2*t) + math.Sin(2*math.Pi*329.6*t))
		mix[i] += pad * (0.8 + 0.2*math.Sin(2*math.Pi*0.25*t))
	}
	out := make([]float32, 2*n)
	for i, v := range mix {
		v = math.Tanh(1.4 * v)
		out[2*i], out[2*i+1] = float32(v), float32(v)
	}
	return out
}

type score struct {
	tp, fp, fn int
	lat        []float64
}

func (s score) precision() float64 { return float64(s.tp) / math.Max(1, float64(s.tp+s.fp)) }
func (s score) recall() float64    { return float64(s.tp) / math.Max(1, float64(s.tp+s.fn)) }

func (s score) f1() float64 {
	p, r := s.precision(), s.recall()
	if p+r == 0 {
		return 0
	}
	return 2 * p * r / (p + r)
}

// latency is the mean delay of the found hits, in seconds.
func (s score) latency() float64 {
	if len(s.lat) == 0 {
		return math.NaN()
	}
	var sum float64
	for _, l := range s.lat {
		sum += l
	}
	return sum / float64(len(s.lat))
}

func (s score) String() string {
	return fmt.Sprintf("hits %3d  found %3d  missed %3d  false %3d  precision %.2f  recall %.2f  F1 %.2f  latency %4.0f ms",
		s.tp+s.fn, s.tp, s.fn, s.fp, s.precision(), s.recall(), s.f1(), s.latency()*1000)
}

// match pairs detections with the truth: a detection within [-20, +80] ms of a
// hit counts once; everything else is a false alarm.
func match(truth, found []float64) score {
	var s score
	used := make([]bool, len(found))
	for _, t := range truth {
		best := -1
		for j, f := range found {
			if !used[j] && f-t >= -0.020 && f-t <= 0.080 && (best < 0 || math.Abs(f-t) < math.Abs(found[best]-t)) {
				best = j
			}
		}
		if best < 0 {
			s.fn++
			continue
		}
		used[best] = true
		s.tp++
		s.lat = append(s.lat, found[best]-t)
	}
	for _, u := range used {
		if !u {
			s.fp++
		}
	}
	return s
}

func TestOnsetMeasure(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	hits := drumTrack(rng)
	pcm := render(hits, rng)

	an, err := visualizer.NewAnalyzer(visualizer.Format{SampleRate: measRate, Channels: 2})
	if err != nil {
		t.Fatal(err)
	}
	// WASAPI hands over ~10 ms packets; the UI samples ~30 times a second.
	const chunk = measRate / 100
	var newEv [3][]float64
	var prev visualizer.Onsets
	var oldEv []float64
	var base, kick, prevKick float64
	nextTick := 0
	for f := 0; f+chunk <= len(pcm)/2; f += chunk {
		bands, err := an.Process(pcm[2*f : 2*(f+chunk)])
		if err != nil {
			t.Fatal(err)
		}
		now := float64(f+chunk) / measRate
		o := an.Onsets()
		for g, pair := range [][2]float64{{o.Kick, prev.Kick}, {o.Snare, prev.Snare}, {o.Hat, prev.Hat}} {
			if pair[0] > pair[1]+1e-9 {
				newEv[g] = append(newEv[g], now)
			}
		}
		prev = o
		if f >= nextTick { // the old kick, at its own 1/30 s tick
			nextTick += measRate / 30
			base, kick = bassKick(base, kick, bassLevel(bands))
			if kick >= 0.4 && kick >= prevKick+0.25 {
				oldEv = append(oldEv, now)
			}
			prevKick = kick
		}
	}

	var truth [3][]float64
	for _, h := range hits {
		truth[h.kind] = append(truth[h.kind], h.at)
	}
	t.Logf("synthetic 120 BPM, %.0f s, loud bassline drops in at 8 s, soft-clipped master", measSecs)
	t.Logf("kick   OLD (bassKick) %s", match(truth[0], oldEv))
	t.Logf("kick   NEW (flux)     %s", match(truth[0], newEv[0]))
	t.Logf("snare  NEW (flux)     %s", match(truth[1], newEv[1]))
	t.Logf("hat    NEW (flux)     %s", match(truth[2], newEv[2]))

	// Floors a little under the measured values (kick F1 0.91 / 27 ms, snare
	// 0.82, hat 1.00 on 2026-09-30): a change that makes detection worse fails.
	oldKick := match(truth[0], oldEv)
	for g, floor := range []float64{0.85, 0.75, 0.95} {
		s := match(truth[g], newEv[g])
		if f1 := s.f1(); f1 < floor {
			t.Errorf("%s F1 %.2f, below the %.2f floor", []string{"kick", "snare", "hat"}[g], f1, floor)
		}
		if l := s.latency(); l > 0.040 {
			t.Errorf("%s latency %.0f ms, over 40 ms", []string{"kick", "snare", "hat"}[g], l*1000)
		}
	}
	if newKick := match(truth[0], newEv[0]); newKick.f1() <= oldKick.f1() {
		t.Errorf("new kick F1 %.2f does not beat the old bassKick's %.2f", newKick.f1(), oldKick.f1())
	}

	// Where the false alarms land: on another drum, on a bass note change
	// (every 2 s from 8 s), or nowhere in particular.
	var noteChanges []float64
	for c := 8.0; c < measSecs; c += 2 {
		noteChanges = append(noteChanges, c)
	}
	near := func(ts []float64, at float64) bool {
		for _, x := range ts {
			if at-x >= -0.020 && at-x <= 0.080 {
				return true
			}
		}
		return false
	}
	names := []string{"kick", "snare", "hat"}
	for g := range 3 {
		causes := map[string]int{}
		for _, at := range newEv[g] {
			if near(truth[g], at) {
				continue
			}
			var why []string
			for o := range 3 {
				if o != g && near(truth[o], at) {
					why = append(why, names[o])
				}
			}
			if near(noteChanges, at) {
				why = append(why, "bass-note")
			}
			if len(why) == 0 {
				why = []string{"nothing"}
			}
			causes[fmt.Sprint(why)]++
		}
		t.Logf("  %-5s false alarms by cause: %v", names[g], causes)
	}
	// Kick false alarms: offset from the nearest kick, in 20 ms buckets.
	offs := map[int]int{}
	for _, at := range newEv[0] {
		if near(truth[0], at) {
			continue
		}
		best := math.Inf(1)
		for _, k := range truth[0] {
			if math.Abs(at-k) < math.Abs(best) {
				best = at - k
			}
		}
		offs[int(math.Floor(best*1000/20))*20]++
	}
	t.Logf("  kick false alarms by ms from the nearest kick: %v", offs)
}
