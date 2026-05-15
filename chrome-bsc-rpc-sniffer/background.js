const STORAGE_KEY = "multiChainRpcSnifferState";
const LEGACY_STORAGE_KEY = "bscRpcSnifferState";
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
const SUPPORTED_CHAINS = [
  {
    key: "bsc",
    name: "BSC",
    rpcFamily: "evm",
    chainId: 56
  },
  {
    key: "base",
    name: "Base",
    rpcFamily: "evm",
    chainId: 8453
  },
  {
    key: "eth",
    name: "Ethereum",
    rpcFamily: "evm",
    chainId: 1
  },
  {
    key: "solana",
    name: "Solana",
    rpcFamily: "solana"
  }
];
const EVM_CHAINS_BY_ID = new Map(
  SUPPORTED_CHAINS.filter((chain) => chain.rpcFamily === "evm").map((chain) => [chain.chainId, chain])
);
const EVM_METHOD_PATTERN = /^(eth|net|web3|debug|trace|txpool|parity|bor|engine)_/i;
const SOLANA_METHODS = new Set([
  "getAccountInfo",
  "getBalance",
  "getBlock",
  "getBlockCommitment",
  "getBlockHeight",
  "getBlockProduction",
  "getBlocks",
  "getBlocksWithLimit",
  "getClusterNodes",
  "getEpochInfo",
  "getEpochSchedule",
  "getFeeForMessage",
  "getFirstAvailableBlock",
  "getGenesisHash",
  "getHealth",
  "getHighestSnapshotSlot",
  "getIdentity",
  "getInflationGovernor",
  "getInflationRate",
  "getInflationReward",
  "getLargestAccounts",
  "getLatestBlockhash",
  "getLeaderSchedule",
  "getMaxRetransmitSlot",
  "getMaxShredInsertSlot",
  "getMinimumBalanceForRentExemption",
  "getMultipleAccounts",
  "getProgramAccounts",
  "getRecentPerformanceSamples",
  "getRecentPrioritizationFees",
  "getSignaturesForAddress",
  "getSignatureStatuses",
  "getSlot",
  "getSlotLeader",
  "getSlotLeaders",
  "getStakeActivation",
  "getSupply",
  "getTokenAccountBalance",
  "getTokenAccountsByDelegate",
  "getTokenAccountsByOwner",
  "getTokenLargestAccounts",
  "getTokenSupply",
  "getTransaction",
  "getTransactionCount",
  "getVersion",
  "isBlockhashValid",
  "minimumLedgerSlot",
  "requestAirdrop",
  "sendTransaction",
  "simulateTransaction"
]);
const SOLANA_WS_METHODS = new Set([
  "accountSubscribe",
  "blockSubscribe",
  "logsSubscribe",
  "programSubscribe",
  "rootSubscribe",
  "signatureSubscribe",
  "slotSubscribe",
  "slotsUpdatesSubscribe",
  "voteSubscribe"
]);
const CREDENTIAL_HEADER_NAMES = new Set([
  "authorization",
  "x-api-key",
  "api-key",
  "x-api-token",
  "x-auth-token",
  "x-project-id",
  "x-client-id",
  "x-access-token",
  "apikey"
]);
const CREDENTIAL_QUERY_NAMES = new Set([
  "apikey",
  "api_key",
  "api-key",
  "apiKey",
  "key",
  "token",
  "access_token",
  "auth",
  "projectid",
  "project_id",
  "projectId",
  "clientid",
  "client_id",
  "dkey"
]);
const PROVIDER_PATH_TOKEN_HOSTS = [
  "alchemy.com",
  "infura.io",
  "quiknode.pro",
  "getblock.io",
  "ankr.com",
  "blastapi.io",
  "p2pify.com",
  "moralis-nodes.com",
  "tenderly.co",
  "helius-rpc.com",
  "drpc.org",
  "nodereal.io"
];

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

function classifyMethodFamily(method) {
  if (EVM_METHOD_PATTERN.test(method)) {
    return "evm";
  }
  if (SOLANA_METHODS.has(method) || SOLANA_WS_METHODS.has(method)) {
    return "solana";
  }
  return "";
}

