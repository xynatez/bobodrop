(() => {
  "use strict";

  const copy = {
    "app.title": ["bobodrop · Transfer Wi-Fi lokal", "bobodrop · Local Wi-Fi transfer"],
    "theme.light": ["Terang", "Light"], "theme.dark": ["Gelap", "Dark"],
    "theme.useLight": ["Gunakan tema terang", "Use light theme"], "theme.useDark": ["Gunakan tema gelap", "Use dark theme"],
    "action.folder": ["Buka folder", "Open folder"], "action.settings": ["Pengaturan", "Settings"],
    "action.retry": ["Coba lagi", "Try again"], "action.dismiss": ["Tutup pemberitahuan", "Dismiss notice"],
    "action.closeDialog": ["Tutup dialog", "Close dialog"], "action.copyLink": ["Salin tautan", "Copy link"],
    "action.copy": ["Salin", "Copy"], "action.copyText": ["Salin teks", "Copy text"],
    "action.all": ["Pilih semua", "Select all"], "action.done": ["Selesai", "Done"],
    "action.cancel": ["Batal", "Cancel"], "action.cancelUpload": ["Batalkan", "Cancel"],
    "connection.connecting": ["Menghubungkan…", "Connecting…"], "connection.reconnecting": ["Menyambungkan kembali…", "Reconnecting…"],
    "connection.online": ["Tersambung", "Connected"], "connection.ready": ["Siap menerima", "Ready to receive"],
    "connection.devices": ["{count} perangkat terhubung", "Connected devices: {count}"],
    "connection.offline": ["Tidak tersambung", "Not connected"], "connection.noNetwork": ["Tidak ada jaringan", "No network"],
    "connection.lost": ["Sambungan terputus", "Connection lost"], "connection.ended": ["Sesi berakhir", "Session ended"],
    "start.title": ["Menyiapkan ruang transfer", "Preparing your transfer desk"],
    "start.message": ["Menghubungkan ke bobodrop di laptop.", "Connecting to bobodrop on the laptop."],
    "start.rejoin": ["Sambungkan kembali", "Reconnect"],
    "start.expired": ["Sesi berakhir atau tautan tidak berlaku. Pindai kode QR baru di laptop. Gunakan jaringan Wi-Fi atau hotspot yang sama.", "The session ended or the link expired. Scan a new QR code on the laptop. Use the same Wi-Fi or hotspot network."],
    "start.offlineTitle": ["Laptop belum tersambung", "Laptop is not connected"],
    "start.offlineHelp": ["Pastikan bobodrop tetap terbuka di laptop dan kedua perangkat menggunakan jaringan Wi-Fi atau hotspot yang sama.", "Keep bobodrop open on the laptop and connect both devices to the same Wi-Fi or hotspot network."],
    "start.unsupportedTitle": ["Browser belum mendukung sambungan langsung", "This browser does not support live connections"],
    "start.unsupportedHelp": ["Buka bobodrop melalui Safari, Chrome, atau Edge versi terbaru.", "Open bobodrop in the latest Safari, Chrome, or Edge."],
    "connection.unsupported": ["Browser tidak didukung", "Browser not supported"],
    "pair.title": ["Sambungkan ponsel", "Connect your phone"],
    "pair.help": ["Pindai dengan kamera Android atau iPhone. Gunakan jaringan Wi-Fi atau hotspot yang sama.", "Scan with your Android or iPhone camera. Use the same Wi-Fi or hotspot network."],
    "pair.qrAlt": ["Kode QR untuk menyambungkan ponsel ke sesi bobodrop ini", "QR code to connect your phone to this bobodrop session"],
    "pair.qrError": ["Kode QR tidak tersedia.", "QR code is unavailable."],
    "pair.phone": ["Ponsel", "Phone"], "pair.address": ["Alamat Wi-Fi / IPv4", "Wi-Fi / IPv4 address"],
    "pair.noLAN": ["Tidak ada alamat LAN. Sambungkan laptop ke Wi-Fi atau hotspot agar ponsel dapat mengaksesnya.", "No LAN address. Connect the laptop to Wi-Fi or a hotspot so your phone can reach it."],
    "pair.joinLink": ["Tautan sambungan ponsel", "Phone connection link"],
    "pair.lanUnavailable": ["LAN belum tersedia", "LAN is unavailable"],
    "pair.devices": ["Perangkat terhubung", "Connected devices"],
    "pair.empty": ["Belum ada perangkat. Pindai kode QR dari ponsel untuk menyambungkan.", "No devices yet. Scan the QR code on your phone to connect."],
    "pair.device": ["Perangkat", "Device"], "pair.connectedAt": ["Tersambung {date}", "Connected {date}"],
    "transfer.label": ["Ruang transfer", "Transfer desk"], "tabs.label": ["Jenis transfer dan panduan", "Transfer types and guide"],
    "tabs.text": ["Teks", "Text"], "tabs.files": ["File", "Files"],
    "tabs.media": ["Layar & suara", "Screen & audio"], "tabs.guide": ["Panduan", "Guide"],
    "transfer.local": ["Wi-Fi lokal", "Local Wi-Fi"],
    "text.label": ["Tempel teks atau tautan", "Paste text or a link"],
    "text.help": ["Teks ini akan tersedia di perangkat yang tersambung.", "This text will be available on connected devices."],
    "text.placeholder": ["Ketik atau tempel teks dan tautan di sini…", "Type or paste text and links here…"],
    "text.send": ["Kirim teks", "Send text"], "text.sending": ["Mengirim…", "Sending…"],
    "text.paste": ["Tempel dari clipboard", "Paste from clipboard"],
    "text.hint": ["Teks tersimpan selama aplikasi berjalan.", "Text stays available while the app is running."],
    "text.history": ["Riwayat teks", "Text history"],
    "text.empty": ["Belum ada teks. Kirim teks atau tautan untuk melihatnya di sini.", "No text yet. Send text or a link to see it here."],
    "text.expand": ["Lihat seluruh teks", "Show full text"], "text.collapse": ["Tampilkan lebih sedikit", "Show less"],
    "file.dropTitle": ["Tarik file ke sini", "Drop files here"], "file.peerTitle": ["Kirim file ke laptop", "Send files to the laptop"],
    "file.help": ["Foto dan dokumen dikirim ke folder laptop.", "Photos and documents are sent to the laptop folder."],
    "file.choose": ["Pilih file", "Choose files"], "file.photo": ["Pilih foto", "Choose photos"],
    "file.limit": ["Maksimal 10 GB per file · Dikirim satu per satu", "Up to 10 GB per file · Sent one at a time"],
    "file.queue": ["Antrean kirim", "Upload queue"], "file.clear": ["Bersihkan selesai", "Clear finished"],
    "file.shared": ["File bersama", "Shared files"],
    "file.empty": ["Belum ada file. Gunakan Pilih file atau Pilih foto untuk mengirim.", "No files yet. Use Choose files or Choose photos to send them."],
    "file.download": ["Unduh", "Download"], "file.noDownload": ["Tautan unduh tidak tersedia.", "Download link is unavailable."],
    "file.photoDrop": ["Safari belum memberikan file foto. Gunakan Pilih foto untuk memilihnya dari galeri, lalu kirim lagi.", "Safari has not provided the photo file. Use Choose photos to select it from your gallery, then send it again."],
    "file.reconnect": ["Sambungkan kembali sebelum mengirim file.", "Reconnect before sending files."],
    "file.tooLarge": ["{name}: ukuran melebihi batas 10 GB.", "{name}: exceeds the 10 GB limit."],
    "file.waiting": ["Menunggu giliran", "Waiting in queue"], "file.cancelled": ["Dibatalkan", "Cancelled"],
    "file.progress": ["Mengirim… {percent}%", "Sending… {percent}%"],
    "file.saving": ["Menyimpan di laptop…", "Saving on the laptop…"], "file.sent": ["Terkirim", "Sent"],
    "file.progressLabel": ["Kemajuan kirim {name}", "Upload progress for {name}"],
    "file.expired": ["Sesi berakhir. Pindai kode QR baru.", "Session ended. Scan a new QR code."],
    "file.saveError": ["File belum tersimpan. Coba lagi.", "File has not been saved. Try again."],
    "file.networkError": ["File belum terkirim. Periksa sambungan; jika foto dari Safari tidak terbaca, gunakan Pilih foto lalu kirim lagi.", "File has not been sent. Check the connection; if Safari cannot read the photo, use Choose photos and send it again."],
    "file.readError": ["File tidak dapat dibaca atau dikirim. Gunakan Pilih foto untuk memilih ulang foto, atau Pilih file untuk file lainnya.", "File cannot be read or sent. Reselect it using Choose photos or Choose files."],
    "file.preparing": ["Membaca file dari seretan", "Reading dropped file"],
    "file.dropLimit": ["Seretan iPhone/iPad dibatasi 32 MiB dalam antrean. Pilih file untuk mengirim file yang lebih besar, atau selesaikan/batalkan antrean dahulu.", "iPhone/iPad drops are limited to 32 MiB in the queue. Use Choose files for larger files, or finish/cancel the queue first."],
    "file.dropError": ["Seret file asli dari perangkat, atau gunakan Pilih foto / Pilih file. Tautan dan teks tidak dapat dikirim sebagai file.", "Drag an actual file from your device, or use Choose photos / Choose files. Links and text cannot be sent as files."],
    "value.noDate": ["Waktu tidak tersedia", "Time unavailable"], "value.noSize": ["Ukuran tidak tersedia", "Size unavailable"],
    "error.disconnected": ["Sambungan ke laptop terputus.", "The connection to the laptop was lost."],
    "error.expired": ["Sesi berakhir. Pindai kode QR baru dari laptop.", "Session ended. Scan a new QR code on the laptop."],
    "error.request": ["Permintaan gagal ({status}). Coba lagi.", "Request failed ({status}). Try again."],
    "error.response": ["Jawaban server tidak dapat dibaca. Coba lagi.", "Cannot read the server response. Try again."],
    "error.state": ["Status perangkat tidak valid.", "Invalid device state."],
    "error.contact": ["Tidak dapat menghubungi laptop.", "Cannot reach the laptop."],
    "error.liveState": ["Status terbaru tidak dapat dibaca. Menyambungkan kembali…", "Cannot read the latest state. Reconnecting…"],
    "error.change": ["Perubahan gagal. Coba lagi.", "Change failed. Try again."],
    "clipboard.unavailable": ["Clipboard tidak tersedia", "Clipboard unavailable"],
    "clipboard.copied": ["Disalin ke clipboard.", "Copied to clipboard."],
    "clipboard.lastCopied": ["Teks masuk terakhir berhasil disalin ke clipboard Windows.", "The latest incoming text was copied to the Windows clipboard."],
    "clipboard.copyFailed": ["Teks diterima, tetapi belum dapat disalin ke clipboard Windows.", "Text received, but it could not be copied to the Windows clipboard."],
    "clipboard.manualCopy": ["Teks belum disalin. Tekan Pilih semua, lalu gunakan menu Salin di ponsel atau Ctrl+C di komputer.", "Text has not been copied. Tap Select all, then use your phone's Copy menu or Ctrl+C on your computer."],
    "clipboard.copyLabel": ["Teks untuk disalin", "Text to copy"],
    "clipboard.manualPaste": ["Tekan lama kolom teks, lalu pilih Tempel dari menu ponsel.", "Press and hold the text field, then choose Paste from your phone's menu."],
    "clipboard.empty": ["Clipboard kosong. Salin teks terlebih dahulu, lalu tekan Tempel.", "Clipboard is empty. Copy some text first, then tap Paste."],
    "clipboard.pasteError": ["Tempel otomatis tidak tersedia. Tekan lama kolom teks, lalu pilih Tempel. Di komputer, gunakan Ctrl+V.", "Automatic paste is unavailable. Press and hold the text field, then choose Paste. On a computer, use Ctrl+V."],
    "text.tooLarge": ["Teks terlalu besar. Maksimal 1 MB per kiriman.", "Text is too large. Up to 1 MB per message."],
    "text.sent": ["Teks terkirim.", "Text sent."], "text.clipboardError": ["Teks terkirim. {error}", "Text sent. {error}"],
    "text.sentCopied": ["Teks terkirim dan berhasil disalin ke clipboard Windows.", "Text sent and copied to the Windows clipboard."],
    "text.sendError": ["Teks belum terkirim. Coba lagi.", "Text has not been sent. Try again."],
    "settings.autoCopy": ["Salin teks masuk ke clipboard Windows", "Copy received text to the Windows clipboard"],
    "settings.autoCopyHelp": ["Menyalin teks setelah Anda mengirimnya melalui tab Teks di bobodrop. Menekan Salin di WhatsApp atau aplikasi HP lain tidak langsung mengirim clipboard ke Windows.", "Copies text after you send it through bobodrop's Text tab. Pressing Copy in WhatsApp or another phone app does not send its clipboard straight to Windows."],
    "settings.saved": ["Pengaturan disimpan.", "Settings saved."],
    "settings.storage": ["Folder penerimaan", "Receiving folder"], "settings.noStorage": ["Folder belum tersedia", "Folder unavailable"],
    "settings.history": ["Riwayat sesi", "Session history"], "settings.clear": ["Hapus riwayat teks", "Clear text history"],
    "settings.clearTitle": ["Hapus riwayat teks?", "Clear text history?"],
    "settings.clearHelp": ["Semua teks dalam sesi ini akan dihapus dari perangkat yang tersambung. File di folder laptop tetap tersedia.", "All text in this session will be removed from connected devices. Files in the laptop folder remain available."],
    "settings.clearConfirm": ["Hapus teks", "Clear text"], "settings.cleared": ["Riwayat teks dihapus.", "Text history cleared."],
    "settings.connections": ["Sambungan perangkat", "Device connections"],
    "settings.rotateHelp": ["Buat kode QR baru untuk mengakhiri akses dari tautan lama.", "Create a new QR code to end access through the old link."],
    "settings.disconnect": ["Putuskan perangkat", "Disconnect devices"],
    "settings.disconnectTitle": ["Putuskan semua perangkat?", "Disconnect all devices?"],
    "settings.disconnectHelp": ["Semua perangkat akan terputus dan tautan lama tidak berlaku lagi. Pindai kode QR baru untuk menyambungkan kembali.", "All devices will disconnect and the old link will expire. Scan the new QR code to reconnect."],
    "settings.disconnected": ["Perangkat diputuskan. Kode QR baru siap digunakan.", "Devices disconnected. The new QR code is ready."],
    "settings.folderOpened": ["Folder penerimaan dibuka di laptop.", "Receiving folder opened on the laptop."],
    "media.preparing": ["Menyiapkan sambungan…", "Preparing connection…"],
    "media.target": ["Perangkat penerima", "Receiving device"],
    "media.targetHelp": ["Pindai kode QR dengan Android atau iPhone terlebih dahulu.", "Scan the QR code with your Android or iPhone first."],
    "media.mode": ["Yang dikirim", "What to send"], "media.audioOnly": ["Suara saja", "Audio only"], "media.screen": ["Layar", "Screen"],
    "media.includeAudio": ["Sertakan suara laptop", "Include laptop audio"],
    "media.allowControl": ["Izinkan kontrol mouse dan keyboard", "Allow mouse and keyboard control"],
    "media.controlHelp": ["Perangkat penerima dapat menggerakkan mouse dan mengetik selama layar dibagikan.", "The receiving device can move the mouse and type while the screen is shared."],
    "media.start": ["Mulai kirim", "Start sharing"], "media.chooserTitle": ["Sumber layar dan suara", "Screen and audio source"],
    "media.phoneOnly": ["Hanya speaker HP", "Phone speaker only"],
    "media.phoneOnlyHelp": ["Matikan suara laptop selama kiriman tanpa mengubah angka volume Windows. Kondisi mute sebelumnya dipulihkan saat berhenti; statusnya tampil setelah kiriman dimulai.", "Silence the laptop during sharing without changing the Windows volume number. Its previous mute state is restored on stop; the result appears after sharing starts."],
    "media.chooserHelp": ["Suara saja dengan Hanya speaker HP memakai audio Windows langsung, tanpa pemilih layar. Untuk Layar, pilih layar yang dibagikan.", "Audio only with Phone speaker only uses Windows audio directly, without a screen chooser. For Screen, choose the screen to share."],
    "media.chooserPermission": ["Jika Hanya speaker HP dimatikan, pilih Seluruh layar dan aktifkan Bagikan audio sistem untuk mengirim suara melalui browser.", "If Phone speaker only is off, choose Entire screen and enable Share system audio to send sound through the browser."],
    "media.browserTitle": ["Browser dan keluaran suara", "Browser and audio output"],
    "media.browserHelp": ["Mulai dari Chrome/Edge biasa melalui localhost atau 127.0.0.1 di laptop Windows. Berbagi layar meminta izin browser; suara khusus HP memakai capture Windows.", "Start in regular Chrome/Edge through localhost or 127.0.0.1 on the Windows laptop. Screen sharing asks for browser permission; phone-only audio uses Windows capture."],
    "media.outputHelp": ["Hanya speaker HP menangkap audio Windows terpisah dari mute speaker. bobodrop tidak memasang perangkat audio virtual.", "Phone speaker only captures Windows audio separately from speaker mute. bobodrop does not install a virtual audio device."],
    "media.livePreparing": ["Menyiapkan kiriman", "Preparing share"], "media.stop": ["Hentikan", "Stop"],
    "media.videoLabel": ["Layar laptop yang dibagikan", "Shared laptop screen"], "media.playVideo": ["Mulai tampilan", "Start display"],
    "media.laptopAudio": ["Suara laptop", "Laptop audio"], "media.tapAudio": ["Ketuk Putar suara untuk mendengarkan.", "Tap Play audio to listen."],
    "media.playAudio": ["Putar suara", "Play audio"], "media.phoneHelpTitle": ["Mendengarkan di ponsel", "Listening on your phone"],
    "media.phoneHelp": ["Biarkan Chrome di Android atau Safari di iPhone tetap terbuka dan atur volume dengan tombol ponsel.", "Keep Chrome on Android or Safari on iPhone open and use your phone's volume buttons."],
    "media.backgroundHelp": ["Layar terkunci atau browser di latar belakang dapat menghentikan kiriman.", "Locking the screen or putting the browser in the background can stop the share."],
    "remote.title": ["Mouse & keyboard laptop", "Laptop mouse & keyboard"],
    "remote.padLabel": ["Touchpad relatif untuk mouse laptop", "Relative touchpad for the laptop mouse"],
    "remote.pad": ["Geser untuk menggerakkan mouse", "Swipe to move the mouse"],
    "remote.padHelp": ["Gerakan relatif, bukan posisi pada layar. Saat touchpad dipilih, tombol panah menggerakkan mouse.", "Movement is relative to the mouse, not a screen position. When the touchpad is focused, arrow keys move the mouse."],
    "remote.left": ["Klik kiri", "Left click"], "remote.right": ["Klik kanan", "Right click"],
    "remote.upLabel": ["Gulir laptop ke atas", "Scroll laptop up"], "remote.downLabel": ["Gulir laptop ke bawah", "Scroll laptop down"],
    "remote.up": ["Gulir ↑", "Scroll ↑"], "remote.down": ["Gulir ↓", "Scroll ↓"],
    "remote.type": ["Ketik untuk laptop", "Type for the laptop"], "remote.send": ["Kirim ketikan", "Send typing"],
    "remote.typeHelp": ["Maksimal 256 karakter per kiriman. Enter pada kolom ini hanya mengirim ketikan.", "Up to 256 characters per message. Enter in this field sends the typed text only."],
    "remote.keysLabel": ["Tombol keyboard laptop", "Laptop keyboard keys"],
    "remote.securityHelp": ["Kontrol mungkin tidak bekerja pada jendela administrator atau layar keamanan Windows.", "Control may not work in administrator windows or Windows security screens."],
    "media.troubleshootTitle": ["Suara laptop tidak terdengar?", "Cannot hear laptop audio?"],
    "media.troubleshoot": ["Putar suara di laptop. Untuk Hanya speaker HP, tunggu capture Windows siap, lalu ketuk Putar suara di ponsel dan atur volume ponsel. Biarkan Windows mute; tombol volume laptop dapat memicu mute ulang. Jika pilihan ini dimatikan, gunakan Seluruh layar dan Bagikan audio sistem pada pemilih browser. Akhiri panggilan aktif di ponsel sebelum mendengarkan musik.", "Play sound on the laptop. For Phone speaker only, wait for Windows capture to be ready, then tap Play audio and adjust your phone's volume. Leave Windows muted; laptop volume keys can trigger mute again. If this option is off, use Entire screen and Share system audio in the browser chooser. End active phone calls before listening to music."],
    "media.diagnosticsTitle": ["Periksa kiriman", "Inspect share"],
    "media.diagnosticsHelp": ["Pengamatan lokal di perangkat ini. Data terakhir tetap tersedia setelah kiriman berhenti, sampai kiriman baru dimulai.", "Local observations on this device. The latest data remains after a share stops, until a new share begins."],
    "media.noDiagnostics": ["Belum tersedia", "Not available yet"],
    "guide.title": ["Gunakan bobodrop di jaringan lokal", "Use bobodrop on your local network"],
    "guide.intro": ["bobodrop berjalan di laptop Windows. Ponsel membuka alamat lokal laptop melalui browser. Transfer tidak membutuhkan internet; kedua perangkat harus saling terjangkau di jaringan yang sama.", "bobodrop runs on a Windows laptop. Your phone opens the laptop's local address in its browser. Transfers do not need internet; both devices must be reachable on the same network."],
    "guide.networkTitle": ["Pilih satu sambungan", "Choose one connection"],
    "guide.wifiTitle": ["Wi-Fi yang sama", "Same Wi-Fi"],
    "guide.wifi": ["Sambungkan laptop dan ponsel ke router Wi-Fi yang sama. Hindari jaringan tamu yang memisahkan perangkat.", "Connect the laptop and phone to the same Wi-Fi router. Avoid guest networks that isolate devices."],
    "guide.phoneHotspotTitle": ["Hotspot ponsel → laptop", "Phone hotspot → laptop"],
    "guide.phoneHotspot": ["Aktifkan hotspot Android atau Personal Hotspot di iPhone, lalu sambungkan Wi-Fi laptop ke hotspot itu. Biarkan hotspot tetap aktif.", "Enable your Android hotspot or iPhone Personal Hotspot, then connect the laptop's Wi-Fi to it. Keep the hotspot active."],
    "guide.laptopHotspotTitle": ["Hotspot laptop → ponsel", "Laptop hotspot → phone"],
    "guide.laptopHotspot": ["Aktifkan Mobile hotspot di Pengaturan Windows bila tersedia, lalu sambungkan ponsel ke nama Wi-Fi laptop. Gunakan alamat antarmuka hotspot yang ditampilkan bobodrop.", "Enable Mobile hotspot in Windows Settings when available, then connect your phone to the laptop's Wi-Fi name. Use the hotspot interface address shown by bobodrop."],
    "guide.pairTitle": ["Sambungkan perangkat", "Pair your devices"],
    "guide.pair": ["Buka bobodrop di laptop. Pilih alamat Wi-Fi / IPv4 yang sesuai jaringan, lalu pindai kode QR dengan kamera ponsel. Di Android, buka dengan Chrome terbaru; di iPhone atau iPad, gunakan Safari terbaru. Jika QR tidak terbuka, salin atau ketik tautan sambungan di browser ponsel.", "Open bobodrop on the laptop. Select the Wi-Fi / IPv4 address for your network, then scan the QR code with your phone's camera. Use the latest Chrome on Android or Safari on iPhone or iPad. If the QR code does not open, copy or type the connection link in your phone's browser."],
    "guide.connectHelp": ["Jika gagal, pastikan Wi-Fi/hotspot tetap aktif dan izinkan bobodrop melalui Windows Firewall menggunakan Izinkan-LAN.cmd. Jika VPN atau isolasi perangkat memblokir jaringan lokal, gunakan jaringan yang mengizinkan sambungan antarperangkat. QR baru diperlukan setelah perangkat diputuskan dari Pengaturan.", "If it fails, keep Wi-Fi/hotspot active and allow bobodrop through Windows Firewall using Izinkan-LAN.cmd. If a VPN or device isolation blocks local access, use a network that allows device-to-device connections. Scan a new QR code after devices are disconnected in Settings."],
    "guide.textTitle": ["Teks dan clipboard", "Text and clipboard"],
    "guide.text": ["Di tab Teks, tempel atau ketik teks/tautan, lalu tekan Kirim teks. Teks muncul di riwayat kedua perangkat. Tekan Salin untuk menyalin teks yang diterima. Di Pengaturan laptop, aktifkan Salin teks masuk ke clipboard Windows agar kiriman juga bisa ditempel di aplikasi Windows.", "In Text, paste or type text/a link, then press Send text. It appears in both devices' history. Press Copy to copy received text. In laptop Settings, enable Copy received text to the Windows clipboard so you can also paste it into a Windows app."],
    "guide.clipboard": ["bobodrop tidak memantau setiap aksi Salin di iPhone atau clipboard saat Safari berada di latar belakang. Gunakan Kirim teks untuk mengirim. Jika Tempel dari clipboard tidak diizinkan browser, tekan lama kolom teks dan pilih Tempel; di laptop gunakan Ctrl+V.", "bobodrop does not watch every Copy action on iPhone or its clipboard while Safari is in the background. Use Send text to send it. If the browser blocks Paste from clipboard, press and hold the text field and choose Paste; on the laptop use Ctrl+V."],
    "guide.filesTitle": ["File dan foto", "Files and photos"],
    "guide.files": ["Di tab File, tekan Pilih file atau Pilih foto, atau seret file asli ke area kirim. File seretan iPhone/iPad dibaca dahulu, dengan batas total antrean 32 MiB. Untuk file lebih besar, gunakan Pilih file; antrean mengirim satu per satu, maksimal 10 GiB per file. File disimpan di folder penerimaan laptop dan dapat diunduh dari File bersama. Gunakan Batalkan atau Coba lagi bila diperlukan.", "In Files, choose files/photos or drag actual files into the upload area. iPhone/iPad drops are read first, with a total queue limit of 32 MiB. Use Choose files for larger files; the queue sends one at a time, up to 10 GiB per file. Files are saved in the laptop's receiving folder and can be downloaded from Shared files. Use Cancel or Try again as needed."],
    "guide.mediaTitle": ["Layar dan suara dari Windows", "Screen and audio from Windows"],
    "guide.media": ["Mulai dari Chrome atau Edge di laptop Windows melalui localhost atau 127.0.0.1. Pada tab Layar & suara, pilih penerima dan mode. Suara saja dengan Hanya speaker HP langsung menangkap audio Windows tanpa pemilih layar. Mode Layar meminta izin layar. Jika Hanya speaker HP dimatikan, pilih Seluruh layar dan Bagikan audio sistem untuk mengirim audio melalui browser.", "Start in Chrome or Edge on the Windows laptop through localhost or 127.0.0.1. In Screen & audio, choose the receiver and mode. Audio only with Phone speaker only captures Windows audio directly without a screen chooser. Screen mode asks for screen permission. If Phone speaker only is off, choose Entire screen and Share system audio for browser audio."],
    "guide.listen": ["Di ponsel, ketuk Putar suara dan gunakan tombol volume ponsel. Biarkan Chrome/Safari tetap terbuka; layar terkunci atau browser di latar belakang dapat menghentikan kiriman. Tekan Hentikan untuk mengakhiri. Setiap berbagi layar meminta izin browser lagi.", "On your phone, tap Play audio and use your phone's volume buttons. Keep Chrome/Safari open; screen lock or a background browser can stop the share. Press Stop to end it. Each screen share asks for browser permission again."],
    "guide.audioQuality": ["Audio Windows memakai stereo 48 kHz pada jalur Hanya speaker HP dan dikirim melalui WebRTC Opus. Kompresinya tetap lossy. Audio eksklusif, DRM, panggilan aktif, dan rute Bluetooth dapat membatasi penangkapan atau pemutaran.", "Windows audio uses 48 kHz stereo in Phone speaker only mode and travels through WebRTC Opus. Compression remains lossy. Exclusive audio, DRM, active calls and Bluetooth routes can limit capture or playback."],
    "guide.phoneOnly": ["Aktifkan Hanya speaker HP agar audio Windows ditangkap terpisah dari mute speaker laptop. Tunggu status keluaran dibisukan, lalu Putar suara di HP atau tablet. Hentikan mengembalikan kondisi mute sebelumnya. Biarkan Windows mute dan atur volume di ponsel. Matikan pilihan ini sebelum mulai bila laptop juga perlu bersuara. Jalur ini memerlukan Windows build 20348 atau lebih baru.", "Enable Phone speaker only to capture Windows audio separately from laptop speaker mute. Wait for muted output confirmation, then Play audio on your phone or tablet. Stop restores the previous mute state. Leave Windows muted and adjust your phone's volume. Disable this option before starting to hear the laptop too. This path requires Windows build 20348 or later."],
    "guide.remoteTitle": ["Kontrol laptop atas izin Anda", "Control the laptop with your permission"],
    "guide.remote": ["Sebelum membagikan Layar, centang Izinkan kontrol mouse dan keyboard. Ponsel terpilih dapat memakai touchpad relatif, klik, gulir, kirim ketikan, dan tombol keyboard. Touchpad yang dipilih juga menerima tombol panah. Hentikan kiriman untuk mengakhiri akses kontrol. Jendela administrator dan layar keamanan Windows dapat menolak kontrol.", "Before sharing Screen, enable Allow mouse and keyboard control. The selected phone can use the relative touchpad, click, scroll, send typing, and keyboard buttons. A focused touchpad also accepts arrow keys. Stop sharing to end control access. Administrator windows and Windows security screens may reject control."],
    "guide.recoveryTitle": ["Sambungan berubah atau gagal?", "Connection changed or failed?"],
    "guide.recovery": ["Jalankan satu bobodrop di laptop. Jika alamat berubah setelah berpindah Wi-Fi atau hotspot, pilih alamat baru dan pindai ulang QR. Jaringan kampus atau tamu dapat memisahkan perangkat. Hotspot tertentu memerlukan data seluler atau sambungan internet agar dapat diaktifkan, meskipun transfer bobodrop memakai jaringan lokal. Coba jaringan lain bila perangkat tidak saling terjangkau.", "Run one bobodrop instance on the laptop. If the address changes after switching Wi-Fi or hotspots, select the new address and scan its QR code again. Campus or guest networks may isolate devices. Some hotspots require mobile data or an internet connection to turn on, even though bobodrop transfers use the local network. Try another network if the devices cannot reach each other."],
    "server.address": ["Alamat bobodrop tidak dikenali. Buka alamat dari laptop.", "Unrecognized bobodrop address. Open the address shown on the laptop."],
    "server.origin": ["Permintaan dari situs lain ditolak.", "Requests from another site are rejected."],
    "server.closed": ["bobodrop sudah ditutup.", "bobodrop has closed."],
    "server.session": ["Sesi belum terhubung atau sudah berakhir. Scan QR terbaru di laptop.", "Session is not connected or has ended. Scan the latest QR code on the laptop."],
    "server.page": ["Halaman bobodrop tidak ditemukan.", "bobodrop page not found."],
    "server.link": ["Tautan ini sudah berakhir. Scan QR terbaru di laptop.", "This link has expired. Scan the latest QR code on the laptop."],
    "server.textEmpty": ["Isi teks sebelum mengirim.", "Enter text before sending."],
    "server.textSend": ["Teks belum dapat dikirim.", "Text could not be sent."],
    "server.ended": ["Sesi sudah berakhir. Scan QR terbaru di laptop.", "Session has ended. Scan the latest QR code on the laptop."],
    "server.autoCopy": ["Pilih pengaturan salin otomatis.", "Choose the auto-copy setting."],
    "server.newSession": ["Sesi baru belum dapat dibuat.", "Could not create a new session."],
    "server.clipboardUnavailable": ["Clipboard Windows tidak tersedia.", "Windows clipboard is unavailable."],
    "server.clipboardReceived": ["Teks diterima, tetapi clipboard Windows tidak tersedia.", "Text received, but the Windows clipboard is unavailable."],
    "server.clipboardWrite": ["Teks diterima, tetapi belum dapat disalin ke clipboard Windows: {detail}", "Text received, but it could not be copied to the Windows clipboard: {detail}"],
    "server.clipboardRead": ["Clipboard Windows belum dapat dibaca: {detail}", "Could not read the Windows clipboard: {detail}"],
    "server.folderUnavailable": ["Folder belum dapat dibuka dari perangkat ini.", "The folder cannot be opened from this device."],
    "server.folderOpen": ["Folder belum dapat dibuka: {detail}", "Could not open the folder: {detail}"],
    "server.method": ["Tindakan ini tidak tersedia untuk permintaan tersebut.", "This action is unavailable for that request."],
    "server.hostOnly": ["Tindakan ini hanya tersedia di laptop.", "This action is only available on the laptop."],
    "server.textLong": ["Teks terlalu panjang. Batasnya 1 MiB.", "Text is too long. The limit is 1 MiB."],
    "server.data": ["Data permintaan tidak valid.", "Invalid request data."],
    "server.fileChoose": ["Pilih satu file untuk dikirim.", "Choose one file to send."],
    "server.uploadData": ["Data unggahan tidak valid.", "Invalid upload data."],
    "server.oneFile": ["Kirim satu file dalam setiap unggahan.", "Send one file per upload."],
    "server.storageFolder": ["File belum dapat disimpan. Periksa folder penyimpanan.", "Could not save the file. Check the storage folder."],
    "server.fileSize": ["Ukuran file melebihi batas 10 GiB.", "File exceeds the 10 GiB limit."],
    "server.storageSpace": ["File belum dapat disimpan. Periksa ruang kosong di laptop.", "Could not save the file. Check free space on the laptop."],
    "server.fileRead": ["File belum dapat dibaca.", "Could not read the file."],
    "server.fileSave": ["File belum dapat disimpan.", "Could not save the file."],
    "server.storage": ["File belum dapat disimpan. Periksa folder dan ruang kosong di laptop.", "Could not save the file. Check the folder and free space on the laptop."],
    "server.uploadIncomplete": ["Unggahan terputus atau data file tidak lengkap. Kirim ulang file.", "Upload interrupted or file data incomplete. Send the file again."],
    "server.fileMissing": ["File tidak ditemukan dalam sesi ini.", "File not found in this session."],
    "server.fileGone": ["File sudah tidak tersedia di folder laptop.", "File is no longer available in the laptop folder."],
    "server.networkChoose": ["Pilih alamat jaringan yang tersedia di laptop.", "Choose an available network address on the laptop."],
    "server.qr": ["QR belum dapat dibuat.", "Could not create the QR code."],
    "server.connectionOrigin": ["Asal koneksi bobodrop tidak tersedia.", "bobodrop connection origin is unavailable."],
    "server.connection": ["Koneksi belum dapat dibuat.", "Could not create the connection."],
    "server.liveConnection": ["Koneksi langsung belum dapat dibuat. Muat ulang bobodrop.", "Could not create a live connection. Reload bobodrop."],
  };
  const dictionaries = { id: {}, en: {} };
  for (const [key, values] of Object.entries(copy)) { dictionaries.id[key] = values[0]; dictionaries.en[key] = values[1]; }
  let language = "id";
  try { if (localStorage.getItem("localdrop-language") === "en") language = "en"; } catch { /* Language remains selectable without storage. */ }
  const listeners = new Set();
  function t(key, params = {}) {
    const value = dictionaries[language][key] ?? dictionaries.id[key] ?? key;
    return value.replace(/\{(\w+)\}/g, (match, name) => Object.hasOwn(params, name) ? String(params[name]) : match);
  }
  function bind(element, key, params = {}, attribute = "textContent") {
    if (!element) return element;
    element.dataset.i18n = attribute === "textContent" ? key : element.dataset.i18n || "";
    if (attribute !== "textContent") element.setAttribute(`data-i18n-${attribute}`, key);
    if (Object.keys(params).length) element.dataset.i18nParams = JSON.stringify(params);
    else delete element.dataset.i18nParams;
    if (attribute === "textContent") element.textContent = t(key, params); else element.setAttribute(attribute, t(key, params));
    return element;
  }
  function apply(root = document) {
    for (const element of root.querySelectorAll("[data-i18n], [data-i18n-placeholder], [data-i18n-aria-label], [data-i18n-alt]")) {
      let params = {};
      try { params = JSON.parse(element.dataset.i18nParams || "{}"); } catch { /* Invalid optional parameters do not stop translation. */ }
      if (element.dataset.i18n) element.textContent = t(element.dataset.i18n, params);
      for (const attribute of ["placeholder", "aria-label", "alt"]) {
        const key = element.getAttribute(`data-i18n-${attribute}`);
        if (key) element.setAttribute(attribute, t(key, params));
      }
    }
    document.documentElement.lang = language;
    document.title = t("app.title");
    const select = document.getElementById("language-select");
    if (select) select.value = language;
  }
  function error(message) {
    if (typeof message !== "string") return t("error.change");
    for (const dictionary of Object.values(dictionaries)) {
      const key = Object.keys(dictionary).find((entry) => dictionary[entry] === message && !/\{\w+\}/.test(dictionary[entry]));
      if (key) return t(key);
    }
    for (const key of ["server.clipboardWrite", "server.clipboardRead", "server.folderOpen"]) {
      for (const dictionary of Object.values(dictionaries)) {
        const prefix = dictionary[key].split("{detail}")[0];
        if (message.startsWith(prefix)) return t(key, { detail: message.slice(prefix.length) });
      }
    }
    const request = /^(?:Permintaan gagal|Request failed) \((\d+)\)\. (?:Coba lagi|Try again)\.$/.exec(message);
    if (request) return t("error.request", { status: request[1] });
    return message;
  }
  function register(entries) {
    for (const lang of ["id", "en"]) Object.assign(dictionaries[lang], entries[lang] || {});
  }
  function setLanguage(next) {
    next = next === "en" ? "en" : "id";
    try { localStorage.setItem("localdrop-language", next); } catch { /* Keep this page's choice. */ }
    if (next === language) return;
    language = next;
    apply();
    for (const listener of listeners) listener(language);
    window.dispatchEvent(new CustomEvent("localdrop:languagechange", { detail: { language } }));
  }
  window.LocalDropI18n = Object.freeze({ t, bind, apply, error, register, setLanguage,
    onChange(callback) { listeners.add(callback); return () => listeners.delete(callback); },
    get language() { return language; }, get locale() { return language === "en" ? "en-US" : "id-ID"; },
  });
  apply();
  document.getElementById("language-select")?.addEventListener("change", (event) => setLanguage(event.target.value));
})();
