import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { AlertTriangle, Check, CheckCircle, X, XCircle } from 'lucide-react';
import { previewOpenPosition } from '../api';

const PRESET_RANGES = [1, 2, 3, 5, 10, 20];

function shortAddr(addr) {
  const value = String(addr || '').trim();
  if (value.length <= 10) return value || '--';
  return `${value.slice(0, 6)}..${value.slice(-4)}`;
}

function formatUsdCompact(value) {
  const num = Number(value);
  if (!Number.isFinite(num) || num <= 0) return '--';
  const abs = Math.abs(num);
  if (abs >= 1000000) return `$${(num / 1000000).toFixed(abs >= 10000000 ? 0 : 1).replace(/\.0$/, '')}M`;
  if (abs >= 1000) return `$${(num / 1000).toFixed(abs >= 10000 ? 0 : 1).replace(/\.0$/, '')}K`;
  if (abs >= 100) return `$${num.toFixed(0)}`;
  if (abs >= 10) return `$${num.toFixed(1).replace(/\.0$/, '')}`;
  return `$${num.toFixed(2).replace(/0+$/, '').replace(/\.$/, '')}`;
}

function parseOptionalPercent(raw) {
  const text = String(raw || '').trim();
  if (!text) return { valid: true, value: undefined };
  const num = Number(text);
  if (!Number.isFinite(num) || num < 0 || num > 100) {
    return { valid: false, value: undefined };
  }
  return { valid: true, value: num };
}

function formatPercent(value) {
  const num = Number(value);
  if (!Number.isFinite(num)) return '--';
  return `${num.toFixed(num >= 1 ? 2 : 3).replace(/0+$/, '').replace(/\.$/, '')}%`;
}

function buildEntrySwapConfirmKey(preview, entrySwapSlippage) {
  return [
    preview?.required ? '1' : '0',
    preview?.from_token_address || '',
    preview?.to_token_address || '',
    preview?.amount_in_raw || '',
    preview?.expected_amount_out_raw || '',
    String(entrySwapSlippage || '').trim(),
  ].join('|');
}

