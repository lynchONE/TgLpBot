import React, { useEffect, useMemo, useRef, useState, useCallback } from 'react';
import HotPoolCard from './components/HotPoolCard.jsx';
import KlineModal from './components/KlineModal.jsx';
import PositionCard from './components/PositionCard.jsx';
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
    fetchAdminRealtimePositions,
    fetchAdminRealtimeUsers,
    fetchGlobalConfig,
    fetchWallets,
    fetchHotPools,
    fetchSearchPools,
    fetchMe,
    fetchRealtimePositions,
    openPosition,
    updateTaskRange,
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

function localizeWebAppError(message, allowEmptyInitData = false) {
    const text = String(message || '').trim();
    if (!text) return '';
    if (text.includes('missing initData')) {
        if (allowEmptyInitData) {
            return '缺少 Telegram initData。本地浏览器调试时，请在 backend/.env 中设置 TELEGRAM_WEBAPP_ALLOW_EMPTY_INITDATA=1，并重启后端。';
        }
        return '缺少 Telegram initData，请从 Telegram 内打开 Mini App。';
    }
    if (text.includes('invalid initData')) {
        return 'Telegram initData 校验失败，请检查 TELEGRAM_BOT_TOKEN 是否正确。';
    }
    return text;
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
];const DEFAULT_WEB_WORKBENCH_WIDGETS = WEB_WORKBENCH_WIDGETS.map((item) => item.key);

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

