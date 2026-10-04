package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	mediaQueueCapacity    = 64
	maxSocketMessageBytes = 96 << 10
	maxSDPBytes           = 64 << 10
)

type MediaICEServer struct {
	URLs       []string `json:"urls"`
	Username   string   `json:"username"`
	Credential string   `json:"credential"`
}

type MediaRelayRequest struct {
	ServerIP string
	HostIP   string
	ViewerIP string
}

type MediaRelayLease struct {
	HostICE   []MediaICEServer
	ViewerICE []MediaICEServer
	// Revoke immediately disables traffic and must not block or call Server.
	Revoke func()
	// Close completes cleanup after the session mutex is released.
	Close func() error
}

type RemoteInput struct {
	Kind   string `json:"kind"`
	DX     int    `json:"dx"`
	DY     int    `json:"dy"`
	Button string `json:"button"`
	Key    string `json:"key"`
	Text   string `json:"text"`
}

type MediaState struct {
	ID          string `json:"id"`
	Mode        string `json:"mode"`
	Audio       bool   `json:"audio"`
	PhoneOnly   bool   `json:"phoneOnly"`
	NativeAudio bool   `json:"nativeAudio,omitempty"`
	Control     bool   `json:"control"`
	ViewerID    string `json:"viewerId"`
	ViewerName  string `json:"viewerName"`
}

type mediaSession struct {
	id            string
	mode          string
	audio         bool
	phoneOnly     bool
	pending       bool
	startCanceled chan struct{}
	output        *AudioOutputLease
	systemAudio   *systemAudioSource
	control       bool
	owner         *wsClient
	viewer        *wsClient
	offered       bool
	answered      bool
	relay         *MediaRelayLease
}

func (m *mediaSession) snapshot() *MediaState {
	if m.pending {
		return nil
	}
	return &MediaState{ID: m.id, Mode: m.mode, Audio: m.audio, PhoneOnly: m.phoneOnly, NativeAudio: m.systemAudio != nil, Control: m.control, ViewerID: m.viewer.connectionID, ViewerName: m.viewer.name}
}

type rateBucket struct {
	tokens  float64
	updated time.Time
}

