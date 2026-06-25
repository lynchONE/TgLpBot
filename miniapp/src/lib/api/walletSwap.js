// Wallet, swap, trade history and liquidity distribution APIs.

function normalizeApiBaseUrl(apiBaseUrl) {
    return String(apiBaseUrl || '').replace(/\/$/, '');
}

async function requestJson(url, options) {
    const resp = await fetch(url, options);
    if (!resp.ok) {
        const text = await resp.text().catch(() => '');
        throw new Error(text || `HTTP ${resp.status}`);
    }
    return resp.json();
}

function postJson(url, payload, signal) {
    return requestJson(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
        signal,
    });
}

export async function fetchTradeHistory({ apiBaseUrl, initData, chain, status, limit, offset, signal }) {
    const base = normalizeApiBaseUrl(apiBaseUrl);
    const url = base + '/api/positions?endpoint=trade_history';
    const payload = { initData };
    if (chain) payload.chain = String(chain);
    if (status) payload.status = String(status);
    if (Number.isFinite(limit)) payload.limit = limit;
    if (Number.isFinite(offset)) payload.offset = offset;
    return postJson(url, payload, signal);
}

export async function walletSwapPreview({ apiBaseUrl, initData, chain, walletId, minValueUsd, signal }) {
    const base = normalizeApiBaseUrl(apiBaseUrl);
    const url = base + '/api/settings?endpoint=wallet_swap_preview';
    const payload = { initData };
    if (chain) payload.chain = String(chain);
    if (walletId) payload.wallet_id = Number(walletId);
    if (Number.isFinite(minValueUsd)) payload.min_value_usd = minValueUsd;
    return postJson(url, payload, signal);
}

export async function walletSwapExecute({ apiBaseUrl, initData, chain, slippagePercent, signal }) {
    const base = normalizeApiBaseUrl(apiBaseUrl);
    const url = base + '/api/settings?endpoint=wallet_swap_execute';
    const payload = { initData };
    if (chain) payload.chain = String(chain);
    if (Number.isFinite(slippagePercent)) payload.slippage_percent = slippagePercent;
    return postJson(url, payload, signal);
}

// --- Wallet CRUD ---
export async function walletCRUD({ apiBaseUrl, initData, action, privateKey, name, walletId, signal }) {
    const base = normalizeApiBaseUrl(apiBaseUrl);
    const url = base + '/api/settings?endpoint=wallet_crud';
    const payload = { initData, action };
    if (privateKey) payload.private_key = privateKey;
    if (name) payload.name = name;
    if (walletId) payload.wallet_id = Number(walletId);
    return postJson(url, payload, signal);
}

// --- Single Token Swap ---
export async function walletSwapSingleQuote({ apiBaseUrl, initData, chain, walletId, fromToken, toToken, amount, slippagePercent, signal }) {
    const base = normalizeApiBaseUrl(apiBaseUrl);
    const url = base + '/api/settings?endpoint=wallet_swap_single';
    const payload = { initData, action: 'quote', from_token: fromToken, to_token: toToken, amount };
    if (chain) payload.chain = String(chain);
    if (walletId) payload.wallet_id = Number(walletId);
    if (Number.isFinite(slippagePercent)) payload.slippage_percent = slippagePercent;
    return postJson(url, payload, signal);
}

export async function walletSwapSingleExecute({ apiBaseUrl, initData, chain, walletId, fromToken, toToken, amount, slippagePercent, provider, quoteId, signal }) {
    const base = normalizeApiBaseUrl(apiBaseUrl);
    const url = base + '/api/settings?endpoint=wallet_swap_single';
    const payload = { initData, action: 'swap', from_token: fromToken, to_token: toToken, amount };
    if (chain) payload.chain = String(chain);
    if (walletId) payload.wallet_id = Number(walletId);
    if (Number.isFinite(slippagePercent)) payload.slippage_percent = slippagePercent;
    if (provider) payload.provider = String(provider);
    if (quoteId) payload.quote_id = String(quoteId);
    return postJson(url, payload, signal);
}

