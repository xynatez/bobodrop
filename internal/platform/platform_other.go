//go:build !windows

package platform

import (
	"context"
	"errors"
)

var errWindowsOnly = errors.New("Fitur ini hanya tersedia di Windows.")

func ClipboardText() (string, error) { return "", errWindowsOnly }
func SetClipboardText(string) error  { return errWindowsOnly }
func OpenURL(string) error           { return errWindowsOnly }
func OpenFolder(string) error        { return errWindowsOnly }

func RunTray(ctx context.Context, _ TrayOptions) error {
	if ctx == nil {
		return errWindowsOnly
	}
	<-ctx.Done()
	return nil
}
