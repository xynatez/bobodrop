//go:build windows

package platform

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const systemAudioStreamFlags = 0x00020000 | 0x00040000 | 0x80000000

var (
	systemAudioActivate              = windows.NewLazySystemDLL("mmdevapi.dll").NewProc("ActivateAudioInterfaceAsync")
	systemAudioFreeThreadedMarshaler = ole32.NewProc("CoCreateFreeThreadedMarshaler")
	systemAudioClientIID             = windows.GUID{Data1: 0x1cb9ad4c, Data2: 0xdbfa, Data3: 0x4c32, Data4: [8]byte{0xb1, 0x78, 0xc2, 0xf5, 0x68, 0xa7, 0x03, 0xb2}}
	systemAudioCaptureIID            = windows.GUID{Data1: 0xc8adbd64, Data2: 0xe71e, Data3: 0x48a0, Data4: [8]byte{0xa4, 0xde, 0x18, 0x5c, 0x39, 0x5c, 0xd3, 0x17}}
	systemAudioCompletionIID         = windows.GUID{Data1: 0x41d949ab, Data2: 0x9862, Data3: 0x444a, Data4: [8]byte{0x80, 0xf6, 0xc2, 0x61, 0x33, 0x4d, 0xa5, 0xeb}}
	systemAudioUnknownIID            = windows.GUID{Data4: [8]byte{0xc0, 0, 0, 0, 0, 0, 0, 0x46}}
	systemAudioMarshalIID            = windows.GUID{Data1: 3, Data4: [8]byte{0xc0, 0, 0, 0, 0, 0, 0, 0x46}}
	systemAudioAgileIID              = windows.GUID{Data1: 0x94ea2b94, Data2: 0xe9cc, Data3: 0x49e0, Data4: [8]byte{0xc0, 0xff, 0xee, 0x64, 0xca, 0x8f, 0x5b, 0x90}}
	systemAudioCallbackTable         = [32]uintptr{
		0: syscall.NewCallback(systemAudioQueryInterface),
		1: syscall.NewCallback(systemAudioAddRef),
		2: syscall.NewCallback(systemAudioReleaseRef),
		3: syscall.NewCallback(systemAudioActivateCompleted),
	}
	systemAudioCallbackOwners = struct {
		sync.Mutex
		live map[*systemAudioActivation]struct{}
	}{live: make(map[*systemAudioActivation]struct{})}
)

type systemAudioActivationParams struct {
	ActivationType uint32
	TargetPID      uint32
	LoopbackMode   uint32
}

// PROPVARIANT's BLOB pointer is at offset 16 on the supported 64-bit ABI.
type systemAudioPropVariant struct {
	Type     uint16
	Reserved [6]byte
	BlobSize uint32
	Padding  uint32
	BlobData *systemAudioActivationParams
}

type systemAudioActivationResult struct {
	client *audioCOM
	err    error
}

type systemAudioActivation struct {
	vtable    *[32]uintptr
	refs      atomic.Int32
	marshaler *audioCOM
	completed chan systemAudioActivationResult
	pinned    runtime.Pinner
}

func newSystemAudioActivation() *systemAudioActivation {
	callback := &systemAudioActivation{vtable: &systemAudioCallbackTable, completed: make(chan systemAudioActivationResult, 1)}
	callback.refs.Store(1)
	callback.pinned.Pin(callback)
	// Native COM references are numbers, not Go GC roots. Retain the handler
	// and its aggregated marshaler until the final COM Release, even after ready.
	systemAudioCallbackOwners.Lock()
	systemAudioCallbackOwners.live[callback] = struct{}{}
	systemAudioCallbackOwners.Unlock()
	return callback
}

func systemAudioQueryInterface(this *systemAudioActivation, iid *windows.GUID, output **audioCOM) uintptr {
	if output == nil || iid == nil {
		return 0x80004003
	}
	*output = nil
	switch *iid {
	case systemAudioUnknownIID, systemAudioCompletionIID, systemAudioAgileIID:
		*output = (*audioCOM)(unsafe.Pointer(this))
		this.refs.Add(1)
		return 0
	case systemAudioMarshalIID:
		if this.marshaler != nil {
			return audioCall(this.marshaler, 0, uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(output)))
		}
	}
	return 0x80004002
}

func systemAudioAddRef(this *systemAudioActivation) uintptr { return uintptr(this.refs.Add(1)) }

func systemAudioReleaseRef(this *systemAudioActivation) uintptr {
	remaining := this.refs.Add(-1)
	if remaining == 0 {
		marshaler := this.marshaler
		this.marshaler = nil
		audioRelease(marshaler)
		this.pinned.Unpin()
		systemAudioCallbackOwners.Lock()
		delete(systemAudioCallbackOwners.live, this)
		systemAudioCallbackOwners.Unlock()
		runtime.KeepAlive(this)
	}
	return uintptr(remaining)
}

