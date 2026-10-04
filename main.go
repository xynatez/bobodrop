package main

import (
	"context"
	"embed"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"localdrop/internal/platform"
	"localdrop/internal/server"
)

//go:embed web/*
var embeddedWeb embed.FS

var version = "1.4.1"

func main() {
	if err := run(); err != nil {
		log.Printf("bobodrop: %v", err)
		os.Exit(1)
	}
}

func run() error {
	if handled, err := platform.RunSystemAudioHelper(os.Args[1:]); handled {
		return err
	}
	if handled, err := platform.RunAudioOutputHelper(os.Args[1:]); handled {
		return err
	}
	port := flag.Int("port", 8787, "Port lokal (default mencoba 8787-8797 jika sibuk)")
	bind := flag.String("bind", "0.0.0.0", "Alamat IPv4 untuk server")
	dataDir := flag.String("data-dir", "", "Folder penerimaan file (default Downloads/LocalDrop)")
	deviceName := flag.String("device-name", "", "Nama laptop yang tampil di HP")
	noOpen := flag.Bool("no-open", false, "Jangan buka antarmuka otomatis")
	noTray := flag.Bool("no-tray", false, "Jalankan tanpa system tray (untuk pengujian)")
	showVersion := flag.Bool("version", false, "Tampilkan versi")
	flag.Parse()
	if *showVersion {
		fmt.Println("bobodrop " + version)
		return nil
	}
	if *port < 1 || *port > 65535 {
		return errors.New("port harus berada antara 1 dan 65535")
	}
	if *dataDir == "" {
		downloads, err := downloadsDirectory()
		if err != nil {
			return fmt.Errorf("menemukan folder Downloads: %w", err)
		}
		*dataDir = filepath.Join(downloads, "LocalDrop")
	}
	absDir, err := filepath.Abs(*dataDir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(absDir, 0700); err != nil {
		return fmt.Errorf("membuat folder penerimaan: %w", err)
	}
	closeLog := configureLog()
	defer closeLog()
	if *deviceName == "" {
		*deviceName, _ = os.Hostname()
		if *deviceName == "" {
			*deviceName = "Laptop Windows"
		}
	}
	explicitPort := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "port" {
			explicitPort = true
		}
	})
	listener, actualPort, err := listen(*bind, *port, !explicitPort)
	if err != nil {
		return fmt.Errorf("server lokal tidak dapat dibuka: %w", err)
	}
	defer listener.Close()
	app, err := server.New(server.Config{
		DeviceName:     *deviceName,
		StorageDir:     absDir,
		Port:           actualPort,
		Addresses:      networkAddresses(),
		ClipboardRead:  platform.ClipboardText,
		ClipboardWrite: platform.SetClipboardText,
		OpenFolder:     func() error { return platform.OpenFolder(absDir) },
		MediaRelay:     prepareMediaRelay,
		SystemAudio: func(ctx context.Context) (*server.SystemAudioCapture, error) {
			capture, err := platform.BeginSystemAudioCapture(ctx)
			if err != nil {
				return nil, err
			}
			return &server.SystemAudioCapture{PCM: capture.PCM, SampleRate: capture.SampleRate, Channels: capture.Channels, Close: capture.Close, Done: capture.Done}, nil
		},
		AudioOutput: func() (*server.AudioOutputLease, error) {
			lease, err := platform.BeginSilentAudioOutput()
			if err != nil {
				return nil, err
			}
			return &server.AudioOutputLease{Revoke: lease.Revoke, Close: lease.Close, Done: lease.Done}, nil
		},
		ControlInput: func(input server.RemoteInput) error {
			return platform.SendRemoteInput(platform.RemoteInput{
				Kind: input.Kind, DX: input.DX, DY: input.DY,
				Button: input.Button, Key: input.Key, Text: input.Text,
			})
		},
	})
	if err != nil {
		return err
	}
	defer app.Close()
	assets, err := fs.Sub(embeddedWeb, "web")
	if err != nil {
		return err
	}
	app.SetAssets(http.FileServer(http.FS(assets)))
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	go func() {
		// DHCP, roaming, and switching Wi-Fi can change addresses while the app
		// stays open. Keep both the QR and allowed HTTP hosts in sync.
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				app.UpdateAddresses(networkAddresses())
			}
		}
	}()
	httpServer := &http.Server{
		Handler:           app.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       90 * time.Second,
		MaxHeaderBytes:    64 << 10,
		// Streaming uploads intentionally have no overall read deadline.
	}
	serveErrors := make(chan error, 1)
	go func() { serveErrors <- httpServer.Serve(listener) }()
	localURL := "http://127.0.0.1:" + strconv.Itoa(actualPort)
	openApp := func() {
		if err := platform.OpenURL(localURL); err != nil {
			log.Printf("Membuka antarmuka: %v", err)
		}
	}
	trayDone := make(chan struct{})
	if !*noTray {
		go func() {
			defer close(trayDone)
			err := platform.RunTray(ctx, platform.TrayOptions{
				Title:    "bobodrop, transfer Wi-Fi",
				OnOpen:   openApp,
				OnFolder: func() { _ = platform.OpenFolder(absDir) },
				OnCopyLink: func() {
					if err := platform.SetClipboardText(app.JoinURL()); err != nil {
						log.Printf("Menyalin tautan: %v", err)
					}
				},
				OnQuit: cancel,
			})
			if err != nil {
				log.Printf("System tray: %v", err)
			}
		}()
	} else {
		close(trayDone)
	}
	defer func() {
		cancel()
		// Let the native message loop remove its icon before process exit.
		select {
		case <-trayDone:
		case <-time.After(2 * time.Second):
		}
	}()
	log.Printf("bobodrop %s aktif di %s; folder %s", version, localURL, absDir)
	if !*noOpen {
		openApp()
	}
	select {
	case <-ctx.Done():
	case err := <-serveErrors:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	}
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		_ = httpServer.Close()
	}
	return nil
}

