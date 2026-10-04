package server

import (
	"context"
	"io"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

// SystemAudioCapture supplies stereo 48 kHz little-endian PCM16. Close must
// stop capture and unblock PCM.Read; it runs outside the session mutex.
type SystemAudioCapture struct {
	PCM        io.ReadCloser
	SampleRate int
	Channels   int
	Close      func() error
	Done       <-chan struct{}
}

const (
	systemAudioStartTimeout  = 6 * time.Second
	systemAudioAttachTimeout = 30 * time.Second
	systemAudioWriteTimeout  = 3 * time.Second
	systemAudioChunkBytes    = 16 << 10
	systemAudioFrameBytes    = 4

	systemAudioUnavailableError = "Audio Windows tidak tersedia di laptop ini."
	systemAudioStartError       = "Audio Windows belum dapat diambil. Gunakan Windows build 20348 atau lebih baru, lalu mulai berbagi lagi."
	systemAudioTimeoutError     = "Audio Windows terlalu lama disiapkan. Mulai berbagi lagi."
	systemAudioFormatError      = "Format audio Windows tidak didukung. Mulai berbagi lagi."
	systemAudioInterruptedError = "Suara Windows terputus. Berbagi dihentikan."
	systemAudioNotReadyError    = "Sumber audio Windows belum siap untuk perangkat ini. Mulai berbagi lagi."
	systemAudioRequiredError    = "Suara khusus HP memerlukan sumber audio Windows. Mulai berbagi lagi."
	systemAudioBusyError        = "Audio Windows sedang digunakan. Hentikan kiriman sebelumnya."
	systemAudioExpiredError     = "Sumber audio Windows kedaluwarsa. Mulai berbagi lagi."
)

type systemAudioSource struct {
	owner, viewer *wsClient
	generation    string
	ctx           context.Context
	cancel        context.CancelFunc
	connection    *websocket.Conn
	capture       *SystemAudioCapture
	ready         bool
	attached      chan struct{}
}

type systemAudioResult struct {
	capture *SystemAudioCapture
	err     error
}

func closeSystemAudioCapture(capture *SystemAudioCapture) {
	if capture == nil {
		return
	}
	if capture.PCM != nil {
		_ = capture.PCM.Close()
	}
	if capture.Close != nil {
		_ = capture.Close()
	}
}

func (s *Server) systemAudioAuthorizedLocked(source *systemAudioSource) bool {
	if source == nil || s.systemAudio != source || source.generation != s.sessionID || !s.clientAuthorizedLocked(source.owner) || !s.clientAuthorizedLocked(source.viewer) {
		return false
	}
	select {
	case <-source.ctx.Done():
		return false
	default:
	}
	if source.capture != nil {
		select {
		case <-source.capture.Done:
			return false
		default:
		}
	}
	return true
}

func (s *Server) stopSystemAudioLocked(source *systemAudioSource) {
	if source == nil || s.systemAudio != source {
		return
	}
	s.systemAudio = nil
	source.cancel()
	connection, capture := source.connection, source.capture
	go func() {
		s.mu.Lock()
		s.mu.Unlock()
		if connection != nil {
			_ = connection.Close()
		}
		closeSystemAudioCapture(capture)
	}()
}

func (s *Server) failSystemAudio(source *systemAudioSource, message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.systemAudio != source {
		return
	}
	if s.media != nil && s.media.systemAudio == source {
		s.stopMediaLocked(message)
	} else {
		s.stopSystemAudioLocked(source)
	}
	if s.clientAuthorizedLocked(source.owner) {
		s.mediaErrorLocked(source.owner, message)
	}
}

// The unbuffered handoff leaves the factory responsible for any late result.
func (s *Server) beginSystemAudio(source *systemAudioSource, timeout time.Duration) string {
	result := make(chan systemAudioResult)
	factory := s.config.SystemAudio
	go func() {
		capture, err := factory(source.ctx)
		select {
		case result <- systemAudioResult{capture, err}:
		case <-source.ctx.Done():
			closeSystemAudioCapture(capture)
		}
	}()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	var returned systemAudioResult
	select {
	case returned = <-result:
	case <-source.ctx.Done():
		return ""
	case <-timer.C:
		source.cancel()
		return systemAudioTimeoutError
	}
	capture := returned.capture
	message := ""
	if returned.err != nil || capture == nil {
		message = systemAudioStartError
	} else if capture.PCM == nil || capture.Close == nil || capture.Done == nil || capture.SampleRate != 48000 || capture.Channels != 2 {
		message = systemAudioFormatError
	}
	s.mu.Lock()
	authorized := s.systemAudioAuthorizedLocked(source)
	if authorized && message == "" {
		select {
		case <-capture.Done:
			message = systemAudioInterruptedError
		default:
			source.capture, source.ready = capture, true
		}
	}
	s.mu.Unlock()
	if !authorized || message != "" {
		go closeSystemAudioCapture(capture)
	}
	if !authorized {
		return ""
	}
	return message
}

func (s *Server) systemAudioSocket(w http.ResponseWriter, r *http.Request, p principal) {
	if !p.host || !isLocalHost(r) || r.Header.Get("Origin") == "" || !sameOrigin(r) {
		writeError(w, http.StatusForbidden, "Asal koneksi bobodrop tidak tersedia.")
		return
	}
	query := r.URL.Query()
	if len(query) != 2 || len(query["connectionId"]) != 1 || len(query["target"]) != 1 || !validDeviceID(query.Get("connectionId")) || !validDeviceID(query.Get("target")) {
		writeError(w, http.StatusBadRequest, systemAudioNotReadyError)
		return
	}
	s.mu.Lock()
	if s.config.SystemAudio == nil {
		s.mu.Unlock()
		writeError(w, http.StatusServiceUnavailable, systemAudioUnavailableError)
		return
	}
	var owner, viewer *wsClient
	for client := range s.clients {
		if !s.clientAuthorizedLocked(client) {
			continue
		}
		if client.host && client.connectionID == query.Get("connectionId") {
			owner = client
		}
		if !client.host && client.connectionID == query.Get("target") {
			viewer = client
		}
	}
	if owner == nil || viewer == nil || p.generation != s.sessionID {
		s.mu.Unlock()
		writeError(w, http.StatusForbidden, systemAudioNotReadyError)
		return
	}
	if s.systemAudio != nil || s.media != nil {
		s.mu.Unlock()
		writeError(w, http.StatusConflict, systemAudioBusyError)
		return
	}
	ctx, cancel := context.WithCancel(s.ctx)
	source := &systemAudioSource{owner: owner, viewer: viewer, generation: s.sessionID, ctx: ctx, cancel: cancel, attached: make(chan struct{})}
	s.systemAudio = source
	s.mu.Unlock()
	defer s.failSystemAudio(source, systemAudioInterruptedError)
	upgrader := websocket.Upgrader{HandshakeTimeout: systemAudioWriteTimeout, ReadBufferSize: 1024, WriteBufferSize: systemAudioChunkBytes,
		CheckOrigin: func(r *http.Request) bool { return r.Header.Get("Origin") != "" && sameOrigin(r) },
		Error: func(w http.ResponseWriter, r *http.Request, status int, reason error) {
			writeError(w, status, "Koneksi langsung belum dapat dibuat. Muat ulang bobodrop.")
		}}
	connection, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer connection.Close()
	s.mu.Lock()
	if !s.systemAudioAuthorizedLocked(source) {
		s.mu.Unlock()
		return
	}
	source.connection = connection
	s.mu.Unlock()
	go s.readSystemAudioSocket(source)
	if message := s.beginSystemAudio(source, systemAudioStartTimeout); message != "" {
		_ = connection.SetWriteDeadline(time.Now().Add(systemAudioWriteTimeout))
		_ = connection.WriteJSON(struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		}{"error", message})
		s.failSystemAudio(source, message)
		return
	}
	s.mu.Lock()
	ready := s.systemAudioAuthorizedLocked(source) && source.ready
	capture := source.capture
	s.mu.Unlock()
	if !ready {
		return
	}
	_ = connection.SetWriteDeadline(time.Now().Add(systemAudioWriteTimeout))
	if connection.WriteJSON(struct {
		Type       string `json:"type"`
		SampleRate int    `json:"sampleRate"`
		Channels   int    `json:"channels"`
		SampleSize int    `json:"sampleSize"`
	}{"ready", 48000, 2, 16}) != nil {
		s.failSystemAudio(source, systemAudioInterruptedError)
		return
	}
	s.streamSystemAudio(source, capture, systemAudioAttachTimeout)
}