func systemAudioActivateCompleted(this *systemAudioActivation, operation *audioCOM) uintptr {
	systemAudioAddRef(this)
	defer systemAudioReleaseRef(this)
	result := systemAudioActivationResult{}
	if operation == nil {
		result.err = errors.New("system audio activation returned no operation")
	} else {
		var activationStatus uint32 = 0x8000ffff
		var unknown *audioCOM
		status := audioCall(operation, 3, uintptr(unsafe.Pointer(&activationStatus)), uintptr(unsafe.Pointer(&unknown)))
		result.err = errors.Join(audioHRESULT("GetActivateResult", status), audioHRESULT("system audio activation", uintptr(activationStatus)))
		if result.err == nil {
			if unknown == nil {
				result.err = errors.New("system audio activation returned no interface")
			} else {
				result.err = audioHRESULT("QueryInterface(IAudioClient)", audioCall(unknown, 0, uintptr(unsafe.Pointer(&systemAudioClientIID)), uintptr(unsafe.Pointer(&result.client))))
				if result.err == nil && result.client == nil {
					result.err = errors.New("system audio activation returned no audio client")
				}
			}
		}
		audioRelease(unknown)
	}
	this.completed <- result
	return 0
}

func BeginSystemAudioCapture(ctx context.Context) (*SystemAudioCapture, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	version := windows.RtlGetVersion()
	if version.MajorVersion < 10 || version.BuildNumber < 20348 {
		return nil, errors.New("system audio capture requires Windows build 20348 or later")
	}
	if unsafe.Sizeof(uintptr(0)) != 8 {
		return nil, errors.New("system audio capture requires 64-bit Windows")
	}
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}
	command := exec.Command(executable, systemAudioHelperFlag)
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return beginSystemAudioChild(ctx, command, systemAudioStartupTimeout)
}

