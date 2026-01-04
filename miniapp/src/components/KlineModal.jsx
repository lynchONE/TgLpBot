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
    const title = useMemo(() => {
        const pair = String(pool?.trading_pair || '').trim() || 'K线图';
        if (pool?.fee_percentage !== undefined && pool?.fee_percentage !== null) {
            return `${pair} (${Number(pool.fee_percentage).toFixed(2)}%)`;
        }
        return pair;
    }, [pool?.trading_pair, pool?.fee_percentage]);
    const effectiveChain = useMemo(() => {
        const c = String(chain || 'bsc').toLowerCase();
        // DexScreener chain slugs mapping if needed, simplified for common ones
        if (c === 'bnb') return 'bsc';
        return c;
    }, [chain]);

    if (!open) return null;

    // Construct Embed URL
    // For V4 pools, the 'poolAddress' (PoolId) does NOT match DexScreener's 'pairAddress'.
    // DexScreener generates a synthetic pair address for V4 pools.
    // Solution: We query DexScreener API by Token Address to find the correct Pair Address.

    const [resolvedAddress, setResolvedAddress] = useState(null);
    const [resolving, setResolving] = useState(false);

    // Check if poolAddress is likely a pool ID (32 bytes / 64 hex chars + 0x = 66 chars)
    const isV4ID = poolAddress && poolAddress.length > 50;

    useEffect(() => {
        if (!open) {
            setResolvedAddress(null);
            setResolving(false);
            return;
        }

        // If it's a standard V2/V3 pool (short address), use it directly.
        if (!isV4ID) {
            setResolvedAddress(poolAddress);
            return;
        }

        // If it's a V4 pool (Long ID), we must find the DexScreener Pair Address.
        const token0 = pool?.token0_address;
        const token1 = pool?.token1_address;

        if (!token0) {
            // Fallback: If no token info, try using the ID directly (unlikely to work but better than nothing)
            setResolvedAddress(poolAddress);
            return;
        }

        setResolving(true);
        const fetchPair = async () => {
            try {
                // Fetch pairs for the first token
                const res = await fetch(`https://api.dexscreener.com/latest/dex/tokens/${token0}`);
                const data = await res.json();

                if (data?.pairs && Array.isArray(data.pairs)) {
                    // Filter for:
                    // 1. Same Chain (bsc)
                    // 2. Contains the other token (token1)
                    // 3. DEX is PancakeSwap (optional, but safe)
                    const targetPair = data.pairs.find(p =>
                        p.chainId === effectiveChain &&
                        p.quoteToken?.address?.toLowerCase() === token1.toLowerCase()
                    );

                    if (targetPair?.pairAddress) {
                        setResolvedAddress(targetPair.pairAddress);
                    } else {
                        // If exact match not found, fallback to poolAddress
                        setResolvedAddress(poolAddress);
                    }
                } else {
                    setResolvedAddress(poolAddress);
                }
            } catch (e) {
                console.error("DexScreener API error:", e);
                setResolvedAddress(poolAddress);
            } finally {
                setResolving(false);
            }
        };

        fetchPair();

    }, [open, isV4ID, poolAddress, pool?.token0_address, pool?.token1_address, effectiveChain]);

    const embedUrl = resolvedAddress
        ? `https://dexscreener.com/${effectiveChain}/${resolvedAddress}?embed=1&theme=${theme === 'light' ? 'light' : 'dark'}&items=0&info=0`
        : '';

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
                    {resolving ? (
                        <div className="flex items-center justify-center h-full text-zinc-500 dark:text-white/40 text-sm animate-pulse">
                            正在寻找 DexScreener 图表...
                        </div>
                    ) : resolvedAddress ? (
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
