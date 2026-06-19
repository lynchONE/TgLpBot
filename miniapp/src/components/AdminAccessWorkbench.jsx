import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
    createAdminAuthCode,
    disableAdminAuthCode,
    enableAdminAuthCode,
    fetchAdminAccessList,
    fetchAdminAnnouncements,
    fetchAdminAuthCodes,
    publishAdminAnnouncement,
    restoreAdminUserAccess,
    revokeAdminUserAccess,
    updateAdminAuthCode,
    updateAdminUserAccess,
} from '../lib/api';

const SECTIONS = [
    { key: 'users', label: '用户' },
    { key: 'codes', label: '授权码' },
    { key: 'announcements', label: '公告' },
];

function errorText(err) {
    return String(err?.message || err || '').trim();
}

function formatDateTime(value) {
    if (!value) return '--';
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) return String(value);
    return date.toLocaleString();
}

function formatDateInput(value) {
    if (!value) return '';
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) return '';
    const pad = (n) => String(n).padStart(2, '0');
    return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`;
}

function formatUserLabel(user) {
    const first = String(user?.first_name || '').trim();
    const last = String(user?.last_name || '').trim();
    const username = String(user?.username || '').trim();
    const fullName = [first, last].filter(Boolean).join(' ');
    if (fullName && username) return `${fullName} (@${username})`;
    if (fullName) return fullName;
    if (username) return `@${username}`;
    return `用户 #${user?.user_id || '--'}`;
}

function statusLabel(status) {
    switch (String(status || '').toLowerCase()) {
        case 'active':
            return '生效中';
        case 'revoked':
        case 'disabled':
            return '已停用';
        case 'expired':
            return '已过期';
        case 'exhausted':
            return '已用完';
        case 'pending':
            return '未生效';
        default:
            return status || '--';
    }
}

function statusClass(status) {
    const value = String(status || '').toLowerCase();
    if (value === 'active') return 'border-emerald-500/25 bg-emerald-500/10 text-emerald-700 dark:text-emerald-200';
    if (value === 'revoked' || value === 'disabled' || value === 'expired' || value === 'exhausted') {
        return 'border-red-500/25 bg-red-500/10 text-red-700 dark:text-red-200';
    }
    return 'border-amber-500/25 bg-amber-500/10 text-amber-700 dark:text-amber-200';
}

function normalizeModuleKeys(value) {
    if (!Array.isArray(value)) return [];
    const seen = new Set();
    const keys = [];
    value.forEach((item) => {
        const key = String(item || '').trim();
        if (!key || seen.has(key)) return;
        seen.add(key);
        keys.push(key);
    });
    return keys;
}

function positiveInt(value, min) {
    const n = Number(value);
    if (!Number.isFinite(n)) return min;
    return Math.max(min, Math.trunc(n));
}

function moduleSummary(keys, modules) {
    const normalized = normalizeModuleKeys(keys);
    if (!normalized.length) return '未授权模块';
    const byKey = new Map((modules || []).map((item) => [String(item.key || '').trim(), item]));
    const labels = normalized.slice(0, 2).map((key) => byKey.get(key)?.label || key);
    const rest = normalized.length - labels.length;
    return rest > 0 ? `${labels.join('、')} +${rest}` : labels.join('、');
}

function applyModulePayload(data, setModuleCatalog, setGrantableModules) {
    if (Array.isArray(data?.module_catalog)) setModuleCatalog(data.module_catalog);
    if (Array.isArray(data?.grantable_modules)) setGrantableModules(data.grantable_modules);
}

function makeAccessDraft(user) {
    return {
        activeTo: formatDateInput(user?.active_to),
        clearActiveTo: !user?.active_to,
        maxWallets: String(Number(user?.max_wallets || 1)),
        maxActiveTasks: String(Number(user?.max_active_tasks || 1)),
        miniAppEnabled: Boolean(user?.mini_app_enabled),
        enabledModules: normalizeModuleKeys(user?.enabled_modules),
        note: String(user?.note || ''),
    };
}

function makeCodeDraft(moduleKeys) {
    return {
        activeTo: '',
        maxRedemptions: '1',
        maxWallets: '1',
        maxActiveTasks: '1',
        miniAppEnabled: true,
        enabledModules: normalizeModuleKeys(moduleKeys),
        note: '',
    };
}

