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

function StepIcon({ status }) {
    if (status === 'done') {
        return (
            <span className="step-icon done">
                <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="3" strokeLinecap="round" strokeLinejoin="round">
                    <path d="M5 13l4 4L19 7" />
                </svg>
            </span>
        );
    }
    if (status === 'active') {
        return <span className="step-icon active"><span className="step-pulse" /></span>;
    }
    if (status === 'error') {
        return (
            <span className="step-icon error">
                <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="3" strokeLinecap="round" strokeLinejoin="round">
                    <path d="M6 18L18 6M6 6l12 12" />
                </svg>
            </span>
        );
    }
    return <span className="step-icon pending" />;
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
        if (targetStatus === 'error') {
            setDisplayStep(targetStep);
            setDisplayStatus('error');
            return;
        }

        if (displayStep < targetStep) {
            const timer = setTimeout(() => {
                setDisplayStep(prev => prev + 1);
            }, STEP_ANIM_MS);
            return () => clearTimeout(timer);
        }
    }, [displayStep, targetStep, targetStatus]);

    // Sync status once we've caught up
    useEffect(() => {
        if (targetStatus === 'error') return;
        if (displayStep >= targetStep) {
            setDisplayStatus(targetStatus);
        }
    }, [displayStep, targetStep, targetStatus]);

    const isDone = displayStatus === 'done' && displayStep >= targetStep;
    const isError = displayStatus === 'error';
    const canClose = isDone || isError || allowClose;
    const title = operation === 'open_position' ? '开仓进度' : '撤仓进度';

    return (
        <div className="modal-overlay" onClick={canClose ? onClose : undefined}>
            <div className="modal-box small" onClick={(e) => e.stopPropagation()}>
                <div className="modal-header">
                    <h3>{title}</h3>
                    {canClose && (
                        <button type="button" className="modal-close" onClick={onClose}>&times;</button>
                    )}
                </div>

                <div className="step-list">
                    {steps.map((label, i) => {
                        let stepStatus;
                        if (i < displayStep) stepStatus = 'done';
                        else if (i === displayStep) stepStatus = displayStatus;
                        else stepStatus = 'pending';

                        return (
                            <div key={i} className={`step-row ${stepStatus}`}>
                                <StepIcon status={stepStatus} />
                                <div className="step-content">
                                    <span className="step-label">{label}</span>
                                    {stepStatus === 'error' && error && (
                                        <div className="step-error">{error}</div>
                                    )}
                                </div>
                            </div>
                        );
                    })}
                </div>

                <div className="step-footer">
                    {isDone ? (
                        <button type="button" className="accent-btn" onClick={onClose}>完成</button>
                    ) : isError ? (
                        <button type="button" className="ghost-chip danger" onClick={onClose}>关闭</button>
                    ) : allowClose ? (
                        <button type="button" className="ghost-chip" onClick={onClose}>后台继续...</button>
                    ) : (
                        <div className="step-hint">请勿关闭此页面...</div>
                    )}
                </div>
            </div>
        </div>
    );
}
