import React, { startTransition, useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { AlertTriangle, ArrowRightLeft, CheckCircle2, ChevronLeft, ChevronRight, Crown, Eraser, History, Medal, RefreshCw, Search, Settings2, Shield, TrendingUp, Trophy, Wallet } from 'lucide-react';
import {
    fetchAssetHistory,
    fetchAssetLPStats,
    fetchAssetOverview,
    clearAssetLPPnLAdjustment,
    clearAssetLPPnLBaseline,
    saveAssetLPPnLAdjustment,
    saveAssetLPPnLBaseline,
} from '../lib/api';
import { getBrandTheme } from '../lib/brand';
import GlobalConfigPage from './GlobalConfigPage.jsx';
import MiniChart from './MiniChart.jsx';
import NumberFlowValue from './NumberFlowValue.jsx';
import TradeHistoryPage from './TradeHistoryPage.jsx';
import WalletManagePage from './WalletManagePage.jsx';

const AVATAR_URLS = Object.entries(
    import.meta.glob('../icon/avatar_*.png', { eager: true, import: 'default' })
).sort(([a], [b]) => a.localeCompare(b, undefined, { numeric: true })).map(([, src]) => src);

const HISTORY_WINDOWS = [7, 30, 90];
const PNL_CALENDAR_WINDOWS = [
    { key: 'month', label: '本月' },
    { key: '30d', label: '30天' },
];
const SMART_MONEY_WINDOWS = [1, 7, 30];
const CHINA_TIME_ZONE = 'Asia/Shanghai';
const LEADERBOARD_METRICS = [
    { key: 'pnl', label: '收益额' },
    { key: 'yield_rate', label: '收益率' },
    { key: 'participation', label: '参与次数' },
];

const SM_PAGE_SIZE = 10;
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

function chinaDateParts(value = new Date()) {
    const date = value instanceof Date ? value : new Date(value);
    if (Number.isNaN(date.getTime())) return null;
    const parts = new Intl.DateTimeFormat('en-CA', {
        timeZone: CHINA_TIME_ZONE,
        year: 'numeric',
        month: '2-digit',
        day: '2-digit',
    }).formatToParts(date);
    const map = {};
    parts.forEach((part) => {
        if (part.type !== 'literal') map[part.type] = part.value;
    });
    if (!map.year || !map.month || !map.day) return null;
    return map;
}

function formatChinaDay(value = new Date()) {
    const parts = chinaDateParts(value);
    if (!parts) return '';
    return `${parts.year}-${parts.month}-${parts.day}`;
}

function formatChinaTime(value) {
    if (!value) return '';
    const date = value instanceof Date ? value : new Date(value);
    if (Number.isNaN(date.getTime())) return '';
    return new Intl.DateTimeFormat('zh-CN', {
        timeZone: CHINA_TIME_ZONE,
        hour12: false,
        hour: '2-digit',
        minute: '2-digit',
        second: '2-digit',
    }).format(date);
}

function dayStamp(day) {
    const parts = String(day || '').split('-').map((item) => Number(item));
    if (parts.length !== 3 || parts.some((item) => !Number.isFinite(item))) return NaN;
    return Date.UTC(parts[0], parts[1] - 1, parts[2]);
}

function filterRowsByWindow(rows, days) {
    if (!Array.isArray(rows) || rows.length === 0) return [];
    const end = dayStamp(rows[rows.length - 1]?.day);
    if (!Number.isFinite(end) || !Number.isFinite(Number(days)) || Number(days) <= 0) return rows;
    const cutoff = end - (Number(days) - 1) * 24 * 60 * 60 * 1000;
    return rows.filter((item) => {
        const stamp = dayStamp(item?.day);
        return Number.isFinite(stamp) && stamp >= cutoff && stamp <= end;
    });
}

function mergeDailyPoints(history, todayPoint) {
    const merged = new Map();
    if (Array.isArray(history)) {
        history.forEach((item) => {
            const day = String(item?.day || '').trim();
            if (day) merged.set(day, item);
        });
    }
    const todayDay = String(todayPoint?.day || '').trim();
    if (todayDay) merged.set(todayDay, todayPoint);
    return [...merged.values()].sort((a, b) => String(a.day).localeCompare(String(b.day)));
}

function filterPnLCalendarRows(rows, windowKey) {
    if (!Array.isArray(rows) || rows.length === 0) return [];
    if (windowKey === '30d') {
        const today = dayStamp(formatChinaDay());
        if (!Number.isFinite(today)) return rows;
        const start = today - 29 * 24 * 60 * 60 * 1000;
        return rows.filter((item) => {
            const stamp = dayStamp(item?.day);
            return Number.isFinite(stamp) && stamp >= start && stamp <= today;
        });
    }
    const monthPrefix = formatChinaDay().slice(0, 7);
    return rows.filter((item) => String(item?.day || '').startsWith(monthPrefix));
}

function summarizePnLCalendarRows(rows) {
    const summary = {
        total: 0,
        positiveDays: 0,
        negativeDays: 0,
    };
    if (!Array.isArray(rows)) return summary;
    rows.forEach((item) => {
        const pnl = Number(item?.final_pnl_usd ?? item?.realized_pnl_usd ?? 0);
        if (!Number.isFinite(pnl)) return;
        summary.total += pnl;
        if (pnl > 0) summary.positiveDays += 1;
        if (pnl < 0) summary.negativeDays += 1;
    });
    summary.total = Number(summary.total.toFixed(2));
    return summary;
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
        <div className={`mini-am-card rounded-2xl border border-zinc-200/80 bg-white p-3 dark:border-white/5 dark:bg-[#14171c] ${className}`.trim()}>
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
        <div className={`mini-am-stat rounded-xl ${toneBg} ring-1 ${toneRing} px-3 py-2.5`}>
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

function WalletAvatar({ address, size = 28, className = '', avatarUrl }) {
    const fallbackSrc = useMemo(() => walletAvatarUrl(address), [address]);
    const preferredSrc = resolveSMAvatarAssetUrl(avatarUrl) || fallbackSrc;
    const [src, setSrc] = useState(preferredSrc);

    useEffect(() => {
        setSrc(preferredSrc);
    }, [preferredSrc]);

    if (!src) return null;
    return (
        <img
            src={src}
            alt=""
            width={size}
            height={size}
            className={`shrink-0 rounded-lg object-cover ${className}`.trim()}
            style={{ width: size, height: size }}
            onError={() => {
                if (src !== fallbackSrc) {
                    setSrc(fallbackSrc);
                }
            }}
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

function previousAreaChartDay(day) {
    const parts = String(day || '').split('-').map((item) => Number(item));
    if (parts.length !== 3 || parts.some((item) => !Number.isFinite(item))) return '';
    const date = new Date(Date.UTC(parts[0], parts[1] - 1, parts[2]));
    date.setUTCDate(date.getUTCDate() - 1);
    return date.toISOString().slice(0, 10);
}

function buildAreaChartData(rows) {
    const mapped = Array.isArray(rows)
        ? rows
            .filter((d) => d.day && Number.isFinite(Number(d.value)))
            .map((d) => ({ time: d.day, value: Number(d.value) }))
        : [];
    if (mapped.length !== 1) return mapped;
    const previousDay = previousAreaChartDay(mapped[0].time);
    if (!previousDay) return mapped;
    return [{ time: previousDay, value: mapped[0].value }, mapped[0]];
}

function computeAreaChartPriceRange(points) {
    if (!Array.isArray(points) || points.length === 0) return null;
    let min = Infinity;
    let max = -Infinity;
    points.forEach((point) => {
        if (!Number.isFinite(point.value)) return;
        min = Math.min(min, point.value);
        max = Math.max(max, point.value);
    });
    if (!Number.isFinite(min) || !Number.isFinite(max)) return null;
    const spread = max - min;
    const padding = spread > 0 ? spread * 0.12 : Math.max(Math.abs(max) * 0.02, 1);
    return {
        minValue: min - padding,
        maxValue: max + padding,
    };
}

/* ─── WebView-safe SVG Area Chart ─── */
function LWAreaChart({ data, color = '#10b981', loading = false }) {
    if (loading) {
        return <div className="animate-pulse rounded-lg bg-zinc-200 dark:bg-zinc-700" style={{ height: 200 }} />;
    }

    const points = buildAreaChartData(data);
    const priceRange = computeAreaChartPriceRange(points);
    const hasPoints = points.length >= 2 && priceRange;
    const width = 320;
    const height = 200;
    const padX = 14;
    const padTop = 12;
    const padBottom = 18;
    const plotWidth = width - padX * 2;
    const plotHeight = height - padTop - padBottom;
    const minValue = priceRange?.minValue;
    const maxValue = priceRange?.maxValue;
    const valueRange = maxValue - minValue;
    const chartPoints = hasPoints
        ? points.map((point, index) => {
            const x = padX + (points.length === 1 ? plotWidth / 2 : (index / (points.length - 1)) * plotWidth);
            const y = padTop + (1 - ((point.value - minValue) / valueRange)) * plotHeight;
            return { x, y };
        })
        : [];
    const linePath = chartPoints.map((point, index) => `${index === 0 ? 'M' : 'L'}${point.x.toFixed(2)} ${point.y.toFixed(2)}`).join(' ');
    const firstPoint = chartPoints[0];
    const lastPoint = chartPoints[chartPoints.length - 1];
    const areaPath = hasPoints
        ? `${linePath} L${lastPoint.x.toFixed(2)} ${height - padBottom} L${firstPoint.x.toFixed(2)} ${height - padBottom} Z`
        : '';
    const guideYs = [0.2, 0.5, 0.8].map((ratio) => padTop + ratio * plotHeight);

    return (
        <div className="relative w-full overflow-hidden rounded-lg" style={{ height }}>
            {hasPoints ? (
                <svg
                    viewBox={`0 0 ${width} ${height}`}
                    preserveAspectRatio="none"
                    className="h-full w-full"
                    role="img"
                    aria-label="asset trend"
                >
                    {guideYs.map((y) => (
                        <line
                            key={y}
                            x1={padX}
                            x2={width - padX}
                            y1={y}
                            y2={y}
                            stroke="currentColor"
                            strokeOpacity="0.08"
                            strokeWidth="1"
                            className="text-zinc-500 dark:text-white"
                        />
                    ))}
                    <path d={areaPath} fill={color} opacity="0.18" />
                    <path
                        d={linePath}
                        fill="none"
                        stroke={color}
                        strokeWidth="2.5"
                        strokeLinecap="round"
                        strokeLinejoin="round"
                        vectorEffect="non-scaling-stroke"
                    />
                    {lastPoint ? (
                        <circle
                            cx={lastPoint.x}
                            cy={lastPoint.y}
                            r="3.5"
                            fill="#0f1116"
                            stroke={color}
                            strokeWidth="2"
                            vectorEffect="non-scaling-stroke"
                        />
                    ) : null}
                </svg>
            ) : (
                <div className="flex h-full items-center justify-center text-[11px] text-zinc-400 dark:text-white/35">
                    暂无趋势数据
                </div>
            )}
        </div>
    );
}

/* ─── PnL Calendar (盈亏日历) ─── */
const PNL_CAL_WEEKDAYS = ['一', '二', '三', '四', '五', '六', '日'];
function PnLCalendar({ data, loading = false, note = '', selectedDay = '', allowNavigation = true, onSelectDay, onDismiss }) {
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

    const monthLabel = new Intl.DateTimeFormat('en-US', {
        timeZone: CHINA_TIME_ZONE,
        year: 'numeric',
        month: 'short',
    }).format(new Date(Date.UTC(year, month, 1, 12, 0, 0)));
    const prevMonth = () => setViewDate(new Date(year, month - 1, 1));
    const nextMonth = () => setViewDate(new Date(year, month + 1, 1));
    const todayStr = formatChinaDay();

    useEffect(() => {
        if (!allowNavigation) setViewDate(new Date());
    }, [allowNavigation]);

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
        const pnl = entry ? Number(entry.final_pnl_usd ?? entry.realized_pnl_usd ?? 0) : null;
        const isToday = dateStr === todayStr;
        const isFuture = dateStr > todayStr;
        const isSelected = dateStr === selectedDay;
        const hasTransfer = Boolean(entry?.transfer_total_count || entry?.has_transfer_in || entry?.has_transfer_out);
        const hasManualAdjustment = Math.abs(Number(entry?.manual_adjustment_usd || 0)) > 0.000001 || Boolean(String(entry?.adjustment_note || '').trim());
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
            <button
                type="button"
                key={day}
                disabled={!entry || isFuture}
                data-pnl-date-cell="true"
                data-pnl-selectable={entry && !isFuture ? 'true' : 'false'}
                onClick={() => {
                    if (entry && !isFuture) onSelectDay?.(entry);
                }}
                className={`relative rounded-md px-1 py-1 text-left transition active:scale-[0.98] ${
                    isToday ? 'bg-emerald-500/15 ring-1 ring-emerald-500/30'
                    : isFuture ? 'bg-zinc-100/30 dark:bg-white/[0.015]'
                    : 'bg-zinc-100/50 dark:bg-white/[0.03]'
                } ${isSelected ? 'ring-1 ring-cyan-400/50 shadow-sm shadow-cyan-400/10' : ''} ${entry && !isFuture ? 'cursor-pointer hover:bg-zinc-100 dark:hover:bg-white/[0.06]' : 'cursor-default'}`}
                style={{ minHeight: 38 }}
            >
                <div className={`text-[9px] leading-none ${dayToneClass}`}>{day}</div>
                <div className={`flex min-h-[20px] items-center justify-center px-0.5 text-center text-[10px] font-semibold leading-tight tabular-nums ${valueToneClass}`}>
                    {pnl !== null ? `${pnl >= 0 ? '+' : ''}${formatUsdCompact(pnl)}` : '0'}
                </div>
                {(hasTransfer || hasManualAdjustment) ? (
                    <div className="absolute bottom-0.5 right-1 flex gap-0.5">
                        {hasTransfer ? <span className="h-1 w-1 rounded-full bg-sky-400" /> : null}
                        {hasManualAdjustment ? <span className="h-1 w-1 rounded-full bg-amber-400" /> : null}
                    </div>
                ) : null}
            </button>
        );
    }
    const remainder = (startOffset + daysInMonth) % 7;
    if (remainder > 0) {
        for (let i = 0; i < 7 - remainder; i++) {
            cells.push(<div key={`t-${i}`} className="rounded-md bg-zinc-100/30 dark:bg-white/[0.015]" style={{ minHeight: 32 }} />);
        }
    }

    return (
        <div
            onPointerDown={(event) => {
                const target = event.target;
                if (!(target instanceof Element)) return;
                if (target.closest('[data-pnl-calendar-keep="true"]')) return;
                const dateCell = target.closest('[data-pnl-date-cell="true"]');
                if (dateCell?.getAttribute('data-pnl-selectable') === 'true') return;
                onDismiss?.();
            }}
        >
            <div className="flex items-center justify-between mb-2">
                <div className="flex items-center gap-1.5">
                    <span className="text-[13px] font-bold text-zinc-900 dark:text-white/90">{monthLabel}</span>
                    <svg className="w-3.5 h-3.5 text-zinc-400 dark:text-white/30" fill="none" stroke="currentColor" strokeWidth="2" viewBox="0 0 24 24">
                        <rect x="3" y="4" width="18" height="18" rx="2" /><line x1="16" y1="2" x2="16" y2="6" /><line x1="8" y1="2" x2="8" y2="6" /><line x1="3" y1="10" x2="21" y2="10" />
                    </svg>
                </div>
                {allowNavigation ? (
                    <div className="flex items-center gap-0.5" data-pnl-calendar-keep="true">
                        <button onClick={prevMonth} className="p-1 rounded-md hover:bg-zinc-200 dark:hover:bg-white/10 text-zinc-500 dark:text-white/40"><ChevronLeft size={14} /></button>
                        <button onClick={nextMonth} className="p-1 rounded-md hover:bg-zinc-200 dark:hover:bg-white/10 text-zinc-500 dark:text-white/40"><ChevronRight size={14} /></button>
                    </div>
                ) : null}
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

function PnLBreakdownEditor({ entry, brand, saving = false, error = '', onSave, onClear }) {
    const [manualValue, setManualValue] = useState('');
    const [note, setNote] = useState('');

    useEffect(() => {
        setManualValue(entry ? String(Number(entry.manual_adjustment_usd || 0)) : '');
        setNote(entry ? String(entry.adjustment_note || '') : '');
    }, [entry]);

    if (!entry) {
        return <Empty text="点击日历中的日期查看盈亏拆解" />;
    }

    const finalPnl = Number(entry.final_pnl_usd ?? entry.realized_pnl_usd ?? 0);
    const hasTransfer = Boolean(entry.transfer_total_count || entry.has_transfer_in || entry.has_transfer_out);
    const stats = [
        { label: '快照盈亏', value: entry.raw_pnl_usd, cls: Number(entry.raw_pnl_usd || 0) >= 0 ? 'text-emerald-600 dark:text-emerald-300' : 'text-red-600 dark:text-red-300' },
        { label: '手动校准', value: entry.manual_adjustment_usd, cls: Number(entry.manual_adjustment_usd || 0) >= 0 ? 'text-emerald-600 dark:text-emerald-300' : 'text-red-600 dark:text-red-300' },
        { label: '校准后', value: finalPnl, cls: finalPnl >= 0 ? 'text-emerald-600 dark:text-emerald-300' : 'text-red-600 dark:text-red-300' },
    ];

    return (
        <div className="mt-2.5 rounded-xl border border-zinc-100 bg-zinc-50/70 p-2.5 dark:border-white/[0.05] dark:bg-[#0f1116]" data-pnl-calendar-keep="true">
            <div className="flex items-start justify-between gap-2">
                <div className="min-w-0">
                    <div className="text-[11px] font-bold text-zinc-900 dark:text-white/90">{entry.day} 盈亏校准</div>
                    <div className="mt-0.5 text-[9px] leading-snug text-zinc-400 dark:text-white/30">默认按每日资产快照差额计算；充值、提现等偏差在这里手动校准。</div>
                </div>
                <span className={`shrink-0 rounded-full px-2 py-0.5 text-[10px] font-bold tabular-nums ring-1 ${
                    finalPnl >= 0
                        ? 'bg-emerald-500/[0.08] text-emerald-600 ring-emerald-500/20 dark:text-emerald-300'
                        : 'bg-red-500/[0.08] text-red-600 ring-red-500/20 dark:text-red-300'
                }`}>
                    {finalPnl >= 0 ? '+' : ''}{formatUsdCompact(finalPnl)}
                </span>
            </div>

            <div className="mt-2 grid grid-cols-3 gap-1.5">
                {stats.map((item) => (
                    <div key={item.label} className="rounded-lg bg-white px-2 py-1.5 ring-1 ring-zinc-200 dark:bg-white/[0.03] dark:ring-white/[0.06]">
                        <div className="text-[9px] text-zinc-400 dark:text-white/30">{item.label}</div>
                        <div className={`mt-0.5 text-[11px] font-bold tabular-nums ${item.cls}`}>
                            {Number(item.value || 0) >= 0 ? '+' : ''}{formatUsdCompact(item.value)}
                        </div>
                    </div>
                ))}
            </div>
            {hasTransfer ? (
                <div className="mt-2 rounded-lg border border-sky-500/15 bg-sky-500/[0.06] px-2 py-1.5 text-[9px] leading-snug text-zinc-500 dark:text-white/40">
                    检测到该日有转账记录：转入 {formatUsdCompact(entry.transfer_in_usd)}，转出 {formatUsdCompact(entry.transfer_out_usd)}。这些数据只作提示，不自动影响盈亏。
                </div>
            ) : null}

            <div className="mt-2 grid grid-cols-1 gap-1.5">
                <label className="flex flex-col gap-1">
                    <span className="text-[9px] text-zinc-400 dark:text-white/30">手动校准 USD</span>
                    <input
                        type="number"
                        step="0.01"
                        value={manualValue}
                        onChange={(e) => setManualValue(e.target.value)}
                        className="h-8 rounded-lg border border-zinc-200 bg-white px-2 text-[12px] text-zinc-800 outline-none focus:border-zinc-300 focus:ring-1 focus:ring-zinc-300 dark:border-white/[0.07] dark:bg-white/[0.04] dark:text-white/85 dark:focus:border-white/15 dark:focus:ring-white/15"
                    />
                </label>
                <label className="flex flex-col gap-1">
                    <span className="text-[9px] text-zinc-400 dark:text-white/30">备注</span>
                    <input
                        type="text"
                        value={note}
                        maxLength={500}
                        onChange={(e) => setNote(e.target.value)}
                        placeholder="例如：补扣未识别转出"
                        className="h-8 rounded-lg border border-zinc-200 bg-white px-2 text-[12px] text-zinc-800 outline-none focus:border-zinc-300 focus:ring-1 focus:ring-zinc-300 dark:border-white/[0.07] dark:bg-white/[0.04] dark:text-white/85 dark:placeholder-white/25 dark:focus:border-white/15 dark:focus:ring-white/15"
                    />
                </label>
            </div>

            {error ? <div className="mt-2 rounded-lg bg-red-500/[0.06] px-2 py-1.5 text-[10px] font-medium text-red-600 dark:text-red-300">{error}</div> : null}

            <div className="mt-2 flex justify-end gap-1.5">
                <button
                    type="button"
                    disabled={saving}
                    onClick={() => onSave?.(entry.day, Number(manualValue || 0), note)}
                    className={`inline-flex items-center gap-1 rounded-full px-2.5 py-1 text-[10px] font-semibold transition active:scale-95 disabled:opacity-50 ${brand.softButtonClass}`}
                >
                    <CheckCircle2 className="h-3 w-3" />
                    保存
                </button>
                <button
                    type="button"
                    disabled={saving}
                    onClick={() => onClear?.(entry.day)}
                    className="inline-flex items-center gap-1 rounded-full bg-zinc-100 px-2.5 py-1 text-[10px] font-semibold text-zinc-500 ring-1 ring-zinc-200 transition active:scale-95 disabled:opacity-50 dark:bg-white/[0.04] dark:text-white/50 dark:ring-white/[0.06]"
                >
                    <Eraser className="h-3 w-3" />
                    清除
                </button>
            </div>
        </div>
    );
}

function ProfitBaselineEditor({ baseline, latestDay = '', brand, saving = false, error = '', onSave, onClear }) {
    const [day, setDay] = useState('');
    const [baseValue, setBaseValue] = useState('');
    const [note, setNote] = useState('');

    useEffect(() => {
        setDay(baseline?.day || latestDay || '');
        setBaseValue(baseline ? String(Number(baseline.base_pnl_usd || 0)) : '0');
        setNote(baseline ? String(baseline.note || '') : '');
    }, [baseline, latestDay]);

    return (
        <div className="mt-2.5 rounded-xl border border-zinc-100 bg-white p-2.5 ring-1 ring-zinc-200/70 dark:border-white/[0.05] dark:bg-white/[0.03] dark:ring-white/[0.06]">
            <div className="flex items-start justify-between gap-2">
                <div className="min-w-0">
                    <div className="text-[11px] font-bold text-zinc-900 dark:text-white/90">总盈利起点</div>
                    <div className="mt-0.5 text-[9px] leading-snug text-zinc-400 dark:text-white/30">
                        {baseline ? `${baseline.day} 起点 ${formatUsd(baseline.base_pnl_usd)}` : '未设置起点，曲线从已返回日盈亏累加'}
                    </div>
                </div>
                {baseline ? (
                    <span className="shrink-0 rounded-full bg-zinc-100 px-2 py-0.5 text-[9px] text-zinc-400 ring-1 ring-zinc-200 dark:bg-white/[0.04] dark:text-white/35 dark:ring-white/[0.06]">
                        {formatChinaTime(baseline.updated_at)}
                    </span>
                ) : null}
            </div>

            <div className="mt-2 grid grid-cols-1 gap-1.5 sm:grid-cols-3">
                <label className="flex flex-col gap-1">
                    <span className="text-[9px] text-zinc-400 dark:text-white/30">起点日期</span>
                    <input
                        type="date"
                        value={day}
                        onChange={(e) => setDay(e.target.value)}
                        className="h-8 rounded-lg border border-zinc-200 bg-white px-2 text-[12px] text-zinc-800 outline-none focus:border-zinc-300 focus:ring-1 focus:ring-zinc-300 dark:border-white/[0.07] dark:bg-white/[0.04] dark:text-white/85 dark:focus:border-white/15 dark:focus:ring-white/15"
                    />
                </label>
                <label className="flex flex-col gap-1">
                    <span className="text-[9px] text-zinc-400 dark:text-white/30">起点盈利 USD</span>
                    <input
                        type="number"
                        step="0.01"
                        value={baseValue}
                        onChange={(e) => setBaseValue(e.target.value)}
                        className="h-8 rounded-lg border border-zinc-200 bg-white px-2 text-[12px] text-zinc-800 outline-none focus:border-zinc-300 focus:ring-1 focus:ring-zinc-300 dark:border-white/[0.07] dark:bg-white/[0.04] dark:text-white/85 dark:focus:border-white/15 dark:focus:ring-white/15"
                    />
                </label>
                <label className="flex flex-col gap-1">
                    <span className="text-[9px] text-zinc-400 dark:text-white/30">备注</span>
                    <input
                        type="text"
                        value={note}
                        maxLength={500}
                        onChange={(e) => setNote(e.target.value)}
                        placeholder="例如：旧钱包迁移"
                        className="h-8 rounded-lg border border-zinc-200 bg-white px-2 text-[12px] text-zinc-800 outline-none focus:border-zinc-300 focus:ring-1 focus:ring-zinc-300 dark:border-white/[0.07] dark:bg-white/[0.04] dark:text-white/85 dark:placeholder-white/25 dark:focus:border-white/15 dark:focus:ring-white/15"
                    />
                </label>
            </div>

            {error ? <div className="mt-2 rounded-lg bg-red-500/[0.06] px-2 py-1.5 text-[10px] font-medium text-red-600 dark:text-red-300">{error}</div> : null}

            <div className="mt-2 flex justify-end gap-1.5">
                <button
                    type="button"
                    disabled={saving}
                    onClick={() => onSave?.(day, Number(baseValue || 0), note)}
                    className={`inline-flex items-center gap-1 rounded-full px-2.5 py-1 text-[10px] font-semibold transition active:scale-95 disabled:opacity-50 ${brand.softButtonClass}`}
                >
                    <CheckCircle2 className="h-3 w-3" />
                    保存起点
                </button>
                <button
                    type="button"
                    disabled={saving}
                    onClick={() => onClear?.()}
                    className="inline-flex items-center gap-1 rounded-full bg-zinc-100 px-2.5 py-1 text-[10px] font-semibold text-zinc-500 ring-1 ring-zinc-200 transition active:scale-95 disabled:opacity-50 dark:bg-white/[0.04] dark:text-white/50 dark:ring-white/[0.06]"
                >
                    <Eraser className="h-3 w-3" />
                    清除
                </button>
            </div>
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
    moduleAccess,
    onNotice,
}) {
    const brand = useMemo(() => getBrandTheme(accentTheme), [accentTheme]);
    const tabs = useMemo(() => {
        const access = moduleAccess && typeof moduleAccess === 'object' ? moduleAccess : {};
        return [
            { key: 'my_assets', label: '我的资产', icon: Wallet, allowed: access.assets !== false },
            { key: 'global_config', label: '全局配置', icon: Settings2, allowed: access.global_config !== false },
            { key: 'wallet_manage', label: '钱包管理', icon: Wallet, allowed: access.wallet_manage !== false },
            { key: 'trade_history', label: '交易记录', icon: History, allowed: access.trade_history !== false },
        ].filter((tab) => tab.allowed);
    }, [moduleAccess]);
    const [activeTab, setActiveTab] = useState('my_assets');

    useEffect(() => {
        if (tabs.some((tab) => tab.key === activeTab)) return;
        setActiveTab(tabs[0]?.key || 'my_assets');
    }, [activeTab, tabs]);

    const [historyDays, setHistoryDays] = useState(30);
    const [trendMode, setTrendMode] = useState('assets');
    const [pnlCalendarWindow, setPnlCalendarWindow] = useState('month');
    const [assetsData, setAssetsData] = useState({ overview: null, history: null, lp: null });
    const [assetsLoading, setAssetsLoading] = useState(false);
    const [assetsRefreshing, setAssetsRefreshing] = useState(false);
    const [assetsError, setAssetsError] = useState('');
    const [selectedPnLDay, setSelectedPnLDay] = useState('');
    const [pnlAdjustmentSaving, setPnlAdjustmentSaving] = useState(false);
    const [pnlAdjustmentError, setPnlAdjustmentError] = useState('');
    const [profitBaselineSaving, setProfitBaselineSaving] = useState(false);
    const [profitBaselineError, setProfitBaselineError] = useState('');
    const [showProfitBaselineEditor, setShowProfitBaselineEditor] = useState(false);

    const hasAssetData = Boolean(assetsData.overview || assetsData.history || assetsData.lp);
    const hasAssetDataRef = useRef(false);

    useEffect(() => {
        hasAssetDataRef.current = hasAssetData;
    }, [hasAssetData]);


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
        loadAssets();
        if (!hasInitData) return undefined;
        const timer = setInterval(() => loadAssets(), Math.max(60, Number(pollIntervalSec || 15)) * 1000);
        return () => clearInterval(timer);
    }, [hasInitData, loadAssets, pollIntervalSec]);

    const chartRows = useMemo(() => seriesRows(assetsData.history, 'total_usd'), [assetsData.history]);
    const profitCurveRows = useMemo(() => {
        const rows = Array.isArray(assetsData.lp?.profit_curve) ? assetsData.lp.profit_curve : [];
        return rows.map((item) => ({ day: item.day, value: Number(item?.value_usd || 0), close: Number(item?.value_usd || 0) }));
    }, [assetsData.lp]);
    const profitTrendRows = useMemo(() => filterRowsByWindow(profitCurveRows, historyDays), [profitCurveRows, historyDays]);
    const activeTrendRows = trendMode === 'profit' ? profitTrendRows : chartRows;
    const activeTrendValue = activeTrendRows[activeTrendRows.length - 1]?.value;
    const latestProfitCurveDay = profitCurveRows[profitCurveRows.length - 1]?.day || formatChinaDay();
    const pnlCalendarAllRows = useMemo(() => mergeDailyPoints(assetsData.lp?.daily_history, assetsData.lp?.today_point), [assetsData.lp]);
    const pnlCalendarRows = useMemo(() => filterPnLCalendarRows(pnlCalendarAllRows, pnlCalendarWindow), [pnlCalendarAllRows, pnlCalendarWindow]);
    const pnlCalendarSummary = useMemo(() => summarizePnLCalendarRows(pnlCalendarRows), [pnlCalendarRows]);
    const pnlWindowLabel = PNL_CALENDAR_WINDOWS.find((item) => item.key === pnlCalendarWindow)?.label || '本月';

    const isLoading = assetsLoading || assetsRefreshing;
    const canManualRefresh = hasInitData;
    const selectedPnLEntry = useMemo(() => {
        if (!selectedPnLDay) return null;
        const rows = Array.isArray(assetsData.lp?.daily_history) ? assetsData.lp.daily_history : [];
        const matched = rows.find((item) => item?.day === selectedPnLDay);
        if (matched) return matched;
        if (assetsData.lp?.today_point?.day === selectedPnLDay) return assetsData.lp.today_point;
        return null;
    }, [assetsData.lp, selectedPnLDay]);

    useEffect(() => {
        if (!selectedPnLDay) return undefined;
        const handlePointerDown = (event) => {
            const target = event.target;
            if (!(target instanceof Element)) return;
            const dateCell = target.closest('[data-pnl-date-cell="true"]');
            if (dateCell?.getAttribute('data-pnl-selectable') === 'true') return;
            if (target.closest('[data-pnl-calendar-keep="true"]')) return;
            setSelectedPnLDay('');
            setPnlAdjustmentError('');
        };
        document.addEventListener('pointerdown', handlePointerDown);
        return () => {
            document.removeEventListener('pointerdown', handlePointerDown);
        };
    }, [selectedPnLDay]);

    useEffect(() => {
        if (selectedPnLDay && assetsData.lp && !selectedPnLEntry) {
            setSelectedPnLDay('');
        }
    }, [assetsData.lp, selectedPnLEntry, selectedPnLDay]);

    useEffect(() => {
        setSelectedPnLDay('');
        setPnlAdjustmentError('');
    }, [pnlCalendarWindow]);

    const handleSavePnLAdjustment = useCallback(async (day, manualAdjustmentUsd, note) => {
        setPnlAdjustmentSaving(true);
        setPnlAdjustmentError('');
        try {
            await saveAssetLPPnLAdjustment({
                apiBaseUrl,
                initData,
                day,
                manualAdjustmentUsd,
                note,
            });
            await loadAssets({ forceRefresh: true });
            setSelectedPnLDay(day);
        } catch (err) {
            setPnlAdjustmentError(errorText(err) || '保存失败');
        } finally {
            setPnlAdjustmentSaving(false);
        }
    }, [apiBaseUrl, initData, loadAssets]);

    const handleClearPnLAdjustment = useCallback(async (day) => {
        setPnlAdjustmentSaving(true);
        setPnlAdjustmentError('');
        try {
            await clearAssetLPPnLAdjustment({
                apiBaseUrl,
                initData,
                day,
            });
            await loadAssets({ forceRefresh: true });
            setSelectedPnLDay(day);
        } catch (err) {
            setPnlAdjustmentError(errorText(err) || '清除失败');
        } finally {
            setPnlAdjustmentSaving(false);
        }
    }, [apiBaseUrl, initData, loadAssets]);

    const handleSaveProfitBaseline = useCallback(async (day, basePnlUsd, note) => {
        setProfitBaselineSaving(true);
        setProfitBaselineError('');
        try {
            await saveAssetLPPnLBaseline({
                apiBaseUrl,
                initData,
                day,
                basePnlUsd,
                note,
            });
            await loadAssets({ forceRefresh: true });
            setTrendMode('profit');
            setShowProfitBaselineEditor(false);
        } catch (err) {
            setProfitBaselineError(errorText(err) || '保存失败');
        } finally {
            setProfitBaselineSaving(false);
        }
    }, [apiBaseUrl, initData, loadAssets]);

    const handleClearProfitBaseline = useCallback(async () => {
        setProfitBaselineSaving(true);
        setProfitBaselineError('');
        try {
            await clearAssetLPPnLBaseline({
                apiBaseUrl,
                initData,
            });
            await loadAssets({ forceRefresh: true });
            setTrendMode('profit');
            setShowProfitBaselineEditor(false);
        } catch (err) {
            setProfitBaselineError(errorText(err) || '清除失败');
        } finally {
            setProfitBaselineSaving(false);
        }
    }, [apiBaseUrl, initData, loadAssets]);

    useEffect(() => {
        if (trendMode !== 'profit' && showProfitBaselineEditor) {
            setShowProfitBaselineEditor(false);
        }
    }, [showProfitBaselineEditor, trendMode]);

    return (
        <div className="mini-am-page flex flex-col gap-3">
            {/* ══════ Header ══════ */}
            <Card className="!p-0 overflow-hidden">
                <div className="px-3.5 pt-3 pb-2.5">
                    <div className="flex items-center justify-between gap-3">
                        <div className="min-w-0">
                            <div className="text-[14px] font-extrabold leading-tight text-zinc-900 dark:text-white/95">我的</div>
                            <div className="mt-0.5 text-[10px] text-zinc-500 dark:text-white/40">
                                {hasInitData ? tabs.map((tab) => tab.label).join(' / ') : '需要有效的 Telegram initData'}
                            </div>
                        </div>
                        {activeTab === 'my_assets' ? (
                            <button
                                type="button"
                                onClick={() => loadAssets({ forceRefresh: true })}
                                disabled={!canManualRefresh || isLoading}
                                className="inline-flex h-7 w-7 items-center justify-center rounded-lg border border-zinc-200/80 bg-zinc-50 text-zinc-500 transition active:scale-95 dark:border-white/5 dark:bg-[#1c2026] dark:text-white/50 dark:hover:bg-white/5 disabled:opacity-40"
                            >
                                <RefreshCw className={`h-3.5 w-3.5 ${isLoading ? 'animate-spin' : ''}`} />
                            </button>
                        ) : null}
                    </div>
                </div>
                <div
                    className="mini-am-tabs grid border-t border-zinc-100 dark:border-white/[0.04]"
                    style={{ gridTemplateColumns: `repeat(${Math.max(tabs.length, 1)}, minmax(0, 1fr))` }}
                >
                    {tabs.map((tab) => {
                        const Icon = tab.icon;
                        const active = activeTab === tab.key;
                        return (
                            <button
                                key={tab.key}
                                type="button"
                                onClick={() => setActiveTab(tab.key)}
                                className={`relative flex flex-1 flex-col items-center justify-center gap-1 px-1.5 py-2.5 text-[10px] font-bold transition sm:flex-row sm:text-[11px] ${
                                    active
                                        ? `${brand.textClass}`
                                        : 'text-zinc-400 hover:text-zinc-600 dark:text-white/35 dark:hover:text-white/60'
                                }`}
                            >
                                <Icon className="h-3.5 w-3.5" />
                                <span className="truncate">{tab.label}</span>
                                {active ? (
                                    <span className={`absolute bottom-0 left-1/2 h-[2px] w-6 -translate-x-1/2 rounded-full ${brand.dotClass}`} />
                                ) : null}
                            </button>
                        );
                    })}
                </div>
            </Card>

            {activeTab === 'my_assets' ? (
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
                    <div className="mini-am-stats grid grid-cols-2 gap-2">
                        <StatBlock label="总资产" value={formatUsd(assetsData.overview?.summary?.total_usd)} tone="accent" />
                        <StatBlock label="钱包余额" value={formatUsd(assetsData.overview?.summary?.wallet_usd)} />
                        <StatBlock label="LP 持仓" value={formatUsd(assetsData.overview?.summary?.position_usd)} />
                        <StatBlock label="未领取手续费" value={formatUsd(assetsData.overview?.summary?.fee_usd)} tone="warn" />
                    </div>

                    {/* trend chart */}
                    <Card>
                        <div className="flex flex-wrap items-center justify-between gap-2">
                            <span className="text-[12px] font-bold text-zinc-900 dark:text-white/90">{trendMode === 'profit' ? '总盈利趋势' : '总资产趋势'}</span>
                            <div className="flex flex-wrap justify-end gap-1.5">
                                <div className="flex gap-1.5">
                                    <Pill active={trendMode === 'assets'} brand={brand} onClick={() => setTrendMode('assets')}>总资产</Pill>
                                    <Pill active={trendMode === 'profit'} brand={brand} onClick={() => setTrendMode('profit')}>总盈利</Pill>
                                </div>
                                <div className="flex gap-1.5">
                                    {HISTORY_WINDOWS.map((d) => (
                                        <Pill key={d} active={historyDays === d} brand={brand} onClick={() => setHistoryDays(d)}>{d}D</Pill>
                                    ))}
                                </div>
                            </div>
                        </div>
                        <div className="mt-3 rounded-xl border border-zinc-100 bg-zinc-50/60 p-3 dark:border-white/[0.04] dark:bg-[#0f1116]">
                            <div className="flex items-end justify-between gap-2">
                                <div>
                                    <div className="text-[9px] font-medium uppercase tracking-wide text-zinc-400 dark:text-white/35">
                                        {trendMode === 'profit' ? '总盈利' : '总资产'}
                                    </div>
                                    <div className="mt-1 text-xl font-extrabold tabular-nums text-zinc-900 dark:text-white leading-none">
                                        <NumberFlowValue value={activeTrendValue || 0} formatter={(v) => formatUsd(v)} />
                                    </div>
                                </div>
                                <div className="flex shrink-0 items-center gap-1.5">
                                    <span className="text-[10px] text-zinc-400 dark:text-white/30">
                                        {trendMode === 'profit' && assetsData.lp?.profit_baseline ? `起点 ${assetsData.lp.profit_baseline.day}` : formatChinaTime(assetsData.overview?.updated_at)}
                                    </span>
                                    {trendMode === 'profit' ? (
                                        <button
                                            type="button"
                                            onClick={() => setShowProfitBaselineEditor((prev) => !prev)}
                                            className={`inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-semibold ring-1 transition active:scale-95 ${
                                                showProfitBaselineEditor
                                                    ? brand.softButtonClass
                                                    : 'bg-zinc-100 text-zinc-500 ring-zinc-200 dark:bg-white/[0.04] dark:text-white/50 dark:ring-white/[0.06]'
                                            }`}
                                        >
                                            <Settings2 className="h-3 w-3" />
                                            起点
                                        </button>
                                    ) : null}
                                </div>
                            </div>
                            <div className="mt-3">
                                <LWAreaChart data={activeTrendRows} color={trendMode === 'profit' ? '#0ea5e9' : '#10b981'} loading={assetsLoading} />
                            </div>
                        </div>
                        {trendMode === 'profit' && showProfitBaselineEditor ? (
                            <ProfitBaselineEditor
                                baseline={assetsData.lp?.profit_baseline}
                                latestDay={latestProfitCurveDay}
                                brand={brand}
                                saving={profitBaselineSaving}
                                error={profitBaselineError}
                                onSave={handleSaveProfitBaseline}
                                onClear={handleClearProfitBaseline}
                            />
                        ) : null}
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
                    </Card>

                    {/* LP daily PnL calendar */}
                    <Card>
                        <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
                            <div>
                                <div className="text-[12px] font-bold text-zinc-900 dark:text-white/90">盈亏日历</div>
                                <div className="mt-0.5 text-[10px] text-zinc-500 dark:text-white/40">
                                    {pnlWindowLabel}已记录 {pnlCalendarRows.length} 天
                                </div>
                            </div>
                            <div className="flex gap-1.5" data-pnl-calendar-keep="true">
                                {PNL_CALENDAR_WINDOWS.map((item) => (
                                    <Pill
                                        key={item.key}
                                        active={pnlCalendarWindow === item.key}
                                        brand={brand}
                                        onClick={() => setPnlCalendarWindow(item.key)}
                                    >
                                        {item.label}
                                    </Pill>
                                ))}
                            </div>
                        </div>
                        <div className="mb-3 grid grid-cols-3 gap-1.5">
                            <div className="rounded-lg bg-zinc-50 px-2.5 py-2 ring-1 ring-zinc-200 dark:bg-white/[0.03] dark:ring-white/[0.06]">
                                <div className="text-[9px] text-zinc-400 dark:text-white/30">{pnlWindowLabel}盈亏</div>
                                <div className={`mt-0.5 text-[12px] font-bold tabular-nums ${pnlCalendarSummary.total >= 0 ? 'text-emerald-600 dark:text-emerald-300' : 'text-red-600 dark:text-red-300'}`}>
                                    {pnlCalendarSummary.total >= 0 ? '+' : ''}{formatUsdCompact(pnlCalendarSummary.total)}
                                </div>
                            </div>
                            <div className="rounded-lg bg-emerald-500/[0.06] px-2.5 py-2 ring-1 ring-emerald-500/15 dark:bg-emerald-500/[0.08]">
                                <div className="text-[9px] text-emerald-600 dark:text-emerald-400">盈利日</div>
                                <div className="mt-0.5 text-[12px] font-bold tabular-nums text-emerald-700 dark:text-emerald-300">{pnlCalendarSummary.positiveDays}</div>
                            </div>
                            <div className="rounded-lg bg-red-500/[0.06] px-2.5 py-2 ring-1 ring-red-500/15 dark:bg-red-500/[0.08]">
                                <div className="text-[9px] text-red-600 dark:text-red-400">亏损日</div>
                                <div className="mt-0.5 text-[12px] font-bold tabular-nums text-red-700 dark:text-red-300">{pnlCalendarSummary.negativeDays}</div>
                            </div>
                        </div>
                        <PnLCalendar
                            data={pnlCalendarRows}
                            loading={assetsLoading}
                            allowNavigation={pnlCalendarWindow === '30d'}
                            note="日历默认按每日资产快照差额计算；如当天有充值、提现等外部资金变化，可点选日期手动校准。蓝点代表有转账提示，金点代表有手动校准。"
                            selectedDay={selectedPnLDay}
                            onSelectDay={(entry) => {
                                setSelectedPnLDay(entry?.day || '');
                                setPnlAdjustmentError('');
                            }}
                            onDismiss={() => {
                                setSelectedPnLDay('');
                                setPnlAdjustmentError('');
                            }}
                        />
                        {selectedPnLEntry ? (
                        <PnLBreakdownEditor
                            entry={selectedPnLEntry}
                            brand={brand}
                            saving={pnlAdjustmentSaving}
                            error={pnlAdjustmentError}
                            onSave={handleSavePnLAdjustment}
                            onClear={handleClearPnLAdjustment}
                        />
                        ) : null}
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
            ) : activeTab === 'global_config' ? (
            <div className="space-y-3">
                <GlobalConfigPage
                    embedded
                    open
                    onClose={() => setActiveTab('my_assets')}
                    apiBaseUrl={apiBaseUrl}
                    initData={initData}
                    accentTheme={accentTheme}
                    onConfigChanged={onNotice ? () => onNotice('全局配置已保存', 'success') : undefined}
                />
            </div>
            ) : activeTab === 'wallet_manage' ? (
            <div className="space-y-3">
                <WalletManagePage
                    embedded
                    open
                    onClose={() => setActiveTab('my_assets')}
                    apiBaseUrl={apiBaseUrl}
                    initData={initData}
                    accentTheme={accentTheme}
                />
            </div>
            ) : (
            <div className="space-y-3">
                <TradeHistoryPage
                    embedded
                    open
                    onClose={() => setActiveTab('my_assets')}
                    apiBaseUrl={apiBaseUrl}
                    initData={initData}
                />
            </div>
            )}
        </div>
    );
}
