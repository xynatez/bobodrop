// Package platform provides the native desktop integration for bobodrop.
package platform

import (
	"errors"
	"net/url"
	"strings"
	"unicode/utf16"
)

// TrayOptions configures the tray menu. Callbacks run in separate goroutines
// and should report any operation errors through the application's own UI/log.
type TrayOptions struct {
	Title      string
	OnOpen     func()
	OnFolder   func()
	OnCopyLink func()
	OnQuit     func()
}

func encodeUTF16Z(text string) ([]uint16, error) {
	if strings.IndexByte(text, 0) >= 0 {
		return nil, errors.New("Teks tidak boleh berisi karakter NUL.")
	}
	return append(utf16.Encode([]rune(text)), 0), nil
}

func decodeUTF16Z(data []uint16) (string, error) {
	for i, word := range data {
		if word == 0 {
			return string(utf16.Decode(data[:i])), nil
		}
	}
	return "", errors.New("Data teks clipboard Windows tidak valid.")
}

// putUTF16Z truncates fixed-size Win32 strings without splitting a surrogate
// pair, and always leaves space for the terminating NUL.
func putUTF16Z(dst []uint16, text string) error {
	data, err := encodeUTF16Z(text)
	if err != nil {
		return err
	}
	clear(dst)
	if len(dst) == 0 {
		return nil
	}
	n := min(len(data)-1, len(dst)-1)
	if n > 0 && data[n-1] >= 0xd800 && data[n-1] <= 0xdbff {
		n--
	}
	copy(dst, data[:n])
	return nil
}

func browserURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u == nil || u.Hostname() == "" || u.Opaque != "" || u.User != nil ||
		strings.ContainsAny(raw, "\"\\\x00\r\n\t") || strings.TrimSpace(raw) != raw {
		return "", errors.New("Tautan bobodrop tidak valid.")
	}
	u.Scheme = strings.ToLower(u.Scheme)
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", errors.New("Tautan harus menggunakan HTTP atau HTTPS.")
	}
	return u.String(), nil
}
