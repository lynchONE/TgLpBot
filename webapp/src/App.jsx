import React, { Suspense, lazy, useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
  BriefcaseBusiness,
  CandlestickChart,
  MousePointer2,
  RefreshCw,
  Shield,
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
  fetchMe,
  fetchMyTradeMarkers,
  fetchNewsFeed,
  fetchRealtimePositions,
  fetchSmartMoneyPoolMarkers,
  fetchTokenCandles,
  fetchWallets,
  generateLoginCode,
  openPosition as apiOpenPosition,
  setTaskPaused,
  stopTask,
  updateTaskRange,
  withdrawLiquidity,
  swapDust,
  triggerRebalance,
  updateTaskMode,
  addLiquidity,
} from './api';
import { WEBAPP_CONFIG } from './config';
import PanelShell, { EmptyState } from './components/PanelShell';
import KlineChart from './components/KlineChart';
import CreatePoolPanel from './components/CreatePoolPanel';
import SmartMoneyDashboard from './components/SmartMoneyDashboard';
import OpenPositionModal from './components/OpenPositionModal';
import StepProgressModal from './components/StepProgressModal';
import AddLiquidityModal from './components/AddLiquidityModal';
import ConfirmDialog from './components/ConfirmDialog';
import GlobalConfigPanel from './components/GlobalConfigPanel';
import WalletManagePanel from './components/WalletManagePanel';
import SwapPanel from './components/SwapPanel';
import TradeHistoryPanel from './components/TradeHistoryPanel';
import GuestHotPoolsLanding from './components/GuestHotPoolsLanding';
import { TopBar, WorkbenchConfigPanel, WorkModeBar } from './components/WorkbenchChrome';
import { NewsShowcase, NewsTicker } from './components/NewsPanels';
import WorkbenchLayout from './components/WorkbenchLayout';
import PositionsPanel, { normalizePositionSmartMoneyGroups } from './components/PositionsPanel';
import HotPoolsPanel from './components/HotPoolsPanel';
import {
  Button,
  IconButton,
  Input,
  Slider,
} from './components/ui';
import {
  fetchSMPoolStats,
  fetchSMWatchWallets,
  saveSMWatchWallets,
  updateSMWallet,
} from './smartMoneyApi';
import uniswapLogo from './img/uniswap.svg';
import pancakeLogo from './img/pancake.svg';
import {
  DEFAULT_WIDGETS,
  WIDGETS,
  buildGmgnUrl,
  canAccessWidget,
  formatPct,
  formatUsd,
  formatUsdCompact,
  computeHotPoolActiveFeeRate,
  normalizePoolAddress,
  normalizeHexAddress,
  normalizeAccessInfo,
  normalizeWidgetSelection,
  pickNonStableTokenAddress,
  resolveKlineTokenOptions,
  normalizeTokenRisk,
  shortAddress,
  inferPoolVersion,
} from './utils';

const LazyAssetManagementPanel = lazy(() => import('./components/AssetManagementPanel'));
const LazyAdminPanel = lazy(() => import('./components/AdminPanel'));

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
const POSITION_SM_RANGE_STALE_MS = 60_000;
const POSITION_SM_RANGE_BATCH_SIZE = 8;
const NEWS_TICKER_MIN_SPEED = 2;
const NEWS_TICKER_MAX_SPEED = 80;
const NEWS_TICKER_DEFAULT_SPEED = 8;

function normalizeNewsTickerSpeed(value) {
  const n = Number(value);
  if (!Number.isFinite(n)) return NEWS_TICKER_DEFAULT_SPEED;
  return Math.min(NEWS_TICKER_MAX_SPEED, Math.max(NEWS_TICKER_MIN_SPEED, Math.round(n)));
}

function normalizeNewsTextKey(value, maxLength) {
  const key = String(value || '')
    .replace(/<[^>]*>/g, '')
    .toLowerCase()
    .replace(/[\s\p{P}\p{S}]+/gu, '')
    .trim();
  if (!key) return '';
  return maxLength && key.length > maxLength ? key.slice(0, maxLength) : key;
}

function normalizeNewsUrlKey(value) {
  const raw = String(value || '').trim();
  if (!raw) return '';
  try {
    const url = new URL(raw);
    url.hash = '';
    ['utm_source', 'utm_medium', 'utm_campaign', 'utm_term', 'utm_content', 'from', 'ref'].forEach((key) => {
      url.searchParams.delete(key);
    });
    return url.toString().toLowerCase();
  } catch {
    return '';
  }
}

function dedupeNewsItems(items, limit) {
  const rows = Array.isArray(items) ? items : [];
  const seen = new Set();
  const out = [];
  rows.forEach((item) => {
    if (!item?.title) return;
    const keys = [
      `title:${normalizeNewsTextKey(item.title, 160)}`,
      `content:${normalizeNewsTextKey(item.content, 220)}`,
      `url:${normalizeNewsUrlKey(item.source_link)}`,
    ].filter((key) => !key.endsWith(':'));
    if (keys.some((key) => seen.has(key))) return;
    keys.forEach((key) => seen.add(key));
    out.push(item);
  });
  return Number.isFinite(limit) && limit > 0 ? out.slice(0, limit) : out;
}

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

function parsePositionCreatedTime(position) {
  const raw = String(position?.running_since || position?.created_at || '').trim();
  if (!raw) return null;
  const ts = Date.parse(raw);
  return Number.isFinite(ts) ? ts : null;
}

function comparePositionsByCreatedAt(a, b) {
  const aTime = parsePositionCreatedTime(a);
  const bTime = parsePositionCreatedTime(b);
  if (aTime !== null && bTime !== null && aTime !== bTime) return aTime - bTime;
  if (aTime !== null && bTime === null) return -1;
  if (aTime === null && bTime !== null) return 1;

  const aTaskId = Number(a?.task_id || 0);
  const bTaskId = Number(b?.task_id || 0);
  if (aTaskId !== bTaskId) return aTaskId - bTaskId;

  const aKey = [
    String(a?.title || ''),
    String(a?.pool_id || a?.pool_address || '').toLowerCase(),
    String(a?.position_id || ''),
    String(a?.version || ''),
    String(a?.exchange || '').toLowerCase(),
  ].join(':');
  const bKey = [
    String(b?.title || ''),
    String(b?.pool_id || b?.pool_address || '').toLowerCase(),
    String(b?.position_id || ''),
    String(b?.version || ''),
    String(b?.exchange || '').toLowerCase(),
  ].join(':');
  return aKey.localeCompare(bKey, undefined, { numeric: true });
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
  loginAccess: 'tglp_web_login_access',
  chain: 'tglp_web_chain',
  widgets: 'tglp_web_widgets',
  sort: 'tglp_web_hot_pools_sort',
  refreshInterval: 'tglp_web_refresh_interval',
  refreshIntervals: 'tglp_web_refresh_intervals_v1',
  accentTheme: 'tglp_web_accent_theme',
  walletId: 'tglp_web_wallet_id',
  klineHeight: 'tglp_web_kline_height',
  hotPoolsHeight: 'tglp_web_hot_pools_height',
  hotPoolsFilter: 'tglp_web_hot_pools_filter_v1',
  smartMoneyWatchWallets: 'tglp_web_sm_watch_wallets',
  newsTickerSpeed: 'tglp_web_news_ticker_speed',
};

