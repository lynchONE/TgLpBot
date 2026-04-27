import React, { useCallback, useEffect, useMemo, useState } from 'react';
import {
    addAdminPoolDataSource,
    checkAdminPoolDataSource,
    deleteAdminPoolDataSource,
    disableAdminPoolDataSource,
    enableAdminPoolDataSource,
    fetchAdminPoolDataSources,
    switchAdminPoolDataSource,
} from '../lib/api';
import { getBrandTheme } from '../lib/brand';

function formatTime(ts) {
    if (!ts) return '--';
    const d = new Date(ts);
    if (Number.isNaN(d.getTime())) return String(ts);
    return d.toLocaleString();
}

function formatSourceType(type) {
    const v = String(type || '').toLowerCase();
    if (v === 'poolm_top_fees') return 'PoolM';
    if (v === 'market_pools') return 'Market Pools';
    return String(type || '--');
}

function splitCSV(raw) {
    return String(raw || '').split(',').map((item) => item.trim()).filter(Boolean);
}

function coverageText(source) {
    const c = source?.last_field_coverage || {};
    const poolCount = Number(c.pool_count || 0);
    if (!poolCount) return '';
    const parts = [`池子 ${poolCount}`];
    if (Number(c.missing_tvl_count || 0) > 0) parts.push(`TVL 缺 ${c.missing_tvl_count}`);
    if (Number(c.missing_active_liquidity_usd_count || 0) > 0) parts.push(`活跃缺 ${c.missing_active_liquidity_usd_count}`);
    if (Number(c.v4_pool_id_fallback_count || 0) > 0) parts.push(`v4 poolId ${c.v4_pool_id_fallback_count}`);
    return parts.join(' / ');
}

