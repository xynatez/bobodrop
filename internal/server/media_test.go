package server

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

const testSDP = "v=0\r\no=- 1 1 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\nm=audio 9 UDP/TLS/RTP/SAVPF 111\r\na=sendrecv\r\n"
const testICE = "candidate:123 1 udp 2122260223 10.5.200.28 54321 typ host generation 0 network-cost 999"

type mediaTestEvent struct {
	Type         string          `json:"type"`
	ID           string          `json:"id"`
	Target       string          `json:"target"`
	ConnectionID string          `json:"connectionId"`
	Mode         string          `json:"mode"`
	Audio        bool            `json:"audio"`
	Control      bool            `json:"control"`
	Kind         string          `json:"kind"`
	Data         json.RawMessage `json:"data"`
	Message      string          `json:"message"`
	Reason       string          `json:"reason"`
	State        State           `json:"state"`
}

type mediaFixture struct {
	s                                  *Server
	host, otherHost, viewer, otherPeer *wsClient
}

func newMediaFixture(t *testing.T) mediaFixture {
	t.Helper()
	s := newTestServer(t)
	add := func(host bool, name string) *wsClient {
		id, err := randomID(24)
		if err != nil {
			t.Fatal(err)
		}
		c := &wsClient{connectionID: id, host: host, generation: s.sessionID, id: "duplicate-device-id", name: name, connectedAt: time.Now().UTC(), queue: make(chan []byte, 1), events: make(chan []byte, mediaQueueCapacity), done: make(chan struct{})}
		s.clients[c] = struct{}{}
		return c
	}
	return mediaFixture{s: s, host: add(true, "Laptop"), otherHost: add(true, "Other host"), viewer: add(false, "iPhone"), otherPeer: add(false, "Other peer")}
}

func sendMedia(t *testing.T, s *Server, c *wsClient, packet any) {
	t.Helper()
	data, err := json.Marshal(packet)
	if err != nil {
		t.Fatal(err)
	}
	s.handleSocketMessage(c, data)
}

func popMedia(t *testing.T, c *wsClient, want string) mediaTestEvent {
	t.Helper()
	select {
	case data := <-c.events:
		var event mediaTestEvent
		if err := json.Unmarshal(data, &event); err != nil {
			t.Fatal(err)
		}
		if event.Type != want {
			t.Fatalf("event %q, want %q: %s", event.Type, want, data)
		}
		return event
	default:
		t.Fatalf("missing %s event", want)
	}
	return mediaTestEvent{}
}

func noMedia(t *testing.T, clients ...*wsClient) {
	t.Helper()
	for _, c := range clients {
		select {
		case data := <-c.events:
			t.Fatalf("unexpected routed event: %s", data)
		default:
		}
	}
}

func startTestMedia(t *testing.T, f mediaFixture, mode string, audio, control bool) string {
	t.Helper()
	sendMedia(t, f.s, f.host, map[string]any{"type": "media-start", "target": f.viewer.connectionID, "mode": mode, "audio": audio, "control": control})
	started := popMedia(t, f.host, "media-started")
	invite := popMedia(t, f.viewer, "media-invite")
	if started.ID == "" || started.ID != invite.ID || started.Target != f.viewer.connectionID || started.Mode != mode || started.Audio != audio || started.Control != control {
		t.Fatal("start acknowledgement/invite disagree")
	}
	noMedia(t, f.otherHost, f.otherPeer)
	return started.ID
}

func signalPacket(id, kind string, data any) map[string]any {
	return map[string]any{"type": "media-signal", "id": id, "kind": kind, "data": data}
}
func inputPacket(id string, input RemoteInput) map[string]any {
	return map[string]any{"type": "media-input", "id": id, "input": input}
}
func description(kind string) map[string]any { return map[string]any{"type": kind, "sdp": testSDP} }
func iceCandidate() map[string]any {
	return map[string]any{"candidate": testICE, "sdpMid": "0", "sdpMLineIndex": 0, "usernameFragment": "abcd"}
}

