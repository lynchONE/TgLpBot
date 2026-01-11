import React from 'react';
import { formatRelativeTime } from '../lib/time';

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
 * AdminOnlineUsers - 在线用户列表组件
 * 显示所有有活跃任务的用户（包括 Auto 和手动）
 */
export default function AdminOnlineUsers({
    users = [],
    loading = false,
    error = '',
    tick = Date.now(),
    onSelectUser,
    selectedUserId,
}) {
    if (error) {
        return (
            <div className="rounded-xl border border-red-500/30 bg-red-500/10 p-3 text-xs text-red-700 dark:text-red-200">
                {error}
            </div>
        );
    }

    if (loading && users.length === 0) {
        return (
            <div className="rounded-xl border border-zinc-200 bg-white/70 p-3 text-xs text-zinc-500 dark:border-white/10 dark:bg-white/5 dark:text-white/60">
                加载中...
            </div>
        );
    }

    if (users.length === 0) {
        return (
            <div className="rounded-xl border border-zinc-200 bg-white/70 p-3 text-xs text-zinc-500 dark:border-white/10 dark:bg-white/5 dark:text-white/60">
                暂无在线用户
            </div>
        );
    }

    return (
        <div className="space-y-2">
            {users.map((u) => {
                const selected = Number(u?.user_id) === Number(selectedUserId);
                const label = formatUserLabel(u);
                const updatedText = formatRelativeTime(u?.updated_at, tick) || '--';
                const autoTasks = Number(u?.auto_tasks) || 0;
                const manualTasks = Number(u?.manual_tasks) || 0;
                const totalTasks = Number(u?.total_tasks) || 0;
                const isAutoEnabled = Boolean(u?.is_auto_enabled);

                return (
                    <button
                        key={u.user_id}
                        type="button"
                        onClick={() => onSelectUser?.(u)}
                        className={`w-full rounded-xl border p-3 text-left transition ${selected
                            ? 'border-emerald-500/40 bg-emerald-500/10 text-emerald-900 dark:text-emerald-100'
                            : 'border-zinc-200 bg-white/70 text-zinc-900 hover:bg-white dark:border-white/10 dark:bg-white/5 dark:text-white/80 dark:hover:bg-white/10'
                            }`}
                    >
                        <div className="flex items-start justify-between gap-3">
                            <div className="min-w-0">
                                <div className="flex items-center gap-2">
                                    <div className="text-sm font-semibold truncate">{label}</div>
                                    {isAutoEnabled && (
                                        <span className="shrink-0 rounded-md bg-emerald-500/15 px-1.5 py-0.5 text-[10px] font-semibold text-emerald-700 ring-1 ring-emerald-500/25 dark:text-emerald-300">
                                            Auto
                                        </span>
                                    )}
                                </div>
                                <div
                                    className={`mt-0.5 text-[11px] ${selected ? 'text-emerald-700/80 dark:text-emerald-200/80' : 'text-zinc-500 dark:text-white/40'
                                        }`}
                                >
                                    {u.telegram_id ? `TG ${u.telegram_id}` : 'TG --'} · ID {u.user_id}
                                </div>
                            </div>
                            <div className="text-right shrink-0">
                                <div className={`text-xs font-semibold ${selected ? 'text-emerald-700 dark:text-emerald-200' : 'text-zinc-700 dark:text-white/70'}`}>
                                    {totalTasks} 个任务
                                </div>
                                <div className={`mt-0.5 text-[11px] ${selected ? 'text-emerald-700/70 dark:text-emerald-200/70' : 'text-zinc-500 dark:text-white/40'}`}>
                                    {autoTasks > 0 && <span className="text-emerald-600 dark:text-emerald-400">{autoTasks} 自动</span>}
                                    {autoTasks > 0 && manualTasks > 0 && <span> / </span>}
                                    {manualTasks > 0 && <span className="text-sky-600 dark:text-sky-400">{manualTasks} 手动</span>}
                                </div>
                                <div
                                    className={`mt-0.5 text-[10px] ${selected ? 'text-emerald-700/60 dark:text-emerald-200/60' : 'text-zinc-400 dark:text-white/30'
                                        }`}
                                >
                                    {updatedText}
                                </div>
                            </div>
                        </div>
                    </button>
                );
            })}
        </div>
    );
}
