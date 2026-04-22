import React, { useEffect, useMemo, useState } from 'react';
import { getBrandTheme } from '../lib/brand';

function StatusIcon({ tone, brand }) {
    if (tone === 'done') {
        return (
            <span className="inline-flex h-14 w-14 items-center justify-center rounded-full bg-gradient-to-br from-emerald-500 to-emerald-600 text-white shadow-[0_12px_32px_rgba(16,185,129,0.28)]">
                <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.6" strokeLinecap="round" strokeLinejoin="round">
                    <path d="M5 13l4 4L19 7" />
                </svg>
            </span>
        );
    }

    if (tone === 'error') {
        return (
            <span className="inline-flex h-14 w-14 items-center justify-center rounded-full bg-gradient-to-br from-red-500 to-orange-500 text-white shadow-[0_12px_32px_rgba(239,68,68,0.28)]">
                <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.6" strokeLinecap="round" strokeLinejoin="round">
                    <path d="M6 6l12 12M18 6 6 18" />
                </svg>
            </span>
        );
    }

    const activeIconClass = brand?.key === 'emerald'
        ? 'bg-gradient-to-br from-emerald-500 to-teal-600 text-white shadow-[0_12px_32px_rgba(16,185,129,0.28)]'
        : 'bg-gradient-to-br from-[#bcff2f] to-[#8fda21] text-[#182108] shadow-[0_12px_32px_rgba(188,255,47,0.22)]';

    return (
        <span className={`inline-flex h-14 w-14 items-center justify-center rounded-full ${activeIconClass}`}>
            <svg
                width="20"
                height="20"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                strokeWidth="2.4"
                strokeLinecap="round"
                strokeLinejoin="round"
                style={{ animation: 'spm-status-spin 1s linear infinite' }}
            >
                <path d="M21 12a9 9 0 1 1-2.64-6.36" />
            </svg>
        </span>
    );
}

function CompactStatusIcon({ tone, brand }) {
    if (tone === 'done') {
        return (
            <span className="inline-flex h-9 w-9 shrink-0 items-center justify-center rounded-full bg-gradient-to-br from-emerald-500 to-emerald-600 text-white shadow-[0_8px_20px_rgba(16,185,129,0.22)]">
                <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.6" strokeLinecap="round" strokeLinejoin="round">
                    <path d="M5 13l4 4L19 7" />
                </svg>
            </span>
        );
    }

    if (tone === 'error') {
        return (
            <span className="inline-flex h-9 w-9 shrink-0 items-center justify-center rounded-full bg-gradient-to-br from-red-500 to-orange-500 text-white shadow-[0_8px_20px_rgba(239,68,68,0.22)]">
                <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.6" strokeLinecap="round" strokeLinejoin="round">
                    <path d="M6 6l12 12M18 6 6 18" />
                </svg>
            </span>
        );
    }

    const activeIconClass = brand?.key === 'emerald'
        ? 'bg-gradient-to-br from-emerald-500 to-teal-600 text-white shadow-[0_8px_20px_rgba(16,185,129,0.22)]'
        : 'bg-gradient-to-br from-[#bcff2f] to-[#8fda21] text-[#182108] shadow-[0_8px_20px_rgba(188,255,47,0.18)]';

    return (
        <span className={`inline-flex h-9 w-9 shrink-0 items-center justify-center rounded-full ${activeIconClass}`}>
            <svg
                width="14"
                height="14"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                strokeWidth="2.4"
                strokeLinecap="round"
                strokeLinejoin="round"
                style={{ animation: 'spm-status-spin 1s linear infinite' }}
            >
                <path d="M21 12a9 9 0 1 1-2.64-6.36" />
            </svg>
        </span>
    );
}

