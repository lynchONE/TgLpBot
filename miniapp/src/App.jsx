import React, { useEffect, useMemo, useRef, useState } from 'react';
import PositionCard from './components/PositionCard.jsx';
import { fetchRealtimePositions } from './lib/api';
import { getTelegramWebApp } from './lib/telegram';

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

const Icon = ({ path, className = '' }) => (
    <svg viewBox="0 0 24 24" fill="currentColor" className={className} aria-hidden="true">
        <path d={path} />
    </svg>
);

const icons = {
    bot: 'M12 2a2 2 0 012 2v1h1a3 3 0 013 3v7a7 7 0 11-14 0V8a3 3 0 013-3h1V4a2 2 0 012-2zm-4 7a1.25 1.25 0 100 2.5A1.25 1.25 0 008 9zm8 0a1.25 1.25 0 100 2.5A1.25 1.25 0 0016 9zm-7.5 6.5h7a3.5 3.5 0 01-7 0z',
    gear: 'M12 8.25a3.75 3.75 0 100 7.5 3.75 3.75 0 000-7.5zm9 3.75l-1.9.95a7.9 7.9 0 01-.5 1.2l.7 2.03-2.12 2.12-2.03-.7c-.38.2-.79.37-1.2.5L12 21l-1.95-1.9c-.41-.13-.82-.3-1.2-.5l-2.03.7-2.12-2.12.7-2.03c-.2-.38-.37-.79-.5-1.2L3 12l1.9-1.95c.13-.41.3-.82.5-1.2l-.7-2.03 2.12-2.12 2.03.7c.38-.2.79-.37 1.2-.5L12 3l1.95 1.9c.41.13.82.3 1.2.5l2.03-.7 2.12 2.12-.7 2.03c.2.38.37.79.5 1.2L21 12z',
    moon: 'M21 14.5A7.5 7.5 0 019.5 3a6.5 6.5 0 1011.5 11.5z',
    sun: 'M12 18a6 6 0 100-12 6 6 0 000 12zm0-16h1v3h-2V2h1zm0 17h1v3h-2v-3h1zM2 11h3v2H2v-2zm17 0h3v2h-3v-2zM4.2 5.6l2.1 2.1-1.4 1.4-2.1-2.1 1.4-1.4zm13.1 13.1l2.1 2.1-1.4 1.4-2.1-2.1 1.4-1.4zM18.4 4.2l1.4 1.4-2.1 2.1-1.4-1.4 2.1-2.1zM5.6 18.4l1.4 1.4-2.1 2.1-1.4-1.4 2.1-2.1z',
    close: 'M6.225 4.811a1 1 0 011.414 0L12 9.172l4.361-4.361a1 1 0 111.414 1.414L13.414 10.586l4.361 4.361a1 1 0 01-1.414 1.414L12 12l-4.361 4.361a1 1 0 01-1.414-1.414l4.361-4.361-4.361-4.361a1 1 0 010-1.414z',
};

