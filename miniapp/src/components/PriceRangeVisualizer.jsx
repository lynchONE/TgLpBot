import React, { useMemo } from 'react';

const formatPrice = (value) => {
    const n = Number(value);
    if (!Number.isFinite(n)) return '--';
    if (n === 0) return '0';
    if (Math.abs(n) < 0.0001) return n.toExponential(4);
    if (Math.abs(n) > 100000) return n.toExponential(4);
    return n.toPrecision(6).replace(/\.?0+$/, '').replace(/e[-+]\d+/i, (match) => match.toLowerCase());
};

const formatPercent = (value) => {
    const n = Number(value);
    if (!Number.isFinite(n)) return '--';
    if (Math.abs(n) >= 10) return `${n.toFixed(1)}%`;
    return `${n.toFixed(2)}%`;
};

export default function PriceRangeVisualizer({
    currentPrice,
    minPrice,
    maxPrice,
    pairLabel,
    gridCount,
    deviation,
    inRange,
    currentGridIndex,
    currentGridLower,
    currentGridUpper,
    taskRangeText = '',
    runningDuration = '',
}) {
    const rawPercent = useMemo(() => {
        if (!Number.isFinite(currentPrice) || !Number.isFinite(minPrice) || !Number.isFinite(maxPrice)) return null;
        if (maxPrice === minPrice) return 50;
        return ((currentPrice - minPrice) / (maxPrice - minPrice)) * 100;
    }, [currentPrice, minPrice, maxPrice]);

    const clampedPercent = rawPercent === null ? null : Math.max(0, Math.min(100, rawPercent));
    const midPrice = Number.isFinite(minPrice) && Number.isFinite(maxPrice) ? (minPrice + maxPrice) / 2 : null;
    const hasRange = Number.isFinite(currentPrice) && Number.isFinite(minPrice) && Number.isFinite(maxPrice);

    const outOfRangeInfo = useMemo(() => {
        if (!hasRange) return null;
        if (currentPrice > maxPrice) {
            const base = Math.abs(maxPrice) > 0 ? Math.abs(maxPrice) : 1;
            const pct = ((currentPrice - maxPrice) / base) * 100;
            return { direction: 'above', percent: Math.max(0, pct) };
        }
        if (currentPrice < minPrice) {
            const base = Math.abs(minPrice) > 0 ? Math.abs(minPrice) : 1;
            const pct = ((minPrice - currentPrice) / base) * 100;
            return { direction: 'below', percent: Math.max(0, pct) };
        }
        return null;
    }, [hasRange, currentPrice, minPrice, maxPrice]);

    const visualInRange = hasRange ? !outOfRangeInfo : Boolean(inRange);

    const rangeStatusText = useMemo(() => {
        if (!Number.isFinite(currentPrice)) return '当前价格不可用';
        if (visualInRange) return `价格 ${formatPrice(currentPrice)} 在范围内`;
        if (!outOfRangeInfo) return `价格 ${formatPrice(currentPrice)} 超出范围`;
        if (outOfRangeInfo.direction === 'above') {
            return `价格 ${formatPrice(currentPrice)} 高于上限 ${formatPercent(outOfRangeInfo.percent)}`;
        }
        return `价格 ${formatPrice(currentPrice)} 低于下限 ${formatPercent(outOfRangeInfo.percent)}`;
    }, [currentPrice, visualInRange, outOfRangeInfo]);

    const gridLines = useMemo(() => {
        if (!gridCount || gridCount < 2 || gridCount > 200) return [];
        const lines = [];
        for (let i = 1; i < gridCount; i++) {
            lines.push((i / gridCount) * 100);
        }
        return lines;
    }, [gridCount]);

    const visibleGridLines = useMemo(() => {
        if (gridLines.length <= 40) return gridLines;
        const step = Math.ceil(gridLines.length / 40);
        return gridLines.filter((_, i) => (i + 1) % step === 0);
    }, [gridLines]);

    return (
        <div className="mt-2.5 rounded-[14px] border border-zinc-200/60 bg-[#1c1e22]/5 p-3.5 dark:border-white/5 dark:bg-[#1f2227]">
            <div className="mb-3 flex items-center justify-between text-zinc-900 dark:text-zinc-100">
                <div className="text-[11px] font-bold opacity-80">
                    价格范围 ({pairLabel || '未知'} {gridCount ? `${gridCount}格` : ''}):
                </div>
                {deviation !== null && deviation !== undefined ? (
                    <div className="rounded-md bg-zinc-800 px-2 py-0.5 text-[11px] font-bold text-white dark:bg-black/50">
                        {deviation.toFixed(2)}%
                    </div>
                ) : null}
            </div>

            <div className="mb-1 flex justify-between px-1 text-[11px] font-bold">
                <span className="text-emerald-600 dark:text-emerald-500">下限</span>
                <span className="text-zinc-400 dark:text-zinc-500">中心</span>
                <span className="text-rose-600 dark:text-rose-500">上限</span>
            </div>

            <div className="relative flex h-6 items-center overflow-hidden rounded-full bg-[#e4e4e7] shadow-inner dark:bg-[#333539]">
                <div className="absolute left-[3%] top-0 bottom-0 w-[2px] bg-emerald-500" />
                <div className="absolute right-[3%] top-0 bottom-0 w-[2px] bg-rose-500" />

                <div className="absolute left-[3%] right-[3%] top-0 bottom-0 flex items-end pb-1.5 opacity-40">
                    {visibleGridLines.map((pct, i) => (
                        <div key={i} className="absolute h-2.5 w-[1px] bg-zinc-500" style={{ left: `${pct}%`, transform: 'translateX(-50%)' }} />
                    ))}
                </div>

                {clampedPercent !== null ? (
                    <div
                        className="absolute top-0 bottom-0 z-10 w-[3px] rounded-full transition-all duration-300"
                        style={{
                            left: `calc(3% + ${clampedPercent * 0.94}%)`,
                            transform: 'translateX(-50%)',
                            backgroundColor: visualInRange ? '#10b981' : '#ef4444',
                            boxShadow: `0 0 6px ${visualInRange ? '#10b981' : '#ef4444'}`,
                        }}
                    />
                ) : null}
            </div>

            <div className="mt-1.5 flex justify-between px-1 font-mono text-[11px] font-bold">
                <span className="text-emerald-600 dark:text-emerald-500">{formatPrice(minPrice)}</span>
                <span className="text-zinc-500 dark:text-zinc-400">{formatPrice(midPrice)}</span>
                <span className="text-rose-600 dark:text-rose-500">{formatPrice(maxPrice)}</span>
            </div>

            <div className="mt-4 flex flex-col items-center gap-2.5">
                {currentGridIndex !== null && currentGridLower !== null && currentGridUpper !== null ? (
                    <div className="flex items-center gap-2 rounded-lg border border-zinc-200/80 bg-zinc-100 px-4 py-1.5 text-[11px] font-bold text-zinc-700 shadow-sm dark:border-white/5 dark:bg-white/5 dark:text-zinc-300">
                        <span className="text-emerald-600 dark:text-emerald-400">第 {currentGridIndex} 格</span>
                        <span className="text-zinc-300 dark:text-zinc-600">|</span>
                        <span className="font-mono">{formatPrice(currentGridLower)} - {formatPrice(currentGridUpper)}</span>
                    </div>
                ) : null}

                <div className={`w-full rounded-lg border py-2.5 text-center text-xs font-bold transition-colors shadow-sm ${visualInRange
                    ? 'border-emerald-200 bg-[#10b981]/10 text-emerald-600 dark:border-emerald-500/20 dark:text-emerald-400'
                    : 'border-rose-200 bg-[#ef4444]/10 text-rose-600 dark:border-rose-500/20 dark:text-rose-400'
                    }`}>
                    {visualInRange ? '✓' : '✗'} {rangeStatusText}
                </div>

                {(taskRangeText || runningDuration) ? (
                    <div className="flex w-full flex-wrap items-center justify-center gap-1.5">
                        {taskRangeText ? (
                            <span className="inline-flex items-center rounded-md bg-sky-500/10 px-2 py-1 text-[10px] font-semibold text-sky-700 ring-1 ring-sky-500/20 dark:bg-sky-500/15 dark:text-sky-300">
                                策略范围 {taskRangeText}
                            </span>
                        ) : null}
                        {runningDuration ? (
                            <span className="inline-flex items-center rounded-md bg-emerald-500/10 px-2 py-1 text-[10px] font-semibold text-emerald-700 ring-1 ring-emerald-500/20 dark:bg-emerald-500/15 dark:text-emerald-300">
                                运行 {runningDuration}
                            </span>
                        ) : null}
                    </div>
                ) : null}
            </div>
        </div>
    );
}
