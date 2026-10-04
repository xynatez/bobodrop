// Package relay provides a TURN/TCP relay restricted to one local media session.
package relay

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pion/logging"
	"github.com/pion/turn/v5"
)

const (
	realm              = "localdrop"
	hostRole           = 0
	peerRole           = 1
	maxConnections     = 4
	maxRoleConnections = 2
	maxTURNFrame       = 65536
	maxDatagram        = 65535
	idleTimeout        = 60 * time.Second
	writeTimeout       = 10 * time.Second
	trafficRate        = 16 << 20
	trafficBurst       = 32 << 20
)

var (
	errRestricted = errors.New("relay endpoint is not permitted")
	errCapacity   = errors.New("relay capacity reached")
	errFrame      = errors.New("invalid or oversized TURN frame")
	errRate       = errors.New("relay traffic limit reached")
)

type Config struct {
	RelayIP  net.IP
	HostIP   net.IP
	PeerIP   net.IP
	Lifetime time.Duration
}

type ICEServer struct {
	URLs       []string `json:"urls"`
	Username   string   `json:"username"`
	Credential string   `json:"credential"`
}

// Lease owns its listener, credentials and both participants' allocations.
// A zero Lifetime keeps credentials valid until the media session revokes them.
type Lease struct {
	relayIP     net.IP
	participant [2]net.IP
	ice         [2]ICEServer
	keys        [2][]byte
	traffic     [2]byteBudget
	mu          sync.Mutex
	connections map[string]*tcpConn
	allocations [2]*packetConn
	listener    *tcpListener
	server      *turn.Server
	timer       *time.Timer
	revoked     atomic.Bool
	done        chan struct{}
	closed      chan struct{}
	closeErr    error
}

func New(config Config) (*Lease, error) {
	relayIP, hostIP, peerIP := ipv4(config.RelayIP), ipv4(config.HostIP), ipv4(config.PeerIP)
	if relayIP == nil || hostIP == nil || peerIP == nil || !assignedIP(relayIP) || !assignedIP(hostIP) {
		return nil, errors.New("relay requires assigned local and participant IPv4 addresses")
	}
	if config.Lifetime < 0 || config.Lifetime > 10*time.Minute {
		return nil, errors.New("relay lifetime is outside the permitted range")
	}
	l := &Lease{relayIP: relayIP, participant: [2]net.IP{hostIP, peerIP},
		connections: make(map[string]*tcpConn), done: make(chan struct{}), closed: make(chan struct{})}
	for role := range l.ice {
		username, err := randomCredential()
		if err != nil {
			return nil, err
		}
		password, err := randomCredential()
		if err != nil {
			return nil, err
		}
		l.ice[role] = ICEServer{Username: username, Credential: password}
		l.keys[role] = turn.GenerateAuthKey(username, realm, password)
		l.traffic[role] = newByteBudget(trafficRate, trafficBurst)
	}
	raw, err := net.Listen("tcp4", "0.0.0.0:0")
	if err != nil {
		return nil, err
	}
	l.listener = &tcpListener{Listener: raw, lease: l}
	port := strconv.Itoa(raw.Addr().(*net.TCPAddr).Port)
	l.ice[hostRole].URLs = []string{"turn:" + net.JoinHostPort("127.0.0.1", port) + "?transport=tcp"}
	l.ice[peerRole].URLs = []string{"turn:" + net.JoinHostPort(relayIP.String(), port) + "?transport=tcp"}
	l.server, err = turn.NewServer(turn.ServerConfig{
		Realm: realm, AuthHandler: l.authenticate, QuotaHandler: l.quota,
		// Discard even dependency diagnostics: invalid usernames must not be logged.
		LoggerFactory: &logging.DefaultLoggerFactory{Writer: io.Discard, DefaultLogLevel: logging.LogLevelDisabled},
		InboundMTU:    maxTURNFrame + 1,
		ListenerConfigs: []turn.ListenerConfig{{Listener: l.listener,
			RelayAddressGenerator: l, PermissionHandler: l.permission}},
	})
	if err != nil {
		_ = raw.Close()
		return nil, err
	}
	if config.Lifetime != 0 {
		l.mu.Lock()
		l.timer = time.AfterFunc(config.Lifetime, l.Revoke)
		l.mu.Unlock()
	}
	return l, nil
}

func randomCredential() (string, error) {
	var value [32]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value[:]), nil
}

func ipv4(ip net.IP) net.IP {
	ip = ip.To4()
	if ip == nil || ip.IsUnspecified() || ip.IsMulticast() || ip.Equal(net.IPv4bcast) || ip[0] == 0 {
		return nil
	}
	return append(net.IP(nil), ip...)
}

func assignedIP(ip net.IP) bool {
	if ip.IsLoopback() {
		return true
	}
	addresses, err := net.InterfaceAddrs()
	if err != nil {
		return false
	}
	for _, address := range addresses {
		if network, ok := address.(*net.IPNet); ok && network.IP.Equal(ip) {
			return true
		}
	}
	return false
}

