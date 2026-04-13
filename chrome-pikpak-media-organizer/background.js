importScripts("shared.js", "planner.js", "metadata-providers.js");

const Shared = self.PikPakOrganizerShared;
const Planner = self.PikPakOrganizerPlanner;
const MetadataProviders = self.PikPakOrganizerMetadataProviders;

const SETTINGS_KEY = Shared.STORAGE_KEYS.settings;
const PLAN_KEY = Shared.STORAGE_KEYS.plan;
const METADATA_CACHE_KEY = Shared.STORAGE_KEYS.metadataCache;

const TAB_MESSAGE_TYPE = "pikpak-organizer-tab-command";
const PAGE_PROGRESS_TYPE = "pikpak-organizer-page-progress";
const STATE_UPDATED_TYPE = "pikpak-organizer-state-updated";
const PAGE_HOST_PATTERN = /(^|\.)mypikpak\.com$/i;
const APPLY_CHUNK_SIZE = 50;

let settings = Shared.normalizeSettings();
let metadataCache = {};
let runtimeState = createEmptyRuntimeState();
let loadPromise = loadState();
let lastScanProgressLogKey = "";
let lastScanProgressLogAt = 0;

function createEmptyRuntimeState() {
  return {
    scanning: false,
    applying: false,
    lastError: "",
    updatedAt: "",
    pageStatus: null,
    plan: null,
    executionLog: [],
    progress: null
  };
}

async function loadState() {
  const stored = await chrome.storage.local.get([SETTINGS_KEY, PLAN_KEY, METADATA_CACHE_KEY]);
  settings = Shared.normalizeSettings(stored[SETTINGS_KEY]);
  metadataCache = stored[METADATA_CACHE_KEY] && typeof stored[METADATA_CACHE_KEY] === "object"
    ? stored[METADATA_CACHE_KEY]
    : {};
  runtimeState.plan = stored[PLAN_KEY] && typeof stored[PLAN_KEY] === "object" ? stored[PLAN_KEY] : null;
  runtimeState.updatedAt = new Date().toISOString();
}

async function persistSettings() {
  await chrome.storage.local.set({
    [SETTINGS_KEY]: settings
  });
}

async function persistPlan() {
  await chrome.storage.local.set({
    [PLAN_KEY]: runtimeState.plan
  });
}

async function persistMetadataCache() {
  await chrome.storage.local.set({
    [METADATA_CACHE_KEY]: metadataCache
  });
}

async function broadcastStateUpdate() {
  try {
    await chrome.runtime.sendMessage({
      type: STATE_UPDATED_TYPE
    });
  } catch (error) {
    // popup may be closed
  }
}

function setRuntimeState(patch) {
  runtimeState = {
    ...runtimeState,
    ...patch,
    updatedAt: new Date().toISOString()
  };
  void broadcastStateUpdate();
}

function buildViewState() {
  return {
    scanning: runtimeState.scanning,
    applying: runtimeState.applying,
    lastError: runtimeState.lastError,
    updatedAt: runtimeState.updatedAt,
    pageStatus: runtimeState.pageStatus,
    plan: runtimeState.plan,
    executionLog: runtimeState.executionLog,
    progress: runtimeState.progress,
    settings
  };
}

function appendExecutionLog(entry) {
  const logs = (runtimeState.executionLog || []).slice(-199);
  logs.push({
    at: new Date().toISOString(),
    ...entry
  });
  runtimeState.executionLog = logs;
  runtimeState.updatedAt = new Date().toISOString();
  void broadcastStateUpdate();
}

function setProgress(patch) {
  runtimeState.progress = {
    active: true,
    phase: "",
    label: "",
    detail: "",
    current: 0,
    total: 0,
    ...runtimeState.progress,
    ...(patch || {})
  };
  runtimeState.updatedAt = new Date().toISOString();
  void broadcastStateUpdate();
}

function clearProgress() {
  runtimeState.progress = null;
  runtimeState.updatedAt = new Date().toISOString();
  void broadcastStateUpdate();
}

