//go:build !windows

package server

import "os"

func moveNoReplace(source, destination string) error {
	if err := os.Link(source, destination); err != nil {
		return err
	}
	_ = os.Remove(source)
	return nil
}
