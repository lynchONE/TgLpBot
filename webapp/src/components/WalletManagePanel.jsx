import React, { useCallback, useEffect, useState } from 'react';
import { fetchWallets } from '../api';
import PanelShell from './PanelShell';
import { Wallet } from 'lucide-react';
import { shortAddress } from '../utils';

export default function WalletManagePanel({ apiBaseUrl, initData, hasInitData, chain = 'bsc' }) {
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

    useEffect(() => { load(); }, [load]);

    const copyAddress = async (addr) => {
        try {
            await navigator.clipboard.writeText(addr);
            setCopiedAddr(addr);
            setTimeout(() => setCopiedAddr(''), 2000);
        } catch { }
    };

    return (
        <PanelShell
            title="钱包管理"
            subtitle={`${wallets.length} 个钱包 · ${chain.toUpperCase()}`}
            icon={Wallet}
            actions={<button type="button" className="panel-action-btn" onClick={load} disabled={loading}>{loading ? '刷新中...' : '刷新'}</button>}
        >
            {error && <div className="panel-error">{error}</div>}
            {loading && wallets.length === 0 ? (
                <div className="panel-loading">加载中...</div>
            ) : wallets.length === 0 ? (
                <div className="empty-state">暂无钱包，请在 Telegram 机器人中使用 /wallet 命令导入</div>
            ) : (
                <div className="wallet-list">
                    {wallets.map((w) => (
                        <div key={w.id || w.address} className="wallet-card">
                            <div className="wallet-card-header">
                                <span className="wallet-name">{w.name || `钱包 ${w.id}`}</span>
                                {w.is_default && <span className="wallet-badge">默认</span>}
                            </div>
                            <button type="button" className="wallet-addr" onClick={() => copyAddress(w.address)} title="点击复制">
                                {shortAddress(w.address)}
                                {copiedAddr === w.address && <span className="copy-ok"> ✓</span>}
                            </button>
                            <div className="wallet-balances">
                                <span>{nativeSymbol}: {w.native_balance === 'N/A' ? '-' : parseFloat(w.native_balance).toFixed(4)}</span>
                                <span>{stableSymbol}: {w.stable_balance === 'N/A' ? '-' : parseFloat(w.stable_balance).toFixed(2)}</span>
                            </div>
                        </div>
                    ))}
                </div>
            )}
            <div className="panel-hint">🔒 导入/删除钱包请在 Telegram 机器人的 /wallet 命令中操作</div>
        </PanelShell>
    );
}


