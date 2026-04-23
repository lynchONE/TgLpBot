import React, { useMemo } from 'react';
import NumberFlowValue from './NumberFlowValue.jsx';

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
    gridStepPct = null,
    rangeBadgeText = '',
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
    const hasGridCount = Number.isFinite(Number(gridCount)) && Number(gridCount) > 0;

    const outOfRangeInfo = useMemo(() => {
        if (!hasRange) return null;
        if (currentPrice > maxPrice) {
            const base = Math.abs(maxPrice) > 0 ? Math.abs(maxPrice) : 1;
            return { direction: 'above', percent: Math.max(0, ((currentPrice - maxPrice) / base) * 100) };
        }
        if (currentPrice < minPrice) {
            const base = Math.abs(minPrice) > 0 ? Math.abs(minPrice) : 1;
            return { direction: 'below', percent: Math.max(0, ((minPrice - currentPrice) / base) * 100) };
        }
        return null;
    }, [hasRange, currentPrice, minPrice, maxPrice]);

    const visualInRange = hasRange ? !outOfRangeInfo : Boolean(inRange);

    const statusText = useMemo(() => {
        const currentLabel = `当前价 ${formatPrice(currentPrice)}`;
        if (!Number.isFinite(currentPrice)) return `${currentLabel} · 暂不可用`;
        if (visualInRange) return `${currentLabel} · 在区间内`;
        if (!outOfRangeInfo) return `${currentLabel} · 超出区间`;
        if (outOfRangeInfo.direction === 'above') return `${currentLabel} · 高于上限 ${formatPercent(outOfRangeInfo.percent)}`;
        return `${currentLabel} · 低于下限 ${formatPercent(outOfRangeInfo.percent)}`;
    }, [currentPrice, visualInRange, outOfRangeInfo]);

    const gridInfoText = useMemo(() => {
        if (currentGridIndex === null || currentGridLower === null || currentGridUpper === null) return '当前网格 --';
        const currentGridLabel = hasGridCount
            ? `第 ${currentGridIndex}/${Number(gridCount)} 格`
            : `第 ${currentGridIndex} 格`;
        return `${currentGridLabel} | ${formatPrice(currentGridLower)} - ${formatPrice(currentGridUpper)}`;
    }, [currentGridIndex, currentGridLower, currentGridUpper, gridCount, hasGridCount]);

    const gridLines = useMemo(() => {
        if (!hasGridCount || Number(gridCount) < 2 || Number(gridCount) > 200) return [];
        const lines = [];
        for (let i = 1; i < Number(gridCount); i += 1) lines.push((i / Number(gridCount)) * 100);
        return lines;
    }, [gridCount, hasGridCount]);

    const visibleGridLines = useMemo(() => {
        if (gridLines.length <= 40) return gridLines;
        const step = Math.ceil(gridLines.length / 40);
        return gridLines.filter((_, i) => (i + 1) % step === 0);
    }, [gridLines]);

    return (
        <div className="mt-2 rounded-xl border border-zinc-200/60 bg-[#1c1e22]/5 p-2.5 dark:border-white/5 dark:bg-[#1f2227]">
            <div className="mb-2 flex flex-wrap items-center justify-between gap-x-2 gap-y-1 text-zinc-900 dark:text-zinc-100">
                <div className="text-[10px] font-bold opacity-85">
                    价格区间 ({pairLabel || '未知'}
                    {hasGridCount ? (
                        <>
                            {' · 共 '}
                            <NumberFlowValue value={Number(gridCount)} formatOptions={{ maximumFractionDigits: 0 }} />
                            {' 格'}
                        </>
                    ) : null}
                    {Number.isFinite(gridStepPct) ? (
                        <>
                            {' · 约 '}
                            <NumberFlowValue value={gridStepPct} formatter={(v) => `${Number(v).toFixed(2)}%/格`} />
                        </>
                    ) : null}
                    )
                </div>
                {rangeBadgeText ? (
                    <div className="rounded bg-zinc-800 px-1.5 py-0.5 text-[10px] font-bold text-white dark:bg-black/50">
                        <NumberFlowValue value={rangeBadgeText} formatter={() => rangeBadgeText} />
                    </div>
                ) : null}
            </div>

            <div className="mb-1 flex justify-between px-1 text-[10px] font-bold">
                <span className="text-emerald-600 dark:text-emerald-500">下限</span>
                <span className="text-zinc-400 dark:text-zinc-500">中位</span>
                <span className="text-rose-600 dark:text-rose-500">上限</span>
            </div>

            <div className="relative flex h-4 items-center overflow-hidden rounded-full bg-[#e4e4e7] shadow-inner dark:bg-[#333539]">
                <div className="absolute bottom-0 left-[3%] top-0 w-[2px] bg-emerald-500" />
                <div className="absolute bottom-0 right-[3%] top-0 w-[2px] bg-rose-500" />

                <div className="absolute bottom-0 left-[3%] right-[3%] top-0 flex items-end pb-1 opacity-40">
                    {visibleGridLines.map((pct, i) => (
                        <div key={i} className="absolute h-2 w-[1px] bg-zinc-500" style={{ left: `${pct}%`, transform: 'translateX(-50%)' }} />
                    ))}
                </div>

                {clampedPercent !== null ? (
                    <div
                        className="absolute bottom-0 top-0 z-10 w-[3px] rounded-full transition-all duration-300"
                        style={{
                            left: `calc(3% + ${clampedPercent * 0.94}%)`,
                            transform: 'translateX(-50%)',
                            backgroundColor: visualInRange ? '#10b981' : '#ef4444',
                            boxShadow: `0 0 6px ${visualInRange ? '#10b981' : '#ef4444'}`,
                        }}
                    />
                ) : null}
            </div>

            <div className="mt-1 flex justify-between px-1 font-mono text-[10px] font-bold">
                <span className="text-emerald-600 dark:text-emerald-500">
                    <NumberFlowValue value={minPrice} formatter={(v) => formatPrice(v)} />
                </span>
                <span className="text-zinc-500 dark:text-zinc-400">
                    <NumberFlowValue value={midPrice} formatter={(v) => formatPrice(v)} />
                </span>
                <span className="text-rose-600 dark:text-rose-500">
                    <NumberFlowValue value={maxPrice} formatter={(v) => formatPrice(v)} />
                </span>
            </div>

            <div
                className={`mt-2 flex flex-col gap-1.5 rounded-lg border px-2 py-1.5 text-[10px] font-semibold sm:flex-row sm:items-center sm:justify-between ${
                    visualInRange
                        ? 'border-emerald-200 bg-emerald-500/8 dark:border-emerald-500/20'
                        : 'border-rose-200 bg-rose-500/8 dark:border-rose-500/20'
                }`}
            >
                <div className="min-w-0 text-zinc-700 dark:text-zinc-300" title={gridInfoText}>
                    <NumberFlowValue value={gridInfoText} formatter={() => gridInfoText} />
                </div>

                <div
                    className={`min-w-0 text-left sm:text-right ${
                        visualInRange ? 'text-emerald-600 dark:text-emerald-400' : 'text-rose-600 dark:text-rose-400'
                    }`}
                    title={statusText}
                >
                    <NumberFlowValue value={statusText} formatter={() => statusText} />
                </div>
            </div>

            {(taskRangeText || runningDuration) ? (
                <div className="mt-1.5 flex flex-wrap items-start gap-2">
                    {taskRangeText ? (
                        <span className="inline-flex max-w-full flex-wrap items-center gap-1 rounded-md bg-sky-500/10 px-2 py-1 text-[10px] font-semibold leading-relaxed text-sky-700 ring-1 ring-sky-500/20 dark:bg-sky-500/15 dark:text-sky-300">
                            <span className="shrink-0">任务区间</span>
                            <span className="min-w-0 break-words">
                                <NumberFlowValue value={taskRangeText} formatter={() => taskRangeText} />
                            </span>
                        </span>
                    ) : null}
                    {runningDuration ? (
                        <span className="inline-flex items-center gap-1 rounded-md bg-emerald-500/10 px-2 py-1 text-[10px] font-semibold text-emerald-700 ring-1 ring-emerald-500/20 dark:bg-emerald-500/15 dark:text-emerald-300">
                            运行 <NumberFlowValue value={runningDuration} formatter={() => runningDuration} />
                        </span>
                    ) : null}
                </div>
            ) : null}
        </div>
    );
}