function resetScanProgressLogging() {
  lastScanProgressLogKey = "";
  lastScanProgressLogAt = 0;
}

function handlePageProgress(progress, sender) {
  if (!runtimeState.scanning || !progress || typeof progress !== "object") {
    return;
  }

  const current = Math.max(0, Number(progress.current) || 0);
  const total = Math.max(0, Number(progress.total) || 0);
  setProgress({
    phase: String(progress.phase || "scan"),
    label: String(progress.label || "正在扫描"),
    detail: String(progress.detail || ""),
    current,
    total
  });

  const message = formatScanProgressMessage(progress);
  if (!message) {
    return;
  }

  const progressKey = [
    String(progress.phase || ""),
    String(progress.folderPath || ""),
    String(progress.pageCount || ""),
    String(progress.foldersCompleted || current),
    String(progress.foldersQueued || total),
    String(progress.itemsDiscovered || "")
  ].join("|");
  const now = Date.now();
  if (progressKey === lastScanProgressLogKey && now - lastScanProgressLogAt < 1200) {
    return;
  }

  lastScanProgressLogKey = progressKey;
  lastScanProgressLogAt = now;
  appendExecutionLog({
    type: "scan-progress",
    ok: true,
    path: String(progress.folderPath || ""),
    tabId: sender && sender.tab ? sender.tab.id : undefined,
    message
  });
}

function formatScanProgressMessage(progress) {
  if (typeof progress.message === "string" && progress.message.trim()) {
    return progress.message.trim();
  }

  const segments = [];
  if (progress.folderPath) {
    segments.push(String(progress.folderPath));
  }
  if (Number(progress.foldersCompleted || progress.current) || Number(progress.foldersQueued || progress.total)) {
    segments.push(
      `目录 ${Number(progress.foldersCompleted || progress.current || 0)}/${Number(progress.foldersQueued || progress.total || 0)}`
    );
  }
  if (Number(progress.itemsDiscovered)) {
    segments.push(`累计 ${Number(progress.itemsDiscovered)} 项`);
  }
  return segments.length ? `扫描中：${segments.join("，")}` : "";
}

async function getCurrentTab() {
  const tabs = await chrome.tabs.query({
    active: true,
    currentWindow: true
  });
  return tabs && tabs.length ? tabs[0] : null;
}

function isSupportedPikPakTab(tab) {
  if (!tab || !tab.url) {
    return false;
  }
  try {
    const parsed = new URL(tab.url);
    return PAGE_HOST_PATTERN.test(parsed.hostname);
  } catch (error) {
    return false;
  }
}

async function sendTabCommand(tabId, command, payload) {
  return chrome.tabs.sendMessage(tabId, {
    type: TAB_MESSAGE_TYPE,
    command,
    payload: payload || {}
  });
}

function getTabCommandPayload(response, fallbackMessage) {
  if (!response || typeof response !== "object") {
    throw new Error(fallbackMessage || "页面没有返回有效结果。");
  }
  if (response.ok === false) {
    throw new Error(response.error || fallbackMessage || "页面执行失败。");
  }
  return response.payload;
}

async function refreshPageStatus(tab) {
  if (!isSupportedPikPakTab(tab)) {
    const pageStatus = {
      ready: false,
      canScan: false,
      canApply: false,
      supported: false,
      url: tab && tab.url ? tab.url : "",
      message: "当前标签页不是 PikPak Web 页面。"
    };
    setRuntimeState({
      pageStatus
    });
    return pageStatus;
  }

  try {
    const response = await sendTabCommand(tab.id, "ping");
    const payload = getTabCommandPayload(response, "未拿到 PikPak 会话，请先刷新当前标签页。");
    const pageStatus = {
      supported: true,
      url: tab.url,
      ...(payload || {})
    };
    setRuntimeState({
      pageStatus,
      lastError: ""
    });
    return pageStatus;
  } catch (error) {
    const pageStatus = {
      ready: false,
      canScan: false,
      canApply: false,
      supported: true,
      url: tab.url,
      message: "未拿到 PikPak 会话，请先刷新当前标签页。"
    };
    setRuntimeState({
      pageStatus,
      lastError: ""
    });
    return pageStatus;
  }
}

