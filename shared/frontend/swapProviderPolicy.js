export const SWAP_PROVIDER_POLICY_OPTIONS = [
    {
        value: 'best',
        label: '自动择优',
        shortLabel: '择优',
        description: 'OKX 和 Binance 同时报价，执行预计到账更多的一条。',
    },
    {
        value: 'okx',
        label: '仅 OKX',
        shortLabel: 'OKX',
        description: '本次开仓和后续撤仓只走 OKX。',
    },
    {
        value: 'binance',
        label: '仅 Binance',
        shortLabel: 'Binance',
        description: '本次开仓和后续撤仓只走 Binance。',
    },
];

export function normalizeSwapProviderPolicy(value) {
    const text = String(value || '').trim().toLowerCase();
    if (text === '') return 'best';
    if (text === 'binnace') return 'binance';
    if (SWAP_PROVIDER_POLICY_OPTIONS.some((option) => option.value === text)) return text;
    return 'best';
}

export function getSwapProviderPolicyOption(value) {
    const normalized = normalizeSwapProviderPolicy(value);
    return SWAP_PROVIDER_POLICY_OPTIONS.find((option) => option.value === normalized) || SWAP_PROVIDER_POLICY_OPTIONS[0];
}

export function formatSwapRouteInfo(route) {
    if (!route || typeof route !== 'object') return '';
    const providerName = String(route.provider_name || '').trim();
    const provider = String(route.provider || '').trim();
    const routeSummary = String(route.route_summary || '').trim();
    const vendorName = String(route.vendor_name || '').trim();
    const label = providerName || getSwapProviderPolicyOption(provider).shortLabel || provider;
    return [label, routeSummary || vendorName].filter(Boolean).join(' · ');
}

export function formatSwapRouteList(routes) {
    if (!Array.isArray(routes) || routes.length === 0) return '';
    const labels = routes.map(formatSwapRouteInfo).filter(Boolean);
    return Array.from(new Set(labels)).join(' / ');
}
