package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	qrcode "github.com/skip2/go-qrcode"
)

func addressRequest(t *testing.T, s *Server, ip, method, path string, body io.Reader, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, "http://"+net.JoinHostPort(ip, "8787")+path, body)
	r.RemoteAddr = "10.5.200.55:12345"
	if cookie != nil {
		r.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	return w
}

func TestUpdateAddressesChangesAllowedHostsAndPreservesSession(t *testing.T) {
	const oldIP, newIP = "10.5.60.112", "10.5.200.28"
	s := newTestServer(t)
	host, peer := hostSession(t, s), peerSession(t, s)
	if w := request(t, s, http.MethodPost, "/api/text", strings.NewReader(`{"text":"tetap tersimpan 🦷"}`), peer, false, nil); w.Code != 201 {
		t.Fatal(w.Body.String())
	}
	file := uploadTestFile(t, s, peer, "alamat berubah.txt", []byte("transfer tersimpan"))
	s.UpdateAddresses([]NetworkAddress{{Name: "Wi-Fi", IP: oldIP}})
	before := decodeState(t, request(t, s, http.MethodGet, "/api/state", nil, host, true, nil))
	beforeURL, err := url.Parse(before.JoinURL)
	if err != nil {
		t.Fatal(err)
	}
	if w := addressRequest(t, s, oldIP, http.MethodGet, "/api/health", nil, nil); w.Code != 200 {
		t.Fatalf("old address should work before refresh: %d", w.Code)
	}
	if w := addressRequest(t, s, newIP, http.MethodGet, "/api/health", nil, nil); w.Code != 403 {
		t.Fatalf("new address should be denied before refresh: %d", w.Code)
	}
	addresses := []NetworkAddress{{Name: " Wi-Fi\r\n", IP: newIP}, {Name: "duplicate", IP: "::ffff:" + newIP}, {Name: "invalid", IP: "evil.example"}}
	s.UpdateAddresses(addresses)
	// Caller-owned slices cannot subsequently change the allowed address snapshot.
	addresses[0].IP = "evil.example"
	addresses[0].Name = "changed by caller"
	after := decodeState(t, request(t, s, http.MethodGet, "/api/state", nil, host, true, nil))
	afterURL, err := url.Parse(after.JoinURL)
	if err != nil {
		t.Fatal(err)
	}
	if after.Role != "host" || after.SessionID != before.SessionID || beforeURL.Path != afterURL.Path {
		t.Fatal("address refresh rotated the session or invalidated the host cookie")
	}
	if afterURL.Host != newIP+":8787" || after.JoinURL != s.JoinURL() || len(after.Addresses) != 1 || after.Addresses[0].IP != newIP || after.Addresses[0].Name != "Wi-Fi" || after.Addresses[0].URL != after.JoinURL {
		t.Fatalf("new join URL/address snapshot disagree: %+v", after)
	}
	if !reflect.DeepEqual(before.Texts, after.Texts) || !reflect.DeepEqual(before.Files, after.Files) {
		t.Fatal("address refresh changed transfer history")
	}
	if w := addressRequest(t, s, oldIP, http.MethodGet, "/api/health", nil, nil); w.Code != 403 {
		t.Fatalf("removed address still allowed: %d", w.Code)
	}
	if w := addressRequest(t, s, newIP, http.MethodGet, "/api/state", nil, peer); w.Code != 200 {
		t.Fatalf("existing peer cookie no longer works at new address: %d", w.Code)
	}
	if w := addressRequest(t, s, newIP, http.MethodGet, beforeURL.Path, nil, nil); w.Code != http.StatusOK || w.Header().Get("Location") != "" || len(w.Result().Cookies()) != 1 {
		t.Fatalf("existing join token no longer pairs at new address: %d", w.Code)
	}
	if w := addressRequest(t, s, newIP, http.MethodGet, file.DownloadURL, nil, peer); w.Code != 200 || w.Body.String() != "transfer tersimpan" {
		t.Fatal("completed file disappeared after address refresh")
	}
	// Compare the entire PNG with the deterministic encoder output for the new
	// URL, proving that the default and selected QR encode that exact payload.
	wantPNG, err := qrcode.Encode(after.JoinURL, qrcode.Medium, 320)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/qr", "/api/qr?ip=" + newIP} {
		w := request(t, s, http.MethodGet, path, nil, host, true, nil)
		if w.Code != 200 || !bytes.Equal(w.Body.Bytes(), wantPNG) {
			t.Fatalf("QR payload does not match the current join URL: %s", path)
		}
	}
	if w := request(t, s, http.MethodGet, "/api/qr?ip="+oldIP, nil, host, true, nil); w.Code != 400 {
		t.Fatal("QR still accepts the removed address")
	}
	s.UpdateAddresses(nil)
	fallback := s.state(true)
	if len(fallback.Addresses) != 0 || fallback.JoinURL != "http://localhost:8787"+afterURL.Path || fallback.SessionID != after.SessionID {
		t.Fatal("no-interface fallback changed the session or kept a stale LAN address")
	}
	for _, ip := range []string{"localhost", "127.0.0.1", "::1"} {
		if w := addressRequest(t, s, ip, http.MethodGet, "/api/health", nil, nil); w.Code != 200 {
			t.Fatalf("loopback address removed from allowed hosts: %s", ip)
		}
	}
	if w := addressRequest(t, s, newIP, http.MethodGet, "/api/health", nil, nil); w.Code != 403 {
		t.Fatal("empty snapshot kept a stale LAN address")
	}
}

