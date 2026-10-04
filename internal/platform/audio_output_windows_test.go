//go:build windows

package platform

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

func TestAudioOutputFakeSubprocess(t *testing.T) {
	mode := os.Getenv("LOCALDROP_AUDIO_FAKE_PROCESS")
	if mode == "" {
		return
	}
	if mode == "parent" {
		command := fakeAudioCommand("helper")
		lease, err := beginAudioOutputChild(command, time.Second)
		if err != nil {
			os.Exit(2)
		}
		_ = lease // Deliberately terminate without Close: the OS closes stdin.
		os.Exit(0)
	}
	if mode == "slow" {
		time.Sleep(100 * time.Millisecond)
	}
	d := &fakeAudioDevices{states: []*fakeAudioState{{id: "fake speaker", scalar: 1}}}
	err := serveAudioOutputHelper(os.Stdin, os.Stdout, func() (audioOutputDevices, error) {
		if mode == "failure" {
			return nil, errors.New("fake failure")
		}
		return d, nil
	}, time.Millisecond)
	if path := os.Getenv("LOCALDROP_AUDIO_FAKE_RESTORED"); path != "" {
		state := map[string]any{"muted": d.states[0].mute(), "closed": d.closed.Load(), "refs": d.states[0].refs.Load()}
		data, _ := json.Marshal(state)
		_ = os.WriteFile(path, data, 0600)
	}
	if err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}

func fakeAudioCommand(mode string) *exec.Cmd {
	command := exec.Command(os.Args[0], "-test.run=^TestAudioOutputFakeSubprocess$")
	command.Env = append(os.Environ(), "LOCALDROP_AUDIO_FAKE_PROCESS="+mode)
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return command
}

