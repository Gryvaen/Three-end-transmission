/**
 * LanRoom 前端：WebSocket 群聊、文件上传、连接信息（二维码）。
 * 协议约定见 README —— presence 广播在线列表，message 广播聊天内容。
 */

const STORAGE_KEY = "lanroom-device-name";
const FILE_ID_RE = /^[a-f0-9]{32}$/;

const platformIcons = {
  android: "🤖",
  windows: "🪟",
  linux: "🐧",
  ios: "📱",
  macos: "🍎",
  unknown: "💻",
};

/** 各平台 UI 文案与主题色 */
const platformUI = {
  android: {
    badge: "Android 版",
    subtitle: "Material 风格",
    defaultName: "Android 设备",
    namePlaceholder: "例如：小明的手机",
    composerPlaceholder: "发消息…",
    themeColor: "#111b21",
  },
  ios: {
    badge: "iOS 版",
    subtitle: "轻触即用",
    defaultName: "iPhone / iPad",
    namePlaceholder: "例如：iPhone",
    composerPlaceholder: "iMessage…",
    themeColor: "#000000",
  },
  windows: {
    badge: "Windows 版",
    subtitle: "Fluent 风格",
    defaultName: "Windows PC",
    namePlaceholder: "例如：DESKTOP-PC",
    composerPlaceholder: "输入消息…",
    themeColor: "#202020",
  },
  linux: {
    badge: "Linux 版",
    subtitle: "GNOME 风格 · 文件可用 lanroom-cli 发送",
    defaultName: "Linux 设备",
    namePlaceholder: "例如：arch-pc",
    composerPlaceholder: "输入消息…",
    themeColor: "#241f31",
  },
  macos: {
    badge: "macOS 版",
    subtitle: "桌面风格",
    defaultName: "Mac",
    namePlaceholder: "例如：MacBook",
    composerPlaceholder: "输入消息…",
    themeColor: "#1e1e1e",
  },
  unknown: {
    badge: "网页版",
    subtitle: "局域网聊天式互传 · 浏览器打开即用",
    defaultName: "我的设备",
    namePlaceholder: "例如：我的设备",
    composerPlaceholder: "输入消息…",
    themeColor: "#0f1419",
  },
};

// DOM 引用集中管理，避免重复 querySelector
const els = {
  joinScreen: document.getElementById("join-screen"),
  chatScreen: document.getElementById("chat-screen"),
  sidebar: document.getElementById("sidebar"),
  sidebarBackdrop: document.getElementById("sidebar-backdrop"),
  platformBadge: document.getElementById("platform-badge"),
  toggleDevicesBtn: document.getElementById("toggle-devices-btn"),
  deviceName: document.getElementById("device-name"),
  joinBtn: document.getElementById("join-btn"),
  showInfoBtn: document.getElementById("show-info-btn"),
  leaveBtn: document.getElementById("leave-btn"),
  infoPanelBtn: document.getElementById("info-panel-btn"),
  deviceList: document.getElementById("device-list"),
  onlineCount: document.getElementById("online-count"),
  messages: document.getElementById("messages"),
  messageInput: document.getElementById("message-input"),
  sendBtn: document.getElementById("send-btn"),
  attachBtn: document.getElementById("attach-btn"),
  fileInput: document.getElementById("file-input"),
  connStatus: document.getElementById("conn-status"),
  selfLabel: document.getElementById("self-label"),
  infoDialog: document.getElementById("info-dialog"),
  closeInfoBtn: document.getElementById("close-info-btn"),
  urlList: document.getElementById("url-list"),
  qrAndroid: document.getElementById("qr-android"),
  qrIos: document.getElementById("qr-ios"),
  qrIosCard: document.getElementById("qr-ios-card"),
  qrAndroidUrl: document.getElementById("qr-android-url"),
  qrIosUrl: document.getElementById("qr-ios-url"),
};

let ws = null;
let selfDevice = null;
let chatName = null;
let reconnectTimer = null;

// --- 工具函数 ---

/** 从 User-Agent 推断平台，连接 WebSocket 时传给服务端 */
function detectPlatform() {
  const ua = navigator.userAgent.toLowerCase();
  if (ua.includes("android")) return "android";
  if (/iphone|ipad|ipod/.test(ua)) return "ios";
  if (ua.includes("windows")) return "windows";
  if (ua.includes("mac os") || ua.includes("macintosh")) return "macos";
  if (ua.includes("linux")) return "linux";
  return "unknown";
}

function isIOSChrome() {
  return /CriOS/i.test(navigator.userAgent);
}

