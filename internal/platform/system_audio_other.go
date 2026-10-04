//go:build !windows

package platform

import (
	"context"
	"errors"
)

func BeginSystemAudioCapture(ctx context.Context) (*SystemAudioCapture, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return nil, errors.New("system audio capture requires Windows build 20348 or later")
}

func RunSystemAudioHelper(args []string) (bool, error) {
	if len(args) > 0 && args[0] == systemAudioHelperFlag {
		return true, errors.New("system audio capture helper requires Windows")
	}
	return false, nil
}
