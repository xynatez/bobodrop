package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

type systemAudioProbe struct {
	capture   *SystemAudioCapture
	writer    *io.PipeWriter
	pcmClosed atomic.Int32
	closed    atomic.Int32
	done      chan struct{}
	finished  chan struct{}
	doneOnce  sync.Once
}

type systemAudioTestPCM struct {
	*io.PipeReader
	s     *Server
	count *atomic.Int32
}

func (pcm *systemAudioTestPCM) Close() error {
	_ = pcm.s.state(true)
	pcm.count.Add(1)
	return pcm.PipeReader.Close()
}

func newSystemAudioProbe(s *Server) *systemAudioProbe {
	reader, writer := io.Pipe()
	p := &systemAudioProbe{writer: writer, done: make(chan struct{}), finished: make(chan struct{})}
	p.capture = &SystemAudioCapture{PCM: &systemAudioTestPCM{reader, s, &p.pcmClosed}, SampleRate: 48000, Channels: 2, Done: p.done, Close: func() error {
		_ = s.state(true)
		_ = writer.Close()
		p.doneOnce.Do(func() { close(p.done) })
		if p.closed.Add(1) == 1 {
			close(p.finished)
		}
		return nil
	}}
	return p
}

func (p *systemAudioProbe) wait(t *testing.T) {
	t.Helper()
	select {
	case <-p.finished:
	case <-time.After(time.Second):
		t.Fatal("capture cleanup blocked under session mutex")
	}
	if p.closed.Load() != 1 || p.pcmClosed.Load() != 1 {
		t.Fatalf("capture cleanup counts: close=%d PCM close=%d", p.closed.Load(), p.pcmClosed.Load())
	}
}

func systemAudioPath(f mediaFixture) string {
	return "/api/system-audio?connectionId=" + url.QueryEscape(f.host.connectionID) + "&target=" + url.QueryEscape(f.viewer.connectionID)
}

func dialSystemAudio(t *testing.T, f mediaFixture) (*websocket.Conn, *httptest.Server) {
	t.Helper()
	web := httptest.NewServer(f.s.Handler())
	t.Cleanup(web.Close)
	connection, response, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(web.URL, "http")+systemAudioPath(f), http.Header{"Origin": {web.URL}, "Cookie": {hostSession(t, f.s).String()}})
	if err != nil {
		if response != nil {
			_ = response.Body.Close()
		}
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	return connection, web
}

func readSystemAudioReady(t *testing.T, connection *websocket.Conn) {
	t.Helper()
	_ = connection.SetReadDeadline(time.Now().Add(time.Second))
	kind, data, err := connection.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	var metadata map[string]any
	if kind != websocket.TextMessage || json.Unmarshal(data, &metadata) != nil || len(metadata) != 4 || metadata["type"] != "ready" || metadata["sampleRate"] != float64(48000) || metadata["channels"] != float64(2) || metadata["sampleSize"] != float64(16) {
		t.Fatalf("unexpected ready metadata: %s", data)
	}
}

func reserveSystemAudioTestSource(f mediaFixture) *systemAudioSource {
	f.s.mu.Lock()
	defer f.s.mu.Unlock()
	ctx, cancel := context.WithCancel(f.s.ctx)
	source := &systemAudioSource{owner: f.host, viewer: f.viewer, generation: f.s.sessionID, ctx: ctx, cancel: cancel, attached: make(chan struct{})}
	f.s.systemAudio = source
	return source
}

func nativeAudioPacket(f mediaFixture) map[string]any {
	return map[string]any{"type": "media-start", "mode": "audio", "audio": true, "nativeAudio": true, "target": f.viewer.connectionID}
}

