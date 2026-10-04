package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

type mediaRelayProbe struct {
	lease          *MediaRelayLease
	revokes        atomic.Int32
	closes         atomic.Int32
	revokeUnlocked atomic.Bool
	closed         chan struct{}
}

func newMediaRelayProbe(s *Server) *mediaRelayProbe {
	p := &mediaRelayProbe{closed: make(chan struct{})}
	p.lease = &MediaRelayLease{
		HostICE:   []MediaICEServer{{URLs: []string{"turn:127.0.0.1:49152?transport=tcp"}, Username: "owner-secret-user", Credential: "owner-secret-password"}},
		ViewerICE: []MediaICEServer{{URLs: []string{"turn:192.168.1.25:49152?transport=tcp"}, Username: "viewer-secret-user", Credential: "viewer-secret-password"}},
		Revoke: func() {
			if s.mu.TryLock() {
				p.revokeUnlocked.Store(true)
				s.mu.Unlock()
			}
			p.revokes.Add(1)
		},
		Close: func() error {
			// Re-entering Server here proves full cleanup does not hold its mutex.
			_ = s.state(true)
			if p.closes.Add(1) == 1 {
				close(p.closed)
			}
			return nil
		},
	}
	return p
}

func (p *mediaRelayProbe) waitClosed(t *testing.T) {
	t.Helper()
	select {
	case <-p.closed:
	case <-time.After(time.Second):
		t.Fatal("relay cleanup did not finish outside the server mutex")
	}
	if p.revokes.Load() != 1 || p.closes.Load() != 1 || p.revokeUnlocked.Load() {
		t.Fatalf("relay lifecycle: revoke=%d close=%d revoke unlocked=%v", p.revokes.Load(), p.closes.Load(), p.revokeUnlocked.Load())
	}
}

func newRelayFixture(t *testing.T) (mediaFixture, *mediaRelayProbe) {
	t.Helper()
	f := newMediaFixture(t)
	f.host.remoteIP, f.host.requestIP = "127.0.0.1", "127.0.0.1"
	f.viewer.remoteIP, f.viewer.requestIP = "192.168.1.30", "192.168.1.25"
	p := newMediaRelayProbe(f.s)
	f.s.config.MediaRelay = func(MediaRelayRequest) (*MediaRelayLease, error) { return p.lease, nil }
	return f, p
}

func relayStartPacket(f mediaFixture) map[string]any {
	return map[string]any{"type": "media-start", "target": f.viewer.connectionID, "mode": "audio", "audio": true}
}