async function scanCurrentTab(options) {
  await loadPromise;
  const scanOptions = options && typeof options === "object" ? options : {};
  const recursive = typeof scanOptions.recursive === "boolean" ? scanOptions.recursive : settings.recursiveScan;
  const resetExecutionLog = scanOptions.resetExecutionLog !== false;
  const startMessage = scanOptions.startMessage || "开始扫描当前页面。";
  const tab = await getCurrentTab();
  if (!tab) {
    throw new Error("未找到当前标签页。");
  }

  setRuntimeState({
    scanning: true,
    lastError: "",
    executionLog: resetExecutionLog ? [] : runtimeState.executionLog
  });
  resetScanProgressLogging();
  setProgress({
    phase: "scan",
    label: "准备扫描页面",
    detail: "",
    current: 0,
    total: 4
  });
  appendExecutionLog({
    type: "scan",
    ok: true,
    message: "开始扫描当前页面。"
  });

  try {
    const pageStatus = await refreshPageStatus(tab);
    if (!pageStatus.canScan && !pageStatus.ready) {
      throw new Error(pageStatus.message || "PikPak 页面尚未准备好。");
    }
    setProgress({
      phase: "scan",
      label: "页面状态已确认",
      detail: pageStatus.message || "",
      current: 1,
      total: 4
    });
    appendExecutionLog({
      type: "scan-page",
      ok: true,
      message: pageStatus.message || "页面状态已确认。"
    });

    const response = await sendTabCommand(tab.id, "scan", {
      recursive
    });
    const scanResult = getTabCommandPayload(response, "扫描失败。");
    if (!scanResult || !Array.isArray(scanResult.items)) {
      throw new Error("扫描结果为空。");
    }
    setProgress({
      phase: "scan",
      label: "已读取页面列表",
      detail: `原始条目 ${scanResult.items.length} 个`,
      current: 2,
      total: 4
    });
    appendExecutionLog({
      type: "scan-list",
      ok: true,
      message: `扫描模式 ${scanResult.scanMode || "api"}，读取到 ${scanResult.items.length} 个原始条目。`
    });
    for (const warning of Array.isArray(scanResult.warnings) ? scanResult.warnings : []) {
      appendExecutionLog({
        type: "scan-warning",
        ok: true,
        message: warning
      });
    }

    const normalizedScan = Planner.normalizeScanResult(scanResult, settings);
    const localPlan = Planner.buildLocalPlan(normalizedScan, settings);
    appendExecutionLog({
      type: "scan-plan",
      ok: true,
      message: `规范化后共 ${normalizedScan.items.length} 个条目，视频 ${localPlan.videos.length} 个，重复组 ${localPlan.duplicateGroups.length} 个。`
    });
    setProgress({
      phase: "scan",
      label: "正在查询番号元数据",
      detail: localPlan.metadataCodes.length
        ? `待解析 ${localPlan.metadataCodes.length} 个番号`
        : "当前没有需要联网解析的番号",
      current: 0,
      total: Math.max(1, localPlan.metadataCodes.length)
    });
    const metadataMap = await resolveMetadataMap(localPlan.metadataCodes, settings, ({ completed, total, code }) => {
      setProgress({
        phase: "metadata",
        label: "正在查询番号元数据",
        detail: total ? `已完成 ${completed}/${total}${code ? `，当前 ${code}` : ""}` : "没有待解析番号",
        current: completed,
        total: Math.max(1, total)
      });
      if (completed === 1 || completed === total || completed % 10 === 0) {
        appendExecutionLog({
          type: "scan-metadata",
          ok: true,
          message: total ? `元数据解析进度 ${completed}/${total}` : "没有待解析番号"
        });
      }
    });
    const plan = Planner.buildFinalPlan(normalizedScan, localPlan, metadataMap, settings);
    setProgress({
      phase: "scan",
      label: "正在汇总预览结果",
      detail: "",
      current: 4,
      total: 4
    });
    appendExecutionLog({
      type: "scan-finish",
      ok: true,
      message:
        `预览已生成：移动 ${plan.summary.moveCount}，删除文件 ${plan.summary.fileDeleteCount}，` +
        `清理源目录 ${plan.summary.sourceFolderCount || 0}，删除空目录 ${plan.summary.folderDeleteCount}，跳过 ${plan.summary.skipCount}。`
    });

    runtimeState.plan = {
      ...plan,
      scanInfo: {
        totalItems: normalizedScan.items.length,
        totalVideos: localPlan.videos.length,
        rootFolderId: normalizedScan.rootFolderId,
        scanMode: scanResult.scanMode || "api",
        requestedRecursive: recursive,
        warnings: Array.isArray(scanResult.warnings) ? scanResult.warnings : []
      }
    };
    runtimeState.scanning = false;
    runtimeState.lastError = "";
    runtimeState.updatedAt = new Date().toISOString();
    await persistPlan();
    resetScanProgressLogging();
    clearProgress();
    await broadcastStateUpdate();
    return buildViewState();
  } catch (error) {
    setRuntimeState({
      scanning: false,
      lastError: getErrorMessage(error)
    });
    appendExecutionLog({
      type: "error",
      ok: false,
      message: getErrorMessage(error)
    });
    resetScanProgressLogging();
    clearProgress();
    throw error;
  }
}

