package server

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type audioOutputProbe struct {
	lease          *AudioOutputLease
	revoked        atomic.Int32
	closed         atomic.Int32
	revokeUnlocked atomic.Bool
	done           chan struct{}
	finished       chan struct{}
	doneOnce       sync.Once
}

func newAudioOutputProbe(s *Server) *audioOutputProbe {
	p := &audioOutputProbe{done: make(chan struct{}), finished: make(chan struct{})}
	p.lease = &AudioOutputLease{Done: p.done, Revoke: func() {
		if s.mu.TryLock() {
			p.revokeUnlocked.Store(true)
			s.mu.Unlock()
		}
		p.revoked.Add(1)
		p.doneOnce.Do(func() { close(p.done) })
	}, Close: func() error {
		_ = s.state(true)
		if p.closed.Add(1) == 1 {
			close(p.finished)
		}
		return nil
	}}
	return p
}
func (p *audioOutputProbe) wait(t *testing.T) {
	t.Helper()
	select {
	case <-p.finished:
	case <-time.After(time.Second):
		t.Fatal("audio output cleanup blocked under server mutex")
	}
	if p.revoked.Load() != 1 || p.closed.Load() != 1 || p.revokeUnlocked.Load() {
		t.Fatalf("audio lease cleanup: revoke=%d close=%d unlocked=%v", p.revoked.Load(), p.closed.Load(), p.revokeUnlocked.Load())
	}
}
func phoneOnlyPacket(f mediaFixture) map[string]any {
	return map[string]any{"type": "media-start", "mode": "audio", "audio": true, "phoneOnly": true, "target": f.viewer.connectionID}
}
func disconnectAudioTestClient(f mediaFixture, c *wsClient) {
	f.s.mu.Lock()
	f.s.disconnectLocked(c, "disconnected")
	f.s.mu.Unlock()
}
func assertPhoneOnlyEvent(t *testing.T, c *wsClient, kind string) string {
	t.Helper()
	select {
	case data := <-c.events:
		var event struct {
			Type, ID  string
			PhoneOnly bool `json:"phoneOnly"`
		}
		if err := json.Unmarshal(data, &event); err != nil || event.Type != kind || !event.PhoneOnly {
			t.Fatalf("phone-only event missing: %s %v", data, err)
		}
		return event.ID
	default:
		t.Fatalf("missing %s", kind)
	}
	return ""
}

func TestAudioOutputConfirmationAndCleanup(t *testing.T) {
	for _, stop := range []string{"participant stop", "owner disconnect", "viewer disconnect", "server close", "rotation", "guard failure", "event overflow"} {
		t.Run(stop, func(t *testing.T) {
			f := newMediaFixture(t)
			p := newAudioOutputProbe(f.s)
			f.s.config.AudioOutput = func() (*AudioOutputLease, error) { _ = f.s.state(true); return p.lease, nil }
			sendMedia(t, f.s, f.host, phoneOnlyPacket(f))
			id := assertPhoneOnlyEvent(t, f.host, "media-started")
			if assertPhoneOnlyEvent(t, f.viewer, "media-invite") != id {
				t.Fatal("participant media IDs differ")
			}
			for _, host := range []bool{false, true} {
				state := f.s.state(host)
				if state.Media == nil || !state.Media.PhoneOnly {
					t.Fatal("state lacked confirmed phoneOnly")
				}
			}
			switch stop {
			case "participant stop":
				sendMedia(t, f.s, f.viewer, map[string]any{"type": "media-stop", "id": id})
			case "owner disconnect":
				disconnectAudioTestClient(f, f.host)
			case "viewer disconnect":
				disconnectAudioTestClient(f, f.viewer)
			case "server close":
				_ = f.s.Close()
			case "rotation":
				f.s.rotate(httptest.NewRecorder())
			case "guard failure":
				p.doneOnce.Do(func() { close(p.done) })
			case "event overflow":
				for i := 0; i < mediaQueueCapacity; i++ {
					f.viewer.events <- []byte(`{}`)
				}
				sendMedia(t, f.s, f.host, signalPacket(id, "offer", description("offer")))
			}
			p.wait(t)
			if f.s.state(true).Media != nil {
				t.Fatal("stop retained active media")
			}
		})
	}
}