function inferRpcFamilies(methods) {
  const families = [];
  const seen = new Set();
  for (const method of methods) {
    const family = classifyMethodFamily(method);
    if (!family || seen.has(family)) {
      continue;
    }
    seen.add(family);
    families.push(family);
  }
  return families;
}

function detectCredential(url, headers) {
  const normalizedHeaders = normalizeHeaders(headers);
  for (const [key, value] of Object.entries(normalizedHeaders)) {
    if (!value) {
      continue;
    }
    if (CREDENTIAL_HEADER_NAMES.has(key) || key.startsWith("x-api-")) {
      return {
        hasCredential: true,
        credentialKind: `header:${key}`
      };
    }
  }

  try {
    const parsed = new URL(url);
    for (const [key, value] of parsed.searchParams.entries()) {
      const normalizedKey = key.trim();
      if (!value || !normalizedKey) {
        continue;
      }
      if (CREDENTIAL_QUERY_NAMES.has(normalizedKey) || CREDENTIAL_QUERY_NAMES.has(normalizedKey.toLowerCase())) {
        return {
          hasCredential: true,
          credentialKind: `query:${normalizedKey}`
        };
      }
    }

    const host = parsed.hostname.toLowerCase();
    const hasProviderPathToken = PROVIDER_PATH_TOKEN_HOSTS.some((providerHost) => host.endsWith(providerHost));
    const pathParts = parsed.pathname.split("/").filter(Boolean);
    if (hasProviderPathToken && pathParts.some((part) => /^[A-Za-z0-9_-]{16,}$/.test(part))) {
      return {
        hasCredential: true,
        credentialKind: "path:provider-token"
      };
    }
  } catch (error) {
    // URL was already normalized before this point.
  }

  return {
    hasCredential: false,
    credentialKind: ""
  };
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
    rpcFamilies: inferRpcFamilies(methods),
    pageUrl: normalizePageUrl(payload.pageUrl || sender?.tab?.url || ""),
    capturedAt: typeof payload.capturedAt === "string" ? payload.capturedAt : new Date().toISOString()
  };
}

function buildEndpointKey(transport, url, headerFingerprint) {
  return [transport, url, headerFingerprint || "{}"].join("::");
}

