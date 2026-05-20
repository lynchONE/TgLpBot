import React, { useCallback, useEffect, useMemo, useState } from 'react';
import AdminOnlineUsers from './AdminOnlineUsers.jsx';
import AdminActiveTasks from './AdminActiveTasks.jsx';
import AdminRPCPool from './AdminRPCPool.jsx';
import AdminPoolDataSources from './AdminPoolDataSources.jsx';
import AdminPrivateZapCard from './AdminPrivateZapCard.jsx';
import AdminAccessWorkbench from './AdminAccessWorkbench.jsx';
import SystemConfigCard from './SystemConfigCard.jsx';
import StatChip from './admin/StatChip.jsx';
import AdminUserDetailDrawer from './admin/AdminUserDetailDrawer.jsx';
import { getBrandTheme } from '../lib/brand';
import {
    fetchAdminActiveTasks,
    fetchAdminOnlineUsers,
    fetchAdminPoolDataSources,
    fetchAdminRPCPool,
} from '../lib/api';

const TOP_TABS = [
    { key: 'operations', label: '运行' },
    { key: 'access', label: '授权' },
    { key: 'system', label: '系统' },
];

const SYSTEM_SUBTABS = [
    { key: 'config', label: '基础配置' },
    { key: 'rpc', label: 'RPC 节点' },
    { key: 'pool_sources', label: '池子源' },
    { key: 'private_zap', label: 'Private Zap' },
];

const LEGACY_TAB_MAP = {
    online_users: 'operations',
    active_tasks: 'operations',
    user_detail: 'operations',
    access_workbench: 'access',
    system_config: 'system',
    rpc_pool: 'system',
    pool_data_sources: 'system',
};

const TASK_STATUS_FILTERS = [
    { key: 'all', label: '全部' },
    { key: 'running', label: '运行中' },
    { key: 'opening', label: '开仓中' },
    { key: 'waiting', label: '等待中' },
    { key: 'stopping', label: '退出中' },
    { key: 'paused', label: '已暂停' },
];

function normalizeQuery(value) {
    return String(value || '').trim().toLowerCase();
}

function deriveRpcHealth(rpcData) {
    const groups = Array.isArray(rpcData?.groups) ? rpcData.groups : [];
    let total = 0;
    let available = 0;
    let latencySum = 0;
    let latencyCount = 0;
    for (const group of groups) {
        const eps = Array.isArray(group?.endpoints) ? group.endpoints : [];
        for (const ep of eps) {
            total += 1;
            const status = String(ep?.status || '').toLowerCase();
            if (status !== 'unavailable') {
                available += 1;
                const lat = Number(ep?.last_latency_ms || 0);
                if (Number.isFinite(lat) && lat > 0) {
                    latencySum += lat;
                    latencyCount += 1;
                }
            }
        }
    }
    if (total === 0) return { tone: 'idle', value: '--', hint: '无节点' };
    const ratio = available / total;
    const tone = ratio >= 0.8 ? 'ok' : ratio >= 0.4 ? 'warn' : 'danger';
    const avgLatency = latencyCount > 0 ? Math.round(latencySum / latencyCount) : 0;
    return {
        tone,
        value: `${available}/${total}`,
        hint: avgLatency > 0 ? `均 ${avgLatency}ms` : '可用 / 总数',
    };
}

function derivePoolSourceHealth(poolData) {
    const groups = Array.isArray(poolData?.groups) ? poolData.groups : [];
    let total = 0;
    let enabled = 0;
    let withError = 0;
    for (const group of groups) {
        const sources = Array.isArray(group?.sources) ? group.sources : [];
        for (const src of sources) {
            total += 1;
            if (src?.is_enabled) enabled += 1;
            if (src?.last_error) withError += 1;
        }
    }
    if (total === 0) return { tone: 'idle', value: '--', hint: '无来源' };
    const tone = withError === 0 ? (enabled === total ? 'ok' : 'warn') : 'danger';
    return {
        tone,
        value: `${enabled}/${total}`,
        hint: withError > 0 ? `${withError} 个有错误` : '启用 / 总数',
    };
}

