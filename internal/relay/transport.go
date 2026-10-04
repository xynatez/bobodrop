package relay

import (
	"encoding/binary"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

type byteBudget struct {
	mu      sync.Mutex
	rate    float64
	burst   float64
	tokens  float64
	updated time.Time
}

func newByteBudget(rate, burst int) byteBudget {
	return byteBudget{rate: float64(rate), burst: float64(burst), tokens: float64(burst), updated: time.Now()}
}

func (b *byteBudget) allow(n int) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	b.tokens = min(b.burst, b.tokens+now.Sub(b.updated).Seconds()*b.rate)
	b.updated = now
	if float64(n) > b.tokens {
		return false
	}
	b.tokens -= float64(n)
	return true
}

type tcpListener struct {
	net.Listener
	lease *Lease
	once  sync.Once
	err   error
}

func (l *tcpListener) Accept() (net.Conn, error) {
	for {
		raw, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		source, ok := raw.RemoteAddr().(*net.TCPAddr)
		role := -1
		if ok && !l.lease.participant[hostRole].Equal(l.lease.participant[peerRole]) {
			for candidate, ip := range l.lease.participant {
				if ip.Equal(source.IP) {
					role = candidate
				}
			}
		}
		l.lease.mu.Lock()
		allowed := ok && (source.IP.Equal(l.lease.participant[hostRole]) || source.IP.Equal(l.lease.participant[peerRole]))
		count := 0
		for _, conn := range l.lease.connections {
			if conn.role == role {
				count++
			}
		}
		if l.lease.revoked.Load() || !allowed || len(l.lease.connections) >= maxConnections || role >= 0 && count >= maxRoleConnections {
			l.lease.mu.Unlock()
			_ = raw.Close()
			if l.lease.revoked.Load() {
				return nil, net.ErrClosed
			}
			continue
		}
		conn := &tcpConn{Conn: raw, lease: l.lease, role: role, done: make(chan struct{}),
			unauthorized: newByteBudget(128<<10, 256<<10)}
		l.lease.connections[source.String()] = conn
		l.lease.mu.Unlock()
		return conn, nil
	}
}

func (l *tcpListener) Close() error {
	l.once.Do(func() {
		l.err = l.Listener.Close()
		if errors.Is(l.err, net.ErrClosed) {
			l.err = nil
		}
	})
	return l.err
}

type tcpConn struct {
	net.Conn
	lease        *Lease
	role         int // guarded by lease.mu; -1 until username binds a shared source IP
	unauthorized byteBudget
	frames       frameGuard // only the single Pion stream reader accesses this
	socketOnce   sync.Once
	doneOnce     sync.Once
	done         chan struct{}
}

func (c *tcpConn) allow(n int) bool {
	if c.lease.revoked.Load() {
		return false
	}
	c.lease.mu.Lock()
	role := c.role
	c.lease.mu.Unlock()
	if role < 0 {
		return c.unauthorized.allow(n)
	}
	return c.lease.traffic[role].allow(n)
}

func (c *tcpConn) Read(data []byte) (int, error) {
	if c.lease.revoked.Load() {
		return 0, net.ErrClosed
	}
	if err := c.Conn.SetReadDeadline(time.Now().Add(idleTimeout)); err != nil {
		return 0, err
	}
	n, err := c.Conn.Read(data)
	if c.lease.revoked.Load() {
		return 0, net.ErrClosed
	}
	if !c.allow(n) {
		return 0, errRate
	}
	if !c.frames.accept(data[:n]) {
		return 0, errFrame
	}
	return n, err
}

func (c *tcpConn) Write(data []byte) (int, error) {
	if c.lease.revoked.Load() {
		return 0, net.ErrClosed
	}
	if len(data) > maxTURNFrame {
		return 0, errFrame
	}
	if !c.allow(len(data)) {
		return 0, errRate
	}
	if err := c.Conn.SetWriteDeadline(time.Now().Add(writeTimeout)); err != nil {
		return 0, err
	}
	return c.Conn.Write(data)
}

