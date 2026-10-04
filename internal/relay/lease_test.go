package relay

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pion/logging"
	"github.com/pion/turn/v5"
)

func testLease(t *testing.T, lifetime time.Duration) *Lease {
	t.Helper()
	loopback := net.ParseIP("127.0.0.1")
	l, err := New(Config{RelayIP: loopback, HostIP: loopback, PeerIP: loopback, Lifetime: lifetime})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeLease(t, l) })
	return l
}

func closeLease(t *testing.T, l *Lease) {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- l.Close() }()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Close: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("relay cleanup did not complete")
	}
}

type testClient struct {
	client  *turn.Client
	conn    net.Conn
	relay   net.PacketConn
	channel atomic.Bool
}

type recordingConn struct {
	net.Conn
	channel *atomic.Bool
}

func (c *recordingConn) Write(data []byte) (int, error) {
	if len(data) >= 4 && data[0]&0xc0 == 0x40 {
		c.channel.Store(true)
	}
	return c.Conn.Write(data)
}

func openClient(t *testing.T, ice ICEServer, source net.IP) *testClient {
	t.Helper()
	address := strings.TrimSuffix(strings.TrimPrefix(ice.URLs[0], "turn:"), "?transport=tcp")
	dialer := net.Dialer{Timeout: time.Second}
	if source != nil {
		dialer.LocalAddr = &net.TCPAddr{IP: source}
	}
	conn, err := dialer.DialContext(context.Background(), "tcp4", address)
	if err != nil {
		t.Fatal(err)
	}
	result := &testClient{conn: conn}
	client, err := turn.NewClient(&turn.ClientConfig{TURNServerAddr: address,
		Conn: turn.NewSTUNConn(&recordingConn{Conn: conn, channel: &result.channel}), Username: ice.Username, Password: ice.Credential, Realm: realm,
		LoggerFactory: &logging.DefaultLoggerFactory{Writer: io.Discard, DefaultLogLevel: logging.LogLevelDisabled}})
	if err != nil {
		_ = conn.Close()
		t.Fatal(err)
	}
	if err = client.Listen(); err != nil {
		_ = conn.Close()
		t.Fatal(err)
	}
	result.client = client
	t.Cleanup(func() {
		_ = conn.Close()
		client.Close()
		if result.relay != nil {
			_ = result.relay.Close()
		}
	})
	return result
}

func (c *testClient) allocate(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var err error
	c.relay, err = c.client.AllocateWithContext(ctx)
	if err != nil {
		t.Fatal(err)
	}
}

func connectedPair(t *testing.T, l *Lease) (*testClient, *testClient) {
	t.Helper()
	host := openClient(t, l.HostICE(), nil)
	peer := openClient(t, l.PeerICE(), nil)
	host.allocate(t)
	peer.allocate(t)
	if err := host.client.CreatePermission(peer.relay.LocalAddr()); err != nil {
		t.Fatal(err)
	}
	if err := peer.client.CreatePermission(host.relay.LocalAddr()); err != nil {
		t.Fatal(err)
	}
	return host, peer
}

func sendPayload(t *testing.T, from, to *testClient, payload []byte) {
	t.Helper()
	if _, err := from.relay.WriteTo(payload, to.relay.LocalAddr()); err != nil {
		t.Fatal(err)
	}
	if err := to.relay.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	data := make([]byte, 1600)
	n, address, err := to.relay.ReadFrom(data)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data[:n], payload) || address.String() != from.relay.LocalAddr().String() {
		t.Fatal("relay payload or source differs")
	}
}