async function quickOrganizeCurrentTab() {
  await scanCurrentTab({
    recursive: settings.recursiveScan,
    resetExecutionLog: true
  });

  const summary = runtimeState.plan && runtimeState.plan.summary ? runtimeState.plan.summary : null;
  const actionableCount = summary
    ? Number(summary.moveCount || 0) + Number(summary.fileDeleteCount || 0) + Number(summary.folderDeleteCount || 0)
    : 0;
  if (!runtimeState.plan || actionableCount <= 0) {
    appendExecutionLog({
      type: "direct-noop",
      ok: true,
      message: "当前目录没有需要执行的移动或删除。"
    });
    return buildViewState();
  }

  appendExecutionLog({
    type: "direct-ready",
    ok: true,
    message: settings.recursiveScan
      ? "直接执行已使用递归扫描结果，开始整理当前目录及其子目录。"
      : "直接执行将只整理当前目录。"
  });

  return applyCurrentPlan({
    resetExecutionLog: false,
    startMessage: settings.recursiveScan ? "开始直接递归整理当前目录。" : "开始直接整理当前目录。"
  });
}

async function resolveMetadataMap(codes, currentSettings, onProgress) {
  const result = {};
  const queue = Array.isArray(codes) ? codes.slice() : [];
  const concurrency = Math.max(1, Math.min(4, currentSettings.maxMetadataConcurrency || 2));
  let metadataCacheChanged = false;
  let completed = 0;

  if (typeof onProgress === "function") {
    onProgress({
      completed,
      total: queue.length,
      code: ""
    });
  }

  async function worker() {
    while (queue.length) {
      const code = queue.shift();
      if (!code) {
        continue;
      }
      const cached = getCachedMetadata(code, currentSettings);
      if (cached) {
        result[code] = cached;
        completed += 1;
        if (typeof onProgress === "function") {
          onProgress({
            completed,
            total: codes.length,
            code
          });
        }
        continue;
      }

      const resolved = await MetadataProviders.resolveActressMetadata(
        code,
        currentSettings,
        fetchTextWithTimeout
      );
      result[code] = resolved;
      metadataCache[code] = {
        value: resolved,
        updatedAt: new Date().toISOString()
      };
      metadataCacheChanged = true;
      completed += 1;
      if (typeof onProgress === "function") {
        onProgress({
          completed,
          total: codes.length,
          code
        });
      }
    }
  }

  await Promise.all(Array.from({ length: concurrency }, () => worker()));
  if (metadataCacheChanged) {
    await persistMetadataCache();
  }
  return result;
}

