import React, { useEffect, useState, useMemo } from 'react';

/**
 * 迷你价格走势图组件
 * 使用SVG绘制简易折线图
 */
export default function MiniChart({
    data, // OHLCV数组: [{close: number}, ...]
    width = 60,
    height = 24,
    strokeColor,
    loading = false,
    error = false
}) {
    // 自动判断颜色：涨绿跌红
    const color = useMemo(() => {
        if (strokeColor) return strokeColor;
        if (!data || data.length < 2) return '#94a3b8'; // 灰色
        const first = data[0]?.close || 0;
        const last = data[data.length - 1]?.close || 0;
        return last >= first ? '#10b981' : '#ef4444'; // 绿色/红色
    }, [data, strokeColor]);

    // 生成SVG路径
    const pathD = useMemo(() => {
        if (!data || data.length < 2) return '';

        const closes = data.map(d => d.close || d.c || 0).filter(v => Number.isFinite(v));
        if (closes.length < 2) return '';

        const min = Math.min(...closes);
        const max = Math.max(...closes);
        const range = max - min || 1;

        const points = closes.map((val, i) => {
            const x = (i / (closes.length - 1)) * width;
            const y = height - ((val - min) / range) * (height - 4) - 2; // 留2px边距
            return `${x},${y}`;
        });

        return `M${points.join(' L')}`;
    }, [data, width, height]);

    if (loading) {
        return (
            <div className="animate-pulse bg-zinc-200 dark:bg-zinc-700 rounded" style={{ width, height }} />
        );
    }

    if (error || !pathD) {
        return (
            <div className="flex items-center justify-center text-zinc-400 dark:text-zinc-600" style={{ width, height }}>
                <svg viewBox="0 0 24 24" fill="currentColor" className="w-4 h-4 opacity-50">
                    <path d="M3 17l6-6 4 4 7-7v4h2V4h-8v2h4l-5 5-4-4-7 7z" />
                </svg>
            </div>
        );
    }

    return (
        <svg
            width={width}
            height={height}
            viewBox={`0 0 ${width} ${height}`}
            className="overflow-visible"
        >
            <path
                d={pathD}
                fill="none"
                stroke={color}
                strokeWidth={1.5}
                strokeLinecap="round"
                strokeLinejoin="round"
            />
        </svg>
    );
}

/**
 * 带数据获取的迷你图表
 */
export function MiniChartWithData({
    poolAddress,
    chain = 'bsc',
    apiBaseUrl,
    width = 60,
    height = 24,
    limit = 24, // 24个点
}) {
    const [data, setData] = useState(null);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState(false);

    useEffect(() => {
        if (!poolAddress || !apiBaseUrl) {
            setLoading(false);
            return;
        }

        let cancelled = false;
        const controller = new AbortController();

        const fetchData = async () => {
            try {
                const base = String(apiBaseUrl || '').replace(/\/$/, '');
                const params = new URLSearchParams({
                    chain,
                    pool_address: poolAddress,
                    timeframe: '1h',
                    limit: String(limit),
                });
                const url = `${base}/api/pool_ohlcv?${params}`;

                const resp = await fetch(url, { method: 'GET', signal: controller.signal });
                if (cancelled) return;

                if (!resp.ok) {
                    setError(true);
                    setLoading(false);
                    return;
                }

                const json = await resp.json();
                if (cancelled) return;

                setData(json.data || json);
                setLoading(false);
            } catch (e) {
                if (cancelled) return;
                setError(true);
                setLoading(false);
            }
        };

        fetchData();

        return () => {
            cancelled = true;
            controller.abort();
        };
    }, [poolAddress, chain, apiBaseUrl, limit]);

    return (
        <MiniChart
            data={data}
            width={width}
            height={height}
            loading={loading}
            error={error}
        />
    );
}