export default function OpenPositionModal({
  apiBaseUrl,
  initData,
  pool,
  chain,
  wallets,
  walletsLoading,
  smartRanges,
  smartRangesLoading,
  selectedWalletId,
  submitError,
  submitRisk,
  onClearSubmitError,
  onWalletSelect,
  onSubmit,
  onClose,
  busy,
}) {
  const [amount, setAmount] = useState('100');
  const [rangeLower, setRangeLower] = useState('2');
  const [rangeUpper, setRangeUpper] = useState('2');
  const [rangeUpperAuto, setRangeUpperAuto] = useState(true);
  const [slippage, setSlippage] = useState('');
  const [entrySwapSlippage, setEntrySwapSlippage] = useState('');
  const [entrySwapSlippageDirty, setEntrySwapSlippageDirty] = useState(false);
  const [entrySwapConfirmed, setEntrySwapConfirmed] = useState(false);
  const [entrySwapPreview, setEntrySwapPreview] = useState(null);
  const [entrySwapPreviewLoading, setEntrySwapPreviewLoading] = useState(false);
  const [entrySwapPreviewError, setEntrySwapPreviewError] = useState('');
  const [previewChecks, setPreviewChecks] = useState([]);
  const [error, setError] = useState('');
  const [riskAck, setRiskAck] = useState(false);

  const pair = pool?.trading_pair || '--';
  const addr = String(pool?.pool_address || '').trim();
  const version = String(pool?.protocol_version || pool?.factory_name || '').trim();
  const checks = previewChecks;
  const warnChecks = checks.filter(c => c.status === 'warn');
  const failChecks = checks.filter(c => c.status === 'fail');
  const blockingFailChecks = failChecks;
  const hasBlockingSafetyFailure = blockingFailChecks.length > 0 || Boolean(submitRisk?.message);
  const blockingSafetyMessage = blockingFailChecks.length > 0
    ? blockingFailChecks.map(c => c.detail || c.label).filter(Boolean).join('; ')
    : '';
  const riskRequiresAck = warnChecks.some(c => c.extra?.risk_ack_required);
  const riskMaxOpenAmount = warnChecks.reduce((m, c) => {
    const v = Number(c.extra?.max_open_amount);
    return (Number.isFinite(v) && v > 0 && (m === null || v < m)) ? v : m;
  }, null);
  const riskLiquidityUsd = warnChecks.reduce((m, c) => {
    const v = Number(c.extra?.liquidity_usd);
    return (Number.isFinite(v) && v >= 0 && m === null) ? v : m;
  }, null);
  const riskMessage = warnChecks.length > 0
    ? (warnChecks.map(c => c.detail || c.label).filter(Boolean).join('；') || null)
    : null;

  const showWalletPicker = Array.isArray(wallets) && wallets.length > 1;
  const visibleSmartRanges = useMemo(() => (
    Array.isArray(smartRanges)
      ? smartRanges
        .filter((item) => Number(item?.range_percent) > 0)
        .slice(0, 6)
      : []
  ), [smartRanges]);
  const resolvedWalletId = useMemo(() => {
    if (!Array.isArray(wallets) || wallets.length === 0) return 0;
    if (wallets.length === 1) return wallets[0].id;
    if (selectedWalletId && wallets.some((w) => w.id === selectedWalletId)) return selectedWalletId;
    const def = wallets.find((w) => w.is_default);
    return def ? def.id : wallets[0].id;
  }, [wallets, selectedWalletId]);

  const taskSlippage = useMemo(() => parseOptionalPercent(slippage), [slippage]);
  const entrySwapSlippageValue = useMemo(() => parseOptionalPercent(entrySwapSlippage), [entrySwapSlippage]);
  const amountValue = Number(amount);
  const rangeLowerValue = Number(rangeLower);
  const rangeUpperValue = Number(rangeUpper);
  const submitRiskMessage = String(submitRisk?.message || '').trim();
  const visibleError = error || entrySwapPreviewError || blockingSafetyMessage || submitRiskMessage || String(submitError || '').trim();

  const previewRequest = useMemo(() => {
    if (!apiBaseUrl || !initData || !addr || !version) return null;
    if (!Number.isFinite(amountValue) || amountValue <= 0) return null;
    if (!Number.isFinite(rangeLowerValue) || rangeLowerValue <= 0 || rangeLowerValue >= 100) return null;
    if (!Number.isFinite(rangeUpperValue) || rangeUpperValue <= 0 || rangeUpperValue >= 100) return null;
    if (!taskSlippage.valid || !entrySwapSlippageValue.valid) return null;
    if (walletsLoading) return null;
    if (showWalletPicker && !resolvedWalletId) return null;
    return {
      apiBaseUrl,
      initData,
      chain,
      poolAddress: addr,
      poolVersion: version,
      amount: amountValue,
      rangeLowerPct: rangeLowerValue,
      rangeUpperPct: rangeUpperValue,
      slippageTolerance: taskSlippage.value,
      entrySwapSlippageTolerance: entrySwapSlippageValue.value,
      allowEntrySwap: true,
      walletId: resolvedWalletId || undefined,
      ackLiquidityRisk: riskAck,
    };
  }, [
    apiBaseUrl,
    initData,
    addr,
    version,
    chain,
    amountValue,
    rangeLowerValue,
    rangeUpperValue,
    taskSlippage,
    entrySwapSlippageValue,
    walletsLoading,
    showWalletPicker,
    resolvedWalletId,
    riskRequiresAck,
    riskAck,
  ]);

  const entrySwapConfirmKey = useMemo(
    () => buildEntrySwapConfirmKey(entrySwapPreview, entrySwapSlippage),
    [entrySwapPreview, entrySwapSlippage],
  );

  useEffect(() => {
    setRiskAck(false);
    setPreviewChecks([]);
    setEntrySwapPreview(null);
    setEntrySwapPreviewError('');
    setEntrySwapPreviewLoading(false);
    setEntrySwapSlippage('');
    setEntrySwapSlippageDirty(false);
    setEntrySwapConfirmed(false);
  }, [addr, version]);

  useEffect(() => {
    if (!riskRequiresAck) setRiskAck(false);
  }, [riskRequiresAck]);

  useEffect(() => {
    if (!entrySwapPreview?.required || entrySwapSlippageDirty) return;
    const recommended = Number(entrySwapPreview?.recommended_slippage_tolerance);
    const current = Number(entrySwapPreview?.current_slippage_tolerance);
    const next = Number.isFinite(recommended) ? recommended : current;
    if (!Number.isFinite(next)) return;
    setEntrySwapSlippage(String(next));
  }, [entrySwapPreview, entrySwapSlippageDirty]);

  useEffect(() => {
    setEntrySwapConfirmed(false);
  }, [entrySwapConfirmKey]);

  useEffect(() => {
    if (!previewRequest) {
      setEntrySwapPreview(null);
      setEntrySwapPreviewLoading(false);
      setEntrySwapPreviewError('');
      setPreviewChecks([]);
      return undefined;
    }

    let active = true;
    const controller = new AbortController();
    setEntrySwapPreviewLoading(true);
    setEntrySwapPreviewError('');

    const timer = window.setTimeout(async () => {
      try {
        const resp = await previewOpenPosition({
          ...previewRequest,
          signal: controller.signal,
        });
        if (!active) return;
        setPreviewChecks(Array.isArray(resp?.checks) ? resp.checks : []);
        setEntrySwapPreview(resp?.entry_swap || { required: false });
      } catch (e) {
        if (!active || controller.signal.aborted) return;
        setEntrySwapPreview(null);
        setPreviewChecks([]);
        setEntrySwapPreviewError(String(e?.message || e || '获取前置兑换预览失败'));
      } finally {
        if (active) {
          setEntrySwapPreviewLoading(false);
        }
      }
    }, 350);

    return () => {
      active = false;
      window.clearTimeout(timer);
      controller.abort();
    };
  }, [previewRequest]);

  const clearErrors = useCallback(() => {
    if (error) setError('');
    if (entrySwapPreviewError) setEntrySwapPreviewError('');
    if (typeof onClearSubmitError === 'function') onClearSubmitError();
  }, [error, entrySwapPreviewError, onClearSubmitError]);

  const applyRange = useCallback((lo, hi) => {
    clearErrors();
    setRangeLower(String(lo));
    setRangeUpper(String(hi));
    setRangeUpperAuto(true);
  }, [clearErrors]);

  const handleRangeLowerChange = useCallback((value) => {
    clearErrors();
    setRangeLower((prevLower) => {
      if (rangeUpperAuto || String(rangeUpper || '').trim() === '' || String(rangeUpper) === String(prevLower)) {
        setRangeUpper(value);
      }
      return value;
    });
  }, [clearErrors, rangeUpper, rangeUpperAuto]);

  const handleRangeUpperChange = useCallback((value) => {
    clearErrors();
    setRangeUpperAuto(false);
    setRangeUpper(value);
  }, [clearErrors]);

  const handleSubmit = useCallback(() => {
    if (!Number.isFinite(amountValue) || amountValue <= 0) {
      setError('请输入有效的开仓金额。');
      return;
    }
    if (!Number.isFinite(rangeLowerValue) || rangeLowerValue <= 0 || rangeLowerValue >= 100) {
      setError('请输入有效的下限范围。');
      return;
    }
    if (!Number.isFinite(rangeUpperValue) || rangeUpperValue <= 0 || rangeUpperValue >= 100) {
      setError('请输入有效的上限范围。');
      return;
    }
    if (!taskSlippage.valid) {
      setError('任务滑点必须在 0 到 100 之间。');
      return;
    }
    if (!entrySwapSlippageValue.valid) {
      setError('前置兑换滑点必须在 0 到 100 之间。');
      return;
    }
    if (showWalletPicker && !resolvedWalletId) {
      setError('请选择钱包。');
      return;
    }
    if (failChecks.length > 0) {
      setError(failChecks.map(c => c.detail || c.label).join('; '));
      return;
    }
    if (riskMaxOpenAmount !== null && amountValue > riskMaxOpenAmount) {
      setError(`当前池子单次开仓金额不能高于 ${riskMaxOpenAmount} USDT。`);
      return;
    }
    if (riskRequiresAck && !riskAck) {
      setError('请先确认低流动性风险。');
      return;
    }
    if (previewRequest && entrySwapPreviewLoading) {
      setError('前置兑换预览仍在加载，请稍后再试。');
      return;
    }
    if (previewRequest && !entrySwapPreview) {
      setError('前置兑换预览尚未就绪，请稍后再试。');
      return;
    }
    if (entrySwapPreview?.required && !entrySwapConfirmed) {
      setError('请先确认前置兑换，再继续开仓。');
      return;
    }

    setError('');
    onSubmit({
      poolAddress: addr,
      poolVersion: version,
      chain,
      amount: amountValue,
      rangeLowerPct: rangeLowerValue,
      rangeUpperPct: rangeUpperValue,
      slippageTolerance: taskSlippage.value,
      entrySwapSlippageTolerance: entrySwapPreview?.required ? entrySwapSlippageValue.value : undefined,
      allowEntrySwap: true,
      confirmEntrySwap: Boolean(entrySwapPreview?.required && entrySwapConfirmed),
      walletId: resolvedWalletId || undefined,
      ackLiquidityRisk: riskAck,
    });
  }, [
    amountValue,
    rangeLowerValue,
    rangeUpperValue,
    taskSlippage,
    entrySwapSlippageValue,
    showWalletPicker,
    resolvedWalletId,
    riskMaxOpenAmount,
    riskRequiresAck,
    riskAck,
    failChecks,
    previewRequest,
    entrySwapPreviewLoading,
    entrySwapPreview,
    entrySwapConfirmed,
    onSubmit,
    addr,
    version,
    chain,
  ]);

  return (
    <div className="modal-overlay" onClick={onClose}>
      <div className="modal-box" onClick={(e) => e.stopPropagation()}>
        <div className="modal-header">
          <h3>开仓</h3>
          <button type="button" className="modal-close" onClick={onClose} disabled={busy}>&times;</button>
        </div>

        <div className="modal-pair">{pair}</div>
        <div className="modal-addr">{addr ? `${addr.slice(0, 10)}...${addr.slice(-8)}` : '--'}</div>
        <div className="modal-info-note" style={{ display: 'flex', gap: '10px', alignItems: 'flex-start', padding: '14px', borderRadius: '16px', border: '1px solid rgba(16, 185, 129, 0.3)', background: 'linear-gradient(135deg, rgba(16, 185, 129, 0.1), transparent)' }}>
          <div style={{ marginTop: '2px', display: 'flex', alignItems: 'center', justifyContent: 'center', width: '20px', height: '20px', borderRadius: '50%', background: 'rgba(16, 185, 129, 0.2)', color: '#10b981', flexShrink: 0 }}>
            <Check size={12} strokeWidth={3} />
          </div>
          <div style={{ fontSize: '12px', lineHeight: 1.6, color: 'var(--text-hint, rgba(255, 255, 255, 0.8))' }}>
            <strong style={{ display: 'block', marginBottom: '4px' }}>私有合约保驾护航</strong>
            首次开仓时会自动部署与您钱包绑定的专属合约，确保交易更安全私密。如遇网络中断，再次重试即可直接复用，不会重复产生部署消耗。
          </div>
        </div>

        {riskMessage ? (
          <div
            style={{
              marginTop: 12,
              padding: 16,
              borderRadius: 16,
              border: '1px solid',
              borderColor: riskRequiresAck ? 'rgba(245, 158, 11, 0.4)' : 'rgba(239, 68, 68, 0.4)',
              background: riskRequiresAck ? 'linear-gradient(135deg, rgba(245, 158, 11, 0.1), rgba(245, 158, 11, 0.05))' : 'linear-gradient(135deg, rgba(239, 68, 68, 0.1), rgba(239, 68, 68, 0.05))',
              color: 'var(--text-color)',
              boxShadow: '0 1px 2px rgba(0,0,0,0.05)',
              display: 'flex',
              gap: 12,
              alignItems: 'flex-start',
            }}
          >
            <AlertTriangle size={20} style={{ color: riskRequiresAck ? '#f59e0b' : '#ef4444', flexShrink: 0, marginTop: 2 }} />
            <div style={{ flex: 1, display: 'flex', flexDirection: 'column', gap: 10 }}>
              <div style={{ fontSize: 13, lineHeight: 1.5, fontWeight: 600, color: riskRequiresAck ? '#d97706' : '#dc2626' }}>{riskMessage}</div>
              {((Number.isFinite(riskLiquidityUsd) && riskLiquidityUsd >= 0) || (Number.isFinite(riskMaxOpenAmount) && riskMaxOpenAmount > 0)) && (
                <div style={{ backgroundColor: 'var(--bg-card-hover, rgba(255,255,255,0.08))', borderRadius: 12, padding: '10px 12px', display: 'flex', flexDirection: 'column', gap: 6 }}>
                  {Number.isFinite(riskLiquidityUsd) && riskLiquidityUsd >= 0 && (
                    <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 12 }}>
                      <span style={{ opacity: 0.8 }}>当前流动性</span>
                      <span style={{ fontFamily: 'monospace', fontWeight: 600 }}>{formatUsdCompact(riskLiquidityUsd)}</span>
                    </div>
                  )}
                  {Number.isFinite(riskMaxOpenAmount) && riskMaxOpenAmount > 0 && (
                    <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 12 }}>
                      <span style={{ opacity: 0.8 }}>最大允许开仓</span>
                      <span style={{ fontFamily: 'monospace', fontWeight: 600 }}>{formatUsdCompact(riskMaxOpenAmount)}</span>
                    </div>
                  )}
                </div>
              )}
              {riskRequiresAck && (
                <label style={{ display: 'flex', gap: 10, alignItems: 'flex-start', marginTop: 4, cursor: 'pointer' }}>
                  <div style={{ position: 'relative', display: 'flex', alignItems: 'center', justifyContent: 'center', marginTop: 2, flexShrink: 0 }}>
                    <input
                      type="checkbox"
                      checked={riskAck}
                      style={{ opacity: 0, position: 'absolute', width: 16, height: 16, cursor: 'pointer', zIndex: 10 }}
                      onChange={(e) => {
                        clearErrors();
                        setRiskAck(e.target.checked);
                      }}
                      disabled={busy}
                    />
                    <div style={{ width: 16, height: 16, borderRadius: 4, border: '2px solid rgba(245, 158, 11, 0.5)', backgroundColor: riskAck ? '#f59e0b' : 'var(--bg-card, rgba(255,255,255,0.2))', transition: 'all 0.2s' }}></div>
                    <Check size={12} strokeWidth={3} style={{ position: 'absolute', color: '#fff', opacity: riskAck ? 1 : 0, transition: 'opacity 0.2s', pointerEvents: 'none' }} />
                  </div>
                  <span style={{ fontSize: 11, lineHeight: 1.4, fontWeight: 500, opacity: 0.9 }}>
                    我已知悉当前池子流动性偏低，确认按限额继续开仓
                  </span>
                </label>
              )}
            </div>
          </div>
        ) : null}

        {walletsLoading ? (
          <div className="wallet-picker-loading">钱包加载中...</div>
        ) : null}

        {showWalletPicker && !walletsLoading ? (
          <div className="wallet-picker">
            <span className="wallet-picker-label">钱包</span>
            <div className="wallet-picker-list">
              {wallets.map((wallet) => {
                const active = wallet.id === resolvedWalletId;
                return (
                  <button
                    key={wallet.id}
                    type="button"
                    className={`wallet-chip ${active ? 'active' : ''}`}
                    onClick={() => {
                      clearErrors();
                      onWalletSelect(wallet.id);
                    }}
                  >
                    <span className="wallet-chip-name">
                      {wallet.name || shortAddr(wallet.address)}
                      {wallet.is_default ? <span className="wallet-chip-default">默认</span> : null}
                    </span>
                    <span className="wallet-chip-addr">{shortAddr(wallet.address)}</span>
                    <span className="wallet-chip-bal">
                      {wallet.native_balance !== 'N/A' ? `${wallet.native_balance}` : ''}
                      {wallet.stable_balance !== 'N/A' ? ` / $${wallet.stable_balance}` : ''}
                    </span>
                  </button>
                );
              })}
            </div>
          </div>
        ) : null}

        <div className="modal-form">
          <label className="modal-field">
            <span>开仓金额 (USDT)</span>
            <input
              type="number"
              value={amount}
              onChange={(e) => {
                clearErrors();
                setAmount(e.target.value);
              }}
              min="1"
              step="10"
            />
          </label>

          <div className="modal-range-section">
            <span className="modal-range-label">快捷区间</span>
            {smartRangesLoading ? (
              <div className="modal-range-hint">聪明钱区间加载中...</div>
            ) : null}
            {visibleSmartRanges.length > 0 ? (
              <>
                <div className="modal-range-picks">
                  {visibleSmartRanges.map((item, index) => {
                    const rangePct = Number(item?.range_percent);
                    const positionCount = Math.max(0, Number(item?.position_count) || 0);
                    const isActive =
                      Math.abs(Number(rangeLower) - rangePct) < 0.05 &&
                      Math.abs(Number(rangeUpper) - rangePct) < 0.05;
                    return (
                      <button
                        key={`${rangePct}-${positionCount}-${index}`}
                        type="button"
                        className={`range-chip smart ${isActive ? 'active' : ''}`}
                        onClick={() => applyRange(rangePct, rangePct)}
                      >
                        <span>{`${rangePct}%${positionCount > 1 ? ` +${positionCount - 1}` : ''}`}</span>
                        <span className="range-chip-sub">{formatUsdCompact(item?.total_amount_usd)}</span>
                      </button>
                    );
                  })}
                </div>
                <div className="modal-range-hint">聪明钱近期开仓净额。</div>
              </>
            ) : null}
            <div className="modal-range-picks modal-range-picks-default">
              {PRESET_RANGES.map((item) => {
                const isActive =
                  Math.abs(Number(rangeLower) - item) < 0.05 &&
                  Math.abs(Number(rangeUpper) - item) < 0.05;
                return (
                  <button
                    key={item}
                    type="button"
                    className={`range-chip ${isActive ? 'active' : ''}`}
                    onClick={() => applyRange(item, item)}
                  >
                    {item}%
                  </button>
                );
              })}
            </div>
          </div>

          <div className="modal-row">
            <label className="modal-field">
              <span>下限 %</span>
              <input
                type="number"
                value={rangeLower}
                onChange={(e) => handleRangeLowerChange(e.target.value)}
                min="0.1"
                step="0.5"
              />
            </label>
            <label className="modal-field">
              <span>上限 %</span>
              <input
                type="number"
                value={rangeUpper}
                onChange={(e) => handleRangeUpperChange(e.target.value)}
                min="0.1"
                step="0.5"
              />
            </label>
          </div>

          <label className="modal-field">
            <span>任务滑点 %</span>
            <input
              type="number"
              value={slippage}
              onChange={(e) => {
                clearErrors();
                setSlippage(e.target.value);
              }}
              min="0"
              step="0.1"
              placeholder="留空则使用全局设置"
            />
          </label>
        </div>

        {(entrySwapPreviewLoading || entrySwapPreview?.required) ? (
          <div className="modal-info-note" style={{ marginTop: 12 }}>
            <div style={{ fontWeight: 600, marginBottom: 8 }}>前置兑换</div>
            {entrySwapPreviewLoading ? (
              <div>正在获取推荐滑点和预计到账数量...</div>
            ) : null}
            {entrySwapPreview?.required ? (
              <>
                <div style={{ marginTop: 6 }}>
                  推荐滑点：{formatPercent(entrySwapPreview?.recommended_slippage_tolerance)}
                </div>
                <div style={{ marginTop: 4 }}>
                  当前滑点：{formatPercent(entrySwapPreview?.current_slippage_tolerance)}
                </div>
                <div style={{ marginTop: 4 }}>
                  预计到账：{entrySwapPreview?.expected_amount_out || '--'} {entrySwapPreview?.to_token_symbol || ''}
                </div>
                <div style={{ marginTop: 4 }}>
                  兑换路径：{entrySwapPreview?.amount_in || '--'} {entrySwapPreview?.from_token_symbol || ''} 到 {entrySwapPreview?.to_token_symbol || ''}
                </div>

                <label className="modal-field" style={{ marginTop: 12 }}>
                  <span>前置兑换滑点 %</span>
                  <input
                    type="number"
                    value={entrySwapSlippage}
                    onChange={(e) => {
                      clearErrors();
                      setEntrySwapSlippageDirty(true);
                      setEntrySwapSlippage(e.target.value);
                    }}
                    min="0"
                    step="0.1"
                    placeholder="仅作用于本次前置兑换"
                  />
                </label>

                <label style={{ display: 'flex', gap: 8, alignItems: 'flex-start', marginTop: 10, cursor: 'pointer' }}>
                  <input
                    type="checkbox"
                    checked={entrySwapConfirmed}
                    onChange={(e) => {
                      clearErrors();
                      setEntrySwapConfirmed(e.target.checked);
                    }}
                    disabled={busy || entrySwapPreviewLoading}
                  />
                  <span>我已确认本次前置兑换，先执行兑换，再继续后续开仓。</span>
                </label>
              </>
            ) : null}
          </div>
        ) : null}

        {visibleError ? (
          <div style={{
            marginTop: 16,
            padding: 16,
            borderRadius: 16,
            border: '1px solid rgba(239, 68, 68, 0.4)',
            background: 'linear-gradient(135deg, rgba(239, 68, 68, 0.1), rgba(239, 68, 68, 0.05))',
            color: 'var(--text-error, #fca5a5)',
            display: 'flex',
            gap: 12,
            alignItems: 'flex-start',
            boxShadow: '0 1px 2px rgba(0,0,0,0.05)',
          }}>
            <div style={{ marginTop: 2, display: 'flex', alignItems: 'center', justifyContent: 'center', width: '20px', height: '20px', borderRadius: '50%', backgroundColor: 'rgba(239, 68, 68, 0.2)', color: '#ef4444', flexShrink: 0 }}>
              <X size={12} strokeWidth={3} />
            </div>
            <div style={{ fontSize: 12, fontWeight: 500, lineHeight: 1.5 }}>{visibleError}</div>
          </div>
        ) : null}

        <div className="modal-actions">
          <button type="button" className="ghost-chip" onClick={onClose} disabled={busy}>取消</button>
          <button type="button" className={`accent-btn ${hasBlockingSafetyFailure ? 'is-blocked' : ''}`} onClick={handleSubmit} disabled={busy || entrySwapPreviewLoading || hasBlockingSafetyFailure}>
            {busy ? '提交中...' : '确认开仓'}
          </button>
        </div>
      </div>
    </div>
  );
}