func TestMediaRoutingUsesSocketIdentityAndNeverBroadcastsSignals(t *testing.T) {
	f := newMediaFixture(t)
	state := f.s.state(true)
	if len(state.Devices) != 2 || state.Devices[0].ID != state.Devices[1].ID || state.Devices[0].ConnectionID == state.Devices[1].ConnectionID || state.Media != nil {
		t.Fatal("supplied duplicate device IDs collapsed distinct sockets")
	}
	sendMedia(t, f.s, f.host, map[string]any{"type": "media-start", "target": f.viewer.id, "mode": "audio", "audio": true})
	popMedia(t, f.host, "media-error")
	if f.s.media != nil {
		t.Fatal("browser-supplied ID granted routing authority")
	}
	id := startTestMedia(t, f, "audio", true, false)
	sendMedia(t, f.s, f.host, signalPacket(id, "offer", description("offer")))
	forwarded := popMedia(t, f.viewer, "media-signal")
	if forwarded.ID != id || forwarded.Kind != "offer" || !bytes.Contains(forwarded.Data, []byte("v=0")) {
		t.Fatal("offer was not routed intact to selected socket")
	}
	noMedia(t, f.host, f.otherHost, f.otherPeer)
	sendMedia(t, f.s, f.viewer, signalPacket(id, "answer", description("answer")))
	if event := popMedia(t, f.host, "media-signal"); event.Kind != "answer" {
		t.Fatal("answer did not return to the owner")
	}
	noMedia(t, f.viewer, f.otherHost, f.otherPeer)
	sendMedia(t, f.s, f.viewer, signalPacket(id, "ice", iceCandidate()))
	if event := popMedia(t, f.host, "media-signal"); event.Kind != "ice" {
		t.Fatal("ICE did not return to owner")
	}
	noMedia(t, f.viewer, f.otherHost, f.otherPeer)
	snapshot := f.s.state(true)
	encoded, _ := json.Marshal(snapshot)
	if snapshot.Media == nil || snapshot.Media.ViewerID != f.viewer.connectionID || snapshot.Media.ID != id || bytes.Contains(encoded, []byte("sdp")) || bytes.Contains(encoded, []byte("candidate:")) {
		t.Fatal("state leaked signaling or lost selected socket")
	}
	connectionID := f.viewer.connectionID
	sendMedia(t, f.s, f.viewer, map[string]any{"type": "hello", "id": "changed-device-id", "name": "New iPhone"})
	if f.viewer.connectionID != connectionID || f.s.state(true).Media.ViewerName != "New iPhone" {
		t.Fatal("hello changed socket identity or failed to update label")
	}
	for _, c := range []*wsClient{f.otherHost, f.otherPeer} {
		sendMedia(t, f.s, c, signalPacket(id, "ice", iceCandidate()))
		popMedia(t, c, "media-error")
		sendMedia(t, f.s, c, map[string]any{"type": "media-stop", "id": id})
		popMedia(t, c, "media-error")
	}
	for _, field := range []string{"sender", "target", "connectionId"} {
		packet := signalPacket(id, "ice", iceCandidate())
		packet[field] = f.otherPeer.connectionID
		sendMedia(t, f.s, f.viewer, packet)
		popMedia(t, f.viewer, "media-error")
	}
	noMedia(t, f.host, f.otherHost, f.otherPeer)
	if f.s.media == nil {
		t.Fatal("unrelated sockets stopped active session")
	}
	sendMedia(t, f.s, f.otherHost, map[string]any{"type": "media-start", "target": f.otherPeer.connectionID, "mode": "audio", "audio": true})
	popMedia(t, f.otherHost, "media-error")
	sendMedia(t, f.s, f.viewer, map[string]any{"type": "media-stop", "id": id})
	popMedia(t, f.host, "media-stopped")
	popMedia(t, f.viewer, "media-stopped")
	if f.s.state(true).Media != nil {
		t.Fatal("stop did not clear active session")
	}
	sendMedia(t, f.s, f.host, signalPacket(id, "ice", iceCandidate()))
	popMedia(t, f.host, "media-error")
}

