import { useCallback, useEffect, useState } from 'react';
import { fetchSystemConfig, updateSystemConfig } from '../lib/api';
import { getBrandTheme } from '../lib/brand';

function toDraftValue(value) {
    if (!Number.isFinite(Number(value)) || Number(value) <= 0) return '';
    return String(value);
}

export default function SystemConfigCard({ apiBaseUrl, initData, accentTheme = 'lime', onNotice }) {
    const brand = getBrandTheme(accentTheme);
    const inputClass = `w-full rounded-lg border border-zinc-200 bg-white px-3 py-2 text-sm text-zinc-900 focus:outline-none focus:ring-1 ${brand.inputFocusClass} ${brand.key === 'emerald' ? 'focus:ring-emerald-500' : 'focus:ring-[#bcff2f]'} dark:border-white/10 dark:bg-white/5 dark:text-white/90`;
    const [config, setConfig] = useState(null);
    const [defaults, setDefaults] = useState(null);
    const [sizingDefaults, setSizingDefaults] = useState(null);
    const [draft, setDraft] = useState({
        zap_price_deviation_max_percent: '',
        zap_min_pool_liquidity_usd: '',
        open_position_target_share_min: '',
        open_position_target_share_max: '',
        open_position_risk_cap_usd: '',
        open_position_risk_cap_ratio: '',
    });
    const [loading, setLoading] = useState(false);
    const [saving, setSaving] = useState(false);
    const [error, setError] = useState('');

    const loadConfig = useCallback(async () => {
        if (!initData) return;
        setLoading(true);
        setError('');
        try {
            const resp = await fetchSystemConfig({ apiBaseUrl, initData });
            setConfig(resp?.config || null);
            setDefaults(resp?.zap_safety_defaults || null);
            setSizingDefaults(resp?.open_position_sizing_defaults || null);
            setDraft({
                zap_price_deviation_max_percent: toDraftValue(resp?.config?.zap_price_deviation_max_percent),
                zap_min_pool_liquidity_usd: toDraftValue(resp?.config?.zap_min_pool_liquidity_usd),
                open_position_target_share_min: toDraftValue(resp?.config?.open_position_target_share_min),
                open_position_target_share_max: toDraftValue(resp?.config?.open_position_target_share_max),
                open_position_risk_cap_usd: toDraftValue(resp?.config?.open_position_risk_cap_usd),
                open_position_risk_cap_ratio: toDraftValue(resp?.config?.open_position_risk_cap_ratio),
            });
        } catch (e) {
            setError(String(e?.message || e));
        } finally {
            setLoading(false);
        }
    }, [apiBaseUrl, initData]);

    useEffect(() => {
        loadConfig();
    }, [loadConfig]);

    const handleSave = useCallback(async () => {
        if (!initData) return;
        setSaving(true);
        setError('');
        try {
            const parseNumber = (value) => {
                const n = Number(value);
                return Number.isFinite(n) ? n : 0;
            };
            const resp = await updateSystemConfig({
                apiBaseUrl,
                initData,
                config: {
                    zap_price_deviation_max_percent: parseNumber(draft.zap_price_deviation_max_percent),
                    zap_min_pool_liquidity_usd: parseNumber(draft.zap_min_pool_liquidity_usd),
                    open_position_target_share_min: parseNumber(draft.open_position_target_share_min),
                    open_position_target_share_max: parseNumber(draft.open_position_target_share_max),
                    open_position_risk_cap_usd: parseNumber(draft.open_position_risk_cap_usd),
                    open_position_risk_cap_ratio: parseNumber(draft.open_position_risk_cap_ratio),
                },
            });
            setConfig(resp?.config || null);
            setDefaults(resp?.zap_safety_defaults || null);
            setSizingDefaults(resp?.open_position_sizing_defaults || null);
            onNotice?.('系统配置已保存', 'success');
        } catch (e) {
            const message = String(e?.message || e);
            setError(message);
            onNotice?.(message, 'error');
        } finally {
            setSaving(false);
        }
    }, [apiBaseUrl, draft, initData, onNotice]);

    return (
        <div className="rounded-2xl border border-zinc-200/70 bg-white/65 p-4 shadow-sm backdrop-blur-sm dark:border-white/10 dark:bg-[#0f1116]/80 dark:shadow-none">
            <div className="flex items-start justify-between gap-3">
                <div>
                    <div className="text-[10px] font-semibold uppercase tracking-[0.18em] text-zinc-500 dark:text-white/45">SYSTEM CONFIG</div>
                    <div className="mt-0.5 text-[15px] font-black text-zinc-900 dark:text-white">基础配置</div>
                    <div className="mt-1 text-[11px] text-zinc-500 dark:text-white/45">
                        管理 Zap 安全阈值与开仓建议默认参数
                    </div>
                </div>
                <button
                    type="button"
                    onClick={loadConfig}
                    disabled={loading}
                    className={`rounded-xl px-3 py-2 text-xs font-semibold ring-1 transition ${
                        loading
                            ? 'cursor-not-allowed bg-zinc-100 text-zinc-400 ring-zinc-200 dark:bg-white/5 dark:text-white/30 dark:ring-white/10'
                            : 'bg-white/80 text-zinc-700 ring-zinc-200 hover:bg-white dark:bg-white/5 dark:text-white/75 dark:ring-white/10 dark:hover:bg-white/10'
                    }`}
                >
                    刷新
                </button>
            </div>

            {error && (
                <div className="mt-3 rounded-xl border border-red-500/30 bg-red-500/10 px-3 py-2 text-xs text-red-700 dark:text-red-200">
                    {error}
                </div>
            )}

            {loading && !config && (
                <div className="mt-3 rounded-xl border border-zinc-200 bg-white/60 px-3 py-3 text-xs text-zinc-500 dark:border-white/10 dark:bg-white/5 dark:text-white/45">
                    加载中...
                </div>
            )}

            <div className="mt-4 grid gap-3">
                <div>
                    <label className="mb-1 block text-xs font-medium text-zinc-600 dark:text-white/60">
                        最大报价偏差 (%)
                        {defaults && (
                            <span className="ml-1 text-zinc-400">
                                默认 {defaults.price_deviation_max_percent}
                            </span>
                        )}
                    </label>
                    <input
                        type="number"
                        step="0.1"
                        value={draft.zap_price_deviation_max_percent}
                        onChange={(e) => setDraft((prev) => ({ ...prev, zap_price_deviation_max_percent: e.target.value }))}
                        className={inputClass}
                        placeholder={defaults ? String(defaults.price_deviation_max_percent) : '1'}
                    />
                </div>

                <div>
                    <label className="mb-1 block text-xs font-medium text-zinc-600 dark:text-white/60">
                        最低池子流动性 (USD)
                        {defaults && (
                            <span className="ml-1 text-zinc-400">
                                默认 {defaults.min_pool_liquidity_usd}
                            </span>
                        )}
                    </label>
                    <input
                        type="number"
                        step="100"
                        value={draft.zap_min_pool_liquidity_usd}
                        onChange={(e) => setDraft((prev) => ({ ...prev, zap_min_pool_liquidity_usd: e.target.value }))}
                        className={inputClass}
                        placeholder={defaults ? String(defaults.min_pool_liquidity_usd) : '1000'}
                    />
                </div>

                <div>
                    <label className="mb-1 block text-xs font-medium text-zinc-600 dark:text-white/60">
                        开仓建议最小占比
                        {sizingDefaults && (
                            <span className="ml-1 text-zinc-400">
                                默认 {sizingDefaults.target_share_min}
                            </span>
                        )}
                    </label>
                    <input
                        type="number"
                        step="0.01"
                        value={draft.open_position_target_share_min}
                        onChange={(e) => setDraft((prev) => ({ ...prev, open_position_target_share_min: e.target.value }))}
                        className={inputClass}
                        placeholder={sizingDefaults ? String(sizingDefaults.target_share_min) : '0.2'}
                    />
                </div>

                <div>
                    <label className="mb-1 block text-xs font-medium text-zinc-600 dark:text-white/60">
                        开仓建议最大占比
                        {sizingDefaults && (
                            <span className="ml-1 text-zinc-400">
                                默认 {sizingDefaults.target_share_max}
                            </span>
                        )}
                    </label>
                    <input
                        type="number"
                        step="0.01"
                        value={draft.open_position_target_share_max}
                        onChange={(e) => setDraft((prev) => ({ ...prev, open_position_target_share_max: e.target.value }))}
                        className={inputClass}
                        placeholder={sizingDefaults ? String(sizingDefaults.target_share_max) : '0.65'}
                    />
                </div>

                <div>
                    <label className="mb-1 block text-xs font-medium text-zinc-600 dark:text-white/60">
                        开仓固定风险上限 (USD)
                        {sizingDefaults && (
                            <span className="ml-1 text-zinc-400">
                                默认 {sizingDefaults.risk_cap_usd}
                            </span>
                        )}
                    </label>
                    <input
                        type="number"
                        step="10"
                        value={draft.open_position_risk_cap_usd}
                        onChange={(e) => setDraft((prev) => ({ ...prev, open_position_risk_cap_usd: e.target.value }))}
                        className={inputClass}
                        placeholder={sizingDefaults ? String(sizingDefaults.risk_cap_usd) : '500'}
                    />
                </div>

                <div>
                    <label className="mb-1 block text-xs font-medium text-zinc-600 dark:text-white/60">
                        开仓风险比例上限
                        {sizingDefaults && (
                            <span className="ml-1 text-zinc-400">
                                默认 {sizingDefaults.risk_cap_ratio}
                            </span>
                        )}
                    </label>
                    <input
                        type="number"
                        step="0.01"
                        value={draft.open_position_risk_cap_ratio}
                        onChange={(e) => setDraft((prev) => ({ ...prev, open_position_risk_cap_ratio: e.target.value }))}
                        className={inputClass}
                        placeholder={sizingDefaults ? String(sizingDefaults.risk_cap_ratio) : '0.2'}
                    />
                </div>
            </div>

            <div className="mt-4 flex justify-end">
                <button
                    type="button"
                    onClick={handleSave}
                    disabled={saving}
                    className={`rounded-xl px-4 py-2 text-sm font-semibold transition ${
                        saving
                            ? 'cursor-not-allowed bg-zinc-300 text-zinc-500 dark:bg-white/10 dark:text-white/30'
                            : brand.solidButtonClass
                    }`}
                >
                    {saving ? '保存中...' : '保存配置'}
                </button>
            </div>
        </div>
    );
}
