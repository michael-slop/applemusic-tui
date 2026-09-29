// fxdemo runs one fx effect full-screen, driven by live system audio, for a
// quick visual/CPU check outside amtui:  go run ./bench/fxdemo flames 80 24 10
package main

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"time"

	"github.com/muesli/termenv"

	"github.com/k1y0miiii/applemusic-tui/fx"
	"github.com/k1y0miiii/applemusic-tui/visualizer"
)

func main() {
	name, cols, rows, secs := os.Args[1], atoi(os.Args[2]), atoi(os.Args[3]), atoi(os.Args[4])
	reactivity := 0.5
	if len(os.Args) > 5 {
		reactivity = float64(atoi(os.Args[5])) / 100
	}
	e := fx.New(name)
	if e == nil {
		fmt.Println("unknown effect", name, fx.Names())
		os.Exit(1)
	}
	e.SetPalette(fx.Palette{
		Background: rgb(5, 7, 10),
		Ramp:       [5]fx.RGB{rgb(0x1A, 0x24, 0x30), rgb(0x3D, 0x8F, 0xA8), rgb(0x62, 0xE6, 0x70), rgb(0xC8, 0xFF, 0xD0), rgb(0xD8, 0xD4, 0xC4)},
		Accent:     rgb(0x62, 0xE6, 0x70), AccentHi: rgb(0xC8, 0xFF, 0xD0), AccentLo: rgb(0x3D, 0x8F, 0xA8),
		Text: rgb(0xD8, 0xD4, 0xC4), Dim: rgb(0xAC, 0xA4, 0xC8), Faint: rgb(0x5C, 0x64, 0x70),
	})
	e.Resize(cols, rows)
	src, err := visualizer.OpenSystemSource()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer src.Close()
	svc, _ := visualizer.NewService(src)
	defer svc.Close()
	var mean, dev, react [32]float64
	var kick, base float64
	primed := false
	fmt.Print("\x1b[2J\x1b[?25l")
	defer fmt.Print("\x1b[?25h")
	t0 := time.Now()
	tick := time.NewTicker(time.Second / 30)
	for time.Since(t0) < time.Duration(secs)*time.Second {
		<-tick.C
		f, _ := svc.Latest()
		b := f.Bands
		if !primed {
			mean, primed = b, true
		}
		for i, v := range b {
			d := v - mean[i]
			mean[i] += d / 60
			dev[i] += (math.Abs(d) - dev[i]) / 60
			react[i] = math.Min(1, math.Max(0, 0.5+d/(2.5*math.Max(dev[i], 0.015))))
		}
		bass := (b[0] + b[1] + b[2] + b[3]) / 4
		gap := bass - base
		base += gap * 0.03
		kick *= 0.85
		if o := 9 * (bass - base - 0.02); o > kick {
			kick = math.Min(1, o)
		}
		avg := func(lo, hi int) float64 {
			s := 0.0
			for i := lo; i <= hi; i++ {
				s += react[i]
			}
			return s / float64(hi-lo+1)
		}
		k := 2 * reactivity // the same scaling amtui applies (react.go)
		var rs [32]float64
		for i := range react {
			rs[i] = math.Min(1, math.Max(0, 0.5+(react[i]-0.5)*k))
		}
		sa := func(lo, hi int) float64 {
			s := 0.0
			for i := lo; i <= hi; i++ {
				s += rs[i]
			}
			return s / float64(hi-lo+1)
		}
		_ = avg
		e.Step(fx.Audio{Bands: b, React: rs, Bass: sa(0, 5), Mid: sa(6, 19), Treble: sa(20, 31), Kick: math.Min(1, kick*k), Playing: true, Reactivity: reactivity, DT: 1.0 / 30})
		fmt.Print("\x1b[H" + fx.Render(e, cols, rows, termenv.TrueColor))
	}
}

func atoi(s string) int { n, _ := strconv.Atoi(s); return n }

func rgb(r, g, b uint8) fx.RGB { return fx.RGB{R: r, G: g, B: b} }
