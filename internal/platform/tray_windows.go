//go:build windows

package platform

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"
)

const (
	wmNull         = 0x0000
	wmDestroy      = 0x0002
	wmClose        = 0x0010
	wmQuit         = 0x0012
	wmContextMenu  = 0x007b
	wmCommand      = 0x0111
	wmTimer        = 0x0113
	wmLeftDouble   = 0x0203
	wmRightUp      = 0x0205
	ninKeySelect   = 0x0401
	wmTrayCallback = 0x8001
	trayTimer      = 1
	trayIconID     = 1
	nimAdd         = 0
	nimDelete      = 2
	nimSetVersion  = 4
	menuOpen       = 1
	menuFolder     = 2
	menuCopyLink   = 3
	menuQuit       = 4
)

type winPoint struct{ X, Y int32 }

type winMessage struct {
	Window  uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Point   winPoint
	Private uint32
}

type winClassEx struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClassExtra int32
	WndExtra   int32
	Instance   uintptr
	Icon       uintptr
	Cursor     uintptr
	Background uintptr
	MenuName   *uint16
	ClassName  *uint16
	SmallIcon  uintptr
}

type notifyIconData struct {
	Size            uint32
	Window          uintptr
	ID              uint32
	Flags           uint32
	CallbackMessage uint32
	Icon            uintptr
	Tip             [128]uint16
	State           uint32
	StateMask       uint32
	Info            [256]uint16
	Version         uint32
	InfoTitle       [64]uint16
	InfoFlags       uint32
	GUID            [16]byte
	BalloonIcon     uintptr
}

type trayState struct {
	options        TrayOptions
	window         uintptr
	threadID       uintptr
	instance       uintptr
	className      []uint16
	taskbarMessage uintptr
	notify         notifyIconData
	trayEnabled    bool
	iconAdded      bool
	version4       bool
	destroyed      bool
	loopRunning    bool
	quitRequested  bool
	closeOnce      sync.Once
	runErr         error
}

var (
	trayWindows            sync.Map
	trayClassSequence      atomic.Uint64
	trayWindowProc         = syscall.NewCallback(trayWndProc)
	registerClassExW       = user32.NewProc("RegisterClassExW")
	unregisterClassW       = user32.NewProc("UnregisterClassW")
	defWindowProcW         = user32.NewProc("DefWindowProcW")
	registerWindowMessageW = user32.NewProc("RegisterWindowMessageW")
	getMessageW            = user32.NewProc("GetMessageW")
	translateMessage       = user32.NewProc("TranslateMessage")
	dispatchMessageW       = user32.NewProc("DispatchMessageW")
	postMessageW           = user32.NewProc("PostMessageW")
	postThreadMessageW     = user32.NewProc("PostThreadMessageW")
	postQuitMessage        = user32.NewProc("PostQuitMessage")
	loadIconW              = user32.NewProc("LoadIconW")
	setTimer               = user32.NewProc("SetTimer")
	killTimer              = user32.NewProc("KillTimer")
	createPopupMenu        = user32.NewProc("CreatePopupMenu")
	appendMenuW            = user32.NewProc("AppendMenuW")
	destroyMenu            = user32.NewProc("DestroyMenu")
	setMenuDefaultItem     = user32.NewProc("SetMenuDefaultItem")
	trackPopupMenu         = user32.NewProc("TrackPopupMenu")
	endMenu                = user32.NewProc("EndMenu")
	setForegroundWindow    = user32.NewProc("SetForegroundWindow")
	getCursorPos           = user32.NewProc("GetCursorPos")
	getModuleHandleW       = kernel32.NewProc("GetModuleHandleW")
	getCurrentThreadID     = kernel32.NewProc("GetCurrentThreadId")
	shellNotifyIconW       = shell32.NewProc("Shell_NotifyIconW")
)

