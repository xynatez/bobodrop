//go:build windows

package platform

import (
	"errors"
	"runtime"
	"sync"
	"time"
	"unsafe"
)

const (
	cfUnicodeText      = 13
	gmMoveableZeroInit = 0x0042
	maxClipboardBytes  = 64 << 20
)

var (
	clipboardMu                sync.Mutex
	openClipboard              = user32.NewProc("OpenClipboard")
	closeClipboard             = user32.NewProc("CloseClipboard")
	emptyClipboard             = user32.NewProc("EmptyClipboard")
	getClipboardData           = user32.NewProc("GetClipboardData")
	setClipboardData           = user32.NewProc("SetClipboardData")
	isClipboardFormatAvailable = user32.NewProc("IsClipboardFormatAvailable")
	globalAlloc                = kernel32.NewProc("GlobalAlloc")
	globalFree                 = kernel32.NewProc("GlobalFree")
	globalLock                 = kernel32.NewProc("GlobalLock")
	globalUnlock               = kernel32.NewProc("GlobalUnlock")
	globalSize                 = kernel32.NewProc("GlobalSize")
	moveMemory                 = kernel32.NewProc("RtlMoveMemory")
)

func openClipboardRetry(owner uintptr) error {
	var lastErr error
	for attempt := 0; attempt < 12; attempt++ {
		ok, _, err := openClipboard.Call(owner)
		if ok != 0 {
			return nil
		}
		lastErr = err
		if attempt < 11 {
			time.Sleep(25 * time.Millisecond)
		}
	}
	return windowsError("Membuka clipboard Windows; coba lagi setelah aplikasi lain selesai menyalin", lastErr)
}

// ClipboardText reads CF_UNICODETEXT. Access is serialized and stays on the
// thread that opened the clipboard until CloseClipboard completes.
func ClipboardText() (string, error) {
	clipboardMu.Lock()
	defer clipboardMu.Unlock()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if err := openClipboardRetry(0); err != nil {
		return "", err
	}
	defer closeClipboard.Call()
	available, _, _ := isClipboardFormatAvailable.Call(cfUnicodeText)
	if available == 0 {
		return "", errors.New("Clipboard Windows tidak berisi teks.")
	}
	handle, _, err := getClipboardData.Call(cfUnicodeText)
	if handle == 0 {
		return "", windowsError("Membaca clipboard Windows", err)
	}
	size, _, err := globalSize.Call(handle)
	if size == 0 {
		return "", windowsError("Membaca ukuran clipboard Windows", err)
	}
	if size < 2 || size > maxClipboardBytes || size%2 != 0 {
		return "", errors.New("Ukuran teks clipboard Windows tidak valid atau melebihi 64 MiB.")
	}
	data, _, err := globalLock.Call(handle)
	if data == 0 {
		return "", windowsError("Mengakses teks clipboard Windows", err)
	}
	defer globalUnlock.Call(handle)
	encoded := make([]uint16, int(size/2))
	// Copy native memory while locked, without giving Go's GC a pointer into a
	// Win32 allocation. Call keeps the Go destination alive until copying ends.
	moveMemory.Call(uintptr(unsafe.Pointer(&encoded[0])), data, size)
	return decodeUTF16Z(encoded)
}

// SetClipboardText transfers a movable UTF-16 memory block to Windows. The
// temporary hidden window supplies the non-NULL clipboard owner Win32 needs.
func SetClipboardText(text string) error {
	encoded, err := encodeUTF16Z(text)
	if err != nil {
		return err
	}
	if len(encoded) > maxClipboardBytes/2 {
		return errors.New("Teks clipboard melebihi 64 MiB.")
	}
	clipboardMu.Lock()
	defer clipboardMu.Unlock()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	class, _ := encodeUTF16Z("STATIC")
	title, _ := encodeUTF16Z("LocalDrop Clipboard")
	owner, _, err := createWindowExW.Call(0, uintptr(unsafe.Pointer(&class[0])),
		uintptr(unsafe.Pointer(&title[0])), 0x80000000, 0, 0, 0, 0, 0, 0, 0, 0)
	if owner == 0 {
		return windowsError("Membuat pemilik clipboard Windows", err)
	}
	defer destroyWindow.Call(owner)
	if err := openClipboardRetry(owner); err != nil {
		return err
	}
	defer closeClipboard.Call()

	handle, _, err := globalAlloc.Call(gmMoveableZeroInit, uintptr(len(encoded)*2))
	if handle == 0 {
		return windowsError("Menyiapkan teks clipboard Windows", err)
	}
	owned := true
	defer func() {
		if owned {
			globalFree.Call(handle)
		}
	}()
	data, _, err := globalLock.Call(handle)
	if data == 0 {
		return windowsError("Mengakses memori clipboard Windows", err)
	}
	moveMemory.Call(data, uintptr(unsafe.Pointer(&encoded[0])), uintptr(len(encoded)*2))
	globalUnlock.Call(handle)
	if ok, _, err := emptyClipboard.Call(); ok == 0 {
		return windowsError("Mengosongkan clipboard Windows", err)
	}
	if result, _, err := setClipboardData.Call(cfUnicodeText, handle); result == 0 {
		return windowsError("Menyalin teks ke clipboard Windows", err)
	}
	owned = false // Windows owns the memory after SetClipboardData succeeds.
	return nil
}
