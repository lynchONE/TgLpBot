import React from 'react';
import { useAccount, useReadContract } from 'wagmi';
import { formatUnits } from 'viem';

// Minimal ABI for Uniswap V3 NonfungiblePositionManager
const NPM_ABI = [
    {
        inputs: [{ name: 'owner', type: 'address' }],
        name: 'balanceOf',
        outputs: [{ name: '', type: 'uint256' }],
        stateMutability: 'view',
        type: 'function',
    },
    {
        inputs: [
            { name: 'owner', type: 'address' },
            { name: 'index', type: 'uint256' },
        ],
        name: 'tokenOfOwnerByIndex',
        outputs: [{ name: '', type: 'uint256' }],
        stateMutability: 'view',
        type: 'function',
    },
    {
        inputs: [{ name: 'tokenId', type: 'uint256' }],
        name: 'positions',
        outputs: [
            { name: 'nonce', type: 'uint96' },
            { name: 'operator', type: 'address' },
            { name: 'token0', type: 'address' },
            { name: 'token1', type: 'address' },
            { name: 'fee', type: 'uint24' },
            { name: 'tickLower', type: 'int24' },
            { name: 'tickUpper', type: 'int24' },
            { name: 'liquidity', type: 'uint128' },
            { name: 'feeGrowthInside0LastX128', type: 'uint256' },
            { name: 'feeGrowthInside1LastX128', type: 'uint256' },
            { name: 'tokensOwed0', type: 'uint128' },
            { name: 'tokensOwed1', type: 'uint128' },
        ],
        stateMutability: 'view',
        type: 'function',
    },
];

const NPM_ADDRESS = '0x7b8A01B39D58278b5DE7e48c8449c9f4F5170613'; // BSC V3 NPM (PancakeSwap V3 usually)

