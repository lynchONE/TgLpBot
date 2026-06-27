import { useCallback, useEffect, useMemo, useState } from 'react';
import { BarChart3, Clock3, ExternalLink, TrendingDown, TrendingUp } from 'lucide-react';
import BottomSheet from './BottomSheet.jsx';
import CustomSelect from './CustomSelect.jsx';
import { fetchTradeHistory } from '../lib/api';

const STATUS_OPTIONS = [
    { value: '', label: '全部状态' },
    { value: 'open', label: '进行中' },
    { value: 'closed', label: '已结束' },
    { value: 'aborted', label: '已中止' },
    { value: 'orphaned', label: '记录缺失' },
];

const STATUS_MAP = {
    open: {
        text: '进行中',
        chip: 'bg-emerald-500/[0.08] text-emerald-700 ring-emerald-500/20 dark:bg-emerald-500/[0.10] dark:text-emerald-300 dark:ring-emerald-400/20',
    },
    closed: {
        text: '已结束',
        chip: 'bg-sky-500/[0.08] text-sky-700 ring-sky-500/20 dark:bg-sky-500/[0.10] dark:text-sky-300 dark:ring-sky-400/20',
    },
    aborted: {
        text: '已中止',
        chip: 'bg-amber-500/[0.08] text-amber-700 ring-amber-500/20 dark:bg-amber-500/[0.10] dark:text-amber-300 dark:ring-amber-400/20',
    },
    orphaned: {
        text: '记录缺失',
        chip: 'bg-zinc-100 text-zinc-500 ring-zinc-200 dark:bg-white/[0.04] dark:text-white/45 dark:ring-white/[0.08]',
    },
};

const CHAIN_OPTIONS = [
    { value: '', label: '全部链' },
    { value: 'bsc', label: 'BSC' },
    { value: 'base', label: 'Base' },
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
    return `${num > 0 ? '+' : ''}$${num.toFixed(2)}`;
}

