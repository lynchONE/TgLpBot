import React, { useCallback, useEffect, useMemo, useState } from 'react';
import {
    addAdminRPCEndpoint,
    checkAdminRPCEndpoint,
    deleteAdminRPCEndpoint,
    disableAdminRPCEndpointNextMonth,
    enableAdminRPCEndpoint,
    fetchAdminRPCPool,
    renameAdminRPCEndpoint,
    switchAdminRPCEndpoint,
} from '../lib/api';
import { getBrandTheme } from '../lib/brand';

/* ── helpers ── */

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
    if (v === 'ws') return 'WS';
    return String(transport || '--');
}

function formatSource(source) {
    const v = String(source || '').toLowerCase();
    if (v === 'db' || v === 'pool') return '节点池';
    if (v === 'env') return '环境变量';
    if (v === 'none') return '无';
    return String(source || '--');
}

function formatDisabledReason(reason) {
    const raw = String(reason || '').trim();
    if (!raw) return '';
    const v = raw.toLowerCase();
    if (v === 'quota_exhausted') return '额度用尽';
    if (v === 'health_fail') return '探活失败';
    if (v === 'manual') return '手动禁用';
    return raw;
}

function isUnavailable(ep) {
    return String(ep?.status || '') === 'unavailable';
}

function maskUrl(url, fallbackMasked) {
    const s = String(url || '').trim();
    if (!s) return '--';
    return String(fallbackMasked || s);
}

function deriveNameFromUrl(raw) {
    const s = String(raw || '').trim();
    if (!s) return '';
    try {
        return String(new URL(s).host || '').trim();
    } catch {
        return '';
    }
}

function endpointDisplayName(ep) {
    const name = String(ep?.name || '').trim();
    if (name) return name;
    const derived = deriveNameFromUrl(ep?.url);
    if (derived) return derived;
    if (ep?.id) return `#${ep.id}`;
    return '--';
}

function getBrandChipClass(brand) {
    return brand?.key === 'emerald'
        ? 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-300'
        : 'bg-[#bcff2f]/12 text-[#6f9616] dark:text-[#e3ffa0]';
}

function getBrandToneClass(brand) {
    return brand?.key === 'emerald'
        ? 'text-emerald-600 dark:text-emerald-400'
        : 'text-[#6f9616] dark:text-[#bcff2f]';
}

function getBrandOutlineButtonClass(brand) {
    return brand?.key === 'emerald'
        ? 'border border-emerald-200 text-emerald-700 bg-emerald-50 hover:bg-emerald-100 dark:border-emerald-500/20 dark:text-emerald-300 dark:bg-emerald-500/10 dark:hover:bg-emerald-500/20'
        : 'border border-[#bcff2f]/35 text-[#6f9616] bg-[#bcff2f]/10 hover:bg-[#bcff2f]/18 dark:border-[#bcff2f]/25 dark:text-[#e3ffa0] dark:bg-[#bcff2f]/10 dark:hover:bg-[#bcff2f]/16';
}

function getBrandFocusBorderClass(brand) {
    return brand?.key === 'emerald' ? 'focus:border-emerald-500' : 'focus:border-[#bcff2f]';
}

/* ── status badge ── */

function StatusDot({ available }) {
    return (
        <span className={`inline-block w-2 h-2 rounded-full shrink-0 ${available ? 'bg-emerald-500' : 'bg-red-500'}`} />
    );
}

/* ── confirm dialog ── */

function ConfirmDialog({ open, title, message, onConfirm, onCancel, danger, brand }) {
    if (!open) return null;
    return (
        <div className="fixed inset-0 z-[100] flex items-center justify-center bg-black/40 backdrop-blur-sm" onClick={onCancel}>
            <div className="mx-4 w-full max-w-sm rounded-2xl border border-zinc-200 bg-white p-5 shadow-xl dark:border-white/10 dark:bg-zinc-900" onClick={e => e.stopPropagation()}>
                <div className="text-sm font-semibold text-zinc-900 dark:text-white">{title}</div>
                <div className="mt-2 text-xs text-zinc-600 dark:text-white/60">{message}</div>
                <div className="mt-4 flex justify-end gap-2">
                    <button type="button" onClick={onCancel}
                        className="rounded-xl px-4 py-2 text-xs font-medium text-zinc-600 hover:bg-zinc-100 dark:text-white/60 dark:hover:bg-white/10 transition">
                        取消
                    </button>
                    <button type="button" onClick={onConfirm}
                        className={`rounded-xl px-4 py-2 text-xs font-semibold text-white transition ${danger ? 'bg-red-600 hover:bg-red-500' : brand.solidButtonClass}`}>
                        确认
                    </button>
                </div>
            </div>
        </div>
    );
}

