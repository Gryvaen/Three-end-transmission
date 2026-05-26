/**
 * LanRoom 前端：WebSocket 群聊、文件上传、连接信息（二维码）。
 * 协议约定见 README —— presence 广播在线列表，message 广播聊天内容。
 */

const STORAGE_KEY = "lanroom-device-name";

const platformIcons = {
  android: "🤖",
  windows: "🪟",
  linux: "🐧",
  ios: "📱",
  macos: "🍎",
  unknown: "💻",
};

// DOM 引用集中管理，避免重复 querySelector
const els = {
  joinScreen: document.getElementById("join-screen"),
  chatScreen: document.getElementById("chat-screen"),
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
  qrImage: document.getElementById("qr-image"),
};

let ws = null;
let selfDevice = null; // 服务端分配的 device.id，用于区分自己发的消息
let reconnectTimer = null;
let preferredURL = null; // 二维码使用的加入地址

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

function defaultDeviceName() {
  const platform = detectPlatform();
  const labels = {
    android: "Android 设备",
    windows: "Windows PC",
    linux: "Linux 设备",
    ios: "iPhone / iPad",
    macos: "Mac",
    unknown: "我的设备",
  };
  return labels[platform] || labels.unknown;
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

    const li = document.createElement("li");
    li.className = "device-item";
    li.innerHTML = `
      <span class="device-icon">${platformIcons[user.platform] || platformIcons.unknown}</span>
      <div>
        <div class="device-name">${escapeHTML(displayName)}</div>
        <div class="device-ip">${escapeHTML(ip)}</div>
        <div class="device-platform">${escapeHTML(user.platform)}</div>
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
    // 图片先经 /api/upload 存盘，聊天里只传 fileId
    const src = payload.fileId ? `/api/files/${payload.fileId}` : payload.content;
    body = `<img class="msg-image" src="${src}" alt="图片" />`;
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
  if (!fileId) return;
  try {
    const resp = await fetch(`/api/files/${fileId}`);
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
    ws.close();
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
    // 已离开聊天页则不再重连
    if (els.chatScreen.classList.contains("hidden")) return;
    reconnectTimer = setTimeout(() => connect(name), 2000);
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
  const isImage = file.type.startsWith("image/");

  sendWS({
    kind: isImage ? "image" : "file",
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
 * 展示加入地址列表与二维码。
 * 二维码优先使用 joinUrl（局域网 IP），Android 无法解析 .local 域名。
 */
async function showInfoDialog() {
  const info = await loadConnectionInfo();
  if (!info) {
    alert("无法获取连接信息");
    return;
  }

  els.urlList.innerHTML = "";

  const items = [];
  const joinUrl = info.joinUrl || "";

  (info.urls || []).forEach((url) => {
    const isMdns = url.endsWith(".local:" + info.port);
    const isJoin = url === joinUrl;
    let label = "局域网 IP";
    if (isJoin) label = "推荐（二维码 / Android）";
    else if (isMdns) label = "mDNS（部分 Android 不可用）";
    items.push({ label, url });
  });

  preferredURL = joinUrl || items[0]?.url || location.href;

  items.forEach((item) => {
    const div = document.createElement("div");
    div.className = "url-item";
    div.innerHTML = `<strong>${item.label}</strong>${item.url}`;
    els.urlList.appendChild(div);
  });

  // 加时间戳避免浏览器缓存旧二维码
  els.qrImage.src = `/api/qrcode?url=${encodeURIComponent(preferredURL)}&t=${Date.now()}`;

  els.infoDialog.showModal();
}

// --- 页面流程 ---

function enterChat(name) {
  localStorage.setItem(STORAGE_KEY, name);
  els.joinScreen.classList.add("hidden");
  els.chatScreen.classList.remove("hidden");
  els.selfLabel.textContent = `当前身份：${name}`;
  connect(name);
}

function leaveChat() {
  if (ws) ws.close();
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

const savedName = localStorage.getItem(STORAGE_KEY);
els.deviceName.value = savedName || defaultDeviceName();

// 带 ?auto=1 时跳过进入页（方便手机扫码后直接进聊天）
if (location.search.includes("auto=1") && savedName) {
  enterChat(savedName);
}
