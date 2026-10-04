package server

import (
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

func TestPairingNavigationBoundary(t *testing.T) {
	s := newTestServer(t)
	path := "/s/" + s.peerSecret
	navigation := map[string]string{"Sec-Fetch-Site": "cross-site", "Sec-Fetch-Mode": "navigate", "Sec-Fetch-Dest": "document"}
	for _, tc := range []struct {
		name, method, path string
		local              bool
		headers            map[string]string
		status             int
	}{
		{"LAN navigation", http.MethodGet, path, false, navigation, 200},
		{"localhost navigation", http.MethodGet, path, true, navigation, 200},
		{"LAN without metadata", http.MethodGet, path, false, nil, 200},
		{"same origin", http.MethodGet, path, true, map[string]string{"Origin": "http://localhost:8787"}, 200},
		{"foreign origin", http.MethodGet, path, false, map[string]string{"Origin": "https://evil.example"}, 403},
		{"null origin", http.MethodGet, path, false, map[string]string{"Origin": "null"}, 403},
		{"origin on navigation", http.MethodGet, path, true, map[string]string{"Sec-Fetch-Site": "cross-site", "Sec-Fetch-Mode": "navigate", "Sec-Fetch-Dest": "document", "Origin": "http://localhost:8787"}, 403},
		{"empty origin on navigation", http.MethodGet, path, false, map[string]string{"Sec-Fetch-Site": "cross-site", "Sec-Fetch-Mode": "navigate", "Sec-Fetch-Dest": "document", "Origin": ""}, 403},
		{"missing mode", http.MethodGet, path, false, map[string]string{"Sec-Fetch-Site": "cross-site", "Sec-Fetch-Dest": "document"}, 403},
		{"missing destination", http.MethodGet, path, false, map[string]string{"Sec-Fetch-Site": "cross-site", "Sec-Fetch-Mode": "navigate"}, 403},
		{"fetch", http.MethodGet, path, false, map[string]string{"Sec-Fetch-Site": "cross-site", "Sec-Fetch-Mode": "cors", "Sec-Fetch-Dest": "empty"}, 403},
		{"iframe", http.MethodGet, path, false, map[string]string{"Sec-Fetch-Site": "cross-site", "Sec-Fetch-Mode": "navigate", "Sec-Fetch-Dest": "iframe"}, 403},
		{"image", http.MethodGet, path, false, map[string]string{"Sec-Fetch-Site": "cross-site", "Sec-Fetch-Mode": "no-cors", "Sec-Fetch-Dest": "image"}, 403},
		{"API", http.MethodGet, "/api/state", false, navigation, 403},
		{"websocket", http.MethodGet, "/ws", false, navigation, 403},
		{"root", http.MethodGet, "/", true, navigation, 403},
		{"outside join prefix", http.MethodGet, "/s", false, navigation, 403},
		{"cross-site POST", http.MethodPost, path, false, navigation, 403},
		{"cross-site HEAD", http.MethodHead, path, false, navigation, 403},
		{"direct POST", http.MethodPost, path, false, nil, 405},
		{"direct HEAD", http.MethodHead, path, false, nil, 405},
		{"stale navigation", http.MethodGet, "/s/stale-token", false, navigation, 401},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := request(t, s, tc.method, tc.path, nil, nil, tc.local, tc.headers)
			if w.Code != tc.status {
				t.Fatalf("got %d, want %d", w.Code, tc.status)
			}
			cookies := w.Result().Cookies()
			if tc.status == 200 {
				if len(cookies) != 1 || cookies[0].Name != peerCookie {
					t.Fatal("pairing must set only the peer cookie")
				}
			} else if len(cookies) != 0 {
				t.Fatal("rejected request set a cookie")
			}
		})
	}
	r := httptest.NewRequest(http.MethodGet, "http://evil.example:8787"+path, nil)
	r.RemoteAddr = "127.0.0.1:12345"
	for name, value := range navigation {
		r.Header.Set(name, value)
	}
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 403 || len(w.Result().Cookies()) != 0 {
		t.Fatal("unknown host can enter pairing")
	}
}

func TestPairingBridgeCookieAndPrivilege(t *testing.T) {
	s := newTestServer(t)
	for _, secure := range []bool{false, true} {
		scheme := "http"
		if secure {
			scheme = "https"
		}
		r := httptest.NewRequest(http.MethodGet, scheme+"://localhost:8787/s/"+s.peerSecret, nil)
		r.RemoteAddr = "127.0.0.1:12345"
		if secure {
			r.TLS = &tls.ConnectionState{}
		}
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		cookies := w.Result().Cookies()
		if w.Code != 200 || len(cookies) != 1 {
			t.Fatal("pairing bridge missing")
		}
		cookie := cookies[0]
		if cookie.Name != peerCookie || cookie.Value != s.peerSecret || !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode || cookie.Path != "/" || cookie.Domain != "" || cookie.Secure != secure {
			t.Fatal("peer cookie protection changed")
		}
		if w.Header().Get("Location") != "" || !strings.Contains(w.Body.String(), `id="pairing-continue" href="/"`) || !strings.Contains(w.Body.String(), `<script>window.location.replace("/");</script>`) {
			t.Fatal("bridge must commit a document with fixed navigation and a real fallback link")
		}
		root := request(t, s, http.MethodGet, "/", nil, cookie, true, nil)
		if root.Code != 200 || root.Body.String() != "app" || len(root.Result().Cookies()) != 0 {
			t.Fatal("paired loopback root did not retain peer authentication")
		}
		state := decodeState(t, request(t, s, http.MethodGet, "/api/state", nil, cookie, true, nil))
		if state.Role != "peer" || state.JoinURL != "" || state.StoragePath != "" {
			t.Fatal("pairing granted host state")
		}
		if request(t, s, http.MethodPost, "/api/session/rotate", nil, cookie, true, nil).Code != 403 {
			t.Fatal("paired loopback peer obtained host privilege")
		}
	}
}

