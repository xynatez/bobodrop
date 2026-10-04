package server

import (
	"errors"
	"log"
	"time"
)

// AudioOutputLease owns temporary endpoint mute state. Revoke must only signal
// cleanup and must not block or call Server; Close restores outside its mutex.
type AudioOutputLease struct {
	Revoke func()
	Close  func() error
	Done   <-chan struct{}
}

const audioOutputStartTimeout = 6 * time.Second

type audioOutputResult struct {
	lease *AudioOutputLease
	err   error
}

func (s *Server) releaseAudioOutputLocked(lease *AudioOutputLease) {
	if lease == nil {
		return
	}
	if lease.Revoke != nil {
		lease.Revoke()
	}
	if lease.Close != nil {
		go func() {
			s.mu.Lock()
			s.mu.Unlock()
			if err := lease.Close(); err != nil {
				const warning = "Windows belum mengonfirmasi pemulihan suara. Periksa pengaturan mute Windows sebelum berbagi lagi."
				log.Print("bobodrop: audio output restoration is pending or failed")
				s.mu.Lock()
				defer s.mu.Unlock()
				for client := range s.clients {
					if client.host && s.clientAuthorizedLocked(client) {
						s.enqueueEventLocked(client, struct {
							Type    string `json:"type"`
							Message string `json:"message"`
						}{"media-output-warning", warning})
					}
				}
			}
		}()
	}
}

// The callback runs without the session mutex. A canceled or timed-out start
// still owns any late result until it has been revoked and restored.
func (s *Server) beginAudioOutputLocked(m *mediaSession, timeout time.Duration) bool {
	result := make(chan audioOutputResult)
	canceled := make(chan struct{})
	factory := s.config.AudioOutput
	go func() {
		lease, err := factory()
		select {
		case result <- audioOutputResult{lease, err}:
		case <-canceled:
			s.mu.Lock()
			s.releaseAudioOutputLocked(lease)
			s.mu.Unlock()
		}
	}()
	s.mu.Unlock()
	timer := time.NewTimer(timeout)
	var returned audioOutputResult
	select {
	case returned = <-result:
	case <-m.startCanceled:
		returned.err = errors.New("media start canceled")
	case <-timer.C:
		returned.err = errors.New("audio output start timed out")
	}
	timer.Stop()
	close(canceled)
	s.mu.Lock()
	lease := returned.lease
	if s.media != m || !s.clientAuthorizedLocked(m.owner) || !s.clientAuthorizedLocked(m.viewer) {
		s.releaseAudioOutputLocked(lease)
		return false
	}
	if returned.err != nil || lease == nil || lease.Revoke == nil || lease.Close == nil {
		s.releaseAudioOutputLocked(lease)
		s.stopMediaLocked("Suara khusus HP belum dapat diaktifkan.")
		s.mediaErrorLocked(m.owner, "Speaker laptop belum dapat dibisukan. Mulai berbagi lagi.")
		return false
	}
	if lease.Done != nil {
		select {
		case <-lease.Done:
			s.releaseAudioOutputLocked(lease)
			s.stopMediaLocked("Suara khusus HP belum dapat diaktifkan.")
			s.mediaErrorLocked(m.owner, "Speaker laptop belum dapat dibisukan. Mulai berbagi lagi.")
			return false
		default:
		}
	}
	m.output = lease
	m.phoneOnly = true
	if lease.Done != nil {
		go func() {
			select {
			case <-lease.Done:
			case <-m.startCanceled:
				return
			}
			s.mu.Lock()
			defer s.mu.Unlock()
			if s.media == m {
				s.stopMediaLocked("Suara khusus HP terputus. Berbagi dihentikan.")
			}
		}()
	}
	return true
}
