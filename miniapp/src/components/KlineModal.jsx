import React, { useMemo } from 'react';

const Icon = ({ path, className = '' }) => (
    <svg viewBox="0 0 24 24" fill="currentColor" className={className} aria-hidden="true">
        <path d={path} />
    </svg>
);

const icons = {
    close: 'M6 18L18 6M6 6l12 12',
};

function normalizeHexPrefixed(v) {
    const raw = String(v || '').trim();
    if (!raw) return '';
    if (raw.startsWith('0x') || raw.startsWith('0X')) return `0x${raw.slice(2)}`;
    return `0x${raw}`;
}

export default function KlineModal({ open, onClose, theme, pool, chain }) {
    const poolAddressRaw = useMemo(() => String(pool?.pool_address || pool?.pool_id || '').trim(), [pool?.pool_address, pool?.pool_id]);
    const poolAddress = useMemo(() => normalizeHexPrefixed(poolAddressRaw), [poolAddressRaw]);
    const title = useMemo(() => String(pool?.trading_pair || '').trim() || 'K线图', [pool?.trading_pair]);
    const effectiveChain = useMemo(() => {
        const c = String(chain || 'bsc').toLowerCase();
        // DexScreener chain slugs mapping if needed, simplified for common ones
        if (c === 'bnb') return 'bsc';
        return c;
    }, [chain]);

    if (!open) return null;

    // Construct DexScreener Embed URL
    // Format: https://dexscreener.com/{chain}/{address}?embed=1&theme={theme}
    // Note: DexScreener uses specific chain slugs (e.g. 'bsc', 'ethereum', 'solana', 'base')
    const embedUrl = `https://dexscreener.com/${effectiveChain}/${poolAddress}?embed=1&theme=${theme === 'light' ? 'light' : 'dark'}&items=0&info=0`;

    return (
        <div className="fixed inset-0 z-50 flex items-end sm:items-center justify-center sm:p-4">
            {/* Backdrop */}
            <button
                type="button"
                className="absolute inset-0 bg-black/60 backdrop-blur-sm transition-opacity"
                onClick={onClose}
                aria-label="关闭"
            />

            {/* Modal Content */}
            <div className="relative w-full max-w-lg overflow-hidden rounded-t-2xl sm:rounded-2xl border border-zinc-200 bg-white shadow-2xl dark:border-white/10 dark:bg-[#111318] flex flex-col h-[85vh] sm:h-[600px]">
                {/* Header */}
                <div className="flex items-center justify-between gap-3 px-4 py-3 border-b border-zinc-100 dark:border-white/5 bg-white/50 dark:bg-white/5 shrink-0">
                    <div className="min-w-0">
                        <div className="truncate text-sm font-bold text-zinc-900 dark:text-white/90">{title}</div>
                        <div className="truncate text-[10px] font-medium text-zinc-500 dark:text-white/40 font-mono mt-0.5">
                            {poolAddress ? `${poolAddress.slice(0, 10)}...${poolAddress.slice(-8)}` : ''}
                        </div>
                    </div>
                    <div className="flex items-center gap-2">
                        <button
                            type="button"
                            onClick={onClose}
                            className="inline-flex h-8 w-8 items-center justify-center rounded-lg bg-zinc-100 text-zinc-600 transition hover:bg-zinc-200 active:bg-zinc-300 dark:bg-white/10 dark:text-white/70 dark:hover:bg-white/20 dark:active:bg-white/25"
                            aria-label="关闭"
                        >
                            <Icon path={icons.close} className="h-5 w-5" />
                        </button>
                    </div>
                </div>

                {/* Iframe Container */}
                <div className="flex-1 w-full bg-[#111318] relative">
                    {poolAddress ? (
                        <iframe
                            src={embedUrl}
                            className="absolute inset-0 w-full h-full border-0"
                            title="DexScreener Chart"
                            allowFullScreen
                        />
                    ) : (
                        <div className="flex items-center justify-center h-full text-zinc-500 dark:text-white/40 text-sm">
                            无效的合约地址
                        </div>
                    )}
                </div>
            </div>
        </div>
    );
}
