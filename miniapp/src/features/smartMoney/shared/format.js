import { formatUSDCompact } from '../../../lib/format';

export function formatRangePercent(value) {
    const num = Number(value);
    if (!Number.isFinite(num) || num <= 0) return '\u2014';
    if (num >= 100) return `\u00b1${Math.round(num)}%`;
    if (num >= 10) return `\u00b1${num.toFixed(1).replace(/\.0$/, '')}%`;
    return `\u00b1${num.toFixed(2).replace(/0+$/, '').replace(/\.$/, '')}%`;
}

export function formatRangePercentPlain(value) {
    const num = Number(value);
    if (!Number.isFinite(num) || num <= 0) return '--';
    if (num >= 100) return `${Math.round(num)}%`;
    if (num >= 10) return `${num.toFixed(1).replace(/\.0$/, '')}%`;
    return `${num.toFixed(2).replace(/0+$/, '').replace(/\.$/, '')}%`;
}

export function getPositionSelectionKey(position) {
    const positionRef = String(position?.position_ref || '').trim();
    if (positionRef) return positionRef;
    const id = String(position?.id || '').trim();
    if (id) return id;
    const wallet = String(position?.wallet_address || '').trim().toLowerCase();
    const pool = String(position?.pool_address || '').trim().toLowerCase();
    const nft = String(position?.nft_token_id || '').trim();
    return [wallet, pool, nft].filter(Boolean).join(':');
}

export const USD_PREVIEW_FORMATTER = new Intl.NumberFormat('en-US', {
    style: 'currency',
    currency: 'USD',
    maximumFractionDigits: 2,
});

export function formatPreviewUsd(value) {
    const num = Number(value);
    if (!Number.isFinite(num)) return '--';
    return USD_PREVIEW_FORMATTER.format(num);
}

export function formatHeatmapUSD(value) {
    const num = Number(value);
    if (!Number.isFinite(num)) return '--';
    if (Math.abs(num) < 0.005) return '$0';
    return formatUSDCompact(num);
}

export function formatHeatmapRate(value) {
    const num = Number(value);
    if (!Number.isFinite(num)) return '--';
    if (num >= 10) return `$${num.toFixed(1).replace(/\.0$/, '')}`;
    if (num >= 1) return `$${num.toFixed(2).replace(/0+$/, '').replace(/\.$/, '')}`;
    return `$${num.toFixed(4).replace(/0+$/, '').replace(/\.$/, '')}`;
}

export function heatmapWindowLabel(windows, value) {
    return windows.find((item) => item.key === value)?.label || '1min';
}

export function formatHeatmapAge(seconds) {
    const value = Number(seconds);
    if (!Number.isFinite(value) || value <= 0) return '--';
    if (value < 60) return `${Math.max(1, Math.round(value))}\u79d2`;
    if (value < 3600) return `${Math.round(value / 60)}\u5206\u949f`;
    if (value < 86400) return `${Math.round(value / 3600)}\u5c0f\u65f6`;
    return `${Math.round(value / 86400)}\u5929`;
}

export function formatWatchActivityAction(value) {
    const eventType = String(value || '').trim();
    if (eventType === 'add') return '\u52a0 LP';
    if (eventType === 'remove') return '\u64a4 LP';
    return eventType || 'LP \u64cd\u4f5c';
}

export function getWatchActivityActionClass(value) {
    return String(value || '').trim() === 'remove'
        ? 'border-red-500/20 bg-red-500/10 text-red-300'
        : 'border-emerald-500/20 bg-emerald-500/10 text-emerald-300';
}

export function heatmapSampleText(row) {
    const status = String(row?.sample_status || '').trim();
    if (status === 'ok') return '\u6837\u672c\u5b8c\u6574';
    if (status === 'partial') return '\u90e8\u5206\u6837\u672c';
    return '\u6837\u672c\u4e0d\u8db3';
}