function resolveOpenPositionView(tone, error, progress) {
    const isDCA = Boolean(progress?.dca);
    const pair = String(progress?.pair || '').trim();
    const currentStep = Number(progress?.currentStep || 0);
    const totalSteps = Number(progress?.totalSteps || 0);

    if (tone === 'done') {
        return {
            tone,
            panelTitle: '开仓状态',
            badge: '已完成',
            headline: '开仓完成',
            summary: pair ? `${pair} 仓位已创建。` : '仓位已创建完成。',
            detail: '持仓列表刷新后会显示最新结果。',
        };
    }

    if (tone === 'error') {
        return {
            tone,
            panelTitle: '开仓状态',
            badge: '失败',
            headline: '开仓失败',
            summary: error || '开仓请求执行失败。',
            detail: '如果这是首次钱包开仓，系统会在下次重试时继续完成“部署私有合约 -> 绑定钱包 -> 开仓”，不会重复部署新的私有合约。',
        };
    }

    // active_dca = first batch done, later batches run in the background ticker
    if (progress?.status === 'active_dca' && isDCA && totalSteps > 1) {
        return {
            tone: 'active_dca',
            panelTitle: '开仓状态',
            badge: `批次 1 / ${totalSteps}`,
            headline: '首批开仓完成',
            summary: pair ? `${pair} 首批已成交，剩余 ${totalSteps - 1} 批将按间隔后台执行。` : `剩余 ${totalSteps - 1} 批将按间隔后台执行。`,
            detail: '你可以关闭此提示，继续其他操作。',
        };
    }

    if (isDCA && totalSteps > 1) {
        return {
            tone,
            panelTitle: '开仓状态',
            badge: '处理中',
            headline: '首批开仓处理中',
            summary: pair ? `${pair} · 首批 1 / ${totalSteps} 正在提交。` : `首批 1 / ${totalSteps} 正在提交。`,
            detail: '处理完成前请勿重复提交。你可以关闭此提示继续操作页面，任务会在后台执行。',
        };
    }

    return {
        tone,
        panelTitle: '开仓状态',
        badge: '处理中',
        headline: '正在处理开仓流程',
        summary: pair ? `${pair} · 系统正在检查私有合约并创建仓位。` : '系统正在检查当前钱包的私有合约绑定状态。',
        detail: '如果这是当前钱包首次开仓，会先部署私有合约再继续。你可以关闭此提示，任务会在后台执行。',
    };
}

function resolveView(operation, progress) {
    const tone = progress?.status === 'error' ? 'error' : progress?.status === 'done' ? 'done' : 'active';
    const currentStep = Number(progress?.currentStep || 0);
    const error = String(progress?.error || '').trim();

    if (operation === 'open_position') {
        return resolveOpenPositionView(tone, error, progress);
    }

    if (operation === 'open_position') {
        return resolveOpenPositionView(tone, error, progress);
    }

    if (tone === 'done') {
        return {
            tone,
            panelTitle: '撤仓状态',
            badge: '已完成',
            headline: '撤仓完成',
            summary: '仓位已经结束处理。',
            detail: '如果列表里已经看不到该仓位，说明撤仓已完成。',
        };
    }

    if (tone === 'error') {
        return {
            tone,
            panelTitle: '撤仓状态',
            badge: '失败',
            headline: '撤仓失败',
            summary: error || '撤仓请求执行失败。',
            detail: '请检查链上状态后重试，或稍后刷新持仓列表确认结果。',
        };
    }

    if (currentStep > 0) {
        return {
            tone,
            panelTitle: '撤仓状态',
            badge: '后台处理中',
            headline: '撤仓请求已提交',
            summary: '系统正在后台执行撤出流动性和兑换。',
            detail: '你可以关闭弹窗，等列表刷新后再查看最终结果。',
        };
    }

    return {
        tone,
        panelTitle: '撤仓状态',
        badge: '提交中',
        headline: '正在提交撤仓请求',
        summary: '系统正在把撤仓请求发送到后端。',
        detail: '请求接受后会自动转为后台处理状态。',
    };
}

