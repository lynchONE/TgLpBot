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

// 标签徽章组件
function GuardBadge({ tone, children }) {
    const cls =
        tone === 'danger'
            ? 'bg-red-500/12 text-red-600 ring-red-500/20 dark:bg-red-500/15 dark:text-red-300 dark:ring-red-400/25'
            : tone === 'warn'
                ? 'bg-amber-500/12 text-amber-700 ring-amber-500/20 dark:bg-amber-500/15 dark:text-amber-300 dark:ring-amber-400/25'
                : 'bg-zinc-100 text-zinc-600 ring-zinc-200/80 dark:bg-white/5 dark:text-white/55 dark:ring-white/8';
    return (
        <span className={`inline-flex items-center rounded-lg px-2 py-0.5 text-[10px] font-semibold ring-1 ${cls}`}>
            {children}
        </span>
    );
}

// 涨跌比较指示器
function CompareIndicator({ current, baseline }) {
    const c = Number(current);
    const b = Number(baseline);
    if (!Number.isFinite(c) || !Number.isFinite(b) || b === 0) return null;
    const pct = ((c - b) / Math.abs(b)) * 100;
    const isUp = pct > 0;
    const isSignificant = Math.abs(pct) >= 1;
    if (!isSignificant) return null;
    return (
        <span className={`ml-1 text-[10px] font-bold ${isUp ? 'text-emerald-500' : 'text-red-500'}`}>
            {isUp ? '↑' : '↓'}{Math.abs(pct).toFixed(0)}%
        </span>
    );
}

// 绿色小勾图标
function CheckIcon() {
    return (
        <svg className="h-3.5 w-3.5 text-emerald-500" viewBox="0 0 16 16" fill="currentColor">
            <path fillRule="evenodd" d="M13.78 4.22a.75.75 0 010 1.06l-7.25 7.25a.75.75 0 01-1.06 0L2.22 9.28a.75.75 0 011.06-1.06L6 10.94l6.72-6.72a.75.75 0 011.06 0z" clipRule="evenodd" />
        </svg>
    );
}

// 条件状态显示
function ConditionStatus({ hit }) {
    if (hit) {
        return (
            <span className="inline-flex items-center justify-end gap-0.5">
                <CheckIcon />
            </span>
        );
    }
    return <span className="text-zinc-300 dark:text-white/20">否</span>;
}

// 数据行组件
function DataRow({ label, current, baseline, renderCurrent, renderBaseline, showCompare = false }) {
    return (
        <>
            <div className="text-zinc-400 dark:text-white/35 py-0.5">{label}</div>
            <div className="text-right font-semibold tabular-nums py-0.5">
                {renderBaseline || baseline}
            </div>
            <div className="text-right font-semibold tabular-nums flex items-center justify-end py-0.5">
                {renderCurrent || current}
                {showCompare && <CompareIndicator current={current} baseline={baseline} />}
            </div>
        </>
    );
}

