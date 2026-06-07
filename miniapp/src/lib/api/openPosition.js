import { readErrorDetails } from './request';

function buildOpenPositionPayload({
    initData,
    chain,
    poolAddress,
    poolVersion,
    amount,
    rangeInputMode,
    rangeLowerPct,
    rangeUpperPct,
    tickLower,
    tickUpper,
    slippageTolerance,
    entrySwapSlippageTolerance,
    allowEntrySwap,
    confirmEntrySwap,
    walletId,
    ackLiquidityRisk,
    dcaEnabled,
    dcaPercentages,
    dcaIntervalSeconds,
    taskMode,
    rebalanceEnabled,
}) {
    const payload = {
        initData,
        chain,
        pool_address: poolAddress,
        pool_version: poolVersion,
        amount,
        range_input_mode: rangeInputMode || 'percentage',
        allow_entry_swap: Boolean(allowEntrySwap),
    };
    if ((rangeInputMode || 'percentage') === 'percentage') {
        payload.range_lower_pct = rangeLowerPct;
        payload.range_upper_pct = rangeUpperPct;
    }
    if (rangeInputMode === 'tick' || rangeInputMode === 'grid') {
        const lowerTick = Number(tickLower);
        const upperTick = Number(tickUpper);
        if (Number.isInteger(lowerTick)) payload.tick_lower = lowerTick;
        if (Number.isInteger(upperTick)) payload.tick_upper = upperTick;
    }
    const wid = Number(walletId);
    if (Number.isFinite(wid) && wid > 0) {
        payload.wallet_id = wid;
    }
    if (Number.isFinite(slippageTolerance)) {
        payload.slippage_tolerance = slippageTolerance;
    }
    if (Number.isFinite(entrySwapSlippageTolerance)) {
        payload.entry_swap_slippage_tolerance = entrySwapSlippageTolerance;
    }
    if (confirmEntrySwap) {
        payload.confirm_entry_swap = true;
    }
    if (ackLiquidityRisk) {
        payload.ack_liquidity_risk = true;
    }
    if (dcaEnabled !== undefined && dcaEnabled !== null) {
        payload.dca_enabled = Boolean(dcaEnabled);
    }
    if (Array.isArray(dcaPercentages) && dcaPercentages.length > 0) {
        payload.dca_percentages = dcaPercentages.map((v) => Number(v));
    }
    const dcaInterval = Number(dcaIntervalSeconds);
    if (Number.isFinite(dcaInterval) && dcaInterval >= 0) {
        payload.dca_interval_seconds = Math.round(dcaInterval * 1000) / 1000;
    }
    if (taskMode !== undefined && taskMode !== null && String(taskMode).trim()) {
        payload.task_mode = String(taskMode).trim();
    } else if (rebalanceEnabled !== undefined && rebalanceEnabled !== null) {
        payload.rebalance_enabled = Boolean(rebalanceEnabled);
    }
    return payload;
}

function createOpenPositionError(resp, detail, messageWhenHttpOnly) {
    const rawMessage = String(detail.message || '').trim();
    const displayMessage = rawMessage === `HTTP ${resp.status}` || rawMessage === ''
        ? `${messageWhenHttpOnly}（HTTP ${resp.status}）`
        : rawMessage;
    const err = new Error(displayMessage);
    err.status = resp.status;
    if (detail.payload && typeof detail.payload === 'object') {
        err.payload = detail.payload;
        Object.assign(err, detail.payload);
    }
    return { err, rawMessage };
}

function shouldTryLegacyOpenPositionUrl({ index, urlCount, rawMessage, status }) {
    return index < urlCount - 1 && (
        rawMessage === `HTTP ${status}` ||
        rawMessage === '' ||
        status === 404 ||
        status === 405
    );
}

async function postOpenPositionWithLegacyUrls({ urls, payload, signal, messageWhenHttpOnly, finalMessage }) {
    let lastError = null;
    for (let i = 0; i < urls.length; i += 1) {
        const resp = await fetch(urls[i], {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(payload),
            signal,
        });
        if (resp.ok) {
            return resp.json();
        }
        const detail = await readErrorDetails(resp);
        const { err, rawMessage } = createOpenPositionError(resp, detail, messageWhenHttpOnly);
        lastError = err;
        if (shouldTryLegacyOpenPositionUrl({ index: i, urlCount: urls.length, rawMessage, status: resp.status })) {
            continue;
        }
        throw err;
    }
    throw lastError || new Error(finalMessage);
}

