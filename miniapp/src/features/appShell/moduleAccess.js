export const VIEW_MODULE_MAP = {
    hot_pools: 'hot_pools',
    positions: 'positions',
    assets: 'assets',
    smart_money: 'smart_money',
    swap: 'swap',
    admin_page: 'admin_panel',
    admin: 'admin_panel',
};

export function normalizeEnabledModules(value) {
    if (!Array.isArray(value)) return [];
    const seen = new Set();
    const keys = [];
    value.forEach((item) => {
        const key = String(item || '').trim();
        if (!key || seen.has(key)) return;
        seen.add(key);
        keys.push(key);
    });
    return keys;
}

export function hasModuleAccess(me, moduleKey) {
    const key = String(moduleKey || '').trim();
    if (!key) return false;
    if (Boolean(me?.is_admin)) return true;
    if (!Boolean(me?.mini_app_enabled)) return false;
    return normalizeEnabledModules(me?.enabled_modules).includes(key);
}

export function canAccessView(me, viewMode) {
    const moduleKey = VIEW_MODULE_MAP[String(viewMode || '').trim()];
    return moduleKey ? hasModuleAccess(me, moduleKey) : false;
}

export function buildTopNavItems({ me, isAdmin }) {
    const baseItems = [
        { key: 'hot_pools', label: '热门池' },
        { key: 'positions', label: '仓位' },
        { key: 'assets', label: '我的' },
        { key: 'smart_money', label: '聪明钱' },
        { key: 'swap', label: '兑换' },
    ];
    const items = me ? baseItems.filter((item) => canAccessView(me, item.key)) : [...baseItems];
    if (isAdmin) {
        items.push({ key: 'admin_page', label: '管理页' });
    }
    return items;
}