func (b *rateBucket) allow(now time.Time, rate, burst float64) bool {
	if b.updated.IsZero() {
		b.tokens = burst
	} else {
		elapsed := now.Sub(b.updated).Seconds()
		if elapsed > 0 {
			b.tokens += elapsed * rate
		}
		if b.tokens > burst {
			b.tokens = burst
		}
	}
	b.updated = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

func decodePacket(data []byte, target any) bool {
	if len(data) == 0 || len(data) > maxSocketMessageBytes || !utf8.Valid(data) {
		return false
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if d.Decode(target) != nil {
		return false
	}
	var extra any
	return d.Decode(&extra) == io.EOF
}

func (s *Server) clientAuthorizedLocked(c *wsClient) bool {
	if s.closed {
		return false
	}
	if _, exists := s.clients[c]; !exists {
		return false
	}
	select {
	case <-c.done:
		return false
	default:
	}
	return c.host || c.generation == s.sessionID
}

func (s *Server) enqueueEventLocked(c *wsClient, event any) bool {
	if _, active := s.clients[c]; !active {
		return false
	}
	select {
	case <-c.done:
		return false
	default:
	}
	data, err := json.Marshal(event)
	if err != nil {
		return false
	}
	select {
	case c.events <- data:
		return true
	default:
		s.disconnectLocked(c, "Koneksi terlalu lambat. Berbagi dihentikan.")
		return false
	}
}

func (s *Server) mediaErrorLocked(c *wsClient, message string) {
	s.enqueueEventLocked(c, struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	}{"media-error", message})
}

func (s *Server) disconnectLocked(c *wsClient, reason string) {
	if _, active := s.clients[c]; !active {
		return
	}
	delete(s.clients, c)
	c.closeAsync()
	if source := s.systemAudio; source != nil && (source.owner == c || source.viewer == c) {
		s.stopSystemAudioLocked(source)
	}
	if s.media != nil && (s.media.owner == c || s.media.viewer == c) {
		s.stopMediaLocked(reason)
	}
	s.broadcastLocked()
}

func (s *Server) stopMediaLocked(reason string) {
	m := s.media
	if m == nil {
		return
	}
	s.media = nil
	s.stopSystemAudioLocked(m.systemAudio)
	if m.startCanceled != nil {
		close(m.startCanceled)
	}
	s.releaseAudioOutputLocked(m.output)
	s.releaseMediaRelayLocked(m.relay)
	// Replace queued active state before the stop acknowledgement. Any state
	// already being written completes before the sequential writer sends it.
	s.broadcastLocked()
	if m.pending {
		return
	}
	event := struct {
		Type   string `json:"type"`
		ID     string `json:"id"`
		Reason string `json:"reason"`
	}{"media-stopped", m.id, reason}
	s.enqueueEventLocked(m.owner, event)
	s.enqueueEventLocked(m.viewer, event)
}

func (s *Server) releaseMediaRelayLocked(lease *MediaRelayLease) {
	if lease == nil {
		return
	}
	if lease.Revoke != nil {
		lease.Revoke()
	}
	if lease.Close != nil {
		closeRelay := lease.Close
		go func() {
			// Wait for the critical section that revoked the lease to finish.
			// Full provider cleanup may block or call Server after this barrier.
			s.mu.Lock()
			s.mu.Unlock()
			_ = closeRelay()
		}()
	}
}

func (s *Server) mediaRelayRequestLocked(owner, viewer *wsClient) (MediaRelayRequest, bool) {
	hostIP, viewerIP, serverIP := net.ParseIP(owner.remoteIP), net.ParseIP(viewer.remoteIP), net.ParseIP(viewer.requestIP)
	if hostIP == nil || hostIP.To4() == nil || !hostIP.IsLoopback() || viewerIP == nil || viewerIP.To4() == nil || (!viewerIP.IsGlobalUnicast() && !viewerIP.IsLoopback()) || serverIP == nil || serverIP.To4() == nil {
		return MediaRelayRequest{}, false
	}
	serverAddress := serverIP.To4().String()
	if !s.allowedHosts[serverAddress] {
		return MediaRelayRequest{}, false
	}
	for _, address := range s.config.Addresses {
		if address.IP == serverAddress {
			return MediaRelayRequest{ServerIP: serverAddress, HostIP: hostIP.To4().String(), ViewerIP: viewerIP.To4().String()}, true
		}
	}
	return MediaRelayRequest{}, false
}

func (s *Server) handleSocketMessage(c *wsClient, data []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.clientAuthorizedLocked(c) {
		return
	}
	if !c.messageRate.allow(time.Now(), 120, 180) {
		s.mediaErrorLocked(c, "Terlalu banyak permintaan. Sambungkan ulang bobodrop.")
		s.disconnectLocked(c, "Terlalu banyak permintaan. Berbagi dihentikan.")
		return
	}
	var envelope struct {
		Type string `json:"type"`
	}
	if len(data) > maxSocketMessageBytes || !utf8.Valid(data) || json.Unmarshal(data, &envelope) != nil {
		s.mediaErrorLocked(c, "Data koneksi tidak valid.")
		return
	}
	switch envelope.Type {
	case "hello":
		var packet struct {
			Type string `json:"type"`
			ID   string `json:"id"`
			Name string `json:"name"`
		}
		if !decodePacket(data, &packet) {
			s.mediaErrorLocked(c, "Data perangkat tidak valid.")
			return
		}
		if c.host {
			return
		}
		if validDeviceID(packet.ID) {
			c.id = packet.ID
		}
		if packet.Name != "" {
			c.name = cleanLabel(packet.Name, "iPhone")
		}
		s.broadcastLocked()
	case "media-start":
		s.startMediaLocked(c, data)
	case "media-signal":
		s.signalMediaLocked(c, data)
	case "media-stop":
		var packet struct {
			Type string `json:"type"`
			ID   string `json:"id"`
		}
		if !decodePacket(data, &packet) || !s.mediaParticipantLocked(c, packet.ID) {
			s.mediaErrorLocked(c, "Sesi berbagi tidak tersedia untuk perangkat ini.")
			return
		}
		s.stopMediaLocked("Berbagi dihentikan.")
	case "media-input":
		s.inputMediaLocked(c, data)
	default:
		s.mediaErrorLocked(c, "Tindakan koneksi tidak dikenali.")
	}
}

func (s *Server) mediaParticipantLocked(c *wsClient, id string) bool {
	return s.media != nil && !s.media.pending && id == s.media.id && (s.media.owner == c || s.media.viewer == c)
}

func (s *Server) startMediaLocked(c *wsClient, data []byte) {
	var packet struct {
		Type        string `json:"type"`
		Target      string `json:"target"`
		Mode        string `json:"mode"`
		Audio       bool   `json:"audio"`
		PhoneOnly   bool   `json:"phoneOnly"`
		NativeAudio bool   `json:"nativeAudio"`
		Control     bool   `json:"control"`
	}
	if !decodePacket(data, &packet) {
		s.mediaErrorLocked(c, "Pilihan berbagi tidak valid.")
		return
	}
	if !c.host {
		s.mediaErrorLocked(c, "Berbagi hanya dapat dimulai dari laptop.")
		return
	}
	if !c.startRate.allow(time.Now(), 2, 4) {
		s.mediaErrorLocked(c, "Tunggu sebentar sebelum memulai berbagi lagi.")
		return
	}
	if s.media != nil {
		s.mediaErrorLocked(c, "Laptop sedang berbagi. Hentikan sesi yang aktif terlebih dahulu.")
		return
	}
	if packet.Mode != "audio" && packet.Mode != "screen" || packet.Mode == "audio" && (!packet.Audio || packet.Control) {
		s.mediaErrorLocked(c, "Pilih suara saja, atau layar dengan izin kontrol terpisah.")
		return
	}
	if packet.Control && s.config.ControlInput == nil {
		s.mediaErrorLocked(c, "Kontrol mouse dan keyboard tidak tersedia di laptop ini.")
		return
	}
	if packet.PhoneOnly && (!packet.Audio || s.config.AudioOutput == nil) {
		s.mediaErrorLocked(c, "Suara khusus HP memerlukan audio sistem dan dukungan Windows.")
		return
	}
	if packet.PhoneOnly && s.config.SystemAudio != nil && !packet.NativeAudio {
		s.mediaErrorLocked(c, systemAudioRequiredError)
		return
	}
	var viewer *wsClient
	for candidate := range s.clients {
		if !candidate.host && candidate.connectionID == packet.Target && s.clientAuthorizedLocked(candidate) {
			viewer = candidate
			break
		}
	}
	if viewer == nil {
		s.mediaErrorLocked(c, "Perangkat penerima sudah terputus. Pilih perangkat yang terhubung.")
		return
	}
	var source *systemAudioSource
	if packet.NativeAudio {
		source = s.systemAudio
		if !packet.Audio || !s.systemAudioAuthorizedLocked(source) || !source.ready || source.owner != c || source.viewer != viewer {
			s.mediaErrorLocked(c, systemAudioNotReadyError)
			return
		}
	} else if s.systemAudio != nil {
		s.mediaErrorLocked(c, systemAudioBusyError)
		return
	}
	id, err := randomID(24)
	if err != nil {
		s.mediaErrorLocked(c, "Sesi berbagi belum dapat dibuat.")
		return
	}
	var lease *MediaRelayLease
	if s.config.MediaRelay != nil {
		request, ok := s.mediaRelayRequestLocked(c, viewer)
		if !ok {
			s.mediaErrorLocked(c, "Alamat penerima berubah. Sambungkan ulang dari kode QR laptop.")
			return
		}
		lease, err = s.config.MediaRelay(request)
		if err != nil || lease == nil {
			s.releaseMediaRelayLocked(lease)
			s.mediaErrorLocked(c, "Jalur koneksi lokal belum dapat dibuat. Mulai berbagi lagi.")
			return
		}
	}
	m := &mediaSession{id: id, mode: packet.Mode, audio: packet.Audio, control: packet.Control, owner: c, viewer: viewer, relay: lease, systemAudio: source, startCanceled: make(chan struct{}), pending: packet.PhoneOnly}
	s.media = m
	if source != nil {
		close(source.attached)
	}
	if packet.PhoneOnly && !s.beginAudioOutputLocked(m, audioOutputStartTimeout) {
		return
	}
	m.pending = false
	// Publish the new snapshot before acknowledgements become visible to the
	// writer; an older idle snapshot must not arrive after media-started.
	s.broadcastLocked()
	var hostICE, viewerICE []MediaICEServer
	if lease != nil {
		hostICE, viewerICE = lease.HostICE, lease.ViewerICE
	}
	if !s.enqueueEventLocked(c, struct {
		Type        string           `json:"type"`
		ID          string           `json:"id"`
		Target      string           `json:"target"`
		Mode        string           `json:"mode"`
		Audio       bool             `json:"audio"`
		PhoneOnly   bool             `json:"phoneOnly"`
		NativeAudio bool             `json:"nativeAudio,omitempty"`
		Control     bool             `json:"control"`
		ICEServers  []MediaICEServer `json:"iceServers,omitempty"`
	}{"media-started", id, viewer.connectionID, m.mode, m.audio, m.phoneOnly, m.systemAudio != nil, m.control, hostICE}) {
		if s.media == m {
			s.stopMediaLocked("Koneksi terputus. Berbagi dihentikan.")
		}
		return
	}
	if s.media != m {
		return
	}
	if !s.enqueueEventLocked(viewer, struct {
		Type        string           `json:"type"`
		ID          string           `json:"id"`
		Mode        string           `json:"mode"`
		Audio       bool             `json:"audio"`
		PhoneOnly   bool             `json:"phoneOnly"`
		NativeAudio bool             `json:"nativeAudio,omitempty"`
		Control     bool             `json:"control"`
		ICEServers  []MediaICEServer `json:"iceServers,omitempty"`
	}{"media-invite", id, m.mode, m.audio, m.phoneOnly, m.systemAudio != nil, m.control, viewerICE}) {
		if s.media == m {
			s.stopMediaLocked("Koneksi terputus. Berbagi dihentikan.")
		}
		return
	}
}

func (s *Server) signalMediaLocked(c *wsClient, data []byte) {
	var packet struct {
		Type string          `json:"type"`
		ID   string          `json:"id"`
		Kind string          `json:"kind"`
		Data json.RawMessage `json:"data"`
	}
	if !decodePacket(data, &packet) || !s.mediaParticipantLocked(c, packet.ID) {
		s.mediaErrorLocked(c, "Sinyal tidak sesuai dengan sesi berbagi perangkat ini.")
		return
	}
	if !c.signalRate.allow(time.Now(), 50, 80) {
		s.mediaErrorLocked(c, "Terlalu banyak sinyal koneksi. Tunggu sebentar.")
		return
	}
	m := s.media
	var recipient *wsClient
	if c == m.owner {
		recipient = m.viewer
	} else {
		recipient = m.owner
	}
	if !s.clientAuthorizedLocked(recipient) {
		s.stopMediaLocked("Perangkat terputus. Berbagi dihentikan.")
		return
	}
	switch packet.Kind {
	case "offer":
		if c != m.owner || m.offered || !validSDP(packet.Data, "offer", m.mode) {
			s.mediaErrorLocked(c, "Penawaran koneksi layar atau suara tidak valid.")
			return
		}
		m.offered = true
	case "answer":
		if c != m.viewer || !m.offered || m.answered || !validSDP(packet.Data, "answer", m.mode) {
			s.mediaErrorLocked(c, "Jawaban koneksi layar atau suara tidak valid.")
			return
		}
		m.answered = true
	case "ice":
		if !validICE(packet.Data) {
			s.mediaErrorLocked(c, "Alamat koneksi layar atau suara tidak valid.")
			return
		}
	default:
		s.mediaErrorLocked(c, "Jenis sinyal koneksi tidak dikenali.")
		return
	}
	s.enqueueEventLocked(recipient, packet)
}

func validSDP(data []byte, kind, mode string) bool {
	if len(data) > 80<<10 {
		return false
	}
	var description struct {
		Type string `json:"type"`
		SDP  string `json:"sdp"`
	}
	if !decodePacket(data, &description) || description.Type != kind || len(description.SDP) == 0 || len(description.SDP) > maxSDPBytes || !utf8.ValidString(description.SDP) {
		return false
	}
	for _, r := range description.SDP {
		if unicode.IsControl(r) && r != '\r' && r != '\n' && r != '\t' {
			return false
		}
	}
	lines := strings.Split(strings.ReplaceAll(description.SDP, "\r\n", "\n"), "\n")
	if len(lines) > 2048 || lines[0] != "v=0" {
		return false
	}
	var origin, session, timing, media bool
	for _, line := range lines {
		if line == "" {
			continue
		}
		if len(line) < 2 || len(line) > 4096 || line[1] != '=' || strings.ContainsRune(line, '\r') {
			return false
		}
		switch line[0] {
		case 'o':
			origin = len(line) > 2
		case 's':
			session = len(line) > 2
		case 't':
			timing = len(line) > 2
		case 'm':
			media = len(line) > 2
		}
		if mode == "audio" && strings.HasPrefix(line, "m=video") {
			return false
		}
		if strings.HasPrefix(line, "a=candidate:") && !validCandidate(strings.TrimPrefix(line, "a=")) {
			return false
		}
	}
	return origin && session && timing && media
}

func validICE(data []byte) bool {
	if len(data) > 4096 {
		return false
	}
	var candidate struct {
		Candidate        *string `json:"candidate"`
		SDPMid           *string `json:"sdpMid"`
		SDPMLineIndex    *int    `json:"sdpMLineIndex"`
		UsernameFragment *string `json:"usernameFragment"`
	}
	if !decodePacket(data, &candidate) || candidate.Candidate == nil || !validCandidate(*candidate.Candidate) {
		return false
	}
	if candidate.SDPMLineIndex != nil && (*candidate.SDPMLineIndex < 0 || *candidate.SDPMLineIndex > 255) {
		return false
	}
	for _, value := range []*string{candidate.SDPMid, candidate.UsernameFragment} {
		if value == nil {
			continue
		}
		if len(*value) > 256 {
			return false
		}
		for _, r := range *value {
			if r < 33 || r > 126 {
				return false
			}
		}
	}
	return true
}

func validCandidate(candidate string) bool {
	if candidate == "" {
		return true
	}
	if len(candidate) > 2048 {
		return false
	}
	for _, r := range candidate {
		if r < 32 || r > 126 {
			return false
		}
	}
	fields := strings.Fields(candidate)
	if len(fields) < 8 || !strings.HasPrefix(fields[0], "candidate:") || len(strings.TrimPrefix(fields[0], "candidate:")) == 0 || len(fields[0]) > 128 {
		return false
	}
	if fields[1] != "1" && fields[1] != "2" {
		return false
	}
	if !strings.EqualFold(fields[2], "udp") && !strings.EqualFold(fields[2], "tcp") {
		return false
	}
	if _, err := strconv.ParseUint(fields[3], 10, 32); err != nil {
		return false
	}
	if !candidateAddress(fields[4]) {
		return false
	}
	port, err := strconv.ParseUint(fields[5], 10, 16)
	if err != nil || port == 0 || fields[6] != "typ" {
		return false
	}
	if fields[7] != "host" && fields[7] != "srflx" && fields[7] != "prflx" && fields[7] != "relay" {
		return false
	}
	if (len(fields)-8)%2 != 0 {
		return false
	}
	var relatedAddressSeen, relatedPortSeen, redactedRelatedAddress bool
	var relatedPort uint64
	for i := 8; i < len(fields); i += 2 {
		switch fields[i] {
		case "raddr":
			if relatedAddressSeen {
				return false
			}
			relatedAddressSeen = true
			if !candidateAddress(fields[i+1]) {
				ip := net.ParseIP(fields[i+1])
				if fields[7] != "relay" || ip == nil || !ip.IsUnspecified() {
					return false
				}
				redactedRelatedAddress = true
			}
		case "rport":
			if relatedPortSeen {
				return false
			}
			relatedPortSeen = true
			var err error
			relatedPort, err = strconv.ParseUint(fields[i+1], 10, 16)
			if err != nil {
				return false
			}
		case "tcptype":
			if fields[i+1] != "active" && fields[i+1] != "passive" && fields[i+1] != "so" {
				return false
			}
		case "generation", "network-id", "network-cost":
			if _, err := strconv.ParseUint(fields[i+1], 10, 32); err != nil {
				return false
			}
		case "ufrag":
			if len(fields[i+1]) > 256 {
				return false
			}
		default:
			return false
		}
	}
	// RFC 8839 uses port 9 for a privacy-redacted related address. Chromium
	// also emits port 0 for relay candidates; neither is the primary endpoint.
	if redactedRelatedAddress && (!relatedPortSeen || relatedPort != 0 && relatedPort != 9) {
		return false
	}
	return true
}

func candidateAddress(address string) bool {
	if ip := net.ParseIP(address); ip != nil {
		return !ip.IsUnspecified() && !ip.IsMulticast()
	}
	if len(address) > 253 || !strings.HasSuffix(strings.ToLower(address), ".local") {
		return false
	}
	for _, label := range strings.Split(address, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, r := range label {
			if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '-' {
				return false
			}
		}
	}
	return true
}

func validateRemoteInput(input RemoteInput) error {
	invalid := errors.New("Kontrol mouse atau keyboard tidak valid.")
	switch input.Kind {
	case "move":
		if input.DX < -500 || input.DX > 500 || input.DY < -500 || input.DY > 500 || input.Button != "" || input.Key != "" || input.Text != "" {
			return invalid
		}
	case "click":
		if input.DX != 0 || input.DY != 0 || input.Key != "" || input.Text != "" || input.Button != "left" && input.Button != "right" && input.Button != "middle" {
			return invalid
		}
	case "scroll":
		if input.DX != 0 || input.DY < -1200 || input.DY > 1200 || input.Button != "" || input.Key != "" || input.Text != "" {
			return invalid
		}
	case "key":
		if input.DX != 0 || input.DY != 0 || input.Button != "" || input.Text != "" {
			return invalid
		}
		switch input.Key {
		case "enter", "tab", "backspace", "escape", "arrowup", "arrowdown", "arrowleft", "arrowright", "delete", "home", "end", "pageup", "pagedown", "ctrl+a", "ctrl+c", "ctrl+v", "ctrl+z":
		default:
			return invalid
		}
	case "text":
		if input.DX != 0 || input.DY != 0 || input.Button != "" || input.Key != "" || !utf8.ValidString(input.Text) || utf8.RuneCountInString(input.Text) == 0 || utf8.RuneCountInString(input.Text) > 256 {
			return invalid
		}
		for _, r := range input.Text {
			if unicode.IsControl(r) {
				return invalid
			}
		}
	default:
		return invalid
	}
	return nil
}

func (s *Server) inputMediaLocked(c *wsClient, data []byte) {
	var packet struct {
		Type  string       `json:"type"`
		ID    string       `json:"id"`
		Input *RemoteInput `json:"input"`
	}
	if !decodePacket(data, &packet) || packet.Input == nil || s.media == nil || packet.ID != s.media.id || s.media.viewer != c || s.media.mode != "screen" || !s.media.control || s.config.ControlInput == nil {
		s.mediaErrorLocked(c, "Kontrol belum diizinkan untuk perangkat dan sesi ini.")
		return
	}
	if !s.clientAuthorizedLocked(s.media.owner) {
		s.stopMediaLocked("Laptop terputus. Berbagi dihentikan.")
		return
	}
	if err := validateRemoteInput(*packet.Input); err != nil {
		s.mediaErrorLocked(c, err.Error())
		return
	}
	if !c.inputRate.allow(time.Now(), 60, 90) {
		s.mediaErrorLocked(c, "Kontrol terlalu cepat. Tunggu sebentar.")
		return
	}
	// The native callback must be synchronous and bounded. Holding the same
	// mutex as stop/rotation prevents dispatch after permission is revoked.
	if err := s.config.ControlInput(*packet.Input); err != nil {
		s.mediaErrorLocked(c, "Kontrol belum dapat dikirim ke Windows: "+err.Error())
	}
}
