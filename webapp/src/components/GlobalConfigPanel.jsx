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

function parseDCAPercentages(raw) {
    if (Array.isArray(raw)) return raw.map((v) => Number(v) || 0);
    if (typeof raw === 'string' && raw.trim()) {
        try {
            const arr = JSON.parse(raw);
            if (Array.isArray(arr)) return arr.map((v) => Number(v) || 0);
        } catch {
            // ignore
        }
    }
    return [50, 50];
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
                dca_enabled: cfg.dca_enabled ?? false,
                dca_percentages: parseDCAPercentages(cfg.dca_percentages_json ?? cfg.dca_percentages),
                dca_interval_seconds: cfg.dca_interval_seconds ?? 30,
                dca_min_split_amount_usdt: cfg.dca_min_split_amount_usdt ?? 0,
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

                    <DCAConfigSection draft={draft} updateDraft={updateDraft} />

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

function DCAConfigSection({ draft, updateDraft }) {
    const percentages = Array.isArray(draft.dca_percentages) ? draft.dca_percentages : [];
    const sum = percentages.reduce((acc, v) => acc + (Number(v) || 0), 0);
    const sumValid = Math.abs(sum - 100) < 0.01;

    const updatePct = (idx, value) => {
        const next = percentages.slice();
        next[idx] = Number(value) || 0;
        updateDraft('dca_percentages', next);
    };
    const addBatch = () => {
        if (percentages.length >= 5) return;
        const n = percentages.length + 1;
        const base = Math.floor((100 / n) * 100) / 100;
        const next = Array(n).fill(base);
        next[n - 1] = Math.round((100 - base * (n - 1)) * 100) / 100;
        updateDraft('dca_percentages', next);
    };
    const removeBatch = (idx) => {
        if (percentages.length <= 2) return;
        updateDraft('dca_percentages', percentages.filter((_, i) => i !== idx));
    };
    const equalize = () => {
        const n = percentages.length || 2;
        const base = Math.floor((100 / n) * 100) / 100;
        const next = Array(n).fill(base);
        next[n - 1] = Math.round((100 - base * (n - 1)) * 100) / 100;
        updateDraft('dca_percentages', next);
    };

    return (
        <div className="config-section">
            <h3 className="config-section-title">分批加仓（防插针）</h3>
            <div className="config-row config-toggle">
                <label>启用分批加仓</label>
                <button
                    type="button"
                    className={`toggle-btn ${draft.dca_enabled ? 'active' : ''}`}
                    onClick={() => updateDraft('dca_enabled', !draft.dca_enabled)}
                >
                    {draft.dca_enabled ? '开启' : '关闭'}
                </button>
            </div>
            {draft.dca_enabled ? (
                <>
                    <div className="config-row" style={{ flexDirection: 'column', alignItems: 'stretch' }}>
                        <label>每批占比（共 {percentages.length} 批）</label>
                        {percentages.map((value, idx) => (
                            <div key={idx} style={{ display: 'flex', gap: 8, alignItems: 'center', marginTop: 6 }}>
                                <span style={{ minWidth: 52, fontSize: 12, opacity: 0.7 }}>
                                    {idx === 0 ? '首批' : `第 ${idx + 1} 批`}
                                </span>
                                <input
                                    type="number"
                                    step="0.1"
                                    min="5"
                                    max="100"
                                    value={value}
                                    onChange={(e) => updatePct(idx, e.target.value)}
                                    style={{ flex: 1 }}
                                />
                                <span style={{ fontSize: 12, opacity: 0.6 }}>%</span>
                                {percentages.length > 2 ? (
                                    <button
                                        type="button"
                                        className="toggle-btn"
                                        onClick={() => removeBatch(idx)}
                                        style={{ minWidth: 'auto', padding: '4px 8px' }}
                                    >
                                        ×
                                    </button>
                                ) : null}
                            </div>
                        ))}
                        <div style={{ display: 'flex', justifyContent: 'space-between', marginTop: 8, fontSize: 12 }}>
                            <span style={{ color: sumValid ? '#16a34a' : '#d97706' }}>
                                合计：{sum.toFixed(2)}% {sumValid ? '✓' : '（必须等于 100%）'}
                            </span>
                            <span style={{ display: 'flex', gap: 8 }}>
                                <button type="button" className="toggle-btn" onClick={equalize}>
                                    平均分配
                                </button>
                                <button
                                    type="button"
                                    className="toggle-btn"
                                    onClick={addBatch}
                                    disabled={percentages.length >= 5}
                                >
                                    ＋ 追加批次
                                </button>
                            </span>
                        </div>
                    </div>
                    <div className="config-row">
                        <label>批次间隔（0–300 秒）</label>
                        <input
                            type="number"
                            step="0.001"
                            min="0"
                            max="300"
                            value={draft.dca_interval_seconds ?? 30}
                            onChange={(e) => updateDraft('dca_interval_seconds', Number(e.target.value) || 0)}
                        />
                    </div>
                    <div className="config-row">
                        <label>涓嶆媶鍗曢槇鍊?(USDT)</label>
                        <input
                            type="number"
                            step="0.1"
                            min="0"
                            value={draft.dca_min_split_amount_usdt ?? 0}
                            onChange={(e) => updateDraft('dca_min_split_amount_usdt', Number(e.target.value) || 0)}
                        />
                    </div>
                    <small style={{ fontSize: 11, opacity: 0.6 }}>
                        首批按正常开仓创建仓位；后续批次按间隔向该仓位追加流动性。支持 0–300 秒与小数秒，0.3 = 300ms。手动关仓或价格跑出区间时，剩余批次自动取消。
                    </small>
                </>
            ) : (
                <small style={{ fontSize: 11, opacity: 0.6 }}>
                    关闭时一次性开仓成交；开启后按上面设置分批。单次开仓时仍可临时覆盖。
                </small>
            )}
        </div>
    );
}

