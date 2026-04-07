import React, { useCallback, useEffect, useState, useRef } from 'react';
import { fetchWallets, walletSwapSingleQuote, walletSwapSingleExecute } from '../api';
import PanelShell from './PanelShell';
import { ArrowDown, Settings, RefreshCw } from 'lucide-react';
import { shortAddress } from '../utils';

const STABLE_ADDRESSES = {
    'bsc': '0x55d398326f99059fF775485246999027B3197955', // USDT
    'base': '0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913' // USDC
};

export default function SwapPanel({ apiBaseUrl, initData, hasInitData, chain = 'bsc' }) {
    const [wallets, setWallets] = useState([]);
    const [selectedWalletId, setSelectedWalletId] = useState('');
    const [walletLoading, setWalletLoading] = useState(false);

    const [fromToken, setFromToken] = useState('');
    const [toToken, setToToken] = useState(STABLE_ADDRESSES['bsc']);
    const [amount, setAmount] = useState('');
    const [slippage, setSlippage] = useState('1.0');
    const [showSettings, setShowSettings] = useState(false);

    const [quoteInfo, setQuoteInfo] = useState(null);
    const [quoting, setQuoting] = useState(false);
    const [quoteError, setQuoteError] = useState('');
    
    const [executing, setExecuting] = useState(false);
    const [execError, setExecError] = useState('');
    const [execSuccess, setExecSuccess] = useState('');
    const [showConfirm, setShowConfirm] = useState(false);

    const quoteTimeout = useRef(null);

    // Update stable token default on chain change
    useEffect(() => {
        if (chain && STABLE_ADDRESSES[chain]) {
            setToToken(STABLE_ADDRESSES[chain]);
        }
    }, [chain]);

    const loadWallets = useCallback(async () => {
        if (!initData) return;
        setWalletLoading(true);
        try {
            const resp = await fetchWallets({ apiBaseUrl, initData, chain });
            const list = resp?.wallets || [];
            setWallets(list);
            if (list.length > 0 && !selectedWalletId) {
                const def = list.find(w => w.is_default) || list[0];
                setSelectedWalletId(def.id);
            }
        } catch (e) {
            console.error("fetchWallets failed", e);
        } finally {
            setWalletLoading(false);
        }
    }, [apiBaseUrl, initData, chain, selectedWalletId]);

    useEffect(() => {
        if (hasInitData) {
            loadWallets();
            setExecError('');
            setExecSuccess('');
        }
    }, [loadWallets, hasInitData]);

    const doQuote = useCallback(async (amt, fToken, tToken, chainId, wId, slip) => {
        if (!amt || parseFloat(amt) <= 0 || !fToken || !tToken || fToken.length !== 42 || tToken.length !== 42) {
            setQuoteInfo(null);
            setQuoteError('');
            return;
        }
        setQuoting(true);
        setQuoteError('');
        setQuoteInfo(null);
        try {
            const resp = await walletSwapSingleQuote({
                apiBaseUrl, initData, chain: chainId, walletId: wId, fromToken: fToken, toToken: tToken, amount: amt, slippagePercent: parseFloat(slip)
            });
            setQuoteInfo(resp);
        } catch (e) {
            setQuoteError(String(e?.message || e));
            setQuoteInfo(null);
        } finally {
            setQuoting(false);
        }
    }, [apiBaseUrl, initData]);

    useEffect(() => {
        if (quoteTimeout.current) clearTimeout(quoteTimeout.current);
        quoteTimeout.current = setTimeout(() => {
            doQuote(amount, fromToken, toToken, chain, selectedWalletId, slippage);
        }, 800);
        return () => clearTimeout(quoteTimeout.current);
    }, [amount, fromToken, toToken, chain, selectedWalletId, slippage, doQuote]);

    const handleSwap = async () => {
        if (!initData) return;
        setExecuting(true);
        setExecError('');
        setExecSuccess('');
        try {
            const resp = await walletSwapSingleExecute({
                apiBaseUrl, initData, chain, walletId: selectedWalletId, fromToken, toToken, amount, slippagePercent: parseFloat(slippage)
            });
            setExecSuccess(resp?.tx_hash || '交易已提交');
            setShowConfirm(false);
            setAmount('');
            setQuoteInfo(null);
        } catch (e) {
            setExecError(String(e?.message || e));
            setShowConfirm(false);
        } finally {
            setExecuting(false);
        }
    };

    const handleReverse = () => {
        setFromToken(toToken);
        setToToken(fromToken);
        setAmount('');
        setQuoteInfo(null);
    };

    const isReadyToSwap = amount && fromToken && toToken && amount > 0 && quoteInfo && !quoting && !executing;

    return (
        <PanelShell 
            title="代币兑换" 
            subtitle="单代币闪兑 · 由 OKX DEX 路由" 
            icon={RefreshCw}
            actions={
                <button 
                    type="button" 
                    className="flex items-center gap-1.5 rounded-lg bg-zinc-100 px-3 py-1.5 text-xs font-semibold text-zinc-600 transition-colors hover:bg-zinc-200 dark:bg-white/10 dark:text-white/80 dark:hover:bg-white/20"
                    onClick={() => setShowSettings(!showSettings)}
                >
                    <Settings size={14} /> 滑点 {slippage}%
                </button>
            }
        >
            <div className="mx-auto max-w-lg">
                {/* Config & Wallet Select */}
                {showSettings && (
                    <div className="mb-4 rounded-2xl bg-zinc-50 p-4 dark:bg-white/5">
                        <div className="mb-3 flex justify-between gap-4">
                            <div className="flex-1">
                                <label className="mb-1 block text-xs font-semibold text-zinc-500 dark:text-zinc-400">选择钱包</label>
                                {walletLoading ? (
                                    <div className="text-xs text-zinc-400">加载钱包中...</div>
                                ) : (
                                    <select
                                        value={selectedWalletId}
                                        onChange={(e) => setSelectedWalletId(e.target.value)}
                                        className="w-full appearance-none rounded-lg border border-zinc-200 bg-white px-3 py-2 text-sm outline-none dark:border-white/10 dark:bg-black/20 dark:text-white"
                                    >
                                        {wallets.map(w => (
                                            <option key={w.id} value={w.id}>{w.name || '钱包'} ({shortAddress(w.address)}) - {chain==='bsc'?'BNB':'ETH'}: {parseFloat(w.native_balance).toFixed(3)}</option>
                                        ))}
                                    </select>
                                )}
                            </div>
                            <div className="flex-[0.7]">
                                <label className="mb-1 block text-xs font-semibold text-zinc-500 dark:text-zinc-400">自定义滑点 (%)</label>
                                <input
                                    type="number"
                                    value={slippage}
                                    onChange={(e) => setSlippage(e.target.value)}
                                    className="w-full rounded-lg border border-zinc-200 px-3 py-2 text-sm outline-none focus:border-indigo-500 dark:border-white/10 dark:bg-black/20 dark:text-white"
                                    placeholder="默认 1.0"
                                />
                            </div>
                        </div>
                    </div>
                )}

                {/* Swap Box */}
                <div className="relative rounded-3xl bg-zinc-50 p-1 dark:bg-white/5">
                    {/* From Box */}
                    <div className="rounded-2xl bg-white p-4 shadow-sm transition-colors hover:border-zinc-200 dark:bg-[#131518] min-h-[110px]">
                        <div className="mb-2 flex items-center justify-between text-sm text-zinc-500 dark:text-zinc-400">
                            <span className="font-medium text-xs">支付</span>
                        </div>
                        <div className="flex items-center gap-3">
                            <input
                                type="text"
                                value={amount}
                                onChange={(e) => setAmount(e.target.value)}
                                className="w-[45%] bg-transparent text-3xl font-bold text-zinc-900 outline-none placeholder:text-zinc-300 dark:text-white dark:placeholder:text-zinc-700"
                                placeholder="0.0"
                            />
                            <div className="flex-1">
                                <input
                                    type="text"
                                    value={fromToken}
                                    onChange={(e) => setFromToken(e.target.value)}
                                    className="w-full rounded-xl border border-transparent bg-zinc-50 px-3 py-2.5 text-xs font-mono text-zinc-900 outline-none focus:bg-white focus:ring-1 focus:ring-indigo-500/50 dark:bg-white/5 dark:text-white dark:focus:bg-black/20 dark:focus:ring-indigo-500/50"
                                    placeholder="输入代币合约地址..."
                                />
                            </div>
                        </div>
                    </div>

                    {/* Reverse Button (Absolute centered) */}
                    <div className="absolute left-1/2 top-1/2 z-10 -translate-x-1/2 -translate-y-1/2">
                        <button
                            onClick={handleReverse}
                            className="flex h-10 w-10 items-center justify-center rounded-xl border-4 border-zinc-50 bg-white text-zinc-400 transition-colors hover:text-indigo-500 dark:border-[#1c1d22] dark:bg-[#282a31] dark:hover:text-indigo-400"
                        >
                            <ArrowDown size={18} strokeWidth={3} />
                        </button>
                    </div>

                    {/* To Box */}
                    <div className="mt-1 rounded-2xl bg-white p-4 shadow-sm transition-colors hover:border-zinc-200 dark:bg-[#131518] min-h-[110px]">
                        <div className="mb-2 flex items-center justify-between text-sm text-zinc-500 dark:text-zinc-400">
                            <span className="font-medium text-xs">获得</span>
                        </div>
                        <div className="flex items-center gap-3">
                            <div className="w-[45%] text-3xl font-bold text-zinc-900 dark:text-white truncate">
                                {quoting ? (
                                    <span className="animate-pulse text-zinc-300 dark:text-zinc-700">...</span>
                                ) : (
                                    quoteInfo?.to_amount_float || '0.0'
                                )}
                            </div>
                            <div className="flex-1">
                                <input
                                    type="text"
                                    value={toToken}
                                    onChange={(e) => setToToken(e.target.value)}
                                    className="w-full rounded-xl border border-transparent bg-zinc-50 px-3 py-2.5 text-xs font-mono text-zinc-900 outline-none focus:bg-white focus:ring-1 focus:ring-indigo-500/50 dark:bg-white/5 dark:text-white dark:focus:bg-black/20 dark:focus:ring-indigo-500/50"
                                    placeholder="输入接收代币合约地址..."
                                />
                            </div>
                        </div>
                    </div>
                </div>

                {/* State Messages */}
                <div className="mt-4">
                    {quoteError && (
                        <div className="rounded-xl border border-amber-500/30 bg-amber-500/5 p-3 text-xs text-amber-700 dark:text-amber-400">
                            <strong>报价失败:</strong> {quoteError}
                        </div>
                    )}
                    {execError && (
                        <div className="rounded-xl border border-red-500/30 bg-red-500/5 p-3 text-xs text-red-700 dark:text-red-400">
                            <strong>兑换失败:</strong> {execError}
                        </div>
                    )}
                    {execSuccess && (
                        <div className="rounded-xl border border-emerald-500/30 bg-emerald-500/5 p-3 text-xs text-emerald-700 dark:text-emerald-400 break-all">
                            ✅ <strong>兑换请求已提交</strong><br/>
                            TxHash: <span className="font-mono opacity-80">{execSuccess}</span>
                        </div>
                    )}
                </div>

                {/* Confirm Dialog */}
                {showConfirm ? (
                    <div className="mt-4 rounded-xl border border-indigo-500/30 bg-indigo-500/5 p-4 text-sm">
                        <p className="mb-4 text-zinc-700 dark:text-zinc-300">
                            确认将支付 <strong>{amount}</strong> 个代币 <br/>
                            兑换为获得约 <strong>{quoteInfo?.to_amount_float || 0}</strong> 个目标代币？<br/>
                            <span className="text-xs text-zinc-500">滑点: {slippage}%</span>
                        </p>
                        <div className="flex gap-3">
                            <button className="panel-action-btn flex-1" onClick={() => setShowConfirm(false)}>取消</button>
                            <button className="config-save-btn flex-1 bg-indigo-500 text-white hover:bg-indigo-600" onClick={handleSwap} disabled={executing}>
                                {executing ? '执行中...' : '提交交易'}
                            </button>
                        </div>
                    </div>
                ) : (
                    <div className="mt-4">
                        <button
                            type="button"
                            onClick={() => setShowConfirm(true)}
                            disabled={!isReadyToSwap}
                            className={`w-full rounded-2xl py-4 text-base font-bold shadow-sm transition-all sm:py-4.5 ${!isReadyToSwap ? 'cursor-not-allowed bg-zinc-200 text-zinc-400 dark:bg-white/5 dark:text-white/30' : 'bg-indigo-500 text-white hover:bg-indigo-600'}`}
                        >
                            {executing ? '执行中...' : !fromToken ? '需填入支付合约' : !amount ? '需输入支付数量' : quoting ? '获取最优报价中...' : !quoteInfo ? '无法兑换' : '确认兑换'}
                        </button>
                    </div>
                )}
            </div>
        </PanelShell>
    );
}
