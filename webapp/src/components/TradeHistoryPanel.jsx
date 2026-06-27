import { useCallback, useEffect, useMemo, useState } from 'react';
import { BarChart3, Clock3, ExternalLink, Target, TrendingDown, TrendingUp } from 'lucide-react';
import { fetchTradeHistory } from '../api';
import PanelShell, { EmptyState, MetricCard } from './PanelShell';
import CustomSelect from './CustomSelect';

const STATUS_MAP = {
  open: { text: '进行中', cls: 'status-open' },
  closed: { text: '已结束', cls: 'status-closed' },
  aborted: { text: '已中止', cls: 'status-aborted' },
  orphaned: { text: '记录缺失', cls: 'status-orphaned' },
};

const STATUS_OPTIONS = [
  { value: '', label: '全部状态' },
  { value: 'open', label: '进行中' },
  { value: 'closed', label: '已结束' },
  { value: 'aborted', label: '已中止' },
  { value: 'orphaned', label: '记录缺失' },
];

function formatUsd(value) {
  if (value === undefined || value === null) return '-';
  const num = Number(value || 0);
  if (!Number.isFinite(num)) return '-';
  return `$${num.toFixed(2)}`;
}

function formatSignedUsd(value) {
  if (value === undefined || value === null) return '-';
  const num = Number(value || 0);
  if (!Number.isFinite(num)) return '-';
  const sign = num > 0 ? '+' : '';
  return `${sign}$${num.toFixed(2)}`;
}

function formatPositionDuration(openedAt, closedAt) {
  if (!openedAt) return '-';
  const start = Date.parse(openedAt);
  if (!Number.isFinite(start)) return '-';
  const endRaw = closedAt ? Date.parse(closedAt) : Date.now();
  const end = Number.isFinite(endRaw) ? endRaw : Date.now();
  const diffSec = Math.max(0, Math.floor((end - start) / 1000));
  if (diffSec < 60) return `${diffSec}秒`;
  const min = Math.floor(diffSec / 60);
  if (min < 60) return `${min}分钟`;
  const hour = Math.floor(min / 60);
  if (hour < 24) return `${hour}小时${min % 60 ? `${min % 60}分` : ''}`;
  const day = Math.floor(hour / 24);
  return `${day}天${hour % 24 ? `${hour % 24}小时` : ''}`;
}

function formatCloseTxDetailLabel(detail) {
  const text = String(detail || '').trim();
  if (!text) return '';
  const parts = text.split('|');
  return String(parts[0] || text).trim();
}

function aggregateTradeSummary(records) {
  const closed = records.filter((record) => record.status === 'closed');
  const totalInvested = records.reduce((acc, record) => acc + (Number(record.open_usdt_spent) || 0), 0);
  const totalProfit = closed.reduce((acc, record) => acc + (Number(record.profit_usdt) || 0), 0);
  const totalGas = closed.reduce((acc, record) => acc + (Number(record.total_gas_usdt) || 0), 0);
  const wins = closed.filter((record) => Number(record.profit_usdt) > 0).length;
  return {
    loaded: records.length,
    closedCount: closed.length,
    totalInvested,
    totalProfit,
    totalGas,
    winRate: closed.length > 0 ? Math.round((wins / closed.length) * 100) : 0,
    wins,
  };
}

