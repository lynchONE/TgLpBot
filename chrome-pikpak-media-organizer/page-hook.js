(function initPikPakOrganizerPageHook() {
  const CONTENT_SOURCE = "pikpak-organizer-content";
  const PAGE_SOURCE = "pikpak-organizer-page";
  const LIST_LIMIT = 100;
  const FORBIDDEN_FORWARD_HEADERS = new Set([
    "content-length",
    "cookie",
    "host",
    "origin",
    "referer",
    "sec-ch-ua",
    "sec-ch-ua-mobile",
    "sec-ch-ua-platform",
    "sec-fetch-dest",
    "sec-fetch-mode",
    "sec-fetch-site"
  ]);

  const state = {
    driveApiBase: "",
    authHeaders: {},
    currentFolderId: "",
    rootParentMode: "param",
    lastObservedAt: "",
    lastError: ""
  };

  hookFetch();
  hookXhr();
  window.addEventListener("message", handleWindowMessage);

  function handleWindowMessage(event) {
    if (event.source !== window || !event.data || event.data.source !== CONTENT_SOURCE) {
      return;
    }
    const { requestId, command, payload } = event.data;
    void executeCommand(command, payload || {})
      .then((result) => {
        postResponse(requestId, true, result, "");
      })
      .catch((error) => {
        postResponse(requestId, false, null, getErrorMessage(error));
      });
  }

  function postResponse(requestId, ok, payload, error) {
    window.postMessage(
      {
        source: PAGE_SOURCE,
        requestId,
        ok,
        payload,
        error
      },
      "*"
    );
  }

  function postProgress(payload) {
    window.postMessage(
      {
        source: PAGE_SOURCE,
        kind: "progress",
        payload: payload || {}
      },
      "*"
    );
  }

  async function executeCommand(command, payload) {
    switch (command) {
      case "ping":
        return buildPageStatus();
      case "scan":
        return scanCurrentFolder(Boolean(payload.recursive));
      case "ensure-folder-path":
        return ensureFolderPath(payload.rootFolderId || state.currentFolderId || "root", payload.segments || []);
      case "list-folder-children":
        return listFolderChildren(payload.parentId || "", payload.limit);
      case "batch-move":
        return apiFetchJson("/files:batchMove", {
          method: "POST",
          body: {
            ids: Array.isArray(payload.ids) ? payload.ids : [],
            to: {
              parent_id: String(payload.parentId || "")
            }
          }
        });
      case "batch-delete":
        return apiFetchJson("/files:batchDelete", {
          method: "POST",
          body: {
            ids: Array.isArray(payload.ids) ? payload.ids : []
          }
        });
      default:
        throw new Error("未知命令: " + String(command));
    }
  }

  function buildPageStatus() {
    const apiReady = Boolean(state.driveApiBase && (state.currentFolderId || state.currentFolderId === "root"));
    const domState = inspectDomState();
    const canScan = apiReady || domState.ready;
    const canApply = apiReady;
    let message = "";
    if (apiReady) {
      message =
        state.currentFolderId || state.currentFolderId === "root"
          ? "PikPak 页面已就绪。"
          : "尚未识别当前目录，请刷新 PikPak 页面。";
    } else if (domState.ready) {
      message = "已识别当前页面文件列表，可扫描预览；如需执行整理，请先刷新页面捕获会话。";
    } else {
      message = "尚未抓到 PikPak 文件列表，请先打开网盘列表并等待页面加载。";
    }

    return {
      ready: canScan,
      canScan,
      canApply,
      domReady: domState.ready,
      domItemCount: domState.itemCount,
      driveApiBase: state.driveApiBase,
      currentFolderId: state.currentFolderId || "",
      rootParentMode: state.rootParentMode,
      lastObservedAt: state.lastObservedAt,
      pageUrl: location.href,
      message
    };
  }

  function hookFetch() {
    const nativeFetch = window.fetch;
    window.fetch = async function patchedFetch(input, init) {
      const requestInfo = buildRequestInfo(input, init);
      const response = await nativeFetch.apply(this, arguments);
      observeResponse(requestInfo, response.clone()).catch(() => {});
      return response;
    };
  }

  function hookXhr() {
    const nativeOpen = XMLHttpRequest.prototype.open;
    const nativeSend = XMLHttpRequest.prototype.send;
    const nativeSetRequestHeader = XMLHttpRequest.prototype.setRequestHeader;

    XMLHttpRequest.prototype.open = function patchedOpen(method, url) {
      this.__pikpakOrganizer = {
        method: String(method || "GET").toUpperCase(),
        url: String(url || ""),
        headers: {}
      };
      return nativeOpen.apply(this, arguments);
    };

    XMLHttpRequest.prototype.setRequestHeader = function patchedSetRequestHeader(name, value) {
      if (this.__pikpakOrganizer) {
        this.__pikpakOrganizer.headers[String(name || "").toLowerCase()] = String(value || "");
      }
      return nativeSetRequestHeader.apply(this, arguments);
    };

    XMLHttpRequest.prototype.send = function patchedSend() {
      this.addEventListener("loadend", () => {
        if (!this.__pikpakOrganizer) {
          return;
        }
        observeTextPayload(
          this.__pikpakOrganizer.url,
          this.__pikpakOrganizer.method,
          this.__pikpakOrganizer.headers,
          this.responseText
        );
      });
      return nativeSend.apply(this, arguments);
    };
  }

  function buildRequestInfo(input, init) {
    const url = typeof input === "string" ? input : input instanceof Request ? input.url : "";
    const method =
      String((init && init.method) || (input instanceof Request ? input.method : "") || "GET").toUpperCase();
    const headers = {};
    const appendHeaders = (source) => {
      if (!source) {
        return;
      }
      if (source instanceof Headers) {
        source.forEach((value, key) => {
          headers[key.toLowerCase()] = value;
        });
        return;
      }
      if (Array.isArray(source)) {
        for (const pair of source) {
          if (!Array.isArray(pair) || pair.length < 2) {
            continue;
          }
          headers[String(pair[0]).toLowerCase()] = String(pair[1]);
        }
        return;
      }
      for (const [key, value] of Object.entries(source)) {
        headers[String(key).toLowerCase()] = String(value);
      }
    };
    if (input instanceof Request) {
      appendHeaders(input.headers);
    }
    appendHeaders(init && init.headers);
    return {
      url,
      method,
      headers
    };
  }

  async function observeResponse(requestInfo, response) {
    let text = "";
    try {
      text = await response.text();
    } catch (error) {
      return;
    }
    observeTextPayload(requestInfo.url, requestInfo.method, requestInfo.headers, text);
  }

  function observeTextPayload(url, method, headers, text) {
    const parsed = tryParseDriveUrl(url);
    if (!parsed) {
      return;
    }

    state.driveApiBase = parsed.origin + "/drive/v1";
    state.authHeaders = {
      ...state.authHeaders,
      ...sanitizeHeaders(headers)
    };

    if (parsed.pathname !== "/drive/v1/files" || method !== "GET") {
      return;
    }

    state.currentFolderId = parsed.searchParams.get("parent_id") || "root";
    state.rootParentMode =
      state.currentFolderId === "root" && !parsed.searchParams.has("parent_id") ? "omit" : "param";
    state.lastObservedAt = new Date().toISOString();

    try {
      const payload = JSON.parse(text);
      if (!Array.isArray(payload.files)) {
        return;
      }
      state.lastError = "";
    } catch (error) {
      state.lastError = "PikPak 返回了非 JSON 数据。";
    }
  }

  function tryParseDriveUrl(url) {
    try {
      const parsed = new URL(String(url || ""), location.href);
      if (!/api-drive\./i.test(parsed.hostname)) {
        return null;
      }
      if (!parsed.pathname.startsWith("/drive/v1/files")) {
        return null;
      }
      return parsed;
    } catch (error) {
      return null;
    }
  }

  function sanitizeHeaders(headers) {
    const result = {};
    for (const [key, value] of Object.entries(headers || {})) {
      const normalizedKey = String(key || "").trim().toLowerCase();
      const normalizedValue = String(value || "").trim();
      if (!normalizedKey || !normalizedValue || FORBIDDEN_FORWARD_HEADERS.has(normalizedKey)) {
        continue;
      }
      result[normalizedKey] = normalizedValue;
    }
    return result;
  }

  async function scanCurrentFolder(recursive) {
    const domScan = scanCurrentFolderFromDom(recursive);
    const apiReady = Boolean(state.driveApiBase && (state.currentFolderId || state.currentFolderId === "root"));
    if (!recursive && domScan && domScan.items.length) {
      return domScan;
    }
    if (recursive && !apiReady && domScan && domScan.items.length) {
      domScan.warnings = [
        "当前启用了递归扫描，但还没有捕获到可递归遍历的页面会话，所以暂只扫描当前页面已渲染的列表。请刷新页面后再重试，可得到完整递归结果。"
      ];
      return domScan;
    }

    ensureReady();
    const rootFolderId = state.currentFolderId || "root";
    const items = [];
    const visited = new Set([String(rootFolderId)]);
    const stack = [
      {
        parentId: rootFolderId,
        pathNames: [],
        pathIds: []
      }
    ];
    let foldersQueued = 1;
    let foldersCompleted = 0;
    let pageFetchCount = 0;

    postProgress({
      phase: recursive ? "scan-recursive" : "scan-folder",
      label: recursive ? "正在递归扫描目录" : "正在扫描当前目录",
      detail: "已开始读取目录内容",
      current: 0,
      total: 1,
      folderPath: formatScanPath([]),
      foldersCompleted: 0,
      foldersQueued: 1,
      pendingFolders: 1,
      itemsDiscovered: 0,
      message: recursive ? "开始递归扫描当前目录。" : "开始扫描当前目录。"
    });

    async function walk() {
      while (stack.length) {
        const currentFolder = stack.pop();
        const displayPath = formatScanPath(currentFolder.pathNames);
        postProgress({
          phase: recursive ? "scan-recursive" : "scan-folder",
          label: recursive ? "正在递归扫描目录" : "正在扫描当前目录",
          detail: `当前目录 ${displayPath}，待扫 ${stack.length + 1} 个目录`,
          current: foldersCompleted,
          total: Math.max(foldersQueued, foldersCompleted + 1),
          folderPath: displayPath,
          foldersCompleted,
          foldersQueued,
          pendingFolders: stack.length + 1,
          itemsDiscovered: items.length,
          message: `开始扫描目录 ${displayPath}，当前待扫 ${stack.length + 1} 个目录。`
        });

        const children = await listAllChildren(currentFolder.parentId, ({ pageCount, batchCount, folderItemCount, hasMore }) => {
          pageFetchCount += 1;
          const discoveredCount = items.length + folderItemCount;
          postProgress({
            phase: recursive ? "scan-recursive" : "scan-folder",
            label: recursive ? "正在递归扫描目录" : "正在扫描当前目录",
            detail:
              `${displayPath} | 第 ${pageCount} 页，本目录 ${folderItemCount} 项，累计 ${discoveredCount} 项` +
              (hasMore ? "，还有后续分页" : ""),
            current: foldersCompleted,
            total: Math.max(foldersQueued, foldersCompleted + 1),
            folderPath: displayPath,
            foldersCompleted,
            foldersQueued,
            pendingFolders: stack.length + 1,
            itemsDiscovered: discoveredCount,
            pageCount,
            pageFetchCount,
            message:
              `扫描目录 ${displayPath}，第 ${pageCount} 页新增 ${batchCount} 项，` +
              `本目录累计 ${folderItemCount} 项，总计 ${discoveredCount} 项` +
              (hasMore ? "，还有下一页。" : "。")
          });
        });

        for (const child of children) {
          const record = {
            ...child,
            scan_parent_id: currentFolder.parentId,
            scan_parent_path_names: currentFolder.pathNames.slice(),
            scan_parent_path_ids: currentFolder.pathIds.slice()
          };
          items.push(record);
          if (recursive && child.kind === "drive#folder" && !visited.has(String(child.id || ""))) {
            visited.add(String(child.id || ""));
            foldersQueued += 1;
            stack.push({
              parentId: String(child.id),
              pathNames: currentFolder.pathNames.concat(String(child.name || "")),
              pathIds: currentFolder.pathIds.concat(String(child.id))
            });
          }
        }

        foldersCompleted += 1;
        postProgress({
          phase: recursive ? "scan-recursive" : "scan-folder",
          label: recursive ? "正在递归扫描目录" : "正在扫描当前目录",
          detail:
            `已扫 ${foldersCompleted}/${foldersQueued} 个目录，待扫 ${stack.length} 个，累计发现 ${items.length} 项`,
          current: foldersCompleted,
          total: Math.max(foldersQueued, foldersCompleted),
          folderPath: displayPath,
          foldersCompleted,
          foldersQueued,
          pendingFolders: stack.length,
          itemsDiscovered: items.length,
          pageFetchCount,
          message:
            `完成目录 ${displayPath}，已扫 ${foldersCompleted}/${foldersQueued} 个目录，` +
            `待扫 ${stack.length} 个，累计发现 ${items.length} 项。`
        });
      }
    }

    try {
      await walk();
    } catch (error) {
      if (isNotFoundError(error) && String(rootFolderId) !== "root") {
        throw new Error(
          "当前页面目录 ID 已失效，或你所在的不是普通网盘文件夹视图。请先进入要整理的真实文件夹后刷新页面，再重试。原始错误: " +
            getErrorMessage(error)
        );
      }
      throw error;
    }

    return {
      rootFolderId,
      rootParentMode: state.rootParentMode,
      driveApiBase: state.driveApiBase,
      observedAt: state.lastObservedAt,
      scanMode: "api",
      warnings: [],
      items
    };
  }

  function scanCurrentFolderFromDom(recursive) {
    const domItems = extractDriveItemsFromDom();
    if (!domItems.length) {
      return null;
    }

    const rootFolderId = state.currentFolderId || "root";
    return {
      rootFolderId,
      rootParentMode: state.rootParentMode,
      driveApiBase: state.driveApiBase,
      observedAt: new Date().toISOString(),
      scanMode: "dom",
      warnings: recursive
        ? ["当前启用了递归扫描，但 DOM 优先模式暂只扫描当前页面已渲染的列表。"]
        : [],
      items: domItems.map((item) => ({
        ...item,
        scan_parent_id: item.scan_parent_id || rootFolderId,
        scan_parent_path_names: Array.isArray(item.scan_parent_path_names) ? item.scan_parent_path_names : [],
        scan_parent_path_ids: Array.isArray(item.scan_parent_path_ids) ? item.scan_parent_path_ids : []
      }))
    };
  }

  function inspectDomState() {
    const items = extractDriveItemsFromDom(5);
    return {
      ready: items.length > 0,
      itemCount: items.length
    };
  }

  function extractDriveItemsFromDom(limit) {
    const maxItems = Math.max(1, Math.min(5000, Math.round(Number(limit) || 1000)));
    const itemsById = new Map();
    const selector = [
      "[data-id]",
      "[data-key]",
      "[data-row-key]",
      "[role='row']",
      "tr",
      "li",
      "article",
      "[class*='item']",
      "[class*='row']",
      "[class*='file']",
      "[class*='folder']"
    ].join(",");
    const nodes = document.querySelectorAll(selector);
    for (const node of nodes) {
      if (!(node instanceof HTMLElement) || !isElementVisible(node)) {
        continue;
      }
      const attributeRecord = extractItemFromAttributes(node);
      addCandidateItem(itemsById, attributeRecord);
      for (const value of getNodeReactRoots(node)) {
        collectItemsFromUnknown(value, itemsById, maxItems);
        if (itemsById.size >= maxItems) {
          break;
        }
      }
      if (itemsById.size >= maxItems) {
        break;
      }
    }
    return Array.from(itemsById.values()).slice(0, maxItems);
  }

  function addCandidateItem(itemsById, item) {
    if (!item || !item.id || !item.name) {
      return;
    }
    const normalized = {
      id: String(item.id),
      name: String(item.name),
      kind: String(item.kind || ""),
      size: Number(item.size || 0),
      mime_type: String(item.mime_type || item.mimeType || ""),
      created_time: item.created_time || item.createdAt || item.createdTime || "",
      user_modified_time:
        item.user_modified_time || item.addedAt || item.added_time || item.modifiedAt || item.updatedAt || "",
      parent_id: item.parent_id || item.parentId || "",
      hash: item.hash || item.file_hash || item.sha1 || item.md5 || "",
      scan_parent_id: item.scan_parent_id || "",
      scan_parent_path_names: Array.isArray(item.scan_parent_path_names) ? item.scan_parent_path_names : [],
      scan_parent_path_ids: Array.isArray(item.scan_parent_path_ids) ? item.scan_parent_path_ids : []
    };
    const existing = itemsById.get(normalized.id);
    if (!existing || scoreCandidateItem(normalized) > scoreCandidateItem(existing)) {
      itemsById.set(normalized.id, normalized);
    }
  }

  function scoreCandidateItem(item) {
    if (!item) {
      return 0;
    }
    let score = 0;
    if (item.id) score += 10;
    if (item.name) score += 10;
    if (item.kind) score += 4;
    if (item.mime_type) score += 4;
    if (item.size) score += 3;
    if (item.created_time) score += 2;
    if (item.user_modified_time) score += 2;
    if (item.parent_id) score += 1;
    if (item.hash) score += 1;
    return score;
  }

  function extractItemFromAttributes(node) {
    const id =
      node.getAttribute("data-id") ||
      node.getAttribute("data-key") ||
      node.getAttribute("data-row-key") ||
      "";
    const name =
      node.getAttribute("data-name") ||
      node.getAttribute("title") ||
      firstMeaningfulLine(node.innerText || "") ||
      "";
    if (!id || !name) {
      return null;
    }
    return {
      id,
      name,
      kind: maybeFolderFromNode(node, name) ? "drive#folder" : "",
      size: 0
    };
  }

  function getNodeReactRoots(node) {
    const roots = [];
    for (const key of Object.getOwnPropertyNames(node)) {
      if (!/^__(reactProps|reactFiber|reactContainer)\$/.test(key)) {
        continue;
      }
      const value = node[key];
      if (!value) {
        continue;
      }
      roots.push(value);
      if (value.memoizedProps) {
        roots.push(value.memoizedProps);
      }
      if (value.pendingProps) {
        roots.push(value.pendingProps);
      }
      if (value.memoizedState) {
        roots.push(value.memoizedState);
      }
    }
    return roots;
  }

  function collectItemsFromUnknown(rootValue, itemsById, maxItems) {
    const queue = [rootValue];
    const seen = new Set();
    while (queue.length && itemsById.size < maxItems) {
      const current = queue.shift();
      if (!current || typeof current !== "object" || seen.has(current)) {
        continue;
      }
      seen.add(current);

      const normalized = normalizePossibleDriveItem(current);
      if (normalized) {
        addCandidateItem(itemsById, normalized);
      }

      if (Array.isArray(current)) {
        for (const entry of current.slice(0, 100)) {
          if (entry && typeof entry === "object") {
            queue.push(entry);
          }
        }
        continue;
      }

      const entries = Object.entries(current);
      for (const [key, value] of entries.slice(0, 40)) {
        if (!value || typeof value !== "object") {
          continue;
        }
        if (
          key === "stateNode" ||
          key === "return" ||
          key === "alternate" ||
          key === "sibling" ||
          key === "_owner"
        ) {
          continue;
        }
        queue.push(value);
      }
    }
  }

  function normalizePossibleDriveItem(raw) {
    const id = pickString(raw.id, raw.file_id, raw.fileId, raw.itemId, raw.node_id, raw.nodeId);
    const name = pickString(raw.name, raw.file_name, raw.fileName, raw.title, raw.display_name);
    if (!id || !name) {
      return null;
    }
    let kind = pickString(raw.kind, raw.item_kind, raw.file_kind, raw.type);
    if (String(kind).toLowerCase() === "folder") {
      kind = "drive#folder";
    }
    if (!kind && (raw.is_dir === true || raw.is_folder === true)) {
      kind = "drive#folder";
    }
    const size = Number(raw.size || raw.file_size || raw.fileSize || 0);
    const mimeType = pickString(raw.mime_type, raw.mimeType, raw.content_type);
    const createdTime = pickString(raw.created_time, raw.createdAt, raw.createdTime);
    const modifiedTime = pickString(
      raw.user_modified_time,
      raw.updatedAt,
      raw.updated_time,
      raw.addedAt,
      raw.added_time,
      raw.modifiedAt
    );
    const parentId = pickString(raw.parent_id, raw.parentId);
    const hash = pickString(raw.hash, raw.file_hash, raw.sha1, raw.md5);
    const hasDriveSignals = Boolean(kind || size || mimeType || createdTime || modifiedTime || parentId || hash);
    if (!hasDriveSignals) {
      return null;
    }
    return {
      id,
      name,
      kind,
      size,
      mime_type: mimeType,
      created_time: createdTime,
      user_modified_time: modifiedTime,
      parent_id: parentId,
      hash
    };
  }

  function pickString() {
    for (const value of arguments) {
      if (typeof value === "string" && value.trim()) {
        return value.trim();
      }
      if (typeof value === "number" && Number.isFinite(value)) {
        return String(value);
      }
    }
    return "";
  }

  function firstMeaningfulLine(text) {
    return String(text || "")
      .split(/\r?\n/)
      .map((line) => line.trim())
      .find((line) => line && line.length <= 240) || "";
  }

  function formatScanPath(pathNames) {
    const segments = Array.isArray(pathNames)
      ? pathNames.map((segment) => String(segment || "").trim()).filter(Boolean)
      : [];
    return segments.length ? "当前目录 / " + segments.join(" / ") : "当前目录";
  }

  function maybeFolderFromNode(node, name) {
    if (/\.[a-z0-9]{2,5}$/i.test(String(name || ""))) {
      return false;
    }
    const ariaLabel = String(node.getAttribute("aria-label") || "").toLowerCase();
    const className = String(node.className || "").toLowerCase();
    return ariaLabel.includes("folder") || ariaLabel.includes("文件夹") || className.includes("folder");
  }

  function isElementVisible(node) {
    const rect = node.getBoundingClientRect();
    if (rect.width <= 0 || rect.height <= 0) {
      return false;
    }
    const style = window.getComputedStyle(node);
    return style.display !== "none" && style.visibility !== "hidden";
  }

  async function listAllChildren(parentId, onProgress) {
    const files = [];
    let pageToken = "";
    let pageCount = 0;
    let folderItemCount = 0;
    do {
      const payload = await fetchFolderChildren(parentId, {
        limit: LIST_LIMIT,
        pageToken
      });
      const currentFiles = payload.items;
      files.push(...currentFiles);
      folderItemCount += currentFiles.length;
      pageToken = String(payload.nextPageToken || "");
      pageCount += 1;
      if (typeof onProgress === "function") {
        onProgress({
          pageCount,
          batchCount: currentFiles.length,
          folderItemCount,
          hasMore: Boolean(pageToken)
        });
      }
    } while (pageToken && pageCount < 200);
    return files;
  }

  async function listFolderChildren(parentId, limit) {
    ensureReady();
    return fetchFolderChildren(parentId, {
      limit: Math.max(1, Math.min(LIST_LIMIT, Math.round(Number(limit) || 1)))
    });
  }

  async function fetchFolderChildren(parentId, options) {
    ensureReady();
    const normalizedParentId = String(parentId || "");
    const isRoot = normalizedParentId === "root";
    const preferredRootMode = options && options.rootParentMode ? String(options.rootParentMode) : state.rootParentMode;
    try {
      return await fetchFolderChildrenOnce(normalizedParentId, options, isRoot ? preferredRootMode : state.rootParentMode);
    } catch (error) {
      if (!isRoot || !isNotFoundError(error)) {
        throw error;
      }
      const fallbackRootMode = preferredRootMode === "omit" ? "param" : "omit";
      const payload = await fetchFolderChildrenOnce(normalizedParentId, options, fallbackRootMode);
      state.rootParentMode = fallbackRootMode;
      return payload;
    }
  }

  async function fetchFolderChildrenOnce(parentId, options, rootParentMode) {
    ensureReady();
    const query = new URLSearchParams();
    query.set(
      "limit",
      String(Math.max(1, Math.min(LIST_LIMIT, Math.round(Number(options && options.limit) || LIST_LIMIT))))
    );
    query.set("thumbnail_size", "SIZE_SMALL");
    if (options && options.pageToken) {
      query.set("page_token", String(options.pageToken));
    }
    if (!(String(parentId) === "root" && rootParentMode === "omit")) {
      query.set("parent_id", String(parentId));
    }

    const payload = await apiFetchJson("/files?" + query.toString(), {
      method: "GET"
    });
    return {
      items: Array.isArray(payload.files) ? payload.files : [],
      nextPageToken: String(payload.next_page_token || "")
    };
  }

  function isNotFoundError(error) {
    return /HTTP 404\b/.test(getErrorMessage(error));
  }

  async function ensureFolderPath(rootFolderId, segments) {
    ensureReady();
    let parentId = String(rootFolderId || state.currentFolderId || "root");
    const created = [];
    const cleanSegments = Array.isArray(segments)
      ? segments.map((segment) => sanitizeFolderName(segment)).filter(Boolean)
      : [];

    for (const segment of cleanSegments) {
      const children = await listAllChildren(parentId);
      let folder = children.find((item) => item.kind === "drive#folder" && String(item.name || "") === segment);
      if (!folder) {
        const createResult = await apiFetchJson("/files", {
          method: "POST",
          body: {
            kind: "drive#folder",
            parent_id: parentId,
            name: segment
          }
        });
        folder = createResult.file || null;
        if (!folder || !folder.id) {
          throw new Error("创建目录失败: " + segment);
        }
        created.push({
          id: String(folder.id),
          name: segment,
          parentId
        });
      }
      parentId = String(folder.id);
    }

    return {
      folderId: parentId,
      created
    };
  }

  async function apiFetchJson(path, options) {
    ensureReady();
    const requestOptions = options || {};
    const url = path.startsWith("http") ? path : state.driveApiBase + path;
    const headers = {
      accept: "application/json, text/plain, */*",
      ...state.authHeaders,
      ...sanitizeHeaders(requestOptions.headers)
    };

    if (typeof requestOptions.body !== "undefined") {
      headers["content-type"] = "application/json";
    } else {
      delete headers["content-type"];
    }

    const response = await fetch(url, {
      method: requestOptions.method || "GET",
      credentials: "include",
      cache: "no-store",
      headers,
      body: typeof requestOptions.body !== "undefined" ? JSON.stringify(requestOptions.body) : undefined
    });
    const text = await response.text();
    let data = {};
    try {
      data = text ? JSON.parse(text) : {};
    } catch (error) {
      throw new Error("PikPak 返回了无法解析的响应: " + text.slice(0, 200));
    }
    if (!response.ok) {
      throw new Error("PikPak 请求失败: HTTP " + response.status + " " + extractErrorMessage(data));
    }
    return data;
  }

  function ensureReady() {
    if (!state.driveApiBase) {
      throw new Error("未捕获到 PikPak drive 请求，请先刷新页面。");
    }
    if (!(state.currentFolderId || state.currentFolderId === "root")) {
      throw new Error("未识别到当前目录，请先刷新页面。");
    }
  }

  function extractErrorMessage(data) {
    if (!data || typeof data !== "object") {
      return "";
    }
    if (typeof data.error_description === "string") {
      return data.error_description;
    }
    if (typeof data.error === "string") {
      return data.error;
    }
    if (typeof data.message === "string") {
      return data.message;
    }
    return "";
  }

  function sanitizeFolderName(value) {
    return String(value || "")
      .replace(/[\\/:*?"<>|]/g, " ")
      .replace(/\s+/g, " ")
      .trim();
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
})();
