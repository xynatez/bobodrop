package server

import (
	"crypto/sha256"
	"encoding/base64"
	"io"
	"net/http"
)

const pairingStyle = `:root {
  color-scheme: light;
  --desk: #F1F6FC;
  --surface: #FFFFFF;
  --surface-secondary: #EAF1F8;
  --ink: #101D40;
  --muted: #51617B;
  --rule: #DCE5F0;
  --edge: #768BA5;
  --primary: #137447;
  --primary-text: #FFFFFF;
  --sans: "Segoe UI Variable", "Segoe UI", -apple-system, BlinkMacSystemFont, system-ui, sans-serif;
  --t14: .875rem;
  --t16: 1rem;
  --t24: 1.5rem;
  --s2: .5rem;
  --s3: .75rem;
  --s4: 1rem;
  --s6: 1.5rem;
  --s8: 2rem;
  --control-radius: 10px;
  --motion: 180ms;
  --press-motion: 100ms;
}
:root[data-theme="dark"] {
  color-scheme: dark;
  --desk: #111A20;
  --surface: #19252D;
  --surface-secondary: #22313B;
  --ink: #F0F4F8;
  --muted: #B0BFCE;
  --rule: #354650;
  --edge: #6F8495;
  --primary: #7DDB99;
  --primary-text: #10251A;
}
* { box-sizing: border-box; }
body {
  margin: 0;
  background: var(--desk);
  color: var(--ink);
  font: var(--t16)/1.5 var(--sans);
  transition: background-color var(--motion), color var(--motion);
}
main { max-width: 42rem; margin: var(--s8) auto; padding: 0 var(--s4); }
header { display: flex; flex-wrap: wrap; align-items: center; gap: var(--s4); padding-bottom: var(--s4); border-bottom: 1px solid var(--rule); }
.wordmark { font-size: var(--t24); font-weight: 700; letter-spacing: -.035em; }
.controls { display: flex; flex-wrap: wrap; align-items: end; gap: var(--s3); margin-left: auto; }
label { display: grid; gap: var(--s2); font-size: var(--t14); font-weight: 600; }
button, select { min-height: 44px; max-width: 100%; padding: var(--s2) var(--s3); border: 1px solid var(--edge); border-radius: var(--control-radius); background: var(--surface); color: var(--ink); font: var(--t16)/1.5 var(--sans); }
button { cursor: pointer; transition: background-color var(--press-motion); }
button:hover, button:active { background: var(--surface-secondary); }
section { padding-top: var(--s6); }
h1 { margin: 0 0 var(--s4); font-size: var(--t24); line-height: 1.2; letter-spacing: -.035em; }
p { margin: 0 0 var(--s4); max-width: 60ch; overflow-wrap: anywhere; }
.hint { color: var(--muted); }
.continue { display: inline-flex; align-items: center; justify-content: center; min-height: 44px; max-width: 100%; padding: var(--s2) var(--s4); border-radius: var(--control-radius); background: var(--primary); color: var(--primary-text); font-weight: 600; text-decoration: none; overflow-wrap: anywhere; }
.continue:hover { text-decoration: underline; }
:is(button, select, a):focus-visible { outline: 3px solid var(--primary); outline-offset: 3px; }
[hidden] { display: none !important; }
noscript p { color: var(--muted); }
@media (min-width: 48rem) { main { padding: 0 var(--s8); } }
@media (prefers-reduced-motion: reduce) { *, *::before, *::after { transition: none !important; } }`

const pairingPreferencesScript = `(() => {
  "use strict";
  const root = document.documentElement;
  const languageControl = document.getElementById("pairing-language");
  const themeControl = document.getElementById("pairing-theme");
  const words = {
    title: ["Sambungkan ke bobodrop", "Connect to bobodrop"],
    pairedTitle: ["Membuka bobodrop", "Opening bobodrop"],
    unpairedTitle: ["Sambungkan ke laptop", "Connect to the laptop"],
    expiredTitle: ["Tautan sudah berakhir", "This link has expired"],
    expired: ["Sesi berakhir atau tautan tidak berlaku. Gunakan QR yang sedang tampil di laptop.", "The session ended or this link is invalid. Use the QR code currently shown on the laptop."],
    scan: ["Scan QR terbaru yang tampil di bobodrop pada laptop dengan kamera Android, iPhone atau iPad, lalu buka tautannya.", "Scan the current QR code shown in bobodrop on the laptop with your Android, iPhone or iPad camera, then open its link."],
    network: ["Sambungkan kedua perangkat ke Wi-Fi yang sama, atau gunakan hotspot ponsel atau laptop.", "Connect both devices to the same Wi-Fi, or use the phone or laptop hotspot."],
    keepOpen: ["Biarkan bobodrop tetap terbuka di laptop saat menyambungkan perangkat.", "Keep bobodrop open on the laptop while connecting your device."],
    accepted: ["Tautan QR diterima. Ruang transfer akan terbuka otomatis.", "QR link accepted. The transfer desk will open automatically."],
    continueHelp: ["Jika halaman belum terbuka, lanjutkan melalui tautan di bawah.", "If the page has not opened, use the link below to continue."],
    continue: ["Lanjutkan ke bobodrop", "Continue to bobodrop"],
    dark: ["Gelap", "Dark"],
    light: ["Terang", "Light"],
    darkAction: ["Gunakan tema gelap", "Use dark theme"],
    lightAction: ["Gunakan tema terang", "Use light theme"]
  };
  let language = "id";
  let theme = "light";
  try {
    if (localStorage.getItem("localdrop-language") === "en") language = "en";
    if (localStorage.getItem("localdrop-theme") === "dark") theme = "dark";
  } catch {}
  function applyTheme(next, persist = false) {
    theme = next === "dark" ? "dark" : "light";
    root.dataset.theme = theme;
    const action = theme === "dark" ? "light" : "dark";
    const index = language === "en" ? 1 : 0;
    themeControl.textContent = words[action][index];
    themeControl.setAttribute("aria-label", words[action + "Action"][index]);
    if (persist) { try { localStorage.setItem("localdrop-theme", theme); } catch {} }
  }
  function applyLanguage(next, persist = false) {
    language = next === "en" ? "en" : "id";
    root.lang = language;
    languageControl.value = language;
    const index = language === "en" ? 1 : 0;
    document.querySelectorAll("[data-copy]").forEach((element) => {
      element.textContent = words[element.dataset.copy][index];
    });
    document.title = words[document.body.dataset.pairing === "paired" ? "pairedTitle" : "title"][index];
    applyTheme(theme);
    if (persist) { try { localStorage.setItem("localdrop-language", language); } catch {} }
  }
  languageControl.addEventListener("change", () => applyLanguage(languageControl.value, true));
  themeControl.addEventListener("click", () => applyTheme(theme === "dark" ? "light" : "dark", true));
  applyLanguage(language);
  document.getElementById("pairing-controls").hidden = false;
})();`