/* ── icon buttons ── */

function IconBtn({ onClick, disabled, title, className, children }) {
    return (
        <button type="button" onClick={onClick} disabled={disabled} title={title}
            className={`inline-flex items-center justify-center w-8 h-8 rounded-lg transition ${disabled ? 'opacity-30 cursor-not-allowed' : 'hover:bg-zinc-100 dark:hover:bg-white/10 active:scale-95'} ${className || ''}`}>
            {children}
        </button>
    );
}

/* ── endpoint row ── */

function EndpointRow({ ep, onSwitch, onDisableNextMonth, onEnable, onRename, onDelete, onCheck, brand }) {
    const [expanded, setExpanded] = useState(false);
    const [nameDraft, setNameDraft] = useState('');
    const [renaming, setRenaming] = useState(false);
    const [checking, setChecking] = useState(false);
    const [confirmDelete, setConfirmDelete] = useState(false);

    const available = !isUnavailable(ep);
    const displayName = endpointDisplayName(ep);
    const latency = Number(ep?.last_latency_ms || 0);

    useEffect(() => {
        setNameDraft(String(ep?.name || '').trim() || deriveNameFromUrl(ep?.url) || '');
    }, [ep?.id, ep?.name, ep?.url]);

    const handleRename = async () => {
        const n = String(nameDraft || '').trim();
        if (!n) return;
        setRenaming(true);
        try { await onRename?.(ep?.id, n); } catch { }
        setRenaming(false);
    };

    const handleCheck = async () => {
        setChecking(true);
        try { await onCheck?.(ep?.id); } catch { }
        setChecking(false);
    };

    return (
        <>
            <div className="group relative rounded-xl border border-zinc-100 bg-white/80 dark:border-white/5 dark:bg-white/[0.03] transition hover:border-zinc-200 dark:hover:border-white/10">
                {/* main row */}
                <div className="flex items-center gap-3 px-3 py-2.5 cursor-pointer" onClick={() => setExpanded(v => !v)}>
                    <StatusDot available={available} />
                    <div className="min-w-0 flex-1">
                        <div className="flex items-center gap-2">
                            <span className="text-[13px] font-medium text-zinc-900 dark:text-white/90 truncate">{displayName}</span>
                            {ep?.is_current && (
                                <span className={`rounded px-1.5 py-px text-[10px] font-bold ${getBrandChipClass(brand)}`}>
                                    IN USE
                                </span>
                            )}
                        </div>
                        <div className="text-[11px] text-zinc-400 dark:text-white/30 truncate mt-0.5">
                            {maskUrl(ep?.url, ep?.url_masked)}
                        </div>
                    </div>
                    <div className="shrink-0 flex items-center gap-1.5">
                        {latency > 0 && (
                            <span className={`text-[11px] font-mono tabular-nums ${latency < 300 ? 'text-emerald-600 dark:text-emerald-400' : latency < 1000 ? 'text-amber-600 dark:text-amber-400' : 'text-red-600 dark:text-red-400'}`}>
                                {latency}ms
                            </span>
                        )}
                        <svg width="14" height="14" viewBox="0 0 24 24" fill="none"
                            className={`text-zinc-400 dark:text-white/30 transition-transform ${expanded ? 'rotate-180' : ''}`}>
                            <path d="M7 10l5 5 5-5" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
                        </svg>
                    </div>
                </div>

                {/* expanded detail */}
                {expanded && (
                    <div className="border-t border-zinc-100 dark:border-white/5 px-3 py-3 space-y-3">
                        {/* meta grid */}
                        <div className="grid grid-cols-2 gap-x-4 gap-y-1.5 text-[11px]">
                            <div className="text-zinc-500 dark:text-white/40">
                                状态
                                <span className={`ml-1.5 font-medium ${available ? getBrandToneClass(brand) : 'text-red-600 dark:text-red-400'}`}>
                                    {available ? '可用' : '不可用'}
                                </span>
                            </div>
                            <div className="text-zinc-500 dark:text-white/40">
                                连续失败 <span className="font-mono ml-1">{Number(ep?.consecutive_failures || 0)}</span>
                            </div>
                            <div className="text-zinc-500 dark:text-white/40">
                                最近检测 <span className="ml-1 text-zinc-700 dark:text-white/60">{formatTime(ep?.last_checked_at)}</span>
                            </div>
                            <div className="text-zinc-500 dark:text-white/40">
                                最近成功 <span className="ml-1 text-zinc-700 dark:text-white/60">{formatTime(ep?.last_success_at)}</span>
                            </div>
                            {ep?.disabled_until && (
                                <div className="col-span-2 text-zinc-500 dark:text-white/40">
                                    禁用至 <span className="ml-1 text-red-600 dark:text-red-400">{formatTime(ep.disabled_until)}</span>
                                    {ep?.disabled_reason && (
                                        <span className="ml-1.5 text-zinc-400">({formatDisabledReason(ep.disabled_reason)})</span>
                                    )}
                                </div>
                            )}
                            {ep?.last_error && (
                                <div className="col-span-2 text-red-600/80 dark:text-red-300/80 break-all">
                                    {String(ep.last_error)}
                                </div>
                            )}
                        </div>

                        {/* rename */}
                        <div className="flex items-center gap-2">
                            <input
                                value={nameDraft}
                                onChange={e => setNameDraft(e.target.value)}
                                placeholder="节点名称"
                                onKeyDown={e => { if (e.key === 'Enter') handleRename(); }}
                                className={`flex-1 min-w-0 rounded-lg border border-zinc-200 bg-zinc-50 px-2.5 py-1.5 text-xs text-zinc-900 outline-none ${getBrandFocusBorderClass(brand)} dark:border-white/10 dark:bg-white/5 dark:text-white`}
                            />
                            <button type="button" onClick={handleRename} disabled={renaming}
                                className="shrink-0 rounded-lg px-2.5 py-1.5 text-[11px] font-medium bg-zinc-100 text-zinc-700 hover:bg-zinc-200 dark:bg-white/5 dark:text-white/70 dark:hover:bg-white/10 transition disabled:opacity-40">
                                {renaming ? '...' : '改名'}
                            </button>
                        </div>

                        {/* action bar */}
                        <div className="flex items-center gap-1.5 flex-wrap">
                            <button type="button" onClick={handleCheck} disabled={checking}
                                className={`rounded-lg px-3 py-1.5 text-[11px] font-medium transition disabled:opacity-40 ${getBrandOutlineButtonClass(brand)}`}>
                                {checking ? (
                                    <span className="inline-flex items-center gap-1">
                                        <svg className="animate-spin h-3 w-3" viewBox="0 0 24 24" fill="none">
                                            <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
                                            <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
                                        </svg>
                                        检测中
                                    </span>
                                ) : '探活检测'}
                            </button>
                            <button type="button"
                                onClick={() => onSwitch?.(ep?.id)}
                                disabled={!available || ep?.is_current}
                                className={`rounded-lg px-3 py-1.5 text-[11px] font-medium transition disabled:opacity-40 ${getBrandOutlineButtonClass(brand)}`}>
                                切换使用
                            </button>
                            {isUnavailable(ep) && (
                                <button type="button" onClick={() => onEnable?.(ep?.id)}
                                    className={`rounded-lg px-3 py-1.5 text-[11px] font-medium transition ${getBrandOutlineButtonClass(brand)}`}>
                                    启用
                                </button>
                            )}
                            {available && (
                                <button type="button" onClick={() => onDisableNextMonth?.(ep?.id)}
                                    className="rounded-lg px-3 py-1.5 text-[11px] font-medium border border-amber-200 text-amber-700 bg-amber-50 hover:bg-amber-100 dark:border-amber-500/20 dark:text-amber-300 dark:bg-amber-500/10 dark:hover:bg-amber-500/20 transition">
                                    禁用到下月
                                </button>
                            )}
                            <button type="button" onClick={() => setConfirmDelete(true)}
                                className="rounded-lg px-3 py-1.5 text-[11px] font-medium border border-red-200 text-red-600 bg-red-50 hover:bg-red-100 dark:border-red-500/20 dark:text-red-400 dark:bg-red-500/10 dark:hover:bg-red-500/20 transition">
                                删除
                            </button>
                        </div>
                    </div>
                )}
            </div>

            <ConfirmDialog
                open={confirmDelete}
                title="删除 RPC 节点"
                message={`确定要删除 "${displayName}" 吗？此操作不可撤销。`}
                danger
                brand={brand}
                onConfirm={() => { setConfirmDelete(false); onDelete?.(ep?.id); }}
                onCancel={() => setConfirmDelete(false)}
            />
        </>
    );
}

