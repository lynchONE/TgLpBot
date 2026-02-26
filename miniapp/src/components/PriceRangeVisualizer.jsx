import React, { useMemo } from 'react';

const formatPrice = (value) => {
    const n = Number(value);
    if (!Number.isFinite(n)) return '--';
    if (n === 0) return '0';
    if (Math.abs(n) < 0.0001) return n.toExponential(4);
    if (Math.abs(n) > 100000) return n.toExponential(4);
    return n.toPrecision(6).replace(/\.?0+$/, '').replace(/e[-+]\d+/i, (match) => match.toLowerCase());
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
    currentGridUpper
}) {
    const rawPercent = useMemo(() => {
        if (!Number.isFinite(currentPrice) || !Number.isFinite(minPrice) || !Number.isFinite(maxPrice)) return null;
        if (maxPrice === minPrice) return 50;
        return ((currentPrice - minPrice) / (maxPrice - minPrice)) * 100;
    }, [currentPrice, minPrice, maxPrice]);

    const clampedPercent = rawPercent === null ? null : Math.max(0, Math.min(100, rawPercent));
    const midPrice = Number.isFinite(minPrice) && Number.isFinite(maxPrice) ? (minPrice + maxPrice) / 2 : null;

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
            {/* Header */}
            <div className="flex items-center justify-between mb-3 text-zinc-900 dark:text-zinc-100">
                <div className="text-[11px] font-bold opacity-80">
                    价格范围 ({pairLabel || '未知'} {gridCount ? `${gridCount}格` : ''}):
                </div>
                {deviation !== null && deviation !== undefined && (
                    <div className="text-[11px] font-bold bg-zinc-800 text-white dark:bg-black/50 px-2 py-0.5 rounded-md">
                        {deviation.toFixed(2)}%
                    </div>
                )}
            </div>

            {/* Track Labels Top */}
            <div className="flex justify-between text-[11px] font-bold mb-1 px-1">
                <span className="text-emerald-600 dark:text-emerald-500">下限</span>
                <span className="text-zinc-400 dark:text-zinc-500">中心</span>
                <span className="text-rose-600 dark:text-rose-500">上限</span>
            </div>

            {/* Track */}
            <div className="relative h-6 bg-[#e4e4e7] dark:bg-[#333539] rounded-full overflow-hidden flex items-center shadow-inner">
                {/* Left/Right bounds */}
                <div className="absolute left-[3%] top-0 bottom-0 w-[2px] bg-emerald-500" />
                <div className="absolute right-[3%] top-0 bottom-0 w-[2px] bg-rose-500" />

                {/* Ticks container */}
                <div className="absolute left-[3%] right-[3%] top-0 bottom-0 flex items-end pb-1.5 opacity-40">
                    {visibleGridLines.map((pct, i) => (
                        <div key={i} className="absolute w-[1px] h-2.5 bg-zinc-500" style={{ left: `${pct}%`, transform: 'translateX(-50%)' }} />
                    ))}
                </div>

                {/* Current Price Pointer */}
                {clampedPercent !== null && (
                    <div
                        className="absolute top-0 bottom-0 w-[3px] rounded-full z-10 transition-all duration-300"
                        style={{
                            left: `calc(3% + ${clampedPercent} * 0.94)`,
                            transform: 'translateX(-50%)',
                            backgroundColor: inRange ? '#10b981' : '#ef4444',
                            boxShadow: `0 0 6px ${inRange ? '#10b981' : '#ef4444'}`
                        }}
                    />
                )}
            </div>

            {/* Track Labels Bottom */}
            <div className="flex justify-between text-[11px] font-bold mt-1.5 px-1 font-mono">
                <span className="text-emerald-600 dark:text-emerald-500">{formatPrice(minPrice)}</span>
                <span className="text-zinc-500 dark:text-zinc-400">{formatPrice(midPrice)}</span>
                <span className="text-rose-600 dark:text-rose-500">{formatPrice(maxPrice)}</span>
            </div>

            {/* Current Grid Info */}
            <div className="mt-4 flex flex-col items-center gap-2.5">
                {currentGridIndex !== null && currentGridLower !== null && currentGridUpper !== null && (
                    <div className="text-[11px] font-bold text-zinc-700 dark:text-zinc-300 bg-zinc-100 dark:bg-white/5 px-4 py-1.5 rounded-lg flex items-center gap-2 shadow-sm border border-zinc-200/80 dark:border-white/5">
                        <span className="text-emerald-600 dark:text-emerald-400">第 {currentGridIndex} 格</span>
                        <span className="text-zinc-300 dark:text-zinc-600">|</span>
                        <span className="font-mono">{formatPrice(currentGridLower)} - {formatPrice(currentGridUpper)}</span>
                    </div>
                )}

                <div className={`w-full text-center py-2.5 rounded-lg text-xs font-bold transition-colors shadow-sm ${inRange
                        ? 'bg-[#10b981]/10 text-emerald-600 dark:text-emerald-400 border border-emerald-200 dark:border-emerald-500/20'
                        : 'bg-[#ef4444]/10 text-rose-600 dark:text-rose-400 border border-rose-200 dark:border-rose-500/20'
                    }`}>
                    {inRange ? '✅' : '❌'} 价格 <span className="font-mono">{formatPrice(currentPrice)}</span> {inRange ? '在范围内' : '超出范围'}
                </div>
            </div>
        </div>
    );
}
