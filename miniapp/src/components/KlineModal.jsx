import React, { useMemo } from 'react';

const Icon = ({ path, className = '' }) => (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" className={className} aria-hidden="true">
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
    const title = useMemo(() => {
        const pair = String(pool?.trading_pair || '').trim() || 'K线图';
        if (pool?.fee_percentage !== undefined && pool?.fee_percentage !== null) {
            return `${pair} (${Number(pool.fee_percentage).toFixed(2)}%)`;
        }
        return pair;
    }, [pool?.trading_pair, pool?.fee_percentage]);

    // Chain slug for DexScreener (uses 'bsc')
    const dexScreenerChain = useMemo(() => {
        const c = String(chain || 'bsc').toLowerCase();
        if (c === 'bnb') return 'bsc';
        return c;
    }, [chain]);

    // Chain slug for DEXTools (uses 'bnb')
    const dextoolsChain = useMemo(() => {
        const c = String(chain || 'bsc').toLowerCase();
        if (c === 'bsc') return 'bnb';
        return c;
    }, [chain]);

    // Check if poolAddress is likely a V4 pool ID (32 bytes / 64 hex chars + 0x = 66 chars)
    const isV4ID = poolAddress && poolAddress.length > 50;

    // Build embed URL based on pool type
    // V4 pools: Use DEXTools (supports Pool ID directly)
    // V2/V3 pools: Use DexScreener (supports pool address)
    const embedUrl = useMemo(() => {
        if (!poolAddress) return '';

        if (isV4ID) {
            // DEXTools widget URL format:
            // https://www.dextools.io/widget-chart/en/{chain}/pe-light/{address}?theme={theme}&chartType=1&chartResolution=30&drawingToolbars=false
            return `https://www.dextools.io/widget-chart/en/${dextoolsChain}/pe-light/${poolAddress}?theme=${theme === 'light' ? 'light' : 'dark'}&chartType=1&chartResolution=30&drawingToolbars=false`;
        } else {
            // DexScreener embed URL format:
            // https://dexscreener.com/{chain}/{address}?embed=1&theme={theme}
            return `https://dexscreener.com/${dexScreenerChain}/${poolAddress}?embed=1&theme=${theme === 'light' ? 'light' : 'dark'}&items=0&info=0`;
        }
    }, [poolAddress, isV4ID, dextoolsChain, dexScreenerChain, theme]);

    if (!open) return null;

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
                        <div className="flex items-center gap-2 mt-0.5">
                            <div className="truncate text-[10px] font-medium text-zinc-500 dark:text-white/40 font-mono">
                                {poolAddress ? `${poolAddress.slice(0, 10)}...${poolAddress.slice(-8)}` : ''}
                            </div>
                            {isV4ID && (
                                <span className="shrink-0 text-[9px] font-semibold px-1.5 py-0.5 rounded bg-purple-100 text-purple-700 dark:bg-purple-900/50 dark:text-purple-300">
                                    V4 · DEXTools
                                </span>
                            )}
                        </div>
                    </div>
                    <div className="flex items-center gap-2">
                        <button
                            type="button"
                            onClick={onClose}
                            className="inline-flex h-8 w-8 items-center justify-center rounded-lg bg-zinc-100 text-zinc-600 transition hover:bg-zinc-200 active:bg-zinc-300 dark:bg-zinc-800 dark:text-white dark:hover:bg-zinc-700 dark:active:bg-zinc-600"
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
                            title={isV4ID ? "DEXTools Chart" : "DexScreener Chart"}
                            allowFullScreen
                        />
                    ) : (
                        <div className="flex flex-col items-center justify-center h-full text-zinc-500 dark:text-white/40 text-sm gap-2">
                            <span>无效的合约地址</span>
                        </div>
                    )}
                </div>
            </div>
        </div>
    );
}
