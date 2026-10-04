//go:build windows

package platform

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	audioOutputStartupTimeout = 4 * time.Second
	audioOutputRestoreTimeout = 4 * time.Second
	audioOutputGuardInterval  = 100 * time.Millisecond
)

var (
	audioCoInitializeEx   = ole32.NewProc("CoInitializeEx")
	audioCoUninitialize   = ole32.NewProc("CoUninitialize")
	audioCoCreateInstance = ole32.NewProc("CoCreateInstance")
	audioCoTaskMemFree    = ole32.NewProc("CoTaskMemFree")
	audioEnumeratorClass  = windows.GUID{Data1: 0xbcde0395, Data2: 0xe52f, Data3: 0x467c, Data4: [8]byte{0x8e, 0x3d, 0xc4, 0x57, 0x92, 0x91, 0x69, 0x2e}}
	audioEnumeratorIID    = windows.GUID{Data1: 0xa95664d2, Data2: 0x9614, Data3: 0x4f35, Data4: [8]byte{0xa7, 0x46, 0xde, 0x8d, 0xb6, 0x36, 0x17, 0xe6}}
	audioVolumeIID        = windows.GUID{Data1: 0x5cdf2c82, Data2: 0x841e, Data3: 0x4546, Data4: [8]byte{0x97, 0x22, 0x0c, 0xf7, 0x40, 0x78, 0x22, 0x9a}}
)

type audioOutputChild struct {
	input    io.WriteCloser
	finished chan struct{}
	done     chan struct{}
	revoke   sync.Once
	doneOnce sync.Once
	err      error
}

func (c *audioOutputChild) signalDone() { c.doneOnce.Do(func() { close(c.done) }) }

func (c *audioOutputChild) Revoke() {
	c.revoke.Do(func() {
		c.signalDone()
		go func() { _ = c.input.Close() }()
	})
}

func (c *audioOutputChild) Close() error {
	c.Revoke()
	timer := time.NewTimer(audioOutputRestoreTimeout)
	defer timer.Stop()
	select {
	case <-c.finished:
		return c.err
	case <-timer.C:
		// Killing a child mid-COM call can strand muted endpoints. It retains
		// its snapshots and continues restoration after this bounded wait.
		return errors.New("Windows audio restoration is still pending")
	}
}

func BeginSilentAudioOutput() (*AudioOutputLease, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}
	command := exec.Command(executable, audioOutputHelperFlag)
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return beginAudioOutputChild(command, audioOutputStartupTimeout)
}

func beginAudioOutputChild(command *exec.Cmd, timeout time.Duration) (*AudioOutputLease, error) {
	var nonce [32]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, err
	}
	key := hex.EncodeToString(nonce[:])
	input, err := command.StdinPipe()
	if err != nil {
		return nil, err
	}
	output, err := command.StdoutPipe()
	if err != nil {
		_ = input.Close()
		return nil, err
	}
	command.Stderr = io.Discard
	if err := command.Start(); err != nil {
		_ = input.Close()
		_ = output.Close()
		return nil, err
	}
	child := &audioOutputChild{input: input, finished: make(chan struct{}), done: make(chan struct{})}
	ready := make(chan error, 1)
	go func() {
		decoder := json.NewDecoder(io.LimitReader(output, 16384))
		decoder.DisallowUnknownFields()
		var status audioOutputMessage
		readErr := decoder.Decode(&status)
		if readErr == nil && (status.Type != "ready" || status.Nonce != key || status.Error != "") {
			readErr = errors.New("Windows did not confirm the audio output lease")
		}
		ready <- readErr
		if readErr == nil {
			var restored audioOutputMessage
			readErr = decoder.Decode(&restored)
			if readErr == nil && (restored.Type != "restored" || restored.Error != "") {
				readErr = errors.New("Windows audio output guard or restoration failed")
			}
		}
		child.signalDone()
		_ = input.Close()
		_ = output.Close()
		child.err = errors.Join(readErr, command.Wait())
		close(child.finished)
	}()
	// The begin message is tiny, but writing it also belongs to the startup
	// deadline. Only the parent owns this pipe's write handle.
	written := make(chan error, 1)
	go func() { written <- json.NewEncoder(input).Encode(audioOutputMessage{Type: "begin", Nonce: key}) }()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case err := <-written:
		if err != nil {
			child.Revoke()
			return nil, err
		}
	case <-timer.C:
		child.Revoke()
		return nil, errors.New("audio output helper startup timed out")
	}
	select {
	case err := <-ready:
		if err != nil {
			child.Revoke()
			return nil, err
		}
	case <-timer.C:
		child.Revoke()
		return nil, errors.New("audio output helper startup timed out")
	}
	select {
	case <-child.done:
		child.Revoke()
		return nil, errors.New("audio output helper stopped during startup")
	default:
	}
	return &AudioOutputLease{Revoke: child.Revoke, Close: child.Close, Done: child.done, stdin: input}, nil
}

