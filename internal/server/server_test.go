package server

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"image/png"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	s, err := New(Config{DeviceName: "Laptop uji", StorageDir: t.TempDir(), Port: 8787, Addresses: []NetworkAddress{{Name: "Wi-Fi", IP: "192.168.1.25"}}})
	if err != nil {
		t.Fatal(err)
	}
	s.SetAssets(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "app") }))
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func request(t *testing.T, s *Server, method, path string, body io.Reader, cookie *http.Cookie, local bool, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	host, remote := "192.168.1.25:8787", "192.168.1.30:12345"
	if local {
		host, remote = "localhost:8787", "127.0.0.1:12345"
	}
	r := httptest.NewRequest(method, "http://"+host+path, body)
	r.RemoteAddr = remote
	if cookie != nil {
		r.AddCookie(cookie)
	}
	for key, value := range headers {
		r.Header.Set(key, value)
	}
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	return w
}

func hostSession(t *testing.T, s *Server) *http.Cookie {
	t.Helper()
	w := request(t, s, http.MethodGet, "/", nil, nil, true, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("bootstrap: %d %s", w.Code, w.Body.String())
	}
	for _, cookie := range w.Result().Cookies() {
		if cookie.Name == hostCookie {
			return cookie
		}
	}
	t.Fatal("missing host cookie")
	return nil
}

func peerSession(t *testing.T, s *Server) *http.Cookie {
	t.Helper()
	u, err := url.Parse(s.JoinURL())
	if err != nil {
		t.Fatal(err)
	}
	w := request(t, s, http.MethodGet, u.Path, nil, nil, false, nil)
	if w.Code != http.StatusOK || w.Header().Get("Location") != "" || !strings.Contains(w.Body.String(), `id="pairing-continue" href="/"`) {
		t.Fatalf("pair: %d %s", w.Code, w.Body.String())
	}
	for _, cookie := range w.Result().Cookies() {
		if cookie.Name == peerCookie {
			return cookie
		}
	}
	t.Fatal("missing peer cookie")
	return nil
}

func decodeState(t *testing.T, w *httptest.ResponseRecorder) State {
	t.Helper()
	if w.Code != http.StatusOK {
		t.Fatalf("state: %d %s", w.Code, w.Body.String())
	}
	var state State
	if err := json.Unmarshal(w.Body.Bytes(), &state); err != nil {
		t.Fatal(err)
	}
	return state
}

func readSocketState(t *testing.T, connection *websocket.Conn, target any) {
	t.Helper()
	for i := 0; i < 20; i++ {
		_, data, err := connection.ReadMessage()
		if err != nil {
			t.Fatal(err)
		}
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(data, &envelope); err != nil {
			t.Fatal(err)
		}
		if envelope.Type != "state" {
			continue
		}
		if err := json.Unmarshal(data, target); err != nil {
			t.Fatal(err)
		}
		return
	}
	t.Fatal("state message missing")
}