const inputClass = 'min-h-[38px] rounded-xl border border-zinc-200 bg-white/85 px-3 text-xs text-zinc-900 outline-none focus:border-lime-400 dark:border-white/10 dark:bg-white/5 dark:text-white/90';
const buttonClass = 'rounded-xl border border-zinc-200 bg-white/80 px-3 py-2 text-xs font-semibold text-zinc-700 shadow-sm transition active:scale-[0.98] dark:border-white/10 dark:bg-white/10 dark:text-white/80';
const primaryButtonClass = 'rounded-xl bg-zinc-950 px-3 py-2 text-xs font-semibold text-white shadow-sm transition active:scale-[0.98] dark:bg-lime-300 dark:text-zinc-950';
const dangerButtonClass = 'rounded-xl border border-red-500/20 bg-red-500/10 px-3 py-2 text-xs font-semibold text-red-700 transition active:scale-[0.98] dark:text-red-200';

function Field({ label, children }) {
    return (
        <label className="flex min-w-0 flex-col gap-1">
            <span className="text-[10px] font-medium text-zinc-500 dark:text-white/45">{label}</span>
            {children}
        </label>
    );
}

function LimitFields({ draft, onChange, includeRedemptions = false }) {
    return (
        <div className="grid grid-cols-2 gap-2">
            <Field label="有效期">
                <input
                    className={inputClass}
                    type="date"
                    value={draft.activeTo}
                    disabled={Boolean(draft.clearActiveTo)}
                    onChange={(event) => onChange({ activeTo: event.target.value })}
                />
            </Field>
            {'clearActiveTo' in draft ? (
                <Field label="长期有效">
                    <label className="flex min-h-[38px] items-center gap-2 rounded-xl border border-zinc-200 bg-white/60 px-3 text-xs text-zinc-600 dark:border-white/10 dark:bg-white/5 dark:text-white/65">
                        <input type="checkbox" checked={Boolean(draft.clearActiveTo)} onChange={(event) => onChange({ clearActiveTo: event.target.checked })} />
                        永久
                    </label>
                </Field>
            ) : null}
            {includeRedemptions ? (
                <Field label="兑换次数">
                    <input className={inputClass} type="number" min="1" value={draft.maxRedemptions} onChange={(event) => onChange({ maxRedemptions: event.target.value })} />
                </Field>
            ) : null}
            <Field label="钱包数">
                <input className={inputClass} type="number" min="0" value={draft.maxWallets} onChange={(event) => onChange({ maxWallets: event.target.value })} />
            </Field>
            <Field label="任务数">
                <input className={inputClass} type="number" min="0" value={draft.maxActiveTasks} onChange={(event) => onChange({ maxActiveTasks: event.target.value })} />
            </Field>
            <Field label="MiniApp">
                <label className="flex min-h-[38px] items-center gap-2 rounded-xl border border-zinc-200 bg-white/60 px-3 text-xs text-zinc-600 dark:border-white/10 dark:bg-white/5 dark:text-white/65">
                    <input type="checkbox" checked={Boolean(draft.miniAppEnabled)} onChange={(event) => onChange({ miniAppEnabled: event.target.checked })} />
                    允许
                </label>
            </Field>
        </div>
    );
}