export async function fetchWalletSwapHistory({ apiBaseUrl, initData, chain, walletId, limit, offset, signal }) {
    const base = normalizeApiBaseUrl(apiBaseUrl);
    const url = base + '/api/settings?endpoint=wallet_swap_history';
    const payload = { initData };
    if (chain) payload.chain = String(chain);
    if (walletId) payload.wallet_id = Number(walletId);
    if (Number.isFinite(limit)) payload.limit = Number(limit);
    if (Number.isFinite(offset)) payload.offset = Number(offset);
    return postJson(url, payload, signal);
}

export async function fetchWalletSwapTokenMetadata({ apiBaseUrl, initData, chain, addresses, signal }) {
    const base = normalizeApiBaseUrl(apiBaseUrl);
    const url = base + '/api/settings?endpoint=wallet_swap_token_metadata';
    const payload = { initData, addresses: Array.isArray(addresses) ? addresses : [] };
    if (chain) payload.chain = String(chain);
    return postJson(url, payload, signal);
}

export async function createWalletSwapLimitOrder({
    apiBaseUrl,
    initData,
    chain,
    walletId,
    fromToken,
    toToken,
    amount,
    targetToAmount,
    targetPrice,
    slippagePercent,
    provider,
    signal,
}) {
    const base = normalizeApiBaseUrl(apiBaseUrl);
    const url = base + '/api/settings?endpoint=wallet_swap_limit_order';
    const payload = {
        initData,
        action: 'create',
        from_token: fromToken,
        to_token: toToken,
        amount,
    };
    if (chain) payload.chain = String(chain);
    if (walletId) payload.wallet_id = Number(walletId);
    if (targetToAmount !== undefined && targetToAmount !== null && String(targetToAmount).trim()) {
        payload.target_to_amount = String(targetToAmount).trim();
    }
    if (targetPrice !== undefined && targetPrice !== null && String(targetPrice).trim()) {
        payload.target_price = String(targetPrice).trim();
    }
    if (Number.isFinite(slippagePercent)) payload.slippage_percent = slippagePercent;
    if (provider) payload.provider = String(provider);
    return postJson(url, payload, signal);
}

export async function fetchWalletSwapLimitOrders({ apiBaseUrl, initData, chain, walletId, limit, offset, signal }) {
    const base = normalizeApiBaseUrl(apiBaseUrl);
    const url = base + '/api/settings?endpoint=wallet_swap_limit_order';
    const payload = { initData, action: 'list' };
    if (chain) payload.chain = String(chain);
    if (walletId) payload.wallet_id = Number(walletId);
    if (Number.isFinite(limit)) payload.limit = Number(limit);
    if (Number.isFinite(offset)) payload.offset = Number(offset);
    return postJson(url, payload, signal);
}

export async function cancelWalletSwapLimitOrder({ apiBaseUrl, initData, chain, orderId, signal }) {
    const base = normalizeApiBaseUrl(apiBaseUrl);
    const url = base + '/api/settings?endpoint=wallet_swap_limit_order';
    const payload = { initData, action: 'cancel', order_id: Number(orderId) };
    if (chain) payload.chain = String(chain);
    return postJson(url, payload, signal);
}

export async function fetchPoolLiquidityDistribution({ apiBaseUrl, initData, chain, protocol, address, radius, signal }) {
    const base = normalizeApiBaseUrl(apiBaseUrl);
    const params = new URLSearchParams();
    if (initData) params.set('initData', String(initData));
    if (chain) params.set('chain', String(chain));
    if (protocol) params.set('protocol', String(protocol));
    if (address) params.set('address', String(address));
    if (Number.isFinite(radius)) params.set('radius', String(radius));
    const url = `${base}/api/pool_liquidity_distribution?${params.toString()}`;
    return requestJson(url, { method: 'GET', signal });
}