function AddSourceForm({ onAdd, adding, error, brand }) {
    const [open, setOpen] = useState(false);
    const [draft, setDraft] = useState({
        name: '',
        sourceType: 'market_pools',
        chain: 'bsc',
        timeframeMinutes: 5,
        limit: 100,
        baseUrl: 'http://localhost:8080',
        pathTemplate: '/api/market/pools',
        protocols: 'v3,v4',
        dexes: 'PancakeswapV3,UniswapV3,UniswapV4',
        setCurrent: false,
    });

    const update = (key, value) => {
        setDraft((prev) => {
            const next = { ...prev, [key]: value };
            if (key === 'sourceType') {
                if (value === 'poolm_top_fees') {
                    next.pathTemplate = '';
                    next.protocols = '';
                    next.dexes = 'pcsv3,univ3,univ4';
                    if (!String(prev.baseUrl || '').trim() || prev.baseUrl === 'http://localhost:8080') {
                        next.baseUrl = 'https://mapi.poolm.xyz';
                    }
                } else {
                    next.pathTemplate = '/api/market/pools';
                    next.protocols = 'v3,v4';
                    next.dexes = 'PancakeswapV3,UniswapV3,UniswapV4';
                    if (!String(prev.baseUrl || '').trim() || prev.baseUrl === 'https://mapi.poolm.xyz') {
                        next.baseUrl = 'http://localhost:8080';
                    }
                }
            }
            return next;
        });
    };

    const submit = () => {
        onAdd?.({
            ...draft,
            protocols: splitCSV(draft.protocols),
            dexes: splitCSV(draft.dexes),
        });
    };

    return (
        <div className="rounded-2xl border border-zinc-200 bg-white/60 dark:border-white/10 dark:bg-white/[0.03] overflow-hidden">
            <button type="button" onClick={() => setOpen((v) => !v)} className="w-full flex items-center justify-between px-4 py-3 text-left">
                <span className="text-sm font-semibold text-zinc-900 dark:text-white/90">添加池子源</span>
                <span className={`text-zinc-400 transition-transform ${open ? 'rotate-180' : ''}`}>⌄</span>
            </button>
            {open && (
                <div className="border-t border-zinc-100 dark:border-white/5 px-4 py-3 space-y-3">
                    <div className="grid grid-cols-2 gap-2">
                        <label className="space-y-1">
                            <div className="text-[11px] text-zinc-500 dark:text-white/50">来源类型</div>
                            <select value={draft.sourceType} onChange={(e) => update('sourceType', e.target.value)}
                                className="w-full rounded-lg border border-zinc-200 bg-zinc-50 px-2.5 py-2 text-xs dark:border-white/10 dark:bg-white/5 dark:text-white">
                                <option value="market_pools">Market Pools</option>
                                <option value="poolm_top_fees">PoolM</option>
                            </select>
                        </label>
                        <label className="space-y-1">
                            <div className="text-[11px] text-zinc-500 dark:text-white/50">链</div>
                            <select value={draft.chain} onChange={(e) => update('chain', e.target.value)}
                                className="w-full rounded-lg border border-zinc-200 bg-zinc-50 px-2.5 py-2 text-xs dark:border-white/10 dark:bg-white/5 dark:text-white">
                                <option value="bsc">BSC</option>
                                <option value="base">Base</option>
                            </select>
                        </label>
                    </div>
                    <label className="block space-y-1">
                        <div className="text-[11px] text-zinc-500 dark:text-white/50">Base URL</div>
                        <input value={draft.baseUrl} onChange={(e) => update('baseUrl', e.target.value)}
                            className="w-full rounded-lg border border-zinc-200 bg-zinc-50 px-2.5 py-2 text-xs dark:border-white/10 dark:bg-white/5 dark:text-white"
                        />
                    </label>
                    <label className="block space-y-1">
                        <div className="text-[11px] text-zinc-500 dark:text-white/50">Path</div>
                        <input value={draft.pathTemplate} onChange={(e) => update('pathTemplate', e.target.value)}
                            placeholder="/api/market/pools"
                            className="w-full rounded-lg border border-zinc-200 bg-zinc-50 px-2.5 py-2 text-xs dark:border-white/10 dark:bg-white/5 dark:text-white"
                        />
                    </label>
                    <div className="grid grid-cols-2 gap-2">
                        <label className="space-y-1">
                            <div className="text-[11px] text-zinc-500 dark:text-white/50">Limit</div>
                            <input type="number" min="1" value={draft.limit} onChange={(e) => update('limit', e.target.value)}
                                className="w-full rounded-lg border border-zinc-200 bg-zinc-50 px-2.5 py-2 text-xs dark:border-white/10 dark:bg-white/5 dark:text-white"
                            />
                        </label>
                        <label className="space-y-1">
                            <div className="text-[11px] text-zinc-500 dark:text-white/50">窗口(分钟)</div>
                            <input type="number" min="1" value={draft.timeframeMinutes} onChange={(e) => update('timeframeMinutes', e.target.value)}
                                className="w-full rounded-lg border border-zinc-200 bg-zinc-50 px-2.5 py-2 text-xs dark:border-white/10 dark:bg-white/5 dark:text-white"
                            />
                        </label>
                    </div>
                    <label className="block space-y-1">
                        <div className="text-[11px] text-zinc-500 dark:text-white/50">Protocol</div>
                        <input value={draft.protocols} onChange={(e) => update('protocols', e.target.value)}
                            className="w-full rounded-lg border border-zinc-200 bg-zinc-50 px-2.5 py-2 text-xs dark:border-white/10 dark:bg-white/5 dark:text-white"
                        />
                    </label>
                    <label className="block space-y-1">
                        <div className="text-[11px] text-zinc-500 dark:text-white/50">DEX</div>
                        <input value={draft.dexes} onChange={(e) => update('dexes', e.target.value)}
                            className="w-full rounded-lg border border-zinc-200 bg-zinc-50 px-2.5 py-2 text-xs dark:border-white/10 dark:bg-white/5 dark:text-white"
                        />
                    </label>
                    <label className="block space-y-1">
                        <div className="text-[11px] text-zinc-500 dark:text-white/50">名称</div>
                        <input value={draft.name} onChange={(e) => update('name', e.target.value)}
                            placeholder="留空使用域名"
                            className="w-full rounded-lg border border-zinc-200 bg-zinc-50 px-2.5 py-2 text-xs dark:border-white/10 dark:bg-white/5 dark:text-white"
                        />
                    </label>
                    <label className="flex items-center gap-2 text-xs text-zinc-600 dark:text-white/60">
                        <input type="checkbox" checked={Boolean(draft.setCurrent)} onChange={(e) => update('setCurrent', e.target.checked)} />
                        添加后设为当前来源
                    </label>
                    {error && <div className="rounded-lg bg-red-50 px-3 py-2 text-[11px] text-red-700 dark:bg-red-500/10 dark:text-red-300">{error}</div>}
                    <button type="button" onClick={submit} disabled={adding || !String(draft.baseUrl || '').trim()}
                        className={`w-full rounded-xl py-2.5 text-xs font-semibold transition ${adding ? 'opacity-50' : brand.solidButtonClass}`}>
                        {adding ? '添加中...' : '添加来源'}
                    </button>
                </div>
            )}
        </div>
    );
}