function ModulePicker({ modules, value, onChange, compact = false }) {
    const selected = useMemo(() => new Set(normalizeModuleKeys(value)), [value]);
    const moduleKeys = useMemo(() => modules.map((item) => String(item.key || '').trim()).filter(Boolean), [modules]);
    const groups = useMemo(() => {
        const map = new Map();
        modules.forEach((item) => {
            const group = String(item.group || '其他').trim() || '其他';
            if (!map.has(group)) map.set(group, []);
            map.get(group).push(item);
        });
        return Array.from(map.entries()).map(([group, items]) => ({ group, items }));
    }, [modules]);

    const emit = useCallback((keys) => {
        onChange(normalizeModuleKeys(keys).filter((key) => moduleKeys.includes(key)));
    }, [moduleKeys, onChange]);

    const toggleKey = (key) => {
        const next = new Set(selected);
        if (next.has(key)) next.delete(key);
        else next.add(key);
        emit(Array.from(next));
    };

    if (!modules.length) {
        return <div className="rounded-xl border border-dashed border-zinc-200 p-3 text-xs text-zinc-500 dark:border-white/10 dark:text-white/45">模块目录为空</div>;
    }

    return (
        <div className="space-y-2 rounded-2xl border border-zinc-200 bg-zinc-50/80 p-3 dark:border-white/10 dark:bg-white/5">
            <div className="flex items-center justify-between gap-2">
                <div>
                    <div className="text-xs font-semibold text-zinc-900 dark:text-white/90">功能模块</div>
                    <div className="mt-0.5 text-[10px] text-zinc-500 dark:text-white/40">{selected.size}/{modules.length}</div>
                </div>
                <div className="flex gap-1.5">
                    <button type="button" className={buttonClass} onClick={() => emit(moduleKeys)}>全选</button>
                    <button type="button" className={buttonClass} onClick={() => emit([])}>清空</button>
                </div>
            </div>
            {groups.map(({ group, items }) => (
                <div key={group} className="space-y-1.5">
                    <div className="text-[10px] font-semibold text-zinc-500 dark:text-white/45">{group}</div>
                    <div className="grid grid-cols-2 gap-1.5">
                        {items.map((item) => {
                            const key = String(item.key || '').trim();
                            const checked = selected.has(key);
                            return (
                                <button
                                    key={key}
                                    type="button"
                                    onClick={() => toggleKey(key)}
                                    className={`min-w-0 rounded-xl border px-2.5 py-2 text-left transition ${checked
                                        ? 'border-lime-400 bg-lime-300/15 text-zinc-950 dark:text-lime-100'
                                        : 'border-zinc-200 bg-white/70 text-zinc-600 dark:border-white/10 dark:bg-white/5 dark:text-white/65'
                                    }`}
                                >
                                    <div className="truncate text-[11px] font-bold">{item.label || key}</div>
                                    {!compact ? <div className="mt-0.5 truncate text-[9px] opacity-60">{key}</div> : null}
                                </button>
                            );
                        })}
                    </div>
                </div>
            ))}
        </div>
    );
}

