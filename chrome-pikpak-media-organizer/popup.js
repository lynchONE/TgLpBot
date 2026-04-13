const Shared = globalThis.PikPakOrganizerShared;

const elements = {
  pageStatusLabel: document.getElementById("pageStatusLabel"),
  deleteCount: document.getElementById("deleteCount"),
  moveCount: document.getElementById("moveCount"),
  skipCount: document.getElementById("skipCount"),
  keepCount: document.getElementById("keepCount"),
  recursiveValue: document.getElementById("recursiveValue"),
  thresholdValue: document.getElementById("thresholdValue"),
  actressPolicyValue: document.getElementById("actressPolicyValue"),
  progressValue: document.getElementById("progressValue"),
  progressText: document.getElementById("progressText"),
  progressBar: document.getElementById("progressBar"),
  summaryText: document.getElementById("summaryText"),
  metadataText: document.getElementById("metadataText"),
  breakdownText: document.getElementById("breakdownText"),
  updatedAtLabel: document.getElementById("updatedAtLabel"),
  targetFolderList: document.getElementById("targetFolderList"),
  sourceFolderList: document.getElementById("sourceFolderList"),
  folderDeleteList: document.getElementById("folderDeleteList"),
  actionList: document.getElementById("actionList"),
  executionList: document.getElementById("executionList"),
  refreshStatusBtn: document.getElementById("refreshStatusBtn"),
  reloadTabBtn: document.getElementById("reloadTabBtn"),
  scanBtn: document.getElementById("scanBtn"),
  applyBtn: document.getElementById("applyBtn"),
  quickOrganizeBtn: document.getElementById("quickOrganizeBtn"),
  clearPlanBtn: document.getElementById("clearPlanBtn"),
  openOptionsBtn: document.getElementById("openOptionsBtn")
};

elements.refreshStatusBtn.addEventListener("click", async () => {
  await sendMessage({ type: "refresh-page-status" });
  await refreshView();
});

elements.reloadTabBtn.addEventListener("click", async () => {
  const tab = await getCurrentTab();
  if (tab && typeof tab.id !== "undefined") {
    await chrome.tabs.reload(tab.id);
  }
});

elements.scanBtn.addEventListener("click", async () => {
  elements.pageStatusLabel.textContent = "正在扫描并生成预览...";
  try {
    await sendMessage({ type: "scan-current" });
    await refreshView();
  } catch (error) {
    renderError(error);
  }
});

elements.quickOrganizeBtn.addEventListener("click", async () => {
  const state = await getState();
  const recursiveEnabled = Boolean(state.settings && state.settings.recursiveScan);
  const confirmMessage = recursiveEnabled
    ? "将直接执行当前目录及其子目录的整理，不先停留在预览。是否继续？"
    : "将直接执行当前目录整理，不先停留在预览。是否继续？";
  if (!window.confirm(confirmMessage)) {
    return;
  }

  elements.pageStatusLabel.textContent = recursiveEnabled
    ? "正在直接递归整理当前目录..."
    : "正在直接整理当前目录...";
  try {
    await sendMessage({ type: "quick-organize-current" });
    await refreshView();
  } catch (error) {
    renderError(error);
  }
});

elements.applyBtn.addEventListener("click", async () => {
  const state = await getState();
  if (!state.plan) {
    return;
  }

  const fileDeleteCount = Number(state.plan.summary.fileDeleteCount || 0);
  const folderDeleteCount = Number(state.plan.summary.folderDeleteCount || 0);
  const sourceFolderCount = Number(state.plan.summary.sourceFolderCount || 0);
  const confirmMessage =
    `将执行 ${state.plan.summary.moveCount} 个移动、${fileDeleteCount} 个文件删除、` +
    `${sourceFolderCount} 个源目录清理和 ${folderDeleteCount} 个空目录清理，是否继续？`;
  if (!window.confirm(confirmMessage)) {
    return;
  }

  elements.pageStatusLabel.textContent = "正在执行移动、删除和空目录清理...";
  try {
    await sendMessage({ type: "apply-plan" });
    await refreshView();
  } catch (error) {
    renderError(error);
  }
});

elements.clearPlanBtn.addEventListener("click", async () => {
  await sendMessage({ type: "clear-plan" });
  await refreshView();
});

elements.openOptionsBtn.addEventListener("click", async () => {
  await chrome.runtime.openOptionsPage();
});

chrome.runtime.onMessage.addListener((message) => {
  if (!message || message.type !== "pikpak-organizer-state-updated") {
    return;
  }
  refreshView().catch(() => {});
});

document.addEventListener("DOMContentLoaded", async () => {
  await refreshView();
});

async function refreshView() {
  const state = await getState();
  renderState(state);
}

