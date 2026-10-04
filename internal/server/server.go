package server

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
)

const (
	hostCookie   = "localdrop_host"
	peerCookie   = "localdrop_peer"
	maxJSONBytes = 1 << 20
)

type NetworkAddress struct {
	Name string `json:"name"`
	IP   string `json:"ip"`
}

type Config struct {
	DeviceName     string
	StorageDir     string
	Port           int
	Addresses      []NetworkAddress
	ClipboardRead  func() (string, error)
	ClipboardWrite func(string) error
	OpenFolder     func() error
	ControlInput   func(RemoteInput) error
	// MediaRelay runs under the session mutex. It must be bounded and must not
	// call back into Server.
	MediaRelay func(MediaRelayRequest) (*MediaRelayLease, error)
	// AudioOutput begins a bounded mute lease; its cleanup restores prior output.
	AudioOutput func() (*AudioOutputLease, error)
	SystemAudio func(context.Context) (*SystemAudioCapture, error)
}

type Text struct {
	ID         string    `json:"id"`
	Text       string    `json:"text"`
	Sender     string    `json:"sender"`
	SenderID   string    `json:"senderId,omitempty"`
	SenderRole string    `json:"senderRole"`
	CreatedAt  time.Time `json:"createdAt"`
}

type File struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Size        int64     `json:"size"`
	MIME        string    `json:"mime"`
	Sender      string    `json:"sender"`
	SenderID    string    `json:"senderId,omitempty"`
	SenderRole  string    `json:"senderRole"`
	CreatedAt   time.Time `json:"createdAt"`
	DownloadURL string    `json:"downloadUrl"`
	path        string
}

type Device struct {
	ID           string    `json:"id"`
	ConnectionID string    `json:"connectionId"`
	Name         string    `json:"name"`
	ConnectedAt  time.Time `json:"connectedAt"`
}

type Settings struct {
	AutoCopy bool `json:"autoCopy"`
}