export default function StepProgressModal({ operation, progress, accentTheme = 'lime', onClose, onRetry }) {
    if (!operation) return null;

    const [allowClose, setAllowClose] = useState(false);
    const [retrying, setRetrying] = useState(false);
    const brand = useMemo(() => getBrandTheme(accentTheme), [accentTheme]);
    const view = useMemo(() => resolveView(operation, progress), [operation, progress]);
    const isCompact = operation === 'close_position' || operation === 'open_position';
    const canRetry = view.tone === 'error' && typeof onRetry === 'function';

    useEffect(() => {
        if (view.tone !== 'error') setRetrying(false);
    }, [view.tone, progress?.status]);

    const handleRetry = async () => {
        if (!canRetry || retrying) return;
        setRetrying(true);
        try {
            await onRetry?.();
        } finally {
            setRetrying(false);
        }
    };

    useEffect(() => {
        if (isCompact) {
            setAllowClose(true);
            return undefined;
        }
        setAllowClose(false);
        const timer = setTimeout(() => setAllowClose(true), 10000);
        return () => clearTimeout(timer);
    }, [isCompact]);

    // Auto-dismiss compact toast once the user has had time to read the result.
    // Open-position: first-batch done (active_dca) → 8s; full done → 5s.
    useEffect(() => {
        if (!onClose) return undefined;
        if (!isCompact) return undefined;
        let delay = 0;
        if (view.tone === 'done') delay = 5000;
        else if (view.tone === 'active_dca') delay = 8000;
        if (!delay) return undefined;
        const timer = setTimeout(() => onClose(), delay);
        return () => clearTimeout(timer);
    }, [isCompact, view.tone, onClose]);

    const isActive = view.tone === 'active' || view.tone === 'active_dca';
    const canClose = isCompact ? true : !isActive || allowClose;
    const activeBadgeClass = brand.key === 'emerald'
        ? 'border border-emerald-500/25 bg-emerald-500/12 text-emerald-600 dark:text-emerald-300'
        : 'border border-[#bcff2f]/30 bg-[#bcff2f]/12 text-[#6f9616] dark:text-[#e3ffa0]';
    const activeHintClass = brand.key === 'emerald'
        ? 'rounded-2xl border border-emerald-500/20 bg-emerald-500/8 px-4 py-3 text-[12px] leading-5 text-emerald-700 dark:text-emerald-200'
        : 'rounded-2xl border border-[#bcff2f]/25 bg-[#bcff2f]/8 px-4 py-3 text-[12px] leading-5 text-[#6f9616] dark:text-[#e3ffa0]';
    const activeDotClass = brand.key === 'emerald' ? 'bg-emerald-500' : 'bg-[#bcff2f]';
    const badgeClass = view.tone === 'done'
        ? 'border border-emerald-500/25 bg-emerald-500/12 text-emerald-600 dark:text-emerald-300'
        : view.tone === 'error'
            ? 'border border-red-500/25 bg-red-500/12 text-red-600 dark:text-red-300'
            : activeBadgeClass;
    const toastShellClass = view.tone === 'done'
        ? 'border-emerald-500/25 bg-white/95 dark:border-emerald-400/20 dark:bg-[#0f1318]/95'
        : view.tone === 'error'
            ? 'border-red-500/25 bg-white/95 dark:border-red-400/20 dark:bg-[#0f1318]/95'
            : 'border-zinc-200/80 bg-white/95 dark:border-white/[0.08] dark:bg-[#0f1318]/95';
    const toastBadgeClass = view.tone === 'done'
        ? 'border border-emerald-500/25 bg-emerald-500/12 text-emerald-600 dark:text-emerald-300'
        : view.tone === 'error'
            ? 'border border-red-500/25 bg-red-500/12 text-red-600 dark:text-red-300'
            : activeBadgeClass;
    const toastHintClass = view.tone === 'done'
        ? 'text-emerald-700 dark:text-emerald-300'
        : view.tone === 'error'
            ? 'text-red-600 dark:text-red-300'
            : brand.key === 'emerald'
                ? 'text-emerald-700 dark:text-emerald-200'
                : 'text-[#6f9616] dark:text-[#e3ffa0]';

    if (isCompact) {
        const isOpen = operation === 'open_position';
        const aboutLabel = isOpen ? '开仓' : '撤仓';
        const ariaLabel = isOpen ? '关闭开仓状态' : '关闭撤仓状态';
        const activeHint = isOpen
            ? '开仓处理中，你可以继续操作页面。'
            : '后台继续撤仓中，你可以继续操作页面。';
        const doneHint = isOpen
            ? (Number(progress?.totalSteps || 0) > 1 && progress?.status === 'done'
                ? '分批加仓全部完成。'
                : '开仓已完成。')
            : '撤仓已完成，不会再阻塞当前界面。';
        const errorHint = isOpen
            ? `${aboutLabel}失败，请检查参数或稍后重试。`
            : `${aboutLabel}失败，可稍后重试或刷新列表确认状态。`;

        return (
            <div
                className="pointer-events-none fixed inset-x-0 z-[180] flex justify-center px-3"
                style={{ bottom: 'calc(env(safe-area-inset-bottom) + 5.5rem)' }}
            >
                <div className={`pointer-events-auto w-full max-w-sm rounded-2xl border p-3 shadow-[0_16px_36px_rgba(0,0,0,0.24)] backdrop-blur ${toastShellClass}`}>
                    <div className="flex items-center justify-between gap-3">
                        <span className={`inline-flex items-center rounded-full px-2.5 py-1 text-[11px] font-bold ${toastBadgeClass}`}>
                            {view.badge}
                        </span>
                        <button
                            type="button"
                            onClick={onClose}
                            className="inline-flex h-7 w-7 items-center justify-center rounded-lg bg-zinc-100 text-zinc-400 transition-colors hover:text-zinc-600 dark:bg-white/[0.06] dark:text-white/30 dark:hover:text-white/60"
                            aria-label={ariaLabel}
                        >
                            <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round">
                                <path d="M18 6 6 18M6 6l12 12" />
                            </svg>
                        </button>
                    </div>

                    <div className="mt-3 flex items-start gap-3">
                        <CompactStatusIcon tone={view.tone} brand={brand} />
                        <div className="min-w-0 flex-1">
                            <div className="text-[15px] font-bold leading-5 text-zinc-900 dark:text-white/95">
                                {view.headline}
                            </div>
                            <div className="mt-1 text-[12px] leading-5 text-zinc-700 dark:text-white/80">
                                {view.summary}
                            </div>
                            {progress?.taskId ? (
                                <div className="mt-2 text-[11px] font-semibold text-zinc-500 dark:text-white/45">
                                    任务 #{progress.taskId}
                                </div>
                            ) : null}
                            <div className={`mt-2 flex items-center gap-1.5 text-[11px] leading-5 ${toastHintClass}`}>
                                {isActive ? (
                                    <>
                                        <span className={`h-1.5 w-1.5 rounded-full animate-pulse ${brand.key === 'emerald' ? 'bg-emerald-500' : 'bg-[#bcff2f]'}`} />
                                        {activeHint}
                                    </>
                                ) : view.tone === 'done' ? (
                                    doneHint
                                ) : (
                                    errorHint
                                )}
                            </div>
                        </div>
                    </div>

                    {canRetry ? (
                        <div className="mt-3 grid grid-cols-2 gap-2">
                            <button
                                type="button"
                                onClick={handleRetry}
                                disabled={retrying}
                                className={`rounded-xl px-3 py-2.5 text-[12px] font-bold transition-all active:scale-[0.98] ${
                                    retrying
                                        ? 'cursor-not-allowed bg-zinc-200 text-zinc-500 dark:bg-white/[0.08] dark:text-white/35'
                                        : brand.gradientButtonClass
                                }`}
                            >
                                {retrying ? '重试中...' : '按原设置重试'}
                            </button>
                            <button
                                type="button"
                                onClick={onClose}
                                className="rounded-xl bg-red-500/10 px-3 py-2.5 text-[12px] font-bold text-red-500 transition-all hover:bg-red-500/15 active:scale-[0.98] dark:text-red-400"
                            >
                                关闭
                            </button>
                        </div>
                    ) : null}
                </div>

                <style>{`
                    @keyframes spm-status-spin { from { transform: rotate(0deg) } to { transform: rotate(360deg) } }
                `}</style>
            </div>
        );
    }

    return (
        <div
            className="fixed inset-0 z-[200] flex items-end justify-center bg-black/60 backdrop-blur-sm animate-[spm-fade_200ms_ease]"
            onClick={canClose ? onClose : undefined}
        >
            <div
                className="w-full max-w-md rounded-t-2xl border-t border-x border-zinc-200/60 bg-white shadow-[0_-8px_40px_rgba(0,0,0,0.4)] dark:border-white/[0.06] dark:bg-[#0f1318] animate-[spm-slide-up_320ms_cubic-bezier(.2,.9,.3,1)] pb-[max(1.25rem,env(safe-area-inset-bottom))]"
                onClick={(e) => e.stopPropagation()}
            >
                <div className="flex justify-center pt-2.5 pb-1">
                    <div className="h-1 w-8 rounded-full bg-zinc-300/60 dark:bg-white/10" />
                </div>

                <div className="px-5 pt-1 pb-4">
                    <div className="mb-4 flex items-start justify-between gap-3">
                        <div className="flex min-w-0 flex-col gap-2">
                            <h3 className="text-[15px] font-bold tracking-tight text-zinc-900 dark:text-white/90">{view.panelTitle}</h3>
                            <span className={`inline-flex w-fit items-center rounded-full px-2.5 py-1 text-[11px] font-bold ${badgeClass}`}>
                                {view.badge}
                            </span>
                        </div>

                        {canClose && (
                            <button
                                type="button"
                                onClick={onClose}
                                className="flex h-6 w-6 items-center justify-center rounded-md bg-zinc-100 text-zinc-400 transition-colors hover:text-zinc-600 dark:bg-white/[0.06] dark:text-white/30 dark:hover:text-white/60"
                            >
                                <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round">
                                    <path d="M18 6 6 18M6 6l12 12" />
                                </svg>
                            </button>
                        )}
                    </div>

                    <div className="flex flex-col gap-4 py-1">
                        <div className="flex justify-center">
                            <StatusIcon tone={view.tone} brand={brand} />
                        </div>

                        <div className="flex flex-col gap-2 text-center">
                            <p className="m-0 text-[24px] font-extrabold leading-[1.2] text-zinc-900 dark:text-white/95">
                                {view.headline}
                            </p>
                            <div className="text-[14px] leading-6 text-zinc-700 dark:text-white/80">
                                {view.summary}
                            </div>
                            {view.detail ? (
                                <div className="text-[12px] leading-5 text-zinc-500 dark:text-white/45">
                                    {view.detail}
                                </div>
                            ) : null}
                        </div>

                        {progress?.taskId ? (
                            <div className="mx-auto inline-flex items-center rounded-full border border-zinc-200 bg-zinc-50 px-3 py-1.5 text-[12px] font-semibold text-zinc-600 dark:border-white/[0.08] dark:bg-white/[0.04] dark:text-white/70">
                                任务 #{progress.taskId}
                            </div>
                        ) : null}

                        {isActive && allowClose ? (
                            <div className={activeHintClass}>
                                可以先关闭这个弹窗，任务会在后台继续执行。
                            </div>
                        ) : null}
                    </div>

                    <div className="mt-4 border-t border-zinc-100 pt-3 dark:border-white/[0.05]">
                        {view.tone === 'done' ? (
                            <button
                                type="button"
                                onClick={onClose}
                                className="w-full rounded-xl bg-gradient-to-b from-emerald-500 to-emerald-600 px-4 py-2.5 text-[13px] font-bold text-white shadow-[0_2px_12px_rgba(16,185,129,0.25)] transition-all hover:shadow-[0_4px_18px_rgba(16,185,129,0.35)] active:scale-[0.98]"
                            >
                                完成
                            </button>
                        ) : view.tone === 'error' ? (
                            <div className={`grid gap-2 ${canRetry ? 'grid-cols-2' : 'grid-cols-1'}`}>
                                {canRetry ? (
                                    <button
                                        type="button"
                                        onClick={handleRetry}
                                        disabled={retrying}
                                        className={`rounded-xl px-4 py-2.5 text-[13px] font-bold transition-all active:scale-[0.98] ${
                                            retrying
                                                ? 'cursor-not-allowed bg-zinc-200 text-zinc-500 dark:bg-white/[0.08] dark:text-white/35'
                                                : brand.gradientButtonClass
                                        }`}
                                    >
                                        {retrying ? '重试中...' : '按原设置重试'}
                                    </button>
                                ) : null}
                                <button
                                    type="button"
                                    onClick={onClose}
                                    className="rounded-xl bg-red-500/10 px-4 py-2.5 text-[13px] font-bold text-red-500 transition-all hover:bg-red-500/15 active:scale-[0.98] dark:text-red-400"
                                >
                                    关闭
                                </button>
                            </div>
                        ) : allowClose ? (
                            <button
                                type="button"
                                onClick={onClose}
                                className="w-full rounded-xl border border-zinc-200 px-4 py-2.5 text-[13px] font-bold text-zinc-500 transition-all hover:bg-zinc-50 active:scale-[0.98] dark:border-white/[0.08] dark:text-white/40 dark:hover:bg-white/[0.03]"
                            >
                                后台继续
                            </button>
                        ) : (
                            <div className="flex items-center justify-center gap-1.5 py-1.5 text-[12px] text-zinc-400 dark:text-white/30">
                                <span className={`h-1.5 w-1.5 rounded-full animate-pulse ${activeDotClass}`} />
                                处理中，请稍候...
                            </div>
                        )}
                    </div>
                </div>
            </div>

            <style>{`
                @keyframes spm-fade { from { opacity: 0 } to { opacity: 1 } }
                @keyframes spm-slide-up { from { transform: translateY(100%) } to { transform: translateY(0) } }
                @keyframes spm-status-spin { from { transform: rotate(0deg) } to { transform: rotate(360deg) } }
            `}</style>
        </div>
    );
}
