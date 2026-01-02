import React, { useMemo, useState } from 'react';
import { copyToClipboard, openLink } from '../lib/telegram';

const Icon = ({ path, className = '' }) => (
    <svg viewBox="0 0 24 24" fill="currentColor" className={className} aria-hidden="true">
        <path d={path} />
    </svg>
);

const icons = {
    copy: 'M16 1H4a2 2 0 00-2 2v14h2V3h12V1zm3 4H8a2 2 0 00-2 2v14a2 2 0 002 2h11a2 2 0 002-2V7a2 2 0 00-2-2zm0 16H8V7h11v14z',
    plus: 'M19 11H13V5h-2v6H5v2h6v6h2v-6h6v-2z',
    check: 'M9 16.2l-3.5-3.5-1.4 1.4L9 19 20.3 7.7l-1.4-1.4z',
    external: 'M14 3h7v7h-2V6.4l-9.3 9.3-1.4-1.4 9.3-9.3H14V3zM5 5h6v2H7v10h10v-4h2v6H5V5z',
    close: 'M6.225 4.811a1 1 0 011.414 0L12 9.172l4.361-4.361a1 1 0 111.414 1.414L13.414 10.586l4.361 4.361a1 1 0 01-1.414 1.414L12 12l-4.361 4.361a1 1 0 01-1.414-1.414l4.361-4.361-4.361-4.361a1 1 0 010-1.414z',
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

function dexLabel(dex) {
    const v = String(dex || '').trim().toLowerCase();
    if (!v) return 'DEX';
    if (v.includes('pancake') || v === 'pcs') return 'PCS';
    if (v.includes('uniswap')) return 'UNI';
    return v.toUpperCase().slice(0, 6);
}

function poolLink(poolAddress) {
    const addr = String(poolAddress || '').trim();
    if (/^0x[a-fA-F0-9]{40}$/.test(addr)) return `https://bscscan.com/address/${addr}`;
    return 'https://poolm.xyz/';
}

export default function HotPoolCard({ pool, metric, watched, onToggleWatch, onHide }) {
    const [copied, setCopied] = useState(false);
    const addr = String(pool?.pool_address || '').trim();

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

    const openPool = () => openLink(poolLink(addr));

    return (
        <div className="rounded-2xl border border-zinc-200 bg-white p-4 shadow-sm dark:border-white/10 dark:bg-[#111318] dark:shadow-none">
            <div className="flex items-start justify-between gap-3">
                <div className="min-w-0">
                    <div className="flex items-center gap-2">
                        <div className="truncate text-sm font-semibold text-zinc-900 dark:text-white/90">
                            {pool?.trading_pair || '--'}
                        </div>
                        {pool?.fee_percentage ? (
                            <div className="rounded-lg bg-sky-500/10 px-2 py-0.5 text-[11px] font-semibold text-sky-700 ring-1 ring-sky-500/20 dark:bg-sky-500/15 dark:text-sky-200 dark:ring-sky-500/30">
                                {formatFeePercent(pool.fee_percentage)}
                            </div>
                        ) : null}
                        <button
                            type="button"
                            onClick={copyAddr}
                            className={`inline-flex h-7 w-7 items-center justify-center rounded-xl border text-zinc-600 shadow-sm transition ${
                                copied
                                    ? 'border-emerald-500/30 bg-emerald-500/10 text-emerald-700 dark:border-emerald-500/30 dark:bg-emerald-500/15 dark:text-emerald-200'
                                    : 'border-zinc-200 bg-zinc-100 hover:bg-zinc-200 active:bg-zinc-200 dark:border-white/10 dark:bg-white/5 dark:text-white/70 dark:hover:bg-white/10 dark:active:bg-white/15'
                            }`}
                            aria-label={copied ? '已复制' : '复制地址'}
                            disabled={!addr}
                        >
                            <Icon path={icons.copy} className="h-4 w-4" />
                        </button>
                    </div>

                    <div className="mt-2 flex flex-wrap items-center gap-x-4 gap-y-1 text-xs">
                        <div className="text-zinc-500 dark:text-white/40">
                            交易量:{' '}
                            <span className="font-semibold text-sky-600 dark:text-sky-200 tabular-nums">
                                {formatUsdCompact(pool?.total_volume)}
                            </span>
                        </div>
                        <div className="text-zinc-500 dark:text-white/40">
                            TVL:{' '}
                            <span className="font-semibold text-zinc-900 dark:text-white/80 tabular-nums">
                                {formatUsdCompact(pool?.current_pool_value)}
                            </span>
                        </div>
                    </div>
                </div>

                <div className="text-right">
                    <div className="flex items-baseline justify-end gap-2">
                        <div className="text-lg font-extrabold text-emerald-700 dark:text-emerald-300 tabular-nums">
                            {mainValue}
                        </div>
                        {priceDisplay ? (
                            <div className={`text-xs font-semibold tabular-nums ${priceDisplayClass}`}>{priceDisplay}</div>
                        ) : null}
                    </div>
                    <div className="mt-0.5 text-[11px] font-semibold text-violet-600 dark:text-violet-300 tabular-nums">
                        {metric === 'fee_rate' ? formatUsd(pool?.total_fees) : formatRatePct(pool?.fee_rate)}
                    </div>
                </div>
            </div>

            <div className="mt-3 flex items-center justify-between">
                <div className="inline-flex items-center rounded-lg bg-amber-500/15 px-2 py-0.5 text-[11px] font-semibold text-amber-800 ring-1 ring-amber-500/25 dark:bg-amber-500/15 dark:text-amber-200 dark:ring-amber-500/30">
                    {dexLabel(pool?.dex)}
                </div>

                <div className="flex items-center gap-2">
                    <button
                        type="button"
                        onClick={() => onHide?.(addr)}
                        className="inline-flex h-9 w-9 items-center justify-center rounded-xl border border-zinc-200 bg-white/70 text-zinc-600 shadow-sm hover:bg-zinc-100 active:bg-zinc-100 dark:border-white/10 dark:bg-white/5 dark:text-white/70 dark:hover:bg-white/10 dark:active:bg-white/15"
                        aria-label="隐藏"
                        disabled={!addr}
                    >
                        <Icon path={icons.close} className="h-5 w-5" />
                    </button>
                    <button
                        type="button"
                        onClick={() => onToggleWatch?.(addr)}
                        className={`inline-flex h-9 w-9 items-center justify-center rounded-xl border shadow-sm ${
                            watched
                                ? 'border-emerald-500/30 bg-emerald-500 text-white hover:bg-emerald-600 active:bg-emerald-700'
                                : 'border-zinc-200 bg-white/70 text-zinc-700 hover:bg-zinc-100 active:bg-zinc-100 dark:border-white/10 dark:bg-white/5 dark:text-white/70 dark:hover:bg-white/10 dark:active:bg-white/15'
                        }`}
                        aria-label={watched ? '取消监控' : '添加监控'}
                        disabled={!addr}
                    >
                        <Icon path={watched ? icons.check : icons.plus} className="h-5 w-5" />
                    </button>
                    <button
                        type="button"
                        onClick={openPool}
                        className="inline-flex h-9 w-9 items-center justify-center rounded-xl border border-zinc-200 bg-white/70 text-zinc-700 shadow-sm hover:bg-zinc-100 active:bg-zinc-100 dark:border-white/10 dark:bg-white/5 dark:text-white/70 dark:hover:bg-white/10 dark:active:bg-white/15"
                        aria-label="打开"
                        disabled={!addr}
                    >
                        <Icon path={icons.external} className="h-5 w-5" />
                    </button>
                </div>
            </div>
        </div>
    );
}