export default function App() {
    const initData = useInitData();
    const [data, setData] = useState(null);
    const [error, setError] = useState('');
    const [loading, setLoading] = useState(false);
    const pollRef = useRef(null);

    const [theme, setTheme] = useState('dark');
    const [settingsOpen, setSettingsOpen] = useState(false);
    const [pollOverrideSec, setPollOverrideSec] = useState(null);
    const [pollDraftSec, setPollDraftSec] = useState('');

    const serverPollIntervalSec = Math.max(1, Number(data?.poll_interval_sec || 1));
    const pollIntervalSec = Math.max(1, Number(pollOverrideSec || serverPollIntervalSec || 1));
    const updatedAt = data?.updated_at;

    const walletAddress = data?.wallet?.address || '';
    const bnbBalance = data?.wallet?.bnb_balance || '0.000000';
    const bnbUsd = data?.wallet?.bnb_usd;
    const summary = data?.summary;
    const positions = data?.positions || [];

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

    const apiBaseUrl = useMemo(() => resolveApiBaseUrl(), []);

    useEffect(() => {
        const tg = getTelegramWebApp();
        const tgTheme = tg?.colorScheme === 'light' ? 'light' : 'dark';
        const savedTheme = storage.get(STORAGE_THEME);
        if (savedTheme === 'light' || savedTheme === 'dark') {
            setTheme(savedTheme);
        } else {
            setTheme(tgTheme);
        }

        const savedPoll = Number(storage.get(STORAGE_POLL_SEC));
        if (Number.isFinite(savedPoll) && savedPoll >= 1) {
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

        const run = async () => {
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

    const applyPollDraft = () => {
        const raw = String(pollDraftSec || '').trim();
        const m = raw.match(/\d+/);
        if (!m) return;
        const n = Number(m[0]);
        if (!Number.isFinite(n)) return;
        const v = Math.max(1, Math.min(300, Math.floor(n)));
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
        const v = Math.max(1, Math.min(300, Math.floor(Number(sec) || 1)));
        setPollOverrideSec(v);
        storage.set(STORAGE_POLL_SEC, String(v));
        setPollDraftSec(String(v));
        setSettingsOpen(false);
    };

    const toggleTheme = () => setTheme((t) => (t === 'dark' ? 'light' : 'dark'));

    return (
        <div className="min-h-screen max-w-[720px] px-4 py-4 pb-[calc(16px+env(safe-area-inset-bottom))] mx-auto">
            <header className="mb-4">
                <div className="flex items-center justify-between gap-3">
                    <div className="flex items-center gap-2">
                        <div className="flex h-9 w-9 items-center justify-center rounded-xl bg-emerald-500/10 text-emerald-700 ring-1 ring-emerald-500/20 dark:bg-emerald-500/15 dark:text-emerald-300 dark:ring-emerald-500/25">
                            <Icon path={icons.bot} className="h-5 w-5" />
                        </div>
                        <div>
                            <div className="text-lg font-extrabold tracking-tight">实时仓位</div>
                            <div className="mt-0.5 text-xs text-zinc-500 dark:text-white/40">
                                {walletAddress ? `钱包：${walletAddress.slice(0, 6)}...${walletAddress.slice(-4)}` : '加载钱包中...'}
                            </div>
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
            </header>

            {error ? (
                <div className="mb-4 rounded-2xl border border-red-500/30 bg-red-500/10 p-4 text-sm text-red-700 dark:text-red-200">
                    {error}
                </div>
            ) : null}

            {loading && !data ? (
                <div className="rounded-2xl border border-zinc-200 bg-white/70 p-6 text-sm text-zinc-500 dark:border-white/10 dark:bg-white/5 dark:text-white/60">
                    加载中...
                </div>
            ) : null}

            {!loading && data && visiblePositions.length === 0 ? (
                <div className="rounded-2xl border border-zinc-200 bg-white/70 p-6 text-sm text-zinc-500 dark:border-white/10 dark:bg-white/5 dark:text-white/60">
                    暂无仓位。请先在机器人里创建/导入钱包并开仓。
                </div>
            ) : null}

            <div className="space-y-4">
                {visiblePositions.map((p) => (
                    <PositionCard
                        key={`${p.version}:${p.position_id}`}
                        position={p}
                        walletAddress={walletAddress}
                        bnbBalance={bnbBalance}
                        pollIntervalSec={pollIntervalSec}
                        updatedAt={updatedAt}
                    />
                ))}
            </div>

            {data?.warnings?.length ? (
                <div className="mt-4 rounded-2xl border border-amber-500/30 bg-amber-500/10 p-4 text-xs text-amber-700 dark:text-amber-200">
                    <div className="font-semibold">提示</div>
                    <ul className="mt-1 list-disc space-y-1 pl-4">
                        {data.warnings.map((w, i) => (
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
                                    当前：{pollIntervalSec}s（{pollOverrideSec ? '自定义' : `默认 ${serverPollIntervalSec}s`})
                                </div>
                                <div className="mt-3 flex flex-wrap gap-2">
                                    {[1, 2, 3, 5, 10, 30].map((sec) => (
                                        <button
                                            key={sec}
                                            type="button"
                                            onClick={() => setQuickPoll(sec)}
                                            className={`rounded-xl px-3 py-1.5 text-xs font-semibold ring-1 ${
                                                pollOverrideSec === sec
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
