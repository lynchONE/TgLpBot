import React, { useEffect, useMemo, useRef, useState } from 'react';
import { openLink } from '../lib/telegram';
import { useDurationFrom, useRelativeTime } from '../lib/time';
import PriceRangeVisualizer from './PriceRangeVisualizer';
import NumberFlowValue from './NumberFlowValue.jsx';
import uniswapIcon from '../image/uniswap.svg';
import pancakeIcon from '../image/pancake.svg';
import { TASK_MODE_OPTIONS, normalizeTaskMode } from '../lib/taskModes';
import {
    formatUsd,
    formatUsdCompact,
    formatFeeUsd,
    formatBotAmount,
    formatPrice,
    formatRangePercentPlain,
} from '../lib/format';

const Icon = ({ path, className = '' }) => (
    <svg viewBox="0 0 24 24" fill="currentColor" className={className} aria-hidden="true">
        <path d={path} />
    </svg>
);

const icons = {
    trend: 'M3 17l6-6 4 4 7-7v4h2V4h-8v2h4l-5 5-4-4-7 7z',
    wallet: 'M4 7a3 3 0 013-3h13v4H7a1 1 0 000 2h14v7a3 3 0 01-3 3H7a3 3 0 01-3-3V7zm16 6h-5v4h5v-4z',
    kebab: 'M12 7a2 2 0 110-4 2 2 0 010 4zm0 7a2 2 0 110-4 2 2 0 010 4zm0 7a2 2 0 110-4 2 2 0 010 4z',
    chevronDown: 'M6 9l6 6 6-6',
    arrowUp: 'M7 14l5-5 5 5',
    arrowDown: 'M7 10l5 5 5-5',
};

function getDexIconConfig(exchangeName, versionText) {
    const text = `${String(exchangeName || '').trim()} ${String(versionText || '').trim()}`.toLowerCase();
    const versionMatch = text.match(/v(\d+)/i);
    const label = versionMatch ? `V${versionMatch[1]}` : String(versionText || '').trim().toUpperCase();
    if (text.includes('uniswap')) {
        return {
            src: uniswapIcon,
            alt: 'Uniswap',
            label,
            bgClass: 'bg-pink-500/15',
            textClass: 'text-pink-600 dark:text-pink-300',
            ringClass: 'ring-pink-500/25 dark:ring-pink-500/30',
        };
    }
    if (text.includes('pancake') || text.includes('pcs')) {
        return {
            src: pancakeIcon,
            alt: 'PancakeSwap',
            label,
            bgClass: 'bg-amber-500/15',
            textClass: 'text-amber-700 dark:text-amber-300',
            ringClass: 'ring-amber-500/25 dark:ring-amber-500/30',
        };
    }
    return null;
}

const SMART_MONEY_RANGE_LIMIT = 4;

function normalizeSmartMoneyRangeGroups(groups) {
    return Array.isArray(groups)
        ? groups.filter((item) => Number(item?.range_percent) > 0)
        : [];
}

function SmartMoneyRangeSummary({ groups }) {
    const [expanded, setExpanded] = useState(false);
    const validGroups = useMemo(() => normalizeSmartMoneyRangeGroups(groups), [groups]);
    const visibleGroups = expanded ? validGroups : validGroups.slice(0, SMART_MONEY_RANGE_LIMIT);
    const hiddenCount = Math.max(0, validGroups.length - visibleGroups.length);
    if (!validGroups.length) return null;
    return (
        <div className="rounded-lg border border-lime-500/20 bg-lime-500/[0.08] px-2.5 py-2">
            <div className="flex items-center justify-between gap-2">
                <div className="text-[10px] font-bold tracking-wide text-lime-700 dark:text-lime-300">聪明钱区间</div>
                <div className="text-[10px] text-lime-700/70 dark:text-lime-300/70">{validGroups.length} 组</div>
            </div>
            <div className="mt-2 flex flex-wrap gap-1.5">
                {visibleGroups.map((group, index) => (
                    <div
                        key={`${Number(group?.range_percent || 0)}:${Number(group?.position_count || 0)}:${index}`}
                        className="inline-flex min-w-0 items-center gap-1.5 rounded-full border border-lime-500/15 bg-black/10 px-2 py-1 text-[10px] text-zinc-600 dark:text-zinc-200"
                    >
                        <span className="shrink-0 font-semibold text-zinc-900 dark:text-white/95">{formatRangePercentPlain(group?.range_percent)}</span>
                        {Math.max(0, Number(group?.position_count) || 0) > 1 ? (
                            <span className="shrink-0 rounded-full bg-white/70 px-1.5 py-0.5 text-[9px] font-semibold text-zinc-600 dark:bg-white/10 dark:text-zinc-300">
                                {Number(group.position_count)} 笔
                            </span>
                        ) : null}
                        <span className="truncate text-zinc-500 dark:text-white/55">{formatUsdCompact(group?.total_amount_usd)}</span>
                    </div>
                ))}
            </div>
            {hiddenCount > 0 ? (
                <button
                    type="button"
                    onClick={() => setExpanded((prev) => !prev)}
                    className="mt-2 text-[10px] font-semibold text-lime-700 transition hover:text-lime-800 dark:text-lime-300 dark:hover:text-lime-200"
                >
                    {expanded ? '收起' : `展开更多 +${hiddenCount}`}
                </button>
            ) : null}
        </div>
    );
}

const QUOTE_SYMBOLS = new Set(['USDT', 'USDC', 'BUSD', 'DAI', 'FRAX', 'USDD', 'FDUSD', 'WBNB', 'WETH', 'WSOL', 'BNB', 'ETH', 'SOL']);
const normalizeSymbol = (value) => String(value || '').trim().toUpperCase();
const isQuoteLikeSymbol = (symbol) => QUOTE_SYMBOLS.has(normalizeSymbol(symbol));

const priceFromTick = (tick, decimals0 = 18, decimals1 = 18) => {
    const n = Number(tick);
    if (!Number.isFinite(n)) return null;
    const dec0 = Number(decimals0);
    const dec1 = Number(decimals1);
    if (!Number.isFinite(dec0) || !Number.isFinite(dec1)) return null;
    const v = Math.pow(1.0001, n);
    if (!Number.isFinite(v)) return null;
    const scale = Math.pow(10, dec0 - dec1);
    const adjusted = v * scale;
    if (!Number.isFinite(adjusted)) return null;
    return adjusted;
};

const safeInvert = (value) => {
    if (!Number.isFinite(value) || value === 0) return null;
    const v = 1 / value;
    return Number.isFinite(v) ? v : null;
};

const getStatusTheme = (label) => {
    if (label?.includes('停止'))
        return { pill: 'bg-red-500/15 text-red-600 ring-red-500/25 dark:text-red-300 dark:ring-red-400/30', dot: 'bg-red-500', bar: 'bg-gradient-to-b from-red-500 to-red-600' };
    if (label?.includes('暂停') || label?.includes('停止中') || label?.includes('撤仓中') || label?.includes('处理中'))
        return { pill: 'bg-amber-500/15 text-amber-700 ring-amber-500/25 dark:text-amber-300 dark:ring-amber-400/30', dot: 'bg-amber-500', bar: 'bg-gradient-to-b from-amber-400 to-amber-500' };
    if (label?.includes('等待') || label?.includes('排队'))
        return { pill: 'bg-sky-500/15 text-sky-700 ring-sky-500/25 dark:text-sky-300 dark:ring-sky-400/30', dot: 'bg-sky-500', bar: 'bg-gradient-to-b from-sky-400 to-sky-500' };
    return { pill: 'bg-emerald-500/15 text-emerald-700 ring-emerald-500/25 dark:text-emerald-300 dark:ring-emerald-400/30', dot: 'bg-emerald-500', bar: 'bg-gradient-to-b from-emerald-400 to-emerald-500' };
};

const normalizeHexPrefixed = (v) => {
    const raw = String(v || '').trim();
    if (!raw) return '';
    if (raw.startsWith('0x') || raw.startsWith('0X')) return `0x${raw.slice(2)}`;
    return `0x${raw}`;
};