/** 移动端：动态计算输入栏高度与键盘偏移 */
let mobileViewportInited = false;

function initMobileViewportFix() {
  const platform = detectPlatform();
  if (platform !== "android" && platform !== "ios") return;
  if (mobileViewportInited) return;

  const composer = document.querySelector(".composer");
  if (!composer) return;

  mobileViewportInited = true;

  const update = () => {
    if (!document.body.classList.contains("in-chat")) return;

    const h = composer.getBoundingClientRect().height;
    document.documentElement.style.setProperty("--composer-offset", `${Math.ceil(h + 12)}px`);

    let keyboardOffset = 0;
    if (window.visualViewport) {
      const vv = window.visualViewport;
      keyboardOffset = Math.max(0, window.innerHeight - vv.height - vv.offsetTop);
    }
    document.documentElement.style.setProperty("--keyboard-offset", `${keyboardOffset}px`);
  };

  update();
  window.addEventListener("resize", update);
  window.addEventListener("orientationchange", update);
  if (window.visualViewport) {
    window.visualViewport.addEventListener("resize", update);
    window.visualViewport.addEventListener("scroll", update);
  }
  if (typeof ResizeObserver !== "undefined") {
    new ResizeObserver(update).observe(composer);
  }
}

/** 滚动时才显示滚动条（设备列表、输入框） */
function initAutoHideScrollbars() {
  const hideDelay = 900;
  const selectors = "#device-list, #message-input";

  document.querySelectorAll(selectors).forEach((el) => {
    let timer = null;

    const show = () => {
      el.classList.add("is-scrolling");
      clearTimeout(timer);
      timer = setTimeout(() => el.classList.remove("is-scrolling"), hideDelay);
    };

    el.addEventListener("scroll", show, { passive: true });
    el.addEventListener("wheel", show, { passive: true });
  });
}

function addJoinHint(id, text) {
  const card = document.querySelector(".join-card");
  if (!card || document.getElementById(id)) return;
  const hint = document.createElement("p");
  hint.id = id;
  hint.className = "browser-hint";
  hint.textContent = text;
  card.insertBefore(hint, card.querySelector(".subtitle"));
}

/** 按平台应用主题、文案与布局（html[data-platform]） */
function initPlatformUI() {
  const platform = detectPlatform();
  const ui = platformUI[platform] || platformUI.unknown;

  document.documentElement.dataset.platform = platform;

  if (els.platformBadge) els.platformBadge.textContent = ui.badge;

  const subtitle = document.querySelector(".subtitle");
  if (subtitle) subtitle.textContent = ui.subtitle;

  els.deviceName.placeholder = ui.namePlaceholder;
  els.messageInput.placeholder = ui.composerPlaceholder;

  let themeMeta = document.querySelector('meta[name="theme-color"]');
  if (!themeMeta) {
    themeMeta = document.createElement("meta");
    themeMeta.name = "theme-color";
    document.head.appendChild(themeMeta);
  }
  themeMeta.content = ui.themeColor;

  if (platform === "ios") {
    let capable = document.querySelector('meta[name="apple-mobile-web-app-capable"]');
    if (!capable) {
      capable = document.createElement("meta");
      capable.name = "apple-mobile-web-app-capable";
      capable.content = "yes";
      document.head.appendChild(capable);
    }
  }

  if (isIOSChrome()) {
    addJoinHint(
      "ios-chrome-hint",
      "iOS 的 Chrome 常无法打开 .local 地址。请改用 Safari，或在地址栏输入局域网 IP（如 http://192.168.x.x:8787）。"
    );
  }

  if (platform === "android") {
    addJoinHint(
      "android-mdns-hint",
      "Android 无法解析 .local 域名（如 myarch.local）。请扫电脑 Hub 页「连接信息」里的 IP 二维码，或手动输入 http://192.168.x.x:8787。"
    );
  }
}

function isMobilePlatform() {
  const p = document.documentElement.dataset.platform;
  return p === "android" || p === "ios";
}

function setDevicesPanel(open) {
  if (!isMobilePlatform()) return;
  els.sidebar?.classList.toggle("open", open);
  els.sidebarBackdrop?.classList.toggle("visible", open);
}

function defaultDeviceName() {
  const platform = detectPlatform();
  return (platformUI[platform] || platformUI.unknown).defaultName;
}

function isInChat() {
  return document.body.classList.contains("in-chat");
}

function safeFileURL(fileId) {
  if (!fileId || !FILE_ID_RE.test(fileId)) return null;
  return `/api/files/${fileId}`;
}

function formatTime(ts) {
  const d = new Date(ts * 1000);
  return d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
}

