import React, { useEffect, useMemo, useState } from 'react';
import BottomSheet from './BottomSheet.jsx';
import NumberFlowValue from './NumberFlowValue.jsx';
import { fetchSmartMoneyPoolAdds } from '../lib/api';
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

function formatPrice(v) {
    const n = Number(v ?? 0);
    if (!Number.isFinite(n) || n <= 0) return '--';
    if (n >= 1000) return n.toFixed(2);
    if (n >= 1) return n.toFixed(4).replace(/\.?0+$/, '');
    return n.toPrecision(4);
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
        if (typeof onNotice === 'function') onNotice(`复制失败: ${String(e?.message || e)}`, 'error');
    }
}

function kpiTone(value) {
    const n = Number(value ?? 0);
    if (!Number.isFinite(n)) return 'text-zinc-500 dark:text-white/40';
    if (n > 0) return 'text-emerald-600 dark:text-emerald-300';
    if (n < 0) return 'text-red-600 dark:text-red-300';
    return 'text-zinc-700 dark:text-white/80';
}

export default function SmartMoneyPoolAddsModal({
    open,
    onClose,
    apiBaseUrl,
    initData,
    chain = 'bsc',
    poolVersion,
    poolId,
    windowHours = 2,
    limit = 60,
    feesLimit = 30,
    onNotice,
    onOpenFollow,
    onOpenPositions,
}) {
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState('');
    const [data, setData] = useState(null);
    const [nonce, setNonce] = useState(0);

    const pv = String(poolVersion || '').trim().toLowerCase();
    const pid = String(poolId || '').trim();
    const chainLabel = String(chain || 'bsc').trim() || 'bsc';

    const pool = data?.pool || null;
    const wallets = Array.isArray(data?.wallets) ? data.wallets : [];
    const warnings = Array.isArray(data?.warnings) ? data.warnings : [];

    const title = useMemo(() => {
        const pair = String(pool?.pair || '').trim();
        const version = String(pool?.pool_version || pv || '').trim().toUpperCase();
        const feePct = Number(pool?.fee_pct ?? 0);
        const feeText = Number.isFinite(feePct) && feePct > 0 ? ` · ${(feePct * 1).toFixed(2)}%` : '';
        if (pair) return `${pair} (${version}${feeText})`;
        return `${version || 'POOL'} ${shortHex(pid, 10, 6) || '--'}`;
    }, [pool?.pair, pool?.pool_version, pool?.fee_pct, pv, pid]);

    useEffect(() => {
        if (!open) {
            setLoading(false);
            setError('');
            setData(null);
            return;
        }
        if (!pv || !pid) {
            setError('pool 为空');
            return;
        }

        let aborted = false;
        const controller = new AbortController();

        setLoading(true);
        setError('');

        fetchSmartMoneyPoolAdds({
            apiBaseUrl,
            initData,
            chain: chainLabel,
            poolVersion: pv,
            poolId: pid,
            windowHours,
            limit,
            feesLimit,
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
    }, [open, apiBaseUrl, initData, chainLabel, pv, pid, windowHours, limit, feesLimit, nonce]);

    if (!open) return null;

    return (
        <BottomSheet
            open={open}
            onClose={() => {
                hapticImpact('light');
                if (typeof onClose === 'function') onClose();
            }}
            maxHeightClass="max-h-[92vh] sm:max-h-[720px] max-w-2xl"
            headerClassName="px-4 py-3 border-b border-zinc-100 dark:border-white/5 bg-zinc-50/50 dark:bg-[#111318]/50 shrink-0"
            contentClassName="p-4 pb-20"
            title={
                <div>
                    <div className="truncate text-sm font-semibold text-zinc-900 dark:text-white/90">
                        {title}
                        {loading ? (
                            <span className="ml-2 inline-flex items-center rounded-lg bg-zinc-100 px-2 py-0.5 text-[10px] font-semibold text-zinc-600 dark:bg-white/5 dark:text-white/60">
                                加载中...
                            </span>
                        ) : null}
                    </div>
                    <div className="mt-1 flex flex-wrap items-center gap-x-2 gap-y-0.5 text-[10px] text-zinc-500 dark:text-white/40">
                        <span>最近 <NumberFlowValue value={Number(windowHours) || 2} formatOptions={{ maximumFractionDigits: 0 }} />h 加池子</span>
                        <span>· <NumberFlowValue value={wallets.length} formatOptions={{ maximumFractionDigits: 0 }} /> 条</span>
                        <span>· 手续费为链上可领取估算</span>
                    </div>
                </div>
            }
            headerRight={
                <button
                    type="button"
                    onClick={() => {
                        hapticImpact('light');
                        setNonce((v) => v + 1);
                    }}
                    className="inline-flex h-8 w-8 items-center justify-center rounded-full bg-zinc-100 text-zinc-600 transition hover:bg-zinc-200 active:bg-zinc-300 dark:bg-white/10 dark:text-white/70 dark:hover:bg-white/20"
                    aria-label="Refresh"
                    title="Refresh"
                >
                    <Icon path={icons.refresh} className="h-4 w-4" />
                </button>
            }
        >

            {warnings.length ? (
                <div className="mt-3 rounded-xl border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-xs text-amber-700 dark:text-amber-200">
                    <div className="font-semibold">提示</div>
                    <ul className="mt-1 list-disc space-y-1 pl-4">
                        {warnings.slice(0, 4).map((w, i) => (
                            <li key={String(i)}>{String(w)}</li>
                        ))}
                    </ul>
                </div>
            ) : null}

            {error ? (
                <div className="mt-3 rounded-xl border border-red-500/30 bg-red-500/10 p-3 text-[11px] text-red-700 dark:border-red-500/20 dark:bg-red-500/5 dark:text-red-200">
                    {error}
                </div>
            ) : null}

            {!error && !loading && wallets.length === 0 ? (
                <div className="mt-3 rounded-xl border border-zinc-200 bg-zinc-50 p-3 text-[11px] text-zinc-500 dark:border-white/10 dark:bg-white/5 dark:text-white/60">
                    暂无加池子记录
                </div>
            ) : null}

            {wallets.length ? (
                <div className="mt-3 space-y-2">
                    {wallets.map((row, index) => {
                        const addr = String(row?.wallet_address || '').trim();
                        const tickLower = Number(row?.tick_lower);
                        const tickUpper = Number(row?.tick_upper);
                        const amt0 = Number(row?.amount0 ?? 0);
                        const amt1 = Number(row?.amount1 ?? 0);
                        const totalUsd = Number(row?.total_usd ?? 0);
                        const feeUsd = Number(row?.claimable_fees_usd ?? 0);
                        const feeStatus = String(row?.fee_status || '').trim();
                        const feeErr = String(row?.fee_error || '').trim();
                        const sym0 = String(pool?.token0_symbol || 'T0').trim();
                        const sym1 = String(pool?.token1_symbol || 'T1').trim();
                        const priceLower = Number(row?.price_lower ?? 0);
                        const priceUpper = Number(row?.price_upper ?? 0);
                        const priceBase = String(row?.price_base || '').trim();
                        const priceQuote = String(row?.price_quote || '').trim();
                        const rangeText = Number.isFinite(priceLower) && priceLower > 0 && Number.isFinite(priceUpper) && priceUpper > 0
                            ? `${formatPrice(priceLower)} - ${formatPrice(priceUpper)} ${priceQuote || ''}`
                            : '--';
                        const feeTone = kpiTone(feeUsd);

                        return (
                            <div key={`${addr || String(index)}:${tickLower}:${tickUpper}:${index}`} className="rounded-2xl border border-zinc-200 bg-white p-3 shadow-sm dark:border-white/10 dark:bg-[#141821] dark:shadow-none">
                                <div className="flex items-start justify-between gap-2">
                                    <div className="min-w-0">
                                        <div className="flex items-center gap-2">
                                            <span className="truncate font-mono text-[11px] font-semibold text-zinc-900 dark:text-white/90">
                                                {shortHex(addr, 10, 8) || '--'}
                                            </span>
                                            <span className="rounded-md bg-zinc-100 px-1.5 py-0.5 text-[10px] font-semibold text-zinc-700 dark:bg-white/5 dark:text-white/60">
                                                #<NumberFlowValue value={index + 1} formatOptions={{ maximumFractionDigits: 0 }} />
                                            </span>
                                        </div>
                                        <div className="mt-1 flex flex-wrap items-center gap-x-2 gap-y-0.5 text-[10px] text-zinc-500 dark:text-white/40">
                                            <span>
                                                区间{' '}
                                                <NumberFlowValue
                                                    value={Number.isFinite(tickLower) ? tickLower : '--'}
                                                    formatter={() => (Number.isFinite(tickLower) ? String(tickLower) : '--')}
                                                />
                                                {' '}→{' '}
                                                <NumberFlowValue
                                                    value={Number.isFinite(tickUpper) ? tickUpper : '--'}
                                                    formatter={() => (Number.isFinite(tickUpper) ? String(tickUpper) : '--')}
                                                />
                                            </span>
                                            <span>· 价格 <NumberFlowValue value={rangeText} formatter={() => rangeText} /></span>
                                            {priceBase ? <span className="opacity-70">({priceBase}/{priceQuote || '--'})</span> : null}
                                        </div>
                                    </div>
                                    <div className="flex items-center gap-2">
                                        <button
                                            type="button"
                                            onClick={() => {
                                                hapticImpact('light');
                                                safeCopy(addr, onNotice);
                                            }}
                                            className="inline-flex items-center rounded-lg bg-zinc-100 px-2 py-1 text-[10px] font-semibold text-zinc-700 hover:bg-zinc-200 dark:bg-white/5 dark:text-white/70 dark:hover:bg-white/10"
                                        >
                                            复制
                                        </button>
                                        <button
                                            type="button"
                                            onClick={() => {
                                                if (typeof onOpenFollow === 'function') {
                                                    hapticImpact('light');
                                                    onOpenFollow(addr);
                                                }
                                            }}
                                            className="inline-flex items-center rounded-lg bg-emerald-500/15 px-2 py-1 text-[10px] font-semibold text-emerald-700 hover:bg-emerald-500/20 dark:bg-emerald-500/10 dark:text-emerald-200 dark:hover:bg-emerald-500/15"
                                        >
                                            跟单
                                        </button>
                                        <button
                                            type="button"
                                            onClick={() => {
                                                if (typeof onOpenPositions === 'function') {
                                                    hapticImpact('light');
                                                    onOpenPositions(addr);
                                                }
                                            }}
                                            className="inline-flex items-center rounded-lg bg-zinc-100 px-2 py-1 text-[10px] font-semibold text-zinc-700 hover:bg-zinc-200 dark:bg-white/5 dark:text-white/70 dark:hover:bg-white/10"
                                        >
                                            仓位
                                        </button>
                                    </div>
                                </div>

                                <div className="mt-2 grid grid-cols-3 gap-2 text-[11px]">
                                    <div className="rounded-xl border border-zinc-200 bg-zinc-50 p-2 dark:border-white/10 dark:bg-[#0f1116]">
                                        <div className="text-[10px] text-zinc-500 dark:text-white/40">加池子金额</div>
                                        <div className="mt-0.5 font-semibold tabular-nums text-zinc-900 dark:text-white/80">
                                            <NumberFlowValue value={totalUsd} formatter={(v) => formatUsd(v)} />
                                        </div>
                                    </div>
                                    <div className="rounded-xl border border-zinc-200 bg-zinc-50 p-2 dark:border-white/10 dark:bg-[#0f1116]">
                                        <div className="text-[10px] text-zinc-500 dark:text-white/40">Token</div>
                                        <div className="mt-0.5 font-semibold tabular-nums text-zinc-900 dark:text-white/80">
                                            <NumberFlowValue value={amt0} formatter={(v) => formatTokenAmount(v)} /> {sym0}
                                        </div>
                                        <div className="text-[10px] text-zinc-500 dark:text-white/40">
                                            <NumberFlowValue value={amt1} formatter={(v) => formatTokenAmount(v)} /> {sym1}
                                        </div>
                                    </div>
                                    <div className="rounded-xl border border-zinc-200 bg-zinc-50 p-2 dark:border-white/10 dark:bg-[#0f1116]">
                                        <div className="text-[10px] text-zinc-500 dark:text-white/40">可领取手续费 (估算)</div>
                                        <div className={`mt-0.5 font-semibold tabular-nums ${feeTone}`}>
                                            {feeStatus === 'ok'
                                                ? <NumberFlowValue value={feeUsd} formatter={(v) => formatUsd(v)} />
                                                : '--'}
                                        </div>
                                        {feeStatus === 'error' && feeErr ? (
                                            <div className="mt-0.5 text-[10px] text-red-600 dark:text-red-300">{feeErr}</div>
                                        ) : (
                                            <div className="mt-0.5 text-[10px] text-zinc-500 dark:text-white/40">{feeStatus ? `状态: ${feeStatus}` : ''}</div>
                                        )}
                                    </div>
                                </div>
                            </div>
                        );
                    })}
                </div>
            ) : null}
        </BottomSheet>
    );
}
