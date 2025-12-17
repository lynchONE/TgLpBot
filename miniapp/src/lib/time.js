export function formatRelativeTime(isoString) {
    if (!isoString) return '';
    const ts = Date.parse(isoString);
    if (!Number.isFinite(ts)) return '';

    const diffMs = Date.now() - ts;
    const diffSec = Math.max(0, Math.floor(diffMs / 1000));
    if (diffSec < 60) return `${diffSec}秒前`;
    const diffMin = Math.floor(diffSec / 60);
    if (diffMin < 60) return `${diffMin}分钟前`;
    const diffH = Math.floor(diffMin / 60);
    if (diffH < 24) return `${diffH}小时前`;
    const diffD = Math.floor(diffH / 24);
    return `${diffD}天前`;
}

export function formatDurationFrom(isoString) {
    if (!isoString) return '';
    const ts = Date.parse(isoString);
    if (!Number.isFinite(ts)) return '';
    const diffSec = Math.max(0, Math.floor((Date.now() - ts) / 1000));
    const min = Math.floor(diffSec / 60);
    if (min < 60) return `${min}分钟`;
    const h = Math.floor(min / 60);
    const remMin = min % 60;
    if (h < 24) return remMin ? `${h}小时${remMin}分` : `${h}小时`;
    const d = Math.floor(h / 24);
    const remH = h % 24;
    return remH ? `${d}天${remH}小时` : `${d}天`;
}

