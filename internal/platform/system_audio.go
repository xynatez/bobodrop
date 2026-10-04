package platform

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"
)

const (
	systemAudioHelperFlag     = "--localdrop-system-audio-helper"
	systemAudioSampleRate     = 48000
	systemAudioChannels       = 2
	systemAudioFrameBytes     = 4
	systemAudioStartupTimeout = 4 * time.Second
	systemAudioKillDelay      = 250 * time.Millisecond
	systemAudioCloseTimeout   = time.Second
)

type SystemAudioCapture struct {
	PCM        io.ReadCloser
	SampleRate int
	Channels   int
	Close      func() error
	Done       <-chan struct{}
}

type systemAudioBegin struct {
	Type  string `json:"type"`
	Nonce string `json:"nonce"`
}

type systemAudioReady struct {
	Type       string `json:"type"`
	Nonce      string `json:"nonce"`
	SampleRate int    `json:"sampleRate"`
	Channels   int    `json:"channels"`
}

// A JSON decoder must not consume PCM buffered after the readiness newline.
func readSystemAudioLine(reader *bufio.Reader, value any) error {
	line, err := reader.ReadSlice('\n')
	if err != nil {
		return errors.Join(errors.New("invalid system audio helper handshake"), err)
	}
	check := json.NewDecoder(bytes.NewReader(line))
	token, err := check.Token()
	if err != nil || token != json.Delim('{') {
		return errors.New("invalid system audio helper handshake")
	}
	fields := make(map[string]bool)
	for check.More() {
		token, err = check.Token()
		key, ok := token.(string)
		if err != nil || !ok || fields[key] {
			return errors.New("duplicate or invalid system audio helper field")
		}
		fields[key] = true
		var raw json.RawMessage
		if err := check.Decode(&raw); err != nil {
			return err
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(line))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return errors.New("extra system audio helper handshake data")
	}
	return nil
}

type systemAudioSource interface {
	Run(context.Context, io.Writer) error
	Close() error
}

func serveSystemAudioHelper(input io.Reader, output io.Writer, open func(context.Context) (systemAudioSource, error)) error {
	reader := bufio.NewReaderSize(input, 4096)
	var begin systemAudioBegin
	if err := readSystemAudioLine(reader, &begin); err != nil {
		return err
	}
	key, err := hex.DecodeString(begin.Nonce)
	if err != nil || len(key) != 32 || len(begin.Nonce) != 64 || begin.Type != "begin" {
		return errors.New("invalid system audio helper handshake")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		// EOF or any further input ends the private capture request.
		_, _ = reader.ReadByte()
		cancel()
	}()
	if ctx.Err() != nil {
		return nil
	}
	source, err := open(ctx)
	if err != nil {
		return err
	}
	if ctx.Err() != nil {
		return source.Close()
	}
	err = json.NewEncoder(output).Encode(systemAudioReady{Type: "ready", Nonce: begin.Nonce, SampleRate: systemAudioSampleRate, Channels: systemAudioChannels})
	if err == nil {
		err = source.Run(ctx, output)
	}
	return errors.Join(err, source.Close())
}

type systemAudioChild struct {
	command  *exec.Cmd
	input    io.WriteCloser
	output   io.ReadCloser
	finished chan struct{}
	done     chan struct{}
	doneOnce sync.Once
	stopOnce sync.Once
	stopping atomic.Bool
	err      error
}

func (c *systemAudioChild) signalDone() { c.doneOnce.Do(func() { close(c.done) }) }

func (c *systemAudioChild) stop() {
	c.stopOnce.Do(func() {
		c.stopping.Store(true)
		c.signalDone()
		_ = c.input.Close()
		_ = c.output.Close()
		go func() {
			timer := time.NewTimer(systemAudioKillDelay)
			defer timer.Stop()
			select {
			case <-c.finished:
			case <-timer.C:
				// This helper owns capture only, so termination cannot strand mute.
				_ = c.command.Process.Kill()
			}
		}()
	})
}

func (c *systemAudioChild) Close() error {
	c.stop()
	timer := time.NewTimer(systemAudioCloseTimeout)
	defer timer.Stop()
	select {
	case <-c.finished:
		return c.err
	case <-timer.C:
		return errors.New("system audio helper termination timed out")
	}
}

type systemAudioPCM struct {
	reader *bufio.Reader
	close  func() error
}

func (p *systemAudioPCM) Read(data []byte) (int, error) { return p.reader.Read(data) }
func (p *systemAudioPCM) Close() error                  { return p.close() }

func beginSystemAudioChild(ctx context.Context, command *exec.Cmd, timeout time.Duration) (*SystemAudioCapture, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var nonce [32]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, err
	}
	key := hex.EncodeToString(nonce[:])
	input, err := command.StdinPipe()
	if err != nil {
		return nil, err
	}
	// Own stdout ourselves: exec.Wait must not close unread readiness/PCM bytes.
	output, writer, err := os.Pipe()
	if err != nil {
		_ = input.Close()
		return nil, err
	}
	command.Stdout = writer
	command.Stderr = io.Discard
	if err := command.Start(); err != nil {
		_ = input.Close()
		_ = output.Close()
		_ = writer.Close()
		return nil, err
	}
	_ = writer.Close()
	child := &systemAudioChild{command: command, input: input, output: output, finished: make(chan struct{}), done: make(chan struct{})}
	go func() {
		err := command.Wait()
		if child.stopping.Load() {
			err = nil
		}
		child.err = err
		_ = input.Close()
		child.signalDone()
		close(child.finished)
	}()
	go func() {
		select {
		case <-ctx.Done():
			child.stop()
		case <-child.finished:
		}
	}()
	reader := bufio.NewReaderSize(output, 4096)
	ready := make(chan error, 1)
	go func() {
		var status systemAudioReady
		err := readSystemAudioLine(reader, &status)
		if err == nil && (status.Type != "ready" || status.Nonce != key || status.SampleRate != systemAudioSampleRate || status.Channels != systemAudioChannels) {
			err = errors.New("Windows did not confirm stereo 48000 Hz system audio capture")
		}
		ready <- err
	}()
	written := make(chan error, 1)
	go func() { written <- json.NewEncoder(input).Encode(systemAudioBegin{Type: "begin", Nonce: key}) }()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	fail := func(err error) (*SystemAudioCapture, error) {
		child.stop()
		// A rejected startup has no stream to drain or mute state to restore.
		// Reap it before returning, including cancellation during activation.
		_ = command.Process.Kill()
		return nil, errors.Join(err, child.Close())
	}
	select {
	case err := <-written:
		if err != nil {
			return fail(err)
		}
	case <-ctx.Done():
		return fail(ctx.Err())
	case <-timer.C:
		return fail(errors.New("system audio helper startup timed out"))
	}
	select {
	case err := <-ready:
		if err != nil {
			return fail(err)
		}
	case <-ctx.Done():
		return fail(ctx.Err())
	case <-timer.C:
		return fail(errors.New("system audio helper startup timed out"))
	}
	select {
	case <-child.done:
		return fail(errors.New("system audio helper stopped during startup"))
	default:
	}
	return &SystemAudioCapture{PCM: &systemAudioPCM{reader: reader, close: child.Close}, SampleRate: systemAudioSampleRate, Channels: systemAudioChannels, Close: child.Close, Done: child.done}, nil
}
