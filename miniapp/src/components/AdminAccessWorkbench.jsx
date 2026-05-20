import React, { useCallback, useEffect, useMemo, useState } from 'react';
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
    { key: 'users', label: '用户授权' },
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

function accessStatusLabel(status) {
    switch (String(status || '').toLowerCase()) {
        case 'active':
            return '已授权';
        case 'revoked':
            return '已停用';
        case 'expired':
            return '已过期';
        case 'pending':
            return '未生效';
        default:
            return '未授权';
    }
}

function codeStatusLabel(status) {
    switch (String(status || '').toLowerCase()) {
        case 'active':
            return '可用';
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

function makeAccessDraft(user) {
    return {
        activeTo: formatDateInput(user?.active_to),
        clearActiveTo: !user?.active_to,
        maxWallets: String(Number(user?.max_wallets || 1)),
        maxActiveTasks: String(Number(user?.max_active_tasks || 1)),
        miniAppEnabled: Boolean(user?.mini_app_enabled),
        note: String(user?.note || ''),
    };
}

const defaultCodeDraft = {
    activeTo: '',
    maxRedemptions: '1',
    maxWallets: '1',
    maxActiveTasks: '1',
    miniAppEnabled: true,
    note: '',
};

function Field({ label, children, wide = false }) {
    return (
        <label className={`flex min-w-0 flex-col gap-1 ${wide ? 'col-span-2' : ''}`}>
            <span className="text-[10px] font-medium text-zinc-500 dark:text-white/45">{label}</span>
            {children}
        </label>
    );
}

const inputClass = 'min-h-[38px] rounded-xl border border-zinc-200 bg-white/80 px-3 text-xs text-zinc-900 outline-none focus:border-lime-400 dark:border-white/10 dark:bg-white/5 dark:text-white/90';
const buttonClass = 'rounded-xl border border-zinc-200 bg-white/80 px-3 py-2 text-xs font-semibold text-zinc-700 shadow-sm transition active:scale-[0.98] dark:border-white/10 dark:bg-white/10 dark:text-white/80';
const primaryButtonClass = 'rounded-xl bg-zinc-950 px-3 py-2 text-xs font-semibold text-white shadow-sm transition active:scale-[0.98] dark:bg-lime-300 dark:text-zinc-950';

export default function AdminAccessWorkbench({ apiBaseUrl, initData, hasInitData, onNotice }) {
    const [section, setSection] = useState('users');
    const [notice, setNotice] = useState('');

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
    const [codeDraft, setCodeDraft] = useState(defaultCodeDraft);
    const [createdCode, setCreatedCode] = useState('');
    const [editingCodeId, setEditingCodeId] = useState(null);

    const [announcements, setAnnouncements] = useState([]);
    const [annLoading, setAnnLoading] = useState(false);
    const [annError, setAnnError] = useState('');
    const [announcementDraft, setAnnouncementDraft] = useState({ title: '系统公告', content: '' });
    const [publishing, setPublishing] = useState(false);

    const showNotice = useCallback((message) => {
        const text = String(message || '').trim();
        if (!text) return;
        setNotice(text);
        onNotice?.(text);
        window.setTimeout(() => setNotice(''), 2600);
    }, [onNotice]);

    const loadUsers = useCallback(async () => {
        if (!hasInitData) return;
        setUsersLoading(true);
        setUsersError('');
        try {
            const data = await fetchAdminAccessList({ apiBaseUrl, initData, pageSize: 20, query: usersQuery });
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
            const data = await fetchAdminAuthCodes({ apiBaseUrl, initData, pageSize: 20 });
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

    const selectedAccess = useMemo(() => selectedUser || users[0] || null, [selectedUser, users]);

    useEffect(() => {
        if (selectedAccess && (!selectedUser || Number(selectedUser.user_id) !== Number(selectedAccess.user_id))) {
            setSelectedUser(selectedAccess);
            setAccessDraft(makeAccessDraft(selectedAccess));
        }
    }, [selectedAccess, selectedUser]);

    const saveSelectedAccess = useCallback(async () => {
        if (!selectedUser?.user_id) return;
        setAccessSaving(true);
        try {
            const payload = {
                max_wallets: Number(accessDraft.maxWallets),
                max_active_tasks: Number(accessDraft.maxActiveTasks),
                mini_app_enabled: Boolean(accessDraft.miniAppEnabled),
                note: accessDraft.note,
            };
            if (accessDraft.clearActiveTo) {
                payload.clear_active_to = true;
            } else if (String(accessDraft.activeTo || '').trim()) {
                payload.active_to = String(accessDraft.activeTo).trim();
            }
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
                max_redemptions: Number(codeDraft.maxRedemptions),
                max_wallets: Number(codeDraft.maxWallets),
                max_active_tasks: Number(codeDraft.maxActiveTasks),
                mini_app_enabled: Boolean(codeDraft.miniAppEnabled),
                note: codeDraft.note,
            };
            if (String(codeDraft.activeTo || '').trim()) payload.active_to = String(codeDraft.activeTo).trim();
            const data = await createAdminAuthCode({ apiBaseUrl, initData, payload });
            setCreatedCode(String(data?.code?.code || ''));
            setCodeDraft(defaultCodeDraft);
            showNotice('授权码已生成');
            await loadCodes();
        } catch (err) {
            setCodesError(errorText(err));
        } finally {
            setCodesLoading(false);
        }
    }, [apiBaseUrl, codeDraft, initData, loadCodes, showNotice]);

    const saveCode = useCallback(async (code) => {
        const payload = {
            max_redemptions: Number(code.max_redemptions || 1),
            max_wallets: Number(code.max_wallets || 1),
            max_active_tasks: Number(code.max_active_tasks || 1),
            mini_app_enabled: Boolean(code.mini_app_enabled),
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
                    <div className="grid grid-cols-2 gap-2">
                        <div className="rounded-2xl border border-zinc-200 bg-white/70 p-3 dark:border-white/10 dark:bg-white/5">
                            <div className="text-[10px] text-zinc-500 dark:text-white/45">用户数</div>
                            <div className="mt-1 text-lg font-semibold text-zinc-900 dark:text-white">{usersTotal}</div>
                        </div>
                        <div className="rounded-2xl border border-zinc-200 bg-white/70 p-3 dark:border-white/10 dark:bg-white/5">
                            <div className="text-[10px] text-zinc-500 dark:text-white/45">当前授权</div>
                            <div className="mt-1 text-lg font-semibold text-zinc-900 dark:text-white">{accessStatusLabel(selectedUser?.status)}</div>
                        </div>
                    </div>
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
                                    <div className="flex items-center justify-between gap-2">
                                        <div className="min-w-0">
                                            <div className="truncate text-sm font-semibold text-zinc-900 dark:text-white/90">{formatUserLabel(user)}</div>
                                            <div className="mt-1 text-xs text-zinc-500 dark:text-white/45">TG {user.telegram_id || '--'} / {accessStatusLabel(user.status)}</div>
                                        </div>
                                        <span className="rounded-full border border-zinc-200 px-2 py-1 text-[10px] text-zinc-500 dark:border-white/10 dark:text-white/50">{user.mini_app_enabled ? 'Mini' : 'No Mini'}</span>
                                    </div>
                                </button>
                            )) : <div className="p-3 text-center text-xs text-zinc-500">{usersLoading ? '正在加载用户...' : '暂无用户'}</div>}
                        </div>
                    </div>
                    <div className="rounded-2xl border border-zinc-200 bg-white/70 p-3 dark:border-white/10 dark:bg-white/5">
                        <div className="text-sm font-semibold text-zinc-900 dark:text-white/90">编辑授权</div>
                        {selectedUser ? (
                            <div className="mt-3 grid grid-cols-2 gap-2">
                                <Field label="到期时间"><input className={inputClass} type="date" value={accessDraft.activeTo} disabled={accessDraft.clearActiveTo} onChange={(event) => setAccessDraft((prev) => ({ ...prev, activeTo: event.target.value }))} /></Field>
                                <Field label="永久"><label className="flex min-h-[38px] items-center gap-2 text-xs text-zinc-600 dark:text-white/65"><input type="checkbox" checked={accessDraft.clearActiveTo} onChange={(event) => setAccessDraft((prev) => ({ ...prev, clearActiveTo: event.target.checked }))} />永久有效</label></Field>
                                <Field label="钱包数"><input className={inputClass} type="number" min="0" value={accessDraft.maxWallets} onChange={(event) => setAccessDraft((prev) => ({ ...prev, maxWallets: event.target.value }))} /></Field>
                                <Field label="任务数"><input className={inputClass} type="number" min="0" value={accessDraft.maxActiveTasks} onChange={(event) => setAccessDraft((prev) => ({ ...prev, maxActiveTasks: event.target.value }))} /></Field>
                                <Field label="MiniApp"><label className="flex min-h-[38px] items-center gap-2 text-xs text-zinc-600 dark:text-white/65"><input type="checkbox" checked={accessDraft.miniAppEnabled} onChange={(event) => setAccessDraft((prev) => ({ ...prev, miniAppEnabled: event.target.checked }))} />允许使用</label></Field>
                                <Field label="备注"><input className={inputClass} value={accessDraft.note} onChange={(event) => setAccessDraft((prev) => ({ ...prev, note: event.target.value }))} /></Field>
                                <div className="col-span-2 flex flex-wrap gap-2">
                                    <button type="button" className={primaryButtonClass} disabled={accessSaving} onClick={saveSelectedAccess}>{accessSaving ? '保存中...' : '保存授权'}</button>
                                    {selectedUser.status === 'revoked' ? (
                                        <button type="button" className={buttonClass} onClick={async () => { await restoreAdminUserAccess({ apiBaseUrl, initData, userId: selectedUser.user_id }); showNotice('授权已恢复'); await loadUsers(); }}>恢复授权</button>
                                    ) : (
                                        <button type="button" className={buttonClass} onClick={async () => { if (!window.confirm('确认停用该用户授权？')) return; await revokeAdminUserAccess({ apiBaseUrl, initData, userId: selectedUser.user_id }); showNotice('授权已停用'); await loadUsers(); }}>停用授权</button>
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
                        {createdCode ? <div className="mt-2 rounded-xl border border-emerald-400/40 bg-emerald-400/10 p-2 text-xs text-emerald-700 dark:text-emerald-200">新授权码：{createdCode}</div> : null}
                        {codesError ? <div className="mt-2 rounded-xl border border-red-400/40 bg-red-400/10 p-2 text-xs text-red-700 dark:text-red-200">{codesError}</div> : null}
                        <div className="mt-3 grid grid-cols-2 gap-2">
                            <Field label="有效期"><input className={inputClass} type="date" value={codeDraft.activeTo} onChange={(event) => setCodeDraft((prev) => ({ ...prev, activeTo: event.target.value }))} /></Field>
                            <Field label="兑换次数"><input className={inputClass} type="number" min="1" value={codeDraft.maxRedemptions} onChange={(event) => setCodeDraft((prev) => ({ ...prev, maxRedemptions: event.target.value }))} /></Field>
                            <Field label="钱包数"><input className={inputClass} type="number" min="0" value={codeDraft.maxWallets} onChange={(event) => setCodeDraft((prev) => ({ ...prev, maxWallets: event.target.value }))} /></Field>
                            <Field label="任务数"><input className={inputClass} type="number" min="0" value={codeDraft.maxActiveTasks} onChange={(event) => setCodeDraft((prev) => ({ ...prev, maxActiveTasks: event.target.value }))} /></Field>
                            <Field label="MiniApp"><label className="flex min-h-[38px] items-center gap-2 text-xs text-zinc-600 dark:text-white/65"><input type="checkbox" checked={codeDraft.miniAppEnabled} onChange={(event) => setCodeDraft((prev) => ({ ...prev, miniAppEnabled: event.target.checked }))} />允许使用</label></Field>
                            <Field label="备注"><input className={inputClass} value={codeDraft.note} onChange={(event) => setCodeDraft((prev) => ({ ...prev, note: event.target.value }))} /></Field>
                        </div>
                        <div className="mt-3 flex gap-2">
                            <button type="button" className={primaryButtonClass} disabled={codesLoading} onClick={createCode}>{codesLoading ? '处理中...' : '生成授权码'}</button>
                            <button type="button" className={buttonClass} disabled={codesLoading} onClick={loadCodes}>刷新</button>
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
                                            <div className="mt-1 text-xs text-zinc-500 dark:text-white/45">{codeStatusLabel(code.status)} / {code.redeemed_count}/{code.max_redemptions} / 到期 {formatDateTime(code.active_to)}</div>
                                        </div>
                                    </div>
                                    {editing ? (
                                        <div className="mt-3 grid grid-cols-2 gap-2">
                                            <Field label="兑换"><input className={inputClass} type="number" min="1" value={code.max_redemptions} onChange={(event) => setCodes((prev) => prev.map((item) => item.id === code.id ? { ...item, max_redemptions: event.target.value } : item))} /></Field>
                                            <Field label="钱包"><input className={inputClass} type="number" min="0" value={code.max_wallets} onChange={(event) => setCodes((prev) => prev.map((item) => item.id === code.id ? { ...item, max_wallets: event.target.value } : item))} /></Field>
                                            <Field label="任务"><input className={inputClass} type="number" min="0" value={code.max_active_tasks} onChange={(event) => setCodes((prev) => prev.map((item) => item.id === code.id ? { ...item, max_active_tasks: event.target.value } : item))} /></Field>
                                            <Field label="到期"><input className={inputClass} type="date" value={formatDateInput(code.active_to)} onChange={(event) => setCodes((prev) => prev.map((item) => item.id === code.id ? { ...item, active_to: event.target.value || null } : item))} /></Field>
                                            <Field label="MiniApp"><label className="flex min-h-[38px] items-center gap-2 text-xs text-zinc-600 dark:text-white/65"><input type="checkbox" checked={Boolean(code.mini_app_enabled)} onChange={(event) => setCodes((prev) => prev.map((item) => item.id === code.id ? { ...item, mini_app_enabled: event.target.checked } : item))} />允许使用</label></Field>
                                            <Field label="备注"><input className={inputClass} value={code.note || ''} onChange={(event) => setCodes((prev) => prev.map((item) => item.id === code.id ? { ...item, note: event.target.value } : item))} /></Field>
                                        </div>
                                    ) : null}
                                    <div className="mt-3 flex flex-wrap gap-2">
                                        {editing ? <button type="button" className={primaryButtonClass} onClick={() => saveCode(code)}>保存</button> : <button type="button" className={buttonClass} onClick={() => setEditingCodeId(code.id)}>编辑</button>}
                                        {code.status === 'disabled' ? (
                                            <button type="button" className={buttonClass} onClick={async () => { await enableAdminAuthCode({ apiBaseUrl, initData, codeId: code.id }); showNotice('授权码已启用'); await loadCodes(); }}>启用</button>
                                        ) : (
                                            <button type="button" className={buttonClass} onClick={async () => { if (!window.confirm('确认停用该授权码？')) return; await disableAdminAuthCode({ apiBaseUrl, initData, codeId: code.id }); showNotice('授权码已停用'); await loadCodes(); }}>停用</button>
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
