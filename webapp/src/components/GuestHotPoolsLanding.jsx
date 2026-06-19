import { Flame, LockKeyhole, RefreshCw } from 'lucide-react';
import LoginCodePanel from './LoginCodePanel';
import { formatUsdCompact, formatUtc8Time, normalizeTokenRisk } from '../utils';
import telegramLogo from '../img/telegram.svg';
import bnbLogo from '../img/bnb.svg';
import baseLogo from '../img/base.svg';
import uniswapLogo from '../img/uniswap.svg';
import pancakeLogo from '../img/pancake.svg';

const GUEST_HOT_POOLS_DISPLAY_LIMIT = 8;

function parseMetricNumber(value) {
  if (value === null || value === undefined || value === '') return NaN;
  const raw = typeof value === 'string' ? value.replace(/,/g, '').trim() : value;
  const direct = Number(raw);
  if (Number.isFinite(direct)) return direct;
  const match = String(value).match(/-?\d+(\.\d+)?/);
  if (!match) return NaN;
  const parsed = Number(match[0]);
  return Number.isFinite(parsed) ? parsed : NaN;
}

function getDexIcon(factoryName) {
  const name = String(factoryName || '').toLowerCase();
  if (name.includes('uniswap')) {
    const m = name.match(/v(\d+)/i);
    return { src: uniswapLogo, label: m ? `V${m[1]}` : '', color: '#ff007a' };
  }
  if (name.includes('pancake') || name.includes('pcs')) {
    const m = name.match(/v(\d+)/i);
    return { src: pancakeLogo, label: m ? `V${m[1]}` : '', color: '#d1884f' };
  }
  return null;
}

