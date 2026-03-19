// Smart Money API functions for webapp
const SM_BASE = '/api/sm';

function normalizeBase(apiBaseUrl) {
    return String(apiBaseUrl || '').trim().replace(/\/$/, '');
}

async function smRequest(url, options = {}) {
    const resp = await fetch(url, options);
    if (!resp.ok) {
        const text = await resp.text().catch(() => '');
        throw new Error(text || `HTTP ${resp.status}`);
    }
    const json = await resp.json();
    if (json.code !== 0 && json.code !== undefined) {
        throw new Error(json.message || 'unknown error');
    }
    return json.data;
}

async function goldenDogRequest(url, options = {}) {
    const resp = await fetch(url, options);
    if (!resp.ok) {
        const text = await resp.text().catch(() => '');
        throw new Error(text || `HTTP ${resp.status}`);
    }
    const json = await resp.json();
    if (!json?.ok) {
        throw new Error(json?.message || 'unknown error');
    }
    return json;
}

export async function fetchSMWallets({ apiBaseUrl, page = 1, size = 20, keyword, source, active, signal }) {
    const base = normalizeBase(apiBaseUrl);
    const params = new URLSearchParams();
    params.set('page', String(page));
    params.set('size', String(size));
    if (keyword) params.set('keyword', keyword);
    if (source) params.set('source', source);
    if (active !== undefined) params.set('active', String(active));
    return smRequest(`${base}${SM_BASE}/wallets?${params}`, { signal });
}

export async function addSMWallet({ apiBaseUrl, address, label, chain, signal }) {
    const base = normalizeBase(apiBaseUrl);
    const params = new URLSearchParams();
    if (chain) params.set('chain', String(chain));
    const qs = params.toString();
    return smRequest(`${base}${SM_BASE}/wallets${qs ? `?${qs}` : ''}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ address, label }),
        signal,
    });
}

export async function updateSMWallet({ apiBaseUrl, address, updates, chain, signal }) {
    const base = normalizeBase(apiBaseUrl);
    const params = new URLSearchParams();
    params.set('address', String(address));
    if (chain) params.set('chain', String(chain));
    return smRequest(`${base}${SM_BASE}/wallets?${params}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(updates),
        signal,
    });
}

export async function deleteSMWallet({ apiBaseUrl, address, chain, signal }) {
    const base = normalizeBase(apiBaseUrl);
    const params = new URLSearchParams();
    params.set('address', String(address));
    if (chain) params.set('chain', String(chain));
    return smRequest(`${base}${SM_BASE}/wallets?${params}`, {
        method: 'DELETE',
        signal,
    });
}

export async function fetchSMContracts({ apiBaseUrl, signal }) {
    const base = normalizeBase(apiBaseUrl);
    return smRequest(`${base}${SM_BASE}/contracts`, { signal });
}

export async function addSMContract({ apiBaseUrl, contract_address, protocol, description, signal }) {
    const base = normalizeBase(apiBaseUrl);
    return smRequest(`${base}${SM_BASE}/contracts`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ contract_address, protocol, description }),
        signal,
    });
}

export async function updateSMContract({ apiBaseUrl, address, updates, signal }) {
    const base = normalizeBase(apiBaseUrl);
    return smRequest(`${base}${SM_BASE}/contracts?address=${encodeURIComponent(address)}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(updates),
        signal,
    });
}

export async function deleteSMContract({ apiBaseUrl, address, signal }) {
    const base = normalizeBase(apiBaseUrl);
    return smRequest(`${base}${SM_BASE}/contracts?address=${encodeURIComponent(address)}`, {
        method: 'DELETE',
        signal,
    });
}

export async function fetchSMPools({ apiBaseUrl, signal }) {
    const base = normalizeBase(apiBaseUrl);
    return smRequest(`${base}${SM_BASE}/pools`, { signal });
}

export async function fetchSMPoolStats({ apiBaseUrl, poolAddress, signal }) {
    const base = normalizeBase(apiBaseUrl);
    return smRequest(`${base}${SM_BASE}/pools?pool=${encodeURIComponent(poolAddress)}`, { signal });
}

export async function fetchSMPositions({ apiBaseUrl, status = 'open', wallet, pool, protocol, page = 1, size = 20, orderBy, signal }) {
    const base = normalizeBase(apiBaseUrl);
    const params = new URLSearchParams();
    params.set('status', status);
    params.set('page', String(page));
    params.set('size', String(size));
    if (wallet) params.set('wallet', wallet);
    if (pool) params.set('pool', pool);
    if (protocol) params.set('protocol', protocol);
    if (orderBy) params.set('order_by', orderBy);
    return smRequest(`${base}${SM_BASE}/positions?${params}`, { signal });
}

export async function fetchSMEvents({ apiBaseUrl, wallet, pool, page = 1, size = 20, signal }) {
    const base = normalizeBase(apiBaseUrl);
    const params = new URLSearchParams();
    params.set('page', String(page));
    params.set('size', String(size));
    if (wallet) params.set('wallet', wallet);
    if (pool) params.set('pool', pool);
    return smRequest(`${base}${SM_BASE}/events?${params}`, { signal });
}

export async function fetchSMStats({ apiBaseUrl, address, signal }) {
    const base = normalizeBase(apiBaseUrl);
    const params = address ? `?address=${encodeURIComponent(address)}` : '';
    return smRequest(`${base}${SM_BASE}/stats${params}`, { signal });
}

export async function fetchSMGoldenDogConfig({ apiBaseUrl, initData, chain = 'bsc', signal }) {
    const base = normalizeBase(apiBaseUrl);
    const params = new URLSearchParams();
    if (initData) params.set('initData', initData);
    if (chain) params.set('chain', chain);
    return goldenDogRequest(`${base}/api/smart_money_golden_dog_config?${params.toString()}`, { signal });
}

export async function saveSMGoldenDogConfig({ apiBaseUrl, initData, chain = 'bsc', config, signal }) {
    const base = normalizeBase(apiBaseUrl);
    return goldenDogRequest(`${base}/api/smart_money_golden_dog_config`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ initData, chain, ...(config || {}) }),
        signal,
    });
}