function SourceRow({ source, onSwitch, onEnable, onDisable, onDelete, onCheck }) {
    const enabled = Boolean(source?.is_enabled);
    const current = Boolean(source?.is_current);
    return (
        <div className="rounded-xl border border-zinc-100 bg-white/80 px-3 py-3 dark:border-white/5 dark:bg-white/[0.03]">
            <div className="flex items-start justify-between gap-3">
                <div className="min-w-0">
                    <div className="flex items-center gap-2">
                        <span className="truncate text-[13px] font-semibold text-zinc-900 dark:text-white/90">{source?.name || '--'}</span>
                        {current && <span className="rounded px-1.5 py-px text-[10px] font-bold bg-emerald-500/15 text-emerald-700 dark:text-emerald-300">IN USE</span>}
                        <span className={`rounded px-1.5 py-px text-[10px] font-bold ${enabled ? 'bg-zinc-100 text-zinc-600 dark:bg-white/10 dark:text-white/60' : 'bg-red-500/10 text-red-600 dark:text-red-300'}`}>
                            {enabled ? '启用' : '停用'}
                        </span>
                    </div>
                    <div className="mt-1 text-[11px] text-zinc-500 dark:text-white/45">
                        {formatSourceType(source?.source_type)} · {source?.chain || 'bsc'} · {source?.timeframe_minutes || 5}m · limit {source?.limit || 100}
                    </div>
                    <div className="mt-1 truncate text-[11px] text-zinc-400 dark:text-white/30">
                        {source?.base_url_masked || source?.base_url || '--'}{source?.path_template ? source.path_template : ''}
                    </div>
                </div>
            </div>
            <div className="mt-2 grid grid-cols-2 gap-x-3 gap-y-1 text-[11px] text-zinc-500 dark:text-white/45">
                <div>最近检测 {formatTime(source?.last_checked_at)}</div>
                <div>最近成功 {formatTime(source?.last_success_at)}</div>
                <div>延迟 {Number(source?.last_latency_ms || 0) || '--'}ms</div>
                <div>{coverageText(source)}</div>
                {source?.last_error && <div className="col-span-2 break-all text-red-600 dark:text-red-300">{source.last_error}</div>}
            </div>
            <div className="mt-3 flex flex-wrap gap-1.5">
                <button type="button" onClick={() => onCheck?.(source?.id)} className="rounded-lg border border-zinc-200 px-3 py-1.5 text-[11px] dark:border-white/10">
                    检查
                </button>
                <button type="button" onClick={() => onSwitch?.(source?.id)} disabled={!enabled || current}
                    className="rounded-lg border border-zinc-200 px-3 py-1.5 text-[11px] disabled:opacity-40 dark:border-white/10">
                    切换
                </button>
                {enabled ? (
                    <button type="button" onClick={() => onDisable?.(source?.id)} className="rounded-lg border border-amber-200 px-3 py-1.5 text-[11px] text-amber-700 dark:border-amber-500/20 dark:text-amber-300">
                        停用
                    </button>
                ) : (
                    <button type="button" onClick={() => onEnable?.(source?.id)} className="rounded-lg border border-emerald-200 px-3 py-1.5 text-[11px] text-emerald-700 dark:border-emerald-500/20 dark:text-emerald-300">
                        启用
                    </button>
                )}
                <button type="button" onClick={() => onDelete?.(source?.id)} className="rounded-lg border border-red-200 px-3 py-1.5 text-[11px] text-red-600 dark:border-red-500/20 dark:text-red-300">
                    删除
                </button>
            </div>
        </div>
    );
}

