import { Suspense, lazy, useEffect, useMemo, useRef, useState, useCallback } from 'react';
import HotPoolCard from './components/HotPoolCard.jsx';
import KlineModal from './components/KlineModal.jsx';
import PositionCard from './components/PositionCard.jsx';
import SystemConfigCard from './components/SystemConfigCard.jsx';
import BottomSheet from './components/BottomSheet.jsx';
import ModuleHeader from './components/ModuleHeader.jsx';
import NumberFlowValue from './components/NumberFlowValue.jsx';
import StepProgressModal from './components/StepProgressModal.jsx';
import LiquidityDistributionChart from './components/LiquidityDistributionChart.jsx';
import GlobalConfigPage from './components/GlobalConfigPage.jsx';
import CustomSelect from './components/CustomSelect.jsx';
import { SkeletonHotPoolCard, SkeletonPositionCard, SkeletonList } from './components/Skeleton.jsx';
import SmartMoneyPage from './components/SmartMoneyPage.jsx';
import { Bot, BarChart2, Droplets, Filter, Search, Moon, Sun, Settings, X, Check, RotateCcw, AlertTriangle, Flame, Eye, Wallet, ArrowLeftRight } from 'lucide-react';
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
    updateTaskRange,
    setTaskPaused,
    stopTask,
    withdrawLiquidity,
    swapDust,
    triggerRebalance,
    updateTaskMode,
    addLiquidity,
} from './lib/api';
import { fetchSMPoolStats } from './lib/smartMoneyApi';
import { hapticImpact, hapticNotification, hapticSelection } from './lib/telegram';
import { formatRelativeTime, useTick } from './lib/time';
import {
    ACCENT_THEME_OPTIONS,
    getBrandTheme,
    normalizeAccentTheme,
} from './lib/brand';
import { getTaskModeMeta, getOutOfRangeActionSummary as getTaskModeActionSummary, normalizeTaskMode } from './lib/taskModes';
import useScrollMemory from './hooks/useScrollMemory';
import useGlobalSettings from './hooks/useGlobalSettings';
import useAuthData from './hooks/useAuthData';
import useInitData from './hooks/useInitData';
import {
    formatUsd,
    formatUsdCompact,
    formatRangePercentCompact,
    formatSignedPercentCompact,
    normalizeTokenRisk,
    shortAddress,
    tokenRiskToneClass,
} from './lib/format';
import { resolveAllowEmptyInitData, resolveApiBaseUrl, localizeWebAppError } from './lib/apiBase';
import { storage } from './lib/storage';
import { buildTopNavItems, hasModuleAccess } from './features/appShell/moduleAccess';
import {
    MODULE_POLL_CONFIG,
    POSITIONS_ACTIVE_POLL_KEY,
    POSITIONS_IDLE_POLL_KEY,
    STORAGE_MODULE_POLL_SECS,
    STORAGE_POLL_SEC,
    clampModulePollSec,
    getModulePollConfig,
    getModulePollSec,
    normalizeModulePollOverrides,
} from './features/appShell/pollConfig';
import {
    DEFAULT_WEB_WORKBENCH_WIDGETS,
    STORAGE_WEB_WORKBENCH_WIDGETS,
    WEB_WORKBENCH_WIDGETS,
    normalizeWebWorkbenchWidgets,
} from './features/appShell/webWorkbench';
import { formatUserLabel } from './features/admin/formatUser';
import {
    HOT_POOL_SORT_TABS,
    HOT_POOLS_RISK_FILTER_ALL,
    HOT_POOLS_RISK_FILTER_OPTIONS,
    STORAGE_HOT_POOLS_FILTER,
    computeHotPoolActiveFeeRate,
    defaultHotPoolsFilter,
    formatDraftNumber,
    hotPoolMatchesRiskFilter,
    normalizeHotPoolsFilter,
    normalizeHotPoolsRiskFilter,
    parseDraftNumber,
    parseMetricNumber,
} from './features/hotPools/filter';
import { buildGmgnUrl } from './features/pools/gmgn';
import { comparePositionsByCreatedAt } from './features/positions/sort';
import {
    OPEN_POSITION_RANGE_OPTIONS,
    POSITION_SM_RANGE_BATCH_SIZE,
    POSITION_SM_RANGE_STALE_MS,
    STORAGE_OPEN_POSITION_HIDE_WALLET_BALANCES,
    STORAGE_OPEN_POSITION_WALLET_ID,
} from './features/openPosition/constants';
import {
    buildAddLiquidityPresetOptions,
    buildDCASummaryItems,
    formatAmountInput,
    formatPercentValue,
    formatPriceInputValue,
    formatPriceValue,
    formatRatioCompact,
    formatUSDTValue,
    parseAmountInput,
    parseOptionalPercent,
    resolvePositionSlippage,
} from './features/openPosition/format';
import {
    buildDefaultFocusedPercentageRange,
    buildDefaultFocusedTickRange,
    buildDisplayPriceRangeFromTicks,
    buildGridBins,
    estimateDisplayGridStepPercent,
    normalizeDisplayPriceTickRange,
    nudgeDisplayPriceBoundary,
    tickToPoolPrice,
} from './features/openPosition/tickMath';
import {
    normalizeOpenPositionPoolVersion,
    normalizePoolKey,
    normalizePositionSmartMoneyGroups,
    resolveOpenPositionPoolChain,
} from './features/openPosition/safety';
import { parseRangeInput } from './features/openPosition/range';
import useOpenPositionDraft from './features/openPosition/useOpenPositionDraft';
import useOpenPositionFlow from './features/openPosition/useOpenPositionFlow';
import useOpenPositionMarketData from './features/openPosition/useOpenPositionMarketData';
import {
    OpenPositionEntrySwapPreviewPanel,
    OpenPositionFooter,
    OpenPositionFundingStep,
    OpenPositionDCAPanel,
    OpenPositionOrderSummary,
    OpenPositionPrecheckPanel,
    OpenPositionStepIndicator,
    OpenPositionTaskModePanel,
} from './features/openPosition/OpenPositionSheetParts.jsx';

const LazyAdminPage = lazy(() => import('./components/AdminPage.jsx'));
const LazySwapModule = lazy(() => import('./components/SwapModule.jsx'));
const LazyAssetManagementPage = lazy(() => import('./components/AssetManagementPage.jsx'));

const CHAIN_SELECT_OPTIONS = [
    { value: 'bsc', label: 'BSC' },
    { value: 'base', label: 'Base' },
];

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
    swap: ArrowLeftRight,
};

const Icon = ({ path: IconCmp, className = '' }) => {
    if (!IconCmp) return null;
    return <IconCmp className={className} strokeWidth={2} />;
};

