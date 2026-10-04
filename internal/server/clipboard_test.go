package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func waitClipboardResponse(t *testing.T, response <-chan *httptest.ResponseRecorder) *httptest.ResponseRecorder {
	t.Helper()
	select {
	case w := <-response:
		return w
	case <-time.After(3 * time.Second):
		t.Fatal("clipboard request did not finish")
		return nil
	}
}

func TestAutoCopyCompletesBeforePublicationAndClipboardRead(t *testing.T) {
	s := newTestServer(t)
	host, peer := hostSession(t, s), peerSession(t, s)
	request(t, s, http.MethodPost, "/api/settings", strings.NewReader(`{"autoCopy":true}`), host, true, nil)
	started, release, readStarted := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	finish := func() { releaseOnce.Do(func() { close(release) }) }
	defer finish()
	var nativeMu sync.Mutex
	current := "previous synthetic clipboard"
	s.config.ClipboardWrite = func(value string) error {
		close(started)
		<-release
		nativeMu.Lock()
		current = value
		nativeMu.Unlock()
		return nil
	}
	s.config.ClipboardRead = func() (string, error) {
		close(readStarted)
		nativeMu.Lock()
		defer nativeMu.Unlock()
		return current, nil
	}
	c := &wsClient{host: true, queue: make(chan []byte, 1), done: make(chan struct{})}
	s.mu.Lock()
	s.clients[c] = struct{}{}
	s.mu.Unlock()
	posted := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		posted <- request(t, s, http.MethodPost, "/api/text", strings.NewReader(`{"text":"latest Unicode 🦷\nsecond line"}`), peer, false, nil)
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("copy did not start")
	}
	select {
	case <-c.queue:
		t.Error("receipt was published before the clipboard write finished")
	default:
	}
	read := make(chan *httptest.ResponseRecorder, 1)
	go func() { read <- request(t, s, http.MethodGet, "/api/clipboard", nil, host, true, nil) }()
	select {
	case <-readStarted:
		t.Error("clipboard read overtook a received-text copy")
	case <-time.After(100 * time.Millisecond):
	}
	finish()
	if w := waitClipboardResponse(t, posted); w.Code != http.StatusCreated {
		t.Fatalf("text response: %d %s", w.Code, w.Body.String())
	}
	w := waitClipboardResponse(t, read)
	var result struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || w.Code != http.StatusOK || result.Text != "latest Unicode 🦷\nsecond line" {
		t.Fatalf("clipboard read returned stale text: %d %s (%v)", w.Code, w.Body.String(), err)
	}
	select {
	case data := <-c.queue:
		if !strings.Contains(string(data), `"status":"copied"`) {
			t.Fatalf("receipt lacks completed copy status: %s", data)
		}
	default:
		t.Fatal("completed receipt was not published")
	}
}

func TestConcurrentAutoCopyKeepsNewestReceivedText(t *testing.T) {
	s := newTestServer(t)
	host, peer := hostSession(t, s), peerSession(t, s)
	request(t, s, http.MethodPost, "/api/settings", strings.NewReader(`{"autoCopy":true}`), host, true, nil)
	firstStarted, secondStarted, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	finish := func() { releaseOnce.Do(func() { close(release) }) }
	defer finish()
	var mu sync.Mutex
	var copied []string
	s.config.ClipboardWrite = func(value string) error {
		if value == "first" {
			close(firstStarted)
			<-release
		} else {
			close(secondStarted)
		}
		mu.Lock()
		copied = append(copied, value)
		mu.Unlock()
		return nil
	}
	first, second := make(chan *httptest.ResponseRecorder, 1), make(chan *httptest.ResponseRecorder, 1)
	go func() {
		first <- request(t, s, http.MethodPost, "/api/text", strings.NewReader(`{"text":"first"}`), peer, false, nil)
	}()
	select {
	case <-firstStarted:
	case <-time.After(3 * time.Second):
		t.Fatal("first copy did not start")
	}
	go func() {
		second <- request(t, s, http.MethodPost, "/api/text", strings.NewReader(`{"text":"second"}`), peer, false, nil)
	}()
	select {
	case <-secondStarted:
		t.Error("second copy overtook the first received text")
	case <-time.After(100 * time.Millisecond):
	}
	finish()
	for _, response := range []<-chan *httptest.ResponseRecorder{first, second} {
		if w := waitClipboardResponse(t, response); w.Code != http.StatusCreated {
			t.Fatalf("text response: %d %s", w.Code, w.Body.String())
		}
	}
	mu.Lock()
	defer mu.Unlock()
	state := s.state(true)
	if len(copied) != 2 || copied[0] != "first" || copied[1] != "second" || state.Texts[0].Text != copied[1] {
		t.Fatalf("copy order %v disagrees with newest text %+v", copied, state.Texts)
	}
}

