import React, { useEffect, useMemo, useRef, useState } from 'react';
import { createChart, HistogramSeries } from 'lightweight-charts';
import { fetchSmartMoney24hPoolAdds } from '../lib/api';
import { copyToClipboard, hapticImpact, hapticNotification } from '../lib/telegram';

const usdFormatter = new Intl.NumberFormat('en-US', {
    style: 'currency',
    currency: 'USD',
    maximumFractionDigits: 0,
});
function formatUsdShort(v) {
    const n = Number(v ?? 0);
    if (!Number.isFinite(n)) return '--';
    if (n >= 1e6) return `$${(n / 1e6).toFixed(1)}M`;
    if (n >= 1e3) return `$${(n / 1e3).toFixed(1)}K`;
    return usdFormatter.format(n);
}

function shortHex(value, head = 6, tail = 4) {
    const s = String(value || '').trim();
    if (!s) return '';
    if (s.length <= head + tail + 2) return s;
    return `${s.slice(0, head)}...${s.slice(-tail)}`;
}

// Hourly trend chart subcomponent
function HourlyTrendChart({ trend, theme = 'dark' }) {
    const containerRef = useRef(null);
    const chartRef = useRef(null);
    const seriesRef = useRef(null);
    const resizeRef = useRef(null);

    const data = useMemo(() => {
        if (!Array.isArray(trend)) return [];
        return trend
            .map((p) => {
                const hourStr = String(p?.hour || '');
                if (!hourStr) return null;
                const ts = Math.floor(new Date(hourStr).getTime() / 1000);
                if (!Number.isFinite(ts) || ts <= 0) return null;
                return { time: ts, value: Number(p?.add_count ?? 0) };
            })
            .filter(Boolean)
            .sort((a, b) => a.time - b.time);
    }, [trend]);

    useEffect(() => {
        const el = containerRef.current;
        if (!el) return;

        const isDark = theme === 'dark';
        const gridColor = isDark ? 'rgba(255,255,255,0.06)' : 'rgba(0,0,0,0.06)';
        const borderColor = isDark ? 'rgba(255,255,255,0.12)' : 'rgba(0,0,0,0.12)';
        const textColor = isDark ? 'rgba(255,255,255,0.82)' : '#27272a';

        if (resizeRef.current) {
            try { resizeRef.current.disconnect(); } catch {}
            resizeRef.current = null;
        }
        if (chartRef.current) {
            try { chartRef.current.remove(); } catch {}
            chartRef.current = null;
            seriesRef.current = null;
        }

        try {
            const chart = createChart(el, {
                width: el.clientWidth || 320,
                height: 180,
                layout: { background: { type: 'solid', color: 'transparent' }, textColor },
                grid: { vertLines: { color: gridColor }, horzLines: { color: gridColor } },
                rightPriceScale: { borderColor, scaleMargins: { top: 0.15, bottom: 0.05 } },
                timeScale: { borderColor, timeVisible: true, secondsVisible: false },
                crosshair: { mode: 0 },
            });

            const series = chart.addSeries(HistogramSeries, {
                color: isDark ? 'rgba(16,185,129,0.75)' : 'rgba(5,150,105,0.75)',
                base: 0,
                priceLineVisible: false,
            });

            chartRef.current = chart;
            seriesRef.current = series;

            if (typeof ResizeObserver !== 'undefined') {
                const ro = new ResizeObserver(() => chart.applyOptions({ width: el.clientWidth || 320 }));
                ro.observe(el);
                resizeRef.current = ro;
            }
        } catch {}

        return () => {
            if (resizeRef.current) { try { resizeRef.current.disconnect(); } catch {} }
            if (chartRef.current) { try { chartRef.current.remove(); } catch {} }
            chartRef.current = null;
            seriesRef.current = null;
            resizeRef.current = null;
        };
    }, [theme]);

    useEffect(() => {
        const chart = chartRef.current;
        const series = seriesRef.current;
        if (!chart || !series || data.length === 0) return;
        try {
            series.setData(data);
            chart.timeScale().fitContent();
        } catch {}
    }, [data]);

    return <div ref={containerRef} className="h-[180px] w-full" />;
}

