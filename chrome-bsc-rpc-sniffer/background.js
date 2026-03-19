const STORAGE_KEY = "bscRpcSnifferState";
const TARGET_CHAIN_ID = 56;
const VALIDATE_TTL_MS = 5 * 60 * 1000;
const REQUEST_TIMEOUT_MS = 10 * 1000;
const MAX_METHODS = 24;
const MAX_SOURCE_PAGES = 12;
const MAX_HEADER_VALUE_LENGTH = 400;
const STATUS_RANK = {
  usable: 0,
  validating: 1,
  pending: 2,
  invalid: 3
};
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
  "sec-fetch-site",
  "user-agent"
]);

const pendingValidations = new Map();
let state = createEmptyState();
let loadStatePromise = loadState();

function createEmptyState() {
  return {
    endpoints: {},
    updatedAt: null
  };
}

function stableStringify(value) {
  if (value === null || typeof value !== "object") {
    return JSON.stringify(value);
  }
  if (Array.isArray(value)) {
    return `[${value.map((item) => stableStringify(item)).join(",")}]`;
  }
  const keys = Object.keys(value).sort();
  return `{${keys
    .map((key) => `${JSON.stringify(key)}:${stableStringify(value[key])}`)
    .join(",")}}`;
}

function normalizeHeaders(rawHeaders) {
  if (!rawHeaders || typeof rawHeaders !== "object") {
    return {};
  }

  const headers = {};
  for (const [key, value] of Object.entries(rawHeaders)) {
    if (typeof value === "undefined" || value === null) {
      continue;
    }
    const headerName = String(key).trim().toLowerCase();
    if (!headerName) {
      continue;
    }
    const headerValue = Array.isArray(value) ? value.join(", ") : String(value).trim();
    if (!headerValue) {
      continue;
    }
    headers[headerName] = headerValue.slice(0, MAX_HEADER_VALUE_LENGTH);
  }
  return headers;
}

function getHeaderFingerprint(headers) {
  const normalized = normalizeHeaders(headers);
  return stableStringify(normalized);
}

function normalizeUrl(rawUrl) {
  try {
    const parsed = new URL(String(rawUrl).trim());
    parsed.hash = "";
    return parsed.toString();
  } catch (error) {
    return "";
  }
}

function normalizePageUrl(rawUrl) {
  if (!rawUrl) {
    return "";
  }
  try {
    return new URL(String(rawUrl).trim()).toString();
  } catch (error) {
    return "";
  }
}

function normalizeTransport(rawTransport, normalizedUrl) {
  const transport = String(rawTransport || "").trim().toLowerCase();
  if (transport === "http" || transport === "ws") {
    return transport;
  }

  try {
    const parsed = new URL(normalizedUrl);
    if (parsed.protocol === "ws:" || parsed.protocol === "wss:") {
      return "ws";
    }
    if (parsed.protocol === "http:" || parsed.protocol === "https:") {
      return "http";
    }
  } catch (error) {
    return "";
  }
  return "";
}

function normalizeMethods(rawMethods) {
  if (!Array.isArray(rawMethods)) {
    return [];
  }

  const seen = new Set();
  const methods = [];
  for (const value of rawMethods) {
    if (typeof value !== "string") {
      continue;
    }
    const method = value.trim();
    if (!method || seen.has(method)) {
      continue;
    }
    seen.add(method);
    methods.push(method);
    if (methods.length >= MAX_METHODS) {
      break;
    }
  }
  return methods;
}

function uniquePush(list, value, maxItems) {
  if (!value || list.includes(value)) {
    return;
  }
  list.push(value);
  if (list.length > maxItems) {
    list.splice(0, list.length - maxItems);
  }
}

function sanitizeObservation(payload, sender) {
  if (!payload || typeof payload !== "object") {
    return null;
  }

  const normalizedUrl = normalizeUrl(payload.url);
  if (!normalizedUrl) {
    return null;
  }

  const transport = normalizeTransport(payload.transport, normalizedUrl);
  if (!transport) {
    return null;
  }

  const methods = normalizeMethods(payload.methods);
  if (!methods.length) {
    return null;
  }

  return {
    url: normalizedUrl,
    transport,
    headers: normalizeHeaders(payload.headers),
    headerFingerprint: getHeaderFingerprint(payload.headers),
    methods,
    pageUrl: normalizePageUrl(payload.pageUrl || sender?.tab?.url || ""),
    capturedAt: typeof payload.capturedAt === "string" ? payload.capturedAt : new Date().toISOString()
  };
}

