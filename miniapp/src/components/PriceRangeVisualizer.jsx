import React, { useMemo } from 'react';
import NumberFlowValue from './NumberFlowValue.jsx';

const TEXT = {
    rangeTitle: '\u4ef7\u683c\u533a\u95f4',
    unknown: '\u672a\u77e5',
    total: '\u5171',
    grids: '\u683c',
    approx: '\u7ea6',
    perGrid: '%/\u683c',
    lower: '\u4e0b\u9650',
    middle: '\u4e2d\u4f4d',
    upper: '\u4e0a\u9650',
    currentPrice: '\u5f53\u524d\u4ef7',
    unavailable: '\u6682\u4e0d\u53ef\u7528',
    inRange: '\u5728\u533a\u95f4\u5185',
    outOfRange: '\u8d85\u51fa\u533a\u95f4',
    aboveUpper: '\u9ad8\u4e8e\u4e0a\u9650',
    belowLower: '\u4f4e\u4e8e\u4e0b\u9650',
    currentBand: '\u5f53\u524d\u533a\u95f4',
    taskRange: '\u4efb\u52a1\u533a\u95f4',
    running: '\u8fd0\u884c',
};

const formatPrice = (value) => {
    const n = Number(value);
    if (!Number.isFinite(n)) return '--';
    if (n === 0) return '0';

    const abs = Math.abs(n);
    if (abs < 0.001 || abs >= 100000) {
        return n.toExponential(4).replace(/e\+?/i, 'e');
    }
    if (abs >= 1000) {
        return n.toLocaleString('en-US', {
            minimumFractionDigits: 0,
            maximumFractionDigits: 2,
        });
    }
    if (abs >= 1) {
        return n
            .toLocaleString('en-US', {
                minimumFractionDigits: 0,
                maximumFractionDigits: 4,
            })
            .replace(/\.?0+$/, '');
    }
    return n.toPrecision(6).replace(/(\.\d*?[1-9])0+$/, '$1').replace(/\.0+$/, '');
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
    currentGridLower,
    currentGridUpper,
    taskRangeText = '',
    runningDuration = '',
}) {
    const currentPriceNum = Number(currentPrice);
    const minPriceNum = Number(minPrice);
    const maxPriceNum = Number(maxPrice);
    const hasRange = Number.isFinite(currentPriceNum) && Number.isFinite(minPriceNum) && Number.isFinite(maxPriceNum);
    const hasGridCount = Number.isFinite(Number(gridCount)) && Number(gridCount) > 0;

    const rawPercent = useMemo(() => {
        if (!hasRange) return null;
        if (maxPriceNum === minPriceNum) return 50;
        return ((currentPriceNum - minPriceNum) / (maxPriceNum - minPriceNum)) * 100;
    }, [currentPriceNum, hasRange, maxPriceNum, minPriceNum]);

    const clampedPercent = rawPercent === null ? null : Math.max(0, Math.min(100, rawPercent));
    const midPrice = Number.isFinite(minPriceNum) && Number.isFinite(maxPriceNum) ? (minPriceNum + maxPriceNum) / 2 : null;

    const outOfRangeInfo = useMemo(() => {
        if (!hasRange) return null;
        if (currentPriceNum > maxPriceNum) {
            const base = Math.abs(maxPriceNum) > 0 ? Math.abs(maxPriceNum) : 1;
            return { direction: 'above', percent: Math.max(0, ((currentPriceNum - maxPriceNum) / base) * 100) };
        }
        if (currentPriceNum < minPriceNum) {
            const base = Math.abs(minPriceNum) > 0 ? Math.abs(minPriceNum) : 1;
            return { direction: 'below', percent: Math.max(0, ((minPriceNum - currentPriceNum) / base) * 100) };
        }
        return null;
    }, [currentPriceNum, hasRange, maxPriceNum, minPriceNum]);

    const visualInRange = hasRange ? !outOfRangeInfo : Boolean(inRange);

    const statusText = useMemo(() => {
        const currentLabel = `${TEXT.currentPrice} ${formatPrice(currentPriceNum)}`;
        if (!Number.isFinite(currentPriceNum)) return `${currentLabel} · ${TEXT.unavailable}`;
        if (visualInRange) return `${currentLabel} · ${TEXT.inRange}`;
        if (!outOfRangeInfo) return `${currentLabel} · ${TEXT.outOfRange}`;
        if (outOfRangeInfo.direction === 'above') return `${currentLabel} · ${TEXT.aboveUpper} ${formatPercent(outOfRangeInfo.percent)}`;
        return `${currentLabel} · ${TEXT.belowLower} ${formatPercent(outOfRangeInfo.percent)}`;
    }, [currentPriceNum, outOfRangeInfo, visualInRange]);

    const currentRangeText = useMemo(() => {
        const lower = Number(currentGridLower);
        const upper = Number(currentGridUpper);
        if (!Number.isFinite(lower) || !Number.isFinite(upper)) return '';
        return `${TEXT.currentBand} ${formatPrice(lower)} - ${formatPrice(upper)}`;
    }, [currentGridLower, currentGridUpper]);

    const detailText = useMemo(() => {
        const parts = [];
        if (taskRangeText) parts.push(`${TEXT.taskRange} ${taskRangeText}`);
        if (currentRangeText) parts.push(currentRangeText);
        if (runningDuration) parts.push(`${TEXT.running} ${runningDuration}`);
        return parts.join(' · ');
    }, [currentRangeText, runningDuration, taskRangeText]);

    const gridLines = useMemo(() => {
        if (!hasGridCount || Number(gridCount) < 2 || Number(gridCount) > 200) return [];
        const lines = [];
        for (let i = 1; i < Number(gridCount); i += 1) {
            lines.push((i / Number(gridCount)) * 100);
        }
        return lines;
    }, [gridCount, hasGridCount]);

    const visibleGridLines = useMemo(() => {
        if (gridLines.length <= 40) return gridLines;
        const step = Math.ceil(gridLines.length / 40);
        return gridLines.filter((_, index) => (index + 1) % step === 0);
    }, [gridLines]);

    return (
        <div className="mt-2 rounded-xl border border-zinc-200/60 bg-[#1c1e22]/5 p-2.5 dark:border-white/5 dark:bg-[#1f2227]">
            <div className="mb-2 flex flex-wrap items-center justify-between gap-x-2 gap-y-1 text-zinc-900 dark:text-zinc-100">
                <div className="text-[10px] font-bold opacity-85">
                    {TEXT.rangeTitle} ({pairLabel || TEXT.unknown}
                    {hasGridCount ? (
                        <>
                            {' · '}
                            {TEXT.total}
                            <NumberFlowValue value={Number(gridCount)} formatOptions={{ maximumFractionDigits: 0 }} />
                            {` ${TEXT.grids}`}
                        </>
                    ) : null}
                    {Number.isFinite(gridStepPct) ? (
                        <>
                            {' · '}
                            {TEXT.approx}
                            <NumberFlowValue value={gridStepPct} formatter={(v) => `${Number(v).toFixed(2)}${TEXT.perGrid}`} />
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
                <span className="text-emerald-600 dark:text-emerald-500">{TEXT.lower}</span>
                <span className="text-zinc-400 dark:text-zinc-500">{TEXT.middle}</span>
                <span className="text-rose-600 dark:text-rose-500">{TEXT.upper}</span>
            </div>

            <div className="relative flex h-4 items-center overflow-hidden rounded-full bg-[#e4e4e7] shadow-inner dark:bg-[#333539]">
                <div className="absolute bottom-0 left-[3%] top-0 w-[2px] bg-emerald-500" />
                <div className="absolute bottom-0 right-[3%] top-0 w-[2px] bg-rose-500" />

                <div className="absolute bottom-0 left-[3%] right-[3%] top-0 flex items-end pb-1 opacity-40">
                    {visibleGridLines.map((pct, index) => (
                        <div
                            key={index}
                            className="absolute h-2 w-[1px] bg-zinc-500"
                            style={{ left: `${pct}%`, transform: 'translateX(-50%)' }}
                        />
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
                    <NumberFlowValue value={minPriceNum} formatter={(v) => formatPrice(v)} />
                </span>
                <span className="text-zinc-500 dark:text-zinc-400">
                    <NumberFlowValue value={midPrice} formatter={(v) => formatPrice(v)} />
                </span>
                <span className="text-rose-600 dark:text-rose-500">
                    <NumberFlowValue value={maxPriceNum} formatter={(v) => formatPrice(v)} />
                </span>
            </div>

            <div
                className={`mt-2 rounded-lg border px-2.5 py-2 text-[10px] font-semibold ${
                    visualInRange
                        ? 'border-emerald-200 bg-emerald-500/8 dark:border-emerald-500/20'
                        : 'border-rose-200 bg-rose-500/8 dark:border-rose-500/20'
                }`}
            >
                <div
                    className={`truncate whitespace-nowrap leading-relaxed ${
                        visualInRange ? 'text-emerald-600 dark:text-emerald-400' : 'text-rose-600 dark:text-rose-400'
                    }`}
                    title={statusText}
                >
                    <NumberFlowValue value={statusText} formatter={() => statusText} />
                </div>
                {detailText ? (
                    <div
                        className="mt-1 truncate whitespace-nowrap leading-relaxed text-zinc-700 dark:text-zinc-300"
                        title={detailText}
                    >
                        <NumberFlowValue value={detailText} formatter={() => detailText} />
                    </div>
                ) : null}
            </div>
        </div>
    );
}
