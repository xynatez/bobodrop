package platform

import (
	"strings"
	"testing"
)

func TestRemoteInputValidation(t *testing.T) {
	valid := []RemoteInput{
		{Kind: "move", DX: -500, DY: 500}, {Kind: "move"},
		{Kind: "click", Button: "left"}, {Kind: "click", Button: "right"}, {Kind: "click", Button: "middle"},
		{Kind: "scroll", DY: -1200}, {Kind: "scroll", DY: 1200}, {Kind: "scroll"},
		{Kind: "text", Text: "日本語 — café 👨‍👩‍👧 🦷"}, {Kind: "text", Text: strings.Repeat("🦷", 256)},
	}
	for _, key := range []string{"enter", "tab", "backspace", "escape", "arrowup", "arrowdown", "arrowleft", "arrowright", "delete", "home", "end", "pageup", "pagedown", "ctrl+a", "ctrl+c", "ctrl+v", "ctrl+z"} {
		valid = append(valid, RemoteInput{Kind: "key", Key: key})
	}
	for _, input := range valid {
		if err := validateRemoteInput(input); err != nil {
			t.Errorf("valid input %+v rejected: %v", input, err)
		}
	}
	for _, input := range invalidRemoteInputs() {
		if err := validateRemoteInput(input); err == nil {
			t.Errorf("invalid input %+v accepted", input)
		}
	}
}

func invalidRemoteInputs() []RemoteInput {
	return []RemoteInput{
		{}, {Kind: "keydown", Key: "control"}, {Kind: "keyup", Key: "control"}, {Kind: "command", Text: "whoami"},
		{Kind: "move", DX: 501}, {Kind: "move", DX: -501}, {Kind: "move", DY: 501}, {Kind: "move", DY: -501},
		{Kind: "move", Button: "left"}, {Kind: "move", Key: "enter"}, {Kind: "move", Text: "x"},
		{Kind: "click"}, {Kind: "click", Button: "x1"}, {Kind: "click", Button: "left", DX: 1},
		{Kind: "click", Button: "left", DY: 1}, {Kind: "click", Button: "left", Key: "enter"}, {Kind: "click", Button: "left", Text: "x"},
		{Kind: "scroll", DY: -1201}, {Kind: "scroll", DY: 1201}, {Kind: "scroll", DX: 1},
		{Kind: "scroll", Button: "left"}, {Kind: "scroll", Key: "enter"}, {Kind: "scroll", Text: "x"},
		{Kind: "key", Key: "Enter"}, {Kind: "key", Key: "control"}, {Kind: "key", Key: "alt+f4"},
		{Kind: "key", Key: "win+r"}, {Kind: "key", Key: "ctrl+x"}, {Kind: "key", Key: "ctrl+shift+v"},
		{Kind: "key", Key: "enter", DX: 1}, {Kind: "key", Key: "enter", DY: 1},
		{Kind: "key", Key: "enter", Button: "left"}, {Kind: "key", Key: "enter", Text: "x"},
		{Kind: "text"}, {Kind: "text", Text: "bad\x00text"}, {Kind: "text", Text: "bad\ntext"},
		{Kind: "text", Text: "bad\rtext"}, {Kind: "text", Text: "bad\ttext"},
		{Kind: "text", Text: "\x1b"}, {Kind: "text", Text: "\u007f"}, {Kind: "text", Text: "\u0085"},
		{Kind: "text", Text: string([]byte{0xff})}, {Kind: "text", Text: strings.Repeat("🦷", 257)},
		{Kind: "text", Text: "x", DX: 1}, {Kind: "text", Text: "x", DY: 1},
		{Kind: "text", Text: "x", Button: "left"}, {Kind: "text", Text: "x", Key: "enter"},
	}
}
