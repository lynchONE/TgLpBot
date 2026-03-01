import React, { useState, useEffect, useCallback, useMemo } from 'react';
import AdminOnlineUsers from './AdminOnlineUsers.jsx';
import AdminActiveTasks from './AdminActiveTasks.jsx';
import SystemConfigCard from './SystemConfigCard.jsx';
import PositionCard from './PositionCard.jsx';
import { SkeletonList, SkeletonPositionCard } from './Skeleton.jsx';
import {
    fetchAdminOnlineUsers,
    fetchAdminActiveTasks,
    fetchAdminRealtimePositions,
    fetchAdminAutoLPStats,
    disableAdminAutoLP,
    fetchAdminUserAccess,
    setAdminSmartMoneyEnabled,
} from '../lib/api';
import { formatRelativeTime } from '../lib/time';
import NumberFlowValue from './NumberFlowValue.jsx';

/**
 * 格式化用户标签
 */
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

/**
 * 管理员子页面 Tab
 */
const ADMIN_TABS = [
    { key: 'online_users', label: '在线用户' },
    { key: 'active_tasks', label: '活跃任务' },
    { key: 'user_detail', label: '用户详情' },
    { key: 'system_config', label: '系统配置' },
];

/**
 * AdminPage - 管理员页面容器组件
 */