func TestAudioOutputRequestValidationAndFailedFactory(t *testing.T) {
	for _, kind := range []string{"no audio", "nil factory", "peer", "factory error", "partial lease", "nil lease", "already done"} {
		t.Run(kind, func(t *testing.T) {
			f := newMediaFixture(t)
			p := newAudioOutputProbe(f.s)
			called := false
			f.s.config.AudioOutput = func() (*AudioOutputLease, error) {
				called = true
				switch kind {
				case "factory error":
					return p.lease, errors.New("failed")
				case "nil lease":
					return nil, nil
				case "partial lease":
					return &AudioOutputLease{Revoke: p.lease.Revoke, Close: p.lease.Close}, errors.New("partial")
				case "already done":
					p.doneOnce.Do(func() { close(p.done) })
				}
				return p.lease, nil
			}
			packet := phoneOnlyPacket(f)
			client := f.host
			switch kind {
			case "no audio":
				packet["mode"] = "screen"
				packet["audio"] = false
			case "nil factory":
				f.s.config.AudioOutput = nil
			case "peer":
				client = f.viewer
			}
			sendMedia(t, f.s, client, packet)
			popMedia(t, client, "media-error")
			if f.s.state(true).Media != nil {
				t.Fatal("failed native lease published active media")
			}
			if called && kind != "nil lease" {
				p.wait(t)
			}
			if !called && p.revoked.Load() != 0 {
				t.Fatal("rejected request acquired native state")
			}
			noMedia(t, f.viewer)
		})
	}
}

func TestAudioOutputRestorationWarningDoesNotStopNewSession(t *testing.T) {
	f := newMediaFixture(t)
	entered, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	lease := &AudioOutputLease{Revoke: func() {}, Close: func() error {
		close(entered)
		<-release
		defer close(finished)
		return errors.New("private native detail must not escape")
	}}
	f.s.mu.Lock()
	f.s.releaseAudioOutputLocked(lease)
	f.s.mu.Unlock()
	<-entered
	newSession := &mediaSession{id: "new-current-session", owner: f.host, viewer: f.viewer, mode: "screen", startCanceled: make(chan struct{})}
	f.s.mu.Lock()
	f.s.media = newSession
	f.s.mu.Unlock()
	close(release)
	<-finished
	deadline := time.After(time.Second)
	for _, host := range []*wsClient{f.host, f.otherHost} {
		select {
		case data := <-host.events:
			var event struct{ Type, Message string }
			if err := json.Unmarshal(data, &event); err != nil || event.Type != "media-output-warning" || event.Message != "Windows belum mengonfirmasi pemulihan suara. Periksa pengaturan mute Windows sebelum berbagi lagi." {
				t.Fatalf("restoration warning is missing or leaks native detail: %s %v", data, err)
			}
		case <-deadline:
			t.Fatal("host restoration warning was not delivered")
		}
	}
	noMedia(t, f.viewer)
	noMedia(t, f.otherPeer)
	f.s.mu.Lock()
	retained := f.s.media == newSession
	f.s.mu.Unlock()
	if !retained {
		t.Fatal("restoration warning stopped a later session")
	}
}