const MIN_REFRESH_INTERVAL_SEC = 2;
const MAX_REFRESH_INTERVAL_SEC = 300;
const POSITIONS_ACTIVE_REFRESH_KEY = 'positions_active';
const POSITIONS_IDLE_REFRESH_KEY = 'positions_idle';
const LEGACY_POSITIONS_REFRESH_KEY = 'positions';
const KLINE_POSITION_REFRESH_KEY = 'kline_position';
const KLINE_IDLE_REFRESH_KEY = 'kline_idle';
const LEGACY_KLINE_REFRESH_KEY = 'gmgn_kline';
const REFRESH_MODULE_CONFIG = [
  { key: 'hot_pools', label: '热门池子', defaultSec: 10, minSec: 2 },
  { key: POSITIONS_ACTIVE_REFRESH_KEY, label: '仓位(有仓位)', defaultSec: 10, minSec: 2 },
  { key: POSITIONS_IDLE_REFRESH_KEY, label: '仓位(无仓位)', defaultSec: 30, minSec: 5 },
  { key: KLINE_POSITION_REFRESH_KEY, label: 'K线(有对应仓位)', defaultSec: 20, minSec: 5 },
  { key: KLINE_IDLE_REFRESH_KEY, label: 'K线(无对应仓位)', defaultSec: 45, minSec: 10 },
  { key: 'assets', label: '我的资产', defaultSec: 60, minSec: 60 },
  { key: 'smart_money', label: '聪明钱', defaultSec: 10, minSec: 2 },
  { key: 'admin_panel', label: '管理面板', defaultSec: 10, minSec: 2 },
];

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

function getRefreshModuleConfig(key) {
  const config = REFRESH_MODULE_CONFIG.find((item) => item.key === key);
  if (!config) {
    throw new Error(`Unknown refresh module: ${key}`);
  }
  return config;
}

function clampRefreshInterval(value, config) {
  if (!config || !Number.isFinite(Number(config.minSec)) || !Number.isFinite(Number(config.defaultSec))) {
    throw new Error('Invalid refresh module config');
  }
  const n = Number(value);
  const minSec = Math.max(MIN_REFRESH_INTERVAL_SEC, Number(config.minSec));
  const defaultSec = Math.max(minSec, Number(config.defaultSec));
  if (!Number.isFinite(n)) return defaultSec;
  return Math.max(minSec, Math.min(MAX_REFRESH_INTERVAL_SEC, Math.round(n)));
}

function buildDefaultRefreshIntervals() {
  return Object.fromEntries(
    REFRESH_MODULE_CONFIG.map((item) => [item.key, clampRefreshInterval(item.defaultSec, item)])
  );
}

function normalizeRefreshIntervals(raw, legacyValue) {
  let parsed = null;
  if (raw) {
    try {
      parsed = JSON.parse(raw);
    } catch {
      parsed = null;
    }
  }
  const legacy = Number(legacyValue);
  const hasLegacy = Number.isFinite(legacy) && legacy >= MIN_REFRESH_INTERVAL_SEC;
  const legacyPositionsValue = parsed && Object.prototype.hasOwnProperty.call(parsed, LEGACY_POSITIONS_REFRESH_KEY)
    ? parsed[LEGACY_POSITIONS_REFRESH_KEY]
    : null;
  const legacyKlineValue = parsed && Object.prototype.hasOwnProperty.call(parsed, LEGACY_KLINE_REFRESH_KEY)
    ? parsed[LEGACY_KLINE_REFRESH_KEY]
    : null;
  const out = {};
  REFRESH_MODULE_CONFIG.forEach((item) => {
    let value = item.defaultSec;
    if (parsed && Object.prototype.hasOwnProperty.call(parsed, item.key)) {
      value = parsed[item.key];
    } else if (item.key === POSITIONS_ACTIVE_REFRESH_KEY && legacyPositionsValue !== null) {
      value = legacyPositionsValue;
    } else if (item.key === KLINE_POSITION_REFRESH_KEY && legacyKlineValue !== null) {
      value = legacyKlineValue;
    } else if (
      item.key !== POSITIONS_IDLE_REFRESH_KEY &&
      item.key !== KLINE_IDLE_REFRESH_KEY &&
      hasLegacy
    ) {
      value = legacy;
    }
    out[item.key] = clampRefreshInterval(value, item);
  });
  return out;
}

function positionHasKlineTrackedValue(position) {
  if (position?.has_liquidity) return true;
  const positionUsd = Number(position?.totals?.position_usd);
  if (Number.isFinite(positionUsd) && positionUsd > 0) return true;
  const feeUsd = Number(position?.totals?.fee_usd);
  return Number.isFinite(feeUsd) && feeUsd > 0;
}

function positionMatchesChain(position, activeChain) {
  const rawChain = position?.chain;
  if (rawChain === undefined || rawChain === null) return false;
  const text = String(rawChain).trim();
  if (!text) return false;
  return normalizeChain(text) === activeChain;
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
  maxFeeRate: null,
  minActiveFeeRate: null,
  minTvl: 1000,
  minMarketCap: null,
  minVolume: 2000,
  minTxCount: null,
};
const HOT_POOLS_RISK_FILTER_ALL = 'all';
const HOT_POOLS_RISK_FILTER_OPTIONS = [
  { key: HOT_POOLS_RISK_FILTER_ALL, label: '全部' },
  { key: 'exclude_low_liquidity', label: '排除低流动性' },
  { key: 'only_low_liquidity', label: '仅低流动性' },
];

const defaultHotPoolsFilter = {
  enabled: false,
  keyword: '',
  riskFilter: HOT_POOLS_RISK_FILTER_ALL,
  ...HOT_POOLS_FILTER_DEFAULTS,
};

function normalizeHotPoolsRiskFilter(value) {
  const key = String(value || '').trim();
  return HOT_POOLS_RISK_FILTER_OPTIONS.some((item) => item.key === key)
    ? key
    : HOT_POOLS_RISK_FILTER_ALL;
}

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

