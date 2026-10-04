# bobodrop

Versi 1.4.1 menambahkan kredit footer **[Alwin @xynatez](https://github.com/xynatez)** pada laptop dan perangkat penerima. Source tersedia di [xynatez/bobodrop](https://github.com/xynatez/bobodrop); paket Windows tersedia pada halaman [Releases](https://github.com/xynatez/bobodrop/releases).

Versi 1.4.0 mengganti nama LocalDrop menjadi bobodrop, memakai logo kucing
transparan dari pengguna, dan memperbarui tema terang/gelap, layout, serta
tipografi. Tautan HTTP/HTTPS pada kiriman teks bisa ditekan. Angka pada tab
Teks, File, dan Layar & suara menunjukkan kiriman baru dari perangkat lain;
angka dihapus saat tab tersebut dilihat.

Pada iPhone dengan alamat HTTP lokal, tombol **Tempel teks** membawa fokus ke
kolom tanpa menghapus draf. Tekan lama kolom lalu pilih **Tempel** dari menu
iOS. Akses clipboard Windows tetap memakai integrasi native.

Antrean audio kini menjaga urutan sampel, membatasi audio lama, dan menyesuaikan
perbedaan laju perangkat. Cadangan awal 50 ms membantu melewati jeda pengiriman
singkat; batas antrean 60 ms mencegah penumpukan tanpa batas. Ini memperbaiki
kontinuitas pada simulasi, bukan jaminan latensi akhir atau suara tanpa jeda
di setiap HP. Pengukuran jaringan dan perangkat tersedia di **Periksa kiriman**.

Buka tautan QR lengkap yang menyertakan `/s/` dan kode sesi. Halaman perantara
menyelesaikan sesi sebelum membuka workspace; cookie tetap HttpOnly dan
SameSite Strict. Alamat pendek di HP yang belum dipasangkan tetap meminta
scan QR terbaru dari laptop.

bobodrop memakai logo transparan yang disediakan pengguna, pilihan Indonesia/English,
dukungan browser Android, iPhone dan iPad, serta tab Panduan/Guide untuk jaringan
lokal dan hotspot. Menu tray Windows menampilkan kedua bahasa.

**Salin teks masuk ke clipboard Windows** berlaku untuk teks yang dikirim melalui bobodrop. Salin di
aplikasi HP lain, tempel pada tab Teks, lalu Kirim teks; pengiriman kini
menunggu hasil penulisan clipboard sebelum menerbitkan tanda terima. Pengaturan
menampilkan keberhasilan atau kegagalan salinan Windows.

Sinkronisasi clipboard semua aplikasi HP ke Windows tanpa membuka bobodrop
belum didukung oleh antarmuka browser ini. Label pengaturan
diganti agar tidak menyatakan kemampuan tersebut. Halaman browser harus memiliki
fokus untuk akses clipboard asinkron, sesuai [batasan Clipboard API](https://www.w3.org/TR/clipboard-apis/#privacy-and-the-asychronous-clipboard-api).
bobodrop tidak memasang aplikasi pendamping iOS atau integrasi Shortcuts.

Pilihan **Hanya speaker HP** memakai lease mute keluaran Windows selama
berbagi audio dan memulihkan kondisi mute sebelumnya saat berhenti. Helper
terpisah memulihkan keluaran jika proses utama berakhir. Angka volume tidak
diturunkan. Jalur ini mengambil audio proses Windows melalui
[WASAPI process loopback](https://learn.microsoft.com/en-us/samples/microsoft/windows-classic-samples/applicationloopbackaudio-sample/),
terpisah dari mute speaker. Windows build 20348 atau lebih baru diperlukan.
Audio PCM stereo 48 kHz diteruskan ke browser tanpa pemrosesan mikrofon,
lalu dikirim memakai Opus stereo. Jika pengambilan native gagal, kiriman
berhenti dan menampilkan pesan; tidak beralih diam-diam ke sumber browser yang
bergantung pada mute speaker. **Suara saja** dengan **Hanya speaker HP**
tidak memerlukan pemilih layar. Bila pilihan tersebut dimatikan, gunakan
pemilih Chrome/Edge dan aktifkan **Bagikan audio sistem**.
Pengaturan sumber yang tersedia terlihat di Periksa kiriman. Target bitrate adalah batas
pengirim/negosiasi, bukan janji throughput atau audio lossless; [Opus RTP](https://www.rfc-editor.org/rfc/rfc7587)
membedakan jam RTP, stereo penerima dan format sumber. Panggilan aktif,
Bluetooth komunikasi, audio eksklusif atau DRM dapat membatasi hasil. Deteksi
perangkat baru atau unmute eksternal memiliki jeda singkat.

English setup instructions are included in the portable package's Petunjuk.txt
and the in-app Guide. Select English in the header. Use the same local Wi-Fi,
phone-to-laptop hotspot, or Windows-to-phone hotspot; no cloud service is used.
Pair Android in Chrome or iPhone in Safari. Automatic clipboard copy applies
to submitted bobodrop text, and Phone speaker only temporarily mutes Windows
outputs while preserving the captured stream and restoring prior mute states.

Transfer teks dan file, berbagi layar laptop, dan memakai speaker iPhone, iPad atau Android untuk suara laptop melalui jaringan lokal. Kontrol mouse dan keyboard laptop tersedia dengan izin dari laptop. Aplikasi Windows berupa satu executable portable dengan antarmuka browser lokal, clipboard native, dan system tray. HP dan tablet tidak perlu memasang aplikasi. Semua aset antarmuka tersimpan di executable; tidak ada akun, server cloud, CDN, atau biaya layanan.

## Jalankan

1. Buka `dist\bobodrop.exe`. bobodrop membuka antarmuka di browser Windows dan tetap aktif di system tray.
2. Hubungkan laptop dan HP/tablet ke jaringan Wi-Fi yang sama, hotspot HP untuk laptop, atau hotspot Windows untuk HP/tablet.
3. Scan QR menggunakan kamera perangkat, kemudian buka tautan di Safari pada iPhone/iPad atau Chrome pada Android.
4. Pilih **Teks** untuk berbagi teks/tautan, atau **File** untuk foto dan dokumen. File yang sudah terkirim dapat diunduh dari kedua perangkat.

File tersimpan di folder Downloads Windows yang sebenarnya, termasuk jika folder tersebut dipindahkan: `Downloads\LocalDrop`. Nama file yang sama tidak menimpa file sebelumnya. Upload ditulis bertahap ke disk; batas per file 10 GiB. Browser, ruang disk, dan kualitas Wi-Fi tetap menentukan keberhasilan transfer besar.

Klik kanan ikon bobodrop di system tray untuk membuka aplikasi/folder, menyalin tautan HP, atau **Keluar / Quit**. Menutup tab browser membiarkan server dan tray tetap aktif.

Antarmuka menyediakan tema terang dan gelap, susunan khusus laptop/tablet/HP, serta transisi pada tab, dialog dan kiriman baru. Tombol **Gelap/Terang** menyimpan pilihan pada browser perangkat itu. Pengaturan **Reduce Motion** menonaktifkan gerak dan transisi. Tema dan perpindahan tab mempertahankan pemutaran media yang sedang aktif.

## Speaker HP/tablet, layar, dan remote

1. Di laptop, buka bobodrop di **Chrome atau Edge biasa, di luar panel Codex**, melalui `http://127.0.0.1:8787` (atau port yang dipakai aplikasi). Buka tab **Layar & suara** setelah HP/tablet tersambung.
2. Pilih perangkat penerima, **Suara saja**, dan **Hanya speaker HP**, lalu klik **Mulai kirim**. Audio diambil langsung dari Windows; tidak ada pemilih layar untuk mode ini. Jika **Hanya speaker HP** dimatikan, browser memakai pemilih layar: pilih **Seluruh layar** dan centang **Bagikan audio sistem**.
3. Di Safari iPhone/iPad atau Chrome Android, buka **Layar & suara** lalu ketuk **Putar suara**. Atur volume dengan tombol perangkat dan biarkan halaman browser terbuka. Pilihan **Hanya speaker HP** membisukan keluaran Windows selama kiriman; matikan pilihan itu bila kedua perangkat perlu bersuara.
4. Untuk berbagi layar, pilih mode **Layar** dan pilih sumber pada dialog Chrome/Edge. Jika suara dan **Hanya speaker HP** diaktifkan, gambar berasal dari pemilih layar dan suara dari Windows secara terpisah. Centang **Izinkan kontrol mouse dan keyboard** jika HP juga harus mengendalikan laptop. HP menyediakan touchpad relatif, klik kiri/kanan, scroll, ketikan, dan tombol khusus. Gerakan dilakukan melalui touchpad di bawah video; ketuk dua kali touchpad untuk klik kiri. Tombol klik kiri/kanan tetap tersedia.
5. Klik **Hentikan** dari laptop atau HP. Berbagi dan izin kontrol juga berakhir jika sumber audio gagal, salah satu koneksi putus, tab ditutup, atau sesi diganti. Kiriman layar berikutnya memerlukan pilihan layar baru; kiriman suara native tidak.

Satu penerima dapat menerima media pada satu waktu. Perangkat lain tetap dapat berbagi teks/file. Suara ini bukan perangkat output virtual di pengaturan Windows. Pengambilan native memakai aliran proses, bukan loopback endpoint speaker; aplikasi sumber tetap perlu berhasil memutar audio di Windows. Kualitas Wi-Fi menentukan jeda dan kestabilan. Konten terlindungi DRM dan mode eksklusif tidak dijamin dapat ditangkap. Safari di latar belakang atau saat iPhone dikunci dapat menghentikan media. Tidak ada pengambilan mikrofon/kamera HP.

Status **Tersambung** di bagian atas menunjukkan koneksi aplikasi. Di tab media, **Menunggu laptop** berarti belum ada sesi yang dikirim ke tab tersebut; **Menyambungkan** berarti aliran media belum siap. Mode **Suara saja** tidak menampilkan layar. Pilih **Layar** di laptop bila ingin mengirim tampilan juga. Jika pemutaran layar tertunda atau dijeda, gunakan **Mulai tampilan** di penerima. Detail **Periksa kiriman** menunjukkan pengambilan sumber, koneksi media, data yang diterima/dikirim, dan pemutaran dari browser saat ini; nilai yang tidak disediakan browser ditandai **Belum tersedia**. Detail ini tidak memuat isi media atau alamat kandidat koneksi.

Foto dan file dari aplikasi Foto/Files iPhone dapat diseret ke area file. bobodrop mulai membaca isi file saat dilepas, lalu mengunggah salinan yang sudah lengkap. Status **Membaca file dari seretan** dapat dibatalkan. Jalur seretan iPhone/iPad dibatasi 32 MiB total dalam antrean; gunakan **Pilih file** untuk file lebih besar, yang tetap dapat dikirim sampai 10 GiB. Jika Safari tidak memberikan file yang dapat dibaca, halaman meminta Anda memilih ulang. bobodrop memeriksa ukuran file yang diterima sebelum menerbitkan tanda terima dan tidak mengunduh tautan gambar sebagai pengganti file seretan.

Kontrol remote berlaku pada desktop Windows biasa; jendela dengan hak administrator, prompt UAC, dan layar login dapat menolaknya. Tidak ada injeksi input sebelum izin eksplisit dari laptop. Fitur ini mengirim layar dan audio **laptop ke iPhone**. Safari iPhone belum menyediakan capture seluruh layar untuk mengirim tampilan aplikasi lain ke laptop; arah tersebut memerlukan integrasi aplikasi iOS tersendiri.

Batas browser dan Windows mengikuti [Screen Capture API](https://developer.mozilla.org/en-US/docs/Web/API/MediaDevices/getDisplayMedia), [pilihan audio Chrome](https://developer.chrome.com/docs/web-platform/screen-sharing-controls), [process loopback Windows](https://learn.microsoft.com/en-us/samples/microsoft/windows-classic-samples/applicationloopbackaudio-sample/), serta [SendInput Windows](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-sendinput).

Media memakai WebRTC dengan jalur langsung dan relay TURN/TCP di laptop. Setiap kiriman membuat port TCP sementara, kredensial berbeda untuk laptop dan penerima, serta maksimal dua alokasi yang hanya dapat mengirim ke endpoint relay pasangannya pada laptop. Penghentian kiriman mencabut kredensial dan menutup koneksi/alokasi tersebut. Tidak ada STUN/TURN internet atau relay cloud, dan HP tetap tidak diminta mengakses mikrofon/kamera. Jalur relay menjaga paket antar alokasi pada laptop, sehingga kedua browser dapat terhubung ke laptop melalui TCP. Pemilihan jalur yang benar-benar dipakai terlihat dari tipe kandidat dan protokol relay pada **Periksa kiriman**.

Jika halaman terbuka tetapi media tetap `new` atau gagal, periksa signaling, pengumpulan kandidat, jumlah kandidat yang dikirim/ditambahkan, dan status ICE pada kedua perangkat. Nilai pemutaran saja belum membuktikan paket tiba. Gunakan satu instance bobodrop; jika menjalankan executable lagi saat yang lama masih aktif, port dan QR bisa berbeda. Scan QR milik instance yang mengirim. Media dienkripsi WebRTC, sedangkan pairing, sinyal, transfer teks/file, dan kontrol masih menggunakan HTTP/WS lokal. Gunakan jaringan dan penerima yang dipercaya.

## Clipboard dan sesi

Teks kedua perangkat tampil secara langsung dan disimpan dalam riwayat sesi, maksimal 100 entri. Di pengaturan Windows, aktifkan **Salin teks masuk ke clipboard Windows** agar teks yang dikirim melalui bobodrop juga masuk clipboard laptop. Pengaturan ini awalnya nonaktif. bobodrop tidak memantau clipboard aplikasi HP lain; tombol tempel di Windows membacanya saat diminta.

Safari pada alamat HTTP lokal dapat membatasi akses clipboard. Jika tombol salin tidak diizinkan, aplikasi menampilkan teks yang dapat dipilih dan disalin manual. Untuk mengirim dari iPhone, tempel langsung ke kotak teks menggunakan menu iOS.

QR memuat token acak per sesi. Hanya perangkat yang menerima tautan sesi dapat bergabung. **Putuskan perangkat** mengganti token dan mengakhiri koneksi lama; scan QR baru untuk masuk lagi. Riwayat teks dan daftar file di antarmuka berlaku selama proses aplikasi berjalan. Sesudah aplikasi ditutup, file yang sudah selesai tetap ada di Downloads, sementara riwayat dan tautan sesi dihapus dari memori.

Transfer menggunakan HTTP di jaringan lokal; token membatasi akses tetapi bukan enkripsi lalu lintas. Gunakan Wi-Fi yang dipercaya. bobodrop tidak mengirim isi teks atau file ke internet.

## Jika iPhone tidak tersambung

- Pastikan kedua perangkat memakai Wi-Fi yang sama; pilih alamat **Wi-Fi** di bawah QR jika laptop memiliki VPN atau adapter virtual.
- Saat Windows meminta akses jaringan untuk bobodrop, izinkan pada jaringan yang Anda gunakan dan percaya.
- Jika diblokir firewall, buka `dist\Izinkan-LAN.cmd`. Windows meminta hak administrator untuk aturan TCP masuk khusus executable bobodrop, mencakup web dan port relay sementara. Bantuan ini mencakup profil Private/Domain; profil Public dibatasi subnet lokal karena Wi-Fi bisa berprofil Public. Skrip tidak mematikan firewall atau mengganti kategori jaringan.
- Wi-Fi tamu/kantor dapat mengaktifkan *client isolation*. Jika itu berlaku, gunakan hotspot/jaringan lain yang mengizinkan perangkat saling terhubung.
- Alamat serta QR diperbarui otomatis saat IP berubah atau laptop berpindah Wi-Fi (sekitar tiga detik). Scan QR terbaru jika tautan sebelumnya tidak dapat dibuka.
- Jika port 8787 sibuk, aplikasi otomatis mencoba 8788–8797. QR selalu memakai port yang berhasil dibuka.

## Build dari sumber

Go 1.25 atau lebih baru diperlukan hanya pada komputer pengembang. Pengguna executable tidak memerlukan Go, Node.js, Python, Docker, atau database.

```powershell
.\build.ps1
# Atau gunakan lokasi SDK tertentu:
.\build.ps1 -GoExe C:\path\to\go.exe
```

Build menjalankan pengujian, `go vet`, kemudian menghasilkan `dist\bobodrop.exe` tanpa jendela terminal. Untuk executable dengan output terminal gunakan `-Console`.

Opsi developer:

```powershell
.\dist\bobodrop.exe --no-open --no-tray --port 8877 --data-dir .\test-downloads
```

Opsi lain: `--bind`, `--device-name`, dan `--version`. Port yang diberikan secara eksplisit tidak memakai fallback. Log diagnostik disimpan di `%APPDATA%\LocalDrop\localdrop.log`; isi clipboard dan token sesi tidak dicatat.

## Struktur

- `main.go`: lifecycle aplikasi, asset embed, pemilihan alamat LAN, dan launcher.
- `internal/server`: sesi, origin/host validation, streaming upload/download, QR, WebSocket, dan API.
- `internal/platform`: system tray serta clipboard dan shell Windows native.
- `web`: antarmuka responsif HTML/CSS/JavaScript tanpa build frontend.
- `DESIGN.md` dan `CONTRACT.md`: brief desain dan kontrak integrasi.

Pengujian otomatis dan screenshot tersimpan di `artifacts` saat verifikasi pengembangan. Uji otomatis dapat meniru dua browser, tetapi koneksi melalui Wi-Fi dan Safari pada iPhone fisik harus dicoba pada jaringan pengguna.