// Distribution bar chart (CSS-based, not lightweight-charts)
function DistributionBars({ items, colorClass = 'bg-emerald-500' }) {
    const maxCount = useMemo(() => {
        if (!Array.isArray(items) || items.length === 0) return 1;
        return Math.max(1, ...items.map((i) => Number(i?.count ?? 0)));
    }, [items]);

    if (!Array.isArray(items) || items.length === 0) {
        return <div className="text-[10px] text-zinc-400 dark:text-white/30">暂无数据</div>;
    }

    return (
        <div className="space-y-1.5">
            {items.map((item, idx) => {
                const count = Number(item?.count ?? 0);
                const pct = maxCount > 0 ? (count / maxCount) * 100 : 0;
                return (
                    <div key={idx} className="flex items-center gap-2">
                        <div className="w-28 shrink-0 text-right text-[10px] font-medium text-zinc-600 dark:text-white/60">
                            {String(item?.range || '--')}
                        </div>
                        <div className="flex-1">
                            <div className="h-4 overflow-hidden rounded-md bg-zinc-100 dark:bg-white/5">
                                <div
                                    className={`h-full rounded-md ${colorClass} transition-all duration-500`}
                                    style={{ width: `${Math.max(2, pct)}%` }}
                                />
                            </div>
                        </div>
                        <div className="w-8 text-right text-[10px] font-semibold tabular-nums text-zinc-700 dark:text-white/70">
                            {count}
                        </div>
                    </div>
                );
            })}
        </div>
    );
}

