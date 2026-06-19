import { formatRelativeTime } from '../lib/time';
import NumberFlowValue from './NumberFlowValue.jsx';
import StatusDot from './admin/StatusDot.jsx';
import { getBrandTheme } from '../lib/brand';

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
    accentTheme = 'lime',
    onSelectUser,
    selectedUserId,
    totalCount,
}) {
    const brand = getBrandTheme(accentTheme);
    const selectedTone = brand.key === 'emerald'
        ? 'border-emerald-500/40 bg-emerald-500/10'
        : 'border-[#bcff2f]/45 bg-[#bcff2f]/10';

    if (error) {
        return (
            <div className="rounded-xl border border-red-500/30 bg-red-500/10 p-3 text-xs text-red-700 dark:text-red-200">
                {error}
            </div>
        );
    }

    if (loading && users.length === 0) {
        return (
            <div className="rounded-xl border border-zinc-200/70 bg-white/50 p-4 text-center text-xs text-zinc-500 dark:border-white/10 dark:bg-[#0f1116]/70 dark:text-white/55">
                加载中…
            </div>
        );
    }

    if (users.length === 0) {
        const isFiltered = typeof totalCount === 'number' && totalCount > 0;
        return (
            <div className="rounded-xl border border-dashed border-zinc-200 p-5 text-center text-xs text-zinc-400 dark:border-white/10 dark:text-white/35">
                {isFiltered ? '没有匹配的在线用户' : '暂无在线用户'}
            </div>
        );
    }

    return (
        <div className="overflow-hidden rounded-2xl border border-zinc-200/70 bg-white/65 backdrop-blur-sm dark:border-white/10 dark:bg-[#0f1116]/80">
            <ul className="divide-y divide-zinc-200/60 dark:divide-white/5">
                {users.map((user) => {
                    const selected = Number(user?.user_id) === Number(selectedUserId);
                    const updatedText = formatRelativeTime(user?.updated_at, tick) || '--';
                    const totalTasks = Number(user?.total_tasks) || 0;
                    return (
                        <li key={user.user_id}>
                            <button
                                type="button"
                                onClick={() => onSelectUser?.(user)}
                                className={`flex w-full items-center gap-3 px-3 py-2.5 text-left transition ${
                                    selected
                                        ? selectedTone
                                        : 'hover:bg-zinc-100/60 dark:hover:bg-white/5'
                                }`}
                            >
                                <StatusDot tone="ok" pulse size="sm" />
                                <div className="min-w-0 flex-1">
                                    <div className="truncate text-[13px] font-semibold text-zinc-900 dark:text-white/90">
                                        {formatUserLabel(user)}
                                    </div>
                                    <div className="mt-0.5 truncate text-[10px] text-zinc-500 dark:text-white/40">
                                        TG {user.telegram_id || '--'} · ID {user.user_id} · {updatedText}
                                    </div>
                                </div>
                                <div className="shrink-0 text-right">
                                    <div className="text-[13px] font-bold tabular-nums text-zinc-900 dark:text-white/85">
                                        <NumberFlowValue value={totalTasks} formatOptions={{ maximumFractionDigits: 0 }} />
                                    </div>
                                    <div className="text-[9px] uppercase tracking-wider text-zinc-400 dark:text-white/30">
                                        任务
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