func TestAutoCopyStatusAndReset(t *testing.T) {
	for _, tc := range []struct {
		name   string
		writer func(string) error
		status string
	}{
		{"copied", func(string) error { return nil }, "copied"},
		{"busy", func(string) error { return errors.New("synthetic clipboard busy") }, "error"},
		{"unavailable", nil, "error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestServer(t)
			host, peer := hostSession(t, s), peerSession(t, s)
			s.config.ClipboardWrite = tc.writer
			request(t, s, http.MethodPost, "/api/settings", strings.NewReader(`{"autoCopy":true}`), host, true, nil)
			w := request(t, s, http.MethodPost, "/api/text", strings.NewReader(`{"text":"received text"}`), peer, false, nil)
			var receipt struct {
				Text
				ClipboardCopied bool   `json:"clipboardCopied"`
				ClipboardError  string `json:"clipboardError"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &receipt); err != nil || w.Code != http.StatusCreated {
				t.Fatalf("text receipt: %d %s (%v)", w.Code, w.Body.String(), err)
			}
			if receipt.ClipboardCopied != (tc.status == "copied") || (receipt.ClipboardError != "") != (tc.status == "error") {
				t.Fatalf("incorrect copy result: %+v", receipt)
			}
			var state struct {
				Clipboard *struct {
					TextID string `json:"textId"`
					Status string `json:"status"`
					Error  string `json:"error"`
				} `json:"clipboard"`
			}
			w = request(t, s, http.MethodGet, "/api/state", nil, host, true, nil)
			if err := json.Unmarshal(w.Body.Bytes(), &state); err != nil || state.Clipboard == nil || state.Clipboard.TextID != receipt.ID || state.Clipboard.Status != tc.status || state.Clipboard.Error != receipt.ClipboardError {
				t.Fatalf("host copy status: %s (%v)", w.Body.String(), err)
			}
			w = request(t, s, http.MethodGet, "/api/state", nil, peer, false, nil)
			if strings.Contains(w.Body.String(), `"clipboard"`) {
				t.Fatal("host clipboard status leaked to peer state")
			}
			w = request(t, s, http.MethodDelete, "/api/texts", nil, host, true, nil)
			if strings.Contains(w.Body.String(), `"clipboard"`) {
				t.Fatal("clipboard status retained after clearing text history")
			}
			request(t, s, http.MethodPost, "/api/text", strings.NewReader(`{"text":"another received text"}`), peer, false, nil)
			w = request(t, s, http.MethodPost, "/api/settings", strings.NewReader(`{"autoCopy":false}`), host, true, nil)
			if strings.Contains(w.Body.String(), `"clipboard"`) {
				t.Fatal("clipboard status retained after disabling auto-copy")
			}
		})
	}
}

func TestAutoCopyDoesNotCopyDisabledOrHostText(t *testing.T) {
	s := newTestServer(t)
	host, peer := hostSession(t, s), peerSession(t, s)
	copies := 0
	s.config.ClipboardWrite = func(string) error { copies++; return nil }
	for _, tc := range []struct {
		name   string
		enable bool
		local  bool
		cookie *http.Cookie
	}{
		{"peer disabled", false, false, peer},
		{"host enabled", true, true, host},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.enable {
				request(t, s, http.MethodPost, "/api/settings", strings.NewReader(`{"autoCopy":true}`), host, true, nil)
			}
			w := request(t, s, http.MethodPost, "/api/text", strings.NewReader(`{"text":"outbound or manual text"}`), tc.cookie, tc.local, nil)
			if w.Code != http.StatusCreated || strings.Contains(w.Body.String(), `"clipboardCopied"`) || strings.Contains(w.Body.String(), `"clipboardError"`) {
				t.Fatalf("unexpected copy feedback: %d %s", w.Code, w.Body.String())
			}
			if copies != 0 || s.state(true).Clipboard != nil {
				t.Fatal("disabled or host text changed the Windows clipboard")
			}
		})
	}
}

func TestDisableAutoCopyWaitsForInFlightWrite(t *testing.T) {
	s := newTestServer(t)
	host, peer := hostSession(t, s), peerSession(t, s)
	request(t, s, http.MethodPost, "/api/settings", strings.NewReader(`{"autoCopy":true}`), host, true, nil)
	started, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	finish := func() { releaseOnce.Do(func() { close(release) }) }
	defer finish()
	s.config.ClipboardWrite = func(string) error {
		close(started)
		<-release
		return nil
	}
	posted, disabled := make(chan *httptest.ResponseRecorder, 1), make(chan *httptest.ResponseRecorder, 1)
	go func() {
		posted <- request(t, s, http.MethodPost, "/api/text", strings.NewReader(`{"text":"in-flight received text"}`), peer, false, nil)
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("copy did not start")
	}
	go func() {
		disabled <- request(t, s, http.MethodPost, "/api/settings", strings.NewReader(`{"autoCopy":false}`), host, true, nil)
	}()
	select {
	case w := <-disabled:
		t.Error("disable completed while an automatic copy was still running")
		disabled <- w
	case <-time.After(100 * time.Millisecond):
	}
	finish()
	if w := waitClipboardResponse(t, posted); w.Code != http.StatusCreated {
		t.Fatalf("text response: %d %s", w.Code, w.Body.String())
	}
	w := waitClipboardResponse(t, disabled)
	if w.Code != http.StatusOK || s.state(true).Settings.AutoCopy || s.state(true).Clipboard != nil {
		t.Fatalf("disable did not clear copy status: %d %s", w.Code, w.Body.String())
	}
}

func TestQueuedAutoCopyRejectsRevokedPeer(t *testing.T) {
	s := newTestServer(t)
	host, peer := hostSession(t, s), peerSession(t, s)
	request(t, s, http.MethodPost, "/api/settings", strings.NewReader(`{"autoCopy":true}`), host, true, nil)
	started, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	finish := func() { releaseOnce.Do(func() { close(release) }) }
	defer finish()
	var mu sync.Mutex
	copies := 0
	s.config.ClipboardWrite = func(string) error {
		mu.Lock()
		copies++
		first := copies == 1
		mu.Unlock()
		if first {
			close(started)
			<-release
		}
		return nil
	}
	first, queued := make(chan *httptest.ResponseRecorder, 1), make(chan *httptest.ResponseRecorder, 1)
	go func() {
		first <- request(t, s, http.MethodPost, "/api/text", strings.NewReader(`{"text":"accepted before rotation"}`), peer, false, nil)
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("first copy did not start")
	}
	p := principal{generation: s.sessionID}
	go func() {
		r := httptest.NewRequest(http.MethodPost, "/api/text", strings.NewReader(`{"text":"queued old session"}`))
		w := httptest.NewRecorder()
		s.addText(w, r, p)
		queued <- w
	}()
	select {
	case w := <-queued:
		t.Error("queued text was accepted while an earlier copy was still running")
		queued <- w
	case <-time.After(100 * time.Millisecond):
	}
	rotated := make(chan *httptest.ResponseRecorder, 1)
	go func() { rotated <- request(t, s, http.MethodPost, "/api/session/rotate", nil, host, true, nil) }()
	if w := waitClipboardResponse(t, rotated); w.Code != http.StatusOK {
		t.Fatalf("rotation was blocked by native clipboard: %d %s", w.Code, w.Body.String())
	}
	finish()
	if w := waitClipboardResponse(t, first); w.Code != http.StatusCreated {
		t.Fatalf("accepted text failed: %d %s", w.Code, w.Body.String())
	}
	if w := waitClipboardResponse(t, queued); w.Code != http.StatusUnauthorized {
		t.Fatalf("revoked queued peer copied: %d %s", w.Code, w.Body.String())
	}
	mu.Lock()
	defer mu.Unlock()
	if copies != 1 || len(s.state(true).Texts) != 1 {
		t.Fatalf("revoked peer left a side effect: copies=%d", copies)
	}
}
