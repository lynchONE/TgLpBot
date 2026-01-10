import React, { useState, useEffect, useCallback } from 'react';
import { fetchSystemConfig, updateSystemConfig } from '../lib/api';

/**
 * SystemConfigCard - 管理员系统配置卡片
 * 用于动态配置 AutoLP 硬筛阈值、宽度策略和退出卫士参数
 */
export default function SystemConfigCard({ apiBaseUrl, initData, onNotice }) {
    const [config, setConfig] = useState(null);
    const [defaults, setDefaults] = useState(null);
    const [widthGuardDefaults, setWidthGuardDefaults] = useState(null);
    const [loading, setLoading] = useState(false);
    const [saving, setSaving] = useState(false);
    const [error, setError] = useState('');
    const [expandedSection, setExpandedSection] = useState(null); // 'filter' | 'width' | 'guard' | null

    // 编辑状态
    const [draft, setDraft] = useState({
        // 硬筛阈值
        autolp_min_pool_value_usd: '',
        autolp_min_fee_percentage: '',
        autolp_min_fee_rate_5m: '',
        autolp_min_total_fees_5m: '',
        autolp_min_total_volume_5m: '',
        autolp_min_tx_5m: '',
        // 宽度策略
        autolp_width_sideways_percent: '',
        autolp_width_mild_uptrend_percent: '',
        autolp_width_rapid_pump_percent: '',
        // 退出卫士
        autolp_guard_volume_drop_percent: '',
        autolp_guard_price_tx_drop_percent: '',
        autolp_guard_no_exit_min_fee_rate_5m: '',
        autolp_guard_low_fee_rate_5m: '',
        autolp_guard_volume_drop_percent_low: '',
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
                    // 硬筛阈值
                    autolp_min_pool_value_usd: String(resp.config.autolp_min_pool_value_usd || ''),
                    autolp_min_fee_percentage: String(resp.config.autolp_min_fee_percentage || ''),
                    autolp_min_fee_rate_5m: String(resp.config.autolp_min_fee_rate_5m || ''),
                    autolp_min_total_fees_5m: String(resp.config.autolp_min_total_fees_5m || ''),
                    autolp_min_total_volume_5m: String(resp.config.autolp_min_total_volume_5m || ''),
                    autolp_min_tx_5m: String(resp.config.autolp_min_tx_5m || ''),
                    // 宽度策略
                    autolp_width_sideways_percent: String(resp.config.autolp_width_sideways_percent || ''),
                    autolp_width_mild_uptrend_percent: String(resp.config.autolp_width_mild_uptrend_percent || ''),
                    autolp_width_rapid_pump_percent: String(resp.config.autolp_width_rapid_pump_percent || ''),
                    // 退出卫士
                    autolp_guard_volume_drop_percent: String(resp.config.autolp_guard_volume_drop_percent || ''),
                    autolp_guard_price_tx_drop_percent: String(resp.config.autolp_guard_price_tx_drop_percent || ''),
                    autolp_guard_no_exit_min_fee_rate_5m: String(resp.config.autolp_guard_no_exit_min_fee_rate_5m || ''),
                    autolp_guard_low_fee_rate_5m: String(resp.config.autolp_guard_low_fee_rate_5m || ''),
                    autolp_guard_volume_drop_percent_low: String(resp.config.autolp_guard_volume_drop_percent_low || ''),
                });
            }
            if (resp?.defaults) {
                setDefaults(resp.defaults);
            }
            if (resp?.width_guard_defaults) {
                setWidthGuardDefaults(resp.width_guard_defaults);
            }
        } catch (e) {
            setError(String(e?.message || e));
        } finally {
            setLoading(false);
        }
    }, [apiBaseUrl, initData]);

    useEffect(() => {
        if (expandedSection && !config) {
            loadConfig();
        }
    }, [expandedSection, config, loadConfig]);

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

            // 硬筛阈值
            updates.autolp_min_pool_value_usd = parseFloat(draft.autolp_min_pool_value_usd);
            updates.autolp_min_fee_percentage = parseFloat(draft.autolp_min_fee_percentage);
            updates.autolp_min_fee_rate_5m = parseFloat(draft.autolp_min_fee_rate_5m);
            updates.autolp_min_total_fees_5m = parseFloat(draft.autolp_min_total_fees_5m);
            updates.autolp_min_total_volume_5m = parseFloat(draft.autolp_min_total_volume_5m);
            updates.autolp_min_tx_5m = parseInt(draft.autolp_min_tx_5m);
            // 宽度策略
            updates.autolp_width_sideways_percent = parseFloat(draft.autolp_width_sideways_percent);
            updates.autolp_width_mild_uptrend_percent = parseFloat(draft.autolp_width_mild_uptrend_percent);
            updates.autolp_width_rapid_pump_percent = parseFloat(draft.autolp_width_rapid_pump_percent);
            // 退出卫士
            updates.autolp_guard_volume_drop_percent = parseFloat(draft.autolp_guard_volume_drop_percent);
            updates.autolp_guard_price_tx_drop_percent = parseFloat(draft.autolp_guard_price_tx_drop_percent);
            updates.autolp_guard_no_exit_min_fee_rate_5m = parseFloat(draft.autolp_guard_no_exit_min_fee_rate_5m);
            updates.autolp_guard_low_fee_rate_5m = parseFloat(draft.autolp_guard_low_fee_rate_5m);
            updates.autolp_guard_volume_drop_percent_low = parseFloat(draft.autolp_guard_volume_drop_percent_low);

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
                    autolp_width_sideways_percent: String(resp.config.autolp_width_sideways_percent || ''),
                    autolp_width_mild_uptrend_percent: String(resp.config.autolp_width_mild_uptrend_percent || ''),
                    autolp_width_rapid_pump_percent: String(resp.config.autolp_width_rapid_pump_percent || ''),
                    autolp_guard_volume_drop_percent: String(resp.config.autolp_guard_volume_drop_percent || ''),
                    autolp_guard_price_tx_drop_percent: String(resp.config.autolp_guard_price_tx_drop_percent || ''),
                    autolp_guard_no_exit_min_fee_rate_5m: String(resp.config.autolp_guard_no_exit_min_fee_rate_5m || ''),
                    autolp_guard_low_fee_rate_5m: String(resp.config.autolp_guard_low_fee_rate_5m || ''),
                    autolp_guard_volume_drop_percent_low: String(resp.config.autolp_guard_volume_drop_percent_low || ''),
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

    const toggleSection = (section) => {
        setExpandedSection(prev => prev === section ? null : section);
    };

    const inputClass = `w-full rounded-lg border border-zinc-200 bg-white px-3 py-2 text-sm 
        dark:border-white/10 dark:bg-white/5 dark:text-white/90
        focus:border-emerald-500 focus:outline-none focus:ring-1 focus:ring-emerald-500`;

    const labelClass = `text-xs font-medium text-zinc-600 dark:text-white/60`;

    const sectionButtonClass = (section) => `flex w-full items-center justify-between py-2 ${expandedSection === section ? 'border-b border-zinc-200 dark:border-white/10' : ''}`;

    // 渲染输入字段
    const renderInput = (key, label, defaultValue, step = '0.01', isInt = false) => (
        <div>
            <label className={labelClass}>
                {label}
                {defaultValue !== undefined && <span className="ml-1 text-zinc-400">默认: {formatDefault(defaultValue, isInt)}</span>}
            </label>
            <input
                type="number"
                step={step}
                value={draft[key]}
                onChange={(e) => handleInputChange(key, e.target.value)}
                placeholder={defaultValue !== undefined ? formatDefault(defaultValue, isInt) : '0'}
                className={inputClass}
            />
        </div>
    );

    return (
        <div className="rounded-2xl border border-zinc-200 bg-white p-4 shadow-sm dark:border-white/10 dark:bg-[#111318] dark:shadow-none">
            <div className="text-sm font-semibold text-zinc-900 dark:text-white/90 mb-3">
                系统配置
            </div>

            {loading && (
                <div className="text-xs text-zinc-500 dark:text-white/50 py-2">加载中...</div>
            )}

            {error && (
                <div className="rounded-xl border border-red-500/30 bg-red-500/10 p-3 text-xs text-red-700 dark:text-red-200 mb-3">
                    {error}
                </div>
            )}

            {/* 硬筛阈值配置 */}
            <div className="border-t border-zinc-100 dark:border-white/5">
                <button type="button" onClick={() => toggleSection('filter')} className={sectionButtonClass('filter')}>
                    <span className="text-xs font-medium text-zinc-700 dark:text-white/80">硬筛阈值</span>
                    <span className="text-xs text-zinc-400">{expandedSection === 'filter' ? '收起' : '展开'}</span>
                </button>
                {expandedSection === 'filter' && config && (
                    <div className="py-3 space-y-3">
                        <div className="text-xs text-zinc-500 dark:text-white/50 mb-2">值为 0 时使用环境变量默认值</div>
                        <div className="grid grid-cols-2 gap-3">
                            {renderInput('autolp_min_pool_value_usd', 'TVL 阈值 (USD)', defaults?.min_pool_value_usd)}
                            {renderInput('autolp_min_fee_percentage', '费率阈值 (%)', defaults?.min_fee_percentage)}
                            {renderInput('autolp_min_fee_rate_5m', '5m 费用率 (%)', defaults?.min_fee_rate_5m, '0.0001')}
                            {renderInput('autolp_min_total_fees_5m', '5m 手续费 (USD)', defaults?.min_total_fees_5m)}
                            {renderInput('autolp_min_total_volume_5m', '5m 成交量 (USD)', defaults?.min_total_volume_5m)}
                            {renderInput('autolp_min_tx_5m', '5m 交易笔数', defaults?.min_tx_5m, '1', true)}
                        </div>
                    </div>
                )}
            </div>

            {/* 宽度策略配置 */}
            <div className="border-t border-zinc-100 dark:border-white/5">
                <button type="button" onClick={() => toggleSection('width')} className={sectionButtonClass('width')}>
                    <span className="text-xs font-medium text-zinc-700 dark:text-white/80">宽度策略</span>
                    <span className="text-xs text-zinc-400">{expandedSection === 'width' ? '收起' : '展开'}</span>
                </button>
                {expandedSection === 'width' && config && (
                    <div className="py-3 space-y-3">
                        <div className="text-xs text-zinc-500 dark:text-white/50 mb-2">LP 区间宽度百分比，值为 0 时使用环境变量默认值</div>
                        <div className="grid grid-cols-1 gap-3">
                            {renderInput('autolp_width_sideways_percent', '横盘宽度 (%)', widthGuardDefaults?.width_sideways_percent)}
                            {renderInput('autolp_width_mild_uptrend_percent', '温和上涨宽度 (%)', widthGuardDefaults?.width_mild_uptrend_percent)}
                            {renderInput('autolp_width_rapid_pump_percent', '急涨宽度 (%)', widthGuardDefaults?.width_rapid_pump_percent)}
                        </div>
                    </div>
                )}
            </div>

            {/* 退出卫士配置 */}
            <div className="border-t border-zinc-100 dark:border-white/5">
                <button type="button" onClick={() => toggleSection('guard')} className={sectionButtonClass('guard')}>
                    <span className="text-xs font-medium text-zinc-700 dark:text-white/80">退出卫士</span>
                    <span className="text-xs text-zinc-400">{expandedSection === 'guard' ? '收起' : '展开'}</span>
                </button>
                {expandedSection === 'guard' && config && (
                    <div className="py-3 space-y-3">
                        <div className="text-xs text-zinc-500 dark:text-white/50 mb-2">退出条件阈值（小数表示百分比，如 0.5 = 50%），值为 0 时使用环境变量默认值</div>
                        <div className="grid grid-cols-1 gap-3">
                            {renderInput('autolp_guard_volume_drop_percent', '成交量下降阈值', widthGuardDefaults?.guard_volume_drop_percent, '0.01')}
                            {renderInput('autolp_guard_price_tx_drop_percent', '价格+交易笔数下降阈值', widthGuardDefaults?.guard_price_tx_drop_percent, '0.01')}
                            {renderInput('autolp_guard_no_exit_min_fee_rate_5m', '手续费率保护阈值 (%)', widthGuardDefaults?.guard_no_exit_min_fee_rate_5m, '0.1')}
                            {renderInput('autolp_guard_low_fee_rate_5m', '低手续费率阈值 (%)', widthGuardDefaults?.guard_low_fee_rate_5m, '0.01')}
                            {renderInput('autolp_guard_volume_drop_percent_low', '低费率时成交量下降阈值', widthGuardDefaults?.guard_volume_drop_percent_low, '0.01')}
                        </div>
                    </div>
                )}
            </div>

            {/* 保存按钮 */}
            {config && (
                <div className="flex justify-end pt-4 border-t border-zinc-100 dark:border-white/5 mt-3">
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
            )}
        </div>
    );
}