func TestMediaRelayCredentialsReachOnlyTheirParticipant(t *testing.T) {
	f, p := newRelayFixture(t)
	var request MediaRelayRequest
	var factoryUnlocked bool
	f.s.config.MediaRelay = func(r MediaRelayRequest) (*MediaRelayLease, error) {
		request = r
		if f.s.mu.TryLock() {
			factoryUnlocked = true
			f.s.mu.Unlock()
		}
		return p.lease, nil
	}
	sendMedia(t, f.s, f.host, relayStartPacket(f))
	if request != (MediaRelayRequest{ServerIP: "192.168.1.25", HostIP: "127.0.0.1", ViewerIP: "192.168.1.30"}) || factoryUnlocked {
		t.Fatalf("unexpected factory request or mutex ownership: %+v unlocked=%v", request, factoryUnlocked)
	}
	var id string
	for _, tc := range []struct {
		client *wsClient
		kind   string
		ice    []MediaICEServer
		secret string
	}{{f.host, "media-started", p.lease.HostICE, "viewer-secret"}, {f.viewer, "media-invite", p.lease.ViewerICE, "owner-secret"}} {
		data := <-tc.client.events
		var event struct {
			Type       string           `json:"type"`
			ID         string           `json:"id"`
			ICEServers []MediaICEServer `json:"iceServers"`
		}
		if err := json.Unmarshal(data, &event); err != nil || event.Type != tc.kind || !reflect.DeepEqual(event.ICEServers, tc.ice) || bytes.Contains(data, []byte(tc.secret)) {
			t.Fatalf("participant received wrong relay credentials: type=%q decode=%v", event.Type, err)
		}
		if id != "" && event.ID != id {
			t.Fatal("relay participants received different media IDs")
		}
		id = event.ID
	}
	noMedia(t, f.otherHost, f.otherPeer)
	for _, c := range []*wsClient{f.host, f.viewer, f.otherHost, f.otherPeer} {
		data := <-c.queue
		if bytes.Contains(data, []byte("iceServers")) || bytes.Contains(data, []byte("secret")) || bytes.Contains(data, []byte("turn:")) {
			t.Fatal("relay configuration escaped through broadcast state")
		}
	}
	for _, host := range []bool{true, false} {
		data, _ := json.Marshal(f.s.state(host))
		if bytes.Contains(data, []byte("iceServers")) || bytes.Contains(data, []byte("secret")) || bytes.Contains(data, []byte("turn:")) {
			t.Fatal("relay configuration escaped through a state snapshot")
		}
	}
	// Matching device IDs and a second host tab still grant no media authority.
	for _, c := range []*wsClient{f.otherHost, f.otherPeer} {
		sendMedia(t, f.s, c, signalPacket(id, "ice", iceCandidate()))
		popMedia(t, c, "media-error")
		sendMedia(t, f.s, c, map[string]any{"type": "media-stop", "id": id})
		popMedia(t, c, "media-error")
	}
	if p.revokes.Load() != 0 {
		t.Fatal("an unrelated tab revoked the selected session")
	}
	sendMedia(t, f.s, f.host, map[string]any{"type": "media-stop", "id": id})
	p.waitClosed(t)
}

func TestMediaRelayRequiresCurrentDetectedRecipientAddress(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(mediaFixture)
	}{
		{"missing server IP", func(f mediaFixture) { f.viewer.requestIP = "" }},
		{"loopback server IP", func(f mediaFixture) { f.viewer.requestIP = "127.0.0.1" }},
		{"undetected server IP", func(f mediaFixture) { f.viewer.requestIP = "192.168.1.99" }},
		{"removed server IP", func(f mediaFixture) { f.s.UpdateAddresses([]NetworkAddress{{Name: "Wi-Fi", IP: "192.168.1.26"}}) }},
		{"empty detected snapshot", func(f mediaFixture) { f.s.UpdateAddresses(nil) }},
		{"allowed but undetected server IP", func(f mediaFixture) { f.s.allowedHosts["192.168.1.99"] = true; f.viewer.requestIP = "192.168.1.99" }},
		{"missing owner source", func(f mediaFixture) { f.host.remoteIP = "" }},
		{"remote owner source", func(f mediaFixture) { f.host.remoteIP = "192.168.1.25" }},
		{"IPv6 owner source", func(f mediaFixture) { f.host.remoteIP = "::1" }},
		{"missing viewer source", func(f mediaFixture) { f.viewer.remoteIP = "" }},
		{"multicast viewer source", func(f mediaFixture) { f.viewer.remoteIP = "224.0.0.1" }},
		{"hostname viewer source", func(f mediaFixture) { f.viewer.remoteIP = "iphone.local" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, _ := newRelayFixture(t)
			calls := 0
			f.s.config.MediaRelay = func(MediaRelayRequest) (*MediaRelayLease, error) { calls++; return nil, nil }
			tc.change(f)
			sendMedia(t, f.s, f.host, relayStartPacket(f))
			popMedia(t, f.host, "media-error")
			if calls != 0 || f.s.state(true).Media != nil {
				t.Fatal("invalid or stale source binding reached the relay factory")
			}
			noMedia(t, f.viewer, f.otherHost, f.otherPeer)
		})
	}
	// Select the address used by this recipient, not the first interface.
	f, p := newRelayFixture(t)
	f.s.UpdateAddresses([]NetworkAddress{{Name: "Ethernet", IP: "10.1.1.5"}, {Name: "Wi-Fi", IP: "192.168.1.25"}})
	f.s.config.MediaRelay = func(r MediaRelayRequest) (*MediaRelayLease, error) {
		if r.ServerIP != f.viewer.requestIP {
			t.Fatalf("factory guessed another interface: %s", r.ServerIP)
		}
		return p.lease, nil
	}
	startTestMedia(t, f, "audio", true, false)
}

