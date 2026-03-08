import React, { useEffect, useState } from 'react';

const OPEN_STEPS = [
    '验证权限与配置',
    '查询池子信息',
    '计算区间范围',
    '创建任务记录',
    '执行链上交易',
];

const CLOSE_STEPS = [
    '提交停止请求',
    '撤出流动性',
    '兑换为 USDT',
    '完成',
];

const STEP_ANIM_MS = 300;

const CheckIcon = () => (
    <svg className="h-3 w-3 text-white" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={3}>
        <path strokeLinecap="round" strokeLinejoin="round" d="M5 13l4 4L19 7" />
    </svg>
);

const XIcon = () => (
    <svg className="h-3 w-3 text-white" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={3}>
        <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" />
    </svg>
);

function StepIcon({ status }) {
    if (status === 'done') {
        return (
            <div className="h-5 w-5 rounded-full bg-emerald-500 flex items-center justify-center shadow-sm shadow-emerald-500/30 transition-all duration-300">
                <CheckIcon />
            </div>
        );
    }
    if (status === 'active') {
        return (
            <div className="h-5 w-5 rounded-full border-2 border-emerald-500 flex items-center justify-center transition-all duration-300">
                <div className="h-2 w-2 rounded-full bg-emerald-500 animate-pulse" />
            </div>
        );
    }
    if (status === 'error') {
        return (
            <div className="h-5 w-5 rounded-full bg-red-500 flex items-center justify-center shadow-sm shadow-red-500/30 transition-all duration-300">
                <XIcon />
            </div>
        );
    }
    return <div className="h-5 w-5 rounded-full border-2 border-zinc-300 dark:border-white/20 transition-all duration-300" />;
}

function stepColor(status) {
    if (status === 'done') return 'text-emerald-600 dark:text-emerald-400';
    if (status === 'active') return 'text-zinc-900 dark:text-white/90';
    if (status === 'error') return 'text-red-600 dark:text-red-400';
    return 'text-zinc-400 dark:text-white/30';
}

export default function StepProgressModal({ operation, progress, onClose }) {
    if (!operation) return null;

    const steps = operation === 'open_position' ? OPEN_STEPS : CLOSE_STEPS;
    const targetStep = progress?.currentStep ?? 0;
    const targetStatus = progress?.status ?? 'active';
    const error = progress?.error || '';

    // Internal animated state
    const [displayStep, setDisplayStep] = useState(0);
    const [displayStatus, setDisplayStatus] = useState('active');
    // Fallback: allow closing after 10s even if not done
    const [allowClose, setAllowClose] = useState(false);

    useEffect(() => {
        const timer = setTimeout(() => setAllowClose(true), 10000);
        return () => clearTimeout(timer);
    }, []);

    // Animate step-by-step toward targetStep
    useEffect(() => {
        // Error: jump immediately
        if (targetStatus === 'error') {
            setDisplayStep(targetStep);
            setDisplayStatus('error');
            return;
        }

        // Need to advance one step
        if (displayStep < targetStep) {
            const timer = setTimeout(() => {
                setDisplayStep(prev => prev + 1);
            }, STEP_ANIM_MS);
            return () => clearTimeout(timer);
        }
    }, [displayStep, targetStep, targetStatus]);

    // Sync status once we've caught up
    useEffect(() => {
        if (targetStatus === 'error') return; // handled above
        if (displayStep >= targetStep) {
            setDisplayStatus(targetStatus);
        }
    }, [displayStep, targetStep, targetStatus]);

    const isDone = displayStatus === 'done' && displayStep >= targetStep;
    const isError = displayStatus === 'error';
    const canClose = isDone || isError || allowClose;
    const title = operation === 'open_position' ? '开仓进度' : '撤仓进度';

    return (
        <div
            className="fixed inset-0 z-[200] flex items-end sm:items-center justify-center bg-black/60 backdrop-blur-sm"
            onClick={canClose ? onClose : undefined}
        >
            <div
                className="w-full sm:max-w-sm rounded-t-2xl sm:rounded-2xl border border-zinc-200/80 bg-white dark:border-white/10 dark:bg-[#131518] shadow-2xl p-5 pb-[max(1.25rem,env(safe-area-inset-bottom))]"
                onClick={(e) => e.stopPropagation()}
            >
                <div className="flex items-center justify-between mb-4">
                    <h3 className="text-base font-bold text-zinc-900 dark:text-white/90">{title}</h3>
                    {canClose && (
                        <button
                            type="button"
                            onClick={onClose}
                            className="h-7 w-7 flex items-center justify-center rounded-lg text-zinc-400 hover:text-zinc-600 hover:bg-zinc-100 dark:text-white/40 dark:hover:text-white/70 dark:hover:bg-white/10 transition-colors text-lg leading-none"
                        >
                            &times;
                        </button>
                    )}
                </div>

                <div className="space-y-3">
                    {steps.map((label, i) => {
                        let stepStatus;
                        if (i < displayStep) stepStatus = 'done';
                        else if (i === displayStep) stepStatus = displayStatus;
                        else stepStatus = 'pending';

                        return (
                            <div key={i} className="flex items-start gap-3">
                                <div className="mt-0.5 shrink-0">
                                    <StepIcon status={stepStatus} />
                                </div>
                                <div className="flex-1 min-w-0">
                                    <div className={`text-sm font-medium transition-colors duration-300 ${stepColor(stepStatus)}`}>
                                        {label}
                                    </div>
                                    {stepStatus === 'error' && error && (
                                        <div className="mt-1 text-xs text-red-500 dark:text-red-400 break-all leading-relaxed">
                                            {error}
                                        </div>
                                    )}
                                </div>
                            </div>
                        );
                    })}
                </div>

                <div className="mt-4 pt-3 border-t border-zinc-100 dark:border-white/10">
                    {isDone ? (
                        <button
                            type="button"
                            onClick={onClose}
                            className="w-full rounded-xl bg-emerald-500 px-4 py-2.5 text-sm font-semibold text-white shadow-sm hover:bg-emerald-600 active:bg-emerald-700 transition-colors"
                        >
                            完成
                        </button>
                    ) : isError ? (
                        <button
                            type="button"
                            onClick={onClose}
                            className="w-full rounded-xl bg-red-500/10 px-4 py-2.5 text-sm font-semibold text-red-600 dark:text-red-400 hover:bg-red-500/20 transition-colors"
                        >
                            关闭
                        </button>
                    ) : allowClose ? (
                        <button
                            type="button"
                            onClick={onClose}
                            className="w-full rounded-xl border border-zinc-200 dark:border-white/10 px-4 py-2.5 text-sm font-semibold text-zinc-500 dark:text-white/50 hover:bg-zinc-100 dark:hover:bg-white/5 transition-colors"
                        >
                            后台继续...
                        </button>
                    ) : (
                        <div className="text-center text-xs text-zinc-400 dark:text-white/40">
                            请勿关闭此页面...
                        </div>
                    )}
                </div>
            </div>
        </div>
    );
}
