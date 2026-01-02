import React, { useMemo, useState, useRef, useEffect } from 'react';
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

// 通用变化指示器组件 - 用于显示数值变化（交易量、费用等）
const ChangeIndicator = ({ currentValue, previousValue, label = '变化' }) => {
    if (previousValue === undefined || previousValue === null) return null;

    const diff = currentValue - previousValue;
    if (diff === 0) return null;

    const isIncrease = diff > 0;
    const absValue = Math.abs(diff);

    // 格式化数字显示
    const formatValue = (val) => {
        if (val >= 1000) {
            return new Intl.NumberFormat('en-US', { notation: 'compact', maximumFractionDigits: 1 }).format(val);
        }
        return val.toFixed(2);
    };

    return (
        <span
            className={`ml-1 inline-flex items-center text-[9px] font-mono font-bold ${isIncrease ? 'text-neon-green' : 'text-red-500'
                }`}
            title={`${label}: ${isIncrease ? '+' : '-'}$${absValue.toFixed(2)}`}
        >
            {isIncrease ? (
                <svg className="w-2.5 h-2.5" fill="currentColor" viewBox="0 0 20 20">
                    <path fillRule="evenodd" d="M5.293 9.707a1 1 0 010-1.414l4-4a1 1 0 011.414 0l4 4a1 1 0 01-1.414 1.414L11 7.414V15a1 1 0 11-2 0V7.414L6.707 9.707a1 1 0 01-1.414 0z" clipRule="evenodd" />
                </svg>
            ) : (
                <svg className="w-2.5 h-2.5" fill="currentColor" viewBox="0 0 20 20">
                    <path fillRule="evenodd" d="M14.707 10.293a1 1 0 010 1.414l-4 4a1 1 0 01-1.414 0l-4-4a1 1 0 111.414-1.414L9 12.586V5a1 1 0 012 0v7.586l2.293-2.293a1 1 0 011.414 0z" clipRule="evenodd" />
                </svg>
            )}
            <span>{formatValue(absValue)}</span>
        </span>
    );
};

const PoolGrid = ({ refetchIntervalMs = 30000 }) => {
    const [interval, setInterval] = useState('h24');
    // 保存上一次的数据 { poolAddress_interval: { volume, tvl } }
    const previousDataRef = useRef({});

    // Use the same API but render differently
    const { data, isLoading, error } = useQuery({
        queryKey: ['pools'],
        queryFn: fetchPools,
        refetchInterval: Math.max(5000, Number(refetchIntervalMs) || 30000),
    });

    const pools = data?.data || [];

    // 当数据更新时，保存当前数据作为下次比较的基准
    useEffect(() => {
        if (pools.length > 0) {
            const currentData = {};
            pools.forEach(pool => {
                const volumeKey = `volume_${interval}`;
                const key = `${pool.address}_${interval}`;
                currentData[key] = {
                    volume: Number(pool[volumeKey] || 0),
                    tvl: Number(pool.reserve_usd || 0)
                };
            });
            // 延迟更新，确保当前渲染能够使用旧数据进行比较
            const timer = setTimeout(() => {
                previousDataRef.current = currentData;
            }, 100);
            return () => clearTimeout(timer);
        }
    }, [pools, interval]);

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
                                            <span className="text-[10px] font-bold text-slate-500 uppercase tracking-wider">{formatDexLabel(pool.factory_name)}</span>
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
                                        <div className="text-xs font-mono font-bold text-slate-300 flex items-center">
                                            ${new Intl.NumberFormat('en-US', { notation: 'compact' }).format(volume)}
                                            <ChangeIndicator
                                                currentValue={volume}
                                                previousValue={previousDataRef.current[`${pool.address}_${interval}`]?.volume}
                                                label="交易量变化"
                                            />
                                        </div>
                                    </div>
                                    <div className="text-center">
                                        <div className="text-[9px] text-slate-500 uppercase tracking-wider mb-0.5">TVL</div>
                                        <div className="text-xs font-mono font-bold text-slate-300 flex items-center justify-center">
                                            ${new Intl.NumberFormat('en-US', { notation: 'compact' }).format(Number(pool.reserve_usd || 0))}
                                            <ChangeIndicator
                                                currentValue={Number(pool.reserve_usd || 0)}
                                                previousValue={previousDataRef.current[`${pool.address}_${interval}`]?.tvl}
                                                label="TVL变化"
                                            />
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
