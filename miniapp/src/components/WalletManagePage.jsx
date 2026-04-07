import React, { useCallback, useEffect, useState } from 'react';
import BottomSheet from './BottomSheet.jsx';
import CustomSelect from './CustomSelect.jsx';
import { fetchWallets, walletCRUD } from '../lib/api';
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
    const [crudAction, setCrudAction] = useState(null); // 'import', 'create', 'rename'
    const [crudForm, setCrudForm] = useState({ name: '', privateKey: '', walletId: null });
    const [crudLoading, setCrudLoading] = useState(false);

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
            setWallets([]); // clear on error
        } finally {
            setLoading(false);
        }
    }, [apiBaseUrl, initData, chain]);

    useEffect(() => {
        if (open) {
            load();
            setCrudAction(null);
            setError('');
        }
    }, [open, load]);

    const copyAddress = async (addr) => {
        try {
            await navigator.clipboard.writeText(addr);
            setCopiedAddr(addr);
            setTimeout(() => setCopiedAddr(''), 2000);
        } catch { /* ignore */ }
    };

    const handleCrudSubmit = async (e) => {
        e.preventDefault();
        setCrudLoading(true);
        setError('');
        try {
            await walletCRUD({
                apiBaseUrl,
                initData,
                action: crudAction,
                privateKey: crudForm.privateKey,
                name: crudForm.name,
                walletId: crudForm.walletId,
            });
            setCrudAction(null);
            setCrudForm({ name: '', privateKey: '', walletId: null });
            await load();
        } catch (err) {
            setError(err.message || '操作失败');
        } finally {
            setCrudLoading(false);
        }
    };

    const handleAction = async (action, w) => {
        if (action === 'set_default' || action === 'delete') {
            if (action === 'delete' && !window.confirm(`确定要删除钱包 ${w.name || w.address.slice(0,6)} 吗？`)) {
                return;
            }
            setLoading(true);
            setError('');
            try {
                await walletCRUD({
                    apiBaseUrl,
                    initData,
                    action,
                    walletId: w.id,
                });
                await load();
            } catch (err) {
                setError(err.message || '操作失败');
                setLoading(false);
            }
        } else if (action === 'rename') {
            setCrudAction('rename');
            setCrudForm({ name: w.name || '', privateKey: '', walletId: w.id });
        }
    };

    const totalNative = wallets.reduce((s, w) => {
        const v = parseFloat(w.native_balance);
        return s + (Number.isFinite(v) ? v : 0);
    }, 0);
    const totalStable = wallets.reduce((s, w) => {
        const v = parseFloat(w.stable_balance);
        return s + (Number.isFinite(v) ? v : 0);
    }, 0);

    const renderCrudForm = () => {
        if (!crudAction) return null;
        const title = crudAction === 'import' ? '导入钱包' : crudAction === 'create' ? '创建钱包' : '重命名钱包';
        return (
            <div className="mb-4 rounded-xl border border-zinc-200/50 bg-zinc-50 p-4 dark:border-white/10 dark:bg-black/20">
                <h3 className="mb-3 text-sm font-bold text-zinc-900 dark:text-white">{title}</h3>
                <form onSubmit={handleCrudSubmit} className="space-y-3">
                    {crudAction === 'import' && (
                        <div>
                            <label className="mb-1 block text-xs text-zinc-500 dark:text-zinc-400">私钥 (Hex)</label>
                            <input
                                type="text"
                                value={crudForm.privateKey}
                                onChange={(e) => setCrudForm({ ...crudForm, privateKey: e.target.value })}
                                className="w-full rounded-lg border border-zinc-200 bg-white px-3 py-2 text-sm text-zinc-900 placeholder-zinc-400 focus:border-emerald-500 focus:outline-none dark:border-zinc-700 dark:bg-zinc-900 dark:text-white dark:focus:border-emerald-500"
                                placeholder="输入私钥..."
                                required
                            />
                        </div>
                    )}
                    <div>
                        <label className="mb-1 block text-xs text-zinc-500 dark:text-zinc-400">钱包名称</label>
                        <input
                            type="text"
                            value={crudForm.name}
                            onChange={(e) => setCrudForm({ ...crudForm, name: e.target.value })}
                            className="w-full rounded-lg border border-zinc-200 bg-white px-3 py-2 text-sm text-zinc-900 placeholder-zinc-400 focus:border-emerald-500 focus:outline-none dark:border-zinc-700 dark:bg-zinc-900 dark:text-white dark:focus:border-emerald-500"
                            placeholder="如: 常用钱包1"
                            required
                        />
                    </div>
                    <div className="flex justify-end gap-2 pt-2">
                        <button
                            type="button"
                            onClick={() => setCrudAction(null)}
                            className="rounded-lg px-4 py-2 text-xs font-semibold text-zinc-500 hover:bg-zinc-200 dark:hover:bg-zinc-800"
                        >
                            取消
                        </button>
                        <button
                            type="submit"
                            disabled={crudLoading}
                            className={`rounded-lg px-4 py-2 text-xs font-semibold shadow-sm transition-all ${crudLoading ? 'cursor-not-allowed opacity-50' : ''} ${brand.solidButtonClass}`}
                        >
                            {crudLoading ? '处理中...' : '确定'}
                        </button>
                    </div>
                </form>
            </div>
        );
    };

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

            {/* Actions Top */}
            {!crudAction && (
                <div className="mb-4 flex gap-2">
                    <button
                        onClick={() => { setCrudAction('create'); setCrudForm({ name: '', privateKey: '', walletId: null }); }}
                        className="flex-1 rounded-xl bg-zinc-100 py-2.5 text-center text-sm font-semibold text-zinc-700 hover:bg-zinc-200 dark:bg-white/5 dark:text-white/80 dark:hover:bg-white/10 transition-colors"
                    >
                        ✨ 创建新钱包
                    </button>
                    <button
                        onClick={() => { setCrudAction('import'); setCrudForm({ name: '', privateKey: '', walletId: null }); }}
                        className="flex-1 rounded-xl bg-zinc-100 py-2.5 text-center text-sm font-semibold text-zinc-700 hover:bg-zinc-200 dark:bg-white/5 dark:text-white/80 dark:hover:bg-white/10 transition-colors"
                    >
                        🔑 导入钱包
                    </button>
                </div>
            )}

            {renderCrudForm()}

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

            {loading && !wallets.length ? (
                <div className="flex items-center justify-center py-12 text-sm text-zinc-400 dark:text-white/40">
                    <svg className="mr-2 h-5 w-5 animate-spin" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                        <circle className="opacity-25" cx="12" cy="12" r="10" /><path className="opacity-75" d="M4 12a8 8 0 018-8" />
                    </svg>
                    加载中...
                </div>
            ) : wallets.length === 0 ? (
                <div className="py-12 text-center text-sm text-zinc-400 dark:text-white/40">
                    <div className="mb-2 text-3xl">🪹</div>
                    <div>暂无钱包</div>
                    <div className="mt-1 text-xs">请点击上方按钮导入或创建钱包</div>
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
                                        className="mt-1 text-[11px] font-mono text-zinc-400 hover:text-zinc-600 dark:text-white/30 dark:hover:text-white/60 transition-colors"
                                        title="点击复制"
                                    >
                                        {w.address ? `${w.address.slice(0, 10)}...${w.address.slice(-8)}` : '-'}
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
                            {/* Wallet Actions */}
                            <div className="mt-3 mt-4 flex items-center gap-2 border-t border-zinc-100 pt-3 dark:border-white/5">
                                {!w.is_default && (
                                    <button
                                        onClick={() => handleAction('set_default', w)}
                                        className="rounded-lg bg-zinc-100 px-3 py-1.5 text-[11px] font-medium text-zinc-600 hover:bg-zinc-200 dark:bg-white/5 dark:text-white/60 dark:hover:bg-white/10 dark:hover:text-white/80 transition-colors"
                                    >
                                        设为默认
                                    </button>
                                )}
                                <button
                                    onClick={() => handleAction('rename', w)}
                                    className="rounded-lg bg-zinc-100 px-3 py-1.5 text-[11px] font-medium text-zinc-600 hover:bg-zinc-200 dark:bg-white/5 dark:text-white/60 dark:hover:bg-white/10 dark:hover:text-white/80 transition-colors"
                                >
                                    重命名
                                </button>
                                <button
                                    onClick={() => handleAction('delete', w)}
                                    className="ml-auto rounded-lg px-3 py-1.5 text-[11px] font-medium text-red-500 hover:bg-red-50 dark:text-red-400 dark:hover:bg-red-500/10 transition-colors"
                                >
                                    删除
                                </button>
                            </div>
                        </div>
                    ))}
                </div>
            )}

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