export default function GuestHotPoolsLanding({
  chain,
  hotPools,
  hotPoolsLoading,
  hotPoolsError,
  hotPoolsUpdatedAt,
  loginBusy,
  loginCode,
  loginError,
  onStartLogin,
  onCancelLogin,
  onChainChange,
}) {
  const rows = Array.isArray(hotPools) ? hotPools.slice(0, GUEST_HOT_POOLS_DISPLAY_LIMIT) : [];
  const totalFees = rows.reduce((sum, row) => {
    const value = parseMetricNumber(row?.total_fees);
    return Number.isFinite(value) ? sum + value : sum;
  }, 0);
  const totalVolume = rows.reduce((sum, row) => {
    const value = parseMetricNumber(row?.total_volume);
    return Number.isFinite(value) ? sum + value : sum;
  }, 0);
  const topFeeRate = rows.reduce((max, row) => {
    const value = parseMetricNumber(row?.fee_rate);
    return Number.isFinite(value) ? Math.max(max, value) : max;
  }, 0);

  return (
    <main className="guest-shell">
      <section className="guest-hero">
        <div className="guest-copy">
          <div className="guest-kicker">
            <Flame size={14} />
            实时热门池子预览
          </div>
          <h2>先看市场，再决定开仓。</h2>
          <p>
            未登录状态展示热门池子的部分榜单和核心交易指标。登录后可查看完整工作台、K 线、仓位管理和开仓弹框。
          </p>
          <div className="guest-actions">
            <button
              type="button"
              className="primary-btn guest-login-btn"
              onClick={onStartLogin}
              disabled={loginBusy}
            >
              {loginBusy ? <RefreshCw size={15} className="spin" /> : <img src={telegramLogo} alt="" />}
              获取 Telegram 登录码
            </button>
            <div className="guest-chain-switch" aria-label="切换链">
              <button
                type="button"
                className={`guest-chain-btn ${chain === 'bsc' ? 'active' : ''}`}
                onClick={() => onChainChange('bsc')}
              >
                <img src={bnbLogo} alt="" />
                BSC
              </button>
              <button
                type="button"
                className={`guest-chain-btn ${chain === 'base' ? 'active' : ''}`}
                onClick={() => onChainChange('base')}
              >
                <img src={baseLogo} alt="" />
                Base
              </button>
            </div>
          </div>
          {loginCode ? (
            <LoginCodePanel
              loginCode={loginCode}
              onCancel={onCancelLogin}
              className="guest-login-code"
            />
          ) : null}
          {loginError ? <div className="error-text guest-error">{loginError}</div> : null}
        </div>

        <div className="guest-market-board">
          <div className="guest-board-head">
            <div>
              <span className="guest-board-label">{chain === 'base' ? 'Base' : 'BSC'} 热门池子</span>
              <strong>热门池子榜</strong>
            </div>
            <div className="guest-board-time">
              {hotPoolsUpdatedAt ? formatUtc8Time(hotPoolsUpdatedAt, true) : '同步中'}
            </div>
          </div>

          <div className="guest-metrics">
            <div>
              <span>预览池子</span>
              <strong>{hotPoolsLoading && rows.length === 0 ? '--' : rows.length}</strong>
            </div>
            <div>
              <span>Fees</span>
              <strong>{rows.length ? formatUsdCompact(totalFees) : '--'}</strong>
            </div>
            <div>
              <span>Volume</span>
              <strong>{rows.length ? formatUsdCompact(totalVolume) : '--'}</strong>
            </div>
            <div>
              <span>最高费率</span>
              <strong>{topFeeRate > 0 ? `${topFeeRate.toFixed(3)}%` : '--'}</strong>
            </div>
          </div>

          <div className="guest-pool-list">
            <div className="guest-pool-list-head">
              <span>Pool</span>
              <span>5m Fees</span>
              <span>Vol</span>
              <span>Rate</span>
            </div>
            {hotPoolsLoading && rows.length === 0 ? (
              Array.from({ length: 6 }).map((_, index) => (
                <div className="guest-pool-row skeleton" key={`guest-skeleton-${index}`}>
                  <span />
                  <span />
                  <span />
                  <span />
                </div>
              ))
            ) : rows.length > 0 ? (
              rows.map((pool, index) => {
                const pair = String(pool?.trading_pair || '--');
                const factoryName = String(pool?.factory_name || pool?.dex || '');
                const dex = getDexIcon(factoryName);
                const protocolVersion = String(pool?.protocol_version || '').trim().toUpperCase();
                const protocolTagText = protocolVersion || (dex ? dex.label : '');
                const displayTokenLogoUrl = String(pool?.display_token_logo_url || '').trim();
                const displayTokenSymbol = String(pool?.display_token_symbol || '').trim();
                const pairInitials = pair.split(/[\/\-]/).map((s) => s.trim().charAt(0).toUpperCase()).join('').slice(0, 2);
                const avatarLabel = (displayTokenSymbol || pairInitials || 'LP').slice(0, 4).toUpperCase();
                const fees = parseMetricNumber(pool?.total_fees);
                const volume = parseMetricNumber(pool?.total_volume);
                const feeRate = parseMetricNumber(pool?.fee_rate);
                const txCount = parseMetricNumber(pool?.transaction_count);
                const tokenRisk = normalizeTokenRisk(pool?.token_risk);
                const hasLowLiquidityRisk = Boolean(tokenRisk?.has_low_liquidity);
                return (
                  <div
                    className={`guest-pool-row ${hasLowLiquidityRisk ? 'low-liquidity' : ''}`}
                    key={`${pool?.protocol_version || ''}:${pool?.pool_address || pool?.pool_id || index}`}
                  >
                    <div className="guest-pool-main">
                      <span className="guest-rank">{index + 1}</span>
                      <span className="guest-pool-avatar">
                        {displayTokenLogoUrl ? <img src={displayTokenLogoUrl} alt="" /> : avatarLabel}
                      </span>
                      <span className="guest-pool-name">
                        <strong>{pair}</strong>
                        <small>
                          {protocolTagText ? <span>{protocolTagText}</span> : null}
                          {Number.isFinite(txCount) && txCount > 0 ? <span>{Math.round(txCount).toLocaleString()} tx</span> : null}
                          {hasLowLiquidityRisk ? <span className="guest-risk">低流动性</span> : null}
                        </small>
                      </span>
                    </div>
                    <div className="guest-pool-value">{Number.isFinite(fees) ? formatUsdCompact(fees) : '--'}</div>
                    <div className="guest-pool-value muted">{Number.isFinite(volume) ? formatUsdCompact(volume) : '--'}</div>
                    <div className="guest-pool-value accent">{Number.isFinite(feeRate) ? `${feeRate.toFixed(3)}%` : '--'}</div>
                  </div>
                );
              })
            ) : (
              <div className="guest-empty">
                {hotPoolsError || '暂无可预览的热门池子数据'}
              </div>
            )}
          </div>

          <div className="guest-board-foot">
            <LockKeyhole size={14} />
            登录后开放完整榜单筛选、K 线、仓位和开仓。
          </div>
        </div>
      </section>
    </main>
  );
}
