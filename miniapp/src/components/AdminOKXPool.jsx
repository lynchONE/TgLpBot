import { useCallback, useEffect, useMemo, useState } from 'react';
import {
    AlertTriangle,
    CheckCircle2,
    Pencil,
    Plus,
    Power,
    RefreshCw,
    RotateCcw,
    Trash2,
} from 'lucide-react';
import {
    addAdminOKXConfig,
    checkAdminOKXConfig,
    deleteAdminOKXConfig,
    disableAdminOKXConfig,
    disableAdminOKXConfigNextMonth,
    enableAdminOKXConfig,
    fetchAdminOKXPool,
    renameAdminOKXConfig,
    switchAdminOKXConfig,
    updateAdminOKXConfig,
} from '../lib/api';
import { getBrandTheme } from '../lib/brand';

function formatTime(ts) {
    if (!ts) return '--';
    const d = new Date(ts);
    if (Number.isNaN(d.getTime())) return String(ts);
    return d.toLocaleString();
}

function formatSource(source) {
    const v = String(source || '').toLowerCase();
    if (v === 'db') return '配置池';
    if (v === 'env') return '.env';
    return String(source || '--');
}

function formatReason(reason) {
    const v = String(reason || '').toLowerCase();
    if (v === 'quota_exhausted') return '额度用尽';
    if (v === 'rate_limited') return '频率限制';
    if (v === 'health_fail') return '健康检查失败';
    if (v === 'auth_fail') return '认证失败';
    if (v === 'manual') return '手动禁用';
    return String(reason || '');
}

function deriveName(config) {
    const name = String(config?.name || '').trim();
    if (name) return name;
    const raw = String(config?.base_url || '').trim();
    if (!raw) return config?.id ? `#${config.id}` : '--';
    try {
        return new URL(raw).host;
    } catch {
        return raw;
    }
}

function isUnavailable(config) {
    return String(config?.status || '').toLowerCase() === 'unavailable' || !config?.is_enabled;
}

function emptyDraft() {
    return {
        name: '',
        baseUrl: 'https://www.okx.com/api/v6/dex/aggregator',
        apiKey: '',
        secretKey: '',
        passphrase: '',
        setCurrent: false,
    };
}

function FoldableError({ children }) {
    if (children === null || typeof children === 'undefined') return null;
    const text = String(children).trim();
    if (!text) return null;
    return (
        <details className="col-span-2 rounded-lg border border-red-500/20 bg-red-500/10 text-red-700 dark:text-red-200">
            <summary className="cursor-pointer list-none px-2.5 py-1.5 text-[11px] font-semibold">最近错误</summary>
            <div className="max-h-32 overflow-auto border-t border-red-500/10 px-2.5 py-2 text-[10px] leading-relaxed break-words">
                {text}
            </div>
        </details>
    );
}

function ActionButton({ children, icon: Icon, danger, active, className = '', ...props }) {
    const color = danger
        ? 'border-red-200 text-red-600 hover:bg-red-50 dark:border-red-500/25 dark:text-red-300 dark:hover:bg-red-500/10'
        : active
          ? 'border-emerald-200 text-emerald-700 hover:bg-emerald-50 dark:border-emerald-500/25 dark:text-emerald-300 dark:hover:bg-emerald-500/10'
          : 'border-zinc-200 text-zinc-600 hover:bg-zinc-50 dark:border-white/10 dark:text-white/65 dark:hover:bg-white/10';
    return (
        <button
            type="button"
            className={`inline-flex h-8 items-center justify-center gap-1.5 rounded-lg border px-2.5 text-[11px] font-semibold transition disabled:cursor-not-allowed disabled:opacity-40 ${color} ${className}`}
            {...props}
        >
            {Icon && <Icon className="h-3.5 w-3.5" />}
            <span>{children}</span>
        </button>
    );
}

function Field({ label, children }) {
    return (
        <label className="block space-y-1">
            <div className="text-[11px] font-medium text-zinc-500 dark:text-white/50">{label}</div>
            {children}
        </label>
    );
}

