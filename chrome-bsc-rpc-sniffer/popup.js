const currentTabLabel = document.getElementById("currentTabLabel");
const totalCount = document.getElementById("totalCount");
const usableCount = document.getElementById("usableCount");
const updatedAt = document.getElementById("updatedAt");
const usableTag = document.getElementById("usableTag");
const listTag = document.getElementById("listTag");
const usableOutput = document.getElementById("usableOutput");
const endpointList = document.getElementById("endpointList");
const endpointTemplate = document.getElementById("endpointTemplate");

const reloadTabBtn = document.getElementById("reloadTabBtn");
const revalidateAllBtn = document.getElementById("revalidateAllBtn");
const copyBtn = document.getElementById("copyBtn");
const exportBtn = document.getElementById("exportBtn");
const clearBtn = document.getElementById("clearBtn");

function formatDateTime(isoString) {
  if (!isoString) {
    return "-";
  }
  const date = new Date(isoString);
  if (Number.isNaN(date.getTime())) {
    return "-";
  }
  return `${date.getHours().toString().padStart(2, "0")}:${date
    .getMinutes()
    .toString()
    .padStart(2, "0")}:${date.getSeconds().toString().padStart(2, "0")}`;
}

function notify(message) {
  currentTabLabel.textContent = message;
}

async function sendMessage(message) {
  return chrome.runtime.sendMessage(message);
}

async function getCurrentTab() {
  const tabs = await chrome.tabs.query({
    active: true,
    currentWindow: true
  });
  return tabs && tabs.length ? tabs[0] : null;
}

async function copyText(text) {
  await navigator.clipboard.writeText(text);
}

function downloadJson(filename, data) {
  const blob = new Blob([JSON.stringify(data, null, 2)], {
    type: "application/json"
  });
  const objectUrl = URL.createObjectURL(blob);
  const anchor = document.createElement("a");
  anchor.href = objectUrl;
  anchor.download = filename;
  anchor.click();
  setTimeout(() => URL.revokeObjectURL(objectUrl), 2000);
}

function escapeHtml(value) {
  return String(value)
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#39;");
}

function renderEndpoint(endpoint) {
  const fragment = endpointTemplate.content.cloneNode(true);
  const card = fragment.querySelector(".endpoint-card");
  const status = fragment.querySelector(".status-pill");
  const endpointUrl = fragment.querySelector(".endpoint-url");
  const endpointMeta = fragment.querySelector(".endpoint-meta");
  const endpointDetails = fragment.querySelector(".endpoint-details");
  const revalidateButton = fragment.querySelector(".revalidate-btn");
  const copyEndpointButton = fragment.querySelector(".copy-endpoint-btn");

  status.textContent =
    endpoint.status === "usable"
      ? "已确认可用"
      : endpoint.status === "validating"
        ? "校验中"
        : endpoint.status === "invalid"
          ? "不可用"
          : "待校验";
  status.classList.add(endpoint.status || "pending");

  endpointUrl.textContent = endpoint.url;

  const metaParts = [
    `协议: ${endpoint.transport.toUpperCase()}`,
    `观察次数: ${endpoint.observationCount}`,
    `最近抓到: ${formatDateTime(endpoint.lastSeenAt)}`
  ];
  if (endpoint.validation && endpoint.validation.latencyMs) {
    metaParts.push(`延迟: ${endpoint.validation.latencyMs} ms`);
  }
  endpointMeta.textContent = metaParts.join(" | ");

  const details = [];
  if (endpoint.validation && endpoint.validation.chainId !== null) {
    details.push(`chainId=${endpoint.validation.chainId}`);
  }
  if (endpoint.validation && endpoint.validation.blockNumber !== null) {
    details.push(`block=${endpoint.validation.blockNumber}`);
  }
  if (endpoint.observedMethods && endpoint.observedMethods.length) {
    details.push(`methods=${endpoint.observedMethods.join(", ")}`);
  }
  if (endpoint.sourcePages && endpoint.sourcePages.length) {
    details.push(`source=${endpoint.sourcePages[endpoint.sourcePages.length - 1]}`);
  }
  if (endpoint.headers && Object.keys(endpoint.headers).length) {
    details.push(`headers=${JSON.stringify(endpoint.headers)}`);
  }
  if (endpoint.validation && endpoint.validation.error) {
    details.push(`error=${endpoint.validation.error}`);
  }
  if (endpoint.validation && endpoint.validation.clientVersion) {
    details.push(`client=${endpoint.validation.clientVersion}`);
  }
  endpointDetails.innerHTML = details.map((item) => `<div><code>${escapeHtml(item)}</code></div>`).join("");

  revalidateButton.addEventListener("click", async () => {
    notify("正在重新校验单个端点…");
    await sendMessage({
      type: "revalidate-endpoint",
      key: endpoint.key
    });
    await refreshState();
  });

  copyEndpointButton.addEventListener("click", async () => {
    await copyText(endpoint.url);
    notify("已复制 URL");
  });

  return card;
}

