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
            <div className="flex min-h-[300px] flex-col items-center justify-center rounded-2xl border border-slate-200/70 bg-white/80 p-8 shadow-sm ring-1 ring-black/5 backdrop-blur dark:border-slate-800/70 dark:bg-[#151718]/70 dark:ring-white/10">
                <div className="mb-4 flex h-16 w-16 items-center justify-center rounded-full bg-slate-100 text-3xl dark:bg-white/5">
                    👛
                </div>
                <h3 className="mb-2 text-xl font-bold text-slate-900 dark:text-white">Wallet Disconnected</h3>
                <p className="max-w-xs text-center text-slate-500 dark:text-slate-400">
                    Connect your wallet to view your active liquidity positions.
                </p>
            </div>
        );
    }

    if (!balance || balance === 0n) {
        return (
            <div className="rounded-2xl border border-slate-200/70 bg-white/80 p-8 text-center shadow-sm ring-1 ring-black/5 backdrop-blur dark:border-slate-800/70 dark:bg-[#151718]/70 dark:ring-white/10">
                <h3 className="text-lg font-bold text-slate-900 dark:text-white">No Positions Found</h3>
                <p className="mt-2 text-sm text-slate-500 dark:text-slate-400">
                    You don&apos;t have any active V3 liquidity positions on this chain.
                </p>
            </div>
        );
    }

    if (!position) {
        return <div className="p-8 text-center text-gray-500">Loading position...</div>;
    }

    // Unpack position data (Simplified for UI)
    // In a real app, you'd fetch Token Symbols and Decimals here using another hook or multicall.
    // For now we mock the symbols but use real raw numbers to prove data connection.
    const [, , token0, token1, fee, tickLower, tickUpper, liquidity, , , tokensOwed0, tokensOwed1] = position;

    // Calculate basic tick range prices (simplified math)
    const priceLower = Math.pow(1.0001, Number(tickLower));
    const priceUpper = Math.pow(1.0001, Number(tickUpper));

    return (
        <div className="relative overflow-hidden rounded-2xl border border-slate-200/70 bg-white/80 p-6 shadow-sm ring-1 ring-black/5 backdrop-blur dark:border-slate-800/70 dark:bg-[#151718]/70 dark:ring-white/10 group">
            <div className="absolute top-0 right-0 p-4 opacity-10 group-hover:opacity-20 transition-opacity">
                <svg width="120" height="120" viewBox="0 0 24 24" fill="currentColor" className="text-blue-500">
                    <path d="M12 2L2 7l10 5 10-5-10-5zm0 9l2.5-1.25L12 8.5l-2.5 1.25L12 11zm0 2.5l-5-2.5-5 2.5L12 22l10-8.5-5-2.5-5 2.5z" />
                </svg>
            </div>

            <div className="relative z-10">
                <div className="flex justify-between items-start mb-6">
                    <div>
                        <div className="flex items-center gap-2 mb-1">
                            <span className="bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300 text-xs px-2 py-1 rounded font-bold">
                                V3 LP #{tokenId.toString()}
                            </span>
                            <span className="bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-300 text-xs px-2 py-1 rounded font-bold">
                                {fee / 10000}% Fee
                            </span>
                        </div>
                        <h3 className="text-2xl font-bold text-gray-900 dark:text-white">
                            Active Position
                        </h3>
                        <div className="mt-1 font-mono text-xs text-slate-400">
                            {token0.slice(0, 6)}... / {token1.slice(0, 6)}...
                        </div>
                    </div>
                </div>

                <div className="grid grid-cols-2 gap-4 mb-6">
                    <div className="rounded-xl border border-slate-200/70 bg-slate-50/80 p-3 dark:border-slate-800/70 dark:bg-white/5">
                        <div className="mb-1 text-xs text-slate-500 dark:text-slate-400">Liquidity</div>
                        <div className="truncate text-lg font-bold text-slate-900 dark:text-white">
                            {liquidity.toString().slice(0, 8)}...
                        </div>
                    </div>
                    <div className="rounded-xl border border-slate-200/70 bg-slate-50/80 p-3 dark:border-slate-800/70 dark:bg-white/5">
                        <div className="mb-1 text-xs text-slate-500 dark:text-slate-400">Unclaimed Fees</div>
                        <div className="text-lg font-bold text-green-600 dark:text-green-400">
                            {formatUnits(tokensOwed0 + tokensOwed1, 18).slice(0, 6)}
                        </div>
                    </div>
                </div>

                <div className="rounded-xl border border-slate-200/70 bg-slate-50/80 p-4 dark:border-slate-800/70 dark:bg-white/5">
                    <div className="flex justify-between items-center mb-4">
                        <span className="text-sm font-medium text-slate-600 dark:text-slate-300">Price Range</span>
                        <div className="w-2 h-2 rounded-full bg-green-500 animate-pulse"></div>
                    </div>

                    <div className="flex justify-between text-center divide-x divide-slate-200/70 dark:divide-slate-700/60">
                        <div className="flex-1 px-2">
                            <div className="mb-1 text-xs text-slate-500 dark:text-slate-400">Min Price</div>
                            <div className="font-mono text-sm font-bold text-slate-900 dark:text-white">{priceLower.toPrecision(5)}</div>
                        </div>
                        <div className="flex-1 px-2">
                            <div className="mb-1 text-xs text-slate-500 dark:text-slate-400">Max Price</div>
                            <div className="font-mono text-sm font-bold text-slate-900 dark:text-white">{priceUpper.toPrecision(5)}</div>
                        </div>
                    </div>
                </div>
            </div>
        </div>
    );
};

export default PositionCard;
