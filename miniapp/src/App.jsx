import React, { Suspense, lazy, useEffect, useMemo, useRef, useState, useCallback } from 'react';
import HotPoolCard from './components/HotPoolCard.jsx';
import KlineModal from './components/KlineModal.jsx';
import PositionCard from './components/PositionCard.jsx';
import SystemConfigCard from './components/SystemConfigCard.jsx';
import BottomSheet from './components/BottomSheet.jsx';
import ModuleHeader from './components/ModuleHeader.jsx';
import NumberFlowValue from './components/NumberFlowValue.jsx';
import StepProgressModal from './components/StepProgressModal.jsx';
import { SkeletonHotPoolCard, SkeletonPositionCard, SkeletonList } from './components/Skeleton.jsx';
import SmartMoneyPage from './components/SmartMoneyPage.jsx';
import { Bot, BarChart2, Droplets, Filter, Search, Moon, Sun, Settings, X, Check, RotateCcw, AlertTriangle, CheckCircle, XCircle, Flame, Eye, Wallet } from 'lucide-react';
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
    prepareOpenPosition,
    previewOpenPosition,
    updateTaskRange,
    setTaskPaused,
    stopTask,
    saveGlobalConfig,
    withdrawLiquidity,
    swapDust,
    triggerRebalance,
    toggleRebalance,
    addLiquidity,
} from './lib/api';
import { fetchSMPoolStats } from './lib/smartMoneyApi';
import { getTelegramWebApp, hapticImpact, hapticNotification, hapticSelection } from './lib/telegram';
import { formatRelativeTime, useTick } from './lib/time';
import {
    ACCENT_THEME_OPTIONS,
    getBrandTheme,
    normalizeAccentTheme,
} from './lib/brand';

const LazyAdminPage = lazy(() => import('./components/AdminPage.jsx'));
const LazyAssetManagementPage = lazy(() => import('./components/AssetManagementPage.jsx'));

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
]; const DEFAULT_WEB_WORKBENCH_WIDGETS = WEB_WORKBENCH_WIDGETS.map((item) => item.key);

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

function formatUsdCompact(v) {
    const n = Number(v ?? 0);
    if (!Number.isFinite(n) || n <= 0 || Math.abs(n) > USD_DISPLAY_LIMIT) return '$--';
    const abs = Math.abs(n);
    if (abs >= 1000000) return `$${(n / 1000000).toFixed(abs >= 10000000 ? 0 : 1).replace(/\.0$/, '')}M`;
    if (abs >= 1000) return `$${(n / 1000).toFixed(abs >= 10000 ? 0 : 1).replace(/\.0$/, '')}K`;
    if (abs >= 100) return `$${n.toFixed(0)}`;
    if (abs >= 10) return `$${n.toFixed(1).replace(/\.0$/, '')}`;
    return `$${n.toFixed(2).replace(/0+$/, '').replace(/\.$/, '')}`;
}

function formatRangePercentCompact(value) {
    const num = Number(value);
    if (!Number.isFinite(num) || num <= 0) return '--';
    if (num >= 100) return `${Math.round(num)}%`;
    if (num >= 10) return `${num.toFixed(1).replace(/\.0$/, '')}%`;
    return `${num.toFixed(2).replace(/0+$/, '').replace(/\.$/, '')}%`;
}

function parseAmountInput(value) {
    return Number(String(value || '').replace(/,/g, '').trim());
}

function roundPresetAmount(value) {
    const num = Number(value);
    if (!Number.isFinite(num) || num <= 0) return 0;
    if (num >= 1000) return Math.round(num / 50) * 50;
    if (num >= 200) return Math.round(num / 20) * 20;
    if (num >= 50) return Math.round(num / 10) * 10;
    if (num >= 10) return Math.round(num / 5) * 5;
    return Math.round(num * 10) / 10;
}

function formatAmountInput(value) {
    const num = Number(value);
    if (!Number.isFinite(num) || num <= 0) return '';
    if (num >= 100) return String(Math.round(num));
    return num.toFixed(num >= 10 ? 1 : 2).replace(/0+$/, '').replace(/\.$/, '');
}

function formatRatioCompact(value) {
    const num = Number(value);
    if (!Number.isFinite(num) || num <= 0) return '--';
    if (num >= 100) return `${Math.round(num)}%`;
    if (num >= 10) return `${num.toFixed(1).replace(/\.0$/, '')}%`;
    return `${num.toFixed(2).replace(/0+$/, '').replace(/\.$/, '')}%`;
}

function buildAddLiquidityPresetOptions(referenceAmount) {
    const presets = [];
    const seen = new Set();

    const pushPreset = (value, hint) => {
        const rounded = roundPresetAmount(value);
        if (!(rounded > 0)) return;
        const key = rounded.toFixed(2);
        if (seen.has(key)) return;
        seen.add(key);
        presets.push({
            value: rounded,
            label: `${formatAmountInput(rounded)} USDT`,
            hint,
        });
    };

    if (referenceAmount > 0) {
        pushPreset(referenceAmount * 0.25, '25% 绛栫暐');
        pushPreset(referenceAmount * 0.5, '50% 绛栫暐');
        pushPreset(referenceAmount, '1x 绛栫暐');
    }

    pushPreset(50, '甯哥敤');
    pushPreset(100, '甯哥敤');
    pushPreset(200, '甯哥敤');

    return presets.slice(0, 4);
}

