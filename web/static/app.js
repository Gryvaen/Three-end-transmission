const STORAGE_KEY = "lanroom-device-name";

const platformIcons = {
  android: "🤖",
  windows: "🪟",
  linux: "🐧",
  ios: "📱",
  macos: "🍎",
  unknown: "💻",
};

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
let selfDevice = null;
let reconnectTimer = null;
let preferredURL = null;

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

function renderDevices(users) {
  els.deviceList.innerHTML = "";
  els.onlineCount.textContent = String(users.length);

  users.forEach((user) => {
    const li = document.createElement("li");
    li.className = "device-item";
    li.innerHTML = `
      <span class="device-icon">${platformIcons[user.platform] || platformIcons.unknown}</span>
      <div>
        <div class="device-name">${escapeHTML(user.name)}</div>
        <div class="device-platform">${escapeHTML(user.platform)}</div>
      </div>
    `;
    els.deviceList.appendChild(li);
  });
}

function escapeHTML(str) {
  return String(str)
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;");
}

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
    const src = payload.fileId ? `/api/files/${payload.fileId}` : payload.content;
    body = `<img class="msg-image" src="${src}" alt="图片" />`;
  } else if (payload.kind === "file") {
    const name = payload.meta?.name || "文件";
    const size = payload.meta?.size ? formatSize(payload.meta.size) : "";
    body = `
      <div class="msg-bubble">
        <div class="file-card">
          <span>📎</span>
          <div>
            <div>${escapeHTML(name)}</div>
            <small>${size}</small><br />
            <a href="/api/files/${payload.fileId}" download>下载</a>
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

    if (data.type === "message") {
      const isSelf = selfDevice && data.from?.id === selfDevice.id;
      appendMessage(data, isSelf);
    }
  };
}

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

async function loadConnectionInfo() {
  const resp = await fetch("/api/info");
  if (!resp.ok) return null;
  return resp.json();
}

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

  els.qrImage.src = `/api/qrcode?url=${encodeURIComponent(preferredURL)}&t=${Date.now()}`;

  els.infoDialog.showModal();
}

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

els.joinBtn.addEventListener("click", () => {
  const name = els.deviceName.value.trim() || defaultDeviceName();
  enterChat(name);
});

els.showInfoBtn.addEventListener("click", showInfoDialog);
els.infoPanelBtn.addEventListener("click", showInfoDialog);
els.closeInfoBtn.addEventListener("click", () => els.infoDialog.close());
els.leaveBtn.addEventListener("click", leaveChat);

els.sendBtn.addEventListener("click", sendText);
els.messageInput.addEventListener("keydown", (e) => {
  if (e.key === "Enter" && !e.shiftKey) {
    e.preventDefault();
    sendText();
  }
});

els.attachBtn.addEventListener("click", () => els.fileInput.click());
els.fileInput.addEventListener("change", async () => {
  const file = els.fileInput.files?.[0];
  els.fileInput.value = "";
  if (file) await uploadAndSend(file);
});

const savedName = localStorage.getItem(STORAGE_KEY);
els.deviceName.value = savedName || defaultDeviceName();

if (location.search.includes("auto=1") && savedName) {
  enterChat(savedName);
}
