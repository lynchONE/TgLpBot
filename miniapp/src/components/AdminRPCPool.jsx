import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
    addAdminRPCEndpoint,
    disableAdminRPCEndpointNextMonth,
    enableAdminRPCEndpoint,
    fetchAdminRPCPool,
    switchAdminRPCEndpoint,
} from '../lib/api';

function FancySelect({ value, onChange, options, placeholder = '请选择', disabled = false }) {
    const rootRef = useRef(null);
    const buttonRef = useRef(null);
    const [open, setOpen] = useState(false);
    const [highlightIndex, setHighlightIndex] = useState(0);

    const selectedIndex = useMemo(() => options.findIndex((o) => o.value === value), [options, value]);
    const selected = useMemo(() => {
        if (selectedIndex < 0) return null;
        return options[selectedIndex] || null;
    }, [options, selectedIndex]);

    const close = useCallback(() => {
        setOpen(false);
    }, []);

    const openMenu = useCallback(() => {
        if (disabled) return;
        setOpen(true);
        setHighlightIndex(Math.max(0, selectedIndex >= 0 ? selectedIndex : 0));
    }, [disabled, selectedIndex]);

    const toggle = useCallback(() => {
        if (disabled) return;
        setOpen((v) => {
            const next = !v;
            if (next) setHighlightIndex(Math.max(0, selectedIndex >= 0 ? selectedIndex : 0));
            return next;
        });
    }, [disabled, selectedIndex]);

    const commit = useCallback((idx) => {
        const opt = options[idx];
        if (!opt) return;
        onChange?.(opt.value);
        close();
        setTimeout(() => buttonRef.current?.focus?.(), 0);
    }, [options, onChange, close]);

    useEffect(() => {
        if (!open) return;
        const onPointerDown = (e) => {
            if (rootRef.current && rootRef.current.contains(e.target)) return;
            close();
        };
        document.addEventListener('pointerdown', onPointerDown);
        return () => document.removeEventListener('pointerdown', onPointerDown);
    }, [open, close]);

    return (
        <div ref={rootRef} className="relative">
            <button
                ref={buttonRef}
                type="button"
                disabled={disabled}
                aria-haspopup="listbox"
                aria-expanded={open ? 'true' : 'false'}
                onClick={toggle}
                onKeyDown={(e) => {
                    if (disabled) return;
                    if (!open) {
                        if (e.key === 'ArrowDown' || e.key === 'ArrowUp' || e.key === 'Enter' || e.key === ' ') {
                            e.preventDefault();
                            openMenu();
                        }
                        return;
                    }

                    if (e.key === 'Escape') {
                        e.preventDefault();
                        close();
                        return;
                    }
                    if (e.key === 'ArrowDown') {
                        e.preventDefault();
                        setHighlightIndex((i) => Math.min(options.length - 1, i + 1));
                        return;
                    }
                    if (e.key === 'ArrowUp') {
                        e.preventDefault();
                        setHighlightIndex((i) => Math.max(0, i - 1));
                        return;
                    }
                    if (e.key === 'Enter') {
                        e.preventDefault();
                        commit(highlightIndex);
                    }
                }}
                className={`w-full rounded-xl border px-3 py-2 text-sm shadow-sm outline-none ring-1 ring-transparent transition flex items-center justify-between gap-2 ${disabled
                    ? 'cursor-not-allowed border-zinc-200 bg-zinc-200/60 text-zinc-500 dark:border-white/10 dark:bg-white/5 dark:text-white/30'
                    : 'border-zinc-200 bg-white/60 text-zinc-900 hover:bg-white/80 focus:ring-emerald-500/40 dark:border-white/10 dark:bg-white/5 dark:text-white dark:hover:bg-white/10'
                    }`}
            >
                <span className="min-w-0 truncate">{selected?.label || placeholder}</span>
                <span className={`shrink-0 transition ${open ? 'rotate-180' : ''}`} aria-hidden="true">
                    <svg width="16" height="16" viewBox="0 0 24 24" fill="none">
                        <path
                            d="M7 10l5 5 5-5"
                            stroke="currentColor"
                            strokeWidth="2"
                            strokeLinecap="round"
                            strokeLinejoin="round"
                        />
                    </svg>
                </span>
            </button>

            {open && (
                <div
                    role="listbox"
                    className="absolute z-50 mt-1 w-full overflow-hidden rounded-xl border border-zinc-200 bg-white/95 shadow-lg backdrop-blur-md dark:border-white/10 dark:bg-zinc-950/90"
                >
                    {(options || []).map((opt, idx) => {
                        const selectedNow = opt.value === value;
                        const highlighted = idx === highlightIndex;
                        return (
                            <button
                                key={String(opt.value)}
                                type="button"
                                role="option"
                                aria-selected={selectedNow ? 'true' : 'false'}
                                onMouseEnter={() => setHighlightIndex(idx)}
                                onClick={() => commit(idx)}
                                className={`w-full px-3 py-2 text-left text-sm flex items-center justify-between gap-2 transition ${highlighted
                                    ? 'bg-zinc-100 text-zinc-900 dark:bg-white/10 dark:text-white'
                                    : 'bg-transparent text-zinc-800 hover:bg-zinc-100 dark:text-white/80 dark:hover:bg-white/10'
                                    }`}
                            >
                                <span className="min-w-0 truncate">{opt.label}</span>
                                {selectedNow && (
                                    <span className="shrink-0 text-emerald-600 dark:text-emerald-400" aria-hidden="true">
                                        ✓
                                    </span>
                                )}
                            </button>
                        );
                    })}
                </div>
            )}
        </div>
    );
}

