import React, { useMemo, useState } from 'react';
import { copyToClipboard } from '../lib/telegram';

const Icon = ({ path, className = '' }) => (
    <svg viewBox="0 0 24 24" fill="currentColor" className={className} aria-hidden="true">
        <path d={path} />
    </svg>
);

const icons = {
    copy: 'M16 1H4a2 2 0 00-2 2v14h2V3h12V1zm3 4H8a2 2 0 00-2 2v14a2 2 0 002 2h11a2 2 0 002-2V7a2 2 0 00-2-2zm0 16H8V7h11v14z',
    chart: 'M5 3v18h18v-2H7V3H5zm5 14H8v-6h2v6zm4 0h-2V7h2v10zm2 0h2v-4h-2v4z',
    arrowUp: 'M5.293 9.707a1 1 0 010-1.414l4-4a1 1 0 011.414 0l4 4a1 1 0 01-1.414 1.414L11 7.414V15a1 1 0 11-2 0V7.414L6.707 9.707a1 1 0 01-1.414 0z',
    arrowDown: 'M14.707 10.293a1 1 0 010 1.414l-4 4a1 1 0 01-1.414 0l-4-4a1 1 0 111.414-1.414L9 12.586V5a1 1 0 012 0v7.586l2.293-2.293a1 1 0 011.414 0z',
};

const usdCompact = new Intl.NumberFormat('en-US', {
    style: 'currency',
    currency: 'USD',
    notation: 'compact',
    maximumFractionDigits: 2,
});

function formatUsd(v) {
    const n = Number(v || 0);
    if (!Number.isFinite(n)) return '$0.00';
    return `$${n.toFixed(2)}`;
}

function formatUsdCompact(v) {
    const n = Number(v || 0);
    if (!Number.isFinite(n)) return '$0.00';
    return usdCompact.format(n);
}

function formatFeePercent(v) {
    const n = Number(v || 0);
    if (!Number.isFinite(n) || n <= 0) return '';
    return `${n.toFixed(2).replace(/\.?0+$/, '')}%`;
}

function formatRatePct(v) {
    const n = Number(v || 0);
    if (!Number.isFinite(n)) return '<0.01%';
    if (Math.abs(n) < 0.01) return '<0.01%';
    return `${n.toFixed(3)}%`;
}

function normalizeDexName(dex) {
    const v = String(dex || '').trim().toLowerCase();
    if (!v) return '';
    if (v.includes('pancake') || v === 'pcs') return 'pancake';
    if (v.includes('uniswap') || v === 'uni') return 'uniswap';
    if (v.includes('sushi')) return 'sushi';
    return v.replace(/[^a-z0-9]+/g, '');
}

function normalizeProtocolVersion(protocolVersion, dex) {
    const proto = String(protocolVersion || '').trim().toLowerCase();
    const fromProto = proto.match(/v?\d+/)?.[0] ?? '';
    if (fromProto) return fromProto.startsWith('v') ? fromProto : `v${fromProto}`;
    const dx = String(dex || '').trim().toLowerCase();
    const fromDex = dx.match(/v\d+/)?.[0] ?? '';
    return fromDex;
}

function dexLabel(pool) {
    // 优先使用 factory_name
    const factoryName = String(pool?.factory_name || '').trim();
    if (factoryName) {
        return factoryName;
    }
    // 回退到原来的逻辑
    const base = normalizeDexName(pool?.dex);
    const version = normalizeProtocolVersion(pool?.protocol_version, pool?.dex);
    if (!base && !version) return 'DEX';
    if (!base) return version.toUpperCase();
    return `${base}${version || ''}`;
}

