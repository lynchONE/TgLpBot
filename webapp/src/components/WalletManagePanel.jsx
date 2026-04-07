import React, { useCallback, useEffect, useState } from 'react';
import { fetchWallets, walletCRUD } from '../api';
import PanelShell from './PanelShell';
import { Wallet, Plus, Download, Edit2, Star, Trash2 } from 'lucide-react';
import { shortAddress } from '../utils';

export default function WalletManagePanel({ apiBaseUrl, initData, hasInitData, chain = 'bsc' }) {
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
        if (hasInitData) {
            load();
            setCrudAction(null);
            setError('');
        }
    }, [load, hasInitData]);

    const copyAddress = async (addr) => {
        try {
            await navigator.clipboard.writeText(addr);
            setCopiedAddr(addr);
            setTimeout(() => setCopiedAddr(''), 2000);
        } catch { }
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
            if (action === 'delete') {
                if (!window.confirm(`确定要删除钱包 ${w.name || shortAddress(w.address)} 吗？`)) return;
            }
            setLoading(true);
            setError('');
            try {
                await walletCRUD({ apiBaseUrl, initData, action, walletId: w.id });
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

    const renderCrudForm = () => {
        if (!crudAction) return null;
        const title = crudAction === 'import' ? '导入钱包' : crudAction === 'create' ? '创建钱包' : '重命名钱包';
        return (
            <div className="am-card mb-4 bg-zinc-50/50 dark:bg-white/[0.02]">
                <h3 className="mb-3 text-sm font-bold text-zinc-900 dark:text-white">{title}</h3>
                <form onSubmit={handleCrudSubmit} className="space-y-3">
                    {crudAction === 'import' && (
                        <div>
                            <label className="mb-1 block text-xs text-zinc-500 dark:text-zinc-400">私钥 (Hex)</label>
                            <input
                                type="text"
                                value={crudForm.privateKey}
                                onChange={(e) => setCrudForm({ ...crudForm, privateKey: e.target.value })}
                                className="w-full rounded-lg border border-zinc-200 bg-white px-3 py-2 text-sm text-zinc-900 placeholder-zinc-400 focus:border-indigo-500 focus:outline-none dark:border-zinc-700/50 dark:bg-black/20 dark:text-white dark:focus:border-indigo-500"
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
                            className="w-full rounded-lg border border-zinc-200 bg-white px-3 py-2 text-sm text-zinc-900 placeholder-zinc-400 focus:border-indigo-500 focus:outline-none dark:border-zinc-700/50 dark:bg-black/20 dark:text-white dark:focus:border-indigo-500"
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
                            className={`rounded-lg bg-indigo-500 px-4 py-2 text-xs font-semibold text-white transition-colors hover:bg-indigo-600 ${crudLoading ? 'cursor-not-allowed opacity-50' : ''}`}
                        >
                            {crudLoading ? '处理中...' : '确定'}
                        </button>
                    </div>
                </form>
            </div>
        );
    };

    return (
        <PanelShell
            title="钱包管理"
            subtitle={`${wallets.length} 个钱包 · ${chain.toUpperCase()}`}
            icon={Wallet}
            actions={<button type="button" className="panel-action-btn" onClick={load} disabled={loading}>{loading ? '刷新中...' : '刷新'}</button>}
        >
            {error && <div className="panel-error">{error}</div>}

            {!crudAction && (
                <div className="mb-4 flex gap-2">
                    <button
                        onClick={() => { setCrudAction('create'); setCrudForm({ name: '', privateKey: '', walletId: null }); }}
                        className="flex flex-1 items-center justify-center gap-1.5 rounded-xl border border-zinc-200/50 bg-white py-2.5 text-sm font-semibold text-zinc-700 hover:bg-zinc-50 dark:border-white/10 dark:bg-black/20 dark:text-white/80 dark:hover:bg-white/5 transition-colors"
                    >
                        <Plus size={16} /> 创建新钱包
                    </button>
                    <button
                        onClick={() => { setCrudAction('import'); setCrudForm({ name: '', privateKey: '', walletId: null }); }}
                        className="flex flex-1 items-center justify-center gap-1.5 rounded-xl border border-zinc-200/50 bg-white py-2.5 text-sm font-semibold text-zinc-700 hover:bg-zinc-50 dark:border-white/10 dark:bg-black/20 dark:text-white/80 dark:hover:bg-white/5 transition-colors"
                    >
                        <Download size={16} /> 导入钱包
                    </button>
                </div>
            )}

            {renderCrudForm()}

            {loading && wallets.length === 0 ? (
                <div className="panel-loading">加载中...</div>
            ) : wallets.length === 0 ? (
                <div className="empty-state">暂无钱包，请点击上方按钮导入或创建</div>
            ) : (
                <div className="wallet-list">
                    {wallets.map((w) => (
                        <div key={w.id || w.address} className="wallet-card">
                            <div className="wallet-card-header">
                                <span className="wallet-name">{w.name || `钱包 ${w.id}`}</span>
                                {w.is_default && <span className="wallet-badge">默认</span>}
                            </div>
                            <button type="button" className="wallet-addr mb-2" onClick={() => copyAddress(w.address)} title="点击复制">
                                {shortAddress(w.address)}
                                {copiedAddr === w.address && <span className="copy-ok"> ✓</span>}
                            </button>
                            <div className="wallet-balances mb-3">
                                <span>{nativeSymbol}: {w.native_balance === 'N/A' ? '-' : parseFloat(w.native_balance).toFixed(4)}</span>
                                <span>{stableSymbol}: {w.stable_balance === 'N/A' ? '-' : parseFloat(w.stable_balance).toFixed(2)}</span>
                            </div>
                            <div className="flex items-center gap-2 border-t border-zinc-100 pt-3 dark:border-white/5">
                                {!w.is_default && (
                                    <button
                                        onClick={() => handleAction('set_default', w)}
                                        className="flex items-center gap-1 rounded-lg bg-zinc-100 px-2 py-1.5 text-[11px] font-medium text-zinc-600 hover:bg-zinc-200 dark:bg-white/5 dark:text-white/60 dark:hover:bg-white/10 dark:hover:text-white/80 transition-colors"
                                    >
                                        <Star size={12} /> 设为默认
                                    </button>
                                )}
                                <button
                                    onClick={() => handleAction('rename', w)}
                                    className="flex items-center gap-1 rounded-lg bg-zinc-100 px-2 py-1.5 text-[11px] font-medium text-zinc-600 hover:bg-zinc-200 dark:bg-white/5 dark:text-white/60 dark:hover:bg-white/10 dark:hover:text-white/80 transition-colors"
                                >
                                    <Edit2 size={12} /> 重命名
                                </button>
                                <button
                                    onClick={() => handleAction('delete', w)}
                                    className="ml-auto flex items-center gap-1 rounded-lg px-2 py-1.5 text-[11px] font-medium text-red-500 hover:bg-red-50 dark:text-red-400 dark:hover:bg-red-500/10 transition-colors"
                                >
                                    <Trash2 size={12} /> 删除
                                </button>
                            </div>
                        </div>
                    ))}
                </div>
            )}
        </PanelShell>
    );
}
