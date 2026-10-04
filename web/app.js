(() => {
  "use strict";

  const $ = (id) => document.getElementById(id);
  const i18n = window.LocalDropI18n;
  i18n.register({
    id: {
      "clipboard.pasteText": "Tempel teks",
      "clipboard.pasteHintTouch": "Tekan lama kolom teks, lalu pilih Tempel dari menu ponsel.",
      "clipboard.pasteHintKeyboard": "Tempel teks di kolom ini dengan Ctrl+V atau menu Tempel.",
      "text.characters": "{count} karakter",
      "tabs.unread": "{label}, {count} kiriman belum dilihat",
      "tabs.unreadCount": "{count} kiriman belum dilihat",
    },
    en: {
      "clipboard.pasteText": "Paste text",
      "clipboard.pasteHintTouch": "Press and hold the text field, then choose Paste from your phone's menu.",
      "clipboard.pasteHintKeyboard": "Paste text in this field with Ctrl+V or the Paste menu.",
      "text.characters": "{count} characters",
      "tabs.unread": "{label}, {count} unseen transfers",
      "tabs.unreadCount": "{count} unseen transfers",
    },
  });
  const t = i18n.t;
  const m = (key, params = {}) => ({ key, params });
  const known = (raw) => ({ raw });
  function setText(element, value) {
    if (typeof element === "string") element = $(element);
    if (value && typeof value === "object" && value.key) i18n.bind(element, value.key, value.params);
    else {
      delete element.dataset.i18n;
      delete element.dataset.i18nParams;
      element.textContent = value?.raw !== undefined ? i18n.error(value.raw) : value;
    }
    if (value?.raw !== undefined) {
      element.dataset.localFormat = "";
      element.localDropFormat = () => i18n.error(value.raw);
    } else { delete element.dataset.localFormat; delete element.localDropFormat; }
    return element;
  }
  function formatted(tag, className, format) {
    const element = node(tag, className, format());
    element.dataset.localFormat = "";
    element.localDropFormat = format;
    return element;
  }
  const messageText = (value) => value?.key ? t(value.key, value.params) : i18n.error(value?.raw ?? value ?? "");
  let theme = "light";
  try { if (localStorage.getItem("localdrop-theme") === "dark") theme = "dark"; } catch { /* The theme still works when storage is blocked. */ }

  function applyTheme(next, persist = false) {
    theme = next === "dark" ? "dark" : "light";
    document.documentElement.dataset.theme = theme;
    const control = $("theme-toggle");
    if (control) {
      i18n.bind(control, theme === "dark" ? "theme.light" : "theme.dark");
      i18n.bind(control, theme === "dark" ? "theme.useLight" : "theme.useDark", {}, "aria-label");
    }
    document.querySelector('meta[name="theme-color"]')?.setAttribute("content", theme === "dark" ? "#111A20" : "#F1F6FC");
    if (persist) { try { localStorage.setItem("localdrop-theme", theme); } catch { /* Keep the choice for this page when storage is blocked. */ } }
  }

  applyTheme(theme);
  $("theme-toggle")?.addEventListener("click", () => applyTheme(theme === "dark" ? "light" : "dark", true));
  const MAX_FILE_BYTES = 10 * 1024 * 1024 * 1024;
  const MAX_DROP_SNAPSHOT_BYTES = 32 * 1024 * 1024;
  const DROP_READ_TIMEOUT = 30000;
  const MAX_TEXT_BYTES = 1024 * 1024;
  const FILE_EXTENSIONS = Object.freeze(Object.assign(Object.create(null), {
    "image/jpeg": "jpg", "image/pjpeg": "jpg", "image/png": "png", "image/x-png": "png",
    "image/heic": "heic", "image/heic-sequence": "heic", "image/heif": "heif", "image/heif-sequence": "heif",
    "image/webp": "webp", "image/avif": "avif", "image/gif": "gif", "image/tiff": "tiff", "image/x-tiff": "tiff",
    "image/bmp": "bmp", "image/x-ms-bmp": "bmp", "image/svg+xml": "svg", "image/jp2": "jp2", "image/jxl": "jxl",
    "application/pdf": "pdf", "text/plain": "txt", "video/mp4": "mp4", "video/quicktime": "mov",
    "audio/mpeg": "mp3", "audio/mp4": "m4a", "application/zip": "zip",
  }));
  const PHOTO_DROP_HELP = m("file.photoDrop");
  const appleMobile = /iPhone|iPad|iPod/i.test(navigator.userAgent) || (navigator.platform === "MacIntel" && navigator.maxTouchPoints > 1);
  const deviceLabel = appleMobile ? "iPhone / iPad" : /Android/i.test(navigator.userAgent) ? "Android" : "Browser";
  let clientID;
  try {
    clientID = sessionStorage.getItem("localdrop-client-id");
    if (!clientID) {
      clientID = typeof crypto.randomUUID === "function" ? crypto.randomUUID() : Array.from(crypto.getRandomValues(new Uint8Array(16)), (byte) => byte.toString(16).padStart(2, "0")).join("");
      sessionStorage.setItem("localdrop-client-id", clientID);
    }
  } catch {
    clientID = `browser-${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`;
  }
  let state = null;
  let socket = null;
  let reconnectTimer = null;
  let reconnectAttempt = 0;
  let loadingState = false;
  let socketStateVersion = 0;
  let connected = false;
  let revoked = false;
  let sending = false;
  let manualPaste = false;
  let selectedIP = "";
  let selectedTab = "text";
  let dialogReturnFocus = null;
  let uploadID = 0;
  let activeUpload = null;
  let dropSnapshotBytes = 0;
  let dropDepth = 0;
  const expandedTexts = new Set();
  const seenReceipts = { texts: new Set(), files: new Set() };
  const newReceipts = { texts: new Set(), files: new Set() };
  const unreadReceipts = { text: new Set(), files: new Set(), media: new Set() };
  const seenMedia = new Set();
  const queue = [];
  const media = new window.LocalDropMedia({
    send: (message) => {
      if (revoked || !socket || socket.readyState !== WebSocket.OPEN) throw new Error(t("error.disconnected"));
      socket.send(JSON.stringify(message));
    },
    selectTab,
  });
  const icons = {
    phone: ["rect", { x: "7", y: "2", width: "10", height: "20", rx: "2" }, "path", { d: "M11 18h2" }],
    copy: ["rect", { x: "8", y: "8", width: "12", height: "13", rx: "1" }, "path", { d: "M16 8V3H3v13h5" }],
    download: ["path", { d: "M12 3v13m-5-5 5 5 5-5M3 16v5h18v-5" }],
    close: ["path", { d: "m6 6 12 12M18 6 6 18" }],
    file: ["path", { d: "M5 3h9l5 5v13H5ZM14 3v5h5M9 13h6m-6 4h6" }],
  };

  function node(tag, className, text) {
    const element = document.createElement(tag);
    if (className) element.className = className;
    if (text !== undefined) setText(element, text);
    return element;
  }

  function icon(name) {
    const svg = document.createElementNS("http://www.w3.org/2000/svg", "svg");
    svg.setAttribute("viewBox", "0 0 24 24");
    svg.setAttribute("aria-hidden", "true");
    const parts = icons[name] || icons.file;
    for (let i = 0; i < parts.length; i += 2) {
      const part = document.createElementNS("http://www.w3.org/2000/svg", parts[i]);
      for (const [key, value] of Object.entries(parts[i + 1])) part.setAttribute(key, value);
      svg.append(part);
    }
    return svg;
  }

  function button(label, className, handler, iconName) {
    const element = node("button", `button ${className || "quiet"}`);
    element.type = "button";
    if (iconName) element.append(icon(iconName));
    element.append(node("span", "", label));
    element.addEventListener("click", handler);
    return element;
  }

  function notice(message, error = false) {
    setText("notice-text", message);
    $("notice").classList.toggle("error", error);
    $("notice").hidden = false;
  }

  function dateLabel(value) {
    const date = new Date(value);
    return Number.isNaN(date.getTime()) ? t("value.noDate") : new Intl.DateTimeFormat(i18n.locale, { day: "numeric", month: "short", hour: "2-digit", minute: "2-digit" }).format(date);
  }

  function sizeLabel(bytes) {
    if (!Number.isFinite(Number(bytes)) || Number(bytes) < 0) return t("value.noSize");
    if (bytes < 1024) return `${new Intl.NumberFormat(i18n.locale).format(bytes)} B`;
    const units = ["KB", "MB", "GB"];
    let size = bytes / 1024;
    let index = 0;
    while (size >= 1024 && index < units.length - 1) { size /= 1024; index++; }
    return `${new Intl.NumberFormat(i18n.locale, { maximumFractionDigits: 1 }).format(size)} ${units[index]}`;
  }

  function onlineConnectionLabel() {
    if (state?.role !== "host") return m("connection.online");
    const count = Array.isArray(state.devices) ? state.devices.length : 0;
    return count ? m("connection.devices", { count }) : m("connection.ready");
  }

  function connection(status, label) {
    connected = status === "online";
    media.connectionChanged(connected);
    // Reconnect the socket as well if an HTTP failure marks this page offline;
    // its close revokes any server-side share, including a pending start.
    if (status === "offline" && socket?.readyState === WebSocket.OPEN) socket.close();
    $("connection").dataset.status = status;
    setText("connection-label", connected ? onlineConnectionLabel() : label);
    updateControls();
    if (connected) pumpQueue();
  }

  function updateControls() {
    const available = !!state && connected && !revoked;
    $("send-text").disabled = !available || sending || !$("text-input").value.trim();
    i18n.bind($("send-text").querySelector("span"), sending ? "text.sending" : "text.send");
    $("choose-file").disabled = !available;
    $("choose-photo").disabled = !available;
    $("paste-text").disabled = !state || revoked;
    i18n.bind($("paste-text").querySelector("span"), state?.role === "peer" && (manualPaste || !canReadClipboard()) ? "clipboard.pasteText" : "text.paste");
    const counter = $("text-length");
    if (counter) setText(counter, m("text.characters", { count: new Intl.NumberFormat(i18n.locale).format(Array.from($("text-input").value).length) }));
    $("settings").disabled = !available;
    $("open-folder").disabled = !available;
    $("copy-link").disabled = !state || revoked || !$("join-url").value;
    $("drop-zone").setAttribute("aria-disabled", String(!available));
  }

  async function api(path, options = {}) {
    const headers = new Headers(options.headers || {});
    headers.set("X-Device-ID", clientID);
    if (options.body && typeof options.body === "string") headers.set("Content-Type", "application/json");
    const response = await fetch(path, { ...options, headers, credentials: "same-origin", cache: "no-store" });
    let data = null;
    try { data = await response.json(); } catch { /* A proxy or connection error may not have JSON. */ }
    if (response.status === 401) {
      revoke();
      throw new Error(t("error.expired"));
    }
    if (!response.ok) throw new Error(data?.error ? i18n.error(data.error) : t("error.request", { status: response.status }));
    if (!data) throw new Error(t("error.response"));
    return data;
  }

  function revoke() {
    if (revoked) return;
    revoked = true;
    for (const unread of Object.values(unreadReceipts)) unread.clear();
    updateUnreadBadges();
    clearTimeout(reconnectTimer);
    socket?.close();
    connection("offline", m("connection.ended"));
    closeDialog();
    if (activeUpload) activeUpload.xhr.abort();
    for (const item of queue) {
      if (["queued", "preparing"].includes(item.status)) { item.status = "cancelled"; item.message = m("connection.ended"); }
      releaseQueuedFile(item);
    }
    renderQueue();
    $("workspace").hidden = true;
    $("start-state").hidden = false;
    setText("start-title", m("start.rejoin"));
    setText("start-message", m("start.expired"));
    $("retry-start").hidden = true;
    $("notice").hidden = true;
  }

  function trackReceipts(next, live) {
    const sameSession = !!state && state.sessionId === next.sessionId && state.role === next.role;
    if (!sameSession) {
      for (const unread of Object.values(unreadReceipts)) unread.clear();
      seenMedia.clear();
    }
    for (const kind of ["texts", "files"]) {
      const tab = kind === "texts" ? "text" : "files";
      const entries = Array.isArray(next[kind]) ? next[kind] : [];
      const currentIDs = new Set(entries.map((entry) => entry.id));
      for (const id of unreadReceipts[tab]) if (!currentIDs.has(id)) unreadReceipts[tab].delete(id);
      newReceipts[kind].clear();
      if (!sameSession) seenReceipts[kind].clear();
      for (const entry of entries) {
        if (entry.id === undefined || entry.id === null) continue;
        if (sameSession && !seenReceipts[kind].has(entry.id)) {
          if (live) newReceipts[kind].add(entry.id);
          if (incomingReceipt(entry, next.role) && !panelVisible(tab)) unreadReceipts[tab].add(entry.id);
        }
        seenReceipts[kind].add(entry.id);
      }
    }
    const share = next.media;
    if (share?.id) {
      const recipient = next.role === "peer" && (share.viewerId === media.connectionID || (next.devices || []).some((device) => device.id === clientID && device.connectionId === share.viewerId));
      trackMediaReceipt(share.id, sameSession && recipient);
    }
    clearViewedReceipts();
  }

  function incomingReceipt(entry, role) {
    if (entry.senderId === clientID) return false;
    if (entry.senderRole === "host") return role === "peer";
    if (entry.senderRole !== "peer") return false;
    return role === "host" || (typeof entry.senderId === "string" && !!entry.senderId);
  }

  function panelVisible(tab) {
    return !document.hidden && selectedTab === tab && !$( `panel-${tab}` )?.hidden;
  }

  function trackMediaReceipt(id, incoming) {
    if (typeof id !== "string" || !id || seenMedia.has(id)) return;
    seenMedia.add(id);
    if (incoming && !panelVisible("media")) unreadReceipts.media.add(id);
  }

  function clearViewedReceipts() {
    if (unreadReceipts[selectedTab] && panelVisible(selectedTab)) unreadReceipts[selectedTab].clear();
    updateUnreadBadges();
  }

  function updateUnreadBadges() {
    for (const tab of ["text", "files", "media", "guide"]) {
      const count = unreadReceipts[tab]?.size || 0;
      const label = t(`tabs.${tab}`);
      const description = count ? t("tabs.unread", { label, count: new Intl.NumberFormat(i18n.locale).format(count) }) : label;
      const control = $(`tab-${tab}`);
      control?.setAttribute("aria-label", description);
      control?.setAttribute("title", description);
      const badge = $(`unread-${tab}`);
      if (!badge) continue;
      const value = count ? new Intl.NumberFormat(i18n.locale).format(count) : "";
      if (badge.textContent !== value) badge.textContent = value;
      badge.hidden = count === 0;
      badge.setAttribute("role", "status");
      badge.setAttribute("aria-live", "polite");
      badge.setAttribute("aria-atomic", "true");
      if (count) badge.setAttribute("aria-label", t("tabs.unreadCount", { count: value }));
      else badge.removeAttribute("aria-label");
    }
  }

  function revealReceipt(row, kind, id) {
    if (!newReceipts[kind].delete(id) || window.matchMedia?.("(prefers-reduced-motion: reduce)").matches) return;
    row.classList.add("receipt-new");
    const clear = () => row.classList.remove("receipt-new");
    row.addEventListener("animationend", clear, { once: true });
    setTimeout(clear, 260);
  }

  function applyState(next, live = false) {
    if (!next || !["host", "peer"].includes(next.role)) throw new Error(t("error.state"));
    if (state && next.role === "peer" && state.sessionId !== next.sessionId) { revoke(); return; }
    trackReceipts(next, live);
    state = next;
    media.stateChanged(next);
    if (connected) setText("connection-label", onlineConnectionLabel());
    document.body.dataset.role = next.role;
    $("start-state").hidden = true;
    $("workspace").hidden = false;
    $("host-name").textContent = next.deviceName || "";
    if (next.role === "host") renderPairing();
    setText("drop-title", m(next.role === "peer" ? "file.peerTitle" : "file.dropTitle"));
    const clipboard = next.role === "host" ? next.clipboard : null;
    $("clipboard-status").hidden = !clipboard;
    $("clipboard-status").classList.toggle("inline-warning", clipboard?.status === "error");
    if (clipboard) {
      if (clipboard.status === "copied") setText("clipboard-status", m("clipboard.lastCopied"));
      else setText("clipboard-status", known(clipboard.error || t("clipboard.copyFailed")));
    }
    renderTexts();
    renderFiles();
    updateControls();
  }

  function renderPairing() {
    const addresses = Array.isArray(state.addresses) ? state.addresses : [];
    if (!addresses.some((address) => address.ip === selectedIP)) selectedIP = addresses[0]?.ip || "";
    const select = $("lan-address");
    select.replaceChildren();
    for (const address of addresses) {
      const option = node("option", "", `${address.name} · ${address.ip}`);
      option.value = address.ip;
      option.selected = address.ip === selectedIP;
      select.append(option);
    }
    if (!addresses.length) { const option = node("option", "", m("pair.lanUnavailable")); option.value = ""; select.append(option); }
    select.disabled = !addresses.length;
    $("no-lan").hidden = addresses.length > 0;
    updateJoinURL();
    const devices = Array.isArray(state.devices) ? state.devices : [];
    $("device-count").textContent = String(devices.length);
    document.querySelector(".pairing").classList.toggle("paired", devices.length > 0);
    $("device-list").replaceChildren();
    if (!devices.length) $("device-list").append(node("li", "empty", m("pair.empty")));
    for (const device of devices) {
      const row = node("li", "device-row");
      const body = node("div", "row-body");
      body.append(node("p", "device-name", device.name || m("pair.device")), formatted("p", "metadata", () => t("pair.connectedAt", { date: dateLabel(device.connectedAt) })));
      row.append(icon("phone"), body);
      $("device-list").append(row);
    }
  }

  function updateJoinURL() {
    const address = (state.addresses || []).find((entry) => entry.ip === selectedIP);
    $("join-url").value = address?.url || state.joinUrl || "";
    const qrURL = `/api/qr?${new URLSearchParams({ ...(selectedIP ? { ip: selectedIP } : {}), session: state.sessionId || "" })}`;
    if ($("qr").getAttribute("src") !== qrURL) {
      $("qr").hidden = false;
      $("qr-state").hidden = true;
      $("qr").src = qrURL;
    }
    updateControls();
  }

  function renderTexts() {
    const entries = Array.isArray(state.texts) ? state.texts : [];
    $("text-count").textContent = String(entries.length);
    $("text-list").replaceChildren();
    if (!entries.length) $("text-list").append(node("li", "empty", m("text.empty")));
    for (const entry of entries) {
      const row = node("li", "activity-row");
      revealReceipt(row, "texts", entry.id);
      const body = node("div", "row-body");
      const content = node("p", "text-content");
      appendLinkedText(content, entry.text);
      const expanded = expandedTexts.has(entry.id);
      const expandable = entry.text.length > 180 || entry.text.split("\n").length > 4;
      if (expandable && !expanded) content.classList.add("collapsed");
      body.append(content);
      if (expandable) {
        const expand = button(m(expanded ? "text.collapse" : "text.expand"), "quiet expand-button", () => {
          const isExpanded = expandedTexts.has(entry.id);
          if (isExpanded) expandedTexts.delete(entry.id); else expandedTexts.add(entry.id);
          content.classList.toggle("collapsed", isExpanded);
          setText(expand.querySelector("span"), m(isExpanded ? "text.expand" : "text.collapse"));
          expand.setAttribute("aria-expanded", String(!isExpanded));
        });
        expand.setAttribute("aria-expanded", String(expanded));
        body.append(expand);
      }
      body.append(formatted("p", "metadata", () => `${entry.sender} · ${dateLabel(entry.createdAt)}`));
      row.append(body, button(m("action.copy"), "secondary row-action", () => copyText(entry.text, m("action.copyText")), "copy"));
      $("text-list").append(row);
    }
  }

  function safeTextURL(value) {
    if (/\\|[\u0000-\u001f\u007f]/.test(value)) return null;
    try {
      const url = new URL(value);
      return ["http:", "https:"].includes(url.protocol) && url.hostname && !url.username && !url.password ? url.href : null;
    } catch { return null; }
  }

  function appendLinkedText(element, text) {
    let offset = 0;
    for (const match of text.matchAll(/https?:\/\/[^\s<>"`]+/gi)) {
      if (match.index && /[\p{L}\p{N}_:/@.%-]/u.test(text[match.index - 1])) continue;
      let start = match.index;
      while (start && !/\s/u.test(text[start - 1])) start--;
      if (/(?:^|[('"=\[{])(?:javascript|data|vbscript):/i.test(text.slice(start, match.index))) continue;
      const raw = match[0];
      const balance = { ")": 0, "]": 0, "}": 0 };
      const opening = { "(": ")", "[": "]", "{": "}" };
      for (const character of raw) {
        if (opening[character]) balance[opening[character]]--;
        else if (Object.hasOwn(balance, character)) balance[character]++;
      }
      let end = raw.length;
      while (end) {
        const character = raw[end - 1];
        if (/[.,!?;:'\u2019\u201d]/u.test(character)) end--;
        else if (balance[character] > 0) { balance[character]--; end--; }
        else break;
      }
      const value = raw.slice(0, end);
      const url = safeTextURL(value);
      if (!url) continue;
      element.append(document.createTextNode(text.slice(offset, match.index)));
      const link = node("a", "text-link", value);
      link.href = url;
      link.target = "_blank";
      link.rel = "noopener noreferrer";
      element.append(link);
      offset = match.index + value.length;
    }
    element.append(document.createTextNode(text.slice(offset)));
  }

  function safeDownloadURL(value) {
    try {
      const url = new URL(value, location.origin);
      return url.origin === location.origin && url.pathname.startsWith("/api/files/") ? url.href : null;
    } catch { return null; }
  }

  function renderFiles() {
    const files = Array.isArray(state.files) ? state.files : [];
    $("file-count").textContent = String(files.length);
    $("file-list").replaceChildren();
    if (!files.length) $("file-list").append(node("li", "empty", m("file.empty")));
    for (const file of files) {
      const row = node("li", "activity-row");
      revealReceipt(row, "files", file.id);
      const body = node("div", "row-body");
      body.append(node("p", "file-name", file.name), formatted("p", "metadata", () => `${sizeLabel(file.size)} · ${file.sender} · ${dateLabel(file.createdAt)}`));
      row.append(body);
      const url = safeDownloadURL(file.downloadUrl);
      if (url) {
        const link = node("a", "button secondary row-action");
        link.href = url;
        link.download = file.name;
        link.append(icon("download"), node("span", "", m("file.download")));
        row.append(link);
      } else body.append(node("p", "inline-warning", m("file.noDownload")));
      $("file-list").append(row);
    }
  }

  async function loadState(initial = false) {
    if (loadingState || revoked) return false;
    loadingState = true;
    const requestStateVersion = socketStateVersion;
    try {
      const snapshot = await api("/api/state");
      // Prefer live WebSocket state if it advanced while HTTP was in flight.
      if (!revoked && requestStateVersion === socketStateVersion) applyState(snapshot);
      return !revoked;
    } catch (error) {
      if (!revoked) {
        connection("offline", m("connection.offline"));
        if (initial && !state) {
          setText("start-title", m("start.offlineTitle"));
          setText("start-message", m("start.offlineHelp"));
          $("retry-start").hidden = false;
        } else notice(known(error.message || t("error.contact")), true);
      }
      return false;
    } finally { loadingState = false; }
  }

  async function connectSocket() {
    if (revoked || socket?.readyState === WebSocket.OPEN || socket?.readyState === WebSocket.CONNECTING) return;
    if (!navigator.onLine) { connection("offline", m("connection.noNetwork")); return; }
    if (!state && !(await loadState(true))) { scheduleReconnect(); return; }
    connection("connecting", m(reconnectAttempt ? "connection.reconnecting" : "connection.connecting"));
    const protocol = location.protocol === "https:" ? "wss:" : "ws:";
    const url = `${protocol}//${location.host}/ws?${new URLSearchParams({ id: clientID, name: state.role === "host" ? state.deviceName || "Laptop" : deviceLabel })}`;
    try { socket = new WebSocket(url); } catch { connection("offline", m("connection.offline")); scheduleReconnect(); return; }
    const current = socket;
    let receivedSnapshot = false;
    current.addEventListener("open", () => {
      if (current !== socket || revoked) return;
      reconnectAttempt = 0;
      connection("online", m("connection.online"));
    });
    current.addEventListener("message", (event) => {
      if (current !== socket || revoked) return;
      try {
        const message = JSON.parse(event.data);
        if (message.type === "state") { socketStateVersion++; applyState(message.state, receivedSnapshot); receivedSnapshot = true; }
        else if (["revoked", "session_revoked"].includes(message.type)) revoke();
        else if (message.type.startsWith("media-")) {
          if (message.type === "media-invite" && state?.role === "peer") {
            trackMediaReceipt(message.id, true);
            clearViewedReceipts();
          }
          media.handleMessage(message);
        }
      } catch { notice(m("error.liveState"), true); current.close(); }
    });
    current.addEventListener("close", () => {
      if (current !== socket || revoked) return;
      socket = null;
      connection("offline", m("connection.lost"));
      scheduleReconnect();
    });
    current.addEventListener("error", () => { if (current === socket) connection("offline", m("connection.lost")); });
  }

  function scheduleReconnect() {
    if (revoked || reconnectTimer) return;
    const delay = Math.min(15000, 1000 * (2 ** Math.min(reconnectAttempt++, 4)));
    reconnectTimer = setTimeout(async () => {
      reconnectTimer = null;
      if (revoked || !navigator.onLine) return;
      if (await loadState()) connectSocket(); else scheduleReconnect();
    }, delay);
  }

  function selectTab(tab, focus = false) {
    selectedTab = tab;
    for (const name of ["text", "files", "media", "guide"]) {
      const selected = name === tab;
      $( `tab-${name}` ).setAttribute("aria-selected", String(selected));
      $( `tab-${name}` ).tabIndex = selected ? 0 : -1;
      $( `panel-${name}` ).hidden = !selected;
    }
    clearViewedReceipts();
    if (focus) $(`tab-${tab}`).focus();
  }

  function openDialog(title, build) {
    closeDialog();
    dialogReturnFocus = document.activeElement;
    setText("dialog-title", title);
    $("dialog-body").replaceChildren();
    build($("dialog-body"));
    if (typeof $("dialog").showModal === "function") $("dialog").showModal();
    else { $("dialog").setAttribute("open", ""); $("dialog").classList.add("fallback"); $("workspace").inert = true; }
    const focus = $("dialog-body").querySelector("[autofocus],textarea,input,button,a");
    (focus || $("dialog-close")).focus();
  }

  function closeDialog() {
    if (!$("dialog").open) return;
    if (typeof $("dialog").close === "function") $("dialog").close(); else $("dialog").removeAttribute("open");
    $("dialog").classList.remove("fallback");
    $("workspace").inert = false;
    if (dialogReturnFocus?.isConnected) dialogReturnFocus.focus();
  }

  async function copyText(text, title = m("action.copyLink")) {
    try {
      if (!navigator.clipboard?.writeText) throw new Error(t("clipboard.unavailable"));
      await navigator.clipboard.writeText(text);
      notice(m("clipboard.copied"));
    } catch {
      openDialog(title, (body) => {
        body.append(node("p", "dialog-copy", m("clipboard.manualCopy")));
        const area = node("textarea", "copy-area");
        area.value = text;
        area.readOnly = true;
        i18n.bind(area, "clipboard.copyLabel", {}, "aria-label");
        body.append(area);
        const actions = node("div", "dialog-actions");
        actions.append(button(m("action.all"), "secondary", () => { area.focus(); area.select(); area.setSelectionRange(0, area.value.length); }), button(m("action.done"), "primary", closeDialog));
        body.append(actions);
      });
    }
  }

  function canReadClipboard() {
    return window.isSecureContext === true && typeof navigator.clipboard?.readText === "function";
  }

  function clearPasteGuidance() {
    if ($("paste-guidance")) $("paste-guidance").hidden = true;
  }

  function guideManualPaste() {
    manualPaste = true;
    const area = $("text-input");
    const start = area.selectionStart;
    const end = area.selectionEnd;
    const direction = area.selectionDirection;
    const guidance = $("paste-guidance");
    if (guidance) {
      setText(guidance, m(appleMobile || navigator.maxTouchPoints > 0 ? "clipboard.pasteHintTouch" : "clipboard.pasteHintKeyboard"));
      guidance.hidden = false;
      const descriptions = new Set((area.getAttribute("aria-describedby") || "").split(/\s+/).filter(Boolean));
      descriptions.add("paste-guidance");
      area.setAttribute("aria-describedby", [...descriptions].join(" "));
    }
    area.focus();
    area.setSelectionRange(start, end, direction);
  }

  async function pasteText() {
    if (!state || revoked) return;
    const control = $("paste-text");
    control.disabled = true;
    try {
      let text;
      if (state.role === "host") text = (await api("/api/clipboard")).text;
      else if (canReadClipboard()) text = await navigator.clipboard.readText();
      else { guideManualPaste(); return; }
      if (!text) { notice(m("clipboard.empty")); return; }
      const area = $("text-input");
      const draft = area.value.slice(0, area.selectionStart) + text + area.value.slice(area.selectionEnd);
      if (new TextEncoder().encode(draft).length > MAX_TEXT_BYTES) { notice(m("text.tooLarge"), true); return; }
      area.setRangeText(text, area.selectionStart, area.selectionEnd, "end");
      manualPaste = false;
      clearPasteGuidance();
      area.focus();
      updateControls();
    } catch (error) {
      if (state.role === "peer") guideManualPaste();
      else { notice(known(error.message), true); $("text-input").focus(); }
    } finally { updateControls(); }
  }

  async function sendText(event) {
    event.preventDefault();
    if (sending || !connected || revoked) return;
    const text = $("text-input").value;
    if (!text.trim()) return;
    if (new TextEncoder().encode(text).length > MAX_TEXT_BYTES) { notice(m("text.tooLarge"), true); return; }
    sending = true;
    updateControls();
    try {
      const result = await api("/api/text", { method: "POST", body: JSON.stringify({ text }) });
      if ($("text-input").value === text) $("text-input").value = "";
      if (result.clipboardError) {
        notice(m("text.sent"), true);
        const label = $("notice-text");
        delete label.dataset.i18n;
        label.dataset.localFormat = "";
        label.localDropFormat = () => t("text.clipboardError", { error: i18n.error(result.clipboardError) });
        label.textContent = label.localDropFormat();
      } else notice(m(result.clipboardCopied ? "text.sentCopied" : "text.sent"));
      await loadState();
    } catch (error) { if (!revoked) notice(known(error.message || t("text.sendError")), true); }
    finally { sending = false; updateControls(); }
  }

  function fileType(file) {
    return typeof file.type === "string" ? file.type.split(";", 1)[0].trim().toLowerCase() : "";
  }

  function emptyPhoto(file) {
    return file.size === 0 && (fileType(file).startsWith("image/") || /\.(jpe?g|png|heic|heif|webp|avif|gif|tiff?|bmp|svg|jp2|jxl)$/i.test(file.name || ""));
  }

  function fileSnapshot(list) {
    try {
      return Array.from(list || []).filter((file) => {
        try { return file instanceof Blob && Number.isFinite(file.size) && file.size >= 0; }
        catch { return false; }
      });
    } catch { return []; }
  }

  function droppedFiles(transfer) {
    let files = [];
    let hasFilePayload = false;
    let hasOtherPayload = false;
    try { files = fileSnapshot(transfer?.files); } catch { /* Safari may hide a protected drag store. */ }
    hasFilePayload = files.length > 0;
    let items = [];
    try { items = Array.from(transfer?.items || []); } catch { /* Try types for a useful fallback message. */ }
    const itemFiles = [];
    for (const item of items) {
      try {
        if (item.kind !== "file") { hasOtherPayload = true; continue; }
        hasFilePayload = true;
        const file = item.getAsFile();
        itemFiles.push(...fileSnapshot([file]));
      } catch { /* One unreadable item must not hide the remaining files. */ }
    }
    try {
      const types = Array.from(transfer?.types || []);
      hasFilePayload ||= types.includes("Files");
      hasOtherPayload ||= types.some((type) => type !== "Files");
    } catch { /* Missing dataTransfer APIs are handled by the picker fallback. */ }
    // Use one source: the same file commonly appears in both files and items.
    const selected = itemFiles.some((file) => !emptyPhoto(file)) ? itemFiles : files.some((file) => !emptyPhoto(file)) ? files : itemFiles.length ? itemFiles : files;
    return { files: selected, hasFilePayload, hasOtherPayload };
  }

  function stopDropRead(item) {
    const preparation = item.preparation;
    if (!preparation) return;
    item.preparation = null;
    clearTimeout(preparation.timer);
    const reader = preparation.reader;
    reader.onload = reader.onerror = reader.onabort = null;
    if (reader.readyState === FileReader.LOADING) reader.abort();
  }

  function releaseQueuedFile(item) {
    stopDropRead(item);
    if (item.snapshotSize) { dropSnapshotBytes -= item.snapshotSize; item.snapshotSize = 0; }
    item.file = null;
  }

  function prepareDroppedFile(item, file) {
    if (file.size > MAX_DROP_SNAPSHOT_BYTES - dropSnapshotBytes) {
      item.status = "error";
      item.message = m("file.dropLimit");
      return;
    }
    item.status = "preparing";
    item.message = m("file.preparing");
    item.snapshotSize = file.size;
    dropSnapshotBytes += item.snapshotSize;
    const type = fileType(file);
    function failed() {
      releaseQueuedFile(item);
      item.status = "error";
      item.message = m("file.readError");
      renderQueue();
      pumpQueue();
    }
    try {
      const reader = new FileReader();
      const preparation = { reader, timer: null };
      item.preparation = preparation;
      reader.onload = () => {
        if (item.preparation !== preparation || revoked) return;
        try {
          if (!(reader.result instanceof ArrayBuffer) || reader.result.byteLength !== item.size) { failed(); return; }
          const snapshot = new Blob([reader.result], { type });
          if (snapshot.size !== item.size) { failed(); return; }
          stopDropRead(item);
          item.file = snapshot;
          item.status = "queued";
          item.message = m("file.waiting");
          renderQueue();
          pumpQueue();
        } catch { failed(); }
      };
      reader.onerror = reader.onabort = failed;
      preparation.timer = setTimeout(failed, DROP_READ_TIMEOUT);
      // Start the native provider read inside the drop event, before it is queued.
      reader.readAsArrayBuffer(file);
    } catch { failed(); }
  }

  function addFiles(files, dropped = false) {
    if (!connected || revoked) { notice(m("file.reconnect"), true); return; }
    let accepted = 0;
    for (const file of files) {
      if (emptyPhoto(file)) { notice(PHOTO_DROP_HELP, true); continue; }
      const id = ++uploadID;
      // Supply a multipart filename without rebuilding or converting the photo bytes.
      // Keep it on the queue item so retries use the same name.
      const name = typeof file.name === "string" && file.name.trim() ? file.name : `bobodrop-${Date.now().toString(36)}-${id}.${FILE_EXTENSIONS[fileType(file)] || "bin"}`;
      if (file.size > MAX_FILE_BYTES) { notice(m("file.tooLarge", { name }), true); continue; }
      const materialize = dropped && appleMobile;
      const item = { id, file: materialize ? null : file, name, size: file.size, status: "queued", percent: 0, message: m("file.waiting") };
      queue.push(item);
      if (materialize) prepareDroppedFile(item, file);
      accepted++;
    }
    if (!accepted) return;
    selectTab("files");
    renderQueue();
    pumpQueue();
  }

  function renderQueue() {
    $("queue-section").hidden = queue.length === 0;
    $("queue-list").replaceChildren();
    $("clear-queue").disabled = !queue.some((item) => ["done", "cancelled", "error"].includes(item.status));
    for (const item of queue) {
      const row = node("li", "queue-row");
      const body = node("div", "row-body");
      body.append(node("p", "file-name", item.name));
      if (["uploading", "processing"].includes(item.status)) {
        const progress = node("progress");
        progress.max = 100;
        progress.value = item.percent;
        i18n.bind(progress, "file.progressLabel", { name: item.name }, "aria-label");
        body.append(progress);
      }
      body.append(formatted("p", `metadata${item.status === "error" ? " queue-error" : ""}`, () => `${sizeLabel(item.size)} · ${messageText(item.message)}`));
      row.append(body);
      if (["preparing", "queued", "uploading", "processing"].includes(item.status)) row.append(button(m("action.cancelUpload"), "quiet row-action", () => cancelUpload(item)));
      if (item.status === "error" && item.file) {
        const retry = button(m("action.retry"), "secondary row-action", () => { item.status = "queued"; item.percent = 0; item.message = m("file.waiting"); renderQueue(); pumpQueue(); });
        retry.disabled = !connected || revoked;
        row.append(retry);
      }
      $("queue-list").append(row);
    }
  }

  function cancelUpload(item) {
    if (activeUpload?.item === item) activeUpload.xhr.abort();
    else { item.status = "cancelled"; item.message = m("file.cancelled"); releaseQueuedFile(item); renderQueue(); pumpQueue(); }
  }

  function pumpQueue() {
    if (activeUpload || !connected || revoked) return;
    const item = queue.find((entry) => ["preparing", "queued"].includes(entry.status));
    if (!item || item.status === "preparing") return;
    const xhr = new XMLHttpRequest();
    activeUpload = { item, xhr };
    item.status = "uploading";
    item.message = m("file.progress", { percent: 0 });
    xhr.open("POST", "/api/files");
    xhr.setRequestHeader("X-Device-ID", clientID);
    xhr.setRequestHeader("X-File-Size", String(item.size));
    xhr.withCredentials = true;
    xhr.upload.addEventListener("progress", (event) => {
      if (activeUpload?.xhr !== xhr) return;
      item.percent = event.lengthComputable ? Math.min(100, Math.round(event.loaded / event.total * 100)) : 0;
      item.status = event.lengthComputable && event.loaded >= event.total ? "processing" : "uploading";
      item.message = item.status === "processing" ? m("file.saving") : event.lengthComputable ? m("file.progress", { percent: item.percent }) : m("text.sending");
      renderQueue();
    });
    function finish(status, message) {
      if (activeUpload?.xhr !== xhr) return;
      item.status = status;
      item.message = message;
      if (status === "done" || status === "cancelled") releaseQueuedFile(item);
      activeUpload = null;
      renderQueue();
      pumpQueue();
    }
    xhr.addEventListener("load", () => {
      let data;
      try { data = JSON.parse(xhr.responseText); } catch { /* Error response may not contain JSON. */ }
      if (xhr.status === 401) { revoke(); finish("error", m("file.expired")); return; }
      if (xhr.status >= 200 && xhr.status < 300 && Array.isArray(data?.files) && data.files.length) {
        finish("done", m("file.sent"));
        loadState();
      } else finish("error", data?.error ? known(data.error) : m("file.saveError"));
    });
    xhr.addEventListener("error", () => finish("error", m("file.networkError")));
    xhr.addEventListener("abort", () => finish("cancelled", m(revoked ? "connection.ended" : "file.cancelled")));
    renderQueue();
    try {
      const payload = new FormData();
      payload.append("files", item.file, item.name);
      xhr.send(payload);
    } catch { finish("error", m("file.readError")); }
  }

  function confirmAction(title, description, label, action) {
    openDialog(title, (body) => {
      body.append(node("p", "dialog-copy", description));
      const error = node("p", "dialog-error");
      error.setAttribute("role", "alert");
      const actions = node("div", "dialog-actions");
      const cancel = button(m("action.cancel"), "secondary", closeDialog);
      const confirm = button(label, "danger", async () => {
        confirm.disabled = true;
        cancel.disabled = true;
        $("dialog-close").disabled = true;
        setText(error, "");
        try { await action(); closeDialog(); }
        catch (failure) { setText(error, known(failure.message || t("error.change"))); }
        finally { confirm.disabled = false; cancel.disabled = false; $("dialog-close").disabled = false; }
      });
      actions.append(cancel, confirm);
      body.append(error, actions);
    });
  }

  function settingsDialog() {
    if (state?.role !== "host" || revoked) return;
    openDialog(m("action.settings"), (body) => {
      const label = node("label", "setting-row");
      const toggle = node("input");
      toggle.type = "checkbox";
      toggle.checked = !!state.settings?.autoCopy;
      label.append(toggle, node("span", "", m("settings.autoCopy")));
      body.append(label, node("p", "dialog-copy", m("settings.autoCopyHelp")));
      const error = node("p", "dialog-error");
      error.setAttribute("role", "alert");
      body.append(error);
      toggle.addEventListener("change", async () => {
        const requested = toggle.checked;
        toggle.disabled = true;
        setText(error, "");
        try { applyState(await api("/api/settings", { method: "POST", body: JSON.stringify({ autoCopy: requested }) })); notice(m("settings.saved")); }
        catch (failure) { toggle.checked = !!state.settings?.autoCopy; setText(error, known(failure.message)); }
        finally { toggle.disabled = false; }
      });
      const storage = node("div", "setting-section");
      storage.append(node("h3", "", m("settings.storage")), node("p", "metadata", state.storagePath || m("settings.noStorage")), button(m("action.folder"), "secondary", openFolder));
      body.append(storage);
      const history = node("div", "setting-section");
      history.append(node("h3", "", m("settings.history")), button(m("settings.clear"), "danger", () => confirmAction(m("settings.clearTitle"), m("settings.clearHelp"), m("settings.clearConfirm"), async () => {
        await api("/api/texts", { method: "DELETE" });
        await loadState();
        notice(m("settings.cleared"));
      })));
      body.append(history);
      const session = node("div", "setting-section");
      session.append(node("h3", "", m("settings.connections")), node("p", "metadata", m("settings.rotateHelp")), button(m("settings.disconnect"), "danger", () => confirmAction(m("settings.disconnectTitle"), m("settings.disconnectHelp"), m("settings.disconnect"), async () => {
        applyState(await api("/api/session/rotate", { method: "POST" }));
        notice(m("settings.disconnected"));
      })));
      body.append(session);
    });
  }

  async function openFolder() {
    const control = $("open-folder");
    control.disabled = true;
    try { await api("/api/open-folder", { method: "POST" }); notice(m("settings.folderOpened")); }
    catch (error) { if (!revoked) notice(known(error.message), true); }
    finally { updateControls(); }
  }

  $("text-form").addEventListener("submit", sendText);
  $("text-input").addEventListener("input", () => { clearPasteGuidance(); updateControls(); });
  $("text-input").addEventListener("paste", (event) => {
    const area = $("text-input");
    let text;
    try { text = event.clipboardData?.getData("text/plain"); } catch { /* Native paste remains available without readable event data. */ }
    if (typeof text === "string") {
      const draft = area.value.slice(0, area.selectionStart) + text + area.value.slice(area.selectionEnd);
      if (new TextEncoder().encode(draft).length > MAX_TEXT_BYTES) { event.preventDefault(); notice(m("text.tooLarge"), true); return; }
    }
    clearPasteGuidance();
    setTimeout(updateControls, 0);
  });
  $("paste-text").addEventListener("click", pasteText);
  $("copy-link").addEventListener("click", () => copyText($("join-url").value));
  $("lan-address").addEventListener("change", () => { selectedIP = $("lan-address").value; updateJoinURL(); });
  $("qr").addEventListener("error", () => { $("qr").hidden = true; $("qr-state").hidden = false; });
  $("qr").addEventListener("load", () => { $("qr").hidden = false; $("qr-state").hidden = true; });
  $("dismiss-notice").addEventListener("click", () => { $("notice").hidden = true; });
  $("settings").addEventListener("click", settingsDialog);
  $("open-folder").addEventListener("click", openFolder);
  $("retry-start").addEventListener("click", async () => { $("retry-start").disabled = true; if (await loadState(true)) connectSocket(); $("retry-start").disabled = false; });
  $("tab-text").addEventListener("click", () => selectTab("text"));
  $("tab-files").addEventListener("click", () => selectTab("files"));
  $("tab-media").addEventListener("click", () => selectTab("media"));
  $("tab-guide").addEventListener("click", () => selectTab("guide"));
  document.querySelector(".tabs").addEventListener("keydown", (event) => {
    if (!["ArrowLeft", "ArrowRight", "Home", "End"].includes(event.key)) return;
    event.preventDefault();
    const tabs = ["text", "files", "media", "guide"];
    const next = (tabs.indexOf(selectedTab) + (event.key === "ArrowLeft" ? -1 : 1) + tabs.length) % tabs.length;
    selectTab(event.key === "Home" ? "text" : event.key === "End" ? "guide" : tabs[next], true);
  });
  $("choose-file").addEventListener("click", () => $("file-input").click());
  $("choose-photo").addEventListener("click", () => $("photo-input").click());
  for (const id of ["file-input", "photo-input"]) $(id).addEventListener("change", () => { addFiles(fileSnapshot($(id).files)); $(id).value = ""; });
  $("drop-zone").addEventListener("dragenter", (event) => { event.preventDefault(); dropDepth++; if (connected) $("drop-zone").classList.add("drag-over"); });
  $("drop-zone").addEventListener("dragover", (event) => { event.preventDefault(); try { if (event.dataTransfer) event.dataTransfer.dropEffect = connected ? "copy" : "none"; } catch { /* Protected drag stores can reject writes. */ } });
  $("drop-zone").addEventListener("dragleave", () => { dropDepth = Math.max(0, dropDepth - 1); if (!dropDepth) $("drop-zone").classList.remove("drag-over"); });
  $("drop-zone").addEventListener("drop", (event) => {
    event.preventDefault();
    dropDepth = 0;
    $("drop-zone").classList.remove("drag-over");
    // Snapshot getAsFile handles synchronously while the drop store is readable.
    let dropped;
    try { dropped = droppedFiles(event.dataTransfer); } catch { dropped = { files: [] }; }
    if (!dropped.files.some((file) => !emptyPhoto(file))) {
      notice(dropped.hasOtherPayload && !dropped.hasFilePayload ? m("file.dropError") : PHOTO_DROP_HELP, true);
      return;
    }
    addFiles(dropped.files, true);
  });
  function preventFileNavigation(event) {
    try { if (Array.from(event.dataTransfer?.types || []).includes("Files")) event.preventDefault(); } catch { /* No readable drag metadata. */ }
  }
  document.addEventListener("dragover", preventFileNavigation);
  document.addEventListener("drop", preventFileNavigation);
  $("clear-queue").addEventListener("click", () => { for (let i = queue.length - 1; i >= 0; i--) if (["done", "cancelled", "error"].includes(queue[i].status)) { releaseQueuedFile(queue[i]); queue.splice(i, 1); } renderQueue(); });
  $("dialog-close").addEventListener("click", closeDialog);
  $("dialog").addEventListener("cancel", (event) => { event.preventDefault(); if (!$("dialog-close").disabled) closeDialog(); });
  $("dialog").addEventListener("keydown", (event) => {
    if (event.key === "Escape") { event.preventDefault(); if (!$("dialog-close").disabled) closeDialog(); }
    if (event.key !== "Tab") return;
    const focusable = [...$("dialog").querySelectorAll("button:not(:disabled),input:not(:disabled),textarea:not(:disabled),a[href],[tabindex='0']")].filter((element) => element.getClientRects().length);
    const first = focusable[0];
    const last = focusable[focusable.length - 1];
    if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last?.focus(); }
    if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first?.focus(); }
  });
  window.addEventListener("offline", () => { connection("offline", m("connection.noNetwork")); socket?.close(); });
  window.addEventListener("online", async () => { clearTimeout(reconnectTimer); reconnectTimer = null; if (await loadState()) connectSocket(); });
  window.addEventListener("pageshow", (event) => { if (event.persisted && !revoked) { loadState().then(() => connectSocket()); } });
  document.addEventListener("visibilitychange", () => {
    clearViewedReceipts();
    if (!document.hidden && !revoked && socket?.readyState !== WebSocket.OPEN) { clearTimeout(reconnectTimer); reconnectTimer = null; loadState().then(() => connectSocket()); }
  });
  window.addEventListener("beforeunload", (event) => { if (activeUpload || queue.some((item) => ["preparing", "queued"].includes(item.status))) { event.preventDefault(); event.returnValue = ""; } });

  i18n.onChange(() => {
    applyTheme(theme);
    for (const element of document.querySelectorAll("[data-local-format]")) {
      if (typeof element.localDropFormat === "function") element.textContent = element.localDropFormat();
    }
    updateControls();
    updateUnreadBadges();
  });

  updateControls();
  updateUnreadBadges();
  if (typeof WebSocket === "undefined") {
    setText("start-title", m("start.unsupportedTitle"));
    setText("start-message", m("start.unsupportedHelp"));
    connection("offline", m("connection.unsupported"));
  } else loadState(true).then((loaded) => { if (loaded) connectSocket(); else if (!revoked) scheduleReconnect(); });
})();