func TestSystemAudioCapabilityAndRequestSecurity(t *testing.T) {
	f := newMediaFixture(t)
	var called atomic.Int32
	f.s.config.SystemAudio = func(context.Context) (*SystemAudioCapture, error) {
		called.Add(1)
		return nil, errors.New("private detail")
	}
	host, peer := hostSession(t, f.s), peerSession(t, f.s)
	if !f.s.state(true).NativeAudio || f.s.state(false).NativeAudio {
		t.Fatal("native capability missing for host or exposed to peer")
	}
	for _, tc := range []struct {
		name, path string
		cookie     *http.Cookie
		local      bool
		headers    map[string]string
		status     int
	}{
		{"no cookie", systemAudioPath(f), nil, true, nil, 401},
		{"peer cookie", systemAudioPath(f), peer, true, nil, 403},
		{"remote host cookie", systemAudioPath(f), host, false, map[string]string{"Origin": "http://192.168.1.25:8787", "X-Forwarded-For": "127.0.0.1"}, 401},
		{"missing origin", systemAudioPath(f), host, true, nil, 403},
		{"cross origin", systemAudioPath(f), host, true, map[string]string{"Origin": "https://evil.example"}, 403},
		{"null origin", systemAudioPath(f), host, true, map[string]string{"Origin": "null"}, 403},
		{"cross site", systemAudioPath(f), host, true, map[string]string{"Origin": "http://localhost:8787", "Sec-Fetch-Site": "cross-site"}, 403},
		{"peer owner", "/api/system-audio?connectionId=" + f.viewer.connectionID + "&target=" + f.otherPeer.connectionID, host, true, map[string]string{"Origin": "http://localhost:8787"}, 403},
		{"host viewer", "/api/system-audio?connectionId=" + f.host.connectionID + "&target=" + f.otherHost.connectionID, host, true, map[string]string{"Origin": "http://localhost:8787"}, 403},
		{"supplied device ID", "/api/system-audio?connectionId=" + f.host.id + "&target=" + f.viewer.id, host, true, map[string]string{"Origin": "http://localhost:8787"}, 403},
		{"unknown target", "/api/system-audio?connectionId=" + f.host.connectionID + "&target=unknown", host, true, map[string]string{"Origin": "http://localhost:8787"}, 403},
		{"duplicate target", systemAudioPath(f) + "&target=" + f.otherPeer.connectionID, host, true, map[string]string{"Origin": "http://localhost:8787"}, 400},
		{"extra query", systemAudioPath(f) + "&token=secret", host, true, map[string]string{"Origin": "http://localhost:8787"}, 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response := request(t, f.s, http.MethodGet, tc.path, nil, tc.cookie, tc.local, tc.headers)
			if response.Code != tc.status {
				t.Fatalf("status=%d want=%d: %s", response.Code, tc.status, response.Body.String())
			}
		})
	}
	f.viewer.generation = "expired"
	if response := request(t, f.s, http.MethodGet, systemAudioPath(f), nil, host, true, map[string]string{"Origin": "http://localhost:8787"}); response.Code != 403 {
		t.Fatal("stale participant authorized capture")
	}
	if called.Load() != 0 || f.s.systemAudio != nil {
		t.Fatal("rejected request opened native capture or reserved source")
	}
	f.s.config.SystemAudio = nil
	if f.s.state(true).NativeAudio {
		t.Fatal("nil factory advertised native capability")
	}
	if response := request(t, f.s, http.MethodGet, systemAudioPath(f), nil, host, true, map[string]string{"Origin": "http://localhost:8787"}); response.Code != 503 {
		t.Fatal("unconfigured capture accepted")
	}
}

func TestSystemAudioReadySingleSourceAndPCMFrames(t *testing.T) {
	f := newMediaFixture(t)
	p := newSystemAudioProbe(f.s)
	f.s.config.SystemAudio = func(context.Context) (*SystemAudioCapture, error) { _ = f.s.state(true); return p.capture, nil }
	connection, web := dialSystemAudio(t, f)
	readSystemAudioReady(t, connection)
	if f.s.state(true).Media != nil {
		t.Fatal("unattached source published active media")
	}
	_, response, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(web.URL, "http")+systemAudioPath(f), http.Header{"Origin": {web.URL}, "Cookie": {hostSession(t, f.s).String()}})
	if err == nil || response == nil || response.StatusCode != http.StatusConflict {
		t.Fatal("second capture source was not rejected")
	}
	_ = response.Body.Close()
	payload := make([]byte, 3*systemAudioChunkBytes+12)
	for i := range payload {
		payload[i] = byte(i*19 + 7)
	}
	writeDone := make(chan error, 1)
	go func() {
		for offset, size := 0, 1; offset < len(payload); size = size%31 + 1 {
			end := min(offset+size, len(payload))
			if _, err := p.writer.Write(payload[offset:end]); err != nil {
				writeDone <- err
				return
			}
			offset = end
		}
		writeDone <- nil
	}()
	var received []byte
	for len(received) < len(payload) {
		_ = connection.SetReadDeadline(time.Now().Add(time.Second))
		kind, data, err := connection.ReadMessage()
		if err != nil || kind != websocket.BinaryMessage || len(data) == 0 || len(data)%4 != 0 || len(data) > systemAudioChunkBytes {
			t.Fatalf("invalid PCM frame: kind=%d length=%d err=%v", kind, len(data), err)
		}
		received = append(received, data...)
	}
	if !bytes.Equal(received, payload) || <-writeDone != nil {
		t.Fatal("partial reads lost or altered PCM samples")
	}
	_ = connection.Close()
	p.wait(t)
}

