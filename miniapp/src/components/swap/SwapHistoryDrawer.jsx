import { useCallback, useEffect, useState } from 'react';
import { ArrowDown, RefreshCw } from 'lucide-react';
import BottomSheet from '../BottomSheet.jsx';
import ConfirmDialog from '../ConfirmDialog.jsx';
import {
    cancelWalletSwapLimitOrder,
    fetchWalletSwapHistory,
    fetchWalletSwapLimitOrders,
} from '../../lib/api';
import { shortAddress } from '../../lib/swapMeta';

const TABS = [
    { key: 'history', label: '兑换历史' },
    { key: 'limit', label: '限价单' },
];

function formatTime(value) {
    if (!value) return '--';
    if (typeof value === 'string' && value.length > 12) return value;
    const d = new Date(value);
    if (Number.isNaN(d.getTime())) return String(value);
    return d.toLocaleString();
}

function HistoryRow({ item }) {
    const fromSym = item?.from_token?.symbol || '--';
    const toSym = item?.to_token?.symbol || '--';
    const status = String(item?.status || 'confirmed').toLowerCase();
    const statusClass = status === 'confirmed' || status === 'success'
        ? 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-300'
        : status === 'failed' || status === 'error'
            ? 'bg-red-500/15 text-red-700 dark:text-red-300'
            : 'bg-amber-500/15 text-amber-700 dark:text-amber-300';
    return (
        <div className="rounded-2xl border border-zinc-200 bg-white p-3 dark:border-white/10 dark:bg-white/[0.03]">
            <div className="flex items-center justify-between gap-2">
                <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-1.5 text-[13px] font-bold text-zinc-900 dark:text-white/90">
                        <span className="tabular-nums">{item?.amount_in_float || '--'}</span>
                        <span className="text-zinc-500 dark:text-white/45">{fromSym}</span>
                        <ArrowDown size={11} className="rotate-[-90deg] text-zinc-400" />
                        <span className="tabular-nums">{item?.amount_out_float || '--'}</span>
                        <span className="text-zinc-500 dark:text-white/45">{toSym}</span>
                    </div>
                    <div className="mt-1 flex items-center gap-1.5 text-[10px] text-zinc-500 dark:text-white/45">
                        <span>{formatTime(item?.created_at)}</span>
                        {item?.provider_label ? (
                            <span className="rounded bg-zinc-100 px-1 py-0.5 text-[9px] font-semibold dark:bg-white/10">
                                {item.provider_label}
                            </span>
                        ) : null}
                    </div>
                </div>
                <div className="flex shrink-0 flex-col items-end gap-1">
                    <span className={`rounded px-1.5 py-0.5 text-[9px] font-bold ${statusClass}`}>
                        {status}
                    </span>
                    {item?.tx_url ? (
                        <a
                            href={item.tx_url}
                            target="_blank"
                            rel="noreferrer"
                            className="font-mono text-[10px] text-zinc-500 underline decoration-dotted dark:text-white/40"
                        >
                            {shortAddress(item?.tx_hash, 6, 4)}
                        </a>
                    ) : item?.tx_hash ? (
                        <span className="font-mono text-[10px] text-zinc-400 dark:text-white/35">
                            {shortAddress(item.tx_hash, 6, 4)}
                        </span>
                    ) : null}
                </div>
            </div>
        </div>
    );
}

