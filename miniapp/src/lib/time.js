import { useState, useEffect } from 'react';

// 计算相对时间字符串
export function formatRelativeTime(isoString, now = Date.now()) {
    if (!isoString) return '';
    const ts = Date.parse(isoString);
    if (!Number.isFinite(ts)) return '';

    const diffMs = now - ts;
    const diffSec = Math.max(0, Math.floor(diffMs / 1000));
    if (diffSec < 60) return `${diffSec}秒前`;
    const diffMin = Math.floor(diffSec / 60);
    if (diffMin < 60) return `${diffMin}分钟前`;
    const diffH = Math.floor(diffMin / 60);
    if (diffH < 24) return `${diffH}小时前`;
    const diffD = Math.floor(diffH / 24);
    return `${diffD}天前`;
}

export function formatDurationFrom(isoString, now = Date.now()) {
    if (!isoString) return '';
    const ts = Date.parse(isoString);
    if (!Number.isFinite(ts)) return '';
    const diffSec = Math.max(0, Math.floor((now - ts) / 1000));
    const min = Math.floor(diffSec / 60);
    if (min < 60) return `${min}分钟`;
    const h = Math.floor(min / 60);
    const remMin = min % 60;
    if (h < 24) return remMin ? `${h}小时${remMin}分` : `${h}小时`;
    const d = Math.floor(h / 24);
    const remH = h % 24;
    return remH ? `${d}天${remH}小时` : `${d}天`;
}

// 全局时钟 Hook - 每秒更新一次，所有使用此Hook的组件会同步更新
let globalTick = Date.now();
let listeners = new Set();
let tickInterval = null;

function startTicking() {
    if (tickInterval) return;
    tickInterval = setInterval(() => {
        globalTick = Date.now();
        listeners.forEach(fn => fn(globalTick));
    }, 1000);
}

function stopTicking() {
    if (tickInterval && listeners.size === 0) {
        clearInterval(tickInterval);
        tickInterval = null;
    }
}

// 使用全局时钟的Hook
export function useTick() {
    const [tick, setTick] = useState(globalTick);

    useEffect(() => {
        const handler = (t) => setTick(t);
        listeners.add(handler);
        startTicking();
        return () => {
            listeners.delete(handler);
            stopTicking();
        };
    }, []);

    return tick;
}

// 实时更新的相对时间 Hook
export function useRelativeTime(isoString) {
    const tick = useTick();
    return formatRelativeTime(isoString, tick);
}

// 实时更新的运行时长 Hook
export function useDurationFrom(isoString) {
    const tick = useTick();
    return formatDurationFrom(isoString, tick);
}
