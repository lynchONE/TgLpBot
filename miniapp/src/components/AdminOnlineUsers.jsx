import React from 'react';
import { formatRelativeTime } from '../lib/time';
import NumberFlowValue from './NumberFlowValue.jsx';

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
            <div className="rounded-xl border border-zinc-200 bg-white/40 p-3 text-xs text-zinc-500 dark:border-white/10 dark:bg-white/5 dark:text-white/60">
                加载中...
            </div>
        );
    }

    if (users.length === 0) {
        return (
            <div className="rounded-xl border border-zinc-200 bg-white/40 p-3 text-xs text-zinc-500 dark:border-white/10 dark:bg-white/5 dark:text-white/60">
                暂无在线用户
            </div>
        );
    }

    return (
        <div className="space-y-2">
            {users.map((user) => {
                const selected = Number(user?.user_id) === Number(selectedUserId);
                const updatedText = formatRelativeTime(user?.updated_at, tick) || '--';
                const totalTasks = Number(user?.total_tasks) || 0;

                return (
                    <button
                        key={user.user_id}
                        type="button"
                        onClick={() => onSelectUser?.(user)}
                        className={`w-full rounded-xl border p-3 text-left transition ${
                            selected
                                ? 'border-emerald-500/40 bg-emerald-500/10 text-emerald-900 dark:text-emerald-100'
                                : 'border-zinc-200 bg-white/70 text-zinc-900 hover:bg-white dark:border-white/10 dark:bg-white/5 dark:text-white/80 dark:hover:bg-white/10'
                        }`}
                    >
                        <div className="flex items-start justify-between gap-3">
                            <div className="min-w-0">
                                <div className="truncate text-sm font-semibold">{formatUserLabel(user)}</div>
                                <div className={`mt-0.5 text-[11px] ${
                                    selected ? 'text-emerald-700/80 dark:text-emerald-200/80' : 'text-zinc-500 dark:text-white/40'
                                }`}>
                                    {user.telegram_id ? `TG ${user.telegram_id}` : 'TG --'} | ID {user.user_id}
                                </div>
                            </div>
                            <div className="shrink-0 text-right">
                                <div className={`text-xs font-semibold ${
                                    selected ? 'text-emerald-700 dark:text-emerald-200' : 'text-zinc-700 dark:text-white/70'
                                }`}>
                                    <NumberFlowValue value={totalTasks} formatOptions={{ maximumFractionDigits: 0 }} /> 个任务
                                </div>
                                <div className={`mt-0.5 text-[10px] ${
                                    selected ? 'text-emerald-700/60 dark:text-emerald-200/60' : 'text-zinc-400 dark:text-white/30'
                                }`}>
                                    <NumberFlowValue value={updatedText} formatter={() => updatedText} />
                                </div>
                            </div>
                        </div>
                    </button>
                );
            })}
        </div>
    );
}