export default function AutoMonitorCard({ task, tick, isBlacklisted = false }) {
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

    const currentAtText = useMemo(() => formatRelativeTime(current?.updated_at, tick) || '--', [current?.updated_at, tick]);

    const exitPending = String(task?.exit_pending_action || '').trim();
    const exitReason = String(task?.exit_pending_reason || '').trim();

    const baselineType = String(gv?.baseline || gp?.baseline || '').trim().toLowerCase();
    const isPeakBaseline = baselineType === 'peak';
    const baselineData = isPeakBaseline && hasPeakSnapshot ? peak : open;
    const hasBaselineSnapshot = isPeakBaseline ? hasPeakSnapshot : hasOpenSnapshot;
    const baselineLabel = isPeakBaseline ? '最高点' : '开仓时';

    const baselineAtText = useMemo(() => {
        if (isPeakBaseline) {
            return hasPeakSnapshot ? '已记录' : '--';
        }
        return formatRelativeTime(open?.at, tick) || '--';
    }, [isPeakBaseline, hasPeakSnapshot, open?.at, tick]);

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

    // 卡片状态色
    const isAlert = volBadge.tone === 'danger' || ptBadge.tone === 'danger' || isBlacklisted;
    const isWarn = !isAlert && (volBadge.tone === 'warn' || ptBadge.tone === 'warn' || paused || exitPending);
    const barClass = isAlert
        ? 'bg-gradient-to-b from-red-500 to-red-600'
        : isWarn
            ? 'bg-gradient-to-b from-amber-400 to-amber-500'
            : 'bg-gradient-to-b from-violet-500 to-violet-600';

    return (
        <div className="relative rounded-2xl border border-zinc-200/80 bg-white/60 backdrop-blur-md shadow-sm overflow-hidden dark:border-white/10 dark:bg-white/5 dark:shadow-none transition-all duration-200 active:scale-[0.985]">
            {/* 左侧彩色状态指示条 */}
            <div className={`absolute left-0 top-0 bottom-0 w-[3px] ${barClass}`} />

            <div className="pl-4 pr-3 pt-3.5 pb-3">
                {/* ── 顶部标题行 ── */}
                <div className="flex items-start justify-between gap-3">
                    <div className="min-w-0">
                        <div className="flex items-baseline gap-1.5 flex-wrap">
                            <span className="text-sm font-bold text-zinc-900 dark:text-white/95">{title}</span>
                            {poolVersion && (
                                <span className="text-[10px] font-semibold rounded-md bg-zinc-100 px-1.5 py-0.5 text-zinc-500 dark:bg-white/8 dark:text-white/45">
                                    {poolVersion.toUpperCase()}
                                </span>
                            )}
                        </div>
                        <div className="mt-0.5 text-[10px] text-zinc-400 dark:text-white/35">
                            #{task?.task_id} · {task?.exchange || '--'} · {status || '--'}
                        </div>
                    </div>
                    <div className="flex flex-wrap items-end justify-end gap-1 shrink-0">
                        {isBlacklisted && <GuardBadge tone="danger">黑名单</GuardBadge>}
                        {paused && <GuardBadge tone="warn">已暂停</GuardBadge>}
                        {exitPending && <GuardBadge tone="warn">撤退: {exitPending}</GuardBadge>}
                    </div>
                </div>

                {/* 撤退原因警告 */}
                {exitReason ? (
                    <div className="mt-2 flex items-start gap-1.5 rounded-xl border border-amber-500/25 bg-amber-500/8 px-3 py-2 text-[11px] text-amber-700 dark:border-amber-400/20 dark:bg-amber-500/10 dark:text-amber-300">
                        <span className="mt-0.5 text-amber-500">⚠</span>
                        <span>{exitReason}</span>
                    </div>
                ) : null}

                {/* ── 基准 vs 当前 对比数据 ── */}
                <div className="mt-3 rounded-xl border border-zinc-100/80 overflow-hidden dark:border-white/10">
                    {/* 表头 */}
                    <div className="grid grid-cols-3 gap-0 border-b border-zinc-100/80 dark:border-white/10">
                        <div className="px-3 py-2 text-[10px] font-semibold text-zinc-400 dark:text-white/30 uppercase tracking-wide">指标</div>
                        <div className={`px-2 py-2 text-[10px] font-semibold uppercase tracking-wide text-right border-l dark:border-white/10 ${isPeakBaseline ? 'text-amber-600/80 dark:text-amber-400/70 bg-amber-500/5' : 'text-sky-600/80 dark:text-sky-400/70 bg-sky-500/5'}`}>
                            基准<span className="ml-1 normal-case text-[9px] opacity-75">({baselineLabel})</span>
                        </div>
                        <div className="px-2 py-2 text-[10px] font-semibold text-zinc-500/80 dark:text-white/45 uppercase tracking-wide text-right border-l bg-zinc-50/80 dark:bg-[#0f1116] dark:border-white/10">
                            当前 <span className="normal-case text-[9px] opacity-60">{currentAtText}</span>
                        </div>
                    </div>

                    {/* 数据行 */}
                    <div className="text-[11px]">
                        {[
                            { key: 'fee_pct', label: '手续费率', fmt: (v) => formatPct(v) },
                            { key: 'fee_rate_5m_pct', label: '5m费用率', fmt: (v) => formatPct(v, 4) },
                            { key: 'fees_5m', label: '5m费用', fmt: (v) => formatUsd(v) },
                            { key: 'volume_5m', label: '5m交易量', fmt: (v) => formatUsd(v) },
                            { key: 'tvl', label: 'TVL', fmt: (v) => formatUsd(v) },
                            { key: 'tx_5m', label: '5m Tx', fmt: (v) => (Number.isFinite(Number(v)) ? v : '--') },
                            { key: 'price', label: '价格', fmt: (v) => formatNum(v, 8) },
                        ].map(({ key, label, fmt }, idx) => {
                            const baseVal = hasBaselineSnapshot ? baselineData?.[key] : null;
                            const curVal = hasCurrentSnapshot ? current?.[key] : null;
                            const pct = (() => {
                                const c = Number(curVal);
                                const b = Number(baseVal);
                                if (!Number.isFinite(c) || !Number.isFinite(b) || b === 0) return null;
                                const p = ((c - b) / Math.abs(b)) * 100;
                                if (Math.abs(p) < 1) return null;
                                return p;
                            })();
                            return (
                                <div key={key} className={`grid grid-cols-3 border-b border-zinc-100/60 dark:border-white/4 last:border-0 ${idx % 2 === 0 ? '' : 'bg-zinc-50/40 dark:bg-white/[0.015]'}`}>
                                    <div className="px-3 py-1.5 text-zinc-500 dark:text-white/40">{label}</div>
                                    <div className={`px-2 py-1.5 text-right tabular-nums border-l dark:border-white/4 ${isPeakBaseline ? 'bg-amber-500/[0.04]' : 'bg-sky-500/[0.04]'}`}>
                                        {baseVal !== null ? fmt(baseVal) : '--'}
                                    </div>
                                    <div className="px-2 py-1.5 text-right tabular-nums border-l dark:border-white/4 flex items-center justify-end gap-0.5">
                                        <span>{curVal !== null ? fmt(curVal) : '--'}</span>
                                        {pct !== null && (
                                            <span className={`text-[9px] font-bold ${pct > 0 ? 'text-emerald-500' : 'text-red-500'}`}>
                                                {pct > 0 ? '↑' : '↓'}{Math.abs(pct).toFixed(0)}%
                                            </span>
                                        )}
                                    </div>
                                </div>
                            );
                        })}
                    </div>
                </div>

                {/* ── 撤退卫士状态 ── */}
                <div className="mt-2.5 rounded-xl border border-zinc-100/80 bg-zinc-50/80 p-3 dark:border-white/10 dark:bg-[#0f1116]">
                    <div className="flex flex-wrap items-center gap-1.5">
                        <div className="text-[11px] font-semibold text-zinc-600 dark:text-white/60 mr-0.5">撤退卫士</div>
                        <GuardBadge tone={volBadge.tone}>交易量: {volBadge.text}</GuardBadge>
                        <GuardBadge tone={ptBadge.tone}>价+Tx: {ptBadge.text}</GuardBadge>
                    </div>

                    {/* 连续跌破/涨破计数 */}
                    <div className="mt-2 flex flex-wrap items-center gap-3 text-[11px]">
                        <div className="flex items-center gap-1">
                            <span className="text-zinc-400 dark:text-white/35">连续跌破:</span>
                            <span className={`font-bold tabular-nums ${task?.range_break_down_streak >= 2 ? 'text-red-500' : task?.range_break_down_streak >= 1 ? 'text-amber-500' : 'text-zinc-600 dark:text-white/60'}`}>
                                {task?.range_break_down_streak || 0}<span className="opacity-50">/2</span>
                            </span>
                        </div>
                        <div className="flex items-center gap-1">
                            <span className="text-zinc-400 dark:text-white/35">连续涨破:</span>
                            <span className={`font-bold tabular-nums ${task?.range_break_up_streak >= 2 ? 'text-amber-500' : 'text-zinc-600 dark:text-white/60'}`}>
                                {task?.range_break_up_streak || 0}<span className="opacity-50">/2</span>
                            </span>
                        </div>
                        {task?.next_range_multiplier > 1 && (
                            <div className="flex items-center gap-1">
                                <span className="text-zinc-400 dark:text-white/35">下次扩大:</span>
                                <span className="font-bold text-amber-500">{task.next_range_multiplier}x</span>
                            </div>
                        )}
                    </div>

                    {/* 详细条件 */}
                    <div className="mt-2.5 grid grid-cols-2 gap-2">
                        {/* 交易量撤退 */}
                        <div className="rounded-lg border border-zinc-200/60 bg-white/80 p-2.5 dark:border-white/6 dark:bg-white/4">
                            <div className="text-[11px] font-semibold text-zinc-700 dark:text-white/70 mb-1.5">交易量撤退</div>
                            <div className="grid grid-cols-2 gap-x-2 gap-y-1 text-[10px]">
                                <div className="text-zinc-400 dark:text-white/35">阈值跌幅</div>
                                <div className="text-right font-semibold tabular-nums">{formatPct(Number(gv?.drop_pct || 0) * 100, 0)}</div>
                                <div className="text-zinc-400 dark:text-white/35">当前/阈值</div>
                                <div className="text-right font-semibold tabular-nums text-[9px] leading-snug">
                                    {formatUsd(gv?.current_volume_5m)}<br />
                                    <span className="opacity-60">/ {formatUsd(gv?.threshold)}</span>
                                </div>
                                {[
                                    { label: '命中阈值', hit: gv?.hit },
                                    { label: '首次标记', hit: gv?.first_mark },
                                    { label: '已上膛', hit: gv?.armed },
                                    { label: '连续下降', hit: gv?.should_exit_now },
                                ].map(({ label, hit }) => (
                                    <React.Fragment key={label}>
                                        <div className="text-zinc-400 dark:text-white/35">{label}</div>
                                        <div className="text-right"><ConditionStatus hit={hit} /></div>
                                    </React.Fragment>
                                ))}
                            </div>
                            {(gv?.blocked_reason || gv?.skip_reason) ? (
                                <div className="mt-1.5 text-[10px] text-zinc-400 dark:text-white/35 border-t border-zinc-100 dark:border-white/6 pt-1.5">
                                    {zhReason(gv?.blocked_reason) || zhReason(gv?.skip_reason)}
                                </div>
                            ) : null}
                        </div>

                        {/* 价格 + Tx 撤退 */}
                        <div className="rounded-lg border border-zinc-200/60 bg-white/80 p-2.5 dark:border-white/6 dark:bg-white/4">
                            <div className="text-[11px] font-semibold text-zinc-700 dark:text-white/70 mb-1.5">价格+Tx撤退</div>
                            <div className="grid grid-cols-2 gap-x-2 gap-y-1 text-[10px]">
                                <div className="text-zinc-400 dark:text-white/35">价格阈值</div>
                                <div className="text-right font-semibold tabular-nums">{formatPct(Number(gp?.price_drop_pct || gp?.drop_pct || 0) * 100, 0)}</div>
                                <div className="text-zinc-400 dark:text-white/35">Tx阈值</div>
                                <div className="text-right font-semibold tabular-nums">{formatPct(Number(gp?.tx_drop_pct || gp?.drop_pct || 0) * 100, 0)}</div>
                                {[
                                    { label: '价格命中', hit: gp?.price_hit },
                                    { label: 'Tx命中', hit: gp?.tx_hit },
                                    { label: '同时命中', hit: gp?.hit },
                                    { label: '首次标记', hit: gp?.first_mark },
                                    { label: '已上膛', hit: gp?.armed },
                                    { label: '满足撤退', hit: gp?.should_exit_now },
                                ].map(({ label, hit }) => (
                                    <React.Fragment key={label}>
                                        <div className="text-zinc-400 dark:text-white/35">{label}</div>
                                        <div className="text-right"><ConditionStatus hit={hit} /></div>
                                    </React.Fragment>
                                ))}
                            </div>
                            {gp?.blocked_reason ? (
                                <div className="mt-1.5 text-[10px] text-zinc-400 dark:text-white/35 border-t border-zinc-100 dark:border-white/6 pt-1.5">
                                    {zhReason(gp?.blocked_reason)}
                                </div>
                            ) : null}
                        </div>
                    </div>
                </div>
            </div>
        </div>
    );
}
