import BottomSheet from '../BottomSheet.jsx';
import { formatQuoteGasCostSummary, formatTokenAmount } from '../../lib/swapMeta';

function Row({ label, value, accent = false }) {
    return (
        <div className="flex items-center justify-between gap-3 py-1.5 text-[12px]">
            <span className="text-zinc-500 dark:text-white/45">{label}</span>
            <span
                className={`break-all text-right font-medium tabular-nums ${
                    accent ? 'text-zinc-900 dark:text-white/90' : 'text-zinc-700 dark:text-white/75'
                }`}
            >
                {value || '--'}
            </span>
        </div>
    );
}

function quoteSelectionKey(entry) {
    return String(entry?.quote_id || entry?.provider || '').trim();
}

function quoteRouteList(quote) {
    if (Array.isArray(quote?.quotes) && quote.quotes.length > 0) return quote.quotes;
    return quote ? [quote] : [];
}

function ProviderCard({ entry, isBest, isSelected, toSymbol, nativeSymbol, onSelect }) {
    const provider = entry?.vendor_name
        ? `${entry?.provider_label || entry?.provider || '--'} · ${entry.vendor_name}`
        : (entry?.provider_label || entry?.provider || entry?.source || '--');
    const status = entry?.status || (entry?.error ? 'error' : 'available');
    const toAmount = entry?.net_to_amount_float || entry?.to_amount_float || formatTokenAmount(entry?.to_amount_human || 0);
    const minReceived = entry?.min_to_amount_float || entry?.min_received_float;
    const priceImpact = String(entry?.price_impact_percent ?? '').trim();
    const routeKey = quoteSelectionKey(entry);
    return (
        <button
            type="button"
            onClick={() => (routeKey ? onSelect?.(routeKey) : null)}
            disabled={!routeKey}
            className={`w-full rounded-2xl border p-3 text-left transition active:scale-[0.99] ${
                isSelected
                    ? 'border-zinc-900 bg-zinc-50 dark:border-white dark:bg-white/5'
                    : 'border-zinc-200 bg-white dark:border-white/10 dark:bg-white/[0.02]'
            }`}
        >
            <div className="flex items-center justify-between gap-2">
                <div className="flex items-center gap-2">
                    <span className="text-[13px] font-bold uppercase text-zinc-900 dark:text-white/90">{provider}</span>
                    {isSelected ? (
                        <span className="rounded bg-zinc-900 px-1.5 py-0.5 text-[9px] font-bold text-white dark:bg-white dark:text-zinc-900">
                            SELECTED
                        </span>
                    ) : isBest ? (
                        <span className="rounded bg-zinc-900 px-1.5 py-0.5 text-[9px] font-bold text-white dark:bg-white dark:text-zinc-900">
                            BEST
                        </span>
                    ) : null}
                </div>
                <span
                    className={`rounded px-1.5 py-0.5 text-[9px] font-bold ${
                        status === 'available'
                            ? 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-300'
                            : 'bg-red-500/15 text-red-700 dark:text-red-300'
                    }`}
                >
                    {status === 'available' ? '可用' : status}
                </span>
            </div>
            <div className="mt-2 text-[18px] font-black tabular-nums text-zinc-900 dark:text-white/95">
                {toAmount} <span className="text-[11px] font-medium text-zinc-400">{toSymbol || ''}</span>
            </div>
            <div className="mt-2 space-y-0.5 text-[11px]">
                {minReceived ? (
                    <Row label="最少收到" value={`${minReceived} ${toSymbol || ''}`} />
                ) : null}
                {entry?.estimated_gas_native || entry?.estimated_gas_usd ? (
                    <Row label="Gas" value={formatQuoteGasCostSummary(entry, nativeSymbol)} />
                ) : null}
                {entry?.route_summary ? (
                    <Row label="路径" value={entry.route_summary} />
                ) : null}
                {priceImpact ? (
                    <Row
                        label="价格冲击"
                        value={`${Number(priceImpact).toFixed(2)}%`}
                    />
                ) : null}
                {entry?.error ? (
                    <div className="mt-1 break-all rounded-lg bg-red-500/10 px-2 py-1 text-[10px] text-red-600 dark:text-red-300">
                        {entry.error}
                    </div>
                ) : null}
            </div>
        </button>
    );
}

