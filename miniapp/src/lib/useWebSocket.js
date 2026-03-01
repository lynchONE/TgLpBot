import { useEffect, useRef, useCallback } from 'react';

/**
 * useWebSocket - manages a WebSocket connection with auto-reconnect.
 *
 * @param {Object} opts
 * @param {string} opts.url        WebSocket URL (ws:// or wss://)
 * @param {function} opts.onMessage  Called with parsed JSON message
 * @param {boolean} [opts.enabled=true]  Set false to disable connection
 */
export function useWebSocket({ url, onMessage, enabled = true }) {
    const wsRef = useRef(null);
    const reconnectTimer = useRef(null);
    const attemptRef = useRef(0);
    const onMessageRef = useRef(onMessage);
    onMessageRef.current = onMessage;

    const connect = useCallback(() => {
        if (!url || !enabled) return;

        try {
            const ws = new WebSocket(url);
            wsRef.current = ws;

            ws.onopen = () => {
                attemptRef.current = 0;
            };

            ws.onmessage = (event) => {
                try {
                    const data = JSON.parse(event.data);
                    if (onMessageRef.current) {
                        onMessageRef.current(data);
                    }
                } catch {
                    // ignore non-JSON messages (pings etc.)
                }
            };

            ws.onclose = () => {
                wsRef.current = null;
                if (!enabled) return;
                // Exponential backoff: 1s, 2s, 4s, 8s, ... max 30s
                const delay = Math.min(1000 * Math.pow(2, attemptRef.current), 30000);
                attemptRef.current += 1;
                reconnectTimer.current = setTimeout(connect, delay);
            };

            ws.onerror = () => {
                // onclose will fire after onerror, triggering reconnect
            };
        } catch {
            // bad URL etc. - schedule retry
            const delay = Math.min(1000 * Math.pow(2, attemptRef.current), 30000);
            attemptRef.current += 1;
            reconnectTimer.current = setTimeout(connect, delay);
        }
    }, [url, enabled]);

    useEffect(() => {
        if (!enabled || !url) return;
        connect();

        return () => {
            if (reconnectTimer.current) {
                clearTimeout(reconnectTimer.current);
                reconnectTimer.current = null;
            }
            if (wsRef.current) {
                wsRef.current.onclose = null; // prevent reconnect on intentional close
                wsRef.current.close();
                wsRef.current = null;
            }
        };
    }, [connect, enabled, url]);
}
