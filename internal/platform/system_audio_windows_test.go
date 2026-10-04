//go:build windows

package platform

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestSystemAudioWindowsActivationABI(t *testing.T) {
	if unsafe.Sizeof(uintptr(0)) != 8 {
		t.Skip("64-bit ABI only")
	}
	var variant systemAudioPropVariant
	var params systemAudioActivationParams
	if unsafe.Sizeof(variant) != 24 || unsafe.Offsetof(variant.BlobSize) != 8 || unsafe.Offsetof(variant.BlobData) != 16 || unsafe.Sizeof(params) != 12 {
		t.Fatal("activation BLOB ABI mismatch")
	}
	format := systemAudioWaveFormat()
	if len(format) != 18 || binary.LittleEndian.Uint16(format[0:2]) != 1 || binary.LittleEndian.Uint16(format[2:4]) != 2 || binary.LittleEndian.Uint32(format[4:8]) != 48000 || binary.LittleEndian.Uint32(format[8:12]) != 192000 || binary.LittleEndian.Uint16(format[12:14]) != 4 || binary.LittleEndian.Uint16(format[14:16]) != 16 || binary.LittleEndian.Uint16(format[16:18]) != 0 {
		t.Fatal("PCM16 stereo 48000 format ABI mismatch", format)
	}
	for _, failure := range []string{"none", "outer", "inner"} {
		t.Run(failure, func(t *testing.T) {
			var unknownTable, operationTable [32]uintptr
			unknown := &audioCOM{vtable: &unknownTable}
			unknownTable[0] = syscall.NewCallback(func(_ uintptr, iid *windows.GUID, output **audioCOM) uintptr {
				if *iid != systemAudioClientIID {
					t.Error("incorrect IAudioClient IID")
				}
				*output = unknown
				return 0
			})
			unknownTable[2] = syscall.NewCallback(func(_ uintptr) uintptr { return 0 })
			operationTable[3] = syscall.NewCallback(func(_ uintptr, status *uint32, output **audioCOM) uintptr {
				*status = 0
				*output = unknown
				if failure == "inner" {
					*status = 0x80004005
				}
				if failure == "outer" {
					return 0x80004005
				}
				return 0
			})
			callback := newSystemAudioActivation()
			defer systemAudioReleaseRef(callback)
			operation := &audioCOM{vtable: &operationTable}
			status := audioCall((*audioCOM)(unsafe.Pointer(callback)), 3, uintptr(unsafe.Pointer(operation)))
			result := <-callback.completed
			if status != 0 || (result.err != nil) != (failure != "none") || (result.client != nil) != (failure == "none") {
				t.Fatal("activation HRESULT handling failed", result.err)
			}
			for _, iid := range []windows.GUID{systemAudioUnknownIID, systemAudioCompletionIID, systemAudioAgileIID} {
				var output *audioCOM
				if status := audioCall((*audioCOM)(unsafe.Pointer(callback)), 0, uintptr(unsafe.Pointer(&iid)), uintptr(unsafe.Pointer(&output))); status != 0 || output == nil {
					t.Fatal("callback interface unavailable")
				}
				audioRelease(output)
			}
			if callback.refs.Load() != 1 {
				t.Fatal("callback COM references leaked")
			}
		})
	}
}

//go:noinline
func newSystemAudioNativeOnlyCallback(marshaler *audioCOM, finalized chan struct{}) (uintptr, <-chan systemAudioActivationResult) {
	callback := newSystemAudioActivation()
	callback.marshaler = marshaler
	runtime.SetFinalizer(callback, func(*systemAudioActivation) { close(finalized) })
	address := uintptr(unsafe.Pointer(callback))
	// The only remaining owner after this function returns is a simulated
	// native COM reference plus the production Go registry.
	systemAudioAddRef(callback)
	completed := callback.completed
	systemAudioReleaseRef(callback)
	return address, completed
}