func TestMediaRelayDirectOnlyCompatibilityAndClientIPPoisoning(t *testing.T) {
	f := newMediaFixture(t) // No endpoint metadata or relay factory.
	sendMedia(t, f.s, f.host, relayStartPacket(f))
	for _, c := range []*wsClient{f.host, f.viewer} {
		data := <-c.events
		if bytes.Contains(data, []byte("iceServers")) {
			t.Fatal("direct-only compatibility changed the start packet")
		}
	}
	if f.s.state(true).Media == nil {
		t.Fatal("nil factory rejected the existing direct path")
	}
	f, _ = newRelayFixture(t)
	calls := 0
	f.s.config.MediaRelay = func(MediaRelayRequest) (*MediaRelayLease, error) { calls++; return nil, nil }
	packet := relayStartPacket(f)
	packet["serverIP"], packet["hostIP"], packet["viewerIP"] = "192.168.1.25", "127.0.0.1", "192.168.1.30"
	sendMedia(t, f.s, f.host, packet)
	popMedia(t, f.host, "media-error")
	if calls != 0 || f.s.state(true).Media != nil {
		t.Fatal("client-supplied endpoint JSON reached the factory")
	}
}

func TestMediaRelayFactoryFailureCleansPartialLeaseWithoutSecrets(t *testing.T) {
	for _, outcome := range []string{"nil lease", "factory error", "partial lease"} {
		t.Run(outcome, func(t *testing.T) {
			f, p := newRelayFixture(t)
			f.s.config.MediaRelay = func(MediaRelayRequest) (*MediaRelayLease, error) {
				switch outcome {
				case "nil lease":
					return nil, nil
				case "factory error":
					return nil, errors.New("private: owner-secret-password 192.168.1.25:49152")
				default:
					return p.lease, errors.New("private: owner-secret-password 192.168.1.25:49152")
				}
			}
			sendMedia(t, f.s, f.host, relayStartPacket(f))
			event := popMedia(t, f.host, "media-error")
			if strings.Contains(event.Message, "private") || strings.Contains(event.Message, "secret") || strings.Contains(event.Message, "192.168") || f.s.state(true).Media != nil {
				t.Fatal("factory failure leaked details or created a media session")
			}
			noMedia(t, f.viewer, f.otherHost, f.otherPeer)
			if outcome == "partial lease" {
				if p.revokes.Load() != 1 {
					t.Fatal("partial lease was not immediately revoked")
				}
				p.waitClosed(t)
			} else if p.revokes.Load() != 0 || p.closes.Load() != 0 {
				t.Fatal("failure without a lease called unrelated cleanup")
			}
		})
	}
}

func fillMediaQueue(c *wsClient) {
	for len(c.events) < cap(c.events) {
		c.events <- []byte(`{"type":"test-queued-event"}`)
	}
}

func TestMediaRelaySetupQueueFailureRevokesLease(t *testing.T) {
	for _, failure := range []string{"owner overflow", "viewer overflow", "owner closes during factory", "viewer closes during factory"} {
		t.Run(failure, func(t *testing.T) {
			f, p := newRelayFixture(t)
			switch failure {
			case "owner overflow":
				fillMediaQueue(f.host)
			case "viewer overflow":
				fillMediaQueue(f.viewer)
			default:
				f.s.config.MediaRelay = func(MediaRelayRequest) (*MediaRelayLease, error) {
					if failure == "owner closes during factory" {
						f.host.close()
					} else {
						f.viewer.close()
					}
					return p.lease, nil
				}
			}
			sendMedia(t, f.s, f.host, relayStartPacket(f))
			if f.s.state(true).Media != nil || p.revokes.Load() != 1 {
				t.Fatal("start acknowledgement failure kept an active relay")
			}
			p.waitClosed(t)
			noMedia(t, f.otherHost, f.otherPeer)
		})
	}
}

