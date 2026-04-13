(function initPikPakOrganizerShared(global) {
  const STORAGE_KEYS = Object.freeze({
    settings: "pikpakMediaOrganizer.settings",
    plan: "pikpakMediaOrganizer.plan",
    metadataCache: "pikpakMediaOrganizer.metadataCache"
  });

  const DEFAULT_SETTINGS = Object.freeze({
    recursiveScan: true,
    minVideoSizeMb: 100,
    subtitleKeywords: [
      "中字",
      "中文字幕",
      "中文",
      "chs",
      "cht",
      "c-sub",
      "c_sub",
      "sub",
      "字幕",
      "cnsub"
    ],
    metadataProviders: [
      "missavJina",
      "duckduckgoSearch",
      "jableJina",
      "javDatabaseJina"
    ],
    codeActressOverrides: {},
    requireChineseActressName: true,
    yearOnlyFallbackEnabled: true,
    deleteMode: "permanent",
    metadataCacheTtlHours: 24 * 30,
    providerTimeoutMs: 20000,
    maxMetadataConcurrency: 2
  });

  const VIDEO_EXTENSIONS = new Set([
    "3gp",
    "asf",
    "avi",
    "flv",
    "m2ts",
    "m4v",
    "mkv",
    "mov",
    "mp4",
    "mpeg",
    "mpg",
    "mts",
    "rm",
    "rmvb",
    "ts",
    "vob",
    "webm",
    "wmv"
  ]);

  const PROVIDER_LABELS = Object.freeze({
    manual_override: "手动覆盖",
    missav_jina: "MissAV(Jina)",
    duckduckgo_search: "DuckDuckGo",
    jable_jina: "Jable(Jina)",
    javdatabase_jina: "JAV Database(Jina)"
  });

  const REASON_LABELS = Object.freeze({
    duplicate_code: "番号重复",
    smaller_file: "小于阈值文件",
    empty_folder: "空目录清理",
    no_code: "未识别到番号",
    no_year: "缺少添加年份",
    no_actress_zh: "未拿到女优中文名",
    already_sorted: "已在目标目录",
    non_video: "非视频文件"
  });

  function cloneDefaultSettings() {
    return JSON.parse(JSON.stringify(DEFAULT_SETTINGS));
  }

  function normalizeSettings(input) {
    const base = cloneDefaultSettings();
    const next = input && typeof input === "object" ? input : {};
    base.recursiveScan = Boolean(next.recursiveScan);
    base.minVideoSizeMb = normalizePositiveNumber(next.minVideoSizeMb, DEFAULT_SETTINGS.minVideoSizeMb);
    base.subtitleKeywords = normalizeStringArray(next.subtitleKeywords, DEFAULT_SETTINGS.subtitleKeywords);
    base.metadataProviders = normalizeStringArray(next.metadataProviders, DEFAULT_SETTINGS.metadataProviders);
    base.requireChineseActressName = next.requireChineseActressName !== false;
    base.yearOnlyFallbackEnabled = Boolean(next.yearOnlyFallbackEnabled);
    base.deleteMode = next.deleteMode === "trash" ? "trash" : "permanent";
    base.metadataCacheTtlHours = normalizePositiveNumber(
      next.metadataCacheTtlHours,
      DEFAULT_SETTINGS.metadataCacheTtlHours
    );
    base.providerTimeoutMs = normalizePositiveNumber(next.providerTimeoutMs, DEFAULT_SETTINGS.providerTimeoutMs);
    base.maxMetadataConcurrency = Math.max(
      1,
      Math.min(4, Math.round(normalizePositiveNumber(next.maxMetadataConcurrency, 2)))
    );
    base.codeActressOverrides = normalizeCodeOverrideMap(next.codeActressOverrides);
    if (!base.metadataProviders.length) {
      base.metadataProviders = [...DEFAULT_SETTINGS.metadataProviders];
    }
    return base;
  }

  function normalizePositiveNumber(value, fallback) {
    const parsed = Number(value);
    if (!Number.isFinite(parsed) || parsed <= 0) {
      return fallback;
    }
    return parsed;
  }

  function normalizeStringArray(value, fallback) {
    const source = Array.isArray(value) ? value : fallback;
    const seen = new Set();
    const list = [];
    for (const entry of source) {
      if (typeof entry !== "string") {
        continue;
      }
      const cleaned = entry.trim();
      if (!cleaned) {
        continue;
      }
      const key = cleaned.toLowerCase();
      if (seen.has(key)) {
        continue;
      }
      seen.add(key);
      list.push(cleaned);
    }
    return list;
  }

  function normalizeCodeOverrideMap(value) {
    if (!value || typeof value !== "object" || Array.isArray(value)) {
      return {};
    }
    const map = {};
    for (const [rawCode, rawName] of Object.entries(value)) {
      const code = extractCode(rawCode);
      const actressNameZh = sanitizeFolderName(String(rawName || "").trim());
      if (!code || !actressNameZh) {
        continue;
      }
      map[code] = actressNameZh;
    }
    return map;
  }

  function stripExtension(name) {
    return String(name || "").replace(/\.[^.]+$/, "");
  }

  function extractCode(name) {
    const text = stripExtension(String(name || ""))
      .replace(/_/g, "-")
      .replace(/\s+/g, " ")
      .toUpperCase();

    const fc2Match = text.match(/FC2[-\s]*PPV[-\s]*(\d{5,9})/i);
    if (fc2Match) {
      return "FC2-PPV-" + fc2Match[1];
    }

    const match = text.match(/(?:^|[^A-Z0-9])([A-Z]{2,10})[-\s]?(\d{2,6})(?:[^A-Z0-9]|$)/);
    if (!match) {
      return "";
    }
    const prefix = match[1].replace(/[^A-Z]/g, "");
    const digits = match[2].replace(/\D/g, "");
    return prefix && digits ? prefix + "-" + digits : "";
  }

  function detectSubtitle(name, keywords) {
    const source = String(name || "").toLowerCase();
    const list = normalizeStringArray(keywords, DEFAULT_SETTINGS.subtitleKeywords);
    if (list.some((keyword) => source.includes(keyword.toLowerCase()))) {
      return true;
    }

    const code = extractCode(name);
    if (!code) {
      return false;
    }

    const segments = code.split("-").map((segment) => escapeRegExp(segment)).filter(Boolean);
    if (!segments.length) {
      return false;
    }

    const normalized = stripExtension(String(name || ""))
      .toUpperCase()
      .replace(/[_\s]+/g, "-");
    const pattern = new RegExp(
      `(?:^|[^A-Z0-9])${segments.join("[-_\\s]?")}(?:[-_\\s]?C)(?:[^A-Z0-9]|$)`,
      "i"
    );
    return pattern.test(normalized);
  }

  function getExtension(name) {
    const match = String(name || "").match(/\.([^.]+)$/);
    return match ? match[1].toLowerCase() : "";
  }

  function isVideoItem(item) {
    if (!item || typeof item !== "object") {
      return false;
    }
    const kind = String(item.kind || "");
    if (kind === "drive#folder") {
      return false;
    }
    const mimeType = String(item.mimeType || item.mime_type || "").toLowerCase();
    if (mimeType.startsWith("video/")) {
      return true;
    }
    return VIDEO_EXTENSIONS.has(getExtension(item.name || item.file_name || ""));
  }

  function parseDate(value) {
    if (!value) {
      return null;
    }
    const date = new Date(value);
    return Number.isNaN(date.getTime()) ? null : date;
  }

  function getYearFromTimestamp(value) {
    const date = parseDate(value);
    return date ? String(date.getFullYear()) : "";
  }

  function toBytesFromMegabytes(value) {
    return Math.round(normalizePositiveNumber(value, DEFAULT_SETTINGS.minVideoSizeMb) * 1024 * 1024);
  }

  function formatBytes(bytes) {
    const value = Number(bytes);
    if (!Number.isFinite(value) || value <= 0) {
      return "0 B";
    }
    const units = ["B", "KB", "MB", "GB", "TB"];
    let size = value;
    let unitIndex = 0;
    while (size >= 1024 && unitIndex < units.length - 1) {
      size /= 1024;
      unitIndex += 1;
    }
    const digits = size >= 100 ? 0 : size >= 10 ? 1 : 2;
    return size.toFixed(digits).replace(/\.0+$/, "").replace(/(\.\d*[1-9])0+$/, "$1") + " " + units[unitIndex];
  }

  function sanitizeFolderName(value) {
    return String(value || "")
      .replace(/[\\/:*?"<>|]/g, " ")
      .replace(/\s+/g, " ")
      .trim();
  }

  function hasCjk(text) {
    return /[\u3400-\u9fff]/.test(String(text || ""));
  }

  function buildPathKey(segments) {
    if (!Array.isArray(segments) || !segments.length) {
      return "";
    }
    return segments.map((segment) => sanitizeFolderName(segment)).filter(Boolean).join("/");
  }

  function buildDisplayPath(segments) {
    return buildPathKey(segments) || "/";
  }

  function escapeRegExp(value) {
    return String(value || "").replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  }

  function chunkArray(items, chunkSize) {
    const size = Math.max(1, Math.round(Number(chunkSize) || 1));
    const chunks = [];
    for (let index = 0; index < items.length; index += size) {
      chunks.push(items.slice(index, index + size));
    }
    return chunks;
  }

  function compareKeepCandidates(left, right) {
    const leftSubtitle = left.hasSubtitle ? 1 : 0;
    const rightSubtitle = right.hasSubtitle ? 1 : 0;
    if (leftSubtitle !== rightSubtitle) {
      return rightSubtitle - leftSubtitle;
    }

    const leftSize = Number(left.size) || 0;
    const rightSize = Number(right.size) || 0;
    if (leftSize !== rightSize) {
      return rightSize - leftSize;
    }

    const leftTime = parseDate(left.addedAt || left.createdAt);
    const rightTime = parseDate(right.addedAt || right.createdAt);
    const leftTimestamp = leftTime ? leftTime.getTime() : Number.MAX_SAFE_INTEGER;
    const rightTimestamp = rightTime ? rightTime.getTime() : Number.MAX_SAFE_INTEGER;
    if (leftTimestamp !== rightTimestamp) {
      return leftTimestamp - rightTimestamp;
    }

    return String(left.itemId || "").localeCompare(String(right.itemId || ""));
  }

  function decodeHtmlEntities(text) {
    return String(text || "")
      .replace(/&nbsp;/g, " ")
      .replace(/&amp;/g, "&")
      .replace(/&quot;/g, '"')
      .replace(/&#39;/g, "'")
      .replace(/&lt;/g, "<")
      .replace(/&gt;/g, ">");
  }

  function stripHtml(text) {
    return decodeHtmlEntities(String(text || "").replace(/<[^>]+>/g, " ")).replace(/\s+/g, " ").trim();
  }

  function providerLabel(providerId) {
    return PROVIDER_LABELS[providerId] || providerId || "";
  }

  function reasonLabel(reasonId) {
    if (reasonId === "source_folder_cleanup") {
      return "Source folder cleanup";
    }
    return REASON_LABELS[reasonId] || reasonId || "";
  }

  function codeToSlug(code) {
    return String(code || "").toLowerCase().replace(/[^a-z0-9]+/g, "-");
  }

  global.PikPakOrganizerShared = Object.freeze({
    STORAGE_KEYS,
    DEFAULT_SETTINGS,
    buildDisplayPath,
    buildPathKey,
    chunkArray,
    cloneDefaultSettings,
    codeToSlug,
    compareKeepCandidates,
    decodeHtmlEntities,
    detectSubtitle,
    escapeRegExp,
    extractCode,
    formatBytes,
    getExtension,
    getYearFromTimestamp,
    hasCjk,
    isVideoItem,
    normalizeSettings,
    parseDate,
    providerLabel,
    reasonLabel,
    sanitizeFolderName,
    stripHtml,
    stripExtension,
    toBytesFromMegabytes
  });
})(globalThis);