func systemAudioCallbackIsRooted(address uintptr) bool {
	systemAudioCallbackOwners.Lock()
	defer systemAudioCallbackOwners.Unlock()
	for callback := range systemAudioCallbackOwners.live {
		if uintptr(unsafe.Pointer(callback)) == address {
			return true
		}
	}
	return false
}

func TestSystemAudioWindowsCallbackSurvivesGCUntilFinalCOMRelease(t *testing.T) {
	var marshalerTable [32]uintptr
	var releases atomic.Int32
	marshalerTable[2] = syscall.NewCallback(func(_ uintptr) uintptr { releases.Add(1); return 0 })
	marshaler := &audioCOM{vtable: &marshalerTable}
	finalized := make(chan struct{})
	address, completed := newSystemAudioNativeOnlyCallback(marshaler, finalized)
	// A ready result precedes the native operation's eventual final Release.
	status, _, _ := syscall.SyscallN(systemAudioCallbackTable[3], address, 0)
	result := <-completed
	if status != 0 || result.err == nil {
		t.Fatal("fake activation callback did not complete")
	}
	for cycle := 0; cycle < 3; cycle++ {
		runtime.GC()
		if !systemAudioCallbackIsRooted(address) || releases.Load() != 0 {
			t.Fatal("completion prematurely released handler or marshaler")
		}
		select {
		case <-finalized:
			t.Fatal("native-owned handler was garbage collected")
		default:
		}
	}
	remaining, _, _ := syscall.SyscallN(systemAudioCallbackTable[2], address)
	if remaining != 0 || systemAudioCallbackIsRooted(address) || releases.Load() != 1 {
		t.Fatal("final COM Release did not unroot handler and release marshaler once")
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		runtime.GC()
		select {
		case <-finalized:
			return
		default:
			time.Sleep(time.Millisecond)
		}
	}
	t.Fatal("handler remained rooted after final COM Release")
}

func TestSystemAudioWindowsActiveCallbackOwnsReference(t *testing.T) {
	var marshalerTable, operationTable [32]uintptr
	var releases atomic.Int32
	marshalerTable[2] = syscall.NewCallback(func(_ uintptr) uintptr { releases.Add(1); return 0 })
	entered, resume := make(chan struct{}), make(chan struct{})
	operationTable[3] = syscall.NewCallback(func(_ uintptr, status *uint32, output **audioCOM) uintptr {
		close(entered)
		<-resume
		*status = 0x80004005
		*output = nil
		return 0
	})
	marshaler := &audioCOM{vtable: &marshalerTable}
	address, completed := newSystemAudioNativeOnlyCallback(marshaler, make(chan struct{}))
	operation := &audioCOM{vtable: &operationTable}
	returned := make(chan uintptr, 1)
	go func() {
		status, _, _ := syscall.SyscallN(systemAudioCallbackTable[3], address, uintptr(unsafe.Pointer(operation)))
		returned <- status
		runtime.KeepAlive(operation)
	}()
	<-entered
	remaining, _, _ := syscall.SyscallN(systemAudioCallbackTable[2], address)
	if remaining != 1 {
		close(resume)
		t.Fatal("active callback did not hold its own COM reference")
	}
	runtime.GC()
	if !systemAudioCallbackIsRooted(address) || releases.Load() != 0 {
		close(resume)
		t.Fatal("active callback lost handler or marshaler ownership")
	}
	close(resume)
	result := <-completed
	if status := <-returned; status != 0 || result.err == nil {
		t.Fatal("activation callback did not return its failure result")
	}
	if systemAudioCallbackIsRooted(address) || releases.Load() != 1 {
		t.Fatal("callback return did not release its last COM reference")
	}
}

