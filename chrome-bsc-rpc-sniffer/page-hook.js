(function () {
  const HOOK_FLAG = "__BSC_RPC_SNIFFER_PAGE_HOOK__";
  const MESSAGE_SOURCE = "bsc-rpc-sniffer:page";
  const RPC_METHOD_PATTERN = /^(eth|net|web3|debug|trace|txpool|parity|bor|engine)_/i;

  if (window[HOOK_FLAG]) {
    return;
  }
  window[HOOK_FLAG] = true;

  function now() {
    return new Date().toISOString();
  }

  function toAbsoluteUrl(rawUrl) {
    try {
      return new URL(String(rawUrl), window.location.href).href;
    } catch (error) {
      return "";
    }
  }

  function transportFromUrl(rawUrl) {
    try {
      const parsed = new URL(rawUrl);
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

  function cloneHeaders(headersLike) {
    const headers = {};
    if (!headersLike) {
      return headers;
    }

    try {
      if (typeof Headers !== "undefined" && headersLike instanceof Headers) {
        headersLike.forEach((value, key) => {
          headers[String(key).toLowerCase()] = String(value);
        });
        return headers;
      }
    } catch (error) {
      return headers;
    }

    if (Array.isArray(headersLike)) {
      for (const entry of headersLike) {
        if (!Array.isArray(entry) || entry.length < 2) {
          continue;
        }
        headers[String(entry[0]).toLowerCase()] = String(entry[1]);
      }
      return headers;
    }

    if (typeof headersLike === "object") {
      for (const [key, value] of Object.entries(headersLike)) {
        if (typeof value === "undefined" || value === null) {
          continue;
        }
        headers[String(key).toLowerCase()] = Array.isArray(value)
          ? value.join(", ")
          : String(value);
      }
    }

    return headers;
  }

  function mergeHeaders(baseHeaders, extraHeaders) {
    return Object.assign({}, cloneHeaders(baseHeaders), cloneHeaders(extraHeaders));
  }

  function decodeArrayBuffer(buffer) {
    try {
      return new TextDecoder().decode(buffer);
    } catch (error) {
      return "";
    }
  }

  function normalizeMethods(methods) {
    const seen = new Set();
    const list = [];
    for (const value of methods || []) {
      if (typeof value !== "string") {
        continue;
      }
      const method = value.trim();
      if (!method || !RPC_METHOD_PATTERN.test(method) || seen.has(method)) {
        continue;
      }
      seen.add(method);
      list.push(method);
    }
    return list;
  }

  function extractMethodsFromParsed(parsed) {
    const items = Array.isArray(parsed) ? parsed : [parsed];
    const methods = [];
    for (const item of items) {
      if (!item || typeof item !== "object") {
        continue;
      }
      if (typeof item.method === "string") {
        methods.push(item.method);
      }
    }
    return normalizeMethods(methods);
  }

  function extractMethodsFromBody(body) {
    if (body === null || typeof body === "undefined") {
      return Promise.resolve([]);
    }

    if (typeof body === "string") {
      try {
        return Promise.resolve(extractMethodsFromParsed(JSON.parse(body)));
      } catch (error) {
        return Promise.resolve([]);
      }
    }

    if (typeof URLSearchParams !== "undefined" && body instanceof URLSearchParams) {
      try {
        const maybeJson = body.get("payload") || body.get("data");
        if (!maybeJson) {
          return Promise.resolve([]);
        }
        return Promise.resolve(extractMethodsFromParsed(JSON.parse(maybeJson)));
      } catch (error) {
        return Promise.resolve([]);
      }
    }

    if (typeof Blob !== "undefined" && body instanceof Blob) {
      return body.text().then(extractMethodsFromBody).catch(() => []);
    }

    if (typeof ArrayBuffer !== "undefined" && body instanceof ArrayBuffer) {
      return extractMethodsFromBody(decodeArrayBuffer(body));
    }

    if (typeof ArrayBuffer !== "undefined" && ArrayBuffer.isView(body)) {
      const view = body;
      const start = view.byteOffset || 0;
      const end = start + view.byteLength;
      return extractMethodsFromBody(view.buffer.slice(start, end));
    }

    if (typeof body === "object") {
      return Promise.resolve(extractMethodsFromParsed(body));
    }

    return Promise.resolve([]);
  }

  function postObservation(payload) {
    try {
      window.postMessage(
        {
          source: MESSAGE_SOURCE,
          payload
        },
        "*"
      );
    } catch (error) {
      // ignore postMessage failures
    }
  }

  function observeRpc(url, body, transportHint, headers) {
    const absoluteUrl = toAbsoluteUrl(url);
    if (!absoluteUrl) {
      return;
    }

    extractMethodsFromBody(body)
      .then((methods) => {
        const rpcMethods = normalizeMethods(methods);
        if (!rpcMethods.length) {
          return;
        }

        const transport = transportHint || transportFromUrl(absoluteUrl);
        if (!transport) {
          return;
        }

        postObservation({
          url: absoluteUrl,
          transport,
          headers: cloneHeaders(headers),
          methods: rpcMethods,
          pageUrl: window.location.href,
          capturedAt: now()
        });
      })
      .catch(() => {
        // ignore parsing failures
      });
  }

  const originalFetch = window.fetch;
  if (typeof originalFetch === "function") {
    window.fetch = function patchedFetch(input, init) {
      let requestUrl = "";
      let requestHeaders = {};
      let requestBody = init && Object.prototype.hasOwnProperty.call(init, "body") ? init.body : undefined;

      try {
        if (typeof input === "string" || input instanceof URL) {
          requestUrl = String(input);
        } else if (input && typeof input.url === "string") {
          requestUrl = input.url;
        }

        if (input && typeof Request !== "undefined" && input instanceof Request) {
          requestHeaders = mergeHeaders(input.headers, init && init.headers);
          if (typeof requestBody === "undefined") {
            input
              .clone()
              .text()
              .then((text) => observeRpc(requestUrl || input.url, text, "http", requestHeaders))
              .catch(() => {});
          }
        } else {
          requestHeaders = cloneHeaders(init && init.headers);
        }
      } catch (error) {
        // ignore fetch inspection failures
      }

      if (typeof requestBody !== "undefined") {
        observeRpc(requestUrl, requestBody, "http", requestHeaders);
      }

      return originalFetch.apply(this, arguments);
    };
  }

  const originalOpen = XMLHttpRequest.prototype.open;
  const originalSend = XMLHttpRequest.prototype.send;
  const originalSetRequestHeader = XMLHttpRequest.prototype.setRequestHeader;

  XMLHttpRequest.prototype.open = function patchedOpen(method, url) {
    this.__bscRpcSnifferUrl = url;
    this.__bscRpcSnifferHeaders = {};
    return originalOpen.apply(this, arguments);
  };

  XMLHttpRequest.prototype.setRequestHeader = function patchedSetRequestHeader(name, value) {
    if (!this.__bscRpcSnifferHeaders) {
      this.__bscRpcSnifferHeaders = {};
    }
    this.__bscRpcSnifferHeaders[String(name).toLowerCase()] = String(value);
    return originalSetRequestHeader.apply(this, arguments);
  };

  XMLHttpRequest.prototype.send = function patchedSend(body) {
    const targetUrl = this.__bscRpcSnifferUrl;
    const headers = this.__bscRpcSnifferHeaders || {};
    if (targetUrl) {
      observeRpc(targetUrl, body, "http", headers);
    }
    return originalSend.apply(this, arguments);
  };

  if (typeof WebSocket === "function") {
    const OriginalWebSocket = window.WebSocket;
    const originalWebSocketSend = OriginalWebSocket.prototype.send;

    function WrappedWebSocket(url, protocols) {
      if (!(this instanceof WrappedWebSocket)) {
        return new WrappedWebSocket(url, protocols);
      }

      let socket;
      if (typeof protocols === "undefined") {
        socket = new OriginalWebSocket(url);
      } else {
        socket = new OriginalWebSocket(url, protocols);
      }
      return socket;
    }

    WrappedWebSocket.prototype = OriginalWebSocket.prototype;
    Object.setPrototypeOf(WrappedWebSocket, OriginalWebSocket);
    WrappedWebSocket.CONNECTING = OriginalWebSocket.CONNECTING;
    WrappedWebSocket.OPEN = OriginalWebSocket.OPEN;
    WrappedWebSocket.CLOSING = OriginalWebSocket.CLOSING;
    WrappedWebSocket.CLOSED = OriginalWebSocket.CLOSED;

    OriginalWebSocket.prototype.send = function patchedWebSocketSend(data) {
      const socketUrl = this.url;
      if (socketUrl) {
        observeRpc(socketUrl, data, "ws", {});
      }
      return originalWebSocketSend.apply(this, arguments);
    };

    window.WebSocket = WrappedWebSocket;
  }
})();
