package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	qrcode "github.com/skip2/go-qrcode"
)

const maxFileBytes int64 = 10 << 30

type contextReader struct {
	context context.Context
	reader  io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.context.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

func safeFilename(original string) string {
	name := path.Base(strings.ReplaceAll(original, "\\", "/"))
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || strings.ContainsRune(`<>:"/\|?*`, r) {
			return '_'
		}
		return r
	}, name)
	name = strings.Trim(name, " .")
	if name == "" {
		name = "file"
	}
	stem := strings.ToUpper(strings.TrimRight(strings.SplitN(name, ".", 2)[0], " "))
	reserved := stem == "CON" || stem == "PRN" || stem == "AUX" || stem == "NUL" || stem == "CLOCK$"
	if strings.HasPrefix(stem, "COM") || strings.HasPrefix(stem, "LPT") {
		suffix := strings.TrimPrefix(strings.TrimPrefix(stem, "COM"), "LPT")
		reserved = reserved || (len([]rune(suffix)) == 1 && strings.ContainsRune("123456789¹²³", []rune(suffix)[0]))
	}
	if reserved {
		name = "_" + name
	}
	// Keep room for collision suffixes on Windows and on byte-limited filesystems.
	if len(name) > 180 {
		ext := filepath.Ext(name)
		if len(ext) > 32 {
			ext = ""
		}
		base := strings.TrimSuffix(name, ext)
		for len(base) > 180-len(ext) {
			_, size := utf8.DecodeLastRuneInString(base)
			base = base[:len(base)-size]
		}
		name = strings.TrimRight(base, " .") + ext
	}
	return name
}

func (s *Server) upload(w http.ResponseWriter, r *http.Request, p principal) {
	expectedSize := int64(-1)
	if values := r.Header.Values("X-File-Size"); len(values) > 0 {
		var err error
		if len(values) != 1 {
			writeError(w, http.StatusBadRequest, "Data unggahan tidak valid.")
			return
		}
		expectedSize, err = strconv.ParseInt(values[0], 10, 64)
		if err != nil || expectedSize < 0 || strconv.FormatInt(expectedSize, 10) != values[0] {
			writeError(w, http.StatusBadRequest, "Data unggahan tidak valid.")
			return
		}
		if expectedSize > maxFileBytes {
			writeError(w, http.StatusRequestEntityTooLarge, "Ukuran file melebihi batas 10 GiB.")
			return
		}
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxFileBytes+(1<<20))
	var abortOnce sync.Once
	abortDone := make(chan struct{})
	abort := func() {
		abortOnce.Do(func() {
			defer close(abortDone)
			// net/http Body.Close waits for an active Read. Interrupt the socket
			// first so revoking a stalled peer can actually remove its temp file.
			_ = http.NewResponseController(w).SetReadDeadline(time.Now())
			_ = r.Body.Close()
		})
	}
	stop := context.AfterFunc(s.ctx, abort)
	var stopPeer func() bool
	if !p.host {
		stopPeer = context.AfterFunc(p.peerContext, abort)
	}
	defer func() {
		stopped := stop()
		if stopPeer != nil {
			stopped = stopPeer() && stopped
		}
		if !stopped {
			// Do not let an asynchronous cancellation change the deadline of a
			// subsequent request that reuses this connection.
			<-abortDone
		}
	}()
	reader, err := r.MultipartReader()
	if err != nil {
		writeError(w, http.StatusBadRequest, "Pilih satu file untuk dikirim.")
		return
	}
	var temp *os.File
	var tempName, name string
	var size int64
	defer func() {
		if temp != nil {
			_ = temp.Close()
		}
		if tempName != "" {
			_ = os.Remove(tempName)
		}
	}()
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			uploadError(w, err)
			return
		}
		if part.FileName() == "" {
			n, err := io.Copy(io.Discard, io.LimitReader(part, 4097))
			if err != nil {
				uploadError(w, err)
				return
			}
			if n > 4096 {
				writeError(w, http.StatusBadRequest, "Data unggahan tidak valid.")
				return
			}
			_ = part.Close()
			continue
		}
		if part.FormName() != "files" || tempName != "" {
			writeError(w, http.StatusBadRequest, "Kirim satu file dalam setiap unggahan.")
			return
		}
		name = safeFilename(part.FileName())
		temp, err = os.CreateTemp(s.config.StorageDir, ".localdrop-*.part")
		if err != nil {
			writeError(w, http.StatusInternalServerError, "File belum dapat disimpan. Periksa folder penyimpanan.")
			return
		}
		tempName = temp.Name()
		ctx := s.ctx
		if !p.host {
			ctx = p.peerContext
		}
		limit := maxFileBytes + 1
		if expectedSize >= 0 && expectedSize < maxFileBytes {
			limit = expectedSize + 1
		}
		size, err = io.CopyBuffer(temp, contextReader{context: ctx, reader: io.LimitReader(part, limit)}, make([]byte, 128<<10))
		if err != nil {
			uploadError(w, err)
			return
		}
		if size > maxFileBytes {
			writeError(w, http.StatusRequestEntityTooLarge, "Ukuran file melebihi batas 10 GiB.")
			return
		}
		if expectedSize >= 0 && size != expectedSize {
			uploadError(w, io.ErrUnexpectedEOF)
			return
		}
		if err := part.Close(); err != nil {
			uploadError(w, err)
			return
		}
	}
	if temp == nil {
		writeError(w, http.StatusBadRequest, "Pilih satu file untuk dikirim.")
		return
	}
	if err := r.Context().Err(); err != nil {
		uploadError(w, err)
		return
	}
	if err := temp.Sync(); err != nil {
		writeError(w, http.StatusInternalServerError, "File belum dapat disimpan. Periksa ruang kosong di laptop.")
		return
	}
	if _, err := temp.Seek(0, io.SeekStart); err != nil {
		writeError(w, http.StatusInternalServerError, "File belum dapat dibaca.")
		return
	}
	var sniff [512]byte
	n, err := temp.Read(sniff[:])
	if err != nil && err != io.EOF {
		writeError(w, http.StatusInternalServerError, "File belum dapat dibaca.")
		return
	}
	contentType := http.DetectContentType(sniff[:n])
	if err := temp.Close(); err != nil {
		writeError(w, http.StatusInternalServerError, "File belum dapat disimpan.")
		return
	}
	temp = nil
	id, err := randomID(18)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "File belum dapat disimpan.")
		return
	}
	s.mu.Lock()
	if !s.stillAuthorizedLocked(p) {
		s.mu.Unlock()
		writeError(w, http.StatusUnauthorized, "Sesi sudah berakhir. Scan QR terbaru di laptop.")
		return
	}
	finalName, finalPath, err := commitFile(tempName, s.config.StorageDir, name)
	if err != nil {
		s.mu.Unlock()
		writeError(w, http.StatusInternalServerError, "File belum dapat disimpan. Periksa folder dan ruang kosong di laptop.")
		return
	}
	entry := File{ID: id, Name: finalName, Size: size, MIME: contentType, Sender: s.senderLocked(r, p), CreatedAt: time.Now().UTC(), DownloadURL: "/api/files/" + id, path: finalPath}
	entry.SenderID, entry.SenderRole = s.receiptOriginLocked(r, p)
	s.files = append([]File{entry}, s.files...)
	s.mu.Unlock()
	s.broadcast()
	writeJSON(w, http.StatusCreated, struct {
		Files []File `json:"files"`
	}{Files: []File{entry}})
}

