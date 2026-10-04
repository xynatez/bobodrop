//go:build windows

package server

import (
	"os"
	"strings"
	"syscall"
	"unsafe"
)

var moveFileEx = syscall.NewLazyDLL("kernel32.dll").NewProc("MoveFileExW")

// MoveFileEx without REPLACE_EXISTING atomically publishes the completed file
// and works on Windows filesystems that do not support hard links.
func moveNoReplace(source, destination string) error {
	from, err := syscall.UTF16PtrFromString(extendedPath(source))
	if err != nil {
		return err
	}
	to, err := syscall.UTF16PtrFromString(extendedPath(destination))
	if err != nil {
		return err
	}
	result, _, callErr := moveFileEx.Call(uintptr(unsafe.Pointer(from)), uintptr(unsafe.Pointer(to)), 0)
	if result != 0 {
		return nil
	}
	if callErr == syscall.ERROR_FILE_EXISTS || callErr == syscall.ERROR_ALREADY_EXISTS {
		return os.ErrExist
	}
	return &os.LinkError{Op: "rename", Old: source, New: destination, Err: callErr}
}

func extendedPath(value string) string {
	if strings.HasPrefix(value, `\\?\`) {
		return value
	}
	if strings.HasPrefix(value, `\\`) {
		return `\\?\UNC\` + strings.TrimPrefix(value, `\\`)
	}
	return `\\?\` + value
}
