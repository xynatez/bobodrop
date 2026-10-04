//go:build windows

package platform

import (
	"encoding/binary"
	"errors"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"unsafe"
)

func TestSendInputNativeLayouts(t *testing.T) {
	var input winInput
	var keyboard winKeyboardInput
	var mouse winMouseInput
	inputSize, unionOffset, mouseSize, keySize := uintptr(40), uintptr(8), uintptr(32), uintptr(24)
	mouseExtra, keyboardExtra := uintptr(24), uintptr(16)
	if unsafe.Sizeof(uintptr(0)) == 4 {
		inputSize, unionOffset, mouseSize, keySize = 28, 4, 24, 16
		mouseExtra, keyboardExtra = 20, 12
	}
	if unsafe.Sizeof(input) != inputSize || unsafe.Offsetof(input.Data) != unionOffset ||
		unsafe.Sizeof(mouse) != mouseSize || unsafe.Sizeof(keyboard) != keySize ||
		unsafe.Offsetof(mouse.ExtraInfo) != mouseExtra || unsafe.Offsetof(keyboard.ExtraInfo) != keyboardExtra {
		t.Fatal("INPUT/MOUSEINPUT/KEYBDINPUT layout does not match Windows ABI")
	}
	if unsafe.Offsetof(keyboard.Scan) != 2 || unsafe.Offsetof(keyboard.Flags) != 4 || unsafe.Offsetof(keyboard.Time) != 8 ||
		unsafe.Offsetof(mouse.DY) != 4 || unsafe.Offsetof(mouse.MouseData) != 8 || unsafe.Offsetof(mouse.Flags) != 12 || unsafe.Offsetof(mouse.Time) != 16 {
		t.Fatal("native input field offsets do not match Windows ABI")
	}
}

func captureRemoteInput(t *testing.T, input RemoteInput) []winInput {
	t.Helper()
	var captured []winInput
	err := sendRemoteInput(input, func(events []winInput) (uint32, error) {
		captured = append(captured, events...)
		return uint32(len(events)), syscall.Errno(0)
	})
	if err != nil {
		t.Fatal(err)
	}
	return captured
}

func inputBytes(event *winInput) []byte {
	return unsafe.Slice((*byte)(unsafe.Pointer(event)), int(unsafe.Sizeof(*event)))
}

func TestRemoteMouseDispatchBytes(t *testing.T) {
	events := captureRemoteInput(t, RemoteInput{Kind: "move", DX: -500, DY: 123})
	if len(events) != 1 {
		t.Fatal("move must use one INPUT")
	}
	data := inputBytes(&events[0])
	u := int(unsafe.Offsetof(events[0].Data))
	if binary.LittleEndian.Uint32(data[:4]) != 0 || int32(binary.LittleEndian.Uint32(data[u:u+4])) != -500 ||
		int32(binary.LittleEndian.Uint32(data[u+4:u+8])) != 123 || binary.LittleEndian.Uint32(data[u+12:u+16]) != 1 {
		t.Fatalf("relative mouse event bytes incorrect: %x", data)
	}
	events = captureRemoteInput(t, RemoteInput{Kind: "scroll", DY: -120})
	data = inputBytes(&events[0])
	if int32(binary.LittleEndian.Uint32(data[u+8:u+12])) != -120 || binary.LittleEndian.Uint32(data[u+12:u+16]) != 0x800 {
		t.Fatalf("signed wheel event bytes incorrect: %x", data)
	}
	for _, test := range []struct {
		button string
		down   uint32
		up     uint32
	}{{"left", 2, 4}, {"right", 8, 16}, {"middle", 32, 64}} {
		events = captureRemoteInput(t, RemoteInput{Kind: "click", Button: test.button})
		if len(events) != 2 || events[0].Data.Flags != test.down || events[1].Data.Flags != test.up || events[0].Type != 0 || events[1].Type != 0 {
			t.Fatalf("%s click was not paired: %+v", test.button, events)
		}
	}
}

func TestRemoteKeyboardDispatchBytes(t *testing.T) {
	events := captureRemoteInput(t, RemoteInput{Kind: "key", Key: "arrowup"})
	u := int(unsafe.Offsetof(events[0].Data))
	for i, event := range events {
		data := inputBytes(&event)
		if binary.LittleEndian.Uint32(data[:4]) != 1 || binary.LittleEndian.Uint16(data[u:u+2]) != 0x26 ||
			binary.LittleEndian.Uint16(data[u+2:u+4]) != 0 || binary.LittleEndian.Uint32(data[u+4:u+8]) != uint32(1+2*i) {
			t.Fatalf("extended arrow input bytes incorrect: %x", data)
		}
	}
	for _, test := range []struct {
		name string
		vk   uint16
	}{{"ctrl+a", 0x41}, {"ctrl+c", 0x43}, {"ctrl+v", 0x56}, {"ctrl+z", 0x5a}} {
		events = captureRemoteInput(t, RemoteInput{Kind: "key", Key: test.name})
		if len(events) != 4 {
			t.Fatal("Ctrl shortcut must have four paired events")
		}
		for i, want := range []winKeyboardInput{{VK: 0x11}, {VK: test.vk}, {VK: test.vk, Flags: 2}, {VK: 0x11, Flags: 2}} {
			if got := keyboardData(&events[i]); events[i].Type != 1 || got != want {
				t.Fatalf("%s event %d: got %+v, want %+v", test.name, i, got, want)
			}
		}
	}
}

