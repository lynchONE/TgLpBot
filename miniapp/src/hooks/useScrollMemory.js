import { useEffect, useRef } from 'react';

const STORAGE_PREFIX = 'tglp_scroll_v1:';

function readScroll(key) {
    try {
        const raw = window.sessionStorage?.getItem(STORAGE_PREFIX + key);
        if (!raw) return null;
        const n = Number(raw);
        return Number.isFinite(n) ? n : null;
    } catch {
        return null;
    }
}

function writeScroll(key, value) {
    try {
        window.sessionStorage?.setItem(STORAGE_PREFIX + key, String(Math.max(0, Math.round(value))));
    } catch {
        // ignore
    }
}

export function useScrollMemory(key) {
    const prevKeyRef = useRef(key);

    useEffect(() => {
        if (typeof window === 'undefined') return undefined;
        const prev = prevKeyRef.current;
        if (prev && prev !== key) {
            writeScroll(prev, window.scrollY || window.pageYOffset || 0);
        }
        const restore = readScroll(key);
        if (restore !== null) {
            const id = window.requestAnimationFrame(() => {
                window.scrollTo({ top: restore, behavior: 'auto' });
            });
            prevKeyRef.current = key;
            return () => window.cancelAnimationFrame(id);
        }
        window.scrollTo({ top: 0, behavior: 'auto' });
        prevKeyRef.current = key;
        return undefined;
    }, [key]);

    useEffect(() => {
        if (typeof window === 'undefined') return undefined;
        const handler = () => {
            writeScroll(prevKeyRef.current, window.scrollY || window.pageYOffset || 0);
        };
        window.addEventListener('beforeunload', handler);
        window.addEventListener('pagehide', handler);
        return () => {
            window.removeEventListener('beforeunload', handler);
            window.removeEventListener('pagehide', handler);
        };
    }, []);
}

export default useScrollMemory;
