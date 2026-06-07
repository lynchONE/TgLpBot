// Wallet, swap, trade history and liquidity distribution APIs.

export async function fetchTradeHistory({ apiBaseUrl, initData, chain, status, limit, offset, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const url = base + '/api/positions?endpoint=trade_history';
    const payload = { initData };
    if (chain) payload.chain = String(chain);
    if (status) payload.status = String(status);
    if (Number.isFinite(limit)) payload.limit = limit;
    if (Number.isFinite(offset)) payload.offset = offset;
    const resp = await fetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
        signal,
    });
    if (!resp.ok) {
        const text = await resp.text().catch(() => '');
        throw new Error(text || `HTTP ${resp.status}`);
    }
    return resp.json();
}

export async function walletSwapPreview({ apiBaseUrl, initData, chain, walletId, minValueUsd, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const url = base + '/api/settings?endpoint=wallet_swap_preview';
    const payload = { initData };
    if (chain) payload.chain = String(chain);
    if (walletId) payload.wallet_id = Number(walletId);
    if (Number.isFinite(minValueUsd)) payload.min_value_usd = minValueUsd;
    const resp = await fetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
        signal,
    });
    if (!resp.ok) {
        const text = await resp.text().catch(() => '');
        throw new Error(text || `HTTP ${resp.status}`);
    }
    return resp.json();
}

export async function walletSwapExecute({ apiBaseUrl, initData, chain, slippagePercent, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const url = base + '/api/settings?endpoint=wallet_swap_execute';
    const payload = { initData };
    if (chain) payload.chain = String(chain);
    if (Number.isFinite(slippagePercent)) payload.slippage_percent = slippagePercent;
    const resp = await fetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
        signal,
    });
    if (!resp.ok) {
        const text = await resp.text().catch(() => '');
        throw new Error(text || `HTTP ${resp.status}`);
    }
    return resp.json();
}

// --- Wallet CRUD ---
export async function walletCRUD({ apiBaseUrl, initData, action, privateKey, name, walletId, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const url = base + '/api/settings?endpoint=wallet_crud';
    const payload = { initData, action };
    if (privateKey) payload.private_key = privateKey;
    if (name) payload.name = name;
    if (walletId) payload.wallet_id = Number(walletId);
    const resp = await fetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
        signal,
    });
    if (!resp.ok) {
        const text = await resp.text().catch(() => '');
        throw new Error(text || `HTTP ${resp.status}`);
    }
    return resp.json();
}

// --- Single Token Swap ---
export async function walletSwapSingleQuote({ apiBaseUrl, initData, chain, walletId, fromToken, toToken, amount, slippagePercent, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const url = base + '/api/settings?endpoint=wallet_swap_single';
    const payload = { initData, action: 'quote', from_token: fromToken, to_token: toToken, amount };
    if (chain) payload.chain = String(chain);
    if (walletId) payload.wallet_id = Number(walletId);
    if (Number.isFinite(slippagePercent)) payload.slippage_percent = slippagePercent;
    const resp = await fetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
        signal,
    });
    if (!resp.ok) {
        const text = await resp.text().catch(() => '');
        throw new Error(text || `HTTP ${resp.status}`);
    }
    return resp.json();
}

export async function walletSwapSingleExecute({ apiBaseUrl, initData, chain, walletId, fromToken, toToken, amount, slippagePercent, provider, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const url = base + '/api/settings?endpoint=wallet_swap_single';
    const payload = { initData, action: 'swap', from_token: fromToken, to_token: toToken, amount };
    if (chain) payload.chain = String(chain);
    if (walletId) payload.wallet_id = Number(walletId);
    if (Number.isFinite(slippagePercent)) payload.slippage_percent = slippagePercent;
    if (provider) payload.provider = String(provider);
    const resp = await fetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
        signal,
    });
    if (!resp.ok) {
        const text = await resp.text().catch(() => '');
        throw new Error(text || `HTTP ${resp.status}`);
    }
    return resp.json();
}

export async function fetchWalletSwapHistory({ apiBaseUrl, initData, chain, walletId, limit, offset, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const url = base + '/api/settings?endpoint=wallet_swap_history';
    const payload = { initData };
    if (chain) payload.chain = String(chain);
    if (walletId) payload.wallet_id = Number(walletId);
    if (Number.isFinite(limit)) payload.limit = Number(limit);
    if (Number.isFinite(offset)) payload.offset = Number(offset);
    const resp = await fetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
        signal,
    });
    if (!resp.ok) {
        const text = await resp.text().catch(() => '');
        throw new Error(text || `HTTP ${resp.status}`);
    }
    return resp.json();
}

export async function fetchWalletSwapTokenMetadata({ apiBaseUrl, initData, chain, addresses, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const url = base + '/api/settings?endpoint=wallet_swap_token_metadata';
    const payload = { initData, addresses: Array.isArray(addresses) ? addresses : [] };
    if (chain) payload.chain = String(chain);
    const resp = await fetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
        signal,
    });
    if (!resp.ok) {
        const text = await resp.text().catch(() => '');
        throw new Error(text || `HTTP ${resp.status}`);
    }
    return resp.json();
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
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
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
    const resp = await fetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
        signal,
    });
    if (!resp.ok) {
        const text = await resp.text().catch(() => '');
        throw new Error(text || `HTTP ${resp.status}`);
    }
    return resp.json();
}

export async function fetchWalletSwapLimitOrders({ apiBaseUrl, initData, chain, walletId, limit, offset, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const url = base + '/api/settings?endpoint=wallet_swap_limit_order';
    const payload = { initData, action: 'list' };
    if (chain) payload.chain = String(chain);
    if (walletId) payload.wallet_id = Number(walletId);
    if (Number.isFinite(limit)) payload.limit = Number(limit);
    if (Number.isFinite(offset)) payload.offset = Number(offset);
    const resp = await fetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
        signal,
    });
    if (!resp.ok) {
        const text = await resp.text().catch(() => '');
        throw new Error(text || `HTTP ${resp.status}`);
    }
    return resp.json();
}

export async function cancelWalletSwapLimitOrder({ apiBaseUrl, initData, chain, orderId, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const url = base + '/api/settings?endpoint=wallet_swap_limit_order';
    const payload = { initData, action: 'cancel', order_id: Number(orderId) };
    if (chain) payload.chain = String(chain);
    const resp = await fetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
        signal,
    });
    if (!resp.ok) {
        const text = await resp.text().catch(() => '');
        throw new Error(text || `HTTP ${resp.status}`);
    }
    return resp.json();
}

export async function fetchPoolLiquidityDistribution({ apiBaseUrl, initData, chain, protocol, address, radius, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const params = new URLSearchParams();
    if (initData) params.set('initData', String(initData));
    if (chain) params.set('chain', String(chain));
    if (protocol) params.set('protocol', String(protocol));
    if (address) params.set('address', String(address));
    if (Number.isFinite(radius)) params.set('radius', String(radius));
    const url = `${base}/api/pool_liquidity_distribution?${params.toString()}`;
    const resp = await fetch(url, { method: 'GET', signal });
    if (!resp.ok) {
        const text = await resp.text().catch(() => '');
        throw new Error(text || `HTTP ${resp.status}`);
    }
    return resp.json();
}
