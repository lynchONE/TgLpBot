import { AlertTriangle, CheckCircle, XCircle } from 'lucide-react';

import { TASK_MODE_OPTIONS } from '../../../lib/taskModes';
import { formatDCAIntervalHint } from '../format';

export function OpenPositionOrderSummary({
    pool,
    amount,
    priceRange,
}) {
    return (
        <div className="rounded-2xl border border-zinc-200/60 bg-zinc-50/60 p-3 dark:border-white/10 dark:bg-white/5">
            <div className="mb-2 text-xs font-semibold text-zinc-500 dark:text-white/50">本单概览</div>
            <div className="space-y-2">
                <div className="flex items-center justify-between gap-3">
                    <span className="text-[13px] text-zinc-500 dark:text-white/45">交易对</span>
                    <span className="text-[13px] font-semibold text-zinc-900 dark:text-white/90">{pool?.trading_pair || '--'}</span>
                </div>
                <div className="flex items-center justify-between gap-3">
                    <span className="text-[13px] text-zinc-500 dark:text-white/45">投入金额</span>
                    <span className="text-[13px] font-semibold tabular-nums text-zinc-900 dark:text-white/90">{amount ? `${amount} USDT` : '--'}</span>
                </div>
                <div className="flex items-center justify-between gap-3">
                    <span className="text-[13px] text-zinc-500 dark:text-white/45">价格区间</span>
                    <span className="text-[13px] font-semibold tabular-nums text-zinc-900 dark:text-white/90">{priceRange ? `${priceRange.lowerText} ~ ${priceRange.upperText}` : '--'}</span>
                </div>
                <div className="flex items-center justify-between gap-3">
                    <span className="text-[13px] text-zinc-500 dark:text-white/45">区间偏移</span>
                    <span className="text-[13px] font-semibold tabular-nums text-zinc-900 dark:text-white/90">{priceRange ? `${priceRange.lowerPctText} / ${priceRange.upperPctText}` : '--'}</span>
                </div>
                <div className="flex items-center justify-between gap-3">
                    <span className="text-[13px] text-zinc-500 dark:text-white/45">区间覆盖</span>
                    <span className="text-[13px] font-semibold tabular-nums text-zinc-900 dark:text-white/90">{priceRange ? `${priceRange.gridCountText} / ${priceRange.widthPctText}` : '--'}</span>
                </div>
            </div>
        </div>
    );
}

export function OpenPositionTaskModePanel({
    taskMode,
    outOfRangeActions,
    loading,
    brand,
    onChangeTaskMode,
}) {
    return (
        <div className="rounded-2xl border border-zinc-200/60 bg-zinc-50/60 p-3 dark:border-white/10 dark:bg-white/5">
            <div className="flex items-center justify-between gap-3">
                <div className="text-xs font-semibold text-zinc-900 dark:text-white/80">本次开仓</div>
                <div className="text-[10px] text-zinc-500 dark:text-white/45 truncate max-w-[200px]">上破:{outOfRangeActions.above} 下破:{outOfRangeActions.below}</div>
            </div>
            <div className="mt-3 grid grid-cols-4 gap-1.5">
                {TASK_MODE_OPTIONS.map((option) => (
                    <button
                        key={option.value}
                        type="button"
                        onClick={() => onChangeTaskMode(option.value)}
                        disabled={loading}
                        title={option.description}
                        className={`mini-open-position-task-option min-h-10 min-w-0 rounded-xl px-1.5 py-1.5 text-center transition ${taskMode === option.value
                            ? `${brand.solidButtonClass} mini-open-position-task-active shadow-sm`
                            : 'border border-zinc-200/50 bg-white/70 text-zinc-700 hover:bg-zinc-100 dark:border-white/10 dark:bg-white/5 dark:text-white/75 dark:hover:bg-white/10'
                            }`}
                    >
                        <div className="truncate text-[10px] font-bold leading-4">{option.shortLabel}</div>
                    </button>
                ))}
            </div>
        </div>
    );
}

