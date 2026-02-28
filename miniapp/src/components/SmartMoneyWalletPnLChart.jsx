import React, { useEffect, useMemo, useRef, useState } from 'react';
import { createChart, HistogramSeries } from 'lightweight-charts';
import NumberFlowValue from './NumberFlowValue.jsx';

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

function normalizeWallets(wallets) {
    if (!Array.isArray(wallets)) return [];
    const now = Math.floor(Date.now() / 1000);
    const out = [];
    const top = wallets.slice(0, 50);
    for (let i = 0; i < top.length; i += 1) {
        const w = top[i];
        const pnl = Number(w?.pnl_usdt_24h ?? 0);
        if (!Number.isFinite(pnl)) continue;
        out.push({
            // Use synthetic timestamps to satisfy the chart API; time axis is hidden.
            time: now - (top.length - i) * 60,
            value: pnl,
        });
    }
    return out;
}

export default function SmartMoneyWalletPnLChart({ wallets, theme = 'dark', windowLabel = '24h' }) {
    const containerRef = useRef(null);
    const chartRef = useRef(null);
    const seriesRef = useRef(null);
    const resizeRef = useRef(null);
    const [chartError, setChartError] = useState('');

    const data = useMemo(() => normalizeWallets(wallets), [wallets]);

    useEffect(() => {
        const el = containerRef.current;
        if (!el) return;

        const isDark = theme === 'dark';
        const gridColor = isDark ? 'rgba(255,255,255,0.06)' : 'rgba(0,0,0,0.06)';
        const borderColor = isDark ? 'rgba(255,255,255,0.12)' : 'rgba(0,0,0,0.12)';
        const textColor = isDark ? 'rgba(255,255,255,0.82)' : '#27272a';

        setChartError('');

        if (resizeRef.current) {
            try {
                resizeRef.current.disconnect();
            } catch {
                // ignore
            }
            resizeRef.current = null;
        }
        if (chartRef.current) {
            try {
                chartRef.current.remove();
            } catch {
                // ignore
            }
            chartRef.current = null;
            seriesRef.current = null;
        }

        try {
            const chart = createChart(el, {
                width: el.clientWidth || 320,
                height: 160,
                layout: { background: { type: 'solid', color: 'transparent' }, textColor },
                grid: {
                    vertLines: { color: gridColor },
                    horzLines: { color: gridColor },
                },
                rightPriceScale: {
                    borderColor,
                    scaleMargins: { top: 0.25, bottom: 0.25 },
                },
                timeScale: {
                    borderColor,
                    visible: false,
                },
                crosshair: { mode: 0 },
            });

            const series = chart.addSeries(HistogramSeries, {
                base: 0,
                priceLineVisible: true,
                priceFormat: {
                    type: 'custom',
                    formatter: (p) => formatUsd(p),
                },
            });

            chartRef.current = chart;
            seriesRef.current = series;

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
            if (chartRef.current) {
                try {
                    chartRef.current.remove();
                } catch {
                    // ignore
                }
                chartRef.current = null;
                seriesRef.current = null;
            }
        };
    }, [theme]);

    useEffect(() => {
        if (chartError) return;
        const chart = chartRef.current;
        const series = seriesRef.current;
        if (!chart || !series) return;

        try {
            const seriesData = data.map((p) => ({
                time: p.time,
                value: p.value,
                color: p.value > 0
                    ? 'rgba(16,185,129,0.75)'
                    : p.value < 0
                        ? 'rgba(239,68,68,0.65)'
                        : 'rgba(161,161,170,0.55)',
            }));
            series.setData(seriesData);
            chart.timeScale().fitContent();
        } catch (err) {
            setChartError(`图表渲染失败：${String(err?.message || err)}`);
        }
    }, [data, chartError]);

    return (
        <div>
            <div className="mb-1 flex items-center justify-between text-[11px] text-zinc-500 dark:text-white/40">
                <span>Top 钱包盈亏（<NumberFlowValue value={windowLabel} formatter={() => windowLabel} />）</span>
                <span><NumberFlowValue value={data.length} formatOptions={{ maximumFractionDigits: 0 }} /> wallets</span>
            </div>
            <div ref={containerRef} className="h-[160px] w-full" />
            {chartError ? (
                <div className="mt-2 rounded-xl border border-red-500/30 bg-red-500/10 p-3 text-xs text-red-700 dark:text-red-200">
                    {chartError}
                </div>
            ) : null}
        </div>
    );
}
