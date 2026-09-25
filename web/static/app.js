/**
 * LanRoom 前端：WebSocket 群聊 / 私信、断点续传上传、连接信息（二维码）、房间口令。
 * 协议约定见 README —— presence 广播在线列表，message 投递聊天内容（to 为空即群发）。
 */

const STORAGE_KEY = "lanroom-device-name";
const DEVICE_ID_KEY = "lanroom-device-id";
const UPLOAD_KEY_PREFIX = "lanroom-upload:";
const UPLOAD_MAX_RETRIES = 10;
const FILE_ID_RE = /^[a-f0-9]{32}$/;

const COMPOSER_PLACEHOLDER = "输入消息…，可粘贴图片";

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
    composerPlaceholder: "发消息…，可粘贴图片",
    themeColor: "#111b21",
  },
  ios: {
    badge: "iOS 版",
    subtitle: "轻触即用",
    defaultName: "iPhone / iPad",
    namePlaceholder: "例如：iPhone",
    composerPlaceholder: "iMessage…，可粘贴图片",
    themeColor: "#000000",
  },
  windows: {
    badge: "Windows 版",
    subtitle: "Fluent 风格",
    defaultName: "Windows PC",
    namePlaceholder: "例如：DESKTOP-PC",
    composerPlaceholder: COMPOSER_PLACEHOLDER,
    themeColor: "#202020",
  },
  linux: {
    badge: "Linux 版",
    subtitle: "GNOME 风格",
    defaultName: "Linux 设备",
    namePlaceholder: "例如：arch-pc",
    composerPlaceholder: COMPOSER_PLACEHOLDER,
    themeColor: "#241f31",
  },
  macos: {
    badge: "macOS 版",
    subtitle: "桌面风格",
    defaultName: "Mac",
    namePlaceholder: "例如：MacBook",
    composerPlaceholder: COMPOSER_PLACEHOLDER,
    themeColor: "#1e1e1e",
  },
  unknown: {
    badge: "网页版",
    subtitle: "局域网聊天式互传 · 浏览器打开即用",
    defaultName: "我的设备",
    namePlaceholder: "例如：我的设备",
    composerPlaceholder: COMPOSER_PLACEHOLDER,
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
  qrJoin: document.getElementById("qr-join"),
  qrJoinUrl: document.getElementById("qr-join-url"),
  copyChatBtn: document.getElementById("copy-chat-btn"),
  pinRow: document.getElementById("pin-row"),
  pinInput: document.getElementById("pin-input"),
  chatMain: document.getElementById("chat-main"),
  dropOverlay: document.getElementById("drop-overlay"),
  dropTarget: document.getElementById("drop-target"),
  targetBar: document.getElementById("target-bar"),
  targetName: document.getElementById("target-name"),
  targetOffline: document.getElementById("target-offline"),
  targetClear: document.getElementById("target-clear"),
  imageViewer: document.getElementById("image-viewer"),
  imageViewerImg: document.getElementById("image-viewer-img"),
  imageViewerDownload: document.getElementById("image-viewer-download"),
  imageViewerClose: document.getElementById("image-viewer-close"),
  dmHint: document.getElementById("dm-hint"),
  peerCount: document.getElementById("peer-count"),
  chatTitle: document.getElementById("chat-title"),
};

let ws = null;
let selfDevice = null;
let chatName = null;
let reconnectTimer = null;
/** 连续重连失败次数，用于指数退避（避免服务端短暂抖动时客户端高频重连放大掉线感） */
let reconnectAttempts = 0;
/** 当前会话消息列表，用于一键复制 */
let chatLog = [];
/** 进行中的上传，离开聊天室时取消 */
const activeUploads = new Set();
/** 私信目标 { id, name }；null 表示群发 */
let sendTarget = null;
/** 最近一次 presence 的在线设备 */
let onlineUsers = [];
/** 上次渲染的设备列表签名：内容没变就不重绘，避免 presence 广播打断手机上的点击 */
let devicesSignature = "";

// --- 本地存储（隐私模式下可能不可用） ---

function storageGet(key) {
  try {
    return localStorage.getItem(key);
  } catch {
    return null;
  }
}

function storageSet(key, value) {
  try {
    localStorage.setItem(key, value);
  } catch {
    /* 忽略 */
  }
}

function storageRemove(key) {
  try {
    localStorage.removeItem(key);
  } catch {
    /* 忽略 */
  }
}

/** 浏览器持久化的设备 ID：重连 / 刷新后保持不变，私信目标据此定位 */
function getDeviceId() {
  let id = storageGet(DEVICE_ID_KEY);
  if (id && /^[0-9a-f-]{36}$/i.test(id)) return id;
  if (crypto.randomUUID) {
    id = crypto.randomUUID(); // 仅安全上下文可用
  } else {
    const b = crypto.getRandomValues(new Uint8Array(16));
    b[6] = (b[6] & 0x0f) | 0x40;
    b[8] = (b[8] & 0x3f) | 0x80;
    const h = [...b].map((x) => x.toString(16).padStart(2, "0")).join("");
    id = `${h.slice(0, 8)}-${h.slice(8, 12)}-${h.slice(12, 16)}-${h.slice(16, 20)}-${h.slice(20)}`;
  }
  storageSet(DEVICE_ID_KEY, id);
  return id;
}