function SegmentedTabs({ tabs, active, onChange, accent }) {
    return (
        <div
            className="grid gap-1 rounded-2xl border border-zinc-200/70 bg-white/70 p-1 shadow-sm dark:border-white/10 dark:bg-[#0f1116]/85 dark:shadow-none"
            style={{ gridTemplateColumns: `repeat(${Math.max(tabs.length, 1)}, minmax(0, 1fr))` }}
        >
            {tabs.map((tab) => (
                <button
                    key={tab.key}
                    type="button"
                    onClick={() => onChange(tab.key)}
                    className={`rounded-xl px-2 py-2 text-[12px] font-bold transition ${
                        active === tab.key
                            ? `${accent} shadow-sm`
                            : 'text-zinc-600 hover:bg-zinc-100 dark:text-white/65 dark:hover:bg-white/10'
                    }`}
                >
                    {tab.label}
                </button>
            ))}
        </div>
    );
}

export default function AdminPage({
    apiBaseUrl,
    initData,
    hasInitData,
    tick,
    pollIntervalSec = 15,
    accentTheme = 'lime',
    visibleTabs,
    initialTab,
    onNotice,
}) {
    const brand = useMemo(() => getBrandTheme(accentTheme), [accentTheme]);

    const allowedTopTabs = useMemo(() => {
        if (!Array.isArray(visibleTabs) || visibleTabs.length === 0) return TOP_TABS;
        const allow = new Set();
        for (const key of visibleTabs) {
            const k = String(key || '').trim();
            if (!k) continue;
            if (TOP_TABS.some((t) => t.key === k)) allow.add(k);
            else if (LEGACY_TAB_MAP[k]) allow.add(LEGACY_TAB_MAP[k]);
        }
        const filtered = TOP_TABS.filter((t) => allow.has(t.key));
        return filtered.length > 0 ? filtered : TOP_TABS;
    }, [visibleTabs]);

    const defaultTopTab = useMemo(() => {
        const requested = String(initialTab || '').trim();
        if (requested) {
            if (allowedTopTabs.some((t) => t.key === requested)) return requested;
            const mapped = LEGACY_TAB_MAP[requested];
            if (mapped && allowedTopTabs.some((t) => t.key === mapped)) return mapped;
        }
        return allowedTopTabs[0]?.key || 'operations';
    }, [initialTab, allowedTopTabs]);

    const [topTab, setTopTab] = useState(defaultTopTab);
    const [systemSubtab, setSystemSubtab] = useState(() => {
        const requested = String(initialTab || '').trim();
        if (requested === 'rpc_pool') return 'rpc';
        if (requested === 'pool_data_sources') return 'pool_sources';
        if (requested === 'system_config') return 'config';
        return 'config';
    });

    useEffect(() => {
        if (!allowedTopTabs.some((t) => t.key === topTab)) setTopTab(defaultTopTab);
    }, [allowedTopTabs, defaultTopTab, topTab]);

    /* Operations data */
    const [onlineUsers, setOnlineUsers] = useState([]);
    const [onlineUsersLoading, setOnlineUsersLoading] = useState(false);
    const [onlineUsersError, setOnlineUsersError] = useState('');

    const [activeTasks, setActiveTasks] = useState([]);
    const [activeTasksLoading, setActiveTasksLoading] = useState(false);
    const [activeTasksError, setActiveTasksError] = useState('');

    /* Overview health data */
    const [rpcHealth, setRpcHealth] = useState({ tone: 'idle', value: '--', hint: '--' });
    const [poolHealth, setPoolHealth] = useState({ tone: 'idle', value: '--', hint: '--' });

    /* Operations UX state */
    const [query, setQuery] = useState('');
    const [taskStatusFilter, setTaskStatusFilter] = useState('all');

    /* User detail drawer */
    const [drawerUser, setDrawerUser] = useState(null);
    const drawerOpen = Boolean(drawerUser);

    const loadOnlineUsers = useCallback(async () => {
        if (!hasInitData) return;
        setOnlineUsersLoading(true);
        setOnlineUsersError('');
        try {
            const data = await fetchAdminOnlineUsers({ apiBaseUrl, initData });
            setOnlineUsers(Array.isArray(data?.users) ? data.users : []);
        } catch (e) {
            setOnlineUsersError(String(e?.message || e));
        } finally {
            setOnlineUsersLoading(false);
        }
    }, [apiBaseUrl, initData, hasInitData]);

    const loadActiveTasks = useCallback(async () => {
        if (!hasInitData) return;
        setActiveTasksLoading(true);
        setActiveTasksError('');
        try {
            const data = await fetchAdminActiveTasks({ apiBaseUrl, initData });
            setActiveTasks(Array.isArray(data?.tasks) ? data.tasks : []);
        } catch (e) {
            setActiveTasksError(String(e?.message || e));
        } finally {
            setActiveTasksLoading(false);
        }
    }, [apiBaseUrl, initData, hasInitData]);

    const loadOverviewHealth = useCallback(async () => {
        if (!hasInitData) return;
        const [rpcRes, poolRes] = await Promise.allSettled([
            fetchAdminRPCPool({ apiBaseUrl, initData }),
            fetchAdminPoolDataSources({ apiBaseUrl, initData }),
        ]);
        if (rpcRes.status === 'fulfilled') setRpcHealth(deriveRpcHealth(rpcRes.value));
        if (poolRes.status === 'fulfilled') setPoolHealth(derivePoolSourceHealth(poolRes.value));
    }, [apiBaseUrl, initData, hasInitData]);

    useEffect(() => {
        if (topTab === 'operations') {
            loadOnlineUsers();
            loadActiveTasks();
            loadOverviewHealth();
        }
    }, [topTab, loadOnlineUsers, loadActiveTasks, loadOverviewHealth]);

    useEffect(() => {
        if (!hasInitData) return undefined;
        const cadence = Math.max(5, pollIntervalSec) * 1000;
        const timer = setInterval(() => {
            if (topTab === 'operations') {
                loadOnlineUsers();
                loadActiveTasks();
            }
        }, cadence);
        return () => clearInterval(timer);
    }, [topTab, hasInitData, pollIntervalSec, loadOnlineUsers, loadActiveTasks]);

    useEffect(() => {
        if (!hasInitData) return undefined;
        const timer = setInterval(loadOverviewHealth, 60_000);
        return () => clearInterval(timer);
    }, [hasInitData, loadOverviewHealth]);

    /* Derived overview */
    const onlineTone = onlineUsers.length > 0 ? 'ok' : 'idle';
    const activeTaskTone = activeTasks.length > 0 ? 'accent' : 'idle';

    const filteredOnlineUsers = useMemo(() => {
        const q = normalizeQuery(query);
        if (!q) return onlineUsers;
        return onlineUsers.filter((user) => {
            const haystack = [
                user?.username,
                user?.first_name,
                user?.last_name,
                user?.telegram_id,
                user?.user_id,
            ].map((v) => String(v || '').toLowerCase()).join(' ');
            return haystack.includes(q);
        });
    }, [onlineUsers, query]);

    const filteredActiveTasks = useMemo(() => {
        const q = normalizeQuery(query);
        return activeTasks.filter((task) => {
            if (taskStatusFilter !== 'all') {
                if (taskStatusFilter === 'paused') {
                    if (!task?.paused) return false;
                } else if (String(task?.status || '').toLowerCase() !== taskStatusFilter) {
                    return false;
                }
            }
            if (!q) return true;
            const haystack = [
                task?.username,
                task?.first_name,
                task?.last_name,
                task?.telegram_id,
                task?.user_id,
                task?.task_id,
                task?.token0_symbol,
                task?.token1_symbol,
            ].map((v) => String(v || '').toLowerCase()).join(' ');
            return haystack.includes(q);
        });
    }, [activeTasks, query, taskStatusFilter]);

    const openUserDetail = useCallback((user) => {
        if (!user) return;
        setDrawerUser({
            user_id: user.user_id,
            telegram_id: user.telegram_id,
            username: user.username,
            first_name: user.first_name,
            last_name: user.last_name,
        });
    }, []);

    const openTaskUser = useCallback((task) => {
        if (!task?.user_id) return;
        openUserDetail({
            user_id: task.user_id,
            telegram_id: task.telegram_id,
            username: task.username,
            first_name: task.first_name,
            last_name: task.last_name,
        });
    }, [openUserDetail]);

    return (
        <div className="space-y-3">
            {/* Top tabs */}
            <SegmentedTabs
                tabs={allowedTopTabs}
                active={topTab}
                onChange={setTopTab}
                accent={brand.navActiveClass}
            />

            {/* Overview stat row */}
            <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
                <StatChip
                    label="在线用户"
                    value={onlineUsers.length}
                    tone={onlineTone}
                    pulse={onlineUsers.length > 0}
                    hint={onlineUsersLoading ? '同步中…' : `${filteredOnlineUsers.length} 命中`}
                />
                <StatChip
                    label="活跃任务"
                    value={activeTasks.length}
                    tone={activeTaskTone}
                    hint={activeTasksLoading ? '同步中…' : `${filteredActiveTasks.length} 命中`}
                />
                <StatChip
                    label="RPC 节点"
                    value={rpcHealth.value}
                    tone={rpcHealth.tone}
                    hint={rpcHealth.hint}
                    onClick={() => { setTopTab('system'); setSystemSubtab('rpc'); }}
                />
                <StatChip
                    label="池子源"
                    value={poolHealth.value}
                    tone={poolHealth.tone}
                    hint={poolHealth.hint}
                    onClick={() => { setTopTab('system'); setSystemSubtab('pool_sources'); }}
                />
            </div>

            {/* Operations panel */}
            {topTab === 'operations' && (
                <div className="space-y-3">
                    <div className="rounded-2xl border border-zinc-200/70 bg-white/70 p-3 shadow-sm dark:border-white/10 dark:bg-[#0f1116]/80 dark:shadow-none">
                        <div className="flex items-center gap-2">
                            <div className="relative flex-1">
                                <svg className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-zinc-400 dark:text-white/40" viewBox="0 0 24 24" fill="none">
                                    <circle cx="11" cy="11" r="7" stroke="currentColor" strokeWidth="2" />
                                    <path d="M21 21l-4.3-4.3" stroke="currentColor" strokeWidth="2" strokeLinecap="round" />
                                </svg>
                                <input
                                    value={query}
                                    onChange={(e) => setQuery(e.target.value)}
                                    placeholder="搜索 @username / TG ID / 任务 / 交易对"
                                    className="w-full rounded-xl border border-zinc-200 bg-white pl-8 pr-3 py-2 text-xs outline-none focus:border-zinc-300 dark:border-white/10 dark:bg-white/5 dark:text-white dark:focus:border-white/20"
                                />
                            </div>
                            {query && (
                                <button
                                    type="button"
                                    onClick={() => setQuery('')}
                                    className="rounded-lg px-2 py-1.5 text-[11px] font-medium text-zinc-500 hover:bg-zinc-100 dark:text-white/55 dark:hover:bg-white/10"
                                >
                                    清空
                                </button>
                            )}
                        </div>
                        <div className="mt-2 flex flex-wrap gap-1">
                            {TASK_STATUS_FILTERS.map((item) => (
                                <button
                                    key={item.key}
                                    type="button"
                                    onClick={() => setTaskStatusFilter(item.key)}
                                    className={`rounded-full px-2.5 py-1 text-[10px] font-semibold transition ${
                                        taskStatusFilter === item.key
                                            ? brand.softButtonClass
                                            : 'bg-zinc-100 text-zinc-600 hover:bg-zinc-200 dark:bg-white/5 dark:text-white/60 dark:hover:bg-white/10'
                                    }`}
                                >
                                    {item.label}
                                </button>
                            ))}
                        </div>
                    </div>

                    <div className="space-y-3">
                        <section className="space-y-2">
                            <div className="flex items-end justify-between">
                                <div className="text-[10px] font-semibold uppercase tracking-[0.18em] text-zinc-500 dark:text-white/40">
                                    在线用户 · {filteredOnlineUsers.length}
                                </div>
                                <button
                                    type="button"
                                    onClick={loadOnlineUsers}
                                    disabled={onlineUsersLoading}
                                    className="text-[10px] font-medium text-zinc-500 hover:text-zinc-800 disabled:opacity-40 dark:text-white/40 dark:hover:text-white/80"
                                >
                                    {onlineUsersLoading ? '刷新中…' : '刷新'}
                                </button>
                            </div>
                            <AdminOnlineUsers
                                users={filteredOnlineUsers}
                                loading={onlineUsersLoading}
                                error={onlineUsersError}
                                tick={tick}
                                accentTheme={accentTheme}
                                onSelectUser={openUserDetail}
                                selectedUserId={drawerUser?.user_id}
                                totalCount={onlineUsers.length}
                            />
                        </section>

                        <section className="space-y-2">
                            <div className="flex items-end justify-between">
                                <div className="text-[10px] font-semibold uppercase tracking-[0.18em] text-zinc-500 dark:text-white/40">
                                    活跃任务 · {filteredActiveTasks.length}
                                </div>
                                <button
                                    type="button"
                                    onClick={loadActiveTasks}
                                    disabled={activeTasksLoading}
                                    className="text-[10px] font-medium text-zinc-500 hover:text-zinc-800 disabled:opacity-40 dark:text-white/40 dark:hover:text-white/80"
                                >
                                    {activeTasksLoading ? '刷新中…' : '刷新'}
                                </button>
                            </div>
                            <AdminActiveTasks
                                tasks={filteredActiveTasks}
                                loading={activeTasksLoading}
                                error={activeTasksError}
                                tick={tick}
                                onSelectTask={openTaskUser}
                                totalCount={activeTasks.length}
                            />
                        </section>
                    </div>
                </div>
            )}

            {/* Access panel */}
            {topTab === 'access' && (
                <AdminAccessWorkbench
                    apiBaseUrl={apiBaseUrl}
                    initData={initData}
                    hasInitData={hasInitData}
                    onNotice={onNotice}
                />
            )}

            {/* System panel */}
            {topTab === 'system' && (
                <div className="space-y-3">
                    <SegmentedTabs
                        tabs={SYSTEM_SUBTABS}
                        active={systemSubtab}
                        onChange={setSystemSubtab}
                        accent={brand.navActiveClass}
                    />

                    {systemSubtab === 'config' && (
                        <SystemConfigCard
                            apiBaseUrl={apiBaseUrl}
                            initData={initData}
                            accentTheme={accentTheme}
                            onNotice={onNotice}
                        />
                    )}
                    {systemSubtab === 'rpc' && (
                        <AdminRPCPool
                            apiBaseUrl={apiBaseUrl}
                            initData={initData}
                            hasInitData={hasInitData}
                            accentTheme={accentTheme}
                            onNotice={onNotice}
                        />
                    )}
                    {systemSubtab === 'pool_sources' && (
                        <AdminPoolDataSources
                            apiBaseUrl={apiBaseUrl}
                            initData={initData}
                            hasInitData={hasInitData}
                            accentTheme={accentTheme}
                            onNotice={onNotice}
                        />
                    )}
                    {systemSubtab === 'private_zap' && (
                        <AdminPrivateZapCard
                            apiBaseUrl={apiBaseUrl}
                            initData={initData}
                            hasInitData={hasInitData}
                            onNotice={onNotice}
                        />
                    )}
                </div>
            )}

            <AdminUserDetailDrawer
                open={drawerOpen}
                user={drawerUser}
                apiBaseUrl={apiBaseUrl}
                initData={initData}
                hasInitData={hasInitData}
                pollIntervalSec={pollIntervalSec}
                onClose={() => setDrawerUser(null)}
            />
        </div>
    );
}
