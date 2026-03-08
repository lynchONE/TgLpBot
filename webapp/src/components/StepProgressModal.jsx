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
    const base = 'spm-indicator';
    if (status === 'done') {
        return (
            <span className={`${base} ${base}--done`}>
                <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="3.5" strokeLinecap="round" strokeLinejoin="round">
                    <path d="M5 13l4 4L19 7" />
                </svg>
            </span>
        );
    }
    if (status === 'active') {
        return (
            <span className={`${base} ${base}--active`}>
                <span className="spm-indicator-icon"><StepSvg type={icon} size={12} /></span>
                <span className="spm-ring" />
            </span>
        );
    }
    if (status === 'error') {
        return (
            <span className={`${base} ${base}--error`}>
                <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="3.5" strokeLinecap="round" strokeLinejoin="round">
                    <path d="M6 18L18 6M6 6l12 12" />
                </svg>
            </span>
        );
    }
    return (
        <span className={`${base} ${base}--pending`}>
            <StepSvg type={icon} size={12} />
        </span>
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

    return (
        <div className="spm-overlay" onClick={canClose ? onClose : undefined}>
            <div className="spm-card" onClick={(e) => e.stopPropagation()}>
                {/* Header */}
                <div className="spm-header">
                    <div className="spm-title-row">
                        <h3 className="spm-title">{title}</h3>
                        {canClose && (
                            <button type="button" className="spm-close" onClick={onClose}>
                                <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round"><path d="M18 6 6 18M6 6l12 12" /></svg>
                            </button>
                        )}
                    </div>
                    {/* Progress bar */}
                    <div className="spm-progress-track">
                        <div
                            className={`spm-progress-fill ${isError ? 'spm-progress-fill--error' : ''}`}
                            style={{ width: `${(doneCount / stepDefs.length) * 100}%` }}
                        />
                    </div>
                </div>

                {/* Steps */}
                <div className="spm-steps">
                    {stepDefs.map((step, i) => {
                        let stepStatus;
                        if (i < displayStep) stepStatus = 'done';
                        else if (i === displayStep) stepStatus = displayStatus;
                        else stepStatus = 'pending';

                        return (
                            <div key={i} className={`spm-step spm-step--${stepStatus}`}>
                                <StepIndicator status={stepStatus} icon={step.icon} />
                                <div className="spm-step-body">
                                    <span className="spm-step-label">{step.label}</span>
                                    {stepStatus === 'error' && error && (
                                        <div className="spm-step-error">{error}</div>
                                    )}
                                </div>
                                {i < stepDefs.length - 1 && (
                                    <div className={`spm-connector ${i < displayStep ? 'spm-connector--done' : ''}`} />
                                )}
                            </div>
                        );
                    })}
                </div>

                {/* Footer */}
                <div className="spm-footer">
                    {isDone ? (
                        <button type="button" className="spm-btn spm-btn--done" onClick={onClose}>
                            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round"><path d="M5 13l4 4L19 7" /></svg>
                            完成
                        </button>
                    ) : isError ? (
                        <button type="button" className="spm-btn spm-btn--error" onClick={onClose}>关闭</button>
                    ) : allowClose ? (
                        <button type="button" className="spm-btn spm-btn--ghost" onClick={onClose}>后台继续...</button>
                    ) : (
                        <div className="spm-hint">
                            <span className="spm-hint-dot" />
                            处理中，请稍候...
                        </div>
                    )}
                </div>
            </div>
        </div>
    );
}
