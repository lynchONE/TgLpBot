import React, { useCallback, useEffect, useMemo, useState } from 'react';
import {
    addAdminRPCEndpoint,
    disableAdminRPCEndpointNextMonth,
    enableAdminRPCEndpoint,
    fetchAdminRPCPool,
    switchAdminRPCEndpoint,
} from '../lib/api';

function formatTime(ts) {
    if (!ts) return '--';
    const d = new Date(ts);
    if (Number.isNaN(d.getTime())) return String(ts);
    return d.toLocaleString();
}

function isUnavailable(endpoint) {
    return String(endpoint?.status || '') === 'unavailable';
}

function maskUrl(url, fallbackMasked) {
    const s = String(url || '').trim();
    if (!s) return '--';
    return String(fallbackMasked || s);
}

export default function AdminRPCPool({ apiBaseUrl, initData, hasInitData, pollIntervalSec = 15, onNotice }) {
    const [data, setData] = useState(null);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState('');

    const [adding, setAdding] = useState(false);
    const [addError, setAddError] = useState('');
    const [draft, setDraft] = useState({
        chain: 'bsc',
        transport: 'http',
        url: '',
        setCurrent: false,
    });

    const groups = useMemo(() => data?.groups || [], [data]);

    const load = useCallback(async () => {
        if (!hasInitData) return;
        setLoading(true);
        setError('');
        try {
            const resp = await fetchAdminRPCPool({ apiBaseUrl, initData });
            setData(resp);
        } catch (e) {
            setError(String(e?.message || e));
        } finally {
            setLoading(false);
        }
    }, [apiBaseUrl, initData, hasInitData]);

    useEffect(() => {
        load();
    }, [load]);

    useEffect(() => {
        if (!hasInitData) return;
        const t = setInterval(() => {
            load();
        }, Math.max(3, Number(pollIntervalSec) || 15) * 1000);
        return () => clearInterval(t);
    }, [hasInitData, pollIntervalSec, load]);

    const handleAdd = useCallback(async () => {
        if (!hasInitData) return;
        const url = String(draft.url || '').trim();
        if (!url) {
            setAddError('URL required');
            return;
        }
        setAdding(true);
        setAddError('');
        try {
            await addAdminRPCEndpoint({
                apiBaseUrl,
                initData,
                chain: draft.chain,
                transport: draft.transport,
                url,
                setCurrent: Boolean(draft.setCurrent),
            });
            setDraft((prev) => ({ ...prev, url: '', setCurrent: false }));
            onNotice?.('RPC endpoint added');
            load();
        } catch (e) {
            setAddError(String(e?.message || e));
        } finally {
            setAdding(false);
        }
    }, [apiBaseUrl, initData, hasInitData, draft, onNotice, load]);

    const handleSwitch = useCallback(async (endpointId) => {
        if (!hasInitData) return;
        try {
            await switchAdminRPCEndpoint({ apiBaseUrl, initData, endpointId });
            onNotice?.('Switched current endpoint');
            load();
        } catch (e) {
            onNotice?.(String(e?.message || e));
        }
    }, [apiBaseUrl, initData, hasInitData, onNotice, load]);

    const handleDisableNextMonth = useCallback(async (endpointId) => {
        if (!hasInitData) return;
        try {
            await disableAdminRPCEndpointNextMonth({ apiBaseUrl, initData, endpointId });
            onNotice?.('Disabled until next month');
            load();
        } catch (e) {
            onNotice?.(String(e?.message || e));
        }
    }, [apiBaseUrl, initData, hasInitData, onNotice, load]);

    const handleEnable = useCallback(async (endpointId) => {
        if (!hasInitData) return;
        try {
            await enableAdminRPCEndpoint({ apiBaseUrl, initData, endpointId });
            onNotice?.('Endpoint enabled');
            load();
        } catch (e) {
            onNotice?.(String(e?.message || e));
        }
    }, [apiBaseUrl, initData, hasInitData, onNotice, load]);

    return (
        <div className="space-y-4">
            <div className="rounded-2xl border border-zinc-200 bg-white/40 backdrop-blur-md p-4 shadow-sm dark:border-white/10 dark:bg-white/5 dark:shadow-none">
                <div className="flex items-start justify-between gap-3">
                    <div>
                        <div className="text-sm font-semibold text-zinc-900 dark:text-white/90">RPC Pool</div>
                        <div className="mt-0.5 text-[11px] text-zinc-500 dark:text-white/40">
                            Auto failover on quota exhausted (cu limit exceeded / quota exceeded / HTTP 429)
                        </div>
                    </div>
                    <button
                        type="button"
                        onClick={load}
                        disabled={loading}
                        className={`shrink-0 rounded-xl px-3 py-2 text-xs font-semibold ring-1 transition ${loading
                            ? 'cursor-not-allowed bg-zinc-200 text-zinc-500 ring-zinc-200 dark:bg-white/10 dark:text-white/30 dark:ring-white/10'
                            : 'bg-zinc-100 text-zinc-700 ring-zinc-200 hover:bg-zinc-200 dark:bg-white/5 dark:text-white/80 dark:ring-white/10 dark:hover:bg-white/10'
                            }`}
                    >
                        {loading ? 'Loading...' : 'Refresh'}
                    </button>
                </div>

                {error && (
                    <div className="mt-3 rounded-xl border border-red-500/30 bg-red-500/10 p-3 text-xs text-red-700 dark:text-red-200">
                        {error}
                    </div>
                )}
            </div>

            <div className="rounded-2xl border border-zinc-200 bg-white/40 backdrop-blur-md p-4 shadow-sm dark:border-white/10 dark:bg-white/5 dark:shadow-none">
                <div className="text-sm font-semibold text-zinc-900 dark:text-white/90 mb-3">Add Endpoint</div>
                <div className="grid grid-cols-1 gap-3">
                    <div className="grid grid-cols-2 gap-3">
                        <label className="space-y-1">
                            <div className="text-[11px] font-semibold text-zinc-600 dark:text-white/60">Chain</div>
                            <select
                                value={draft.chain}
                                onChange={(e) => setDraft((p) => ({ ...p, chain: e.target.value }))}
                                className="w-full rounded-xl border border-zinc-200 bg-white/60 px-3 py-2 text-sm text-zinc-900 shadow-sm outline-none focus:border-emerald-500 dark:border-white/10 dark:bg-white/5 dark:text-white"
                            >
                                <option value="bsc">bsc</option>
                                <option value="base">base</option>
                            </select>
                        </label>
                        <label className="space-y-1">
                            <div className="text-[11px] font-semibold text-zinc-600 dark:text-white/60">Transport</div>
                            <select
                                value={draft.transport}
                                onChange={(e) => setDraft((p) => ({ ...p, transport: e.target.value }))}
                                className="w-full rounded-xl border border-zinc-200 bg-white/60 px-3 py-2 text-sm text-zinc-900 shadow-sm outline-none focus:border-emerald-500 dark:border-white/10 dark:bg-white/5 dark:text-white"
                            >
                                <option value="http">http</option>
                                <option value="ws">ws</option>
                            </select>
                        </label>
                    </div>
                    <label className="space-y-1">
                        <div className="text-[11px] font-semibold text-zinc-600 dark:text-white/60">URL</div>
                        <input
                            value={draft.url}
                            onChange={(e) => setDraft((p) => ({ ...p, url: e.target.value }))}
                            placeholder={draft.transport === 'ws' ? 'wss://...' : 'https://...'}
                            className="w-full rounded-xl border border-zinc-200 bg-white/60 px-3 py-2 text-sm text-zinc-900 shadow-sm outline-none focus:border-emerald-500 dark:border-white/10 dark:bg-white/5 dark:text-white"
                        />
                    </label>
                    <label className="flex items-center gap-2 text-xs text-zinc-600 dark:text-white/60">
                        <input
                            type="checkbox"
                            checked={Boolean(draft.setCurrent)}
                            onChange={(e) => setDraft((p) => ({ ...p, setCurrent: e.target.checked }))}
                        />
                        Set as current
                    </label>

                    {addError && (
                        <div className="rounded-xl border border-red-500/30 bg-red-500/10 p-3 text-xs text-red-700 dark:text-red-200">
                            {addError}
                        </div>
                    )}

                    <div className="flex justify-end">
                        <button
                            type="button"
                            onClick={handleAdd}
                            disabled={adding}
                            className={`rounded-xl px-4 py-2 text-sm font-semibold transition ${adding
                                ? 'cursor-not-allowed bg-zinc-300 text-zinc-500 dark:bg-white/10 dark:text-white/30'
                                : 'bg-emerald-600 text-white hover:bg-emerald-500 dark:bg-emerald-500 dark:hover:bg-emerald-400'
                                }`}
                        >
                            {adding ? 'Adding...' : 'Add'}
                        </button>
                    </div>
                </div>
            </div>

            {groups.map((g) => (
                <div
                    key={`${g.chain}:${g.transport}`}
                    className="rounded-2xl border border-zinc-200 bg-white/40 backdrop-blur-md p-4 shadow-sm dark:border-white/10 dark:bg-white/5 dark:shadow-none"
                >
                    <div className="flex items-start justify-between gap-3">
                        <div className="min-w-0">
                            <div className="text-sm font-semibold text-zinc-900 dark:text-white/90">
                                {String(g.chain || '--')} / {String(g.transport || '--')}
                            </div>
                            <div className="mt-0.5 text-[11px] text-zinc-500 dark:text-white/40">
                                Current: {maskUrl(g.effective_url, g.effective_url_masked)} ({String(g.effective_source || '--')})
                            </div>
                            {g.env_url && (
                                <div className="mt-0.5 text-[11px] text-zinc-400 dark:text-white/30">
                                    Env: {maskUrl(g.env_url, g.env_url_masked)}
                                </div>
                            )}
                        </div>
                    </div>

                    <div className="mt-3 space-y-2">
                        {(g.endpoints || []).length === 0 && (
                            <div className="rounded-xl border border-zinc-200 bg-white/40 backdrop-blur-md p-3 text-xs text-zinc-500 dark:border-white/10 dark:bg-white/5 dark:text-white/60">
                                No endpoints in pool (using env fallback if configured)
                            </div>
                        )}

                        {(g.endpoints || []).map((ep) => (
                            <div
                                key={ep.id}
                                className="rounded-2xl border border-zinc-200 bg-white/60 p-3 shadow-sm dark:border-white/10 dark:bg-white/5"
                            >
                                <div className="flex items-start justify-between gap-3">
                                    <div className="min-w-0">
                                        <div className="flex flex-wrap items-center gap-2">
                                            <div className="text-xs font-semibold text-zinc-900 dark:text-white/90 truncate">
                                                #{ep.id} {maskUrl(ep.url, ep.url_masked)}
                                            </div>
                                            {ep.is_current && (
                                                <span className="rounded-lg bg-emerald-500/10 px-2 py-0.5 text-[11px] font-semibold text-emerald-700 ring-1 ring-emerald-500/25 dark:text-emerald-300">
                                                    current
                                                </span>
                                            )}
                                            {isUnavailable(ep) ? (
                                                <span className="rounded-lg bg-red-500/10 px-2 py-0.5 text-[11px] font-semibold text-red-700 ring-1 ring-red-500/25 dark:text-red-200">
                                                    unavailable
                                                </span>
                                            ) : (
                                                <span className="rounded-lg bg-zinc-500/10 px-2 py-0.5 text-[11px] font-semibold text-zinc-700 ring-1 ring-zinc-500/25 dark:text-white/70">
                                                    available
                                                </span>
                                            )}
                                        </div>

                                        <div className="mt-1 grid grid-cols-2 gap-x-3 gap-y-1 text-[11px] text-zinc-500 dark:text-white/40">
                                            <div>latency: {Number(ep.last_latency_ms || 0)} ms</div>
                                            <div>fails: {Number(ep.consecutive_failures || 0)}</div>
                                            <div>checked: {formatTime(ep.last_checked_at)}</div>
                                            <div>success: {formatTime(ep.last_success_at)}</div>
                                            {ep.disabled_until && (
                                                <div className="col-span-2">
                                                    disabled_until: {formatTime(ep.disabled_until)} {ep.disabled_reason ? `(${ep.disabled_reason})` : ''}
                                                </div>
                                            )}
                                            {ep.last_error && (
                                                <div className="col-span-2 text-red-700/80 dark:text-red-200/80">
                                                    last_error: {String(ep.last_error)}
                                                </div>
                                            )}
                                        </div>
                                    </div>

                                    <div className="shrink-0 flex flex-col gap-2">
                                        <button
                                            type="button"
                                            onClick={() => handleSwitch(ep.id)}
                                            disabled={isUnavailable(ep)}
                                            className={`rounded-xl px-3 py-2 text-xs font-semibold ring-1 transition ${isUnavailable(ep)
                                                ? 'cursor-not-allowed bg-zinc-200 text-zinc-500 ring-zinc-200 dark:bg-white/10 dark:text-white/30 dark:ring-white/10'
                                                : 'bg-white text-zinc-700 ring-zinc-200 hover:bg-zinc-100 dark:bg-white/5 dark:text-white/80 dark:ring-white/10 dark:hover:bg-white/10'
                                                }`}
                                        >
                                            Switch
                                        </button>
                                        <button
                                            type="button"
                                            onClick={() => handleDisableNextMonth(ep.id)}
                                            className="rounded-xl px-3 py-2 text-xs font-semibold ring-1 bg-red-500/10 text-red-700 ring-red-500/20 hover:bg-red-500/15 dark:text-red-200"
                                        >
                                            Disable (month)
                                        </button>
                                        {isUnavailable(ep) && (
                                            <button
                                                type="button"
                                                onClick={() => handleEnable(ep.id)}
                                                className="rounded-xl px-3 py-2 text-xs font-semibold ring-1 bg-emerald-500/10 text-emerald-700 ring-emerald-500/20 hover:bg-emerald-500/15 dark:text-emerald-200"
                                            >
                                                Enable
                                            </button>
                                        )}
                                    </div>
                                </div>
                            </div>
                        ))}
                    </div>
                </div>
            ))}
        </div>
    );
}

