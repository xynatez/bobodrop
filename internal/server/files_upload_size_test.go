package server

import (
	"bytes"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
)

func TestUploadDeclaredSize(t *testing.T) {
	for _, tc := range []struct {
		name     string
		declared string
		data     []byte
		status   int
	}{
		{"matching provider bytes", "7", []byte("payload"), http.StatusCreated},
		{"empty ordinary file", "0", nil, http.StatusCreated},
		{"legacy client without size", "", []byte("legacy"), http.StatusCreated},
		{"provider returned no bytes", "7", nil, http.StatusBadRequest},
		{"provider returned short bytes", "7", []byte("pay"), http.StatusBadRequest},
		{"provider returned extra bytes", "2", []byte("payload"), http.StatusBadRequest},
		{"negative", "-1", nil, http.StatusBadRequest},
		{"not decimal", "1.5", nil, http.StatusBadRequest},
		{"overflow", "9223372036854775808", nil, http.StatusBadRequest},
		{"duplicate joined values", "7, 7", nil, http.StatusBadRequest},
		{"over file limit", strconv.FormatInt(maxFileBytes+1, 10), nil, http.StatusRequestEntityTooLarge},
		{"file limit still requires full bytes", strconv.FormatInt(maxFileBytes, 10), nil, http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestServer(t)
			peer := peerSession(t, s)
			body, contentType := multipartBody(t, "provider.zip", tc.data)
			headers := map[string]string{"Content-Type": contentType}
			if tc.declared != "" {
				headers["X-File-Size"] = tc.declared
			}
			w := request(t, s, http.MethodPost, "/api/files", bytes.NewReader(body), peer, false, headers)
			if w.Code != tc.status {
				t.Fatalf("status = %d, want %d: %s", w.Code, tc.status, w.Body.String())
			}
			entries, err := os.ReadDir(s.config.StorageDir)
			if err != nil {
				t.Fatal(err)
			}
			if tc.status != http.StatusCreated {
				if len(entries) != 0 || len(s.state(true).Files) != 0 {
					t.Fatal("incomplete upload left disk files or a receipt")
				}
				return
			}
			if len(entries) != 1 || len(s.state(true).Files) != 1 {
				t.Fatal("complete upload did not create exactly one file and receipt")
			}
			file := s.state(true).Files[0]
			w = request(t, s, http.MethodGet, file.DownloadURL, nil, peer, false, nil)
			if w.Code != http.StatusOK || file.Size != int64(len(tc.data)) || !bytes.Equal(w.Body.Bytes(), tc.data) {
				t.Fatal("download or receipt differs from declared provider bytes")
			}
		})
	}
}

func TestTruncatedDeclaredUploadCleansTemporaryFile(t *testing.T) {
	s := newTestServer(t)
	peer := peerSession(t, s)
	data := bytes.Repeat([]byte("zip bytes"), 1000)
	body, contentType := multipartBody(t, "provider.zip", data)
	w := request(t, s, http.MethodPost, "/api/files", bytes.NewReader(body[:len(body)-30]), peer, false, map[string]string{
		"Content-Type": contentType,
		"X-File-Size":  strconv.Itoa(len(data)),
	})
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "Unggahan terputus") {
		t.Fatalf("truncated body = %d %s", w.Code, w.Body.String())
	}
	entries, err := os.ReadDir(s.config.StorageDir)
	if err != nil || len(entries) != 0 || len(s.state(true).Files) != 0 {
		t.Fatalf("incomplete multipart left disk files or receipts: %v %v", entries, err)
	}
}