// Normalize legacy smart money range groups before rendering.
function SmartMoneyRangeSummaryClean({ groups }) {
    const [expanded, setExpanded] = useState(false);
    const validGroups = useMemo(() => normalizeSmartMoneyRangeGroups(groups), [groups]);
    const visibleGroups = expanded ? validGroups : validGroups.slice(0, SMART_MONEY_RANGE_LIMIT);
    const hiddenCount = Math.max(0, validGroups.length - visibleGroups.length);
    if (!validGroups.length) return null;
    return (
        <div className="rounded-lg border border-lime-500/20 bg-lime-500/[0.08] px-2.5 py-2">
            <div className="flex items-center justify-between gap-2">
                <div className="text-[10px] font-bold tracking-wide text-lime-700 dark:text-lime-300">聪明钱区间</div>
                <div className="text-[10px] text-lime-700/70 dark:text-lime-300/70">{validGroups.length} 组</div>
            </div>
            <div className="mt-2 flex flex-wrap gap-1.5">
                {visibleGroups.map((group, index) => (
                    <div
                        key={`${Number(group?.range_percent || 0)}:${Number(group?.position_count || 0)}:${index}`}
                        className="inline-flex min-w-0 items-center gap-1.5 rounded-full border border-lime-500/15 bg-black/10 px-2 py-1 text-[10px] text-zinc-600 dark:text-zinc-200"
                    >
                        <span className="shrink-0 font-semibold text-zinc-900 dark:text-white/95">{formatRangePercentPlain(group?.range_percent)}</span>
                        {Math.max(0, Number(group?.position_count) || 0) > 1 ? (
                            <span className="shrink-0 rounded-full bg-white/70 px-1.5 py-0.5 text-[9px] font-semibold text-zinc-600 dark:bg-white/10 dark:text-zinc-300">
                                {Number(group.position_count)} 笔
                            </span>
                        ) : null}
                        <span className="truncate text-zinc-500 dark:text-white/55">{formatUsdCompact(group?.total_amount_usd)}</span>
                    </div>
                ))}
            </div>
            {hiddenCount > 0 ? (
                <button
                    type="button"
                    onClick={() => setExpanded((prev) => !prev)}
                    className="mt-2 text-[10px] font-semibold text-lime-700 transition hover:text-lime-800 dark:text-lime-300 dark:hover:text-lime-200"
                >
                    {expanded ? '收起' : `展开更多 +${hiddenCount}`}
                </button>
            ) : null}
        </div>
    );
}

function getPositionStatusTheme(label) {
    if (label?.includes('已停止') || label?.includes('错误')) {
        return {
            pill: 'bg-red-500/15 text-red-600 ring-red-500/25 dark:text-red-300 dark:ring-red-400/30',
            dot: 'bg-red-500',
        };
    }
    if (
        label?.includes('已暂停')
        || label?.includes('停止中')
        || label?.includes('撤仓中')
        || label?.includes('止损中')
        || label?.includes('再平衡中')
        || label?.includes('撤仓结束中')
    ) {
        return {
            pill: 'bg-amber-500/15 text-amber-700 ring-amber-500/25 dark:text-amber-300 dark:ring-amber-400/30',
            dot: 'bg-amber-500',
        };
    }
    if (label?.includes('等待')) {
        return {
            pill: 'bg-sky-500/15 text-sky-700 ring-sky-500/25 dark:text-sky-300 dark:ring-sky-400/30',
            dot: 'bg-sky-500',
        };
    }
    return {
        pill: 'bg-emerald-500/15 text-emerald-700 ring-emerald-500/25 dark:text-emerald-300 dark:ring-emerald-400/30',
        dot: 'bg-emerald-500',
    };
}

function buildTaskRangeDisplay(position) {
    const low = Number(position?.task_range_lower_pct);
    const up = Number(position?.task_range_upper_pct);
    if (!Number.isFinite(low) || !Number.isFinite(up) || low <= 0 || up <= 0) return null;
    const asymmetric = Math.abs(low - up) >= 0.01;
    const avg = (low + up) / 2;
    const totalWidth = low + up;
    const summaryText = asymmetric ? `下 ${low.toFixed(2)}% / 上 ${up.toFixed(2)}%` : `±${avg.toFixed(2)}%`;
    let text = summaryText;
    const amountUsdt = Number(position?.task_amount_usdt);
    if (Number.isFinite(amountUsdt) && amountUsdt > 0) {
        text += ` · 金额 $${amountUsdt.toFixed(2)}`;
    }
    return { text, badgeText: `宽度 ${totalWidth.toFixed(2)}%` };
}

function isStoppedStatus(label) {
    return String(label || '').includes('已停止');
}

function isBusyStatus(label) {
    const text = String(label || '');
    return (
        text.includes('停止中')
        || text.includes('撤仓中')
        || text.includes('止损中')
        || text.includes('再平衡中')
        || text.includes('撤仓结束中')
    );
}

const FEE_TIER_BY_TICK_SPACING = {
    1: 100,
    10: 500,
    50: 2500,
    60: 3000,
    100: 5000,
    200: 10000,
    2000: 20000,
};

function inferFeeTierFromTickSpacing(tickSpacing) {
    return FEE_TIER_BY_TICK_SPACING[Number(tickSpacing)] || 0;
}

function formatFeeTierPercent(feeTier) {
    const n = Number(feeTier || 0);
    if (!Number.isFinite(n) || n <= 0) return null;
    return `${(n / 10000).toFixed(4)}%`;
}

function buildPositionPairTitle(position, token0, token1) {
    const left = String(token0?.symbol || '').trim();
    const right = String(token1?.symbol || '').trim();
    if (left && right) return `${left}-${right}`;
    const rawTitle = String(position?.title || '').trim();
    if (!rawTitle) return '--';
    const parts = rawTitle.split('-').map((item) => String(item || '').trim()).filter(Boolean);
    if (parts.length >= 3) return parts.slice(1, -1).join('-');
    return rawTitle;
}

