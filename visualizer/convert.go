package visualizer

// converter turns interleaved float32 PCM from one Format into another, so a
// capture that changes device mid-run (source_windows.go follows the default
// output) can keep the Format its Source promised. Channels are mapped (mono
// is duplicated, extra channels beyond the front pair dropped); the rate is
// resampled by linear interpolation, carrying the position across chunks.
// That is plenty for a spectrum: nothing here is ever played back.
type converter struct {
	in, out Format
	step    float64   // input frames per output frame
	pos     float64   // where the next output frame sits, in frames after prev
	prev    []float32 // last input frame of the previous chunk (out's channels)
	have    bool      // prev holds a frame
	buf     []float32
}

func newConverter(in, out Format) *converter {
	return &converter{in: in, out: out,
		step: float64(in.SampleRate) / float64(out.SampleRate),
		prev: make([]float32, out.Channels)}
}

// convert returns samples in the out Format. With equal formats it returns
// the input unchanged. The result is reused by the next call.
func (c *converter) convert(s []float32) []float32 {
	if c.in == c.out {
		return s
	}
	inCh, outCh := c.in.Channels, c.out.Channels
	frames := len(s) / inCh
	// Map channels first: frame i of the input as out's channels.
	mapped := c.buf[:0]
	for f := 0; f < frames; f++ {
		frame := s[f*inCh : (f+1)*inCh]
		for o := 0; o < outCh; o++ {
			mapped = append(mapped, frame[min(o, inCh-1)])
		}
	}
	if c.in.SampleRate == c.out.SampleRate {
		c.buf = mapped
		return mapped
	}
	// Resample over S = [prev] + mapped (prev only once a chunk has been
	// seen): output frames sit at positions p in S, stepping by in/out rate,
	// each a blend of the two frames around it. The last frame of S becomes
	// the next chunk's prev, so p carries over relative to it.
	n := frames
	if c.have {
		n++
	}
	get := func(k, ch int) float32 {
		if c.have {
			if k == 0 {
				return c.prev[ch]
			}
			k--
		}
		return mapped[k*outCh+ch]
	}
	start := len(mapped)
	res := mapped
	p := c.pos
	for ; n > 0 && p <= float64(n-1); p += c.step {
		k := int(p)
		t := float32(p - float64(k))
		for ch := 0; ch < outCh; ch++ {
			a := get(k, ch)
			b := a
			if k+1 < n {
				b = get(k+1, ch)
			}
			res = append(res, a+(b-a)*t)
		}
	}
	if n > 0 {
		for ch := 0; ch < outCh; ch++ {
			c.prev[ch] = get(n-1, ch)
		}
		c.pos = p - float64(n-1)
		c.have = true
	}
	out := res[start:]
	c.buf = res
	return out
}
