export function normalizePoolKey(value) {
    const raw = String(value || '').trim();
    if (!raw) return '';
    const body = raw.startsWith('0x') || raw.startsWith('0X') ? raw.slice(2) : raw;
    if (!/^[a-fA-F0-9]{40}$/.test(body) && !/^[a-fA-F0-9]{64}$/.test(body)) {
        return '';
    }
    return `0x${body.toLowerCase()}`;
}

export function resolveOpenPositionPoolChain(pool, fallbackChain = 'bsc') {
    const explicitChain = String(pool?.chain || '').trim().toLowerCase();
    if (explicitChain) return explicitChain;
    if (Number(pool?.chain_id) === 8453) return 'base';
    return String(fallbackChain || 'bsc').trim().toLowerCase() || 'bsc';
}

export function normalizeOpenPositionPoolVersion(pool) {
    const directCandidates = [
        pool?.protocol_version,
        pool?.pool_version,
        pool?.protocol,
        pool?.factory_name,
        pool?.dex,
    ];
    for (const candidate of directCandidates) {
        const raw = String(candidate || '').trim().toLowerCase();
        if (!raw) continue;
        const matched = raw.match(/v?\d+/)?.[0] ?? '';
        if (matched) return matched.startsWith('v') ? matched : `v${matched}`;
    }
    const aliasCandidates = [pool?.protocol, pool?.factory_name, pool?.dex];
    for (const candidate of aliasCandidates) {
        const raw = String(candidate || '').trim().toLowerCase();
        if (!raw) continue;
        if (raw.includes('v4')) return 'v4';
        if (raw.includes('v3') || raw.includes('pancake') || raw.includes('aerodrome') || raw.includes('slipstream')) return 'v3';
    }
    return '';
}

export function normalizePositionSmartMoneyGroups(groups) {
    return Array.isArray(groups)
        ? groups.filter((item) => Number(item?.range_percent) > 0)
        : [];
}

export function buildEntrySwapConfirmKey(preview, entrySwapSlippage) {
    return [
        preview?.required ? '1' : '0',
        preview?.from_token_address || '',
        preview?.to_token_address || '',
        preview?.amount_in_raw || '',
        preview?.expected_amount_out_raw || '',
        String(entrySwapSlippage || '').trim(),
    ].join('|');
}

export function getOutOfRangeActionSummary(rebalanceEnabled) {
    return {
        above: rebalanceEnabled ? '自动再平衡' : '自动撤仓并结束',
        below: rebalanceEnabled ? '自动再平衡' : '自动撤仓并结束',
    };
}

export function resolveOpenPositionErrorPayload(error) {
    if (!error || typeof error !== 'object') return null;
    if (error.payload && typeof error.payload === 'object') return error.payload;
    return error;
}

export function isOpenPositionSafetyError(error) {
    const payload = resolveOpenPositionErrorPayload(error);
    if (!payload) return false;
    const code = String(payload?.code || '').trim();
    return Boolean(
        code === 'zap_safety_check_failed' ||
        code === 'token_honeypot' ||
        code.startsWith('pool_') ||
        typeof payload?.liquidity_usd === 'number' ||
        typeof payload?.max_open_amount === 'number' ||
        typeof payload?.price_deviation_percent === 'number' ||
        Boolean(payload?.token_risk) ||
        Boolean(payload?.risk_ack_required)
    );
}

export function extractOpenPositionErrorChecks(error, fallbackKey = 'submit_safety') {
    const payload = resolveOpenPositionErrorPayload(error);
    if (Array.isArray(payload?.checks) && payload.checks.length > 0) {
        return payload.checks;
    }
    if (!isOpenPositionSafetyError(payload)) {
        return [];
    }
    const detail = String(error?.message || payload?.message || '').trim() || '校验失败，请稍后重试。';
    return [{
        key: fallbackKey,
        status: 'fail',
        label: '安全校验',
        detail,
    }];
}