func TestPairingPagesAreSelfContainedAndHashed(t *testing.T) {
	s := newTestServer(t)
	assets := 0
	s.SetAssets(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { assets++ }))
	for _, path := range []string{"/", "/app.js", "/s/stale-token", "/s/" + s.hostSecret, "/s/" + s.peerSecret + "?next=do-not-reflect-me"} {
		w := request(t, s, http.MethodGet, path, nil, nil, false, nil)
		body, policy := w.Body.String(), w.Header().Get("Content-Security-Policy")
		if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Referrer-Policy") != "no-referrer" || w.Header().Get("X-Frame-Options") != "DENY" || w.Header().Get("X-Content-Type-Options") != "nosniff" || w.Header().Get("Content-Type") != "text/html; charset=utf-8" {
			t.Fatal("entry page lost security headers")
		}
		for _, directive := range []string{"default-src 'none'", "frame-ancestors 'none'", "base-uri 'none'", "form-action 'none'"} {
			if !strings.Contains(policy, directive) {
				t.Fatalf("missing CSP directive %s", directive)
			}
		}
		if strings.Contains(policy, "unsafe-inline") || strings.Contains(policy, "'self'") {
			t.Fatal("entry page CSP allows unfixed resources")
		}
		for _, tag := range []string{"script", "style"} {
			blocks := regexp.MustCompile(`(?s)<`+tag+`>(.*?)</`+tag+`>`).FindAllStringSubmatch(body, -1)
			if len(blocks) == 0 {
				t.Fatalf("missing %s", tag)
			}
			for _, block := range blocks {
				hash := sha256.Sum256([]byte(block[1]))
				if !strings.Contains(policy, "'sha256-"+base64.StdEncoding.EncodeToString(hash[:])+"'") {
					t.Fatalf("CSP does not authorize exact %s bytes", tag)
				}
			}
		}
		for _, forbidden := range []string{s.peerSecret, s.hostSecret, "stale-token", "do-not-reflect-me", `src=`, `<link`, `<base`, `http://`, `https://`} {
			if strings.Contains(body, forbidden) {
				t.Fatalf("entry page reflects a secret or uses an external resource: %q", forbidden)
			}
		}
		for _, control := range []string{`id="pairing-language"`, `id="pairing-theme"`, "localdrop-language", "localdrop-theme", `<noscript>`, `lang="en"`} {
			if !strings.Contains(body, control) {
				t.Fatalf("missing entry page preference or fallback: %s", control)
			}
		}
		if w.Code == 401 && (!strings.Contains(body, "Scan QR terbaru") || !strings.Contains(body, "Android, iPhone atau iPad") || !strings.Contains(body, "hotspot")) {
			t.Fatal("recovery page lacks current QR and network guidance")
		}
	}
	if assets != 0 {
		t.Fatal("entry pages accessed protected assets")
	}
}

func TestPairingOnlyAcceptsCurrentGeneration(t *testing.T) {
	s := newTestServer(t)
	host := hostSession(t, s)
	oldToken, oldPeer := s.peerSecret, peerSession(t, s)
	if w := request(t, s, http.MethodPost, "/api/session/rotate", nil, host, true, nil); w.Code != 200 {
		t.Fatal("rotation failed")
	}
	for _, token := range []string{oldToken, "", s.hostSecret, strings.Repeat("x", len(s.peerSecret)), s.peerSecret + "/extra"} {
		w := request(t, s, http.MethodGet, "/s/"+token, nil, nil, false, nil)
		if w.Code != 401 || len(w.Result().Cookies()) != 0 || !strings.Contains(w.Body.String(), `data-pairing="expired"`) || strings.Contains(w.Body.String(), oldToken) {
			t.Fatal("invalid token paired or did not receive useful recovery")
		}
	}
	if request(t, s, http.MethodGet, "/api/state", nil, oldPeer, false, nil).Code != 401 {
		t.Fatal("old generation peer cookie is still authorized")
	}
	if request(t, s, http.MethodGet, "/api/state", nil, peerSession(t, s), false, nil).Code != 200 {
		t.Fatal("current generation cannot pair")
	}
}
