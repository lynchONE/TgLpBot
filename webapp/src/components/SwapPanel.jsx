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
                    className="panel-action-btn"
                    onClick={() => setShowSettings(!showSettings)}
                    style={{ display: 'flex', alignItems: 'center', gap: '6px' }}
                >
                    <Settings size={14} /> 滑点 {slippage}%
                </button>
            }
        >
            <div style={{ maxWidth: '500px', margin: '0 auto' }}>
                {showSettings && (
                    <div style={{ marginBottom: '16px', background: 'rgba(18, 26, 40, 0.4)', padding: '16px', borderRadius: '12px', border: '1px solid rgba(136, 157, 191, 0.18)' }}>
                        <div style={{ display: 'flex', gap: '16px', justifyContent: 'space-between', marginBottom: '12px' }}>
                            <div style={{ flex: 1 }}>
                                <label style={{ display: 'block', fontSize: '12px', color: 'var(--text-muted)', marginBottom: '8px', fontWeight: 600 }}>选择钱包</label>
                                {walletLoading ? (
                                    <div style={{ fontSize: '12px', color: 'var(--text-muted)' }}>加载钱包中...</div>
                                ) : (
                                    <select
                                        value={selectedWalletId}
                                        onChange={(e) => setSelectedWalletId(e.target.value)}
                                        style={{ width: '100%', padding: '10px 12px', borderRadius: '8px', background: 'rgba(9, 14, 22, 0.6)', border: '1px solid rgba(136, 157, 191, 0.2)', color: 'var(--text)', outline: 'none' }}
                                    >
                                        {wallets.map(w => (
                                            <option key={w.id} value={w.id}>{w.name || '钱包'} ({shortAddress(w.address)}) - {chain==='bsc'?'BNB':'ETH'}: {parseFloat(w.native_balance).toFixed(3)}</option>
                                        ))}
                                    </select>
                                )}
                            </div>
                            <div style={{ flex: '0.7' }}>
                                <label style={{ display: 'block', fontSize: '12px', color: 'var(--text-muted)', marginBottom: '8px', fontWeight: 600 }}>自定义滑点 (%)</label>
                                <input
                                    type="number"
                                    value={slippage}
                                    onChange={(e) => setSlippage(e.target.value)}
                                    style={{ width: '100%', padding: '10px 12px', borderRadius: '8px', background: 'rgba(9, 14, 22, 0.6)', border: '1px solid rgba(136, 157, 191, 0.2)', color: 'var(--text)', outline: 'none' }}
                                    placeholder="默认 1.0"
                                />
                            </div>
                        </div>
                    </div>
                )}

                <div style={{ position: 'relative', background: 'rgba(18, 26, 40, 0.3)', padding: '4px', borderRadius: '20px', border: '1px solid rgba(136, 157, 191, 0.15)' }}>
                    <div style={{ background: 'rgba(9, 14, 22, 0.7)', padding: '16px', borderRadius: '16px', minHeight: '100px' }}>
                        <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: '12px', color: 'var(--text-muted)', marginBottom: '12px', fontWeight: 600 }}>
                            <span>支付</span>
                        </div>
                        <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
                            <input
                                type="text"
                                value={amount}
                                onChange={(e) => setAmount(e.target.value)}
                                style={{ width: '40%', background: 'transparent', fontSize: '24px', fontWeight: 'bold', color: 'var(--text)', outline: 'none', border: 'none' }}
                                placeholder="0.0"
                            />
                            <div style={{ flex: 1 }}>
                                <input
                                    type="text"
                                    value={fromToken}
                                    onChange={(e) => setFromToken(e.target.value)}
                                    style={{ width: '100%', padding: '12px', borderRadius: '12px', background: 'rgba(18, 26, 40, 0.8)', border: '1px solid rgba(136, 157, 191, 0.2)', color: 'var(--text)', outline: 'none', fontFamily: 'monospace', fontSize: '12px' }}
                                    placeholder="输入代币合约地址..."
                                />
                            </div>
                        </div>
                    </div>

                    <div style={{ position: 'absolute', left: '50%', top: '50%', transform: 'translate(-50%, -50%)', zIndex: 10 }}>
                        <button
                            onClick={handleReverse}
                            style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', width: '40px', height: '40px', borderRadius: '12px', background: 'var(--bg-card)', border: '4px solid var(--bg-body)', color: 'var(--text-muted)', cursor: 'pointer', transition: 'color 0.2s' }}
                        >
                            <ArrowDown size={18} strokeWidth={3} />
                        </button>
                    </div>

                    <div style={{ background: 'rgba(9, 14, 22, 0.7)', padding: '16px', borderRadius: '16px', marginTop: '4px', minHeight: '100px' }}>
                        <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: '12px', color: 'var(--text-muted)', marginBottom: '12px', fontWeight: 600 }}>
                            <span>获得</span>
                        </div>
                        <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
                            <div style={{ width: '40%', fontSize: '24px', fontWeight: 'bold', color: 'var(--text)', whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>
                                {quoting ? (
                                    <span style={{ opacity: 0.5 }}>...</span>
                                ) : (
                                    quoteInfo?.to_amount_float || '0.0'
                                )}
                            </div>
                            <div style={{ flex: 1 }}>
                                <input
                                    type="text"
                                    value={toToken}
                                    onChange={(e) => setToToken(e.target.value)}
                                    style={{ width: '100%', padding: '12px', borderRadius: '12px', background: 'rgba(18, 26, 40, 0.8)', border: '1px solid rgba(136, 157, 191, 0.2)', color: 'var(--text)', outline: 'none', fontFamily: 'monospace', fontSize: '12px' }}
                                    placeholder="输入接收代币合约地址..."
                                />
                            </div>
                        </div>
                    </div>
                </div>

                <div style={{ marginTop: '16px' }}>
                    {quoteError && (
                        <div className="panel-error">
                            <strong>报价失败:</strong> {quoteError}
                        </div>
                    )}
                    {execError && (
                        <div className="panel-error">
                            <strong>兑换失败:</strong> {execError}
                        </div>
                    )}
                    {execSuccess && (
                        <div className="panel-success" style={{ wordBreak: 'break-all' }}>
                            ✅ <strong>兑换请求已提交</strong><br/>
                            TxHash: <span style={{ opacity: 0.8, fontFamily: 'monospace' }}>{execSuccess}</span>
                        </div>
                    )}
                </div>

                {showConfirm ? (
                    <div style={{ marginTop: '16px', padding: '16px', borderRadius: '12px', background: 'rgba(59, 130, 246, 0.1)', border: '1px solid rgba(59, 130, 246, 0.3)', fontSize: '14px' }}>
                        <p style={{ marginBottom: '16px', color: 'var(--text)' }}>
                            确认将支付 <strong>{amount}</strong> 个代币 <br/>
                            兑换为获得约 <strong>{quoteInfo?.to_amount_float || 0}</strong> 个目标代币？<br/>
                            <span style={{ fontSize: '12px', color: 'var(--text-muted)' }}>滑点: {slippage}%</span>
                        </p>
                        <div style={{ display: 'flex', gap: '12px' }}>
                            <button className="panel-action-btn" style={{ flex: 1 }} onClick={() => setShowConfirm(false)}>取消</button>
                            <button className="config-save-btn" style={{ flex: 1 }} onClick={handleSwap} disabled={executing}>
                                {executing ? '执行中...' : '提交交易'}
                            </button>
                        </div>
                    </div>
                ) : (
                    <div style={{ marginTop: '16px' }}>
                        <button
                            type="button"
                            onClick={() => setShowConfirm(true)}
                            disabled={!isReadyToSwap}
                            className={!isReadyToSwap ? 'panel-action-btn' : 'config-save-btn'}
                            style={{ width: '100%', padding: '16px', fontSize: '16px', borderRadius: '12px', fontWeight: 'bold' }}
                        >
                            {executing ? '执行中...' : !fromToken ? '需填入支付合约' : !amount ? '需输入支付数量' : quoting ? '获取最优报价中...' : !quoteInfo ? '无法兑换' : '确认兑换'}
                        </button>
                    </div>
                )}
            </div>
        </PanelShell>
    );
}