export default function App() {
    const initData = useInitData();
    const tick = useTick();
    const [me, setMe] = useState(null);
    const [data, setData] = useState(null);
    const [error, setError] = useState('');
    const [loading, setLoading] = useState(false);
    const pollRef = useRef(null);
    const [viewMode, setViewMode] = useState('hot_pools');
    useScrollMemory(viewMode);

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
        riskFilter: defaultHotPoolsFilter.riskFilter,
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
    const openPositionDraft = useOpenPositionDraft();
    const {
        openPositionPool,
        setOpenPositionPool,
        openPositionStep,
        setOpenPositionStep,
        openPositionAmount,
        setOpenPositionAmount,
        openPositionRangeLower,
        setOpenPositionRangeLower,
        openPositionRangeUpper,
        setOpenPositionRangeUpper,
        openPositionRangeUpperAuto,
        setOpenPositionRangeUpperAuto,
        openPositionRangeInputMode,
        setOpenPositionRangeInputMode,
        openPositionTickLower,
        setOpenPositionTickLower,
        openPositionTickUpper,
        setOpenPositionTickUpper,
        openPositionPriceLower,
        setOpenPositionPriceLower,
        openPositionPriceUpper,
        setOpenPositionPriceUpper,
        openPositionInvertPrice,
        setOpenPositionInvertPrice,
        openPositionGridBoundaryTarget,
        setOpenPositionGridBoundaryTarget,
        openPositionSlippage,
        setOpenPositionSlippage,
        openPositionError,
        setOpenPositionError,
        openPositionPrepareChecks,
        openPositionChecks,
        setOpenPositionChecks,
        openPositionRiskAck,
        setOpenPositionRiskAck,
        openPositionEntrySwapPreview,
        setOpenPositionEntrySwapPreview,
        openPositionEntrySwapPreviewLoading,
        setOpenPositionEntrySwapPreviewLoading,
        openPositionPreviewPending,
        setOpenPositionPreviewPending,
        openPositionPreviewSuspended,
        setOpenPositionPreviewSuspended,
        openPositionEntrySwapPreviewError,
        setOpenPositionEntrySwapPreviewError,
        openPositionDefaultRangeSeededRef,
        openPositionPreviewResumeTimerRef,
        openPositionAutoSingleSideRangeRef,
        openPositionPreparePrivateZapInfo,
        openPositionPrivateZapInfo,
        setOpenPositionPrivateZapInfo,
        openPositionPrepareTokenRisk,
        openPositionPreviewTokenRisk,
        openPositionRangeEditor,
        openPositionPreviewRangeEditor,
        openPositionSizingAdvice,
        setOpenPositionSizingAdvice,
        openPositionEntrySwapSlippage,
        setOpenPositionEntrySwapSlippage,
        openPositionEntrySwapSlippageDirty,
        setOpenPositionEntrySwapSlippageDirty,
        openPositionEntrySwapConfirm,
        openPositionLoading,
        setOpenPositionLoading,
        openPositionSmartRanges,
        setOpenPositionSmartRanges,
        openPositionSmartRangesLoading,
        setOpenPositionSmartRangesLoading,
        openPositionDCAEnabled,
        setOpenPositionDCAEnabled,
        openPositionDCAPercentages,
        setOpenPositionDCAPercentages,
        openPositionDCAInterval,
        setOpenPositionDCAInterval,
        openPositionDCAExpanded,
        setOpenPositionDCAExpanded,
        openPositionTaskMode,
        setOpenPositionTaskMode,
        openPositionWalletBalancesHidden,
        setOpenPositionWalletBalancesHidden,
        openPositionLiqProfile,
        setOpenPositionLiqProfile,
        openPositionLiqProfileLoading,
        setOpenPositionLiqProfileLoading,
        openPositionLiqProfileError,
        setOpenPositionLiqProfileError,
        openPositionWalletId,
        setOpenPositionWalletId,
        resetOpenPositionDraft,
    } = openPositionDraft;
    const [operationProgress, setOperationProgress] = useState(null);
    const [walletsData, setWalletsData] = useState(null);
    const [walletsError, setWalletsError] = useState('');
    const [walletsLoading, setWalletsLoading] = useState(false);
    const [taskRangeEdit, setTaskRangeEdit] = useState(null);
    const [taskRangeLower, setTaskRangeLower] = useState('');
    const [taskRangeUpper, setTaskRangeUpper] = useState('');
    const [taskRangeUpperAuto, setTaskRangeUpperAuto] = useState(true);
    const [taskRangeAmount, setTaskRangeAmount] = useState('');
    const [taskRangeError, setTaskRangeError] = useState('');
    const [taskRangeLoading, setTaskRangeLoading] = useState(false);

    const [addLiqModal, setAddLiqModal] = useState(null); // { taskId, title }
    const [addLiqAmount, setAddLiqAmount] = useState('');
    const [addLiqSlippage, setAddLiqSlippage] = useState('');
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

    const { theme, setTheme, toggleTheme, accentTheme, setAccentTheme } = useGlobalSettings();
    const [settingsOpen, setSettingsOpen] = useState(false);
    const [modulePollOverrides, setModulePollOverrides] = useState(() =>
        normalizeModulePollOverrides(storage.get(STORAGE_MODULE_POLL_SECS), storage.get(STORAGE_POLL_SEC))
    );
    const [modulePollDrafts, setModulePollDrafts] = useState({});
    const [confirmState, setConfirmState] = useState(null);
    const [notice, setNotice] = useState(null);
    const [globalConfigOpen, setGlobalConfigOpen] = useState(false);
    const [globalConfig, setGlobalConfig] = useState(null);

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
    useEffect(() => {
        storage.set(
            STORAGE_OPEN_POSITION_HIDE_WALLET_BALANCES,
            openPositionWalletBalancesHidden ? '1' : '0',
        );
    }, [openPositionWalletBalancesHidden]);
    const activeOpenPositionChecks = useMemo(() => (
        Array.isArray(openPositionChecks) && openPositionChecks.length > 0
            ? openPositionChecks
            : (Array.isArray(openPositionPrepareChecks) ? openPositionPrepareChecks : [])
    ), [openPositionChecks, openPositionPrepareChecks]);
    const activeOpenPositionPrivateZapInfo = openPositionPrivateZapInfo || openPositionPreparePrivateZapInfo;
    const openPositionTokenRisk = normalizeTokenRisk(
        openPositionPreviewTokenRisk || openPositionPrepareTokenRisk || openPositionPool?.token_risk
    );
    const openPositionTokenRiskTone = tokenRiskToneClass(openPositionTokenRisk);
    const openPositionTokenRiskSymbol = openPositionTokenRisk?.token_symbol || shortAddress(openPositionTokenRisk?.token_address);
    const openPositionEffectiveRangeEditor = openPositionPreviewRangeEditor || openPositionRangeEditor;
    const openPositionFailChecks = activeOpenPositionChecks.filter((item) => item.status === 'fail');
    const openPositionHasBlockingSafetyFailure = openPositionFailChecks.length > 0;
    const openPositionSubmitDisabled = openPositionLoading
        || openPositionPreviewPending
        || openPositionPreviewSuspended
        || openPositionHasBlockingSafetyFailure;
    const openPositionDisplayChecks = useMemo(() => (
        Array.isArray(activeOpenPositionChecks)
            ? activeOpenPositionChecks.filter((item) => {
                const key = String(item?.key || '').trim();
                return key !== 'entry_swap' && String(item?.status || '').trim() !== 'pass';
            })
            : []
    ), [activeOpenPositionChecks]);
    const openPositionShowPrivateZapProtectionHint = Boolean(activeOpenPositionPrivateZapInfo?.show_protection_hint);
    const openPositionRecommendedPositions = [];
    const openPositionWalletOptions = Array.isArray(walletsData?.wallets) ? walletsData.wallets : [];
    const openPositionTickLowerValue = Number(String(openPositionTickLower || '').trim());
    const openPositionTickUpperValue = Number(String(openPositionTickUpper || '').trim());
    const openPositionToken0Decimals = Number(openPositionPool?.token0_decimals ?? openPositionPool?.token0?.decimals ?? 18) || 18;
    const openPositionToken1Decimals = Number(openPositionPool?.token1_decimals ?? openPositionPool?.token1?.decimals ?? 18) || 18;
    const openPositionToken0Symbol = String(openPositionPool?.token0_symbol || openPositionPool?.token0?.symbol || '').toUpperCase();
    const openPositionToken1Symbol = String(openPositionPool?.token1_symbol || openPositionPool?.token1?.symbol || '').toUpperCase();
    const openPositionStableOrQuote = useMemo(
        () => new Set(['USDT', 'USDC', 'BUSD', 'DAI', 'FDUSD', 'TUSD', 'USDD', 'USDE']),
        [],
    );
    const openPositionQuoteIsToken1 = openPositionStableOrQuote.has(openPositionToken1Symbol)
        ? true
        : (openPositionStableOrQuote.has(openPositionToken0Symbol) ? false : undefined);
    const openPositionDefaultInvert = openPositionStableOrQuote.has(openPositionToken0Symbol);
    const openPositionPriceTickRange = useMemo(() => (
        normalizeDisplayPriceTickRange(
            openPositionPriceLower,
            openPositionPriceUpper,
            openPositionInvertPrice,
            openPositionToken0Decimals,
            openPositionToken1Decimals,
            Number(openPositionEffectiveRangeEditor?.tick_spacing),
            Number(openPositionEffectiveRangeEditor?.min_tick),
            Number(openPositionEffectiveRangeEditor?.max_tick),
        )
    ), [
        openPositionPriceLower,
        openPositionPriceUpper,
        openPositionInvertPrice,
        openPositionToken0Decimals,
        openPositionToken1Decimals,
        openPositionEffectiveRangeEditor?.tick_spacing,
        openPositionEffectiveRangeEditor?.min_tick,
        openPositionEffectiveRangeEditor?.max_tick,
    ]);
    const openPositionSelectedManualTickLower = useMemo(() => {
        if (openPositionRangeInputMode === 'price') return openPositionPriceTickRange?.lowerTick ?? null;
        return Number.isInteger(openPositionTickLowerValue) ? openPositionTickLowerValue : null;
    }, [openPositionRangeInputMode, openPositionPriceTickRange, openPositionTickLowerValue]);
    const openPositionSelectedManualTickUpper = useMemo(() => {
        if (openPositionRangeInputMode === 'price') return openPositionPriceTickRange?.upperTick ?? null;
        return Number.isInteger(openPositionTickUpperValue) ? openPositionTickUpperValue : null;
    }, [openPositionRangeInputMode, openPositionPriceTickRange, openPositionTickUpperValue]);
    // 向导步骤校验：与 preview useEffect 内的合法性判断保持一致，作为「下一步」按钮的 gate
    const openPositionStep0Valid = useMemo(() => {
        const amount = Number(String(openPositionAmount || '').trim());
        if (!Number.isFinite(amount) || amount <= 0) return false;
        return parseOptionalPercent(openPositionSlippage).valid;
    }, [openPositionAmount, openPositionSlippage]);
    const openPositionStep1Valid = useMemo(() => {
        if (openPositionRangeInputMode === 'percentage') {
            // 内联等价于 parseRangeInput（该函数定义在本 memo 之后，此处直接调用会触发 TDZ）
            const lowerN = Number(String(openPositionRangeLower || '').trim());
            const upperN = Number(String(openPositionRangeUpper || '').trim());
            if (!Number.isFinite(lowerN) || !Number.isFinite(upperN)) return false;
            const lower = Math.abs(lowerN);
            const upper = Math.abs(upperN);
            return lower > 0 && upper > 0 && lower < 100 && upper < 100;
        }
        return Number.isInteger(openPositionSelectedManualTickLower)
            && Number.isInteger(openPositionSelectedManualTickUpper)
            && openPositionSelectedManualTickLower < openPositionSelectedManualTickUpper;
    }, [openPositionRangeInputMode, openPositionRangeLower, openPositionRangeUpper, openPositionSelectedManualTickLower, openPositionSelectedManualTickUpper]);
    const openPositionSyncPriceInputsFromTicks = useCallback((lowerTick, upperTick) => {
        const displayRange = buildDisplayPriceRangeFromTicks(
            lowerTick,
            upperTick,
            openPositionInvertPrice,
            openPositionToken0Decimals,
            openPositionToken1Decimals,
        );
        if (!displayRange) return false;
        setOpenPositionPriceLower(formatPriceInputValue(displayRange.lowerPrice));
        setOpenPositionPriceUpper(formatPriceInputValue(displayRange.upperPrice));
        return true;
    }, [openPositionInvertPrice, openPositionToken0Decimals, openPositionToken1Decimals]);
    const applyOpenPositionTickRange = useCallback((lowerTick, upperTick, options = {}) => {
        if (!Number.isInteger(lowerTick) || !Number.isInteger(upperTick) || upperTick <= lowerTick) return false;
        setOpenPositionTickLower(String(lowerTick));
        setOpenPositionTickUpper(String(upperTick));
        openPositionSyncPriceInputsFromTicks(lowerTick, upperTick);
        if (options.clear !== false) setOpenPositionError('');
        return true;
    }, [openPositionSyncPriceInputsFromTicks]);
    const openPositionGridBins = useMemo(
        () => buildGridBins(openPositionEffectiveRangeEditor),
        [openPositionEffectiveRangeEditor],
    );
    const openPositionDefaultFocusedRange = useMemo(
        () => buildDefaultFocusedTickRange(openPositionEffectiveRangeEditor),
        [openPositionEffectiveRangeEditor],
    );
    const openPositionDefaultFocusedPercentageRange = useMemo(
        () => buildDefaultFocusedPercentageRange(openPositionEffectiveRangeEditor),
        [openPositionEffectiveRangeEditor],
    );
    const openPositionRangeShapeLabel = useMemo(() => {
        switch (String(openPositionEffectiveRangeEditor?.position_shape || '').trim()) {
            case 'single_token0':
            case 'single_token1':
                return `单边 ${openPositionEffectiveRangeEditor?.dominant_token_symbol || '--'}`;
            case 'dual_sided':
                return '双边';
            default:
                return '';
        }
    }, [openPositionEffectiveRangeEditor]);
    useEffect(() => {
        setOpenPositionInvertPrice(openPositionDefaultInvert);
    }, [openPositionDefaultInvert]);
    const openPositionPriceRange = useMemo(() => {
        const refTick = Number(openPositionLiqProfile?.current_tick);
        const fallbackTick = Number(openPositionEffectiveRangeEditor?.current_tick);
        const baseTick = Number.isFinite(refTick) ? refTick : (Number.isFinite(fallbackTick) ? fallbackTick : null);
        if (baseTick === null) return null;
        const applyDisplay = (value) => (openPositionInvertPrice && value > 0 ? 1 / value : value);
        const fmt = (value) => {
            if (!Number.isFinite(value) || value <= 0) return '--';
            if (value >= 1_000_000) return value.toExponential(3);
            if (value >= 1) return value.toLocaleString(undefined, { maximumFractionDigits: 4 });
            if (value >= 0.0001) return value.toLocaleString(undefined, { maximumFractionDigits: 6 });
            return value.toExponential(3);
        };
        const resolvedLowerTick = (() => {
            if (openPositionRangeInputMode !== 'percentage') {
                return Number.isInteger(openPositionSelectedManualTickLower)
                    ? openPositionSelectedManualTickLower
                    : (openPositionDefaultFocusedRange?.lowerTick ?? null);
            }
            const lowerPct = Number(openPositionRangeLower);
            if (!Number.isFinite(lowerPct) || lowerPct <= 0) return openPositionDefaultFocusedRange?.lowerTick ?? null;
            const ratio = 1 - lowerPct / 100;
            if (ratio <= 0) return openPositionDefaultFocusedRange?.lowerTick ?? null;
            return Math.round(baseTick + Math.log(ratio) / Math.log(1.0001));
        })();
        const resolvedUpperTick = (() => {
            if (openPositionRangeInputMode !== 'percentage') {
                return Number.isInteger(openPositionSelectedManualTickUpper)
                    ? openPositionSelectedManualTickUpper
                    : (openPositionDefaultFocusedRange?.upperTick ?? null);
            }
            const upperPct = Number(openPositionRangeUpper);
            if (!Number.isFinite(upperPct) || upperPct <= 0) return openPositionDefaultFocusedRange?.upperTick ?? null;
            const ratio = 1 + upperPct / 100;
            return Math.round(baseTick + Math.log(ratio) / Math.log(1.0001));
        })();
        const currentPoolPrice = tickToPoolPrice(baseTick, openPositionToken0Decimals, openPositionToken1Decimals);
        const lowerPoolPrice = Number.isInteger(resolvedLowerTick)
            ? tickToPoolPrice(resolvedLowerTick, openPositionToken0Decimals, openPositionToken1Decimals)
            : null;
        const upperPoolPrice = Number.isInteger(resolvedUpperTick)
            ? tickToPoolPrice(resolvedUpperTick, openPositionToken0Decimals, openPositionToken1Decimals)
            : null;
        const currentDisplay = applyDisplay(currentPoolPrice);
        const lowerDisplay = lowerPoolPrice ? applyDisplay(lowerPoolPrice) : null;
        const upperDisplay = upperPoolPrice ? applyDisplay(upperPoolPrice) : null;
        const displayMin = lowerDisplay !== null && upperDisplay !== null ? Math.min(lowerDisplay, upperDisplay) : null;
        const displayMax = lowerDisplay !== null && upperDisplay !== null ? Math.max(lowerDisplay, upperDisplay) : null;
        const gridStepPct = estimateDisplayGridStepPercent(
            baseTick,
            Number(openPositionEffectiveRangeEditor?.tick_spacing),
            openPositionInvertPrice,
            openPositionToken0Decimals,
            openPositionToken1Decimals,
        );
        const toPct = (value) => {
            if (!Number.isFinite(currentDisplay) || currentDisplay <= 0 || !Number.isFinite(value) || value <= 0) return null;
            return ((value / currentDisplay) - 1) * 100;
        };
        return {
            currentText: fmt(currentDisplay),
            lowerText: displayMin !== null ? fmt(displayMin) : '--',
            upperText: displayMax !== null ? fmt(displayMax) : '--',
            lowerPctText: displayMin !== null ? formatSignedPercentCompact(toPct(displayMin)) : '--',
            upperPctText: displayMax !== null ? formatSignedPercentCompact(toPct(displayMax)) : '--',
            baseSymbol: openPositionInvertPrice ? openPositionToken1Symbol : openPositionToken0Symbol,
            quoteSymbol: openPositionInvertPrice ? openPositionToken0Symbol : openPositionToken1Symbol,
            gridStepPctText: Number.isFinite(gridStepPct) ? formatRangePercentCompact(gridStepPct) : '--',
            tickSpacing: Number(openPositionEffectiveRangeEditor?.tick_spacing),
        };
    }, [
        openPositionLiqProfile,
        openPositionEffectiveRangeEditor,
        openPositionRangeInputMode,
        openPositionRangeLower,
        openPositionRangeUpper,
        openPositionSelectedManualTickLower,
        openPositionSelectedManualTickUpper,
        openPositionDefaultFocusedRange,
        openPositionInvertPrice,
        openPositionToken0Decimals,
        openPositionToken1Decimals,
        openPositionToken0Symbol,
        openPositionToken1Symbol,
    ]);
    const openPositionDCASum = useMemo(
        () => openPositionDCAPercentages.reduce((acc, v) => acc + (Number(v) || 0), 0),
        [openPositionDCAPercentages],
    );
    const openPositionDCASumValid = Math.abs(openPositionDCASum - 100) < 0.01;
    const openPositionDCASummaryItems = useMemo(
        () => buildDCASummaryItems(openPositionAmount, openPositionDCAPercentages),
        [openPositionAmount, openPositionDCAPercentages],
    );
    const openPositionAmountValue = Number(String(openPositionAmount || '').trim());
    const openPositionGlobalDCAMinSplitAmount = useMemo(() => {
        const n = Number(globalConfig?.dca_min_split_amount_usdt);
        return Number.isFinite(n) && n > 0 ? n : 0;
    }, [globalConfig?.dca_min_split_amount_usdt]);
    const openPositionDCAAmountBelowThreshold = openPositionGlobalDCAMinSplitAmount > 0
        && Number.isFinite(openPositionAmountValue)
        && openPositionAmountValue > 0
        && openPositionAmountValue < openPositionGlobalDCAMinSplitAmount;
    const openPositionEffectiveDCAEnabled = openPositionDCAEnabled && !openPositionDCAAmountBelowThreshold;
    const openPositionGlobalSlippageHint = useMemo(() => {
        const n = Number(globalConfig?.slippage_tolerance);
        if (!Number.isFinite(n) || n < 0) return '留空则使用全局配置';
        return `本次开仓采用全局配置滑点: ${formatPercentValue(n)}`;
    }, [globalConfig?.slippage_tolerance]);
    const openPositionSlippagePlaceholder = useMemo(() => {
        const n = Number(globalConfig?.slippage_tolerance);
        return Number.isFinite(n) && n >= 0 ? String(n) : '0.5';
    }, [globalConfig?.slippage_tolerance]);
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

    const userServerPollIntervalSec = Math.max(1, Number(data?.poll_interval_sec || 1));
    const adminServerPollIntervalSec = Math.max(1, Number(adminPositions?.poll_interval_sec || userServerPollIntervalSec));
    const userHasPositions = Array.isArray(data?.positions) && data.positions.length > 0;
    const positionsPollKey = userHasPositions ? POSITIONS_ACTIVE_POLL_KEY : POSITIONS_IDLE_POLL_KEY;
    const pollIntervalSec = getModulePollSec(
        positionsPollKey,
        positionsPollKey === POSITIONS_ACTIVE_POLL_KEY ? userServerPollIntervalSec : getModulePollConfig(POSITIONS_IDLE_POLL_KEY).defaultSec,
        modulePollOverrides,
    );
    const hotPoolsDefaultPollSec = 10;
    const hotPoolsPollIntervalSec = getModulePollSec('hot_pools', hotPoolsDefaultPollSec, modulePollOverrides);
    const assetsPollIntervalSec = getModulePollSec('assets', 60, modulePollOverrides);
    const smartMoneyPollIntervalSec = getModulePollSec('smart_money', 15, modulePollOverrides);
    const adminPagePollIntervalSec = getModulePollSec('admin_page', 15, modulePollOverrides);
    const adminPollIntervalSec = getModulePollSec('admin', adminServerPollIntervalSec, modulePollOverrides);
    const swapPollIntervalSec = getModulePollSec('swap', 8, modulePollOverrides);
    const adminListPollSec = Math.max(3, adminPollIntervalSec);
    const isAdmin = Boolean(me?.is_admin || data?.is_admin || adminPositions?.is_admin);
    const showAdmin = isAdmin && viewMode === 'admin';
    const isHotPools = viewMode === 'hot_pools';
    const isPositions = viewMode === 'positions';
    const isAssets = viewMode === 'assets';
    const isSmartMoney = viewMode === 'smart_money';
    const isAdminPage = isAdmin && viewMode === 'admin_page';
    const isSwap = viewMode === 'swap';
    const topNavItems = useMemo(
        () => buildTopNavItems({ me, isAdmin }),
        [isAdmin, me],
    );
    const hasKlineAccess = hasModuleAccess(me, 'gmgn_kline');
    const hasCreatePoolAccess = hasModuleAccess(me, 'create_pool');
    const hasAssetsAccess = hasModuleAccess(me, 'assets');
    const showWalletSummaryCard = !showAdmin && !isHotPools && !isAssets && !isAdminPage && !isSwap;
    const activePollIntervalSec = showAdmin
        ? adminPollIntervalSec
        : isHotPools
            ? hotPoolsPollIntervalSec
            : isAssets
                ? assetsPollIntervalSec
                : isSmartMoney
                    ? smartMoneyPollIntervalSec
                    : isAdminPage
                        ? adminPagePollIntervalSec
                        : pollIntervalSec;
    const settingsPollIntervalSec = activePollIntervalSec;

    useEffect(() => {
        if (!settingsOpen) return;
        const nextDrafts = {};
        MODULE_POLL_CONFIG.forEach((item) => {
            const moduleDefaultSec = item.key === POSITIONS_ACTIVE_POLL_KEY
                ? userServerPollIntervalSec
                : item.key === 'admin'
                    ? adminServerPollIntervalSec
                    : item.defaultSec;
            nextDrafts[item.key] = String(getModulePollSec(item.key, moduleDefaultSec, modulePollOverrides));
        });
        setModulePollDrafts(nextDrafts);
    }, [adminServerPollIntervalSec, modulePollOverrides, settingsOpen, userServerPollIntervalSec]);

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
    const positions = useMemo(() => {
        const rows = Array.isArray(activeData?.positions) ? activeData.positions : [];
        return [...rows].sort(comparePositionsByCreatedAt);
    }, [activeData]);
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
    const addLiqPositionSlippage = resolvePositionSlippage(addLiqPosition);
    const addLiqParsedSlippage = parseOptionalPercent(addLiqSlippage);
    const addLiqPresetOptions = useMemo(
        () => buildAddLiquidityPresetOptions(addLiqReferenceAmount),
        [addLiqReferenceAmount]
    );
    const addLiqHintText = Number.isFinite(addLiqParsedAmount) && addLiqParsedAmount > 0 && addLiqReferenceAmount > 0
        ? `约为参考仓位的 ${formatRatioCompact((addLiqParsedAmount / addLiqReferenceAmount) * 100)}`
        : '输入 USDT 金额后会显示与当前仓位的大致比例。';

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
        // Multi-wallet: sum all wallets' stable balance + positions + fees'
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
                detail: walletAddress ? `${walletAddress.slice(0, 6)}...${walletAddress.slice(-4)}` : '未连接钱包',
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
        const hasRiskFilter = normalizeHotPoolsRiskFilter(hotPoolsFilter.riskFilter) !== HOT_POOLS_RISK_FILTER_ALL;
        const hasNumbers = [hotPoolsFilter.minFees, hotPoolsFilter.minFeeRate, hotPoolsFilter.minActiveFeeRate, hotPoolsFilter.minTvl, hotPoolsFilter.minVolume, hotPoolsFilter.minTxCount].some((v) => Number.isFinite(v));
        return hasKeyword || hasRiskFilter || hasNumbers;
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
            const riskFilter = normalizeHotPoolsRiskFilter(hotPoolsFilter.riskFilter);
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
                if (!hotPoolMatchesRiskFilter(row, riskFilter)) return false;
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

    const refreshRealtimePositionsNow = useCallback(async ({ signal, setBusy = false, updateError = false } = {}) => {
        if (!hasInitData) return null;
        if (setBusy) setLoading(true);
        if (updateError) setError('');
        try {
            const resp = await fetchRealtimePositions({ apiBaseUrl, initData, signal });
            if (signal?.aborted) return null;
            setData(resp);
            return resp;
        } catch (e) {
            if (signal?.aborted) return null;
            if (updateError) setError(String(e?.message || e));
            throw e;
        } finally {
            if (setBusy && !signal?.aborted) setLoading(false);
        }
    }, [apiBaseUrl, initData, hasInitData]);

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
        const charCount = [...text].length;
        const duration = Math.min(8000, Math.max(3500, 3500 + Math.floor(charCount / 10) * 600));
        noticeTimerRef.current = setTimeout(() => setNotice(null), duration);
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
        if (!me || topNavItems.some((item) => item.key === viewMode)) return;
        setViewMode(topNavItems[0]?.key || 'hot_pools');
    }, [me, topNavItems, viewMode]);

    useEffect(() => {
        return () => {
            if (noticeTimerRef.current) clearTimeout(noticeTimerRef.current);
        };
    }, []);

    useEffect(() => {
        const updateProgress = () => {
            const elapsed = Date.now() - lastPollTimeRef.current;
            const progress = Math.min(100, (elapsed / (activePollIntervalSec * 1000)) * 100);
            setPollProgress(progress);
        };

        updateProgress();

        pollProgressRef.current = setInterval(updateProgress, 100);

        return () => {
            if (pollProgressRef.current) clearInterval(pollProgressRef.current);
        };
    }, [activePollIntervalSec]);

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
        if (!hotPoolsFilterOpen) return;
        setHotPoolsFilterDraft({
            enabled: hotPoolsFilter.enabled,
            keyword: String(hotPoolsFilter.keyword || ''),
            riskFilter: normalizeHotPoolsRiskFilter(hotPoolsFilter.riskFilter),
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
        const controller = new AbortController();
        let inFlight = false;

        const run = async () => {
            if (inFlight) return;
            inFlight = true;
            try {
                await refreshRealtimePositionsNow({ signal: controller.signal, setBusy: true, updateError: true });
            } catch (e) {
                if (!controller.signal.aborted) {
                    console.error('[RealtimePositions] Load failed:', e);
                }
            } finally {
                inFlight = false;
            }
        };

        run();

        if (pollRef.current) clearInterval(pollRef.current);
        pollRef.current = setInterval(run, pollIntervalSec * 1000);

        return () => {
            controller.abort();
            if (pollRef.current) clearInterval(pollRef.current);
        };
    }, [hasInitData, pollIntervalSec, refreshRealtimePositionsNow]);

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
        adminPositionsPollRef.current = setInterval(run, adminPollIntervalSec * 1000);

        return () => {
            aborted = true;
            controller.abort();
            if (adminPositionsPollRef.current) clearInterval(adminPositionsPollRef.current);
        };
    }, [adminPollIntervalSec, apiBaseUrl, initData, hasInitData, showAdmin, adminSelectedUserId]);

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

    const persistModulePollOverrides = useCallback((next) => {
        storage.set(STORAGE_MODULE_POLL_SECS, JSON.stringify(next));
        storage.remove(STORAGE_POLL_SEC);
    }, []);

    const setModulePollOverride = useCallback((key, value) => {
        const config = getModulePollConfig(key);
        const nextValue = clampModulePollSec(value, config);
        setModulePollOverrides((prev) => {
            const next = { ...prev, [key]: nextValue };
            persistModulePollOverrides(next);
            return next;
        });
        setModulePollDrafts((prev) => ({ ...prev, [key]: String(nextValue) }));
    }, [persistModulePollOverrides]);

    const setModulePollDraft = useCallback((key, value) => {
        setModulePollDrafts((prev) => ({ ...prev, [key]: value }));
    }, []);

    const commitModulePollDraft = useCallback((key, effectiveSec) => {
        const raw = String(modulePollDrafts[key] ?? '').trim();
        if (!raw) {
            setModulePollDrafts((prev) => ({ ...prev, [key]: String(effectiveSec) }));
            return;
        }
        setModulePollOverride(key, raw);
    }, [modulePollDrafts, setModulePollOverride]);

    const clearModulePollOverride = useCallback((key) => {
        setModulePollOverrides((prev) => {
            const next = { ...prev };
            delete next[key];
            persistModulePollOverrides(next);
            return next;
        });
        setModulePollDrafts((prev) => {
            const next = { ...prev };
            delete next[key];
            return next;
        });
    }, [persistModulePollOverrides]);

    const clearAllModulePollOverrides = useCallback(() => {
        setModulePollOverrides({});
        setModulePollDrafts({});
        persistModulePollOverrides({});
    }, [persistModulePollOverrides]);

    const applyHotPoolsFilter = () => {
        const keyword = String(hotPoolsFilterDraft.keyword || '').trim();
        const next = normalizeHotPoolsFilter({
            enabled: hotPoolsFilterDraft.enabled,
            keyword,
            riskFilter: hotPoolsFilterDraft.riskFilter,
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
            riskFilter: HOT_POOLS_RISK_FILTER_ALL,
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
            setPoolSearchError('请输入池子地址、交易对或代币符号。');
            setPoolSearchResults([]);
            setPoolSearchPerformed(false);
            return;
        }
        if (!hasInitData) {
            setPoolSearchError('缺少 Telegram initData，无法搜索池子。开发环境可在 backend/.env 开启 TELEGRAM_WEBAPP_ALLOW_EMPTY_INITDATA=1。');
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

    const defaultQuickRangeOptions = useMemo(() => ([
        { key: '1', label: '1%', lowerValue: '1', upperValue: '1', subLabel: '快捷范围' },
        { key: '3', label: '3%', lowerValue: '3', upperValue: '3', subLabel: '快捷范围' },
        { key: '5', label: '5%', lowerValue: '5', upperValue: '5', subLabel: '快捷范围' },
        { key: '10', label: '10%', lowerValue: '10', upperValue: '10', subLabel: '快捷范围' },
        { key: '20', label: '20%', lowerValue: '20', upperValue: '20', subLabel: '快捷范围' },
    ]), []);
    const smartQuickRangeOptions = useMemo(() => (
        Array.isArray(openPositionSmartRanges)
            ? openPositionSmartRanges
                .filter((item) => Number(item?.range_percent) > 0)
                .slice(0, 5)
                .map((item, index) => {
                    const rangePercent = Number(item?.range_percent);
                    return {
                        key: `smart-${rangePercent}-${index}`,
                        label: formatRangePercentCompact(rangePercent),
                        subLabel: formatUsdCompact(item?.total_amount_usd),
                        lowerValue: String(rangePercent),
                        upperValue: String(rangePercent),
                        smart: true,
                    };
                })
            : []
    ), [openPositionSmartRanges]);
    const openPositionHasSmartQuickRanges = smartQuickRangeOptions.length > 0;
    const openPositionQuickRangeOptions = useMemo(
        () => (openPositionHasSmartQuickRanges ? smartQuickRangeOptions : defaultQuickRangeOptions),
        [openPositionHasSmartQuickRanges, smartQuickRangeOptions, defaultQuickRangeOptions],
    );
    const openPositionQuickRangeIntro = openPositionHasSmartQuickRanges
        ? '优先展示聪明钱常用区间；点一下即可套用，也可以继续手动微调。'
        : '可直接选 1 / 3 / 5 / 10 / 20 的默认区间，后续仍可切到 Tick/价格模式细调。';
    const openPositionVisibleRangeMode = openPositionRangeInputMode === 'percentage' ? 'percentage' : 'grid';
    const openPositionShowLiquidityChart = openPositionVisibleRangeMode === 'grid';
    const openPositionOutOfRangeActions = useMemo(
        () => getTaskModeActionSummary(openPositionTaskMode),
        [openPositionTaskMode],
    );
    const openPositionTaskSlippage = parseOptionalPercent(openPositionSlippage);
    const openPositionNeedsHighSlippageConfirm = openPositionTaskSlippage.valid
        && Number.isFinite(openPositionTaskSlippage.value)
        && openPositionTaskSlippage.value > 1;
    const openPositionDisplayedLowerPct = Number(
        openPositionEffectiveRangeEditor?.range_lower_pct ?? openPositionRangeLower,
    );
    const openPositionDisplayedUpperPct = Number(
        openPositionEffectiveRangeEditor?.range_upper_pct ?? openPositionRangeUpper,
    );

    const suppressOpenPositionPreview = useCallback((delay = 900) => {
        setOpenPositionPreviewSuspended(true);
        setOpenPositionEntrySwapPreviewLoading(false);
        setOpenPositionPreviewPending(false);
        if (openPositionPreviewResumeTimerRef.current) {
            window.clearTimeout(openPositionPreviewResumeTimerRef.current);
        }
        openPositionPreviewResumeTimerRef.current = window.setTimeout(() => {
            setOpenPositionPreviewSuspended(false);
            openPositionPreviewResumeTimerRef.current = null;
        }, delay);
    }, []);

    useEffect(() => () => {
        if (openPositionPreviewResumeTimerRef.current) {
            window.clearTimeout(openPositionPreviewResumeTimerRef.current);
        }
    }, []);

    const applyOpenPositionQuickRange = useCallback((option) => {
        if (!option) return;
        suppressOpenPositionPreview();
        setOpenPositionRangeInputMode('percentage');
        setOpenPositionRangeLower(option.lowerValue);
        setOpenPositionRangeUpper(option.upperValue);
        setOpenPositionRangeUpperAuto(true);
        setOpenPositionError('');
    }, [suppressOpenPositionPreview]);

    const handleOpenPositionRangeLowerChange = useCallback((value) => {
        suppressOpenPositionPreview();
        setOpenPositionRangeInputMode('percentage');
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
    }, [openPositionRangeUpper, openPositionRangeUpperAuto, suppressOpenPositionPreview]);

    const handleOpenPositionRangeUpperChange = useCallback((value) => {
        suppressOpenPositionPreview();
        setOpenPositionRangeInputMode('percentage');
        setOpenPositionRangeUpperAuto(false);
        setOpenPositionRangeUpper(value);
        setOpenPositionError('');
    }, [suppressOpenPositionPreview]);

    const syncOpenPositionTicksFromEditor = useCallback((editor) => {
        const lower = Number(editor?.tick_lower);
        const upper = Number(editor?.tick_upper);
        return applyOpenPositionTickRange(lower, upper, { clear: false });
    }, [applyOpenPositionTickRange]);

    const applyDefaultOpenPositionTickRange = useCallback(() => {
        if (syncOpenPositionTicksFromEditor(openPositionPreviewRangeEditor)) return;
        if (openPositionDefaultFocusedRange && applyOpenPositionTickRange(openPositionDefaultFocusedRange.lowerTick, openPositionDefaultFocusedRange.upperTick, { clear: false })) {
            return;
        }
        const lower = Number(openPositionEffectiveRangeEditor?.anchor_tick_lower);
        const upper = Number(openPositionEffectiveRangeEditor?.anchor_tick_upper);
        applyOpenPositionTickRange(lower, upper, { clear: false });
    }, [openPositionPreviewRangeEditor, openPositionDefaultFocusedRange, openPositionEffectiveRangeEditor, syncOpenPositionTicksFromEditor, applyOpenPositionTickRange]);

    const handleOpenPositionRangeInputModeChange = useCallback((mode) => {
        suppressOpenPositionPreview();
        setOpenPositionRangeInputMode(mode);
        setOpenPositionError('');
        if (mode !== 'percentage') {
            if (!syncOpenPositionTicksFromEditor(openPositionPreviewRangeEditor)) {
                applyDefaultOpenPositionTickRange();
            }
        }
    }, [openPositionPreviewRangeEditor, applyDefaultOpenPositionTickRange, syncOpenPositionTicksFromEditor, suppressOpenPositionPreview]);

    const nudgeOpenPositionTickBoundary = useCallback((target, delta) => {
        const spacing = Number(openPositionEffectiveRangeEditor?.tick_spacing);
        if (!Number.isFinite(spacing) || spacing <= 0) return;
        const minTick = Number(openPositionEffectiveRangeEditor?.min_tick);
        const maxTick = Number(openPositionEffectiveRangeEditor?.max_tick);
        let nextLower = Number.isInteger(openPositionSelectedManualTickLower)
            ? openPositionSelectedManualTickLower
            : Number(openPositionEffectiveRangeEditor?.tick_lower);
        let nextUpper = Number.isInteger(openPositionSelectedManualTickUpper)
            ? openPositionSelectedManualTickUpper
            : Number(openPositionEffectiveRangeEditor?.tick_upper);
        if (!Number.isInteger(nextLower)) nextLower = Number(openPositionEffectiveRangeEditor?.anchor_tick_lower);
        if (!Number.isInteger(nextUpper)) nextUpper = Number(openPositionEffectiveRangeEditor?.anchor_tick_upper);
        if (!Number.isInteger(nextLower) || !Number.isInteger(nextUpper)) return;
        const nextRange = nudgeDisplayPriceBoundary(
            target,
            delta,
            openPositionInvertPrice,
            spacing,
            nextLower,
            nextUpper,
            minTick,
            maxTick,
        );
        if (!nextRange) return;
        suppressOpenPositionPreview();
        setOpenPositionRangeInputMode('tick');
        applyOpenPositionTickRange(nextRange.lowerTick, nextRange.upperTick);
    }, [
        openPositionEffectiveRangeEditor,
        openPositionSelectedManualTickLower,
        openPositionSelectedManualTickUpper,
        openPositionInvertPrice,
        applyOpenPositionTickRange,
        suppressOpenPositionPreview,
    ]);

    const applyOpenPositionGridBin = useCallback((bin) => {
        if (!bin) return;
        const spacing = Number(openPositionEffectiveRangeEditor?.tick_spacing);
        if (!Number.isFinite(spacing) || spacing <= 0) return;
        let nextLower = Number.isInteger(openPositionSelectedManualTickLower)
            ? openPositionSelectedManualTickLower
            : Number(openPositionEffectiveRangeEditor?.anchor_tick_lower);
        let nextUpper = Number.isInteger(openPositionSelectedManualTickUpper)
            ? openPositionSelectedManualTickUpper
            : Number(openPositionEffectiveRangeEditor?.anchor_tick_upper);
        if (openPositionGridBoundaryTarget === 'lower') {
            nextLower = bin.lowerTick;
            if (nextLower >= nextUpper) nextUpper = nextLower + spacing;
        } else {
            nextUpper = bin.upperTick;
            if (nextUpper <= nextLower) nextLower = nextUpper - spacing;
        }
        suppressOpenPositionPreview();
        applyOpenPositionTickRange(nextLower, nextUpper);
    }, [
        openPositionEffectiveRangeEditor,
        openPositionSelectedManualTickLower,
        openPositionSelectedManualTickUpper,
        openPositionGridBoundaryTarget,
        applyOpenPositionTickRange,
        suppressOpenPositionPreview,
    ]);

    const shiftOpenPositionRangeToSingleSide = useCallback((side) => {
        const spacing = Number(openPositionEffectiveRangeEditor?.tick_spacing);
        if (!Number.isFinite(spacing) || spacing <= 0) return;
        const anchorLower = Number(openPositionEffectiveRangeEditor?.anchor_tick_lower);
        const anchorUpper = Number(openPositionEffectiveRangeEditor?.anchor_tick_upper);
        if (!Number.isInteger(anchorLower) || !Number.isInteger(anchorUpper)) return;
        const minTick = Number(openPositionEffectiveRangeEditor?.min_tick);
        const maxTick = Number(openPositionEffectiveRangeEditor?.max_tick);
        const resolvedCurrentLower = Number(openPositionEffectiveRangeEditor?.tick_lower);
        const resolvedCurrentUpper = Number(openPositionEffectiveRangeEditor?.tick_upper);
        const currentLower = Number.isInteger(openPositionSelectedManualTickLower)
            ? openPositionSelectedManualTickLower
            : (Number.isInteger(resolvedCurrentLower) ? resolvedCurrentLower : anchorLower);
        const currentUpper = Number.isInteger(openPositionSelectedManualTickUpper)
            ? openPositionSelectedManualTickUpper
            : (Number.isInteger(resolvedCurrentUpper) ? resolvedCurrentUpper : anchorUpper);
        const width = Math.max(spacing, currentUpper - currentLower);
        let nextLower = currentLower;
        let nextUpper = currentUpper;
        if (side === 'lower') {
            nextUpper = anchorLower;
            nextLower = nextUpper - width;
            if (Number.isFinite(minTick) && nextLower < minTick) {
                nextLower = minTick;
                nextUpper = nextLower + width;
            }
        } else {
            nextLower = anchorUpper;
            nextUpper = nextLower + width;
            if (Number.isFinite(maxTick) && nextUpper > maxTick) {
                nextUpper = maxTick;
                nextLower = nextUpper - width;
            }
        }
        suppressOpenPositionPreview();
        setOpenPositionRangeInputMode('tick');
        applyOpenPositionTickRange(nextLower, nextUpper);
    }, [
        openPositionEffectiveRangeEditor,
        openPositionSelectedManualTickLower,
        openPositionSelectedManualTickUpper,
        applyOpenPositionTickRange,
        suppressOpenPositionPreview,
    ]);

    useEffect(() => {
        if (openPositionRangeInputMode !== 'percentage') return;
        if (openPositionDefaultRangeSeededRef.current) return;
        if (String(openPositionRangeLower || '').trim() || String(openPositionRangeUpper || '').trim()) return;
        if (!openPositionDefaultFocusedPercentageRange) return;
        setOpenPositionRangeLower(openPositionDefaultFocusedPercentageRange.lowerValue);
        setOpenPositionRangeUpper(openPositionDefaultFocusedPercentageRange.upperValue);
        openPositionDefaultRangeSeededRef.current = true;
    }, [
        openPositionRangeInputMode,
        openPositionRangeLower,
        openPositionRangeUpper,
        openPositionDefaultFocusedPercentageRange,
    ]);

    useEffect(() => {
        if (openPositionRangeInputMode === 'percentage') return;
        if (String(openPositionTickLower || '').trim() && String(openPositionTickUpper || '').trim()) return;
        applyDefaultOpenPositionTickRange();
    }, [
        openPositionRangeInputMode,
        openPositionTickLower,
        openPositionTickUpper,
        applyDefaultOpenPositionTickRange,
    ]);

    const openPositionLastInvertRef = useRef(openPositionInvertPrice);
    useEffect(() => {
        if (openPositionLastInvertRef.current === openPositionInvertPrice) return;
        openPositionLastInvertRef.current = openPositionInvertPrice;
        if (openPositionRangeInputMode !== 'price') return;
        if (!Number.isInteger(openPositionSelectedManualTickLower) || !Number.isInteger(openPositionSelectedManualTickUpper) || openPositionSelectedManualTickUpper <= openPositionSelectedManualTickLower) {
            return;
        }
        openPositionSyncPriceInputsFromTicks(openPositionSelectedManualTickLower, openPositionSelectedManualTickUpper);
    }, [
        openPositionInvertPrice,
        openPositionRangeInputMode,
        openPositionSelectedManualTickLower,
        openPositionSelectedManualTickUpper,
        openPositionSyncPriceInputsFromTicks,
    ]);

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

    const openPositionModal = (pool) => {
        let chain = resolveOpenPositionPoolChain(pool, hotPoolsData?.chain || 'bsc');
        if (!multiChainEnabled) chain = userDefaultChain;
        const poolAddress = String(pool?.pool_address || '').trim();
        const poolVersion = normalizeOpenPositionPoolVersion(pool);
        setOpenPositionPool({
            ...pool,
            chain,
            ...(poolVersion ? { protocol_version: poolVersion, pool_version: poolVersion } : {}),
        });
        setOpenPositionSmartRanges(Array.isArray(pool?.range_groups) ? pool.range_groups : []);
        setOpenPositionSmartRangesLoading(Boolean(poolAddress));
        // Seed DCA defaults from the (possibly cached) global config so the user can override per open.
        const cfgDCAEnabled = Boolean(globalConfig?.dca_enabled);
        const cfgDCAIntervalRaw = Number(globalConfig?.dca_interval_seconds);
        const cfgDCAInterval = Number.isFinite(cfgDCAIntervalRaw) && cfgDCAIntervalRaw >= 0 ? cfgDCAIntervalRaw : 30;
        let cfgDCAPcts = [50, 50];
        const rawPcts = globalConfig?.dca_percentages_json;
        if (typeof rawPcts === 'string' && rawPcts.trim()) {
            try {
                const arr = JSON.parse(rawPcts);
                if (Array.isArray(arr) && arr.length >= 2) cfgDCAPcts = arr.map((v) => Number(v) || 0);
            } catch {
                // ignore
            }
        }
        setOpenPositionDCAEnabled(cfgDCAEnabled);
        setOpenPositionDCAPercentages(cfgDCAPcts);
        setOpenPositionDCAInterval(cfgDCAInterval);
        setOpenPositionDCAExpanded(false);
        resetOpenPositionDraft();
    };

    const closeOpenPosition = () => {
        setOpenPositionPool(null);
        setOpenPositionStep(0);
        setOpenPositionSmartRanges([]);
        setOpenPositionSmartRangesLoading(false);
        setOpenPositionDCAExpanded(false);
    };

    useOpenPositionMarketData({
        apiBaseUrl,
        initData,
        hasInitData,
        openPositionPool,
        openPositionShowLiquidityChart,
        setGlobalConfig,
        setOpenPositionSmartRanges,
        setOpenPositionSmartRangesLoading,
        setOpenPositionLiqProfile,
        setOpenPositionLiqProfileLoading,
        setOpenPositionLiqProfileError,
    });
    const openPositionChartLowerTick = useMemo(() => {
        if (openPositionRangeInputMode !== 'percentage') {
            return Number.isInteger(openPositionSelectedManualTickLower)
                ? openPositionSelectedManualTickLower
                : (openPositionDefaultFocusedRange?.lowerTick ?? null);
        }
        const ed = openPositionEffectiveRangeEditor;
        if (!ed || !Number.isFinite(Number(ed.current_tick))) return null;
        const lowerPct = Number(openPositionRangeLower);
        if (!Number.isFinite(lowerPct) || lowerPct <= 0) return openPositionDefaultFocusedRange?.lowerTick ?? null;
        const ratio = 1 - lowerPct / 100;
        if (ratio <= 0) return openPositionDefaultFocusedRange?.lowerTick ?? null;
        return Math.round(Number(ed.current_tick) + Math.log(ratio) / Math.log(1.0001));
    }, [openPositionRangeInputMode, openPositionSelectedManualTickLower, openPositionEffectiveRangeEditor, openPositionRangeLower, openPositionDefaultFocusedRange]);

    const openPositionChartUpperTick = useMemo(() => {
        if (openPositionRangeInputMode !== 'percentage') {
            return Number.isInteger(openPositionSelectedManualTickUpper)
                ? openPositionSelectedManualTickUpper
                : (openPositionDefaultFocusedRange?.upperTick ?? null);
        }
        const ed = openPositionEffectiveRangeEditor;
        if (!ed || !Number.isFinite(Number(ed.current_tick))) return null;
        const upperPct = Number(openPositionRangeUpper);
        if (!Number.isFinite(upperPct) || upperPct <= 0) return openPositionDefaultFocusedRange?.upperTick ?? null;
        const ratio = 1 + upperPct / 100;
        return Math.round(Number(ed.current_tick) + Math.log(ratio) / Math.log(1.0001));
    }, [openPositionRangeInputMode, openPositionSelectedManualTickUpper, openPositionEffectiveRangeEditor, openPositionRangeUpper, openPositionDefaultFocusedRange]);
    const openPositionResolvedSelectionShape = useMemo(() => {
        const currentTick = Number(openPositionLiqProfile?.current_tick ?? openPositionEffectiveRangeEditor?.current_tick);
        const lowerTick = Number(openPositionChartLowerTick);
        const upperTick = Number(openPositionChartUpperTick);
        if (!Number.isFinite(currentTick) || !Number.isFinite(lowerTick) || !Number.isFinite(upperTick) || upperTick <= lowerTick) {
            return { shape: '', dominantTokenSymbol: '' };
        }
        if (currentTick < lowerTick) {
            return { shape: 'single_token0', dominantTokenSymbol: openPositionToken0Symbol };
        }
        if (currentTick >= upperTick) {
            return { shape: 'single_token1', dominantTokenSymbol: openPositionToken1Symbol };
        }
        return { shape: 'dual_sided', dominantTokenSymbol: '' };
    }, [
        openPositionLiqProfile,
        openPositionEffectiveRangeEditor,
        openPositionChartLowerTick,
        openPositionChartUpperTick,
        openPositionToken0Symbol,
        openPositionToken1Symbol,
    ]);
    const openPositionIsSingleSidedSelection = String(openPositionResolvedSelectionShape.shape || '').startsWith('single_');

    useEffect(() => {
        if (!openPositionIsSingleSidedSelection) {
            openPositionAutoSingleSideRangeRef.current = '';
            return;
        }
        const signature = `${openPositionResolvedSelectionShape.shape}:${openPositionChartLowerTick}:${openPositionChartUpperTick}`;
        if (!signature || openPositionAutoSingleSideRangeRef.current === signature) return;
        openPositionAutoSingleSideRangeRef.current = signature;
        if (openPositionDCAEnabled) setOpenPositionDCAEnabled(false);
    }, [
        openPositionIsSingleSidedSelection,
        openPositionResolvedSelectionShape.shape,
        openPositionChartLowerTick,
        openPositionChartUpperTick,
        openPositionDCAEnabled,
    ]);

    const onOpenPositionChartRangeChange = useCallback(({ lower, upper }) => {
        if (!openPositionLiqProfile) return;
        suppressOpenPositionPreview(1100);
        const nextLower = Number.isFinite(lower)
            ? lower
            : (Number.isInteger(openPositionSelectedManualTickLower) ? openPositionSelectedManualTickLower : openPositionChartLowerTick);
        const nextUpper = Number.isFinite(upper)
            ? upper
            : (Number.isInteger(openPositionSelectedManualTickUpper) ? openPositionSelectedManualTickUpper : openPositionChartUpperTick);
        if (!Number.isInteger(nextLower) || !Number.isInteger(nextUpper) || nextUpper <= nextLower) return;
        setOpenPositionRangeInputMode('tick');
        applyOpenPositionTickRange(nextLower, nextUpper);
    }, [
        openPositionLiqProfile,
        openPositionSelectedManualTickLower,
        openPositionSelectedManualTickUpper,
        openPositionChartLowerTick,
        openPositionChartUpperTick,
        applyOpenPositionTickRange,
        suppressOpenPositionPreview,
    ]);

    const handleOpenPositionChartRangeDragStart = useCallback(() => {
        suppressOpenPositionPreview(1200);
        if (!Number.isInteger(openPositionChartLowerTick) || !Number.isInteger(openPositionChartUpperTick) || openPositionChartUpperTick <= openPositionChartLowerTick) return;
        setOpenPositionRangeInputMode('tick');
        applyOpenPositionTickRange(openPositionChartLowerTick, openPositionChartUpperTick, { clear: false });
    }, [openPositionChartLowerTick, openPositionChartUpperTick, applyOpenPositionTickRange, suppressOpenPositionPreview]);

    const handleOpenPositionChartRangeDragEnd = useCallback(() => {
        suppressOpenPositionPreview(850);
    }, [suppressOpenPositionPreview]);

    const onOpenPositionChartBinSelect = useCallback((bin) => {
        if (!bin) return;
        const lower = Number(bin?.tick_lower);
        const upper = Number(bin?.tick_upper);
        if (!Number.isInteger(lower) || !Number.isInteger(upper) || upper <= lower) return;
        suppressOpenPositionPreview();
        setOpenPositionRangeInputMode('tick');
        applyOpenPositionTickRange(lower, upper);
    }, [applyOpenPositionTickRange, suppressOpenPositionPreview]);

    const { handleOpenPosition, openPositionRetryAction } = useOpenPositionFlow({
        apiBaseUrl,
        initData,
        hasInitData,
        multiWalletEnabled,
        walletsData,
        walletsError,
        walletsLoading,
        setWalletsData,
        setWalletsError,
        setWalletsLoading,
        draft: openPositionDraft,
        activeOpenPositionChecks,
        openPositionTickLowerValue,
        openPositionTickUpperValue,
        openPositionSelectedManualTickLower,
        openPositionSelectedManualTickUpper,
        openPositionIsSingleSidedSelection,
        openPositionGlobalDCAMinSplitAmount,
        operationProgress,
        setOperationProgress,
        requestConfirm,
        refreshRealtimePositionsNow,
    });

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
        if (!hasAssetsAccess) return;
        setGlobalConfigOpen(true);
    };

    const handleSetTaskPaused = async (taskId, paused) => {
        if (!hasInitData || showAdmin) return;
        const id = Number(taskId);
        if (!Number.isFinite(id) || id <= 0) return;

        const wantPaused = Boolean(paused);
		const ok = await requestConfirm({
			title: wantPaused ? '暂停任务' : '恢复任务',
			message: wantPaused
				? '确认暂停该任务？\n暂停后不会创建新的订单。'
				: '确认恢复该任务？\n恢复后会继续按策略创建订单。',
			confirmText: wantPaused ? '暂停' : '恢复',
		});
        if (!ok) return;

        try {
            await setTaskPaused({ apiBaseUrl, initData, taskId: id, paused: wantPaused });
			showNotice(wantPaused ? '任务已暂停。' : '任务已恢复。', 'success');
        } catch (e) {
            showNotice(String(e?.message || e), 'error');
        }
    };

    const handleStopTask = async (taskId) => {
        if (!hasInitData || showAdmin) return;
        const id = Number(taskId);
        if (!Number.isFinite(id) || id <= 0) return;

		const ok = await requestConfirm({
			title: '停止仓位',
			message: '确认停止该仓位？\n系统会关闭相关任务，并尽量将剩余价值结算为 USDT。',
			confirmText: '停止',
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
				showNotice(`任务 #${id} 未找到，可能已关闭或删除。`, 'warning');
                try {
                    const resp = await fetchRealtimePositions({ apiBaseUrl, initData });
                    setData(resp);
                } catch {
                    // ignore
                }
                return;
            }
			setOperationProgress(prev => prev?.operation === 'close_position'
				? { ...prev, status: 'error', error: msg || '停止仓位失败。' } : prev);
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

    useEffect(() => {
        if (!operationProgress) return;
        if (operationProgress.operation !== 'open_position') return;
        if (!operationProgress.dca) return;
        if (operationProgress.status !== 'active_dca' && operationProgress.status !== 'active') return;

        const taskId = Number(operationProgress.taskId);
        if (!Number.isFinite(taskId) || taskId <= 0) return;

        const rows = Array.isArray(data?.positions) ? data.positions : [];
        const matched = rows.find((p) => Number(p?.task_id) === taskId);
        const dca = matched?.dca;
        if (!dca?.enabled || !dca?.plan_valid) return;

        const executed = Number(dca.executed_count);
        const total = Number(dca.total_count);
        if (!Number.isFinite(executed) || !Number.isFinite(total) || total <= 1) return;

        const currentStep = Math.max(1, Math.min(total, Math.trunc(executed)));
        const finished = Boolean(dca.finished) || Boolean(dca.completed) || Boolean(dca.canceled);

        setOperationProgress((prev) => {
            if (!prev || prev.operation !== 'open_position' || !prev.dca) return prev;
            if (Number(prev.taskId) !== taskId) return prev;

            const nextStatus = finished ? 'done' : 'active_dca';
            const nextStep = finished && Boolean(dca.completed) ? total : currentStep;
            if (
                prev.status === nextStatus &&
                Number(prev.currentStep) === nextStep &&
                Number(prev.totalSteps) === total
            ) {
                return prev;
            }
            return {
                ...prev,
                status: nextStatus,
                currentStep: nextStep,
                totalSteps: total,
                dcaCompleted: Boolean(dca.completed),
                dcaCanceled: Boolean(dca.canceled),
            };
        });
    }, [data, operationProgress]);

    const handleDeleteTask = async (taskId) => {
        if (!hasInitData || showAdmin) return;
        const id = Number(taskId);
        if (!Number.isFinite(id) || id <= 0) return;

		const ok = await requestConfirm({
			title: '删除任务',
			message: '确认删除该任务？\n此操作不可撤销，并会移除该任务配置。',
			confirmText: '删除',
			tone: 'danger',
		});
        if (!ok) return;

        try {
            const resp = await deleteTask({ apiBaseUrl, initData, taskId: id });
			showNotice(resp?.message || '任务已删除。', 'success');
        } catch (e) {
            showNotice(String(e?.message || e), 'error');
        }
    };

    const handleWithdrawLiquidity = async (taskId) => {
        if (!hasInitData || showAdmin) return;
        const id = Number(taskId);
        if (!Number.isFinite(id) || id <= 0) return;
        const ok = await requestConfirm({
            title: '取回全部流动性？',
            message: '确认要取回全部流动性？\n该操作只会撤出仓位流动性，不会自动兑换为 USDT，并会停止任务。',
            confirmText: '取回',
            tone: 'danger',
        });
        if (!ok) return;
        try {
            const resp = await withdrawLiquidity({ apiBaseUrl, initData, taskId: id });
            showNotice(resp?.message || '取回流动性请求已提交。', 'success');
        } catch (e) {
            showNotice(String(e?.message || e), 'error');
        }
    };

    const handlePartialExit = async (taskId, exitPercent) => {
        if (!hasInitData || showAdmin) return;
        const id = Number(taskId);
        if (!Number.isFinite(id) || id <= 0) return;
        const pct = Number(exitPercent);
        if (!Number.isFinite(pct) || pct <= 0 || pct > 100) {
            showNotice('撤仓比例必须大于 0 且不超过 100。', 'error');
            return;
        }
        const ok = await requestConfirm({
            title: pct >= 100 ? '停止仓位？' : '部分撤仓？',
            message: pct >= 100
                ? '确认停止该仓位？\n系统会关闭相关任务，并尽量将剩余价值结算为 USDT。'
                : `确认撤出当前仓位的 ${pct}% 并兑换为 USDT？\n任务会保留剩余仓位继续运行。`,
            confirmText: pct >= 100 ? '停止' : '撤仓',
            tone: 'danger',
        });
        if (!ok) return;
        try {
            if (pct >= 100) {
                setOperationProgress({ operation: 'close_position', taskId: id, currentStep: 0, totalSteps: 4, status: 'active', error: '' });
                const resp = await stopTask({ apiBaseUrl, initData, taskId: id });
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
                return;
            }
            const resp = await stopTask({ apiBaseUrl, initData, taskId: id, exitPercent: pct });
            showNotice(resp?.message || '部分撤仓请求已提交。', 'success');
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
            showNotice(resp?.message || '碎币兑换请求已提交。', 'success');
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
            showNotice(resp?.message || '再平衡请求已提交。', 'success');
        } catch (e) {
            showNotice(String(e?.message || e), 'error');
        }
    };

    const handleUpdateTaskMode = async (taskId, taskMode) => {
        if (!hasInitData || showAdmin) return;
        const id = Number(taskId);
        if (!Number.isFinite(id) || id <= 0) return;
        try {
            const resp = await updateTaskMode({ apiBaseUrl, initData, taskId: id, taskMode });
            showNotice(resp?.ok ? `Mode: ${getTaskModeMeta(taskMode).label}` : 'Task mode updated.', 'success');
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
            title: String(position?.title || '').trim() || `补充流动性 #${id}`,
            task_slippage_tolerance: resolvePositionSlippage(position),
        });
        setAddLiqAmount('');
        const currentSlippage = resolvePositionSlippage(position);
        setAddLiqSlippage(Number.isFinite(currentSlippage) ? String(currentSlippage) : '');
        setAddLiqError('');
    };

    const closeAddLiqModal = () => {
        if (addLiqLoading) return;
        setAddLiqModal(null);
        setAddLiqAmount('');
        setAddLiqSlippage('');
        setAddLiqError('');
    };

    const submitAddLiquidity = async () => {
        if (!addLiqModal) return;
        const amount = parseAmountInput(addLiqAmount);
        if (!Number.isFinite(amount) || amount <= 0) {
            setAddLiqError('请输入有效的补仓金额。');
            return;
        }
        const slippage = parseOptionalPercent(addLiqSlippage);
        if (!slippage.valid) {
            setAddLiqError('滑点必须在 0 到 100 之间。');
            return;
        }
        setAddLiqLoading(true);
        setAddLiqError('');
        try {
            const resp = await addLiquidity({
                apiBaseUrl,
                initData,
                taskId: addLiqModal.taskId,
                amountUsdt: amount,
                slippageTolerance: slippage.value,
            });
            setAddLiqModal(null);
            setAddLiqAmount('');
            setAddLiqSlippage('');
            showNotice(resp?.message || '补充流动性已提交。', 'success');
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
            title: String(position?.title || '').trim() || `修改任务区间 #${id}`,
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
			setTaskRangeError('金额必须大于 0 USDT。');
            return;
        }
        if (!range || range.lower <= 0 || range.upper <= 0 || range.lower >= 100 || range.upper >= 100) {
            setTaskRangeError('区间百分比必须在 0 到 100 之间。');
            return;
        }

		const ok = await requestConfirm({
			title: '更新任务区间',
			message: '确认更新任务区间？\n确认后机器人会使用新的区间和金额。',
			confirmText: '更新',
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
            showNotice('任务区间已更新', 'success');
            setTaskRangeEdit(null);
            setTaskRangeLower('');
            setTaskRangeUpper('');
            setTaskRangeUpperAuto(true);
            setTaskRangeAmount('');
        } catch (e) {
			setTaskRangeError(String(e?.message || e || '更新失败。'));
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
            `${paused ? '批量暂停' : '批量恢复'}已完成：成功 ${successCount}，失败 ${failCount}`,
            failCount === 0 ? 'success' : 'warning'
        );
    };

    const localUpdateSecAgo = useMemo(() => {
        const elapsed = tick - lastPollTimeRef.current;
        return Math.max(0, Math.floor(elapsed / 1000));
    }, [tick]);

    const moduleMetaByMode = useMemo(() => ({
        hot_pools: {
            title: '热门池',
            icon: icons.fire,
            subtitle: `5 分钟窗口 | ${hotPoolsData ? `${localUpdateSecAgo} 秒前更新` : hotPoolsLoading ? '加载中...' : '等待数据'} | 轮询 ${hotPoolsPollIntervalSec}s`,
        },
        positions: {
            title: '仓位',
            icon: icons.bot,
            subtitle: walletAddress ? `钱包 ${walletAddress.slice(0, 6)}...${walletAddress.slice(-4)}` : '钱包未连接',
        },
        assets: {
            title: '我的',
            icon: icons.wallet,
            subtitle: '我的资产 / 全局配置 / 钱包管理 / 交易记录',
        },
        smart_money: {
            title: '聪明钱',
            icon: icons.eye,
            subtitle: '钱包追踪 / 监控提醒',
        },
        swap: {
            title: '兑换',
            icon: icons.swap,
            subtitle: '一键兑换 · 市价/限价',
        },
        admin_page: {
            title: '管理页',
            icon: icons.gear,
            subtitle: '系统配置 / RPC 池',
        },
        admin: {
            title: '管理',
            icon: icons.gear,
            subtitle: adminSelectedUser
                ? `用户 ${formatUserLabel(adminSelectedUser)}`
                : adminUsersLoading && adminUsers.length === 0
                    ? '加载中...'
                    : adminUsers.length
                        ? `共 ${adminUsers.length} 个用户`
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
            ? '正在加载用户仓位...'
            : '当前用户没有活跃仓位'
        : '请选择用户查看仓位';
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
        <div className={`min-h-screen max-w-[720px] xl:max-w-[960px] 2xl:max-w-[1080px] mx-auto px-4 pt-[max(1rem,env(safe-area-inset-top))] ${(isPositions || isSwap) ? 'pb-[calc(96px+env(safe-area-inset-bottom))]' : 'pb-[calc(80px+env(safe-area-inset-bottom))]'}`}>
            {notice ? (
                <div className="fixed left-1/2 top-[calc(env(safe-area-inset-top)+64px)] z-50 w-[calc(100%-2rem)] max-w-md -translate-x-1/2">
                    <div className={`rounded-xl px-3 py-2 text-sm font-semibold shadow-lg ${noticeClass}`}>
                        {notice.message}
                    </div>
                </div>
            ) : null}
            <div className="progress-bar-container">
                <div
                    className={`progress-bar ${loading || hotPoolsLoading ? 'loading' : ''}`}
                    style={{ width: loading || hotPoolsLoading ? undefined : `${pollProgress}%` }}
                />
            </div>
            <header className={isAssets ? 'mb-1' : 'mb-4'}>
                {!isAssets ? (
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
                ) : null}


                {showAdmin ? (
                    <ModuleHeader
                        title="管理工作台"
                        subtitle={hasAdminPositions
                            ? adminSelectedUser
                                ? `用户 ${formatUserLabel(adminSelectedUser)}`
                                : ''
                            : adminSummaryPlaceholder}
                        actions={hasAdminPositions ? (
                            <div className="text-right">
                                <div className="text-[11px] text-zinc-500 dark:text-white/40">轮询间隔</div>
                                <div className="text-sm font-semibold tabular-nums">
                                    <NumberFlowValue value={adminPollIntervalSec} formatOptions={{ maximumFractionDigits: 0 }} />s
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
                        <Suspense fallback={<div className="rounded-2xl border border-zinc-200/80 bg-white px-4 py-5 text-sm text-zinc-500 dark:border-white/5 dark:bg-[#14171c] dark:text-white/45">正在加载我的资产...</div>}>
                            <LazyAssetManagementPage
                                apiBaseUrl={apiBaseUrl}
                                initData={initData}
                                hasInitData={hasInitData}
                                isAdmin={isAdmin}
                                tick={tick}
                                pollIntervalSec={assetsPollIntervalSec}
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
                            pollIntervalSec={smartMoneyPollIntervalSec}
                            onOpenPosition={openPositionModal}
                            onNotice={showNotice}
                        />
                    </div>
                ) : isAdminPage ? (
                    <div className="mb-2">
                        <Suspense fallback={<div className="rounded-2xl border border-zinc-200/80 bg-white px-4 py-5 text-sm text-zinc-500 dark:border-white/5 dark:bg-[#14171c] dark:text-white/45">正在加载管理页...</div>}>
                            <LazyAdminPage
                                apiBaseUrl={apiBaseUrl}
                                initData={initData}
                                hasInitData={hasInitData}
                                tick={tick}
                                pollIntervalSec={adminPagePollIntervalSec}
                                accentTheme={accentTheme}
                                onNotice={showNotice}
                            />
                        </Suspense>
                    </div>
                ) : isSwap ? (
                    <div className="mb-2">
                        <Suspense fallback={<div className="rounded-2xl border border-zinc-200/80 bg-white px-4 py-5 text-sm text-zinc-500 dark:border-white/5 dark:bg-[#14171c] dark:text-white/45">正在加载兑换模块...</div>}>
                            <LazySwapModule
                                apiBaseUrl={apiBaseUrl}
                                initData={initData}
                                hasInitData={hasInitData}
                                accentTheme={accentTheme}
                                pollIntervalSec={swapPollIntervalSec}
                                multiChainEnabled={multiChainEnabled}
                                onNotice={showNotice}
                            />
                        </Suspense>
                    </div>
                ) : isHotPools ? (
                    <ModuleHeader
                        title={hotPoolsSort === 'fee_rate' ? '费率排序' : hotPoolsSort === 'volume' ? '成交量排序' : '涨跌幅排序'}
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
                                        <span className={`absolute -right-0.5 -top-0.5 h-2 w-2 rounded-full ring-2 ring-white dark:ring-[#14171c] ${brand.dotClass}`} />
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
                                        我的资产
                                    </div>
                                    <div className="mt-2.5 text-[10px] font-medium text-zinc-500 dark:text-white/45">总资产</div>
                                    <div className="mt-1 text-[24px] font-black leading-none tracking-tight text-zinc-950 dark:text-white">
                                        <NumberFlowValue value={totalUsd} formatter={(v) => formatUsd(v)} />
                                    </div>
                                    <div className="mt-2 flex flex-wrap gap-1.5 text-[10px] text-zinc-500 dark:text-white/50">
                                        {!multiWalletSummary ? (
                                            <span className="rounded-full border border-white/70 bg-white/70 px-2 py-1 font-mono dark:border-white/10 dark:bg-white/5">
                                                {walletAddress ? `${walletAddress.slice(0, 6)}...${walletAddress.slice(-4)}` : '未连接钱包'}
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
                                        disabled={!hasInitData || !hasAssetsAccess}
                                        className={`inline-flex shrink-0 rounded-2xl px-3 py-2 text-[10px] font-semibold ring-1 backdrop-blur-md transition-colors ${hasInitData && hasAssetsAccess
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
                        暂无热门池子数据。
                    </div>
                ) : null
            }

            {
                isHotPools && !hotPoolsLoading && !hotPoolsError && hotPoolsData && hotPoolsRows.length > 0 && hotPoolsFilterEnabled && hotPoolsVisibleRows.length === 0 ? (
                    <div className="mb-4 rounded-2xl border border-zinc-200 bg-white/70 p-6 text-sm text-zinc-500 dark:border-white/10 dark:bg-white/5 dark:text-white/60">
                        当前筛选条件下暂无池子。
                    </div>
                ) : null
            }

            {
                !isHotPools && showAdmin ? (
                    <Suspense fallback={<div className="mb-4 rounded-2xl border border-zinc-200/80 bg-white px-4 py-5 text-sm text-zinc-500 dark:border-white/5 dark:bg-[#14171c] dark:text-white/45">正在加载管理模块...</div>}>
                        <LazyAdminPage
                            apiBaseUrl={apiBaseUrl}
                            initData={initData}
                            hasInitData={hasInitData}
                            tick={tick}
                            pollIntervalSec={adminPollIntervalSec}
                            accentTheme={accentTheme}
                            onNotice={showNotice}
                        />
                    </Suspense>
                ) : null
            }

            {
                !isHotPools && initDataMissing ? (
                    <div className="mb-4 rounded-2xl border border-amber-500/30 bg-amber-500/10 p-4 text-sm text-amber-700 dark:text-amber-200">
                        缺少 Telegram initData，请从 Telegram 机器人入口打开页面。
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
                            {batchMode ? '退出批量' : '批量管理'}
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
                                onOpenKline={hasKlineAccess ? setKlinePool : undefined}
                                onOpenPosition={hasCreatePoolAccess ? openPositionModal : undefined}
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
                                        onPartialExit={handlePartialExit}
                                        onDeleteTask={handleDeleteTask}
                                        onUpdateTaskRange={openTaskRangeModal}
                                        onWithdrawLiquidity={handleWithdrawLiquidity}
                                        onSwapDust={handleSwapDust}
                                        onTriggerRebalance={handleTriggerRebalance}
                                        onUpdateTaskMode={handleUpdateTaskMode}
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
                        <div className="font-semibold">风险提示</div>
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
                            aria-label="关闭搜索池"
                        />
                        <div className="absolute inset-x-0 bottom-0 max-h-[85vh] overflow-y-auto rounded-t-2xl border border-zinc-200 bg-white p-4 shadow-2xl dark:border-white/10 dark:bg-[#14171c] dark:shadow-none">
                            <div className="flex items-center justify-between">
                                <div className="inline-flex items-center justify-center rounded-xl border border-zinc-200 bg-zinc-100 p-2 text-zinc-700 dark:border-white/10 dark:bg-white/5 dark:text-white/80">
                                    <Icon path={icons.search} className="h-4 w-4" />
                                </div>
                                <button
                                    type="button"
                                    onClick={closePoolSearch}
                                    className="inline-flex h-9 w-9 items-center justify-center rounded-xl border border-zinc-200 bg-zinc-100 text-zinc-900 hover:bg-zinc-200 active:bg-zinc-200 dark:border-white/10 dark:bg-white/5 dark:text-white/80 dark:hover:bg-white/10 dark:active:bg-white/15"
                                    aria-label="关闭搜索池"
                                >
                                    <Icon path={icons.close} className="h-5 w-5" />
                                </button>
                            </div>

                            <div className="mt-4 space-y-3 pb-20">
                                <div className="rounded-2xl border border-zinc-200 bg-zinc-50/90 p-3 dark:border-white/10 dark:bg-[#0f1116]">
                                    <div className="text-[11px] text-zinc-500 dark:text-white/40">支持池地址、代币符号或关键词搜索（例如 CAKE / USDT）</div>
                                    <div className="mt-2 flex items-center gap-2">
                                        <div className="text-[11px] text-zinc-500 dark:text-white/40">链</div>
                                        <CustomSelect
                                            value={poolSearchChain}
                                            onChange={(value) => {
                                                setPoolSearchChain(value);
                                                setPoolSearchResults([]);
                                                setPoolSearchError('');
                                                setPoolSearchPerformed(false);
                                            }}
                                            options={CHAIN_SELECT_OPTIONS}
                                            disabled={!multiChainEnabled}
                                            className="w-28"
                                        />
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
                                            placeholder="输入 USDT / WBNB / 0x..."
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
                                        支持输入池地址、代币符号或池子关键词，结果默认按 TVL 和活跃度筛前 10 个。
                                    </div>
                                </div>

                                {!hasInitData ? (
                                    <div className="rounded-xl border border-amber-500/30 bg-amber-500/10 p-3 text-xs text-amber-700 dark:text-amber-200">
                                        缺少 Telegram initData，请从 Mini App 内重新打开后再搜索池子。
                                    </div>
                                ) : null}

                                {poolSearchError ? (
                                    <div className="rounded-xl border border-red-500/30 bg-red-500/10 p-3 text-xs text-red-700 dark:text-red-200">
                                        {poolSearchError}
                                    </div>
                                ) : null}

                                {poolSearchPerformed && !poolSearchLoading && !poolSearchError && poolSearchResults.length === 0 ? (
                                    <div className="rounded-xl border border-zinc-200 bg-white/70 p-3 text-xs text-zinc-500 dark:border-white/10 dark:bg-white/5 dark:text-white/60">
                                        没有找到匹配的池子，换个关键词再试。
                                    </div>
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
                                                    onOpenKline={hasKlineAccess ? setKlinePool : undefined}
                                                    onOpenPosition={hasCreatePoolAccess ? selectPoolFromSearch : undefined}
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
                            aria-label="关闭热门池筛选"
                        />
                        <div className="absolute inset-x-0 bottom-0 rounded-t-2xl border border-zinc-200 bg-white p-4 shadow-2xl dark:border-white/10 dark:bg-[#14171c] dark:shadow-none">
                            <div className="flex items-center justify-between">
                                <div className="inline-flex items-center justify-center rounded-xl border border-zinc-200 bg-zinc-100 p-2 text-zinc-700 dark:border-white/10 dark:bg-white/5 dark:text-white/80">
                                    <Icon path={icons.filter} className="h-4 w-4" />
                                </div>
                                <button
                                    type="button"
                                    onClick={() => setHotPoolsFilterOpen(false)}
                                    className="inline-flex h-9 w-9 items-center justify-center rounded-xl border border-zinc-200 bg-zinc-100 text-zinc-900 hover:bg-zinc-200 active:bg-zinc-200 dark:border-white/10 dark:bg-white/5 dark:text-white/80 dark:hover:bg-white/10 dark:active:bg-white/15"
                                    aria-label="关闭热门池筛选"
                                >
                                    <Icon path={icons.close} className="h-5 w-5" />
                                </button>
                            </div>

                            <div className="mt-4 space-y-4 pb-20">
                                <div className="rounded-2xl border border-zinc-200 bg-zinc-50/90 p-3 dark:border-white/10 dark:bg-[#0f1116]">
                                    <div className="flex items-center justify-between gap-3">
                                        <div className="min-w-0">
                                            <div className="text-[11px] font-semibold text-zinc-700 dark:text-white/80">热门池筛选</div>
                                            <div className="mt-1 text-[11px] text-zinc-500 dark:text-white/40">
                                                {hotPoolsFilterDraft.enabled ? '已启用筛选，结果会按当前条件即时过滤。' : '未启用筛选，将展示完整热门池列表。'}
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
                                            {hotPoolsFilterDraft.enabled ? '已开启' : '已关闭'}
                                        </button>
                                    </div>
                                </div>
                                <div className="rounded-2xl border border-zinc-200 bg-zinc-50/90 p-3 dark:border-white/10 dark:bg-[#0f1116]">
                                    <div className="mt-1">
                                        <div className="text-[11px] text-zinc-500 dark:text-white/40">关键词（池名 / 代币 / 地址）</div>
                                        <input
                                            value={hotPoolsFilterDraft.keyword}
                                            onChange={(e) => setHotPoolsFilterDraft((prev) => ({ ...prev, keyword: e.target.value }))}
                                            className={`mt-1 w-full rounded-xl border border-zinc-200 bg-white/70 px-3 py-2 text-sm text-zinc-900 shadow-sm outline-none ring-0 placeholder:text-zinc-400 ${brand.inputFocusClass} dark:border-white/10 dark:bg-white/5 dark:text-white/90 dark:placeholder:text-white/30`}
                                            placeholder="例如 USDT / WBNB / 0x..."
                                        />
                                    </div>
                                    <div className="mt-3">
                                        <div className="text-[11px] text-zinc-500 dark:text-white/40">OKX 低流动性</div>
                                        <CustomSelect
                                            value={hotPoolsFilterDraft.riskFilter}
                                            onChange={(value) => setHotPoolsFilterDraft((prev) => ({ ...prev, riskFilter: value }))}
                                            options={HOT_POOLS_RISK_FILTER_OPTIONS}
                                            className="mt-1"
                                        />
                                    </div>
                                    <div className="mt-3 grid grid-cols-2 gap-3">
                                        <div>
                                            <div className="text-[11px] text-zinc-500 dark:text-white/40">累计费用 &gt;= (USD)</div>
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
                                                placeholder="留空"
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
                                            <div className="text-[11px] text-zinc-500 dark:text-white/40">成交量 &gt;= (USD)</div>
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
                                                placeholder="留空"
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
                                            重置
                                        </button>
                                        <button
                                            type="button"
                                            onClick={clearHotPoolsFilter}
                                            className="inline-flex items-center gap-2 rounded-xl bg-white/70 px-3 py-2 text-xs font-semibold text-zinc-700 ring-1 ring-zinc-200 hover:bg-white dark:bg-white/5 dark:text-white/70 dark:ring-white/10"
                                            aria-label="清空筛选"
                                            title="清空筛选"
                                        >
                                            <Icon path={icons.close} className="h-4 w-4" />
                                            清空
                                        </button>
                                    </div>
                                </div>
                            </div>
                        </div>
                    </div>
                ) : null
            }

            {globalConfigOpen ? (
                <GlobalConfigPage
                    open={globalConfigOpen}
                    onClose={() => setGlobalConfigOpen(false)}
                    apiBaseUrl={apiBaseUrl}
                    initData={initData}
                    accentTheme={accentTheme}
                    onConfigChanged={setGlobalConfig}
                />
            ) : null}

            {
                settingsOpen ? (
                    <div className="fixed inset-0 z-[60]">
                        <button
                            type="button"
                            className="absolute inset-0 cursor-default bg-black/40"
                            onClick={() => setSettingsOpen(false)}
                            aria-label="关闭设置"
                        />
                        <div className="absolute inset-x-0 bottom-0 rounded-t-2xl border border-zinc-200 bg-white p-4 shadow-2xl dark:border-white/10 dark:bg-[#14171c] dark:shadow-none">
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
                                <div className="rounded-2xl border border-zinc-200 bg-zinc-50/90 p-3 dark:border-white/10 dark:bg-[#0f1116]">
                                    <div className="text-xs font-semibold text-zinc-900 dark:text-white/80">主题配色</div>
                                    <div className="mt-0.5 text-[11px] text-zinc-500 dark:text-white/40">切换 Mini App 的强调色，只影响当前设备上的界面展示。</div>
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
                                    <div className="text-xs font-semibold text-zinc-900 dark:text-white/80">刷新频率</div>
                                    <div className="mt-0.5 text-[11px] text-zinc-500 dark:text-white/40">
                                        当前模块 <NumberFlowValue value={settingsPollIntervalSec} formatOptions={{ maximumFractionDigits: 0 }} />s；各模块独立保存到当前设备。
                                    </div>
                                    <div className="mt-1 text-[11px] text-zinc-500 dark:text-white/40">
                                        仓位会按当前是否有仓位自动切换，当前是{userHasPositions ? '有仓位' : '无仓位'}档；无仓位档默认 30 秒。
                                    </div>
                                    <div className="mt-3 space-y-2">
                                        {MODULE_POLL_CONFIG.filter((item) => item.key !== 'admin' || isAdmin).map((item) => {
                                            const moduleDefaultSec = item.key === POSITIONS_ACTIVE_POLL_KEY
                                                ? userServerPollIntervalSec
                                                : item.key === 'admin'
                                                    ? adminServerPollIntervalSec
                                                    : item.defaultSec;
                                            const effectiveSec = getModulePollSec(item.key, moduleDefaultSec, modulePollOverrides);
                                            const overridden = Object.prototype.hasOwnProperty.call(modulePollOverrides, item.key);
                                            const draftValue = Object.prototype.hasOwnProperty.call(modulePollDrafts, item.key)
                                                ? modulePollDrafts[item.key]
                                                : String(effectiveSec);
                                            return (
                                                <label key={item.key} className="flex items-center justify-between gap-3 rounded-xl border border-zinc-200 bg-white/70 px-3 py-2 dark:border-white/10 dark:bg-white/5">
                                                    <span className="min-w-0 text-xs font-semibold text-zinc-700 dark:text-white/75">
                                                        {item.label}
                                                        <span className="ml-1 text-[10px] font-normal text-zinc-400 dark:text-white/35">
                                                            min {item.minSec}s
                                                        </span>
                                                    </span>
                                                    <span className="flex shrink-0 items-center gap-2">
                                                        <input
                                                            type="number"
                                                            min={item.minSec}
                                                            max={MAX_POLL_INTERVAL_SEC}
                                                            value={draftValue}
                                                            onChange={(e) => setModulePollDraft(item.key, e.target.value)}
                                                            onBlur={() => commitModulePollDraft(item.key, effectiveSec)}
                                                            onKeyDown={(e) => {
                                                                if (e.key === 'Enter') {
                                                                    e.preventDefault();
                                                                    commitModulePollDraft(item.key, effectiveSec);
                                                                    e.currentTarget.blur();
                                                                }
                                                            }}
                                                            className={`w-20 rounded-xl border border-zinc-200 bg-white/80 px-2 py-1.5 text-right text-sm font-semibold tabular-nums text-zinc-900 outline-none ${brand.inputFocusClass} dark:border-white/10 dark:bg-black/20 dark:text-white/90`}
                                                        />
                                                        <span className="text-[11px] text-zinc-500 dark:text-white/40">s</span>
                                                        <button
                                                            type="button"
                                                            onClick={() => clearModulePollOverride(item.key)}
                                                            disabled={!overridden}
                                                            className="rounded-lg px-2 py-1 text-[11px] font-semibold text-zinc-500 ring-1 ring-zinc-200 disabled:opacity-35 dark:text-white/45 dark:ring-white/10"
                                                        >
                                                            默认
                                                        </button>
                                                    </span>
                                                </label>
                                            );
                                        })}
                                    </div>
                                    <button
                                        type="button"
                                        onClick={clearAllModulePollOverrides}
                                        className="mt-3 rounded-xl bg-white/70 px-3 py-1.5 text-xs font-semibold text-zinc-700 ring-1 ring-zinc-200 hover:bg-white dark:bg-white/5 dark:text-white/70 dark:ring-white/10"
                                    >
                                        全部恢复默认
                                    </button>
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
                        maxHeightClass="max-h-[92vh]"
                        className="bg-white dark:bg-[#14171c] backdrop-blur-none"
                        headerClassName="px-4 pt-3 pb-2.5"
                        contentClassName="px-4 pb-5"
                        footerClassName="px-4 pt-3 pb-[calc(env(safe-area-inset-bottom)+0.85rem)]"
                        footer={
                            <OpenPositionFooter
                                openPositionError={openPositionError}
                                openPositionStep={openPositionStep}
                                openPositionLoading={openPositionLoading}
                                openPositionStep0Valid={openPositionStep0Valid}
                                openPositionStep1Valid={openPositionStep1Valid}
                                openPositionSubmitDisabled={openPositionSubmitDisabled}
                                brand={brand}
                                onPrevious={() => setOpenPositionStep((s) => Math.max(0, s - 1))}
                                onNext={() => setOpenPositionStep((s) => Math.min(2, s + 1))}
                                onSubmit={handleOpenPosition}
                            />
                        }
                        title={
                            <div className="min-w-0">
                                <div className="truncate text-sm font-semibold text-zinc-900 dark:text-white/90">开仓</div>
                                <div className="mt-0.5 truncate text-[11px] font-medium text-zinc-500 dark:text-white/40">
                                    {openPositionPool?.trading_pair || '--'}
                                </div>
                            </div>
                        }
                    >
                        <div className="pb-2">
                            {/* 步骤指示器 */}
                            <OpenPositionStepIndicator
                                openPositionStep={openPositionStep}
                                brand={brand}
                                onStepClick={setOpenPositionStep}
                            />
                            {/* Step 0 · 资金 */}
                            <OpenPositionFundingStep
                                active={openPositionStep === 0}
                                multiWalletEnabled={multiWalletEnabled}
                                walletsLoading={walletsLoading}
                                walletsData={walletsData}
                                walletsError={walletsError}
                                walletOptions={openPositionWalletOptions}
                                walletBalancesHidden={openPositionWalletBalancesHidden}
                                walletId={openPositionWalletId}
                                privateZapHintVisible={openPositionShowPrivateZapProtectionHint}
                                tokenRisk={openPositionTokenRisk}
                                tokenRiskTone={openPositionTokenRiskTone}
                                tokenRiskSymbol={openPositionTokenRiskSymbol}
                                amount={openPositionAmount}
                                slippage={openPositionSlippage}
                                slippagePlaceholder={openPositionSlippagePlaceholder}
                                globalSlippageHint={openPositionGlobalSlippageHint}
                                needsHighSlippageConfirm={openPositionNeedsHighSlippageConfirm}
                                taskSlippage={openPositionTaskSlippage}
                                recommendedPositions={openPositionRecommendedPositions}
                                brand={brand}
                                onToggleWalletBalancesHidden={() => setOpenPositionWalletBalancesHidden((prev) => !prev)}
                                onSelectWallet={(id) => {
                                    setOpenPositionWalletId(id);
                                    storage.set(STORAGE_OPEN_POSITION_WALLET_ID, id);
                                    setOpenPositionError('');
                                    hapticSelection();
                                }}
                                onAmountChange={(value) => {
                                    setOpenPositionAmount(value);
                                    setOpenPositionError('');
                                }}
                                onSlippageChange={(value) => {
                                    setOpenPositionSlippage(value);
                                    setOpenPositionError('');
                                }}
                                onApplyRecommendedAmount={(value) => {
                                    setOpenPositionAmount(value);
                                    setOpenPositionError('');
                                }}
                            />
                            {/* Step 1 · 区间 */}
                            <div className={`${openPositionStep === 1 ? '' : 'hidden'}`}>
                            <div className="rounded-2xl border border-zinc-200/60 bg-zinc-50/60 p-3 dark:border-white/10 dark:bg-white/5">
                                <div className="flex items-center justify-between gap-3">
                                    <div className="text-xs font-semibold text-zinc-900 dark:text-white/85">区间配置</div>
                                    <div className="flex rounded-lg bg-zinc-200/50 p-0.5 dark:bg-white/10">
                                        {OPEN_POSITION_RANGE_OPTIONS.map((option) => (
                                            <button
                                                key={option.key}
                                                type="button"
                                                onClick={() => handleOpenPositionRangeInputModeChange(option.key)}
                                                className={`px-3 py-1 text-[11px] font-semibold rounded-md transition ${openPositionVisibleRangeMode === option.key
                                                    ? 'bg-white text-zinc-900 shadow-sm dark:bg-[#2a2d36] dark:text-white'
                                                    : 'text-zinc-500 hover:text-zinc-700 dark:text-white/50 dark:hover:text-white/80'
                                                    }`}
                                            >
                                                {option.label}
                                            </button>
                                        ))}
                                    </div>
                                </div>

                                {openPositionVisibleRangeMode === 'percentage' ? (
                                    <div className="mt-3">
                                        <div className="flex space-x-1.5 overflow-x-auto pb-1" style={{ scrollbarWidth: 'none' }}>
                                            {openPositionQuickRangeOptions.map((option) => {
                                                const lowerValue = Number(option.lowerValue);
                                                const upperValue = Number(option.upperValue);
                                                const isActive =
                                                    Number.isFinite(openPositionDisplayedLowerPct) &&
                                                    Number.isFinite(openPositionDisplayedUpperPct) &&
                                                    Math.abs(openPositionDisplayedLowerPct - lowerValue) < 0.05 &&
                                                    Math.abs(openPositionDisplayedUpperPct - upperValue) < 0.05;
                                                return (
                                                    <button
                                                        key={option.key}
                                                        type="button"
                                                        onClick={() => applyOpenPositionQuickRange(option)}
                                                        className={`shrink-0 rounded-full border px-3 py-1 text-[10px] font-semibold transition ${isActive
                                                            ? `${brand.selectionClass} text-zinc-900 dark:text-white`
                                                            : option.smart 
                                                                ? 'border-amber-200/50 bg-amber-50/50 text-amber-700 dark:border-amber-500/20 dark:bg-amber-500/10 dark:text-amber-400' 
                                                                : 'border-zinc-200/50 bg-white/50 text-zinc-600 hover:bg-zinc-100 dark:border-white/10 dark:bg-white/5 dark:text-white/60 dark:hover:bg-white/10'
                                                            }`}
                                                    >
                                                        {option.label}
                                                    </button>
                                                );
                                            })}
                                        </div>
                                        <div className="mt-2 flex items-center gap-2">
                                            <div className="relative flex-1">
                                                <input
                                                    value={openPositionRangeLower}
                                                    onChange={(e) => handleOpenPositionRangeLowerChange(e.target.value)}
                                                    inputMode="decimal"
                                                    className={`w-full rounded-xl border border-zinc-200/50 bg-white/70 py-1.5 pl-3 pr-7 text-sm text-center text-zinc-900 shadow-sm outline-none ring-0 placeholder:text-zinc-400 ${brand.inputFocusClass} dark:border-white/10 dark:bg-white/5 dark:text-white/90`}
                                                    placeholder="下限"
                                                />
                                                <span className="absolute right-3 top-1/2 -translate-y-1/2 text-[10px] font-medium text-zinc-400">%</span>
                                            </div>
                                            <span className="text-zinc-300 dark:text-zinc-700">-</span>
                                            <div className="relative flex-1">
                                                <input
                                                    value={openPositionRangeUpper}
                                                    onChange={(e) => handleOpenPositionRangeUpperChange(e.target.value)}
                                                    inputMode="decimal"
                                                    className={`w-full rounded-xl border border-zinc-200/50 bg-white/70 py-1.5 pl-3 pr-7 text-sm text-center text-zinc-900 shadow-sm outline-none ring-0 placeholder:text-zinc-400 ${brand.inputFocusClass} dark:border-white/10 dark:bg-white/5 dark:text-white/90`}
                                                    placeholder="上限"
                                                />
                                                <span className="absolute right-3 top-1/2 -translate-y-1/2 text-[10px] font-medium text-zinc-400">%</span>
                                            </div>
                                        </div>
                                        {openPositionEffectiveRangeEditor ? (
                                            <div className="mt-2 flex justify-between px-1 text-[10px] text-zinc-500 dark:text-white/50">
                                                <span>现价：{openPositionPriceRange?.currentText || '--'}</span>
                                                <span>目标：{formatPriceValue(openPositionEffectiveRangeEditor?.range_lower_price)} - {formatPriceValue(openPositionEffectiveRangeEditor?.range_upper_price)}</span>
                                            </div>
                                        ) : null}
                                    </div>
                                ) : (
                                    <div className="mt-3 grid gap-3">
                                        <div className="rounded-2xl bg-zinc-50/50 p-2.5 dark:bg-[#0f1116]/50">
                                            <div className="flex items-center justify-between gap-3">
                                                <div>
                                                    <div className="text-[10px] font-semibold uppercase tracking-[0.16em] text-zinc-500 dark:text-white/45">Price Range</div>
                                                    <div className="mt-1 text-sm font-semibold text-zinc-900 dark:text-white/90">
                                                        {openPositionPriceRange?.baseSymbol || '--'}/{openPositionPriceRange?.quoteSymbol || '--'}
                                                    </div>
                                                </div>
                                                <button
                                                    type="button"
                                                    onClick={() => setOpenPositionInvertPrice((prev) => !prev)}
                                                    className="rounded-full border border-zinc-200 bg-white/80 px-2.5 py-1 text-[11px] font-semibold text-zinc-700 transition hover:bg-zinc-100 dark:border-white/10 dark:bg-white/5 dark:text-white/75 dark:hover:bg-white/10"
                                                >
                                                    切换报价方向
                                                </button>
                                            </div>

                                            <div className="mt-2 text-[11px] text-zinc-500 dark:text-white/45">
                                                这里会显示当前价格和流动性格子，适合细调 Tick、拖拽上下边界，或直接点格子选区间。
                                            </div>

                                            {openPositionShowLiquidityChart ? (
                                                <div className="mt-3 rounded-2xl border border-zinc-200 bg-zinc-50/90 p-2 dark:border-white/10 dark:bg-[#0f1116]">
                                                    <div className="mb-1 flex items-center justify-between gap-2 text-[10px]">
                                                        <span className="font-semibold text-zinc-700 dark:text-white/70">流动性分布</span>
                                                        <span className="text-zinc-500 dark:text-white/40">
                                                            {openPositionLiqProfileError
                                                                ? '数据暂不可用'
                                                                : (openPositionLiqProfile
                                                                    ? [
                                                                        openPositionLiqProfile.protocol?.toUpperCase(),
                                                                        openPositionPriceRange?.gridStepPctText && openPositionPriceRange.gridStepPctText !== '--'
                                                                            ? `Tick 网格 ${openPositionPriceRange.gridStepPctText}`
                                                                            : '',
                                                                    ].filter(Boolean).join(' | ')
                                                                    : '')}
                                                        </span>
                                                    </div>
                                                    <LiquidityDistributionChart
                                                        bins={openPositionLiqProfile?.bins || []}
                                                        currentTick={Number(openPositionLiqProfile?.current_tick)}
                                                        tickSpacing={Number(openPositionLiqProfile?.tick_spacing)}
                                                        rangeLowerTick={openPositionChartLowerTick}
                                                        rangeUpperTick={openPositionChartUpperTick}
                                                        onRangeChange={onOpenPositionChartRangeChange}
                                                        onRangeDragStart={handleOpenPositionChartRangeDragStart}
                                                        onRangeDragEnd={handleOpenPositionChartRangeDragEnd}
                                                        onBinSelect={onOpenPositionChartBinSelect}
                                                        loading={openPositionLiqProfileLoading}
                                                        token0Decimals={openPositionToken0Decimals}
                                                        token1Decimals={openPositionToken1Decimals}
                                                        invertPrice={openPositionInvertPrice}
                                                        tokenLeftLabel={openPositionInvertPrice ? openPositionToken1Symbol : openPositionToken0Symbol}
                                                        tokenRightLabel={openPositionInvertPrice ? openPositionToken0Symbol : openPositionToken1Symbol}
                                                        quoteIsToken1={openPositionQuoteIsToken1}
                                                        titleText="流动性分布"
                                                        titlePlacement="left"
                                                        height={148}
                                                    />
                                                    <div className="mt-1 text-[11px] text-zinc-500 dark:text-white/40">
                                                        仅在 Tick/价格 模式下展示，用来辅助拖拽区间和点击流动性格子。
                                                    </div>
                                                </div>
                                            ) : null}

                                            <div className="mt-2.5 grid grid-cols-2 gap-2">
                                                <div className="rounded-xl border border-zinc-200/80 bg-white/90 p-2.5 dark:border-white/10 dark:bg-white/5">
                                                    <div className="text-[11px] text-zinc-500 dark:text-white/45">下边界价格</div>
                                                    <div className="mt-1 flex items-end gap-1.5">
                                                        <div className="text-[15px] font-semibold text-zinc-900 dark:text-white/90">{openPositionPriceRange?.lowerText || '--'}</div>
                                                        <div className="pb-0.5 text-[10px] text-zinc-500 dark:text-white/45">{openPositionPriceRange?.lowerPctText || '--'}</div>
                                                    </div>
                                                    <div className="mt-2 grid grid-cols-2 gap-1.5">
                                                        <button
                                                            type="button"
                                                            onClick={() => nudgeOpenPositionTickBoundary('lower', -1)}
                                                            className="rounded-full border border-zinc-200 bg-white/80 px-0 py-1 text-[11px] font-semibold text-zinc-700 transition hover:bg-zinc-100 dark:border-white/10 dark:bg-white/5 dark:text-white/75 dark:hover:bg-white/10"
                                                        >
                                                            -1 格
                                                        </button>
                                                        <button
                                                            type="button"
                                                            onClick={() => nudgeOpenPositionTickBoundary('lower', 1)}
                                                            className="rounded-full border border-zinc-200 bg-white/80 px-0 py-1 text-[11px] font-semibold text-zinc-700 transition hover:bg-zinc-100 dark:border-white/10 dark:bg-white/5 dark:text-white/75 dark:hover:bg-white/10"
                                                        >
                                                            +1 格
                                                        </button>
                                                    </div>
                                                </div>

                                                <div className="rounded-xl border border-zinc-200/80 bg-white/90 p-2.5 dark:border-white/10 dark:bg-white/5">
                                                    <div className="text-[11px] text-zinc-500 dark:text-white/45">上边界价格</div>
                                                    <div className="mt-1 flex items-end gap-1.5">
                                                        <div className="text-[15px] font-semibold text-zinc-900 dark:text-white/90">{openPositionPriceRange?.upperText || '--'}</div>
                                                        <div className="pb-0.5 text-[10px] text-zinc-500 dark:text-white/45">{openPositionPriceRange?.upperPctText || '--'}</div>
                                                    </div>
                                                    <div className="mt-2 grid grid-cols-2 gap-1.5">
                                                        <button
                                                            type="button"
                                                            onClick={() => nudgeOpenPositionTickBoundary('upper', -1)}
                                                            className="rounded-full border border-zinc-200 bg-white/80 px-0 py-1 text-[11px] font-semibold text-zinc-700 transition hover:bg-zinc-100 dark:border-white/10 dark:bg-white/5 dark:text-white/75 dark:hover:bg-white/10"
                                                        >
                                                            -1 格
                                                        </button>
                                                        <button
                                                            type="button"
                                                            onClick={() => nudgeOpenPositionTickBoundary('upper', 1)}
                                                            className="rounded-full border border-zinc-200 bg-white/80 px-0 py-1 text-[11px] font-semibold text-zinc-700 transition hover:bg-zinc-100 dark:border-white/10 dark:bg-white/5 dark:text-white/75 dark:hover:bg-white/10"
                                                        >
                                                            +1 格
                                                        </button>
                                                    </div>
                                                </div>
                                            </div>

                                            <div className="mt-3 grid gap-2 text-[11px] text-zinc-600 dark:text-white/60">
                                                <div className="flex items-center justify-between gap-3">
                                                    <span>当前价格</span>
                                                    <span className="font-semibold text-zinc-900 dark:text-white/90">{openPositionPriceRange?.currentText || '--'}</span>
                                                </div>
                                            </div>
                                        </div>

                                        <div className={`rounded-2xl border p-3 ${String(openPositionEffectiveRangeEditor?.position_shape || '').startsWith('single_')
                                            ? 'border-emerald-500/25 bg-emerald-500/10'
                                            : 'border-sky-500/20 bg-sky-500/10'
                                            }`}>
                                            <div className="flex flex-wrap items-center justify-between gap-2">
                                                <div className={`inline-flex items-center rounded-full border px-2.5 py-1 text-[10px] font-semibold ${String(openPositionEffectiveRangeEditor?.position_shape || '').startsWith('single_')
                                                    ? 'border-emerald-500/25 bg-emerald-500/10 text-emerald-700 dark:text-emerald-200'
                                                    : 'border-sky-500/20 bg-sky-500/10 text-sky-700 dark:text-sky-200'
                                                    }`}>
                                                    {String(openPositionEffectiveRangeEditor?.position_shape || '').startsWith('single_')
                                                        ? `当前形态：${openPositionRangeShapeLabel || `单边 ${openPositionEffectiveRangeEditor?.dominant_token_symbol || '--'}`}`
                                                        : '当前形态：双边'}
                                                </div>
                                                <div className="flex flex-wrap gap-1.5">
                                                    <button
                                                        type="button"
                                                        onClick={() => shiftOpenPositionRangeToSingleSide('lower')}
                                                        className="rounded-full border border-zinc-200 bg-white/80 px-2.5 py-1 text-[11px] font-semibold text-zinc-700 transition hover:bg-zinc-100 dark:border-white/10 dark:bg-white/5 dark:text-white/75 dark:hover:bg-white/10"
                                                    >
                                                        切到下侧单边
                                                    </button>
                                                    <button
                                                        type="button"
                                                        onClick={() => shiftOpenPositionRangeToSingleSide('upper')}
                                                        className="rounded-full border border-zinc-200 bg-white/80 px-2.5 py-1 text-[11px] font-semibold text-zinc-700 transition hover:bg-zinc-100 dark:border-white/10 dark:bg-white/5 dark:text-white/75 dark:hover:bg-white/10"
                                                    >
                                                        切到上侧单边
                                                    </button>
                                                </div>
                                            </div>
                                            <div className="mt-2 text-[11px] leading-5 text-zinc-600 dark:text-white/60">
                                                {String(openPositionEffectiveRangeEditor?.position_shape || '').startsWith('single_')
                                                    ? `当前金额会先集中换成 ${openPositionEffectiveRangeEditor?.dominant_token_symbol || '--'}，以单边方式等待价格回到区间后再提供流动性。`
                                                    : '当前区间会按双边资金分布开仓，价格落在区间内时会同时持有两侧资产。'}
                                            </div>
                                            <div className="mt-2 text-[10px] text-zinc-500 dark:text-white/45">
                                                提示：切换单边后，系统会保留当前区间设置，只调整资金分布方向。
                                            </div>
                                        </div>
                                    </div>
                                )}

                            </div>

                            </div>{/* /Step1 区间 */}
                            {/* Step 2 · 策略 & 确认 */}
                            <div className={`space-y-3 ${openPositionStep === 2 ? '' : 'hidden'}`}>
                            {/* 订单摘要 */}
                            <OpenPositionOrderSummary
                                pool={openPositionPool}
                                amount={openPositionAmount}
                                priceRange={openPositionPriceRange}
                            />
                            <OpenPositionTaskModePanel
                                taskMode={openPositionTaskMode}
                                outOfRangeActions={openPositionOutOfRangeActions}
                                loading={openPositionLoading}
                                onChangeTaskMode={(value) => {
                                    setOpenPositionTaskMode(value);
                                    setOpenPositionError('');
                                }}
                            />

                            <OpenPositionDCAPanel
                                enabled={openPositionDCAEnabled}
                                expanded={openPositionDCAExpanded}
                                loading={openPositionLoading}
                                singleSided={openPositionIsSingleSidedSelection}
                                summaryItems={openPositionDCASummaryItems}
                                interval={openPositionDCAInterval}
                                brand={brand}
                                onToggleEnabled={() => {
                                    setOpenPositionDCAEnabled(!openPositionDCAEnabled);
                                    setOpenPositionError('');
                                }}
                                onToggleExpanded={() => setOpenPositionDCAExpanded((v) => !v)}
                            >
                                    <div className="mt-3">
                                        {openPositionGlobalDCAMinSplitAmount > 0 ? (
                                            <div className="text-[10px] leading-4 text-zinc-500 dark:text-white/45">
                                                全局最小拆分金额为 {formatUSDTValue(openPositionGlobalDCAMinSplitAmount)} USDT。
                                            </div>
                                        ) : null}
                                        {openPositionDCAEnabled && !openPositionEffectiveDCAEnabled && openPositionDCAAmountBelowThreshold ? (
                                            <div className="mt-2 rounded-xl border border-amber-500/25 bg-amber-500/10 px-2.5 py-2 text-[10px] leading-4 text-amber-700 dark:border-amber-400/25 dark:bg-amber-400/10 dark:text-amber-200">
                                                当前开仓金额低于门槛，本次将单笔执行。
                                            </div>
                                        ) : null}
                                        {openPositionDCAEnabled ? (
                                            <>
                                                <div className="mt-3 text-[11px] font-semibold text-zinc-900 dark:text-white/85">
                                                    分批比例（共 {openPositionDCAPercentages.length} 笔）
                                                </div>
                                                <div className="mt-2 space-y-2">
                                                    {openPositionDCAPercentages.map((value, idx) => (
                                                        <div key={idx} className="flex items-center gap-2">
                                                            <span className="w-8 shrink-0 text-[11px] font-semibold text-zinc-500 dark:text-white/45">
                                                                {idx === 0 ? '首笔' : `第${idx + 1}`}
                                                            </span>
                                                            <div className="relative flex-1">
                                                                <input
                                                                    type="number"
                                                                    step="0.1"
                                                                    min="5"
                                                                    max="100"
                                                                    value={value}
                                                                    onChange={(e) => {
                                                                        const next = openPositionDCAPercentages.slice();
                                                                        next[idx] = Number(e.target.value) || 0;
                                                                        setOpenPositionDCAPercentages(next);
                                                                        setOpenPositionError('');
                                                                    }}
                                                                    inputMode="decimal"
                                                                    disabled={openPositionLoading}
                                                                    className={`w-full rounded-xl border border-zinc-200/50 bg-white/70 py-1.5 pl-3 pr-7 text-sm text-zinc-900 shadow-sm outline-none ring-0 ${brand.inputFocusClass} dark:border-white/10 dark:bg-white/5 dark:text-white/90`}
                                                                />
                                                                <span className="absolute right-3 top-1/2 -translate-y-1/2 text-[10px] text-zinc-500 border-none">%</span>
                                                            </div>
                                                            {openPositionDCAPercentages.length > 2 ? (
                                                                <button
                                                                    type="button"
                                                                    onClick={() => {
                                                                        setOpenPositionDCAPercentages(openPositionDCAPercentages.filter((_, i) => i !== idx));
                                                                        setOpenPositionError('');
                                                                    }}
                                                                    disabled={openPositionLoading}
                                                                    className="rounded-xl px-2 py-1.5 text-[11px] font-semibold text-red-500/80 transition hover:bg-red-50 dark:hover:bg-red-500/10"
                                                                >
                                                                    删除
                                                                </button>
                                                            ) : null}
                                                        </div>
                                                    ))}
                                                </div>
                                                <div className="mt-3 flex items-center justify-between gap-2">
                                                    <div className={`text-[10px] font-semibold ${openPositionDCASumValid ? 'text-emerald-600 dark:text-emerald-300' : 'text-amber-600 dark:text-amber-300'}`}>
                                                        合计：{openPositionDCASum.toFixed(2)}% {openPositionDCASumValid ? '?' : '（需100%）'}
                                                    </div>
                                                    <div className="flex items-center gap-2">
                                                        <button
                                                            type="button"
                                                            onClick={() => {
                                                                const n = openPositionDCAPercentages.length || 2;
                                                                const base = Math.floor((100 / n) * 100) / 100;
                                                                const next = Array(n).fill(base);
                                                                next[n - 1] = Math.round((100 - base * (n - 1)) * 100) / 100;
                                                                setOpenPositionDCAPercentages(next);
                                                                setOpenPositionError('');
                                                            }}
                                                            disabled={openPositionLoading}
                                                            className="rounded-full border border-zinc-200/50 bg-white/70 px-2 py-1 text-[10px] font-semibold text-zinc-600 transition hover:bg-white dark:border-white/10 dark:bg-white/5 dark:text-white/60 dark:hover:bg-white/10"
                                                        >
                                                            均分
                                                        </button>
                                                        <button
                                                            type="button"
                                                            onClick={() => {
                                                                if (openPositionDCAPercentages.length >= 5) return;
                                                                const n = openPositionDCAPercentages.length + 1;
                                                                const base = Math.floor((100 / n) * 100) / 100;
                                                                const next = Array(n).fill(base);
                                                                next[n - 1] = Math.round((100 - base * (n - 1)) * 100) / 100;
                                                                setOpenPositionDCAPercentages(next);
                                                                setOpenPositionError('');
                                                            }}
                                                            disabled={openPositionLoading || openPositionDCAPercentages.length >= 5}
                                                            className="rounded-full border border-zinc-200/50 bg-white/70 px-2 py-1 text-[10px] font-semibold text-zinc-600 transition hover:bg-white disabled:opacity-40 dark:border-white/10 dark:bg-white/5 dark:text-white/60 dark:hover:bg-white/10"
                                                        >
                                                            + 一笔
                                                        </button>
                                                    </div>
                                                </div>
                                                <div className="mt-3 flex items-center justify-between gap-2">
                                                    <span className="text-[11px] font-semibold text-zinc-600 dark:text-white/60">间隔</span>
                                                    <div className="relative flex w-32">
                                                        <input
                                                            type="number"
                                                            step="0.001"
                                                            min="0"
                                                            max="300"
                                                            value={openPositionDCAInterval}
                                                            onChange={(e) => {
                                                                setOpenPositionDCAInterval(Number(e.target.value) || 0);
                                                                setOpenPositionError('');
                                                            }}
                                                            inputMode="decimal"
                                                            disabled={openPositionLoading}
                                                            className={`w-full rounded-xl border border-zinc-200/50 bg-white/70 py-1.5 pl-3 pr-8 text-sm text-zinc-900 shadow-sm outline-none ring-0 ${brand.inputFocusClass} dark:border-white/10 dark:bg-white/5 dark:text-white/90`}
                                                        />
                                                        <span className="absolute right-3 top-1/2 -translate-y-1/2 text-[10px] text-zinc-500">秒</span>
                                                    </div>
                                                </div>
                                            </>
                                        ) : null}
                                    </div>
                            </OpenPositionDCAPanel>




                            <OpenPositionPrecheckPanel
                                loading={openPositionEntrySwapPreviewLoading}
                                displayChecks={openPositionDisplayChecks}
                                error={openPositionEntrySwapPreviewError}
                            />

                            <OpenPositionEntrySwapPreviewPanel
                                loading={openPositionEntrySwapPreviewLoading}
                                preview={openPositionEntrySwapPreview}
                            />

                            {/* footer action rendered above */}
                            </div>{/* /Step2 确认 */}
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
                        <div className="absolute inset-x-0 bottom-0 rounded-t-2xl border border-zinc-200 bg-white p-4 shadow-2xl dark:border-white/10 dark:bg-[#14171c] dark:shadow-none">
                            <div className="flex items-center justify-between gap-2">
                                <div className="min-w-0">
                                    <div className="text-sm font-semibold text-zinc-900 dark:text-white/90">更新区间</div>
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
                                    <div className="text-xs font-semibold text-zinc-900 dark:text-white/80">目标区间 (%)</div>
                                    <div className="mt-2 grid grid-cols-2 gap-2">
                                        <input
                                            value={taskRangeLower}
                                            onChange={(e) => handleTaskRangeLowerChange(e.target.value)}
                                            inputMode="decimal"
                                            className={`w-full rounded-xl border border-zinc-200 bg-white/70 px-3 py-2 text-sm text-zinc-900 shadow-sm outline-none ring-0 placeholder:text-zinc-400 ${brand.inputFocusClass} dark:border-white/10 dark:bg-white/5 dark:text-white/90 dark:placeholder:text-white/30`}
                                            placeholder="下边界 %"
                                        />
                                        <input
                                            value={taskRangeUpper}
                                            onChange={(e) => handleTaskRangeUpperChange(e.target.value)}
                                            inputMode="decimal"
                                            className={`w-full rounded-xl border border-zinc-200 bg-white/70 px-3 py-2 text-sm text-zinc-900 shadow-sm outline-none ring-0 placeholder:text-zinc-400 ${brand.inputFocusClass} dark:border-white/10 dark:bg-white/5 dark:text-white/90 dark:placeholder:text-white/30`}
                                            placeholder="上边界 %"
                                        />
                                    </div>
                                    <div className="mt-2 text-[11px] text-zinc-500 dark:text-white/40">
                                        按当前价格中心调整，输入相对中价的百分比偏移。
                                    </div>
                                </div>

                                <div className="rounded-xl border border-zinc-200 bg-zinc-50 p-3 dark:border-white/10 dark:bg-[#0f1116]">
                                    <div className="text-xs font-semibold text-zinc-900 dark:text-white/80">额外补仓金额 (USDT)</div>
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
                                        如需在调整区间时补一点资金，可填写追加金额；留空则只更新区间不追加。
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
                                    {taskRangeLoading ? '提交中...' : '确认更新区间'}
                                </button>
                            </div>
                        </div>
                    </div>
                ) : null
            }

            {addLiqModal ? (
                <div className="fixed inset-0 z-[60]">
                    <button
                        type="button"
                        className="absolute inset-0 bg-black/40"
                        onClick={closeAddLiqModal}
                        aria-label="关闭补仓弹窗"
                    />
                    <div className="absolute inset-x-0 bottom-0 rounded-t-[28px] border border-zinc-200 bg-white p-4 shadow-2xl dark:border-white/10 dark:bg-[#14171c] dark:shadow-none">
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
                                aria-label="关闭补仓弹窗"
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
                                        <div className="mt-1 text-sm font-semibold text-zinc-950 dark:text-white">输入补充金额</div>
                                        <div className="mt-1 text-[11px] leading-5 text-zinc-500 dark:text-white/45">
                                            {addLiqHintText}
                                        </div>
                                    </div>
                                </div>

                                <div className="mt-4 grid grid-cols-2 gap-2">
                                    <div className="rounded-2xl border border-zinc-200/80 bg-white/80 px-3 py-3 dark:border-white/10 dark:bg-white/5">
                                        <div className="text-[10px] font-semibold uppercase tracking-[0.18em] text-zinc-500 dark:text-white/35">Current Value</div>
                                        <div className="mt-1 text-base font-semibold text-zinc-950 dark:text-white">
                                            {addLiqCurrentValue > 0 ? formatUsdCompact(addLiqCurrentValue) : '$--'}
                                        </div>
                                    </div>
                                    <div className="rounded-2xl border border-zinc-200/80 bg-white/80 px-3 py-3 dark:border-white/10 dark:bg-white/5">
                                        <div className="text-[10px] font-semibold uppercase tracking-[0.18em] text-zinc-500 dark:text-white/35">Reference</div>
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
                                        <div className="text-[11px] font-semibold uppercase tracking-[0.22em] text-zinc-500 dark:text-white/35">Top-up Amount</div>
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

                                <div className={`mt-3 rounded-[20px] border px-3 py-3 ${addLiqParsedSlippage.valid
                                    ? 'border-zinc-200 bg-white/80 dark:border-white/10 dark:bg-white/5'
                                    : 'border-red-500/30 bg-red-500/10'
                                }`}>
                                    <div className="flex items-center justify-between gap-2">
                                        <div className="text-[11px] font-semibold uppercase tracking-[0.18em] text-zinc-500 dark:text-white/35">Slippage</div>
                                        <div className="text-[10px] text-zinc-500 dark:text-white/40">
                                            当前 {Number.isFinite(addLiqPositionSlippage) ? `${addLiqPositionSlippage}%` : '--'}
                                        </div>
                                    </div>
                                    <div className="mt-2 flex items-center gap-2">
                                        <input
                                            value={addLiqSlippage}
                                            onChange={(e) => { setAddLiqSlippage(e.target.value); setAddLiqError(''); }}
                                            inputMode="decimal"
                                            placeholder={Number.isFinite(addLiqPositionSlippage) ? String(addLiqPositionSlippage) : '0.5'}
                                            disabled={addLiqLoading}
                                            className="min-w-0 flex-1 border-0 bg-transparent p-0 text-xl font-semibold text-zinc-950 outline-none placeholder:text-zinc-300 dark:text-white dark:placeholder:text-white/20"
                                        />
                                        <span className="inline-flex items-center rounded-full border border-zinc-200 bg-white px-3 py-1 text-[11px] font-semibold text-zinc-700 shadow-sm dark:border-white/10 dark:bg-white/10 dark:text-white/75">
                                            %
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
                                <div className="mb-2 text-xs font-semibold text-zinc-900 dark:text-white/80">补仓金额 (USDT)</div>
                                <input
                                    value={addLiqAmount}
                                    onChange={(e) => { setAddLiqAmount(e.target.value); setAddLiqError(''); }}
                                    inputMode="decimal"
                                    placeholder="输入 USDT 金额"
                                    disabled={addLiqLoading}
                                    className={`w-full rounded-xl border border-zinc-200 bg-white/70 px-3 py-2 text-sm text-zinc-900 shadow-sm outline-none ring-0 placeholder:text-zinc-400 ${brand.inputFocusClass} dark:border-white/10 dark:bg-white/5 dark:text-white/90 dark:placeholder:text-white/30`}
                                />
                                <div className="mt-2 text-[11px] text-zinc-500 dark:text-white/40">
                                    输入的 USDT 会按当前仓位比例补入对应资产。
                                </div>
                            </div>

                            {addLiqError ? (
                                <div className="rounded-xl border border-red-500/30 bg-red-500/10 p-3 text-xs text-red-700 dark:text-red-200">
                                    {addLiqError}
                                </div>
                            ) : null}

                            <button
                                type="button"
                                onClick={submitAddLiquidity}
                                disabled={addLiqLoading || !(Number.isFinite(addLiqParsedAmount) && addLiqParsedAmount > 0) || !addLiqParsedSlippage.valid}
                                className={`w-full rounded-2xl px-3 py-3 text-sm font-semibold shadow-sm transition ${addLiqLoading || !(Number.isFinite(addLiqParsedAmount) && addLiqParsedAmount > 0) || !addLiqParsedSlippage.valid
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
                                        补仓提交中...
                                    </span>
                                ) : '确认'}
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
                        aria-label="关闭确认弹窗"
                        />
                        <div className="relative w-full max-w-md overflow-hidden rounded-t-2xl sm:rounded-2xl border border-zinc-200 bg-white p-4 shadow-2xl dark:border-white/10 dark:bg-[#14171c]">
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
                                    {confirmState.confirmText || '继续'}
                                </button>
                            </div>
                        </div>
                    </div>
                ) : null
            }

            {/* Bottom Navigation */}
            <div className="fixed bottom-0 left-0 right-0 z-50 pointer-events-none pb-[max(0.75rem,env(safe-area-inset-bottom))] px-4">
                <nav className="pointer-events-auto max-w-[400px] mx-auto flex items-center justify-between rounded-full border border-zinc-200/60 bg-white/95 px-3 py-2.5 shadow-2xl backdrop-blur-xl dark:border-white/10 dark:bg-[#1c2026]/95 dark:shadow-black/70 ring-1 ring-black/5 dark:ring-white/5">
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
                                aria-label={item.label}
                                className={`relative flex min-h-[44px] min-w-[44px] flex-col items-center justify-center rounded-full px-4 py-2.5 transition-all duration-300 ${isActive
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
                    onRetry={openPositionRetryAction}
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
