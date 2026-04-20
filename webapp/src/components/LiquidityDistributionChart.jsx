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
    barActive: 'linear-gradient(to top, rgba(255, 196, 0, 0.95), rgba(255, 196, 0, 0.55))',
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

function formatLiquidityCompact(value) {
    try {
        const bi = typeof value === 'bigint' ? value : BigInt(String(value || '0').trim() || '0');
        if (bi === 0n) return '0';
        const sign = bi < 0n ? '-' : '';
        const abs = bi < 0n ? -bi : bi;
        const str = abs.toString();
        if (str.length <= 3) return sign + str;
        const units = ['', 'K', 'M', 'B', 'T', 'Qa', 'Qi', 'Sx', 'Sp', 'Oc', 'No', 'Dc'];
        const unitIdx = Math.floor((str.length - 1) / 3);
        if (unitIdx >= units.length) return sign + str.slice(0, str.length - (units.length - 1) * 3) + 'e' + ((units.length - 1) * 3);
        const intLen = str.length - unitIdx * 3;
        const head = str.slice(0, intLen);
        const tail = str.slice(intLen, intLen + 2).replace(/0+$/, '');
        return sign + head + (tail ? '.' + tail : '') + units[unitIdx];
    } catch {
        return '0';
    }
}

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
    const [hoveredBin, setHoveredBin] = useState(null);

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

            <div style={{ position: 'absolute', left: 8, right: 8, top: 18, bottom: 26, display: 'flex', alignItems: 'flex-end', gap: 2 }}>
                {sortedBins.map((bin, i) => {
                    const liq = bigIntToNumber(safeBigInt(bin.liquidity));
                    const ratio = liq / maxLiq;
                    const heightPct = Math.max(2, ratio * 100);
                    const inside = inRange(bin);
                    const isActive = Boolean(bin.is_active);
                    const isHovered = hoveredBin?.index === bin.index;
                    const bg = isActive ? colors.barActive : (inside ? colors.barInside : colors.barOutside);
                    return (
                        <div
                            key={bin.index ?? `${bin.tick_lower}-${bin.tick_upper}`}
                            onPointerEnter={() => setHoveredBin({ index: bin.index, bin, barIdx: i })}
                            onPointerLeave={() => setHoveredBin((prev) => (prev?.index === bin.index ? null : prev))}
                            style={{
                                flex: 1,
                                minWidth: 2,
                                height: `${heightPct}%`,
                                borderTopLeftRadius: 2,
                                borderTopRightRadius: 2,
                                background: bg,
                                transition: 'all 150ms ease',
                                outline: isHovered ? '1px solid rgba(236, 242, 255, 0.55)' : 'none',
                                cursor: 'default',
                            }}
                        />
                    );
                })}
            </div>

            {(() => {
                if (!Number.isFinite(currentTick) || !tickRange) return null;
                const activeBin = sortedBins.find((b) => b.is_active)
                    || sortedBins.find((b) => currentTick >= b.tick_lower && currentTick < b.tick_upper);
                if (!activeBin) return null;
                const centerTick = (activeBin.tick_lower + activeBin.tick_upper) / 2;
                const cx = tickToX(centerTick);
                return (
                    <div style={{ position: 'absolute', left: cx - 8, top: 0, width: 16, height: 14, pointerEvents: 'none', zIndex: 12 }}>
                        <svg viewBox="0 0 16 14" width="16" height="14">
                            <path d="M 8 12 L 2 2 L 14 2 Z" fill="rgba(255, 196, 0, 0.95)" stroke="rgba(30, 30, 30, 0.5)" strokeWidth="0.6" />
                        </svg>
                    </div>
                );
            })()}

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

            {hoveredBin?.bin ? (() => {
                const bin = hoveredBin.bin;
                const center = (bin.tick_lower + bin.tick_upper) / 2;
                const cx = tickToX(center);
                const priceLower = tickToPriceRatio(bin.tick_lower, token0Decimals, token1Decimals);
                const priceUpper = tickToPriceRatio(bin.tick_upper, token0Decimals, token1Decimals);
                const lowerDisp = invertPrice && priceLower ? 1 / priceLower : priceLower;
                const upperDisp = invertPrice && priceUpper ? 1 / priceUpper : priceUpper;
                const liquidityText = formatLiquidityCompact(bin.liquidity);
                const tooltipWidth = 170;
                const tipLeft = Math.max(4, Math.min((width || 0) - tooltipWidth - 4, cx - tooltipWidth / 2));
                return (
                    <div style={{
                        position: 'absolute', top: 18, left: tipLeft, width: tooltipWidth,
                        padding: '8px 10px', borderRadius: 10,
                        background: 'rgba(10, 14, 22, 0.96)', border: '1px solid rgba(134, 153, 184, 0.35)',
                        boxShadow: '0 8px 20px rgba(0, 0, 0, 0.45)',
                        fontSize: 11, lineHeight: 1.5, color: '#ecf2ff',
                        pointerEvents: 'none', zIndex: 30,
                    }}>
                        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'baseline' }}>
                            <span style={{ color: '#9aa8c4' }}>区间</span>
                            {bin.is_active ? <span style={{ fontSize: 9, fontWeight: 700, color: '#ffd166' }}>当前</span> : null}
                        </div>
                        <div style={{ fontFamily: 'ui-monospace, SFMono-Regular, monospace', fontSize: 11 }}>
                            {formatPriceCompact(lowerDisp)} → {formatPriceCompact(upperDisp)}
                        </div>
                        <div style={{ display: 'flex', justifyContent: 'space-between', marginTop: 4 }}>
                            <span style={{ color: '#9aa8c4' }}>Tick</span>
                            <span style={{ fontFamily: 'ui-monospace, SFMono-Regular, monospace' }}>{bin.tick_lower} ~ {bin.tick_upper}</span>
                        </div>
                        <div style={{ display: 'flex', justifyContent: 'space-between' }}>
                            <span style={{ color: '#9aa8c4' }}>流动性 L</span>
                            <span style={{ fontFamily: 'ui-monospace, SFMono-Regular, monospace', color: '#bcff2f', fontWeight: 700 }}>{liquidityText}</span>
                        </div>
                    </div>
                );
            })() : null}
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
