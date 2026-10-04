package platform

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

var systemAudioTestPCM = []byte{1, 2, 3, 4, 10, 34, 123, 125}

type fakeSystemAudioSource struct {
	closed atomic.Int32
	marker string
}

func (s *fakeSystemAudioSource) Run(ctx context.Context, output io.Writer) error {
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		if _, err := output.Write(systemAudioTestPCM); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (s *fakeSystemAudioSource) Close() error {
	s.closed.Add(1)
	if s.marker != "" {
		return os.WriteFile(s.marker, []byte("closed"), 0600)
	}
	return nil
}

func fakeSystemAudioCommand(mode string) *exec.Cmd {
	command := exec.Command(os.Args[0], "-test.run=^TestSystemAudioFakeSubprocess$")
	command.Env = append(os.Environ(), "LOCALDROP_SYSTEM_AUDIO_FAKE_PROCESS="+mode)
	return command
}

func TestSystemAudioFakeSubprocess(t *testing.T) {
	mode := os.Getenv("LOCALDROP_SYSTEM_AUDIO_FAKE_PROCESS")
	if mode == "" {
		return
	}
	if mode == "native" {
		_, err := RunSystemAudioHelper([]string{systemAudioHelperFlag})
		if err != nil {
			if path := os.Getenv("LOCALDROP_SYSTEM_AUDIO_NATIVE_ERROR"); path != "" {
				_ = os.WriteFile(path, []byte(err.Error()), 0600)
			}
			os.Exit(1)
		}
		os.Exit(0)
	}
	if mode == "parent" {
		capture, err := beginSystemAudioChild(context.Background(), fakeSystemAudioCommand("helper"), time.Second)
		if err != nil {
			os.Exit(2)
		}
		_ = capture
		os.Exit(0)
	}
	if mode == "slow" {
		time.Sleep(time.Minute)
		os.Exit(0)
	}
	if mode == "wrong" || mode == "duplicate" || mode == "stuck" || mode == "exit" {
		var begin systemAudioBegin
		if err := json.NewDecoder(os.Stdin).Decode(&begin); err != nil {
			os.Exit(3)
		}
		if mode == "duplicate" {
			_, _ = io.WriteString(os.Stdout, `{"type":"ready","type":"ready","nonce":"`+begin.Nonce+`","sampleRate":48000,"channels":2}`+"\n")
		} else {
			rate := systemAudioSampleRate
			if mode == "wrong" {
				rate = 44100
			}
			_ = json.NewEncoder(os.Stdout).Encode(systemAudioReady{Type: "ready", Nonce: begin.Nonce, SampleRate: rate, Channels: 2})
		}
		if mode == "exit" {
			time.Sleep(100 * time.Millisecond)
			os.Exit(7)
		}
		time.Sleep(time.Minute)
		os.Exit(0)
	}
	err := serveSystemAudioHelper(os.Stdin, os.Stdout, func(context.Context) (systemAudioSource, error) {
		if mode == "failure" {
			return nil, errors.New("fake capture failed")
		}
		return &fakeSystemAudioSource{marker: os.Getenv("LOCALDROP_SYSTEM_AUDIO_FAKE_CLOSED")}, nil
	})
	if err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}

func TestSystemAudioChildPreservesBufferedPCMAndCloses(t *testing.T) {
	capture, err := beginSystemAudioChild(context.Background(), fakeSystemAudioCommand("helper"), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = capture.Close() })
	if capture.SampleRate != 48000 || capture.Channels != 2 {
		t.Fatal("incorrect capture format")
	}
	data := make([]byte, len(systemAudioTestPCM))
	if _, err := io.ReadFull(capture.PCM, data); err != nil || !bytes.Equal(data, systemAudioTestPCM) {
		t.Fatal("readiness consumed PCM", data, err)
	}
	if err := capture.PCM.Close(); err != nil {
		t.Fatal(err)
	}
	if err := capture.Close(); err != nil {
		t.Fatal("Close is not idempotent", err)
	}
	select {
	case <-capture.Done:
	default:
		t.Fatal("Close did not signal Done")
	}
}

func TestSystemAudioChildFailuresStayBounded(t *testing.T) {
	for _, mode := range []string{"slow", "wrong", "duplicate", "failure"} {
		t.Run(mode, func(t *testing.T) {
			timeout := time.Second
			if mode == "slow" {
				timeout = 20 * time.Millisecond
			}
			start := time.Now()
			capture, err := beginSystemAudioChild(context.Background(), fakeSystemAudioCommand(mode), timeout)
			if capture != nil || err == nil {
				t.Fatal("invalid helper accepted")
			}
			if time.Since(start) > 2*time.Second {
				t.Fatal("startup exceeded deadline")
			}
		})
	}
}

