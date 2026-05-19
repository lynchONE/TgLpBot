import React, { startTransition, useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { ChevronLeft, ChevronRight, Medal, RefreshCw, Search, Trophy } from 'lucide-react';
import {
  fetchAdminSmartMoneyLeaderboard,
  fetchAdminSmartMoneyOverview,
  fetchAdminSmartMoneyWallet,
} from '../api';
import { resolveSMAvatarAssetUrl } from '../smartMoneyApi';
import { EmptyState, MetricCard } from './PanelShell';

const WALLET_AVATAR_ICONS = Object.entries(
  import.meta.glob('../icon/avatar_*.png', { eager: true, import: 'default' })
)
  .sort(([pathA], [pathB]) => pathA.localeCompare(pathB, undefined, { numeric: true }))
  .map(([, src]) => src);

const SMART_MONEY_WINDOWS = [1, 7, 30];
const CHINA_TIME_ZONE = 'Asia/Shanghai';
const LEADERBOARD_METRICS = [
  { key: 'pnl', label: '收益额' },
  { key: 'yield_rate', label: '收益率' },
  { key: 'participation', label: '参与次数' },
];
const PAGE_SIZE = 10;

function errorText(err) {
  return String(err?.message || err || '').trim();
}

function isIgnorableSmartMoneyDataError(err) {
  const message = errorText(err).toLowerCase();
  return message.includes("unknown column 'open_lp_usd'") || message.includes('unknown column `open_lp_usd`');
}

function formatUsd(value) {
  const number = Number(value || 0);
  if (!Number.isFinite(number)) return '$--';
  return new Intl.NumberFormat('en-US', {
    style: 'currency',
    currency: 'USD',
    maximumFractionDigits: 2,
  }).format(number);
}

function formatUsdCompact(value) {
  const number = Number(value || 0);
  if (!Number.isFinite(number)) return '$--';
  const abs = Math.abs(number);
  if (abs >= 1000000) return `$${(number / 1000000).toFixed(abs >= 10000000 ? 0 : 1).replace(/\.0$/, '')}M`;
  if (abs >= 1000) return `$${(number / 1000).toFixed(abs >= 10000 ? 0 : 1).replace(/\.0$/, '')}K`;
  if (abs >= 100) return `$${number.toFixed(0)}`;
  if (abs >= 10) return `$${number.toFixed(1).replace(/\.0$/, '')}`;
  return `$${number.toFixed(2).replace(/0+$/, '').replace(/\.$/, '')}`;
}

function formatPct(value, digits = 2) {
  const number = Number(value || 0);
  if (!Number.isFinite(number)) return '--';
  return `${(number * 100).toFixed(digits).replace(/\.?0+$/, '')}%`;
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

function monthStart(value = new Date()) {
  const date = value instanceof Date ? value : new Date(value);
  if (Number.isNaN(date.getTime())) return new Date();
  return new Date(date.getFullYear(), date.getMonth(), 1);
}

function calendarHistoryDaysForMonth(value = new Date()) {
  const start = monthStart(value);
  const today = new Date();
  const todayStart = new Date(today.getFullYear(), today.getMonth(), today.getDate());
  const diffDays = Math.ceil((todayStart.getTime() - start.getTime()) / 86400000) + 32;
  return Math.max(30, Math.min(365, diffDays));
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

function walletSourceLabel(wallet) {
  const source = String(wallet?.source ?? wallet?.wallet_source ?? '').trim();
  if (source === 'manual') return '手动添加';
  if (source === 'contract_interaction') return '合约发现';
  return source || '未标记来源';
}

function walletSourceContractLabel(wallet) {
  const raw = String(wallet?.source_contract ?? wallet?.wallet_source_contract ?? '').trim();
  if (!/^0x[0-9a-fA-F]{40}$/.test(raw)) return '';
  return `来源合约 ${raw.slice(0, 6)}...${raw.slice(-4)}`;
}

function walletAvatarIdx(address) {
  const raw = String(address || '').trim();
  if (!WALLET_AVATAR_ICONS.length || raw.length < 6) return 0;
  return parseInt(raw.slice(-4), 16) % WALLET_AVATAR_ICONS.length;
}

function resolveWalletAvatarSrc(address, avatarUrl) {
  const preferred = resolveSMAvatarAssetUrl(avatarUrl);
  if (preferred) return preferred;
  return WALLET_AVATAR_ICONS[walletAvatarIdx(address)] || WALLET_AVATAR_ICONS[0] || '';
}

function WalletAvatar({ address, avatarUrl }) {
  const fallbackSrc = WALLET_AVATAR_ICONS[walletAvatarIdx(address)] || WALLET_AVATAR_ICONS[0] || '';
  const preferredSrc = resolveWalletAvatarSrc(address, avatarUrl);
  const [src, setSrc] = useState(preferredSrc);

  useEffect(() => {
    setSrc(preferredSrc);
  }, [preferredSrc]);

  if (!src) return null;

  return (
    <img
      src={src}
      alt=""
      style={{
        width: 30,
        height: 30,
        flexShrink: 0,
        borderRadius: 10,
        objectFit: 'cover',
        border: '1px solid rgba(136, 157, 191, 0.16)',
      }}
      onError={() => {
        if (src !== fallbackSrc) {
          setSrc(fallbackSrc);
        }
      }}
    />
  );
}

function RankBadge({ rank }) {
  if (rank === 1) {
    return (
      <span className="am-rank top">
        <Trophy size={12} />
      </span>
    );
  }
  if (rank === 2 || rank === 3) {
    return (
      <span className="am-rank top">
        <Medal size={12} />
      </span>
    );
  }
  return <span className="am-rank">{rank}</span>;
}

const PNL_CAL_WEEKDAYS = ['一', '二', '三', '四', '五', '六', '日'];

function PnLCalendar({ data, loading = false, viewDate, onMonthChange }) {
  const currentViewDate = viewDate instanceof Date ? viewDate : new Date();
  const changeMonth = useCallback((delta) => {
    const next = new Date(currentViewDate.getFullYear(), currentViewDate.getMonth() + delta, 1);
    if (typeof onMonthChange === 'function') {
      onMonthChange(next);
    }
  }, [currentViewDate, onMonthChange]);
  const year = currentViewDate.getFullYear();
  const month = currentViewDate.getMonth();
  const daysInMonth = new Date(year, month + 1, 0).getDate();
  const firstDayJS = new Date(year, month, 1).getDay();
  const startOffset = firstDayJS === 0 ? 6 : firstDayJS - 1;

  const pnlMap = useMemo(() => {
    const map = {};
    if (Array.isArray(data)) data.forEach((item) => {
      if (item?.day) map[item.day] = item;
    });
    return map;
  }, [data]);

  const monthLabel = new Intl.DateTimeFormat('en-US', {
    timeZone: CHINA_TIME_ZONE,
    year: 'numeric',
    month: 'short',
  }).format(new Date(Date.UTC(year, month, 1, 12, 0, 0)));
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
            <rect x="3" y="4" width="18" height="18" rx="2" />
            <line x1="16" y1="2" x2="16" y2="6" />
            <line x1="8" y1="2" x2="8" y2="6" />
            <line x1="3" y1="10" x2="21" y2="10" />
          </svg>
        </div>
        <div className="pnl-cal-nav">
          <button type="button" onClick={() => changeMonth(-1)}><ChevronLeft size={14} /></button>
          <button type="button" onClick={() => changeMonth(1)}><ChevronRight size={14} /></button>
        </div>
      </div>
      <div className="pnl-cal-grid">
        {PNL_CAL_WEEKDAYS.map((dayLabel) => (
          <div key={dayLabel} className="pnl-cal-weekday">{dayLabel}</div>
        ))}
        {cells}
      </div>
    </div>
  );
}

export default function SmartMoneyAssetsPanel({
  apiBaseUrl,
  initData,
  hasInitData,
  isAdmin = false,
  refreshInterval = 10,
}) {
  const [days, setDays] = useState(7);
  const [view, setView] = useState('wallets');
  const [walletKeyword, setWalletKeyword] = useState('');
  const [walletPage, setWalletPage] = useState(0);
  const [leaderboardKeyword, setLeaderboardKeyword] = useState('');
  const [leaderboardMetric, setLeaderboardMetric] = useState('pnl');
  const [leaderboardPage, setLeaderboardPage] = useState(0);
  const [selectedWalletId, setSelectedWalletId] = useState('');
  const [selectedWalletMeta, setSelectedWalletMeta] = useState(null);
  const [detailWalletId, setDetailWalletId] = useState('');
  const [overview, setOverview] = useState(null);
  const [leaderboard, setLeaderboard] = useState(null);
  const [walletDetail, setWalletDetail] = useState(null);
  const [detailCalendarMonth, setDetailCalendarMonth] = useState(() => monthStart(new Date()));
  const [loading, setLoading] = useState(false);
  const [refreshing, setRefreshing] = useState(false);
  const [detailLoading, setDetailLoading] = useState(false);
  const [error, setError] = useState('');

  const hasData = Boolean(overview || leaderboard || walletDetail);
  const hasDataRef = useRef(false);

  useEffect(() => {
    hasDataRef.current = hasData;
  }, [hasData]);

  const selectWallet = useCallback((wallet, { openDetail = false } = {}) => {
    if (!wallet) return;
    const id = walletKey(wallet);
    setSelectedWalletId(id);
    setSelectedWalletMeta(wallet);
    if (openDetail) {
      setView('wallets');
      setDetailWalletId(id);
      setDetailCalendarMonth(monthStart(new Date()));
    }
  }, []);

  const applyWalletRows = useCallback((rows) => {
    if (!Array.isArray(rows)) return;
    if (!selectedWalletId && rows[0]) {
      selectWallet(rows[0]);
      return;
    }
    const matchedWallet = rows.find((item) => walletKey(item) === selectedWalletId);
    if (matchedWallet) setSelectedWalletMeta(matchedWallet);
  }, [selectWallet, selectedWalletId]);

  const mergeOverview = useCallback((patch) => {
    if (!patch) return;
    setOverview((current) => ({ ...(current || {}), ...patch }));
  }, []);

  const loadSmartMoneySummary = useCallback(async ({ forceRefresh = false } = {}) => {
    if (!hasInitData || !isAdmin) return;
    try {
      const summary = await fetchAdminSmartMoneyOverview({
        apiBaseUrl,
        initData,
        days,
        section: 'summary',
        forceRefresh,
      });
      startTransition(() => {
        mergeOverview(summary || {});
      });
    } catch (err) {
      if (!isIgnorableSmartMoneyDataError(err)) setError(errorText(err));
    }
  }, [apiBaseUrl, days, hasInitData, initData, isAdmin, mergeOverview]);

  const loadSmartMoneyWallets = useCallback(async ({ forceRefresh = false } = {}) => {
    if (!hasInitData || !isAdmin) return;
    try {
      const wallets = await fetchAdminSmartMoneyOverview({
        apiBaseUrl,
        initData,
        days,
        page: walletPage + 1,
        pageSize: PAGE_SIZE,
        keyword: walletKeyword,
        section: 'wallets',
        forceRefresh,
      });
      startTransition(() => {
        mergeOverview(wallets || {});
      });
      applyWalletRows(Array.isArray(wallets?.wallets) ? wallets.wallets : []);
    } catch (err) {
      if (!isIgnorableSmartMoneyDataError(err)) setError(errorText(err));
    }
  }, [apiBaseUrl, applyWalletRows, days, hasInitData, initData, isAdmin, mergeOverview, walletKeyword, walletPage]);

  const loadSmartMoneyLeaderboard = useCallback(async ({ forceRefresh = false } = {}) => {
    if (!hasInitData || !isAdmin) return;
    try {
      const nextLeaderboard = await fetchAdminSmartMoneyLeaderboard({
        apiBaseUrl,
        initData,
        days: 1,
        metric: leaderboardMetric,
        page: leaderboardPage + 1,
        pageSize: PAGE_SIZE,
        keyword: leaderboardKeyword,
        forceRefresh,
      });
      startTransition(() => {
        setLeaderboard(nextLeaderboard || null);
      });
    } catch (err) {
      if (!isIgnorableSmartMoneyDataError(err)) setError(errorText(err));
    }
  }, [apiBaseUrl, hasInitData, initData, isAdmin, leaderboardKeyword, leaderboardMetric, leaderboardPage]);

  const loadSmartMoney = useCallback(async ({ forceRefresh = false } = {}) => {
    if (!hasInitData || !isAdmin) return;
    if (hasDataRef.current) setRefreshing(true);
    else setLoading(true);
    setError('');
    try {
      await Promise.allSettled([
        loadSmartMoneySummary({ forceRefresh }),
        loadSmartMoneyWallets({ forceRefresh }),
        loadSmartMoneyLeaderboard({ forceRefresh }),
      ]);
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  }, [
    hasInitData,
    isAdmin,
    loadSmartMoneyLeaderboard,
    loadSmartMoneySummary,
    loadSmartMoneyWallets,
  ]);

  const selectedWallet = useMemo(() => {
    const rows = Array.isArray(overview?.wallets) ? overview.wallets : [];
    return rows.find((item) => walletKey(item) === selectedWalletId) || selectedWalletMeta || null;
  }, [overview?.wallets, selectedWalletId, selectedWalletMeta]);

  const loadWalletDetail = useCallback(async ({ forceRefresh = false } = {}) => {
    if (!selectedWallet || !hasInitData || !isAdmin) return;
    setDetailLoading(true);
    const detailDays = Math.max(days, calendarHistoryDaysForMonth(detailCalendarMonth));
    try {
      const detail = await fetchAdminSmartMoneyWallet({
        apiBaseUrl,
        initData,
        address: selectedWallet.address,
        chainId: selectedWallet.chain_id,
        days: detailDays,
        forceRefresh,
      });
      startTransition(() => {
        setWalletDetail(detail || null);
      });
      setError('');
    } catch (err) {
      if (!isIgnorableSmartMoneyDataError(err)) {
        setError(errorText(err));
      }
    } finally {
      setDetailLoading(false);
    }
  }, [apiBaseUrl, days, detailCalendarMonth, hasInitData, initData, isAdmin, selectedWallet]);

  useEffect(() => {
    if (!hasInitData || !isAdmin) return undefined;
    const timer = setInterval(() => {
      loadSmartMoney();
      if (view === 'wallets' && detailWalletId && selectedWallet) {
        loadWalletDetail();
      }
    }, Math.max(60, Number(refreshInterval || 10)) * 1000);
    return () => clearInterval(timer);
  }, [detailWalletId, hasInitData, isAdmin, loadSmartMoney, loadWalletDetail, refreshInterval, selectedWallet, view]);

  useEffect(() => {
    if (view !== 'wallets' || !detailWalletId || !selectedWallet) return;
    loadWalletDetail();
  }, [detailWalletId, loadWalletDetail, selectedWallet, view]);

  useEffect(() => {
    setWalletPage(0);
  }, [days, walletKeyword]);

  useEffect(() => {
    loadSmartMoneySummary();
  }, [loadSmartMoneySummary]);

  useEffect(() => {
    loadSmartMoneyWallets();
  }, [loadSmartMoneyWallets]);

  useEffect(() => {
    setLeaderboardPage(0);
  }, [leaderboardKeyword, leaderboardMetric]);

  useEffect(() => {
    loadSmartMoneyLeaderboard();
  }, [loadSmartMoneyLeaderboard]);

  const walletRows = useMemo(
    () => (Array.isArray(overview?.wallets) ? overview.wallets : []),
    [overview?.wallets]
  );
  const walletTotalPages = Math.max(1, Number(overview?.wallet_total_pages || 1));
  const leaderboardRows = useMemo(
    () => (Array.isArray(leaderboard?.list) ? leaderboard.list : []),
    [leaderboard?.list]
  );
  const leaderboardTotalPages = Math.max(1, Number(leaderboard?.total_pages || 1));
  const historyCount = Array.isArray(walletDetail?.history) ? walletDetail.history.length : 0;
  const pnlCalendarData = useMemo(() => {
    const rows = Array.isArray(walletDetail?.history) ? [...walletDetail.history] : [];
    rows.sort((left, right) => String(left?.day || '').localeCompare(String(right?.day || '')));
    return rows.map((item) => ({
      day: item?.day,
      realized_pnl_usd: Number(item?.estimated_realized_pnl_usd || 0),
    }));
  }, [walletDetail?.history]);

  const isBusy = loading || refreshing;

  if (!hasInitData) {
    return <EmptyState text="请先完成 Telegram 登录后查看聪明钱资产。" />;
  }

  if (!isAdmin) {
    return <EmptyState text="聪明钱资产仅对管理员开放。" />;
  }

  return (
    <div className="am-stack">
      <div className="am-actions">
        {SMART_MONEY_WINDOWS.map((value) => (
          <button
            type="button"
            key={value}
            className={`am-pill ${days === value ? 'active' : ''}`}
            onClick={() => setDays(value)}
          >
            {value === 1 ? '昨日' : `${value}D`}
          </button>
        ))}
        <button
          type="button"
          className="am-tab-btn"
          disabled={isBusy}
          onClick={() => {
            loadSmartMoney({ forceRefresh: true });
            if (detailWalletId && selectedWallet) {
              loadWalletDetail({ forceRefresh: true });
            }
          }}
        >
          <RefreshCw size={12} className={isBusy ? 'animate-spin' : undefined} />
          刷新
        </button>
      </div>

      {error ? <div className="am-error">{error}</div> : null}

      <div className="am-metric-row">
        <MetricCard label="总资产" value={formatUsd(overview?.summary?.total_usd)} tone="strong" />
        <MetricCard label="原生币" value={formatUsd(overview?.summary?.native_usd)} />
        <MetricCard label="稳定币" value={formatUsd(overview?.summary?.stable_usd)} />
        <MetricCard label="代币持仓" value={formatUsd(overview?.summary?.tracked_token_usd)} />
        <MetricCard label="Open LP" value={formatUsd(overview?.summary?.open_lp_usd)} />
        <MetricCard label="代币种类" value={String(Number(overview?.summary?.tracked_token_count || 0))} />
      </div>

      <div className="am-actions">
        <button
          type="button"
          className={`am-tab-btn ${view === 'wallets' ? 'active' : ''}`}
          onClick={() => {
            setView('wallets');
            if (!detailWalletId) setWalletDetail(null);
          }}
        >
          钱包总览
        </button>
        <button
          type="button"
          className={`am-tab-btn ${view === 'leaderboard' ? 'active' : ''}`}
          onClick={() => {
            setView('leaderboard');
            setDetailWalletId('');
          }}
        >
          排行榜
        </button>
      </div>

      {view === 'wallets' && !detailWalletId ? (
        <div className="am-card">
          <div className="am-card-header">
            <div className="am-card-title">聪明钱钱包</div>
            <div className="am-item-sub">共 {Number(overview?.wallet_total || walletRows.length || 0)} 个</div>
          </div>
          <div className="am-form" style={{ gridTemplateColumns: 'minmax(0, 1fr)' }}>
            <label className="am-field">
              <span>搜索地址或标签</span>
              <div style={{ position: 'relative' }}>
                <Search
                  size={14}
                  style={{
                    position: 'absolute',
                    top: '50%',
                    left: 10,
                    transform: 'translateY(-50%)',
                    color: 'var(--text-muted)',
                  }}
                />
                <input
                  style={{ paddingLeft: 32 }}
                  value={walletKeyword}
                  onChange={(event) => setWalletKeyword(event.target.value)}
                  placeholder="地址 / 标签"
                />
              </div>
            </label>
          </div>

          <div className="am-list">
            {walletRows.length > 0 ? walletRows.map((wallet) => {
              const selected = walletKey(wallet) === selectedWalletId;
              return (
                <button
                  type="button"
                  key={walletKey(wallet)}
                  className={`am-list-item am-list-btn ${selected ? 'selected' : ''}`}
                  onClick={() => selectWallet(wallet, { openDetail: true })}
                >
                  <div className="am-rank-row" style={{ minWidth: 0 }}>
                    <WalletAvatar address={wallet.address} avatarUrl={wallet.avatar_url} />
                    <div style={{ minWidth: 0 }}>
                      <div className="am-item-title">{walletLabel(wallet)}</div>
                      <div className="am-item-sub">
                        {formatChain(wallet.chain_id)} / {walletSourceLabel(wallet)} / {Number(wallet.active_pool_count || 0)} 池 / {Number(wallet.today_event_count || 0)} 事件
                        {walletSourceContractLabel(wallet) ? ` / ${walletSourceContractLabel(wallet)}` : ''}
                      </div>
                    </div>
                  </div>
                  <div className="am-list-end">
                    <strong>{formatUsdCompact(wallet?.assets?.total_usd)}</strong>
                    <ChevronRight size={14} />
                  </div>
                </button>
              );
            }) : <EmptyState text={loading ? '正在加载聪明钱钱包...' : '暂无钱包数据'} />}
          </div>

          <div className="am-actions">
            <button
              type="button"
              className="am-action-btn"
              disabled={walletPage <= 0}
              onClick={() => setWalletPage((value) => Math.max(0, value - 1))}
            >
              上一页
            </button>
            <span className="am-item-sub">第 {walletPage + 1} / {walletTotalPages} 页</span>
            <button
              type="button"
              className="am-action-btn"
              disabled={walletPage >= walletTotalPages - 1}
              onClick={() => setWalletPage((value) => Math.min(walletTotalPages - 1, value + 1))}
            >
              下一页
            </button>
          </div>
        </div>
      ) : null}

      {view === 'wallets' && detailWalletId ? (
        <div className="am-stack">
          <div className="am-actions">
            <button
              type="button"
              className="am-action-btn"
              onClick={() => {
                setDetailWalletId('');
                setWalletDetail(null);
              }}
            >
              <ChevronLeft size={12} />
              返回列表
            </button>
          </div>

          <div className="am-wallet-item">
            <div className="am-wallet-head">
              <div className="am-rank-row" style={{ minWidth: 0 }}>
                <WalletAvatar
                  address={selectedWallet?.address}
                  avatarUrl={selectedWallet?.avatar_url || walletDetail?.wallet?.avatar_url}
                />
                <div style={{ minWidth: 0 }}>
                  <div className="am-item-title">{walletLabel(selectedWallet)}</div>
                  <div className="am-item-sub">
                    {formatChain(selectedWallet?.chain_id)} / {walletSourceLabel(walletDetail?.wallet || selectedWallet)} / {selectedWallet?.address || '--'}
                    {walletSourceContractLabel(walletDetail?.wallet || selectedWallet) ? ` / ${walletSourceContractLabel(walletDetail?.wallet || selectedWallet)}` : ''}
                  </div>
                </div>
              </div>
              <div className="am-wallet-total">
                <div className="am-item-sub">总资产</div>
                <strong>{formatUsdCompact(walletDetail?.wallet?.assets?.total_usd)}</strong>
              </div>
            </div>

            <div className="am-wallet-breakdown">
              <div className="am-wallet-cell">
                <span>今日收益</span>
                <strong>{formatUsd(walletDetail?.today?.estimated_realized_pnl_usd)}</strong>
              </div>
              <div className="am-wallet-cell">
                <span>加仓次数</span>
                <strong>{Number(walletDetail?.today?.add_count || 0)}</strong>
              </div>
              <div className="am-wallet-cell">
                <span>减仓次数</span>
                <strong>{Number(walletDetail?.today?.remove_count || 0)}</strong>
              </div>
            </div>
          </div>

          <div className="am-card">
            <div className="am-card-header">
              <div className="am-card-title">盈亏日历</div>
              <div className="am-item-sub">{detailLoading ? '加载中...' : `${historyCount} 天`}</div>
            </div>
            {pnlCalendarData.length > 0 ? (
              <PnLCalendar
                data={pnlCalendarData}
                loading={detailLoading}
                viewDate={detailCalendarMonth}
                onMonthChange={setDetailCalendarMonth}
              />
            ) : <EmptyState text={detailLoading ? '正在加载钱包详情...' : '暂无盈亏日历'} />}
          </div>
        </div>
      ) : null}

      {view === 'leaderboard' ? (
        <div className="am-card">
          <div className="am-card-header">
            <div className="am-card-title">昨日排行榜</div>
            <div className="am-actions" style={{ marginLeft: 'auto' }}>
              {LEADERBOARD_METRICS.map((metric) => (
                <button
                  type="button"
                  key={metric.key}
                  className={`am-pill ${leaderboardMetric === metric.key ? 'active' : ''}`}
                  onClick={() => setLeaderboardMetric(metric.key)}
                >
                  {metric.label}
                </button>
              ))}
            </div>
          </div>

          <div className="am-form" style={{ gridTemplateColumns: 'minmax(0, 1fr)' }}>
            <label className="am-field">
              <span>搜索地址或标签</span>
              <div style={{ position: 'relative' }}>
                <Search
                  size={14}
                  style={{
                    position: 'absolute',
                    top: '50%',
                    left: 10,
                    transform: 'translateY(-50%)',
                    color: 'var(--text-muted)',
                  }}
                />
                <input
                  style={{ paddingLeft: 32 }}
                  value={leaderboardKeyword}
                  onChange={(event) => setLeaderboardKeyword(event.target.value)}
                  placeholder="地址 / 标签"
                />
              </div>
            </label>
          </div>

          <div className="am-list">
            {leaderboardRows.length > 0 ? leaderboardRows.map((item, index) => {
              const rank = leaderboardPage * PAGE_SIZE + index + 1;
              const pnl = Number(item?.estimated_realized_pnl_usd || 0);
              const yieldRate = Number(item?.yield_rate || 0);
              const participationCount = Number(item?.participation_count || 0);
              const primaryColor = leaderboardMetric === 'participation'
                ? 'var(--text-primary)'
                : ((leaderboardMetric === 'yield_rate' ? yieldRate : pnl) >= 0 ? 'var(--positive)' : 'var(--negative)');
              let primaryText = `${pnl >= 0 ? '+' : ''}${formatUsdCompact(pnl)}`;
              let secondaryText = formatPct(yieldRate);
              if (leaderboardMetric === 'yield_rate') {
                primaryText = formatPct(yieldRate);
                secondaryText = `${pnl >= 0 ? '+' : ''}${formatUsdCompact(pnl)}`;
              } else if (leaderboardMetric === 'participation') {
                primaryText = `${participationCount} 次`;
                secondaryText = `${pnl >= 0 ? '+' : ''}${formatUsdCompact(pnl)}`;
              }
              return (
                <button
                  type="button"
                  key={`${item?.address || '--'}:${item?.chain_id || 0}`}
                  className={`am-list-item am-list-btn ${rank <= 3 ? 'am-top-rank' : ''}`}
                  onClick={() => selectWallet(item, { openDetail: true })}
                >
                  <div className="am-rank-row" style={{ minWidth: 0 }}>
                    <RankBadge rank={rank} />
                    <WalletAvatar address={item.address} avatarUrl={item.avatar_url} />
                    <div style={{ minWidth: 0 }}>
                      <div className="am-item-title">{walletLabel(item)}</div>
                      <div className="am-item-sub">
                        {formatChain(item.chain_id)} / {walletSourceLabel(item)} / 参与 {Number(item.participation_count || 0)} 次
                        {walletSourceContractLabel(item) ? ` / ${walletSourceContractLabel(item)}` : ''}
                      </div>
                    </div>
                  </div>
                  <div className="am-list-end" style={{ flexDirection: 'column', alignItems: 'flex-end', gap: 2 }}>
                    <strong style={{ color: primaryColor }}>
                      {primaryText}
                    </strong>
                    <span className="am-item-sub">{secondaryText}</span>
                  </div>
                </button>
              );
            }) : <EmptyState text={loading ? '正在加载排行榜...' : '暂无排行榜数据'} />}
          </div>

          <div className="am-actions">
            <button
              type="button"
              className="am-action-btn"
              disabled={leaderboardPage <= 0}
              onClick={() => setLeaderboardPage((value) => Math.max(0, value - 1))}
            >
              上一页
            </button>
            <span className="am-item-sub">第 {leaderboardPage + 1} / {leaderboardTotalPages} 页</span>
            <button
              type="button"
              className="am-action-btn"
              disabled={leaderboardPage >= leaderboardTotalPages - 1}
              onClick={() => setLeaderboardPage((value) => Math.min(leaderboardTotalPages - 1, value + 1))}
            >
              下一页
            </button>
          </div>
        </div>
      ) : null}
    </div>
  );
}
