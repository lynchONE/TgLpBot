import React from 'react';

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
    const { currentStep = 0, status = 'active', error = '' } = progress || {};
    const isDone = status === 'done';
    const isError = status === 'error';
    const canClose = isDone || isError;
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
                        if (i < currentStep) stepStatus = 'done';
                        else if (i === currentStep) stepStatus = status;
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
                    ) : (
                        <div className="step-hint">请勿关闭此页面...</div>
                    )}
                </div>
            </div>
        </div>
    );
}
