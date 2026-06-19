import { useCallback, useEffect, useMemo, useState } from 'react';
import { fetchAdminPrivateZap, invalidateAdminPrivateZap } from '../lib/api';
import ConfirmDialog from './ConfirmDialog.jsx';

function formatChain(chain) {
    const v = String(chain || '').toLowerCase();
    if (v === 'bsc') return 'BSC';
    if (v === 'base') return 'Base';
    return String(chain || '--').toUpperCase();
}

function formatKind(kind) {
    const v = String(kind || '').toLowerCase();
    if (v === 'atomic_increase_zap') return 'Atomic Increase Zap';
    if (v === 'zap_simple') return 'Zap Simple';
    return String(kind || '--');
}

const DEFAULT_KINDS = ['zap_simple', 'atomic_increase_zap'];

export default function AdminPrivateZapCard({ apiBaseUrl, initData, hasInitData, onNotice }) {
    const [chains, setChains] = useState([]);
    const [kinds, setKinds] = useState(DEFAULT_KINDS);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState('');
    const [invalidatingKey, setInvalidatingKey] = useState('');
    const [lastResult, setLastResult] = useState(null);
    const [confirmInvalidate, setConfirmInvalidate] = useState(null);

    const loadChains = useCallback(async () => {
        if (!hasInitData) return;
        setLoading(true);
        setError('');
        try {
            const data = await fetchAdminPrivateZap({ apiBaseUrl, initData });
            setChains(Array.isArray(data?.chains) ? data.chains : []);
            setKinds(Array.isArray(data?.kinds) && data.kinds.length > 0 ? data.kinds : DEFAULT_KINDS);
        } catch (e) {
            setError(String(e?.message || e));
        } finally {
            setLoading(false);
        }
    }, [apiBaseUrl, initData, hasInitData]);

    useEffect(() => {
        loadChains();
    }, [loadChains]);

    const handleInvalidate = useCallback(async (chain, kind) => {
        const normalizedChain = String(chain || '').trim().toLowerCase();
        const normalizedKind = String(kind || '').trim().toLowerCase();
        if (!normalizedChain || !normalizedKind) return;
        setConfirmInvalidate({ chain: normalizedChain, kind: normalizedKind });
    }, []);

    const confirmInvalidateAction = useCallback(async () => {
        const normalizedChain = String(confirmInvalidate?.chain || '').trim().toLowerCase();
        const normalizedKind = String(confirmInvalidate?.kind || '').trim().toLowerCase();
        if (!normalizedChain || !normalizedKind) return;
        setConfirmInvalidate(null);
        const busyKey = `${normalizedChain}:${normalizedKind}`;
        setInvalidatingKey(busyKey);
        setError('');
        try {
            const data = await invalidateAdminPrivateZap({ apiBaseUrl, initData, chain: normalizedChain, kind: normalizedKind });
            setChains(Array.isArray(data?.chains) ? data.chains : []);
            setKinds(Array.isArray(data?.kinds) && data.kinds.length > 0 ? data.kinds : DEFAULT_KINDS);
            setLastResult(data?.result || null);
            const clearedBindings = Number(data?.result?.cleared_bindings || 0);
            const clearedCacheKeys = Number(data?.result?.cleared_cache_keys || 0);
            onNotice?.(
                `${formatChain(normalizedChain)} ${formatKind(normalizedKind)} invalidated: ${clearedBindings} bindings cleared, ${clearedCacheKeys} cache keys deleted`,
                'success',
            );
        } catch (e) {
            const msg = String(e?.message || e);
            setError(msg);
            onNotice?.(msg, 'error');
        } finally {
            setInvalidatingKey('');
        }
    }, [apiBaseUrl, confirmInvalidate, initData, onNotice]);

    const chainList = useMemo(() => Array.isArray(chains) ? chains : [], [chains]);
    const kindList = useMemo(() => Array.isArray(kinds) && kinds.length > 0 ? kinds : DEFAULT_KINDS, [kinds]);

    return (
        <div className="rounded-2xl border border-zinc-200/70 bg-white/65 p-4 shadow-sm backdrop-blur-sm dark:border-white/10 dark:bg-[#0f1116]/80 dark:shadow-none">
            <div className="flex items-center justify-between gap-3">
                <div>
                    <div className="text-[10px] font-semibold uppercase tracking-[0.18em] text-zinc-500 dark:text-white/45">PRIVATE ZAP</div>
                    <div className="mt-0.5 text-[15px] font-black text-zinc-900 dark:text-white">合约绑定</div>
                    <div className="mt-1 text-[11px] text-zinc-500 dark:text-white/45">
                        Invalidate bindings by chain and contract kind. The selected kind will redeploy on next use.
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
                    Last action: {formatChain(lastResult.chain)}, {formatKind(lastResult.kind)}, {Number(lastResult.cleared_bindings || 0)} bindings cleared, {Number(lastResult.cleared_cache_keys || 0)} cache keys deleted.
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
                    const normalizedChain = String(chain || '').trim().toLowerCase();
                    return (
                        <div
                            key={normalizedChain}
                            className="rounded-xl border border-zinc-200 bg-white/60 px-3 py-3 dark:border-white/10 dark:bg-white/5"
                        >
                            <div className="flex items-center justify-between gap-3">
                                <div>
                                    <div className="text-sm font-semibold text-zinc-900 dark:text-white/90">
                                        {formatChain(normalizedChain)}
                                    </div>
                                    <div className="mt-1 text-[11px] text-zinc-500 dark:text-white/45">
                                        Clear stored bound address and Redis cache for the selected contract kind.
                                    </div>
                                </div>
                            </div>
                            <div className="mt-3 flex flex-wrap gap-2">
                                {kindList.map((kind) => {
                                    const normalizedKind = String(kind || '').trim().toLowerCase();
                                    const busy = invalidatingKey === `${normalizedChain}:${normalizedKind}`;
                                    return (
                                        <button
                                            key={`${normalizedChain}:${normalizedKind}`}
                                            type="button"
                                            onClick={() => handleInvalidate(normalizedChain, normalizedKind)}
                                            disabled={busy}
                                            className={`rounded-xl px-3 py-2 text-xs font-semibold ring-1 transition ${busy
                                                ? 'cursor-not-allowed bg-amber-500/10 text-amber-700/70 ring-amber-500/20 dark:text-amber-200/60'
                                                : 'bg-amber-500/15 text-amber-700 ring-amber-500/25 hover:bg-amber-500/20 dark:text-amber-200'
                                                }`}
                                        >
                                            {busy ? `Invalidating ${formatKind(normalizedKind)}...` : `Invalidate ${formatKind(normalizedKind)}`}
                                        </button>
                                    );
                                })}
                            </div>
                        </div>
                    );
                })}
            </div>
            <ConfirmDialog
                open={Boolean(confirmInvalidate)}
                title="Invalidate Private Zap"
                message={`Invalidate ${formatKind(confirmInvalidate?.kind)} bindings on ${formatChain(confirmInvalidate?.chain)}? Users will redeploy this contract kind on next use.`}
                confirmText="Invalidate"
                cancelText="Cancel"
                danger
                loading={Boolean(invalidatingKey)}
                onConfirm={confirmInvalidateAction}
                onCancel={() => setConfirmInvalidate(null)}
            />
        </div>
    );
}