func TestSystemAudioChildCancelAndStuckClose(t *testing.T) {
	for _, mode := range []string{"helper", "stuck"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			capture, err := beginSystemAudioChild(ctx, fakeSystemAudioCommand(mode), time.Second)
			if err != nil {
				t.Fatal(err)
			}
			cancel()
			select {
			case <-capture.Done:
			case <-time.After(time.Second):
				t.Fatal("cancel did not signal Done")
			}
			start := time.Now()
			if err := capture.Close(); err != nil {
				t.Fatal(err)
			}
			if time.Since(start) > systemAudioCloseTimeout {
				t.Fatal("Close exceeded deadline")
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if capture, err := beginSystemAudioChild(ctx, fakeSystemAudioCommand("helper"), time.Second); capture != nil || !errors.Is(err, context.Canceled) {
		t.Fatal("pre-canceled capture started", err)
	}
}

func TestSystemAudioChildUnexpectedExitSignalsDone(t *testing.T) {
	capture, err := beginSystemAudioChild(context.Background(), fakeSystemAudioCommand("exit"), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-capture.Done:
	case <-time.After(2 * time.Second):
		t.Fatal("helper exit did not signal Done")
	}
	if err := capture.Close(); err == nil {
		t.Fatal("unexpected helper exit status lost")
	}
}

func TestSystemAudioChildStartupCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	time.AfterFunc(20*time.Millisecond, cancel)
	start := time.Now()
	capture, err := beginSystemAudioChild(ctx, fakeSystemAudioCommand("slow"), time.Second)
	if capture != nil || !errors.Is(err, context.Canceled) {
		t.Fatal("startup cancellation did not return context error", err)
	}
	if time.Since(start) > 500*time.Millisecond {
		t.Fatal("startup cancellation exceeded bound")
	}
}

func TestSystemAudioParentExitClosesPrivateInput(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "closed.txt")
	command := fakeSystemAudioCommand("parent")
	command.Env = append(command.Env, "LOCALDROP_SYSTEM_AUDIO_FAKE_CLOSED="+marker)
	if err := command.Run(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(marker); err == nil && string(data) == "closed" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("parent exit did not stop helper")
}

func TestSystemAudioHelperHandshakeValidation(t *testing.T) {
	for _, line := range []string{"", "{}\n", `{"type":"begin","nonce":"` + strings.Repeat("g", 64) + `"}` + "\n", `{"type":"begin","nonce":"` + strings.Repeat("a", 64) + `","extra":1}` + "\n", `{"type":"begin","type":"begin","nonce":"` + strings.Repeat("a", 64) + `"}` + "\n", strings.Repeat("x", 5000) + "\n"} {
		opened := false
		err := serveSystemAudioHelper(strings.NewReader(line), io.Discard, func(context.Context) (systemAudioSource, error) { opened = true; return nil, nil })
		if err == nil || opened {
			t.Fatal("invalid handshake opened capture")
		}
	}
}

func TestSystemAudioHelperEOFStopsSource(t *testing.T) {
	input, parent := io.Pipe()
	output, child := io.Pipe()
	defer input.Close()
	defer parent.Close()
	defer output.Close()
	defer child.Close()
	source := &fakeSystemAudioSource{}
	finished := make(chan error, 1)
	go func() {
		finished <- serveSystemAudioHelper(input, child, func(context.Context) (systemAudioSource, error) { return source, nil })
	}()
	if err := json.NewEncoder(parent).Encode(systemAudioBegin{Type: "begin", Nonce: strings.Repeat("a", 64)}); err != nil {
		t.Fatal(err)
	}
	reader := bufio.NewReader(output)
	var ready systemAudioReady
	if err := readSystemAudioLine(reader, &ready); err != nil || ready.SampleRate != 48000 {
		t.Fatal(ready, err)
	}
	go func() { _, _ = io.Copy(io.Discard, reader) }()
	_ = parent.Close()
	select {
	case err := <-finished:
		if err != nil || source.closed.Load() != 1 {
			t.Fatal("EOF cleanup failed", err)
		}
	case <-time.After(time.Second):
		t.Fatal("EOF did not stop source")
	}
}
