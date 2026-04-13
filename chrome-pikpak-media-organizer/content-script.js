(() => {
  const TAB_MESSAGE_TYPE = "pikpak-organizer-tab-command";
  const PAGE_PROGRESS_TYPE = "pikpak-organizer-page-progress";
  const CONTENT_SOURCE = "pikpak-organizer-content";
  const PAGE_SOURCE = "pikpak-organizer-page";
  const REQUEST_TIMEOUT_MS = 30 * 60 * 1000;

  window.addEventListener("message", handlePageProgressMessage);

  chrome.runtime.onMessage.addListener((message, sender, sendResponse) => {
    if (!message || message.type !== TAB_MESSAGE_TYPE || !message.command) {
      return false;
    }

    const requestId = createRequestId();
    const timeoutId = setTimeout(() => {
      window.removeEventListener("message", handleResponse);
      sendResponse({
        ok: false,
        error: "页面响应超时"
      });
    }, REQUEST_TIMEOUT_MS);

    function handleResponse(event) {
      if (event.source !== window || !event.data || event.data.source !== PAGE_SOURCE) {
        return;
      }
      if (event.data.requestId !== requestId) {
        return;
      }
      clearTimeout(timeoutId);
      window.removeEventListener("message", handleResponse);
      sendResponse({
        ok: Boolean(event.data.ok),
        payload: event.data.payload,
        error: event.data.error || ""
      });
    }

    window.addEventListener("message", handleResponse);
    window.postMessage(
      {
        source: CONTENT_SOURCE,
        requestId,
        command: message.command,
        payload: message.payload || {}
      },
      "*"
    );
    return true;
  });

  function createRequestId() {
    return "pikpak-organizer-" + Date.now() + "-" + Math.random().toString(36).slice(2, 10);
  }

  function handlePageProgressMessage(event) {
    if (event.source !== window || !event.data || event.data.source !== PAGE_SOURCE) {
      return;
    }
    if (event.data.kind !== "progress") {
      return;
    }
    void chrome.runtime.sendMessage({
      type: PAGE_PROGRESS_TYPE,
      progress: event.data.payload || {}
    }).catch(() => {});
  }
})();
