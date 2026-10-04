package platform

import (
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeAudioState struct {
	mu        sync.Mutex
	id        string
	muted     bool
	scalar    float32
	setError  error
	ignoreSet bool
	refs      atomic.Int32
}

func (s *fakeAudioState) mute() bool { s.mu.Lock(); defer s.mu.Unlock(); return s.muted }
func (s *fakeAudioState) unmute()    { s.mu.Lock(); s.muted = false; s.mu.Unlock() }

type fakeAudioEndpoint struct {
	state    *fakeAudioState
	released bool
}

func (e *fakeAudioEndpoint) ID() string           { return e.state.id }
func (e *fakeAudioEndpoint) Muted() (bool, error) { return e.state.mute(), nil }
func (e *fakeAudioEndpoint) SetMute(value bool) error {
	e.state.mu.Lock()
	defer e.state.mu.Unlock()
	if e.state.setError != nil {
		return e.state.setError
	}
	if !e.state.ignoreSet {
		e.state.muted = value
	}
	return nil
}
func (e *fakeAudioEndpoint) Release() {
	if !e.released {
		e.released = true
		e.state.refs.Add(-1)
	}
}

type fakeAudioDevices struct {
	mu           sync.Mutex
	states       []*fakeAudioState
	err          error
	closed       atomic.Int32
	journal      map[string]bool
	journalError error
}

func (d *fakeAudioDevices) Active() ([]audioOutputEndpoint, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.err != nil {
		return nil, d.err
	}
	result := make([]audioOutputEndpoint, 0, len(d.states))
	for _, state := range d.states {
		state.refs.Add(1)
		result = append(result, &fakeAudioEndpoint{state: state})
	}
	return result, nil
}
func (d *fakeAudioDevices) Close() { d.closed.Add(1) }
func (d *fakeAudioDevices) Remember(id string, muted bool) error {
	if d.journalError != nil {
		return d.journalError
	}
	if d.journal == nil {
		d.journal = make(map[string]bool)
	}
	d.journal[id] = muted
	return nil
}
func (d *fakeAudioDevices) Restored(id string) error { delete(d.journal, id); return nil }

func TestAudioOutputSnapshotsGuardAndRestoreWithoutChangingVolume(t *testing.T) {
	speaker := &fakeAudioState{id: "speaker", scalar: 1}
	headphones := &fakeAudioState{id: "headphones", muted: true, scalar: .42}
	d := &fakeAudioDevices{states: []*fakeAudioState{speaker, headphones}}
	lease, err := newSilentAudioOutput(d)
	if err != nil {
		t.Fatal(err)
	}
	if !speaker.mute() || !headphones.mute() || d.journal["speaker"] || !d.journal["headphones"] {
		t.Fatal("mute/snapshots not confirmed")
	}
	speaker.unmute()
	usb := &fakeAudioState{id: "USB", scalar: .81}
	d.states = []*fakeAudioState{speaker, usb}
	if err := lease.guard(); err != nil {
		t.Fatal(err)
	}
	if !speaker.mute() || !usb.mute() {
		t.Fatal("external unmute or new endpoint escaped guard")
	}
	if err := lease.restore(); err != nil {
		t.Fatal(err)
	}
	if speaker.mute() || usb.mute() || !headphones.mute() {
		t.Fatal("original mute states not restored, including removed endpoint")
	}
	if speaker.scalar != 1 || headphones.scalar != .42 || usb.scalar != .81 {
		t.Fatal("volume scalar changed")
	}
	for _, state := range []*fakeAudioState{speaker, headphones, usb} {
		if state.refs.Load() != 0 {
			t.Fatalf("endpoint %s leaked %d refs", state.id, state.refs.Load())
		}
	}
	if len(d.journal) != 0 || d.closed.Load() != 1 {
		t.Fatal("restoration did not clear journal/release devices")
	}
}

func TestAudioOutputFailedStartRestoresPartialMute(t *testing.T) {
	for _, kind := range []string{"set-error", "unconfirmed", "journal-error", "no-output"} {
		t.Run(kind, func(t *testing.T) {
			a := &fakeAudioState{id: "a"}
			b := &fakeAudioState{id: "b"}
			d := &fakeAudioDevices{states: []*fakeAudioState{a, b}}
			switch kind {
			case "set-error":
				b.setError = errors.New("driver refused")
			case "unconfirmed":
				b.ignoreSet = true
			case "journal-error":
				d.journalError = errors.New("disk refused")
			case "no-output":
				d.states = nil
			}
			lease, err := newSilentAudioOutput(d)
			if err == nil || lease != nil {
				t.Fatal("failed mute acquired a lease")
			}
			if a.mute() || b.mute() || a.refs.Load() != 0 || b.refs.Load() != 0 || d.closed.Load() != 1 {
				t.Fatal("failed start leaked endpoint state or references")
			}
			if kind == "set-error" && len(d.journal) != 1 {
				t.Fatal("unconfirmed restoration lost its recovery record")
			}
		})
	}
}

func TestAudioOutputHelperEOFRestoresAndGuardFailureReports(t *testing.T) {
	for _, guardFailure := range []bool{false, true} {
		t.Run(map[bool]string{false: "parent EOF", true: "guard failed"}[guardFailure], func(t *testing.T) {
			input, parent := io.Pipe()
			output, child := io.Pipe()
			defer input.Close()
			defer parent.Close()
			defer output.Close()
			defer child.Close()
			state := &fakeAudioState{id: "speaker", scalar: 1}
			d := &fakeAudioDevices{states: []*fakeAudioState{state}}
			finished := make(chan error, 1)
			go func() {
				finished <- serveAudioOutputHelper(input, child, func() (audioOutputDevices, error) { return d, nil }, time.Millisecond)
			}()
			if err := json.NewEncoder(parent).Encode(audioOutputMessage{Type: "begin", Nonce: strings.Repeat("a", 64)}); err != nil {
				t.Fatal(err)
			}
			decoder := json.NewDecoder(output)
			var ready audioOutputMessage
			if err := decoder.Decode(&ready); err != nil || ready.Type != "ready" || !state.mute() {
				t.Fatalf("ready: %+v %v", ready, err)
			}
			if guardFailure {
				d.mu.Lock()
				d.err = errors.New("endpoint lost")
				d.mu.Unlock()
			} else {
				_ = parent.Close()
			}
			var restored audioOutputMessage
			if err := decoder.Decode(&restored); err != nil || restored.Type != "restored" {
				t.Fatalf("restored: %+v %v", restored, err)
			}
			select {
			case err := <-finished:
				if (err != nil) != guardFailure || (restored.Error != "") != guardFailure {
					t.Fatalf("guard status mismatch: %v %+v", err, restored)
				}
			case <-time.After(time.Second):
				t.Fatal("EOF did not finish helper")
			}
			if state.mute() || state.scalar != 1 || state.refs.Load() != 0 || d.closed.Load() != 1 {
				t.Fatal("helper termination failed to restore/release")
			}
		})
	}
}

func TestAudioOutputHelperInvalidHandshakeNeverOpensOutputs(t *testing.T) {
	for _, packet := range []string{"", `{\"type\":\"begin\"}`, `{\"type\":\"begin\",\"nonce\":\"` + strings.Repeat("a", 64) + `\",\"unknown\":true}`} {
		opened := false
		err := serveAudioOutputHelper(strings.NewReader(packet), io.Discard, func() (audioOutputDevices, error) { opened = true; return nil, nil }, time.Millisecond)
		if err == nil || opened {
			t.Fatal("invalid handshake touched audio outputs")
		}
	}
}

func TestAudioOutputRecoveryContinuesAfterMissingEndpoint(t *testing.T) {
	for i := 0; i < 20; i++ {
		state := &fakeAudioState{id: "speaker", muted: true}
		journal := map[string]bool{"missing USB": false, "speaker": false}
		err := recoverAudioOutputJournal(journal, func(id string) (audioOutputEndpoint, error) {
			if id == "missing USB" {
				return nil, errors.New("endpoint removed")
			}
			state.refs.Add(1)
			return &fakeAudioEndpoint{state: state}, nil
		}, func(id string) error { delete(journal, id); return nil })
		if err == nil || state.mute() || state.refs.Load() != 0 || len(journal) != 1 {
			t.Fatal("missing endpoint prevented available endpoint recovery")
		}
		if _, ok := journal["missing USB"]; !ok {
			t.Fatal("unrestored endpoint lost recovery record")
		}
	}
}
