import React, { Suspense, lazy, startTransition, useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { AlertTriangle, ArrowRightLeft, ChevronLeft, ChevronRight, Crown, Medal, RefreshCw, Search, Settings2, Shield, TrendingUp, Trophy, Wallet } from 'lucide-react';
import { createChart, AreaSeries, HistogramSeries, ColorType } from 'lightweight-charts';
import {
    fetchAdminSmartMoneyLeaderboard,
    fetchAdminSmartMoneyOverview,
    fetchAdminSmartMoneyWallet,
    fetchAssetHistory,
    fetchAssetLPStats,
    fetchAssetOverview,
} from '../lib/api';
import { getBrandTheme } from '../lib/brand';
import MiniChart from './MiniChart.jsx';
import NumberFlowValue from './NumberFlowValue.jsx';

const LazyAdminPage = lazy(() => import('./AdminPage.jsx'));

const AVATAR_URLS = Object.entries(
    import.meta.glob('../icon/avatar_*.png', { eager: true, import: 'default' })
).sort(([a], [b]) => a.localeCompare(b, undefined, { numeric: true })).map(([, src]) => src);

const HISTORY_WINDOWS = [7, 30, 90];
const SMART_MONEY_WINDOWS = [1, 7, 30];
const LEADERBOARD_METRICS = [
    { key: 'pnl', label: '收益额' },
    { key: 'yield_rate', label: '收益率' },
    { key: 'participation', label: '参与次数' },
];

const usdFmt = new Intl.NumberFormat('en-US', { style: 'currency', currency: 'USD', maximumFractionDigits: 2 });
function formatUsd(value) {
    const num = Number(value || 0);
    if (!Number.isFinite(num)) return '$--';
    return usdFmt.format(num);
}

function formatUsdCompact(value) {
    const num = Number(value || 0);
    if (!Number.isFinite(num)) return '$--';
    const abs = Math.abs(num);
    if (abs >= 1000000) return `$${(num / 1000000).toFixed(abs >= 10000000 ? 0 : 1).replace(/\.0$/, '')}M`;
    if (abs >= 1000) return `$${(num / 1000).toFixed(abs >= 10000 ? 0 : 1).replace(/\.0$/, '')}K`;
    if (abs >= 100) return `$${num.toFixed(0)}`;
    if (abs >= 10) return `$${num.toFixed(1).replace(/\.0$/, '')}`;
    return `$${num.toFixed(2).replace(/0+$/, '').replace(/\.$/, '')}`;
}

function formatPct(value, digits = 2) {
    const num = Number(value || 0);
    if (!Number.isFinite(num)) return '--';
    return `${(num * 100).toFixed(digits).replace(/\.?0+$/, '')}%`;
}

function formatChain(chainId) {
    return Number(chainId) === 8453 ? 'Base' : 'BSC';
}

function walletKey(wallet) {
    return `${Number(wallet?.chain_id || 0)}:${String(wallet?.address || '').toLowerCase()}`;
}

function walletLabel(wallet) {
    const label = String(wallet?.label || '').trim();
    if (label) return label;
    const address = String(wallet?.address || '').trim();
    return address ? `${address.slice(0, 6)}...${address.slice(-4)}` : '--';
}

function errorText(err) {
    return String(err?.message || err || '').trim();
}

function hasTransferMarker(item) {
    return Boolean(item?.has_transfer_in || item?.has_transfer_out || Number(item?.transfer_in_count || 0) > 0 || Number(item?.transfer_out_count || 0) > 0);
}

function TransferBadges({ item, compact = false }) {
    const inCount = Number(item?.transfer_in_count || 0);
    const outCount = Number(item?.transfer_out_count || 0);
    const badges = [];
    if (item?.has_transfer_in || inCount > 0) {
        badges.push({
            key: 'in',
            label: compact ? '入' : `转入${inCount > 0 ? ` ${inCount}` : ''}`,
            className: 'border-emerald-500/25 bg-emerald-500/[0.10] text-emerald-700 dark:text-emerald-300',
        });
    }
    if (item?.has_transfer_out || outCount > 0) {
        badges.push({
            key: 'out',
            label: compact ? '出' : `转出${outCount > 0 ? ` ${outCount}` : ''}`,
            className: 'border-amber-500/25 bg-amber-500/[0.10] text-amber-700 dark:text-amber-300',
        });
    }
    if (!badges.length) return null;
    return (
        <div className={`flex flex-wrap gap-1 ${compact ? 'justify-center min-h-[12px]' : ''}`.trim()}>
            {badges.map((badge) => (
                <span
                    key={badge.key}
                    className={`inline-flex items-center justify-center rounded-full border px-1.5 py-0.5 font-bold leading-none ${badge.className}`}
                    style={{ fontSize: compact ? 9 : 10 }}
                >
                    {badge.label}
                </span>
            ))}
        </div>
    );
}

function isIgnorableSmartMoneyDataError(err) {
    const message = errorText(err).toLowerCase();
    return message.includes("unknown column 'open_lp_usd'") || message.includes("unknown column `open_lp_usd`");
}

function seriesRows(history, metric) {
    const rows = Array.isArray(history?.history) ? [...history.history] : [];
    if (history?.today?.day) rows.push(history.today);
    return rows.map((item) => ({ day: item.day, value: Number(item?.[metric] || 0), close: Number(item?.[metric] || 0) }));
}

/* ─── Pill toggle (matches PositionCard / HotPoolCard style) ─── */
function Pill({ active, brand, onClick, children }) {
    return (
        <button
            type="button"
            onClick={onClick}
            className={`inline-flex items-center rounded-full px-2.5 py-1 text-[10px] font-semibold ring-1 transition active:scale-95 ${
                active
                    ? `${brand.softButtonClass}`
                    : 'bg-zinc-100 text-zinc-500 ring-zinc-200 hover:bg-zinc-50 dark:bg-white/[0.04] dark:text-white/50 dark:ring-white/[0.06] dark:hover:bg-white/[0.07]'
            }`}
        >
            {children}
        </button>
    );
}

/* ─── Search input for smart money ─── */
function SmSearchInput({ value, onChange, placeholder }) {
    return (
        <div className="relative">
            <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-zinc-400 dark:text-white/30" />
            <input
                type="text"
                value={value}
                onChange={(e) => onChange(e.target.value)}
                placeholder={placeholder}
                className="w-full rounded-xl border border-zinc-200 bg-zinc-50/80 py-2 pl-8 pr-3 text-[11px] text-zinc-700 placeholder-zinc-400 outline-none ring-0 transition focus:border-zinc-300 focus:ring-1 focus:ring-zinc-300 dark:border-white/[0.06] dark:bg-white/[0.03] dark:text-white/80 dark:placeholder-white/25 dark:focus:border-white/10 dark:focus:ring-white/10"
            />
        </div>
    );
}

/* ─── Pagination for smart money ─── */
function SmPagination({ page, totalPages, onPageChange }) {
    if (totalPages <= 1) return null;
    return (
        <div className="flex items-center justify-center gap-3 pt-2">
            <button type="button" disabled={page <= 0} onClick={() => onPageChange(page - 1)} className="inline-flex items-center rounded-lg px-2 py-1 text-[10px] font-medium text-zinc-500 ring-1 ring-zinc-200 transition enabled:hover:bg-zinc-100 enabled:active:scale-95 disabled:opacity-30 dark:text-white/50 dark:ring-white/[0.06] dark:enabled:hover:bg-white/[0.06]">上一页</button>
            <span className="text-[10px] tabular-nums text-zinc-400 dark:text-white/35">{page + 1} / {totalPages}</span>
            <button type="button" disabled={page >= totalPages - 1} onClick={() => onPageChange(page + 1)} className="inline-flex items-center rounded-lg px-2 py-1 text-[10px] font-medium text-zinc-500 ring-1 ring-zinc-200 transition enabled:hover:bg-zinc-100 enabled:active:scale-95 disabled:opacity-30 dark:text-white/50 dark:ring-white/[0.06] dark:enabled:hover:bg-white/[0.06]">下一页</button>
        </div>
    );
}

/* ─── Card wrapper ─── */
function Card({ children, className = '' }) {
    return (
        <div className={`rounded-2xl border border-zinc-200/80 bg-white p-3 dark:border-white/5 dark:bg-[#131518] ${className}`.trim()}>
            {children}
        </div>
    );
}

/* ─── Metric stat (compact, like PositionCard value blocks) ─── */
function StatBlock({ label, value, sub, tone = 'default' }) {
    const toneRing = tone === 'accent'
        ? 'ring-emerald-500/20 dark:ring-emerald-400/25'
        : tone === 'warn'
            ? 'ring-amber-500/20 dark:ring-amber-400/25'
            : 'ring-zinc-200 dark:ring-white/[0.06]';
    const toneBg = tone === 'accent'
        ? 'bg-emerald-500/[0.06] dark:bg-emerald-500/[0.08]'
        : tone === 'warn'
            ? 'bg-amber-500/[0.06] dark:bg-amber-500/[0.08]'
            : 'bg-zinc-50 dark:bg-white/[0.03]';
    return (
        <div className={`rounded-xl ${toneBg} ring-1 ${toneRing} px-3 py-2.5`}>
            <div className="text-[9px] font-medium uppercase tracking-wide text-zinc-400 dark:text-white/35">{label}</div>
            <div className="mt-1 text-base font-extrabold tabular-nums text-zinc-900 dark:text-white/95 leading-none">
                <NumberFlowValue value={value} formatter={() => value} />
            </div>
            {sub ? <div className="mt-1.5 text-[10px] text-zinc-500 dark:text-white/40">{sub}</div> : null}
        </div>
    );
}

/* ─── Empty state ─── */
function Empty({ text }) {
    return (
        <div className="flex items-center justify-center rounded-xl border border-dashed border-zinc-200 bg-zinc-50/50 px-4 py-6 text-[11px] text-zinc-400 dark:border-white/[0.06] dark:bg-white/[0.02] dark:text-white/30">
            {text}
        </div>
    );
}

/* ─── Wallet Avatar (address-based icon image) ─── */
function walletAvatarUrl(address) {
    if (!AVATAR_URLS.length) return '';
    const hex = String(address || '').toLowerCase();
    let hash = 0;
    for (let i = 0; i < hex.length; i++) hash = ((hash << 5) - hash + hex.charCodeAt(i)) | 0;
    return AVATAR_URLS[Math.abs(hash) % AVATAR_URLS.length] || AVATAR_URLS[0] || '';
}

function WalletAvatar({ address, size = 28, className = '' }) {
    const src = useMemo(() => walletAvatarUrl(address), [address]);
    if (!src) return null;
    return (
        <img
            src={src}
            alt=""
            width={size}
            height={size}
            className={`shrink-0 rounded-lg object-cover ${className}`.trim()}
            style={{ width: size, height: size }}
        />
    );
}

/* ─── Rank badge for leaderboard ─── */
function RankBadge({ rank }) {
    if (rank === 1) {
        return (
            <span className="inline-flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-gradient-to-br from-yellow-400 to-amber-500 shadow-sm shadow-amber-500/30">
                <Trophy className="h-3.5 w-3.5 text-white" />
            </span>
        );
    }
    if (rank === 2) {
        return (
            <span className="inline-flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-gradient-to-br from-slate-300 to-slate-400 shadow-sm shadow-slate-400/30">
                <Medal className="h-3.5 w-3.5 text-white" />
            </span>
        );
    }
    if (rank === 3) {
        return (
            <span className="inline-flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-gradient-to-br from-amber-600 to-amber-700 shadow-sm shadow-amber-700/30">
                <Medal className="h-3.5 w-3.5 text-white" />
            </span>
        );
    }
    return (
        <span className="inline-flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-zinc-100 text-[11px] font-bold tabular-nums text-zinc-500 dark:bg-white/[0.06] dark:text-white/50">
            {rank}
        </span>
    );
}

/* ─── TradingView-style Area Chart (lightweight-charts v5) ─── */
function LWAreaChart({ data, color = '#10b981', loading = false }) {
    const containerRef = useRef(null);
    const chartRef = useRef(null);
    const seriesRef = useRef(null);

    useEffect(() => {
        if (!containerRef.current) return;
        const isDark = document.documentElement.classList.contains('dark') || window.matchMedia?.('(prefers-color-scheme: dark)')?.matches;
        const chart = createChart(containerRef.current, {
            width: containerRef.current.clientWidth,
            height: 200,
            layout: {
                background: { type: ColorType.Solid, color: 'transparent' },
                textColor: isDark ? 'rgba(255,255,255,0.35)' : 'rgba(0,0,0,0.35)',
                fontFamily: "system-ui, -apple-system, 'Segoe UI', sans-serif",
                fontSize: 10,
            },
            grid: {
                vertLines: { color: isDark ? 'rgba(255,255,255,0.04)' : 'rgba(0,0,0,0.04)' },
                horzLines: { color: isDark ? 'rgba(255,255,255,0.04)' : 'rgba(0,0,0,0.04)' },
            },
            rightPriceScale: {
                borderVisible: false,
                scaleMargins: { top: 0.1, bottom: 0.05 },
            },
            timeScale: {
                borderVisible: false,
                fixLeftEdge: true,
                fixRightEdge: true,
                timeVisible: false,
            },
            crosshair: {
                horzLine: { color: isDark ? 'rgba(255,255,255,0.15)' : 'rgba(0,0,0,0.15)', style: 2 },
                vertLine: { color: isDark ? 'rgba(255,255,255,0.15)' : 'rgba(0,0,0,0.15)', style: 2 },
            },
            handleScroll: false,
            handleScale: false,
        });
        const series = chart.addSeries(AreaSeries, {
            lineColor: color,
            lineWidth: 2,
            topColor: `${color}40`,
            bottomColor: `${color}05`,
            priceFormat: { type: 'custom', formatter: (v) => {
                const abs = Math.abs(v);
                if (abs >= 1000000) return `$${(v / 1000000).toFixed(1)}M`;
                if (abs >= 1000) return `$${(v / 1000).toFixed(abs >= 10000 ? 0 : 1)}K`;
                return `$${v.toFixed(0)}`;
            }},
            crosshairMarkerRadius: 4,
            crosshairMarkerBorderColor: color,
            crosshairMarkerBackgroundColor: isDark ? '#131518' : '#ffffff',
        });
        chartRef.current = chart;
        seriesRef.current = series;

        const ro = new ResizeObserver((entries) => {
            for (const entry of entries) chart.applyOptions({ width: entry.contentRect.width });
        });
        ro.observe(containerRef.current);

        return () => {
            ro.disconnect();
            chart.remove();
            chartRef.current = null;
            seriesRef.current = null;
        };
    }, [color]);

    useEffect(() => {
        if (!seriesRef.current || !data || data.length < 1) return;
        const mapped = data
            .filter((d) => d.day && Number.isFinite(d.value))
            .map((d) => ({ time: d.day, value: d.value }));
        seriesRef.current.setData(mapped);
        chartRef.current?.timeScale().fitContent();
    }, [data]);

    if (loading) {
        return <div className="animate-pulse rounded-lg bg-zinc-200 dark:bg-zinc-700" style={{ height: 200 }} />;
    }

    return <div ref={containerRef} className="w-full rounded-lg overflow-hidden" style={{ minHeight: 200 }} />;
}

/* ─── PnL Calendar (盈亏日历) ─── */
const PNL_CAL_WEEKDAYS = ['一', '二', '三', '四', '五', '六', '日'];
function PnLCalendar({ data, loading = false, note = '' }) {
    const [viewDate, setViewDate] = useState(() => new Date());
    const year = viewDate.getFullYear();
    const month = viewDate.getMonth();
    const daysInMonth = new Date(year, month + 1, 0).getDate();
    const firstDayJS = new Date(year, month, 1).getDay();
    const startOffset = firstDayJS === 0 ? 6 : firstDayJS - 1;

    const pnlMap = useMemo(() => {
        const map = {};
        if (Array.isArray(data)) data.forEach((d) => { if (d.day) map[d.day] = d; });
        return map;
    }, [data]);

    const monthLabel = new Date(year, month).toLocaleDateString('en-US', { year: 'numeric', month: 'short' });
    const prevMonth = () => setViewDate(new Date(year, month - 1, 1));
    const nextMonth = () => setViewDate(new Date(year, month + 1, 1));
    const now = new Date();
    const todayStr = `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}-${String(now.getDate()).padStart(2, '0')}`;

    if (loading) {
        return <div className="animate-pulse rounded-lg bg-zinc-200 dark:bg-zinc-700" style={{ height: 200 }} />;
    }

    const cells = [];
    for (let i = 0; i < startOffset; i++) {
        cells.push(<div key={`e-${i}`} className="rounded-md bg-zinc-100/50 dark:bg-white/[0.02]" style={{ minHeight: 32 }} />);
    }
    for (let day = 1; day <= daysInMonth; day++) {
        const dateStr = `${year}-${String(month + 1).padStart(2, '0')}-${String(day).padStart(2, '0')}`;
        const entry = pnlMap[dateStr];
        const pnl = entry ? Number(entry.realized_pnl_usd || 0) : null;
        const hasTransfer = hasTransferMarker(entry);
        const isToday = dateStr === todayStr;
        const isFuture = new Date(year, month, day) > now;
        const dayToneClass = isToday
            ? 'text-emerald-700 dark:text-emerald-300'
            : isFuture
                ? 'text-zinc-300 dark:text-white/15'
                : 'text-zinc-400 dark:text-white/30';
        const valueToneClass = pnl !== null
            ? pnl >= 0
                ? 'text-emerald-600 dark:text-emerald-400'
                : 'text-red-500 dark:text-red-400'
            : 'text-transparent';
        cells.push(
            <div
                key={day}
                className={`rounded-md px-1 py-1 ${
                    isToday ? 'bg-emerald-500/15 ring-1 ring-emerald-500/30'
                    : isFuture ? 'bg-zinc-100/30 dark:bg-white/[0.015]'
                    : 'bg-zinc-100/50 dark:bg-white/[0.03]'
                }`}
                style={{ minHeight: 38 }}
            >
                <div className={`text-[9px] leading-none ${dayToneClass}`}>{day}</div>
                <div className={`flex min-h-[20px] items-center justify-center px-0.5 text-center text-[10px] font-semibold leading-tight tabular-nums ${valueToneClass}`}>
                    {pnl !== null ? `${pnl >= 0 ? '+' : ''}${formatUsdCompact(pnl)}` : '0'}
                </div>
                {hasTransfer ? <TransferBadges item={entry} compact /> : <div className="min-h-[12px]" />}
            </div>
        );
    }
    const remainder = (startOffset + daysInMonth) % 7;
    if (remainder > 0) {
        for (let i = 0; i < 7 - remainder; i++) {
            cells.push(<div key={`t-${i}`} className="rounded-md bg-zinc-100/30 dark:bg-white/[0.015]" style={{ minHeight: 32 }} />);
        }
    }

    return (
        <div>
            <div className="flex items-center justify-between mb-2">
                <div className="flex items-center gap-1.5">
                    <span className="text-[13px] font-bold text-zinc-900 dark:text-white/90">{monthLabel}</span>
                    <svg className="w-3.5 h-3.5 text-zinc-400 dark:text-white/30" fill="none" stroke="currentColor" strokeWidth="2" viewBox="0 0 24 24">
                        <rect x="3" y="4" width="18" height="18" rx="2" /><line x1="16" y1="2" x2="16" y2="6" /><line x1="8" y1="2" x2="8" y2="6" /><line x1="3" y1="10" x2="21" y2="10" />
                    </svg>
                </div>
                <div className="flex items-center gap-0.5">
                    <button onClick={prevMonth} className="p-1 rounded-md hover:bg-zinc-200 dark:hover:bg-white/10 text-zinc-500 dark:text-white/40"><ChevronLeft size={14} /></button>
                    <button onClick={nextMonth} className="p-1 rounded-md hover:bg-zinc-200 dark:hover:bg-white/10 text-zinc-500 dark:text-white/40"><ChevronRight size={14} /></button>
                </div>
            </div>
            <div className="grid grid-cols-7 gap-1">
                {PNL_CAL_WEEKDAYS.map((d) => (
                    <div key={d} className="text-center text-[9px] text-zinc-400 dark:text-white/20 pb-0.5">{d}</div>
                ))}
                {cells}
            </div>
            {note ? (
                <div className="mt-2 flex items-start gap-1.5 text-[10px] leading-[1.45] text-zinc-500 dark:text-white/40">
                    <ArrowRightLeft className="mt-0.5 h-3.5 w-3.5 shrink-0" />
                    <span>{note}</span>
                </div>
            ) : null}
        </div>
    );
}

/* ─── Per-pool PnL list for today ─── */
function formatPoolPair(pool) {
    const token0 = String(pool?.token0_symbol || '').trim();
    const token1 = String(pool?.token1_symbol || '').trim();
    if (token0 || token1) return `${token0 || '?'} / ${token1 || '?'}`;
    const poolId = String(pool?.pool_id || '').trim();
    return poolId ? `${poolId.slice(0, 6)}...${poolId.slice(-4)}` : '未命名池子';
}

function summarizeTodayPools(pools) {
    if (!Array.isArray(pools) || pools.length === 0) {
        return {
            rows: [],
            positiveRows: [],
            negativeRows: [],
            flatCount: 0,
            topPositive: [],
            topNegative: [],
            remainingCount: 0,
            maxAbsPnl: 1,
        };
    }

    const merged = new Map();
    for (const pool of pools) {
        const chain = String(pool?.chain || 'bsc').toUpperCase();
        const poolId = String(pool?.pool_id || '').trim();
        const pair = formatPoolPair(pool);
        const key = poolId ? `${chain}:${poolId}` : `${chain}:${pair}`;
        const prev = merged.get(key) || {
            key,
            chain,
            pair,
            closed_count: 0,
            profit_usd: 0,
        };
        prev.closed_count += Number(pool?.closed_count || 0);
        prev.profit_usd += Number(pool?.profit_usd || 0);
        merged.set(key, prev);
    }

    const rows = [...merged.values()].sort((a, b) => Math.abs(b.profit_usd) - Math.abs(a.profit_usd));
    const positiveRows = rows.filter((row) => row.profit_usd > 0).sort((a, b) => b.profit_usd - a.profit_usd);
    const negativeRows = rows.filter((row) => row.profit_usd < 0).sort((a, b) => a.profit_usd - b.profit_usd);
    const topPositive = positiveRows.slice(0, 3);
    const topNegative = negativeRows.slice(0, 3);
    const featuredKeys = new Set([...topPositive, ...topNegative].map((row) => row.key));

    return {
        rows,
        positiveRows,
        negativeRows,
        flatCount: rows.length - positiveRows.length - negativeRows.length,
        topPositive,
        topNegative,
        remainingCount: rows.filter((row) => !featuredKeys.has(row.key)).length,
        maxAbsPnl: Math.max(...rows.map((row) => Math.abs(row.profit_usd)), 1),
    };
}

function TodayPoolContributionRow({ row, maxAbsPnl }) {
    const pnl = Number(row?.profit_usd || 0);
    const positive = pnl >= 0;
    const ratio = Math.max(Math.abs(pnl) / Math.max(maxAbsPnl, 1), 0.08);

    return (
        <div className="rounded-xl border border-zinc-100 bg-zinc-50/60 px-3 py-2.5 dark:border-white/[0.04] dark:bg-[#0d0f12]">
            <div className="flex items-center justify-between gap-2">
                <div className="min-w-0 truncate text-[11px] font-semibold text-zinc-900 dark:text-white/90">
                    {row?.pair || '未命名池子'}
                </div>
                <span className={`inline-flex items-center rounded-full px-2 py-0.5 text-[10px] font-bold tabular-nums ring-1 ${
                    positive
                        ? 'bg-emerald-500/[0.08] text-emerald-600 ring-emerald-500/20 dark:bg-emerald-500/[0.12] dark:text-emerald-300 dark:ring-emerald-400/25'
                        : 'bg-red-500/[0.08] text-red-600 ring-red-500/20 dark:bg-red-500/[0.12] dark:text-red-300 dark:ring-red-400/25'
                }`}>
                    {positive ? '+' : ''}{formatUsdCompact(pnl)}
                </span>
            </div>
            <div className="mt-1 flex flex-wrap gap-x-2 gap-y-1 text-[9px] text-zinc-400 dark:text-white/30">
                <span>{String(row?.chain || 'BSC').toUpperCase()}</span>
                <span>{Number(row?.closed_count || 0)} 笔</span>
            </div>
            <div className="mt-2 h-1.5 overflow-hidden rounded-full bg-zinc-100 dark:bg-white/[0.04]">
                <div
                    className={positive ? 'h-full rounded-full bg-emerald-400/90' : 'h-full rounded-full bg-red-400/90'}
                    style={{ width: `${ratio * 100}%` }}
                />
            </div>
        </div>
    );
}

function TodayPoolPnL({ pools, brand }) {
    const [view, setView] = useState('leaders');
    const summary = useMemo(() => summarizeTodayPools(pools), [pools]);
    const showDetailsTab = summary.remainingCount > 0 || summary.flatCount > 0;

    useEffect(() => {
        if (!showDetailsTab && view !== 'leaders') {
            setView('leaders');
        }
    }, [showDetailsTab, view]);

    if (!summary.rows.length) {
        return <Empty text="今日暂无平仓记录" />;
    }

    return (
        <div className="flex flex-col gap-2.5">
            <div className="flex items-start justify-between gap-2">
                <div>
                    <div className="text-[10px] font-medium text-zinc-500 dark:text-white/40">池子贡献</div>
                    <div className="mt-0.5 text-[10px] text-zinc-400 dark:text-white/30">默认只看贡献榜，完整明细收进二级视图</div>
                </div>
                <div className="flex gap-1.5">
                    <Pill active={view === 'leaders'} brand={brand} onClick={() => setView('leaders')}>贡献榜</Pill>
                    {showDetailsTab ? <Pill active={view === 'details'} brand={brand} onClick={() => setView('details')}>全部明细</Pill> : null}
                </div>
            </div>

            <div className="grid grid-cols-2 gap-1.5">
                {[
                    { label: '参与池子', value: summary.rows.length, tone: '' },
                    { label: '盈利池', value: summary.positiveRows.length, tone: 'text-emerald-600 dark:text-emerald-300' },
                    { label: '亏损池', value: summary.negativeRows.length, tone: 'text-red-600 dark:text-red-300' },
                    { label: '持平池', value: summary.flatCount, tone: '' },
                ].map((item) => (
                    <div key={item.label} className="rounded-lg bg-zinc-50 px-2.5 py-2 ring-1 ring-zinc-200 dark:bg-white/[0.03] dark:ring-white/[0.06]">
                        <div className="text-[9px] text-zinc-400 dark:text-white/30">{item.label}</div>
                        <div className={`mt-0.5 text-[13px] font-bold tabular-nums text-zinc-900 dark:text-white/90 ${item.tone}`.trim()}>
                            {item.value}
                        </div>
                    </div>
                ))}
            </div>

            {view === 'leaders' ? (
                <div className="flex flex-col gap-2.5">
                    <div className="flex flex-col gap-1.5">
                        <div className="flex items-center justify-between gap-2">
                            <span className="text-[10px] font-semibold text-zinc-700 dark:text-white/70">Top 盈利</span>
                            <span className="text-[9px] text-zinc-400 dark:text-white/30">{summary.topPositive.length} 个</span>
                        </div>
                        {summary.topPositive.length > 0
                            ? summary.topPositive.map((row) => <TodayPoolContributionRow key={row.key} row={row} maxAbsPnl={summary.maxAbsPnl} />)
                            : <Empty text="今日暂无盈利池" />}
                    </div>

                    <div className="flex flex-col gap-1.5">
                        <div className="flex items-center justify-between gap-2">
                            <span className="text-[10px] font-semibold text-zinc-700 dark:text-white/70">Top 亏损</span>
                            <span className="text-[9px] text-zinc-400 dark:text-white/30">{summary.topNegative.length} 个</span>
                        </div>
                        {summary.topNegative.length > 0
                            ? summary.topNegative.map((row) => <TodayPoolContributionRow key={row.key} row={row} maxAbsPnl={summary.maxAbsPnl} />)
                            : <Empty text="今日暂无亏损池" />}
                    </div>

                    {showDetailsTab ? (
                        <div className="text-[10px] text-zinc-400 dark:text-white/30">
                            其余 {summary.remainingCount} 个池子已折叠，点“全部明细”查看完整列表。
                        </div>
                    ) : null}
                </div>
            ) : (
                <div className="flex max-h-[280px] flex-col gap-1.5 overflow-y-auto pr-1">
                    {summary.rows.map((row) => (
                        <TodayPoolContributionRow key={row.key} row={row} maxAbsPnl={summary.maxAbsPnl} />
                    ))}
                </div>
            )}
        </div>
    );
}

/* ─── Donut chart for wallet distribution ─── */
const WALLET_COLORS = ['#10b981', '#0ea5e9', '#8b5cf6', '#f59e0b', '#ec4899', '#06b6d4', '#f97316', '#84cc16'];

function DonutChart({ wallets }) {
    const items = useMemo(() => {
        if (!Array.isArray(wallets) || wallets.length === 0) return [];
        // Merge wallets by address across chains
        const byAddr = new Map();
        for (const w of wallets) {
            const addr = String(w.wallet_address || '').toLowerCase();
            const key = addr || `id-${w.wallet_id}`;
            if (byAddr.has(key)) {
                const prev = byAddr.get(key);
                prev.value += Math.max(0, Number(w.total_usd || 0));
                prev.native += Number(w.native_usd || 0);
                prev.stable += Number(w.stable_usd || 0);
                prev.token += Number(w.token_usd || 0);
                if (w.chain && !prev.chains.includes(String(w.chain).toUpperCase())) {
                    prev.chains.push(String(w.chain).toUpperCase());
                }
            } else {
                byAddr.set(key, {
                    label: addr ? `${addr.slice(0, 6)}...${addr.slice(-4)}` : `钱包 #${w.wallet_id}`,
                    chains: w.chain ? [String(w.chain).toUpperCase()] : [],
                    value: Math.max(0, Number(w.total_usd || 0)),
                    native: Number(w.native_usd || 0),
                    stable: Number(w.stable_usd || 0),
                    token: Number(w.token_usd || 0),
                });
            }
        }
        return [...byAddr.values()]
            .filter((item) => item.value > 0)
            .map((item, i) => ({ ...item, color: WALLET_COLORS[i % WALLET_COLORS.length] }));
    }, [wallets]);

    const total = useMemo(() => items.reduce((s, item) => s + item.value, 0), [items]);

    if (items.length === 0) return null;

    // donut arcs
    const cx = 60, cy = 60, r = 48, innerR = 30;
    let startAngle = -Math.PI / 2;
    const arcs = items.map((item) => {
        const fraction = item.value / total;
        const angle = fraction * Math.PI * 2;
        const endAngle = startAngle + angle;
        const largeArc = angle > Math.PI ? 1 : 0;
        const gap = items.length > 1 ? 0.02 : 0;
        const s = startAngle + gap;
        const e = endAngle - gap;
        const x1o = cx + r * Math.cos(s), y1o = cy + r * Math.sin(s);
        const x2o = cx + r * Math.cos(e), y2o = cy + r * Math.sin(e);
        const x1i = cx + innerR * Math.cos(e), y1i = cy + innerR * Math.sin(e);
        const x2i = cx + innerR * Math.cos(s), y2i = cy + innerR * Math.sin(s);
        const d = `M${x1o},${y1o} A${r},${r} 0 ${largeArc} 1 ${x2o},${y2o} L${x1i},${y1i} A${innerR},${innerR} 0 ${largeArc} 0 ${x2i},${y2i} Z`;
        startAngle = endAngle;
        return { ...item, d, pct: (fraction * 100).toFixed(1) };
    });

    return (
        <div className="flex items-start gap-4">
            <svg width="120" height="120" viewBox="0 0 120 120" className="shrink-0">
                {arcs.map((arc, i) => (
                    <path key={i} d={arc.d} fill={arc.color} fillOpacity="0.85" />
                ))}
                <text x={cx} y={cy - 2} textAnchor="middle" fill="currentColor" fillOpacity="0.4" fontSize="8" fontWeight="500">总计</text>
                <text x={cx} y={cy + 10} textAnchor="middle" fill="currentColor" fillOpacity="0.9" fontSize="11" fontWeight="700">{formatUsdCompact(total)}</text>
            </svg>
            <div className="flex flex-1 flex-col gap-1.5 min-w-0 pt-1">
                {arcs.map((arc, i) => (
                    <div key={i} className="flex items-center gap-2 min-w-0">
                        <span className="h-2 w-2 shrink-0 rounded-full" style={{ backgroundColor: arc.color }} />
                        <span className="truncate text-[10px] text-zinc-600 dark:text-white/60">{arc.label}</span>
                        <span className="ml-auto shrink-0 text-[10px] font-bold tabular-nums text-zinc-800 dark:text-white/80">{formatUsdCompact(arc.value)}</span>
                        <span className="shrink-0 text-[9px] text-zinc-400 dark:text-white/30">{arc.pct}%</span>
                    </div>
                ))}
            </div>
        </div>
    );
}

/* ─── Horizontal stacked bar for wallet asset breakdown ─── */
function WalletStackedBar({ wallets }) {
    const items = useMemo(() => {
        if (!Array.isArray(wallets)) return [];
        // Merge wallets by address across chains
        const byAddr = new Map();
        for (const w of wallets) {
            const addr = String(w.wallet_address || '').toLowerCase();
            const key = addr || `id-${w.wallet_id}`;
            if (byAddr.has(key)) {
                const prev = byAddr.get(key);
                prev.native += Math.max(0, Number(w.native_usd || 0));
                prev.stable += Math.max(0, Number(w.stable_usd || 0));
                prev.token += Math.max(0, Number(w.token_usd || 0));
                prev.total += Math.max(0, Number(w.total_usd || 0));
                if (w.chain && !prev.chains.includes(String(w.chain).toUpperCase())) {
                    prev.chains.push(String(w.chain).toUpperCase());
                }
            } else {
                byAddr.set(key, {
                    label: addr ? `${addr.slice(0, 6)}...${addr.slice(-4)}` : `#${w.wallet_id}`,
                    chains: w.chain ? [String(w.chain).toUpperCase()] : [],
                    native: Math.max(0, Number(w.native_usd || 0)),
                    stable: Math.max(0, Number(w.stable_usd || 0)),
                    token: Math.max(0, Number(w.token_usd || 0)),
                    total: Math.max(0, Number(w.total_usd || 0)),
                });
            }
        }
        return [...byAddr.values()];
    }, [wallets]);

    const maxVal = useMemo(() => Math.max(...items.map((i) => i.total), 1), [items]);

    if (items.length === 0) return null;

    const COLORS = { native: '#0ea5e9', stable: '#10b981', token: '#8b5cf6' };

    return (
        <div className="flex flex-col gap-2.5">
            {/* legend */}
            <div className="flex items-center gap-3">
                {[
                    { key: 'native', label: '原生币', color: COLORS.native },
                    { key: 'stable', label: '稳定币', color: COLORS.stable },
                    { key: 'token', label: '代币', color: COLORS.token },
                ].map((item) => (
                    <div key={item.key} className="flex items-center gap-1">
                        <span className="h-1.5 w-1.5 rounded-full" style={{ backgroundColor: item.color }} />
                        <span className="text-[9px] text-zinc-400 dark:text-white/35">{item.label}</span>
                    </div>
                ))}
            </div>
            {items.map((item, i) => {
                const barWidth = (item.total / maxVal) * 100;
                const nPct = item.total > 0 ? (item.native / item.total) * 100 : 0;
                const sPct = item.total > 0 ? (item.stable / item.total) * 100 : 0;
                const tPct = item.total > 0 ? (item.token / item.total) * 100 : 0;
                return (
                    <div key={i}>
                        <div className="flex items-center justify-between gap-2 mb-1">
                            <div className="flex items-center gap-1.5 min-w-0">
                                <span className="text-[10px] font-semibold text-zinc-800 dark:text-white/80 truncate">{item.label}</span>
                                <span className="text-[9px] text-zinc-400 dark:text-white/30">{item.chains?.join(' / ') || ''}</span>
                            </div>
                            <span className="text-[10px] font-bold tabular-nums text-zinc-900 dark:text-white/90 shrink-0">{formatUsdCompact(item.total)}</span>
                        </div>
                        <div className="h-3 rounded-full overflow-hidden bg-zinc-100 dark:bg-white/[0.04]" style={{ width: `${Math.max(barWidth, 8)}%` }}>
                            <div className="flex h-full">
                                {item.native > 0 && <div className="h-full rounded-l-full" style={{ width: `${nPct}%`, backgroundColor: COLORS.native }} />}
                                {item.stable > 0 && <div className="h-full" style={{ width: `${sPct}%`, backgroundColor: COLORS.stable }} />}
                                {item.token > 0 && <div className="h-full rounded-r-full" style={{ width: `${tPct}%`, backgroundColor: COLORS.token }} />}
                            </div>
                        </div>
                    </div>
                );
            })}
        </div>
    );
}

export default function AssetManagementPage({
    apiBaseUrl,
    initData,
    hasInitData,
    isAdmin = false,
    tick,
    pollIntervalSec = 15,
    accentTheme = 'lime',
    onNotice,
}) {
    const brand = useMemo(() => getBrandTheme(accentTheme), [accentTheme]);
    const tabs = useMemo(() => {
        const list = [{ key: 'my_assets', label: '我的资产', icon: Wallet }];
        if (isAdmin) {
            list.push(
                { key: 'smart_money_assets', label: '聪明钱', icon: TrendingUp },
                { key: 'operations', label: '运行管理', icon: Shield },
                { key: 'system', label: '系统', icon: Settings2 },
            );
        }
        return list;
    }, [isAdmin]);
    const [activeTab, setActiveTab] = useState('my_assets');

    const [historyDays, setHistoryDays] = useState(30);
    const [assetsData, setAssetsData] = useState({ overview: null, history: null, lp: null });
    const [assetsLoading, setAssetsLoading] = useState(false);
    const [assetsRefreshing, setAssetsRefreshing] = useState(false);
    const [assetsError, setAssetsError] = useState('');

    const [smartMoneyDays, setSmartMoneyDays] = useState(7);
    const [leaderboardMetric, setLeaderboardMetric] = useState('pnl');
    const [smartMoneyOverview, setSmartMoneyOverview] = useState(null);
    const [smartMoneyWallet, setSmartMoneyWallet] = useState(null);
    const [smartMoneyLeaderboard, setSmartMoneyLeaderboard] = useState(null);
    const [smartMoneyLoading, setSmartMoneyLoading] = useState(false);
    const [smartMoneyRefreshing, setSmartMoneyRefreshing] = useState(false);
    const [smartMoneyError, setSmartMoneyError] = useState('');
    const [selectedWalletId, setSelectedWalletId] = useState('');
    const [smSubTab, setSmSubTab] = useState('wallets');
    const [smWalletSearch, setSmWalletSearch] = useState('');
    const [smWalletPage, setSmWalletPage] = useState(0);
    const [smLeaderSearch, setSmLeaderSearch] = useState('');
    const [smLeaderPage, setSmLeaderPage] = useState(0);
    const [smDrillWalletId, setSmDrillWalletId] = useState('');

    useEffect(() => {
        if (!tabs.some((tab) => tab.key === activeTab)) {
            setActiveTab('my_assets');
        }
    }, [activeTab, tabs]);

    const hasAssetData = Boolean(assetsData.overview || assetsData.history || assetsData.lp);
    const hasSmartMoneyData = Boolean(smartMoneyOverview || smartMoneyLeaderboard || smartMoneyWallet);
    const hasAssetDataRef = useRef(false);
    const hasSmartMoneyDataRef = useRef(false);

    useEffect(() => {
        hasAssetDataRef.current = hasAssetData;
    }, [hasAssetData]);

    useEffect(() => {
        hasSmartMoneyDataRef.current = hasSmartMoneyData;
    }, [hasSmartMoneyData]);

    const loadAssets = useCallback(async ({ forceRefresh = false } = {}) => {
        if (!hasInitData) return;
        if (hasAssetDataRef.current) setAssetsRefreshing(true);
        else setAssetsLoading(true);
        setAssetsError('');
        try {
            const overviewPromise = fetchAssetOverview({ apiBaseUrl, initData, forceRefresh });
            const historyPromise = fetchAssetHistory({ apiBaseUrl, initData, days: historyDays, forceRefresh });
            const lpPromise = fetchAssetLPStats({ apiBaseUrl, initData, forceRefresh });

            overviewPromise
                .then((overview) => {
                    startTransition(() => {
                        setAssetsData((prev) => ({ ...prev, overview: overview || null }));
                    });
                })
                .catch(() => {});

            const [overviewResult, historyResult, lpResult] = await Promise.allSettled([
                overviewPromise,
                historyPromise,
                lpPromise,
            ]);

            const nextState = {};
            const errors = [];

            if (overviewResult.status === 'fulfilled') {
                nextState.overview = overviewResult.value || null;
            } else {
                errors.push(errorText(overviewResult.reason));
            }
            if (historyResult.status === 'fulfilled') {
                nextState.history = historyResult.value || null;
            } else {
                errors.push(errorText(historyResult.reason));
            }
            if (lpResult.status === 'fulfilled') {
                nextState.lp = lpResult.value || null;
            } else {
                errors.push(errorText(lpResult.reason));
            }

            startTransition(() => {
                setAssetsData((prev) => ({ ...prev, ...nextState }));
            });
            setAssetsError(errors.find(Boolean) || '');
        } catch (err) {
            setAssetsError(String(err?.message || err));
        } finally {
            setAssetsLoading(false);
            setAssetsRefreshing(false);
        }
    }, [apiBaseUrl, hasInitData, historyDays, initData]);

    useEffect(() => {
        if (activeTab !== 'my_assets') return undefined;
        loadAssets();
        if (!hasInitData) return undefined;
        const timer = setInterval(() => loadAssets(), Math.max(60, Number(pollIntervalSec || 15)) * 1000);
        return () => clearInterval(timer);
    }, [activeTab, hasInitData, loadAssets, pollIntervalSec]);

    const loadSmartMoney = useCallback(async ({ forceRefresh = false } = {}) => {
        if (!hasInitData || !isAdmin) return;
        if (hasSmartMoneyDataRef.current) setSmartMoneyRefreshing(true);
        else setSmartMoneyLoading(true);
        setSmartMoneyError('');
        try {
            const [overviewResult, leaderboardResult] = await Promise.allSettled([
                fetchAdminSmartMoneyOverview({ apiBaseUrl, initData, days: smartMoneyDays, forceRefresh }),
                fetchAdminSmartMoneyLeaderboard({ apiBaseUrl, initData, days: 1, metric: leaderboardMetric, limit: 20, forceRefresh }),
            ]);
            const overview = overviewResult.status === 'fulfilled' ? overviewResult.value : null;
            const leaderboard = leaderboardResult.status === 'fulfilled' ? leaderboardResult.value : null;
            const wallets = Array.isArray(overview?.wallets) ? overview.wallets : [];
            startTransition(() => {
                if (overviewResult.status === 'fulfilled') setSmartMoneyOverview(overview || null);
                if (leaderboardResult.status === 'fulfilled') setSmartMoneyLeaderboard(leaderboard || null);
            });
            if (!wallets.some((item) => walletKey(item) === selectedWalletId)) {
                setSelectedWalletId(wallets[0] ? walletKey(wallets[0]) : '');
            }
            const rejected = [overviewResult, leaderboardResult]
                .filter((item) => item.status === 'rejected')
                .map((item) => item.reason);
            const fatalError = rejected.find((item) => !isIgnorableSmartMoneyDataError(item));
            if (fatalError) {
                setSmartMoneyError(errorText(fatalError));
            }
        } catch (err) {
            setSmartMoneyError(errorText(err));
        } finally {
            setSmartMoneyLoading(false);
            setSmartMoneyRefreshing(false);
        }
    }, [apiBaseUrl, hasInitData, initData, isAdmin, leaderboardMetric, selectedWalletId, smartMoneyDays]);

    useEffect(() => {
        if (activeTab !== 'smart_money_assets') return undefined;
        loadSmartMoney();
        if (!hasInitData || !isAdmin) return undefined;
        const timer = setInterval(() => loadSmartMoney(), Math.max(60, Number(pollIntervalSec || 15)) * 1000);
        return () => clearInterval(timer);
    }, [activeTab, hasInitData, isAdmin, loadSmartMoney, pollIntervalSec]);

    const selectedWallet = useMemo(() => {
        const wallets = Array.isArray(smartMoneyOverview?.wallets) ? smartMoneyOverview.wallets : [];
        return wallets.find((item) => walletKey(item) === selectedWalletId) || null;
    }, [selectedWalletId, smartMoneyOverview]);

    const loadSmartMoneyWallet = useCallback(async ({ wallet, forceRefresh = false } = {}) => {
        if (!wallet || !hasInitData || !isAdmin) return;
        try {
            const detail = await fetchAdminSmartMoneyWallet({
                apiBaseUrl,
                initData,
                address: wallet.address,
                chainId: wallet.chain_id,
                days: smartMoneyDays,
                forceRefresh,
            });
            startTransition(() => {
                setSmartMoneyWallet(detail || null);
            });
            setSmartMoneyError('');
        } catch (err) {
            if (isIgnorableSmartMoneyDataError(err)) {
                setSmartMoneyError('');
            } else {
                setSmartMoneyError(errorText(err));
            }
        }
    }, [apiBaseUrl, hasInitData, initData, isAdmin, smartMoneyDays]);

    useEffect(() => {
        if (activeTab !== 'smart_money_assets' || !selectedWallet || !hasInitData || !isAdmin) return undefined;
        let disposed = false;
        const run = async (forceRefresh = false) => {
            await loadSmartMoneyWallet({ wallet: selectedWallet, forceRefresh });
            if (disposed) return;
        };
        run();
        const timer = setInterval(() => run(), Math.max(60, Number(pollIntervalSec || 15)) * 1000);
        return () => {
            disposed = true;
            clearInterval(timer);
        };
    }, [activeTab, hasInitData, isAdmin, loadSmartMoneyWallet, pollIntervalSec, selectedWallet]);

    const chartRows = useMemo(() => seriesRows(assetsData.history, 'wallet_usd'), [assetsData.history]);
    const smartMoneyRows = useMemo(
        () => (Array.isArray(smartMoneyWallet?.history) ? smartMoneyWallet.history.map((item) => ({ close: Number(item?.total_usd || 0) })) : []),
        [smartMoneyWallet],
    );

    const smartMoneyPnlCalData = useMemo(() => {
        const history = Array.isArray(smartMoneyWallet?.history) ? [...smartMoneyWallet.history].sort((a, b) => a.day.localeCompare(b.day)) : [];
        if (history.length < 2) return [];
        return history.slice(1).map((item, i) => ({
            day: item.day,
            realized_pnl_usd: Number(item.total_usd || 0) - Number(history[i].total_usd || 0),
            has_transfer_in: Boolean(item.has_transfer_in),
            has_transfer_out: Boolean(item.has_transfer_out),
            transfer_in_count: Number(item.transfer_in_count || 0),
            transfer_out_count: Number(item.transfer_out_count || 0),
        }));
    }, [smartMoneyWallet?.history]);

    const SM_PAGE_SIZE = 10;

    const filteredWallets = useMemo(() => {
        const list = Array.isArray(smartMoneyOverview?.wallets) ? smartMoneyOverview.wallets : [];
        if (!smWalletSearch.trim()) return list;
        const q = smWalletSearch.trim().toLowerCase();
        return list.filter((w) => {
            const addr = String(w.address || '').toLowerCase();
            const label = String(w.label || '').toLowerCase();
            return addr.includes(q) || label.includes(q);
        });
    }, [smartMoneyOverview, smWalletSearch]);

    const walletTotalPages = Math.max(1, Math.ceil(filteredWallets.length / SM_PAGE_SIZE));
    const pagedWallets = useMemo(() => filteredWallets.slice(smWalletPage * SM_PAGE_SIZE, (smWalletPage + 1) * SM_PAGE_SIZE), [filteredWallets, smWalletPage]);

    const filteredLeaderboard = useMemo(() => {
        const list = Array.isArray(smartMoneyLeaderboard?.list) ? smartMoneyLeaderboard.list : [];
        if (!smLeaderSearch.trim()) return list;
        const q = smLeaderSearch.trim().toLowerCase();
        return list.filter((item) => {
            const addr = String(item.address || '').toLowerCase();
            const label = String(item.label || '').toLowerCase();
            return addr.includes(q) || label.includes(q);
        });
    }, [smartMoneyLeaderboard, smLeaderSearch]);

    const leaderTotalPages = Math.max(1, Math.ceil(filteredLeaderboard.length / SM_PAGE_SIZE));
    const pagedLeaderboard = useMemo(() => filteredLeaderboard.slice(smLeaderPage * SM_PAGE_SIZE, (smLeaderPage + 1) * SM_PAGE_SIZE), [filteredLeaderboard, smLeaderPage]);

    const isLoading = assetsLoading || assetsRefreshing || smartMoneyLoading || smartMoneyRefreshing;
    const canManualRefresh = hasInitData && (activeTab === 'my_assets' || activeTab === 'smart_money_assets');

    return (
        <div className="flex flex-col gap-3">
            {/* ══════ Header ══════ */}
            <Card className="!p-0 overflow-hidden">
                <div className="px-3.5 pt-3 pb-2.5">
                    <div className="flex items-center justify-between gap-3">
                        <div className="min-w-0">
                            <div className="text-[14px] font-extrabold leading-tight text-zinc-900 dark:text-white/95">资产管理</div>
                            <div className="mt-0.5 text-[10px] text-zinc-500 dark:text-white/40">
                                {hasInitData ? '资产快照 / 趋势 / LP 统计' : '需要有效的 Telegram initData'}
                            </div>
                        </div>
                        <button
                            type="button"
                            onClick={() => {
                                if (activeTab === 'my_assets') loadAssets({ forceRefresh: true });
                                if (activeTab === 'smart_money_assets') {
                                    loadSmartMoney({ forceRefresh: true });
                                    if (selectedWallet) loadSmartMoneyWallet({ wallet: selectedWallet, forceRefresh: true });
                                }
                            }}
                            disabled={!canManualRefresh || isLoading}
                            className="inline-flex h-7 w-7 items-center justify-center rounded-lg border border-zinc-200/80 bg-zinc-50 text-zinc-500 transition active:scale-95 dark:border-white/5 dark:bg-[#1a1c20] dark:text-white/50 dark:hover:bg-white/5 disabled:opacity-40"
                        >
                            <RefreshCw className={`h-3.5 w-3.5 ${isLoading ? 'animate-spin' : ''}`} />
                        </button>
                    </div>
                </div>
                {/* tab bar */}
                <div className="flex border-t border-zinc-100 dark:border-white/[0.04]">
                    {tabs.map((tab) => {
                        const Icon = tab.icon;
                        const active = activeTab === tab.key;
                        return (
                            <button
                                key={tab.key}
                                type="button"
                                onClick={() => setActiveTab(tab.key)}
                                className={`relative flex flex-1 items-center justify-center gap-1.5 px-2 py-2.5 text-[11px] font-semibold transition ${
                                    active
                                        ? `${brand.textClass}`
                                        : 'text-zinc-400 hover:text-zinc-600 dark:text-white/35 dark:hover:text-white/60'
                                }`}
                            >
                                <Icon className="h-3.5 w-3.5" />
                                <span className="truncate">{tab.label}</span>
                                {active && (
                                    <span className={`absolute bottom-0 left-1/2 h-[2px] w-6 -translate-x-1/2 rounded-full ${brand.dotClass}`} />
                                )}
                            </button>
                        );
                    })}
                </div>
            </Card>

            {/* ══════ My Assets Tab ══════ */}
            {activeTab === 'my_assets' && (
                <>
                    {assetsError && (
                        <div className="rounded-xl border border-red-500/20 bg-red-500/[0.06] px-3 py-2.5 text-[11px] font-medium text-red-600 ring-1 ring-red-500/15 dark:text-red-300">{assetsError}</div>
                    )}
                    {assetsData.overview?.warnings?.length > 0 && (
                        <div className="rounded-xl border border-amber-500/20 bg-amber-500/[0.06] px-3 py-2.5 text-[11px] ring-1 ring-amber-500/15">
                            <div className="flex items-center gap-1.5 font-semibold text-amber-700 dark:text-amber-300"><AlertTriangle className="h-3 w-3" />数据提示</div>
                            <div className="mt-1.5 flex flex-wrap gap-1.5">
                                {assetsData.overview.warnings.map((w) => (
                                    <span key={w} className="inline-flex rounded-full border border-amber-500/20 bg-amber-500/10 px-2 py-0.5 text-[10px] font-semibold text-amber-700 dark:text-amber-200">{w}</span>
                                ))}
                            </div>
                        </div>
                    )}

                    {/* overview metrics */}
                    <div className="grid grid-cols-2 gap-2">
                        <StatBlock label="总资产" value={formatUsd(assetsData.overview?.summary?.total_usd)} tone="accent" />
                        <StatBlock label="钱包余额" value={formatUsd(assetsData.overview?.summary?.wallet_usd)} />
                        <StatBlock label="LP 持仓" value={formatUsd(assetsData.overview?.summary?.position_usd)} />
                        <StatBlock label="未领取手续费" value={formatUsd(assetsData.overview?.summary?.fee_usd)} tone="warn" />
                    </div>

                    {/* trend chart */}
                    <Card>
                        <div className="flex items-center justify-between gap-2">
                            <span className="text-[12px] font-bold text-zinc-900 dark:text-white/90">钱包余额趋势</span>
                            <div className="flex gap-1.5">
                                {HISTORY_WINDOWS.map((d) => (
                                    <Pill key={d} active={historyDays === d} brand={brand} onClick={() => setHistoryDays(d)}>{d}D</Pill>
                                ))}
                            </div>
                        </div>
                        <div className="mt-3 rounded-xl border border-zinc-100 bg-zinc-50/60 p-3 dark:border-white/[0.04] dark:bg-[#0d0f12]">
                            <div className="flex items-end justify-between gap-2">
                                <div>
                                    <div className="text-[9px] font-medium uppercase tracking-wide text-zinc-400 dark:text-white/35">
                                        钱包余额
                                    </div>
                                    <div className="mt-1 text-xl font-extrabold tabular-nums text-zinc-900 dark:text-white leading-none">
                                        <NumberFlowValue value={chartRows[chartRows.length - 1]?.value || 0} formatter={(v) => formatUsd(v)} />
                                    </div>
                                </div>
                                <span className="text-[10px] text-zinc-400 dark:text-white/30">
                                    {assetsData.overview?.updated_at ? new Date(assetsData.overview.updated_at).toLocaleTimeString() : ''}
                                </span>
                            </div>
                            <div className="mt-3">
                                <LWAreaChart data={chartRows} color="#0ea5e9" loading={assetsLoading} />
                            </div>
                        </div>
                    </Card>

                    {/* today data */}
                    <Card>
                        <div className="flex items-center justify-between gap-2">
                            <span className="text-[12px] font-bold text-zinc-900 dark:text-white/90">今日盈亏</span>
                            <span className={`inline-flex items-center rounded-full px-2.5 py-0.5 text-[11px] font-bold tabular-nums ring-1 ${
                                Number(assetsData.lp?.today?.realized_pnl_usd || 0) >= 0
                                    ? 'bg-emerald-500/[0.08] text-emerald-600 ring-emerald-500/20 dark:bg-emerald-500/[0.12] dark:text-emerald-300'
                                    : 'bg-red-500/[0.08] text-red-600 ring-red-500/20 dark:bg-red-500/[0.12] dark:text-red-300'
                            }`}>
                                {Number(assetsData.lp?.today?.realized_pnl_usd || 0) >= 0 ? '+' : ''}{formatUsd(assetsData.lp?.today?.realized_pnl_usd)}
                            </span>
                        </div>
                        <div className="mt-2 grid grid-cols-4 gap-1.5">
                            <div className="rounded-lg bg-zinc-50 px-2 py-1.5 ring-1 ring-zinc-200 dark:bg-white/[0.03] dark:ring-white/[0.06]">
                                <div className="text-[9px] text-zinc-400 dark:text-white/30">平仓</div>
                                <div className="mt-0.5 text-[11px] font-bold tabular-nums text-zinc-800 dark:text-white/80">{Number(assetsData.lp?.today?.closed_count || 0)}</div>
                            </div>
                            <div className="rounded-lg bg-zinc-50 px-2 py-1.5 ring-1 ring-zinc-200 dark:bg-white/[0.03] dark:ring-white/[0.06]">
                                <div className="text-[9px] text-zinc-400 dark:text-white/30">胜率</div>
                                <div className="mt-0.5 text-[11px] font-bold tabular-nums text-zinc-800 dark:text-white/80">{formatPct(assetsData.lp?.today?.win_rate)}</div>
                            </div>
                            <div className="rounded-lg bg-emerald-500/[0.06] px-2 py-1.5 ring-1 ring-emerald-500/15 dark:bg-emerald-500/[0.08]">
                                <div className="text-[9px] text-emerald-600 dark:text-emerald-400">盈利</div>
                                <div className="mt-0.5 text-[11px] font-bold tabular-nums text-emerald-700 dark:text-emerald-300">{Number(assetsData.lp?.today?.win_count || 0)}</div>
                            </div>
                            <div className="rounded-lg bg-red-500/[0.06] px-2 py-1.5 ring-1 ring-red-500/15 dark:bg-red-500/[0.08]">
                                <div className="text-[9px] text-red-600 dark:text-red-400">亏损</div>
                                <div className="mt-0.5 text-[11px] font-bold tabular-nums text-red-700 dark:text-red-300">{Number(assetsData.lp?.today?.loss_count || 0)}</div>
                            </div>
                        </div>
                        <div className="mt-3">
                            <TodayPoolPnL pools={assetsData.lp?.today_pools} brand={brand} />
                        </div>
                    </Card>

                    {/* LP daily PnL calendar */}
                    <Card>
                        <PnLCalendar data={assetsData.lp?.daily_history} loading={assetsLoading} />
                        {/* summary stats below chart */}
                        {Array.isArray(assetsData.lp?.windows) && assetsData.lp.windows.length > 0 && (
                            <div className="mt-2.5 grid grid-cols-3 gap-1.5">
                                {assetsData.lp.windows.map((item) => (
                                    <div key={item.days} className="rounded-lg bg-zinc-50 px-2.5 py-2 ring-1 ring-zinc-200 dark:bg-white/[0.03] dark:ring-white/[0.06]">
                                        <div className="text-[9px] text-zinc-400 dark:text-white/30">{item.days}D</div>
                                        <div className={`mt-0.5 text-[11px] font-bold tabular-nums ${Number(item.realized_pnl_usd || 0) >= 0 ? 'text-emerald-600 dark:text-emerald-300' : 'text-red-600 dark:text-red-300'}`}>
                                            {Number(item.realized_pnl_usd || 0) >= 0 ? '+' : ''}{formatUsdCompact(item.realized_pnl_usd)}
                                        </div>
                                        <div className="mt-0.5 text-[9px] text-zinc-400 dark:text-white/30">{item.closed_count || 0} 笔 · {formatPct(item.win_rate)}</div>
                                    </div>
                                ))}
                            </div>
                        )}
                    </Card>

                    {/* wallet distribution */}
                    <Card>
                        <div className="text-[12px] font-bold text-zinc-900 dark:text-white/90">钱包资产分布</div>
                        {Array.isArray(assetsData.overview?.wallets) && assetsData.overview.wallets.length > 0 ? (
                            <div className="mt-3 flex flex-col gap-4">
                                <DonutChart wallets={assetsData.overview.wallets} />
                                <div className="border-t border-zinc-100 dark:border-white/[0.04] pt-3">
                                    <WalletStackedBar wallets={assetsData.overview.wallets} />
                                </div>
                            </div>
                        ) : <div className="mt-2.5"><Empty text={assetsLoading ? '加载中...' : '暂无钱包数据'} /></div>}
                    </Card>
                </>
            )}

            {/* ══════ Smart Money Tab ══════ */}
            {activeTab === 'smart_money_assets' && (
                <>
                    {smartMoneyError && (
                        <div className="rounded-xl border border-red-500/20 bg-red-500/[0.06] px-3 py-2.5 text-[11px] font-medium text-red-600 ring-1 ring-red-500/15 dark:text-red-300">{smartMoneyError}</div>
                    )}

                    <div className="flex flex-wrap gap-1.5">
                        {SMART_MONEY_WINDOWS.map((d) => (
                            <Pill key={d} active={smartMoneyDays === d} brand={brand} onClick={() => setSmartMoneyDays(d)}>{d === 1 ? '昨日' : `${d}D`}</Pill>
                        ))}
                    </div>

                    <div className="grid grid-cols-2 gap-2">
                        <StatBlock label="总资产" value={formatUsd(smartMoneyOverview?.summary?.total_usd)} tone="accent" />
                        <StatBlock label="原生币" value={formatUsd(smartMoneyOverview?.summary?.native_usd)} />
                        <StatBlock label="稳定币" value={formatUsd(smartMoneyOverview?.summary?.stable_usd)} />
                        <StatBlock label="代币持仓" value={formatUsd(smartMoneyOverview?.summary?.tracked_token_usd)} />
                        <StatBlock label="Open LP" value={formatUsd(smartMoneyOverview?.summary?.open_lp_usd)} />
                        <StatBlock label="代币种类" value={`${Number(smartMoneyOverview?.summary?.tracked_token_count || 0)} 个`} />
                    </div>

                    {/* sub-tab pills */}
                    <div className="flex gap-1.5">
                        <Pill active={smSubTab === 'wallets'} brand={brand} onClick={() => { setSmSubTab('wallets'); setSmDrillWalletId(''); }}>钱包总览</Pill>
                        <Pill active={smSubTab === 'leaderboard'} brand={brand} onClick={() => { setSmSubTab('leaderboard'); setSmDrillWalletId(''); }}>排行榜</Pill>
                    </div>

                    {/* ── wallets sub-tab ── */}
                    {smSubTab === 'wallets' && !smDrillWalletId && (
                        <Card>
                            <div className="flex items-center justify-between gap-2">
                                <span className="text-[12px] font-bold text-zinc-900 dark:text-white/90">钱包总览</span>
                                <span className="inline-flex items-center rounded-full bg-zinc-100 px-2 py-0.5 text-[10px] font-semibold text-zinc-500 ring-1 ring-zinc-200 dark:bg-white/[0.04] dark:text-white/50 dark:ring-white/[0.06]">
                                    {filteredWallets.length} 个
                                </span>
                            </div>
                            <div className="mt-2">
                                <SmSearchInput value={smWalletSearch} onChange={(v) => { setSmWalletSearch(v); setSmWalletPage(0); }} placeholder="搜索地址或标签" />
                            </div>
                            <div className="mt-2.5 flex flex-col gap-2">
                                {pagedWallets.length > 0 ? pagedWallets.map((wallet) => {
                                    const selected = walletKey(wallet) === selectedWalletId;
                                    const assets = wallet.assets || {};
                                    const total = Number(assets.total_usd || 0);
                                    const nativePct = total > 0 ? (Number(assets.native_usd || 0) / total * 100) : 0;
                                    const stablePct = total > 0 ? (Number(assets.stable_usd || 0) / total * 100) : 0;
                                    const tokenPct = total > 0 ? (Number(assets.tracked_token_usd || 0) / total * 100) : 0;
                                    const lpPct = total > 0 ? (Number(assets.open_lp_usd || 0) / total * 100) : 0;
                                    return (
                                        <button
                                            key={walletKey(wallet)}
                                            type="button"
                                            onClick={() => { const wk = walletKey(wallet); setSelectedWalletId(wk); setSmDrillWalletId(wk); }}
                                            className={`flex w-full flex-col gap-2 rounded-xl border px-3 py-2.5 text-left transition active:scale-[0.98] ${
                                                selected
                                                    ? `${brand.selectionClass} dark:text-white`
                                                    : 'border-zinc-100 bg-zinc-50/60 text-zinc-700 hover:bg-white dark:border-white/[0.04] dark:bg-[#0d0f12] dark:text-white/75 dark:hover:bg-white/[0.06]'
                                            }`}
                                        >
                                            <div className="flex items-center justify-between gap-3 w-full">
                                                <div className="flex items-center gap-2.5 min-w-0">
                                                    <WalletAvatar address={wallet.address} size={28} />
                                                    <div className="min-w-0">
                                                        <div className="truncate text-[12px] font-semibold">{walletLabel(wallet)}</div>
                                                        <div className="mt-0.5 text-[10px] opacity-60">{formatChain(wallet.chain_id)} · {Number(wallet.today_event_count || 0)} 事件 · {Number(wallet.active_pool_count || 0)} 池</div>
                                                    </div>
                                                </div>
                                                <div className="flex items-center gap-1.5 shrink-0">
                                                    <span className="text-[13px] font-bold tabular-nums">{formatUsdCompact(wallet.assets?.total_usd)}</span>
                                                    <ChevronRight className="h-3 w-3 opacity-40" />
                                                </div>
                                            </div>
                                            {/* asset breakdown bar */}
                                            {total > 0 && (
                                                <div className="w-full">
                                                    <div className="h-1.5 w-full rounded-full overflow-hidden bg-zinc-200/60 dark:bg-white/[0.06]">
                                                        <div className="flex h-full">
                                                            {nativePct > 0 && <div className="h-full" style={{ width: `${nativePct}%`, backgroundColor: '#0ea5e9' }} />}
                                                            {stablePct > 0 && <div className="h-full" style={{ width: `${stablePct}%`, backgroundColor: '#10b981' }} />}
                                                            {tokenPct > 0 && <div className="h-full" style={{ width: `${tokenPct}%`, backgroundColor: '#8b5cf6' }} />}
                                                            {lpPct > 0 && <div className="h-full" style={{ width: `${lpPct}%`, backgroundColor: '#f59e0b' }} />}
                                                        </div>
                                                    </div>
                                                    <div className="mt-1 flex flex-wrap gap-x-3 gap-y-0.5">
                                                        {Number(assets.native_usd || 0) > 0 && <span className="flex items-center gap-1 text-[9px] text-zinc-400 dark:text-white/35"><span className="h-1.5 w-1.5 rounded-full" style={{ backgroundColor: '#0ea5e9' }} />原生 {formatUsdCompact(assets.native_usd)}</span>}
                                                        {Number(assets.stable_usd || 0) > 0 && <span className="flex items-center gap-1 text-[9px] text-zinc-400 dark:text-white/35"><span className="h-1.5 w-1.5 rounded-full" style={{ backgroundColor: '#10b981' }} />稳定 {formatUsdCompact(assets.stable_usd)}</span>}
                                                        {Number(assets.tracked_token_usd || 0) > 0 && <span className="flex items-center gap-1 text-[9px] text-zinc-400 dark:text-white/35"><span className="h-1.5 w-1.5 rounded-full" style={{ backgroundColor: '#8b5cf6' }} />代币 {formatUsdCompact(assets.tracked_token_usd)}</span>}
                                                        {Number(assets.open_lp_usd || 0) > 0 && <span className="flex items-center gap-1 text-[9px] text-zinc-400 dark:text-white/35"><span className="h-1.5 w-1.5 rounded-full" style={{ backgroundColor: '#f59e0b' }} />LP {formatUsdCompact(assets.open_lp_usd)}</span>}
                                                    </div>
                                                </div>
                                            )}
                                        </button>
                                    );
                                }) : <Empty text={smartMoneyLoading ? '加载中...' : '暂无钱包数据'} />}
                            </div>
                            <SmPagination page={smWalletPage} totalPages={walletTotalPages} onPageChange={setSmWalletPage} />
                        </Card>
                    )}

                    {/* ── wallet drill-in detail ── */}
                    {smSubTab === 'wallets' && smDrillWalletId && (
                        <Card>
                            <button type="button" onClick={() => setSmDrillWalletId('')} className="inline-flex items-center gap-1 text-[11px] font-medium text-zinc-500 hover:text-zinc-700 dark:text-white/40 dark:hover:text-white/70 transition mb-2">
                                <ChevronLeft className="h-3.5 w-3.5" />返回列表
                            </button>
                            {selectedWallet && smartMoneyWallet ? (
                                <div className="flex flex-col gap-2.5">
                                    {/* wallet header */}
                                    <div className="flex items-center gap-3 rounded-xl bg-emerald-500/[0.06] ring-1 ring-emerald-500/20 dark:bg-emerald-500/[0.08] dark:ring-emerald-400/25 px-3 py-2.5">
                                        <WalletAvatar address={selectedWallet.address} size={36} />
                                        <div className="flex-1 min-w-0">
                                            <div className="text-[12px] font-bold text-zinc-900 dark:text-white/95 truncate">{walletLabel(selectedWallet)}</div>
                                            <div className="mt-0.5 text-[10px] text-zinc-500 dark:text-white/40">
                                                {formatChain(selectedWallet.chain_id)} · 总资产 <span className="font-bold text-zinc-900 dark:text-white/90">{formatUsdCompact(smartMoneyWallet.wallet?.assets?.total_usd)}</span>
                                            </div>
                                        </div>
                                    </div>

                                    {/* today activity grid */}
                                    <div className="grid grid-cols-3 gap-1.5">
                                        <StatBlock label="今日收益" value={formatUsd(smartMoneyWallet.today?.estimated_realized_pnl_usd)} tone={Number(smartMoneyWallet.today?.estimated_realized_pnl_usd || 0) >= 0 ? 'accent' : 'warn'} />
                                        <StatBlock label="加仓次数" value={`${Number(smartMoneyWallet.today?.add_count || 0)} 次`} />
                                        <StatBlock label="撤仓次数" value={`${Number(smartMoneyWallet.today?.remove_count || 0)} 次`} />
                                        <StatBlock label="活跃池数" value={`${Number(smartMoneyWallet.today?.active_pool_count || 0)} 池`} />
                                        <StatBlock label="已匹配" value={`${Number(smartMoneyWallet.today?.matched_remove_count || 0)} 次`} />
                                        <StatBlock label="未匹配" value={`${Number(smartMoneyWallet.today?.unmatched_remove_count || 0)} 次`} tone={Number(smartMoneyWallet.today?.unmatched_remove_count || 0) > 0 ? 'warn' : 'default'} />
                                    </div>

                                    {/* asset distribution bar */}
                                    {(() => {
                                        const wa = smartMoneyWallet.wallet?.assets || {};
                                        const total = Number(wa.total_usd || 0);
                                        if (total <= 0) return null;
                                        const nativePct = Number(wa.native_usd || 0) / total * 100;
                                        const stablePct = Number(wa.stable_usd || 0) / total * 100;
                                        const tokenPct = Number(wa.tracked_token_usd || 0) / total * 100;
                                        const lpPct = Number(wa.open_lp_usd || 0) / total * 100;
                                        return (
                                            <div className="rounded-xl border border-zinc-100 bg-zinc-50/60 px-3 py-2.5 dark:border-white/[0.04] dark:bg-[#0d0f12]">
                                                <div className="text-[9px] font-medium uppercase tracking-wide text-zinc-400 dark:text-white/35 mb-2">资产分布</div>
                                                <div className="h-2 w-full rounded-full overflow-hidden bg-zinc-200/60 dark:bg-white/[0.06]">
                                                    <div className="flex h-full">
                                                        {nativePct > 0 && <div className="h-full" style={{ width: `${nativePct}%`, backgroundColor: '#0ea5e9' }} />}
                                                        {stablePct > 0 && <div className="h-full" style={{ width: `${stablePct}%`, backgroundColor: '#10b981' }} />}
                                                        {tokenPct > 0 && <div className="h-full" style={{ width: `${tokenPct}%`, backgroundColor: '#8b5cf6' }} />}
                                                        {lpPct > 0 && <div className="h-full" style={{ width: `${lpPct}%`, backgroundColor: '#f59e0b' }} />}
                                                    </div>
                                                </div>
                                                <div className="mt-1.5 flex flex-wrap gap-x-3 gap-y-0.5">
                                                    <span className="flex items-center gap-1 text-[9px] text-zinc-400 dark:text-white/35"><span className="h-1.5 w-1.5 rounded-full" style={{ backgroundColor: '#0ea5e9' }} />原生 {formatUsdCompact(wa.native_usd)}</span>
                                                    <span className="flex items-center gap-1 text-[9px] text-zinc-400 dark:text-white/35"><span className="h-1.5 w-1.5 rounded-full" style={{ backgroundColor: '#10b981' }} />稳定 {formatUsdCompact(wa.stable_usd)}</span>
                                                    <span className="flex items-center gap-1 text-[9px] text-zinc-400 dark:text-white/35"><span className="h-1.5 w-1.5 rounded-full" style={{ backgroundColor: '#8b5cf6' }} />代币 {formatUsdCompact(wa.tracked_token_usd)}</span>
                                                    <span className="flex items-center gap-1 text-[9px] text-zinc-400 dark:text-white/35"><span className="h-1.5 w-1.5 rounded-full" style={{ backgroundColor: '#f59e0b' }} />LP {formatUsdCompact(wa.open_lp_usd)}</span>
                                                </div>
                                            </div>
                                        );
                                    })()}

                                    {/* PnL calendar (daily balance diff) */}
                                    <PnLCalendar
                                        data={smartMoneyPnlCalData}
                                        note="按日资产快照差额估算；若出现“转入/转出”标记，说明该日检测到资金划转，当天盈亏会受划转影响。"
                                    />

                                    {/* window stats */}
                                    <div className="grid grid-cols-3 gap-1.5">
                                        {Array.isArray(smartMoneyWallet.windows) && smartMoneyWallet.windows.map((item) => (
                                            <StatBlock key={item.days} label={`${item.days}D`} value={formatUsd(item.estimated_realized_pnl_usd)} sub={`${formatPct(item.yield_rate)} · ${Number(item.active_pool_count || 0)} 池`} />
                                        ))}
                                    </div>

                                    {/* warnings */}
                                    {Array.isArray(smartMoneyWallet.warnings) && smartMoneyWallet.warnings.length > 0 && (
                                        <div className="flex flex-col gap-1.5">
                                            {smartMoneyWallet.warnings.map((warn, i) => (
                                                <div key={i} className="flex items-start gap-2 rounded-xl border border-amber-500/20 bg-amber-500/[0.06] px-3 py-2 dark:border-amber-400/15 dark:bg-amber-500/[0.08]">
                                                    <AlertTriangle className="h-3.5 w-3.5 shrink-0 text-amber-500 mt-0.5" />
                                                    <span className="text-[10px] text-amber-700 dark:text-amber-300">{typeof warn === 'string' ? warn : warn.message || JSON.stringify(warn)}</span>
                                                </div>
                                            ))}
                                        </div>
                                    )}
                                </div>
                            ) : <Empty text={smartMoneyLoading ? '加载中...' : '选择钱包查看明细'} />}
                        </Card>
                    )}

                    {/* ── leaderboard sub-tab ── */}
                    {smSubTab === 'leaderboard' && (
                        <Card>
                            <div className="flex items-center justify-between gap-2">
                                    <span className="text-[12px] font-bold text-zinc-900 dark:text-white/90">昨日快照排行</span>
                                    <div className="flex gap-1.5">
                                        {LEADERBOARD_METRICS.map((m) => (
                                            <Pill key={m.key} active={leaderboardMetric === m.key} brand={brand} onClick={() => setLeaderboardMetric(m.key)}>{m.label}</Pill>
                                        ))}
                                    </div>
                                </div>
                            <div className="mt-1.5 flex items-center gap-1.5 text-[9px] text-zinc-400 dark:text-white/30">
                                <ArrowRightLeft className="h-3 w-3 shrink-0" />
                                <span>
                                    {smartMoneyLeaderboard?.snapshot_day && (smartMoneyLeaderboard?.compared_day || smartMoneyLeaderboard?.start_day)
                                        ? `榜单基于 ${smartMoneyLeaderboard.snapshot_day} 相对 ${smartMoneyLeaderboard.compared_day || smartMoneyLeaderboard.start_day} 的资产快照`
                                        : '榜单基于昨日资产快照'}
                                </span>
                            </div>
                            <div className="mt-2">
                                <SmSearchInput value={smLeaderSearch} onChange={(v) => { setSmLeaderSearch(v); setSmLeaderPage(0); }} placeholder="搜索地址或标签" />
                            </div>
                            <div className="mt-2.5 flex flex-col gap-2">
                                {pagedLeaderboard.length > 0 ? pagedLeaderboard.map((item) => {
                                    const metricText = leaderboardMetric === 'yield_rate' ? formatPct(item.metric_value) : leaderboardMetric === 'participation' ? `${Number(item.metric_value || 0)} 次` : formatUsd(item.metric_value);
                                    const pnl = Number(item.estimated_realized_pnl_usd || 0);
                                    const isTop3 = item.rank <= 3;
                                    return (
                                        <div
                                            key={`${item.rank}:${item.address}`}
                                            className={`flex items-center gap-3 rounded-xl border px-3 py-3 transition ${
                                                isTop3
                                                    ? 'border-amber-500/15 bg-gradient-to-r from-amber-500/[0.04] to-transparent dark:border-amber-400/10 dark:from-amber-500/[0.06]'
                                                    : 'border-zinc-100 bg-zinc-50/60 dark:border-white/[0.04] dark:bg-[#0d0f12]'
                                            }`}
                                        >
                                            <RankBadge rank={Number(item.rank || 0)} />
                                            <WalletAvatar address={item.address} size={32} />
                                            <div className="flex-1 min-w-0">
                                                <div className="flex items-center gap-1.5">
                                                    <span className="truncate text-[12px] font-semibold text-zinc-900 dark:text-white/90">
                                                        {item.label || `${item.address.slice(0, 6)}...${item.address.slice(-4)}`}
                                                    </span>
                                                    <span className="shrink-0 rounded bg-zinc-100 px-1 py-0.5 text-[8px] font-medium text-zinc-500 dark:bg-white/[0.06] dark:text-white/40">
                                                        {formatChain(item.chain_id)}
                                                    </span>
                                                </div>
                                                <div className="mt-0.5 flex items-center gap-2 text-[10px] text-zinc-500 dark:text-white/40">
                                                    <span>{Number(item.active_pool_count || 0)} 池</span>
                                                    <span>·</span>
                                                    <span>{Number(item.participation_count || 0)} 次操作</span>
                                                    {Number(item.unmatched_remove_count || 0) > 0 && (
                                                        <>
                                                            <span>·</span>
                                                            <span className="text-amber-500">{item.unmatched_remove_count} 未匹配</span>
                                                        </>
                                                    )}
                                                </div>
                                                {hasTransferMarker(item) && (
                                                    <div className="mt-1">
                                                        <TransferBadges item={item} />
                                                    </div>
                                                )}
                                            </div>
                                            <div className="text-right shrink-0">
                                                <div className={`text-[13px] font-bold tabular-nums ${
                                                    leaderboardMetric === 'pnl'
                                                        ? pnl >= 0 ? 'text-emerald-600 dark:text-emerald-400' : 'text-red-500 dark:text-red-400'
                                                        : 'text-zinc-900 dark:text-white'
                                                }`}>{metricText}</div>
                                                {leaderboardMetric !== 'pnl' && (
                                                    <div className={`mt-0.5 text-[10px] tabular-nums ${pnl >= 0 ? 'text-emerald-600 dark:text-emerald-400' : 'text-red-500 dark:text-red-400'}`}>
                                                        {pnl >= 0 ? '+' : ''}{formatUsdCompact(pnl)}
                                                    </div>
                                                )}
                                                {leaderboardMetric === 'pnl' && Number(item.yield_rate || 0) !== 0 && (
                                                    <div className="mt-0.5 text-[10px] text-zinc-500 dark:text-white/40 tabular-nums">
                                                        {formatPct(item.yield_rate)}
                                                    </div>
                                                )}
                                            </div>
                                        </div>
                                    );
                                }) : <Empty text={smartMoneyLoading ? '加载中...' : '暂无排行榜数据'} />}
                            </div>
                            <SmPagination page={smLeaderPage} totalPages={leaderTotalPages} onPageChange={setSmLeaderPage} />
                        </Card>
                    )}
                </>
            )}

            {/* ══════ Operations & System Tabs ══════ */}
            {activeTab === 'operations' && (
                <Suspense fallback={<Card><div className="text-[11px] text-zinc-400 dark:text-white/35">正在加载管理模块...</div></Card>}>
                    <LazyAdminPage apiBaseUrl={apiBaseUrl} initData={initData} hasInitData={hasInitData} tick={tick} pollIntervalSec={pollIntervalSec} accentTheme={accentTheme} visibleTabs={['online_users', 'active_tasks', 'user_detail']} initialTab="online_users" onNotice={onNotice} />
                </Suspense>
            )}
            {activeTab === 'system' && (
                <Suspense fallback={<Card><div className="text-[11px] text-zinc-400 dark:text-white/35">正在加载系统模块...</div></Card>}>
                    <LazyAdminPage apiBaseUrl={apiBaseUrl} initData={initData} hasInitData={hasInitData} tick={tick} pollIntervalSec={pollIntervalSec} accentTheme={accentTheme} visibleTabs={['system_config', 'rpc_pool']} initialTab="system_config" onNotice={onNotice} />
                </Suspense>
            )}
        </div>
    );
}
