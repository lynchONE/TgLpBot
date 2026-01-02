import React, { useMemo, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Tooltip } from 'antd';

const fetchPools = async () => {
    const response = await fetch('http://localhost:8080/api/pools');
    if (!response.ok) {
        throw new Error('Network response was not ok');
    }
    return response.json();
};

const formatDexLabel = (dexId) => {
    if (!dexId) return 'DEX';
    const cleaned = String(dexId).replace(/[_-]+/g, ' ').trim();
    return cleaned.replace(/\b\w/g, (c) => c.toUpperCase());
};

const PoolGrid = ({ refetchIntervalMs = 30000 }) => {
    const [interval, setInterval] = useState('h24');

    // Use the same API but render differently
    const { data, isLoading, error } = useQuery({
        queryKey: ['pools'],
        queryFn: fetchPools,
        refetchInterval: Math.max(5000, Number(refetchIntervalMs) || 30000),
    });

    const pools = data?.data || [];

    const intervalLabel = useMemo(() => {
        switch (interval) {
            case 'm5': return '5M';
            case 'h1': return '1H';
            case 'h6': return '6H';
            default: return '24H';
        }
    }, [interval]);

    const getKey = (prefix) => `${prefix}_${interval}`;

    if (error) return (
        <div className="glass-panel p-6 rounded-3xl text-center border-red-500/20">
            <div className="text-red-400 font-bold mb-2">⚠ SYSTEM MALFUNCTION</div>
            <div className="text-xs text-red-500/80 font-mono">{error.message}</div>
        </div>
    );

    return (
        <div className="w-full">
            {/* Control Bar */}
            <div className="flex justify-between items-center mb-6">
                <div className="flex items-center gap-2">
                    <span className="relative flex h-2 w-2">
                        <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-neon-cyan opacity-75"></span>
                        <span className="relative inline-flex rounded-full h-2 w-2 bg-neon-cyan"></span>
                    </span>
                    <h2 className="text-sm font-bold text-white uppercase tracking-widest">Market Feed</h2>
                </div>

                <div className="flex bg-midnight-950/50 rounded-lg p-1 border border-white/5">
                    {['m5', 'h1', 'h6', 'h24'].map((v) => (
                        <button
                            key={v}
                            onClick={() => setInterval(v)}
                            className={`px-3 py-1 rounded text-[10px] font-bold uppercase tracking-wider transition-all duration-300 ${interval === v
                                    ? 'bg-neon-blue text-white shadow-[0_0_10px_rgba(59,130,246,0.5)]'
                                    : 'text-slate-500 hover:text-slate-300'
                                }`}
                        >
                            {v.toUpperCase()}
                        </button>
                    ))}
                </div>
            </div>

            {/* Grid Layout */}
            {isLoading ? (
                <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4 animate-pulse">
                    {[1, 2, 3, 4, 5, 6].map(i => (
                        <div key={i} className="h-32 rounded-2xl bg-white/5 border border-white/5"></div>
                    ))}
                </div>
            ) : (
                <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-3 gap-4">
                    {pools.map((pool) => {
                        const volumeKey = getKey('volume');
                        const feeKey = getKey('fee_usd');
                        const aprKey = getKey('fee_apr');
                        const changeKey = getKey('price_change');

                        const volume = Number(pool[volumeKey] || 0);
                        const apr = Number(pool[aprKey] || 0);
                        const change = Number(pool[changeKey] || 0);
                        const isPos = change >= 0;

                        return (
                            <div key={pool.id} className="group relative overflow-hidden rounded-2xl bg-midnight-900/40 border border-white/5 p-5 hover:bg-midnight-900/60 hover:border-neon-purple/30 transition-all duration-300 hover:-translate-y-1 hover:shadow-lg">
                                {/* Hover Gradient Line */}
                                <div className="absolute top-0 left-0 w-1 h-full bg-gradient-to-b from-neon-purple to-transparent opacity-0 group-hover:opacity-100 transition-opacity"></div>

                                <div className="flex justify-between items-start mb-4">
                                    <div>
                                        <div className="flex items-center gap-2 mb-1">
                                            <span className="text-[10px] font-bold text-slate-500 uppercase tracking-wider">{formatDexLabel(pool.dex_id)}</span>
                                            <span className="text-[10px] font-bold text-neon-blue bg-neon-blue/10 px-1.5 py-0.5 rounded border border-neon-blue/20">
                                                {Number(pool.pool_fee_percentage).toFixed(2)}%
                                            </span>
                                        </div>
                                        <h3 className="font-display text-lg font-bold text-white group-hover:text-neon-cyan transition-colors truncate max-w-[180px]">
                                            {pool.name}
                                        </h3>
                                        <Tooltip title={pool.address}>
                                            <div className="text-[10px] font-mono text-slate-600 cursor-pointer hover:text-slate-400 truncate">
                                                {pool.address}
                                            </div>
                                        </Tooltip>
                                    </div>
                                    <div className={`flex flex-col items-end`}>
                                        <span className={`text-sm font-mono font-bold ${isPos ? 'text-neon-green' : 'text-red-500'}`}>
                                            {isPos ? '+' : ''}{change.toFixed(2)}%
                                        </span>
                                        <span className="text-[10px] text-slate-500 font-mono">${Number(pool.price_usd).toFixed(4)}</span>
                                    </div>
                                </div>

                                <div className="grid grid-cols-3 gap-2 py-3 border-t border-white/5">
                                    <div>
                                        <div className="text-[9px] text-slate-500 uppercase tracking-wider mb-0.5">Vol ({intervalLabel})</div>
                                        <div className="text-xs font-mono font-bold text-slate-300">
                                            ${new Intl.NumberFormat('en-US', { notation: 'compact' }).format(volume)}
                                        </div>
                                    </div>
                                    <div className="text-center">
                                        <div className="text-[9px] text-slate-500 uppercase tracking-wider mb-0.5">TVL</div>
                                        <div className="text-xs font-mono font-bold text-slate-300">
                                            ${new Intl.NumberFormat('en-US', { notation: 'compact' }).format(Number(pool.reserve_usd || 0))}
                                        </div>
                                    </div>
                                    <div className="text-right">
                                        <div className="text-[9px] text-slate-500 uppercase tracking-wider mb-0.5">APR</div>
                                        <div className="text-xs font-mono font-bold text-neon-purple drop-shadow-[0_0_5px_rgba(168,85,247,0.5)]">
                                            {apr.toFixed(1)}%
                                        </div>
                                    </div>
                                </div>

                                {/* Background Decoration */}
                                <div className="absolute -bottom-4 -right-4 w-24 h-24 bg-neon-purple/5 blur-2xl rounded-full group-hover:bg-neon-purple/10 transition-colors"></div>
                            </div>
                        );
                    })}
                </div>
            )}
        </div>
    );
};

export default PoolGrid;
