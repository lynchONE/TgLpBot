import React, { useEffect, useMemo, useRef, useState } from 'react';
import PositionCard from './components/PositionCard.jsx';
import { fetchRealtimePositions } from './lib/api';
import { getTelegramWebApp } from './lib/telegram';
import { formatRelativeTime } from './lib/time';

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
const STORAGE_SHOW_CLOSED = 'tglp_show_closed';

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
    const [showClosedPositions, setShowClosedPositions] = useState(false);

    const serverPollIntervalSec = Math.max(1, Number(data?.poll_interval_sec || 1));
    const pollIntervalSec = Math.max(1, Number(pollOverrideSec || serverPollIntervalSec || 1));
    const updatedAt = data?.updated_at;

    const walletAddress = data?.wallet?.address || '';
    const bnbBalance = data?.wallet?.bnb_balance || '0.000000';
    const bnbUsd = data?.wallet?.bnb_usd;
    const summary = data?.summary;
    const positions = data?.positions || [];

    const visiblePositions = useMemo(() => {
        if (showClosedPositions) return positions;
        return positions.filter((p) => p?.has_liquidity !== false);
    }, [positions, showClosedPositions]);

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

        setShowClosedPositions(storage.get(STORAGE_SHOW_CLOSED) === '1');
    }, []);

    useEffect(() => {
        const isDark = theme === 'dark';
        document.documentElement.classList.toggle('dark', isDark);
        storage.set(STORAGE_THEME, isDark ? 'dark' : 'light');

        const tg = getTelegramWebApp();
        try {
            tg?.setHeaderColor?.(isDark ? '#0b0c0f' : '#f8fafc');
            tg?.setBackgroundColor?.(isDark ? '#0b0c0f' : '#f8fafc');
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
        const n = Number(pollDraftSec);
        if (!Number.isFinite(n)) return;
        const v = Math.max(1, Math.min(60, Math.floor(n)));
        setPollOverrideSec(v);
        storage.set(STORAGE_POLL_SEC, String(v));
    };

    const clearPollOverride = () => {
        setPollOverrideSec(null);
        setPollDraftSec('');
        storage.remove(STORAGE_POLL_SEC);
    };

    const setQuickPoll = (sec) => {
        const v = Math.max(1, Math.min(60, Math.floor(Number(sec) || 1)));
        setPollOverrideSec(v);
        storage.set(STORAGE_POLL_SEC, String(v));
        setPollDraftSec(String(v));
    };

    const toggleTheme = () => setTheme((t) => (t === 'dark' ? 'light' : 'dark'));

    return (
        <div className="min-h-screen px-4 py-4 pb-[calc(16px+env(safe-area-inset-bottom))]">
            <header className="mb-4">
                <div className="flex items-center justify-between gap-3">
                    <div className="flex items-center gap-2">
                        <div className="flex h-9 w-9 items-center justify-center rounded-2xl bg-fuchsia-500/15 text-fuchsia-600 ring-1 ring-fuchsia-500/25 dark:bg-fuchsia-500/15 dark:text-fuchsia-300 dark:ring-fuchsia-500/25">
                            <Icon path={icons.bot} className="h-5 w-5" />
                        </div>
                        <div>
                            <div className="text-lg font-extrabold tracking-tight">实时仓位</div>
                            <div className="mt-0.5 text-xs text-slate-500 dark:text-white/40">
                                {walletAddress ? `钱包：${walletAddress.slice(0, 6)}...${walletAddress.slice(-4)}` : '加载钱包中...'}
                            </div>
                        </div>
                    </div>

                    <div className="flex items-center gap-2">
                        <button
                            type="button"
                            onClick={toggleTheme}
                            className="inline-flex h-9 w-9 items-center justify-center rounded-2xl border border-slate-200 bg-white/70 text-slate-700 shadow-sm hover:bg-white active:bg-white dark:border-white/10 dark:bg-white/5 dark:text-white/80"
                            aria-label="切换主题"
                        >
                            <Icon path={theme === 'dark' ? icons.sun : icons.moon} className="h-5 w-5" />
                        </button>
                        <button
                            type="button"
                            onClick={() => setSettingsOpen(true)}
                            className="inline-flex h-9 w-9 items-center justify-center rounded-2xl border border-slate-200 bg-white/70 text-slate-700 shadow-sm hover:bg-white active:bg-white dark:border-white/10 dark:bg-white/5 dark:text-white/80"
                            aria-label="设置"
                        >
                            <Icon path={icons.gear} className="h-5 w-5" />
                        </button>
                    </div>
                </div>

                <div className="mt-3 rounded-3xl border border-slate-200 bg-white/70 p-4 shadow-sm backdrop-blur dark:border-white/10 dark:bg-white/5">
                    <div className="flex items-start justify-between gap-4">
                        <div>
                            <div className="text-[11px] text-slate-500 dark:text-white/40">钱包总余额（涉及代币）</div>
                            <div className="mt-0.5 text-2xl font-extrabold tabular-nums text-slate-900 dark:text-fuchsia-300">
                                {formatUsd(summary?.wallet_usd)}
                            </div>
                            <div className="mt-1 text-[11px] text-slate-500 dark:text-white/40 tabular-nums">
                                {bnbBalance} BNB{typeof bnbUsd === 'number' ? ` ≈ ${formatUsd(bnbUsd)}` : ''}
                            </div>
                        </div>
                        <div className="text-right">
                            <div className="text-[11px] text-slate-500 dark:text-white/40">自动刷新</div>
                            <div className="text-sm font-semibold tabular-nums">{pollIntervalSec}s</div>
                            <div className="mt-1 text-[11px] text-slate-500 dark:text-white/40">{formatRelativeTime(updatedAt)}</div>
                        </div>
                    </div>

                    <div className="mt-3 grid grid-cols-3 gap-2">
                        <div className="rounded-2xl border border-slate-200 bg-slate-50 p-3 dark:border-white/10 dark:bg-black/20">
                            <div className="text-[11px] text-slate-500 dark:text-white/40">仓位</div>
                            <div className="mt-0.5 text-sm font-semibold tabular-nums text-sky-700 dark:text-sky-300">
                                {formatUsd(summary?.position_usd)}
                            </div>
                        </div>
                        <div className="rounded-2xl border border-slate-200 bg-slate-50 p-3 dark:border-white/10 dark:bg-black/20">
                            <div className="text-[11px] text-slate-500 dark:text-white/40">手续费</div>
                            <div className="mt-0.5 text-sm font-semibold tabular-nums text-emerald-700 dark:text-emerald-300">
                                {formatUsd(summary?.fee_usd)}
                            </div>
                        </div>
                        <div className="rounded-2xl border border-slate-200 bg-slate-50 p-3 dark:border-white/10 dark:bg-black/20">
                            <div className="text-[11px] text-slate-500 dark:text-white/40">总计</div>
                            <div className="mt-0.5 text-sm font-extrabold tabular-nums text-fuchsia-700 dark:text-fuchsia-300">
                                {formatUsd(summary?.total_usd)}
                            </div>
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
                <div className="rounded-2xl border border-slate-200 bg-white/70 p-6 text-sm text-slate-500 dark:border-white/10 dark:bg-white/5 dark:text-white/60">
                    加载中...
                </div>
            ) : null}

            {!loading && data && visiblePositions.length === 0 ? (
                <div className="rounded-2xl border border-slate-200 bg-white/70 p-6 text-sm text-slate-500 dark:border-white/10 dark:bg-white/5 dark:text-white/60">
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
                    <div className="absolute inset-x-0 bottom-0 rounded-t-3xl border border-slate-200 bg-white p-4 shadow-2xl dark:border-white/10 dark:bg-[#0f1116]">
                        <div className="flex items-center justify-between">
                            <div className="text-sm font-semibold text-slate-900 dark:text-white/90">设置</div>
                            <button
                                type="button"
                                onClick={() => setSettingsOpen(false)}
                                className="inline-flex h-9 w-9 items-center justify-center rounded-2xl border border-slate-200 bg-white/70 text-slate-700 hover:bg-white active:bg-white dark:border-white/10 dark:bg-white/5 dark:text-white/80"
                                aria-label="关闭"
                            >
                                <Icon path={icons.close} className="h-5 w-5" />
                            </button>
                        </div>

                        <div className="mt-4 space-y-4">
                            <div className="rounded-2xl border border-slate-200 bg-slate-50 p-3 dark:border-white/10 dark:bg-black/20">
                                <div className="text-xs font-semibold text-slate-900 dark:text-white/80">自动刷新</div>
                                <div className="mt-0.5 text-[11px] text-slate-500 dark:text-white/40">
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
                                                    ? 'bg-fuchsia-500/15 text-fuchsia-700 ring-fuchsia-500/25 dark:text-fuchsia-300'
                                                    : 'bg-white/70 text-slate-700 ring-slate-200 hover:bg-white dark:bg-white/5 dark:text-white/70 dark:ring-white/10'
                                            }`}
                                        >
                                            {sec}s
                                        </button>
                                    ))}
                                    <button
                                        type="button"
                                        onClick={clearPollOverride}
                                        className="rounded-xl bg-white/70 px-3 py-1.5 text-xs font-semibold text-slate-700 ring-1 ring-slate-200 hover:bg-white dark:bg-white/5 dark:text-white/70 dark:ring-white/10"
                                    >
                                        跟随默认
                                    </button>
                                </div>

                                <div className="mt-3 flex items-center gap-2">
                                    <input
                                        value={pollDraftSec}
                                        onChange={(e) => setPollDraftSec(e.target.value)}
                                        inputMode="numeric"
                                        className="w-28 rounded-xl border border-slate-200 bg-white/70 px-3 py-2 text-sm text-slate-900 shadow-sm outline-none ring-0 placeholder:text-slate-400 focus:border-fuchsia-400 dark:border-white/10 dark:bg-white/5 dark:text-white/90 dark:placeholder:text-white/30"
                                        placeholder="1-60"
                                    />
                                    <button
                                        type="button"
                                        onClick={applyPollDraft}
                                        className="rounded-xl bg-fuchsia-500 px-3 py-2 text-sm font-semibold text-white shadow-sm hover:bg-fuchsia-600 active:bg-fuchsia-700"
                                    >
                                        应用
                                    </button>
                                </div>
                            </div>

                            <div className="flex items-center justify-between rounded-2xl border border-slate-200 bg-slate-50 p-3 dark:border-white/10 dark:bg-black/20">
                                <div>
                                    <div className="text-xs font-semibold text-slate-900 dark:text-white/80">显示已清仓</div>
                                    <div className="mt-0.5 text-[11px] text-slate-500 dark:text-white/40">流动性为 0 的任务</div>
                                </div>
                                <button
                                    type="button"
                                    onClick={() => {
                                        setShowClosedPositions((v) => {
                                            const next = !v;
                                            storage.set(STORAGE_SHOW_CLOSED, next ? '1' : '0');
                                            return next;
                                        });
                                    }}
                                    className={`h-8 w-14 rounded-full p-1 ring-1 transition ${
                                        showClosedPositions
                                            ? 'bg-emerald-500/20 ring-emerald-500/30'
                                            : 'bg-slate-200 ring-slate-300 dark:bg-white/10 dark:ring-white/10'
                                    }`}
                                    aria-label="切换显示已清仓"
                                >
                                    <div
                                        className={`h-6 w-6 rounded-full bg-white shadow transition ${
                                            showClosedPositions ? 'translate-x-6' : 'translate-x-0'
                                        }`}
                                    />
                                </button>
                            </div>
                        </div>
                    </div>
                </div>
            ) : null}
        </div>
    );
}
