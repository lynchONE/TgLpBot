export const SMART_MONEY_POOL_FILTER_STORAGE_KEY = 'tglp_smart_money_pool_filter_v1';
export const EMPTY_SMART_MONEY_POOL_FILTER = { minSmartMoneyUsd: null, maxFeeRate: null };

export const SMART_MONEY_POOL_SOURCE_TABS = [
    { key: 'all', label: '鍏ㄩ儴', source: '' },
    { key: 'manual', label: '鎵嬪姩娣诲姞', source: 'manual' },
    { key: 'contract', label: '鍚堢害鍙戠幇', source: 'contract_interaction' },
];

export const SMART_MONEY_POOL_SOURCE_BY_KEY = Object.fromEntries(
    SMART_MONEY_POOL_SOURCE_TABS.map((item) => [item.key, item.source]),
);

export function parseOptionalNumber(value) {
    const text = String(value ?? '').replace(/,/g, '').trim();
    if (!text) return null;
    const match = text.match(/-?\d+(\.\d+)?/);
    if (!match) return null;
    const num = Number(match[0]);
    if (!Number.isFinite(num)) return null;
    return Math.max(0, num);
}

export function formatOptionalNumber(value) {
    return Number.isFinite(value) ? String(value) : '';
}

export function normalizeStoredSmartMoneyPoolFilter(value) {
    if (!value || typeof value !== 'object') {
        return { ...EMPTY_SMART_MONEY_POOL_FILTER };
    }
    return {
        minSmartMoneyUsd: Number.isFinite(Number(value.minSmartMoneyUsd)) ? Number(value.minSmartMoneyUsd) : null,
        maxFeeRate: Number.isFinite(Number(value.maxFeeRate)) ? Number(value.maxFeeRate) : null,
    };
}

export function readStoredSmartMoneyPoolFilter() {
    if (typeof window === 'undefined' || !window.localStorage) {
        return { ...EMPTY_SMART_MONEY_POOL_FILTER };
    }
    try {
        const raw = window.localStorage.getItem(SMART_MONEY_POOL_FILTER_STORAGE_KEY);
        if (!raw) return { ...EMPTY_SMART_MONEY_POOL_FILTER };
        return normalizeStoredSmartMoneyPoolFilter(JSON.parse(raw));
    } catch {
        return { ...EMPTY_SMART_MONEY_POOL_FILTER };
    }
}

export function writeStoredSmartMoneyPoolFilter(value) {
    if (typeof window === 'undefined' || !window.localStorage) {
        return;
    }
    try {
        const normalized = normalizeStoredSmartMoneyPoolFilter(value);
        const isEmpty = !Number.isFinite(normalized.minSmartMoneyUsd) && !Number.isFinite(normalized.maxFeeRate);
        if (isEmpty) {
            window.localStorage.removeItem(SMART_MONEY_POOL_FILTER_STORAGE_KEY);
            return;
        }
        window.localStorage.setItem(SMART_MONEY_POOL_FILTER_STORAGE_KEY, JSON.stringify(normalized));
    } catch {
        // ignore storage failures
    }
}
