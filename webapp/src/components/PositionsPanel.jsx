import React, { useMemo, useState } from 'react';
import { AlertTriangle, BriefcaseBusiness } from 'lucide-react';
import PanelShell, { EmptyState, MetricCard } from './PanelShell';
import NumberFlowValue from './NumberFlowValue';
import TaskActionMenu from './TaskActionMenu';
import {
  compactPrice,
  computePriceRange,
  formatDuration,
  formatNumber,
  formatUsd,
  formatUsdCompact,
  normalizePoolAddress,
  shortAddress,
} from '../utils';
import { TASK_MODE_OPTIONS, getTaskModeMeta, normalizeTaskMode } from '../taskModes';

const POSITION_SM_RANGE_LIMIT = 4;
const FEE_TIER_BY_TICK_SPACING = {
  1: 100,
  10: 500,
  50: 2500,
  60: 3000,
  100: 5000,
  200: 10000,
  2000: 20000,
};

function normalizeWalletAddress(value) {
  const raw = String(value || '').trim();
  if (!/^0x[0-9a-fA-F]{40}$/.test(raw)) return '';
  return `0x${raw.slice(2).toLowerCase()}`;
}

export function normalizePositionSmartMoneyGroups(groups) {
  return Array.isArray(groups)
    ? groups.filter((item) => Number(item?.range_percent) > 0)
    : [];
}

function formatPositionSmartMoneyRangePercent(value) {
  const num = Number(value);
  if (!Number.isFinite(num) || num <= 0) return '--';
  if (num >= 100) return `${Math.round(num)}%`;
  if (num >= 10) return `${num.toFixed(1).replace(/\.0$/, '')}%`;
  return `${num.toFixed(2).replace(/0+$/, '').replace(/\.$/, '')}%`;
}

function formatFixedFeePercent(value) {
  const num = Number(value || 0);
  if (!Number.isFinite(num) || num <= 0) return '';
  return `${num.toFixed(4)}%`;
}

function formatFeeTierPercent(feeTier, tickSpacing) {
  const bps = Number(feeTier || 0) || FEE_TIER_BY_TICK_SPACING[Number(tickSpacing)] || 0;
  if (!Number.isFinite(bps) || bps <= 0) return '';
  return formatFixedFeePercent(bps / 10000);
}

function buildPositionPairTitle(position, token0, token1) {
  const left = String(token0?.symbol || '').trim();
  const right = String(token1?.symbol || '').trim();
  if (left && right) return `${left}-${right}`;
  const rawTitle = String(position?.title || '').trim();
  if (!rawTitle) return '--';
  const parts = rawTitle.split('-').map((item) => String(item || '').trim()).filter(Boolean);
  if (parts.length >= 3) return parts.slice(1, -1).join('-');
  return rawTitle;
}

function PositionSmartMoneyRangeSummary({ groups }) {
  const [expanded, setExpanded] = useState(false);
  const validGroups = useMemo(() => normalizePositionSmartMoneyGroups(groups), [groups]);
  const visibleGroups = expanded ? validGroups : validGroups.slice(0, POSITION_SM_RANGE_LIMIT);
  const hiddenCount = Math.max(0, validGroups.length - visibleGroups.length);
  if (!validGroups.length) return null;
  return (
    <div className="pos-sm-ranges">
      <div className="pos-sm-ranges-head">
        <span className="pos-sm-ranges-title">聪明钱金额区间</span>
        <span className="pos-sm-ranges-count">{validGroups.length}档</span>
      </div>
      <div className="pos-sm-ranges-list">
        {visibleGroups.map((group, index) => (
          <div
            key={`${Number(group?.range_percent || 0)}:${Number(group?.position_count || 0)}:${index}`}
            className="pos-sm-range-chip"
          >
            <span className="pos-sm-range-chip-pct">{formatPositionSmartMoneyRangePercent(group?.range_percent)}</span>
            {Math.max(0, Number(group?.position_count) || 0) > 1 ? (
              <span className="pos-sm-range-chip-badge">{Number(group.position_count)}个</span>
            ) : null}
            <span className="pos-sm-range-chip-amount">{formatUsdCompact(group?.total_amount_usd)}</span>
          </div>
        ))}
      </div>
      {hiddenCount > 0 ? (
        <button type="button" className="pos-sm-ranges-toggle" onClick={() => setExpanded((prev) => !prev)}>
          {expanded ? '收起区间' : `更多区间 +${hiddenCount}`}
        </button>
      ) : null}
    </div>
  );
}

