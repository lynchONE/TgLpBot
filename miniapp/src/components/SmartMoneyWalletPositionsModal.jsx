import React, { useEffect, useMemo, useState } from 'react';
import BottomSheet from './BottomSheet.jsx';
import NumberFlowValue from './NumberFlowValue.jsx';
import { fetchSmartMoneyWalletPositions } from '../lib/api';
import { copyToClipboard, hapticImpact, hapticNotification } from '../lib/telegram';

const Icon = ({ path, className = '' }) => (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" className={className} aria-hidden="true">
        <path d={path} />
    </svg>
);

const icons = {
    close: 'M6 18L18 6M6 6l12 12',
    refresh: 'M21 12a9 9 0 1 1-2.64-6.36M21 3v6h-6',
};

const USD_DISPLAY_LIMIT = 1e15;
const usdFormatter = new Intl.NumberFormat('en-US', {
    style: 'currency',
    currency: 'USD',
    maximumFractionDigits: 2,
});

const compactNumberFormatter = new Intl.NumberFormat('en-US', {
    notation: 'compact',
    maximumFractionDigits: 2,
});

function formatUsd(v) {
    const n = Number(v ?? 0);
    if (!Number.isFinite(n) || Math.abs(n) > USD_DISPLAY_LIMIT) return '$--';
    return usdFormatter.format(n);
}

function formatTokenAmount(v) {
    const n = Number(v ?? 0);
    if (!Number.isFinite(n) || Math.abs(n) > USD_DISPLAY_LIMIT) return '--';
    const abs = Math.abs(n);
    if (abs === 0) return '0';
    if (abs >= 1000) return compactNumberFormatter.format(n);
    if (abs >= 1) return n.toFixed(4).replace(/\.?0+$/, '');
    return n.toPrecision(4);
}

function formatPct(v, digits = 2) {
    const n = Number(v);
    if (!Number.isFinite(n)) return '--';
    return `${n.toFixed(digits)}%`;
}

function shortHex(value, head = 8, tail = 6) {
    const s = String(value || '').trim();
    if (!s) return '';
    if (s.length <= head + tail + 2) return s;
    return `${s.slice(0, head)}...${s.slice(-tail)}`;
}

async function safeCopy(value, onNotice) {
    const text = String(value || '').trim();
    if (!text) return;
    try {
        await copyToClipboard(text);
        hapticNotification('success');
        if (typeof onNotice === 'function') onNotice('已复制', 'success');
    } catch (e) {
        hapticNotification('error');
        if (typeof onNotice === 'function') onNotice(`复制失败：${String(e?.message || e)}`, 'error');
    }
}

function kpiTone(value) {
    const n = Number(value ?? 0);
    if (!Number.isFinite(n)) return 'text-zinc-500 dark:text-white/40';
    if (n > 0) return 'text-emerald-600 dark:text-emerald-300';
    if (n < 0) return 'text-red-600 dark:text-red-300';
    return 'text-zinc-700 dark:text-white/80';
}