func TestMediaStartPermissionAndModeValidation(t *testing.T) {
	for _, tc := range []struct {
		name, mode                       string
		audio, control, peer, hostTarget bool
	}{
		{"peer start", "audio", true, false, true, false},
		{"host target", "audio", true, false, false, true},
		{"invalid mode", "camera", true, false, false, false},
		{"audio disabled", "audio", false, false, false, false},
		{"audio control", "audio", true, true, false, false},
		{"native callback absent", "screen", false, true, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newMediaFixture(t)
			c, target := f.host, f.viewer.connectionID
			if tc.peer {
				c = f.viewer
			}
			if tc.hostTarget {
				target = f.otherHost.connectionID
			}
			sendMedia(t, f.s, c, map[string]any{"type": "media-start", "target": target, "mode": tc.mode, "audio": tc.audio, "control": tc.control})
			popMedia(t, c, "media-error")
			if f.s.media != nil {
				t.Fatal("invalid start created session")
			}
		})
	}
	for _, mode := range []string{"audio", "screen"} {
		t.Run("control default "+mode, func(t *testing.T) {
			f := newMediaFixture(t)
			sendMedia(t, f.s, f.host, map[string]any{"type": "media-start", "target": f.viewer.connectionID, "mode": mode, "audio": mode == "audio"})
			started := popMedia(t, f.host, "media-started")
			popMedia(t, f.viewer, "media-invite")
			if started.Control || f.s.media.control {
				t.Fatal("control enabled by default")
			}
		})
	}
}

func TestSDPAndICEValidation(t *testing.T) {
	encode := func(value any) []byte { data, _ := json.Marshal(value); return data }
	if !validSDP(encode(description("offer")), "offer", "audio") {
		t.Fatal("valid SDP rejected")
	}
	for _, sdp := range []string{"not SDP", strings.Replace(testSDP, "v=0", "v=1", 1), strings.Replace(testSDP, "m=audio", "x=audio", 1), testSDP + "a=bad\x00\r\n", testSDP + "m=video 9 UDP/TLS/RTP/SAVPF 96\r\n", strings.Repeat("a", maxSDPBytes+1)} {
		if validSDP(encode(map[string]any{"type": "offer", "sdp": sdp}), "offer", "audio") {
			t.Fatalf("invalid SDP accepted: %.40q", sdp)
		}
	}
	if validSDP(encode(description("answer")), "offer", "audio") || validSDP(encode(map[string]any{"type": "offer", "sdp": testSDP, "target": "spoof"}), "offer", "audio") {
		t.Fatal("SDP role/extra fields accepted")
	}
	for _, candidate := range []map[string]any{iceCandidate(), {"candidate": "", "sdpMid": nil, "sdpMLineIndex": nil}, {"candidate": "candidate:123 1 tcp 123456 host-123.local 9 typ host tcptype active generation 0"}, {"candidate": "candidate:123 1 udp 123456 fe80::1234 54321 typ host"}} {
		if !validICE(encode(candidate)) {
			t.Fatalf("valid ICE rejected: %v", candidate)
		}
	}
	for _, candidate := range []map[string]any{{}, {"candidate": "candidate:123 1 udp 123456 evil.example 54321 typ host"}, {"candidate": "candidate:123 1 udp 123456 10.5.200.28 99999 typ host"}, {"candidate": testICE, "sdpMLineIndex": -1}, {"candidate": testICE, "sdpMLineIndex": 256}, {"candidate": testICE, "sdpMid": "x\x00"}, {"candidate": testICE + "\r\n"}, {"candidate": testICE, "target": "spoof"}, {"candidate": testICE + " command launch"}, {"candidate": strings.Repeat("a", 2049)}} {
		if validICE(encode(candidate)) {
			t.Fatalf("invalid ICE accepted: %v", candidate)
		}
	}
	f := newMediaFixture(t)
	id := startTestMedia(t, f, "audio", true, false)
	for _, tc := range []struct {
		c    *wsClient
		kind string
		data any
	}{
		{f.viewer, "offer", description("offer")}, {f.host, "answer", description("answer")},
		{f.viewer, "answer", description("answer")}, {f.host, "offer", map[string]any{"type": "answer", "sdp": testSDP}},
		{f.host, "ice", map[string]any{"candidate": "invalid"}}, {f.host, "unknown", iceCandidate()},
	} {
		sendMedia(t, f.s, tc.c, signalPacket(id, tc.kind, tc.data))
		popMedia(t, tc.c, "media-error")
	}
	noMedia(t, f.host, f.viewer, f.otherHost, f.otherPeer)
	sendMedia(t, f.s, f.host, signalPacket(id, "offer", description("offer")))
	popMedia(t, f.viewer, "media-signal")
	sendMedia(t, f.s, f.host, signalPacket(id, "offer", description("offer")))
	popMedia(t, f.host, "media-error")
}

