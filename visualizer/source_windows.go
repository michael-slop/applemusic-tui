//go:build windows

package visualizer

import (
	"errors"
	"fmt"
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

const (
	wasapiQueueDepth   = 8
	wasapiBuffer       = 200 * time.Millisecond
	wasapiPollInterval = 10 * time.Millisecond
	waveFormatFloat    = 0x0003
	waveFormatExt      = 0xFFFE
)

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

		var dev *wca.IMMDevice
		if err := enum.GetDefaultAudioEndpoint(wca.ERender, wca.EConsole, &dev); err != nil {
			fail(fmt.Errorf("default output device: %w", err))
			return
		}
		defer dev.Release()

		var client *wca.IAudioClient
		if err := dev.Activate(wca.IID_IAudioClient, wca.CLSCTX_ALL, nil, &client); err != nil {
			fail(fmt.Errorf("activate audio client: %w", err))
			return
		}
		defer client.Release()

		var wfx *wca.WAVEFORMATEX
		if err := client.GetMixFormat(&wfx); err != nil {
			fail(fmt.Errorf("mix format: %w", err))
			return
		}
		defer ole.CoTaskMemFree(uintptr(unsafe.Pointer(wfx)))

		isFloat := wfx.WBitsPerSample == 32 &&
			(wfx.WFormatTag == waveFormatFloat || wfx.WFormatTag == waveFormatExt)
		isPCM16 := wfx.WBitsPerSample == 16
		if !isFloat && !isPCM16 {
			fail(fmt.Errorf("unsupported mix format: tag %#x, %d bits", wfx.WFormatTag, wfx.WBitsPerSample))
			return
		}
		channels := int(wfx.NChannels)
		format := Format{SampleRate: int(wfx.NSamplesPerSec), Channels: channels}

		if err := client.Initialize(wca.AUDCLNT_SHAREMODE_SHARED, wca.AUDCLNT_STREAMFLAGS_LOOPBACK,
			wca.REFERENCE_TIME(wasapiBuffer/100), 0, wfx, nil); err != nil {
			fail(fmt.Errorf("initialise loopback stream: %w", err))
			return
		}
		var capture *wca.IAudioCaptureClient
		if err := client.GetService(wca.IID_IAudioCaptureClient, &capture); err != nil {
			fail(fmt.Errorf("capture service: %w", err))
			return
		}
		defer capture.Release()
		if err := client.Start(); err != nil {
			fail(fmt.Errorf("start loopback stream: %w", err))
			return
		}
		defer client.Stop()

		ready <- opened{format: format}
		<-sourceSet
		if source == nil {
			return
		}

		var samples []float32
		ticker := time.NewTicker(wasapiPollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
			}
			for {
				var packet uint32
				if err := capture.GetNextPacketSize(&packet); err != nil {
					source.fail(fmt.Errorf("loopback packet size: %w", err))
					return
				}
				if packet == 0 {
					break
				}
				var data *byte
				var frames, flags uint32
				var devPos, qpcPos uint64
				if err := capture.GetBuffer(&data, &frames, &flags, &devPos, &qpcPos); err != nil {
					source.fail(fmt.Errorf("loopback buffer: %w", err))
					return
				}
				n := int(frames) * channels
				samples = samples[:0]
				switch {
				case flags&wca.AUDCLNT_BUFFERFLAGS_SILENT != 0 || data == nil:
					samples = append(samples, make([]float32, n)...)
				case isFloat:
					samples = append(samples, unsafe.Slice((*float32)(unsafe.Pointer(data)), n)...)
				default:
					for _, v := range unsafe.Slice((*int16)(unsafe.Pointer(data)), n) {
						samples = append(samples, float32(v)/math.MaxInt16)
					}
				}
				_ = capture.ReleaseBuffer(frames)
				_, _ = source.write(samples)
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
