import React, { useMemo } from 'react';
import { formatRelativeTime } from '../lib/time';

const USD_DISPLAY_LIMIT = 1e15;
const usdFormatter = new Intl.NumberFormat('en-US', {
    style: 'currency',
    currency: 'USD',
    maximumFractionDigits: 2,
});

function formatUsd(v) {
    const n = Number(v ?? 0);
    if (!Number.isFinite(n) || Math.abs(n) > USD_DISPLAY_LIMIT) return '$--';
    return usdFormatter.format(n);
}

function formatPct(v, digits = 2) {
    const n = Number(v);
    if (!Number.isFinite(n)) return '--';
    return `${n.toFixed(digits)}%`;
}

function formatNum(v, digits = 2) {
    const n = Number(v);
    if (!Number.isFinite(n)) return '--';
    return n.toFixed(digits);
}

function zhReason(reason) {
    const r = String(reason || '').trim();
    if (!r) return '';
    if (r === 'task paused') return '任务已暂停';
    if (r === 'exit give up') return '已放弃自动撤退';
    if (r.startsWith('exit pending:')) return `已有撤退动作：${r.replace('exit pending:', '').trim()}`;
    if (r === 'fee rate too high') return '费用率过高，跳过交易量撤退';
    return r;
}

function GuardBadge({ tone, children }) {
    const cls =
        tone === 'danger'
            ? 'bg-red-500/15 text-red-700 ring-red-500/25 dark:text-red-200'
            : tone === 'warn'
                ? 'bg-amber-500/15 text-amber-700 ring-amber-500/25 dark:text-amber-200'
                : 'bg-zinc-200/70 text-zinc-700 ring-zinc-300 dark:bg-white/5 dark:text-white/70 dark:ring-white/10';
    return (
        <span className={`inline-flex items-center rounded-lg px-2 py-0.5 text-[11px] font-semibold ring-1 ${cls}`}>
            {children}
        </span>
    );
}

function CompareIndicator({ current, baseline, reverse = false, suffix = '' }) {
    const c = Number(current);
    const b = Number(baseline);
    if (!Number.isFinite(c) || !Number.isFinite(b) || b === 0) return null;
    const pct = ((c - b) / Math.abs(b)) * 100;
    const isUp = pct > 0;
    const isSignificant = Math.abs(pct) >= 1;
    if (!isSignificant) return null;
    // reverse为true时，下跌显示为红色（如交易量下跌是危险的）
    const colorClass = reverse
        ? (isUp ? 'text-emerald-500' : 'text-red-500')
        : (isUp ? 'text-emerald-500' : 'text-red-500');
    const arrow = isUp ? '↑' : '↓';
    return (
        <span className={`ml-1 text-[10px] font-bold ${colorClass}`}>
            {arrow}{Math.abs(pct).toFixed(0)}%{suffix}
        </span>
    );
}

