(function () {
  const BRIDGE_FLAG = "__BSC_RPC_SNIFFER_BRIDGE__";
  const MESSAGE_SOURCE = "bsc-rpc-sniffer:page";

  if (window[BRIDGE_FLAG]) {
    return;
  }
  window[BRIDGE_FLAG] = true;

  window.addEventListener("message", (event) => {
    if (event.source !== window || !event.data || event.data.source !== MESSAGE_SOURCE) {
      return;
    }

    chrome.runtime.sendMessage(
      {
        type: "network-observation",
        payload: event.data.payload
      },
      () => {
        void chrome.runtime.lastError;
      }
    );
  });
})();
