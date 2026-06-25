import { AlertTriangle, Flame, Search, Settings, SlidersHorizontal, X } from 'lucide-react';
import PanelShell, { EmptyState } from './PanelShell';
import NumberFlowValue from './NumberFlowValue';
import { Button, IconButton, Input, Slider } from './ui';
import {
  computeHotPoolActiveFeeRate,
  formatPriceDisplay,
  formatUsdCompact,
  normalizePoolAddress,
  normalizeTokenRisk,
  parseHotPoolBadges,
  resolveHotPoolFilterToken,
  shortAddress,
  tokenRiskLabel,
  tokenRiskSummary,
  tokenRiskToneClass,
} from '../utils';
import flashIcon from '../img/flash.svg';

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

function resolveHotPoolMarketCapDisplay(pool) {
  const candidates = [
    pool?.fdv_usd,
    pool?.current_token_fdv_usd,
    pool?.market_cap_usd,
  ];
  for (const candidate of candidates) {
    const value = parseMetricNumber(candidate);
    if (Number.isFinite(value) && value > 0) return value;
  }
  return NaN;
}

function resolveHotPoolMarketCapLabel(pool) {
  const fdv = parseMetricNumber(pool?.fdv_usd);
  if (Number.isFinite(fdv) && fdv > 0) return 'FDV';
  const legacyFDV = parseMetricNumber(pool?.current_token_fdv_usd);
  if (Number.isFinite(legacyFDV) && legacyFDV > 0) return 'FDV';
  return '市值';
}

function formatFixedFeePercent(value) {
  const num = Number(value || 0);
  if (!Number.isFinite(num) || num <= 0 || num > 100) return '';
  return `${num.toFixed(4)}%`;
}

function formatPoolTradingFee(pool) {
  if (pool?.fee_dynamic) return '动态';
  return formatFixedFeePercent(pool?.fee_percentage);
}

function formatCompactCount(value) {
  const num = Number(value);
  if (!Number.isFinite(num)) return '--';
  if (num <= 0) return '0笔';
  const abs = Math.abs(num);
  if (abs >= 1000000000) return `${(num / 1000000000).toFixed(abs >= 10000000000 ? 0 : 1).replace(/\.0$/, '')}B笔`;
  if (abs >= 1000000) return `${(num / 1000000).toFixed(abs >= 10000000 ? 0 : 1).replace(/\.0$/, '')}M笔`;
  if (abs >= 1000) return `${(num / 1000).toFixed(abs >= 10000 ? 0 : 1).replace(/\.0$/, '')}K笔`;
  return `${Math.round(num)}笔`;
}

function formatDeltaNumber(value, mode = 'usd') {
  const num = Number(value);
  if (!Number.isFinite(num)) return '';
  if (mode === 'rate') return `${num.toFixed(3)}%`;
  if (mode === 'count') return formatCompactCount(num).replace(/[^\d.KMB]+$/u, '');
  return formatUsdCompact(num).replace('$', '');
}

function splitUsdCompact(value) {
  const text = formatUsdCompact(value);
  if (text === '$--') return { symbol: '$', value: '--' };
  if (text.startsWith('-$')) return { symbol: '-$', value: text.slice(2) };
  if (text.startsWith('$')) return { symbol: '$', value: text.slice(1) };
  return { symbol: '', value: text };
}

function HotPoolPrimaryMetric({ mode, value }) {
  if (mode === 'rate') {
    const num = Number(value);
    const text = Number.isFinite(num) && num > 0 ? `${num.toFixed(3)}%` : '--';
    return <span className="pool-main-number">{text}</span>;
  }

  const parts = splitUsdCompact(value);
  return (
    <span className="pool-main-amount">
      {parts.symbol ? <span className="pool-main-symbol">{parts.symbol}</span> : null}
      <span className="pool-main-number">{parts.value}</span>
    </span>
  );
}