export function OpenPositionDCAPanel({
    enabled,
    expanded,
    loading,
    singleSided,
    summaryItems,
    interval,
    onToggleEnabled,
    onToggleExpanded,
    children,
}) {
    return (
        <div className="rounded-2xl border border-zinc-200/60 bg-zinc-50/60 p-3 dark:border-white/10 dark:bg-white/5">
            <div className="flex items-center justify-between gap-3">
                <div className="flex items-center gap-1.5 text-xs font-semibold text-zinc-900 dark:text-white/80">
                    分批开仓
                </div>
                <div className="flex items-center gap-2">
                    <span className="text-[11px] font-medium text-zinc-500 dark:text-white/45">{singleSided ? '单边不支持' : (enabled ? '已启用' : '已关闭')}</span>
                    <button
                        type="button"
                        role="switch"
                        aria-checked={enabled}
                        onClick={onToggleEnabled}
                        disabled={loading || singleSided}
                        className={`mini-open-position-dca-switch relative inline-flex h-6 w-11 shrink-0 items-center rounded-full transition disabled:cursor-not-allowed disabled:opacity-40 ${enabled ? 'is-enabled' : 'bg-zinc-200 dark:bg-white/15'}`}
                    >
                        <span className={`inline-block h-5 w-5 transform rounded-full shadow transition ${enabled ? 'translate-x-[22px]' : 'translate-x-0.5 bg-white'}`} />
                    </button>
                </div>
            </div>
            <button
                type="button"
                onClick={onToggleExpanded}
                disabled={loading}
                className="mt-3 flex w-full items-center gap-2 rounded-xl border border-zinc-200/50 bg-white/70 px-3 py-2 text-left transition hover:bg-white dark:border-white/10 dark:bg-white/5 dark:hover:bg-white/10"
            >
                <div className="flex min-w-0 flex-1 items-center gap-1.5 overflow-x-auto whitespace-nowrap" style={{ scrollbarWidth: 'none' }}>
                    {enabled ? (
                        <>
                            {summaryItems.map((item) => (
                                <span
                                    key={item.key}
                                    className="inline-flex items-center gap-1 rounded-full border border-zinc-200/50 bg-zinc-50 px-2 py-1 text-[10px] font-semibold text-zinc-700 dark:border-white/10 dark:bg-[#14171c]/50 dark:text-white/70"
                                >
                                    <span className="opacity-70">{item.label}</span>
                                    <span>{item.amount}</span>
                                </span>
                            ))}
                            <span className="inline-flex items-center rounded-full border border-zinc-200/50 bg-zinc-50 px-2 py-1 text-[10px] font-bold text-zinc-700 dark:border-white/10 dark:bg-[#14171c]/50 dark:text-white/70">
                                间隔 {formatDCAIntervalHint(interval)}
                            </span>
                        </>
                    ) : (
                        <span className="text-[11px] text-zinc-500 dark:text-white/45">
                            减少单次成交市场冲击
                        </span>
                    )}
                </div>
                <span className="shrink-0 text-[10px] font-medium text-zinc-500 dark:text-white/40">
                    {expanded ? '收起' : '展开'}
                </span>
            </button>
            {expanded ? children : null}
        </div>
    );
}

export function OpenPositionPrecheckPanel({
    loading,
    displayChecks,
    error,
}) {
    if (!loading && displayChecks.length === 0 && !error) return null;

    return (
        <div className="mt-4">
            <div className="mb-2 text-xs font-semibold text-zinc-900 dark:text-white/80">开仓前检查</div>
            {loading ? (
                <div className="text-[11px] text-zinc-500 dark:text-white/40">正在更新预检结果...</div>
            ) : null}
            {error ? (
                <div className="mt-1 rounded-lg border border-red-500/30 bg-red-500/10 p-2 text-[11px] text-red-700 dark:text-red-200">
                    {error}
                </div>
            ) : null}
            {displayChecks.length > 0 ? (
                <div className="space-y-2">
                    {displayChecks.map((item) => {
                        const isWarn = item.status === 'warn';
                        const isFail = item.status === 'fail';
                        return (
                            <div key={item.key} className="rounded-lg p-2 " style={{
                                background: isFail ? 'rgba(239,68,68,0.07)' : isWarn ? 'rgba(234,179,8,0.07)' : 'rgba(34,197,94,0.07)'
                            }}>
                                <div className="flex items-start gap-2">
                                    <div className={`mt-0.5 shrink-0 ${isFail ? 'text-red-500' : isWarn ? 'text-amber-500' : 'text-emerald-500'}`}>
                                        {isFail ? <XCircle className="h-4 w-4" /> : isWarn ? <AlertTriangle className="h-4 w-4" /> : <CheckCircle className="h-4 w-4" />}
                                    </div>
                                    <div className="flex-1 min-w-0">
                                        <div className="flex items-center justify-between gap-2">
                                            <span className={`text-[11px] font-semibold ${isFail ? 'text-red-700 dark:text-red-300' : isWarn ? 'text-amber-700 dark:text-amber-300' : 'text-emerald-700 dark:text-emerald-300'}`}>{item.label}</span>
                                            {item.detail ? <span className="text-[10px] text-zinc-500 dark:text-white/40 text-right">{item.detail}</span> : null}
                                        </div>
                                        {isWarn ? (
                                            <div className="mt-2 text-[11px] leading-tight opacity-80">建议先确认价格、滑点和兑换路径，再决定是否继续开仓。</div>
                                        ) : null}
                                    </div>
                                </div>
                            </div>
                        );
                    })}
                </div>
            ) : null}
        </div>
    );
}