function PositionsSummary({ positions, walletBalances }) {
  const summary = positions?.summary || {};
  const multi = Array.isArray(walletBalances) && walletBalances.length > 1;
  const allWalletsUsd = multi
    ? walletBalances.reduce((s, w) => s + Number(w.stable_balance === 'N/A' ? 0 : w.stable_balance || 0), 0)
    : null;
  const walletUsd = allWalletsUsd !== null ? allWalletsUsd : (summary?.wallet_usd ?? 0);
  const totalUsd = walletUsd + Number(summary?.position_usd || 0) + Number(summary?.fee_usd || 0);
  const walletMetricCards = multi
    ? walletBalances.map((wb, idx) => ({
      key: String(wb?.id || wb?.address || idx),
      label: wb?.name || shortAddress(wb?.address || '', 6, 4) || `钱包 ${idx + 1}`,
      value: wb?.stable_balance !== 'N/A' ? formatUsd(wb.stable_balance) : '$--',
    }))
    : [
      {
        key: 'wallet-total',
        label: '钱包',
        value: formatUsd(walletUsd),
      },
    ];
  const summaryMetricCount = walletMetricCards.length + 3;
  return (
    <div className="summary-grid summary-grid-wallets" style={{ '--summary-wallet-cols': summaryMetricCount }}>
      <MetricCard label="总资产" value={formatUsd(totalUsd)} tone="strong" />
      {walletMetricCards.map((card) => (
        <MetricCard key={card.key} label={card.label} value={card.value} />
      ))}
      <MetricCard label="仓位" value={formatUsd(summary?.position_usd)} />
      <MetricCard label="手续费" value={formatUsd(summary?.fee_usd)} />
    </div>
  );
}

function PositionWarnings({ warnings }) {
  const rows = Array.from(
    new Set(
      (Array.isArray(warnings) ? warnings : [])
        .map((item) => String(item || '').trim())
        .filter(Boolean),
    ),
  );
  if (rows.length === 0) return null;
  return (
    <div className="mt-3">
      {rows.map((warning, index) => (
        <div key={`${warning}-${index}`} className="warning-box">
          <AlertTriangle size={14} />
          <span>{warning}</span>
        </div>
      ))}
    </div>
  );
}