/* ── transport group ── */

function TransportGroup({ group, transport, onSwitch, onDisableNextMonth, onEnable, onRename, onDelete, onCheck, brand }) {
    const endpoints = group?.endpoints || [];
    const unavailableCount = endpoints.filter(ep => isUnavailable(ep)).length;
    const availableCount = endpoints.length - unavailableCount;

    const effID = Number(group?.effective_endpoint_id || 0);
    const effEndpoint = effID ? endpoints.find(ep => Number(ep?.id) === effID) : null;
    const effName = effEndpoint ? endpointDisplayName(effEndpoint) : '';
    const effURL = maskUrl(group?.effective_url, group?.effective_url_masked);
    const effectiveLabel = effName || (effURL !== '--' ? effURL : '未配置');

    return (
        <div className="space-y-2">
            {/* header */}
            <div className="flex items-center justify-between px-1">
                <div className="flex items-center gap-2">
                    <span className="text-xs font-bold text-zinc-700 dark:text-white/80 uppercase tracking-wider">
                        {formatTransport(transport)}
                    </span>
                    <span className="text-[11px] text-zinc-400 dark:text-white/30">
                        {availableCount}可用 / {unavailableCount}不可用
                    </span>
                </div>
                <div className="text-[11px] text-zinc-500 dark:text-white/40 truncate max-w-[60%] text-right">
                    当前: {effectiveLabel}
                    <span className="ml-1 text-zinc-400 dark:text-white/25">({formatSource(group?.effective_source)})</span>
                </div>
            </div>

            {/* env fallback hint */}
            {group?.env_url && (
                <div className="rounded-lg bg-zinc-50 dark:bg-white/[0.02] px-3 py-1.5 text-[11px] text-zinc-400 dark:text-white/30 truncate">
                    ENV 兜底: {maskUrl(group.env_url, group.env_url_masked)}
                </div>
            )}

            {/* endpoint list */}
            {endpoints.length === 0 ? (
                <div className="rounded-xl border border-dashed border-zinc-200 dark:border-white/10 py-6 text-center text-xs text-zinc-400 dark:text-white/30">
                    暂无节点
                </div>
            ) : (
                <div className="space-y-1.5">
                    {endpoints.map(ep => (
                        <EndpointRow
                            key={ep.id}
                            ep={ep}
                            onSwitch={onSwitch}
                            onDisableNextMonth={onDisableNextMonth}
                            onEnable={onEnable}
                            onRename={onRename}
                            onDelete={onDelete}
                            onCheck={onCheck}
                            brand={brand}
                        />
                    ))}
                </div>
            )}
        </div>
    );
}

