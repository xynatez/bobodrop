param([string]$GoExe = "go", [switch]$Console, [string]$OutputDir = (Join-Path $PSScriptRoot "dist"))
$ErrorActionPreference = "Stop"
Set-Location -LiteralPath $PSScriptRoot
$releaseDir = [System.IO.Path]::GetFullPath($OutputDir)
New-Item -ItemType Directory -Path $releaseDir -Force | Out-Null
$env:CGO_ENABLED = "0"
$flags = "-s -w -X main.version=1.4.1"
if (-not $Console) { $flags += " -H=windowsgui" }
& $GoExe test ./...
if ($LASTEXITCODE -ne 0) { throw "Pengujian gagal" }
& $GoExe vet ./...
if ($LASTEXITCODE -ne 0) { throw "Pemeriksaan Go gagal" }
& $GoExe build -trimpath -ldflags $flags -o (Join-Path $releaseDir "bobodrop.exe") .
if ($LASTEXITCODE -ne 0) { throw "Build gagal" }
Copy-Item -LiteralPath (Join-Path $PSScriptRoot "Enable-LAN.ps1") -Destination (Join-Path $releaseDir "Enable-LAN.ps1") -Force
Copy-Item -LiteralPath (Join-Path $PSScriptRoot "Izinkan-LAN.cmd") -Destination (Join-Path $releaseDir "Izinkan-LAN.cmd") -Force
Copy-Item -LiteralPath (Join-Path $PSScriptRoot "THIRD-PARTY-NOTICES.txt") -Destination (Join-Path $releaseDir "THIRD-PARTY-NOTICES.txt") -Force
@'
BOBODROP 1.4.1

1. Klik bobodrop.exe. Antarmuka terbuka di browser Windows.
2. Hubungkan laptop dan HP ke Wi-Fi yang sama, atau gunakan hotspot HP
   untuk laptop / hotspot Windows untuk HP. Tidak ada server cloud.
3. Scan QR dengan kamera; gunakan Safari iPhone/iPad atau Chrome Android.
   Buka tautan lengkap hasil scan, termasuk /s/ dan kode sesi. Jika halaman sambungan
   meminta QR, scan kode terbaru dari satu bobodrop yang sedang berjalan.
4. Kirim teks, foto, atau file dari kedua perangkat. Pilih Indonesia/English
   di header. Tab Panduan/Guide menjelaskan sambungan dan penggunaan.
5. Untuk speaker HP/tablet, buka Layar & suara pada laptop melalui Chrome/Edge
   biasa di luar panel Codex, melalui alamat 127.0.0.1 yang dibuka aplikasi.
   Pilih penerima, Suara saja dan Hanya speaker HP, lalu Mulai kirim.
   Audio diambil langsung dari Windows tanpa pemilih layar. Di HP/tablet,
   buka Layar & suara lalu ketuk Putar suara. Biarkan browser terbuka.
6. Untuk layar dan remote, pilih Layar. Centang izin kontrol bila diperlukan.
   HP menyediakan touchpad, klik, scroll, dan ketikan untuk laptop.
   Ketuk dua kali touchpad untuk klik kiri; tombol klik juga tetap tersedia.
   Hentikan berbagi melalui tombol Hentikan pada salah satu perangkat.
7. Jika layar belum tampil, ketuk Mulai tampilan. Buka Periksa kiriman
   untuk melihat sambungan media dan data yang benar-benar diterima.
   Status Tersambung di atas hanya menunjukkan koneksi aplikasi.
8. Foto/file yang diseret dari aplikasi Foto/Files iPhone dibaca dahulu.
   Status membaca dapat dibatalkan. Seretan iPhone/iPad dibatasi 32 MiB
   total antrean; gunakan Pilih file untuk file lebih besar (hingga 10 GiB).
   Jika Safari tidak memberikan file, pilih ulang melalui tombol pemilih.
9. Kiriman media menyediakan jalur TCP lokal melalui laptop. Kredensial
   jalur ini berlaku untuk pasangan yang sedang berbagi dan dicabut saat
   dihentikan. Tidak perlu relay internet atau izin mikrofon/kamera HP.
10. Tombol Gelap/Terang mengganti tema pada laptop atau HP. Pilihan disimpan
    di browser. Animasi mengikuti pengaturan Reduce Motion perangkat.
11. Salin teks masuk ke clipboard Windows berlaku untuk teks yang diterima
    lewat bobodrop. Copy di aplikasi HP lain saja belum mengirimnya.
    Tempel di tab Teks, lalu Kirim teks. Periksa status salinan di Pengaturan.
12. Pilih Hanya speaker HP untuk membisukan keluaran Windows selama berbagi.
    Angka volume tidak diturunkan. Kondisi mute sebelumnya dipulihkan saat
    berhenti; helper pemulihan tetap bekerja jika proses utama berhenti.
    Audio diambil dari aliran proses Windows, terpisah dari mute speaker.
    Perlu Windows build 20348 atau lebih baru. Jika pilihan ini dimatikan,
    pilih Seluruh layar dan Bagikan audio sistem pada pemilih Chrome/Edge.
13. Audio native memakai PCM stereo 48 kHz, lalu Opus stereo tanpa pemrosesan
    mikrofon. Periksa kiriman menampilkan jalur dan pengaturan yang tersedia.
    Opus tetap memakai kompresi. Panggilan aktif dan jalur Bluetooth komunikasi
    di HP dapat membatasi suara. Konten DRM atau audio eksklusif dapat menolak
    penangkapan. Perubahan perangkat/unmute dapat memerlukan waktu deteksi.
14. Tautan HTTP/HTTPS dalam riwayat teks dapat ditekan. Angka pada tab Teks,
    File dan Layar & suara menunjukkan kiriman baru dari perangkat lain.
    Buka tab tersebut untuk menghapus angka kiriman yang belum dilihat.