function formatSize(bytes) {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

function setConnected(online) {
  els.connStatus.textContent = online ? "已连接" : "未连接";
  els.connStatus.classList.toggle("online", online);
  els.connStatus.classList.toggle("offline", !online);
}

/** 渲染左侧在线设备列表（由 presence 消息驱动） */
function renderDevices(users) {
  els.deviceList.innerHTML = "";
  els.onlineCount.textContent = String(users.length);

  const nameCount = {};
  users.forEach((user) => {
    const name = user.name || "匿名设备";
    nameCount[name] = (nameCount[name] || 0) + 1;
  });

  const nameIndex = {};

  users.forEach((user) => {
    let displayName = user.name || "匿名设备";
    if (nameCount[displayName] > 1) {
      nameIndex[displayName] = (nameIndex[displayName] || 0) + 1;
      displayName = `${displayName} #${nameIndex[displayName]}`;
    }

    const ip = user.ip || "未知";

    const platformKey = String(user.platform || "unknown");
    const icon = platformIcons[platformKey] || platformIcons.unknown;

    const li = document.createElement("li");
    li.className = "device-item";
    li.innerHTML = `
      <span class="device-icon">${icon}</span>
      <div>
        <div class="device-name">${escapeHTML(displayName)}</div>
        <div class="device-ip">${escapeHTML(ip)}</div>
        <div class="device-platform">${escapeHTML(platformKey)}</div>
      </div>
    `;
    els.deviceList.appendChild(li);
  });
}

/** 防止聊天内容 XSS（用户昵称、文字消息） */
function escapeHTML(str) {
  return String(str)
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;");
}

function escapeAttr(str) {
  return String(str)
    .replaceAll("&", "&amp;")
    .replaceAll('"', "&quot;")
    .replaceAll("<", "&lt;")
    .replaceAll("'", "&#39;");
}

/**
 * 将服务端广播的消息渲染到聊天区。
 * payload.kind: text | image | file
 */
function appendMessage(msg, isSelf) {
  const wrapper = document.createElement("div");
  wrapper.className = `msg${isSelf ? " self" : ""}`;

  const fromName = msg.from?.name || "未知设备";
  const time = formatTime(msg.timestamp || Math.floor(Date.now() / 1000));

  let body = "";
  const payload = msg.payload || {};

  if (payload.kind === "text") {
    body = `<div class="msg-bubble">${escapeHTML(payload.content || "")}</div>`;
  } else if (payload.kind === "image") {
    const imgUrl = safeFileURL(payload.fileId);
    body = imgUrl
      ? `<img class="msg-image" src="${escapeAttr(imgUrl)}" alt="图片" />`
      : `<div class="msg-bubble">[图片不可用]</div>`;
  } else if (payload.kind === "file") {
    const name = payload.meta?.name || "文件";
    const size = payload.meta?.size ? formatSize(payload.meta.size) : "";
    const safeName = escapeHTML(name);
    body = `
      <div class="msg-bubble">
        <div class="file-card">
          <span>📎</span>
          <div>
            <div>${safeName}</div>
            <small>${size}</small><br />
            <button type="button" class="download-btn" data-file-id="${escapeAttr(payload.fileId)}" data-file-name="${escapeAttr(name)}">下载</button>
          </div>
        </div>
      </div>`;
  } else {
    body = `<div class="msg-bubble">[不支持的消息类型]</div>`;
  }

  wrapper.innerHTML = `
    <div class="msg-meta">${escapeHTML(fromName)} · ${time}</div>
    ${body}
  `;

  els.messages.appendChild(wrapper);
  els.messages.scrollTop = els.messages.scrollHeight;
}

/** 通过 fetch + Blob 下载（Linux/dwm 下比 <a download> 可靠） */
async function downloadFile(fileId, fileName) {
  const url = safeFileURL(fileId);
  if (!url) return;
  try {
    const resp = await fetch(url);
    if (!resp.ok) {
      alert("下载失败，文件可能已过期");
      return;
    }
    const blob = await resp.blob();
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = fileName || "download";
    a.rel = "noopener";
    document.body.appendChild(a);
    a.click();
    a.remove();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
  } catch (err) {
    alert(`下载失败：${err.message}`);
  }
}

// --- WebSocket ---

/** 建立 WebSocket；断线后在聊天页内自动重连 */
function connect(name) {
  if (ws) {
    ws.onclose = null;
    ws.close();
    ws = null;
  }

  const platform = detectPlatform();
  const params = new URLSearchParams({ name, platform });
  const protocol = location.protocol === "https:" ? "wss" : "ws";
  ws = new WebSocket(`${protocol}://${location.host}/ws?${params}`);

  ws.onopen = () => {
    setConnected(true);
    if (reconnectTimer) {
      clearTimeout(reconnectTimer);
      reconnectTimer = null;
    }
  };

  ws.onclose = () => {
    setConnected(false);
    ws = null;
    if (!isInChat() || !chatName) return;
    reconnectTimer = setTimeout(() => {
      if (isInChat() && chatName) connect(chatName);
    }, 2000);
  };

  ws.onerror = () => {
    setConnected(false);
  };

  ws.onmessage = (event) => {
    let data;
    try {
      data = JSON.parse(event.data);
    } catch {
      return;
    }

    if (data.type === "presence") {
      renderDevices(data.users || []);
      return;
    }

    if (data.type === "welcome") {
      selfDevice = data.device || null;
      return;
    }

    if (data.type === "history") {
      els.messages.innerHTML = "";
      (data.messages || []).forEach((msg) => {
        const isSelf = selfDevice && msg.from?.id === selfDevice.id;
        appendMessage(msg, isSelf);
      });
      return;
    }

    if (data.type === "message") {
      const isSelf = selfDevice && data.from?.id === selfDevice.id;
      appendMessage(data, isSelf);
    }
  };
}

/** 向服务端发送聊天消息（服务端会广播给所有在线客户端） */
function sendWS(payload) {
  if (!ws || ws.readyState !== WebSocket.OPEN) return;
  ws.send(JSON.stringify({ type: "message", payload }));
}

async function sendText() {
  const text = els.messageInput.value.trim();
  if (!text) return;
  sendWS({ kind: "text", content: text });
  els.messageInput.value = "";
}

/**
 * 文件/图片发送流程：先 HTTP 上传到 Hub，再通过 WebSocket 广播 fileId。
 * 其他设备收到消息后，从 /api/files/{id} 下载。
 */
async function uploadAndSend(file) {
  const form = new FormData();
  form.append("file", file);

  const resp = await fetch("/api/upload", { method: "POST", body: form });
  if (!resp.ok) {
    alert("上传失败，请重试");
    return;
  }

  const result = await resp.json();

  sendWS({
    kind: result.mime?.startsWith("image/") ? "image" : "file",
    fileId: result.fileId,
    meta: {
      name: result.name,
      size: result.size,
      mime: result.mime,
    },
  });
}

// --- 连接信息 / 二维码 ---

async function loadConnectionInfo() {
  const resp = await fetch("/api/info");
  if (!resp.ok) return null;
  return resp.json();
}

/**
 * 是否为局域网 IPv4 地址（适合手机扫码 / 二维码）。
 */
function isLANIPv4Origin(origin) {
  try {
    const host = new URL(origin).hostname.toLowerCase();
    return (
      /^192\.168\.\d{1,3}\.\d{1,3}$/.test(host) ||
      /^10\.\d{1,3}\.\d{1,3}\.\d{1,3}$/.test(host)
    );
  } catch {
    return false;
  }
}

function isMdnsURL(url) {
  try {
    return new URL(url).hostname.toLowerCase().endsWith(".local");
  } catch {
    return String(url).includes(".local");
  }
}

function firstIPv4JoinURL(info) {
  if (info.joinUrl && !isMdnsURL(info.joinUrl)) return info.joinUrl;
  for (const url of info.urls || []) {
    if (!isMdnsURL(url)) return url;
  }
  return "";
}

function firstMdnsJoinURL(info) {
  if (info.mdnsUrl) return info.mdnsUrl;
  if (info.joinUrl && isMdnsURL(info.joinUrl)) return info.joinUrl;
  for (const url of info.urls || []) {
    if (isMdnsURL(url)) return url;
  }
  return "";
}

/** Android 二维码：局域网 IPv4 */
function pickAndroidJoinURL(info) {
  if (isLANIPv4Origin(location.origin)) {
    return location.origin;
  }
  const ipv4 = firstIPv4JoinURL(info);
  if (ipv4) return ipv4;
  return info.joinUrl || location.href;
}

/**
 * 展示加入地址列表与双二维码（Android IPv4 / iOS .local）。
 */
async function showInfoDialog() {
  const info = await loadConnectionInfo();
  if (!info) {
    alert("无法获取连接信息");
    return;
  }

  els.urlList.innerHTML = "";

  const androidUrl = pickAndroidJoinURL(info);
  const iosUrl = firstMdnsJoinURL(info);
  const ts = Date.now();

  const items = [];

  (info.urls || []).forEach((url) => {
    const isMdns = isMdnsURL(url);
    let label = "局域网 IP";
    if (url === androidUrl) label = "Android 二维码";
    else if (url === iosUrl) label = "iOS 二维码";
    else if (isMdns) label = "mDNS（iOS / 电脑）";
    if (items.some((i) => i.url === url)) return;
    items.push({ label, url });
  });

  items.forEach((item) => {
    const div = document.createElement("div");
    div.className = "url-item";
    div.innerHTML = `<strong>${escapeHTML(item.label)}</strong>${escapeHTML(item.url)}`;
    els.urlList.appendChild(div);
  });

  if (els.qrAndroid && androidUrl) {
    els.qrAndroid.src = `/api/qrcode?url=${encodeURIComponent(androidUrl)}&t=${ts}&tag=android`;
    if (els.qrAndroidUrl) els.qrAndroidUrl.textContent = androidUrl;
  }

  if (els.qrIosCard && els.qrIos) {
    if (iosUrl) {
      els.qrIosCard.classList.remove("hidden");
      els.qrIos.src = `/api/qrcode?url=${encodeURIComponent(iosUrl)}&t=${ts}&tag=ios`;
      if (els.qrIosUrl) els.qrIosUrl.textContent = iosUrl;
    } else {
      els.qrIosCard.classList.add("hidden");
      els.qrIos.removeAttribute("src");
      if (els.qrIosUrl) els.qrIosUrl.textContent = "";
    }
  }

  els.infoDialog.showModal();
}

// --- 页面流程 ---

function enterChat(name) {
  chatName = name;
  localStorage.setItem(STORAGE_KEY, name);
  document.body.classList.add("in-chat");
  document.documentElement.classList.add("in-chat");
  els.joinScreen.classList.add("hidden");
  els.chatScreen.classList.remove("hidden");
  els.selfLabel.textContent = `当前身份：${name}`;
  setDevicesPanel(false);
  initMobileViewportFix();
  connect(name);
}

function leaveChat() {
  if (reconnectTimer) {
    clearTimeout(reconnectTimer);
    reconnectTimer = null;
  }
  if (ws) {
    ws.onclose = null;
    ws.close();
    ws = null;
  }
  chatName = null;
  selfDevice = null;
  document.body.classList.remove("in-chat");
  document.documentElement.classList.remove("in-chat");
  document.documentElement.style.removeProperty("--composer-offset");
  document.documentElement.style.removeProperty("--keyboard-offset");
  setDevicesPanel(false);
  els.chatScreen.classList.add("hidden");
  els.joinScreen.classList.remove("hidden");
  els.messages.innerHTML = "";
}

// --- 事件绑定 ---

els.joinBtn.addEventListener("click", () => {
  const name = els.deviceName.value.trim() || defaultDeviceName();
  enterChat(name);
});

els.showInfoBtn.addEventListener("click", showInfoDialog);
els.infoPanelBtn.addEventListener("click", showInfoDialog);
els.closeInfoBtn.addEventListener("click", () => els.infoDialog.close());
els.leaveBtn.addEventListener("click", leaveChat);

els.toggleDevicesBtn?.addEventListener("click", () => {
  setDevicesPanel(!els.sidebar?.classList.contains("open"));
});
els.sidebarBackdrop?.addEventListener("click", () => setDevicesPanel(false));

els.messages.addEventListener("click", (e) => {
  const btn = e.target.closest(".download-btn");
  if (!btn) return;
  e.preventDefault();
  downloadFile(btn.dataset.fileId, btn.dataset.fileName);
});

els.sendBtn.addEventListener("click", sendText);
els.messageInput.addEventListener("keydown", (e) => {
  // Enter 发送，Shift+Enter 换行
  if (e.key === "Enter" && !e.shiftKey) {
    e.preventDefault();
    sendText();
  }
});

// 📎 按钮触发 file input；Linux 下若无效可用 lanroom-cli
els.attachBtn.addEventListener("click", () => els.fileInput.click());
els.fileInput.addEventListener("change", async () => {
  const file = els.fileInput.files?.[0];
  els.fileInput.value = ""; // 允许重复选择同一文件
  if (file) await uploadAndSend(file);
});

// --- 初始化 ---

initPlatformUI();
initAutoHideScrollbars();

const savedName = localStorage.getItem(STORAGE_KEY);
els.deviceName.value = savedName || defaultDeviceName();

// 带 ?auto=1 时跳过进入页（方便手机扫码后直接进聊天）
if (location.search.includes("auto=1") && savedName) {
  enterChat(savedName);
}
