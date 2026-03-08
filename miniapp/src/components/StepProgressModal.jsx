import React, { useEffect, useState } from 'react';

const OPEN_STEPS = [
    { label: '验证权限与配置', icon: 'shield' },
    { label: '查询池子信息', icon: 'search' },
    { label: '计算区间范围', icon: 'calc' },
    { label: '创建任务记录', icon: 'file' },
    { label: '执行链上交易', icon: 'chain' },
];

const CLOSE_STEPS = [
    { label: '提交停止请求', icon: 'send' },
    { label: '撤出流动性', icon: 'withdraw' },
    { label: '兑换为 USDT', icon: 'swap' },
    { label: '完成', icon: 'check' },
];

const STEP_ANIM_MS = 300;

const StepSvg = ({ type, size = 14 }) => {
    const props = { width: size, height: size, viewBox: '0 0 24 24', fill: 'none', stroke: 'currentColor', strokeWidth: 2, strokeLinecap: 'round', strokeLinejoin: 'round' };
    switch (type) {
        case 'shield': return <svg {...props}><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z" /></svg>;
        case 'search': return <svg {...props}><circle cx="11" cy="11" r="8" /><path d="m21 21-4.3-4.3" /></svg>;
        case 'calc':   return <svg {...props}><rect x="4" y="2" width="16" height="20" rx="2" /><path d="M8 6h8M8 10h8M8 14h4" /></svg>;
        case 'file':   return <svg {...props}><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8Z" /><path d="M14 2v6h6" /></svg>;
        case 'chain':  return <svg {...props}><path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71" /><path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71" /></svg>;
        case 'send':   return <svg {...props}><path d="m22 2-7 20-4-9-9-4Z" /><path d="m22 2-11 11" /></svg>;
        case 'withdraw': return <svg {...props}><path d="M21 12V7H5a2 2 0 0 1 0-4h14v4" /><path d="M3 5v14a2 2 0 0 0 2 2h16v-5" /><path d="M18 12a2 2 0 0 0 0 4h4v-4Z" /></svg>;
        case 'swap':   return <svg {...props}><path d="m16 3 4 4-4 4" /><path d="M20 7H4" /><path d="m8 21-4-4 4-4" /><path d="M4 17h16" /></svg>;
        case 'check':  return <svg {...props}><path d="M22 11.08V12a10 10 0 1 1-5.93-9.14" /><path d="M22 4 12 14.01l-3-3" /></svg>;
        default: return null;
    }
};

function StepIndicator({ status, icon }) {
    if (status === 'done') {
        return (
            <div className="h-5 w-5 rounded-full bg-emerald-500 flex items-center justify-center shadow-[0_0_10px_rgba(16,185,129,0.3)] transition-all duration-300">
                <svg className="w-3 h-3 text-white" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={3.5} strokeLinecap="round" strokeLinejoin="round">
                    <path d="M5 13l4 4L19 7" />
                </svg>
            </div>
        );
    }
    if (status === 'active') {
        return (
            <div className="relative h-5 w-5 rounded-full bg-emerald-500/10 flex items-center justify-center transition-all duration-300">
                <span className="relative z-10 text-emerald-500"><StepSvg type={icon} size={12} /></span>
                <span className="absolute inset-[-2px] rounded-full border-[1.5px] border-emerald-500 animate-[spm-ring_2s_ease-in-out_infinite]" />
            </div>
        );
    }
    if (status === 'error') {
        return (
            <div className="h-5 w-5 rounded-full bg-red-500 flex items-center justify-center shadow-[0_0_10px_rgba(239,68,68,0.3)] transition-all duration-300">
                <svg className="w-3 h-3 text-white" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={3.5} strokeLinecap="round" strokeLinejoin="round">
                    <path d="M6 18L18 6M6 6l12 12" />
                </svg>
            </div>
        );
    }
    return (
        <div className="h-5 w-5 rounded-full border-[1.5px] border-zinc-300/30 dark:border-white/10 flex items-center justify-center text-zinc-300/30 dark:text-white/15 transition-all duration-300">
            <StepSvg type={icon} size={12} />
        </div>
    );
}

