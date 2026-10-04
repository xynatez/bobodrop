//go:build windows

package platform

import (
	"context"
	"os"
	"runtime"
	"testing"
	"time"
	"unsafe"
)

func TestWin32StructureLayout(t *testing.T) {
	// Sizes/offsets from the Windows SDK ABI, including MSG.lPrivate and the
	// Vista+ NOTIFYICONDATAW fields. Run on both pointer widths when building.
	wantClass, wantMessage, wantNotify := uintptr(80), uintptr(48), uintptr(976)
	wantIconOffset, wantTipOffset, wantBalloonOffset := uintptr(32), uintptr(40), uintptr(968)
	if unsafe.Sizeof(uintptr(0)) == 4 {
		wantClass, wantMessage, wantNotify = 48, 32, 956
		wantIconOffset, wantTipOffset, wantBalloonOffset = 20, 24, 952
	}
	if got := unsafe.Sizeof(winClassEx{}); got != wantClass {
		t.Fatalf("WNDCLASSEXW size = %d, want %d", got, wantClass)
	}
	if got := unsafe.Sizeof(winMessage{}); got != wantMessage {
		t.Fatalf("MSG size = %d, want %d", got, wantMessage)
	}
	var notify notifyIconData
	if got := unsafe.Sizeof(notify); got != wantNotify {
		t.Fatalf("NOTIFYICONDATAW size = %d, want %d", got, wantNotify)
	}
	if unsafe.Offsetof(notify.Icon) != wantIconOffset || unsafe.Offsetof(notify.Tip) != wantTipOffset ||
		unsafe.Offsetof(notify.BalloonIcon) != wantBalloonOffset {
		t.Fatal("NOTIFYICONDATAW field offsets differ from the Windows ABI")
	}
}

func TestHiddenTrayWindowMessageLoop(t *testing.T) {
	// Exercise a real Win32 callback and message queue without adding a tray
	// icon, opening a menu, launching an app, or touching the user's clipboard.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	opened := make(chan struct{}, 1)
	s, err := createTrayWindow(TrayOptions{OnOpen: func() { opened <- struct{}{} }})
	if err != nil {
		t.Fatal(err)
	}
	defer s.cleanup()
	if result, _, err := postMessageW.Call(s.window, wmCommand, menuOpen, 0); result == 0 {
		t.Fatal(err)
	}
	if result, _, err := postMessageW.Call(s.window, wmClose, 0, 0); result == 0 {
		t.Fatal(err)
	}
	if err := s.messageLoop(); err != nil {
		t.Fatal(err)
	}
	if !s.destroyed || s.iconAdded {
		t.Fatal("hidden window did not cleanly shut down")
	}
	select {
	case <-opened:
	case <-time.After(time.Second):
		t.Fatal("native WM_COMMAND did not invoke the open callback")
	}
}

func TestCanceledTrayDoesNotCreateWindow(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := RunTray(ctx, TrayOptions{}); err != nil {
		t.Fatal(err)
	}
}

func TestTrayDoubleClickAndExplicitQuitCallbacks(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	opened := make(chan struct{}, 1)
	quit := make(chan struct{}, 2)
	s, err := createTrayWindow(TrayOptions{
		OnOpen: func() { opened <- struct{}{} },
		OnQuit: func() { quit <- struct{}{} },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer s.cleanup()
	s.version4 = true
	for _, message := range []struct{ id, wParam, lParam uintptr }{
		{wmTrayCallback, 0, trayIconID<<16 | wmLeftDouble},
		{wmCommand, menuQuit, 0},
		{wmCommand, menuQuit, 0},
	} {
		if result, _, err := postMessageW.Call(s.window, message.id, message.wParam, message.lParam); result == 0 {
			t.Fatal(err)
		}
	}
	if err := s.messageLoop(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-opened:
	case <-time.After(time.Second):
		t.Fatal("native version-4 double click did not invoke open")
	}
	select {
	case <-quit:
	case <-time.After(time.Second):
		t.Fatal("explicit exit did not invoke quit")
	}
	if len(quit) != 0 {
		t.Fatal("duplicate exit event invoked quit twice")
	}
}

func TestTrayContextCancellation(t *testing.T) {
	if os.Getenv("LOCALDROP_TEST_TRAY") != "1" {
		t.Skip("opt-in smoke displays a tray icon briefly; does not open other UI or touch clipboard")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()
	if err := RunTray(ctx, TrayOptions{}); err != nil {
		t.Fatal(err)
	}
	if ctx.Err() == nil {
		t.Fatal("tray exited before context cancellation")
	}
	trayWindows.Range(func(_, _ any) bool {
		t.Error("tray window retained after context cancellation")
		return true
	})
}

func TestUnicodeMovableWindowsMemory(t *testing.T) {
	// Test the memory APIs used by the clipboard without opening the shared
	// clipboard or replacing any of the user's existing formats.
	text := "Clipboard LocalDrop — 日本語 📷🦷"
	encoded, err := encodeUTF16Z(text)
	if err != nil {
		t.Fatal(err)
	}
	size := uintptr(len(encoded) * 2)
	handle, _, err := globalAlloc.Call(gmMoveableZeroInit, size)
	if handle == 0 {
		t.Fatal(err)
	}
	defer globalFree.Call(handle)
	data, _, err := globalLock.Call(handle)
	if data == 0 {
		t.Fatal(err)
	}
	defer globalUnlock.Call(handle)
	moveMemory.Call(data, uintptr(unsafe.Pointer(&encoded[0])), size)
	read := make([]uint16, len(encoded))
	moveMemory.Call(uintptr(unsafe.Pointer(&read[0])), data, size)
	got, err := decodeUTF16Z(read)
	if err != nil || got != text {
		t.Fatalf("native UTF-16 memory round trip = %q, %v; want %q", got, err, text)
	}
}