function buildEndpointKey(transport, url, headerFingerprint) {
  return [transport, url, headerFingerprint || "{}"].join("::");
}

function createEndpointRecord(observation) {
  const key = buildEndpointKey(observation.transport, observation.url, observation.headerFingerprint);
  return {
    key,
    url: observation.url,
    transport: observation.transport,
    headers: observation.headers,
    headerFingerprint: observation.headerFingerprint,
    observedMethods: [...observation.methods],
    sourcePages: observation.pageUrl ? [observation.pageUrl] : [],
    observationCount: 1,
    firstSeenAt: observation.capturedAt,
    lastSeenAt: observation.capturedAt,
    lastObservedMethod: observation.methods[0] || "",
    status: "pending",
    validation: {
      checkedAt: "",
      latencyMs: 0,
      chainId: null,
      rawChainId: "",
      blockNumber: null,
      clientVersion: "",
      error: ""
    },
    updatedAt: observation.capturedAt
  };
}

function upsertEndpoint(observation) {
  const key = buildEndpointKey(observation.transport, observation.url, observation.headerFingerprint);
  let endpoint = state.endpoints[key];

  if (!endpoint) {
    endpoint = createEndpointRecord(observation);
    state.endpoints[key] = endpoint;
    return endpoint;
  }

  endpoint.lastSeenAt = observation.capturedAt;
  endpoint.updatedAt = observation.capturedAt;
  endpoint.observationCount += 1;
  endpoint.lastObservedMethod = observation.methods[0] || endpoint.lastObservedMethod;
  endpoint.headers = observation.headers;
  endpoint.headerFingerprint = observation.headerFingerprint;

  for (const method of observation.methods) {
    uniquePush(endpoint.observedMethods, method, MAX_METHODS);
  }
  if (observation.pageUrl) {
    uniquePush(endpoint.sourcePages, observation.pageUrl, MAX_SOURCE_PAGES);
  }

  return endpoint;
}

function sortEndpoints(endpoints) {
  return endpoints.sort((left, right) => {
    const leftRank = STATUS_RANK[left.status] ?? 9;
    const rightRank = STATUS_RANK[right.status] ?? 9;
    if (leftRank !== rightRank) {
      return leftRank - rightRank;
    }

    const leftTime = Date.parse(left.lastSeenAt || left.firstSeenAt || 0) || 0;
    const rightTime = Date.parse(right.lastSeenAt || right.firstSeenAt || 0) || 0;
    return rightTime - leftTime;
  });
}

function buildUsableExport(endpoints) {
  return endpoints
    .filter((endpoint) => endpoint.status === "usable")
    .map((endpoint) => ({
      url: endpoint.url,
      transport: endpoint.transport,
      headers: endpoint.headers,
      chainId: endpoint.validation.chainId,
      blockNumber: endpoint.validation.blockNumber,
      latencyMs: endpoint.validation.latencyMs,
      clientVersion: endpoint.validation.clientVersion,
      lastCheckedAt: endpoint.validation.checkedAt,
      sourcePages: endpoint.sourcePages,
      observedMethods: endpoint.observedMethods
    }));
}

function buildViewState() {
  const endpoints = sortEndpoints(Object.values(state.endpoints || {}));
  const usable = buildUsableExport(endpoints);
  return {
    updatedAt: state.updatedAt,
    total: endpoints.length,
    usableCount: usable.length,
    endpoints,
    usable,
    usableText: JSON.stringify(usable, null, 2)
  };
}

async function loadState() {
  try {
    const saved = await chrome.storage.local.get(STORAGE_KEY);
    if (saved && saved[STORAGE_KEY] && typeof saved[STORAGE_KEY] === "object") {
      state = saved[STORAGE_KEY];
    } else {
      state = createEmptyState();
      await persistState();
    }
  } catch (error) {
    state = createEmptyState();
  }
}