func TestAudioOutputChildHandshakeAndEOFCleanup(t *testing.T) {
	lease, err := beginAudioOutputChild(fakeAudioCommand("helper"), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	lease.Revoke()
	select {
	case <-lease.Done:
	default:
		t.Fatal("Revoke did not immediately signal Done")
	}
	if err := lease.Close(); err != nil {
		t.Fatal(err)
	}
	if err := lease.Close(); err != nil {
		t.Fatal("Close is not idempotent:", err)
	}
}

func TestAudioOutputChildTimeoutAndFailureStayBounded(t *testing.T) {
	for _, mode := range []string{"slow", "failure"} {
		t.Run(mode, func(t *testing.T) {
			timeout := time.Second
			if mode == "slow" {
				timeout = 10 * time.Millisecond
			}
			start := time.Now()
			lease, err := beginAudioOutputChild(fakeAudioCommand(mode), timeout)
			if err == nil || lease != nil {
				t.Fatal("invalid helper confirmed audio mute")
			}
			if time.Since(start) > 2*time.Second {
				t.Fatal("helper failure exceeded startup deadline")
			}
		})
	}
}

func TestAudioOutputParentCrashRestoresThroughEOF(t *testing.T) {
	path := filepath.Join(t.TempDir(), "restored.json")
	command := fakeAudioCommand("parent")
	command.Env = append(command.Env, "LOCALDROP_AUDIO_FAKE_RESTORED="+path)
	if err := command.Run(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil {
			var result struct {
				Muted        bool
				Closed, Refs int
			}
			if err := json.Unmarshal(data, &result); err != nil {
				t.Fatal(err)
			}
			if result.Muted || result.Closed != 1 || result.Refs != 0 {
				t.Fatalf("parent crash left audio state: %s", data)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("parent crash did not close child stdin and restore audio")
}

func TestAudioOutputWindowsCOMMuteABI(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	var table [32]uintptr
	var calls []uintptr
	muted := int32(0)
	table[14] = syscall.NewCallback(func(_ uintptr, value uintptr, context uintptr) uintptr {
		calls = append(calls, value, context)
		muted = int32(value)
		return 0
	})
	table[15] = syscall.NewCallback(func(_ uintptr, value *int32) uintptr { *value = muted; return 0 })
	table[2] = syscall.NewCallback(func(_ uintptr) uintptr { return 0 })
	e := &windowsAudioOutputEndpoint{id: "fake", volume: &audioCOM{vtable: &table}}
	for _, value := range []bool{true, false} {
		if err := e.SetMute(value); err != nil {
			t.Fatal(err)
		}
		if actual, err := e.Muted(); err != nil || actual != value {
			t.Fatal("BOOL ABI mismatch", actual, err)
		}
	}
	e.Release()
	if fmt.Sprint(calls) != "[1 0 0 0]" {
		t.Fatal("SetMute called incorrect vtable/signature", calls)
	}
	if audioHRESULT("failed", 0x80004005) == nil || audioHRESULT("S_FALSE", 1) != nil {
		t.Fatal("HRESULT signed status handling is wrong")
	}
}

func TestAudioOutputRecoveryJournalWritesAndClears(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	d := &windowsAudioOutputs{journalPath: path, originals: make(map[string]bool)}
	if err := d.Remember("speaker", false); err != nil {
		t.Fatal(err)
	}
	if err := d.Remember("headphones", true); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var original map[string]bool
	if err := json.Unmarshal(data, &original); err != nil || len(original) != 2 || original["speaker"] || !original["headphones"] {
		t.Fatalf("journal lost original mute: %s %v", data, err)
	}
	if err := d.Restored("speaker"); err != nil {
		t.Fatal(err)
	}
	if err := d.Restored("headphones"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("restored journal was not removed", err)
	}
}

type audioNativeState struct {
	muted  bool
	scalar float32
}

func readNativeAudioStates() (map[string]audioNativeState, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	result, _, _ := audioCoInitializeEx.Call(0, 0)
	if err := audioHRESULT("CoInitializeEx", result); err != nil {
		return nil, err
	}
	defer audioCoUninitialize.Call()
	d := &windowsAudioOutputs{}
	result, _, _ = audioCoCreateInstance.Call(uintptr(unsafe.Pointer(&audioEnumeratorClass)), 0, 23, uintptr(unsafe.Pointer(&audioEnumeratorIID)), uintptr(unsafe.Pointer(&d.enumerator)))
	if err := audioHRESULT("CoCreateInstance", result); err != nil {
		return nil, err
	}
	defer d.Close()
	endpoints, err := d.Active()
	if err != nil {
		return nil, err
	}
	states := make(map[string]audioNativeState)
	for _, endpoint := range endpoints {
		e := endpoint.(*windowsAudioOutputEndpoint)
		muted, muteErr := e.Muted()
		var scalar float32
		volumeErr := audioHRESULT("GetMasterVolumeLevelScalar", audioCall(e.volume, 9, uintptr(unsafe.Pointer(&scalar))))
		e.Release()
		err = errors.Join(err, muteErr, volumeErr)
		states[e.id] = audioNativeState{muted: muted, scalar: scalar}
	}
	return states, err
}

// This opt-in test is the only test that changes real endpoint mute state.
// The lead must coordinate its run while no user audio session is active.
func TestAudioOutputNativeRoundTrip(t *testing.T) {
	executable := os.Getenv("LOCALDROP_AUDIO_NATIVE_TEST_EXE")
	if executable == "" {
		t.Skip("real Windows audio test requires explicitly configured executable")
	}
	if !filepath.IsAbs(executable) || !strings.HasSuffix(strings.ToLower(executable), ".exe") {
		t.Fatal("native test requires absolute executable path")
	}
	for _, termination := range []string{"Close", "parent EOF"} {
		t.Run(termination, func(t *testing.T) {
			original, err := readNativeAudioStates()
			if err != nil || len(original) == 0 {
				t.Fatal("cannot snapshot native endpoint state", err)
			}
			command := exec.Command(executable, audioOutputHelperFlag)
			command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
			lease, err := beginAudioOutputChild(command, audioOutputStartupTimeout)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := lease.Close(); err != nil {
					t.Error("restoration cleanup:", err)
				}
			})
			active, err := readNativeAudioStates()
			if err != nil || len(active) != len(original) {
				t.Fatal("native endpoint set changed", err)
			}
			for id, state := range original {
				if now, ok := active[id]; !ok || !now.muted || now.scalar != state.scalar {
					t.Fatalf("endpoint %s mute/scalar mismatch: before=%+v during=%+v", id, state, now)
				}
			}
			if termination == "parent EOF" {
				// StdinPipe is parent-only; closing it is the same EOF delivered
				// when the parent process terminates without calling Revoke.
				if err := lease.stdin.Close(); err != nil {
					t.Fatal(err)
				}
				select {
				case <-lease.Done:
				case <-time.After(audioOutputRestoreTimeout):
					t.Fatal("parent EOF did not restore and stop helper")
				}
			}
			if err := lease.Close(); err != nil {
				t.Fatal(err)
			}
			restored, err := readNativeAudioStates()
			if err != nil || len(restored) != len(original) {
				t.Fatal("cannot inspect restored endpoints", err)
			}
			for id, state := range original {
				if now, ok := restored[id]; !ok || now != state {
					t.Fatalf("endpoint %s not restored: before=%+v after=%+v", id, state, now)
				}
			}
			t.Logf("%d native endpoints confirmed muted; original mute and volume scalar restored via %s", len(original), termination)
		})
	}
}
