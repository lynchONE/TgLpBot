// System and admin infrastructure settings APIs.

export async function fetchSystemConfig({ apiBaseUrl, initData, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const url = `${base}/api/admin?endpoint=system_config`;
    const resp = await fetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ initData }),
        signal,
    });
    if (!resp.ok) {
        const text = await resp.text().catch(() => '');
        throw new Error(text || `HTTP ${resp.status}`);
    }
    return resp.json();
}

export async function updateSystemConfig({ apiBaseUrl, initData, config, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const url = `${base}/api/admin?endpoint=system_config`;
    const payload = { initData, ...config };
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


// RPC Pool (Admin)

async function adminRPCPoolRequest({ apiBaseUrl, payload, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const url = `${base}/api/admin?endpoint=rpc_pool`;
    const resp = await fetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload || {}),
        signal,
    });
    if (!resp.ok) {
        const text = await resp.text().catch(() => '');
        throw new Error(text || `HTTP ${resp.status}`);
    }
    return resp.json();
}

export async function fetchAdminRPCPool({ apiBaseUrl, initData, signal }) {
    return adminRPCPoolRequest({
        apiBaseUrl,
        payload: { initData, action: 'list' },
        signal,
    });
}

export async function addAdminRPCEndpoint({ apiBaseUrl, initData, chain, transport, name, url, setCurrent, signal }) {
    return adminRPCPoolRequest({
        apiBaseUrl,
        payload: {
            initData,
            action: 'add',
            chain,
            transport,
            name,
            url,
            set_current: Boolean(setCurrent),
        },
        signal,
    });
}

export async function renameAdminRPCEndpoint({ apiBaseUrl, initData, endpointId, name, signal }) {
    return adminRPCPoolRequest({
        apiBaseUrl,
        payload: { initData, action: 'rename', endpoint_id: Number(endpointId), name },
        signal,
    });
}

export async function switchAdminRPCEndpoint({ apiBaseUrl, initData, endpointId, signal }) {
    return adminRPCPoolRequest({
        apiBaseUrl,
        payload: { initData, action: 'switch', endpoint_id: Number(endpointId) },
        signal,
    });
}

export async function disableAdminRPCEndpointNextMonth({ apiBaseUrl, initData, endpointId, signal }) {
    return adminRPCPoolRequest({
        apiBaseUrl,
        payload: { initData, action: 'disable', endpoint_id: Number(endpointId), disable_next_month: true },
        signal,
    });
}

export async function enableAdminRPCEndpoint({ apiBaseUrl, initData, endpointId, signal }) {
    return adminRPCPoolRequest({
        apiBaseUrl,
        payload: { initData, action: 'enable', endpoint_id: Number(endpointId) },
        signal,
    });
}

export async function deleteAdminRPCEndpoint({ apiBaseUrl, initData, endpointId, signal }) {
    return adminRPCPoolRequest({
        apiBaseUrl,
        payload: { initData, action: 'delete', endpoint_id: Number(endpointId) },
        signal,
    });
}

export async function checkAdminRPCEndpoint({ apiBaseUrl, initData, endpointId, signal }) {
    return adminRPCPoolRequest({
        apiBaseUrl,
        payload: { initData, action: 'check', endpoint_id: Number(endpointId) },
        signal,
    });
}

// Pool data sources (Admin)

async function adminPoolDataSourcesRequest({ apiBaseUrl, payload, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const url = `${base}/api/admin?endpoint=pool_data_sources`;
    const resp = await fetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload || {}),
        signal,
    });
    if (!resp.ok) {
        const text = await resp.text().catch(() => '');
        throw new Error(text || `HTTP ${resp.status}`);
    }
    return resp.json();
}

export async function fetchAdminPoolDataSources({ apiBaseUrl, initData, signal }) {
    return adminPoolDataSourcesRequest({
        apiBaseUrl,
        payload: { initData, action: 'list' },
        signal,
    });
}