export default function SwapQuoteDetails({
    open,
    onClose,
    quote,
    selectedQuoteKey,
    onSelectQuote,
    fromToken,
    toToken,
    nativeSymbol,
}) {
    const providers = quoteRouteList(quote);
    const bestQuoteID = String(quote?.best_quote_id || '').trim();
    const selectedKey = selectedQuoteKey || bestQuoteID || quoteSelectionKey(providers.find((entry) => entry?.status === 'available') || providers[0]);
    const selected = providers.find((entry) => quoteSelectionKey(entry) === selectedKey) || providers[0] || quote;
    const priceImpact = String(selected?.price_impact_percent ?? '').trim();

    return (
        <BottomSheet open={open} onClose={onClose} title="报价详情" maxHeightClass="max-h-[88vh]">
            <div className="space-y-3">
                <div className="rounded-2xl border border-zinc-200 bg-zinc-50 p-3 dark:border-white/10 dark:bg-white/5">
                    <Row label="兑换" value={`${fromToken?.symbol || '--'} → ${toToken?.symbol || '--'}`} accent />
                    {quote?.from_amount_float ? (
                        <Row label="支付" value={`${quote.from_amount_float} ${fromToken?.symbol || ''}`} />
                    ) : null}
                    {selected?.net_to_amount_float || quote?.to_amount_float ? (
                        <Row label="到账估算" value={`${selected?.net_to_amount_float || quote.to_amount_float} ${toToken?.symbol || ''}`} accent />
                    ) : null}
                    {selected?.min_to_amount_float || quote?.min_to_amount_float ? (
                        <Row label="最少收到" value={`${selected?.min_to_amount_float || quote.min_to_amount_float} ${toToken?.symbol || ''}`} />
                    ) : null}
                    {priceImpact ? (
                        <Row label="价格冲击" value={`${Number(priceImpact).toFixed(2)}%`} />
                    ) : null}
                    {selected?.estimated_gas_native || selected?.estimated_gas_usd || quote?.estimated_gas_native || quote?.estimated_gas_usd ? (
                        <Row label="Gas 估算" value={formatQuoteGasCostSummary(selected || quote, nativeSymbol)} />
                    ) : null}
                    {quote?.exchange_rate ? (
                        <Row
                            label="汇率"
                            value={`1 ${fromToken?.symbol || ''} ≈ ${Number(quote.exchange_rate).toLocaleString('en-US', { maximumFractionDigits: 8 })} ${toToken?.symbol || ''}`}
                        />
                    ) : null}
                </div>

                {providers.length > 0 ? (
                    <div className="space-y-2">
                        <div className="text-[10px] font-bold uppercase tracking-[0.18em] text-zinc-400 dark:text-white/35">
                            路由报价 · {providers.length}
                        </div>
                        {providers.map((entry, idx) => (
                            <ProviderCard
                                key={quoteSelectionKey(entry) || `${entry?.provider || idx}`}
                                entry={entry}
                                isBest={
                                    bestQuoteID
                                        ? String(entry?.quote_id || '').trim() === bestQuoteID
                                        : Boolean(entry?.recommended) || idx === 0
                                }
                                isSelected={quoteSelectionKey(entry) === selectedKey}
                                onSelect={onSelectQuote}
                                fromSymbol={fromToken?.symbol}
                                toSymbol={toToken?.symbol}
                                nativeSymbol={nativeSymbol}
                            />
                        ))}
                    </div>
                ) : null}

                {!quote ? (
                    <div className="rounded-xl border border-dashed border-zinc-200 p-6 text-center text-xs text-zinc-400 dark:border-white/10 dark:text-white/35">
                        暂无报价数据
                    </div>
                ) : null}
            </div>
        </BottomSheet>
    );
}