15. Di iPhone pada alamat HTTP lokal, ketuk Tempel teks, lalu tekan lama
    kolom dan pilih Tempel dari menu iOS. Draf dan posisi kursor dipertahankan.
    Windows tetap dapat membaca clipboard langsung dari tombol tersebut.
16. Antrean audio dibatasi dan disesuaikan untuk mengurangi penumpukan serta
    jeda singkat. Latensi dan kestabilan tetap bergantung pada jaringan/HP.

File diterima di Downloads\LocalDrop. Tutup aplikasi melalui klik kanan
ikon bobodrop pada system tray > Keluar. Menutup tab browser membiarkan
aplikasi tetap berjalan.

Jika HP/tablet tidak bisa terhubung, jalankan Izinkan-LAN.cmd. Windows akan
meminta izin administrator untuk aturan firewall bobodrop. Firewall
tetap aktif dan profil jaringan tidak diubah. Wi-Fi tamu/kantor dengan
client isolation dapat memerlukan jaringan lain.

Safari dapat membatasi clipboard pada HTTP: gunakan menu tempel iOS atau
pilih teks dan salin manual saat tombol salin otomatis tidak tersedia.
Riwayat teks dan daftar file dalam aplikasi berlaku selama sesi berjalan;
file yang sudah selesai tetap disimpan di Downloads sesudah aplikasi ditutup.

Gunakan Wi-Fi yang dipercaya. Transfer langsung melalui HTTP lokal;
token QR membatasi akses, tetapi lalu lintas tidak dienkripsi.
Media layar/suara memakai enkripsi WebRTC; sinyal dan kontrol masih memakai
HTTP/WS lokal. Relay media berada di laptop, melalui port TCP sementara.
Jika koneksi tetap gagal, buka Periksa kiriman pada kedua perangkat.
Gunakan hanya satu instance bobodrop dan scan QR milik instance tersebut.

Fitur suara menangkap audio digital, bukan membuat perangkat output baru
di Windows. Aplikasi sumber harus dapat memutar audio di Windows; pengambilan
native tidak bergantung pada mute speaker. Ada jeda sesuai jaringan. Safari
yang dikunci/dipindah ke
latar belakang dapat menghentikan pemutaran. Aplikasi/desktop Windows
dengan hak administrator dapat menolak kontrol remote.

Sumber dan petunjuk developer: folder proyek bobodrop, README.md.

ENGLISH

Open bobodrop.exe once. Pair an iPhone, iPad or Android device using the QR
code. Use the same reachable Wi-Fi network, a phone hotspot connected to the
laptop, or a Windows hotspot connected to the phone. Transfers use the local
network; there is no cloud server. Guest/campus isolation may block devices.
Open the complete scanned link, including /s/ and the session code. If asked to connect,
scan the current QR from the one running bobodrop instance.
Choose English in the header; open Guide for connection and usage instructions.

Text, photos and files can be sent from either device. Received files stay in
Downloads\LocalDrop. Copy received text to the Windows clipboard applies to
text SENT through bobodrop: paste the phone text into Text, then Send text. Merely
pressing Copy in another phone app does not send it. Settings shows copy status.
HTTP/HTTPS receipt links are clickable. Text, Files and Screen & audio show
counts for new arrivals from other devices until the corresponding tab is viewed.
On an iPhone over local HTTP, select Paste text, then long-press the composer
and choose Paste from iOS. The draft/caret are retained; Windows reads its
native clipboard directly. Audio queues now preserve short interruptions
and limit old audio; actual latency/stability still depend on the network/device.
Dropped iPhone/iPad files are read before upload, with a cancel action and a
32 MiB queue limit. Use Choose files for larger files, up to 10 GiB.

For phone audio, open the laptop page in ordinary Chrome/Edge through localhost
or 127.0.0.1. Select the receiver, Audio only and Phone speaker only, then
Start sharing. Audio comes directly from Windows; no screen picker is needed.
On the phone, select Play audio. Keep
Safari/Chrome open. Phone speaker only mutes Windows outputs temporarily,
preserves the volume number and restores their previous mute state on stop.
Native capture uses Windows process streams independently of speaker mute
and requires Windows build 20348 or later. If Phone speaker only is off,
choose Entire screen and Share system audio in the browser picker.
Native capture uses stereo 48 kHz PCM and stereo Opus compression; it is not
lossless. Calls, Bluetooth
communication routes, DRM and exclusive-mode audio can affect the result.

For screen and remote control, select Screen and explicitly allow mouse and
keyboard control. Double-tap the touchpad to left-click; dedicated click buttons
remain available. Stop on either device to end capture and control permission.
Full phone-device screen capture is not provided by this browser interface.
Light/Dark and language choices are local to each browser. Reduce Motion is
respected. Close from the Windows tray menu Quit; closing a browser tab leaves
the app running. Run only one bobodrop instance and scan its current QR.

Use a trusted local network. Text/files, pairing and controls use local HTTP/WS;
media uses WebRTC encryption. Phone browsers may restrict automatic clipboard
access over HTTP; use manual copy/paste when prompted. A locked/background
phone browser may stop media. Downloads persist; session history/links do not.
'@ | Set-Content -LiteralPath (Join-Path $releaseDir "Petunjuk.txt") -Encoding utf8
$releaseHash = Get-FileHash -LiteralPath (Join-Path $releaseDir "bobodrop.exe") -Algorithm SHA256
($releaseHash.Hash.ToLowerInvariant() + "  bobodrop.exe") | Set-Content -LiteralPath (Join-Path $releaseDir "SHA256.txt") -Encoding ascii
$releaseHash