func TestSystemAudioWindowsInitializeUsesOnlySupportedSlots(t *testing.T) {
	var table, captureTable [32]uintptr
	client := &audioCOM{vtable: &table}
	capture := &audioCOM{vtable: &captureTable}
	var calls []int
	table[3] = syscall.NewCallback(func(_ uintptr, mode uintptr, flags uintptr, duration uintptr, period uintptr, format *[18]byte, session uintptr) uintptr {
		calls = append(calls, 3)
		if mode != 0 || flags != systemAudioStreamFlags || duration != 0 || period != 0 || *format != systemAudioWaveFormat() || session != 0 {
			t.Error("Initialize ABI mismatch")
		}
		return 0
	})
	table[13] = syscall.NewCallback(func(_ uintptr, event uintptr) uintptr {
		calls = append(calls, 13)
		if event == 0 {
			t.Error("missing capture event")
		}
		return 0
	})
	table[14] = syscall.NewCallback(func(_ uintptr, iid *windows.GUID, output **audioCOM) uintptr {
		calls = append(calls, 14)
		if *iid != systemAudioCaptureIID {
			t.Error("incorrect capture IID")
		}
		*output = capture
		return 0
	})
	table[10] = syscall.NewCallback(func(_ uintptr) uintptr { calls = append(calls, 10); return 0 })
	table[11] = syscall.NewCallback(func(_ uintptr) uintptr { calls = append(calls, 11); return 0 })
	table[2] = syscall.NewCallback(func(_ uintptr) uintptr { calls = append(calls, 2); return 0 })
	captureTable[2] = syscall.NewCallback(func(_ uintptr) uintptr { calls = append(calls, 102); return 0 })
	source, err := initializeWindowsSystemAudio(context.Background(), client)
	if err != nil {
		t.Fatal(err)
	}
	if err := source.Close(); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(calls) != "[3 13 14 10 11 102 2]" {
		t.Fatal("incorrect COM slots or cleanup order", calls)
	}
}

type systemAudioInspectWriter struct {
	buffer bytes.Buffer
	check  func()
}

func (w *systemAudioInspectWriter) Write(data []byte) (int, error) {
	w.check()
	return w.buffer.Write(data)
}

func TestSystemAudioWindowsDrainsAndCopiesBeforeRelease(t *testing.T) {
	var table [32]uintptr
	packet := []byte{1, 2, 3, 4, 5, 6, 7, 8}
	index := 0
	held := false
	table[5] = syscall.NewCallback(func(_ uintptr, size *uint32) uintptr {
		if index < 2 {
			*size = 2
		} else {
			*size = 0
		}
		return 0
	})
	table[3] = syscall.NewCallback(func(_ uintptr, data **byte, frames *uint32, flags *uint32, device uintptr, qpc uintptr) uintptr {
		if device != 0 || qpc != 0 || held {
			t.Error("GetBuffer ABI or ownership mismatch")
		}
		held = true
		*frames = 2
		*flags = 0
		*data = &packet[0]
		if index == 1 {
			*flags = 2
			*data = nil
		}
		return 0
	})
	table[4] = syscall.NewCallback(func(_ uintptr, frames uintptr) uintptr {
		if frames != 2 || !held {
			t.Error("ReleaseBuffer ABI or ownership mismatch")
		}
		held = false
		for i := range packet {
			packet[i] = 99
		}
		index++
		return 0
	})
	source := &windowsSystemAudio{capture: &audioCOM{vtable: &table}}
	writer := &systemAudioInspectWriter{check: func() {
		if held {
			t.Error("pipe write holds WASAPI buffer")
		}
	}}
	if err := source.drain(context.Background(), writer); err != nil {
		t.Fatal(err)
	}
	expected := []byte{1, 2, 3, 4, 5, 6, 7, 8, 0, 0, 0, 0, 0, 0, 0, 0}
	if index != 2 || !bytes.Equal(writer.buffer.Bytes(), expected) {
		t.Fatal("packet copy/silent handling failed", writer.buffer.Bytes())
	}
}

type systemAudioWaveHeader struct {
	Data     *byte
	Length   uint32
	Recorded uint32
	User     uintptr
	Flags    uint32
	Loops    uint32
	Next     *systemAudioWaveHeader
	Reserved uintptr
}

type systemAudioTestTone struct {
	device   uintptr
	header   systemAudioWaveHeader
	data     []byte
	prepared bool
	pinned   runtime.Pinner
}