func TestAuthorizationAndOrigins(t *testing.T) {
	s := newTestServer(t)
	host := hostSession(t, s)
	peer := peerSession(t, s)
	if !host.HttpOnly || host.SameSite != http.SameSiteStrictMode || !peer.HttpOnly || peer.SameSite != http.SameSiteStrictMode {
		t.Fatal("cookies must be HttpOnly and SameSite Strict")
	}
	for _, tc := range []struct {
		name, method, path string
		cookie             *http.Cookie
		local              bool
		headers            map[string]string
		status             int
	}{
		{"anonymous API", http.MethodGet, "/api/state", nil, false, nil, 401},
		{"remote host cookie", http.MethodGet, "/api/clipboard", host, false, map[string]string{"X-Forwarded-For": "127.0.0.1", "X-Device-ID": "host"}, 401},
		{"peer settings", http.MethodPost, "/api/settings", peer, false, nil, 403},
		{"peer rotate", http.MethodPost, "/api/session/rotate", peer, false, nil, 403},
		{"peer clipboard", http.MethodGet, "/api/clipboard", peer, false, nil, 403},
		{"peer folder", http.MethodPost, "/api/open-folder", peer, false, nil, 403},
		{"peer QR", http.MethodGet, "/api/qr", peer, false, nil, 403},
		{"peer clear", http.MethodDelete, "/api/texts", peer, false, nil, 403},
		{"cross origin", http.MethodPost, "/api/session/rotate", host, true, map[string]string{"Origin": "https://evil.example"}, 403},
		{"null origin", http.MethodGet, "/api/state", host, true, map[string]string{"Origin": "null"}, 403},
		{"cross-site no origin", http.MethodGet, "/api/state", host, true, map[string]string{"Sec-Fetch-Site": "cross-site"}, 403},
		{"same origin", http.MethodGet, "/api/state", host, true, map[string]string{"Origin": "http://localhost:8787"}, 200},
		{"health public", http.MethodGet, "/api/health", nil, false, nil, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := request(t, s, tc.method, tc.path, strings.NewReader(`{"autoCopy":true}`), tc.cookie, tc.local, tc.headers)
			if w.Code != tc.status {
				t.Fatalf("got %d, want %d: %s", w.Code, tc.status, w.Body.String())
			}
		})
	}
	for _, tc := range []struct{ host, remote string }{{"evil.example:8787", "127.0.0.1:99"}, {"localhost:8787", "192.168.1.30:99"}, {"192.168.1.25:8787", "127.0.0.1:99"}} {
		r := httptest.NewRequest(http.MethodGet, "http://"+tc.host+"/api/state", nil)
		r.RemoteAddr = tc.remote
		r.Header.Set("X-Forwarded-For", "127.0.0.1")
		r.AddCookie(host)
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		if w.Code == 200 {
			t.Fatalf("forged host accepted: %+v", tc)
		}
	}
	state := decodeState(t, request(t, s, http.MethodGet, "/api/state", nil, peer, false, nil))
	if state.Role != "peer" || state.JoinURL != "" || state.StoragePath != "" || state.QRURL != "" || len(state.Addresses) != 0 {
		t.Fatalf("peer received privileged state: %+v", state)
	}
	w := request(t, s, http.MethodGet, "/", nil, nil, false, nil)
	if w.Code != 401 || !strings.Contains(w.Body.String(), "Scan QR terbaru") || strings.Contains(w.Body.String(), s.peerSecret) {
		t.Fatal("unpaired page is missing instructions or leaks token")
	}
	if w.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Fatal("token could leak through referrer")
	}
}