func TestMediaRelayEveryTerminationRevokesImmediately(t *testing.T) {
	for _, action := range []string{"owner stop", "viewer stop", "owner disconnect", "viewer disconnect", "owner overflow", "viewer overflow", "message flood", "closed owner input", "closed recipient signal", "rotation", "server close"} {
		t.Run(action, func(t *testing.T) {
			f, p := newRelayFixture(t)
			var inputCalls atomic.Int32
			f.s.config.ControlInput = func(RemoteInput) error { inputCalls.Add(1); return nil }
			id := startTestMedia(t, f, "screen", false, true)
			switch action {
			case "owner stop", "viewer stop":
				c := f.host
				if action == "viewer stop" {
					c = f.viewer
				}
				sendMedia(t, f.s, c, map[string]any{"type": "media-stop", "id": id})
			case "owner disconnect", "viewer disconnect":
				c := f.host
				if action == "viewer disconnect" {
					c = f.viewer
				}
				f.s.mu.Lock()
				f.s.disconnectLocked(c, "Perangkat terputus.")
				f.s.mu.Unlock()
			case "owner overflow":
				fillMediaQueue(f.host)
				sendMedia(t, f.s, f.viewer, signalPacket(id, "ice", iceCandidate()))
			case "viewer overflow":
				fillMediaQueue(f.viewer)
				sendMedia(t, f.s, f.host, signalPacket(id, "ice", iceCandidate()))
			case "message flood":
				f.viewer.messageRate = rateBucket{updated: time.Now().Add(time.Second)}
				sendMedia(t, f.s, f.viewer, map[string]any{"type": "hello", "name": "flood"})
			case "closed owner input":
				f.host.close()
				sendMedia(t, f.s, f.viewer, inputPacket(id, RemoteInput{Kind: "click", Button: "left"}))
			case "closed recipient signal":
				f.viewer.close()
				sendMedia(t, f.s, f.host, signalPacket(id, "ice", iceCandidate()))
			case "rotation":
				f.s.rotate(httptest.NewRecorder())
			case "server close":
				_ = f.s.Close()
			}
			if p.revokes.Load() != 1 || f.s.state(true).Media != nil {
				t.Fatal("termination did not immediately revoke the active lease")
			}
			p.waitClosed(t)
			f.s.mu.Lock()
			f.s.stopMediaLocked("Berbagi dihentikan.")
			f.s.mu.Unlock()
			_ = f.s.Close()
			sendMedia(t, f.s, f.viewer, inputPacket(id, RemoteInput{Kind: "click", Button: "left"}))
			if p.revokes.Load() != 1 || p.closes.Load() != 1 || inputCalls.Load() != 0 {
				t.Fatal("termination repeated cleanup or allowed stale remote input")
			}
		})
	}
}

func TestMediaRelayCloseWaitsForUnlockAndNeverBlocksSession(t *testing.T) {
	f, p := newRelayFixture(t)
	entered, release := make(chan struct{}), make(chan struct{})
	defer close(release)
	p.lease.Close = func() error {
		close(entered)
		_ = f.s.state(true)
		<-release
		p.closes.Add(1)
		close(p.closed)
		return nil
	}
	startTestMedia(t, f, "audio", true, false)
	f.s.mu.Lock()
	f.s.stopMediaLocked("Berbagi dihentikan.")
	if p.revokes.Load() != 1 {
		f.s.mu.Unlock()
		t.Fatal("Revoke was deferred until after unlock")
	}
	select {
	case <-entered:
		f.s.mu.Unlock()
		t.Fatal("full cleanup entered while the revoking critical section was held")
	case <-time.After(20 * time.Millisecond):
	}
	f.s.mu.Unlock()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("full cleanup did not start after unlock")
	}
	available := make(chan struct{})
	go func() { _ = f.s.state(true); close(available) }()
	select {
	case <-available:
	case <-time.After(time.Second):
		t.Fatal("blocking full cleanup held the session mutex")
	}
	if p.revokeUnlocked.Load() {
		t.Fatal("immediate Revoke ran outside the session mutex")
	}
}

