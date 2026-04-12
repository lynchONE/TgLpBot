import React, { useCallback, useEffect, useState } from 'react';
import BottomSheet from './BottomSheet.jsx';
import CustomSelect from './CustomSelect.jsx';
import { fetchTradeHistory } from '../lib/api';

const STATUS_OPTIONS = [
    { value: '', label: '全部状态' },
    { value: 'open', label: '🟢 进行中' },
    { value: 'closed', label: '✅ 已结束' },
    { value: 'aborted', label: '⚠️ 已中止' },
    { value: 'orphaned', label: '🔵 记录缺失' },
];

const STATUS_MAP = {
    open: { emoji: '🟢', text: '进行中', color: 'text-emerald-600 dark:text-emerald-400' },
    closed: { emoji: '✅', text: '已结束', color: 'text-blue-600 dark:text-blue-400' },
    aborted: { emoji: '⚠️', text: '已中止', color: 'text-amber-600 dark:text-amber-400' },
    orphaned: { emoji: '🔵', text: '记录缺失', color: 'text-zinc-500 dark:text-white/50' },
};

const CHAIN_OPTIONS = [
    { value: '', label: '全部链' },
    { value: 'bsc', label: 'BSC', icon: '🟡' },
    { value: 'base', label: 'Base', icon: '🔵' },
];

function formatUsd(value) {
    if (value === undefined || value === null) return '-';
    const num = Number(value || 0);
    if (!Number.isFinite(num)) return '-';
    return `$${num.toFixed(2)}`;
}