func TestTCPRelayBothDirectionsAndCleanup(t *testing.T) {
	l := testLease(t, 0)
	host, peer := connectedPair(t, l)
	sendPayload(t, host, peer, []byte("host screen and speaker"))
	sendPayload(t, peer, host, []byte("receiver consent response"))
	// Observe an actual ChannelData frame on the client-to-server TCP leg,
	// rather than assuming the asynchronous channel binding has completed.
	deadline := time.Now().Add(time.Second)
	for !host.channel.Load() && time.Now().Before(deadline) {
		sendPayload(t, host, peer, []byte("bound channel payload"))
		time.Sleep(time.Millisecond)
	}
	if !host.channel.Load() {
		t.Fatal("client did not establish a ChannelData binding")
	}
	if host.relay.LocalAddr().String() == peer.relay.LocalAddr().String() || l.server.AllocationCount() != 2 {
		t.Fatal("expected two distinct allocations on the same relay IP")
	}
	l.Revoke()
	select {
	case <-l.Done():
	default:
		t.Fatal("Revoke must invalidate immediately")
	}
	l.mu.Lock()
	allocation := l.allocations[hostRole]
	l.mu.Unlock()
	if allocation != nil {
		if _, err := allocation.WriteTo([]byte("revoked"), peer.relay.LocalAddr()); !errors.Is(err, net.ErrClosed) {
			t.Fatal("existing allocation accepted traffic after Revoke")
		}
	}
	_, _ = host.relay.WriteTo([]byte("revoked bound channel"), peer.relay.LocalAddr())
	_ = peer.relay.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	if _, _, err := peer.relay.ReadFrom(make([]byte, 1600)); err == nil {
		t.Fatal("established ChannelData delivered after Revoke")
	}
	closeLease(t, l)
	assertEmpty(t, l)
	closeLease(t, l)
	address := strings.TrimSuffix(strings.TrimPrefix(l.HostICE().URLs[0], "turn:"), "?transport=tcp")
	if conn, err := net.DialTimeout("tcp4", address, 100*time.Millisecond); err == nil {
		_ = conn.Close()
		t.Fatal("closed relay still accepts connections")
	}
}

func assertEmpty(t *testing.T, l *Lease) {
	t.Helper()
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.connections) != 0 || l.allocations[0] != nil || l.allocations[1] != nil || l.server.AllocationCount() != 0 {
		t.Fatal("relay resources remain after Close")
	}
}

func TestCredentialsAreUniqueAndReturnedByValue(t *testing.T) {
	l := testLease(t, 0)
	other := testLease(t, 0)
	host, peer := l.HostICE(), l.PeerICE()
	if host.Username == peer.Username || host.Credential == peer.Credential || host.Username == other.HostICE().Username {
		t.Fatal("credentials must be unique per role and lease")
	}
	if !strings.HasPrefix(host.URLs[0], "turn:127.0.0.1:") || !strings.HasSuffix(peer.URLs[0], "?transport=tcp") {
		t.Fatal("expected numeric local TCP TURN URLs")
	}
	host.URLs[0] = "changed"
	if l.HostICE().URLs[0] == "changed" {
		t.Fatal("caller changed lease configuration")
	}
}

func TestWrongCredentialsAndSourceBinding(t *testing.T) {
	for _, bad := range []string{"password", "username", "other lease"} {
		t.Run(bad, func(t *testing.T) {
			l := testLease(t, 0)
			ice := l.HostICE()
			switch bad {
			case "password":
				ice.Credential = "incorrect"
			case "username":
				ice.Username = "unknown"
			case "other lease":
				other := testLease(t, 0)
				ice.Username, ice.Credential = other.HostICE().Username, other.HostICE().Credential
			}
			client := openClient(t, ice, nil)
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			if _, err := client.client.AllocateWithContext(ctx); err == nil {
				t.Fatal("invalid credentials allocated a relay")
			}
			if l.server.AllocationCount() != 0 {
				t.Fatal("invalid credentials left an allocation")
			}
		})
	}
	loopback := net.ParseIP("127.0.0.1")
	l, err := New(Config{RelayIP: loopback, HostIP: loopback, PeerIP: net.ParseIP("127.0.0.2")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeLease(t, l) })
	wrongSource := openClient(t, l.PeerICE(), loopback)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := wrongSource.client.AllocateWithContext(ctx); err == nil {
		t.Fatal("peer credentials worked from the host source IP")
	}
	rightSource := openClient(t, l.PeerICE(), net.ParseIP("127.0.0.2"))
	rightSource.allocate(t)
	if l.server.AllocationCount() != 1 {
		t.Fatal("exact peer source did not allocate")
	}
}