function buildTopNavItems({ isAdmin }) {
    const items = [
        { key: 'hot_pools', label: '热门池子' },
        { key: 'positions', label: '仓位' },
    ];
    if (isAdmin) items.push({ key: 'admin', label: '管理' });
    return items;
}
const HOT_POOL_SORT_TABS = [
    { key: 'fees', label: '手续费' },
    { key: 'fee_rate', label: '费率' },
    { key: 'volume', label: '交易量' },
];
export default function App() {
    const initData = useInitData();
    const tick = useTick(); // 闁诲骸婀遍崑鐐差渻閸岀偛绫嶉柛顐ｆ礃鐎殿參鏌ㄥ☉妯绘拱闁伙讣绱曠划鏃堝箳閹惧鍑介梺鍝勫€块。锔剧博閺夋垟鏋?
    const [me, setMe] = useState(null);
    const [data, setData] = useState(null);
    const [error, setError] = useState('');
    const [loading, setLoading] = useState(false);
    const pollRef = useRef(null);
    const [viewMode, setViewMode] = useState('hot_pools');

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
    // 婵烇絽娲︾换鍌炴偤閵婏妇鈻斿┑鐘辫兌椤忚鲸绻涢崱顓犵？闁稿骸缍婂濠氬Ω閿旂偓寤洪柣搴㈢⊕閸旀牠寮抽悢鐓庣妞ゆ洖妫涚粈澶愭煟椤剙濡虹紒顭戝墰閹峰鏁嶉崟顓熸瘓闂佸憡鐟﹂敋閻?
    const previousHotPoolsDataRef = useRef({});
    const [klinePool, setKlinePool] = useState(null);
    const [openPositionPool, setOpenPositionPool] = useState(null);
    const [openPositionAmount, setOpenPositionAmount] = useState('');
    const [openPositionRangeLower, setOpenPositionRangeLower] = useState('');
    const [openPositionRangeUpper, setOpenPositionRangeUpper] = useState('');
    const [openPositionRangeUpperAuto, setOpenPositionRangeUpperAuto] = useState(true);
    const [openPositionSlippage, setOpenPositionSlippage] = useState('');
    const [openPositionAllowSwap, setOpenPositionAllowSwap] = useState(false);
    const [openPositionError, setOpenPositionError] = useState('');
    const [openPositionLoading, setOpenPositionLoading] = useState(false);
    const [operationProgress, setOperationProgress] = useState(null);
    const [walletsData, setWalletsData] = useState(null);
    const [walletsError, setWalletsError] = useState('');
    const [walletsLoading, setWalletsLoading] = useState(false);
    const [openPositionWalletId, setOpenPositionWalletId] = useState(() => storage.get(STORAGE_OPEN_POSITION_WALLET_ID) || '');

    const [taskRangeEdit, setTaskRangeEdit] = useState(null);
    const [taskRangeLower, setTaskRangeLower] = useState('');
    const [taskRangeUpper, setTaskRangeUpper] = useState('');
    const [taskRangeUpperAuto, setTaskRangeUpperAuto] = useState(true);
    const [taskRangeAmount, setTaskRangeAmount] = useState('');
    const [taskRangeError, setTaskRangeError] = useState('');
    const [taskRangeLoading, setTaskRangeLoading] = useState(false);

    // 婵帗绋掗崹鐢稿箖閺囥垹纭€闁哄洨濮寸瑧闂?
    const [blacklist, setBlacklist] = useState(new Set());
    // 闂佸憡鍔曢崲鎻掔暤閸儱绀嗘俊銈呭閳ь剙鍟撮幃鈺呮嚋绾版ê浜?
    const [cooldowns, setCooldowns] = useState([]);
    const [cooldownRemovingPair, setCooldownRemovingPair] = useState('');

    const [adminUsers, setAdminUsers] = useState([]);
    const [adminUsersError, setAdminUsersError] = useState('');
    const [adminUsersLoading, setAdminUsersLoading] = useState(false);
    const [adminSelectedUserId, setAdminSelectedUserId] = useState(null);
    const [adminPositions, setAdminPositions] = useState(null);
    const [adminPositionsError, setAdminPositionsError] = useState('');
    const [adminPositionsLoading, setAdminPositionsLoading] = useState(false);
    const adminUsersPollRef = useRef(null);
    const adminPositionsPollRef = useRef(null);
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

    // 闂佸憡姊绘慨鎯归崶銊︿氦婵炴垶锚椤斿﹪鏌ｅΟ鍨厫闁?
    const [pollProgress, setPollProgress] = useState(0);
    const pollProgressRef = useRef(null);
    const lastPollTimeRef = useRef(Date.now());
    const brand = useMemo(() => getBrandTheme(accentTheme), [accentTheme]);

    // 闂佸綊娼х紞濠囧闯濞差亜绠肩€广儱瀚粙濠囨煟濡灝鐓愰柍?
    const [batchMode, setBatchMode] = useState(false);
    const [selectedTaskIds, setSelectedTaskIds] = useState(new Set());
    const [batchLoading, setBatchLoading] = useState(false);

    const serverPollIntervalSec = Math.max(1, Number(data?.poll_interval_sec || adminPositions?.poll_interval_sec || 1));
    const pollIntervalSec = Math.max(1, Number(pollOverrideSec || serverPollIntervalSec || 1));
    const adminListPollSec = Math.max(3, pollIntervalSec);
    const isAdmin = Boolean(me?.is_admin || data?.is_admin || adminPositions?.is_admin);
    const showAdmin = isAdmin && viewMode === 'admin';
    const isHotPools = viewMode === 'hot_pools';
    const isPositions = viewMode === 'positions';
    const topNavItems = useMemo(
        () => buildTopNavItems({ isAdmin }),
        [isAdmin],
    );
    const showWalletSummaryCard = !showAdmin && !isHotPools;
    const hotPoolsDefaultPollSec = 10;
    const hotPoolsPollIntervalSec = Math.max(5, Number(pollOverrideSec || hotPoolsDefaultPollSec));
    const settingsPollIntervalSec = isHotPools ? hotPoolsPollIntervalSec : pollIntervalSec;
    const settingsServerPollIntervalSec = isHotPools ? hotPoolsDefaultPollSec : serverPollIntervalSec;

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
            return true;
        });
    }, [positions]);

    const visibleTaskPositions = visiblePositions;

    // 婵炲濮寸花鑲╁垝閵婏附濯寸€广儱妫涢埀顒夊灠椤?pool_address -> position_usd 闂佸搫瀚慨鎾儍閻樼粯鏅柛顐犲灪閺嗗繐霉濠婂啴顎楁繝鈧鍫熷€绘い鎾卞灪閿涘本鎱ㄩ崷顓炐㈤柣鈩冪懄缁嬪绻濋崘鈹炬灃缂備讲鍋撻柣鎴灻惁顔济归悩铏鞍闁绘牭绲跨划鐢稿箻閸涱垳顦?
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

    // 闂佸吋鍎抽崲鑼躲亹閸ヮ剚鍋ㄩ柕濠忕畱閻撴洖霉閻樿櫕灏紓宥呮噺缁嬪顢橀悩宕囨殸濠殿喖婀辨慨鎾偤濞嗘挸鎹堕柡澶嬪缁插鏌涢幒鎿冩畽闁靛棗鍟撮弫宥夊醇閵忊剝娈㈡繛瀛樼矊缁ㄨ偐妲愰崜浣虹＜?hot_pools API闂?
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
        // 1. 闂佺绻愰悧鎰崲濡吋鍋樼€光偓閳ь剟鐛崶顒€瀚夊璺虹灱閹斤綁姊?
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
                // 婵犵鈧啿鈧綊鎮樻径鎰仺闁靛绠戦悡鏇㈡煛閸繍妲风紒顔哄妽閹峰懎顓奸崨顔垮惈闁哄鏅滈悷銈夋煂濠婂唭褔鎮╅懠顒佹啢闂佹寧绋戦惌渚€鎮滈敂鑺ヤ氦闁搞儮鏅濋幗锝夋⒑椤愩埄妾х紒杈ㄧ懄閹便劎鈧綆鍓涢惌鎺楁煛閸曨偄鈷旈柕鍥ㄥ哺閺?
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

        // 2. 婵炴垶鎸鹃崕銈夋儊閳╁啰鈻旀い蹇撴噽濞笺劑鎮楀☉娆忓闁秆冿躬瀹?userPositionUsd 闁诲孩绋掗〃鍡涱敊?
        const enriched = filtered.map(pool => {
            const addr = String(pool?.pool_address || '').toLowerCase();
            return {
                ...pool,
                userPositionUsd: positionsPoolMap.get(addr) || 0
            };
        });

        // 3. 闂佸湱鍎ょ敮鎺旇姳椤撱垺鏅慨姗嗗幗缁犳帒霉閻樿櫕灏紓宥呮嚇閹啴宕熼鐘崇€俊鐐€涢鎰濠靛绠板璺侯槺濞夈垹霉閿濆懐效闁革絿鍋撻敍鎰攽鐎ｎ偒鈧牠骞栭弶鎴︾崪缂侀亶浜跺畷妤呭嫉閻㈢數鈻忔繛锝呮处缁诲啰鈧灚妫冨畷銏ゆ偄缁楄　鍋撴惔銏″劅?
        return enriched.sort((a, b) => {
            if (a.userPositionUsd > 0 && b.userPositionUsd <= 0) return -1;
            if (b.userPositionUsd > 0 && a.userPositionUsd <= 0) return 1;
            if (a.userPositionUsd > 0 && b.userPositionUsd > 0) {
                return b.userPositionUsd - a.userPositionUsd;
            }
            return 0; // 婵烇絽娲︾换鍐偓鍨瀹曘垽鎮㈢粭琛″亾鎼淬垺鍎?
        });
    }, [hotPoolsFilter, hotPoolsFilterEnabled, hotPoolsRows, positionsPoolMap]);

    // 闂佸搫顑呯€氼剛绱撻幘缁樺€绘い鎾卞灪閿涘本鎱ㄩ崷顓炐㈤柣鈩冪懇閹啴宕熼銏犳綉闂佸憡鐟ュ鍫曞汲閻旂厧绠叉い鏃囧Г琛奸柣?(protocol_version:pool_address -> previous data)
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
    }, [isAdmin, viewMode]);

    useEffect(() => {
        const tg = getTelegramWebApp();
        const savedTheme = storage.get(STORAGE_THEME);
        if (savedTheme === 'light' || savedTheme === 'dark') {
            setTheme(savedTheme);
        } else {
            // 婵帗绋掗…鍫ヮ敇缂佹ɑ濯撮悹鎭掑妽閺嗗繘鏌￠崱姗嗘畽濠㈢懓锕ョ粙澶嬬節濮樺吋姣?
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

    // 闁哄鏅滅粙鎴犫偓瑙勫▕瀵爼妾辨い鎾存倐瀵喚鎹勯悜妯煎綔 - 闂佸搫瀚晶浠嬪Φ濮橆厽濮滄い鏃€顑欓崵鍕煕婵犲啫绗╂い鎾存倐瀵?
    useEffect(() => {
        const currentPollSec = isHotPools ? hotPoolsPollIntervalSec : pollIntervalSec;

        const updateProgress = () => {
            const elapsed = Date.now() - lastPollTimeRef.current;
            const progress = Math.min(100, (elapsed / (currentPollSec * 1000)) * 100);
            setPollProgress(progress);
        };

        // 缂備焦鏌ㄩ鍛暤閸℃稑鍗抽悗娑櫳戦悡鈧繛鎴炴尨閸嬫挻绻?
        updateProgress();

        // 濠?00ms闂佸搫娲ら悺銊╁蓟婵犲啯浜ゆ繛鎴灻?
        pollProgressRef.current = setInterval(updateProgress, 100);

        return () => {
            if (pollProgressRef.current) clearInterval(pollProgressRef.current);
        };
    }, [isHotPools, hotPoolsPollIntervalSec, pollIntervalSec]);

    // 闁哄鍎愰崰娑㈩敋濡ゅ啠鍋撻悷鐗堟拱闁搞劍宀稿顕€宕奸弴鐐搭仧缂傚倸鍠氶崰娑氭崲濡粯鍎?
    const lastUpdatedAtRef = useRef(null);
    useEffect(() => {
        // 婵炶揪缍€濞夋洟寮?updatedAt 闂佸搫顦崕閬嶅垂娴犲妫樻い鎾跺枑濞堝爼鏌熺拠鈥虫灍婵″弶鎮傚畷銉╂晜閻愵剙鐒稿┑顔界箰缁叉儳煤閸ф妫樺Λ棰佽兌閸?
        const currentUpdatedAt = data?.updated_at || hotPoolsData?.updated_at;
        if (currentUpdatedAt && currentUpdatedAt !== lastUpdatedAtRef.current) {
            lastPollTimeRef.current = Date.now();
            setPollProgress(0);
            // 闂佸憡鐟禍婊冿耿椤忓牊鍎戦柣鏂垮閸斺偓闂佸搫鐗嗛ˇ浼村蓟婵犲洤鏋侀柣妤€鐗嗙粊锕傛煛閸愨晛鍔剁悮婵嬫煕濞嗘劕鐏辩悮婵嬫偡濞嗗浚妲哥€殿喛濮ら敍?
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

    // 闂佺粯鍩堥崣鍐ㄎ涢鈧晥闁绘灏欓幗宥夋煛娴ｅ搫顣肩€规挷鐒﹂幈銊р偓锝庡墰閻帡鏌涢弮鍌毿繛鏉戞喘閺佸秹宕奸妷顔芥闂佸憡姊绘慨鎯归崶顒佹櫖?
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
                // 闂侀潻璐熼崝宥吤洪崸妤€妫橀柣妤€鐗婂▓鍫曟煙鐠団€虫灆缂佺媴缍佸畷婊冾吋韫囨洜顦繛锝呮处缁诲倿鎮洪妸銊ｄ汗闁规儳鍟块·鍛存煛娴ｅ搫顣肩€规挷鐒﹂幏鍛煥閳ь剛鎷归悢鐓庡偍闁糕剝顨呴拺澶愭煛娴ｅ搫顣肩€规挷绶氶弫宥夊醇濠婂啠鏋忛梺?setState 闂佹悶鍎抽崑鐘绘儍閻斿吋鐒奸柛顭戝枛鐢娊姊婚崒銈呭箹閻庡灚锕㈤獮蹇涘垂椤旇偐鍘掗梺鍝勫敳閸曨剚顔嶉梺纭咁嚃閸ｎ垳妲?
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
            setPoolSearchError('请输入池子地址或关键词。');
            setPoolSearchResults([]);
            setPoolSearchPerformed(false);
            return;
        }
        if (!hasInitData) {
            setPoolSearchError('缺少 Telegram initData。本地浏览器调试时，请在 backend/.env 中设置 TELEGRAM_WEBAPP_ALLOW_EMPTY_INITDATA=1。');
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
        { label: '闁?%', value: '闁?' },
        { label: '闁?%', value: '闁?' },
        { label: '闁?%', value: '闁?' },
        { label: '闁?0%', value: '闁?0' },
        { label: '闁?0%', value: '闁?0' },
        { label: '闁?0%', value: '闁?0' },
    ];
    const effectiveQuickRangeOptions = useMemo(() => quickRangeOptions.slice(0, 6), []);

    const parseRangeInput = (lowerRaw, upperRaw) => {
        const lower = Number(String(lowerRaw || '').trim());
        const upper = Number(String(upperRaw || '').trim());
        if (!Number.isFinite(lower) || !Number.isFinite(upper)) return null;
        return { lower: Math.abs(lower), upper: Math.abs(upper) };
    };

    const handleOpenPositionRangeLowerChange = useCallback((value) => {
        setOpenPositionRangeLower((prevLower) => {
            if (
                openPositionRangeUpperAuto ||
                String(openPositionRangeUpper || '').trim() === '' ||
                String(openPositionRangeUpper) === String(prevLower)
            ) {
                setOpenPositionRangeUpper(value);
            }
            return value;
        });
        setOpenPositionError('');
    }, [openPositionRangeUpper, openPositionRangeUpperAuto]);

    const handleOpenPositionRangeUpperChange = useCallback((value) => {
        setOpenPositionRangeUpperAuto(false);
        setOpenPositionRangeUpper(value);
        setOpenPositionError('');
    }, []);

    const handleTaskRangeLowerChange = useCallback((value) => {
        setTaskRangeLower((prevLower) => {
            if (
                taskRangeUpperAuto ||
                String(taskRangeUpper || '').trim() === '' ||
                String(taskRangeUpper) === String(prevLower)
            ) {
                setTaskRangeUpper(value);
            }
            return value;
        });
        setTaskRangeError('');
    }, [taskRangeUpper, taskRangeUpperAuto]);

    const handleTaskRangeUpperChange = useCallback((value) => {
        setTaskRangeUpperAuto(false);
        setTaskRangeUpper(value);
        setTaskRangeError('');
    }, []);

    const resetOpenPositionDraft = () => {
        setOpenPositionAmount('');
        setOpenPositionRangeLower('');
        setOpenPositionRangeUpper('');
        setOpenPositionRangeUpperAuto(true);
        setOpenPositionSlippage('');

        setOpenPositionError('');
    };

    const openPositionModal = (pool) => {
        const addr = String(pool?.pool_address || '').trim().toLowerCase();
        if (addr && blacklist.has(addr)) {
            hapticNotification('error');
            showNotice('This pool is already blacklisted.', 'error');
            return;
        }
        let chain = String(pool?.chain || hotPoolsData?.chain || 'bsc').trim().toLowerCase() || 'bsc';
        if (!multiChainEnabled) chain = userDefaultChain;
        const poolVersion = String(pool?.protocol_version || pool?.pool_version || '').trim().toLowerCase();
        setOpenPositionPool({
            ...pool,
            chain,
            ...(poolVersion ? { protocol_version: poolVersion, pool_version: poolVersion } : {}),
        });
        resetOpenPositionDraft();
    };

    const closeOpenPosition = () => {
        if (openPositionLoading) return;
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
            setOpenPositionError('Telegram initData is required.');
            return;
        }
        const poolAddr = String(openPositionPool?.pool_address || '').trim().toLowerCase();
        if (poolAddr && blacklist.has(poolAddr)) {
            setOpenPositionError('This pool is blacklisted.');
            return;
        }
        const amount = Number(String(openPositionAmount || '').trim());
        if (!Number.isFinite(amount) || amount <= 0) {
            setOpenPositionError('Enter a valid amount.');
            return;
        }
        const range = parseRangeInput(openPositionRangeLower, openPositionRangeUpper);
        if (!range || range.lower <= 0 || range.upper <= 0 || range.lower >= 100 || range.upper >= 100) {
            setOpenPositionError('Range must be between 0 and 100.');
            return;
        }

        const slippageRaw = String(openPositionSlippage || '').trim();
        let slippage = undefined;
        if (slippageRaw) {
            const v = Number(slippageRaw);
            if (!Number.isFinite(v) || v < 0 || v > 100) {
                setOpenPositionError('Slippage must be between 0 and 100.');
                return;
            }
            slippage = v;
        }

        if (multiWalletEnabled) {
            if (walletsLoading) {
                setOpenPositionError('钱包列表仍在加载，请稍后再试。');
                return;
            }
            if (walletsError) {
                setOpenPositionError(walletsError);
                return;
            }
            const list = Array.isArray(walletsData?.wallets) ? walletsData.wallets : [];
            if (list.length === 0) {
                setOpenPositionError('No available wallet found.');
                return;
            }
            if (list.length > 1) {
                const wid = Number(openPositionWalletId);
                if (!Number.isFinite(wid) || wid <= 0) {
                    setOpenPositionError('Select a wallet.');
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
            setOpenPositionError(msg || 'Open position failed.');
            setOperationProgress(prev => prev?.operation === 'open_position'
                ? { ...prev, status: 'error', error: msg || 'Open position failed.' } : prev);
        } finally {
            setOpenPositionLoading(false);
        }
    };

    // 婵帗绋掗崹鐢稿箖閺囥垹纭€闁哄洨鍠愰幆娆徝归敐鍡欑煀妞わ腹鏅犻幃?
    const handleBlacklist = useCallback(async (pool, add) => {
        if (!hasInitData || !pool?.pool_address) return;
        const addr = String(pool.pool_address).trim().toLowerCase();
        try {
            if (add) {
                await addToBlacklist({ apiBaseUrl, initData, poolAddress: addr });
                setBlacklist(prev => new Set(prev).add(addr));
                hapticNotification('success');
                showNotice(`Added ${pool?.trading_pair || addr} to blacklist.`, 'warning');
            } else {
                await removeFromBlacklist({ apiBaseUrl, initData, poolAddress: addr });
                setBlacklist(prev => {
                    const next = new Set(prev);
                    next.delete(addr);
                    return next;
                });
                hapticNotification('success');
                showNotice(`Removed ${pool?.trading_pair || addr} from blacklist.`, 'info');
            }
        } catch (e) {
            hapticNotification('error');
            showNotice(`婵帗绋掗崹鐢稿箖閺囥垹纭€闁哄洨鍠愰幆娆徝归敐鍡欑煀闁靛洦鍨归幏? ${e?.message || e}`, 'error');
        }
    }, [apiBaseUrl, initData, hasInitData]);

    const openBlacklistPrompt = useCallback((pool) => {
        const addr = String(pool?.pool_address || '').trim().toLowerCase();
        if (!addr) return;
        if (!hasInitData) {
            showNotice('Telegram initData is required.', 'error');
            return;
        }
        if (blacklist.has(addr)) {
            showNotice('This pool is already blacklisted.', 'info');
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

    // 闂佸憡姊绘慨鎯归崶顭戞付闁瑰瓨绻冮崐鎶芥煕濡や焦绀€闁割煈浜為幃?
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

    // 闂佸憡姊绘慨鎯归崶顒€绀冪€瑰嫭婢樼粊閬嶆煕閹烘搩娈欓柕?
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
            title: 'Remove cooldown',
            message: `Remove cooldown for ${pair}?\nThis action cannot be undone.`,
            confirmText: 'Remove',
            tone: 'danger',
        });
        if (!ok) return;

        setCooldownRemovingPair(pair);
        try {
            const resp = await removeCooldown({ apiBaseUrl, initData, tradingPair: pair });
            showNotice(resp?.message || `閻庤鐡曠亸顏囶暰闂傚倸瀚ㄩ崐鏇㈠窗鎼淬劌纭€? ${pair}`, 'success');
            loadCooldowns();
        } catch (e) {
            showNotice(`闁荤喐鐟辩紞浣糕枍鎼淬劌绀冪€瑰嫭婢樼粊鍗烆熆閹壆绨块悷? ${String(e?.message || e)}`, 'error');
        } finally {
            setCooldownRemovingPair('');
        }
    }, [apiBaseUrl, initData, hasInitData, cooldownRemovingPair, loadCooldowns, requestConfirm]);

    // 闂佸憡甯楃换鍌烇綖閹版澘绀岄柡宥冨妽椤ρ囨煕閺冨倸鞋婵炴潙娲﹂—鈧柟瀛樼箖閸婃娊鏌涘Δ浣圭闁硅渹鍗冲畷妯虹暋閺夎法銈遍梺鍛婂笚椤ㄥ濡?
    useEffect(() => {
        if (hasInitData) {
            loadBlacklist();
            loadCooldowns();
        }
    }, [hasInitData, loadBlacklist, loadCooldowns]);

    const loadGlobalConfig = async () => {
        if (!hasInitData) {
            setGlobalConfigError('Telegram initData is required.');
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

    const handleSetTaskPaused = async (taskId, paused) => {
        if (!hasInitData || showAdmin) return;
        const id = Number(taskId);
        if (!Number.isFinite(id) || id <= 0) return;

        const wantPaused = Boolean(paused);
        const ok = await requestConfirm({
            title: wantPaused ? 'Pause task' : 'Resume task',
            message: wantPaused
                ? 'Pause this task?\nIt will stop creating new orders.'
                : 'Resume this task?\nIt will continue creating new orders.',
            confirmText: wantPaused ? 'Pause' : 'Resume',
        });
        if (!ok) return;

        try {
            await setTaskPaused({ apiBaseUrl, initData, taskId: id, paused: wantPaused });
            showNotice(wantPaused ? 'Task paused.' : 'Task resumed.', 'success');
        } catch (e) {
            showNotice(String(e?.message || e), 'error');
        }
    };

    const handleStopTask = async (taskId) => {
        if (!hasInitData || showAdmin) return;
        const id = Number(taskId);
        if (!Number.isFinite(id) || id <= 0) return;

        const ok = await requestConfirm({
            title: 'Stop position',
            message: 'Stop this position?\nIt will close the related task and settle outstanding value in USDT.',
            confirmText: 'Stop',
            tone: 'danger',
        });
        if (!ok) return;

        setOperationProgress({ operation: 'close_position', taskId: id, currentStep: 0, totalSteps: 4, status: 'active', error: '' });
        try {
            const resp = await stopTask({ apiBaseUrl, initData, taskId: id });
            if (resp?.status === 'stopped' || resp?.pending === false) {
                // Already stopped or immediate stop 闂?all done
                setOperationProgress(prev => prev?.operation === 'close_position'
                    ? { ...prev, currentStep: 3, status: 'done' } : prev);
            } else {
                // Async 闂?advance to step 1 only if WS hasn't already gone further
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
                showNotice(`Task #${id} was not found. It may have already been closed or deleted.`, 'warning');
                try {
                    const resp = await fetchRealtimePositions({ apiBaseUrl, initData });
                    setData(resp);
                } catch {
                    // ignore
                }
                return;
            }
            setOperationProgress(prev => prev?.operation === 'close_position'
                ? { ...prev, status: 'error', error: msg || 'Stop position failed.' } : prev);
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
            title: 'Delete task',
            message: 'Delete this task?\nThis action cannot be undone and will remove its configuration.',
            confirmText: 'Delete',
            tone: 'danger',
        });
        if (!ok) return;

        try {
            const resp = await deleteTask({ apiBaseUrl, initData, taskId: id });
            showNotice(resp?.message || 'Task deleted.', 'success');
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
            title: String(position?.title || '').trim() || `婵炲濮鹃褎鎱?#${id}`,
        });
        setTaskRangeLower(Number.isFinite(low) && low > 0 ? String(low) : '');
        setTaskRangeUpper(Number.isFinite(up) && up > 0 ? String(up) : '');
        setTaskRangeUpperAuto(true);
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
        setTaskRangeUpperAuto(true);
        setTaskRangeAmount('');
        setTaskRangeError('');
    };

    const submitTaskRange = async () => {
        if (!taskRangeEdit) return;
        if (!hasInitData || showAdmin) return;

        const range = parseRangeInput(taskRangeLower, taskRangeUpper);
        const amount = Number(String(taskRangeAmount || '').trim());
        if (!Number.isFinite(amount) || amount <= 0) {
            setTaskRangeError('Amount must be greater than 0 USDT.');
            return;
        }
        if (!range || range.lower <= 0 || range.upper <= 0 || range.lower >= 100 || range.upper >= 100) {
            setTaskRangeError('Range must be between 0 and 100.');
            return;
        }

        const ok = await requestConfirm({
            title: 'Update task range',
            message: 'Update the task range?\nThe bot will use the new range and amount after confirmation.',
            confirmText: 'Update',
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
            showNotice('任务区间已更新。', 'success');
            setTaskRangeEdit(null);
            setTaskRangeLower('');
            setTaskRangeUpper('');
            setTaskRangeUpperAuto(true);
            setTaskRangeAmount('');
        } catch (e) {
            setTaskRangeError(String(e?.message || e || 'Update failed.'));
        } finally {
            setTaskRangeLoading(false);
        }
    };

    // 闂佸綊娼х紞濠囧闯濞差亜绠肩€广儱瀚粙濠囨煕閹达絽袚闁?
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
            `闂佸綊娼х紞濠囧闯?{paused ? '闂佸搫妫楅崐鍛婄? : '闂佽鍘归崹褰捤?}闁诲海鎳撻張顒勫垂濮樿埖鏅慨姗嗗幖閻忓洭鏌?${successCount}闂佹寧绋戦懟顖炲Φ閹寸姵瀚?${failCount}`,
            failCount === 0 ? 'success' : 'warning'
        );
    };

    // 闁荤姳绶ょ槐鏇㈡偩婵犳艾瀚夋い鎺嗗亾婵犫偓閹绢喖绀嗛梺鍨儐閻撯偓闂佸憡鑹惧ù椋庡垝閳╁啯浜ら柛銉㈡櫆閻ｈ京绱掓径瀣瑲闁?
    const localUpdateSecAgo = useMemo(() => {
        const elapsed = tick - lastPollTimeRef.current;
        return Math.max(0, Math.floor(elapsed / 1000));
    }, [tick]);

    const moduleMetaByMode = useMemo(() => ({
        hot_pools: {
            title: '热门池子',
            icon: icons.fire,
            subtitle: `5 分钟 | ${hotPoolsData ? `${localUpdateSecAgo} 秒前更新` : hotPoolsLoading ? '加载中...' : '未加载'} | 轮询 ${hotPoolsPollIntervalSec}s`,
        },
        positions: {
            title: '仓位',
            icon: icons.bot,
            subtitle: walletAddress ? `钱包 ${walletAddress.slice(0, 6)}...${walletAddress.slice(-4)}` : '钱包未连接',
        },
        admin: {
            title: '管理',
            icon: icons.gear,
            subtitle: adminSelectedUser
                ? `闂佹椿娼块崝宥夊春濞戙垺鏅?{formatUserLabel(adminSelectedUser)}`
                : adminUsersLoading && adminUsers.length === 0
                    ? '闂佸憡姊绘慨鎯归崶顒佸仺闁靛绠戦悡鏇炩槈?..'
                    : adminUsers.length
                        ? `闂侀潻璐熼崝搴ㄥ吹鎼淬劍鍋ㄩ柕濠忕畱閻撴洟鏌?{adminUsers.length}`
                        : '暂无可管理用户',
        },
    }), [
        adminSelectedUser,
        adminUsers,
        adminUsersLoading,
        hotPoolsData,
        hotPoolsLoading,
        hotPoolsPollIntervalSec,
        localUpdateSecAgo,
        tick,
        walletAddress,
    ]);
    const activeModuleMeta = moduleMetaByMode[viewMode] || moduleMetaByMode.positions;

    const hasAdminPositions = Boolean(adminPositions);
    const adminSummaryPlaceholder = adminSelectedUserId
        ? adminPositionsLoading
            ? '闂佸憡姊绘慨鎯归崶顒佸仺闁靛绠戦悡鏇灻归悩铏矮缂傚秴鎳忕粙?..'
            : '该用户暂无仓位数据'
        : '请先选择一个管理员用户';
    const showEmptyPositions = isPositions && Boolean(activeData) && visiblePositions.length === 0;
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
    const blacklistPromptPool = blacklistPrompt?.pool || null;
    const blacklistPromptPair = String(blacklistPromptPool?.trading_pair || '').trim();
    const blacklistPromptAddr = String(blacklistPromptPool?.pool_address || '').trim().toLowerCase();
    const blacklistPromptAddrShort = blacklistPromptAddr.length > 12
        ? `${blacklistPromptAddr.slice(0, 6)}...${blacklistPromptAddr.slice(-4)}`
        : blacklistPromptAddr;

    const initDataMissing = viewMode !== 'hot_pools' && !hasInitData;
    const noticeClass = notice?.tone === 'error'
        ? 'bg-red-600 text-white'
        : notice?.tone === 'success'
            ? brand.successNoticeClass
            : 'bg-zinc-900 text-white dark:bg-white/10 dark:text-white';
    const globalCfg = globalConfig || {};
    const rebalanceText = Number.isFinite(Number(globalCfg.rebalance_timeout))
        ? `${Number(globalCfg.rebalance_timeout)} s`
        : '--';
    const stopLossDelayText = Number.isFinite(Number(globalCfg.stop_loss_delay_seconds))
        ? `${Number(globalCfg.stop_loss_delay_seconds)} s`
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

    const hotPoolsErrorText = useMemo(
        () => localizeWebAppError(hotPoolsError, allowEmptyInitData),
        [hotPoolsError, allowEmptyInitData],
    );
    const activeErrorText = useMemo(
        () => localizeWebAppError(activeError, allowEmptyInitData),
        [activeError, allowEmptyInitData],
    );

    return (
        <div className={`min-h-screen max-w-[720px] px-4 py-4 mx-auto ${isPositions ? 'pb-[calc(96px+env(safe-area-inset-bottom))]' : 'pb-[calc(80px+env(safe-area-inset-bottom))]'}`}>
            {notice ? (
                <div className="fixed left-1/2 top-[calc(env(safe-area-inset-top)+64px)] z-50 w-[calc(100%-2rem)] max-w-md -translate-x-1/2">
                    <div className={`rounded-xl px-3 py-2 text-sm font-semibold shadow-lg ${noticeClass}`}>
                        {notice.message}
                    </div>
                </div>
            ) : null}
            {/* 婵＄偑鍊曢悥濂稿磿閹绢喖绀夐柣妯煎劋缁佷即寮堕埡鍌溾槈閻庤濞婂?*/}
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
                            aria-label="Toggle theme"
                        >
                            <Icon path={theme === 'dark' ? icons.moon : icons.sun} className="h-5 w-5" />
                        </button>
                        <button
                            type="button"
                            onClick={() => setSettingsOpen(true)}
                            className="inline-flex h-9 w-9 items-center justify-center rounded-xl border border-zinc-200 bg-zinc-100 text-zinc-900 shadow-sm hover:bg-zinc-200 active:bg-zinc-200 dark:border-white/10 dark:bg-white/5 dark:text-white/80 dark:hover:bg-white/10 dark:active:bg-white/15"
                            aria-label="打开设置"
                        >
                            <Icon path={icons.gear} className="h-5 w-5" />
                        </button>
                    </div>
                </div>


                {showAdmin ? (
                    <ModuleHeader
                        title="管理面板"
                        subtitle={hasAdminPositions
                            ? adminSelectedUser
                                ? `用户 ${formatUserLabel(adminSelectedUser)}`
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
                                <div className="text-[11px] text-zinc-500 dark:text-white/40">总资产</div>
                                <div className={`mt-0.5 text-2xl font-extrabold tabular-nums text-zinc-900 ${brand.textClass}`}>
                                    <NumberFlowValue value={totalUsd} formatter={(v) => formatUsd(v)} />
                                </div>
                                <div className="mt-1 text-[11px] text-zinc-500 dark:text-white/40 tabular-nums">
                                    <NumberFlowValue value={bnbBalance} formatter={() => String(bnbBalance ?? '0')} /> BNB
                                    {typeof bnbUsd === 'number' ? <> | <NumberFlowValue value={bnbUsd} formatter={(v) => formatUsd(v)} /></> : ''}
                                </div>
                            </div>
                        ) : null}
                    </ModuleHeader>
                ) : isHotPools ? (
                    <ModuleHeader
                        title={hotPoolsSort === 'fee_rate' ? '费率排行' : hotPoolsSort === 'volume' ? '交易量排行' : '手续费排行'}
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
                                    aria-label="搜索池子"
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
                                    aria-label="筛选"
                                    title="筛选"
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
                    <div className="mt-3 overflow-hidden rounded-[24px] border border-zinc-200/80 bg-[radial-gradient(circle_at_top_left,_rgba(16,185,129,0.16),_transparent_42%),linear-gradient(135deg,_rgba(255,255,255,0.92),_rgba(244,247,255,0.78))] p-3 shadow-[0_16px_40px_-24px_rgba(15,23,42,0.38)] dark:border-white/10 dark:bg-[radial-gradient(circle_at_top_left,_rgba(16,185,129,0.16),_transparent_38%),linear-gradient(135deg,_rgba(24,27,32,0.98),_rgba(15,17,21,0.94))] dark:shadow-[0_18px_48px_-28px_rgba(0,0,0,0.7)]">
                        <div className="flex flex-col gap-2.5">
                            <div className="flex items-start justify-between gap-3">
                                <div className="min-w-0 flex-1">
                                    <div className="inline-flex items-center rounded-full border border-emerald-500/20 bg-emerald-500/10 px-2.5 py-1 text-[10px] font-semibold uppercase tracking-[0.24em] text-emerald-700 dark:border-emerald-400/20 dark:bg-emerald-400/10 dark:text-emerald-300/90">
                                        仓位总览
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
                                                {typeof bnbUsd === 'number' ? <> | <NumberFlowValue value={bnbUsd} formatter={(v) => formatUsd(v)} /></> : null}
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
                ) : null}

            </header>

            {
                isHotPools && hotPoolsErrorText ? (
                    <div className="mb-4 rounded-2xl border border-red-500/30 bg-red-500/10 p-4 text-sm text-red-700 dark:text-red-200">
                        {hotPoolsErrorText}
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
                        闂佸搫妫楅崐鐟拔涢妶澶嬪€绘い鎾卞灪閿涘本鎱ㄩ崷顓炐㈤柣鈩冪懇瀵偊鎮ч崼婵堛偊闂?
                    </div>
                ) : null
            }

            {
                isHotPools && !hotPoolsLoading && !hotPoolsError && hotPoolsData && hotPoolsRows.length > 0 && hotPoolsFilterEnabled && hotPoolsVisibleRows.length === 0 ? (
                    <div className="mb-4 rounded-2xl border border-zinc-200 bg-white/70 p-6 text-sm text-zinc-500 dark:border-white/10 dark:bg-white/5 dark:text-white/60">
                        缂備焦绋掗惄顖炲焵椤掆偓椤︻垶骞冨Δ鍛辈闁稿本绋掗敓銉╂煟閹搭厼寮俊顐墮鏁堥柣妯垮皺閹藉秹鏌℃担鍝勵暭鐎规挷绶氭俊?
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
                        闁荤姴娲╁〒鍦垝?Telegram 闂佸搫鐗嗛幖顐⑩枍閹烘挾顩叉繛宸簻濞咃繝鏌?闁诲骸婀遍崑鐐差渻閸屾稓顩烽柟鎹愬皺缁?闂佸湱顭堥ˇ鐢稿箰閹惰棄绠ラ柟鎯х－绾捐顪冮妶鍥ㄦ毈婵為棿鍗抽弫宥夊醇濠靛洤鍓ㄩ梺鍛婂笚閻燂箑螞閵堝應鏋栭柡鍥ｆ閸ゃ垽鏌涘▎鎰ⅶ缂傚秵锕㈤幆鍐礋椤忓棙灏濇繛杈剧到缁嬪嫮妲愬璺何?
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

            {/* 闂佸綊娼х紞濠囧闯濞差亜绠肩€广儱瀚粙濠勨偓瑙勬偠閸庨亶宕ｉ崸妤€鍐€?*/}
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
                            {batchMode ? '退出批量模式' : '批量模式'}
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
                                    {batchLoading ? '处理中...' : '暂停所选'}
                                </button>
                                <button
                                    type="button"
                                    onClick={() => batchPauseTasks(false)}
                                    disabled={selectedTaskIds.size === 0 || batchLoading}
                                    className="inline-flex items-center rounded-xl bg-emerald-500/15 px-2 py-1 text-xs font-semibold text-emerald-700 hover:bg-emerald-500/25 disabled:opacity-50 dark:text-emerald-200"
                                >
                                    闂佸綊娼х紞濠囧闯濞差亜绠掗柕蹇曞濡?
                                </button>
                            </div>
                        )}
                    </div>
                )
            }

            {/* 缂備礁顦…宄扳枍鎼淬垻顩?闂佸搫妫楅崐鐟拔涢妶澶嬪殜妞ゅ繐瀚婵炲濮鹃褎鎱?闂佸湱绮崝妤呭Φ?*/}

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
                        : !showAdmin && activeData
                                ? (
                                    <>
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
                            aria-label="关闭搜索"
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
                                    aria-label="关闭搜索"
                                >
                                    <Icon path={icons.close} className="h-5 w-5" />
                                </button>
                            </div>

                            <div className="mt-4 space-y-3 pb-20">
                                <div className="rounded-xl border border-zinc-200 bg-zinc-50 p-3 dark:border-white/10 dark:bg-[#0f1116]">
                                    <div className="text-[11px] text-zinc-500 dark:text-white/40">闂佺懓鍚嬬划搴ㄥ磼閵娿儛褔鎮╅懠顒佹啢 (濠殿喖婀辨慨鎾偤濮楊湂/婵炲濯寸徊鐣岀博閻㈢瑙︾€广儱娉?</div>
                                    <div className="mt-2 flex items-center gap-2">
                                        <div className="text-[11px] text-zinc-500 dark:text-white/40">链</div>
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
                                            placeholder="婵炴挻鑹鹃鍛淬€?USDT / WBNB / 0x..."
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
                                        闂佽　鍋撴い鏍ㄧ☉閻︻噣鏌熺粙娆炬Ч闁活偅绮庨埀顒佺⊕閳ь剚顑忛梺鍛婄矊婵傛棃宕鍌涙殰濞达絽鎲￠崐宕囩磼婢跺苯顣奸柟顔肩－濡叉劙锝為鍓ь槷缂傚倷鐒﹂幐濠氭倶婢舵劕绠?TVL 闂佺锕ョ敮鎺旇姳椤撱垺鏅悘鐐靛亾娴犳ê顭?10 闂?
                                    </div>
                                </div>

                                {!hasInitData ? (
                                    <div className="rounded-xl border border-amber-500/30 bg-amber-500/10 p-3 text-xs text-amber-700 dark:text-amber-200">
                                        闂佸搫鐗滄禍锝夌嵁韫囨稑鐭楅柡宓啰鍘?Telegram initData闂佹寧绋戦惌渚€顢氶浣侯浄閹兼番鍨虹花姘舵煕閿濆啫濡烘慨鐟邦樀瀹曟濡烽妷銉х▔闂佺懓鐏氶幐鍝ユ閹寸偑浜滈柣銏犳啞濡椼劑鏌?
                                    </div>
                                ) : null}

                                {poolSearchError ? (
                                    <div className="rounded-xl border border-red-500/30 bg-red-500/10 p-3 text-xs text-red-700 dark:text-red-200">
                                        {poolSearchError}
                                    </div>
                                ) : null}

                                {poolSearchPerformed && !poolSearchLoading && !poolSearchError && poolSearchResults.length === 0 ? (
                                    <div className="rounded-xl border border-zinc-200 bg-white/70 p-3 text-xs text-zinc-500 dark:border-white/10 dark:bg-white/5 dark:text-white/60">
                                        闂佸搫鐗滄禍婵囩珶濮椻偓瀹曟岸骞嶉鐣屻偛闂佺绻楀▍鏇㈡儓濠婂應鍋撳☉娆忓闁?
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
                            aria-label="关闭筛选"
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
                                    aria-label="关闭筛选"
                                >
                                    <Icon path={icons.close} className="h-5 w-5" />
                                </button>
                            </div>

                            <div className="mt-4 space-y-4 pb-20">
                                <div className="rounded-xl border border-zinc-200 bg-zinc-50 p-3 dark:border-white/10 dark:bg-[#0f1116]">
                                    <div className="mt-1">
                                        <div className="text-[11px] text-zinc-500 dark:text-white/40">闂佺懓鍚嬬划搴ㄥ磼?(婵炲瓨鍤庨崐鏍ｅΔ鍐ｅ亾?闂侀潻闄勫妯侯焽?</div>
                                        <input
                                            value={hotPoolsFilterDraft.keyword}
                                            onChange={(e) => setHotPoolsFilterDraft((prev) => ({ ...prev, keyword: e.target.value }))}
                                            className={`mt-1 w-full rounded-xl border border-zinc-200 bg-white/70 px-3 py-2 text-sm text-zinc-900 shadow-sm outline-none ring-0 placeholder:text-zinc-400 ${brand.inputFocusClass} dark:border-white/10 dark:bg-white/5 dark:text-white/90 dark:placeholder:text-white/30`}
                                            placeholder="婵炴挻鑹鹃鍛淬€?USDT"
                                        />
                                    </div>
                                    <div className="mt-3 grid grid-cols-2 gap-3">
                                        <div>
                                            <div className="text-[11px] text-zinc-500 dark:text-white/40">闂佸綊娼ч鍥偨閼姐倖瀚?闂?(USD)</div>
                                            <input
                                                value={hotPoolsFilterDraft.minFees}
                                                onChange={(e) => setHotPoolsFilterDraft((prev) => ({ ...prev, minFees: e.target.value }))}
                                                inputMode="decimal"
                                                className={`mt-1 w-full rounded-xl border border-zinc-200 bg-white/70 px-3 py-2 text-sm text-zinc-900 shadow-sm outline-none ring-0 placeholder:text-zinc-400 ${brand.inputFocusClass} dark:border-white/10 dark:bg-white/5 dark:text-white/90 dark:placeholder:text-white/30`}
                                                placeholder={String(defaultHotPoolsFilter.minFees)}
                                            />
                                        </div>
                                        <div>
                                            <div className="text-[11px] text-zinc-500 dark:text-white/40">闁荤姵鍔曞﹢閬嶅极閵堝鍋?闂?(%)</div>
                                            <input
                                                value={hotPoolsFilterDraft.minFeeRate}
                                                onChange={(e) => setHotPoolsFilterDraft((prev) => ({ ...prev, minFeeRate: e.target.value }))}
                                                inputMode="decimal"
                                                className={`mt-1 w-full rounded-xl border border-zinc-200 bg-white/70 px-3 py-2 text-sm text-zinc-900 shadow-sm outline-none ring-0 placeholder:text-zinc-400 ${brand.inputFocusClass} dark:border-white/10 dark:bg-white/5 dark:text-white/90 dark:placeholder:text-white/30`}
                                                placeholder={String(defaultHotPoolsFilter.minFeeRate)}
                                            />
                                        </div>
                                        <div>
                                            <div className="text-[11px] text-zinc-500 dark:text-white/40">TVL 闂?(USD)</div>
                                            <input
                                                value={hotPoolsFilterDraft.minTvl}
                                                onChange={(e) => setHotPoolsFilterDraft((prev) => ({ ...prev, minTvl: e.target.value }))}
                                                inputMode="decimal"
                                                className={`mt-1 w-full rounded-xl border border-zinc-200 bg-white/70 px-3 py-2 text-sm text-zinc-900 shadow-sm outline-none ring-0 placeholder:text-zinc-400 ${brand.inputFocusClass} dark:border-white/10 dark:bg-white/5 dark:text-white/90 dark:placeholder:text-white/30`}
                                                placeholder={String(defaultHotPoolsFilter.minTvl)}
                                            />
                                        </div>
                                        <div>
                                            <div className="text-[11px] text-zinc-500 dark:text-white/40">婵炲瓨鍤庨崐鏍ｅΔ鍛厒?闂?(USD)</div>
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
                                            aria-label="应用筛选"
                                            title="应用筛选"
                                        >
                                            <Icon path={icons.check} className="h-4 w-4" />
                                            闁圭厧鐡ㄥ濠氬极?
                                        </button>
                                        <button
                                            type="button"
                                            onClick={resetHotPoolsFilter}
                                            className="inline-flex items-center gap-2 rounded-xl bg-white/70 px-3 py-2 text-xs font-semibold text-zinc-700 ring-1 ring-zinc-200 hover:bg-white dark:bg-white/5 dark:text-white/70 dark:ring-white/10"
                                            aria-label="恢复默认筛选"
                                            title="恢复默认筛选"
                                        >
                                            <Icon path={icons.reset} className="h-4 w-4" />
                                            婵帗绋掗…鍫ヮ敇?
                                        </button>
                                        <button
                                            type="button"
                                            onClick={clearHotPoolsFilter}
                                            className="inline-flex items-center gap-2 rounded-xl bg-white/70 px-3 py-2 text-xs font-semibold text-zinc-700 ring-1 ring-zinc-200 hover:bg-white dark:bg-white/5 dark:text-white/70 dark:ring-white/10"
                                            aria-label="清空筛选条件"
                                            title="清空筛选条件"
                                        >
                                            <Icon path={icons.close} className="h-4 w-4" />
                                            濠电偞鎸搁幊鎰板煘閺嶎厼绾ч柍銉ュ级椤?
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
                                    aria-label="关闭全局配置"
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
                                        闂佸憡姊绘慨鎯归崶銊р枖?..
                                    </div>
                                ) : null}
                                {globalConfig ? (
                                    <div className="rounded-xl border border-zinc-200 bg-zinc-50 p-3 dark:border-white/10 dark:bg-[#0f1116]">
                                        <div className="grid grid-cols-2 gap-3 text-xs text-zinc-500 dark:text-white/50">
                                            <div>
                                                <div>Rebalance Timeout</div>
                                                <div className="mt-1 text-sm font-semibold text-zinc-900 dark:text-white/80">
                                                    <NumberFlowValue value={rebalanceText} formatter={() => rebalanceText} />
                                                </div>
                                            </div>
                                            <div>
                                                <div>Slippage</div>
                                                <div className="mt-1 text-sm font-semibold text-zinc-900 dark:text-white/80">
                                                    <NumberFlowValue value={slippageText} formatter={() => slippageText} />
                                                </div>
                                            </div>
                                            <div>
                                                <div>Stop Loss</div>
                                                <div className="mt-1 text-sm font-semibold text-zinc-900 dark:text-white/80">{formatOnOff(globalCfg.stop_loss_enabled)}</div>
                                            </div>
                                            <div>
                                                <div>Stop Loss Delay</div>
                                                <div className="mt-1 text-sm font-semibold text-zinc-900 dark:text-white/80">
                                                    <NumberFlowValue value={stopLossDelayText} formatter={() => stopLossDelayText} />
                                                </div>
                                            </div>
                                            <div>
                                                <div>Auto Reinvest</div>
                                                <div className="mt-1 text-sm font-semibold text-zinc-900 dark:text-white/80">{formatOnOff(globalCfg.auto_reinvest)}</div>
                                            </div>
                                            <div>
                                                <div>Residual Tolerance</div>
                                                <div className="mt-1 text-sm font-semibold text-zinc-900 dark:text-white/80">
                                                    <NumberFlowValue value={residualText} formatter={() => residualText} />
                                                </div>
                                            </div>
                                            <div>
                                                <div>闂佸搫鍟ㄩ崕杈╂崲閺冨牊鐒绘慨妯夸含閸欌偓</div>
                                                <div className="mt-1 text-sm font-semibold text-zinc-900 dark:text-white/80">{formatOnOff(globalCfg.extra_notifications_enabled)}</div>
                                            </div>
                                            <div>
                                                <div>中文代币过滤</div>
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
                                    闂佸憡甯￠弨閬嶅蓟?
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
                                <div className="text-sm font-semibold text-zinc-900 dark:text-white/90">Settings</div>
                                <button
                                    type="button"
                                    onClick={() => setSettingsOpen(false)}
                                    className="inline-flex h-9 w-9 items-center justify-center rounded-xl border border-zinc-200 bg-zinc-100 text-zinc-900 hover:bg-zinc-200 active:bg-zinc-200 dark:border-white/10 dark:bg-white/5 dark:text-white/80 dark:hover:bg-white/10 dark:active:bg-white/15"
                                    aria-label="关闭设置"
                                >
                                    <Icon path={icons.close} className="h-5 w-5" />
                                </button>
                            </div>

                            <div className="mt-4 space-y-4">
                                <div className="rounded-xl border border-zinc-200 bg-zinc-50 p-3 dark:border-white/10 dark:bg-[#0f1116]">
                                    <div className="text-xs font-semibold text-zinc-900 dark:text-white/80">Accent Theme</div>
                                    <div className="mt-0.5 text-[11px] text-zinc-500 dark:text-white/40">Choose the color theme for the miniapp interface.</div>
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
                                    <div className="text-xs font-semibold text-zinc-900 dark:text-white/80">闂佺厧顨庢禍婊勬叏閳哄懎绀嗛梺鍨儐閻撯偓</div>
                                    <div className="mt-0.5 text-[11px] text-zinc-500 dark:text-white/40">
                                        Default interval <NumberFlowValue value={settingsPollIntervalSec} formatOptions={{ maximumFractionDigits: 0 }} />s
                                        {pollOverrideSec
                                            ? 'Override enabled.'
                                            : <>Server default <NumberFlowValue value={settingsServerPollIntervalSec} formatOptions={{ maximumFractionDigits: 0 }} />s</>}
                                        .
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
                                            闁荤姾娅ｉ崰鏍р枔閵忊槅娓舵俊顖涱儥閸?
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
                                            缂佺虎鍙庨崰鏍偩?
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
                                <div className="truncate text-sm font-semibold text-zinc-900 dark:text-white/90">Open Position</div>
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
                                        <div className="text-xs font-semibold text-zinc-900 dark:text-white/80">钱包</div>
                                        <div className="text-[11px] text-zinc-500 dark:text-white/40">
                                            {walletsLoading
                                                ? 'Loading...'
                                                : [
                                                    String(walletsData?.chain || '').toUpperCase(),
                                                    walletsData?.native_symbol && walletsData?.stable_symbol
                                                        ? `${walletsData.native_symbol}/${walletsData.stable_symbol}`
                                                        : '',
                                                ].filter(Boolean).join(' | ')}
                                        </div>
                                    </div>

                                    {walletsError ? (
                                        <div className="mt-2 rounded-xl border border-red-500/30 bg-red-500/10 p-2 text-xs text-red-700 dark:text-red-200">
                                            {walletsError}
                                        </div>
                                    ) : null}

                                    {!walletsLoading && !walletsError && Array.isArray(walletsData?.wallets) && walletsData.wallets.length === 0 ? (
                                        <div className="mt-2 text-xs text-zinc-500 dark:text-white/50">No wallet found.</div>
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
                                                                    {name || shortAddr || `钱包 ${id}`}
                                                                </div>
                                                                {w?.is_default ? (
                                                                    <span className="shrink-0 rounded-full bg-zinc-500/10 px-2 py-0.5 text-[10px] font-semibold text-zinc-600 dark:text-white/60">
                                                                        Default
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
                                <div className="text-xs font-semibold text-zinc-900 dark:text-white/80">Amount (USDT)</div>
                                <input
                                    value={openPositionAmount}
                                    onChange={(e) => {
                                        setOpenPositionAmount(e.target.value);
                                        setOpenPositionError('');
                                    }}
                                    inputMode="decimal"
                                    className={`mt-2 w-full rounded-xl border border-zinc-200 bg-white/70 px-3 py-2 text-sm text-zinc-900 shadow-sm outline-none ring-0 placeholder:text-zinc-400 ${brand.inputFocusClass} dark:border-white/10 dark:bg-white/5 dark:text-white/90 dark:placeholder:text-white/30`}
                                    placeholder="e.g. 100"
                                />
                            </div>

                            <div className="rounded-xl border border-zinc-200 bg-zinc-50 p-3 dark:border-white/10 dark:bg-[#0f1116]">
                                <div className="text-xs font-semibold text-zinc-900 dark:text-white/80">Custom Range (%)</div>
                                <div className="mt-2 grid grid-cols-2 gap-2">
                                    <input
                                        value={openPositionRangeLower}
                                        onChange={(e) => handleOpenPositionRangeLowerChange(e.target.value)}
                                        inputMode="decimal"
                                        className={`w-full rounded-xl border border-zinc-200 bg-white/70 px-3 py-2 text-sm text-zinc-900 shadow-sm outline-none ring-0 placeholder:text-zinc-400 ${brand.inputFocusClass} dark:border-white/10 dark:bg-white/5 dark:text-white/90 dark:placeholder:text-white/30`}
                                        placeholder="Lower %"
                                    />
                                    <input
                                        value={openPositionRangeUpper}
                                        onChange={(e) => handleOpenPositionRangeUpperChange(e.target.value)}
                                        inputMode="decimal"
                                        className={`w-full rounded-xl border border-zinc-200 bg-white/70 px-3 py-2 text-sm text-zinc-900 shadow-sm outline-none ring-0 placeholder:text-zinc-400 ${brand.inputFocusClass} dark:border-white/10 dark:bg-white/5 dark:text-white/90 dark:placeholder:text-white/30`}
                                        placeholder="Upper %"
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
                                                setOpenPositionRangeUpperAuto(true);
                                                setOpenPositionError('');
                                            }}
                                            className="rounded-lg px-2 py-1 text-[11px] font-semibold text-amber-700 ring-1 ring-amber-500/30 bg-gradient-to-r from-amber-50 via-amber-100/60 to-yellow-100/60 hover:from-amber-100 hover:via-amber-200/70 hover:to-yellow-200/70 dark:text-amber-200 dark:ring-amber-400/30 dark:from-amber-500/10 dark:via-amber-400/10 dark:to-yellow-400/10"
                                        >
                                            {option.label}
                                        </button>
                                    ))}
                                </div>
                                <div className="mt-2 text-[11px] text-zinc-500 dark:text-white/40">
                                    Enter lower and upper percentages. For example, 1 / 3 means down 1% and up 3%.
                                </div>
                            </div>

                            <div className="rounded-xl border border-zinc-200 bg-zinc-50 p-3 dark:border-white/10 dark:bg-[#0f1116]">
                                <div className="text-xs font-semibold text-zinc-900 dark:text-white/80">Slippage (%)</div>
                                <div className="mt-0.5 text-[11px] text-zinc-500 dark:text-white/40">Leave empty to use the global slippage setting.</div>
                                <input
                                    value={openPositionSlippage}
                                    onChange={(e) => {
                                        setOpenPositionSlippage(e.target.value);
                                        setOpenPositionError('');
                                    }}
                                    inputMode="decimal"
                                    className={`mt-2 w-full rounded-xl border border-zinc-200 bg-white/70 px-3 py-2 text-sm text-zinc-900 shadow-sm outline-none ring-0 placeholder:text-zinc-400 ${brand.inputFocusClass} dark:border-white/10 dark:bg-white/5 dark:text-white/90 dark:placeholder:text-white/30`}
                                    placeholder="e.g. 0.5 (optional)"
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
                                {openPositionLoading ? '提交中...' : '确认开仓'}
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
                            aria-label="Close update range"
                        />
                        <div className="absolute inset-x-0 bottom-0 rounded-t-2xl border border-zinc-200 bg-white p-4 shadow-2xl dark:border-white/10 dark:bg-[#111318] dark:shadow-none">
                            <div className="flex items-center justify-between gap-2">
                                <div className="min-w-0">
                                    <div className="text-sm font-semibold text-zinc-900 dark:text-white/90">修改任务区间</div>
                                    <div className="mt-0.5 text-[11px] text-zinc-500 dark:text-white/40 truncate">
                                        {taskRangeEdit?.title || '--'}
                                    </div>
                                </div>
                                <button
                                    type="button"
                                    onClick={closeTaskRangeModal}
                                    className="inline-flex h-9 w-9 items-center justify-center rounded-xl border border-zinc-200 bg-zinc-100 text-zinc-900 hover:bg-zinc-200 active:bg-zinc-200 dark:border-white/10 dark:bg-white/5 dark:text-white/80 dark:hover:bg-white/10 dark:active:bg-white/15"
                                    aria-label="Close update range"
                                    disabled={taskRangeLoading}
                                >
                                    <Icon path={icons.close} className="h-5 w-5" />
                                </button>
                            </div>

                            <div className="mt-4 space-y-4">
                                <div className="rounded-xl border border-zinc-200 bg-zinc-50 p-3 dark:border-white/10 dark:bg-[#0f1116]">
                                    <div className="text-xs font-semibold text-zinc-900 dark:text-white/80">闂佸搫鍊瑰妯间焊椤栫偞鈷?(%)</div>
                                    <div className="mt-2 grid grid-cols-2 gap-2">
                                        <input
                                            value={taskRangeLower}
                                            onChange={(e) => handleTaskRangeLowerChange(e.target.value)}
                                            inputMode="decimal"
                                            className={`w-full rounded-xl border border-zinc-200 bg-white/70 px-3 py-2 text-sm text-zinc-900 shadow-sm outline-none ring-0 placeholder:text-zinc-400 ${brand.inputFocusClass} dark:border-white/10 dark:bg-white/5 dark:text-white/90 dark:placeholder:text-white/30`}
                                            placeholder="婵炴垶鎸搁澶婎瀶?%"
                                        />
                                        <input
                                            value={taskRangeUpper}
                                            onChange={(e) => handleTaskRangeUpperChange(e.target.value)}
                                            inputMode="decimal"
                                            className={`w-full rounded-xl border border-zinc-200 bg-white/70 px-3 py-2 text-sm text-zinc-900 shadow-sm outline-none ring-0 placeholder:text-zinc-400 ${brand.inputFocusClass} dark:border-white/10 dark:bg-white/5 dark:text-white/90 dark:placeholder:text-white/30`}
                                            placeholder="婵炴垶鎸搁敃顏勵瀶?%"
                                        />
                                    </div>
                                    <div className="mt-2 text-[11px] text-zinc-500 dark:text-white/40">
                                        婵烇絽娴傞崰妤呭极婵傜瑙﹂幖杈剧稻閻ｉ亶鏌涢弽銊︺仢婵☆偉娉曟禍鎼佸幢濡や浇鍚梺闈涙閸旀帞绮崨顓涙瀻闁炽儱鍟块弲娆愵殽閻愯尙鏋冮柕鍡忓亾闂佸搫鍟崇紙浼村焵椤掍礁鐏╅柡浣规崌瀵偊宕煎☉鎺戜壕?
                                    </div>
                                </div>

                                <div className="rounded-xl border border-zinc-200 bg-zinc-50 p-3 dark:border-white/10 dark:bg-[#0f1116]">
                                    <div className="text-xs font-semibold text-zinc-900 dark:text-white/80">婵炴垶鎸搁鍡涱敃婵傚憡鐓傜€广儱鎳橀幐顒勬偠濞戝磭绡€闁革絿鍋撻敍?(USDT)</div>
                                    <div className="mt-2">
                                        <input
                                            value={taskRangeAmount}
                                            onChange={(e) => {
                                                setTaskRangeAmount(e.target.value);
                                                setTaskRangeError('');
                                            }}
                                            inputMode="decimal"
                                            className={`w-full rounded-xl border border-zinc-200 bg-white/70 px-3 py-2 text-sm text-zinc-900 shadow-sm outline-none ring-0 placeholder:text-zinc-400 ${brand.inputFocusClass} dark:border-white/10 dark:bg-white/5 dark:text-white/90 dark:placeholder:text-white/30`}
                                            placeholder="USDT amount"
                                        />
                                    </div>
                                    <div className="mt-2 text-[11px] text-zinc-500 dark:text-white/40">
                                        閻熸粎澧楅幐鍛婃櫠閻樿绠板ù锝夘棑濞夈垹鈽夐幘宕囆＄紒鍙樺嵆閹嫮鈧稒锚婢跺秹鏌涘▎鎯奉亝鎱ㄩ埡鍛櫖鐎光偓閸曨偅顏ユ俊顐ゅ缁诲倿骞忔导鏉戠婵炲樊浜濋敍鐔兼⒑椤旀寧顥夐柣銊ｅ灲瀹曠兘濡搁敐鍌氫壕闁归偊浜為悷鎾寸箾閸″繆鍋撻崘鍙夋珒濡ょ姷鍋涢悿鍥Υ閳ь剟鏌￠崘顓犲牚闁逞屽墯閸ㄥ綊寮幘璇叉瀬闁割偅绋堥崑?
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
                            aria-label="取消拉黑"
                        />
                        <div className="relative w-full max-w-md overflow-hidden rounded-t-2xl sm:rounded-2xl border border-red-500/20 bg-white p-4 shadow-2xl dark:border-red-500/20 dark:bg-[#111318]">
                            <div className="flex items-start gap-3">
                                <div className="flex h-11 w-11 items-center justify-center rounded-2xl bg-red-500/15 text-red-600 ring-1 ring-red-500/30 dark:text-red-200">
                                    <Icon path={icons.alert} className="h-6 w-6" />
                                </div>
                                <div className="min-w-0">
                                    <div className="text-base font-extrabold text-zinc-900 dark:text-white/90">加入黑名单</div>
                                    <div className="mt-1 text-xs text-zinc-500 dark:text-white/50">
                                        闁诲繐绻愬Λ娆撴儓濠婂應鍋撳☉娆忓濠殿喚鍋ゅ畷妤呭Ψ閳哄啯瀚ч梺鍛婅壘缁夋潙鐣烽悢鐓庤Е閹煎瓨绻勭粣妤呮⒒閸愵亞甯涙い鎺撶洴閹嫮绮欓崹顔肩稑濠殿喖婀辨慨鎾偤濞嗗浚鍤曢柍褜鍓氱粋?
                                    </div>
                                </div>
                                <button
                                    type="button"
                                    onClick={closeBlacklistPrompt}
                                    className="ml-auto inline-flex h-8 w-8 items-center justify-center rounded-lg border border-zinc-200 bg-zinc-100 text-zinc-900 hover:bg-zinc-200 active:bg-zinc-200 dark:border-white/10 dark:bg-white/5 dark:text-white/80 dark:hover:bg-white/10 dark:active:bg-white/15"
                                    aria-label="关闭黑名单确认"
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
                                        待确认
                                    </div>
                                </div>
                            </div>

                            <div className="mt-3 space-y-2 text-xs text-zinc-600 dark:text-white/60">
                                <div className="flex items-start gap-2">
                                    <span className="mt-0.5 inline-flex h-4 w-4 items-center justify-center rounded-full bg-red-500/15 text-red-600 dark:text-red-200">1</span>
                                    <span>加入黑名单后，将阻止该池子的后续开仓。</span>
                                </div>
                                <div className="flex items-start gap-2">
                                    <span className="mt-0.5 inline-flex h-4 w-4 items-center justify-center rounded-full bg-zinc-500/15 text-zinc-600 dark:text-white/60">2</span>
                                    <span>后续可在黑名单列表中移除。</span>
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
                            aria-label="取消确认"
                        />
                        <div className="relative w-full max-w-md overflow-hidden rounded-t-2xl sm:rounded-2xl border border-zinc-200 bg-white p-4 shadow-2xl dark:border-white/10 dark:bg-[#111318]">
                            <div className="flex items-center justify-between gap-2">
                                <div className="text-sm font-semibold text-zinc-900 dark:text-white/90">{confirmState.title}</div>
                                <button
                                    type="button"
                                    onClick={() => closeConfirm(false)}
                                    className="inline-flex h-8 w-8 items-center justify-center rounded-lg border border-zinc-200 bg-zinc-100 text-zinc-900 hover:bg-zinc-200 active:bg-zinc-200 dark:border-white/10 dark:bg-white/5 dark:text-white/80 dark:hover:bg-white/10 dark:active:bg-white/15"
                                    aria-label="关闭确认弹窗"
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
        </div>
    );
}