export default function AdminAccessWorkbench({ apiBaseUrl, initData, hasInitData, onNotice }) {
    const [section, setSection] = useState('users');
    const [notice, setNotice] = useState('');

    const [moduleCatalog, setModuleCatalog] = useState([]);
    const [grantableModules, setGrantableModules] = useState([]);

    const [users, setUsers] = useState([]);
    const [usersTotal, setUsersTotal] = useState(0);
    const [usersQuery, setUsersQuery] = useState('');
    const [usersLoading, setUsersLoading] = useState(false);
    const [usersError, setUsersError] = useState('');
    const [selectedUser, setSelectedUser] = useState(null);
    const [accessDraft, setAccessDraft] = useState(makeAccessDraft(null));
    const [accessSaving, setAccessSaving] = useState(false);

    const [codes, setCodes] = useState([]);
    const [codesTotal, setCodesTotal] = useState(0);
    const [codesLoading, setCodesLoading] = useState(false);
    const [codesError, setCodesError] = useState('');
    const [codeDraft, setCodeDraft] = useState(makeCodeDraft([]));
    const [createdCode, setCreatedCode] = useState('');
    const [editingCodeId, setEditingCodeId] = useState(null);

    const [announcements, setAnnouncements] = useState([]);
    const [annLoading, setAnnLoading] = useState(false);
    const [annError, setAnnError] = useState('');
    const [announcementDraft, setAnnouncementDraft] = useState({ title: '系统公告', content: '' });
    const [publishing, setPublishing] = useState(false);

    const codeModulesInitializedRef = useRef(false);
    const moduleKeys = useMemo(() => grantableModules.map((item) => String(item.key || '').trim()).filter(Boolean), [grantableModules]);

    useEffect(() => {
        if (codeModulesInitializedRef.current || moduleKeys.length === 0) return;
        codeModulesInitializedRef.current = true;
        setCodeDraft((prev) => ({ ...prev, enabledModules: moduleKeys }));
    }, [moduleKeys]);

    const showNotice = useCallback((message) => {
        const text = String(message || '').trim();
        if (!text) return;
        setNotice(text);
        onNotice?.(text, 'success');
        window.setTimeout(() => setNotice(''), 2600);
    }, [onNotice]);

    const loadUsers = useCallback(async () => {
        if (!hasInitData) return;
        setUsersLoading(true);
        setUsersError('');
        try {
            const data = await fetchAdminAccessList({ apiBaseUrl, initData, pageSize: 24, query: usersQuery });
            applyModulePayload(data, setModuleCatalog, setGrantableModules);
            const items = Array.isArray(data?.items) ? data.items : [];
            setUsers(items);
            setUsersTotal(Number(data?.total || items.length));
            if (selectedUser?.user_id) {
                const next = items.find((item) => Number(item.user_id) === Number(selectedUser.user_id));
                if (next) {
                    setSelectedUser(next);
                    setAccessDraft(makeAccessDraft(next));
                }
            }
        } catch (err) {
            setUsersError(errorText(err));
        } finally {
            setUsersLoading(false);
        }
    }, [apiBaseUrl, hasInitData, initData, selectedUser?.user_id, usersQuery]);

    const loadCodes = useCallback(async () => {
        if (!hasInitData) return;
        setCodesLoading(true);
        setCodesError('');
        try {
            const data = await fetchAdminAuthCodes({ apiBaseUrl, initData, pageSize: 24 });
            applyModulePayload(data, setModuleCatalog, setGrantableModules);
            const items = Array.isArray(data?.items) ? data.items : [];
            setCodes(items);
            setCodesTotal(Number(data?.total || items.length));
        } catch (err) {
            setCodesError(errorText(err));
        } finally {
            setCodesLoading(false);
        }
    }, [apiBaseUrl, hasInitData, initData]);

    const loadAnnouncements = useCallback(async () => {
        if (!hasInitData) return;
        setAnnLoading(true);
        setAnnError('');
        try {
            const data = await fetchAdminAnnouncements({ apiBaseUrl, initData, pageSize: 20 });
            setAnnouncements(Array.isArray(data?.items) ? data.items : []);
        } catch (err) {
            setAnnError(errorText(err));
        } finally {
            setAnnLoading(false);
        }
    }, [apiBaseUrl, hasInitData, initData]);

    useEffect(() => {
        if (section === 'users') loadUsers();
        if (section === 'codes') loadCodes();
        if (section === 'announcements') loadAnnouncements();
    }, [loadAnnouncements, loadCodes, loadUsers, section]);

    useEffect(() => {
        const next = selectedUser || users[0] || null;
        if (!next) return;
        if (selectedUser && Number(selectedUser.user_id) === Number(next.user_id)) return;
        setSelectedUser(next);
        setAccessDraft(makeAccessDraft(next));
    }, [selectedUser, users]);

    const patchAccessDraft = (patch) => setAccessDraft((prev) => ({ ...prev, ...patch }));
    const patchCodeDraft = (patch) => setCodeDraft((prev) => ({ ...prev, ...patch }));
    const updateCodeRow = (codeId, patch) => {
        setCodes((prev) => prev.map((item) => Number(item.id) === Number(codeId) ? { ...item, ...patch } : item));
    };

    const saveSelectedAccess = useCallback(async () => {
        if (!selectedUser?.user_id) return;
        setAccessSaving(true);
        setUsersError('');
        try {
            const payload = {
                max_wallets: positiveInt(accessDraft.maxWallets, 0),
                max_active_tasks: positiveInt(accessDraft.maxActiveTasks, 0),
                mini_app_enabled: Boolean(accessDraft.miniAppEnabled),
                enabled_modules: normalizeModuleKeys(accessDraft.enabledModules),
                note: accessDraft.note,
            };
            if (accessDraft.clearActiveTo) payload.clear_active_to = true;
            else if (String(accessDraft.activeTo || '').trim()) payload.active_to = String(accessDraft.activeTo).trim();
            const data = await updateAdminUserAccess({ apiBaseUrl, initData, userId: selectedUser.user_id, patch: payload });
            setSelectedUser(data);
            setAccessDraft(makeAccessDraft(data));
            showNotice('用户授权已更新');
            await loadUsers();
        } catch (err) {
            setUsersError(errorText(err));
        } finally {
            setAccessSaving(false);
        }
    }, [accessDraft, apiBaseUrl, initData, loadUsers, selectedUser?.user_id, showNotice]);

    const createCode = useCallback(async () => {
        setCodesLoading(true);
        setCodesError('');
        try {
            const payload = {
                max_redemptions: positiveInt(codeDraft.maxRedemptions, 1),
                max_wallets: positiveInt(codeDraft.maxWallets, 0),
                max_active_tasks: positiveInt(codeDraft.maxActiveTasks, 0),
                mini_app_enabled: Boolean(codeDraft.miniAppEnabled),
                enabled_modules: normalizeModuleKeys(codeDraft.enabledModules),
                note: codeDraft.note,
            };
            if (String(codeDraft.activeTo || '').trim()) payload.active_to = String(codeDraft.activeTo).trim();
            const data = await createAdminAuthCode({ apiBaseUrl, initData, payload });
            setCreatedCode(String(data?.code?.code || ''));
            setCodeDraft(makeCodeDraft(moduleKeys));
            showNotice('授权码已生成');
            await loadCodes();
        } catch (err) {
            setCodesError(errorText(err));
        } finally {
            setCodesLoading(false);
        }
    }, [apiBaseUrl, codeDraft, initData, loadCodes, moduleKeys, showNotice]);

    const saveCode = useCallback(async (code) => {
        const payload = {
            max_redemptions: positiveInt(code.max_redemptions, 1),
            max_wallets: positiveInt(code.max_wallets, 0),
            max_active_tasks: positiveInt(code.max_active_tasks, 0),
            mini_app_enabled: Boolean(code.mini_app_enabled),
            enabled_modules: normalizeModuleKeys(code.enabled_modules),
            note: String(code.note || ''),
        };
        if (code.active_to) payload.active_to = formatDateInput(code.active_to);
        if (!code.active_to) payload.clear_active_to = true;
        await updateAdminAuthCode({ apiBaseUrl, initData, codeId: code.id, patch: payload });
        setEditingCodeId(null);
        showNotice('授权码已更新');
        await loadCodes();
    }, [apiBaseUrl, initData, loadCodes, showNotice]);

    const publishAnnouncement = useCallback(async () => {
        if (!window.confirm('确认向 Telegram 用户广播这条公告？')) return;
        setPublishing(true);
        setAnnError('');
        try {
            const data = await publishAdminAnnouncement({
                apiBaseUrl,
                initData,
                title: announcementDraft.title,
                content: announcementDraft.content,
            });
            setAnnouncementDraft({ title: '系统公告', content: '' });
            showNotice(`公告已发布，成功 ${Number(data?.sent_count || 0)}，失败 ${Number(data?.failed_count || 0)}`);
            await loadAnnouncements();
        } catch (err) {
            setAnnError(errorText(err));
        } finally {
            setPublishing(false);
        }
    }, [announcementDraft, apiBaseUrl, initData, loadAnnouncements, showNotice]);

    return (
        <div className="space-y-3">
            {notice ? <div className="rounded-xl border border-emerald-400/40 bg-emerald-400/10 p-3 text-xs font-semibold text-emerald-700 dark:text-emerald-200">{notice}</div> : null}
            <div className="rounded-3xl border border-zinc-200 bg-white/80 p-4 shadow-sm dark:border-white/10 dark:bg-white/5">
                <div className="text-[11px] font-semibold uppercase tracking-[0.22em] text-lime-600 dark:text-lime-300">Access</div>
                <div className="mt-1 flex items-end justify-between gap-3">
                    <div>
                        <div className="text-xl font-black text-zinc-950 dark:text-white">授权工作台</div>
                        <div className="mt-1 text-xs text-zinc-500 dark:text-white/45">{grantableModules.length} 个可授权模块</div>
                    </div>
                    <div className="text-right text-xs text-zinc-500 dark:text-white/45">
                        <div>用户 {usersTotal || users.length}</div>
                        <div>授权码 {codesTotal || codes.length}</div>
                    </div>
                </div>
            </div>

            <div className="grid grid-cols-3 gap-1 rounded-2xl border border-zinc-200 bg-white/70 p-1 shadow-sm dark:border-white/10 dark:bg-white/5 dark:shadow-none">
                {SECTIONS.map((item) => (
                    <button
                        key={item.key}
                        type="button"
                        onClick={() => setSection(item.key)}
                        className={`rounded-xl px-2 py-2 text-[11px] font-semibold transition ${section === item.key ? 'bg-zinc-950 text-white dark:bg-lime-300 dark:text-zinc-950' : 'text-zinc-600 hover:bg-zinc-100 dark:text-white/65 dark:hover:bg-white/10'}`}
                    >
                        {item.label}
                    </button>
                ))}
            </div>

            {section === 'users' ? (
                <div className="space-y-3">
                    <div className="rounded-2xl border border-zinc-200 bg-white/70 p-3 dark:border-white/10 dark:bg-white/5">
                        <div className="flex gap-2">
                            <input className={`${inputClass} flex-1`} value={usersQuery} onChange={(event) => setUsersQuery(event.target.value)} placeholder="@username / Telegram ID" />
                            <button type="button" className={buttonClass} disabled={usersLoading} onClick={loadUsers}>查询</button>
                        </div>
                        {usersError ? <div className="mt-2 rounded-xl border border-red-400/40 bg-red-400/10 p-2 text-xs text-red-700 dark:text-red-200">{usersError}</div> : null}
                        <div className="mt-3 space-y-2">
                            {users.length > 0 ? users.map((user) => (
                                <button
                                    type="button"
                                    key={user.user_id}
                                    className={`w-full rounded-2xl border p-3 text-left transition ${Number(user.user_id) === Number(selectedUser?.user_id) ? 'border-lime-400 bg-lime-300/10' : 'border-zinc-200 bg-white/70 dark:border-white/10 dark:bg-white/5'}`}
                                    onClick={() => {
                                        setSelectedUser(user);
                                        setAccessDraft(makeAccessDraft(user));
                                    }}
                                >
                                    <div className="flex items-start justify-between gap-2">
                                        <div className="min-w-0">
                                            <div className="truncate text-sm font-semibold text-zinc-900 dark:text-white/90">{formatUserLabel(user)}</div>
                                            <div className="mt-1 text-xs text-zinc-500 dark:text-white/45">TG {user.telegram_id || '--'}</div>
                                            <div className="mt-1 truncate text-[10px] text-zinc-400 dark:text-white/35">{moduleSummary(user.enabled_modules, moduleCatalog)}</div>
                                        </div>
                                        <span className={`shrink-0 rounded-full border px-2 py-1 text-[10px] font-semibold ${statusClass(user.status)}`}>{statusLabel(user.status)}</span>
                                    </div>
                                </button>
                            )) : <div className="p-3 text-center text-xs text-zinc-500">{usersLoading ? '正在加载用户...' : '暂无用户'}</div>}
                        </div>
                    </div>

                    <div className="rounded-2xl border border-zinc-200 bg-white/70 p-3 dark:border-white/10 dark:bg-white/5">
                        <div className="text-sm font-semibold text-zinc-900 dark:text-white/90">编辑授权</div>
                        {selectedUser ? (
                            <div className="mt-3 space-y-3">
                                <LimitFields draft={accessDraft} onChange={patchAccessDraft} />
                                <Field label="备注">
                                    <input className={inputClass} value={accessDraft.note} onChange={(event) => patchAccessDraft({ note: event.target.value })} />
                                </Field>
                                <ModulePicker modules={grantableModules} value={accessDraft.enabledModules} onChange={(enabledModules) => patchAccessDraft({ enabledModules })} />
                                <div className="flex flex-wrap gap-2">
                                    <button type="button" className={primaryButtonClass} disabled={accessSaving} onClick={saveSelectedAccess}>{accessSaving ? '保存中...' : '保存授权'}</button>
                                    {selectedUser.status === 'revoked' ? (
                                        <button type="button" className={buttonClass} onClick={async () => { await restoreAdminUserAccess({ apiBaseUrl, initData, userId: selectedUser.user_id }); showNotice('授权已恢复'); await loadUsers(); }}>恢复授权</button>
                                    ) : (
                                        <button type="button" className={dangerButtonClass} onClick={async () => { if (!window.confirm('确认停用该用户授权？')) return; await revokeAdminUserAccess({ apiBaseUrl, initData, userId: selectedUser.user_id }); showNotice('授权已停用'); await loadUsers(); }}>停用授权</button>
                                    )}
                                </div>
                            </div>
                        ) : <div className="mt-3 text-xs text-zinc-500">请选择一个用户</div>}
                    </div>
                </div>
            ) : null}

            {section === 'codes' ? (
                <div className="space-y-3">
                    <div className="rounded-2xl border border-zinc-200 bg-white/70 p-3 dark:border-white/10 dark:bg-white/5">
                        <div className="flex items-center justify-between">
                            <div className="text-sm font-semibold text-zinc-900 dark:text-white/90">生成授权码</div>
                            <div className="text-xs text-zinc-500">共 {codesTotal} 个</div>
                        </div>
                        {createdCode ? (
                            <div className="mt-2 flex items-center justify-between gap-2 rounded-xl border border-emerald-400/40 bg-emerald-400/10 p-2 text-xs text-emerald-700 dark:text-emerald-200">
                                <span className="min-w-0 break-all font-mono font-bold">{createdCode}</span>
                                <button type="button" className={buttonClass} onClick={() => navigator.clipboard?.writeText(createdCode)}>复制</button>
                            </div>
                        ) : null}
                        {codesError ? <div className="mt-2 rounded-xl border border-red-400/40 bg-red-400/10 p-2 text-xs text-red-700 dark:text-red-200">{codesError}</div> : null}
                        <div className="mt-3 space-y-3">
                            <LimitFields draft={codeDraft} onChange={patchCodeDraft} includeRedemptions />
                            <Field label="备注">
                                <input className={inputClass} value={codeDraft.note} onChange={(event) => patchCodeDraft({ note: event.target.value })} />
                            </Field>
                            <ModulePicker modules={grantableModules} value={codeDraft.enabledModules} onChange={(enabledModules) => patchCodeDraft({ enabledModules })} />
                            <div className="flex gap-2">
                                <button type="button" className={primaryButtonClass} disabled={codesLoading} onClick={createCode}>{codesLoading ? '处理中...' : '生成授权码'}</button>
                                <button type="button" className={buttonClass} disabled={codesLoading} onClick={loadCodes}>刷新</button>
                            </div>
                        </div>
                    </div>

                    <div className="space-y-2">
                        {codes.length > 0 ? codes.map((code) => {
                            const editing = Number(editingCodeId) === Number(code.id);
                            return (
                                <div key={code.id} className="rounded-2xl border border-zinc-200 bg-white/70 p-3 dark:border-white/10 dark:bg-white/5">
                                    <div className="flex items-start justify-between gap-2">
                                        <div className="min-w-0">
                                            <div className="break-all text-sm font-semibold text-zinc-900 dark:text-white/90">{code.code}</div>
                                            <div className="mt-1 text-xs text-zinc-500 dark:text-white/45">{statusLabel(code.status)} / {code.redeemed_count}/{code.max_redemptions} / 到期 {formatDateTime(code.active_to)}</div>
                                            <div className="mt-1 text-[10px] text-zinc-400 dark:text-white/35">{moduleSummary(code.enabled_modules, moduleCatalog)}</div>
                                        </div>
                                        <span className={`shrink-0 rounded-full border px-2 py-1 text-[10px] font-semibold ${statusClass(code.status)}`}>{statusLabel(code.status)}</span>
                                    </div>
                                    {editing ? (
                                        <div className="mt-3 space-y-3">
                                            <LimitFields
                                                draft={{
                                                    activeTo: formatDateInput(code.active_to),
                                                    maxRedemptions: String(code.max_redemptions || 1),
                                                    maxWallets: String(code.max_wallets || 0),
                                                    maxActiveTasks: String(code.max_active_tasks || 0),
                                                    miniAppEnabled: Boolean(code.mini_app_enabled),
                                                }}
                                                onChange={(patch) => {
                                                    const next = {};
                                                    if (Object.prototype.hasOwnProperty.call(patch, 'activeTo')) next.active_to = patch.activeTo || null;
                                                    if (Object.prototype.hasOwnProperty.call(patch, 'maxRedemptions')) next.max_redemptions = patch.maxRedemptions;
                                                    if (Object.prototype.hasOwnProperty.call(patch, 'maxWallets')) next.max_wallets = patch.maxWallets;
                                                    if (Object.prototype.hasOwnProperty.call(patch, 'maxActiveTasks')) next.max_active_tasks = patch.maxActiveTasks;
                                                    if (Object.prototype.hasOwnProperty.call(patch, 'miniAppEnabled')) next.mini_app_enabled = patch.miniAppEnabled;
                                                    setCodes((prev) => prev.map((item) => item.id === code.id ? { ...item, ...next } : item));
                                                }}
                                                includeRedemptions
                                            />
                                            <Field label="备注">
                                                <input className={inputClass} value={code.note || ''} onChange={(event) => updateCodeRow(code.id, { note: event.target.value })} />
                                            </Field>
                                            <ModulePicker modules={grantableModules} value={code.enabled_modules} onChange={(enabledModules) => updateCodeRow(code.id, { enabled_modules: enabledModules })} compact />
                                        </div>
                                    ) : null}
                                    <div className="mt-3 flex flex-wrap gap-2">
                                        {editing ? <button type="button" className={primaryButtonClass} onClick={() => saveCode(code)}>保存</button> : <button type="button" className={buttonClass} onClick={() => setEditingCodeId(code.id)}>编辑</button>}
                                        {code.status === 'disabled' ? (
                                            <button type="button" className={buttonClass} onClick={async () => { await enableAdminAuthCode({ apiBaseUrl, initData, codeId: code.id }); showNotice('授权码已启用'); await loadCodes(); }}>启用</button>
                                        ) : (
                                            <button type="button" className={dangerButtonClass} onClick={async () => { if (!window.confirm('确认停用该授权码？')) return; await disableAdminAuthCode({ apiBaseUrl, initData, codeId: code.id }); showNotice('授权码已停用'); await loadCodes(); }}>停用</button>
                                        )}
                                    </div>
                                </div>
                            );
                        }) : <div className="p-3 text-center text-xs text-zinc-500">{codesLoading ? '正在加载授权码...' : '暂无授权码'}</div>}
                    </div>
                </div>
            ) : null}

            {section === 'announcements' ? (
                <div className="space-y-3">
                    <div className="rounded-2xl border border-zinc-200 bg-white/70 p-3 dark:border-white/10 dark:bg-white/5">
                        <div className="text-sm font-semibold text-zinc-900 dark:text-white/90">发布公告</div>
                        {annError ? <div className="mt-2 rounded-xl border border-red-400/40 bg-red-400/10 p-2 text-xs text-red-700 dark:text-red-200">{annError}</div> : null}
                        <div className="mt-3 space-y-2">
                            <Field label="标题"><input className={inputClass} value={announcementDraft.title} onChange={(event) => setAnnouncementDraft((prev) => ({ ...prev, title: event.target.value }))} /></Field>
                            <Field label="正文"><textarea className={`${inputClass} min-h-[150px] py-3`} value={announcementDraft.content} onChange={(event) => setAnnouncementDraft((prev) => ({ ...prev, content: event.target.value }))} /></Field>
                        </div>
                        <div className="mt-3 flex gap-2">
                            <button type="button" className={primaryButtonClass} disabled={publishing || !announcementDraft.content.trim()} onClick={publishAnnouncement}>{publishing ? '发布中...' : '发布公告'}</button>
                            <button type="button" className={buttonClass} disabled={annLoading} onClick={loadAnnouncements}>刷新记录</button>
                        </div>
                    </div>
                    <div className="space-y-2">
                        {announcements.length > 0 ? announcements.map((item) => (
                            <div key={item.id} className="rounded-2xl border border-zinc-200 bg-white/70 p-3 dark:border-white/10 dark:bg-white/5">
                                <div className="text-sm font-semibold text-zinc-900 dark:text-white/90">{item.title || '系统公告'}</div>
                                <div className="mt-1 text-xs text-zinc-500 dark:text-white/45">{formatDateTime(item.created_at)} / 成功 {Number(item.sent_count || 0)} / 失败 {Number(item.failed_count || 0)}</div>
                                <div className="mt-2 line-clamp-3 text-xs text-zinc-600 dark:text-white/60">{String(item.content || '')}</div>
                            </div>
                        )) : <div className="p-3 text-center text-xs text-zinc-500">{annLoading ? '正在加载公告...' : '暂无公告'}</div>}
                    </div>
                </div>
            ) : null}
        </div>
    );
}