function AddConfigForm({ brand, adding, error, onAdd }) {
    const [open, setOpen] = useState(false);
    const [draft, setDraft] = useState(emptyDraft);

    const update = (key, value) => setDraft((prev) => ({ ...prev, [key]: value }));

    const submit = async () => {
        const ok = await onAdd?.(draft);
        if (ok) {
            setDraft(emptyDraft());
            setOpen(false);
        }
    };

    return (
        <div className={brand.cardCompactClass}>
            <button
                type="button"
                onClick={() => setOpen((v) => !v)}
                className="flex w-full items-center justify-between px-4 py-3 text-left"
            >
                <span className="inline-flex items-center gap-2 text-sm font-semibold text-zinc-900 dark:text-white/90">
                    <Plus className="h-4 w-4" />
                    添加 OKX 配置
                </span>
                <span className={`text-zinc-400 transition-transform ${open ? 'rotate-180' : ''}`}>⌄</span>
            </button>
            {open && (
                <div className="space-y-3 border-t border-zinc-100 px-4 py-3 dark:border-white/5">
                    <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
                        <Field label="名称">
                            <input
                                value={draft.name}
                                onChange={(e) => update('name', e.target.value)}
                                placeholder="留空使用域名"
                                className={`${brand.inputClass} w-full text-xs`}
                            />
                        </Field>
                        <Field label="Base URL">
                            <input
                                value={draft.baseUrl}
                                onChange={(e) => update('baseUrl', e.target.value)}
                                className={`${brand.inputClass} w-full text-xs`}
                            />
                        </Field>
                    </div>
                    <Field label="API Key">
                        <input
                            value={draft.apiKey}
                            onChange={(e) => update('apiKey', e.target.value)}
                            autoComplete="off"
                            className={`${brand.inputClass} w-full text-xs`}
                        />
                    </Field>
                    <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
                        <Field label="Secret Key">
                            <input
                                type="password"
                                value={draft.secretKey}
                                onChange={(e) => update('secretKey', e.target.value)}
                                autoComplete="new-password"
                                className={`${brand.inputClass} w-full text-xs`}
                            />
                        </Field>
                        <Field label="Passphrase">
                            <input
                                type="password"
                                value={draft.passphrase}
                                onChange={(e) => update('passphrase', e.target.value)}
                                autoComplete="new-password"
                                className={`${brand.inputClass} w-full text-xs`}
                            />
                        </Field>
                    </div>
                    <label className="flex items-center gap-2 text-xs text-zinc-600 dark:text-white/60">
                        <input
                            type="checkbox"
                            checked={Boolean(draft.setCurrent)}
                            onChange={(e) => update('setCurrent', e.target.checked)}
                        />
                        添加后设为当前配置
                    </label>
                    {error && <div className={brand.errorBoxClass}>{error}</div>}
                    <button
                        type="button"
                        onClick={submit}
                        disabled={
                            adding ||
                            !String(draft.baseUrl || '').trim() ||
                            !String(draft.apiKey || '').trim() ||
                            !String(draft.secretKey || '').trim() ||
                            !String(draft.passphrase || '').trim()
                        }
                        className={`inline-flex min-h-[40px] w-full items-center justify-center gap-2 rounded-xl px-3 text-sm font-semibold transition disabled:cursor-not-allowed disabled:opacity-50 ${brand.solidButtonClass}`}
                    >
                        {adding && <RefreshCw className="h-4 w-4 animate-spin" />}
                        {adding ? '添加中' : '添加配置'}
                    </button>
                </div>
            )}
        </div>
    );
}