function resolveHotPoolMarketCap(pool) {
  const fdv = parseMetricNumber(pool?.fdv_usd);
  if (Number.isFinite(fdv) && fdv > 0) return fdv;
  return parseMetricNumber(pool?.current_token_fdv_usd);
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
  if (Object.prototype.hasOwnProperty.call(value, 'riskFilter')) {
    base.riskFilter = normalizeHotPoolsRiskFilter(value.riskFilter);
  }
  if (Object.prototype.hasOwnProperty.call(value, 'minFees')) {
    base.minFees = parseNullableNumber(value.minFees);
  }
  if (Object.prototype.hasOwnProperty.call(value, 'minFeeRate')) {
    base.minFeeRate = parseNullableNumber(value.minFeeRate);
  }
  if (Object.prototype.hasOwnProperty.call(value, 'maxFeeRate')) {
    base.maxFeeRate = parseNullableNumber(value.maxFeeRate);
  }
  if (Object.prototype.hasOwnProperty.call(value, 'minActiveFeeRate')) {
    base.minActiveFeeRate = parseNullableNumber(value.minActiveFeeRate);
  }
  if (Object.prototype.hasOwnProperty.call(value, 'minTvl')) {
    base.minTvl = parseNullableNumber(value.minTvl);
  }
  if (Object.prototype.hasOwnProperty.call(value, 'minMarketCap')) {
    base.minMarketCap = parseNullableNumber(value.minMarketCap);
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

function hotPoolMatchesRiskFilter(pool, filterKey) {
  const key = normalizeHotPoolsRiskFilter(filterKey);
  if (key === HOT_POOLS_RISK_FILTER_ALL) return true;

  const risk = normalizeTokenRisk(pool?.token_risk);
  const isLowLiquidity = Boolean(risk?.has_low_liquidity);

  switch (key) {
    case 'exclude_low_liquidity':
      return !isLowLiquidity;
    case 'only_low_liquidity':
      return isLowLiquidity;
    default:
      return true;
  }
}

function buildHotPoolsFilterDraft(filter) {
  return {
    enabled: Boolean(filter?.enabled),
    keyword: String(filter?.keyword || ''),
    riskFilter: normalizeHotPoolsRiskFilter(filter?.riskFilter),
    minFees: formatDraftNumber(filter?.minFees),
    minFeeRate: formatDraftNumber(filter?.minFeeRate),
    maxFeeRate: formatDraftNumber(filter?.maxFeeRate),
    minActiveFeeRate: formatDraftNumber(filter?.minActiveFeeRate),
    minTvl: formatDraftNumber(filter?.minTvl),
    minMarketCap: formatDraftNumber(filter?.minMarketCap),
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

function markerWalletSourceLabel(source) {
  const value = String(source || '').trim();
  if (value === 'manual') return '手动添加';
  if (value === 'contract_interaction') return '合约发现';
  return value || '未标记来源';
}

function markerWalletSourceContractLabel(value) {
  const address = normalizeWalletAddress(value);
  return address ? `来源合约 ${shortAddress(address, 6, 4)}` : '';
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

function parseAccessInfo(raw) {
  if (!raw) return null;
  try {
    return normalizeAccessInfo(JSON.parse(raw));
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
  const [accessInfo, setAccessInfo] = useState(() => parseAccessInfo(storageGet(STORAGE.loginAccess)));

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
  const [featuredNews, setFeaturedNews] = useState([]);
  const [tickerNews, setTickerNews] = useState([]);
  const [newsLoading, setNewsLoading] = useState(false);
  const [newsError, setNewsError] = useState('');
  const [newsStatus, setNewsStatus] = useState('empty');
  const [newsTickerSpeed, setNewsTickerSpeed] = useState(() =>
    normalizeNewsTickerSpeed(storageGet(STORAGE.newsTickerSpeed))
  );

  const [positions, setPositions] = useState(null);
  const [positionsLoading, setPositionsLoading] = useState(false);
  const [positionsError, setPositionsError] = useState('');
  const [positionSmartMoneyRanges, setPositionSmartMoneyRanges] = useState({});
  const positionSmartMoneyRangesRef = useRef(positionSmartMoneyRanges);

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
  const klineMarkerRequestSeqRef = useRef(0);
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
  const [refreshIntervals, setRefreshIntervals] = useState(() =>
    normalizeRefreshIntervals(storageGet(STORAGE.refreshIntervals), storageGet(STORAGE.refreshInterval))
  );
  const [refreshIntervalDrafts, setRefreshIntervalDrafts] = useState(() =>
    Object.fromEntries(Object.entries(refreshIntervals).map(([key, value]) => [key, String(value)]))
  );
  const [accentTheme, setAccentTheme] = useState(() =>
    normalizeAccentTheme(storageGet(STORAGE.accentTheme) || 'green')
  );
  const [draggingKey, setDraggingKey] = useState('');
  const [dragOverKey, setDragOverKey] = useState('');
  const klineToolDockRef = useRef(null);
  const hotPoolsHeightControlRef = useRef(null);
  const hotPoolsFilterRef = useRef(null);

  const hasInitData = Boolean(initData);
  const isAdminUser = Boolean(accessInfo?.is_admin || positions?.is_admin);
  const widgetAccessInfo = useMemo(() => {
    if (!accessInfo) return null;
    return normalizeAccessInfo({ ...accessInfo, is_admin: isAdminUser });
  }, [accessInfo, isAdminUser]);
  const availableWidgets = useMemo(() => {
    if (!hasInitData) return WIDGETS.filter((item) => item.key !== 'admin_panel');
    if (!widgetAccessInfo) return WIDGETS.filter((item) => item.key !== 'admin_panel');
    return WIDGETS.filter((item) => canAccessWidget(item.key, widgetAccessInfo));
  }, [hasInitData, widgetAccessInfo]);
  const activeWidgets = useMemo(() => {
    const map = Object.fromEntries(availableWidgets.map((w) => [w.key, w]));
    return widgets.map((k) => map[k]).filter(Boolean);
  }, [availableWidgets, widgets]);
  useEffect(() => {
    if (!hasInitData || !accessInfo) return;
    const allowed = new Set(availableWidgets.map((item) => item.key));
    setWidgets((prev) => {
      const next = prev.filter((key) => allowed.has(key));
      if (next.length === prev.length) return prev;
      return next.length ? next : availableWidgets.slice(0, 1).map((item) => item.key);
    });
  }, [accessInfo, availableWidgets, hasInitData]);
  const layoutClass = moduleLayoutClass(activeWidgets.length);
  const workLayoutClass = workMode ? `work-mode layout-work-${Math.min(activeWidgets.length, 4)}` : layoutClass;
  const hasTrackedPositions = Array.isArray(positions?.positions) && positions.positions.length > 0;
  const hotPoolsRefreshInterval = refreshIntervals.hot_pools;
  const positionsRefreshInterval = hasTrackedPositions
    ? refreshIntervals[POSITIONS_ACTIVE_REFRESH_KEY]
    : refreshIntervals[POSITIONS_IDLE_REFRESH_KEY];
  const assetsRefreshInterval = refreshIntervals.assets;
  const smartMoneyRefreshInterval = refreshIntervals.smart_money;
  const adminRefreshInterval = refreshIntervals.admin_panel;
  const updateRefreshInterval = useCallback((key, value) => {
    const config = getRefreshModuleConfig(key);
    const nextValue = clampRefreshInterval(value, config);
    setRefreshIntervals((prev) => ({
      ...prev,
      [key]: nextValue,
    }));
    setRefreshIntervalDrafts((prev) => ({
      ...prev,
      [key]: String(nextValue),
    }));
  }, []);
  const updateRefreshIntervalDraft = useCallback((key, value) => {
    setRefreshIntervalDrafts((prev) => ({
      ...prev,
      [key]: value,
    }));
  }, []);
  const commitRefreshIntervalDraft = useCallback((key) => {
    const raw = String(refreshIntervalDrafts[key] ?? '').trim();
    if (!raw) {
      setRefreshIntervalDrafts((prev) => ({
        ...prev,
        [key]: String(refreshIntervals[key]),
      }));
      return;
    }
    updateRefreshInterval(key, raw);
  }, [refreshIntervalDrafts, refreshIntervals, updateRefreshInterval]);
  const resetRefreshIntervals = useCallback(() => {
    const next = buildDefaultRefreshIntervals();
    setRefreshIntervals(next);
    setRefreshIntervalDrafts(Object.fromEntries(Object.entries(next).map(([key, value]) => [key, String(value)])));
  }, []);

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
    const hasRiskFilter = normalizeHotPoolsRiskFilter(hotPoolsFilter.riskFilter) !== HOT_POOLS_RISK_FILTER_ALL;
    const hasNumbers = [
      hotPoolsFilter.minFees,
      hotPoolsFilter.minFeeRate,
      hotPoolsFilter.maxFeeRate,
      hotPoolsFilter.minActiveFeeRate,
      hotPoolsFilter.minTvl,
      hotPoolsFilter.minMarketCap,
      hotPoolsFilter.minVolume,
      hotPoolsFilter.minTxCount,
    ].some((value) => Number.isFinite(value));
    return hasKeyword || hasRiskFilter || hasNumbers;
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
  const klinePositionTokenAddresses = useMemo(() => {
    const activeChain = normalizeChain(selectedPool?.chain || chain);
    const rows = Array.isArray(positions?.positions) ? positions.positions : [];
    const seen = new Set();
    rows.forEach((position) => {
      if (!positionHasKlineTrackedValue(position)) return;
      if (!positionMatchesChain(position, activeChain)) return;
      if (!Array.isArray(position?.token_rows)) return;
      position.token_rows.forEach((token) => {
        const address = normalizeHexAddress(token?.address);
        if (address) seen.add(address);
      });
    });
    return seen;
  }, [chain, positions, selectedPool?.chain]);
  const klineHasTrackedPositionToken = Boolean(
    klineTokenAddress && klinePositionTokenAddresses.has(klineTokenAddress)
  );
  const klineRefreshInterval = klineHasTrackedPositionToken
    ? refreshIntervals[KLINE_POSITION_REFRESH_KEY]
    : refreshIntervals[KLINE_IDLE_REFRESH_KEY];
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
    const maxFeeRate = hotPoolsFilterEnabled ? hotPoolsFilter.maxFeeRate : null;
    const minActiveFeeRate = hotPoolsFilterEnabled ? hotPoolsFilter.minActiveFeeRate : null;
    const minTvl = hotPoolsFilterEnabled ? hotPoolsFilter.minTvl : null;
    const minMarketCap = hotPoolsFilterEnabled ? hotPoolsFilter.minMarketCap : null;
    const minVolume = hotPoolsFilterEnabled ? hotPoolsFilter.minVolume : null;
    const minTxCount = hotPoolsFilterEnabled ? hotPoolsFilter.minTxCount : null;
    const riskFilter = hotPoolsFilterEnabled
      ? normalizeHotPoolsRiskFilter(hotPoolsFilter.riskFilter)
      : HOT_POOLS_RISK_FILTER_ALL;
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
        if (hasPosition && !hotPoolsFilterEnabled) return true;

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
        if (Number.isFinite(maxFeeRate) && (!Number.isFinite(feeRate) || feeRate > maxFeeRate)) return false;

        const activeFeeRate = computeHotPoolActiveFeeRate(row);
        const tvl = parseMetricNumber(row?.current_pool_value);
        const marketCap = resolveHotPoolMarketCap(row);
        const volume = parseMetricNumber(row?.total_volume);
        const txCount = parseMetricNumber(row?.transaction_count);
        if (Number.isFinite(minFees) && fees < minFees) return false;
        if (Number.isFinite(minFeeRate) && feeRate < minFeeRate) return false;
        if (Number.isFinite(minActiveFeeRate) && (!Number.isFinite(activeFeeRate) || activeFeeRate < minActiveFeeRate)) return false;
        if (Number.isFinite(minTvl) && tvl < minTvl) return false;
        if (Number.isFinite(minMarketCap) && (!Number.isFinite(marketCap) || marketCap < minMarketCap)) return false;
        if (Number.isFinite(minVolume) && volume < minVolume) return false;
        if (Number.isFinite(minTxCount) && txCount < minTxCount) return false;
        if (!hotPoolMatchesRiskFilter(row, riskFilter)) return false;
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
    return [...rows].sort(comparePositionsByCreatedAt);
  }, [positions]);
  const positionSmartMoneyPoolAddresses = useMemo(() => {
    const seen = new Set();
    sortedPositions.forEach((row) => {
      const poolId = normalizePoolAddress(row?.pool_id || row?.pool_address);
      if (poolId) seen.add(poolId);
    });
    return Array.from(seen).sort();
  }, [sortedPositions]);
  const positionSmartMoneyPoolKey = useMemo(
    () => positionSmartMoneyPoolAddresses.join(','),
    [positionSmartMoneyPoolAddresses]
  );
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

  useEffect(() => {
    positionSmartMoneyRangesRef.current = positionSmartMoneyRanges;
  }, [positionSmartMoneyRanges]);

  useEffect(() => {
    if (positionSmartMoneyPoolAddresses.length === 0) return undefined;

    const now = Date.now();
    const pending = positionSmartMoneyPoolAddresses.filter((poolAddress) => {
      const cached = positionSmartMoneyRangesRef.current[poolAddress];
      return !cached || now - Number(cached.fetchedAt || 0) >= POSITION_SM_RANGE_STALE_MS;
    });
    if (pending.length === 0) return undefined;

    const controller = new AbortController();
    let cancelled = false;

    const loadPoolStats = async (poolAddress) => {
      try {
        const resp = await fetchSMPoolStats({
          apiBaseUrl,
          poolAddress,
          signal: controller.signal,
        });
        if (cancelled) return;
        setPositionSmartMoneyRanges((prev) => ({
          ...prev,
          [poolAddress]: {
            fetchedAt: Date.now(),
            groups: normalizePositionSmartMoneyGroups(resp?.range_groups),
          },
        }));
      } catch {
        if (cancelled || controller.signal.aborted) return;
        setPositionSmartMoneyRanges((prev) => ({
          ...prev,
          [poolAddress]: {
            ...(prev[poolAddress] || {}),
            fetchedAt: Date.now(),
            groups: [],
          },
        }));
      }
    };

    (async () => {
      for (let index = 0; index < pending.length && !cancelled; index += POSITION_SM_RANGE_BATCH_SIZE) {
        const batch = pending.slice(index, index + POSITION_SM_RANGE_BATCH_SIZE);
        await Promise.all(batch.map((poolAddress) => loadPoolStats(poolAddress)));
      }
    })();

    return () => {
      cancelled = true;
      controller.abort();
    };
  }, [apiBaseUrl, positionSmartMoneyPoolKey, positionSmartMoneyPoolAddresses]);

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
          source: String(marker?.wallet_source || '').trim(),
          sourceContract: String(marker?.wallet_source_contract || '').trim(),
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
        current.source = String(marker?.wallet_source || '').trim();
        current.sourceContract = String(marker?.wallet_source_contract || '').trim();
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
  const applyKlineWatchWalletResponse = useCallback((resp) => {
    const nextWallets = Array.isArray(resp?.wallets)
      ? Array.from(new Set(resp.wallets.map((item) => normalizeWalletAddress(item)).filter(Boolean))).sort()
      : [];
    setKlineWatchedWallets(nextWallets);
  }, []);
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
      riskFilter: hotPoolsFilterDraft.riskFilter,
      minFees: parseDraftNumber(hotPoolsFilterDraft.minFees),
      minFeeRate: parseDraftNumber(hotPoolsFilterDraft.minFeeRate),
      maxFeeRate: parseDraftNumber(hotPoolsFilterDraft.maxFeeRate),
      minActiveFeeRate: parseDraftNumber(hotPoolsFilterDraft.minActiveFeeRate),
      minTvl: parseDraftNumber(hotPoolsFilterDraft.minTvl),
      minMarketCap: parseDraftNumber(hotPoolsFilterDraft.minMarketCap),
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
      riskFilter: HOT_POOLS_RISK_FILTER_ALL,
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
      riskFilter: HOT_POOLS_RISK_FILTER_ALL,
      minFees: null,
      minFeeRate: null,
      maxFeeRate: null,
      minActiveFeeRate: null,
      minTvl: null,
      minMarketCap: null,
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
    storageSet(STORAGE.refreshIntervals, JSON.stringify(refreshIntervals));
    storageSet(STORAGE.refreshInterval, String(refreshIntervals[POSITIONS_ACTIVE_REFRESH_KEY]));
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
    if (accessInfo) {
      storageSet(STORAGE.loginAccess, JSON.stringify(accessInfo));
    } else {
      storageRemove(STORAGE.loginAccess);
    }
  }, [accentTheme, accessInfo, chain, hotPoolsPanelHeight, hotSort, initData, klineChartHeight, klineWatchedWallets, loginUser, refreshIntervals, widgets]);

  useEffect(() => {
    if (!hasInitData) return undefined;
    const controller = new AbortController();
    (async () => {
      try {
        const data = await fetchMe({ apiBaseUrl, initData, signal: controller.signal });
        setAccessInfo(normalizeAccessInfo({
          allowed: true,
          is_admin: data?.is_admin,
          mini_app_enabled: data?.mini_app_enabled,
          enabled_modules: data?.enabled_modules,
          module_catalog: data?.module_catalog,
        }));
      } catch (err) {
        if (controller.signal.aborted) return;
        setLoginError(String(err?.message || err || '刷新权限失败'));
      }
    })();
    return () => controller.abort();
  }, [apiBaseUrl, hasInitData, initData]);

  useEffect(() => {
    if (!hasInitData) return undefined;
    let cancelled = false;
    const localWallets = parseStoredWatchWallets(storageGet(STORAGE.smartMoneyWatchWallets));
    (async () => {
      try {
        const resp = await fetchSMWatchWallets({ apiBaseUrl, initData, chain });
        if (cancelled) return;
        const remoteWallets = Array.isArray(resp?.wallets)
          ? Array.from(new Set(resp.wallets.map((item) => normalizeWalletAddress(item)).filter(Boolean))).sort()
          : [];
        const mergedWallets = Array.from(new Set([...remoteWallets, ...localWallets])).sort();
        if (mergedWallets.length !== remoteWallets.length) {
          const synced = await saveSMWatchWallets({ apiBaseUrl, initData, chain, wallets: mergedWallets });
          if (cancelled) return;
          applyKlineWatchWalletResponse(synced);
          return;
        }
        applyKlineWatchWalletResponse(resp);
      } catch {
        // Keep local fallback when backend sync is unavailable.
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [apiBaseUrl, applyKlineWatchWalletResponse, chain, hasInitData, initData]);

  useEffect(() => {
    if (!workMode) return;
    const handler = (e) => { if (e.key === 'Escape') setWorkMode(false); };
    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  }, [workMode]);

  useEffect(() => {
    if (!showSettings) return;
    setRefreshIntervalDrafts(Object.fromEntries(Object.entries(refreshIntervals).map(([key, value]) => [key, String(value)])));
    const handler = (e) => {
      if (e.target.closest('.settings-wrap') || e.target.closest('.settings-popover')) return;
      setShowSettings(false);
    };
    document.addEventListener('mousedown', handler);
    return () => document.removeEventListener('mousedown', handler);
  }, [refreshIntervals, showSettings]);

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
      setHotPoolsLoading(true);
      setHotPoolsError('');
      try {
        const useAdvancedFilters = hasInitData && hotPoolsFilterEnabled;
        const activeTokenFilterAddress = hasInitData ? hotTokenFilter?.address || '' : '';
        const resp = await fetchHotPools({
          apiBaseUrl,
          initData: hasInitData ? initData : '',
          chain,
          sort: hasInitData ? resolveHotPoolServerSort(hotSort) : 'fees',
          timeframeMinutes: 5,
          limit: hasInitData ? hotPoolsLimit : 60,
          tokenAddress: activeTokenFilterAddress,
          includePools: hasInitData && !activeTokenFilterAddress && hotPoolIncludeKey ? hotPoolIncludeKey.split(',') : undefined,
          maxFeeRate: useAdvancedFilters && Number.isFinite(hotPoolsFilter.maxFeeRate) ? hotPoolsFilter.maxFeeRate : undefined,
          minMarketCapUsd: useAdvancedFilters && Number.isFinite(hotPoolsFilter.minMarketCap) ? hotPoolsFilter.minMarketCap : undefined,
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
    [apiBaseUrl, chain, hasInitData, hotPoolIncludeKey, hotPoolsFilter.maxFeeRate, hotPoolsFilter.minMarketCap, hotPoolsFilterEnabled, hotPoolsLimit, hotSort, hotTokenFilter?.address, initData]
  );

  const loadNewsFeeds = useCallback(
    async (signal) => {
      setNewsLoading(true);
      setNewsError('');
      try {
        const [featuredResp, tickerResp] = await Promise.allSettled([
          fetchNewsFeed({ apiBaseUrl, feed: 'featured', limit: 6, signal }),
          fetchNewsFeed({ apiBaseUrl, feed: 'ticker', limit: 24, signal }),
        ]);

        let nextError = '';
        let nextStatus = 'ok';
        if (featuredResp.status === 'fulfilled') {
          setFeaturedNews(dedupeNewsItems(featuredResp.value?.items, 6));
          nextStatus = featuredResp.value?.status || nextStatus;
          if (featuredResp.value?.message) nextError = String(featuredResp.value.message);
        } else if (featuredResp.reason?.name !== 'AbortError') {
          setFeaturedNews([]);
          nextError = String(featuredResp.reason?.message || featuredResp.reason || '新闻读取失败');
          nextStatus = 'error';
        }

        if (tickerResp.status === 'fulfilled') {
          setTickerNews(dedupeNewsItems(tickerResp.value?.items, 24));
          if (!nextError && tickerResp.value?.message) nextError = String(tickerResp.value.message);
        } else if (tickerResp.reason?.name !== 'AbortError') {
          setTickerNews([]);
          if (!nextError) nextError = String(tickerResp.reason?.message || tickerResp.reason || 'ticker 读取失败');
          nextStatus = 'error';
        }

        setNewsStatus(nextStatus);
        setNewsError(nextError);
      } finally {
        setNewsLoading(false);
      }
    },
    [apiBaseUrl]
  );

  const updateNewsTickerSpeed = useCallback((speedKey) => {
    const normalized = normalizeNewsTickerSpeed(speedKey);
    setNewsTickerSpeed(normalized);
    storageSet(STORAGE.newsTickerSpeed, String(normalized));
  }, []);

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
          poolAddress: selectedPoolAddress,
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
      selectedPoolAddress,
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

      const seq = klineMarkerRequestSeqRef.current + 1;
      klineMarkerRequestSeqRef.current = seq;
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
        } else if (smartResp.reason?.name !== 'AbortError') {
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

        if (signal?.aborted || klineMarkerRequestSeqRef.current !== seq) return;
        setKlineMarkers(nextMarkers);
        setKlineMarkersError(nextError);
      } catch (e) {
        if (e?.name !== 'AbortError' && klineMarkerRequestSeqRef.current === seq) {
          setKlineMarkers([]);
          setKlineMarkersError(String(e?.message || e));
        }
      } finally {
        if (klineMarkerRequestSeqRef.current === seq) {
          setKlineMarkersLoading(false);
        }
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
    loadNewsFeeds(ctrl.signal);
    return () => ctrl.abort();
  }, [loadNewsFeeds]);

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
    const timer = window.setInterval(() => loadHotPools(), hotPoolsRefreshInterval * 1000);
    return () => window.clearInterval(timer);
  }, [hotPoolsRefreshInterval, loadHotPools]);

  useEffect(() => {
    const timer = window.setInterval(() => loadNewsFeeds(), 60_000);
    return () => window.clearInterval(timer);
  }, [loadNewsFeeds]);

  useEffect(() => {
    if (!hasInitData) return undefined;
    const timer = window.setInterval(() => loadPositions(), positionsRefreshInterval * 1000);
    return () => window.clearInterval(timer);
  }, [hasInitData, loadPositions, positionsRefreshInterval]);

  useEffect(() => {
    if (!hasInitData) return undefined;
    const timer = window.setInterval(() => loadWalletBalances(), Math.max(positionsRefreshInterval * 1000, 30_000));
    return () => window.clearInterval(timer);
  }, [hasInitData, loadWalletBalances, positionsRefreshInterval]);

  useEffect(() => {
    if (!hasInitData || !klineTokenAddress) return undefined;
    const timer = window.setInterval(() => setKlineRefreshNonce((n) => n + 1), klineRefreshInterval * 1000);
    return () => window.clearInterval(timer);
  }, [hasInitData, klineRefreshInterval, klineTokenAddress]);

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
    setAccessInfo(normalizeAccessInfo(resp?.access));
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
    setAccessInfo(null);
    setWorkMode(false);
    setHotTokenFilter(null);
    setHotPools([]);
    setHotPoolsUpdatedAt('');
    setHotPoolsError('');
    storageRemove(STORAGE.initData);
    storageRemove(STORAGE.loginUser);
    storageRemove(STORAGE.loginAccess);
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

  const handleToggleKlineWatch = useCallback((walletAddress, nextWatched) => {
    const address = normalizeWalletAddress(walletAddress);
    if (!address) return;
    setKlineWatchToggleMap((prev) => ({ ...prev, [address]: true }));

    const clearBusy = () => {
      setKlineWatchToggleMap((prev) => {
        if (!prev[address]) return prev;
        const next = { ...prev };
        delete next[address];
        return next;
      });
    };

    if (!hasInitData) {
      setKlineWatchedWallets((prev) => {
        const next = new Set(prev);
        const shouldWatch = typeof nextWatched === 'boolean' ? nextWatched : !next.has(address);
        if (shouldWatch) next.add(address);
        else next.delete(address);
        return Array.from(next).sort();
      });
      window.setTimeout(clearBusy, 0);
      return;
    }

    const watched = typeof nextWatched === 'boolean' ? nextWatched : !klineWatchedWalletSet.has(address);
    saveSMWatchWallets({ apiBaseUrl, initData, chain, walletAddress: address, watched })
      .then((resp) => {
        applyKlineWatchWalletResponse(resp);
      })
      .catch(() => {
        // Ignore and keep previous state if remote persistence fails.
      })
      .finally(clearBusy);
  }, [apiBaseUrl, applyKlineWatchWalletResponse, chain, hasInitData, initData, klineWatchedWalletSet]);

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
  const [addLiqPosition, setAddLiqPosition] = useState(null);
  const [operationProgress, setOperationProgress] = useState(null);
  const [confirmDialog, setConfirmDialog] = useState(null);

  const requestConfirm = useCallback((options) => new Promise((resolve) => {
    setConfirmDialog({
      ...options,
      onResolve: resolve,
    });
  }), []);

  const closeConfirmDialog = useCallback((value) => {
    setConfirmDialog((current) => {
      current?.onResolve?.(value);
      return null;
    });
  }, []);

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
      const resp = await apiOpenPosition({ apiBaseUrl, initData, ...params });
      const taskId = Number(resp?.task_id);
      setOperationProgress(prev => prev?.operation === 'open_position'
        ? {
          ...prev,
          taskId: Number.isFinite(taskId) && taskId > 0 ? taskId : prev.taskId,
          currentStep: 3,
          status: 'active',
        } : prev);
      setOpenPosSubmitError('');
      setOpenPosRisk(null);
      setOpenPosSmartRanges([]);
      setOpenPosSmartRangesLoading(false);
      await loadPositions();
      setOperationProgress(prev => prev?.operation === 'open_position'
        ? {
          ...prev,
          taskId: Number.isFinite(taskId) && taskId > 0 ? taskId : prev.taskId,
          currentStep: 4,
          status: 'done',
        } : prev);
      setOpenPosPool(null);
    } catch (e) {
      const msg = String(e?.message || e);
      const errorCode = String(e?.code || '').trim();
      const isSafetyFailure = e && typeof e === 'object' && (
        typeof e?.liquidity_usd === 'number' ||
        typeof e?.max_open_amount === 'number' ||
        Boolean(e?.risk_ack_required) ||
        Boolean(e?.token_risk) ||
        typeof e?.price_deviation_percent === 'number' ||
        errorCode === 'token_honeypot' ||
        errorCode === 'zap_safety_check_failed' ||
        errorCode.startsWith('pool_')
      );
      const risk = isSafetyFailure
        ? {
          code: errorCode,
          message: msg,
          liquidity_usd: Number(e?.liquidity_usd),
          min_liquidity_usd: Number(e?.min_liquidity_usd),
          max_open_amount: Number(e?.max_open_amount),
          risk_ack_required: Boolean(e?.risk_ack_required),
          price_deviation_percent: Number(e?.price_deviation_percent),
          price_deviation_max_percent: Number(e?.price_deviation_max_percent),
          token_risk: e?.token_risk || null,
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

  const handleTaskPartialExit = useCallback(async (taskId, exitPercent) => {
    const pct = Number(exitPercent);
    if (!Number.isFinite(pct) || pct <= 0 || pct > 100) return;
    const message = pct >= 100
      ? '确认停止该仓位？\n系统会关闭相关任务，并尽量将剩余价值结算为 USDT。'
      : `确认撤出当前仓位的 ${pct}% 并兑换为 USDT？\n任务会保留剩余仓位继续运行。`;
    const ok = await requestConfirm({
      title: pct >= 100 ? '停止仓位？' : '部分撤仓？',
      message,
      confirmText: pct >= 100 ? '停止' : '撤仓',
      danger: true,
    });
    if (!ok) return;
    if (pct >= 100) {
      await handleTaskStop(taskId);
      return;
    }
    const resp = await stopTask({ apiBaseUrl, initData, taskId, exitPercent: pct });
    if (resp?.message) {
      setPositionsError('');
    }
    loadPositions();
  }, [apiBaseUrl, initData, handleTaskStop, loadPositions, requestConfirm]);

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

  const handleWithdrawLiquidity = useCallback(async (taskId) => {
    const ok = await requestConfirm({
      title: '取回全部流动性？',
      message: '确认要取回全部流动性？\n该操作只会撤出仓位流动性，不会自动兑换为 USDT，并会停止任务。',
      confirmText: '取回',
      danger: true,
    });
    if (!ok) return;
    await withdrawLiquidity({ apiBaseUrl, initData, taskId });
    loadPositions();
  }, [apiBaseUrl, initData, loadPositions, requestConfirm]);

  const handleSwapDust = useCallback(async (taskId) => {
    await swapDust({ apiBaseUrl, initData, taskId });
    loadPositions();
  }, [apiBaseUrl, initData, loadPositions]);

  const handleTriggerRebalance = useCallback(async (taskId) => {
    await triggerRebalance({ apiBaseUrl, initData, taskId });
    loadPositions();
  }, [apiBaseUrl, initData, loadPositions]);

  const handleUpdateTaskMode = useCallback(async (taskId, taskMode) => {
    await updateTaskMode({ apiBaseUrl, initData, taskId, taskMode });
    loadPositions();
  }, [apiBaseUrl, initData, loadPositions]);

  const handleAddLiquidity = useCallback(async (taskId, position) => {
    const resolvedTaskId = Number(taskId || position?.task_id || 0);
    if (!Number.isFinite(resolvedTaskId) || resolvedTaskId <= 0) return;
    setAddLiqPosition({ ...position, task_id: resolvedTaskId });
  }, []);

  const openAddLiquidityModal = useCallback((taskId, position) => {
    const resolvedTaskId = Number(taskId || position?.task_id || 0);
    if (!Number.isFinite(resolvedTaskId) || resolvedTaskId <= 0) return;
    setAddLiqPosition({ ...position, task_id: resolvedTaskId });
  }, []);

  const confirmAddLiquidity = useCallback(async (payload) => {
    const taskId = Number(addLiqPosition?.task_id || 0);
    if (!Number.isFinite(taskId) || taskId <= 0) {
      throw new Error('Task is missing for add liquidity.');
    }
    const amount = typeof payload === 'object' && payload !== null ? payload.amount : payload;
    const slippageTolerance = typeof payload === 'object' && payload !== null ? payload.slippageTolerance : undefined;
    await addLiquidity({ apiBaseUrl, initData, taskId, amountUsdt: amount, slippageTolerance });
    loadPositions().catch(() => {});
  }, [addLiqPosition, apiBaseUrl, initData, loadPositions]);

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
      <HotPoolsPanel
        workMode={workMode}
        displayLimit={HOT_POOLS_DISPLAY_LIMIT}
        minPanelHeight={MIN_HOT_POOLS_PANEL_HEIGHT}
        maxPanelHeight={MAX_HOT_POOLS_PANEL_HEIGHT}
        panelHeight={hotPoolsPanelHeight}
        heightSettingsOpen={hotPoolsHeightSettingsOpen}
        panelHeightCustomized={hotPoolsPanelHeightCustomized}
        heightControlRef={hotPoolsHeightControlRef}
        filterRef={hotPoolsFilterRef}
        hotPools={hotPools}
        filteredHotPools={filteredHotPools}
        hotPoolsLoading={hotPoolsLoading}
        hotPoolsError={hotPoolsError}
        hotPoolsUpdatedAt={hotPoolsUpdatedAt}
        hotSort={hotSort}
        hotInlineSort={hotInlineSort}
        hotPoolsFilterOpen={hotPoolsFilterOpen}
        hotPoolsFilterEnabled={hotPoolsFilterEnabled}
        hotPoolsFilterDraft={hotPoolsFilterDraft}
        hotPoolsFilterDefaults={HOT_POOLS_FILTER_DEFAULTS}
        hotPoolsRiskFilterOptions={HOT_POOLS_RISK_FILTER_OPTIONS}
        hotTokenFilter={hotTokenFilter}
        keyword={keyword}
        searchOpen={searchOpen}
        selectedPoolAddress={selectedPoolAddress}
        chain={chain}
        getDexIcon={getDexIcon}
        onPanelHeightToggle={() => setHotPoolsHeightSettingsOpen((prev) => !prev)}
        onPanelHeightClose={() => setHotPoolsHeightSettingsOpen(false)}
        onPanelHeightChange={(value) => setHotPoolsPanelHeight(clampHotPoolsPanelHeight(value, hotPoolsDefaultHeightRef.current))}
        onPanelHeightReset={resetHotPoolsPanelHeight}
        onHotSortChange={(key) => {
          setHotSort(key);
          setHotInlineSort('');
        }}
        onHotInlineSortChange={setHotInlineSort}
        onSearchToggle={() => {
          setHotPoolsFilterOpen(false);
          setSearchOpen((v) => !v);
        }}
        onKeywordChange={setKeyword}
        onFilterToggle={() => {
          if (hotPoolsFilterOpen) {
            setHotPoolsFilterOpen(false);
            return;
          }
          openHotPoolsFilter();
        }}
        onFilterClose={() => setHotPoolsFilterOpen(false)}
        onFilterDraftChange={setHotPoolsFilterDraft}
        onFilterApply={applyHotPoolsFilter}
        onFilterReset={resetHotPoolsFilter}
        onFilterClear={clearHotPoolsFilter}
        onTokenFilterClear={() => setHotTokenFilter(null)}
        onToggleTokenFilter={(filterToken) => {
          setHotTokenFilter((prev) => (
            prev?.address === filterToken.address ? null : filterToken
          ));
        }}
        onCopyAddress={copyAddr}
        onSelectPool={selectPool}
        onOpenPosition={openPositionModal}
        operationProgress={renderOperationProgress('hot_pools')}
      />
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
                    <Button
                      key={item.key}
                      type="button"
                      variant="ghost"
                      size="sm"
                      active={klineActiveTokenSide === item.key}
                      className={`ghost-chip ${klineActiveTokenSide === item.key ? 'active' : ''}`}
                      onClick={() => setKlineTokenSide(item.key)}
                    >
                      {item.symbol}
                    </Button>
                  ))
                ) : (
                  <div className="kline-token-pill">
                    {klineActiveToken?.symbol || 'Token'}
                  </div>
                )}
              </div>

              <div className="kline-toolbar-group">
                {KLINE_INTERVALS.map((item) => (
                  <Button
                    key={item.key}
                    type="button"
                    variant="ghost"
                    size="sm"
                    active={klineInterval === item.key}
                    className={`ghost-chip ${klineInterval === item.key ? 'active' : ''}`}
                    onClick={() => setKlineInterval(item.key)}
                  >
                    {item.label}
                  </Button>
                ))}
              </div>

              <div className="kline-toolbar-group align-right">
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  className="ghost-chip"
                  onClick={refreshKline}
                  disabled={klineLoading}
                >
                  <RefreshCw size={12} />
                  刷新
                </Button>
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
                    <IconButton
                      key={tool.key}
                      type="button"
                      className={`kline-tool-btn ${klineDrawTool === tool.key ? 'active' : ''}`}
                      active={klineDrawTool === tool.key}
                      onClick={() => setKlineDrawTool(tool.key)}
                      title={tool.title}
                      aria-label={tool.title}
                    >
                      <Icon size={16} />
                    </IconButton>
                  );
                })}

                <div className="kline-filter-shell">
                  <IconButton
                    type="button"
                    className={`kline-tool-btn ${klineMarkerFilterOpen || klineFilterActive ? 'active' : ''}`}
                    active={klineMarkerFilterOpen || klineFilterActive}
                    onClick={() => {
                      setKlineHeightSettingsOpen(false);
                      setKlineMarkerFilterOpen((prev) => !prev);
                    }}
                    title="气泡筛选"
                    aria-label="气泡筛选"
                  >
                    <SlidersHorizontal size={16} />
                  </IconButton>

                  {klineMarkerFilterOpen ? (
                    <div className="popover kline-filter-popover tool-dock">
                      <div className="kline-filter-popover-head">
                        <div>
                          <div className="kline-filter-popover-title">气泡筛选</div>
                          <div className="kline-filter-popover-sub">仅筛选当前已加载的气泡</div>
                        </div>
                        <IconButton
                          type="button"
                          className="icon-link"
                          onClick={() => setKlineMarkerFilterOpen(false)}
                          title="Close"
                          aria-label="Close"
                        >
                          <X size={14} />
                        </IconButton>
                      </div>

                      <label className="kline-filter-field">
                        <span>最低金额</span>
                        <Input
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
                                    {shortAddress(wallet.address, 6, 4)} · {markerWalletSourceLabel(wallet.source)} · {wallet.markerCount} 个气泡 · 最高 {formatUsdCompact(wallet.maxEstimatedUsd)}
                                    {markerWalletSourceContractLabel(wallet.sourceContract) ? ` · ${markerWalletSourceContractLabel(wallet.sourceContract)}` : ''}
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

                <IconButton
                  type="button"
                  className={`kline-tool-btn ${klineHeightSettingsOpen || klineChartHeightCustomized ? 'active' : ''}`}
                  active={klineHeightSettingsOpen || klineChartHeightCustomized}
                  onClick={() => {
                    setKlineMarkerFilterOpen(false);
                    setKlineHeightSettingsOpen((prev) => !prev);
                  }}
                  title="图表高度"
                  aria-label="图表高度"
                >
                  <Settings size={16} />
                </IconButton>

                <IconButton
                  type="button"
                  className="kline-tool-btn"
                  onClick={clearKlineDrawing}
                  disabled={!klineCandles.length}
                  title="Clear"
                  aria-label="Clear"
                >
                  <X size={16} />
                </IconButton>

                {klineHeightSettingsOpen ? (
                  <div className="popover kline-settings-popover tool-dock">
                    <div className="kline-filter-popover-head">
                      <div>
                        <div className="kline-filter-popover-title">图表高度</div>
                        <div className="kline-filter-popover-sub">仅保存在当前浏览器</div>
                      </div>
                      <IconButton
                        type="button"
                        className="icon-link"
                        onClick={() => setKlineHeightSettingsOpen(false)}
                        title="Close"
                        aria-label="Close"
                      >
                        <X size={14} />
                      </IconButton>
                    </div>

                    <div className="kline-height-value">{klineChartHeight}px</div>

                    <Slider
                      className="kline-height-slider"
                      min={MIN_KLINE_CHART_HEIGHT}
                      max={MAX_KLINE_CHART_HEIGHT}
                      step={20}
                      value={[klineChartHeight]}
                      onValueChange={([value]) => setKlineChartHeight(clampKlineChartHeight(value))}
                    />

                    <label className="kline-filter-field">
                      <span>高度</span>
                      <div className="kline-height-input-row">
                        <Input
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
      <PositionsPanel
        positions={positions}
        positionsLoading={positionsLoading}
        positionsError={positionsError}
        sortedPositions={sortedPositions}
        walletBalances={walletBalances}
        walletMetaByKey={walletMetaByKey}
        positionSmartMoneyRanges={positionSmartMoneyRanges}
        chain={chain}
        taskActionPos={taskActionPos}
        onTaskActionPosChange={setTaskActionPos}
        onSelectPool={selectPool}
        onTaskPause={handleTaskPause}
        onTaskStop={handleTaskStop}
        onTaskPartialExit={handleTaskPartialExit}
        onTaskDelete={handleTaskDelete}
        onTaskEditRange={handleTaskEditRange}
        onWithdrawLiquidity={handleWithdrawLiquidity}
        onSwapDust={handleSwapDust}
        onTriggerRebalance={handleTriggerRebalance}
        onUpdateTaskMode={handleUpdateTaskMode}
        onAddLiquidity={openAddLiquidityModal}
        onCloseTaskActionMenu={() => setTaskActionPos(null)}
        getDexIcon={getDexIcon}
        operationProgress={renderOperationProgress('positions')}
      />
    ),

    assets: (
      <Suspense fallback={(
        <PanelShell title="我的" subtitle="正在加载模块" icon={BriefcaseBusiness}>
          <EmptyState text="正在加载我的模块..." />
        </PanelShell>
      )}>
        <LazyAssetManagementPanel
          apiBaseUrl={apiBaseUrl}
          initData={initData}
          hasInitData={hasInitData}
          isAdmin={isAdminUser}
          refreshInterval={assetsRefreshInterval}
        />
      </Suspense>
    ),

    smart_money: (
      <SmartMoneyDashboard
        apiBaseUrl={apiBaseUrl}
        initData={initData}
        watchedWallets={klineWatchedWallets}
        watchedWalletSet={klineWatchedWalletSet}
        watchToggleMap={klineWatchToggleMap}
        onToggleWatchWallet={handleToggleKlineWatch}
        onSelectPool={selectPool}
        activePoolAddress={selectedPoolAddress}
        refreshInterval={smartMoneyRefreshInterval}
        onOpenPosition={(pool) => openPositionModal({
          ...pool,
          chain: String(pool?.chain || (Number(pool?.chain_id) === 8453 ? 'base' : chain)).toLowerCase(),
          panelKey: 'smart_money',
        })}
        isAdmin={isAdminUser}
      />
    ),

    global_config: (
      <GlobalConfigPanel
        apiBaseUrl={apiBaseUrl}
        initData={initData}
        hasInitData={hasInitData}
      />
    ),

    wallet_manage: (
      <WalletManagePanel
        apiBaseUrl={apiBaseUrl}
        initData={initData}
        hasInitData={hasInitData}
        chain={chain}
      />
    ),

    swap: (
      <SwapPanel
        apiBaseUrl={apiBaseUrl}
        initData={initData}
        hasInitData={hasInitData}
        chain={chain}
        onChainChange={setChain}
      />
    ),

    trade_history: (
      <TradeHistoryPanel
        apiBaseUrl={apiBaseUrl}
        initData={initData}
        hasInitData={hasInitData}
      />
    ),

    admin_panel: (
      <Suspense fallback={(
        <PanelShell title="管理员" subtitle="正在加载模块" icon={Shield}>
          <EmptyState text="正在加载管理员模块..." />
        </PanelShell>
      )}>
        <LazyAdminPanel
          apiBaseUrl={apiBaseUrl}
          initData={initData}
          hasInitData={hasInitData}
          isAdmin={isAdminUser}
          refreshInterval={adminRefreshInterval}
        />
      </Suspense>
    ),
  };

  return (
    <div
      className={`app-shell ${workMode && hasInitData ? 'work-mode-shell' : ''} ${hasInitData ? '' : 'guest-shell-mode'}`}
      data-accent-theme={accentTheme}
    >
      <div className="bg-orb orb-a" />
      <div className="bg-orb orb-b" />
      <div className="bg-grid" />

      {workMode && hasInitData ? (
        <WorkModeBar onExit={() => setWorkMode(false)} />
      ) : (
        <>
          <TopBar
            hasInitData={hasInitData}
            loginUser={loginUser}
            loginCode={loginCode}
            loginBusy={loginBusy}
            showSettings={showSettings}
            onSettingsOpenChange={setShowSettings}
            onStartLogin={startCodeLogin}
            onCancelLogin={() => { setLoginCode(''); setLoginError(''); }}
            onLogout={logout}
            newsShowcase={(
              <NewsShowcase
                items={featuredNews}
                loading={newsLoading}
                error={newsError}
                status={newsStatus}
                onOpen={openExternal}
              />
            )}
            settings={{
              refreshModuleConfig: REFRESH_MODULE_CONFIG,
              maxRefreshIntervalSec: MAX_REFRESH_INTERVAL_SEC,
              refreshIntervalDrafts,
              refreshIntervals,
              onRefreshDraftChange: updateRefreshIntervalDraft,
              onRefreshDraftCommit: commitRefreshIntervalDraft,
              onResetRefreshIntervals: resetRefreshIntervals,
              accentThemes: ACCENT_THEMES,
              accentTheme,
              onAccentThemeChange: setAccentTheme,
              newsTickerSpeed,
              newsTickerMinSpeed: NEWS_TICKER_MIN_SPEED,
              newsTickerMaxSpeed: NEWS_TICKER_MAX_SPEED,
              onNewsTickerSpeedChange: updateNewsTickerSpeed,
              hasTrackedPositions,
              klineHasTrackedPositionToken,
            }}
          />

      {loginError && hasInitData ? <div className="error-text top-error">{loginError}</div> : null}

      <WorkbenchConfigPanel
        hasInitData={hasInitData}
        chain={chain}
        onChainChange={setChain}
        availableWidgets={availableWidgets}
        widgets={widgets}
        onToggleWidget={toggleWidget}
        onEnterWorkMode={() => setWorkMode(true)}
      />
      </>
      )}

      {!hasInitData ? (
        <GuestHotPoolsLanding
          chain={chain}
          hotPools={hotPools}
          hotPoolsLoading={hotPoolsLoading}
          hotPoolsError={hotPoolsError}
          hotPoolsUpdatedAt={hotPoolsUpdatedAt}
          loginBusy={loginBusy}
          loginCode={loginCode}
          loginError={loginError}
          onStartLogin={startCodeLogin}
          onCancelLogin={() => { setLoginCode(''); setLoginError(''); }}
          onChainChange={setChain}
        />
      ) : (
      <WorkbenchLayout
        className={`workbench ${workLayoutClass}`}
        activeWidgets={activeWidgets}
        panelMap={panelMap}
        hotPoolsPanelHeight={hotPoolsPanelHeight}
        draggingKey={draggingKey}
        dragOverKey={dragOverKey}
        onDragOverWidget={(e, widgetKey) => {
          e.preventDefault();
          e.dataTransfer.dropEffect = 'move';
          if (draggingKey && draggingKey !== widgetKey) {
            setDragOverKey(widgetKey);
          }
        }}
        onDropWidget={(e, widgetKey) => {
          e.preventDefault();
          const from = e.dataTransfer.getData('text/plain') || draggingKey;
          if (from && from !== widgetKey) {
            setWidgets((prev) => reorderList(prev, from, widgetKey));
          }
          setDraggingKey('');
          setDragOverKey('');
        }}
        onDragEnd={() => {
          setDraggingKey('');
          setDragOverKey('');
        }}
        onDragStartWidget={(e, widgetKey) => {
          setDraggingKey(widgetKey);
          e.dataTransfer.effectAllowed = 'move';
          e.dataTransfer.setData('text/plain', widgetKey);
        }}
      />
      )}

      {hasInitData ? (
        <NewsTicker
          items={tickerNews}
          loading={newsLoading}
          error={newsError}
          speedPxPerSec={newsTickerSpeed}
          onOpen={openExternal}
        />
      ) : null}

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

      {addLiqPosition ? (
        <AddLiquidityModal
          position={addLiqPosition}
          onConfirm={confirmAddLiquidity}
          onClose={() => setAddLiqPosition(null)}
        />
      ) : null}

      <ConfirmDialog
        open={Boolean(confirmDialog)}
        title={confirmDialog?.title}
        message={confirmDialog?.message}
        confirmText={confirmDialog?.confirmText}
        cancelText={confirmDialog?.cancelText}
        danger={Boolean(confirmDialog?.danger)}
        onConfirm={() => closeConfirmDialog(true)}
        onCancel={() => closeConfirmDialog(false)}
      />
    </div>
  );
}
