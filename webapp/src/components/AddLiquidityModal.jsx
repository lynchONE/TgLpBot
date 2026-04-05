import React, { useState, useRef, useEffect, useCallback } from 'react';

export default function AddLiquidityModal({ position, onConfirm, onClose }) {
  const [amount, setAmount] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const inputRef = useRef(null);

  useEffect(() => {
    const t = setTimeout(() => inputRef.current?.focus(), 80);
    return () => clearTimeout(t);
  }, []);

  // Close on Escape
  useEffect(() => {
    const handler = (e) => { if (e.key === 'Escape') onClose(); };
    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  }, [onClose]);

  const parsedAmount = Number(amount);
  const isValid = Number.isFinite(parsedAmount) && parsedAmount > 0;

  const handleSubmit = useCallback(async () => {
    if (!isValid || submitting) return;
    setSubmitting(true);
    try {
      await onConfirm(parsedAmount);
      onClose();
    } catch {
      setSubmitting(false);
    }
  }, [isValid, submitting, parsedAmount, onConfirm, onClose]);

  const handleKeyDown = useCallback((e) => {
    if (e.key === 'Enter') handleSubmit();
  }, [handleSubmit]);

  const title = position
    ? `${position.token_rows?.[0]?.symbol || ''}/${position.token_rows?.[1]?.symbol || ''}`
    : '补充流动性';

  const currentValue = position?.totals?.position_usd;

  return (
    <div className="add-liq-overlay" onClick={onClose}>
      <div className="add-liq-card" onClick={(e) => e.stopPropagation()}>
        <h3 className="add-liq-title">💧 补充流动性</h3>
        <p className="add-liq-subtitle">
          {title}
          {currentValue > 0 && <> · 当前仓位价值 ${currentValue.toFixed(2)}</>}
        </p>

        <div className="add-liq-input-wrap">
          <input
            ref={inputRef}
            className="add-liq-input"
            type="number"
            min="0"
            step="any"
            placeholder="输入金额"
            value={amount}
            onChange={(e) => setAmount(e.target.value)}
            onKeyDown={handleKeyDown}
            disabled={submitting}
          />
          <span className="add-liq-unit">USDT</span>
        </div>

        <div className="add-liq-actions">
          <button className="add-liq-btn-cancel" onClick={onClose} disabled={submitting}>
            取消
          </button>
          <button className="add-liq-btn-confirm" onClick={handleSubmit} disabled={!isValid || submitting}>
            {submitting ? '提交中...' : '确认补充'}
          </button>
        </div>
      </div>
    </div>
  );
}