function renderState(viewState) {
  totalCount.textContent = String(viewState.total || 0);
  usableCount.textContent = String(viewState.usableCount || 0);
  updatedAt.textContent = formatDateTime(viewState.updatedAt);
  usableTag.textContent = `${viewState.usableCount || 0} 个`;
  listTag.textContent = `${viewState.total || 0} 条`;
  usableOutput.value = viewState.usableText || "[]";

  endpointList.innerHTML = "";
  if (!viewState.endpoints || !viewState.endpoints.length) {
    endpointList.innerHTML = '<div class="empty-state">打开目标网页并触发链上请求后，这里会出现捕获结果。</div>';
    return;
  }

  const fragment = document.createDocumentFragment();
  for (const endpoint of viewState.endpoints) {
    fragment.appendChild(renderEndpoint(endpoint));
  }
  endpointList.appendChild(fragment);
}

async function refreshState() {
  const response = await sendMessage({
    type: "get-state"
  });
  if (!response || !response.ok) {
    throw new Error(response && response.error ? response.error : "读取状态失败");
  }
  renderState(response.state);
}

reloadTabBtn.addEventListener("click", async () => {
  const tab = await getCurrentTab();
  if (!tab || typeof tab.id === "undefined") {
    notify("没有可重载的标签页");
    return;
  }
  await chrome.tabs.reload(tab.id);
  notify("当前页已重载，等待抓包结果…");
});

revalidateAllBtn.addEventListener("click", async () => {
  notify("正在重新校验全部端点…");
  await sendMessage({
    type: "revalidate-all"
  });
  await refreshState();
});

copyBtn.addEventListener("click", async () => {
  await copyText(usableOutput.value || "[]");
  notify("确认可用结果已复制");
});

exportBtn.addEventListener("click", async () => {
  const response = await sendMessage({
    type: "get-state"
  });
  if (!response || !response.ok) {
    notify("导出失败");
    return;
  }
  downloadJson("bsc-rpc-usable.json", response.state.usable || []);
  notify("已导出 JSON");
});

clearBtn.addEventListener("click", async () => {
  await sendMessage({
    type: "clear-state"
  });
  await refreshState();
  notify("抓包记录已清空");
});

chrome.runtime.onMessage.addListener((message) => {
  if (!message || message.type !== "state-updated") {
    return;
  }
  refreshState().catch(() => {});
});

document.addEventListener("DOMContentLoaded", async () => {
  try {
    const tab = await getCurrentTab();
    if (tab && tab.url) {
      const parsed = new URL(tab.url);
      notify(`当前页: ${parsed.host}`);
    } else {
      notify("当前页不可读");
    }
  } catch (error) {
    notify("当前页不可读");
  }

  try {
    await refreshState();
  } catch (error) {
    notify(`读取状态失败: ${error.message || error}`);
  }
});
