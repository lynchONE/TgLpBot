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
    refresh: 'M17.65 6.35A7.95 7.95 0 0012 4V1L7 6l5 5V7a5 5 0 11-5 5H5a7 7 0 107.65-5.65z',
    link: 'M3.9 12a5 5 0 015-5h3v2h-3a3 3 0 000 6h3v2h-3a5 5 0 01-5-5zm7-1h2v2h-2v-2zm4.1-4h3a5 5 0 010 10h-3v-2h3a3 3 0 000-6h-3V7z',
    kebab: 'M12 7a2 2 0 110-4 2 2 0 010 4zm0 7a2 2 0 110-4 2 2 0 010 4zm0 7a2 2 0 110-4 2 2 0 010 4z',
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
            if (nonZero === 2) {
                cut = i + 1;
                break;
            }
        }
    }
    const trimmed = frac.slice(0, cut);
    return trimmed ? `${sign}${intPart}.${trimmed}` : `${sign}${intPart}`;
};

const pillClassForStatus = (label) => {
    if (label?.includes('错误'))
        return 'bg-red-500/10 text-red-700 ring-red-500/20 dark:bg-red-500/15 dark:text-red-300 dark:ring-red-500/30';
    if (label?.includes('暂停'))
        return 'bg-amber-500/10 text-amber-800 ring-amber-500/20 dark:bg-amber-500/15 dark:text-amber-300 dark:ring-amber-500/30';
    if (label?.includes('停止'))
        return 'bg-amber-500/10 text-amber-800 ring-amber-500/20 dark:bg-amber-500/15 dark:text-amber-300 dark:ring-amber-500/30';
    if (label?.includes('等待'))
        return 'bg-sky-500/10 text-sky-800 ring-sky-500/20 dark:bg-sky-500/15 dark:text-sky-300 dark:ring-sky-500/30';
    return 'bg-emerald-500/10 text-emerald-800 ring-emerald-500/20 dark:bg-emerald-500/15 dark:text-emerald-300 dark:ring-emerald-500/30';
};

