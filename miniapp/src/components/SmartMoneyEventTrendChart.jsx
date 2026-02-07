import React, { useEffect, useMemo, useRef, useState } from 'react';
import { createChart, HistogramSeries } from 'lightweight-charts';

function normalizeTrend(trend) {
    if (!Array.isArray(trend)) return [];
    const now = Math.floor(Date.now() / 1000);
    const hourNow = Math.floor(now / 3600) * 3600;
    return trend
        .map((p) => {
            const h = Number(p?.hours_ago ?? 0);
            const add = Number(p?.add_events ?? 0);
            const remove = Number(p?.remove_events ?? 0);
            if (!Number.isFinite(h) || h < 0) return null;
            const time = hourNow - Math.floor(h) * 3600;
            return {
                time,
                add: Number.isFinite(add) ? add : 0,
                remove: Number.isFinite(remove) ? remove : 0,
            };
        })
        .filter(Boolean)
        .sort((a, b) => a.time - b.time);
}

export default function SmartMoneyEventTrendChart({ trend, theme = 'dark' }) {
    const containerRef = useRef(null);
    const chartRef = useRef(null);
    const addSeriesRef = useRef(null);
    const removeSeriesRef = useRef(null);
    const resizeRef = useRef(null);
    const [chartError, setChartError] = useState('');

    const series = useMemo(() => normalizeTrend(trend), [trend]);

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
            addSeriesRef.current = null;
            removeSeriesRef.current = null;
        }

        try {
            const chart = createChart(el, {
                width: el.clientWidth || 320,
                height: 200,
                layout: { background: { type: 'solid', color: 'transparent' }, textColor },
                grid: {
                    vertLines: { color: gridColor },
                    horzLines: { color: gridColor },
                },
                rightPriceScale: {
                    borderColor,
                    scaleMargins: { top: 0.2, bottom: 0.2 },
                },
                timeScale: {
                    borderColor,
                    timeVisible: true,
                    secondsVisible: false,
                },
                crosshair: { mode: 0 },
            });

            const addSeries = chart.addSeries(HistogramSeries, {
                color: isDark ? 'rgba(16,185,129,0.75)' : 'rgba(5,150,105,0.75)',
                base: 0,
                priceLineVisible: false,
            });
            const removeSeries = chart.addSeries(HistogramSeries, {
                color: isDark ? 'rgba(239,68,68,0.65)' : 'rgba(220,38,38,0.65)',
                base: 0,
                priceLineVisible: false,
            });

            chartRef.current = chart;
            addSeriesRef.current = addSeries;
            removeSeriesRef.current = removeSeries;

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
                addSeriesRef.current = null;
                removeSeriesRef.current = null;
            }
        };
    }, [theme]);

    useEffect(() => {
        if (chartError) return;
        const chart = chartRef.current;
        const addSeries = addSeriesRef.current;
        const removeSeries = removeSeriesRef.current;
        if (!chart || !addSeries || !removeSeries) return;

        const addData = series.map((p) => ({
            time: p.time,
            value: Math.max(0, p.add),
        }));
        const removeData = series.map((p) => ({
            time: p.time,
            value: -Math.max(0, p.remove),
        }));

        try {
            addSeries.setData(addData);
            removeSeries.setData(removeData);
            chart.timeScale().fitContent();
        } catch (err) {
            setChartError(`图表渲染失败：${String(err?.message || err)}`);
        }
    }, [series, chartError]);

    return (
        <div>
            <div ref={containerRef} className="h-[200px] w-full" />
            {chartError ? (
                <div className="mt-2 rounded-xl border border-red-500/30 bg-red-500/10 p-3 text-xs text-red-700 dark:text-red-200">
                    {chartError}
                </div>
            ) : null}
        </div>
    );
}