const pairingNavigationScript = `window.location.replace("/");`

const pairingHeader = `<main><header><span class="wordmark">bobodrop</span><div class="controls" id="pairing-controls" hidden><label for="pairing-language">Bahasa / Language<select id="pairing-language"><option value="id">Indonesia</option><option value="en">English</option></select></label><button id="pairing-theme" type="button" aria-label="Gunakan tema gelap">Gelap</button></div></header><section aria-labelledby="pairing-title">`

const pairingRecoveryContent = `<p data-copy="scan">Scan QR terbaru yang tampil di bobodrop pada laptop dengan kamera Android, iPhone atau iPad, lalu buka tautannya.</p><p class="hint" data-copy="network">Sambungkan kedua perangkat ke Wi-Fi yang sama, atau gunakan hotspot ponsel atau laptop.</p><p class="hint" data-copy="keepOpen">Biarkan bobodrop tetap terbuka di laptop saat menyambungkan perangkat.</p><noscript><p lang="en">Scan the current QR code shown in bobodrop on the laptop with your Android, iPhone or iPad camera, then open its link. Connect both devices to the same Wi-Fi, or use the phone or laptop hotspot. Keep bobodrop open on the laptop.</p></noscript>`

const pairedContent = `<h1 id="pairing-title" data-copy="pairedTitle">Membuka bobodrop</h1><p data-copy="accepted">Tautan QR diterima. Ruang transfer akan terbuka otomatis.</p><p class="hint" data-copy="continueHelp">Jika halaman belum terbuka, lanjutkan melalui tautan di bawah.</p><a class="continue" id="pairing-continue" href="/" data-copy="continue">Lanjutkan ke bobodrop<noscript><span lang="en"> / Continue to bobodrop</span></noscript></a><noscript><p lang="en">QR link accepted. Open the transfer desk using the link above.</p></noscript>`

func inlineSource(content string) string {
	hash := sha256.Sum256([]byte(content))
	return "'sha256-" + base64.StdEncoding.EncodeToString(hash[:]) + "'"
}

func writePairingPage(w http.ResponseWriter, status int, state, content string) {
	scriptSources := inlineSource(pairingPreferencesScript)
	if state == "paired" {
		scriptSources += " " + inlineSource(pairingNavigationScript)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src "+scriptSources+"; style-src "+inlineSource(pairingStyle)+"; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, `<!doctype html><html lang="id" data-theme="light"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Sambungkan ke bobodrop</title><style>`+pairingStyle+`</style></head><body data-pairing="`+state+`">`+pairingHeader+content+`</section></main><script>`+pairingPreferencesScript+`</script>`)
	if state == "paired" {
		_, _ = io.WriteString(w, `<script>`+pairingNavigationScript+`</script>`)
	}
	_, _ = io.WriteString(w, `</body></html>`)
}

func pairedPage(w http.ResponseWriter) {
	writePairingPage(w, http.StatusOK, "paired", pairedContent)
}

func unpairedPage(w http.ResponseWriter) {
	writePairingPage(w, http.StatusUnauthorized, "unpaired", `<h1 id="pairing-title" data-copy="unpairedTitle">Sambungkan ke laptop</h1>`+pairingRecoveryContent)
}

func expiredPairingPage(w http.ResponseWriter) {
	writePairingPage(w, http.StatusUnauthorized, "expired", `<h1 id="pairing-title" data-copy="expiredTitle">Tautan sudah berakhir</h1><p data-copy="expired">Sesi berakhir atau tautan tidak berlaku. Gunakan QR yang sedang tampil di laptop.</p>`+pairingRecoveryContent)
}
