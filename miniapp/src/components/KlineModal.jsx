import React, { useMemo } from 'react';
import BottomSheet from './BottomSheet.jsx';
import NumberFlowValue from './NumberFlowValue.jsx';

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
            return `${pair} (${Number(pool.fee_percentage).toFixed(4)}%)`;
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
            // https://www.dextools.io/widget-chart/en/{chain}/pe-light/{address}?theme={theme}&chartType=1&chartResolution=1&drawingToolbars=false
            // chartResolution=1 表示 1 分钟 K 线
            return `https://www.dextools.io/widget-chart/en/${dextoolsChain}/pe-light/${poolAddress}?theme=${theme === 'light' ? 'light' : 'dark'}&chartType=1&chartResolution=1&drawingToolbars=false`;
        } else {
            // DexScreener embed URL format:
            // https://dexscreener.com/{chain}/{address}?embed=1&theme={theme}&interval=1
            // interval=1 表示 1 分钟 K 线
            return `https://dexscreener.com/${dexScreenerChain}/${poolAddress}?embed=1&theme=${theme === 'light' ? 'light' : 'dark'}&items=0&info=0&interval=1`;
        }
    }, [poolAddress, isV4ID, dextoolsChain, dexScreenerChain, theme]);

    return (
        <BottomSheet
            open={open}
            onClose={onClose}
            maxHeightClass="h-[92vh] sm:h-[700px] max-h-none"
            className="overflow-hidden"
            headerClassName="px-4 py-3 border-b border-zinc-100 dark:border-white/5 bg-zinc-50/50 dark:bg-[#14171c]/50 shrink-0"
            contentClassName=""
            title={
                <div>
                    <div className="truncate text-sm font-bold text-zinc-900 dark:text-white/90">
                        <NumberFlowValue value={title} formatter={() => title} />
                    </div>
                    <div className="flex items-center gap-2 mt-0.5">
                        <div className="truncate text-[10px] font-medium text-zinc-500 dark:text-white/40 font-mono">
                            {poolAddress ? <NumberFlowValue value={`${poolAddress.slice(0, 10)}...${poolAddress.slice(-8)}`} formatter={(v) => v} /> : ''}
                        </div>
                        {isV4ID && (
                            <span className="shrink-0 text-[9px] font-semibold px-1.5 py-0.5 rounded bg-purple-100 text-purple-700 dark:bg-purple-900/50 dark:text-purple-300">
                                V4 · DEXTools
                            </span>
                        )}
                    </div>
                </div>
            }
        >
            <div className="w-full h-full bg-[#14171c] relative min-h-[500px]">
                {poolAddress ? (
                    <iframe
                        src={embedUrl}
                        className="absolute inset-0 w-full h-full border-0"
                        title={isV4ID ? "DEXTools Chart" : "DexScreener Chart"}
                        allowFullScreen
                    />
                ) : (
                    <div className="flex flex-col items-center justify-center h-full text-zinc-500 dark:text-white/40 text-sm gap-2 mt-10">
                        <span>无效的合约地址</span>
                    </div>
                )}
            </div>
        </BottomSheet>
    );
}