var systemAudioWinMM = windows.NewLazySystemDLL("winmm.dll")

func systemAudioWinMMResult(operation string, result uintptr) error {
	if result != 0 {
		return fmt.Errorf("%s failed (MMRESULT %d)", operation, result)
	}
	return nil
}

func startSystemAudioTestTone() (*systemAudioTestTone, error) {
	tone := &systemAudioTestTone{data: make([]byte, systemAudioSampleRate*systemAudioFrameBytes*3)}
	for frame := 0; frame < len(tone.data)/systemAudioFrameBytes; frame++ {
		for channel, frequency := range []float64{997, 1499} {
			value := int16(12000 * math.Sin(2*math.Pi*frequency*float64(frame)/systemAudioSampleRate))
			binary.LittleEndian.PutUint16(tone.data[frame*4+channel*2:], uint16(value))
		}
	}
	tone.header.Data = &tone.data[0]
	tone.header.Length = uint32(len(tone.data))
	tone.pinned.Pin(&tone.data[0])
	tone.pinned.Pin(&tone.header)
	format := systemAudioWaveFormat()
	result, _, _ := systemAudioWinMM.NewProc("waveOutOpen").Call(uintptr(unsafe.Pointer(&tone.device)), 0xffffffff, uintptr(unsafe.Pointer(&format)), 0, 0, 0)
	err := systemAudioWinMMResult("waveOutOpen", result)
	if err == nil {
		result, _, _ = systemAudioWinMM.NewProc("waveOutPrepareHeader").Call(tone.device, uintptr(unsafe.Pointer(&tone.header)), unsafe.Sizeof(tone.header))
		err = systemAudioWinMMResult("waveOutPrepareHeader", result)
		tone.prepared = err == nil
	}
	if err == nil {
		result, _, _ = systemAudioWinMM.NewProc("waveOutWrite").Call(tone.device, uintptr(unsafe.Pointer(&tone.header)), unsafe.Sizeof(tone.header))
		err = systemAudioWinMMResult("waveOutWrite", result)
	}
	if err != nil {
		_ = tone.Close()
		return nil, err
	}
	return tone, nil
}

func (tone *systemAudioTestTone) Close() error {
	var err error
	if tone.device != 0 {
		result, _, _ := systemAudioWinMM.NewProc("waveOutReset").Call(tone.device)
		err = systemAudioWinMMResult("waveOutReset", result)
		if tone.prepared {
			result, _, _ = systemAudioWinMM.NewProc("waveOutUnprepareHeader").Call(tone.device, uintptr(unsafe.Pointer(&tone.header)), unsafe.Sizeof(tone.header))
			if next := systemAudioWinMMResult("waveOutUnprepareHeader", result); next != nil {
				err = next
			}
		}
		result, _, _ = systemAudioWinMM.NewProc("waveOutClose").Call(tone.device)
		if next := systemAudioWinMMResult("waveOutClose", result); next != nil {
			err = next
		}
		tone.device = 0
	}
	tone.pinned.Unpin()
	runtime.KeepAlive(tone)
	return err
}

func systemAudioToneStrength(pcm []byte, channel int, frequency float64) (float64, float64) {
	var energy, cosine, sine float64
	frames := len(pcm) / systemAudioFrameBytes
	for frame := 0; frame < frames; frame++ {
		value := float64(int16(binary.LittleEndian.Uint16(pcm[frame*4+channel*2:])))
		angle := 2 * math.Pi * frequency * float64(frame) / systemAudioSampleRate
		energy += value * value
		cosine += value * math.Cos(angle)
		sine += value * math.Sin(angle)
	}
	return math.Sqrt(energy / float64(frames)), 2 * math.Hypot(cosine, sine) / float64(frames)
}

