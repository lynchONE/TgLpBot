import React, { useState } from 'react';
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
    const [tilt, setTilt] = useState({ x: 0, y: 0 });

    const handleMouseMove = (e) => {
        const { left, top, width, height } = e.currentTarget.getBoundingClientRect();
        const x = (e.clientX - left - width / 2) / 25;
        const y = (e.clientY - top - height / 2) / 25;
        setTilt({ x, y });
    };

    const handleMouseLeave = () => {
        setTilt({ x: 0, y: 0 });
    };

    const { data: balance } = useReadContract({
        address: NPM_ADDRESS,
        abi: NPM_ABI,
        functionName: 'balanceOf',
        args: [address],
        query: { enabled: isConnected, refetchInterval },
    });

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
            <div className="glass-panel w-full h-full min-h-[300px] flex flex-col items-center justify-center rounded-3xl p-8 border border-white/5 bg-midnight-950/50">
                <div className="relative mb-6">
                    <div className="absolute inset-0 bg-neon-purple blur-xl opacity-20 animate-pulse-slow"></div>
                    <div className="relative flex h-20 w-20 items-center justify-center rounded-2xl bg-midnight-900 border border-white/10 text-4xl shadow-2xl">
                        🔐
                    </div>
                </div>
                <h3 className="mb-2 text-2xl font-display font-bold text-white tracking-wide">Wallet Locked</h3>
                <p className="max-w-xs text-center text-slate-400 font-light">
                    Connect wallet to initialize dashboard link.
                </p>
            </div>
        );
    }

    if (!balance || balance === 0n) {
        return (
            <div className="glass-panel w-full h-full rounded-3xl p-8 text-center flex flex-col items-center justify-center border-dashed border-2 border-white/10 bg-midnight-950/30">
                <h3 className="text-xl font-bold text-white mb-2">No Active Zones</h3>
                <p className="text-sm text-slate-400 max-w-xs">
                    No active liquidity positions detected. Deploy capital to view stats.
                </p>
            </div>
        );
    }

    if (!position) {
        return (
            <div className="glass-panel w-full h-full rounded-3xl p-8 flex items-center justify-center">
                <div className="flex flex-col items-center gap-4">
                    <span className="relative flex h-3 w-3">
                        <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-neon-cyan opacity-75"></span>
                        <span className="relative inline-flex rounded-full h-3 w-3 bg-neon-cyan"></span>
                    </span>
                    <span className="text-neon-cyan font-mono text-xs tracking-[0.2em] animate-pulse">INITIALIZING DATA STREAM...</span>
                </div>
            </div>
        );
    }

    // Unpack position data
    const [, , token0, token1, fee, tickLower, tickUpper, liquidity, , , tokensOwed0, tokensOwed1] = position;

    const priceLower = Math.pow(1.0001, Number(tickLower));
    const priceUpper = Math.pow(1.0001, Number(tickUpper));

    return (
        <div
            className="relative w-full h-full perspective-1000 group cursor-default"
            onMouseMove={handleMouseMove}
            onMouseLeave={handleMouseLeave}
        >
            <div
                className="relative h-full w-full rounded-3xl bg-midnight-900/80 border border-white/10 p-6 shadow-2xl transition-transform duration-200 ease-out preserve-3d backdrop-blur-md overflow-hidden"
                style={{
                    transform: `rotateX(${-tilt.y}deg) rotateY(${tilt.x}deg)`,
                    boxShadow: `
                        ${-tilt.x * 2}px ${-tilt.y * 2}px 20px rgba(0,0,0,0.5),
                        0 0 20px rgba(168, 85, 247, 0.1)
                    `
                }}
            >
                {/* Decorative Cyber Lines */}
                <div className="absolute top-0 left-0 w-full h-1 bg-gradient-to-r from-transparent via-neon-cyan to-transparent opacity-50"></div>
                <div className="absolute bottom-0 right-0 w-full h-1 bg-gradient-to-r from-transparent via-neon-purple to-transparent opacity-50"></div>

                {/* Content Layer */}
                <div className="relative z-10 h-full flex flex-col justify-between">
                    {/* Header */}
                    <div className="flex justify-between items-start">
                        <div>
                            <div className="flex items-center gap-2 mb-2">
                                <span className="bg-neon-blue/10 border border-neon-blue/30 text-neon-blue text-[9px] uppercase tracking-[0.1em] px-2 py-1 rounded-sm font-bold shadow-[0_0_10px_rgba(59,130,246,0.2)]">
                                    V3 LINK
                                </span>
                                <span className="bg-neon-green/10 border border-neon-green/30 text-neon-green text-[9px] uppercase tracking-[0.1em] px-2 py-1 rounded-sm font-bold shadow-[0_0_10px_rgba(34,197,94,0.2)]">
                                    #{tokenId.toString()}
                                </span>
                            </div>
                            <h3 className="font-display text-3xl font-bold text-white tracking-tight drop-shadow-lg">
                                LP Node 01
                            </h3>
                            <div className="mt-1 font-mono text-xs text-slate-400 font-medium tracking-wide flex items-center gap-2">
                                <span>{token0.slice(0, 4)}</span>
                                <span className="text-white/20">/</span>
                                <span>{token1.slice(0, 4)}</span>
                                <span className="text-neon-cyan px-2">●</span>
                                <span>{(fee / 10000).toFixed(2)}% Tier</span>
                            </div>
                        </div>
                        <div className="w-12 h-12 rounded-full border border-white/10 bg-white/5 flex items-center justify-center animate-spin-slow">
                            <svg className="w-6 h-6 text-slate-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1} d="M19.428 15.428a2 2 0 00-1.022-.547l-2.384-.477a6 6 0 00-3.86.517l-.318.158a6 6 0 01-3.86.517L6.05 15.21a2 2 0 00-1.806.547M8 4h8l-1 1v5.172a2 2 0 00.586 1.414l5 5c1.26 1.26.367 3.414-1.415 3.414H4.828c-1.782 0-2.674-2.154-1.414-3.414l5-5A2 2 0 009 10.172V5L8 4z" />
                            </svg>
                        </div>
                    </div>

                    {/* Stats Grid */}
                    <div className="grid grid-cols-2 gap-4 my-6">
                        <div className="relative group/stat p-4 rounded-xl bg-midnight-950/50 border border-white/5 hover:border-neon-cyan/50 transition-colors duration-300">
                            <div className="absolute -inset-0.5 bg-gradient-to-r from-neon-cyan to-blue-600 rounded-xl blur opacity-0 group-hover/stat:opacity-20 transition duration-500"></div>
                            <div className="relative">
                                <div className="text-[10px] text-slate-500 uppercase tracking-wider font-bold mb-1">Total Liquidity</div>
                                <div className="text-xl font-display font-bold text-white tracking-wide">
                                    {liquidity.toString().slice(0, 6)}<span className="text-slate-600 text-sm">...</span>
                                </div>
                            </div>
                        </div>

                        <div className="relative group/stat p-4 rounded-xl bg-midnight-950/50 border border-white/5 hover:border-neon-green/50 transition-colors duration-300">
                            <div className="absolute -inset-0.5 bg-gradient-to-r from-neon-green to-emerald-600 rounded-xl blur opacity-0 group-hover/stat:opacity-20 transition duration-500"></div>
                            <div className="relative">
                                <div className="text-[10px] text-slate-500 uppercase tracking-wider font-bold mb-1">Unclaimed Yield</div>
                                <div className="text-xl font-display font-bold text-neon-green tracking-wide drop-shadow-[0_0_8px_rgba(34,197,94,0.5)]">
                                    {formatUnits(tokensOwed0 + tokensOwed1, 18).slice(0, 6)}
                                </div>
                            </div>
                        </div>
                    </div>

                    {/* Range Visualization */}
                    <div className="space-y-3">
                        <div className="flex justify-between items-center px-1">
                            <span className="text-[10px] font-bold text-slate-500 uppercase">Range Status</span>
                            <span className="flex items-center gap-1.5 bg-neon-green/10 px-2 py-0.5 rounded-full border border-neon-green/20">
                                <span className="relative flex h-1.5 w-1.5">
                                    <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-neon-green opacity-75"></span>
                                    <span className="relative inline-flex rounded-full h-1.5 w-1.5 bg-neon-green"></span>
                                </span>
                                <span className="text-[9px] text-neon-green font-bold uppercase tracking-wider">In Range</span>
                            </span>
                        </div>

                        {/* Custom Range Bar */}
                        <div className="h-1.5 w-full bg-slate-800 rounded-full overflow-hidden flex items-center">
                            <div className="h-full bg-slate-800 w-[10%]"></div>
                            <div className="h-full bg-gradient-to-r from-neon-cyan via-white to-neon-purple w-[80%] relative shadow-[0_0_15px_rgba(6,182,212,0.5)]">
                                <div className="absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 w-1 h-3 bg-white rounded-full shadow-[0_0_10px_white]"></div>
                            </div>
                            <div className="h-full bg-slate-800 w-[10%]"></div>
                        </div>

                        <div className="flex justify-between font-mono text-[10px] text-slate-400">
                            <span>{priceLower.toPrecision(5)}</span>
                            <span>{priceUpper.toPrecision(5)}</span>
                        </div>
                    </div>
                </div>
            </div>
        </div>
    );
};

export default PositionCard;
