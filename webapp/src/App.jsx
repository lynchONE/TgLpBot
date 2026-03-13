import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
  AlertTriangle,
  BrainCircuit,
  BriefcaseBusiness,
  CandlestickChart,
  Check,
  Copy,
  Flame,
  GripVertical,
  Layers3,
  Link2,
  LogOut,
  Maximize,
  Minimize,
  RefreshCw,
  Search,
  Settings,
  SlidersHorizontal,
  X,
} from 'lucide-react';
import {
  checkLoginCode,
  deleteTask,
  fetchHotPools,
  fetchRealtimePositions,
  fetchSmartMoneyOverview,
  fetchSmartMoneyPoolAdds,
  fetchSmartMoneyPoolMarkers,
  fetchMyTradeMarkers,
  fetchTokenCandles,
  fetchWallets,
  generateLoginCode,
  openPosition as apiOpenPosition,
  setTaskPaused,
  stopTask,
  updateTaskRange,
} from './api';
import { WEBAPP_CONFIG } from './config';
import PanelShell, { EmptyState, MetricCard } from './components/PanelShell';
import KlineChart from './components/KlineChart';
import OpenPositionModal from './components/OpenPositionModal';
import StepProgressModal from './components/StepProgressModal';
import TaskActionMenu from './components/TaskActionMenu';
import NumberFlowValue from './components/NumberFlowValue';
import telegramLogo from './img/telegram.svg';
import uniswapLogo from './img/uniswap.svg';
import pancakeLogo from './img/pancake.svg';
import bnbLogo from './img/bnb.svg';
import baseLogo from './img/base.svg';
import flashIcon from './img/flash.svg';
import {
  DEFAULT_WIDGETS,
  WIDGETS,
  buildGmgnUrl,
  compactPrice,
  formatNumber,
  formatPct,
  formatPriceDisplay,
  formatUtc8DateTime,
  formatUtc8Time,
  formatUsd,
  formatUsdCompact,
  normalizePoolAddress,
  normalizeHexAddress,
  normalizeWidgetSelection,
  pickNonStableTokenAddress,
  resolveHotPoolFilterToken,
  resolveKlineTokenOptions,
  shortAddress,
  inferPoolVersion,
  computePriceRange,
  formatDuration,
  toUnixSeconds,
} from './utils';

const KLINE_INTERVALS = [
  { key: '1m', label: '1m', bucketSec: 60, limit: 240, timeframe: 'minute', aggregate: 1, poolLimit: 300 },
  { key: '5m', label: '5m', bucketSec: 300, limit: 240, timeframe: 'minute', aggregate: 5, poolLimit: 260 },
  { key: '15m', label: '15m', bucketSec: 900, limit: 240, timeframe: 'minute', aggregate: 15, poolLimit: 220 },
  { key: '1H', label: '1H', bucketSec: 3600, limit: 240, timeframe: 'hour', aggregate: 1, poolLimit: 200 },
];
const SMART_POOL_WINDOW_HOURS = 24;
const SMART_PNL_WINDOW_HOURS = 24;
const HOT_POOLS_DISPLAY_LIMIT = 20;
const KLINE_MARKER_WINDOW_HOURS = 24;
const KLINE_MARKER_FETCH_LIMIT = 1200;
const KLINE_MARKER_RANGE_DEBOUNCE_MS = 240;
const KLINE_MARKER_LIVE_REFRESH_MS = 4000;
const KLINE_MARKER_IDLE_REFRESH_MS = 12000;
const ACCENT_THEMES = [
  { key: 'green', label: '绿色' },
  { key: 'yellow', label: '黄色' },
];

function getKlineIntervalMeta(bar) {
  return KLINE_INTERVALS.find((item) => item.key === bar) || KLINE_INTERVALS[0];
}

function normalizeKlineRange(range) {
  const from = Number(range?.from || 0);
  const to = Number(range?.to || 0);
  if (!from || !to) return null;
  return from <= to ? { from, to } : { from: to, to: from };
}

function klineRangesEqual(a, b) {
  return Number(a?.from || 0) === Number(b?.from || 0) && Number(a?.to || 0) === Number(b?.to || 0);
}

function findNearestCandleClose(rows, targetTs) {
  const target = Number(targetTs || 0);
  if (!target || !Array.isArray(rows) || !rows.length) return 0;

  let low = 0;
  let high = rows.length - 1;
  while (low <= high) {
    const mid = Math.floor((low + high) / 2);
    const midTime = Number(rows[mid]?.t || 0);
    if (midTime === target) return Number(rows[mid]?.c || 0);
    if (midTime < target) low = mid + 1;
    else high = mid - 1;
  }

  const prev = high >= 0 ? rows[high] : null;
  const next = low < rows.length ? rows[low] : null;
  const prevTime = Number(prev?.t || 0);
  const nextTime = Number(next?.t || 0);

  if (!prev && !next) return 0;
  if (!prev) return Number(next?.c || 0);
  if (!next) return Number(prev?.c || 0);
  return Math.abs(target - prevTime) <= Math.abs(nextTime - target)
    ? Number(prev?.c || 0)
    : Number(next?.c || 0);
}

const STORAGE = {
  initData: 'tglp_web_init_data',
  loginUser: 'tglp_web_login_user',
  chain: 'tglp_web_chain',
  widgets: 'tglp_web_widgets',
  sort: 'tglp_web_hot_pools_sort',
  refreshInterval: 'tglp_web_refresh_interval',
  accentTheme: 'tglp_web_accent_theme',
  walletId: 'tglp_web_wallet_id',
};

function storageGet(key) {
  try {
    return window.localStorage.getItem(key);
  } catch {
    return null;
  }
}

function storageSet(key, value) {
  try {
    window.localStorage.setItem(key, value);
  } catch {
    // ignore
  }
}

function storageRemove(key) {
  try {
    window.localStorage.removeItem(key);
  } catch {
    // ignore
  }
}

function normalizeChain(value) {
  const chain = String(value || '').trim().toLowerCase();
  return chain === 'base' ? 'base' : 'bsc';
}

function normalizeAccentTheme(value) {
  return String(value || '').trim().toLowerCase() === 'yellow' ? 'yellow' : 'green';
}

function moduleLayoutClass(count) {
  if (count <= 1) return 'layout-1';
  if (count === 2) return 'layout-2';
  if (count === 3) return 'layout-3';
  return 'layout-4';
}

function reorderList(list, fromKey, toKey) {
  const rows = [...list];
  const from = rows.indexOf(fromKey);
  const to = rows.indexOf(toKey);
  if (from < 0 || to < 0 || from === to) return rows;
  const [item] = rows.splice(from, 1);
  rows.splice(to, 0, item);
  return rows;
}

function aggregatePoolAddWallets(rows) {
  const map = new Map();

  (Array.isArray(rows) ? rows : []).forEach((row, index) => {
    const rawAddr = String(row?.wallet_address || '').trim();
    const addr = rawAddr.toLowerCase();
    const key = addr || `__unknown_${index}`;
    const totalUsd = Number(row?.total_usd ?? 0);
    const eventCount = Number(row?.event_count ?? 0);
    const priceLower = Number(row?.price_lower ?? 0);
    const priceUpper = Number(row?.price_upper ?? 0);
    const hasRange =
      Number.isFinite(priceLower) &&
      priceLower > 0 &&
      Number.isFinite(priceUpper) &&
      priceUpper > 0;
    const rangeScore = hasRange ? totalUsd : -1;

    if (!map.has(key)) {
      map.set(key, {
        ...row,
        wallet_address: rawAddr,
        total_usd: totalUsd,
        event_count: eventCount,
        _rangeScore: rangeScore,
      });
      return;
    }

    const prev = map.get(key);
    prev.total_usd = Number(prev.total_usd ?? 0) + totalUsd;
    prev.event_count = Number(prev.event_count ?? 0) + eventCount;

    if (rangeScore > Number(prev._rangeScore ?? -1)) {
      prev.price_lower = row?.price_lower;
      prev.price_upper = row?.price_upper;
      prev.price_base = row?.price_base;
      prev.price_quote = row?.price_quote;
      prev._rangeScore = rangeScore;
    }
  });

  return Array.from(map.values())
    .map(({ _rangeScore, ...row }) => row)
    .sort((a, b) => (
      Number(b?.total_usd ?? 0) - Number(a?.total_usd ?? 0) ||
      Number(b?.event_count ?? 0) - Number(a?.event_count ?? 0) ||
      String(a?.wallet_address || '').localeCompare(String(b?.wallet_address || ''))
    ));
}

function parseLoginUser(raw) {
  if (!raw) return null;
  try {
    const user = JSON.parse(raw);
    return user && typeof user === 'object' ? user : null;
  } catch {
    return null;
  }
}

function openExternal(url) {
  if (!url) return;
  window.open(url, '_blank', 'noopener,noreferrer');
}

function buildDexScreenerEmbedUrl(pool, chainName) {
  if (!pool) return '';
  const c = String(pool?.chain || chainName || 'bsc').toLowerCase() === 'base' ? 'base' : 'bsc';
  const factory = String(pool?.factory_name || '').toLowerCase();
  const version = String(pool?.pool_version || pool?.protocol_version || '').toLowerCase();
  const isV4 = factory.includes('v4') || version.includes('v4');
  // V4 pools: DEXScreener doesn't recognise pool ID, use non-stable token address instead
  const addr = isV4
    ? pickNonStableTokenAddress(pool)
    : normalizePoolAddress(pool?.pool_address || pool?.pool_id);
  if (!addr) return '';
  return `https://dexscreener.com/${c}/${addr}?embed=1&theme=dark&trades=1&info=0&interval=1&chartType=price`;
}

function getDexIcon(factoryName) {
  const name = String(factoryName || '').toLowerCase();
  if (name.includes('uniswap')) {
    const m = name.match(/v(\d+)/i);
    return { src: uniswapLogo, label: m ? `V${m[1]}` : '', color: '#ff007a' };
  }
  if (name.includes('pancake') || name.includes('pcs')) {
    const m = name.match(/v(\d+)/i);
    return { src: pancakeLogo, label: m ? `V${m[1]}` : '', color: '#d1884f' };
  }
  return null;
}