function MetricDelta({ currentValue, previousValue, mode = 'usd' }) {
  const current = Number(currentValue);
  const previous = Number(previousValue);
  if (!Number.isFinite(current) || !Number.isFinite(previous)) return null;
  const diff = current - previous;
  if (!Number.isFinite(diff) || diff === 0) return null;
  const up = diff > 0;
  const text = formatDeltaNumber(Math.abs(diff), mode);
  if (!text) return null;

  return (
    <span className={`pool-metric-delta ${up ? 'up' : 'down'}`} title={`${up ? '+' : '-'}${text}`}>
      <span className="pool-metric-delta-arrow">{up ? '↑' : '↓'}</span>
      <NumberFlowValue value={Math.abs(diff)} formatter={() => text} />
    </span>
  );
}

function HotPoolsHeightSettings({
  open,
  controlRef,
  active,
  panelHeight,
  minPanelHeight,
  maxPanelHeight,
  onToggle,
  onClose,
  onHeightChange,
  onReset,
}) {
  return (
    <div className="settings-wrap" ref={controlRef}>
      <IconButton
        type="button"
        className={`icon-link ${active ? 'active' : ''}`}
        active={active}
        onClick={onToggle}
        title={`热门池子高度 ${panelHeight}px`}
        aria-label="热门池子高度"
      >
        <Settings size={14} />
      </IconButton>

      {open ? (
        <div className="popover kline-settings-popover panel-height-popover">
          <div className="kline-filter-popover-head">
            <div>
              <div className="kline-filter-popover-title">热门池子高度</div>
              <div className="kline-filter-popover-sub">仅保存在当前浏览器</div>
            </div>
            <IconButton
              type="button"
              className="icon-link"
              onClick={onClose}
              title="Close"
              aria-label="Close"
            >
              <X size={14} />
            </IconButton>
          </div>

          <div className="kline-height-value">{panelHeight}px</div>

          <Slider
            className="kline-height-slider"
            min={minPanelHeight}
            max={maxPanelHeight}
            step={20}
            value={[panelHeight]}
            onValueChange={([value]) => onHeightChange(value)}
          />

          <label className="kline-filter-field">
            <span>高度</span>
            <div className="kline-height-input-row">
              <Input
                type="number"
                min={minPanelHeight}
                max={maxPanelHeight}
                step="20"
                inputMode="numeric"
                value={panelHeight}
                onChange={(e) => {
                  const nextValue = Number(e.target.value);
                  if (!Number.isFinite(nextValue)) return;
                  onHeightChange(nextValue);
                }}
              />
              <span className="kline-height-unit">px</span>
            </div>
          </label>

          <div className="kline-filter-actions">
            <button type="button" className="ghost-chip" onClick={onReset}>
              默认
            </button>
          </div>
        </div>
      ) : null}
    </div>
  );
}

