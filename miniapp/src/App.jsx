import React, { useEffect, useMemo, useRef, useState, useCallback } from 'react';
import HotPoolCard from './components/HotPoolCard.jsx';
import KlineModal from './components/KlineModal.jsx';
import AutoMonitorCard from './components/AutoMonitorCard.jsx';
import PositionCard from './components/PositionCard.jsx';
import AutoPnLCurveCard from './components/AutoPnLCurveCard.jsx';
import SmartMoneyCard from './components/SmartMoneyCard.jsx';
import SystemConfigCard from './components/SystemConfigCard.jsx';
import BottomSheet from './components/BottomSheet.jsx';
import ModuleHeader from './components/ModuleHeader.jsx';
import NumberFlowValue from './components/NumberFlowValue.jsx';
import StepProgressModal from './components/StepProgressModal.jsx';
import { SkeletonHotPoolCard, SkeletonPositionCard, SkeletonList } from './components/Skeleton.jsx';
import AdminPage from './components/AdminPage.jsx';
import { Bot, BarChart2, Filter, Search, Moon, Sun, Settings, X, Check, RotateCcw, AlertTriangle, Flame } from 'lucide-react';
import {
    deleteTask,
    disableAdminAutoLP,
    fetchAutoMonitor,
    fetchAutoLPPnLCurve,
    fetchAdminAutoLPStats,
    fetchAdminRealtimePositions,
    fetchAdminRealtimeUsers,
    fetchGlobalConfig,
    fetchWallets,
    fetchHotPools,
    fetchSearchPools,
    fetchMe,
    fetchSmartMoneyOverview,
    fetchSmartMoneyPoolAdds,
    fetchRealtimePositions,
    openPosition,
    updateTaskRange,
    setAutoLPGuardCompareToPeak,
    setTaskPaused,
    stopTask,
    addToBlacklist,
    removeFromBlacklist,
    fetchBlacklist,
    fetchCooldowns,
    removeCooldown,
    saveGlobalConfig,
} from './lib/api';
import { getTelegramWebApp, hapticImpact, hapticNotification, hapticSelection } from './lib/telegram';
import { formatRelativeTime, useTick } from './lib/time';
import {
    ACCENT_THEME_OPTIONS,
    getBrandTheme,
    normalizeAccentTheme,
} from './lib/brand';

function resolveApiBaseUrl() {
    const queryApiBase = new URLSearchParams(window.location.search).get('apiBaseUrl');
    if (queryApiBase && queryApiBase.trim()) return queryApiBase.trim();

    const envBase = String(import.meta.env.VITE_API_BASE_URL || '').trim();
    if (envBase) {
        try {
            const pageProto = window.location.protocol;
            const envProto = new URL(envBase).protocol;
            if (pageProto === 'https:' && envProto === 'http:') {
                return '';
            }
        } catch {
            // ignore URL parse errors and keep envBase as-is
        }
        return envBase;
    }

    const host = window.location.hostname;
    if (host === 'localhost' || host === '127.0.0.1') {
        return 'http://localhost:8080';
    }

    // Production default: same-origin `/api/*` (e.g. via Vercel Function proxy)
    return '';
}

function resolveAllowEmptyInitData() {
    const queryAllow = new URLSearchParams(window.location.search).get('allowEmptyInitData');
    if (queryAllow && ['1', 'true', 'yes', 'y', 'on'].includes(queryAllow.toLowerCase())) {
        return true;
    }

    const envAllow = String(import.meta.env.VITE_ALLOW_EMPTY_INITDATA || '').trim().toLowerCase();
    if (['1', 'true', 'yes', 'y', 'on'].includes(envAllow)) {
        return true;
    }

    const host = window.location.hostname;
    return host === 'localhost' || host === '127.0.0.1';
}

function useInitData() {
    const [initData, setInitData] = useState('');
    useEffect(() => {
        const tg = getTelegramWebApp();
        if (!tg) {
            const fromQuery = new URLSearchParams(window.location.search).get('initData');
            if (fromQuery) setInitData(fromQuery);
            return;
        }
        try {
            tg.ready?.();
            tg.expand?.();
        } catch {
            // ignore
        }
        setInitData(tg.initData || '');
    }, []);
    return initData;
}

const storage = {
    get(key) {
        try {
            return window.localStorage?.getItem(key) ?? null;
        } catch {
            return null;
        }
    },
    set(key, value) {
        try {
            window.localStorage?.setItem(key, value);
        } catch {
            // ignore
        }
    },
    remove(key) {
        try {
            window.localStorage?.removeItem(key);
        } catch {
            // ignore
        }
    },
};

const STORAGE_THEME = 'tglp_theme';
const STORAGE_ACCENT_THEME = 'tglp_accent_theme';
const STORAGE_POLL_SEC = 'tglp_poll_interval_sec';
const STORAGE_HOT_POOLS_FILTER = 'tglp_hot_pools_filter_v1';
const STORAGE_OPEN_POSITION_WALLET_ID = 'tglp_open_position_wallet_id';
const STORAGE_WEB_WORKBENCH_WIDGETS = 'tglp_web_workbench_widgets_v1';

const WEB_WORKBENCH_WIDGETS = [
    { key: 'hot_pools', label: '热门池子' },
    { key: 'gmgn_kline', label: 'K线' },
    { key: 'positions', label: '仓位' },
    { key: 'smart_money', label: '聪明钱' },
];
const DEFAULT_WEB_WORKBENCH_WIDGETS = WEB_WORKBENCH_WIDGETS.map((item) => item.key);

const GMGN_STABLE_SYMBOLS = new Set(['usdc', 'usdt', 'busd', 'dai', 'frax', 'usdd', 'fdusd', 'wbnb', 'weth', 'wsol', 'bnb', 'eth', 'sol']);

function normalizeWebWorkbenchWidgets(value) {
    if (!Array.isArray(value)) return [...DEFAULT_WEB_WORKBENCH_WIDGETS];
    const allow = new Set(DEFAULT_WEB_WORKBENCH_WIDGETS);
    const seen = new Set();
    const next = [];
    for (const raw of value) {
        const key = String(raw || '').trim();
        if (!allow.has(key) || seen.has(key)) continue;
        seen.add(key);
        next.push(key);
    }
    if (next.length === 0) return [...DEFAULT_WEB_WORKBENCH_WIDGETS];
    return next;
}

function pickGmgnTokenAddress(pool) {
    const pair = String(pool?.trading_pair || '').trim();
    const token0 = String(pool?.token0_address || '').trim();
    const token1 = String(pool?.token1_address || '').trim();
    if (!pair) return token0 || token1;

    const symbols = pair.split('/').map((part) => String(part || '').trim().toLowerCase());
    if (symbols.length !== 2) return token0 || token1;

    const [leftSymbol, rightSymbol] = symbols;
    const leftStable = GMGN_STABLE_SYMBOLS.has(leftSymbol);
    const rightStable = GMGN_STABLE_SYMBOLS.has(rightSymbol);
    if (leftStable && !rightStable) return token1 || token0;
    if (rightStable && !leftStable) return token0 || token1;
    return token0 || token1;
}

function buildGmgnUrl(pool, fallbackChain = 'bsc') {
    const tokenAddress = pickGmgnTokenAddress(pool);
    if (!tokenAddress) return '';
    const chain = String(pool?.chain || fallbackChain || 'bsc').trim().toLowerCase() === 'base' ? 'base' : 'bsc';
    return `https://gmgn.ai/${chain}/token/${tokenAddress}`;
}

const USD_DISPLAY_LIMIT = 1e15;
const usdFormatter = new Intl.NumberFormat('en-US', {
    style: 'currency',
    currency: 'USD',
    maximumFractionDigits: 2,
});

function formatUsd(v) {
    const n = Number(v ?? 0);
    if (!Number.isFinite(n) || Math.abs(n) > USD_DISPLAY_LIMIT) return '$--';
    return usdFormatter.format(n);
}

const defaultHotPoolsFilter = {
    enabled: true,
    keyword: '',
    minFees: 60,
    minFeeRate: 0.3,
    minTvl: 1000,
    minVolume: 2000,
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
    if (Object.prototype.hasOwnProperty.call(value, 'minTvl')) {
        base.minTvl = parseNullableNumber(value.minTvl);
    }
    if (Object.prototype.hasOwnProperty.call(value, 'minVolume')) {
        base.minVolume = parseNullableNumber(value.minVolume);
    }
    return base;
}

