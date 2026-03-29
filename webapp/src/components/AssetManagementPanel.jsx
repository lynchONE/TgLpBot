import React, { startTransition, useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { createChart, AreaSeries, HistogramSeries, ColorType } from 'lightweight-charts';
import {
  Activity,
  AlertTriangle,
  ArrowRightLeft,
  Ban,
  CheckCircle2,
  ChevronLeft,
  ChevronRight,
  ClipboardList,
  Crown,
  Medal,
  Plus,
  RefreshCw,
  Search,
  Server,
  Settings2,
  Shield,
  Trash2,
  TrendingUp,
  Trophy,
  Users,
  Wallet,
  Zap,
} from 'lucide-react';
import {
  addAdminRPCEndpoint,
  checkAdminRPCEndpoint,
  deleteAdminRPCEndpoint,
  disableAdminRPCEndpointNextMonth,
  enableAdminRPCEndpoint,
  fetchAdminActiveTasks,
  fetchAdminOnlineUsers,
  fetchAdminPrivateZap,
  fetchAdminRPCPool,
  fetchAdminRealtimePositions,
  fetchAdminSmartMoneyLeaderboard,
  fetchAdminSmartMoneyOverview,
  fetchAdminSmartMoneyWallet,
  fetchAssetHistory,
  fetchAssetLPStats,
  fetchAssetOverview,
  fetchSystemConfig,
  invalidateAdminPrivateZap,
  renameAdminRPCEndpoint,
  switchAdminRPCEndpoint,
  updateSystemConfig,
} from '../api';
import { resolveSMAvatarAssetUrl } from '../smartMoneyApi';
import PanelShell, { EmptyState, MetricCard } from './PanelShell';

const HISTORY_WINDOWS = [7, 30, 90];
const CHINA_TIME_ZONE = 'Asia/Shanghai';
const HISTORY_METRICS = [
  { key: 'total_usd', label: '总资产', color: '#59f09d' },
  { key: 'wallet_usd', label: '钱包余额', color: '#52d1ff' },
  { key: 'position_usd', label: 'LP 持仓', color: '#c792ff' },
  { key: 'fee_usd', label: '手续费', color: '#ffae42' },
];
const SMART_MONEY_WINDOWS = [1, 7, 30];
const LEADERBOARD_METRICS = [
  { key: 'pnl', label: '收益额' },
  { key: 'yield_rate', label: '收益率' },
  { key: 'participation', label: '参与次数' },
];

function formatUsd(value) {
  const num = Number(value || 0);
  if (!Number.isFinite(num)) return '$--';
  return new Intl.NumberFormat('en-US', { style: 'currency', currency: 'USD', maximumFractionDigits: 2 }).format(num);
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
  return Boolean(
    item?.has_transfer_in ||
    item?.has_transfer_out ||
    Number(item?.transfer_total_count || 0) > 0 ||
    Number(item?.transfer_in_count || 0) > 0 ||
    Number(item?.transfer_out_count || 0) > 0 ||
    Number(item?.transfer_net_usd || 0) !== 0 ||
    Number(item?.transfer_in_usd || 0) > 0 ||
    Number(item?.transfer_out_usd || 0) > 0
  );
}

function TransferBadges({ item, compact = false }) {
  const totalCount = Number(item?.transfer_total_count || 0) || (Number(item?.transfer_in_count || 0) + Number(item?.transfer_out_count || 0));
  const inCount = Number(item?.transfer_in_count || 0);
  const outCount = Number(item?.transfer_out_count || 0);
  const inUsd = Number(item?.transfer_in_usd || 0);
  const outUsd = Number(item?.transfer_out_usd || 0);
  const rawNetUsd = item?.transfer_net_usd !== undefined && item?.transfer_net_usd !== null
    ? Number(item.transfer_net_usd || 0)
    : (inUsd - outUsd);
  if (!hasTransferMarker(item)) return null;
  const absNetUsd = Math.abs(rawNetUsd);
  const isNetIn = rawNetUsd > 0;
  const isNetOut = rawNetUsd < 0;
  const badge = {
    key: isNetIn ? 'net-in' : isNetOut ? 'net-out' : 'net-flat',
    color: isNetIn ? '#16a34a' : isNetOut ? '#ea580c' : '#0891b2',
    background: isNetIn ? 'rgba(22, 163, 74, 0.12)' : isNetOut ? 'rgba(234, 88, 12, 0.12)' : 'rgba(8, 145, 178, 0.12)',
    border: isNetIn ? 'rgba(22, 163, 74, 0.22)' : isNetOut ? 'rgba(234, 88, 12, 0.22)' : 'rgba(8, 145, 178, 0.22)',
  };
  const amountText = absNetUsd > 0 ? formatUsdCompact(absNetUsd) : '';
  badge.label = compact
    ? (absNetUsd > 0 ? `${isNetOut ? '-' : '+'}${amountText.replace('$', '')}` : (totalCount > 0 ? `${totalCount}笔` : '转账'))
    : `${isNetIn ? '净转入' : isNetOut ? '净转出' : '净转账'}${amountText ? ` ${amountText}` : ''}${totalCount > 0 ? ` · ${totalCount}笔` : ''}`;
  return (
    <div style={{ display: 'flex', alignItems: 'center', justifyContent: compact ? 'center' : 'flex-start', gap: 4, flexWrap: 'wrap', minHeight: compact ? 12 : 'auto' }}>
      <span
        key={badge.key}
        style={{
          display: 'inline-flex',
          alignItems: 'center',
          justifyContent: 'center',
          minWidth: compact ? 14 : 0,
          padding: compact ? '0 4px' : '1px 6px',
          borderRadius: 999,
          border: `1px solid ${badge.border}`,
          background: badge.background,
          color: badge.color,
          fontSize: compact ? 9 : 10,
          fontWeight: 700,
          lineHeight: compact ? '12px' : '14px',
        }}
      >
        {badge.label}
      </span>
    </div>
  );
}

function isIgnorableSmartMoneyDataError(err) {
  const message = errorText(err).toLowerCase();
  return message.includes("unknown column 'open_lp_usd'") || message.includes("unknown column `open_lp_usd`");
}

/* ─── Wallet Avatar (address-based icon image) ─── */
const AVATAR_URLS = Object.entries(
  import.meta.glob('../icon/avatar_*.png', { eager: true, import: 'default' })
).sort(([a], [b]) => a.localeCompare(b, undefined, { numeric: true })).map(([, src]) => src);

function walletAvatarUrl(address) {
  if (!AVATAR_URLS.length) return '';
  const hex = String(address || '').toLowerCase();
  let hash = 0;
  for (let i = 0; i < hex.length; i++) hash = ((hash << 5) - hash + hex.charCodeAt(i)) | 0;
  return AVATAR_URLS[Math.abs(hash) % AVATAR_URLS.length] || AVATAR_URLS[0] || '';
}

function WalletAvatar({ address, size = 28, avatarUrl }) {
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
      style={{ width: size, height: size, flexShrink: 0, borderRadius: size * 0.22, objectFit: 'cover' }}
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
  const base = { display: 'inline-flex', alignItems: 'center', justifyContent: 'center', width: 28, height: 28, borderRadius: '50%', flexShrink: 0 };
  if (rank === 1) return <span style={{ ...base, background: 'linear-gradient(135deg, #facc15, #f59e0b)', boxShadow: '0 1px 4px rgba(245,158,11,0.3)' }}><Trophy size={14} color="#fff" /></span>;
  if (rank === 2) return <span style={{ ...base, background: 'linear-gradient(135deg, #cbd5e1, #94a3b8)', boxShadow: '0 1px 4px rgba(148,163,184,0.3)' }}><Medal size={14} color="#fff" /></span>;
  if (rank === 3) return <span style={{ ...base, background: 'linear-gradient(135deg, #d97706, #92400e)', boxShadow: '0 1px 4px rgba(146,64,14,0.3)' }}><Medal size={14} color="#fff" /></span>;
  return <span style={{ ...base, background: 'rgba(136,157,191,0.1)', color: 'rgba(136,157,191,0.7)', fontSize: 11, fontWeight: 700 }}>{rank}</span>;
}

/* ─── TradingView Area Chart (lightweight-charts v5) ─── */
function LWAreaChart({ points, stroke = '#52d1ff', height = 220 }) {
  const containerRef = useRef(null);
  const chartRef = useRef(null);
  const seriesRef = useRef(null);

  useEffect(() => {
    if (!containerRef.current) return;
    const chart = createChart(containerRef.current, {
      width: containerRef.current.clientWidth,
      height,
      layout: {
        background: { type: ColorType.Solid, color: 'transparent' },
        textColor: 'rgba(154, 168, 196, 0.6)',
        fontFamily: "'Space Grotesk', system-ui, sans-serif",
        fontSize: 10,
      },
      grid: {
        vertLines: { color: 'rgba(134, 153, 184, 0.06)' },
        horzLines: { color: 'rgba(134, 153, 184, 0.06)' },
      },
      rightPriceScale: {
        borderVisible: false,
        scaleMargins: { top: 0.08, bottom: 0.05 },
      },
      timeScale: {
        borderVisible: false,
        fixLeftEdge: true,
        fixRightEdge: true,
        timeVisible: false,
      },
      crosshair: {
        horzLine: { color: 'rgba(154, 168, 196, 0.2)', style: 2 },
        vertLine: { color: 'rgba(154, 168, 196, 0.2)', style: 2 },
      },
      handleScroll: false,
      handleScale: false,
    });

    const series = chart.addSeries(AreaSeries, {
      lineColor: stroke,
      lineWidth: 2,
      topColor: `${stroke}40`,
      bottomColor: `${stroke}05`,
      priceFormat: {
        type: 'custom',
        formatter: (v) => {
          const abs = Math.abs(v);
          if (abs >= 1000000) return `$${(v / 1000000).toFixed(1)}M`;
          if (abs >= 1000) return `$${(v / 1000).toFixed(abs >= 10000 ? 0 : 1)}K`;
          return `$${v.toFixed(0)}`;
        },
      },
      crosshairMarkerRadius: 4,
      crosshairMarkerBorderColor: stroke,
      crosshairMarkerBackgroundColor: '#070b11',
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
  }, [stroke, height]);

  useEffect(() => {
    if (!seriesRef.current || !points || points.length < 1) return;
    const mapped = points
      .filter((d) => d.day && Number.isFinite(Number(d.value)))
      .map((d) => ({ time: d.day, value: Number(d.value) }));
    seriesRef.current.setData(mapped);
    chartRef.current?.timeScale().fitContent();
  }, [points]);

  return (
    <div style={{ position: 'relative', minHeight: height }}>
      <div ref={containerRef} style={{ minHeight: height, borderRadius: 'var(--radius-md)', overflow: 'hidden' }} />
      {(!points || points.length < 2) && (
        <div className="am-chart-empty" style={{ position: 'absolute', inset: 0, zIndex: 1 }}>暂无趋势数据</div>
      )}
    </div>
  );
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

  const monthLabel = new Intl.DateTimeFormat('en-US', {
    timeZone: CHINA_TIME_ZONE,
    year: 'numeric',
    month: 'short',
  }).format(new Date(Date.UTC(year, month, 1, 12, 0, 0)));
  const prevMonth = () => setViewDate(new Date(year, month - 1, 1));
  const nextMonth = () => setViewDate(new Date(year, month + 1, 1));
  const todayStr = formatChinaDay();

  if (loading) {
    return <div className="am-chart-empty">加载中...</div>;
  }

  const cells = [];
  for (let i = 0; i < startOffset; i++) {
    cells.push(<div key={`e-${i}`} className="pnl-cal-cell pnl-cal-empty" />);
  }
  for (let day = 1; day <= daysInMonth; day++) {
    const dateStr = `${year}-${String(month + 1).padStart(2, '0')}-${String(day).padStart(2, '0')}`;
    const entry = pnlMap[dateStr];
    const pnl = entry ? Number(entry.realized_pnl_usd || 0) : null;
    const hasTransfer = hasTransferMarker(entry);
    const isToday = dateStr === todayStr;
    const isFuture = dateStr > todayStr;
    const cls = ['pnl-cal-cell'];
    if (isToday) cls.push('pnl-cal-today');
    else if (isFuture) cls.push('pnl-cal-future');
    if (pnl !== null) cls.push(pnl >= 0 ? 'pnl-cal-pos' : 'pnl-cal-neg');
    cells.push(
      <div key={day} className={cls.join(' ')}>
        <div className="pnl-cal-day">{day}</div>
        <div className="pnl-cal-value">
          {pnl !== null ? `${pnl >= 0 ? '+' : ''}${formatUsdCompact(pnl)}` : '0'}
        </div>
        {hasTransfer ? <TransferBadges item={entry} compact /> : <div style={{ minHeight: 12 }} />}
      </div>
    );
  }
  const remainder = (startOffset + daysInMonth) % 7;
  if (remainder > 0) {
    for (let i = 0; i < 7 - remainder; i++) {
      cells.push(<div key={`t-${i}`} className="pnl-cal-cell pnl-cal-empty" />);
    }
  }

  return (
    <div className="pnl-calendar">
      <div className="pnl-cal-header">
        <div className="pnl-cal-month">
          <span>{monthLabel}</span>
          <svg width="14" height="14" fill="none" stroke="currentColor" strokeWidth="2" viewBox="0 0 24 24">
            <rect x="3" y="4" width="18" height="18" rx="2" /><line x1="16" y1="2" x2="16" y2="6" /><line x1="8" y1="2" x2="8" y2="6" /><line x1="3" y1="10" x2="21" y2="10" />
          </svg>
        </div>
        <div className="pnl-cal-nav">
          <button type="button" onClick={prevMonth}><ChevronLeft size={14} /></button>
          <button type="button" onClick={nextMonth}><ChevronRight size={14} /></button>
        </div>
      </div>
      <div className="pnl-cal-grid">
        {PNL_CAL_WEEKDAYS.map((d) => (
          <div key={d} className="pnl-cal-weekday">{d}</div>
        ))}
        {cells}
      </div>
      {note ? (
        <div style={{ display: 'flex', alignItems: 'flex-start', gap: 6, marginTop: 8, fontSize: 10, lineHeight: 1.45, color: 'var(--text-muted)' }}>
          <ArrowRightLeft size={12} style={{ flexShrink: 0, marginTop: 1, opacity: 0.7 }} />
          <span>{note}</span>
        </div>
      ) : null}
    </div>
  );
}

/* ─── Per-pool PnL overview for today ─── */
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

function PoolContributionRow({ row, maxAbsPnl }) {
  const pnl = Number(row?.profit_usd || 0);
  const ratio = Math.max(Math.abs(pnl) / Math.max(maxAbsPnl, 1), 0.08);
  const positive = pnl >= 0;

  return (
    <div className="am-pool-row">
      <div className="am-pool-row-head">
        <div className="am-pool-row-title">{row?.pair || '未命名池子'}</div>
        <span className={`am-badge ${positive ? 'am-badge-ok' : 'am-badge-warn'}`}>
          {positive ? '+' : ''}{formatUsdCompact(pnl)}
        </span>
      </div>
      <div className="am-pool-row-meta">
        <span>{String(row?.chain || 'BSC').toUpperCase()}</span>
        <span>{Number(row?.closed_count || 0)} 笔</span>
      </div>
      <div className="am-pool-bar-track">
        <div className={`am-pool-bar ${positive ? 'positive' : 'negative'}`} style={{ width: `${ratio * 100}%` }} />
      </div>
    </div>
  );
}

function TodayPoolPnL({ pools }) {
  const [view, setView] = useState('leaders');
  const summary = useMemo(() => summarizeTodayPools(pools), [pools]);
  const showDetailsTab = summary.remainingCount > 0 || summary.flatCount > 0;

  useEffect(() => {
    if (!showDetailsTab && view !== 'leaders') {
      setView('leaders');
    }
  }, [showDetailsTab, view]);

  if (!summary.rows.length) {
    return <EmptyState text="今日暂无平仓记录" />;
  }

  return (
    <div className="am-pool-card am-pool-card-compact">
      <div className="am-pool-toolbar">
        <div>
          <div className="am-card-title" style={{ fontSize: 12 }}>平仓池子贡献</div>
          <div className="am-item-sub" style={{ margin: 0 }}>仅统计今日平仓记录，不等于总资产快照盈亏</div>
        </div>
        <div className="am-pill-group">
          <button type="button" className={`am-pill ${view === 'leaders' ? 'active' : ''}`} onClick={() => setView('leaders')}>
            贡献榜
          </button>
          {showDetailsTab ? (
            <button type="button" className={`am-pill ${view === 'details' ? 'active' : ''}`} onClick={() => setView('details')}>
              全部明细
            </button>
          ) : null}
        </div>
      </div>

      <div className="am-pool-summary-grid">
        <div className="am-pool-summary">
          <div className="am-pool-summary-label">参与池子</div>
          <div className="am-pool-summary-value">{summary.rows.length}</div>
        </div>
        <div className="am-pool-summary">
          <div className="am-pool-summary-label">盈利池</div>
          <div className="am-pool-summary-value is-positive">{summary.positiveRows.length}</div>
        </div>
        <div className="am-pool-summary">
          <div className="am-pool-summary-label">亏损池</div>
          <div className="am-pool-summary-value is-negative">{summary.negativeRows.length}</div>
        </div>
        <div className="am-pool-summary">
          <div className="am-pool-summary-label">持平池</div>
          <div className="am-pool-summary-value">{summary.flatCount}</div>
        </div>
      </div>

      {view === 'leaders' ? (
        <>
          <div className="am-pool-board">
            <div className="am-pool-column">
              <div className="am-pool-section-head">
                <span className="am-pool-section-title">Top 盈利</span>
                <span className="am-pool-section-count">{summary.topPositive.length} 个</span>
              </div>
              {summary.topPositive.length > 0 ? summary.topPositive.map((row) => (
                <PoolContributionRow key={row.key} row={row} maxAbsPnl={summary.maxAbsPnl} />
              )) : <div className="am-pool-empty">今日暂无盈利池</div>}
            </div>

            <div className="am-pool-column">
              <div className="am-pool-section-head">
                <span className="am-pool-section-title">Top 亏损</span>
                <span className="am-pool-section-count">{summary.topNegative.length} 个</span>
              </div>
              {summary.topNegative.length > 0 ? summary.topNegative.map((row) => (
                <PoolContributionRow key={row.key} row={row} maxAbsPnl={summary.maxAbsPnl} />
              )) : <div className="am-pool-empty">今日暂无亏损池</div>}
            </div>
          </div>

          {showDetailsTab ? (
            <div className="am-pool-note">
              其余 {summary.remainingCount} 个池子已折叠，点“全部明细”查看完整列表。
            </div>
          ) : null}
        </>
      ) : (
        <div className="am-pool-details">
          {summary.rows.map((row) => (
            <PoolContributionRow key={row.key} row={row} maxAbsPnl={summary.maxAbsPnl} />
          ))}
        </div>
      )}
    </div>
  );
}

/* ─── Small sparkline for smart money wallet detail (kept simple) ─── */
function SparklineChart({ points, stroke = '#52d1ff' }) {
  const values = Array.isArray(points) ? points.map((item) => Number(item?.value || 0)).filter(Number.isFinite) : [];
  if (values.length < 2) {
    return <div className="am-chart-empty">暂无趋势数据</div>;
  }
  const min = Math.min(...values);
  const max = Math.max(...values);
  const range = max - min || 1;
  const d = values.map((value, index) => {
    const x = (index / (values.length - 1)) * 100;
    const y = 100 - ((value - min) / range) * 88 - 6;
    return `${index === 0 ? 'M' : 'L'} ${x} ${y}`;
  }).join(' ');
  const areaD = `${d} L 100 100 L 0 100 Z`;
  return (
    <svg className="am-chart" viewBox="0 0 100 100" preserveAspectRatio="none">
      <defs>
        <linearGradient id={`am-grad-${stroke.replace('#', '')}`} x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stopColor={stroke} stopOpacity="0.2" />
          <stop offset="100%" stopColor={stroke} stopOpacity="0" />
        </linearGradient>
      </defs>
      <path d={areaD} fill={`url(#am-grad-${stroke.replace('#', '')})`} />
      <path d={d} fill="none" stroke={stroke} strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  );
}

const WALLET_COLORS = ['#59f09d', '#52d1ff', '#c792ff', '#ffae42', '#ff6b9d', '#06d6a0', '#ffa630', '#84cc16'];

function DonutChart({ wallets }) {
  const items = useMemo(() => {
    if (!Array.isArray(wallets) || wallets.length === 0) return [];
    const merged = {};
    wallets.forEach((w) => {
      const addr = w.wallet_address || `#${w.wallet_id}`;
      if (!merged[addr]) merged[addr] = { addr, total: 0 };
      merged[addr].total += Math.max(0, Number(w.total_usd || 0));
    });
    return Object.values(merged).map((m, i) => ({
      label: m.addr.startsWith('#') ? m.addr : `${m.addr.slice(0, 6)}...${m.addr.slice(-4)}`,
      value: m.total,
      color: WALLET_COLORS[i % WALLET_COLORS.length],
    })).filter((i) => i.value > 0);
  }, [wallets]);
  const total = useMemo(() => items.reduce((s, i) => s + i.value, 0), [items]);
  if (!items.length) return null;

  const cx = 56, cy = 56, r = 44, ir = 28;
  let sa = -Math.PI / 2;
  const arcs = items.map((item) => {
    const frac = item.value / total;
    const angle = frac * Math.PI * 2;
    const ea = sa + angle;
    const la = angle > Math.PI ? 1 : 0;
    const gap = items.length > 1 ? 0.02 : 0;
    const s = sa + gap, e = ea - gap;
    const d = `M${cx + r * Math.cos(s)},${cy + r * Math.sin(s)} A${r},${r} 0 ${la} 1 ${cx + r * Math.cos(e)},${cy + r * Math.sin(e)} L${cx + ir * Math.cos(e)},${cy + ir * Math.sin(e)} A${ir},${ir} 0 ${la} 0 ${cx + ir * Math.cos(s)},${cy + ir * Math.sin(s)} Z`;
    sa = ea;
    return { ...item, d, pct: (frac * 100).toFixed(1) };
  });

  return (
    <div style={{ display: 'flex', alignItems: 'flex-start', gap: 16 }}>
      <svg width="112" height="112" viewBox="0 0 112 112" style={{ flexShrink: 0 }}>
        {arcs.map((a, i) => <path key={i} d={a.d} fill={a.color} fillOpacity="0.85" />)}
        <text x={cx} y={cy - 2} textAnchor="middle" fill="var(--text-muted)" fontSize="8" fontWeight="500">总计</text>
        <text x={cx} y={cy + 10} textAnchor="middle" fill="var(--text)" fontSize="12" fontWeight="700">{formatUsdCompact(total)}</text>
      </svg>
      <div style={{ display: 'flex', flexDirection: 'column', gap: 6, flex: 1, minWidth: 0, paddingTop: 4 }}>
        {arcs.map((a, i) => (
          <div key={i} style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <span style={{ width: 8, height: 8, borderRadius: '50%', background: a.color, flexShrink: 0 }} />
            <span className="am-item-sub" style={{ margin: 0, flex: 1, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{a.label}</span>
            <strong style={{ fontSize: 11, flexShrink: 0 }}>{formatUsdCompact(a.value)}</strong>
            <span style={{ fontSize: 9, color: 'var(--text-muted)', flexShrink: 0 }}>{a.pct}%</span>
          </div>
        ))}
      </div>
    </div>
  );
}

function WalletBarChart({ wallets }) {
  const items = useMemo(() => {
    if (!Array.isArray(wallets)) return [];
    const merged = {};
    wallets.forEach((w) => {
      const addr = w.wallet_address || `#${w.wallet_id}`;
      if (!merged[addr]) merged[addr] = { addr, native: 0, stable: 0, token: 0, total: 0 };
      merged[addr].native += Math.max(0, Number(w.native_usd || 0));
      merged[addr].stable += Math.max(0, Number(w.stable_usd || 0));
      merged[addr].token += Math.max(0, Number(w.token_usd || 0));
      merged[addr].total += Math.max(0, Number(w.total_usd || 0));
    });
    return Object.values(merged).map((m) => ({
      label: m.addr.startsWith('#') ? m.addr : `${m.addr.slice(0, 6)}...${m.addr.slice(-4)}`,
      native: m.native,
      stable: m.stable,
      token: m.token,
      total: m.total,
    }));
  }, [wallets]);
  const maxVal = useMemo(() => Math.max(...items.map((i) => i.total), 1), [items]);
  if (!items.length) return null;

  const C = { native: '#52d1ff', stable: '#59f09d', token: '#c792ff' };

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
      <div style={{ display: 'flex', gap: 12 }}>
        {[{ k: 'native', l: '原生币', c: C.native }, { k: 'stable', l: '稳定币', c: C.stable }, { k: 'token', l: '代币', c: C.token }].map((x) => (
          <div key={x.k} style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
            <span style={{ width: 6, height: 6, borderRadius: '50%', background: x.c }} />
            <span style={{ fontSize: 10, color: 'var(--text-muted)' }}>{x.l}</span>
          </div>
        ))}
      </div>
      {items.map((item, i) => {
        const barW = (item.total / maxVal) * 100;
        const nP = item.total > 0 ? (item.native / item.total) * 100 : 0;
        const sP = item.total > 0 ? (item.stable / item.total) * 100 : 0;
        const tP = item.total > 0 ? (item.token / item.total) * 100 : 0;
        return (
          <div key={i}>
            <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 4 }}>
              <span style={{ fontSize: 11, fontWeight: 600, color: 'var(--text)' }}>{item.label}</span>
              <span style={{ fontSize: 11, fontWeight: 700, color: 'var(--text)' }}>{formatUsdCompact(item.total)}</span>
            </div>
            <div style={{ height: 10, borderRadius: 999, overflow: 'hidden', background: 'rgba(136,157,191,0.08)', width: `${Math.max(barW, 10)}%` }}>
              <div style={{ display: 'flex', height: '100%' }}>
                {item.native > 0 && <div style={{ width: `${nP}%`, background: C.native, borderRadius: '999px 0 0 999px' }} />}
                {item.stable > 0 && <div style={{ width: `${sP}%`, background: C.stable }} />}
                {item.token > 0 && <div style={{ width: `${tP}%`, background: C.token, borderRadius: '0 999px 999px 0' }} />}
              </div>
            </div>
          </div>
        );
      })}
    </div>
  );
}

export default function AssetManagementPanel({
  apiBaseUrl,
  initData,
  hasInitData,
  isAdmin = false,
  refreshInterval = 10,
}) {
  const tabs = useMemo(() => {
    const list = [{ key: 'my_assets', label: '我的资产', icon: Wallet }];
    if (isAdmin) {
      list.push(
        { key: 'smart_money', label: '聪明钱', icon: TrendingUp },
        { key: 'operations', label: '运行管理', icon: Shield },
        { key: 'system', label: '系统', icon: Settings2 }
      );
    }
    return list;
  }, [isAdmin]);

  const [activeTab, setActiveTab] = useState('my_assets');
  const [historyDays, setHistoryDays] = useState(30);
  const [historyMetric] = useState('wallet_usd');
  const [assetState, setAssetState] = useState({ overview: null, history: null, lp: null });
  const [assetLoading, setAssetLoading] = useState(false);
  const [assetRefreshing, setAssetRefreshing] = useState(false);
  const [assetError, setAssetError] = useState('');

  const [smartMoneyDays, setSmartMoneyDays] = useState(7);
  const [leaderboardMetric, setLeaderboardMetric] = useState('pnl');
  const [smartMoneyOverview, setSmartMoneyOverview] = useState(null);
  const [smartMoneyWallet, setSmartMoneyWallet] = useState(null);
  const [smartMoneyLeaderboard, setSmartMoneyLeaderboard] = useState(null);
  const [smartMoneyLoading, setSmartMoneyLoading] = useState(false);
  const [smartMoneyRefreshing, setSmartMoneyRefreshing] = useState(false);
  const [smartMoneyError, setSmartMoneyError] = useState('');
  const [selectedWalletId, setSelectedWalletId] = useState('');
  const [selectedWalletMeta, setSelectedWalletMeta] = useState(null);
  const [smSubTab, setSmSubTab] = useState('wallets');
  const [smWalletSearch, setSmWalletSearch] = useState('');
  const [smWalletPage, setSmWalletPage] = useState(0);
  const [smLeaderSearch, setSmLeaderSearch] = useState('');
  const [smLeaderPage, setSmLeaderPage] = useState(0);
  const [smDrillWalletId, setSmDrillWalletId] = useState('');

  const SM_PAGE_SIZE = 10;

  const [opsLoading, setOpsLoading] = useState(false);
  const [opsError, setOpsError] = useState('');
  const [onlineUsers, setOnlineUsers] = useState([]);
  const [activeTasks, setActiveTasks] = useState([]);
  const [selectedUser, setSelectedUser] = useState(null);
  const [userPositions, setUserPositions] = useState(null);

  const [systemLoading, setSystemLoading] = useState(false);
  const [systemError, setSystemError] = useState('');
  const [systemConfig, setSystemConfig] = useState(null);
  const [configDraft, setConfigDraft] = useState({
    zap_price_deviation_max_percent: '',
    zap_min_pool_liquidity_usd: '',
  });
  const [rpcPool, setRpcPool] = useState(null);
  const [privateZap, setPrivateZap] = useState(null);
  const [rpcDraft, setRpcDraft] = useState({ chain: 'bsc', transport: 'http', url: '', name: '', setCurrent: false });

  useEffect(() => {
    if (!tabs.some((tab) => tab.key === activeTab)) {
      setActiveTab('my_assets');
    }
  }, [activeTab, tabs]);

  const hasAssetData = Boolean(assetState.overview || assetState.history || assetState.lp);
  const hasSmartMoneyData = Boolean(smartMoneyOverview || smartMoneyLeaderboard || smartMoneyWallet);
  const hasAssetDataRef = useRef(false);
  const hasSmartMoneyDataRef = useRef(false);

  useEffect(() => {
    hasAssetDataRef.current = hasAssetData;
  }, [hasAssetData]);

  useEffect(() => {
    hasSmartMoneyDataRef.current = hasSmartMoneyData;
  }, [hasSmartMoneyData]);

  const selectSmartMoneyWallet = useCallback((wallet, { openDetail = false } = {}) => {
    if (!wallet) return;
    const nextWallet = {
      address: wallet.address,
      chain_id: wallet.chain_id,
      label: wallet.label,
      assets: wallet.assets,
      active_pool_count: wallet.active_pool_count,
      today_event_count: wallet.today_event_count,
      last_active_at: wallet.last_active_at,
    };
    const nextWalletId = walletKey(nextWallet);
    setSelectedWalletId(nextWalletId);
    setSelectedWalletMeta(nextWallet);
    if (openDetail) {
      setSmSubTab('wallets');
      setSmDrillWalletId(nextWalletId);
    }
  }, []);

  const loadAssets = useCallback(async ({ forceRefresh = false } = {}) => {
    if (!hasInitData) return;
    if (hasAssetDataRef.current) setAssetRefreshing(true);
    else setAssetLoading(true);
    setAssetError('');
    try {
      const overviewPromise = fetchAssetOverview({ apiBaseUrl, initData, forceRefresh });
      const historyPromise = fetchAssetHistory({ apiBaseUrl, initData, days: historyDays, forceRefresh });
      const lpPromise = fetchAssetLPStats({ apiBaseUrl, initData, forceRefresh });

      overviewPromise
        .then((overview) => {
          startTransition(() => {
            setAssetState((prev) => ({ ...prev, overview: overview || null }));
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
        setAssetState((prev) => ({ ...prev, ...nextState }));
      });
      setAssetError(errors.find(Boolean) || '');
    } catch (err) {
      setAssetError(String(err?.message || err));
    } finally {
      setAssetLoading(false);
      setAssetRefreshing(false);
    }
  }, [apiBaseUrl, hasInitData, historyDays, initData]);

  const loadSmartMoney = useCallback(async ({ forceRefresh = false } = {}) => {
    if (!hasInitData || !isAdmin) return;
    if (hasSmartMoneyDataRef.current) setSmartMoneyRefreshing(true);
    else setSmartMoneyLoading(true);
    setSmartMoneyError('');
    try {
      const [overviewResult, leaderboardResult] = await Promise.allSettled([
        fetchAdminSmartMoneyOverview({
          apiBaseUrl,
          initData,
          days: smartMoneyDays,
          page: smWalletPage + 1,
          pageSize: SM_PAGE_SIZE,
          keyword: smWalletSearch,
          forceRefresh,
        }),
        fetchAdminSmartMoneyLeaderboard({
          apiBaseUrl,
          initData,
          days: 1,
          metric: leaderboardMetric,
          page: smLeaderPage + 1,
          pageSize: SM_PAGE_SIZE,
          keyword: smLeaderSearch,
          forceRefresh,
        }),
      ]);
      const overview = overviewResult.status === 'fulfilled' ? overviewResult.value : null;
      const leaderboard = leaderboardResult.status === 'fulfilled' ? leaderboardResult.value : null;
      const wallets = Array.isArray(overview?.wallets) ? overview.wallets : [];
      startTransition(() => {
        if (overviewResult.status === 'fulfilled') setSmartMoneyOverview(overview || null);
        if (leaderboardResult.status === 'fulfilled') setSmartMoneyLeaderboard(leaderboard || null);
      });
      const current = wallets.find((item) => walletKey(item) === selectedWalletId);
      if (current) {
        setSelectedWalletMeta((prev) => ({ ...(prev || {}), ...current }));
      } else if (!selectedWalletId && wallets[0]) {
        selectSmartMoneyWallet(wallets[0]);
      } else if (!wallets.length && !smDrillWalletId) {
        setSelectedWalletId('');
        setSelectedWalletMeta(null);
        setSmartMoneyWallet(null);
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
  }, [
    SM_PAGE_SIZE,
    apiBaseUrl,
    hasInitData,
    initData,
    isAdmin,
    leaderboardMetric,
    selectSmartMoneyWallet,
    selectedWalletId,
    smLeaderPage,
    smLeaderSearch,
    smDrillWalletId,
    smWalletPage,
    smWalletSearch,
    smartMoneyDays,
  ]);

  const loadOperations = useCallback(async () => {
    if (!hasInitData || !isAdmin) return;
    setOpsLoading(true);
    setOpsError('');
    try {
      const [usersResp, tasksResp] = await Promise.all([
        fetchAdminOnlineUsers({ apiBaseUrl, initData, limit: 50 }),
        fetchAdminActiveTasks({ apiBaseUrl, initData, limit: 100 }),
      ]);
      setOnlineUsers(Array.isArray(usersResp?.users) ? usersResp.users : []);
      setActiveTasks(Array.isArray(tasksResp?.tasks) ? tasksResp.tasks : []);
    } catch (err) {
      setOpsError(String(err?.message || err));
    } finally {
      setOpsLoading(false);
    }
  }, [apiBaseUrl, hasInitData, initData, isAdmin]);

  const loadUserPositions = useCallback(async (user) => {
    const userId = Number(user?.user_id || 0);
    if (!hasInitData || !isAdmin || userId <= 0) {
      setUserPositions(null);
      return;
    }
    try {
      const data = await fetchAdminRealtimePositions({ apiBaseUrl, initData, userId });
      setUserPositions(data || null);
      setSelectedUser(user || null);
    } catch (err) {
      setOpsError(String(err?.message || err));
      setUserPositions(null);
    }
  }, [apiBaseUrl, hasInitData, initData, isAdmin]);

  const loadSystem = useCallback(async () => {
    if (!hasInitData || !isAdmin) return;
    setSystemLoading(true);
    setSystemError('');
    try {
      const [configResp, rpcResp, zapResp] = await Promise.all([
        fetchSystemConfig({ apiBaseUrl, initData }),
        fetchAdminRPCPool({ apiBaseUrl, initData }),
        fetchAdminPrivateZap({ apiBaseUrl, initData }),
      ]);
      setSystemConfig(configResp || null);
      setConfigDraft({
        zap_price_deviation_max_percent: String(configResp?.config?.zap_price_deviation_max_percent ?? ''),
        zap_min_pool_liquidity_usd: String(configResp?.config?.zap_min_pool_liquidity_usd ?? ''),
      });
      setRpcPool(rpcResp || null);
      setPrivateZap(zapResp || null);
    } catch (err) {
      setSystemError(String(err?.message || err));
    } finally {
      setSystemLoading(false);
    }
  }, [apiBaseUrl, hasInitData, initData, isAdmin]);

  useEffect(() => {
    if (activeTab !== 'my_assets') return undefined;
    loadAssets();
    if (!hasInitData) return undefined;
    const timer = setInterval(() => loadAssets(), Math.max(60, Number(refreshInterval || 10)) * 1000);
    return () => clearInterval(timer);
  }, [activeTab, hasInitData, loadAssets, refreshInterval]);

  useEffect(() => {
    if (activeTab !== 'smart_money') return undefined;
    loadSmartMoney();
    if (!hasInitData || !isAdmin) return undefined;
    const timer = setInterval(() => loadSmartMoney(), Math.max(60, Number(refreshInterval || 10)) * 1000);
    return () => clearInterval(timer);
  }, [activeTab, hasInitData, isAdmin, loadSmartMoney, refreshInterval]);

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

  const selectedWallet = useMemo(() => {
    const wallets = Array.isArray(smartMoneyOverview?.wallets) ? smartMoneyOverview.wallets : [];
    return wallets.find((item) => walletKey(item) === selectedWalletId) || selectedWalletMeta || null;
  }, [selectedWalletId, selectedWalletMeta, smartMoneyOverview]);

  useEffect(() => {
    if (activeTab !== 'smart_money' || smSubTab !== 'wallets' || !smDrillWalletId || !selectedWallet || !hasInitData || !isAdmin) {
      return undefined;
    }
    let disposed = false;
    const run = async (forceRefresh = false) => {
      await loadSmartMoneyWallet({ wallet: selectedWallet, forceRefresh });
      if (disposed) return;
    };
    run();
    const timer = setInterval(() => run(), Math.max(60, Number(refreshInterval || 10)) * 1000);
    return () => {
      disposed = true;
      clearInterval(timer);
    };
  }, [activeTab, hasInitData, isAdmin, loadSmartMoneyWallet, refreshInterval, selectedWallet, smDrillWalletId, smSubTab]);

  useEffect(() => {
    if (activeTab !== 'operations') return undefined;
    loadOperations();
    if (!hasInitData || !isAdmin) return undefined;
    const timer = setInterval(loadOperations, Math.max(60, Number(refreshInterval || 10)) * 1000);
    return () => clearInterval(timer);
  }, [activeTab, hasInitData, isAdmin, loadOperations, refreshInterval]);

  useEffect(() => {
    if (activeTab !== 'system') return undefined;
    loadSystem();
    if (!hasInitData || !isAdmin) return undefined;
    const timer = setInterval(loadSystem, Math.max(60, Number(refreshInterval || 10)) * 1000);
    return () => clearInterval(timer);
  }, [activeTab, hasInitData, isAdmin, loadSystem, refreshInterval]);

  const chartPoints = useMemo(() => {
    const rows = Array.isArray(assetState?.history?.history) ? [...assetState.history.history] : [];
    if (assetState?.history?.today?.day) rows.push(assetState.history.today);
    return rows.map((item) => ({ day: item.day, value: Number(item?.[historyMetric] || 0) }));
  }, [assetState.history, historyMetric]);

  const smartMoneyPnlCalData = useMemo(() => {
    const history = Array.isArray(smartMoneyWallet?.history) ? [...smartMoneyWallet.history].sort((a, b) => a.day.localeCompare(b.day)) : [];
    if (!history.length) return [];
    return history.map((item) => ({
      day: item.day,
      realized_pnl_usd: Number(item.estimated_realized_pnl_usd || 0),
      has_transfer_in: Boolean(item.has_transfer_in),
      has_transfer_out: Boolean(item.has_transfer_out),
      transfer_in_count: Number(item.transfer_in_count || 0),
      transfer_out_count: Number(item.transfer_out_count || 0),
      transfer_total_count: Number(item.transfer_total_count || 0),
      transfer_in_usd: Number(item.transfer_in_usd || 0),
      transfer_out_usd: Number(item.transfer_out_usd || 0),
      transfer_net_usd: Number(item.transfer_net_usd || 0),
    }));
  }, [smartMoneyWallet?.history]);

  const handleSaveSystemConfig = useCallback(async () => {
    try {
      const payload = await updateSystemConfig({
        apiBaseUrl,
        initData,
        config: {
          zap_price_deviation_max_percent: Number(configDraft.zap_price_deviation_max_percent || 0),
          zap_min_pool_liquidity_usd: Number(configDraft.zap_min_pool_liquidity_usd || 0),
        },
      });
      setSystemConfig(payload || null);
      setConfigDraft({
        zap_price_deviation_max_percent: String(payload?.config?.zap_price_deviation_max_percent ?? ''),
        zap_min_pool_liquidity_usd: String(payload?.config?.zap_min_pool_liquidity_usd ?? ''),
      });
    } catch (err) {
      setSystemError(String(err?.message || err));
    }
  }, [apiBaseUrl, configDraft, initData]);

  const refreshSystemAfter = useCallback(async (fn) => {
    try {
      await fn();
      await loadSystem();
    } catch (err) {
      setSystemError(String(err?.message || err));
    }
  }, [loadSystem]);

  const handleAddRPC = useCallback(async () => {
    if (!String(rpcDraft.url || '').trim()) return;
    await refreshSystemAfter(() => addAdminRPCEndpoint({
      apiBaseUrl,
      initData,
      chain: rpcDraft.chain,
      transport: rpcDraft.transport,
      name: rpcDraft.name,
      url: rpcDraft.url,
      setCurrent: rpcDraft.setCurrent,
    }));
    setRpcDraft({ chain: 'bsc', transport: 'http', url: '', name: '', setCurrent: false });
  }, [apiBaseUrl, initData, refreshSystemAfter, rpcDraft]);

  const overviewWallets = useMemo(
    () => (Array.isArray(smartMoneyOverview?.wallets) ? smartMoneyOverview.wallets : []),
    [smartMoneyOverview]
  );
  const walletTotal = Math.max(0, Number(smartMoneyOverview?.wallet_total || 0) || overviewWallets.length);
  const walletTotalPages = Math.max(1, Number(smartMoneyOverview?.wallet_total_pages || 0) || 1);
  const pagedWallets = overviewWallets;
  const leaderboardRows = useMemo(
    () => (Array.isArray(smartMoneyLeaderboard?.list) ? smartMoneyLeaderboard.list : []),
    [smartMoneyLeaderboard]
  );
  const leaderTotalPages = Math.max(1, Number(smartMoneyLeaderboard?.total_pages || 0) || 1);

  useEffect(() => {
    if (smWalletPage > walletTotalPages - 1) {
      setSmWalletPage(Math.max(walletTotalPages - 1, 0));
    }
  }, [smWalletPage, walletTotalPages]);

  useEffect(() => {
    if (smLeaderPage > leaderTotalPages - 1) {
      setSmLeaderPage(Math.max(leaderTotalPages - 1, 0));
    }
  }, [leaderTotalPages, smLeaderPage]);

  useEffect(() => {
    setSmLeaderPage(0);
  }, [leaderboardMetric]);

  const isRefreshing = assetLoading || assetRefreshing || smartMoneyLoading || smartMoneyRefreshing || opsLoading || systemLoading;

  const subtitle = activeTab === 'smart_money'
    ? '聪明钱资产、排行榜与钱包详情'
    : activeTab === 'operations'
      ? '在线用户、活跃任务与用户持仓'
      : activeTab === 'system'
        ? '系统配置、RPC 与 Private Zap'
        : '资产快照、历史趋势与 LP 统计';

  const metricColor = '#52d1ff';

  const actions = (
    <div className="am-actions">
      {tabs.map((tab) => {
        const Icon = tab.icon;
        return (
          <button
            type="button"
            key={tab.key}
            className={`am-tab-btn ${activeTab === tab.key ? 'active' : ''}`}
            onClick={() => setActiveTab(tab.key)}
          >
            <Icon size={12} />
            {tab.label}
          </button>
        );
      })}
      <button
        type="button"
        className="am-tab-btn"
        disabled={isRefreshing}
        onClick={() => {
          if (activeTab === 'my_assets') loadAssets({ forceRefresh: true });
          if (activeTab === 'smart_money') {
            loadSmartMoney({ forceRefresh: true });
            if (selectedWallet) loadSmartMoneyWallet({ wallet: selectedWallet, forceRefresh: true });
          }
          if (activeTab === 'operations') loadOperations();
          if (activeTab === 'system') loadSystem();
        }}
      >
        <RefreshCw size={12} className={isRefreshing ? 'animate-spin' : undefined} />
        刷新
      </button>
    </div>
  );

  return (
    <PanelShell title="资产管理" subtitle={subtitle} icon={activeTab === 'smart_money' ? TrendingUp : activeTab === 'operations' ? Shield : activeTab === 'system' ? Settings2 : Wallet} actions={actions}>
      {!hasInitData ? <EmptyState text="请先完成 Telegram 登录后查看资产管理数据" /> : null}

      {hasInitData && activeTab === 'my_assets' ? (
        <div className="am-stack">
          {assetError ? <div className="am-error">{assetError}</div> : null}
          <div className="am-metric-row">
            <MetricCard label="总资产" value={formatUsd(assetState.overview?.summary?.total_usd)} tone="strong" />
            <MetricCard label="钱包余额" value={formatUsd(assetState.overview?.summary?.wallet_usd)} />
            <MetricCard label="LP 持仓" value={formatUsd(assetState.overview?.summary?.position_usd)} />
            <MetricCard label="未领取手续费" value={formatUsd(assetState.overview?.summary?.fee_usd)} />
          </div>

          {assetState.overview?.warnings?.length ? (
            <div className="am-warn-row">
              {assetState.overview.warnings.map((warning) => (
                <span key={warning} className="am-badge am-badge-warn">{warning}</span>
              ))}
            </div>
          ) : null}

          <div className="am-card">
            <div className="am-card-header">
              <div className="am-card-title">资产趋势</div>
              <div className="am-pill-group">
                {HISTORY_WINDOWS.map((days) => (
                  <button key={days} type="button" className={`am-pill ${historyDays === days ? 'active' : ''}`} onClick={() => setHistoryDays(days)}>
                    {days}D
                  </button>
                ))}
              </div>
            </div>
            <div className="am-chart-header">
              <div>
                <div className="am-chart-label">钱包余额</div>
                <div className="am-chart-value">{formatUsd(chartPoints[chartPoints.length - 1]?.value)}</div>
              </div>
              <span className="am-badge">{formatChinaTime(assetState.overview?.updated_at)}</span>
            </div>
            <LWAreaChart points={chartPoints} stroke={metricColor} />
          </div>

          <div className="am-two-col">
            <div className="am-card">
              {(() => {
                const realizedPnl = Number(assetState.lp?.today?.realized_pnl_usd || 0);
                const pnlLabel = realizedPnl > 0 ? '今日盈利' : realizedPnl < 0 ? '今日亏损' : '今日盈亏';
                const pnlColor = realizedPnl >= 0 ? 'var(--positive)' : 'var(--negative)';
                return (
                  <>
              <div className="am-card-header">
                <div className="am-card-title">今日盈亏</div>
              </div>
              <div className="am-stat-grid am-stat-grid-3 am-today-pnl-grid">
                <div className="am-stat am-stat-compact am-stat-pnl">
                  <div className="am-stat-label">{pnlLabel}</div>
                  <div className="am-stat-value" style={{ color: pnlColor }}>
                    {realizedPnl > 0 ? '+' : ''}{formatUsd(realizedPnl)}
                  </div>
                </div>
                <div className="am-stat am-stat-compact">
                  <div className="am-stat-label">平仓笔数</div>
                  <div className="am-stat-value">{Number(assetState.lp?.today?.closed_count || 0)}</div>
                </div>
                <div className="am-stat am-stat-compact">
                  <div className="am-stat-label">胜率</div>
                  <div className="am-stat-value">{formatPct(assetState.lp?.today?.win_rate)}</div>
                  <div className="am-stat-sub">{Number(assetState.lp?.today?.win_count || 0)}W / {Number(assetState.lp?.today?.loss_count || 0)}L</div>
                </div>
              </div>
                  </>
                );
              })()}
              <TodayPoolPnL pools={assetState.lp?.today_pools} />
            </div>

            <div className="am-card">
              <PnLCalendar
                data={assetState.lp?.daily_history}
                loading={assetLoading}
                note="收益额按当日总资产变化减去当日净转账计算；今日盈亏按实时总资产对比昨日快照，并剔除今日净转账。"
              />
              {Array.isArray(assetState.lp?.windows) && assetState.lp.windows.length > 0 && (
                <div className="am-stat-grid am-stat-grid-3" style={{ marginTop: 10 }}>
                  {assetState.lp.windows.map((item) => (
                    <div key={item.days} className="am-stat">
                      <div className="am-stat-label">{item.days}D</div>
                      <div className="am-stat-value" style={{ color: Number(item.realized_pnl_usd || 0) >= 0 ? 'var(--positive)' : 'var(--negative)' }}>
                        {Number(item.realized_pnl_usd || 0) >= 0 ? '+' : ''}{formatUsdCompact(item.realized_pnl_usd)}
                      </div>
                      <div className="am-stat-sub">{item.closed_count || 0} 笔 · {formatPct(item.win_rate)}</div>
                    </div>
                  ))}
                </div>
              )}
            </div>
          </div>

          <div className="am-card">
            <div className="am-card-title">钱包资产分布</div>
            {Array.isArray(assetState.overview?.wallets) && assetState.overview.wallets.length > 0 ? (
              <div className="am-stack" style={{ marginTop: 12 }}>
                <DonutChart wallets={assetState.overview.wallets} />
                <div style={{ borderTop: '1px solid rgba(136,157,191,0.1)', paddingTop: 12 }}>
                  <WalletBarChart wallets={assetState.overview.wallets} />
                </div>
              </div>
            ) : <EmptyState text={assetLoading ? '正在加载...' : '暂无钱包数据'} />}
          </div>
        </div>
      ) : null}

      {hasInitData && activeTab === 'smart_money' ? (
        <div className="am-stack">
          {smartMoneyError ? <div className="am-error">{smartMoneyError}</div> : null}
          <div className="am-pill-group">
            {SMART_MONEY_WINDOWS.map((days) => (
              <button key={days} type="button" className={`am-pill ${smartMoneyDays === days ? 'active' : ''}`} onClick={() => setSmartMoneyDays(days)}>
                {days === 1 ? '昨日' : `${days}D`}
              </button>
            ))}
          </div>
          <div className="am-metric-row">
            <MetricCard label="总资产" value={formatUsd(smartMoneyOverview?.summary?.total_usd)} tone="strong" />
            <MetricCard label="原生币" value={formatUsd(smartMoneyOverview?.summary?.native_usd)} />
            <MetricCard label="稳定币" value={formatUsd(smartMoneyOverview?.summary?.stable_usd)} />
            <MetricCard label="代币持仓" value={formatUsd(smartMoneyOverview?.summary?.tracked_token_usd)} />
            <MetricCard label="Open LP" value={formatUsd(smartMoneyOverview?.summary?.open_lp_usd)} />
            <MetricCard label="代币种类" value={`${Number(smartMoneyOverview?.summary?.tracked_token_count || 0)} 个`} />
          </div>

          {/* sub-tab pills */}
          <div className="am-pill-group">
            <button type="button" className={`am-pill ${smSubTab === 'wallets' ? 'active' : ''}`} onClick={() => { setSmSubTab('wallets'); setSmDrillWalletId(''); }}>钱包总览</button>
            <button type="button" className={`am-pill ${smSubTab === 'leaderboard' ? 'active' : ''}`} onClick={() => { setSmSubTab('leaderboard'); setSmDrillWalletId(''); }}>排行榜</button>
          </div>

          {/* ── wallets sub-tab ── */}
          {smSubTab === 'wallets' && !smDrillWalletId ? (
            <div className="am-card">
              <div className="am-card-header">
                <div className="am-card-title">钱包总览</div>
                <span className="am-badge">{walletTotal} 个</span>
              </div>
              <div style={{ position: 'relative', marginTop: 8 }}>
                <Search size={14} style={{ position: 'absolute', left: 10, top: '50%', transform: 'translateY(-50%)', opacity: 0.35 }} />
                <input
                  type="text"
                  value={smWalletSearch}
                  onChange={(e) => { setSmWalletSearch(e.target.value); setSmWalletPage(0); }}
                  placeholder="搜索地址或标签"
                  className="am-search-input"
                  style={{ width: '100%', padding: '7px 12px 7px 32px', borderRadius: 10, border: '1px solid rgba(136,157,191,0.12)', background: 'rgba(136,157,191,0.04)', fontSize: 12, outline: 'none', color: 'inherit' }}
                />
              </div>
              <div className="am-list">
                {pagedWallets.length > 0 ? pagedWallets.map((wallet) => {
                  const assets = wallet.assets || {};
                  const total = Number(assets.total_usd || 0);
                  const nativePct = total > 0 ? (Number(assets.native_usd || 0) / total * 100) : 0;
                  const stablePct = total > 0 ? (Number(assets.stable_usd || 0) / total * 100) : 0;
                  const tokenPct = total > 0 ? (Number(assets.tracked_token_usd || 0) / total * 100) : 0;
                  const lpPct = total > 0 ? (Number(assets.open_lp_usd || 0) / total * 100) : 0;
                  return (
                    <button key={walletKey(wallet)} type="button" className={`am-list-item am-list-btn ${walletKey(wallet) === selectedWalletId ? 'selected' : ''}`} onClick={() => selectSmartMoneyWallet(wallet, { openDetail: true })}>
                      <div style={{ display: 'flex', alignItems: 'center', gap: 10, flex: 1, minWidth: 0 }}>
                        <WalletAvatar address={wallet.address} avatarUrl={wallet.avatar_url} size={28} />
                        <div style={{ minWidth: 0 }}>
                          <div className="am-item-title">{walletLabel(wallet)}</div>
                          <div className="am-item-sub">{formatChain(wallet.chain_id)} · {Number(wallet.today_event_count || 0)} 事件 · {Number(wallet.active_pool_count || 0)} 池</div>
                          {total > 0 && (
                            <div style={{ marginTop: 4, display: 'flex', flexDirection: 'column', gap: 2, width: '100%' }}>
                              <div style={{ height: 4, borderRadius: 2, overflow: 'hidden', background: 'rgba(136,157,191,0.08)', width: '100%', display: 'flex' }}>
                                {nativePct > 0 && <div style={{ width: `${nativePct}%`, height: '100%', background: '#52d1ff' }} />}
                                {stablePct > 0 && <div style={{ width: `${stablePct}%`, height: '100%', background: '#59f09d' }} />}
                                {tokenPct > 0 && <div style={{ width: `${tokenPct}%`, height: '100%', background: '#c792ff' }} />}
                                {lpPct > 0 && <div style={{ width: `${lpPct}%`, height: '100%', background: '#ffae42' }} />}
                              </div>
                            </div>
                          )}
                        </div>
                      </div>
                      <div className="am-list-end">
                        <strong>{formatUsdCompact(wallet.assets?.total_usd)}</strong>
                        <ChevronRight size={12} />
                      </div>
                    </button>
                  );
                }) : <EmptyState text={smartMoneyLoading ? '正在加载...' : '暂无钱包数据'} />}
              </div>
              {walletTotalPages > 1 && (
                <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 12, paddingTop: 8 }}>
                  <button type="button" disabled={smWalletPage <= 0} onClick={() => setSmWalletPage(smWalletPage - 1)} style={{ padding: '4px 10px', borderRadius: 8, border: '1px solid rgba(136,157,191,0.15)', background: 'transparent', fontSize: 11, cursor: smWalletPage <= 0 ? 'default' : 'pointer', opacity: smWalletPage <= 0 ? 0.3 : 1, color: 'inherit' }}>上一页</button>
                  <span style={{ fontSize: 11, opacity: 0.5 }}>{smWalletPage + 1} / {walletTotalPages}</span>
                  <button type="button" disabled={smWalletPage >= walletTotalPages - 1} onClick={() => setSmWalletPage(smWalletPage + 1)} style={{ padding: '4px 10px', borderRadius: 8, border: '1px solid rgba(136,157,191,0.15)', background: 'transparent', fontSize: 11, cursor: smWalletPage >= walletTotalPages - 1 ? 'default' : 'pointer', opacity: smWalletPage >= walletTotalPages - 1 ? 0.3 : 1, color: 'inherit' }}>下一页</button>
                </div>
              )}
            </div>
          ) : null}

          {/* ── wallet drill-in detail ── */}
          {smSubTab === 'wallets' && smDrillWalletId ? (
            <div className="am-card">
              <button type="button" onClick={() => setSmDrillWalletId('')} style={{ display: 'inline-flex', alignItems: 'center', gap: 4, fontSize: 12, background: 'none', border: 'none', cursor: 'pointer', padding: 0, marginBottom: 8, opacity: 0.55, color: 'inherit' }}>
                <ChevronLeft size={14} />返回列表
              </button>
              {selectedWallet && smartMoneyWallet ? (
                <div className="am-stack">
                  {/* wallet header */}
                  <div style={{ display: 'flex', alignItems: 'center', gap: 12, padding: '8px 0' }}>
                    <WalletAvatar address={selectedWallet.address} avatarUrl={selectedWallet.avatar_url || smartMoneyWallet.wallet?.avatar_url} size={36} />
                    <div style={{ flex: 1, minWidth: 0 }}>
                      <div className="am-item-title" style={{ fontSize: 13 }}>{walletLabel(selectedWallet)}</div>
                      <div className="am-item-sub">{formatChain(selectedWallet.chain_id)} · 总资产 <strong>{formatUsdCompact(smartMoneyWallet.wallet?.assets?.total_usd)}</strong></div>
                    </div>
                  </div>

                  {/* today activity grid */}
                  <div className="am-stat-grid am-stat-grid-3">
                    <div className="am-stat">
                      <div className="am-stat-label">今日收益</div>
                      <div className="am-stat-value" style={{ color: Number(smartMoneyWallet.today?.estimated_realized_pnl_usd || 0) >= 0 ? '#59f09d' : '#ff6b6b' }}>{formatUsd(smartMoneyWallet.today?.estimated_realized_pnl_usd)}</div>
                    </div>
                    <div className="am-stat">
                      <div className="am-stat-label">加仓次数</div>
                      <div className="am-stat-value">{Number(smartMoneyWallet.today?.add_count || 0)} 次</div>
                    </div>
                    <div className="am-stat">
                      <div className="am-stat-label">撤仓次数</div>
                      <div className="am-stat-value">{Number(smartMoneyWallet.today?.remove_count || 0)} 次</div>
                    </div>
                    <div className="am-stat">
                      <div className="am-stat-label">活跃池数</div>
                      <div className="am-stat-value">{Number(smartMoneyWallet.today?.active_pool_count || 0)} 池</div>
                    </div>
                    <div className="am-stat">
                      <div className="am-stat-label">已匹配</div>
                      <div className="am-stat-value">{Number(smartMoneyWallet.today?.matched_remove_count || 0)} 次</div>
                    </div>
                    <div className="am-stat">
                      <div className="am-stat-label">未匹配</div>
                      <div className="am-stat-value" style={{ color: Number(smartMoneyWallet.today?.unmatched_remove_count || 0) > 0 ? '#ffae42' : undefined }}>{Number(smartMoneyWallet.today?.unmatched_remove_count || 0)} 次</div>
                    </div>
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
                      <div style={{ borderRadius: 10, border: '1px solid rgba(136,157,191,0.08)', padding: '10px 12px', background: 'rgba(136,157,191,0.03)' }}>
                        <div style={{ fontSize: 9, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.05em', opacity: 0.4, marginBottom: 8 }}>资产分布</div>
                        <div style={{ height: 6, borderRadius: 3, overflow: 'hidden', background: 'rgba(136,157,191,0.08)', display: 'flex' }}>
                          {nativePct > 0 && <div style={{ width: `${nativePct}%`, height: '100%', background: '#52d1ff' }} />}
                          {stablePct > 0 && <div style={{ width: `${stablePct}%`, height: '100%', background: '#59f09d' }} />}
                          {tokenPct > 0 && <div style={{ width: `${tokenPct}%`, height: '100%', background: '#c792ff' }} />}
                          {lpPct > 0 && <div style={{ width: `${lpPct}%`, height: '100%', background: '#ffae42' }} />}
                        </div>
                        <div style={{ marginTop: 6, display: 'flex', flexWrap: 'wrap', gap: '2px 12px', fontSize: 10, opacity: 0.5 }}>
                          <span style={{ display: 'flex', alignItems: 'center', gap: 4 }}><span style={{ width: 6, height: 6, borderRadius: 3, background: '#52d1ff', flexShrink: 0 }} />原生 {formatUsdCompact(wa.native_usd)}</span>
                          <span style={{ display: 'flex', alignItems: 'center', gap: 4 }}><span style={{ width: 6, height: 6, borderRadius: 3, background: '#59f09d', flexShrink: 0 }} />稳定 {formatUsdCompact(wa.stable_usd)}</span>
                          <span style={{ display: 'flex', alignItems: 'center', gap: 4 }}><span style={{ width: 6, height: 6, borderRadius: 3, background: '#c792ff', flexShrink: 0 }} />代币 {formatUsdCompact(wa.tracked_token_usd)}</span>
                          <span style={{ display: 'flex', alignItems: 'center', gap: 4 }}><span style={{ width: 6, height: 6, borderRadius: 3, background: '#ffae42', flexShrink: 0 }} />LP {formatUsdCompact(wa.open_lp_usd)}</span>
                        </div>
                      </div>
                    );
                  })()}

                  {/* PnL calendar (daily balance diff) */}
                  <PnLCalendar
                    data={smartMoneyPnlCalData}
                    note="收益额按当日总资产变化减去当日净转账计算；若当天存在多笔转账，仅展示净转入或净转出的汇总说明。"
                  />

                  {/* window stats */}
                  <div className="am-stat-grid am-stat-grid-3">
                    {Array.isArray(smartMoneyWallet.windows) ? smartMoneyWallet.windows.map((item) => (
                      <div key={item.days} className="am-stat">
                        <div className="am-stat-label">{item.days}D</div>
                        <div className="am-stat-value">{formatUsd(item.estimated_realized_pnl_usd)}</div>
                        <div className="am-stat-sub">{formatPct(item.yield_rate)} · {Number(item.active_pool_count || 0)} 池</div>
                      </div>
                    )) : null}
                  </div>

                  {/* warnings */}
                  {Array.isArray(smartMoneyWallet.warnings) && smartMoneyWallet.warnings.length > 0 && (
                    <div className="am-stack" style={{ gap: 6 }}>
                      {smartMoneyWallet.warnings.map((warn, i) => (
                        <div key={i} style={{ display: 'flex', alignItems: 'flex-start', gap: 8, borderRadius: 10, border: '1px solid rgba(255,174,66,0.2)', background: 'rgba(255,174,66,0.06)', padding: '8px 12px' }}>
                          <AlertTriangle size={14} style={{ flexShrink: 0, color: '#ffae42', marginTop: 1 }} />
                          <span style={{ fontSize: 11, color: '#ffae42' }}>{typeof warn === 'string' ? warn : warn.message || JSON.stringify(warn)}</span>
                        </div>
                      ))}
                    </div>
                  )}
                </div>
              ) : <EmptyState text={smartMoneyLoading ? '正在加载...' : '选择钱包查看明细'} />}
            </div>
          ) : null}

          {/* ── leaderboard sub-tab ── */}
          {smSubTab === 'leaderboard' ? (
            <div className="am-card">
              <div className="am-card-header">
                <div className="am-card-title">昨日快照排行</div>
                <div className="am-pill-group">
                  {LEADERBOARD_METRICS.map((item) => (
                    <button key={item.key} type="button" className={`am-pill ${leaderboardMetric === item.key ? 'active' : ''}`} onClick={() => setLeaderboardMetric(item.key)}>
                      {item.label}
                    </button>
                  ))}
                </div>
              </div>
              <div style={{ display: 'flex', alignItems: 'center', gap: 6, flexWrap: 'wrap', fontSize: 10, opacity: 0.55, marginTop: 4 }}>
                <ArrowRightLeft size={11} />
                <span>
                  {smartMoneyLeaderboard?.snapshot_day && (smartMoneyLeaderboard?.compared_day || smartMoneyLeaderboard?.start_day)
                    ? `榜单基于 ${smartMoneyLeaderboard.snapshot_day} 相对 ${smartMoneyLeaderboard.compared_day || smartMoneyLeaderboard.start_day} 的资产快照`
                    : '榜单基于昨日资产快照'}
                </span>
              </div>
              <div style={{ position: 'relative', marginTop: 8 }}>
                <Search size={14} style={{ position: 'absolute', left: 10, top: '50%', transform: 'translateY(-50%)', opacity: 0.35 }} />
                <input
                  type="text"
                  value={smLeaderSearch}
                  onChange={(e) => { setSmLeaderSearch(e.target.value); setSmLeaderPage(0); }}
                  placeholder="搜索地址或标签"
                  className="am-search-input"
                  style={{ width: '100%', padding: '7px 12px 7px 32px', borderRadius: 10, border: '1px solid rgba(136,157,191,0.12)', background: 'rgba(136,157,191,0.04)', fontSize: 12, outline: 'none', color: 'inherit' }}
                />
              </div>
                <div className="am-list">
                  {leaderboardRows.length > 0 ? leaderboardRows.map((item) => {
                    const metricText = leaderboardMetric === 'yield_rate' ? formatPct(item.metric_value) : leaderboardMetric === 'participation' ? `${Number(item.metric_value || 0)} 次` : formatUsd(item.metric_value);
                    const pnl = Number(item.estimated_realized_pnl_usd || 0);
                    return (
                    <button
                      key={`${item.rank}:${item.address}`}
                      type="button"
                      className={`am-list-item am-list-btn ${item.rank <= 3 ? 'am-top-rank' : ''}`}
                      onClick={() => selectSmartMoneyWallet(item, { openDetail: true })}
                    >
                      <div className="am-rank-row">
                        <RankBadge rank={Number(item.rank || 0)} />
                        <WalletAvatar address={item.address} avatarUrl={item.avatar_url} size={30} />
                        <div>
                          <div className="am-item-title">{item.label || `${item.address.slice(0, 6)}...${item.address.slice(-4)}`}</div>
                          <div className="am-item-sub">{formatChain(item.chain_id)} · {Number(item.active_pool_count || 0)} 池 · {Number(item.participation_count || 0)} 次操作</div>
                          {hasTransferMarker(item) ? <div style={{ marginTop: 4 }}><TransferBadges item={item} /></div> : null}
                        </div>
                      </div>
                      <div className="am-list-end" style={{ flexDirection: 'column', alignItems: 'flex-end', gap: 2 }}>
                        <strong style={{ color: leaderboardMetric === 'pnl' ? (pnl >= 0 ? '#59f09d' : '#ff6b6b') : undefined }}>{metricText}</strong>
                        {leaderboardMetric !== 'pnl' && <span className="am-item-sub" style={{ color: pnl >= 0 ? '#59f09d' : '#ff6b6b' }}>{pnl >= 0 ? '+' : ''}{formatUsdCompact(pnl)}</span>}
                        {leaderboardMetric === 'pnl' && Number(item.yield_rate || 0) !== 0 && <span className="am-item-sub">{formatPct(item.yield_rate)}</span>}
                      </div>
                    </button>
                  );
                }) : <EmptyState text={smartMoneyLoading ? '正在加载...' : '暂无排行榜数据'} />}
              </div>
              {leaderTotalPages > 1 && (
                <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 12, paddingTop: 8 }}>
                  <button type="button" disabled={smLeaderPage <= 0} onClick={() => setSmLeaderPage(smLeaderPage - 1)} style={{ padding: '4px 10px', borderRadius: 8, border: '1px solid rgba(136,157,191,0.15)', background: 'transparent', fontSize: 11, cursor: smLeaderPage <= 0 ? 'default' : 'pointer', opacity: smLeaderPage <= 0 ? 0.3 : 1, color: 'inherit' }}>上一页</button>
                  <span style={{ fontSize: 11, opacity: 0.5 }}>{smLeaderPage + 1} / {leaderTotalPages}</span>
                  <button type="button" disabled={smLeaderPage >= leaderTotalPages - 1} onClick={() => setSmLeaderPage(smLeaderPage + 1)} style={{ padding: '4px 10px', borderRadius: 8, border: '1px solid rgba(136,157,191,0.15)', background: 'transparent', fontSize: 11, cursor: smLeaderPage >= leaderTotalPages - 1 ? 'default' : 'pointer', opacity: smLeaderPage >= leaderTotalPages - 1 ? 0.3 : 1, color: 'inherit' }}>下一页</button>
                </div>
              )}
            </div>
          ) : null}
        </div>
      ) : null}

      {hasInitData && activeTab === 'operations' ? (
        <div className="am-stack">
          {opsError ? <div className="am-error">{opsError}</div> : null}
          <div className="am-two-col">
            <div className="am-card">
              <div className="am-card-header">
                <div className="am-card-title"><Users size={14} /> 在线用户</div>
                <span className="am-badge">{onlineUsers.length} 人</span>
              </div>
              <div className="am-list">
                {onlineUsers.length > 0 ? onlineUsers.map((user) => (
                  <button key={user.user_id} type="button" className={`am-list-item am-list-btn ${Number(selectedUser?.user_id || 0) === Number(user.user_id || 0) ? 'selected' : ''}`} onClick={() => loadUserPositions(user)}>
                    <div>
                      <div className="am-item-title">{user.username ? `@${user.username}` : `用户 ${user.user_id}`}</div>
                      <div className="am-item-sub">TG {user.telegram_id || '--'} · 任务 {Number(user.total_tasks || 0)}</div>
                    </div>
                    <strong>{user.updated_at ? new Date(user.updated_at).toLocaleTimeString() : '--'}</strong>
                  </button>
                )) : <EmptyState text={opsLoading ? '正在加载...' : '暂无在线用户'} />}
              </div>
            </div>

            <div className="am-card">
              <div className="am-card-header">
                <div className="am-card-title"><ClipboardList size={14} /> 活跃任务</div>
                <span className="am-badge">{activeTasks.length} 条</span>
              </div>
              <div className="am-list">
                {activeTasks.length > 0 ? activeTasks.map((task) => (
                  <button
                    key={task.task_id}
                    type="button"
                    className="am-list-item am-list-btn"
                    onClick={() => loadUserPositions({ user_id: task.user_id, username: task.username, telegram_id: task.telegram_id })}
                  >
                    <div>
                      <div className="am-item-title">{`${task.token0_symbol || '--'}/${task.token1_symbol || '--'}`}</div>
                      <div className="am-item-sub">@{task.username || 'unknown'} · #{task.task_id}</div>
                    </div>
                    <strong>{Number(task.amount_usdt || 0).toFixed(2)} USDT</strong>
                  </button>
                )) : <EmptyState text={opsLoading ? '正在加载...' : '暂无活跃任务'} />}
              </div>
            </div>
          </div>

          <div className="am-card">
            <div className="am-card-header">
              <div className="am-card-title"><Activity size={14} /> 用户持仓</div>
              <span className="am-badge">{selectedUser?.username ? `@${selectedUser.username}` : selectedUser?.user_id ? `用户 ${selectedUser.user_id}` : '未选择'}</span>
            </div>
            {Array.isArray(userPositions?.positions) && userPositions.positions.length > 0 ? (
              <div className="am-list">
                {userPositions.positions.map((position, index) => (
                  <div key={`${position.position_id || index}:${position.pool_id || ''}`} className="am-list-item">
                    <div>
                      <div className="am-item-title">{position.title || position.pool_id || '--'}</div>
                      <div className="am-item-sub">{position.status_label || '--'} · {position.chain || '--'} · 钱包 {position.wallet_id || '--'}</div>
                    </div>
                    <strong>{formatUsd(position.current_value_usd || position.totals?.total_usd)}</strong>
                  </div>
                ))}
              </div>
            ) : <EmptyState text={selectedUser ? '暂无活跃持仓' : '选择用户后查看持仓'} />}
          </div>
        </div>
      ) : null}

      {hasInitData && activeTab === 'system' ? (
        <div className="am-stack">
          {systemError ? <div className="am-error">{systemError}</div> : null}
          <div className="am-two-col">
            <div className="am-card">
              <div className="am-card-header">
                <div className="am-card-title"><Settings2 size={14} /> 系统配置</div>
                <button type="button" className="am-pill active" onClick={handleSaveSystemConfig}>保存</button>
              </div>
              <div className="am-form">
                <label className="am-field">
                  <span>最大报价偏差 (%)</span>
                  <input value={configDraft.zap_price_deviation_max_percent} onChange={(e) => setConfigDraft((prev) => ({ ...prev, zap_price_deviation_max_percent: e.target.value }))} />
                </label>
                <label className="am-field">
                  <span>最低池子流动性 (USD)</span>
                  <input value={configDraft.zap_min_pool_liquidity_usd} onChange={(e) => setConfigDraft((prev) => ({ ...prev, zap_min_pool_liquidity_usd: e.target.value }))} />
                </label>
              </div>
              <div className="am-item-sub" style={{ marginTop: 8 }}>当前: 偏差 {systemConfig?.config?.zap_price_deviation_max_percent ?? '--'}, 流动性 {systemConfig?.config?.zap_min_pool_liquidity_usd ?? '--'}</div>
            </div>

            <div className="am-card">
              <div className="am-card-header">
                <div className="am-card-title"><Zap size={14} /> 私有 Zap</div>
                <span className="am-badge">{Array.isArray(privateZap?.chains) ? privateZap.chains.length : 0} 条链</span>
              </div>
              <div className="am-list">
                {Array.isArray(privateZap?.chains) && privateZap.chains.length > 0 ? privateZap.chains.map((chain) => (
                  <div key={chain} className="am-list-item">
                    <div>
                      <div className="am-item-title">{String(chain || '').toUpperCase()}</div>
                      <div className="am-item-sub">清空绑定地址与缓存</div>
                    </div>
                    <button type="button" className="am-action-btn" onClick={() => refreshSystemAfter(() => invalidateAdminPrivateZap({ apiBaseUrl, initData, chain }))}>清空</button>
                  </div>
                )) : <EmptyState text={systemLoading ? '正在加载...' : '暂无数据'} />}
              </div>
            </div>
          </div>

          <div className="am-card">
            <div className="am-card-header">
              <div className="am-card-title"><Server size={14} /> RPC 池</div>
              <button type="button" className="am-pill" onClick={loadSystem}>刷新</button>
            </div>
            <div className="am-form am-form-inline">
              <label className="am-field">
                <span>链</span>
                <select value={rpcDraft.chain} onChange={(e) => setRpcDraft((prev) => ({ ...prev, chain: e.target.value }))}>
                  <option value="bsc">BSC</option>
                  <option value="base">Base</option>
                </select>
              </label>
              <label className="am-field">
                <span>协议</span>
                <select value={rpcDraft.transport} onChange={(e) => setRpcDraft((prev) => ({ ...prev, transport: e.target.value }))}>
                  <option value="http">HTTP</option>
                  <option value="ws">WS</option>
                </select>
              </label>
              <label className="am-field am-field-grow">
                <span>URL</span>
                <input value={rpcDraft.url} onChange={(e) => setRpcDraft((prev) => ({ ...prev, url: e.target.value }))} placeholder="https:// 或 wss://" />
              </label>
              <label className="am-field">
                <span>名称</span>
                <input value={rpcDraft.name} onChange={(e) => setRpcDraft((prev) => ({ ...prev, name: e.target.value }))} placeholder="可选" />
              </label>
              <label className="am-field am-field-check">
                <input type="checkbox" checked={rpcDraft.setCurrent} onChange={(e) => setRpcDraft((prev) => ({ ...prev, setCurrent: e.target.checked }))} />
                <span>设为当前</span>
              </label>
              <button type="button" className="am-action-btn" onClick={handleAddRPC}><Plus size={12} /> 添加</button>
            </div>
            <div className="am-list">
              {Array.isArray(rpcPool?.groups) && rpcPool.groups.length > 0 ? rpcPool.groups.map((group) => (
                <div key={`${group.chain}:${group.transport}`} className="am-rpc-group">
                  <div className="am-rpc-group-head">
                    <div className="am-item-title">{String(group.chain || '').toUpperCase()} / {String(group.transport || '').toUpperCase()}</div>
                    <div className="am-item-sub">来源 {group.effective_source || '--'} · {group.effective_url_masked || '--'}</div>
                  </div>
                  <div className="am-list">
                    {Array.isArray(group.endpoints) && group.endpoints.length > 0 ? group.endpoints.map((endpoint) => {
                      const latency = Number(endpoint.last_latency_ms || 0);
                      const failures = Number(endpoint.consecutive_failures || 0);
                      const lastChecked = endpoint.last_checked_at ? new Date(endpoint.last_checked_at).toLocaleTimeString() : '';
                      const lastError = String(endpoint.last_error || '').trim();
                      const isAvailable = endpoint.status === 'available';
                      const statusLabel = endpoint.is_current ? '使用中' : isAvailable ? '可用' : '不可用';
                      const statusClass = endpoint.is_current ? 'am-badge-ok' : isAvailable ? '' : 'am-badge-warn';
                      return (
                        <div key={endpoint.id} className="am-list-item am-list-item-wrap">
                          <div style={{ flex: 1, minWidth: 0 }}>
                            <div className="am-item-title">{endpoint.name || `#${endpoint.id}`}</div>
                            <div className="am-item-sub">{endpoint.url_masked || endpoint.url || '--'}</div>
                            <div className="am-item-sub" style={{ marginTop: 3, display: 'flex', flexWrap: 'wrap', gap: '6px 12px', alignItems: 'center' }}>
                              {latency > 0 && (
                                <span style={{ color: latency < 200 ? '#59f09d' : latency < 500 ? '#ffae42' : '#ff6b6b', fontWeight: 600, fontSize: 11 }}>
                                  {latency}ms
                                </span>
                              )}
                              {lastChecked && <span>检测: {lastChecked}</span>}
                              {failures > 0 && <span style={{ color: '#ff6b6b' }}>连续失败 {failures} 次</span>}
                              {endpoint.disabled_until && <span style={{ color: '#ffae42' }}>停用至 {new Date(endpoint.disabled_until).toLocaleDateString()}</span>}
                            </div>
                            {lastError && (
                              <div className="am-item-sub" style={{ marginTop: 2, color: '#ff6b6b', wordBreak: 'break-all' }}>
                                {lastError.length > 80 ? lastError.slice(0, 80) + '...' : lastError}
                              </div>
                            )}
                          </div>
                          <div className="am-btn-group">
                            <span className={`am-badge ${statusClass}`}>{statusLabel}</span>
                            {!endpoint.is_current && <button type="button" className="am-icon-btn" title="切换为当前" onClick={() => refreshSystemAfter(() => switchAdminRPCEndpoint({ apiBaseUrl, initData, endpointId: endpoint.id }))}><ArrowRightLeft size={13} /></button>}
                            <button type="button" className="am-icon-btn" title="检测连通性" onClick={async (e) => {
                              const btn = e.currentTarget;
                              btn.disabled = true;
                              btn.style.opacity = '0.5';
                              try {
                                await refreshSystemAfter(() => checkAdminRPCEndpoint({ apiBaseUrl, initData, endpointId: endpoint.id }));
                              } finally {
                                btn.disabled = false;
                                btn.style.opacity = '';
                              }
                            }}><CheckCircle2 size={13} /></button>
                            {isAvailable && !endpoint.is_current && <button type="button" className="am-icon-btn" title="下月停用" onClick={() => refreshSystemAfter(() => disableAdminRPCEndpointNextMonth({ apiBaseUrl, initData, endpointId: endpoint.id }))}><Ban size={13} /></button>}
                            {!isAvailable && <button type="button" className="am-icon-btn" title="启用" onClick={() => refreshSystemAfter(() => enableAdminRPCEndpoint({ apiBaseUrl, initData, endpointId: endpoint.id }))}><CheckCircle2 size={13} /></button>}
                            {!endpoint.is_current && <button type="button" className="am-icon-btn am-icon-btn-danger" title="删除" onClick={() => refreshSystemAfter(() => deleteAdminRPCEndpoint({ apiBaseUrl, initData, endpointId: endpoint.id }))}><Trash2 size={13} /></button>}
                          </div>
                          <label className="am-rename">
                            <span>改名</span>
                            <input defaultValue={endpoint.name || ''} onBlur={(e) => {
                              const nextName = String(e.target.value || '').trim();
                              if (nextName && nextName !== String(endpoint.name || '').trim()) {
                                refreshSystemAfter(() => renameAdminRPCEndpoint({ apiBaseUrl, initData, endpointId: endpoint.id, name: nextName }));
                              }
                            }} />
                          </label>
                        </div>
                      );
                    }) : <EmptyState text="暂无端点" />}
                  </div>
                </div>
              )) : <EmptyState text={systemLoading ? '正在加载...' : '暂无 RPC 数据'} />}
            </div>
          </div>
        </div>
      ) : null}
    </PanelShell>
  );
}
