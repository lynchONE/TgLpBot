import React, { useMemo } from 'react';
import { formatRelativeTime } from '../lib/time';
import { copyToClipboard, hapticNotification, hapticImpact } from '../lib/telegram';

const USD_DISPLAY_LIMIT = 1e15;
const usdFormatter = new Intl.NumberFormat('en-US', {
    style: 'currency',
    currency: 'USD',
    maximumFractionDigits: 2,
});

const compactNumberFormatter = new Intl.NumberFormat('en-US', {
    notation: 'compact',
    maximumFractionDigits: 1,
});

function formatUsd(v) {
    const n = Number(v ?? 0);
    if (!Number.isFinite(n) || Math.abs(n) > USD_DISPLAY_LIMIT) return '$--';
    return usdFormatter.format(n);
}

function formatCompact(v) {
    const n = Number(v ?? 0);
    if (!Number.isFinite(n)) return '--';
    return compactNumberFormatter.format(n);
}

function formatPct(v, digits = 2) {
    const n = Number(v);
    if (!Number.isFinite(n)) return '--';
    return `${n.toFixed(digits)}%`;
}

function formatShare(v) {
    const n = Number(v);
    if (!Number.isFinite(n)) return '--';
    return `${(n * 100).toFixed(1)}%`;
}

function shortHex(value, head = 6, tail = 4) {
    const s = String(value || '').trim();
    if (!s) return '';
    if (s.length <= head + tail + 2) return s;
    return `${s.slice(0, head)}...${s.slice(-tail)}`;
}

