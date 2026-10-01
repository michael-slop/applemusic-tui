//go:build windows

package visualizer

import (
	"errors"
	"fmt"
	"log"
	"math"
	"runtime"
	"time"
	"unsafe"

	"github.com/go-ole/go-ole"
	"github.com/moutend/go-wca/pkg/wca"
)

// Windows: capture what the default output device is playing through a
// WASAPI loopback stream (the Windows counterpart of a PulseAudio monitor).
// Pure Go via go-wca, so the Windows build still needs no cgo.
//
// The default device can change while amtui runs — headphones plugged in or
// connected — and the music follows it while a stream opened once does not:
// the visualizer went flat, listening to speakers nothing played through. So
// the capture checks the default device every second and reopens on a new
// one, and a device that disappears is reopened rather than ending the
// visualizer. The Source's Format stays what it opened with; a device with
// another rate or channel count is converted to it (convert.go), so nothing
// past this file sees the switch.

const (
	wasapiQueueDepth   = 8
	wasapiBuffer       = 200 * time.Millisecond
	wasapiPollInterval = 10 * time.Millisecond
	wasapiFollowEvery  = time.Second
	waveFormatFloat    = 0x0003
	waveFormatExt      = 0xFFFE
)

// endpoint is one open loopback stream.
type endpoint struct {
	id      string
	dev     *wca.IMMDevice
	client  *wca.IAudioClient
	capture *wca.IAudioCaptureClient
	format  Format
	isFloat bool
}

func (e *endpoint) close() {
	if e.client != nil {
		_ = e.client.Stop()
	}
	if e.capture != nil {
		e.capture.Release()
	}
	if e.client != nil {
		e.client.Release()
	}
	if e.dev != nil {
		e.dev.Release()
	}
}

// defaultID is the default render device's id, or "" when there is none.
func defaultID(enum *wca.IMMDeviceEnumerator) string {
	var dev *wca.IMMDevice
	if err := enum.GetDefaultAudioEndpoint(wca.ERender, wca.EConsole, &dev); err != nil {
		return ""
	}
	defer dev.Release()
	var id string
	if err := dev.GetId(&id); err != nil {
		return ""
	}
	return id
}

// openDefault starts a loopback stream on the current default render device.
func openDefault(enum *wca.IMMDeviceEnumerator) (*endpoint, error) {
	e := &endpoint{}
	ok := false
	defer func() {
		if !ok {
			e.close()
		}
	}()
	if err := enum.GetDefaultAudioEndpoint(wca.ERender, wca.EConsole, &e.dev); err != nil {
		return nil, fmt.Errorf("default output device: %w", err)
	}
	_ = e.dev.GetId(&e.id)
	if err := e.dev.Activate(wca.IID_IAudioClient, wca.CLSCTX_ALL, nil, &e.client); err != nil {
		return nil, fmt.Errorf("activate audio client: %w", err)
	}
	var wfx *wca.WAVEFORMATEX
	if err := e.client.GetMixFormat(&wfx); err != nil {
		return nil, fmt.Errorf("mix format: %w", err)
	}
	defer ole.CoTaskMemFree(uintptr(unsafe.Pointer(wfx)))
	e.isFloat = wfx.WBitsPerSample == 32 &&
		(wfx.WFormatTag == waveFormatFloat || wfx.WFormatTag == waveFormatExt)
	if !e.isFloat && wfx.WBitsPerSample != 16 {
		return nil, fmt.Errorf("unsupported mix format: tag %#x, %d bits", wfx.WFormatTag, wfx.WBitsPerSample)
	}
	e.format = Format{SampleRate: int(wfx.NSamplesPerSec), Channels: int(wfx.NChannels)}
	if err := e.client.Initialize(wca.AUDCLNT_SHAREMODE_SHARED, wca.AUDCLNT_STREAMFLAGS_LOOPBACK,
		wca.REFERENCE_TIME(wasapiBuffer/100), 0, wfx, nil); err != nil {
		return nil, fmt.Errorf("initialise loopback stream: %w", err)
	}
	if err := e.client.GetService(wca.IID_IAudioCaptureClient, &e.capture); err != nil {
		return nil, fmt.Errorf("capture service: %w", err)
	}
	if err := e.client.Start(); err != nil {
		return nil, fmt.Errorf("start loopback stream: %w", err)
	}
	ok = true
	return e, nil
}