export async function addAdminPoolDataSource({
    apiBaseUrl,
    initData,
    name,
    sourceType,
    chain,
    timeframeMinutes,
    limit,
    baseUrl,
    pathTemplate,
    queryTemplate,
    protocols,
    dexes,
    setCurrent,
    signal,
}) {
    return adminPoolDataSourcesRequest({
        apiBaseUrl,
        payload: {
            initData,
            action: 'add',
            name,
            source_type: sourceType,
            chain,
            timeframe_minutes: Number(timeframeMinutes),
            limit: Number(limit),
            base_url: baseUrl,
            path_template: pathTemplate,
            query_template: queryTemplate || {},
            protocols: Array.isArray(protocols) ? protocols : [],
            dexes: Array.isArray(dexes) ? dexes : [],
            set_current: Boolean(setCurrent),
        },
        signal,
    });
}

export async function updateAdminPoolDataSource({
    apiBaseUrl,
    initData,
    sourceId,
    name,
    sourceType,
    chain,
    timeframeMinutes,
    limit,
    baseUrl,
    pathTemplate,
    queryTemplate,
    protocols,
    dexes,
    setCurrent,
    signal,
}) {
    return adminPoolDataSourcesRequest({
        apiBaseUrl,
        payload: {
            initData,
            action: 'update',
            source_id: Number(sourceId),
            name,
            source_type: sourceType,
            chain,
            timeframe_minutes: Number(timeframeMinutes),
            limit: Number(limit),
            base_url: baseUrl,
            path_template: pathTemplate,
            query_template: queryTemplate || {},
            protocols: Array.isArray(protocols) ? protocols : [],
            dexes: Array.isArray(dexes) ? dexes : [],
            set_current: Boolean(setCurrent),
        },
        signal,
    });
}

export async function switchAdminPoolDataSource({ apiBaseUrl, initData, sourceId, signal }) {
    return adminPoolDataSourcesRequest({
        apiBaseUrl,
        payload: { initData, action: 'switch', source_id: Number(sourceId) },
        signal,
    });
}

export async function enableAdminPoolDataSource({ apiBaseUrl, initData, sourceId, signal }) {
    return adminPoolDataSourcesRequest({
        apiBaseUrl,
        payload: { initData, action: 'enable', source_id: Number(sourceId) },
        signal,
    });
}

export async function disableAdminPoolDataSource({ apiBaseUrl, initData, sourceId, signal }) {
    return adminPoolDataSourcesRequest({
        apiBaseUrl,
        payload: { initData, action: 'disable', source_id: Number(sourceId) },
        signal,
    });
}

export async function deleteAdminPoolDataSource({ apiBaseUrl, initData, sourceId, signal }) {
    return adminPoolDataSourcesRequest({
        apiBaseUrl,
        payload: { initData, action: 'delete', source_id: Number(sourceId) },
        signal,
    });
}

export async function checkAdminPoolDataSource({ apiBaseUrl, initData, sourceId, signal }) {
    return adminPoolDataSourcesRequest({
        apiBaseUrl,
        payload: { initData, action: 'check', source_id: Number(sourceId) },
        signal,
    });
}

// OKX API config pool (Admin)

async function adminOKXPoolRequest({ apiBaseUrl, payload, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const url = `${base}/api/admin?endpoint=okx_pool`;
    const resp = await fetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload || {}),
        signal,
    });
    if (!resp.ok) {
        const text = await resp.text().catch(() => '');
        throw new Error(text || `HTTP ${resp.status}`);
    }
    return resp.json();
}

export async function fetchAdminOKXPool({ apiBaseUrl, initData, signal }) {
    return adminOKXPoolRequest({
        apiBaseUrl,
        payload: { initData, action: 'list' },
        signal,
    });
}

