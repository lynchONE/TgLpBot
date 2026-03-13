import React, { useMemo, useState, useRef, useCallback } from 'react';
import { copyToClipboard, hapticNotification, hapticImpact, openLink } from '../lib/telegram';
import uniswapIcon from '../image/uniswap.svg';
import pancakeIcon from '../image/pancake.svg';
import gmgnIcon from '../image/gmgn.svg';
import flashIcon from '../image/flash.svg';
import NumberFlowValue from './NumberFlowValue.jsx';
import { brandTextClass } from '../lib/brand';

const Icon = ({ path, className = '' }) => (
    <svg viewBox="0 0 24 24" fill="currentColor" className={className} aria-hidden="true">
        <path d={path} />
    </svg>
);

const icons = {
    copy: 'M16 1H4a2 2 0 00-2 2v14h2V3h12V1zm3 4H8a2 2 0 00-2 2v14a2 2 0 002 2h11a2 2 0 002-2V7a2 2 0 00-2-2zm0 16H8V7h11v14z',
    chart: 'M5 3v18h18v-2H7V3H5zm5 14H8v-6h2v6zm4 0h-2V7h2v10zm2 0h2v-4h-2v4z',
    arrowUp: 'M5.293 9.707a1 1 0 010-1.414l4-4a1 1 0 011.414 0l4 4a1 1 0 01-1.414 1.414L11 7.414V15a1 1 0 11-2 0V7.414L6.707 9.707a1 1 0 01-1.414 0z',
    arrowDown: 'M14.707 10.293a1 1 0 010 1.414l-4 4a1 1 0 01-1.414 0l-4-4a1 1 0 111.414-1.414L9 12.586V5a1 1 0 012 0v7.586l2.293-2.293a1 1 0 011.414 0z',
};

// Uniswap 图标组件 - 使用官方图标
const UniswapIcon = ({ className = '' }) => (
    <img src={uniswapIcon} alt="Uniswap" className={className} />
);

// PancakeSwap 图标组件 - 使用官方图标
const PancakeIcon = ({ className = '' }) => (
    <img src={pancakeIcon} alt="PancakeSwap" className={className} />
);

// 获取交易所图标和样式配置
function getDexIconConfig(factoryName) {
    const name = String(factoryName || '').toLowerCase();

    if (name.includes('uniswap')) {
        // 提取版本号
        const versionMatch = name.match(/v(\d+)/i);
        const version = versionMatch ? `V${versionMatch[1]}` : '';
        return {
            icon: UniswapIcon,
            label: version,
            bgClass: 'bg-pink-500/15',
            textClass: 'text-pink-600 dark:text-pink-300',
            ringClass: 'ring-pink-500/25 dark:ring-pink-500/30',
        };
    }

    if (name.includes('pancake') || name.includes('pcs')) {
        const versionMatch = name.match(/v(\d+)/i);
        const version = versionMatch ? `V${versionMatch[1]}` : '';
        return {
            icon: PancakeIcon,
            label: version,
            bgClass: 'bg-amber-500/15',
            textClass: 'text-amber-700 dark:text-amber-300',
            ringClass: 'ring-amber-500/25 dark:ring-amber-500/30',
        };
    }

    // 默认样式（其他交易所）
    return {
        icon: null,
        label: factoryName || 'DEX',
        bgClass: 'bg-violet-500/15',
        textClass: 'text-violet-700 dark:text-violet-300',
        ringClass: 'ring-violet-500/25 dark:ring-violet-500/30',
    };
}

// 交易所标签组件
const DexBadge = ({ pool }) => {
    const factoryName = String(pool?.factory_name || '').trim();
    const config = getDexIconConfig(factoryName || dexLabel(pool));
    const IconComponent = config.icon;

    return (
        <div className={`inline-flex items-center gap-1 rounded-lg ${config.bgClass} px-2 py-0.5 text-[11px] font-semibold ${config.textClass} ring-1 ${config.ringClass}`}>
            {IconComponent && <IconComponent className="w-4 h-4" />}
            <span>{config.label || dexLabel(pool)}</span>
        </div>
    );
};

const usdCompact = new Intl.NumberFormat('en-US', {
    style: 'currency',
    currency: 'USD',
    notation: 'compact',
    maximumFractionDigits: 2,
});
const USD_DISPLAY_LIMIT = 1e15;
const usdFormatter = new Intl.NumberFormat('en-US', {
    style: 'currency',
    currency: 'USD',
    maximumFractionDigits: 2,
});