func TestUpdateAddressesNormalizesAndSkipsUnchangedBroadcast(t *testing.T) {
	s := newTestServer(t)
	c := &wsClient{host: true, queue: make(chan []byte, 1)}
	s.mu.Lock()
	s.clients[c] = struct{}{}
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.clients, c)
		s.mu.Unlock()
	}()
	invalidIPs := []string{"evil.example", "https://192.168.1.25", "192.168.1.25:8787", "user@192.168.1.25", "192.168.1.25/evil", "192.168.1.25\\evil", "192.168.1.25\r\n", "localhost", "0.0.0.0", "255.255.255.255", "224.0.0.1", "127.0.0.2", "169.254.2.3", "2001:db8::1"}
	input := []NetworkAddress{{Name: " Wi-Fi\n", IP: "192.168.1.25"}, {Name: "duplicate", IP: "::ffff:192.168.1.25"}}
	for _, ip := range invalidIPs {
		input = append(input, NetworkAddress{Name: "invalid", IP: ip})
	}
	s.UpdateAddresses(input)
	select {
	case <-c.queue:
		t.Fatal("unchanged normalized snapshot emitted a redundant broadcast")
	default:
	}
	for _, ip := range invalidIPs {
		// These are invalid LAN snapshot values but valid request authorities
		// for hosts already allowed independently of the snapshot.
		if ip == "localhost" || ip == "192.168.1.25:8787" {
			continue
		}
		r := httptest.NewRequest(http.MethodGet, "http://localhost:8787/api/health", nil)
		r.Host = ip
		r.RemoteAddr = "10.5.200.55:12345"
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		if w.Code != 403 {
			t.Errorf("malicious/unsupported host allowed: %q", ip)
		}
	}
	s.UpdateAddresses([]NetworkAddress{{Name: "Wi-Fi baru", IP: "192.168.1.25"}})
	select {
	case data := <-c.queue:
		var event struct {
			Type  string `json:"type"`
			State State  `json:"state"`
		}
		if err := json.Unmarshal(data, &event); err != nil || event.Type != "state" || event.State.Addresses[0].Name != "Wi-Fi baru" {
			t.Fatal("changed interface name was not broadcast")
		}
	default:
		t.Fatal("changed snapshot did not emit a state broadcast")
	}
	s.UpdateAddresses([]NetworkAddress{{Name: "Wi-Fi baru", IP: "192.168.1.25"}})
	select {
	case <-c.queue:
		t.Fatal("repeated address snapshot emitted a redundant broadcast")
	default:
	}
}

func TestUpdateAddressesBroadcastsToLiveHostWebSocket(t *testing.T) {
	s := newTestServer(t)
	host := hostSession(t, s)
	web := httptest.NewServer(s.Handler())
	defer web.Close()
	headers := http.Header{"Cookie": []string{host.String()}, "Origin": []string{web.URL}}
	connection, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(web.URL, "http")+"/ws?id=host-browser", headers)
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
	if event.State.Role != "host" {
		t.Fatal("host socket did not receive initial state")
	}
	sessionID := event.State.SessionID
	s.UpdateAddresses([]NetworkAddress{{Name: "Wi-Fi", IP: "10.5.200.28"}})
	readSocketState(t, connection, &event)
	if event.Type != "state" || event.State.Role != "host" || event.State.SessionID != sessionID || event.State.JoinURL != s.JoinURL() || len(event.State.Addresses) != 1 || event.State.Addresses[0].IP != "10.5.200.28" {
		t.Fatalf("live host did not receive the refreshed address: %+v", event)
	}
}

func TestUpdateAddressesConcurrentWithRequests(t *testing.T) {
	s := newTestServer(t)
	host := hostSession(t, s)
	var workers sync.WaitGroup
	workers.Add(3)
	go func() {
		defer workers.Done()
		for i := 0; i < 200; i++ {
			ip := "192.168.1.25"
			if i%2 == 1 {
				ip = "10.5.200.28"
			}
			s.UpdateAddresses([]NetworkAddress{{Name: "Wi-Fi", IP: ip}})
		}
	}()
	go func() {
		defer workers.Done()
		for i := 0; i < 200; i++ {
			for _, ip := range []string{"192.168.1.25", "10.5.200.28"} {
				if w := addressRequest(t, s, ip, http.MethodGet, "/api/health", nil, nil); w.Code != 200 && w.Code != 403 {
					t.Errorf("concurrent address validation returned %d", w.Code)
				}
			}
			state := s.state(true)
			if len(state.Addresses) != 1 || state.JoinURL != state.Addresses[0].URL {
				t.Error("concurrent state exposed inconsistent address snapshots")
			}
			_ = s.JoinURL()
		}
	}()
	go func() {
		defer workers.Done()
		for i := 0; i < 50; i++ {
			if w := request(t, s, http.MethodGet, "/api/qr", nil, host, true, nil); w.Code != 200 {
				t.Errorf("concurrent QR request returned %d", w.Code)
			}
		}
	}()
	workers.Wait()
}
