import { useCallback, useEffect, useRef, useState } from 'react';
import { WEBAPP_CONFIG } from '../config';

function buildWsUrl(initData) {
  let base = String(WEBAPP_CONFIG.wsBaseUrl || '').trim();
  if (!base) {
    base = String(WEBAPP_CONFIG.apiBaseUrl || '').trim().replace(/\/$/, '');
  }
  const wsBase = base.replace(/^http/, 'ws');
  return `${wsBase}/api/ws?initData=${encodeURIComponent(initData)}`;
}

/**
 * Hook that maintains a WebSocket connection for real-time kline updates.
 *
 * @param {object} opts
 * @param {string} opts.initData  - Telegram initData for auth
 * @param {string} opts.chain     - Chain name (bsc / base)
 * @param {string} opts.tokenAddress - Token contract address
 * @param {string} opts.bar       - Candle bar interval (1m / 5m / 15m / 1H)
 * @param {boolean} opts.enabled  - Whether to connect
 * @returns {{ lastUpdate: object|null, connected: boolean }}
 */
export default function useKlineWS({ initData, chain, tokenAddress, bar, enabled = true }) {
  const wsRef = useRef(null);
  const reconnectTimerRef = useRef(null);
  const subRef = useRef(null);
  const [lastUpdate, setLastUpdate] = useState(null);
  const [connected, setConnected] = useState(false);

  const sendSubscribe = useCallback((socket, c, t, b) => {
    if (!socket || socket.readyState !== WebSocket.OPEN) return;
    if (!t || !b) return;
    socket.send(JSON.stringify({
      type: 'subscribe_kline',
      chain: c || 'bsc',
      token_address: t,
      bar: b,
    }));
    subRef.current = { chain: c, tokenAddress: t, bar: b };
  }, []);

  const sendUnsubscribe = useCallback((socket) => {
    if (!socket || socket.readyState !== WebSocket.OPEN) return;
    socket.send(JSON.stringify({ type: 'unsubscribe_kline' }));
    subRef.current = null;
  }, []);

  // Manage connection lifecycle.
  useEffect(() => {
    if (!initData || !enabled) {
      if (wsRef.current) {
        wsRef.current.close();
        wsRef.current = null;
      }
      setConnected(false);
      return;
    }

    let alive = true;

    const connect = () => {
      if (!alive) return;
      const url = buildWsUrl(initData);
      const socket = new WebSocket(url);
      wsRef.current = socket;

      socket.onopen = () => {
        if (!alive) { socket.close(); return; }
        setConnected(true);
        if (tokenAddress && bar) {
          sendSubscribe(socket, chain, tokenAddress, bar);
        }
      };

      socket.onmessage = (event) => {
        try {
          const data = JSON.parse(event.data);
          if (data.type === 'kline_update') {
            setLastUpdate(data);
          }
        } catch { /* ignore non-JSON */ }
      };

      socket.onclose = () => {
        if (!alive) return;
        setConnected(false);
        wsRef.current = null;
        subRef.current = null;
        reconnectTimerRef.current = setTimeout(connect, 3000);
      };

      socket.onerror = () => { /* onclose fires after onerror */ };
    };

    connect();

    return () => {
      alive = false;
      if (reconnectTimerRef.current) {
        clearTimeout(reconnectTimerRef.current);
        reconnectTimerRef.current = null;
      }
      if (wsRef.current) {
        wsRef.current.close();
        wsRef.current = null;
      }
      subRef.current = null;
      setConnected(false);
    };
  }, [initData, enabled]); // eslint-disable-line react-hooks/exhaustive-deps

  // Update subscription when params change.
  useEffect(() => {
    const socket = wsRef.current;
    if (!socket || socket.readyState !== WebSocket.OPEN) return;

    if (!tokenAddress || !bar) {
      sendUnsubscribe(socket);
      return;
    }

    const cur = subRef.current;
    if (cur?.chain === chain && cur?.tokenAddress === tokenAddress && cur?.bar === bar) {
      return;
    }

    sendSubscribe(socket, chain, tokenAddress, bar);
  }, [chain, tokenAddress, bar, connected, sendSubscribe, sendUnsubscribe]);

  return { lastUpdate, connected };
}
