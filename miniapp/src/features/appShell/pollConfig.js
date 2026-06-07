export const STORAGE_POLL_SEC = 'tglp_poll_interval_sec';
export const STORAGE_MODULE_POLL_SECS = 'tglp_module_poll_interval_secs_v1';
export const MIN_POLL_INTERVAL_SEC = 2;
export const MAX_POLL_INTERVAL_SEC = 300;
export const POSITIONS_ACTIVE_POLL_KEY = 'positions_active';
export const POSITIONS_IDLE_POLL_KEY = 'positions_idle';
export const LEGACY_POSITIONS_POLL_KEY = 'positions';

export const MODULE_POLL_CONFIG = [
    { key: POSITIONS_ACTIVE_POLL_KEY, label: '仓位(有仓位)', defaultSec: 10, minSec: 2 },
    { key: POSITIONS_IDLE_POLL_KEY, label: '仓位(无仓位)', defaultSec: 30, minSec: 5 },
    { key: 'hot_pools', label: '热门池', defaultSec: 10, minSec: 2 },
    { key: 'assets', label: '我的资产', defaultSec: 60, minSec: 60 },
    { key: 'smart_money', label: '聪明钱', defaultSec: 15, minSec: 2 },
    { key: 'admin_page', label: '管理页', defaultSec: 15, minSec: 5 },
    { key: 'admin', label: '管理工作台', defaultSec: 10, minSec: 3 },
    { key: 'swap', label: '兑换', defaultSec: 8, minSec: 5 },
];

export function getModulePollConfig(key) {
    const config = MODULE_POLL_CONFIG.find((item) => item.key === key);
    if (!config) {
        throw new Error(`Unknown poll module: ${key}`);
    }
    return config;
}

export function clampModulePollSec(value, config) {
    if (!config || !Number.isFinite(Number(config.minSec)) || !Number.isFinite(Number(config.defaultSec))) {
        throw new Error('Invalid poll module config');
    }
    const n = Number(value);
    const minSec = Math.max(MIN_POLL_INTERVAL_SEC, Number(config.minSec));
    const defaultSec = Math.max(minSec, Number(config.defaultSec));
    if (!Number.isFinite(n)) return defaultSec;
    return Math.max(minSec, Math.min(MAX_POLL_INTERVAL_SEC, Math.floor(n)));
}

export function normalizeModulePollOverrides(raw, legacyValue) {
    let parsed = null;
    if (raw) {
        try {
            parsed = JSON.parse(raw);
        } catch {
            parsed = null;
        }
    }
    const out = {};
    const legacyPositionsValue = parsed && Object.prototype.hasOwnProperty.call(parsed, LEGACY_POSITIONS_POLL_KEY)
        ? parsed[LEGACY_POSITIONS_POLL_KEY]
        : null;
    MODULE_POLL_CONFIG.forEach((item) => {
        if (parsed && Object.prototype.hasOwnProperty.call(parsed, item.key)) {
            out[item.key] = clampModulePollSec(parsed[item.key], item);
        } else if (item.key === POSITIONS_ACTIVE_POLL_KEY && legacyPositionsValue !== null) {
            out[item.key] = clampModulePollSec(legacyPositionsValue, item);
        }
    });
    if (Object.keys(out).length > 0) return out;

    const legacy = Number(legacyValue);
    if (Number.isFinite(legacy) && legacy >= MIN_POLL_INTERVAL_SEC) {
        MODULE_POLL_CONFIG.forEach((item) => {
            if (item.key === POSITIONS_IDLE_POLL_KEY) {
                return;
            }
            out[item.key] = clampModulePollSec(legacy, item);
        });
    }
    return out;
}

export function getModulePollSec(key, defaultSec, overrides) {
    const config = getModulePollConfig(key);
    if (overrides && Object.prototype.hasOwnProperty.call(overrides, key)) {
        return clampModulePollSec(overrides[key], config);
    }
    return clampModulePollSec(defaultSec, config);
}
