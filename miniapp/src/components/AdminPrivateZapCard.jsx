import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { fetchAdminPrivateZap, invalidateAdminPrivateZap } from '../lib/api';

function formatChain(chain) {
    const v = String(chain || '').toLowerCase();
    if (v === 'bsc') return 'BSC';
    if (v === 'base') return 'Base';
    return String(chain || '--').toUpperCase();
}

export default function AdminPrivateZapCard({ apiBaseUrl, initData, hasInitData, onNotice }) {
    const [chains, setChains] = useState([]);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState('');
    const [invalidatingChain, setInvalidatingChain] = useState('');
    const [lastResult, setLastResult] = useState(null);

    const loadChains = useCallback(async () => {
        if (!hasInitData) return;
        setLoading(true);
        setError('');
        try {
            const data = await fetchAdminPrivateZap({ apiBaseUrl, initData });
            setChains(Array.isArray(data?.chains) ? data.chains : []);
        } catch (e) {
            setError(String(e?.message || e));
        } finally {
            setLoading(false);
        }
    }, [apiBaseUrl, initData, hasInitData]);

    useEffect(() => {
        loadChains();
    }, [loadChains]);

    const handleInvalidate = useCallback(async (chain) => {
        const normalized = String(chain || '').trim().toLowerCase();
        if (!normalized) return;
        if (typeof window !== 'undefined') {
            const ok = window.confirm(`Invalidate existing Private Zap bindings on ${formatChain(normalized)}? Users will redeploy on next open.`);
            if (!ok) return;
        }
        setInvalidatingChain(normalized);
        setError('');
        try {
            const data = await invalidateAdminPrivateZap({ apiBaseUrl, initData, chain: normalized });
            setChains(Array.isArray(data?.chains) ? data.chains : []);
            setLastResult(data?.result || null);
            const clearedBindings = Number(data?.result?.cleared_bindings || 0);
            const clearedCacheKeys = Number(data?.result?.cleared_cache_keys || 0);
            onNotice?.(
                `${formatChain(normalized)} Private Zap invalidated: ${clearedBindings} bindings cleared, ${clearedCacheKeys} cache keys deleted`,
                'success',
            );
        } catch (e) {
            const msg = String(e?.message || e);
            setError(msg);
            onNotice?.(msg, 'error');
        } finally {
            setInvalidatingChain('');
        }
    }, [apiBaseUrl, initData, onNotice]);

    const chainList = useMemo(() => Array.isArray(chains) ? chains : [], [chains]);

    return (
        <div className="rounded-2xl border border-zinc-200 bg-white/40 p-4 shadow-sm backdrop-blur-md dark:border-white/10 dark:bg-white/5 dark:shadow-none">
            <div className="flex items-center justify-between gap-3">
                <div>
                    <div className="text-sm font-semibold text-zinc-900 dark:text-white/90">Private Zap</div>
                    <div className="mt-1 text-[11px] text-zinc-500 dark:text-white/45">
                        Invalidate bindings by chain. Users will deploy and bind a fresh Private Zap on the next open.
                    </div>
                </div>
                <button
                    type="button"
                    onClick={loadChains}
                    disabled={loading}
                    className={`rounded-xl px-3 py-2 text-xs font-semibold ring-1 transition ${loading
                        ? 'cursor-not-allowed bg-zinc-100 text-zinc-400 ring-zinc-200 dark:bg-white/5 dark:text-white/30 dark:ring-white/10'
                        : 'bg-white/80 text-zinc-700 ring-zinc-200 hover:bg-white dark:bg-white/5 dark:text-white/75 dark:ring-white/10 dark:hover:bg-white/10'
                        }`}
                >
                    Refresh
                </button>
            </div>

            {error && (
                <div className="mt-3 rounded-xl border border-red-500/30 bg-red-500/10 px-3 py-2 text-xs text-red-700 dark:text-red-200">
                    {error}
                </div>
            )}

            {lastResult && (
                <div className="mt-3 rounded-xl border border-emerald-500/20 bg-emerald-500/10 px-3 py-2 text-xs text-emerald-700 dark:text-emerald-200">
                    Last action: {formatChain(lastResult.chain)}, {Number(lastResult.cleared_bindings || 0)} bindings cleared, {Number(lastResult.cleared_cache_keys || 0)} cache keys deleted.
                </div>
            )}

            <div className="mt-3 space-y-2">
                {loading && chainList.length === 0 && (
                    <div className="rounded-xl border border-zinc-200 bg-white/60 px-3 py-3 text-xs text-zinc-500 dark:border-white/10 dark:bg-white/5 dark:text-white/45">
                        Loading...
                    </div>
                )}

                {!loading && chainList.length === 0 && (
                    <div className="rounded-xl border border-zinc-200 bg-white/60 px-3 py-3 text-xs text-zinc-500 dark:border-white/10 dark:bg-white/5 dark:text-white/45">
                        No enabled chains available.
                    </div>
                )}

                {chainList.map((chain) => {
                    const normalized = String(chain || '').trim().toLowerCase();
                    const busy = invalidatingChain === normalized;
                    return (
                        <div
                            key={normalized}
                            className="flex items-center justify-between gap-3 rounded-xl border border-zinc-200 bg-white/60 px-3 py-3 dark:border-white/10 dark:bg-white/5"
                        >
                            <div>
                                <div className="text-sm font-semibold text-zinc-900 dark:text-white/90">
                                    {formatChain(normalized)}
                                </div>
                                <div className="mt-1 text-[11px] text-zinc-500 dark:text-white/45">
                                    Clear stored bound address and Redis cache for this chain.
                                </div>
                            </div>
                            <button
                                type="button"
                                onClick={() => handleInvalidate(normalized)}
                                disabled={busy}
                                className={`rounded-xl px-3 py-2 text-xs font-semibold ring-1 transition ${busy
                                    ? 'cursor-not-allowed bg-amber-500/10 text-amber-700/70 ring-amber-500/20 dark:text-amber-200/60'
                                    : 'bg-amber-500/15 text-amber-700 ring-amber-500/25 hover:bg-amber-500/20 dark:text-amber-200'
                                    }`}
                            >
                                {busy ? 'Invalidating...' : 'Invalidate'}
                            </button>
                        </div>
                    );
                })}
            </div>
        </div>
    );
}