function createEndpointRecord(observation) {
  const key = buildEndpointKey(observation.transport, observation.url, observation.headerFingerprint);
  const credential = detectCredential(observation.url, observation.headers);
  return {
    key,
    url: observation.url,
    transport: observation.transport,
    headers: observation.headers,
    headerFingerprint: observation.headerFingerprint,
    observedMethods: [...observation.methods],
    rpcFamilies: [...observation.rpcFamilies],
    sourcePages: observation.pageUrl ? [observation.pageUrl] : [],
    observationCount: 1,
    firstSeenAt: observation.capturedAt,
    lastSeenAt: observation.capturedAt,
    lastObservedMethod: observation.methods[0] || "",
    chain: "",
    chainName: "",
    rpcFamily: "",
    hasCredential: credential.hasCredential,
    credentialKind: credential.credentialKind,
    status: "pending",
    validation: {
      checkedAt: "",
      latencyMs: 0,
      chainId: null,
      rawChainId: "",
      blockNumber: null,
      slot: null,
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
  const credential = detectCredential(observation.url, observation.headers);
  endpoint.hasCredential = credential.hasCredential;
  endpoint.credentialKind = credential.credentialKind;
  if (!Array.isArray(endpoint.rpcFamilies)) {
    endpoint.rpcFamilies = [];
  }

  for (const method of observation.methods) {
    uniquePush(endpoint.observedMethods, method, MAX_METHODS);
  }
  for (const family of observation.rpcFamilies) {
    uniquePush(endpoint.rpcFamilies, family, MAX_METHODS);
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

function isExportableEndpoint(endpoint) {
  return endpoint.status === "usable" && endpoint.hasCredential === true;
}

function buildUsableExport(endpoints) {
  return endpoints
    .filter(isExportableEndpoint)
    .map((endpoint) => {
      const item = {
        chain: endpoint.chain,
        chainName: endpoint.chainName,
        rpcFamily: endpoint.rpcFamily,
        url: endpoint.url,
        transport: endpoint.transport,
        headers: endpoint.headers,
        credentialKind: endpoint.credentialKind,
        latencyMs: endpoint.validation.latencyMs,
        clientVersion: endpoint.validation.clientVersion,
        lastCheckedAt: endpoint.validation.checkedAt,
        sourcePages: endpoint.sourcePages,
        observedMethods: endpoint.observedMethods
      };
      if (endpoint.rpcFamily === "solana") {
        item.slot = endpoint.validation.slot;
      } else {
        item.chainId = endpoint.validation.chainId;
        item.blockNumber = endpoint.validation.blockNumber;
      }
      return item;
    });
}

function buildViewState() {
  const endpoints = sortEndpoints(Object.values(state.endpoints || {}));
  const usable = buildUsableExport(endpoints);
  const chainCounts = {};
  for (const endpoint of endpoints) {
    if (!endpoint.chain) {
      continue;
    }
    chainCounts[endpoint.chain] = (chainCounts[endpoint.chain] || 0) + 1;
  }
  return {
    updatedAt: state.updatedAt,
    total: endpoints.length,
    usableCount: usable.length,
    chainCounts,
    endpoints,
    usable,
    usableText: JSON.stringify(usable, null, 2)
  };
}

function migrateEndpoint(endpoint) {
  if (!endpoint || typeof endpoint !== "object") {
    return null;
  }
  const credential = detectCredential(endpoint.url || "", endpoint.headers || {});
  endpoint.rpcFamilies = Array.isArray(endpoint.rpcFamilies)
    ? endpoint.rpcFamilies
    : inferRpcFamilies(endpoint.observedMethods || []);
  endpoint.chain = typeof endpoint.chain === "string" ? endpoint.chain : "";
  endpoint.chainName = typeof endpoint.chainName === "string" ? endpoint.chainName : "";
  endpoint.rpcFamily = typeof endpoint.rpcFamily === "string" ? endpoint.rpcFamily : "";
  endpoint.hasCredential = credential.hasCredential;
  endpoint.credentialKind = credential.credentialKind;
  endpoint.validation = endpoint.validation && typeof endpoint.validation === "object" ? endpoint.validation : {};
  if (typeof endpoint.validation.slot === "undefined") {
    endpoint.validation.slot = null;
  }
  if (typeof endpoint.validation.blockNumber === "undefined") {
    endpoint.validation.blockNumber = null;
  }
  if (typeof endpoint.validation.chainId === "undefined") {
    endpoint.validation.chainId = null;
  }
  if (typeof endpoint.validation.clientVersion === "undefined") {
    endpoint.validation.clientVersion = "";
  }
  if (typeof endpoint.validation.error === "undefined") {
    endpoint.validation.error = "";
  }
  return endpoint;
}

function migrateState(rawState) {
  const nextState = rawState && typeof rawState === "object" ? rawState : createEmptyState();
  nextState.endpoints = nextState.endpoints && typeof nextState.endpoints === "object" ? nextState.endpoints : {};
  for (const [key, endpoint] of Object.entries(nextState.endpoints)) {
    const migrated = migrateEndpoint(endpoint);
    if (migrated) {
      nextState.endpoints[key] = migrated;
    } else {
      delete nextState.endpoints[key];
    }
  }
  nextState.updatedAt = nextState.updatedAt || null;
  return nextState;
}

async function loadState() {
  try {
    const saved = await chrome.storage.local.get([STORAGE_KEY, LEGACY_STORAGE_KEY]);
    if (saved && saved[STORAGE_KEY] && typeof saved[STORAGE_KEY] === "object") {
      state = migrateState(saved[STORAGE_KEY]);
    } else if (saved && saved[LEGACY_STORAGE_KEY] && typeof saved[LEGACY_STORAGE_KEY] === "object") {
      state = migrateState(saved[LEGACY_STORAGE_KEY]);
      await persistState();
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
    return "鏈煡閿欒";
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
    return "RPC returned an error";
  }
  const code = typeof errorPayload.code !== "undefined" ? `(${errorPayload.code}) ` : "";
  const message = typeof errorPayload.message === "string" ? errorPayload.message : "RPC returned an error";
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
      throw new Error("Response is not valid JSON");
    }

    if (payload && payload.error) {
      throw new Error(formatRpcError(payload.error));
    }
    if (!payload || typeof payload.result === "undefined") {
      throw new Error("RPC response is missing result");
    }
    return payload.result;
  } finally {
    clearTimeout(timeoutId);
  }
}

async function validateEvmHttpEndpoint(endpoint) {
  const startedAt = Date.now();
  const chainIdRaw = await rpcCallHttp(endpoint.url, endpoint.headers, "eth_chainId", []);
  const chainId = parseChainId(chainIdRaw);
  const chain = EVM_CHAINS_BY_ID.get(chainId);
  if (!chain) {
    return {
      ok: false,
      rawChainId: String(chainIdRaw || ""),
      chainId,
      blockNumber: null,
      slot: null,
      clientVersion: "",
      latencyMs: Date.now() - startedAt,
      error: `Unsupported EVM chainId: ${String(chainIdRaw || "")}`
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
      slot: null,
      clientVersion: "",
      latencyMs: Date.now() - startedAt,
      error: "eth_blockNumber returned an invalid value"
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
    chain: chain.key,
    chainName: chain.name,
    rpcFamily: chain.rpcFamily,
    rawChainId: String(chainIdRaw || ""),
    chainId,
    blockNumber,
    slot: null,
    clientVersion,
    latencyMs: Date.now() - startedAt,
    error: ""
  };
}

async function validateSolanaHttpEndpoint(endpoint) {
  const startedAt = Date.now();
  const slotRaw = await rpcCallHttp(endpoint.url, endpoint.headers, "getSlot", []);
  const slot = typeof slotRaw === "number" && Number.isFinite(slotRaw) ? slotRaw : null;
  if (!Number.isFinite(slot) || slot <= 0) {
    return {
      ok: false,
      chain: "solana",
      chainName: "Solana",
      rpcFamily: "solana",
      chainId: null,
      rawChainId: "",
      blockNumber: null,
      slot: null,
      clientVersion: "",
      latencyMs: Date.now() - startedAt,
      error: "getSlot returned an invalid value"
    };
  }

  let clientVersion = "";
  try {
    const rawVersion = await rpcCallHttp(endpoint.url, endpoint.headers, "getVersion", []);
    if (rawVersion && typeof rawVersion === "object") {
      clientVersion = rawVersion["solana-core"] || stableStringify(rawVersion);
    } else {
      clientVersion = String(rawVersion || "");
    }
  } catch (error) {
    try {
      const health = await rpcCallHttp(endpoint.url, endpoint.headers, "getHealth", []);
      clientVersion = typeof health === "string" ? `health:${health}` : "";
    } catch (healthError) {
      clientVersion = "";
    }
  }

  return {
    ok: true,
    chain: "solana",
    chainName: "Solana",
    rpcFamily: "solana",
    rawChainId: "",
    chainId: null,
    blockNumber: null,
    slot,
    clientVersion,
    latencyMs: Date.now() - startedAt,
    error: ""
  };
}

async function validateHttpEndpointByFamily(endpoint) {
  const families = Array.isArray(endpoint.rpcFamilies) ? endpoint.rpcFamilies : [];
  const orderedFamilies = families.length ? families : ["evm", "solana"];
  let lastResult = null;

  for (const family of orderedFamilies) {
    if (family === "evm") {
      lastResult = await validateEvmHttpEndpoint(endpoint);
    } else if (family === "solana") {
      lastResult = await validateSolanaHttpEndpoint(endpoint);
    } else {
      continue;
    }
    if (lastResult.ok) {
      return lastResult;
    }
  }

  return lastResult || {
    ok: false,
    error: "No supported RPC method family observed",
    latencyMs: 0,
    chain: "",
    chainName: "",
    rpcFamily: "",
    chainId: null,
    rawChainId: "",
    blockNumber: null,
    slot: null,
    clientVersion: ""
  };
}

async function validateHttpEndpoint(endpoint) {
  return validateHttpEndpointByFamily(endpoint);
}

async function validateEvmWsEndpoint(endpoint) {
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
        slot: null,
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
      fail(`WS initialization failed: ${getErrorMessage(error)}`);
      return;
    }

    timeoutId = setTimeout(() => {
      fail("WS validation timed out");
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
      fail("WS connection failed");
    });

    socket.addEventListener("close", (event) => {
      if (!settled) {
        fail(`WS connection closed: ${event.code}`);
      }
    });

    socket.addEventListener("open", async () => {
      try {
        const chainIdRaw = await call("eth_chainId", []);
        const chainId = parseChainId(chainIdRaw);
        const chain = EVM_CHAINS_BY_ID.get(chainId);
        if (!chain) {
          fail(`Unsupported EVM chainId: ${String(chainIdRaw || "")}`, {
            rawChainId: String(chainIdRaw || ""),
            chainId
          });
          return;
        }

        const blockNumberRaw = await call("eth_blockNumber", []);
        const blockNumber = parseBlockNumber(blockNumberRaw);
        if (!Number.isFinite(blockNumber) || blockNumber <= 0) {
          fail("eth_blockNumber returned an invalid value", {
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
          chain: chain.key,
          chainName: chain.name,
          rpcFamily: chain.rpcFamily,
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

async function validateSolanaWsEndpoint(endpoint) {
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
        slot: null,
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
      fail(`WS initialization failed: ${getErrorMessage(error)}`);
      return;
    }

    timeoutId = setTimeout(() => {
      fail("WS validation timed out");
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
      fail("WS connection failed");
    });

    socket.addEventListener("close", (event) => {
      if (!settled) {
        fail(`WS connection closed: ${event.code}`);
      }
    });

    socket.addEventListener("open", async () => {
      try {
        const slotRaw = await call("getSlot", []);
        const slot = typeof slotRaw === "number" && Number.isFinite(slotRaw) ? slotRaw : null;
        if (!Number.isFinite(slot) || slot <= 0) {
          fail("getSlot returned an invalid value");
          return;
        }

        let clientVersion = "";
        try {
          const rawVersion = await call("getVersion", []);
          if (rawVersion && typeof rawVersion === "object") {
            clientVersion = rawVersion["solana-core"] || stableStringify(rawVersion);
          } else {
            clientVersion = String(rawVersion || "");
          }
        } catch (error) {
          clientVersion = "";
        }

        finish({
          ok: true,
          chain: "solana",
          chainName: "Solana",
          rpcFamily: "solana",
          error: "",
          slot,
          clientVersion
        });
      } catch (error) {
        fail(getErrorMessage(error));
      }
    });
  });
}

async function validateWsEndpoint(endpoint) {
  const families = Array.isArray(endpoint.rpcFamilies) ? endpoint.rpcFamilies : [];
  const orderedFamilies = families.length ? families : ["evm", "solana"];
  let lastResult = null;

  for (const family of orderedFamilies) {
    if (family === "evm") {
      lastResult = await validateEvmWsEndpoint(endpoint);
    } else if (family === "solana") {
      lastResult = await validateSolanaWsEndpoint(endpoint);
    } else {
      continue;
    }
    if (lastResult.ok) {
      return lastResult;
    }
  }

  return lastResult || {
    ok: false,
    error: "No supported RPC method family observed",
    latencyMs: 0,
    chain: "",
    chainName: "",
    rpcFamily: "",
    chainId: null,
    rawChainId: "",
    blockNumber: null,
    slot: null,
    clientVersion: ""
  };
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
        chain: "",
        chainName: "",
        rpcFamily: "",
        chainId: null,
        rawChainId: "",
        blockNumber: null,
        slot: null,
        clientVersion: ""
      };
    }

    const current = state.endpoints[endpointKey];
    if (!current) {
      return result;
    }

    current.status = result.ok ? "usable" : "invalid";
    current.chain = result.chain || "";
    current.chainName = result.chainName || "";
    current.rpcFamily = result.rpcFamily || "";
    const credential = detectCredential(current.url, current.headers);
    current.hasCredential = credential.hasCredential;
    current.credentialKind = credential.credentialKind;
    current.validation = {
      checkedAt: new Date().toISOString(),
      latencyMs: result.latencyMs || 0,
      chainId: typeof result.chainId === "number" ? result.chainId : null,
      rawChainId: result.rawChainId || "",
      blockNumber: typeof result.blockNumber === "number" ? result.blockNumber : null,
      slot: typeof result.slot === "number" ? result.slot : null,
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
        error: "Invalid message"
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
            error: "Missing endpoint key"
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
          error: `鏈煡娑堟伅绫诲瀷: ${String(message.type)}`
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