export default function App() {
  const apiBaseUrl = WEBAPP_CONFIG.apiBaseUrl;

  const [initData, setInitData] = useState(() => String(storageGet(STORAGE.initData) || '').trim());
  const [loginUser, setLoginUser] = useState(() => parseLoginUser(storageGet(STORAGE.loginUser)));

  const [chain, setChain] = useState(() =>
    normalizeChain(storageGet(STORAGE.chain) || WEBAPP_CONFIG.defaultChain)
  );
  const [widgets, setWidgets] = useState(() => {
    const raw = storageGet(STORAGE.widgets);
    if (!raw) return [...DEFAULT_WIDGETS];
    try {
      return normalizeWidgetSelection(JSON.parse(raw));
    } catch {
      return [...DEFAULT_WIDGETS];
    }
  });
  const [hotSort, setHotSort] = useState(() => {
    const raw = String(storageGet(STORAGE.sort) || '').toLowerCase();
    return raw === 'fee_rate' || raw === 'volume' || raw === 'fees' ? raw : 'fees';
  });

  const [keyword, setKeyword] = useState('');
  const [searchOpen, setSearchOpen] = useState(false);
  const [hotPools, setHotPools] = useState([]);
  const [hotPoolsLoading, setHotPoolsLoading] = useState(false);
  const [hotPoolsError, setHotPoolsError] = useState('');
  const [hotPoolsUpdatedAt, setHotPoolsUpdatedAt] = useState('');
  const [hotTokenFilter, setHotTokenFilter] = useState(null);

  const [positions, setPositions] = useState(null);
  const [positionsLoading, setPositionsLoading] = useState(false);
  const [positionsError, setPositionsError] = useState('');

  const [walletBalances, setWalletBalances] = useState(null);
  const [walletBalancesChain, setWalletBalancesChain] = useState('');
  const [smart, setSmart] = useState(null);
  const [smartLoading, setSmartLoading] = useState(false);
  const [smartError, setSmartError] = useState('');

  const [selectedPool, setSelectedPool] = useState(null);
  const [klineInterval, setKlineInterval] = useState('5m');
  const [klineTokenSide, setKlineTokenSide] = useState('auto');
  const [klineCandles, setKlineCandles] = useState([]);
  const [klineLoading, setKlineLoading] = useState(false);
  const [klineError, setKlineError] = useState('');
  const [klineSource, setKlineSource] = useState('');
  const [klineMarkers, setKlineMarkers] = useState([]);
  const [klineMarkerStats, setKlineMarkerStats] = useState({
    totalEvents: 0,
    addCount: 0,
    removeCount: 0,
    walletCount: 0,
    truncated: false,
    loadedEvents: 0,
  });
  const [klineMarkersLoading, setKlineMarkersLoading] = useState(false);
  const [klineMarkersError, setKlineMarkersError] = useState('');
  const [klineOverlayEnabled, setKlineOverlayEnabled] = useState(true);
  const [klineOverlayAvailable, setKlineOverlayAvailable] = useState(true);
  const [klineRefreshNonce, setKlineRefreshNonce] = useState(0);
  const [klineMarkerRefreshNonce, setKlineMarkerRefreshNonce] = useState(0);
  const [selectedMarkerCluster, setSelectedMarkerCluster] = useState(null);
  const [klineVisibleRange, setKlineVisibleRange] = useState(null);

  const [refreshing, setRefreshing] = useState(false);
  const [loginBusy, setLoginBusy] = useState(false);
  const [loginError, setLoginError] = useState('');
  const [workMode, setWorkMode] = useState(false);
  const [showSettings, setShowSettings] = useState(false);
  const [refreshInterval, setRefreshInterval] = useState(() => {
    const saved = Number(storageGet(STORAGE.refreshInterval));
    return saved >= 10 ? saved : 10;
  });
  const [accentTheme, setAccentTheme] = useState(() =>
    normalizeAccentTheme(storageGet(STORAGE.accentTheme) || 'green')
  );
  const [draggingKey, setDraggingKey] = useState('');
  const [dragOverKey, setDragOverKey] = useState('');
  const klineVisibleRangeTimerRef = useRef(null);

  const hasInitData = Boolean(initData);
  const activeWidgets = useMemo(() => {
    const map = Object.fromEntries(WIDGETS.map((w) => [w.key, w]));
    return widgets.map((k) => map[k]).filter(Boolean);
  }, [widgets]);
  const layoutClass = moduleLayoutClass(activeWidgets.length);
  const workLayoutClass = workMode ? `work-mode layout-work-${Math.min(activeWidgets.length, 4)}` : layoutClass;

  const selectedPoolAddress = useMemo(
    () => normalizePoolAddress(selectedPool?.pool_address || selectedPool?.pool_id),
    [selectedPool]
  );
  const selectedPoolVersion = useMemo(() => inferPoolVersion(selectedPool), [selectedPool]);
  const klineTokenMeta = useMemo(() => resolveKlineTokenOptions(selectedPool), [selectedPool]);
  const klineTokenOptions = klineTokenMeta.options || [];
  const klineDefaultTokenSide = klineTokenMeta.defaultKey || '';
  const klineActiveTokenSide = useMemo(() => {
    if (!klineTokenOptions.length) return '';
    if (klineTokenSide && klineTokenSide !== 'auto') {
      return klineTokenOptions.some((item) => item.key === klineTokenSide)
        ? klineTokenSide
        : klineDefaultTokenSide;
    }
    return klineDefaultTokenSide;
  }, [klineDefaultTokenSide, klineTokenOptions, klineTokenSide]);
  const klineActiveToken = useMemo(
    () => klineTokenOptions.find((item) => item.key === klineActiveTokenSide) || null,
    [klineActiveTokenSide, klineTokenOptions]
  );
  const klineTokenAddress = useMemo(
    () => normalizeHexAddress(klineActiveToken?.address),
    [klineActiveToken]
  );
  const klineIntervalMeta = useMemo(() => getKlineIntervalMeta(klineInterval), [klineInterval]);
  const selectedPoolGmgnUrl = useMemo(() => buildGmgnUrl(selectedPool, chain), [selectedPool, chain]);
  const selectedPoolEmbedUrl = useMemo(
    () => buildDexScreenerEmbedUrl(selectedPool, chain),
    [selectedPool, chain]
  );

  const filteredHotPools = useMemo(() => {
    const q = String(keyword || '').trim().toLowerCase();
    const positionPoolMap = new Map();
    const positionRows = Array.isArray(positions?.positions) ? positions.positions : [];
    positionRows.forEach((row) => {
      const poolId = normalizePoolAddress(row?.pool_id);
      if (!poolId) return;
      const totalUsd = Number(row?.totals?.position_usd || 0) + Number(row?.totals?.fee_usd || 0);
      if (!Number.isFinite(totalUsd) || totalUsd <= 0) return;
      positionPoolMap.set(poolId, (positionPoolMap.get(poolId) || 0) + totalUsd);
    });

    const enriched = hotPools
      .filter((row) => {
        const addr = normalizePoolAddress(row?.pool_address || row?.pool_id);
        if (addr && positionPoolMap.has(addr)) return true;
        if (!q) return true;
        const pair = String(row?.trading_pair || '').toLowerCase();
        return pair.includes(q) || String(addr || '').toLowerCase().includes(q);
      })
      .map((row, index) => {
        const addr = normalizePoolAddress(row?.pool_address || row?.pool_id);
        return {
          ...row,
          userPositionUsd: addr ? Number(positionPoolMap.get(addr) || 0) : 0,
          _listIndex: index,
        };
      });

    return enriched
      .sort((a, b) => {
        const aPos = Number(a?.userPositionUsd || 0);
        const bPos = Number(b?.userPositionUsd || 0);
        if (aPos > 0 && bPos <= 0) return -1;
        if (bPos > 0 && aPos <= 0) return 1;
        if (aPos > 0 && bPos > 0 && aPos !== bPos) return bPos - aPos;
        return Number(a?._listIndex || 0) - Number(b?._listIndex || 0);
      })
      .map(({ _listIndex, ...row }) => row);
  }, [hotPools, keyword, positions]);
  const hotPoolIncludeAddresses = useMemo(() => {
    const rows = Array.isArray(positions?.positions) ? positions.positions : [];
    const seen = new Set();
    rows.forEach((row) => {
      const poolId = normalizePoolAddress(row?.pool_id);
      if (poolId) seen.add(poolId);
    });
    return Array.from(seen).sort();
  }, [positions]);
  const hotPoolIncludeKey = useMemo(
    () => hotPoolIncludeAddresses.join(','),
    [hotPoolIncludeAddresses]
  );
  const hotPoolsLimit = hotTokenFilter?.address ? 200 : 60;

  const sortedPositions = useMemo(() => {
    const rows = Array.isArray(positions?.positions) ? positions.positions : [];
    return [...rows].sort(
      (a, b) => Number(b?.totals?.total_usd || 0) - Number(a?.totals?.total_usd || 0)
    );
  }, [positions]);

  const smartPools = useMemo(() => {
    const rows = Array.isArray(smart?.pools) ? smart.pools : [];
    return [...rows].sort((a, b) => (
      Number(b?.wallet_count || 0) - Number(a?.wallet_count || 0) ||
      Number(b?.added_liquidity || 0) - Number(a?.added_liquidity || 0) ||
      String(a?.pool_id || '').localeCompare(String(b?.pool_id || ''))
    ));
  }, [smart]);
  const smartDisplayPools = useMemo(() => smartPools.slice(0, 10), [smartPools]);

  const smartWallets = useMemo(() => {
    const rows = Array.isArray(smart?.wallets_24h) ? smart.wallets_24h : [];
    return [...rows].sort((a, b) => Number(b?.pnl_usdt_24h || 0) - Number(a?.pnl_usdt_24h || 0));
  }, [smart]);
  const klineMarkerWalletCount = useMemo(() => {
    const value = Number(klineMarkerStats?.walletCount || 0);
    if (value > 0) return value;
    const seen = new Set();
    (Array.isArray(klineMarkers) ? klineMarkers : []).forEach((row) => {
      const addr = String(row?.wallet_address || '').trim().toLowerCase();
      if (addr) seen.add(addr);
    });
    return seen.size;
  }, [klineMarkerStats, klineMarkers]);
  const klineMarkerAddCount = useMemo(() => {
    const value = Number(klineMarkerStats?.addCount || 0);
    if (value > 0) return value;
    return (Array.isArray(klineMarkers) ? klineMarkers : []).filter((row) => String(row?.action || '').toLowerCase() !== 'remove').length;
  }, [klineMarkerStats, klineMarkers]);
  const klineMarkerRemoveCount = useMemo(() => {
    const value = Number(klineMarkerStats?.removeCount || 0);
    if (value > 0) return value;
    return (Array.isArray(klineMarkers) ? klineMarkers : []).filter((row) => String(row?.action || '').toLowerCase() === 'remove').length;
  }, [klineMarkerStats, klineMarkers]);
  const klineMarkerEventCount = useMemo(() => {
    const value = Number(klineMarkerStats?.totalEvents || 0);
    if (value > 0) return value;
    return klineMarkers.length;
  }, [klineMarkerStats, klineMarkers.length]);
  const klineViewportKey = useMemo(
    () => `${selectedPoolAddress || 'pool'}:${klineTokenAddress || 'token'}:${klineInterval}`,
    [klineInterval, klineTokenAddress, selectedPoolAddress]
  );
  const klineCandleRange = useMemo(() => {
    const rows = Array.isArray(klineCandles) ? klineCandles : [];
    if (!rows.length) return null;
    const first = toUnixSeconds(rows[0]?.t);
    const last = toUnixSeconds(rows[rows.length - 1]?.t);
    if (!first || !last) return null;
    return first <= last ? { from: first, to: last } : { from: last, to: first };
  }, [klineCandles]);
  const klineMarkerQueryRange = useMemo(
    () => normalizeKlineRange(klineVisibleRange) || normalizeKlineRange(klineCandleRange),
    [klineCandleRange, klineVisibleRange]
  );
  const klineCandlePriceRows = useMemo(() => (
    (Array.isArray(klineCandles) ? klineCandles : [])
      .map((row) => ({ t: toUnixSeconds(row?.t), c: Number(row?.c || 0) }))
      .filter((row) => row.t > 0 && Number.isFinite(row.c))
      .sort((a, b) => a.t - b.t)
  ), [klineCandles]);
  const resolveMarkerCandleClose = useCallback(
    (ts) => findNearestCandleClose(klineCandlePriceRows, ts),
    [klineCandlePriceRows]
  );
  const klineMarkerRangeFrom = Number(klineMarkerQueryRange?.from || 0);
  const klineMarkerRangeTo = Number(klineMarkerQueryRange?.to || 0);
  const klineMarkersWatchingLatest = useMemo(() => {
    if (!klineMarkerRangeTo) return true;
    const now = Math.floor(Date.now() / 1000);
    return klineMarkerRangeTo >= now - Math.max(klineIntervalMeta.bucketSec * 2, 120);
  }, [klineIntervalMeta.bucketSec, klineMarkerRangeTo]);
  const klineMarkersRefreshMs = useMemo(
    () => (klineMarkersWatchingLatest ? KLINE_MARKER_LIVE_REFRESH_MS : KLINE_MARKER_IDLE_REFRESH_MS),
    [klineMarkersWatchingLatest]
  );
  const handleKlineVisibleRangeChange = useCallback((nextRange) => {
    const normalized = normalizeKlineRange(nextRange);
    if (!normalized) return;
    if (klineVisibleRangeTimerRef.current) {
      window.clearTimeout(klineVisibleRangeTimerRef.current);
    }
    klineVisibleRangeTimerRef.current = window.setTimeout(() => {
      setKlineVisibleRange((prev) => (klineRangesEqual(prev, normalized) ? prev : normalized));
      klineVisibleRangeTimerRef.current = null;
    }, KLINE_MARKER_RANGE_DEBOUNCE_MS);
  }, []);

  useEffect(() => {
    storageSet(STORAGE.chain, chain);
    storageSet(STORAGE.widgets, JSON.stringify(widgets));
    storageSet(STORAGE.sort, hotSort);
    storageSet(STORAGE.refreshInterval, String(refreshInterval));
    storageSet(STORAGE.accentTheme, accentTheme);

    if (initData) {
      storageSet(STORAGE.initData, initData);
    } else {
      storageRemove(STORAGE.initData);
    }
    if (loginUser) {
      storageSet(STORAGE.loginUser, JSON.stringify(loginUser));
    } else {
      storageRemove(STORAGE.loginUser);
    }
  }, [accentTheme, chain, hotSort, initData, loginUser, refreshInterval, widgets]);

  useEffect(() => {
    if (!workMode) return;
    const handler = (e) => { if (e.key === 'Escape') setWorkMode(false); };
    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  }, [workMode]);

  useEffect(() => {
    if (!showSettings) return;
    const handler = (e) => {
      if (!e.target.closest('.settings-wrap')) setShowSettings(false);
    };
    document.addEventListener('mousedown', handler);
    return () => document.removeEventListener('mousedown', handler);
  }, [showSettings]);

  const selectPool = useCallback(
    (pool, fallbackChain) => {
      const normalized = normalizePoolAddress(pool?.pool_address || pool?.pool_id);
      const next = {
        ...pool,
        chain: String(pool?.chain || fallbackChain || chain || 'bsc').toLowerCase(),
        pool_address: normalized || pool?.pool_address || pool?.pool_id,
      };
      setSelectedPool(next);
    },
    [chain]
  );

  const loadHotPools = useCallback(
    async (signal) => {
      if (!hasInitData) {
        setHotPools([]);
        setHotPoolsError('请先点击右上角 Telegram 图标扫码登录。');
        return;
      }
      setHotPoolsLoading(true);
      setHotPoolsError('');
      try {
        const resp = await fetchHotPools({
          apiBaseUrl,
          initData,
          chain,
          sort: hotSort,
          timeframeMinutes: 5,
          limit: hotPoolsLimit,
          tokenAddress: hotTokenFilter?.address || '',
          includePools: hotTokenFilter?.address ? undefined : (hotPoolIncludeKey ? hotPoolIncludeKey.split(',') : undefined),
          signal,
        });
        setHotPools(Array.isArray(resp?.data) ? resp.data : []);
        setHotPoolsUpdatedAt(resp?.updated_at || new Date().toISOString());
      } catch (e) {
        if (e?.name !== 'AbortError') setHotPoolsError(String(e?.message || e));
      } finally {
        setHotPoolsLoading(false);
      }
    },
    [apiBaseUrl, chain, hasInitData, hotPoolIncludeKey, hotPoolsLimit, hotSort, hotTokenFilter?.address, initData]
  );

  const loadPositions = useCallback(
    async (signal) => {
      if (!hasInitData) {
        setPositions(null);
        setPositionsError('请先点击右上角 Telegram 图标扫码登录。');
        return;
      }
      setPositionsLoading(true);
      setPositionsError('');
      try {
        setPositions(await fetchRealtimePositions({ apiBaseUrl, initData, signal }));
      } catch (e) {
        if (e?.name !== 'AbortError') setPositionsError(String(e?.message || e));
      } finally {
        setPositionsLoading(false);
      }
    },
    [apiBaseUrl, hasInitData, initData]
  );

  const loadWalletBalances = useCallback(
    async (signal) => {
      if (!hasInitData) return;
      try {
        const resp = await fetchWallets({ apiBaseUrl, initData, chain, signal });
        setWalletBalances(resp?.wallets || []);
        setWalletBalancesChain(resp?.chain || chain);
      } catch (e) {
        if (e?.name !== 'AbortError') setWalletBalances(null);
      }
    },
    [apiBaseUrl, chain, hasInitData, initData]
  );

  const loadSmart = useCallback(
    async (signal) => {
      if (!hasInitData) {
        setSmart(null);
        setSmartError('请先点击右上角 Telegram 图标扫码登录。');
        return;
      }
      setSmartLoading(true);
      setSmartError('');
      try {
        setSmart(
          await fetchSmartMoneyOverview({
            apiBaseUrl,
            initData,
            chain,
            poolLimit: 24,
            walletLimit: 20,
            poolsWindowHours: SMART_POOL_WINDOW_HOURS,
            pnlWindowHours: SMART_PNL_WINDOW_HOURS,
            signal,
          })
        );
      } catch (e) {
        if (e?.name !== 'AbortError') setSmartError(String(e?.message || e));
      } finally {
        setSmartLoading(false);
      }
    },
    [apiBaseUrl, chain, hasInitData, initData]
  );

  const loadKline = useCallback(
    async (signal) => {
      if (!hasInitData) {
        setKlineCandles([]);
        setKlineSource('');
        setKlineError('请先点击右上角 Telegram 图标扫码登录。');
        return;
      }
      if (!klineTokenAddress) {
        setKlineCandles([]);
        setKlineSource('');
        setKlineError('');
        return;
      }

      setKlineLoading(true);
      setKlineError('');
      try {
        const resp = await fetchTokenCandles({
          apiBaseUrl,
          initData,
          chain: selectedPool?.chain || chain,
          tokenAddress: klineTokenAddress,
          bar: klineIntervalMeta.key,
          limit: klineIntervalMeta.limit,
          signal,
        });
        setKlineSource('token-usd');
        setKlineCandles(Array.isArray(resp?.candles) ? resp.candles : []);
      } catch (e) {
        if (e?.name !== 'AbortError') {
          setKlineSource('');
          setKlineError(String(e?.message || e));
        }
      } finally {
        setKlineLoading(false);
      }
    },
    [
      apiBaseUrl,
      chain,
      hasInitData,
      initData,
      klineIntervalMeta.key,
      klineIntervalMeta.limit,
      klineTokenAddress,
      selectedPool?.chain,
    ]
  );

  const loadKlineMarkers = useCallback(
    async (signal) => {
      if (!hasInitData || !selectedPoolAddress || !selectedPoolVersion) {
        setKlineMarkers([]);
        setKlineMarkerStats({ totalEvents: 0, addCount: 0, removeCount: 0, walletCount: 0, truncated: false, loadedEvents: 0 });
        setKlineMarkersError('');
        return;
      }
      if (!klineOverlayEnabled || !klineOverlayAvailable) {
        setKlineMarkers([]);
        setKlineMarkerStats({ totalEvents: 0, addCount: 0, removeCount: 0, walletCount: 0, truncated: false, loadedEvents: 0 });
        setKlineMarkersError('');
        return;
      }

      setKlineMarkersLoading(true);
      setKlineMarkersError('');
      try {
        const startTs = klineMarkerRangeFrom;
        const endTs = klineMarkerRangeTo;
        const fallbackWindowSec = KLINE_MARKER_WINDOW_HOURS * 3600;
        const rangeWindowSec = startTs > 0 && endTs >= startTs
          ? Math.max(klineIntervalMeta.bucketSec, endTs - startTs)
          : fallbackWindowSec;
        const [smartResp, myResp] = await Promise.allSettled([
          fetchSmartMoneyPoolMarkers({
            apiBaseUrl,
            initData,
            chain: selectedPool?.chain || chain,
            poolVersion: selectedPoolVersion,
            poolId: selectedPoolAddress,
            bucketSec: klineIntervalMeta.bucketSec,
            windowHours: KLINE_MARKER_WINDOW_HOURS,
            startTs,
            endTs,
            limit: KLINE_MARKER_FETCH_LIMIT,
            signal,
          }),
          fetchMyTradeMarkers({
            apiBaseUrl,
            initData,
            chain: selectedPool?.chain || chain,
            poolId: selectedPoolAddress,
            bucketSec: klineIntervalMeta.bucketSec,
            startTs,
            endTs,
            windowSec: rangeWindowSec,
            signal,
          }),
        ]);

        if (smartResp.status === 'rejected') {
          const e = smartResp.reason;
          if (e?.name === 'AbortError') return;
          if (e?.status === 403) {
            setKlineOverlayAvailable(false);
            setKlineOverlayEnabled(false);
            setKlineMarkers([]);
            setKlineMarkerStats({ totalEvents: 0, addCount: 0, removeCount: 0, walletCount: 0, truncated: false, loadedEvents: 0 });
            setKlineMarkersError('当前账号没有聪明钱权限，已切换为纯 K 线模式。');
            return;
          }
          setKlineMarkersError(String(e?.message || e));
        }

        setKlineOverlayAvailable(true);
        const smartEvents = smartResp.status === 'fulfilled' && Array.isArray(smartResp.value?.events) ? smartResp.value.events : [];
        const myEvents = myResp.status === 'fulfilled' && Array.isArray(myResp.value?.events) ? myResp.value.events : [];
        const smartValue = smartResp.status === 'fulfilled' ? smartResp.value : null;
        setKlineMarkerStats({
          totalEvents: Number(smartValue?.total_events || smartEvents.length || 0),
          addCount: Number(smartValue?.add_count || 0),
          removeCount: Number(smartValue?.remove_count || 0),
          walletCount: Number(smartValue?.wallet_count || 0),
          truncated: Boolean(smartValue?.truncated),
          loadedEvents: smartEvents.length,
        });
        setKlineMarkers([...smartEvents, ...myEvents]);
      } catch (e) {
        if (e?.name === 'AbortError') return;
        setKlineMarkerStats({ totalEvents: 0, addCount: 0, removeCount: 0, walletCount: 0, truncated: false, loadedEvents: 0 });
        setKlineMarkersError(String(e?.message || e));
      } finally {
        setKlineMarkersLoading(false);
      }
    },
    [
      apiBaseUrl,
      chain,
      hasInitData,
      initData,
      klineIntervalMeta.bucketSec,
      klineMarkerRangeFrom,
      klineMarkerRangeTo,
      klineOverlayAvailable,
      klineOverlayEnabled,
      selectedPool?.chain,
      selectedPoolAddress,
      selectedPoolVersion,
    ]
  );

  useEffect(() => {
    const ctrl = new AbortController();
    loadHotPools(ctrl.signal);
    return () => ctrl.abort();
  }, [loadHotPools]);

  useEffect(() => {
    const ctrl = new AbortController();
    loadPositions(ctrl.signal);
    return () => ctrl.abort();
  }, [loadPositions]);

  useEffect(() => {
    const ctrl = new AbortController();
    loadSmart(ctrl.signal);
    return () => ctrl.abort();
  }, [loadSmart]);

  useEffect(() => {
    const ctrl = new AbortController();
    loadWalletBalances(ctrl.signal);
    return () => ctrl.abort();
  }, [loadWalletBalances]);

  useEffect(() => {
    const ctrl = new AbortController();
    loadKline(ctrl.signal);
    return () => ctrl.abort();
  }, [loadKline, klineRefreshNonce]);

  useEffect(() => {
    const ctrl = new AbortController();
    loadKlineMarkers(ctrl.signal);
    return () => ctrl.abort();
  }, [loadKlineMarkers, klineMarkerRefreshNonce, klineRefreshNonce]);

  useEffect(() => {
    if (!hasInitData) return undefined;
    const timer = window.setInterval(() => loadHotPools(), refreshInterval * 1000);
    return () => window.clearInterval(timer);
  }, [hasInitData, loadHotPools, refreshInterval]);

  useEffect(() => {
    if (!hasInitData) return undefined;
    const timer = window.setInterval(() => loadPositions(), refreshInterval * 1000);
    return () => window.clearInterval(timer);
  }, [hasInitData, loadPositions, refreshInterval]);

  useEffect(() => {
    if (!hasInitData) return undefined;
    const timer = window.setInterval(() => loadWalletBalances(), Math.max(refreshInterval * 1000, 30_000));
    return () => window.clearInterval(timer);
  }, [hasInitData, loadWalletBalances, refreshInterval]);

  useEffect(() => {
    if (!hasInitData || !klineTokenAddress) return undefined;
    const interval = 20_000;
    const timer = window.setInterval(() => setKlineRefreshNonce((n) => n + 1), interval);
    return () => window.clearInterval(timer);
  }, [hasInitData, klineTokenAddress]);

  useEffect(() => {
    if (!hasInitData) return undefined;
    const timer = window.setInterval(() => loadSmart(), Math.max(refreshInterval * 1000, 30_000));
    return () => window.clearInterval(timer);
  }, [hasInitData, loadSmart, refreshInterval]);

  useEffect(() => {
    if (!hasInitData || !selectedPoolAddress || !klineOverlayEnabled || !klineOverlayAvailable) return undefined;
    const timer = window.setInterval(() => setKlineMarkerRefreshNonce((n) => n + 1), klineMarkersRefreshMs);
    return () => window.clearInterval(timer);
  }, [hasInitData, klineMarkersRefreshMs, klineOverlayAvailable, klineOverlayEnabled, selectedPoolAddress]);

  useEffect(() => () => {
    if (klineVisibleRangeTimerRef.current) {
      window.clearTimeout(klineVisibleRangeTimerRef.current);
      klineVisibleRangeTimerRef.current = null;
    }
  }, []);

  useEffect(() => {
    if (!hotPools.length) return;
    setSelectedPool((prev) => {
      const prevAddr = normalizePoolAddress(prev?.pool_address || prev?.pool_id);
      if (!prevAddr) return { ...hotPools[0], chain };
      const matched = hotPools.find(
        (row) => normalizePoolAddress(row?.pool_address || row?.pool_id) === prevAddr
      );
      return matched ? { ...matched, chain } : prev;
    });
  }, [chain, hotPools]);

  useEffect(() => {
    setHotTokenFilter(null);
  }, [chain]);

  useEffect(() => {
    if (klineVisibleRangeTimerRef.current) {
      window.clearTimeout(klineVisibleRangeTimerRef.current);
      klineVisibleRangeTimerRef.current = null;
    }
    setKlineVisibleRange(null);
  }, [klineViewportKey]);

  useEffect(() => {
    setKlineTokenSide('auto');
    setSelectedMarkerCluster(null);
    setKlineMarkers([]);
    setKlineMarkerStats({ totalEvents: 0, addCount: 0, removeCount: 0, walletCount: 0, truncated: false, loadedEvents: 0 });
    setKlineMarkersError('');
    setKlineSource('');
    setKlineMarkerRefreshNonce(0);
  }, [selectedPoolAddress]);

  useEffect(() => {
    if (!klineOverlayEnabled) {
      setSelectedMarkerCluster(null);
    }
  }, [klineOverlayEnabled]);

  useEffect(() => {
    setKlineOverlayAvailable(true);
  }, [initData]);

  const [loginCode, setLoginCode] = useState('');
  const [loginCodeExpiry, setLoginCodeExpiry] = useState(0);
  const pollRef = useRef(null);

  const handleLoginFromResp = useCallback((resp) => {
    const nextInitData = String(resp?.initData || '').trim();
    if (!nextInitData) throw new Error('后端未返回 initData');
    setInitData(nextInitData);
    setLoginUser(resp?.user || null);
    setLoginCode('');
  }, []);

  const startCodeLogin = useCallback(async () => {
    setLoginBusy(true);
    setLoginError('');
    setLoginCode('');
    try {
      const resp = await generateLoginCode({ apiBaseUrl });
      if (!resp?.code) throw new Error('生成验证码失败');
      setLoginCode(resp.code);
      setLoginCodeExpiry(Date.now() + (resp.expires_in || 300) * 1000);
    } catch (e) {
      setLoginError(String(e?.message || e));
    } finally {
      setLoginBusy(false);
    }
  }, [apiBaseUrl]);

  // Poll for code confirmation
  useEffect(() => {
    if (!loginCode || hasInitData) {
      if (pollRef.current) clearInterval(pollRef.current);
      return;
    }

    const poll = async () => {
      if (Date.now() > loginCodeExpiry) {
        setLoginCode('');
        setLoginError('验证码已过期，请重新获取');
        if (pollRef.current) clearInterval(pollRef.current);
        return;
      }
      try {
        const resp = await checkLoginCode({ apiBaseUrl, code: loginCode });
        if (resp?.ok && resp?.initData) {
          handleLoginFromResp(resp);
          if (pollRef.current) clearInterval(pollRef.current);
        } else if (resp?.status === 'expired') {
          setLoginCode('');
          setLoginError('验证码已过期，请重新获取');
          if (pollRef.current) clearInterval(pollRef.current);
        }
      } catch {
        // ignore poll errors
      }
    };

    pollRef.current = setInterval(poll, 2000);
    return () => {
      if (pollRef.current) clearInterval(pollRef.current);
    };
  }, [apiBaseUrl, handleLoginFromResp, hasInitData, loginCode, loginCodeExpiry]);

  const logout = useCallback(() => {
    setInitData('');
    setLoginUser(null);
    storageRemove(STORAGE.initData);
    storageRemove(STORAGE.loginUser);
    setHotPools([]);
    setPositions(null);
    setSmart(null);
  }, []);

  const refreshAll = useCallback(async () => {
    if (!hasInitData) return;
    setRefreshing(true);
    await Promise.allSettled([loadHotPools(), loadPositions(), loadSmart()]);
    setKlineRefreshNonce((v) => v + 1);
    setRefreshing(false);
  }, [hasInitData, loadHotPools, loadPositions, loadSmart]);

  const refreshKline = useCallback(() => {
    setKlineRefreshNonce((v) => v + 1);
  }, []);

  const toggleWidget = useCallback((key) => {
    setWidgets((prev) => {
      const exists = prev.includes(key);
      if (exists && prev.length === 1) return prev;
      if (exists) return prev.filter((x) => x !== key);
      return [...prev, key];
    });
  }, []);

  const [openPosPool, setOpenPosPool] = useState(null);
  const [openPosBusy, setOpenPosBusy] = useState(false);
  const [openPosWallets, setOpenPosWallets] = useState(null);
  const [openPosWalletsLoading, setOpenPosWalletsLoading] = useState(false);
  const [openPosWalletId, setOpenPosWalletId] = useState(() => {
    const saved = Number(storageGet(STORAGE.walletId));
    return Number.isFinite(saved) && saved > 0 ? saved : 0;
  });
  const [taskActionPos, setTaskActionPos] = useState(null);
  const [operationProgress, setOperationProgress] = useState(null);

  const loadWalletsForModal = useCallback(async (posChain) => {
    if (!hasInitData) return;
    setOpenPosWalletsLoading(true);
    try {
      const resp = await fetchWallets({ apiBaseUrl, initData, chain: posChain || chain });
      setOpenPosWallets(resp?.wallets || []);
    } catch {
      setOpenPosWallets(null);
    } finally {
      setOpenPosWalletsLoading(false);
    }
  }, [apiBaseUrl, chain, hasInitData, initData]);

  const openPositionModal = useCallback((pool) => {
    const resolvedChain = normalizeChain(pool?.chain || chain);
    const resolvedVersion = String(
      pool?.protocol_version || pool?.pool_version || inferPoolVersion(pool) || ''
    )
      .trim()
      .toLowerCase();

    setOpenPosPool({
      ...pool,
      chain: resolvedChain,
      ...(resolvedVersion ? { protocol_version: resolvedVersion, pool_version: resolvedVersion } : {}),
    });
    loadWalletsForModal(resolvedChain);
  }, [chain, loadWalletsForModal]);

  useEffect(() => {
    if (!openPosPool || !hasInitData) return undefined;

    const existing = Array.isArray(openPosPool?.smartMoneyWallets)
      ? openPosPool.smartMoneyWallets
      : [];
    if (existing.length > 0) return undefined;

    const poolId = normalizePoolAddress(openPosPool?.pool_address || openPosPool?.pool_id);
    const poolVersion = String(
      openPosPool?.protocol_version || openPosPool?.pool_version || inferPoolVersion(openPosPool) || ''
    )
      .trim()
      .toLowerCase();
    if (!poolId || !poolVersion) return undefined;

    let cancelled = false;
    const ctrl = new AbortController();

    fetchSmartMoneyPoolAdds({
      apiBaseUrl,
      initData,
      chain: openPosPool?.chain || chain,
      poolVersion,
      poolId,
      windowHours: SMART_POOL_WINDOW_HOURS,
      limit: 120,
      signal: ctrl.signal,
    })
      .then((res) => {
        if (cancelled) return;
        const wallets = Array.isArray(res?.wallets) ? res.wallets : [];
        setOpenPosPool((prev) => {
          if (!prev) return prev;
          const prevId = normalizePoolAddress(prev?.pool_address || prev?.pool_id);
          const prevVersion = String(
            prev?.protocol_version || prev?.pool_version || inferPoolVersion(prev) || ''
          )
            .trim()
            .toLowerCase();
          if (prevId !== poolId || prevVersion !== poolVersion) return prev;
          return { ...prev, smartMoneyWallets: wallets };
        });
      })
      .catch((e) => {
        if (cancelled || e?.name === 'AbortError') return;
      });

    return () => {
      cancelled = true;
      ctrl.abort();
    };
  }, [apiBaseUrl, chain, hasInitData, initData, openPosPool]);

  const handleOpenPosition = useCallback(async (params) => {
    const panelKey = openPosPool?.panelKey || 'hot_pools';
    setOpenPosBusy(true);
    setOperationProgress({
      panelKey,
      operation: 'open_position',
      currentStep: 0,
      totalSteps: 5,
      status: 'active',
      error: '',
    });
    setOpenPosPool(null);
    try {
      await apiOpenPosition({ apiBaseUrl, initData, ...params });
      setOperationProgress(prev => prev?.operation === 'open_position'
        ? { ...prev, currentStep: 4, status: 'done' } : prev);
      loadPositions();
    } catch (e) {
      const msg = String(e?.message || e);
      setOperationProgress(prev => prev?.operation === 'open_position'
        ? { ...prev, status: 'error', error: msg } : prev);
    } finally {
      setOpenPosBusy(false);
    }
  }, [apiBaseUrl, initData, loadPositions, openPosPool]);

  const handleTaskPause = useCallback(async (taskId, paused) => {
    await setTaskPaused({ apiBaseUrl, initData, taskId, paused });
    loadPositions();
  }, [apiBaseUrl, initData, loadPositions]);

  const handleTaskStop = useCallback(async (taskId) => {
    setOperationProgress({
      panelKey: 'positions',
      operation: 'close_position',
      taskId,
      currentStep: 0,
      totalSteps: 4,
      status: 'active',
      error: '',
    });
    try {
      const resp = await stopTask({ apiBaseUrl, initData, taskId });
      if (resp?.status === 'stopped' || resp?.pending === false) {
        setOperationProgress(prev => prev?.operation === 'close_position'
          ? { ...prev, currentStep: 3, status: 'done' } : prev);
      } else {
        setOperationProgress(prev => {
          if (!prev || prev.operation !== 'close_position') return prev;
          if (prev.status === 'done' || prev.status === 'error') return prev;
          if (prev.currentStep > 1) return prev;
          return { ...prev, currentStep: 1, status: 'active' };
        });
      }
      loadPositions();
    } catch (e) {
      const msg = String(e?.message || e);
      setOperationProgress(prev => prev?.operation === 'close_position'
        ? { ...prev, status: 'error', error: msg } : prev);
    }
  }, [apiBaseUrl, initData, loadPositions]);

  // Polling fallback: detect close completion from positions data
  useEffect(() => {
    if (!operationProgress) return;
    if (operationProgress.operation !== 'close_position') return;
    if (operationProgress.status === 'done' || operationProgress.status === 'error') return;
    const taskId = operationProgress.taskId;
    if (!taskId) return;
    const rows = Array.isArray(positions?.positions) ? positions.positions : null;
    if (!rows) return; // data not loaded yet
    const found = rows.some(p => Number(p?.task_id) === Number(taskId));
    if (!found) {
      setOperationProgress(prev => {
        if (!prev || prev.operation !== 'close_position') return prev;
        if (prev.status === 'done' || prev.status === 'error') return prev;
        return { ...prev, currentStep: 3, status: 'done' };
      });
    }
  }, [positions, operationProgress]);

  const handleTaskDelete = useCallback(async (taskId) => {
    await deleteTask({ apiBaseUrl, initData, taskId });
    loadPositions();
  }, [apiBaseUrl, initData, loadPositions]);

  const handleTaskEditRange = useCallback(async (taskId, rl, ru, amt) => {
    await updateTaskRange({ apiBaseUrl, initData, taskId, rangeLowerPct: rl, rangeUpperPct: ru, amountUSDT: amt });
    loadPositions();
  }, [apiBaseUrl, initData, loadPositions]);

  const copyAddr = useCallback((addr) => {
    navigator.clipboard?.writeText(addr).catch(() => {});
  }, []);

  const renderOperationProgress = (panelKey) => {
    if (!operationProgress || operationProgress.panelKey !== panelKey) return null;
    return (
      <StepProgressModal
        operation={operationProgress.operation}
        progress={operationProgress}
        onClose={() => {
          const op = operationProgress;
          setOperationProgress(null);
          if (op?.status === 'done' && op?.operation === 'open_position') {
            setOpenPosPool(null);
          }
        }}
      />
    );
  };

  const summary = positions?.summary || {};
  const smartSummary = smart?.summary || {};

  // Pool adds preview data: { "v3:0x1234": { status, wallets, totalUsd, error } }
  const [poolAddsMap, setPoolAddsMap] = useState({});

  // Auto-load pool adds for top pools when smart data changes
  useEffect(() => {
    if (!smartDisplayPools.length || !hasInitData) return;
    const ctrl = new AbortController();
    const toLoad = smartDisplayPools;
    toLoad.forEach((pool) => {
      const key = `${pool?.pool_version || ''}:${pool?.pool_id || ''}`;
      if (poolAddsMap[key]?.status === 'loading') return;
      setPoolAddsMap((prev) => ({
        ...prev,
        [key]: {
          status: 'loading',
          wallets: Array.isArray(prev[key]?.wallets) ? prev[key].wallets : [],
          totalUsd: Number(prev[key]?.totalUsd || 0),
          error: '',
        },
      }));
      fetchSmartMoneyPoolAdds({
        apiBaseUrl,
        initData,
        chain,
        poolVersion: pool?.pool_version,
        poolId: pool?.pool_id,
        windowHours: SMART_POOL_WINDOW_HOURS,
        limit: 20,
        signal: ctrl.signal,
      })
        .then((res) => {
          const wallets = Array.isArray(res?.wallets) ? res.wallets : [];
          const totalUsd = wallets.reduce((s, w) => s + Number(w?.total_usd || 0), 0);
          setPoolAddsMap((prev) => ({
            ...prev,
            [key]: { status: 'success', wallets, totalUsd, error: '' },
          }));
        })
        .catch((e) => {
          if (e?.name === 'AbortError') return;
          setPoolAddsMap((prev) => ({
            ...prev,
            [key]: { status: 'error', wallets: [], totalUsd: 0, error: String(e?.message || e) },
          }));
        });
    });
    return () => ctrl.abort();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [smartDisplayPools, hasInitData, apiBaseUrl, initData, chain]);

  const panelMap = {
    hot_pools: (
      <PanelShell
        title="热门池子"
        subtitle={`支持搜索与排序 · 展示前 ${HOT_POOLS_DISPLAY_LIMIT} 条`}
        icon={Flame}
      >
        <div className="sort-tabs">
          {[{ key: 'fees', label: 'Fees' }, { key: 'fee_rate', label: 'Fee Rate' }, { key: 'volume', label: 'Volume' }].map((item) => (
            <button
              type="button"
              key={item.key}
              className={`sort-tab ${hotSort === item.key ? 'active' : ''}`}
              onClick={() => setHotSort(item.key)}
            >
              {item.label}
            </button>
          ))}
          <button type="button" className={`sort-tab search-toggle ${searchOpen ? 'active' : ''}`} onClick={() => setSearchOpen((v) => !v)}>
            <Search size={12} />
          </button>
        </div>

        {searchOpen && (
          <div className="search-row">
            <Search size={14} />
            <input value={keyword} onChange={(e) => setKeyword(e.target.value)} placeholder="搜索交易对/地址" autoFocus />
          </div>
        )}

        {hotPoolsError ? <div className="error-text">{hotPoolsError}</div> : null}
        {hotTokenFilter?.address ? (
          <div className="hot-token-filter-bar">
            <span className="hot-token-filter-chip">
              同币筛选: {hotTokenFilter.symbol || shortAddress(hotTokenFilter.address, 6, 4)}
            </span>
            <button type="button" className="mini-link accent" onClick={() => setHotTokenFilter(null)}>
              清除
            </button>
          </div>
        ) : null}

        <div className="data-list">
          {hotPoolsLoading && filteredHotPools.length === 0 ? (
            <EmptyState text="正在加载热门池子..." />
          ) : filteredHotPools.length === 0 ? (
            <EmptyState text="暂无可展示的池子数据" />
          ) : (
            filteredHotPools.slice(0, HOT_POOLS_DISPLAY_LIMIT).map((pool, idx) => {
              const addr = normalizePoolAddress(pool?.pool_address || '');
              const selected = selectedPoolAddress && addr === selectedPoolAddress;
              const feePct = Number(pool?.fee_percentage || 0);
              const feeRate = Number(pool?.fee_rate || 0);
              const volume = Number(pool?.total_volume || 0);
              const totalFees = Number(pool?.total_fees || 0);
              const tvl = Number(pool?.current_pool_value || 0);
              const txCount = Number(pool?.transaction_count || 0);
              const priceDisplay = String(pool?.price_display || '');
              const factoryName = String(pool?.factory_name || pool?.dex || '');
              const userPosUsd = Number(pool?.userPositionUsd || 0);
              const pair = String(pool?.trading_pair || '--');
              const pairInitials = pair.split(/[\/\-]/).map((s) => s.trim().charAt(0).toUpperCase()).join('').slice(0, 2);
              const protocolVersion = String(pool?.protocol_version || '').trim().toUpperCase();
              const displayTokenLogoUrl = String(pool?.display_token_logo_url || '').trim();
              const displayTokenSymbol = String(pool?.display_token_symbol || '').trim();
              const avatarLabel = (displayTokenSymbol || pairInitials || 'LP').slice(0, 4).toUpperCase();
              const dex = getDexIcon(factoryName);
              const protocolTagText = protocolVersion || dex?.label || '';
              const avatarSrc = displayTokenLogoUrl || dex?.src || '';
              const filterToken = resolveHotPoolFilterToken(pool);
              const avatarFilterActive = filterToken && hotTokenFilter?.address === filterToken.address;

              const isHighFeeRate = feeRate >= 1;

              return (
                <div
                  key={`${pool?.protocol_version || ''}:${addr || idx}`}
                  className={`pool-row ${selected ? 'selected' : ''} ${isHighFeeRate ? 'high-fee' : ''}`}
                  onClick={() => selectPool({ ...pool, chain }, chain)}
                >
                  {/* Avatar */}
                  <button
                    type="button"
                    className={`pool-avatar ${filterToken ? 'filterable' : ''} ${avatarFilterActive ? 'active' : ''}`}
                    style={dex ? { borderColor: dex.color + '60' } : undefined}
                    title={filterToken ? `筛选 ${filterToken.symbol} 的池子` : '该池子无法按单一非稳定币筛选'}
                    onClick={(e) => {
                      if (!filterToken) return;
                      e.stopPropagation();
                      setHotTokenFilter((prev) => (
                        prev?.address === filterToken.address ? null : filterToken
                      ));
                    }}
                  >
                    {avatarSrc ? (
                      <>
                        <img
                          src={avatarSrc}
                          alt=""
                          data-fallback-to-dex={displayTokenLogoUrl && dex?.src ? '1' : '0'}
                          data-dex-src={dex?.src || ''}
                          onError={(e) => {
                            const nextSrc = e.currentTarget.dataset.dexSrc || '';
                            if (e.currentTarget.dataset.fallbackToDex === '1' && nextSrc) {
                              e.currentTarget.dataset.fallbackToDex = '0';
                              e.currentTarget.src = nextSrc;
                              return;
                            }
                            e.currentTarget.style.display = 'none';
                            const fallback = e.currentTarget.parentElement?.querySelector('.pool-avatar-fallback');
                            if (fallback) fallback.style.display = 'flex';
                          }}
                        />
                        <span className="pool-avatar-fallback" style={{ display: 'none' }}>{avatarLabel}</span>
                      </>
                    ) : <span>{avatarLabel}</span>}
                  </button>

                  {/* Info block */}
                  <div className="pool-info">
                    <div className="pool-name-line">
                      <span className="pool-name">{pair}</span>
                      <button type="button" className="copy-tiny" onClick={(e) => { e.stopPropagation(); copyAddr(addr); }} title="复制地址">
                        <svg viewBox="0 0 24 24" fill="currentColor" width="11" height="11"><path d="M16 1H4a2 2 0 00-2 2v14h2V3h12V1zm3 4H8a2 2 0 00-2 2v14a2 2 0 002 2h11a2 2 0 002-2V7a2 2 0 00-2-2zm0 16H8V7h11v14z"/></svg>
                      </button>
                      {feePct > 0 && <span className="tag tag-blue"><NumberFlowValue value={feePct} formatter={(v) => `${Number(v).toFixed(2).replace(/\.?0+$/, '')}%`} /></span>}
                      {protocolTagText && (
                        <span className="tag tag-dex tag-dex-inline">
                          {dex?.src ? <img src={dex.src} alt="" /> : null}
                          <span>{protocolTagText}</span>
                        </span>
                      )}
                      {userPosUsd > 0 && (
                        <span className="tag tag-purple">
                          持仓 <NumberFlowValue value={userPosUsd} formatter={(v) => formatUsdCompact(v)} />
                        </span>
                      )}
                    </div>
                    <div className="pool-meta-line">
                      <span className="meta-cyan">Vol <b><NumberFlowValue value={volume} formatter={(v) => formatUsdCompact(v)} /></b></span>
                      <span className="dot-sep" />
                      <span className="meta-cyan">TVL <b><NumberFlowValue value={tvl} formatter={(v) => formatUsdCompact(v)} /></b></span>
                      <span className="dot-sep" />
                      <span className="meta-orange"><NumberFlowValue value={txCount} formatter={(v) => `${Number(v || 0).toLocaleString()}笔`} /></span>
                      {feeRate > 0 && (<><span className="dot-sep" /><span className="meta-accent"><b><NumberFlowValue value={feeRate} formatter={(v) => `${Number(v).toFixed(3)}%`} /></b></span></>)}
                    </div>
                  </div>

                  {/* Values block */}
                  <div className="pool-values">
                    <div className="pool-main-val">
                      <NumberFlowValue
                        value={hotSort === 'volume' ? volume : hotSort === 'fee_rate' ? feeRate : totalFees}
                        formatter={(v) => hotSort === 'fee_rate' ? (Number(v) > 0 ? `${Number(v).toFixed(3)}%` : '--') : formatUsdCompact(v)}
                      />
                    </div>
                    {priceDisplay ? (
                      <div className={`pool-sub-val ${priceDisplay.includes('↑') || priceDisplay.includes('+') ? 'up' : priceDisplay.includes('↓') || priceDisplay.includes('-') ? 'down' : ''}`} title={priceDisplay}>
                        <NumberFlowValue value={priceDisplay} formatter={() => formatPriceDisplay(priceDisplay)} />
                      </div>
                    ) : feeRate > 0 && hotSort !== 'fee_rate' ? (
                      <div className="pool-sub-val purple"><NumberFlowValue value={feeRate} formatter={(v) => `${Number(v).toFixed(3)}%`} /></div>
                    ) : null}
                  </div>

                  {/* Action */}
                  <button
                    type="button"
                    className="pool-buy-btn"
                    aria-label="开仓"
                    onClick={(e) => { e.stopPropagation(); openPositionModal({ ...pool, chain, panelKey: 'hot_pools' }); }}
                  >
                    <img src={flashIcon} alt="" className="open-lightning-icon" aria-hidden="true" />
                    <span className="open-buy-text">买入</span>
                  </button>
                </div>
              );
            })
          )}
        </div>

        <div className="panel-footnote">
          更新时间: {hotPoolsUpdatedAt ? new Date(hotPoolsUpdatedAt).toLocaleTimeString() : '--'}
        </div>
        {renderOperationProgress('hot_pools')}
      </PanelShell>
    ),

    gmgn_kline: (
      <PanelShell
        title="K线行情"
        subtitle={selectedPool?.trading_pair || '请选择池子'}
        icon={CandlestickChart}
      >
        {!selectedPoolAddress ? (
          <EmptyState text="点选池子后自动加载 K 线" />
        ) : !klineTokenAddress ? (
          <EmptyState text="当前池子缺少可用代币地址，暂时无法加载 K 线" />
        ) : (
          <>
            <div className="kline-toolbar">
              <div className="kline-toolbar-group">
                {klineTokenOptions.length > 1 ? (
                  klineTokenOptions.map((item) => (
                    <button
                      key={item.key}
                      type="button"
                      className={`ghost-chip ${klineActiveTokenSide === item.key ? 'active' : ''}`}
                      onClick={() => setKlineTokenSide(item.key)}
                    >
                      {item.symbol}
                    </button>
                  ))
                ) : (
                  <div className="kline-token-pill">
                    {klineActiveToken?.symbol || 'Token'}
                  </div>
                )}
              </div>

              <div className="kline-toolbar-group">
                {KLINE_INTERVALS.map((item) => (
                  <button
                    key={item.key}
                    type="button"
                    className={`ghost-chip ${klineInterval === item.key ? 'active' : ''}`}
                    onClick={() => setKlineInterval(item.key)}
                  >
                    {item.label}
                  </button>
                ))}
              </div>

              <div className="kline-toolbar-group align-right">
                <button
                  type="button"
                  className={`ghost-chip ${klineOverlayEnabled ? 'active' : ''}`}
                  onClick={() => {
                    if (!klineOverlayAvailable) return;
                    setKlineOverlayEnabled((v) => !v);
                    setSelectedMarkerCluster(null);
                  }}
                  disabled={!klineOverlayAvailable}
                >
                  <Layers3 size={12} />
                  聪明钱
                </button>
                <button
                  type="button"
                  className="ghost-chip"
                  onClick={refreshKline}
                  disabled={klineLoading || klineMarkersLoading}
                >
                  <RefreshCw size={12} />
                  刷新
                </button>
              </div>
            </div>

            <div className="kline-summary-row">
              <div className="kline-summary-item">
                <span className="label">展示代币</span>
                <span className="value">{klineActiveToken?.symbol || '--'}</span>
              </div>
              <div className="kline-summary-item mono">
                <span className="label">地址</span>
                <span className="value">{shortAddress(klineTokenAddress, 6, 4)}</span>
              </div>
              <div className="kline-summary-item">
                <span className="label">事件({KLINE_MARKER_WINDOW_HOURS}h)</span>
                <span className="value">{klineMarkerEventCount}</span>
              </div>
              <div className="kline-summary-item">
                <span className="label">钱包</span>
                <span className="value">{klineMarkerWalletCount}</span>
              </div>
            </div>

            <KlineChart
              candles={klineCandles}
              markers={klineOverlayEnabled ? klineMarkers : []}
              loading={klineLoading}
              error={klineError}
              onVisibleRangeChange={handleKlineVisibleRangeChange}
              viewportKey={klineViewportKey}
              activeMarkerId={selectedMarkerCluster?.id || ''}
              onMarkerClick={(cluster) => setSelectedMarkerCluster(cluster)}
              userAvatarUrl={loginUser?.photo_url || ''}
            />

            {klineMarkersError ? <div className="kline-inline-note">{klineMarkersError}</div> : null}
            {!klineMarkersError && klineMarkerStats?.truncated ? (
              <div className="kline-inline-note">
                聪明钱事件较多，覆盖层当前加载最近 {formatNumber(klineMarkerStats.loadedEvents)} / {formatNumber(klineMarkerStats.totalEvents)} 条聪明钱事件。
              </div>
            ) : null}

            {selectedMarkerCluster ? (
              <div className="kline-marker-drawer">
                <div className="kline-marker-drawer-head">
                  <div>
                    <div className="kline-marker-drawer-title">
                      {selectedMarkerCluster.action === 'remove' ? '减仓活动' : '加仓活动'}
                    </div>
                    <div className="kline-marker-drawer-sub">
                      {formatUtc8DateTime(selectedMarkerCluster.time)} UTC+8 · {selectedMarkerCluster.items?.length || 0} 条
                    </div>
                  </div>
                  <button
                    type="button"
                    className="icon-link"
                    onClick={() => setSelectedMarkerCluster(null)}
                    title="关闭详情"
                  >
                    <X size={14} />
                  </button>
                </div>

                <div className="kline-marker-drawer-list">
                  {(selectedMarkerCluster.items || []).map((item) => {
                    const txUrl = String(item?.tx_url || '').trim();
                    const walletLabel = String(item?.wallet_label || '').trim();
                    const amountUSD = Number(item?.estimated_usd || 0);
                    const lower = Number(item?.price_lower || 0);
                    const upper = Number(item?.price_upper || 0);
                    const hasPnLEstimate = Boolean(item?.has_pnl_estimate);
                    const pnlEstimateUSD = Number(item?.pnl_estimate_usd || 0);
                    const costBasisUSD = Number(item?.cost_basis_usd || 0);
                    const markerChartPrice = item?.action === 'remove'
                      ? resolveMarkerCandleClose(Number(item?.t || 0))
                      : 0;
                    const chartPriceLabel = klineActiveToken?.symbol
                      ? `${klineActiveToken.symbol} K线价`
                      : 'K线价';
                    return (
                      <div key={item?.event_id || `${item?.wallet_address}:${item?.t}`} className="kline-marker-event">
                        <div className="kline-marker-event-main">
                          <div className="kline-marker-wallet">
                            {walletLabel || shortAddress(item?.wallet_address || '', 6, 4)}
                          </div>
                          <div className={`kline-marker-action ${item?.action === 'remove' ? 'remove' : 'add'}`}>
                            {item?.action === 'remove' ? '减仓' : '加仓'}
                          </div>
                        </div>
                        <div className="kline-marker-event-sub">
                          <span>{formatUtc8Time(Number(item?.t || 0))}</span>
                          <span>${formatNumber(amountUSD, amountUSD > 100 ? 0 : 2)}</span>
                          {lower > 0 && upper > 0 ? (
                            <span>{compactPrice(lower)} → {compactPrice(upper)}</span>
                          ) : null}
                          {txUrl ? (
                            <button type="button" className="mini-link" onClick={() => openExternal(txUrl)}>
                              Tx
                            </button>
                          ) : null}
                        </div>
                        {item?.action === 'remove' ? (
                          <div className="kline-marker-metrics">
                            {hasPnLEstimate ? (
                              <>
                                <span className="kline-marker-chip neutral">
                                  估算成本 {formatUsd(costBasisUSD)}
                                </span>
                                <span className={`kline-marker-chip ${pnlEstimateUSD >= 0 ? 'positive' : 'negative'}`}>
                                  估算盈亏 {formatUsd(pnlEstimateUSD)}
                                </span>
                              </>
                            ) : (
                              <span className="kline-marker-chip warn">历史成本不足，暂不显示估算盈亏</span>
                            )}
                            {Number.isFinite(markerChartPrice) && markerChartPrice > 0 ? (
                              <span className="kline-marker-chip neutral">
                                {chartPriceLabel} {compactPrice(markerChartPrice)}
                              </span>
                            ) : null}
                          </div>
                        ) : null}
                      </div>
                    );
                  })}
                </div>
              </div>
            ) : null}

            <div className="kline-ext-links">
              {selectedPoolGmgnUrl && (
                <button type="button" className="kline-ext-btn gmgn" onClick={() => openExternal(selectedPoolGmgnUrl)}>
                  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" width="13" height="13"><path d="M18 13v6a2 2 0 01-2 2H5a2 2 0 01-2-2V8a2 2 0 012-2h6"/><polyline points="15 3 21 3 21 9"/><line x1="10" y1="14" x2="21" y2="3"/></svg>
                  GMGN 查看 KOL·活动·交易者
                </button>
              )}
            </div>
          </>
        )}
      </PanelShell>
    ),

    positions: (
      <PanelShell
        title="仓位"
        subtitle={
          positions?.wallet?.address ? shortAddress(positions.wallet.address, 8, 6) : '钱包未连接'
        }
        icon={BriefcaseBusiness}
      >
        {positionsError ? <div className="error-text">{positionsError}</div> : null}

        {(() => {
          const multi = Array.isArray(walletBalances) && walletBalances.length > 1;
          const allWalletsUsd = multi
            ? walletBalances.reduce((s, w) => s + Number(w.stable_balance === 'N/A' ? 0 : w.stable_balance || 0), 0)
            : null;
          const walletUsd = allWalletsUsd !== null ? allWalletsUsd : (summary?.wallet_usd ?? 0);
          const totalUsd = walletUsd + Number(summary?.position_usd || 0) + Number(summary?.fee_usd || 0);
          const walletMetricCards = multi
            ? walletBalances.map((wb, idx) => ({
                key: String(wb?.id || wb?.address || idx),
                label: wb?.name || shortAddress(wb?.address || '', 6, 4) || `钱包 ${idx + 1}`,
                value: wb?.stable_balance !== 'N/A' ? formatUsd(wb.stable_balance) : '$--',
              }))
            : [
                {
                  key: 'wallet-total',
                  label: '钱包',
                  value: formatUsd(walletUsd),
                },
              ];
          const summaryMetricCount = walletMetricCards.length + 3;
          return (
              <div
                className="summary-grid summary-grid-wallets"
                style={{ '--summary-wallet-cols': summaryMetricCount }}
              >
                <MetricCard label="总资产" value={formatUsd(totalUsd)} tone="strong" />
                {walletMetricCards.map((card) => (
                  <MetricCard key={card.key} label={card.label} value={card.value} />
                ))}
                <MetricCard label="仓位" value={formatUsd(summary?.position_usd)} />
                <MetricCard label="手续费" value={formatUsd(summary?.fee_usd)} />
              </div>
          );
        })()}

        <div className="data-list">
          {positionsLoading && sortedPositions.length === 0 ? (
            <EmptyState text="正在加载仓位..." />
          ) : sortedPositions.length === 0 ? (
            <EmptyState text="暂无仓位数据" />
          ) : (
            sortedPositions.slice(0, 50).map((p, idx) => {
              const taskId = Number(p?.task_id || 0);
              const statusLabel = String(p?.status_label || '运行中');
              const pnl = Number(p?.absolute_pnl_usd || 0);
              const hasPnl = Boolean(p?.has_pnl) || Number.isFinite(pnl) && pnl !== 0;
              const totalVal = Number(p?.current_value_usd || p?.totals?.total_usd || 0);
              const inRange = Boolean(p?.in_range);
              const token0 = p?.token_rows?.[0];
              const token1 = p?.token_rows?.[1];
              const taskRangeLo = Number(p?.task_range_lower_pct);
              const taskRangeUp = Number(p?.task_range_upper_pct);
              const taskAmount = Number(p?.task_amount_usdt);
              const priceRange = computePriceRange(p);

              const statusClass = statusLabel.includes('错误') ? 'st-error' :
                statusLabel.includes('暂停') || statusLabel.includes('停止') || statusLabel.includes('撤出') ? 'st-warn' :
                statusLabel.includes('等待') ? 'st-wait' : 'st-ok';

              return (
                <div key={String(p?.position_id || idx)} className="pos-card">
                  <div className="pos-card-header">
                    <div className="pos-card-left"
                      onClick={() => selectPool({
                        pool_id: p?.pool_id,
                        pool_address: p?.pool_id,
                        trading_pair: p?.title,
                        protocol_version: p?.version,
                        factory_name: p?.exchange,
                        token0_address: token0?.address,
                        token1_address: token1?.address,
                        token0_symbol: token0?.symbol,
                        token1_symbol: token1?.symbol,
                        chain: p?.chain || chain,
                      }, p?.chain || chain)}>
                      <div className="pos-pair-row">
                        <span className="pos-pair-name">{p?.title || shortAddress(p?.pool_id || '')}</span>
                        {p?.tick_spacing && (
                          <span className="badge badge-fee">{
                            { 1: '0.01%', 10: '0.05%', 50: '0.25%', 60: '0.30%', 100: '0.50%', 200: '1%' }[Number(p.tick_spacing)] || ''
                          }</span>
                        )}
                      </div>
                      <div className="pos-status-row">
                        <span className={`status-pill ${statusClass}`}>
                          <span className="status-dot" />
                          {statusLabel}
                        </span>
                        {taskId > 0 && <span className="pos-task-id">#{taskId}</span>}
                        <span className={`range-pill ${inRange ? 'in' : 'out'}`}>
                          {inRange ? 'In Range' : 'Out'}
                          {priceRange?.outOfRange && (
                            <span className="range-pill-oor"> {priceRange.outOfRange.direction === 'above' ? '↑' : '↓'}{priceRange.outOfRange.pct.toFixed(1)}%</span>
                          )}
                        </span>
                        {p?.running_since && <span className="pos-running-dur">{formatDuration(p.running_since)}</span>}
                      </div>
                    </div>
                    <div className="pos-card-right-block">
                      <div className="pos-metrics">
                        <div className="pos-total">{formatUsd(totalVal)}</div>
                        {hasPnl && (
                          <div className={`pos-pnl ${pnl >= 0 ? 'positive' : 'negative'}`}>
                            {pnl >= 0 ? '+' : ''}{formatNumber(pnl, 2)}
                          </div>
                        )}
                      </div>
                      {taskId > 0 && (
                        <div className="pos-card-actions">
                          {taskId > 0 && (
                            <div className="pos-action-anchor">
                              <button type="button" className="icon-btn-tiny" onClick={(e) => { e.stopPropagation(); setTaskActionPos((prev) => prev?.task_id === p?.task_id ? null : p); }} title="任务操作">
                                <svg viewBox="0 0 24 24" fill="currentColor" width="14" height="14"><path d="M12 7a2 2 0 110-4 2 2 0 010 4zm0 7a2 2 0 110-4 2 2 0 010 4zm0 7a2 2 0 110-4 2 2 0 010 4z"/></svg>
                              </button>
                              {taskActionPos?.task_id === p?.task_id && (
                                <TaskActionMenu
                                  position={taskActionPos}
                                  onPause={handleTaskPause}
                                  onStop={handleTaskStop}
                                  onDelete={handleTaskDelete}
                                  onEditRange={handleTaskEditRange}
                                  onClose={() => setTaskActionPos(null)}
                                />
                              )}
                            </div>
                          )}
                        </div>
                      )}
                    </div>
                  </div>

                  {(token0 || token1) && (
                    <div className="pos-token-table">
                      <div className="pos-token-head">
                        <span>Token</span><span>钱包</span><span>仓位</span><span>手续费</span>
                      </div>
                      {[token0, token1].filter(Boolean).map((tk) => (
                        <div key={tk.address || tk.symbol} className="pos-token-row">
                          <div className="pos-tk-name">
                            <div>{tk.symbol}</div>
                            <div className="pos-tk-price">${Number(tk.price_usd || 0).toFixed(4)}</div>
                          </div>
                          <div className="pos-tk-cell">
                            <div>{tk.wallet_amount ?? '--'}</div>
                            <div className="pos-tk-usd">{formatUsd(tk.wallet_usd)}</div>
                          </div>
                          <div className="pos-tk-cell">
                            <div>{tk.position_amount ?? '--'}</div>
                            <div className="pos-tk-usd">{formatUsd(tk.position_usd)}</div>
                          </div>
                          <div className="pos-tk-cell fee">
                            <div>{tk.fee_amount ?? '--'}</div>
                            <div className="pos-tk-usd">{formatUsd(tk.fee_usd)}</div>
                          </div>
                        </div>
                      ))}
                      <div className="pos-token-foot">
                        <span>小计</span>
                        <span>{formatUsd(p?.totals?.wallet_usd)}</span>
                        <span>{formatUsd(p?.totals?.position_usd)}</span>
                        <span className="fee">{formatUsd(p?.totals?.fee_usd)}</span>
                      </div>
                    </div>
                  )}

                  {priceRange && (
                    <div className="pos-price-range">
                      <div className="pos-price-range-header">
                        <span className="pos-price-range-label">价格范围 ({priceRange.pairLabel}{priceRange.gridCount ? ` ${priceRange.gridCount}格` : ''})</span>
                        {Number.isFinite(priceRange.deviation) && priceRange.deviation > 0 && (
                          <span className="pos-price-range-dev">{priceRange.deviation.toFixed(2)}%</span>
                        )}
                      </div>
                      <div className="pos-price-range-bar-wrap">
                        <div className="pos-price-range-bar">
                          <div className="pos-price-range-limit lo" />
                          <div className="pos-price-range-limit hi" />
                          {priceRange.visibleGridLines?.map((pct, i) => (
                            <div key={i} className="pos-price-range-grid" style={{ left: `calc(3% + ${pct * 0.94}%)` }} />
                          ))}
                          <div
                            className={`pos-price-range-cursor ${priceRange.inRange ? 'in' : 'out'}`}
                            style={{ left: `calc(3% + ${priceRange.percent * 0.94}%)` }}
                          />
                        </div>
                      </div>
                      <div className="pos-price-range-labels">
                        <span className="lo">{compactPrice(priceRange.rangeMin)}</span>
                        <span className="cur">{compactPrice((priceRange.rangeMin + priceRange.rangeMax) / 2)}</span>
                        <span className="hi">{compactPrice(priceRange.rangeMax)}</span>
                      </div>
                    </div>
                  )}

                  {Number.isFinite(taskRangeLo) && taskRangeLo > 0 && (
                    <div className="pos-range-info">
                      <span>范围: {Math.abs(taskRangeLo - taskRangeUp) < 0.01
                        ? `±${((taskRangeLo + taskRangeUp) / 2).toFixed(2)}%`
                        : `下 ${taskRangeLo.toFixed(2)}% / 上 ${taskRangeUp.toFixed(2)}%`}
                      </span>
                      {Number.isFinite(taskAmount) && taskAmount > 0 && <span> | ${taskAmount.toFixed(2)}</span>}
                      {priceRange && <span className="pos-range-cur-price">当前价 {compactPrice(priceRange.currentPrice)}</span>}
                    </div>
                  )}
                </div>
              );
            })
          )}
        </div>
        {renderOperationProgress('positions')}
      </PanelShell>
    ),

    smart_money: (
      <PanelShell
        title="聪明钱"
        subtitle={`按参与钱包数排序的前 ${smartDisplayPools.length} 个池子`}
        icon={BrainCircuit}
      >
        {smartError ? <div className="error-text">{smartError}</div> : null}

        <div className="data-list compact">
          {smartLoading && smartDisplayPools.length === 0 ? (
            <EmptyState text="正在加载聪明钱数据..." />
          ) : smartDisplayPools.length === 0 ? (
            <EmptyState text="暂无监控钱包加 LP 数据" />
          ) : (
            smartDisplayPools.map((pool, idx) => {
              const poolKey = `${pool?.pool_version || ''}:${pool?.pool_id || ''}`;
              const adds = poolAddsMap[poolKey];
              const wallets = aggregatePoolAddWallets(adds?.wallets || []);
              const totalUsd = adds?.totalUsd || 0;
              const walletCount = wallets.length || Number(pool?.wallet_count || 0);
              const version = String(pool?.pool_version || '').toUpperCase();
              const feePct = Number(pool?.fee_pct || 0);
              const pair = String(pool?.pair || '').trim() || shortAddress(pool?.pool_id || '');
              const pairInitials = pair.split(/[\/\-]/).map((s) => s.trim().charAt(0).toUpperCase()).join('').slice(0, 2);
              const displayTokenLogoUrl = String(pool?.display_token_logo_url || '').trim();
              const displayTokenSymbol = String(pool?.display_token_symbol || '').trim();
              const dexName = String(pool?.exchange || pool?.factory_name || '').trim();
              const dex = getDexIcon(dexName);
              const protocolTagText = version || dex?.label || '';
              const avatarLabel = (displayTokenSymbol || pairInitials || 'LP').slice(0, 4).toUpperCase();
              const avatarSrc = displayTokenLogoUrl || dex?.src || '';

              return (
                <div
                  key={poolKey || idx}
                  className="sm-pool-card"
                  onClick={() =>
                    selectPool(
                      {
                        pool_id: pool?.pool_id,
                        pool_address: pool?.pool_id,
                        trading_pair: pair,
                        factory_name: dexName,
                        token0: pool?.token0,
                        token1: pool?.token1,
                        token0_symbol: pool?.token0_symbol,
                        token1_symbol: pool?.token1_symbol,
                        chain,
                      },
                      chain
                    )
                  }
                >
                  <div className="sm-pool-header">
                    <div className="sm-pool-left">
                      <div className="sm-avatar" style={dex ? { borderColor: `${dex.color}60` } : undefined}>
                        {avatarSrc ? (
                          <>
                            <img
                              src={avatarSrc}
                              alt=""
                              data-fallback-to-dex={displayTokenLogoUrl && dex?.src ? '1' : '0'}
                              data-dex-src={dex?.src || ''}
                              onError={(e) => {
                                const nextSrc = e.currentTarget.dataset.dexSrc || '';
                                if (e.currentTarget.dataset.fallbackToDex === '1' && nextSrc) {
                                  e.currentTarget.dataset.fallbackToDex = '0';
                                  e.currentTarget.src = nextSrc;
                                  return;
                                }
                                e.currentTarget.style.display = 'none';
                                const fallback = e.currentTarget.parentElement?.querySelector('.sm-avatar-fallback');
                                if (fallback) fallback.style.display = 'flex';
                              }}
                            />
                            <span className="sm-avatar-fallback" style={{ display: 'none' }}>{avatarLabel}</span>
                          </>
                        ) : <span>{avatarLabel}</span>}
                      </div>
                      <div className="sm-pool-main">
                        <div className="sm-title-row">
                          <span className="sm-pair">{pair}</span>
                          <span className="sm-rank">#{idx + 1}</span>
                        </div>
                        <div className="sm-meta-tags">
                          {protocolTagText ? (
                            <span className="tag tag-dex tag-dex-inline">
                              {dex?.src ? <img src={dex.src} alt="" /> : null}
                              <span>{protocolTagText}</span>
                            </span>
                          ) : null}
                          {feePct > 0 ? <span className="tag tag-blue">{formatPct(feePct)}</span> : null}
                        </div>
                      </div>
                    </div>
                    <div className="sm-pool-right">
                      {totalUsd > 0 ? (
                        <span className="sm-total-usd">${formatNumber(Math.round(totalUsd))}</span>
                      ) : null}
                      <span className="sm-wallet-count">
                        {formatNumber(walletCount)} 钱包
                      </span>
                    </div>
                  </div>

                  {adds?.status === 'loading' && wallets.length === 0 ? (
                    <div className="sm-wallet-loading">加载钱包明细...</div>
                  ) : null}

                  {wallets.length > 0 ? (
                    <div className="sm-wallet-list">
                      {wallets.slice(0, 5).map((w, wi) => {
                        const addr = String(w?.wallet_address || '').trim();
                        const usd = Number(w?.total_usd ?? 0);
                        const priceLower = Number(w?.price_lower ?? 0);
                        const priceUpper = Number(w?.price_upper ?? 0);
                        const quote = String(w?.price_quote || '').trim();
                        const hasRange =
                          Number.isFinite(priceLower) &&
                          priceLower > 0 &&
                          Number.isFinite(priceUpper) &&
                          priceUpper > 0;
                        const rangePct = hasRange
                          ? (Math.abs(priceUpper - priceLower) / (priceUpper + priceLower)) * 100
                          : 0;

                        return (
                          <div key={addr || wi} className="sm-wallet-row">
                            <div className="sm-wallet-addr">{shortAddress(addr, 6, 4)}</div>
                            <div className="sm-wallet-stats">
                              <span className="sm-wallet-usd">${formatNumber(Math.round(usd))}</span>
                              {hasRange ? (
                                <span className="sm-wallet-range-pct">±{rangePct.toFixed(1)}%</span>
                              ) : null}
                            </div>
                            {hasRange ? (
                              <div className="sm-wallet-range">
                                <span className="sm-range-label">区间</span>
                                <span className="sm-range-val">{compactPrice(priceLower)}</span>
                                <span className="sm-range-arrow">&rarr;</span>
                                <span className="sm-range-val">{compactPrice(priceUpper)}</span>
                                {quote ? <span className="sm-range-quote">{quote}</span> : null}
                              </div>
                            ) : null}
                          </div>
                        );
                      })}
                    </div>
                  ) : null}

                  <div className="sm-pool-actions">
                    <button
                      type="button"
                      className="sm-action-btn sm-open-btn"
                      aria-label="开仓"
                      onClick={(e) => {
                        e.stopPropagation();
                        openPositionModal({
                          pool_id: pool?.pool_id,
                          pool_address: pool?.pool_id,
                          trading_pair: pair,
                          protocol_version: version,
                          factory_name: dexName,
                          chain,
                          panelKey: 'smart_money',
                          smartMoneyWallets: wallets,
                        });
                      }}
                    >
                      <img src={flashIcon} alt="" className="open-lightning-icon" aria-hidden="true" />
                      <span className="open-buy-text">买入</span>
                    </button>
                    <button type="button" className="sm-action-btn sm-copy-btn" onClick={(e) => {
                      e.stopPropagation();
                      copyAddr(pool?.pool_id || '');
                    }}>复制池子ID</button>
                  </div>
                </div>
              );
            })
          )}
        </div>
        {renderOperationProgress('smart_money')}
      </PanelShell>
    ),
  };

  return (
    <div className={`app-shell ${workMode ? 'work-mode-shell' : ''}`} data-accent-theme={accentTheme}>
      <div className="bg-orb orb-a" />
      <div className="bg-orb orb-b" />
      <div className="bg-grid" />

      {workMode ? (
        <div className="work-mode-bar">
          <button type="button" className="work-mode-exit-btn" onClick={() => setWorkMode(false)}>
            <Minimize size={14} />
            退出工作模式
          </button>
        </div>
      ) : (
        <>
          <header className="top-bar">
            <div className="title-block">
              <div className="eyebrow">lynchL</div>
              <h1>LP交易工作台</h1>
            </div>

            <div className="top-actions">
          {loginUser ? (
            <div className="user-chip">
              {loginUser?.photo_url ? (
                <img src={loginUser.photo_url} alt="avatar" className="user-avatar" />
              ) : (
                <div className="user-avatar fallback">{String(loginUser?.first_name || '?').slice(0, 1)}</div>
              )}
              <div className="user-meta">
                <div className="user-name">{loginUser?.first_name || 'Telegram User'}</div>
                <div className="user-sub">@{loginUser?.username || 'unknown'}</div>
              </div>
              <div className="settings-wrap">
                <button type="button" className="settings-btn" onClick={() => setShowSettings((v) => !v)}>
                  <Settings size={15} />
                </button>
                {showSettings && (
                  <div className="settings-popover">
                    <div className="settings-row">
                      <span className="settings-label">数据刷新间隔</span>
                      <div className="settings-input-wrap">
                        <input
                          type="number"
                          className="settings-input"
                          min={10}
                          value={refreshInterval}
                          onChange={(e) => setRefreshInterval(e.target.value === '' ? '' : Number(e.target.value))}
                          onBlur={() => setRefreshInterval((v) => Math.max(10, Math.round(Number(v) || 10)))}
                        />
                        <span className="settings-unit">秒</span>
                      </div>
                    </div>
                    <div className="settings-row settings-row-stack">
                      <span className="settings-label">主题色</span>
                      <div className="settings-theme-group">
                        {ACCENT_THEMES.map((theme) => (
                          <button
                            key={theme.key}
                            type="button"
                            className={`settings-theme-btn ${accentTheme === theme.key ? 'active' : ''}`}
                            onClick={() => setAccentTheme(theme.key)}
                          >
                            <span className={`settings-theme-dot theme-dot-${theme.key}`} />
                            {theme.label}
                          </button>
                        ))}
                      </div>
                    </div>
                    <div className="settings-hint">默认绿色，你也可以切回黄色主色。</div>
                    <div className="settings-hint">最低 10 秒，当前每 {refreshInterval} 秒刷新</div>
                    <div className="settings-hint" style={{ marginTop: 6 }}>K线使用 REST 轮询刷新。</div>
                  </div>
                )}
              </div>
              <button type="button" className="logout-btn" onClick={logout}>
                <LogOut size={13} />
                退出
              </button>
            </div>
          ) : loginCode ? (
            <div className="login-code-box">
              <div className="login-code-top">
                <div className="login-code-badge">验证码</div>
                <div className="login-code-value">{loginCode}</div>
              </div>
              <div className="login-code-cmd-row">
                <code className="login-code-cmd">/weblogin {loginCode}</code>
                <button
                  type="button"
                  className="login-copy-btn"
                  onClick={(e) => {
                    navigator.clipboard.writeText(`/weblogin ${loginCode}`);
                    const btn = e.currentTarget;
                    btn.classList.add('copied');
                    setTimeout(() => btn.classList.remove('copied'), 1500);
                  }}
                >
                  <Copy size={12} className="copy-icon" />
                  <Check size={12} className="check-icon" />
                </button>
              </div>
              <div className="login-code-hint">在 Telegram Bot 中发送上方指令完成登录</div>
              <button type="button" className="ghost-chip" onClick={() => { setLoginCode(''); setLoginError(''); }}>
                取消
              </button>
            </div>
          ) : (
            <button
              type="button"
              className="telegram-icon-btn"
              onClick={startCodeLogin}
              disabled={loginBusy}
              title="获取登录验证码"
              aria-label="获取登录验证码"
            >
              <img src={telegramLogo} alt="Telegram" />
            </button>
          )}
        </div>
      </header>

      {loginError ? <div className="error-text top-error">{loginError}</div> : null}

      <section className="config-panel">
        <div className="config-head">
          <SlidersHorizontal size={14} />
          <span>布局与链设置</span>
        </div>

        <div className="chain-toggles">
          <button type="button" className={`chain-btn ${chain === 'bsc' ? 'active' : ''}`} onClick={() => setChain('bsc')}>
            <img src={bnbLogo} alt="BSC" className="chain-icon" />
            <span>BSC</span>
          </button>
          <button type="button" className={`chain-btn ${chain === 'base' ? 'active' : ''}`} onClick={() => setChain('base')}>
            <img src={baseLogo} alt="Base" className="chain-icon" />
            <span>Base</span>
          </button>
        </div>

        <div className="widget-toggles">
          {WIDGETS.map((item) => (
            <button
              type="button"
              key={item.key}
              className={`toggle-chip ${widgets.includes(item.key) ? 'active' : ''}`}
              onClick={() => toggleWidget(item.key)}
            >
              {item.label}
            </button>
          ))}
          <button type="button" className="work-mode-btn" onClick={() => setWorkMode(true)}>
            <Maximize size={13} />
            工作模式
          </button>
        </div>

        {!hasInitData ? (
          <div className="warning-box">
            <AlertTriangle size={14} />
            <span>请点击右上角 Telegram 图标获取验证码，在 Bot 中发送 /weblogin 验证码 完成登录。</span>
          </div>
        ) : null}
      </section>
      </>
      )}

      <main className={`workbench ${workLayoutClass}`}>
        {activeWidgets.map((widget) => (
          <div
            key={widget.key}
            className={`module-slot module-${widget.key} ${
              draggingKey === widget.key ? 'dragging' : ''
            } ${dragOverKey === widget.key ? 'drop-target' : ''}`}
            onDragOver={(e) => {
              e.preventDefault();
              e.dataTransfer.dropEffect = 'move';
              if (draggingKey && draggingKey !== widget.key) {
                setDragOverKey(widget.key);
              }
            }}
            onDrop={(e) => {
              e.preventDefault();
              const from = e.dataTransfer.getData('text/plain') || draggingKey;
              if (from && from !== widget.key) {
                setWidgets((prev) => reorderList(prev, from, widget.key));
              }
              setDraggingKey('');
              setDragOverKey('');
            }}
            onDragEnd={() => {
              setDraggingKey('');
              setDragOverKey('');
            }}
          >
            <div
              className="drag-hint"
              draggable
              title="按住拖动调整模块顺序"
              onDragStart={(e) => {
                setDraggingKey(widget.key);
                e.dataTransfer.effectAllowed = 'move';
                e.dataTransfer.setData('text/plain', widget.key);
              }}
              onDragEnd={() => {
                setDraggingKey('');
                setDragOverKey('');
              }}
            >
              <GripVertical size={12} />
            </div>
            {panelMap[widget.key]}
          </div>
        ))}
      </main>

      {openPosPool && (
        <OpenPositionModal
          pool={openPosPool}
          chain={openPosPool?.chain || chain}
          wallets={openPosWallets}
          walletsLoading={openPosWalletsLoading}
          selectedWalletId={openPosWalletId}
          onWalletSelect={(id) => { setOpenPosWalletId(id); storageSet(STORAGE.walletId, String(id)); }}
          onSubmit={handleOpenPosition}
          onClose={() => setOpenPosPool(null)}
          busy={openPosBusy}
        />
      )}
    </div>
  );
}