func RunSystemAudioHelper(args []string) (bool, error) {
	if len(args) == 0 || args[0] != systemAudioHelperFlag {
		return false, nil
	}
	if len(args) != 1 {
		return true, errors.New("invalid system audio helper arguments")
	}
	if unsafe.Sizeof(uintptr(0)) != 8 {
		return true, errors.New("system audio capture requires 64-bit Windows")
	}
	version := windows.RtlGetVersion()
	if version.MajorVersion < 10 || version.BuildNumber < 20348 {
		return true, errors.New("system audio capture requires Windows build 20348 or later")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	status, _, _ := audioCoInitializeEx.Call(0, 0)
	if err := audioHRESULT("CoInitializeEx", status); err != nil {
		return true, err
	}
	defer audioCoUninitialize.Call()
	return true, serveSystemAudioHelper(os.Stdin, os.Stdout, func(ctx context.Context) (systemAudioSource, error) {
		go func() {
			<-ctx.Done()
			timer := time.NewTimer(systemAudioCloseTimeout)
			defer timer.Stop()
			<-timer.C
			// Parent EOF also bounds a stuck activation or stdout write.
			os.Exit(0)
		}()
		return openWindowsSystemAudio(ctx)
	})
}

func activateWindowsSystemAudio() (*audioCOM, error) {
	if err := systemAudioActivate.Find(); err != nil {
		return nil, err
	}
	callback := newSystemAudioActivation()
	status, _, _ := systemAudioFreeThreadedMarshaler.Call(uintptr(unsafe.Pointer(callback)), uintptr(unsafe.Pointer(&callback.marshaler)))
	if err := audioHRESULT("CoCreateFreeThreadedMarshaler", status); err != nil {
		systemAudioReleaseRef(callback)
		return nil, err
	}
	params := systemAudioActivationParams{ActivationType: 1, TargetPID: uint32(os.Getpid()), LoopbackMode: 1}
	variant := systemAudioPropVariant{Type: 65, BlobSize: uint32(unsafe.Sizeof(params)), BlobData: &params}
	path, _ := windows.UTF16PtrFromString("VAD\\Process_Loopback")
	var operation *audioCOM
	status, _, _ = systemAudioActivate.Call(uintptr(unsafe.Pointer(path)), uintptr(unsafe.Pointer(&systemAudioClientIID)), uintptr(unsafe.Pointer(&variant)), uintptr(unsafe.Pointer(callback)), uintptr(unsafe.Pointer(&operation)))
	err := audioHRESULT("ActivateAudioInterfaceAsync", status)
	var result systemAudioActivationResult
	if err == nil {
		// Do not abandon Go/COM callback memory during a pending activation.
		// Cancellation terminates this dedicated process if completion stalls.
		result = <-callback.completed
		err = result.err
	}
	audioRelease(operation)
	systemAudioReleaseRef(callback)
	runtime.KeepAlive(callback)
	runtime.KeepAlive(params)
	runtime.KeepAlive(variant)
	runtime.KeepAlive(path)
	if err != nil {
		audioRelease(result.client)
		return nil, err
	}
	return result.client, nil
}

func systemAudioWaveFormat() [18]byte {
	var format [18]byte
	binary.LittleEndian.PutUint16(format[0:2], 1)
	binary.LittleEndian.PutUint16(format[2:4], systemAudioChannels)
	binary.LittleEndian.PutUint32(format[4:8], systemAudioSampleRate)
	binary.LittleEndian.PutUint32(format[8:12], systemAudioSampleRate*systemAudioFrameBytes)
	binary.LittleEndian.PutUint16(format[12:14], systemAudioFrameBytes)
	binary.LittleEndian.PutUint16(format[14:16], 16)
	return format
}

type windowsSystemAudio struct {
	client  *audioCOM
	capture *audioCOM
	event   windows.Handle
	started bool
}

func openWindowsSystemAudio(ctx context.Context) (systemAudioSource, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	client, err := activateWindowsSystemAudio()
	if err != nil {
		return nil, err
	}
	return initializeWindowsSystemAudio(ctx, client)
}

func initializeWindowsSystemAudio(ctx context.Context, client *audioCOM) (systemAudioSource, error) {
	source := &windowsSystemAudio{client: client}
	fail := func(err error) (systemAudioSource, error) { return nil, errors.Join(err, source.Close()) }
	if ctx.Err() != nil {
		return fail(ctx.Err())
	}
	format := systemAudioWaveFormat()
	if err := audioHRESULT("IAudioClient.Initialize", audioCall(client, 3, 0, systemAudioStreamFlags, 0, 0, uintptr(unsafe.Pointer(&format[0])), 0)); err != nil {
		return fail(err)
	}
	var err error
	source.event, err = windows.CreateEvent(nil, 0, 0, nil)
	if err != nil {
		return fail(err)
	}
	if err := audioHRESULT("IAudioClient.SetEventHandle", audioCall(client, 13, uintptr(source.event))); err != nil {
		return fail(err)
	}
	if err := audioHRESULT("IAudioClient.GetService(IAudioCaptureClient)", audioCall(client, 14, uintptr(unsafe.Pointer(&systemAudioCaptureIID)), uintptr(unsafe.Pointer(&source.capture)))); err != nil {
		return fail(err)
	}
	if source.capture == nil {
		return fail(errors.New("system audio capture service is unavailable"))
	}
	if err := audioHRESULT("IAudioClient.Start", audioCall(client, 10)); err != nil {
		return fail(err)
	}
	source.started = true
	return source, nil
}

func (s *windowsSystemAudio) Run(ctx context.Context, output io.Writer) error {
	for ctx.Err() == nil {
		status, err := windows.WaitForSingleObject(s.event, 50)
		if err != nil {
			return err
		}
		if status == uint32(windows.WAIT_TIMEOUT) {
			continue
		}
		if status != windows.WAIT_OBJECT_0 {
			return errors.New("system audio event wait failed")
		}
		if err := s.drain(ctx, output); err != nil {
			return err
		}
	}
	return nil
}

func (s *windowsSystemAudio) drain(ctx context.Context, output io.Writer) error {
	for ctx.Err() == nil {
		var available uint32
		if err := audioHRESULT("GetNextPacketSize", audioCall(s.capture, 5, uintptr(unsafe.Pointer(&available)))); err != nil {
			return err
		}
		if available == 0 {
			return nil
		}
		var data *byte
		var frames, flags uint32
		if err := audioHRESULT("IAudioCaptureClient.GetBuffer", audioCall(s.capture, 3, uintptr(unsafe.Pointer(&data)), uintptr(unsafe.Pointer(&frames)), uintptr(unsafe.Pointer(&flags)), 0, 0)); err != nil {
			return err
		}
		var copyErr error
		var packet []byte
		if frames > systemAudioSampleRate {
			copyErr = errors.New("system audio packet exceeds capture limit")
		} else if frames != 0 {
			packet = make([]byte, int(frames)*systemAudioFrameBytes)
			if flags&2 == 0 {
				if data == nil {
					copyErr = errors.New("system audio packet has no samples")
				} else {
					copy(packet, unsafe.Slice(data, len(packet)))
				}
			}
		}
		err := errors.Join(copyErr, audioHRESULT("IAudioCaptureClient.ReleaseBuffer", audioCall(s.capture, 4, uintptr(frames))))
		if err != nil {
			return err
		}
		// Copy and release the WASAPI buffer before a potentially blocked pipe.
		for len(packet) != 0 && ctx.Err() == nil {
			count := min(len(packet), 16384)
			n, err := output.Write(packet[:count])
			if err != nil {
				return err
			}
			if n != count {
				return io.ErrShortWrite
			}
			packet = packet[count:]
		}
	}
	return nil
}

func (s *windowsSystemAudio) Close() error {
	var err error
	if s.started {
		err = audioHRESULT("IAudioClient.Stop", audioCall(s.client, 11))
		s.started = false
	}
	audioRelease(s.capture)
	s.capture = nil
	audioRelease(s.client)
	s.client = nil
	if s.event != 0 {
		err = errors.Join(err, windows.CloseHandle(s.event))
		s.event = 0
	}
	return err
}