const defaultHotPoolsFilter = {
    enabled: true,
    keyword: '',
    minFees: 60,
    minFeeRate: 0.3,
    minActiveFeeRate: null,
    minTvl: 1000,
    minVolume: 2000,
    minTxCount: null,
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

function computeHotPoolActiveFeeRate(pool) {
    const totalFees = Number(pool?.total_fees ?? 0);
    const activeLiquidityUsd = Number(pool?.activeLiquidityUSD ?? pool?.active_liquidity_usd ?? 0);
    if (!Number.isFinite(totalFees) || !Number.isFinite(activeLiquidityUsd) || activeLiquidityUsd <= 0) {
        return null;
    }
    return (totalFees / activeLiquidityUsd) * 100;
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

function parseOptionalPercent(raw) {
    const text = String(raw || '').trim();
    if (!text) return { valid: true, value: undefined };
    const num = Number(text);
    if (!Number.isFinite(num) || num < 0 || num > 100) {
        return { valid: false, value: undefined };
    }
    return { valid: true, value: num };
}

function formatSharePercent(value) {
    const num = Number(value);
    if (!Number.isFinite(num) || num < 0) return '--';
    const percent = num * 100;
    if (percent >= 100) return `${Math.round(percent)}%`;
    if (percent >= 10) return `${percent.toFixed(1).replace(/\.0$/, '')}%`;
    return `${percent.toFixed(2).replace(/0+$/, '').replace(/\.$/, '')}%`;
}

function formatSizingModeLabel(mode) {
    switch (String(mode || '').trim()) {
        case 'conservative':
            return '保守';
        case 'neutral':
            return '中性';
        case 'aggressive':
            return '激进';
        default:
            return '--';
    }
}

function getSizingEfficiencyMeta(efficiency) {
    switch (String(efficiency || '').trim()) {
        case 'high':
            return {
                label: '高效率',
                chipClass: 'border-emerald-500/30 bg-emerald-500/10 text-emerald-700 dark:text-emerald-200',
            };
        case 'medium':
            return {
                label: '中效率',
                chipClass: 'border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-200',
            };
        default:
            return {
                label: '低效率',
                chipClass: 'border-red-500/30 bg-red-500/10 text-red-700 dark:text-red-200',
            };
    }
}

const POSITION_SM_RANGE_STALE_MS = 60_000;
const POSITION_SM_RANGE_BATCH_SIZE = 8;

function normalizePoolKey(value) {
    const raw = String(value || '').trim();
    if (!raw) return '';
    const body = raw.startsWith('0x') || raw.startsWith('0X') ? raw.slice(2) : raw;
    if (!/^[a-fA-F0-9]{40}$/.test(body) && !/^[a-fA-F0-9]{64}$/.test(body)) {
        return '';
    }
    return `0x${body.toLowerCase()}`;
}

function normalizePositionSmartMoneyGroups(groups) {
    return Array.isArray(groups)
        ? groups.filter((item) => Number(item?.range_percent) > 0)
        : [];
}


function buildEntrySwapConfirmKey(preview, entrySwapSlippage) {
    return [
        preview?.required ? '1' : '0',
        preview?.from_token_address || '',
        preview?.to_token_address || '',
        preview?.amount_in_raw || '',
        preview?.expected_amount_out_raw || '',
        String(entrySwapSlippage || '').trim(),
    ].join('|');
}

function resolveOpenPositionErrorPayload(error) {
    if (!error || typeof error !== 'object') return null;
    if (error.payload && typeof error.payload === 'object') return error.payload;
    return error;
}

function isOpenPositionSafetyError(error) {
    const payload = resolveOpenPositionErrorPayload(error);
    if (!payload) return false;
    const code = String(payload?.code || '').trim();
    return Boolean(
        code === 'zap_safety_check_failed' ||
        code.startsWith('pool_') ||
        typeof payload?.liquidity_usd === 'number' ||
        typeof payload?.max_open_amount === 'number' ||
        typeof payload?.price_deviation_percent === 'number' ||
        Boolean(payload?.risk_ack_required)
    );
}

function extractOpenPositionErrorChecks(error, fallbackKey = 'submit_safety') {
    const payload = resolveOpenPositionErrorPayload(error);
    if (Array.isArray(payload?.checks) && payload.checks.length > 0) {
        return payload.checks;
    }
    if (!isOpenPositionSafetyError(payload)) {
        return [];
    }
    const detail = String(error?.message || payload?.message || '').trim() || '安全检查未通过';
    return [{
        key: fallbackKey,
        status: 'fail',
        label: '安全检查',
        detail,
    }];
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
    eye: Eye,
    wallet: Wallet,
};

const Icon = ({ path: IconCmp, className = '' }) => {
    if (!IconCmp) return null;
    return <IconCmp className={className} strokeWidth={2} />;
};

function buildTopNavItems({ isAdmin }) {
    const items = [
        { key: 'hot_pools', label: '热门池子' },
        { key: 'positions', label: '仓位' },
        { key: 'assets', label: '我的' },
        { key: 'smart_money', label: '聪明钱' },
    ];
    if (isAdmin) {
        items.push({ key: 'admin_page', label: '管理员' });
    }
    return items;
}
const HOT_POOL_SORT_TABS = [
    { key: 'fees', label: '手续费' },
    { key: 'fee_rate', label: '费率' },
    { key: 'volume', label: '交易量' },
];
export default function App() {
    const initData = useInitData();
    const tick = useTick(); // 驱动相对时间与轮询状态展示
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
        minActiveFeeRate: formatDraftNumber(defaultHotPoolsFilter.minActiveFeeRate),
        minTvl: String(defaultHotPoolsFilter.minTvl),
        minVolume: String(defaultHotPoolsFilter.minVolume),
        minTxCount: formatDraftNumber(defaultHotPoolsFilter.minTxCount),
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
    const [openPositionPrepareChecks, setOpenPositionPrepareChecks] = useState([]);
    const [openPositionChecks, setOpenPositionChecks] = useState([]);
    const [openPositionRiskAck, setOpenPositionRiskAck] = useState(false);
    const [openPositionEntrySwapPreview, setOpenPositionEntrySwapPreview] = useState(null);
    const [openPositionEntrySwapPreviewLoading, setOpenPositionEntrySwapPreviewLoading] = useState(false);
    const [openPositionEntrySwapPreviewError, setOpenPositionEntrySwapPreviewError] = useState('');
    const [openPositionPreparePrivateZapInfo, setOpenPositionPreparePrivateZapInfo] = useState(null);
    const [openPositionPrivateZapInfo, setOpenPositionPrivateZapInfo] = useState(null);
    const [openPositionSizingAdvice, setOpenPositionSizingAdvice] = useState(null);
    const [openPositionEntrySwapSlippage, setOpenPositionEntrySwapSlippage] = useState('');
    const [openPositionEntrySwapSlippageDirty, setOpenPositionEntrySwapSlippageDirty] = useState(false);
    const [openPositionEntrySwapConfirm, setOpenPositionEntrySwapConfirm] = useState(false);
    const [openPositionLoading, setOpenPositionLoading] = useState(false);
    const [openPositionSmartRanges, setOpenPositionSmartRanges] = useState([]);
    const [openPositionSmartRangesLoading, setOpenPositionSmartRangesLoading] = useState(false);
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

    const [addLiqModal, setAddLiqModal] = useState(null); // { taskId, title }
    const [addLiqAmount, setAddLiqAmount] = useState('');
    const [addLiqError, setAddLiqError] = useState('');
    const [addLiqLoading, setAddLiqLoading] = useState(false);


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
    const activeOpenPositionChecks = useMemo(() => (
        Array.isArray(openPositionChecks) && openPositionChecks.length > 0
            ? openPositionChecks
            : (Array.isArray(openPositionPrepareChecks) ? openPositionPrepareChecks : [])
    ), [openPositionChecks, openPositionPrepareChecks]);
    const activeOpenPositionPrivateZapInfo = openPositionPrivateZapInfo || openPositionPreparePrivateZapInfo;
    const openPositionFailChecks = activeOpenPositionChecks.filter((item) => item.status === 'fail');
    const openPositionHasBlockingSafetyFailure = openPositionFailChecks.length > 0;
    const openPositionSubmitDisabled = openPositionLoading || openPositionEntrySwapPreviewLoading || openPositionHasBlockingSafetyFailure;
    const openPositionDisplayChecks = useMemo(() => (
        Array.isArray(activeOpenPositionChecks)
            ? activeOpenPositionChecks.filter((item) => String(item?.key || '').trim() !== 'entry_swap')
            : []
    ), [activeOpenPositionChecks]);
    const openPositionShowPrivateZapProtectionHint = Boolean(activeOpenPositionPrivateZapInfo?.show_protection_hint);
    const openPositionRecommendedPositions = Array.isArray(openPositionSizingAdvice?.recommended_positions)
        ? openPositionSizingAdvice.recommended_positions
        : [];
    const openPositionSizingWarnings = Array.isArray(openPositionSizingAdvice?.warnings)
        ? openPositionSizingAdvice.warnings
        : [];
    const openPositionSizingInputs = openPositionSizingAdvice?.inputs && typeof openPositionSizingAdvice.inputs === 'object'
        ? openPositionSizingAdvice.inputs
        : null;
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

    const [pollProgress, setPollProgress] = useState(0);
    const pollProgressRef = useRef(null);
    const lastPollTimeRef = useRef(Date.now());
    const brand = useMemo(() => getBrandTheme(accentTheme), [accentTheme]);

    const [batchMode, setBatchMode] = useState(false);
    const [selectedTaskIds, setSelectedTaskIds] = useState(new Set());
    const [batchLoading, setBatchLoading] = useState(false);
    const [positionSmartMoneyRanges, setPositionSmartMoneyRanges] = useState({});
    const positionSmartMoneyRangesRef = useRef(positionSmartMoneyRanges);

    const serverPollIntervalSec = Math.max(1, Number(data?.poll_interval_sec || adminPositions?.poll_interval_sec || 1));
    const pollIntervalSec = Math.max(1, Number(pollOverrideSec || serverPollIntervalSec || 1));
    const adminListPollSec = Math.max(3, pollIntervalSec);
    const isAdmin = Boolean(me?.is_admin || data?.is_admin || adminPositions?.is_admin);
    const showAdmin = isAdmin && viewMode === 'admin';
    const isHotPools = viewMode === 'hot_pools';
    const isPositions = viewMode === 'positions';
    const isAssets = viewMode === 'assets';
    const isSmartMoney = viewMode === 'smart_money';
    const isAdminPage = isAdmin && viewMode === 'admin_page';
    const topNavItems = useMemo(
        () => buildTopNavItems({ isAdmin }),
        [isAdmin],
    );
    const showWalletSummaryCard = !showAdmin && !isHotPools && !isAssets && !isAdminPage;
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
    const addLiqPosition = useMemo(() => {
        if (!addLiqModal) return null;
        const taskId = Number(addLiqModal?.taskId || addLiqModal?.task_id || 0);
        const matched = positions.find((item) => Number(item?.task_id || 0) === taskId);
        return matched ? { ...matched, ...addLiqModal, taskId } : addLiqModal;
    }, [addLiqModal, positions]);
    const addLiqCurrentValue = Number(
        addLiqPosition?.current_value_usd
        ?? addLiqPosition?.totals?.total_usd
        ?? addLiqPosition?.totals?.position_usd
        ?? 0
    );
    const addLiqReferenceAmount = Number(
        addLiqPosition?.task_amount_usdt
        ?? addLiqPosition?.net_invested_usd
        ?? addLiqPosition?.initial_cost_usd
        ?? 0
    );
    const addLiqParsedAmount = parseAmountInput(addLiqAmount);
    const addLiqPresetOptions = useMemo(
        () => buildAddLiquidityPresetOptions(addLiqReferenceAmount),
        [addLiqReferenceAmount]
    );
    const addLiqHintText = Number.isFinite(addLiqParsedAmount) && addLiqParsedAmount > 0 && addLiqReferenceAmount > 0
        ? `约为原策略金额的 ${formatRatioCompact((addLiqParsedAmount / addLiqReferenceAmount) * 100)}，会按当前池价买入并补进仓位。`
        : '输入要追加的 USDT 金额，系统会按当前池价买入并补进当前仓位。';

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
    const visibleTaskPositionPoolAddresses = useMemo(() => {
        const seen = new Set();
        visibleTaskPositions.forEach((position) => {
            const poolId = normalizePoolKey(position?.pool_id || position?.pool_address);
            if (poolId) seen.add(poolId);
        });
        return Array.from(seen).sort();
    }, [visibleTaskPositions]);
    const visibleTaskPositionPoolKey = useMemo(
        () => visibleTaskPositionPoolAddresses.join(','),
        [visibleTaskPositionPoolAddresses]
    );

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

    const positionsPoolAddresses = useMemo(() => {
        return Array.from(positionsPoolMap.keys());
    }, [positionsPoolMap]);

    const hotPoolsRows = useMemo(() => {
        return Array.isArray(hotPoolsData?.data) ? hotPoolsData.data : [];
    }, [hotPoolsData]);

    const hotPoolsFilterEnabled = useMemo(() => {
        if (!hotPoolsFilter.enabled) return false;
        const hasKeyword = String(hotPoolsFilter.keyword || '').trim().length > 0;
        const hasNumbers = [hotPoolsFilter.minFees, hotPoolsFilter.minFeeRate, hotPoolsFilter.minActiveFeeRate, hotPoolsFilter.minTvl, hotPoolsFilter.minVolume, hotPoolsFilter.minTxCount].some((v) => Number.isFinite(v));
        return hasKeyword || hasNumbers;
    }, [hotPoolsFilter]);

    const hotPoolsVisibleRows = useMemo(() => {
        let filtered = hotPoolsRows;
        if (hotPoolsFilterEnabled) {
            const minFees = hotPoolsFilter.minFees;
            const minFeeRate = hotPoolsFilter.minFeeRate;
            const minActiveFeeRate = hotPoolsFilter.minActiveFeeRate;
            const minTvl = hotPoolsFilter.minTvl;
            const minVolume = hotPoolsFilter.minVolume;
            const minTxCount = hotPoolsFilter.minTxCount;
            const keyword = String(hotPoolsFilter.keyword || '').trim().toLowerCase();
            filtered = hotPoolsRows.filter((row) => {
                const fees = parseMetricNumber(row?.total_fees);
                const feeRate = parseMetricNumber(row?.fee_rate);
                const activeFeeRate = computeHotPoolActiveFeeRate(row);
                const tvl = parseMetricNumber(row?.current_pool_value);
                const volume = parseMetricNumber(row?.total_volume);
                const txCount = parseMetricNumber(row?.transaction_count);
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
                if (Number.isFinite(minActiveFeeRate) && (!Number.isFinite(activeFeeRate) || activeFeeRate < minActiveFeeRate)) return false;
                if (Number.isFinite(minTvl) && tvl < minTvl) return false;
                if (Number.isFinite(minVolume) && volume < minVolume) return false;
                if (Number.isFinite(minTxCount) && txCount < minTxCount) return false;
                return true;
            });
        }

        const enriched = filtered.map(pool => {
            const addr = String(pool?.pool_address || '').toLowerCase();
            return {
                ...pool,
                userPositionUsd: positionsPoolMap.get(addr) || 0
            };
        });

        return enriched.sort((a, b) => {
            if (a.userPositionUsd > 0 && b.userPositionUsd <= 0) return -1;
            if (b.userPositionUsd > 0 && a.userPositionUsd <= 0) return 1;
            if (a.userPositionUsd > 0 && b.userPositionUsd > 0) {
                return b.userPositionUsd - a.userPositionUsd;
            }
            return 0;
        });
    }, [hotPoolsFilter, hotPoolsFilterEnabled, hotPoolsRows, positionsPoolMap]);

    const previousHotPoolsMap = useMemo(() => {
        return previousHotPoolsDataRef.current;
    }, [hotPoolsRows]);

    const apiBaseUrl = useMemo(() => resolveApiBaseUrl(), []);
    const allowEmptyInitData = useMemo(() => resolveAllowEmptyInitData(), []);
    const hasInitData = Boolean(initData) || allowEmptyInitData;

    useEffect(() => {
        positionSmartMoneyRangesRef.current = positionSmartMoneyRanges;
    }, [positionSmartMoneyRanges]);

    useEffect(() => {
        if (showAdmin || !isPositions || visibleTaskPositionPoolAddresses.length === 0) return undefined;

        const now = Date.now();
        const pending = visibleTaskPositionPoolAddresses.filter((poolAddress) => {
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
    }, [apiBaseUrl, isPositions, showAdmin, visibleTaskPositionPoolKey, visibleTaskPositionPoolAddresses]);

    const requestConfirm = (options) => new Promise((resolve) => {
        confirmResolveRef.current = resolve;
        setConfirmState({
            title: options?.title || '纭鎿嶄綔',
            message: options?.message || '',
            confirmText: options?.confirmText || '纭',
            cancelText: options?.cancelText || '鍙栨秷',
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
        if (viewMode !== 'admin') return;
        setViewMode(isAdmin ? 'assets' : 'positions');
    }, [isAdmin, viewMode]);

    useEffect(() => {
        const tg = getTelegramWebApp();
        const savedTheme = storage.get(STORAGE_THEME);
        if (savedTheme === 'light' || savedTheme === 'dark') {
            setTheme(savedTheme);
        } else {
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

    useEffect(() => {
        const currentPollSec = isHotPools ? hotPoolsPollIntervalSec : pollIntervalSec;

        const updateProgress = () => {
            const elapsed = Date.now() - lastPollTimeRef.current;
            const progress = Math.min(100, (elapsed / (currentPollSec * 1000)) * 100);
            setPollProgress(progress);
        };

        updateProgress();

        pollProgressRef.current = setInterval(updateProgress, 100);

        return () => {
            if (pollProgressRef.current) clearInterval(pollProgressRef.current);
        };
    }, [isHotPools, hotPoolsPollIntervalSec, pollIntervalSec]);

    const lastUpdatedAtRef = useRef(null);
    useEffect(() => {
        const currentUpdatedAt = data?.updated_at || hotPoolsData?.updated_at;
        if (currentUpdatedAt && currentUpdatedAt !== lastUpdatedAtRef.current) {
            lastPollTimeRef.current = Date.now();
            setPollProgress(0);
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
            minActiveFeeRate: formatDraftNumber(hotPoolsFilter.minActiveFeeRate),
            minTvl: formatDraftNumber(hotPoolsFilter.minTvl),
            minVolume: formatDraftNumber(hotPoolsFilter.minVolume),
            minTxCount: formatDraftNumber(hotPoolsFilter.minTxCount),
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
            enabled: hotPoolsFilterDraft.enabled,
            keyword,
            minFees: parseDraftNumber(hotPoolsFilterDraft.minFees),
            minFeeRate: parseDraftNumber(hotPoolsFilterDraft.minFeeRate),
            minActiveFeeRate: parseDraftNumber(hotPoolsFilterDraft.minActiveFeeRate),
            minTvl: parseDraftNumber(hotPoolsFilterDraft.minTvl),
            minVolume: parseDraftNumber(hotPoolsFilterDraft.minVolume),
            minTxCount: parseDraftNumber(hotPoolsFilterDraft.minTxCount),
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
            minActiveFeeRate: null,
            minTvl: null,
            minVolume: null,
            minTxCount: null,
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

    const defaultQuickRangeOptions = useMemo(() => ([
        { key: '1', label: '1%', lowerValue: '1', upperValue: '1' },
        { key: '2', label: '2%', lowerValue: '2', upperValue: '2' },
        { key: '3', label: '3%', lowerValue: '3', upperValue: '3' },
        { key: '5', label: '5%', lowerValue: '5', upperValue: '5' },
        { key: '10', label: '10%', lowerValue: '10', upperValue: '10' },
        { key: '20', label: '20%', lowerValue: '20', upperValue: '20' },
    ]), []);
    const smartQuickRangeOptions = useMemo(() => (
        Array.isArray(openPositionSmartRanges)
            ? openPositionSmartRanges
                .filter((item) => Number(item?.range_percent) > 0)
                .slice(0, 6)
                .map((item, index) => {
                    const rangePercent = Number(item?.range_percent);
                    const positionCount = Math.max(0, Number(item?.position_count) || 0);
                    return {
                        key: `smart-${rangePercent}-${positionCount}-${index}`,
                        label: `${formatRangePercentCompact(rangePercent)}${positionCount > 1 ? ` +${positionCount - 1}` : ''}`,
                        subLabel: formatUsdCompact(item?.total_amount_usd),
                        lowerValue: String(rangePercent),
                        upperValue: String(rangePercent),
                        smart: true,
                    };
                })
            : []
    ), [openPositionSmartRanges]);
    const primaryQuickRangeOptions = smartQuickRangeOptions.length > 0 ? smartQuickRangeOptions : defaultQuickRangeOptions;

    const parseRangeInput = (lowerRaw, upperRaw) => {
        const lower = Number(String(lowerRaw || '').trim());
        const upper = Number(String(upperRaw || '').trim());
        if (!Number.isFinite(lower) || !Number.isFinite(upper)) return null;
        return { lower: Math.abs(lower), upper: Math.abs(upper) };
    };

    const openPositionEntrySwapConfirmKey = useMemo(
        () => buildEntrySwapConfirmKey(openPositionEntrySwapPreview, openPositionEntrySwapSlippage),
        [openPositionEntrySwapPreview, openPositionEntrySwapSlippage],
    );

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
        setOpenPositionPrepareChecks([]);
        setOpenPositionEntrySwapPreview(null);
        setOpenPositionEntrySwapPreviewLoading(false);
        setOpenPositionEntrySwapPreviewError('');
        setOpenPositionPreparePrivateZapInfo(null);
        setOpenPositionPrivateZapInfo(null);
        setOpenPositionSizingAdvice(null);
        setOpenPositionEntrySwapSlippage('');
        setOpenPositionEntrySwapSlippageDirty(false);
        setOpenPositionEntrySwapConfirm(false);

        setOpenPositionError('');
        setOpenPositionChecks([]);
        setOpenPositionRiskAck(false);
    };

    const openPositionModal = (pool) => {
        let chain = String(pool?.chain || hotPoolsData?.chain || 'bsc').trim().toLowerCase() || 'bsc';
        if (!multiChainEnabled) chain = userDefaultChain;
        const poolAddress = String(pool?.pool_address || '').trim();
        const poolVersion = String(pool?.protocol_version || pool?.pool_version || '').trim().toLowerCase();
        setOpenPositionPool({
            ...pool,
            chain,
            ...(poolVersion ? { protocol_version: poolVersion, pool_version: poolVersion } : {}),
        });
        setOpenPositionSmartRanges(Array.isArray(pool?.range_groups) ? pool.range_groups : []);
        setOpenPositionSmartRangesLoading(Boolean(poolAddress));
        resetOpenPositionDraft();
    };

    const closeOpenPosition = () => {
        if (openPositionLoading) return;
        setOpenPositionPool(null);
        setOpenPositionSmartRanges([]);
        setOpenPositionSmartRangesLoading(false);
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
        if (!openPositionPool) return;

        let aborted = false;
        const controller = new AbortController();
        const poolAddress = String(openPositionPool?.pool_address || '').trim();
        if (!poolAddress) {
            setOpenPositionSmartRanges([]);
            setOpenPositionSmartRangesLoading(false);
            return undefined;
        }

        setOpenPositionSmartRangesLoading(true);
        fetchSMPoolStats({ apiBaseUrl, poolAddress, signal: controller.signal })
            .then((resp) => {
                if (aborted) return;
                const nextGroups = Array.isArray(resp?.range_groups) ? resp.range_groups : [];
                setOpenPositionSmartRanges((prev) => (nextGroups.length > 0 ? nextGroups : prev));
            })
            .catch(() => {
                if (aborted) return;
            })
            .finally(() => {
                if (aborted) return;
                setOpenPositionSmartRangesLoading(false);
            });

        return () => {
            aborted = true;
            controller.abort();
        };
    }, [apiBaseUrl, openPositionPool]);

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

    useEffect(() => {
        if (!openPositionPool || !hasInitData) {
            setOpenPositionPrepareChecks([]);
            setOpenPositionPreparePrivateZapInfo(null);
            return undefined;
        }

        let walletId;
        if (multiWalletEnabled && !walletsLoading && !walletsError) {
            const list = Array.isArray(walletsData?.wallets) ? walletsData.wallets : [];
            if (list.length === 1) {
                const onlyId = Number(list[0]?.id);
                if (Number.isFinite(onlyId) && onlyId > 0) {
                    walletId = onlyId;
                }
            } else if (list.length > 1) {
                const wid = Number(openPositionWalletId);
                if (Number.isFinite(wid) && wid > 0) {
                    walletId = wid;
                }
            }
        }

        let active = true;
        const controller = new AbortController();
        prepareOpenPosition({
            apiBaseUrl,
            initData,
            chain: openPositionPool?.chain || 'bsc',
            poolAddress: openPositionPool?.pool_address,
            poolVersion: openPositionPool?.protocol_version,
            walletId,
            signal: controller.signal,
        })
            .then((resp) => {
                if (!active) return;
                setOpenPositionPrepareChecks(Array.isArray(resp?.checks) ? resp.checks : []);
                setOpenPositionPreparePrivateZapInfo(resp?.private_zap && typeof resp.private_zap === 'object' ? resp.private_zap : null);
            })
            .catch(() => {
                if (!active || controller.signal.aborted) return;
                setOpenPositionPrepareChecks([]);
                setOpenPositionPreparePrivateZapInfo(null);
            });

        return () => {
            active = false;
            controller.abort();
        };
    }, [
        apiBaseUrl,
        initData,
        hasInitData,
        openPositionPool,
        multiWalletEnabled,
        walletsLoading,
        walletsError,
        walletsData,
        openPositionWalletId,
    ]);

    useEffect(() => {
        if (!openPositionEntrySwapPreview?.required || openPositionEntrySwapSlippageDirty) return;
        const recommended = Number(openPositionEntrySwapPreview?.recommended_slippage_tolerance);
        const current = Number(openPositionEntrySwapPreview?.current_slippage_tolerance);
        const next = Number.isFinite(recommended) ? recommended : current;
        if (!Number.isFinite(next)) return;
        setOpenPositionEntrySwapSlippage(String(next));
    }, [openPositionEntrySwapPreview, openPositionEntrySwapSlippageDirty]);

    useEffect(() => {
        setOpenPositionEntrySwapConfirm(false);
    }, [openPositionEntrySwapConfirmKey]);

    useEffect(() => {
        if (!openPositionPool || !hasInitData) {
            setOpenPositionEntrySwapPreview(null);
            setOpenPositionEntrySwapPreviewLoading(false);
            setOpenPositionEntrySwapPreviewError('');
            setOpenPositionPrivateZapInfo(null);
            setOpenPositionSizingAdvice(null);
            return undefined;
        }

        const poolAddr = String(openPositionPool?.pool_address || '').trim().toLowerCase();
        void poolAddr;

        const amount = Number(String(openPositionAmount || '').trim());
        const range = parseRangeInput(openPositionRangeLower, openPositionRangeUpper);
        const taskSlippage = parseOptionalPercent(openPositionSlippage);
        const entrySwapSlippage = parseOptionalPercent(openPositionEntrySwapSlippage);
        if (!Number.isFinite(amount) || amount <= 0 || !range || range.lower <= 0 || range.upper <= 0 || range.lower >= 100 || range.upper >= 100 || !taskSlippage.valid || !entrySwapSlippage.valid) {
            setOpenPositionEntrySwapPreview(null);
            setOpenPositionEntrySwapPreviewLoading(false);
            setOpenPositionEntrySwapPreviewError('');
            setOpenPositionPrivateZapInfo(null);
            setOpenPositionSizingAdvice(null);
            setOpenPositionChecks([]);
            return undefined;
        }

        let walletId = openPositionWalletId;
        if (multiWalletEnabled) {
            if (walletsLoading) {
                setOpenPositionEntrySwapPreview(null);
                setOpenPositionEntrySwapPreviewLoading(false);
                setOpenPositionEntrySwapPreviewError('');
                setOpenPositionPrivateZapInfo(null);
                setOpenPositionSizingAdvice(null);
                return undefined;
            }
            if (walletsError) {
                setOpenPositionEntrySwapPreview(null);
                setOpenPositionEntrySwapPreviewLoading(false);
                setOpenPositionEntrySwapPreviewError('');
                setOpenPositionPrivateZapInfo(null);
                setOpenPositionSizingAdvice(null);
                return undefined;
            }
            const list = Array.isArray(walletsData?.wallets) ? walletsData.wallets : [];
            if (list.length === 0) {
                setOpenPositionEntrySwapPreview(null);
                setOpenPositionEntrySwapPreviewLoading(false);
                setOpenPositionEntrySwapPreviewError('');
                setOpenPositionPrivateZapInfo(null);
                setOpenPositionSizingAdvice(null);
                return undefined;
            }
            if (list.length > 1) {
                const wid = Number(openPositionWalletId);
                walletId = wid;
                if (!Number.isFinite(wid) || wid <= 0) {
                    setOpenPositionEntrySwapPreview(null);
                    setOpenPositionEntrySwapPreviewLoading(false);
                    setOpenPositionEntrySwapPreviewError('');
                    setOpenPositionPrivateZapInfo(null);
                    setOpenPositionSizingAdvice(null);
                    return undefined;
                }
            } else {
                const onlyId = Number(list[0]?.id);
                if (Number.isFinite(onlyId) && onlyId > 0) {
                    walletId = onlyId;
                }
            }
        }

        let active = true;
        const controller = new AbortController();
        setOpenPositionEntrySwapPreviewLoading(true);
        setOpenPositionEntrySwapPreviewError('');
        setOpenPositionPrivateZapInfo(null);
        setOpenPositionSizingAdvice(null);

        const timer = setTimeout(async () => {
            try {
                const resp = await previewOpenPosition({
                    apiBaseUrl,
                    initData,
                    chain: openPositionPool?.chain || 'bsc',
                    poolAddress: openPositionPool?.pool_address,
                    poolVersion: openPositionPool?.protocol_version,
                    amount,
                    rangeLowerPct: range.lower,
                    rangeUpperPct: range.upper,
                    slippageTolerance: taskSlippage.value,
                    entrySwapSlippageTolerance: entrySwapSlippage.value,
                    allowEntrySwap: true,
                    walletId,
                    ackLiquidityRisk: openPositionRiskAck,
                    signal: controller.signal,
                });
                if (!active) return;
                setOpenPositionChecks(Array.isArray(resp?.checks) ? resp.checks : []);
                setOpenPositionEntrySwapPreview(resp?.entry_swap || { required: false });
                setOpenPositionPrivateZapInfo(resp?.private_zap && typeof resp.private_zap === 'object' ? resp.private_zap : null);
                setOpenPositionSizingAdvice(resp?.sizing_advice && typeof resp.sizing_advice === 'object' ? resp.sizing_advice : null);
            } catch (e) {
                if (!active || controller.signal.aborted) return;
                const msg = String(e?.message || e || '').trim();
                const payload = resolveOpenPositionErrorPayload(e);
                const failChecks = extractOpenPositionErrorChecks(e, 'preview_safety');
                const entrySwapInfo = payload?.entry_swap && typeof payload.entry_swap === 'object'
                    ? payload.entry_swap
                    : null;
                setOpenPositionEntrySwapPreview(entrySwapInfo);
                setOpenPositionPrivateZapInfo(payload?.private_zap && typeof payload.private_zap === 'object' ? payload.private_zap : null);
                setOpenPositionSizingAdvice(payload?.sizing_advice && typeof payload.sizing_advice === 'object' ? payload.sizing_advice : null);
                setOpenPositionChecks(failChecks);
                setOpenPositionEntrySwapPreviewError(failChecks.length > 0 ? '' : (msg || '获取前置兑换预览失败'));
            } finally {
                if (active) {
                    setOpenPositionEntrySwapPreviewLoading(false);
                }
            }
        }, 350);

        return () => {
            active = false;
            clearTimeout(timer);
            controller.abort();
        };
    }, [
        apiBaseUrl,
        initData,
        hasInitData,
        openPositionPool,
        openPositionAmount,
        openPositionRangeLower,
        openPositionRangeUpper,
        openPositionSlippage,
        openPositionEntrySwapSlippage,
        openPositionRiskAck,
        multiWalletEnabled,
        walletsLoading,
        walletsError,
        walletsData,
        openPositionWalletId,
    ]);

    const handleOpenPosition = async () => {
        if (!openPositionPool) return;
        if (!hasInitData) {
            setOpenPositionError('缺少 Telegram 身份信息，请从机器人重新打开小程序。');
            return;
        }
        const amount = Number(String(openPositionAmount || '').trim());
        if (!Number.isFinite(amount) || amount <= 0) {
            setOpenPositionError('请输入有效的开仓金额。');
            return;
        }
        const warnChecks = activeOpenPositionChecks.filter(c => c.status === 'warn');
        const failChecks = activeOpenPositionChecks.filter(c => c.status === 'fail');
        if (failChecks.length > 0) {
            setOpenPositionError(failChecks.map(c => c.detail || c.label).join('; '));
            return;
        }
        const requiresAck = warnChecks.some(c => c.extra?.risk_ack_required);
        const maxOpenAmount = warnChecks.reduce((m, c) => {
            const v = Number(c.extra?.max_open_amount);
            return (Number.isFinite(v) && v > 0 && (m === null || v < m)) ? v : m;
        }, null);
        if (maxOpenAmount !== null && amount > maxOpenAmount) {
            setOpenPositionError(`当前池子单次开仓金额不能高于 ${maxOpenAmount} USDT`);
            return;
        }
        const range = parseRangeInput(openPositionRangeLower, openPositionRangeUpper);
        if (!range || range.lower <= 0 || range.upper <= 0 || range.lower >= 100 || range.upper >= 100) {
            setOpenPositionError('区间必须在 0 到 100 之间。');
            return;
        }

        const slippageRaw = String(openPositionSlippage || '').trim();
        let slippage = undefined;
        if (slippageRaw) {
            const v = Number(slippageRaw);
            if (!Number.isFinite(v) || v < 0 || v > 100) {
                setOpenPositionError('滑点必须在 0 到 100 之间。');
                return;
            }
            slippage = v;
        }

        const slippageParsed = parseOptionalPercent(openPositionSlippage);
        if (!slippageParsed.valid) {
            setOpenPositionError('任务滑点必须在 0 到 100 之间。');
            return;
        }
        const entrySwapSlippageParsed = parseOptionalPercent(openPositionEntrySwapSlippage);
        if (!entrySwapSlippageParsed.valid) {
            setOpenPositionError('前置兑换滑点必须在 0 到 100 之间。');
            return;
        }
        let walletId = openPositionWalletId;

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
                setOpenPositionError('未找到可用钱包。');
                return;
            }
            if (list.length > 1) {
                const wid = Number(openPositionWalletId);
                walletId = wid;
                if (!Number.isFinite(wid) || wid <= 0) {
                    setOpenPositionError('请选择钱包。');
                    return;
                }
            } else {
                const onlyId = String(list[0]?.id || '').trim();
                walletId = onlyId;
                if (onlyId && String(openPositionWalletId || '') !== onlyId) {
                    setOpenPositionWalletId(onlyId);
                    storage.set(STORAGE_OPEN_POSITION_WALLET_ID, onlyId);
                }
            }
        }

        if (openPositionEntrySwapPreviewLoading) {
            setOpenPositionError('前置兑换预览仍在加载，请稍后再试。');
            return;
        }
        if (openPositionEntrySwapPreviewError) {
            setOpenPositionError(openPositionEntrySwapPreviewError);
            return;
        }
        if (openPositionEntrySwapPreview?.required && !openPositionEntrySwapConfirm) {
            setOpenPositionError('请先确认前置兑换，再继续开仓。');
            return;
        }

        setOpenPositionLoading(true);
        setOpenPositionError('');
        setOperationProgress({ operation: 'open_position', currentStep: 1, totalSteps: 4, status: 'active', error: '' });
        try {
            await openPosition({
                apiBaseUrl,
                initData,
                chain: openPositionPool?.chain || 'bsc',
                poolAddress: openPositionPool?.pool_address,
                poolVersion: openPositionPool?.protocol_version,
                amount,
                rangeLowerPct: range.lower,
                rangeUpperPct: range.upper,
                slippageTolerance: slippageParsed.value,
                entrySwapSlippageTolerance: openPositionEntrySwapPreview?.required ? entrySwapSlippageParsed.value : undefined,
                allowEntrySwap: true,
                confirmEntrySwap: Boolean(openPositionEntrySwapPreview?.required && openPositionEntrySwapConfirm),
                walletId,
                ackLiquidityRisk: requiresAck && openPositionRiskAck,
            });
            setOpenPositionError('');
            setOpenPositionChecks([]);
            setOpenPositionEntrySwapPreview(null);
            setOpenPositionEntrySwapPreviewError('');
            setOpenPositionEntrySwapConfirm(false);
            setOperationProgress(prev => prev?.operation === 'open_position'
                ? { ...prev, currentStep: 4, status: 'done' } : prev);
            setOpenPositionPool(null);
            resetOpenPositionDraft();
        } catch (e) {
            const msg = String(e?.message || e || '').trim();
            const payload = resolveOpenPositionErrorPayload(e);
            const entrySwapInfo = payload?.entry_swap && typeof payload.entry_swap === 'object'
                ? payload.entry_swap
                : null;
            const failChecks = extractOpenPositionErrorChecks(e, 'submit_safety');
            if (entrySwapInfo) {
                setOpenPositionEntrySwapPreview(entrySwapInfo);
                setOpenPositionEntrySwapPreviewError('');
            }
            if (failChecks.length > 0) {
                setOpenPositionChecks((prev) => {
                    const merged = Array.isArray(prev)
                        ? prev.filter((item) => !failChecks.some((next) => next?.key === item?.key))
                        : [];
                    return [...merged, ...failChecks];
                });
            }
            setOpenPositionError(msg || '开仓失败。');
            setOperationProgress(prev => prev?.operation === 'open_position'
                ? { ...prev, status: 'error', error: msg || '开仓失败。' } : prev);
        } finally {
            setOpenPositionLoading(false);
        }
    };

    const loadGlobalConfig = async () => {
        if (!hasInitData) {
            setGlobalConfigError('缺少 Telegram 身份信息，请从机器人重新打开小程序。');
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
                // Already stopped or completed immediately.
                setOperationProgress(prev => prev?.operation === 'close_position'
                    ? { ...prev, currentStep: 3, status: 'done' } : prev);
            } else {
                // Async path: advance to step 1 only if WS has not already moved forward.
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

    const handleWithdrawLiquidity = async (taskId) => {
        if (!hasInitData || showAdmin) return;
        const id = Number(taskId);
        if (!Number.isFinite(id) || id <= 0) return;
        const ok = await requestConfirm({
            title: '取回流动性',
            message: '确认要取回流动性并兑换为 USDT？\n该操作会撤出仓位并停止任务。',
            confirmText: '纭鍙栧洖',
            tone: 'danger',
        });
        if (!ok) return;
        try {
            const resp = await withdrawLiquidity({ apiBaseUrl, initData, taskId: id });
            showNotice(resp?.message || '娴佸姩鎬у凡鍙栧洖', 'success');
        } catch (e) {
            showNotice(String(e?.message || e), 'error');
        }
    };

    const handleSwapDust = async (taskId) => {
        if (!hasInitData || showAdmin) return;
        const id = Number(taskId);
        if (!Number.isFinite(id) || id <= 0) return;
        try {
            const resp = await swapDust({ apiBaseUrl, initData, taskId: id });
            showNotice(resp?.message || '残余已兑换', 'success');
        } catch (e) {
            showNotice(String(e?.message || e), 'error');
        }
    };

    const handleTriggerRebalance = async (taskId) => {
        if (!hasInitData || showAdmin) return;
        const id = Number(taskId);
        if (!Number.isFinite(id) || id <= 0) return;
        try {
            const resp = await triggerRebalance({ apiBaseUrl, initData, taskId: id });
            showNotice(resp?.message || '鍐嶅钩琛″凡瑙﹀彂', 'success');
        } catch (e) {
            showNotice(String(e?.message || e), 'error');
        }
    };

    const handleToggleRebalance = async (taskId, enabled) => {
        if (!hasInitData || showAdmin) return;
        const id = Number(taskId);
        if (!Number.isFinite(id) || id <= 0) return;
        try {
            const resp = await toggleRebalance({ apiBaseUrl, initData, taskId: id, rebalanceEnabled: enabled });
            showNotice(enabled ? '再平衡已开启' : '再平衡已关闭（超区间将直接停止）', 'success');
        } catch (e) {
            showNotice(String(e?.message || e), 'error');
        }
    };

    const handleAddLiquidity = (taskId, position) => {
        if (!hasInitData || showAdmin) return;
        const id = Number(taskId);
        if (!Number.isFinite(id) || id <= 0) return;
        setAddLiqModal({
            taskId: id,
            title: String(position?.title || '').trim() || `浠诲姟 #${id}`,
        });
        setAddLiqAmount('');
        setAddLiqError('');
    };

    const closeAddLiqModal = () => {
        if (addLiqLoading) return;
        setAddLiqModal(null);
        setAddLiqAmount('');
        setAddLiqError('');
    };

    const submitAddLiquidity = async () => {
        if (!addLiqModal) return;
        const amount = parseAmountInput(addLiqAmount);
        if (!Number.isFinite(amount) || amount <= 0) {
            setAddLiqError('璇疯緭鍏ユ湁鏁堢殑閲戦');
            return;
        }
        setAddLiqLoading(true);
        setAddLiqError('');
        try {
            const resp = await addLiquidity({ apiBaseUrl, initData, taskId: addLiqModal.taskId, amountUsdt: amount });
            setAddLiqModal(null);
            setAddLiqAmount('');
            showNotice(resp?.message || '补充流动性成功', 'success');
        } catch (e) {
            setAddLiqError(String(e?.message || e));
        } finally {
            setAddLiqLoading(false);
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
            title: String(position?.title || '').trim() || `浠诲姟 #${id}`,
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
            setTaskRangeError('区间必须在 0 到 100 之间。');
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
        assets: {
            title: '我的',
            icon: icons.wallet,
            subtitle: '我的资产 / 全局配置 / 钱包 / 交易历史',
        },
        smart_money: {
            title: '聪明钱',
            icon: icons.eye,
            subtitle: '聪明钱监控 / 聪明钱资产',
        },
        admin_page: {
            title: '管理员',
            icon: icons.gear,
            subtitle: '运行管理 / 系统',
        },
        admin: {
            title: '管理',
            icon: icons.gear,
            subtitle: adminSelectedUser
                ? `用户：${formatUserLabel(adminSelectedUser)}`
                : adminUsersLoading && adminUsers.length === 0
                    ? '加载用户中...'
                    : adminUsers.length
                        ? `Auto 用户 ${adminUsers.length} 个`
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
            ? '加载用户仓位中...'
            : '该用户暂无仓位数据'
        : '请先选择一个管理员用户';
    const showEmptyPositions = isPositions && Boolean(activeData) && visiblePositions.length === 0;
    const hotPoolsPairMap = useMemo(() => {
        const m = new Map();
        for (const row of hotPoolsRows) {
            const addr = String(row?.pool_address || '').trim().toLowerCase();
            const pair = String(row?.trading_pair || '').trim();
            if (addr && pair && !m.has(addr)) m.set(addr, pair);
        }
        return m;
    }, [hotPoolsRows]);

    const initDataMissing = viewMode !== 'hot_pools' && !hasInitData;
    const noticeClass = notice?.tone === 'error'
        ? 'bg-red-600 text-white'
        : notice?.tone === 'success'
            ? brand.successNoticeClass
            : 'bg-zinc-900 text-white dark:bg-white/10 dark:text-white';
    const globalCfg = globalConfig || {};
    const rebalanceText = Number.isFinite(Number(globalCfg.rebalance_timeout))
        ? (Number(globalCfg.rebalance_timeout) <= 0 ? '立即' : `${Number(globalCfg.rebalance_timeout)} s`)
        : '--';
    const stopLossDelayText = Number.isFinite(Number(globalCfg.stop_loss_delay_seconds))
        ? `${Number(globalCfg.stop_loss_delay_seconds)} s`
        : '--';
    const slippageText = Number.isFinite(Number(globalCfg.slippage_tolerance))
        ? `${Number(globalCfg.slippage_tolerance).toFixed(2)}%`
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
        <div className={`min-h-screen max-w-[720px] mx-auto px-4 pt-[max(1rem,env(safe-area-inset-top))] ${isPositions ? 'pb-[calc(96px+env(safe-area-inset-bottom))]' : 'pb-[calc(80px+env(safe-area-inset-bottom))]'}`}>
            {notice ? (
                <div className="fixed left-1/2 top-[calc(env(safe-area-inset-top)+64px)] z-50 w-[calc(100%-2rem)] max-w-md -translate-x-1/2">
                    <div className={`rounded-xl px-3 py-2 text-sm font-semibold shadow-lg ${noticeClass}`}>
                        {notice.message}
                    </div>
                </div>
            ) : null}
            {/* 濠碉紕鍋戦崐鏇㈡偉婵傜纾块柟缁㈠枛缁€澶愭煟濡厧鍔嬬紒浣峰嵆瀵爼鍩￠崒婧炬闁诲氦顫夋繛濠傤嚕?*/}
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
                ) : isAssets ? (
                    <div className="mb-2">
                        <Suspense fallback={<div className="rounded-2xl border border-zinc-200/80 bg-white px-4 py-5 text-sm text-zinc-500 dark:border-white/5 dark:bg-[#131518] dark:text-white/45">正在加载我的模块...</div>}>
                            <LazyAssetManagementPage
                                apiBaseUrl={apiBaseUrl}
                                initData={initData}
                                hasInitData={hasInitData}
                                isAdmin={isAdmin}
                                tick={tick}
                                pollIntervalSec={pollIntervalSec}
                                accentTheme={accentTheme}
                                onNotice={showNotice}
                            />
                        </Suspense>
                    </div>
                ) : isSmartMoney ? (
                    <div className="mb-2">
                        <SmartMoneyPage
                            apiBaseUrl={apiBaseUrl}
                            initData={initData}
                            hasInitData={hasInitData}
                            isAdmin={isAdmin}
                            accentTheme={accentTheme}
                            tick={tick}
                            pollIntervalSec={pollIntervalSec}
                            onOpenPosition={openPositionModal}
                            onNotice={showNotice}
                        />
                    </div>
                ) : isAdminPage ? (
                    <div className="mb-2">
                        <Suspense fallback={<div className="rounded-2xl border border-zinc-200/80 bg-white px-4 py-5 text-sm text-zinc-500 dark:border-white/5 dark:bg-[#131518] dark:text-white/45">正在加载管理员模块...</div>}>
                            <LazyAdminPage
                                apiBaseUrl={apiBaseUrl}
                                initData={initData}
                                hasInitData={hasInitData}
                                tick={tick}
                                pollIntervalSec={pollIntervalSec}
                                accentTheme={accentTheme}
                                onNotice={showNotice}
                            />
                        </Suspense>
                    </div>
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
                                            {totalWalletCount} 个钱包                                        </span>
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
                        暂无热门池子数据。                    </div>
                ) : null
            }

            {
                isHotPools && !hotPoolsLoading && !hotPoolsError && hotPoolsData && hotPoolsRows.length > 0 && hotPoolsFilterEnabled && hotPoolsVisibleRows.length === 0 ? (
                    <div className="mb-4 rounded-2xl border border-zinc-200 bg-white/70 p-6 text-sm text-zinc-500 dark:border-white/10 dark:bg-white/5 dark:text-white/60">
                        筛选后暂无热门池子数据。                    </div>
                ) : null
            }

            {
                !isHotPools && showAdmin ? (
                    <Suspense fallback={<div className="mb-4 rounded-2xl border border-zinc-200/80 bg-white px-4 py-5 text-sm text-zinc-500 dark:border-white/5 dark:bg-[#131518] dark:text-white/45">正在加载管理模块...</div>}>
                        <LazyAdminPage
                            apiBaseUrl={apiBaseUrl}
                            initData={initData}
                            hasInitData={hasInitData}
                            tick={tick}
                            pollIntervalSec={pollIntervalSec}
                            accentTheme={accentTheme}
                            onNotice={showNotice}
                        />
                    </Suspense>
                ) : null
            }

            {
                !isHotPools && initDataMissing ? (
                    <div className="mb-4 rounded-2xl border border-amber-500/30 bg-amber-500/10 p-4 text-sm text-amber-700 dark:text-amber-200">
                        未获取到 Telegram initData，请从机器人入口打开页面。                    </div>
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
                                    恢复所选
                                </button>
                            </div>
                        )}
                    </div>
                )
            }

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
                                apiBaseUrl={apiBaseUrl}
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
                                        onWithdrawLiquidity={handleWithdrawLiquidity}
                                        onSwapDust={handleSwapDust}
                                        onTriggerRebalance={handleTriggerRebalance}
                                        onToggleRebalance={handleToggleRebalance}
                                        onAddLiquidity={handleAddLiquidity}
                                        batchMode={batchMode}
                                        isSelected={selectedTaskIds.has(p.task_id)}
                                        onToggleSelect={() => toggleTaskSelection(p.task_id)}
                                        smartMoneyRangeGroups={
                                            positionSmartMoneyRanges[normalizePoolKey(p?.pool_id || p?.pool_address)]?.groups || []
                                        }
                                    />
                                ))}
                            </>
                        )
                        : null}
            </div>

            {
                isPositions && activeData?.warnings?.length ? (
                    <div className="mt-4 rounded-2xl border border-amber-500/30 bg-amber-500/10 p-4 text-xs text-amber-700 dark:text-amber-200">
                        <div className="font-semibold">鎻愮ず</div>
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
                            aria-label="鍏抽棴鎼滅储"
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
                                    aria-label="鍏抽棴鎼滅储"
                                >
                                    <Icon path={icons.close} className="h-5 w-5" />
                                </button>
                            </div>

                            <div className="mt-4 space-y-3 pb-20">
                                <div className="rounded-xl border border-zinc-200 bg-zinc-50 p-3 dark:border-white/10 dark:bg-[#0f1116]">
                                    <div className="text-[11px] text-zinc-500 dark:text-white/40">鎼滅储姹犲瓙 (姹犲瓙ID/浠ｅ竵鍚嶇О)</div>
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
                                        支持按池子 ID 和代币名称搜索，结果按 TVL 倒序，最多 10 条。</div>
                                </div>

                                {!hasInitData ? (
                                    <div className="rounded-xl border border-amber-500/30 bg-amber-500/10 p-3 text-xs text-amber-700 dark:text-amber-200">
                                        未获取到 Telegram initData，请从机器人入口打开页面。                                    </div>
                                ) : null}

                                {poolSearchError ? (
                                    <div className="rounded-xl border border-red-500/30 bg-red-500/10 p-3 text-xs text-red-700 dark:text-red-200">
                                        {poolSearchError}
                                    </div>
                                ) : null}

                                {poolSearchPerformed && !poolSearchLoading && !poolSearchError && poolSearchResults.length === 0 ? (
                                    <div className="rounded-xl border border-zinc-200 bg-white/70 p-3 text-xs text-zinc-500 dark:border-white/10 dark:bg-white/5 dark:text-white/60">
                                        未找到相关池子。                                    </div>
                                ) : null}

                                {poolSearchResults.length > 0 ? (
                                    <div className="space-y-3">
                                        {poolSearchResults.map((pool, idx) => {
                                            const addr = String(pool?.pool_address || '').trim().toLowerCase();
                                            const key = `${String(pool?.protocol_version || '').trim()}:${addr || String(idx)}`;
                                            return (
                                                <HotPoolCard
                                                    key={key}
                                                    pool={pool}
                                                    metric={hotPoolsSort}
                                                    previousData={null}
                                                    accentTheme={accentTheme}
                                                    apiBaseUrl={apiBaseUrl}
                                                    onOpenKline={setKlinePool}
                                                    onOpenPosition={selectPoolFromSearch}
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
                                    <div className="flex items-center justify-between gap-3">
                                        <div className="min-w-0">
                                            <div className="text-[11px] font-semibold text-zinc-700 dark:text-white/80">筛选状态</div>
                                            <div className="mt-1 text-[11px] text-zinc-500 dark:text-white/40">
                                                {hotPoolsFilterDraft.enabled ? '已启用，应用后会按下方条件筛选' : '已关闭，条件会保留但不会生效'}
                                            </div>
                                        </div>
                                        <button
                                            type="button"
                                            onClick={() => setHotPoolsFilterDraft((prev) => ({ ...prev, enabled: !prev.enabled }))}
                                            className={`inline-flex min-w-[72px] items-center justify-center rounded-xl px-3 py-2 text-xs font-semibold shadow-sm transition ${hotPoolsFilterDraft.enabled
                                                ? brand.solidButtonClass
                                                : 'bg-white/70 text-zinc-700 ring-1 ring-zinc-200 hover:bg-white dark:bg-white/5 dark:text-white/70 dark:ring-white/10 dark:hover:bg-white/10'
                                                }`}
                                            aria-pressed={hotPoolsFilterDraft.enabled}
                                            title={hotPoolsFilterDraft.enabled ? '关闭筛选' : '启用筛选'}
                                        >
                                            {hotPoolsFilterDraft.enabled ? '已启用' : '已关闭'}
                                        </button>
                                    </div>
                                </div>
                                <div className="rounded-xl border border-zinc-200 bg-zinc-50 p-3 dark:border-white/10 dark:bg-[#0f1116]">
                                    <div className="mt-1">
                                        <div className="text-[11px] text-zinc-500 dark:text-white/40">搜索 (交易对 / 地址)</div>
                                        <input
                                            value={hotPoolsFilterDraft.keyword}
                                            onChange={(e) => setHotPoolsFilterDraft((prev) => ({ ...prev, keyword: e.target.value }))}
                                            className={`mt-1 w-full rounded-xl border border-zinc-200 bg-white/70 px-3 py-2 text-sm text-zinc-900 shadow-sm outline-none ring-0 placeholder:text-zinc-400 ${brand.inputFocusClass} dark:border-white/10 dark:bg-white/5 dark:text-white/90 dark:placeholder:text-white/30`}
                                            placeholder="例如 USDT"
                                        />
                                    </div>
                                    <div className="mt-3 grid grid-cols-2 gap-3">
                                        <div>
                                            <div className="text-[11px] text-zinc-500 dark:text-white/40">手续费 &gt;= (USD)</div>
                                            <input
                                                value={hotPoolsFilterDraft.minFees}
                                                onChange={(e) => setHotPoolsFilterDraft((prev) => ({ ...prev, minFees: e.target.value }))}
                                                inputMode="decimal"
                                                className={`mt-1 w-full rounded-xl border border-zinc-200 bg-white/70 px-3 py-2 text-sm text-zinc-900 shadow-sm outline-none ring-0 placeholder:text-zinc-400 ${brand.inputFocusClass} dark:border-white/10 dark:bg-white/5 dark:text-white/90 dark:placeholder:text-white/30`}
                                                placeholder={String(defaultHotPoolsFilter.minFees)}
                                            />
                                        </div>
                                        <div>
                                            <div className="text-[11px] text-zinc-500 dark:text-white/40">费率 &gt;= (%)</div>
                                            <input
                                                value={hotPoolsFilterDraft.minFeeRate}
                                                onChange={(e) => setHotPoolsFilterDraft((prev) => ({ ...prev, minFeeRate: e.target.value }))}
                                                inputMode="decimal"
                                                className={`mt-1 w-full rounded-xl border border-zinc-200 bg-white/70 px-3 py-2 text-sm text-zinc-900 shadow-sm outline-none ring-0 placeholder:text-zinc-400 ${brand.inputFocusClass} dark:border-white/10 dark:bg-white/5 dark:text-white/90 dark:placeholder:text-white/30`}
                                                placeholder={String(defaultHotPoolsFilter.minFeeRate)}
                                            />
                                        </div>
                                        <div>
                                            <div className="text-[11px] text-zinc-500 dark:text-white/40">活跃费率 &gt;= (%)</div>
                                            <input
                                                value={hotPoolsFilterDraft.minActiveFeeRate}
                                                onChange={(e) => setHotPoolsFilterDraft((prev) => ({ ...prev, minActiveFeeRate: e.target.value }))}
                                                inputMode="decimal"
                                                className={`mt-1 w-full rounded-xl border border-zinc-200 bg-white/70 px-3 py-2 text-sm text-zinc-900 shadow-sm outline-none ring-0 placeholder:text-zinc-400 ${brand.inputFocusClass} dark:border-white/10 dark:bg-white/5 dark:text-white/90 dark:placeholder:text-white/30`}
                                                placeholder="可选"
                                            />
                                        </div>
                                        <div>
                                            <div className="text-[11px] text-zinc-500 dark:text-white/40">TVL &gt;= (USD)</div>
                                            <input
                                                value={hotPoolsFilterDraft.minTvl}
                                                onChange={(e) => setHotPoolsFilterDraft((prev) => ({ ...prev, minTvl: e.target.value }))}
                                                inputMode="decimal"
                                                className={`mt-1 w-full rounded-xl border border-zinc-200 bg-white/70 px-3 py-2 text-sm text-zinc-900 shadow-sm outline-none ring-0 placeholder:text-zinc-400 ${brand.inputFocusClass} dark:border-white/10 dark:bg-white/5 dark:text-white/90 dark:placeholder:text-white/30`}
                                                placeholder={String(defaultHotPoolsFilter.minTvl)}
                                            />
                                        </div>
                                        <div>
                                            <div className="text-[11px] text-zinc-500 dark:text-white/40">交易量 &gt;= (USD)</div>
                                            <input
                                                value={hotPoolsFilterDraft.minVolume}
                                                onChange={(e) => setHotPoolsFilterDraft((prev) => ({ ...prev, minVolume: e.target.value }))}
                                                inputMode="decimal"
                                                className={`mt-1 w-full rounded-xl border border-zinc-200 bg-white/70 px-3 py-2 text-sm text-zinc-900 shadow-sm outline-none ring-0 placeholder:text-zinc-400 ${brand.inputFocusClass} dark:border-white/10 dark:bg-white/5 dark:text-white/90 dark:placeholder:text-white/30`}
                                                placeholder={String(defaultHotPoolsFilter.minVolume)}
                                            />
                                        </div>
                                        <div>
                                            <div className="text-[11px] text-zinc-500 dark:text-white/40">交易笔数 &gt;=</div>
                                            <input
                                                value={hotPoolsFilterDraft.minTxCount}
                                                onChange={(e) => setHotPoolsFilterDraft((prev) => ({ ...prev, minTxCount: e.target.value }))}
                                                inputMode="decimal"
                                                className={`mt-1 w-full rounded-xl border border-zinc-200 bg-white/70 px-3 py-2 text-sm text-zinc-900 shadow-sm outline-none ring-0 placeholder:text-zinc-400 ${brand.inputFocusClass} dark:border-white/10 dark:bg-white/5 dark:text-white/90 dark:placeholder:text-white/30`}
                                                placeholder="可选"
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
                                            应用
                                        </button>
                                        <button
                                            type="button"
                                            onClick={resetHotPoolsFilter}
                                            className="inline-flex items-center gap-2 rounded-xl bg-white/70 px-3 py-2 text-xs font-semibold text-zinc-700 ring-1 ring-zinc-200 hover:bg-white dark:bg-white/5 dark:text-white/70 dark:ring-white/10"
                                            aria-label="恢复默认筛选"
                                            title="恢复默认筛选"
                                        >
                                            <Icon path={icons.reset} className="h-4 w-4" />
                                            默认
                                        </button>
                                        <button
                                            type="button"
                                            onClick={clearHotPoolsFilter}
                                            className="inline-flex items-center gap-2 rounded-xl bg-white/70 px-3 py-2 text-xs font-semibold text-zinc-700 ring-1 ring-zinc-200 hover:bg-white dark:bg-white/5 dark:text-white/70 dark:ring-white/10"
                                            aria-label="清空筛选条件"
                                            title="清空筛选条件"
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
                                                <div>止损开关</div>
                                                <div className="mt-1 text-sm font-semibold text-zinc-900 dark:text-white/80">{formatOnOff(globalCfg.stop_loss_enabled)}</div>
                                            </div>
                                            <div>
                                                <div>止损延迟</div>
                                                <div className="mt-1 text-sm font-semibold text-zinc-900 dark:text-white/80">
                                                    <NumberFlowValue value={stopLossDelayText} formatter={() => stopLossDelayText} />
                                                </div>
                                            </div>
                                            <div>
                                                <div>自动复投</div>
                                                <div className="mt-1 text-sm font-semibold text-zinc-900 dark:text-white/80">{formatOnOff(globalCfg.auto_reinvest)}</div>
                                            </div>
                                            <div>
                                                <div>鏃ュ織閫氱煡</div>
                                                <div className="mt-1 text-sm font-semibold text-zinc-900 dark:text-white/80">{formatOnOff(globalCfg.extra_notifications_enabled)}</div>
                                            </div>
                                            <div>
                                                <div>杩囨护涓枃浠ｅ竵</div>
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
                                    aria-label="关闭设置"
                                >
                                    <Icon path={icons.close} className="h-5 w-5" />
                                </button>
                            </div>

                            <div className="mt-4 space-y-4">
                                <div className="rounded-xl border border-zinc-200 bg-zinc-50 p-3 dark:border-white/10 dark:bg-[#0f1116]">
                                    <div className="text-xs font-semibold text-zinc-900 dark:text-white/80">涓昏壊</div>
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
                                        默认间隔 <NumberFlowValue value={settingsPollIntervalSec} formatOptions={{ maximumFractionDigits: 0 }} />s
                                        {pollOverrideSec
                                            ? '已启用自定义。'
                                            : <>服务器默认 <NumberFlowValue value={settingsServerPollIntervalSec} formatOptions={{ maximumFractionDigits: 0 }} />s。</>}
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
                                <div className="truncate text-sm font-semibold text-zinc-900 dark:text-white/90">开仓</div>
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
                                                ].filter(Boolean).join(' | ')}
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
                                                                    {name || shortAddr || `钱包 ${id}`}
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

                            {openPositionShowPrivateZapProtectionHint ? (
                                <div className="rounded-xl border border-emerald-500/25 bg-gradient-to-br from-emerald-500/12 to-transparent p-3 dark:border-emerald-400/20 dark:from-emerald-400/10">
                                    <div className="flex items-start gap-3">
                                        <div className="mt-0.5 flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-emerald-500/15 text-emerald-600 dark:text-emerald-300">
                                            <Check className="h-3 w-3" strokeWidth={3} />
                                        </div>
                                        <div className="min-w-0">
                                            <div className="text-xs font-semibold text-zinc-900 dark:text-white/85">私有合约保驾护航</div>
                                            <div className="mt-1 text-[11px] leading-5 text-zinc-600 dark:text-white/60">
                                                首次开仓会自动部署与当前钱包绑定的专属合约。部署成功后可直接复用，不会重复产生部署消耗。                                            </div>
                                        </div>
                                    </div>
                                </div>
                            ) : null}

                            <div className="rounded-xl border border-zinc-200 bg-zinc-50 p-3 dark:border-white/10 dark:bg-[#0f1116]">
                                <div className="text-xs font-semibold text-zinc-900 dark:text-white/80">开仓金额 (USDT)</div>
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
                                        onChange={(e) => handleOpenPositionRangeLowerChange(e.target.value)}
                                        inputMode="decimal"
                                        className={`w-full rounded-xl border border-zinc-200 bg-white/70 px-3 py-2 text-sm text-zinc-900 shadow-sm outline-none ring-0 placeholder:text-zinc-400 ${brand.inputFocusClass} dark:border-white/10 dark:bg-white/5 dark:text-white/90 dark:placeholder:text-white/30`}
                                        placeholder="下限 %"
                                    />
                                    <input
                                        value={openPositionRangeUpper}
                                        onChange={(e) => handleOpenPositionRangeUpperChange(e.target.value)}
                                        inputMode="decimal"
                                        className={`w-full rounded-xl border border-zinc-200 bg-white/70 px-3 py-2 text-sm text-zinc-900 shadow-sm outline-none ring-0 placeholder:text-zinc-400 ${brand.inputFocusClass} dark:border-white/10 dark:bg-white/5 dark:text-white/90 dark:placeholder:text-white/30`}
                                        placeholder="上限 %"
                                    />
                                </div>
                                {openPositionSmartRangesLoading ? (
                                    <div className="mt-2 text-[11px] text-zinc-500 dark:text-white/40">
                                        聪明钱区间加载中...
                                    </div>
                                ) : null}
                                <div className="mt-2 flex flex-wrap gap-1.5">
                                    {primaryQuickRangeOptions.map((option) => {
                                        const lowerValue = Number(option.lowerValue);
                                        const upperValue = Number(option.upperValue);
                                        const isActive =
                                            Math.abs(Number(openPositionRangeLower) - lowerValue) < 0.05 &&
                                            Math.abs(Number(openPositionRangeUpper) - upperValue) < 0.05;
                                        return (
                                            <button
                                                key={option.key}
                                                type="button"
                                                onClick={() => {
                                                    setOpenPositionRangeLower(option.lowerValue);
                                                    setOpenPositionRangeUpper(option.upperValue);
                                                    setOpenPositionRangeUpperAuto(true);
                                                    setOpenPositionError('');
                                                }}
                                                className={`inline-flex min-w-[72px] flex-col items-start rounded-lg border px-2 py-1.5 text-left text-[11px] font-semibold transition ${isActive
                                                    ? `${brand.selectionClass} text-zinc-900 dark:text-white`
                                                    : option.smart
                                                        ? 'border-amber-200 bg-gradient-to-r from-amber-50 via-amber-100/60 to-yellow-100/60 text-amber-700 hover:from-amber-100 hover:via-amber-200/70 hover:to-yellow-200/70 dark:border-amber-400/30 dark:from-amber-500/10 dark:via-amber-400/10 dark:to-yellow-400/10 dark:text-amber-200'
                                                        : 'border-zinc-200 bg-white/70 text-zinc-700 hover:bg-zinc-100 dark:border-white/10 dark:bg-white/5 dark:text-white/75 dark:hover:bg-white/10'
                                                    }`}
                                            >
                                                <span className="leading-none">{option.label}</span>
                                                {option.subLabel ? (
                                                    <span className="mt-1 text-[10px] font-medium opacity-70">{option.subLabel}</span>
                                                ) : null}
                                            </button>
                                        );
                                    })}
                                </div>
                                {smartQuickRangeOptions.length > 0 ? (
                                    <>
                                        <div className="mt-2 text-[11px] text-zinc-500 dark:text-white/40">
                                            聪明钱近期开仓金额                                        </div>
                                        <div className="mt-2 flex flex-wrap gap-1.5">
                                            {defaultQuickRangeOptions.map((option) => {
                                                const lowerValue = Number(option.lowerValue);
                                                const upperValue = Number(option.upperValue);
                                                const isActive =
                                                    Math.abs(Number(openPositionRangeLower) - lowerValue) < 0.05 &&
                                                    Math.abs(Number(openPositionRangeUpper) - upperValue) < 0.05;
                                                return (
                                                    <button
                                                        key={`default-${option.key}`}
                                                        type="button"
                                                        onClick={() => {
                                                            setOpenPositionRangeLower(option.lowerValue);
                                                            setOpenPositionRangeUpper(option.upperValue);
                                                            setOpenPositionRangeUpperAuto(true);
                                                            setOpenPositionError('');
                                                        }}
                                                        className={`rounded-lg border px-2 py-1 text-[11px] font-semibold transition ${isActive
                                                            ? `${brand.selectionClass} text-zinc-900 dark:text-white`
                                                            : 'border-zinc-200 bg-white/70 text-zinc-700 hover:bg-zinc-100 dark:border-white/10 dark:bg-white/5 dark:text-white/75 dark:hover:bg-white/10'
                                                            }`}
                                                    >
                                                        {option.label}
                                                    </button>
                                                );
                                            })}
                                        </div>
                                        <div className="mt-2 text-[11px] text-zinc-500 dark:text-white/40">
                                            下方为默认区间                                        </div>
                                    </>
                                ) : null}
                                <div className="mt-2 text-[11px] text-zinc-500 dark:text-white/40">
                                    输入下限和上限百分比。例如 1 / 3 表示下跌 1%、上涨 3%。                                </div>
                            </div>

                            <div className="rounded-xl border border-zinc-200 bg-zinc-50 p-3 dark:border-white/10 dark:bg-[#0f1116]">
                                <div className="text-xs font-semibold text-zinc-900 dark:text-white/80">滑点 (%)</div>
                                <div className="mt-0.5 text-[11px] text-zinc-500 dark:text-white/40">留空则使用全局滑点设置。</div>
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

                            {(openPositionRecommendedPositions.length > 0 || openPositionSizingWarnings.length > 0) ? (
                                <div className="rounded-xl border border-zinc-200 bg-zinc-50 p-3 dark:border-white/10 dark:bg-[#0f1116]">
                                    <div className="text-xs font-semibold text-zinc-900 dark:text-white/80">最优加仓建议</div>
                                    {openPositionSizingInputs ? (
                                        <div className="mt-2 flex flex-wrap gap-2 text-[11px] text-zinc-500 dark:text-white/45">
                                            {Number.isFinite(Number(openPositionSizingInputs?.active_liquidity_usd)) ? (
                                                <span>活跃流动性 {formatUsdCompact(openPositionSizingInputs.active_liquidity_usd)}</span>
                                            ) : null}
                                            {Number.isFinite(Number(openPositionSizingInputs?.capital_total)) ? (
                                                <span>钱包资金 {formatUsdCompact(openPositionSizingInputs.capital_total)}</span>
                                            ) : null}
                                            {Number.isFinite(Number(openPositionSizingInputs?.effective_risk_cap_usd)) ? (
                                                <span>有效上限 {formatUsdCompact(openPositionSizingInputs.effective_risk_cap_usd)}</span>
                                            ) : null}
                                        </div>
                                    ) : null}

                                    {openPositionRecommendedPositions.length > 0 ? (
                                        <div className="mt-3 space-y-2">
                                            {openPositionRecommendedPositions.map((item, index) => {
                                                const efficiencyMeta = getSizingEfficiencyMeta(item?.efficiency);
                                                return (
                                                    <div
                                                        key={`${item?.mode || 'mode'}-${index}`}
                                                        className="rounded-xl border border-zinc-200/80 bg-white/70 p-3 dark:border-white/10 dark:bg-white/[0.04]"
                                                    >
                                                        <div className="flex items-center justify-between gap-2">
                                                            <div className="text-[13px] font-semibold text-zinc-900 dark:text-white/85">
                                                                {formatSizingModeLabel(item?.mode)}
                                                            </div>
                                                            <span className={`rounded-full border px-2 py-1 text-[11px] font-semibold ${efficiencyMeta.chipClass}`}>
                                                                {efficiencyMeta.label}
                                                            </span>
                                                        </div>
                                                        <div className="mt-3 grid grid-cols-3 gap-2">
                                                            <div>
                                                                <div className="text-[11px] text-zinc-500 dark:text-white/45">推荐加仓</div>
                                                                <div className="mt-1 text-sm font-semibold text-zinc-900 dark:text-white/85">
                                                                    {formatUsdCompact(item?.liquidity_to_add)}
                                                                </div>
                                                            </div>
                                                            <div>
                                                                <div className="text-[11px] text-zinc-500 dark:text-white/45">预期占比</div>
                                                                <div className="mt-1 text-sm font-semibold text-zinc-900 dark:text-white/85">
                                                                    {formatSharePercent(item?.expected_share)}
                                                                </div>
                                                            </div>
                                                            <div>
                                                                <div className="text-[11px] text-zinc-500 dark:text-white/45">风险暴露</div>
                                                                <div className="mt-1 text-sm font-semibold text-zinc-900 dark:text-white/85">
                                                                    {formatUsdCompact(item?.risk_exposure)}
                                                                </div>
                                                            </div>
                                                        </div>
                                                    </div>
                                                );
                                            })}
                                        </div>
                                    ) : null}

                                    {openPositionSizingWarnings.length > 0 ? (
                                        <div className="mt-3 space-y-2">
                                            {openPositionSizingWarnings.map((warning, index) => (
                                                <div
                                                    key={`${warning}-${index}`}
                                                    className="rounded-lg border border-amber-500/25 bg-amber-500/10 px-3 py-2 text-[11px] leading-tight text-amber-700 dark:text-amber-200"
                                                >
                                                    {warning}
                                                </div>
                                            ))}
                                        </div>
                                    ) : null}
                                </div>
                            ) : null}

                            {(openPositionEntrySwapPreviewLoading || openPositionDisplayChecks.length > 0 || openPositionEntrySwapPreviewError) ? (
                                <div className="rounded-xl border border-zinc-200 bg-zinc-50 p-3 dark:border-white/10 dark:bg-[#0f1116]">
                                    <div className="text-xs font-semibold text-zinc-900 dark:text-white/80 mb-2">安全检查</div>
                                    {openPositionEntrySwapPreviewLoading ? (
                                        <div className="text-[11px] text-zinc-500 dark:text-white/40">正在检查...</div>
                                    ) : null}
                                    {openPositionEntrySwapPreviewError ? (
                                        <div className="mt-1 rounded-lg border border-red-500/30 bg-red-500/10 p-2 text-[11px] text-red-700 dark:text-red-200">
                                            {openPositionEntrySwapPreviewError}
                                        </div>
                                    ) : null}
                                    {openPositionDisplayChecks.length > 0 ? (
                                        <div className="space-y-2">
                                            {openPositionDisplayChecks.map((item) => {
                                                const isPass = item.status === 'pass';
                                                const isWarn = item.status === 'warn';
                                                const isFail = item.status === 'fail';
                                                return (
                                                    <div key={item.key} className="rounded-lg p-2 " style={{
                                                        background: isFail ? 'rgba(239,68,68,0.07)' : isWarn ? 'rgba(234,179,8,0.07)' : 'rgba(34,197,94,0.07)'
                                                    }}>
                                                        <div className="flex items-start gap-2">
                                                            <div className={`mt-0.5 shrink-0 ${isFail ? 'text-red-500' : isWarn ? 'text-amber-500' : 'text-emerald-500'}`}>
                                                                {isFail ? <XCircle className="h-4 w-4" /> : isWarn ? <AlertTriangle className="h-4 w-4" /> : <CheckCircle className="h-4 w-4" />}
                                                            </div>
                                                            <div className="flex-1 min-w-0">
                                                                <div className="flex items-center justify-between gap-2">
                                                                    <span className={`text-[11px] font-semibold ${isFail ? 'text-red-700 dark:text-red-300' : isWarn ? 'text-amber-700 dark:text-amber-300' : 'text-emerald-700 dark:text-emerald-300'}`}>{item.label}</span>
                                                                    {item.detail ? <span className="text-[10px] text-zinc-500 dark:text-white/40 text-right">{item.detail}</span> : null}
                                                                </div>
                                                                {false && openPositionEntrySwapPreview?.required ? (
                                                                    <div className="mt-2 space-y-1 text-[11px] text-zinc-600 dark:text-white/60">
                                                                        <div>兑换路径：{openPositionEntrySwapPreview?.amount_in || '--'} {openPositionEntrySwapPreview?.from_token_symbol || ''} → {openPositionEntrySwapPreview?.to_token_symbol || '--'}</div>
                                                                        <div>预计到账：{openPositionEntrySwapPreview?.expected_amount_out || '--'} {openPositionEntrySwapPreview?.to_token_symbol || ''}</div>
                                                                        <div>推荐滑点：{Number(openPositionEntrySwapPreview?.recommended_slippage_tolerance).toFixed(3).replace(/0+$/, '').replace(/\.$/, '')}%</div>
                                                                        <input
                                                                            value={openPositionEntrySwapSlippage}
                                                                            onChange={(e) => {
                                                                                setOpenPositionEntrySwapSlippageDirty(true);
                                                                                setOpenPositionEntrySwapSlippage(e.target.value);
                                                                                setOpenPositionError('');
                                                                            }}
                                                                            inputMode="decimal"
                                                                            className={`mt-1 w-full rounded-xl border border-zinc-200 bg-white/70 px-3 py-1.5 text-sm text-zinc-900 shadow-sm outline-none ring-0 placeholder:text-zinc-400 ${brand.inputFocusClass} dark:border-white/10 dark:bg-white/5 dark:text-white/90 dark:placeholder:text-white/30`}
                                                                            placeholder="前置兑换滑点（可选）"
                                                                        />
                                                                        <label className="mt-2 flex items-start gap-2">
                                                                            <input
                                                                                type="checkbox"
                                                                                checked={openPositionEntrySwapConfirm}
                                                                                onChange={(e) => {
                                                                                    setOpenPositionEntrySwapConfirm(e.target.checked);
                                                                                    setOpenPositionError('');
                                                                                }}
                                                                                disabled={openPositionLoading || openPositionEntrySwapPreviewLoading}
                                                                            />
                                                                            <span className="text-[11px] leading-tight">我已确认本次前置兑换</span>
                                                                        </label>
                                                                    </div>
                                                                ) : null}
                                                                {isWarn ? (
                                                                    <div className="mt-2 text-[11px] leading-tight opacity-80">已提示风险，可直接继续；若要开仓，请留意滑点、成交波动和单次限额。</div>
                                                                ) : null}
                                                            </div>
                                                        </div>
                                                    </div>
                                                );
                                            })}
                                        </div>
                                    ) : null}
                                </div>
                            ) : null}

                            {(openPositionEntrySwapPreviewLoading || openPositionEntrySwapPreview?.required) ? (
                                <div className="rounded-xl border border-zinc-200 bg-zinc-50 p-3 dark:border-white/10 dark:bg-[#0f1116]">
                                    <div className="text-xs font-semibold text-zinc-900 dark:text-white/80 mb-2">前置兑换</div>
                                    {openPositionEntrySwapPreviewLoading ? (
                                        <div className="text-[11px] text-zinc-500 dark:text-white/40">正在获取推荐滑点和预计到账...</div>
                                    ) : null}
                                    {openPositionEntrySwapPreview?.required ? (
                                        <div className="space-y-1 text-[11px] text-zinc-600 dark:text-white/60">
                                            <div>推荐滑点：{Number(openPositionEntrySwapPreview?.recommended_slippage_tolerance).toFixed(3).replace(/0+$/, '').replace(/\.$/, '')}%</div>
                                            <div>当前滑点：{Number(openPositionEntrySwapPreview?.current_slippage_tolerance).toFixed(3).replace(/0+$/, '').replace(/\.$/, '')}%</div>
                                            <div>预计到账：{openPositionEntrySwapPreview?.expected_amount_out || '--'} {openPositionEntrySwapPreview?.to_token_symbol || ''}</div>
                                            <div>兑换路径：{openPositionEntrySwapPreview?.amount_in || '--'} {openPositionEntrySwapPreview?.from_token_symbol || ''} → {openPositionEntrySwapPreview?.to_token_symbol || '--'}</div>
                                            <input
                                                value={openPositionEntrySwapSlippage}
                                                onChange={(e) => {
                                                    setOpenPositionEntrySwapSlippageDirty(true);
                                                    setOpenPositionEntrySwapSlippage(e.target.value);
                                                    setOpenPositionError('');
                                                }}
                                                inputMode="decimal"
                                                className={`mt-2 w-full rounded-xl border border-zinc-200 bg-white/70 px-3 py-2 text-sm text-zinc-900 shadow-sm outline-none ring-0 placeholder:text-zinc-400 ${brand.inputFocusClass} dark:border-white/10 dark:bg-white/5 dark:text-white/90 dark:placeholder:text-white/30`}
                                                placeholder="前置兑换滑点（可选）"
                                            />
                                            <label className="mt-2 flex items-start gap-2">
                                                <input
                                                    type="checkbox"
                                                    checked={openPositionEntrySwapConfirm}
                                                    onChange={(e) => {
                                                        setOpenPositionEntrySwapConfirm(e.target.checked);
                                                        setOpenPositionError('');
                                                    }}
                                                    disabled={openPositionLoading || openPositionEntrySwapPreviewLoading}
                                                />
                                                <span className="text-[11px] leading-tight">我已确认本次前置兑换，先执行兑换，再继续后续开仓。</span>
                                            </label>
                                        </div>
                                    ) : null}
                                </div>
                            ) : null}

                            {openPositionError ? (
                                <div className="rounded-2xl border border-red-500/40 bg-gradient-to-br from-red-500/10 to-transparent p-4 shadow-sm text-red-800 dark:border-red-500/30 dark:text-red-200">
                                    <div className="flex items-start gap-3">
                                        <div className="mt-0.5 flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-red-500/20 text-red-600 dark:text-red-400">
                                            <X className="h-3 w-3" strokeWidth={3} />
                                        </div>
                                        <div className="text-[12px] font-medium leading-relaxed">
                                            {openPositionError}
                                        </div>
                                    </div>
                                </div>
                            ) : null}
                            <button
                                type="button"
                                onClick={handleOpenPosition}
                                disabled={openPositionSubmitDisabled}
                                className={`w-full rounded-xl px-3 py-2 text-sm font-semibold shadow-sm transition ${openPositionSubmitDisabled
                                    ? 'cursor-not-allowed bg-zinc-200 text-zinc-500 shadow-none dark:bg-white/10 dark:text-white/30'
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
                                    <div className="text-xs font-semibold text-zinc-900 dark:text-white/80">新区间 (%)</div>
                                    <div className="mt-2 grid grid-cols-2 gap-2">
                                        <input
                                            value={taskRangeLower}
                                            onChange={(e) => handleTaskRangeLowerChange(e.target.value)}
                                            inputMode="decimal"
                                            className={`w-full rounded-xl border border-zinc-200 bg-white/70 px-3 py-2 text-sm text-zinc-900 shadow-sm outline-none ring-0 placeholder:text-zinc-400 ${brand.inputFocusClass} dark:border-white/10 dark:bg-white/5 dark:text-white/90 dark:placeholder:text-white/30`}
                                            placeholder="下限 %"
                                        />
                                        <input
                                            value={taskRangeUpper}
                                            onChange={(e) => handleTaskRangeUpperChange(e.target.value)}
                                            inputMode="decimal"
                                            className={`w-full rounded-xl border border-zinc-200 bg-white/70 px-3 py-2 text-sm text-zinc-900 shadow-sm outline-none ring-0 placeholder:text-zinc-400 ${brand.inputFocusClass} dark:border-white/10 dark:bg-white/5 dark:text-white/90 dark:placeholder:text-white/30`}
                                            placeholder="上限 %"
                                        />
                                    </div>
                                    <div className="mt-2 text-[11px] text-zinc-500 dark:text-white/40">
                                        修改后的区间将在【下次再平衡时】生效。                                    </div>
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
                                            placeholder="USDT amount"
                                        />
                                    </div>
                                    <div className="mt-2 text-[11px] text-zinc-500 dark:text-white/40">
                                        当前持仓不会直接变动，金额和区间都将在【下次再平衡时】生效。                                    </div>
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

            {/* 鈹€鈹€鈹€ 琛ュ厖娴佸姩鎬?Modal 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€ */}
            {addLiqModal ? (
                <div className="fixed inset-0 z-[60]">
                    <button
                        type="button"
                        className="absolute inset-0 bg-black/40"
                        onClick={closeAddLiqModal}
                        aria-label="关闭"
                    />
                    <div className="absolute inset-x-0 bottom-0 rounded-t-[28px] border border-zinc-200 bg-white p-4 shadow-2xl dark:border-white/10 dark:bg-[#111318] dark:shadow-none">
                        <div className="flex items-center justify-between gap-2">
                            <div className="min-w-0">
                                <div className="text-sm font-semibold text-zinc-900 dark:text-white/90">补充流动性</div>
                                <div className="mt-0.5 text-[11px] text-zinc-500 dark:text-white/40 truncate">
                                    {addLiqPosition?.title || addLiqModal.title}
                                </div>
                            </div>
                            <button
                                type="button"
                                onClick={closeAddLiqModal}
                                disabled={addLiqLoading}
                                className="inline-flex h-9 w-9 items-center justify-center rounded-xl border border-zinc-200 bg-zinc-100 text-zinc-900 hover:bg-zinc-200 active:bg-zinc-200 dark:border-white/10 dark:bg-white/5 dark:text-white/80 dark:hover:bg-white/10 dark:active:bg-white/15"
                                aria-label="关闭"
                            >
                                <X className="h-5 w-5" />
                            </button>
                        </div>

                        <div className="mt-4 space-y-3">
                            <div className="rounded-[28px] border border-zinc-200/90 bg-[radial-gradient(circle_at_top_left,rgba(188,255,47,0.16),transparent_40%),radial-gradient(circle_at_bottom_right,rgba(16,185,129,0.10),transparent_38%),linear-gradient(180deg,rgba(255,255,255,0.98),rgba(244,244,245,0.95))] p-4 shadow-[0_18px_44px_rgba(15,23,42,0.08)] dark:border-white/10 dark:bg-[radial-gradient(circle_at_top_left,rgba(188,255,47,0.16),transparent_40%),radial-gradient(circle_at_bottom_right,rgba(16,185,129,0.10),transparent_38%),linear-gradient(180deg,rgba(20,24,18,0.96),rgba(11,14,12,0.98))] dark:shadow-none">
                                <div className="flex items-start gap-3">
                                    <div className={`inline-flex h-11 w-11 items-center justify-center rounded-2xl ${brand.iconChipClass}`}>
                                        <Droplets className="h-5 w-5" />
                                    </div>
                                    <div className="min-w-0 flex-1">
                                        <div className="text-[11px] font-semibold uppercase tracking-[0.22em] text-zinc-500 dark:text-white/35">USDT Top-up</div>
                                        <div className="mt-1 text-sm font-semibold text-zinc-950 dark:text-white">为当前仓位追加预算</div>
                                        <div className="mt-1 text-[11px] leading-5 text-zinc-500 dark:text-white/45">
                                            {addLiqHintText}
                                        </div>
                                    </div>
                                </div>

                                <div className="mt-4 grid grid-cols-2 gap-2">
                                    <div className="rounded-2xl border border-zinc-200/80 bg-white/80 px-3 py-3 dark:border-white/10 dark:bg-white/5">
                                        <div className="text-[10px] font-semibold uppercase tracking-[0.18em] text-zinc-500 dark:text-white/35">当前仓位</div>
                                        <div className="mt-1 text-base font-semibold text-zinc-950 dark:text-white">
                                            {addLiqCurrentValue > 0 ? formatUsdCompact(addLiqCurrentValue) : '$--'}
                                        </div>
                                    </div>
                                    <div className="rounded-2xl border border-zinc-200/80 bg-white/80 px-3 py-3 dark:border-white/10 dark:bg-white/5">
                                        <div className="text-[10px] font-semibold uppercase tracking-[0.18em] text-zinc-500 dark:text-white/35">策略参考</div>
                                        <div className="mt-1 text-base font-semibold text-zinc-950 dark:text-white">
                                            {addLiqReferenceAmount > 0 ? formatUsdCompact(addLiqReferenceAmount) : '$--'}
                                        </div>
                                    </div>
                                </div>

                                <div className={`mt-4 rounded-[22px] border px-4 py-4 transition ${Number.isFinite(addLiqParsedAmount) && addLiqParsedAmount > 0
                                    ? brand.selectionClass
                                    : 'border-zinc-200 bg-white/80 dark:border-white/10 dark:bg-white/5'
                                }`}>
                                    <div className="flex items-center justify-between gap-2">
                                        <div className="text-[11px] font-semibold uppercase tracking-[0.22em] text-zinc-500 dark:text-white/35">补充金额</div>
                                        <div className={`rounded-full px-2.5 py-1 text-[10px] font-semibold ${brand.softButtonClass}`}>
                                            Custom
                                        </div>
                                    </div>
                                    <div className="mt-3 flex items-center gap-3">
                                        <span className="text-xl font-semibold text-zinc-400 dark:text-white/35">$</span>
                                        <input
                                            value={addLiqAmount}
                                            onChange={(e) => { setAddLiqAmount(e.target.value); setAddLiqError(''); }}
                                            onKeyDown={(e) => {
                                                if (e.key === 'Enter') {
                                                    e.preventDefault();
                                                    submitAddLiquidity();
                                                }
                                            }}
                                            inputMode="decimal"
                                            placeholder="0.00"
                                            autoFocus
                                            disabled={addLiqLoading}
                                            className="min-w-0 flex-1 border-0 bg-transparent p-0 text-[30px] font-semibold tracking-[-0.04em] text-zinc-950 outline-none placeholder:text-zinc-300 dark:text-white dark:placeholder:text-white/20"
                                        />
                                        <span className="inline-flex items-center rounded-full border border-zinc-200 bg-white px-3 py-1 text-[11px] font-semibold text-zinc-700 shadow-sm dark:border-white/10 dark:bg-white/10 dark:text-white/75">
                                            USDT
                                        </span>
                                    </div>
                                </div>

                                {addLiqPresetOptions.length ? (
                                    <div className="mt-3 grid grid-cols-2 gap-2">
                                        {addLiqPresetOptions.map((preset) => {
                                            const active = Number.isFinite(addLiqParsedAmount) && Math.abs(addLiqParsedAmount - preset.value) < 0.001;
                                            return (
                                                <button
                                                    key={`${preset.value}-${preset.hint}`}
                                                    type="button"
                                                    disabled={addLiqLoading}
                                                    onClick={() => {
                                                        hapticSelection();
                                                        setAddLiqAmount(formatAmountInput(preset.value));
                                                        setAddLiqError('');
                                                    }}
                                                    className={`rounded-2xl border px-3 py-3 text-left transition ${active
                                                        ? brand.selectionClass
                                                        : 'border-zinc-200 bg-white/80 hover:bg-white dark:border-white/10 dark:bg-white/5 dark:hover:bg-white/10'
                                                    } ${addLiqLoading ? 'cursor-not-allowed opacity-60' : ''}`}
                                                >
                                                    <div className="text-sm font-semibold text-zinc-950 dark:text-white">{preset.label}</div>
                                                    <div className="mt-1 text-[11px] text-zinc-500 dark:text-white/40">{preset.hint}</div>
                                                </button>
                                            );
                                        })}
                                    </div>
                                ) : null}
                            </div>
                            <div className="hidden rounded-xl border border-zinc-200 bg-zinc-50 p-3 dark:border-white/10 dark:bg-[#0f1116]">
                                <div className="text-xs font-semibold text-zinc-900 dark:text-white/80 mb-2">补充金额 (USDT)</div>
                                <input
                                    value={addLiqAmount}
                                    onChange={(e) => { setAddLiqAmount(e.target.value); setAddLiqError(''); }}
                                    inputMode="decimal"
                                    placeholder="请输入 USDT 金额"
                                    disabled={addLiqLoading}
                                    className={`w-full rounded-xl border border-zinc-200 bg-white/70 px-3 py-2 text-sm text-zinc-900 shadow-sm outline-none ring-0 placeholder:text-zinc-400 ${brand.inputFocusClass} dark:border-white/10 dark:bg-white/5 dark:text-white/90 dark:placeholder:text-white/30`}
                                />
                                <div className="mt-2 text-[11px] text-zinc-500 dark:text-white/40">
                                    将使用 USDT 按当前池价买入并补充至现有仓位。                                </div>
                            </div>

                            {addLiqError ? (
                                <div className="rounded-xl border border-red-500/30 bg-red-500/10 p-3 text-xs text-red-700 dark:text-red-200">
                                    {addLiqError}
                                </div>
                            ) : null}

                            <button
                                type="button"
                                onClick={submitAddLiquidity}
                                disabled={addLiqLoading || !(Number.isFinite(addLiqParsedAmount) && addLiqParsedAmount > 0)}
                                className={`w-full rounded-2xl px-3 py-3 text-sm font-semibold shadow-sm transition ${addLiqLoading || !(Number.isFinite(addLiqParsedAmount) && addLiqParsedAmount > 0)
                                    ? `${brand.solidButtonClass} cursor-not-allowed opacity-60`
                                    : brand.solidButtonClass
                                }`}
                            >
                                {addLiqLoading ? (
                                    <span className="flex items-center justify-center gap-2">
                                        <svg className="h-4 w-4 animate-spin" viewBox="0 0 24 24" fill="none">
                                            <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
                                            <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v4a4 4 0 00-4 4H4z" />
                                        </svg>
                                        处理中，请稍候...
                                    </span>
                                ) : '确认补充'}
                            </button>
                        </div>
                    </div>
                </div>
            ) : null}

            {
                confirmState ? (
                    <div className="fixed inset-0 z-[60] flex items-end sm:items-center justify-center sm:p-4">
                        <button
                            type="button"
                            className="absolute inset-0 bg-black/60 backdrop-blur-sm"
                            onClick={() => closeConfirm(false)}
                            aria-label="鍙栨秷纭"
                        />
                        <div className="relative w-full max-w-md overflow-hidden rounded-t-2xl sm:rounded-2xl border border-zinc-200 bg-white p-4 shadow-2xl dark:border-white/10 dark:bg-[#111318]">
                            <div className="flex items-center justify-between gap-2">
                                <div className="text-sm font-semibold text-zinc-900 dark:text-white/90">{confirmState.title}</div>
                                <button
                                    type="button"
                                    onClick={() => closeConfirm(false)}
                                    className="inline-flex h-8 w-8 items-center justify-center rounded-lg border border-zinc-200 bg-zinc-100 text-zinc-900 hover:bg-zinc-200 active:bg-zinc-200 dark:border-white/10 dark:bg-white/5 dark:text-white/80 dark:hover:bg-white/10 dark:active:bg-white/15"
                                    aria-label="鍏抽棴纭寮圭獥"
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
                                    {confirmState.cancelText || '鍙栨秷'}
                                </button>
                                <button
                                    type="button"
                                    onClick={() => closeConfirm(true)}
                                    className={`flex-1 rounded-xl px-3 py-2 text-sm font-semibold ${confirmButtonClass}`}
                                >
                                    {confirmState.confirmText || '纭'}
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
                        if (item.key === 'assets') iconPath = icons.wallet;
                        if (item.key === 'smart_money') iconPath = icons.eye;
                        if (item.key === 'admin') iconPath = icons.gear;
                        if (item.key === 'admin_page') iconPath = icons.gear;

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
                    accentTheme={accentTheme}
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