export default function StepProgressModal({ operation, progress, onClose }) {
    if (!operation) return null;

    const stepDefs = operation === 'open_position' ? OPEN_STEPS : CLOSE_STEPS;
    const targetStep = progress?.currentStep ?? 0;
    const targetStatus = progress?.status ?? 'active';
    const error = progress?.error || '';

    const [displayStep, setDisplayStep] = useState(0);
    const [displayStatus, setDisplayStatus] = useState('active');
    const [allowClose, setAllowClose] = useState(false);

    useEffect(() => {
        const timer = setTimeout(() => setAllowClose(true), 10000);
        return () => clearTimeout(timer);
    }, []);

    useEffect(() => {
        if (targetStatus === 'error') {
            setDisplayStep(targetStep);
            setDisplayStatus('error');
            return;
        }
        if (displayStep < targetStep) {
            const timer = setTimeout(() => setDisplayStep(prev => prev + 1), STEP_ANIM_MS);
            return () => clearTimeout(timer);
        }
    }, [displayStep, targetStep, targetStatus]);

    useEffect(() => {
        if (targetStatus === 'error') return;
        if (displayStep >= targetStep) setDisplayStatus(targetStatus);
    }, [displayStep, targetStep, targetStatus]);

    const isDone = displayStatus === 'done' && displayStep >= targetStep;
    const isError = displayStatus === 'error';
    const canClose = isDone || isError || allowClose;
    const title = operation === 'open_position' ? '开仓进度' : '撤仓进度';
    const doneCount = stepDefs.filter((_, i) => i < displayStep || (i === displayStep && displayStatus === 'done')).length;
    const pct = (doneCount / stepDefs.length) * 100;

    return (
        <div
            className="fixed inset-0 z-[200] flex items-end justify-center bg-black/60 backdrop-blur-sm animate-[spm-fade_200ms_ease]"
            onClick={canClose ? onClose : undefined}
        >
            <div
                className="w-full max-w-md rounded-t-2xl border-t border-x border-zinc-200/60 dark:border-white/[0.06] bg-white dark:bg-[#0f1318] shadow-[0_-8px_40px_rgba(0,0,0,0.4)] animate-[spm-slide-up_320ms_cubic-bezier(.2,.9,.3,1)] pb-[max(1.25rem,env(safe-area-inset-bottom))]"
                onClick={(e) => e.stopPropagation()}
            >
                {/* Handle bar */}
                <div className="flex justify-center pt-2.5 pb-1">
                    <div className="w-8 h-1 rounded-full bg-zinc-300/60 dark:bg-white/10" />
                </div>

                <div className="px-5 pt-1 pb-4">
                    {/* Header */}
                    <div className="flex items-center justify-between mb-3">
                        <h3 className="text-[15px] font-bold text-zinc-900 dark:text-white/90 tracking-tight">{title}</h3>
                        {canClose && (
                            <button
                                type="button"
                                onClick={onClose}
                                className="h-6 w-6 flex items-center justify-center rounded-md bg-zinc-100 dark:bg-white/[0.06] text-zinc-400 dark:text-white/30 hover:text-zinc-600 dark:hover:text-white/60 transition-colors"
                            >
                                <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round"><path d="M18 6 6 18M6 6l12 12" /></svg>
                            </button>
                        )}
                    </div>

                    {/* Progress bar */}
                    <div className="h-[3px] rounded-full bg-zinc-100 dark:bg-white/[0.05] overflow-hidden mb-4">
                        <div
                            className={`h-full rounded-full transition-all duration-500 ease-out ${isError ? 'bg-gradient-to-r from-red-500 to-red-400 shadow-[0_0_8px_rgba(239,68,68,0.3)]' : 'bg-gradient-to-r from-emerald-500 to-emerald-400 shadow-[0_0_8px_rgba(16,185,129,0.3)]'}`}
                            style={{ width: `${pct}%` }}
                        />
                    </div>

                    {/* Steps */}
                    <div className="flex flex-col">
                        {stepDefs.map((step, i) => {
                            let stepStatus;
                            if (i < displayStep) stepStatus = 'done';
                            else if (i === displayStep) stepStatus = displayStatus;
                            else stepStatus = 'pending';

                            const labelColor = stepStatus === 'done' ? 'text-emerald-600 dark:text-emerald-400'
                                : stepStatus === 'active' ? 'text-zinc-900 dark:text-white/90'
                                : stepStatus === 'error' ? 'text-red-600 dark:text-red-400'
                                : 'text-zinc-300 dark:text-white/20';

                            return (
                                <div key={i} className="relative flex items-start gap-2.5 py-[5px]">
                                    {/* Connector */}
                                    {i < stepDefs.length - 1 && (
                                        <div
                                            className={`absolute left-[9.5px] top-[26px] bottom-[-5px] w-px transition-colors duration-300 ${i < displayStep ? 'bg-emerald-500/25' : 'bg-zinc-200/60 dark:bg-white/[0.05]'}`}
                                        />
                                    )}
                                    <div className="shrink-0 mt-[1px]">
                                        <StepIndicator status={stepStatus} icon={step.icon} />
                                    </div>
                                    <div className="flex-1 min-w-0">
                                        <span className={`text-[13px] font-semibold transition-colors duration-300 ${labelColor}`}>{step.label}</span>
                                        {stepStatus === 'error' && error && (
                                            <div className="mt-1 text-[11px] text-red-500 dark:text-red-400/80 break-all leading-relaxed">{error}</div>
                                        )}
                                    </div>
                                </div>
                            );
                        })}
                    </div>

                    {/* Footer */}
                    <div className="mt-3 pt-3 border-t border-zinc-100 dark:border-white/[0.05]">
                        {isDone ? (
                            <button
                                type="button"
                                onClick={onClose}
                                className="w-full flex items-center justify-center gap-1.5 rounded-xl bg-gradient-to-b from-emerald-500 to-emerald-600 px-4 py-2.5 text-[13px] font-bold text-white shadow-[0_2px_12px_rgba(16,185,129,0.25)] hover:shadow-[0_4px_18px_rgba(16,185,129,0.35)] active:scale-[0.98] transition-all"
                            >
                                <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round"><path d="M5 13l4 4L19 7" /></svg>
                                完成
                            </button>
                        ) : isError ? (
                            <button
                                type="button"
                                onClick={onClose}
                                className="w-full rounded-xl bg-red-500/10 px-4 py-2.5 text-[13px] font-bold text-red-500 dark:text-red-400 hover:bg-red-500/15 active:scale-[0.98] transition-all"
                            >
                                关闭
                            </button>
                        ) : allowClose ? (
                            <button
                                type="button"
                                onClick={onClose}
                                className="w-full rounded-xl border border-zinc-200 dark:border-white/[0.08] px-4 py-2.5 text-[13px] font-bold text-zinc-400 dark:text-white/40 hover:bg-zinc-50 dark:hover:bg-white/[0.03] active:scale-[0.98] transition-all"
                            >
                                后台继续...
                            </button>
                        ) : (
                            <div className="flex items-center justify-center gap-1.5 py-1.5 text-[12px] text-zinc-400 dark:text-white/30">
                                <span className="w-1.5 h-1.5 rounded-full bg-emerald-500 animate-pulse" />
                                处理中，请稍候...
                            </div>
                        )}
                    </div>
                </div>
            </div>

            <style>{`
                @keyframes spm-fade { from { opacity: 0 } to { opacity: 1 } }
                @keyframes spm-slide-up { from { transform: translateY(100%) } to { transform: translateY(0) } }
                @keyframes spm-ring { 0%,100% { opacity: .6; transform: scale(1) } 50% { opacity: 0; transform: scale(1.35) } }
            `}</style>
        </div>
    );
}
