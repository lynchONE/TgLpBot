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
} from '../api';
import ConfirmDialog from './ConfirmDialog';
import { EmptyState, MetricCard } from './PanelShell';

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

export default function AdminAccessWorkbench({ apiBaseUrl, initData, hasInitData, onNotice }) {
  const [section, setSection] = useState('users');
  const [notice, setNotice] = useState('');
  const [confirmAction, setConfirmAction] = useState(null);
  const [confirmBusy, setConfirmBusy] = useState(false);

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
    if (typeof onNotice === 'function') onNotice(text);
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

  const runConfirmAction = useCallback(async () => {
    const action = confirmAction?.action;
    if (confirmBusy || typeof action !== 'function') return;
    setConfirmBusy(true);
    try {
      await action();
    } finally {
      setConfirmBusy(false);
      setConfirmAction(null);
    }
  }, [confirmAction, confirmBusy]);

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
    <div className="am-stack">
      {notice ? <div className="panel-success">{notice}</div> : null}
      <div className="am-actions">
        {SECTIONS.map((item) => (
          <button key={item.key} type="button" className={`am-tab-btn ${section === item.key ? 'active' : ''}`} onClick={() => setSection(item.key)}>
            {item.label}
          </button>
        ))}
      </div>

      {section === 'users' ? (
        <>
          <div className="am-metric-row">
            <MetricCard label="用户数" value={String(usersTotal)} tone="strong" />
            <MetricCard label="当前用户" value={selectedUser?.user_id ? `#${selectedUser.user_id}` : '--'} />
            <MetricCard label="授权状态" value={accessStatusLabel(selectedUser?.status)} />
            <MetricCard label="MiniApp" value={selectedUser?.mini_app_enabled ? '开' : '关'} />
          </div>
          <div className="am-two-col">
            <div className="am-card">
              <div className="am-card-header">
                <div className="am-card-title">用户授权</div>
                <button type="button" className="am-action-btn" disabled={usersLoading} onClick={loadUsers}>刷新</button>
              </div>
              <div className="am-form">
                <label className="am-field am-field-grow">
                  <span>搜索</span>
                  <input value={usersQuery} onChange={(event) => setUsersQuery(event.target.value)} placeholder="@username / Telegram ID" />
                </label>
                <button type="button" className="am-action-btn" disabled={usersLoading} onClick={loadUsers}>查询</button>
              </div>
              {usersError ? <div className="am-error">{usersError}</div> : null}
              <div className="am-list">
                {users.length > 0 ? users.map((user) => (
                  <button
                    type="button"
                    key={user.user_id}
                    className={`am-list-item am-list-btn ${Number(user.user_id) === Number(selectedUser?.user_id) ? 'selected' : ''}`}
                    onClick={() => {
                      setSelectedUser(user);
                      setAccessDraft(makeAccessDraft(user));
                    }}
                  >
                    <div style={{ minWidth: 0 }}>
                      <div className="am-item-title">{formatUserLabel(user)}</div>
                      <div className="am-item-sub">TG {user.telegram_id || '--'} / {accessStatusLabel(user.status)}</div>
                    </div>
                    <span className={user.status === 'active' ? 'am-badge am-badge-ok' : 'am-badge am-badge-warn'}>{user.mini_app_enabled ? 'Mini' : 'No Mini'}</span>
                  </button>
                )) : <EmptyState text={usersLoading ? '正在加载用户...' : '暂无用户'} />}
              </div>
            </div>
            <div className="am-card">
              <div className="am-card-header">
                <div className="am-card-title">编辑授权</div>
                <span className="am-item-sub">{selectedUser ? formatUserLabel(selectedUser) : '未选择'}</span>
              </div>
              {selectedUser ? (
                <>
                  <div className="am-form">
                    <label className="am-field">
                      <span>到期时间</span>
                      <input type="date" value={accessDraft.activeTo} disabled={accessDraft.clearActiveTo} onChange={(event) => setAccessDraft((prev) => ({ ...prev, activeTo: event.target.value }))} />
                    </label>
                    <label className="am-field am-field-check">
                      <input type="checkbox" checked={accessDraft.clearActiveTo} onChange={(event) => setAccessDraft((prev) => ({ ...prev, clearActiveTo: event.target.checked }))} />
                      <span>永久有效</span>
                    </label>
                    <label className="am-field">
                      <span>钱包数</span>
                      <input type="number" min="0" value={accessDraft.maxWallets} onChange={(event) => setAccessDraft((prev) => ({ ...prev, maxWallets: event.target.value }))} />
                    </label>
                    <label className="am-field">
                      <span>任务数</span>
                      <input type="number" min="0" value={accessDraft.maxActiveTasks} onChange={(event) => setAccessDraft((prev) => ({ ...prev, maxActiveTasks: event.target.value }))} />
                    </label>
                    <label className="am-field am-field-check">
                      <input type="checkbox" checked={accessDraft.miniAppEnabled} onChange={(event) => setAccessDraft((prev) => ({ ...prev, miniAppEnabled: event.target.checked }))} />
                      <span>MiniApp 权限</span>
                    </label>
                    <label className="am-field am-field-grow">
                      <span>备注</span>
                      <input value={accessDraft.note} onChange={(event) => setAccessDraft((prev) => ({ ...prev, note: event.target.value }))} />
                    </label>
                  </div>
                  <div className="am-actions">
                    <span className="am-item-sub">
                      钱包 {Number(selectedUser.wallet_count || 0)} / {Number(accessDraft.maxWallets || 0)}，任务 {Number(selectedUser.active_task_count || 0)} / {Number(accessDraft.maxActiveTasks || 0)}
                    </span>
                    <button type="button" className="am-action-btn" disabled={accessSaving} onClick={saveSelectedAccess}>{accessSaving ? '保存中...' : '保存授权'}</button>
                    {selectedUser.status === 'revoked' ? (
                      <button type="button" className="am-action-btn" onClick={async () => { await restoreAdminUserAccess({ apiBaseUrl, initData, userId: selectedUser.user_id }); showNotice('授权已恢复'); await loadUsers(); }}>恢复授权</button>
                    ) : (
                      <button type="button" className="am-action-btn" onClick={() => setConfirmAction({
                        title: '停用用户授权',
                        message: `确认停用 ${formatUserLabel(selectedUser)} 的授权？`,
                        confirmText: '停用',
                        danger: true,
                        action: async () => {
                          await revokeAdminUserAccess({ apiBaseUrl, initData, userId: selectedUser.user_id });
                          showNotice('授权已停用');
                          await loadUsers();
                        },
                      })}>停用授权</button>
                    )}
                  </div>
                </>
              ) : <EmptyState text="请选择一个用户" />}
            </div>
          </div>
        </>
      ) : null}

      {section === 'codes' ? (
        <div className="am-two-col">
          <div className="am-card">
            <div className="am-card-header">
              <div className="am-card-title">生成授权码</div>
              <span className="am-item-sub">共 {codesTotal} 个</span>
            </div>
            {createdCode ? <div className="panel-success">新授权码：{createdCode}</div> : null}
            {codesError ? <div className="am-error">{codesError}</div> : null}
            <div className="am-form">
              <label className="am-field">
                <span>有效期</span>
                <input type="date" value={codeDraft.activeTo} onChange={(event) => setCodeDraft((prev) => ({ ...prev, activeTo: event.target.value }))} />
              </label>
              <label className="am-field">
                <span>兑换次数</span>
                <input type="number" min="1" value={codeDraft.maxRedemptions} onChange={(event) => setCodeDraft((prev) => ({ ...prev, maxRedemptions: event.target.value }))} />
              </label>
              <label className="am-field">
                <span>钱包数</span>
                <input type="number" min="0" value={codeDraft.maxWallets} onChange={(event) => setCodeDraft((prev) => ({ ...prev, maxWallets: event.target.value }))} />
              </label>
              <label className="am-field">
                <span>任务数</span>
                <input type="number" min="0" value={codeDraft.maxActiveTasks} onChange={(event) => setCodeDraft((prev) => ({ ...prev, maxActiveTasks: event.target.value }))} />
              </label>
              <label className="am-field am-field-check">
                <input type="checkbox" checked={codeDraft.miniAppEnabled} onChange={(event) => setCodeDraft((prev) => ({ ...prev, miniAppEnabled: event.target.checked }))} />
                <span>MiniApp 权限</span>
              </label>
              <label className="am-field am-field-grow">
                <span>备注</span>
                <input value={codeDraft.note} onChange={(event) => setCodeDraft((prev) => ({ ...prev, note: event.target.value }))} />
              </label>
            </div>
            <div className="am-actions">
              <button type="button" className="am-action-btn" disabled={codesLoading} onClick={createCode}>{codesLoading ? '处理中...' : '生成授权码'}</button>
              <button type="button" className="am-action-btn" disabled={codesLoading} onClick={loadCodes}>刷新</button>
            </div>
          </div>
          <div className="am-card">
            <div className="am-card-header">
              <div className="am-card-title">授权码列表</div>
            </div>
            <div className="am-list">
              {codes.length > 0 ? codes.map((code) => {
                const editing = Number(editingCodeId) === Number(code.id);
                return (
                  <div key={code.id} className="am-list-item am-list-item-wrap">
                    <div style={{ minWidth: 0, flex: 1 }}>
                      <div className="am-item-title">{code.code}</div>
                      <div className="am-item-sub">{codeStatusLabel(code.status)} / {code.redeemed_count}/{code.max_redemptions} / 到期 {formatDateTime(code.active_to)}</div>
                      {editing ? (
                        <div className="am-form">
                          <label className="am-field">
                            <span>兑换</span>
                            <input type="number" min="1" value={code.max_redemptions} onChange={(event) => setCodes((prev) => prev.map((item) => item.id === code.id ? { ...item, max_redemptions: event.target.value } : item))} />
                          </label>
                          <label className="am-field">
                            <span>钱包</span>
                            <input type="number" min="0" value={code.max_wallets} onChange={(event) => setCodes((prev) => prev.map((item) => item.id === code.id ? { ...item, max_wallets: event.target.value } : item))} />
                          </label>
                          <label className="am-field">
                            <span>任务</span>
                            <input type="number" min="0" value={code.max_active_tasks} onChange={(event) => setCodes((prev) => prev.map((item) => item.id === code.id ? { ...item, max_active_tasks: event.target.value } : item))} />
                          </label>
                          <label className="am-field">
                            <span>到期</span>
                            <input type="date" value={formatDateInput(code.active_to)} onChange={(event) => setCodes((prev) => prev.map((item) => item.id === code.id ? { ...item, active_to: event.target.value || null } : item))} />
                          </label>
                          <label className="am-field am-field-check">
                            <input type="checkbox" checked={Boolean(code.mini_app_enabled)} onChange={(event) => setCodes((prev) => prev.map((item) => item.id === code.id ? { ...item, mini_app_enabled: event.target.checked } : item))} />
                            <span>MiniApp</span>
                          </label>
                          <label className="am-field am-field-grow">
                            <span>备注</span>
                            <input value={code.note || ''} onChange={(event) => setCodes((prev) => prev.map((item) => item.id === code.id ? { ...item, note: event.target.value } : item))} />
                          </label>
                        </div>
                      ) : null}
                    </div>
                    <div className="am-btn-group">
                      {editing ? (
                        <button type="button" className="am-action-btn" onClick={() => saveCode(code)}>保存</button>
                      ) : (
                        <button type="button" className="am-action-btn" onClick={() => setEditingCodeId(code.id)}>编辑</button>
                      )}
                      {code.status === 'disabled' ? (
                        <button type="button" className="am-action-btn" onClick={async () => { await enableAdminAuthCode({ apiBaseUrl, initData, codeId: code.id }); showNotice('授权码已启用'); await loadCodes(); }}>启用</button>
                      ) : (
                        <button type="button" className="am-action-btn" onClick={() => setConfirmAction({
                          title: '停用授权码',
                          message: `确认停用授权码 ${code.code}？`,
                          confirmText: '停用',
                          danger: true,
                          action: async () => {
                            await disableAdminAuthCode({ apiBaseUrl, initData, codeId: code.id });
                            showNotice('授权码已停用');
                            await loadCodes();
                          },
                        })}>停用</button>
                      )}
                    </div>
                  </div>
                );
              }) : <EmptyState text={codesLoading ? '正在加载授权码...' : '暂无授权码'} />}
            </div>
          </div>
        </div>
      ) : null}

      {section === 'announcements' ? (
        <div className="am-two-col">
          <div className="am-card">
            <div className="am-card-header">
              <div className="am-card-title">发布公告</div>
            </div>
            {annError ? <div className="am-error">{annError}</div> : null}
            <div className="am-form">
              <label className="am-field am-field-grow">
                <span>标题</span>
                <input value={announcementDraft.title} onChange={(event) => setAnnouncementDraft((prev) => ({ ...prev, title: event.target.value }))} />
              </label>
              <label className="am-field am-field-grow" style={{ flexBasis: '100%' }}>
                <span>正文</span>
                <textarea value={announcementDraft.content} onChange={(event) => setAnnouncementDraft((prev) => ({ ...prev, content: event.target.value }))} rows={7} />
              </label>
            </div>
            <div className="am-actions">
              <button
                type="button"
                className="am-action-btn"
                disabled={publishing || !announcementDraft.content.trim()}
                onClick={() => setConfirmAction({
                  title: '发布公告',
                  message: '确认向 Telegram 用户广播这条公告？',
                  confirmText: '发布',
                  action: publishAnnouncement,
                })}
              >
                {publishing ? '发布中...' : '发布公告'}
              </button>
              <button type="button" className="am-action-btn" disabled={annLoading} onClick={loadAnnouncements}>刷新记录</button>
            </div>
          </div>
          <div className="am-card">
            <div className="am-card-header">
              <div className="am-card-title">公告记录</div>
            </div>
            <div className="am-list">
              {announcements.length > 0 ? announcements.map((item) => (
                <div key={item.id} className="am-list-item am-list-item-wrap">
                  <div style={{ minWidth: 0 }}>
                    <div className="am-item-title">{item.title || '系统公告'}</div>
                    <div className="am-item-sub">{formatDateTime(item.created_at)} / 成功 {Number(item.sent_count || 0)} / 失败 {Number(item.failed_count || 0)}</div>
                    <div className="am-item-sub">{String(item.content || '').slice(0, 120)}</div>
                  </div>
                </div>
              )) : <EmptyState text={annLoading ? '正在加载公告...' : '暂无公告'} />}
            </div>
          </div>
        </div>
      ) : null}

      <ConfirmDialog
        open={Boolean(confirmAction)}
        title={confirmAction?.title}
        message={confirmAction?.message}
        confirmText={confirmAction?.confirmText}
        danger={Boolean(confirmAction?.danger)}
        loading={confirmBusy}
        onConfirm={runConfirmAction}
        onCancel={() => {
          if (!confirmBusy) setConfirmAction(null);
        }}
      />
    </div>
  );
}