func TestSystemAudioNativeStartup(t *testing.T) {
	if os.Getenv("LOCALDROP_SYSTEM_AUDIO_NATIVE_STARTUP") != "1" {
		t.Skip("requires explicit read-only native activation test")
	}
	errorPath := filepath.Join(t.TempDir(), "activation-error.txt")
	command := fakeSystemAudioCommand("native")
	command.Env = append(command.Env, "LOCALDROP_SYSTEM_AUDIO_NATIVE_ERROR="+errorPath)
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	capture, err := beginSystemAudioChild(ctx, command, systemAudioStartupTimeout)
	if err != nil {
		if detail, readErr := os.ReadFile(errorPath); readErr == nil {
			t.Fatalf("native activation: %v; %s", err, detail)
		}
		t.Fatal(err)
	}
	if err := capture.Close(); err != nil {
		t.Fatal(err)
	}
	t.Log("actual process-loopback activation confirmed stereo48000 PCM16 readiness; no PCM read or endpoint changes")
}

// Opt in only after coordinating with the lead and stopping existing sharing.
// Captured samples stay in memory; only aggregate tone measurements are logged.
func TestSystemAudioNativeMutedTone(t *testing.T) {
	executable := os.Getenv("LOCALDROP_SYSTEM_AUDIO_NATIVE_TEST_EXE")
	if executable == "" {
		t.Skip("requires explicit native executable and coordinated idle endpoints")
	}
	if !filepath.IsAbs(executable) || !strings.HasSuffix(strings.ToLower(executable), ".exe") {
		t.Fatal("native test requires absolute executable path")
	}
	original, err := readNativeAudioStates()
	if err != nil || len(original) == 0 {
		t.Fatal("cannot snapshot endpoint mute/scalar", err)
	}
	command := exec.Command(executable, audioOutputHelperFlag)
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	lease, err := beginAudioOutputChild(command, audioOutputStartupTimeout)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := lease.Close(); err != nil {
			t.Error("mute restoration", err)
		}
		after, err := readNativeAudioStates()
		if err != nil || len(after) != len(original) {
			t.Error("cannot confirm restored endpoint set", err)
			return
		}
		for id, state := range original {
			if after[id] != state {
				t.Error("endpoint mute/scalar was not restored")
			}
		}
		t.Logf("original mute and scalar restored for %d endpoints", len(original))
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	command = exec.Command(executable, systemAudioHelperFlag)
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	capture, err := beginSystemAudioChild(ctx, command, systemAudioStartupTimeout)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := capture.Close(); err != nil {
			t.Error(err)
		}
	})
	tone, err := startSystemAudioTestTone()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := tone.Close(); err != nil {
			t.Error(err)
		}
	})
	pcm := make([]byte, systemAudioSampleRate/10*systemAudioFrameBytes)
	matched := 0
	var sumRMS [2]float64
	for block := 0; block < 12; block++ {
		if _, err := io.ReadFull(capture.PCM, pcm); err != nil {
			t.Fatal("native PCM ended", err)
		}
		states, err := readNativeAudioStates()
		if err != nil || len(states) != len(original) {
			t.Fatal("cannot confirm muted endpoints", err)
		}
		for id, state := range original {
			if now, ok := states[id]; !ok || !now.muted || now.scalar != state.scalar {
				t.Fatal("endpoint escaped mute or changed scalar during tone capture")
			}
		}
		correlated := true
		for channel, frequency := range []float64{997, 1499} {
			rms, strength := systemAudioToneStrength(pcm, channel, frequency)
			sumRMS[channel] += rms
			if rms < 500 || strength < 800 || strength < rms*.2 {
				correlated = false
			}
		}
		if correlated {
			matched++
		}
	}
	if matched < 8 {
		t.Fatalf("muted PCM did not contain controlled stereo tones: %d/12 correlated blocks, RMS %.1f/%.1f", matched, sumRMS[0]/12, sumRMS[1]/12)
	}
	t.Logf("stereo48000 PCM16 while endpoint muted: %d/12 100ms tone-correlated blocks; mean RMS %.1f/%.1f; no PCM recording", matched, sumRMS[0]/12, sumRMS[1]/12)
}