func TestSystemAudioNativeStartRequiresReadyExactParticipants(t *testing.T) {
	for _, kind := range []string{"absent", "starting", "other owner", "other target", "no audio", "native omitted"} {
		t.Run(kind, func(t *testing.T) {
			f := newMediaFixture(t)
			p := newSystemAudioProbe(f.s)
			var muteCalls atomic.Int32
			f.s.config.SystemAudio = func(context.Context) (*SystemAudioCapture, error) { return p.capture, nil }
			f.s.config.AudioOutput = func() (*AudioOutputLease, error) { muteCalls.Add(1); return nil, nil }
			if kind != "absent" {
				source := reserveSystemAudioTestSource(f)
				f.s.mu.Lock()
				source.capture, source.ready = p.capture, kind != "starting"
				f.s.mu.Unlock()
			}
			client, packet := f.host, nativeAudioPacket(f)
			switch kind {
			case "other owner":
				client = f.otherHost
			case "other target":
				packet["target"] = f.otherPeer.connectionID
			case "no audio":
				packet["mode"], packet["audio"] = "screen", false
			case "native omitted":
				delete(packet, "nativeAudio")
				packet["phoneOnly"] = true
			}
			sendMedia(t, f.s, client, packet)
			popMedia(t, client, "media-error")
			if f.s.state(true).Media != nil || muteCalls.Load() != 0 {
				t.Fatal("invalid native start created media or muted endpoint")
			}
		})
	}
}

func TestSystemAudioNativeMediaLifecycleReleasesCaptureAndMute(t *testing.T) {
	for _, stop := range []string{"media stop", "socket close", "owner disconnect", "viewer disconnect", "rotation", "server close", "helper exit", "PCM error", "event overflow", "output failure"} {
		t.Run(stop, func(t *testing.T) {
			f := newMediaFixture(t)
			p := newSystemAudioProbe(f.s)
			mute := newAudioOutputProbe(f.s)
			f.s.config.SystemAudio = func(context.Context) (*SystemAudioCapture, error) { return p.capture, nil }
			f.s.config.AudioOutput = func() (*AudioOutputLease, error) { return mute.lease, nil }
			connection, _ := dialSystemAudio(t, f)
			readSystemAudioReady(t, connection)
			packet := nativeAudioPacket(f)
			packet["phoneOnly"] = true
			sendMedia(t, f.s, f.host, packet)
			id := assertPhoneOnlyEvent(t, f.host, "media-started")
			assertPhoneOnlyEvent(t, f.viewer, "media-invite")
			if state := f.s.state(true); state.Media == nil || !state.Media.NativeAudio || !state.Media.PhoneOnly {
				t.Fatal("confirmed native media metadata missing")
			}
			switch stop {
			case "media stop":
				sendMedia(t, f.s, f.viewer, map[string]any{"type": "media-stop", "id": id})
			case "socket close":
				_ = connection.Close()
			case "owner disconnect":
				disconnectAudioTestClient(f, f.host)
			case "viewer disconnect":
				disconnectAudioTestClient(f, f.viewer)
			case "rotation":
				f.s.rotate(httptest.NewRecorder())
			case "server close":
				_ = f.s.Close()
			case "helper exit":
				p.doneOnce.Do(func() { close(p.done) })
			case "PCM error":
				_ = p.writer.CloseWithError(errors.New("private device detail"))
			case "event overflow":
				for i := 0; i < mediaQueueCapacity; i++ {
					f.viewer.events <- []byte(`{}`)
				}
				sendMedia(t, f.s, f.host, signalPacket(id, "offer", description("offer")))
			case "output failure":
				mute.doneOnce.Do(func() { close(mute.done) })
			}
			p.wait(t)
			mute.wait(t)
			f.s.mu.Lock()
			retained := f.s.systemAudio != nil || f.s.media != nil
			f.s.mu.Unlock()
			if retained {
				t.Fatal("stopped source retained reservation or mute session")
			}
		})
	}
}

