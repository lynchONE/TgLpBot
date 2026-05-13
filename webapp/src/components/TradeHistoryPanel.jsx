import React, { useCallback, useEffect, useState } from 'react';
import { BarChart2 } from 'lucide-react';
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
                        const profitPositive = rec.profit_pct > 0;
                        const profitNegative = rec.profit_pct < 0;
                        const hasOpenSnapshot = Number(rec.open_stable_before || 0) > 0 || Number(rec.open_stable_after || 0) > 0;
                        const hasCloseSnapshot = Number(rec.close_stable_before || 0) > 0 || Number(rec.close_stable_after || 0) > 0;

                        return (
                            <div key={rec.id} className="trade-record-card">
                                <div className="trade-record-header">
                                    <div className="trade-pair">
                                        <strong>{pair}</strong>
                                        {rec.chain && <span className="trade-chain-badge">{rec.chain.toUpperCase()}</span>}
                                    </div>
                                    <span className={`trade-status ${st.cls}`}>{st.emoji} {st.text}</span>
                                </div>
                                <div className="trade-record-meta">
                                    <span>{rec.exchange || '-'} / {rec.pool_version || ''}</span>
                                </div>
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
                                    {rec.status === 'closed' && (
                                        <div className="trade-detail-row">
                                            <span className="trade-detail-label">收益(扣Gas)</span>
                                            <span className={profitPositive ? 'profit-positive' : profitNegative ? 'profit-negative' : ''}>
                                                {rec.profit_usdt !== undefined
                                                    ? `${rec.profit_usdt >= 0 ? '+' : ''}$${Number(rec.profit_usdt || 0).toFixed(2)} (${rec.profit_pct >= 0 ? '+' : ''}${Number(rec.profit_pct || 0).toFixed(2)}%)`
                                                    : '-'
                                                }
                                            </span>
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