function trendBarScale(trend) {
    if (!Array.isArray(trend) || !trend.length) return 1;
    let max = 0;
    for (const point of trend) {
        const total = Number(point?.total_events ?? 0);
        if (Number.isFinite(total) && total > max) max = total;
    }
    return max > 0 ? max : 1;
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

export default function SmartMoneyCard({ overview, loading = false, tick, onNotice }) {
    const pools = Array.isArray(overview?.pools) ? overview.pools : [];
    const wallets = Array.isArray(overview?.wallets_24h) ? overview.wallets_24h : [];
    const warnings = Array.isArray(overview?.warnings) ? overview.warnings : [];
    const summary = overview?.summary || {};
    const histogram = Array.isArray(overview?.pnl_histogram_24h) ? overview.pnl_histogram_24h : [];
    const trend = Array.isArray(overview?.event_trend_24h) ? overview.event_trend_24h : [];

    const updatedAtText = useMemo(
        () => formatRelativeTime(overview?.updated_at, tick) || '--',
        [overview?.updated_at, tick],
    );

    const topWallets = useMemo(() => wallets.slice(0, 20), [wallets]);
    const topPools = useMemo(() => pools.slice(0, 12), [pools]);
    const barMax = useMemo(() => trendBarScale(trend), [trend]);

    return (
        <div className="rounded-2xl border border-zinc-200 bg-white p-4 shadow-sm dark:border-white/10 dark:bg-[#111318] dark:shadow-none">
            <div className="flex items-start justify-between gap-3">
                <div>
                    <div className="text-sm font-extrabold text-zinc-900 dark:text-white/90">
                        Smart Money
                        {loading ? (
                            <span className="ml-2 inline-flex items-center rounded-lg bg-zinc-100 px-2 py-0.5 text-[10px] font-semibold text-zinc-600 dark:bg-white/5 dark:text-white/60">
                                加载中...
                            </span>
                        ) : null}
                    </div>
                    <div className="mt-0.5 text-[11px] text-zinc-500 dark:text-white/40">
                        最近1h池子 {pools.length} 个 · 最近24h钱包 {wallets.length} 个 · 更新 {updatedAtText}
                    </div>
                </div>
                <button
                    type="button"
                    onClick={() => {
                        hapticImpact('light');
                        safeCopy(JSON.stringify(overview || {}), onNotice);
                    }}
                    disabled={!overview}
                    className={`inline-flex items-center rounded-xl px-3 py-2 text-xs font-semibold ring-1 ${overview
                        ? 'bg-white text-zinc-700 ring-zinc-200 hover:bg-zinc-50 dark:bg-white/5 dark:text-white/80 dark:ring-white/10 dark:hover:bg-white/10'
                        : 'cursor-not-allowed bg-zinc-100 text-zinc-400 ring-zinc-200 dark:bg-white/5 dark:text-white/30 dark:ring-white/10'
                        }`}
                >
                    复制JSON
                </button>
            </div>

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

            <div className="mt-3 grid grid-cols-2 gap-2 sm:grid-cols-4">
                <div className="rounded-xl border border-zinc-200 bg-zinc-50 px-3 py-2 dark:border-white/10 dark:bg-[#0f1116]">
                    <div className="text-[10px] text-zinc-500 dark:text-white/40">24h总PnL</div>
                    <div className={`mt-1 text-sm font-extrabold tabular-nums ${kpiTone(summary?.total_pnl_usdt_24h)}`}>
                        {formatUsd(summary?.total_pnl_usdt_24h)}
                    </div>
                </div>
                <div className="rounded-xl border border-zinc-200 bg-zinc-50 px-3 py-2 dark:border-white/10 dark:bg-[#0f1116]">
                    <div className="text-[10px] text-zinc-500 dark:text-white/40">24h In / Out</div>
                    <div className="mt-1 text-sm font-extrabold tabular-nums text-zinc-900 dark:text-white/90">
                        {formatCompact(summary?.total_in_usdt_24h)} / {formatCompact(summary?.total_out_usdt_24h)}
                    </div>
                </div>
                <div className="rounded-xl border border-zinc-200 bg-zinc-50 px-3 py-2 dark:border-white/10 dark:bg-[#0f1116]">
                    <div className="text-[10px] text-zinc-500 dark:text-white/40">钱包胜率</div>
                    <div className="mt-1 text-sm font-extrabold tabular-nums text-zinc-900 dark:text-white/90">
                        {formatShare(summary?.coverage_ratio_24h)}
                    </div>
                    <div className="text-[10px] text-zinc-500 dark:text-white/40">
                        +{Number(summary?.positive_wallets_24h ?? 0)} / -{Number(summary?.negative_wallets_24h ?? 0)} / 0 {Number(summary?.zero_wallets_24h ?? 0)}
                    </div>
                </div>
                <div className="rounded-xl border border-zinc-200 bg-zinc-50 px-3 py-2 dark:border-white/10 dark:bg-[#0f1116]">
                    <div className="text-[10px] text-zinc-500 dark:text-white/40">事件活跃度</div>
                    <div className="mt-1 text-sm font-extrabold tabular-nums text-zinc-900 dark:text-white/90">
                        1h {Number(summary?.total_events_1h ?? 0)} · 24h {Number(summary?.total_events_24h ?? 0)}
                    </div>
                    <div className="text-[10px] text-zinc-500 dark:text-white/40">
                        缺价Token {Number(summary?.missing_price_token_count ?? 0)}
                    </div>
                </div>
            </div>

            <div className="mt-3 grid grid-cols-1 gap-3 lg:grid-cols-2">
                <div className="rounded-xl border border-zinc-200 bg-zinc-50 p-3 dark:border-white/10 dark:bg-[#0f1116]">
                    <div className="flex items-center justify-between">
                        <div className="text-xs font-semibold text-zinc-700 dark:text-white/80">24h事件趋势</div>
                        <div className="text-[11px] text-zinc-500 dark:text-white/40">最近24小时</div>
                    </div>
                    {trend.length ? (
                        <div className="mt-2 grid grid-cols-24 items-end gap-1">
                            {trend.map((point, idx) => {
                                const total = Number(point?.total_events ?? 0);
                                const add = Number(point?.add_events ?? 0);
                                const remove = Number(point?.remove_events ?? 0);
                                const totalPct = Math.max(0.04, Math.min(1, total / barMax));
                                const addPct = total > 0 ? Math.max(0.05, add / total) : 0;
                                const removePct = total > 0 ? Math.max(0.05, remove / total) : 0;
                                return (
                                    <div key={String(idx)} className="group flex h-24 items-end">
                                        <div
                                            className="relative w-full rounded-t bg-zinc-300/70 dark:bg-white/20"
                                            style={{ height: `${Math.round(totalPct * 100)}%` }}
                                            title={`${point?.hours_ago}h ago · add ${add} / remove ${remove} / total ${total}`}
                                        >
                                            {total > 0 ? (
                                                <>
                                                    <div
                                                        className="absolute bottom-0 left-0 right-0 rounded-t bg-emerald-500/80"
                                                        style={{ height: `${Math.round(addPct * 100)}%` }}
                                                    />
                                                    <div
                                                        className="absolute left-0 right-0 rounded-t bg-red-500/70"
                                                        style={{
                                                            bottom: `${Math.round(addPct * 100)}%`,
                                                            height: `${Math.round(removePct * 100)}%`,
                                                        }}
                                                    />
                                                </>
                                            ) : null}
                                        </div>
                                    </div>
                                );
                            })}
                        </div>
                    ) : (
                        <div className="mt-2 rounded-xl border border-zinc-200 bg-white/70 p-3 text-[11px] text-zinc-500 dark:border-white/10 dark:bg-white/5 dark:text-white/60">
                            暂无趋势数据
                        </div>
                    )}
                    <div className="mt-2 flex items-center gap-3 text-[10px] text-zinc-500 dark:text-white/40">
                        <span className="inline-flex items-center gap-1"><i className="h-2 w-2 rounded-full bg-emerald-500/90" />Add</span>
                        <span className="inline-flex items-center gap-1"><i className="h-2 w-2 rounded-full bg-red-500/80" />Remove</span>
                    </div>
                </div>

                <div className="rounded-xl border border-zinc-200 bg-zinc-50 p-3 dark:border-white/10 dark:bg-[#0f1116]">
                    <div className="flex items-center justify-between">
                        <div className="text-xs font-semibold text-zinc-700 dark:text-white/80">24h PnL分布</div>
                        <div className="text-[11px] text-zinc-500 dark:text-white/40">按钱包聚合</div>
                    </div>
                    {histogram.length ? (
                        <div className="mt-2 space-y-1.5">
                            {histogram.map((bucket) => {
                                const share = Number(bucket?.share ?? 0);
                                const walletsInBucket = Number(bucket?.wallets ?? 0);
                                const pnlBucket = Number(bucket?.total_pnl_usdt_24h ?? 0);
                                const tone = pnlBucket > 0
                                    ? 'bg-emerald-500/80'
                                    : pnlBucket < 0
                                        ? 'bg-red-500/80'
                                        : 'bg-zinc-400/70';
                                return (
                                    <div key={String(bucket?.label || '')}>
                                        <div className="mb-0.5 flex items-center justify-between text-[10px] text-zinc-500 dark:text-white/40">
                                            <span>{bucket?.label || '--'}</span>
                                            <span>{walletsInBucket} · {formatShare(share)}</span>
                                        </div>
                                        <div className="h-2 w-full overflow-hidden rounded bg-zinc-200/70 dark:bg-white/10">
                                            <div className={`h-full ${tone}`} style={{ width: `${Math.max(2, Math.round(share * 100))}%` }} />
                                        </div>
                                    </div>
                                );
                            })}
                        </div>
                    ) : (
                        <div className="mt-2 rounded-xl border border-zinc-200 bg-white/70 p-3 text-[11px] text-zinc-500 dark:border-white/10 dark:bg-white/5 dark:text-white/60">
                            暂无分布数据
                        </div>
                    )}
                </div>
            </div>

            <div className="mt-3 grid grid-cols-1 gap-3 xl:grid-cols-2">
                <div className="rounded-xl border border-zinc-200 bg-zinc-50 p-3 dark:border-white/10 dark:bg-[#0f1116]">
                    <div className="mb-2 flex items-center justify-between">
                        <div className="text-xs font-semibold text-zinc-700 dark:text-white/80">最近1h参与池子</div>
                        <div className="text-[11px] text-zinc-500 dark:text-white/40">Top {topPools.length}</div>
                    </div>
                    {topPools.length ? (
                        <div className="overflow-x-auto">
                            <table className="min-w-full text-left text-[11px]">
                                <thead className="text-zinc-500 dark:text-white/40">
                                    <tr>
                                        <th className="pb-1 pr-3 font-medium">池子</th>
                                        <th className="pb-1 pr-3 font-medium">版本/费率</th>
                                        <th className="pb-1 pr-3 text-right font-medium">钱包数</th>
                                        <th className="pb-1 pr-0 text-right font-medium">操作</th>
                                    </tr>
                                </thead>
                                <tbody className="text-zinc-800 dark:text-white/85">
                                    {topPools.map((pool) => {
                                        const poolId = String(pool?.pool_id || '').trim();
                                        const pair = String(pool?.pair || '').trim();
                                        const version = String(pool?.pool_version || '').trim().toUpperCase();
                                        const feePct = Number(pool?.fee_pct);
                                        const walletCount = Number(pool?.wallet_count ?? 0);
                                        return (
                                            <tr key={`${version}:${poolId}`} className="border-t border-zinc-200/70 dark:border-white/10">
                                                <td className="py-1.5 pr-3 font-semibold">{pair || shortHex(poolId, 10, 6) || '--'}</td>
                                                <td className="py-1.5 pr-3 text-zinc-500 dark:text-white/40">
                                                    {version || '--'}
                                                    {Number.isFinite(feePct) && feePct > 0 ? ` · ${formatPct(feePct)}` : ''}
                                                </td>
                                                <td className="py-1.5 pr-3 text-right tabular-nums">{Number.isFinite(walletCount) ? walletCount : '--'}</td>
                                                <td className="py-1.5 pr-0 text-right">
                                                    <button
                                                        type="button"
                                                        onClick={() => {
                                                            hapticImpact('light');
                                                            safeCopy(poolId, onNotice);
                                                        }}
                                                        className="inline-flex items-center rounded-lg bg-zinc-100 px-2 py-1 text-[10px] font-semibold text-zinc-700 hover:bg-zinc-200 dark:bg-white/5 dark:text-white/70 dark:hover:bg-white/10"
                                                    >
                                                        复制
                                                    </button>
                                                </td>
                                            </tr>
                                        );
                                    })}
                                </tbody>
                            </table>
                        </div>
                    ) : (
                        <div className="rounded-xl border border-zinc-200 bg-white/70 p-3 text-[11px] text-zinc-500 dark:border-white/10 dark:bg-white/5 dark:text-white/60">
                            暂无池子数据
                        </div>
                    )}
                </div>

                <div className="rounded-xl border border-zinc-200 bg-zinc-50 p-3 dark:border-white/10 dark:bg-[#0f1116]">
                    <div className="mb-2 flex items-center justify-between">
                        <div className="text-xs font-semibold text-zinc-700 dark:text-white/80">最近24h钱包盈亏</div>
                        <div className="text-[11px] text-zinc-500 dark:text-white/40">Top {topWallets.length}</div>
                    </div>
                    {topWallets.length ? (
                        <div className="overflow-x-auto">
                            <table className="min-w-full text-left text-[11px]">
                                <thead className="text-zinc-500 dark:text-white/40">
                                    <tr>
                                        <th className="pb-1 pr-3 font-medium">钱包</th>
                                        <th className="pb-1 pr-3 text-right font-medium">In</th>
                                        <th className="pb-1 pr-3 text-right font-medium">Out</th>
                                        <th className="pb-1 pr-3 text-right font-medium">PnL</th>
                                        <th className="pb-1 pr-3 text-right font-medium">1h/24h</th>
                                        <th className="pb-1 pr-0 text-right font-medium">操作</th>
                                    </tr>
                                </thead>
                                <tbody className="text-zinc-800 dark:text-white/85">
                                    {topWallets.map((wallet) => {
                                        const addr = String(wallet?.wallet_address || '').trim();
                                        const inUsd = Number(wallet?.in_usdt_24h ?? 0);
                                        const outUsd = Number(wallet?.out_usdt_24h ?? 0);
                                        const pnl = Number(wallet?.pnl_usdt_24h ?? 0);
                                        const margin = Number(wallet?.pnl_margin_pct ?? 0);
                                        const cnt1h = Number(wallet?.event_count_1h ?? 0);
                                        const cnt24h = Number(wallet?.event_count_24h ?? 0);
                                        const pnlTone = kpiTone(pnl);
                                        return (
                                            <tr key={addr} className="border-t border-zinc-200/70 dark:border-white/10">
                                                <td className="py-1.5 pr-3 font-semibold">{shortHex(addr, 10, 8) || '--'}</td>
                                                <td className="py-1.5 pr-3 text-right tabular-nums">{formatUsd(inUsd)}</td>
                                                <td className="py-1.5 pr-3 text-right tabular-nums">{formatUsd(outUsd)}</td>
                                                <td className={`py-1.5 pr-3 text-right tabular-nums font-semibold ${pnlTone}`}>
                                                    {formatUsd(pnl)}
                                                    <span className="ml-1 text-[10px] opacity-70">({formatPct(margin)})</span>
                                                </td>
                                                <td className="py-1.5 pr-3 text-right tabular-nums text-zinc-500 dark:text-white/40">
                                                    {Number.isFinite(cnt1h) ? cnt1h : '--'} / {Number.isFinite(cnt24h) ? cnt24h : '--'}
                                                </td>
                                                <td className="py-1.5 pr-0 text-right">
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
                                                </td>
                                            </tr>
                                        );
                                    })}
                                </tbody>
                            </table>
                        </div>
                    ) : (
                        <div className="rounded-xl border border-zinc-200 bg-white/70 p-3 text-[11px] text-zinc-500 dark:border-white/10 dark:bg-white/5 dark:text-white/60">
                            暂无钱包数据
                        </div>
                    )}
                </div>
            </div>
        </div>
    );
}

