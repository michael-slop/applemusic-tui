// wasapiprobe opens the visualizer's system audio source for a few seconds and
// reports what it captured. Used to check the Windows loopback backend.
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
	f := src.Format()
	fmt.Printf("source %s: %d Hz, %d ch\n", src.Name(), f.SampleRate, f.Channels)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	buf := make([]float32, 4096)
	total, peak := 0, 0.0
	for {
		n, err := src.Read(ctx, buf)
		total += n
		for _, v := range buf[:n] {
			peak = math.Max(peak, math.Abs(float64(v)))
		}
		if err != nil || ctx.Err() != nil {
			break
		}
	}
	fmt.Printf("captured %d samples (%.1f s of audio), peak %.4f\n", total, float64(total)/float64(f.SampleRate*f.Channels), peak)
}
