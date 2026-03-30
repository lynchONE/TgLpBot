import React, { Suspense, lazy, useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
  AlertTriangle,
  BriefcaseBusiness,
  CandlestickChart,
  Check,
  Copy,
  Flame,
  GripVertical,
  LogOut,
  Maximize,
  Minimize,
  MousePointer2,
  RefreshCw,
  Search,
  Settings,
  Slash,
  SlidersHorizontal,
  Square,
  X,
} from 'lucide-react';
import {
  checkLoginCode,
  deleteTask,
  fetchHotPools,
  fetchMyTradeMarkers,
  fetchRealtimePositions,
  fetchSmartMoneyPoolMarkers,
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
import CreatePoolPanel from './components/CreatePoolPanel';
import SmartMoneyDashboard from './components/SmartMoneyDashboard';
import OpenPositionModal from './components/OpenPositionModal';
import StepProgressModal from './components/StepProgressModal';
import TaskActionMenu from './components/TaskActionMenu';
import NumberFlowValue from './components/NumberFlowValue';
import { fetchSMPoolStats, updateSMWallet } from './smartMoneyApi';
import telegramLogo from './img/telegram.svg';
import uniswapLogo from './img/uniswap.svg';
import pancakeLogo from './img/pancake.svg';
import bnbLogo from './img/bnb.svg';
import baseLogo from './img/base.svg';
import flashIcon from './img/flash.svg';
import siteLogo from './img/logo.png';
import {
  DEFAULT_WIDGETS,
  WIDGETS,
  buildGmgnUrl,
  compactPrice,
  formatNumber,
  formatPct,
  formatPriceDisplay,
  formatUsd,
  formatUsdCompact,
  computeHotPoolActiveFeeRate,
  normalizePoolAddress,
  normalizeHexAddress,
  normalizeWidgetSelection,
  parseHotPoolBadges,
  pickNonStableTokenAddress,
  resolveHotPoolFilterToken,
  resolveKlineTokenOptions,
  shortAddress,
  inferPoolVersion,
  computePriceRange,
  formatDuration,
} from './utils';

const LazyAssetManagementPanel = lazy(() => import('./components/AssetManagementPanel'));

const KLINE_INTERVALS = [
  { key: '1m', label: '1m', bucketSec: 60, limit: 240, timeframe: 'minute', aggregate: 1, poolLimit: 300 },
  { key: '5m', label: '5m', bucketSec: 300, limit: 240, timeframe: 'minute', aggregate: 5, poolLimit: 260 },
  { key: '15m', label: '15m', bucketSec: 900, limit: 240, timeframe: 'minute', aggregate: 15, poolLimit: 220 },
  { key: '1H', label: '1H', bucketSec: 3600, limit: 240, timeframe: 'hour', aggregate: 1, poolLimit: 200 },
];
const HOT_POOLS_DISPLAY_LIMIT = 20;
const DEFAULT_KLINE_CHART_HEIGHT = 520;
const MIN_KLINE_CHART_HEIGHT = 360;
const MAX_KLINE_CHART_HEIGHT = 1200;
const DEFAULT_HOT_POOLS_PANEL_HEIGHT_FALLBACK = 760;
const MIN_HOT_POOLS_PANEL_HEIGHT = 420;
const MAX_HOT_POOLS_PANEL_HEIGHT = 1400;
const ACCENT_THEMES = [
  { key: 'green', label: '绿色' },
  { key: 'yellow', label: '黄色' },
];
const KLINE_DRAW_TOOLS = [
  { key: 'none', title: 'Cursor', icon: MousePointer2 },
  { key: 'line', title: 'Line', icon: Slash },
  { key: 'rect', title: 'Rect', icon: Square },
];
const HOT_POOL_SORT_OPTIONS = [
  { key: 'fees', label: 'Fees', serverKey: 'fees' },
  { key: 'volume', label: 'Volume', serverKey: 'volume' },
  { key: 'tvl', label: 'TVL', serverKey: 'volume' },
  { key: 'tx_count', label: 'Tx', serverKey: 'volume' },
  { key: 'fee_rate', label: 'Fee Rate', serverKey: 'fee_rate' },
  { key: 'active_fee_rate', label: 'Active', serverKey: 'fee_rate' },
];

function getKlineIntervalMeta(bar) {
  return KLINE_INTERVALS.find((item) => item.key === bar) || KLINE_INTERVALS[0];
}

function normalizeHotPoolSort(value) {
  const key = String(value || '').trim().toLowerCase();
  return key === 'fee_rate' || key === 'volume' || key === 'fees' ? key : 'fees';
}

function resolveHotPoolServerSort(sortKey) {
  return HOT_POOL_SORT_OPTIONS.find((item) => item.key === sortKey)?.serverKey || 'fees';
}

function getHotPoolSortRankValue(pool, sortKey) {
  switch (sortKey) {
    case 'volume':
      return Number(pool?.total_volume || 0);
    case 'tvl':
      return Number(pool?.current_pool_value || 0);
    case 'tx_count':
      return Number(pool?.transaction_count || 0);
    case 'fee_rate': {
      const tvl = Number(pool?.current_pool_value || 0);
      const feeRate = Number(pool?.fee_rate || 0);
      return Number.isFinite(tvl) && tvl > 0 && Number.isFinite(feeRate) ? feeRate : Number.NEGATIVE_INFINITY;
    }
    case 'active_fee_rate': {
      const value = computeHotPoolActiveFeeRate(pool);
      return Number.isFinite(value) ? value : Number.NEGATIVE_INFINITY;
    }
    case 'fees':
    default:
      return Number(pool?.total_fees || 0);
  }
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

function clampKlineChartHeight(value) {
  const numeric = Math.round(Number(value));
  if (!Number.isFinite(numeric)) return DEFAULT_KLINE_CHART_HEIGHT;
  return Math.min(MAX_KLINE_CHART_HEIGHT, Math.max(MIN_KLINE_CHART_HEIGHT, numeric));
}

function getDefaultHotPoolsPanelHeight() {
  if (typeof window !== 'undefined') {
    const viewportHeight = Number(window.innerHeight || 0);
    if (Number.isFinite(viewportHeight) && viewportHeight > 0) {
      return Math.round(viewportHeight * 0.75);
    }
  }
  return DEFAULT_HOT_POOLS_PANEL_HEIGHT_FALLBACK;
}

function clampHotPoolsPanelHeight(value, fallback = getDefaultHotPoolsPanelHeight()) {
  const numeric = Math.round(Number(value));
  if (!Number.isFinite(numeric)) return fallback;
  return Math.min(MAX_HOT_POOLS_PANEL_HEIGHT, Math.max(MIN_HOT_POOLS_PANEL_HEIGHT, numeric));
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
  klineHeight: 'tglp_web_kline_height',
  hotPoolsHeight: 'tglp_web_hot_pools_height',
  hotPoolsFilter: 'tglp_web_hot_pools_filter_v1',
  smartMoneyWatchWallets: 'tglp_web_sm_watch_wallets',
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

function normalizeWalletAddress(value) {
  const raw = String(value || '').trim();
  if (!/^0x[0-9a-fA-F]{40}$/.test(raw)) return '';
  return `0x${raw.slice(2).toLowerCase()}`;
}

function parseStoredWatchWallets(raw) {
  if (!raw) return [];
  try {
    const values = JSON.parse(raw);
    if (!Array.isArray(values)) return [];
    return Array.from(
      new Set(values.map((item) => normalizeWalletAddress(item)).filter(Boolean))
    ).sort();
  } catch {
    return [];
  }
}

function parseKlineMarkerFilterUsd(raw) {
  const text = String(raw ?? '').replace(/,/g, '').trim();
  if (!text) return 0;
  const value = Number(text);
  return Number.isFinite(value) && value > 0 ? value : 0;
}

const HOT_POOLS_FILTER_DEFAULTS = {
  minFees: 60,
  minFeeRate: 0.3,
  minActiveFeeRate: null,
  minTvl: 1000,
  minVolume: 2000,
  minTxCount: null,
};

const defaultHotPoolsFilter = {
  enabled: false,
  keyword: '',
  ...HOT_POOLS_FILTER_DEFAULTS,
};

function parseNullableNumber(value) {
  if (value === null || value === undefined || value === '') return null;
  const n = Number(value);
  if (!Number.isFinite(n)) return null;
  return Math.max(0, n);
}

function parseMetricNumber(value) {
  if (value === null || value === undefined || value === '') return NaN;
  const raw = typeof value === 'string' ? value.replace(/,/g, '').trim() : value;
  const direct = Number(raw);
  if (Number.isFinite(direct)) return direct;
  const match = String(value).match(/-?\d+(\.\d+)?/);
  if (!match) return NaN;
  const parsed = Number(match[0]);
  return Number.isFinite(parsed) ? parsed : NaN;
}

function normalizeHotPoolsFilter(value) {
  const base = { ...defaultHotPoolsFilter };
  if (!value || typeof value !== 'object') return base;
  if (Object.prototype.hasOwnProperty.call(value, 'enabled')) {
    base.enabled = Boolean(value.enabled);
  }
  if (Object.prototype.hasOwnProperty.call(value, 'keyword')) {
    const raw = String(value.keyword ?? '').trim();
    base.keyword = raw.length > 64 ? raw.slice(0, 64) : raw;
  }
  if (Object.prototype.hasOwnProperty.call(value, 'minFees')) {
    base.minFees = parseNullableNumber(value.minFees);
  }
  if (Object.prototype.hasOwnProperty.call(value, 'minFeeRate')) {
    base.minFeeRate = parseNullableNumber(value.minFeeRate);
  }
  if (Object.prototype.hasOwnProperty.call(value, 'minActiveFeeRate')) {
    base.minActiveFeeRate = parseNullableNumber(value.minActiveFeeRate);
  }
  if (Object.prototype.hasOwnProperty.call(value, 'minTvl')) {
    base.minTvl = parseNullableNumber(value.minTvl);
  }
  if (Object.prototype.hasOwnProperty.call(value, 'minVolume')) {
    base.minVolume = parseNullableNumber(value.minVolume);
  }
  if (Object.prototype.hasOwnProperty.call(value, 'minTxCount')) {
    base.minTxCount = parseNullableNumber(value.minTxCount);
  }
  return base;
}

function parseDraftNumber(raw) {
  const text = String(raw || '').replace(/,/g, '').trim();
  if (!text) return null;
  const match = text.match(/-?\d+(\.\d+)?/);
  if (!match) return null;
  const n = Number(match[0]);
  if (!Number.isFinite(n)) return null;
  return Math.max(0, n);
}

function formatDraftNumber(value) {
  return Number.isFinite(value) ? String(value) : '';
}

function buildHotPoolsFilterDraft(filter) {
  return {
    enabled: Boolean(filter?.enabled),
    keyword: String(filter?.keyword || ''),
    minFees: formatDraftNumber(filter?.minFees),
    minFeeRate: formatDraftNumber(filter?.minFeeRate),
    minActiveFeeRate: formatDraftNumber(filter?.minActiveFeeRate),
    minTvl: formatDraftNumber(filter?.minTvl),
    minVolume: formatDraftNumber(filter?.minVolume),
    minTxCount: formatDraftNumber(filter?.minTxCount),
  };
}

function klineMarkerEventId(marker) {
  return String(marker?.event_id || '').trim();
}

function markerWalletDisplayName(marker) {
  const label = String(marker?.wallet_label || '').trim();
  if (label) return label;
  const address = normalizeWalletAddress(marker?.wallet_address);
  return address ? address.slice(-4) : '--';
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
    return normalizeHotPoolSort(storageGet(STORAGE.sort));
  });
  const [hotInlineSort, setHotInlineSort] = useState('');

  const [keyword, setKeyword] = useState('');
  const [searchOpen, setSearchOpen] = useState(false);
  const [hotPools, setHotPools] = useState([]);
  const [hotPoolsLoading, setHotPoolsLoading] = useState(false);
  const [hotPoolsError, setHotPoolsError] = useState('');
  const [hotPoolsUpdatedAt, setHotPoolsUpdatedAt] = useState('');
  const [hotTokenFilter, setHotTokenFilter] = useState(null);
  const [hotPoolsFilterOpen, setHotPoolsFilterOpen] = useState(false);
  const [hotPoolsFilter, setHotPoolsFilter] = useState(() => {
    const saved = storageGet(STORAGE.hotPoolsFilter);
    if (!saved) return defaultHotPoolsFilter;
    try {
      return normalizeHotPoolsFilter(JSON.parse(saved));
    } catch {
      return defaultHotPoolsFilter;
    }
  });
  const [hotPoolsFilterDraft, setHotPoolsFilterDraft] = useState(() =>
    buildHotPoolsFilterDraft(defaultHotPoolsFilter)
  );
  const hotPoolsDefaultHeightRef = useRef(getDefaultHotPoolsPanelHeight());
  const [hotPoolsHeightSettingsOpen, setHotPoolsHeightSettingsOpen] = useState(false);
  const [hotPoolsPanelHeight, setHotPoolsPanelHeight] = useState(() =>
    clampHotPoolsPanelHeight(
      storageGet(STORAGE.hotPoolsHeight) || hotPoolsDefaultHeightRef.current,
      hotPoolsDefaultHeightRef.current
    )
  );

  const [positions, setPositions] = useState(null);
  const [positionsLoading, setPositionsLoading] = useState(false);
  const [positionsError, setPositionsError] = useState('');

  const [walletBalances, setWalletBalances] = useState(null);
  const [walletBalancesChain, setWalletBalancesChain] = useState('');

  const [selectedPool, setSelectedPool] = useState(null);
  const [klineInterval, setKlineInterval] = useState('1m');
  const [klineTokenSide, setKlineTokenSide] = useState('auto');
  const [klineCandles, setKlineCandles] = useState([]);
  const [klineLoading, setKlineLoading] = useState(false);
  const [klineError, setKlineError] = useState('');
  const [klineSource, setKlineSource] = useState('');
  const [klineRefreshNonce, setKlineRefreshNonce] = useState(0);
  const [klineDrawTool, setKlineDrawTool] = useState('none');
  const [klineDrawResetNonce, setKlineDrawResetNonce] = useState(0);
  const [klineHeightSettingsOpen, setKlineHeightSettingsOpen] = useState(false);
  const [klineMarkerFilterOpen, setKlineMarkerFilterOpen] = useState(false);
  const [klineMarkerMinUsdInput, setKlineMarkerMinUsdInput] = useState('');
  const [klineMarkerWalletSelection, setKlineMarkerWalletSelection] = useState([]);
  const [klineChartHeight, setKlineChartHeight] = useState(() =>
    clampKlineChartHeight(storageGet(STORAGE.klineHeight) || DEFAULT_KLINE_CHART_HEIGHT)
  );
  const [klineMarkers, setKlineMarkers] = useState([]);
  const [klineMarkersLoading, setKlineMarkersLoading] = useState(false);
  const [klineMarkersError, setKlineMarkersError] = useState('');
  const [klineActiveMarkerId, setKlineActiveMarkerId] = useState('');
  const [klineFocusedWalletAddress, setKlineFocusedWalletAddress] = useState('');
  const [klineWatchedWallets, setKlineWatchedWallets] = useState(() =>
    parseStoredWatchWallets(storageGet(STORAGE.smartMoneyWatchWallets))
  );
  const [klineWatchToggleMap, setKlineWatchToggleMap] = useState({});

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
  const klineToolDockRef = useRef(null);
  const hotPoolsHeightControlRef = useRef(null);
  const hotPoolsFilterRef = useRef(null);

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
  const hotPoolsFilterEnabled = useMemo(() => {
    if (!hotPoolsFilter.enabled) return false;
    const hasKeyword = String(hotPoolsFilter.keyword || '').trim().length > 0;
    const hasNumbers = [
      hotPoolsFilter.minFees,
      hotPoolsFilter.minFeeRate,
      hotPoolsFilter.minActiveFeeRate,
      hotPoolsFilter.minTvl,
      hotPoolsFilter.minVolume,
      hotPoolsFilter.minTxCount,
    ].some((value) => Number.isFinite(value));
    return hasKeyword || hasNumbers;
  }, [hotPoolsFilter]);
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
  const activeHotPoolRankSort = hotInlineSort || hotSort;
  const filteredHotPools = useMemo(() => {
    const q = String(keyword || '').trim().toLowerCase();
    const filterKeyword = hotPoolsFilterEnabled
      ? String(hotPoolsFilter.keyword || '').trim().toLowerCase()
      : '';
    const minFees = hotPoolsFilterEnabled ? hotPoolsFilter.minFees : null;
    const minFeeRate = hotPoolsFilterEnabled ? hotPoolsFilter.minFeeRate : null;
    const minActiveFeeRate = hotPoolsFilterEnabled ? hotPoolsFilter.minActiveFeeRate : null;
    const minTvl = hotPoolsFilterEnabled ? hotPoolsFilter.minTvl : null;
    const minVolume = hotPoolsFilterEnabled ? hotPoolsFilter.minVolume : null;
    const minTxCount = hotPoolsFilterEnabled ? hotPoolsFilter.minTxCount : null;
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
        const hasPosition = Boolean(addr && positionPoolMap.has(addr));
        if (hasPosition) return true;

        if (q) {
          const pair = String(row?.trading_pair || '').toLowerCase();
          if (!pair.includes(q) && !String(addr || '').toLowerCase().includes(q)) return false;
        }

        if (filterKeyword) {
          const pair = String(row?.trading_pair || '').toLowerCase();
          const token0 = String(row?.token0_address || '').toLowerCase();
          const token1 = String(row?.token1_address || '').toLowerCase();
          const addrText = String(addr || '').toLowerCase();
          const matched = (
            pair.includes(filterKeyword) ||
            addrText.includes(filterKeyword) ||
            token0.includes(filterKeyword) ||
            token1.includes(filterKeyword)
          );
          if (!matched) return false;
        }

        const fees = parseMetricNumber(row?.total_fees);
        const feeRate = parseMetricNumber(row?.fee_rate);
        const activeFeeRate = computeHotPoolActiveFeeRate(row);
        const tvl = parseMetricNumber(row?.current_pool_value);
        const volume = parseMetricNumber(row?.total_volume);
        const txCount = parseMetricNumber(row?.transaction_count);
        if (Number.isFinite(minFees) && fees < minFees) return false;
        if (Number.isFinite(minFeeRate) && feeRate < minFeeRate) return false;
        if (Number.isFinite(minActiveFeeRate) && (!Number.isFinite(activeFeeRate) || activeFeeRate < minActiveFeeRate)) return false;
        if (Number.isFinite(minTvl) && tvl < minTvl) return false;
        if (Number.isFinite(minVolume) && volume < minVolume) return false;
        if (Number.isFinite(minTxCount) && txCount < minTxCount) return false;
        return true;
      })
      .map((row, index) => {
        const addr = normalizePoolAddress(row?.pool_address || row?.pool_id);
        return {
          ...row,
          userPositionUsd: addr ? Number(positionPoolMap.get(addr) || 0) : 0,
          _sortValue: getHotPoolSortRankValue(row, activeHotPoolRankSort),
          _listIndex: index,
        };
      });

    return enriched
      .sort((a, b) => {
        const aMetric = typeof a?._sortValue === 'number' ? a._sortValue : Number.NEGATIVE_INFINITY;
        const bMetric = typeof b?._sortValue === 'number' ? b._sortValue : Number.NEGATIVE_INFINITY;
        if (aMetric !== bMetric) return bMetric - aMetric;
        const aPos = Number(a?.userPositionUsd || 0);
        const bPos = Number(b?.userPositionUsd || 0);
        if (aPos !== bPos) return bPos - aPos;
        return Number(a?._listIndex || 0) - Number(b?._listIndex || 0);
      })
      .map(({ _listIndex, _sortValue, ...row }) => row);
  }, [activeHotPoolRankSort, hotPools, hotPoolsFilter, hotPoolsFilterEnabled, keyword, positions]);
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
  const walletMetaByKey = useMemo(() => {
    const map = new Map();
    (Array.isArray(walletBalances) ? walletBalances : []).forEach((wallet, index) => {
      const id = Number(wallet?.id || 0);
      const address = normalizeWalletAddress(wallet?.address);
      const label = String(wallet?.name || '').trim() || shortAddress(address || `wallet-${index}`, 6, 4);
      const meta = { id, address, label, isDefault: Boolean(wallet?.is_default) };
      if (id > 0) map.set(`id:${id}`, meta);
      if (address) map.set(`addr:${address}`, meta);
    });
    return map;
  }, [walletBalances]);

  const klineChartHeightCustomized = klineChartHeight !== DEFAULT_KLINE_CHART_HEIGHT;
  const hotPoolsPanelHeightCustomized = hotPoolsPanelHeight !== hotPoolsDefaultHeightRef.current;
  const klineViewportKey = useMemo(
    () => `${selectedPoolAddress || 'pool'}:${klineTokenAddress || 'token'}:${klineInterval}`,
    [klineInterval, klineTokenAddress, selectedPoolAddress]
  );
  const klineMarkerRange = useMemo(() => {
    if (!Array.isArray(klineCandles) || !klineCandles.length) return null;
    const from = Number(klineCandles[0]?.t || 0);
    const to = Number(klineCandles[klineCandles.length - 1]?.t || 0);
    if (!from || !to) return null;
    return from <= to ? { from, to } : { from: to, to: from };
  }, [klineCandles]);
  const klineMarkerFilterMinUsd = useMemo(
    () => parseKlineMarkerFilterUsd(klineMarkerMinUsdInput),
    [klineMarkerMinUsdInput]
  );
  const klineMarkerWalletOptions = useMemo(() => {
    const byWallet = new Map();
    klineMarkers.forEach((marker) => {
      const address = normalizeWalletAddress(marker?.wallet_address);
      if (!address) return;
      const estimatedUsd = Number(marker?.estimated_usd || 0);
      const latestTs = Number(marker?.t || marker?.bucket_t || 0);
      const nextLabel = markerWalletDisplayName(marker);
      const current = byWallet.get(address);
      if (!current) {
        byWallet.set(address, {
          address,
          label: nextLabel,
          latestTs,
          markerCount: 1,
          maxEstimatedUsd: Number.isFinite(estimatedUsd) && estimatedUsd > 0 ? estimatedUsd : 0,
        });
        return;
      }
      current.markerCount += 1;
      current.maxEstimatedUsd = Math.max(
        current.maxEstimatedUsd,
        Number.isFinite(estimatedUsd) && estimatedUsd > 0 ? estimatedUsd : 0
      );
      if (latestTs >= current.latestTs) {
        current.latestTs = latestTs;
        current.label = nextLabel;
      }
    });
    return Array.from(byWallet.values()).sort((a, b) => {
      if (b.latestTs !== a.latestTs) return b.latestTs - a.latestTs;
      if (b.maxEstimatedUsd !== a.maxEstimatedUsd) return b.maxEstimatedUsd - a.maxEstimatedUsd;
      return a.label.localeCompare(b.label);
    });
  }, [klineMarkers]);
  const klineMarkerWalletSelectionSet = useMemo(
    () => new Set(klineMarkerWalletSelection),
    [klineMarkerWalletSelection]
  );
  const klineFilteredMarkers = useMemo(() => (
    klineMarkers.filter((marker) => {
      const isMyTrade = Boolean(marker?.is_my_trade);
      const walletAddress = normalizeWalletAddress(marker?.wallet_address);
      if (!isMyTrade && klineMarkerWalletSelectionSet.size > 0 && !klineMarkerWalletSelectionSet.has(walletAddress)) {
        return false;
      }
      if (klineMarkerFilterMinUsd > 0) {
        const estimatedUsd = Number(marker?.estimated_usd || 0);
        if (!Number.isFinite(estimatedUsd) || estimatedUsd < klineMarkerFilterMinUsd) return false;
      }
      return true;
    })
  ), [klineMarkerFilterMinUsd, klineMarkerWalletSelectionSet, klineMarkers]);
  const klineFilterActive = klineMarkerFilterMinUsd > 0 || klineMarkerWalletSelection.length > 0;
  const klineFilteredOutCount = Math.max(0, klineMarkers.length - klineFilteredMarkers.length);
  const klineAllWalletsSelected = (
    klineMarkerWalletOptions.length > 0 &&
    klineMarkerWalletSelection.length === klineMarkerWalletOptions.length
  );
  const klineWatchedWalletSet = useMemo(
    () => new Set(klineWatchedWallets),
    [klineWatchedWallets]
  );
  const klineRangeOverlays = useMemo(() => {
    if (!klineMarkers.length || !klineWatchedWallets.length) return [];
    const watched = new Set(klineWatchedWallets);
    const latestByWallet = new Map();
    klineMarkers.forEach((marker, index) => {
      if (String(marker?.action || '').toLowerCase() !== 'add') return;
      const walletAddress = normalizeWalletAddress(marker?.wallet_address);
      if (!walletAddress || !watched.has(walletAddress)) return;
      const markerTime = Number(marker?.t || marker?.bucket_t || 0);
      const current = latestByWallet.get(walletAddress);
      if (!current || markerTime >= current.markerTime) {
        latestByWallet.set(walletAddress, { marker, index, markerTime });
      }
    });
    return Array.from(latestByWallet.values())
      .map(({ marker, index }) => {
        const walletAddress = normalizeWalletAddress(marker?.wallet_address);
        const priceLower = Number(marker?.price_lower || 0);
        const priceUpper = Number(marker?.price_upper || 0);
        const midPrice = Number(marker?.mid_price || marker?.anchor_price || 0) ||
          (priceLower > 0 && priceUpper > 0 ? (priceLower + priceUpper) / 2 : 0);
        if (!walletAddress || !Number.isFinite(midPrice) || midPrice <= 0) return null;
        const overlayKey = klineMarkerEventId(marker) || `${walletAddress}:${index}`;
        return {
          id: `watch:${overlayKey}`,
          type: 'mid',
          color: 'blue',
          price: midPrice,
          label: markerWalletDisplayName(marker),
        };
      })
      .filter(Boolean);
  }, [klineMarkers, klineWatchedWallets]);
  const clearKlineDrawing = useCallback(() => {
    setKlineDrawResetNonce((prev) => prev + 1);
  }, []);
  const resetKlineChartHeight = useCallback(() => {
    setKlineChartHeight(DEFAULT_KLINE_CHART_HEIGHT);
  }, []);
  const resetHotPoolsPanelHeight = useCallback(() => {
    setHotPoolsPanelHeight(hotPoolsDefaultHeightRef.current);
  }, []);
  const openHotPoolsFilter = useCallback(() => {
    setSearchOpen(false);
    setHotPoolsFilterDraft(buildHotPoolsFilterDraft(hotPoolsFilter));
    setHotPoolsFilterOpen(true);
  }, [hotPoolsFilter]);
  const applyHotPoolsFilter = useCallback(() => {
    const next = normalizeHotPoolsFilter({
      enabled: hotPoolsFilterDraft.enabled,
      keyword: String(hotPoolsFilterDraft.keyword || '').trim(),
      minFees: parseDraftNumber(hotPoolsFilterDraft.minFees),
      minFeeRate: parseDraftNumber(hotPoolsFilterDraft.minFeeRate),
      minActiveFeeRate: parseDraftNumber(hotPoolsFilterDraft.minActiveFeeRate),
      minTvl: parseDraftNumber(hotPoolsFilterDraft.minTvl),
      minVolume: parseDraftNumber(hotPoolsFilterDraft.minVolume),
      minTxCount: parseDraftNumber(hotPoolsFilterDraft.minTxCount),
    });
    setHotPoolsFilter(next);
    storageSet(STORAGE.hotPoolsFilter, JSON.stringify(next));
    setHotPoolsFilterOpen(false);
  }, [hotPoolsFilterDraft]);
  const resetHotPoolsFilter = useCallback(() => {
    const next = normalizeHotPoolsFilter({
      enabled: true,
      keyword: '',
      ...HOT_POOLS_FILTER_DEFAULTS,
    });
    setHotPoolsFilter(next);
    setHotPoolsFilterDraft(buildHotPoolsFilterDraft(next));
    storageSet(STORAGE.hotPoolsFilter, JSON.stringify(next));
    setHotPoolsFilterOpen(false);
  }, []);
  const clearHotPoolsFilter = useCallback(() => {
    const next = normalizeHotPoolsFilter({
      enabled: false,
      keyword: '',
      minFees: null,
      minFeeRate: null,
      minActiveFeeRate: null,
      minTvl: null,
      minVolume: null,
      minTxCount: null,
    });
    setHotPoolsFilter(next);
    setHotPoolsFilterDraft(buildHotPoolsFilterDraft(next));
    storageSet(STORAGE.hotPoolsFilter, JSON.stringify(next));
    setHotPoolsFilterOpen(false);
  }, []);

  useEffect(() => {
    if (!klineMarkerWalletSelection.length) return;
    const validWallets = new Set(klineMarkerWalletOptions.map((item) => item.address));
    setKlineMarkerWalletSelection((prev) => {
      const next = prev.filter((address) => validWallets.has(address));
      return next.length === prev.length ? prev : next;
    });
  }, [klineMarkerWalletOptions, klineMarkerWalletSelection.length]);

  useEffect(() => {
    if (!klineActiveMarkerId) return;
    const stillVisible = klineFilteredMarkers.some(
      (marker) => klineMarkerEventId(marker) === klineActiveMarkerId
    );
    if (stillVisible) return;
    setKlineActiveMarkerId('');
    setKlineFocusedWalletAddress('');
  }, [klineActiveMarkerId, klineFilteredMarkers]);

  useEffect(() => {
    if (!klineHeightSettingsOpen && !klineMarkerFilterOpen && !hotPoolsHeightSettingsOpen && !hotPoolsFilterOpen) return undefined;
    const handlePointerDown = (event) => {
      if (klineToolDockRef.current?.contains(event.target)) return;
      if (hotPoolsHeightControlRef.current?.contains(event.target)) return;
      if (hotPoolsFilterRef.current?.contains(event.target)) return;
      setKlineHeightSettingsOpen(false);
      setKlineMarkerFilterOpen(false);
      setHotPoolsHeightSettingsOpen(false);
      setHotPoolsFilterOpen(false);
    };
    document.addEventListener('mousedown', handlePointerDown);
    return () => document.removeEventListener('mousedown', handlePointerDown);
  }, [hotPoolsFilterOpen, hotPoolsHeightSettingsOpen, klineHeightSettingsOpen, klineMarkerFilterOpen]);

  useEffect(() => {
    storageSet(STORAGE.chain, chain);
    storageSet(STORAGE.widgets, JSON.stringify(widgets));
    storageSet(STORAGE.sort, hotSort);
    storageSet(STORAGE.refreshInterval, String(refreshInterval));
    storageSet(STORAGE.accentTheme, accentTheme);
    storageSet(STORAGE.klineHeight, String(klineChartHeight));
    storageSet(STORAGE.hotPoolsHeight, String(hotPoolsPanelHeight));
    storageSet(STORAGE.smartMoneyWatchWallets, JSON.stringify(klineWatchedWallets));

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
  }, [accentTheme, chain, hotPoolsPanelHeight, hotSort, initData, klineChartHeight, klineWatchedWallets, loginUser, refreshInterval, widgets]);

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
          sort: resolveHotPoolServerSort(hotSort),
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
      if (!hasInitData || !selectedPoolAddress || !klineMarkerRange) {
        setKlineMarkers([]);
        setKlineMarkersError('');
        setKlineMarkersLoading(false);
        return;
      }

      setKlineMarkersLoading(true);
      setKlineMarkersError('');
      try {
        const rangeHours = Math.max(1, Math.ceil((klineMarkerRange.to - klineMarkerRange.from) / 3600) + 1);
        const markerParams = {
          apiBaseUrl,
          initData,
          chain: selectedPool?.chain || chain,
          poolId: selectedPoolAddress,
          bucketSec: klineIntervalMeta.bucketSec,
          startTs: klineMarkerRange.from,
          endTs: klineMarkerRange.to,
          signal,
        };
        const [smartResp, myResp] = await Promise.allSettled([
          fetchSmartMoneyPoolMarkers({
            ...markerParams,
            poolVersion: selectedPoolVersion,
            windowHours: rangeHours,
            limit: klineIntervalMeta.poolLimit,
          }),
          fetchMyTradeMarkers({
            ...markerParams,
            windowSec: Math.max(
              klineIntervalMeta.bucketSec,
              Number(klineMarkerRange.to || 0) - Number(klineMarkerRange.from || 0) + (klineIntervalMeta.bucketSec * 2)
            ),
          }),
        ]);

        const nextMarkers = [];
        let nextError = '';

        if (smartResp.status === 'fulfilled') {
          nextMarkers.push(...(Array.isArray(smartResp.value?.events) ? smartResp.value.events : []));
        } else {
          nextError = String(smartResp.reason?.message || smartResp.reason || '');
        }

        if (myResp.status === 'fulfilled') {
          const myEvents = Array.isArray(myResp.value?.events) ? myResp.value.events : [];
          nextMarkers.push(...myEvents.map((event, index) => ({
            ...event,
            event_id: String(event?.event_id || '').trim() || `my:${String(event?.action || 'add').toLowerCase()}:${Number(event?.t || event?.bucket_t || 0)}:${String(event?.tx_hash || '').toLowerCase() || index}`,
            is_my_trade: true,
          })));
        }

        nextMarkers.sort((a, b) => {
          const timeA = Number(a?.t || a?.bucket_t || 0);
          const timeB = Number(b?.t || b?.bucket_t || 0);
          if (timeA !== timeB) return timeA - timeB;
          return String(a?.action || '').localeCompare(String(b?.action || ''));
        });

        setKlineMarkers(nextMarkers);
        setKlineMarkersError(nextError);
      } catch (e) {
        if (e?.name !== 'AbortError') {
          setKlineMarkers([]);
          setKlineMarkersError(String(e?.message || e));
        }
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
      klineIntervalMeta.poolLimit,
      klineMarkerRange,
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
  }, [loadKlineMarkers, klineRefreshNonce]);

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
    setKlineDrawTool('none');
    setKlineDrawResetNonce((prev) => prev + 1);
    setKlineHeightSettingsOpen(false);
    setKlineMarkerFilterOpen(false);
    setKlineActiveMarkerId('');
    setKlineFocusedWalletAddress('');
  }, [klineViewportKey]);

  useEffect(() => {
    setKlineTokenSide('auto');
    setKlineSource('');
    setKlineDrawTool('none');
    setKlineDrawResetNonce((prev) => prev + 1);
    setKlineHeightSettingsOpen(false);
    setKlineMarkerFilterOpen(false);
    setKlineMarkers([]);
    setKlineMarkersError('');
    setKlineActiveMarkerId('');
    setKlineFocusedWalletAddress('');
  }, [selectedPoolAddress]);

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
    setKlineMarkers([]);
    setKlineMarkersError('');
    setKlineActiveMarkerId('');
    setKlineFocusedWalletAddress('');
  }, []);

  const refreshAll = useCallback(async () => {
    if (!hasInitData) return;
    setRefreshing(true);
    await Promise.allSettled([loadHotPools(), loadPositions()]);
    setKlineRefreshNonce((v) => v + 1);
    setRefreshing(false);
  }, [hasInitData, loadHotPools, loadPositions]);

  const refreshKline = useCallback(() => {
    setKlineRefreshNonce((v) => v + 1);
  }, []);

  const handleKlineMarkerClick = useCallback((cluster) => {
    if (!cluster?.id) {
      setKlineActiveMarkerId('');
      setKlineFocusedWalletAddress('');
      return;
    }
    const nextId = klineActiveMarkerId === cluster.id ? '' : cluster.id;
    setKlineActiveMarkerId(nextId);
    setKlineFocusedWalletAddress('');
  }, [klineActiveMarkerId]);

  const handleToggleKlineWatch = useCallback((walletAddress) => {
    const address = normalizeWalletAddress(walletAddress);
    if (!address) return;
    setKlineWatchToggleMap((prev) => ({ ...prev, [address]: true }));
    setKlineWatchedWallets((prev) => {
      const next = new Set(prev);
      if (next.has(address)) next.delete(address);
      else next.add(address);
      return Array.from(next).sort();
    });
    window.setTimeout(() => {
      setKlineWatchToggleMap((prev) => {
        if (!prev[address]) return prev;
        const next = { ...prev };
        delete next[address];
        return next;
      });
    }, 0);
  }, []);

  const handleSaveMarkerWalletLabel = useCallback(async (walletAddress, nextLabel) => {
    const address = normalizeWalletAddress(walletAddress);
    if (!address) throw new Error('钱包地址无效');
    const label = String(nextLabel || '').trim();
    await updateSMWallet({
      apiBaseUrl,
      address,
      updates: { label },
      chain: normalizeChain(selectedPool?.chain || chain),
    });
    setKlineMarkers((prev) => prev.map((marker) => (
      normalizeWalletAddress(marker?.wallet_address) === address
        ? { ...marker, wallet_label: label }
        : marker
    )));
  }, [apiBaseUrl, chain, selectedPool?.chain]);

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
  const [openPosSubmitError, setOpenPosSubmitError] = useState('');
  const [openPosRisk, setOpenPosRisk] = useState(null);
  const [openPosWallets, setOpenPosWallets] = useState(null);
  const [openPosWalletsLoading, setOpenPosWalletsLoading] = useState(false);
  const [openPosSmartRanges, setOpenPosSmartRanges] = useState([]);
  const [openPosSmartRangesLoading, setOpenPosSmartRangesLoading] = useState(false);
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

  const loadSmartRangesForModal = useCallback(async (poolAddress) => {
    const normalizedPoolAddress = normalizePoolAddress(poolAddress);
    if (!normalizedPoolAddress) {
      setOpenPosSmartRanges([]);
      setOpenPosSmartRangesLoading(false);
      return;
    }

    setOpenPosSmartRangesLoading(true);
    try {
      const resp = await fetchSMPoolStats({ apiBaseUrl, poolAddress: normalizedPoolAddress });
      const nextGroups = Array.isArray(resp?.range_groups) ? resp.range_groups : [];
      setOpenPosSmartRanges((prev) => (nextGroups.length > 0 ? nextGroups : prev));
    } catch {
      setOpenPosSmartRanges((prev) => prev);
    } finally {
      setOpenPosSmartRangesLoading(false);
    }
  }, [apiBaseUrl]);

  const openPositionModal = useCallback((pool) => {
    const resolvedChain = normalizeChain(pool?.chain || chain);
    const resolvedVersion = String(
      pool?.protocol_version || pool?.pool_version || inferPoolVersion(pool) || ''
    )
      .trim()
      .toLowerCase();
    const normalizedPoolAddress = normalizePoolAddress(pool?.pool_address || pool?.pool_id);

    setOpenPosSubmitError('');
    setOpenPosRisk(null);
    setOpenPosSmartRanges(Array.isArray(pool?.range_groups) ? pool.range_groups : []);
    setOpenPosSmartRangesLoading(Boolean(normalizedPoolAddress));
    setOpenPosPool({
      ...pool,
      chain: resolvedChain,
      ...(resolvedVersion ? { protocol_version: resolvedVersion, pool_version: resolvedVersion } : {}),
    });
    loadWalletsForModal(resolvedChain);
    loadSmartRangesForModal(normalizedPoolAddress);
  }, [chain, loadSmartRangesForModal, loadWalletsForModal]);

  const handleOpenPosition = useCallback(async (params) => {
    const panelKey = openPosPool?.panelKey || 'hot_pools';
    setOpenPosBusy(true);
    setOpenPosSubmitError('');
    setOperationProgress({
      panelKey,
      operation: 'open_position',
      currentStep: 1,
      totalSteps: 4,
      status: 'active',
      error: '',
    });
    try {
      await apiOpenPosition({ apiBaseUrl, initData, ...params });
      setOperationProgress(prev => prev?.operation === 'open_position'
        ? { ...prev, currentStep: 4, status: 'done' } : prev);
      setOpenPosSubmitError('');
      setOpenPosRisk(null);
      setOpenPosSmartRanges([]);
      setOpenPosSmartRangesLoading(false);
      setOpenPosPool(null);
      loadPositions();
    } catch (e) {
      const msg = String(e?.message || e);
      const risk = e && typeof e === 'object' && (
        typeof e?.liquidity_usd === 'number' ||
        typeof e?.max_open_amount === 'number' ||
        Boolean(e?.risk_ack_required) ||
        typeof e?.price_deviation_percent === 'number'
      )
        ? {
          code: String(e?.code || ''),
          message: msg,
          liquidity_usd: Number(e?.liquidity_usd),
          min_liquidity_usd: Number(e?.min_liquidity_usd),
          max_open_amount: Number(e?.max_open_amount),
          risk_ack_required: Boolean(e?.risk_ack_required),
          price_deviation_percent: Number(e?.price_deviation_percent),
          price_deviation_max_percent: Number(e?.price_deviation_max_percent),
        }
        : null;
      setOperationProgress((prev) => (prev?.operation === 'open_position' ? null : prev));
      setOpenPosRisk(risk);
      setOpenPosSubmitError(risk ? '' : msg);
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
  const panelMap = {
    create_pool: (
      <CreatePoolPanel
        apiBaseUrl={apiBaseUrl}
        initData={initData}
        hasInitData={hasInitData}
      />
    ),
    hot_pools: (
      <PanelShell
        title="热门池子"
        subtitle={`支持搜索与排序 · 展示前 ${HOT_POOLS_DISPLAY_LIMIT} 条`}
        icon={Flame}
        actions={!workMode ? (
          <div className="settings-wrap" ref={hotPoolsHeightControlRef}>
            <button
              type="button"
              className={`icon-link ${hotPoolsHeightSettingsOpen || hotPoolsPanelHeightCustomized ? 'active' : ''}`}
              onClick={() => setHotPoolsHeightSettingsOpen((prev) => !prev)}
              title={`热门池子高度 ${hotPoolsPanelHeight}px`}
              aria-label="热门池子高度"
            >
              <Settings size={14} />
            </button>

            {hotPoolsHeightSettingsOpen ? (
              <div className="kline-settings-popover panel-height-popover">
                <div className="kline-filter-popover-head">
                  <div>
                    <div className="kline-filter-popover-title">热门池子高度</div>
                    <div className="kline-filter-popover-sub">仅保存在当前浏览器</div>
                  </div>
                  <button
                    type="button"
                    className="icon-link"
                    onClick={() => setHotPoolsHeightSettingsOpen(false)}
                    title="Close"
                  >
                    <X size={14} />
                  </button>
                </div>

                <div className="kline-height-value">{hotPoolsPanelHeight}px</div>

                <input
                  className="kline-height-slider"
                  type="range"
                  min={MIN_HOT_POOLS_PANEL_HEIGHT}
                  max={MAX_HOT_POOLS_PANEL_HEIGHT}
                  step="20"
                  value={hotPoolsPanelHeight}
                  onChange={(e) => setHotPoolsPanelHeight(clampHotPoolsPanelHeight(e.target.value, hotPoolsDefaultHeightRef.current))}
                />

                <label className="kline-filter-field">
                  <span>高度</span>
                  <div className="kline-height-input-row">
                    <input
                      type="number"
                      min={MIN_HOT_POOLS_PANEL_HEIGHT}
                      max={MAX_HOT_POOLS_PANEL_HEIGHT}
                      step="20"
                      inputMode="numeric"
                      value={hotPoolsPanelHeight}
                      onChange={(e) => {
                        const nextValue = Number(e.target.value);
                        if (!Number.isFinite(nextValue)) return;
                        setHotPoolsPanelHeight(clampHotPoolsPanelHeight(nextValue, hotPoolsDefaultHeightRef.current));
                      }}
                    />
                    <span className="kline-height-unit">px</span>
                  </div>
                </label>

                <div className="kline-filter-actions">
                  <button type="button" className="ghost-chip" onClick={resetHotPoolsPanelHeight}>
                    默认
                  </button>
                </div>
              </div>
            ) : null}
          </div>
        ) : null}
      >
        <div className="hot-pools-toolbar-shell" ref={hotPoolsFilterRef}>
          <div className="sort-tabs">
            {[{ key: 'fees', label: 'Fees' }, { key: 'fee_rate', label: 'Fee Rate' }, { key: 'volume', label: 'Volume' }].map((item) => (
              <button
                type="button"
                key={item.key}
                className={`sort-tab ${hotSort === item.key ? 'active' : ''}`}
                onClick={() => {
                  setHotSort(item.key);
                  setHotInlineSort('');
                }}
              >
                {item.label}
              </button>
            ))}
            <button
              type="button"
              className={`sort-tab icon-only search-toggle ${searchOpen ? 'active' : ''}`}
              onClick={() => {
                setHotPoolsFilterOpen(false);
                setSearchOpen((v) => !v);
              }}
              title="搜索池子"
              aria-label="搜索池子"
            >
              <Search size={12} />
            </button>
            <button
              type="button"
              className={`sort-tab icon-only filter-toggle ${hotPoolsFilterEnabled ? 'active' : ''}`}
              onClick={() => {
                if (hotPoolsFilterOpen) {
                  setHotPoolsFilterOpen(false);
                  return;
                }
                openHotPoolsFilter();
              }}
              title="筛选池子"
              aria-label="筛选池子"
            >
              <SlidersHorizontal size={12} />
              {hotPoolsFilterEnabled ? <span className="hot-filter-dot" /> : null}
            </button>
          </div>

          {hotPoolsFilterOpen ? (
            <div className="kline-filter-popover hot-pools-filter-popover">
              <div className="kline-filter-popover-head">
                <div>
                  <div className="kline-filter-popover-title">热门池子筛选</div>
                  <div className="kline-filter-popover-sub">仅筛选当前已加载的热门池子</div>
                </div>
                <button
                  type="button"
                  className="icon-link"
                  onClick={() => setHotPoolsFilterOpen(false)}
                  title="Close"
                >
                  <X size={14} />
                </button>
              </div>

              <div className="hot-pools-filter-toggle-row">
                <div className="hot-pools-filter-toggle-copy">
                  <span className="hot-pools-filter-toggle-label">筛选状态</span>
                  <span className="hot-pools-filter-toggle-state">
                    {hotPoolsFilterDraft.enabled ? '已启用，应用后按下方条件筛选' : '已关闭，条件会保留但不会生效'}
                  </span>
                </div>
                <button
                  type="button"
                  className={`ghost-chip ${hotPoolsFilterDraft.enabled ? 'active' : ''}`}
                  onClick={() => setHotPoolsFilterDraft((prev) => ({ ...prev, enabled: !prev.enabled }))}
                  aria-pressed={hotPoolsFilterDraft.enabled}
                  title={hotPoolsFilterDraft.enabled ? '关闭筛选' : '启用筛选'}
                >
                  {hotPoolsFilterDraft.enabled ? '已启用' : '已关闭'}
                </button>
              </div>

              <label className="kline-filter-field">
                <span>搜索 (交易对 / 地址)</span>
                <input
                  value={hotPoolsFilterDraft.keyword}
                  onChange={(e) => setHotPoolsFilterDraft((prev) => ({ ...prev, keyword: e.target.value }))}
                  placeholder="例如 USDT"
                />
              </label>

              <div className="hot-pools-filter-grid">
                <label className="kline-filter-field">
                  <span>手续费 ≥ (USD)</span>
                  <input
                    value={hotPoolsFilterDraft.minFees}
                    onChange={(e) => setHotPoolsFilterDraft((prev) => ({ ...prev, minFees: e.target.value }))}
                    inputMode="decimal"
                    placeholder={String(HOT_POOLS_FILTER_DEFAULTS.minFees)}
                  />
                </label>

                <label className="kline-filter-field">
                  <span>费用率 ≥ (%)</span>
                  <input
                    value={hotPoolsFilterDraft.minFeeRate}
                    onChange={(e) => setHotPoolsFilterDraft((prev) => ({ ...prev, minFeeRate: e.target.value }))}
                    inputMode="decimal"
                    placeholder={String(HOT_POOLS_FILTER_DEFAULTS.minFeeRate)}
                  />
                </label>

                <label className="kline-filter-field">
                  <span>活跃费率 ≥ (%)</span>
                  <input
                    value={hotPoolsFilterDraft.minActiveFeeRate}
                    onChange={(e) => setHotPoolsFilterDraft((prev) => ({ ...prev, minActiveFeeRate: e.target.value }))}
                    inputMode="decimal"
                    placeholder="可选"
                  />
                </label>

                <label className="kline-filter-field">
                  <span>TVL ≥ (USD)</span>
                  <input
                    value={hotPoolsFilterDraft.minTvl}
                    onChange={(e) => setHotPoolsFilterDraft((prev) => ({ ...prev, minTvl: e.target.value }))}
                    inputMode="decimal"
                    placeholder={String(HOT_POOLS_FILTER_DEFAULTS.minTvl)}
                  />
                </label>

                <label className="kline-filter-field">
                  <span>交易量 ≥ (USD)</span>
                  <input
                    value={hotPoolsFilterDraft.minVolume}
                    onChange={(e) => setHotPoolsFilterDraft((prev) => ({ ...prev, minVolume: e.target.value }))}
                    inputMode="decimal"
                    placeholder={String(HOT_POOLS_FILTER_DEFAULTS.minVolume)}
                  />
                </label>

                <label className="kline-filter-field">
                  <span>交易笔数 ≥</span>
                  <input
                    value={hotPoolsFilterDraft.minTxCount}
                    onChange={(e) => setHotPoolsFilterDraft((prev) => ({ ...prev, minTxCount: e.target.value }))}
                    inputMode="decimal"
                    placeholder="可选"
                  />
                </label>
              </div>

              <div className="kline-filter-actions">
                <button type="button" className="ghost-chip active" onClick={applyHotPoolsFilter}>
                  应用
                </button>
                <button type="button" className="ghost-chip" onClick={resetHotPoolsFilter}>
                  默认
                </button>
                <button type="button" className="ghost-chip" onClick={clearHotPoolsFilter}>
                  清空条件
                </button>
              </div>
            </div>
          ) : null}
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
        {hotPoolsFilterEnabled && !hotPoolsLoading && hotPools.length > 0 && filteredHotPools.length === 0 ? (
          <div className="hot-filter-empty-note">
            当前筛选条件下没有可展示的热门池子，调整筛选或清空条件后再试。
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
              const activeLiquidityFeeRate = computeHotPoolActiveFeeRate(pool);
              const txCount = Number(pool?.transaction_count || 0);
              const priceDisplay = String(pool?.price_display || '');
              const feeRateAvailable = Number.isFinite(tvl) && tvl > 0 && Number.isFinite(feeRate);
              const feeRateText = feeRateAvailable ? `${feeRate.toFixed(3)}%` : '--';
              const activeLiquidityFeeRateAvailable = Number.isFinite(activeLiquidityFeeRate);
              const activeLiquidityFeeRateText = activeLiquidityFeeRateAvailable ? `${activeLiquidityFeeRate.toFixed(3)}%` : '--';
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
              const avatarSrc = displayTokenLogoUrl;
              const filterToken = resolveHotPoolFilterToken(pool);
              const avatarFilterActive = filterToken && hotTokenFilter?.address === filterToken.address;
              const badges = parseHotPoolBadges(pool?.badges);

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
                          onError={(e) => {
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
                    {badges.length > 0 && (
                      <div className="pool-badge-line">
                        {badges.map((badge, badgeIdx) => (
                          <span
                            key={`${badge.text}:${badgeIdx}`}
                            className="tag tag-badge pool-badge-chip"
                            data-tip={badge.tip}
                            aria-label={badge.tip}
                            tabIndex={0}
                          >
                            <span>{badge.text}</span>
                          </span>
                        ))}
                      </div>
                    )}
                    <div className="pool-meta-line">
                      <button
                        type="button"
                        className={`pool-meta-sort meta-cyan ${hotInlineSort === 'volume' ? 'active' : ''}`}
                        onClick={(e) => {
                          e.stopPropagation();
                          setHotInlineSort((prev) => (prev === 'volume' ? '' : 'volume'));
                        }}
                        title="按 Vol 降序排序"
                      >
                        <span>Vol</span>
                        <b><NumberFlowValue value={volume} formatter={(v) => formatUsdCompact(v)} /></b>
                      </button>
                      <span className="dot-sep" />
                      <button
                        type="button"
                        className={`pool-meta-sort meta-cyan ${hotInlineSort === 'tvl' ? 'active' : ''}`}
                        onClick={(e) => {
                          e.stopPropagation();
                          setHotInlineSort((prev) => (prev === 'tvl' ? '' : 'tvl'));
                        }}
                        title="按 TVL 降序排序"
                      >
                        <span>TVL</span>
                        <b><NumberFlowValue value={tvl} formatter={(v) => formatUsdCompact(v)} /></b>
                      </button>
                      <span className="dot-sep" />
                      <button
                        type="button"
                        className={`pool-meta-sort meta-orange ${hotInlineSort === 'tx_count' ? 'active' : ''}`}
                        onClick={(e) => {
                          e.stopPropagation();
                          setHotInlineSort((prev) => (prev === 'tx_count' ? '' : 'tx_count'));
                        }}
                        title="按笔数降序排序"
                      >
                        <b><NumberFlowValue value={txCount} formatter={(v) => `${Number(v || 0).toLocaleString()}笔`} /></b>
                      </button>
                      <span className="dot-sep" />
                      <button
                        type="button"
                        className={`pool-meta-sort meta-accent ${hotInlineSort === 'fee_rate' ? 'active' : ''} ${feeRateAvailable ? '' : 'muted'}`}
                        onClick={(e) => {
                          e.stopPropagation();
                          setHotInlineSort((prev) => (prev === 'fee_rate' ? '' : 'fee_rate'));
                        }}
                        title="按费率降序排序"
                      >
                        <span>费率</span>
                        <b>
                          {feeRateAvailable ? (
                            <NumberFlowValue value={feeRate} formatter={(v) => `${Number(v).toFixed(3)}%`} />
                          ) : '--'}
                        </b>
                      </button>
                      <span className="dot-sep" />
                      <button
                        type="button"
                        className={`pool-meta-sort meta-gold ${hotInlineSort === 'active_fee_rate' ? 'active' : ''} ${activeLiquidityFeeRateAvailable ? '' : 'muted'}`}
                        onClick={(e) => {
                          e.stopPropagation();
                          setHotInlineSort((prev) => (prev === 'active_fee_rate' ? '' : 'active_fee_rate'));
                        }}
                        title="按活跃降序排序"
                      >
                        <span>活跃</span>
                        <b>
                          {activeLiquidityFeeRateAvailable ? (
                            <NumberFlowValue value={activeLiquidityFeeRate} formatter={() => activeLiquidityFeeRateText} />
                          ) : activeLiquidityFeeRateText}
                        </b>
                      </button>
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
                    ) : hotSort !== 'fee_rate' ? (
                      <div className={`pool-sub-val purple ${feeRateAvailable ? '' : 'muted'}`}>
                        {feeRateAvailable ? (
                          <NumberFlowValue value={feeRate} formatter={() => feeRateText} />
                        ) : feeRateText}
                      </div>
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
                    <span className="open-buy-text">开仓</span>
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
                  className="ghost-chip"
                  onClick={refreshKline}
                  disabled={klineLoading}
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
                <span className="label">气泡</span>
                <span className="value">{klineFilteredMarkers.length}/{klineMarkers.length}</span>
              </div>
              <div className="kline-summary-item">
                <span className="label">筛选</span>
                <span className="value">{klineFilterActive ? '已启用' : '全部'}</span>
              </div>
            </div>

            <div className="kline-chart-shell">
              <div className="kline-tool-dock" ref={klineToolDockRef}>
                {KLINE_DRAW_TOOLS.map((tool) => {
                  const Icon = tool.icon;
                  return (
                    <button
                      key={tool.key}
                      type="button"
                      className={`kline-tool-btn ${klineDrawTool === tool.key ? 'active' : ''}`}
                      onClick={() => setKlineDrawTool(tool.key)}
                      title={tool.title}
                      aria-label={tool.title}
                    >
                      <Icon size={16} />
                    </button>
                  );
                })}

                <div className="kline-filter-shell">
                  <button
                    type="button"
                    className={`kline-tool-btn ${klineMarkerFilterOpen || klineFilterActive ? 'active' : ''}`}
                    onClick={() => {
                      setKlineHeightSettingsOpen(false);
                      setKlineMarkerFilterOpen((prev) => !prev);
                    }}
                    title="气泡筛选"
                    aria-label="气泡筛选"
                  >
                    <SlidersHorizontal size={16} />
                  </button>

                  {klineMarkerFilterOpen ? (
                    <div className="kline-filter-popover tool-dock">
                      <div className="kline-filter-popover-head">
                        <div>
                          <div className="kline-filter-popover-title">气泡筛选</div>
                          <div className="kline-filter-popover-sub">仅筛选当前已加载的气泡</div>
                        </div>
                        <button
                          type="button"
                          className="icon-link"
                          onClick={() => setKlineMarkerFilterOpen(false)}
                          title="Close"
                        >
                          <X size={14} />
                        </button>
                      </div>

                      <label className="kline-filter-field">
                        <span>最低金额</span>
                        <input
                          type="number"
                          min="0"
                          step="100"
                          inputMode="decimal"
                          placeholder="0 = 全部"
                          value={klineMarkerMinUsdInput}
                          onChange={(e) => setKlineMarkerMinUsdInput(e.target.value)}
                        />
                      </label>

                      <div className="kline-filter-actions">
                        <button
                          type="button"
                          className="ghost-chip"
                          onClick={() => setKlineMarkerWalletSelection(klineMarkerWalletOptions.map((item) => item.address))}
                          disabled={!klineMarkerWalletOptions.length || klineAllWalletsSelected}
                        >
                          全选
                        </button>
                        <button
                          type="button"
                          className="ghost-chip"
                          onClick={() => setKlineMarkerWalletSelection([])}
                          disabled={!klineMarkerWalletSelection.length}
                        >
                          清空钱包
                        </button>
                        <button
                          type="button"
                          className="ghost-chip"
                          onClick={() => {
                            setKlineMarkerMinUsdInput('');
                            setKlineMarkerWalletSelection([]);
                          }}
                          disabled={!klineFilterActive}
                        >
                          重置
                        </button>
                      </div>

                      {klineMarkerWalletOptions.length ? (
                        <div className="kline-filter-wallets">
                          {klineMarkerWalletOptions.map((wallet) => {
                            const checked = klineMarkerWalletSelectionSet.has(wallet.address);
                            return (
                              <label key={wallet.address} className="kline-filter-wallet-option">
                                <input
                                  type="checkbox"
                                  checked={checked}
                                  onChange={() => {
                                    setKlineMarkerWalletSelection((prev) => {
                                      const next = new Set(prev);
                                      if (next.has(wallet.address)) next.delete(wallet.address);
                                      else next.add(wallet.address);
                                      return Array.from(next).sort();
                                    });
                                  }}
                                />
                                <span className="kline-filter-wallet-main">
                                  <span className="kline-filter-wallet-name">{wallet.label}</span>
                                  <span className="kline-filter-wallet-meta">
                                    {shortAddress(wallet.address, 6, 4)} · {wallet.markerCount} 个气泡 · 最高 {formatUsdCompact(wallet.maxEstimatedUsd)}
                                  </span>
                                </span>
                              </label>
                            );
                          })}
                        </div>
                      ) : (
                        <div className="kline-filter-empty">当前窗口没有可筛选的钱包气泡</div>
                      )}

                      <div className="kline-filter-popover-sub">
                        显示 {klineFilteredMarkers.length} / {klineMarkers.length} 个气泡
                        {klineFilteredOutCount > 0 ? `，已隐藏 ${klineFilteredOutCount} 个` : ''}
                      </div>
                    </div>
                  ) : null}
                </div>

                <button
                  type="button"
                  className={`kline-tool-btn ${klineHeightSettingsOpen || klineChartHeightCustomized ? 'active' : ''}`}
                  onClick={() => {
                    setKlineMarkerFilterOpen(false);
                    setKlineHeightSettingsOpen((prev) => !prev);
                  }}
                  title="图表高度"
                  aria-label="图表高度"
                >
                  <Settings size={16} />
                </button>

                <button
                  type="button"
                  className="kline-tool-btn"
                  onClick={clearKlineDrawing}
                  disabled={!klineCandles.length}
                  title="Clear"
                  aria-label="Clear"
                >
                  <X size={16} />
                </button>

                {klineHeightSettingsOpen ? (
                  <div className="kline-settings-popover tool-dock">
                    <div className="kline-filter-popover-head">
                      <div>
                        <div className="kline-filter-popover-title">图表高度</div>
                        <div className="kline-filter-popover-sub">仅保存在当前浏览器</div>
                      </div>
                      <button
                        type="button"
                        className="icon-link"
                        onClick={() => setKlineHeightSettingsOpen(false)}
                        title="Close"
                      >
                        <X size={14} />
                      </button>
                    </div>

                    <div className="kline-height-value">{klineChartHeight}px</div>

                    <input
                      className="kline-height-slider"
                      type="range"
                      min={MIN_KLINE_CHART_HEIGHT}
                      max={MAX_KLINE_CHART_HEIGHT}
                      step="20"
                      value={klineChartHeight}
                      onChange={(e) => setKlineChartHeight(clampKlineChartHeight(e.target.value))}
                    />

                    <label className="kline-filter-field">
                      <span>高度</span>
                      <div className="kline-height-input-row">
                        <input
                          type="number"
                          min={MIN_KLINE_CHART_HEIGHT}
                          max={MAX_KLINE_CHART_HEIGHT}
                          step="20"
                          inputMode="numeric"
                          value={klineChartHeight}
                          onChange={(e) => {
                            const nextValue = Number(e.target.value);
                            if (!Number.isFinite(nextValue)) return;
                            setKlineChartHeight(clampKlineChartHeight(nextValue));
                          }}
                        />
                        <span className="kline-height-unit">px</span>
                      </div>
                    </label>

                    <div className="kline-filter-actions">
                      <button type="button" className="ghost-chip" onClick={resetKlineChartHeight}>
                        默认
                      </button>
                    </div>
                  </div>
                ) : null}
              </div>

              <KlineChart
                candles={klineCandles}
                markers={klineFilteredMarkers}
                rangeOverlays={klineRangeOverlays}
                loading={klineLoading}
                error={klineError}
                viewportKey={klineViewportKey}
                activeMarkerId={klineActiveMarkerId}
                highlightWalletAddress={klineFocusedWalletAddress}
                watchedWalletSet={klineWatchedWalletSet}
                watchToggleMap={klineWatchToggleMap}
                onMarkerClick={handleKlineMarkerClick}
                onToggleWatch={handleToggleKlineWatch}
                onSaveWalletLabel={handleSaveMarkerWalletLabel}
                drawingTool={klineDrawTool}
                drawingResetNonce={klineDrawResetNonce}
                chartHeight={klineChartHeight}
                userAvatarUrl={loginUser?.photo_url || ''}
              />
            </div>

            {klineMarkersError ? (
              <div className="kline-inline-note">
                聪明钱气泡加载失败：{klineMarkersError}
              </div>
            ) : null}
            {!klineMarkersError && !klineMarkersLoading && selectedPoolAddress && klineCandles.length > 0 && klineMarkers.length === 0 ? (
              <div className="kline-inline-note">
                当前时间窗口没有聪明钱开仓/撤仓气泡。
              </div>
            ) : null}
            {!klineMarkersError && !klineMarkersLoading && klineMarkers.length > 0 && klineFilteredMarkers.length === 0 ? (
              <div className="kline-inline-note">
                当前筛选条件下没有可展示的气泡，点左侧筛选按钮可调整或重置。
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

        {(() => {
          const warnings = Array.from(
            new Set(
              (Array.isArray(positions?.warnings) ? positions.warnings : [])
                .map((item) => String(item || '').trim())
                .filter(Boolean),
            ),
          );
          if (warnings.length === 0) return null;
          return (
            <div className="mt-3">
              {warnings.map((warning, index) => (
                <div key={`${warning}-${index}`} className="warning-box">
                  <AlertTriangle size={14} />
                  <span>{warning}</span>
                </div>
              ))}
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
              const positionWalletMeta = walletMetaByKey.get(`id:${Number(p?.wallet_id || 0)}`) ||
                walletMetaByKey.get(`addr:${normalizeWalletAddress(p?.wallet_address)}`);
              const positionWalletText = positionWalletMeta?.label ||
                shortAddress(normalizeWalletAddress(p?.wallet_address) || '', 6, 4) ||
                '默认钱包';

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
                        <span className="pos-wallet-chip">钱包 {positionWalletText}</span>
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

    assets: (
      <Suspense fallback={(
        <PanelShell title="资产管理" subtitle="正在加载模块" icon={BriefcaseBusiness}>
          <EmptyState text="正在加载资产管理模块..." />
        </PanelShell>
      )}>
        <LazyAssetManagementPanel
          apiBaseUrl={apiBaseUrl}
          initData={initData}
          hasInitData={hasInitData}
          isAdmin={Boolean(positions?.is_admin)}
          refreshInterval={refreshInterval}
        />
      </Suspense>
    ),

    smart_money: (
      <SmartMoneyDashboard
        apiBaseUrl={apiBaseUrl}
        initData={initData}
        onSelectPool={selectPool}
        activePoolAddress={selectedPoolAddress}
        refreshInterval={refreshInterval}
        onOpenPosition={(pool) => openPositionModal({
          ...pool,
          chain: String(pool?.chain || (Number(pool?.chain_id) === 8453 ? 'base' : chain)).toLowerCase(),
          panelKey: 'smart_money',
        })}
      />
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
              <h1>
                <img src={siteLogo} alt="lynchL" className="title-logo" />
              </h1>
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
            style={widget.key === 'hot_pools' ? { '--hot-pools-panel-height': `${hotPoolsPanelHeight}px` } : undefined}
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
          apiBaseUrl={apiBaseUrl}
          initData={initData}
          pool={openPosPool}
          chain={openPosPool?.chain || chain}
          wallets={openPosWallets}
          walletsLoading={openPosWalletsLoading}
          smartRanges={openPosSmartRanges}
          smartRangesLoading={openPosSmartRangesLoading}
          selectedWalletId={openPosWalletId}
          submitError={openPosSubmitError}
          submitRisk={openPosRisk}
          onClearSubmitError={() => {
            setOpenPosSubmitError('');
            setOpenPosRisk(null);
          }}
          onWalletSelect={(id) => {
            setOpenPosSubmitError('');
            setOpenPosWalletId(id);
            storageSet(STORAGE.walletId, String(id));
          }}
          onSubmit={handleOpenPosition}
          onClose={() => {
            setOpenPosSubmitError('');
            setOpenPosRisk(null);
            setOpenPosSmartRanges([]);
            setOpenPosSmartRangesLoading(false);
            setOpenPosPool(null);
          }}
          busy={openPosBusy}
        />
      )}
    </div>
  );
}
