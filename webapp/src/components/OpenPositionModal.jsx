import React, { useCallback, useMemo, useState } from 'react';

const PRESET_RANGES = [1, 2, 3, 5, 10, 20];

function extractSmartMoneyRanges(wallets) {
  if (!Array.isArray(wallets) || wallets.length === 0) return [];
  const ranges = [];
  for (const w of wallets) {
    const lower = Number(w?.price_lower ?? 0);
    const upper = Number(w?.price_upper ?? 0);
    if (!lower || !upper || lower <= 0 || upper <= 0) continue;
    const mid = (lower + upper) / 2;
    const pct = ((upper - lower) / mid) * 50; // half-width percent ≈ ±pct
    if (!Number.isFinite(pct) || pct <= 0) continue;
    const usd = Number(w?.total_usd ?? 0);
    const addr = String(w?.wallet_address || '').trim();
    ranges.push({ pct: Math.round(pct * 100) / 100, usd, addr, lower, upper });
  }
  ranges.sort((a, b) => b.usd - a.usd);
  // deduplicate similar ranges (within 0.3%)
  const unique = [];
  for (const r of ranges) {
    if (unique.some((u) => Math.abs(u.pct - r.pct) < 0.3)) continue;
    unique.push(r);
    if (unique.length >= 4) break;
  }
  return unique;
}

function shortAddr(addr) {
  const s = String(addr || '');
  if (s.length <= 10) return s || '--';
  return `${s.slice(0, 6)}..${s.slice(-4)}`;
}

export default function OpenPositionModal({
  pool,
  chain,
  wallets,
  walletsLoading,
  selectedWalletId,
  onWalletSelect,
  onSubmit,
  onClose,
  busy,
}) {
  const [amount, setAmount] = useState('100');
  const [rangeLower, setRangeLower] = useState('2');
  const [rangeUpper, setRangeUpper] = useState('2');
  const [slippage, setSlippage] = useState('');
  const [error, setError] = useState('');

  const pair = pool?.trading_pair || '--';
  const addr = String(pool?.pool_address || '').trim();
  const version = String(pool?.protocol_version || pool?.factory_name || '').trim();

  const showWalletPicker = Array.isArray(wallets) && wallets.length > 1;
  const resolvedWalletId = useMemo(() => {
    if (!Array.isArray(wallets) || wallets.length === 0) return 0;
    if (wallets.length === 1) return wallets[0].id;
    if (selectedWalletId && wallets.some((w) => w.id === selectedWalletId)) return selectedWalletId;
    const def = wallets.find((w) => w.is_default);
    return def ? def.id : wallets[0].id;
  }, [wallets, selectedWalletId]);

  const smartRanges = useMemo(
    () => extractSmartMoneyRanges(pool?.smartMoneyWallets),
    [pool?.smartMoneyWallets]
  );
  const hasSmartRanges = smartRanges.length > 0;

  const applyRange = useCallback((lo, hi) => {
    setRangeLower(String(lo));
    setRangeUpper(String(hi));
  }, []);

  const handleSubmit = useCallback(() => {
    const amt = Number(amount);
    const rl = Number(rangeLower);
    const ru = Number(rangeUpper);
    const sl = Number(slippage);

    if (!Number.isFinite(amt) || amt <= 0) { setError('请输入有效的金额'); return; }
    if (!Number.isFinite(rl) || rl <= 0) { setError('请输入有效的下限范围'); return; }
    if (!Number.isFinite(ru) || ru <= 0) { setError('请输入有效的上限范围'); return; }
    if (showWalletPicker && !resolvedWalletId) { setError('请选择钱包'); return; }

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
    });
  }, [amount, rangeLower, rangeUpper, slippage, addr, version, chain, onSubmit, showWalletPicker, resolvedWalletId]);

  return (
    <div className="modal-overlay" onClick={onClose}>
      <div className="modal-box" onClick={(e) => e.stopPropagation()}>
        <div className="modal-header">
          <h3>⚡ 开仓</h3>
          <button type="button" className="modal-close" onClick={onClose}>&times;</button>
        </div>

        <div className="modal-pair">{pair}</div>
        <div className="modal-addr">{addr ? `${addr.slice(0, 10)}...${addr.slice(-8)}` : '--'}</div>

        {error && <div className="error-text">{error}</div>}

        {/* Wallet selector */}
        {walletsLoading && (
          <div className="wallet-picker-loading">加载钱包...</div>
        )}
        {showWalletPicker && !walletsLoading && (
          <div className="wallet-picker">
            <span className="wallet-picker-label">选择钱包</span>
            <div className="wallet-picker-list">
              {wallets.map((w) => {
                const active = w.id === resolvedWalletId;
                return (
                  <button
                    key={w.id}
                    type="button"
                    className={`wallet-chip ${active ? 'active' : ''}`}
                    onClick={() => onWalletSelect(w.id)}
                  >
                    <span className="wallet-chip-name">
                      {w.name || shortAddr(w.address)}
                      {w.is_default && <span className="wallet-chip-default">默认</span>}
                    </span>
                    <span className="wallet-chip-addr">{shortAddr(w.address)}</span>
                    <span className="wallet-chip-bal">
                      {w.native_balance !== 'N/A' ? `${w.native_balance}` : ''}{' '}
                      {w.stable_balance !== 'N/A' ? `/ $${w.stable_balance}` : ''}
                    </span>
                  </button>
                );
              })}
            </div>
          </div>
        )}

        <div className="modal-form">
          <label className="modal-field">
            <span>金额 (USDT)</span>
            <input type="number" value={amount} onChange={(e) => setAmount(e.target.value)} min="1" step="10" />
          </label>

          {/* Range quick picks */}
          <div className="modal-range-section">
            <span className="modal-range-label">快捷区间</span>
            <div className="modal-range-picks">
              {hasSmartRanges ? (
                smartRanges.map((sr, i) => {
                  const pctDisplay = sr.pct.toFixed(sr.pct >= 10 ? 0 : 1);
                  const isActive = Math.abs(Number(rangeLower) - sr.pct) < 0.05 && Math.abs(Number(rangeUpper) - sr.pct) < 0.05;
                  return (
                    <button
                      key={i}
                      type="button"
                      className={`range-chip smart ${isActive ? 'active' : ''}`}
                      onClick={() => applyRange(sr.pct, sr.pct)}
                      title={`${shortAddr(sr.addr)} $${Math.round(sr.usd)}`}
                    >
                      ±{pctDisplay}%
                      <span className="range-chip-sub">${sr.usd >= 1000 ? `${(sr.usd / 1000).toFixed(0)}K` : Math.round(sr.usd)}</span>
                    </button>
                  );
                })
              ) : (
                PRESET_RANGES.map((v) => {
                  const isActive = Math.abs(Number(rangeLower) - v) < 0.05 && Math.abs(Number(rangeUpper) - v) < 0.05;
                  return (
                    <button
                      key={v}
                      type="button"
                      className={`range-chip ${isActive ? 'active' : ''}`}
                      onClick={() => applyRange(v, v)}
                    >
                      ±{v}%
                    </button>
                  );
                })
              )}
            </div>
            {hasSmartRanges && (
              <div className="modal-range-hint">聪明钱区间 · 按金额排序</div>
            )}
          </div>

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
            <span>滑点 % (留空则使用全局配置)</span>
            <input type="number" value={slippage} onChange={(e) => setSlippage(e.target.value)} min="0.1" step="0.1" placeholder="默认全局配置" />
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