export default function AdminPage({
    apiBaseUrl,
    initData,
    hasInitData,
    tick,
    pollIntervalSec = 15,
    onNotice,
}) {
    // 子页面状态
    const [activeTab, setActiveTab] = useState('online_users');

    // 在线用户状态
    const [onlineUsers, setOnlineUsers] = useState([]);
    const [onlineUsersLoading, setOnlineUsersLoading] = useState(false);
    const [onlineUsersError, setOnlineUsersError] = useState('');

    // 活跃任务状态
    const [activeTasks, setActiveTasks] = useState([]);
    const [activeTasksLoading, setActiveTasksLoading] = useState(false);
    const [activeTasksError, setActiveTasksError] = useState('');

    // 用户详情状态
    const [selectedUser, setSelectedUser] = useState(null);
    const [userPositions, setUserPositions] = useState(null);
    const [userPositionsLoading, setUserPositionsLoading] = useState(false);
    const [userPositionsError, setUserPositionsError] = useState('');
    const [userAutoStats, setUserAutoStats] = useState(null);
    const [userAutoStatsLoading, setUserAutoStatsLoading] = useState(false);
    const [userAutoStatsError, setUserAutoStatsError] = useState('');
    const [disableAutoLoading, setDisableAutoLoading] = useState(false);
    const [disableAutoError, setDisableAutoError] = useState('');
    const [disableAutoResult, setDisableAutoResult] = useState(null);

    // SmartMoney 权限管理
    const [userAccess, setUserAccess] = useState(null);
    const [smartMoneyToggling, setSmartMoneyToggling] = useState(false);
    const [smartMoneyError, setSmartMoneyError] = useState('');

    // 加载在线用户
    const loadOnlineUsers = useCallback(async () => {
        if (!hasInitData) return;
        setOnlineUsersLoading(true);
        setOnlineUsersError('');
        try {
            const data = await fetchAdminOnlineUsers({ apiBaseUrl, initData });
            setOnlineUsers(data?.users || []);
        } catch (e) {
            setOnlineUsersError(String(e?.message || e));
        } finally {
            setOnlineUsersLoading(false);
        }
    }, [apiBaseUrl, initData, hasInitData]);

    // 加载活跃任务
    const loadActiveTasks = useCallback(async () => {
        if (!hasInitData) return;
        setActiveTasksLoading(true);
        setActiveTasksError('');
        try {
            const data = await fetchAdminActiveTasks({ apiBaseUrl, initData });
            setActiveTasks(data?.tasks || []);
        } catch (e) {
            setActiveTasksError(String(e?.message || e));
        } finally {
            setActiveTasksLoading(false);
        }
    }, [apiBaseUrl, initData, hasInitData]);

    // 加载用户仓位
    const loadUserPositions = useCallback(async (userId) => {
        if (!hasInitData || !userId) return;
        setUserPositionsLoading(true);
        setUserPositionsError('');
        try {
            const data = await fetchAdminRealtimePositions({ apiBaseUrl, initData, userId });
            setUserPositions(data);
        } catch (e) {
            setUserPositionsError(String(e?.message || e));
        } finally {
            setUserPositionsLoading(false);
        }
    }, [apiBaseUrl, initData, hasInitData]);

    // 加载用户 Auto 统计
    const loadUserAutoStats = useCallback(async (userId) => {
        if (!hasInitData || !userId) return;
        setUserAutoStatsLoading(true);
        setUserAutoStatsError('');
        try {
            const data = await fetchAdminAutoLPStats({ apiBaseUrl, initData, userId });
            setUserAutoStats(data);
        } catch (e) {
            setUserAutoStatsError(String(e?.message || e));
        } finally {
            setUserAutoStatsLoading(false);
        }
    }, [apiBaseUrl, initData, hasInitData]);

    // 关闭用户 Auto
    const handleDisableAuto = useCallback(async () => {
        setDisableAutoLoading(true);
        setDisableAutoError('');
        setDisableAutoResult(null);
        try {
            const result = await disableAdminAutoLP({
                apiBaseUrl,
                initData,
                userId: selectedUser.user_id,
            });
            setDisableAutoResult(result);
            onNotice?.('已发起关闭 Auto', 'success');
        } catch (e) {
            setDisableAutoError(String(e?.message || e));
        } finally {
            setDisableAutoLoading(false);
        }
    }, [apiBaseUrl, initData, hasInitData, selectedUser, onNotice]);

    // 加载用户访问权限
    const loadUserAccess = useCallback(async (userId) => {
        if (!hasInitData || !userId) return;
        try {
            const data = await fetchAdminUserAccess({ apiBaseUrl, initData, userId });
            setUserAccess(data);
        } catch {
            setUserAccess(null);
        }
    }, [apiBaseUrl, initData, hasInitData]);

    // 切换 SmartMoney 权限
    const handleToggleSmartMoney = useCallback(async () => {
        if (!hasInitData || !selectedUser?.user_id) return;
        const newEnabled = !(userAccess?.smart_money_enabled ?? false);
        setSmartMoneyToggling(true);
        setSmartMoneyError('');
        try {
            const updated = await setAdminSmartMoneyEnabled({
                apiBaseUrl,
                initData,
                userId: selectedUser.user_id,
                enabled: newEnabled,
            });
            setUserAccess(updated);
            onNotice?.(`聪明钱权限已${newEnabled ? '开启' : '关闭'}`);
        } catch (e) {
            setSmartMoneyError(String(e?.message || e));
        } finally {
            setSmartMoneyToggling(false);
        }
    }, [apiBaseUrl, initData, hasInitData, selectedUser, userAccess, onNotice]);

    // 初始加载
    useEffect(() => {
        if (activeTab === 'online_users') {
            loadOnlineUsers();
        } else if (activeTab === 'active_tasks') {
            loadActiveTasks();
        }
    }, [activeTab, loadOnlineUsers, loadActiveTasks]);

    // 选择用户后加载详情
    useEffect(() => {
        if (selectedUser?.user_id) {
            loadUserPositions(selectedUser.user_id);
            loadUserAutoStats(selectedUser.user_id);
            loadUserAccess(selectedUser.user_id);
        }
    }, [selectedUser, loadUserPositions, loadUserAutoStats, loadUserAccess]);

    // 轮询刷新
    useEffect(() => {
        if (!hasInitData) return;
        const interval = setInterval(() => {
            if (activeTab === 'online_users') {
                loadOnlineUsers();
            } else if (activeTab === 'active_tasks') {
                loadActiveTasks();
            } else if (activeTab === 'user_detail' && selectedUser?.user_id) {
                loadUserPositions(selectedUser.user_id);
            }
        }, pollIntervalSec * 1000);
        return () => clearInterval(interval);
    }, [hasInitData, activeTab, selectedUser, pollIntervalSec, loadOnlineUsers, loadActiveTasks, loadUserPositions]);

    // 用户点击处理
    const handleSelectUser = useCallback((user) => {
        setSelectedUser(user);
        setUserPositions(null);
        setUserAutoStats(null);
        setUserAccess(null);
        setUserPositionsError('');
        setUserAutoStatsError('');
        setDisableAutoError('');
        setDisableAutoResult(null);
        setSmartMoneyError('');
        setActiveTab('user_detail');
    }, []);

    // 从活跃任务点击用户
    const handleTaskSelectUser = useCallback((task) => {
        if (!task?.user_id) return;
        const user = {
            user_id: task.user_id,
            telegram_id: task.telegram_id,
            username: task.username,
            first_name: task.first_name,
            last_name: task.last_name,
        };
        handleSelectUser(user);
    }, [handleSelectUser]);

    // 用户仓位列表
    const userPositionsList = useMemo(() => {
        if (!userPositions?.positions) return [];
        return userPositions.positions;
    }, [userPositions]);

    // 汇总数据
    const userSummary = useMemo(() => {
        if (!userPositions) return null;
        return {
            totalUsd: userPositions.summary?.total_usd || 0,
            bnbBalance: userPositions.wallet?.bnb_balance || '0',
            bnbUsd: userPositions.wallet?.bnb_usd,
            walletAddress: userPositions.wallet?.address,
        };
    }, [userPositions]);

    return (
        <div className="space-y-4">
            {/* 子页面 Tab 切换 */}
            <div className="flex gap-1 rounded-2xl border border-zinc-200 bg-zinc-100/70 p-1 text-xs font-semibold dark:border-white/10 dark:bg-white/5">
                {ADMIN_TABS.map((tab) => (
                    <button
                        key={tab.key}
                        type="button"
                        onClick={() => setActiveTab(tab.key)}
                        aria-pressed={activeTab === tab.key}
                        className={`flex-1 rounded-xl px-2 py-2 transition ${activeTab === tab.key
                            ? 'bg-white text-zinc-900 shadow-sm dark:bg-white/15 dark:text-white'
                            : 'text-zinc-600 hover:bg-white/60 dark:text-white/50 dark:hover:bg-white/10'
                            }`}
                    >
                        {tab.label}
                    </button>
                ))}
            </div>

            {/* 在线用户页面 */}
            {activeTab === 'online_users' && (
                <div className="rounded-2xl border border-zinc-200 bg-white/40 backdrop-blur-md p-4 shadow-sm dark:border-white/10 dark:bg-white/5 dark:shadow-none">
                    <div className="flex items-center justify-between mb-3">
                        <div className="text-sm font-semibold text-zinc-900 dark:text-white/90">在线用户</div>
                        <div className="text-[11px] text-zinc-500 dark:text-white/40">
                            <NumberFlowValue value={onlineUsers.length} formatOptions={{ maximumFractionDigits: 0 }} /> 人
                        </div>
                    </div>
                    <AdminOnlineUsers
                        users={onlineUsers}
                        loading={onlineUsersLoading}
                        error={onlineUsersError}
                        tick={tick}
                        onSelectUser={handleSelectUser}
                        selectedUserId={selectedUser?.user_id}
                    />
                </div>
            )}

            {/* 活跃任务页面 */}
            {activeTab === 'active_tasks' && (
                <div className="rounded-2xl border border-zinc-200 bg-white/40 backdrop-blur-md p-4 shadow-sm dark:border-white/10 dark:bg-white/5 dark:shadow-none">
                    <div className="flex items-center justify-between mb-3">
                        <div className="text-sm font-semibold text-zinc-900 dark:text-white/90">活跃任务</div>
                        <div className="text-[11px] text-zinc-500 dark:text-white/40">
                            <NumberFlowValue value={activeTasks.length} formatOptions={{ maximumFractionDigits: 0 }} /> 个
                        </div>
                    </div>
                    <AdminActiveTasks
                        tasks={activeTasks}
                        loading={activeTasksLoading}
                        error={activeTasksError}
                        tick={tick}
                        onSelectTask={handleTaskSelectUser}
                    />
                </div>
            )}

            {/* 系统配置页面 */}
            {activeTab === 'system_config' && (
                <SystemConfigCard apiBaseUrl={apiBaseUrl} initData={initData} onNotice={onNotice} />
            )}

            {/* 用户详情页面 */}
            {activeTab === 'user_detail' && (
                <>
                    {!selectedUser ? (
                        <div className="rounded-2xl border border-zinc-200 bg-white/40 backdrop-blur-md p-6 text-sm text-zinc-500 dark:border-white/10 dark:bg-white/5 dark:text-white/60">
                            请从「在线用户」或「活跃任务」中选择用户查看详情。
                        </div>
                    ) : (
                        <>
                            {/* 用户摘要卡片 */}
                            <div className="rounded-2xl border border-zinc-200 bg-white/40 backdrop-blur-md p-4 shadow-sm dark:border-white/10 dark:bg-white/5 dark:shadow-none">
                                <div className="flex items-start justify-between gap-3">
                                    <div className="min-w-0">
                                        <div className="flex flex-wrap items-center gap-2">
                                            <div className="text-sm font-semibold text-zinc-900 dark:text-white/90">
                                                {formatUserLabel(selectedUser)}
                                            </div>
                                            {userAutoStats?.config?.enabled && (
                                                <span className="rounded-lg bg-emerald-500/10 px-2 py-0.5 text-[11px] font-semibold text-emerald-700 ring-1 ring-emerald-500/25 dark:text-emerald-300">
                                                    Auto 开启
                                                </span>
                                            )}
                                        </div>
                                        <div className="mt-0.5 text-[11px] text-zinc-500 dark:text-white/40">
                                            {selectedUser.telegram_id ? `TG ${selectedUser.telegram_id}` : 'TG --'} · ID {selectedUser.user_id}
                                        </div>
                                        {userSummary?.walletAddress && (
                                            <div className="mt-0.5 text-[11px] text-zinc-400 dark:text-white/30 truncate">
                                                钱包: {userSummary.walletAddress}
                                            </div>
                                        )}
                                    </div>
                                    <button
                                        type="button"
                                        onClick={() => {
                                            setSelectedUser(null);
                                            setActiveTab('online_users');
                                        }}
                                        className="shrink-0 rounded-xl px-3 py-2 text-xs font-semibold ring-1 bg-zinc-100 text-zinc-700 ring-zinc-200 hover:bg-zinc-200 dark:bg-white/5 dark:text-white/80 dark:ring-white/10 dark:hover:bg-white/10"
                                    >
                                        返回
                                    </button>
                                </div>

                                {/* 余额信息 */}
                                {userSummary && (
                                    <div className="mt-3 pt-3 border-t border-zinc-100 dark:border-white/5">
                                        <div className="flex items-start justify-between gap-4">
                                            <div>
                                                <div className="text-[11px] text-zinc-500 dark:text-white/40">总余额</div>
                                                <div className="mt-0.5 text-xl font-extrabold tabular-nums text-zinc-900 dark:text-emerald-300">
                                                    <NumberFlowValue value={userSummary.totalUsd} formatter={(v) => `$${Number(v || 0).toFixed(2)}`} />
                                                </div>
                                                <div className="mt-0.5 text-[11px] text-zinc-500 dark:text-white/40 tabular-nums">
                                                    <NumberFlowValue value={userSummary.bnbBalance} formatter={() => String(userSummary.bnbBalance || '0')} /> BNB
                                                    {typeof userSummary.bnbUsd === 'number' ? (
                                                        <span>
                                                            {' '}≈ <NumberFlowValue value={userSummary.bnbUsd} formatter={(v) => `$${Number(v || 0).toFixed(2)}`} />
                                                        </span>
                                                    ) : ''}
                                                </div>
                                            </div>
                                            <button
                                                type="button"
                                                onClick={handleDisableAuto}
                                                disabled={disableAutoLoading}
                                                className={`rounded-xl px-3 py-2 text-xs font-semibold ring-1 transition ${disableAutoLoading
                                                    ? 'cursor-not-allowed bg-rose-500/10 text-rose-700/70 ring-rose-500/15 dark:text-rose-200/60'
                                                    : 'bg-rose-500/15 text-rose-700 ring-rose-500/25 hover:bg-rose-500/20 dark:text-rose-200'
                                                    }`}
                                            >
                                                {disableAutoLoading ? '关闭中...' : '关闭 Auto'}
                                            </button>
                                        </div>
                                    </div>
                                )}

                                {disableAutoError && (
                                    <div className="mt-3 rounded-xl border border-red-500/30 bg-red-500/10 p-3 text-xs text-red-700 dark:text-red-200">
                                        {disableAutoError}
                                    </div>
                                )}

                                {disableAutoResult && (
                                    <div className="mt-3 rounded-xl border border-emerald-500/30 bg-emerald-500/10 p-3 text-xs text-emerald-700 dark:text-emerald-200">
                                        已发起关闭：找到 <NumberFlowValue value={disableAutoResult.tasks_found || 0} formatOptions={{ maximumFractionDigits: 0 }} /> 个 Auto 任务，已请求撤出 <NumberFlowValue value={disableAutoResult.exit_requested || 0} formatOptions={{ maximumFractionDigits: 0 }} /> 个。
                                    </div>
                                )}

                                {/* 聪明钱权限 */}
                                <div className="mt-3 pt-3 border-t border-zinc-100 dark:border-white/5">
                                    <div className="flex items-center justify-between gap-3">
                                        <div>
                                            <div className="text-[11px] font-semibold text-zinc-700 dark:text-white/70">聪明钱权限</div>
                                            <div className="text-[10px] text-zinc-400 dark:text-white/30">
                                                {userAccess ? (userAccess.smart_money_enabled ? '已开启' : '未开启') : '加载中…'}
                                            </div>
                                        </div>
                                        <button
                                            type="button"
                                            onClick={handleToggleSmartMoney}
                                            disabled={smartMoneyToggling || !userAccess}
                                            className={`rounded-xl px-3 py-2 text-xs font-semibold ring-1 transition disabled:opacity-50 ${
                                                userAccess?.smart_money_enabled
                                                    ? 'bg-amber-500/10 text-amber-700 ring-amber-500/25 hover:bg-amber-500/20 dark:text-amber-300'
                                                    : 'bg-emerald-500/10 text-emerald-700 ring-emerald-500/25 hover:bg-emerald-500/20 dark:text-emerald-300'
                                            }`}
                                        >
                                            {smartMoneyToggling ? '…' : (userAccess?.smart_money_enabled ? '撤销权限' : '授予权限')}
                                        </button>
                                    </div>
                                    {smartMoneyError && (
                                        <div className="mt-2 text-[10px] text-red-600 dark:text-red-400">{smartMoneyError}</div>
                                    )}
                                </div>
                            </div>

                            {/* Auto 统计 */}
                            {userAutoStats?.stats && (
                                <div className="rounded-2xl border border-zinc-200 bg-white/40 backdrop-blur-md p-4 shadow-sm dark:border-white/10 dark:bg-white/5 dark:shadow-none">
                                    <div className="text-sm font-semibold text-zinc-900 dark:text-white/90 mb-3">Auto 统计</div>
                                    {userAutoStats.stats.window_label && (
                                        <div className="mb-2 text-[11px] text-zinc-500 dark:text-white/40">
                                            周期：{userAutoStats.stats.window_label}
                                        </div>
                                    )}
                                    <div className="grid grid-cols-2 gap-3 text-xs">
                                        <div className="rounded-xl border border-zinc-200 bg-zinc-50 p-3 dark:border-white/10 dark:bg-[#0f1116]">
                                            <div className="text-[11px] text-zinc-500 dark:text-white/40">累计收益</div>
                                            <div className="mt-0.5 text-sm font-extrabold tabular-nums text-emerald-700 dark:text-emerald-300">
                                                <NumberFlowValue value={userAutoStats?.formatted?.profit_usdt ?? '--'} formatter={() => `${userAutoStats?.formatted?.profit_usdt ?? '--'}`} /> USDT
                                            </div>
                                        </div>
                                        <div className="rounded-xl border border-zinc-200 bg-zinc-50 p-3 dark:border-white/10 dark:bg-[#0f1116]">
                                            <div className="text-[11px] text-zinc-500 dark:text-white/40">Gas 消耗</div>
                                            <div className="mt-0.5 text-sm font-extrabold tabular-nums text-zinc-900 dark:text-white/80">
                                                <NumberFlowValue value={userAutoStats?.formatted?.gas_usdt ?? '--'} formatter={() => `${userAutoStats?.formatted?.gas_usdt ?? '--'}`} /> USDT
                                            </div>
                                        </div>
                                        <div className="rounded-xl border border-zinc-200 bg-zinc-50 p-3 dark:border-white/10 dark:bg-[#0f1116]">
                                            <div className="text-[11px] text-zinc-500 dark:text-white/40">开仓 / 再平衡</div>
                                            <div className="mt-0.5 text-sm font-extrabold tabular-nums text-zinc-900 dark:text-white/80">
                                                <NumberFlowValue value={userAutoStats.stats.open_count || 0} formatOptions={{ maximumFractionDigits: 0 }} /> / <NumberFlowValue value={userAutoStats.stats.rebalance_count || 0} formatOptions={{ maximumFractionDigits: 0 }} />
                                            </div>
                                        </div>
                                        <div className="rounded-xl border border-zinc-200 bg-zinc-50 p-3 dark:border-white/10 dark:bg-[#0f1116]">
                                            <div className="text-[11px] text-zinc-500 dark:text-white/40">撤退卫士</div>
                                            <div className="mt-0.5 text-sm font-extrabold tabular-nums text-zinc-900 dark:text-white/80">
                                                <NumberFlowValue value={userAutoStats.stats.guard_count || 0} formatOptions={{ maximumFractionDigits: 0 }} />
                                            </div>
                                        </div>
                                    </div>
                                </div>
                            )}

                            {userAutoStatsError && (
                                <div className="rounded-xl border border-red-500/30 bg-red-500/10 p-3 text-xs text-red-700 dark:text-red-200">
                                    {userAutoStatsError}
                                </div>
                            )}

                            {/* 用户仓位 */}
                            <div className="rounded-2xl border border-zinc-200 bg-white/40 backdrop-blur-md p-4 shadow-sm dark:border-white/10 dark:bg-white/5 dark:shadow-none">
                                <div className="flex items-center justify-between mb-3">
                                    <div className="text-sm font-semibold text-zinc-900 dark:text-white/90">用户仓位</div>
                                    <div className="text-[11px] text-zinc-500 dark:text-white/40">
                                        <NumberFlowValue value={userPositionsList.length} formatOptions={{ maximumFractionDigits: 0 }} /> 个
                                    </div>
                                </div>

                                {userPositionsError && (
                                    <div className="rounded-xl border border-red-500/30 bg-red-500/10 p-3 text-xs text-red-700 dark:text-red-200">
                                        {userPositionsError}
                                    </div>
                                )}

                                {userPositionsLoading && userPositionsList.length === 0 && (
                                    <SkeletonList count={2} Card={SkeletonPositionCard} />
                                )}

                                {!userPositionsLoading && userPositionsList.length === 0 && !userPositionsError && (
                                    <div className="rounded-xl border border-zinc-200 bg-white/40 backdrop-blur-md p-3 text-xs text-zinc-500 dark:border-white/10 dark:bg-white/5 dark:text-white/60">
                                        该用户暂无仓位
                                    </div>
                                )}

                                <div className="space-y-3">
                                    {userPositionsList.map((p) => (
                                        <PositionCard
                                            key={[
                                                String(p?.chain || ''),
                                                String(p?.version || ''),
                                                String(p?.exchange || ''),
                                                String(p?.pool_id || ''),
                                                String(p?.position_id || ''),
                                                String(p?.task_id || ''),
                                            ].join(':')}
                                            position={p}
                                            walletAddress={userSummary?.walletAddress}
                                            bnbBalance={userSummary?.bnbBalance}
                                            pollIntervalSec={pollIntervalSec}
                                            updatedAt={userPositions?.updated_at}
                                            allowTaskActions={false}
                                        />
                                    ))}
                                </div>
                            </div>
                        </>
                    )}
                </>
            )}
        </div>
    );
}