export default function AdminPoolDataSources({ apiBaseUrl, initData, hasInitData, pollIntervalSec = 15, accentTheme = 'lime', onNotice }) {
    const brand = useMemo(() => getBrandTheme(accentTheme), [accentTheme]);
    const [data, setData] = useState(null);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState('');
    const [adding, setAdding] = useState(false);
    const [addError, setAddError] = useState('');

    const groups = useMemo(() => data?.groups || [], [data]);

    const load = useCallback(async () => {
        if (!hasInitData) return;
        setLoading(true);
        setError('');
        try {
            setData(await fetchAdminPoolDataSources({ apiBaseUrl, initData }));
        } catch (e) {
            setError(String(e?.message || e));
        } finally {
            setLoading(false);
        }
    }, [apiBaseUrl, initData, hasInitData]);

    useEffect(() => { load(); }, [load]);

    useEffect(() => {
        if (!hasInitData) return undefined;
        const timer = setInterval(load, Math.max(5, Number(pollIntervalSec) || 15) * 1000);
        return () => clearInterval(timer);
    }, [hasInitData, load, pollIntervalSec]);

    const handleAdd = useCallback(async (draft) => {
        setAdding(true);
        setAddError('');
        try {
            await addAdminPoolDataSource({
                apiBaseUrl,
                initData,
                name: draft.name,
                sourceType: draft.sourceType,
                chain: draft.chain,
                timeframeMinutes: Number(draft.timeframeMinutes) || 5,
                limit: Number(draft.limit) || 100,
                baseUrl: draft.baseUrl,
                pathTemplate: draft.pathTemplate,
                protocols: draft.protocols,
                dexes: draft.dexes,
                setCurrent: draft.setCurrent,
            });
            onNotice?.('已添加池子数据源');
            load();
        } catch (e) {
            setAddError(String(e?.message || e));
        } finally {
            setAdding(false);
        }
    }, [apiBaseUrl, initData, load, onNotice]);

    const wrap = (fn, message) => async (sourceId) => {
        try {
            await fn({ apiBaseUrl, initData, sourceId });
            if (message) onNotice?.(message);
            load();
        } catch (e) {
            onNotice?.(String(e?.message || e));
        }
    };

    const handleSwitch = useCallback(wrap(switchAdminPoolDataSource, '已切换池子数据源'), [apiBaseUrl, initData, load, onNotice]);
    const handleEnable = useCallback(wrap(enableAdminPoolDataSource, '已启用池子数据源'), [apiBaseUrl, initData, load, onNotice]);
    const handleDisable = useCallback(wrap(disableAdminPoolDataSource, '已停用池子数据源'), [apiBaseUrl, initData, load, onNotice]);
    const handleCheck = useCallback(wrap(checkAdminPoolDataSource, '检查完成'), [apiBaseUrl, initData, load, onNotice]);
    const handleDelete = useCallback(wrap(deleteAdminPoolDataSource, '已删除池子数据源'), [apiBaseUrl, initData, load, onNotice]);

    return (
        <div className="space-y-3">
            <div className="flex items-center justify-between">
                <div>
                    <div className="text-base font-bold text-zinc-900 dark:text-white">池子数据源</div>
                    <div className="mt-0.5 text-[11px] text-zinc-400 dark:text-white/30">
                        {groups.reduce((sum, group) => sum + (group?.sources?.length || 0), 0)} 个来源
                    </div>
                </div>
                <button type="button" onClick={load} disabled={loading}
                    className="rounded-xl px-3 py-1.5 text-xs font-medium ring-1 ring-zinc-200 transition hover:bg-zinc-100 disabled:opacity-40 dark:ring-white/10 dark:hover:bg-white/10">
                    {loading ? '刷新中' : '刷新'}
                </button>
            </div>
            {error && <div className="rounded-xl bg-red-50 px-3 py-2 text-xs text-red-700 dark:bg-red-500/10 dark:text-red-300">{error}</div>}
            <AddSourceForm onAdd={handleAdd} adding={adding} error={addError} brand={brand} />
            {groups.map((group) => (
                <div key={`${group.chain}:${group.timeframe_minutes}`} className="rounded-2xl border border-zinc-200 bg-white/60 p-4 dark:border-white/10 dark:bg-white/[0.03]">
                    <div className="mb-3 flex items-center justify-between gap-3">
                        <div className="text-sm font-bold text-zinc-900 dark:text-white/90">
                            {(group.chain || 'bsc').toUpperCase()} · {group.timeframe_minutes || 5}m
                        </div>
                        <div className="truncate text-right text-[11px] text-zinc-500 dark:text-white/45">
                            当前 {group.effective_source?.name || group.env_fallback?.name || '--'}
                        </div>
                    </div>
                    {group.sources?.length ? (
                        <div className="space-y-2">
                            {group.sources.map((source) => (
                                <SourceRow
                                    key={source.id}
                                    source={source}
                                    onSwitch={handleSwitch}
                                    onEnable={handleEnable}
                                    onDisable={handleDisable}
                                    onDelete={handleDelete}
                                    onCheck={handleCheck}
                                />
                            ))}
                        </div>
                    ) : (
                        <div className="rounded-xl border border-dashed border-zinc-200 px-3 py-4 text-center text-xs text-zinc-400 dark:border-white/10 dark:text-white/35">
                            当前使用 ENV 兜底：{group.env_fallback?.base_url_masked || group.env_fallback?.base_url || '--'}
                        </div>
                    )}
                </div>
            ))}
        </div>
    );
}
