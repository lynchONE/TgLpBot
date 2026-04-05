import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Droplets, Sparkles, X } from 'lucide-react';
import { formatUsdCompact } from '../utils';

function parseAmountInput(value) {
  return Number(String(value || '').replace(/,/g, '').trim());
}

function roundPresetAmount(value) {
  const num = Number(value);
  if (!Number.isFinite(num) || num <= 0) return 0;
  if (num >= 1000) return Math.round(num / 50) * 50;
  if (num >= 200) return Math.round(num / 20) * 20;
  if (num >= 50) return Math.round(num / 10) * 10;
  if (num >= 10) return Math.round(num / 5) * 5;
  return Math.round(num * 10) / 10;
}

function formatAmountInput(value) {
  const num = Number(value);
  if (!Number.isFinite(num) || num <= 0) return '';
  if (num >= 100) return String(Math.round(num));
  return num.toFixed(num >= 10 ? 1 : 2).replace(/0+$/, '').replace(/\.$/, '');
}

function formatRatio(value) {
  const num = Number(value);
  if (!Number.isFinite(num) || num <= 0) return '--';
  if (num >= 100) return `${Math.round(num)}%`;
  if (num >= 10) return `${num.toFixed(1).replace(/\.0$/, '')}%`;
  return `${num.toFixed(2).replace(/0+$/, '').replace(/\.$/, '')}%`;
}

function buildPresetOptions(referenceAmount) {
  const presets = [];
  const seen = new Set();

  const pushPreset = (value, hint) => {
    const rounded = roundPresetAmount(value);
    if (!(rounded > 0)) return;
    const key = rounded.toFixed(2);
    if (seen.has(key)) return;
    seen.add(key);
    presets.push({
      value: rounded,
      label: `${formatAmountInput(rounded)} USDT`,
      hint,
    });
  };

  if (referenceAmount > 0) {
    pushPreset(referenceAmount * 0.25, '25% 策略');
    pushPreset(referenceAmount * 0.5, '50% 策略');
    pushPreset(referenceAmount, '1x 策略');
  }

  pushPreset(50, '常用');
  pushPreset(100, '常用');
  pushPreset(200, '常用');

  return presets.slice(0, 4);
}

function resolvePositionTitle(position) {
  const explicitTitle = String(position?.title || '').trim();
  if (explicitTitle) return explicitTitle;

  const symbols = [position?.token_rows?.[0]?.symbol, position?.token_rows?.[1]?.symbol]
    .map((item) => String(item || '').trim())
    .filter(Boolean);
  if (symbols.length > 0) return symbols.join('/');

  const taskId = Number(position?.task_id || 0);
  return taskId > 0 ? `任务 #${taskId}` : '补充流动性';
}