func TestAudioOutputPendingStartCanceledWithoutBlockingServer(t *testing.T) {
	for _, cancel := range []string{"disconnect", "close", "rotation"} {
		t.Run(cancel, func(t *testing.T) {
			f := newMediaFixture(t)
			p := newAudioOutputProbe(f.s)
			entered, release, returned := make(chan struct{}), make(chan struct{}), make(chan struct{})
			f.s.config.AudioOutput = func() (*AudioOutputLease, error) { close(entered); <-release; return p.lease, nil }
			data, _ := json.Marshal(phoneOnlyPacket(f))
			go func() { f.s.handleSocketMessage(f.host, data); close(returned) }()
			<-entered
			if f.s.state(true).Media != nil {
				t.Fatal("pending mute published confirmation")
			}
			sendMedia(t, f.s, f.otherHost, phoneOnlyPacket(f))
			popMedia(t, f.otherHost, "media-error")
			switch cancel {
			case "disconnect":
				disconnectAudioTestClient(f, f.viewer)
			case "close":
				_ = f.s.Close()
			case "rotation":
				f.s.rotate(httptest.NewRecorder())
			}
			select {
			case <-returned:
			case <-time.After(time.Second):
				t.Fatal("cancel waited for native factory")
			}
			close(release)
			p.wait(t)
			if f.s.state(true).Media != nil {
				t.Fatal("late lease resurrected canceled media")
			}
			noMedia(t, f.host, f.viewer)
		})
	}
}

func TestAudioOutputFactoryTimeoutRestoresLateLease(t *testing.T) {
	f := newMediaFixture(t)
	p := newAudioOutputProbe(f.s)
	release := make(chan struct{})
	f.s.config.AudioOutput = func() (*AudioOutputLease, error) { <-release; return p.lease, nil }
	m := &mediaSession{owner: f.host, viewer: f.viewer, pending: true, startCanceled: make(chan struct{})}
	f.s.mu.Lock()
	f.s.media = m
	if f.s.beginAudioOutputLocked(m, 10*time.Millisecond) {
		t.Fatal("timed out factory confirmed mute")
	}
	f.s.mu.Unlock()
	popMedia(t, f.host, "media-error")
	close(release)
	p.wait(t)
	if f.s.state(true).Media != nil {
		t.Fatal("timed out lease resurrected media")
	}
}

func TestAudioOutputFailureReleasesRelayAndNeverConfirms(t *testing.T) {
	f, relay := newRelayFixture(t)
	p := newAudioOutputProbe(f.s)
	f.s.config.AudioOutput = func() (*AudioOutputLease, error) { return p.lease, errors.New("mute failed") }
	sendMedia(t, f.s, f.host, phoneOnlyPacket(f))
	popMedia(t, f.host, "media-error")
	noMedia(t, f.viewer)
	p.wait(t)
	relay.waitClosed(t)
	if f.s.state(true).Media != nil {
		t.Fatal("failed mute kept relay media active")
	}
}

func TestAudioOutputOptionalModeDoesNotAcquireOrConfirmMute(t *testing.T) {
	f := newMediaFixture(t)
	f.s.config.AudioOutput = func() (*AudioOutputLease, error) { t.Fatal("ordinary audio invoked mute helper"); return nil, nil }
	startTestMedia(t, f, "audio", true, false)
	if f.s.state(true).Media.PhoneOnly || f.s.state(false).Media.PhoneOnly {
		t.Fatal("ordinary audio claimed phone-only mute")
	}
}

func TestAudioOutputLateLeaseDoesNotStopReplacementMedia(t *testing.T) {
	f := newMediaFixture(t)
	p := newAudioOutputProbe(f.s)
	entered, release, returned := make(chan struct{}), make(chan struct{}), make(chan struct{})
	f.s.config.AudioOutput = func() (*AudioOutputLease, error) { close(entered); <-release; return p.lease, nil }
	data, _ := json.Marshal(phoneOnlyPacket(f))
	go func() { f.s.handleSocketMessage(f.host, data); close(returned) }()
	<-entered
	f.s.mu.Lock()
	f.s.stopMediaLocked("canceled")
	f.s.mu.Unlock()
	<-returned
	id := startTestMedia(t, f, "screen", false, false)
	close(release)
	p.wait(t)
	if media := f.s.state(true).Media; media == nil || media.ID != id || media.PhoneOnly {
		t.Fatal("late mute lease replaced/stopped new media")
	}
	noMedia(t, f.host, f.viewer)
}