async function persistState() {
  state.updatedAt = new Date().toISOString();
  await chrome.storage.local.set({
    [STORAGE_KEY]: state
  });
  await broadcastStateUpdate();
}

async function broadcastStateUpdate() {
  try {
    await chrome.runtime.sendMessage({
      type: "state-updated"
    });
  } catch (error) {
    // popup may not be open
  }
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

function formatRpcError(errorPayload) {
  if (!errorPayload || typeof errorPayload !== "object") {
    return "RPC 返回错误";
  }
  const code = typeof errorPayload.code !== "undefined" ? `(${errorPayload.code}) ` : "";
  const message = typeof errorPayload.message === "string" ? errorPayload.message : "RPC 返回错误";
  return `${code}${message}`;
}

function parseChainId(rawValue) {
  if (typeof rawValue === "number" && Number.isFinite(rawValue)) {
    return rawValue;
  }
  if (typeof rawValue !== "string") {
    return null;
  }
  const value = rawValue.trim().toLowerCase();
  if (!value) {
    return null;
  }
  if (value.startsWith("0x")) {
    const parsed = Number.parseInt(value, 16);
    return Number.isFinite(parsed) ? parsed : null;
  }
  const parsed = Number.parseInt(value, 10);
  return Number.isFinite(parsed) ? parsed : null;
}

function parseBlockNumber(rawValue) {
  return parseChainId(rawValue);
}

function buildFetchHeaders(endpointHeaders) {
  const headers = {
    "content-type": "application/json"
  };

  for (const [key, value] of Object.entries(normalizeHeaders(endpointHeaders))) {
    if (FORBIDDEN_FORWARD_HEADERS.has(key) || key === "content-type") {
      continue;
    }
    headers[key] = value;
  }

  return headers;
}

async function rpcCallHttp(url, endpointHeaders, method, params) {
  const controller = new AbortController();
  const timeoutId = setTimeout(() => controller.abort(), REQUEST_TIMEOUT_MS);

  try {
    const response = await fetch(url, {
      method: "POST",
      headers: buildFetchHeaders(endpointHeaders),
      body: JSON.stringify({
        jsonrpc: "2.0",
        id: Date.now(),
        method,
        params: Array.isArray(params) ? params : []
      }),
      cache: "no-store",
      signal: controller.signal
    });

    const text = await response.text();
    if (!response.ok) {
      throw new Error(`HTTP ${response.status}: ${text.slice(0, 180)}`);
    }

    let payload;
    try {
      payload = JSON.parse(text);
    } catch (error) {
      throw new Error("返回不是合法 JSON");
    }

    if (payload && payload.error) {
      throw new Error(formatRpcError(payload.error));
    }
    if (!payload || typeof payload.result === "undefined") {
      throw new Error("RPC 缺少 result 字段");
    }
    return payload.result;
  } finally {
    clearTimeout(timeoutId);
  }
}

async function validateHttpEndpoint(endpoint) {
  const startedAt = Date.now();
  const chainIdRaw = await rpcCallHttp(endpoint.url, endpoint.headers, "eth_chainId", []);
  const chainId = parseChainId(chainIdRaw);
  if (chainId !== TARGET_CHAIN_ID) {
    return {
      ok: false,
      rawChainId: String(chainIdRaw || ""),
      chainId,
      blockNumber: null,
      clientVersion: "",
      latencyMs: Date.now() - startedAt,
      error: `链 ID 不是 BSC 主网: ${String(chainIdRaw || "")}`
    };
  }

  const blockNumberRaw = await rpcCallHttp(endpoint.url, endpoint.headers, "eth_blockNumber", []);
  const blockNumber = parseBlockNumber(blockNumberRaw);
  if (!Number.isFinite(blockNumber) || blockNumber <= 0) {
    return {
      ok: false,
      rawChainId: String(chainIdRaw || ""),
      chainId,
      blockNumber: null,
      clientVersion: "",
      latencyMs: Date.now() - startedAt,
      error: "eth_blockNumber 返回无效"
    };
  }

  let clientVersion = "";
  try {
    const rawClientVersion = await rpcCallHttp(endpoint.url, endpoint.headers, "web3_clientVersion", []);
    clientVersion = typeof rawClientVersion === "string" ? rawClientVersion : String(rawClientVersion || "");
  } catch (error) {
    clientVersion = "";
  }

  return {
    ok: true,
    rawChainId: String(chainIdRaw || ""),
    chainId,
    blockNumber,
    clientVersion,
    latencyMs: Date.now() - startedAt,
    error: ""
  };
}

async function validateWsEndpoint(endpoint) {
  const startedAt = Date.now();

  return new Promise((resolve) => {
    let settled = false;
    let socket = null;
    let timeoutId = null;
    let requestId = 1;
    const pending = new Map();

    function finish(result) {
      if (settled) {
        return;
      }
      settled = true;
      clearTimeout(timeoutId);
      try {
        if (socket && socket.readyState === WebSocket.OPEN) {
          socket.close(1000, "done");
        }
      } catch (error) {
        // ignore socket close failures
      }
      resolve({
        latencyMs: Date.now() - startedAt,
        clientVersion: "",
        blockNumber: null,
        chainId: null,
        rawChainId: "",
        ...result
      });
    }

    function fail(message, extra) {
      finish({
        ok: false,
        error: message,
        ...(extra || {})
      });
    }

    function call(method, params) {
      return new Promise((resolveCall, rejectCall) => {
        const id = requestId++;
        pending.set(id, {
          resolve: resolveCall,
          reject: rejectCall
        });
        socket.send(
          JSON.stringify({
            jsonrpc: "2.0",
            id,
            method,
            params: Array.isArray(params) ? params : []
          })
        );
      });
    }

    try {
      socket = new WebSocket(endpoint.url);
    } catch (error) {
      fail(`WS 初始化失败: ${getErrorMessage(error)}`);
      return;
    }

    timeoutId = setTimeout(() => {
      fail("WS 校验超时");
    }, REQUEST_TIMEOUT_MS);

    socket.addEventListener("message", (event) => {
      let payload;
      try {
        payload = JSON.parse(event.data);
      } catch (error) {
        return;
      }

      const messages = Array.isArray(payload) ? payload : [payload];
      for (const message of messages) {
        if (!message || typeof message.id === "undefined") {
          continue;
        }
        const pendingCall = pending.get(message.id);
        if (!pendingCall) {
          continue;
        }
        pending.delete(message.id);
        if (message.error) {
          pendingCall.reject(new Error(formatRpcError(message.error)));
        } else {
          pendingCall.resolve(message.result);
        }
      }
    });

    socket.addEventListener("error", () => {
      fail("WS 连接失败");
    });

    socket.addEventListener("close", (event) => {
      if (!settled) {
        fail(`WS 连接关闭: ${event.code}`);
      }
    });

    socket.addEventListener("open", async () => {
      try {
        const chainIdRaw = await call("eth_chainId", []);
        const chainId = parseChainId(chainIdRaw);
        if (chainId !== TARGET_CHAIN_ID) {
          fail(`链 ID 不是 BSC 主网: ${String(chainIdRaw || "")}`, {
            rawChainId: String(chainIdRaw || ""),
            chainId
          });
          return;
        }

        const blockNumberRaw = await call("eth_blockNumber", []);
        const blockNumber = parseBlockNumber(blockNumberRaw);
        if (!Number.isFinite(blockNumber) || blockNumber <= 0) {
          fail("eth_blockNumber 返回无效", {
            rawChainId: String(chainIdRaw || ""),
            chainId
          });
          return;
        }

        let clientVersion = "";
        try {
          const rawClientVersion = await call("web3_clientVersion", []);
          clientVersion =
            typeof rawClientVersion === "string" ? rawClientVersion : String(rawClientVersion || "");
        } catch (error) {
          clientVersion = "";
        }

        finish({
          ok: true,
          error: "",
          rawChainId: String(chainIdRaw || ""),
          chainId,
          blockNumber,
          clientVersion
        });
      } catch (error) {
        fail(getErrorMessage(error));
      }
    });
  });
}

async function validateEndpoint(endpoint) {
  if (endpoint.transport === "ws") {
    return validateWsEndpoint(endpoint);
  }
  return validateHttpEndpoint(endpoint);
}

function shouldValidate(endpoint, force) {
  if (!endpoint) {
    return false;
  }
  if (force) {
    return true;
  }
  if (pendingValidations.has(endpoint.key)) {
    return false;
  }
  if (!endpoint.validation || !endpoint.validation.checkedAt) {
    return true;
  }
  const checkedAt = Date.parse(endpoint.validation.checkedAt);
  if (!checkedAt) {
    return true;
  }
  if (Date.now() - checkedAt > VALIDATE_TTL_MS) {
    return true;
  }
  return endpoint.status !== "usable";
}

async function runValidation(endpointKey, force) {
  await loadStatePromise;
  const endpoint = state.endpoints[endpointKey];
  if (!endpoint || !shouldValidate(endpoint, force)) {
    return endpoint;
  }

  if (pendingValidations.has(endpointKey)) {
    return pendingValidations.get(endpointKey);
  }

  endpoint.status = "validating";
  endpoint.updatedAt = new Date().toISOString();
  await persistState();

  const validationPromise = (async () => {
    let result;
    try {
      result = await validateEndpoint(endpoint);
    } catch (error) {
      result = {
        ok: false,
        error: getErrorMessage(error),
        latencyMs: 0,
        chainId: null,
        rawChainId: "",
        blockNumber: null,
        clientVersion: ""
      };
    }

    const current = state.endpoints[endpointKey];
    if (!current) {
      return result;
    }

    current.status = result.ok ? "usable" : "invalid";
    current.validation = {
      checkedAt: new Date().toISOString(),
      latencyMs: result.latencyMs || 0,
      chainId: typeof result.chainId === "number" ? result.chainId : null,
      rawChainId: result.rawChainId || "",
      blockNumber: typeof result.blockNumber === "number" ? result.blockNumber : null,
      clientVersion: result.clientVersion || "",
      error: result.error || ""
    };
    current.updatedAt = new Date().toISOString();
    await persistState();
    return result;
  })().finally(() => {
    pendingValidations.delete(endpointKey);
  });

  pendingValidations.set(endpointKey, validationPromise);
  return validationPromise;
}

async function revalidateAll(force) {
  await loadStatePromise;
  const keys = Object.keys(state.endpoints || {});
  await Promise.all(keys.map((key) => runValidation(key, force)));
  return buildViewState();
}

async function handleObservationMessage(payload, sender) {
  await loadStatePromise;
  const observation = sanitizeObservation(payload, sender);
  if (!observation) {
    return {
      ok: false,
      ignored: true
    };
  }

  const endpoint = upsertEndpoint(observation);
  await persistState();
  void runValidation(endpoint.key, false);

  return {
    ok: true
  };
}

chrome.runtime.onInstalled.addListener(() => {
  loadStatePromise = loadState();
});

chrome.runtime.onStartup.addListener(() => {
  loadStatePromise = loadState();
});

chrome.runtime.onMessage.addListener((message, sender, sendResponse) => {
  (async () => {
    await loadStatePromise;

    if (!message || typeof message !== "object") {
      sendResponse({
        ok: false,
        error: "无效消息"
      });
      return;
    }

    switch (message.type) {
      case "network-observation":
        sendResponse(await handleObservationMessage(message.payload, sender));
        return;
      case "get-state":
        sendResponse({
          ok: true,
          state: buildViewState()
        });
        return;
      case "clear-state":
        state = createEmptyState();
        pendingValidations.clear();
        await persistState();
        sendResponse({
          ok: true,
          state: buildViewState()
        });
        return;
      case "revalidate-all":
        sendResponse({
          ok: true,
          state: await revalidateAll(true)
        });
        return;
      case "revalidate-endpoint":
        if (!message.key) {
          sendResponse({
            ok: false,
            error: "缺少 endpoint key"
          });
          return;
        }
        await runValidation(String(message.key), true);
        sendResponse({
          ok: true,
          state: buildViewState()
        });
        return;
      default:
        sendResponse({
          ok: false,
          error: `未知消息类型: ${String(message.type)}`
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
