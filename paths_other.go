//go:build !windows

package main

import (
	"os"
	"path/filepath"
)

func downloadsDirectory() (string, error) {
	home, err := os.UserHomeDir()
	return filepath.Join(home, "Downloads"), err
}