func listen(bind string, port int, fallback bool) (net.Listener, int, error) {
	last := port
	if fallback && port == 8787 {
		last = 8797
	}
	var lastErr error
	for candidate := port; candidate <= last; candidate++ {
		listener, err := net.Listen("tcp4", net.JoinHostPort(bind, strconv.Itoa(candidate)))
		if err == nil {
			return listener, candidate, nil
		}
		lastErr = err
	}
	return nil, 0, lastErr
}

func networkAddresses() []server.NetworkAddress {
	preferred := ""
	// A UDP connect selects the default route; no packet is sent.
	if connection, err := net.Dial("udp4", "192.0.2.1:9"); err == nil {
		preferred = connection.LocalAddr().(*net.UDPAddr).IP.String()
		_ = connection.Close()
	}
	interfaces, _ := net.Interfaces()
	addresses := make([]server.NetworkAddress, 0)
	for _, network := range interfaces {
		if network.Flags&net.FlagUp == 0 || network.Flags&net.FlagLoopback != 0 {
			continue
		}
		entries, _ := network.Addrs()
		for _, entry := range entries {
			ip, _, err := net.ParseCIDR(entry.String())
			if err != nil || ip.To4() == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
				continue
			}
			addresses = append(addresses, server.NetworkAddress{Name: network.Name, IP: ip.String()})
		}
	}
	sort.SliceStable(addresses, func(i, j int) bool {
		score := func(address server.NetworkAddress) int {
			if address.IP == preferred {
				return 0
			}
			name := strings.ToLower(address.Name)
			if strings.Contains(name, "wi-fi") || strings.Contains(name, "wlan") {
				return 1
			}
			if strings.Contains(name, "vmware") || strings.Contains(name, "virtual") || strings.Contains(name, "vethernet") {
				return 3
			}
			return 2
		}
		return score(addresses[i]) < score(addresses[j])
	})
	return addresses
}

func configureLog() func() {
	base, err := os.UserConfigDir()
	if err != nil {
		return func() {}
	}
	directory := filepath.Join(base, "LocalDrop")
	if err := os.MkdirAll(directory, 0700); err != nil {
		return func() {}
	}
	path := filepath.Join(directory, "localdrop.log")
	// Cap diagnostics without preserving clipboard contents or session tokens.
	if info, err := os.Stat(path); err == nil && info.Size() > 2<<20 {
		_ = os.Truncate(path, 0)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return func() {}
	}
	// GUI executables may have no valid stderr handle; persist diagnostics first.
	log.SetOutput(io.MultiWriter(file, os.Stderr))
	return func() { _ = file.Close() }
}