func RunAudioOutputHelper(args []string) (bool, error) {
	if len(args) == 0 || args[0] != audioOutputHelperFlag {
		return false, nil
	}
	if len(args) != 1 {
		return true, errors.New("invalid audio output helper arguments")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	result, _, _ := audioCoInitializeEx.Call(0, 0) // COINIT_MULTITHREADED
	if err := audioHRESULT("CoInitializeEx", result); err != nil {
		return true, err
	}
	defer audioCoUninitialize.Call()
	return true, serveAudioOutputHelper(os.Stdin, os.Stdout, openWindowsAudioOutputs, audioOutputGuardInterval)
}

type audioCOM struct{ vtable *[32]uintptr }

//go:uintptrescapes
func audioCall(object *audioCOM, slot int, args ...uintptr) uintptr {
	arguments := append([]uintptr{uintptr(unsafe.Pointer(object))}, args...)
	result, _, _ := syscall.SyscallN(object.vtable[slot], arguments...)
	runtime.KeepAlive(object)
	return result
}

func audioRelease(object *audioCOM) {
	if object != nil {
		audioCall(object, 2)
	}
}

func audioHRESULT(operation string, result uintptr) error {
	if int32(result) < 0 {
		return fmt.Errorf("%s failed (HRESULT 0x%08x)", operation, uint32(result))
	}
	return nil
}

type windowsAudioOutputs struct {
	enumerator  *audioCOM
	mutex       windows.Handle
	journalPath string
	originals   map[string]bool
}

func openWindowsAudioOutputs() (audioOutputDevices, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, err
	}
	sid := user.User.Sid.String()
	// The recovery journal is profile-wide, so leases across Windows sessions
	// for this SID must share its lock too.
	name, err := windows.UTF16PtrFromString("Global\\LocalDrop.AudioOutput." + sid)
	if err != nil {
		return nil, err
	}
	mutex, err := windows.CreateMutex(nil, false, name)
	if err != nil {
		return nil, err
	}
	wait, err := windows.WaitForSingleObject(mutex, 0)
	if err != nil || wait != windows.WAIT_OBJECT_0 && wait != windows.WAIT_ABANDONED {
		_ = windows.CloseHandle(mutex)
		return nil, errors.Join(errors.New("another LocalDrop audio output lease is active"), err)
	}
	devices := &windowsAudioOutputs{mutex: mutex, originals: make(map[string]bool)}
	result, _, _ := audioCoCreateInstance.Call(uintptr(unsafe.Pointer(&audioEnumeratorClass)), 0, 23,
		uintptr(unsafe.Pointer(&audioEnumeratorIID)), uintptr(unsafe.Pointer(&devices.enumerator)))
	if err := audioHRESULT("CoCreateInstance(IMMDeviceEnumerator)", result); err != nil {
		devices.Close()
		return nil, err
	}
	appData, err := os.UserCacheDir()
	if err != nil {
		devices.Close()
		return nil, err
	}
	devices.journalPath = filepath.Join(appData, "LocalDrop", "audio-output-"+sid+".json")
	if err := devices.recoverJournal(); err != nil {
		devices.Close()
		return nil, err
	}
	return devices, nil
}

func (d *windowsAudioOutputs) Active() ([]audioOutputEndpoint, error) {
	var collection *audioCOM
	if err := audioHRESULT("EnumAudioEndpoints", audioCall(d.enumerator, 3, 0, 1, uintptr(unsafe.Pointer(&collection)))); err != nil {
		return nil, err
	}
	defer audioRelease(collection)
	var count uint32
	if err := audioHRESULT("GetCount", audioCall(collection, 3, uintptr(unsafe.Pointer(&count)))); err != nil {
		return nil, err
	}
	if count > 256 {
		return nil, errors.New("too many active audio endpoints")
	}
	endpoints := make([]audioOutputEndpoint, 0, count)
	for i := uint32(0); i < count; i++ {
		var device *audioCOM
		err := audioHRESULT("IMMDeviceCollection.Item", audioCall(collection, 4, uintptr(i), uintptr(unsafe.Pointer(&device))))
		var endpoint *windowsAudioOutputEndpoint
		if err == nil {
			endpoint, err = activateAudioOutput(device)
		}
		audioRelease(device)
		if err != nil {
			for _, endpoint := range endpoints {
				endpoint.Release()
			}
			return nil, err
		}
		endpoints = append(endpoints, endpoint)
	}
	return endpoints, nil
}

