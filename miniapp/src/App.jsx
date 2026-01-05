import React, { useEffect, useMemo, useRef, useState } from 'react';
import HotPoolCard from './components/HotPoolCard.jsx';
import KlineModal from './components/KlineModal.jsx';
import PositionCard from './components/PositionCard.jsx';
import { disableAdminAutoLP, fetchAdminAutoLPStats, fetchAdminRealtimePositions, fetchAdminRealtimeUsers, fetchHotPools, fetchMe, fetchRealtimePositions, openPosition, setTaskPaused } from './lib/api';
import { getTelegramWebApp } from './lib/telegram';
import { formatRelativeTime, useTick } from './lib/time';

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
const STORAGE_POLL_SEC = 'tglp_poll_interval_sec';
const STORAGE_HOT_POOLS_FILTER = 'tglp_hot_pools_filter_v1';

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

const Icon = ({ path, className = '' }) => (
    <svg viewBox="0 0 24 24" fill="currentColor" stroke="currentColor" strokeWidth="0.5" className={className} aria-hidden="true">
        <path d={path} />
    </svg>
);

const icons = {
    bot: 'M12 2a2 2 0 012 2v1h1a3 3 0 013 3v7a7 7 0 11-14 0V8a3 3 0 013-3h1V4a2 2 0 012-2zm-4 7a1.25 1.25 0 100 2.5A1.25 1.25 0 008 9zm8 0a1.25 1.25 0 100 2.5A1.25 1.25 0 0016 9zm-7.5 6.5h7a3.5 3.5 0 01-7 0z',
    chart: 'M4 19h16v2H2V3h2v16zm4-2H6v-6h2v6zm5 0h-2V7h2v10zm5 0h-2v-4h2v4z',
    filter: 'M4 5h16l-6.5 7.5v5l-3 1.5v-6.5L4 5z',
    moon: 'M12 3a9 9 0 109 9c0-.46-.04-.92-.1-1.36a5.389 5.389 0 01-4.4 2.26 5.403 5.403 0 01-3.14-9.8c-.44-.06-.9-.1-1.36-.1z',
    sun: 'M12 7a5 5 0 100 10 5 5 0 000-10zM2 13h2a1 1 0 100-2H2a1 1 0 100 2zm18 0h2a1 1 0 100-2h-2a1 1 0 100 2zM11 2v2a1 1 0 102 0V2a1 1 0 10-2 0zm0 18v2a1 1 0 102 0v-2a1 1 0 10-2 0zM5.99 4.58a1 1 0 10-1.41 1.41l1.06 1.06a1 1 0 001.41-1.41L5.99 4.58zm12.37 12.37a1 1 0 10-1.41 1.41l1.06 1.06a1 1 0 001.41-1.41l-1.06-1.06zm1.06-10.96a1 1 0 10-1.41-1.41l-1.06 1.06a1 1 0 001.41 1.41l1.06-1.06zM7.05 18.36a1 1 0 10-1.41-1.41l-1.06 1.06a1 1 0 001.41 1.41l1.06-1.06z',
    close: 'M6.225 4.811a1 1 0 011.414 0L12 9.172l4.361-4.361a1 1 0 111.414 1.414L13.414 10.586l4.361 4.361a1 1 0 01-1.414 1.414L12 12l-4.361 4.361a1 1 0 01-1.414-1.414l4.361-4.361-4.361-4.361a1 1 0 010-1.414z',
    check: 'M9 16.17L4.83 12 3.41 13.41 9 19l12-12-1.41-1.41L9 16.17z',
    reset: 'M12 5V2L7 7l5 5V9a5 5 0 11-5 5H5a7 7 0 107-7z',
    ban: 'M12 2a10 10 0 100 20 10 10 0 000-20zm6 10a5.96 5.96 0 01-1.26 3.67L8.33 7.26A5.96 5.96 0 0112 6a6 6 0 016 6zm-12 0a5.96 5.96 0 011.26-3.67l8.41 8.41A5.96 5.96 0 0112 18a6 6 0 01-6-6z',
};

