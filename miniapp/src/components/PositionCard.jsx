import React, { useEffect, useMemo, useRef, useState } from 'react';
import { openLink } from '../lib/telegram';
import { useDurationFrom, useRelativeTime } from '../lib/time';
import PriceRangeVisualizer from './PriceRangeVisualizer';

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

const USD_DISPLAY_LIMIT = 1e15;
const usdFormatter = new Intl.NumberFormat('en-US', {
    style: 'currency',
    currency: 'USD',
    maximumFractionDigits: 2,
});

const formatUsd = (v) => {
    const n = Number(v ?? 0);
    if (!Number.isFinite(n) || Math.abs(n) > USD_DISPLAY_LIMIT) return '$--';
    return usdFormatter.format(n);
};

const formatFeeUsd = (v) => {
    const n = Number(v ?? 0);
    if (!Number.isFinite(n) || Math.abs(n) > USD_DISPLAY_LIMIT) return '$--';
    if (n === 0) return usdFormatter.format(0);
    const abs = Math.abs(n);
    if (abs < 0.01) return `${n < 0 ? '-' : ''}<$0.01`;
    return usdFormatter.format(n);
};

const STABLE_SYMBOLS = new Set(['USDT', 'USDC', 'BUSD', 'DAI']);
const normalizeSymbol = (value) => String(value || '').trim().toUpperCase();
const isStableSymbol = (symbol) => STABLE_SYMBOLS.has(normalizeSymbol(symbol));

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

const formatPrice = (value) => {
    const n = Number(value);
    if (!Number.isFinite(n)) return '--';
    if (n === 0) return '0';
    const sign = n < 0 ? '-' : '';
    let s = Math.abs(n).toFixed(18).replace(/\.?0+$/, '');
    if (!s.includes('.')) return `${sign}${s}`;
    const [intPart, fracRaw] = s.split('.');
    const frac = fracRaw || '';
    let nonZero = 0;
    let cut = frac.length;
    for (let i = 0; i < frac.length; i++) {
        if (frac[i] !== '0') {
            nonZero += 1;
            if (nonZero === 2) { cut = i + 1; break; }
        }
    }
    const trimmed = frac.slice(0, cut);
    return trimmed ? `${sign}${intPart}.${trimmed}` : `${sign}${intPart}`;
};

const getStatusTheme = (label) => {
    if (label?.includes('错误'))
        return { pill: 'bg-red-500/15 text-red-600 ring-red-500/25 dark:text-red-300 dark:ring-red-400/30', dot: 'bg-red-500', bar: 'bg-gradient-to-b from-red-500 to-red-600' };
    if (label?.includes('暂停') || label?.includes('停止') || label?.includes('再平衡') || label?.includes('撤出'))
        return { pill: 'bg-amber-500/15 text-amber-700 ring-amber-500/25 dark:text-amber-300 dark:ring-amber-400/30', dot: 'bg-amber-500', bar: 'bg-gradient-to-b from-amber-400 to-amber-500' };
    if (label?.includes('等待'))
        return { pill: 'bg-sky-500/15 text-sky-700 ring-sky-500/25 dark:text-sky-300 dark:ring-sky-400/30', dot: 'bg-sky-500', bar: 'bg-gradient-to-b from-sky-400 to-sky-500' };
    return { pill: 'bg-emerald-500/15 text-emerald-700 ring-emerald-500/25 dark:text-emerald-300 dark:ring-emerald-400/30', dot: 'bg-emerald-500', bar: 'bg-gradient-to-b from-emerald-400 to-emerald-500' };
};

const normalizeHexPrefixed = (v) => {
    const raw = String(v || '').trim();
    if (!raw) return '';
    if (raw.startsWith('0x') || raw.startsWith('0X')) return `0x${raw.slice(2)}`;
    return `0x${raw}`;
};

// 由 tickSpacing 推导费率标签
const feeRateFromTickSpacing = (ts) => {
    const map = { 1: '0.01%', 10: '0.05%', 50: '0.25%', 60: '0.30%', 100: '0.50%', 200: '1%', 2000: '2%' };
    return map[Number(ts)] ?? null;
};