func (s *Server) readSystemAudioSocket(source *systemAudioSource) {
	connection := source.connection
	connection.SetReadLimit(1024)
	_ = connection.SetReadDeadline(time.Now().Add(60 * time.Second))
	connection.SetPongHandler(func(string) error { return connection.SetReadDeadline(time.Now().Add(60 * time.Second)) })
	// This socket carries server PCM only. Browser close and protocol errors
	// revoke capture even while the native factory is still starting.
	_, _, _ = connection.ReadMessage()
	s.failSystemAudio(source, systemAudioInterruptedError)
}

type systemAudioFrame struct {
	data []byte
	err  error
}

func readSystemAudioPCM(ctx context.Context, pcm io.Reader, frames chan<- systemAudioFrame) {
	buffer := make([]byte, systemAudioChunkBytes)
	carry, emptyReads := 0, 0
	for {
		n, err := pcm.Read(buffer[carry:])
		if n == 0 && err == nil {
			emptyReads++
			if emptyReads < 100 {
				select {
				case <-ctx.Done():
					return
				default:
					continue
				}
			}
			err = io.ErrNoProgress
		} else {
			emptyReads = 0
		}
		total := carry + n
		aligned := total - total%systemAudioFrameBytes
		if aligned > 0 {
			frame := systemAudioFrame{data: append([]byte(nil), buffer[:aligned]...)}
			select {
			case frames <- frame:
			case <-ctx.Done():
				return
			}
		}
		carry = copy(buffer, buffer[aligned:total])
		if err != nil {
			if carry != 0 {
				err = io.ErrUnexpectedEOF
			}
			select {
			case frames <- systemAudioFrame{err: err}:
			case <-ctx.Done():
			}
			return
		}
	}
}

