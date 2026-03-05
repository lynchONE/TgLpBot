import React, { useCallback, useState } from 'react';

export default function OpenPositionModal({ pool, chain, onSubmit, onClose, busy }) {
  const [amount, setAmount] = useState('100');
  const [rangeLower, setRangeLower] = useState('2');
  const [rangeUpper, setRangeUpper] = useState('2');
  const [slippage, setSlippage] = useState('1');
  const [entrySwap, setEntrySwap] = useState(true);
  const [error, setError] = useState('');

  const pair = pool?.trading_pair || '--';
  const addr = String(pool?.pool_address || '').trim();
  const version = String(pool?.protocol_version || pool?.factory_name || '').trim();

  const handleSubmit = useCallback(() => {
    const amt = Number(amount);
    const rl = Number(rangeLower);
    const ru = Number(rangeUpper);
    const sl = Number(slippage);

    if (!Number.isFinite(amt) || amt <= 0) { setError('请输入有效的金额'); return; }
    if (!Number.isFinite(rl) || rl <= 0) { setError('请输入有效的下限范围'); return; }
    if (!Number.isFinite(ru) || ru <= 0) { setError('请输入有效的上限范围'); return; }

    setError('');
    onSubmit({
      poolAddress: addr,
      poolVersion: version,
      chain,
      amount: amt,
      rangeLowerPct: rl,
      rangeUpperPct: ru,
      slippageTolerance: Number.isFinite(sl) ? sl : 1,
      allowEntrySwap: entrySwap,
    });
  }, [amount, rangeLower, rangeUpper, slippage, entrySwap, addr, version, chain, onSubmit]);

  return (
    <div className="modal-overlay" onClick={onClose}>
      <div className="modal-box" onClick={(e) => e.stopPropagation()}>
        <div className="modal-header">
          <h3>开仓</h3>
          <button type="button" className="modal-close" onClick={onClose}>&times;</button>
        </div>

        <div className="modal-pair">{pair}</div>
        <div className="modal-addr">{addr ? `${addr.slice(0, 10)}...${addr.slice(-8)}` : '--'}</div>

        {error && <div className="error-text">{error}</div>}

        <div className="modal-form">
          <label className="modal-field">
            <span>金额 (USDT)</span>
            <input type="number" value={amount} onChange={(e) => setAmount(e.target.value)} min="1" step="10" />
          </label>

          <div className="modal-row">
            <label className="modal-field">
              <span>下限 %</span>
              <input type="number" value={rangeLower} onChange={(e) => setRangeLower(e.target.value)} min="0.1" step="0.5" />
            </label>
            <label className="modal-field">
              <span>上限 %</span>
              <input type="number" value={rangeUpper} onChange={(e) => setRangeUpper(e.target.value)} min="0.1" step="0.5" />
            </label>
          </div>

          <label className="modal-field">
            <span>滑点 %</span>
            <input type="number" value={slippage} onChange={(e) => setSlippage(e.target.value)} min="0.1" step="0.1" />
          </label>

          <label className="modal-check">
            <input type="checkbox" checked={entrySwap} onChange={(e) => setEntrySwap(e.target.checked)} />
            <span>允许入场兑换</span>
          </label>
        </div>

        <div className="modal-actions">
          <button type="button" className="ghost-chip" onClick={onClose} disabled={busy}>取消</button>
          <button type="button" className="accent-btn" onClick={handleSubmit} disabled={busy}>
            {busy ? '提交中...' : '确认开仓'}
          </button>
        </div>
      </div>
    </div>
  );
}