export async function addAdminOKXConfig({
    apiBaseUrl,
    initData,
    name,
    baseUrl,
    apiKey,
    secretKey,
    passphrase,
    setCurrent,
    signal,
}) {
    return adminOKXPoolRequest({
        apiBaseUrl,
        payload: {
            initData,
            action: 'add',
            name,
            base_url: baseUrl,
            api_key: apiKey,
            secret_key: secretKey,
            passphrase,
            set_current: Boolean(setCurrent),
        },
        signal,
    });
}

export async function updateAdminOKXConfig({
    apiBaseUrl,
    initData,
    configId,
    name,
    baseUrl,
    apiKey,
    secretKey,
    passphrase,
    setCurrent,
    signal,
}) {
    return adminOKXPoolRequest({
        apiBaseUrl,
        payload: {
            initData,
            action: 'update',
            config_id: Number(configId),
            name,
            base_url: baseUrl,
            api_key: apiKey,
            secret_key: secretKey,
            passphrase,
            set_current: Boolean(setCurrent),
        },
        signal,
    });
}

export async function renameAdminOKXConfig({ apiBaseUrl, initData, configId, name, signal }) {
    return adminOKXPoolRequest({
        apiBaseUrl,
        payload: { initData, action: 'rename', config_id: Number(configId), name },
        signal,
    });
}

export async function switchAdminOKXConfig({ apiBaseUrl, initData, configId, signal }) {
    return adminOKXPoolRequest({
        apiBaseUrl,
        payload: { initData, action: 'switch', config_id: Number(configId) },
        signal,
    });
}

export async function disableAdminOKXConfig({ apiBaseUrl, initData, configId, signal }) {
    return adminOKXPoolRequest({
        apiBaseUrl,
        payload: { initData, action: 'disable', config_id: Number(configId) },
        signal,
    });
}

export async function disableAdminOKXConfigNextMonth({ apiBaseUrl, initData, configId, signal }) {
    return adminOKXPoolRequest({
        apiBaseUrl,
        payload: { initData, action: 'disable', config_id: Number(configId), disable_next_month: true },
        signal,
    });
}

export async function enableAdminOKXConfig({ apiBaseUrl, initData, configId, signal }) {
    return adminOKXPoolRequest({
        apiBaseUrl,
        payload: { initData, action: 'enable', config_id: Number(configId) },
        signal,
    });
}

export async function deleteAdminOKXConfig({ apiBaseUrl, initData, configId, signal }) {
    return adminOKXPoolRequest({
        apiBaseUrl,
        payload: { initData, action: 'delete', config_id: Number(configId) },
        signal,
    });
}

export async function checkAdminOKXConfig({ apiBaseUrl, initData, configId, signal }) {
    return adminOKXPoolRequest({
        apiBaseUrl,
        payload: { initData, action: 'check', config_id: Number(configId) },
        signal,
    });
}

async function adminPrivateZapRequest({ apiBaseUrl, payload, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const url = `${base}/api/admin?endpoint=private_zap`;
    const resp = await fetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload || {}),
        signal,
    });
    if (!resp.ok) {
        const text = await resp.text().catch(() => '');
        throw new Error(text || `HTTP ${resp.status}`);
    }
    return resp.json();
}

export async function fetchAdminPrivateZap({ apiBaseUrl, initData, signal }) {
    return adminPrivateZapRequest({
        apiBaseUrl,
        payload: { initData, action: 'list' },
        signal,
    });
}

export async function invalidateAdminPrivateZap({ apiBaseUrl, initData, chain, kind, signal }) {
    return adminPrivateZapRequest({
        apiBaseUrl,
        payload: { initData, action: 'invalidate', chain, kind },
        signal,
    });
}

export async function saveGlobalConfig({ apiBaseUrl, initData, config, signal }) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const url = base + '/api/settings?endpoint=global_config';
    const resp = await fetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ initData, action: 'save', ...config }),
        signal,
    });
    if (!resp.ok) {
        const text = await resp.text().catch(() => '');
        throw new Error(text || 'HTTP ' + resp.status);
    }
    return resp.json();
}

