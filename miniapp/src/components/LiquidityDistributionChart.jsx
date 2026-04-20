import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';

const DEFAULT_HEIGHT = 220;
const HANDLE_HIT_PX = 18;

function safeBigInt(value) {
    try {
        if (value === null || value === undefined) return 0n;
        const trimmed = String(value).trim();
        if (!trimmed) return 0n;
        return BigInt(trimmed);
    } catch {
        return 0n;
    }
}

function bigIntToNumber(value) {
    try {
        if (typeof value === 'bigint') {
            if (value === 0n) return 0;
            // 把超大 BigInt 缩到 Number 安全范围内做归一化时按比例处理
            const max = BigInt(Number.MAX_SAFE_INTEGER);
            if (value <= max) return Number(value);
            const ratio = Number(value / max);
            return ratio * Number.MAX_SAFE_INTEGER;
        }
        return Number(value) || 0;
    } catch {
        return 0;
    }
}

function tickToPriceRatio(tick, token0Decimals = 18, token1Decimals = 18) {
    if (!Number.isFinite(tick)) return null;
    const decimalsAdj = Math.pow(10, (Number(token0Decimals) || 18) - (Number(token1Decimals) || 18));
    return Math.pow(1.0001, tick) * decimalsAdj;
}

function formatPriceCompact(value) {
    const n = Number(value);
    if (!Number.isFinite(n) || n <= 0) return '--';
    if (n >= 1_000_000) return n.toExponential(3);
    if (n >= 1) return n.toLocaleString(undefined, { maximumFractionDigits: 4 });
    if (n >= 0.0001) return n.toLocaleString(undefined, { maximumFractionDigits: 6 });
    return n.toExponential(3);
}

function clampTick(tick, min, max) {
    if (!Number.isFinite(tick)) return min;
    if (tick < min) return min;
    if (tick > max) return max;
    return tick;
}

/**
 * LiquidityDistributionChart
 *
 * 视觉：柱状液体分布 + 当前价虚线 + 选中区间高亮 + 可拖拽端点。
 *
 * props:
 *   - bins: [{ index, tick_lower, tick_upper, liquidity (string), is_active }]
 *   - currentTick: number
 *   - tickSpacing: number
 *   - rangeLowerTick / rangeUpperTick: number (可选；为空则不显示区间)
 *   - onRangeChange?: ({ lower, upper }) => void
 *   - height?: number (px)
 *   - token0Decimals / token1Decimals: number (用于 tick→price 换算)
 *   - invertPrice?: boolean (toggle 单价方向)
 *   - accent?: 'emerald' | 'sky' | 'violet'
 *   - className?: string
 */
