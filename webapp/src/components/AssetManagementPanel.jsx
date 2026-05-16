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
  Eraser,
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
  clearAssetLPPnLAdjustment,
  clearAssetLPPnLBaseline,
  saveAssetLPPnLAdjustment,
  saveAssetLPPnLBaseline,
  switchAdminRPCEndpoint,
  updateSystemConfig,
} from '../api';
import { resolveSMAvatarAssetUrl } from '../smartMoneyApi';
import PanelShell, { EmptyState, MetricCard } from './PanelShell';
import GlobalConfigPanel from './GlobalConfigPanel';
import WalletManagePanel from './WalletManagePanel';
import TradeHistoryPanel from './TradeHistoryPanel';

const HISTORY_WINDOWS = [7, 30, 90];
const PNL_CALENDAR_WINDOWS = [
  { key: 'month', label: '本月' },
  { key: '30d', label: '30天' },
];
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

function dayStamp(day) {
  const parts = String(day || '').split('-').map((item) => Number(item));
  if (parts.length !== 3 || parts.some((item) => !Number.isFinite(item))) return NaN;
  return Date.UTC(parts[0], parts[1] - 1, parts[2]);
}

function filterPointsByWindow(points, days) {
  if (!Array.isArray(points) || points.length === 0) return [];
  const end = dayStamp(points[points.length - 1]?.day);
  if (!Number.isFinite(end) || !Number.isFinite(Number(days)) || Number(days) <= 0) return points;
  const cutoff = end - (Number(days) - 1) * 24 * 60 * 60 * 1000;
  return points.filter((item) => {
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

function filterPnLCalendarPoints(points, windowKey) {
  if (!Array.isArray(points) || points.length === 0) return [];
  if (windowKey === '30d') {
    const today = dayStamp(formatChinaDay());
    if (!Number.isFinite(today)) return points;
    const start = today - 29 * 24 * 60 * 60 * 1000;
    return points.filter((item) => {
      const stamp = dayStamp(item?.day);
      return Number.isFinite(stamp) && stamp >= start && stamp <= today;
    });
  }
  const monthPrefix = formatChinaDay().slice(0, 7);
  return points.filter((item) => String(item?.day || '').startsWith(monthPrefix));
}

function summarizePnLCalendarPoints(points) {
  const summary = {
    total: 0,
    positiveDays: 0,
    negativeDays: 0,
  };
  if (!Array.isArray(points)) return summary;
  points.forEach((item) => {
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

function formatPrivateZapKind(kind) {
  const normalized = String(kind || '').trim().toLowerCase();
  if (normalized === 'atomic_increase_zap') return 'Atomic Increase Zap';
  if (normalized === 'zap_simple') return 'Zap Simple';
  return kind || '--';
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
    if (!seriesRef.current) return;
    const mapped = points
      ? points
      .filter((d) => d.day && Number.isFinite(Number(d.value)))
        .map((d) => ({ time: d.day, value: Number(d.value) }))
      : [];
    seriesRef.current.setData(mapped);
    if (mapped.length > 0) {
      chartRef.current?.timeScale().fitContent();
    }
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
    return <div className="am-chart-empty">加载中...</div>;
  }

  const cells = [];
  for (let i = 0; i < startOffset; i++) {
    cells.push(<div key={`e-${i}`} className="pnl-cal-cell pnl-cal-empty" />);
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
    const cls = ['pnl-cal-cell'];
    if (isToday) cls.push('pnl-cal-today');
    else if (isFuture) cls.push('pnl-cal-future');
    if (isSelected) cls.push('pnl-cal-selected');
    if (entry && onSelectDay) cls.push('pnl-cal-clickable');
    if (pnl !== null) cls.push(pnl >= 0 ? 'pnl-cal-pos' : 'pnl-cal-neg');
    cells.push(
      <button
        key={day}
        type="button"
        className={cls.join(' ')}
        disabled={!entry || isFuture}
        data-pnl-date-cell="true"
        data-pnl-selectable={entry && !isFuture ? 'true' : 'false'}
        onClick={() => {
          if (entry && !isFuture) onSelectDay?.(entry);
        }}
      >
        <div className="pnl-cal-day">{day}</div>
        <div className="pnl-cal-value">
          {pnl !== null ? `${pnl >= 0 ? '+' : ''}${formatUsdCompact(pnl)}` : '0'}
        </div>
        {(hasTransfer || hasManualAdjustment) && (
          <div className="pnl-cal-flags" aria-hidden="true">
            {hasTransfer ? <span className="pnl-cal-flag transfer" /> : null}
            {hasManualAdjustment ? <span className="pnl-cal-flag manual" /> : null}
          </div>
        )}
      </button>
    );
  }
  const remainder = (startOffset + daysInMonth) % 7;
  if (remainder > 0) {
    for (let i = 0; i < 7 - remainder; i++) {
      cells.push(<div key={`t-${i}`} className="pnl-cal-cell pnl-cal-empty" />);
    }
  }

  return (
    <div
      className="pnl-calendar"
      onPointerDown={(event) => {
        const target = event.target;
        if (!(target instanceof Element)) return;
        if (target.closest('[data-pnl-calendar-keep="true"]')) return;
        const dateCell = target.closest('[data-pnl-date-cell="true"]');
        if (dateCell?.getAttribute('data-pnl-selectable') === 'true') return;
        onDismiss?.();
      }}
    >
      <div className="pnl-cal-header">
        <div className="pnl-cal-month">
          <span>{monthLabel}</span>
          <svg width="14" height="14" fill="none" stroke="currentColor" strokeWidth="2" viewBox="0 0 24 24">
            <rect x="3" y="4" width="18" height="18" rx="2" /><line x1="16" y1="2" x2="16" y2="6" /><line x1="8" y1="2" x2="8" y2="6" /><line x1="3" y1="10" x2="21" y2="10" />
          </svg>
        </div>
        {allowNavigation ? (
          <div className="pnl-cal-nav" data-pnl-calendar-keep="true">
            <button type="button" onClick={prevMonth}><ChevronLeft size={14} /></button>
            <button type="button" onClick={nextMonth}><ChevronRight size={14} /></button>
          </div>
        ) : null}
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

function PnLBreakdownEditor({ entry, saving = false, error = '', onSave, onClear }) {
  const [manualValue, setManualValue] = useState('');
  const [note, setNote] = useState('');

  useEffect(() => {
    setManualValue(entry ? String(Number(entry.manual_adjustment_usd || 0)) : '');
    setNote(entry ? String(entry.adjustment_note || '') : '');
  }, [entry]);

  if (!entry) {
    return <EmptyState text="点击日历中的日期查看盈亏拆解" />;
  }

  const finalPnl = Number(entry.final_pnl_usd ?? entry.realized_pnl_usd ?? 0);
  const hasTransfer = Boolean(entry.transfer_total_count || entry.has_transfer_in || entry.has_transfer_out);
  const breakdown = [
    { label: '快照盈亏', value: entry.raw_pnl_usd, tone: Number(entry.raw_pnl_usd || 0) >= 0 ? 'positive' : 'negative' },
    { label: '手动校准', value: entry.manual_adjustment_usd, tone: Number(entry.manual_adjustment_usd || 0) >= 0 ? 'positive' : 'negative' },
    { label: '校准后', value: finalPnl, tone: finalPnl >= 0 ? 'positive' : 'negative' },
  ];

  return (
    <div className="pnl-editor" data-pnl-calendar-keep="true">
      <div className="pnl-editor-head">
        <div>
          <div className="am-card-title" style={{ fontSize: 12 }}>{entry.day} 盈亏校准</div>
          <div className="am-item-sub" style={{ margin: 0 }}>默认按每日资产快照差额计算；充值、提现等偏差在这里手动校准。</div>
        </div>
        <span className={`am-badge ${finalPnl >= 0 ? 'am-badge-ok' : 'am-badge-warn'}`}>
          {finalPnl >= 0 ? '+' : ''}{formatUsd(finalPnl)}
        </span>
      </div>
      <div className="pnl-editor-grid">
        {breakdown.map((item) => (
          <div key={item.label} className="pnl-editor-stat">
            <div className="pnl-editor-label">{item.label}</div>
            <div className={`pnl-editor-value ${item.tone}`}>{Number(item.value || 0) >= 0 ? '+' : ''}{formatUsdCompact(item.value)}</div>
          </div>
        ))}
      </div>
      {hasTransfer ? (
        <div className="pnl-editor-transfer-hint">
          检测到该日有转账记录：转入 {formatUsdCompact(entry.transfer_in_usd)}，转出 {formatUsdCompact(entry.transfer_out_usd)}。这些数据只作提示，不自动影响盈亏。
        </div>
      ) : null}
      <div className="pnl-editor-form">
        <label>
          <span>手动校准 USD</span>
          <input
            type="number"
            step="0.01"
            value={manualValue}
            onChange={(e) => setManualValue(e.target.value)}
            placeholder="0.00"
          />
        </label>
        <label>
          <span>备注</span>
          <input
            type="text"
            value={note}
            maxLength={500}
            onChange={(e) => setNote(e.target.value)}
            placeholder="例如：补扣未识别转出"
          />
        </label>
      </div>
      {error ? <div className="pnl-editor-error">{error}</div> : null}
      <div className="pnl-editor-actions">
        <button
          type="button"
          className="am-pill active"
          disabled={saving}
          onClick={() => onSave?.(entry.day, Number(manualValue || 0), note)}
        >
          <CheckCircle2 size={12} />
          保存修正
        </button>
        <button
          type="button"
          className="am-pill"
          disabled={saving}
          onClick={() => onClear?.(entry.day)}
        >
          <Eraser size={12} />
          清除
        </button>
      </div>
    </div>
  );
}

function ProfitBaselineEditor({ baseline, latestDay = '', saving = false, error = '', onSave, onClear }) {
  const [day, setDay] = useState('');
  const [baseValue, setBaseValue] = useState('');
  const [note, setNote] = useState('');

  useEffect(() => {
    setDay(baseline?.day || latestDay || '');
    setBaseValue(baseline ? String(Number(baseline.base_pnl_usd || 0)) : '0');
    setNote(baseline ? String(baseline.note || '') : '');
  }, [baseline, latestDay]);

  return (
    <div className="profit-baseline-editor">
      <div className="profit-baseline-head">
        <div>
          <div className="am-card-title" style={{ fontSize: 12 }}>总盈利起点</div>
          <div className="am-item-sub" style={{ margin: 0 }}>
            {baseline ? `${baseline.day} 起点 ${formatUsd(baseline.base_pnl_usd)}` : '未设置起点，曲线从已返回日盈亏累加'}
          </div>
        </div>
        {baseline ? <span className="am-badge">{formatChinaTime(baseline.updated_at)}</span> : null}
      </div>
      <div className="profit-baseline-form">
        <label>
          <span>起点日期</span>
          <input type="date" value={day} onChange={(e) => setDay(e.target.value)} />
        </label>
        <label>
          <span>起点盈利 USD</span>
          <input type="number" step="0.01" value={baseValue} onChange={(e) => setBaseValue(e.target.value)} placeholder="0.00" />
        </label>
        <label>
          <span>备注</span>
          <input type="text" maxLength={500} value={note} onChange={(e) => setNote(e.target.value)} placeholder="例如：从旧钱包迁移后重算" />
        </label>
      </div>
      {error ? <div className="pnl-editor-error">{error}</div> : null}
      <div className="profit-baseline-actions">
        <button
          type="button"
          className="am-pill active"
          disabled={saving}
          onClick={() => onSave?.(day, Number(baseValue || 0), note)}
        >
          <CheckCircle2 size={12} />
          保存起点
        </button>
        <button
          type="button"
          className="am-pill"
          disabled={saving}
          onClick={() => onClear?.()}
        >
          <Eraser size={12} />
          清除
        </button>
      </div>
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
    const list = [
      { key: 'my_assets', label: '我的资产', icon: Wallet },
      { key: 'global_config', label: '全局配置', icon: Settings2 },
      { key: 'wallet_manage', label: '钱包管理', icon: Wallet },
      { key: 'trade_history', label: '交易历史', icon: Settings2 },
    ];
    return list;
  }, []);

  const [activeTab, setActiveTab] = useState('my_assets');
  const [historyDays, setHistoryDays] = useState(30);
  const historyMetric = 'total_usd';
  const [trendMode, setTrendMode] = useState('assets');
  const [pnlCalendarWindow, setPnlCalendarWindow] = useState('month');
  const [assetState, setAssetState] = useState({ overview: null, history: null, lp: null });
  const [assetLoading, setAssetLoading] = useState(false);
  const [assetRefreshing, setAssetRefreshing] = useState(false);
  const [assetError, setAssetError] = useState('');
  const [selectedPnLDay, setSelectedPnLDay] = useState('');
  const [pnlAdjustmentSaving, setPnlAdjustmentSaving] = useState(false);
  const [pnlAdjustmentError, setPnlAdjustmentError] = useState('');
  const [profitBaselineSaving, setProfitBaselineSaving] = useState(false);
  const [profitBaselineError, setProfitBaselineError] = useState('');
  const [showProfitBaselineEditor, setShowProfitBaselineEditor] = useState(false);

  useEffect(() => {
    if (!tabs.some((tab) => tab.key === activeTab)) {
      setActiveTab('my_assets');
    }
  }, [activeTab, tabs]);

  const hasAssetData = Boolean(assetState.overview || assetState.history || assetState.lp);
  const hasAssetDataRef = useRef(false);

  useEffect(() => {
    hasAssetDataRef.current = hasAssetData;
  }, [hasAssetData]);

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

  useEffect(() => {
    if (activeTab !== 'my_assets') return undefined;
    loadAssets();
    if (!hasInitData) return undefined;
    const timer = setInterval(() => loadAssets(), Math.max(60, Number(refreshInterval || 10)) * 1000);
    return () => clearInterval(timer);
  }, [activeTab, hasInitData, loadAssets, refreshInterval]);

  const chartPoints = useMemo(() => {
    const rows = Array.isArray(assetState?.history?.history) ? [...assetState.history.history] : [];
    if (assetState?.history?.today?.day) rows.push(assetState.history.today);
    return rows.map((item) => ({ day: item.day, value: Number(item?.[historyMetric] || 0) }));
  }, [assetState.history, historyMetric]);
  const profitCurvePoints = useMemo(() => {
    const rows = Array.isArray(assetState?.lp?.profit_curve) ? assetState.lp.profit_curve : [];
    return rows.map((item) => ({ day: item.day, value: Number(item?.value_usd || 0) }));
  }, [assetState.lp]);
  const profitTrendPoints = useMemo(() => filterPointsByWindow(profitCurvePoints, historyDays), [profitCurvePoints, historyDays]);
  const activeTrendPoints = trendMode === 'profit' ? profitTrendPoints : chartPoints;
  const activeTrendValue = activeTrendPoints[activeTrendPoints.length - 1]?.value;
  const latestProfitCurveDay = profitCurvePoints[profitCurvePoints.length - 1]?.day || formatChinaDay();
  const pnlCalendarAllPoints = useMemo(() => mergeDailyPoints(assetState.lp?.daily_history, assetState.lp?.today_point), [assetState.lp]);
  const pnlCalendarPoints = useMemo(() => filterPnLCalendarPoints(pnlCalendarAllPoints, pnlCalendarWindow), [pnlCalendarAllPoints, pnlCalendarWindow]);
  const pnlCalendarSummary = useMemo(() => summarizePnLCalendarPoints(pnlCalendarPoints), [pnlCalendarPoints]);
  const pnlWindowLabel = PNL_CALENDAR_WINDOWS.find((item) => item.key === pnlCalendarWindow)?.label || '本月';

  const isRefreshing = assetLoading || assetRefreshing;
  const selectedPnLEntry = useMemo(() => {
    if (!selectedPnLDay) return null;
    const rows = Array.isArray(assetState.lp?.daily_history) ? assetState.lp.daily_history : [];
    const matched = rows.find((item) => item?.day === selectedPnLDay);
    if (matched) return matched;
    if (assetState.lp?.today_point?.day === selectedPnLDay) return assetState.lp.today_point;
    return null;
  }, [assetState.lp, selectedPnLDay]);

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
    if (selectedPnLDay && assetState.lp && !selectedPnLEntry) {
      setSelectedPnLDay('');
    }
  }, [assetState.lp, selectedPnLEntry, selectedPnLDay]);

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

  const subtitle = activeTab === 'global_config'
    ? '全局配置管理'
    : activeTab === 'wallet_manage'
      ? '钱包管理'
      : activeTab === 'trade_history'
        ? '交易历史记录'
        : '资产快照、历史趋势与 LP 统计';

  const metricColor = '#59f09d';

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
      {activeTab === 'my_assets' ? (
        <button
          type="button"
          className="am-tab-btn"
          disabled={isRefreshing}
          onClick={() => loadAssets({ forceRefresh: true })}
        >
          <RefreshCw size={12} className={isRefreshing ? 'animate-spin' : undefined} />
          刷新
        </button>
      ) : null}
    </div>
  );

  return (
    <PanelShell title="我的" subtitle={subtitle} icon={Wallet} actions={actions}>
      {!hasInitData ? <EmptyState text="请先完成 Telegram 登录后查看数据" /> : null}

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
              <div className="am-card-title">{trendMode === 'profit' ? '总盈利趋势' : '总资产趋势'}</div>
              <div className="am-trend-controls">
                <div className="am-pill-group">
                  <button type="button" className={`am-pill ${trendMode === 'assets' ? 'active' : ''}`} onClick={() => setTrendMode('assets')}>
                    总资产
                  </button>
                  <button type="button" className={`am-pill ${trendMode === 'profit' ? 'active' : ''}`} onClick={() => setTrendMode('profit')}>
                    总盈利
                  </button>
                </div>
                <div className="am-pill-group">
                  {HISTORY_WINDOWS.map((days) => (
                    <button key={days} type="button" className={`am-pill ${historyDays === days ? 'active' : ''}`} onClick={() => setHistoryDays(days)}>
                      {days}D
                    </button>
                  ))}
                </div>
              </div>
            </div>
            <div className="am-chart-header">
              <div>
                <div className="am-chart-label">{trendMode === 'profit' ? '总盈利' : '总资产'}</div>
                <div className="am-chart-value">{formatUsd(activeTrendValue)}</div>
              </div>
              <div className="am-chart-meta">
                <span className="am-badge">
                  {trendMode === 'profit' && assetState.lp?.profit_baseline ? `起点 ${assetState.lp.profit_baseline.day}` : formatChinaTime(assetState.overview?.updated_at)}
                </span>
                {trendMode === 'profit' ? (
                  <button
                    type="button"
                    className={`am-pill ${showProfitBaselineEditor ? 'active' : ''}`}
                    onClick={() => setShowProfitBaselineEditor((prev) => !prev)}
                  >
                    <Settings2 size={12} />
                    起点设置
                  </button>
                ) : null}
              </div>
            </div>
            <LWAreaChart points={activeTrendPoints} stroke={trendMode === 'profit' ? '#52d1ff' : metricColor} />
            {trendMode === 'profit' && showProfitBaselineEditor ? (
              <ProfitBaselineEditor
                baseline={assetState.lp?.profit_baseline}
                latestDay={latestProfitCurveDay}
                saving={profitBaselineSaving}
                error={profitBaselineError}
                onSave={handleSaveProfitBaseline}
                onClear={handleClearProfitBaseline}
              />
            ) : null}
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
            </div>

            <div className="am-card">
              <div className="am-card-header">
                <div>
                  <div className="am-card-title">盈亏日历</div>
                  <div className="am-item-sub">{pnlWindowLabel}已记录 {pnlCalendarPoints.length} 天</div>
                </div>
                <div className="am-pill-group" data-pnl-calendar-keep="true">
                  {PNL_CALENDAR_WINDOWS.map((item) => (
                    <button
                      key={item.key}
                      type="button"
                      className={`am-pill ${pnlCalendarWindow === item.key ? 'active' : ''}`}
                      onClick={() => setPnlCalendarWindow(item.key)}
                    >
                      {item.label}
                    </button>
                  ))}
                </div>
              </div>
              <div className="am-stat-grid am-stat-grid-3 pnl-window-summary">
                <div className="am-stat">
                  <div className="am-stat-label">{pnlWindowLabel}盈亏</div>
                  <div className="am-stat-value" style={{ color: pnlCalendarSummary.total >= 0 ? 'var(--positive)' : 'var(--negative)' }}>
                    {pnlCalendarSummary.total >= 0 ? '+' : ''}{formatUsdCompact(pnlCalendarSummary.total)}
                  </div>
                </div>
                <div className="am-stat am-stat-positive">
                  <div className="am-stat-label">盈利日</div>
                  <div className="am-stat-value">{pnlCalendarSummary.positiveDays}</div>
                </div>
                <div className="am-stat am-stat-negative">
                  <div className="am-stat-label">亏损日</div>
                  <div className="am-stat-value">{pnlCalendarSummary.negativeDays}</div>
                </div>
              </div>
              <PnLCalendar
                data={pnlCalendarPoints}
                loading={assetLoading}
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
              <div style={{ marginTop: 10 }}>
                <PnLBreakdownEditor
                  entry={selectedPnLEntry}
                  saving={pnlAdjustmentSaving}
                  error={pnlAdjustmentError}
                  onSave={handleSavePnLAdjustment}
                  onClear={handleClearPnLAdjustment}
                />
              </div>
              ) : null}
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

      {hasInitData && activeTab === 'global_config' ? (
        <GlobalConfigPanel apiBaseUrl={apiBaseUrl} initData={initData} hasInitData={hasInitData} embedded />
      ) : null}

      {hasInitData && activeTab === 'wallet_manage' ? (
        <WalletManagePanel apiBaseUrl={apiBaseUrl} initData={initData} hasInitData={hasInitData} chain="bsc" embedded />
      ) : null}

      {hasInitData && activeTab === 'trade_history' ? (
        <TradeHistoryPanel apiBaseUrl={apiBaseUrl} initData={initData} hasInitData={hasInitData} embedded />
      ) : null}
    </PanelShell>
  );
}