func TestPeerEndpointRestrictions(t *testing.T) {
	l := testLease(t, 0)
	host, peer := connectedPair(t, l)
	l.mu.Lock()
	allocation := l.allocations[hostRole]
	l.mu.Unlock()
	foreign := []*net.UDPAddr{
		{IP: net.ParseIP("8.8.8.8"), Port: 53},
		{IP: net.ParseIP("192.168.29.240"), Port: 1234},
		{IP: net.ParseIP("127.0.0.1"), Port: 9},
		host.relay.LocalAddr().(*net.UDPAddr),
	}
	for _, address := range foreign {
		if _, err := allocation.WriteTo([]byte("blocked"), address); !errors.Is(err, errRestricted) {
			t.Fatal("relay allowed a foreign or same-role destination")
		}
	}
	for _, ip := range []net.IP{net.ParseIP("8.8.8.8"), net.ParseIP("192.168.29.240")} {
		if err := host.client.CreatePermission(&net.UDPAddr{IP: ip, Port: 1234}); err == nil {
			t.Fatal("TURN CreatePermission allowed a foreign IP")
		}
	}
	// Permission is IP-only in TURN. The socket still denies another local port.
	trap, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer trap.Close()
	if _, err = host.relay.WriteTo([]byte("must not reach a local service"), trap.LocalAddr()); err != nil {
		t.Fatal(err)
	}
	_ = trap.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	if _, _, err = trap.ReadFrom(make([]byte, 1600)); err == nil {
		t.Fatal("TURN forwarded to an unrelated laptop port")
	}
	// An unregistered local source also cannot inject into an allocation.
	_, _ = trap.WriteTo([]byte("injected"), peer.relay.LocalAddr())
	_ = peer.relay.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	if _, _, err = peer.relay.ReadFrom(make([]byte, 1600)); err == nil {
		t.Fatal("allocation accepted traffic from an unrelated local port")
	}
	sendPayload(t, host, peer, []byte("valid peer still works"))
}

func TestAllocationQuotaUnderConcurrency(t *testing.T) {
	l := testLease(t, 0)
	clients := []*testClient{
		openClient(t, l.HostICE(), nil), openClient(t, l.HostICE(), nil),
		openClient(t, l.PeerICE(), nil), openClient(t, l.PeerICE(), nil),
	}
	var wg sync.WaitGroup
	results := make(chan bool, len(clients))
	for _, client := range clients {
		wg.Add(1)
		go func(c *testClient) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			var err error
			c.relay, err = c.client.AllocateWithContext(ctx)
			results <- err == nil
		}(client)
	}
	wg.Wait()
	close(results)
	successes := 0
	for success := range results {
		if success {
			successes++
		}
	}
	if successes != 2 || l.server.AllocationCount() != 2 {
		t.Fatalf("expected two allocations, got %d successes and %d allocations", successes, l.server.AllocationCount())
	}
	closeLease(t, l)
	assertEmpty(t, l)
}

func TestExpiryClosesLiveAndUnauthenticatedConnections(t *testing.T) {
	// Allow real TCP setup to finish even while browser QA loads the host.
	l := testLease(t, 3*time.Second)
	host, peer := connectedPair(t, l)
	sendPayload(t, host, peer, []byte("before expiration"))
	_ = openClient(t, l.HostICE(), nil)
	select {
	case <-l.Done():
	case <-time.After(4 * time.Second):
		t.Fatal("finite lease did not expire")
	}
	closeLease(t, l)
	assertEmpty(t, l)
}

func TestImmediateExpiry(t *testing.T) {
	for _, lifetime := range []time.Duration{time.Nanosecond, 50 * time.Nanosecond, time.Microsecond} {
		l := testLease(t, lifetime)
		select {
		case <-l.Done():
		case <-time.After(time.Second):
			t.Fatal("short lease did not expire")
		}
		closeLease(t, l)
		assertEmpty(t, l)
	}
}

func TestRevocationDuringAllocationAndConcurrentClose(t *testing.T) {
	for range 12 {
		l := testLease(t, 0)
		host := openClient(t, l.HostICE(), nil)
		peer := openClient(t, l.PeerICE(), nil)
		var wg sync.WaitGroup
		start := make(chan struct{})
		for _, client := range []*testClient{host, peer} {
			wg.Add(1)
			go func(c *testClient) {
				defer wg.Done()
				<-start
				ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
				defer cancel()
				c.relay, _ = c.client.AllocateWithContext(ctx)
			}(client)
		}
		close(start)
		l.Revoke()
		closed := make(chan error, 3)
		for range 3 {
			go func() { closed <- l.Close() }()
		}
		wg.Wait()
		for range 3 {
			select {
			case err := <-closed:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(time.Second):
				t.Fatal("concurrent Close failed to complete")
			}
		}
		assertEmpty(t, l)
	}
}

