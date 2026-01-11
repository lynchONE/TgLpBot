import React, { useEffect, useMemo, useRef, useState } from 'react';
import { createChart, createSeriesMarkers, LineSeries, LineStyle } from 'lightweight-charts';

const USD_DISPLAY_LIMIT = 1e15;
const usdFormatter = new Intl.NumberFormat('en-US', {
    style: 'currency',
    currency: 'USD',
    maximumFractionDigits: 2,
});

function formatUsd(v) {
    const n = Number(v ?? 0);
    if (!Number.isFinite(n) || Math.abs(n) > USD_DISPLAY_LIMIT) return '$--';
    return usdFormatter.format(n);
}

function formatEventTime(t) {
    const n = Number(t);
    if (!Number.isFinite(n) || n <= 0) return '--';
    const d = new Date(n * 1000);
    const mm = String(d.getMonth() + 1).padStart(2, '0');
    const dd = String(d.getDate()).padStart(2, '0');
    const hh = String(d.getHours()).padStart(2, '0');
    const mi = String(d.getMinutes()).padStart(2, '0');
    return `${mm}-${dd} ${hh}:${mi}`;
}

function normalizeSeries(series) {
    if (!Array.isArray(series)) return [];
    return series
        .map((p) => ({
            time: Number(p?.t || 0),
            value: Number(p?.value || 0),
        }))
        .filter((p) => Number.isFinite(p.time) && p.time > 0 && Number.isFinite(p.value));
}

