package platform

import (
	"strings"
	"testing"
)

func TestClipboardUTF16RoundTrip(t *testing.T) {
	for _, text := range []string{"", "Tautan iPhone\r\nBaris kedua", "Foto gigi — café 日本語 العربية 📷🦷", strings.Repeat("🦷", 4096)} {
		encoded, err := encodeUTF16Z(text)
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := decodeUTF16Z(encoded)
		if err != nil || decoded != text {
			t.Fatalf("UTF-16 round trip: got %q, err %v; want %q", decoded, err, text)
		}
	}
	if _, err := encodeUTF16Z("before\x00after"); err == nil {
		t.Fatal("embedded NUL must be rejected before a clipboard write")
	}
	if _, err := decodeUTF16Z([]uint16{'n', 'o'}); err == nil {
		t.Fatal("unterminated clipboard memory must be rejected")
	}
	text, err := decodeUTF16Z([]uint16{'o', 'k', 0, 'x'})
	if err != nil || text != "ok" {
		t.Fatalf("data after terminator: got %q, %v", text, err)
	}
}

func TestTooltipKeepsCompleteUnicodeCharacters(t *testing.T) {
	for _, test := range []struct {
		size int
		want string
	}{{1, ""}, {2, "A"}, {3, "A"}, {4, "A🦷"}, {5, "A🦷B"}} {
		data := make([]uint16, test.size)
		for i := range data {
			data[i] = 'x'
		}
		if err := putUTF16Z(data, "A🦷B"); err != nil {
			t.Fatal(err)
		}
		text, err := decodeUTF16Z(data)
		if err != nil || text != test.want {
			t.Fatalf("buffer size %d: got %q, %v; want %q", test.size, text, err, test.want)
		}
	}
}

func TestBrowserURLRejectsExecutableAndShellTargets(t *testing.T) {
	for _, raw := range []string{"file:///C:/Windows/notepad.exe", "javascript:alert(1)", "shell:Downloads", "C:\\Windows\\notepad.exe", "https://", "http://user@localhost/", "http://localhost/\n", " http://localhost/", "http://localhost/\"x", "http://localhost/\\x"} {
		if result, err := browserURL(raw); err == nil {
			t.Errorf("unsafe target %q accepted as %q", raw, result)
		}
	}
	for _, raw := range []string{"http://127.0.0.1:8765/", "http://192.168.1.20:8765/s/abc", "https://localhost/", "http://[::1]:8765/"} {
		if _, err := browserURL(raw); err != nil {
			t.Errorf("valid browser target %q: %v", raw, err)
		}
	}
}