func TestReliableMediaQueueSurvivesStateAndStopsOnOverflow(t *testing.T) {
	f := newMediaFixture(t)
	id := startTestMedia(t, f, "audio", true, false)
	sendMedia(t, f.s, f.host, signalPacket(id, "offer", description("offer")))
	for i := 0; i < 100; i++ {
		f.s.broadcast()
	}
	if event := popMedia(t, f.viewer, "media-signal"); event.Kind != "offer" {
		t.Fatal("state coalescing discarded reliable offer")
	}
	if len(f.viewer.queue) != 1 {
		t.Fatal("state queue did not coalesce")
	}
	f.s.mu.Lock()
	for i := 0; i < mediaQueueCapacity; i++ {
		if !f.s.enqueueEventLocked(f.viewer, map[string]any{"type": "media-signal", "id": id, "kind": "ice", "data": map[string]any{"candidate": "", "sdpMLineIndex": i}}) {
			t.Fatal("queue overflowed prematurely")
		}
	}
	f.s.mu.Unlock()
	sendMedia(t, f.s, f.host, signalPacket(id, "ice", iceCandidate()))
	if f.s.media != nil {
		t.Fatal("overflow kept active media session")
	}
	select {
	case <-f.viewer.done:
	default:
		t.Fatal("overflow did not disconnect slow receiver")
	}
	if _, active := f.s.clients[f.viewer]; active {
		t.Fatal("overflow kept receiver authorized")
	}
	popMedia(t, f.host, "media-stopped")
	for i := 0; i < mediaQueueCapacity; i++ {
		event := popMedia(t, f.viewer, "media-signal")
		var candidate struct {
			Index int `json:"sdpMLineIndex"`
		}
		if json.Unmarshal(event.Data, &candidate) != nil || candidate.Index != i {
			t.Fatal("reliable event FIFO order changed")
		}
	}
}

func TestRemoteInputValidation(t *testing.T) {
	valid := []RemoteInput{{Kind: "move", DX: -500, DY: 500}, {Kind: "click", Button: "left"}, {Kind: "click", Button: "right"}, {Kind: "click", Button: "middle"}, {Kind: "scroll", DY: -1200}, {Kind: "scroll", DY: 1200}, {Kind: "text", Text: strings.Repeat("🦷", 256)}}
	for _, key := range []string{"enter", "tab", "backspace", "escape", "arrowup", "arrowdown", "arrowleft", "arrowright", "delete", "home", "end", "pageup", "pagedown", "ctrl+a", "ctrl+c", "ctrl+v", "ctrl+z"} {
		valid = append(valid, RemoteInput{Kind: "key", Key: key})
	}
	for _, input := range valid {
		if err := validateRemoteInput(input); err != nil {
			t.Fatalf("valid input rejected: %+v", input)
		}
	}
	invalid := []RemoteInput{{Kind: "move", DX: 501}, {Kind: "move", DY: -501}, {Kind: "move", Key: "enter"}, {Kind: "click", Button: "left", DX: 1}, {Kind: "click", Button: "double"}, {Kind: "scroll", DY: 1201}, {Kind: "scroll", DX: 1}, {Kind: "key", Key: "ctrl+alt+delete"}, {Kind: "key", Key: "alt+f4"}, {Kind: "key", Key: "enter", Text: "x"}, {Kind: "text", Text: ""}, {Kind: "text", Text: strings.Repeat("🦷", 257)}, {Kind: "text", Text: "line\nbreak"}, {Kind: "text", Text: "tab\t"}, {Kind: "text", Text: "zero\x00"}, {Kind: "text", Text: string([]byte{0xff})}, {Kind: "text", Text: "x", Button: "left"}, {Kind: "command", Text: "https://example.com"}}
	for _, input := range invalid {
		if validateRemoteInput(input) == nil {
			t.Fatalf("invalid input accepted: %+v", input)
		}
	}
}