func commitFile(tempName, dir, name string) (string, string, error) {
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	for collision := 0; collision < 100000; collision++ {
		candidate := name
		if collision > 0 {
			candidate = fmt.Sprintf("%s (%d)%s", base, collision+1, ext)
		}
		destination := filepath.Join(dir, candidate)
		if err := moveNoReplace(tempName, destination); err != nil {
			if errors.Is(err, os.ErrExist) {
				continue
			}
			return "", "", err
		}
		return candidate, destination, nil
	}
	return "", "", errors.New("too many filename collisions")
}

func uploadError(w http.ResponseWriter, err error) {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		writeError(w, http.StatusRequestEntityTooLarge, "Ukuran file melebihi batas 10 GiB.")
		return
	}
	writeError(w, http.StatusBadRequest, "Unggahan terputus atau data file tidak lengkap. Kirim ulang file.")
}

func (s *Server) download(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/files/")
	s.mu.Lock()
	var entry File
	for _, f := range s.files {
		if f.ID == id {
			entry = f
			break
		}
	}
	s.mu.Unlock()
	if entry.ID == "" {
		writeError(w, http.StatusNotFound, "File tidak ditemukan dalam sesi ini.")
		return
	}
	file, err := os.Open(entry.path)
	if err != nil {
		writeError(w, http.StatusNotFound, "File sudah tidak tersedia di folder laptop.")
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		writeError(w, http.StatusNotFound, "File sudah tidak tersedia di folder laptop.")
		return
	}
	w.Header().Set("Content-Type", entry.MIME)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": entry.Name}))
	w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'")
	http.ServeContent(w, r, entry.Name, info.ModTime(), file)
}

func (s *Server) qr(w http.ResponseWriter, r *http.Request) {
	ip := r.URL.Query().Get("ip")
	s.mu.Lock()
	valid := ip == "" || (len(s.config.Addresses) == 0 && ip == "localhost")
	for _, a := range s.config.Addresses {
		if a.IP == ip {
			valid = true
			break
		}
	}
	if !valid {
		s.mu.Unlock()
		writeError(w, http.StatusBadRequest, "Pilih alamat jaringan yang tersedia di laptop.")
		return
	}
	joinURL := s.joinURLLocked(ip)
	s.mu.Unlock()
	png, err := qrcode.Encode(joinURL, qrcode.Medium, 320)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "QR belum dapat dibuat.")
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(png)
}
