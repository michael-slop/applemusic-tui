// onsetprobe records system audio through the visualizer's own source, then
// runs the onset detector (visualizer/onset.go) and the old bass kick over it.
// Real music has no answer sheet, so it reports what needs none: hits per
// minute, the tempo each detector's kicks imply (compare with the song's real
// BPM), and a timeline to read along with the song.
//
//	go run ./bench/onsetprobe record 30 song.pcm   # 30 s of what is playing
//	go run ./bench/onsetprobe analyze song.pcm
package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/k1y0miiii/applemusic-tui/visualizer"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Println("usage: onsetprobe record <secs> <file> | analyze <file>")
		os.Exit(2)
	}
	switch os.Args[1] {
	case "record":
		secs, _ := strconv.Atoi(os.Args[2])
		record(secs, os.Args[3])
	case "analyze":
		analyze(os.Args[2])
	}
}

// The file is a 12-byte header (rate, channels, frames as uint32) followed by
// interleaved float32 samples, little endian.
func record(secs int, path string) {
	src, err := visualizer.OpenSystemSource()
	if err != nil {
		fmt.Println("open:", err)
		os.Exit(1)
	}
	defer src.Close()
	format := src.Format()
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(secs)*time.Second)
	defer cancel()
	var pcm []float32
	buf := make([]float32, 4096)
	for ctx.Err() == nil {
		n, err := src.Read(ctx, buf)
		pcm = append(pcm, buf[:n]...)
		if err != nil {
			break
		}
	}
	f, err := os.Create(path)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer f.Close()
	frames := len(pcm) / format.Channels
	binary.Write(f, binary.LittleEndian, [3]uint32{uint32(format.SampleRate), uint32(format.Channels), uint32(frames)})
	binary.Write(f, binary.LittleEndian, pcm[:frames*format.Channels])
	var peak float64
	for _, v := range pcm {
		peak = math.Max(peak, math.Abs(float64(v)))
	}
	fmt.Printf("recorded %.1f s, %d Hz x %d, peak %.3f -> %s\n",
		float64(frames)/float64(format.SampleRate), format.SampleRate, format.Channels, peak, path)
}

func analyze(path string) {
	f, err := os.Open(path)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	var hdr [3]uint32
	binary.Read(f, binary.LittleEndian, &hdr)
	rate, ch, frames := int(hdr[0]), int(hdr[1]), int(hdr[2])
	pcm := make([]float32, frames*ch)
	binary.Read(f, binary.LittleEndian, pcm)
	f.Close()

	an, err := visualizer.NewAnalyzer(visualizer.Format{SampleRate: rate, Channels: ch})
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	chunk := rate / 100
	var ev [4][]float64 // kick, snare, hat, old kick
	var prev visualizer.Onsets
	var base, kick, prevKick float64
	nextTick := 0
	for fr := 0; fr+chunk <= frames; fr += chunk {
		bands, _ := an.Process(pcm[fr*ch : (fr+chunk)*ch])
		now := float64(fr+chunk) / float64(rate)
		o := an.Onsets()
		for g, p := range [][2]float64{{o.Kick, prev.Kick}, {o.Snare, prev.Snare}, {o.Hat, prev.Hat}} {
			if p[0] > p[1]+1e-9 {
				ev[g] = append(ev[g], now)
			}
		}
		prev = o
		if fr >= nextTick {
			nextTick += rate / 30
			base, kick = oldBassKick(base, kick, (bands[0]+bands[1]+bands[2]+bands[3])/4)
			if kick >= 0.4 && kick >= prevKick+0.25 {
				ev[3] = append(ev[3], now)
			}
			prevKick = kick
		}
	}
	secs := float64(frames) / float64(rate)
	fmt.Printf("%s: %.1f s\n", path, secs)
	for i, name := range []string{"kick  NEW", "snare NEW", "hat   NEW", "kick  OLD"} {
		_, strength := tempo(ev[i], secs)
		med, spread, share := spacing(ev[i])
		fmt.Printf("  %s  %4d hits  %5.1f/min  periodicity %.2f  gap median %3.0f ms (=%5.1f BPM)  spread %3.0f ms  within 30 ms of median %3.0f%%\n",
			name, len(ev[i]), float64(len(ev[i]))/secs*60, strength, med*1000, 60/math.Max(med, 1e-9), spread*1000, share*100)
	}
	fmt.Println("  timeline, 50 ms per column (K kick, S snare, H hat, k old kick):")
	for row := 0.0; row < math.Min(secs, 12); row += 4 {
		var lines [4]strings.Builder
		for c := row; c < row+4; c += 0.05 {
			for i, sym := range []string{"K", "S", "H", "k"} {
				mark := "."
				for _, t := range ev[i] {
					if t >= c && t < c+0.05 {
						mark = sym
					}
				}
				lines[i].WriteString(mark)
			}
		}
		fmt.Printf("  %4.0fs\n", row)
		for i := range lines {
			fmt.Printf("        %s\n", lines[i].String())
		}
	}
}

// tempo autocorrelates the onset train (10 ms bins) over lags of 60-200 BPM and
// returns the best tempo and how strongly the train repeats at it (0..1).
func tempo(events []float64, secs float64) (float64, float64) {
	n := int(secs * 100)
	if len(events) < 4 || n < 200 {
		return 0, 0
	}
	train := make([]float64, n)
	for _, t := range events {
		if i := int(t * 100); i < n {
			train[i] = 1
			if i+1 < n {
				train[i+1] = 0.5 // one bin of slack for jitter
			}
		}
	}
	var zero float64
	for _, v := range train {
		zero += v * v
	}
	best, bestLag := 0.0, 0
	for lag := 30; lag <= 100; lag++ { // 200 .. 60 BPM
		var s float64
		for i := 0; i+lag < n; i++ {
			s += train[i] * train[i+lag]
		}
		if s > best {
			best, bestLag = s, lag
		}
	}
	if bestLag == 0 {
		return 0, 0
	}
	return 6000 / float64(bestLag), best / zero
}

// spacing returns the median gap between hits, the median distance of a gap
// from that median, and the share of gaps within 30 ms of it: on a steady beat
// a good detector's gaps all sit on one value.
func spacing(events []float64) (median, spread, share float64) {
	if len(events) < 3 {
		return 0, 0, 0
	}
	gaps := make([]float64, 0, len(events)-1)
	for i := 1; i < len(events); i++ {
		gaps = append(gaps, events[i]-events[i-1])
	}
	med := func(xs []float64) float64 {
		s := append([]float64(nil), xs...)
		sort.Float64s(s)
		return s[len(s)/2]
	}
	median = med(gaps)
	dev := make([]float64, len(gaps))
	in := 0
	for i, g := range gaps {
		dev[i] = math.Abs(g - median)
		if dev[i] <= 0.030 {
			in++
		}
	}
	return median, med(dev), float64(in) / float64(len(gaps))
}

// oldBassKick is orb.go's bassKick, copied because main is not importable; kept
// in step by hand (this is a measuring tool).
func oldBassKick(baseline, kick, bass float64) (float64, float64) {
	gap := bass - baseline
	memory := 0.03
	if gap > 0.25 || gap < 0 {
		memory = 0.25
	}
	baseline += gap * memory
	kick *= 0.85
	if over := 9.0 * (bass - baseline - 0.02); over > kick {
		kick = math.Min(1, over)
	}
	return baseline, math.Max(0, kick)
}
