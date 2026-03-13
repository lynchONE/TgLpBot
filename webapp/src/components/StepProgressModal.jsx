import React, { useEffect, useMemo, useRef, useState } from 'react';

function StatusIcon({ tone }) {
  if (tone === 'done') {
    return (
      <span style={styles.iconCircleDone}>
        <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.6" strokeLinecap="round" strokeLinejoin="round">
          <path d="M5 13l4 4L19 7" />
        </svg>
      </span>
    );
  }

  if (tone === 'error') {
    return (
      <span style={styles.iconCircleError}>
        <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.6" strokeLinecap="round" strokeLinejoin="round">
          <path d="M6 6l12 12M18 6 6 18" />
        </svg>
      </span>
    );
  }

  return (
    <span style={styles.iconCircleActive}>
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

const styles = {
  content: {
    display: 'flex',
    flexDirection: 'column',
    gap: 16,
    padding: '4px 0 2px',
  },
  headerLeft: {
    display: 'flex',
    flexDirection: 'column',
    gap: 8,
  },
  badge: {
    display: 'inline-flex',
    alignItems: 'center',
    width: 'fit-content',
    borderRadius: 999,
    padding: '4px 10px',
    fontSize: 12,
    fontWeight: 700,
    letterSpacing: '0.01em',
  },
  iconWrap: {
    display: 'flex',
    justifyContent: 'center',
  },
  iconCircleActive: {
    display: 'inline-flex',
    alignItems: 'center',
    justifyContent: 'center',
    width: 56,
    height: 56,
    borderRadius: '50%',
    color: '#f8fafc',
    background: 'linear-gradient(135deg, #2563eb, #0f766e)',
    boxShadow: '0 12px 30px rgba(37, 99, 235, 0.28)',
  },
  iconCircleDone: {
    display: 'inline-flex',
    alignItems: 'center',
    justifyContent: 'center',
    width: 56,
    height: 56,
    borderRadius: '50%',
    color: '#ffffff',
    background: 'linear-gradient(135deg, #16a34a, #059669)',
    boxShadow: '0 12px 30px rgba(22, 163, 74, 0.25)',
  },
  iconCircleError: {
    display: 'inline-flex',
    alignItems: 'center',
    justifyContent: 'center',
    width: 56,
    height: 56,
    borderRadius: '50%',
    color: '#ffffff',
    background: 'linear-gradient(135deg, #dc2626, #f97316)',
    boxShadow: '0 12px 30px rgba(220, 38, 38, 0.25)',
  },
  textBlock: {
    display: 'flex',
    flexDirection: 'column',
    gap: 8,
    textAlign: 'center',
  },
  headline: {
    margin: 0,
    fontSize: 22,
    lineHeight: 1.2,
    fontWeight: 800,
    color: '#f8fafc',
  },
  summary: {
    fontSize: 14,
    lineHeight: 1.6,
    color: 'rgba(226, 232, 240, 0.92)',
  },
  detail: {
    fontSize: 12,
    lineHeight: 1.6,
    color: 'rgba(148, 163, 184, 0.92)',
  },
  taskMeta: {
    display: 'inline-flex',
    justifyContent: 'center',
    alignItems: 'center',
    alignSelf: 'center',
    borderRadius: 999,
    padding: '6px 10px',
    fontSize: 12,
    fontWeight: 600,
    color: 'rgba(226, 232, 240, 0.92)',
    background: 'rgba(15, 23, 42, 0.75)',
    border: '1px solid rgba(148, 163, 184, 0.18)',
  },
  note: {
    borderRadius: 14,
    padding: '12px 14px',
    fontSize: 12,
    lineHeight: 1.6,
    color: 'rgba(191, 219, 254, 0.94)',
    background: 'rgba(37, 99, 235, 0.12)',
    border: '1px solid rgba(96, 165, 250, 0.24)',
  },
};

export default function StepProgressModal({ operation, progress, onClose }) {
  if (!operation) return null;

  const [allowClose, setAllowClose] = useState(false);
  const overlayRef = useRef(null);
  const view = useMemo(() => resolveView(operation, progress), [operation, progress]);

  useEffect(() => {
    const el = overlayRef.current;
    if (!el) return undefined;
    const parent = el.parentElement;
    if (!parent) return undefined;
    const height = parent.offsetHeight;
    parent.style.minHeight = `${height}px`;
    return () => {
      parent.style.minHeight = '';
    };
  }, []);

  useEffect(() => {
    const timer = setTimeout(() => setAllowClose(true), 10000);
    return () => clearTimeout(timer);
  }, []);

  const isActive = view.tone === 'active';
  const canClose = !isActive || allowClose;

  return (
    <div className="spm-overlay" ref={overlayRef} onClick={canClose ? onClose : undefined}>
      <div className="spm-card" onClick={(event) => event.stopPropagation()}>
        <div className="spm-header">
          <div className="spm-title-row">
            <div style={styles.headerLeft}>
              <h3 className="spm-title">{view.panelTitle}</h3>
              <span
                style={{
                  ...styles.badge,
                  color: view.tone === 'done' ? '#dcfce7' : view.tone === 'error' ? '#fecaca' : '#dbeafe',
                  background: view.tone === 'done' ? 'rgba(22, 163, 74, 0.18)' : view.tone === 'error' ? 'rgba(220, 38, 38, 0.18)' : 'rgba(37, 99, 235, 0.18)',
                  border: view.tone === 'done' ? '1px solid rgba(74, 222, 128, 0.24)' : view.tone === 'error' ? '1px solid rgba(248, 113, 113, 0.24)' : '1px solid rgba(96, 165, 250, 0.24)',
                }}
              >
                {view.badge}
              </span>
            </div>

            <button
              type="button"
              className={`spm-close ${canClose ? '' : 'spm-close--hidden'}`}
              onClick={onClose}
              disabled={!canClose}
              tabIndex={canClose ? 0 : -1}
              aria-hidden={!canClose}
            >
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round">
                <path d="M18 6 6 18M6 6l12 12" />
              </svg>
            </button>
          </div>
        </div>

        <div style={styles.content}>
          <div style={styles.iconWrap}>
            <StatusIcon tone={view.tone} />
          </div>

          <div style={styles.textBlock}>
            <p style={styles.headline}>{view.headline}</p>
            <div style={styles.summary}>{view.summary}</div>
            {view.detail ? <div style={styles.detail}>{view.detail}</div> : null}
          </div>

          {progress?.taskId ? (
            <div style={styles.taskMeta}>任务 #{progress.taskId}</div>
          ) : null}

          {isActive && allowClose ? (
            <div style={styles.note}>可以先关闭这个弹窗，任务会在后台继续执行。</div>
          ) : null}
        </div>

        <div className="spm-footer">
          {view.tone === 'done' ? (
            <button type="button" className="spm-btn spm-btn--done" onClick={onClose}>
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
                <path d="M5 13l4 4L19 7" />
              </svg>
              完成
            </button>
          ) : view.tone === 'error' ? (
            <button type="button" className="spm-btn spm-btn--error" onClick={onClose}>
              关闭
            </button>
          ) : allowClose ? (
            <button type="button" className="spm-btn spm-btn--ghost" onClick={onClose}>
              后台继续
            </button>
          ) : (
            <div className="spm-hint">
              <span className="spm-hint-dot" />
              处理中，请稍候...
            </div>
          )}
        </div>
      </div>

      <style>{`
        @keyframes spm-status-spin {
          from { transform: rotate(0deg); }
          to { transform: rotate(360deg); }
        }
      `}</style>
    </div>
  );
}
