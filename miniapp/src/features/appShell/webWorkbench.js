export const STORAGE_WEB_WORKBENCH_WIDGETS = 'tglp_web_workbench_widgets_v1';

export const WEB_WORKBENCH_WIDGETS = [
    { key: 'hot_pools', label: '热门池' },
    { key: 'gmgn_kline', label: 'K线' },
    { key: 'positions', label: '仓位' },
];

export const DEFAULT_WEB_WORKBENCH_WIDGETS = WEB_WORKBENCH_WIDGETS.map((item) => item.key);

export function normalizeWebWorkbenchWidgets(value) {
    if (!Array.isArray(value)) return [...DEFAULT_WEB_WORKBENCH_WIDGETS];
    const allow = new Set(DEFAULT_WEB_WORKBENCH_WIDGETS);
    const seen = new Set();
    const next = [];
    for (const raw of value) {
        const key = String(raw || '').trim();
        if (!allow.has(key) || seen.has(key)) continue;
        seen.add(key);
        next.push(key);
    }
    if (next.length === 0) return [...DEFAULT_WEB_WORKBENCH_WIDGETS];
    return next;
}
