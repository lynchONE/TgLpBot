import React, { useCallback, useEffect, useState } from 'react';
import BottomSheet from './BottomSheet.jsx';
import CustomSelect from './CustomSelect.jsx';
import ConfirmDialog from './ConfirmDialog.jsx';
import { walletSwapPreview, walletSwapExecute } from '../lib/api';
import { getBrandTheme } from '../lib/brand';

const CHAIN_OPTIONS = [
    { value: 'bsc', label: 'BSC', icon: '🟡' },
    { value: 'base', label: 'Base', icon: '🔵' },
];

export default function SwapPage({ open, onClose, apiBaseUrl, initData, accentTheme = 'lime', multiChainEnabled = true }) {
    const brand = getBrandTheme(accentTheme);
    const [chain, setChain] = useState('bsc');
    const [tokens, setTokens] = useState([]);
    const [scanning, setScanning] = useState(false);
    const [scanError, setScanError] = useState('');
    const [scanned, setScanned] = useState(false);

    const [showConfirm, setShowConfirm] = useState(false);
    const [executing, setExecuting] = useState(false);
    const [result, setResult] = useState(null);
    const [execError, setExecError] = useState('');

    const scan = useCallback(async () => {
        if (!initData) return;
        setScanning(true);
        setScanError('');
        setScanned(false);
        setResult(null);
        setExecError('');
        try {
            const resp = await walletSwapPreview({ apiBaseUrl, initData, chain, minValueUsd: 0.1 });
            setTokens(resp?.tokens || []);
            setScanned(true);
        } catch (e) {
            setScanError(String(e?.message || e));
        } finally {
            setScanning(false);
        }
    }, [apiBaseUrl, initData, chain]);

    useEffect(() => {
        if (open) {
            setTokens([]);
            setScanned(false);
            setResult(null);
            setExecError('');
        }
    }, [open, chain]);

    const totalValue = tokens.reduce((s, t) => s + (t.value_usdt || 0), 0);

    const doSwap = useCallback(async () => {
        if (!initData) return;
        setExecuting(true);
        setExecError('');
        setResult(null);
        try {
            const resp = await walletSwapExecute({ apiBaseUrl, initData, chain });
            setResult(resp);
            setShowConfirm(false);
            // Re-scan to refresh
            setScanned(false);
        } catch (e) {
            setExecError(String(e?.message || e));
            setShowConfirm(false);
        } finally {
            setExecuting(false);
        }
    }, [apiBaseUrl, initData, chain]);

    return (
        <BottomSheet open={open} onClose={onClose} title="一键兑换" maxHeightClass="max-h-[90vh]">
            <div className="mb-1 text-xs text-zinc-500 dark:text-white/40">
                将钱包中的零散代币一键兑换为 USDT（稳定币）
            </div>

            {/* Chain selector */}
            {multiChainEnabled && (
                <div className="mt-3 mb-4">
                    <CustomSelect
                        value={chain}
                        onChange={setChain}
                        options={CHAIN_OPTIONS}
                        placeholder="选择链"
                    />
                </div>
            )}

            {/* Scan button */}
            {!scanned && !scanning && (
                <button
                    type="button"
                    onClick={scan}
                    disabled={scanning}
                    className={`w-full rounded-xl px-4 py-3 text-sm font-bold shadow-sm transition-all ${brand.solidButtonClass}`}
                >
                    🔍 扫描可兑换代币
                </button>
            )}

            {scanError && (
                <div className="mt-3 rounded-xl border border-red-500/30 bg-red-500/10 p-3 text-xs text-red-700 dark:text-red-300">
                    {scanError}
                </div>
            )}

            {scanning && (
                <div className="flex items-center justify-center py-12 text-sm text-zinc-400 dark:text-white/40">
                    <svg className="mr-2 h-5 w-5 animate-spin" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                        <circle className="opacity-25" cx="12" cy="12" r="10" /><path className="opacity-75" d="M4 12a8 8 0 018-8" />
                    </svg>
                    扫描中，请稍候...
                </div>
            )}

            {/* Scan results */}
            {scanned && !scanning && (
                <div className="mt-3 space-y-3">
                    {tokens.length === 0 ? (
                        <div className="py-8 text-center text-sm text-zinc-400 dark:text-white/40">
                            <div className="mb-2 text-3xl">✨</div>
                            <div>没有找到可兑换的代币</div>
                            <div className="mt-1 text-xs">钱包已经很干净了</div>
                        </div>
                    ) : (
                        <>
                            <div className="rounded-2xl border border-zinc-200/50 bg-zinc-50/50 p-3 dark:border-white/[0.06] dark:bg-white/[0.02]">
                                <div className="text-[11px] text-zinc-400 dark:text-white/30">可兑换代币总价值</div>
                                <div className="mt-1 text-xl font-bold text-zinc-900 dark:text-white/90">
                                    ≈ ${totalValue.toFixed(2)} USDT
                                </div>
                                <div className="mt-1 text-xs text-zinc-400 dark:text-white/30">共 {tokens.length} 个代币</div>
                            </div>

                            <div className="space-y-2">
                                {tokens.map((t, i) => (
                                    <div
                                        key={i}
                                        className="flex items-center justify-between rounded-xl border border-zinc-200/50 bg-white/70 px-4 py-3 dark:border-white/[0.06] dark:bg-white/[0.03]"
                                    >
                                        <div className="min-w-0">
                                            <div className="text-sm font-semibold text-zinc-900 dark:text-white/90">{t.symbol || '???'}</div>
                                            <div className="mt-0.5 text-xs text-zinc-400 dark:text-white/30">{t.balance}</div>
                                        </div>
                                        <div className="text-right">
                                            <div className="text-sm font-bold text-zinc-900 dark:text-white/90">
                                                ≈ ${t.value_usdt?.toFixed(2) ?? '0.00'}
                                            </div>
                                        </div>
                                    </div>
                                ))}
                            </div>

                            <div className="flex gap-3">
                                <button
                                    type="button"
                                    onClick={scan}
                                    disabled={scanning}
                                    className="flex-1 rounded-xl border border-zinc-200 bg-white px-4 py-3 text-sm font-bold text-zinc-700 transition-colors hover:bg-zinc-50 dark:border-white/10 dark:bg-white/5 dark:text-white/70 dark:hover:bg-white/10"
                                >
                                    重新扫描
                                </button>
                                <button
                                    type="button"
                                    onClick={() => setShowConfirm(true)}
                                    disabled={executing}
                                    className={`flex-1 rounded-xl px-4 py-3 text-sm font-bold shadow-sm transition-all ${executing ? 'cursor-not-allowed opacity-50' : ''} ${brand.solidButtonClass}`}
                                >
                                    🔄 全部兑换
                                </button>
                            </div>
                        </>
                    )}
                </div>
            )}

            {/* Execution result */}
            {execError && (
                <div className="mt-3 rounded-xl border border-red-500/30 bg-red-500/10 p-3 text-xs text-red-700 dark:text-red-300">
                    兑换失败: {execError}
                </div>
            )}

            {result && (
                <div className="mt-3 space-y-2">
                    {result.swapped?.length > 0 && (
                        <div className="rounded-xl border border-emerald-500/30 bg-emerald-500/10 p-3">
                            <div className="text-xs font-semibold text-emerald-700 dark:text-emerald-300">✅ 兑换成功</div>
                            <div className="mt-1 space-y-0.5 text-xs text-emerald-600 dark:text-emerald-400/80">
                                {result.swapped.map((s, i) => <div key={i}>{s}</div>)}
                            </div>
                        </div>
                    )}
                    {result.failed?.length > 0 && (
                        <div className="rounded-xl border border-red-500/30 bg-red-500/10 p-3">
                            <div className="text-xs font-semibold text-red-700 dark:text-red-300">❌ 兑换失败</div>
                            <div className="mt-1 space-y-0.5 text-xs text-red-600 dark:text-red-400/80">
                                {result.failed.map((f, i) => <div key={i}>{f}</div>)}
                            </div>
                        </div>
                    )}
                </div>
            )}

            <ConfirmDialog
                open={showConfirm}
                title="确认兑换"
                message={`将 ${tokens.length} 个代币（总价值 ≈ $${totalValue.toFixed(2)}）全部兑换为 USDT？\n\n此操作不可撤销。`}
                confirmText="确认兑换"
                cancelText="取消"
                danger={false}
                loading={executing}
                onConfirm={doSwap}
                onCancel={() => setShowConfirm(false)}
            />
        </BottomSheet>
    );
}