func TestMediaAcknowledgementsNeverExposeOlderQueuedState(t *testing.T) {
	f := newMediaFixture(t)
	type observed struct {
		event mediaTestEvent
		state State
		err   string
	}
	observe := func(c *wsClient) <-chan observed {
		result := make(chan observed, 1)
		go func() {
			var value observed
			if err := json.Unmarshal(<-c.events, &value.event); err != nil {
				value.err = err.Error()
			}
			select {
			case data := <-c.queue:
				var packet struct {
					State State `json:"state"`
				}
				if err := json.Unmarshal(data, &packet); err != nil {
					value.err = err.Error()
				}
				value.state = packet.State
			default:
				value.err = "snapshot was not enqueued before acknowledgement"
			}
			result <- value
		}()
		return result
	}
	wait := func(result <-chan observed) observed {
		select {
		case value := <-result:
			if value.err != "" {
				t.Fatal(value.err)
			}
			return value
		case <-time.After(time.Second):
			t.Fatal("acknowledgement missing")
			return observed{}
		}
	}
	for i := 0; i < 40; i++ {
		f.host.startRate = rateBucket{}
		f.s.broadcast() // Deliberately leave an older idle snapshot queued.
		result := observe(f.host)
		sendMedia(t, f.s, f.host, map[string]any{"type": "media-start", "target": f.viewer.connectionID, "mode": "audio", "audio": true})
		started := wait(result)
		if started.event.Type != "media-started" || started.state.Media == nil || started.state.Media.ID != started.event.ID {
			t.Fatal("old idle snapshot was available after start acknowledgement")
		}
		popMedia(t, f.viewer, "media-invite")
		f.s.broadcast() // Deliberately leave an older active snapshot queued.
		result = observe(f.host)
		sendMedia(t, f.s, f.host, map[string]any{"type": "media-stop", "id": started.event.ID})
		stopped := wait(result)
		if stopped.event.Type != "media-stopped" || stopped.state.Media != nil {
			t.Fatal("old active snapshot was available after stop acknowledgement")
		}
		popMedia(t, f.viewer, "media-stopped")
	}
}

func TestRemoteInputRejectsOwnerClosedBeforeCleanup(t *testing.T) {
	f := newMediaFixture(t)
	calls := 0
	f.s.config.ControlInput = func(RemoteInput) error { calls++; return nil }
	id := startTestMedia(t, f, "screen", false, true)
	f.host.close() // Simulate writer failure before the reader removes its map entry.
	sendMedia(t, f.s, f.viewer, inputPacket(id, RemoteInput{Kind: "click", Button: "left"}))
	if calls != 0 || f.s.state(true).Media != nil {
		t.Fatal("closed owner permitted remote input before map cleanup")
	}
	popMedia(t, f.viewer, "media-stopped")
}

func TestRemoteInputRequiresOptInSelectedLiveScreenRecipient(t *testing.T) {
	for _, tc := range []struct {
		name, mode string
		control    bool
	}{{"audio mode", "audio", false}, {"screen disabled", "screen", false}, {"screen enabled", "screen", true}} {
		t.Run(tc.name, func(t *testing.T) {
			f := newMediaFixture(t)
			calls := 0
			f.s.config.ControlInput = func(RemoteInput) error { calls++; return nil }
			id := startTestMedia(t, f, tc.mode, tc.mode == "audio", tc.control)
			for _, c := range []*wsClient{f.host, f.otherHost, f.otherPeer} {
				sendMedia(t, f.s, c, inputPacket(id, RemoteInput{Kind: "click", Button: "left"}))
				popMedia(t, c, "media-error")
			}
			sendMedia(t, f.s, f.viewer, inputPacket("stale-id", RemoteInput{Kind: "click", Button: "left"}))
			popMedia(t, f.viewer, "media-error")
			sendMedia(t, f.s, f.viewer, inputPacket(id, RemoteInput{Kind: "move", DX: 501}))
			popMedia(t, f.viewer, "media-error")
			sendMedia(t, f.s, f.viewer, inputPacket(id, RemoteInput{Kind: "text", Text: "Halo 🦷"}))
			if tc.control {
				if calls != 1 {
					t.Fatalf("enabled input calls=%d", calls)
				}
				noMedia(t, f.viewer)
			} else {
				popMedia(t, f.viewer, "media-error")
				if calls != 0 {
					t.Fatal("input dispatched without screen control opt-in")
				}
			}
			sendMedia(t, f.s, f.host, map[string]any{"type": "media-stop", "id": id})
			popMedia(t, f.host, "media-stopped")
			popMedia(t, f.viewer, "media-stopped")
			before := calls
			sendMedia(t, f.s, f.viewer, inputPacket(id, RemoteInput{Kind: "click", Button: "left"}))
			popMedia(t, f.viewer, "media-error")
			if calls != before {
				t.Fatal("stopped session dispatched input")
			}
		})
	}
	f := newMediaFixture(t)
	f.s.config.ControlInput = func(RemoteInput) error { return errors.New("desktop tidak tersedia") }
	id := startTestMedia(t, f, "screen", false, true)
	sendMedia(t, f.s, f.viewer, inputPacket(id, RemoteInput{Kind: "key", Key: "enter"}))
	if event := popMedia(t, f.viewer, "media-error"); !strings.Contains(event.Message, "desktop tidak tersedia") {
		t.Fatal("native failure not reported to receiver")
	}
}