export default function PositionCard({
    position,
    walletAddress,
    bnbBalance,
    pollIntervalSec,
    updatedAt,
    allowTaskActions = true,
    showAbsolutePnl = true,
    onSetTaskPaused,
    onStopTask,
    onPartialExit,
    onDeleteTask,
    onUpdateTaskRange,
    onWithdrawLiquidity,
    onSwapDust,
    onTriggerRebalance,
    onUpdateTaskMode,
    onAddLiquidity,
    batchMode = false,
    isSelected = false,
    onToggleSelect,
    headerAccessory = null,
    smartMoneyRangeGroups = [],
}) {
    const [expanded, setExpanded] = useState(true);

    const runningDuration = useDurationFrom(position?.running_since);
    const updateTimeText = useRelativeTime(updatedAt);

    const token0 = position?.token_rows?.[0];
    const token1 = position?.token_rows?.[1];

    const displayTokenSide = useMemo(() => {
        const quote0 = isQuoteLikeSymbol(token0?.symbol);
        const quote1 = isQuoteLikeSymbol(token1?.symbol);
        if (quote0 && !quote1) return 1;
        return 0;
    }, [token0?.symbol, token1?.symbol]);

    const displaySymbol = displayTokenSide === 0 ? token0?.symbol : token1?.symbol;
    const quoteSymbol = displayTokenSide === 0 ? token1?.symbol : token0?.symbol;
    const pairLabel = displaySymbol && quoteSymbol ? `${displaySymbol}/${quoteSymbol}` : displaySymbol || quoteSymbol || '';
    const shouldInvertPrice = displayTokenSide === 1;

    const displayTitle = useMemo(
        () => buildPositionPairTitle(position, token0, token1),
        [position, token0, token1]
    );
    const dexConfig = useMemo(
        () => getDexIconConfig(position?.exchange, position?.version),
        [position?.exchange, position?.version]
    );

    const { totalValue, pnlAbsolute, hasPnL } = useMemo(() => {
        const backendCurrentValue = Number(position?.current_value_usd);
        const backendAbsolutePnL = Number(position?.absolute_pnl_usd);
        const backendHasPnL = Boolean(position?.has_pnl)
            && Number.isFinite(backendCurrentValue)
            && Number.isFinite(backendAbsolutePnL);
        if (backendHasPnL) {
            return {
                totalValue: backendCurrentValue,
                pnlAbsolute: backendAbsolutePnL,
                hasPnL: true,
            };
        }

        const positionUsd = Number(position?.totals?.position_usd || 0);
        const feeUsd = Number(position?.totals?.fee_usd || 0);
        const total = (Number.isFinite(positionUsd) ? positionUsd : 0) + (Number.isFinite(feeUsd) ? feeUsd : 0);
        const netInvested = Number(position?.net_invested_usd);
        const initialCost = Number(position?.initial_cost_usd);
        const costBasis = Number.isFinite(netInvested) && netInvested > 0
            ? netInvested
            : (Number.isFinite(initialCost) && initialCost > 0 ? initialCost : 0);
        if (costBasis > 0) {
            const pnl = total - costBasis;
            return { totalValue: total, pnlAbsolute: pnl, hasPnL: true };
        }
        return { totalValue: total, pnlAbsolute: 0, hasPnL: false };
    }, [
        position?.has_pnl,
        position?.current_value_usd,
        position?.absolute_pnl_usd,
        position?.totals?.position_usd,
        position?.totals?.fee_usd,
        position?.initial_cost_usd,
        position?.net_invested_usd,
    ]);

    const chain = useMemo(() => String(position?.chain || '').trim().toLowerCase() || 'bsc', [position?.chain]);
    const gmgnNetwork = useMemo(() => (chain === 'base' ? 'base' : 'bsc'), [chain]);
    const gmgnBase = useMemo(() => `https://gmgn.ai/${gmgnNetwork}`, [gmgnNetwork]);
    const geckoNetwork = useMemo(() => (chain === 'base' ? 'base' : 'bsc'), [chain]);

    const poolLink = useMemo(() => {
        const pool = normalizeHexPrefixed(position?.pool_id);
        if (!pool) return null;
        if (/^0x[a-fA-F0-9]{40}$/.test(pool)) return `${gmgnBase}/address/${pool}`;
        if (/^0x[a-fA-F0-9]{64}$/.test(pool)) return `https://www.geckoterminal.com/${geckoNetwork}/pools/${pool.toLowerCase()}`;
        return null;
    }, [position?.pool_id, gmgnBase, geckoNetwork]);

    const openWallet = () => openLink(`${gmgnBase}/address/${walletAddress}`);
    const openPool = () => poolLink && openLink(poolLink);
    const openToken = (addr) => addr && openLink(`${gmgnBase}/token/${addr}`);

    const decimals0 = useMemo(() => Number(token0?.decimals ?? 18), [token0?.decimals]);
    const decimals1 = useMemo(() => Number(token1?.decimals ?? 18), [token1?.decimals]);

    const currentTick = Number(position?.current_tick);
    const tickLowerRaw = Number(position?.tick_lower);
    const tickUpperRaw = Number(position?.tick_upper);
    const tickSpacingRaw = Number(position?.pool?.tickSpacing ?? position?.tick_spacing);

    const currentPriceBase = useMemo(() => priceFromTick(currentTick, decimals0, decimals1), [currentTick, decimals0, decimals1]);
    const currentPrice = shouldInvertPrice ? safeInvert(currentPriceBase) : currentPriceBase;

    const currentGridIndex = useMemo(() => {
        if (!Number.isFinite(currentTick) || !Number.isFinite(tickLowerRaw) || !tickSpacingRaw) return null;
        return Math.floor((currentTick - tickLowerRaw) / tickSpacingRaw) + 1;
    }, [currentTick, tickLowerRaw, tickSpacingRaw]);

    const { gridLower, gridUpper } = useMemo(() => {
        if (currentGridIndex === null || !tickSpacingRaw || !Number.isFinite(tickLowerRaw)) return { gridLower: null, gridUpper: null };
        const t1 = tickLowerRaw + (currentGridIndex - 1) * tickSpacingRaw;
        const t2 = t1 + tickSpacingRaw;
        const p1Base = priceFromTick(t1, decimals0, decimals1);
        const p2Base = priceFromTick(t2, decimals0, decimals1);
        if (p1Base === null || p2Base === null) return { gridLower: null, gridUpper: null };
        const p1 = shouldInvertPrice ? safeInvert(p1Base) : p1Base;
        const p2 = shouldInvertPrice ? safeInvert(p2Base) : p2Base;
        if (p1 === null || p2 === null) return { gridLower: null, gridUpper: null };
        return { gridLower: Math.min(p1, p2), gridUpper: Math.max(p1, p2) };
    }, [currentGridIndex, tickLowerRaw, tickSpacingRaw, decimals0, decimals1, shouldInvertPrice]);

    const gridCountRaw = useMemo(() => {
        if (!Number.isFinite(tickLowerRaw) || !Number.isFinite(tickUpperRaw)) return null;
        const diff = Math.abs(tickUpperRaw - tickLowerRaw);
        if (tickSpacingRaw && tickSpacingRaw > 0) return Math.round(diff / tickSpacingRaw);
        return null;
    }, [tickLowerRaw, tickUpperRaw, tickSpacingRaw]);

    const gridStepPct = useMemo(() => {
        if (!Number.isFinite(tickSpacingRaw) || tickSpacingRaw <= 0) return null;
        const pct = (Math.pow(1.0001, tickSpacingRaw) - 1) * 100;
        return Number.isFinite(pct) && pct > 0 ? pct : null;
    }, [tickSpacingRaw]);

    const rangeLowerBase = useMemo(() => priceFromTick(position?.tick_lower, decimals0, decimals1), [position?.tick_lower, decimals0, decimals1]);
    const rangeUpperBase = useMemo(() => priceFromTick(position?.tick_upper, decimals0, decimals1), [position?.tick_upper, decimals0, decimals1]);
    const rangeLower = shouldInvertPrice ? safeInvert(rangeLowerBase) : rangeLowerBase;
    const rangeUpper = shouldInvertPrice ? safeInvert(rangeUpperBase) : rangeUpperBase;
    const rangeReady = Number.isFinite(rangeLower) && Number.isFinite(rangeUpper);
    const rangeMin = rangeReady ? Math.min(rangeLower, rangeUpper) : null;
    const rangeMax = rangeReady ? Math.max(rangeLower, rangeUpper) : null;

    const taskRange = useMemo(() => {
        const low = Number(position?.task_range_lower_pct);
        const up = Number(position?.task_range_upper_pct);
        if (!Number.isFinite(low) || !Number.isFinite(up) || low <= 0 || up <= 0) return null;
        const asymmetric = Math.abs(low - up) >= 0.01;
        const avg = (low + up) / 2;
        const totalWidth = low + up;
        const summaryText = asymmetric ? `下 ${low.toFixed(2)}% / 上 ${up.toFixed(2)}%` : `±${avg.toFixed(2)}%`;
        let text = `${summaryText} · 总宽度 ${totalWidth.toFixed(2)}%`;
        const amountUsdt = Number(position?.task_amount_usdt);
        if (Number.isFinite(amountUsdt) && amountUsdt > 0) {
            text += ` | $${amountUsdt.toFixed(2)}`;
        }
        return { text, badgeText: `宽度 ${totalWidth.toFixed(2)}%` };
    }, [position?.task_range_lower_pct, position?.task_range_upper_pct, position?.task_amount_usdt]);

    const displayTaskRange = useMemo(() => buildTaskRangeDisplay(position), [
        position?.task_range_lower_pct,
        position?.task_range_upper_pct,
        position?.task_amount_usdt,
    ]);

    const taskId = useMemo(() => {
        const raw = Number(position?.task_id);
        return Number.isFinite(raw) && raw > 0 ? raw : 0;
    }, [position?.task_id]);

    const taskPaused = Boolean(position?.task_paused);
    const currentTaskMode = normalizeTaskMode(position?.task_mode, position?.task_paused);
    const statusLabel = String(position?.status_label || '');
    const isStopped = statusLabel.includes('停止') || statusLabel.includes('结束');
    const isStopping = statusLabel.includes('停止中') || statusLabel.includes('撤仓中') || statusLabel.includes('处理中');
    const isStoppedState = isStoppedStatus(statusLabel);
    const isBusyState = isBusyStatus(statusLabel);
    const hasActions = typeof onSetTaskPaused === 'function' || typeof onStopTask === 'function' || typeof onPartialExit === 'function' || typeof onDeleteTask === 'function' || typeof onUpdateTaskRange === 'function';
    const canTaskAction = Boolean(allowTaskActions) && hasActions && taskId > 0;
    const canPauseAction = canTaskAction && typeof onSetTaskPaused === 'function' && !isBusyState;
    const canUpdateRangeAction = canTaskAction && typeof onUpdateTaskRange === 'function' && !isBusyState;
    const canStopAction = canTaskAction && typeof onStopTask === 'function' && !isStoppedState && !isBusyState;
    const canDeleteAction = canTaskAction && typeof onDeleteTask === 'function' && !isBusyState;
    const hasLiquidity = Boolean(position?.has_liquidity);
    const canPartialExit = canTaskAction && typeof onPartialExit === 'function' && hasLiquidity && !isBusyState;
    const canWithdraw = canTaskAction && typeof onWithdrawLiquidity === 'function' && hasLiquidity && !isBusyState;
    const canSwapDust = canTaskAction && typeof onSwapDust === 'function' && !isBusyState;
    const canTriggerRebalance = canTaskAction && typeof onTriggerRebalance === 'function' && hasLiquidity && !isStoppedState && !isBusyState;
    const canUpdateTaskMode = canTaskAction && typeof onUpdateTaskMode === 'function' && !isStoppedState && !isBusyState;
    const canAddLiquidity = canTaskAction && typeof onAddLiquidity === 'function' && !isStoppedState && !isBusyState;

    const [menuOpen, setMenuOpen] = useState(false);
    const [actionPending, setActionPending] = useState('');
    const [exitPanelOpen, setExitPanelOpen] = useState(false);
    const [exitPercent, setExitPercent] = useState('25');
    const menuRef = useRef(null);

    useEffect(() => {
        if (!menuOpen) return;
        const onPointerDown = (e) => {
            if (!menuRef.current || menuRef.current.contains(e.target)) return;
            setMenuOpen(false);
        };
        document.addEventListener('mousedown', onPointerDown);
        document.addEventListener('touchstart', onPointerDown);
        return () => {
            document.removeEventListener('mousedown', onPointerDown);
            document.removeEventListener('touchstart', onPointerDown);
        };
    }, [menuOpen]);

    const runAction = async (action, handler) => {
        if (!handler || actionPending) return;
        setActionPending(action);
        try { await handler(); } finally { setActionPending(''); setMenuOpen(false); }
    };

    const togglePause = () => runAction('pause', () => onSetTaskPaused?.(taskId, !taskPaused));
    const editRange = () => runAction('range', () => onUpdateTaskRange?.(taskId, position));
    const stopTask = () => runAction('stop', () => onStopTask?.(taskId));
    const deleteTask = () => runAction('delete', () => onDeleteTask?.(taskId));
    const openWithdrawPanel = () => {
        if (actionPending) return;
        setExitPanelOpen(true);
        setMenuOpen(false);
    };
    const withdrawLiquidity = (pct = exitPercent) => {
        const value = Number(pct);
        if (!Number.isFinite(value) || value <= 0 || value > 100) return;
        runAction('withdraw', () => onPartialExit?.(taskId, value));
    };
    const withdrawAllLiquidity = () => runAction('withdrawAll', () => onWithdrawLiquidity?.(taskId));
    const swapDust = () => runAction('dust', () => onSwapDust?.(taskId));
    const triggerRebalance = () => runAction('rebalance', () => onTriggerRebalance?.(taskId));
    const updateTaskMode = (nextMode) => runAction('taskMode', () => onUpdateTaskMode?.(taskId, nextMode));
    const addLiquidity = () => runAction('addLiq', () => onAddLiquidity?.(taskId, position));

    const pnlPositive = pnlAbsolute >= 0;
    const statusTheme = getPositionStatusTheme(statusLabel);
    const parsedExitPercent = Number(exitPercent);
    const canSubmitExitPercent = Number.isFinite(parsedExitPercent) && parsedExitPercent > 0 && parsedExitPercent <= 100;

    // Prefer backend fee tier, then infer from tick spacing.
    const feeLabel = useMemo(() => {
        const feeTier = Number(position?.fee_tier || 0) || inferFeeTierFromTickSpacing(position?.pool?.tickSpacing ?? position?.tick_spacing);
        return formatFeeTierPercent(feeTier);
    }, [position?.fee_tier, position?.pool?.tickSpacing, position?.tick_spacing]);

    return (
        <div className="relative rounded-2xl border border-zinc-200/80 bg-white dark:border-white/5 dark:bg-[#14171c] shadow-sm overflow-hidden transition-all duration-200 active:scale-[0.985]">
            <div className="px-3 pt-3 pb-2 flex flex-col gap-2">
                <div className="flex items-start justify-between gap-2">
                    <div className="flex items-start gap-2 min-w-0 flex-1">
                        {batchMode && (
                            <button type="button" onClick={onToggleSelect}
                                className={`mt-0.5 flex h-4 w-4 shrink-0 items-center justify-center rounded border-2 transition-all active:scale-90 ${isSelected ? 'border-emerald-500 bg-emerald-500 text-white shadow-sm shadow-emerald-500/30' : 'border-zinc-300 bg-white dark:border-zinc-600 dark:bg-zinc-800'}`}>
                                {isSelected && (
                                    <svg className="h-3 w-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={3}>
                                        <path strokeLinecap="round" strokeLinejoin="round" d="M5 13l4 4L19 7" />
                                    </svg>
                                )}
                            </button>
                        )}
                        <div className="flex flex-col gap-1 min-w-0 flex-1">
                            <div className="flex items-center gap-1.5 flex-wrap pr-1">
                                {dexConfig ? (
                                    <span className={`inline-flex shrink-0 items-center gap-1 rounded-lg px-1.5 py-0.5 text-[10px] font-bold ring-1 ${dexConfig.bgClass} ${dexConfig.textClass} ${dexConfig.ringClass}`}>
                                        <img src={dexConfig.src} alt={dexConfig.alt} className="h-3.5 w-3.5" />
                                        {dexConfig.label ? <span>{dexConfig.label}</span> : null}
                                    </span>
                                ) : null}
                                <span className="text-[15px] font-bold text-zinc-900 dark:text-white/95 leading-tight truncate max-w-full">
                                    {displayTitle}
                                </span>
                                {feeLabel && (
                                    <span className="inline-flex items-center rounded bg-violet-500/12 px-1.5 py-0.5 text-[10px] font-bold text-violet-600 dark:bg-violet-500/18 dark:text-violet-300 ring-1 ring-violet-500/20 dark:ring-violet-400/25 shrink-0">
                                        {feeLabel}
                                    </span>
                                )}
                            </div>
                            <div className="flex flex-wrap items-center gap-1.5 pr-1">
                                <span className={`inline-flex items-center gap-1 rounded-full px-1.5 py-0.5 text-[10px] font-semibold ring-1 shrink-0 ${statusTheme.pill}`}>
                                    <span className={`h-1 w-1 rounded-full shrink-0 ${statusTheme.dot}`} />
                                    <span className="truncate max-w-[70px]">{statusLabel || '状态未知'}</span>
                                </span>
                                {taskId > 0 && (
                                    <span className="text-[10px] font-medium text-zinc-400 dark:text-white/30 shrink-0">
                                        #<NumberFlowValue value={taskId} formatOptions={{ maximumFractionDigits: 0 }} />
                                    </span>
                                )}
                                {updateTimeText ? (
                                    <span className="inline-flex items-center rounded-full bg-zinc-100 px-1.5 py-0.5 text-[10px] font-semibold text-zinc-600 ring-1 ring-zinc-200 dark:bg-white/10 dark:text-white/70 dark:ring-white/15">
                                        更新 <NumberFlowValue value={updateTimeText} formatter={() => updateTimeText} />
                                    </span>
                                ) : null}
                            </div>
                        </div>
                    </div>

                    <div className="ml-auto flex shrink-0 items-start gap-2 pl-2">
                        {false && <>
                        <div className="text-right">
                            <div className="mb-0.5 text-[9px] font-medium uppercase tracking-wide text-zinc-400 dark:text-white/35">仓值</div>
                            <div className="text-lg font-extrabold text-zinc-900 dark:text-white/95 tabular-nums leading-none">
                                <NumberFlowValue value={totalValue} formatter={(v) => formatUsd(v)} />
                            </div>
                            {showAbsolutePnl && hasPnL && (
                                <div className="mt-1.5">
                                    <div className={`inline-flex items-center rounded-full px-1.5 py-0.5 text-[11px] font-bold tabular-nums shadow-sm ${pnlPositive
                                        ? 'bg-emerald-500/15 text-emerald-500 dark:bg-emerald-500/20 dark:text-emerald-400 ring-1 ring-emerald-500/20'
                                        : 'bg-red-500/15 text-red-500 dark:bg-red-500/20 dark:text-red-400 ring-1 ring-red-500/20'
                                        }`}>
                                        <NumberFlowValue
                                            value={pnlAbsolute}
                                            formatter={(v) => `${Number(v) >= 0 ? '+' : ''}${formatBotAmount(v)}`}
                                        />
                                    </div>
                                </div>
                            )}
                        </div>
                        <div className="text-right">
                            <div className="mb-0.5 text-[9px] font-medium uppercase tracking-wide text-zinc-400 dark:text-white/35">价值</div>
                            <div className="text-lg font-extrabold text-zinc-900 dark:text-white/95 tabular-nums leading-none">
                                <NumberFlowValue value={totalValue} formatter={(v) => formatUsd(v)} />
                            </div>
                            {showAbsolutePnl && hasPnL && (
                                <div className="mt-1.5">
                                    <div className={`inline-flex items-center rounded-full px-1.5 py-0.5 text-[11px] font-bold tabular-nums shadow-sm ${pnlPositive
                                        ? 'bg-emerald-500/15 text-emerald-500 dark:bg-emerald-500/20 dark:text-emerald-400 ring-1 ring-emerald-500/20'
                                        : 'bg-red-500/15 text-red-500 dark:bg-red-500/20 dark:text-red-400 ring-1 ring-red-500/20'
                                        }`}>
                                        <NumberFlowValue
                                            value={pnlAbsolute}
                                            formatter={(v) => `${Number(v) >= 0 ? '+' : ''}${formatBotAmount(v)}`}
                                        />
                                    </div>
                                </div>
                            )}
                        </div>
                        </>}

                        <div className="text-right">
                            <div className="mb-0.5 text-[9px] font-medium uppercase tracking-wide text-zinc-400 dark:text-white/35">仓值</div>
                            <div className="text-lg font-extrabold text-zinc-900 dark:text-white/95 tabular-nums leading-none">
                                <NumberFlowValue value={totalValue} formatter={(v) => formatUsd(v)} />
                            </div>
                            {showAbsolutePnl && hasPnL && (
                                <div className="mt-1.5">
                                    <div className={`inline-flex items-center rounded-full px-1.5 py-0.5 text-[11px] font-bold tabular-nums shadow-sm ${pnlPositive
                                        ? 'bg-emerald-500/15 text-emerald-500 dark:bg-emerald-500/20 dark:text-emerald-400 ring-1 ring-emerald-500/20'
                                        : 'bg-red-500/15 text-red-500 dark:bg-red-500/20 dark:text-red-400 ring-1 ring-red-500/20'
                                        }`}>
                                        <NumberFlowValue
                                            value={pnlAbsolute}
                                            formatter={(v) => `${Number(v) >= 0 ? '+' : ''}${formatBotAmount(v)}`}
                                        />
                                    </div>
                                </div>
                            )}
                        </div>

                        {headerAccessory}

                        {canTaskAction && (
                            <div className="relative z-20" ref={menuRef}>
                                <button
                                    type="button"
                                    onClick={() => setMenuOpen((v) => !v)}
                                    className="inline-flex h-7 w-7 items-center justify-center rounded-lg border border-zinc-200/80 bg-zinc-50 text-zinc-500 hover:bg-zinc-100 hover:text-zinc-700 active:scale-95 transition-all dark:border-white/5 dark:bg-[#1c2026] dark:text-white/50 dark:hover:bg-white/5 dark:hover:text-white/80"
                                    aria-label="更多操作"
                                    disabled={Boolean(actionPending)}
                                >
                                    <Icon path={icons.kebab} className="h-4 w-4" />
                                </button>
                                {menuOpen && (
                                    <div className="absolute right-0 top-full z-30 mt-1.5 w-36 overflow-hidden rounded-xl border border-zinc-200/80 bg-white/95 backdrop-blur-xl shadow-xl dark:border-white/10 dark:bg-[#1c2026]/95">
                                        {typeof onSetTaskPaused === 'function' && (
                                            <button type="button" onClick={togglePause} disabled={!canPauseAction || Boolean(actionPending)}
                                                className="w-full px-3 py-2 text-left text-xs font-semibold text-zinc-700 hover:bg-zinc-100/80 disabled:opacity-40 transition-colors dark:text-white/70 dark:hover:bg-white/5">
                                                {actionPending === 'pause' ? '处理中...' : taskPaused ? '恢复任务' : '暂停任务'}
                                            </button>
                                        )}
                                        {typeof onUpdateTaskRange === 'function' && (
                                            <button type="button" onClick={editRange} disabled={!canUpdateRangeAction || Boolean(actionPending)}
                                                className="w-full border-t border-zinc-100/80 px-3 py-2 text-left text-xs font-semibold text-zinc-700 hover:bg-zinc-100/80 disabled:opacity-40 transition-colors dark:border-white/5 dark:text-white/70 dark:hover:bg-white/5">
                                                {actionPending === 'range' ? '处理中...' : '修改区间'}
                                            </button>
                                        )}
                                        {typeof onAddLiquidity === 'function' && (
                                            <button type="button" onClick={addLiquidity} disabled={!canAddLiquidity || Boolean(actionPending)}
                                                className="w-full border-t border-zinc-100/80 px-3 py-2 text-left text-xs font-semibold text-zinc-700 hover:bg-zinc-100/80 disabled:opacity-40 transition-colors dark:border-white/5 dark:text-white/70 dark:hover:bg-white/5">
                                                {actionPending === 'addLiq' ? '处理中...' : '补充流动性'}
                                            </button>
                                        )}
                                        {typeof onSwapDust === 'function' && (
                                            <button type="button" onClick={swapDust} disabled={!canSwapDust || Boolean(actionPending)}
                                                className="w-full border-t border-zinc-100/80 px-3 py-2 text-left text-xs font-semibold text-violet-600 hover:bg-violet-50 disabled:opacity-40 transition-colors dark:border-white/5 dark:text-violet-400 dark:hover:bg-violet-500/10">
                                                {actionPending === 'dust' ? '处理中...' : '碎币兑换'}
                                            </button>
                                        )}
                                        {typeof onTriggerRebalance === 'function' && (
                                            <button type="button" onClick={triggerRebalance} disabled={!canTriggerRebalance || Boolean(actionPending)}
                                                className="w-full border-t border-zinc-100/80 px-3 py-2 text-left text-xs font-semibold text-blue-600 hover:bg-blue-50 disabled:opacity-40 transition-colors dark:border-white/5 dark:text-blue-400 dark:hover:bg-blue-500/10">
                                                {actionPending === 'rebalance' ? '处理中...' : '立即再平衡'}
                                            </button>
                                        )}
                                        {typeof onPartialExit === 'function' && (
                                            <button type="button" onClick={openWithdrawPanel} disabled={!canPartialExit || Boolean(actionPending)}
                                                className="w-full border-t border-zinc-100/80 px-3 py-2 text-left text-xs font-semibold text-sky-600 hover:bg-sky-50 disabled:opacity-40 transition-colors dark:border-white/5 dark:text-sky-400 dark:hover:bg-sky-500/10">
                                                {actionPending === 'withdraw' ? '处理中...' : '部分撤仓'}
                                            </button>
                                        )}
                                        {typeof onWithdrawLiquidity === 'function' && (
                                            <button type="button" onClick={withdrawAllLiquidity} disabled={!canWithdraw || Boolean(actionPending)}
                                                className="w-full border-t border-zinc-100/80 px-3 py-2 text-left text-xs font-semibold text-cyan-600 hover:bg-cyan-50 disabled:opacity-40 transition-colors dark:border-white/5 dark:text-cyan-400 dark:hover:bg-cyan-500/10">
                                                {actionPending === 'withdrawAll' ? '处理中...' : '取回流动性'}
                                            </button>
                                        )}
                                        {typeof onStopTask === 'function' && (
                                            <button type="button" onClick={stopTask} disabled={!canStopAction || Boolean(actionPending)}
                                                className="w-full border-t border-zinc-100/80 px-3 py-2 text-left text-xs font-semibold text-amber-600 hover:bg-amber-50 disabled:opacity-40 transition-colors dark:border-white/5 dark:text-amber-400 dark:hover:bg-amber-500/10">
                                                {actionPending === 'stop' ? '处理中...' : isBusyState ? '处理中...' : '停止任务'}
                                            </button>
                                        )}
                                        {typeof onDeleteTask === 'function' && (
                                            <button type="button" onClick={deleteTask} disabled={!canDeleteAction || Boolean(actionPending)}
                                                className="w-full border-t border-zinc-100/80 px-3 py-2 text-left text-xs font-semibold text-red-600 hover:bg-red-50 disabled:opacity-40 transition-colors dark:border-white/5 dark:text-red-400 dark:hover:bg-red-500/10">
                                                {actionPending === 'delete' ? '删除中...' : '删除任务'}
                                            </button>
                                        )}
                                    </div>
                                )}
                            </div>
                        )}

                        {false && canTaskAction && (
                            <div className="relative z-20" ref={menuRef}>
                                <button
                                    type="button"
                                    onClick={() => setMenuOpen((v) => !v)}
                                    className="inline-flex h-7 w-7 items-center justify-center rounded-lg border border-zinc-200/80 bg-zinc-50 text-zinc-500 hover:bg-zinc-100 hover:text-zinc-700 active:scale-95 transition-all dark:border-white/5 dark:bg-[#1c2026] dark:text-white/50 dark:hover:bg-white/5 dark:hover:text-white/80"
                                    aria-label="任务操作"
                                    disabled={Boolean(actionPending)}
                                >
                                    <Icon path={icons.kebab} className="h-4 w-4" />
                                </button>
                                {menuOpen && (
                                    <div className="absolute right-0 top-full z-30 mt-1.5 w-32 overflow-hidden rounded-xl border border-zinc-200/80 bg-white/95 backdrop-blur-xl shadow-xl dark:border-white/10 dark:bg-[#1c2026]/95">
                                        {typeof onSetTaskPaused === 'function' && (
                                            <button type="button" onClick={togglePause} disabled={!canPauseAction || Boolean(actionPending)}
                                                className="w-full px-3 py-2 text-left text-xs font-semibold text-zinc-700 hover:bg-zinc-100/80 disabled:opacity-40 transition-colors dark:text-white/70 dark:hover:bg-white/5">
                                                {actionPending === 'pause' ? '处理中...' : taskPaused ? '恢复任务' : '暂停任务'}
                                            </button>
                                        )}
                                        {typeof onUpdateTaskRange === 'function' && (
                                            <button type="button" onClick={editRange} disabled={!canUpdateRangeAction || Boolean(actionPending)}
                                                className="w-full border-t border-zinc-100/80 px-3 py-2 text-left text-xs font-semibold text-zinc-700 hover:bg-zinc-100/80 disabled:opacity-40 transition-colors dark:border-white/5 dark:text-white/70 dark:hover:bg-white/5">
                                                {actionPending === 'range' ? '处理中...' : '修改区间'}
                                            </button>
                                        )}
                                        {typeof onStopTask === 'function' && (
                                            <button type="button" onClick={stopTask} disabled={!canStopAction || Boolean(actionPending)}
                                                className="w-full border-t border-zinc-100/80 px-3 py-2 text-left text-xs font-semibold text-amber-600 hover:bg-amber-50 disabled:opacity-40 transition-colors dark:border-white/5 dark:text-amber-400 dark:hover:bg-amber-500/10">
                                                {actionPending === 'stop' ? '处理中...' : isStopping ? '停止中...' : '停止任务'}
                                            </button>
                                        )}
                                        {typeof onAddLiquidity === 'function' && (
                                            <button type="button" onClick={addLiquidity} disabled={!canAddLiquidity || Boolean(actionPending)}
                                                className="w-full border-t border-zinc-100/80 px-3 py-2 text-left text-xs font-semibold text-zinc-700 hover:bg-zinc-100/80 disabled:opacity-40 transition-colors dark:border-white/5 dark:text-white/70 dark:hover:bg-white/5">
                                                {actionPending === 'addLiq' ? '处理中...' : '补充流动性'}
                                            </button>
                                        )}
                                        {typeof onDeleteTask === 'function' && (
                                            <button type="button" onClick={deleteTask} disabled={!canDeleteAction || Boolean(actionPending)}
                                                className="w-full border-t border-zinc-100/80 px-3 py-2 text-left text-xs font-semibold text-red-600 hover:bg-red-50 disabled:opacity-40 transition-colors dark:border-white/5 dark:text-red-400 dark:hover:bg-red-500/10">
                                                {actionPending === 'delete' ? '删除中...' : '删除任务'}
                                            </button>
                                        )}
                                    </div>
                                )}
                            </div>
                        )}
                    </div>
                </div>

                {canTaskAction && (
                    <div className="flex flex-wrap items-center gap-1.5 pb-0.5">
                        {exitPanelOpen && (
                            <div className="w-full rounded-xl border border-sky-400/25 bg-sky-50/80 p-2 dark:border-sky-500/20 dark:bg-sky-500/10">
                                <div className="mb-2 flex items-center justify-between gap-2">
                                    <span className="text-[11px] font-bold text-sky-700 dark:text-sky-300">撤出比例</span>
                                    <button type="button" onClick={() => setExitPanelOpen(false)} className="text-[11px] font-semibold text-zinc-500 dark:text-white/50">关闭</button>
                                </div>
                                <div className="mb-2 grid grid-cols-4 gap-1.5">
                                    {[25, 50, 75, 100].map((pctOption) => (
                                        <button
                                            key={pctOption}
                                            type="button"
                                            onClick={() => setExitPercent(String(pctOption))}
                                            disabled={Boolean(actionPending)}
                                            className={`h-8 rounded-lg border text-[11px] font-bold transition-all active:scale-95 disabled:opacity-40 ${Number(exitPercent) === pctOption
                                                ? 'border-sky-400 bg-sky-500 text-white shadow-sm shadow-sky-500/25'
                                                : 'border-sky-200 bg-white text-sky-700 dark:border-sky-500/20 dark:bg-white/5 dark:text-sky-300'
                                                }`}
                                        >
                                            {pctOption}%
                                        </button>
                                    ))}
                                </div>
                                <div className="flex items-center gap-2">
                                    <input
                                        type="number"
                                        min="0.01"
                                        max="100"
                                        step="0.01"
                                        value={exitPercent}
                                        onChange={(e) => setExitPercent(e.target.value)}
                                        className="h-9 min-w-0 flex-1 rounded-lg border border-sky-200 bg-white px-2 text-sm font-semibold text-zinc-900 outline-none focus:border-sky-400 dark:border-sky-500/20 dark:bg-black/20 dark:text-white"
                                    />
                                    <button
                                        type="button"
                                        onClick={() => withdrawLiquidity(parsedExitPercent)}
                                        disabled={!canPartialExit || Boolean(actionPending) || !canSubmitExitPercent}
                                        className="h-9 rounded-lg bg-sky-500 px-3 text-xs font-bold text-white shadow-sm shadow-sky-500/25 active:scale-95 disabled:opacity-40"
                                    >
                                        {actionPending === 'withdraw' ? '...' : parsedExitPercent >= 100 ? '全撤' : '撤仓'}
                                    </button>
                                </div>
                            </div>
                        )}
                        {typeof onPartialExit === 'function' && (
                            <button type="button" onClick={openWithdrawPanel} disabled={!canPartialExit || Boolean(actionPending)}
                                title="撤出流动性"
                                className="inline-flex h-7 shrink-0 items-center gap-1 rounded-xl border border-sky-400/40 bg-sky-50 px-2.5 text-[10.5px] font-semibold text-sky-700 shadow-sm transition-all active:scale-95 disabled:opacity-40 hover:bg-sky-100 dark:bg-sky-500/15 dark:text-sky-400 dark:border-sky-500/25 dark:hover:bg-sky-500/25">
                                <svg viewBox="0 0 24 24" fill="currentColor" className="h-3.5 w-3.5 shrink-0" aria-hidden="true">
                                    <path d="M19 9h-4V3H9v6H5l7 7 7-7zM5 18v2h14v-2H5z" />
                                </svg>
                                <span>{actionPending === 'withdraw' ? '...' : '撤仓'}</span>
                            </button>
                        )}
                        {typeof onWithdrawLiquidity === 'function' && (
                            <button type="button" onClick={withdrawAllLiquidity} disabled={!canWithdraw || Boolean(actionPending)}
                                title="取回流动性"
                                className="inline-flex h-7 shrink-0 items-center gap-1 rounded-xl border border-cyan-400/40 bg-cyan-50 px-2.5 text-[10.5px] font-semibold text-cyan-700 shadow-sm transition-all active:scale-95 disabled:opacity-40 hover:bg-cyan-100 dark:bg-cyan-500/15 dark:text-cyan-400 dark:border-cyan-500/25 dark:hover:bg-cyan-500/25">
                                <svg viewBox="0 0 24 24" fill="currentColor" className="h-3.5 w-3.5 shrink-0" aria-hidden="true">
                                    <path d="M5 20h14v-2H5v2zM19 9h-4V3H9v6H5l7 7 7-7z" />
                                </svg>
                                <span>{actionPending === 'withdrawAll' ? '...' : '取回'}</span>
                            </button>
                        )}
                        {typeof onUpdateTaskMode === 'function' && (
                            <>
                                {TASK_MODE_OPTIONS.map((option) => (
                                    <button key={option.value} type="button" onClick={() => updateTaskMode(option.value)} disabled={!canUpdateTaskMode || Boolean(actionPending)}
                                        title={option.description}
                                        className={`inline-flex h-7 shrink-0 items-center rounded-xl border px-2.5 text-[10.5px] font-semibold shadow-sm transition-all active:scale-95 disabled:opacity-40 ${currentTaskMode === option.value
                                            ? 'border-emerald-400/40 bg-emerald-50 text-emerald-700 hover:bg-emerald-100 dark:bg-emerald-500/15 dark:text-emerald-400 dark:border-emerald-500/25 dark:hover:bg-emerald-500/25'
                                            : 'border-zinc-300/60 bg-zinc-100 text-zinc-500 hover:bg-zinc-200 dark:bg-white/5 dark:text-zinc-400 dark:border-white/10 dark:hover:bg-white/10'
                                            }`}>
                                        <span>{option.shortLabel}</span>
                                    </button>
                                ))}
                            </>
                        )}
                    </div>
                )}

                {false && canTaskAction && (
                    <div className="flex items-center gap-1.5 overflow-x-auto pb-0.5 [scrollbar-width:none] [-ms-overflow-style:none]">
                        {typeof onPartialExit === 'function' && (
                            <button type="button" onClick={openWithdrawPanel} disabled={!canPartialExit || Boolean(actionPending)}
                                title="撤出流动性"
                                className="inline-flex h-7 shrink-0 items-center gap-1 rounded-xl border border-sky-400/40 bg-sky-50 px-2.5 text-[10.5px] font-semibold text-sky-700 shadow-sm transition-all active:scale-95 disabled:opacity-40 hover:bg-sky-100 dark:bg-sky-500/15 dark:text-sky-400 dark:border-sky-500/25 dark:hover:bg-sky-500/25">
                                <svg viewBox="0 0 24 24" fill="currentColor" className="h-3.5 w-3.5 shrink-0" aria-hidden="true">
                                    <path d="M19 9h-4V3H9v6H5l7 7 7-7zM5 18v2h14v-2H5z" />
                                </svg>
                                <span>{actionPending === 'withdraw' ? '...' : '撤仓'}</span>
                            </button>
                        )}
                        {typeof onSwapDust === 'function' && (
                            <button type="button" onClick={swapDust} disabled={!canSwapDust || Boolean(actionPending)}
                                title="兑换碎币"
                                className="inline-flex h-7 shrink-0 items-center gap-1 rounded-xl border border-violet-400/40 bg-violet-50 px-2.5 text-[10.5px] font-semibold text-violet-700 shadow-sm transition-all active:scale-95 disabled:opacity-40 hover:bg-violet-100 dark:bg-violet-500/15 dark:text-violet-400 dark:border-violet-500/25 dark:hover:bg-violet-500/25">
                                <svg viewBox="0 0 24 24" fill="currentColor" className="h-3.5 w-3.5 shrink-0" aria-hidden="true">
                                    <path d="M7.5 21H2V9h5.5v12zm7.25-18h-5.5v18h5.5V3zM22 11h-5.5v10H22V11z" />
                                </svg>
                                <span>{actionPending === 'dust' ? '...' : '碎币兑换'}</span>
                            </button>
                        )}
                        {typeof onTriggerRebalance === 'function' && (
                            <button type="button" onClick={triggerRebalance} disabled={!canTriggerRebalance || Boolean(actionPending)}
                                title="立即触发再平衡"
                                className="inline-flex h-7 shrink-0 items-center gap-1 rounded-xl border border-blue-400/40 bg-blue-50 px-2.5 text-[10.5px] font-semibold text-blue-700 shadow-sm transition-all active:scale-95 disabled:opacity-40 hover:bg-blue-100 dark:bg-blue-500/15 dark:text-blue-400 dark:border-blue-500/25 dark:hover:bg-blue-500/25">
                                <svg viewBox="0 0 24 24" fill="currentColor" className="h-3.5 w-3.5 shrink-0" aria-hidden="true">
                                    <path d="M12 6V1.5l-4.5 4.5L12 10.5V6c3.31 0 6 2.69 6 6 0 1.01-.25 1.97-.7 2.8l1.46 1.46C19.54 15.03 20 13.57 20 12c0-4.42-3.58-8-8-8zm0 14c-3.31 0-6-2.69-6-6 0-1.01.25-1.97.7-2.8L5.24 9.74C4.46 10.97 4 12.43 4 14c0 4.42 3.58 8 8 8v4.5l4.5-4.5L12 17.5V20z" />
                                </svg>
                                <span>{actionPending === 'rebalance' ? '...' : '再平衡'}</span>
                            </button>
                        )}
                                                {typeof onUpdateTaskMode === 'function' && (
                            <>
                                {TASK_MODE_OPTIONS.map((option) => (
                                    <button key={option.value} type="button" onClick={() => updateTaskMode(option.value)} disabled={!canUpdateTaskMode || Boolean(actionPending)}
                                        title={option.description}
                                        className={`inline-flex h-7 shrink-0 items-center rounded-xl border px-2.5 text-[10.5px] font-semibold shadow-sm transition-all active:scale-95 disabled:opacity-40 ${currentTaskMode === option.value
                                            ? 'border-emerald-400/40 bg-emerald-50 text-emerald-700 hover:bg-emerald-100 dark:bg-emerald-500/15 dark:text-emerald-400 dark:border-emerald-500/25 dark:hover:bg-emerald-500/25'
                                            : 'border-zinc-300/60 bg-zinc-100 text-zinc-500 hover:bg-zinc-200 dark:bg-white/5 dark:text-zinc-400 dark:border-white/10 dark:hover:bg-white/10'
                                            }`}>
                                        <span>{option.shortLabel}</span>
                                    </button>
                                ))}
                            </>
                        )}
                    </div>
                )}

                {Array.isArray(smartMoneyRangeGroups) && smartMoneyRangeGroups.length > 0 ? (
                    <SmartMoneyRangeSummaryClean groups={smartMoneyRangeGroups} />
                ) : null}

                <div className="rounded-lg border border-zinc-100 bg-zinc-50/50 dark:border-white/5 dark:bg-[#1c2026]">
                    <button type="button" onClick={() => setExpanded(!expanded)}
                        className="w-full flex items-center justify-between px-2.5 py-1.5">
                        <div className="flex items-center gap-1.5">
                            <div className="text-[10px] font-semibold text-zinc-500 dark:text-white/50 uppercase tracking-wide">资产明细</div>
                            {!expanded && (
                                <div className="text-[9px] text-zinc-400 dark:text-white/35 tabular-nums">
                                    仓位 <NumberFlowValue value={position?.totals?.position_usd} formatter={(v) => formatUsd(v)} />
                                    {' · '}
                                    手续费 <NumberFlowValue value={position?.totals?.fee_usd} formatter={(v) => formatFeeUsd(v)} />
                                </div>
                            )}
                        </div>
                        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"
                            className={`h-3 w-3 text-zinc-400 transition-transform duration-200 ${expanded ? 'rotate-180' : ''}`}>
                            <path d={icons.chevronDown} />
                        </svg>
                    </button>

                    <div className={`collapsible-content ${expanded ? 'expanded' : 'collapsed'}`}>
                        <div className="px-3 pb-3">
                            <div className="grid grid-cols-[1.5fr_1fr_1fr_1fr] gap-2 border-b border-zinc-200/60 pb-1.5 dark:border-white/10">
                                <div className="text-[11px] font-bold text-zinc-500 dark:text-white/60 tracking-wide uppercase">Token</div>
                                <div className="text-[11px] font-bold text-zinc-500 dark:text-white/60 tracking-wide uppercase text-right flex items-center justify-end gap-1">
                                    <Icon path={icons.wallet} className="h-2.5 w-2.5" />钱包
                                </div>
                                <div className="text-[11px] font-bold text-zinc-500 dark:text-white/60 tracking-wide uppercase text-right">仓位</div>
                                <div className="text-[11px] font-bold text-emerald-600/80 dark:text-emerald-500/80 tracking-wide uppercase text-right">手续费</div>
                            </div>

                            {[token0, token1].filter(Boolean).map((row) => (
                                <div key={row.address} className="grid grid-cols-[1.5fr_1fr_1fr_1fr] gap-2 items-center py-2 border-b border-zinc-100/60 dark:border-white/10 last:border-0">
                                    <div className="min-w-0 pr-1">
                                        <div className="text-[13px] font-bold text-zinc-900 dark:text-white/95 truncate">{row.symbol}</div>
                                        <div className="text-xs text-zinc-500 dark:text-white/50 font-mono">
                                            <NumberFlowValue
                                                value={row.price_usd_text || `$${Number(row.price_usd || 0).toFixed(4)}`}
                                                formatter={() => row.price_usd_text || `$${Number(row.price_usd || 0).toFixed(4)}`}
                                            />
                                        </div>
                                    </div>
                                    <div className="text-right min-w-0">
                                        <div className="text-[13px] font-bold text-zinc-900 dark:text-white/95 font-mono tabular-nums truncate">
                                            <NumberFlowValue value={row.wallet_amount} formatter={() => String(row.wallet_amount ?? '--')} />
                                        </div>
                                        <div className="text-xs text-zinc-500 dark:text-white/50 font-mono tabular-nums truncate">
                                            <NumberFlowValue value={row.wallet_usd} formatter={(v) => formatUsd(v)} />
                                        </div>
                                    </div>
                                    <div className="text-right min-w-0">
                                        <div className="text-[13px] font-bold text-zinc-900 dark:text-white/95 font-mono tabular-nums truncate">
                                            <NumberFlowValue value={row.position_amount} formatter={() => String(row.position_amount ?? '--')} />
                                        </div>
                                        <div className="text-xs text-zinc-500 dark:text-white/50 font-mono tabular-nums truncate">
                                            <NumberFlowValue value={row.position_usd} formatter={(v) => formatUsd(v)} />
                                        </div>
                                    </div>
                                    <div className="text-right min-w-0">
                                        <div className="text-[13px] font-bold text-emerald-600 dark:text-emerald-400 font-mono tabular-nums truncate">
                                            <NumberFlowValue value={row.fee_amount} formatter={() => String(row.fee_amount ?? '--')} />
                                        </div>
                                        <div className="text-xs text-emerald-600/70 dark:text-emerald-400/70 font-mono tabular-nums truncate">
                                            <NumberFlowValue value={row.fee_usd} formatter={(v) => formatFeeUsd(v)} />
                                        </div>
                                    </div>
                                </div>
                            ))}

                            <div className="pt-2">
                                <div className="grid grid-cols-4 gap-1.5">
                                    {[
                                        { key: 'wallet', label: '钱包', onClick: openWallet, disabled: false },
                                        { key: 'pool', label: '池子', onClick: openPool, disabled: !poolLink },
                                        { key: 'token0', label: token0?.symbol || 'Token0', onClick: () => openToken(token0?.address), disabled: !token0?.address },
                                        { key: 'token1', label: token1?.symbol || 'Token1', onClick: () => openToken(token1?.address), disabled: !token1?.address },
                                    ].map(({ key, label, onClick, disabled }) => (
                                        <button
                                            key={key}
                                            onClick={onClick}
                                            disabled={disabled}
                                            title={label}
                                            className="inline-flex min-w-0 items-center justify-center rounded-lg border border-zinc-200 bg-white px-1.5 py-1 text-[10px] font-semibold text-zinc-700 hover:bg-zinc-100 active:scale-[0.98] disabled:opacity-40 dark:border-white/15 dark:bg-white/10 dark:text-white/80 dark:hover:bg-white/15"
                                        >
                                            <span className="truncate">{label}</span>
                                        </button>
                                    ))}
                                </div>
                            </div>

                            <div className="pt-2 grid grid-cols-[1.5fr_1fr_1fr_1fr] gap-2 mt-1 border-t border-zinc-100/60 dark:border-white/10">
                                <div className="text-xs font-bold text-zinc-500 dark:text-white/70">合计</div>
                                <div className="text-right text-xs font-bold text-zinc-900 dark:text-white/95 font-mono tabular-nums truncate">
                                    <NumberFlowValue value={position?.totals?.wallet_usd} formatter={(v) => formatUsd(v)} />
                                </div>
                                <div className="text-right text-xs font-bold text-zinc-900 dark:text-white/95 font-mono tabular-nums truncate">
                                    <NumberFlowValue value={position?.totals?.position_usd} formatter={(v) => formatUsd(v)} />
                                </div>
                                <div className="text-right text-xs font-bold text-emerald-600 dark:text-emerald-400 font-mono tabular-nums truncate">
                                    <NumberFlowValue value={position?.totals?.fee_usd} formatter={(v) => formatFeeUsd(v)} />
                                </div>
                            </div>
                        </div>
                    </div>
                </div>

                {false && (
                <div className="rounded-lg border border-zinc-100 bg-zinc-50/50 dark:border-white/5 dark:bg-[#1c2026]">
                    <button type="button" onClick={() => setExpanded(!expanded)}
                        className="w-full flex items-center justify-between px-2.5 py-1.5">
                        <div className="flex items-center gap-1.5">
                            <div className="text-[10px] font-semibold text-zinc-500 dark:text-white/50 uppercase tracking-wide">资产明细</div>
                            {!expanded && (
                                <div className="text-[9px] text-zinc-400 dark:text-white/35 tabular-nums">
                                    仓位 <NumberFlowValue value={position?.totals?.position_usd} formatter={(v) => formatUsd(v)} /> · 手续费 <NumberFlowValue value={position?.totals?.fee_usd} formatter={(v) => formatFeeUsd(v)} />
                                </div>
                            )}
                        </div>
                        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"
                            className={`h-3 w-3 text-zinc-400 transition-transform duration-200 ${expanded ? 'rotate-180' : ''}`}>
                            <path d={icons.chevronDown} />
                        </svg>
                    </button>

                    <div className={`collapsible-content ${expanded ? 'expanded' : 'collapsed'}`}>
                        <div className="px-3 pb-3">
                            <div className="grid grid-cols-[1.5fr_1fr_1fr_1fr] gap-2 pb-1.5 border-b border-zinc-200/60 dark:border-white/10">
                                <div className="text-[11px] font-bold text-zinc-500 dark:text-white/60 tracking-wide uppercase">Token</div>
                                <div className="text-[11px] font-bold text-zinc-500 dark:text-white/60 tracking-wide uppercase text-right flex items-center justify-end gap-1">
                                    <Icon path={icons.wallet} className="h-2.5 w-2.5" />钱包
                                </div>
                                <div className="text-[11px] font-bold text-zinc-500 dark:text-white/60 tracking-wide uppercase text-right">仓位</div>
                                <div className="text-[11px] font-bold text-emerald-600/80 dark:text-emerald-500/80 tracking-wide uppercase text-right">手续费</div>
                            </div>

                            {[token0, token1].filter(Boolean).map((row) => (
                                <div key={row.address} className="grid grid-cols-[1.5fr_1fr_1fr_1fr] gap-2 items-center py-2 border-b border-zinc-100/60 dark:border-white/10 last:border-0">
                                    <div className="min-w-0 pr-1">
                                        <div className="text-[13px] font-bold text-zinc-900 dark:text-white/95 truncate">{row.symbol}</div>
                                        <div className="text-xs text-zinc-500 dark:text-white/50 font-mono">
                                            <NumberFlowValue
                                                value={row.price_usd_text || `$${Number(row.price_usd || 0).toFixed(4)}`}
                                                formatter={() => row.price_usd_text || `$${Number(row.price_usd || 0).toFixed(4)}`}
                                            />
                                        </div>
                                    </div>
                                    <div className="text-right min-w-0">
                                        <div className="text-[13px] font-bold text-zinc-900 dark:text-white/95 font-mono tabular-nums truncate">
                                            <NumberFlowValue value={row.wallet_amount} formatter={() => String(row.wallet_amount ?? '--')} />
                                        </div>
                                        <div className="text-xs text-zinc-500 dark:text-white/50 font-mono tabular-nums truncate">
                                            <NumberFlowValue value={row.wallet_usd} formatter={(v) => formatUsd(v)} />
                                        </div>
                                    </div>
                                    <div className="text-right min-w-0">
                                        <div className="text-[13px] font-bold text-zinc-900 dark:text-white/95 font-mono tabular-nums truncate">
                                            <NumberFlowValue value={row.position_amount} formatter={() => String(row.position_amount ?? '--')} />
                                        </div>
                                        <div className="text-xs text-zinc-500 dark:text-white/50 font-mono tabular-nums truncate">
                                            <NumberFlowValue value={row.position_usd} formatter={(v) => formatUsd(v)} />
                                        </div>
                                    </div>
                                    <div className="text-right min-w-0">
                                        <div className="text-[13px] font-bold text-emerald-600 dark:text-emerald-400 font-mono tabular-nums truncate">
                                            <NumberFlowValue value={row.fee_amount} formatter={() => String(row.fee_amount ?? '--')} />
                                        </div>
                                        <div className="text-xs text-emerald-600/70 dark:text-emerald-400/70 font-mono tabular-nums truncate">
                                            <NumberFlowValue value={row.fee_usd} formatter={(v) => formatFeeUsd(v)} />
                                        </div>
                                    </div>
                                </div>
                            ))}

                            <div className="pt-2">
                                <div className="grid grid-cols-4 gap-1.5">
                                    {[
                                        { key: 'wallet', label: '钱包', onClick: openWallet, disabled: false },
                                        { key: 'pool', label: '池子', onClick: openPool, disabled: !poolLink },
                                        { key: 'token0', label: token0?.symbol || 'Token0', onClick: () => openToken(token0?.address), disabled: !token0?.address },
                                        { key: 'token1', label: token1?.symbol || 'Token1', onClick: () => openToken(token1?.address), disabled: !token1?.address },
                                    ].map(({ key, label, onClick, disabled }) => (
                                        <button
                                            key={key}
                                            onClick={onClick}
                                            disabled={disabled}
                                            title={label}
                                            className="inline-flex min-w-0 items-center justify-center rounded-lg border border-zinc-200 bg-white px-1.5 py-1 text-[10px] font-semibold text-zinc-700 hover:bg-zinc-100 active:scale-[0.98] disabled:opacity-40 dark:border-white/15 dark:bg-white/10 dark:text-white/80 dark:hover:bg-white/15"
                                        >
                                            <span className="truncate">{label}</span>
                                        </button>
                                    ))}
                                </div>
                            </div>

                            <div className="pt-2 grid grid-cols-[1.5fr_1fr_1fr_1fr] gap-2 mt-1 border-t border-zinc-100/60 dark:border-white/10">
                                <div className="text-xs font-bold text-zinc-500 dark:text-white/70">合计</div>
                                <div className="text-right text-xs font-bold text-zinc-900 dark:text-white/95 font-mono tabular-nums truncate">
                                    <NumberFlowValue value={position?.totals?.wallet_usd} formatter={(v) => formatUsd(v)} />
                                </div>
                                <div className="text-right text-xs font-bold text-zinc-900 dark:text-white/95 font-mono tabular-nums truncate">
                                    <NumberFlowValue value={position?.totals?.position_usd} formatter={(v) => formatUsd(v)} />
                                </div>
                                <div className="text-right text-xs font-bold text-emerald-600 dark:text-emerald-400 font-mono tabular-nums truncate">
                                    <NumberFlowValue value={position?.totals?.fee_usd} formatter={(v) => formatFeeUsd(v)} />
                                </div>
                            </div>
                        </div>
                    </div>
                </div>

                )}

                <PriceRangeVisualizer
                    currentPrice={currentPrice}
                    minPrice={rangeMin}
                    maxPrice={rangeMax}
                    pairLabel={pairLabel}
                    gridCount={gridCountRaw}
                    gridStepPct={gridStepPct}
                    rangeBadgeText={displayTaskRange?.badgeText || ''}
                    inRange={position?.in_range}
                    currentGridIndex={currentGridIndex}
                    currentGridLower={gridLower}
                    currentGridUpper={gridUpper}
                    taskRangeText={displayTaskRange?.text || ''}
                    runningDuration={runningDuration}
                />

            </div>
        </div>
    );
}
