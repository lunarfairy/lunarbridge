const state = {
  settings: null,
  devices: [],
  selected: null,
  files: [],
  pendingPair: null,
};

const $ = (id) => document.getElementById(id);

function setStatus(text) {
  $("status").textContent = text;
}

function log(text) {
  const box = $("log");
  const time = new Date().toLocaleTimeString();
  box.textContent = `[${time}] ${text}\n` + box.textContent;
}

async function api(path, options = {}) {
  let response;
  try {
    response = await fetch(path, options);
  } catch (error) {
    throw new Error("无法连接本机 LunarBridge 服务。请确认程序还在运行，然后刷新浏览器页面。");
  }
  if (!response.ok) {
    const message = await response.text();
    throw new Error(message.trim() || response.statusText);
  }
  const type = response.headers.get("content-type") || "";
  if (type.includes("application/json")) {
    return response.json();
  }
  return response.text();
}

async function loadSettings() {
  state.settings = await api("/api/settings");
  $("subtitle").textContent = `${state.settings.deviceName} · 本机服务 ${state.settings.uiAddress} · 互传地址 ${state.settings.peerAddress}`;
  $("pairingCode").textContent = state.settings.pairingCode;
  $("deviceName").value = state.settings.deviceName;
  $("receiveDir").value = state.settings.receiveDir;
}

async function loadDevices() {
  state.devices = await api("/api/devices");
  renderDevices();
}

function renderDevices() {
  const list = $("deviceList");
  list.innerHTML = "";
  if (state.devices.length === 0) {
    const empty = document.createElement("div");
    empty.className = "muted";
    empty.textContent = "还没有发现设备。确认两台电脑在同一 Wi‑Fi，或手动输入对方 IP:端口。";
    list.appendChild(empty);
    return;
  }
  for (const device of state.devices) {
    const item = document.createElement("div");
    item.className = `device ${state.selected?.deviceId === device.deviceId ? "selected" : ""}`;
    item.innerHTML = `
      <div class="deviceTop">
        <div class="deviceName"></div>
        <span class="badge">${device.paired ? "已配对" : "未配对"}</span>
      </div>
      <div class="address"></div>
      <div class="deviceActions"></div>
    `;
    item.querySelector(".deviceName").textContent = device.deviceName || device.deviceId;
    item.querySelector(".address").textContent = device.address;
    const actions = item.querySelector(".deviceActions");
    const selectBtn = document.createElement("button");
    selectBtn.textContent = device.paired ? "选择" : "配对";
    selectBtn.onclick = (event) => {
      event.stopPropagation();
      if (device.paired) {
        selectDevice(device);
      } else {
        openPairDialog(device);
      }
    };
    actions.appendChild(selectBtn);
    item.onclick = () => {
      if (device.paired) {
        selectDevice(device);
      } else {
        openPairDialog(device);
      }
    };
    list.appendChild(item);
  }
}

function selectDevice(device) {
  state.selected = device;
  $("selectedDevice").textContent = `${device.deviceName} · ${device.address}`;
  $("sendBtn").disabled = state.files.length === 0;
  renderDevices();
}

function openPairDialog(device) {
  state.pendingPair = device;
  $("pairTarget").textContent = `${device.deviceName} · ${device.address}`;
  $("pairCodeInput").value = "";
  $("pairDialog").showModal();
}

async function pairPendingDevice() {
  const device = state.pendingPair;
  if (!device) return;
  const code = $("pairCodeInput").value.trim();
  if (!code) return;
  setStatus("配对中");
  const paired = await api("/api/pair", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ address: device.address, code }),
  });
  log(`已配对 ${paired.deviceName}`);
  $("pairDialog").close();
  setStatus("待命");
  await loadDevices();
  const found = state.devices.find((d) => d.deviceId === paired.deviceId);
  if (found) selectDevice(found);
}

function setFiles(files) {
  state.files = Array.from(files || []);
  const count = state.files.length;
  const size = state.files.reduce((sum, file) => sum + file.size, 0);
  $("fileSummary").textContent = count === 0
    ? "还没有选择文件。"
    : `${count} 个项目 · ${formatBytes(size)}`;
  $("sendBtn").disabled = count === 0 || !state.selected;
}

function formatBytes(bytes) {
  const units = ["B", "KB", "MB", "GB", "TB"];
  let size = bytes;
  let unit = 0;
  while (size >= 1024 && unit < units.length - 1) {
    size /= 1024;
    unit++;
  }
  return `${size.toFixed(unit === 0 ? 0 : 1)} ${units[unit]}`;
}

async function sendFiles() {
  if (!state.selected || state.files.length === 0) return;
  const form = new FormData();
  form.append("deviceId", state.selected.deviceId);
  form.append("address", state.selected.address);
  for (const file of state.files) {
    form.append("files", file, file.webkitRelativePath || file.name);
  }
  $("progressBar").style.width = "15%";
  setStatus("发送中");
  try {
    const result = await api("/api/send", { method: "POST", body: form });
    $("progressBar").style.width = "100%";
    log(`发送完成：${result.count} 个文件`);
    setStatus("完成");
  } finally {
    setTimeout(() => {
      $("progressBar").style.width = "0%";
      setStatus("待命");
    }, 1000);
  }
}

async function saveSettings() {
  setStatus("保存中");
  await api("/api/settings", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      deviceName: $("deviceName").value.trim(),
      receiveDir: $("receiveDir").value.trim(),
    }),
  });
  await loadSettings();
  setStatus("已保存");
}

async function addManualAddress() {
  const address = $("manualAddress").value.trim();
  if (!address) return;
  await api("/api/manual", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ address }),
  });
  $("manualAddress").value = "";
  await loadDevices();
}

async function refreshAll() {
  try {
    await loadSettings();
    await loadDevices();
    setStatus("待命");
  } catch (error) {
    setStatus("错误");
    log(error.message);
  }
}

$("refreshBtn").onclick = refreshAll;
$("manualBtn").onclick = () => addManualAddress().catch((error) => log(error.message));
$("chooseFilesBtn").onclick = () => $("fileInput").click();
$("chooseFolderBtn").onclick = () => $("folderInput").click();
$("fileInput").onchange = (event) => setFiles(event.target.files);
$("folderInput").onchange = (event) => setFiles(event.target.files);
$("sendBtn").onclick = () => sendFiles().catch((error) => {
  setStatus("发送失败");
  log(error.message);
});
$("saveSettingsBtn").onclick = () => saveSettings().catch((error) => {
  setStatus("保存失败");
  log(error.message);
});
$("pairConfirmBtn").onclick = (event) => {
  event.preventDefault();
  pairPendingDevice().catch((error) => {
    setStatus("配对失败");
    log(error.message);
  });
};

refreshAll();
setInterval(() => {
  loadDevices().catch((error) => {
    setStatus("本机服务断开");
    log(error.message);
  });
}, 3000);