function LimitOrderRow({ order, onCancel, busy }) {
    const open = String(order?.status || '').toLowerCase() === 'open';
    const fromSym = order?.from_token?.symbol || '--';
    const toSym = order?.to_token?.symbol || '--';
    return (
        <div className="rounded-2xl border border-zinc-200 bg-white p-3 dark:border-white/10 dark:bg-white/[0.03]">
            <div className="flex items-start justify-between gap-2">
                <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-1.5 text-[13px] font-bold text-zinc-900 dark:text-white/90">
                        <span className="tabular-nums">{order?.from_amount_float || '--'}</span>
                        <span className="text-zinc-500 dark:text-white/45">{fromSym}</span>
                        <ArrowDown size={11} className="rotate-[-90deg] text-zinc-400" />
                        <span className="tabular-nums">{order?.target_to_amount_float || '--'}</span>
                        <span className="text-zinc-500 dark:text-white/45">{toSym}</span>
                    </div>
                    <div className="mt-1 flex flex-wrap items-center gap-1.5 text-[10px] text-zinc-500 dark:text-white/45">
                        <span
                            className={`rounded px-1.5 py-0.5 text-[9px] font-bold ${
                                open
                                    ? 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-300'
                                    : 'bg-zinc-200 text-zinc-600 dark:bg-white/10 dark:text-white/55'
                            }`}
                        >
                            {order?.status || '--'}
                        </span>
                        {order?.provider_label ? <span>{order.provider_label}</span> : null}
                        {order?.created_at ? <span>{formatTime(order.created_at)}</span> : null}
                    </div>
                    {order?.last_quote_to_amount_float ? (
                        <div className="mt-1 text-[10px] text-zinc-400 dark:text-white/35">
                            最近报价 {order.last_quote_to_amount_float} {toSym} · {formatTime(order.last_checked_at)}
                        </div>
                    ) : null}
                    {order?.last_error ? (
                        <div className="mt-1 break-all rounded-lg bg-red-500/10 px-2 py-1 text-[10px] text-red-600 dark:text-red-300">
                            {order.last_error}
                        </div>
                    ) : null}
                    {order?.tx_url ? (
                        <a
                            href={order.tx_url}
                            target="_blank"
                            rel="noreferrer"
                            className="mt-1 inline-block font-mono text-[10px] text-zinc-500 underline decoration-dotted dark:text-white/40"
                        >
                            {shortAddress(order.tx_hash, 8, 6)}
                        </a>
                    ) : null}
                </div>
                <button
                    type="button"
                    onClick={() => onCancel?.(order)}
                    disabled={!open || busy}
                    className={`shrink-0 rounded-xl border px-2.5 py-1.5 text-[11px] font-bold transition ${
                        open && !busy
                            ? 'border-red-500/30 bg-red-500/10 text-red-600 hover:bg-red-500/15 dark:text-red-300'
                            : 'border-zinc-200 text-zinc-400 dark:border-white/10 dark:text-white/30'
                    }`}
                >
                    {busy ? '...' : open ? '取消' : '已结束'}
                </button>
            </div>
        </div>
    );
}

