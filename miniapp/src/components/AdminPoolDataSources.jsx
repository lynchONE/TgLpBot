import { useCallback, useEffect, useMemo, useState } from 'react';
import {
    addAdminPoolDataSource,
    checkAdminPoolDataSource,
    deleteAdminPoolDataSource,
    disableAdminPoolDataSource,
    enableAdminPoolDataSource,
    fetchAdminPoolDataSources,
    switchAdminPoolDataSource,
    updateAdminPoolDataSource,
} from '../lib/api';
import { getBrandTheme } from '../lib/brand';
import CustomSelect from './CustomSelect.jsx';

const SOURCE_TYPE_OPTIONS = [
    { value: 'market_pools', label: 'Market Pools' },
    { value: 'poolm_top_fees', label: 'PoolM' },
];

const CHAIN_OPTIONS = [
    { value: 'bsc', label: 'BSC' },
    { value: 'base', label: 'Base' },
];

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

function sourceToDraft(source) {
    return {
        name: String(source?.name || ''),
        sourceType: String(source?.source_type || 'market_pools'),
        chain: String(source?.chain || 'bsc'),
        timeframeMinutes: Number(source?.timeframe_minutes || 5),
        limit: Number(source?.limit || 100),
        baseUrl: String(source?.base_url || ''),
        pathTemplate: String(source?.path_template || ''),
        protocols: Array.isArray(source?.protocols) ? source.protocols.join(',') : '',
        dexes: Array.isArray(source?.dexes) ? source.dexes.join(',') : '',
        setCurrent: false,
    };
}

function updateSourceDraft(prev, key, value) {
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
}