function formatDuration(openedAt, closedAt) {
    if (!openedAt) return '-';
    const start = Date.parse(openedAt);
    if (!Number.isFinite(start)) return '-';
    const parsedEnd = closedAt ? Date.parse(closedAt) : Date.now();
    const end = Number.isFinite(parsedEnd) ? parsedEnd : Date.now();
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

function aggregateSummary(records) {
    const closed = records.filter((record) => record.status === 'closed');
    const totalInvested = records.reduce((acc, record) => acc + (Number(record.open_usdt_spent) || 0), 0);
    const totalProfit = closed.reduce((acc, record) => acc + (Number(record.profit_usdt) || 0), 0);
    const wins = closed.filter((record) => Number(record.profit_usdt) > 0).length;
    return {
        closedCount: closed.length,
        totalInvested,
        totalProfit,
        winRate: closed.length > 0 ? Math.round((wins / closed.length) * 100) : 0,
        wins,
    };
}

export default function TradeHistoryPage({ open = true, onClose, apiBaseUrl, initData, multiChainEnabled = true, embedded = false }) {
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
        } finally {
            setLoading(false);
        }
    }, [apiBaseUrl, initData, chain, status]);

    useEffect(() => {
        if (open) load(0);
    }, [open, load]);

    const summary = useMemo(() => aggregateSummary(records), [records]);
    const hasMore = records.length < total;
    const profitTone = summary.totalProfit > 0 ? 'positive' : summary.totalProfit < 0 ? 'negative' : 'default';

    return (
        <TradeHistoryFrame embedded={embedded} open={open} onClose={onClose} title="交易历史" maxHeightClass="max-h-[90vh]">
            <section className="rounded-2xl border border-zinc-200/80 bg-white p-3 dark:border-white/5 dark:bg-[#14171c]">
                <div className="flex items-start gap-3">
                    <div className="inline-flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-sky-500/10 text-sky-700 ring-1 ring-sky-500/20 dark:bg-sky-500/15 dark:text-sky-200 dark:ring-sky-500/25">
                        <BarChart3 className="h-5 w-5" />
                    </div>
                    <div className="min-w-0 flex-1">
                        <div className="text-[14px] font-extrabold leading-tight text-zinc-900 dark:text-white/95">交易历史</div>
                        <div className="mt-0.5 text-[10px] text-zinc-500 dark:text-white/40">
                            已加载 {records.length} / {total} 条记录
                        </div>
                    </div>
                </div>
                <div className="mt-3 grid grid-cols-2 gap-2">
                    <HistoryStat label="总投入" value={formatUsd(summary.totalInvested)} />
                    <HistoryStat label="净盈亏" value={summary.closedCount > 0 ? formatSignedUsd(summary.totalProfit) : '-'} tone={profitTone} />
                    <HistoryStat label="已结束" value={`${summary.closedCount} 笔`} />
                    <HistoryStat label="胜率" value={summary.closedCount > 0 ? `${summary.winRate}%` : '-'} />
                </div>
            </section>

            <section className="rounded-2xl border border-zinc-200/80 bg-white p-3 dark:border-white/5 dark:bg-[#14171c]">
                <div className="mb-2 flex items-center justify-between gap-2">
                    <div className="text-[12px] font-bold text-zinc-900 dark:text-white/90">筛选</div>
                    <div className="text-[10px] text-zinc-400 dark:text-white/30">{loading ? '刷新中...' : `${records.length} 条`}</div>
                </div>
                <div className="grid grid-cols-2 gap-2">
                    {multiChainEnabled ? (
                        <CustomSelect
                            value={chain}
                            onChange={setChain}
                            options={CHAIN_OPTIONS}
                            placeholder="链"
                        />
                    ) : null}
                    <CustomSelect
                        value={status}
                        onChange={setStatus}
                        options={STATUS_OPTIONS}
                        placeholder="状态"
                        className={multiChainEnabled ? '' : 'col-span-2'}
                    />
                </div>
            </section>

            {error ? (
                <div className="rounded-xl border border-red-500/30 bg-red-500/10 p-3 text-xs text-red-700 dark:text-red-300">
                    {error}
                </div>
            ) : null}

            {loading && records.length === 0 ? (
                <div className="flex items-center justify-center py-12 text-sm text-zinc-400 dark:text-white/40">
                    <svg className="mr-2 h-5 w-5 animate-spin" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                        <circle className="opacity-25" cx="12" cy="12" r="10" />
                        <path className="opacity-75" d="M4 12a8 8 0 018-8" />
                    </svg>
                    加载中...
                </div>
            ) : records.length === 0 ? (
                <div className="rounded-2xl border border-dashed border-zinc-200 bg-white/70 px-5 py-10 text-center text-sm text-zinc-400 dark:border-white/[0.08] dark:bg-white/[0.02] dark:text-white/40">
                    <BarChart3 className="mx-auto mb-3 h-9 w-9 opacity-45" />
                    <div className="font-bold text-zinc-700 dark:text-white/65">暂无交易记录</div>
                    <div className="mt-1 text-xs">当前筛选条件下没有记录。</div>
                </div>
            ) : (
                <div className="space-y-2.5">
                    {records.map((rec) => (
                        <TradeRecordCard key={rec.id} rec={rec} />
                    ))}

                    {hasMore ? (
                        <button
                            type="button"
                            onClick={() => load(offset + PAGE_SIZE)}
                            disabled={loading}
                            className={`w-full rounded-xl border border-zinc-200 bg-white px-4 py-3 text-sm font-bold text-zinc-600 transition-colors hover:bg-zinc-50 dark:border-white/5 dark:bg-[#14171c] dark:text-white/60 dark:hover:bg-white/[0.03] ${loading ? 'cursor-not-allowed opacity-50' : ''}`}
                        >
                            {loading ? '加载中...' : '加载更多'}
                        </button>
                    ) : null}
                </div>
            )}
        </TradeHistoryFrame>
    );
}

function TradeHistoryFrame({ embedded, children, ...sheetProps }) {
    if (embedded) {
        return <div className="space-y-3 pb-1">{children}</div>;
    }
    return <BottomSheet {...sheetProps}>{children}</BottomSheet>;
}

