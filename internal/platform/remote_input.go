package platform

import (
	"errors"
	"unicode"
	"unicode/utf8"
)

// RemoteInput is a bounded action from an explicitly authorized screen viewer.
// Authorization and rate limiting belong to the caller; validation is repeated
// here before any native input is sent.
type RemoteInput struct {
	Kind   string `json:"kind"`
	DX     int    `json:"dx"`
	DY     int    `json:"dy"`
	Button string `json:"button"`
	Key    string `json:"key"`
	Text   string `json:"text"`
}

type remoteKey struct {
	VK       uint16
	Extended bool
	Control  bool
}

var remoteKeys = map[string]remoteKey{
	"enter":      {VK: 0x0d},
	"tab":        {VK: 0x09},
	"backspace":  {VK: 0x08},
	"escape":     {VK: 0x1b},
	"arrowleft":  {VK: 0x25, Extended: true},
	"arrowup":    {VK: 0x26, Extended: true},
	"arrowright": {VK: 0x27, Extended: true},
	"arrowdown":  {VK: 0x28, Extended: true},
	"delete":     {VK: 0x2e, Extended: true},
	"home":       {VK: 0x24, Extended: true},
	"end":        {VK: 0x23, Extended: true},
	"pageup":     {VK: 0x21, Extended: true},
	"pagedown":   {VK: 0x22, Extended: true},
	"ctrl+a":     {VK: 0x41, Control: true},
	"ctrl+c":     {VK: 0x43, Control: true},
	"ctrl+v":     {VK: 0x56, Control: true},
	"ctrl+z":     {VK: 0x5a, Control: true},
}

func validateRemoteInput(input RemoteInput) error {
	invalid := errors.New("Aksi kontrol atau parameternya tidak valid.")
	switch input.Kind {
	case "move":
		if input.DX < -500 || input.DX > 500 || input.DY < -500 || input.DY > 500 ||
			input.Button != "" || input.Key != "" || input.Text != "" {
			return invalid
		}
	case "click":
		if input.DX != 0 || input.DY != 0 || input.Key != "" || input.Text != "" ||
			(input.Button != "left" && input.Button != "right" && input.Button != "middle") {
			return invalid
		}
	case "scroll":
		if input.DX != 0 || input.DY < -1200 || input.DY > 1200 ||
			input.Button != "" || input.Key != "" || input.Text != "" {
			return invalid
		}
	case "key":
		if input.DX != 0 || input.DY != 0 || input.Button != "" || input.Text != "" {
			return invalid
		}
		if _, ok := remoteKeys[input.Key]; !ok {
			return errors.New("Tombol kontrol tidak diizinkan.")
		}
	case "text":
		if input.DX != 0 || input.DY != 0 || input.Button != "" || input.Key != "" {
			return invalid
		}
		if input.Text == "" || len(input.Text) > 256*utf8.UTFMax || !utf8.ValidString(input.Text) || utf8.RuneCountInString(input.Text) > 256 {
			return errors.New("Teks kontrol harus berisi 1–256 karakter Unicode.")
		}
		for _, r := range input.Text {
			if unicode.IsControl(r) {
				return errors.New("Teks kontrol tidak boleh berisi karakter kontrol; gunakan tombol Enter atau Tab.")
			}
		}
	default:
		return invalid
	}
	return nil
}