function HotPoolsFilterPopover({
  draft,
  filterDefaults,
  riskFilterOptions,
  onDraftChange,
  onApply,
  onReset,
  onClear,
  onClose,
}) {
  return (
    <div className="popover kline-filter-popover hot-pools-filter-popover">
      <div className="kline-filter-popover-head">
        <div>
          <div className="kline-filter-popover-title">热门池子筛选</div>
          <div className="kline-filter-popover-sub">仅筛选当前已加载的热门池子</div>
        </div>
        <IconButton
          type="button"
          className="icon-link"
          onClick={onClose}
          title="Close"
          aria-label="Close"
        >
          <X size={14} />
        </IconButton>
      </div>

      <div className="hot-pools-filter-toggle-row">
        <div className="hot-pools-filter-toggle-copy">
          <span className="hot-pools-filter-toggle-label">筛选状态</span>
          <span className="hot-pools-filter-toggle-state">
            {draft.enabled ? '已启用，应用后按下方条件筛选' : '已关闭，条件会保留但不会生效'}
          </span>
        </div>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          active={draft.enabled}
          className={`ghost-chip ${draft.enabled ? 'active' : ''}`}
          onClick={() => onDraftChange((prev) => ({ ...prev, enabled: !prev.enabled }))}
          aria-pressed={draft.enabled}
          title={draft.enabled ? '关闭筛选' : '启用筛选'}
        >
          {draft.enabled ? '已启用' : '已关闭'}
        </Button>
      </div>

      <label className="kline-filter-field">
        <span>搜索 (交易对 / 地址)</span>
        <Input
          value={draft.keyword}
          onChange={(e) => onDraftChange((prev) => ({ ...prev, keyword: e.target.value }))}
          placeholder="例如 USDT"
        />
      </label>

      <div className="hot-pools-filter-grid">
        <label className="kline-filter-field">
          <span>手续费 ≥ (USD)</span>
          <Input
            value={draft.minFees}
            onChange={(e) => onDraftChange((prev) => ({ ...prev, minFees: e.target.value }))}
            inputMode="decimal"
            placeholder={String(filterDefaults.minFees)}
          />
        </label>

        <label className="kline-filter-field">
          <span>费用率 ≥ (%)</span>
          <Input
            value={draft.minFeeRate}
            onChange={(e) => onDraftChange((prev) => ({ ...prev, minFeeRate: e.target.value }))}
            inputMode="decimal"
            placeholder={String(filterDefaults.minFeeRate)}
          />
        </label>

        <label className="kline-filter-field">
          <span>排除费率 &gt; (%)</span>
          <Input
            value={draft.maxFeeRate}
            onChange={(e) => onDraftChange((prev) => ({ ...prev, maxFeeRate: e.target.value }))}
            inputMode="decimal"
            placeholder="可选"
          />
        </label>

        <label className="kline-filter-field">
          <span>活跃费率 ≥ (%)</span>
          <Input
            value={draft.minActiveFeeRate}
            onChange={(e) => onDraftChange((prev) => ({ ...prev, minActiveFeeRate: e.target.value }))}
            inputMode="decimal"
            placeholder="可选"
          />
        </label>

        <label className="kline-filter-field">
          <span>TVL ≥ (USD)</span>
          <Input
            value={draft.minTvl}
            onChange={(e) => onDraftChange((prev) => ({ ...prev, minTvl: e.target.value }))}
            inputMode="decimal"
            placeholder={String(filterDefaults.minTvl)}
          />
        </label>

        <label className="kline-filter-field">
          <span>FDV ≥ (USD)</span>
          <Input
            value={draft.minMarketCap}
            onChange={(e) => onDraftChange((prev) => ({ ...prev, minMarketCap: e.target.value }))}
            inputMode="decimal"
            placeholder="可选"
          />
        </label>

        <label className="kline-filter-field">
          <span>交易量 ≥ (USD)</span>
          <Input
            value={draft.minVolume}
            onChange={(e) => onDraftChange((prev) => ({ ...prev, minVolume: e.target.value }))}
            inputMode="decimal"
            placeholder={String(filterDefaults.minVolume)}
          />
        </label>

        <label className="kline-filter-field">
          <span>交易笔数 ≥</span>
          <Input
            value={draft.minTxCount}
            onChange={(e) => onDraftChange((prev) => ({ ...prev, minTxCount: e.target.value }))}
            inputMode="decimal"
            placeholder="可选"
          />
        </label>

        <label className="kline-filter-field hot-pools-filter-field-wide">
          <span>OKX 低流动性</span>
          <select
            value={draft.riskFilter}
            onChange={(e) => onDraftChange((prev) => ({ ...prev, riskFilter: e.target.value }))}
          >
            {riskFilterOptions.map((item) => (
              <option key={item.key} value={item.key}>{item.label}</option>
            ))}
          </select>
        </label>
      </div>

      <div className="kline-filter-actions">
        <Button type="button" variant="primary" size="sm" className="ghost-chip active" onClick={onApply}>
          应用
        </Button>
        <Button type="button" variant="ghost" size="sm" className="ghost-chip" onClick={onReset}>
          默认
        </Button>
        <Button type="button" variant="ghost" size="sm" className="ghost-chip" onClick={onClear}>
          清空条件
        </Button>
      </div>
    </div>
  );
}