func (c *tcpConn) closeSocket() { c.socketOnce.Do(func() { _ = c.Conn.Close() }) }

func (c *tcpConn) Close() error {
	c.closeSocket()
	c.doneOnce.Do(func() {
		c.lease.mu.Lock()
		delete(c.lease.connections, c.RemoteAddr().String())
		c.lease.mu.Unlock()
		close(c.done)
	})
	return nil
}

type packetConn struct {
	*net.UDPConn
	lease      *Lease
	role       int
	traffic    byteBudget
	stopped    atomic.Bool
	socketOnce sync.Once
	doneOnce   sync.Once
	done       chan struct{}
}

func (c *packetConn) permitted(address net.Addr) bool {
	peerAddress, ok := address.(*net.UDPAddr)
	if !ok || c.lease.revoked.Load() || c.stopped.Load() || !peerAddress.IP.Equal(c.lease.relayIP) {
		return false
	}
	c.lease.mu.Lock()
	defer c.lease.mu.Unlock()
	peer := c.lease.allocations[1-c.role]
	return peer != nil && !peer.stopped.Load() && peer.UDPConn.LocalAddr().(*net.UDPAddr).Port == peerAddress.Port
}

func (c *packetConn) ReadFrom(data []byte) (int, net.Addr, error) {
	for {
		if c.lease.revoked.Load() || c.stopped.Load() {
			return 0, nil, net.ErrClosed
		}
		n, address, err := c.UDPConn.ReadFrom(data)
		if err != nil {
			return 0, nil, err
		}
		if c.lease.revoked.Load() || c.stopped.Load() {
			return 0, nil, net.ErrClosed
		}
		if !c.traffic.allow(n) {
			return 0, nil, errRate
		}
		if n <= maxDatagram && c.permitted(address) {
			return n, address, nil
		}
	}
}

func (c *packetConn) WriteTo(data []byte, address net.Addr) (int, error) {
	if c.lease.revoked.Load() || c.stopped.Load() {
		return 0, net.ErrClosed
	}
	if len(data) > maxDatagram {
		return 0, errFrame
	}
	if !c.permitted(address) {
		return 0, errRestricted
	}
	if !c.traffic.allow(len(data)) {
		return 0, errRate
	}
	return c.UDPConn.WriteTo(data, address)
}

func (c *packetConn) closeSocket() {
	c.stopped.Store(true)
	c.socketOnce.Do(func() { _ = c.UDPConn.Close() })
}

func (c *packetConn) Close() error {
	c.closeSocket()
	c.doneOnce.Do(func() {
		c.lease.mu.Lock()
		if c.lease.allocations[c.role] == c {
			c.lease.allocations[c.role] = nil
		}
		c.lease.mu.Unlock()
		close(c.done)
	})
	return nil
}

// frameGuard validates a fragmented TCP stream without buffering its payload.
// This also rejects lengths that would overflow Pion's uint16 frame arithmetic.
type frameGuard struct {
	header    [20]byte
	used      int
	remaining int
}

func (f *frameGuard) accept(data []byte) bool {
	for len(data) != 0 {
		if f.remaining != 0 {
			n := min(f.remaining, len(data))
			f.remaining -= n
			data = data[n:]
			continue
		}
		f.header[f.used] = data[0]
		f.used++
		data = data[1:]
		if f.used < 4 {
			continue
		}
		size := int(binary.BigEndian.Uint16(f.header[2:4]))
		if f.header[0]&0xc0 == 0x40 {
			padded := (size + 3) &^ 3
			if padded+4 >= maxTURNFrame {
				return false
			}
			f.remaining, f.used = padded, 0
			continue
		}
		if f.header[0]&0xc0 != 0 || size%4 != 0 || size+20 >= maxTURNFrame {
			return false
		}
		if f.used == 8 && binary.BigEndian.Uint32(f.header[4:8]) != 0x2112a442 {
			return false
		}
		if f.used == 20 {
			f.remaining, f.used = size, 0
		}
	}
	return true
}