function formatPairLabel(tradingPair) {
    const v = String(tradingPair || '').trim();
    if (!v) return '--';
    return v.replace(/\//g, '/\u200B');
}

// 通用变化指示器组件 - 用于显示数值变化（费用、交易量等）
const ChangeIndicator = ({ currentValue, previousValue, label = '变化' }) => {
    if (previousValue === undefined || previousValue === null) return null;

    const current = Number(currentValue || 0);
    const previous = Number(previousValue || 0);
    const diff = current - previous;
    if (diff === 0 || !Number.isFinite(diff)) return null;

    const isIncrease = diff > 0;
    const absValue = Math.abs(diff);

    // 格式化数字显示
    const formatValue = (val) => {
        if (val >= 1000) {
            return usdCompact.format(val).replace('$', '');
        }
        return val.toFixed(2);
    };

    return (
        <span
            className={`ml-1 inline-flex items-center text-[10px] font-bold ${isIncrease ? 'text-emerald-600 dark:text-emerald-400' : 'text-rose-600 dark:text-rose-400'
                }`}
            title={`${label}: ${isIncrease ? '+' : '-'}$${absValue.toFixed(2)}`}
        >
            <svg className="w-2.5 h-2.5" fill="currentColor" viewBox="0 0 20 20">
                <path fillRule="evenodd" d={isIncrease ? icons.arrowUp : icons.arrowDown} clipRule="evenodd" />
            </svg>
            <span>{formatValue(absValue)}</span>
        </span>
    );
};

export default function HotPoolCard({ pool, metric, previousData, onOpenKline }) {
    const [copied, setCopied] = useState(false);
    const addr = String(pool?.pool_address || '').trim();
    const canOpenKline = useMemo(() => /^0x[a-fA-F0-9]{40}$/.test(addr), [addr]);

    const mainValue = useMemo(() => {
        if (metric === 'volume') return formatUsdCompact(pool?.total_volume);
        if (metric === 'fee_rate') return formatRatePct(pool?.fee_rate);
        return formatUsd(pool?.total_fees);
    }, [metric, pool?.fee_rate, pool?.total_fees, pool?.total_volume]);

    const priceDisplay = useMemo(() => {
        const v = String(pool?.price_display || '').trim();
        return v ? v : '';
    }, [pool?.price_display]);

    const priceDisplayClass = useMemo(() => {
        if (!priceDisplay) return '';
        if (priceDisplay.includes('↓') || priceDisplay.includes('-')) return 'text-rose-600 dark:text-rose-300';
        if (priceDisplay.includes('↑') || priceDisplay.includes('+')) return 'text-emerald-700 dark:text-emerald-300';
        return 'text-zinc-600 dark:text-white/60';
    }, [priceDisplay]);

    const copyAddr = async () => {
        if (!addr) return;
        try {
            await copyToClipboard(addr);
            setCopied(true);
            setTimeout(() => setCopied(false), 1200);
        } catch {
            // ignore
        }
    };

    return (
        <div className="rounded-2xl border border-zinc-200 bg-white p-4 shadow-sm dark:border-white/10 dark:bg-[#111318] dark:shadow-none">
            <div className="flex items-start justify-between gap-3">
                <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-2">
                        <div
                            className="max-w-[90px] text-xs font-semibold leading-4 text-zinc-900 dark:text-white/90 truncate"
                            title={pool?.trading_pair || ''}
                        >
                            {formatPairLabel(pool?.trading_pair)}
                        </div>
                        {pool?.fee_percentage ? (
                            <div className="rounded-lg bg-sky-500/10 px-2 py-0.5 text-[11px] font-semibold text-sky-700 ring-1 ring-sky-500/20 dark:bg-sky-500/15 dark:text-sky-200 dark:ring-sky-500/30">
                                {formatFeePercent(pool.fee_percentage)}
                            </div>
                        ) : null}
                        <button
                            type="button"
                            onClick={copyAddr}
                            className={`inline-flex h-7 w-7 items-center justify-center rounded-xl border text-zinc-600 shadow-sm transition ${copied
                                ? 'border-emerald-500/30 bg-emerald-500/10 text-emerald-700 dark:border-emerald-500/30 dark:bg-emerald-500/15 dark:text-emerald-200'
                                : 'border-zinc-200 bg-zinc-100 hover:bg-zinc-200 active:bg-zinc-200 dark:border-white/10 dark:bg-white/5 dark:text-white/70 dark:hover:bg-white/10 dark:active:bg-white/15'
                                }`}
                            aria-label={copied ? '已复制' : '复制地址'}
                            disabled={!addr}
                        >
                            <Icon path={icons.copy} className="h-4 w-4" />
                        </button>
                        <button
                            type="button"
                            onClick={() => onOpenKline?.(pool)}
                            className="inline-flex h-7 w-7 items-center justify-center rounded-xl border border-zinc-200 bg-zinc-100 text-zinc-600 shadow-sm transition hover:bg-zinc-200 active:bg-zinc-200 disabled:opacity-40 dark:border-white/10 dark:bg-white/5 dark:text-white/70 dark:hover:bg-white/10 dark:active:bg-white/15"
                            aria-label="K线图"
                            title="查看K线图"
                            disabled={!canOpenKline || typeof onOpenKline !== 'function'}
                        >
                            <Icon path={icons.chart} className="h-4 w-4" />
                        </button>
                    </div>

                    <div className="mt-2 flex flex-wrap items-center gap-x-4 gap-y-1 text-xs">
                        <div className="text-zinc-500 dark:text-white/40 flex items-center">
                            交易量:{' '}
                            <span className="font-semibold text-sky-600 dark:text-sky-200 tabular-nums">
                                {formatUsdCompact(pool?.total_volume)}
                            </span>
                            <ChangeIndicator
                                currentValue={pool?.total_volume}
                                previousValue={previousData?.total_volume}
                                label="交易量变化"
                            />
                        </div>
                        <div className="text-zinc-500 dark:text-white/40 flex items-center">
                            TVL:{' '}
                            <span className="font-semibold text-zinc-900 dark:text-white/80 tabular-nums">
                                {formatUsdCompact(pool?.current_pool_value)}
                            </span>
                            <ChangeIndicator
                                currentValue={pool?.current_pool_value}
                                previousValue={previousData?.current_pool_value}
                                label="TVL变化"
                            />
                        </div>
                        {pool?.transaction_count > 0 ? (
                            <div className="text-zinc-500 dark:text-white/40 flex items-center">
                                交易笔数:{' '}
                                <span className="font-semibold text-orange-600 dark:text-orange-300 tabular-nums">
                                    {pool.transaction_count.toLocaleString()}
                                </span>
                            </div>
                        ) : null}
                    </div>
                </div>

                <div className="text-right shrink-0 min-w-[110px]">
                    <div className="flex items-baseline justify-end gap-1 flex-wrap">
                        <div className="text-base font-extrabold text-emerald-700 dark:text-emerald-300 tabular-nums flex items-center">
                            {mainValue}
                            <ChangeIndicator
                                currentValue={metric === 'volume' ? pool?.total_volume : pool?.total_fees}
                                previousValue={metric === 'volume' ? previousData?.total_volume : previousData?.total_fees}
                                label={metric === 'volume' ? '交易量变化' : '费用变化'}
                            />
                        </div>
                    </div>
                    {priceDisplay ? (
                        <div
                            className={`mt-0.5 text-[10px] font-semibold tabular-nums truncate max-w-[110px] ${priceDisplayClass}`}
                            title={priceDisplay}
                        >
                            {priceDisplay}
                        </div>
                    ) : null}
                    <div className="mt-0.5 text-[10px] font-semibold text-violet-600 dark:text-violet-300 tabular-nums">
                        {metric === 'fee_rate' ? formatUsd(pool?.total_fees) : formatRatePct(pool?.fee_rate)}
                    </div>
                </div>
            </div>

            <div className="mt-3 flex items-center">
                <div className="inline-flex items-center rounded-lg bg-amber-500/15 px-2 py-0.5 text-[11px] font-semibold text-amber-800 ring-1 ring-amber-500/25 dark:bg-amber-500/15 dark:text-amber-200 dark:ring-amber-500/30">
                    {dexLabel(pool)}
                </div>
            </div>
        </div>
    );
}