const deviceId = getDeviceId();

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

  if (isMobilePlatform()) {
    addJoinHint(
      "mobile-join-hint",
      "手机请扫 Hub 页「连接信息」里的二维码，或手动输入 http://192.168.x.x:8787。"
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

async function fetchFileBlob(fileId) {
  const url = safeFileURL(fileId);
  if (!url) return null;
  const resp = await fetch(url);
  if (!resp.ok) return null;
  return resp.blob();
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

function isOpen() {
  return ws && ws.readyState === WebSocket.OPEN;
}

/** 更新连接状态；断线时禁用发送，避免消息"看似发出"实则丢失 */
function setConnected(online) {
  els.connStatus.textContent = online ? "已连接" : isInChat() ? "重连中…" : "未连接";
  els.connStatus.classList.toggle("online", online);
  els.connStatus.classList.toggle("offline", !online);
  els.sendBtn.disabled = !online;
  els.attachBtn.disabled = !online;
}

/** 渲染左侧在线设备列表（由 presence 消息驱动）；点击其他设备切换私信目标 */
function renderDevices(users) {
  onlineUsers = users;
  const peers = users.filter((u) => u.id !== deviceId).length;
  els.dmHint.classList.toggle("hidden", peers === 0);
  els.peerCount.classList.toggle("hidden", peers === 0);
  els.peerCount.textContent = String(peers);

  const signature = JSON.stringify(users.map((u) => [u.id, u.name, u.ip, u.platform]));
  if (signature === devicesSignature) {
    renderTargetBar();
    return;
  }
  devicesSignature = signature;
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
    const isSelf = user.id === deviceId;

    const li = document.createElement("li");
    li.className = "device-item";
    if (!isSelf) {
      li.classList.add("selectable");
      li.dataset.deviceId = user.id;
      li.dataset.deviceName = displayName;
      li.title = "点击私信此设备";
      li.classList.toggle("selected", sendTarget?.id === user.id);
    }
    li.innerHTML = `
      <span class="device-icon">${icon}</span>
      <div>
        <div class="device-name">${escapeHTML(displayName)}${isSelf ? ' <span class="device-self">（本机）</span>' : ""}</div>
        <div class="device-ip">${escapeHTML(ip)}</div>
        <div class="device-platform">${escapeHTML(platformKey)}</div>
      </div>
    `;
    els.deviceList.appendChild(li);
  });

  renderTargetBar();
}

/** 设置 / 取消私信目标 */
function setSendTarget(target) {
  sendTarget = target;
  els.deviceList.querySelectorAll(".device-item.selectable").forEach((li) => {
    li.classList.toggle("selected", li.dataset.deviceId === target?.id);
  });
  renderTargetBar();
}

function renderTargetBar() {
  const target = sendTarget;
  els.targetBar.classList.toggle("hidden", !target);
  els.dropTarget.textContent = target ? target.name : "群聊";
  els.chatTitle.textContent = target ? `私聊 · ${target.name}` : "群聊";
  els.chatTitle.classList.toggle("private", !!target);
  if (!target) return;
  const online = onlineUsers.some((u) => u.id === target.id);
  els.targetName.textContent = target.name;
  els.targetOffline.classList.toggle("hidden", online);
  els.targetBar.classList.toggle("offline", !online);
}

/** 当前发送目标的 to 字段（群发返回 undefined） */
function currentRecipients() {
  return sendTarget ? [sendTarget.id] : undefined;
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

/** 将单条消息转为可复制的纯文本（文字 / 图片链接 / 文件名） */
function messageCopyText(msg) {
  const payload = msg.payload || {};
  if (payload.kind === "text") {
    return payload.content || "";
  }
  if (payload.kind === "image") {
    const url = safeFileURL(payload.fileId);
    return url ? `${location.origin}${url}` : "";
  }
  if (payload.kind === "file") {
    return payload.meta?.name || "";
  }
  return "";
}

/** 单条图片：优先复制图片本身，失败则复制链接 */
async function copyImageToClipboard(fileId) {
  const url = safeFileURL(fileId);
  if (!url) return false;

  const link = `${location.origin}${url}`;
  try {
    const blob = await fetchFileBlob(fileId);
    if (!blob) return copyToClipboard(link);
    const type = blob.type?.startsWith("image/") ? blob.type : "image/png";
    if (navigator.clipboard?.write && typeof ClipboardItem !== "undefined") {
      await navigator.clipboard.write([new ClipboardItem({ [type]: blob })]);
      return true;
    }
  } catch {
    /* 降级为链接 */
  }
  return copyToClipboard(link);
}

async function copyTextOrAlert(text, emptyHint) {
  const trimmed = text?.trim() || "";
  if (!trimmed) {
    alert(emptyHint || "没有可复制的文字内容");
    return;
  }
  const ok = await copyToClipboard(trimmed);
  if (!ok) alert("复制失败，请检查浏览器权限");
  return ok;
}

async function copyToClipboard(text) {
  if (!text?.trim()) return false;
  try {
    await navigator.clipboard.writeText(text);
    return true;
  } catch {
    const ta = document.createElement("textarea");
    ta.value = text;
    ta.style.cssText = "position:fixed;left:-9999px;top:0";
    document.body.appendChild(ta);
    ta.select();
    const ok = document.execCommand("copy");
    ta.remove();
    return ok;
  }
}

function flashCopyBtn(btn, ok, okLabel = "已复制", failLabel = "失败") {
  if (!btn) return;
  const prev = btn.textContent;
  btn.textContent = ok ? okLabel : failLabel;
  btn.disabled = true;
  setTimeout(() => {
    btn.textContent = prev;
    btn.disabled = false;
  }, 1600);
}

async function copyAllChat() {
  if (chatLog.length === 0) {
    alert("暂无聊天内容可复制");
    return;
  }
  const text = chatLog.map(messageCopyText).filter((line) => line.trim()).join("\n");
  const ok = await copyTextOrAlert(text, "暂无聊天内容可复制");
  if (ok) flashCopyBtn(els.copyChatBtn, true);
}

async function copyOneMessage(idx) {
  const msg = chatLog[idx];
  if (!msg) return;

  const payload = msg.payload || {};
  let ok = false;
  if (payload.kind === "image" && !canCopyImages()) {
    // http://局域网 IP 不是安全上下文，无法写入图片剪贴板：改为保存
    const btn = els.messages.querySelector(`.msg-copy-btn[data-msg-idx="${idx}"]`);
    await downloadFile(payload.fileId, payload.meta?.name || "image", btn);
    return;
  }
  if (payload.kind === "image") {
    ok = await copyImageToClipboard(payload.fileId);
    if (!ok) alert("图片复制失败，请检查权限或文件是否过期");
  } else {
    ok = await copyTextOrAlert(messageCopyText(msg), "该条没有可复制的文字内容");
  }

  if (ok) {
    const btn = els.messages.querySelector(`.msg-copy-btn[data-msg-idx="${idx}"]`);
    flashCopyBtn(btn, true);
  }
}

function canCopyImages() {
  return window.isSecureContext && !!navigator.clipboard?.write && typeof ClipboardItem !== "undefined";
}

/** 私信标签文字：自己发出显示接收者，收到显示"私信我" */
function privateLabel(msg, isSelf) {
  if (!msg.to?.length) return "";
  if (!isSelf) return "私信我";
  const names = (msg.recipients || []).map((d) => {
    const online = onlineUsers.find((u) => u.id === d.id);
    return d.name || online?.name || "未知设备";
  });
  return `私信 → ${names.join("、") || "未知设备"}`;
}

/**
 * 将服务端投递的消息渲染到聊天区。
 * payload.kind: text | image | file
 */
function appendMessage(msg, isSelf) {
  const logIdx = chatLog.length;
  chatLog.push(msg);

  const wrapper = document.createElement("div");
  const privateTag = privateLabel(msg, isSelf);
  wrapper.className = `msg${isSelf ? " self" : ""}${privateTag ? " private" : ""}`;
  wrapper.dataset.msgIdx = String(logIdx);

  const fromName = msg.from?.name || "未知设备";
  const time = formatTime(msg.timestamp || Math.floor(Date.now() / 1000));

  let body = "";
  const payload = msg.payload || {};
  // 别人发的消息：点发送者名字或"私信我"即可私聊回复
  const replyable = !isSelf && msg.from?.id && msg.from.id !== deviceId;
  const replyAttrs = replyable
    ? ` data-reply-id="${escapeAttr(msg.from.id)}" data-reply-name="${escapeAttr(fromName)}" title="私聊回复 ${escapeAttr(fromName)}"`
    : "";
  const senderHTML = replyable
    ? `<span class="msg-sender reply"${replyAttrs}>${escapeHTML(fromName)}</span>`
    : escapeHTML(fromName);
  const saveInstead = payload.kind === "image" && !canCopyImages();
  const copyLabel = saveInstead ? "保存" : "复制";
  const copyTitle = saveInstead ? "保存图片" : "复制此条";

  if (payload.kind === "text") {
    body = `<div class="msg-bubble">${escapeHTML(payload.content || "")}</div>`;
  } else if (payload.kind === "image") {
    const imgUrl = safeFileURL(payload.fileId);
    body = imgUrl
      ? `<img class="msg-image" src="${escapeAttr(imgUrl)}" alt="图片" loading="lazy"
           data-file-id="${escapeAttr(payload.fileId)}" data-file-name="${escapeAttr(payload.meta?.name || "image")}" />`
      : `<div class="msg-bubble">[图片不可用]</div>`;
  } else if (payload.kind === "file") {
    const name = payload.meta?.name || "文件";
    const size = payload.meta?.size ? formatSize(payload.meta.size) : "";
    const safeName = escapeHTML(name);
    const fileUrl = safeFileURL(payload.fileId);
    body = fileUrl
      ? `
      <div class="msg-bubble file-bubble">
        <a class="file-card file-download" href="${escapeAttr(fileUrl)}" download="${escapeAttr(name)}"
           data-file-id="${escapeAttr(payload.fileId)}" data-file-name="${escapeAttr(name)}">
          <span class="file-icon" aria-hidden="true">📎</span>
          <div class="file-info">
            <div class="file-name">${safeName}</div>
            <small class="file-size">${size}</small>
            <span class="download-label">下载</span>
          </div>
        </a>
      </div>`
      : `<div class="msg-bubble">[文件不可用]</div>`;
  } else {
    body = `<div class="msg-bubble">[不支持的消息类型]</div>`;
  }

  wrapper.innerHTML = `
    <div class="msg-meta">
      <span>${senderHTML} · ${time}${privateTag ? ` · <span class="msg-private-tag${replyable ? " reply" : ""}"${replyAttrs}>${escapeHTML(privateTag)}</span>` : ""}</span>
      <button type="button" class="msg-copy-btn" data-msg-idx="${logIdx}" title="${copyTitle}">${copyLabel}</button>
    </div>
    ${body}
  `;

  // 上传中的占位条目始终留在底部，新消息插在它们之前
  els.messages.insertBefore(wrapper, els.messages.querySelector(".upload-pending"));
  scrollToBottom();
}

function scrollToBottom() {
  els.messages.scrollTop = els.messages.scrollHeight;
}

/** Linux 桌面 fetch+Blob；移动端/其他平台用原生链接，立刻有系统响应 */
function needsBlobDownload() {
  return document.documentElement.dataset.platform === "linux";
}

/** 通过 fetch + Blob 下载（Linux/dwm 下比 <a download> 可靠） */
async function downloadFile(fileId, fileName, triggerEl) {
  const url = safeFileURL(fileId);
  if (!url) return;

  if (triggerEl) {
    triggerEl.classList.add("is-downloading");
    triggerEl.setAttribute("aria-busy", "true");
  }

  try {
    const blob = await fetchFileBlob(fileId);
    if (!blob) {
      alert("下载失败，文件可能已过期");
      return;
    }
    const blobUrl = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = blobUrl;
    a.download = fileName || "download";
    a.rel = "noopener";
    document.body.appendChild(a);
    a.click();
    a.remove();
    setTimeout(() => URL.revokeObjectURL(blobUrl), 1000);
  } catch (err) {
    alert(`下载失败：${err.message}`);
  } finally {
    if (triggerEl) {
      triggerEl.classList.remove("is-downloading");
      triggerEl.removeAttribute("aria-busy");
    }
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
  const params = new URLSearchParams({ name, platform, id: deviceId });
  const protocol = location.protocol === "https:" ? "wss" : "ws";
  ws = new WebSocket(`${protocol}://${location.host}/ws?${params}`);

  ws.onopen = () => {
    setConnected(true);
    reconnectAttempts = 0;
    if (reconnectTimer) {
      clearTimeout(reconnectTimer);
      reconnectTimer = null;
    }
  };

  ws.onclose = () => {
    ws = null;
    setConnected(false);
    if (!isInChat() || !chatName) return;
    // 指数退避 + 抖动：避免服务端短暂抖动时客户端高频重连、反复被踢，放大掉线感
    const backoff = Math.min(1000 * 2 ** reconnectAttempts, 15000);
    const delay = backoff + Math.random() * 500;
    reconnectAttempts++;
    reconnectTimer = setTimeout(async () => {
      if (!isInChat() || !chatName) return;
      // 握手被 401 拒绝时浏览器只给出 1006：查一下是否需要口令（Hub 重启后会话失效）
      const info = await loadConnectionInfo().catch(() => null);
      if (info?.pinRequired && !info.authorized) {
        leaveChat();
        showPinRow(true);
        alert("需要重新输入房间口令");
        return;
      }
      connect(chatName);
    }, delay);
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
      // 保留正在上传的进度卡片（重连时上传仍在继续）
      els.messages.querySelectorAll(".msg:not(.upload-pending)").forEach((el) => el.remove());
      chatLog = [];
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

/** 手机切后台/锁屏时连接可能已经静默断开；回到前台时立即检查并重连，
 *  不用等最长 60s 的 pong 超时才被动发现（表现为"明明显示已连接但发不出去"）。 */
function reconnectIfStale() {
  if (!isInChat() || !chatName) return;
  if (isOpen()) return;
  if (reconnectTimer) {
    clearTimeout(reconnectTimer);
    reconnectTimer = null;
  }
  reconnectAttempts = 0;
  connect(chatName);
}

document.addEventListener("visibilitychange", () => {
  if (document.visibilityState === "visible") reconnectIfStale();
});
window.addEventListener("pageshow", reconnectIfStale);

/** 发送聊天消息；to 为空表示群发。未连接时返回 false */
function sendWS(payload, to) {
  if (!isOpen()) return false;
  const msg = { type: "message", payload };
  if (to?.length) msg.to = to;
  ws.send(JSON.stringify(msg));
  return true;
}

async function sendText() {
  const text = els.messageInput.value.trim();
  if (!text) return;
  // 只有真正发出去才清空输入框，断线时保留草稿
  if (sendWS({ kind: "text", content: text }, currentRecipients())) {
    els.messageInput.value = "";
  }
}

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

/** 等待 WebSocket 重连（上传耗时较长，结束时可能恰好在重连） */
async function waitForOpen(timeoutMs) {
  const deadline = Date.now() + timeoutMs;
  while (!isOpen() && Date.now() < deadline && isInChat()) {
    await sleep(500);
  }
  return isOpen();
}

class UploadError extends Error {
  constructor(message, { retryable = false, status = 0 } = {}) {
    super(message);
    this.retryable = retryable;
    this.status = status;
  }
}

/**
 * XMLHttpRequest 封装：fetch 不支持上传进度。
 * 返回 { status, data }；网络错误抛出可重试的 UploadError。
 */
function xhrRequest(method, url, { body, json, onUploadProgress, signal } = {}) {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    xhr.open(method, url);
    if (json !== undefined) xhr.setRequestHeader("Content-Type", "application/json");
    if (onUploadProgress) xhr.upload.onprogress = (e) => onUploadProgress(e.loaded);

    const onAbort = () => xhr.abort();
    signal?.addEventListener("abort", onAbort, { once: true });
    const done = () => signal?.removeEventListener("abort", onAbort);

    xhr.onload = () => {
      done();
      let data = null;
      try {
        data = JSON.parse(xhr.responseText);
      } catch {
        data = xhr.responseText;
      }
      resolve({ status: xhr.status, data });
    };
    xhr.onerror = () => {
      done();
      reject(new UploadError("网络中断", { retryable: true }));
    };
    xhr.onabort = () => {
      done();
      reject(new DOMException("已取消", "AbortError"));
    };
    xhr.send(json !== undefined ? JSON.stringify(json) : body);
  });
}

function httpError(res, fallback) {
  const detail = typeof res.data === "string" ? res.data.trim().slice(0, 200) : "";
  if (res.status === 401) return new UploadError("需要房间口令，请刷新页面重新进入", { status: 401 });
  // 5xx / 400（分片中途断开）可重试；413、404 等不可重试
  const retryable = res.status >= 500 || res.status === 400 || res.status === 0;
  return new UploadError(`${fallback} (HTTP ${res.status})${detail ? `：${detail}` : ""}`, {
    retryable,
    status: res.status,
  });
}

/** 创建或恢复上传会话，返回 { uploadId, offset, chunkSize, file? } */
async function openUploadSession(file, storeKey, signal) {
  const savedId = storageGet(storeKey);
  if (savedId && FILE_ID_RE.test(savedId)) {
    const res = await xhrRequest("GET", `/api/uploads/${savedId}`, { signal });
    if (res.status === 200 && res.data?.uploadId) return res.data;
    storageRemove(storeKey); // 已过期或被清理，重新开始
  }

  const res = await xhrRequest("POST", "/api/uploads", {
    json: { name: file.name, size: file.size, mime: file.type },
    signal,
  });
  if (res.status !== 201) throw httpError(res, "创建上传失败");
  if (!res.data.file) storageSet(storeKey, res.data.uploadId);
  return res.data;
}

/**
 * 断点续传：按分片 PUT，网络中断后指数退避并向服务端查询 offset 继续。
 * 刷新页面后重新选择同一文件，会从上次的进度继续。
 * onProgress(uploadedBytes)；返回 { fileId, name, size, mime }。
 */
async function uploadResumable(file, onProgress, signal) {
  const storeKey = `${UPLOAD_KEY_PREFIX}${file.name}|${file.size}|${file.lastModified}`;
  let session = await openUploadSession(file, storeKey, signal);
  const { uploadId, chunkSize } = session;
  let offset = session.offset || 0;
  let failures = 0;

  try {
    while (!session.file) {
      onProgress(offset);
      const end = Math.min(offset + chunkSize, file.size);
      try {
        const res = await xhrRequest("PUT", `/api/uploads/${uploadId}?offset=${offset}`, {
          body: file.slice(offset, end),
          onUploadProgress: (loaded) => onProgress(offset + loaded),
          signal,
        });
        if (res.status === 200 || res.status === 409) {
          // 409：服务端进度与本地不一致，以服务端为准
          session = res.data;
          offset = session.offset;
          if (res.status === 200) failures = 0;
          continue;
        }
        throw httpError(res, "上传失败");
      } catch (err) {
        if (err.name === "AbortError" || !err.retryable) throw err;
        failures++;
        if (failures > UPLOAD_MAX_RETRIES) throw new UploadError(`${err.message}，重试 ${UPLOAD_MAX_RETRIES} 次后放弃`);
        onProgress(offset, `网络中断，${Math.min(2 ** (failures - 1), 30)}s 后重试（${failures}/${UPLOAD_MAX_RETRIES}）`);
        await sleep(Math.min(1000 * 2 ** (failures - 1), 30000));
        if (signal.aborted) throw new DOMException("已取消", "AbortError");
        // 断线期间可能已写入部分数据：向服务端确认实际进度
        const state = await xhrRequest("GET", `/api/uploads/${uploadId}`, { signal }).catch((e) => {
          if (e.name === "AbortError") throw e;
          return null;
        });
        if (state?.status === 200) {
          session = state.data;
          offset = session.offset;
        } else if (state && state.status !== 0) {
          throw httpError(state, "上传会话已失效");
        }
      }
    }
  } catch (err) {
    if (err.name === "AbortError") {
      storageRemove(storeKey);
      xhrRequest("DELETE", `/api/uploads/${uploadId}`).catch(() => {});
    } else if (!err.retryable && err.status && err.status !== 401) {
      // 会话失效、超过上限等：丢弃续传记录；多次重试失败或需口令时保留，之后可续传
      storageRemove(storeKey);
    }
    throw err;
  }

  storageRemove(storeKey);
  onProgress(file.size);
  return session.file;
}

/** 聊天区中的"上传中"占位条目 */
function createUploadCard(file, targetName) {
  const el = document.createElement("div");
  el.className = `msg self upload-pending${targetName ? " private" : ""}`;
  el.innerHTML = `
    <div class="msg-meta"><span>上传中${targetName ? ` · <span class="msg-private-tag">私信 → ${escapeHTML(targetName)}</span>` : ""}</span></div>
    <div class="msg-bubble upload-card">
      <div class="upload-name">📎 ${escapeHTML(file.name)}</div>
      <div class="upload-progress"><div class="upload-progress-bar"></div></div>
      <div class="upload-status">
        <span class="upload-text">准备中…</span>
        <button type="button" class="upload-cancel">取消</button>
      </div>
    </div>`;
  els.messages.appendChild(el);
  scrollToBottom();

  const bar = el.querySelector(".upload-progress-bar");
  const text = el.querySelector(".upload-text");
  let lastBytes = 0;
  let lastTime = performance.now();
  let speed = 0;

  return {
    el,
    cancelBtn: el.querySelector(".upload-cancel"),
    update(bytes, note) {
      const pct = file.size ? Math.min(100, (bytes / file.size) * 100) : 100;
      bar.style.width = `${pct.toFixed(1)}%`;
      const now = performance.now();
      if (now - lastTime >= 500) {
        speed = ((bytes - lastBytes) / (now - lastTime)) * 1000;
        lastBytes = bytes;
        lastTime = now;
      }
      text.textContent =
        note || `${pct.toFixed(0)}% · ${formatSize(bytes)} / ${formatSize(file.size)}${speed > 0 ? ` · ${formatSize(speed)}/s` : ""}`;
    },
    remove() {
      el.remove();
    },
  };
}

/**
 * 文件/图片发送流程：分片上传到 Hub（可断点续传），再通过 WebSocket 投递 fileId。
 * 其他设备收到消息后，从 /api/files/{id} 下载。
 */
async function uploadAndSend(file) {
  if (!isInChat()) return; // 已离开聊天室：批量上传的剩余文件直接跳过
  if (!isOpen()) {
    alert("未连接聊天室，无法发送文件。请确认顶部显示「已连接」后再试。");
    return;
  }

  // 目标在上传开始时确定，上传过程中切换目标不影响本文件
  const to = currentRecipients();
  const card = createUploadCard(file, sendTarget?.name);
  const controller = new AbortController();
  card.cancelBtn.addEventListener("click", () => controller.abort(), { once: true });
  activeUploads.add(controller);

  let result;
  try {
    result = await uploadResumable(file, (bytes, note) => card.update(bytes, note), controller.signal);
  } catch (err) {
    if (err.name !== "AbortError") alert(`「${file.name}」上传失败：${err.message}`);
    return;
  } finally {
    activeUploads.delete(controller);
    card.remove();
  }

  const payload = {
    kind: result.mime?.startsWith("image/") ? "image" : "file",
    fileId: result.fileId,
    meta: {
      name: result.name,
      size: result.size,
      mime: result.mime,
    },
  };
  if (!sendWS(payload, to) && !((await waitForOpen(30000)) && sendWS(payload, to))) {
    alert(`「${file.name}」已上传，但聊天室连接中断，未能发送。请在恢复连接后重新发送该文件。`);
  }
}

// --- 连接信息 / 二维码 ---

async function loadConnectionInfo() {
  const resp = await fetch("/api/info", { cache: "no-store" });
  if (!resp.ok) return null;
  return resp.json();
}

function isLANIPv4Host(host) {
  const h = String(host).toLowerCase();
  return /^192\.168\.\d{1,3}\.\d{1,3}$/.test(h) || /^10\.\d{1,3}\.\d{1,3}\.\d{1,3}$/.test(h);
}

/** 加入地址：优先当前页已是局域网 IP，否则用服务端 joinUrl；保留服务端附带的口令 fragment */
function pickJoinURL(info) {
  if (isLANIPv4Host(location.hostname)) {
    const hash = info.joinUrl?.includes("#") ? info.joinUrl.slice(info.joinUrl.indexOf("#")) : "";
    return `${location.origin}/${hash}`;
  }
  return info.joinUrl || location.href;
}

/** 展示加入地址与二维码 */
async function showInfoDialog() {
  const info = await loadConnectionInfo();
  if (!info) {
    alert("无法获取连接信息");
    return;
  }

  els.urlList.innerHTML = "";

  const joinUrl = pickJoinURL(info);
  const ts = Date.now();

  (info.urls || []).forEach((url) => {
    const div = document.createElement("div");
    div.className = "url-item";
    div.innerHTML = `<strong>局域网地址</strong>${escapeHTML(url)}`;
    els.urlList.appendChild(div);
  });

  if (els.qrJoin && joinUrl) {
    els.qrJoin.src = `/api/qrcode?url=${encodeURIComponent(joinUrl)}&t=${ts}`;
    if (els.qrJoinUrl) els.qrJoinUrl.textContent = joinUrl;
  }

  els.infoDialog.showModal();
}

// --- 页面流程 ---

function enterChat(name) {
  chatName = name;
  storageSet(STORAGE_KEY, name);
  document.body.classList.add("in-chat");
  document.documentElement.classList.add("in-chat");
  els.joinScreen.classList.add("hidden");
  els.chatScreen.classList.remove("hidden");
  els.selfLabel.textContent = `当前身份：${name}`;
  setConnected(false);
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
  chatLog = [];
  setSendTarget(null);
  devicesSignature = "";
  activeUploads.forEach((c) => c.abort());
  document.body.classList.remove("in-chat");
  document.documentElement.classList.remove("in-chat");
  document.documentElement.style.removeProperty("--composer-offset");
  document.documentElement.style.removeProperty("--keyboard-offset");
  setDevicesPanel(false);
  els.chatScreen.classList.add("hidden");
  els.joinScreen.classList.remove("hidden");
  els.messages.innerHTML = "";
}

/** 输入框粘贴剪贴板图片时直接上传发送 */
async function handleComposerPaste(e) {
  const cd = e.clipboardData;
  if (!cd) return;

  const imageFiles = [];
  for (const item of cd.items) {
    if (item.kind !== "file" || !item.type.startsWith("image/")) continue;
    const file = item.getAsFile();
    if (file) imageFiles.push(file);
  }
  if (imageFiles.length === 0) return;

  e.preventDefault();
  for (let i = 0; i < imageFiles.length; i++) {
    let file = imageFiles[i];
    if (!file.name) {
      const ext = (file.type.split("/")[1] || "png").replace("jpeg", "jpg");
      file = new File([file], `paste-${Date.now()}-${i}.${ext}`, { type: file.type });
    }
    await uploadAndSend(file);
  }
}

// --- 事件绑定 ---

els.joinBtn.addEventListener("click", async () => {
  const name = els.deviceName.value.trim() || defaultDeviceName();
  if (!els.pinRow.classList.contains("hidden")) {
    els.joinBtn.disabled = true;
    const result = await submitPIN(els.pinInput.value.trim());
    els.joinBtn.disabled = false;
    if (!result.ok) {
      alert(result.message);
      els.pinInput.focus();
      return;
    }
    showPinRow(false);
  }
  enterChat(name);
});

els.pinInput.addEventListener("keydown", (e) => {
  if (e.key === "Enter") els.joinBtn.click();
});

els.deviceList.addEventListener("click", (e) => {
  const li = e.target.closest(".device-item.selectable");
  if (!li) return;
  const same = sendTarget?.id === li.dataset.deviceId;
  setSendTarget(same ? null : { id: li.dataset.deviceId, name: li.dataset.deviceName });
  setDevicesPanel(false);
  if (!same) els.messageInput.focus();
});
els.targetClear.addEventListener("click", () => setSendTarget(null));

for (const btn of [els.showInfoBtn, els.infoPanelBtn]) {
  btn?.addEventListener("click", showInfoDialog);
}
els.closeInfoBtn.addEventListener("click", () => els.infoDialog.close());
els.leaveBtn.addEventListener("click", leaveChat);

els.toggleDevicesBtn?.addEventListener("click", () => {
  setDevicesPanel(!els.sidebar?.classList.contains("open"));
});
els.sidebarBackdrop?.addEventListener("click", () => setDevicesPanel(false));

els.messages.addEventListener("click", (e) => {
  const copyBtn = e.target.closest(".msg-copy-btn");
  if (copyBtn) {
    e.preventDefault();
    copyOneMessage(Number(copyBtn.dataset.msgIdx));
    return;
  }

  const reply = e.target.closest("[data-reply-id]");
  if (reply) {
    setSendTarget({ id: reply.dataset.replyId, name: reply.dataset.replyName });
    els.messageInput.focus();
    return;
  }

  const img = e.target.closest(".msg-image");
  if (img) {
    openImageViewer(img);
    return;
  }

  const link = e.target.closest(".file-download");
  if (!link || link.classList.contains("is-downloading")) return;
  if (!needsBlobDownload()) return; // 移动端：交给原生 <a download>，立即响应
  e.preventDefault();
  downloadFile(link.dataset.fileId, link.dataset.fileName, link);
});

els.copyChatBtn?.addEventListener("click", copyAllChat);

els.sendBtn.addEventListener("click", sendText);
els.messageInput.addEventListener("paste", handleComposerPaste);
els.messageInput.addEventListener("keydown", (e) => {
  // Enter 发送，Shift+Enter 换行
  if (e.key === "Enter" && !e.shiftKey) {
    e.preventDefault();
    sendText();
  }
});

// 📎 按钮触发 file input
els.attachBtn.addEventListener("click", () => els.fileInput.click());
els.fileInput.addEventListener("change", async () => {
  const files = [...(els.fileInput.files || [])];
  els.fileInput.value = ""; // 允许重复选择同一文件
  for (const file of files) {
    await uploadAndSend(file);
  }
});

// --- 拖拽上传 ---

let dragDepth = 0;

function isFileDrag(e) {
  return [...(e.dataTransfer?.types || [])].includes("Files");
}

els.chatMain.addEventListener("dragenter", (e) => {
  if (!isFileDrag(e)) return;
  e.preventDefault();
  dragDepth++; // 子元素间移动会触发成对的 enter/leave，用计数避免闪烁
  els.dropOverlay.classList.remove("hidden");
});
els.chatMain.addEventListener("dragover", (e) => {
  if (!isFileDrag(e)) return;
  e.preventDefault();
  e.dataTransfer.dropEffect = isOpen() ? "copy" : "none";
});
els.chatMain.addEventListener("dragleave", () => {
  if (dragDepth === 0) return;
  dragDepth--;
  if (dragDepth === 0) els.dropOverlay.classList.add("hidden");
});
els.chatMain.addEventListener("drop", async (e) => {
  if (!isFileDrag(e)) return;
  e.preventDefault();
  dragDepth = 0;
  els.dropOverlay.classList.add("hidden");
  for (const file of [...e.dataTransfer.files]) {
    await uploadAndSend(file);
  }
});

// --- 图片预览 ---

function openImageViewer(img) {
  els.imageViewerImg.src = img.src;
  els.imageViewerDownload.dataset.fileId = img.dataset.fileId || "";
  els.imageViewerDownload.dataset.fileName = img.dataset.fileName || "image";
  els.imageViewer.showModal();
}

els.imageViewerClose.addEventListener("click", () => els.imageViewer.close());
els.imageViewerDownload.addEventListener("click", () => {
  const { fileId, fileName } = els.imageViewerDownload.dataset;
  downloadFile(fileId, fileName, els.imageViewerDownload);
});
// 点击图片以外的区域关闭（Esc 由 <dialog> 原生处理）
els.imageViewer.addEventListener("click", (e) => {
  if (e.target === els.imageViewer) els.imageViewer.close();
});
els.imageViewer.addEventListener("close", () => {
  els.imageViewerImg.removeAttribute("src");
});

// --- 房间口令 ---

function showPinRow(show) {
  els.pinRow.classList.toggle("hidden", !show);
  if (show) els.pinInput.value = "";
}

/** 提交口令，成功后服务端下发会话 cookie */
async function submitPIN(pin) {
  if (!pin) return { ok: false, message: "请输入房间口令" };
  try {
    const resp = await fetch("/api/auth", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ pin }),
    });
    if (resp.ok) return { ok: true };
    if (resp.status === 429) return { ok: false, message: "尝试次数过多，请 1 分钟后再试" };
    return { ok: false, message: "口令错误" };
  } catch (err) {
    return { ok: false, message: `无法连接 Hub：${err.message}` };
  }
}

/** 扫码链接里的 #pin=xxx 自动认证，然后从地址栏移除 */
async function consumePINFromHash() {
  const m = location.hash.match(/(?:^#|&)pin=([^&]*)/);
  if (!m) return;
  history.replaceState(null, "", location.pathname + location.search);
  await submitPIN(decodeURIComponent(m[1]));
}

// --- 初始化 ---

initPlatformUI();
initAutoHideScrollbars();

const savedName = storageGet(STORAGE_KEY);
els.deviceName.value = savedName || defaultDeviceName();

(async () => {
  await consumePINFromHash();
  const info = await loadConnectionInfo().catch(() => null);
  const needPIN = !!(info?.pinRequired && !info.authorized);
  showPinRow(needPIN);

  // 带 ?auto=1 时跳过进入页（方便手机扫码后直接进聊天）
  if (location.search.includes("auto=1") && savedName && !needPIN) {
    enterChat(savedName);
  }
})();
