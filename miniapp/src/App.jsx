import React, { useEffect, useMemo, useRef, useState } from 'react';
import HotPoolCard from './components/HotPoolCard.jsx';
import PositionCard from './components/PositionCard.jsx';
import { disableAdminAutoLP, fetchAdminAutoLPStats, fetchAdminRealtimePositions, fetchAdminRealtimeUsers, fetchHotPools, fetchRealtimePositions } from './lib/api';
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

function formatUsd(v) {
    const n = Number(v || 0);
    if (!Number.isFinite(n)) return '$0.00';
    return `$${n.toFixed(2)}`;
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
    <svg viewBox="0 0 24 24" fill="currentColor" className={className} aria-hidden="true">
        <path d={path} />
    </svg>
);

const icons = {
    bot: 'M12 2a2 2 0 012 2v1h1a3 3 0 013 3v7a7 7 0 11-14 0V8a3 3 0 013-3h1V4a2 2 0 012-2zm-4 7a1.25 1.25 0 100 2.5A1.25 1.25 0 008 9zm8 0a1.25 1.25 0 100 2.5A1.25 1.25 0 0016 9zm-7.5 6.5h7a3.5 3.5 0 01-7 0z',
    chart: 'M4 19h16v2H2V3h2v16zm4-2H6v-6h2v6zm5 0h-2V7h2v10zm5 0h-2v-4h2v4z',
    gear: 'M12 8.25a3.75 3.75 0 100 7.5 3.75 3.75 0 000-7.5zm9 3.75l-1.9.95a7.9 7.9 0 01-.5 1.2l.7 2.03-2.12 2.12-2.03-.7c-.38.2-.79.37-1.2.5L12 21l-1.95-1.9c-.41-.13-.82-.3-1.2-.5l-2.03.7-2.12-2.12.7-2.03c-.2-.38-.37-.79-.5-1.2L3 12l1.9-1.95c.13-.41.3-.82.5-1.2l-.7-2.03 2.12-2.12 2.03.7c.38-.2.79-.37 1.2-.5L12 3l1.95 1.9c.41.13.82.3 1.2.5l2.03-.7 2.12 2.12-.7 2.03c.2.38.37.79.5 1.2L21 12z',
    moon: 'M21 14.5A7.5 7.5 0 019.5 3a6.5 6.5 0 1011.5 11.5z',
    sun: 'M12 18a6 6 0 100-12 6 6 0 000 12zm0-16h1v3h-2V2h1zm0 17h1v3h-2v-3h1zM2 11h3v2H2v-2zm17 0h3v2h-3v-2zM4.2 5.6l2.1 2.1-1.4 1.4-2.1-2.1 1.4-1.4zm13.1 13.1l2.1 2.1-1.4 1.4-2.1-2.1 1.4-1.4zM18.4 4.2l1.4 1.4-2.1 2.1-1.4-1.4 2.1-2.1zM5.6 18.4l1.4 1.4-2.1 2.1-1.4-1.4 2.1-2.1z',
    close: 'M6.225 4.811a1 1 0 011.414 0L12 9.172l4.361-4.361a1 1 0 111.414 1.414L13.414 10.586l4.361 4.361a1 1 0 01-1.414 1.414L12 12l-4.361 4.361a1 1 0 01-1.414-1.414l4.361-4.361-4.361-4.361a1 1 0 010-1.414z',
};

export default function App() {
    const initData = useInitData();
    const tick = useTick(); // 实时时钟，每秒更新一次
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
    // 保存上一次热门池子数据，用于计算变化
    const previousHotPoolsDataRef = useRef({});

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
    const isAdmin = Boolean(data?.is_admin || adminPositions?.is_admin);
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

    // 构建热门池子的历史数据映射 (protocol_version:pool_address -> previous data)
    const previousHotPoolsMap = useMemo(() => {
        return previousHotPoolsDataRef.current;
    }, [hotPoolsRows]);

    const apiBaseUrl = useMemo(() => resolveApiBaseUrl(), []);

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

    const toggleTheme = () => setTheme((t) => (t === 'dark' ? 'light' : 'dark'));

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
                            className="inline-flex h-9 w-9 items-center justify-center rounded-xl border border-zinc-200 bg-zinc-100 text-zinc-900 shadow-sm hover:bg-zinc-200 active:bg-zinc-200 dark:border-white/10 dark:bg-white/5 dark:text-white/80 dark:hover:bg-white/10 dark:active:bg-white/15"
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
                                </div>
                            </div>
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

            {!isHotPools && activeError ? (
                <div className="mb-4 rounded-2xl border border-red-500/30 bg-red-500/10 p-4 text-sm text-red-700 dark:text-red-200">
                    {activeError}
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
                    ? hotPoolsRows.map((row) => {
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
        </div>
    );
}