function formatTime(ts) {
    if (!ts) return '--';
    const d = new Date(ts);
    if (Number.isNaN(d.getTime())) return String(ts);
    return d.toLocaleString();
}

function formatChain(chain) {
    const v = String(chain || '').toLowerCase();
    if (v === 'bsc') return 'BSC';
    if (v === 'base') return 'Base';
    return String(chain || '--');
}

function formatTransport(transport) {
    const v = String(transport || '').toLowerCase();
    if (v === 'http') return 'HTTP';
    if (v === 'ws') return 'WebSocket';
    return String(transport || '--');
}

function formatSource(source) {
    const v = String(source || '').toLowerCase();
    if (v === 'pool') return '节点池';
    if (v === 'env') return '环境变量';
    if (v === 'none') return '无';
    return String(source || '--');
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
            setAddError('请填写 URL');
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
            onNotice?.('已添加 RPC 节点');
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
            onNotice?.('已切换当前节点');
            load();
        } catch (e) {
            onNotice?.(String(e?.message || e));
        }
    }, [apiBaseUrl, initData, hasInitData, onNotice, load]);

    const handleDisableNextMonth = useCallback(async (endpointId) => {
        if (!hasInitData) return;
        try {
            await disableAdminRPCEndpointNextMonth({ apiBaseUrl, initData, endpointId });
            onNotice?.('已禁用到下月');
            load();
        } catch (e) {
            onNotice?.(String(e?.message || e));
        }
    }, [apiBaseUrl, initData, hasInitData, onNotice, load]);

    const handleEnable = useCallback(async (endpointId) => {
        if (!hasInitData) return;
        try {
            await enableAdminRPCEndpoint({ apiBaseUrl, initData, endpointId });
            onNotice?.('节点已启用');
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
                        <div className="text-sm font-semibold text-zinc-900 dark:text-white/90">RPC 节点池</div>
                        <div className="mt-0.5 text-[11px] text-zinc-500 dark:text-white/40">
                            当触发额度/频控时自动切换（cu limit exceeded / quota exceeded / HTTP 429）
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
                        {loading ? '加载中...' : '刷新'}
                    </button>
                </div>

                {error && (
                    <div className="mt-3 rounded-xl border border-red-500/30 bg-red-500/10 p-3 text-xs text-red-700 dark:text-red-200">
                        {error}
                    </div>
                )}
            </div>

            <div className="rounded-2xl border border-zinc-200 bg-white/40 backdrop-blur-md p-4 shadow-sm dark:border-white/10 dark:bg-white/5 dark:shadow-none">
                <div className="text-sm font-semibold text-zinc-900 dark:text-white/90 mb-3">添加节点</div>
                <div className="grid grid-cols-1 gap-3">
                    <div className="grid grid-cols-2 gap-3">
                        <label className="space-y-1">
                            <div className="text-[11px] font-semibold text-zinc-600 dark:text-white/60">链</div>
                            <FancySelect
                                value={draft.chain}
                                onChange={(v) => setDraft((p) => ({ ...p, chain: v }))}
                                options={[
                                    { value: 'bsc', label: 'BSC' },
                                    { value: 'base', label: 'Base' },
                                ]}
                            />
                        </label>
                        <label className="space-y-1">
                            <div className="text-[11px] font-semibold text-zinc-600 dark:text-white/60">类型</div>
                            <FancySelect
                                value={draft.transport}
                                onChange={(v) => setDraft((p) => ({ ...p, transport: v }))}
                                options={[
                                    { value: 'http', label: 'HTTP' },
                                    { value: 'ws', label: 'WebSocket' },
                                ]}
                            />
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
                        设为当前
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
                            {adding ? '添加中...' : '添加'}
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
                                {formatChain(g.chain)} / {formatTransport(g.transport)}
                            </div>
                            <div className="mt-0.5 text-[11px] text-zinc-500 dark:text-white/40">
                                当前：{maskUrl(g.effective_url, g.effective_url_masked)}（{formatSource(g.effective_source)}）
                            </div>
                            {g.env_url && (
                                <div className="mt-0.5 text-[11px] text-zinc-400 dark:text-white/30">
                                    环境变量：{maskUrl(g.env_url, g.env_url_masked)}
                                </div>
                            )}
                        </div>
                    </div>

                    <div className="mt-3 space-y-2">
                        {(g.endpoints || []).length === 0 && (
                            <div className="rounded-xl border border-zinc-200 bg-white/40 backdrop-blur-md p-3 text-xs text-zinc-500 dark:border-white/10 dark:bg-white/5 dark:text-white/60">
                                节点池为空（如配置了环境变量，会使用环境变量兜底）
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
                                                    当前
                                                </span>
                                            )}
                                            {isUnavailable(ep) ? (
                                                <span className="rounded-lg bg-red-500/10 px-2 py-0.5 text-[11px] font-semibold text-red-700 ring-1 ring-red-500/25 dark:text-red-200">
                                                    不可用
                                                </span>
                                            ) : (
                                                <span className="rounded-lg bg-zinc-500/10 px-2 py-0.5 text-[11px] font-semibold text-zinc-700 ring-1 ring-zinc-500/25 dark:text-white/70">
                                                    可用
                                                </span>
                                            )}
                                        </div>

                                        <div className="mt-1 grid grid-cols-2 gap-x-3 gap-y-1 text-[11px] text-zinc-500 dark:text-white/40">
                                            <div>延迟：{Number(ep.last_latency_ms || 0)} ms</div>
                                            <div>连续失败：{Number(ep.consecutive_failures || 0)}</div>
                                            <div>最近检测：{formatTime(ep.last_checked_at)}</div>
                                            <div>最近成功：{formatTime(ep.last_success_at)}</div>
                                            {ep.disabled_until && (
                                                <div className="col-span-2">
                                                    禁用至：{formatTime(ep.disabled_until)} {ep.disabled_reason ? `(${ep.disabled_reason})` : ''}
                                                </div>
                                            )}
                                            {ep.last_error && (
                                                <div className="col-span-2 text-red-700/80 dark:text-red-200/80">
                                                    最近错误：{String(ep.last_error)}
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
                                            切换
                                        </button>
                                        <button
                                            type="button"
                                            onClick={() => handleDisableNextMonth(ep.id)}
                                            className="rounded-xl px-3 py-2 text-xs font-semibold ring-1 bg-red-500/10 text-red-700 ring-red-500/20 hover:bg-red-500/15 dark:text-red-200"
                                        >
                                            禁用到下月
                                        </button>
                                        {isUnavailable(ep) && (
                                            <button
                                                type="button"
                                                onClick={() => handleEnable(ep.id)}
                                                className="rounded-xl px-3 py-2 text-xs font-semibold ring-1 bg-emerald-500/10 text-emerald-700 ring-emerald-500/20 hover:bg-emerald-500/15 dark:text-emerald-200"
                                            >
                                                启用
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
