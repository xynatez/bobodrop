//go:build !windows

package platform

import "errors"

func BeginSilentAudioOutput() (*AudioOutputLease, error) {
	return nil, errors.New("phone-only audio output requires Windows")
}

func RunAudioOutputHelper(args []string) (bool, error) {
	if len(args) > 0 && args[0] == audioOutputHelperFlag {
		return true, errors.New("audio output helper requires Windows")
	}
	return false, nil
}