func TestSystemAudioPendingFactoryCancellationAndLateCleanup(t *testing.T) {
	for _, stop := range []string{"socket close", "owner disconnect", "viewer disconnect", "rotation", "server close"} {
		t.Run(stop, func(t *testing.T) {
			f := newMediaFixture(t)
			p := newSystemAudioProbe(f.s)
			entered, release, canceled := make(chan struct{}), make(chan struct{}), make(chan struct{})
			f.s.config.SystemAudio = func(ctx context.Context) (*SystemAudioCapture, error) {
				_ = f.s.state(true)
				close(entered)
				<-ctx.Done()
				close(canceled)
				<-release
				return p.capture, nil
			}
			connection, _ := dialSystemAudio(t, f)
			<-entered
			if f.s.state(true).Media != nil {
				t.Fatal("pending factory created media")
			}
			switch stop {
			case "socket close":
				_ = connection.Close()
			case "owner disconnect":
				disconnectAudioTestClient(f, f.host)
			case "viewer disconnect":
				disconnectAudioTestClient(f, f.viewer)
			case "rotation":
				f.s.rotate(httptest.NewRecorder())
			case "server close":
				_ = f.s.Close()
			}
			select {
			case <-canceled:
			case <-time.After(time.Second):
				t.Fatal("capture factory context was not canceled")
			}
			close(release)
			p.wait(t)
			f.s.mu.Lock()
			retained := f.s.systemAudio != nil || f.s.media != nil
			f.s.mu.Unlock()
			if retained {
				t.Fatal("late factory resurrected canceled source")
			}
		})
	}
}

func TestSystemAudioFailureDuringPendingMuteRestoresLateLease(t *testing.T) {
	for _, stop := range []string{"socket close", "helper exit", "rotation"} {
		t.Run(stop, func(t *testing.T) {
			f := newMediaFixture(t)
			p := newSystemAudioProbe(f.s)
			mute := newAudioOutputProbe(f.s)
			entered, release, returned := make(chan struct{}), make(chan struct{}), make(chan struct{})
			f.s.config.SystemAudio = func(context.Context) (*SystemAudioCapture, error) { return p.capture, nil }
			f.s.config.AudioOutput = func() (*AudioOutputLease, error) { close(entered); <-release; return mute.lease, nil }
			connection, _ := dialSystemAudio(t, f)
			readSystemAudioReady(t, connection)
			packet := nativeAudioPacket(f)
			packet["phoneOnly"] = true
			data, _ := json.Marshal(packet)
			go func() { f.s.handleSocketMessage(f.host, data); close(returned) }()
			<-entered
			if f.s.state(true).Media != nil {
				t.Fatal("unconfirmed pending mute published active native media")
			}
			switch stop {
			case "socket close":
				_ = connection.Close()
			case "helper exit":
				p.doneOnce.Do(func() { close(p.done) })
			case "rotation":
				f.s.rotate(httptest.NewRecorder())
			}
			select {
			case <-returned:
			case <-time.After(time.Second):
				t.Fatal("failed capture did not cancel pending mute startup")
			}
			p.wait(t)
			close(release)
			mute.wait(t)
			if f.s.state(true).Media != nil {
				t.Fatal("late mute result resurrected failed capture")
			}
			noMedia(t, f.viewer)
		})
	}
}