/* ── add form ── */

function AddForm({ onAdd, adding, addError, brand }) {
    const [open, setOpen] = useState(false);
    const [draft, setDraft] = useState({
        chain: 'bsc', transport: 'http', name: '', url: '', setCurrent: false,
    });

    const handleSubmit = () => {
        onAdd?.(draft);
        setDraft(prev => ({ ...prev, name: '', url: '', setCurrent: false }));
    };

    return (
        <div className="rounded-2xl border border-zinc-200 bg-white/60 dark:border-white/10 dark:bg-white/[0.03] overflow-hidden">
            <button type="button" onClick={() => setOpen(v => !v)}
                className="w-full flex items-center justify-between px-4 py-3 text-left">
                <span className="text-sm font-semibold text-zinc-900 dark:text-white/90">
                    添加节点
                </span>
                <svg width="16" height="16" viewBox="0 0 24 24" fill="none"
                    className={`text-zinc-400 dark:text-white/30 transition-transform ${open ? 'rotate-180' : ''}`}>
                    <path d="M7 10l5 5 5-5" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
                </svg>
            </button>

            {open && (
                <div className="border-t border-zinc-100 dark:border-white/5 px-4 py-3 space-y-3">
                    <div className="grid grid-cols-2 gap-2">
                        <label className="space-y-1">
                            <div className="text-[11px] font-medium text-zinc-500 dark:text-white/50">链</div>
                            <select value={draft.chain} onChange={e => setDraft(p => ({ ...p, chain: e.target.value }))}
                                className={`w-full rounded-lg border border-zinc-200 bg-zinc-50 px-2.5 py-2 text-xs text-zinc-900 outline-none ${getBrandFocusBorderClass(brand)} dark:border-white/10 dark:bg-white/5 dark:text-white`}>
                                <option value="bsc">BSC</option>
                                <option value="base">Base</option>
                            </select>
                        </label>
                        <label className="space-y-1">
                            <div className="text-[11px] font-medium text-zinc-500 dark:text-white/50">类型</div>
                            <select value={draft.transport} onChange={e => setDraft(p => ({ ...p, transport: e.target.value }))}
                                className={`w-full rounded-lg border border-zinc-200 bg-zinc-50 px-2.5 py-2 text-xs text-zinc-900 outline-none ${getBrandFocusBorderClass(brand)} dark:border-white/10 dark:bg-white/5 dark:text-white`}>
                                <option value="http">HTTP</option>
                                <option value="ws">WebSocket</option>
                            </select>
                        </label>
                    </div>
                    <label className="block space-y-1">
                        <div className="text-[11px] font-medium text-zinc-500 dark:text-white/50">URL</div>
                        <input value={draft.url}
                            onChange={e => setDraft(p => {
                                const nextUrl = e.target.value;
                                const keepName = String(p.name || '').trim();
                                const derived = deriveNameFromUrl(nextUrl);
                                return { ...p, url: nextUrl, name: keepName ? p.name : (derived || p.name) };
                            })}
                            placeholder={draft.transport === 'ws' ? 'wss://...' : 'https://...'}
                            className={`w-full rounded-lg border border-zinc-200 bg-zinc-50 px-2.5 py-2 text-xs text-zinc-900 outline-none ${getBrandFocusBorderClass(brand)} dark:border-white/10 dark:bg-white/5 dark:text-white`}
                        />
                    </label>
                    <label className="block space-y-1">
                        <div className="text-[11px] font-medium text-zinc-500 dark:text-white/50">名称 <span className="text-zinc-400">(留空自动使用域名)</span></div>
                        <input value={draft.name} onChange={e => setDraft(p => ({ ...p, name: e.target.value }))}
                            placeholder="例如：主用 / 备用1"
                            className={`w-full rounded-lg border border-zinc-200 bg-zinc-50 px-2.5 py-2 text-xs text-zinc-900 outline-none ${getBrandFocusBorderClass(brand)} dark:border-white/10 dark:bg-white/5 dark:text-white`}
                        />
                    </label>
                    <label className="flex items-center gap-2 text-xs text-zinc-600 dark:text-white/60">
                        <input type="checkbox" checked={Boolean(draft.setCurrent)}
                            onChange={e => setDraft(p => ({ ...p, setCurrent: e.target.checked }))}
                            className="rounded"
                        />
                        添加后设为当前节点
                    </label>

                    {addError && (
                        <div className="rounded-lg bg-red-50 dark:bg-red-500/10 px-3 py-2 text-[11px] text-red-700 dark:text-red-300">
                            {addError}
                        </div>
                    )}

                    <button type="button" onClick={handleSubmit} disabled={adding || !String(draft.url || '').trim()}
                        className={`w-full rounded-xl py-2.5 text-xs font-semibold transition ${adding || !String(draft.url || '').trim()
                            ? 'bg-zinc-200 text-zinc-500 dark:bg-white/10 dark:text-white/30 cursor-not-allowed'
                            : `${brand.solidButtonClass} active:scale-[0.98]`
                            }`}>
                        {adding ? '添加中...' : '添加节点'}
                    </button>
                </div>
            )}
        </div>
    );
}