export default function PositionCard({
    position,
    walletAddress,
    bnbBalance,
    pollIntervalSec,
    updatedAt,
    allowTaskActions = true,
    onSetTaskPaused,
    onStopTask,
    onDeleteTask,
    onUpdateTaskRange,
    batchMode = false,
    isSelected = false,
    onToggleSelect,
}) {
    const [expanded, setExpanded] = useState(true);

    const runningDuration = useDurationFrom(position?.running_since);
    const updateTimeText = useRelativeTime(updatedAt);

    const token0 = position?.token_rows?.[0];
    const token1 = position?.token_rows?.[1];

    const stableIndex = useMemo(() => {
        if (isStableSymbol(token0?.symbol)) return 0;
        if (isStableSymbol(token1?.symbol)) return 1;
        const p0 = Number(token0?.price_usd);
        if (Number.isFinite(p0) && p0 > 0.98 && p0 < 1.02) return 0;
        const p1 = Number(token1?.price_usd);
        if (Number.isFinite(p1) && p1 > 0.98 && p1 < 1.02) return 1;
        return -1;
    }, [token0?.symbol, token1?.symbol, token0?.price_usd, token1?.price_usd]);

    const baseSymbol = stableIndex === 0 ? token1?.symbol : token0?.symbol;
    const quoteSymbol = stableIndex === 0 ? token0?.symbol : token1?.symbol;
    const pairLabel = baseSymbol && quoteSymbol ? `${baseSymbol}/${quoteSymbol}` : baseSymbol || quoteSymbol || '';

    const displayTitle = useMemo(() => {
        if (!position?.title) return pairLabel || '--';
        const parts = position.title.split('-');
        if (parts.length >= 3) {
            // "panv3-USDT-POWER-1.0%" -> "USDT-POWER"
            return parts.slice(1, -1).join('-');
        }
        return position.title;
    }, [position?.title, pairLabel]);

    const { totalValue, pnlAbsolute, pnlPercent, hasPnL } = useMemo(() => {
        const positionUsd = Number(position?.totals?.position_usd || 0);
        const feeUsd = Number(position?.totals?.fee_usd || 0);
        const total = (Number.isFinite(positionUsd) ? positionUsd : 0) + (Number.isFinite(feeUsd) ? feeUsd : 0);
        const initialCost = Number(position?.initial_cost_usd || position?.net_invested_usd || 0);
        if (Number.isFinite(initialCost) && initialCost > 0) {
            const pnl = total - initialCost;
            return { totalValue: total, pnlAbsolute: pnl, pnlPercent: (pnl / initialCost) * 100, hasPnL: true };
        }
        return { totalValue: total, pnlAbsolute: 0, pnlPercent: 0, hasPnL: false };
    }, [position?.totals?.position_usd, position?.totals?.fee_usd, position?.initial_cost_usd, position?.net_invested_usd]);

    const chain = useMemo(() => String(position?.chain || '').trim().toLowerCase() || 'bsc', [position?.chain]);
    const explorerBase = useMemo(() => (chain === 'base' ? 'https://basescan.org' : 'https://bscscan.com'), [chain]);
    const geckoNetwork = useMemo(() => (chain === 'base' ? 'base' : 'bsc'), [chain]);

    const poolLink = useMemo(() => {
        const pool = normalizeHexPrefixed(position?.pool_id);
        if (!pool) return null;
        if (/^0x[a-fA-F0-9]{40}$/.test(pool)) return `${explorerBase}/address/${pool}`;
        if (/^0x[a-fA-F0-9]{64}$/.test(pool)) return `https://www.geckoterminal.com/${geckoNetwork}/pools/${pool.toLowerCase()}`;
        return null;
    }, [position?.pool_id, explorerBase, geckoNetwork]);

    const openWallet = () => openLink(`${explorerBase}/address/${walletAddress}`);
    const openPool = () => poolLink && openLink(poolLink);
    const openToken = (addr) => addr && openLink(`${explorerBase}/token/${addr}`);

    const decimals0 = useMemo(() => Number(token0?.decimals ?? 18), [token0?.decimals]);
    const decimals1 = useMemo(() => Number(token1?.decimals ?? 18), [token1?.decimals]);

    const currentTick = Number(position?.current_tick);
    const tickLowerRaw = Number(position?.tick_lower);
    const tickUpperRaw = Number(position?.tick_upper);
    const tickSpacingRaw = Number(position?.pool?.tickSpacing ?? position?.tick_spacing);

    const currentPriceBase = useMemo(() => priceFromTick(currentTick, decimals0, decimals1), [currentTick, decimals0, decimals1]);
    const currentPrice = stableIndex === 0 ? safeInvert(currentPriceBase) : currentPriceBase;

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
        const p1 = stableIndex === 0 ? safeInvert(p1Base) : p1Base;
        const p2 = stableIndex === 0 ? safeInvert(p2Base) : p2Base;
        if (p1 === null || p2 === null) return { gridLower: null, gridUpper: null };
        return { gridLower: Math.min(p1, p2), gridUpper: Math.max(p1, p2) };
    }, [currentGridIndex, tickLowerRaw, tickSpacingRaw, decimals0, decimals1, stableIndex]);

    const gridCountRaw = useMemo(() => {
        if (!Number.isFinite(tickLowerRaw) || !Number.isFinite(tickUpperRaw)) return null;
        const diff = Math.abs(tickUpperRaw - tickLowerRaw);
        if (tickSpacingRaw && tickSpacingRaw > 0) return Math.round(diff / tickSpacingRaw);
        return null;
    }, [tickLowerRaw, tickUpperRaw, tickSpacingRaw]);

    const openPrice = useMemo(() => {
        if (stableIndex < 0) return null;
        const n = Number(position?.open_price);
        return Number.isFinite(n) && n > 0 ? n : null;
    }, [position?.open_price, stableIndex]);

    const rangeLowerBase = useMemo(() => priceFromTick(position?.tick_lower, decimals0, decimals1), [position?.tick_lower, decimals0, decimals1]);
    const rangeUpperBase = useMemo(() => priceFromTick(position?.tick_upper, decimals0, decimals1), [position?.tick_upper, decimals0, decimals1]);
    const rangeLower = stableIndex === 0 ? safeInvert(rangeLowerBase) : rangeLowerBase;
    const rangeUpper = stableIndex === 0 ? safeInvert(rangeUpperBase) : rangeUpperBase;
    const rangeReady = Number.isFinite(rangeLower) && Number.isFinite(rangeUpper);
    const rangeMin = rangeReady ? Math.min(rangeLower, rangeUpper) : null;
    const rangeMax = rangeReady ? Math.max(rangeLower, rangeUpper) : null;

    const taskRange = useMemo(() => {
        const low = Number(position?.task_range_lower_pct);
        const up = Number(position?.task_range_upper_pct);
        if (!Number.isFinite(low) || !Number.isFinite(up) || low <= 0 || up <= 0) return null;
        const asymmetric = Math.abs(low - up) >= 0.01;
        const avg = (low + up) / 2;
        return { text: asymmetric ? `下 ${low.toFixed(2)}% / 上 ${up.toFixed(2)}%` : `±${avg.toFixed(2)}%`, deviation: avg };
    }, [position?.task_range_lower_pct, position?.task_range_upper_pct]);

    const taskId = useMemo(() => {
        const raw = Number(position?.task_id);
        return Number.isFinite(raw) && raw > 0 ? raw : 0;
    }, [position?.task_id]);

    const taskPaused = Boolean(position?.task_paused);
    const statusLabel = String(position?.status_label || '');
    const isStopped = statusLabel.includes('已停止');
    const isStopping = statusLabel.includes('停止中') || statusLabel.includes('撤出中');
    const hasActions = typeof onSetTaskPaused === 'function' || typeof onStopTask === 'function' || typeof onDeleteTask === 'function' || typeof onUpdateTaskRange === 'function';
    const canTaskAction = Boolean(allowTaskActions) && hasActions && taskId > 0;
    const canPauseAction = canTaskAction && typeof onSetTaskPaused === 'function' && !isStopping;
    const canUpdateRangeAction = canTaskAction && typeof onUpdateTaskRange === 'function' && !isStopping;
    const canStopAction = canTaskAction && typeof onStopTask === 'function' && !isStopped && !isStopping;
    const canDeleteAction = canTaskAction && typeof onDeleteTask === 'function' && !isStopping;

    const [menuOpen, setMenuOpen] = useState(false);
    const [actionPending, setActionPending] = useState('');
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

    const pnlPositive = pnlAbsolute >= 0;
    const statusTheme = getStatusTheme(statusLabel);

    // 费率标签（从 tickSpacing 推导）
    const feeLabel = useMemo(() => {
        const ts = position?.pool?.tickSpacing ?? position?.tick_spacing;
        return feeRateFromTickSpacing(ts);
    }, [position?.pool?.tickSpacing, position?.tick_spacing]);

    return (
        <div className="relative rounded-2xl border border-zinc-200/80 bg-white dark:border-white/5 dark:bg-[#131518] shadow-sm overflow-hidden transition-all duration-200 active:scale-[0.985]">
            <div className="px-3 pt-3 pb-2 flex flex-col gap-2">
                {/* ══════════════════════════════════════════
                    区域 1：标题行 & 操作菜单
                ══════════════════════════════════════════ */}
                <div className="flex items-start justify-between gap-2">
                    {/* 左侧主要信息 */}
                    <div className="flex items-start gap-2 min-w-0 flex-1">
                        {/* 批量复选框 */}
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
                            {/* 交易对名称 + 费率 */}
                            <div className="flex items-center gap-1.5 flex-wrap pr-1">
                                <span className="text-[15px] font-bold text-zinc-900 dark:text-white/95 leading-tight truncate max-w-full">
                                    {displayTitle}
                                </span>
                                {feeLabel && (
                                    <span className="inline-flex items-center rounded bg-violet-500/12 px-1 text-[9px] font-bold text-violet-600 dark:bg-violet-500/18 dark:text-violet-300 ring-1 ring-violet-500/20 dark:ring-violet-400/25 shrink-0">
                                        {feeLabel}
                                    </span>
                                )}
                            </div>
                            {/* 状态 + 任务ID */}
                            <div className="flex flex-wrap items-center gap-1.5 pr-1">
                                <span className={`inline-flex items-center gap-1 rounded-full px-1.5 py-0.5 text-[10px] font-semibold ring-1 shrink-0 ${statusTheme.pill}`}>
                                    <span className={`h-1 w-1 rounded-full shrink-0 ${statusTheme.dot}`} />
                                    <span className="truncate max-w-[70px]">{statusLabel || '运行中'}</span>
                                </span>
                                {taskId > 0 && (
                                    <span className="text-[10px] font-medium text-zinc-400 dark:text-white/30 shrink-0">
                                        #{taskId}
                                    </span>
                                )}
                            </div>
                        </div>
                    </div>

                    {/* 右侧：总价值 + 操作菜单 */}
                    <div className="flex shrink-0 items-start gap-2">
                        {/* 总价值 + PnL */}
                        <div className="text-right">
                            <div className="text-[9px] font-medium text-zinc-400 dark:text-white/35 uppercase tracking-wide mb-0.5">总计</div>
                            <div className="text-lg font-extrabold text-zinc-900 dark:text-white/95 tabular-nums leading-none">
                                {formatUsd(totalValue)}
                            </div>
                            {hasPnL && (
                                <div className={`mt-1 inline-flex items-center gap-0.5 rounded px-1 text-[10px] font-bold tabular-nums ${pnlPositive ? 'bg-emerald-500/12 text-emerald-600 dark:text-emerald-400' : 'bg-red-500/12 text-red-600 dark:text-red-400'}`}>
                                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round" className="h-2 w-2 shrink-0">
                                        <path d={pnlPositive ? icons.arrowUp : icons.arrowDown} />
                                    </svg>
                                    {pnlAbsolute >= 0 ? '+' : ''}{formatUsd(pnlAbsolute)}
                                </div>
                            )}
                        </div>

                        {/* 操作菜单 */}
                        {canTaskAction && (
                            <div className="relative z-20" ref={menuRef}>
                                <button
                                    type="button"
                                    onClick={() => setMenuOpen((v) => !v)}
                                    className="inline-flex h-7 w-7 items-center justify-center rounded-lg border border-zinc-200/80 bg-zinc-50 text-zinc-500 hover:bg-zinc-100 hover:text-zinc-700 active:scale-95 transition-all dark:border-white/5 dark:bg-[#1a1c20] dark:text-white/50 dark:hover:bg-white/5 dark:hover:text-white/80"
                                    aria-label="任务操作"
                                    disabled={Boolean(actionPending)}
                                >
                                    <Icon path={icons.kebab} className="h-4 w-4" />
                                </button>
                                {menuOpen && (
                                    <div className="absolute right-0 top-full z-30 mt-1.5 w-32 overflow-hidden rounded-xl border border-zinc-200/80 bg-white/95 backdrop-blur-xl shadow-xl dark:border-white/10 dark:bg-[#1f2126]/95">
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

                {/* ══════════════════════════════════════════
                    区域 2：余额明细（可折叠）
                ══════════════════════════════════════════ */}
                <div className="rounded-lg border border-zinc-100 bg-zinc-50/50 dark:border-white/5 dark:bg-[#1a1c20]">
                    <button type="button" onClick={() => setExpanded(!expanded)}
                        className="w-full flex items-center justify-between px-2.5 py-1.5">
                        <div className="flex items-center gap-1.5">
                            <div className="text-[10px] font-semibold text-zinc-500 dark:text-white/50 uppercase tracking-wide">余额明细</div>
                            {!expanded && (
                                <div className="text-[9px] text-zinc-400 dark:text-white/35 tabular-nums">
                                    仓位 {formatUsd(position?.totals?.position_usd)} · 费用 {formatFeeUsd(position?.totals?.fee_usd)}
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
                            {/* 表头 */}
                            <div className="grid grid-cols-[1.5fr_1fr_1fr_1fr] gap-2 pb-1.5 border-b border-zinc-200/60 dark:border-white/10">
                                <div className="text-[11px] font-bold text-zinc-500 dark:text-white/60 tracking-wide uppercase">Token</div>
                                <div className="text-[11px] font-bold text-zinc-500 dark:text-white/60 tracking-wide uppercase text-right flex items-center justify-end gap-1">
                                    <Icon path={icons.wallet} className="h-2.5 w-2.5" />钱包
                                </div>
                                <div className="text-[11px] font-bold text-zinc-500 dark:text-white/60 tracking-wide uppercase text-right">仓位</div>
                                <div className="text-[11px] font-bold text-emerald-600/80 dark:text-emerald-500/80 tracking-wide uppercase text-right">手续费</div>
                            </div>

                            {/* Token 行 */}
                            {[token0, token1].filter(Boolean).map((row) => (
                                <div key={row.address} className="grid grid-cols-[1.5fr_1fr_1fr_1fr] gap-2 items-center py-2 border-b border-zinc-100/60 dark:border-white/10 last:border-0">
                                    <div className="min-w-0 pr-1">
                                        <div className="text-xs font-bold text-zinc-900 dark:text-white/95 truncate">{row.symbol}</div>
                                        <div className="text-[11px] text-zinc-500 dark:text-white/50 font-mono">
                                            {row.price_usd_text || `$${Number(row.price_usd || 0).toFixed(4)}`}
                                        </div>
                                    </div>
                                    <div className="text-right min-w-0">
                                        <div className="text-xs font-bold text-zinc-900 dark:text-white/95 font-mono tabular-nums truncate">{row.wallet_amount}</div>
                                        <div className="text-[11px] text-zinc-500 dark:text-white/50 font-mono tabular-nums truncate">{formatUsd(row.wallet_usd)}</div>
                                    </div>
                                    <div className="text-right min-w-0">
                                        <div className="text-xs font-bold text-zinc-900 dark:text-white/95 font-mono tabular-nums truncate">{row.position_amount}</div>
                                        <div className="text-[11px] text-zinc-500 dark:text-white/50 font-mono tabular-nums truncate">{formatUsd(row.position_usd)}</div>
                                    </div>
                                    <div className="text-right min-w-0">
                                        <div className="text-xs font-bold text-emerald-600 dark:text-emerald-400 font-mono tabular-nums truncate">{row.fee_amount}</div>
                                        <div className="text-[11px] text-emerald-600/70 dark:text-emerald-400/70 font-mono tabular-nums truncate">{formatFeeUsd(row.fee_usd)}</div>
                                    </div>
                                </div>
                            ))}

                            {/* 小计行 */}
                            <div className="pt-2 grid grid-cols-[1.5fr_1fr_1fr_1fr] gap-2 mt-1 border-t border-zinc-100/60 dark:border-white/10">
                                <div className="text-[11px] font-bold text-zinc-500 dark:text-white/70">小计</div>
                                <div className="text-right text-[11px] font-bold text-zinc-900 dark:text-white/95 font-mono tabular-nums truncate">{formatUsd(position?.totals?.wallet_usd)}</div>
                                <div className="text-right text-[11px] font-bold text-zinc-900 dark:text-white/95 font-mono tabular-nums truncate">{formatUsd(position?.totals?.position_usd)}</div>
                                <div className="text-right text-[11px] font-bold text-emerald-600 dark:text-emerald-400 font-mono tabular-nums truncate">{formatFeeUsd(position?.totals?.fee_usd)}</div>
                            </div>
                        </div>
                    </div>
                </div>

                {/* ══════════════════════════════════════════
                    区域 3：价格范围可视化（移动到余额后）
                ══════════════════════════════════════════ */}
                <PriceRangeVisualizer
                    currentPrice={currentPrice}
                    minPrice={rangeMin}
                    maxPrice={rangeMax}
                    pairLabel={pairLabel}
                    gridCount={gridCountRaw}
                    deviation={taskRange?.deviation}
                    inRange={position?.in_range}
                    currentGridIndex={currentGridIndex}
                    currentGridLower={gridLower}
                    currentGridUpper={gridUpper}
                />

                {/* ══════════════════════════════════════════
                    区域 4：底部操作行（策略区间 + 快捷链接）
                ══════════════════════════════════════════ */}
                <div className="flex items-center gap-2">
                    {/* 策略区间（左侧，flex-1） */}
                    {taskRange ? (
                        <div className="flex-1 min-w-0 flex items-center gap-1.5 rounded-xl border border-zinc-100/80 bg-zinc-50/80 px-2.5 py-2 dark:border-white/10 dark:bg-[#0f1116]">
                            <div className="h-1.5 w-1.5 rounded-full bg-sky-500 shrink-0" />
                            <div className="text-[10px] text-zinc-500 dark:text-white/45 truncate">再平衡</div>
                            <div className="text-[11px] font-bold tabular-nums text-sky-600 dark:text-sky-400 ml-auto shrink-0">{taskRange.text}</div>
                        </div>
                    ) : (
                        <div className="flex-1" />
                    )}

                    {/* 快捷链接按钮（右侧）*/}
                    <div className="flex items-center gap-1 shrink-0">
                        {[
                            { label: '钱包', onClick: openWallet, disabled: false },
                            { label: '池子', onClick: openPool, disabled: !poolLink },
                            { label: token0?.symbol || 'T0', onClick: () => openToken(token0?.address), disabled: !token0?.address },
                            { label: token1?.symbol || 'T1', onClick: () => openToken(token1?.address), disabled: !token1?.address },
                        ].map(({ label, onClick, disabled }) => (
                            <button
                                key={label}
                                onClick={onClick}
                                disabled={disabled}
                                title={label}
                                className="rounded-lg border border-zinc-200/80 bg-white/60 px-2 py-1.5 text-[10px] font-semibold text-zinc-600 hover:bg-white hover:border-zinc-300 active:scale-95 disabled:opacity-35 transition-all dark:border-white/8 dark:bg-white/4 dark:text-white/55 dark:hover:bg-white/10 dark:hover:border-white/15 truncate max-w-[44px]"
                            >
                                {label}
                            </button>
                        ))}
                    </div>
                </div>

            </div>
        </div>
    );
}