function HotPoolRow({
  pool,
  previousData,
  index,
  chain,
  hotSort,
  hotInlineSort,
  hotTokenFilter,
  selectedPoolAddress,
  getDexIcon,
  onCopyAddress,
  onSelectPool,
  onOpenPosition,
  onInlineSortChange,
  onToggleTokenFilter,
}) {
  const addr = normalizePoolAddress(pool?.pool_address || '');
  const selected = selectedPoolAddress && addr === selectedPoolAddress;
  const feePctText = formatPoolTradingFee(pool);
  const feeRate = Number(pool?.fee_rate || 0);
  const volume = Number(pool?.total_volume || 0);
  const totalFees = Number(pool?.total_fees || 0);
  const tvl = Number(pool?.current_pool_value || 0);
  const marketCap = resolveHotPoolMarketCapDisplay(pool);
  const marketCapLabel = resolveHotPoolMarketCapLabel(pool);
  const marketCapAvailable = Number.isFinite(marketCap) && marketCap > 0;
  const activeLiquidityFeeRate = computeHotPoolActiveFeeRate(pool);
  const txCount = Number(pool?.transaction_count || 0);
  const priceDisplay = String(pool?.price_display || '');
  const feeRateAvailable = Number.isFinite(tvl) && tvl > 0 && Number.isFinite(feeRate);
  const feeRateText = feeRateAvailable ? `${feeRate.toFixed(3)}%` : '--';
  const activeLiquidityFeeRateAvailable = Number.isFinite(activeLiquidityFeeRate);
  const activeLiquidityFeeRateText = activeLiquidityFeeRateAvailable ? `${activeLiquidityFeeRate.toFixed(3)}%` : '--';
  const mainDeltaMode = hotSort === 'fee_rate' ? 'rate' : 'usd';
  const mainDeltaCurrent = hotSort === 'volume' ? volume : hotSort === 'fee_rate' ? feeRate : totalFees;
  const mainDeltaPrevious = hotSort === 'volume'
    ? previousData?.total_volume
    : hotSort === 'fee_rate'
      ? previousData?.fee_rate
      : previousData?.total_fees;
  const factoryName = String(pool?.factory_name || pool?.dex || '');
  const userPosUsd = Number(pool?.userPositionUsd || 0);
  const pair = String(pool?.trading_pair || '--');
  const pairInitials = pair.split(/[\/-]/).map((s) => s.trim().charAt(0).toUpperCase()).join('').slice(0, 2);
  const protocolVersion = String(pool?.protocol_version || '').trim().toUpperCase();
  const displayTokenLogoUrl = String(pool?.display_token_logo_url || '').trim();
  const displayTokenSymbol = String(pool?.display_token_symbol || '').trim();
  const avatarLabel = (displayTokenSymbol || pairInitials || 'LP').slice(0, 4).toUpperCase();
  const dex = getDexIcon(factoryName);
  const protocolTagText = protocolVersion || dex?.label || '';
  const avatarSrc = displayTokenLogoUrl;
  const filterToken = resolveHotPoolFilterToken(pool);
  const avatarFilterActive = filterToken && hotTokenFilter?.address === filterToken.address;
  const badges = parseHotPoolBadges(pool?.badges);
  const tokenRisk = normalizeTokenRisk(pool?.token_risk);
  const tokenRiskTone = tokenRiskToneClass(tokenRisk);
  const hasLowLiquidityRisk = Boolean(tokenRisk?.has_low_liquidity);
  const isHighFeeRate = feeRate >= 1;

  return (
    <div
      key={`${pool?.protocol_version || ''}:${addr || index}`}
      className={`pool-row ${selected ? 'selected' : ''} ${isHighFeeRate ? 'high-fee' : ''} ${hasLowLiquidityRisk ? 'low-liquidity' : ''}`}
      onClick={() => onSelectPool({ ...pool, chain }, chain)}
    >
      <button
        type="button"
        className={`pool-avatar ${filterToken ? 'filterable' : ''} ${avatarFilterActive ? 'active' : ''}`}
        title={filterToken ? `筛选 ${filterToken.symbol} 的池子` : '该池子无法按单一非稳定币筛选'}
        onClick={(e) => {
          if (!filterToken) return;
          e.stopPropagation();
          onToggleTokenFilter(filterToken);
        }}
      >
        {avatarSrc ? (
          <>
            <img
              src={avatarSrc}
              alt=""
              onError={(e) => {
                e.currentTarget.style.display = 'none';
                const fallback = e.currentTarget.parentElement?.querySelector('.pool-avatar-fallback');
                if (fallback) fallback.style.display = 'flex';
              }}
            />
            <span className="pool-avatar-fallback" style={{ display: 'none' }}>{avatarLabel}</span>
          </>
        ) : <span>{avatarLabel}</span>}
      </button>

      <div className="pool-info">
        <div className="pool-name-line">
          <span className="pool-name">{pair}</span>
          <button type="button" className="copy-tiny" onClick={(e) => { e.stopPropagation(); onCopyAddress(addr); }} title="复制地址">
            <svg viewBox="0 0 24 24" fill="currentColor" width="11" height="11"><path d="M16 1H4a2 2 0 00-2 2v14h2V3h12V1zm3 4H8a2 2 0 00-2 2v14a2 2 0 002 2h11a2 2 0 002-2V7a2 2 0 00-2-2zm0 16H8V7h11v14z"/></svg>
          </button>
          {feePctText && <span className="tag tag-blue"><NumberFlowValue value={feePctText} formatter={() => feePctText} /></span>}
          {protocolTagText && (
            <span className="tag tag-dex tag-dex-inline">
              {dex?.src ? <img src={dex.src} alt="" /> : null}
              <span>{protocolTagText}</span>
            </span>
          )}
          {userPosUsd > 0 && (
            <span className="tag tag-purple">
              持仓 <NumberFlowValue value={userPosUsd} formatter={(v) => formatUsdCompact(v)} />
            </span>
          )}
          {tokenRisk ? (
            <span
              className={`tag pool-risk-chip is-${tokenRiskTone} ${hasLowLiquidityRisk ? 'is-low-liquidity' : ''}`}
              title={tokenRiskSummary(tokenRisk)}
            >
              {hasLowLiquidityRisk ? <AlertTriangle size={11} strokeWidth={2.4} aria-hidden="true" /> : null}
              {tokenRiskLabel(tokenRisk)}
            </span>
          ) : null}
        </div>
        {badges.length > 0 && (
          <div className="pool-badge-line">
            {badges.map((badge, badgeIdx) => (
              <span
                key={`${badge.text}:${badgeIdx}`}
                className={`tag tag-badge pool-badge-chip ${
                  /(^|[\s·])(?:币安\s*alpha|binance\s*alpha|alpha)(?:$|[\s·])/i.test(String(badge.text || '')) ? 'pool-badge-chip-alpha' : ''
                }`}
                data-tip={badge.tip}
                aria-label={badge.tip}
                tabIndex={0}
              >
                <span>{badge.text}</span>
              </span>
            ))}
          </div>
        )}
      </div>

      <div className="pool-meta-line">
          <button
            type="button"
            className={`pool-meta-sort meta-cyan ${hotInlineSort === 'volume' ? 'active' : ''}`}
            onClick={(e) => {
              e.stopPropagation();
              onInlineSortChange((prev) => (prev === 'volume' ? '' : 'volume'));
            }}
            title="按 Vol 降序排序"
            aria-pressed={hotInlineSort === 'volume'}
          >
            <span>Vol</span>
            <b>
              <NumberFlowValue value={volume} formatter={(v) => formatUsdCompact(v)} />
              <MetricDelta currentValue={volume} previousValue={previousData?.total_volume} />
            </b>
          </button>
          <span className="dot-sep" />
          <button
            type="button"
            className={`pool-meta-sort meta-cyan ${hotInlineSort === 'tvl' ? 'active' : ''}`}
            onClick={(e) => {
              e.stopPropagation();
              onInlineSortChange((prev) => (prev === 'tvl' ? '' : 'tvl'));
            }}
            title="按 TVL 降序排序"
            aria-pressed={hotInlineSort === 'tvl'}
          >
            <span>TVL</span>
            <b>
              <NumberFlowValue value={tvl} formatter={(v) => formatUsdCompact(v)} />
              <MetricDelta currentValue={tvl} previousValue={previousData?.current_pool_value} />
            </b>
          </button>
          {marketCapAvailable ? (
            <>
              <span className="dot-sep" />
              <span className="pool-meta-static meta-green">
                <span>{marketCapLabel}</span>
                <b>
                  <NumberFlowValue value={marketCap} formatter={(v) => formatUsdCompact(v)} />
                  <MetricDelta currentValue={marketCap} previousValue={resolveHotPoolMarketCapDisplay(previousData)} />
                </b>
              </span>
            </>
          ) : null}
          <span className="dot-sep" />
          <button
            type="button"
            className={`pool-meta-sort meta-orange ${hotInlineSort === 'tx_count' ? 'active' : ''}`}
            onClick={(e) => {
              e.stopPropagation();
              onInlineSortChange((prev) => (prev === 'tx_count' ? '' : 'tx_count'));
            }}
            title="按笔数降序排序"
            aria-label="按交易笔数降序排序"
            aria-pressed={hotInlineSort === 'tx_count'}
          >
            <b>
              <NumberFlowValue value={txCount} formatter={formatCompactCount} />
              <MetricDelta currentValue={txCount} previousValue={previousData?.transaction_count} mode="count" />
            </b>
          </button>
          <span className="dot-sep" />
          <button
            type="button"
            className={`pool-meta-sort meta-accent ${hotInlineSort === 'fee_rate' ? 'active' : ''} ${feeRateAvailable ? '' : 'muted'}`}
            onClick={(e) => {
              e.stopPropagation();
              onInlineSortChange((prev) => (prev === 'fee_rate' ? '' : 'fee_rate'));
            }}
            title="按费率降序排序"
            aria-pressed={hotInlineSort === 'fee_rate'}
          >
            <span>费率</span>
            <b>
              {feeRateAvailable ? (
                <>
                  <NumberFlowValue value={feeRate} formatter={(v) => `${Number(v).toFixed(3)}%`} />
                  <MetricDelta currentValue={feeRate} previousValue={previousData?.fee_rate} mode="rate" />
                </>
              ) : '--'}
            </b>
          </button>
          <span className="dot-sep" />
          <button
            type="button"
            className={`pool-meta-sort meta-gold ${hotInlineSort === 'active_fee_rate' ? 'active' : ''} ${activeLiquidityFeeRateAvailable ? '' : 'muted'}`}
            onClick={(e) => {
              e.stopPropagation();
              onInlineSortChange((prev) => (prev === 'active_fee_rate' ? '' : 'active_fee_rate'));
            }}
            title="按活跃降序排序"
            aria-pressed={hotInlineSort === 'active_fee_rate'}
          >
            <span>活跃</span>
            <b>
              {activeLiquidityFeeRateAvailable ? (
                <>
                  <NumberFlowValue value={activeLiquidityFeeRate} formatter={() => activeLiquidityFeeRateText} />
                  <MetricDelta
                    currentValue={activeLiquidityFeeRate}
                    previousValue={computeHotPoolActiveFeeRate(previousData)}
                    mode="rate"
                  />
                </>
              ) : activeLiquidityFeeRateText}
            </b>
          </button>
      </div>

      <div className="pool-values">
        <div className="pool-main-val">
          <HotPoolPrimaryMetric
            mode={hotSort === 'fee_rate' ? 'rate' : 'usd'}
            value={hotSort === 'volume' ? volume : hotSort === 'fee_rate' ? feeRate : totalFees}
          />
          <MetricDelta currentValue={mainDeltaCurrent} previousValue={mainDeltaPrevious} mode={mainDeltaMode} />
        </div>
        {priceDisplay ? (
          <div className={`pool-sub-val ${priceDisplay.includes('↑') || priceDisplay.includes('+') ? 'up' : priceDisplay.includes('↓') || priceDisplay.includes('-') ? 'down' : ''}`} title={priceDisplay}>
            <NumberFlowValue value={priceDisplay} formatter={() => formatPriceDisplay(priceDisplay)} />
          </div>
        ) : hotSort !== 'fee_rate' ? (
          <div className={`pool-sub-val purple ${feeRateAvailable ? '' : 'muted'}`}>
            {feeRateAvailable ? (
              <NumberFlowValue value={feeRate} formatter={() => feeRateText} />
            ) : feeRateText}
          </div>
        ) : null}
      </div>

      <button
        type="button"
        className="pool-buy-btn"
        aria-label="开仓"
        onClick={(e) => { e.stopPropagation(); onOpenPosition({ ...pool, chain, panelKey: 'hot_pools' }); }}
      >
        <img src={flashIcon} alt="" className="open-lightning-icon" aria-hidden="true" />
        <span className="open-buy-text">开仓</span>
      </button>
    </div>
  );
}