func (l *Lease) HostICE() ICEServer { return l.iceFor(hostRole) }
func (l *Lease) PeerICE() ICEServer { return l.iceFor(peerRole) }

func (l *Lease) iceFor(role int) ICEServer {
	ice := l.ice[role]
	ice.URLs = append([]string(nil), ice.URLs...)
	return ice
}

// Done closes immediately on revocation or expiration. Close waits for cleanup.
func (l *Lease) Done() <-chan struct{} { return l.done }

// Revoke may run under the application session mutex: it only invalidates the
// lease and schedules cleanup. Every authentication and transport checks it.
func (l *Lease) Revoke() {
	if l.revoked.CompareAndSwap(false, true) {
		close(l.done)
		go l.closeResources()
	}
}

func (l *Lease) Close() error {
	l.Revoke()
	<-l.closed
	return l.closeErr
}

func (l *Lease) closeResources() {
	l.mu.Lock()
	timer := l.timer
	l.mu.Unlock()
	if timer != nil {
		timer.Stop()
	}
	l.closeErr = l.server.Close()
	l.mu.Lock()
	connections := make([]*tcpConn, 0, len(l.connections))
	for _, conn := range l.connections {
		connections = append(connections, conn)
	}
	allocations := l.allocations
	l.mu.Unlock()
	for _, conn := range connections {
		conn.closeSocket()
	}
	for _, conn := range allocations {
		if conn != nil {
			conn.closeSocket()
		}
	}
	// Pion calls these wrappers' Close after exiting its packet loops. Waiting
	// also covers an Allocate request already being handled when Revoke ran.
	for _, conn := range connections {
		<-conn.done
	}
	for _, conn := range allocations {
		if conn != nil {
			<-conn.done
		}
	}
	close(l.closed)
}

func (l *Lease) roleFor(username string) int {
	for role := range l.ice {
		if l.ice[role].Username == username {
			return role
		}
	}
	return -1
}

func (l *Lease) authenticate(request *turn.RequestAttributes) (string, []byte, bool) {
	if l.revoked.Load() || request.Realm != realm {
		return "", nil, false
	}
	role := l.roleFor(request.Username)
	source, ok := request.SrcAddr.(*net.TCPAddr)
	if !ok || role < 0 || !source.IP.Equal(l.participant[role]) {
		return "", nil, false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	conn := l.connections[source.String()]
	if l.revoked.Load() || conn == nil || conn.role >= 0 && conn.role != role {
		return "", nil, false
	}
	if conn.role < 0 {
		count := 0
		for _, active := range l.connections {
			if active.role == role {
				count++
			}
		}
		if count >= maxRoleConnections {
			return "", nil, false
		}
		conn.role = role
	}
	return request.Username, l.keys[role], true
}

func (l *Lease) quota(username, requestRealm string, source net.Addr) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	role := l.roleFor(username)
	conn := l.connections[source.String()]
	return !l.revoked.Load() && requestRealm == realm && role >= 0 && conn != nil &&
		conn.role == role && l.allocations[role] == nil
}

func (l *Lease) permission(source net.Addr, peerIP net.IP) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	conn := l.connections[source.String()]
	if l.revoked.Load() || conn == nil || conn.role < 0 || !peerIP.Equal(l.relayIP) {
		return false
	}
	peer := l.allocations[1-conn.role]
	return peer != nil && !peer.stopped.Load()
}

// Validate and the Allocate methods implement Pion's RelayAddressGenerator.
func (l *Lease) Validate() error { return nil }

func (l *Lease) AllocatePacketConn(config turn.AllocateListenerConfig) (net.PacketConn, net.Addr, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	role := l.roleFor(config.UserID)
	if l.revoked.Load() {
		return nil, nil, net.ErrClosed
	}
	// Refuse EVEN-PORT/reservations and requested ports; every socket is local
	// and OS-selected. The anonymous allocator probes used for EVEN-PORT fail.
	if role < 0 || config.Realm != realm || config.Network != "udp4" || config.RequestedPort != 0 {
		return nil, nil, errRestricted
	}
	if l.allocations[role] != nil {
		return nil, nil, errCapacity
	}
	raw, err := net.ListenUDP("udp4", &net.UDPAddr{IP: l.relayIP})
	if err != nil {
		return nil, nil, err
	}
	conn := &packetConn{UDPConn: raw, lease: l, role: role, done: make(chan struct{}),
		traffic: newByteBudget(trafficRate, trafficBurst)}
	l.allocations[role] = conn
	return conn, raw.LocalAddr(), nil
}

// TURN over TCP carries UDP allocations. RFC 6062 TCP peer forwarding is disabled.
func (l *Lease) AllocateListener(turn.AllocateListenerConfig) (net.Listener, net.Addr, error) {
	return nil, nil, errRestricted
}

func (l *Lease) AllocateConn(turn.AllocateConnConfig) (net.Conn, error) {
	return nil, errRestricted
}
