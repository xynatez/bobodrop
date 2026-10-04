//go:build windows

package platform

import (
	"fmt"
	"sync"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

const (
	inputMouse      = 0
	inputKeyboard   = 1
	keyExtended     = 0x0001
	keyUp           = 0x0002
	keyUnicode      = 0x0004
	mouseMove       = 0x0001
	mouseLeftDown   = 0x0002
	mouseLeftUp     = 0x0004
	mouseRightDown  = 0x0008
	mouseRightUp    = 0x0010
	mouseMiddleDown = 0x0020
	mouseMiddleUp   = 0x0040
	mouseWheel      = 0x0800
)

type winMouseInput struct {
	DX        int32
	DY        int32
	MouseData uint32
	Flags     uint32
	Time      uint32
	ExtraInfo uintptr
}

type winKeyboardInput struct {
	VK        uint16
	Scan      uint16
	Flags     uint32
	Time      uint32
	ExtraInfo uintptr
}

// MOUSEINPUT is the largest INPUT union member and supplies its native
// alignment: INPUT is 40 bytes on amd64 and 28 bytes on 386.
type winInput struct {
	Type uint32
	Data winMouseInput
}

type inputDispatcher func([]winInput) (uint32, error)

var (
	remoteInputMu sync.Mutex
	winSendInput  = user32.NewProc("SendInput")
)

// SendRemoteInput sends one validated, paired batch to the current Windows
// desktop. Windows may deny input to an elevated app or a secure desktop.
func SendRemoteInput(input RemoteInput) error {
	return sendRemoteInput(input, dispatchWindowsInput)
}

func dispatchWindowsInput(events []winInput) (uint32, error) {
	result, _, err := winSendInput.Call(uintptr(len(events)), uintptr(unsafe.Pointer(&events[0])), unsafe.Sizeof(winInput{}))
	return uint32(result), err
}

func sendRemoteInput(input RemoteInput, dispatch inputDispatcher) error {
	if err := validateRemoteInput(input); err != nil {
		return err
	}
	events := remoteInputEvents(input)
	if len(events) == 0 {
		return nil // Zero movement/wheel delta is a valid no-op.
	}
	remoteInputMu.Lock()
	defer remoteInputMu.Unlock()
	sent, err := dispatch(events)
	if sent == uint32(len(events)) {
		return nil
	}
	message := fmt.Sprintf("Windows tidak mengirim semua input kontrol (%d/%d). Aplikasi administrator atau desktop aman dapat memblokir kontrol.", sent, len(events))
	if sent > 0 && sent < uint32(len(events)) {
		// A partial batch can leave a button/modifier down. Release only the
		// keys/buttons actually pressed by the accepted prefix, in reverse order.
		if releases := releasesAfterPartialInput(events[:sent]); len(releases) > 0 {
			if released, _ := dispatch(releases); released != uint32(len(releases)) {
				message += " Windows juga menolak pelepasan sebagian tombol."
			}
		}
	}
	if err != nil && err != syscall.Errno(0) {
		return fmt.Errorf("%s: %w", message, err)
	}
	return fmt.Errorf("%s", message)
}

func remoteInputEvents(input RemoteInput) []winInput {
	switch input.Kind {
	case "move":
		if input.DX == 0 && input.DY == 0 {
			return nil
		}
		return []winInput{{Type: inputMouse, Data: winMouseInput{DX: int32(input.DX), DY: int32(input.DY), Flags: mouseMove}}}
	case "scroll":
		if input.DY == 0 {
			return nil
		}
		return []winInput{{Type: inputMouse, Data: winMouseInput{MouseData: uint32(int32(input.DY)), Flags: mouseWheel}}}
	case "click":
		down, up := uint32(mouseLeftDown), uint32(mouseLeftUp)
		switch input.Button {
		case "right":
			down, up = mouseRightDown, mouseRightUp
		case "middle":
			down, up = mouseMiddleDown, mouseMiddleUp
		}
		return []winInput{
			{Type: inputMouse, Data: winMouseInput{Flags: down}},
			{Type: inputMouse, Data: winMouseInput{Flags: up}},
		}
	case "key":
		key := remoteKeys[input.Key]
		flags := uint32(0)
		if key.Extended {
			flags = keyExtended
		}
		events := []winInput{keyboardEvent(key.VK, 0, flags), keyboardEvent(key.VK, 0, flags|keyUp)}
		if key.Control {
			events = []winInput{keyboardEvent(0x11, 0, 0), events[0], events[1], keyboardEvent(0x11, 0, keyUp)}
		}
		return events
	case "text":
		encoded := utf16.Encode([]rune(input.Text))
		events := make([]winInput, 0, len(encoded)*2)
		for _, scan := range encoded {
			events = append(events, keyboardEvent(0, scan, keyUnicode), keyboardEvent(0, scan, keyUnicode|keyUp))
		}
		return events
	}
	return nil
}

func keyboardEvent(vk, scan uint16, flags uint32) winInput {
	event := winInput{Type: inputKeyboard}
	keyboard := (*winKeyboardInput)(unsafe.Pointer(&event.Data))
	keyboard.VK = vk
	keyboard.Scan = scan
	keyboard.Flags = flags
	return event
}

func keyboardData(event *winInput) winKeyboardInput {
	return *(*winKeyboardInput)(unsafe.Pointer(&event.Data))
}

func releasesAfterPartialInput(events []winInput) []winInput {
	var held []winInput
	for _, event := range events {
		release := event
		isUp := false
		if event.Type == inputKeyboard {
			keyboard := keyboardData(&event)
			isUp = keyboard.Flags&keyUp != 0
			release = keyboardEvent(keyboard.VK, keyboard.Scan, keyboard.Flags|keyUp)
		} else {
			switch event.Data.Flags {
			case mouseLeftDown:
				release.Data.Flags = mouseLeftUp
			case mouseRightDown:
				release.Data.Flags = mouseRightUp
			case mouseMiddleDown:
				release.Data.Flags = mouseMiddleUp
			case mouseLeftUp, mouseRightUp, mouseMiddleUp:
				isUp = true
			default:
				continue
			}
		}
		if !isUp {
			held = append(held, release)
			continue
		}
		for i := len(held) - 1; i >= 0; i-- {
			if held[i].Type != release.Type {
				continue
			}
			matches := held[i].Data.Flags == release.Data.Flags
			if release.Type == inputKeyboard {
				matches = keyboardData(&held[i]) == keyboardData(&release)
			}
			if matches {
				held = append(held[:i], held[i+1:]...)
				break
			}
		}
	}
	for left, right := 0, len(held)-1; left < right; left, right = left+1, right-1 {
		held[left], held[right] = held[right], held[left]
	}
	return held
}
