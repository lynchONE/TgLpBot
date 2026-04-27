import React, { useCallback, useEffect, useMemo, useState } from 'react';
import AdminOnlineUsers from './AdminOnlineUsers.jsx';
import AdminActiveTasks from './AdminActiveTasks.jsx';
import AdminRPCPool from './AdminRPCPool.jsx';
import AdminPoolDataSources from './AdminPoolDataSources.jsx';
import AdminPrivateZapCard from './AdminPrivateZapCard.jsx';
import SystemConfigCard from './SystemConfigCard.jsx';
import PositionCard from './PositionCard.jsx';
import { getBrandTheme } from '../lib/brand';
import {
    fetchAdminActiveTasks,
    fetchAdminOnlineUsers,
    fetchAdminRealtimePositions,
    fetchAdminUserAccess,
} from '../lib/api';

function formatUserLabel(user) {
    if (!user) return '--';
    const first = String(user.first_name || '').trim();
    const last = String(user.last_name || '').trim();
    const username = String(user.username || '').trim();
    const fullName = [first, last].filter(Boolean).join(' ');
    if (fullName && username) return `${fullName} (@${username})`;
    if (fullName) return fullName;
    if (username) return `@${username}`;
    return `用户 ${user.user_id || '--'}`;
}

const ADMIN_TABS = [
    { key: 'online_users', label: '在线用户' },
    { key: 'active_tasks', label: '活跃任务' },
    { key: 'user_detail', label: '用户详情' },
    { key: 'system_config', label: '系统配置' },
    { key: 'rpc_pool', label: 'RPC' },
    { key: 'pool_data_sources', label: '池子源' },
];

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
    const tabs = useMemo(() => {
        if (!Array.isArray(visibleTabs) || visibleTabs.length === 0) return ADMIN_TABS;
        const allow = new Set(visibleTabs.map((item) => String(item || '').trim()).filter(Boolean));
        const next = ADMIN_TABS.filter((tab) => allow.has(tab.key));
        return next.length > 0 ? next : ADMIN_TABS;
    }, [visibleTabs]);
    const defaultTab = useMemo(() => {
        const requested = String(initialTab || '').trim();
        if (requested && tabs.some((tab) => tab.key === requested)) return requested;
        return tabs[0]?.key || 'online_users';
    }, [initialTab, tabs]);
    const [activeTab, setActiveTab] = useState(defaultTab);

    const [onlineUsers, setOnlineUsers] = useState([]);
    const [onlineUsersLoading, setOnlineUsersLoading] = useState(false);
    const [onlineUsersError, setOnlineUsersError] = useState('');

    const [activeTasks, setActiveTasks] = useState([]);
    const [activeTasksLoading, setActiveTasksLoading] = useState(false);
    const [activeTasksError, setActiveTasksError] = useState('');

    const [selectedUser, setSelectedUser] = useState(null);
    const [userPositions, setUserPositions] = useState(null);
    const [userPositionsLoading, setUserPositionsLoading] = useState(false);
    const [userPositionsError, setUserPositionsError] = useState('');

    const [userAccess, setUserAccess] = useState(null);

    useEffect(() => {
        if (!tabs.some((tab) => tab.key === activeTab)) {
            setActiveTab(defaultTab);
        }
    }, [activeTab, defaultTab, tabs]);

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

    const loadUserPositions = useCallback(async (userId) => {
        if (!hasInitData || !userId) return;
        setUserPositionsLoading(true);
        setUserPositionsError('');
        try {
            const data = await fetchAdminRealtimePositions({ apiBaseUrl, initData, userId });
            setUserPositions(data || null);
        } catch (e) {
            setUserPositionsError(String(e?.message || e));
        } finally {
            setUserPositionsLoading(false);
        }
    }, [apiBaseUrl, initData, hasInitData]);

    const loadUserAccess = useCallback(async (userId) => {
        if (!hasInitData || !userId) return;
        try {
            const data = await fetchAdminUserAccess({ apiBaseUrl, initData, userId });
            setUserAccess(data || null);
        } catch {
            setUserAccess(null);
        }
    }, [apiBaseUrl, initData, hasInitData]);

    useEffect(() => {
        if (activeTab === 'online_users') {
            loadOnlineUsers();
        } else if (activeTab === 'active_tasks') {
            loadActiveTasks();
        }
    }, [activeTab, loadOnlineUsers, loadActiveTasks]);

    useEffect(() => {
        if (!selectedUser?.user_id) return;
        loadUserPositions(selectedUser.user_id);
        loadUserAccess(selectedUser.user_id);
    }, [selectedUser, loadUserPositions, loadUserAccess]);

    useEffect(() => {
        if (!hasInitData) return undefined;
        const timer = setInterval(() => {
            if (activeTab === 'online_users') {
                loadOnlineUsers();
            } else if (activeTab === 'active_tasks') {
                loadActiveTasks();
            } else if (activeTab === 'user_detail' && selectedUser?.user_id) {
                loadUserPositions(selectedUser.user_id);
            }
        }, Math.max(5, pollIntervalSec) * 1000);
        return () => clearInterval(timer);
    }, [activeTab, hasInitData, loadActiveTasks, loadOnlineUsers, loadUserPositions, pollIntervalSec, selectedUser]);

    const handleSelectUser = useCallback((user) => {
        setSelectedUser(user || null);
        setUserPositions(null);
        setUserAccess(null);
        setUserPositionsError('');
        if (tabs.some((tab) => tab.key === 'user_detail')) {
            setActiveTab('user_detail');
        }
    }, [tabs]);

    const handleSelectTaskUser = useCallback((task) => {
        if (!task?.user_id) return;
        handleSelectUser({
            user_id: task.user_id,
            telegram_id: task.telegram_id,
            username: task.username,
            first_name: task.first_name,
            last_name: task.last_name,
        });
    }, [handleSelectUser]);

    const userPositionsList = useMemo(() => {
        if (!Array.isArray(userPositions?.positions)) return [];
        return userPositions.positions;
    }, [userPositions]);

    return (
        <div className="space-y-4">
            <div
                className="grid gap-1 rounded-2xl border border-zinc-200 bg-white/70 p-1 shadow-sm dark:border-white/10 dark:bg-white/5 dark:shadow-none"
                style={{ gridTemplateColumns: `repeat(${Math.max(tabs.length, 1)}, minmax(0, 1fr))` }}
            >
                {tabs.map((tab) => (
                    <button
                        key={tab.key}
                        type="button"
                        onClick={() => setActiveTab(tab.key)}
                        className={`rounded-xl px-2 py-2 text-[11px] font-semibold transition ${
                            activeTab === tab.key
                                ? `${brand.navActiveClass} shadow-sm`
                                : 'text-zinc-600 hover:bg-zinc-100 dark:text-white/65 dark:hover:bg-white/10'
                        }`}
                    >
                        {tab.label}
                    </button>
                ))}
            </div>

            {activeTab === 'online_users' && (
                <AdminOnlineUsers
                    users={onlineUsers}
                    loading={onlineUsersLoading}
                    error={onlineUsersError}
                    tick={tick}
                    accentTheme={accentTheme}
                    onSelectUser={handleSelectUser}
                    selectedUserId={selectedUser?.user_id}
                />
            )}

            {activeTab === 'active_tasks' && (
                <AdminActiveTasks
                    tasks={activeTasks}
                    loading={activeTasksLoading}
                    error={activeTasksError}
                    tick={tick}
                    onSelectTask={handleSelectTaskUser}
                />
            )}

            {activeTab === 'user_detail' && (
                <div className="space-y-3">
                    <div className="rounded-2xl border border-zinc-200 bg-white/70 p-4 shadow-sm dark:border-white/10 dark:bg-white/5 dark:shadow-none">
                        <div className="flex items-center justify-between gap-3">
                            <div>
                                <div className="text-sm font-semibold text-zinc-900 dark:text-white/90">
                                    {selectedUser ? formatUserLabel(selectedUser) : '请选择用户'}
                                </div>
                                <div className="mt-1 text-xs text-zinc-500 dark:text-white/45">
                                    {selectedUser
                                        ? `TG ${selectedUser.telegram_id || '--'} · 用户 ID ${selectedUser.user_id || '--'}`
                                        : '从在线用户或活跃任务中选中一个用户后查看详情。'}
                                </div>
                            </div>

                        </div>
                    </div>

                    {userPositionsError && (
                        <div className="rounded-xl border border-red-500/30 bg-red-500/10 p-3 text-xs text-red-700 dark:text-red-200">
                            {userPositionsError}
                        </div>
                    )}

                    {userPositionsLoading && userPositionsList.length === 0 && (
                        <div className="rounded-xl border border-zinc-200 bg-white/40 p-3 text-xs text-zinc-500 dark:border-white/10 dark:bg-white/5 dark:text-white/60">
                            加载用户仓位中...
                        </div>
                    )}

                    {!userPositionsLoading && selectedUser && userPositionsList.length === 0 && !userPositionsError && (
                        <div className="rounded-xl border border-zinc-200 bg-white/40 p-3 text-xs text-zinc-500 dark:border-white/10 dark:bg-white/5 dark:text-white/60">
                            当前用户没有活跃仓位。
                        </div>
                    )}

                    {userPositionsList.map((position) => (
                        <PositionCard
                            key={[
                                String(position?.chain || ''),
                                String(position?.version || ''),
                                String(position?.pool_id || ''),
                                String(position?.position_id || ''),
                                String(position?.task_id || ''),
                            ].join(':')}
                            position={position}
                            walletAddress={userPositions?.wallet?.address || ''}
                            bnbBalance={userPositions?.wallet?.bnb_balance || ''}
                            pollIntervalSec={pollIntervalSec}
                            updatedAt={userPositions?.updated_at}
                            allowTaskActions={false}
                        />
                    ))}
                </div>
            )}

            {activeTab === 'system_config' && (
                <div className="space-y-3">
                    <SystemConfigCard apiBaseUrl={apiBaseUrl} initData={initData} accentTheme={accentTheme} onNotice={onNotice} />
                    <AdminPrivateZapCard apiBaseUrl={apiBaseUrl} initData={initData} hasInitData={hasInitData} onNotice={onNotice} />
                </div>
            )}

            {activeTab === 'rpc_pool' && (
                <AdminRPCPool apiBaseUrl={apiBaseUrl} initData={initData} hasInitData={hasInitData} accentTheme={accentTheme} onNotice={onNotice} />
            )}

            {activeTab === 'pool_data_sources' && (
                <AdminPoolDataSources apiBaseUrl={apiBaseUrl} initData={initData} hasInitData={hasInitData} accentTheme={accentTheme} onNotice={onNotice} />
            )}
        </div>
    );
}