func TestRotationAndTextDelivery(t *testing.T) {
	s := newTestServer(t)
	host, peer := hostSession(t, s), peerSession(t, s)
	if w := request(t, s, http.MethodPost, "/api/settings", strings.NewReader(`{"autoCopy":true}`), host, true, nil); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	s.config.ClipboardWrite = func(string) error { return errors.New("clipboard sibuk") }
	w := request(t, s, http.MethodPost, "/api/text", strings.NewReader(`{"text":"Halo — gigi 🦷\nbaris kedua"}`), peer, false, map[string]string{"X-Device-ID": "host"})
	if w.Code != 201 {
		t.Fatalf("text: %d %s", w.Code, w.Body.String())
	}
	var entry struct {
		Text
		ClipboardError string `json:"clipboardError"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &entry); err != nil {
		t.Fatal(err)
	}
	if entry.Sender != "Perangkat" || entry.ClipboardError == "" || entry.Text.Text != "Halo — gigi 🦷\nbaris kedua" {
		t.Fatalf("unexpected text: %+v", entry)
	}
	before := decodeState(t, request(t, s, http.MethodGet, "/api/state", nil, host, true, nil))
	oldURL, _ := url.Parse(before.JoinURL)
	after := decodeState(t, request(t, s, http.MethodPost, "/api/session/rotate", nil, host, true, nil))
	if before.JoinURL == after.JoinURL || before.SessionID == after.SessionID || len(after.Texts) != 1 || after.Role != "host" {
		t.Fatal("rotation did not preserve host/history and change token")
	}
	if w := request(t, s, http.MethodGet, "/api/state", nil, peer, false, nil); w.Code != 401 {
		t.Fatal("old peer cookie remains valid")
	}
	if w := request(t, s, http.MethodGet, oldURL.Path, nil, nil, false, nil); w.Code != 401 {
		t.Fatal("old join token remains valid")
	}
	if w := request(t, s, http.MethodGet, "/api/state", nil, peerSession(t, s), false, nil); w.Code != 200 {
		t.Fatal("new token cannot pair")
	}
	if w := request(t, s, http.MethodDelete, "/api/texts", nil, host, true, nil); w.Code != 200 {
		t.Fatal("host cannot clear text")
	}
	if state := s.state(true); len(state.Texts) != 0 {
		t.Fatal("text clear failed")
	}
}

func TestTextLimitsAndHostClipboard(t *testing.T) {
	s := newTestServer(t)
	host := hostSession(t, s)
	copies := 0
	s.config.ClipboardWrite = func(string) error { copies++; return nil }
	s.config.ClipboardRead = func() (string, error) { return "clipboard Unicode 🦷", nil }
	s.config.OpenFolder = func() error { return nil }
	request(t, s, http.MethodPost, "/api/settings", strings.NewReader(`{"autoCopy":true}`), host, true, nil)
	for i := 0; i < 105; i++ {
		w := request(t, s, http.MethodPost, "/api/text", strings.NewReader(`{"text":"pesan"}`), host, true, nil)
		if w.Code != 201 {
			t.Fatal(w.Body.String())
		}
	}
	if state := s.state(true); len(state.Texts) != 100 {
		t.Fatalf("history size %d", len(state.Texts))
	}
	if copies != 0 {
		t.Fatal("host outbound text was auto-copied")
	}
	for _, body := range []string{`{"text":"  "}`, `{"text":"ok"} {"text":"second"}`, `{"text":"ok","role":"host"}`} {
		if w := request(t, s, http.MethodPost, "/api/text", strings.NewReader(body), host, true, nil); w.Code != 400 {
			t.Fatalf("invalid body accepted: %s", body)
		}
	}
	tooLarge := `{"text":"` + strings.Repeat("a", maxJSONBytes) + `"}`
	if w := request(t, s, http.MethodPost, "/api/text", strings.NewReader(tooLarge), host, true, nil); w.Code != 413 {
		t.Fatalf("oversized text: %d", w.Code)
	}
	if w := request(t, s, http.MethodGet, "/api/clipboard", nil, host, true, nil); w.Code != 200 || !strings.Contains(w.Body.String(), "clipboard Unicode") {
		t.Fatal("clipboard response failed")
	}
	if w := request(t, s, http.MethodPost, "/api/open-folder", nil, host, true, nil); w.Code != 200 {
		t.Fatal("folder response failed")
	}
}

func multipartBody(t *testing.T, name string, data []byte) ([]byte, string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("files", name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return body.Bytes(), writer.FormDataContentType()
}

func uploadTestFile(t *testing.T, s *Server, cookie *http.Cookie, name string, data []byte) File {
	t.Helper()
	body, contentType := multipartBody(t, name, data)
	w := request(t, s, http.MethodPost, "/api/files", bytes.NewReader(body), cookie, false, map[string]string{"Content-Type": contentType})
	if w.Code != 201 {
		t.Fatalf("upload: %d %s", w.Code, w.Body.String())
	}
	var response struct {
		Files []File `json:"files"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Files) != 1 {
		t.Fatal("expected one uploaded file")
	}
	return response.Files[0]
}