function OKXConfigRow({ config, brand, busy, onRename, onUpdate, onSwitch, onDisable, onDisableNextMonth, onEnable, onDelete, onCheck }) {
    const [expanded, setExpanded] = useState(false);
    const [nameDraft, setNameDraft] = useState('');
    const [editDraft, setEditDraft] = useState({
        baseUrl: '',
        apiKey: '',
        secretKey: '',
        passphrase: '',
    });

    useEffect(() => {
        setNameDraft(deriveName(config));
        setEditDraft({
            baseUrl: String(config?.base_url || '').trim(),
            apiKey: '',
            secretKey: '',
            passphrase: '',
        });
    }, [config?.id, config?.name, config?.base_url]);

    const unavailable = isUnavailable(config);
    const latency = Number(config?.last_latency_ms || 0);
    const name = deriveName(config);

    return (
        <div className="rounded-xl border border-zinc-200/70 bg-white/80 dark:border-white/10 dark:bg-white/[0.03]">
            <button
                type="button"
                onClick={() => setExpanded((v) => !v)}
                className="flex w-full items-center gap-3 px-3 py-3 text-left"
            >
                <span className={`h-2.5 w-2.5 shrink-0 rounded-full ${unavailable ? 'bg-red-500' : 'bg-emerald-500'}`} />
                <span className="min-w-0 flex-1">
                    <span className="flex items-center gap-2">
                        <span className="truncate text-[13px] font-semibold text-zinc-900 dark:text-white/90">{name}</span>
                        {config?.is_current && (
                            <span className={brand.softButtonClass + ' rounded px-1.5 py-px text-[10px] font-bold'}>IN USE</span>
                        )}
                        {!config?.is_enabled && (
                            <span className="rounded bg-red-500/10 px-1.5 py-px text-[10px] font-bold text-red-600 dark:text-red-300">OFF</span>
                        )}
                    </span>
                    <span className="mt-0.5 block truncate text-[11px] text-zinc-400 dark:text-white/35">
                        {config?.base_url_masked || config?.base_url || '--'} · {config?.api_key_masked || 'no key'}
                    </span>
                </span>
                {latency > 0 && (
                    <span className="font-mono text-[11px] text-zinc-500 dark:text-white/45">{latency}ms</span>
                )}
                <span className={`text-zinc-400 transition-transform ${expanded ? 'rotate-180' : ''}`}>⌄</span>
            </button>

            {expanded && (
                <div className="space-y-3 border-t border-zinc-100 px-3 py-3 dark:border-white/5">
                    <div className="grid grid-cols-2 gap-x-3 gap-y-1 text-[11px] text-zinc-500 dark:text-white/45">
                        <div>状态：{unavailable ? '不可用' : '可用'}</div>
                        <div>连续失败：{Number(config?.consecutive_failures || 0)}</div>
                        <div>最近检查：{formatTime(config?.last_checked_at)}</div>
                        <div>最近成功：{formatTime(config?.last_success_at)}</div>
                        {config?.disabled_until && (
                            <div className="col-span-2 text-amber-700 dark:text-amber-300">
                                禁用至：{formatTime(config.disabled_until)}
                                {config?.disabled_reason ? ` · ${formatReason(config.disabled_reason)}` : ''}
                            </div>
                        )}
                        <FoldableError>{config?.last_error}</FoldableError>
                    </div>

                    <div className="grid grid-cols-[minmax(0,1fr)_auto] gap-2">
                        <input
                            value={nameDraft}
                            onChange={(e) => setNameDraft(e.target.value)}
                            className={`${brand.inputClass} min-w-0 text-xs`}
                        />
                        <ActionButton
                            icon={Pencil}
                            disabled={busy || !String(nameDraft || '').trim()}
                            onClick={() => onRename?.(config.id, nameDraft)}
                        >
                            改名
                        </ActionButton>
                    </div>

                    <div className="rounded-xl border border-zinc-100 bg-zinc-50/70 p-3 dark:border-white/5 dark:bg-white/[0.03]">
                        <div className="mb-2 text-[11px] font-semibold text-zinc-500 dark:text-white/50">更新连接信息</div>
                        <div className="space-y-2">
                            <input
                                value={editDraft.baseUrl}
                                onChange={(e) => setEditDraft((prev) => ({ ...prev, baseUrl: e.target.value }))}
                                className={`${brand.inputClass} w-full text-xs`}
                            />
                            <input
                                value={editDraft.apiKey}
                                onChange={(e) => setEditDraft((prev) => ({ ...prev, apiKey: e.target.value }))}
                                placeholder="API Key 留空则不改"
                                autoComplete="off"
                                className={`${brand.inputClass} w-full text-xs`}
                            />
                            <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
                                <input
                                    type="password"
                                    value={editDraft.secretKey}
                                    onChange={(e) => setEditDraft((prev) => ({ ...prev, secretKey: e.target.value }))}
                                    placeholder="Secret 留空则不改"
                                    autoComplete="new-password"
                                    className={`${brand.inputClass} w-full text-xs`}
                                />
                                <input
                                    type="password"
                                    value={editDraft.passphrase}
                                    onChange={(e) => setEditDraft((prev) => ({ ...prev, passphrase: e.target.value }))}
                                    placeholder="Passphrase 留空则不改"
                                    autoComplete="new-password"
                                    className={`${brand.inputClass} w-full text-xs`}
                                />
                            </div>
                            <ActionButton
                                icon={RotateCcw}
                                disabled={busy || !String(editDraft.baseUrl || '').trim()}
                                onClick={() => onUpdate?.(config.id, { ...editDraft, name: nameDraft })}
                            >
                                保存更新
                            </ActionButton>
                        </div>
                    </div>

                    <div className="flex flex-wrap gap-1.5">
                        <ActionButton icon={RefreshCw} disabled={busy} onClick={() => onCheck?.(config.id)}>
                            检测
                        </ActionButton>
                        <ActionButton
                            icon={CheckCircle2}
                            active
                            disabled={busy || unavailable || config?.is_current}
                            onClick={() => onSwitch?.(config.id)}
                        >
                            切换
                        </ActionButton>
                        {unavailable ? (
                            <ActionButton icon={Power} active disabled={busy} onClick={() => onEnable?.(config.id)}>
                                启用
                            </ActionButton>
                        ) : (
                            <ActionButton icon={Power} disabled={busy} onClick={() => onDisable?.(config.id)}>
                                停用
                            </ActionButton>
                        )}
                        <ActionButton icon={AlertTriangle} disabled={busy || unavailable} onClick={() => onDisableNextMonth?.(config.id)}>
                            禁用到下月
                        </ActionButton>
                        <ActionButton icon={Trash2} danger disabled={busy} onClick={() => onDelete?.(config.id, name)}>
                            删除
                        </ActionButton>
                    </div>
                </div>
            )}
        </div>
    );
}