function PositionCard({
  position,
  index,
  chain,
  walletMetaByKey,
  positionSmartMoneyRanges,
  taskActionPos,
  onTaskActionPosChange,
  onSelectPool,
  onTaskPause,
  onTaskStop,
  onTaskPartialExit,
  onTaskDelete,
  onTaskEditRange,
  onWithdrawLiquidity,
  onSwapDust,
  onTriggerRebalance,
  onUpdateTaskMode,
  onAddLiquidity,
  onCloseTaskActionMenu,
  getDexIcon,
}) {
  const p = position;
  const taskId = Number(p?.task_id || 0);
  const statusLabel = String(p?.status_label || '运行中');
  const pnl = Number(p?.absolute_pnl_usd || 0);
  const hasPnl = Boolean(p?.has_pnl) || Number.isFinite(pnl) && pnl !== 0;
  const totalVal = Number(p?.current_value_usd || p?.totals?.total_usd || 0);
  const inRange = Boolean(p?.in_range);
  const token0 = p?.token_rows?.[0];
  const token1 = p?.token_rows?.[1];
  const taskRangeLo = Number(p?.task_range_lower_pct);
  const taskRangeUp = Number(p?.task_range_upper_pct);
  const taskAmount = Number(p?.task_amount_usdt);
  const hasTaskRange = Number.isFinite(taskRangeLo) && taskRangeLo > 0 && Number.isFinite(taskRangeUp) && taskRangeUp > 0;
  const taskRangeSymmetric = hasTaskRange ? Math.abs(taskRangeLo - taskRangeUp) < 0.01 : false;
  const taskRangeHalfWidth = hasTaskRange ? ((taskRangeLo + taskRangeUp) / 2) : null;
  const taskRangeTotalWidth = hasTaskRange ? (taskRangeLo + taskRangeUp) : null;
  const tickSpacing = Number(p?.tick_spacing);
  const gridStepPct = Number.isFinite(tickSpacing) && tickSpacing > 0
    ? ((Math.pow(1.0001, tickSpacing) - 1) * 100)
    : null;
  const taskRangeLabel = hasTaskRange
    ? (taskRangeSymmetric
      ? `±${taskRangeHalfWidth.toFixed(2)}%`
      : `下 ${taskRangeLo.toFixed(2)}% / 上 ${taskRangeUp.toFixed(2)}%`)
    : '';
  const taskRangeSummary = hasTaskRange && Number.isFinite(taskRangeTotalWidth)
    ? `${taskRangeLabel}（总宽 ${taskRangeTotalWidth.toFixed(2)}%）`
    : '';
  const priceRange = computePriceRange(p);
  const poolAddress = normalizePoolAddress(p?.pool_id || p?.pool_address);
  const smartMoneyRangeGroups = poolAddress
    ? positionSmartMoneyRanges[poolAddress]?.groups
    : [];
  const positionWalletMeta = walletMetaByKey.get(`id:${Number(p?.wallet_id || 0)}`) ||
    walletMetaByKey.get(`addr:${normalizeWalletAddress(p?.wallet_address)}`);
  const positionWalletText = positionWalletMeta?.label ||
    shortAddress(normalizeWalletAddress(p?.wallet_address) || '', 6, 4) ||
    '默认钱包';
  const pairTitle = buildPositionPairTitle(p, token0, token1);
  const feeLabel = formatFeeTierPercent(p?.fee_tier, p?.tick_spacing);
  const dex = getDexIcon(`${String(p?.exchange || '').trim()} ${String(p?.version || '').trim()}`);
  const currentTaskMode = normalizeTaskMode(p?.task_mode, p?.task_paused);
  const currentTaskModeMeta = getTaskModeMeta(currentTaskMode);

  const statusClass = statusLabel.includes('错误') ? 'st-error' :
    statusLabel.includes('暂停') || statusLabel.includes('停止') || statusLabel.includes('撤出') ? 'st-warn' :
    statusLabel.includes('等待') ? 'st-wait' : 'st-ok';

  return (
    <div key={String(p?.position_id || index)} className="pos-card">
      <div className="pos-card-header">
        <div
          className="pos-card-left"
          onClick={() => onSelectPool({
            pool_id: p?.pool_id,
            pool_address: p?.pool_id,
            trading_pair: pairTitle,
            protocol_version: p?.version,
            factory_name: p?.exchange,
            token0_address: token0?.address,
            token1_address: token1?.address,
            token0_symbol: token0?.symbol,
            token1_symbol: token1?.symbol,
            fee_tier: p?.fee_tier,
            fee_percentage: Number(p?.fee_tier || 0) > 0 ? Number(p.fee_tier) / 10000 : 0,
            chain: p?.chain || chain,
          }, p?.chain || chain)}
        >
          <div className="pos-pair-row">
            {dex?.src ? (
              <span className="badge badge-dex pos-dex-tag" style={dex.color ? { '--pos-dex-color': dex.color } : undefined}>
                <img src={dex.src} alt="" />
                {dex.label ? <span>{dex.label}</span> : null}
              </span>
            ) : null}
            <span className="pos-pair-name">{pairTitle || shortAddress(p?.pool_id || '')}</span>
            {feeLabel ? <span className="badge badge-fee">{feeLabel}</span> : null}
          </div>
          <div className="pos-status-row">
            <span className={`status-pill ${statusClass}`}>
              <span className="status-dot" />
              {statusLabel}
            </span>
            <span className="pos-wallet-chip">钱包 {positionWalletText}</span>
            {taskId > 0 && <span className="pos-task-id">#{taskId}</span>}
            {taskId > 0 && <span className="pos-wallet-chip">{currentTaskModeMeta.shortLabel}</span>}
            <span className={`range-pill ${inRange ? 'in' : 'out'}`}>
              {inRange ? 'In Range' : 'Out'}
              {priceRange?.outOfRange && (
                <span className="range-pill-oor"> {priceRange.outOfRange.direction === 'above' ? '↑' : '↓'}{priceRange.outOfRange.pct.toFixed(1)}%</span>
              )}
            </span>
            {p?.running_since && <span className="pos-running-dur">{formatDuration(p.running_since)}</span>}
          </div>
        </div>
        <div className="pos-card-right-block">
          <div className="pos-metrics">
            <div className="pos-total">{formatUsd(totalVal)}</div>
            {hasPnl && (
              <div className={`pos-pnl ${pnl >= 0 ? 'positive' : 'negative'}`}>
                {pnl >= 0 ? '+' : ''}{formatNumber(pnl, 2)}
              </div>
            )}
          </div>
          {taskId > 0 && (
            <div className="pos-card-actions">
              <div className="pos-action-anchor">
                <button
                  type="button"
                  className="icon-btn-tiny"
                  onClick={(e) => { e.stopPropagation(); onTaskActionPosChange((prev) => prev?.task_id === p?.task_id ? null : p); }}
                  title="任务操作"
                  aria-label="任务操作"
                  aria-expanded={taskActionPos?.task_id === p?.task_id}
                >
                  <svg viewBox="0 0 24 24" fill="currentColor" width="14" height="14"><path d="M12 7a2 2 0 110-4 2 2 0 010 4zm0 7a2 2 0 110-4 2 2 0 010 4zm0 7a2 2 0 110-4 2 2 0 010 4z"/></svg>
                </button>
                {taskActionPos?.task_id === p?.task_id && (
                  <TaskActionMenu
                    position={taskActionPos}
                    onPause={onTaskPause}
                    onStop={onTaskStop}
                    onPartialExit={onTaskPartialExit}
                    onDelete={onTaskDelete}
                    onEditRange={onTaskEditRange}
                    onWithdrawLiquidity={onWithdrawLiquidity}
                    onSwapDust={onSwapDust}
                    onTriggerRebalance={onTriggerRebalance}
                    onAddLiquidity={onAddLiquidity}
                    onClose={onCloseTaskActionMenu}
                  />
                )}
              </div>
            </div>
          )}
        </div>
      </div>

      {taskId > 0 && (
        <div className="pos-action-bar">
          {TASK_MODE_OPTIONS.map((option) => (
            <button
              type="button"
              key={`${taskId}-${option.value}`}
              className={`pos-action-btn mode ${currentTaskMode === option.value ? 'active' : ''}`}
              title={option.description}
              onClick={() => onUpdateTaskMode(taskId, option.value)}
              disabled={statusLabel.includes('已停止') || statusLabel.includes('停止中') || statusLabel.includes('撤出中')}
              aria-pressed={currentTaskMode === option.value}
            >
              <span>{option.shortLabel}</span>
            </button>
          ))}
          <button type="button" className="pos-action-btn withdraw" title="取回流动性"
            onClick={() => onWithdrawLiquidity(taskId)}
            disabled={!p?.has_liquidity || statusLabel.includes('停止中') || statusLabel.includes('撤出中')}>
            <svg viewBox="0 0 24 24" fill="currentColor" width="14" height="14">
              <path d="M19 9h-4V3H9v6H5l7 7 7-7zM5 18v2h14v-2H5z" />
            </svg>
            <span>取回</span>
          </button>
          <button type="button" className="pos-action-btn dust" title="兑换残余"
            onClick={() => onSwapDust(taskId)}
            disabled={statusLabel.includes('停止中') || statusLabel.includes('撤出中')}>
            <svg viewBox="0 0 24 24" fill="currentColor" width="14" height="14">
              <path d="M7.5 21H2V9h5.5v12zm7.25-18h-5.5v18h5.5V3zM22 11h-5.5v10H22V11z" />
            </svg>
            <span>兑残</span>
          </button>
          <button type="button" className="pos-action-btn rebalance" title="立即触发再平衡"
            onClick={() => onTriggerRebalance(taskId)}
            disabled={!p?.has_liquidity || statusLabel.includes('已停止') || statusLabel.includes('停止中') || statusLabel.includes('撤出中')}>
            <svg viewBox="0 0 24 24" fill="currentColor" width="14" height="14">
              <path d="M12 6V1.5l-4.5 4.5L12 10.5V6c3.31 0 6 2.69 6 6 0 1.01-.25 1.97-.7 2.8l1.46 1.46C19.54 15.03 20 13.57 20 12c0-4.42-3.58-8-8-8zm0 14c-3.31 0-6-2.69-6-6 0-1.01.25-1.97.7-2.8L5.24 9.74C4.46 10.97 4 12.43 4 14c0 4.42 3.58 8 8 8v4.5l4.5-4.5L12 17.5V20z" />
            </svg>
            <span>再平衡</span>
          </button>
        </div>
      )}

      {Array.isArray(smartMoneyRangeGroups) && smartMoneyRangeGroups.length > 0 ? (
        <PositionSmartMoneyRangeSummary groups={smartMoneyRangeGroups} />
      ) : null}

      {(token0 || token1) && (
        <div className="pos-token-table">
          <div className="pos-token-head">
            <span>Token</span><span>钱包</span><span>仓位</span><span>手续费</span>
          </div>
          {[token0, token1].filter(Boolean).map((tk) => (
            <div key={tk.address || tk.symbol} className="pos-token-row">
              <div className="pos-tk-name">
                <div>{tk.symbol}</div>
                <div className="pos-tk-price">${Number(tk.price_usd || 0).toFixed(4)}</div>
              </div>
              <div className="pos-tk-cell">
                <div>{tk.wallet_amount ?? '--'}</div>
                <div className="pos-tk-usd">{formatUsd(tk.wallet_usd)}</div>
              </div>
              <div className="pos-tk-cell">
                <div>{tk.position_amount ?? '--'}</div>
                <div className="pos-tk-usd">{formatUsd(tk.position_usd)}</div>
              </div>
              <div className="pos-tk-cell fee">
                <div>{tk.fee_amount ?? '--'}</div>
                <div className="pos-tk-usd">{formatUsd(tk.fee_usd)}</div>
              </div>
            </div>
          ))}
          <div className="pos-token-foot">
            <span>小计</span>
            <span>{formatUsd(p?.totals?.wallet_usd)}</span>
            <span>{formatUsd(p?.totals?.position_usd)}</span>
            <span className="fee">{formatUsd(p?.totals?.fee_usd)}</span>
          </div>
        </div>
      )}

      {priceRange && (
        <div className="pos-price-range">
          <div className="pos-price-range-header">
            <span className="pos-price-range-label">价格范围 ({priceRange.pairLabel}{priceRange.gridCount ? ` 共${priceRange.gridCount}格` : ''}{Number.isFinite(gridStepPct) ? ` · 约${gridStepPct.toFixed(2)}%/格` : ''})</span>
            {hasTaskRange && Number.isFinite(taskRangeTotalWidth) && (
              <span className="pos-price-range-dev">总宽 {taskRangeTotalWidth.toFixed(2)}%</span>
            )}
          </div>
          <div className="pos-price-range-bar-wrap">
            <div className="pos-price-range-bar">
              <div className="pos-price-range-limit lo" />
              <div className="pos-price-range-limit hi" />
              {priceRange.visibleGridLines?.map((pct, i) => (
                <div key={i} className="pos-price-range-grid" style={{ left: `calc(3% + ${pct * 0.94}%)` }} />
              ))}
              <div
                className={`pos-price-range-cursor ${priceRange.inRange ? 'in' : 'out'}`}
                style={{ left: `calc(3% + ${priceRange.percent * 0.94}%)` }}
              />
            </div>
          </div>
          <div className="pos-price-range-labels">
            <span className="lo">{compactPrice(priceRange.rangeMin)}</span>
            <span className="cur">{compactPrice((priceRange.rangeMin + priceRange.rangeMax) / 2)}</span>
            <span className="hi">{compactPrice(priceRange.rangeMax)}</span>
          </div>
        </div>
      )}

      {hasTaskRange && (
        <div className="pos-range-info">
          <span>任务区间: {taskRangeSummary}</span>
          {Number.isFinite(taskAmount) && taskAmount > 0 && <span> | ${taskAmount.toFixed(2)}</span>}
          {priceRange && <span className="pos-range-cur-price">当前价 {compactPrice(priceRange.currentPrice)}</span>}
        </div>
      )}
    </div>
  );
}

export default function PositionsPanel({
  positions,
  positionsLoading,
  positionsError,
  sortedPositions,
  walletBalances,
  walletMetaByKey,
  positionSmartMoneyRanges,
  chain,
  taskActionPos,
  onTaskActionPosChange,
  onSelectPool,
  onTaskPause,
  onTaskStop,
  onTaskPartialExit,
  onTaskDelete,
  onTaskEditRange,
  onWithdrawLiquidity,
  onSwapDust,
  onTriggerRebalance,
  onUpdateTaskMode,
  onAddLiquidity,
  onCloseTaskActionMenu,
  getDexIcon,
  operationProgress,
}) {
  return (
    <PanelShell
      title="仓位"
      subtitle={positions?.wallet?.address ? shortAddress(positions.wallet.address, 8, 6) : '钱包未连接'}
      icon={BriefcaseBusiness}
    >
      {positionsError ? <div className="error-text">{positionsError}</div> : null}

      <PositionsSummary positions={positions} walletBalances={walletBalances} />
      <PositionWarnings warnings={positions?.warnings} />

      <div className="data-list">
        {positionsLoading && sortedPositions.length === 0 ? (
          <EmptyState text="正在加载仓位..." />
        ) : sortedPositions.length === 0 ? (
          <EmptyState text="暂无仓位数据" />
        ) : (
          sortedPositions.slice(0, 50).map((position, index) => (
            <PositionCard
              key={String(position?.position_id || index)}
              position={position}
              index={index}
              chain={chain}
              walletMetaByKey={walletMetaByKey}
              positionSmartMoneyRanges={positionSmartMoneyRanges}
              taskActionPos={taskActionPos}
              onTaskActionPosChange={onTaskActionPosChange}
              onSelectPool={onSelectPool}
              onTaskPause={onTaskPause}
              onTaskStop={onTaskStop}
              onTaskPartialExit={onTaskPartialExit}
              onTaskDelete={onTaskDelete}
              onTaskEditRange={onTaskEditRange}
              onWithdrawLiquidity={onWithdrawLiquidity}
              onSwapDust={onSwapDust}
              onTriggerRebalance={onTriggerRebalance}
              onUpdateTaskMode={onUpdateTaskMode}
              onAddLiquidity={onAddLiquidity}
              onCloseTaskActionMenu={onCloseTaskActionMenu}
              getDexIcon={getDexIcon}
            />
          ))
        )}
      </div>
      {operationProgress}
    </PanelShell>
  );
}