export default function LiquidityDistributionChart({
    bins = [],
    currentTick = null,
    tickSpacing = null,
    rangeLowerTick = null,
    rangeUpperTick = null,
    onRangeChange,
    height = DEFAULT_HEIGHT,
    token0Decimals = 18,
    token1Decimals = 18,
    invertPrice = false,
    accent = 'emerald',
    className = '',
    loading = false,
    emptyText = '暂无流动性数据',
}) {
    const containerRef = useRef(null);
    const [containerWidth, setContainerWidth] = useState(0);
    const [draggingHandle, setDraggingHandle] = useState(null);

    useEffect(() => {
        const el = containerRef.current;
        if (!el) return undefined;
        const ro = new ResizeObserver((entries) => {
            for (const entry of entries) {
                setContainerWidth(entry.contentRect.width);
            }
        });
        ro.observe(el);
        setContainerWidth(el.getBoundingClientRect().width);
        return () => ro.disconnect();
    }, []);

    const sortedBins = useMemo(() => {
        if (!Array.isArray(bins) || bins.length === 0) return [];
        return [...bins].sort((a, b) => (a.tick_lower ?? 0) - (b.tick_lower ?? 0));
    }, [bins]);

    const tickRange = useMemo(() => {
        if (sortedBins.length === 0) return null;
        const min = sortedBins[0].tick_lower;
        const max = sortedBins[sortedBins.length - 1].tick_upper;
        return { min, max, span: max - min };
    }, [sortedBins]);

    const liquidityValues = useMemo(() => sortedBins.map((b) => safeBigInt(b.liquidity)), [sortedBins]);
    const maxLiqNumber = useMemo(() => {
        let max = 0;
        for (const v of liquidityValues) {
            const n = bigIntToNumber(v);
            if (n > max) max = n;
        }
        return max || 1;
    }, [liquidityValues]);

    const tickToX = useCallback((tick) => {
        if (!tickRange || tickRange.span === 0 || containerWidth === 0) return 0;
        const t = clampTick(tick, tickRange.min, tickRange.max);
        return ((t - tickRange.min) / tickRange.span) * containerWidth;
    }, [tickRange, containerWidth]);

    const xToTick = useCallback((x) => {
        if (!tickRange || tickRange.span === 0 || containerWidth === 0) return tickRange?.min ?? 0;
        const ratio = Math.max(0, Math.min(1, x / containerWidth));
        const raw = tickRange.min + ratio * tickRange.span;
        if (Number.isFinite(tickSpacing) && tickSpacing > 0) {
            return Math.round(raw / tickSpacing) * tickSpacing;
        }
        return Math.round(raw);
    }, [tickRange, containerWidth, tickSpacing]);

    const accentClasses = useMemo(() => {
        switch (accent) {
            case 'sky':
                return { in: 'from-sky-400/85 to-sky-500/40', out: 'from-zinc-400/40 to-zinc-500/15' };
            case 'violet':
                return { in: 'from-violet-400/85 to-violet-500/40', out: 'from-zinc-400/40 to-zinc-500/15' };
            default:
                return { in: 'from-emerald-400/90 to-emerald-500/45', out: 'from-zinc-400/35 to-zinc-500/10' };
        }
    }, [accent]);

    const handlePointerMove = useCallback((event) => {
        if (!draggingHandle) return;
        if (!containerRef.current) return;
        const rect = containerRef.current.getBoundingClientRect();
        const x = event.clientX - rect.left;
        const tick = xToTick(x);
        if (typeof onRangeChange !== 'function') return;
        if (draggingHandle === 'lower') {
            const upperBound = Number.isFinite(rangeUpperTick) ? rangeUpperTick - (tickSpacing || 1) : tickRange.max;
            onRangeChange({ lower: clampTick(tick, tickRange.min, upperBound), upper: rangeUpperTick });
        } else if (draggingHandle === 'upper') {
            const lowerBound = Number.isFinite(rangeLowerTick) ? rangeLowerTick + (tickSpacing || 1) : tickRange.min;
            onRangeChange({ lower: rangeLowerTick, upper: clampTick(tick, lowerBound, tickRange.max) });
        }
    }, [draggingHandle, xToTick, onRangeChange, rangeLowerTick, rangeUpperTick, tickRange, tickSpacing]);

    const handlePointerUp = useCallback(() => setDraggingHandle(null), []);

    useEffect(() => {
        if (!draggingHandle) return undefined;
        window.addEventListener('pointermove', handlePointerMove);
        window.addEventListener('pointerup', handlePointerUp);
        window.addEventListener('pointercancel', handlePointerUp);
        return () => {
            window.removeEventListener('pointermove', handlePointerMove);
            window.removeEventListener('pointerup', handlePointerUp);
            window.removeEventListener('pointercancel', handlePointerUp);
        };
    }, [draggingHandle, handlePointerMove, handlePointerUp]);

    const lowerX = Number.isFinite(rangeLowerTick) ? tickToX(rangeLowerTick) : null;
    const upperX = Number.isFinite(rangeUpperTick) ? tickToX(rangeUpperTick) : null;
    const currentX = Number.isFinite(currentTick) ? tickToX(currentTick) : null;

    const startBinPriceText = useMemo(() => {
        if (!tickRange) return '';
        const price = tickToPriceRatio(tickRange.min, token0Decimals, token1Decimals);
        const display = invertPrice && price ? 1 / price : price;
        return formatPriceCompact(display);
    }, [tickRange, token0Decimals, token1Decimals, invertPrice]);

    const endBinPriceText = useMemo(() => {
        if (!tickRange) return '';
        const price = tickToPriceRatio(tickRange.max, token0Decimals, token1Decimals);
        const display = invertPrice && price ? 1 / price : price;
        return formatPriceCompact(display);
    }, [tickRange, token0Decimals, token1Decimals, invertPrice]);

    const currentPriceText = useMemo(() => {
        if (!Number.isFinite(currentTick)) return '';
        const price = tickToPriceRatio(currentTick, token0Decimals, token1Decimals);
        const display = invertPrice && price ? 1 / price : price;
        return formatPriceCompact(display);
    }, [currentTick, token0Decimals, token1Decimals, invertPrice]);

    if (loading) {
        return (
            <div
                ref={containerRef}
                className={`relative flex items-center justify-center rounded-xl border border-white/8 bg-gradient-to-b from-zinc-900/40 to-zinc-900/10 text-xs text-zinc-400 ${className}`}
                style={{ height }}
            >
                <div className="flex items-center gap-2">
                    <div className="h-3 w-3 animate-spin rounded-full border-2 border-emerald-400/40 border-t-emerald-400" />
                    流动性分布加载中...
                </div>
            </div>
        );
    }

    if (!sortedBins.length || !tickRange) {
        return (
            <div
                ref={containerRef}
                className={`relative flex items-center justify-center rounded-xl border border-white/8 bg-gradient-to-b from-zinc-900/40 to-zinc-900/10 text-xs text-zinc-400 ${className}`}
                style={{ height }}
            >
                {emptyText}
            </div>
        );
    }

    const inRange = (bin) => {
        if (!Number.isFinite(rangeLowerTick) || !Number.isFinite(rangeUpperTick)) return true;
        return bin.tick_upper > rangeLowerTick && bin.tick_lower < rangeUpperTick;
    };

    return (
        <div
            ref={containerRef}
            className={`relative overflow-hidden rounded-xl border border-white/10 bg-gradient-to-b from-zinc-900/60 via-zinc-900/30 to-zinc-900/10 dark:from-black/40 dark:via-black/20 dark:to-transparent ${className}`}
            style={{ height }}
        >
            {/* 区间高亮背景 */}
            {Number.isFinite(lowerX) && Number.isFinite(upperX) && upperX > lowerX ? (
                <div
                    className="pointer-events-none absolute inset-y-0 bg-emerald-500/10"
                    style={{ left: lowerX, width: upperX - lowerX }}
                />
            ) : null}

            {/* 柱子层 */}
            <div className="absolute inset-x-2 bottom-6 top-3 flex items-end gap-[2px]">
                {sortedBins.map((bin) => {
                    const liq = bigIntToNumber(safeBigInt(bin.liquidity));
                    const ratio = liq / maxLiqNumber;
                    const heightPct = Math.max(2, ratio * 100);
                    const inside = inRange(bin);
                    const colorClass = inside ? accentClasses.in : accentClasses.out;
                    return (
                        <div
                            key={bin.index ?? `${bin.tick_lower}-${bin.tick_upper}`}
                            className={`flex-1 rounded-t-sm bg-gradient-to-t ${colorClass} transition-all duration-150`}
                            style={{ height: `${heightPct}%`, minWidth: 2 }}
                            title={`tick [${bin.tick_lower}, ${bin.tick_upper}) · L=${bin.liquidity}`}
                        />
                    );
                })}
            </div>

            {/* 区间端点：虚线 + 拖拽手柄 */}
            {Number.isFinite(lowerX) ? (
                <RangeHandle
                    x={lowerX}
                    color="emerald"
                    side="lower"
                    onDown={() => onRangeChange && setDraggingHandle('lower')}
                    interactive={typeof onRangeChange === 'function'}
                />
            ) : null}
            {Number.isFinite(upperX) ? (
                <RangeHandle
                    x={upperX}
                    color="rose"
                    side="upper"
                    onDown={() => onRangeChange && setDraggingHandle('upper')}
                    interactive={typeof onRangeChange === 'function'}
                />
            ) : null}

            {/* 当前价虚线 */}
            {Number.isFinite(currentX) ? (
                <div
                    className="pointer-events-none absolute inset-y-2 z-10 w-0 border-l border-dashed border-amber-300/85"
                    style={{ left: currentX, boxShadow: '0 0 8px rgba(252, 211, 77, 0.35)' }}
                />
            ) : null}

            {/* 底部价格刻度 */}
            <div className="pointer-events-none absolute inset-x-2 bottom-1 flex items-center justify-between text-[10px] font-mono text-zinc-400">
                <span className="rounded bg-black/30 px-1 py-0.5">{startBinPriceText}</span>
                {currentPriceText ? (
                    <span className="rounded bg-amber-500/15 px-1 py-0.5 text-amber-300 ring-1 ring-amber-400/30">
                        当前 {currentPriceText}
                    </span>
                ) : null}
                <span className="rounded bg-black/30 px-1 py-0.5">{endBinPriceText}</span>
            </div>
        </div>
    );
}

