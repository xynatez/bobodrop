//go:build windows

package platform

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"unsafe"
)

var (
	shellExecuteW  = shell32.NewProc("ShellExecuteW")
	coInitializeEx = ole32.NewProc("CoInitializeEx")
	coUninitialize = ole32.NewProc("CoUninitialize")
)

func OpenURL(raw string) error {
	target, err := browserURL(raw)
	if err != nil {
		return err
	}
	return shellOpen(target)
}

func OpenFolder(path string) error {
	if path == "" {
		return errors.New("Folder penyimpanan belum tersedia.")
	}
	if _, err := encodeUTF16Z(path); err != nil {
		return err
	}
	target, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("Lokasi folder penyimpanan tidak valid: %w", err)
	}
	info, err := os.Stat(target)
	if err != nil {
		return fmt.Errorf("Folder penyimpanan tidak dapat dibuka: %w", err)
	}
	if !info.IsDir() {
		return errors.New("Lokasi penyimpanan bukan folder.")
	}
	return shellOpen(target)
}

func shellOpen(target string) error {
	file, err := encodeUTF16Z(target)
	if err != nil {
		return err
	}
	verb, _ := encodeUTF16Z("open")
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	result, _, _ := coInitializeEx.Call(0, 0x0002|0x0004) // STA; disable legacy OLE DDE.
	if int32(result) >= 0 {
		defer coUninitialize.Call()
	} else if uint32(result) != 0x80010106 { // An existing COM apartment can still open ordinary URLs/folders.
		return fmt.Errorf("Menyiapkan Windows Shell gagal (0x%08x).", uint32(result))
	}
	result, _, _ = shellExecuteW.Call(0, uintptr(unsafe.Pointer(&verb[0])),
		uintptr(unsafe.Pointer(&file[0])), 0, 0, 1)
	if result <= 32 {
		return fmt.Errorf("Windows tidak dapat membuka tautan atau folder (kode %d).", result)
	}
	return nil
}
