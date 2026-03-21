import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { createChart, AreaSeries, HistogramSeries, ColorType } from 'lightweight-charts';
import {
  Activity,
  ArrowRightLeft,
  Ban,
  CheckCircle2,
  ChevronRight,
  ClipboardList,
  Crown,
  Medal,
  Plus,
  RefreshCw,
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
import PanelShell, { EmptyState, MetricCard } from './PanelShell';

const HISTORY_WINDOWS = [7, 30, 90];
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

function WalletAvatar({ address, size = 28 }) {
  const src = useMemo(() => walletAvatarUrl(address), [address]);
  if (!src) return null;
  return <img src={src} alt="" width={size} height={size} style={{ width: size, height: size, flexShrink: 0, borderRadius: size * 0.22, objectFit: 'cover' }} />;
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

  if (!points || points.length < 2) {
    return <div className="am-chart-empty">暂无趋势数据</div>;
  }

  return <div ref={containerRef} style={{ minHeight: height, borderRadius: 'var(--radius-md)', overflow: 'hidden' }} />;
}

/* ─── TradingView Histogram Chart for daily LP PnL ─── */
function LWHistogramChart({ data, height = 160 }) {
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
        scaleMargins: { top: 0.15, bottom: 0.05 },
      },
      timeScale: {
        borderVisible: false,
        fixLeftEdge: true,
        fixRightEdge: true,
        timeVisible: false,
      },
      handleScroll: false,
      handleScale: false,
    });

    const series = chart.addSeries(HistogramSeries, {
      priceFormat: {
        type: 'custom',
        formatter: (v) => {
          const abs = Math.abs(v);
          if (abs >= 1000) return `$${(v / 1000).toFixed(1)}K`;
          return `$${v.toFixed(0)}`;
        },
      },
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
    };
  }, [height]);

  useEffect(() => {
    if (!seriesRef.current || !data || data.length < 1) return;
    const mapped = data
      .filter((d) => d.day)
      .map((d) => ({
        time: d.day,
        value: Number(d.realized_pnl_usd || 0),
        color: Number(d.realized_pnl_usd || 0) >= 0 ? 'rgba(34, 211, 138, 0.7)' : 'rgba(255, 94, 118, 0.7)',
      }));
    seriesRef.current.setData(mapped);
    chartRef.current?.timeScale().fitContent();
  }, [data]);

  if (!data || data.length === 0) {
    return <div className="am-chart-empty">暂无每日 LP 数据</div>;
  }

  return <div ref={containerRef} style={{ minHeight: height, borderRadius: 'var(--radius-md)', overflow: 'hidden' }} />;
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
    return wallets.map((w, i) => ({
      label: w.wallet_address ? `${w.wallet_address.slice(0, 6)}...${w.wallet_address.slice(-4)}` : `#${w.wallet_id}`,
      chain: String(w.chain || '').toUpperCase(),
      value: Math.max(0, Number(w.total_usd || 0)),
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
            <span className="am-item-sub" style={{ margin: 0, flex: 1, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{a.label} · {a.chain}</span>
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
    return wallets.map((w) => ({
      label: w.wallet_address ? `${w.wallet_address.slice(0, 6)}...${w.wallet_address.slice(-4)}` : `#${w.wallet_id}`,
      chain: String(w.chain || '').toUpperCase(),
      native: Math.max(0, Number(w.native_usd || 0)),
      stable: Math.max(0, Number(w.stable_usd || 0)),
      token: Math.max(0, Number(w.token_usd || 0)),
      total: Math.max(0, Number(w.total_usd || 0)),
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
              <span style={{ fontSize: 11, fontWeight: 600, color: 'var(--text)' }}>{item.label} <span style={{ fontSize: 9, fontWeight: 400, color: 'var(--text-muted)' }}>{item.chain}</span></span>
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
  const [historyMetric, setHistoryMetric] = useState('total_usd');
  const [assetState, setAssetState] = useState({ overview: null, history: null, lp: null });
  const [assetLoading, setAssetLoading] = useState(false);
  const [assetError, setAssetError] = useState('');

  const [smartMoneyDays, setSmartMoneyDays] = useState(7);
  const [leaderboardMetric, setLeaderboardMetric] = useState('pnl');
  const [smartMoneyOverview, setSmartMoneyOverview] = useState(null);
  const [smartMoneyWallet, setSmartMoneyWallet] = useState(null);
  const [smartMoneyLeaderboard, setSmartMoneyLeaderboard] = useState(null);
  const [smartMoneyLoading, setSmartMoneyLoading] = useState(false);
  const [smartMoneyError, setSmartMoneyError] = useState('');
  const [selectedWalletId, setSelectedWalletId] = useState('');

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

  const loadAssets = useCallback(async () => {
    if (!hasInitData) return;
    setAssetLoading(true);
    setAssetError('');
    try {
      const [overview, history, lp] = await Promise.all([
        fetchAssetOverview({ apiBaseUrl, initData }),
        fetchAssetHistory({ apiBaseUrl, initData, days: historyDays }),
        fetchAssetLPStats({ apiBaseUrl, initData }),
      ]);
      setAssetState({ overview: overview || null, history: history || null, lp: lp || null });
    } catch (err) {
      setAssetError(String(err?.message || err));
    } finally {
      setAssetLoading(false);
    }
  }, [apiBaseUrl, hasInitData, historyDays, initData]);

  const loadSmartMoney = useCallback(async () => {
    if (!hasInitData || !isAdmin) return;
    setSmartMoneyLoading(true);
    setSmartMoneyError('');
    try {
      const [overviewResult, leaderboardResult] = await Promise.allSettled([
        fetchAdminSmartMoneyOverview({ apiBaseUrl, initData, days: smartMoneyDays }),
        fetchAdminSmartMoneyLeaderboard({ apiBaseUrl, initData, days: smartMoneyDays, metric: leaderboardMetric, limit: 20 }),
      ]);
      const overview = overviewResult.status === 'fulfilled' ? overviewResult.value : null;
      const leaderboard = leaderboardResult.status === 'fulfilled' ? leaderboardResult.value : null;
      const wallets = Array.isArray(overview?.wallets) ? overview.wallets : [];
      setSmartMoneyOverview(overview || null);
      setSmartMoneyLeaderboard(leaderboard || null);
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
    }
  }, [apiBaseUrl, hasInitData, initData, isAdmin, leaderboardMetric, selectedWalletId, smartMoneyDays]);

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
    const timer = setInterval(loadAssets, Math.max(15, Number(refreshInterval || 10)) * 1000);
    return () => clearInterval(timer);
  }, [activeTab, hasInitData, loadAssets, refreshInterval]);

  useEffect(() => {
    if (activeTab !== 'smart_money') return undefined;
    loadSmartMoney();
    if (!hasInitData || !isAdmin) return undefined;
    const timer = setInterval(loadSmartMoney, Math.max(20, Number(refreshInterval || 10)) * 1000);
    return () => clearInterval(timer);
  }, [activeTab, hasInitData, isAdmin, loadSmartMoney, refreshInterval]);

  useEffect(() => {
    const wallets = Array.isArray(smartMoneyOverview?.wallets) ? smartMoneyOverview.wallets : [];
    const current = wallets.find((item) => walletKey(item) === selectedWalletId);
    if (activeTab !== 'smart_money' || !current || !hasInitData || !isAdmin) return undefined;
    let disposed = false;
    const run = async () => {
      try {
        const detail = await fetchAdminSmartMoneyWallet({
          apiBaseUrl,
          initData,
          address: current.address,
          chainId: current.chain_id,
          days: smartMoneyDays,
        });
        if (!disposed) setSmartMoneyWallet(detail || null);
      } catch (err) {
        if (!disposed) {
          setSmartMoneyWallet(null);
          if (isIgnorableSmartMoneyDataError(err)) {
            setSmartMoneyError('');
          } else {
            setSmartMoneyError(errorText(err));
          }
        }
      }
    };
    run();
    const timer = setInterval(run, Math.max(20, Number(refreshInterval || 10)) * 1000);
    return () => {
      disposed = true;
      clearInterval(timer);
    };
  }, [activeTab, apiBaseUrl, hasInitData, initData, isAdmin, refreshInterval, selectedWalletId, smartMoneyDays, smartMoneyOverview]);

  useEffect(() => {
    if (activeTab !== 'operations') return undefined;
    loadOperations();
    if (!hasInitData || !isAdmin) return undefined;
    const timer = setInterval(loadOperations, Math.max(20, Number(refreshInterval || 10)) * 1000);
    return () => clearInterval(timer);
  }, [activeTab, hasInitData, isAdmin, loadOperations, refreshInterval]);

  useEffect(() => {
    if (activeTab !== 'system') return undefined;
    loadSystem();
    if (!hasInitData || !isAdmin) return undefined;
    const timer = setInterval(loadSystem, Math.max(30, Number(refreshInterval || 10)) * 1000);
    return () => clearInterval(timer);
  }, [activeTab, hasInitData, isAdmin, loadSystem, refreshInterval]);

  const chartPoints = useMemo(() => {
    const rows = Array.isArray(assetState?.history?.history) ? [...assetState.history.history] : [];
    if (assetState?.history?.today?.day) rows.push(assetState.history.today);
    return rows.map((item) => ({ day: item.day, value: Number(item?.[historyMetric] || 0) }));
  }, [assetState.history, historyMetric]);

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

  const selectedWallet = useMemo(() => {
    const wallets = Array.isArray(smartMoneyOverview?.wallets) ? smartMoneyOverview.wallets : [];
    return wallets.find((item) => walletKey(item) === selectedWalletId) || null;
  }, [selectedWalletId, smartMoneyOverview]);

  const subtitle = activeTab === 'smart_money'
    ? '聪明钱资产、排行榜与钱包详情'
    : activeTab === 'operations'
      ? '在线用户、活跃任务与用户持仓'
      : activeTab === 'system'
        ? '系统配置、RPC 与 Private Zap'
        : '资产快照、历史趋势与 LP 统计';

  const metricColor = HISTORY_METRICS.find((m) => m.key === historyMetric)?.color || '#59f09d';

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
        onClick={() => {
          if (activeTab === 'my_assets') loadAssets();
          if (activeTab === 'smart_money') loadSmartMoney();
          if (activeTab === 'operations') loadOperations();
          if (activeTab === 'system') loadSystem();
        }}
      >
        <RefreshCw size={12} />
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
            <div className="am-pill-group am-pill-group-wrap">
              {HISTORY_METRICS.map((item) => (
                <button key={item.key} type="button" className={`am-pill ${historyMetric === item.key ? 'active' : ''}`} onClick={() => setHistoryMetric(item.key)}>
                  {item.label}
                </button>
              ))}
            </div>
            <div className="am-chart-header">
              <div>
                <div className="am-chart-label">{HISTORY_METRICS.find((item) => item.key === historyMetric)?.label || '总资产'}</div>
                <div className="am-chart-value">{formatUsd(chartPoints[chartPoints.length - 1]?.value)}</div>
              </div>
              <span className="am-badge">{assetState.overview?.updated_at ? new Date(assetState.overview.updated_at).toLocaleTimeString() : ''}</span>
            </div>
            <LWAreaChart points={chartPoints} stroke={metricColor} />
          </div>

          <div className="am-two-col">
            <div className="am-card">
              <div className="am-card-header">
                <div className="am-card-title">今日盈亏</div>
                <span className={`am-badge ${Number(assetState.lp?.today?.realized_pnl_usd || 0) >= 0 ? 'am-badge-ok' : 'am-badge-warn'}`}>
                  {Number(assetState.lp?.today?.realized_pnl_usd || 0) >= 0 ? '+' : ''}{formatUsd(assetState.lp?.today?.realized_pnl_usd)}
                </span>
              </div>
              <div className="am-stat-grid">
                <div className="am-stat">
                  <div className="am-stat-label">平仓笔数</div>
                  <div className="am-stat-value">{Number(assetState.lp?.today?.closed_count || 0)}</div>
                </div>
                <div className="am-stat">
                  <div className="am-stat-label">胜率</div>
                  <div className="am-stat-value">{formatPct(assetState.lp?.today?.win_rate)}</div>
                  <div className="am-stat-sub">{Number(assetState.lp?.today?.win_count || 0)}W / {Number(assetState.lp?.today?.loss_count || 0)}L</div>
                </div>
              </div>
              {Array.isArray(assetState.lp?.today_pools) && assetState.lp.today_pools.length > 0 ? (
                <div className="am-list" style={{ marginTop: 10 }}>
                  <div className="am-item-sub" style={{ margin: 0, marginBottom: 4 }}>各池子盈亏</div>
                  {assetState.lp.today_pools.map((pool, i) => {
                    const pnl = Number(pool.profit_usd || 0);
                    return (
                      <div key={`${pool.pool_id}-${i}`} className="am-list-item">
                        <div>
                          <div className="am-item-title">{pool.token0_symbol || '?'}/{pool.token1_symbol || '?'}</div>
                          <div className="am-item-sub">{String(pool.chain || 'bsc').toUpperCase()} · {pool.closed_count || 0} 笔</div>
                        </div>
                        <span className={`am-badge ${pnl >= 0 ? 'am-badge-ok' : 'am-badge-warn'}`}>{pnl >= 0 ? '+' : ''}{formatUsd(pnl)}</span>
                      </div>
                    );
                  })}
                </div>
              ) : <EmptyState text="今日暂无平仓记录" />}
            </div>

            <div className="am-card">
              <div className="am-card-title">每日 LP 盈亏</div>
              <div className="am-item-sub" style={{ margin: 0 }}>近30天每日平仓盈亏柱状图</div>
              {Array.isArray(assetState.lp?.daily_history) && assetState.lp.daily_history.length > 0 ? (
                <LWHistogramChart data={assetState.lp.daily_history} />
              ) : <EmptyState text={assetLoading ? '正在加载...' : '暂无每日数据'} />}
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

          <div className="am-two-col">
            <div className="am-card">
              <div className="am-card-header">
                <div className="am-card-title">钱包总览</div>
                <span className="am-badge">{Array.isArray(smartMoneyOverview?.wallets) ? smartMoneyOverview.wallets.length : 0} 个</span>
              </div>
              <div className="am-list">
                {Array.isArray(smartMoneyOverview?.wallets) && smartMoneyOverview.wallets.length > 0 ? smartMoneyOverview.wallets.map((wallet) => {
                  const assets = wallet.assets || {};
                  const total = Number(assets.total_usd || 0);
                  const nativePct = total > 0 ? (Number(assets.native_usd || 0) / total * 100) : 0;
                  const stablePct = total > 0 ? (Number(assets.stable_usd || 0) / total * 100) : 0;
                  const tokenPct = total > 0 ? (Number(assets.tracked_token_usd || 0) / total * 100) : 0;
                  const lpPct = total > 0 ? (Number(assets.open_lp_usd || 0) / total * 100) : 0;
                  return (
                    <button key={walletKey(wallet)} type="button" className={`am-list-item am-list-btn ${walletKey(wallet) === selectedWalletId ? 'selected' : ''}`} onClick={() => setSelectedWalletId(walletKey(wallet))}>
                      <div style={{ display: 'flex', alignItems: 'center', gap: 10, flex: 1, minWidth: 0 }}>
                        <WalletAvatar address={wallet.address} size={28} />
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
            </div>

            <div className="am-card">
              <div className="am-card-title">钱包详情</div>
              {selectedWallet && smartMoneyWallet ? (
                <div className="am-stack">
                  <div style={{ display: 'flex', alignItems: 'center', gap: 12, padding: '8px 0' }}>
                    <WalletAvatar address={selectedWallet.address} size={36} />
                    <div style={{ flex: 1, minWidth: 0 }}>
                      <div className="am-item-title" style={{ fontSize: 13 }}>{walletLabel(selectedWallet)}</div>
                      <div className="am-item-sub">{formatChain(selectedWallet.chain_id)} · 今日收益 {formatUsd(smartMoneyWallet.today?.estimated_realized_pnl_usd)}</div>
                    </div>
                    <strong style={{ fontSize: 16 }}>{formatUsdCompact(smartMoneyWallet.wallet?.assets?.total_usd)}</strong>
                  </div>
                  <div className="am-stat-grid">
                    <div className="am-stat">
                      <div className="am-stat-label">今日收益</div>
                      <div className="am-stat-value">{formatUsd(smartMoneyWallet.today?.estimated_realized_pnl_usd)}</div>
                    </div>
                    <div className="am-stat">
                      <div className="am-stat-label">Add / Remove</div>
                      <div className="am-stat-value">{Number(smartMoneyWallet.today?.add_count || 0)} / {Number(smartMoneyWallet.today?.remove_count || 0)}</div>
                    </div>
                  </div>
                  <div className="am-chart-box">
                    <div className="am-chart-label">30 天趋势</div>
                    <SparklineChart points={Array.isArray(smartMoneyWallet.history) ? smartMoneyWallet.history.map((item) => ({ value: Number(item?.total_usd || 0) })) : []} />
                  </div>
                  <div className="am-stat-grid am-stat-grid-3">
                    {Array.isArray(smartMoneyWallet.windows) ? smartMoneyWallet.windows.map((item) => (
                      <div key={item.days} className="am-stat">
                        <div className="am-stat-label">{item.days}D</div>
                        <div className="am-stat-value">{formatUsd(item.estimated_realized_pnl_usd)}</div>
                        <div className="am-stat-sub">{formatPct(item.yield_rate)} · {Number(item.active_pool_count || 0)} 池</div>
                      </div>
                    )) : null}
                  </div>
                </div>
              ) : <EmptyState text="选择钱包查看明细" />}
            </div>
          </div>

          <div className="am-card">
            <div className="am-card-header">
              <div className="am-card-title">排行榜</div>
              <div className="am-pill-group">
                {LEADERBOARD_METRICS.map((item) => (
                  <button key={item.key} type="button" className={`am-pill ${leaderboardMetric === item.key ? 'active' : ''}`} onClick={() => setLeaderboardMetric(item.key)}>
                    {item.label}
                  </button>
                ))}
              </div>
            </div>
            <div className="am-list">
              {Array.isArray(smartMoneyLeaderboard?.list) && smartMoneyLeaderboard.list.length > 0 ? smartMoneyLeaderboard.list.map((item) => {
                const metricText = leaderboardMetric === 'yield_rate' ? formatPct(item.metric_value) : leaderboardMetric === 'participation' ? `${Number(item.metric_value || 0)} 次` : formatUsd(item.metric_value);
                const pnl = Number(item.estimated_realized_pnl_usd || 0);
                return (
                  <div key={`${item.rank}:${item.address}`} className={`am-list-item ${item.rank <= 3 ? 'am-top-rank' : ''}`}>
                    <div className="am-rank-row">
                      <RankBadge rank={Number(item.rank || 0)} />
                      <WalletAvatar address={item.address} size={30} />
                      <div>
                        <div className="am-item-title">{item.label || `${item.address.slice(0, 6)}...${item.address.slice(-4)}`}</div>
                        <div className="am-item-sub">{formatChain(item.chain_id)} · {Number(item.active_pool_count || 0)} 池 · {Number(item.participation_count || 0)} 次操作</div>
                      </div>
                    </div>
                    <div className="am-list-end" style={{ flexDirection: 'column', alignItems: 'flex-end', gap: 2 }}>
                      <strong style={{ color: leaderboardMetric === 'pnl' ? (pnl >= 0 ? '#59f09d' : '#ff6b6b') : undefined }}>{metricText}</strong>
                      {leaderboardMetric !== 'pnl' && <span className="am-item-sub" style={{ color: pnl >= 0 ? '#59f09d' : '#ff6b6b' }}>{pnl >= 0 ? '+' : ''}{formatUsdCompact(pnl)}</span>}
                      {leaderboardMetric === 'pnl' && Number(item.yield_rate || 0) !== 0 && <span className="am-item-sub">{formatPct(item.yield_rate)}</span>}
                    </div>
                  </div>
                );
              }) : <EmptyState text={smartMoneyLoading ? '正在加载...' : '暂无排行榜数据'} />}
            </div>
          </div>
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
                <div className="am-card-title"><Zap size={14} /> Private Zap</div>
                <span className="am-badge">{Array.isArray(privateZap?.chains) ? privateZap.chains.length : 0} 条链</span>
              </div>
              <div className="am-list">
                {Array.isArray(privateZap?.chains) && privateZap.chains.length > 0 ? privateZap.chains.map((chain) => (
                  <div key={chain} className="am-list-item">
                    <div>
                      <div className="am-item-title">{String(chain || '').toUpperCase()}</div>
                      <div className="am-item-sub">清空绑定地址与缓存</div>
                    </div>
                    <button type="button" className="am-action-btn" onClick={() => refreshSystemAfter(() => invalidateAdminPrivateZap({ apiBaseUrl, initData, chain }))}>Invalidate</button>
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
                    {Array.isArray(group.endpoints) && group.endpoints.length > 0 ? group.endpoints.map((endpoint) => (
                      <div key={endpoint.id} className="am-list-item am-list-item-wrap">
                        <div>
                          <div className="am-item-title">{endpoint.name || `#${endpoint.id}`}</div>
                          <div className="am-item-sub">{endpoint.url_masked || endpoint.url || '--'}</div>
                        </div>
                        <div className="am-btn-group">
                          <span className={`am-badge ${endpoint.is_current ? 'am-badge-ok' : ''}`}>{endpoint.is_current ? 'IN USE' : endpoint.status || '--'}</span>
                          <button type="button" className="am-icon-btn" title="切换" onClick={() => refreshSystemAfter(() => switchAdminRPCEndpoint({ apiBaseUrl, initData, endpointId: endpoint.id }))}><ArrowRightLeft size={13} /></button>
                          <button type="button" className="am-icon-btn" title="检测" onClick={() => refreshSystemAfter(() => checkAdminRPCEndpoint({ apiBaseUrl, initData, endpointId: endpoint.id }))}><CheckCircle2 size={13} /></button>
                          <button type="button" className="am-icon-btn" title="下月停用" onClick={() => refreshSystemAfter(() => disableAdminRPCEndpointNextMonth({ apiBaseUrl, initData, endpointId: endpoint.id }))}><Ban size={13} /></button>
                          <button type="button" className="am-icon-btn" title="启用" onClick={() => refreshSystemAfter(() => enableAdminRPCEndpoint({ apiBaseUrl, initData, endpointId: endpoint.id }))}><CheckCircle2 size={13} /></button>
                          <button type="button" className="am-icon-btn am-icon-btn-danger" title="删除" onClick={() => refreshSystemAfter(() => deleteAdminRPCEndpoint({ apiBaseUrl, initData, endpointId: endpoint.id }))}><Trash2 size={13} /></button>
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
                    )) : <EmptyState text="暂无端点" />}
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