function getCachedMetadata(code, currentSettings) {
  const entry = metadataCache[code];
  if (!entry || !entry.value || !entry.updatedAt) {
    return null;
  }
  const ttlMs = currentSettings.metadataCacheTtlHours * 60 * 60 * 1000;
  const updatedAt = Date.parse(entry.updatedAt);
  if (!updatedAt || Date.now() - updatedAt > ttlMs) {
    return null;
  }
  return entry.value;
}

async function fetchTextWithTimeout(url, timeoutMs) {
  const controller = new AbortController();
  const timeoutId = setTimeout(() => controller.abort(), timeoutMs || 20000);
  try {
    const response = await fetch(url, {
      method: "GET",
      cache: "no-store",
      credentials: "omit",
      signal: controller.signal,
      headers: {
        accept: "text/html, text/plain, */*"
      }
    });
    const text = await response.text();
    if (!response.ok) {
      throw new Error("HTTP " + response.status + ": " + text.slice(0, 160));
    }
    return text;
  } finally {
    clearTimeout(timeoutId);
  }
}

async function applyCurrentPlan(options) {
  await loadPromise;
  const applyOptions = options && typeof options === "object" ? options : {};
  const resetExecutionLog = applyOptions.resetExecutionLog !== false;
  const startMessage = applyOptions.startMessage || "开始执行整理计划。";
  if (!runtimeState.plan) {
    throw new Error("当前没有可执行的计划。");
  }
  if (runtimeState.applying) {
    throw new Error("当前已有执行任务。");
  }

  const tab = await getCurrentTab();
  if (!tab) {
    throw new Error("未找到当前标签页。");
  }

  setRuntimeState({
    applying: true,
    lastError: "",
    executionLog: resetExecutionLog ? [] : runtimeState.executionLog
  });
  appendExecutionLog({
    type: "apply",
    ok: true,
    message: "开始执行整理计划。"
  });

  try {
    const pageStatus = await refreshPageStatus(tab);
    if (!pageStatus.canApply) {
      throw new Error(pageStatus.message || "当前页面尚未准备好执行整理。请先刷新页面捕获会话。");
    }

    const targetFolderIds = new Map();
    const groupedMoves = groupMovesByPath(runtimeState.plan.moveActions || []);
    const moveChunkCount = groupedMoves.reduce((count, group) => {
      return count + Shared.chunkArray(group.items.map((item) => item.itemId), APPLY_CHUNK_SIZE).length;
    }, 0);
    const deleteChunks = Shared.chunkArray(
      (runtimeState.plan.deleteActions || []).map((item) => item.itemId),
      APPLY_CHUNK_SIZE
    );
    const folderDeleteActions = sortFolderDeleteActions(runtimeState.plan.folderDeleteActions || []);
    const totalSteps = groupedMoves.length + moveChunkCount + deleteChunks.length + folderDeleteActions.length;
    let completedSteps = 0;
    setProgress({
      phase: "apply",
      label: "准备执行计划",
      detail: `共 ${totalSteps} 个步骤`,
      current: 0,
      total: Math.max(1, totalSteps)
    });
    appendExecutionLog({
      type: "apply-summary",
      ok: true,
      message:
        `待移动 ${runtimeState.plan.summary.moveCount}，待删除文件 ${runtimeState.plan.summary.fileDeleteCount}，` +
        `待清理源目录 ${runtimeState.plan.summary.sourceFolderCount || 0}，待删除空目录 ${runtimeState.plan.summary.folderDeleteCount}。`
    });

    for (const moveGroup of groupedMoves) {
      let targetFolderId = targetFolderIds.get(moveGroup.targetPathKey);
      if (!targetFolderId) {
        const ensureResponse = await sendTabCommand(tab.id, "ensure-folder-path", {
          rootFolderId: runtimeState.plan.rootFolderId,
          segments: moveGroup.targetSegments
        });
        const ensurePayload = getTabCommandPayload(ensureResponse, "创建目标目录失败。");
        targetFolderId = ensurePayload ? ensurePayload.folderId : "";
        if (!targetFolderId) {
          throw new Error("创建目标目录失败: " + moveGroup.targetPathKey);
        }
        targetFolderIds.set(moveGroup.targetPathKey, targetFolderId);
        completedSteps += 1;
        setProgress({
          phase: "apply",
          label: "正在确保目标目录",
          detail: moveGroup.targetPathKey,
          current: completedSteps,
          total: Math.max(1, totalSteps)
        });
        if (Array.isArray(ensurePayload.created) && ensurePayload.created.length) {
          for (const folder of ensurePayload.created) {
            appendExecutionLog({
              type: "create-folder",
              ok: true,
              path: moveGroup.targetPathKey,
              message: `已创建目录 ${folder.name}`
            });
          }
        } else {
          appendExecutionLog({
            type: "ensure-folder",
            ok: true,
            path: moveGroup.targetPathKey,
            message: "目标目录已存在。"
          });
        }
      }

      const idChunks = Shared.chunkArray(
        moveGroup.items.map((item) => item.itemId),
        APPLY_CHUNK_SIZE
      );
      for (const ids of idChunks) {
        getTabCommandPayload(
          await sendTabCommand(tab.id, "batch-move", {
            ids,
            parentId: targetFolderId
          }),
          "移动文件失败。"
        );
        completedSteps += 1;
        setProgress({
          phase: "apply",
          label: "正在移动文件",
          detail: `${moveGroup.targetPathKey}（${ids.length} 个）`,
          current: completedSteps,
          total: Math.max(1, totalSteps)
        });
        appendExecutionLog({
          type: "move",
          ok: true,
          path: moveGroup.targetPathKey,
          ids,
          message: `已移动 ${ids.length} 个条目到 ${moveGroup.targetPathKey}`
        });
      }
    }

    for (const ids of deleteChunks) {
      if (!ids.length) {
        continue;
      }
      getTabCommandPayload(
        await sendTabCommand(tab.id, "batch-delete", {
          ids,
          deleteMode: settings.deleteMode
        }),
        "删除文件失败。"
      );
      completedSteps += 1;
      setProgress({
        phase: "apply",
        label: "正在删除文件",
        detail: `本批次 ${ids.length} 个条目`,
        current: completedSteps,
        total: Math.max(1, totalSteps)
      });
      appendExecutionLog({
        type: "delete",
        ok: true,
        ids,
        message: `已删除 ${ids.length} 个文件`
      });
    }

    for (const action of folderDeleteActions) {
      const inspectPayload = getTabCommandPayload(
        await sendTabCommand(tab.id, "list-folder-children", {
          parentId: action.itemId,
          limit: 1
        }),
        "检查目录状态失败。"
      );
      const remainingItems = inspectPayload && Array.isArray(inspectPayload.items) ? inspectPayload.items : [];
      if (remainingItems.length) {
        completedSteps += 1;
        setProgress({
          phase: "apply",
          label: "正在检查空目录",
          detail: `${action.currentPath} 仍有内容，已跳过`,
          current: completedSteps,
          total: Math.max(1, totalSteps)
        });
        appendExecutionLog({
          type: "skip-folder-delete",
          ok: true,
          path: action.currentPath,
          message: "目录仍有内容，跳过删除。"
        });
        continue;
      }

      getTabCommandPayload(
        await sendTabCommand(tab.id, "batch-delete", {
          ids: [action.itemId],
          deleteMode: settings.deleteMode
        }),
        "删除空目录失败。"
      );
      completedSteps += 1;
      setProgress({
        phase: "apply",
        label: "正在删除空目录",
        detail: action.currentPath,
        current: completedSteps,
        total: Math.max(1, totalSteps)
      });
      appendExecutionLog({
        type: "delete-folder",
        ok: true,
        path: action.currentPath,
        ids: [action.itemId],
        message: `已删除空目录 ${action.currentPath}`
      });
    }

    setRuntimeState({
      applying: false,
      lastError: ""
    });
    appendExecutionLog({
      type: "apply-finish",
      ok: true,
      message: "整理计划执行完成。"
    });
    clearProgress();
    return buildViewState();
  } catch (error) {
    appendExecutionLog({
      type: "error",
      ok: false,
      message: getErrorMessage(error)
    });
    setRuntimeState({
      applying: false,
      lastError: getErrorMessage(error)
    });
    clearProgress();
    throw error;
  }
}