func TestMediaRelayWebSocketEndpointsIgnoreForwardedAndQueryAddresses(t *testing.T) {
	s := newTestServer(t)
	p := newMediaRelayProbe(s)
	requests := make(chan MediaRelayRequest, 1)
	s.config.MediaRelay = func(r MediaRelayRequest) (*MediaRelayLease, error) { requests <- r; return p.lease, nil }
	hostCookie, peerCookie := hostSession(t, s), peerSession(t, s)
	web := httptest.NewServer(s.Handler())
	defer web.Close()
	actual, err := url.Parse(web.URL)
	if err != nil {
		t.Fatal(err)
	}
	_, port, err := net.SplitHostPort(actual.Host)
	if err != nil {
		t.Fatal(err)
	}
	dial := func(host string, cookie *http.Cookie) (*websocket.Conn, string) {
		origin := "http://" + net.JoinHostPort(host, port)
		headers := http.Header{
			"Origin":           []string{origin},
			"Cookie":           []string{cookie.String()},
			"X-Forwarded-For":  []string{"203.0.113.77"},
			"X-Forwarded-Host": []string{"203.0.113.88"},
			"Forwarded":        []string{"for=203.0.113.77;host=203.0.113.88"},
		}
		dialer := websocket.Dialer{NetDialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, actual.Host)
		}}
		c, _, err := dialer.Dial("ws"+strings.TrimPrefix(origin, "http")+"/ws?id=duplicate-device-id&name=iPhone&serverIP=203.0.113.88&hostIP=203.0.113.77&viewerIP=203.0.113.77", headers)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = c.Close() })
		_ = c.SetReadDeadline(time.Now().Add(3 * time.Second))
		var ready mediaTestEvent
		if err := c.ReadJSON(&ready); err != nil || ready.Type != "media-ready" {
			t.Fatalf("missing connection identity: %v type=%q", err, ready.Type)
		}
		return c, ready.ConnectionID
	}
	owner, _ := dial("localhost", hostCookie)
	viewer, viewerID := dial("192.168.1.25", peerCookie)
	if err := owner.WriteJSON(map[string]any{"type": "media-start", "target": viewerID, "mode": "audio", "audio": true}); err != nil {
		t.Fatal(err)
	}
	select {
	case r := <-requests:
		if r != (MediaRelayRequest{ServerIP: "192.168.1.25", HostIP: "127.0.0.1", ViewerIP: "127.0.0.1"}) {
			t.Fatalf("factory trusted a forwarded/query address or guessed a host: %+v", r)
		}
	case <-time.After(time.Second):
		t.Fatal("actual websocket endpoints never reached the factory")
	}
	readEvent := func(c *websocket.Conn, want string) mediaTestEvent {
		for i := 0; i < 20; i++ {
			var event mediaTestEvent
			if err := c.ReadJSON(&event); err != nil {
				t.Fatal(err)
			}
			if event.Type == "state" {
				continue
			}
			if event.Type != want {
				t.Fatalf("event=%q want=%q", event.Type, want)
			}
			return event
		}
		t.Fatal("media event missing")
		return mediaTestEvent{}
	}
	started := readEvent(owner, "media-started")
	readEvent(viewer, "media-invite")
	if err := viewer.Close(); err != nil {
		t.Fatal(err)
	}
	if stopped := readEvent(owner, "media-stopped"); stopped.ID != started.ID {
		t.Fatal("actual recipient disconnect revoked another session")
	}
	p.waitClosed(t)
}