function RangeHandle({ x, color = 'emerald', side, onDown, interactive }) {
    const lineColor = color === 'rose' ? 'border-rose-400' : 'border-emerald-400';
    const dotColor = color === 'rose' ? 'bg-rose-400' : 'bg-emerald-400';
    return (
        <div
            className={`absolute inset-y-0 z-20 ${interactive ? 'cursor-ew-resize' : ''}`}
            style={{ left: x - HANDLE_HIT_PX / 2, width: HANDLE_HIT_PX }}
            onPointerDown={(e) => {
                if (!interactive) return;
                e.preventDefault();
                onDown?.();
            }}
        >
            <div className={`pointer-events-none absolute inset-y-0 left-1/2 w-0 -translate-x-1/2 border-l-2 ${lineColor}`} />
            <div
                className={`pointer-events-none absolute left-1/2 top-1 h-3 w-3 -translate-x-1/2 rounded-full ${dotColor} ring-2 ring-zinc-900/60 shadow-[0_0_8px_rgba(0,0,0,0.4)]`}
            />
            <div
                className={`pointer-events-none absolute left-1/2 bottom-7 h-3 w-3 -translate-x-1/2 rounded-full ${dotColor} ring-2 ring-zinc-900/60 shadow-[0_0_8px_rgba(0,0,0,0.4)]`}
            />
            <div
                className={`pointer-events-none absolute left-1/2 top-1/2 h-7 w-7 -translate-x-1/2 -translate-y-1/2 rounded-full ${color === 'rose' ? 'bg-rose-500/15' : 'bg-emerald-500/15'} blur-sm`}
            />
            <div className={`pointer-events-none absolute left-1/2 -translate-x-1/2 -top-1.5 px-1 py-0.5 rounded text-[9px] font-bold ${side === 'lower' ? 'text-emerald-300' : 'text-rose-300'}`}>
                {side === 'lower' ? '下' : '上'}
            </div>
        </div>
    );
}
