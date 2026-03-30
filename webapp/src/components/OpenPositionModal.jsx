import React, { useCallback, useEffect, useMemo, useState } from 'react';

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

export default function OpenPositionModal({
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
  const [error, setError] = useState('');
  const [riskAck, setRiskAck] = useState(false);

  const pair = pool?.trading_pair || '--';
  const addr = String(pool?.pool_address || '').trim();
  const version = String(pool?.protocol_version || pool?.factory_name || '').trim();
  const riskMessage = String(submitRisk?.message || '').trim();
  const riskLiquidityUsd = Number(submitRisk?.liquidity_usd);
  const riskMaxOpenAmount = Number(submitRisk?.max_open_amount);
  const riskRequiresAck = Boolean(submitRisk?.risk_ack_required);

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

  const visibleError = error || String(submitError || '').trim();

  useEffect(() => {
    setRiskAck(false);
  }, [addr]);

  useEffect(() => {
    if (!riskRequiresAck) setRiskAck(false);
  }, [riskRequiresAck]);

  const clearErrors = useCallback(() => {
    if (error) setError('');
    if (typeof onClearSubmitError === 'function') onClearSubmitError();
  }, [error, onClearSubmitError]);

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
    const amt = Number(amount);
    const rl = Number(rangeLower);
    const ru = Number(rangeUpper);
    const sl = Number(slippage);

    if (!Number.isFinite(amt) || amt <= 0) {
      setError('请输入有效的开仓金额');
      return;
    }
    if (!Number.isFinite(rl) || rl <= 0) {
      setError('请输入有效的下限范围');
      return;
    }
    if (!Number.isFinite(ru) || ru <= 0) {
      setError('请输入有效的上限范围');
      return;
    }
    if (showWalletPicker && !resolvedWalletId) {
      setError('请选择钱包');
      return;
    }
    if (Number.isFinite(riskMaxOpenAmount) && riskMaxOpenAmount > 0 && amt > riskMaxOpenAmount) {
      setError(`当前池子单次开仓金额不能高于 ${riskMaxOpenAmount} USDT`);
      return;
    }
    if (riskRequiresAck && !riskAck) {
      setError('请先确认低流动性风险');
      return;
    }

    setError('');
    onSubmit({
      poolAddress: addr,
      poolVersion: version,
      chain,
      amount: amt,
      rangeLowerPct: rl,
      rangeUpperPct: ru,
      slippageTolerance: Number.isFinite(sl) && sl > 0 ? sl : undefined,
      allowEntrySwap: true,
      walletId: resolvedWalletId || undefined,
      ackLiquidityRisk: riskRequiresAck && riskAck,
    });
  }, [
    amount,
    rangeLower,
    rangeUpper,
    slippage,
    showWalletPicker,
    resolvedWalletId,
    riskMaxOpenAmount,
    riskRequiresAck,
    riskAck,
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
        <div className="modal-info-note">
          如果这是当前钱包首次开仓，系统会先部署私有合约，部署完成后绑定到当前钱包，再继续正式开仓。
          首次流程失败后再次重试，会继续复用已部署地址完成绑定，不会重复部署新的私有合约。
        </div>

        {riskMessage ? (
          <div
            className="modal-info-note"
            style={{
              marginTop: 12,
              borderColor: riskRequiresAck ? 'rgba(245, 158, 11, 0.35)' : 'rgba(239, 68, 68, 0.35)',
              background: riskRequiresAck ? 'rgba(245, 158, 11, 0.10)' : 'rgba(239, 68, 68, 0.10)',
              color: riskRequiresAck ? '#b45309' : '#b91c1c',
            }}
          >
            <div>{riskMessage}</div>
            {Number.isFinite(riskLiquidityUsd) && riskLiquidityUsd >= 0 ? (
              <div style={{ marginTop: 6 }}>当前流动性: {formatUsdCompact(riskLiquidityUsd)}</div>
            ) : null}
            {Number.isFinite(riskMaxOpenAmount) && riskMaxOpenAmount > 0 ? (
              <div style={{ marginTop: 4 }}>当前最大允许开仓金额: {formatUsdCompact(riskMaxOpenAmount)}</div>
            ) : null}
            {riskRequiresAck ? (
              <label style={{ display: 'flex', gap: 8, alignItems: 'flex-start', marginTop: 10, cursor: 'pointer' }}>
                <input
                  type="checkbox"
                  checked={riskAck}
                  onChange={(e) => {
                    clearErrors();
                    setRiskAck(e.target.checked);
                  }}
                  disabled={busy}
                />
                <span>我已知悉当前池子流动性偏低，确认按限额继续开仓</span>
              </label>
            ) : null}
          </div>
        ) : null}

        {visibleError ? <div className="error-text">{visibleError}</div> : null}

        {walletsLoading ? (
          <div className="wallet-picker-loading">加载钱包中...</div>
        ) : null}

        {showWalletPicker && !walletsLoading ? (
          <div className="wallet-picker">
            <span className="wallet-picker-label">选择钱包</span>
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
            <span>金额 (USDT)</span>
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
                <div className="modal-range-hint">聪明钱当前净投入</div>
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
            {visibleSmartRanges.length > 0 ? (
              <div className="modal-range-hint">下方为默认区间</div>
            ) : null}
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
            <span>滑点 %</span>
            <input
              type="number"
              value={slippage}
              onChange={(e) => {
                clearErrors();
                setSlippage(e.target.value);
              }}
              min="0.1"
              step="0.1"
              placeholder="留空则使用全局配置"
            />
          </label>
        </div>

        <div className="modal-actions">
          <button type="button" className="ghost-chip" onClick={onClose} disabled={busy}>取消</button>
          <button type="button" className="accent-btn" onClick={handleSubmit} disabled={busy}>
            {busy ? '提交中...' : visibleError || riskMessage ? '重试开仓' : '确认开仓'}
          </button>
        </div>
      </div>
    </div>
  );
}
