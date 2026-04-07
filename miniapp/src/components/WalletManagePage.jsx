import React, { useCallback, useEffect, useState } from 'react';
import BottomSheet from './BottomSheet.jsx';
import CustomSelect from './CustomSelect.jsx';
import { fetchWallets } from '../lib/api';
import { getBrandTheme } from '../lib/brand';

const CHAIN_OPTIONS = [
    { value: 'bsc', label: 'BSC', icon: '🟡' },
    { value: 'base', label: 'Base', icon: '🔵' },
];

export default function WalletManagePage({ open, onClose, apiBaseUrl, initData, accentTheme = 'lime', multiChainEnabled = true }) {
    const brand = getBrandTheme(accentTheme);
    const [chain, setChain] = useState('bsc');
    const [wallets, setWallets] = useState([]);
    const [nativeSymbol, setNativeSymbol] = useState('BNB');
    const [stableSymbol, setStableSymbol] = useState('USDT');
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState('');
    const [copiedAddr, setCopiedAddr] = useState('');

    const load = useCallback(async () => {
        if (!initData) return;
        setLoading(true);
        setError('');
        try {
            const resp = await fetchWallets({ apiBaseUrl, initData, chain });
            setWallets(resp?.wallets || []);
            setNativeSymbol(resp?.native_symbol || 'BNB');
            setStableSymbol(resp?.stable_symbol || 'USDT');
        } catch (e) {
            setError(String(e?.message || e));
        } finally {
            setLoading(false);
        }
    }, [apiBaseUrl, initData, chain]);

    useEffect(() => {
        if (open) load();
    }, [open, load]);

    const copyAddress = async (addr) => {
        try {
            await navigator.clipboard.writeText(addr);
            setCopiedAddr(addr);
            setTimeout(() => setCopiedAddr(''), 2000);
        } catch { /* ignore */ }
    };

    const totalNative = wallets.reduce((s, w) => {
        const v = parseFloat(w.native_balance);
        return s + (Number.isFinite(v) ? v : 0);
    }, 0);
    const totalStable = wallets.reduce((s, w) => {
        const v = parseFloat(w.stable_balance);
        return s + (Number.isFinite(v) ? v : 0);
    }, 0);

    return (
        <BottomSheet open={open} onClose={onClose} title="钱包管理" maxHeightClass="max-h-[90vh]">
            {/* Chain selector */}
            {multiChainEnabled && (
                <div className="mb-4">
                    <CustomSelect
                        value={chain}
                        onChange={setChain}
                        options={CHAIN_OPTIONS}
                        placeholder="选择链"
                    />
                </div>
            )}

            {error && (
                <div className="mb-4 rounded-xl border border-red-500/30 bg-red-500/10 p-3 text-xs text-red-700 dark:text-red-300">
                    {error}
                </div>
            )}

            {/* Summary */}
            {wallets.length > 0 && (
                <div className="mb-4 grid grid-cols-2 gap-3">
                    <div className="rounded-2xl border border-zinc-200/50 bg-zinc-50/50 p-3 dark:border-white/[0.06] dark:bg-white/[0.02]">
                        <div className="text-[11px] text-zinc-400 dark:text-white/30">{nativeSymbol} 总计</div>
                        <div className="mt-1 text-lg font-bold text-zinc-900 dark:text-white/90">
                            {totalNative.toFixed(4)}
                        </div>
                    </div>
                    <div className="rounded-2xl border border-zinc-200/50 bg-zinc-50/50 p-3 dark:border-white/[0.06] dark:bg-white/[0.02]">
                        <div className="text-[11px] text-zinc-400 dark:text-white/30">{stableSymbol} 总计</div>
                        <div className="mt-1 text-lg font-bold text-zinc-900 dark:text-white/90">
                            {totalStable.toFixed(2)}
                        </div>
                    </div>
                </div>
            )}

            {loading ? (
                <div className="flex items-center justify-center py-12 text-sm text-zinc-400 dark:text-white/40">
                    <svg className="mr-2 h-5 w-5 animate-spin" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                        <circle className="opacity-25" cx="12" cy="12" r="10" /><path className="opacity-75" d="M4 12a8 8 0 018-8" />
                    </svg>
                    加载中...
                </div>
            ) : wallets.length === 0 ? (
                <div className="py-12 text-center text-sm text-zinc-400 dark:text-white/40">
                    <div className="mb-2 text-3xl">🔑</div>
                    <div>暂无钱包</div>
                    <div className="mt-1 text-xs">请在 Telegram 机器人中使用 /wallet 命令导入钱包</div>
                </div>
            ) : (
                <div className="space-y-3">
                    {wallets.map((w) => (
                        <div
                            key={w.id || w.address}
                            className="rounded-2xl border border-zinc-200/50 bg-white/70 p-4 transition-colors dark:border-white/[0.06] dark:bg-white/[0.03]"
                        >
                            <div className="flex items-start justify-between gap-2">
                                <div className="min-w-0 flex-1">
                                    <div className="flex items-center gap-2">
                                        <span className="text-sm font-bold text-zinc-900 dark:text-white/90">
                                            {w.name || `钱包 ${w.id}`}
                                        </span>
                                        {w.is_default && (
                                            <span className="inline-flex items-center rounded-md bg-emerald-500/10 px-1.5 py-0.5 text-[10px] font-semibold text-emerald-600 ring-1 ring-emerald-500/20 dark:text-emerald-400">
                                                默认
                                            </span>
                                        )}
                                    </div>
                                    <button
                                        type="button"
                                        onClick={() => copyAddress(w.address)}
                                        className="mt-1 text-xs font-mono text-zinc-400 hover:text-zinc-600 dark:text-white/30 dark:hover:text-white/60 transition-colors"
                                        title="点击复制"
                                    >
                                        {w.address ? `${w.address.slice(0, 8)}...${w.address.slice(-6)}` : '-'}
                                        {copiedAddr === w.address && (
                                            <span className="ml-2 text-emerald-500">✓ 已复制</span>
                                        )}
                                    </button>
                                </div>
                            </div>
                            <div className="mt-3 grid grid-cols-2 gap-3">
                                <div>
                                    <div className="text-[11px] text-zinc-400 dark:text-white/30">{nativeSymbol}</div>
                                    <div className="mt-0.5 text-sm font-semibold text-zinc-900 dark:text-white/80">
                                        {w.native_balance === 'N/A' ? '-' : parseFloat(w.native_balance).toFixed(4)}
                                    </div>
                                </div>
                                <div>
                                    <div className="text-[11px] text-zinc-400 dark:text-white/30">{stableSymbol}</div>
                                    <div className="mt-0.5 text-sm font-semibold text-zinc-900 dark:text-white/80">
                                        {w.stable_balance === 'N/A' ? '-' : parseFloat(w.stable_balance).toFixed(2)}
                                    </div>
                                </div>
                            </div>
                        </div>
                    ))}
                </div>
            )}

            {/* Hint */}
            <div className="mt-4 rounded-xl border border-amber-500/20 bg-amber-500/5 p-3 text-xs text-amber-700 dark:text-amber-300/80">
                <div className="flex items-start gap-2">
                    <span className="text-base">🔒</span>
                    <div>
                        <div className="font-semibold">安全提示</div>
                        <div className="mt-0.5 text-amber-600 dark:text-amber-400/70">导入/删除钱包和设置默认钱包请在 Telegram 机器人的 /wallet 命令中操作，以确保私钥安全。</div>
                    </div>
                </div>
            </div>

            {/* Refresh button */}
            <div className="mt-4">
                <button
                    type="button"
                    onClick={load}
                    disabled={loading}
                    className={`w-full rounded-xl px-4 py-3 text-sm font-bold shadow-sm transition-all ${loading ? 'cursor-not-allowed opacity-50' : ''} ${brand.solidButtonClass}`}
                >
                    {loading ? '刷新中...' : '刷新余额'}
                </button>
            </div>
        </BottomSheet>
    );
}

