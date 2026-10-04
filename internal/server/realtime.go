package server

import (
	"encoding/json"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type wsClient struct {
	connection   *websocket.Conn
	connectionID string
	remoteIP     string
	requestIP    string
	host         bool
	generation   string
	id           string
	name         string
	connectedAt  time.Time
	queue        chan []byte
	events       chan []byte
	done         chan struct{}
	once         sync.Once
	messageRate  rateBucket
	signalRate   rateBucket
	inputRate    rateBucket
	startRate    rateBucket
}

func (c *wsClient) close() {
	c.once.Do(func() {
		close(c.done)
		if c.connection != nil {
			_ = c.connection.Close()
		}
	})
}

func (c *wsClient) closeAsync() {
	c.once.Do(func() {
		close(c.done)
		if c.connection != nil {
			go c.connection.Close()
		}
	})
}

func validDeviceID(value string) bool {
	if len(value) == 0 || len(value) > 128 {
		return false
	}
	for _, r := range value {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '-' && r != '_' {
			return false
		}
	}
	return true
}

func (s *Server) websocket(w http.ResponseWriter, r *http.Request, p principal) {
	if r.Header.Get("Origin") == "" {
		writeError(w, http.StatusForbidden, "Asal koneksi bobodrop tidak tersedia.")
		return
	}
	id := r.URL.Query().Get("id")
	if !validDeviceID(id) {
		var err error
		id, err = randomID(18)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Koneksi belum dapat dibuat.")
			return
		}
	}
	connectionID, err := randomID(24)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Koneksi belum dapat dibuat.")
		return
	}
	upgrader := websocket.Upgrader{HandshakeTimeout: 10 * time.Second, ReadBufferSize: 1024, WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool { return r.Header.Get("Origin") != "" && sameOrigin(r) },
		Error: func(w http.ResponseWriter, r *http.Request, status int, reason error) {
			writeError(w, status, "Koneksi langsung belum dapat dibuat. Muat ulang bobodrop.")
		}}
	connection, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	c := &wsClient{connection: connection, connectionID: connectionID, host: p.host, generation: p.generation, id: id,
		name: cleanLabel(r.URL.Query().Get("name"), "iPhone"), connectedAt: time.Now().UTC(), queue: make(chan []byte, 1), events: make(chan []byte, mediaQueueCapacity), done: make(chan struct{})}
	c.remoteIP, c.requestIP = socketRequestIPs(r)
	s.mu.Lock()
	if !s.stillAuthorizedLocked(p) {
		s.mu.Unlock()
		c.close()
		return
	}
	s.clients[c] = struct{}{}
	s.enqueueEventLocked(c, struct {
		Type         string `json:"type"`
		ConnectionID string `json:"connectionId"`
	}{"media-ready", connectionID})
	s.mu.Unlock()
	go s.writeSocket(c)
	s.broadcast()
	defer func() {
		c.close()
		s.mu.Lock()
		s.disconnectLocked(c, "Perangkat terputus. Berbagi dihentikan.")
		s.mu.Unlock()
		s.broadcast()
	}()
	connection.SetReadLimit(maxSocketMessageBytes)
	_ = connection.SetReadDeadline(time.Now().Add(60 * time.Second))
	connection.SetPongHandler(func(string) error { return connection.SetReadDeadline(time.Now().Add(60 * time.Second)) })
	for {
		kind, data, err := connection.ReadMessage()
		if err != nil {
			return
		}
		if kind != websocket.TextMessage {
			data = nil
		}
		s.handleSocketMessage(c, data)
	}
}

func socketRequestIPs(r *http.Request) (remoteIP, requestIP string) {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		if ip := net.ParseIP(host); ip != nil {
			remoteIP = ip.String()
		}
	}
	if host, ok := requestHost(r); ok {
		if ip := net.ParseIP(host); ip != nil {
			requestIP = ip.String()
		}
	}
	return remoteIP, requestIP
}

func (s *Server) writeSocket(c *wsClient) {
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	defer c.close()
	write := func(data []byte) bool {
		_ = c.connection.SetWriteDeadline(time.Now().Add(10 * time.Second))
		return c.connection.WriteMessage(websocket.TextMessage, data) == nil
	}
	for {
		select {
		case <-c.done:
			return
		default:
		}
		// Media events retain FIFO order and take priority over replaceable state.
		select {
		case data := <-c.events:
			if !write(data) {
				return
			}
			continue
		default:
		}
		select {
		case <-c.done:
			return
		case data := <-c.queue:
			if !write(data) {
				return
			}
		case data := <-c.events:
			if !write(data) {
				return
			}
		case <-ticker.C:
			if c.connection.WriteControl(websocket.PingMessage, nil, time.Now().Add(10*time.Second)) != nil {
				return
			}
		}
	}
}

func (s *Server) broadcast() {
	// Serializing while locked keeps broadcasts in mutation order. A slow reader
	// receives only the latest snapshot and cannot block other devices.
	s.mu.Lock()
	defer s.mu.Unlock()
	s.broadcastLocked()
}

func (s *Server) broadcastLocked() {
	var hostData, peerData []byte
	for c := range s.clients {
		var data []byte
		if c.host {
			if hostData == nil {
				hostData, _ = json.Marshal(struct {
					Type  string `json:"type"`
					State State  `json:"state"`
				}{"state", s.stateLocked(true)})
			}
			data = hostData
		} else {
			if peerData == nil {
				peerData, _ = json.Marshal(struct {
					Type  string `json:"type"`
					State State  `json:"state"`
				}{"state", s.stateLocked(false)})
			}
			data = peerData
		}
		select {
		case <-c.queue:
		default:
		}
		select {
		case c.queue <- data:
		default:
		}
	}
}
