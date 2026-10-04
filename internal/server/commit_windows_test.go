//go:build windows

package server

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWindowsAtomicCommitLongPathAndCollision(t *testing.T) {
	dir := filepath.Join(t.TempDir(), strings.Repeat("folder", 20))
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	name := strings.Repeat("filename", 20) + " 🦷.txt"
	original := []byte("existing completed file")
	if err := os.WriteFile(filepath.Join(dir, name), original, 0600); err != nil {
		t.Fatal(err)
	}
	temp, err := os.CreateTemp(dir, ".localdrop-*.part")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = temp.Write([]byte("new completed file"))
	_ = temp.Close()
	finalName, finalPath, err := commitFile(temp.Name(), dir, name)
	if err != nil {
		t.Fatal(err)
	}
	if finalName == name || !strings.Contains(finalName, " (2)") {
		t.Fatalf("collision name %q", finalName)
	}
	if data, err := os.ReadFile(filepath.Join(dir, name)); err != nil || !bytes.Equal(data, original) {
		t.Fatal("existing file was overwritten")
	}
	if data, err := os.ReadFile(finalPath); err != nil || string(data) != "new completed file" {
		t.Fatal("long Unicode path commit failed")
	}
	if _, err := os.Stat(temp.Name()); !os.IsNotExist(err) {
		t.Fatal("temp was not atomically moved")
	}
}