function groupMovesByPath(moveActions) {
  const groups = new Map();
  for (const action of moveActions || []) {
    const key = action.targetPathKey;
    const current = groups.get(key) || {
      targetPathKey: key,
      targetSegments: action.targetSegments || [],
      items: []
    };
    current.items.push(action);
    groups.set(key, current);
  }
  return Array.from(groups.values());
}

function sortFolderDeleteActions(folderDeleteActions) {
  return (folderDeleteActions || [])
    .slice()
    .sort((left, right) => {
      const leftDepth = Number(left.depth) || 0;
      const rightDepth = Number(right.depth) || 0;
      return rightDepth - leftDepth || String(right.currentPath || "").localeCompare(String(left.currentPath || ""));
    });
}

function getErrorMessage(error) {
  if (!error) {
    return "未知错误";
  }
  if (typeof error === "string") {
    return error;
  }
  if (error instanceof Error && error.message) {
    return error.message;
  }
  return String(error);
}

async function saveSettings(nextSettings) {
  settings = Shared.normalizeSettings(nextSettings);
  await persistSettings();
  await broadcastStateUpdate();
  return settings;
}

async function resetSettings() {
  settings = Shared.normalizeSettings();
  await persistSettings();
  await broadcastStateUpdate();
  return settings;
}