async function getState() {
  const response = await sendMessage({ type: "get-state" });
  if (!response || !response.ok) {
    throw new Error(response && response.error ? response.error : "读取状态失败");
  }
  return response.state;
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

function renderState(state) {
  const plan = state.plan;
  const currentSettings = state.settings || Shared.normalizeSettings();

  elements.recursiveValue.textContent = currentSettings.recursiveScan ? "开启" : "关闭";
  elements.thresholdValue.textContent = `${currentSettings.minVideoSizeMb} MB`;
  elements.actressPolicyValue.textContent = currentSettings.requireChineseActressName
    ? currentSettings.yearOnlyFallbackEnabled
      ? "女优中文名优先，无则按年份"
      : "无中文名则跳过"
    : "允许非中文名";

  elements.pageStatusLabel.textContent = state.pageStatus
    ? state.pageStatus.message || "PikPak 页面状态未知"
    : "正在识别 PikPak 页面...";

  elements.deleteCount.textContent = String(plan ? plan.summary.deleteCount : 0);
  elements.moveCount.textContent = String(plan ? plan.summary.moveCount : 0);
  elements.skipCount.textContent = String(plan ? plan.summary.skipCount : 0);
  elements.keepCount.textContent = String(plan ? plan.summary.keepCount : 0);
  elements.updatedAtLabel.textContent = state.updatedAt ? formatDateTime(state.updatedAt) : "-";

  const canApply = Boolean(state.pageStatus && state.pageStatus.canApply);
  elements.applyBtn.disabled = !plan || state.scanning || state.applying || !canApply;
  elements.quickOrganizeBtn.disabled = state.scanning || state.applying || !canApply;
  elements.scanBtn.disabled = state.scanning || state.applying;
  elements.clearPlanBtn.disabled = !plan || state.scanning || state.applying;

  renderProgress(state.progress);

  if (!plan) {
    elements.summaryText.textContent = state.lastError || "尚未生成计划。";
    elements.metadataText.textContent = "";
    elements.breakdownText.textContent = "";
    renderTagList(elements.targetFolderList, [], "还没有目标目录。");
    renderTagList(elements.sourceFolderList, [], "还没有源目录清理候选。");
    renderTagList(elements.folderDeleteList, [], "还没有目录删除候选。");
    renderEmpty(elements.actionList, "打开 PikPak 网盘页面，刷新一次，再点击“扫描并生成预览”。");
  } else {
    const summaryParts = [
      `扫描到 ${plan.scanInfo.totalItems} 个条目`,
      `视频 ${plan.scanInfo.totalVideos} 个`,
      `重复组 ${plan.summary.duplicateGroupCount} 个`,
      `源目录清理 ${plan.summary.sourceFolderCount || 0} 个`,
      `空目录清理 ${plan.summary.folderDeleteCount || 0} 个`
    ];
    if (plan.scanInfo.scanMode === "dom") {
      summaryParts.push("当前为 DOM 扫描");
    }
    elements.summaryText.textContent = summaryParts.join("，") + "。";

    const unresolved = plan.metadata.unresolvedCodes || [];
    const metadataParts = Object.entries(plan.metadata.providerHits || {}).map(
      ([providerId, count]) => `${Shared.providerLabel(providerId)} ${count}`
    );
    const warnings = Array.isArray(plan.scanInfo.warnings) ? plan.scanInfo.warnings : [];
    if (warnings.length) {
      metadataParts.unshift(warnings.join(" "));
    }
    if (unresolved.length) {
      metadataParts.push(`未解析番号 ${unresolved.length}`);
    }
    elements.metadataText.textContent = metadataParts.join(" | ");

    elements.breakdownText.textContent =
      `待删除文件 ${plan.summary.fileDeleteCount || 0} 个，其中视频 ${plan.summary.videoDeleteCount || 0} 个、` +
      `其他文件 ${plan.summary.otherFileDeleteCount || 0} 个；待清理源目录 ${plan.summary.sourceFolderCount || 0} 个；` +
      `待删除空目录 ${plan.summary.folderDeleteCount || 0} 个；待确保目录 ${plan.summary.targetFolderCount || 0} 个。`;

    renderTagList(elements.targetFolderList, plan.targetFolders || [], "还没有目标目录。");
    renderTagList(elements.sourceFolderList, plan.sourceCleanupRoots || [], "还没有源目录清理候选。");
    renderTagList(elements.folderDeleteList, plan.folderDeletePaths || [], "还没有目录删除候选。");
    renderActions(plan.actions || []);
  }

  renderExecutionLog(state.executionLog || []);
  if (state.lastError) {
    elements.pageStatusLabel.textContent = state.lastError;
  }
}

function renderProgress(progress) {
  if (!progress || progress.active === false) {
    elements.progressValue.textContent = "空闲";
    elements.progressText.textContent = "等待开始。";
    elements.progressBar.style.width = "0%";
    return;
  }

  const total = Math.max(0, Number(progress.total) || 0);
  const current = Math.max(0, Number(progress.current) || 0);
  const percent = total > 0 ? Math.max(0, Math.min(100, Math.round((current / total) * 100))) : 0;

  elements.progressValue.textContent = total > 0 ? `${current}/${total}` : progress.phase || "运行中";
  elements.progressText.textContent = [progress.label, progress.detail].filter(Boolean).join(" | ") || "运行中";
  elements.progressBar.style.width = `${percent}%`;
}

function renderActions(actions) {
  if (!actions.length) {
    renderEmpty(elements.actionList, "当前计划为空。");
    return;
  }

  const fragment = document.createDocumentFragment();
  for (const action of actions.slice(0, 200)) {
    const node = document.createElement("article");
    node.className = "action-item";
    node.innerHTML = `
      <div class="action-head">
        <span class="action-type type-${action.type}">${actionTypeLabel(action.type)}</span>
        <span class="action-name">${escapeHtml(action.name || "")}</span>
      </div>
      <div class="action-meta">当前位置: ${escapeHtml(action.currentPath || "-")}</div>
      ${action.targetPath ? `<div class="action-meta">目标位置: ${escapeHtml(action.targetPath)}</div>` : ""}
      ${action.code ? `<div class="action-meta">番号: <span class="mono">${escapeHtml(action.code)}</span></div>` : ""}
      ${
        action.actressNameZh
          ? `<div class="action-meta">女优: ${escapeHtml(action.actressNameZh)}${
              action.metadataSource ? ` (${escapeHtml(Shared.providerLabel(action.metadataSource))})` : ""
            }</div>`
          : ""
      }
      ${
        action.reasons && action.reasons.length
          ? `<div class="action-reasons">原因: ${escapeHtml(
              action.reasons.map((reason) => reason.label + (reason.detail ? `(${reason.detail})` : "")).join(" / ")
            )}</div>`
          : ""
      }
    `;
    fragment.appendChild(node);
  }

  if (actions.length > 200) {
    const overflow = document.createElement("div");
    overflow.className = "empty";
    overflow.textContent = `仅展示前 200 条，实际计划共 ${actions.length} 条。`;
    fragment.appendChild(overflow);
  }

  elements.actionList.innerHTML = "";
  elements.actionList.appendChild(fragment);
}

function renderExecutionLog(logs) {
  if (!logs.length) {
    renderEmpty(elements.executionList, "还没有运行记录。");
    return;
  }

  const fragment = document.createDocumentFragment();
  for (const log of logs) {
    const node = document.createElement("article");
    node.className = "execution-item";
    node.innerHTML = `
      <div class="execution-head">
        <span class="execution-type type-${log.ok === false ? "delete" : log.type}">${escapeHtml(log.type || "log")}</span>
        <strong>${escapeHtml(log.path || log.message || "")}</strong>
      </div>
      <div class="execution-time">${escapeHtml(formatLogTime(log.at))}</div>
      <div class="execution-meta">${escapeHtml(buildLogMeta(log))}</div>
    `;
    fragment.appendChild(node);
  }

  elements.executionList.innerHTML = "";
  elements.executionList.appendChild(fragment);
}

function buildLogMeta(log) {
  if (Array.isArray(log.ids) && log.ids.length) {
    return `涉及 ${log.ids.length} 个条目`;
  }
  if (log.folderId) {
    return `目录 ID: ${log.folderId}`;
  }
  return log.message || "";
}

function renderEmpty(container, text) {
  container.innerHTML = "";
  const node = document.createElement("div");
  node.className = "empty";
  node.textContent = text;
  container.appendChild(node);
}

function renderTagList(container, items, emptyText) {
  container.innerHTML = "";
  const values = Array.isArray(items) ? items.filter(Boolean) : [];
  if (!values.length) {
    const emptyNode = document.createElement("div");
    emptyNode.className = "empty";
    emptyNode.textContent = emptyText;
    container.appendChild(emptyNode);
    return;
  }

  const fragment = document.createDocumentFragment();
  for (const value of values.slice(0, 30)) {
    const node = document.createElement("span");
    node.className = "tag";
    node.textContent = value;
    fragment.appendChild(node);
  }
  if (values.length > 30) {
    const overflow = document.createElement("span");
    overflow.className = "tag";
    overflow.textContent = `其余 ${values.length - 30} 个未展开`;
    fragment.appendChild(overflow);
  }
  container.appendChild(fragment);
}

function renderError(error) {
  const message = error instanceof Error ? error.message : String(error || "未知错误");
  elements.pageStatusLabel.textContent = message;
}

function actionTypeLabel(type) {
  switch (type) {
    case "delete":
      return "删除";
    case "move":
      return "移动";
    case "keep":
      return "已就位";
    case "skip":
      return "跳过";
    default:
      return type || "未知";
  }
}

function escapeHtml(value) {
  return String(value || "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#39;");
}

function formatDateTime(value) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return "-";
  }
  return `${date.getMonth() + 1}/${date.getDate()} ${date.getHours().toString().padStart(2, "0")}:${date
    .getMinutes()
    .toString()
    .padStart(2, "0")}`;
}

function formatLogTime(value) {
  if (!value) {
    return "-";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return "-";
  }
  return `${date.getHours().toString().padStart(2, "0")}:${date.getMinutes().toString().padStart(2, "0")}:${date
    .getSeconds()
    .toString()
    .padStart(2, "0")}`;
}