export default function AutoMonitorCard({ task, tick }) {
    const title = String(task?.title || '').trim() || 'Auto 任务';
    const poolVersion = String(task?.pool_version || '').trim();
    const status = String(task?.status || '').trim();
    const paused = Boolean(task?.paused);

    const open = task?.open || {};
    const peak = task?.peak || {};
    const current = task?.current || {};
    const gv = task?.guard_volume || {};
    const gp = task?.guard_price_tx || {};

    const hasOpenSnapshot = Boolean(open?.at);
    const hasPeakSnapshot = Boolean(peak?.ok);
    const hasCurrentSnapshot = Boolean(current?.ok);

    const openAtText = useMemo(() => formatRelativeTime(open?.at, tick) || '--', [open?.at, tick]);
    const currentAtText = useMemo(() => formatRelativeTime(current?.updated_at, tick) || '--', [current?.updated_at, tick]);

    const exitPending = String(task?.exit_pending_action || '').trim();
    const exitReason = String(task?.exit_pending_reason || '').trim();

    // 计算基准数据 (根据配置使用peak或open)
    const baselineType = String(gv?.baseline || gp?.baseline || '').trim().toLowerCase();
    const baselineData = baselineType === 'peak' && hasPeakSnapshot ? peak : open;
    const baselineLabel = baselineType === 'peak' ? '最高点' : '开仓时';

    const baselineText = useMemo(() => {
        const baseline = String(gv?.baseline || gp?.baseline || '').trim().toLowerCase();
        if (baseline === 'peak') return '最高点';
        if (baseline === 'open') return '开仓时';
        return '--';
    }, [gv?.baseline, gp?.baseline]);

    const volBadge = useMemo(() => {
        if (!gv?.enabled) return { tone: 'muted', text: '不可用' };
        if (gv?.blocked) return { tone: 'muted', text: zhReason(gv?.blocked_reason) || '已阻塞' };
        if (gv?.skip) return { tone: 'muted', text: zhReason(gv?.skip_reason) || '已跳过' };
        if (gv?.should_exit_now) return { tone: 'danger', text: '满足撤退' };
        if (gv?.first_mark) return { tone: 'warn', text: '首次标记' };
        if (gv?.hit) return { tone: 'warn', text: '命中阈值' };
        return { tone: 'muted', text: '未命中' };
    }, [gv]);

    const ptBadge = useMemo(() => {
        if (!gp?.enabled) return { tone: 'muted', text: '不可用' };
        if (gp?.blocked) return { tone: 'muted', text: zhReason(gp?.blocked_reason) || '已阻塞' };
        if (gp?.should_exit_now) return { tone: 'danger', text: '满足撤退' };
        if (gp?.first_mark) return { tone: 'warn', text: '首次标记' };
        if (gp?.hit) return { tone: 'warn', text: '命中阈值' };
        return { tone: 'muted', text: '未命中' };
    }, [gp]);

    return (
        <div className="rounded-2xl border border-zinc-200 bg-white p-4 shadow-sm dark:border-white/10 dark:bg-[#111318] dark:shadow-none">
            <div className="flex items-start justify-between gap-3">
                <div>
                    <div className="text-sm font-extrabold text-zinc-900 dark:text-white/90">
                        {title} {poolVersion ? <span className="text-xs font-semibold text-zinc-500 dark:text-white/40">· {poolVersion.toUpperCase()}</span> : null}
                    </div>
                    <div className="mt-0.5 text-[11px] text-zinc-500 dark:text-white/40">
                        #{task?.task_id} · {task?.exchange || '--'} · 状态 {status || '--'}
                    </div>
                </div>
                <div className="flex flex-col items-end gap-1">
                    {paused ? <GuardBadge tone="warn">已暂停</GuardBadge> : null}
                    {exitPending ? <GuardBadge tone="warn">撤退中：{exitPending}</GuardBadge> : null}
                </div>
            </div>

            {exitReason ? (
                <div className="mt-2 rounded-xl border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-[11px] text-amber-700 dark:text-amber-200">
                    {exitReason}
                </div>
            ) : null}

            <div className="mt-3 grid grid-cols-2 gap-3">
                <div className="rounded-xl border border-zinc-200 bg-zinc-50 p-3 dark:border-white/10 dark:bg-[#0f1116]">
                    <div className="flex items-center justify-between">
                        <div className="text-xs font-semibold text-zinc-900 dark:text-white/80">开仓参考</div>
                        <div className="text-[11px] text-zinc-500 dark:text-white/40">{openAtText}</div>
                    </div>
                    <div className="mt-2 grid grid-cols-2 gap-x-3 gap-y-2 text-[11px]">
                        <div className="text-zinc-500 dark:text-white/40">手续费率</div>
                        <div className="text-right font-semibold tabular-nums">{hasOpenSnapshot ? formatPct(open?.fee_pct) : '--'}</div>
                        <div className="text-zinc-500 dark:text-white/40">5m 费用率</div>
                        <div className="text-right font-semibold tabular-nums">{hasOpenSnapshot ? formatPct(open?.fee_rate_5m_pct, 4) : '--'}</div>
                        <div className="text-zinc-500 dark:text-white/40">5m 费用</div>
                        <div className="text-right font-semibold tabular-nums">{hasOpenSnapshot ? formatUsd(open?.fees_5m) : '--'}</div>
                        <div className="text-zinc-500 dark:text-white/40">5m 交易量</div>
                        <div className="text-right font-semibold tabular-nums">{formatUsd(open?.volume_5m)}</div>
                        <div className="text-zinc-500 dark:text-white/40">TVL</div>
                        <div className="text-right font-semibold tabular-nums">{hasOpenSnapshot ? formatUsd(open?.tvl) : '--'}</div>
                        <div className="text-zinc-500 dark:text-white/40">5m Tx</div>
                        <div className="text-right font-semibold tabular-nums">{Number.isFinite(Number(open?.tx_5m)) ? open.tx_5m : '--'}</div>
                        <div className="text-zinc-500 dark:text-white/40">价格</div>
                        <div className="text-right font-semibold tabular-nums">{formatNum(open?.price, 8)}</div>
                    </div>
                </div>

                <div className="rounded-xl border border-zinc-200 bg-zinc-50 p-3 dark:border-white/10 dark:bg-[#0f1116]">
                    <div className="flex items-center justify-between">
                        <div className="flex items-center gap-2">
                            <div className="text-xs font-semibold text-zinc-900 dark:text-white/80">当前池子</div>
                            <span className="inline-flex items-center rounded px-1.5 py-0.5 text-[9px] font-medium bg-zinc-100 text-zinc-500 dark:bg-white/5 dark:text-white/40">
                                对比: {baselineLabel}
                            </span>
                        </div>
                        <div className="text-[11px] text-zinc-500 dark:text-white/40">{currentAtText}</div>
                    </div>
                    <div className="mt-2 grid grid-cols-2 gap-x-3 gap-y-2 text-[11px]">
                        <div className="text-zinc-500 dark:text-white/40">手续费率</div>
                        <div className="text-right font-semibold tabular-nums flex items-center justify-end">
                            {hasCurrentSnapshot ? formatPct(current?.fee_pct) : '--'}
                        </div>
                        <div className="text-zinc-500 dark:text-white/40">5m 费用率</div>
                        <div className="text-right font-semibold tabular-nums flex items-center justify-end">
                            {hasCurrentSnapshot ? formatPct(current?.fee_rate_5m_pct, 4) : '--'}
                            <CompareIndicator current={current?.fee_rate_5m_pct} baseline={baselineData?.fee_rate_5m_pct} />
                        </div>
                        <div className="text-zinc-500 dark:text-white/40">5m 费用</div>
                        <div className="text-right font-semibold tabular-nums flex items-center justify-end">
                            {hasCurrentSnapshot ? formatUsd(current?.fees_5m) : '--'}
                            <CompareIndicator current={current?.fees_5m} baseline={baselineData?.fees_5m} />
                        </div>
                        <div className="text-zinc-500 dark:text-white/40">5m 交易量</div>
                        <div className="text-right font-semibold tabular-nums flex items-center justify-end">
                            {hasCurrentSnapshot ? formatUsd(current?.volume_5m) : '--'}
                            <CompareIndicator current={current?.volume_5m} baseline={baselineData?.volume_5m} />
                        </div>
                        <div className="text-zinc-500 dark:text-white/40">TVL</div>
                        <div className="text-right font-semibold tabular-nums flex items-center justify-end">
                            {hasCurrentSnapshot ? formatUsd(current?.tvl) : '--'}
                            <CompareIndicator current={current?.tvl} baseline={baselineData?.tvl} />
                        </div>
                        <div className="text-zinc-500 dark:text-white/40">5m Tx</div>
                        <div className="text-right font-semibold tabular-nums flex items-center justify-end">
                            {hasCurrentSnapshot && Number.isFinite(Number(current?.tx_5m)) ? current.tx_5m : '--'}
                            <CompareIndicator current={current?.tx_5m} baseline={baselineData?.tx_5m} />
                        </div>
                        <div className="text-zinc-500 dark:text-white/40">价格</div>
                        <div className="text-right font-semibold tabular-nums flex items-center justify-end">
                            {hasCurrentSnapshot ? formatNum(current?.price, 8) : '--'}
                            <CompareIndicator current={current?.price} baseline={baselineData?.price} />
                        </div>
                    </div>
                </div>
            </div>

            <div className="mt-3 rounded-xl border border-zinc-200 bg-zinc-50 p-3 dark:border-white/10 dark:bg-[#0f1116]">
                <div className="flex items-center justify-between">
                    <div className="text-xs font-semibold text-zinc-900 dark:text-white/80">开仓后最高</div>
                    <div className="text-[11px] text-zinc-500 dark:text-white/40">{hasPeakSnapshot ? '已记录' : '--'}</div>
                </div>
                <div className="mt-2 grid grid-cols-2 gap-x-3 gap-y-2 text-[11px]">
                    <div className="text-zinc-500 dark:text-white/40">手续费率</div>
                    <div className="text-right font-semibold tabular-nums">{hasPeakSnapshot ? formatPct(peak?.fee_pct) : '--'}</div>
                    <div className="text-zinc-500 dark:text-white/40">5m 费用率</div>
                    <div className="text-right font-semibold tabular-nums">{hasPeakSnapshot ? formatPct(peak?.fee_rate_5m_pct, 4) : '--'}</div>
                    <div className="text-zinc-500 dark:text-white/40">5m 费用</div>
                    <div className="text-right font-semibold tabular-nums">{hasPeakSnapshot ? formatUsd(peak?.fees_5m) : '--'}</div>
                    <div className="text-zinc-500 dark:text-white/40">5m 交易量</div>
                    <div className="text-right font-semibold tabular-nums">{hasPeakSnapshot ? formatUsd(peak?.volume_5m) : '--'}</div>
                    <div className="text-zinc-500 dark:text-white/40">TVL</div>
                    <div className="text-right font-semibold tabular-nums">{hasPeakSnapshot ? formatUsd(peak?.tvl) : '--'}</div>
                    <div className="text-zinc-500 dark:text-white/40">5m Tx</div>
                    <div className="text-right font-semibold tabular-nums">{hasPeakSnapshot && Number.isFinite(Number(peak?.tx_5m)) ? peak.tx_5m : '--'}</div>
                    <div className="text-zinc-500 dark:text-white/40">价格</div>
                    <div className="text-right font-semibold tabular-nums">{hasPeakSnapshot ? formatNum(peak?.price, 8) : '--'}</div>
                </div>
            </div>

            <div className="mt-3 rounded-xl border border-zinc-200 bg-white p-3 dark:border-white/10 dark:bg-white/5">
                <div className="flex flex-wrap items-center gap-1.5">
                    <div className="text-xs font-semibold text-zinc-900 dark:text-white/80 mr-1">撤退卫士</div>
                    <GuardBadge tone={volBadge.tone}>{`交易量:${volBadge.text}`}</GuardBadge>
                    <GuardBadge tone={ptBadge.tone}>{`价+Tx:${ptBadge.text}`}</GuardBadge>
                </div>

                {/* 连续跌破/涨破计数 */}
                <div className="mt-2 flex items-center gap-3 text-[11px]">
                    <div className="flex items-center gap-1">
                        <span className="text-zinc-500 dark:text-white/40">连续跌破:</span>
                        <span className={`font-semibold ${task?.range_break_down_streak >= 2 ? 'text-red-500' : task?.range_break_down_streak >= 1 ? 'text-amber-500' : ''}`}>
                            {task?.range_break_down_streak || 0}/2
                        </span>
                    </div>
                    <div className="flex items-center gap-1">
                        <span className="text-zinc-500 dark:text-white/40">连续涨破:</span>
                        <span className={`font-semibold ${task?.range_break_up_streak >= 2 ? 'text-amber-500' : ''}`}>
                            {task?.range_break_up_streak || 0}/2
                        </span>
                    </div>
                    {task?.next_range_multiplier > 1 ? (
                        <div className="flex items-center gap-1">
                            <span className="text-zinc-500 dark:text-white/40">下次扩大:</span>
                            <span className="font-semibold text-amber-500">{task.next_range_multiplier}x</span>
                        </div>
                    ) : null}
                </div>

                <div className="mt-2 grid grid-cols-2 gap-3 text-[11px]">
                    <div className="rounded-lg border border-zinc-200 bg-zinc-50 p-2 dark:border-white/10 dark:bg-[#0f1116]">
                        <div className="font-semibold text-zinc-900 dark:text-white/80">交易量撤退</div>
                        <div className="mt-1 grid grid-cols-2 gap-x-2 gap-y-1">
                            <div className="text-zinc-500 dark:text-white/40">阈值跌幅</div>
                            <div className="text-right font-semibold tabular-nums">{formatPct(Number(gv?.drop_pct || 0) * 100, 0)}</div>
                            <div className="text-zinc-500 dark:text-white/40">当前/阈值</div>
                            <div className="text-right font-semibold tabular-nums">
                                {formatUsd(gv?.current_volume_5m)} / {formatUsd(gv?.threshold)}
                            </div>
                            <div className="text-zinc-500 dark:text-white/40">命中阈值</div>
                            <div className="text-right font-semibold">{gv?.hit ? '是' : '否'}</div>
                            <div className="text-zinc-500 dark:text-white/40">首次标记</div>
                            <div className="text-right font-semibold">{gv?.first_mark ? '是' : '否'}</div>
                            <div className="text-zinc-500 dark:text-white/40">已上膛</div>
                            <div className="text-right font-semibold">{gv?.armed ? '是' : '否'}</div>
                            <div className="text-zinc-500 dark:text-white/40">连续下降</div>
                            <div className="text-right font-semibold">{gv?.should_exit_now ? '是' : '否'}</div>
                        </div>
                        {gv?.blocked_reason || gv?.skip_reason ? (
                            <div className="mt-2 text-zinc-500 dark:text-white/50">
                                {zhReason(gv?.blocked_reason) || zhReason(gv?.skip_reason)}
                            </div>
                        ) : null}
                    </div>

                    <div className="rounded-lg border border-zinc-200 bg-zinc-50 p-2 dark:border-white/10 dark:bg-[#0f1116]">
                        <div className="font-semibold text-zinc-900 dark:text-white/80">价格 + Tx 撤退</div>
                        <div className="mt-1 grid grid-cols-2 gap-x-2 gap-y-1">
                            <div className="text-zinc-500 dark:text-white/40">价格阈值</div>
                            <div className="text-right font-semibold tabular-nums">{formatPct(Number(gp?.price_drop_pct || gp?.drop_pct || 0) * 100, 0)}</div>
                            <div className="text-zinc-500 dark:text-white/40">Tx阈值</div>
                            <div className="text-right font-semibold tabular-nums">{formatPct(Number(gp?.tx_drop_pct || gp?.drop_pct || 0) * 100, 0)}</div>
                            <div className="text-zinc-500 dark:text-white/40">价格命中</div>
                            <div className="text-right font-semibold">{gp?.price_hit ? '是' : '否'}</div>
                            <div className="text-zinc-500 dark:text-white/40">Tx 命中</div>
                            <div className="text-right font-semibold">{gp?.tx_hit ? '是' : '否'}</div>
                            <div className="text-zinc-500 dark:text-white/40">同时命中</div>
                            <div className="text-right font-semibold">{gp?.hit ? '是' : '否'}</div>
                            <div className="text-zinc-500 dark:text-white/40">首次标记</div>
                            <div className="text-right font-semibold">{gp?.first_mark ? '是' : '否'}</div>
                            <div className="text-zinc-500 dark:text-white/40">已上膛</div>
                            <div className="text-right font-semibold">{gp?.armed ? '是' : '否'}</div>
                            <div className="text-zinc-500 dark:text-white/40">满足撤退</div>
                            <div className="text-right font-semibold">{gp?.should_exit_now ? '是' : '否'}</div>
                        </div>
                        {gp?.blocked_reason ? (
                            <div className="mt-2 text-zinc-500 dark:text-white/50">{zhReason(gp?.blocked_reason)}</div>
                        ) : null}
                    </div>
                </div>
            </div>
        </div>
    );
}
