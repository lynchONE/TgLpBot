import React, { useEffect, useMemo, useRef, useState } from 'react';
import { createChart, HistogramSeries } from 'lightweight-charts';
import { fetchSmartMoney24hPoolAdds } from '../lib/api';
import { copyToClipboard, hapticImpact, hapticNotification } from '../lib/telegram';

const usdFormatter = new Intl.NumberFormat('en-US', {
    style: 'currency',
    currency: 'USD',
    maximumFractionDigits: 0,
});

function formatUsd(v) {
    const n = Number(v ?? 0);
    if (!Number.isFinite(n)) return '--';
    return usdFormatter.format(n);
}

function formatUsdCompact(v) {
    const n = Number(v ?? 0);
    if (!Number.isFinite(n)) return '--';
    if (n >= 1e6) return `$${(n / 1e6).toFixed(2)}M`;
    if (n >= 1e3) return `$${(n / 1e3).toFixed(1)}K`;
    return formatUsd(n);
}

function shortHex(value, head = 6, tail = 4) {
    const s = String(value || '').trim();
    if (!s) return '';
    if (s.length <= head + tail + 2) return s;
    return `${s.slice(0, head)}...${s.slice(-tail)}`;
}

function HourlyTrendChart({ trend, theme = 'dark' }) {
    const containerRef = useRef(null);
    const chartRef = useRef(null);
    const seriesRef = useRef(null);
    const resizeRef = useRef(null);

    const data = useMemo(() => {
        if (!Array.isArray(trend)) return [];
        return trend
            .map((row) => {
                const hour = String(row?.hour || '');
                if (!hour) return null;
                const ts = Math.floor(new Date(hour).getTime() / 1000);
                if (!Number.isFinite(ts) || ts <= 0) return null;
                return {
                    time: ts,
                    value: Number(row?.add_count ?? 0),
                };
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
                const ro = new ResizeObserver(() => {
                    chart.applyOptions({ width: el.clientWidth || 320 });
                });
                ro.observe(el);
                resizeRef.current = ro;
            }
        } catch {}

        return () => {
            if (resizeRef.current) {
                try { resizeRef.current.disconnect(); } catch {}
            }
            if (chartRef.current) {
                try { chartRef.current.remove(); } catch {}
            }
            resizeRef.current = null;
            chartRef.current = null;
            seriesRef.current = null;
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

function DistributionBars({ title, items, colorClass = 'bg-emerald-500' }) {
    const maxCount = useMemo(() => {
        if (!Array.isArray(items) || items.length === 0) return 1;
        return Math.max(1, ...items.map((it) => Number(it?.count ?? 0)));
    }, [items]);

    return (
        <div className="rounded-xl border border-zinc-200 bg-zinc-50 p-3 dark:border-white/10 dark:bg-white/[0.02]">
            <div className="mb-2 text-xs font-semibold text-zinc-700 dark:text-white/80">{title}</div>
            {!Array.isArray(items) || items.length === 0 ? (
                <div className="text-[10px] text-zinc-400 dark:text-white/30">暂无数据</div>
            ) : (
                <div className="space-y-1.5">
                    {items.map((item, idx) => {
                        const count = Number(item?.count ?? 0);
                        const pct = maxCount > 0 ? (count / maxCount) * 100 : 0;
                        return (
                            <div key={`${item?.range || idx}`} className="flex items-center gap-2">
                                <div className="w-24 shrink-0 text-right text-[10px] font-medium text-zinc-600 dark:text-white/60">
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
            )}
        </div>
    );
}

export default function SmartMoney24hPoolAddsCard({ apiBaseUrl, initData, chain, onNotice, onQuickOpenPool }) {
    const [data, setData] = useState(null);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState('');
    const [refreshTick, setRefreshTick] = useState(0);
    const [windowHours, setWindowHours] = useState(24);
    const [poolLimit, setPoolLimit] = useState(30);

    useEffect(() => {
        if (!initData) {
            setLoading(false);
            setError('缺少 initData，无法加载 24h 加池数据');
            return;
        }
        let cancelled = false;
        const ac = new AbortController();
        const timeout = setTimeout(() => {
            try {
                ac.abort();
            } catch {
                // ignore
            }
        }, 15000);

        setLoading(true);
        setError('');
        fetchSmartMoney24hPoolAdds({
            apiBaseUrl,
            initData,
            chain,
            windowHours,
            poolLimit,
            topWalletLimit: 20,
            signal: ac.signal,
        })
            .then((resp) => {
                if (!cancelled) setData(resp || null);
            })
            .catch((err) => {
                if (cancelled) return;
                const message = String(err?.message || err || '');
                if (message.toLowerCase().includes('aborted') || message.toLowerCase().includes('abort')) {
                    setError('请求超时，请点“刷新”重试');
                    return;
                }
                setError(message || '加载失败');
            })
            .finally(() => {
                if (!cancelled) setLoading(false);
            });

        return () => {
            cancelled = true;
            clearTimeout(timeout);
            ac.abort();
        };
    }, [apiBaseUrl, initData, chain, windowHours, poolLimit, refreshTick]);

    const pools = useMemo(() => (Array.isArray(data?.pools) ? data.pools : []), [data]);
    const hourlyTrend = useMemo(() => (Array.isArray(data?.hourly_trend) ? data.hourly_trend : []), [data]);
    const tickRangeDist = useMemo(() => (Array.isArray(data?.tick_range_distribution) ? data.tick_range_distribution : []), [data]);
    const poolAmountDist = useMemo(() => (Array.isArray(data?.pool_amount_distribution) ? data.pool_amount_distribution : []), [data]);
    const walletPoolDist = useMemo(() => (Array.isArray(data?.wallet_pool_distribution) ? data.wallet_pool_distribution : []), [data]);
    const topWallets = useMemo(() => (Array.isArray(data?.top_wallets) ? data.top_wallets : []), [data]);
    const warnings = useMemo(() => (Array.isArray(data?.warnings) ? data.warnings : []), [data]);

    const maxWallets = useMemo(() => {
        if (!pools.length) return 1;
        return Math.max(1, ...pools.map((pool) => Number(pool?.wallet_count ?? 0)));
    }, [pools]);

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
            <div className="flex flex-wrap items-center justify-between gap-2">
                <div className="inline-flex items-center gap-1 rounded-lg bg-zinc-100 p-1 dark:bg-white/5">
                    {[12, 24, 48, 72].map((h) => (
                        <button
                            key={h}
                            type="button"
                            onClick={() => {
                                hapticImpact('light');
                                setWindowHours(h);
                            }}
                            className={`rounded-md px-2 py-1 text-[10px] font-semibold transition ${
                                windowHours === h
                                    ? 'bg-emerald-500 text-white'
                                    : 'text-zinc-600 hover:bg-zinc-200 dark:text-white/70 dark:hover:bg-white/10'
                            }`}
                        >
                            {h}h
                        </button>
                    ))}
                </div>
                <div className="inline-flex items-center gap-2">
                    <select
                        value={poolLimit}
                        onChange={(e) => setPoolLimit(Number(e.target.value))}
                        className="rounded-lg border border-zinc-200 bg-white px-2 py-1 text-[10px] font-semibold text-zinc-700 outline-none dark:border-white/10 dark:bg-white/5 dark:text-white/70"
                    >
                        <option value={20}>Top 20 pools</option>
                        <option value={30}>Top 30 pools</option>
                        <option value={50}>Top 50 pools</option>
                    </select>
                    <button
                        type="button"
                        onClick={() => {
                            hapticImpact('light');
                            setRefreshTick((v) => v + 1);
                        }}
                        className="rounded-lg bg-zinc-100 px-2 py-1 text-[10px] font-semibold text-zinc-700 hover:bg-zinc-200 dark:bg-white/5 dark:text-white/70 dark:hover:bg-white/10"
                    >
                        刷新
                    </button>
                </div>
            </div>

            <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
                <div className="rounded-xl border border-zinc-200 bg-zinc-50 p-2.5 text-center dark:border-white/10 dark:bg-white/[0.02]">
                    <div className="text-lg font-bold tabular-nums text-emerald-600 dark:text-emerald-400">
                        {Number(data?.total_pools ?? pools.length)}
                    </div>
                    <div className="text-[10px] text-zinc-500 dark:text-white/40">池子数</div>
                </div>
                <div className="rounded-xl border border-zinc-200 bg-zinc-50 p-2.5 text-center dark:border-white/10 dark:bg-white/[0.02]">
                    <div className="text-lg font-bold tabular-nums text-blue-600 dark:text-blue-400">
                        {Number(data?.total_wallets ?? 0)}
                    </div>
                    <div className="text-[10px] text-zinc-500 dark:text-white/40">参与钱包</div>
                </div>
                <div className="rounded-xl border border-zinc-200 bg-zinc-50 p-2.5 text-center dark:border-white/10 dark:bg-white/[0.02]">
                    <div className="text-lg font-bold tabular-nums text-amber-600 dark:text-amber-400">
                        {Number(data?.total_events ?? 0)}
                    </div>
                    <div className="text-[10px] text-zinc-500 dark:text-white/40">加池事件</div>
                </div>
                <div className="rounded-xl border border-zinc-200 bg-zinc-50 p-2.5 text-center dark:border-white/10 dark:bg-white/[0.02]">
                    <div className="text-lg font-bold tabular-nums text-fuchsia-600 dark:text-fuchsia-400">
                        {formatUsdCompact(data?.total_amount_usd ?? 0)}
                    </div>
                    <div className="text-[10px] text-zinc-500 dark:text-white/40">总金额</div>
                </div>
            </div>

            {hourlyTrend.length > 0 && (
                <div className="rounded-xl border border-zinc-200 bg-zinc-50 p-2 dark:border-white/10 dark:bg-white/[0.02]">
                    <div className="mb-1 text-xs font-semibold text-zinc-700 dark:text-white/80">每小时加池趋势</div>
                    <HourlyTrendChart trend={hourlyTrend} theme="dark" />
                </div>
            )}

            <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                <DistributionBars title="池子金额区间分布 (USD)" items={poolAmountDist} colorClass="bg-amber-500" />
                <DistributionBars title="钱包加池数量分布" items={walletPoolDist} colorClass="bg-emerald-500" />
            </div>

            {tickRangeDist.length > 0 && (
                <DistributionBars title="区间宽度分布 (ticks)" items={tickRangeDist} colorClass="bg-blue-500" />
            )}

            <div className="grid grid-cols-1 gap-3 lg:grid-cols-2">
                <div className="rounded-xl border border-zinc-200 bg-zinc-50 p-3 dark:border-white/10 dark:bg-white/[0.02]">
                    <div className="mb-2 text-xs font-semibold text-zinc-700 dark:text-white/80">钱包加池排行</div>
                    {topWallets.length === 0 ? (
                        <div className="text-[11px] text-zinc-400 dark:text-white/30">暂无数据</div>
                    ) : (
                        <div className="space-y-1.5">
                            {topWallets.slice(0, 12).map((row, idx) => {
                                const addr = String(row?.wallet_address || '').trim();
                                return (
                                    <div key={`${addr || idx}`} className="flex items-center justify-between rounded-lg border border-zinc-200 bg-white px-2 py-1.5 dark:border-white/10 dark:bg-white/5">
                                        <div className="min-w-0">
                                            <button
                                                type="button"
                                                onClick={() => {
                                                    copyToClipboard(addr);
                                                    hapticNotification('success');
                                                    if (onNotice) onNotice('已复制钱包地址');
                                                }}
                                                className="font-mono text-[11px] font-semibold text-zinc-800 hover:text-emerald-600 dark:text-white/80 dark:hover:text-emerald-300"
                                                title={addr}
                                            >
                                                #{idx + 1} {shortHex(addr, 8, 6)}
                                            </button>
                                        </div>
                                        <div className="text-[10px] text-zinc-600 dark:text-white/60">
                                            <span className="font-semibold text-emerald-600 dark:text-emerald-300">{Number(row?.pool_count ?? 0)} 池</span>
                                            <span className="mx-1 text-zinc-400 dark:text-white/30">/</span>
                                            <span>{Number(row?.add_count ?? 0)} 次</span>
                                        </div>
                                    </div>
                                );
                            })}
                        </div>
                    )}
                </div>

                <div className="rounded-xl border border-zinc-200 bg-zinc-50 p-3 dark:border-white/10 dark:bg-white/[0.02]">
                    <div className="mb-2 flex items-center justify-between">
                        <div className="text-xs font-semibold text-zinc-700 dark:text-white/80">池子排行</div>
                        <div className="text-[10px] text-zinc-400 dark:text-white/30">{pools.length} 个</div>
                    </div>

                    {pools.length === 0 ? (
                        <div className="text-[11px] text-zinc-400 dark:text-white/30">暂无加池记录</div>
                    ) : (
                        <div className="space-y-2">
                            {pools.slice(0, 12).map((pool, idx) => {
                                const poolId = String(pool?.pool_id || '').trim();
                                const pair = String(pool?.pair || '').trim();
                                const version = String(pool?.pool_version || '').trim().toUpperCase();
                                const walletCount = Number(pool?.wallet_count ?? 0);
                                const eventCount = Number(pool?.event_count ?? 0);
                                const totalUsd = Number(pool?.total_amount_usd ?? 0);
                                const barPct = maxWallets > 0 ? (walletCount / maxWallets) * 100 : 0;

                                return (
                                    <div key={`${poolId || idx}`} className="relative overflow-hidden rounded-lg border border-zinc-200 bg-white p-2.5 dark:border-white/10 dark:bg-white/5">
                                        <div
                                            className="absolute inset-y-0 left-0 bg-emerald-500/[0.07] dark:bg-emerald-500/[0.05]"
                                            style={{ width: `${Math.max(3, barPct)}%` }}
                                        />
                                        <div className="relative">
                                            <div className="flex items-center justify-between gap-2">
                                                <div className="min-w-0">
                                                    <div className="truncate text-[12px] font-bold text-zinc-900 dark:text-white/90">
                                                        #{idx + 1} {pair || shortHex(poolId)}
                                                    </div>
                                                    <div className="mt-0.5 text-[10px] text-zinc-500 dark:text-white/40">
                                                        {version || '--'} · {walletCount} 钱包 · {eventCount} 次
                                                    </div>
                                                </div>
                                                <div className="shrink-0 text-right">
                                                    <div className="text-[11px] font-semibold tabular-nums text-amber-600 dark:text-amber-300">
                                                        {formatUsdCompact(totalUsd)}
                                                    </div>
                                                    <button
                                                        type="button"
                                                        onClick={() => {
                                                            copyToClipboard(poolId);
                                                            hapticNotification('success');
                                                            if (onNotice) onNotice('已复制池子地址');
                                                        }}
                                                        className="mt-0.5 font-mono text-[9px] text-zinc-400 hover:text-emerald-600 dark:text-white/30 dark:hover:text-emerald-300"
                                                    >
                                                        {shortHex(poolId, 8, 6)}
                                                    </button>
                                                </div>
                                            </div>
                                            <div className="mt-2 flex justify-end">
                                                <button
                                                    type="button"
                                                    onClick={() => {
                                                        hapticImpact('light');
                                                        if (typeof onQuickOpenPool === 'function') {
                                                            onQuickOpenPool({
                                                                pool_address: poolId,
                                                                pool_id: poolId,
                                                                protocol_version: String(pool?.pool_version || '').trim().toLowerCase(),
                                                                trading_pair: pair,
                                                                chain,
                                                                smartMoneyWallets: [],
                                                            });
                                                        }
                                                    }}
                                                    disabled={!poolId}
                                                    className="rounded-lg bg-blue-500/15 px-2 py-1 text-[10px] font-semibold text-blue-600 hover:bg-blue-500/20 disabled:opacity-50 dark:text-blue-300"
                                                >
                                                    快速开仓
                                                </button>
                                            </div>
                                        </div>
                                    </div>
                                );
                            })}
                        </div>
                    )}
                </div>
            </div>

            {warnings.length > 0 && (
                <div className="rounded-xl border border-amber-500/30 bg-amber-500/10 p-2.5 text-[10px] text-amber-700 dark:border-amber-500/20 dark:bg-amber-500/5 dark:text-amber-200">
                    {warnings.slice(0, 3).map((w, i) => (
                        <div key={i}>{String(w)}</div>
                    ))}
                </div>
            )}
        </div>
    );
}
