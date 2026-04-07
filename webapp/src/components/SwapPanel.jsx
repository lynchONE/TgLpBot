import React, { useCallback, useState } from 'react';
import { walletSwapPreview, walletSwapExecute } from '../api';
import PanelShell from './PanelShell';
import { RefreshCw } from 'lucide-react';

export default function SwapPanel({ apiBaseUrl, initData, hasInitData, chain = 'bsc' }) {
    const [tokens, setTokens] = useState([]);
    const [scanning, setScanning] = useState(false);
    const [scanError, setScanError] = useState('');
    const [scanned, setScanned] = useState(false);
    const [executing, setExecuting] = useState(false);
    const [result, setResult] = useState(null);
    const [execError, setExecError] = useState('');
    const [showConfirm, setShowConfirm] = useState(false);

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
            setScanned(false);
        } catch (e) {
            setExecError(String(e?.message || e));
            setShowConfirm(false);
        } finally {
            setExecuting(false);
        }
    }, [apiBaseUrl, initData, chain]);

    return (
        <PanelShell title="一键兑换" subtitle="将零散代币兑换为 USDT" icon={RefreshCw}>
            {!scanned && !scanning && (
                <button type="button" className="config-save-btn" onClick={scan}>🔍 扫描可兑换代币</button>
            )}

            {scanError && <div className="panel-error">{scanError}</div>}
            {scanning && <div className="panel-loading">扫描中，请稍候...</div>}

            {scanned && !scanning && tokens.length === 0 && (
                <div className="empty-state">✨ 没有找到可兑换的代币，钱包已经很干净了</div>
            )}

            {scanned && !scanning && tokens.length > 0 && (
                <div className="swap-results">
                    <div className="swap-summary">
                        <span>可兑换代币总价值</span>
                        <strong>≈ ${totalValue.toFixed(2)} USDT</strong>
                        <span className="swap-count">共 {tokens.length} 个代币</span>
                    </div>

                    <div className="swap-token-list">
                        {tokens.map((t, i) => (
                            <div key={i} className="swap-token-row">
                                <div className="swap-token-info">
                                    <span className="swap-token-symbol">{t.symbol || '???'}</span>
                                    <span className="swap-token-balance">{t.balance}</span>
                                </div>
                                <span className="swap-token-value">≈ ${t.value_usdt?.toFixed(2) ?? '0.00'}</span>
                            </div>
                        ))}
                    </div>

                    <div className="swap-actions">
                        <button type="button" className="panel-action-btn" onClick={scan}>重新扫描</button>
                        {!showConfirm ? (
                            <button type="button" className="config-save-btn" onClick={() => setShowConfirm(true)}>🔄 全部兑换</button>
                        ) : (
                            <div className="swap-confirm">
                                <p>确认将 {tokens.length} 个代币（≈ ${totalValue.toFixed(2)}）兑换为 USDT？</p>
                                <div className="swap-confirm-btns">
                                    <button type="button" className="panel-action-btn" onClick={() => setShowConfirm(false)}>取消</button>
                                    <button type="button" className="config-save-btn" onClick={doSwap} disabled={executing}>
                                        {executing ? '执行中...' : '确认兑换'}
                                    </button>
                                </div>
                            </div>
                        )}
                    </div>
                </div>
            )}

            {execError && <div className="panel-error">兑换失败: {execError}</div>}
            {result?.swapped?.length > 0 && (
                <div className="panel-success">
                    ✅ 兑换成功: {result.swapped.map((s, i) => <div key={i}>{s}</div>)}
                </div>
            )}
            {result?.failed?.length > 0 && (
                <div className="panel-error">
                    ❌ 部分失败: {result.failed.map((f, i) => <div key={i}>{f}</div>)}
                </div>
            )}
        </PanelShell>
    );
}