func (s *Server) streamSystemAudio(source *systemAudioSource, capture *SystemAudioCapture, attachTimeout time.Duration) {
	frames := make(chan systemAudioFrame, 1)
	go readSystemAudioPCM(source.ctx, capture.PCM, frames)
	timer := time.NewTimer(attachTimeout)
	defer timer.Stop()
	attachDeadline, attached := timer.C, source.attached
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-source.ctx.Done():
			return
		case <-capture.Done:
			s.failSystemAudio(source, systemAudioInterruptedError)
			return
		case <-attached:
			timer.Stop()
			attachDeadline, attached = nil, nil
		case <-attachDeadline:
			s.mu.Lock()
			active := s.media != nil && s.media.systemAudio == source
			s.mu.Unlock()
			if !active {
				s.failSystemAudio(source, systemAudioExpiredError)
				return
			}
			attachDeadline = nil
		case <-ticker.C:
			if source.connection.WriteControl(websocket.PingMessage, nil, time.Now().Add(systemAudioWriteTimeout)) != nil {
				s.failSystemAudio(source, systemAudioInterruptedError)
				return
			}
		case frame := <-frames:
			if frame.err != nil {
				s.failSystemAudio(source, systemAudioInterruptedError)
				return
			}
			_ = source.connection.SetWriteDeadline(time.Now().Add(systemAudioWriteTimeout))
			if source.connection.WriteMessage(websocket.BinaryMessage, frame.data) != nil {
				s.failSystemAudio(source, systemAudioInterruptedError)
				return
			}
		}
	}
}
