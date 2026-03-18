import React, { useCallback, useEffect, useState } from 'react';
import { fetchSystemConfig, updateSystemConfig } from '../lib/api';

const inputClass = 'w-full rounded-lg border border-zinc-200 bg-white px-3 py-2 text-sm text-zinc-900 focus:border-emerald-500 focus:outline-none focus:ring-1 focus:ring-emerald-500 dark:border-white/10 dark:bg-white/5 dark:text-white/90';

function toDraftValue(value) {
    if (!Number.isFinite(Number(value)) || Number(value) <= 0) return '';
    return String(value);
}

export default function SystemConfigCard({ apiBaseUrl, initData, onNotice }) {
    const [config, setConfig] = useState(null);
    const [defaults, setDefaults] = useState(null);
    const [draft, setDraft] = useState({
        zap_price_deviation_max_percent: '',
        zap_min_pool_liquidity_usd: '',
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
            setDraft({
                zap_price_deviation_max_percent: toDraftValue(resp?.config?.zap_price_deviation_max_percent),
                zap_min_pool_liquidity_usd: toDraftValue(resp?.config?.zap_min_pool_liquidity_usd),
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
                },
            });
            setConfig(resp?.config || null);
            setDefaults(resp?.zap_safety_defaults || null);
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
        <div className="rounded-2xl border border-zinc-200 bg-white/40 p-4 shadow-sm backdrop-blur-md dark:border-white/10 dark:bg-white/5 dark:shadow-none">
            <div className="flex items-start justify-between gap-3">
                <div>
                    <div className="text-sm font-semibold text-zinc-900 dark:text-white/90">系统配置</div>
                    <div className="mt-1 text-[11px] text-zinc-500 dark:text-white/45">
                        当前仅保留 Zap 安全阈值。
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
            </div>

            <div className="mt-4 flex justify-end">
                <button
                    type="button"
                    onClick={handleSave}
                    disabled={saving}
                    className={`rounded-xl px-4 py-2 text-sm font-semibold transition ${
                        saving
                            ? 'cursor-not-allowed bg-zinc-300 text-zinc-500 dark:bg-white/10 dark:text-white/30'
                            : 'bg-emerald-600 text-white hover:bg-emerald-500 dark:bg-emerald-500 dark:hover:bg-emerald-400'
                    }`}
                >
                    {saving ? '保存中...' : '保存配置'}
                </button>
            </div>
        </div>
    );
}
