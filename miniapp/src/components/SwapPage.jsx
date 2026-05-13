import React, { useCallback, useEffect, useState, useRef } from 'react';
import BottomSheet from './BottomSheet.jsx';
import CustomSelect from './CustomSelect.jsx';
import ConfirmDialog from './ConfirmDialog.jsx';
import { fetchWallets, walletSwapSingleQuote, walletSwapSingleExecute } from '../lib/api';
import { getBrandTheme } from '../lib/brand';
import { ChevronDown, ArrowDown, Settings } from 'lucide-react';

const CHAIN_OPTIONS = [
    { value: 'bsc', label: 'BSC', icon: '🟡' },
    { value: 'base', label: 'Base', icon: '🔵' },
];

const STABLE_ADDRESSES = {
    'bsc': '0x55d398326f99059fF775485246999027B3197955', // USDT
    'base': '0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913' // USDC
};

export default function SwapPage({ open, onClose, apiBaseUrl, initData, accentTheme = 'lime', multiChainEnabled = true }) {
    const brand = getBrandTheme(accentTheme);
    const [chain, setChain] = useState('bsc');
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
        if (open) {
            loadWallets();
            setExecError('');
            setExecSuccess('');
        }
    }, [open, loadWallets]);

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

    // Auto quote on input change
    useEffect(() => {
        if (!open) return;
        if (quoteTimeout.current) clearTimeout(quoteTimeout.current);
        quoteTimeout.current = setTimeout(() => {
            doQuote(amount, fromToken, toToken, chain, selectedWalletId, slippage);
        }, 800);
        return () => clearTimeout(quoteTimeout.current);
    }, [amount, fromToken, toToken, chain, selectedWalletId, slippage, doQuote, open]);

    const handleSwap = async () => {
        if (!initData) return;
        setExecuting(true);
        setExecError('');
        setExecSuccess('');
        try {
            const resp = await walletSwapSingleExecute({
                apiBaseUrl, initData, chain, walletId: selectedWalletId, fromToken, toToken, amount, slippagePercent: parseFloat(slippage), provider: quoteInfo?.best_provider || quoteInfo?.provider
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

    const getWalletOptions = () => {
        return wallets.map(w => ({
            value: w.id,
            label: `${w.name || '钱包'} (${w.address.slice(0,6)}...${w.address.slice(-4)})`
        }));
    };

    return (
        <BottomSheet open={open} onClose={onClose} title="代币兑换" maxHeightClass="max-h-[90vh]">
            {/* Header / Network */}
            <div className="mb-4 flex items-center justify-between">
                {multiChainEnabled ? (
                    <div className="w-1/2">
                        <CustomSelect value={chain} onChange={setChain} options={CHAIN_OPTIONS} />
                    </div>
                ) : <div />}
                <button 
                    onClick={() => setShowSettings(!showSettings)}
                    className="flex items-center gap-1.5 rounded-full bg-zinc-100 px-3 py-1.5 text-xs font-semibold text-zinc-600 transition-colors hover:bg-zinc-200 dark:bg-white/10 dark:text-white/80 dark:hover:bg-white/20"
                >
                    <Settings size={14} /> {slippage}%
                </button>
            </div>

            {/* Config & Wallet Select */}
            {showSettings && (
                <div className="mb-4 rounded-2xl bg-zinc-50 p-4 dark:bg-white/5">
                    <div className="mb-3">
                        <label className="mb-1 block text-xs font-semibold text-zinc-500 dark:text-zinc-400">滑点 (Slippage Tolerance)</label>
                        <div className="flex gap-2">
                            {['0.5', '1.0', '3.0'].map(val => (
                                <button
                                    key={val}
                                    onClick={() => setSlippage(val)}
                                    className={`flex-1 rounded-lg py-1.5 text-sm font-semibold transition-colors ${slippage === val ? brand.solidButtonClass : 'bg-white text-zinc-700 shadow-sm dark:bg-black/20 dark:text-white/80'}`}
                                >
                                    {val}%
                                </button>
                            ))}
                            <div className="relative flex-1">
                                <input
                                    type="number"
                                    value={slippage}
                                    onChange={(e) => setSlippage(e.target.value)}
                                    className="w-full rounded-lg border border-zinc-200 px-3 py-1.5 text-right text-sm outline-none focus:border-indigo-500 dark:border-white/10 dark:bg-black/20 dark:text-white focus:dark:border-indigo-500"
                                    placeholder="自定义"
                                />
                                <span className="absolute right-3 top-1/2 -translate-y-1/2 text-xs font-semibold text-zinc-400">%</span>
                            </div>
                        </div>
                    </div>
                    <div>
                        <label className="mb-1 block text-xs font-semibold text-zinc-500 dark:text-zinc-400">选择钱包</label>
                        {walletLoading ? (
                            <div className="text-xs text-zinc-400">加载钱包中...</div>
                        ) : (
                            <CustomSelect
                                value={selectedWalletId}
                                onChange={setSelectedWalletId}
                                options={wallets.map(w => ({
                                    value: w.id,
                                    label: `${w.name || '钱包'} (${w.address.slice(0, 6)}...${w.address.slice(-4)}) - ${chain === 'bsc' ? 'BNB' : 'ETH'}: ${w.native_balance}`,
                                }))}
                                placeholder="选择钱包"
                            />
                        )}
                    </div>
                </div>
            )}

            {/* Swap Box */}
            <div className="relative rounded-3xl bg-zinc-50 p-1 dark:bg-white/5">
                {/* From Box */}
                <div className="rounded-2xl bg-white p-4 shadow-sm transition-colors hover:border-zinc-200 dark:bg-[#131518] dark:shadow-none min-h-[100px]">
                    <div className="mb-2 flex items-center justify-between text-sm text-zinc-500 dark:text-zinc-400">
                        <span className="font-medium text-xs">支付 (Token Contract)</span>
                    </div>
                    <div className="flex items-center gap-3">
                        <input
                            type="text"
                            value={amount}
                            onChange={(e) => setAmount(e.target.value)}
                            className="w-1/3 bg-transparent text-3xl font-bold text-zinc-900 outline-none placeholder:text-zinc-300 dark:text-white dark:placeholder:text-zinc-700"
                            placeholder="0.0"
                        />
                        <input
                            type="text"
                            value={fromToken}
                            onChange={(e) => setFromToken(e.target.value)}
                            className="flex-1 rounded-xl border border-transparent bg-zinc-50 px-3 py-2 text-xs font-mono text-zinc-900 outline-none focus:border-indigo-500/50 focus:bg-white dark:bg-white/5 dark:text-white dark:focus:border-indigo-400/30 dark:focus:bg-black/20"
                            placeholder="输入代币合约地址..."
                        />
                    </div>
                </div>

                {/* Arrow */}
                <div className="absolute left-1/2 top-1/2 z-10 -translate-x-1/2 -translate-y-1/2">
                    <button
                        onClick={handleReverse}
                        className="flex h-10 w-10 items-center justify-center rounded-xl border-4 border-zinc-50 bg-white text-zinc-400 transition-colors hover:text-indigo-500 dark:border-[#131518] dark:bg-[#1f2128] dark:hover:text-indigo-400"
                    >
                        <ArrowDown size={18} strokeWidth={3} />
                    </button>
                </div>

                {/* To Box */}
                <div className="mt-1 rounded-2xl bg-white p-4 shadow-sm transition-colors hover:border-zinc-200 dark:bg-[#131518] dark:shadow-none min-h-[100px]">
                    <div className="mb-2 flex items-center justify-between text-sm text-zinc-500 dark:text-zinc-400">
                        <span className="font-medium text-xs">获得 (Token Contract)</span>
                    </div>
                    <div className="flex flex-col gap-1">
                        <div className="flex items-center gap-3">
                            <div className="w-1/3 text-3xl font-bold text-zinc-900 dark:text-white truncate">
                                {quoting ? (
                                    <span className="animate-pulse text-zinc-300 dark:text-zinc-700">报价中...</span>
                                ) : (
                                    quoteInfo?.to_amount_float || '0.0'
                                )}
                            </div>
                            <input
                                type="text"
                                value={toToken}
                                onChange={(e) => setToToken(e.target.value)}
                                className="flex-1 rounded-xl border border-transparent bg-zinc-50 px-3 py-2 text-xs font-mono text-zinc-900 outline-none focus:border-indigo-500/50 focus:bg-white dark:bg-white/5 dark:text-white dark:focus:border-indigo-400/30 dark:focus:bg-black/20"
                                placeholder="输入接收代币合约地址..."
                            />
                        </div>
                    </div>
                </div>
            </div>

            {/* Error / Success Messages */}
            <div className="mt-4">
                {quoteError && (
                    <div className="rounded-xl border border-amber-500/30 bg-amber-500/10 p-3 text-xs text-amber-700 dark:text-amber-400">
                        报价失败: {quoteError}
                    </div>
                )}
                {execError && (
                    <div className="rounded-xl border border-red-500/30 bg-red-500/10 p-3 text-xs text-red-700 dark:text-red-300">
                        兑换失败: {execError}
                    </div>
                )}
                {execSuccess && (
                    <div className="rounded-xl border border-emerald-500/30 bg-emerald-500/10 p-3 text-xs text-emerald-700 dark:text-emerald-300 break-all">
                        ✅ 兑换请求已提交<br/>TxHash: {execSuccess}
                    </div>
                )}
            </div>

            {/* Swap Button */}
            <div className="mt-4">
                <button
                    type="button"
                    onClick={() => setShowConfirm(true)}
                    disabled={!amount || !fromToken || !toToken || amount <= 0 || !quoteInfo || quoting || executing}
                    className={`w-full rounded-2xl py-4 text-base font-bold text-white shadow-sm transition-all sm:py-4.5 ${(!amount || !fromToken || !toToken || amount <= 0 || !quoteInfo || quoting || executing) ? 'cursor-not-allowed bg-zinc-200 text-zinc-400 dark:bg-white/5 dark:text-white/30' : brand.solidButtonClass}`}
                >
                    {executing ? '执行中...' : !fromToken ? '需输入合约' : !amount ? '输入金额' : quoting ? '获取报价中...' : !quoteInfo ? '无法兑换' : '确认兑换'}
                </button>
            </div>

            {/* Confirm Dialog */}
            <ConfirmDialog
                open={showConfirm}
                title="确认发起兑换？"
                message={`支付: ${amount} 代币 (合约...${fromToken.slice(-6)})\n接收: ≈ ${quoteInfo?.to_amount_float || 0} 代币\n滑点: ${slippage}%\n钱包: ...${(wallets.find(w => w.id == selectedWalletId)?.address || '').slice(-6)}\n\n此操作由 OKX DEX 路由，不可逆。`}
                confirmText="提交交易"
                cancelText="取消"
                danger={false}
                loading={executing}
                onConfirm={handleSwap}
                onCancel={() => setShowConfirm(false)}
            />
        </BottomSheet>
    );
}
