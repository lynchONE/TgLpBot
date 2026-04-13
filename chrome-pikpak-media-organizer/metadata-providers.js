(function initPikPakOrganizerMetadataProviders(global) {
  const Shared = global.PikPakOrganizerShared;

  async function resolveActressMetadata(code, settings, fetchText) {
    const overrideName = settings.codeActressOverrides[code];
    if (overrideName) {
      return {
        code,
        actressNameZh: overrideName,
        actressNameOriginal: overrideName,
        source: "manual_override",
        confidence: 1
      };
    }

    const providers = normalizeProviders(settings.metadataProviders);
    for (const providerId of providers) {
      try {
        const result = await providerMap[providerId](code, settings, fetchText);
        if (result && (result.actressNameZh || result.actressNameOriginal)) {
          return {
            code,
            actressNameZh: result.actressNameZh || "",
            actressNameOriginal: result.actressNameOriginal || "",
            source: result.source || providerId,
            confidence: Number.isFinite(result.confidence) ? result.confidence : 0.5
          };
        }
      } catch (error) {
        console.warn("[pikpak-organizer] metadata provider failed:", providerId, error);
      }
    }

    return {
      code,
      actressNameZh: "",
      actressNameOriginal: "",
      source: "",
      confidence: 0
    };
  }

  function normalizeProviders(list) {
    const normalized = Array.isArray(list) ? list.slice() : [];
    return normalized.filter((providerId) => providerMap[providerId]);
  }

  async function resolveFromMissavJina(code, settings, fetchText) {
    const slug = Shared.codeToSlug(code);
    const text = await fetchText("https://r.jina.ai/http://missav.live/dm80/cn/" + slug, settings.providerTimeoutMs);
    const title = matchFirstGroup(text, /^Title:\s*(.+)$/m);
    if (!title) {
      return null;
    }

    const segments = title.split(" - ").map((segment) => segment.trim()).filter(Boolean);
    const candidate = segments.length ? segments[segments.length - 1] : "";
    const actressNameZh = Shared.hasCjk(candidate) ? Shared.sanitizeFolderName(candidate) : "";
    if (!actressNameZh) {
      return null;
    }

    return {
      actressNameZh,
      actressNameOriginal: actressNameZh,
      source: "missav_jina",
      confidence: 0.92
    };
  }

  async function resolveFromDuckDuckGoSearch(code, settings, fetchText) {
    const url = "https://duckduckgo.com/html/?q=" + encodeURIComponent(code + " 女优");
    const html = await fetchText(url, settings.providerTimeoutMs);
    const blocks = Array.from(html.matchAll(/<div class="result results_links[\s\S]*?<\/div>\s*<\/div>/g)).map(
      (match) => match[0]
    );
    for (const block of blocks) {
      const title = Shared.stripHtml(matchFirstGroup(block, /<a[^>]*class="result__a"[^>]*>([\s\S]*?)<\/a>/i));
      const snippet = Shared.stripHtml(matchFirstGroup(block, /<a[^>]*class="result__snippet"[^>]*>([\s\S]*?)<\/a>/i));
      const resultUrl = Shared.stripHtml(matchFirstGroup(block, /<a[^>]*class="result__url"[^>]*>([\s\S]*?)<\/a>/i)).toLowerCase();

      let actressNameZh = "";
      if (resultUrl.includes("missav")) {
        actressNameZh = extractTrailingCjkName(title);
      } else if (resultUrl.includes("getav")) {
        actressNameZh = extractLabelValue(snippet, /主演[:：]\s*([^。]+)/i);
      } else if (resultUrl.includes("avwikidb")) {
        actressNameZh = extractTrailingCjkName(snippet);
      } else if (resultUrl.includes("jable")) {
        actressNameZh = extractTrailingCjkName(title);
      } else {
        actressNameZh = extractLabelValue(snippet, /主演[:：]\s*([^。]+)/i) || extractTrailingCjkName(title);
      }

      actressNameZh = Shared.sanitizeFolderName(firstActorName(actressNameZh));
      if (actressNameZh && Shared.hasCjk(actressNameZh)) {
        return {
          actressNameZh,
          actressNameOriginal: actressNameZh,
          source: "duckduckgo_search",
          confidence: 0.72
        };
      }
    }
    return null;
  }

  async function resolveFromJableJina(code, settings, fetchText) {
    const slug = Shared.codeToSlug(code);
    const text = await fetchText("https://r.jina.ai/http://jable.tv/videos/" + slug + "/", settings.providerTimeoutMs);
    const title = matchFirstGroup(text, /^Title:\s*(.+)$/m);
    const candidate = extractTrailingCjkName(title);
    if (!candidate) {
      return null;
    }
    return {
      actressNameZh: "",
      actressNameOriginal: candidate,
      source: "jable_jina",
      confidence: 0.55
    };
  }

  async function resolveFromJavDatabaseJina(code, settings, fetchText) {
    const slug = Shared.codeToSlug(code);
    const text = await fetchText(
      "https://r.jina.ai/http://www.javdatabase.com/movies/" + slug + "/",
      settings.providerTimeoutMs
    );
    const title = matchFirstGroup(text, /^Title:\s*(.+)$/m);
    if (!title) {
      return null;
    }
    const match = title.match(/^[A-Z0-9-]+\s*-\s*(.+?)\s*-\s*JAV Database$/i);
    const actressNameOriginal = match ? match[1].trim() : "";
    if (!actressNameOriginal) {
      return null;
    }
    return {
      actressNameZh: "",
      actressNameOriginal,
      source: "javdatabase_jina",
      confidence: 0.5
    };
  }

  function matchFirstGroup(text, pattern) {
    const match = String(text || "").match(pattern);
    return match ? match[1].trim() : "";
  }

  function extractTrailingCjkName(text) {
    const cleaned = String(text || "")
      .replace(/\s*-\s*MissAV$/i, "")
      .replace(/\s*-\s*Jable.*$/i, "")
      .trim();
    const matches = Array.from(cleaned.matchAll(/([\u3400-\u9fff]{2,8}(?:[·・][\u3400-\u9fff]{1,4})?)/g)).map(
      (match) => match[1]
    );
    return matches.length ? matches[matches.length - 1] : "";
  }

  function extractLabelValue(text, pattern) {
    const match = String(text || "").match(pattern);
    return match ? match[1].trim() : "";
  }

  function firstActorName(text) {
    return String(text || "")
      .split(/[、,，/]/)
      .map((item) => item.trim())
      .filter(Boolean)[0] || "";
  }

  const providerMap = Object.freeze({
    missavJina: resolveFromMissavJina,
    duckduckgoSearch: resolveFromDuckDuckGoSearch,
    jableJina: resolveFromJableJina,
    javDatabaseJina: resolveFromJavDatabaseJina
  });

  global.PikPakOrganizerMetadataProviders = Object.freeze({
    providerMap,
    resolveActressMetadata
  });
})(globalThis);
