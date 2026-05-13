import React, { useCallback, useEffect, useState } from 'react';
import { fetchWallets, walletCRUD } from '../api';
import PanelShell from './PanelShell';
import { Wallet, Plus, Download, Edit2, Star, Trash2 } from 'lucide-react';
import { shortAddress } from '../utils';
import ConfirmDialog from './ConfirmDialog';

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
    const [deleteTarget, setDeleteTarget] = useState(null);

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
            setWallets([]); 
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
            setError(String(err?.message || err));
        } finally {
            setCrudLoading(false);
        }
    };

    const handleAction = async (action, w) => {
        if (action === 'delete') {
            setDeleteTarget(w);
            return;
        }
        if (action === 'set_default' || action === 'delete') {
            setLoading(true);
            setError('');
            try {
            await walletCRUD({ apiBaseUrl, initData, action, walletId: w.id });
            await load();
        } catch (err) {
                setError(String(err?.message || err));
                setLoading(false);
            }
        } else if (action === 'rename') {
            setCrudAction('rename');
            setCrudForm({ name: w.name || '', privateKey: '', walletId: w.id });
        }
    };

    const confirmDelete = async () => {
        const w = deleteTarget;
        if (!w) return;
        setDeleteTarget(null);
        setLoading(true);
        setError('');
        try {
            await walletCRUD({ apiBaseUrl, initData, action: 'delete', walletId: w.id });
            await load();
        } catch (err) {
            setError(String(err?.message || err));
            setLoading(false);
        }
    };

    const renderCrudForm = () => {
        if (!crudAction) return null;
        const title = crudAction === 'import' ? '导入钱包' : crudAction === 'create' ? '创建钱包' : '重命名钱包';
        return (
            <div style={{ background: 'rgba(18, 26, 40, 0.4)', borderRadius: '12px', padding: '16px', marginBottom: '16px', border: '1px solid rgba(136, 157, 191, 0.18)' }}>
                <h3 style={{ margin: '0 0 12px 0', fontSize: '14px', fontWeight: 'bold', color: 'var(--text)' }}>{title}</h3>
                <form onSubmit={handleCrudSubmit} style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
                    {crudAction === 'import' && (
                        <div>
                            <label style={{ display: 'block', marginBottom: '6px', fontSize: '12px', color: 'var(--text-muted)' }}>私钥 (Hex)</label>
                            <input
                                type="text"
                                value={crudForm.privateKey}
                                onChange={(e) => setCrudForm({ ...crudForm, privateKey: e.target.value })}
                                style={{ width: '100%', padding: '10px 12px', borderRadius: '8px', background: 'rgba(9, 14, 22, 0.6)', border: '1px solid rgba(136, 157, 191, 0.2)', color: 'var(--text)', outline: 'none' }}
                                placeholder="输入私钥..."
                                required
                            />
                        </div>
                    )}
                    <div>
                        <label style={{ display: 'block', marginBottom: '6px', fontSize: '12px', color: 'var(--text-muted)' }}>钱包名称</label>
                        <input
                            type="text"
                            value={crudForm.name}
                            onChange={(e) => setCrudForm({ ...crudForm, name: e.target.value })}
                            style={{ width: '100%', padding: '10px 12px', borderRadius: '8px', background: 'rgba(9, 14, 22, 0.6)', border: '1px solid rgba(136, 157, 191, 0.2)', color: 'var(--text)', outline: 'none' }}
                            placeholder="如: 常用钱包1"
                            required
                        />
                     </div>
                     <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '10px', marginTop: '10px' }}>
                        <button
                            type="button"
                            onClick={() => setCrudAction(null)}
                            className="panel-action-btn"
                        >
                            取消
                        </button>
                        <button
                            type="submit"
                            disabled={crudLoading}
                            className="config-save-btn"
                            style={{ opacity: crudLoading ? 0.5 : 1, cursor: crudLoading ? 'not-allowed' : 'pointer' }}
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
                <div style={{ display: 'flex', gap: '12px', marginBottom: '20px' }}>
                    <button
                        onClick={() => { setCrudAction('create'); setCrudForm({ name: '', privateKey: '', walletId: null }); }}
                        style={{ flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center', gap: '6px', background: 'rgba(18, 26, 40, 0.8)', border: '1px solid rgba(136, 157, 191, 0.2)', padding: '12px', borderRadius: '12px', color: 'var(--text)', fontSize: '14px', fontWeight: 'bold', cursor: 'pointer' }}
                    >
                        <Plus size={16} /> 创建新钱包
                    </button>
                    <button
                        onClick={() => { setCrudAction('import'); setCrudForm({ name: '', privateKey: '', walletId: null }); }}
                        style={{ flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center', gap: '6px', background: 'rgba(18, 26, 40, 0.8)', border: '1px solid rgba(136, 157, 191, 0.2)', padding: '12px', borderRadius: '12px', color: 'var(--text)', fontSize: '14px', fontWeight: 'bold', cursor: 'pointer' }}
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
                            <button type="button" className="wallet-addr" style={{ marginBottom: '12px' }} onClick={() => copyAddress(w.address)} title="点击复制">
                                {shortAddress(w.address)}
                                {copiedAddr === w.address && <span className="copy-ok"> ✓</span>}
                            </button>
                            <div className="wallet-balances" style={{ marginBottom: '16px' }}>
                                <span>{nativeSymbol}: {w.native_balance === 'N/A' ? '-' : parseFloat(w.native_balance).toFixed(4)}</span>
                                <span>{stableSymbol}: {w.stable_balance === 'N/A' ? '-' : parseFloat(w.stable_balance).toFixed(2)}</span>
                            </div>
                            <div style={{ display: 'flex', alignItems: 'center', gap: '8px', borderTop: '1px solid rgba(136, 157, 191, 0.1)', paddingTop: '12px' }}>
                                {!w.is_default && (
                                    <button
                                        onClick={() => handleAction('set_default', w)}
                                        style={{ display: 'flex', alignItems: 'center', gap: '4px', background: 'rgba(18, 26, 40, 0.6)', border: 'none', borderRadius: '8px', padding: '6px 10px', fontSize: '12px', color: 'var(--text-muted)', cursor: 'pointer' }}
                                    >
                                        <Star size={12} /> 设为默认
                                    </button>
                                )}
                                <button
                                    onClick={() => handleAction('rename', w)}
                                    style={{ display: 'flex', alignItems: 'center', gap: '4px', background: 'rgba(18, 26, 40, 0.6)', border: 'none', borderRadius: '8px', padding: '6px 10px', fontSize: '12px', color: 'var(--text-muted)', cursor: 'pointer' }}
                                >
                                    <Edit2 size={12} /> 重命名
                                </button>
                                <button
                                    onClick={() => handleAction('delete', w)}
                                    style={{ marginLeft: 'auto', display: 'flex', alignItems: 'center', gap: '4px', background: 'rgba(239, 68, 68, 0.1)', border: 'none', borderRadius: '8px', padding: '6px 10px', fontSize: '12px', color: '#ef4444', cursor: 'pointer' }}
                                >
                                    <Trash2 size={12} /> 删除
                                </button>
                            </div>
                        </div>
                    ))}
                </div>
            )}
            <ConfirmDialog
                open={Boolean(deleteTarget)}
                title="删除钱包"
                message={`确定要删除钱包 ${deleteTarget?.name || shortAddress(deleteTarget?.address || '')} 吗？`}
                confirmText="删除"
                danger
                loading={loading}
                onConfirm={confirmDelete}
                onCancel={() => setDeleteTarget(null)}
            />
        </PanelShell>
    );
}