const isPoolAddressLike = (v) => /^(0x)?[a-fA-F0-9]{40}$/.test(String(v || '').trim()) || /^(0x)?[a-fA-F0-9]{64}$/.test(String(v || '').trim());

function formatUsd(v) {
    const n = Number(v ?? 0);
    if (!Number.isFinite(n) || Math.abs(n) > USD_DISPLAY_LIMIT) return '$--';
    return usdFormatter.format(n);
}

function formatUsdCompact(v) {
    const n = Number(v || 0);
    if (!Number.isFinite(n)) return '$0.00';
    return usdCompact.format(n);
}

// 持仓标签组件
const PositionBadge = ({ pool }) => {
    const usd = Number(pool?.userPositionUsd || 0);
    if (usd <= 0) return null;

    return (
        <div className="inline-flex items-center gap-1 rounded-lg bg-purple-500/15 px-2 py-0.5 text-[11px] font-bold text-purple-700 ring-1 ring-purple-500/25 dark:bg-purple-500/20 dark:text-purple-200 dark:ring-purple-500/30">
            <span>
                💰 持仓 <NumberFlowValue value={usd} formatter={(v) => formatUsdCompact(v)} />
            </span>
        </div>
    );
};

function formatFeePercent(v) {
    const n = Number(v || 0);
    if (!Number.isFinite(n) || n <= 0) return '';
    return `${n.toFixed(2).replace(/\.?0+$/, '')}%`;
}

function formatRatePct(v) {
    const n = Number(v || 0);
    if (!Number.isFinite(n)) return '<0.01%';
    if (Math.abs(n) < 0.01) return '<0.01%';
    return `${n.toFixed(3)}%`;
}

function normalizeDexName(dex) {
    const v = String(dex || '').trim().toLowerCase();
    if (!v) return '';
    if (v.includes('pancake') || v === 'pcs') return 'pancake';
    if (v.includes('uniswap') || v === 'uni') return 'uniswap';
    if (v.includes('sushi')) return 'sushi';
    return v.replace(/[^a-z0-9]+/g, '');
}

function normalizeProtocolVersion(protocolVersion, dex) {
    const proto = String(protocolVersion || '').trim().toLowerCase();
    const fromProto = proto.match(/v?\d+/)?.[0] ?? '';
    if (fromProto) return fromProto.startsWith('v') ? fromProto : `v${fromProto}`;
    const dx = String(dex || '').trim().toLowerCase();
    const fromDex = dx.match(/v\d+/)?.[0] ?? '';
    return fromDex;
}

function dexLabel(pool) {
    // 优先使用 factory_name
    const factoryName = String(pool?.factory_name || '').trim();
    if (factoryName) {
        return factoryName;
    }
    // 回退到原来的逻辑
    const base = normalizeDexName(pool?.dex);
    const version = normalizeProtocolVersion(pool?.protocol_version, pool?.dex);
    if (!base && !version) return 'DEX';
    if (!base) return version.toUpperCase();
    return `${base}${version || ''}`;
}