export default function TradeHistoryPage({ open, onClose, apiBaseUrl, initData, multiChainEnabled = true }) {
    const [chain, setChain] = useState('');
    const [status, setStatus] = useState('');
    const [records, setRecords] = useState([]);
    const [total, setTotal] = useState(0);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState('');
    const [offset, setOffset] = useState(0);
    const PAGE_SIZE = 20;

    const load = useCallback(async (newOffset = 0) => {
        if (!initData) return;
        setLoading(true);
        setError('');
        try {
            const resp = await fetchTradeHistory({
                apiBaseUrl,
                initData,
                chain: chain || undefined,
                status: status || undefined,
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
    }, [apiBaseUrl, initData, chain, status]);

    useEffect(() => {
        if (open) load(0);
    }, [open, load]);

    const hasMore = records.length < total;

    return (
        <BottomSheet open={open} onClose={onClose} title="交易历史" maxHeightClass="max-h-[90vh]">
            <div className="mb-4 flex gap-2">
                {multiChainEnabled && (
                    <div className="flex-1">
                        <CustomSelect
                            value={chain}
                            onChange={setChain}
                            options={CHAIN_OPTIONS}
                            placeholder="链"
                        />
                    </div>
                )}
                <div className="flex-1">
                    <CustomSelect
                        value={status}
                        onChange={setStatus}
                        options={STATUS_OPTIONS}
                        placeholder="状态"
                    />
                </div>
            </div>

            {error && (
                <div className="mb-3 rounded-xl border border-red-500/30 bg-red-500/10 p-3 text-xs text-red-700 dark:text-red-300">
                    {error}
                </div>
            )}

            {records.length > 0 && (
                <div className="mb-3 text-xs text-zinc-400 dark:text-white/30">
                    共 {total} 条记录 / 当前显示 {records.length} 条
                </div>
            )}

            {loading && records.length === 0 ? (
                <div className="flex items-center justify-center py-12 text-sm text-zinc-400 dark:text-white/40">
                    <svg className="mr-2 h-5 w-5 animate-spin" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                        <circle className="opacity-25" cx="12" cy="12" r="10" />
                        <path className="opacity-75" d="M4 12a8 8 0 018-8" />
                    </svg>
                    加载中...
                </div>
            ) : records.length === 0 ? (
                <div className="py-12 text-center text-sm text-zinc-400 dark:text-white/40">
                    <div className="mb-2 text-3xl">📳</div>
                    <div>暂无交易记录</div>
                </div>
            ) : (
                <div className="space-y-3">
                    {records.map((rec) => (
                        <TradeRecordCard key={rec.id} rec={rec} />
                    ))}

                    {hasMore && (
                        <button
                            type="button"
                            onClick={() => load(offset + PAGE_SIZE)}
                            disabled={loading}
                            className={`w-full rounded-xl border border-zinc-200 bg-white/70 px-4 py-3 text-sm font-semibold text-zinc-600 transition-colors hover:bg-white dark:border-white/10 dark:bg-white/5 dark:text-white/60 dark:hover:bg-white/10 ${loading ? 'cursor-not-allowed opacity-50' : ''}`}
                        >
                            {loading ? '加载中...' : '加载更多'}
                        </button>
                    )}
                </div>
            )}
        </BottomSheet>
    );
}

function TradeRecordCard({ rec }) {
    const st = STATUS_MAP[rec.status] || STATUS_MAP.open;
    const pair = [rec.token0_symbol, rec.token1_symbol].filter(Boolean).join('/') || '-';
    const profitPositive = rec.profit_pct > 0;
    const profitNegative = rec.profit_pct < 0;
    const hasOpenSnapshot = Number(rec.open_stable_before || 0) > 0 || Number(rec.open_stable_after || 0) > 0;
    const hasCloseSnapshot = Number(rec.close_stable_before || 0) > 0 || Number(rec.close_stable_after || 0) > 0;

    return (
        <div className="rounded-2xl border border-zinc-200/50 bg-white/70 p-4 transition-colors dark:border-white/[0.06] dark:bg-white/[0.03]">
            <div className="flex items-start justify-between gap-2">
                <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-2">
                        <span className="text-sm font-bold text-zinc-900 dark:text-white/90">{pair}</span>
                        {rec.chain && (
                            <span className="inline-flex items-center rounded-md bg-zinc-100 px-1.5 py-0.5 text-[10px] font-medium text-zinc-500 dark:bg-white/5 dark:text-white/40">
                                {rec.chain.toUpperCase()}
                            </span>
                        )}
                    </div>
                    <div className="mt-0.5 text-xs text-zinc-400 dark:text-white/30">
                        {rec.exchange || '-'} / {rec.pool_version || ''}
                    </div>
                </div>
                <span className={`inline-flex items-center gap-1 rounded-lg px-2 py-1 text-xs font-semibold ${st.color}`}>
                    <span>{st.emoji}</span>
                    {st.text}
                </span>
            </div>

            <div className="mt-3 grid grid-cols-2 gap-x-4 gap-y-2 text-xs">
                <div>
                    <div className="text-zinc-400 dark:text-white/30">开仓时间</div>
                    <div className="mt-0.5 font-medium text-zinc-700 dark:text-white/70">{rec.opened_at || '-'}</div>
                </div>
                <div>
                    <div className="text-zinc-400 dark:text-white/30">投入 USDT</div>
                    <div className="mt-0.5 font-medium text-zinc-700 dark:text-white/70">
                        {formatUsd(rec.open_usdt_spent)}
                    </div>
                </div>

                {hasOpenSnapshot && (
                    <>
                        <div>
                            <div className="text-zinc-400 dark:text-white/30">开仓前余额</div>
                            <div className="mt-0.5 font-medium text-zinc-700 dark:text-white/70">{formatUsd(rec.open_stable_before)}</div>
                        </div>
                        <div>
                            <div className="text-zinc-400 dark:text-white/30">开仓后余额</div>
                            <div className="mt-0.5 font-medium text-zinc-700 dark:text-white/70">{formatUsd(rec.open_stable_after)}</div>
                        </div>
                    </>
                )}

                {rec.closed_at && (
                    <>
                        <div>
                            <div className="text-zinc-400 dark:text-white/30">撤仓时间</div>
                            <div className="mt-0.5 font-medium text-zinc-700 dark:text-white/70">{rec.closed_at}</div>
                        </div>
                        <div>
                            <div className="text-zinc-400 dark:text-white/30">撤出 USDT</div>
                            <div className="mt-0.5 font-medium text-zinc-700 dark:text-white/70">
                                {formatUsd(rec.close_usdt_received)}
                            </div>
                        </div>
                    </>
                )}

                {hasCloseSnapshot && (
                    <>
                        <div>
                            <div className="text-zinc-400 dark:text-white/30">撤仓前余额</div>
                            <div className="mt-0.5 font-medium text-zinc-700 dark:text-white/70">{formatUsd(rec.close_stable_before)}</div>
                        </div>
                        <div>
                            <div className="text-zinc-400 dark:text-white/30">撤仓后余额</div>
                            <div className="mt-0.5 font-medium text-zinc-700 dark:text-white/70">{formatUsd(rec.close_stable_after)}</div>
                        </div>
                    </>
                )}

                {rec.status === 'closed' && (
                    <>
                        <div>
                            <div className="text-zinc-400 dark:text-white/30">Gas 费用</div>
                            <div className="mt-0.5 font-medium text-zinc-700 dark:text-white/70">
                                {formatUsd(rec.total_gas_usdt)}
                            </div>
                        </div>
                        <div>
                            <div className="text-zinc-400 dark:text-white/30">收益(扣Gas)</div>
                            <div className={`mt-0.5 font-bold ${profitPositive ? 'text-emerald-600 dark:text-emerald-400' : profitNegative ? 'text-red-500 dark:text-red-400' : 'text-zinc-700 dark:text-white/70'}`}>
                                {rec.profit_usdt !== undefined && rec.profit_usdt !== null
                                    ? `${rec.profit_usdt >= 0 ? '+' : ''}$${Number(rec.profit_usdt || 0).toFixed(2)} (${rec.profit_pct >= 0 ? '+' : ''}${Number(rec.profit_pct || 0).toFixed(2)}%)`
                                    : '-'
                                }
                            </div>
                        </div>
                    </>
                )}
            </div>

            {(rec.open_tx_url || rec.close_tx_url) && (
                <div className="mt-3 flex flex-wrap gap-2">
                    {rec.open_tx_url && (
                        <a
                            href={rec.open_tx_url}
                            target="_blank"
                            rel="noopener noreferrer"
                            className="inline-flex items-center gap-1 rounded-lg bg-zinc-100 px-2 py-1 text-[11px] font-medium text-zinc-600 transition-colors hover:bg-zinc-200 dark:bg-white/5 dark:text-white/50 dark:hover:bg-white/10"
                        >
                            🔗 开仓 Tx
                        </a>
                    )}
                    {rec.close_tx_url && (
                        <a
                            href={rec.close_tx_url}
                            target="_blank"
                            rel="noopener noreferrer"
                            className="inline-flex items-center gap-1 rounded-lg bg-zinc-100 px-2 py-1 text-[11px] font-medium text-zinc-600 transition-colors hover:bg-zinc-200 dark:bg-white/5 dark:text-white/50 dark:hover:bg-white/10"
                        >
                            🔗 撤仓 Tx
                        </a>
                    )}
                </div>
            )}
        </div>
    );
}
