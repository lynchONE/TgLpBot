import React from 'react';
import { formatRelativeTime } from '../lib/time';
import NumberFlowValue from './NumberFlowValue.jsx';

/**
 * 格式化用户标签
 */
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

/**
 * 格式化交易对
 */
function formatTradingPair(task) {
    const t0 = String(task?.token0_symbol || '').trim();
    const t1 = String(task?.token1_symbol || '').trim();
    if (t0 && t1) return `${t0}/${t1}`;
    return '--';
}

/**
 * 格式化费率
 */
function formatFee(fee) {
    if (!Number.isFinite(fee) || fee <= 0) return '';
    return `${(fee / 10000).toFixed(2)}%`;
}

/**
 * 格式化状态
 */
function formatStatus(status) {
    switch (status) {
        case 'opening': return '开仓中';
        case 'running': return '运行中';
        case 'waiting': return '等待中';
        case 'stopping': return '撤退中';
        case 'stopped': return '已停止';
        case 'error': return '错误';
        default: return status || '--';
    }
}

/**
 * 状态颜色类
 */
function statusColorClass(status) {
    switch (status) {
        case 'running': return 'bg-emerald-500/15 text-emerald-700 ring-emerald-500/25 dark:text-emerald-300';
        case 'opening': return 'bg-sky-500/15 text-sky-700 ring-sky-500/25 dark:text-sky-300';
        case 'waiting': return 'bg-amber-500/15 text-amber-700 ring-amber-500/25 dark:text-amber-300';
        case 'stopping': return 'bg-rose-500/15 text-rose-700 ring-rose-500/25 dark:text-rose-300';
        default: return 'bg-zinc-500/10 text-zinc-700 ring-zinc-500/20 dark:text-white/60';
    }
}

/**
 * AdminActiveTasks - 活跃任务列表组件
 * 显示所有正在运行的任务
 */
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
            <div className="rounded-xl border border-zinc-200 bg-white/40 backdrop-blur-md p-3 text-xs text-zinc-500 dark:border-white/10 dark:bg-white/5 dark:text-white/60">
                加载中...
            </div>
        );
    }

    if (tasks.length === 0) {
        return (
            <div className="rounded-xl border border-zinc-200 bg-white/40 backdrop-blur-md p-3 text-xs text-zinc-500 dark:border-white/10 dark:bg-white/5 dark:text-white/60">
                暂无活跃任务
            </div>
        );
    }

    // 按用户分组统计
    const userStats = {};
    for (const t of tasks) {
        const uid = t.user_id;
        if (!userStats[uid]) {
            userStats[uid] = { auto: 0, manual: 0 };
        }
        if (t.is_auto) {
            userStats[uid].auto++;
        } else {
            userStats[uid].manual++;
        }
    }

    const autoCount = tasks.filter(t => t.is_auto).length;
    const manualCount = tasks.length - autoCount;

    return (
        <div className="space-y-3">
            {/* 统计摘要 */}
            <div className="flex items-center justify-between rounded-xl border border-zinc-200 bg-zinc-50 px-3 py-2 dark:border-white/10 dark:bg-[#0f1116]">
                <div className="text-xs text-zinc-600 dark:text-white/60">
                    共 <span className="font-semibold text-zinc-900 dark:text-white"><NumberFlowValue value={tasks.length} formatOptions={{ maximumFractionDigits: 0 }} /></span> 个活跃任务
                </div>
                <div className="flex items-center gap-3 text-[11px]">
                    <span className="text-emerald-600 dark:text-emerald-400">
                        <span className="font-semibold"><NumberFlowValue value={autoCount} formatOptions={{ maximumFractionDigits: 0 }} /></span> 自动
                    </span>
                    <span className="text-sky-600 dark:text-sky-400">
                        <span className="font-semibold"><NumberFlowValue value={manualCount} formatOptions={{ maximumFractionDigits: 0 }} /></span> 手动
                    </span>
                </div>
            </div>

            {/* 任务列表 */}
            <div className="space-y-2">
                {tasks.map((t) => {
                    const tradingPair = formatTradingPair(t);
                    const fee = formatFee(t.fee);
                    const userLabel = formatUserLabel(t);
                    const status = formatStatus(t.status);
                    const statusClass = statusColorClass(t.status);
                    const lastCheck = formatRelativeTime(t.last_check_time, tick) || '--';
                    const amountUsd = Number.isFinite(t.amount_usdt) ? t.amount_usdt.toFixed(2) : '--';

                    return (
                        <div
                            key={t.task_id}
                            className="rounded-xl border border-zinc-200 bg-white/40 backdrop-blur-md p-3 dark:border-white/10 dark:bg-white/5"
                            onClick={() => onSelectTask?.(t)}
                        >
                            <div className="flex items-start justify-between gap-3">
                                <div className="min-w-0">
                                    <div className="flex items-center gap-2 flex-wrap">
                                        <span className="text-sm font-semibold text-zinc-900 dark:text-white/90 truncate">
                                            {tradingPair}
                                        </span>
                                        {fee && (
                                            <span className="shrink-0 text-[10px] text-zinc-500 dark:text-white/40">
                                                {fee}
                                            </span>
                                        )}
                                        <span className={`shrink-0 rounded-md px-1.5 py-0.5 text-[10px] font-semibold ring-1 ${t.is_auto
                                            ? 'bg-emerald-500/15 text-emerald-700 ring-emerald-500/25 dark:text-emerald-300'
                                            : 'bg-sky-500/15 text-sky-700 ring-sky-500/25 dark:text-sky-300'
                                            }`}>
                                            {t.is_auto ? '自动' : '手动'}
                                        </span>
                                        <span className={`shrink-0 rounded-md px-1.5 py-0.5 text-[10px] font-semibold ring-1 ${statusClass}`}>
                                            {status}
                                        </span>
                                        {t.paused && (
                                            <span className="shrink-0 rounded-md bg-amber-500/15 px-1.5 py-0.5 text-[10px] font-semibold text-amber-700 ring-1 ring-amber-500/25 dark:text-amber-300">
                                                已暂停
                                            </span>
                                        )}
                                    </div>
                                    <div className="mt-1 text-[11px] text-zinc-500 dark:text-white/40">
                                        {userLabel} · ID <NumberFlowValue value={t.task_id || 0} formatOptions={{ maximumFractionDigits: 0 }} />
                                    </div>
                                </div>
                                <div className="text-right shrink-0">
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
