import React, { useCallback, useEffect, useMemo, useState } from 'react';
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

function extractRiskPayload(error) {
  if (!error || typeof error !== 'object') return null;
  const hasRisk =
    typeof error?.liquidity_usd === 'number' ||
    typeof error?.max_open_amount === 'number' ||
    typeof error?.price_deviation_percent === 'number' ||
    Boolean(error?.risk_ack_required);
  if (!hasRisk) return null;
  return {
    code: String(error?.code || ''),
    message: String(error?.message || ''),
    liquidity_usd: Number(error?.liquidity_usd),
    min_liquidity_usd: Number(error?.min_liquidity_usd),
    max_open_amount: Number(error?.max_open_amount),
    risk_ack_required: Boolean(error?.risk_ack_required),
    price_deviation_percent: Number(error?.price_deviation_percent),
    price_deviation_max_percent: Number(error?.price_deviation_max_percent),
  };
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
  const [previewRisk, setPreviewRisk] = useState(null);
  const [error, setError] = useState('');
  const [riskAck, setRiskAck] = useState(false);

  const pair = pool?.trading_pair || '--';
  const addr = String(pool?.pool_address || '').trim();
  const version = String(pool?.protocol_version || pool?.factory_name || '').trim();
  const activeRisk = previewRisk || submitRisk;
  const riskMessage = String(activeRisk?.message || '').trim();
  const riskLiquidityUsd = Number(activeRisk?.liquidity_usd);
  const riskMaxOpenAmount = Number(activeRisk?.max_open_amount);
  const riskRequiresAck = Boolean(activeRisk?.risk_ack_required);

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
  const visibleError = error || entrySwapPreviewError || String(submitError || '').trim();

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
      ackLiquidityRisk: riskRequiresAck && riskAck,
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
    setPreviewRisk(null);
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
      setPreviewRisk(null);
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
        setPreviewRisk(null);
        setEntrySwapPreview(resp?.entry_swap || { required: false });
      } catch (e) {
        if (!active || controller.signal.aborted) return;
        const risk = extractRiskPayload(e);
        setEntrySwapPreview(null);
        if (risk) {
          setPreviewRisk(risk);
          setEntrySwapPreviewError('');
        } else {
          setPreviewRisk(null);
          setEntrySwapPreviewError(String(e?.message || e || 'Failed to load entry swap preview.'));
        }
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
      setError('Enter a valid amount.');
      return;
    }
    if (!Number.isFinite(rangeLowerValue) || rangeLowerValue <= 0 || rangeLowerValue >= 100) {
      setError('Enter a valid lower range.');
      return;
    }
    if (!Number.isFinite(rangeUpperValue) || rangeUpperValue <= 0 || rangeUpperValue >= 100) {
      setError('Enter a valid upper range.');
      return;
    }
    if (!taskSlippage.valid) {
      setError('Task slippage must be between 0 and 100.');
      return;
    }
    if (!entrySwapSlippageValue.valid) {
      setError('Entry swap slippage must be between 0 and 100.');
      return;
    }
    if (showWalletPicker && !resolvedWalletId) {
      setError('Select a wallet.');
      return;
    }
    if (Number.isFinite(riskMaxOpenAmount) && riskMaxOpenAmount > 0 && amountValue > riskMaxOpenAmount) {
      setError(`Max open amount is ${riskMaxOpenAmount} USDT.`);
      return;
    }
    if (riskRequiresAck && !riskAck) {
      setError('Confirm the liquidity risk first.');
      return;
    }
    if (previewRequest && entrySwapPreviewLoading) {
      setError('Entry swap preview is still loading.');
      return;
    }
    if (previewRequest && !entrySwapPreview && !riskMessage) {
      setError('Entry swap preview is not ready yet.');
      return;
    }
    if (entrySwapPreview?.required && !entrySwapConfirmed) {
      setError('Confirm the entry swap before opening.');
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
      ackLiquidityRisk: riskRequiresAck && riskAck,
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
    previewRequest,
    entrySwapPreviewLoading,
    entrySwapPreview,
    entrySwapConfirmed,
    riskMessage,
    onSubmit,
    addr,
    version,
    chain,
  ]);

  return (
    <div className="modal-overlay" onClick={onClose}>
      <div className="modal-box" onClick={(e) => e.stopPropagation()}>
        <div className="modal-header">
          <h3>Open Position</h3>
          <button type="button" className="modal-close" onClick={onClose} disabled={busy}>&times;</button>
        </div>

        <div className="modal-pair">{pair}</div>
        <div className="modal-addr">{addr ? `${addr.slice(0, 10)}...${addr.slice(-8)}` : '--'}</div>
        <div className="modal-info-note">
          If this wallet is opening a position for the first time, the bot may deploy and bind its private zap contract first.
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
              <div style={{ marginTop: 6 }}>Liquidity: {formatUsdCompact(riskLiquidityUsd)}</div>
            ) : null}
            {Number.isFinite(riskMaxOpenAmount) && riskMaxOpenAmount > 0 ? (
              <div style={{ marginTop: 4 }}>Max open amount: {formatUsdCompact(riskMaxOpenAmount)}</div>
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
                <span>I understand the liquidity risk and want to continue within the limit.</span>
              </label>
            ) : null}
          </div>
        ) : null}

        {walletsLoading ? (
          <div className="wallet-picker-loading">Loading wallets...</div>
        ) : null}

        {showWalletPicker && !walletsLoading ? (
          <div className="wallet-picker">
            <span className="wallet-picker-label">Wallet</span>
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
                      {wallet.is_default ? <span className="wallet-chip-default">Default</span> : null}
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
            <span>Amount (USDT)</span>
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
            <span className="modal-range-label">Quick Range</span>
            {smartRangesLoading ? (
              <div className="modal-range-hint">Loading smart ranges...</div>
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
                <div className="modal-range-hint">Smart-money net amount recently opened.</div>
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
              <span>Lower %</span>
              <input
                type="number"
                value={rangeLower}
                onChange={(e) => handleRangeLowerChange(e.target.value)}
                min="0.1"
                step="0.5"
              />
            </label>
            <label className="modal-field">
              <span>Upper %</span>
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
            <span>Task Slippage %</span>
            <input
              type="number"
              value={slippage}
              onChange={(e) => {
                clearErrors();
                setSlippage(e.target.value);
              }}
              min="0"
              step="0.1"
              placeholder="Leave empty to use global settings"
            />
          </label>
        </div>

        {(entrySwapPreviewLoading || entrySwapPreview?.required) ? (
          <div className="modal-info-note" style={{ marginTop: 12 }}>
            <div style={{ fontWeight: 600, marginBottom: 8 }}>Entry Swap</div>
            {entrySwapPreviewLoading ? (
              <div>Checking recommended slippage and expected receive amount...</div>
            ) : null}
            {entrySwapPreview?.required ? (
              <>
                <div style={{ marginTop: 6 }}>
                  Recommended slippage: {formatPercent(entrySwapPreview?.recommended_slippage_tolerance)}
                </div>
                <div style={{ marginTop: 4 }}>
                  Current slippage: {formatPercent(entrySwapPreview?.current_slippage_tolerance)}
                </div>
                <div style={{ marginTop: 4 }}>
                  Estimated receive: {entrySwapPreview?.expected_amount_out || '--'} {entrySwapPreview?.to_token_symbol || ''}
                </div>
                <div style={{ marginTop: 4 }}>
                  Route: {entrySwapPreview?.amount_in || '--'} {entrySwapPreview?.from_token_symbol || ''} to {entrySwapPreview?.to_token_symbol || ''}
                </div>

                <label className="modal-field" style={{ marginTop: 12 }}>
                  <span>Entry Swap Slippage %</span>
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
                    placeholder="Adjust only for this entry swap"
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
                  <span>Confirm this entry swap first, then continue opening the position.</span>
                </label>
              </>
            ) : null}
          </div>
        ) : null}

        {visibleError ? <div className="error-text">{visibleError}</div> : null}

        <div className="modal-actions">
          <button type="button" className="ghost-chip" onClick={onClose} disabled={busy}>Cancel</button>
          <button type="button" className="accent-btn" onClick={handleSubmit} disabled={busy}>
            {busy ? 'Submitting...' : 'Open Position'}
          </button>
        </div>
      </div>
    </div>
  );
}
