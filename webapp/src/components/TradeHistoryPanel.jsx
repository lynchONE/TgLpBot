import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { BarChart2, TrendingUp, TrendingDown, Award, Target, DollarSign } from 'lucide-react';
import { fetchTradeHistory } from '../api';
import PanelShell from './PanelShell';
import CustomSelect from './CustomSelect';

const STATUS_MAP = {
    open: { emoji: '🟢', text: '进行中', cls: 'status-open' },
    closed: { emoji: '✅', text: '已结束', cls: 'status-closed' },
    aborted: { emoji: '⚠️', text: '已中止', cls: 'status-aborted' },
    orphaned: { emoji: '🔵', text: '记录缺失', cls: 'status-orphaned' },
};

const STATUS_OPTIONS = [
    { value: '', label: '全部' },
    { value: 'open', label: '进行中' },
    { value: 'closed', label: '已结束' },
    { value: 'aborted', label: '已中止' },
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
    if (min < 60) {
        const remSec = diffSec % 60;
        return remSec && min < 10 ? `${min}m${remSec}s` : `${min}分钟`;
    }
    const h = Math.floor(min / 60);
    const remMin = min % 60;
    if (h < 24) return remMin ? `${h}h${remMin}m` : `${h}h`;
    const d = Math.floor(h / 24);
    const remH = h % 24;
    return remH ? `${d}d${remH}h` : `${d}d`;
}

function aggregateTradeSummary(records) {
    const closed = records.filter((r) => r.status === 'closed');
    const totalInvested = records.reduce((acc, r) => acc + (Number(r.open_usdt_spent) || 0), 0);
    const totalProfit = closed.reduce((acc, r) => acc + (Number(r.profit_usdt) || 0), 0);
    const totalGas = closed.reduce((acc, r) => acc + (Number(r.total_gas_usdt) || 0), 0);
    const wins = closed.filter((r) => Number(r.profit_usdt) > 0).length;
    const winRate = closed.length > 0 ? Math.round((wins / closed.length) * 100) : 0;
    let best = null;
    let worst = null;
    for (const r of closed) {
        const p = Number(r.profit_usdt);
        if (!Number.isFinite(p)) continue;
        if (!best || p > Number(best.profit_usdt)) best = r;
        if (!worst || p < Number(worst.profit_usdt)) worst = r;
    }
    return {
        loaded: records.length,
        closedCount: closed.length,
        totalInvested,
        totalProfit,
        totalGas,
        winRate,
        wins,
        best,
        worst,
    };
}

function TradeHistorySummary({ records, total }) {
    const summary = useMemo(() => aggregateTradeSummary(records), [records]);
    if (summary.loaded === 0) return null;
    const profitClass = summary.totalProfit > 0 ? 'pos' : summary.totalProfit < 0 ? 'neg' : '';
    const winClass = summary.winRate >= 60 ? 'pos' : summary.winRate > 0 && summary.winRate < 40 ? 'neg' : '';

    return (
        <div className="trade-summary">
            <div className="trade-summary-head">
                <div className="trade-summary-title">
                    <BarChart2 size={13} /> 概览
                </div>
                <div className="trade-summary-scope">
                    基于已加载 <strong>{summary.loaded}</strong> / {total} 笔
                    {summary.closedCount > 0 && (
                        <span className="trade-summary-scope-sub">（{summary.closedCount} 笔已结束）</span>
                    )}
                </div>
            </div>
            <div className="trade-summary-grid">
                <div className="trade-summary-cell">
                    <div className="trade-summary-label"><DollarSign size={11} /> 总投入</div>
                    <div className="trade-summary-value">{formatUsd(summary.totalInvested)}</div>
                </div>
                <div className="trade-summary-cell">
                    <div className="trade-summary-label">
                        {summary.totalProfit >= 0 ? <TrendingUp size={11} /> : <TrendingDown size={11} />}
                        净盈亏（扣 Gas）
                    </div>
                    <div className={`trade-summary-value ${profitClass}`}>
                        {summary.closedCount > 0 ? formatSignedUsd(summary.totalProfit) : '—'}
                    </div>
                    {summary.closedCount > 0 && summary.totalGas > 0 && (
                        <div className="trade-summary-sub">Gas {formatUsd(summary.totalGas)}</div>
                    )}
                </div>
                <div className="trade-summary-cell">
                    <div className="trade-summary-label"><Target size={11} /> 胜率</div>
                    <div className={`trade-summary-value ${winClass}`}>
                        {summary.closedCount > 0 ? `${summary.winRate}%` : '—'}
                    </div>
                    {summary.closedCount > 0 && (
                        <div className="trade-summary-sub">{summary.wins} / {summary.closedCount}</div>
                    )}
                </div>
                <div className="trade-summary-cell">
                    <div className="trade-summary-label"><Award size={11} /> 最赚 / 最亏</div>
                    <div className="trade-summary-value trade-summary-value--mini">
                        <span className="pos">
                            {summary.best ? formatSignedUsd(summary.best.profit_usdt) : '—'}
                        </span>
                        <span className="neg">
                            {summary.worst ? formatSignedUsd(summary.worst.profit_usdt) : '—'}
                        </span>
                    </div>
                </div>
            </div>
        </div>
    );
}

