import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { createChart } from 'lightweight-charts';
import { fetchPoolOHLCV } from '../lib/api';

const Icon = ({ path, className = '' }) => (
    <svg viewBox="0 0 24 24" fill="currentColor" className={className} aria-hidden="true">
        <path d={path} />
    </svg>
);

const icons = {
    close: 'M6 18L18 6M6 6l12 12',
    refresh: 'M17.65 6.35A7.95 7.95 0 0012 4V1L7 6l5 5V7a5 5 0 11-5 5H5a7 7 0 107.65-5.65z',
};

const TIMEFRAMES = [
    { key: '5m', label: '5m', timeframe: 'minute', aggregate: 5 },
    { key: '15m', label: '15m', timeframe: 'minute', aggregate: 15 },
    { key: '1h', label: '1h', timeframe: 'hour', aggregate: 1 },
    { key: '4h', label: '4h', timeframe: 'hour', aggregate: 4 },
];

const isHexAddress = (v) => /^0x[a-fA-F0-9]{40}$/.test(String(v || '').trim());

export default function KlineModal({ open, onClose, apiBaseUrl, theme, pool, chain }) {
    const containerRef = useRef(null);
    const chartRef = useRef(null);
    const candleSeriesRef = useRef(null);
    const volumeSeriesRef = useRef(null);
    const requestIdRef = useRef(0);
    const requestControllerRef = useRef(null);

    const [timeframeKey, setTimeframeKey] = useState('5m');
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState('');
    const [candles, setCandles] = useState([]);
    const cacheRef = useRef(new Map());

    const poolAddress = useMemo(() => String(pool?.pool_address || '').trim(), [pool?.pool_address]);
    const title = useMemo(() => String(pool?.trading_pair || '').trim() || 'K线图', [pool?.trading_pair]);
    const effectiveChain = useMemo(() => String(chain || '').trim() || 'bsc', [chain]);

    useEffect(() => {
        if (!open) return;
        cacheRef.current = new Map();
        setError('');
        setCandles([]);
        setTimeframeKey('5m');
    }, [open, poolAddress]);

    const chartTheme = useMemo(() => {
        const isDark = theme === 'dark';
        return {
            background: isDark ? '#111318' : '#ffffff',
            text: isDark ? 'rgba(255,255,255,0.70)' : 'rgba(24,24,27,0.80)',
            grid: isDark ? 'rgba(255,255,255,0.06)' : 'rgba(24,24,27,0.08)',
            border: isDark ? 'rgba(255,255,255,0.10)' : 'rgba(24,24,27,0.12)',
            up: '#10b981',
            down: '#f43f5e',
        };
    }, [theme]);

    useEffect(() => {
        if (!open) return;
        const el = containerRef.current;
        if (!el) return;
        if (chartRef.current) return;

        const chart = createChart(el, {
            autoSize: true,
            layout: {
                background: { color: chartTheme.background },
                textColor: chartTheme.text,
                fontFamily:
                    'ui-sans-serif, system-ui, -apple-system, Segoe UI, Roboto, Helvetica, Arial, "Apple Color Emoji", "Segoe UI Emoji"',
            },
            grid: {
                vertLines: { color: chartTheme.grid },
                horzLines: { color: chartTheme.grid },
            },
            rightPriceScale: {
                borderColor: chartTheme.border,
            },
            timeScale: {
                borderColor: chartTheme.border,
                timeVisible: true,
                secondsVisible: false,
            },
            crosshair: {
                vertLine: { color: chartTheme.border, labelBackgroundColor: chartTheme.background },
                horzLine: { color: chartTheme.border, labelBackgroundColor: chartTheme.background },
            },
        });

        const candleSeries = chart.addCandlestickSeries({
            upColor: chartTheme.up,
            downColor: chartTheme.down,
            borderUpColor: chartTheme.up,
            borderDownColor: chartTheme.down,
            wickUpColor: chartTheme.up,
            wickDownColor: chartTheme.down,
        });

        const volumeSeries = chart.addHistogramSeries({
            priceFormat: { type: 'volume' },
            priceScaleId: '',
            color: chartTheme.up,
            scaleMargins: { top: 0.82, bottom: 0 },
        });

        chartRef.current = chart;
        candleSeriesRef.current = candleSeries;
        volumeSeriesRef.current = volumeSeries;

        return () => {
            try {
                requestControllerRef.current?.abort?.();
            } catch {
                // ignore
            }
            requestControllerRef.current = null;
            chart.remove();
            chartRef.current = null;
            candleSeriesRef.current = null;
            volumeSeriesRef.current = null;
        };
    }, [open, chartTheme.background, chartTheme.border, chartTheme.down, chartTheme.grid, chartTheme.text, chartTheme.up]);

    useEffect(() => {
        if (!open) return;
        const chart = chartRef.current;
        if (!chart) return;
        chart.applyOptions({
            layout: { background: { color: chartTheme.background }, textColor: chartTheme.text },
            grid: { vertLines: { color: chartTheme.grid }, horzLines: { color: chartTheme.grid } },
            rightPriceScale: { borderColor: chartTheme.border },
            timeScale: { borderColor: chartTheme.border },
            crosshair: {
                vertLine: { color: chartTheme.border, labelBackgroundColor: chartTheme.background },
                horzLine: { color: chartTheme.border, labelBackgroundColor: chartTheme.background },
            },
        });
        candleSeriesRef.current?.applyOptions({
            upColor: chartTheme.up,
            downColor: chartTheme.down,
            borderUpColor: chartTheme.up,
            borderDownColor: chartTheme.down,
            wickUpColor: chartTheme.up,
            wickDownColor: chartTheme.down,
        });
        volumeSeriesRef.current?.applyOptions({ color: chartTheme.up });
    }, [open, chartTheme]);

    const selectedTimeframe = useMemo(() => {
        return TIMEFRAMES.find((t) => t.key === timeframeKey) || TIMEFRAMES[0];
    }, [timeframeKey]);

    const loadCandles = useCallback(
        async ({ force = false } = {}) => {
            if (!open) return;
            if (!isHexAddress(poolAddress)) return;
            const cacheKey = `${effectiveChain}:${poolAddress}:${selectedTimeframe.timeframe}:${selectedTimeframe.aggregate}`;
            if (!force && cacheRef.current.has(cacheKey)) {
                setCandles(cacheRef.current.get(cacheKey));
                return;
            }

            const requestId = requestIdRef.current + 1;
            requestIdRef.current = requestId;

            try {
                requestControllerRef.current?.abort?.();
            } catch {
                // ignore
            }
            const controller = new AbortController();
            requestControllerRef.current = controller;

            setLoading(true);
            setError('');
            try {
                const resp = await fetchPoolOHLCV({
                    apiBaseUrl,
                    chain: effectiveChain,
                    poolAddress,
                    timeframe: selectedTimeframe.timeframe,
                    aggregate: selectedTimeframe.aggregate,
                    limit: 200,
                    signal: controller.signal,
                });
                if (requestIdRef.current !== requestId) return;
                const rows = Array.isArray(resp?.candles) ? resp.candles : [];
                cacheRef.current.set(cacheKey, rows);
                setCandles(rows);
            } catch (e) {
                if (requestIdRef.current !== requestId) return;
                setError(String(e?.message || e));
                setCandles([]);
            } finally {
                if (requestIdRef.current !== requestId) return;
                setLoading(false);
            }
        },
        [apiBaseUrl, effectiveChain, open, poolAddress, selectedTimeframe.aggregate, selectedTimeframe.timeframe]
    );

    useEffect(() => {
        if (!open) return;
        loadCandles({ force: false });
    }, [open, poolAddress, timeframeKey, loadCandles]);

    const candleData = useMemo(() => {
        return (Array.isArray(candles) ? candles : [])
            .map((c) => ({
                time: Number(c?.t || 0),
                open: Number(c?.o || 0),
                high: Number(c?.h || 0),
                low: Number(c?.l || 0),
                close: Number(c?.c || 0),
            }))
            .filter((c) => Number.isFinite(c.time) && c.time > 0);
    }, [candles]);

    const volumeData = useMemo(() => {
        return (Array.isArray(candles) ? candles : [])
            .map((c) => {
                const time = Number(c?.t || 0);
                const open = Number(c?.o || 0);
                const close = Number(c?.c || 0);
                const value = Number(c?.v || 0);
                if (!Number.isFinite(time) || time <= 0 || !Number.isFinite(value) || value < 0) return null;
                const isUp = Number.isFinite(open) && Number.isFinite(close) ? close >= open : true;
                return {
                    time,
                    value,
                    color: isUp ? 'rgba(16,185,129,0.35)' : 'rgba(244,63,94,0.35)',
                };
            })
            .filter(Boolean);
    }, [candles]);

    useEffect(() => {
        if (!open) return;
        candleSeriesRef.current?.setData(candleData);
        volumeSeriesRef.current?.setData(volumeData);
        chartRef.current?.timeScale().fitContent();
    }, [open, candleData, volumeData]);

    if (!open) return null;

    return (
        <div className="fixed inset-0 z-50">
            <button
                type="button"
                className="absolute inset-0 bg-black/50"
                onClick={onClose}
                aria-label="关闭"
            />
            <div className="absolute inset-x-0 bottom-0 max-h-[86vh] rounded-t-2xl border border-zinc-200 bg-white p-4 shadow-2xl dark:border-white/10 dark:bg-[#111318] dark:shadow-none">
                <div className="flex items-center justify-between gap-2">
                    <div className="min-w-0">
                        <div className="truncate text-sm font-semibold text-zinc-900 dark:text-white/90">{title}</div>
                        <div className="mt-0.5 truncate text-[11px] text-zinc-500 dark:text-white/40">
                            {poolAddress ? `${poolAddress.slice(0, 10)}...${poolAddress.slice(-6)}` : ''}
                        </div>
                    </div>
                    <div className="flex items-center gap-2">
                        <button
                            type="button"
                            onClick={() => loadCandles({ force: true })}
                            disabled={loading || !isHexAddress(poolAddress)}
                            className="inline-flex h-9 w-9 items-center justify-center rounded-xl border border-zinc-200 bg-zinc-100 text-zinc-900 shadow-sm hover:bg-zinc-200 active:bg-zinc-200 disabled:opacity-40 dark:border-white/10 dark:bg-white/5 dark:text-white/80 dark:hover:bg-white/10 dark:active:bg-white/15"
                            aria-label="刷新K线"
                            title="刷新"
                        >
                            <Icon path={icons.refresh} className="h-5 w-5" />
                        </button>
                        <button
                            type="button"
                            onClick={onClose}
                            className="inline-flex h-9 w-9 items-center justify-center rounded-xl border border-zinc-200 bg-zinc-100 text-zinc-900 shadow-sm hover:bg-zinc-200 active:bg-zinc-200 dark:border-white/10 dark:bg-white/5 dark:text-white/80 dark:hover:bg-white/10 dark:active:bg-white/15"
                            aria-label="关闭"
                        >
                            <Icon path={icons.close} className="h-5 w-5" />
                        </button>
                    </div>
                </div>

                <div className="mt-3 flex items-center justify-between gap-3">
                    <div className="flex rounded-2xl border border-zinc-200 bg-zinc-100/70 p-1 text-xs font-semibold dark:border-white/10 dark:bg-white/5">
                        {TIMEFRAMES.map((t) => (
                            <button
                                key={t.key}
                                type="button"
                                onClick={() => setTimeframeKey(t.key)}
                                aria-pressed={timeframeKey === t.key}
                                className={`rounded-xl px-3 py-2 transition ${timeframeKey === t.key
                                    ? 'bg-white text-zinc-900 shadow-sm dark:bg-white/15 dark:text-white'
                                    : 'text-zinc-600 hover:bg-white/60 dark:text-white/50 dark:hover:bg-white/10'
                                    }`}
                            >
                                {t.label}
                            </button>
                        ))}
                    </div>
                    <div className="text-[11px] text-zinc-500 dark:text-white/40">
                        {loading ? '加载中...' : error ? '加载失败' : candles.length ? `共 ${candles.length} 根` : '暂无数据'}
                    </div>
                </div>

                {error ? (
                    <div className="mt-3 rounded-xl border border-red-500/30 bg-red-500/10 p-3 text-xs text-red-700 dark:text-red-200">
                        {error}
                    </div>
                ) : null}

                <div className="mt-3 overflow-hidden rounded-2xl border border-zinc-200 bg-zinc-50 dark:border-white/10 dark:bg-[#0f1116]">
                    <div ref={containerRef} className="h-[380px] w-full" />
                </div>

                <div className="mt-2 text-[11px] text-zinc-500 dark:text-white/35">数据源：GeckoTerminal</div>
            </div>
        </div>
    );
}