func TestSystemAudioLateFactoryCannotStopNewSession(t *testing.T) {
	f := newMediaFixture(t)
	old, current := newSystemAudioProbe(f.s), newSystemAudioProbe(f.s)
	release := make(chan struct{})
	f.s.config.SystemAudio = func(context.Context) (*SystemAudioCapture, error) { <-release; return old.capture, nil }
	oldSource := reserveSystemAudioTestSource(f)
	if message := f.s.beginSystemAudio(oldSource, 15*time.Millisecond); message != systemAudioTimeoutError {
		t.Fatalf("old factory did not time out: %q", message)
	}
	f.s.failSystemAudio(oldSource, systemAudioTimeoutError)
	popMedia(t, f.host, "media-error")
	f.s.config.SystemAudio = func(context.Context) (*SystemAudioCapture, error) { return current.capture, nil }
	currentSource := reserveSystemAudioTestSource(f)
	if message := f.s.beginSystemAudio(currentSource, time.Second); message != "" {
		t.Fatal(message)
	}
	sendMedia(t, f.s, f.host, nativeAudioPacket(f))
	id := popMedia(t, f.host, "media-started").ID
	popMedia(t, f.viewer, "media-invite")
	close(release)
	old.wait(t)
	f.s.mu.Lock()
	retained := f.s.systemAudio == currentSource && f.s.media != nil && f.s.media.id == id
	f.s.mu.Unlock()
	if !retained || current.closed.Load() != 0 {
		t.Fatal("old capture cleanup revoked current source or media")
	}
	sendMedia(t, f.s, f.host, map[string]any{"type": "media-stop", "id": "stale-session"})
	popMedia(t, f.host, "media-error")
	if f.s.state(true).Media == nil {
		t.Fatal("stale stop revoked current native source")
	}
	sendMedia(t, f.s, f.host, map[string]any{"type": "media-stop", "id": id})
	current.wait(t)
}

func TestSystemAudioStartupTimeoutAndReauthorization(t *testing.T) {
	for _, kind := range []string{"timeout", "generation changed", "participant removed"} {
		t.Run(kind, func(t *testing.T) {
			f := newMediaFixture(t)
			p := newSystemAudioProbe(f.s)
			entered, release := make(chan struct{}), make(chan struct{})
			f.s.config.SystemAudio = func(context.Context) (*SystemAudioCapture, error) { close(entered); <-release; return p.capture, nil }
			source := reserveSystemAudioTestSource(f)
			returned := make(chan string, 1)
			go func() { returned <- f.s.beginSystemAudio(source, 25*time.Millisecond) }()
			<-entered
			if kind != "timeout" {
				f.s.mu.Lock()
				if kind == "generation changed" {
					f.s.sessionID = "new-generation"
				} else {
					delete(f.s.clients, f.viewer)
				}
				f.s.mu.Unlock()
				close(release)
			}
			select {
			case message := <-returned:
				if kind == "timeout" && message != systemAudioTimeoutError || kind != "timeout" && message != "" {
					t.Fatalf("unexpected startup result %q", message)
				}
			case <-time.After(time.Second):
				t.Fatal("capture startup did not return within deadline")
			}
			f.s.failSystemAudio(source, systemAudioTimeoutError)
			if kind == "timeout" {
				close(release)
			}
			p.wait(t)
			if source.ready || source.capture != nil {
				t.Fatal("timed out or unauthorized capture became ready")
			}
		})
	}
}

func TestSystemAudioStartupErrorsAreSanitizedAndNeverMute(t *testing.T) {
	for _, kind := range []string{"factory error", "nil capture", "wrong rate", "wrong channels", "no close", "no done", "already done"} {
		t.Run(kind, func(t *testing.T) {
			f := newMediaFixture(t)
			p := newSystemAudioProbe(f.s)
			var muteCalls atomic.Int32
			f.s.config.AudioOutput = func() (*AudioOutputLease, error) { muteCalls.Add(1); return nil, nil }
			f.s.config.SystemAudio = func(context.Context) (*SystemAudioCapture, error) {
				switch kind {
				case "factory error":
					return p.capture, errors.New("SECRET private pipe nonce")
				case "nil capture":
					return nil, nil
				case "wrong rate":
					p.capture.SampleRate = 44100
				case "wrong channels":
					p.capture.Channels = 1
				case "no close":
					p.capture.Close = nil
				case "no done":
					p.capture.Done = nil
				case "already done":
					p.doneOnce.Do(func() { close(p.done) })
				}
				return p.capture, nil
			}
			connection, _ := dialSystemAudio(t, f)
			_ = connection.SetReadDeadline(time.Now().Add(time.Second))
			messageKind, data, err := connection.ReadMessage()
			var message struct{ Type, Message string }
			if err != nil || messageKind != websocket.TextMessage || json.Unmarshal(data, &message) != nil || message.Type != "error" || strings.Contains(message.Message, "SECRET") || message.Message == "" {
				t.Fatalf("missing sanitized capture error: %s %v", data, err)
			}
			sendMedia(t, f.s, f.host, phoneOnlyPacket(f))
			if muteCalls.Load() != 0 || f.s.state(true).Media != nil {
				t.Fatal("failed native capture fell back to browser capture plus mute")
			}
			if kind != "nil capture" && p.capture.Close != nil {
				p.wait(t)
			} else if p.capture.Close == nil {
				deadline := time.Now().Add(time.Second)
				for p.pcmClosed.Load() != 1 {
					if time.Now().After(deadline) {
						t.Fatal("invalid capture PCM was not closed")
					}
					time.Sleep(time.Millisecond)
				}
				_ = p.writer.Close()
			}
		})
	}
}

