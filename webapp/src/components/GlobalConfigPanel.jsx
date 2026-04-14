import React, { useCallback, useEffect, useState } from 'react';
import { fetchGlobalConfig, saveGlobalConfig } from '../api';
import PanelShell from './PanelShell';
import { Settings } from 'lucide-react';

const CHAIN_OPTIONS = [
    { value: 'bsc', label: 'BSC' },
    { value: 'base', label: 'Base' },
];

function formatRebalanceTimeout(value) {
    const seconds = Number(value);
    if (!Number.isFinite(seconds)) return '--';
    return seconds <= 0 ? 'Immediate' : `${seconds}s`;
}

export default function GlobalConfigPanel({ apiBaseUrl, initData, hasInitData }) {
    const [config, setConfig] = useState(null);
    const [loading, setLoading] = useState(false);
    const [saving, setSaving] = useState(false);
    const [error, setError] = useState('');
    const [success, setSuccess] = useState('');
    const [draft, setDraft] = useState({});

    const loadConfig = useCallback(async () => {
        if (!initData) return;
        setLoading(true);
        setError('');
        try {
            const resp = await fetchGlobalConfig({ apiBaseUrl, initData });
            const cfg = resp?.config || resp || {};
            setConfig(cfg);
            setDraft({
                rebalance_timeout: cfg.rebalance_timeout ?? 10,
                stop_loss_enabled: cfg.stop_loss_enabled ?? false,
                stop_loss_threshold: cfg.stop_loss_threshold ?? 10,
                stop_loss_delay_seconds: cfg.stop_loss_delay_seconds ?? 0,
                slippage_tolerance: cfg.slippage_tolerance ?? 0.5,
                auto_reinvest: cfg.auto_reinvest ?? false,
                residual_tolerance: cfg.residual_tolerance ?? 1.0,
                zap_loss_tolerance: cfg.zap_loss_tolerance ?? 0.5,
                extra_notifications_enabled: cfg.extra_notifications_enabled ?? true,
                filter_chinese_tokens: cfg.filter_chinese_tokens ?? false,
                multi_chain_enabled: cfg.multi_chain_enabled ?? true,
                default_chain: cfg.default_chain || 'bsc',
                multi_wallet_enabled: cfg.multi_wallet_enabled ?? false,
                bark_enabled: cfg.bark_enabled ?? false,
                bark_server: cfg.bark_server || '',
                bark_group: cfg.bark_group || '',
            });
        } catch (e) {
            setError(String(e?.message || e));
        } finally {
            setLoading(false);
        }
    }, [apiBaseUrl, initData]);

    useEffect(() => { loadConfig(); }, [loadConfig]);

    const handleSave = useCallback(async () => {
        if (!initData) return;
        setSaving(true);
        setError('');
        setSuccess('');
        try {
            const resp = await saveGlobalConfig({ apiBaseUrl, initData, config: draft });
            setConfig(resp?.config || resp || {});
            setSuccess('配置已保存');
            setTimeout(() => setSuccess(''), 3000);
        } catch (e) {
            setError(String(e?.message || e));
        } finally {
            setSaving(false);
        }
    }, [apiBaseUrl, initData, draft]);

    const updateDraft = (key, value) => setDraft(prev => ({ ...prev, [key]: value }));

    return (
        <PanelShell title="全局配置" subtitle="管理交易、止损、通知等设置" icon={Settings}>
            {error && <div className="panel-error">{error}</div>}
            {success && <div className="panel-success">{success}</div>}

            {loading && !config ? (
                <div className="panel-loading">加载中...</div>
            ) : config ? (
                <div className="config-grid">
                    <div className="config-section">
                        <h3 className="config-section-title">交易设置</h3>
                        <div className="config-row">
                            <label>再平衡超时 (秒)</label>
                            <input type="number" value={draft.rebalance_timeout} onChange={e => updateDraft('rebalance_timeout', Number(e.target.value) || 0)} />
                            <small>{`-1 means immediate. Current: ${formatRebalanceTimeout(draft.rebalance_timeout)}`}</small>
                        </div>
                        <div className="config-row">
                            <label>滑点容忍 (%)</label>
                            <input type="number" step="0.1" value={draft.slippage_tolerance} onChange={e => updateDraft('slippage_tolerance', Number(e.target.value) || 0)} />
                        </div>
                        <div className="config-row">
                            <label>Zap 损耗容忍 (%)</label>
                            <input type="number" step="0.1" value={draft.zap_loss_tolerance} onChange={e => updateDraft('zap_loss_tolerance', Number(e.target.value) || 0)} />
                        </div>
                    </div>

                    <div className="config-section">
                        <h3 className="config-section-title">止损设置</h3>
                        <div className="config-row config-toggle">
                            <label>止损开关</label>
                            <button type="button" className={`toggle-btn ${draft.stop_loss_enabled ? 'active' : ''}`} onClick={() => updateDraft('stop_loss_enabled', !draft.stop_loss_enabled)}>
                                {draft.stop_loss_enabled ? '开启' : '关闭'}
                            </button>
                        </div>
                        {draft.stop_loss_enabled && (
                            <>
                                <div className="config-row">
                                    <label>阈值 (%)</label>
                                    <input type="number" step="0.1" value={draft.stop_loss_threshold} onChange={e => updateDraft('stop_loss_threshold', Number(e.target.value) || 0)} />
                                </div>
                                <div className="config-row">
                                    <label>延迟 (秒)</label>
                                    <input type="number" value={draft.stop_loss_delay_seconds} onChange={e => updateDraft('stop_loss_delay_seconds', Number(e.target.value) || 0)} />
                                </div>
                            </>
                        )}
                    </div>

                    <div className="config-section">
                        <h3 className="config-section-title">自动功能</h3>
                        <div className="config-row config-toggle">
                            <label>自动复投</label>
                            <button type="button" className={`toggle-btn ${draft.auto_reinvest ? 'active' : ''}`} onClick={() => updateDraft('auto_reinvest', !draft.auto_reinvest)}>
                                {draft.auto_reinvest ? '开启' : '关闭'}
                            </button>
                        </div>
                    </div>

                    <div className="config-section">
                        <h3 className="config-section-title">链与钱包</h3>
                        <div className="config-row config-toggle">
                            <label>多链模式</label>
                            <button type="button" className={`toggle-btn ${draft.multi_chain_enabled ? 'active' : ''}`} onClick={() => updateDraft('multi_chain_enabled', !draft.multi_chain_enabled)}>
                                {draft.multi_chain_enabled ? '开启' : '关闭'}
                            </button>
                        </div>
                        <div className="config-row">
                            <label>默认链</label>
                            <select value={draft.default_chain} onChange={e => updateDraft('default_chain', e.target.value)}>
                                {CHAIN_OPTIONS.map(o => <option key={o.value} value={o.value}>{o.label}</option>)}
                            </select>
                        </div>
                        <div className="config-row config-toggle">
                            <label>多钱包模式</label>
                            <button type="button" className={`toggle-btn ${draft.multi_wallet_enabled ? 'active' : ''}`} onClick={() => updateDraft('multi_wallet_enabled', !draft.multi_wallet_enabled)}>
                                {draft.multi_wallet_enabled ? '开启' : '关闭'}
                            </button>
                        </div>
                    </div>

                    <div className="config-section">
                        <h3 className="config-section-title">通知</h3>
                        <div className="config-row config-toggle">
                            <label>日志通知</label>
                            <button type="button" className={`toggle-btn ${draft.extra_notifications_enabled ? 'active' : ''}`} onClick={() => updateDraft('extra_notifications_enabled', !draft.extra_notifications_enabled)}>
                                {draft.extra_notifications_enabled ? '开启' : '关闭'}
                            </button>
                        </div>
                        <div className="config-row config-toggle">
                            <label>过滤中文代币</label>
                            <button type="button" className={`toggle-btn ${draft.filter_chinese_tokens ? 'active' : ''}`} onClick={() => updateDraft('filter_chinese_tokens', !draft.filter_chinese_tokens)}>
                                {draft.filter_chinese_tokens ? '开启' : '关闭'}
                            </button>
                        </div>
                    </div>

                    <button type="button" className="config-save-btn" disabled={saving} onClick={handleSave}>
                        {saving ? '保存中...' : '保存配置'}
                    </button>
                </div>
            ) : null}
        </PanelShell>
    );
}

