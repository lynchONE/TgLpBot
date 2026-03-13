import React, { useEffect, useMemo, useState } from 'react';

function StatusIcon({ tone }) {
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

    return (
        <span className="inline-flex h-14 w-14 items-center justify-center rounded-full bg-gradient-to-br from-blue-500 to-teal-600 text-white shadow-[0_12px_32px_rgba(59,130,246,0.28)]">
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

function resolveView(operation, progress) {
    const tone = progress?.status === 'error' ? 'error' : progress?.status === 'done' ? 'done' : 'active';
    const currentStep = Number(progress?.currentStep || 0);
    const error = String(progress?.error || '').trim();

    if (operation === 'open_position') {
        if (tone === 'done') {
            return {
                tone,
                panelTitle: '开仓状态',
                badge: '已完成',
                headline: '开仓成功',
                summary: '新仓位已经创建完成。',
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
                detail: '请检查参数、钱包余额和链上状态后重试。',
            };
        }

        return {
            tone,
            panelTitle: '开仓状态',
            badge: '处理中',
            headline: '开仓请求已提交',
            summary: '系统正在校验参数并创建仓位。',
            detail: '处理完成前请勿重复提交相同请求。',
        };
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

export default function StepProgressModal({ operation, progress, onClose }) {
    if (!operation) return null;

    const [allowClose, setAllowClose] = useState(false);
    const view = useMemo(() => resolveView(operation, progress), [operation, progress]);

    useEffect(() => {
        const timer = setTimeout(() => setAllowClose(true), 10000);
        return () => clearTimeout(timer);
    }, []);

    const isActive = view.tone === 'active';
    const canClose = !isActive || allowClose;
    const badgeClass = view.tone === 'done'
        ? 'border border-emerald-500/25 bg-emerald-500/12 text-emerald-600 dark:text-emerald-300'
        : view.tone === 'error'
            ? 'border border-red-500/25 bg-red-500/12 text-red-600 dark:text-red-300'
            : 'border border-blue-500/25 bg-blue-500/12 text-blue-600 dark:text-blue-300';

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
                            <StatusIcon tone={view.tone} />
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
                            <div className="rounded-2xl border border-blue-500/20 bg-blue-500/8 px-4 py-3 text-[12px] leading-5 text-blue-700 dark:text-blue-200">
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
                            <button
                                type="button"
                                onClick={onClose}
                                className="w-full rounded-xl bg-red-500/10 px-4 py-2.5 text-[13px] font-bold text-red-500 transition-all hover:bg-red-500/15 active:scale-[0.98] dark:text-red-400"
                            >
                                关闭
                            </button>
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
                                <span className="h-1.5 w-1.5 rounded-full bg-emerald-500 animate-pulse" />
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