func TestStreamUploadDownloadAndSafeNames(t *testing.T) {
	s := newTestServer(t)
	peer := peerSession(t, s)
	data := bytes.Repeat([]byte("streamed Unicode 🦷\n"), 50000)
	first := uploadTestFile(t, s, peer, "../../foto gigi 🦷.txt", data)
	second := uploadTestFile(t, s, peer, `C:\Windows\foto gigi 🦷.txt`, []byte("kedua"))
	if first.Name != "foto gigi 🦷.txt" || second.Name != "foto gigi 🦷 (2).txt" || first.Size != int64(len(data)) {
		t.Fatalf("unsafe/colliding names: %+v %+v", first, second)
	}
	w := request(t, s, http.MethodGet, first.DownloadURL, nil, peer, false, nil)
	if w.Code != 200 || !bytes.Equal(w.Body.Bytes(), data) {
		t.Fatal("download differs from upload")
	}
	disposition, params, err := mime.ParseMediaType(w.Header().Get("Content-Disposition"))
	if err != nil || disposition != "attachment" || params["filename"] != first.Name {
		t.Fatalf("unsafe disposition: %s", w.Header().Get("Content-Disposition"))
	}
	w = request(t, s, http.MethodGet, first.DownloadURL, nil, peer, false, map[string]string{"Range": "bytes=4-19"})
	if w.Code != 206 || !bytes.Equal(w.Body.Bytes(), data[4:20]) {
		t.Fatalf("range support failed: %d", w.Code)
	}
	html := uploadTestFile(t, s, peer, "<script>.html", []byte("<html><script>alert(1)</script></html>"))
	w = request(t, s, http.MethodGet, html.DownloadURL, nil, peer, false, nil)
	if !strings.HasPrefix(w.Header().Get("Content-Disposition"), "attachment") || w.Header().Get("X-Content-Type-Options") != "nosniff" || !strings.Contains(w.Header().Get("Content-Security-Policy"), "sandbox") {
		t.Fatal("HTML upload can execute in app origin")
	}
	for _, tc := range []struct{ input, want string }{{"CON.txt", "_CON.txt"}, {"LPT1", "_LPT1"}, {"..", "file"}, {"../x?.txt", "x_.txt"}, {"a.txt. ", "a.txt"}, {"COM¹.txt", "_COM¹.txt"}} {
		if got := safeFilename(tc.input); got != tc.want {
			t.Errorf("safeFilename(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
	entries, err := os.ReadDir(s.config.StorageDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("unexpected files/temp residue: %v", entries)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	restarted, err := New(s.config)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	if len(restarted.state(true).Files) != 0 || len(restarted.state(true).Texts) != 0 {
		t.Fatal("metadata/history persisted across restart")
	}
	if stored, err := os.ReadFile(filepath.Join(s.config.StorageDir, first.Name)); err != nil || !bytes.Equal(stored, data) {
		t.Fatal("completed file was removed on restart")
	}
}

func TestAbortedAndInvalidUploadsLeaveNoFiles(t *testing.T) {
	s := newTestServer(t)
	peer := peerSession(t, s)
	body, contentType := multipartBody(t, "partial.bin", bytes.Repeat([]byte("x"), 100000))
	truncated := body[:len(body)-100]
	w := request(t, s, http.MethodPost, "/api/files", bytes.NewReader(truncated), peer, false, map[string]string{"Content-Type": contentType})
	if w.Code != 400 {
		t.Fatalf("truncated upload: %d %s", w.Code, w.Body.String())
	}
	var multiple bytes.Buffer
	writer := multipart.NewWriter(&multiple)
	for _, name := range []string{"one.txt", "two.txt"} {
		part, err := writer.CreateFormFile("files", name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = part.Write([]byte("payload"))
	}
	_ = writer.Close()
	w = request(t, s, http.MethodPost, "/api/files", &multiple, peer, false, map[string]string{"Content-Type": writer.FormDataContentType()})
	if w.Code != 400 {
		t.Fatal("multiple files were committed")
	}
	entries, err := os.ReadDir(s.config.StorageDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 || len(s.state(true).Files) != 0 {
		t.Fatal("partial upload or metadata leaked")
	}
}

func TestRealQR(t *testing.T) {
	s := newTestServer(t)
	host := hostSession(t, s)
	w := request(t, s, http.MethodGet, "/api/qr?ip=192.168.1.25", nil, host, true, nil)
	if w.Code != 200 || w.Header().Get("Content-Type") != "image/png" || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("QR response failed")
	}
	image, err := png.Decode(bytes.NewReader(w.Body.Bytes()))
	if err != nil || image.Bounds().Dx() != 320 || image.Bounds().Dy() != 320 {
		t.Fatal("QR is not a real 320px PNG")
	}
	if w := request(t, s, http.MethodGet, "/api/qr?ip=evil.example", nil, host, true, nil); w.Code != 400 {
		t.Fatal("QR accepts arbitrary host")
	}
}

func TestStreamingUploadRevokedBeforeCompletion(t *testing.T) {
	s := newTestServer(t)
	host, peer := hostSession(t, s), peerSession(t, s)
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	multipartWriter := multipart.NewWriter(writer)
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		done <- request(t, s, http.MethodPost, "/api/files", reader, peer, false,
			map[string]string{"Content-Type": multipartWriter.FormDataContentType()})
	}()
	chunkWritten := make(chan error, 1)
	go func() {
		part, err := multipartWriter.CreateFormFile("files", "streaming.bin")
		if err == nil {
			_, err = part.Write(bytes.Repeat([]byte("stream"), 20000))
		}
		chunkWritten <- err
		// Deliberately leave the multipart boundary and body unfinished.
	}()
	select {
	case err := <-chunkWritten:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("server did not consume the streaming body")
	}
	deadline := time.Now().Add(3 * time.Second)
	sawPartial := false
	for time.Now().Before(deadline) {
		entries, err := os.ReadDir(s.config.StorageDir)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) == 1 {
			partial, err := os.Open(filepath.Join(s.config.StorageDir, entries[0].Name()))
			if err != nil {
				t.Fatal(err)
			}
			var bytesOnDisk [32]byte
			n, _ := partial.Read(bytesOnDisk[:])
			_ = partial.Close()
			if n > 0 && strings.HasSuffix(entries[0].Name(), ".part") {
				sawPartial = true
				break
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !sawPartial {
		t.Fatal("file bytes were not streamed to disk before the body completed")
	}
	if len(s.state(true).Files) != 0 {
		t.Fatal("incomplete file was published")
	}
	if w := request(t, s, http.MethodPost, "/api/session/rotate", nil, host, true, nil); w.Code != 200 {
		t.Fatal("rotation failed")
	}
	select {
	case response := <-done:
		if response.Code == http.StatusCreated {
			t.Fatal("revoked upload succeeded")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("rotation failed to close the stalled upload body")
	}
	entries, err := os.ReadDir(s.config.StorageDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 || len(s.state(true).Files) != 0 {
		t.Fatal("revoked stream left files or metadata")
	}
}

func TestTCPStalledUploadIsRemovedOnRotation(t *testing.T) {
	s := newTestServer(t)
	host, peer := hostSession(t, s), peerSession(t, s)
	web := httptest.NewServer(s.Handler())
	defer web.Close()
	u, err := url.Parse(web.URL)
	if err != nil {
		t.Fatal(err)
	}
	connection, err := net.DialTimeout("tcp", u.Host, 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("files", "stalled-network.bin")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write(bytes.Repeat([]byte("network-stream"), 10000))
	// Announce a much larger body and keep the TCP socket open without sending
	// the remaining data. Body.Close alone deadlocks behind Body.Read here.
	var wire bytes.Buffer
	_, _ = io.WriteString(&wire, "POST /api/files HTTP/1.1\r\nHost: "+u.Host+"\r\nCookie: "+peer.String()+"\r\nContent-Type: "+writer.FormDataContentType()+"\r\nContent-Length: 10485760\r\n\r\n")
	_, _ = wire.Write(body.Bytes())
	_ = connection.SetWriteDeadline(time.Now().Add(3 * time.Second))
	if _, err := connection.Write(wire.Bytes()); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	sawPartial := false
	for time.Now().Before(deadline) {
		entries, err := os.ReadDir(s.config.StorageDir)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) == 1 {
			partial, err := os.Open(filepath.Join(s.config.StorageDir, entries[0].Name()))
			if err != nil {
				t.Fatal(err)
			}
			var data [16]byte
			n, _ := partial.Read(data[:])
			_ = partial.Close()
			if n > 0 {
				sawPartial = true
				break
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !sawPartial {
		t.Fatal("TCP upload did not start writing a temporary file")
	}
	rotation, err := http.NewRequest(http.MethodPost, web.URL+"/api/session/rotate", nil)
	if err != nil {
		t.Fatal(err)
	}
	rotation.AddCookie(host)
	client := &http.Client{Timeout: 3 * time.Second}
	response, err := client.Do(rotation)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatalf("rotation status %d", response.StatusCode)
	}
	_ = connection.SetReadDeadline(time.Now().Add(3 * time.Second))
	response, err = http.ReadResponse(bufio.NewReader(connection), nil)
	if err != nil {
		t.Fatalf("stalled request did not terminate after rotation: %v", err)
	}
	_, _ = io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
	if response.StatusCode == http.StatusCreated {
		t.Fatal("stalled revoked upload was committed")
	}
	// The client socket is still open; cleanup must come from server revocation.
	entries, err := os.ReadDir(s.config.StorageDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 || len(s.state(true).Files) != 0 {
		t.Fatal("rotated TCP stream left partial files or metadata")
	}
}

func TestWebSocketAuthDeviceLifecycleAndRotation(t *testing.T) {
	s := newTestServer(t)
	host, peer := hostSession(t, s), peerSession(t, s)
	web := httptest.NewServer(s.Handler())
	defer web.Close()
	wsURL := "ws" + strings.TrimPrefix(web.URL, "http") + "/ws?id=browser-123&name=" + url.QueryEscape("iPhone uji")
	dial := func(cookie *http.Cookie, origin string) (*websocket.Conn, *http.Response, error) {
		headers := http.Header{}
		if cookie != nil {
			headers.Set("Cookie", cookie.String())
		}
		if origin != "" {
			headers.Set("Origin", origin)
		}
		return websocket.DefaultDialer.Dial(wsURL, headers)
	}
	for _, tc := range []struct {
		cookie *http.Cookie
		origin string
		want   int
	}{{nil, web.URL, 401}, {peer, "http://evil.example", 403}, {peer, "", 403}} {
		connection, response, err := dial(tc.cookie, tc.origin)
		if connection != nil {
			connection.Close()
		}
		if response != nil {
			response.Body.Close()
		}
		if err == nil || response == nil || response.StatusCode != tc.want {
			t.Fatalf("unauthorized websocket accepted: %v %+v", err, response)
		}
	}
	connection, _, err := dial(peer, web.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	_ = connection.SetReadDeadline(time.Now().Add(3 * time.Second))
	var event struct {
		Type  string `json:"type"`
		State State  `json:"state"`
	}
	readSocketState(t, connection, &event)
	if event.Type != "state" || event.State.Role != "peer" || len(event.State.Devices) != 1 || event.State.JoinURL != "" {
		t.Fatalf("invalid initial WS state: %+v", event)
	}
	w := request(t, s, http.MethodPost, "/api/text", strings.NewReader(`{"text":"live"}`), peer, false, map[string]string{"X-Device-ID": "browser-123"})
	if w.Code != 201 || !strings.Contains(w.Body.String(), "iPhone uji") {
		t.Fatal("sender did not come from authenticated websocket")
	}
	readSocketState(t, connection, &event)
	if len(event.State.Texts) != 1 {
		t.Fatal("text event missing")
	}
	request(t, s, http.MethodPost, "/api/session/rotate", nil, host, true, nil)
	if len(s.state(true).Devices) != 0 {
		t.Fatal("rotation kept connected devices")
	}
	if _, _, err := connection.ReadMessage(); err == nil {
		t.Fatal("rotation left websocket open")
	}
	peer = peerSession(t, s)
	connection, _, err = dial(peer, web.URL)
	if err != nil {
		t.Fatal(err)
	}
	_ = connection.SetReadDeadline(time.Now().Add(3 * time.Second))
	readSocketState(t, connection, &event)
	_ = connection.Close()
	deadline := time.Now().Add(3 * time.Second)
	for len(s.state(true).Devices) != 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if len(s.state(true).Devices) != 0 {
		t.Fatal("disconnected device remains in state")
	}
}