// drain hands every waiting packet to emit, as float32 in the endpoint's own
// format. An error means the stream is gone (usually the device was removed).
func (e *endpoint) drain(buf *[]float32, emit func([]float32)) error {
	for {
		var packet uint32
		if err := e.capture.GetNextPacketSize(&packet); err != nil {
			return fmt.Errorf("loopback packet size: %w", err)
		}
		if packet == 0 {
			return nil
		}
		var data *byte
		var frames, flags uint32
		var devPos, qpcPos uint64
		if err := e.capture.GetBuffer(&data, &frames, &flags, &devPos, &qpcPos); err != nil {
			return fmt.Errorf("loopback buffer: %w", err)
		}
		n := int(frames) * e.format.Channels
		s := (*buf)[:0]
		switch {
		case flags&wca.AUDCLNT_BUFFERFLAGS_SILENT != 0 || data == nil:
			s = append(s, make([]float32, n)...)
		case e.isFloat:
			s = append(s, unsafe.Slice((*float32)(unsafe.Pointer(data)), n)...)
		default:
			for _, v := range unsafe.Slice((*int16)(unsafe.Pointer(data)), n) {
				s = append(s, float32(v)/math.MaxInt16)
			}
		}
		_ = e.capture.ReleaseBuffer(frames)
		*buf = s
		emit(s)
	}
}

func openSystemSource() (Source, error) {
	type opened struct {
		format Format
		err    error
	}
	ready := make(chan opened, 1)
	stop := make(chan struct{})
	done := make(chan struct{})
	var source *pcmSource
	sourceSet := make(chan struct{})

	// COM objects live on the thread that created them: one locked goroutine
	// owns the whole capture from initialisation to release.
	go func() {
		defer close(done)
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		fail := func(err error) { ready <- opened{err: err} }
		if err := ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED); err != nil {
			var oleErr *ole.OleError
			// S_FALSE: already initialised on this thread — fine.
			if !errors.As(err, &oleErr) || oleErr.Code() != 1 {
				fail(fmt.Errorf("initialise COM: %w", err))
				return
			}
		}
		defer ole.CoUninitialize()

		var enum *wca.IMMDeviceEnumerator
		if err := wca.CoCreateInstance(wca.CLSID_MMDeviceEnumerator, 0, wca.CLSCTX_ALL,
			wca.IID_IMMDeviceEnumerator, &enum); err != nil {
			fail(fmt.Errorf("create device enumerator: %w", err))
			return
		}
		defer enum.Release()

		cur, err := openDefault(enum)
		if err != nil {
			fail(err)
			return
		}
		defer func() {
			if cur != nil {
				cur.close()
			}
		}()
		out := cur.format // what the Source promised; every device is converted to it

		ready <- opened{format: out}
		<-sourceSet
		if source == nil {
			return
		}

		var raw []float32
		conv := newConverter(cur.format, out)
		emit := func(s []float32) { _, _ = source.write(conv.convert(s)) }
		ticker := time.NewTicker(wasapiPollInterval)
		defer ticker.Stop()
		lastFollow := time.Now()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
			}
			if cur != nil {
				if err := cur.drain(&raw, emit); err != nil {
					log.Printf("visualizer: %s lost (%v); following the default device", cur.id, err)
					cur.close()
					cur = nil
				}
			}
			if time.Since(lastFollow) < wasapiFollowEvery {
				continue
			}
			lastFollow = time.Now()
			if id := defaultID(enum); cur == nil || (id != "" && id != cur.id) {
				if cur != nil {
					cur.close()
					cur = nil
				}
				next, err := openDefault(enum)
				if err != nil {
					log.Printf("visualizer: cannot open the default output yet: %v", err)
					continue
				}
				log.Printf("visualizer: now capturing %s (%d Hz x %d)", next.id, next.format.SampleRate, next.format.Channels)
				cur, conv = next, newConverter(next.format, out)
			}
		}
	}()

	res := <-ready
	if res.err != nil {
		close(sourceSet)
		<-done
		return nil, res.err
	}
	if err := validateSourceFormat(res.format); err != nil {
		close(sourceSet)
		<-done
		return nil, fmt.Errorf("WASAPI loopback format: %w", err)
	}
	var err error
	source, err = newPCMSource("WASAPI", res.format, wasapiQueueDepth, func() error {
		select {
		case <-stop:
		default:
			close(stop)
		}
		<-done
		return nil
	})
	close(sourceSet)
	if err != nil {
		close(stop)
		<-done
		return nil, err
	}
	return source, nil
}