function HistoryStat({ label, value, tone = 'default' }) {
    const toneClass = tone === 'positive'
        ? 'text-emerald-600 dark:text-emerald-300'
        : tone === 'negative'
            ? 'text-red-500 dark:text-red-300'
            : 'text-zinc-900 dark:text-white/95';
    return (
        <div className="rounded-xl bg-zinc-50 px-3 py-2.5 ring-1 ring-zinc-200 dark:bg-white/[0.03] dark:ring-white/[0.06]">
            <div className="text-[9px] font-medium uppercase tracking-wide text-zinc-400 dark:text-white/35">{label}</div>
            <div className={`mt-1 truncate text-base font-extrabold leading-none tabular-nums ${toneClass}`}>{value}</div>
        </div>
    );
}

function TradeRecordCard({ rec }) {
    const st = STATUS_MAP[rec.status] || STATUS_MAP.open;
    const pair = [rec.token0_symbol, rec.token1_symbol].filter(Boolean).join('/') || '-';
    const profitNum = Number(rec.profit_usdt);
    const profitPositive = Number.isFinite(profitNum) && profitNum > 0;
    const profitNegative = Number.isFinite(profitNum) && profitNum < 0;
    const profitPctNum = Number(rec.profit_pct);
    const hasOpenSnapshot = Number(rec.open_stable_before || 0) > 0 || Number(rec.open_stable_after || 0) > 0;
    const hasCloseSnapshot = Number(rec.close_stable_before || 0) > 0 || Number(rec.close_stable_after || 0) > 0;
    const showProfit = rec.status === 'closed' && Number.isFinite(profitNum);
    const duration = formatDuration(rec.opened_at, rec.closed_at);
    const closeTxDetails = Array.isArray(rec.close_tx_details)
        ? rec.close_tx_details.map(formatCloseTxDetailLabel).filter(Boolean)
        : [];

    return (
        <div className="overflow-hidden rounded-2xl border border-zinc-200/80 bg-white p-3 dark:border-white/5 dark:bg-[#14171c]">
            <div className="flex items-start justify-between gap-3">
                <div className="min-w-0 flex-1">
                    <div className="flex min-w-0 items-center gap-2">
                        <span className="truncate text-sm font-extrabold text-zinc-900 dark:text-white/90">{pair}</span>
                        {rec.chain ? (
                            <span className="inline-flex shrink-0 items-center rounded-md bg-zinc-100 px-1.5 py-0.5 text-[10px] font-bold text-zinc-500 dark:bg-white/[0.06] dark:text-white/45">
                                {String(rec.chain).toUpperCase()}
                            </span>
                        ) : null}
                    </div>
                    <div className="mt-0.5 flex flex-wrap items-center gap-1.5 text-[10px] text-zinc-400 dark:text-white/30">
                        <span>{rec.exchange || '-'}</span>
                        {rec.pool_version ? <span>/ {rec.pool_version}</span> : null}
                        {duration !== '-' ? (
                            <span className="inline-flex items-center gap-1 rounded-md bg-zinc-100 px-1.5 py-0.5 dark:bg-white/[0.05]">
                                <Clock3 className="h-3 w-3" />
                                {duration}
                            </span>
                        ) : null}
                    </div>
                </div>
                <span className={`inline-flex shrink-0 items-center rounded-lg px-2 py-1 text-[11px] font-bold ring-1 ${st.chip}`}>
                    {st.text}
                </span>
            </div>

            {showProfit ? (
                <div className={`mt-3 rounded-xl px-3 py-2.5 ring-1 ${profitPositive ? 'bg-emerald-500/[0.06] ring-emerald-500/15 dark:bg-emerald-500/[0.08]' : profitNegative ? 'bg-red-500/[0.06] ring-red-500/15 dark:bg-red-500/[0.08]' : 'bg-zinc-50 ring-zinc-200 dark:bg-white/[0.03] dark:ring-white/[0.06]'}`}>
                    <div className="flex items-center justify-between gap-2">
                        <div className="text-[9px] font-medium uppercase tracking-wide text-zinc-400 dark:text-white/35">收益(扣 Gas)</div>
                        {profitPositive ? <TrendingUp className="h-4 w-4 text-emerald-500" /> : profitNegative ? <TrendingDown className="h-4 w-4 text-red-500" /> : null}
                    </div>
                    <div className={`mt-1 flex flex-wrap items-baseline gap-2 text-base font-extrabold leading-none tabular-nums ${profitPositive ? 'text-emerald-600 dark:text-emerald-300' : profitNegative ? 'text-red-500 dark:text-red-300' : 'text-zinc-900 dark:text-white/95'}`}>
                        <span>{formatSignedUsd(profitNum)}</span>
                        {Number.isFinite(profitPctNum) ? (
                            <span className="rounded-md bg-white/70 px-1.5 py-0.5 text-[10px] dark:bg-white/[0.06]">
                                {profitPctNum >= 0 ? '+' : ''}{profitPctNum.toFixed(2)}%
                            </span>
                        ) : null}
                    </div>
                </div>
            ) : null}

            <div className="mt-3 grid grid-cols-2 gap-2">
                <DetailTile label="开仓时间" value={rec.opened_at || '-'} />
                <DetailTile label="投入 USDT" value={formatUsd(rec.open_usdt_spent)} />
                {hasOpenSnapshot ? (
                    <>
                        <DetailTile label="开仓前余额" value={formatUsd(rec.open_stable_before)} />
                        <DetailTile label="开仓后余额" value={formatUsd(rec.open_stable_after)} />
                    </>
                ) : null}
                {rec.closed_at ? (
                    <>
                        <DetailTile label="撤仓时间" value={rec.closed_at} />
                        <DetailTile label="撤出 USDT" value={formatUsd(rec.close_usdt_received)} />
                    </>
                ) : null}
                {hasCloseSnapshot ? (
                    <>
                        <DetailTile label="撤仓前余额" value={formatUsd(rec.close_stable_before)} />
                        <DetailTile label="撤仓后余额" value={formatUsd(rec.close_stable_after)} />
                    </>
                ) : null}
                {rec.status === 'closed' ? (
                    <DetailTile label="Gas 费用" value={formatUsd(rec.total_gas_usdt)} />
                ) : null}
            </div>

            {closeTxDetails.length > 0 ? (
                <div className="mt-3 flex flex-wrap gap-1.5 border-t border-zinc-100 pt-3 dark:border-white/5">
                    {closeTxDetails.map((detail, index) => (
                        <span
                            key={`${detail}-${index}`}
                            className="inline-flex items-center rounded-lg bg-emerald-500/10 px-2.5 py-1.5 text-[11px] font-bold text-emerald-700 ring-1 ring-emerald-500/15 dark:bg-emerald-400/10 dark:text-emerald-200 dark:ring-emerald-400/20"
                        >
                            撤仓路由：{detail}
                        </span>
                    ))}
                </div>
            ) : null}

            {(rec.open_tx_url || rec.close_tx_url) ? (
                <div className="mt-3 flex flex-wrap gap-2 border-t border-zinc-100 pt-3 dark:border-white/5">
                    {rec.open_tx_url ? (
                        <TxLink href={rec.open_tx_url}>开仓 Tx</TxLink>
                    ) : null}
                    {rec.close_tx_url ? (
                        <TxLink href={rec.close_tx_url}>撤仓 Tx</TxLink>
                    ) : null}
                </div>
            ) : null}
        </div>
    );
}

function DetailTile({ label, value }) {
    return (
        <div className="min-w-0 rounded-xl bg-zinc-50 px-3 py-2.5 ring-1 ring-zinc-200 dark:bg-white/[0.03] dark:ring-white/[0.06]">
            <div className="text-[9px] font-medium uppercase tracking-wide text-zinc-400 dark:text-white/35">{label}</div>
            <div className="mt-1 truncate text-[12px] font-bold text-zinc-800 dark:text-white/80">{value}</div>
        </div>
    );
}

function TxLink({ href, children }) {
    return (
        <a
            href={href}
            target="_blank"
            rel="noopener noreferrer"
            className="inline-flex items-center gap-1.5 rounded-lg bg-zinc-100 px-2.5 py-1.5 text-[11px] font-bold text-zinc-600 transition hover:bg-zinc-200 dark:bg-white/[0.06] dark:text-white/60 dark:hover:bg-white/10"
        >
            <ExternalLink className="h-3.5 w-3.5" />
            {children}
        </a>
    );
}