// RunTray owns a hidden top-level window on one OS thread. A top-level window
// (rather than HWND_MESSAGE) receives Explorer's TaskbarCreated broadcast.
func RunTray(ctx context.Context, options TrayOptions) error {
	if ctx == nil {
		return errors.New("Konteks tray bobodrop belum tersedia.")
	}
	if ctx.Err() != nil {
		return nil
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	s, err := createTrayWindow(options)
	if err != nil {
		return err
	}
	defer s.cleanup()
	s.trayEnabled = true
	if !s.addIcon() {
		return errors.New("Ikon tray bobodrop tidak dapat ditambahkan ke Windows.")
	}
	stop := make(chan struct{})
	watcherDone := make(chan struct{})
	go func() {
		defer close(watcherDone)
		select {
		case <-ctx.Done():
			s.postClose()
		case <-stop:
		}
	}()
	defer func() {
		close(stop)
		<-watcherDone
	}()
	return s.messageLoop()
}

func createTrayWindow(options TrayOptions) (*trayState, error) {
	if options.Title == "" {
		options.Title = "bobodrop"
	}
	title, err := encodeUTF16Z(options.Title)
	if err != nil {
		return nil, err
	}
	s := &trayState{options: options}
	s.notify.Size = uint32(unsafe.Sizeof(s.notify))
	s.notify.ID = trayIconID
	s.notify.Flags = 0x0001 | 0x0002 | 0x0004 | 0x0080 // message, icon, tooltip, show tooltip with v4.
	s.notify.CallbackMessage = wmTrayCallback
	if err := putUTF16Z(s.notify.Tip[:], options.Title); err != nil {
		return nil, err
	}
	s.instance, _, err = getModuleHandleW.Call(0)
	if s.instance == 0 {
		return nil, windowsError("Memuat aplikasi bobodrop", err)
	}
	s.threadID, _, _ = getCurrentThreadID.Call()
	s.notify.Icon, _, err = loadIconW.Call(s.instance, 1) // Embedded bobodrop resource, when present.
	if s.notify.Icon == 0 {
		s.notify.Icon, _, err = loadIconW.Call(0, 32512) // Shared IDI_APPLICATION fallback for tests.
	}
	if s.notify.Icon == 0 {
		return nil, windowsError("Memuat ikon bobodrop", err)
	}
	taskbar, _ := encodeUTF16Z("TaskbarCreated")
	s.taskbarMessage, _, err = registerWindowMessageW.Call(uintptr(unsafe.Pointer(&taskbar[0])))
	if s.taskbarMessage == 0 {
		return nil, windowsError("Mendaftarkan pesan tray Windows", err)
	}
	s.className, _ = encodeUTF16Z(fmt.Sprintf("bobodrop.Tray.%d", trayClassSequence.Add(1)))
	class := winClassEx{
		Size: uint32(unsafe.Sizeof(winClassEx{})), Style: 0x0008,
		WndProc: trayWindowProc, Instance: s.instance, Icon: s.notify.Icon,
		ClassName: &s.className[0], SmallIcon: s.notify.Icon,
	}
	if atom, _, err := registerClassExW.Call(uintptr(unsafe.Pointer(&class))); atom == 0 {
		return nil, windowsError("Mendaftarkan jendela tray bobodrop", err)
	}
	s.window, _, err = createWindowExW.Call(0x0080, uintptr(unsafe.Pointer(&s.className[0])),
		uintptr(unsafe.Pointer(&title[0])), 0x80000000, 0, 0, 0, 0, 0, 0, s.instance, 0)
	if s.window == 0 {
		unregisterClassW.Call(uintptr(unsafe.Pointer(&s.className[0])), s.instance)
		return nil, windowsError("Membuat jendela tray bobodrop", err)
	}
	s.notify.Window = s.window
	trayWindows.Store(s.window, s)
	return s, nil
}

func (s *trayState) cleanup() {
	s.removeIcon()
	killTimer.Call(s.window, trayTimer)
	if !s.destroyed {
		destroyWindow.Call(s.window)
	}
	trayWindows.Delete(s.window)
	unregisterClassW.Call(uintptr(unsafe.Pointer(&s.className[0])), s.instance)
}

func (s *trayState) addIcon() bool {
	if s.iconAdded {
		return true
	}
	result, _, _ := shellNotifyIconW.Call(nimAdd, uintptr(unsafe.Pointer(&s.notify)))
	if result == 0 {
		return false
	}
	s.iconAdded = true
	s.notify.Version = 4
	result, _, _ = shellNotifyIconW.Call(nimSetVersion, uintptr(unsafe.Pointer(&s.notify)))
	s.version4 = result != 0
	return true
}

func (s *trayState) removeIcon() {
	if s.iconAdded {
		shellNotifyIconW.Call(nimDelete, uintptr(unsafe.Pointer(&s.notify)))
		s.iconAdded = false
	}
}

func (s *trayState) postClose() {
	s.closeOnce.Do(func() {
		if result, _, _ := postMessageW.Call(s.window, wmClose, 0, 0); result == 0 {
			postThreadMessageW.Call(s.threadID, wmQuit, 0, 0)
		}
	})
}

func (s *trayState) messageLoop() error {
	s.loopRunning = true
	defer func() { s.loopRunning = false }()
	var message winMessage
	for {
		result, _, err := getMessageW.Call(uintptr(unsafe.Pointer(&message)), 0, 0, 0)
		if int32(result) == -1 {
			return windowsError("Membaca pesan tray bobodrop", err)
		}
		if result == 0 {
			return s.runErr
		}
		translateMessage.Call(uintptr(unsafe.Pointer(&message)))
		dispatchMessageW.Call(uintptr(unsafe.Pointer(&message)))
	}
}

func trayWndProc(window, message, wParam, lParam uintptr) uintptr {
	value, found := trayWindows.Load(window)
	if !found {
		result, _, _ := defWindowProcW.Call(window, message, wParam, lParam)
		return result
	}
	s := value.(*trayState)
	if message == s.taskbarMessage {
		if !s.trayEnabled {
			return 0
		}
		s.iconAdded = false // Explorer discarded all tray icons during restart.
		if !s.addIcon() {
			if timer, _, err := setTimer.Call(window, trayTimer, 500, 0); timer == 0 {
				s.runErr = windowsError("Memulihkan ikon tray bobodrop", err)
				destroyWindow.Call(window)
			}
		} else {
			killTimer.Call(window, trayTimer)
		}
		return 0
	}
	switch message {
	case wmTrayCallback:
		event := lParam
		if s.version4 {
			if uint16(lParam>>16) != trayIconID {
				return 0
			}
			event = lParam & 0xffff
		} else if wParam != trayIconID {
			return 0
		}
		switch event {
		case wmLeftDouble, ninKeySelect:
			s.command(menuOpen)
		case wmContextMenu, wmRightUp:
			if s.version4 && event == wmRightUp {
				return 0 // Version 4 uses WM_CONTEXTMENU for mouse and keyboard menus.
			}
			point := winPoint{int32(int16(wParam)), int32(int16(wParam >> 16))}
			if !s.version4 || event == wmRightUp {
				getCursorPos.Call(uintptr(unsafe.Pointer(&point)))
			}
			if err := s.showMenu(point); err != nil {
				s.runErr = err
				destroyWindow.Call(window)
			}
		}
		return 0
	case wmCommand:
		s.command(wParam & 0xffff)
		return 0
	case wmTimer:
		if wParam == trayTimer && s.addIcon() {
			killTimer.Call(window, trayTimer)
		}
		return 0
	case wmClose:
		endMenu.Call()
		destroyWindow.Call(window)
		return 0
	case wmDestroy:
		s.closeOnce.Do(func() {}) // Suppress a late cancellation post to a recycled HWND/thread.
		s.destroyed = true
		s.removeIcon()
		killTimer.Call(window, trayTimer)
		if s.loopRunning {
			postQuitMessage.Call(0)
		}
		return 0
	}
	result, _, _ := defWindowProcW.Call(window, message, wParam, lParam)
	return result
}

func (s *trayState) command(id uintptr) {
	var callback func()
	switch id {
	case menuOpen:
		callback = s.options.OnOpen
	case menuFolder:
		callback = s.options.OnFolder
	case menuCopyLink:
		callback = s.options.OnCopyLink
	case menuQuit:
		if !s.quitRequested {
			s.quitRequested = true
			go func() {
				if s.options.OnQuit != nil {
					s.options.OnQuit()
				}
				s.postClose()
			}()
		}
		return
	}
	if callback != nil {
		go callback()
	}
}

func (s *trayState) showMenu(point winPoint) error {
	menu, _, err := createPopupMenu.Call()
	if menu == 0 {
		return windowsError("Membuat menu bobodrop", err)
	}
	defer destroyMenu.Call(menu)
	items := []struct {
		id      uintptr
		label   string
		enabled bool
	}{
		{menuOpen, "Buka bobodrop / Open bobodrop", s.options.OnOpen != nil},
		{menuFolder, "Buka folder / Open folder", s.options.OnFolder != nil},
		{menuCopyLink, "Salin tautan HP / Copy phone link", s.options.OnCopyLink != nil},
		{0, "", false},
		{menuQuit, "Keluar / Quit", true},
	}
	for _, item := range items {
		flags := uintptr(0)
		label, _ := encodeUTF16Z(item.label)
		if item.id == 0 {
			flags = 0x0800 // MF_SEPARATOR
		} else if !item.enabled {
			flags = 0x0001 // MF_GRAYED
		}
		if result, _, err := appendMenuW.Call(menu, flags, item.id, uintptr(unsafe.Pointer(&label[0]))); result == 0 {
			return windowsError("Menambahkan menu bobodrop", err)
		}
	}
	setMenuDefaultItem.Call(menu, menuOpen, 0)
	setForegroundWindow.Call(s.window)
	command, _, _ := trackPopupMenu.Call(menu, 0x0100|0x0080|0x0002,
		uintptr(point.X), uintptr(point.Y), 0, s.window, 0)
	postMessageW.Call(s.window, wmNull, 0, 0)
	if !s.destroyed {
		s.command(command)
	}
	return nil
}