export default function SmartMoneyWalletPositionsModal({
    open,
    onClose,
    apiBaseUrl,
    initData,
    chain = 'bsc',
    walletAddress,
    windowHours,
    limit = 30,
    onNotice,
}) {
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState('');
    const [data, setData] = useState(null);
    const [nonce, setNonce] = useState(0);

    const addr = String(walletAddress || '').trim();
    const chainLabel = String(chain || 'bsc').trim() || 'bsc';
    const windowLabel = Number.isFinite(Number(windowHours)) && Number(windowHours) > 0 ? `${Number(windowHours)}h` : '';

    const positions = Array.isArray(data?.positions) ? data.positions : [];
    const warnings = Array.isArray(data?.warnings) ? data.warnings : [];
    const totalUsd = useMemo(
        () => positions.reduce((sum, p) => sum + (Number(p?.position_usd ?? 0) || 0), 0),
        [positions],
    );

    useEffect(() => {
        if (!open) {
            setLoading(false);
            setError('');
            setData(null);
            return;
        }
        if (!addr) {
            setError('walletAddress 为空');
            return;
        }

        let aborted = false;
        const controller = new AbortController();

        setLoading(true);
        setError('');

        fetchSmartMoneyWalletPositions({
            apiBaseUrl,
            initData,
            chain: chainLabel,
            walletAddress: addr,
            windowHours,
            limit,
            signal: controller.signal,
        })
            .then((resp) => {
                if (aborted) return;
                setData(resp);
            })
            .catch((e) => {
                if (aborted) return;
                setError(String(e?.message || e));
            })
            .finally(() => {
                if (aborted) return;
                setLoading(false);
            });

        return () => {
            aborted = true;
            controller.abort();
        };
    }, [open, apiBaseUrl, initData, chainLabel, addr, windowHours, limit, nonce]);

    if (!open) return null;

    return (
        <BottomSheet
            open={open}
            onClose={onClose}
            maxHeightClass="h-[92vh] sm:h-[720px] max-h-none"
            headerClassName="px-4 py-3 border-b border-zinc-100 dark:border-white/5 bg-zinc-50/50 dark:bg-[#111318]/50 shrink-0"
            contentClassName="p-4"
            title={
                <div>
                    <div className="truncate text-sm font-bold text-zinc-900 dark:text-white/90">钱包仓位</div>
                    <div className="mt-0.5 flex items-center gap-2 text-[10px] text-zinc-500 dark:text-white/40 font-mono">
                        <span className="truncate">{shortHex(addr, 12, 10) || '--'}</span>
                        <span className="shrink-0">·</span>
                        <span className="shrink-0">{chainLabel}</span>
                        {windowLabel ? (
                            <>
                                <span className="shrink-0">·</span>
                                <span className="shrink-0">最近<NumberFlowValue value={windowLabel} formatter={() => windowLabel} /></span>
                            </>
                        ) : null}
                    </div>
                </div>
            }
            headerRight={
                <>
                    <button
                        type="button"
                        onClick={() => {
                            hapticImpact('light');
                            safeCopy(addr, onNotice);
                        }}
                        className="inline-flex items-center rounded-lg bg-zinc-100 px-2.5 py-1 text-[11px] font-semibold text-zinc-700 hover:bg-zinc-200 dark:bg-white/5 dark:text-white/70 dark:hover:bg-white/10"
                    >
                        复制
                    </button>
                    <button
                        type="button"
                        onClick={() => {
                            hapticImpact('light');
                            setNonce((v) => v + 1);
                        }}
                        className="inline-flex h-8 w-8 items-center justify-center rounded-full bg-zinc-100 text-zinc-600 transition hover:bg-zinc-200 active:bg-zinc-300 dark:bg-white/10 dark:text-white/70 dark:hover:bg-white/20"
                        aria-label="刷新"
                        title="刷新"
                    >
                        <Icon path={icons.refresh} className="h-4 w-4" />
                    </button>
                </>
            }
        >
            <div className="grid grid-cols-2 gap-2 text-xs">
                <div className="rounded-xl border border-zinc-200 bg-zinc-50 p-2 dark:border-white/10 dark:bg-[#0f1116]">
                    <div className="text-[11px] text-zinc-500 dark:text-white/40">活跃仓位</div>
                    <div className="mt-0.5 font-semibold tabular-nums text-zinc-900 dark:text-white/80">
                        <NumberFlowValue value={positions.length} formatOptions={{ maximumFractionDigits: 0 }} />
                    </div>
                </div>
                <div className="rounded-xl border border-zinc-200 bg-zinc-50 p-2 dark:border-white/10 dark:bg-[#0f1116]">
                    <div className="text-[11px] text-zinc-500 dark:text-white/40">总仓位估值</div>
                    <div className={`mt-0.5 font-semibold tabular-nums ${kpiTone(totalUsd)}`}>
                        <NumberFlowValue value={totalUsd} formatter={(v) => formatUsd(v)} />
                    </div>
                </div>
            </div>

            {loading ? (
                <div className="mt-3 text-xs text-zinc-500 dark:text-white/50">加载中...</div>
            ) : null}
            {error ? (
                <div className="mt-3 rounded-xl border border-red-500/30 bg-red-500/10 p-3 text-xs text-red-700 dark:border-red-500/20 dark:bg-red-500/5 dark:text-red-200">
                    {error}
                </div>
            ) : null}
            {warnings.length ? (
                <div className="mt-3 rounded-xl border border-amber-500/30 bg-amber-500/10 p-3 text-xs text-amber-700 dark:border-amber-500/20 dark:bg-amber-500/5 dark:text-amber-200">
                    <div className="font-semibold">提示</div>
                    <ul className="mt-1 list-disc space-y-1 pl-4">
                        {warnings.slice(0, 6).map((w, i) => (
                            <li key={String(i)}>{String(w)}</li>
                        ))}
                    </ul>
                </div>
            ) : null}

            {!loading && !error && positions.length === 0 ? (
                <div className="mt-3 rounded-xl border border-zinc-200 bg-white/40 backdrop-blur-md p-3 text-[11px] text-zinc-500 dark:border-white/10 dark:bg-white/5 dark:text-white/60">
                    该钱包当前无活跃 LP 仓位
                </div>
            ) : null}

            {positions.length ? (
                <div className="mt-3 space-y-2">
                    {positions.map((p) => {
                        const version = String(p?.pool_version || '').trim().toUpperCase() || '--';
                        const poolId = String(p?.pool_id || '').trim();
                        const positionId = String(p?.position_id || '').trim();
                        const exchange = String(p?.exchange || '').trim();
                        const pair = String(p?.pair || '').trim() || '--';
                        const feePct = Number(p?.fee_pct);
                        const inRange = Boolean(p?.in_range);
                        const tickLower = Number(p?.tick_lower ?? 0);
                        const tickUpper = Number(p?.tick_upper ?? 0);
                        const currentTick = Number(p?.current_tick ?? 0);
                        const sym0 = String(p?.token0_symbol || '').trim();
                        const sym1 = String(p?.token1_symbol || '').trim();
                        const amount0 = Number(p?.amount0 ?? 0);
                        const amount1 = Number(p?.amount1 ?? 0);
                        const usd0 = Number(p?.amount0_usd ?? 0);
                        const usd1 = Number(p?.amount1_usd ?? 0);
                        const posUsd = Number(p?.position_usd ?? 0);

                        return (
                            <div key={`${version}:${poolId}:${positionId}`} className="rounded-2xl border border-zinc-200 bg-white/40 backdrop-blur-md p-3 dark:border-white/10 dark:bg-white/5">
                                <div className="flex items-start justify-between gap-3">
                                    <div className="min-w-0">
                                        <div className="flex items-center gap-2">
                                            <div className="truncate text-sm font-extrabold text-zinc-900 dark:text-white/90">{pair}</div>
                                            <span className="shrink-0 rounded bg-zinc-100 px-1.5 py-0.5 text-[10px] font-semibold text-zinc-700 dark:bg-white/10 dark:text-white/70">
                                                {version}
                                            </span>
                                            {exchange ? (
                                                <span className="shrink-0 rounded bg-zinc-100 px-1.5 py-0.5 text-[10px] font-semibold text-zinc-700 dark:bg-white/10 dark:text-white/70">
                                                    {exchange}
                                                </span>
                                            ) : null}
                                            {Number.isFinite(feePct) && feePct > 0 ? (
                                                <span className="shrink-0 rounded bg-zinc-100 px-1.5 py-0.5 text-[10px] font-semibold text-zinc-700 dark:bg-white/10 dark:text-white/70">
                                                    <NumberFlowValue value={feePct} formatter={(v) => formatPct(v)} />
                                                </span>
                                            ) : null}
                                            <span className={`shrink-0 rounded px-1.5 py-0.5 text-[10px] font-semibold ${inRange ? 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-200' : 'bg-zinc-500/10 text-zinc-700 dark:text-white/60'}`}>
                                                {inRange ? 'In Range' : 'Out Range'}
                                            </span>
                                        </div>
                                        <div className="mt-1 flex flex-wrap items-center gap-x-2 gap-y-1 text-[11px] text-zinc-500 dark:text-white/40">
                                            <span className="font-mono">{shortHex(poolId, 10, 8) || '--'}</span>
                                            <span>·</span>
                                            <span className="font-mono">#<NumberFlowValue value={positionId || '--'} formatter={() => (positionId || '--')} /></span>
                                        </div>
                                    </div>
                                    <div className="shrink-0 text-right">
                                        <div className="text-[10px] text-zinc-500 dark:text-white/40">仓位估值</div>
                                        <div className={`mt-0.5 text-sm font-extrabold tabular-nums ${kpiTone(posUsd)}`}>
                                            <NumberFlowValue value={posUsd} formatter={(v) => formatUsd(v)} />
                                        </div>
                                    </div>
                                </div>

                                <div className="mt-3 grid grid-cols-2 gap-2 text-[11px]">
                                    <div className="rounded-xl border border-zinc-200 bg-zinc-50 p-2 dark:border-white/10 dark:bg-[#0f1116]">
                                        <div className="text-[10px] text-zinc-500 dark:text-white/40">区间</div>
                                        <div className="mt-0.5 font-semibold tabular-nums text-zinc-900 dark:text-white/80">
                                            <NumberFlowValue
                                                value={Number.isFinite(tickLower) ? tickLower : '--'}
                                                formatter={() => (Number.isFinite(tickLower) ? String(tickLower) : '--')}
                                            />
                                            {' '}→{' '}
                                            <NumberFlowValue
                                                value={Number.isFinite(tickUpper) ? tickUpper : '--'}
                                                formatter={() => (Number.isFinite(tickUpper) ? String(tickUpper) : '--')}
                                            />
                                        </div>
                                        <div className="text-[10px] text-zinc-500 dark:text-white/40">
                                            当前{' '}
                                            <NumberFlowValue
                                                value={Number.isFinite(currentTick) ? currentTick : '--'}
                                                formatter={() => (Number.isFinite(currentTick) ? String(currentTick) : '--')}
                                            />
                                        </div>
                                    </div>
                                    <div className="rounded-xl border border-zinc-200 bg-zinc-50 p-2 dark:border-white/10 dark:bg-[#0f1116]">
                                        <div className="text-[10px] text-zinc-500 dark:text-white/40">Token Amount</div>
                                        <div className="mt-0.5 font-semibold tabular-nums text-zinc-900 dark:text-white/80">
                                            <NumberFlowValue value={amount0} formatter={(v) => formatTokenAmount(v)} /> {sym0 || 'T0'}
                                        </div>
                                        <div className="text-[10px] text-zinc-500 dark:text-white/40">
                                            <NumberFlowValue value={amount1} formatter={(v) => formatTokenAmount(v)} /> {sym1 || 'T1'}
                                        </div>
                                    </div>
                                </div>

                                <div className="mt-2 grid grid-cols-2 gap-2 text-[11px]">
                                    <div className="rounded-xl border border-zinc-200 bg-zinc-50 p-2 dark:border-white/10 dark:bg-[#0f1116]">
                                        <div className="text-[10px] text-zinc-500 dark:text-white/40">{sym0 || 'Token0'} 估值</div>
                                        <div className="mt-0.5 font-semibold tabular-nums text-zinc-900 dark:text-white/80">
                                            <NumberFlowValue value={usd0} formatter={(v) => formatUsd(v)} />
                                        </div>
                                    </div>
                                    <div className="rounded-xl border border-zinc-200 bg-zinc-50 p-2 dark:border-white/10 dark:bg-[#0f1116]">
                                        <div className="text-[10px] text-zinc-500 dark:text-white/40">{sym1 || 'Token1'} 估值</div>
                                        <div className="mt-0.5 font-semibold tabular-nums text-zinc-900 dark:text-white/80">
                                            <NumberFlowValue value={usd1} formatter={(v) => formatUsd(v)} />
                                        </div>
                                    </div>
                                </div>

                                <div className="mt-3 flex items-center justify-end gap-2">
                                    <button
                                        type="button"
                                        onClick={() => {
                                            hapticImpact('light');
                                            safeCopy(poolId, onNotice);
                                        }}
                                        className="inline-flex items-center rounded-lg bg-zinc-100 px-2 py-1 text-[10px] font-semibold text-zinc-700 hover:bg-zinc-200 dark:bg-white/5 dark:text-white/70 dark:hover:bg-white/10"
                                    >
                                        复制 PoolID
                                    </button>
                                    <button
                                        type="button"
                                        onClick={() => {
                                            hapticImpact('light');
                                            safeCopy(positionId, onNotice);
                                        }}
                                        className="inline-flex items-center rounded-lg bg-zinc-100 px-2 py-1 text-[10px] font-semibold text-zinc-700 hover:bg-zinc-200 dark:bg-white/5 dark:text-white/70 dark:hover:bg-white/10"
                                    >
                                        复制 PositionID
                                    </button>
                                </div>
                            </div>
                        );
                    })}
                </div>
            ) : null}
        </BottomSheet>
    );
}
