package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestReceiptOriginUsesAuthenticatedRoleAndCurrentLiveClient(t *testing.T) {
	s := newTestServer(t)
	host := &wsClient{id: "host-browser", host: true, generation: s.sessionID}
	peer := &wsClient{id: "phone-browser", generation: s.sessionID}
	stale := &wsClient{id: "stale-browser", generation: "expired"}
	s.clients[host] = struct{}{}
	s.clients[peer] = struct{}{}
	s.clients[stale] = struct{}{}
	for _, tc := range []struct {
		name, header, wantID, wantRole string
		host                           bool
	}{
		{"host", host.id, host.id, "host", true},
		{"peer", peer.id, peer.id, "peer", false},
		{"peer cannot claim host origin", host.id, "", "peer", false},
		{"host cannot claim peer origin", peer.id, "", "host", true},
		{"stale client", stale.id, "", "peer", false},
		{"unknown client", "unknown", "", "peer", false},
		{"missing ID", "", "", "peer", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/api/text", nil)
			r.Header.Set("X-Device-ID", tc.header)
			s.mu.Lock()
			id, role := s.receiptOriginLocked(r, principal{host: tc.host, generation: s.sessionID})
			s.mu.Unlock()
			if id != tc.wantID || role != tc.wantRole {
				t.Fatalf("got %q/%q, want %q/%q", id, role, tc.wantID, tc.wantRole)
			}
		})
	}
	delete(s.clients, host)
	delete(s.clients, peer)
	delete(s.clients, stale)
}