export function OpenPositionEntrySwapPreviewPanel({
    loading,
    preview,
    slippage,
    brand,
    onSlippageChange,
}) {
    if (!loading && !preview?.required) return null;

    return (
        <div className="rounded-xl border border-amber-400/30 bg-gradient-to-r from-amber-500/10 via-amber-500/5 to-transparent px-3 py-2 dark:border-amber-400/25 dark:from-amber-400/10 dark:via-amber-400/5">
            {loading ? (
                <div className="flex items-center gap-2 text-[11px] text-amber-700 dark:text-amber-200">
                    <svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.6" strokeLinecap="round" className="animate-spin"><path d="M21 12a9 9 0 1 1-2.64-6.36"/></svg>
                    正在获取前置兑换预览...
                </div>
            ) : preview?.required ? (
                <div>
                    <div className="flex items-center justify-between gap-3">
                        <div className="flex min-w-0 items-center gap-1.5">
                            <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.4" strokeLinecap="round" strokeLinejoin="round" className="shrink-0 text-amber-600 dark:text-amber-300"><path d="M7 17l5-5-5-5M13 17l5-5-5-5"/></svg>
                            <span className="text-[11px] font-bold text-amber-700 dark:text-amber-200">需要前置兑换</span>
                            <span className="truncate text-[11px] text-zinc-600 dark:text-white/60">
                                {preview?.amount_in || '--'} {preview?.from_token_symbol || ''} → <span className="font-semibold text-zinc-900 dark:text-white/90">{preview?.expected_amount_out || '--'} {preview?.to_token_symbol || ''}</span>
                            </span>
                        </div>
                        <span className="shrink-0 rounded-full border border-amber-500/30 bg-amber-500/15 px-1.5 py-0.5 text-[10px] font-semibold text-amber-700 dark:border-amber-400/30 dark:bg-amber-400/15 dark:text-amber-200">
                            建议滑点 {Number(preview?.recommended_slippage_tolerance).toFixed(2).replace(/0+$/, '').replace(/\.$/, '')}%
                        </span>
                    </div>
                    <div className="mt-2 flex items-center justify-between gap-3">
                        <span className="text-[11px] font-semibold text-amber-700 dark:text-amber-200">本次前置兑换滑点</span>
                        <label className="relative w-28">
                            <input
                                type="number"
                                value={slippage ?? ''}
                                onChange={(event) => onSlippageChange?.(event.target.value)}
                                min="0"
                                step="0.1"
                                inputMode="decimal"
                                className={`h-8 w-full rounded-xl border border-amber-500/25 bg-white/70 py-1 pl-3 pr-7 text-right text-xs font-semibold tabular-nums text-zinc-900 shadow-sm outline-none ring-0 ${brand.inputFocusClass} dark:border-amber-400/20 dark:bg-white/5 dark:text-white/90`}
                                placeholder="0.1"
                            />
                            <span className="pointer-events-none absolute right-2 top-1/2 -translate-y-1/2 text-[10px] font-semibold text-zinc-500 dark:text-white/45">%</span>
                        </label>
                    </div>
                </div>
            ) : null}
        </div>
    );
}
