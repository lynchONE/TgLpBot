import React, { useMemo } from 'react';
import { useDurationFrom, useRelativeTime } from '../lib/time';

const formatPrice = (value) => {
    const n = Number(value);
    if (!Number.isFinite(n)) return '--';
    if (n === 0) return '0';
    // Format to 6 significant digits or suitable precision
    return n.toPrecision(6).replace(/\.?0+$/, '');
};

/**
 * Visualizes the price range with a progress bar and status indicators.
 * 
 * Props:
 * - currentPrice: Number
 * - minPrice: Number
 * - maxPrice: Number
 * - token0: { symbol, decimals }
 * - token1: { symbol, decimals }
 * - tickLower: Number
 * - tickUpper: Number
 * - tickSpacing: Number (optional)
 * - inRange: Boolean
 */
export default function PriceRangeVisualizer({
    currentPrice,
    minPrice,
    maxPrice,
    token0,
    token1,
    tickLower,
    tickUpper,
    tickSpacing,
    inRange
}) {
    // 1. Calculate Grid Count (Tick Spacing Multiples)
    const gridCount = useMemo(() => {
        const lower = Number(tickLower);
        const upper = Number(tickUpper);
        if (!Number.isFinite(lower) || !Number.isFinite(upper)) return null;

        const diff = Math.abs(upper - lower);
        // If tickSpacing is provided, use it
        if (tickSpacing && tickSpacing > 0) {
            return Math.round(diff / tickSpacing);
        }
        // Otherwise, if diff is small enough, maybe it is the count itself (unlikely for V3 ticks unless processed)
        // For now, we return null if unknown
        return null;
    }, [tickLower, tickUpper, tickSpacing]);

    // 2. Identify pair symbol
    // Assuming prices are already inverted if needed by parent
    // We just display what is given.
    // Parent should handle "USDT/FST" vs "FST/USDT" ordering before passing min/max/current.

    // Calculate progress for the bar
    // Range: 0% = minPrice, 100% = maxPrice.
    // But we want to show out of range too?
    // User image shows a bar. If current is out of range, verify behavior.
    // Logic: 
    // The bar represents the *Range*.
    // If Price < Min, the marker is at 0% (or slightly left/hidden?).
    // User image: A red vertical line is visible.
    // Let's us clamp the marker to 0-100% but change color/icon if out.
    // Wait, the user image shows:
    // Green (Lower), Grey (Mid), Red (Upper) TICKS on the bar.
    // And a separate RED vertical BAR for current price.
    // If current price is out of range (e.g. lower), the text says "Low 10%".
    // I will render the bar representing [Min, Max].
    // Current Price marker will be positioned relative to this.

    const percent = useMemo(() => {
        if (!Number.isFinite(currentPrice) || !Number.isFinite(minPrice) || !Number.isFinite(maxPrice)) return 50;
        if (maxPrice === minPrice) return 50;
        const p = ((currentPrice - minPrice) / (maxPrice - minPrice)) * 100;
        return Math.max(0, Math.min(100, p));
    }, [currentPrice, minPrice, maxPrice]);

    // Calculate Deviation for title (e.g. ±6.00%)
    // Usually (max - min) / 2 / mid * 100 ? Or just (max/min - 1) ?
    const deviation = useMemo(() => {
        if (!Number.isFinite(minPrice) || !Number.isFinite(maxPrice)) return null;
        const mid = (minPrice + maxPrice) / 2;
        if (mid === 0) return null;
        const range = maxPrice - minPrice;
        // half range percent
        return ((range / 2) / mid) * 100;
    }, [minPrice, maxPrice]);

    const outOfRangePercent = useMemo(() => {
        if (inRange) return null;
        if (!Number.isFinite(currentPrice) || !Number.isFinite(minPrice) || !Number.isFinite(maxPrice)) return null;

        // If price < min
        if (currentPrice < minPrice) {
            return ((minPrice - currentPrice) / minPrice) * 100;
        }
        // If price > max
        if (currentPrice > maxPrice) {
            return ((currentPrice - maxPrice) / maxPrice) * 100;
        }
        return 0;
    }, [currentPrice, minPrice, maxPrice, inRange]);

    const isBelow = currentPrice < minPrice;

    // Mid price
    const midPrice = (minPrice + maxPrice) / 2;

    return (
        <div className="w-full">
            {/* Header */}
            <div className="flex justify-between items-center mb-2">
                <div className="text-xs font-semibold text-zinc-700 dark:text-zinc-300">
                    价格范围 {gridCount ? `(${gridCount}格)` : ''}:
                </div>
                <div className="bg-zinc-100 dark:bg-zinc-800 text-zinc-600 dark:text-zinc-400 text-xs px-1.5 py-0.5 rounded">
                    {deviation ? `${deviation.toFixed(2)}%` : '--'}
                </div>
            </div>

            {/* Labels Row (Top) */}
            <div className="flex justify-between text-[10px] font-medium mb-1">
                <span className="text-emerald-500">下限</span>
                <span className="text-zinc-500">中心</span>
                <span className="text-rose-500">上限</span>
            </div>

            {/* Visual Bar */}
            <div className="relative h-8 w-full select-none mb-1">
                {/* Track */}
                <div className="absolute top-1/2 left-0 right-0 h-4 -mt-2 bg-zinc-200 dark:bg-zinc-700 rounded-full overflow-hidden">
                    {/* Lower Marker Line */}
                    <div className="absolute top-0 bottom-0 w-0.5 bg-emerald-500 z-10" style={{ left: '10%' }}></div>
                    {/* Mid Marker Line */}
                    <div className="absolute top-0 bottom-0 w-0.5 bg-zinc-400 z-10" style={{ left: '50%' }}></div>
                    {/* Upper Marker Line */}
                    <div className="absolute top-0 bottom-0 w-0.5 bg-rose-500 z-10" style={{ left: '90%' }}></div>

                    {/* We map [Min, Max] to [10%, 90%] of the visual width to allow space for "Out of range" viz? 
                        Actually, user image shows ticks inside the bar.
                        Let's treat the *whole bar* as the range for simplicity first, 
                        or add padding.
                        If I use 0% and 100% as min/max, then checks differ.
                        
                        Refining based on image:
                        The image shows a bar.
                        Green line at some offset. Red line at some offset.
                        Gray line in middle.
                        Maybe the bar represents a wider scope?
                        No, usually the bar is the range.
                        Let's stick to Bar = Range (0% to 100%).
                     */}
                </div>

                {/* Re-rendering Track Logic for Bar = Range */}
                <div className="absolute top-1/2 left-0 right-0 h-4 -mt-2 bg-zinc-200 dark:bg-zinc-700 rounded-full border border-zinc-300 dark:border-zinc-600">
                    {/* Ticks within the bar */}
                    {/* Min - 0% */}
                    <div className="absolute top-0 bottom-0 w-0.5 bg-emerald-500 left-0"></div>
                    {/* Mid - 50% */}
                    <div className="absolute top-0 bottom-0 w-0.5 bg-zinc-400 left-1/2 -ml-px"></div>
                    {/* Max - 100% */}
                    <div className="absolute right-0 top-0 bottom-0 w-0.5 bg-rose-500"></div>
                </div>

                {/* Current Price Marker */}
                <div
                    className={`absolute top-0 bottom-0 w-1 z-20 transition-all duration-500 ${inRange
                            ? 'bg-emerald-500 shadow-[0_0_4px_rgba(16,185,129,0.5)]'
                            : 'bg-rose-600 dark:bg-rose-500 shadow-[0_0_4px_rgba(225,29,72,0.5)]'
                        }`}
                    style={{
                        left: `${percent}%`,
                        opacity: 1
                    }}
                ></div>
            </div>

            {/* Price Values Row */}
            <div className="flex justify-between text-[11px] font-bold tabular-nums">
                <span className="text-emerald-500">{formatPrice(minPrice)}</span>
                <span className="text-zinc-500">{formatPrice(midPrice)}</span>
                <span className="text-rose-500">{formatPrice(maxPrice)}</span>
            </div>

            {/* Out of Range Status */}
            {!inRange && (
                <div className="mt-3 space-y-2">
                    {/* Range Info Box */}
                    <div className="bg-zinc-100 dark:bg-zinc-800 rounded-lg py-1.5 px-3 flex items-center justify-center text-xs font-medium text-zinc-700 dark:text-zinc-300">
                        <span className="text-orange-600 dark:text-orange-400 mr-2 font-bold">超出范围</span>
                        <span className="tabular-nums opacity-80"> | {formatPrice(minPrice)} - {formatPrice(maxPrice)}</span>
                    </div>

                    {/* Alert Box */}
                    <div className="bg-rose-50 dark:bg-rose-900/20 rounded-lg p-2 flex items-center justify-center text-xs text-rose-700 dark:text-rose-300">
                        <span className="mr-1">{isBelow ? '⬇' : '⬆'}</span>
                        <span>价格 {formatPrice(currentPrice)} {isBelow ? '低于下限' : '高于上限'} </span>
                        <span className="font-bold ml-1">{outOfRangePercent?.toFixed(3)}%</span>
                    </div>
                </div>
            )}
        </div>
    );
}