function SourceDraftFields({
    draft,
    update,
    inputClassName,
    sourceTypeLabel = '来源类型',
    spacingClassName = '',
}) {
    const baseBlockClassName = 'block space-y-1';
    const spacedBlockClassName = spacingClassName ? spacingClassName + ' ' + baseBlockClassName : baseBlockClassName;
    const spacedGridClassName = spacingClassName ? spacingClassName + ' grid grid-cols-2 gap-2' : 'grid grid-cols-2 gap-2';

    return (
        <>
            <div className="grid grid-cols-2 gap-2">
                <label className="space-y-1">
                    <div className="text-[11px] text-zinc-500 dark:text-white/50">{sourceTypeLabel}</div>
                    <CustomSelect value={draft.sourceType} onChange={(value) => update('sourceType', value)} options={SOURCE_TYPE_OPTIONS} />
                </label>
                <label className="space-y-1">
                    <div className="text-[11px] text-zinc-500 dark:text-white/50">链</div>
                    <CustomSelect value={draft.chain} onChange={(value) => update('chain', value)} options={CHAIN_OPTIONS} />
                </label>
            </div>
            <label className={spacedBlockClassName}>
                <div className="text-[11px] text-zinc-500 dark:text-white/50">Base URL</div>
                <input value={draft.baseUrl} onChange={(e) => update('baseUrl', e.target.value)}
                    className={inputClassName}
                />
            </label>
            <label className={spacedBlockClassName}>
                <div className="text-[11px] text-zinc-500 dark:text-white/50">Path</div>
                <input value={draft.pathTemplate} onChange={(e) => update('pathTemplate', e.target.value)}
                    placeholder="/api/market/pools"
                    className={inputClassName}
                />
            </label>
            <div className={spacedGridClassName}>
                <label className="space-y-1">
                    <div className="text-[11px] text-zinc-500 dark:text-white/50">Limit</div>
                    <input type="number" min="1" value={draft.limit} onChange={(e) => update('limit', e.target.value)}
                        className={inputClassName}
                    />
                </label>
                <label className="space-y-1">
                    <div className="text-[11px] text-zinc-500 dark:text-white/50">窗口(分钟)</div>
                    <input type="number" min="1" value={draft.timeframeMinutes} onChange={(e) => update('timeframeMinutes', e.target.value)}
                        className={inputClassName}
                    />
                </label>
            </div>
            <label className={spacedBlockClassName}>
                <div className="text-[11px] text-zinc-500 dark:text-white/50">Protocol</div>
                <input value={draft.protocols} onChange={(e) => update('protocols', e.target.value)}
                    className={inputClassName}
                />
            </label>
            <label className={spacedBlockClassName}>
                <div className="text-[11px] text-zinc-500 dark:text-white/50">DEX</div>
                <input value={draft.dexes} onChange={(e) => update('dexes', e.target.value)}
                    className={inputClassName}
                />
            </label>
            <label className={spacedBlockClassName}>
                <div className="text-[11px] text-zinc-500 dark:text-white/50">名称</div>
                <input value={draft.name} onChange={(e) => update('name', e.target.value)}
                    placeholder="留空使用域名"
                    className={inputClassName}
                />
            </label>
        </>
    );
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
        setDraft((prev) => updateSourceDraft(prev, key, value));
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
                    <SourceDraftFields
                        draft={draft}
                        update={update}
                        inputClassName="w-full rounded-lg border border-zinc-200 bg-zinc-50 px-2.5 py-2 text-xs dark:border-white/10 dark:bg-white/5 dark:text-white"
                    />
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

function EditableSourceRow({ source, onSwitch, onEnable, onDisable, onDelete, onCheck, onUpdate, updating, updateError, brand }) {
    const enabled = Boolean(source?.is_enabled);
    const current = Boolean(source?.is_current);
    const [editing, setEditing] = useState(false);
    const [draft, setDraft] = useState(() => sourceToDraft(source));

    useEffect(() => {
        setDraft(sourceToDraft(source));
        setEditing(false);
    }, [source?.id, source?.name, source?.source_type, source?.chain, source?.timeframe_minutes, source?.limit, source?.base_url, source?.path_template]);

    const update = (key, value) => setDraft((prev) => updateSourceDraft(prev, key, value));
    const submit = () => onUpdate?.(source?.id, { ...draft, protocols: splitCSV(draft.protocols), dexes: splitCSV(draft.dexes) });

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
                        {formatSourceType(source?.source_type)} / {source?.chain || 'bsc'} / {source?.timeframe_minutes || 5}m / limit {source?.limit || 100}
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
                <button type="button" onClick={() => setEditing((v) => !v)} className="rounded-lg border border-zinc-200 px-3 py-1.5 text-[11px] dark:border-white/10">
                    {editing ? '收起' : '编辑'}
                </button>
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
            {editing && (
                <div className="mt-3 rounded-xl border border-zinc-100 bg-zinc-50/80 p-3 dark:border-white/10 dark:bg-white/[0.04]">
                    <SourceDraftFields
                        draft={draft}
                        update={update}
                        inputClassName="w-full rounded-lg border border-zinc-200 bg-white px-2.5 py-2 text-xs dark:border-white/10 dark:bg-white/5 dark:text-white"
                        sourceTypeLabel="类型"
                        spacingClassName="mt-2"
                    />
                    <label className="mt-2 flex items-center gap-2 text-xs text-zinc-600 dark:text-white/60">
                        <input type="checkbox" checked={Boolean(draft.setCurrent)} onChange={(e) => update('setCurrent', e.target.checked)} />
                        保存后设为当前来源
                    </label>
                    {updateError && <div className="mt-2 rounded-lg bg-red-50 px-3 py-2 text-[11px] text-red-700 dark:bg-red-500/10 dark:text-red-300">{updateError}</div>}
                    <div className="mt-3 flex gap-2">
                        <button type="button" onClick={submit} disabled={updating || !String(draft.baseUrl || '').trim()}
                            className={`flex-1 rounded-xl py-2.5 text-xs font-semibold transition ${updating ? 'opacity-50' : brand.solidButtonClass}`}>
                            {updating ? '保存中...' : '保存修改'}
                        </button>
                        <button type="button" onClick={() => { setDraft(sourceToDraft(source)); setEditing(false); }} disabled={updating}
                            className="rounded-xl border border-zinc-200 px-3 py-2.5 text-xs font-semibold text-zinc-600 disabled:opacity-50 dark:border-white/10 dark:text-white/60">
                            取消
                        </button>
                    </div>
                </div>
            )}
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
    const [updatingId, setUpdatingId] = useState(0);
    const [updateErrors, setUpdateErrors] = useState({});

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

    const handleUpdate = useCallback(async (sourceId, draft) => {
        const id = Number(sourceId);
        if (!id) return;
        setUpdatingId(id);
        setUpdateErrors((prev) => ({ ...prev, [id]: '' }));
        try {
            await updateAdminPoolDataSource({
                apiBaseUrl,
                initData,
                sourceId: id,
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
            onNotice?.('池子数据源已保存');
            load();
        } catch (e) {
            setUpdateErrors((prev) => ({ ...prev, [id]: String(e?.message || e) }));
        } finally {
            setUpdatingId(0);
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
            <div className="rounded-2xl border border-zinc-200/70 bg-white/65 px-3 py-3 backdrop-blur-sm dark:border-white/10 dark:bg-[#0f1116]/80">
                <div className="flex items-center justify-between">
                    <div>
                        <div className="text-[10px] font-semibold uppercase tracking-[0.18em] text-zinc-500 dark:text-white/45">POOL DATA</div>
                        <div className="mt-0.5 text-[15px] font-black text-zinc-900 dark:text-white">池子数据源</div>
                        <div className="mt-1 text-[11px] text-zinc-500 dark:text-white/40">
                            {groups.reduce((sum, group) => sum + (group?.sources?.length || 0), 0)} 个来源 · {groups.length} 个分组
                        </div>
                    </div>
                    <button type="button" onClick={load} disabled={loading}
                        className="rounded-xl px-3 py-1.5 text-[11px] font-semibold ring-1 ring-zinc-200 transition hover:bg-zinc-100 disabled:opacity-40 dark:ring-white/10 dark:hover:bg-white/10">
                        {loading ? '刷新中' : '刷新'}
                    </button>
                </div>
            </div>
            {error && <div className="rounded-xl bg-red-50 px-3 py-2 text-xs text-red-700 dark:bg-red-500/10 dark:text-red-300">{error}</div>}
            <AddSourceForm onAdd={handleAdd} adding={adding} error={addError} brand={brand} />
            {groups.map((group) => {
                const sources = Array.isArray(group?.sources) ? group.sources : [];
                const enabledCount = sources.filter((s) => s?.is_enabled).length;
                const errorCount = sources.filter((s) => s?.last_error).length;
                const hasCurrent = sources.some((s) => s?.is_current);
                const dotClass = errorCount > 0
                    ? 'bg-red-500'
                    : hasCurrent
                        ? 'bg-emerald-500'
                        : 'bg-zinc-400 dark:bg-zinc-500';
                return (
                <div key={`${group.chain}:${group.timeframe_minutes}`} className="rounded-2xl border border-zinc-200 bg-white/60 p-4 dark:border-white/10 dark:bg-white/[0.03]">
                    <div className="mb-3 flex items-center justify-between gap-3">
                        <div className="flex items-center gap-2">
                            <span className={`inline-block h-2 w-2 rounded-full ring-2 ring-white/70 dark:ring-[#0c0e12] ${dotClass}`} />
                            <div className="text-sm font-bold text-zinc-900 dark:text-white/90">
                                {(group.chain || 'bsc').toUpperCase()} · {group.timeframe_minutes || 5}m
                            </div>
                            <span className="rounded-full bg-zinc-100 px-1.5 py-0.5 text-[9px] font-semibold tabular-nums text-zinc-500 dark:bg-white/5 dark:text-white/45">
                                {enabledCount}/{sources.length}
                            </span>
                        </div>
                        <div className="truncate text-right text-[11px] text-zinc-500 dark:text-white/45">
                            当前 {group.effective_source?.name || group.env_fallback?.name || '--'}
                        </div>
                    </div>
                    {group.sources?.length ? (
                        <div className="space-y-2">
                            {group.sources.map((source) => (
                                <EditableSourceRow
                                    key={source.id}
                                    source={source}
                                    onSwitch={handleSwitch}
                                    onEnable={handleEnable}
                                    onDisable={handleDisable}
                                    onDelete={handleDelete}
                                    onCheck={handleCheck}
                                    onUpdate={handleUpdate}
                                    updating={updatingId === Number(source.id)}
                                    updateError={updateErrors[source.id]}
                                    brand={brand}
                                />
                            ))}
                        </div>
                    ) : (
                        <div className="rounded-xl border border-dashed border-zinc-200 px-3 py-4 text-center text-xs text-zinc-400 dark:border-white/10 dark:text-white/35">
                            当前使用 ENV 兜底：{group.env_fallback?.base_url_masked || group.env_fallback?.base_url || '--'}
                        </div>
                    )}
                </div>
                );
            })}
        </div>
    );
}