export default function AddLiquidityModal({ position, onConfirm, onClose }) {
  const [amount, setAmount] = useState('');
  const [error, setError] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const inputRef = useRef(null);

  const currentValue = Number(
    position?.current_value_usd
    ?? position?.totals?.total_usd
    ?? position?.totals?.position_usd
    ?? 0
  );
  const referenceAmount = Number(
    position?.task_amount_usdt
    ?? position?.net_invested_usd
    ?? position?.initial_cost_usd
    ?? 0
  );
  const parsedAmount = parseAmountInput(amount);
  const isValid = Number.isFinite(parsedAmount) && parsedAmount > 0;
  const title = resolvePositionTitle(position);
  const presets = useMemo(() => buildPresetOptions(referenceAmount), [referenceAmount]);
  const ratioText = isValid && referenceAmount > 0
    ? `约为原策略金额的 ${formatRatio((parsedAmount / referenceAmount) * 100)}，会按当前池价买入并追加到现有仓位。`
    : '输入要追加的 USDT 金额，系统会按当前池价买入并补进当前仓位。';

  const requestClose = useCallback(() => {
    if (submitting) return;
    setError('');
    onClose();
  }, [onClose, submitting]);

  useEffect(() => {
    setAmount('');
    setError('');
  }, [position]);

  useEffect(() => {
    const timer = window.setTimeout(() => inputRef.current?.focus(), 80);
    return () => window.clearTimeout(timer);
  }, []);

  useEffect(() => {
    const handler = (event) => {
      if (event.key === 'Escape') requestClose();
    };
    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  }, [requestClose]);

  const handleSubmit = useCallback(async () => {
    if (!isValid || submitting) return;
    setSubmitting(true);
    setError('');
    try {
      await onConfirm(parsedAmount);
      onClose();
    } catch (submitError) {
      setError(String(submitError?.message || submitError || '补充流动性失败'));
      setSubmitting(false);
    }
  }, [isValid, onClose, onConfirm, parsedAmount, submitting]);

  const handleKeyDown = useCallback((event) => {
    if (event.key === 'Enter') {
      event.preventDefault();
      handleSubmit();
    }
  }, [handleSubmit]);

  const handlePresetClick = useCallback((value) => {
    setAmount(formatAmountInput(value));
    setError('');
    inputRef.current?.focus();
  }, []);

  return (
    <div className="add-liq-overlay" onClick={requestClose}>
      <div className="add-liq-card" onClick={(event) => event.stopPropagation()}>
        <div className="add-liq-shell">
          <div className="add-liq-header">
            <div className="add-liq-brand">
              <span className="add-liq-icon">
                <Droplets size={18} strokeWidth={2.1} />
              </span>
              <div className="add-liq-title-wrap">
                <h3 className="add-liq-title">补充流动性</h3>
                <p className="add-liq-subtitle">{title}</p>
              </div>
            </div>
            <button
              type="button"
              className="add-liq-close"
              onClick={requestClose}
              disabled={submitting}
              aria-label="关闭"
            >
              <X size={18} />
            </button>
          </div>

          <div className="add-liq-stats">
            <div className="add-liq-stat">
              <span className="add-liq-stat-label">当前仓位</span>
              <strong className="add-liq-stat-value">
                {currentValue > 0 ? formatUsdCompact(currentValue) : '$--'}
              </strong>
            </div>
            <div className="add-liq-stat">
              <span className="add-liq-stat-label">策略参考</span>
              <strong className="add-liq-stat-value">
                {referenceAmount > 0 ? formatUsdCompact(referenceAmount) : '$--'}
              </strong>
            </div>
          </div>

          <div className={`add-liq-input-panel${isValid ? ' is-active' : ''}`}>
            <div className="add-liq-field-head">
              <span>补充金额</span>
              <span className="add-liq-field-tag">
                <Sparkles size={12} />
                自定义
              </span>
            </div>
            <div className="add-liq-input-shell">
              <span className="add-liq-currency">$</span>
              <input
                ref={inputRef}
                className="add-liq-input"
                type="text"
                inputMode="decimal"
                placeholder="0.00"
                value={amount}
                onChange={(event) => {
                  setAmount(event.target.value);
                  if (error) setError('');
                }}
                onKeyDown={handleKeyDown}
                disabled={submitting}
                autoComplete="off"
              />
              <span className="add-liq-unit-badge">USDT</span>
            </div>
            <div className="add-liq-note">{ratioText}</div>
          </div>

          {presets.length > 0 ? (
            <div className="add-liq-presets">
              {presets.map((preset) => {
                const active = isValid && Math.abs(parsedAmount - preset.value) < 0.001;
                return (
                  <button
                    key={`${preset.value}-${preset.hint}`}
                    type="button"
                    className={`add-liq-preset${active ? ' active' : ''}`}
                    onClick={() => handlePresetClick(preset.value)}
                    disabled={submitting}
                  >
                    <span className="add-liq-preset-value">{preset.label}</span>
                    <span className="add-liq-preset-hint">{preset.hint}</span>
                  </button>
                );
              })}
            </div>
          ) : null}

          {error ? <div className="add-liq-error">{error}</div> : null}

          <div className="add-liq-actions">
            <button
              type="button"
              className="add-liq-btn-cancel"
              onClick={requestClose}
              disabled={submitting}
            >
              取消
            </button>
            <button
              type="button"
              className="add-liq-btn-confirm"
              onClick={handleSubmit}
              disabled={!isValid || submitting}
            >
              {submitting ? '处理中...' : '确认补充'}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}
