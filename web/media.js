(() => {
  "use strict";

  const KEYS = [
    ["enter", "Enter"], ["tab", "Tab"], ["backspace", "Backspace"], ["escape", "Esc"],
    ["arrowleft", "←"], ["arrowup", "↑"], ["arrowdown", "↓"], ["arrowright", "→"],
    ["delete", "Delete"], ["home", "Home"], ["end", "End"], ["pageup", "Page Up"], ["pagedown", "Page Down"],
    ["ctrl+a", "Ctrl+A"], ["ctrl+c", "Ctrl+C"], ["ctrl+v", "Ctrl+V"], ["ctrl+z", "Ctrl+Z"],
  ];
  const clamp = (value, limit) => Math.max(-limit, Math.min(limit, Math.round(value)));
  const TAP_DURATION = 250;
  const DOUBLE_TAP_INTERVAL = 400;
  const TAP_MOVEMENT = 10;
  const DOUBLE_TAP_DISTANCE = 24;
  const validText = (text) => text.length > 0 && [...text].length <= 256 && !/[\u0000-\u001f\u007f-\u009f]/u.test(text);
  const AUDIO_BITRATE = 256000;
  const musicAudioConstraints = () => ({ echoCancellation: false, noiseSuppression: false, autoGainControl: false, channelCount: { ideal: 2 }, sampleRate: { ideal: 48000 }, sampleSize: { ideal: 16 } });
  const MEDIA_COPY = {
    "Sambungan ke laptop terputus.": "The connection to the laptop was lost.",
    "Sambungan terputus. Mulai kiriman baru setelah tersambung.": "Connection lost. Start a new share after reconnecting.",
    "Sesi diganti. Mulai kiriman baru jika diperlukan.": "The session changed. Start a new share if needed.",
    "Perangkat penerima terputus. Pilih perangkat yang tersambung.": "The receiving device disconnected. Choose a connected device.",
    "Kiriman dihentikan.": "Sharing stopped.",
    "Kontrol belum dapat dijalankan.": "Control could not run. Try again.",
    "Kiriman belum dapat dimulai.": "Sharing could not start. Try again.",
    "Sambungan media gagal. Mulai kiriman baru.": "The media connection failed. Start a new share.",
    "Buka bobodrop di Chrome atau Edge pada laptop melalui alamat localhost atau 127.0.0.1.": "Open bobodrop in Chrome or Edge on the laptop at localhost or 127.0.0.1.",
    "Belum ada perangkat penerima. Pindai kode QR dengan ponsel.": "No receiving device yet. Scan the QR code with your phone.",
    "Pilih sumber pada pemilih berbagi Chrome atau Edge.": "Choose a source in the Chrome or Edge sharing chooser.",
    "Menyiapkan audio Windows untuk ponsel…": "Preparing Windows audio for your phone…",
    "Audio Windows siap. Menyiapkan perangkat penerima…": "Windows audio is ready. Preparing the receiving device…",
    "Audio Windows belum didukung browser ini. Buka Chrome atau Edge terbaru melalui localhost atau 127.0.0.1.": "This browser does not support Windows audio capture. Open the latest Chrome or Edge at localhost or 127.0.0.1.",
    "Audio Windows tidak tersedia di laptop ini.": "Windows audio capture is unavailable on this laptop.",
    "Audio Windows belum dapat diambil. Gunakan Windows build 20348 atau lebih baru, lalu mulai berbagi lagi.": "Windows audio could not be captured. Use Windows build 20348 or later, then start sharing again.",
    "Audio Windows terlalu lama disiapkan. Mulai berbagi lagi.": "Windows audio took too long to prepare. Start sharing again.",
    "Format audio Windows tidak didukung. Mulai berbagi lagi.": "The Windows audio format is unsupported. Start sharing again.",
    "Suara Windows terputus. Berbagi dihentikan.": "Windows audio was interrupted. Sharing stopped.",
    "Sumber audio Windows belum siap untuk perangkat ini. Mulai berbagi lagi.": "The Windows audio source is not ready for this device. Start sharing again.",
    "Suara khusus HP memerlukan sumber audio Windows. Mulai berbagi lagi.": "Phone-only audio requires a Windows audio source. Start sharing again.",
    "Audio Windows sedang digunakan. Hentikan kiriman sebelumnya.": "Windows audio is in use. Stop the previous share.",
    "Sumber audio Windows kedaluwarsa. Mulai berbagi lagi.": "The Windows audio source expired. Start sharing again.",
    "Audio sistem belum dibagikan. Pilih Seluruh layar dan aktifkan Bagikan audio sistem, lalu coba lagi.": "System audio was not shared. Choose Entire screen and enable Share system audio, then try again.",
    "Layar belum tersedia. Pilih sumber layar dan coba lagi.": "The screen is unavailable. Choose a screen source and try again.",
    "Layar akan dikirim tanpa suara. Untuk suara, hentikan lalu pilih Seluruh layar dan Bagikan audio sistem.": "The screen will be shared without audio. For audio, stop and choose Entire screen and Share system audio.",
    "Browser belum menerapkan semua pengaturan musik. Periksa kanal, laju sampel, dan codec pada Periksa kiriman.": "The browser could not apply all music settings. Check channels, sample rate, and codec in Inspect share.",
    "Berbagi dihentikan dari browser.": "Sharing was stopped in the browser.",
    "Menyiapkan perangkat penerima…": "Preparing the receiving device…",
    "Perangkat penerima belum menjawab. Mulai kiriman baru.": "The receiving device has not responded. Start a new share.",
    "Izin berbagi dibatalkan atau tidak diberikan. Klik Mulai kirim untuk mencoba lagi.": "Sharing permission was cancelled or denied. Press Start sharing to try again.",
    "Sumber layar atau suara tidak dapat dibaca. Tutup aplikasi yang memakai sumber tersebut, lalu coba lagi.": "The screen or audio source could not be read. Close applications using that source, then try again.",
    "Browser belum dapat mengambil layar atau suara.": "The browser could not capture the screen or audio. Try again.",
    "Membuat offer": "Creating offer", "Memasang offer lokal": "Setting local offer", "Offer dikirim": "Offer sent",
    "Kiriman sebelumnya dihentikan.": "The previous share stopped.",
    "Menunggu audio laptop…": "Waiting for laptop audio…",
    "Browser ini belum mendukung kiriman media. Gunakan Chrome terbaru di Android atau Safari terbaru di iPhone/iPad.": "This browser does not support media sharing. Use the latest Chrome on Android or Safari on iPhone/iPad.",
    "Windows belum mengonfirmasi pemulihan suara. Periksa pengaturan mute Windows sebelum berbagi lagi.": "Windows has not confirmed audio restoration. Check the Windows mute setting before sharing again.",
    "Konfigurasi sambungan media tidak valid.": "The media connection configuration is invalid.",
    "Peer dibuat": "Peer created", "Menyambungkan suara laptop…": "Connecting laptop audio…", "Menyambungkan layar dan suara…": "Connecting screen and audio…",
    "Aplikasi tersambung, tetapi kiriman media belum tersambung. Periksa apakah jaringan mengizinkan media antar perangkat, lalu coba lagi.": "The app is connected, but media has not connected. Check that the network allows media between devices, then try again.",
    "Kandidat sambungan belum dapat dikirim. Mulai kiriman baru.": "Connection candidates could not be sent. Start a new share.",
    "Kiriman tersambung ke perangkat penerima.": "Sharing connected to the receiving device.", "Kiriman laptop tersambung.": "Laptop sharing connected.",
    "Sambungan media terputus. Periksa Wi-Fi lalu mulai kiriman baru.": "The media connection was lost. Check Wi-Fi, then start a new share.",
    "Kiriman dari laptop sudah berakhir.": "Sharing from the laptop ended.",
    "Terlalu banyak kandidat sambungan. Mulai kiriman baru.": "Too many connection candidates. Start a new share.",
    "Menambahkan kandidat ICE": "Adding ICE candidate", "Kandidat ICE ditambahkan": "ICE candidate added",
    "Urutan sambungan media tidak valid.": "The media connection sequence is invalid.",
    "Memasang offer penerima": "Setting remote offer", "Memasang answer penerima": "Setting remote answer",
    "Membuat answer": "Creating answer", "Memasang answer lokal": "Setting local answer", "Answer dikirim": "Answer sent", "Answer terpasang": "Answer set",
    "Kesalahan pemutaran": "Playback error", "Dikirim": "Sent", "Diterima": "Received", "Belum tersedia": "Not available yet",
    "Kiriman dibatalkan. Tutup pemilih berbagi jika masih terbuka.": "Sharing cancelled. Close the sharing chooser if it is still open.",
    "Kontrol terlalu cepat. Tunggu sebentar lalu coba lagi.": "Control input is too fast. Wait a moment and try again.",
    "Sambungan kontrol terputus.": "The control connection was lost.",
    "Perangkat": "Device", "perangkat penerima": "receiving device", "Belum ada perangkat penerima": "No receiving device yet",
    "Menyiapkan sambungan…": "Preparing connection…",
    "Tidak tersambung. Kiriman media memerlukan kedua perangkat di Wi-Fi yang sama.": "Not connected. Media sharing requires both devices on the same Wi-Fi network.",
    "Browser belum mendukung layar dan suara. Gunakan Chrome/Edge di laptop, Chrome terbaru di Android, atau Safari terbaru di iPhone.": "This browser does not support screen and audio sharing. Use Chrome/Edge on the laptop, the latest Chrome on Android, or the latest Safari on iPhone.",
    "Buka Chrome atau Edge di laptop melalui localhost atau 127.0.0.1 untuk berbagi layar dan suara.": "Open Chrome or Edge on the laptop at localhost or 127.0.0.1 to share screen and audio.",
    "Kiriman aktif dari tab laptop lain. Hentikan di tab tersebut untuk memulai di sini.": "Sharing is active in another laptop tab. Stop it there to start here.",
    "Menunggu kiriman laptop…": "Waiting for laptop sharing…", "Laptop sedang mengirim ke perangkat lain.": "The laptop is sharing with another device.",
    "Pilih perangkat penerima, lalu mulai kirim.": "Choose a receiving device, then start sharing.",
    "Belum ada kiriman layar atau suara. Di laptop, pilih perangkat ini lalu Mulai kirim pada tab Layar & suara.": "No screen or audio share yet. On the laptop, choose this device and press Start sharing in Screen & audio.",
    "Gambar laptop tampil. Menunggu data suara dari laptop…": "The laptop image is displayed. Waiting for audio data from the laptop…",
    "Gambar laptop tampil. Ketuk Putar suara untuk mendengarkan.": "The laptop image is displayed. Tap Play audio to listen.", "Gambar laptop tampil.": "The laptop image is displayed.",
    "Gambar belum tampil. Ketuk Mulai tampilan. Jika tetap kosong, hentikan lalu pilih Seluruh layar di Chrome/Edge laptop.": "The image is not displayed yet. Tap Start display. If it stays blank, stop and choose Entire screen in Chrome/Edge on the laptop.",
    "Sambungan siap. Menunggu gambar dari laptop…": "Connection ready. Waiting for the laptop image…", "Sambungan siap. Menunggu data suara dari laptop…": "Connection ready. Waiting for audio data from the laptop…",
    "Pemutar suara berjalan. Atur volume dengan tombol ponsel.": "Audio playback is running. Use your phone's volume buttons.", "Sambungan siap. Ketuk Putar suara untuk mendengarkan.": "Connection ready. Tap Play audio to listen.",
    "Suara saja": "Audio only", "Layar": "Screen", "Menyiapkan kiriman": "Preparing share",
    "Laptop → {receiver} · {mode}": "Laptop → {receiver} · {mode}", "Laptop → perangkat ini · {mode}": "Laptop → this device · {mode}",
    "Jeda suara": "Pause audio", "Putar suara": "Play audio",
    "Pemutaran belum berhasil. Ketuk Putar suara lagi dan biarkan browser terbuka.": "Playback has not started. Tap Play audio again and keep the browser open.",
    "Menunggu track audio dari laptop…": "Waiting for the laptop audio track…", "Pemutaran siap. Menunggu data suara dari laptop…": "Playback ready. Waiting for audio data from the laptop…",
    "Ketuk Putar suara untuk mendengarkan.": "Tap Play audio to listen.", "Menunggu data suara. Ketuk Putar suara untuk menyiapkan pemutaran.": "Waiting for audio data. Tap Play audio to prepare playback.",
    "Kirim tombol {key} ke laptop": "Send the {key} key to the laptop",
    "Ketikan melebihi 256 karakter.": "Typed text exceeds 256 characters.", "Ketikan berisi karakter kontrol yang tidak didukung.": "Typed text contains unsupported control characters.", "Ketikan dikirim ke laptop.": "Typed text sent to the laptop.",
    "Kiriman dihentikan karena halaman ditutup.": "Sharing stopped because the page was closed.",
    "Menyiapkan suara hanya di ponsel. Menunggu konfirmasi keluaran Windows.": "Preparing phone-only audio. Waiting for Windows output confirmation.",
    "Keluaran suara Windows dibisukan selama kiriman ini. Putar suara di ponsel.": "Windows audio output is muted for this share. Play audio on your phone.",
    "Suara hanya di ponsel tidak aktif. Keluaran Windows mengikuti pengaturan sebelumnya.": "Phone-only audio is off. Windows output follows its previous settings.",
    "Suara khusus HP memerlukan audio sistem dan dukungan Windows.": "Phone-only audio requires system audio and Windows support.",
    "Speaker laptop belum dapat dibisukan. Mulai berbagi lagi.": "The laptop speakers could not be muted. Start sharing again.",
    "Suara khusus HP belum dapat diaktifkan.": "Phone-only audio could not be enabled. Try sharing again.",
    "Suara khusus HP terputus. Berbagi dihentikan.": "Phone-only audio was interrupted. Sharing stopped.",
    "Terlalu banyak permintaan. Sambungkan ulang bobodrop.": "Too many requests. Reconnect to bobodrop.",
    "Data koneksi tidak valid.": "Connection data is invalid.", "Data perangkat tidak valid.": "Device data is invalid.",
    "Sesi berbagi tidak tersedia untuk perangkat ini.": "The sharing session is unavailable for this device.", "Berbagi dihentikan.": "Sharing stopped.",
    "Tindakan koneksi tidak dikenali.": "The connection action is not recognized.", "Pilihan berbagi tidak valid.": "The sharing selection is invalid.",
    "Berbagi hanya dapat dimulai dari laptop.": "Sharing can only start from the laptop.", "Tunggu sebentar sebelum memulai berbagi lagi.": "Wait a moment before sharing again.",
    "Laptop sedang berbagi. Hentikan sesi yang aktif terlebih dahulu.": "The laptop is sharing. Stop the active session first.",
    "Pilih suara saja, atau layar dengan izin kontrol terpisah.": "Choose audio only, or screen with separate control permission.",
    "Kontrol mouse dan keyboard tidak tersedia di laptop ini.": "Mouse and keyboard control is unavailable on this laptop.",
    "Perangkat penerima sudah terputus. Pilih perangkat yang terhubung.": "The receiving device disconnected. Choose a connected device.",
    "Sesi berbagi belum dapat dibuat.": "The sharing session could not be created. Try again.",
    "Alamat penerima berubah. Sambungkan ulang dari kode QR laptop.": "The receiving address changed. Reconnect using the laptop's QR code.",
    "Jalur koneksi lokal belum dapat dibuat. Mulai berbagi lagi.": "The local connection could not be created. Start sharing again.",
    "Koneksi terputus. Berbagi dihentikan.": "Connection lost. Sharing stopped.",
    "Sinyal tidak sesuai dengan sesi berbagi perangkat ini.": "The signal does not match this device's sharing session.",
    "Terlalu banyak sinyal koneksi. Tunggu sebentar.": "Too many connection signals. Wait a moment.",
    "Perangkat terputus. Berbagi dihentikan.": "Device disconnected. Sharing stopped.",
    "Penawaran koneksi layar atau suara tidak valid.": "The screen or audio connection offer is invalid.",
    "Jawaban koneksi layar atau suara tidak valid.": "The screen or audio connection answer is invalid.",
    "Alamat koneksi layar atau suara tidak valid.": "The screen or audio connection address is invalid.",
    "Jenis sinyal koneksi tidak dikenali.": "The connection signal type is not recognized.",
    "Kontrol belum diizinkan untuk perangkat dan sesi ini.": "Control is not permitted for this device and session.",
    "Laptop terputus. Berbagi dihentikan.": "Laptop disconnected. Sharing stopped.",
    "Kontrol mouse atau keyboard tidak valid.": "Mouse or keyboard input is invalid.", "Kontrol terlalu cepat. Tunggu sebentar.": "Control input is too fast. Wait a moment.",
    "Kontrol belum dapat dikirim ke Windows: ": "Control could not be sent to Windows: ",
    "bobodrop ditutup. Berbagi dihentikan.": "bobodrop closed. Sharing stopped.", "Sesi diperbarui. Berbagi dihentikan.": "The session was renewed. Sharing stopped.",
    "{live} aktif / {muted} dibisukan": "{live} live / {muted} muted",
    "Sumber audio": "Audio source", "Sumber video": "Video source", "Track audio diterima": "Received audio tracks", "Track video diterima": "Received video tracks",
    "Jalur pengambilan audio": "Audio capture path", "Audio Windows langsung": "Native Windows audio", "Audio dari pemilih browser": "Browser sharing audio",
    "Sambungan media": "Media connection", "Pengumpulan ICE": "ICE gathering", "Deskripsi lokal / penerima": "Local / remote description", "Langkah negosiasi": "Negotiation step", "Kesalahan negosiasi": "Negotiation error",
    "Kandidat dikumpulkan / dikirim": "Candidates gathered / sent", "Kandidat diterima / ditambahkan": "Candidates received / added", "Kesalahan kandidat / kode terakhir": "Candidate errors / latest code",
    "Jalur lokal": "Local path", "Jalur penerima": "Remote path", "Arah statistik": "Statistics direction", "Byte audio": "Audio bytes", "Sampel audio diterima": "Received audio samples", "Byte video": "Video bytes", "Energi audio": "Audio energy",
    "Frame diterima": "Decoded frames", "Frame dikirim": "Encoded frames", "Frame": "Frames", "Pemutaran video": "Video playback", "Pemutaran audio": "Audio playback",
    "Pengaturan sumber audio": "Audio source settings", "Pengaturan audio diterima": "Received audio settings", "kanal": "channels", "laju sampel": "sample rate", "ukuran sampel": "sample size",
    "Pembatalan gema / pengurangan bising / penguatan otomatis": "Echo cancellation / noise suppression / automatic gain",
    "Kesalahan pengaturan musik / codec / batas pengirim": "Music settings / codec / sender limit errors", "Codec audio": "Audio codec", "Parameter Opus dinegosiasikan": "Negotiated Opus parameters",
    "Batas bitrate yang diminta": "Requested bitrate cap", "Bitrate audio terukur": "Measured audio bitrate", "Suara hanya di ponsel dikonfirmasi": "Phone-only audio confirmed",
    "Target buffer penerima": "Requested receiver buffer", "Buffer audio penerima (interval)": "Receiver audio buffer (interval)", "Sampel audio pengganti (interval)": "Concealed audio samples (interval)", "Paket audio hilang (interval)": "Lost audio packets (interval)",
    "Antrean audio native utama / pemutar": "Native audio main / playback queue", "Target antrean audio native": "Native audio queue target", "Frame audio native dibuang": "Discarded native audio frames", "Kekurangan audio native / frame": "Native audio underruns / frames", "Koreksi jam audio native": "Native audio clock correction",
    "Laju jam codec / kapasitas kanal": "Codec clock rate / channel capacity", "Jeda / kesiapan": "Paused / readiness", "Ukuran / jeda / kesiapan": "Dimensions / paused / readiness",
    "Ya": "Yes", "Tidak": "No", "Aktif": "On", "Nonaktif": "Off",
    "Baru": "New", "Menyambungkan": "Connecting", "Tersambung": "Connected", "Selesai": "Completed", "Terputus": "Disconnected", "Gagal": "Failed", "Ditutup": "Closed", "Stabil": "Stable", "Mengumpulkan": "Gathering", "Memeriksa": "Checking",
    "Offer lokal tersedia": "Local offer available", "Offer penerima tersedia": "Remote offer available", "Answer lokal sementara": "Local provisional answer", "Answer penerima sementara": "Remote provisional answer",
    "Offer": "Offer", "Answer": "Answer", "Answer sementara": "Provisional answer", "Batalkan negosiasi": "Rollback", "Lokal": "Local", "Refleksif server": "Server reflexive", "Refleksif peer": "Peer reflexive", "Relay": "Relay",
    "Enter": "Enter", "Tab": "Tab", "Hapus sebelumnya": "Backspace", "Esc": "Esc", "Hapus": "Delete", "Awal": "Home", "Akhir": "End", "Halaman atas": "Page Up", "Halaman bawah": "Page Down", "Panah kiri": "Left arrow", "Panah atas": "Up arrow", "Panah bawah": "Down arrow", "Panah kanan": "Right arrow",
  };
  window.LocalDropI18n?.register({
    id: { ...Object.fromEntries(Object.keys(MEDIA_COPY).map((key) => [key, key])), "remote.padGestureHelp": "Geser untuk menggerakkan mouse. Ketuk dua kali di touchpad untuk satu klik kiri. Gerakan relatif; tombol panah juga menggerakkan mouse saat touchpad dipilih." },
    en: { ...MEDIA_COPY, "remote.padGestureHelp": "Swipe to move the mouse. Double-tap the touchpad for one left click. Movement is relative; arrow keys also move the mouse when the touchpad is focused." },
  });
  const mediaText = (key, params = {}) => window.LocalDropI18n?.t(key, params) ?? key.replace(/\{(\w+)\}/g, (match, name) => Object.hasOwn(params, name) ? String(params[name]) : match);
  const mediaError = (message) => {
    const prefix = "Kontrol belum dapat dikirim ke Windows: ";
    if (message.startsWith(prefix)) return mediaText(prefix) + (window.LocalDropI18n?.error(message.slice(prefix.length)) ?? message.slice(prefix.length));
    return window.LocalDropI18n?.error(mediaText(message)) ?? mediaText(message);
  };
  const KEY_LABELS = { enter: "Enter", tab: "Tab", backspace: "Hapus sebelumnya", escape: "Esc", delete: "Hapus", home: "Awal", end: "Akhir", pageup: "Halaman atas", pagedown: "Halaman bawah", arrowleft: "Panah kiri", arrowup: "Panah atas", arrowdown: "Panah bawah", arrowright: "Panah kanan" };
  const DIAGNOSTIC_STATES = { new: "Baru", connecting: "Menyambungkan", connected: "Tersambung", completed: "Selesai", disconnected: "Terputus", failed: "Gagal", closed: "Ditutup", stable: "Stabil", gathering: "Mengumpulkan", complete: "Selesai", checking: "Memeriksa", "have-local-offer": "Offer lokal tersedia", "have-remote-offer": "Offer penerima tersedia", "have-local-pranswer": "Answer lokal sementara", "have-remote-pranswer": "Answer penerima sementara", offer: "Offer", answer: "Answer", pranswer: "Answer sementara", rollback: "Batalkan negosiasi", host: "Lokal", srflx: "Refleksif server", prflx: "Refleksif peer", relay: "Relay" };

  class LocalDropMedia {
    constructor({ send, selectTab }) {
      this.send = send;
      this.selectTab = selectTab;
      this.state = null;
      this.connected = false;
      this.connectionID = "";
      this.session = null;
      this.pending = null;
      this.source = null;
      this.nativeAudio = null;
      this.captureAbort = null;
      this.pc = null;
      this.remote = null;
      this.ice = [];
      this.epoch = 0;
      this.phase = "idle";
      this.error = "";
      this.warning = "";
      this.statusMessage = "";
      this.remoteStatus = "";
      this.signalChain = Promise.resolve();
      this.connectionTimer = null;
      this.startTimer = null;
      this.moveTimer = null;
      this.pointer = null;
      this.padPointers = new Set();
      this.lastTap = null;
      this.move = { dx: 0, dy: 0 };
      this.inputTokens = 8;
      this.inputTokenTime = performance.now();
      this.targetFingerprint = "";
      this.audioPlaying = false;
      this.mediaListeners = new Set();
      this.playbackListeners = { video: [], audio: [] };
      this.statsTimer = null;
      this.statsCall = null;
      this.videoFrameRequest = null;
      this.videoPlaying = false;
      this.videoStalled = false;
      this.videoFrameSeen = false;
      this.videoPlayAttempt = 0;
      this.audioPlayAttempt = 0;
      this.mediaConnectedAt = null;
      this.audioMeasurement = null;
      this.diagnostics = this.emptyDiagnostics();
      this.$ = (id) => document.getElementById(id);
      this.bind();
      window.LocalDropI18n?.onChange(() => this.render());
      this.render();
    }

    mode() { return document.querySelector('input[name="media-mode"]:checked')?.value || "audio"; }
    supported() { return typeof RTCPeerConnection !== "undefined" && typeof MediaStream !== "undefined"; }
    nativeSelected() { return this.state?.nativeAudio === true && (this.mode() === "audio" || this.$("media-include-audio").checked) && !!this.$("media-phone-only")?.checked; }
    captureSupported() { return this.supported() && window.isSecureContext && (!this.nativeSelected() || !!window.LocalDropNativeAudio?.supported()) && ((this.nativeSelected() && this.mode() === "audio") || !!navigator.mediaDevices?.getDisplayMedia); }

    nativeFailure(error) {
      if (Object.hasOwn(MEDIA_COPY, error?.message || "")) return error.message;
      return { unsupported: "Audio Windows belum didukung browser ini. Buka Chrome atau Edge terbaru melalui localhost atau 127.0.0.1.", format: "Format audio Windows tidak didukung. Mulai berbagi lagi.", timeout: "Audio Windows terlalu lama disiapkan. Mulai berbagi lagi.", interrupted: "Suara Windows terputus. Berbagi dihentikan." }[error?.code] || "Audio Windows belum dapat diambil. Gunakan Windows build 20348 atau lebih baru, lalu mulai berbagi lagi.";
    }

    sendMessage(message) {
      if (!this.connected) throw new Error("Sambungan ke laptop terputus.");
      this.send(message);
    }

    connectionChanged(connected) {
      this.connected = connected;
      if (!connected) {
        this.connectionID = "";
        if (this.session || this.pending || this.source) this.cleanup("Sambungan terputus. Mulai kiriman baru setelah tersambung.", false);
      }
      this.render();
    }

    stateChanged(state) {
      const rotated = this.state && this.state.sessionId !== state.sessionId;
      this.state = state;
      if (rotated) { this.cleanup("Sesi diganti. Mulai kiriman baru jika diperlukan.", false); this.resetDiagnostics(); }
      if (this.pending && !(state.devices || []).some((device) => device.connectionId === this.pending.target)) {
        this.cleanup("Perangkat penerima terputus. Pilih perangkat yang tersambung.", true);
      }
      // Snapshots are coalesced and may be older than reliable media events.
      // Stop the current stream only on its reliable stop event or local failure.
      this.render();
    }

    handleMessage(message) {
      if (message.type === "media-output-warning") {
        if (this.state?.role === "host") {
          this.warning = message.message || "Windows belum mengonfirmasi pemulihan suara. Periksa pengaturan mute Windows sebelum berbagi lagi.";
          this.render();
        }
        return;
      }
      if (message.type === "media-ready") { this.connectionID = message.connectionId || ""; this.render(); return; }
      if (message.type === "media-stopped") {
        if (this.session?.id === message.id) this.cleanup(message.reason || "Kiriman dihentikan.", false);
        return;
      }
      if (message.type === "media-error") {
        if (this.state?.role === "peer" && this.phase === "active" && this.session?.control) {
          this.remoteStatus = message.message || "Kontrol belum dapat dijalankan.";
          this.render();
        } else this.fail(message.message || "Kiriman belum dapat dimulai.");
        return;
      }
      const epoch = this.epoch;
      this.signalChain = this.signalChain.then(async () => {
        if (epoch !== this.epoch || !this.connected) {
          if (this.connected && this.state?.role === "host" && message.type === "media-started" && typeof message.id === "string" && message.id && message.id !== this.session?.id) {
            try { this.sendMessage({ type: "media-stop", id: message.id }); } catch { /* Server disconnect also releases the output lease. */ }
          }
          return;
        }
        if (message.type === "media-started") await this.started(message);
        else if (message.type === "media-invite") await this.invited(message);
        else if (message.type === "media-signal") await this.signal(message);
      }).catch((error) => {
        if (epoch !== this.epoch) return;
        this.diagnostics.negotiationException ||= this.exceptionName(error);
        this.fail("Sambungan media gagal. Mulai kiriman baru.");
      });
    }

    async start(event) {
      event.preventDefault();
      if (!this.state || this.state.role !== "host" || !this.connected || !this.connectionID || this.session || this.pending || this.state.media) return;
      if (!this.captureSupported()) { this.fail(this.nativeSelected() ? this.nativeFailure({ code: "unsupported" }) : "Buka bobodrop di Chrome atau Edge pada laptop melalui alamat localhost atau 127.0.0.1."); return; }
      const target = this.$("media-target").value;
      if (!(this.state.devices || []).some((device) => device.connectionId === target)) { this.fail("Belum ada perangkat penerima. Pindai kode QR dengan ponsel."); return; }
      const mode = this.mode();
      const audio = mode === "audio" || this.$("media-include-audio").checked;
      const control = mode === "screen" && this.$("media-allow-control").checked;
      const phoneOnly = audio && !!this.$("media-phone-only")?.checked;
      const native = phoneOnly && this.state.nativeAudio === true;
      const epoch = ++this.epoch;
      this.resetDiagnostics();
      this.diagnostics.audioPath = audio ? native ? "Audio Windows langsung" : "Audio dari pemilih browser" : null;
      this.pending = { target, mode, audio, control, phoneOnly, ...(native ? { nativeAudio: true } : {}) };
      const abort = this.captureAbort = new AbortController();
      const current = () => epoch === this.epoch && !!this.pending && this.connected && !abort.signal.aborted;
      this.phase = "capturing";
      this.error = "";
      this.warning = "";
      this.statusMessage = native && mode === "audio" ? "Menyiapkan audio Windows untuk ponsel…" : "Pilih sumber pada pemilih berbagi Chrome atau Edge.";
      this.render();
      try {
        // Invoke directly in the submit gesture, before any awaited operation.
        const display = native && mode === "audio" ? Promise.resolve(null) : Promise.resolve(navigator.mediaDevices.getDisplayMedia({
          video: { displaySurface: "monitor", width: { ideal: 1280 }, height: { ideal: 720 }, frameRate: { ideal: 15, max: 30 } },
          audio: audio && !native ? musicAudioConstraints() : false,
          systemAudio: audio && !native ? "include" : "exclude",
          selfBrowserSurface: "exclude",
          monitorTypeSurfaces: "include",
          surfaceSwitching: "exclude",
        })).then((captured) => {
          if (!current()) captured.getTracks().forEach((track) => track.stop());
          else this.source = captured;
          return captured;
        });
        const nativeSource = native ? window.LocalDropNativeAudio.open({ connectionId: this.connectionID, target, signal: abort.signal }).then((capture) => {
          if (!current()) { capture.close(); return null; }
          this.nativeAudio = capture;
          this.listenMedia(capture.stream, "native-audio-error", (event) => {
            if (epoch === this.epoch && this.nativeAudio === capture) this.fail(this.nativeFailure(event.error));
          });
          return capture;
        }) : Promise.resolve(null);
        const [screen, capture] = await Promise.all([display, nativeSource]);
        if (!current()) return;
        if (native && (!capture || !capture.stream.getAudioTracks().some((track) => track.readyState === "live"))) throw Object.assign(new Error("Suara Windows terputus. Berbagi dihentikan."), { name: "NativeAudioError" });
        if (native) screen?.getAudioTracks().forEach((track) => track.stop());
        const captured = native ? new MediaStream([...(screen?.getVideoTracks() || []), ...capture.stream.getAudioTracks()]) : screen;
        this.source = captured;
        const hasAudio = captured.getAudioTracks().some((track) => track.readyState === "live");
        if (mode === "audio" && !hasAudio) throw new Error("Audio sistem belum dibagikan. Pilih Seluruh layar dan aktifkan Bagikan audio sistem, lalu coba lagi.");
        if (mode === "screen" && !captured.getVideoTracks().some((track) => track.readyState === "live")) throw new Error("Layar belum tersedia. Pilih sumber layar dan coba lagi.");
        if (mode === "screen" && audio && !hasAudio) this.warning = "Layar akan dikirim tanpa suara. Untuk suara, hentikan lalu pilih Seluruh layar dan Bagikan audio sistem.";
        this.pending.audio = hasAudio && audio;
        this.pending.phoneOnly = this.pending.audio && phoneOnly;
        for (const track of captured.getTracks()) {
          this.listenMedia(track, "ended", () => { if (epoch === this.epoch && this.source === captured) this.cleanup("Berbagi dihentikan dari browser.", true); });
          for (const event of ["mute", "unmute"]) this.listenMedia(track, event, () => {
            if (epoch !== this.epoch || this.source !== captured) return;
            if (track.kind === "video" && this.$("media-video").srcObject) {
              this.videoStalled = !!track.muted;
              if (!track.muted) this.playVideo();
            }
            this.render();
          });
        }
        if (this.pending.audio) {
          for (const track of captured.getAudioTracks()) {
            try { track.contentHint = "music"; } catch { /* Optional content hint. */ }
            try { if (!native && track.applyConstraints) await track.applyConstraints(musicAudioConstraints()); }
            catch (error) {
              if (epoch !== this.epoch || this.source !== captured) return;
              this.diagnostics.audioConstraintException = this.exceptionName(error);
              this.warning = "Browser belum menerapkan semua pengaturan musik. Periksa kanal, laju sampel, dan codec pada Periksa kiriman.";
            }
            if (epoch !== this.epoch || this.source !== captured || !this.pending || !this.connected) return;
          }
        }
        this.phase = "waiting";
        this.statusMessage = native ? "Audio Windows siap. Menyiapkan perangkat penerima…" : "Menyiapkan perangkat penerima…";
        this.startTimer = setTimeout(() => { if (epoch === this.epoch) this.fail("Perangkat penerima belum menjawab. Mulai kiriman baru."); }, 15000);
        this.sendMessage({ type: "media-start", ...this.pending });
        this.render();
      } catch (error) {
        if (epoch !== this.epoch) return;
        const message = error.name === "NativeAudioError" ? this.nativeFailure(error) : error.name === "NotAllowedError" || error.name === "AbortError" ? "Izin berbagi dibatalkan atau tidak diberikan. Klik Mulai kirim untuk mencoba lagi." : error.name === "NotReadableError" ? "Sumber layar atau suara tidak dapat dibaca. Tutup aplikasi yang memakai sumber tersebut, lalu coba lagi." : Object.hasOwn(MEDIA_COPY, error.message) ? error.message : "Browser belum dapat mengambil layar atau suara.";
        this.fail(message);
      }
    }

    async started(message) {
      if (this.session?.id === message.id) return;
      if (this.state?.role !== "host" || !this.pending || this.phase !== "waiting" || !this.source || message.target !== this.pending.target) {
        if (message.id) this.sendMessage({ type: "media-stop", id: message.id });
        return;
      }
      clearTimeout(this.startTimer);
      this.startTimer = null;
      const iceServers = this.eventICEServers(message);
      const pending = this.pending;
      this.pending = null;
      this.session = { id: message.id, target: message.target, mode: message.mode, audio: !!message.audio, phoneOnly: !!message.audio && message.phoneOnly === true, nativeAudio: pending.nativeAudio === true, control: message.control, owner: true, iceServers };
      const pc = this.createPeer();
      // Browser audio capture retains its local video track without sending it.
      const tracks = pending.mode === "audio" ? this.source.getAudioTracks() : [...this.source.getVideoTracks(), ...(message.audio ? this.source.getAudioTracks() : [])];
      for (const track of tracks) pc.addTrack(track, this.source);
      this.configureAudioCodecs(pc);
      await this.configureAudioSenders(pc);
      if (!this.isCurrent(pc)) return;
      const offer = await this.negotiationStep(pc, "Membuat offer", () => pc.createOffer());
      if (!this.isCurrent(pc)) return;
      await this.negotiationStep(pc, "Memasang offer lokal", () => pc.setLocalDescription(this.musicDescription(offer)));
      if (!this.isCurrent(pc)) return;
      this.sendMessage({ type: "media-signal", id: message.id, kind: "offer", data: { type: pc.localDescription.type, sdp: pc.localDescription.sdp } });
      this.diagnostics.negotiationStep = "Offer dikirim";
      this.render();
    }

    async invited(message) {
      if (this.state?.role !== "peer") return;
      if (this.session?.id === message.id) return;
      if (this.session) this.cleanup("Kiriman sebelumnya dihentikan.", false);
      this.resetDiagnostics();
      const iceServers = this.eventICEServers(message);
      this.session = { id: message.id, mode: message.mode, audio: !!message.audio, phoneOnly: !!message.audio && message.phoneOnly === true, control: !!message.control, owner: false, receiverConnectionId: this.connectionID, iceServers };
      this.error = "";
      this.warning = "";
      this.audioPlaying = false;
      this.$("media-audio-status").textContent = mediaText("Menunggu audio laptop…");
      this.selectTab("media");
      if (!this.supported()) { this.fail("Browser ini belum mendukung kiriman media. Gunakan Chrome terbaru di Android atau Safari terbaru di iPhone/iPad."); return; }
      this.remote = new MediaStream();
      this.createPeer();
      this.render();
    }

    isCurrent(pc) { return this.pc === pc && !!this.session && this.connected; }

    musicDescription(description) {
      if (typeof description?.sdp !== "string") return description;
      const newline = description.sdp.includes("\r\n") ? "\r\n" : "\n";
      const sections = description.sdp.split(newline + "m=");
      for (let index = sections[0].startsWith("m=") ? 0 : 1; index < sections.length; index++) {
        const lines = (index === 0 ? sections[index] : "m=" + sections[index]).split(newline);
        const media = /^m=audio\s+(\d+)\s+\S+\s+(.+)$/.exec(lines[0]);
        if (!media || Number(media[1]) === 0) continue;
        const payloads = new Set(media[2].trim().split(/\s+/));
        const opus = lines.map((line) => /^a=rtpmap:(\d+)\s+opus\/48000\/2\s*$/i.exec(line)).filter((match) => match && payloads.has(match[1]));
        for (const match of opus) {
          const payload = match[1];
          const fmtp = new RegExp(`^a=fmtp:${payload}\\s+`, "i");
          const position = lines.findIndex((line) => fmtp.test(line));
          const parameters = new Map();
          if (position >= 0) for (const part of lines[position].replace(fmtp, "").split(";")) {
            const [name, ...value] = part.trim().split("=");
            if (name) parameters.set(name.toLowerCase(), value.join("="));
          }
          for (const [name, value] of Object.entries({ stereo: "1", "sprop-stereo": "1", maxaveragebitrate: String(AUDIO_BITRATE), usedtx: "0", useinbandfec: "1", maxplaybackrate: "48000" })) parameters.set(name, value);
          const line = `a=fmtp:${payload} ${[...parameters].map(([name, value]) => value ? `${name}=${value}` : name).join(";")}`;
          if (position >= 0) lines[position] = line;
          else lines.splice(lines.findIndex((line) => line === match[0]) + 1, 0, line);
        }
        sections[index] = index === 0 ? lines.join(newline) : lines.join(newline).slice(2);
      }
      return { type: description.type, sdp: sections.join(newline + "m=") };
    }

    configureAudioCodecs(pc) {
      if (!this.isCurrent(pc) || !this.session.audio) return;
      try {
        const codecs = typeof RTCRtpSender !== "undefined" ? RTCRtpSender.getCapabilities?.("audio")?.codecs : null;
        const opus = codecs?.filter((codec) => codec.mimeType?.toLowerCase() === "audio/opus");
        if (!opus?.length) return;
        const preferred = [...opus, ...codecs.filter((codec) => codec.mimeType?.toLowerCase() !== "audio/opus")];
        for (const transceiver of pc.getTransceivers?.() || []) {
          if ((transceiver.sender?.track?.kind || transceiver.receiver?.track?.kind) === "audio") transceiver.setCodecPreferences?.(preferred);
        }
      } catch (error) { if (this.isCurrent(pc)) this.diagnostics.audioCodecException = this.exceptionName(error); }
    }

    async configureAudioSenders(pc) {
      for (const sender of pc.getSenders?.() || []) {
        if (!this.isCurrent(pc)) return;
        if (sender.track?.kind !== "audio" || !sender.getParameters || !sender.setParameters) continue;
        try {
          const parameters = sender.getParameters();
          if (!parameters.encodings?.length) continue;
          for (const encoding of parameters.encodings) encoding.maxBitrate = AUDIO_BITRATE;
          await sender.setParameters(parameters);
          if (!this.isCurrent(pc)) return;
          this.diagnostics.senderAudioBitrate = sender.getParameters().encodings?.[0]?.maxBitrate ?? null;
        } catch (error) { if (this.isCurrent(pc)) this.diagnostics.audioSenderException = this.exceptionName(error); }
      }
    }

    eventICEServers(message) {
      try { return this.validatedICEServers(message.iceServers); }
      catch (error) {
        if (message.id && this.connected) this.sendMessage({ type: "media-stop", id: message.id });
        throw error;
      }
    }

    validatedICEServers(config) {
      // Only authenticated per-participant events may supply a local TCP relay.
      if (config === undefined) return [];
      const invalid = () => { throw new TypeError("Konfigurasi sambungan media tidak valid."); };
      if (!Array.isArray(config) || config.length !== 1) return invalid();
      const server = config[0];
      if (!server || typeof server !== "object" || Array.isArray(server) || Object.keys(server).some((key) => !["urls", "username", "credential"].includes(key))) return invalid();
      if (!Array.isArray(server.urls) || server.urls.length !== 1 || typeof server.urls[0] !== "string" || server.urls[0].length > 100) return invalid();
      const match = /^turn:(\d{1,3}(?:\.\d{1,3}){3}):([1-9]\d{0,4})\?transport=tcp$/.exec(server.urls[0]);
      const numericIPv4 = (host) => typeof host === "string" && /^\d{1,3}(?:\.\d{1,3}){3}$/.test(host) && host.split(".").every((part) => Number(part) <= 255 && String(Number(part)) === part);
      if (!match || !numericIPv4(match[1]) || Number(match[2]) > 65535) return invalid();
      const expected = this.state?.role === "host" ? "127.0.0.1" : window.location.hostname;
      if (!numericIPv4(expected) || match[1] !== expected) return invalid();
      if (typeof server.username !== "string" || !/^[\x21-\x7e]{1,128}$/.test(server.username) || typeof server.credential !== "string" || !/^[\x21-\x7e]{1,256}$/.test(server.credential)) return invalid();
      return [{ urls: [server.urls[0]], username: server.username, credential: server.credential }];
    }

    async negotiationStep(pc, step, operation) {
      if (!this.isCurrent(pc)) return;
      this.diagnostics.negotiationStep = step;
      this.render();
      try {
        const result = await operation();
        if (!this.isCurrent(pc)) return;
        return result;
      } catch (error) {
        if (!this.isCurrent(pc)) return;
        this.diagnostics.negotiationException = this.exceptionName(error);
        throw new Error("Sambungan media gagal. Mulai kiriman baru.");
      }
    }

    createPeer() {
      const pc = new RTCPeerConnection({ iceServers: this.session.iceServers, iceTransportPolicy: "all" });
      this.pc = pc;
      this.ice = [];
      Object.assign(this.diagnostics, { gatheredCandidates: 0, sentCandidates: 0, receivedCandidates: 0, addedCandidates: 0, candidateErrors: 0, negotiationStep: "Peer dibuat" });
      this.phase = "connecting";
      this.mediaConnectedAt = null;
      this.statusMessage = this.session.mode === "audio" ? "Menyambungkan suara laptop…" : "Menyambungkan layar dan suara…";
      this.connectionTimer = setTimeout(() => { if (this.isCurrent(pc) && this.phase !== "active") this.fail("Aplikasi tersambung, tetapi kiriman media belum tersambung. Periksa apakah jaringan mengizinkan media antar perangkat, lalu coba lagi."); }, 30000);
      this.listenMedia(pc, "icecandidate", (event) => {
        if (!this.isCurrent(pc)) return;
        if (!event.candidate) { this.snapshotDiagnostics(); this.render(); return; }
        this.diagnostics.gatheredCandidates++;
        try {
          this.sendMessage({ type: "media-signal", id: this.session.id, kind: "ice", data: event.candidate.toJSON ? event.candidate.toJSON() : event.candidate });
          this.diagnostics.sentCandidates++;
          this.render();
        } catch (error) { this.diagnostics.negotiationException = this.exceptionName(error); this.fail("Kandidat sambungan belum dapat dikirim. Mulai kiriman baru."); }
      });
      this.listenMedia(pc, "icecandidateerror", (event) => {
        if (!this.isCurrent(pc)) return;
        this.diagnostics.candidateErrors++;
        this.diagnostics.candidateErrorCode = Number.isInteger(event.errorCode) && event.errorCode >= 0 && event.errorCode <= 999 ? event.errorCode : null;
        this.render();
      });
      for (const name of ["signalingstatechange", "icegatheringstatechange"]) this.listenMedia(pc, name, () => { if (this.isCurrent(pc)) this.render(); });
      const updateConnection = () => {
        if (!this.isCurrent(pc)) return;
        const connectionState = pc.connectionState;
        const iceState = pc.iceConnectionState;
        if (connectionState === "connected" || (!connectionState && ["connected", "completed"].includes(iceState))) {
          clearTimeout(this.connectionTimer);
          this.connectionTimer = null;
          this.phase = "active";
          if (this.mediaConnectedAt === null) this.mediaConnectedAt = performance.now();
          this.statusMessage = this.session.owner ? "Kiriman tersambung ke perangkat penerima." : "Kiriman laptop tersambung.";
          this.render();
        } else if (["failed", "disconnected", "closed"].includes(connectionState) || ["failed", "disconnected", "closed"].includes(iceState)) {
          this.fail("Sambungan media terputus. Periksa Wi-Fi lalu mulai kiriman baru.");
        }
      };
      this.listenMedia(pc, "connectionstatechange", updateConnection);
      this.listenMedia(pc, "iceconnectionstatechange", updateConnection);
      this.listenMedia(pc, "track", (event) => {
        if (!this.isCurrent(pc) || this.session.owner) return;
        if (!this.remote.getTracks().includes(event.track)) {
          this.remote.addTrack(event.track);
          this.listenMedia(event.track, "ended", () => { if (this.isCurrent(pc)) this.cleanup("Kiriman dari laptop sudah berakhir.", true); });
          for (const name of ["mute", "unmute"]) this.listenMedia(event.track, name, () => {
            if (!this.isCurrent(pc) || !this.remote?.getTracks().includes(event.track)) return;
            if (event.track.kind === "video") {
              this.videoStalled = !!event.track.muted;
              if (!event.track.muted) this.playVideo();
            }
            this.render();
          });
        }
        if (event.track.kind === "video") this.attachVideo(new MediaStream(this.remote.getVideoTracks()));
        if (event.track.kind === "audio") {
          this.configureAudioReceiver(event.receiver);
          this.attachAudio(new MediaStream(this.remote.getAudioTracks()));
        }
        this.render();
      });
      this.listenMedia(document, "visibilitychange", () => {
        if (!this.isCurrent(pc) || document.hidden) return;
        if (this.$("media-video").srcObject) this.playVideo();
        this.render();
      });
      if (this.session.owner && this.session.mode === "screen") this.attachVideo(new MediaStream(this.source.getVideoTracks()));
      this.pollStats(pc, this.epoch, this.session);
      this.render();
      return pc;
    }

    async signal(message) {
      if (!this.session || message.id !== this.session.id || !this.pc) return;
      const pc = this.pc;
      if (message.kind === "ice") {
        if (!message.data?.candidate) return;
        this.diagnostics.receivedCandidates++;
        if (!pc.remoteDescription) {
          if (this.ice.length >= 128) throw new Error("Terlalu banyak kandidat sambungan. Mulai kiriman baru.");
          this.ice.push(message.data);
        } else {
          await this.negotiationStep(pc, "Menambahkan kandidat ICE", () => pc.addIceCandidate(message.data));
          if (!this.isCurrent(pc)) return;
          this.diagnostics.addedCandidates++;
          this.diagnostics.negotiationStep = "Kandidat ICE ditambahkan";
        }
        this.render();
        return;
      }
      if (message.kind !== (this.session.owner ? "answer" : "offer")) throw new Error("Urutan sambungan media tidak valid.");
      await this.negotiationStep(pc, message.kind === "offer" ? "Memasang offer penerima" : "Memasang answer penerima", () => pc.setRemoteDescription({ type: message.kind, sdp: message.data.sdp }));
      if (!this.isCurrent(pc)) return;
      const candidates = this.ice.splice(0);
      for (const candidate of candidates) {
        await this.negotiationStep(pc, "Menambahkan kandidat ICE", () => pc.addIceCandidate(candidate));
        if (!this.isCurrent(pc)) return;
        this.diagnostics.addedCandidates++;
      }
      if (!this.session.owner) {
        this.configureAudioCodecs(pc);
        const answer = await this.negotiationStep(pc, "Membuat answer", () => pc.createAnswer());
        if (!this.isCurrent(pc)) return;
        await this.negotiationStep(pc, "Memasang answer lokal", () => pc.setLocalDescription(this.musicDescription(answer)));
        if (!this.isCurrent(pc)) return;
        this.sendMessage({ type: "media-signal", id: this.session.id, kind: "answer", data: { type: pc.localDescription.type, sdp: pc.localDescription.sdp } });
        this.diagnostics.negotiationStep = "Answer dikirim";
      } else {
        this.diagnostics.negotiationStep = "Answer terpasang";
        await this.configureAudioSenders(pc);
        if (!this.isCurrent(pc)) return;
      }
      this.render();
    }

    listenMedia(target, name, listener) {
      target.addEventListener(name, listener);
      const dispose = () => { target.removeEventListener(name, listener); this.mediaListeners.delete(dispose); };
      this.mediaListeners.add(dispose);
      return dispose;
    }

    playbackContext(element) { return { epoch: this.epoch, session: this.session, stream: element.srcObject, element }; }
    currentPlayback(context) {
      return !!context.stream && !!context.session && this.connected && context.epoch === this.epoch && context.session === this.session && context.stream === context.element.srcObject;
    }

    sameTracks(first, second) {
      if (!first || !second) return false;
      const a = first.getTracks();
      const b = second.getTracks();
      return a.length === b.length && a.every((track) => b.includes(track));
    }

    clearPlaybackListeners(kind) {
      for (const dispose of this.playbackListeners[kind].splice(0)) dispose();
      if (kind === "video" && this.videoFrameRequest !== null) {
        this.$("media-video").cancelVideoFrameCallback?.(this.videoFrameRequest);
        this.videoFrameRequest = null;
      }
    }

    attachVideo(stream) {
      const video = this.$("media-video");
      if (!this.session || this.sameTracks(video.srcObject, stream)) return;
      this.clearPlaybackListeners("video");
      video.muted = true;
      video.playsInline = true;
      video.srcObject = stream;
      this.videoPlaying = false;
      this.videoStalled = stream.getVideoTracks().some((track) => track.muted);
      this.videoFrameSeen = false;
      const context = this.playbackContext(video);
      const listen = (name, listener) => this.playbackListeners.video.push(this.listenMedia(video, name, () => {
        if (this.currentPlayback(context)) listener();
      }));
      listen("playing", () => { this.videoPlaying = true; this.videoStalled = false; this.observeVideoFrame(); this.render(); });
      for (const name of ["pause", "waiting", "stalled"]) listen(name, () => {
        this.videoStalled = true;
        if (name === "pause") this.videoPlaying = false;
        this.render();
      });
      for (const name of ["loadedmetadata", "loadeddata", "canplay", "resize"]) listen(name, () => {
        this.observeVideoFrame();
        this.render();
        if (video.paused || !this.hasVideoImage()) this.playVideo();
      });
      listen("timeupdate", () => { const ready = this.hasVideoImage(); this.observeVideoFrame(); if (ready !== this.hasVideoImage()) this.render(); });
      listen("error", () => { ++this.videoPlayAttempt; this.videoStalled = true; this.diagnostics.videoException = "MediaError"; this.render(); });
      if (typeof video.requestVideoFrameCallback === "function") {
        const onFrame = () => {
          if (!this.currentPlayback(context)) return;
          this.videoFrameRequest = null;
          const ready = this.hasVideoImage();
          if (video.videoWidth > 0 && video.videoHeight > 0) {
            this.videoFrameSeen = true;
            this.videoStalled = false;
            this.videoPlaying = !video.paused;
          }
          if (ready !== this.hasVideoImage()) this.render();
          if (this.currentPlayback(context)) this.videoFrameRequest = video.requestVideoFrameCallback(onFrame);
        };
        this.videoFrameRequest = video.requestVideoFrameCallback(onFrame);
      }
      // Make the element and its retry action visible before asking Safari to play.
      this.$("media-video-wrap").hidden = false;
      this.$("media-play-video").hidden = false;
      this.render();
      this.playVideo();
    }

    observeVideoFrame() {
      const video = this.$("media-video");
      if (!this.session || !video.srcObject || video.videoWidth <= 0 || video.videoHeight <= 0) return;
      let frames = null;
      try { frames = video.getVideoPlaybackQuality?.().totalVideoFrames; } catch { /* Optional browser counter. */ }
      if ((Number.isFinite(frames) && frames > 0) || video.currentTime > 0) this.videoFrameSeen = true;
    }

    hasVideoImage() {
      const video = this.$("media-video");
      return !!this.session && !!video.srcObject && this.videoFrameSeen && this.videoPlaying && !this.videoStalled && !video.paused && video.videoWidth > 0 && video.videoHeight > 0 && video.srcObject.getVideoTracks().some((track) => track.readyState === "live" && !track.muted);
    }

    async playVideo() {
      const video = this.$("media-video");
      const context = this.playbackContext(video);
      if (!this.currentPlayback(context) || !context.stream.getVideoTracks().some((track) => track.readyState === "live")) return;
      const attempt = ++this.videoPlayAttempt;
      video.muted = true;
      video.playsInline = true;
      this.$("media-video-wrap").hidden = false;
      this.$("media-play-video").hidden = false;
      try {
        const play = video.play();
        await play;
        if (!this.currentPlayback(context) || attempt !== this.videoPlayAttempt) return;
        this.videoPlaying = !video.paused;
        this.videoStalled = context.stream.getVideoTracks().some((track) => track.muted);
        this.diagnostics.videoException = null;
        this.observeVideoFrame();
        this.render();
      } catch (error) {
        if (!this.currentPlayback(context) || attempt !== this.videoPlayAttempt) return;
        this.videoPlaying = false;
        this.diagnostics.videoException = this.exceptionName(error);
        this.render();
      }
    }

    attachAudio(stream) {
      const audio = this.$("media-audio");
      if (!this.session || this.sameTracks(audio.srcObject, stream)) return;
      this.clearPlaybackListeners("audio");
      audio.srcObject = stream;
      audio.muted = false;
      audio.volume = 1;
      this.audioPlaying = false;
      const context = this.playbackContext(audio);
      const listen = (name, listener) => this.playbackListeners.audio.push(this.listenMedia(audio, name, () => {
        if (this.currentPlayback(context)) listener();
      }));
      listen("playing", () => { this.audioPlaying = !audio.paused; this.diagnostics.audioException = null; this.render(); });
      for (const name of ["pause", "ended", "waiting", "stalled"]) listen(name, () => { if (name === "pause" || name === "ended") this.audioPlaying = false; this.render(); });
      for (const name of ["loadedmetadata", "canplay"]) listen(name, () => this.render());
      listen("error", () => {
        ++this.audioPlayAttempt;
        this.audioPlaying = false;
        this.diagnostics.audioException = "MediaError";
        this.render();
      });
      this.render();
    }

    async playAudio() {
      const audio = this.$("media-audio");
      const context = this.playbackContext(audio);
      if (!this.currentPlayback(context) || this.session.owner || !context.stream.getAudioTracks().some((track) => track.readyState === "live")) return;
      const attempt = ++this.audioPlayAttempt;
      if (this.audioPlaying) { audio.pause(); this.audioPlaying = false; this.render(); return; }
      audio.muted = false;
      audio.volume = 1;
      try {
        // Keep play() directly in the tap/click handler, before the first await.
        const play = audio.play();
        await play;
        if (!this.currentPlayback(context) || attempt !== this.audioPlayAttempt) return;
        this.audioPlaying = !audio.paused;
        this.diagnostics.audioException = null;
        this.render();
      } catch (error) {
        if (!this.currentPlayback(context) || attempt !== this.audioPlayAttempt) return;
        this.audioPlaying = false;
        this.diagnostics.audioException = this.exceptionName(error);
        this.render();
      }
    }

    exceptionName(error) {
      const allowed = ["NotAllowedError", "AbortError", "NotSupportedError", "InvalidStateError", "InvalidModificationError", "OverconstrainedError", "SecurityError", "NotReadableError", "OperationError", "TypeError", "MediaError", "Error"];
      return allowed.includes(error?.name) ? error.name : "Kesalahan pemutaran";
    }

    emptyDiagnostics() {
      return { captureAudio: null, captureVideo: null, incomingAudio: null, incomingVideo: null, captureAudioSettings: null, incomingAudioSettings: null, audioConstraintException: null, audioCodecException: null, audioSenderException: null, senderAudioBitrate: null, audioCodec: null, audioFmtp: null, audioBitrate: null, phoneOnly: null, connection: null, ice: null, signaling: null, gathering: null, localDescription: null, remoteDescription: null, gatheredCandidates: null, sentCandidates: null, receivedCandidates: null, addedCandidates: null, candidateErrors: null, candidateErrorCode: null, negotiationStep: null, negotiationException: null, localCandidateType: null, remoteCandidateType: null, localRelayProtocol: null, remoteRelayProtocol: null, dtls: null, direction: null, audioBytes: null, audioSamples: null, videoBytes: null, frames: null, audioEnergy: null, videoState: null, audioState: null, videoException: null, audioException: null };
    }

    resetDiagnostics() {
      this.diagnostics = this.emptyDiagnostics();
      this.videoFrameSeen = this.videoPlaying = this.videoStalled = false;
      this.mediaConnectedAt = null;
      this.audioMeasurement = null;
      this.renderDiagnostics();
    }

    configureAudioReceiver(receiver) {
      this.diagnostics.audioJitterTarget = null;
      try {
        if (!receiver || !("jitterBufferTarget" in receiver)) return;
        receiver.jitterBufferTarget = 40;
        if (receiver.jitterBufferTarget === 40) this.diagnostics.audioJitterTarget = 40;
      } catch { /* Keep the browser's default when the optional hint is unavailable. */ }
    }

    trackCounts(stream, kind) {
      if (!stream) return null;
      const tracks = stream.getTracks().filter((track) => track.kind === kind);
      return { live: tracks.filter((track) => track.readyState === "live").length, muted: tracks.filter((track) => track.muted).length };
    }

    audioSettings(stream) {
      const track = stream?.getAudioTracks().find((track) => track.readyState === "live");
      if (!track?.getSettings) return null;
      try {
        const settings = track.getSettings();
        return Object.fromEntries(["channelCount", "sampleRate", "sampleSize", "echoCancellation", "noiseSuppression", "autoGainControl"].map((key) => [key, typeof settings[key] === "boolean" || (typeof settings[key] === "number" && Number.isFinite(settings[key]) && settings[key] >= 0) ? settings[key] : null]));
      } catch { return null; }
    }

    opusParameters(value) {
      if (typeof value !== "string") return null;
      const allowed = /^(stereo|sprop-stereo|usedtx|useinbandfec|minptime|maxplaybackrate|maxaveragebitrate|sprop-maxcapturerate|cbr)\s*=\s*(\d{1,9})$/i;
      return value.split(";").map((part) => allowed.exec(part.trim())).filter(Boolean).map((match) => `${match[1].toLowerCase()}=${match[2]}`).join("; ") || null;
    }

    negotiatedOpusParameters(pc) {
      const answer = pc.localDescription?.type === "answer" ? pc.localDescription : pc.remoteDescription?.type === "answer" ? pc.remoteDescription : null;
      if (typeof answer?.sdp !== "string") return null;
      const sections = answer.sdp.split(/\r?\n(?=m=)/);
      for (const section of sections) {
        const media = /^m=audio\s+(\d+)\s+\S+\s+([^\r\n]+)/.exec(section);
        if (!media || Number(media[1]) === 0) continue;
        const payloads = new Set(media[2].trim().split(/\s+/));
        const opus = [...section.matchAll(/^a=rtpmap:(\d+)\s+opus\/48000\/2\s*$/gim)].find((match) => payloads.has(match[1]));
        if (!opus) continue;
        const parameters = new RegExp(`^a=fmtp:${opus[1]}\\s+([^\\r\\n]+)`, "im").exec(section);
        if (parameters) return this.opusParameters(parameters[1]);
      }
      return null;
    }

    snapshotDiagnostics() {
      if (!this.session && !this.pending && !this.source && !this.pc) return;
      const diagnostic = this.diagnostics;
      diagnostic.captureAudio = this.trackCounts(this.source, "audio");
      diagnostic.captureVideo = this.trackCounts(this.source, "video");
      diagnostic.incomingAudio = this.trackCounts(this.remote, "audio");
      diagnostic.incomingVideo = this.trackCounts(this.remote, "video");
      diagnostic.captureAudioSettings = this.audioSettings(this.source);
      diagnostic.incomingAudioSettings = this.audioSettings(this.remote);
      diagnostic.phoneOnly = this.session ? this.session.phoneOnly === true : null;
      if (this.nativeAudio?.stats) diagnostic.nativeAudio = this.nativeAudio.stats();
      if (this.pc) {
        diagnostic.connection = this.pc.connectionState || null;
        diagnostic.ice = this.pc.iceConnectionState || null;
        diagnostic.signaling = this.pc.signalingState || null;
        diagnostic.gathering = this.pc.iceGatheringState || null;
        const type = (description) => ["offer", "answer", "pranswer", "rollback"].includes(description?.type) ? description.type : null;
        diagnostic.localDescription = type(this.pc.localDescription);
        diagnostic.remoteDescription = type(this.pc.remoteDescription);
        diagnostic.audioFmtp = this.negotiatedOpusParameters(this.pc);
      }
      const video = this.$("media-video");
      const audio = this.$("media-audio");
      if (video.srcObject) diagnostic.videoState = { width: video.videoWidth, height: video.videoHeight, paused: video.paused, readyState: video.readyState };
      if (audio.srcObject) diagnostic.audioState = { paused: audio.paused, readyState: audio.readyState };
    }

    async pollStats(pc, epoch, session) {
      if (this.pc !== pc || epoch !== this.epoch || session !== this.session || !this.session) return;
      // A new session also waits for a previous session's in-flight measurement.
      if (this.statsCall) {
        try { await this.statsCall; } catch { /* Statistics may be unavailable. */ }
        if (this.pc !== pc || epoch !== this.epoch || session !== this.session) return;
      }
      try {
        const call = typeof pc.getStats === "function" ? Promise.resolve().then(() => pc.getStats()) : Promise.resolve(null);
        this.statsCall = call;
        let report;
        try { report = await call; }
        finally { if (this.statsCall === call) this.statsCall = null; }
        if (this.pc !== pc || epoch !== this.epoch || session !== this.session) return;
        const rows = report && typeof report.values === "function" ? [...report.values()] : [];
        const direction = session.owner ? "outbound-rtp" : "inbound-rtp";
        const rtp = (kind) => rows.filter((row) => row.type === direction && (row.kind || row.mediaType) === kind && !row.isRemote);
        const sum = (selected, field) => {
          const values = selected.map((row) => row[field]).filter((value) => typeof value === "number" && Number.isFinite(value) && value >= 0);
          return values.length ? values.reduce((total, value) => total + value, 0) : null;
        };
        this.diagnostics.direction = session.owner ? "Dikirim" : "Diterima";
        this.diagnostics.audioBytes = sum(rtp("audio"), session.owner ? "bytesSent" : "bytesReceived");
        const audioRows = rtp("audio");
        const codec = rows.find((row) => row.type === "codec" && row.id === audioRows[0]?.codecId);
        this.diagnostics.audioCodec = codec && typeof codec.mimeType === "string" && /^audio\/[a-z0-9.-]{1,32}$/i.test(codec.mimeType) ? { mimeType: codec.mimeType, clockRate: Number.isFinite(codec.clockRate) ? codec.clockRate : null, channels: Number.isFinite(codec.channels) ? codec.channels : null } : null;
        const timestamp = audioRows.length && audioRows.every((row) => Number.isFinite(row.timestamp)) ? Math.max(...audioRows.map((row) => row.timestamp)) : null;
        const measurement = { bytes: this.diagnostics.audioBytes, timestamp, ids: JSON.stringify(audioRows.map((row) => row.id).sort()), jitterDelay: sum(audioRows, "jitterBufferDelay"), jitterEmitted: sum(audioRows, "jitterBufferEmittedCount"), concealed: sum(audioRows, "concealedSamples"), lost: sum(audioRows, "packetsLost") };
        const previous = this.audioMeasurement;
        this.diagnostics.audioBitrate = previous && measurement.ids === previous.ids && Number.isFinite(measurement.bytes) && Number.isFinite(previous.bytes) && Number.isFinite(timestamp) && Number.isFinite(previous.timestamp) && timestamp > previous.timestamp && measurement.bytes >= previous.bytes ? (measurement.bytes - previous.bytes) * 8000 / (timestamp - previous.timestamp) : null;
        const delta = (field) => !session.owner && previous && measurement.ids === previous.ids && Number.isFinite(timestamp) && timestamp > previous.timestamp && Number.isFinite(measurement[field]) && Number.isFinite(previous[field]) && measurement[field] >= previous[field] ? measurement[field] - previous[field] : null;
        const jitterDelay = delta("jitterDelay"), jitterEmitted = delta("jitterEmitted");
        this.diagnostics.audioJitterMs = jitterDelay !== null && jitterEmitted > 0 ? jitterDelay * 1000 / jitterEmitted : null;
        this.diagnostics.audioConcealed = delta("concealed");
        this.diagnostics.audioLost = delta("lost");
        this.audioMeasurement = measurement;
        this.diagnostics.audioSamples = session.owner ? null : sum(rtp("audio"), "totalSamplesReceived");
        this.diagnostics.videoBytes = sum(rtp("video"), session.owner ? "bytesSent" : "bytesReceived");
        this.diagnostics.frames = sum(rtp("video"), session.owner ? "framesEncoded" : "framesDecoded");
        this.diagnostics.audioEnergy = sum(session.owner ? rows.filter((row) => row.type === "media-source" && (row.kind || row.mediaType) === "audio") : rtp("audio"), "totalAudioEnergy");
        const transports = rows.filter((row) => row.type === "transport");
        const transport = transports.find((row) => row.selectedCandidatePairId) || transports[0];
        const pair = rows.find((row) => row.type === "candidate-pair" && row.id === transport?.selectedCandidatePairId) || rows.find((row) => row.type === "candidate-pair" && row.state === "succeeded" && (row.nominated || row.selected));
        const local = pair && rows.find((row) => row.type === "local-candidate" && row.id === pair.localCandidateId);
        const remote = pair && rows.find((row) => row.type === "remote-candidate" && row.id === pair.remoteCandidateId);
        const candidateType = (row) => ["host", "srflx", "prflx", "relay"].includes(row?.candidateType) ? row.candidateType : null;
        const relayProtocol = (row) => row?.candidateType === "relay" && ["tcp", "udp", "tls"].includes(row.relayProtocol) ? row.relayProtocol : null;
        this.diagnostics.localCandidateType = candidateType(local);
        this.diagnostics.remoteCandidateType = candidateType(remote);
        this.diagnostics.localRelayProtocol = relayProtocol(local);
        this.diagnostics.remoteRelayProtocol = relayProtocol(remote);
        this.diagnostics.dtls = ["new", "connecting", "connected", "closed", "failed"].includes(transport?.dtlsState) ? transport.dtlsState : null;
      } catch {
        if (this.pc !== pc || epoch !== this.epoch || session !== this.session) return;
        this.diagnostics.audioBytes = this.diagnostics.audioSamples = this.diagnostics.videoBytes = this.diagnostics.frames = this.diagnostics.audioEnergy = null;
        this.diagnostics.audioCodec = this.diagnostics.audioBitrate = null;
        this.diagnostics.audioJitterMs = this.diagnostics.audioConcealed = this.diagnostics.audioLost = null;
        this.audioMeasurement = null;
        this.diagnostics.localCandidateType = this.diagnostics.remoteCandidateType = this.diagnostics.localRelayProtocol = this.diagnostics.remoteRelayProtocol = this.diagnostics.dtls = null;
      }
      if (this.pc !== pc || epoch !== this.epoch || session !== this.session) return;
      this.observeVideoFrame();
      this.render();
      // Schedule after completion, so getStats calls never overlap in a session.
      this.statsTimer = setTimeout(() => { this.statsTimer = null; this.pollStats(pc, epoch, session); }, 1000);
    }

    hasInboundAudioData() {
      return this.phase === "active" && !!this.session && !this.session.owner && this.diagnostics.direction === "Diterima" && [this.diagnostics.audioBytes, this.diagnostics.audioSamples, this.diagnostics.audioEnergy].some((value) => Number.isFinite(value) && value > 0);
    }

    renderDiagnostics() {
      const diagnostic = this.diagnostics;
      const unavailable = mediaText("Belum tersedia");
      const counts = (value) => value ? mediaText("{live} aktif / {muted} dibisukan", value) : unavailable;
      const number = (value) => typeof value === "number" && Number.isFinite(value) ? new Intl.NumberFormat(window.LocalDropI18n?.locale || "id-ID", { maximumFractionDigits: 6 }).format(value) : unavailable;
      const boolean = (value) => typeof value === "boolean" ? mediaText(value ? "Ya" : "Tidak") : unavailable;
      const state = (value) => value ? mediaText(DIAGNOSTIC_STATES[value] || value) : unavailable;
      const row = (label, value) => `${mediaText(label)}: ${value}`;
      const settings = (value) => value ? `${mediaText("kanal")}=${number(value.channelCount)} / ${mediaText("laju sampel")}=${number(value.sampleRate)} Hz / ${mediaText("ukuran sampel")}=${number(value.sampleSize)} bit` : unavailable;
      const video = diagnostic.videoState;
      const audio = diagnostic.audioState;
      const processing = diagnostic.captureAudioSettings;
      const codec = diagnostic.audioCodec;
      this.$("media-diagnostics-values").textContent = [
        row("Sumber audio", counts(diagnostic.captureAudio)),
        row("Jalur pengambilan audio", diagnostic.audioPath ? mediaText(diagnostic.audioPath) : unavailable),
        row("Sumber video", counts(diagnostic.captureVideo)),
        row("Track audio diterima", counts(diagnostic.incomingAudio)),
        row("Track video diterima", counts(diagnostic.incomingVideo)),
        row("Pengaturan sumber audio", settings(processing)),
        row("Pengaturan audio diterima", settings(diagnostic.incomingAudioSettings)),
        row("Pembatalan gema / pengurangan bising / penguatan otomatis", processing ? `${boolean(processing.echoCancellation)} / ${boolean(processing.noiseSuppression)} / ${boolean(processing.autoGainControl)}` : unavailable),
        row("Kesalahan pengaturan musik / codec / batas pengirim", `${mediaText(diagnostic.audioConstraintException || unavailable)} / ${mediaText(diagnostic.audioCodecException || unavailable)} / ${mediaText(diagnostic.audioSenderException || unavailable)}`),
        row("Codec audio", codec?.mimeType || unavailable),
        row("Laju jam codec / kapasitas kanal", codec ? `${number(codec.clockRate)} Hz / ${number(codec.channels)}` : unavailable),
        row("Parameter Opus dinegosiasikan", diagnostic.audioFmtp || unavailable),
        row("Batas bitrate yang diminta", `${number(diagnostic.senderAudioBitrate)} bit/s`),
        row("Bitrate audio terukur", `${number(diagnostic.audioBitrate)} bit/s`),
        row("Target buffer penerima", `${number(diagnostic.audioJitterTarget)} ms`),
        row("Buffer audio penerima (interval)", `${number(diagnostic.audioJitterMs)} ms`),
        row("Sampel audio pengganti (interval)", number(diagnostic.audioConcealed)),
        row("Paket audio hilang (interval)", number(diagnostic.audioLost)),
        row("Antrean audio native utama / pemutar", `${number(diagnostic.nativeAudio?.mainQueueMs)} / ${number(diagnostic.nativeAudio?.workletQueueMs)} ms`),
        row("Target antrean audio native", `${number(diagnostic.nativeAudio?.targetMs)} ms`),
        row("Frame audio native dibuang", number(diagnostic.nativeAudio ? diagnostic.nativeAudio.discardedFrames + (diagnostic.nativeAudio.workletDiscardedFrames || 0) : null)),
        row("Kekurangan audio native / frame", `${number(diagnostic.nativeAudio?.underruns)} / ${number(diagnostic.nativeAudio?.underrunFrames)}`),
        row("Koreksi jam audio native", `${number(diagnostic.nativeAudio?.driftPpm)} ppm`),
        row("Suara hanya di ponsel dikonfirmasi", boolean(diagnostic.phoneOnly)),
        row("Sambungan media", state(diagnostic.connection)),
        row("ICE", state(diagnostic.ice)),
        row("Signaling", state(diagnostic.signaling)),
        row("Pengumpulan ICE", state(diagnostic.gathering)),
        row("Deskripsi lokal / penerima", `${state(diagnostic.localDescription)} / ${state(diagnostic.remoteDescription)}`),
        row("Langkah negosiasi", diagnostic.negotiationStep ? mediaText(diagnostic.negotiationStep) : unavailable),
        row("Kesalahan negosiasi", diagnostic.negotiationException ? mediaText(diagnostic.negotiationException) : unavailable),
        row("Kandidat dikumpulkan / dikirim", `${number(diagnostic.gatheredCandidates)} / ${number(diagnostic.sentCandidates)}`),
        row("Kandidat diterima / ditambahkan", `${number(diagnostic.receivedCandidates)} / ${number(diagnostic.addedCandidates)}`),
        row("Kesalahan kandidat / kode terakhir", `${number(diagnostic.candidateErrors)} / ${number(diagnostic.candidateErrorCode)}`),
        row("Jalur lokal", `${state(diagnostic.localCandidateType)} / ${mediaText("Relay")} ${diagnostic.localRelayProtocol || unavailable}`),
        row("Jalur penerima", `${state(diagnostic.remoteCandidateType)} / ${mediaText("Relay")} ${diagnostic.remoteRelayProtocol || unavailable}`),
        row("DTLS", state(diagnostic.dtls)),
        row("Arah statistik", diagnostic.direction ? mediaText(diagnostic.direction) : unavailable),
        row("Byte audio", number(diagnostic.audioBytes)),
        row("Sampel audio diterima", number(diagnostic.audioSamples)),
        row("Byte video", number(diagnostic.videoBytes)),
        row(diagnostic.direction === "Diterima" ? "Frame diterima" : diagnostic.direction === "Dikirim" ? "Frame dikirim" : "Frame", number(diagnostic.frames)),
        row("Energi audio", number(diagnostic.audioEnergy)),
        row("Video", video ? `${mediaText("Ukuran / jeda / kesiapan")}: ${number(video.width)} × ${number(video.height)} / ${boolean(video.paused)} / ${number(video.readyState)}` : unavailable),
        row("Audio", audio ? `${mediaText("Jeda / kesiapan")}: ${boolean(audio.paused)} / ${number(audio.readyState)}` : unavailable),
        row("Pemutaran video", diagnostic.videoException ? mediaText(diagnostic.videoException) : unavailable),
        row("Pemutaran audio", diagnostic.audioException ? mediaText(diagnostic.audioException) : unavailable),
      ].join("\n");
    }

    fail(message) {
      this.cleanup("", true);
      this.phase = "error";
      this.error = message;
      this.render();
    }

    cleanup(message = "Kiriman dihentikan.", notify = true) {
      const id = this.session?.id;
      this.snapshotDiagnostics();
      ++this.epoch;
      ++this.videoPlayAttempt;
      ++this.audioPlayAttempt;
      this.captureAbort?.abort();
      this.captureAbort = null;
      this.nativeAudio?.close();
      this.nativeAudio = null;
      if (notify && id && this.connected) { try { this.sendMessage({ type: "media-stop", id }); } catch { /* Local cleanup always runs. */ } }
      clearTimeout(this.startTimer);
      clearTimeout(this.connectionTimer);
      clearTimeout(this.moveTimer);
      clearTimeout(this.statsTimer);
      this.statsTimer = null;
      this.clearPlaybackListeners("video");
      this.clearPlaybackListeners("audio");
      for (const dispose of [...this.mediaListeners]) dispose();
      this.startTimer = this.connectionTimer = this.moveTimer = null;
      const pc = this.pc;
      this.pc = null;
      if (pc) pc.close();
      for (const stream of [this.source, this.remote]) stream?.getTracks().forEach((track) => track.stop());
      this.source = this.remote = null;
      if (this.session) this.session.iceServers = [];
      this.session = this.pending = null;
      this.ice = [];
      this.resetPointerGesture();
      this.move = { dx: 0, dy: 0 };
      this.audioPlaying = false;
      this.videoPlaying = this.videoStalled = this.videoFrameSeen = false;
      this.mediaConnectedAt = null;
      this.inputTokens = 8;
      this.inputTokenTime = performance.now();
      this.signalChain = Promise.resolve();
      for (const id of ["media-video", "media-audio"]) { this.$(id).pause(); this.$(id).srcObject = null; }
      this.$("media-play-video").hidden = true;
      this.$("media-allow-control").checked = false;
      this.$("remote-text").value = "";
      this.remoteStatus = "";
      this.$("remote-pad").classList.remove("tracking");
      this.phase = "stopped";
      this.statusMessage = message;
      this.warning = "";
      this.error = "";
      this.render();
    }

    canControl() {
      const transportReady = this.pc && (this.pc.connectionState === "connected" || (!this.pc.connectionState && ["connected", "completed"].includes(this.pc.iceConnectionState)));
      return !!transportReady && this.connected && this.state?.role === "peer" && this.phase === "active" && !!this.session && !this.session.owner && this.session.mode === "screen" && this.session.control && !!this.connectionID && this.session.receiverConnectionId === this.connectionID;
    }

    resetPointerGesture() {
      const id = this.pointer?.id;
      this.pointer = null;
      this.lastTap = null;
      this.padPointers?.clear();
      const pad = this.$("remote-pad");
      pad.classList.remove("tracking");
      if (id !== undefined && pad.hasPointerCapture?.(id)) {
        try { pad.releasePointerCapture(id); } catch { /* Cancellation may already have released capture. */ }
      }
    }

    input(input) {
      if (!this.canControl()) return false;
      // Pointer movement is coalesced separately; allow short click/key bursts.
      const now = performance.now();
      if (input.kind !== "move") {
        this.inputTokens = Math.min(8, this.inputTokens + (now - this.inputTokenTime) * 20 / 1000);
        this.inputTokenTime = now;
        if (this.inputTokens < 1) { this.remoteStatus = "Kontrol terlalu cepat. Tunggu sebentar lalu coba lagi."; this.render(); return false; }
        this.inputTokens--;
      }
      try { this.sendMessage({ type: "media-input", id: this.session.id, input }); return true; }
      catch { this.cleanup("Sambungan kontrol terputus.", false); return false; }
    }

    movePointer(dx, dy) {
      if (!this.canControl()) return;
      this.move.dx = clamp(this.move.dx + dx, 500);
      this.move.dy = clamp(this.move.dy + dy, 500);
      if (this.moveTimer) return;
      this.moveTimer = setTimeout(() => {
        this.moveTimer = null;
        const input = { kind: "move", dx: this.move.dx, dy: this.move.dy };
        this.move = { dx: 0, dy: 0 };
        if (input.dx || input.dy) this.input(input);
      }, 50);
    }

    render() {
      this.snapshotDiagnostics();
      this.renderDiagnostics();
      const host = this.state?.role === "host";
      const devices = (this.state?.devices || []).filter((device) => device.connectionId);
      const fingerprint = JSON.stringify([window.LocalDropI18n?.language, devices.map((device) => [device.connectionId, device.name])]);
      const target = this.$("media-target");
      if (fingerprint !== this.targetFingerprint || !target.options.length) {
        const selected = target.value;
        target.replaceChildren();
        for (const device of devices) {
          const option = document.createElement("option");
          option.value = device.connectionId;
          option.textContent = device.name || mediaText("Perangkat");
          target.append(option);
        }
        if (!devices.length) { const option = document.createElement("option"); option.value = ""; option.textContent = mediaText("Belum ada perangkat penerima"); target.append(option); }
        else if (devices.some((device) => device.connectionId === selected)) target.value = selected;
        this.targetFingerprint = fingerprint;
      }
      const busy = !!this.pending || !!this.session;
      const elsewhere = !!this.state?.media && !busy;
      this.$("media-setup").hidden = busy || elsewhere;
      const screen = this.mode() === "screen";
      this.$("media-screen-options").hidden = !screen;
      const audioAvailable = busy ? (this.session || this.pending).audio : !screen || this.$("media-include-audio").checked;
      const phoneOnlyOptions = this.$("media-phone-only-options");
      const phoneOnly = this.$("media-phone-only");
      if (phoneOnlyOptions) phoneOnlyOptions.hidden = !audioAvailable;
      if (phoneOnly) phoneOnly.disabled = busy || elsewhere || !host || !this.connected || !audioAvailable;
      const outputStatus = this.$("media-output-status");
      if (outputStatus) {
        outputStatus.hidden = !busy || !audioAvailable;
        outputStatus.textContent = mediaText(this.session?.phoneOnly ? "Keluaran suara Windows dibisukan selama kiriman ini. Putar suara di ponsel." : this.pending?.phoneOnly ? "Menyiapkan suara hanya di ponsel. Menunggu konfirmasi keluaran Windows." : "Suara hanya di ponsel tidak aktif. Keluaran Windows mengikuti pengaturan sebelumnya.");
      }
      this.$("media-start").disabled = !host || !this.connected || !this.connectionID || !devices.length || busy || elsewhere || !this.captureSupported();
      target.disabled = busy || !devices.length || !this.connected;
      this.$("media-target-help").hidden = devices.length > 0;
      this.$("media-live").hidden = !busy;
      this.$("media-error").hidden = !this.error;
      this.$("media-error").textContent = mediaError(this.error);
      this.$("media-warning").hidden = !this.warning;
      this.$("media-warning").textContent = mediaError(this.warning);
      let status = this.statusMessage;
      if (!this.state) status = "Menyiapkan sambungan…";
      else if (!this.connected) status = "Tidak tersambung. Kiriman media memerlukan kedua perangkat di Wi-Fi yang sama.";
      else if (!this.supported()) status = "Browser belum mendukung layar dan suara. Gunakan Chrome/Edge di laptop, Chrome terbaru di Android, atau Safari terbaru di iPhone.";
      else if (host && !this.captureSupported()) status = this.nativeSelected() ? this.nativeFailure({ code: "unsupported" }) : "Buka Chrome atau Edge di laptop melalui localhost atau 127.0.0.1 untuk berbagi layar dan suara.";
      else if (elsewhere) status = host ? "Kiriman aktif dari tab laptop lain. Hentikan di tab tersebut untuk memulai di sini." : this.state.media.viewerId === this.connectionID ? "Menunggu kiriman laptop…" : "Laptop sedang mengirim ke perangkat lain.";
      else if (!busy && !status) status = host ? devices.length ? "Pilih perangkat penerima, lalu mulai kirim." : "Belum ada perangkat penerima. Pindai kode QR dengan ponsel." : "Belum ada kiriman layar atau suara. Di laptop, pilih perangkat ini lalu Mulai kirim pada tab Layar & suara.";
      else if (!host && this.session && this.phase === "active") {
        if (this.session.mode === "screen") {
          if (this.hasVideoImage()) status = this.session.audio && !this.hasInboundAudioData() ? "Gambar laptop tampil. Menunggu data suara dari laptop…" : this.session.audio && !this.audioPlaying ? "Gambar laptop tampil. Ketuk Putar suara untuk mendengarkan." : "Gambar laptop tampil.";
          else if (this.mediaConnectedAt !== null && performance.now() - this.mediaConnectedAt >= 8000) status = "Gambar belum tampil. Ketuk Mulai tampilan. Jika tetap kosong, hentikan lalu pilih Seluruh layar di Chrome/Edge laptop.";
          else status = "Sambungan siap. Menunggu gambar dari laptop…";
        } else {
          status = !this.hasInboundAudioData() ? "Sambungan siap. Menunggu data suara dari laptop…" : this.audioPlaying ? "Pemutar suara berjalan. Atur volume dengan tombol ponsel." : "Sambungan siap. Ketuk Putar suara untuk mendengarkan.";
        }
      }
      this.$("media-status").textContent = mediaError(status);
      if (busy) {
        const current = this.session || this.pending;
        const modeLabel = mediaText(current.mode === "audio" ? "Suara saja" : "Layar");
        this.$("media-live-title").textContent = this.phase === "active" ? modeLabel : mediaText("Menyiapkan kiriman");
        const receiver = devices.find((device) => device.connectionId === current.target)?.name;
        this.$("media-live-detail").textContent = mediaText(host ? "Laptop → {receiver} · {mode}" : "Laptop → perangkat ini · {mode}", { receiver: receiver || mediaText("perangkat penerima"), mode: modeLabel });
      }
      const videoPresent = this.session?.mode === "screen" && !!this.$("media-video").srcObject;
      this.$("media-video-wrap").hidden = !videoPresent;
      this.$("media-play-video").hidden = !videoPresent || this.hasVideoImage();
      const incomingAudio = !!this.session && !this.session.owner && this.session.audio;
      this.$("media-audio-row").hidden = !incomingAudio;
      const audioTracks = this.$("media-audio").srcObject?.getAudioTracks() || [];
      this.$("media-play-audio").disabled = !incomingAudio || !this.connected || !audioTracks.some((track) => track.readyState === "live");
      this.$("media-play-audio").textContent = mediaText(this.audioPlaying ? "Jeda suara" : "Putar suara");
      if (incomingAudio) {
        const hasData = this.hasInboundAudioData();
        this.$("media-audio-status").textContent = mediaText(this.diagnostics.audioException ? "Pemutaran belum berhasil. Ketuk Putar suara lagi dan biarkan browser terbuka." : !audioTracks.length ? "Menunggu track audio dari laptop…" : this.audioPlaying ? hasData ? "Pemutar suara berjalan. Atur volume dengan tombol ponsel." : "Pemutaran siap. Menunggu data suara dari laptop…" : hasData ? "Ketuk Putar suara untuk mendengarkan." : "Menunggu data suara. Ketuk Putar suara untuk menyiapkan pemutaran.");
      }
      this.$("media-peer-help").hidden = host || !this.session;
      const control = this.canControl();
      window.LocalDropI18n?.bind(this.$("remote-pad-help"), "remote.padGestureHelp");
      this.$("remote-status").textContent = mediaError(this.remoteStatus);
      this.$("remote-send-text").textContent = window.LocalDropI18n?.t("remote.send") ?? "Kirim ketikan";
      for (const button of this.$("remote-keys").querySelectorAll("[data-remote-key]")) {
        const key = button.dataset.remoteKey;
        const label = mediaText(KEY_LABELS[key] || button.dataset.remoteLabel);
        button.textContent = key.startsWith("arrow") ? button.dataset.remoteLabel : label;
        button.setAttribute("aria-label", mediaText("Kirim tombol {key} ke laptop", { key: label }));
      }
      this.$("panel-media").dataset.remoteControl = String(control);
      this.$("remote-controls").hidden = !control;
      this.$("remote-send-text").disabled = !control || !validText(this.$("remote-text").value);
      for (const button of this.$("remote-controls").querySelectorAll("button")) if (button.id !== "remote-send-text") button.disabled = !control;
      if (!control) {
        clearTimeout(this.moveTimer);
        this.moveTimer = null;
        this.move = { dx: 0, dy: 0 };
        this.resetPointerGesture();
      }
    }

    bind() {
      this.$("media-setup").addEventListener("submit", (event) => this.start(event));
      for (const input of document.querySelectorAll('input[name="media-mode"]')) input.addEventListener("change", () => {
        if (this.mode() !== "screen") this.$("media-allow-control").checked = false;
        this.render();
      });
      this.$("media-include-audio").addEventListener("change", () => this.render());
      this.$("media-phone-only")?.addEventListener("change", () => this.render());
      this.$("media-stop").addEventListener("click", () => this.cleanup(this.phase === "capturing" ? "Kiriman dibatalkan. Tutup pemilih berbagi jika masih terbuka." : "Kiriman dihentikan.", true));
      this.$("media-play-audio").addEventListener("click", () => this.playAudio());
      this.$("media-play-video").addEventListener("click", () => this.playVideo());
      const pad = this.$("remote-pad");
      pad.addEventListener("pointerdown", (event) => {
        if (!this.canControl()) { this.resetPointerGesture(); return; }
        if (event.pointerType === "mouse" && event.button !== 0) {
          if (this.pointer) this.pointer.tap = false;
          this.lastTap = null;
          return;
        }
        this.padPointers.add(event.pointerId);
        if (this.pointer || this.padPointers.size > 1 || event.isPrimary === false) {
          if (this.pointer) this.pointer.tap = false;
          this.lastTap = null;
          return;
        }
        event.preventDefault();
        this.pointer = { id: event.pointerId, x: event.clientX, y: event.clientY, startX: event.clientX, startY: event.clientY,
          time: performance.now(), distance: 0, tap: true, type: event.pointerType, session: this.session, sessionID: this.session.id, epoch: this.epoch };
        pad.setPointerCapture(event.pointerId);
        pad.classList.add("tracking");
        pad.focus();
      });
      pad.addEventListener("pointermove", (event) => {
        if (this.pointer?.id !== event.pointerId || !this.canControl()) return;
        event.preventDefault();
        const dx = event.clientX - this.pointer.x;
        const dy = event.clientY - this.pointer.y;
        this.pointer.distance += Math.hypot(dx, dy);
        if (this.pointer.distance > TAP_MOVEMENT) { this.pointer.tap = false; this.lastTap = null; }
        this.movePointer(dx, dy);
        this.pointer.x = event.clientX;
        this.pointer.y = event.clientY;
      });
      const release = (event) => {
        if (!this.padPointers.delete(event.pointerId)) return;
        if (this.pointer?.id !== event.pointerId) { this.lastTap = null; return; }
        const pointer = this.pointer;
        this.pointer = null;
        pad.classList.remove("tracking");
        const now = performance.now();
        const distance = Math.hypot(event.clientX - pointer.startX, event.clientY - pointer.startY);
        if (event.type !== "pointerup" || !pointer.tap || this.padPointers.size || !this.canControl() || pointer.session !== this.session || pointer.sessionID !== this.session.id || pointer.epoch !== this.epoch || now - pointer.time > TAP_DURATION || distance > TAP_MOVEMENT) {
          this.lastTap = null;
          return;
        }
        const previous = this.lastTap;
        this.lastTap = null;
        if (previous && previous.session === this.session && previous.sessionID === this.session.id && previous.epoch === this.epoch && previous.type === pointer.type && now - previous.time <= DOUBLE_TAP_INTERVAL && Math.hypot(event.clientX - previous.x, event.clientY - previous.y) <= DOUBLE_TAP_DISTANCE) {
          this.input({ kind: "click", button: "left" });
        } else this.lastTap = { x: event.clientX, y: event.clientY, time: now, type: pointer.type, session: this.session, sessionID: this.session.id, epoch: this.epoch };
      };
      pad.addEventListener("pointerup", release);
      pad.addEventListener("pointercancel", release);
      pad.addEventListener("lostpointercapture", release);
      pad.addEventListener("dblclick", (event) => event.preventDefault());
      pad.addEventListener("contextmenu", (event) => event.preventDefault());
      pad.addEventListener("keydown", (event) => {
        const moves = { ArrowLeft: [-20, 0], ArrowRight: [20, 0], ArrowUp: [0, -20], ArrowDown: [0, 20] };
        if (moves[event.key] && this.canControl()) { event.preventDefault(); this.movePointer(...moves[event.key]); }
      });
      for (const button of document.querySelectorAll("[data-remote-click]")) button.addEventListener("click", () => this.input({ kind: "click", button: button.dataset.remoteClick }));
      for (const button of document.querySelectorAll("[data-remote-scroll]")) button.addEventListener("click", () => this.input({ kind: "scroll", dy: clamp(Number(button.dataset.remoteScroll), 1200) }));
      for (const [key, label] of KEYS) {
        const button = document.createElement("button");
        button.type = "button";
        button.className = "button secondary";
        button.dataset.remoteKey = key;
        button.dataset.remoteLabel = label;
        button.textContent = key.startsWith("arrow") ? label : mediaText(KEY_LABELS[key] || label);
        button.setAttribute("aria-label", mediaText("Kirim tombol {key} ke laptop", { key: mediaText(KEY_LABELS[key] || label) }));
        button.addEventListener("click", () => this.input({ kind: "key", key }));
        this.$("remote-keys").append(button);
      }
      this.$("remote-text").addEventListener("input", () => {
        const text = this.$("remote-text").value;
        this.remoteStatus = [...text].length > 256 ? "Ketikan melebihi 256 karakter." : text && !validText(text) ? "Ketikan berisi karakter kontrol yang tidak didukung." : "";
        this.render();
      });
      this.$("remote-text-form").addEventListener("submit", (event) => {
        event.preventDefault();
        const text = this.$("remote-text").value;
        if (!validText(text) || !this.canControl()) return;
        if (this.input({ kind: "text", text })) {
          this.$("remote-text").value = "";
          this.remoteStatus = "Ketikan dikirim ke laptop.";
          this.render();
        }
      });
      window.addEventListener("pagehide", () => this.cleanup("Kiriman dihentikan karena halaman ditutup.", true));
    }
  }

  window.LocalDropMedia = LocalDropMedia;
})();