export async function previewOpenPosition({
    apiBaseUrl,
    initData,
    chain,
    poolAddress,
    poolVersion,
    amount,
    rangeInputMode,
    rangeLowerPct,
    rangeUpperPct,
    tickLower,
    tickUpper,
    slippageTolerance,
    entrySwapSlippageTolerance,
    allowEntrySwap,
    walletId,
    ackLiquidityRisk,
    dcaEnabled,
    dcaPercentages,
    dcaIntervalSeconds,
    taskMode,
    rebalanceEnabled,
    signal,
}) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const payload = buildOpenPositionPayload({
        initData,
        chain,
        poolAddress,
        poolVersion,
        amount,
        rangeInputMode,
        rangeLowerPct,
        rangeUpperPct,
        tickLower,
        tickUpper,
        slippageTolerance,
        entrySwapSlippageTolerance,
        allowEntrySwap,
        walletId,
        ackLiquidityRisk,
        dcaEnabled,
        dcaPercentages,
        dcaIntervalSeconds,
        taskMode,
        rebalanceEnabled,
    });
    return postOpenPositionWithLegacyUrls({
        urls: [
            `${base}/api/open_position_preview`,
            `${base}/api/trading?endpoint=open_position_preview`,
        ],
        payload,
        signal,
        messageWhenHttpOnly: '获取前置兑换预览失败',
        finalMessage: '获取前置兑换预览失败',
    });
}

export async function prepareOpenPosition({
    apiBaseUrl,
    initData,
    chain,
    poolAddress,
    poolVersion,
    walletId,
    signal,
}) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const payload = {
        initData,
        chain,
        pool_address: poolAddress,
        pool_version: poolVersion,
    };
    const wid = Number(walletId);
    if (Number.isFinite(wid) && wid > 0) {
        payload.wallet_id = wid;
    }
    return postOpenPositionWithLegacyUrls({
        urls: [
            `${base}/api/open_position_prepare`,
            `${base}/api/trading?endpoint=open_position_prepare`,
        ],
        payload,
        signal,
        messageWhenHttpOnly: '获取开仓预检测失败',
        finalMessage: '获取开仓预检测失败',
    });
}

export async function openPosition({
    apiBaseUrl,
    initData,
    chain,
    poolAddress,
    poolVersion,
    amount,
    rangeInputMode,
    rangeLowerPct,
    rangeUpperPct,
    tickLower,
    tickUpper,
    slippageTolerance,
    entrySwapSlippageTolerance,
    allowEntrySwap,
    confirmEntrySwap,
    walletId,
    ackLiquidityRisk,
    dcaEnabled,
    dcaPercentages,
    dcaIntervalSeconds,
    taskMode,
    rebalanceEnabled,
    signal,
}) {
    const base = String(apiBaseUrl || '').replace(/\/$/, '');
    const url = `${base}/api/trading?endpoint=open_position`;
    const payload = buildOpenPositionPayload({
        initData,
        chain,
        poolAddress,
        poolVersion,
        amount,
        rangeInputMode,
        rangeLowerPct,
        rangeUpperPct,
        tickLower,
        tickUpper,
        slippageTolerance,
        entrySwapSlippageTolerance,
        allowEntrySwap,
        confirmEntrySwap,
        walletId,
        ackLiquidityRisk,
        dcaEnabled,
        dcaPercentages,
        dcaIntervalSeconds,
        taskMode,
        rebalanceEnabled,
    });
    const resp = await fetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
        signal,
    });
    if (!resp.ok) {
        const detail = await readErrorDetails(resp);
        const err = new Error(detail.message);
        err.status = resp.status;
        if (detail.payload && typeof detail.payload === 'object') {
            err.payload = detail.payload;
            Object.assign(err, detail.payload);
        }
        throw err;
    }
    return resp.json();
}