func TestRemoteUnicodeSurrogatesAndBoundedBatch(t *testing.T) {
	events := captureRemoteInput(t, RemoteInput{Kind: "text", Text: "A🦷"})
	if len(events) != 6 {
		t.Fatal("A plus supplementary character requires six Unicode INPUT events")
	}
	for i, scan := range []uint16{0x0041, 0x0041, 0xd83e, 0xd83e, 0xddb7, 0xddb7} {
		keyboard := keyboardData(&events[i])
		wantFlags := uint32(4)
		if i%2 == 1 {
			wantFlags |= 2
		}
		if events[i].Type != 1 || keyboard.VK != 0 || keyboard.Scan != scan || keyboard.Flags != wantFlags {
			t.Fatalf("Unicode event %d: %+v", i, keyboard)
		}
	}
	if events := captureRemoteInput(t, RemoteInput{Kind: "text", Text: strings.Repeat("🦷", 256)}); len(events) != 1024 {
		t.Fatalf("maximum batch = %d events, want 1024", len(events))
	}
}

func TestInvalidRemoteInputNeverDispatches(t *testing.T) {
	for _, input := range invalidRemoteInputs() {
		calls := 0
		err := sendRemoteInput(input, func(events []winInput) (uint32, error) {
			calls++
			return uint32(len(events)), nil
		})
		if err == nil || calls != 0 {
			t.Fatalf("invalid input reached dispatcher: %+v, calls=%d, err=%v", input, calls, err)
		}
	}
	for _, input := range []RemoteInput{{Kind: "move"}, {Kind: "scroll"}} {
		if events := captureRemoteInput(t, input); len(events) != 0 {
			t.Fatal("zero motion must be a no-op")
		}
	}
}

func TestPartialRemoteInputReleasesOnlyAcceptedDowns(t *testing.T) {
	for _, accepted := range []uint32{1, 2, 3} {
		var batches [][]winInput
		err := sendRemoteInput(RemoteInput{Kind: "key", Key: "ctrl+a"}, func(events []winInput) (uint32, error) {
			batches = append(batches, append([]winInput(nil), events...))
			if len(batches) == 1 {
				return accepted, syscall.ERROR_ACCESS_DENIED
			}
			return uint32(len(events)), nil
		})
		if !errors.Is(err, syscall.ERROR_ACCESS_DENIED) || len(batches) != 2 {
			t.Fatalf("partial batch did not report failure and dispatch releases: %v, batches=%d", err, len(batches))
		}
		want := []winKeyboardInput{{VK: 0x11, Flags: 2}}
		if accepted == 2 {
			want = append([]winKeyboardInput{{VK: 0x41, Flags: 2}}, want...)
		}
		if len(batches[1]) != len(want) {
			t.Fatalf("accepted %d events: recovery len %d, want %d", accepted, len(batches[1]), len(want))
		}
		for i, key := range want {
			if got := keyboardData(&batches[1][i]); got != key {
				t.Fatalf("recovery event %+v, want %+v", got, key)
			}
		}
	}
	var recovery []winInput
	calls := 0
	if err := sendRemoteInput(RemoteInput{Kind: "click", Button: "right"}, func(events []winInput) (uint32, error) {
		calls++
		if calls == 1 {
			return 1, nil
		}
		recovery = append(recovery, events...)
		return uint32(len(events)), nil
	}); err == nil || len(recovery) != 1 || recovery[0].Data.Flags != 16 {
		t.Fatalf("partial right click recovery incorrect: %+v, %v", recovery, err)
	}
}

func TestBlockedSendInputReportsUsefulErrorWithoutRetry(t *testing.T) {
	calls := 0
	err := sendRemoteInput(RemoteInput{Kind: "key", Key: "enter"}, func([]winInput) (uint32, error) {
		calls++
		return 0, syscall.Errno(0) // UIPI can fail without useful GetLastError.
	})
	if err == nil || !strings.Contains(err.Error(), "administrator") || calls != 1 {
		t.Fatalf("blocked SendInput response: calls=%d, err=%v", calls, err)
	}
}

func TestRemoteInputDispatchIsSerialized(t *testing.T) {
	var active, overlap atomic.Int32
	var wg sync.WaitGroup
	errors := make(chan error, 32)
	dispatch := func(events []winInput) (uint32, error) {
		if active.Add(1) != 1 {
			overlap.Add(1)
		}
		runtime.Gosched()
		active.Add(-1)
		return uint32(len(events)), nil
	}
	for range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errors <- sendRemoteInput(RemoteInput{Kind: "key", Key: "enter"}, dispatch)
		}()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	if overlap.Load() != 0 {
		t.Fatal("native input batches overlapped")
	}
}
