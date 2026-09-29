// bandprobe listens to system audio through the visualizer's own source and
// analyzer for a few seconds and reports how much each band actually moves.
package main

import (
	"context"
	"fmt"
	"math"
	"os"
	"time"

	"github.com/k1y0miiii/applemusic-tui/visualizer"
)

func main() {
	src, err := visualizer.OpenSystemSource()
	if err != nil {
		fmt.Println("open:", err)
		os.Exit(1)
	}
	defer src.Close()
	an, err := visualizer.NewAnalyzer(src.Format())
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	buf := make([]float32, 2048)
	var lo, hi, sum, sq [32]float64
	for i := range lo {
		lo[i] = 1
	}
	n := 0
	warm := time.Now().Add(1500 * time.Millisecond)
	for ctx.Err() == nil {
		k, err := src.Read(ctx, buf)
		if err != nil {
			break
		}
		b, err := an.Process(buf[:k])
		if err != nil {
			break
		}
		if time.Now().Before(warm) {
			continue
		}
		for i, v := range b {
			lo[i], hi[i] = math.Min(lo[i], v), math.Max(hi[i], v)
			sum[i] += v
			sq[i] += v * v
		}
		n++
	}
	if n == 0 {
		fmt.Println("no audio captured")
		return
	}
	fmt.Println("band   Hz-ish   mean   min    max   typical wobble (std dev)")
	for _, i := range []int{0, 2, 4, 6, 8, 12, 16, 20, 24, 28, 31} {
		m := sum[i] / float64(n)
		hz := []string{"25", "", "60", "", "130", "", "250", "", "~400", "", "", "", "~1k", "", "", "", "~2.2k", "", "", "", "~4.7k", "", "", "", "~10k", "", "", "", "~13k", "", "", "16k"}[i]
		fmt.Printf("%4d  %6s   %.2f   %.2f   %.2f   %.3f\n", i, hz, m, lo[i], hi[i], math.Sqrt(math.Max(0, sq[i]/float64(n)-m*m)))
	}
	// what the torus does with it: tube radius = 1.0 * (0.55 + 0.85*band)
	var mean float64
	for i := range sum {
		mean += sum[i] / float64(n)
	}
	mean /= 32
	fmt.Printf("average band %.2f -> average tube radius %.2f (silence 0.55, max 1.40)\n", mean, 0.55+0.85*mean)
}