func TestSystemAudioUnattachedTimeout(t *testing.T) {
	f := newMediaFixture(t)
	p := newSystemAudioProbe(f.s)
	source := reserveSystemAudioTestSource(f)
	f.s.mu.Lock()
	source.capture, source.ready = p.capture, true
	f.s.mu.Unlock()
	finished := make(chan struct{})
	go func() { f.s.streamSystemAudio(source, p.capture, 15*time.Millisecond); close(finished) }()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("unattached source timeout did not release reservation")
	}
	p.wait(t)
	if message := popMedia(t, f.host, "media-error").Message; message != systemAudioExpiredError {
		t.Fatalf("wrong unattached source error: %q", message)
	}
}

func TestSystemAudioPCMReaderHandlesChunksAndIncompleteEOF(t *testing.T) {
	for _, size := range []int{systemAudioChunkBytes*2 + 4, systemAudioChunkBytes + 3} {
		ctx, cancel := context.WithCancel(context.Background())
		frames := make(chan systemAudioFrame, 1)
		payload := bytes.Repeat([]byte{0x12}, size)
		go readSystemAudioPCM(ctx, bytes.NewReader(payload), frames)
		var received []byte
		for {
			select {
			case frame := <-frames:
				if frame.err != nil {
					if size%4 != 0 && frame.err != io.ErrUnexpectedEOF || size%4 == 0 && frame.err != io.EOF {
						t.Fatalf("wrong stream termination: %v", frame.err)
					}
					if !bytes.Equal(received, payload[:size-size%4]) {
						t.Fatal("reader changed complete PCM frames")
					}
					cancel()
					goto nextSize
				}
				if len(frame.data)%4 != 0 || len(frame.data) > systemAudioChunkBytes {
					t.Fatal("reader emitted partial or oversized frame")
				}
				received = append(received, frame.data...)
			case <-time.After(time.Second):
				t.Fatal("reader did not finish")
			}
		}
	nextSize:
	}
}

type systemAudioSlowWriter struct{ closed chan struct{} }

func (writer *systemAudioSlowWriter) Read(buffer []byte) (int, error) {
	select {
	case <-writer.closed:
		return 0, io.EOF
	default:
	}
	clear(buffer)
	return len(buffer), nil
}

func (writer *systemAudioSlowWriter) Close() error { close(writer.closed); return nil }

func TestSystemAudioSlowSocketHasBoundedWrites(t *testing.T) {
	f := newMediaFixture(t)
	pcm := &systemAudioSlowWriter{closed: make(chan struct{})}
	finished := make(chan struct{})
	f.s.config.SystemAudio = func(context.Context) (*SystemAudioCapture, error) {
		return &SystemAudioCapture{PCM: pcm, SampleRate: 48000, Channels: 2, Done: make(chan struct{}), Close: func() error { close(finished); return nil }}, nil
	}
	web := httptest.NewServer(f.s.Handler())
	defer web.Close()
	parsed, _ := url.Parse(web.URL)
	dialer := websocket.Dialer{NetDialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		connection, err := (&net.Dialer{}).DialContext(ctx, network, address)
		if tcp, ok := connection.(*net.TCPConn); ok {
			_ = tcp.SetReadBuffer(1024)
		}
		return connection, err
	}}
	connection, _, err := dialer.Dial("ws://"+parsed.Host+systemAudioPath(f), http.Header{"Origin": {web.URL}, "Cookie": {hostSession(t, f.s).String()}})
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	readSystemAudioReady(t, connection)
	select {
	case <-finished:
	case <-time.After(systemAudioWriteTimeout + 3*time.Second):
		t.Fatal("slow client did not reach bounded write timeout")
	}
	f.s.mu.Lock()
	retained := f.s.systemAudio != nil
	f.s.mu.Unlock()
	if retained {
		t.Fatal("slow client retained capture reservation")
	}
}