function formatPairLabel(tradingPair) {
    const v = String(tradingPair || '').trim();
    if (!v) return '--';
    return v.replace(/\//g, '/\u200B');
}

// 通用变化指示器组件 - 用于显示数值变化（费用、交易量等）
// 如果本轮数据无变化(diff===0)，保持上次的变化箭头不消失
const ChangeIndicator = ({ currentValue, previousValue, label = '变化' }) => {
    const lastRef = useRef(null);
    if (previousValue === undefined || previousValue === null) {
        return lastRef.current ? lastRef.current.el : null;
    }

    const current = Number(currentValue || 0);
    const previous = Number(previousValue || 0);
    const diff = current - previous;

    if (!Number.isFinite(diff)) return lastRef.current ? lastRef.current.el : null;

    // diff===0 时使用上次缓存的结果
    if (diff === 0 && lastRef.current) return lastRef.current.el;
    if (diff === 0) return null;

    const isIncrease = diff > 0;
    const absValue = Math.abs(diff);

    // 格式化数字显示
    const formatValue = (val) => {
        if (val >= 1000) {
            return usdCompact.format(val).replace('$', '');
        }
        return val.toFixed(2);
    };

    const el = (
        <span
            className={`ml-1 inline-flex items-center text-[10px] font-bold ${isIncrease ? 'text-emerald-600 dark:text-emerald-400' : 'text-rose-600 dark:text-rose-400'
                }`}
            title={`${label}: ${isIncrease ? '+' : '-'}$${absValue.toFixed(2)}`}
        >
            <svg className="w-2.5 h-2.5" fill="currentColor" viewBox="0 0 20 20">
                <path fillRule="evenodd" d={isIncrease ? icons.arrowUp : icons.arrowDown} clipRule="evenodd" />
            </svg>
            <NumberFlowValue value={absValue} formatter={(v) => formatValue(v)} />
        </span>
    );
    lastRef.current = { el };
    return el;
};

// 数量变化指示器组件 - 用于显示交易笔数等非美元数值的变化
// 如果本轮数据无变化(diff===0)，保持上次的变化箭头不消失
const CountChangeIndicator = ({ currentValue, previousValue, label = '变化' }) => {
    const lastRef = useRef(null);
    if (previousValue === undefined || previousValue === null) {
        return lastRef.current ? lastRef.current.el : null;
    }

    const current = Number(currentValue || 0);
    const previous = Number(previousValue || 0);
    const diff = current - previous;

    if (!Number.isFinite(diff)) return lastRef.current ? lastRef.current.el : null;

    // diff===0 时使用上次缓存的结果
    if (diff === 0 && lastRef.current) return lastRef.current.el;
    if (diff === 0) return null;

    const isIncrease = diff > 0;
    const absValue = Math.abs(diff);

    // 格式化数量显示（无美元符号）
    const formatCount = (val) => {
        if (val >= 1000000) {
            return (val / 1000000).toFixed(1) + 'M';
        }
        if (val >= 1000) {
            return (val / 1000).toFixed(1) + 'K';
        }
        return Math.round(val).toString();
    };

    const el = (
        <span
            className={`ml-1 inline-flex items-center text-[10px] font-bold ${isIncrease ? 'text-emerald-600 dark:text-emerald-400' : 'text-rose-600 dark:text-rose-400'
                }`}
            title={`${label}: ${isIncrease ? '+' : '-'}${absValue.toLocaleString()}`}
        >
            <svg className="w-2.5 h-2.5" fill="currentColor" viewBox="0 0 20 20">
                <path fillRule="evenodd" d={isIncrease ? icons.arrowUp : icons.arrowDown} clipRule="evenodd" />
            </svg>
            <NumberFlowValue value={absValue} formatter={(v) => formatCount(v)} />
        </span>
    );
    lastRef.current = { el };
    return el;
};

const STABLE_COINS = ['usdc', 'usdt', 'busd', 'dai', 'frax', 'usdd', 'fdusd', 'wbnb', 'weth', 'wsol', 'bnb', 'eth', 'sol'];

export default function HotPoolCard({ pool, metric, previousData, onOpenKline, onOpenPosition, onBlacklist, onBlacklistRequest, rank, apiBaseUrl, isBlacklisted = false, chain }) {
    const [copied, setCopied] = useState(false);
    const addr = String(pool?.pool_address || '').trim();
    const canOpenKline = useMemo(() => isPoolAddressLike(addr), [addr]);

    // 左滑触发黑名单
    const swipeRef = useRef({ x: 0, y: 0, triggered: false });
    const swipeThreshold = 60;
    const swipeSlack = 12;

    const handleTouchStart = useCallback((e) => {
        const touch = e.touches?.[0];
        if (!touch) return;
        swipeRef.current = { x: touch.clientX, y: touch.clientY, triggered: false };
    }, []);

    const handleTouchMove = useCallback((e) => {
        const touch = e.touches?.[0];
        if (!touch) return;
        const dx = touch.clientX - swipeRef.current.x;
        const dy = touch.clientY - swipeRef.current.y;
        if (swipeRef.current.triggered) return;
        if (dx < -swipeThreshold && Math.abs(dx) > Math.abs(dy) + swipeSlack) {
            swipeRef.current.triggered = true;
            hapticImpact('heavy');
            if (onBlacklistRequest && typeof onBlacklistRequest === 'function') {
                onBlacklistRequest(pool);
                return;
            }
            if (isBlacklisted) {
                hapticNotification('warning');
                return;
            }
            if (onBlacklist && typeof onBlacklist === 'function') {
                onBlacklist(pool, true);
            }
        }
    }, [pool, onBlacklist, onBlacklistRequest, isBlacklisted]);

    const handleTouchEnd = useCallback(() => {
        swipeRef.current = { x: 0, y: 0, triggered: false };
    }, []);

    // 根据排名确定渐变背景类
    const rankClass = useMemo(() => {
        if (rank === 1) return 'rank-gold';
        if (rank === 2) return 'rank-silver';
        if (rank === 3) return 'rank-bronze';
        return '';
    }, [rank]);

    const gmgnNetwork = useMemo(() => (chain === 'base' ? 'base' : 'bsc'), [chain]);

    const gmgnTokenAddr = useMemo(() => {
        if (!pool || !pool.trading_pair) return null;
        const tokens = pool.trading_pair.split('/').map(t => t.trim().toLowerCase());
        if (tokens.length !== 2) return pool.token0_address || pool.token1_address;

        const t0 = tokens[0];
        const t1 = tokens[1];

        const t0IsStable = STABLE_COINS.includes(t0);
        const t1IsStable = STABLE_COINS.includes(t1);

        // 如果左边是稳定币，右边不是，则返回 token1
        if (t0IsStable && !t1IsStable) return pool.token1_address;
        // 如果右边是稳定币，左边不是，则返回 token0
        if (t1IsStable && !t0IsStable) return pool.token0_address;

        // 如果无法判断或两个都是，默认返回 token0
        return pool.token0_address || pool.token1_address;
    }, [pool, chain]);

    const openGmgn = useCallback(() => {
        if (gmgnTokenAddr) {
            hapticImpact('light');
            openLink(`https://gmgn.ai/${gmgnNetwork}/token/${gmgnTokenAddr}`);
        }
    }, [gmgnTokenAddr, gmgnNetwork]);

    const priceDisplay = useMemo(() => {
        const v = String(pool?.price_display || '').trim();
        return v ? v : '';
    }, [pool?.price_display]);

    const priceDisplayClass = useMemo(() => {
        if (!priceDisplay) return '';
        if (priceDisplay.includes('↓') || priceDisplay.includes('-')) return 'text-rose-600 dark:text-rose-300';
        if (priceDisplay.includes('↑') || priceDisplay.includes('+')) return 'text-emerald-700 dark:text-emerald-300';
        return 'text-zinc-600 dark:text-white/60';
    }, [priceDisplay]);

    const volumeValue = useMemo(() => Number(pool?.total_volume ?? 0), [pool?.total_volume]);
    const tvlValue = useMemo(() => Number(pool?.current_pool_value ?? 0), [pool?.current_pool_value]);
    const feeRateValue = useMemo(() => Number(pool?.fee_rate ?? 0), [pool?.fee_rate]);
    const totalFeesValue = useMemo(() => Number(pool?.total_fees ?? 0), [pool?.total_fees]);
    const showVolume = useMemo(() => Number.isFinite(volumeValue) && volumeValue > 0, [volumeValue]);
    const showTVL = useMemo(() => Number.isFinite(tvlValue) && tvlValue > 0, [tvlValue]);
    const showFeeRate = useMemo(() => Number.isFinite(feeRateValue) && feeRateValue > 0, [feeRateValue]);
    const showTotalFees = useMemo(() => Number.isFinite(totalFeesValue) && totalFeesValue > 0, [totalFeesValue]);
    const secondaryMetricText = useMemo(() => {
        if (metric === 'fee_rate') {
            return showTotalFees ? formatUsd(totalFeesValue) : '';
        }
        return showFeeRate ? formatRatePct(feeRateValue) : '';
    }, [metric, showFeeRate, showTotalFees, feeRateValue, totalFeesValue]);

    const copyAddr = async () => {
        if (!addr) return;
        hapticImpact('light'); // 按钮点击反馈
        try {
            await copyToClipboard(addr);
            setCopied(true);
            hapticNotification('success'); // 复制成功反馈
            setTimeout(() => setCopied(false), 1200);
            await copyToClipboard(addr);
            setCopied(true);
            hapticNotification('success'); // 复制成功反馈
            setTimeout(() => setCopied(false), 1200);
        } catch {
            hapticNotification('error'); // 复制失败反馈
        }
    };

    return (
        <div
            className={`rounded-2xl border border-zinc-200 bg-white/40 backdrop-blur-md p-4 shadow-sm transition-transform duration-200 active:scale-[0.98] dark:border-white/10 dark:bg-white/5 dark:shadow-none ${rankClass} ${isBlacklisted ? 'opacity-50 ring-2 ring-red-500/30' : ''}`}
            onTouchStart={handleTouchStart}
            onTouchEnd={handleTouchEnd}
            onTouchMove={handleTouchMove}
        >
            <div className="flex items-start justify-between gap-3">
                <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-2">
                        <div
                            className="max-w-[90px] text-xs font-semibold leading-4 text-zinc-900 dark:text-white/90 truncate"
                            title={pool?.trading_pair || ''}
                        >
                            {formatPairLabel(pool?.trading_pair)}
                        </div>
                        {pool?.fee_percentage ? (
                            <div className="rounded-lg bg-sky-500/10 px-2 py-0.5 text-[11px] font-semibold text-sky-700 ring-1 ring-sky-500/20 dark:bg-sky-500/15 dark:text-sky-200 dark:ring-sky-500/30">
                                <NumberFlowValue value={pool.fee_percentage} formatter={(v) => formatFeePercent(v)} />
                            </div>
                        ) : null}
                        <button
                            type="button"
                            onClick={copyAddr}
                            className={`inline-flex h-7 w-7 items-center justify-center rounded-xl border text-zinc-600 shadow-sm transition ${copied
                                ? 'border-emerald-500/30 bg-emerald-500/10 text-emerald-700 dark:border-emerald-500/30 dark:bg-emerald-500/15 dark:text-emerald-200'
                                : 'border-zinc-200 bg-zinc-100 hover:bg-zinc-200 active:bg-zinc-200 dark:border-white/10 dark:bg-white/5 dark:text-white/70 dark:hover:bg-white/10 dark:active:bg-white/15'
                                }`}
                            aria-label={copied ? '已复制' : '复制地址'}
                            disabled={!addr}
                        >
                            <Icon path={icons.copy} className="h-4 w-4" />
                        </button>
                        <button
                            type="button"
                            onClick={() => onOpenKline?.(pool)}
                            className="inline-flex h-7 w-7 items-center justify-center rounded-xl border border-zinc-200 bg-zinc-100 text-zinc-600 shadow-sm transition hover:bg-zinc-200 active:bg-zinc-200 disabled:opacity-40 dark:border-white/10 dark:bg-white/5 dark:text-white/70 dark:hover:bg-white/10 dark:active:bg-white/15"
                            aria-label="K线图"
                            title="查看K线图"
                            disabled={!canOpenKline || typeof onOpenKline !== 'function'}
                        >
                            <Icon path={icons.chart} className="h-4 w-4" />
                        </button>
                    </div>

                    <div className="mt-2 text-xs space-y-1">
                        {/* 第一行：交易量 */}
                        {showVolume ? (
                            <div className="text-zinc-500 dark:text-white/40 flex items-center">
                                交易量:{' '}
                                <span className="font-semibold text-sky-600 dark:text-sky-200 tabular-nums">
                                    <NumberFlowValue value={volumeValue} formatter={(v) => formatUsdCompact(v)} />
                                </span>
                                <ChangeIndicator
                                    currentValue={pool?.total_volume}
                                    previousValue={previousData?.total_volume}
                                    label="交易量变化"
                                />
                            </div>
                        ) : null}
                        {/* 第二行：TVL */}
                        {showTVL ? (
                            <div className="text-zinc-500 dark:text-white/40 flex items-center">
                                TVL:{' '}
                                <span className="font-semibold text-zinc-900 dark:text-white/80 tabular-nums">
                                    <NumberFlowValue value={tvlValue} formatter={(v) => formatUsdCompact(v)} />
                                </span>
                                <ChangeIndicator
                                    currentValue={pool?.current_pool_value}
                                    previousValue={previousData?.current_pool_value}
                                    label="TVL变化"
                                />
                            </div>
                        ) : null}
                    </div>
                </div>

                <div className="text-right shrink-0 min-w-[110px]">
                    <div className="flex items-baseline justify-end gap-1 flex-wrap">
                        <div className={`text-base font-extrabold tabular-nums flex items-center ${brandTextClass}`}>
                            {metric === 'volume' ? (
                                <NumberFlowValue value={pool?.total_volume} formatter={(v) => {
                                    const n = Number(v ?? 0);
                                    return Number.isFinite(n) && n > 0 ? formatUsdCompact(n) : '--';
                                }} />
                            ) : metric === 'fee_rate' ? (
                                <NumberFlowValue value={pool?.fee_rate} formatter={(v) => {
                                    const n = Number(v ?? 0);
                                    return Number.isFinite(n) && n > 0 ? formatRatePct(n) : '--';
                                }} />
                            ) : (
                                <NumberFlowValue value={pool?.total_fees} formatter={(v) => {
                                    const n = Number(v ?? 0);
                                    return Number.isFinite(n) && n > 0 ? formatUsd(n) : '--';
                                }} />
                            )}
                            <ChangeIndicator
                                currentValue={metric === 'volume' ? pool?.total_volume : pool?.total_fees}
                                previousValue={metric === 'volume' ? previousData?.total_volume : previousData?.total_fees}
                                label={metric === 'volume' ? '交易量变化' : '费用变化'}
                            />
                        </div>
                    </div>
                    {priceDisplay ? (
                        <div
                            className={`mt-0.5 text-[10px] font-semibold tabular-nums truncate max-w-[110px] ${priceDisplayClass}`}
                            title={priceDisplay}
                        >
                            <NumberFlowValue value={priceDisplay} formatter={() => priceDisplay} />
                        </div>
                    ) : null}
                    {secondaryMetricText ? (
                        <div className="mt-0.5 text-[10px] font-semibold text-violet-600 dark:text-violet-300 tabular-nums">
                            <NumberFlowValue value={secondaryMetricText} formatter={() => secondaryMetricText} />
                        </div>
                    ) : null}
                    {pool?.transaction_count > 0 ? (
                        <div className="mt-0.5 text-[10px] text-zinc-500 dark:text-white/40 flex items-center justify-end">
                            交易笔数:{' '}
                            <span className="font-semibold text-orange-600 dark:text-orange-300 tabular-nums">
                                <NumberFlowValue
                                    value={pool.transaction_count}
                                    formatter={(v) => Number(v || 0).toLocaleString()}
                                />
                            </span>
                            <CountChangeIndicator
                                currentValue={pool?.transaction_count}
                                previousValue={previousData?.transaction_count}
                                label="交易笔数变化"
                            />
                        </div>
                    ) : null}
                </div>
            </div>

            <div className="mt-3 flex items-center justify-between gap-2">
                <div className="flex items-center gap-2 flex-wrap">
                    <DexBadge pool={pool} />
                    {gmgnTokenAddr ? (
                        <button
                            type="button"
                            onClick={(e) => { e.stopPropagation(); openGmgn(); }}
                            className="inline-flex items-center gap-1 rounded-lg bg-zinc-800 px-2 py-0.5 text-[11px] font-semibold text-white ring-1 ring-white/10 dark:bg-[#1a1c23] dark:ring-white/10 transition-colors hover:bg-zinc-700 dark:hover:bg-[#252831]"
                            title="在 GMGN 查看代币"
                        >
                            <img src={gmgnIcon} alt="GMGN" className="h-3.5 w-3.5" />
                            <span>GMGN</span>
                        </button>
                    ) : null}
                    <PositionBadge pool={pool} />
                    {isBlacklisted ? (
                        <div className="inline-flex items-center gap-1 rounded-lg bg-red-500/15 px-2 py-0.5 text-[11px] font-bold text-red-700 ring-1 ring-red-500/25 dark:bg-red-500/20 dark:text-red-200 dark:ring-red-500/30">
                            <span>🚫 黑名单</span>
                        </div>
                    ) : null}
                </div>
                <button
                    type="button"
                    onClick={() => onOpenPosition?.(pool)}
                    disabled={typeof onOpenPosition !== 'function' || isBlacklisted}
                    className="inline-flex items-center gap-1.5 rounded-full border border-black/70 bg-[linear-gradient(180deg,#303811_0%,#252d0d_100%)] px-3 py-1 text-[11px] font-semibold leading-none text-[#bcff2f] shadow-[inset_0_1px_0_rgba(255,255,255,0.03)] transition hover:bg-[linear-gradient(180deg,#353f14_0%,#2a3210_100%)] disabled:cursor-not-allowed disabled:opacity-50"
                >
                    <img src={flashIcon} alt="" aria-hidden="true" className="h-3 w-3 shrink-0 object-contain" />
                    {isBlacklisted ? '黑名单' : '一键开仓'}
                </button>
            </div>
        </div >
    );
}
