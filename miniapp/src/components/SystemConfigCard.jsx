import React, { useState, useEffect, useCallback } from 'react';
import { fetchSystemConfig, updateSystemConfig } from '../lib/api';

/**
 * SystemConfigCard - 管理员系统配置卡片
 * 用于动态配置 AutoLP 硬筛阈值
 */
export default function SystemConfigCard({ apiBaseUrl, initData, onNotice }) {
    const [config, setConfig] = useState(null);
    const [defaults, setDefaults] = useState(null);
    const [loading, setLoading] = useState(false);
    const [saving, setSaving] = useState(false);
    const [error, setError] = useState('');
    const [expanded, setExpanded] = useState(false);

    // 编辑状态
    const [draft, setDraft] = useState({
        autolp_min_pool_value_usd: '',
        autolp_min_fee_percentage: '',
        autolp_min_fee_rate_5m: '',
        autolp_min_total_fees_5m: '',
        autolp_min_total_volume_5m: '',
        autolp_min_tx_5m: '',
    });

    // 加载配置
    const loadConfig = useCallback(async () => {
        if (!initData) return;
        setLoading(true);
        setError('');
        try {
            const resp = await fetchSystemConfig({ apiBaseUrl, initData });
            if (resp?.config) {
                setConfig(resp.config);
                setDraft({
                    autolp_min_pool_value_usd: String(resp.config.autolp_min_pool_value_usd || ''),
                    autolp_min_fee_percentage: String(resp.config.autolp_min_fee_percentage || ''),
                    autolp_min_fee_rate_5m: String(resp.config.autolp_min_fee_rate_5m || ''),
                    autolp_min_total_fees_5m: String(resp.config.autolp_min_total_fees_5m || ''),
                    autolp_min_total_volume_5m: String(resp.config.autolp_min_total_volume_5m || ''),
                    autolp_min_tx_5m: String(resp.config.autolp_min_tx_5m || ''),
                });
            }
            if (resp?.defaults) {
                setDefaults(resp.defaults);
            }
        } catch (e) {
            setError(String(e?.message || e));
        } finally {
            setLoading(false);
        }
    }, [apiBaseUrl, initData]);

    useEffect(() => {
        if (expanded && !config) {
            loadConfig();
        }
    }, [expanded, config, loadConfig]);

    // 保存配置
    const handleSave = async () => {
        if (!initData) return;
        setSaving(true);
        setError('');
        try {
            const updates = {};
            const parseFloat = (v) => {
                const n = Number(v);
                return Number.isFinite(n) ? n : 0;
            };
            const parseInt = (v) => {
                const n = Math.floor(Number(v));
                return Number.isFinite(n) ? n : 0;
            };

            updates.autolp_min_pool_value_usd = parseFloat(draft.autolp_min_pool_value_usd);
            updates.autolp_min_fee_percentage = parseFloat(draft.autolp_min_fee_percentage);
            updates.autolp_min_fee_rate_5m = parseFloat(draft.autolp_min_fee_rate_5m);
            updates.autolp_min_total_fees_5m = parseFloat(draft.autolp_min_total_fees_5m);
            updates.autolp_min_total_volume_5m = parseFloat(draft.autolp_min_total_volume_5m);
            updates.autolp_min_tx_5m = parseInt(draft.autolp_min_tx_5m);

            const resp = await updateSystemConfig({ apiBaseUrl, initData, config: updates });
            if (resp?.config) {
                setConfig(resp.config);
                setDraft({
                    autolp_min_pool_value_usd: String(resp.config.autolp_min_pool_value_usd || ''),
                    autolp_min_fee_percentage: String(resp.config.autolp_min_fee_percentage || ''),
                    autolp_min_fee_rate_5m: String(resp.config.autolp_min_fee_rate_5m || ''),
                    autolp_min_total_fees_5m: String(resp.config.autolp_min_total_fees_5m || ''),
                    autolp_min_total_volume_5m: String(resp.config.autolp_min_total_volume_5m || ''),
                    autolp_min_tx_5m: String(resp.config.autolp_min_tx_5m || ''),
                });
            }
            if (onNotice) onNotice('配置已保存', 'success');
        } catch (e) {
            setError(String(e?.message || e));
            if (onNotice) onNotice(String(e?.message || e), 'error');
        } finally {
            setSaving(false);
        }
    };

    const handleInputChange = (key, value) => {
        setDraft(prev => ({ ...prev, [key]: value }));
    };

    const formatDefault = (value, isInt = false) => {
        if (value === null || value === undefined) return '-';
        if (isInt) return String(Math.floor(value));
        return String(value);
    };

    const inputClass = `w-full rounded-lg border border-zinc-200 bg-white px-3 py-2 text-sm 
        dark:border-white/10 dark:bg-white/5 dark:text-white/90
        focus:border-emerald-500 focus:outline-none focus:ring-1 focus:ring-emerald-500`;

    const labelClass = `text-xs font-medium text-zinc-600 dark:text-white/60`;

    return (
        <div className="rounded-2xl border border-zinc-200 bg-white p-4 shadow-sm dark:border-white/10 dark:bg-[#111318] dark:shadow-none">
            <button
                type="button"
                onClick={() => setExpanded(!expanded)}
                className="flex w-full items-center justify-between"
            >
                <div className="text-sm font-semibold text-zinc-900 dark:text-white/90">
                    硬筛阈值配置
                </div>
                <div className="text-xs text-zinc-500 dark:text-white/40">
                    {expanded ? '收起' : '展开'}
                </div>
            </button>

            {expanded && (
                <div className="mt-4 space-y-4">
                    {loading && (
                        <div className="text-xs text-zinc-500 dark:text-white/50">加载中...</div>
                    )}

                    {error && (
                        <div className="rounded-xl border border-red-500/30 bg-red-500/10 p-3 text-xs text-red-700 dark:text-red-200">
                            {error}
                        </div>
                    )}

                    {config && (
                        <>
                            <div className="text-xs text-zinc-500 dark:text-white/50 mb-2">
                                值为 0 时使用环境变量默认值
                            </div>

                            <div className="grid grid-cols-2 gap-3">
                                <div>
                                    <label className={labelClass}>
                                        TVL 阈值 (USD)
                                        {defaults && <span className="ml-1 text-zinc-400">默认: {formatDefault(defaults.min_pool_value_usd)}</span>}
                                    </label>
                                    <input
                                        type="number"
                                        value={draft.autolp_min_pool_value_usd}
                                        onChange={(e) => handleInputChange('autolp_min_pool_value_usd', e.target.value)}
                                        placeholder={defaults ? formatDefault(defaults.min_pool_value_usd) : '0'}
                                        className={inputClass}
                                    />
                                </div>

                                <div>
                                    <label className={labelClass}>
                                        费率阈值 (%)
                                        {defaults && <span className="ml-1 text-zinc-400">默认: {formatDefault(defaults.min_fee_percentage)}</span>}
                                    </label>
                                    <input
                                        type="number"
                                        step="0.01"
                                        value={draft.autolp_min_fee_percentage}
                                        onChange={(e) => handleInputChange('autolp_min_fee_percentage', e.target.value)}
                                        placeholder={defaults ? formatDefault(defaults.min_fee_percentage) : '0'}
                                        className={inputClass}
                                    />
                                </div>

                                <div>
                                    <label className={labelClass}>
                                        5m 费用率 (%)
                                        {defaults && <span className="ml-1 text-zinc-400">默认: {formatDefault(defaults.min_fee_rate_5m)}</span>}
                                    </label>
                                    <input
                                        type="number"
                                        step="0.0001"
                                        value={draft.autolp_min_fee_rate_5m}
                                        onChange={(e) => handleInputChange('autolp_min_fee_rate_5m', e.target.value)}
                                        placeholder={defaults ? formatDefault(defaults.min_fee_rate_5m) : '0'}
                                        className={inputClass}
                                    />
                                </div>

                                <div>
                                    <label className={labelClass}>
                                        5m 手续费 (USD)
                                        {defaults && <span className="ml-1 text-zinc-400">默认: {formatDefault(defaults.min_total_fees_5m)}</span>}
                                    </label>
                                    <input
                                        type="number"
                                        value={draft.autolp_min_total_fees_5m}
                                        onChange={(e) => handleInputChange('autolp_min_total_fees_5m', e.target.value)}
                                        placeholder={defaults ? formatDefault(defaults.min_total_fees_5m) : '0'}
                                        className={inputClass}
                                    />
                                </div>

                                <div>
                                    <label className={labelClass}>
                                        5m 成交量 (USD)
                                        {defaults && <span className="ml-1 text-zinc-400">默认: {formatDefault(defaults.min_total_volume_5m)}</span>}
                                    </label>
                                    <input
                                        type="number"
                                        value={draft.autolp_min_total_volume_5m}
                                        onChange={(e) => handleInputChange('autolp_min_total_volume_5m', e.target.value)}
                                        placeholder={defaults ? formatDefault(defaults.min_total_volume_5m) : '0'}
                                        className={inputClass}
                                    />
                                </div>

                                <div>
                                    <label className={labelClass}>
                                        5m 交易笔数
                                        {defaults && <span className="ml-1 text-zinc-400">默认: {formatDefault(defaults.min_tx_5m, true)}</span>}
                                    </label>
                                    <input
                                        type="number"
                                        step="1"
                                        value={draft.autolp_min_tx_5m}
                                        onChange={(e) => handleInputChange('autolp_min_tx_5m', e.target.value)}
                                        placeholder={defaults ? formatDefault(defaults.min_tx_5m, true) : '0'}
                                        className={inputClass}
                                    />
                                </div>
                            </div>

                            <div className="flex justify-end pt-2">
                                <button
                                    type="button"
                                    onClick={handleSave}
                                    disabled={saving}
                                    className={`rounded-xl px-4 py-2 text-sm font-semibold transition ${saving
                                        ? 'cursor-not-allowed bg-zinc-300 text-zinc-500 dark:bg-white/10 dark:text-white/30'
                                        : 'bg-emerald-600 text-white hover:bg-emerald-500 dark:bg-emerald-500 dark:hover:bg-emerald-400'
                                        }`}
                                >
                                    {saving ? '保存中...' : '保存配置'}
                                </button>
                            </div>
                        </>
                    )}
                </div>
            )}
        </div>
    );
}
