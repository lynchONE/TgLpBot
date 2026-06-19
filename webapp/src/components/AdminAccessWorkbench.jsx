import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
  Ban,
  Check,
  ChevronDown,
  Copy,
  KeyRound,
  Megaphone,
  Plus,
  Power,
  PowerOff,
  RefreshCw,
  RotateCcw,
  Save,
  Search,
  UsersRound,
} from 'lucide-react';
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
import { EmptyState } from './PanelShell';
import AdminStatChip from './admin/AdminStatChip';

const SECTIONS = [
  { key: 'users', label: '用户授权', icon: UsersRound },
  { key: 'codes', label: '授权码', icon: KeyRound },
  { key: 'announcements', label: '公告', icon: Megaphone },
];

const USERS_PAGE_SIZE = 24;
const CODES_PAGE_SIZE = 24;

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

function statusText(status) {
  switch (String(status || '').toLowerCase()) {
    case 'active':
      return '生效中';
    case 'revoked':
      return '已停用';
    case 'expired':
      return '已过期';
    case 'pending':
      return '未生效';
    case 'disabled':
      return '已停用';
    case 'exhausted':
      return '已用完';
    default:
      return status || '未授权';
  }
}

function statusTone(status) {
  const value = String(status || '').toLowerCase();
  if (value === 'active') return 'ok';
  if (value === 'revoked' || value === 'disabled' || value === 'expired' || value === 'exhausted') return 'danger';
  return 'warn';
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

function moduleMap(modules) {
  return new Map((modules || []).map((item) => [String(item.key || '').trim(), item]));
}

function moduleSummary(keys, modules) {
  const normalized = normalizeModuleKeys(keys);
  if (!normalized.length) return '未授权模块';
  const byKey = moduleMap(modules);
  const labels = normalized.slice(0, 3).map((key) => byKey.get(key)?.label || key);
  const rest = normalized.length - labels.length;
  return rest > 0 ? `${labels.join('、')} +${rest}` : labels.join('、');
}

function groupModuleSummary(items, selected) {
  const selectedLabels = items
    .filter((item) => selected.has(String(item.key || '').trim()))
    .slice(0, 3)
    .map((item) => item.label || item.key);
  if (!selectedLabels.length) return '本组未选择';
  const selectedCount = items.filter((item) => selected.has(String(item.key || '').trim())).length;
  const rest = selectedCount - selectedLabels.length;
  return rest > 0 ? `${selectedLabels.join('、')} +${rest}` : selectedLabels.join('、');
}

function applyModulePayload(data, setModuleCatalog, setGrantableModules) {
  if (Array.isArray(data?.module_catalog)) {
    setModuleCatalog(data.module_catalog);
  }
  if (Array.isArray(data?.grantable_modules)) {
    setGrantableModules(data.grantable_modules);
  }
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

function SectionTabs({ section, onChange }) {
  return (
    <div className="aaw-tabs" role="tablist" aria-label="管理员模块">
      {SECTIONS.map((item) => {
        const Icon = item.icon;
        return (
          <button
            key={item.key}
            type="button"
            role="tab"
            className={`aaw-tab ${section === item.key ? 'active' : ''}`}
            aria-selected={section === item.key}
            onClick={() => onChange(item.key)}
          >
            <Icon size={16} />
            <span>{item.label}</span>
          </button>
        );
      })}
    </div>
  );
}

function ModulePermissionEditor({ modules, value, onChange, dense = false }) {
  const [openGroups, setOpenGroups] = useState(() => new Set());
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

  useEffect(() => {
    setOpenGroups((prev) => {
      const validGroups = new Set(groups.map((item) => item.group));
      const next = new Set(Array.from(prev).filter((group) => validGroups.has(group)));
      return next;
    });
  }, [groups]);

  const emit = useCallback((keys) => {
    onChange(normalizeModuleKeys(keys).filter((key) => moduleKeys.includes(key)));
  }, [moduleKeys, onChange]);

  const toggleKey = useCallback((key) => {
    const next = new Set(selected);
    if (next.has(key)) next.delete(key);
    else next.add(key);
    emit(Array.from(next));
  }, [emit, selected]);

  const toggleGroup = useCallback((items) => {
    const keys = items.map((item) => String(item.key || '').trim()).filter(Boolean);
    const allSelected = keys.every((key) => selected.has(key));
    const next = new Set(selected);
    keys.forEach((key) => {
      if (allSelected) next.delete(key);
      else next.add(key);
    });
    emit(Array.from(next));
  }, [emit, selected]);

  const toggleOpen = useCallback((group) => {
    setOpenGroups((prev) => {
      const next = new Set(prev);
      if (next.has(group)) next.delete(group);
      else next.add(group);
      return next;
    });
  }, []);

  const expandAll = useCallback(() => {
    setOpenGroups(new Set(groups.map((item) => item.group)));
  }, [groups]);

  const collapseAll = useCallback(() => {
    setOpenGroups(new Set());
  }, []);

  if (!modules.length) {
    return <div className="aaw-empty-line">模块目录为空</div>;
  }

  return (
    <div className={`aaw-module-editor ${dense ? 'dense' : ''}`}>
      <div className="aaw-module-toolbar">
        <div>
          <strong>功能模块</strong>
          <span>{selected.size}/{modules.length}</span>
        </div>
        <div className="aaw-inline-actions">
          <button type="button" className="aaw-mini-btn" onClick={() => emit(moduleKeys)}>全部授权</button>
          <button type="button" className="aaw-mini-btn ghost" onClick={() => emit([])}>清空</button>
          <button type="button" className="aaw-mini-btn ghost" onClick={expandAll}>展开全部</button>
          <button type="button" className="aaw-mini-btn ghost" onClick={collapseAll}>收起</button>
        </div>
      </div>
      <div className="aaw-module-groups">
        {groups.map(({ group, items }) => {
          const groupKeys = items.map((item) => String(item.key || '').trim()).filter(Boolean);
          const groupSelected = groupKeys.filter((key) => selected.has(key)).length;
          const open = openGroups.has(group);
          return (
            <section key={group} className={`aaw-module-group ${open ? 'open' : ''}`}>
              <div className="aaw-module-group-head">
                <button type="button" className="aaw-module-group-toggle" onClick={() => toggleOpen(group)}>
                  <ChevronDown size={14} />
                  <span>{group}</span>
                  <em>{groupSelected}/{groupKeys.length}</em>
                </button>
                <span className="aaw-module-group-summary">{groupModuleSummary(items, selected)}</span>
                <button type="button" className="aaw-module-group-action" onClick={() => toggleGroup(items)}>
                  {groupSelected === groupKeys.length ? '取消本组' : '选择本组'}
                </button>
              </div>
              {open ? (
                <div className="aaw-module-grid">
                  {items.map((item) => {
                    const key = String(item.key || '').trim();
                    const checked = selected.has(key);
                    return (
                      <label key={key} className={`aaw-module-check ${checked ? 'checked' : ''}`}>
                        <input type="checkbox" checked={checked} onChange={() => toggleKey(key)} />
                        <span className="aaw-checkmark">{checked ? <Check size={14} /> : null}</span>
                        <span className="aaw-module-copy">
                          <strong>{item.label || key}</strong>
                          <small>{key}</small>
                        </span>
                      </label>
                    );
                  })}
                </div>
              ) : null}
            </section>
          );
        })}
      </div>
    </div>
  );
}

function LimitFields({ draft, onChange, includeRedemptions = false }) {
  return (
    <div className="aaw-form-grid">
      <label className="aaw-field">
        <span>有效期</span>
        <input
          type="date"
          value={draft.activeTo}
          disabled={Boolean(draft.clearActiveTo)}
          onChange={(event) => onChange({ activeTo: event.target.value })}
        />
      </label>
      {'clearActiveTo' in draft ? (
        <label className="aaw-check-row">
          <input
            type="checkbox"
            checked={Boolean(draft.clearActiveTo)}
            onChange={(event) => onChange({ clearActiveTo: event.target.checked })}
          />
          <span>长期有效</span>
        </label>
      ) : null}
      {includeRedemptions ? (
        <label className="aaw-field">
          <span>兑换次数</span>
          <input
            type="number"
            min="1"
            value={draft.maxRedemptions}
            onChange={(event) => onChange({ maxRedemptions: event.target.value })}
          />
        </label>
      ) : null}
      <label className="aaw-field">
        <span>钱包数</span>
        <input
          type="number"
          min="0"
          value={draft.maxWallets}
          onChange={(event) => onChange({ maxWallets: event.target.value })}
        />
      </label>
      <label className="aaw-field">
        <span>任务数</span>
        <input
          type="number"
          min="0"
          value={draft.maxActiveTasks}
          onChange={(event) => onChange({ maxActiveTasks: event.target.value })}
        />
      </label>
      <label className="aaw-check-row">
        <input
          type="checkbox"
          checked={Boolean(draft.miniAppEnabled)}
          onChange={(event) => onChange({ miniAppEnabled: event.target.checked })}
        />
        <span>MiniApp</span>
      </label>
    </div>
  );
}

export default function AdminAccessWorkbench({ apiBaseUrl, initData, hasInitData, onNotice }) {
  const [section, setSection] = useState('users');
  const [notice, setNotice] = useState('');
  const [confirmAction, setConfirmAction] = useState(null);
  const [confirmBusy, setConfirmBusy] = useState(false);

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

  const initializedCodeModulesRef = useRef(false);
  const moduleKeys = useMemo(
    () => grantableModules.map((item) => String(item.key || '').trim()).filter(Boolean),
    [grantableModules]
  );

  useEffect(() => {
    if (initializedCodeModulesRef.current || moduleKeys.length === 0) return;
    initializedCodeModulesRef.current = true;
    setCodeDraft((prev) => ({ ...prev, enabledModules: moduleKeys }));
  }, [moduleKeys]);

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
      const data = await fetchAdminAccessList({
        apiBaseUrl,
        initData,
        pageSize: USERS_PAGE_SIZE,
        query: usersQuery,
      });
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
      throw err;
    } finally {
      setUsersLoading(false);
    }
  }, [apiBaseUrl, hasInitData, initData, selectedUser?.user_id, usersQuery]);

  const loadCodes = useCallback(async () => {
    if (!hasInitData) return;
    setCodesLoading(true);
    setCodesError('');
    try {
      const data = await fetchAdminAuthCodes({
        apiBaseUrl,
        initData,
        pageSize: CODES_PAGE_SIZE,
      });
      applyModulePayload(data, setModuleCatalog, setGrantableModules);
      const items = Array.isArray(data?.items) ? data.items : [];
      setCodes(items);
      setCodesTotal(Number(data?.total || items.length));
    } catch (err) {
      setCodesError(errorText(err));
      throw err;
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
      throw err;
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
    if (!selectedAccess) return;
    if (selectedUser && Number(selectedUser.user_id) === Number(selectedAccess.user_id)) return;
    setSelectedUser(selectedAccess);
    setAccessDraft(makeAccessDraft(selectedAccess));
  }, [selectedAccess, selectedUser]);

  const patchAccessDraft = useCallback((patch) => {
    setAccessDraft((prev) => ({ ...prev, ...patch }));
  }, []);

  const patchCodeDraft = useCallback((patch) => {
    setCodeDraft((prev) => ({ ...prev, ...patch }));
  }, []);

  const updateCodeRow = useCallback((codeId, patch) => {
    setCodes((prev) => prev.map((item) => (
      Number(item.id) === Number(codeId) ? { ...item, ...patch } : item
    )));
  }, []);

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
      if (accessDraft.clearActiveTo) {
        payload.clear_active_to = true;
      } else if (String(accessDraft.activeTo || '').trim()) {
        payload.active_to = String(accessDraft.activeTo).trim();
      }
      const data = await updateAdminUserAccess({
        apiBaseUrl,
        initData,
        userId: selectedUser.user_id,
        patch: payload,
      });
      setSelectedUser(data);
      setAccessDraft(makeAccessDraft(data));
      showNotice('用户授权已更新');
      await loadUsers();
    } catch (err) {
      setUsersError(errorText(err));
      throw err;
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
      throw err;
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
      throw err;
    } finally {
      setPublishing(false);
    }
  }, [announcementDraft, apiBaseUrl, initData, loadAnnouncements, showNotice]);

  const activeUsers = users.filter((item) => String(item.status || '').toLowerCase() === 'active').length;
  const activeCodes = codes.filter((item) => String(item.status || '').toLowerCase() === 'active').length;

  return (
    <div className="aaw-shell">
      {notice ? <div className="aaw-notice">{notice}</div> : null}
      <div className="am-stat-row" style={{ marginBottom: 12 }}>
        <AdminStatChip
          label="用户总数"
          value={usersTotal || users.length}
          tone="accent"
          hint={`${activeUsers} 生效中`}
        />
        <AdminStatChip
          label="生效用户"
          value={activeUsers}
          tone={activeUsers > 0 ? 'ok' : 'idle'}
        />
        <AdminStatChip
          label="授权码"
          value={codesTotal || codes.length}
          tone={activeCodes > 0 ? 'ok' : 'idle'}
          hint={`${activeCodes} 生效中`}
        />
        <AdminStatChip
          label="可授权模块"
          value={grantableModules.length}
          tone="idle"
        />
      </div>

      <SectionTabs section={section} onChange={setSection} />

      {section === 'users' ? (
        <div className="aaw-split">
          <section className="aaw-panel aaw-list-panel">
            <div className="aaw-panel-head">
              <div>
                <h3>用户</h3>
                <span>{usersTotal || users.length} 条记录</span>
              </div>
              <button type="button" className="aaw-icon-btn" disabled={usersLoading} onClick={loadUsers} title="刷新">
                <RefreshCw size={15} />
              </button>
            </div>
            <div className="aaw-search">
              <Search size={15} />
              <input
                value={usersQuery}
                onChange={(event) => setUsersQuery(event.target.value)}
                onKeyDown={(event) => {
                  if (event.key === 'Enter') loadUsers();
                }}
                placeholder="@username / Telegram ID"
              />
              <button type="button" onClick={loadUsers} disabled={usersLoading}>查询</button>
            </div>
            {usersError ? <div className="aaw-error">{usersError}</div> : null}
            <div className="aaw-rows">
              {users.length > 0 ? users.map((user) => (
                <button
                  type="button"
                  key={user.user_id}
                  className={`aaw-user-row ${Number(user.user_id) === Number(selectedUser?.user_id) ? 'selected' : ''}`}
                  onClick={() => {
                    setSelectedUser(user);
                    setAccessDraft(makeAccessDraft(user));
                  }}
                >
                  <span className="aaw-avatar">{String(formatUserLabel(user)).slice(0, 1).toUpperCase()}</span>
                  <span className="aaw-row-main">
                    <strong>{formatUserLabel(user)}</strong>
                    <small>TG {user.telegram_id || '--'} · {moduleSummary(user.enabled_modules, moduleCatalog)}</small>
                  </span>
                  <span className={`aaw-badge ${statusTone(user.status)}`}>{statusText(user.status)}</span>
                </button>
              )) : <EmptyState text={usersLoading ? '正在加载用户...' : '暂无用户'} />}
            </div>
          </section>

          <section className="aaw-panel aaw-editor-panel">
            <div className="aaw-panel-head">
              <div>
                <h3>编辑授权</h3>
                <span>{selectedUser ? formatUserLabel(selectedUser) : '未选择用户'}</span>
              </div>
              {selectedUser ? <span className={`aaw-badge ${statusTone(selectedUser.status)}`}>{statusText(selectedUser.status)}</span> : null}
            </div>
            {selectedUser ? (
              <>
                <LimitFields draft={accessDraft} onChange={patchAccessDraft} />
                <label className="aaw-field full">
                  <span>备注</span>
                  <input value={accessDraft.note} onChange={(event) => patchAccessDraft({ note: event.target.value })} />
                </label>
                <ModulePermissionEditor
                  modules={grantableModules}
                  value={accessDraft.enabledModules}
                  onChange={(enabledModules) => patchAccessDraft({ enabledModules })}
                />
                <div className="aaw-quota-line">
                  <span>钱包 {Number(selectedUser.wallet_count || 0)} / {positiveInt(accessDraft.maxWallets, 0)}</span>
                  <span>任务 {Number(selectedUser.active_task_count || 0)} / {positiveInt(accessDraft.maxActiveTasks, 0)}</span>
                </div>
                <div className="aaw-actions">
                  <button type="button" className="aaw-primary-btn" disabled={accessSaving} onClick={saveSelectedAccess}>
                    <Save size={15} />
                    {accessSaving ? '保存中...' : '保存授权'}
                  </button>
                  {selectedUser.status === 'revoked' ? (
                    <button
                      type="button"
                      className="aaw-secondary-btn"
                      onClick={async () => {
                        await restoreAdminUserAccess({ apiBaseUrl, initData, userId: selectedUser.user_id });
                        showNotice('授权已恢复');
                        await loadUsers();
                      }}
                    >
                      <RotateCcw size={15} />
                      恢复授权
                    </button>
                  ) : (
                    <button
                      type="button"
                      className="aaw-danger-btn"
                      onClick={() => setConfirmAction({
                        title: '停用用户授权',
                        message: `确认停用 ${formatUserLabel(selectedUser)} 的授权？`,
                        confirmText: '停用',
                        danger: true,
                        action: async () => {
                          await revokeAdminUserAccess({ apiBaseUrl, initData, userId: selectedUser.user_id });
                          showNotice('授权已停用');
                          await loadUsers();
                        },
                      })}
                    >
                      <Ban size={15} />
                      停用授权
                    </button>
                  )}
                </div>
              </>
            ) : <EmptyState text="请选择一个用户" />}
          </section>
        </div>
      ) : null}

      {section === 'codes' ? (
        <div className="aaw-split">
          <section className="aaw-panel">
            <div className="aaw-panel-head">
              <div>
                <h3>生成授权码</h3>
                <span>{activeCodes} 个可用</span>
              </div>
              <button type="button" className="aaw-icon-btn" disabled={codesLoading} onClick={loadCodes} title="刷新">
                <RefreshCw size={15} />
              </button>
            </div>
            {createdCode ? (
              <div className="aaw-created-code">
                <span>{createdCode}</span>
                <button
                  type="button"
                  onClick={async () => {
                    await navigator.clipboard.writeText(createdCode);
                    showNotice('授权码已复制');
                  }}
                >
                  <Copy size={14} />
                  复制
                </button>
              </div>
            ) : null}
            {codesError ? <div className="aaw-error">{codesError}</div> : null}
            <LimitFields draft={codeDraft} onChange={patchCodeDraft} includeRedemptions />
            <label className="aaw-field full">
              <span>备注</span>
              <input value={codeDraft.note} onChange={(event) => patchCodeDraft({ note: event.target.value })} />
            </label>
            <ModulePermissionEditor
              modules={grantableModules}
              value={codeDraft.enabledModules}
              onChange={(enabledModules) => patchCodeDraft({ enabledModules })}
            />
            <div className="aaw-actions">
              <button type="button" className="aaw-primary-btn" disabled={codesLoading} onClick={createCode}>
                <Plus size={15} />
                {codesLoading ? '处理中...' : '生成授权码'}
              </button>
            </div>
          </section>

          <section className="aaw-panel aaw-code-panel">
            <div className="aaw-panel-head">
              <div>
                <h3>授权码列表</h3>
                <span>{codesTotal || codes.length} 条记录</span>
              </div>
            </div>
            <div className="aaw-code-list">
              {codes.length > 0 ? codes.map((code) => {
                const editing = Number(editingCodeId) === Number(code.id);
                return (
                  <article key={code.id} className="aaw-code-card">
                    <div className="aaw-code-top">
                      <div>
                        <strong>{code.code}</strong>
                        <span>{statusText(code.status)} · {code.redeemed_count}/{code.max_redemptions} · 到期 {formatDateTime(code.active_to)}</span>
                      </div>
                      <span className={`aaw-badge ${statusTone(code.status)}`}>{statusText(code.status)}</span>
                    </div>
                    {editing ? (
                      <div className="aaw-code-edit">
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
                            updateCodeRow(code.id, next);
                          }}
                          includeRedemptions
                        />
                        <label className="aaw-field full">
                          <span>备注</span>
                          <input value={code.note || ''} onChange={(event) => updateCodeRow(code.id, { note: event.target.value })} />
                        </label>
                        <ModulePermissionEditor
                          modules={grantableModules}
                          value={code.enabled_modules}
                          onChange={(enabledModules) => updateCodeRow(code.id, { enabled_modules: enabledModules })}
                          dense
                        />
                      </div>
                    ) : (
                      <div className="aaw-code-meta">
                        <span>钱包 {code.max_wallets}</span>
                        <span>任务 {code.max_active_tasks}</span>
                        <span>{code.mini_app_enabled ? 'MiniApp 开' : 'MiniApp 关'}</span>
                        <span>{moduleSummary(code.enabled_modules, moduleCatalog)}</span>
                      </div>
                    )}
                    <div className="aaw-actions compact">
                      {editing ? (
                        <button type="button" className="aaw-primary-btn" onClick={() => saveCode(code)}>
                          <Save size={15} />
                          保存
                        </button>
                      ) : (
                        <button type="button" className="aaw-secondary-btn" onClick={() => setEditingCodeId(code.id)}>
                          编辑
                        </button>
                      )}
                      {code.status === 'disabled' ? (
                        <button
                          type="button"
                          className="aaw-secondary-btn"
                          onClick={async () => {
                            await enableAdminAuthCode({ apiBaseUrl, initData, codeId: code.id });
                            showNotice('授权码已启用');
                            await loadCodes();
                          }}
                        >
                          <Power size={15} />
                          启用
                        </button>
                      ) : (
                        <button
                          type="button"
                          className="aaw-danger-btn"
                          onClick={() => setConfirmAction({
                            title: '停用授权码',
                            message: `确认停用授权码 ${code.code}？`,
                            confirmText: '停用',
                            danger: true,
                            action: async () => {
                              await disableAdminAuthCode({ apiBaseUrl, initData, codeId: code.id });
                              showNotice('授权码已停用');
                              await loadCodes();
                            },
                          })}
                        >
                          <PowerOff size={15} />
                          停用
                        </button>
                      )}
                    </div>
                  </article>
                );
              }) : <EmptyState text={codesLoading ? '正在加载授权码...' : '暂无授权码'} />}
            </div>
          </section>
        </div>
      ) : null}

      {section === 'announcements' ? (
        <div className="aaw-split">
          <section className="aaw-panel">
            <div className="aaw-panel-head">
              <div>
                <h3>发布公告</h3>
                <span>Telegram 广播</span>
              </div>
            </div>
            {annError ? <div className="aaw-error">{annError}</div> : null}
            <label className="aaw-field full">
              <span>标题</span>
              <input value={announcementDraft.title} onChange={(event) => setAnnouncementDraft((prev) => ({ ...prev, title: event.target.value }))} />
            </label>
            <label className="aaw-field full">
              <span>正文</span>
              <textarea rows={8} value={announcementDraft.content} onChange={(event) => setAnnouncementDraft((prev) => ({ ...prev, content: event.target.value }))} />
            </label>
            <div className="aaw-actions">
              <button
                type="button"
                className="aaw-primary-btn"
                disabled={publishing || !announcementDraft.content.trim()}
                onClick={() => setConfirmAction({
                  title: '发布公告',
                  message: '确认向 Telegram 用户广播这条公告？',
                  confirmText: '发布',
                  action: publishAnnouncement,
                })}
              >
                <Megaphone size={15} />
                {publishing ? '发布中...' : '发布公告'}
              </button>
              <button type="button" className="aaw-secondary-btn" disabled={annLoading} onClick={loadAnnouncements}>
                <RefreshCw size={15} />
                刷新记录
              </button>
            </div>
          </section>

          <section className="aaw-panel">
            <div className="aaw-panel-head">
              <div>
                <h3>公告记录</h3>
                <span>{announcements.length} 条记录</span>
              </div>
            </div>
            <div className="aaw-code-list">
              {announcements.length > 0 ? announcements.map((item) => (
                <article key={item.id} className="aaw-code-card">
                  <div className="aaw-code-top">
                    <div>
                      <strong>{item.title || '系统公告'}</strong>
                      <span>{formatDateTime(item.created_at)} · 成功 {Number(item.sent_count || 0)} · 失败 {Number(item.failed_count || 0)}</span>
                    </div>
                  </div>
                  <p className="aaw-ann-content">{String(item.content || '').slice(0, 160)}</p>
                </article>
              )) : <EmptyState text={annLoading ? '正在加载公告...' : '暂无公告'} />}
            </div>
          </section>
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