type ClipboardStatus struct {
	TextID string `json:"textId"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

type addressState struct {
	NetworkAddress
	URL string `json:"url"`
}

type State struct {
	Role        string           `json:"role"`
	DeviceName  string           `json:"deviceName"`
	SessionID   string           `json:"sessionId"`
	JoinURL     string           `json:"joinUrl,omitempty"`
	QRURL       string           `json:"qrUrl,omitempty"`
	Addresses   []addressState   `json:"addresses,omitempty"`
	StoragePath string           `json:"storagePath,omitempty"`
	Texts       []Text           `json:"texts"`
	Files       []File           `json:"files"`
	Devices     []Device         `json:"devices"`
	Settings    Settings         `json:"settings"`
	Clipboard   *ClipboardStatus `json:"clipboard,omitempty"`
	Media       *MediaState      `json:"media"`
	NativeAudio bool             `json:"nativeAudio,omitempty"`
}

type principal struct {
	host        bool
	generation  string
	peerContext context.Context
}

type Server struct {
	config          Config
	allowedHosts    map[string]bool
	mu              sync.Mutex
	clipboardMu     sync.Mutex
	assets          http.Handler
	hostSecret      string
	peerSecret      string
	sessionID       string
	texts           []Text
	files           []File
	settings        Settings
	clipboardStatus *ClipboardStatus
	clients         map[*wsClient]struct{}
	media           *mediaSession
	systemAudio     *systemAudioSource
	ctx             context.Context
	cancel          context.CancelFunc
	peerContext     context.Context
	cancelPeers     context.CancelFunc
	closed          bool
}

func randomID(bytes int) (string, error) {
	b := make([]byte, bytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func New(config Config) (*Server, error) {
	if config.Port < 1 || config.Port > 65535 {
		return nil, errors.New("port bobodrop tidak valid")
	}
	if config.StorageDir == "" {
		return nil, errors.New("folder penyimpanan belum ditentukan")
	}
	dir, err := filepath.Abs(config.StorageDir)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("membuat folder penyimpanan: %w", err)
	}
	config.StorageDir = dir
	config.DeviceName = cleanLabel(config.DeviceName, "Laptop")
	addresses, allowedHosts := normalizeAddresses(config.Addresses)
	config.Addresses = addresses
	hostSecret, err := randomID(32)
	if err != nil {
		return nil, err
	}
	peerSecret, err := randomID(32)
	if err != nil {
		return nil, err
	}
	sessionID, err := randomID(12)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	peerContext, cancelPeers := context.WithCancel(ctx)
	s := &Server{config: config, hostSecret: hostSecret, peerSecret: peerSecret, sessionID: sessionID,
		allowedHosts: allowedHosts,
		clients:      make(map[*wsClient]struct{}), ctx: ctx, cancel: cancel, peerContext: peerContext, cancelPeers: cancelPeers}
	return s, nil
}

func (s *Server) Handler() http.Handler { return http.HandlerFunc(s.serveHTTP) }

func (s *Server) SetAssets(handler http.Handler) {
	s.mu.Lock()
	s.assets = handler
	s.mu.Unlock()
}

func (s *Server) JoinURL() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.joinURLLocked("")
}

func (s *Server) joinURLLocked(ip string) string {
	if ip == "" {
		ip = "localhost"
		if len(s.config.Addresses) > 0 {
			ip = s.config.Addresses[0].IP
		}
	}
	return "http://" + net.JoinHostPort(ip, strconv.Itoa(s.config.Port)) + "/s/" + s.peerSecret
}

func (s *Server) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.stopMediaLocked("bobodrop ditutup. Berbagi dihentikan.")
	s.stopSystemAudioLocked(s.systemAudio)
	s.cancel()
	clients := make([]*wsClient, 0, len(s.clients))
	for c := range s.clients {
		clients = append(clients, c)
	}
	clear(s.clients)
	s.mu.Unlock()
	for _, c := range clients {
		c.close()
	}
	return nil
}

func requestHost(r *http.Request) (string, bool) {
	host := r.Host
	if strings.ContainsAny(host, " /\\@\t\r\n") || host == "" {
		return "", false
	}
	if h, p, err := net.SplitHostPort(host); err == nil {
		port, err := strconv.Atoi(p)
		if err != nil || port < 1 || port > 65535 {
			return "", false
		}
		host = h
	} else if strings.Contains(host, ":") {
		// IPv6 authorities require brackets, even without a port.
		if !strings.HasPrefix(host, "[") || !strings.HasSuffix(host, "]") {
			return "", false
		}
		host = strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")
	}
	if ip := net.ParseIP(host); ip != nil {
		host = ip.String()
	}
	return strings.ToLower(host), true
}

func isLocalHost(r *http.Request) bool {
	host, ok := requestHost(r)
	if !ok {
		return false
	}
	if host != "localhost" {
		ip := net.ParseIP(host)
		if ip == nil || !ip.IsLoopback() {
			return false
		}
	}
	remote, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(remote)
	return ip != nil && ip.IsLoopback()
}

func sameOrigin(r *http.Request) bool {
	if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		return false
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return u.Scheme == scheme && strings.EqualFold(u.Host, r.Host)
}

func pairingNavigation(r *http.Request) bool {
	return r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/s/") &&
		r.Header.Get("Sec-Fetch-Site") == "cross-site" &&
		r.Header.Get("Sec-Fetch-Mode") == "navigate" &&
		r.Header.Get("Sec-Fetch-Dest") == "document" && len(r.Header.Values("Origin")) == 0
}

func validSecret(got, want string) bool {
	return len(got) == len(want) && subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

func (s *Server) authenticate(r *http.Request) (principal, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return principal{}, false
	}
	if cookie, err := r.Cookie(hostCookie); err == nil && isLocalHost(r) && validSecret(cookie.Value, s.hostSecret) {
		return principal{host: true, generation: s.sessionID}, true
	}
	if cookie, err := r.Cookie(peerCookie); err == nil && validSecret(cookie.Value, s.peerSecret) {
		return principal{generation: s.sessionID, peerContext: s.peerContext}, true
	}
	return principal{}, false
}

func sessionCookie(r *http.Request, name, value string) *http.Cookie {
	return &http.Cookie{Name: name, Value: value, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: r.TLS != nil}
}

func (s *Server) serveHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "no-referrer")
	host, ok := requestHost(r)
	s.mu.Lock()
	allowedHost := ok && s.allowedHosts[host]
	s.mu.Unlock()
	if !allowedHost {
		writeError(w, http.StatusForbidden, "Alamat bobodrop tidak dikenali. Buka alamat dari laptop.")
		return
	}
	if !sameOrigin(r) && !pairingNavigation(r) {
		writeError(w, http.StatusForbidden, "Permintaan dari situs lain ditolak.")
		return
	}
	if r.URL.Path == "/api/health" {
		if !method(w, r, http.MethodGet) {
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	s.mu.Lock()
	closed := s.closed
	s.mu.Unlock()
	if closed {
		writeError(w, http.StatusServiceUnavailable, "bobodrop sudah ditutup.")
		return
	}
	if strings.HasPrefix(r.URL.Path, "/s/") {
		s.join(w, r)
		return
	}
	p, authenticated := s.authenticate(r)
	if !authenticated && r.URL.Path == "/" && (r.Method == http.MethodGet || r.Method == http.MethodHead) && isLocalHost(r) {
		s.mu.Lock()
		http.SetCookie(w, sessionCookie(r, hostCookie, s.hostSecret))
		p, authenticated = principal{host: true, generation: s.sessionID}, true
		s.mu.Unlock()
	}
	if !authenticated {
		if strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/ws" {
			writeError(w, http.StatusUnauthorized, "Sesi belum terhubung atau sudah berakhir. Scan QR terbaru di laptop.")
		} else {
			unpairedPage(w)
		}
		return
	}
	switch r.URL.Path {
	case "/api/state":
		if method(w, r, http.MethodGet) {
			writeJSON(w, http.StatusOK, s.state(p.host))
		}
	case "/api/text":
		if method(w, r, http.MethodPost) {
			s.addText(w, r, p)
		}
	case "/api/texts":
		if method(w, r, http.MethodDelete) && requireHost(w, p) {
			s.clearTexts(w)
		}
	case "/api/files":
		if method(w, r, http.MethodPost) {
			s.upload(w, r, p)
		}
	case "/api/qr":
		if method(w, r, http.MethodGet) && requireHost(w, p) {
			s.qr(w, r)
		}
	case "/api/settings":
		if method(w, r, http.MethodPost) && requireHost(w, p) {
			s.setSettings(w, r)
		}
	case "/api/session/rotate":
		if method(w, r, http.MethodPost) && requireHost(w, p) {
			s.rotate(w)
		}
	case "/api/clipboard":
		if method(w, r, http.MethodGet) && requireHost(w, p) {
			s.clipboard(w)
		}
	case "/api/open-folder":
		if method(w, r, http.MethodPost) && requireHost(w, p) {
			s.openFolder(w)
		}
	case "/api/system-audio":
		if method(w, r, http.MethodGet) && requireHost(w, p) {
			s.systemAudioSocket(w, r, p)
		}
	case "/ws":
		if method(w, r, http.MethodGet) {
			s.websocket(w, r, p)
		}
	default:
		if strings.HasPrefix(r.URL.Path, "/api/files/") {
			if method(w, r, http.MethodGet, http.MethodHead) {
				s.download(w, r)
			}
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			writeError(w, http.StatusNotFound, "Halaman bobodrop tidak ditemukan.")
			return
		}
		s.mu.Lock()
		assets := s.assets
		s.mu.Unlock()
		if assets == nil {
			http.NotFound(w, r)
			return
		}
		assets.ServeHTTP(w, r)
	}
}

func (s *Server) join(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodGet) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	token := strings.TrimPrefix(r.URL.Path, "/s/")
	if !validSecret(token, s.peerSecret) {
		expiredPairingPage(w)
		return
	}
	http.SetCookie(w, sessionCookie(r, peerCookie, s.peerSecret))
	pairedPage(w)
}

func (s *Server) state(host bool) State {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stateLocked(host)
}

func (s *Server) stateLocked(host bool) State {
	state := State{Role: "peer", DeviceName: s.config.DeviceName, SessionID: s.sessionID,
		Texts: append([]Text{}, s.texts...), Files: append([]File{}, s.files...), Devices: []Device{}, Settings: s.settings}
	for c := range s.clients {
		if c.host {
			continue
		}
		state.Devices = append(state.Devices, Device{ID: c.id, ConnectionID: c.connectionID, Name: c.name, ConnectedAt: c.connectedAt})
	}
	if s.media != nil {
		state.Media = s.media.snapshot()
	}
	sort.Slice(state.Devices, func(i, j int) bool { return state.Devices[i].ConnectedAt.After(state.Devices[j].ConnectedAt) })
	if host {
		state.Role = "host"
		state.NativeAudio = s.config.SystemAudio != nil
		if s.clipboardStatus != nil {
			status := *s.clipboardStatus
			state.Clipboard = &status
		}
		state.JoinURL = s.joinURLLocked("")
		state.QRURL = "/api/qr"
		state.StoragePath = s.config.StorageDir
		for _, a := range s.config.Addresses {
			state.Addresses = append(state.Addresses, addressState{NetworkAddress: a, URL: s.joinURLLocked(a.IP)})
		}
	}
	return state
}

func (s *Server) senderLocked(r *http.Request, p principal) string {
	if p.host {
		return "Laptop"
	}
	id := r.Header.Get("X-Device-ID")
	for c := range s.clients {
		if !c.host && c.id == id && c.generation == p.generation {
			return c.name
		}
	}
	return "Perangkat"
}

func (s *Server) receiptOriginLocked(r *http.Request, p principal) (string, string) {
	role := "peer"
	if p.host {
		role = "host"
	}
	id := r.Header.Get("X-Device-ID")
	for c := range s.clients {
		if c.host == p.host && c.id == id && c.generation == p.generation {
			return c.id, role
		}
	}
	return "", role
}

func (s *Server) stillAuthorizedLocked(p principal) bool {
	return !s.closed && (p.host || p.generation == s.sessionID)
}

func (s *Server) addText(w http.ResponseWriter, r *http.Request, p principal) {
	var body struct {
		Text string `json:"text"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if strings.TrimSpace(body.Text) == "" {
		writeError(w, http.StatusBadRequest, "Isi teks sebelum mengirim.")
		return
	}
	id, err := randomID(18)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Teks belum dapat dikirim.")
		return
	}
	// Keep receipt order, native writes and clipboard reads together without
	// holding the session mutex while Windows retries a busy clipboard.
	s.clipboardMu.Lock()
	defer s.clipboardMu.Unlock()
	s.mu.Lock()
	if !s.stillAuthorizedLocked(p) {
		s.mu.Unlock()
		writeError(w, http.StatusUnauthorized, "Sesi sudah berakhir. Scan QR terbaru di laptop.")
		return
	}
	entry := Text{ID: id, Text: body.Text, Sender: s.senderLocked(r, p), CreatedAt: time.Now().UTC()}
	entry.SenderID, entry.SenderRole = s.receiptOriginLocked(r, p)
	autoCopy := !p.host && s.settings.AutoCopy
	s.mu.Unlock()
	clipboardError := ""
	clipboardCopied := false
	if autoCopy {
		if s.config.ClipboardWrite == nil {
			clipboardError = "Teks diterima, tetapi clipboard Windows tidak tersedia."
		} else if err := s.config.ClipboardWrite(body.Text); err != nil {
			clipboardError = "Teks diterima, tetapi belum dapat disalin ke clipboard Windows: " + err.Error()
		} else {
			clipboardCopied = true
		}
	}
	s.mu.Lock()
	s.texts = append([]Text{entry}, s.texts...)
	if len(s.texts) > 100 {
		s.texts = s.texts[:100]
	}
	if autoCopy {
		status := "copied"
		if !clipboardCopied {
			status = "error"
		}
		s.clipboardStatus = &ClipboardStatus{TextID: entry.ID, Status: status, Error: clipboardError}
	}
	s.broadcastLocked()
	s.mu.Unlock()
	writeJSON(w, http.StatusCreated, struct {
		Text
		ClipboardCopied bool   `json:"clipboardCopied,omitempty"`
		ClipboardError  string `json:"clipboardError,omitempty"`
	}{Text: entry, ClipboardCopied: clipboardCopied, ClipboardError: clipboardError})
}