func TestMediaRelayUDPAndTCPCandidateValidation(t *testing.T) {
	const chromium = "candidate:3146341842 1 udp 16785407 192.168.1.25 49153 typ relay raddr 0.0.0.0 rport 0 generation 0 ufrag abcd network-cost 999"
	const relay = "candidate:relay1 1 udp 1677734911 192.168.1.25 49153 typ relay "
	for _, tc := range []struct {
		name, candidate string
		valid           bool
	}{
		{"captured Chromium privacy", chromium, true},
		{"UDP with related endpoint", relay + "raddr 127.0.0.1 rport 49154 generation 0 ufrag abc network-id 1 network-cost 999", true},
		{"TCP with related endpoint", "candidate:relay2 1 tcp 1677734911 192.168.1.25 49153 typ relay raddr 127.0.0.1 rport 49154 tcptype passive generation 0", true},
		{"RFC IPv4 privacy", relay + "raddr 0.0.0.0 rport 9", true},
		{"Chromium IPv6 privacy", relay + "raddr :: rport 0", true},
		{"RFC IPv6 privacy", relay + "raddr :: rport 9", true},
		{"related unicast syntax", relay + "raddr 203.0.113.1 rport 49154", true},
		{"unspecified primary IPv4", strings.Replace(chromium, "192.168.1.25", "0.0.0.0", 1), false},
		{"unspecified primary IPv6", strings.Replace(chromium, "192.168.1.25", "::", 1), false},
		{"multicast primary IPv4", strings.Replace(chromium, "192.168.1.25", "224.0.0.1", 1), false},
		{"multicast primary IPv6", strings.Replace(chromium, "192.168.1.25", "ff02::1", 1), false},
		{"zero primary port", strings.Replace(chromium, "49153", "0", 1), false},
		{"privacy with another related port", relay + "raddr 0.0.0.0 rport 49154", false},
		{"privacy without related port", relay + "raddr 0.0.0.0", false},
		{"privacy with duplicate related port", relay + "raddr 0.0.0.0 rport 49154 rport 0", false},
		{"privacy with duplicate related address", relay + "raddr 0.0.0.0 raddr 127.0.0.1 rport 0", false},
		{"privacy host candidate", strings.Replace(chromium, "typ relay", "typ host", 1), false},
		{"privacy reflexive candidate", strings.Replace(chromium, "typ relay", "typ srflx", 1), false},
		{"multicast related endpoint", relay + "raddr 224.0.0.1 rport 49154", false},
		{"foreign hostname", relay + "raddr evil.example rport 49154", false},
		{"invalid TCP type", "candidate:relay1 1 tcp 123 192.168.1.25 49153 typ relay tcptype arbitrary", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, _ := json.Marshal(map[string]any{"candidate": tc.candidate, "sdpMid": "0", "sdpMLineIndex": 0, "usernameFragment": "abcd"})
			if validICE(data) != tc.valid {
				t.Fatalf("ICE candidate validity did not match %v", tc.valid)
			}
			data, _ = json.Marshal(map[string]any{"type": "offer", "sdp": testSDP + "a=" + tc.candidate + "\r\n"})
			if validSDP(data, "offer", "audio") != tc.valid {
				t.Fatalf("SDP candidate validity did not match %v", tc.valid)
			}
		})
	}
	f, _ := newRelayFixture(t)
	id := startTestMedia(t, f, "audio", true, false)
	sendMedia(t, f.s, f.host, signalPacket(id, "ice", map[string]any{"candidate": chromium, "sdpMid": "0", "sdpMLineIndex": 0, "usernameFragment": "abcd"}))
	if event := popMedia(t, f.viewer, "media-signal"); event.Kind != "ice" {
		t.Fatal("captured Chromium relay candidate was not routed to the selected receiver")
	}
	noMedia(t, f.host, f.otherHost, f.otherPeer)
}