export default function TradeHistoryPanel({ apiBaseUrl, initData, hasInitData = true, embedded = false }) {
  const [records, setRecords] = useState([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [offset, setOffset] = useState(0);
  const [statusFilter, setStatusFilter] = useState('');
  const PAGE_SIZE = 20;

  const load = useCallback(async (newOffset = 0) => {
    if (!initData) return;
    setLoading(true);
    setError('');
    try {
      const resp = await fetchTradeHistory({
        apiBaseUrl,
        initData,
        status: statusFilter || undefined,
        limit: PAGE_SIZE,
        offset: newOffset,
      });
      const nextRecords = resp?.records || [];
      if (newOffset === 0) {
        setRecords(nextRecords);
      } else {
        setRecords((prev) => [...prev, ...nextRecords]);
      }
      setTotal(resp?.total || 0);
      setOffset(newOffset);
    } catch (e) {
      setError(String(e?.message || e));
      throw e;
    } finally {
      setLoading(false);
    }
  }, [apiBaseUrl, initData, statusFilter]);

  useEffect(() => {
    if (hasInitData) load(0);
  }, [hasInitData, load]);

  const summary = useMemo(() => aggregateTradeSummary(records), [records]);
  const hasMore = records.length < total;
  const profitTone = summary.totalProfit > 0 ? 'positive' : summary.totalProfit < 0 ? 'negative' : 'default';
  const winTone = summary.closedCount > 0 && summary.winRate >= 60 ? 'positive' : summary.closedCount > 0 && summary.winRate < 40 ? 'negative' : 'default';

  const body = (
    <div className="am-stack">
      {!hasInitData ? <EmptyState text="请先完成 Telegram 登录后查看交易历史" /> : null}
      {error ? <div className="am-error">{error}</div> : null}

      <div className="am-metric-row">
        <MetricCard label="总投入" value={formatUsd(summary.totalInvested)} tone="strong" />
        <MetricCard label="净盈亏" value={summary.closedCount > 0 ? formatSignedUsd(summary.totalProfit) : '-'} tone={profitTone} />
        <MetricCard label="已结束" value={`${summary.closedCount} 笔`} />
        <MetricCard label="胜率" value={summary.closedCount > 0 ? `${summary.winRate}%` : '-'} tone={winTone} />
      </div>

      <div className="am-card trade-filter-card">
        <div className="am-card-header">
          <div className="am-card-title">
            <Target size={14} />
            筛选
          </div>
          <span className="am-badge">
            已加载 {records.length} / {total} 条
            {summary.closedCount > 0 ? ` · Gas ${formatUsd(summary.totalGas)}` : ''}
          </span>
        </div>
        <div className="trade-history-filters">
          <CustomSelect value={statusFilter} onChange={setStatusFilter} options={STATUS_OPTIONS} className="trade-filter-select" />
        </div>
      </div>

      {loading && records.length === 0 ? (
        <div className="panel-loading">加载中...</div>
      ) : records.length === 0 ? (
        <EmptyState text="当前筛选条件下没有交易记录" />
      ) : (
        <div className="trade-history-list trade-history-list--asset">
          {records.map((record) => (
            <TradeRecordCard key={record.id} record={record} />
          ))}

          {hasMore ? (
            <button type="button" className="am-action-btn load-more-btn" onClick={() => load(offset + PAGE_SIZE)} disabled={loading}>
              {loading ? '加载中...' : '加载更多'}
            </button>
          ) : null}
        </div>
      )}
    </div>
  );

  if (embedded) return body;

  return (
    <PanelShell
      title="交易历史"
      subtitle={`共 ${total} 条记录`}
      icon={BarChart3}
    >
      {body}
    </PanelShell>
  );
}

function TradeRecordCard({ record }) {
  const st = STATUS_MAP[record.status] || STATUS_MAP.open;
  const pair = [record.token0_symbol, record.token1_symbol].filter(Boolean).join('/') || '-';
  const profitNum = Number(record.profit_usdt);
  const profitPositive = Number.isFinite(profitNum) && profitNum > 0;
  const profitNegative = Number.isFinite(profitNum) && profitNum < 0;
  const profitPctNum = Number(record.profit_pct);
  const hasOpenSnapshot = Number(record.open_stable_before || 0) > 0 || Number(record.open_stable_after || 0) > 0;
  const hasCloseSnapshot = Number(record.close_stable_before || 0) > 0 || Number(record.close_stable_after || 0) > 0;
  const showGas = record.status === 'closed' && Number(record.total_gas_usdt || 0) > 0;
  const duration = formatPositionDuration(record.opened_at, record.closed_at);
  const closeTxDetails = Array.isArray(record.close_tx_details)
    ? record.close_tx_details.map(formatCloseTxDetailLabel).filter(Boolean)
    : [];

  return (
    <div className={`trade-record-card trade-record-card--asset${record.status === 'closed' ? (profitPositive ? ' has-profit' : profitNegative ? ' has-loss' : '') : ''}`}>
      <div className="trade-record-header">
        <div className="trade-pair-wrap">
          <div className="trade-pair">
            <strong>{pair}</strong>
            {record.chain ? <span className="trade-chain-badge">{String(record.chain).toUpperCase()}</span> : null}
          </div>
          <div className="trade-record-meta">
            <span>{record.exchange || '-'} / {record.pool_version || ''}</span>
            {duration !== '-' ? (
              <span className="trade-record-duration">
                <Clock3 size={11} />
                {duration}
              </span>
            ) : null}
          </div>
        </div>
        <span className={`trade-status ${st.cls}`}>{st.text}</span>
      </div>

      {record.status === 'closed' && Number.isFinite(profitNum) ? (
        <div className="trade-profit-bar">
          <div className="trade-profit-bar-label">收益(扣 Gas)</div>
          <div className={`trade-profit-bar-value ${profitPositive ? 'profit-positive' : profitNegative ? 'profit-negative' : ''}`}>
            {profitPositive ? <TrendingUp size={15} /> : profitNegative ? <TrendingDown size={15} /> : null}
            <span className="trade-profit-bar-amount">{formatSignedUsd(profitNum)}</span>
            {Number.isFinite(profitPctNum) ? (
              <span className="trade-profit-bar-pct">
                {profitPctNum >= 0 ? '+' : ''}{profitPctNum.toFixed(2)}%
              </span>
            ) : null}
          </div>
        </div>
      ) : null}

      <div className="trade-record-details">
        <DetailCell label="开仓时间" value={record.opened_at || '-'} />
        <DetailCell label="投入" value={formatUsd(record.open_usdt_spent)} />
        {hasOpenSnapshot ? (
          <>
            <DetailCell label="开仓前余额" value={formatUsd(record.open_stable_before)} />
            <DetailCell label="开仓后余额" value={formatUsd(record.open_stable_after)} />
          </>
        ) : null}
        {record.closed_at ? (
          <>
            <DetailCell label="撤仓时间" value={record.closed_at} />
            <DetailCell label="撤出" value={formatUsd(record.close_usdt_received)} />
          </>
        ) : null}
        {hasCloseSnapshot ? (
          <>
            <DetailCell label="撤仓前余额" value={formatUsd(record.close_stable_before)} />
            <DetailCell label="撤仓后余额" value={formatUsd(record.close_stable_after)} />
          </>
        ) : null}
        {showGas ? <DetailCell label="Gas 费" value={formatUsd(record.total_gas_usdt)} accent="warn" /> : null}
      </div>

      {closeTxDetails.length > 0 ? (
        <div className="trade-record-links">
          {closeTxDetails.map((detail, index) => (
            <span key={`${detail}-${index}`} className="am-badge">
              撤仓路由：{detail}
            </span>
          ))}
        </div>
      ) : null}

      {(record.open_tx_url || record.close_tx_url) ? (
        <div className="trade-record-links">
          {record.open_tx_url ? <TxLink href={record.open_tx_url}>开仓 Tx</TxLink> : null}
          {record.close_tx_url ? <TxLink href={record.close_tx_url}>撤仓 Tx</TxLink> : null}
        </div>
      ) : null}
    </div>
  );
}

function DetailCell({ label, value, accent = '' }) {
  return (
    <div className={`trade-detail-cell ${accent}`}>
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  );
}

function TxLink({ href, children }) {
  return (
    <a href={href} target="_blank" rel="noopener noreferrer">
      <ExternalLink size={12} />
      {children}
    </a>
  );
}