func TestRoleConnectionLimitOnSharedSource(t *testing.T) {
	l := testLease(t, 0)
	first := openClient(t, l.HostICE(), nil)
	first.allocate(t)
	second := openClient(t, l.HostICE(), nil)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, _ = second.client.AllocateWithContext(ctx) // Quota rejects, but its role remains bound.
	third := openClient(t, l.HostICE(), nil)
	if _, err := third.client.AllocateWithContext(ctx); err == nil {
		t.Fatal("third socket authenticated for one role")
	}
	peer := openClient(t, l.PeerICE(), nil)
	peer.allocate(t)
	l.mu.Lock()
	counts := [2]int{}
	for _, conn := range l.connections {
		if conn.role >= 0 {
			counts[conn.role]++
		}
	}
	l.mu.Unlock()
	if counts[hostRole] > maxRoleConnections || counts[peerRole] > maxRoleConnections {
		t.Fatal("per-role socket bound exceeded")
	}
}

func TestTCPConnectionAndFrameBounds(t *testing.T) {
	l := testLease(t, 0)
	clients := make([]*testClient, 0, maxConnections)
	for range maxConnections {
		clients = append(clients, openClient(t, l.HostICE(), nil))
	}
	extra := openClient(t, l.PeerICE(), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := extra.client.AllocateWithContext(ctx); err == nil {
		t.Fatal("fifth TCP connection allocated")
	}
	l.mu.Lock()
	count := len(l.connections)
	l.mu.Unlock()
	if count > maxConnections {
		t.Fatal("TCP admission exceeded its bound")
	}
	closeLease(t, l)
	assertEmpty(t, l)
	_ = clients

	for _, bad := range [][]byte{
		{0, 1, 255, 252},  // STUN header plus body would overflow the frame limit.
		{0, 1, 255, 236},  // Exactly 65536 bytes wraps Pion's uint16 total length.
		{64, 1, 255, 255}, // ChannelData padding exceeds the frame limit.
		{64, 1, 255, 252}, // Exactly 65536 bytes also wraps for ChannelData.
		{128, 0, 0, 0},
		{0, 1, 0, 0, 0, 0, 0, 0}, // Invalid STUN magic cookie.
	} {
		var guard frameGuard
		if guard.accept(bad) {
			t.Fatal("invalid frame header accepted")
		}
	}
	stunFrame := make([]byte, 24)
	binary.BigEndian.PutUint16(stunFrame[:2], 1)
	binary.BigEndian.PutUint16(stunFrame[2:4], 4)
	binary.BigEndian.PutUint32(stunFrame[4:8], 0x2112a442)
	channel := []byte{64, 1, 0, 3, 1, 2, 3, 0}
	var guard frameGuard
	for _, value := range append(stunFrame, channel...) {
		if !guard.accept([]byte{value}) {
			t.Fatal("fragmented valid frame rejected")
		}
	}
	if guard.used != 0 || guard.remaining != 0 {
		t.Fatal("valid frames left unfinished parser state")
	}
}

func TestConfigAndDisabledTCPForwarding(t *testing.T) {
	valid := net.ParseIP("127.0.0.1")
	for _, config := range []Config{
		{},
		{RelayIP: net.ParseIP("8.8.8.8"), HostIP: valid, PeerIP: valid},
		{RelayIP: valid, HostIP: valid, PeerIP: net.ParseIP("0.0.0.0")},
		{RelayIP: valid, HostIP: valid, PeerIP: net.ParseIP("::1")},
		{RelayIP: valid, HostIP: valid, PeerIP: valid, Lifetime: -1},
		{RelayIP: valid, HostIP: valid, PeerIP: valid, Lifetime: 11 * time.Minute},
	} {
		if l, err := New(config); err == nil {
			closeLease(t, l)
			t.Fatal("invalid relay configuration accepted")
		}
	}
	l := testLease(t, 0)
	client := openClient(t, l.HostICE(), nil)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := client.client.AllocateTCPWithContext(ctx); err == nil {
		t.Fatal("outgoing TCP peer allocation was enabled")
	}
	if _, err := l.AllocateConn(turn.AllocateConnConfig{}); !errors.Is(err, errRestricted) {
		t.Fatal("outgoing TCP relay connection was enabled")
	}
	if l.server.AllocationCount() != 0 {
		t.Fatal("rejected TCP allocation leaked")
	}

	budget := newByteBudget(1, 8)
	if !budget.allow(8) || budget.allow(1) {
		t.Fatal("traffic burst was not bounded")
	}
}