/* ── main component ── */

export default function AdminRPCPool({ apiBaseUrl, initData, hasInitData, pollIntervalSec = 15, accentTheme = 'lime', onNotice }) {
    const brand = useMemo(() => getBrandTheme(accentTheme), [accentTheme]);
    const [data, setData] = useState(null);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState('');
    const [adding, setAdding] = useState(false);
    const [addError, setAddError] = useState('');

    const groups = useMemo(() => data?.groups || [], [data]);
    const groupMap = useMemo(() => {
        const out = {};
        for (const g of groups) {
            const chain = String(g?.chain || '').toLowerCase();
            const transport = String(g?.transport || '').toLowerCase();
            if (chain && transport) out[`${chain}:${transport}`] = g;
        }
        return out;
    }, [groups]);

    const chains = useMemo(() => {
        const found = new Set(groups.map(g => String(g?.chain || '').toLowerCase()).filter(Boolean));
        const ordered = [];
        for (const k of ['bsc', 'base']) {
            if (found.has(k)) { ordered.push(k); found.delete(k); }
        }
        for (const rest of Array.from(found).sort()) ordered.push(rest);
        return ordered;
    }, [groups]);

    const load = useCallback(async () => {
        if (!hasInitData) return;
        setLoading(true);
        setError('');
        try {
            setData(await fetchAdminRPCPool({ apiBaseUrl, initData }));
        } catch (e) {
            setError(String(e?.message || e));
        } finally {
            setLoading(false);
        }
    }, [apiBaseUrl, initData, hasInitData]);

    useEffect(() => { load(); }, [load]);

    useEffect(() => {
        if (!hasInitData) return;
        const t = setInterval(load, Math.max(3, Number(pollIntervalSec) || 15) * 1000);
        return () => clearInterval(t);
    }, [hasInitData, pollIntervalSec, load]);

    const handleAdd = useCallback(async (draft) => {
        if (!hasInitData) return;
        const url = String(draft.url || '').trim();
        if (!url) { setAddError('请填写 URL'); return; }
        setAdding(true);
        setAddError('');
        try {
            await addAdminRPCEndpoint({
                apiBaseUrl, initData,
                chain: draft.chain, transport: draft.transport,
                name: String(draft.name || '').trim(), url,
                setCurrent: Boolean(draft.setCurrent),
            });
            onNotice?.('已添加 RPC 节点');
            load();
        } catch (e) {
            setAddError(String(e?.message || e));
        } finally {
            setAdding(false);
        }
    }, [apiBaseUrl, initData, hasInitData, onNotice, load]);

    const wrap = (fn, successMsg) => async (...args) => {
        if (!hasInitData) return;
        try {
            await fn(...args);
            if (successMsg) onNotice?.(successMsg);
            load();
        } catch (e) {
            onNotice?.(String(e?.message || e));
        }
    };

    const handleSwitch = useCallback(wrap(
        (id) => switchAdminRPCEndpoint({ apiBaseUrl, initData, endpointId: id }),
        '已切换当前节点'
    ), [apiBaseUrl, initData, hasInitData, onNotice, load]);

    const handleDisableNextMonth = useCallback(wrap(
        (id) => disableAdminRPCEndpointNextMonth({ apiBaseUrl, initData, endpointId: id }),
        '已禁用到下月'
    ), [apiBaseUrl, initData, hasInitData, onNotice, load]);

    const handleEnable = useCallback(wrap(
        (id) => enableAdminRPCEndpoint({ apiBaseUrl, initData, endpointId: id }),
        '节点已启用'
    ), [apiBaseUrl, initData, hasInitData, onNotice, load]);

    const handleRename = useCallback(async (endpointId, name) => {
        if (!hasInitData) return;
        await renameAdminRPCEndpoint({ apiBaseUrl, initData, endpointId: Number(endpointId), name: String(name).trim() });
        onNotice?.('名称已更新');
        load();
    }, [apiBaseUrl, initData, hasInitData, onNotice, load]);

    const handleDelete = useCallback(wrap(
        (id) => deleteAdminRPCEndpoint({ apiBaseUrl, initData, endpointId: id }),
        '节点已删除'
    ), [apiBaseUrl, initData, hasInitData, onNotice, load]);

    const handleCheck = useCallback(wrap(
        (id) => checkAdminRPCEndpoint({ apiBaseUrl, initData, endpointId: id }),
        '探活完成'
    ), [apiBaseUrl, initData, hasInitData, onNotice, load]);

    /* total stats */
    const totalEndpoints = groups.reduce((s, g) => s + (g?.endpoints?.length || 0), 0);
    const totalAvailable = groups.reduce((s, g) => s + (g?.endpoints || []).filter(ep => !isUnavailable(ep)).length, 0);

    return (
        <div className="space-y-3">
            {/* header */}
            <div className="flex items-center justify-between">
                <div>
                    <div className="text-base font-bold text-zinc-900 dark:text-white">RPC 节点池</div>
                    <div className="text-[11px] text-zinc-400 dark:text-white/30 mt-0.5">
                        {totalEndpoints > 0 ? (
                            <>{totalAvailable}<span className={getBrandToneClass(brand)}> 可用</span> / {totalEndpoints} 总计</>
                        ) : '额度/频控触发时自动切换'}
                    </div>
                </div>
                <button type="button" onClick={load} disabled={loading}
                    className={`rounded-xl px-3 py-1.5 text-xs font-medium ring-1 ring-zinc-200 dark:ring-white/10 transition ${loading ? 'opacity-40 cursor-not-allowed' : 'hover:bg-zinc-100 dark:hover:bg-white/10 active:scale-95'}`}>
                    {loading ? (
                        <span className="inline-flex items-center gap-1">
                            <svg className="animate-spin h-3 w-3" viewBox="0 0 24 24" fill="none">
                                <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
                                <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
                            </svg>
                            刷新中
                        </span>
                    ) : '刷新'}
                </button>
            </div>

            {error && (
                <div className="rounded-xl bg-red-50 dark:bg-red-500/10 border border-red-200 dark:border-red-500/20 px-3 py-2 text-xs text-red-700 dark:text-red-300">
                    {error}
                </div>
            )}

            {/* add form */}
            <AddForm onAdd={handleAdd} adding={adding} addError={addError} brand={brand} />

            {/* chain sections */}
            {chains.map(chain => (
                <div key={chain} className="rounded-2xl border border-zinc-200 bg-white/60 dark:border-white/10 dark:bg-white/[0.03] p-4 space-y-4">
                    <div className="text-sm font-bold text-zinc-900 dark:text-white/90">
                        {formatChain(chain)}
                    </div>

                    {['http', 'ws'].map(transport => {
                        const key = `${chain}:${transport}`;
                        const g = groupMap[key] || {
                            chain, transport,
                            effective_source: 'none', effective_url: '', effective_url_masked: '',
                            endpoints: [],
                        };
                        return (
                            <TransportGroup
                                key={key}
                                group={g}
                                transport={transport}
                                onSwitch={handleSwitch}
                                onDisableNextMonth={handleDisableNextMonth}
                                onEnable={handleEnable}
                                onRename={handleRename}
                                onDelete={handleDelete}
                                onCheck={handleCheck}
                                brand={brand}
                            />
                        );
                    })}
                </div>
            ))}
        </div>
    );
}
