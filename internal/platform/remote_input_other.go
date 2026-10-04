//go:build !windows

package platform

func SendRemoteInput(RemoteInput) error { return errWindowsOnly }
