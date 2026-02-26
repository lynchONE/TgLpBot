import React, { useEffect, useMemo, useRef, useState } from 'react';

const formatPrice = (value) => {
    const n = Number(value);
    if (!Number.isFinite(n)) return '--';
    if (n === 0) return '0';
    return n.toPrecision(6).replace(/\.?0+$/, '');
};

// 根据 tick spacing 推断费率标签
const feeRateFromTickSpacing = (ts) => {
    const map = { 1: '0.01%', 10: '0.05%', 50: '0.25%', 60: '0.30%', 100: '0.50%', 200: '1%', 2000: '2%' };
    return map[ts] ?? null;
};

// 把 [0,100] 映射到 [4,96]，给两侧留一点 padding
const mapPct = (p) => 4 + p * 0.92;

export default function PriceRangeVisualizer({
    currentPrice,
    openPrice,
    minPrice,
    maxPrice,
    token0,
    token1,
    tickLower,
    tickUpper,
    tickSpacing,
    inRange,
    pollIntervalSec,
    runningDuration,
    updateTimeText,
}) {
    // 价格变化时触发动画脉冲
    const [isPulse, setIsPulse] = useState(false);
    const prevPriceRef = useRef(currentPrice);

    useEffect(() => {
        if (prevPriceRef.current !== currentPrice && Number.isFinite(currentPrice)) {
            setIsPulse(true);
            const timer = setTimeout(() => setIsPulse(false), 400);
            prevPriceRef.current = currentPrice;
            return () => clearTimeout(timer);
        }
    }, [currentPrice]);

    // 格数（tick 区间 / tick spacing）
    const gridCount = useMemo(() => {
        const lower = Number(tickLower);
        const upper = Number(tickUpper);
        if (!Number.isFinite(lower) || !Number.isFinite(upper)) return null;
        const diff = Math.abs(upper - lower);
        if (tickSpacing && tickSpacing > 0) return Math.round(diff / tickSpacing);
        return null;
    }, [tickLower, tickUpper, tickSpacing]);

    // 费率标签
    const feeLabel = useMemo(() => {
        const ts = Number(tickSpacing);
        if (!Number.isFinite(ts) || ts <= 0) return null;
        return feeRateFromTickSpacing(ts);
    }, [tickSpacing]);

    // 偏移幅度
    const deviation = useMemo(() => {
        if (!Number.isFinite(minPrice) || !Number.isFinite(maxPrice)) return null;
        const mid = (minPrice + maxPrice) / 2;
        if (mid === 0) return null;
        return ((maxPrice - minPrice) / 2 / mid) * 100;
    }, [minPrice, maxPrice]);

    // 当前价映射到 [0,100]（超出区间时 clamp 到 0/100）
    const rawPercent = useMemo(() => {
        if (!Number.isFinite(currentPrice) || !Number.isFinite(minPrice) || !Number.isFinite(maxPrice)) return null;
        if (maxPrice === minPrice) return 50;
        return ((currentPrice - minPrice) / (maxPrice - minPrice)) * 100;
    }, [currentPrice, minPrice, maxPrice]);

    // clamp 后用于指针位置
    const clampedPercent = rawPercent === null ? null : Math.max(0, Math.min(100, rawPercent));
    const finalPercent = clampedPercent === null ? null : mapPct(clampedPercent);

    // 开仓价映射
    const openRawPercent = useMemo(() => {
        if (!Number.isFinite(openPrice) || !Number.isFinite(minPrice) || !Number.isFinite(maxPrice)) return null;
        if (maxPrice === minPrice) return null;
        const p = ((openPrice - minPrice) / (maxPrice - minPrice)) * 100;
        if (!Number.isFinite(p) || p < 0 || p > 100) return null;
        return p;
    }, [openPrice, minPrice, maxPrice]);
    const openFinalPercent = openRawPercent === null ? null : mapPct(openRawPercent);

    // 超出范围偏差
    const outOfRangePercent = useMemo(() => {
        if (inRange || rawPercent === null) return null;
        if (rawPercent < 0) return Math.abs(rawPercent);
        if (rawPercent > 100) return rawPercent - 100;
        return 0;
    }, [inRange, rawPercent]);
    const isBelow = Number.isFinite(currentPrice) && Number.isFinite(minPrice) && currentPrice < minPrice;

    const midPrice = Number.isFinite(minPrice) && Number.isFinite(maxPrice) ? (minPrice + maxPrice) / 2 : null;

    // 生成格子分隔线位置数组（在 bar 内侧 4%-96% 之间均匀分布）
    // gridLines 是内部分割线（不含两端边界），数量 = gridCount - 1
    const gridLines = useMemo(() => {
        if (!gridCount || gridCount < 2 || gridCount > 60) return [];
        const lines = [];
        for (let i = 1; i < gridCount; i++) {
            // 格子内部百分比位置（相对于 bar 区间 [0,100]）
            const pct = (i / gridCount) * 100;
            lines.push(mapPct(pct));
        }
        return lines;
    }, [gridCount]);

    // 显示的格子数（超多时缩减，避免太密）
    const visibleGridLines = useMemo(() => {
        if (gridLines.length <= 20) return gridLines;
        // 每隔 n 条显示一条
        const step = Math.ceil(gridLines.length / 20);
        return gridLines.filter((_, i) => (i + 1) % step === 0);
    }, [gridLines]);

    // 指针颜色方案
    const pointerColor = inRange
        ? { bg: '#10b981', shadow: 'rgba(16,185,129,0.7)', ring: 'rgba(16,185,129,0.25)' }
        : { bg: '#ef4444', shadow: 'rgba(239,68,68,0.7)', ring: 'rgba(239,68,68,0.2)' };

    const priceTextColor = inRange
        ? 'text-emerald-500 dark:text-emerald-400'
        : 'text-rose-500 dark:text-rose-400';

    return (
        <div className="mt-2.5 rounded-xl border border-zinc-100/80 bg-zinc-50/80 p-3 dark:border-white/10 dark:bg-[#0f1116]">

            {/* ── 标题行 ── */}
            <div className="flex items-center justify-between gap-2 mb-3">
                <div className="flex items-center gap-1.5 flex-wrap">
                    <span className="text-[11px] font-semibold text-zinc-600 dark:text-white/60 uppercase tracking-wide">
                        价格范围
                    </span>
                    {/* 费率 badge */}
                    {feeLabel && (
                        <span className="text-[10px] font-semibold rounded-md bg-violet-500/12 px-1.5 py-0.5 text-violet-600 dark:bg-violet-500/15 dark:text-violet-300">
                            {feeLabel}
                        </span>
                    )}
                    {/* 格数 badge */}
                    {gridCount ? (
                        <span className="text-[10px] font-medium rounded-md bg-zinc-200/70 px-1.5 py-0.5 text-zinc-500 dark:bg-white/10 dark:text-white/40">
                            {gridCount} 格
                        </span>
                    ) : null}
                    {/* 幅度 badge */}
                    {deviation ? (
                        <span className="text-[10px] font-medium rounded-md bg-zinc-200/70 px-1.5 py-0.5 text-zinc-500 dark:bg-white/10 dark:text-white/40">
                            ±{deviation.toFixed(2)}%
                        </span>
                    ) : null}
                </div>

                {/* 当前价格大字 */}
                <div className={`text-sm font-extrabold tabular-nums leading-none transition-colors duration-300 ${priceTextColor}`}>
                    {Number.isFinite(currentPrice) ? formatPrice(currentPrice) : '--'}
                </div>
            </div>

            {/* ════════════════════════════════════
                价格轨道主体
            ════════════════════════════════════ */}
            <div className="relative select-none" style={{ height: 44 }}>

                {/* ── 轨道底层 ── */}
                <div
                    className="absolute inset-x-0 rounded-full overflow-hidden"
                    style={{ top: '50%', transform: 'translateY(-50%)', height: 20 }}
                >
                    {/* 基础背景 */}
                    <div className="absolute inset-0 bg-zinc-200/90 dark:bg-white/10" />

                    {/* 区间激活填充：in-range 时亮绿，out 时暗红 */}
                    <div
                        className={`absolute top-0 bottom-0 transition-colors duration-500 ${inRange
                            ? 'bg-gradient-to-r from-emerald-400/25 via-emerald-300/30 to-emerald-400/25 dark:from-emerald-500/20 dark:via-emerald-400/25 dark:to-emerald-500/20'
                            : 'bg-gradient-to-r from-rose-400/10 via-rose-300/15 to-rose-400/10 dark:from-rose-500/10 dark:via-rose-400/12 dark:to-rose-500/10'
                            }`}
                        style={{ left: '4%', right: '4%' }}
                    />

                    {/* 格子分隔线（内部，半透明细线） */}
                    {visibleGridLines.map((pos, i) => (
                        <div
                            key={i}
                            className="absolute top-0 bottom-0"
                            style={{
                                left: `${pos}%`,
                                width: 1,
                                background: 'rgba(0,0,0,0.10)',
                            }}
                        />
                    ))}
                    {/* 深色模式下格子线 */}
                    {visibleGridLines.map((pos, i) => (
                        <div
                            key={`d${i}`}
                            className="absolute top-0 bottom-0 dark:block hidden"
                            style={{
                                left: `${pos}%`,
                                width: 1,
                                background: 'rgba(255,255,255,0.12)',
                            }}
                        />
                    ))}

                    {/* 左边界（下限，绿） */}
                    <div
                        className="absolute top-0 bottom-0 bg-emerald-500"
                        style={{ left: '4%', width: 2 }}
                    />
                    {/* 右边界（上限，红） */}
                    <div
                        className="absolute top-0 bottom-0 bg-rose-500"
                        style={{ left: '96%', width: 2 }}
                    />
                </div>

                {/* ── 开仓价标记 ── */}
                {openFinalPercent !== null && (
                    <div
                        className="absolute top-0 bottom-0 z-10"
                        style={{
                            left: `${openFinalPercent}%`,
                            transform: 'translateX(-50%)',
                            width: 2,
                            background: 'linear-gradient(to bottom, transparent 0%, #38bdf8 20%, #38bdf8 80%, transparent 100%)',
                            opacity: 0.9,
                        }}
                        title={`开仓价 ${formatPrice(openPrice)}`}
                    />
                )}

                {/* ── 当前价格指针（精致滑块样式）── */}
                {finalPercent !== null && (
                    <div
                        className="absolute top-1/2 z-20"
                        style={{
                            left: `${finalPercent}%`,
                            transform: 'translateX(-50%) translateY(-50%)',
                            transition: 'left 0.6s cubic-bezier(0.25, 0.46, 0.45, 0.94)',
                        }}
                    >
                        {/* 外圈光晕（pulse 时更大） */}
                        <div
                            className="absolute rounded-full"
                            style={{
                                width: isPulse ? 28 : 22,
                                height: isPulse ? 28 : 22,
                                top: '50%',
                                left: '50%',
                                transform: 'translate(-50%, -50%)',
                                background: pointerColor.ring,
                                transition: 'all 0.3s ease',
                            }}
                        />
                        {/* 中圈（半透明） */}
                        <div
                            className="absolute rounded-full"
                            style={{
                                width: 16,
                                height: 16,
                                top: '50%',
                                left: '50%',
                                transform: 'translate(-50%, -50%)',
                                background: pointerColor.bg,
                                opacity: 0.3,
                            }}
                        />
                        {/* 核心圆点 */}
                        <div
                            style={{
                                width: 10,
                                height: 10,
                                borderRadius: '50%',
                                background: pointerColor.bg,
                                boxShadow: `0 0 ${isPulse ? 12 : 8}px ${pointerColor.shadow}, 0 0 0 2px white`,
                                transition: 'box-shadow 0.3s ease',
                            }}
                        />
                        {/* 上方小三角标（指向轨道）*/}
                        <div
                            className="absolute"
                            style={{
                                left: '50%',
                                bottom: '100%',
                                transform: 'translateX(-50%)',
                                marginBottom: 3,
                                width: 0,
                                height: 0,
                                borderLeft: '4px solid transparent',
                                borderRight: '4px solid transparent',
                                borderTop: `5px solid ${pointerColor.bg}`,
                                opacity: 0.9,
                            }}
                        />
                    </div>
                )}
            </div>

            {/* ── 价格标签行 ── */}
            <div className="mt-1 flex items-start justify-between text-[10px] font-bold tabular-nums px-0.5">
                {/* 下限 */}
                <div className="text-left">
                    <div className="text-emerald-500 dark:text-emerald-400">{formatPrice(minPrice)}</div>
                    <div className="text-[9px] font-normal text-zinc-400 dark:text-white/25 mt-0.5">下限</div>
                </div>

                {/* 中间：开仓价 或 中间价 */}
                <div className="text-center">
                    {openFinalPercent !== null ? (
                        <>
                            <div className="text-sky-500 dark:text-sky-400">{formatPrice(openPrice)}</div>
                            <div className="text-[9px] font-normal text-sky-400/70 dark:text-sky-400/60 mt-0.5">开仓</div>
                        </>
                    ) : midPrice !== null ? (
                        <>
                            <div className="text-zinc-400 dark:text-white/30">{formatPrice(midPrice)}</div>
                            <div className="text-[9px] font-normal text-zinc-300 dark:text-white/20 mt-0.5">中点</div>
                        </>
                    ) : null}
                </div>

                {/* 上限 */}
                <div className="text-right">
                    <div className="text-rose-500 dark:text-rose-400">{formatPrice(maxPrice)}</div>
                    <div className="text-[9px] font-normal text-zinc-400 dark:text-white/25 mt-0.5">上限</div>
                </div>
            </div>

            {/* ── 超出范围提示 ── */}
            {!inRange && (
                <div className="mt-2 flex items-center gap-1.5 rounded-lg border border-rose-500/20 bg-rose-500/8 px-2.5 py-1.5 dark:border-rose-400/15 dark:bg-rose-500/10">
                    <span className="text-rose-500 text-sm">{isBelow ? '⬇' : '⬆'}</span>
                    <span className="text-[11px] text-rose-600 dark:text-rose-400">
                        价格 {formatPrice(currentPrice)} {isBelow ? '低于下限' : '高于上限'}
                    </span>
                    <span className="text-[11px] font-bold text-rose-600 dark:text-rose-400 ml-auto tabular-nums">
                        {outOfRangePercent?.toFixed(2)}%
                    </span>
                </div>
            )}

            {/* ── 底部统计行（紧凑横排）── */}
            <div className="mt-2.5 flex items-center border-t border-zinc-200/60 dark:border-white/10 pt-2 text-[10px]">
                <span className="text-zinc-400 dark:text-white/35">刷新</span>
                <span className="ml-1 font-bold text-zinc-600 dark:text-white/60 tabular-nums">{pollIntervalSec}s</span>
                <span className="mx-2 text-zinc-300 dark:text-white/15">·</span>
                <span className="text-zinc-400 dark:text-white/35">运行</span>
                <span className="ml-1 font-bold text-emerald-600 dark:text-emerald-400 tabular-nums">{runningDuration}</span>
                <span className="mx-2 text-zinc-300 dark:text-white/15">·</span>
                <span className="ml-auto text-zinc-400 dark:text-white/35 tabular-nums">{updateTimeText}</span>
            </div>
        </div>
    );
}
