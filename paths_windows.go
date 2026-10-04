//go:build windows

package main

import (
	"fmt"
	"syscall"
	"unsafe"
)

func downloadsDirectory() (string, error) {
	// FOLDERID_Downloads honors redirected Windows known folders.
	downloadsID := syscall.GUID{Data1: 0x374DE290, Data2: 0x123F, Data3: 0x4565, Data4: [8]byte{0x91, 0x64, 0x39, 0xC4, 0x92, 0x5E, 0x46, 0x7B}}
	var result *uint16
	hr, _, _ := syscall.NewLazyDLL("shell32.dll").NewProc("SHGetKnownFolderPath").Call(
		uintptr(unsafe.Pointer(&downloadsID)), 0, 0, uintptr(unsafe.Pointer(&result)),
	)
	if hr != 0 {
		return "", fmt.Errorf("SHGetKnownFolderPath HRESULT 0x%08x", hr)
	}
	if result == nil {
		return "", fmt.Errorf("SHGetKnownFolderPath menghasilkan alamat kosong")
	}
	defer syscall.NewLazyDLL("ole32.dll").NewProc("CoTaskMemFree").Call(uintptr(unsafe.Pointer(result)))
	length := 0
	for length < 32768 && *(*uint16)(unsafe.Add(unsafe.Pointer(result), length*2)) != 0 {
		length++
	}
	return syscall.UTF16ToString(unsafe.Slice(result, length)), nil
}
