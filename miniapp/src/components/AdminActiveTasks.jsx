import React from 'react';
import { formatRelativeTime } from '../lib/time';
import NumberFlowValue from './NumberFlowValue.jsx';

function formatUserLabel(task) {
    if (!task) return '--';
    const first = String(task.first_name || '').trim();
    const last = String(task.last_name || '').trim();
    const username = String(task.username || '').trim();
    const fullName = [first, last].filter(Boolean).join(' ');
    if (fullName && username) return `${fullName} (@${username})`;
    if (fullName) return fullName;
    if (username) return `@${username}`;
    return `用户 ${task.user_id || '--'}`;
}

function formatTradingPair(task) {
    const token0 = String(task?.token0_symbol || '').trim();
    const token1 = String(task?.token1_symbol || '').trim();
    if (token0 && token1) return `${token0}/${token1}`;
    return '--';
}

function formatFee(fee) {
    if (!Number.isFinite(fee) || fee <= 0) return '';
    return `${(fee / 10000).toFixed(4)}%`;
}

function formatStatus(status) {
    switch (status) {
        case 'opening': return '开仓中';
        case 'running': return '运行中';
        case 'waiting': return '等待中';
        case 'stopping': return '退出中';
        case 'stopped': return '已停止';
        case 'error': return '错误';
        default: return status || '--';
    }
}

function statusColorClass(status) {
    switch (status) {
        case 'running': return 'bg-emerald-500/15 text-emerald-700 ring-emerald-500/25 dark:text-emerald-300';
        case 'opening': return 'bg-sky-500/15 text-sky-700 ring-sky-500/25 dark:text-sky-300';
        case 'waiting': return 'bg-amber-500/15 text-amber-700 ring-amber-500/25 dark:text-amber-300';
        case 'stopping': return 'bg-rose-500/15 text-rose-700 ring-rose-500/25 dark:text-rose-300';
        default: return 'bg-zinc-500/10 text-zinc-700 ring-zinc-500/20 dark:text-white/60';
    }
}

export default function AdminActiveTasks({
    tasks = [],
    loading = false,
    error = '',
    tick = Date.now(),
    onSelectTask,
}) {
    if (error) {
        return (
            <div className="rounded-xl border border-red-500/30 bg-red-500/10 p-3 text-xs text-red-700 dark:text-red-200">
                {error}
            </div>
        );
    }

    if (loading && tasks.length === 0) {
        return (
            <div className="rounded-xl border border-zinc-200 bg-white/40 p-3 text-xs text-zinc-500 dark:border-white/10 dark:bg-white/5 dark:text-white/60">
                加载中...
            </div>
        );
    }

    if (tasks.length === 0) {
        return (
            <div className="rounded-xl border border-zinc-200 bg-white/40 p-3 text-xs text-zinc-500 dark:border-white/10 dark:bg-white/5 dark:text-white/60">
                暂无活跃任务
            </div>
        );
    }

    return (
        <div className="space-y-3">
            <div className="flex items-center justify-between rounded-xl border border-zinc-200 bg-zinc-50 px-3 py-2 dark:border-white/10 dark:bg-[#0f1116]">
                <div className="text-xs text-zinc-600 dark:text-white/60">
                    共 <span className="font-semibold text-zinc-900 dark:text-white">
                        <NumberFlowValue value={tasks.length} formatOptions={{ maximumFractionDigits: 0 }} />
                    </span> 个活跃任务
                </div>
            </div>

            <div className="space-y-2">
                {tasks.map((task) => {
                    const lastCheck = formatRelativeTime(task.last_check_time, tick) || '--';
                    const amountUsd = Number.isFinite(task.amount_usdt) ? task.amount_usdt.toFixed(2) : '--';

                    return (
                        <div
                            key={task.task_id}
                            className="rounded-xl border border-zinc-200 bg-white/40 p-3 dark:border-white/10 dark:bg-white/5"
                            onClick={() => onSelectTask?.(task)}
                        >
                            <div className="flex items-start justify-between gap-3">
                                <div className="min-w-0">
                                    <div className="flex flex-wrap items-center gap-2">
                                        <span className="truncate text-sm font-semibold text-zinc-900 dark:text-white/90">
                                            {formatTradingPair(task)}
                                        </span>
                                        {formatFee(task.fee) && (
                                            <span className="shrink-0 text-[10px] text-zinc-500 dark:text-white/40">
                                                {formatFee(task.fee)}
                                            </span>
                                        )}
                                        <span className={`shrink-0 rounded-md px-1.5 py-0.5 text-[10px] font-semibold ring-1 ${statusColorClass(task.status)}`}>
                                            {formatStatus(task.status)}
                                        </span>
                                        {task.paused && (
                                            <span className="shrink-0 rounded-md bg-amber-500/15 px-1.5 py-0.5 text-[10px] font-semibold text-amber-700 ring-1 ring-amber-500/25 dark:text-amber-300">
                                                已暂停
                                            </span>
                                        )}
                                    </div>
                                    <div className="mt-1 text-[11px] text-zinc-500 dark:text-white/40">
                                        {formatUserLabel(task)} | 任务 ID <NumberFlowValue value={task.task_id || 0} formatOptions={{ maximumFractionDigits: 0 }} />
                                    </div>
                                </div>
                                <div className="shrink-0 text-right">
                                    <div className="text-sm font-semibold tabular-nums text-zinc-900 dark:text-white/80">
                                        <NumberFlowValue value={amountUsd} formatter={() => `$${amountUsd}`} />
                                    </div>
                                    <div className="mt-0.5 text-[10px] text-zinc-400 dark:text-white/30">
                                        <NumberFlowValue value={lastCheck} formatter={() => lastCheck} />
                                    </div>
                                </div>
                            </div>
                        </div>
                    );
                })}
            </div>
        </div>
    );
}