export default function App() {
    const initData = useInitData();
    const tick = useTick(); // 实时时钟，每秒更新一次
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
        minFees: String(defaultHotPoolsFilter.minFees),
        minFeeRate: String(defaultHotPoolsFilter.minFeeRate),
        minTvl: String(defaultHotPoolsFilter.minTvl),
        minVolume: String(defaultHotPoolsFilter.minVolume),
    }));
    // 保存上一次热门池子数据，用于计算变化
    const previousHotPoolsDataRef = useRef({});
    const [klinePool, setKlinePool] = useState(null);
    const [openPositionPool, setOpenPositionPool] = useState(null);
    const [openPositionAmount, setOpenPositionAmount] = useState('');
    const [openPositionRange, setOpenPositionRange] = useState('');
    const [openPositionAllowSwap, setOpenPositionAllowSwap] = useState(false);
    const [openPositionError, setOpenPositionError] = useState('');
    const [openPositionLoading, setOpenPositionLoading] = useState(false);
    const [openPositionSuccess, setOpenPositionSuccess] = useState('');

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

    const [theme, setTheme] = useState('dark');
    const [settingsOpen, setSettingsOpen] = useState(false);
    const [pollOverrideSec, setPollOverrideSec] = useState(null);
    const [pollDraftSec, setPollDraftSec] = useState('');

    const serverPollIntervalSec = Math.max(1, Number(data?.poll_interval_sec || adminPositions?.poll_interval_sec || 1));
    const pollIntervalSec = Math.max(1, Number(pollOverrideSec || serverPollIntervalSec || 1));
    const adminListPollSec = Math.max(3, pollIntervalSec);
    const adminStatsPollSec = Math.max(5, pollIntervalSec * 2);
    const isAdmin = Boolean(me?.is_admin || data?.is_admin || adminPositions?.is_admin);
    const showAdmin = isAdmin && viewMode === 'admin';
    const isHotPools = viewMode === 'hot_pools';
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
        const server = typeof summary?.total_usd === 'number' ? summary.total_usd : null;
        const walletUsd = walletUsdFromTokens + (typeof bnbUsd === 'number' ? bnbUsd : 0);
        const computed = walletUsd + totalsFromPositions.positionUsd + totalsFromPositions.feeUsd;
        if (server !== null && server > 0) return server;
        if (computed > 0) return computed;
        return server ?? computed;
    }, [summary?.total_usd, walletUsdFromTokens, bnbUsd, totalsFromPositions.positionUsd, totalsFromPositions.feeUsd]);

    const visiblePositions = useMemo(() => {
        return positions.filter((p) => p?.has_liquidity !== false);
    }, [positions]);

    const hotPoolsRows = useMemo(() => {
        return Array.isArray(hotPoolsData?.data) ? hotPoolsData.data : [];
    }, [hotPoolsData]);

    const hotPoolsFilterEnabled = useMemo(() => {
        if (!hotPoolsFilter.enabled) return false;
        return [hotPoolsFilter.minFees, hotPoolsFilter.minFeeRate, hotPoolsFilter.minTvl, hotPoolsFilter.minVolume].some((v) => Number.isFinite(v));
    }, [hotPoolsFilter]);

    const hotPoolsVisibleRows = useMemo(() => {
        if (!hotPoolsFilterEnabled) return hotPoolsRows;
        const minFees = hotPoolsFilter.minFees;
        const minFeeRate = hotPoolsFilter.minFeeRate;
        const minTvl = hotPoolsFilter.minTvl;
        const minVolume = hotPoolsFilter.minVolume;
        return hotPoolsRows.filter((row) => {
            const fees = parseMetricNumber(row?.total_fees);
            const feeRate = parseMetricNumber(row?.fee_rate);
            const tvl = parseMetricNumber(row?.current_pool_value);
            const volume = parseMetricNumber(row?.total_volume);
            if (Number.isFinite(minFees) && fees < minFees) return false;
            if (Number.isFinite(minFeeRate) && feeRate < minFeeRate) return false;
            if (Number.isFinite(minTvl) && tvl < minTvl) return false;
            if (Number.isFinite(minVolume) && volume < minVolume) return false;
            return true;
        });
    }, [hotPoolsFilter, hotPoolsFilterEnabled, hotPoolsRows]);

    const hotPoolsFilterLabel = useMemo(() => {
        if (!hotPoolsFilterEnabled) return '筛选关闭';
        return `筛选 ${hotPoolsVisibleRows.length}/${hotPoolsRows.length}`;
    }, [hotPoolsFilterEnabled, hotPoolsRows.length, hotPoolsVisibleRows.length]);

    // 构建热门池子的历史数据映射 (protocol_version:pool_address -> previous data)
    const previousHotPoolsMap = useMemo(() => {
        return previousHotPoolsDataRef.current;
    }, [hotPoolsRows]);

    const apiBaseUrl = useMemo(() => resolveApiBaseUrl(), []);

    useEffect(() => {
        if (!initData) return;
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
    }, [apiBaseUrl, initData]);

    useEffect(() => {
        if (!isAdmin && viewMode === 'admin') {
            setViewMode('positions');
        }
    }, [isAdmin, viewMode]);

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
    }, []);

    useEffect(() => {
        const isDark = theme === 'dark';
        document.documentElement.classList.toggle('dark', isDark);
        storage.set(STORAGE_THEME, isDark ? 'dark' : 'light');

        const tg = getTelegramWebApp();
        try {
            tg?.setHeaderColor?.(isDark ? '#0b0f14' : '#fafafa');
            tg?.setBackgroundColor?.(isDark ? '#0b0f14' : '#fafafa');
        } catch {
            // ignore
        }
    }, [theme]);

    useEffect(() => {
        if (!settingsOpen) return;
        setPollDraftSec(pollOverrideSec ? String(pollOverrideSec) : '');
    }, [settingsOpen, pollOverrideSec]);

    useEffect(() => {
        if (!hotPoolsFilterOpen) return;
        setHotPoolsFilterDraft({
            enabled: hotPoolsFilter.enabled,
            minFees: formatDraftNumber(hotPoolsFilter.minFees),
            minFeeRate: formatDraftNumber(hotPoolsFilter.minFeeRate),
            minTvl: formatDraftNumber(hotPoolsFilter.minTvl),
            minVolume: formatDraftNumber(hotPoolsFilter.minVolume),
        });
    }, [hotPoolsFilterOpen, hotPoolsFilter]);

    useEffect(() => {
        if (!initData) return;
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
    }, [apiBaseUrl, initData, pollIntervalSec]);

    useEffect(() => {
        if (!initData || !showAdmin) return;
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
    }, [apiBaseUrl, initData, showAdmin, adminListPollSec]);

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
        if (!initData || !showAdmin || !adminSelectedUserId) return;
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
    }, [apiBaseUrl, initData, showAdmin, adminSelectedUserId, pollIntervalSec]);

    useEffect(() => {
        if (!initData || !showAdmin || !adminSelectedUserId) return;
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
    }, [apiBaseUrl, initData, showAdmin, adminSelectedUserId, adminStatsPollSec]);

    // 热门池子数据始终加载（预加载）
    useEffect(() => {
        let aborted = false;
        const controller = new AbortController();
        let inFlight = false;

        const run = async () => {
            if (inFlight) return;
            inFlight = true;
            setHotPoolsLoading(true);
            setHotPoolsError('');
            try {
                const resp = await fetchHotPools({
                    apiBaseUrl,
                    sort: hotPoolsSort,
                    chain: 'bsc',
                    timeframeMinutes: 5,
                    limit: 20,
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
    }, [apiBaseUrl, isHotPools, hotPoolsSort, hotPoolsPollIntervalSec]);

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
        const next = normalizeHotPoolsFilter({
            enabled: hotPoolsFilterDraft.enabled,
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

    const disableHotPoolsFilter = () => {
        const next = { ...hotPoolsFilter, enabled: false };
        setHotPoolsFilter(next);
        storage.set(STORAGE_HOT_POOLS_FILTER, JSON.stringify(next));
        setHotPoolsFilterOpen(false);
    };

    const toggleTheme = () => setTheme((t) => (t === 'dark' ? 'light' : 'dark'));

    const quickRangeOptions = [
        { label: '±3%', value: '±3' },
        { label: '±5%', value: '±5' },
        { label: '±8%', value: '±8' },
        { label: '±10%', value: '±10' },
        { label: '1% / 3%', value: '1 3' },
    ];

    const parseRangeInput = (raw) => {
        const text = String(raw || '').trim();
        if (!text) return null;
        const hasSym = /(\+\/?-|±|正负)/.test(text);
        const matches = text.match(/-?\d+(?:\.\d+)?/g) || [];
        const numbers = matches.map((v) => Math.abs(Number(v))).filter((v) => Number.isFinite(v));
        if (!numbers.length) return null;
        if (hasSym || numbers.length === 1) {
            return { lower: numbers[0], upper: numbers[0] };
        }
        return { lower: numbers[0], upper: numbers[1] };
    };

    const resetOpenPositionDraft = () => {
        setOpenPositionAmount('');
        setOpenPositionRange('+-5');
        setOpenPositionAllowSwap(false);
        setOpenPositionError('');
        setOpenPositionSuccess('');
    };

    const openPositionModal = (pool) => {
        setOpenPositionPool(pool);
        resetOpenPositionDraft();
    };

    const closeOpenPosition = () => {
        if (openPositionLoading) return;
        setOpenPositionPool(null);
    };

    const handleOpenPosition = async () => {
        if (!openPositionPool) return;
        if (!initData) {
            setOpenPositionError('未获取到 Telegram initData，请从机器人入口打开页面。');
            return;
        }
        const amount = Number(String(openPositionAmount || '').trim());
        if (!Number.isFinite(amount) || amount <= 0) {
            setOpenPositionError('请输入有效的金额。');
            return;
        }
        const range = parseRangeInput(openPositionRange);
        if (!range || range.lower <= 0 || range.upper <= 0 || range.lower >= 100 || range.upper >= 100) {
            setOpenPositionError('区间无效，请输入 0-100 之间的百分比。');
            return;
        }

        setOpenPositionLoading(true);
        setOpenPositionError('');
        setOpenPositionSuccess('');
        try {
            const resp = await openPosition({
                apiBaseUrl,
                initData,
                poolAddress: openPositionPool?.pool_address,
                poolVersion: openPositionPool?.protocol_version,
                amount,
                rangeLowerPct: range.lower,
                rangeUpperPct: range.upper,
                allowEntrySwap: openPositionAllowSwap,
            });
            const hash = String(resp?.tx_hash || '').trim();
            setOpenPositionSuccess(hash ? `开仓成功，交易哈希：${hash}` : '开仓成功，已提交链上交易。');
        } catch (e) {
            const msg = String(e?.message || e || '').trim();
            if (msg.includes('entry swap required')) {
                setOpenPositionError('该池子不含 USDT，需在机器人里确认兑换后才能开仓。');
            } else {
                setOpenPositionError(msg || '开仓失败，请稍后重试。');
            }
        } finally {
            setOpenPositionLoading(false);
        }
    };

    const handleAdminDisableAuto = async () => {
        if (!initData || !showAdmin || !adminSelectedUserId || adminDisableLoading) return;

        const label = adminSelectedUser
            ? formatUserLabel(adminSelectedUser)
            : `用户 ${String(adminSelectedUserId)}`;

        const ok = window.confirm(`确认关闭 ${label} 的 Auto？\n将撤出自动仓位并兑换成稳定币。`);
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
        if (!initData || showAdmin) return;
        const id = Number(taskId);
        if (!Number.isFinite(id) || id <= 0) return;

        const wantPaused = Boolean(paused);
        const ok = window.confirm(wantPaused
            ? '确认暂停该任务？\n暂停后将不再自动执行再平衡/止损等操作。'
            : '确认恢复该任务？\n恢复后将继续自动执行再平衡/止损等操作。');
        if (!ok) return;

        await setTaskPaused({ apiBaseUrl, initData, taskId: id, paused: wantPaused });
    };

    const headerTitle = showAdmin ? '管理面板' : isHotPools ? '热门池子' : '实时仓位';
    const headerSubtext = showAdmin
        ? adminSelectedUser
            ? `用户：${formatUserLabel(adminSelectedUser)}`
            : adminUsersLoading && adminUsers.length === 0
                ? '加载用户中...'
                : adminUsers.length
                    ? `开启Auto用户：${adminUsers.length}`
                    : '暂无开启Auto用户'
        : isHotPools
            ? `5m · ${hotPoolsData?.updated_at ? `更新：${formatRelativeTime(hotPoolsData.updated_at, tick)}` : hotPoolsLoading ? '加载中...' : '暂无数据'} · 自动刷新 ${hotPoolsPollIntervalSec}s`
            : walletAddress
                ? `钱包：${walletAddress.slice(0, 6)}...${walletAddress.slice(-4)}`
                : '加载钱包中...';
    const hasAdminPositions = Boolean(adminPositions);
    const adminSummaryPlaceholder = adminSelectedUserId
        ? adminPositionsLoading
            ? '加载用户仓位中...'
            : '用户仓位暂不可用'
        : '请选择用户查看实时仓位';
    const showEmptyPositions = !isHotPools && Boolean(activeData) && visiblePositions.length === 0;

    const initDataMissing = viewMode !== 'hot_pools' && !initData;

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
        <div className="min-h-screen max-w-[720px] px-4 py-4 pb-[calc(16px+env(safe-area-inset-bottom))] mx-auto">
            <header className="mb-4">
                <div className="flex items-center justify-between gap-3">
                    <div className="flex items-center gap-2">
                        <div className="flex h-9 w-9 items-center justify-center rounded-xl bg-emerald-500/10 text-emerald-700 ring-1 ring-emerald-500/20 dark:bg-emerald-500/15 dark:text-emerald-300 dark:ring-emerald-500/25">
                            <Icon path={isHotPools ? icons.chart : icons.bot} className="h-5 w-5" />
                        </div>
                        <div>
                            <div className="text-lg font-extrabold tracking-tight">{headerTitle}</div>
                            <div className="mt-0.5 text-xs text-zinc-500 dark:text-white/40">{headerSubtext}</div>
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

                <div
                    className={`mt-3 grid ${isAdmin ? 'grid-cols-3' : 'grid-cols-2'
                        } gap-1 rounded-2xl border border-zinc-200 bg-zinc-100/70 p-1 text-xs font-semibold dark:border-white/10 dark:bg-white/5`}
                >
                    <button
                        type="button"
                        onClick={() => setViewMode('hot_pools')}
                        aria-pressed={viewMode === 'hot_pools'}
                        className={`rounded-xl px-3 py-2 transition ${viewMode === 'hot_pools'
                            ? 'bg-white text-zinc-900 shadow-sm dark:bg-white/15 dark:text-white'
                            : 'text-zinc-600 hover:bg-white/60 dark:text-white/50 dark:hover:bg-white/10'
                            }`}
                    >
                        热门池子
                    </button>
                    <button
                        type="button"
                        onClick={() => setViewMode('positions')}
                        aria-pressed={viewMode === 'positions'}
                        className={`rounded-xl px-3 py-2 transition ${viewMode === 'positions'
                            ? 'bg-white text-zinc-900 shadow-sm dark:bg-white/15 dark:text-white'
                            : 'text-zinc-600 hover:bg-white/60 dark:text-white/50 dark:hover:bg-white/10'
                            }`}
                    >
                        实时仓位
                    </button>
                    {isAdmin ? (
                        <button
                            type="button"
                            onClick={() => setViewMode('admin')}
                            aria-pressed={viewMode === 'admin'}
                            className={`rounded-xl px-3 py-2 transition ${viewMode === 'admin'
                                ? 'bg-white text-zinc-900 shadow-sm dark:bg-white/15 dark:text-white'
                                : 'text-zinc-600 hover:bg-white/60 dark:text-white/50 dark:hover:bg-white/10'
                                }`}
                        >
                            管理
                        </button>
                    ) : null}
                </div>

                {showAdmin ? (
                    hasAdminPositions ? (
                        <div className="mt-3 rounded-2xl border border-zinc-200 bg-white p-4 shadow-sm dark:border-white/10 dark:bg-[#111318] dark:shadow-none">
                            <div className="flex items-start justify-between gap-4">
                                <div>
                                    <div className="text-[11px] text-zinc-500 dark:text-white/40">总余额</div>
                                    <div className="mt-0.5 text-2xl font-extrabold tabular-nums text-zinc-900 dark:text-emerald-300">
                                        {formatUsd(totalUsd)}
                                    </div>
                                    <div className="mt-1 text-[11px] text-zinc-500 dark:text-white/40 tabular-nums">
                                        {bnbBalance} BNB{typeof bnbUsd === 'number' ? ` ≈ ${formatUsd(bnbUsd)}` : ''}
                                    </div>
                                </div>
                                <div className="text-right">
                                    <div className="text-[11px] text-zinc-500 dark:text-white/40">自动刷新</div>
                                    <div className="text-sm font-semibold tabular-nums">{pollIntervalSec}s</div>
                                </div>
                            </div>
                        </div>
                    ) : (
                        <div className="mt-3 rounded-2xl border border-zinc-200 bg-white/70 p-4 text-sm text-zinc-500 dark:border-white/10 dark:bg-white/5 dark:text-white/60">
                            {adminSummaryPlaceholder}
                        </div>
                    )
                ) : isHotPools ? (
                    <div className="mt-3 rounded-2xl border border-zinc-200 bg-white p-4 shadow-sm dark:border-white/10 dark:bg-[#111318] dark:shadow-none">
                        <div className="flex items-center justify-between gap-3">
                            <div>
                                <div className="text-sm font-semibold text-zinc-900 dark:text-white/90">费用排行</div>
                                <div className="mt-0.5 text-[11px] text-zinc-500 dark:text-white/40">
                                    {hotPoolsData?.updated_at ? `更新：${formatRelativeTime(hotPoolsData.updated_at, tick)}` : hotPoolsLoading ? '加载中...' : '暂无数据'}
                                    {hotPoolsData && hotPoolsFilterLabel ? ` · ${hotPoolsFilterLabel}` : ''}
                                </div>
                            </div>
                            <div className="flex items-center gap-2">
                                <div className="flex rounded-2xl border border-zinc-200 bg-zinc-100/70 p-1 text-xs font-semibold dark:border-white/10 dark:bg-white/5">
                                    {[
                                        { key: 'fees', label: '费用' },
                                        { key: 'fee_rate', label: '费用率' },
                                        { key: 'volume', label: '交易量' },
                                    ].map((tab) => (
                                        <button
                                            key={tab.key}
                                            type="button"
                                            onClick={() => setHotPoolsSort(tab.key)}
                                            aria-pressed={hotPoolsSort === tab.key}
                                            className={`rounded-xl px-3 py-2 transition ${hotPoolsSort === tab.key
                                                ? 'bg-emerald-500 text-white shadow-sm'
                                                : 'text-zinc-600 hover:bg-white/60 dark:text-white/50 dark:hover:bg-white/10'
                                                }`}
                                        >
                                            {tab.label}
                                        </button>
                                    ))}
                                </div>
                                <button
                                    type="button"
                                    onClick={() => setHotPoolsFilterOpen(true)}
                                    className={`relative inline-flex h-9 w-9 items-center justify-center rounded-xl ring-1 transition ${hotPoolsFilterEnabled
                                        ? 'bg-emerald-500/15 text-emerald-700 ring-emerald-500/25 dark:text-emerald-200'
                                        : 'bg-white/70 text-zinc-700 ring-zinc-200 hover:bg-white dark:bg-white/5 dark:text-white/70 dark:ring-white/10'
                                        }`}
                                    aria-label="Filter"
                                    title="Filter"
                                >
                                    <Icon path={icons.filter} className="h-4 w-4" />
                                    {hotPoolsFilterEnabled ? (
                                        <span className="absolute -right-0.5 -top-0.5 h-2.5 w-2.5 rounded-full bg-emerald-400 ring-2 ring-white dark:ring-[#111318]" />
                                    ) : null}
                                </button>
                            </div>
                        </div>
                    </div>
                ) : (
                    <div className="mt-3 rounded-2xl border border-zinc-200 bg-white p-4 shadow-sm dark:border-white/10 dark:bg-[#111318] dark:shadow-none">
                        <div className="flex items-start justify-between gap-4">
                            <div>
                                <div className="text-[11px] text-zinc-500 dark:text-white/40">总余额</div>
                                <div className="mt-0.5 text-2xl font-extrabold tabular-nums text-zinc-900 dark:text-emerald-300">
                                    {formatUsd(totalUsd)}
                                </div>
                                <div className="mt-1 text-[11px] text-zinc-500 dark:text-white/40 tabular-nums">
                                    {bnbBalance} BNB{typeof bnbUsd === 'number' ? ` ≈ ${formatUsd(bnbUsd)}` : ''}
                                </div>
                            </div>
                            <div className="text-right">
                                <div className="text-[11px] text-zinc-500 dark:text-white/40">自动刷新</div>
                                <div className="text-sm font-semibold tabular-nums">{pollIntervalSec}s</div>
                            </div>
                        </div>
                    </div>
                )}
            </header>

            {isHotPools && hotPoolsError ? (
                <div className="mb-4 rounded-2xl border border-red-500/30 bg-red-500/10 p-4 text-sm text-red-700 dark:text-red-200">
                    {hotPoolsError}
                </div>
            ) : null}

            {isHotPools && hotPoolsLoading && hotPoolsRows.length === 0 ? (
                <div className="mb-4 rounded-2xl border border-zinc-200 bg-white/70 p-6 text-sm text-zinc-500 dark:border-white/10 dark:bg-white/5 dark:text-white/60">
                    加载中...
                </div>
            ) : null}

            {isHotPools && !hotPoolsLoading && !hotPoolsError && hotPoolsData && hotPoolsRows.length === 0 ? (
                <div className="mb-4 rounded-2xl border border-zinc-200 bg-white/70 p-6 text-sm text-zinc-500 dark:border-white/10 dark:bg-white/5 dark:text-white/60">
                    暂无热门池子数据。
                </div>
            ) : null}

            {isHotPools && !hotPoolsLoading && !hotPoolsError && hotPoolsData && hotPoolsRows.length > 0 && hotPoolsFilterEnabled && hotPoolsVisibleRows.length === 0 ? (
                <div className="mb-4 rounded-2xl border border-zinc-200 bg-white/70 p-6 text-sm text-zinc-500 dark:border-white/10 dark:bg-white/5 dark:text-white/60">
                    筛选后暂无热门池子数据。
                </div>
            ) : null}

            {!isHotPools && showAdmin ? (
                <div className="mb-4 rounded-2xl border border-zinc-200 bg-white p-4 shadow-sm dark:border-white/10 dark:bg-[#111318] dark:shadow-none">
                    <div className="flex items-center justify-between">
                        <div className="text-sm font-semibold text-zinc-900 dark:text-white/90">开启Auto用户</div>
                        <div className="text-[11px] text-zinc-500 dark:text-white/40">{adminUsers.length} 人</div>
                    </div>

                    {adminUsersError ? (
                        <div className="mt-3 rounded-xl border border-red-500/30 bg-red-500/10 p-3 text-xs text-red-700 dark:text-red-200">
                            {adminUsersError}
                        </div>
                    ) : null}

                    {adminUsersLoading && adminUsers.length === 0 ? (
                        <div className="mt-3 rounded-xl border border-zinc-200 bg-white/70 p-3 text-xs text-zinc-500 dark:border-white/10 dark:bg-white/5 dark:text-white/60">
                            加载中...
                        </div>
                    ) : null}

                    {!adminUsersLoading && adminUsers.length === 0 ? (
                        <div className="mt-3 rounded-xl border border-zinc-200 bg-white/70 p-3 text-xs text-zinc-500 dark:border-white/10 dark:bg-white/5 dark:text-white/60">
                            暂无开启Auto用户
                        </div>
                    ) : null}

                    {adminUsers.length ? (
                        <div className="mt-3 space-y-2">
                            {adminUsers.map((u) => {
                                const selected = Number(u?.user_id) === Number(adminSelectedUserId);
                                const label = formatUserLabel(u);
                                const updatedText = formatRelativeTime(u?.updated_at, tick) || '--';
                                return (
                                    <button
                                        key={u.user_id}
                                        type="button"
                                        onClick={() => {
                                            if (Number(u?.user_id) === Number(adminSelectedUserId)) return;
                                            setAdminSelectedUserId(u.user_id);
                                            setAdminPositions(null);
                                            setAdminPositionsError('');
                                        }}
                                        className={`w-full rounded-xl border p-3 text-left transition ${selected
                                            ? 'border-emerald-500/40 bg-emerald-500/10 text-emerald-900 dark:text-emerald-100'
                                            : 'border-zinc-200 bg-white/70 text-zinc-900 hover:bg-white dark:border-white/10 dark:bg-white/5 dark:text-white/80 dark:hover:bg-white/10'
                                            }`}
                                    >
                                        <div className="flex items-start justify-between gap-3">
                                            <div>
                                                <div className="text-sm font-semibold">{label}</div>
                                                <div
                                                    className={`mt-0.5 text-[11px] ${selected ? 'text-emerald-700/80 dark:text-emerald-200/80' : 'text-zinc-500 dark:text-white/40'
                                                        }`}
                                                >
                                                    {u.telegram_id ? `TG ${u.telegram_id}` : 'TG --'} · ID {u.user_id}
                                                </div>
                                            </div>
                                            <div className="text-right">
                                                <div className={`text-xs font-semibold ${selected ? 'text-emerald-700 dark:text-emerald-200' : 'text-zinc-700 dark:text-white/70'}`}>
                                                    {u.active_tasks} 个任务
                                                </div>
                                                <div
                                                    className={`mt-0.5 text-[11px] ${selected ? 'text-emerald-700/70 dark:text-emerald-200/70' : 'text-zinc-500 dark:text-white/40'
                                                        }`}
                                                >
                                                    {updatedText}
                                                </div>
                                            </div>
                                        </div>
                                    </button>
                                );
                            })}
                        </div>
                    ) : null}
                </div>
            ) : null}

            {!isHotPools && showAdmin && adminSelectedUserId ? (
                <div className="mb-4 rounded-2xl border border-zinc-200 bg-white p-4 shadow-sm dark:border-white/10 dark:bg-[#111318] dark:shadow-none">
                    <div className="flex items-start justify-between gap-3">
                        <div className="min-w-0">
                            <div className="flex flex-wrap items-center gap-2">
                                <div className="text-sm font-semibold text-zinc-900 dark:text-white/90">Auto 统计</div>
                                <div
                                    className={`rounded-lg px-2 py-0.5 text-[11px] font-semibold ring-1 ${adminAutoStats?.config?.enabled
                                        ? 'bg-emerald-500/10 text-emerald-700 ring-emerald-500/25 dark:text-emerald-300'
                                        : 'bg-zinc-500/10 text-zinc-700 ring-zinc-500/20 dark:text-white/60'
                                        }`}
                                >
                                    {adminAutoStats?.config?.enabled ? 'Auto 开启' : 'Auto 已关闭'}
                                </div>
                            </div>
                            <div className="mt-0.5 text-[11px] text-zinc-500 dark:text-white/40">
                                {adminSelectedUser ? `${formatUserLabel(adminSelectedUser)} · ID ${adminSelectedUserId}` : `用户 ID ${adminSelectedUserId}`}
                            </div>
                            {adminAutoStats?.stats?.window_label ? (
                                <div className="mt-0.5 text-[11px] text-zinc-500 dark:text-white/40">
                                    周期：{adminAutoStats.stats.window_label}
                                </div>
                            ) : null}
                        </div>

                        <button
                            type="button"
                            onClick={handleAdminDisableAuto}
                            disabled={adminDisableLoading}
                            className={`rounded-xl px-3 py-2 text-xs font-semibold ring-1 transition ${adminDisableLoading
                                ? 'cursor-not-allowed bg-rose-500/10 text-rose-700/70 ring-rose-500/15 dark:text-rose-200/60'
                                : 'bg-rose-500/15 text-rose-700 ring-rose-500/25 hover:bg-rose-500/20 dark:text-rose-200'
                                }`}
                        >
                            {adminDisableLoading ? '关闭中...' : '关闭 Auto'}
                        </button>
                    </div>

                    {adminDisableError ? (
                        <div className="mt-3 rounded-xl border border-red-500/30 bg-red-500/10 p-3 text-xs text-red-700 dark:text-red-200">
                            {adminDisableError}
                        </div>
                    ) : null}

                    {adminDisableResult ? (
                        <div className="mt-3 rounded-xl border border-emerald-500/30 bg-emerald-500/10 p-3 text-xs text-emerald-700 dark:text-emerald-200">
                            已发起关闭：找到 {adminDisableResult.tasks_found} 个 Auto 任务，已请求撤出 {adminDisableResult.exit_requested} 个。
                        </div>
                    ) : null}

                    {adminAutoStatsError ? (
                        <div className="mt-3 rounded-xl border border-red-500/30 bg-red-500/10 p-3 text-xs text-red-700 dark:text-red-200">
                            {adminAutoStatsError}
                        </div>
                    ) : null}

                    {adminAutoStatsLoading && !adminAutoStats ? (
                        <div className="mt-3 rounded-xl border border-zinc-200 bg-white/70 p-3 text-xs text-zinc-500 dark:border-white/10 dark:bg-white/5 dark:text-white/60">
                            加载中...
                        </div>
                    ) : null}

                    {adminAutoStats?.stats ? (
                        <div className="mt-3 grid grid-cols-2 gap-3 text-xs">
                            <div className="rounded-xl border border-zinc-200 bg-zinc-50 p-3 dark:border-white/10 dark:bg-[#0f1116]">
                                <div className="text-[11px] text-zinc-500 dark:text-white/40">累计收益</div>
                                <div className="mt-0.5 text-sm font-extrabold tabular-nums text-emerald-700 dark:text-emerald-300">
                                    {adminAutoStats?.formatted?.profit_usdt ?? '--'} USDT
                                </div>
                            </div>
                            <div className="rounded-xl border border-zinc-200 bg-zinc-50 p-3 dark:border-white/10 dark:bg-[#0f1116]">
                                <div className="text-[11px] text-zinc-500 dark:text-white/40">Gas 消耗</div>
                                <div className="mt-0.5 text-sm font-extrabold tabular-nums text-zinc-900 dark:text-white/80">
                                    {adminAutoStats?.formatted?.gas_usdt ?? '--'} USDT
                                </div>
                            </div>
                            <div className="rounded-xl border border-zinc-200 bg-zinc-50 p-3 dark:border-white/10 dark:bg-[#0f1116]">
                                <div className="text-[11px] text-zinc-500 dark:text-white/40">开仓 / 再平衡</div>
                                <div className="mt-0.5 text-sm font-extrabold tabular-nums text-zinc-900 dark:text-white/80">
                                    {adminAutoStats.stats.open_count} / {adminAutoStats.stats.rebalance_count}
                                </div>
                            </div>
                            <div className="rounded-xl border border-zinc-200 bg-zinc-50 p-3 dark:border-white/10 dark:bg-[#0f1116]">
                                <div className="text-[11px] text-zinc-500 dark:text-white/40">撤退卫士</div>
                                <div className="mt-0.5 text-sm font-extrabold tabular-nums text-zinc-900 dark:text-white/80">
                                    {adminAutoStats.stats.guard_count}
                                </div>
                            </div>

                            <div className="rounded-xl border border-zinc-200 bg-zinc-50 p-3 dark:border-white/10 dark:bg-[#0f1116] col-span-2">
                                <div className="flex flex-wrap gap-x-4 gap-y-2">
                                    <div className="text-[11px] text-zinc-500 dark:text-white/40">
                                        盈利交易对：
                                        <span className="ml-1 font-semibold text-zinc-900 dark:text-white/80">
                                            {adminAutoStats.stats.best_pair ? `${adminAutoStats.stats.best_pair}（${adminAutoStats?.formatted?.best_profit_usdt ?? '--'} USDT）` : '--'}
                                        </span>
                                    </div>
                                    <div className="text-[11px] text-zinc-500 dark:text-white/40">
                                        亏损交易对：
                                        <span className="ml-1 font-semibold text-zinc-900 dark:text-white/80">
                                            {adminAutoStats.stats.worst_pair ? `${adminAutoStats.stats.worst_pair}（${adminAutoStats?.formatted?.worst_profit_usdt ?? '--'} USDT）` : '--'}
                                        </span>
                                    </div>
                                </div>
                            </div>
                        </div>
                    ) : null}
                </div>
            ) : null}

            {!isHotPools && initDataMissing ? (
                <div className="mb-4 rounded-2xl border border-amber-500/30 bg-amber-500/10 p-4 text-sm text-amber-700 dark:text-amber-200">
                    请从 Telegram 机器人里的“实时仓位”按钮打开页面（否则无法读取你的仓位）。
                </div>
            ) : null}

            {!isHotPools && activeErrorText ? (
                <div className="mb-4 rounded-2xl border border-red-500/30 bg-red-500/10 p-4 text-sm text-red-700 dark:text-red-200">
                    {activeErrorText}
                </div>
            ) : null}

            {!isHotPools && showAdmin && !adminSelectedUserId ? (
                <div className="rounded-2xl border border-zinc-200 bg-white/70 p-6 text-sm text-zinc-500 dark:border-white/10 dark:bg-white/5 dark:text-white/60">
                    请选择用户查看实时仓位。
                </div>
            ) : null}

            {!isHotPools && activeLoading && !activeData ? (
                <div className="rounded-2xl border border-zinc-200 bg-white/70 p-6 text-sm text-zinc-500 dark:border-white/10 dark:bg-white/5 dark:text-white/60">
                    加载中...
                </div>
            ) : null}

            {showEmptyPositions ? (
                <div className="rounded-2xl border border-zinc-200 bg-white/70 p-6 text-sm text-zinc-500 dark:border-white/10 dark:bg-white/5 dark:text-white/60">
                    暂无仓位。请先在机器人里导入钱包并开仓。
                </div>
            ) : null}

            <div className="space-y-4">
                {isHotPools
                    ? hotPoolsVisibleRows.map((row) => {
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
                                onOpenKline={setKlinePool}
                                onOpenPosition={openPositionModal}
                            />
                        );
                    })
                    : activeData
                        ? visiblePositions.map((p) => (
                            <PositionCard
                                key={`${p.version}:${p.position_id}`}
                                position={p}
                                walletAddress={walletAddress}
                                bnbBalance={bnbBalance}
                                pollIntervalSec={pollIntervalSec}
                                updatedAt={updatedAt}
                                allowTaskActions={!showAdmin && Boolean(initData)}
                                onSetTaskPaused={handleSetTaskPaused}
                            />
                        ))
                        : null}
            </div>

            {!isHotPools && activeData?.warnings?.length ? (
                <div className="mt-4 rounded-2xl border border-amber-500/30 bg-amber-500/10 p-4 text-xs text-amber-700 dark:text-amber-200">
                    <div className="font-semibold">提示</div>
                    <ul className="mt-1 list-disc space-y-1 pl-4">
                        {activeData.warnings.map((w, i) => (
                            <li key={String(i)}>{w}</li>
                        ))}
                    </ul>
                </div>
            ) : null}

            {hotPoolsFilterOpen ? (
                <div className="fixed inset-0 z-50">
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

                        <div className="mt-4 space-y-4">
                            <div className="rounded-xl border border-zinc-200 bg-zinc-50 p-3 dark:border-white/10 dark:bg-[#0f1116]">
                                <div className="flex items-center justify-between gap-2">
                                    <button
                                        type="button"
                                        onClick={() => setHotPoolsFilterDraft((prev) => ({ ...prev, enabled: !prev.enabled }))}
                                        className={`inline-flex items-center gap-2 rounded-xl px-3 py-1.5 text-xs font-semibold ring-1 ${hotPoolsFilterDraft.enabled
                                            ? 'bg-emerald-500/15 text-emerald-700 ring-emerald-500/25 dark:text-emerald-200'
                                            : 'bg-white/70 text-zinc-700 ring-zinc-200 hover:bg-white dark:bg-white/5 dark:text-white/70 dark:ring-white/10'
                                            }`}
                                        aria-label="切换筛选"
                                        title="切换筛选"
                                    >
                                        <span className={`h-2 w-2 rounded-full ${hotPoolsFilterDraft.enabled ? 'bg-emerald-500' : 'bg-zinc-400'}`} />
                                        <span className="text-[10px] tabular-nums">{hotPoolsFilterDraft.enabled ? '开启' : '关闭'}</span>
                                    </button>
                                </div>

                                <div className="mt-3 grid grid-cols-2 gap-3">
                                    <div>
                                        <div className="text-[11px] text-zinc-500 dark:text-white/40">手续费 >= (USD)</div>
                                        <input
                                            value={hotPoolsFilterDraft.minFees}
                                            onChange={(e) => setHotPoolsFilterDraft((prev) => ({ ...prev, minFees: e.target.value }))}
                                            inputMode="decimal"
                                            className="mt-1 w-full rounded-xl border border-zinc-200 bg-white/70 px-3 py-2 text-sm text-zinc-900 shadow-sm outline-none ring-0 placeholder:text-zinc-400 focus:border-emerald-400 dark:border-white/10 dark:bg-white/5 dark:text-white/90 dark:placeholder:text-white/30"
                                            placeholder={String(defaultHotPoolsFilter.minFees)}
                                        />
                                    </div>
                                    <div>
                                        <div className="text-[11px] text-zinc-500 dark:text-white/40">费用率 >= (%)</div>
                                        <input
                                            value={hotPoolsFilterDraft.minFeeRate}
                                            onChange={(e) => setHotPoolsFilterDraft((prev) => ({ ...prev, minFeeRate: e.target.value }))}
                                            inputMode="decimal"
                                            className="mt-1 w-full rounded-xl border border-zinc-200 bg-white/70 px-3 py-2 text-sm text-zinc-900 shadow-sm outline-none ring-0 placeholder:text-zinc-400 focus:border-emerald-400 dark:border-white/10 dark:bg-white/5 dark:text-white/90 dark:placeholder:text-white/30"
                                            placeholder={String(defaultHotPoolsFilter.minFeeRate)}
                                        />
                                    </div>
                                    <div>
                                        <div className="text-[11px] text-zinc-500 dark:text-white/40">TVL >= (USD)</div>
                                        <input
                                            value={hotPoolsFilterDraft.minTvl}
                                            onChange={(e) => setHotPoolsFilterDraft((prev) => ({ ...prev, minTvl: e.target.value }))}
                                            inputMode="decimal"
                                            className="mt-1 w-full rounded-xl border border-zinc-200 bg-white/70 px-3 py-2 text-sm text-zinc-900 shadow-sm outline-none ring-0 placeholder:text-zinc-400 focus:border-emerald-400 dark:border-white/10 dark:bg-white/5 dark:text-white/90 dark:placeholder:text-white/30"
                                            placeholder={String(defaultHotPoolsFilter.minTvl)}
                                        />
                                    </div>
                                    <div>
                                        <div className="text-[11px] text-zinc-500 dark:text-white/40">交易量 >= (USD)</div>
                                        <input
                                            value={hotPoolsFilterDraft.minVolume}
                                            onChange={(e) => setHotPoolsFilterDraft((prev) => ({ ...prev, minVolume: e.target.value }))}
                                            inputMode="decimal"
                                            className="mt-1 w-full rounded-xl border border-zinc-200 bg-white/70 px-3 py-2 text-sm text-zinc-900 shadow-sm outline-none ring-0 placeholder:text-zinc-400 focus:border-emerald-400 dark:border-white/10 dark:bg-white/5 dark:text-white/90 dark:placeholder:text-white/30"
                                            placeholder={String(defaultHotPoolsFilter.minVolume)}
                                        />
                                    </div>
                                </div>

                                <div className="mt-3 flex flex-wrap gap-2">
                                    <button
                                        type="button"
                                        onClick={applyHotPoolsFilter}
                                        className="inline-flex items-center gap-2 rounded-xl bg-emerald-500 px-3 py-2 text-xs font-semibold text-white shadow-sm hover:bg-emerald-600 active:bg-emerald-700"
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
                                        onClick={disableHotPoolsFilter}
                                        className="inline-flex items-center gap-2 rounded-xl bg-white/70 px-3 py-2 text-xs font-semibold text-zinc-700 ring-1 ring-zinc-200 hover:bg-white dark:bg-white/5 dark:text-white/70 dark:ring-white/10"
                                        aria-label="不筛选"
                                        title="不筛选"
                                    >
                                        <Icon path={icons.ban} className="h-4 w-4" />
                                        不筛选
                                    </button>
                                </div>
                            </div>
                        </div>
                    </div>
                </div>
            ) : null}

            {settingsOpen ? (
                <div className="fixed inset-0 z-50">
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
                                <div className="text-xs font-semibold text-zinc-900 dark:text-white/80">自动刷新</div>
                                <div className="mt-0.5 text-[11px] text-zinc-500 dark:text-white/40">
                                    当前：{settingsPollIntervalSec}s（{pollOverrideSec ? '自定义' : `默认 ${settingsServerPollIntervalSec}s`})
                                </div>
                                <div className="mt-3 flex flex-wrap gap-2">
                                    {[5, 10, 15, 30, 60].map((sec) => (
                                        <button
                                            key={sec}
                                            type="button"
                                            onClick={() => setQuickPoll(sec)}
                                            className={`rounded-xl px-3 py-1.5 text-xs font-semibold ring-1 ${pollOverrideSec === sec
                                                ? 'bg-emerald-500/15 text-emerald-700 ring-emerald-500/25 dark:text-emerald-300'
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
                                        className="w-28 rounded-xl border border-zinc-200 bg-white/70 px-3 py-2 text-sm text-zinc-900 shadow-sm outline-none ring-0 placeholder:text-zinc-400 focus:border-emerald-400 dark:border-white/10 dark:bg-white/5 dark:text-white/90 dark:placeholder:text-white/30"
                                        placeholder="1-300"
                                    />
                                    <button
                                        type="button"
                                        onClick={applyPollDraft}
                                        className="rounded-xl bg-emerald-500 px-3 py-2 text-sm font-semibold text-white shadow-sm hover:bg-emerald-600 active:bg-emerald-700"
                                    >
                                        确定
                                    </button>
                                </div>
                            </div>
                        </div>
                    </div>
                </div>
            ) : null}

            {openPositionPool ? (
                <div className="fixed inset-0 z-50">
                    <button
                        type="button"
                        className="absolute inset-0 bg-black/40"
                        onClick={closeOpenPosition}
                        aria-label="关闭开仓"
                    />
                    <div className="absolute inset-x-0 bottom-0 rounded-t-2xl border border-zinc-200 bg-white p-4 shadow-2xl dark:border-white/10 dark:bg-[#111318] dark:shadow-none">
                        <div className="flex items-center justify-between gap-2">
                            <div className="min-w-0">
                                <div className="text-sm font-semibold text-zinc-900 dark:text-white/90">一键开仓</div>
                                <div className="mt-0.5 text-[11px] text-zinc-500 dark:text-white/40 truncate">
                                    {openPositionPool?.trading_pair || '--'}
                                </div>
                            </div>
                            <button
                                type="button"
                                onClick={closeOpenPosition}
                                className="inline-flex h-9 w-9 items-center justify-center rounded-xl border border-zinc-200 bg-zinc-100 text-zinc-900 hover:bg-zinc-200 active:bg-zinc-200 dark:border-white/10 dark:bg-white/5 dark:text-white/80 dark:hover:bg-white/10 dark:active:bg-white/15"
                                aria-label="关闭"
                            >
                                <Icon path={icons.close} className="h-5 w-5" />
                            </button>
                        </div>

                        <div className="mt-4 space-y-4">
                            <div className="rounded-xl border border-zinc-200 bg-zinc-50 p-3 dark:border-white/10 dark:bg-[#0f1116]">
                                <div className="text-xs font-semibold text-zinc-900 dark:text-white/80">投入金额 (USDT)</div>
                                <input
                                    value={openPositionAmount}
                                    onChange={(e) => {
                                        setOpenPositionAmount(e.target.value);
                                        setOpenPositionError('');
                                    }}
                                    inputMode="decimal"
                                    className="mt-2 w-full rounded-xl border border-zinc-200 bg-white/70 px-3 py-2 text-sm text-zinc-900 shadow-sm outline-none ring-0 placeholder:text-zinc-400 focus:border-emerald-400 dark:border-white/10 dark:bg-white/5 dark:text-white/90 dark:placeholder:text-white/30"
                                    placeholder="例如 100"
                                />
                            </div>

                            <div className="rounded-xl border border-zinc-200 bg-zinc-50 p-3 dark:border-white/10 dark:bg-[#0f1116]">
                                <div className="text-xs font-semibold text-zinc-900 dark:text-white/80">自定义区间 (%)</div>
                                <input
                                    value={openPositionRange}
                                    onChange={(e) => {
                                        setOpenPositionRange(e.target.value);
                                        setOpenPositionError('');
                                    }}
                                    inputMode="text"
                                    className="mt-2 w-full rounded-xl border border-zinc-200 bg-white/70 px-3 py-2 text-sm text-zinc-900 shadow-sm outline-none ring-0 placeholder:text-zinc-400 focus:border-emerald-400 dark:border-white/10 dark:bg-white/5 dark:text-white/90 dark:placeholder:text-white/30"
                                    placeholder="例如 ±5 / 1 3"
                                />
                                <div className="mt-2 flex flex-wrap gap-2">
                                    {quickRangeOptions.map((option) => (
                                        <button
                                            key={option.value}
                                            type="button"
                                            onClick={() => {
                                                setOpenPositionRange(option.value);
                                                setOpenPositionError('');
                                            }}
                                            className="rounded-xl px-3 py-1.5 text-xs font-semibold text-emerald-700 ring-1 ring-emerald-500/30 bg-gradient-to-r from-emerald-50 via-emerald-100/60 to-sky-100/60 hover:from-emerald-100 hover:via-emerald-200/70 hover:to-sky-200/70 dark:text-emerald-200 dark:ring-emerald-400/30 dark:from-emerald-500/10 dark:via-emerald-400/10 dark:to-sky-400/10"
                                        >
                                            {option.label}
                                        </button>
                                    ))}
                                </div>
                                <div className="mt-2 text-[11px] text-zinc-500 dark:text-white/40">
                                    支持对称输入（±5）或非对称输入（1 3 表示下 1% 上 3%）。
                                </div>
                            </div>

                            <div className="rounded-xl border border-zinc-200 bg-zinc-50 p-3 dark:border-white/10 dark:bg-[#0f1116]">
                                <div className="flex items-center justify-between gap-3">
                                    <div>
                                        <div className="text-xs font-semibold text-zinc-900 dark:text-white/80">允许兑换</div>
                                        <div className="mt-0.5 text-[11px] text-zinc-500 dark:text-white/40">
                                            池子不含 USDT 时，允许自动兑换入场代币。
                                        </div>
                                    </div>
                                    <button
                                        type="button"
                                        onClick={() => {
                                            setOpenPositionAllowSwap((v) => !v);
                                            setOpenPositionError('');
                                        }}
                                        className={`inline-flex h-8 items-center rounded-full border px-3 text-xs font-semibold transition ${openPositionAllowSwap
                                            ? 'border-emerald-500/50 bg-emerald-500/20 text-emerald-700 dark:text-emerald-200'
                                            : 'border-zinc-200 bg-white/70 text-zinc-600 dark:border-white/10 dark:bg-white/5 dark:text-white/60'
                                            }`}
                                    >
                                        {openPositionAllowSwap ? '已开启' : '已关闭'}
                                    </button>
                                </div>
                            </div>

                            {openPositionError ? (
                                <div className="rounded-xl border border-red-500/30 bg-red-500/10 p-3 text-xs text-red-700 dark:text-red-200">
                                    {openPositionError}
                                </div>
                            ) : null}
                            {openPositionSuccess ? (
                                <div className="rounded-xl border border-emerald-500/30 bg-emerald-500/10 p-3 text-xs text-emerald-700 dark:text-emerald-200">
                                    {openPositionSuccess}
                                </div>
                            ) : null}

                            <button
                                type="button"
                                onClick={handleOpenPosition}
                                disabled={openPositionLoading}
                                className={`w-full rounded-xl px-3 py-2 text-sm font-semibold text-white shadow-sm transition ${openPositionLoading
                                    ? 'cursor-not-allowed bg-emerald-500/60'
                                    : 'bg-emerald-500 hover:bg-emerald-600 active:bg-emerald-700'
                                    }`}
                            >
                                {openPositionLoading ? '开仓中...' : '确认开仓'}
                            </button>
                        </div>
                    </div>
                </div>
            ) : null}

            <KlineModal
                open={Boolean(klinePool)}
                onClose={() => setKlinePool(null)}
                apiBaseUrl={apiBaseUrl}
                theme={theme}
                pool={klinePool}
                chain={hotPoolsData?.chain || 'bsc'}
            />
        </div>
    );
}