func (s *Server) clearTexts(w http.ResponseWriter) {
	s.clipboardMu.Lock()
	defer s.clipboardMu.Unlock()
	s.mu.Lock()
	s.texts = nil
	s.clipboardStatus = nil
	s.mu.Unlock()
	s.broadcast()
	writeJSON(w, http.StatusOK, s.state(true))
}

func (s *Server) setSettings(w http.ResponseWriter, r *http.Request) {
	var body struct {
		AutoCopy *bool `json:"autoCopy"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.AutoCopy == nil {
		writeError(w, http.StatusBadRequest, "Pilih pengaturan salin otomatis.")
		return
	}
	s.clipboardMu.Lock()
	defer s.clipboardMu.Unlock()
	s.mu.Lock()
	s.settings.AutoCopy = *body.AutoCopy
	if !s.settings.AutoCopy {
		s.clipboardStatus = nil
	}
	s.mu.Unlock()
	s.broadcast()
	writeJSON(w, http.StatusOK, s.state(true))
}

func (s *Server) rotate(w http.ResponseWriter) {
	token, err := randomID(32)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Sesi baru belum dapat dibuat.")
		return
	}
	id, err := randomID(12)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Sesi baru belum dapat dibuat.")
		return
	}
	s.mu.Lock()
	s.stopMediaLocked("Sesi diperbarui. Berbagi dihentikan.")
	s.stopSystemAudioLocked(s.systemAudio)
	s.cancelPeers()
	s.peerContext, s.cancelPeers = context.WithCancel(s.ctx)
	s.peerSecret, s.sessionID = token, id
	clients := []*wsClient{}
	for c := range s.clients {
		if !c.host {
			clients = append(clients, c)
			delete(s.clients, c)
		}
	}
	s.mu.Unlock()
	for _, c := range clients {
		c.close()
	}
	s.broadcast()
	writeJSON(w, http.StatusOK, s.state(true))
}

func (s *Server) clipboard(w http.ResponseWriter) {
	s.clipboardMu.Lock()
	defer s.clipboardMu.Unlock()
	if s.config.ClipboardRead == nil {
		writeError(w, http.StatusServiceUnavailable, "Clipboard Windows tidak tersedia.")
		return
	}
	value, err := s.config.ClipboardRead()
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "Clipboard Windows belum dapat dibaca: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"text": value})
}

func (s *Server) openFolder(w http.ResponseWriter) {
	if s.config.OpenFolder == nil {
		writeError(w, http.StatusServiceUnavailable, "Folder belum dapat dibuka dari perangkat ini.")
		return
	}
	if err := s.config.OpenFolder(); err != nil {
		writeError(w, http.StatusInternalServerError, "Folder belum dapat dibuka: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func cleanLabel(value, fallback string) string {
	runes := []rune(strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, value)))
	if len(runes) > 60 {
		runes = runes[:60]
	}
	if len(runes) == 0 {
		return fallback
	}
	return string(runes)
}

func method(w http.ResponseWriter, r *http.Request, allowed ...string) bool {
	for _, m := range allowed {
		if r.Method == m {
			return true
		}
	}
	w.Header().Set("Allow", strings.Join(allowed, ", "))
	writeError(w, http.StatusMethodNotAllowed, "Tindakan ini tidak tersedia untuk permintaan tersebut.")
	return false
}

func requireHost(w http.ResponseWriter, p principal) bool {
	if p.host {
		return true
	}
	writeError(w, http.StatusForbidden, "Tindakan ini hanya tersedia di laptop.")
	return false
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	err := decoder.Decode(target)
	if err == nil {
		var extra any
		if nextErr := decoder.Decode(&extra); nextErr != io.EOF {
			err = nextErr
			if err == nil {
				err = errors.New("unexpected JSON data")
			}
		}
	}
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(w, http.StatusRequestEntityTooLarge, "Teks terlalu panjang. Batasnya 1 MiB.")
		} else {
			writeError(w, http.StatusBadRequest, "Data permintaan tidak valid.")
		}
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