export default function HotPoolsPanel({
  workMode,
  displayLimit,
  minPanelHeight,
  maxPanelHeight,
  panelHeight,
  heightSettingsOpen,
  panelHeightCustomized,
  heightControlRef,
  filterRef,
  hotPools,
  previousHotPoolsMap,
  filteredHotPools,
  hotPoolsLoading,
  hotPoolsError,
  hotPoolsUpdatedAt,
  hotSort,
  hotInlineSort,
  hotPoolsFilterOpen,
  hotPoolsFilterEnabled,
  hotPoolsFilterDraft,
  hotPoolsFilterDefaults,
  hotPoolsRiskFilterOptions,
  hotTokenFilter,
  keyword,
  searchOpen,
  selectedPoolAddress,
  chain,
  getDexIcon,
  onPanelHeightToggle,
  onPanelHeightClose,
  onPanelHeightChange,
  onPanelHeightReset,
  onHotSortChange,
  onHotInlineSortChange,
  onSearchToggle,
  onKeywordChange,
  onFilterToggle,
  onFilterClose,
  onFilterDraftChange,
  onFilterApply,
  onFilterReset,
  onFilterClear,
  onTokenFilterClear,
  onToggleTokenFilter,
  onCopyAddress,
  onSelectPool,
  onOpenPosition,
  operationProgress,
}) {
  return (
    <PanelShell
      title="热门池子"
      subtitle={`支持搜索与排序 · 展示前 ${displayLimit} 条`}
      icon={Flame}
      actions={!workMode ? (
        <HotPoolsHeightSettings
          open={heightSettingsOpen}
          controlRef={heightControlRef}
          active={heightSettingsOpen || panelHeightCustomized}
          panelHeight={panelHeight}
          minPanelHeight={minPanelHeight}
          maxPanelHeight={maxPanelHeight}
          onToggle={onPanelHeightToggle}
          onClose={onPanelHeightClose}
          onHeightChange={onPanelHeightChange}
          onReset={onPanelHeightReset}
        />
      ) : null}
    >
      <div className="hot-pools-toolbar-shell" ref={filterRef}>
        <div className="sort-tabs">
          {[{ key: 'fees', label: 'Fees' }, { key: 'fee_rate', label: 'Fee Rate' }, { key: 'volume', label: 'Volume' }].map((item) => (
            <Button
              type="button"
              key={item.key}
              variant="ghost"
              size="sm"
              active={hotSort === item.key}
              className={`sort-tab ${hotSort === item.key ? 'active' : ''}`}
              onClick={() => onHotSortChange(item.key)}
            >
              {item.label}
            </Button>
          ))}
          <IconButton
            type="button"
            className={`sort-tab icon-only search-toggle ${searchOpen ? 'active' : ''}`}
            active={searchOpen}
            onClick={onSearchToggle}
            title="搜索池子"
            aria-label="搜索池子"
          >
            <Search size={12} />
          </IconButton>
          <IconButton
            type="button"
            className={`sort-tab icon-only filter-toggle ${hotPoolsFilterEnabled ? 'active' : ''}`}
            active={hotPoolsFilterEnabled}
            onClick={onFilterToggle}
            title="筛选池子"
            aria-label="筛选池子"
          >
            <SlidersHorizontal size={12} />
            {hotPoolsFilterEnabled ? <span className="hot-filter-dot" /> : null}
          </IconButton>
        </div>

        {hotPoolsFilterOpen ? (
          <HotPoolsFilterPopover
            draft={hotPoolsFilterDraft}
            filterDefaults={hotPoolsFilterDefaults}
            riskFilterOptions={hotPoolsRiskFilterOptions}
            onDraftChange={onFilterDraftChange}
            onApply={onFilterApply}
            onReset={onFilterReset}
            onClear={onFilterClear}
            onClose={onFilterClose}
          />
        ) : null}
      </div>

      {searchOpen && (
        <div className="search-row">
          <Search size={14} />
          <input value={keyword} onChange={(e) => onKeywordChange(e.target.value)} placeholder="搜索交易对/地址" autoFocus />
        </div>
      )}

      {hotPoolsError ? <div className="error-text">{hotPoolsError}</div> : null}
      {hotTokenFilter?.address ? (
        <div className="hot-token-filter-bar">
          <span className="hot-token-filter-chip">
            同币筛选: {hotTokenFilter.symbol || shortAddress(hotTokenFilter.address, 6, 4)}
          </span>
          <button type="button" className="mini-link accent" onClick={onTokenFilterClear}>
            清除
          </button>
        </div>
      ) : null}
      {hotPoolsFilterEnabled && !hotPoolsLoading && hotPools.length > 0 && filteredHotPools.length === 0 ? (
        <div className="hot-filter-empty-note">
          当前筛选条件下没有可展示的热门池子，调整筛选或清空条件后再试。
        </div>
      ) : null}

      <div className="data-list">
        {hotPoolsLoading && filteredHotPools.length === 0 ? (
          <EmptyState text="正在加载热门池子..." />
        ) : filteredHotPools.length === 0 ? (
          <EmptyState text="暂无可展示的池子数据" />
        ) : (
          filteredHotPools.slice(0, displayLimit).map((pool, index) => {
            const poolAddress = normalizePoolAddress(pool?.pool_address || pool?.pool_id);
            return (
              <HotPoolRow
                key={`${pool?.protocol_version || ''}:${poolAddress || index}`}
                pool={pool}
                previousData={poolAddress ? previousHotPoolsMap?.[poolAddress] : null}
                index={index}
                chain={chain}
                hotSort={hotSort}
                hotInlineSort={hotInlineSort}
                hotTokenFilter={hotTokenFilter}
                selectedPoolAddress={selectedPoolAddress}
                getDexIcon={getDexIcon}
                onCopyAddress={onCopyAddress}
                onSelectPool={onSelectPool}
                onOpenPosition={onOpenPosition}
                onInlineSortChange={onHotInlineSortChange}
                onToggleTokenFilter={onToggleTokenFilter}
              />
            );
          })
        )}
      </div>

      <div className="panel-footnote">
        更新时间: {hotPoolsUpdatedAt ? new Date(hotPoolsUpdatedAt).toLocaleTimeString() : '--'}
      </div>
      {operationProgress}
    </PanelShell>
  );
}