func TestMediaDisconnectRotationAndCloseRevokeInput(t *testing.T) {
	for _, action := range []string{"owner disconnect", "viewer disconnect", "rotation", "close"} {
		t.Run(action, func(t *testing.T) {
			f := newMediaFixture(t)
			calls := 0
			f.s.config.ControlInput = func(RemoteInput) error { calls++; return nil }
			id := startTestMedia(t, f, "screen", false, true)
			switch action {
			case "owner disconnect", "viewer disconnect":
				c := f.host
				if action == "viewer disconnect" {
					c = f.viewer
				}
				f.s.mu.Lock()
				f.s.disconnectLocked(c, "Perangkat terputus.")
				f.s.mu.Unlock()
			case "rotation":
				f.s.rotate(httptest.NewRecorder())
			case "close":
				_ = f.s.Close()
			}
			if f.s.state(true).Media != nil {
				t.Fatal("lifecycle event kept active session")
			}
			sendMedia(t, f.s, f.viewer, inputPacket(id, RemoteInput{Kind: "click", Button: "left"}))
			sendMedia(t, f.s, f.viewer, signalPacket(id, "ice", iceCandidate()))
			if calls != 0 {
				t.Fatal("stale socket dispatched input after revocation")
			}
		})
	}
}

func TestRemoteInputDispatchIsAtomicWithStop(t *testing.T) {
	f := newMediaFixture(t)
	entered, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	f.s.config.ControlInput = func(RemoteInput) error { close(entered); <-release; calls.Add(1); return nil }
	id := startTestMedia(t, f, "screen", false, true)
	inputDone := make(chan struct{})
	go func() {
		sendMedia(t, f.s, f.viewer, inputPacket(id, RemoteInput{Kind: "click", Button: "left"}))
		close(inputDone)
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("input callback did not start")
	}
	stopDone := make(chan struct{})
	go func() { sendMedia(t, f.s, f.host, map[string]any{"type": "media-stop", "id": id}); close(stopDone) }()
	select {
	case <-stopDone:
		t.Fatal("stop returned while an authorized dispatch was running")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	select {
	case <-inputDone:
	case <-time.After(time.Second):
		t.Fatal("input did not finish")
	}
	select {
	case <-stopDone:
	case <-time.After(time.Second):
		t.Fatal("stop did not finish")
	}
	if calls.Load() != 1 || f.s.state(true).Media != nil {
		t.Fatal("dispatch/stop ordering incorrect")
	}
	sendMedia(t, f.s, f.viewer, inputPacket(id, RemoteInput{Kind: "key", Key: "enter"}))
	if calls.Load() != 1 {
		t.Fatal("input dispatched after stop completed")
	}
}

func TestMediaRateLimits(t *testing.T) {
	now := time.Now()
	var bucket rateBucket
	for i := 0; i < 3; i++ {
		if !bucket.allow(now, 2, 3) {
			t.Fatal("burst rejected prematurely")
		}
	}
	if bucket.allow(now, 2, 3) || !bucket.allow(now.Add(time.Second), 2, 3) {
		t.Fatal("rate bucket did not enforce/refill burst")
	}
	f := newMediaFixture(t)
	calls := 0
	f.s.config.ControlInput = func(RemoteInput) error { calls++; return nil }
	id := startTestMedia(t, f, "screen", false, true)
	f.viewer.inputRate = rateBucket{updated: time.Now().Add(time.Second)}
	sendMedia(t, f.s, f.viewer, inputPacket(id, RemoteInput{Kind: "move", DX: 1}))
	popMedia(t, f.viewer, "media-error")
	if calls != 0 {
		t.Fatal("rate-limited input dispatched")
	}
	f.host.signalRate = rateBucket{updated: time.Now().Add(time.Second)}
	sendMedia(t, f.s, f.host, signalPacket(id, "ice", iceCandidate()))
	popMedia(t, f.host, "media-error")
	noMedia(t, f.viewer)
	f.viewer.messageRate = rateBucket{updated: time.Now().Add(time.Second)}
	sendMedia(t, f.s, f.viewer, map[string]any{"type": "hello", "name": "flood"})
	if f.s.media != nil {
		t.Fatal("flooding participant kept active session")
	}
	select {
	case <-f.viewer.done:
	default:
		t.Fatal("flooding socket not disconnected")
	}
}

func TestWebSocketCryptographicIdentityAndMediaRouting(t *testing.T) {
	s := newTestServer(t)
	host, peer := hostSession(t, s), peerSession(t, s)
	web := httptest.NewServer(s.Handler())
	defer web.Close()
	dial := func(cookie *http.Cookie) (*websocket.Conn, string) {
		headers := http.Header{"Origin": []string{web.URL}, "Cookie": []string{cookie.String()}}
		c, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(web.URL, "http")+"/ws?id=duplicate-device-id&name=iPhone", headers)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { c.Close() })
		_ = c.SetReadDeadline(time.Now().Add(3 * time.Second))
		var ready mediaTestEvent
		if err := c.ReadJSON(&ready); err != nil || ready.Type != "media-ready" {
			t.Fatalf("missing media-ready: %v %+v", err, ready)
		}
		decoded, err := base64.RawURLEncoding.DecodeString(ready.ConnectionID)
		if err != nil || len(decoded) != 24 {
			t.Fatal("connection ID is not a cryptographic 24-byte identifier")
		}
		return c, ready.ConnectionID
	}
	owner, ownerID := dial(host)
	viewer, viewerID := dial(peer)
	other, otherID := dial(peer)
	if ownerID == viewerID || viewerID == otherID || ownerID == otherID {
		t.Fatal("duplicate supplied device IDs reused connection identity")
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
				t.Fatalf("event=%s want=%s", event.Type, want)
			}
			return event
		}
		t.Fatal("media event missing")
		return mediaTestEvent{}
	}
	if err := owner.WriteJSON(map[string]any{"type": "media-start", "target": viewerID, "mode": "audio", "audio": true, "control": false}); err != nil {
		t.Fatal(err)
	}
	started := readEvent(owner, "media-started")
	invite := readEvent(viewer, "media-invite")
	if started.ID != invite.ID || started.Target != viewerID {
		t.Fatal("real socket start routing mismatch")
	}
	if err := owner.WriteJSON(signalPacket(started.ID, "offer", description("offer"))); err != nil {
		t.Fatal(err)
	}
	if event := readEvent(viewer, "media-signal"); event.Kind != "offer" {
		t.Fatal("real socket offer not forwarded")
	}
	if err := other.WriteJSON(signalPacket(started.ID, "answer", description("answer"))); err != nil {
		t.Fatal(err)
	}
	readEvent(other, "media-error")
	if err := viewer.WriteJSON(signalPacket(started.ID, "answer", description("answer"))); err != nil {
		t.Fatal(err)
	}
	if event := readEvent(owner, "media-signal"); event.Kind != "answer" {
		t.Fatal("real socket answer not forwarded")
	}
	if err := viewer.Close(); err != nil {
		t.Fatal(err)
	}
	readEvent(owner, "media-stopped")
	if s.state(true).Media != nil {
		t.Fatal("real recipient disconnect kept active session")
	}
}