func (d *windowsAudioOutputs) Close() {
	audioRelease(d.enumerator)
	d.enumerator = nil
	if d.mutex != 0 {
		_ = windows.ReleaseMutex(d.mutex)
		_ = windows.CloseHandle(d.mutex)
		d.mutex = 0
	}
}

func (d *windowsAudioOutputs) saveJournal() error {
	if len(d.originals) == 0 {
		if err := os.Remove(d.journalPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(d.journalPath), 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(d.journalPath), ".audio-output-")
	if err != nil {
		return err
	}
	name := file.Name()
	defer os.Remove(name)
	err = json.NewEncoder(file).Encode(d.originals)
	if err == nil {
		err = file.Sync()
	}
	err = errors.Join(err, file.Close())
	if err != nil {
		return err
	}
	return os.Rename(name, d.journalPath)
}

func (d *windowsAudioOutputs) Remember(id string, muted bool) error {
	d.originals[id] = muted
	return d.saveJournal()
}

func (d *windowsAudioOutputs) Restored(id string) error {
	original := d.originals[id]
	delete(d.originals, id)
	if err := d.saveJournal(); err != nil {
		d.originals[id] = original
		return err
	}
	return nil
}

func (d *windowsAudioOutputs) recoverJournal() error {
	file, err := os.Open(d.journalPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(io.LimitReader(file, 1<<20))
	err = decoder.Decode(&d.originals)
	if err == nil {
		var extra any
		if decoder.Decode(&extra) != io.EOF {
			err = errors.New("invalid audio output recovery journal")
		}
	}
	_ = file.Close()
	if err != nil || d.originals == nil || len(d.originals) > 256 {
		return errors.Join(errors.New("invalid audio output recovery journal"), err)
	}
	return recoverAudioOutputJournal(d.originals, func(id string) (audioOutputEndpoint, error) {
		name, err := windows.UTF16PtrFromString(id)
		if err != nil {
			return nil, err
		}
		var device *audioCOM
		err = audioHRESULT("GetDevice for mute recovery", audioCall(d.enumerator, 5, uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(&device))))
		var endpoint *windowsAudioOutputEndpoint
		if err == nil {
			endpoint, err = activateAudioOutput(device)
		}
		audioRelease(device)
		if err != nil {
			return nil, err
		}
		return endpoint, nil
	}, d.Restored)
}

type windowsAudioOutputEndpoint struct {
	id     string
	volume *audioCOM
}

func activateAudioOutput(device *audioCOM) (*windowsAudioOutputEndpoint, error) {
	var id *uint16
	if err := audioHRESULT("IMMDevice.GetId", audioCall(device, 5, uintptr(unsafe.Pointer(&id)))); err != nil {
		return nil, err
	}
	identifier := windows.UTF16PtrToString(id)
	audioCoTaskMemFree.Call(uintptr(unsafe.Pointer(id)))
	endpoint := &windowsAudioOutputEndpoint{id: identifier}
	if err := audioHRESULT("Activate(IAudioEndpointVolume)", audioCall(device, 3, uintptr(unsafe.Pointer(&audioVolumeIID)), 23, 0, uintptr(unsafe.Pointer(&endpoint.volume)))); err != nil {
		endpoint.Release()
		return nil, err
	}
	return endpoint, nil
}

func (e *windowsAudioOutputEndpoint) ID() string { return e.id }

func (e *windowsAudioOutputEndpoint) Muted() (bool, error) {
	var muted int32
	err := audioHRESULT("IAudioEndpointVolume.GetMute", audioCall(e.volume, 15, uintptr(unsafe.Pointer(&muted))))
	return muted != 0, err
}

func (e *windowsAudioOutputEndpoint) SetMute(muted bool) error {
	value := uintptr(0)
	if muted {
		value = 1
	}
	return audioHRESULT("IAudioEndpointVolume.SetMute", audioCall(e.volume, 14, value, 0))
}

func (e *windowsAudioOutputEndpoint) Release() { audioRelease(e.volume); e.volume = nil }