chrome.runtime.onInstalled.addListener(() => {
  loadPromise = loadState();
});

chrome.runtime.onStartup.addListener(() => {
  loadPromise = loadState();
});

chrome.runtime.onMessage.addListener((message, sender, sendResponse) => {
  (async () => {
    await loadPromise;
    if (!message || typeof message !== "object") {
      sendResponse({
        ok: false,
        error: "无效消息"
      });
      return;
    }

    switch (message.type) {
      case PAGE_PROGRESS_TYPE:
        handlePageProgress(message.progress, sender);
        sendResponse({
          ok: true
        });
        return;
      case "get-state":
        sendResponse({
          ok: true,
          state: buildViewState()
        });
        return;
      case "refresh-page-status": {
        const tab = await getCurrentTab();
        sendResponse({
          ok: true,
          state: buildViewState(),
          pageStatus: await refreshPageStatus(tab)
        });
        return;
      }
      case "scan-current":
        sendResponse({
          ok: true,
          state: await scanCurrentTab()
        });
        return;
      case "quick-organize-current":
        sendResponse({
          ok: true,
          state: await quickOrganizeCurrentTab()
        });
        return;
      case "apply-plan":
        sendResponse({
          ok: true,
          state: await applyCurrentPlan()
        });
        return;
      case "get-settings":
        sendResponse({
          ok: true,
          settings
        });
        return;
      case "save-settings":
        sendResponse({
          ok: true,
          settings: await saveSettings(message.settings)
        });
        return;
      case "reset-settings":
        sendResponse({
          ok: true,
          settings: await resetSettings()
        });
        return;
      case "clear-plan":
        runtimeState.plan = null;
        runtimeState.executionLog = [];
        runtimeState.lastError = "";
        await persistPlan();
        await broadcastStateUpdate();
        sendResponse({
          ok: true,
          state: buildViewState()
        });
        return;
      default:
        sendResponse({
          ok: false,
          error: "未知消息类型: " + String(message.type)
        });
    }
  })().catch((error) => {
    sendResponse({
      ok: false,
      error: getErrorMessage(error)
    });
  });
  return true;
});
