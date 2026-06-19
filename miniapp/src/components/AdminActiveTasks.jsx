import { formatRelativeTime } from '../lib/time';
import NumberFlowValue from './NumberFlowValue.jsx';
import StatusDot from './admin/StatusDot.jsx';

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

function statusTone(status) {
    switch (status) {
        case 'running': return 'ok';
        case 'opening': return 'accent';
        case 'waiting': return 'warn';
        case 'stopping': return 'danger';
        case 'error': return 'danger';
        default: return 'idle';
    }
}

function statusBadge(status) {
    switch (status) {
        case 'running': return 'bg-emerald-500/15 text-emerald-700 ring-emerald-500/20 dark:text-emerald-300';
        case 'opening': return 'bg-sky-500/15 text-sky-700 ring-sky-500/20 dark:text-sky-300';
        case 'waiting': return 'bg-amber-500/15 text-amber-700 ring-amber-500/20 dark:text-amber-300';
        case 'stopping': return 'bg-rose-500/15 text-rose-700 ring-rose-500/20 dark:text-rose-300';
        case 'error': return 'bg-red-500/15 text-red-700 ring-red-500/20 dark:text-red-300';
        default: return 'bg-zinc-500/10 text-zinc-700 ring-zinc-500/20 dark:text-white/60';
    }
}

export default function AdminActiveTasks({
    tasks = [],
    loading = false,
    error = '',
    tick = Date.now(),
    onSelectTask,
    totalCount,
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
            <div className="rounded-xl border border-zinc-200/70 bg-white/50 p-4 text-center text-xs text-zinc-500 dark:border-white/10 dark:bg-[#0f1116]/70 dark:text-white/55">
                加载中…
            </div>
        );
    }

    if (tasks.length === 0) {
        const isFiltered = typeof totalCount === 'number' && totalCount > 0;
        return (
            <div className="rounded-xl border border-dashed border-zinc-200 p-5 text-center text-xs text-zinc-400 dark:border-white/10 dark:text-white/35">
                {isFiltered ? '没有匹配的活跃任务' : '暂无活跃任务'}
            </div>
        );
    }

    return (
        <div className="overflow-hidden rounded-2xl border border-zinc-200/70 bg-white/65 backdrop-blur-sm dark:border-white/10 dark:bg-[#0f1116]/80">
            <ul className="divide-y divide-zinc-200/60 dark:divide-white/5">
                {tasks.map((task) => {
                    const lastCheck = formatRelativeTime(task.last_check_time, tick) || '--';
                    const amountUsd = Number.isFinite(task.amount_usdt) ? task.amount_usdt.toFixed(2) : '--';
                    const tone = statusTone(task.status);
                    const badge = statusBadge(task.status);
                    return (
                        <li key={task.task_id}>
                            <button
                                type="button"
                                onClick={() => onSelectTask?.(task)}
                                className="flex w-full items-start gap-3 px-3 py-2.5 text-left transition hover:bg-zinc-100/60 dark:hover:bg-white/5"
                            >
                                <div className="mt-1"><StatusDot tone={tone} pulse={tone === 'ok' || tone === 'accent'} size="sm" /></div>
                                <div className="min-w-0 flex-1">
                                    <div className="flex flex-wrap items-center gap-1.5">
                                        <span className="truncate text-[13px] font-bold text-zinc-900 dark:text-white/90">
                                            {formatTradingPair(task)}
                                        </span>
                                        {formatFee(task.fee) && (
                                            <span className="shrink-0 rounded bg-zinc-100 px-1 py-px text-[9px] font-medium tabular-nums text-zinc-500 dark:bg-white/5 dark:text-white/45">
                                                {formatFee(task.fee)}
                                            </span>
                                        )}
                                        <span className={`shrink-0 rounded px-1.5 py-0.5 text-[9px] font-bold ring-1 ${badge}`}>
                                            {formatStatus(task.status)}
                                        </span>
                                        {task.paused && (
                                            <span className="shrink-0 rounded bg-amber-500/15 px-1.5 py-0.5 text-[9px] font-bold text-amber-700 ring-1 ring-amber-500/20 dark:text-amber-300">
                                                暂停
                                            </span>
                                        )}
                                    </div>
                                    <div className="mt-0.5 truncate text-[10px] text-zinc-500 dark:text-white/40">
                                        {formatUserLabel(task)} · #<NumberFlowValue value={task.task_id || 0} formatOptions={{ maximumFractionDigits: 0 }} /> · {lastCheck}
                                    </div>
                                </div>
                                <div className="shrink-0 text-right">
                                    <div className="text-[13px] font-bold tabular-nums text-zinc-900 dark:text-white/85">
                                        <NumberFlowValue value={amountUsd} formatter={() => `$${amountUsd}`} />
                                    </div>
                                    <div className="text-[9px] uppercase tracking-wider text-zinc-400 dark:text-white/30">
                                        持仓
                                    </div>
                                </div>
                            </button>
                        </li>
                    );
                })}
            </ul>
        </div>
    );
}