export default function AutoPnLCurveCard({ data, loading, error, theme = 'dark' }) {
    const containerRef = useRef(null);
    const chartRef = useRef(null);
    const realizedSeriesRef = useRef(null);
    const totalSeriesRef = useRef(null);
    const markersRef = useRef(null);
    const resizeRef = useRef(null);

    const [showAllEvents, setShowAllEvents] = useState(false);
    const [chartError, setChartError] = useState('');
    const [chartNonce, setChartNonce] = useState(0);

    const windowLabel = String(data?.window_label || '').trim();
    const realizedProfit = Number(data?.realized_profit_usdt ?? 0);
    const unrealizedProfit = Number(data?.unrealized_profit_usdt ?? 0);
    const totalProfit = Number(data?.total_profit_usdt ?? 0);

    const realizedSeries = useMemo(() => normalizeSeries(data?.series_realized), [data?.series_realized]);
    const totalSeries = useMemo(() => normalizeSeries(data?.series_total), [data?.series_total]);

    const markers = useMemo(() => {
        const events = Array.isArray(data?.events) ? data.events : [];
        const out = [];
        for (const e of events) {
            const type = String(e?.type || '').trim();
            const t = Number(e?.t || 0);
            if (!Number.isFinite(t) || t <= 0) continue;

            const pair = String(e?.pair || '').trim();
            if (type === 'open') {
                const openUSDT = Number(e?.open_usdt ?? 0);
                const text = pair ? `${pair} · 开仓 ${formatUsd(openUSDT)}` : `开仓 ${formatUsd(openUSDT)}`;
                out.push({
                    time: t,
                    position: 'belowBar',
                    color: '#60a5fa',
                    shape: 'arrowUp',
                    text,
                });
                continue;
            }
            if (type === 'close') {
                const profit = Number(e?.profit_usdt ?? 0);
                const isProfit = Number.isFinite(profit) && profit >= 0;
                const textProfit = Number.isFinite(profit) ? `${isProfit ? '+' : '-'}${formatUsd(Math.abs(profit))}` : '--';
                const text = pair ? `${pair} · 平仓 ${textProfit}` : `平仓 ${textProfit}`;
                out.push({
                    time: t,
                    position: 'aboveBar',
                    color: isProfit ? '#10b981' : '#ef4444',
                    shape: 'arrowDown',
                    text,
                });
            }
        }
        return out;
    }, [data?.events]);

    const collapseLimit = 20;

    const allEvents = useMemo(() => {
        const events = Array.isArray(data?.events) ? data.events : [];
        return events
            .map((e) => ({
                type: String(e?.type || '').trim(),
                t: Number(e?.t || 0),
                pair: String(e?.pair || '').trim(),
                openUSDT: Number(e?.open_usdt ?? 0),
                profitUSDT: Number(e?.profit_usdt ?? 0),
            }))
            .filter((e) => (e.type === 'open' || e.type === 'close') && Number.isFinite(e.t) && e.t > 0)
            .sort((a, b) => b.t - a.t);
    }, [data?.events]);

    const eventsForList = useMemo(() => {
        if (!showAllEvents) return [];
        return allEvents.slice(0, collapseLimit);
    }, [allEvents, showAllEvents]);

    const hasEvents = allEvents.length > 0;
    const hasMoreEvents = allEvents.length > collapseLimit;

    useEffect(() => {
        const el = containerRef.current;
        if (!el) return;

        const isDark = theme === 'dark';
        const gridColor = isDark ? 'rgba(255,255,255,0.06)' : 'rgba(0,0,0,0.06)';
        const borderColor = isDark ? 'rgba(255,255,255,0.12)' : 'rgba(0,0,0,0.12)';
        const textColor = isDark ? 'rgba(255,255,255,0.82)' : '#27272a';

        setChartError('');

        if (chartRef.current) {
            try {
                chartRef.current.remove();
            } catch {
                // ignore
            }
            chartRef.current = null;
            realizedSeriesRef.current = null;
            totalSeriesRef.current = null;
        }
        if (markersRef.current) {
            try {
                markersRef.current.detach();
            } catch {
                // ignore
            }
            markersRef.current = null;
        }
        if (resizeRef.current) {
            try {
                resizeRef.current.disconnect();
            } catch {
                // ignore
            }
            resizeRef.current = null;
        }

        try {
            const chart = createChart(el, {
                width: el.clientWidth || 320,
                height: 240,
                layout: { background: { type: 'solid', color: 'transparent' }, textColor },
                grid: {
                    vertLines: { color: gridColor },
                    horzLines: { color: gridColor },
                },
                rightPriceScale: {
                    borderColor,
                    scaleMargins: { top: 0.15, bottom: 0.15 },
                },
                timeScale: {
                    borderColor,
                    timeVisible: true,
                    secondsVisible: false,
                },
                crosshair: { mode: 0 },
            });

            const realized = chart.addSeries(LineSeries, {
                color: isDark ? '#60a5fa' : '#2563eb',
                lineWidth: 2,
                priceLineVisible: false,
            });
            const total = chart.addSeries(LineSeries, {
                color: isDark ? '#34d399' : '#10b981',
                lineWidth: 2,
                lineStyle: LineStyle.Dashed,
                priceLineVisible: false,
            });

            chartRef.current = chart;
            realizedSeriesRef.current = realized;
            totalSeriesRef.current = total;
            markersRef.current = createSeriesMarkers(total, []);

            if (typeof ResizeObserver !== 'undefined') {
                const ro = new ResizeObserver(() => {
                    const w = el.clientWidth || 320;
                    chart.applyOptions({ width: w });
                });
                ro.observe(el);
                resizeRef.current = ro;
            }
        } catch (err) {
            setChartError(`图表初始化失败：${String(err?.message || err)}`);
        }

        return () => {
            if (resizeRef.current) {
                try {
                    resizeRef.current.disconnect();
                } catch {
                    // ignore
                }
                resizeRef.current = null;
            }
            if (markersRef.current) {
                try {
                    markersRef.current.detach();
                } catch {
                    // ignore
                }
                markersRef.current = null;
            }
            if (chartRef.current) {
                try {
                    chartRef.current.remove();
                } catch {
                    // ignore
                }
                chartRef.current = null;
                realizedSeriesRef.current = null;
                totalSeriesRef.current = null;
            }
        };
    }, [theme, chartNonce]);

    useEffect(() => {
        if (chartError) return;
        const chart = chartRef.current;
        const realized = realizedSeriesRef.current;
        const total = totalSeriesRef.current;
        const markersApi = markersRef.current;
        if (!chart || !realized || !total) return;

        try {
            realized.setData(realizedSeries);
            total.setData(totalSeries.length ? totalSeries : realizedSeries);
            markersApi?.setMarkers?.(markers);
            chart.timeScale().fitContent();
        } catch (err) {
            setChartError(`图表渲染失败：${String(err?.message || err)}`);
        }
    }, [realizedSeries, totalSeries, markers, theme, chartNonce, chartError]);

    const hasData = Boolean(data && (realizedSeries.length > 0 || totalSeries.length > 0));
    const warnings = Array.isArray(data?.warnings) ? data.warnings : [];
    const truncated = Boolean(data?.truncated);

    return (
        <div className="rounded-2xl border border-zinc-200 bg-white p-4 shadow-sm dark:border-white/10 dark:bg-[#111318] dark:shadow-none">
            <div className="flex items-start justify-between gap-3">
                <div>
                    <div className="text-sm font-extrabold text-zinc-900 dark:text-white/90">盈利曲线</div>
                    <div className="mt-0.5 text-[11px] text-zinc-500 dark:text-white/40">
                        {windowLabel || 'AutoLP'}
                        {truncated ? ' · 已截断(最近400笔)' : ''}
                    </div>
                </div>
                <div className="text-right">
                    <div className="text-[11px] text-zinc-500 dark:text-white/40">总收益</div>
                    <div className={`text-lg font-extrabold tabular-nums ${totalProfit >= 0 ? 'text-emerald-600 dark:text-emerald-300' : 'text-red-600 dark:text-red-300'}`}>
                        {formatUsd(totalProfit)}
                    </div>
                </div>
            </div>

            <div className="mt-3 grid grid-cols-3 gap-2 text-xs">
                <div className="rounded-xl border border-zinc-200 bg-zinc-50 p-2 dark:border-white/10 dark:bg-[#0f1116]">
                    <div className="text-[11px] text-zinc-500 dark:text-white/40">已实现(含Gas)</div>
                    <div className="mt-0.5 font-semibold tabular-nums text-zinc-900 dark:text-white/80">{formatUsd(realizedProfit)}</div>
                </div>
                <div className="rounded-xl border border-zinc-200 bg-zinc-50 p-2 dark:border-white/10 dark:bg-[#0f1116]">
                    <div className="text-[11px] text-zinc-500 dark:text-white/40">未实现(浮动)</div>
                    <div className="mt-0.5 font-semibold tabular-nums text-zinc-900 dark:text-white/80">{formatUsd(unrealizedProfit)}</div>
                </div>
                <div className="rounded-xl border border-zinc-200 bg-zinc-50 p-2 dark:border-white/10 dark:bg-[#0f1116]">
                    <div className="text-[11px] text-zinc-500 dark:text-white/40">交易数</div>
                    <div className="mt-0.5 font-semibold tabular-nums text-zinc-900 dark:text-white/80">{Number(data?.trades_count ?? 0) || 0}</div>
                </div>
            </div>

            <div className="mt-3">
                <div ref={containerRef} className="h-[240px] w-full" />
                {chartError ? (
                    <div className="mt-3 flex items-center justify-between gap-2 rounded-xl border border-red-500/30 bg-red-500/10 p-3 text-xs text-red-700 dark:text-red-200">
                        <div className="min-w-0 break-words">{chartError}</div>
                        <button
                            type="button"
                            onClick={() => {
                                setChartError('');
                                setChartNonce((v) => v + 1);
                            }}
                            className="shrink-0 rounded-lg bg-red-500/15 px-2.5 py-1 text-[11px] font-semibold text-red-700 hover:bg-red-500/20 dark:text-red-200"
                        >
                            重试
                        </button>
                    </div>
                ) : null}
                {!hasData && loading ? (
                    <div className="mt-3 text-xs text-zinc-500 dark:text-white/50">加载中...</div>
                ) : null}
                {!hasData && !loading && !error ? (
                    <div className="mt-3 text-xs text-zinc-500 dark:text-white/50">暂无已实现收益数据</div>
                ) : null}
                {error ? (
                    <div className="mt-3 text-xs text-red-600 dark:text-red-300">{String(error)}</div>
                ) : null}
                {warnings.length ? (
                    <div className="mt-3 rounded-xl border border-amber-500/30 bg-amber-500/10 p-3 text-xs text-amber-700 dark:border-amber-500/20 dark:bg-amber-500/5 dark:text-amber-200">
                        <div className="font-semibold">提示</div>
                        <ul className="mt-1 list-disc space-y-1 pl-4">
                            {warnings.map((w, i) => (
                                <li key={String(i)}>{w}</li>
                            ))}
                        </ul>
                    </div>
                ) : null}
            </div>

            {hasEvents ? (
                <div className="mt-4">
                    <div className="flex items-center justify-between">
                        <div className="text-xs font-semibold text-zinc-900 dark:text-white/80">开/平仓记录</div>
                        {hasMoreEvents ? (
                            <button
                                type="button"
                                onClick={() => setShowAllEvents((v) => !v)}
                                className="text-[11px] font-semibold text-emerald-600 hover:text-emerald-700 dark:text-emerald-300 dark:hover:text-emerald-200"
                            >
                                {showAllEvents ? '收起' : '展开'}
                            </button>
                        ) : null}
                    </div>
                    {eventsForList.length ? (
                        <div
                            className="mt-2 max-h-64 overflow-auto rounded-xl border border-zinc-200 bg-zinc-50 p-2 text-[11px] dark:border-white/10 dark:bg-[#0f1116]"
                        >
                            <div className="space-y-1">
                                {eventsForList.map((e, idx) => {
                                    const isOpen = e.type === 'open';
                                    const label = isOpen ? '开仓' : '平仓';
                                    const tone = isOpen ? 'text-sky-700 dark:text-sky-300' : (e.profitUSDT >= 0 ? 'text-emerald-700 dark:text-emerald-300' : 'text-red-700 dark:text-red-300');
                                    const valueText = isOpen ? formatUsd(e.openUSDT) : `${e.profitUSDT >= 0 ? '+' : '-'}${formatUsd(Math.abs(e.profitUSDT))}`;
                                    return (
                                        <div key={`${e.type}:${e.t}:${idx}`} className="flex items-center justify-between gap-2 rounded-lg bg-white/60 px-2 py-1 dark:bg-white/5">
                                            <div className="min-w-0">
                                                <div className={`font-semibold ${tone}`}>
                                                    {label} {e.pair || '--'} <span className="ml-2 text-zinc-500 dark:text-white/40">{formatEventTime(e.t)}</span>
                                                </div>
                                            </div>
                                            <div className={`shrink-0 font-semibold tabular-nums ${tone}`}>{valueText}</div>
                                        </div>
                                    );
                                })}
                            </div>
                        </div>
                    ) : null}
                </div>
            ) : null}
        </div>
    );
}