export default function TradeHistoryPanel({ apiBaseUrl, initData }) {
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
            if (newOffset === 0) {
                setRecords(resp?.records || []);
            } else {
                setRecords(prev => [...prev, ...(resp?.records || [])]);
            }
            setTotal(resp?.total || 0);
            setOffset(newOffset);
        } catch (e) {
            setError(String(e?.message || e));
        } finally {
            setLoading(false);
        }
    }, [apiBaseUrl, initData, statusFilter]);

    useEffect(() => {
        load(0);
    }, [load]);

    const hasMore = records.length < total;

    return (
        <PanelShell
            title="交易历史"
            subtitle={`共 ${total} 条记录`}
            icon={BarChart2}
            actions={(
                <div className="trade-history-filters">
                    <CustomSelect value={statusFilter} onChange={setStatusFilter} options={STATUS_OPTIONS} className="trade-filter-select" />
                </div>
            )}
        >
            {records.length > 0 && <TradeHistorySummary records={records} total={total} />}
            {error && <div className="panel-error">{error}</div>}
            {loading && records.length === 0 ? (
                <div className="panel-loading">加载中...</div>
            ) : records.length === 0 ? (
                <div className="empty-state">📳 暂无交易记录</div>
            ) : (
                <div className="trade-history-list">
                    {records.map((rec) => {
                        const st = STATUS_MAP[rec.status] || STATUS_MAP.open;
                        const pair = [rec.token0_symbol, rec.token1_symbol].filter(Boolean).join('/') || '-';
                        const profitNum = Number(rec.profit_usdt);
                        const profitPositive = Number.isFinite(profitNum) && profitNum > 0;
                        const profitNegative = Number.isFinite(profitNum) && profitNum < 0;
                        const profitPctNum = Number(rec.profit_pct);
                        const hasOpenSnapshot = Number(rec.open_stable_before || 0) > 0 || Number(rec.open_stable_after || 0) > 0;
                        const hasCloseSnapshot = Number(rec.close_stable_before || 0) > 0 || Number(rec.close_stable_after || 0) > 0;
                        const showGas = rec.status === 'closed' && Number(rec.total_gas_usdt || 0) > 0;
                        const duration = formatPositionDuration(rec.opened_at, rec.closed_at);

                        return (
                            <div key={rec.id} className={`trade-record-card${rec.status === 'closed' ? (profitPositive ? ' has-profit' : profitNegative ? ' has-loss' : '') : ''}`}>
                                <div className="trade-record-header">
                                    <div className="trade-pair">
                                        <strong>{pair}</strong>
                                        {rec.chain && <span className="trade-chain-badge">{rec.chain.toUpperCase()}</span>}
                                    </div>
                                    <span className={`trade-status ${st.cls}`}>{st.emoji} {st.text}</span>
                                </div>
                                <div className="trade-record-meta">
                                    <span>{rec.exchange || '-'} / {rec.pool_version || ''}</span>
                                    {duration !== '-' && (
                                        <span className="trade-record-duration">⏱ {duration}</span>
                                    )}
                                </div>

                                {rec.status === 'closed' && Number.isFinite(profitNum) && (
                                    <div className="trade-profit-bar">
                                        <div className="trade-profit-bar-label">收益(扣Gas)</div>
                                        <div className={`trade-profit-bar-value ${profitPositive ? 'profit-positive' : profitNegative ? 'profit-negative' : ''}`}>
                                            <span className="trade-profit-bar-amount">{formatSignedUsd(profitNum)}</span>
                                            {Number.isFinite(profitPctNum) && (
                                                <span className="trade-profit-bar-pct">
                                                    {profitPctNum >= 0 ? '+' : ''}{profitPctNum.toFixed(2)}%
                                                </span>
                                            )}
                                        </div>
                                    </div>
                                )}

                                <div className="trade-record-details">
                                    <div className="trade-detail-row">
                                        <span className="trade-detail-label">开仓时间</span>
                                        <span>{rec.opened_at || '-'}</span>
                                    </div>
                                    <div className="trade-detail-row">
                                        <span className="trade-detail-label">投入</span>
                                        <span>{formatUsd(rec.open_usdt_spent)}</span>
                                    </div>
                                    {hasOpenSnapshot && (
                                        <>
                                            <div className="trade-detail-row">
                                                <span className="trade-detail-label">开仓前余额</span>
                                                <span>{formatUsd(rec.open_stable_before)}</span>
                                            </div>
                                            <div className="trade-detail-row">
                                                <span className="trade-detail-label">开仓后余额</span>
                                                <span>{formatUsd(rec.open_stable_after)}</span>
                                            </div>
                                        </>
                                    )}
                                    {rec.closed_at && (
                                        <>
                                            <div className="trade-detail-row">
                                                <span className="trade-detail-label">撤仓时间</span>
                                                <span>{rec.closed_at}</span>
                                            </div>
                                            <div className="trade-detail-row">
                                                <span className="trade-detail-label">撤出</span>
                                                <span>{formatUsd(rec.close_usdt_received)}</span>
                                            </div>
                                            {hasCloseSnapshot && (
                                                <>
                                                    <div className="trade-detail-row">
                                                        <span className="trade-detail-label">撤仓前余额</span>
                                                        <span>{formatUsd(rec.close_stable_before)}</span>
                                                    </div>
                                                    <div className="trade-detail-row">
                                                        <span className="trade-detail-label">撤仓后余额</span>
                                                        <span>{formatUsd(rec.close_stable_after)}</span>
                                                    </div>
                                                </>
                                            )}
                                        </>
                                    )}
                                    {showGas && (
                                        <div className="trade-detail-row">
                                            <span className="trade-detail-label">Gas 费</span>
                                            <span className="trade-gas">{formatUsd(rec.total_gas_usdt)}</span>
                                        </div>
                                    )}
                                </div>
                                {(rec.open_tx_url || rec.close_tx_url) && (
                                    <div className="trade-record-links">
                                        {rec.open_tx_url && <a href={rec.open_tx_url} target="_blank" rel="noopener noreferrer">🔗 开仓 Tx</a>}
                                        {rec.close_tx_url && <a href={rec.close_tx_url} target="_blank" rel="noopener noreferrer">🔗 撤仓 Tx</a>}
                                    </div>
                                )}
                            </div>
                        );
                    })}

                    {hasMore && (
                        <button type="button" className="panel-action-btn load-more-btn" onClick={() => load(offset + PAGE_SIZE)} disabled={loading}>
                            {loading ? '加载中...' : '加载更多'}
                        </button>
                    )}
                </div>
            )}
        </PanelShell>
    );
}