function parseDraftNumber(raw) {
    const text = String(raw || '').trim();
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

function formatUserLabel(user) {
    if (!user) return '未知用户';
    const username = String(user.username || '').trim();
    if (username) return `@${username}`;
    const first = String(user.first_name || '').trim();
    const last = String(user.last_name || '').trim();
    const full = `${first} ${last}`.trim();
    if (full) return full;
    const telegramId = String(user.telegram_id || '').trim();
    if (telegramId) return `TG ${telegramId}`;
    const userId = String(user.user_id || '').trim();
    if (userId) return `用户 ${userId}`;
    return '未知用户';
}

function formatOnOff(value) {
    return value ? '开启' : '关闭';
}

const icons = {
    bot: Bot,
    chart: BarChart2,
    filter: Filter,
    search: Search,
    moon: Moon,
    sun: Sun,
    gear: Settings,
    close: X,
    check: Check,
    reset: RotateCcw,
    alert: AlertTriangle,
    fire: Flame,
};

const Icon = ({ path: IconCmp, className = '' }) => {
    if (!IconCmp) return null;
    return <IconCmp className={className} strokeWidth={2} />;
};

function buildTopNavItems({ isAdmin, smartMoneyEnabled }) {
    const items = [
        { key: 'hot_pools', label: '热门池子' },
        { key: 'positions', label: '实时仓位' },
        { key: 'monitor', label: '监控' },
    ];
    if (smartMoneyEnabled) items.push({ key: 'smart_money', label: '聪明钱' });
    if (isAdmin) items.push({ key: 'admin', label: '管理' });
    return items;
}

const HOT_POOL_SORT_TABS = [
    { key: 'fees', label: '费用' },
    { key: 'fee_rate', label: '费用率' },
    { key: 'volume', label: '交易量' },
];

const POSITION_TASK_TABS = [
    { key: 'all', label: '全部' },
    { key: 'manual', label: '手动任务' },
    { key: 'auto', label: 'Auto任务' },
];

export default function App() {
    const initData = useInitData();
    const tick = useTick(); // 实时时钟，每秒更新一次
    const [me, setMe] = useState(null);
    const [data, setData] = useState(null);
    const [error, setError] = useState('');
    const [loading, setLoading] = useState(false);
    const pollRef = useRef(null);
    const [viewMode, setViewMode] = useState('hot_pools');
    const [positionsTaskTab, setPositionsTaskTab] = useState('all'); // all | manual | auto
    const [autoMonitor, setAutoMonitor] = useState(null);
    const [autoMonitorError, setAutoMonitorError] = useState('');
    const [autoMonitorLoading, setAutoMonitorLoading] = useState(false);
    const autoMonitorPollRef = useRef(null);
    const [smartMoney, setSmartMoney] = useState(null);
    const [smartMoneyError, setSmartMoneyError] = useState('');
    const [smartMoneyLoading, setSmartMoneyLoading] = useState(false);
    const smartMoneyPollRef = useRef(null);
    const [autoGuardBaselineUpdating, setAutoGuardBaselineUpdating] = useState(false);

    const [autoPnLCurve, setAutoPnLCurve] = useState(null);
    const [autoPnLCurveError, setAutoPnLCurveError] = useState('');
    const [autoPnLCurveLoading, setAutoPnLCurveLoading] = useState(false);
    const autoPnLCurvePollRef = useRef(null);

    const [hotPoolsSort, setHotPoolsSort] = useState('fees');
    const [hotPoolsData, setHotPoolsData] = useState(null);
    const [hotPoolsError, setHotPoolsError] = useState('');
    const [hotPoolsLoading, setHotPoolsLoading] = useState(false);
    const hotPoolsPollRef = useRef(null);
    const [hotPoolsFilterOpen, setHotPoolsFilterOpen] = useState(false);
    const [hotPoolsFilter, setHotPoolsFilter] = useState(() => {
        const saved = storage.get(STORAGE_HOT_POOLS_FILTER);
        if (!saved) return defaultHotPoolsFilter;
        try {
            return normalizeHotPoolsFilter(JSON.parse(saved));
        } catch {
            return defaultHotPoolsFilter;
        }
    });
    const [hotPoolsFilterDraft, setHotPoolsFilterDraft] = useState(() => ({
        enabled: defaultHotPoolsFilter.enabled,
        keyword: String(defaultHotPoolsFilter.keyword || ''),
        minFees: String(defaultHotPoolsFilter.minFees),
        minFeeRate: String(defaultHotPoolsFilter.minFeeRate),
        minTvl: String(defaultHotPoolsFilter.minTvl),
        minVolume: String(defaultHotPoolsFilter.minVolume),
    }));

    const [poolSearchOpen, setPoolSearchOpen] = useState(false);
    const [poolSearchChain, setPoolSearchChain] = useState('bsc');
    const [poolSearchQuery, setPoolSearchQuery] = useState('');
    const [poolSearchResults, setPoolSearchResults] = useState([]);
    const [poolSearchPerformed, setPoolSearchPerformed] = useState(false);
    const [poolSearchError, setPoolSearchError] = useState('');
    const [poolSearchLoading, setPoolSearchLoading] = useState(false);
    const poolSearchInputRef = useRef(null);
    const poolSearchControllerRef = useRef(null);
    // 保存上一次热门池子数据，用于计算变化
    const previousHotPoolsDataRef = useRef({});
    const [klinePool, setKlinePool] = useState(null);
    const [openPositionPool, setOpenPositionPool] = useState(null);
    const [openPositionAmount, setOpenPositionAmount] = useState('');
    const [openPositionRangeLower, setOpenPositionRangeLower] = useState('');
    const [openPositionRangeUpper, setOpenPositionRangeUpper] = useState('');
    const [openPositionSlippage, setOpenPositionSlippage] = useState('');
    const [openPositionAllowSwap, setOpenPositionAllowSwap] = useState(false);
    const [openPositionError, setOpenPositionError] = useState('');
    const [openPositionLoading, setOpenPositionLoading] = useState(false);
    const [operationProgress, setOperationProgress] = useState(null);
    const [openPositionSmartWallets, setOpenPositionSmartWallets] = useState([]);
    const [openPositionSmartWalletsLoading, setOpenPositionSmartWalletsLoading] = useState(false);
    const [walletsData, setWalletsData] = useState(null);
    const [walletsError, setWalletsError] = useState('');
    const [walletsLoading, setWalletsLoading] = useState(false);
    const [openPositionWalletId, setOpenPositionWalletId] = useState(() => storage.get(STORAGE_OPEN_POSITION_WALLET_ID) || '');

    const [taskRangeEdit, setTaskRangeEdit] = useState(null);
    const [taskRangeLower, setTaskRangeLower] = useState('');
    const [taskRangeUpper, setTaskRangeUpper] = useState('');
    const [taskRangeAmount, setTaskRangeAmount] = useState('');
    const [taskRangeError, setTaskRangeError] = useState('');
    const [taskRangeLoading, setTaskRangeLoading] = useState(false);

    // 黑名单状态
    const [blacklist, setBlacklist] = useState(new Set());
    // 冷却列表状态
    const [cooldowns, setCooldowns] = useState([]);
    const [cooldownRemovingPair, setCooldownRemovingPair] = useState('');

    const [adminUsers, setAdminUsers] = useState([]);
    const [adminUsersError, setAdminUsersError] = useState('');
    const [adminUsersLoading, setAdminUsersLoading] = useState(false);
    const [adminSelectedUserId, setAdminSelectedUserId] = useState(null);
    const [adminPositions, setAdminPositions] = useState(null);
    const [adminPositionsError, setAdminPositionsError] = useState('');
    const [adminPositionsLoading, setAdminPositionsLoading] = useState(false);
    const [adminAutoStats, setAdminAutoStats] = useState(null);
    const [adminAutoStatsError, setAdminAutoStatsError] = useState('');
    const [adminAutoStatsLoading, setAdminAutoStatsLoading] = useState(false);
    const [adminDisableError, setAdminDisableError] = useState('');
    const [adminDisableLoading, setAdminDisableLoading] = useState(false);
    const [adminDisableResult, setAdminDisableResult] = useState(null);
    const adminUsersPollRef = useRef(null);
    const adminPositionsPollRef = useRef(null);
    const adminAutoStatsPollRef = useRef(null);
    const adminSelectedRef = useRef(null);
    const confirmResolveRef = useRef(null);
    const noticeTimerRef = useRef(null);

    const [theme, setTheme] = useState('dark');
    const [accentTheme, setAccentTheme] = useState(() => normalizeAccentTheme(storage.get(STORAGE_ACCENT_THEME)));
    const [settingsOpen, setSettingsOpen] = useState(false);
    const [pollOverrideSec, setPollOverrideSec] = useState(null);
    const [pollDraftSec, setPollDraftSec] = useState('');
    const [confirmState, setConfirmState] = useState(null);
    const [notice, setNotice] = useState(null);
    const [blacklistPrompt, setBlacklistPrompt] = useState(null);
    const [blacklistPromptLoading, setBlacklistPromptLoading] = useState(false);
    const [globalConfigOpen, setGlobalConfigOpen] = useState(false);
    const [globalConfig, setGlobalConfig] = useState(null);
    const [globalConfigError, setGlobalConfigError] = useState('');
    const [globalConfigLoading, setGlobalConfigLoading] = useState(false);

    const [isDesktopWebMode, setIsDesktopWebMode] = useState(() => {
        if (typeof window === 'undefined') return false;
        return window.matchMedia('(min-width: 1024px)').matches;
    });
    const [webWorkbenchWidgets, setWebWorkbenchWidgets] = useState(() => {
        const saved = storage.get(STORAGE_WEB_WORKBENCH_WIDGETS);
        if (!saved) return [...DEFAULT_WEB_WORKBENCH_WIDGETS];
        try {
            return normalizeWebWorkbenchWidgets(JSON.parse(saved));
        } catch {
            return [...DEFAULT_WEB_WORKBENCH_WIDGETS];
        }
    });
    const [webWorkbenchGmgnPool, setWebWorkbenchGmgnPool] = useState(null);

    const multiChainEnabled = globalConfig?.multi_chain_enabled ?? true;
    const multiWalletEnabled = globalConfig?.multi_wallet_enabled ?? false;
    const [posWalletBalances, setPosWalletBalances] = useState(null);
    const userDefaultChain = useMemo(() => {
        const raw = String(globalConfig?.default_chain || 'bsc').trim().toLowerCase();
        if (raw === 'base' || raw === 'bsc') return raw;
        return 'bsc';
    }, [globalConfig?.default_chain]);

    // Single-chain mode: lock pool search chain to default chain.
    useEffect(() => {
        if (!multiChainEnabled) {
            setPoolSearchChain(userDefaultChain);
        }
    }, [multiChainEnabled, userDefaultChain]);

    // 加载进度状态
    const [pollProgress, setPollProgress] = useState(0);
    const pollProgressRef = useRef(null);
    const lastPollTimeRef = useRef(Date.now());
    const brand = useMemo(() => getBrandTheme(accentTheme), [accentTheme]);

    // 批量操作状态
    const [batchMode, setBatchMode] = useState(false);
    const [selectedTaskIds, setSelectedTaskIds] = useState(new Set());
    const [batchLoading, setBatchLoading] = useState(false);

    const serverPollIntervalSec = Math.max(1, Number(data?.poll_interval_sec || adminPositions?.poll_interval_sec || 1));
    const pollIntervalSec = Math.max(1, Number(pollOverrideSec || serverPollIntervalSec || 1));
    const adminListPollSec = Math.max(3, pollIntervalSec);
    const adminStatsPollSec = Math.max(5, pollIntervalSec * 2);
    const isAdmin = Boolean(me?.is_admin || data?.is_admin || adminPositions?.is_admin);
    const smartMoneyEnabled = Boolean(me?.smart_money_enabled || data?.smart_money_enabled || isAdmin);
    const showAdmin = isAdmin && viewMode === 'admin';
    const isHotPools = viewMode === 'hot_pools';
    const isMonitor = viewMode === 'monitor';
    const isPositions = viewMode === 'positions';
    const isSmartMoney = viewMode === 'smart_money';
    const topNavItems = useMemo(
        () => buildTopNavItems({ isAdmin, smartMoneyEnabled }),
        [isAdmin, smartMoneyEnabled],
    );
    const showWalletSummaryCard = !showAdmin && !isHotPools && !isSmartMoney;
    const hotPoolsDefaultPollSec = 10;
    const hotPoolsPollIntervalSec = Math.max(5, Number(pollOverrideSec || hotPoolsDefaultPollSec));
    const settingsPollIntervalSec = isHotPools ? hotPoolsPollIntervalSec : pollIntervalSec;
    const settingsServerPollIntervalSec = isHotPools ? hotPoolsDefaultPollSec : serverPollIntervalSec;
    const monitorPollSec = Math.max(3, pollIntervalSec);
    const autoPnLCurvePollSec = 15;
    const smartMoneyPollSec = 60;
    const smartMoneyPoolsWindowHours = 24;
    const smartMoneyPnLWindowHours = 24;

    const adminSelectedUser = useMemo(() => {
        if (!adminSelectedUserId) return null;
        return adminUsers.find((u) => Number(u?.user_id) === Number(adminSelectedUserId)) || null;
    }, [adminUsers, adminSelectedUserId]);

    const activeData = showAdmin ? adminPositions : data;
    const updatedAt = activeData?.updated_at;

    const walletAddress = activeData?.wallet?.address || '';
    const bnbBalance = activeData?.wallet?.bnb_balance || '0.000000';
    const bnbUsd = activeData?.wallet?.bnb_usd;
    const summary = activeData?.summary;
    const positions = activeData?.positions || [];

    const activeError = showAdmin ? adminPositionsError : error;
    const activeLoading = showAdmin ? adminPositionsLoading : loading;

    const walletUsdFromTokens = useMemo(() => {
        const byAddr = new Map();
        for (const p of positions) {
            const rows = p?.token_rows || [];
            for (const row of rows) {
                const addr = String(row?.address || '').trim().toLowerCase();
                if (!addr) continue;
                const usd = Number(row?.wallet_usd || 0);
                if (!Number.isFinite(usd)) continue;
                const prev = byAddr.get(addr);
                if (prev === undefined || usd > prev) byAddr.set(addr, usd);
            }
        }
        let sum = 0;
        for (const v of byAddr.values()) sum += v;
        return sum;
    }, [positions]);

    const totalsFromPositions = useMemo(() => {
        let positionUsd = 0;
        let feeUsd = 0;
        for (const p of positions) {
            positionUsd += Number(p?.totals?.position_usd || 0);
            feeUsd += Number(p?.totals?.fee_usd || 0);
        }
        return { positionUsd, feeUsd };
    }, [positions]);

    const totalUsd = useMemo(() => {
        // Multi-wallet: sum all wallets' stable balance + positions + fees
        const multiWallets = Array.isArray(posWalletBalances?.wallets) && posWalletBalances.wallets.length > 1;
        if (multiWallets) {
            const allWalletsUsd = posWalletBalances.wallets.reduce(
                (s, w) => s + Number(w.stable_balance === 'N/A' ? 0 : w.stable_balance || 0), 0
            );
            return allWalletsUsd + totalsFromPositions.positionUsd + totalsFromPositions.feeUsd;
        }
        const server = typeof summary?.total_usd === 'number' ? summary.total_usd : null;
        const walletUsd = walletUsdFromTokens + (typeof bnbUsd === 'number' ? bnbUsd : 0);
        const computed = walletUsd + totalsFromPositions.positionUsd + totalsFromPositions.feeUsd;
        if (server !== null && server > 0) return server;
        if (computed > 0) return computed;
        return server ?? computed;
    }, [summary?.total_usd, walletUsdFromTokens, bnbUsd, totalsFromPositions.positionUsd, totalsFromPositions.feeUsd, posWalletBalances]);

    const multiWalletSummary = Array.isArray(posWalletBalances?.wallets) && posWalletBalances.wallets.length > 1;

    const singleWalletUsd = useMemo(() => {
        const serverWalletUsd = Number(summary?.wallet_usd);
        if (Number.isFinite(serverWalletUsd) && serverWalletUsd >= 0) return serverWalletUsd;
        return walletUsdFromTokens + (typeof bnbUsd === 'number' ? bnbUsd : 0);
    }, [summary?.wallet_usd, walletUsdFromTokens, bnbUsd]);

    const walletSummaryCards = useMemo(() => {
        if (multiWalletSummary) {
            return posWalletBalances.wallets.slice(0, 3).map((w, idx) => ({
                key: String(w?.id || w?.address || idx),
                label: w?.name || `${String(w?.address || '').slice(0, 6)}..${String(w?.address || '').slice(-4)}`,
                value: w?.stable_balance !== 'N/A' ? formatUsd(w.stable_balance) : '$--',
                detail: String(w?.address || '').trim(),
            }));
        }

        const singleWalletValue =
            Array.isArray(posWalletBalances?.wallets) && posWalletBalances.wallets.length === 1
                ? (posWalletBalances.wallets[0]?.stable_balance !== 'N/A'
                    ? formatUsd(posWalletBalances.wallets[0]?.stable_balance)
                    : formatUsd(singleWalletUsd))
                : formatUsd(singleWalletUsd);

        return [
            {
                key: 'wallet',
                label: '钱包',
                value: singleWalletValue,
                detail: walletAddress ? `${walletAddress.slice(0, 6)}...${walletAddress.slice(-4)}` : '未连接',
            },
        ];
    }, [multiWalletSummary, posWalletBalances, singleWalletUsd, walletAddress]);

    const summaryMetricCards = useMemo(() => ([
        ...walletSummaryCards,
        {
            key: 'position',
            label: '仓位',
            value: formatUsd(totalsFromPositions.positionUsd),
            detail: '',
        },
        {
            key: 'fee',
            label: '手续费',
            value: formatUsd(totalsFromPositions.feeUsd),
            detail: '',
        },
    ]), [walletSummaryCards, totalsFromPositions.positionUsd, totalsFromPositions.feeUsd]);

    const summaryMetricDense = summaryMetricCards.length >= 5;
    const totalWalletCount = Array.isArray(posWalletBalances?.wallets) ? posWalletBalances.wallets.length : walletSummaryCards.length;

    const visiblePositions = useMemo(() => {
        return positions.filter((p) => {
            if (p?.has_liquidity !== false) return true;
            const taskId = Number(p?.task_id || 0);
            if (!Number.isFinite(taskId) || taskId <= 0) return false;
            const label = String(p?.status_label || '');
            return (
                label.includes('再平衡') ||
                label.includes('撤出') ||
                label.includes('停止中') ||
                label.includes('止损') ||
                label.includes('等待')
            );
        });
    }, [positions]);

    const visibleTaskPositions = useMemo(() => {
        if (positionsTaskTab === 'all') return visiblePositions;
        const wantAuto = positionsTaskTab === 'auto';
        return visiblePositions.filter((p) => {
            const taskId = Number(p?.task_id || 0);
            if (!Number.isFinite(taskId) || taskId <= 0) return false;
            const isAuto = Boolean(p?.task_is_auto);
            return wantAuto ? isAuto : !isAuto;
        });
    }, [positionsTaskTab, visiblePositions]);

    // 智能切换实时仓位Tab：根据当前任务类型自动选择
    const hasAutoTasks = useMemo(() => {
        return visiblePositions.some((p) => {
            const taskId = Number(p?.task_id || 0);
            return Number.isFinite(taskId) && taskId > 0 && Boolean(p?.task_is_auto);
        });
    }, [visiblePositions]);

    const hasManualTasks = useMemo(() => {
        return visiblePositions.some((p) => {
            const taskId = Number(p?.task_id || 0);
            return Number.isFinite(taskId) && taskId > 0 && !Boolean(p?.task_is_auto);
        });
    }, [visiblePositions]);

    const positionsTabTouchedRef = useRef(false);
    useEffect(() => {
        if (!isPositions || showAdmin) return;
        positionsTabTouchedRef.current = false;
    }, [isPositions, showAdmin]);

    // 智能设置实时仓位 Tab：默认按任务类型自动选择（用户手动切换后不强制覆盖）
    useEffect(() => {
        if (!isPositions || showAdmin) return;
        if (visiblePositions.length === 0) return;

        const desiredTab = hasAutoTasks && hasManualTasks
            ? 'all'
            : hasAutoTasks
                ? 'auto'
                : hasManualTasks
                    ? 'manual'
                    : 'all';

        if (!positionsTabTouchedRef.current) {
            if (positionsTaskTab !== desiredTab) {
                setPositionsTaskTab(desiredTab);
                setSelectedTaskIds(new Set());
                setBatchMode(false);
            }
            return;
        }

        // 用户已手动选择 Tab：仅在当前 Tab 已无任务时自动兜底跳转，避免空页面。
        if (positionsTaskTab === 'auto' && !hasAutoTasks && hasManualTasks) {
            setPositionsTaskTab('manual');
            setSelectedTaskIds(new Set());
            setBatchMode(false);
        } else if (positionsTaskTab === 'manual' && !hasManualTasks && hasAutoTasks) {
            setPositionsTaskTab('auto');
            setSelectedTaskIds(new Set());
            setBatchMode(false);
        }
    }, [isPositions, showAdmin, visiblePositions.length, hasAutoTasks, hasManualTasks, positionsTaskTab]);

    // 从仓位构建 pool_address -> position_usd 映射（用于在热门池子上显示持仓标签）
    const positionsPoolMap = useMemo(() => {
        const map = new Map();
        for (const p of positions) {
            const poolId = String(p?.pool_id || '').toLowerCase();
            if (!poolId) continue;
            const positionUsd = Number(p?.totals?.position_usd || 0) + Number(p?.totals?.fee_usd || 0);
            const existing = map.get(poolId) || 0;
            map.set(poolId, existing + positionUsd);
        }
        return map;
    }, [positions]);

    // 获取用户仓位中的池子地址列表（用于传给 hot_pools API）
    const positionsPoolAddresses = useMemo(() => {
        return Array.from(positionsPoolMap.keys());
    }, [positionsPoolMap]);

    const hotPoolsRows = useMemo(() => {
        return Array.isArray(hotPoolsData?.data) ? hotPoolsData.data : [];
    }, [hotPoolsData]);

    const hotPoolsFilterEnabled = useMemo(() => {
        if (!hotPoolsFilter.enabled) return false;
        const hasKeyword = String(hotPoolsFilter.keyword || '').trim().length > 0;
        const hasNumbers = [hotPoolsFilter.minFees, hotPoolsFilter.minFeeRate, hotPoolsFilter.minTvl, hotPoolsFilter.minVolume].some((v) => Number.isFinite(v));
        return hasKeyword || hasNumbers;
    }, [hotPoolsFilter]);

    const hotPoolsVisibleRows = useMemo(() => {
        // 1. 先进行现有筛选
        let filtered = hotPoolsRows;
        if (hotPoolsFilterEnabled) {
            const minFees = hotPoolsFilter.minFees;
            const minFeeRate = hotPoolsFilter.minFeeRate;
            const minTvl = hotPoolsFilter.minTvl;
            const minVolume = hotPoolsFilter.minVolume;
            const keyword = String(hotPoolsFilter.keyword || '').trim().toLowerCase();
            filtered = hotPoolsRows.filter((row) => {
                const fees = parseMetricNumber(row?.total_fees);
                const feeRate = parseMetricNumber(row?.fee_rate);
                const tvl = parseMetricNumber(row?.current_pool_value);
                const volume = parseMetricNumber(row?.total_volume);
                // 如果用户有仓位在这个池子，跳过筛选（始终显示）
                const poolAddr = String(row?.pool_address || '').toLowerCase();
                if (positionsPoolMap.has(poolAddr)) return true;
                if (keyword) {
                    const pair = String(row?.trading_pair || '').toLowerCase();
                    const addr = String(row?.pool_address || '').toLowerCase();
                    const t0 = String(row?.token0_address || '').toLowerCase();
                    const t1 = String(row?.token1_address || '').toLowerCase();
                    const hit = pair.includes(keyword) || addr.includes(keyword) || t0.includes(keyword) || t1.includes(keyword);
                    if (!hit) return false;
                }
                if (Number.isFinite(minFees) && fees < minFees) return false;
                if (Number.isFinite(minFeeRate) && feeRate < minFeeRate) return false;
                if (Number.isFinite(minTvl) && tvl < minTvl) return false;
                if (Number.isFinite(minVolume) && volume < minVolume) return false;
                return true;
            });
        }

        // 2. 为每个池子添加 userPositionUsd 字段
        const enriched = filtered.map(pool => {
            const addr = String(pool?.pool_address || '').toLowerCase();
            return {
                ...pool,
                userPositionUsd: positionsPoolMap.get(addr) || 0
            };
        });

        // 3. 排序：有仓位的置顶，按仓位金额降序；其余保持原顺序
        return enriched.sort((a, b) => {
            if (a.userPositionUsd > 0 && b.userPositionUsd <= 0) return -1;
            if (b.userPositionUsd > 0 && a.userPositionUsd <= 0) return 1;
            if (a.userPositionUsd > 0 && b.userPositionUsd > 0) {
                return b.userPositionUsd - a.userPositionUsd;
            }
            return 0; // 保持原顺序
        });
    }, [hotPoolsFilter, hotPoolsFilterEnabled, hotPoolsRows, positionsPoolMap]);

    // 构建热门池子的历史数据映射 (protocol_version:pool_address -> previous data)
    const previousHotPoolsMap = useMemo(() => {
        return previousHotPoolsDataRef.current;
    }, [hotPoolsRows]);

    const apiBaseUrl = useMemo(() => resolveApiBaseUrl(), []);
    const allowEmptyInitData = useMemo(() => resolveAllowEmptyInitData(), []);
    const hasInitData = Boolean(initData) || allowEmptyInitData;

    const requestConfirm = (options) => new Promise((resolve) => {
        confirmResolveRef.current = resolve;
        setConfirmState({
            title: options?.title || '确认操作',
            message: options?.message || '',
            confirmText: options?.confirmText || '确认',
            cancelText: options?.cancelText || '取消',
            tone: options?.tone || 'primary',
        });
    });

    const closeConfirm = (result) => {
        const resolve = confirmResolveRef.current;
        confirmResolveRef.current = null;
        setConfirmState(null);
        if (typeof resolve === 'function') resolve(result);
    };

    const showNotice = (message, tone = 'info') => {
        const text = String(message || '').trim();
        if (!text) return;
        setNotice({ message: text, tone });
        if (noticeTimerRef.current) clearTimeout(noticeTimerRef.current);
        noticeTimerRef.current = setTimeout(() => setNotice(null), 3200);
    };

    const guardCompareToPeak = autoMonitor?.config?.guard_compare_to_peak !== false;
    const toggleAutoGuardBaseline = async () => {
        if (!hasInitData) {
            showNotice('未获取到 initData，无法修改对比基准。', 'error');
            return;
        }
        if (autoGuardBaselineUpdating) return;
        const next = !guardCompareToPeak;
        setAutoGuardBaselineUpdating(true);
        try {
            await setAutoLPGuardCompareToPeak({ apiBaseUrl, initData, guardCompareToPeak: next });
            hapticSelection();
            showNotice(`撤退对比基准已切换：${next ? '最高点' : '开仓时'}`, 'success');
            try {
                const resp = await fetchAutoMonitor({ apiBaseUrl, initData });
                setAutoMonitor(resp);
            } catch {
                // ignore; polling will update soon
                setAutoMonitor((prev) =>
                    prev
                        ? {
                            ...prev,
                            config: {
                                ...(prev?.config || {}),
                                guard_compare_to_peak: next,
                            },
                        }
                        : prev,
                );
            }
        } catch (e) {
            hapticNotification('error');
            showNotice(`切换失败：${String(e?.message || e)}`, 'error');
        } finally {
            setAutoGuardBaselineUpdating(false);
        }
    };

    useEffect(() => {
        if (!hasInitData) return;
        let aborted = false;
        const controller = new AbortController();

        const run = async () => {
            try {
                const resp = await fetchMe({ apiBaseUrl, initData, signal: controller.signal });
                if (aborted) return;
                setMe(resp);
            } catch {
                // ignore; fallback to `realtime_positions` response
            }
        };

        run();

        return () => {
            aborted = true;
            controller.abort();
        };
    }, [apiBaseUrl, initData, hasInitData]);

    useEffect(() => {
        if (!isAdmin && viewMode === 'admin') setViewMode('positions');
        if (!smartMoneyEnabled && viewMode === 'smart_money') setViewMode('hot_pools');
    }, [isAdmin, smartMoneyEnabled, viewMode]);

    useEffect(() => {
        const tg = getTelegramWebApp();
        const savedTheme = storage.get(STORAGE_THEME);
        if (savedTheme === 'light' || savedTheme === 'dark') {
            setTheme(savedTheme);
        } else {
            // 默认使用暗色主题
            setTheme('dark');
        }

        const savedPoll = Number(storage.get(STORAGE_POLL_SEC));
        if (Number.isFinite(savedPoll) && savedPoll >= 5) {
            setPollOverrideSec(Math.floor(savedPoll));
        }
        setAccentTheme(normalizeAccentTheme(storage.get(STORAGE_ACCENT_THEME)));
    }, []);

    useEffect(() => {
        const isDark = theme === 'dark';
        document.documentElement.classList.toggle('dark', isDark);
        storage.set(STORAGE_THEME, isDark ? 'dark' : 'light');
        storage.set(STORAGE_ACCENT_THEME, accentTheme);

        const tg = getTelegramWebApp();
        try {
            tg?.setHeaderColor?.(isDark ? '#0b0f14' : '#fafafa');
            tg?.setBackgroundColor?.(isDark ? '#0b0f14' : '#fafafa');
        } catch {
            // ignore
        }
    }, [accentTheme, theme]);

    useEffect(() => {
        return () => {
            if (noticeTimerRef.current) clearTimeout(noticeTimerRef.current);
        };
    }, []);

    // 进度条计时器 - 显示轮询倒计时
    useEffect(() => {
        const currentPollSec = isHotPools ? hotPoolsPollIntervalSec : pollIntervalSec;

        const updateProgress = () => {
            const elapsed = Date.now() - lastPollTimeRef.current;
            const progress = Math.min(100, (elapsed / (currentPollSec * 1000)) * 100);
            setPollProgress(progress);
        };

        // 立即更新一次
        updateProgress();

        // 每100ms更新进度
        pollProgressRef.current = setInterval(updateProgress, 100);

        return () => {
            if (pollProgressRef.current) clearInterval(pollProgressRef.current);
        };
    }, [isHotPools, hotPoolsPollIntervalSec, pollIntervalSec]);

    // 轮询完成时重置进度
    const lastUpdatedAtRef = useRef(null);
    useEffect(() => {
        // 使用 updatedAt 来判断数据是否真正更新了
        const currentUpdatedAt = data?.updated_at || hotPoolsData?.updated_at;
        if (currentUpdatedAt && currentUpdatedAt !== lastUpdatedAtRef.current) {
            lastPollTimeRef.current = Date.now();
            setPollProgress(0);
            // 只在真正有新数据时触发触觉反馈
            if (lastUpdatedAtRef.current !== null) {
                hapticSelection();
            }
            lastUpdatedAtRef.current = currentUpdatedAt;
        }
    }, [data?.updated_at, hotPoolsData?.updated_at]);

    useEffect(() => {
        if (!settingsOpen) return;
        setPollDraftSec(pollOverrideSec ? String(pollOverrideSec) : '');
    }, [settingsOpen, pollOverrideSec]);

    useEffect(() => {
        if (!hotPoolsFilterOpen) return;
        setHotPoolsFilterDraft({
            enabled: hotPoolsFilter.enabled,
            keyword: String(hotPoolsFilter.keyword || ''),
            minFees: formatDraftNumber(hotPoolsFilter.minFees),
            minFeeRate: formatDraftNumber(hotPoolsFilter.minFeeRate),
            minTvl: formatDraftNumber(hotPoolsFilter.minTvl),
            minVolume: formatDraftNumber(hotPoolsFilter.minVolume),
        });
    }, [hotPoolsFilterOpen, hotPoolsFilter]);

    useEffect(() => {
        if (!hasInitData) return;
        let aborted = false;
        const controller = new AbortController();
        let inFlight = false;

        const run = async () => {
            if (inFlight) return;
            inFlight = true;
            setLoading(true);
            setError('');
            try {
                const resp = await fetchRealtimePositions({ apiBaseUrl, initData, signal: controller.signal });
                if (aborted) return;
                setData(resp);
            } catch (e) {
                if (aborted) return;
                setError(String(e?.message || e));
            } finally {
                inFlight = false;
                if (!aborted) setLoading(false);
            }
        };

        run();

        if (pollRef.current) clearInterval(pollRef.current);
        pollRef.current = setInterval(run, pollIntervalSec * 1000);

        return () => {
            aborted = true;
            controller.abort();
            if (pollRef.current) clearInterval(pollRef.current);
        };
    }, [apiBaseUrl, initData, hasInitData, pollIntervalSec]);

    // Fetch per-wallet balances for multi-wallet display
    useEffect(() => {
        if (!hasInitData || !multiWalletEnabled) {
            setPosWalletBalances(null);
            return;
        }
        let aborted = false;
        const controller = new AbortController();
        const run = () => {
            fetchWallets({ apiBaseUrl, initData, chain: userDefaultChain, signal: controller.signal })
                .then((resp) => { if (!aborted) setPosWalletBalances(resp || null); })
                .catch(() => { if (!aborted) setPosWalletBalances(null); });
        };
        run();
        const timer = setInterval(run, Math.max(pollIntervalSec * 1000, 30000));
        return () => { aborted = true; controller.abort(); clearInterval(timer); };
    }, [apiBaseUrl, initData, hasInitData, multiWalletEnabled, userDefaultChain, pollIntervalSec]);

    useEffect(() => {
        if (!hasInitData || showAdmin || !isMonitor) return;
        let aborted = false;
        const controller = new AbortController();
        let inFlight = false;

        const run = async () => {
            if (inFlight) return;
            inFlight = true;
            setAutoMonitorLoading(true);
            setAutoMonitorError('');
            try {
                const resp = await fetchAutoMonitor({ apiBaseUrl, initData, signal: controller.signal });
                if (aborted) return;
                setAutoMonitor(resp);
            } catch (e) {
                if (aborted) return;
                setAutoMonitorError(String(e?.message || e));
            } finally {
                inFlight = false;
                if (!aborted) setAutoMonitorLoading(false);
            }
        };

        run();

        if (autoMonitorPollRef.current) clearInterval(autoMonitorPollRef.current);
        autoMonitorPollRef.current = setInterval(run, monitorPollSec * 1000);

        return () => {
            aborted = true;
            controller.abort();
            if (autoMonitorPollRef.current) clearInterval(autoMonitorPollRef.current);
        };
    }, [apiBaseUrl, initData, hasInitData, showAdmin, isMonitor, monitorPollSec]);

    useEffect(() => {
        if (!hasInitData || showAdmin || !isPositions || positionsTaskTab !== 'auto') return;
        let aborted = false;
        const controller = new AbortController();
        let inFlight = false;

        const run = async () => {
            if (inFlight) return;
            inFlight = true;
            setAutoPnLCurveLoading(true);
            setAutoPnLCurveError('');
            try {
                const resp = await fetchAutoLPPnLCurve({ apiBaseUrl, initData, signal: controller.signal });
                if (aborted) return;
                setAutoPnLCurve(resp);
            } catch (e) {
                if (aborted) return;
                setAutoPnLCurveError(String(e?.message || e));
            } finally {
                inFlight = false;
                if (!aborted) setAutoPnLCurveLoading(false);
            }
        };

        run();

        if (autoPnLCurvePollRef.current) clearInterval(autoPnLCurvePollRef.current);
        autoPnLCurvePollRef.current = setInterval(run, autoPnLCurvePollSec * 1000);

        return () => {
            aborted = true;
            controller.abort();
            if (autoPnLCurvePollRef.current) clearInterval(autoPnLCurvePollRef.current);
        };
    }, [apiBaseUrl, initData, hasInitData, showAdmin, isPositions, positionsTaskTab, autoPnLCurvePollSec]);

    useEffect(() => {
        if (!hasInitData || !showAdmin) return;
        let aborted = false;
        const controller = new AbortController();
        let inFlight = false;

        const run = async () => {
            if (inFlight) return;
            inFlight = true;
            setAdminUsersLoading(true);
            setAdminUsersError('');
            try {
                const resp = await fetchAdminRealtimeUsers({
                    apiBaseUrl,
                    initData,
                    limit: 50,
                    signal: controller.signal,
                });
                if (aborted) return;
                const users = Array.isArray(resp?.users) ? resp.users : [];
                setAdminUsers(users);
            } catch (e) {
                if (aborted) return;
                setAdminUsersError(String(e?.message || e));
            } finally {
                inFlight = false;
                if (!aborted) setAdminUsersLoading(false);
            }
        };

        run();

        if (adminUsersPollRef.current) clearInterval(adminUsersPollRef.current);
        adminUsersPollRef.current = setInterval(run, adminListPollSec * 1000);

        return () => {
            aborted = true;
            controller.abort();
            if (adminUsersPollRef.current) clearInterval(adminUsersPollRef.current);
        };
    }, [apiBaseUrl, initData, hasInitData, showAdmin, adminListPollSec]);

    useEffect(() => {
        if (!showAdmin) return;
        if (adminSelectedUserId) return;
        if (!adminUsers.length) {
            setAdminPositions(null);
            setAdminAutoStats(null);
            return;
        }
        setAdminSelectedUserId(adminUsers[0].user_id);
    }, [showAdmin, adminUsers, adminSelectedUserId]);

    useEffect(() => {
        if (!hasInitData || !showAdmin || !adminSelectedUserId) return;
        let aborted = false;
        const controller = new AbortController();
        let inFlight = false;

        const selectedChanged = adminSelectedRef.current !== adminSelectedUserId;
        adminSelectedRef.current = adminSelectedUserId;
        if (selectedChanged) {
            setAdminPositions(null);
            setAdminPositionsError('');
            setAdminAutoStats(null);
            setAdminAutoStatsError('');
            setAdminDisableError('');
            setAdminDisableResult(null);
        }

        const run = async () => {
            if (inFlight) return;
            inFlight = true;
            setAdminPositionsLoading(true);
            setAdminPositionsError('');
            try {
                const resp = await fetchAdminRealtimePositions({
                    apiBaseUrl,
                    initData,
                    userId: adminSelectedUserId,
                    signal: controller.signal,
                });
                if (aborted) return;
                setAdminPositions(resp);
            } catch (e) {
                if (aborted) return;
                setAdminPositionsError(String(e?.message || e));
            } finally {
                inFlight = false;
                if (!aborted) setAdminPositionsLoading(false);
            }
        };

        run();

        if (adminPositionsPollRef.current) clearInterval(adminPositionsPollRef.current);
        adminPositionsPollRef.current = setInterval(run, pollIntervalSec * 1000);

        return () => {
            aborted = true;
            controller.abort();
            if (adminPositionsPollRef.current) clearInterval(adminPositionsPollRef.current);
        };
    }, [apiBaseUrl, initData, hasInitData, showAdmin, adminSelectedUserId, pollIntervalSec]);

    useEffect(() => {
        if (!hasInitData || !showAdmin || !adminSelectedUserId) return;
        let aborted = false;
        const controller = new AbortController();
        let inFlight = false;

        const run = async () => {
            if (inFlight) return;
            inFlight = true;
            setAdminAutoStatsLoading(true);
            setAdminAutoStatsError('');
            try {
                const resp = await fetchAdminAutoLPStats({
                    apiBaseUrl,
                    initData,
                    userId: adminSelectedUserId,
                    signal: controller.signal,
                });
                if (aborted) return;
                setAdminAutoStats(resp);
            } catch (e) {
                if (aborted) return;
                setAdminAutoStatsError(String(e?.message || e));
            } finally {
                inFlight = false;
                if (!aborted) setAdminAutoStatsLoading(false);
            }
        };

        run();

        if (adminAutoStatsPollRef.current) clearInterval(adminAutoStatsPollRef.current);
        adminAutoStatsPollRef.current = setInterval(run, adminStatsPollSec * 1000);

        return () => {
            aborted = true;
            controller.abort();
            if (adminAutoStatsPollRef.current) clearInterval(adminAutoStatsPollRef.current);
        };
    }, [apiBaseUrl, initData, hasInitData, showAdmin, adminSelectedUserId, adminStatsPollSec]);

    // 热门池子数据始终加载（预加载）
    useEffect(() => {
        let aborted = false;
        const controller = new AbortController();
        let inFlight = false;

        const run = async () => {
            if (!hasInitData) {
                setHotPoolsLoading(false);
                setHotPoolsError('');
                return;
            }
            if (inFlight) return;
            inFlight = true;
            setHotPoolsLoading(true);
            setHotPoolsError('');
            try {
                const resp = await fetchHotPools({
                    apiBaseUrl,
                    initData,
                    sort: hotPoolsSort,
                    chain: multiChainEnabled ? 'bsc' : userDefaultChain,
                    timeframeMinutes: 5,
                    limit: 20,
                    includePools: positionsPoolAddresses,
                    signal: controller.signal,
                });
                if (aborted) return;
                // 在更新数据之前，保存当前数据作为历史数据（使用 setState 回调避免闭包拿到旧数据）
                setHotPoolsData((prev) => {
                    if (prev?.data) {
                        const prevMap = {};
                        for (const pool of prev.data) {
                            const addr = String(pool?.pool_address || '').trim().toLowerCase();
                            if (!addr) continue;
                            const proto = String(pool?.protocol_version || '').trim();
                            const key = `${proto}:${addr}`;
                            prevMap[key] = pool;
                        }
                        previousHotPoolsDataRef.current = prevMap;
                    }
                    return resp;
                });
            } catch (e) {
                if (aborted) return;
                setHotPoolsError(String(e?.message || e));
            } finally {
                inFlight = false;
                if (!aborted) setHotPoolsLoading(false);
            }
        };

        run();

        if (hotPoolsPollRef.current) clearInterval(hotPoolsPollRef.current);
        hotPoolsPollRef.current = setInterval(run, hotPoolsPollIntervalSec * 1000);

        return () => {
            aborted = true;
            controller.abort();
            if (hotPoolsPollRef.current) clearInterval(hotPoolsPollRef.current);
        };
    }, [apiBaseUrl, initData, hasInitData, isHotPools, hotPoolsSort, hotPoolsPollIntervalSec, positionsPoolAddresses.join(','), multiChainEnabled, userDefaultChain]);

    useEffect(() => {
        if (!hasInitData || !isSmartMoney || !smartMoneyEnabled) {
            if (smartMoneyPollRef.current) clearInterval(smartMoneyPollRef.current);
            return;
        }
        let aborted = false;
        const controller = new AbortController();
        let inFlight = false;

        const run = async () => {
            if (inFlight) return;
            inFlight = true;
            setSmartMoneyLoading(true);
            setSmartMoneyError('');
            try {
                const resp = await fetchSmartMoneyOverview({
                    apiBaseUrl,
                    initData,
                    chain: 'bsc',
                    poolLimit: 10,
                    walletLimit: 50,
                    poolsWindowHours: smartMoneyPoolsWindowHours,
                    pnlWindowHours: smartMoneyPnLWindowHours,
                    signal: controller.signal,
                });
                if (aborted) return;
                setSmartMoney(resp);
            } catch (e) {
                if (aborted) return;
                setSmartMoneyError(String(e?.message || e));
            } finally {
                inFlight = false;
                if (!aborted) setSmartMoneyLoading(false);
            }
        };

        run();

        if (smartMoneyPollRef.current) clearInterval(smartMoneyPollRef.current);
        smartMoneyPollRef.current = setInterval(run, smartMoneyPollSec * 1000);

        return () => {
            aborted = true;
            controller.abort();
            if (smartMoneyPollRef.current) clearInterval(smartMoneyPollRef.current);
        };
    }, [
        apiBaseUrl,
        initData,
        hasInitData,
        isSmartMoney,
        smartMoneyEnabled,
        smartMoneyPollSec,
        smartMoneyPoolsWindowHours,
        smartMoneyPnLWindowHours,
    ]);

    const applyPollDraft = () => {
        const raw = String(pollDraftSec || '').trim();
        const m = raw.match(/\d+/);
        if (!m) return;
        const n = Number(m[0]);
        if (!Number.isFinite(n)) return;
        const v = Math.max(5, Math.min(300, Math.floor(n)));
        setPollOverrideSec(v);
        storage.set(STORAGE_POLL_SEC, String(v));
        setSettingsOpen(false);
    };

    const clearPollOverride = () => {
        setPollOverrideSec(null);
        setPollDraftSec('');
        storage.remove(STORAGE_POLL_SEC);
        setSettingsOpen(false);
    };

    const setQuickPoll = (sec) => {
        const v = Math.max(5, Math.min(300, Math.floor(Number(sec) || 5)));
        setPollOverrideSec(v);
        storage.set(STORAGE_POLL_SEC, String(v));
        setPollDraftSec(String(v));
        setSettingsOpen(false);
    };

    const applyHotPoolsFilter = () => {
        const keyword = String(hotPoolsFilterDraft.keyword || '').trim();
        const next = normalizeHotPoolsFilter({
            enabled: true,
            keyword,
            minFees: parseDraftNumber(hotPoolsFilterDraft.minFees),
            minFeeRate: parseDraftNumber(hotPoolsFilterDraft.minFeeRate),
            minTvl: parseDraftNumber(hotPoolsFilterDraft.minTvl),
            minVolume: parseDraftNumber(hotPoolsFilterDraft.minVolume),
        });
        setHotPoolsFilter(next);
        storage.set(STORAGE_HOT_POOLS_FILTER, JSON.stringify(next));
        setHotPoolsFilterOpen(false);
    };

    const resetHotPoolsFilter = () => {
        setHotPoolsFilter(defaultHotPoolsFilter);
        storage.set(STORAGE_HOT_POOLS_FILTER, JSON.stringify(defaultHotPoolsFilter));
        setHotPoolsFilterOpen(false);
    };

    const clearHotPoolsFilter = () => {
        const cleared = normalizeHotPoolsFilter({
            enabled: false,
            keyword: '',
            minFees: null,
            minFeeRate: null,
            minTvl: null,
            minVolume: null,
        });
        setHotPoolsFilter(cleared);
        storage.set(STORAGE_HOT_POOLS_FILTER, JSON.stringify(cleared));
        setHotPoolsFilterOpen(false);
    };

    const openPoolSearch = () => {
        setHotPoolsFilterOpen(false);
        setPoolSearchOpen(true);
        setPoolSearchQuery('');
        setPoolSearchResults([]);
        setPoolSearchError('');
        setPoolSearchPerformed(false);
        hapticImpact('light');
        setTimeout(() => poolSearchInputRef.current?.focus?.(), 50);
    };

    const closePoolSearch = () => {
        if (poolSearchControllerRef.current) {
            try {
                poolSearchControllerRef.current.abort();
            } catch {
                // ignore
            }
            poolSearchControllerRef.current = null;
        }
        setPoolSearchOpen(false);
    };

    const runPoolSearch = async () => {
        if (poolSearchLoading) return;
        const keyword = String(poolSearchQuery || '').trim();
        if (!keyword) {
            setPoolSearchError('请输入池子ID或代币名称。');
            setPoolSearchResults([]);
            setPoolSearchPerformed(false);
            return;
        }
        if (!hasInitData) {
            setPoolSearchError('未获取到 Telegram initData，请从机器人入口打开页面。');
            return;
        }

        const controller = new AbortController();
        if (poolSearchControllerRef.current) {
            try {
                poolSearchControllerRef.current.abort();
            } catch {
                // ignore
            }
        }
        poolSearchControllerRef.current = controller;

        setPoolSearchLoading(true);
        setPoolSearchError('');
        setPoolSearchPerformed(true);
        try {
            const resp = await fetchSearchPools({
                apiBaseUrl,
                initData,
                q: keyword,
                chain: poolSearchChain,
                limit: 10,
                signal: controller.signal,
            });
            if (controller.signal.aborted) return;
            const rows = Array.isArray(resp?.data) ? resp.data : [];
            setPoolSearchResults(rows.slice(0, 10).map((p) => ({ ...p, chain: poolSearchChain })));
        } catch (e) {
            if (controller.signal.aborted) return;
            setPoolSearchResults([]);
            setPoolSearchError(String(e?.message || e));
        } finally {
            if (poolSearchControllerRef.current === controller) {
                poolSearchControllerRef.current = null;
            }
            setPoolSearchLoading(false);
        }
    };

    const selectPoolFromSearch = (pool) => {
        closePoolSearch();
        setTimeout(() => openPositionModal(pool), 0);
    };

    const toggleTheme = () => setTheme((t) => (t === 'dark' ? 'light' : 'dark'));

    const quickRangeOptions = [
        { label: '±1%', value: '±1' },
        { label: '±3%', value: '±3' },
        { label: '±5%', value: '±5' },
        { label: '±10%', value: '±10' },
        { label: '±20%', value: '±20' },
        { label: '±30%', value: '±30' },
    ];
    const smartMoneyQuickRangeOptions = useMemo(() => {
        const wallets = Array.isArray(openPositionSmartWallets) ? openPositionSmartWallets : [];
        if (!wallets.length) return [];

        const byRange = new Map();
        wallets.forEach((w) => {
            const lower = Number(w?.price_lower ?? 0);
            const upper = Number(w?.price_upper ?? 0);
            if (!Number.isFinite(lower) || !Number.isFinite(upper) || lower <= 0 || upper <= 0) return;

            const denom = Math.abs(lower) + Math.abs(upper);
            if (!Number.isFinite(denom) || denom <= 0) return;

            const pct = (Math.abs(upper - lower) / denom) * 100;
            if (!Number.isFinite(pct) || pct <= 0) return;

            const key = pct.toFixed(1);
            const prev = byRange.get(key);
            if (prev) {
                prev.count += 1;
                return;
            }
            byRange.set(key, { pct: Number(key), count: 1 });
        });

        return Array.from(byRange.values())
            .sort((a, b) => {
                if (a.count !== b.count) return b.count - a.count;
                return a.pct - b.pct;
            })
            .map((item) => ({
                label: `±${item.pct.toFixed(item.pct >= 10 ? 0 : 1)}%`,
                value: item.pct.toFixed(1),
                source: 'smart_money',
                count: item.count,
            }));
    }, [openPositionSmartWallets]);

    const effectiveQuickRangeOptions = useMemo(() => {
        if (smartMoneyQuickRangeOptions.length > 0) return smartMoneyQuickRangeOptions.slice(0, 6);
        return quickRangeOptions.slice(0, 6);
    }, [quickRangeOptions, smartMoneyQuickRangeOptions]);

    const parseRangeInput = (lowerRaw, upperRaw) => {
        const lower = Number(String(lowerRaw || '').trim());
        const upper = Number(String(upperRaw || '').trim());
        if (!Number.isFinite(lower) || !Number.isFinite(upper)) return null;
        return { lower: Math.abs(lower), upper: Math.abs(upper) };
    };

    const resetOpenPositionDraft = () => {
        setOpenPositionAmount('');
        setOpenPositionRangeLower('');
        setOpenPositionRangeUpper('');
        setOpenPositionSlippage('');

        setOpenPositionError('');
    };

    const openPositionModal = (pool) => {
        const addr = String(pool?.pool_address || '').trim().toLowerCase();
        if (addr && blacklist.has(addr)) {
            hapticNotification('error');
            showNotice('该池子已加入黑名单，不能开仓。', 'error');
            return;
        }
        let chain = String(pool?.chain || hotPoolsData?.chain || 'bsc').trim().toLowerCase() || 'bsc';
        if (!multiChainEnabled) chain = userDefaultChain;
        const smartWallets = Array.isArray(pool?.smartMoneyWallets) ? pool.smartMoneyWallets : [];
        const poolVersion = String(pool?.protocol_version || pool?.pool_version || '').trim().toLowerCase();
        setOpenPositionSmartWallets(smartWallets);
        setOpenPositionSmartWalletsLoading(false);
        setOpenPositionPool({
            ...pool,
            chain,
            ...(poolVersion ? { protocol_version: poolVersion, pool_version: poolVersion } : {}),
        });
        resetOpenPositionDraft();
    };

    const closeOpenPosition = () => {
        if (openPositionLoading) return;
        setOpenPositionSmartWallets([]);
        setOpenPositionSmartWalletsLoading(false);
        setOpenPositionPool(null);
    };

    // Refresh config when opening the modal so toggles from the bot take effect without a full reload.
    useEffect(() => {
        if (!openPositionPool || !hasInitData) return;

        let aborted = false;
        const controller = new AbortController();

        fetchGlobalConfig({ apiBaseUrl, initData, signal: controller.signal })
            .then((resp) => {
                if (aborted) return;
                setGlobalConfig(resp?.config || resp || null);
            })
            .catch(() => {
                // ignore; keep existing config
            });

        return () => {
            aborted = true;
            controller.abort();
        };
    }, [apiBaseUrl, initData, hasInitData, openPositionPool]);

    useEffect(() => {
        if (!openPositionPool || !hasInitData) return;

        const existing = Array.isArray(openPositionPool?.smartMoneyWallets)
            ? openPositionPool.smartMoneyWallets
            : [];
        if (existing.length > 0) {
            setOpenPositionSmartWallets(existing);
            setOpenPositionSmartWalletsLoading(false);
            return;
        }

        const poolId = String(openPositionPool?.pool_id || openPositionPool?.pool_address || '').trim();
        const poolVersion = String(openPositionPool?.protocol_version || openPositionPool?.pool_version || '')
            .trim()
            .toLowerCase();
        const chain = String(openPositionPool?.chain || hotPoolsData?.chain || 'bsc').trim().toLowerCase() || 'bsc';

        if (!poolId || !poolVersion) {
            setOpenPositionSmartWallets([]);
            setOpenPositionSmartWalletsLoading(false);
            return;
        }

        let aborted = false;
        const controller = new AbortController();

        setOpenPositionSmartWalletsLoading(true);
        fetchSmartMoneyPoolAdds({
            apiBaseUrl,
            initData,
            chain,
            poolVersion,
            poolId,
            windowHours: 24,
            limit: 120,
            feesLimit: 0,
            signal: controller.signal,
        })
            .then((resp) => {
                if (aborted) return;
                const wallets = Array.isArray(resp?.wallets) ? resp.wallets : [];
                setOpenPositionSmartWallets(wallets);
            })
            .catch(() => {
                if (aborted) return;
                setOpenPositionSmartWallets([]);
            })
            .finally(() => {
                if (!aborted) setOpenPositionSmartWalletsLoading(false);
            });

        return () => {
            aborted = true;
            controller.abort();
        };
    }, [apiBaseUrl, initData, hasInitData, hotPoolsData?.chain, openPositionPool]);

    useEffect(() => {
        if (!openPositionPool || !hasInitData || !multiWalletEnabled) return;

        let aborted = false;
        const controller = new AbortController();

        setWalletsLoading(true);
        setWalletsError('');

        const chain = String(openPositionPool?.chain || '').trim().toLowerCase();
        fetchWallets({ apiBaseUrl, initData, chain, signal: controller.signal })
            .then((resp) => {
                if (aborted) return;
                setWalletsData(resp || null);

                const list = Array.isArray(resp?.wallets) ? resp.wallets : [];
                if (list.length === 0) {
                    setOpenPositionWalletId('');
                    storage.remove(STORAGE_OPEN_POSITION_WALLET_ID);
                    return;
                }

                const saved = String(storage.get(STORAGE_OPEN_POSITION_WALLET_ID) || '').trim();
                const savedOk = saved && list.some((w) => String(w?.id) === saved);
                const next = savedOk ? saved : String((list.find((w) => w?.is_default) || list[0])?.id || '');
                setOpenPositionWalletId(next);
                if (next) storage.set(STORAGE_OPEN_POSITION_WALLET_ID, next);
            })
            .catch((e) => {
                if (aborted) return;
                setWalletsData(null);
                setWalletsError(String(e?.message || e));
            })
            .finally(() => {
                if (aborted) return;
                setWalletsLoading(false);
            });

        return () => {
            aborted = true;
            controller.abort();
        };
    }, [apiBaseUrl, initData, hasInitData, multiWalletEnabled, openPositionPool]);

    const handleOpenPosition = async () => {
        if (!openPositionPool) return;
        if (!hasInitData) {
            setOpenPositionError('未获取到 Telegram initData，请从机器人入口打开页面。');
            return;
        }
        const poolAddr = String(openPositionPool?.pool_address || '').trim().toLowerCase();
        if (poolAddr && blacklist.has(poolAddr)) {
            setOpenPositionError('该池子已加入黑名单，不能开仓。');
            return;
        }
        const amount = Number(String(openPositionAmount || '').trim());
        if (!Number.isFinite(amount) || amount <= 0) {
            setOpenPositionError('请输入有效的金额。');
            return;
        }
        const range = parseRangeInput(openPositionRangeLower, openPositionRangeUpper);
        if (!range || range.lower <= 0 || range.upper <= 0 || range.lower >= 100 || range.upper >= 100) {
            setOpenPositionError('区间无效，请输入 0-100 之间的百分比。');
            return;
        }

        const slippageRaw = String(openPositionSlippage || '').trim();
        let slippage = undefined;
        if (slippageRaw) {
            const v = Number(slippageRaw);
            if (!Number.isFinite(v) || v < 0 || v > 100) {
                setOpenPositionError('滑点无效，请输入 0-100 之间的百分比（不填则使用全局滑点）。');
                return;
            }
            slippage = v;
        }

        if (multiWalletEnabled) {
            if (walletsLoading) {
                setOpenPositionError('钱包加载中，请稍后再试。');
                return;
            }
            if (walletsError) {
                setOpenPositionError(walletsError);
                return;
            }
            const list = Array.isArray(walletsData?.wallets) ? walletsData.wallets : [];
            if (list.length === 0) {
                setOpenPositionError('未找到可用的钱包。');
                return;
            }
            if (list.length > 1) {
                const wid = Number(openPositionWalletId);
                if (!Number.isFinite(wid) || wid <= 0) {
                    setOpenPositionError('请选择钱包。');
                    return;
                }
            } else {
                const onlyId = String(list[0]?.id || '').trim();
                if (onlyId && String(openPositionWalletId || '') !== onlyId) {
                    setOpenPositionWalletId(onlyId);
                    storage.set(STORAGE_OPEN_POSITION_WALLET_ID, onlyId);
                }
            }
        }

        setOpenPositionLoading(true);
        setOpenPositionError('');
        setOperationProgress({ operation: 'open_position', currentStep: 0, totalSteps: 5, status: 'active', error: '' });
        try {
            const resp = await openPosition({
                apiBaseUrl,
                initData,
                chain: openPositionPool?.chain || 'bsc',
                poolAddress: openPositionPool?.pool_address,
                poolVersion: openPositionPool?.protocol_version,
                amount,
                rangeLowerPct: range.lower,
                rangeUpperPct: range.upper,
                slippageTolerance: slippage,
                allowEntrySwap: true,
                walletId: openPositionWalletId,
            });
            // Ensure done state even if WS event was missed
            setOperationProgress(prev => prev?.operation === 'open_position'
                ? { ...prev, currentStep: 4, status: 'done' } : prev);
            setOpenPositionPool(null);
            resetOpenPositionDraft();
        } catch (e) {
            const msg = String(e?.message || e || '').trim();
            setOpenPositionError(msg || '开仓失败，请稍后重试。');
            setOperationProgress(prev => prev?.operation === 'open_position'
                ? { ...prev, status: 'error', error: msg || '开仓失败' } : prev);
        } finally {
            setOpenPositionLoading(false);
        }
    };

    // 黑名单操作处理
    const handleBlacklist = useCallback(async (pool, add) => {
        if (!hasInitData || !pool?.pool_address) return;
        const addr = String(pool.pool_address).trim().toLowerCase();
        try {
            if (add) {
                await addToBlacklist({ apiBaseUrl, initData, poolAddress: addr });
                setBlacklist(prev => new Set(prev).add(addr));
                hapticNotification('success');
                showNotice(`已将 ${pool?.trading_pair || addr} 加入黑名单`, 'warning');
            } else {
                await removeFromBlacklist({ apiBaseUrl, initData, poolAddress: addr });
                setBlacklist(prev => {
                    const next = new Set(prev);
                    next.delete(addr);
                    return next;
                });
                hapticNotification('success');
                showNotice(`已将 ${pool?.trading_pair || addr} 移出黑名单`, 'info');
            }
        } catch (e) {
            hapticNotification('error');
            showNotice(`黑名单操作失败: ${e?.message || e}`, 'error');
        }
    }, [apiBaseUrl, initData, hasInitData]);

    const openBlacklistPrompt = useCallback((pool) => {
        const addr = String(pool?.pool_address || '').trim().toLowerCase();
        if (!addr) return;
        if (!hasInitData) {
            showNotice('未获取到 Telegram initData，请从机器人入口打开页面。', 'error');
            return;
        }
        if (blacklist.has(addr)) {
            showNotice('该池子已在黑名单中。', 'info');
            return;
        }
        setBlacklistPrompt({ pool, addr });
    }, [blacklist, hasInitData, showNotice]);

    const closeBlacklistPrompt = useCallback(() => {
        if (blacklistPromptLoading) return;
        setBlacklistPrompt(null);
    }, [blacklistPromptLoading]);

    const confirmBlacklistPrompt = useCallback(async () => {
        if (!blacklistPrompt?.pool) return;
        setBlacklistPromptLoading(true);
        try {
            await handleBlacklist(blacklistPrompt.pool, true);
            setBlacklistPrompt(null);
        } finally {
            setBlacklistPromptLoading(false);
        }
    }, [blacklistPrompt, handleBlacklist]);

    // 加载黑名单列表
    const loadBlacklist = useCallback(async () => {
        if (!hasInitData) return;
        try {
            const resp = await fetchBlacklist({ apiBaseUrl, initData });
            if (resp?.blacklist) {
                setBlacklist(new Set(resp.blacklist.map(a => String(a).toLowerCase())));
            }
        } catch (e) {
            console.error('[Blacklist] Load failed:', e);
        }
    }, [apiBaseUrl, initData, hasInitData]);

    // 加载冷却列表
    const loadCooldowns = useCallback(async () => {
        if (!hasInitData) return;
        try {
            const resp = await fetchCooldowns({ apiBaseUrl, initData });
            if (resp?.cooldowns) {
                setCooldowns(resp.cooldowns);
            }
        } catch (e) {
            console.error('[Cooldowns] Load failed:', e);
        }
    }, [apiBaseUrl, initData, hasInitData]);

    const handleRemoveCooldown = useCallback(async (tradingPair) => {
        const pair = String(tradingPair || '').trim();
        if (!hasInitData || !pair || cooldownRemovingPair) return;

        const ok = await requestConfirm({
            title: '解除冷却',
            message: `确认解除 ${pair} 的冷却？\n解除后该代币相关池子可再次开仓。`,
            confirmText: '确认解除',
            tone: 'danger',
        });
        if (!ok) return;

        setCooldownRemovingPair(pair);
        try {
            const resp = await removeCooldown({ apiBaseUrl, initData, tradingPair: pair });
            showNotice(resp?.message || `已解除冷却: ${pair}`, 'success');
            loadCooldowns();
        } catch (e) {
            showNotice(`解除冷却失败: ${String(e?.message || e)}`, 'error');
        } finally {
            setCooldownRemovingPair('');
        }
    }, [apiBaseUrl, initData, hasInitData, cooldownRemovingPair, loadCooldowns, requestConfirm]);

    // 初始化时加载黑名单和冷却列表
    useEffect(() => {
        if (hasInitData) {
            loadBlacklist();
            loadCooldowns();
        }
    }, [hasInitData, loadBlacklist, loadCooldowns]);

    const loadGlobalConfig = async () => {
        if (!hasInitData) {
            setGlobalConfigError('未获取到 Telegram initData，请从机器人入口打开页面。');
            return;
        }
        setGlobalConfigLoading(true);
        setGlobalConfigError('');
        try {
            const resp = await fetchGlobalConfig({ apiBaseUrl, initData });
            setGlobalConfig(resp?.config || resp || null);
        } catch (e) {
            setGlobalConfigError(String(e?.message || e));
        } finally {
            setGlobalConfigLoading(false);
        }
    };

    // Load once on boot so chain-mode settings can affect UX (single-chain mode hides chain selectors).
    useEffect(() => {
        if (!hasInitData) return;
        let aborted = false;
        const controller = new AbortController();

        fetchGlobalConfig({ apiBaseUrl, initData, signal: controller.signal })
            .then((resp) => {
                if (aborted) return;
                setGlobalConfig(resp?.config || resp || null);
            })
            .catch((e) => {
                if (aborted) return;
                console.error('[GlobalConfig] Load failed:', e);
            });

        return () => {
            aborted = true;
            controller.abort();
        };
    }, [apiBaseUrl, initData, hasInitData]);

    const openGlobalConfig = () => {
        setGlobalConfigOpen(true);
        loadGlobalConfig();
    };

    const handleAdminDisableAuto = async () => {
        if (!hasInitData || !showAdmin || !adminSelectedUserId || adminDisableLoading) return;

        const label = adminSelectedUser
            ? formatUserLabel(adminSelectedUser)
            : `用户 ${String(adminSelectedUserId)}`;

        const ok = await requestConfirm({
            title: '关闭 Auto',
            message: `确认关闭 ${label} 的 Auto？\n将撤出自动仓位并兑换成稳定币。`,
            confirmText: '确认关闭',
            tone: 'danger',
        });
        if (!ok) return;

        setAdminDisableLoading(true);
        setAdminDisableError('');
        setAdminDisableResult(null);

        try {
            const resp = await disableAdminAutoLP({
                apiBaseUrl,
                initData,
                userId: adminSelectedUserId,
                reason: '🛑 管理员已关闭 AutoLP',
            });
            setAdminDisableResult(resp);
        } catch (e) {
            setAdminDisableError(String(e?.message || e));
        } finally {
            setAdminDisableLoading(false);
        }
    };

    const handleSetTaskPaused = async (taskId, paused) => {
        if (!hasInitData || showAdmin) return;
        const id = Number(taskId);
        if (!Number.isFinite(id) || id <= 0) return;

        const wantPaused = Boolean(paused);
        const ok = await requestConfirm({
            title: wantPaused ? '暂停任务' : '恢复任务',
            message: wantPaused
                ? '确认暂停该任务？\n暂停后将不再自动执行再平衡/止损等操作。'
                : '确认恢复该任务？\n恢复后将继续自动执行再平衡/止损等操作。',
            confirmText: wantPaused ? '确认暂停' : '确认恢复',
        });
        if (!ok) return;

        try {
            await setTaskPaused({ apiBaseUrl, initData, taskId: id, paused: wantPaused });
            showNotice(wantPaused ? '任务已暂停' : '任务已恢复', 'success');
        } catch (e) {
            showNotice(String(e?.message || e), 'error');
        }
    };

    const handleStopTask = async (taskId) => {
        if (!hasInitData || showAdmin) return;
        const id = Number(taskId);
        if (!Number.isFinite(id) || id <= 0) return;

        const ok = await requestConfirm({
            title: '停止任务',
            message: '确认停止该任务？\n停止后将撤出流动性并兑换成 USDT，可能需要几十秒。',
            confirmText: '确认停止',
            tone: 'danger',
        });
        if (!ok) return;

        setOperationProgress({ operation: 'close_position', taskId: id, currentStep: 0, totalSteps: 4, status: 'active', error: '' });
        try {
            const resp = await stopTask({ apiBaseUrl, initData, taskId: id });
            if (resp?.status === 'stopped' || resp?.pending === false) {
                // Already stopped or immediate stop — all done
                setOperationProgress(prev => prev?.operation === 'close_position'
                    ? { ...prev, currentStep: 3, status: 'done' } : prev);
            } else {
                // Async — advance to step 1 only if WS hasn't already gone further
                setOperationProgress(prev => {
                    if (!prev || prev.operation !== 'close_position') return prev;
                    if (prev.status === 'done' || prev.status === 'error') return prev;
                    if (prev.currentStep > 1) return prev;
                    return { ...prev, currentStep: 1, status: 'active' };
                });
            }
        } catch (e) {
            const msg = String(e?.message || e || '').trim();
            if (msg.includes('task not found')) {
                setOperationProgress(null);
                showNotice(`任务 #${id} 不存在（可能已删除/已停止），已刷新列表。`, 'warning');
                try {
                    const resp = await fetchRealtimePositions({ apiBaseUrl, initData });
                    setData(resp);
                } catch {
                    // ignore
                }
                return;
            }
            setOperationProgress(prev => prev?.operation === 'close_position'
                ? { ...prev, status: 'error', error: msg || '停止失败' } : prev);
        }
    };

    // Polling fallback: detect close completion from positions data
    useEffect(() => {
        if (!operationProgress) return;
        if (operationProgress.operation !== 'close_position') return;
        if (operationProgress.status === 'done' || operationProgress.status === 'error') return;
        const taskId = operationProgress.taskId;
        if (!taskId) return;
        const positions = data?.positions;
        if (!positions) return; // data not loaded yet
        const found = positions.some(p => Number(p?.task_id) === Number(taskId));
        if (!found) {
            setOperationProgress(prev => {
                if (!prev || prev.operation !== 'close_position') return prev;
                if (prev.status === 'done' || prev.status === 'error') return prev;
                return { ...prev, currentStep: 3, status: 'done' };
            });
        }
    }, [data, operationProgress]);

    const handleDeleteTask = async (taskId) => {
        if (!hasInitData || showAdmin) return;
        const id = Number(taskId);
        if (!Number.isFinite(id) || id <= 0) return;

        const ok = await requestConfirm({
            title: '删除任务',
            message: '确认删除该任务？\n删除只会从列表移除，不会撤出流动性/兑换 USDT。',
            confirmText: '确认删除',
            tone: 'danger',
        });
        if (!ok) return;

        try {
            const resp = await deleteTask({ apiBaseUrl, initData, taskId: id });
            showNotice(resp?.message || '任务已删除', 'success');
        } catch (e) {
            showNotice(String(e?.message || e), 'error');
        }
    };

    const openTaskRangeModal = useCallback((taskId, position) => {
        if (!hasInitData || showAdmin) return;
        const id = Number(taskId);
        if (!Number.isFinite(id) || id <= 0) return;
        const low = Number(position?.task_range_lower_pct);
        const up = Number(position?.task_range_upper_pct);
        const amount = Number(position?.task_amount_usdt);
        const fallbackAmount = Number(position?.net_invested_usd ?? position?.initial_cost_usd);
        setTaskRangeEdit({
            taskId: id,
            title: String(position?.title || '').trim() || `任务 #${id}`,
        });
        setTaskRangeLower(Number.isFinite(low) && low > 0 ? String(low) : '');
        setTaskRangeUpper(Number.isFinite(up) && up > 0 ? String(up) : '');
        setTaskRangeAmount(
            Number.isFinite(amount) && amount > 0
                ? String(amount)
                : (Number.isFinite(fallbackAmount) && fallbackAmount > 0 ? fallbackAmount.toFixed(2) : ''),
        );
        setTaskRangeError('');
    }, [hasInitData, showAdmin]);

    const closeTaskRangeModal = () => {
        if (taskRangeLoading) return;
        setTaskRangeEdit(null);
        setTaskRangeLower('');
        setTaskRangeUpper('');
        setTaskRangeAmount('');
        setTaskRangeError('');
    };

    const submitTaskRange = async () => {
        if (!taskRangeEdit) return;
        if (!hasInitData || showAdmin) return;

        const range = parseRangeInput(taskRangeLower, taskRangeUpper);
        const amount = Number(String(taskRangeAmount || '').trim());
        if (!Number.isFinite(amount) || amount <= 0) {
            setTaskRangeError('金额无效，请输入大于 0 的 USDT 数值。');
            return;
        }
        if (!range || range.lower <= 0 || range.upper <= 0 || range.lower >= 100 || range.upper >= 100) {
            setTaskRangeError('区间无效，请输入 0-100 之间的百分比。');
            return;
        }

        const ok = await requestConfirm({
            title: '修改再平衡参数',
            message: '确认修改？\n修改后的区间和金额都将在【下次再平衡时】生效。\n注意：本次修改不会直接改变当前的持仓。',
            confirmText: '确认修改',
        });
        if (!ok) return;

        setTaskRangeLoading(true);
        setTaskRangeError('');
        try {
            await updateTaskRange({
                apiBaseUrl,
                initData,
                taskId: taskRangeEdit.taskId,
                rangeLowerPct: range.lower,
                rangeUpperPct: range.upper,
                amountUSDT: amount,
            });
            showNotice('区间已更新（下次再平衡生效）', 'success');
            setTaskRangeEdit(null);
            setTaskRangeLower('');
            setTaskRangeUpper('');
            setTaskRangeAmount('');
        } catch (e) {
            setTaskRangeError(String(e?.message || e || '修改失败'));
        } finally {
            setTaskRangeLoading(false);
        }
    };

    // 批量操作函数
    const toggleTaskSelection = (taskId) => {
        const newSet = new Set(selectedTaskIds);
        if (newSet.has(taskId)) {
            newSet.delete(taskId);
        } else {
            newSet.add(taskId);
        }
        setSelectedTaskIds(newSet);
        hapticSelection();
    };

    const selectAllTasks = () => {
        const allIds = new Set();
        visibleTaskPositions.forEach(p => {
            if (p?.task_id) allIds.add(p.task_id);
        });
        setSelectedTaskIds(allIds);
        hapticImpact('light');
    };

    const deselectAllTasks = () => {
        setSelectedTaskIds(new Set());
        hapticImpact('light');
    };

    const batchPauseTasks = async (paused) => {
        if (selectedTaskIds.size === 0) return;
        setBatchLoading(true);
        let successCount = 0;
        let failCount = 0;

        for (const taskId of selectedTaskIds) {
            try {
                await setTaskPaused({ apiBaseUrl, initData, taskId, paused });
                successCount++;
            } catch {
                failCount++;
            }
        }

        setBatchLoading(false);
        setSelectedTaskIds(new Set());
        setBatchMode(false);
        hapticNotification(failCount === 0 ? 'success' : 'warning');
        showNotice(
            `批量${paused ? '暂停' : '恢复'}完成：成功 ${successCount}，失败 ${failCount}`,
            failCount === 0 ? 'success' : 'warning'
        );
    };

    // 计算本地刷新后经过的秒数
    const localUpdateSecAgo = useMemo(() => {
        const elapsed = tick - lastPollTimeRef.current;
        return Math.max(0, Math.floor(elapsed / 1000));
    }, [tick]);

    const moduleMetaByMode = useMemo(() => ({
        hot_pools: {
            title: '热门池子',
            icon: icons.fire, // Changed to fire icon
            subtitle: `5m · ${hotPoolsData ? `更新：${localUpdateSecAgo}秒前` : hotPoolsLoading ? '加载中...' : '暂无数据'} · 自动刷新 ${hotPoolsPollIntervalSec}s`,
        },
        positions: {
            title: '实时仓位',
            icon: icons.bot,
            subtitle: walletAddress ? `钱包：${walletAddress.slice(0, 6)}...${walletAddress.slice(-4)}` : '加载钱包中...',
        },
        monitor: {
            title: '自动任务监控',
            icon: icons.chart, // Changed to chart icon
            subtitle: `Auto任务：${Array.isArray(autoMonitor?.tasks) ? autoMonitor.tasks.length : 0} · 更新：${formatRelativeTime(autoMonitor?.updated_at, tick) || '--'}`,
        },
        smart_money: {
            title: '聪明钱',
            icon: icons.search,
            subtitle: `最近2h池子 ${Array.isArray(smartMoney?.pools) ? smartMoney.pools.length : 0} · 更新 ${formatRelativeTime(smartMoney?.updated_at, tick) || '--'}`,
        },
        admin: {
            title: '管理面板',
            icon: icons.gear, // Changed to gear icon
            subtitle: adminSelectedUser
                ? `用户：${formatUserLabel(adminSelectedUser)}`
                : adminUsersLoading && adminUsers.length === 0
                    ? '加载用户中...'
                    : adminUsers.length
                        ? `开启Auto用户：${adminUsers.length}`
                        : '暂无开启Auto用户',
        },
    }), [
        adminSelectedUser,
        adminUsers,
        adminUsersLoading,
        autoMonitor,
        hotPoolsData,
        hotPoolsLoading,
        hotPoolsPollIntervalSec,
        localUpdateSecAgo,
        monitorPollSec,
        smartMoney,
        smartMoneyPollSec,
        tick,
        walletAddress,
    ]);
    const activeModuleMeta = moduleMetaByMode[viewMode] || moduleMetaByMode.positions;

    const hasAdminPositions = Boolean(adminPositions);
    const adminSummaryPlaceholder = adminSelectedUserId
        ? adminPositionsLoading
            ? '加载用户仓位中...'
            : '用户仓位暂不可用'
        : '请选择用户查看实时仓位';
    const showEmptyPositions = isPositions && Boolean(activeData) && visiblePositions.length === 0;
    const showEmptyTaskTab = isPositions && Boolean(activeData) && !showEmptyPositions && positionsTaskTab !== 'all' && visibleTaskPositions.length === 0;
    const monitorTasks = useMemo(() => (Array.isArray(autoMonitor?.tasks) ? autoMonitor.tasks : []), [autoMonitor]);
    const blacklistList = useMemo(() => Array.from(blacklist).sort(), [blacklist]);
    const hotPoolsPairMap = useMemo(() => {
        const m = new Map();
        for (const row of hotPoolsRows) {
            const addr = String(row?.pool_address || '').trim().toLowerCase();
            const pair = String(row?.trading_pair || '').trim();
            if (addr && pair && !m.has(addr)) m.set(addr, pair);
        }
        return m;
    }, [hotPoolsRows]);
    const monitorPoolTitleMap = useMemo(() => {
        const m = new Map();
        for (const t of monitorTasks) {
            const addr = String(t?.pool_id || '').trim().toLowerCase();
            const title = String(t?.title || '').trim();
            if (addr && title && !m.has(addr)) m.set(addr, title);
        }
        return m;
    }, [monitorTasks]);
    const blacklistPromptPool = blacklistPrompt?.pool || null;
    const blacklistPromptPair = String(blacklistPromptPool?.trading_pair || '').trim();
    const blacklistPromptAddr = String(blacklistPromptPool?.pool_address || '').trim().toLowerCase();
    const blacklistPromptAddrShort = blacklistPromptAddr.length > 12
        ? `${blacklistPromptAddr.slice(0, 6)}...${blacklistPromptAddr.slice(-4)}`
        : blacklistPromptAddr;
    const showEmptyAutoTasks = isMonitor && Boolean(autoMonitor) && monitorTasks.length === 0 && !autoMonitorLoading && !autoMonitorError;

    const initDataMissing = viewMode !== 'hot_pools' && !hasInitData;
    const noticeClass = notice?.tone === 'error'
        ? 'bg-red-600 text-white'
        : notice?.tone === 'success'
            ? brand.successNoticeClass
            : 'bg-zinc-900 text-white dark:bg-white/10 dark:text-white';
    const globalCfg = globalConfig || {};
    const rebalanceText = Number.isFinite(Number(globalCfg.rebalance_timeout))
        ? `${Number(globalCfg.rebalance_timeout)} 秒`
        : '--';
    const stopLossDelayText = Number.isFinite(Number(globalCfg.stop_loss_delay_seconds))
        ? `${Number(globalCfg.stop_loss_delay_seconds)} 秒`
        : '--';
    const slippageText = Number.isFinite(Number(globalCfg.slippage_tolerance))
        ? `${Number(globalCfg.slippage_tolerance).toFixed(2)}%`
        : '--';
    const residualText = Number.isFinite(Number(globalCfg.residual_tolerance))
        ? `${Number(globalCfg.residual_tolerance).toFixed(2)}%`
        : '--';
    const confirmButtonClass = confirmState?.tone === 'danger'
        ? 'bg-red-500 text-white hover:bg-red-600 active:bg-red-700'
        : brand.solidButtonClass;

    const activeErrorText = useMemo(() => {
        const msg = String(activeError || '').trim();
        if (!msg) return '';
        if (msg.includes('missing initData')) {
            return '未获取到 Telegram WebApp 的 initData：请从机器人里的“实时仓位”按钮打开。';
        }
        if (msg.includes('invalid initData')) {
            return 'initData 校验失败：请检查后端 TELEGRAM_BOT_TOKEN 是否正确，或 initData 是否过期。';
        }
        return msg;
    }, [activeError]);

    return (
        <div className={`min-h-screen max-w-[720px] px-4 py-4 mx-auto ${isPositions ? 'pb-[calc(96px+env(safe-area-inset-bottom))]' : 'pb-[calc(80px+env(safe-area-inset-bottom))]'}`}>
            {notice ? (
                <div className="fixed left-1/2 top-[calc(env(safe-area-inset-top)+64px)] z-50 w-[calc(100%-2rem)] max-w-md -translate-x-1/2">
                    <div className={`rounded-xl px-3 py-2 text-sm font-semibold shadow-lg ${noticeClass}`}>
                        {notice.message}
                    </div>
                </div>
            ) : null}
            {/* 顶部加载进度条 */}
            <div className="progress-bar-container">
                <div
                    className={`progress-bar ${loading || hotPoolsLoading ? 'loading' : ''}`}
                    style={{ width: loading || hotPoolsLoading ? undefined : `${pollProgress}%` }}
                />
            </div>
            <header className="mb-4">
                <div className="flex items-center justify-between gap-3">
                    <div className="flex items-center gap-2">
                        <div className={`flex h-9 w-9 items-center justify-center rounded-xl ${brand.iconChipClass}`}>
                            <Icon path={activeModuleMeta.icon} className="h-5 w-5" />
                        </div>
                        <div>
                            <div className="text-lg font-extrabold tracking-tight">{activeModuleMeta.title}</div>
                            <div className="mt-0.5 text-xs text-zinc-500 dark:text-white/40">
                                <NumberFlowValue value={activeModuleMeta.subtitle} formatter={() => activeModuleMeta.subtitle} />
                            </div>
                        </div>
                    </div>

                    <div className="flex items-center gap-2">
                        <button
                            type="button"
                            onClick={toggleTheme}
                            className={`inline-flex h-9 w-9 items-center justify-center rounded-xl border shadow-sm ${theme === 'dark' ? 'border-white/20 bg-white/10 text-white hover:bg-white/20' : 'border-zinc-300 bg-zinc-100 text-zinc-900 hover:bg-zinc-200'}`}
                            aria-label="切换主题"
                        >
                            <Icon path={theme === 'dark' ? icons.moon : icons.sun} className="h-5 w-5" />
                        </button>
                        <button
                            type="button"
                            onClick={() => setSettingsOpen(true)}
                            className="inline-flex h-9 w-9 items-center justify-center rounded-xl border border-zinc-200 bg-zinc-100 text-zinc-900 shadow-sm hover:bg-zinc-200 active:bg-zinc-200 dark:border-white/10 dark:bg-white/5 dark:text-white/80 dark:hover:bg-white/10 dark:active:bg-white/15"
                            aria-label="设置"
                        >
                            <Icon path={icons.gear} className="h-5 w-5" />
                        </button>
                    </div>
                </div>


                {showAdmin ? (
                    <ModuleHeader
                        title="管理概览"
                        subtitle={hasAdminPositions
                            ? adminSelectedUser
                                ? `用户：${formatUserLabel(adminSelectedUser)}`
                                : ''
                            : adminSummaryPlaceholder}
                        actions={hasAdminPositions ? (
                            <div className="text-right">
                                <div className="text-[11px] text-zinc-500 dark:text-white/40">自动刷新</div>
                                <div className="text-sm font-semibold tabular-nums">
                                    <NumberFlowValue value={pollIntervalSec} formatOptions={{ maximumFractionDigits: 0 }} />s
                                </div>
                            </div>
                        ) : null}
                    >
                        {hasAdminPositions ? (
                            <div>
                                <div className="text-[11px] text-zinc-500 dark:text-white/40">总余额</div>
                                <div className={`mt-0.5 text-2xl font-extrabold tabular-nums text-zinc-900 ${brand.textClass}`}>
                                    <NumberFlowValue value={totalUsd} formatter={(v) => formatUsd(v)} />
                                </div>
                                <div className="mt-1 text-[11px] text-zinc-500 dark:text-white/40 tabular-nums">
                                    <NumberFlowValue value={bnbBalance} formatter={() => String(bnbBalance ?? '0')} /> BNB
                                    {typeof bnbUsd === 'number' ? <> ≈ <NumberFlowValue value={bnbUsd} formatter={(v) => formatUsd(v)} /></> : ''}
                                </div>
                            </div>
                        ) : null}
                    </ModuleHeader>
                ) : isHotPools ? (
                    <ModuleHeader
                        title={hotPoolsSort === 'fee_rate' ? '费用率排行' : hotPoolsSort === 'volume' ? '交易量排行' : '费用排行'}
                        actions={(
                            <>
                                <div className="flex shrink-0 p-0.5 bg-zinc-100/80 rounded-xl dark:bg-[#16181d] shadow-inner ring-1 ring-zinc-200/50 dark:ring-black/20">
                                    {HOT_POOL_SORT_TABS.map((tab) => (
                                        <button
                                            key={tab.key}
                                            type="button"
                                            onClick={() => setHotPoolsSort(tab.key)}
                                            aria-pressed={hotPoolsSort === tab.key}
                                            className={`relative rounded-lg px-2.5 py-1 text-[12px] font-bold whitespace-nowrap transition-all duration-300 ${hotPoolsSort === tab.key
                                                ? brand.gradientButtonClass
                                                : 'text-zinc-500 hover:text-zinc-700 dark:text-zinc-400 dark:hover:text-zinc-200 hover:bg-zinc-200/50 dark:hover:bg-white/5'
                                                }`}
                                        >
                                            {tab.label}
                                        </button>
                                    ))}
                                </div>
                                <button
                                    type="button"
                                    onClick={openPoolSearch}
                                    className="inline-flex h-9 w-9 items-center justify-center rounded-2xl bg-white/70 text-zinc-700 ring-1 ring-zinc-200 transition hover:bg-white dark:bg-white/5 dark:text-white/70 dark:ring-white/10"
                                    aria-label="Search"
                                    title="搜索池子"
                                >
                                    <Icon path={icons.search} className="h-4 w-4" />
                                </button>
                                <button
                                    type="button"
                                    onClick={() => {
                                        closePoolSearch();
                                        setHotPoolsFilterOpen(true);
                                    }}
                                    className={`relative inline-flex h-9 w-9 items-center justify-center rounded-2xl ring-1 transition ${hotPoolsFilterEnabled
                                        ? brand.softButtonClass
                                        : 'bg-white/70 text-zinc-700 ring-zinc-200 hover:bg-white dark:bg-white/5 dark:text-white/70 dark:ring-white/10'
                                        }`}
                                    aria-label="Filter"
                                    title="Filter"
                                >
                                    <Icon path={icons.filter} className="h-3.5 w-3.5" />
                                    {hotPoolsFilterEnabled ? (
                                        <span className={`absolute -right-0.5 -top-0.5 h-2 w-2 rounded-full ring-2 ring-white dark:ring-[#111318] ${brand.dotClass}`} />
                                    ) : null}
                                </button>
                            </>
                        )}
                    />
                ) : showWalletSummaryCard ? (
                    <>
                        <div className="mt-3 overflow-hidden rounded-[24px] border border-zinc-200/80 bg-[radial-gradient(circle_at_top_left,_rgba(16,185,129,0.16),_transparent_42%),linear-gradient(135deg,_rgba(255,255,255,0.92),_rgba(244,247,255,0.78))] p-3 shadow-[0_16px_40px_-24px_rgba(15,23,42,0.38)] dark:border-white/10 dark:bg-[radial-gradient(circle_at_top_left,_rgba(16,185,129,0.16),_transparent_38%),linear-gradient(135deg,_rgba(24,27,32,0.98),_rgba(15,17,21,0.94))] dark:shadow-[0_18px_48px_-28px_rgba(0,0,0,0.7)]">
                            <div className="flex flex-col gap-2.5">
                                <div className="flex items-start justify-between gap-3">
                                    <div className="min-w-0 flex-1">
                                        <div className="inline-flex items-center rounded-full border border-emerald-500/20 bg-emerald-500/10 px-2.5 py-1 text-[10px] font-semibold uppercase tracking-[0.24em] text-emerald-700 dark:border-emerald-400/20 dark:bg-emerald-400/10 dark:text-emerald-300/90">
                                            {isMonitor ? '监控概览' : '仓位总览'}
                                        </div>

                                        <div className="mt-2.5 text-[10px] font-medium text-zinc-500 dark:text-white/45">总资产</div>
                                        <div className="mt-1 text-[24px] font-black leading-none tracking-tight text-zinc-950 dark:text-white">
                                            <NumberFlowValue value={totalUsd} formatter={(v) => formatUsd(v)} />
                                        </div>

                                        <div className="mt-2 flex flex-wrap gap-1.5 text-[10px] text-zinc-500 dark:text-white/50">
                                            {!multiWalletSummary ? (
                                                <span className="rounded-full border border-white/70 bg-white/70 px-2 py-1 font-mono dark:border-white/10 dark:bg-white/5">
                                                    {walletAddress ? `${walletAddress.slice(0, 6)}...${walletAddress.slice(-4)}` : '未连接'}
                                                </span>
                                            ) : null}
                                            {!multiWalletSummary ? (
                                                <span className="rounded-full border border-white/70 bg-white/70 px-2 py-1 dark:border-white/10 dark:bg-white/5">
                                                    <NumberFlowValue value={bnbBalance} formatter={() => String(bnbBalance ?? '0')} /> BNB
                                                    {typeof bnbUsd === 'number' ? <> · <NumberFlowValue value={bnbUsd} formatter={(v) => formatUsd(v)} /></> : null}
                                                </span>
                                            ) : null}
                                        </div>
                                    </div>

                                    <div className="flex shrink-0 flex-col items-end gap-1.5">
                                        <button
                                            type="button"
                                            onClick={openGlobalConfig}
                                            disabled={!hasInitData}
                                            className={`inline-flex shrink-0 rounded-2xl px-3 py-2 text-[10px] font-semibold ring-1 backdrop-blur-md transition-colors ${hasInitData
                                                ? 'bg-white/80 text-zinc-700 ring-zinc-200 hover:bg-white dark:bg-white/10 dark:text-white/90 dark:ring-white/10 dark:hover:bg-white/20'
                                                : 'cursor-not-allowed bg-zinc-100 text-zinc-400 ring-zinc-200 dark:bg-white/5 dark:text-white/30 dark:ring-white/10'
                                                }`}
                                        >
                                            全局配置
                                        </button>
                                        {multiWalletSummary ? (
                                            <span className="rounded-full border border-white/70 bg-white/70 px-2 py-1 text-[10px] font-semibold text-zinc-600 dark:border-white/10 dark:bg-white/5 dark:text-white/65">
                                                {totalWalletCount} 个钱包
                                            </span>
                                        ) : null}
                                    </div>
                                </div>

                                <div className={`flex gap-1 ${summaryMetricDense ? 'gap-0.5' : ''}`}>
                                    {summaryMetricCards.map((card) => (
                                        <div
                                            key={card.key}
                                            className={`min-w-0 flex-1 rounded-[18px] border border-white/70 bg-white/75 backdrop-blur-md dark:border-white/10 dark:bg-white/5 ${summaryMetricDense ? 'px-1.25 py-1.25' : 'px-1.5 py-1.5'}`}
                                        >
                                            <div className={`truncate font-semibold uppercase text-zinc-500 dark:text-white/40 ${summaryMetricDense ? 'text-[7px] tracking-[0.04em]' : 'text-[8px] tracking-[0.08em]'}`}>
                                                {card.label}
                                            </div>
                                            <div className={`mt-0.5 truncate font-bold tabular-nums text-zinc-950 dark:text-white ${summaryMetricDense ? 'text-[10px]' : 'text-[11px]'}`}>
                                                {card.value}
                                            </div>
                                        </div>
                                    ))}
                                </div>
                            </div>
                        </div>
                        {false && (
                    <div className="mt-3 rounded-2xl border border-zinc-200 bg-white/40 backdrop-blur-md p-3 px-4 shadow-sm dark:border-white/5 dark:bg-[#16181c] dark:shadow-none">
                        <div className="flex flex-row items-center justify-between gap-2">
                            <div className="flex flex-col">
                                <div className="flex items-center gap-2">
                                    <div className="text-[15px] font-bold text-zinc-900 dark:text-white/95">{isMonitor ? '监控概览' : '仓位概览'}</div>
                                    {!(Array.isArray(posWalletBalances?.wallets) && posWalletBalances.wallets.length > 1) && (
                                        <div className="text-xs text-zinc-500 dark:text-white/40">
                                            <NumberFlowValue value={bnbBalance} formatter={() => String(bnbBalance ?? '0')} /> BNB
                                            {typeof bnbUsd === 'number' ? <> ≈ <NumberFlowValue value={bnbUsd} formatter={(v) => formatUsd(v)} /></> : ''}
                                        </div>
                                    )}
                                </div>
                                {Array.isArray(posWalletBalances?.wallets) && posWalletBalances.wallets.length > 1 && (
                                    <div className="mt-0.5 flex flex-wrap gap-x-3 gap-y-0 text-[10px] text-zinc-400 dark:text-white/30 tabular-nums">
                                        {posWalletBalances.wallets.map((w) => (
                                            <span key={w.id} className="whitespace-nowrap">
                                                <span className="font-medium text-zinc-500 dark:text-white/45">{w.name || `${String(w.address || '').slice(0, 6)}..${String(w.address || '').slice(-4)}`}</span>
                                                {' '}${w.stable_balance !== 'N/A' ? w.stable_balance : '--'}
                                            </span>
                                        ))}
                                    </div>
                                )}
                                <div className="mt-1 flex items-baseline gap-2">
                                    <div className="text-[11px] text-zinc-500 dark:text-white/50">总余额</div>
                                    <div className="text-xl font-extrabold tabular-nums text-zinc-900 dark:text-emerald-400 tracking-tight">
                                        <NumberFlowValue value={totalUsd} formatter={(v) => formatUsd(v)} />
                                    </div>
                                </div>
                            </div>
                            <div className="flex flex-col items-end gap-1.5 shrink-0">
                                <div className="text-[10px] text-zinc-500 dark:text-white/40 flex items-center gap-1">
                                    刷新 <span className="font-semibold text-zinc-700 dark:text-white/70"><NumberFlowValue value={pollIntervalSec} formatOptions={{ maximumFractionDigits: 0 }} />s</span>
                                </div>
                                <button
                                    type="button"
                                    onClick={openGlobalConfig}
                                    disabled={!hasInitData}
                                    className={`inline-flex rounded-lg px-2.5 py-1 text-[11px] font-semibold ring-1 min-w-[64px] justify-center transition-colors ${hasInitData
                                        ? 'bg-white text-zinc-700 ring-zinc-200 hover:bg-zinc-50 dark:bg-white/10 dark:text-white/90 dark:ring-white/10 dark:hover:bg-white/20'
                                        : 'cursor-not-allowed bg-zinc-100 text-zinc-400 ring-zinc-200 dark:bg-white/5 dark:text-white/30 dark:ring-white/10'
                                        }`}
                                >
                                    全局配置
                                </button>
                            </div>
                        </div>
                    </div >
                        )}
                    </>
                ) : null
                }

            </header >

            {
                isHotPools && hotPoolsError ? (
                    <div className="mb-4 rounded-2xl border border-red-500/30 bg-red-500/10 p-4 text-sm text-red-700 dark:text-red-200">
                        {hotPoolsError}
                    </div>
                ) : null
            }

            {
                isHotPools && hotPoolsLoading && hotPoolsRows.length === 0 ? (
                    <SkeletonList count={5} Card={SkeletonHotPoolCard} />
                ) : null
            }

            {
                isHotPools && !hotPoolsLoading && !hotPoolsError && hotPoolsData && hotPoolsRows.length === 0 ? (
                    <div className="mb-4 rounded-2xl border border-zinc-200 bg-white/70 p-6 text-sm text-zinc-500 dark:border-white/10 dark:bg-white/5 dark:text-white/60">
                        暂无热门池子数据。
                    </div>
                ) : null
            }

            {
                isHotPools && !hotPoolsLoading && !hotPoolsError && hotPoolsData && hotPoolsRows.length > 0 && hotPoolsFilterEnabled && hotPoolsVisibleRows.length === 0 ? (
                    <div className="mb-4 rounded-2xl border border-zinc-200 bg-white/70 p-6 text-sm text-zinc-500 dark:border-white/10 dark:bg-white/5 dark:text-white/60">
                        筛选后暂无热门池子数据。
                    </div>
                ) : null
            }

            {
                !isHotPools && showAdmin ? (
                    <AdminPage
                        apiBaseUrl={apiBaseUrl}
                        initData={initData}
                        hasInitData={hasInitData}
                        tick={tick}
                        pollIntervalSec={pollIntervalSec}
                        onNotice={showNotice}
                    />
                ) : null
            }

            {
                !isHotPools && initDataMissing ? (
                    <div className="mb-4 rounded-2xl border border-amber-500/30 bg-amber-500/10 p-4 text-sm text-amber-700 dark:text-amber-200">
                        请从 Telegram 机器人里的"实时仓位"按钮打开页面（否则无法读取你的仓位）。
                    </div>
                ) : null
            }

            {
                isPositions && activeErrorText ? (
                    <div className="mb-4 rounded-2xl border border-red-500/30 bg-red-500/10 p-4 text-sm text-red-700 dark:text-red-200">
                        {activeErrorText}
                    </div>
                ) : null
            }

            {
                isMonitor && autoMonitorError ? (
                    <div className="mb-4 rounded-2xl border border-red-500/30 bg-red-500/10 p-4 text-sm text-red-700 dark:text-red-200">
                        {autoMonitorError}
                    </div>
                ) : null
            }

            {
                isSmartMoney && smartMoneyError ? (
                    <div className="mb-4 rounded-2xl border border-red-500/30 bg-red-500/10 p-4 text-sm text-red-700 dark:text-red-200">
                        {smartMoneyError}
                    </div>
                ) : null
            }

            {
                isPositions && activeLoading && !activeData ? (
                    <SkeletonList count={2} Card={SkeletonPositionCard} />
                ) : null
            }

            {
                isMonitor && autoMonitorLoading && !autoMonitor ? (
                    <SkeletonList count={2} Card={SkeletonPositionCard} />
                ) : null
            }

            {
                isSmartMoney && smartMoneyLoading && !smartMoney ? (
                    <SkeletonList count={2} Card={SkeletonPositionCard} />
                ) : null
            }

            {
                isMonitor && !showAdmin ? (
                    <div className="mb-3 flex items-center justify-between gap-2 rounded-xl border border-zinc-200/50 bg-zinc-50 px-3 py-2 dark:border-white/6 dark:bg-[#0b0f14]">
                        <div className="flex items-center gap-2">
                            <span className="text-xs font-medium text-zinc-500 dark:text-white/50">对比基准</span>
                            <button
                                type="button"
                                onClick={toggleAutoGuardBaseline}
                                disabled={!hasInitData || autoGuardBaselineUpdating}
                                className={`inline-flex items-center gap-1.5 rounded-lg px-2.5 py-1 text-xs font-semibold transition-all ${!hasInitData || autoGuardBaselineUpdating
                                    ? 'cursor-not-allowed bg-zinc-100 text-zinc-400 dark:bg-white/5 dark:text-white/25'
                                    : guardCompareToPeak
                                        ? 'bg-amber-500/10 text-amber-600 ring-1 ring-amber-500/20 hover:bg-amber-500/15 hover:ring-amber-500/30 dark:bg-amber-500/10 dark:text-amber-400 dark:ring-amber-500/20 dark:hover:bg-amber-500/15'
                                        : 'bg-sky-500/10 text-sky-600 ring-1 ring-sky-500/20 hover:bg-sky-500/15 hover:ring-sky-500/30 dark:bg-sky-500/10 dark:text-sky-400 dark:ring-sky-500/20 dark:hover:bg-sky-500/15'
                                    }`}
                            >
                                {guardCompareToPeak ? (
                                    <>
                                        <svg className="h-3 w-3" viewBox="0 0 16 16" fill="currentColor"><path d="M8 2l2 4h4l-3.5 3 1.5 5L8 11l-4 3 1.5-5L2 6h4l2-4z" /></svg>
                                        最高点
                                    </>
                                ) : (
                                    <>
                                        <svg className="h-3 w-3" viewBox="0 0 16 16" fill="currentColor"><path d="M8 14A6 6 0 108 2a6 6 0 000 12zm0-2a4 4 0 110-8 4 4 0 010 8zm0-3a1 1 0 100-2 1 1 0 000 2z" /></svg>
                                        开仓时
                                    </>
                                )}
                                {autoGuardBaselineUpdating && <span className="ml-0.5 animate-pulse">…</span>}
                            </button>
                        </div>
                        <span className="text-[10px] text-zinc-400 dark:text-white/30">点击切换</span>
                    </div>
                ) : null
            }

            {/* 批量操作工具栏 */}
            {
                isPositions && !showAdmin && visiblePositions.length > 1 && (
                    <div className="mb-4 flex items-center justify-between gap-2">
                        <button
                            type="button"
                            onClick={() => {
                                setBatchMode(!batchMode);
                                if (batchMode) setSelectedTaskIds(new Set());
                                hapticImpact('light');
                            }}
                            className={`inline-flex items-center gap-1 rounded-xl px-3 py-1.5 text-xs font-semibold transition ${batchMode
                                ? 'bg-sky-500/20 text-sky-700 ring-1 ring-sky-500/30 dark:text-sky-200'
                                : 'bg-zinc-100 text-zinc-600 hover:bg-zinc-200 dark:bg-white/5 dark:text-white/70 dark:hover:bg-white/10'
                                }`}
                        >
                            {batchMode ? '退出多选' : '批量操作'}
                        </button>

                        {batchMode && (
                            <div className="flex items-center gap-2">
                                <button
                                    type="button"
                                    onClick={selectedTaskIds.size === visiblePositions.length ? deselectAllTasks : selectAllTasks}
                                    className="inline-flex items-center rounded-xl bg-zinc-100 px-2 py-1 text-xs font-semibold text-zinc-600 hover:bg-zinc-200 dark:bg-white/5 dark:text-white/70 dark:hover:bg-white/10"
                                >
                                    {selectedTaskIds.size === visiblePositions.length ? '取消全选' : '全选'}
                                </button>
                                <span className="text-xs text-zinc-500 dark:text-white/50">
                                    已选 {selectedTaskIds.size}
                                </span>
                                <button
                                    type="button"
                                    onClick={() => batchPauseTasks(true)}
                                    disabled={selectedTaskIds.size === 0 || batchLoading}
                                    className="inline-flex items-center rounded-xl bg-amber-500/15 px-2 py-1 text-xs font-semibold text-amber-700 hover:bg-amber-500/25 disabled:opacity-50 dark:text-amber-200"
                                >
                                    {batchLoading ? '处理中...' : '批量暂停'}
                                </button>
                                <button
                                    type="button"
                                    onClick={() => batchPauseTasks(false)}
                                    disabled={selectedTaskIds.size === 0 || batchLoading}
                                    className="inline-flex items-center rounded-xl bg-emerald-500/15 px-2 py-1 text-xs font-semibold text-emerald-700 hover:bg-emerald-500/25 disabled:opacity-50 dark:text-emerald-200"
                                >
                                    批量恢复
                                </button>
                            </div>
                        )}
                    </div>
                )
            }

            {/* 移除了"暂无自动任务"提示 */}

            <div className="space-y-4 animate-fade-in-up">
                {isHotPools
                    ? hotPoolsVisibleRows.map((row, index) => {
                        const proto = String(row?.protocol_version || '').trim();
                        const addr = String(row?.pool_address || '').trim().toLowerCase();
                        const poolKey = `${proto}:${addr}`;
                        const prevData = previousHotPoolsMap[poolKey];
                        return (
                            <HotPoolCard
                                key={`${proto}:${addr}`}
                                pool={row}
                                metric={hotPoolsSort}
                                previousData={prevData}
                                accentTheme={accentTheme}
                                onOpenKline={setKlinePool}
                                onOpenPosition={openPositionModal}
                                onBlacklistRequest={openBlacklistPrompt}
                                rank={index + 1}
                                apiBaseUrl={apiBaseUrl}
                                isBlacklisted={blacklist.has(addr)}
                                chain={hotPoolsData?.chain || 'bsc'}
                            />
                        );
                    })
                    : isMonitor
                        ? (
                            <>
                                {monitorTasks.map((t) => (
                                    <AutoMonitorCard
                                        key={String(t?.task_id)}
                                        task={t}
                                        tick={tick}
                                        isBlacklisted={blacklist.has(String(t?.pool_id || '').trim().toLowerCase())}
                                    />
                                ))}
                                {/* 黑名单池子展示 */}
                                {!showAdmin && blacklistList.length > 0 ? (
                                    <div className="mt-4 rounded-2xl border border-red-500/30 bg-red-500/10 p-4 dark:border-red-500/20 dark:bg-red-500/5">
                                        <div className="flex items-center gap-2 text-sm font-semibold text-red-700 dark:text-red-300">
                                            <span>🚫</span>
                                            <span>黑名单池子</span>
                                            <span className="ml-auto text-[11px] font-normal text-red-600 dark:text-red-400">
                                                这些池子禁止开仓
                                            </span>
                                        </div>
                                        <div className="mt-2 space-y-2">
                                            {blacklistList.map((addr) => {
                                                const label = hotPoolsPairMap.get(addr) || monitorPoolTitleMap.get(addr) || '';
                                                const shortAddr = addr.length > 12 ? `${addr.slice(0, 6)}...${addr.slice(-4)}` : addr;
                                                return (
                                                    <div key={addr} className="flex items-center justify-between gap-3 rounded-xl bg-red-500/10 px-3 py-2 text-[11px] dark:bg-red-500/10">
                                                        <div className="min-w-0">
                                                            <div className="font-semibold text-red-800 dark:text-red-200 truncate">
                                                                {label || shortAddr}
                                                            </div>
                                                            {label ? (
                                                                <div className="mt-0.5 text-[10px] text-red-700/70 dark:text-red-200/60 truncate">
                                                                    {shortAddr}
                                                                </div>
                                                            ) : null}
                                                        </div>
                                                        <button
                                                            type="button"
                                                            disabled={!hasInitData}
                                                            onClick={() => handleBlacklist({ pool_address: addr, trading_pair: label || addr }, false)}
                                                            className="shrink-0 inline-flex items-center rounded-lg bg-white/60 px-2 py-1 text-[11px] font-semibold text-red-700 ring-1 ring-red-500/20 hover:bg-white/80 disabled:opacity-50 disabled:cursor-not-allowed dark:bg-white/10 dark:text-red-200 dark:ring-red-500/25 dark:hover:bg-white/15"
                                                        >
                                                            移除
                                                        </button>
                                                    </div>
                                                );
                                            })}
                                        </div>
                                    </div>
                                ) : null}
                                {/* 冷却中的交易对展示（移到底部） */}
                                {cooldowns.length > 0 ? (
                                    <div className="mt-4 rounded-2xl border border-amber-500/30 bg-amber-500/10 p-4 dark:border-amber-500/20 dark:bg-amber-500/5">
                                        <div className="flex items-center gap-2 text-sm font-semibold text-amber-700 dark:text-amber-300">
                                            <span>⏸️</span>
                                            <span>冷却中的代币</span>
                                            <span className="ml-auto text-[11px] font-normal text-amber-600 dark:text-amber-400">
                                                该代币相关池子禁止开仓
                                            </span>
                                        </div>
                                        <div className="mt-2 space-y-2">
                                            {cooldowns.map((cd, idx) => (
                                                <div key={cd.trading_pair + idx} className="flex items-center justify-between rounded-xl bg-amber-500/10 px-3 py-2 text-[11px] dark:bg-amber-500/10">
                                                    <div className="font-semibold text-amber-800 dark:text-amber-200">{cd.trading_pair}</div>
                                                    <div className="flex items-center gap-2">
                                                        <div className="flex items-center gap-2 text-amber-600 dark:text-amber-400">
                                                            <span>{cd.remaining_minutes}分钟后解除</span>
                                                            <span className="text-[10px]">({cd.expires_at})</span>
                                                        </div>
                                                        <button
                                                            type="button"
                                                            disabled={!hasInitData || Boolean(cooldownRemovingPair)}
                                                            onClick={() => handleRemoveCooldown(cd.trading_pair)}
                                                            className="shrink-0 inline-flex items-center rounded-lg bg-white/60 px-2 py-1 text-[11px] font-semibold text-amber-700 ring-1 ring-amber-500/20 hover:bg-white/80 disabled:opacity-50 disabled:cursor-not-allowed dark:bg-white/10 dark:text-amber-200 dark:ring-amber-500/25 dark:hover:bg-white/15"
                                                        >
                                                            {cooldownRemovingPair === String(cd.trading_pair || '').trim() ? '解除中...' : '解除'}
                                                        </button>
                                                    </div>
                                                </div>
                                            ))}
                                        </div>
                                    </div>
                                ) : null}
                            </>
                        )
                        : isSmartMoney
                            ? (
                                <SmartMoneyCard
                                    overview={smartMoney}
                                    loading={smartMoneyLoading}
                                    tick={tick}
                                    accentTheme={accentTheme}
                                    onNotice={showNotice}
                                    apiBaseUrl={apiBaseUrl}
                                    initData={initData}
                                    theme={theme}
                                    onQuickOpenPool={openPositionModal}
                                />
                            )
                            : !showAdmin && activeData
                                ? (
                                    <>
                                        {isPositions ? (
                                            <div
                                                className="grid grid-cols-3 gap-1 p-0.5 bg-zinc-100/80 rounded-xl dark:bg-[#16181d] shadow-inner ring-1 ring-zinc-200/50 dark:ring-black/20"
                                            >
                                                {POSITION_TASK_TABS.map((tab) => (
                                                    <button
                                                        key={tab.key}
                                                        type="button"
                                                        onClick={() => {
                                                            positionsTabTouchedRef.current = true;
                                                            setPositionsTaskTab(tab.key);
                                                            setSelectedTaskIds(new Set());
                                                            setBatchMode(false);
                                                        }}
                                                        aria-pressed={positionsTaskTab === tab.key}
                                                        className={`relative rounded-lg px-2.5 py-1 text-[12px] font-bold transition-all duration-300 ${positionsTaskTab === tab.key
                                                            ? 'bg-gradient-to-r from-blue-500 to-indigo-500 text-white shadow-md shadow-blue-500/20 dark:from-blue-600 dark:to-indigo-600 dark:text-white dark:shadow-blue-900/40 ring-1 ring-black/5 dark:ring-white/10'
                                                            : 'text-zinc-500 hover:text-zinc-700 dark:text-zinc-400 dark:hover:text-zinc-200 hover:bg-zinc-200/50 dark:hover:bg-white/5'
                                                            }`}
                                                    >
                                                        {tab.label}
                                                    </button>
                                                ))}
                                            </div>
                                        ) : null}

                                        {visibleTaskPositions.map((p) => (
                                            <PositionCard
                                                key={[
                                                    String(p?.chain || ''),
                                                    String(p?.version || ''),
                                                    String(p?.exchange || ''),
                                                    String(p?.pool_id || ''),
                                                    String(p?.position_id || ''),
                                                    String(p?.task_id || ''),
                                                ].join(':')}
                                                position={p}
                                                walletAddress={walletAddress}
                                                bnbBalance={bnbBalance}
                                                pollIntervalSec={pollIntervalSec}
                                                updatedAt={updatedAt}
                                                allowTaskActions={!showAdmin && hasInitData}
                                                onSetTaskPaused={handleSetTaskPaused}
                                                onStopTask={handleStopTask}
                                                onDeleteTask={handleDeleteTask}
                                                onUpdateTaskRange={openTaskRangeModal}
                                                batchMode={batchMode}
                                                isSelected={selectedTaskIds.has(p.task_id)}
                                                onToggleSelect={() => toggleTaskSelection(p.task_id)}
                                            />
                                        ))}

                                        {isPositions && positionsTaskTab === 'auto' ? (
                                            <AutoPnLCurveCard
                                                data={autoPnLCurve}
                                                loading={autoPnLCurveLoading}
                                                error={autoPnLCurveError}
                                                theme={theme}
                                            />
                                        ) : null}
                                    </>
                                )
                                : null}
            </div>

            {
                isPositions && activeData?.warnings?.length ? (
                    <div className="mt-4 rounded-2xl border border-amber-500/30 bg-amber-500/10 p-4 text-xs text-amber-700 dark:text-amber-200">
                        <div className="font-semibold">提示</div>
                        <ul className="mt-1 list-disc space-y-1 pl-4">
                            {activeData.warnings.map((w, i) => (
                                <li key={String(i)}>{w}</li>
                            ))}
                        </ul>
                    </div>
                ) : null
            }

            {
                poolSearchOpen ? (
                    <div className="fixed inset-0 z-[60]">
                        <button
                            type="button"
                            className="absolute inset-0 cursor-default bg-black/40"
                            onClick={closePoolSearch}
                            aria-label="Close search"
                        />
                        <div className="absolute inset-x-0 bottom-0 max-h-[85vh] overflow-y-auto rounded-t-2xl border border-zinc-200 bg-white p-4 shadow-2xl dark:border-white/10 dark:bg-[#111318] dark:shadow-none">
                            <div className="flex items-center justify-between">
                                <div className="inline-flex items-center justify-center rounded-xl border border-zinc-200 bg-zinc-100 p-2 text-zinc-700 dark:border-white/10 dark:bg-white/5 dark:text-white/80">
                                    <Icon path={icons.search} className="h-4 w-4" />
                                </div>
                                <button
                                    type="button"
                                    onClick={closePoolSearch}
                                    className="inline-flex h-9 w-9 items-center justify-center rounded-xl border border-zinc-200 bg-zinc-100 text-zinc-900 hover:bg-zinc-200 active:bg-zinc-200 dark:border-white/10 dark:bg-white/5 dark:text-white/80 dark:hover:bg-white/10 dark:active:bg-white/15"
                                    aria-label="Close"
                                >
                                    <Icon path={icons.close} className="h-5 w-5" />
                                </button>
                            </div>

                            <div className="mt-4 space-y-3 pb-20">
                                <div className="rounded-xl border border-zinc-200 bg-zinc-50 p-3 dark:border-white/10 dark:bg-[#0f1116]">
                                    <div className="text-[11px] text-zinc-500 dark:text-white/40">搜索池子 (池子ID/代币名称)</div>
                                    <div className="mt-2 flex items-center gap-2">
                                        <div className="text-[11px] text-zinc-500 dark:text-white/40">Chain</div>
                                        <select
                                            value={poolSearchChain}
                                            onChange={(e) => {
                                                setPoolSearchChain(e.target.value);
                                                setPoolSearchResults([]);
                                                setPoolSearchError('');
                                                setPoolSearchPerformed(false);
                                            }}
                                            disabled={!multiChainEnabled}
                                            className={`rounded-xl border border-zinc-200 bg-white/70 px-3 py-2 text-sm text-zinc-900 shadow-sm outline-none ring-0 ${brand.inputFocusClass} dark:border-white/10 dark:bg-white/5 dark:text-white/90`}
                                        >
                                            <option value="bsc">BSC</option>
                                            <option value="base">Base</option>
                                        </select>
                                    </div>
                                    <div className="mt-1 flex gap-2">
                                        <input
                                            ref={poolSearchInputRef}
                                            value={poolSearchQuery}
                                            onChange={(e) => {
                                                setPoolSearchQuery(e.target.value);
                                                setPoolSearchResults([]);
                                                setPoolSearchError('');
                                                setPoolSearchPerformed(false);
                                            }}
                                            onKeyDown={(e) => {
                                                if (e.key === 'Enter') {
                                                    e.preventDefault();
                                                    runPoolSearch();
                                                }
                                            }}
                                            className={`flex-1 rounded-xl border border-zinc-200 bg-white/70 px-3 py-2 text-sm text-zinc-900 shadow-sm outline-none ring-0 placeholder:text-zinc-400 ${brand.inputFocusClass} dark:border-white/10 dark:bg-white/5 dark:text-white/90 dark:placeholder:text-white/30`}
                                            placeholder="例如 USDT / WBNB / 0x..."
                                        />
                                        <button
                                            type="button"
                                            onClick={runPoolSearch}
                                            disabled={!hasInitData || poolSearchLoading}
                                            className={`shrink-0 rounded-xl px-3 py-2 text-sm font-semibold ring-1 transition ${!hasInitData || poolSearchLoading
                                                ? 'cursor-not-allowed bg-zinc-100 text-zinc-400 ring-zinc-200 dark:bg-white/5 dark:text-white/30 dark:ring-white/10'
                                                : `${brand.solidButtonClass} ${brand.solidRingClass}`
                                                }`}
                                        >
                                            {poolSearchLoading ? '搜索中...' : '搜索'}
                                        </button>
                                    </div>
                                    <div className="mt-2 text-[11px] text-zinc-500 dark:text-white/40">
                                        支持按池子ID和代币名称搜索，结果按 TVL 倒序，最多 10 条
                                    </div>
                                </div>

                                {!hasInitData ? (
                                    <div className="rounded-xl border border-amber-500/30 bg-amber-500/10 p-3 text-xs text-amber-700 dark:text-amber-200">
                                        未获取到 Telegram initData，请从机器人入口打开页面。
                                    </div>
                                ) : null}

                                {poolSearchError ? (
                                    <div className="rounded-xl border border-red-500/30 bg-red-500/10 p-3 text-xs text-red-700 dark:text-red-200">
                                        {poolSearchError}
                                    </div>
                                ) : null}

                                {poolSearchPerformed && !poolSearchLoading && !poolSearchError && poolSearchResults.length === 0 ? (
                                    <div className="rounded-xl border border-zinc-200 bg-white/70 p-3 text-xs text-zinc-500 dark:border-white/10 dark:bg-white/5 dark:text-white/60">
                                        未找到相关池子。
                                    </div>
                                ) : null}

                                {poolSearchResults.length > 0 ? (
                                    <div className="space-y-3">
                                        {poolSearchResults.map((pool, idx) => {
                                            const addr = String(pool?.pool_address || '').trim().toLowerCase();
                                            const key = `${String(pool?.protocol_version || '').trim()}:${addr || String(idx)}`;
                                            const isBlacklisted = addr ? blacklist.has(addr) : false;
                                            return (
                                                <HotPoolCard
                                                    key={key}
                                                    pool={pool}
                                                    metric={hotPoolsSort}
                                                    previousData={null}
                                                    rank={idx + 1}
                                                    accentTheme={accentTheme}
                                                    apiBaseUrl={apiBaseUrl}
                                                    isBlacklisted={isBlacklisted}
                                                    onOpenKline={setKlinePool}
                                                    onOpenPosition={selectPoolFromSearch}
                                                    onBlacklistRequest={openBlacklistPrompt}
                                                    chain={poolSearchChain}
                                                />
                                            );
                                        })}
                                    </div>
                                ) : null}
                            </div>
                        </div>
                    </div>
                ) : null
            }

            {
                hotPoolsFilterOpen ? (
                    <div className="fixed inset-0 z-[60]">
                        <button
                            type="button"
                            className="absolute inset-0 cursor-default bg-black/40"
                            onClick={() => setHotPoolsFilterOpen(false)}
                            aria-label="Close filter"
                        />
                        <div className="absolute inset-x-0 bottom-0 rounded-t-2xl border border-zinc-200 bg-white p-4 shadow-2xl dark:border-white/10 dark:bg-[#111318] dark:shadow-none">
                            <div className="flex items-center justify-between">
                                <div className="inline-flex items-center justify-center rounded-xl border border-zinc-200 bg-zinc-100 p-2 text-zinc-700 dark:border-white/10 dark:bg-white/5 dark:text-white/80">
                                    <Icon path={icons.filter} className="h-4 w-4" />
                                </div>
                                <button
                                    type="button"
                                    onClick={() => setHotPoolsFilterOpen(false)}
                                    className="inline-flex h-9 w-9 items-center justify-center rounded-xl border border-zinc-200 bg-zinc-100 text-zinc-900 hover:bg-zinc-200 active:bg-zinc-200 dark:border-white/10 dark:bg-white/5 dark:text-white/80 dark:hover:bg-white/10 dark:active:bg-white/15"
                                    aria-label="Close"
                                >
                                    <Icon path={icons.close} className="h-5 w-5" />
                                </button>
                            </div>

                            <div className="mt-4 space-y-4 pb-20">
                                <div className="rounded-xl border border-zinc-200 bg-zinc-50 p-3 dark:border-white/10 dark:bg-[#0f1116]">
                                    <div className="mt-1">
                                        <div className="text-[11px] text-zinc-500 dark:text-white/40">搜索 (交易对/地址)</div>
                                        <input
                                            value={hotPoolsFilterDraft.keyword}
                                            onChange={(e) => setHotPoolsFilterDraft((prev) => ({ ...prev, keyword: e.target.value }))}
                                            className={`mt-1 w-full rounded-xl border border-zinc-200 bg-white/70 px-3 py-2 text-sm text-zinc-900 shadow-sm outline-none ring-0 placeholder:text-zinc-400 ${brand.inputFocusClass} dark:border-white/10 dark:bg-white/5 dark:text-white/90 dark:placeholder:text-white/30`}
                                            placeholder="例如 USDT"
                                        />
                                    </div>
                                    <div className="mt-3 grid grid-cols-2 gap-3">
                                        <div>
                                            <div className="text-[11px] text-zinc-500 dark:text-white/40">手续费 ≥ (USD)</div>
                                            <input
                                                value={hotPoolsFilterDraft.minFees}
                                                onChange={(e) => setHotPoolsFilterDraft((prev) => ({ ...prev, minFees: e.target.value }))}
                                                inputMode="decimal"
                                                className={`mt-1 w-full rounded-xl border border-zinc-200 bg-white/70 px-3 py-2 text-sm text-zinc-900 shadow-sm outline-none ring-0 placeholder:text-zinc-400 ${brand.inputFocusClass} dark:border-white/10 dark:bg-white/5 dark:text-white/90 dark:placeholder:text-white/30`}
                                                placeholder={String(defaultHotPoolsFilter.minFees)}
                                            />
                                        </div>
                                        <div>
                                            <div className="text-[11px] text-zinc-500 dark:text-white/40">费用率 ≥ (%)</div>
                                            <input
                                                value={hotPoolsFilterDraft.minFeeRate}
                                                onChange={(e) => setHotPoolsFilterDraft((prev) => ({ ...prev, minFeeRate: e.target.value }))}
                                                inputMode="decimal"
                                                className={`mt-1 w-full rounded-xl border border-zinc-200 bg-white/70 px-3 py-2 text-sm text-zinc-900 shadow-sm outline-none ring-0 placeholder:text-zinc-400 ${brand.inputFocusClass} dark:border-white/10 dark:bg-white/5 dark:text-white/90 dark:placeholder:text-white/30`}
                                                placeholder={String(defaultHotPoolsFilter.minFeeRate)}
                                            />
                                        </div>
                                        <div>
                                            <div className="text-[11px] text-zinc-500 dark:text-white/40">TVL ≥ (USD)</div>
                                            <input
                                                value={hotPoolsFilterDraft.minTvl}
                                                onChange={(e) => setHotPoolsFilterDraft((prev) => ({ ...prev, minTvl: e.target.value }))}
                                                inputMode="decimal"
                                                className={`mt-1 w-full rounded-xl border border-zinc-200 bg-white/70 px-3 py-2 text-sm text-zinc-900 shadow-sm outline-none ring-0 placeholder:text-zinc-400 ${brand.inputFocusClass} dark:border-white/10 dark:bg-white/5 dark:text-white/90 dark:placeholder:text-white/30`}
                                                placeholder={String(defaultHotPoolsFilter.minTvl)}
                                            />
                                        </div>
                                        <div>
                                            <div className="text-[11px] text-zinc-500 dark:text-white/40">交易量 ≥ (USD)</div>
                                            <input
                                                value={hotPoolsFilterDraft.minVolume}
                                                onChange={(e) => setHotPoolsFilterDraft((prev) => ({ ...prev, minVolume: e.target.value }))}
                                                inputMode="decimal"
                                                className={`mt-1 w-full rounded-xl border border-zinc-200 bg-white/70 px-3 py-2 text-sm text-zinc-900 shadow-sm outline-none ring-0 placeholder:text-zinc-400 ${brand.inputFocusClass} dark:border-white/10 dark:bg-white/5 dark:text-white/90 dark:placeholder:text-white/30`}
                                                placeholder={String(defaultHotPoolsFilter.minVolume)}
                                            />
                                        </div>
                                    </div>

                                    <div className="mt-3 flex flex-wrap gap-2">
                                        <button
                                            type="button"
                                            onClick={applyHotPoolsFilter}
                                            className={`inline-flex items-center gap-2 rounded-xl px-3 py-2 text-xs font-semibold shadow-sm ${brand.solidButtonClass}`}
                                            aria-label="应用"
                                            title="应用"
                                        >
                                            <Icon path={icons.check} className="h-4 w-4" />
                                            应用
                                        </button>
                                        <button
                                            type="button"
                                            onClick={resetHotPoolsFilter}
                                            className="inline-flex items-center gap-2 rounded-xl bg-white/70 px-3 py-2 text-xs font-semibold text-zinc-700 ring-1 ring-zinc-200 hover:bg-white dark:bg-white/5 dark:text-white/70 dark:ring-white/10"
                                            aria-label="默认"
                                            title="默认"
                                        >
                                            <Icon path={icons.reset} className="h-4 w-4" />
                                            默认
                                        </button>
                                        <button
                                            type="button"
                                            onClick={clearHotPoolsFilter}
                                            className="inline-flex items-center gap-2 rounded-xl bg-white/70 px-3 py-2 text-xs font-semibold text-zinc-700 ring-1 ring-zinc-200 hover:bg-white dark:bg-white/5 dark:text-white/70 dark:ring-white/10"
                                            aria-label="清空条件"
                                            title="清空条件"
                                        >
                                            <Icon path={icons.close} className="h-4 w-4" />
                                            清空条件
                                        </button>
                                    </div>
                                </div>
                            </div>
                        </div>
                    </div>
                ) : null
            }

            {
                globalConfigOpen ? (
                    <div className="fixed inset-0 z-[60]">
                        <button
                            type="button"
                            className="absolute inset-0 cursor-default bg-black/40"
                            onClick={() => setGlobalConfigOpen(false)}
                            aria-label="关闭全局配置"
                        />
                        <div className="absolute inset-x-0 bottom-0 rounded-t-2xl border border-zinc-200 bg-white p-4 shadow-2xl dark:border-white/10 dark:bg-[#111318] dark:shadow-none">
                            <div className="flex items-center justify-between">
                                <div className="text-sm font-semibold text-zinc-900 dark:text-white/90">全局配置</div>
                                <button
                                    type="button"
                                    onClick={() => setGlobalConfigOpen(false)}
                                    className="inline-flex h-9 w-9 items-center justify-center rounded-xl border border-zinc-200 bg-zinc-100 text-zinc-900 hover:bg-zinc-200 active:bg-zinc-200 dark:border-white/10 dark:bg-white/5 dark:text-white/80 dark:hover:bg-white/10 dark:active:bg-white/15"
                                    aria-label="关闭"
                                >
                                    <Icon path={icons.close} className="h-5 w-5" />
                                </button>
                            </div>

                            <div className="mt-4 space-y-3">
                                {globalConfigError ? (
                                    <div className="rounded-xl border border-red-500/30 bg-red-500/10 p-3 text-xs text-red-700 dark:text-red-200">
                                        {globalConfigError}
                                    </div>
                                ) : null}
                                {globalConfigLoading && !globalConfig ? (
                                    <div className="rounded-xl border border-zinc-200 bg-white/70 p-3 text-xs text-zinc-500 dark:border-white/10 dark:bg-white/5 dark:text-white/60">
                                        加载中...
                                    </div>
                                ) : null}
                                {globalConfig ? (
                                    <div className="rounded-xl border border-zinc-200 bg-zinc-50 p-3 dark:border-white/10 dark:bg-[#0f1116]">
                                        <div className="grid grid-cols-2 gap-3 text-xs text-zinc-500 dark:text-white/50">
                                            <div>
                                                <div>再平衡超时</div>
                                                <div className="mt-1 text-sm font-semibold text-zinc-900 dark:text-white/80">
                                                    <NumberFlowValue value={rebalanceText} formatter={() => rebalanceText} />
                                                </div>
                                            </div>
                                            <div>
                                                <div>滑点</div>
                                                <div className="mt-1 text-sm font-semibold text-zinc-900 dark:text-white/80">
                                                    <NumberFlowValue value={slippageText} formatter={() => slippageText} />
                                                </div>
                                            </div>
                                            <div>
                                                <div>秒止损</div>
                                                <div className="mt-1 text-sm font-semibold text-zinc-900 dark:text-white/80">{formatOnOff(globalCfg.stop_loss_enabled)}</div>
                                            </div>
                                            <div>
                                                <div>秒止损阈值</div>
                                                <div className="mt-1 text-sm font-semibold text-zinc-900 dark:text-white/80">
                                                    <NumberFlowValue value={stopLossDelayText} formatter={() => stopLossDelayText} />
                                                </div>
                                            </div>
                                            <div>
                                                <div>复投</div>
                                                <div className="mt-1 text-sm font-semibold text-zinc-900 dark:text-white/80">{formatOnOff(globalCfg.auto_reinvest)}</div>
                                            </div>
                                            <div>
                                                <div>剩余资产容忍度</div>
                                                <div className="mt-1 text-sm font-semibold text-zinc-900 dark:text-white/80">
                                                    <NumberFlowValue value={residualText} formatter={() => residualText} />
                                                </div>
                                            </div>
                                            <div>
                                                <div>日志通知</div>
                                                <div className="mt-1 text-sm font-semibold text-zinc-900 dark:text-white/80">{formatOnOff(globalCfg.extra_notifications_enabled)}</div>
                                            </div>
                                            <div>
                                                <div>过滤中文代币</div>
                                                <div className="mt-1 text-sm font-semibold text-zinc-900 dark:text-white/80">{formatOnOff(globalCfg.filter_chinese_tokens)}</div>
                                            </div>
                                        </div>
                                    </div>
                                ) : null}
                            </div>

                            <div className="mt-4 flex flex-wrap gap-2">
                                <button
                                    type="button"
                                    onClick={loadGlobalConfig}
                                    disabled={globalConfigLoading}
                                    className={`inline-flex items-center gap-2 rounded-xl px-3 py-2 text-xs font-semibold ring-1 ${globalConfigLoading
                                        ? `cursor-not-allowed ${brand.solidButtonClass} ${brand.solidRingClass} opacity-50`
                                        : `${brand.solidButtonClass} ${brand.solidRingClass}`
                                        }`}
                                >
                                    刷新
                                </button>
                            </div>
                        </div>
                    </div>
                ) : null
            }

            {
                settingsOpen ? (
                    <div className="fixed inset-0 z-[60]">
                        <button
                            type="button"
                            className="absolute inset-0 cursor-default bg-black/40"
                            onClick={() => setSettingsOpen(false)}
                            aria-label="关闭设置"
                        />
                        <div className="absolute inset-x-0 bottom-0 rounded-t-2xl border border-zinc-200 bg-white p-4 shadow-2xl dark:border-white/10 dark:bg-[#111318] dark:shadow-none">
                            <div className="flex items-center justify-between">
                                <div className="text-sm font-semibold text-zinc-900 dark:text-white/90">设置</div>
                                <button
                                    type="button"
                                    onClick={() => setSettingsOpen(false)}
                                    className="inline-flex h-9 w-9 items-center justify-center rounded-xl border border-zinc-200 bg-zinc-100 text-zinc-900 hover:bg-zinc-200 active:bg-zinc-200 dark:border-white/10 dark:bg-white/5 dark:text-white/80 dark:hover:bg-white/10 dark:active:bg-white/15"
                                    aria-label="关闭"
                                >
                                    <Icon path={icons.close} className="h-5 w-5" />
                                </button>
                            </div>

                            <div className="mt-4 space-y-4">
                                <div className="rounded-xl border border-zinc-200 bg-zinc-50 p-3 dark:border-white/10 dark:bg-[#0f1116]">
                                    <div className="text-xs font-semibold text-zinc-900 dark:text-white/80">主色</div>
                                    <div className="mt-0.5 text-[11px] text-zinc-500 dark:text-white/40">默认新绿，也可以切回原来的绿色。</div>
                                    <div className="mt-3 flex flex-wrap gap-2">
                                        {ACCENT_THEME_OPTIONS.map((option) => {
                                            const active = accentTheme === option.key;
                                            return (
                                                <button
                                                    key={option.key}
                                                    type="button"
                                                    onClick={() => setAccentTheme(option.key)}
                                                    className={`inline-flex items-center gap-2 rounded-xl px-3 py-1.5 text-xs font-semibold ring-1 transition ${active
                                                        ? brand.softButtonClass
                                                        : 'bg-white/70 text-zinc-700 ring-zinc-200 hover:bg-white dark:bg-white/5 dark:text-white/70 dark:ring-white/10'
                                                        }`}
                                                >
                                                    <span className={`h-2.5 w-2.5 rounded-full ${option.key === 'lime' ? 'bg-[#bcff2f]' : 'bg-emerald-500'}`} />
                                                    {option.label}
                                                </button>
                                            );
                                        })}
                                    </div>
                                </div>
                                <div className="rounded-xl border border-zinc-200 bg-zinc-50 p-3 dark:border-white/10 dark:bg-[#0f1116]">
                                    <div className="text-xs font-semibold text-zinc-900 dark:text-white/80">自动刷新</div>
                                    <div className="mt-0.5 text-[11px] text-zinc-500 dark:text-white/40">
                                        当前：<NumberFlowValue value={settingsPollIntervalSec} formatOptions={{ maximumFractionDigits: 0 }} />s（
                                        {pollOverrideSec
                                            ? '自定义'
                                            : <>默认 <NumberFlowValue value={settingsServerPollIntervalSec} formatOptions={{ maximumFractionDigits: 0 }} />s</>}
                                        ）
                                    </div>
                                    <div className="mt-3 flex flex-wrap gap-2">
                                        {[5, 10, 15, 30, 60].map((sec) => (
                                            <button
                                                key={sec}
                                                type="button"
                                                onClick={() => setQuickPoll(sec)}
                                            className={`rounded-xl px-3 py-1.5 text-xs font-semibold ring-1 ${pollOverrideSec === sec
                                                    ? brand.softButtonClass
                                                    : 'bg-white/70 text-zinc-700 ring-zinc-200 hover:bg-white dark:bg-white/5 dark:text-white/70 dark:ring-white/10'
                                                    }`}
                                            >
                                                {sec}s
                                            </button>
                                        ))}
                                        <button
                                            type="button"
                                            onClick={clearPollOverride}
                                            className="rounded-xl bg-white/70 px-3 py-1.5 text-xs font-semibold text-zinc-700 ring-1 ring-zinc-200 hover:bg-white dark:bg-white/5 dark:text-white/70 dark:ring-white/10"
                                        >
                                            跟随默认
                                        </button>
                                    </div>

                                    <div className="mt-3 flex items-center gap-2">
                                        <input
                                            value={pollDraftSec}
                                            onChange={(e) => setPollDraftSec(e.target.value)}
                                            onKeyDown={(e) => {
                                                if (e.key === 'Enter') {
                                                    e.preventDefault();
                                                    applyPollDraft();
                                                }
                                            }}
                                            inputMode="numeric"
                                            className={`w-28 rounded-xl border border-zinc-200 bg-white/70 px-3 py-2 text-sm text-zinc-900 shadow-sm outline-none ring-0 placeholder:text-zinc-400 ${brand.inputFocusClass} dark:border-white/10 dark:bg-white/5 dark:text-white/90 dark:placeholder:text-white/30`}
                                            placeholder="1-300"
                                        />
                                        <button
                                            type="button"
                                            onClick={applyPollDraft}
                                            className={`rounded-xl px-3 py-2 text-sm font-semibold shadow-sm ${brand.solidButtonClass}`}
                                        >
                                            确定
                                        </button>
                                    </div>
                                </div>
                            </div>
                        </div>
                    </div>
                ) : null
            }

            {
                openPositionPool ? (
                    <BottomSheet
                        open={Boolean(openPositionPool)}
                        onClose={closeOpenPosition}
                        maxHeightClass="max-h-[85vh]"
                        className="bg-white dark:bg-[#111318] backdrop-blur-none"
                        headerClassName="px-4 pt-4 pb-3"
                        contentClassName="px-4 pb-[max(1.5rem,env(safe-area-inset-bottom))]"
                        title={
                            <div className="min-w-0">
                                <div className="truncate text-sm font-semibold text-zinc-900 dark:text-white/90">一键开仓</div>
                                <div className="mt-0.5 truncate text-[11px] font-medium text-zinc-500 dark:text-white/40">
                                    {openPositionPool?.trading_pair || '--'}
                                </div>
                            </div>
                        }
                    >
                        <div className="space-y-4">
                            {multiWalletEnabled ? (
                                <div className="rounded-xl border border-zinc-200 bg-zinc-50 p-3 dark:border-white/10 dark:bg-[#0f1116]">
                                    <div className="flex items-center justify-between gap-2">
                                        <div className="text-xs font-semibold text-zinc-900 dark:text-white/80">选择钱包</div>
                                        <div className="text-[11px] text-zinc-500 dark:text-white/40">
                                            {walletsLoading
                                                ? '加载中...'
                                                : [
                                                    String(walletsData?.chain || '').toUpperCase(),
                                                    walletsData?.native_symbol && walletsData?.stable_symbol
                                                        ? `${walletsData.native_symbol}/${walletsData.stable_symbol}`
                                                        : '',
                                                ].filter(Boolean).join(' · ')}
                                        </div>
                                    </div>

                                    {walletsError ? (
                                        <div className="mt-2 rounded-xl border border-red-500/30 bg-red-500/10 p-2 text-xs text-red-700 dark:text-red-200">
                                            {walletsError}
                                        </div>
                                    ) : null}

                                    {!walletsLoading && !walletsError && Array.isArray(walletsData?.wallets) && walletsData.wallets.length === 0 ? (
                                        <div className="mt-2 text-xs text-zinc-500 dark:text-white/50">未找到钱包</div>
                                    ) : null}

                                    <div className="mt-2 max-h-56 overflow-y-auto overscroll-contain space-y-2 pr-1">
                                        {(Array.isArray(walletsData?.wallets) ? walletsData.wallets : []).map((w) => {
                                            const id = String(w?.id || '').trim();
                                            const addr = String(w?.address || '').trim();
                                            const name = String(w?.name || '').trim();
                                            const shortAddr = addr.length > 12 ? `${addr.slice(0, 6)}...${addr.slice(-4)}` : addr;
                                            const selected = id && id === String(openPositionWalletId || '').trim();

                                            return (
                                                <button
                                                    key={id || addr}
                                                    type="button"
                                                    onClick={() => {
                                                        if (!id) return;
                                                        setOpenPositionWalletId(id);
                                                        storage.set(STORAGE_OPEN_POSITION_WALLET_ID, id);
                                                        setOpenPositionError('');
                                                        hapticSelection();
                                                    }}
                                                    className={`w-full rounded-xl border px-3 py-2 text-left transition ${selected
                                                        ? brand.selectionClass
                                                        : 'border-zinc-200 bg-white/70 hover:bg-white dark:border-white/10 dark:bg-white/5 dark:hover:bg-white/10'
                                                        }`}
                                                >
                                                    <div className="flex items-center justify-between gap-3">
                                                        <div className="min-w-0">
                                                            <div className="flex items-center gap-2">
                                                                <div className="truncate text-sm font-semibold text-zinc-900 dark:text-white/85">
                                                                    {name || shortAddr || `Wallet ${id}`}
                                                                </div>
                                                                {w?.is_default ? (
                                                                    <span className="shrink-0 rounded-full bg-zinc-500/10 px-2 py-0.5 text-[10px] font-semibold text-zinc-600 dark:text-white/60">
                                                                        默认
                                                                    </span>
                                                                ) : null}
                                                            </div>
                                                            <div className="mt-0.5 truncate text-[11px] text-zinc-500 dark:text-white/40">
                                                                {addr || '--'}
                                                            </div>
                                                        </div>
                                                        <div className="shrink-0 text-right">
                                                            <div className="text-xs font-semibold tabular-nums text-zinc-900 dark:text-white/85">
                                                                {String(w?.stable_balance ?? '--')} {walletsData?.stable_symbol || 'USDT'}
                                                            </div>
                                                            <div className="mt-0.5 text-[11px] tabular-nums text-zinc-500 dark:text-white/45">
                                                                {String(w?.native_balance ?? '--')} {walletsData?.native_symbol || 'BNB'}
                                                            </div>
                                                        </div>
                                                    </div>
                                                </button>
                                            );
                                        })}
                                    </div>
                                </div>
                            ) : null}
                            <div className="rounded-xl border border-zinc-200 bg-zinc-50 p-3 dark:border-white/10 dark:bg-[#0f1116]">
                                <div className="text-xs font-semibold text-zinc-900 dark:text-white/80">投入金额 (USDT)</div>
                                <input
                                    value={openPositionAmount}
                                    onChange={(e) => {
                                        setOpenPositionAmount(e.target.value);
                                        setOpenPositionError('');
                                    }}
                                    inputMode="decimal"
                                    className={`mt-2 w-full rounded-xl border border-zinc-200 bg-white/70 px-3 py-2 text-sm text-zinc-900 shadow-sm outline-none ring-0 placeholder:text-zinc-400 ${brand.inputFocusClass} dark:border-white/10 dark:bg-white/5 dark:text-white/90 dark:placeholder:text-white/30`}
                                    placeholder="例如 100"
                                />
                            </div>

                            <div className="rounded-xl border border-zinc-200 bg-zinc-50 p-3 dark:border-white/10 dark:bg-[#0f1116]">
                                <div className="text-xs font-semibold text-zinc-900 dark:text-white/80">自定义区间 (%)</div>
                                <div className="mt-2 grid grid-cols-2 gap-2">
                                    <input
                                        value={openPositionRangeLower}
                                        onChange={(e) => {
                                            setOpenPositionRangeLower(e.target.value);
                                            setOpenPositionError('');
                                        }}
                                        inputMode="decimal"
                                        className={`w-full rounded-xl border border-zinc-200 bg-white/70 px-3 py-2 text-sm text-zinc-900 shadow-sm outline-none ring-0 placeholder:text-zinc-400 ${brand.inputFocusClass} dark:border-white/10 dark:bg-white/5 dark:text-white/90 dark:placeholder:text-white/30`}
                                        placeholder="下限 %"
                                    />
                                    <input
                                        value={openPositionRangeUpper}
                                        onChange={(e) => {
                                            setOpenPositionRangeUpper(e.target.value);
                                            setOpenPositionError('');
                                        }}
                                        inputMode="decimal"
                                        className={`w-full rounded-xl border border-zinc-200 bg-white/70 px-3 py-2 text-sm text-zinc-900 shadow-sm outline-none ring-0 placeholder:text-zinc-400 ${brand.inputFocusClass} dark:border-white/10 dark:bg-white/5 dark:text-white/90 dark:placeholder:text-white/30`}
                                        placeholder="上限 %"
                                    />
                                </div>
                                <div className="mt-2 flex flex-wrap gap-1.5">
                                    {effectiveQuickRangeOptions.map((option) => (
                                        <button
                                            key={option.value}
                                            type="button"
                                            onClick={() => {
                                                if (option.value.includes(' ')) {
                                                    const parts = option.value.split(/\s+/);
                                                    setOpenPositionRangeLower(parts[0] || '');
                                                    setOpenPositionRangeUpper(parts[1] || '');
                                                } else {
                                                    const normalized = option.value.replace(/[^0-9.]/g, '');
                                                    setOpenPositionRangeLower(normalized);
                                                    setOpenPositionRangeUpper(normalized);
                                                }
                                                setOpenPositionError('');
                                            }}
                                            className="rounded-lg px-2 py-1 text-[11px] font-semibold text-amber-700 ring-1 ring-amber-500/30 bg-gradient-to-r from-amber-50 via-amber-100/60 to-yellow-100/60 hover:from-amber-100 hover:via-amber-200/70 hover:to-yellow-200/70 dark:text-amber-200 dark:ring-amber-400/30 dark:from-amber-500/10 dark:via-amber-400/10 dark:to-yellow-400/10"
                                        >
                                            {option.label}
                                        </button>
                                    ))}
                                </div>
                                <div className="mt-2 text-[11px] text-zinc-500 dark:text-white/40">
                                    请输入下限与上限百分比（如 1 / 3 表示下 1% 上 3%）。
                                </div>
                            </div>

                            {/* Smart Money Quick Ranges */}
                            {(() => {
                                const wallets = [];
                                if (!wallets.length) return null;

                                const rangeMap = new Map();
                                wallets.forEach(w => {
                                    const lower = Number(w?.price_lower ?? 0);
                                    const upper = Number(w?.price_upper ?? 0);
                                    if (Number.isFinite(lower) && lower > 0 && Number.isFinite(upper) && upper > 0) {
                                        const pct = ((Math.abs(upper - lower) / (upper + lower)) * 100);
                                        // key up to 1 decimal point to group similar ranges together
                                        const key = pct.toFixed(1);
                                        if (!rangeMap.has(key)) {
                                            rangeMap.set(key, pct);
                                        }
                                    }
                                });

                                const uniqueRanges = Array.from(rangeMap.values()).sort((a, b) => a - b);
                                if (!uniqueRanges.length) return null;

                                return (
                                    <div className="rounded-xl border border-zinc-200 bg-zinc-50 p-3 dark:border-white/10 dark:bg-[#0f1116]">
                                        <div className="flex items-center justify-between">
                                            <div className="text-xs font-semibold text-zinc-900 dark:text-white/80 flex items-center gap-1.5">
                                                <span className="flex h-4 w-4 items-center justify-center rounded-sm bg-blue-500/20 text-blue-600 dark:text-blue-300">
                                                    <Icon path={icons.layer} className="h-3 w-3" />
                                                </span>
                                                聪明钱区间快捷开单
                                            </div>
                                        </div>
                                        <div className="mt-2 text-[11px] text-zinc-500 dark:text-white/40">一键填充该池子中热门聪明钱开仓的具体区间：</div>
                                        <div className="mt-2.5 flex flex-wrap gap-2">
                                            {uniqueRanges.slice(0, 5).map(pct => {
                                                const half = pct / 2;
                                                return (
                                                    <React.Fragment key={pct}>
                                                        {pct >= 1.0 ? (
                                                            <button
                                                                type="button"
                                                                onClick={() => {
                                                                    setOpenPositionRangeLower(half.toFixed(2));
                                                                    setOpenPositionRangeUpper(half.toFixed(2));
                                                                    hapticSelection();
                                                                }}
                                                                className="rounded-lg border border-zinc-200 bg-white px-2 py-1 text-[11px] font-semibold text-zinc-700 hover:bg-zinc-50 dark:border-white/10 dark:bg-[#1a1c23] dark:text-white/80 dark:hover:bg-white/5 transition-colors"
                                                            >
                                                                ±{half.toFixed(2)}%
                                                            </button>
                                                        ) : null}
                                                        <button
                                                            type="button"
                                                            onClick={() => {
                                                                setOpenPositionRangeLower(pct.toFixed(2));
                                                                setOpenPositionRangeUpper(pct.toFixed(2));
                                                                hapticSelection();
                                                            }}
                                                            className="rounded-lg border border-blue-200 bg-blue-50 px-2 py-1 text-[11px] font-bold text-blue-700 hover:bg-blue-100 dark:border-blue-500/20 dark:bg-blue-500/10 dark:text-blue-300 dark:hover:bg-blue-500/20 transition-colors"
                                                        >
                                                            ±{pct.toFixed(2)}%
                                                        </button>
                                                    </React.Fragment>
                                                );
                                            })}
                                        </div>
                                    </div>
                                );
                            })()}

                            <div className="rounded-xl border border-zinc-200 bg-zinc-50 p-3 dark:border-white/10 dark:bg-[#0f1116]">
                                <div className="text-xs font-semibold text-zinc-900 dark:text-white/80">滑点 (%)</div>
                                <div className="mt-0.5 text-[11px] text-zinc-500 dark:text-white/40">不填则使用全局滑点，仅对本次开仓与后续再平衡生效。</div>
                                <input
                                    value={openPositionSlippage}
                                    onChange={(e) => {
                                        setOpenPositionSlippage(e.target.value);
                                        setOpenPositionError('');
                                    }}
                                    inputMode="decimal"
                                    className={`mt-2 w-full rounded-xl border border-zinc-200 bg-white/70 px-3 py-2 text-sm text-zinc-900 shadow-sm outline-none ring-0 placeholder:text-zinc-400 ${brand.inputFocusClass} dark:border-white/10 dark:bg-white/5 dark:text-white/90 dark:placeholder:text-white/30`}
                                    placeholder="例如 0.5（可选）"
                                />
                            </div>



                            {openPositionError ? (
                                <div className="rounded-xl border border-red-500/30 bg-red-500/10 p-3 text-xs text-red-700 dark:text-red-200">
                                    {openPositionError}
                                </div>
                            ) : null}
                            <button
                                type="button"
                                onClick={handleOpenPosition}
                                disabled={openPositionLoading}
                                className={`w-full rounded-xl px-3 py-2 text-sm font-semibold shadow-sm transition ${openPositionLoading
                                    ? 'cursor-not-allowed bg-[#bcff2f]/55 text-[#182108]'
                                    : brand.solidButtonClass
                                    }`}
                            >
                                {openPositionLoading ? '开仓中...' : '确认开仓'}
                            </button>
                        </div>
                    </BottomSheet>
                ) : null
            }

            {
                taskRangeEdit ? (
                    <div className="fixed inset-0 z-[60]">
                        <button
                            type="button"
                            className="absolute inset-0 bg-black/40"
                            onClick={closeTaskRangeModal}
                            aria-label="关闭修改区间"
                        />
                        <div className="absolute inset-x-0 bottom-0 rounded-t-2xl border border-zinc-200 bg-white p-4 shadow-2xl dark:border-white/10 dark:bg-[#111318] dark:shadow-none">
                            <div className="flex items-center justify-between gap-2">
                                <div className="min-w-0">
                                    <div className="text-sm font-semibold text-zinc-900 dark:text-white/90">修改再平衡参数</div>
                                    <div className="mt-0.5 text-[11px] text-zinc-500 dark:text-white/40 truncate">
                                        {taskRangeEdit?.title || '--'}
                                    </div>
                                </div>
                                <button
                                    type="button"
                                    onClick={closeTaskRangeModal}
                                    className="inline-flex h-9 w-9 items-center justify-center rounded-xl border border-zinc-200 bg-zinc-100 text-zinc-900 hover:bg-zinc-200 active:bg-zinc-200 dark:border-white/10 dark:bg-white/5 dark:text-white/80 dark:hover:bg-white/10 dark:active:bg-white/15"
                                    aria-label="关闭"
                                    disabled={taskRangeLoading}
                                >
                                    <Icon path={icons.close} className="h-5 w-5" />
                                </button>
                            </div>

                            <div className="mt-4 space-y-4">
                                <div className="rounded-xl border border-zinc-200 bg-zinc-50 p-3 dark:border-white/10 dark:bg-[#0f1116]">
                                    <div className="text-xs font-semibold text-zinc-900 dark:text-white/80">新区间 (%)</div>
                                    <div className="mt-2 grid grid-cols-2 gap-2">
                                        <input
                                            value={taskRangeLower}
                                            onChange={(e) => {
                                                setTaskRangeLower(e.target.value);
                                                setTaskRangeError('');
                                            }}
                                            inputMode="decimal"
                                            className={`w-full rounded-xl border border-zinc-200 bg-white/70 px-3 py-2 text-sm text-zinc-900 shadow-sm outline-none ring-0 placeholder:text-zinc-400 ${brand.inputFocusClass} dark:border-white/10 dark:bg-white/5 dark:text-white/90 dark:placeholder:text-white/30`}
                                            placeholder="下限 %"
                                        />
                                        <input
                                            value={taskRangeUpper}
                                            onChange={(e) => {
                                                setTaskRangeUpper(e.target.value);
                                                setTaskRangeError('');
                                            }}
                                            inputMode="decimal"
                                            className={`w-full rounded-xl border border-zinc-200 bg-white/70 px-3 py-2 text-sm text-zinc-900 shadow-sm outline-none ring-0 placeholder:text-zinc-400 ${brand.inputFocusClass} dark:border-white/10 dark:bg-white/5 dark:text-white/90 dark:placeholder:text-white/30`}
                                            placeholder="上限 %"
                                        />
                                    </div>
                                    <div className="mt-2 text-[11px] text-zinc-500 dark:text-white/40">
                                        修改后的区间将在【下次再平衡时】生效。
                                    </div>
                                </div>

                                <div className="rounded-xl border border-zinc-200 bg-zinc-50 p-3 dark:border-white/10 dark:bg-[#0f1116]">
                                    <div className="text-xs font-semibold text-zinc-900 dark:text-white/80">下次重平衡金额 (USDT)</div>
                                    <div className="mt-2">
                                        <input
                                            value={taskRangeAmount}
                                            onChange={(e) => {
                                                setTaskRangeAmount(e.target.value);
                                                setTaskRangeError('');
                                            }}
                                            inputMode="decimal"
                                            className={`w-full rounded-xl border border-zinc-200 bg-white/70 px-3 py-2 text-sm text-zinc-900 shadow-sm outline-none ring-0 placeholder:text-zinc-400 ${brand.inputFocusClass} dark:border-white/10 dark:bg-white/5 dark:text-white/90 dark:placeholder:text-white/30`}
                                            placeholder="USDT 金额"
                                        />
                                    </div>
                                    <div className="mt-2 text-[11px] text-zinc-500 dark:text-white/40">
                                        当前持仓不会直接变动，金额和区间都将在【下次再平衡时】生效。
                                    </div>
                                </div>

                                {taskRangeError ? (
                                    <div className="rounded-xl border border-red-500/30 bg-red-500/10 p-3 text-xs text-red-700 dark:text-red-200">
                                        {taskRangeError}
                                    </div>
                                ) : null}
                                <button
                                    type="button"
                                    onClick={submitTaskRange}
                                    disabled={taskRangeLoading}
                                    className={`w-full rounded-xl px-3 py-2 text-sm font-semibold shadow-sm transition ${taskRangeLoading
                                        ? `${brand.solidButtonClass} cursor-not-allowed opacity-60`
                                        : brand.solidButtonClass
                                        }`}
                                >
                                    {taskRangeLoading ? '保存中...' : '确认修改'}
                                </button>
                            </div>
                        </div>
                    </div>
                ) : null
            }

            {
                blacklistPrompt ? (
                    <div className="fixed inset-0 z-[65] flex items-end justify-center sm:items-center sm:p-4">
                        <button
                            type="button"
                            className="absolute inset-0 bg-black/60 backdrop-blur-sm"
                            onClick={closeBlacklistPrompt}
                            aria-label="取消"
                        />
                        <div className="relative w-full max-w-md overflow-hidden rounded-t-2xl sm:rounded-2xl border border-red-500/20 bg-white p-4 shadow-2xl dark:border-red-500/20 dark:bg-[#111318]">
                            <div className="flex items-start gap-3">
                                <div className="flex h-11 w-11 items-center justify-center rounded-2xl bg-red-500/15 text-red-600 ring-1 ring-red-500/30 dark:text-red-200">
                                    <Icon path={icons.alert} className="h-6 w-6" />
                                </div>
                                <div className="min-w-0">
                                    <div className="text-base font-extrabold text-zinc-900 dark:text-white/90">加入黑名单</div>
                                    <div className="mt-1 text-xs text-zinc-500 dark:text-white/50">
                                        将池子加入黑名单后会阻止相关池子开仓
                                    </div>
                                </div>
                                <button
                                    type="button"
                                    onClick={closeBlacklistPrompt}
                                    className="ml-auto inline-flex h-8 w-8 items-center justify-center rounded-lg border border-zinc-200 bg-zinc-100 text-zinc-900 hover:bg-zinc-200 active:bg-zinc-200 dark:border-white/10 dark:bg-white/5 dark:text-white/80 dark:hover:bg-white/10 dark:active:bg-white/15"
                                    aria-label="关闭"
                                >
                                    <Icon path={icons.close} className="h-4 w-4" />
                                </button>
                            </div>

                            <div className="mt-4 rounded-2xl border border-red-500/20 bg-red-500/10 p-3">
                                <div className="flex items-center justify-between gap-3">
                                    <div className="min-w-0">
                                        <div className="text-sm font-semibold text-red-800 dark:text-red-200 truncate">
                                            {blacklistPromptPair || '未知池子'}
                                        </div>
                                        <div className="mt-0.5 text-[11px] text-red-700/70 dark:text-red-200/70">
                                            {blacklistPromptAddrShort || '--'}
                                        </div>
                                    </div>
                                    <div className="shrink-0 rounded-lg bg-red-500/15 px-2 py-1 text-[10px] font-semibold text-red-700 dark:text-red-200">
                                        长按触发
                                    </div>
                                </div>
                            </div>

                            <div className="mt-3 space-y-2 text-xs text-zinc-600 dark:text-white/60">
                                <div className="flex items-start gap-2">
                                    <span className="mt-0.5 inline-flex h-4 w-4 items-center justify-center rounded-full bg-red-500/15 text-red-600 dark:text-red-200">1</span>
                                    <span>包含当前代币的所有池子将被禁止开仓。</span>
                                </div>
                                <div className="flex items-start gap-2">
                                    <span className="mt-0.5 inline-flex h-4 w-4 items-center justify-center rounded-full bg-zinc-500/15 text-zinc-600 dark:text-white/60">2</span>
                                    <span>解除黑名单请前往「监控」页面。</span>
                                </div>
                            </div>

                            <div className="mt-4 flex items-center gap-2">
                                <button
                                    type="button"
                                    onClick={closeBlacklistPrompt}
                                    disabled={blacklistPromptLoading}
                                    className="flex-1 rounded-xl border border-zinc-200 bg-white px-3 py-2 text-sm font-semibold text-zinc-700 hover:bg-zinc-50 active:bg-zinc-100 disabled:cursor-not-allowed disabled:opacity-60 dark:border-white/10 dark:bg-white/5 dark:text-white/70 dark:hover:bg-white/10 dark:active:bg-white/15"
                                >
                                    取消
                                </button>
                                <button
                                    type="button"
                                    onClick={confirmBlacklistPrompt}
                                    disabled={blacklistPromptLoading}
                                    className={`flex-1 rounded-xl px-3 py-2 text-sm font-semibold text-white shadow-sm transition ${blacklistPromptLoading
                                        ? 'cursor-not-allowed bg-red-500/60'
                                        : 'bg-red-500 hover:bg-red-600 active:bg-red-700'
                                        }`}
                                >
                                    {blacklistPromptLoading ? '处理中...' : '确认加入'}
                                </button>
                            </div>
                        </div>
                    </div>
                ) : null
            }

            {
                confirmState ? (
                    <div className="fixed inset-0 z-[60] flex items-end sm:items-center justify-center sm:p-4">
                        <button
                            type="button"
                            className="absolute inset-0 bg-black/60 backdrop-blur-sm"
                            onClick={() => closeConfirm(false)}
                            aria-label="取消"
                        />
                        <div className="relative w-full max-w-md overflow-hidden rounded-t-2xl sm:rounded-2xl border border-zinc-200 bg-white p-4 shadow-2xl dark:border-white/10 dark:bg-[#111318]">
                            <div className="flex items-center justify-between gap-2">
                                <div className="text-sm font-semibold text-zinc-900 dark:text-white/90">{confirmState.title}</div>
                                <button
                                    type="button"
                                    onClick={() => closeConfirm(false)}
                                    className="inline-flex h-8 w-8 items-center justify-center rounded-lg border border-zinc-200 bg-zinc-100 text-zinc-900 hover:bg-zinc-200 active:bg-zinc-200 dark:border-white/10 dark:bg-white/5 dark:text-white/80 dark:hover:bg-white/10 dark:active:bg-white/15"
                                    aria-label="关闭"
                                >
                                    <Icon path={icons.close} className="h-4 w-4" />
                                </button>
                            </div>
                            {confirmState.message ? (
                                <div className="mt-2 text-sm text-zinc-600 whitespace-pre-line dark:text-white/60">
                                    {confirmState.message}
                                </div>
                            ) : null}
                            <div className="mt-4 flex items-center gap-2">
                                <button
                                    type="button"
                                    onClick={() => closeConfirm(false)}
                                    className="flex-1 rounded-xl border border-zinc-200 bg-white px-3 py-2 text-sm font-semibold text-zinc-700 hover:bg-zinc-50 active:bg-zinc-100 dark:border-white/10 dark:bg-white/5 dark:text-white/70 dark:hover:bg-white/10 dark:active:bg-white/15"
                                >
                                    {confirmState.cancelText || '取消'}
                                </button>
                                <button
                                    type="button"
                                    onClick={() => closeConfirm(true)}
                                    className={`flex-1 rounded-xl px-3 py-2 text-sm font-semibold ${confirmButtonClass}`}
                                >
                                    {confirmState.confirmText || '确认'}
                                </button>
                            </div>
                        </div>
                    </div>
                ) : null
            }

            {/* Bottom Navigation */}
            <div className="fixed bottom-0 left-0 right-0 z-50 pointer-events-none pb-[max(0.75rem,env(safe-area-inset-bottom))] px-4">
                <nav className="pointer-events-auto max-w-[400px] mx-auto flex items-center justify-between rounded-full border border-zinc-200/60 bg-white/95 px-3 py-2.5 shadow-2xl backdrop-blur-xl dark:border-white/10 dark:bg-[#1a1c23]/95 dark:shadow-black/70 ring-1 ring-black/5 dark:ring-white/5">
                    {topNavItems.map((item) => {
                        const isActive = viewMode === item.key;
                        let iconPath = icons.bot;
                        if (item.key === 'hot_pools') iconPath = icons.fire;
                        if (item.key === 'positions') iconPath = icons.chart;
                        if (item.key === 'monitor') iconPath = icons.alert;
                        if (item.key === 'smart_money') iconPath = icons.bot;
                        if (item.key === 'admin') iconPath = icons.gear;

                        return (
                            <button
                                key={item.key}
                                type="button"
                                onClick={() => setViewMode(item.key)}
                                aria-pressed={isActive}
                                className={`relative flex flex-col items-center justify-center rounded-full px-4 py-1.5 transition-all duration-300 ${isActive
                                    ? brand.navActiveClass
                                    : 'text-zinc-400 hover:text-zinc-600 dark:text-zinc-500 dark:hover:text-zinc-300'
                                    }`}
                            >
                                <Icon path={iconPath} className={`h-5 w-5 transition-transform duration-300 ${isActive ? 'scale-110 mb-0.5' : 'mb-0 scale-100'}`} />
                                {isActive && <span className="text-[10px] font-bold tracking-wide mt-0.5">{item.label}</span>}
                            </button>
                        );
                    })}
                </nav>
            </div>

            <KlineModal
                open={Boolean(klinePool)}
                onClose={() => setKlinePool(null)}
                apiBaseUrl={apiBaseUrl}
                theme={theme}
                pool={klinePool}
                chain={klinePool?.chain || hotPoolsData?.chain || 'bsc'}
            />

            {operationProgress && (
                <StepProgressModal
                    operation={operationProgress.operation}
                    progress={operationProgress}
                    onClose={() => {
                        const op = operationProgress;
                        setOperationProgress(null);
                        if (op?.status === 'done' && op?.operation === 'open_position') {
                            setOpenPositionPool(null);
                            resetOpenPositionDraft();
                        }
                    }}
                />
            )}
        </div >
    );
}