const PositionCard = ({ refetchIntervalMs = 30000 }) => {
    const { address, isConnected } = useAccount();
    const refetchInterval = Math.max(5000, Number(refetchIntervalMs) || 30000);

    const { data: balance } = useReadContract({
        address: NPM_ADDRESS,
        abi: NPM_ABI,
        functionName: 'balanceOf',
        args: [address],
        query: { enabled: isConnected, refetchInterval },
    });

    // Fetch first token ID if balance > 0
    const { data: tokenId } = useReadContract({
        address: NPM_ADDRESS,
        abi: NPM_ABI,
        functionName: 'tokenOfOwnerByIndex',
        args: [address, 0n],
        query: { enabled: !!balance && balance > 0n, refetchInterval },
    });

    const { data: position } = useReadContract({
        address: NPM_ADDRESS,
        abi: NPM_ABI,
        functionName: 'positions',
        args: [tokenId],
        query: { enabled: !!tokenId, refetchInterval },
    });

    if (!isConnected) {
        return (
            <div className="glass-card flex min-h-[300px] flex-col items-center justify-center rounded-3xl p-8">
                <div className="mb-4 flex h-16 w-16 items-center justify-center rounded-2xl bg-secondary/50 text-3xl shadow-inner">
                    👛
                </div>
                <h3 className="mb-2 text-xl font-bold text-foreground">Wallet Disconnected</h3>
                <p className="max-w-xs text-center text-muted-foreground">
                    Connect your wallet to view your active liquidity positions.
                </p>
            </div>
        );
    }

    if (!balance || balance === 0n) {
        return (
            <div className="glass-card rounded-3xl p-8 text-center">
                <h3 className="text-lg font-bold text-foreground">No Positions Found</h3>
                <p className="mt-2 text-sm text-muted-foreground">
                    You don't have any active V3 liquidity positions on this chain.
                </p>
            </div>
        );
    }

    if (!position) {
        return (
            <div className="glass-card rounded-3xl p-8 flex items-center justify-center">
                <div className="animate-pulse text-muted-foreground">Loading position data...</div>
            </div>
        );
    }

    // Unpack position data
    const [, , token0, token1, fee, tickLower, tickUpper, liquidity, , , tokensOwed0, tokensOwed1] = position;

    // Calculate basic tick range prices (simplified math)
    const priceLower = Math.pow(1.0001, Number(tickLower));
    const priceUpper = Math.pow(1.0001, Number(tickUpper));

    return (
        <div className="relative overflow-hidden rounded-3xl border border-white/10 bg-gradient-to-br from-[#1e293b] to-[#0f172a] p-6 text-white shadow-xl ring-1 ring-white/5 group hover:shadow-2xl hover:-translate-y-1 transition-all duration-300">
            {/* Background Decor */}
            <div className="absolute top-0 right-0 p-4 opacity-5 group-hover:opacity-10 transition-opacity duration-500">
                <svg width="180" height="180" viewBox="0 0 24 24" fill="currentColor">
                    <path d="M12 2L2 7l10 5 10-5-10-5zm0 9l2.5-1.25L12 8.5l-2.5 1.25L12 11zm0 2.5l-5-2.5-5 2.5L12 22l10-8.5-5-2.5-5 2.5z" />
                </svg>
            </div>

            <div className="absolute top-0 right-0 w-full h-full bg-gradient-to-br from-blue-500/10 to-purple-500/10 rounded-3xl pointer-events-none"></div>

            <div className="relative z-10">
                <div className="flex justify-between items-start mb-8">
                    <div>
                        <div className="flex items-center gap-2 mb-2">
                            <span className="bg-white/10 border border-white/10 text-white/90 text-[10px] uppercase tracking-wider px-2 py-0.5 rounded-full font-bold backdrop-blur-md">
                                V3 Position
                            </span>
                            <span className="bg-green-500/20 text-green-300 border border-green-500/20 text-[10px] uppercase tracking-wider px-2 py-0.5 rounded-full font-bold backdrop-blur-md">
                                #{tokenId.toString()}
                            </span>
                        </div>
                        <h3 className="font-display text-2xl font-bold text-white tracking-tight">
                            Liquidity Pool
                        </h3>
                        <div className="mt-1 font-mono text-xs text-slate-400 font-medium tracking-wide">
                            {token0.slice(0, 6)}... / {token1.slice(0, 6)}...
                            <span className="mx-2 text-slate-600">|</span>
                            {fee / 10000}% Fee
                        </div>
                    </div>
                </div>

                <div className="grid grid-cols-2 gap-4 mb-6">
                    <div className="rounded-2xl bg-white/5 border border-white/5 p-4 backdrop-blur-sm hover:bg-white/10 transition-colors">
                        <div className="mb-1 text-xs text-slate-400 uppercase tracking-wider font-semibold">Liquidity</div>
                        <div className="truncate text-lg font-bold font-display text-white">
                            {liquidity.toString().slice(0, 8)}...
                        </div>
                    </div>
                    <div className="rounded-2xl bg-white/5 border border-white/5 p-4 backdrop-blur-sm hover:bg-white/10 transition-colors">
                        <div className="mb-1 text-xs text-slate-400 uppercase tracking-wider font-semibold">Unclaimed Fees</div>
                        <div className="text-lg font-bold font-display text-emerald-400">
                            {formatUnits(tokensOwed0 + tokensOwed1, 18).slice(0, 6)}
                        </div>
                    </div>
                </div>

                <div className="rounded-2xl bg-black/20 border border-white/5 p-5 backdrop-blur-md">
                    <div className="flex justify-between items-center mb-4">
                        <span className="text-xs font-semibold uppercase tracking-wider text-slate-400">Price Range</span>
                        <div className="flex items-center gap-1.5">
                            <span className="text-[10px] text-green-400 font-bold uppercase">Active</span>
                            <span className="relative flex h-2 w-2">
                                <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-green-400 opacity-75"></span>
                                <span className="relative inline-flex rounded-full h-2 w-2 bg-green-500"></span>
                            </span>
                        </div>
                    </div>

                    <div className="flex justify-between text-center divide-x divide-white/10">
                        <div className="flex-1 px-2">
                            <div className="mb-1 text-[10px] text-slate-400">Min Price</div>
                            <div className="font-mono text-sm font-bold text-white">{priceLower.toPrecision(5)}</div>
                        </div>
                        <div className="flex-1 px-2">
                            <div className="mb-1 text-[10px] text-slate-400">Max Price</div>
                            <div className="font-mono text-sm font-bold text-white">{priceUpper.toPrecision(5)}</div>
                        </div>
                    </div>
                </div>
            </div>
        </div>
    );
};

export default PositionCard;