export default function SwapHistoryDrawer({
    open,
    onClose,
    apiBaseUrl,
    initData,
    chain,
    walletId,
    onNotice,
}) {
    const [tab, setTab] = useState('history');

    const [history, setHistory] = useState([]);
    const [historyLoading, setHistoryLoading] = useState(false);
    const [historyError, setHistoryError] = useState('');

    const [orders, setOrders] = useState([]);
    const [ordersLoading, setOrdersLoading] = useState(false);
    const [ordersError, setOrdersError] = useState('');

    const [busyOrderId, setBusyOrderId] = useState('');
    const [confirmCancel, setConfirmCancel] = useState(null);

    const loadHistory = useCallback(async () => {
        if (!initData || !walletId) return;
        setHistoryLoading(true);
        setHistoryError('');
        try {
            const resp = await fetchWalletSwapHistory({
                apiBaseUrl,
                initData,
                chain,
                walletId,
                limit: 30,
            });
            setHistory(Array.isArray(resp?.items) ? resp.items : []);
        } catch (e) {
            setHistoryError(String(e?.message || e));
        } finally {
            setHistoryLoading(false);
        }
    }, [apiBaseUrl, initData, chain, walletId]);

    const loadOrders = useCallback(async () => {
        if (!initData || !walletId) return;
        setOrdersLoading(true);
        setOrdersError('');
        try {
            const resp = await fetchWalletSwapLimitOrders({
                apiBaseUrl,
                initData,
                chain,
                walletId,
                limit: 30,
            });
            setOrders(Array.isArray(resp?.items) ? resp.items : []);
        } catch (e) {
            setOrdersError(String(e?.message || e));
        } finally {
            setOrdersLoading(false);
        }
    }, [apiBaseUrl, initData, chain, walletId]);

    useEffect(() => {
        if (!open) return;
        if (tab === 'history') loadHistory();
        if (tab === 'limit') loadOrders();
    }, [open, tab, loadHistory, loadOrders]);

    const handleCancel = useCallback(async () => {
        const order = confirmCancel;
        if (!order?.id) return;
        setBusyOrderId(String(order.id));
        try {
            await cancelWalletSwapLimitOrder({
                apiBaseUrl,
                initData,
                chain,
                orderId: order.id,
            });
            onNotice?.('限价单已取消');
            setConfirmCancel(null);
            await loadOrders();
        } catch (e) {
            onNotice?.(String(e?.message || e));
        } finally {
            setBusyOrderId('');
        }
    }, [confirmCancel, apiBaseUrl, initData, chain, loadOrders, onNotice]);

    const activeLoading = tab === 'history' ? historyLoading : ordersLoading;
    const refresh = () => (tab === 'history' ? loadHistory() : loadOrders());

    return (
        <>
            <BottomSheet
                open={open}
                onClose={onClose}
                title="兑换记录"
                maxHeightClass="max-h-[92vh]"
                headerRight={(
                    <button
                        type="button"
                        onClick={refresh}
                        disabled={activeLoading || !walletId}
                        className="inline-flex h-8 items-center gap-1 rounded-full bg-zinc-100 px-3 text-[11px] font-bold text-zinc-700 disabled:opacity-40 dark:bg-white/10 dark:text-white/75"
                    >
                        <RefreshCw size={12} className={activeLoading ? 'animate-spin' : undefined} />
                        刷新
                    </button>
                )}
            >
                <div className="grid grid-cols-2 gap-1 rounded-2xl bg-zinc-100 p-1 dark:bg-white/5">
                    {TABS.map((item) => (
                        <button
                            key={item.key}
                            type="button"
                            onClick={() => setTab(item.key)}
                            className={`rounded-xl py-2 text-[12px] font-bold transition ${
                                tab === item.key
                                    ? 'bg-zinc-900 text-white shadow-sm dark:bg-white dark:text-zinc-900'
                                    : 'text-zinc-600 dark:text-white/55'
                            }`}
                        >
                            {item.label}
                        </button>
                    ))}
                </div>

                <div className="mt-3 space-y-2">
                    {tab === 'history' ? (
                        <>
                            {historyError ? (
                                <div className="rounded-xl border border-red-500/30 bg-red-500/10 p-3 text-xs text-red-700 dark:text-red-200">
                                    {historyError}
                                </div>
                            ) : null}
                            {!historyError && historyLoading && history.length === 0 ? (
                                <div className="rounded-xl border border-zinc-200 p-4 text-center text-xs text-zinc-400 dark:border-white/10 dark:text-white/35">
                                    正在加载兑换历史…
                                </div>
                            ) : null}
                            {!historyError && !historyLoading && history.length === 0 ? (
                                <div className="rounded-xl border border-dashed border-zinc-200 p-6 text-center text-xs text-zinc-400 dark:border-white/10 dark:text-white/35">
                                    当前钱包暂无兑换记录
                                </div>
                            ) : null}
                            {history.map((item) => (
                                <HistoryRow key={item.id || item.tx_hash} item={item} />
                            ))}
                        </>
                    ) : (
                        <>
                            {ordersError ? (
                                <div className="rounded-xl border border-red-500/30 bg-red-500/10 p-3 text-xs text-red-700 dark:text-red-200">
                                    {ordersError}
                                </div>
                            ) : null}
                            {!ordersError && ordersLoading && orders.length === 0 ? (
                                <div className="rounded-xl border border-zinc-200 p-4 text-center text-xs text-zinc-400 dark:border-white/10 dark:text-white/35">
                                    正在加载限价单…
                                </div>
                            ) : null}
                            {!ordersError && !ordersLoading && orders.length === 0 ? (
                                <div className="rounded-xl border border-dashed border-zinc-200 p-6 text-center text-xs text-zinc-400 dark:border-white/10 dark:text-white/35">
                                    当前钱包暂无限价单
                                </div>
                            ) : null}
                            {orders.map((order) => (
                                <LimitOrderRow
                                    key={order.id}
                                    order={order}
                                    busy={String(order.id) === busyOrderId}
                                    onCancel={(o) => setConfirmCancel(o)}
                                />
                            ))}
                        </>
                    )}
                </div>
            </BottomSheet>
            <ConfirmDialog
                open={Boolean(confirmCancel)}
                title="取消限价单？"
                message={confirmCancel
                    ? `将取消 ${confirmCancel?.from_token?.symbol || ''} → ${confirmCancel?.to_token?.symbol || ''} 的限价单，未成交部分将释放。`
                    : ''}
                confirmText="确认取消"
                cancelText="保留"
                danger
                loading={Boolean(busyOrderId)}
                onConfirm={handleCancel}
                onCancel={() => setConfirmCancel(null)}
            />
        </>
    );
}