export default function AdminOKXPool({ apiBaseUrl, initData, hasInitData, pollIntervalSec = 15, accentTheme = 'lime', onNotice }) {
    const brand = useMemo(() => getBrandTheme(accentTheme), [accentTheme]);
    const [data, setData] = useState(null);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState('');
    const [addError, setAddError] = useState('');
    const [adding, setAdding] = useState(false);
    const [busyID, setBusyID] = useState(0);

    const configs = useMemo(() => (Array.isArray(data?.configs) ? data.configs : []), [data]);

    const load = useCallback(async () => {
        if (!hasInitData) return;
        setLoading(true);
        setError('');
        try {
            setData(await fetchAdminOKXPool({ apiBaseUrl, initData }));
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
        if (!hasInitData) return undefined;
        const cadence = Math.max(5, Number(pollIntervalSec) || 15) * 1000;
        const timer = setInterval(load, cadence);
        return () => clearInterval(timer);
    }, [hasInitData, load, pollIntervalSec]);

    const applyResult = useCallback((next) => {
        if (next) setData(next);
    }, []);

    const notify = useCallback((message, type = 'success') => {
        if (onNotice) onNotice(message, type);
    }, [onNotice]);

    const runAction = useCallback(async (id, action, successMessage) => {
        setBusyID(Number(id) || -1);
        try {
            const next = await action();
            applyResult(next);
            notify(successMessage);
        } catch (e) {
            notify(String(e?.message || e), 'error');
        } finally {
            setBusyID(0);
        }
    }, [applyResult, notify]);

    const handleAdd = useCallback(async (draft) => {
        setAdding(true);
        setAddError('');
        try {
            const next = await addAdminOKXConfig({ apiBaseUrl, initData, ...draft });
            applyResult(next);
            notify('OKX 配置已添加');
            return true;
        } catch (e) {
            setAddError(String(e?.message || e));
            return false;
        } finally {
            setAdding(false);
        }
    }, [apiBaseUrl, initData, applyResult, notify]);

    const handleDelete = useCallback((id, name) => {
        if (!window.confirm(`删除 OKX 配置 "${name}"？`)) return;
        runAction(id, () => deleteAdminOKXConfig({ apiBaseUrl, initData, configId: id }), 'OKX 配置已删除');
    }, [apiBaseUrl, initData, runAction]);

    return (
        <div className="space-y-3">
            <div className={brand.cardClass + ' p-4'}>
                <div className="flex items-start justify-between gap-3">
                    <div className="min-w-0">
                        <div className="text-[11px] font-semibold uppercase tracking-[0.16em] text-zinc-400 dark:text-white/35">
                            OKX API Pool
                        </div>
                        <div className="mt-1 flex items-center gap-2">
                            <span className="text-sm font-semibold text-zinc-900 dark:text-white/90">
                                当前：{formatSource(data?.effective_source)}
                            </span>
                            {data?.effective_config_id && (
                                <span className={brand.softButtonClass + ' rounded px-2 py-0.5 text-[10px] font-bold'}>
                                    #{data.effective_config_id}
                                </span>
                            )}
                        </div>
                        <div className="mt-1 truncate text-xs text-zinc-500 dark:text-white/50">
                            {data?.effective_base_url_masked || data?.effective_base_url || '--'}
                            {data?.effective_api_key_masked ? ` · ${data.effective_api_key_masked}` : ''}
                        </div>
                    </div>
                    <ActionButton icon={RefreshCw} disabled={loading} onClick={load}>
                        {loading ? '刷新中' : '刷新'}
                    </ActionButton>
                </div>
                {data?.env_base_url && (
                    <div className="mt-3 rounded-xl border border-zinc-100 bg-zinc-50/80 px-3 py-2 text-[11px] text-zinc-500 dark:border-white/5 dark:bg-white/[0.03] dark:text-white/45">
                        .env 备用：{data.env_base_url_masked || data.env_base_url}
                        {data?.env_api_key_masked ? ` · ${data.env_api_key_masked}` : ''}
                    </div>
                )}
            </div>

            <AddConfigForm brand={brand} adding={adding} error={addError} onAdd={handleAdd} />

            {error && <div className={brand.errorBoxClass}>{error}</div>}

            <div className="space-y-2">
                {configs.map((item) => (
                    <OKXConfigRow
                        key={item.id}
                        config={item}
                        brand={brand}
                        busy={busyID === item.id || busyID === -1}
                        onRename={(id, name) =>
                            runAction(id, () => renameAdminOKXConfig({ apiBaseUrl, initData, configId: id, name }), 'OKX 配置已改名')
                        }
                        onUpdate={(id, draft) =>
                            runAction(
                                id,
                                () =>
                                    updateAdminOKXConfig({
                                        apiBaseUrl,
                                        initData,
                                        configId: id,
                                        name: draft.name,
                                        baseUrl: draft.baseUrl,
                                        apiKey: draft.apiKey,
                                        secretKey: draft.secretKey,
                                        passphrase: draft.passphrase,
                                    }),
                                'OKX 配置已更新',
                            )
                        }
                        onSwitch={(id) =>
                            runAction(id, () => switchAdminOKXConfig({ apiBaseUrl, initData, configId: id }), 'OKX 当前配置已切换')
                        }
                        onDisable={(id) =>
                            runAction(id, () => disableAdminOKXConfig({ apiBaseUrl, initData, configId: id }), 'OKX 配置已停用')
                        }
                        onDisableNextMonth={(id) =>
                            runAction(id, () => disableAdminOKXConfigNextMonth({ apiBaseUrl, initData, configId: id }), 'OKX 配置已禁用到下月')
                        }
                        onEnable={(id) =>
                            runAction(id, () => enableAdminOKXConfig({ apiBaseUrl, initData, configId: id }), 'OKX 配置已启用')
                        }
                        onDelete={handleDelete}
                        onCheck={(id) =>
                            runAction(id, () => checkAdminOKXConfig({ apiBaseUrl, initData, configId: id }), 'OKX 配置检测已完成')
                        }
                    />
                ))}
            </div>

            {!loading && configs.length === 0 && (
                <div className={brand.emptyStateClass}>
                    <div className="text-sm font-semibold">还没有 OKX 配置</div>
                    <div className="text-xs">未添加配置时会继续使用 .env 中的 OKX 设置。</div>
                </div>
            )}
        </div>
    );
}
