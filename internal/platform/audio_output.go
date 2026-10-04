package platform

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"
)

const audioOutputHelperFlag = "--localdrop-audio-output-helper"

// AudioOutputLease restores each endpoint's original mute state when revoked.
// Revoke only signals cleanup; Close waits a bounded time for restoration.
type AudioOutputLease struct {
	Revoke func()
	Close  func() error
	Done   <-chan struct{}
	stdin  io.WriteCloser
}

type audioOutputEndpoint interface {
	ID() string
	Muted() (bool, error)
	SetMute(bool) error
	Release()
}

type audioOutputDevices interface {
	Active() ([]audioOutputEndpoint, error)
	Close()
}

type audioOutputJournal interface {
	Remember(string, bool) error
	Restored(string) error
}

type audioOutputSnapshot struct {
	endpoint audioOutputEndpoint
	muted    bool
}

type silentAudioOutput struct {
	devices   audioOutputDevices
	snapshots map[string]audioOutputSnapshot
}

func newSilentAudioOutput(devices audioOutputDevices) (*silentAudioOutput, error) {
	output := &silentAudioOutput{devices: devices, snapshots: make(map[string]audioOutputSnapshot)}
	if err := output.guard(); err != nil {
		return nil, errors.Join(err, output.restore())
	}
	return output, nil
}

func (o *silentAudioOutput) guard() error {
	endpoints, err := o.devices.Active()
	if err != nil {
		return err
	}
	// Active returns owned references. Keep one per endpoint until restoration,
	// including devices removed or disabled during the lease.
	defer func() {
		for _, endpoint := range endpoints {
			if endpoint != nil {
				endpoint.Release()
			}
		}
	}()
	if len(endpoints) == 0 {
		return errors.New("no active Windows audio output")
	}
	for i, endpoint := range endpoints {
		id := endpoint.ID()
		if id == "" {
			return errors.New("audio endpoint has no identifier")
		}
		muted, err := endpoint.Muted()
		if err != nil {
			return err
		}
		if _, exists := o.snapshots[id]; !exists {
			if len(o.snapshots) >= 256 {
				return errors.New("too many audio output endpoints")
			}
			if journal, ok := o.devices.(audioOutputJournal); ok {
				if err := journal.Remember(id, muted); err != nil {
					return err
				}
			}
			o.snapshots[id] = audioOutputSnapshot{endpoint: endpoint, muted: muted}
			endpoints[i] = nil
		}
		if !muted {
			if err := endpoint.SetMute(true); err != nil {
				return err
			}
		}
		if confirmed, err := endpoint.Muted(); err != nil || !confirmed {
			return errors.Join(errors.New("Windows did not confirm output mute"), err)
		}
	}
	return nil
}

func (o *silentAudioOutput) restore() error {
	var failures []error
	for id, snapshot := range o.snapshots {
		restored := false
		if err := snapshot.endpoint.SetMute(snapshot.muted); err != nil {
			failures = append(failures, err)
		} else if actual, err := snapshot.endpoint.Muted(); err != nil || actual != snapshot.muted {
			failures = append(failures, errors.Join(errors.New("Windows did not confirm mute restoration"), err))
		} else {
			restored = true
		}
		if journal, ok := o.devices.(audioOutputJournal); ok && restored {
			if err := journal.Restored(id); err != nil {
				failures = append(failures, err)
			}
		}
		snapshot.endpoint.Release()
		delete(o.snapshots, id)
	}
	o.devices.Close()
	return errors.Join(failures...)
}

func recoverAudioOutputJournal(originals map[string]bool, resolve func(string) (audioOutputEndpoint, error), restored func(string) error) error {
	var failures []error
	for id, muted := range originals {
		endpoint, err := resolve(id)
		if err == nil {
			err = endpoint.SetMute(muted)
			if err == nil {
				actual, readErr := endpoint.Muted()
				err = readErr
				if err == nil && actual != muted {
					err = errors.New("Windows did not confirm recovered mute state")
				}
			}
		}
		if endpoint != nil {
			endpoint.Release()
		}
		if err == nil {
			err = restored(id)
		}
		if err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

type audioOutputMessage struct {
	Type  string `json:"type"`
	Nonce string `json:"nonce,omitempty"`
	Error string `json:"error,omitempty"`
}

// EOF on this private pipe also covers an ungraceful exit of the parent.
func serveAudioOutputHelper(input io.Reader, output io.Writer, open func() (audioOutputDevices, error), interval time.Duration) error {
	decoder := json.NewDecoder(io.LimitReader(input, 4096))
	decoder.DisallowUnknownFields()
	var begin audioOutputMessage
	if err := decoder.Decode(&begin); err != nil || begin.Type != "begin" || len(begin.Nonce) != 64 || begin.Error != "" {
		return errors.New("invalid audio output helper handshake")
	}
	ended := make(chan struct{})
	go func() {
		var extra any
		_ = decoder.Decode(&extra)
		close(ended)
	}()
	select {
	case <-ended:
		return nil
	default:
	}
	devices, err := open()
	if err != nil {
		_ = json.NewEncoder(output).Encode(audioOutputMessage{Type: "error", Error: err.Error()})
		return err
	}
	lease, err := newSilentAudioOutput(devices)
	if err != nil {
		_ = json.NewEncoder(output).Encode(audioOutputMessage{Type: "error", Error: err.Error()})
		return err
	}
	var reason error
	select {
	case <-ended:
		return lease.restore()
	default:
	}
	if err := json.NewEncoder(output).Encode(audioOutputMessage{Type: "ready", Nonce: begin.Nonce}); err != nil {
		return errors.Join(err, lease.restore())
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
wait:
	for {
		select {
		case <-ended:
			break wait
		case <-ticker.C:
			if err := lease.guard(); err != nil {
				reason = fmt.Errorf("audio output guard: %w", err)
				break wait
			}
		}
	}
	err = errors.Join(reason, lease.restore())
	message := audioOutputMessage{Type: "restored"}
	if err != nil {
		message.Error = err.Error()
	}
	return errors.Join(err, json.NewEncoder(output).Encode(message))
}
