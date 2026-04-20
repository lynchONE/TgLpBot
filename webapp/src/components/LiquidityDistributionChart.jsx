import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';

const DEFAULT_HEIGHT = 240;
const HANDLE_HIT_PX = 20;

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
            const max = BigInt(Number.MAX_SAFE_INTEGER);
            if (value <= max) return Number(value);
            return Number(value / max) * Number.MAX_SAFE_INTEGER;
        }
        return Number(value) || 0;
    } catch {
        return 0;
    }
}

function tickToPriceRatio(tick, t0Decimals = 18, t1Decimals = 18) {
    if (!Number.isFinite(tick)) return null;
    const adj = Math.pow(10, (Number(t0Decimals) || 18) - (Number(t1Decimals) || 18));
    return Math.pow(1.0001, tick) * adj;
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

const colors = {
    container: 'rgba(15, 21, 32, 0.6)',
    border: 'rgba(134, 153, 184, 0.2)',
    barInside: 'linear-gradient(to top, rgba(34, 211, 138, 0.85), rgba(34, 211, 138, 0.35))',
    barOutside: 'linear-gradient(to top, rgba(154, 168, 196, 0.45), rgba(154, 168, 196, 0.12))',
    rangeBg: 'rgba(34, 211, 138, 0.1)',
    handleLower: '#22d38a',
    handleUpper: '#ff5e76',
    currentLine: 'rgba(255, 196, 0, 0.85)',
    currentGlow: '0 0 10px rgba(255, 196, 0, 0.45)',
    priceTagBg: 'rgba(0, 0, 0, 0.45)',
    priceTagText: '#ecf2ff',
    currentTagBg: 'rgba(255, 196, 0, 0.18)',
    currentTagBorder: 'rgba(255, 196, 0, 0.45)',
    currentTagText: '#ffd166',
    emptyText: '#9aa8c4',
};

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
    loading = false,
    emptyText = '暂无流动性数据',
    style,
}) {
    const containerRef = useRef(null);
    const [width, setWidth] = useState(0);
    const [draggingHandle, setDraggingHandle] = useState(null);

    useEffect(() => {
        const el = containerRef.current;
        if (!el) return undefined;
        const ro = new ResizeObserver((entries) => {
            for (const entry of entries) setWidth(entry.contentRect.width);
        });
        ro.observe(el);
        setWidth(el.getBoundingClientRect().width);
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

    const maxLiq = useMemo(() => {
        let m = 0;
        for (const b of sortedBins) {
            const n = bigIntToNumber(safeBigInt(b.liquidity));
            if (n > m) m = n;
        }
        return m || 1;
    }, [sortedBins]);

    const tickToX = useCallback((tick) => {
        if (!tickRange || tickRange.span === 0 || width === 0) return 0;
        const t = clampTick(tick, tickRange.min, tickRange.max);
        return ((t - tickRange.min) / tickRange.span) * width;
    }, [tickRange, width]);

    const xToTick = useCallback((x) => {
        if (!tickRange || tickRange.span === 0 || width === 0) return tickRange?.min ?? 0;
        const ratio = Math.max(0, Math.min(1, x / width));
        const raw = tickRange.min + ratio * tickRange.span;
        if (Number.isFinite(tickSpacing) && tickSpacing > 0) {
            return Math.round(raw / tickSpacing) * tickSpacing;
        }
        return Math.round(raw);
    }, [tickRange, width, tickSpacing]);

    const onPointerMove = useCallback((event) => {
        if (!draggingHandle || !containerRef.current) return;
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

    const onPointerUp = useCallback(() => setDraggingHandle(null), []);

    useEffect(() => {
        if (!draggingHandle) return undefined;
        window.addEventListener('pointermove', onPointerMove);
        window.addEventListener('pointerup', onPointerUp);
        window.addEventListener('pointercancel', onPointerUp);
        return () => {
            window.removeEventListener('pointermove', onPointerMove);
            window.removeEventListener('pointerup', onPointerUp);
            window.removeEventListener('pointercancel', onPointerUp);
        };
    }, [draggingHandle, onPointerMove, onPointerUp]);

    const lowerX = Number.isFinite(rangeLowerTick) ? tickToX(rangeLowerTick) : null;
    const upperX = Number.isFinite(rangeUpperTick) ? tickToX(rangeUpperTick) : null;
    const currentX = Number.isFinite(currentTick) ? tickToX(currentTick) : null;

    const startPriceText = useMemo(() => {
        if (!tickRange) return '';
        const p = tickToPriceRatio(tickRange.min, token0Decimals, token1Decimals);
        return formatPriceCompact(invertPrice && p ? 1 / p : p);
    }, [tickRange, token0Decimals, token1Decimals, invertPrice]);

    const endPriceText = useMemo(() => {
        if (!tickRange) return '';
        const p = tickToPriceRatio(tickRange.max, token0Decimals, token1Decimals);
        return formatPriceCompact(invertPrice && p ? 1 / p : p);
    }, [tickRange, token0Decimals, token1Decimals, invertPrice]);

    const currentPriceText = useMemo(() => {
        if (!Number.isFinite(currentTick)) return '';
        const p = tickToPriceRatio(currentTick, token0Decimals, token1Decimals);
        return formatPriceCompact(invertPrice && p ? 1 / p : p);
    }, [currentTick, token0Decimals, token1Decimals, invertPrice]);

    const containerStyle = {
        position: 'relative',
        overflow: 'hidden',
        borderRadius: 'var(--radius-md, 12px)',
        border: `1px solid ${colors.border}`,
        background: 'linear-gradient(to bottom, rgba(15,21,32,0.7), rgba(15,21,32,0.3))',
        height,
        ...style,
    };

    if (loading) {
        return (
            <div ref={containerRef} style={{ ...containerStyle, display: 'flex', alignItems: 'center', justifyContent: 'center', color: colors.emptyText, fontSize: 12 }}>
                <span style={{ display: 'inline-flex', alignItems: 'center', gap: 8 }}>
                    <span style={{ width: 12, height: 12, borderRadius: '50%', border: '2px solid rgba(34,211,138,0.4)', borderTopColor: 'rgba(34,211,138,1)', animation: 'lpd-spin 0.8s linear infinite' }} />
                    流动性分布加载中...
                </span>
                <style>{`@keyframes lpd-spin { to { transform: rotate(360deg); } }`}</style>
            </div>
        );
    }

    if (!sortedBins.length || !tickRange) {
        return (
            <div ref={containerRef} style={{ ...containerStyle, display: 'flex', alignItems: 'center', justifyContent: 'center', color: colors.emptyText, fontSize: 12 }}>
                {emptyText}
            </div>
        );
    }

    const inRange = (bin) => {
        if (!Number.isFinite(rangeLowerTick) || !Number.isFinite(rangeUpperTick)) return true;
        return bin.tick_upper > rangeLowerTick && bin.tick_lower < rangeUpperTick;
    };

    return (
        <div ref={containerRef} style={containerStyle}>
            {Number.isFinite(lowerX) && Number.isFinite(upperX) && upperX > lowerX ? (
                <div
                    style={{ position: 'absolute', top: 0, bottom: 0, left: lowerX, width: upperX - lowerX, background: colors.rangeBg, pointerEvents: 'none' }}
                />
            ) : null}

            <div style={{ position: 'absolute', left: 8, right: 8, top: 12, bottom: 26, display: 'flex', alignItems: 'flex-end', gap: 2 }}>
                {sortedBins.map((bin) => {
                    const liq = bigIntToNumber(safeBigInt(bin.liquidity));
                    const ratio = liq / maxLiq;
                    const heightPct = Math.max(2, ratio * 100);
                    const inside = inRange(bin);
                    return (
                        <div
                            key={bin.index ?? `${bin.tick_lower}-${bin.tick_upper}`}
                            title={`tick [${bin.tick_lower}, ${bin.tick_upper}) · L=${bin.liquidity}`}
                            style={{
                                flex: 1,
                                minWidth: 2,
                                height: `${heightPct}%`,
                                borderTopLeftRadius: 2,
                                borderTopRightRadius: 2,
                                background: inside ? colors.barInside : colors.barOutside,
                                transition: 'all 150ms ease',
                            }}
                        />
                    );
                })}
            </div>

            {Number.isFinite(lowerX) ? (
                <RangeHandle
                    x={lowerX}
                    color={colors.handleLower}
                    side="lower"
                    interactive={typeof onRangeChange === 'function'}
                    onDown={() => setDraggingHandle('lower')}
                />
            ) : null}
            {Number.isFinite(upperX) ? (
                <RangeHandle
                    x={upperX}
                    color={colors.handleUpper}
                    side="upper"
                    interactive={typeof onRangeChange === 'function'}
                    onDown={() => setDraggingHandle('upper')}
                />
            ) : null}

            {Number.isFinite(currentX) ? (
                <div style={{
                    position: 'absolute', top: 8, bottom: 24, left: currentX, width: 0,
                    borderLeft: `1px dashed ${colors.currentLine}`,
                    boxShadow: colors.currentGlow,
                    pointerEvents: 'none', zIndex: 10,
                }} />
            ) : null}

            <div style={{
                position: 'absolute', left: 8, right: 8, bottom: 4,
                display: 'flex', justifyContent: 'space-between', alignItems: 'center',
                fontSize: 10, fontFamily: 'ui-monospace, SFMono-Regular, monospace', pointerEvents: 'none',
            }}>
                <span style={{ background: colors.priceTagBg, color: colors.priceTagText, padding: '2px 6px', borderRadius: 4 }}>{startPriceText}</span>
                {currentPriceText ? (
                    <span style={{ background: colors.currentTagBg, color: colors.currentTagText, padding: '2px 6px', borderRadius: 4, border: `1px solid ${colors.currentTagBorder}` }}>
                        当前 {currentPriceText}
                    </span>
                ) : null}
                <span style={{ background: colors.priceTagBg, color: colors.priceTagText, padding: '2px 6px', borderRadius: 4 }}>{endPriceText}</span>
            </div>
        </div>
    );
}

function RangeHandle({ x, color, side, interactive, onDown }) {
    return (
        <div
            style={{
                position: 'absolute', top: 0, bottom: 0,
                left: x - HANDLE_HIT_PX / 2, width: HANDLE_HIT_PX,
                cursor: interactive ? 'ew-resize' : 'default',
                zIndex: 20,
            }}
            onPointerDown={(e) => {
                if (!interactive) return;
                e.preventDefault();
                onDown?.();
            }}
        >
            <div style={{ position: 'absolute', top: 0, bottom: 0, left: '50%', width: 0, borderLeft: `2px solid ${color}`, transform: 'translateX(-1px)', pointerEvents: 'none' }} />
            <div style={{ position: 'absolute', top: 4, left: '50%', width: 12, height: 12, borderRadius: '50%', background: color, transform: 'translateX(-50%)', boxShadow: `0 0 8px ${color}99`, pointerEvents: 'none' }} />
            <div style={{ position: 'absolute', bottom: 28, left: '50%', width: 12, height: 12, borderRadius: '50%', background: color, transform: 'translateX(-50%)', boxShadow: `0 0 8px ${color}99`, pointerEvents: 'none' }} />
            <div style={{
                position: 'absolute', top: -2, left: '50%', transform: 'translateX(-50%)',
                fontSize: 9, fontWeight: 700, color, padding: '1px 4px', borderRadius: 3, pointerEvents: 'none',
            }}>{side === 'lower' ? '下' : '上'}</div>
        </div>
    );
}