const normalizeHexPrefixed = (v) => {
    const raw = String(v || '').trim();
    if (!raw) return '';
    if (raw.startsWith('0x') || raw.startsWith('0X')) return `0x${raw.slice(2)}`;
    return `0x${raw}`;
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
}) {
    // 实时更新的时间显示
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

    const titleRight = useMemo(() => {
        const positionUsd = Number(position?.totals?.position_usd || 0);
        const feeUsd = Number(position?.totals?.fee_usd || 0);
        const total = (Number.isFinite(positionUsd) ? positionUsd : 0) + (Number.isFinite(feeUsd) ? feeUsd : 0);
        return formatUsd(total);
    }, [position?.totals?.position_usd, position?.totals?.fee_usd]);

    const poolLink = useMemo(() => {
        const pool = normalizeHexPrefixed(position?.pool_id);
        if (!pool) return null;
        if (/^0x[a-fA-F0-9]{40}$/.test(pool)) return `https://bscscan.com/address/${pool}`;
        if (/^0x[a-fA-F0-9]{64}$/.test(pool)) return `https://www.geckoterminal.com/bsc/pools/${pool.toLowerCase()}`;
        return null;
    }, [position?.pool_id]);

    const openWallet = () => openLink(`https://bscscan.com/address/${walletAddress}`);
    const openPool = () => poolLink && openLink(poolLink);
    const openToken = (addr) => addr && openLink(`https://bscscan.com/token/${addr}`);

    const decimals0 = useMemo(() => Number(token0?.decimals ?? 18), [token0?.decimals]);
    const decimals1 = useMemo(() => Number(token1?.decimals ?? 18), [token1?.decimals]);

    const currentPriceBase = useMemo(
        () => priceFromTick(position?.current_tick, decimals0, decimals1),
        [position?.current_tick, decimals0, decimals1]
    );
    const currentPrice = stableIndex === 0 ? safeInvert(currentPriceBase) : currentPriceBase;

    const rangeLowerBase = useMemo(
        () => priceFromTick(position?.tick_lower, decimals0, decimals1),
        [position?.tick_lower, decimals0, decimals1]
    );
    const rangeUpperBase = useMemo(
        () => priceFromTick(position?.tick_upper, decimals0, decimals1),
        [position?.tick_upper, decimals0, decimals1]
    );
    const rangeLower = stableIndex === 0 ? safeInvert(rangeLowerBase) : rangeLowerBase;
    const rangeUpper = stableIndex === 0 ? safeInvert(rangeUpperBase) : rangeUpperBase;
    const rangeReady = Number.isFinite(rangeLower) && Number.isFinite(rangeUpper);
    const rangeMin = rangeReady ? Math.min(rangeLower, rangeUpper) : null;
    const rangeMax = rangeReady ? Math.max(rangeLower, rangeUpper) : null;

    const priceProgress = useMemo(() => {
        if (!Number.isFinite(currentPrice) || !Number.isFinite(rangeMin) || !Number.isFinite(rangeMax)) return null;
        const den = rangeMax - rangeMin;
        if (!Number.isFinite(den) || den <= 0) return null;
        const p = (currentPrice - rangeMin) / den;
        if (!Number.isFinite(p)) return null;
        return Math.max(0, Math.min(1, p));
    }, [currentPrice, rangeMin, rangeMax]);
    const progressPercent = useMemo(() => {
        if (priceProgress === null) return null;
        return Math.max(0, Math.min(100, priceProgress * 100));
    }, [priceProgress]);

    const currentPriceText = Number.isFinite(currentPrice)
        ? `${formatPrice(currentPrice)}${quoteSymbol ? ` ${quoteSymbol}` : ''}`
        : '--';
    const rangeText = rangeReady
        ? `${formatPrice(rangeMin)} ~ ${formatPrice(rangeMax)}${quoteSymbol ? ` ${quoteSymbol}` : ''}`
        : '--';
    const pairMetaText = pairLabel ? `${pairLabel} · ` : '';

    const taskId = useMemo(() => {
        const raw = Number(position?.task_id);
        return Number.isFinite(raw) && raw > 0 ? raw : 0;
    }, [position?.task_id]);
    const taskPaused = Boolean(position?.task_paused);
    const statusLabel = String(position?.status_label || '');
    const isStopped = statusLabel.includes('已停止');
    const isStopping = statusLabel.includes('停止中') || statusLabel.includes('撤出中');
    const hasActions = typeof onSetTaskPaused === 'function' || typeof onStopTask === 'function' || typeof onDeleteTask === 'function';
    const canTaskAction = Boolean(allowTaskActions) && hasActions && taskId > 0;
    const canPauseAction = canTaskAction && typeof onSetTaskPaused === 'function' && !isStopping;
    const canStopAction = canTaskAction && typeof onStopTask === 'function' && !isStopped && !isStopping;
    const canDeleteAction = canTaskAction && typeof onDeleteTask === 'function' && !isStopping;

    const [menuOpen, setMenuOpen] = useState(false);
    const [actionPending, setActionPending] = useState('');
    const menuRef = useRef(null);

    useEffect(() => {
        if (!menuOpen) return;
        const onPointerDown = (e) => {
            if (!menuRef.current) return;
            if (menuRef.current.contains(e.target)) return;
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
        try {
            await handler();
        } finally {
            setActionPending('');
            setMenuOpen(false);
        }
    };

    const togglePause = () => runAction('pause', () => onSetTaskPaused?.(taskId, !taskPaused));
    const stopTask = () => runAction('stop', () => onStopTask?.(taskId));
    const deleteTask = () => runAction('delete', () => onDeleteTask?.(taskId));

    return (
        <div className="relative rounded-2xl border border-zinc-200 bg-white p-4 shadow-sm dark:border-white/10 dark:bg-[#111318] dark:shadow-none">
            {canTaskAction ? (
                <div className="absolute right-4 top-4 z-20" ref={menuRef}>
                    <button
                        type="button"
                        onClick={() => setMenuOpen((v) => !v)}
                        className="inline-flex h-9 w-9 items-center justify-center rounded-xl border border-zinc-200 bg-white/70 text-zinc-700 hover:bg-white active:bg-white dark:border-white/10 dark:bg-[#0f1116] dark:text-white/70 dark:hover:bg-white/10 dark:active:bg-white/15"
                        aria-label="任务操作"
                        aria-expanded={menuOpen}
                        disabled={Boolean(actionPending)}
                    >
                        <Icon path={icons.kebab} className="h-5 w-5" />
                    </button>
                    {menuOpen ? (
                        <div className="absolute right-0 top-full z-20 mt-2 w-36 overflow-hidden rounded-xl border border-zinc-200 bg-white shadow-lg dark:border-white/10 dark:bg-[#111318]">
                            {typeof onSetTaskPaused === 'function' ? (
                                <button
                                    type="button"
                                    onClick={togglePause}
                                    disabled={!canPauseAction || Boolean(actionPending)}
                                    className="w-full px-3 py-2 text-left text-xs font-semibold text-zinc-800 hover:bg-zinc-100 active:bg-zinc-100 disabled:opacity-50 dark:text-white/80 dark:hover:bg-white/10 dark:active:bg-white/10"
                                >
                                    {actionPending === 'pause' ? '处理中...' : taskPaused ? '恢复任务' : '暂停任务'}
                                </button>
                            ) : null}
                            {typeof onStopTask === 'function' ? (
                                <button
                                    type="button"
                                    onClick={stopTask}
                                    disabled={!canStopAction || Boolean(actionPending)}
                                    className="w-full border-t border-zinc-100 px-3 py-2 text-left text-xs font-semibold text-amber-700 hover:bg-amber-50 active:bg-amber-50 disabled:opacity-50 dark:border-white/10 dark:text-amber-300 dark:hover:bg-white/10 dark:active:bg-white/10"
                                >
                                    {actionPending === 'stop' ? '处理中...' : isStopping ? '停止中...' : '停止任务'}
                                </button>
                            ) : null}
                            {typeof onDeleteTask === 'function' ? (
                                <button
                                    type="button"
                                    onClick={deleteTask}
                                    disabled={!canDeleteAction || Boolean(actionPending)}
                                    className="w-full border-t border-zinc-100 px-3 py-2 text-left text-xs font-semibold text-red-600 hover:bg-red-50 active:bg-red-50 disabled:opacity-50 dark:border-white/10 dark:text-red-300 dark:hover:bg-white/10 dark:active:bg-white/10"
                                >
                                    {actionPending === 'delete' ? '删除中...' : '删除任务'}
                                </button>
                            ) : null}
                        </div>
                    ) : null}
                </div>
            ) : null}
            <div className="flex items-start justify-between gap-3">
                <div>
                    <div className="text-base font-semibold text-zinc-900 dark:text-white/90">{position?.title}</div>
                    <div className="mt-2 flex flex-wrap items-center gap-2">
                        <span
                            className={`inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-semibold ring-1 ${pillClassForStatus(
                                position?.status_label
                            )}`}
                        >
                            <span className="h-1.5 w-1.5 rounded-full bg-current opacity-90" />
                            {position?.status_label || '运行中'}
                        </span>
                        <span className="inline-flex items-center gap-1 rounded-full bg-white/70 px-2 py-0.5 text-xs text-zinc-700 ring-1 ring-zinc-200 dark:bg-[#0f1116] dark:text-white/70 dark:ring-white/10">
                            <Icon path={icons.trend} className="h-3.5 w-3.5 text-zinc-500 dark:text-white/60" />
                            {bnbBalance} BNB
                        </span>
                    </div>
                </div>
                <div className={`text-right ${canTaskAction ? 'pr-12' : ''}`}>
                    <div className="text-xs text-zinc-500 dark:text-white/50">总计（仓位+手续费）</div>
                    <div className="text-lg font-extrabold text-emerald-700 dark:text-emerald-300">{titleRight}</div>
                </div>
            </div>

            <div className="mt-4 rounded-xl border border-zinc-200 bg-zinc-50 p-3 dark:border-white/10 dark:bg-[#0f1116]">
                <div className="flex items-center justify-between">
                    <div className="text-xs font-semibold text-zinc-700 dark:text-white/70">余额信息</div>
                    <div className="text-[11px] text-zinc-500 dark:text-white/40">钱包 vs 仓位 vs 手续费</div>
                </div>
                <div className="mt-3 grid grid-cols-4 gap-2 text-[11px] text-zinc-500 dark:text-white/40">
                    <div>Token</div>
                    <div className="flex items-center gap-1 justify-end">
                        <Icon path={icons.wallet} className="h-3.5 w-3.5" />
                        钱包
                    </div>
                    <div className="justify-end flex items-center gap-1"># 仓位</div>
                    <div className="justify-end flex items-center gap-1 text-emerald-700/80 dark:text-emerald-300/80">手续费</div>
                </div>

                {[token0, token1].filter(Boolean).map((row) => (
                    <div key={row.address} className="mt-3 grid grid-cols-4 gap-2 items-start">
                        <div className="min-w-0">
                            <div className="text-sm font-bold text-zinc-900 dark:text-white/90 truncate" title={row.symbol}>{row.symbol}</div>
                            <div className="text-[11px] text-zinc-500 dark:text-white/40 truncate">{row.price_usd_text || `$${Number(row.price_usd || 0).toFixed(4)}`}</div>
                        </div>
                        <div className="text-right">
                            <div className="text-sm font-semibold text-zinc-900 dark:text-white/90 tabular-nums">{row.wallet_amount}</div>
                            <div className="text-[11px] text-zinc-500 dark:text-white/40 tabular-nums">{formatUsd(row.wallet_usd)}</div>
                        </div>
                        <div className="text-right">
                            <div className="text-sm font-semibold text-zinc-900 dark:text-white/90 tabular-nums">{row.position_amount}</div>
                            <div className="text-[11px] text-zinc-500 dark:text-white/40 tabular-nums">{formatUsd(row.position_usd)}</div>
                        </div>
                        <div className="text-right">
                            <div className="text-sm font-semibold text-emerald-700 dark:text-emerald-300 tabular-nums">{row.fee_amount}</div>
                            <div className="text-[11px] text-emerald-700/70 dark:text-emerald-300/70 tabular-nums">{formatUsd(row.fee_usd)}</div>
                        </div>
                    </div>
                ))}

                <div className="mt-3 border-t border-zinc-200 pt-3 grid grid-cols-4 gap-2 text-sm font-semibold tabular-nums dark:border-white/10">
                    <div className="text-zinc-600 dark:text-white/60">小计</div>
                    <div className="text-right text-zinc-900 dark:text-white/80">{formatUsd(position?.totals?.wallet_usd)}</div>
                    <div className="text-right text-zinc-900 dark:text-white/80">{formatUsd(position?.totals?.position_usd)}</div>
                    <div className="text-right text-emerald-700 dark:text-emerald-300">{formatUsd(position?.totals?.fee_usd)}</div>
                </div>
            </div>

            <div className="mt-3 grid grid-cols-4 gap-2">
                <button
                    onClick={openWallet}
                    className="rounded-xl border border-zinc-200 bg-white/70 py-2 text-xs font-semibold text-zinc-700 hover:bg-white active:bg-white dark:border-white/10 dark:bg-[#0f1116] dark:text-white/70 dark:hover:bg-white/10 dark:active:bg-white/15"
                >
                    钱包
                </button>
                <button
                    onClick={openPool}
                    disabled={!poolLink}
                    className="rounded-xl border border-zinc-200 bg-white/70 py-2 text-xs font-semibold text-zinc-700 hover:bg-white active:bg-white disabled:opacity-40 dark:border-white/10 dark:bg-[#0f1116] dark:text-white/70 dark:hover:bg-white/10 dark:active:bg-white/15"
                >
                    池子
                </button>
                <button
                    onClick={() => openToken(token0?.address)}
                    disabled={!token0?.address}
                    className="rounded-xl border border-zinc-200 bg-white/70 py-2 text-xs font-semibold text-zinc-700 hover:bg-white active:bg-white disabled:opacity-40 dark:border-white/10 dark:bg-[#0f1116] dark:text-white/70 dark:hover:bg-white/10 dark:active:bg-white/15 truncate px-1"
                    title={token0?.symbol || 'Token0'}
                >
                    {token0?.symbol || 'Token0'}
                </button>
                <button
                    onClick={() => openToken(token1?.address)}
                    disabled={!token1?.address}
                    className="rounded-xl border border-zinc-200 bg-white/70 py-2 text-xs font-semibold text-zinc-700 hover:bg-white active:bg-white disabled:opacity-40 dark:border-white/10 dark:bg-[#0f1116] dark:text-white/70 dark:hover:bg-white/10 dark:active:bg-white/15 truncate px-1"
                    title={token1?.symbol || 'Token1'}
                >
                    {token1?.symbol || 'Token1'}
                </button>
            </div>

            <PriceRangeVisualizer
                currentPrice={currentPrice}
                minPrice={rangeMin}
                maxPrice={rangeMax}
                token0={token0}
                token1={token1}
                tickLower={position?.tick_lower}
                tickUpper={position?.tick_upper}
                tickSpacing={position?.pool?.tickSpacing || position?.tick_spacing}
                inRange={position?.in_range}
                // Stats props
                pollIntervalSec={pollIntervalSec}
                runningDuration={runningDuration}
                updateTimeText={updateTimeText}
            />
        </div>
    );
}