export default function SmartMoney24hPoolAddsCard({ apiBaseUrl, initData, chain, onNotice, onQuickOpenPool }) {
    const [data, setData] = useState(null);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState('');

    useEffect(() => {
        if (!apiBaseUrl || !initData) return;
        let cancelled = false;
        const ac = new AbortController();

        setLoading(true);
        setError('');
        fetchSmartMoney24hPoolAdds({ apiBaseUrl, initData, chain, signal: ac.signal })
            .then((resp) => {
                if (cancelled) return;
                setData(resp);
            })
            .catch((err) => {
                if (cancelled) return;
                setError(String(err?.message || err || '加载失败'));
            })
            .finally(() => {
                if (!cancelled) setLoading(false);
            });

        return () => {
            cancelled = true;
            ac.abort();
        };
    }, [apiBaseUrl, initData, chain]);

    const pools = useMemo(() => Array.isArray(data?.pools) ? data.pools : [], [data]);
    const hourlyTrend = useMemo(() => Array.isArray(data?.hourly_trend) ? data.hourly_trend : [], [data]);
    const tickRangeDist = useMemo(() => Array.isArray(data?.tick_range_distribution) ? data.tick_range_distribution : [], [data]);
    const maxWallets = useMemo(() => Math.max(1, ...pools.map((p) => Number(p?.wallet_count ?? 0))), [pools]);

    if (loading) {
        return (
            <div className="mt-3 space-y-2">
                {[1, 2, 3].map((i) => (
                    <div key={i} className="h-16 animate-pulse rounded-xl bg-zinc-100 dark:bg-white/5" />
                ))}
            </div>
        );
    }

    if (error) {
        return (
            <div className="mt-3 rounded-xl border border-red-500/30 bg-red-500/10 p-3 text-[11px] text-red-700 dark:border-red-500/20 dark:bg-red-500/5 dark:text-red-200">
                {error}
            </div>
        );
    }

    return (
        <div className="mt-3 space-y-4">
            {/* Summary */}
            <div className="grid grid-cols-3 gap-2">
                <div className="rounded-xl border border-zinc-200 bg-zinc-50 p-2.5 text-center dark:border-white/10 dark:bg-white/[0.02]">
                    <div className="text-lg font-bold tabular-nums text-emerald-600 dark:text-emerald-400">
                        {data?.total_pools ?? 0}
                    </div>
                    <div className="text-[10px] text-zinc-500 dark:text-white/40">池子数</div>
                </div>
                <div className="rounded-xl border border-zinc-200 bg-zinc-50 p-2.5 text-center dark:border-white/10 dark:bg-white/[0.02]">
                    <div className="text-lg font-bold tabular-nums text-blue-600 dark:text-blue-400">
                        {data?.total_wallets ?? 0}
                    </div>
                    <div className="text-[10px] text-zinc-500 dark:text-white/40">参与钱包</div>
                </div>
                <div className="rounded-xl border border-zinc-200 bg-zinc-50 p-2.5 text-center dark:border-white/10 dark:bg-white/[0.02]">
                    <div className="text-lg font-bold tabular-nums text-amber-600 dark:text-amber-400">
                        {data?.total_events ?? 0}
                    </div>
                    <div className="text-[10px] text-zinc-500 dark:text-white/40">加池事件</div>
                </div>
            </div>

            {/* Hourly Trend Chart */}
            {hourlyTrend.length > 0 && (
                <div>
                    <div className="mb-1 text-xs font-semibold text-zinc-700 dark:text-white/80">
                        每小时加池趋势
                    </div>
                    <div className="rounded-xl border border-zinc-200 bg-zinc-50 p-2 dark:border-white/10 dark:bg-white/[0.02]">
                        <HourlyTrendChart trend={hourlyTrend} theme="dark" />
                    </div>
                </div>
            )}

            {/* Pool Ranking */}
            <div>
                <div className="mb-2 flex items-center justify-between">
                    <div className="text-xs font-semibold text-zinc-700 dark:text-white/80">
                        池子排行 ({data?.window_hours ?? 24}h)
                    </div>
                    <span className="text-[10px] text-zinc-400 dark:text-white/30">{pools.length} 个</span>
                </div>

                {pools.length === 0 ? (
                    <div className="rounded-xl border border-dashed border-zinc-300 bg-zinc-50 p-4 text-center text-[11px] text-zinc-400 dark:border-white/10 dark:bg-white/[0.02] dark:text-white/30">
                        暂无加池记录
                    </div>
                ) : (
                    <div className="space-y-2">
                        {pools.map((pool, idx) => {
                            const poolId = String(pool?.pool_id || '').trim();
                            const pair = String(pool?.pair || '').trim();
                            const version = String(pool?.pool_version || '').toUpperCase();
                            const feePct = Number(pool?.fee_pct ?? 0);
                            const walletCount = Number(pool?.wallet_count ?? 0);
                            const eventCount = Number(pool?.event_count ?? 0);
                            const barPct = maxWallets > 0 ? (walletCount / maxWallets) * 100 : 0;

                            return (
                                <div
                                    key={poolId || idx}
                                    className="relative overflow-hidden rounded-xl border border-zinc-200 bg-white p-3 dark:border-white/10 dark:bg-white/[0.02]"
                                >
                                    {/* Background bar */}
                                    <div
                                        className="absolute inset-y-0 left-0 bg-emerald-500/[0.06] dark:bg-emerald-500/[0.04]"
                                        style={{ width: `${Math.max(3, barPct)}%` }}
                                    />

                                    <div className="relative">
                                        {/* Row 1: Rank + Pair + Version */}
                                        <div className="flex items-center gap-2">
                                            <span className="flex h-5 w-5 items-center justify-center rounded-md bg-emerald-500/10 text-[10px] font-bold text-emerald-700 dark:bg-emerald-500/15 dark:text-emerald-300">
                                                {idx + 1}
                                            </span>
                                            <span className="text-[12px] font-bold text-zinc-900 dark:text-white/90">
                                                {pair || shortHex(poolId)}
                                            </span>
                                            {version && (
                                                <span className="rounded bg-zinc-100 px-1 py-0.5 text-[9px] font-semibold text-zinc-500 dark:bg-white/10 dark:text-white/50">
                                                    {version}
                                                </span>
                                            )}
                                            {feePct > 0 && (
                                                <span className="text-[10px] text-zinc-400 dark:text-white/35">
                                                    {feePct.toFixed(4)}%
                                                </span>
                                            )}
                                        </div>

                                        {/* Row 2: Stats */}
                                        <div className="mt-1.5 flex items-center gap-3 text-[10px]">
                                            <span className="font-semibold text-emerald-600 dark:text-emerald-400">
                                                {walletCount} 钱包
                                            </span>
                                            <span className="text-zinc-500 dark:text-white/40">
                                                {eventCount} 次加池
                                            </span>
                                            {pool?.exchange && (
                                                <span className="text-zinc-400 dark:text-white/30">
                                                    {pool.exchange}
                                                </span>
                                            )}
                                        </div>

                                        {/* Row 3: Pool ID (copyable) */}
                                        <button
                                            type="button"
                                            onClick={() => {
                                                copyToClipboard(poolId);
                                                hapticNotification('success');
                                                if (onNotice) onNotice('已复制池子地址');
                                            }}
                                            className="mt-1 font-mono text-[9px] text-zinc-400 hover:text-emerald-600 dark:text-white/25 dark:hover:text-emerald-400"
                                        >
                                            {shortHex(poolId, 10, 6)}
                                        </button>
                                    </div>
                                </div>
                            );
                        })}
                    </div>
                )}
            </div>

            {/* Tick Range Distribution */}
            {tickRangeDist.length > 0 && (
                <div>
                    <div className="mb-2 text-xs font-semibold text-zinc-700 dark:text-white/80">
                        区间宽度分布
                    </div>
                    <div className="rounded-xl border border-zinc-200 bg-zinc-50 p-3 dark:border-white/10 dark:bg-white/[0.02]">
                        <DistributionBars items={tickRangeDist} colorClass="bg-blue-500" />
                    </div>
                </div>
            )}
        </div>
    );
}
