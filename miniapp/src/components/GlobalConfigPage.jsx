import React, { useCallback, useEffect, useState } from 'react';
import BottomSheet from './BottomSheet.jsx';
import ToggleSwitch from './ToggleSwitch.jsx';
import CustomSelect from './CustomSelect.jsx';
import { fetchGlobalConfig, saveGlobalConfig } from '../lib/api';
import { getBrandTheme } from '../lib/brand';

const CHAIN_OPTIONS = [
    { value: 'bsc', label: 'BSC', icon: '🟡' },
    { value: 'base', label: 'Base', icon: '🔵' },
];

export default function GlobalConfigPage({ open, onClose, apiBaseUrl, initData, accentTheme = 'lime', onConfigChanged }) {
    const brand = getBrandTheme(accentTheme);
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
                rebalance_timeout: cfg.rebalance_timeout ?? 300,
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

    useEffect(() => {
        if (open) loadConfig();
    }, [open, loadConfig]);

    const handleSave = useCallback(async () => {
        if (!initData) return;
        setSaving(true);
        setError('');
        setSuccess('');
        try {
            const resp = await saveGlobalConfig({ apiBaseUrl, initData, config: draft });
            setConfig(resp?.config || resp || {});
            setSuccess('保存成功');
            onConfigChanged?.(resp?.config || resp);
            setTimeout(() => setSuccess(''), 2000);
        } catch (e) {
            setError(String(e?.message || e));
        } finally {
            setSaving(false);
        }
    }, [apiBaseUrl, initData, draft, onConfigChanged]);

    const updateDraft = (key, value) => {
        setDraft(prev => ({ ...prev, [key]: value }));
    };

    const inputClass = `w-full rounded-xl border border-zinc-200 bg-white/70 px-3 py-2.5 text-sm text-zinc-900 shadow-sm outline-none transition-colors placeholder:text-zinc-400 ${brand.inputFocusClass} dark:border-white/10 dark:bg-white/5 dark:text-white/90 dark:placeholder:text-white/30`;

    return (
        <BottomSheet open={open} onClose={onClose} title="全局配置" maxHeightClass="max-h-[90vh]">
            {error && (
                <div className="mb-4 rounded-xl border border-red-500/30 bg-red-500/10 p-3 text-xs text-red-700 dark:text-red-300">
                    {error}
                </div>
            )}
            {success && (
                <div className="mb-4 rounded-xl border border-emerald-500/30 bg-emerald-500/10 p-3 text-xs text-emerald-700 dark:text-emerald-300">
                    {success}
                </div>
            )}

            {loading && !config ? (
                <div className="flex items-center justify-center py-12 text-sm text-zinc-400 dark:text-white/40">
                    <svg className="mr-2 h-5 w-5 animate-spin" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                        <circle className="opacity-25" cx="12" cy="12" r="10" /><path className="opacity-75" d="M4 12a8 8 0 018-8" />
                    </svg>
                    加载中...
                </div>
            ) : config ? (
                <div className="space-y-5">
                    {/* 交易设置 */}
                    <Section title="交易设置">
                        <FieldRow label="再平衡超时 (秒)">
                            <input
                                type="number"
                                value={draft.rebalance_timeout}
                                onChange={e => updateDraft('rebalance_timeout', Number(e.target.value) || 0)}
                                className={inputClass}
                                placeholder="300"
                            />
                        </FieldRow>
                        <FieldRow label="滑点容忍 (%)">
                            <input
                                type="number"
                                step="0.1"
                                value={draft.slippage_tolerance}
                                onChange={e => updateDraft('slippage_tolerance', Number(e.target.value) || 0)}
                                className={inputClass}
                                placeholder="0.5"
                            />
                        </FieldRow>
                        <FieldRow label="残余容忍 (%)">
                            <input
                                type="number"
                                step="0.1"
                                value={draft.residual_tolerance}
                                onChange={e => updateDraft('residual_tolerance', Number(e.target.value) || 0)}
                                className={inputClass}
                                placeholder="1.0"
                            />
                        </FieldRow>
                        <FieldRow label="Zap 损耗容忍 (%)">
                            <input
                                type="number"
                                step="0.1"
                                value={draft.zap_loss_tolerance}
                                onChange={e => updateDraft('zap_loss_tolerance', Number(e.target.value) || 0)}
                                className={inputClass}
                                placeholder="0.5"
                            />
                        </FieldRow>
                    </Section>

                    {/* 止损设置 */}
                    <Section title="止损设置">
                        <ToggleSwitch
                            label="止损开关"
                            description="超出范围时自动止损"
                            checked={draft.stop_loss_enabled}
                            onChange={v => updateDraft('stop_loss_enabled', v)}
                        />
                        {draft.stop_loss_enabled && (
                            <>
                                <FieldRow label="止损阈值 (范围宽度 %)">
                                    <input
                                        type="number"
                                        step="0.1"
                                        value={draft.stop_loss_threshold}
                                        onChange={e => updateDraft('stop_loss_threshold', Number(e.target.value) || 0)}
                                        className={inputClass}
                                        placeholder="10"
                                    />
                                </FieldRow>
                                <FieldRow label="止损延迟 (秒)">
                                    <input
                                        type="number"
                                        value={draft.stop_loss_delay_seconds}
                                        onChange={e => updateDraft('stop_loss_delay_seconds', Number(e.target.value) || 0)}
                                        className={inputClass}
                                        placeholder="0"
                                    />
                                </FieldRow>
                            </>
                        )}
                    </Section>

                    {/* 自动功能 */}
                    <Section title="自动功能">
                        <ToggleSwitch
                            label="自动复投"
                            description="利润自动再投资"
                            checked={draft.auto_reinvest}
                            onChange={v => updateDraft('auto_reinvest', v)}
                        />
                    </Section>

                    {/* 链与钱包 */}
                    <Section title="链与钱包">
                        <ToggleSwitch
                            label="多链模式"
                            description="启用后可在 BSC 和 Base 之间切换"
                            checked={draft.multi_chain_enabled}
                            onChange={v => updateDraft('multi_chain_enabled', v)}
                        />
                        <FieldRow label="默认链">
                            <CustomSelect
                                value={draft.default_chain}
                                onChange={v => updateDraft('default_chain', v)}
                                options={CHAIN_OPTIONS}
                            />
                        </FieldRow>
                        <ToggleSwitch
                            label="多钱包模式"
                            description="启用后支持多个钱包"
                            checked={draft.multi_wallet_enabled}
                            onChange={v => updateDraft('multi_wallet_enabled', v)}
                        />
                    </Section>

                    {/* 通知设置 */}
                    <Section title="通知设置">
                        <ToggleSwitch
                            label="日志通知"
                            description="接收额外运行日志通知"
                            checked={draft.extra_notifications_enabled}
                            onChange={v => updateDraft('extra_notifications_enabled', v)}
                        />
                        <ToggleSwitch
                            label="Bark 推送"
                            description="通过 Bark 发送 iOS 推送通知"
                            checked={draft.bark_enabled}
                            onChange={v => updateDraft('bark_enabled', v)}
                        />
                        {draft.bark_enabled && (
                            <>
                                <FieldRow label="Bark 服务器">
                                    <input
                                        type="text"
                                        value={draft.bark_server}
                                        onChange={e => updateDraft('bark_server', e.target.value)}
                                        className={inputClass}
                                        placeholder="https://api.day.app"
                                    />
                                </FieldRow>
                                <FieldRow label="Bark 分组">
                                    <input
                                        type="text"
                                        value={draft.bark_group}
                                        onChange={e => updateDraft('bark_group', e.target.value)}
                                        className={inputClass}
                                        placeholder="TgLpBot"
                                    />
                                </FieldRow>
                            </>
                        )}
                    </Section>

                    {/* 过滤 */}
                    <Section title="过滤">
                        <ToggleSwitch
                            label="过滤中文代币"
                            description="隐藏中文名称的代币"
                            checked={draft.filter_chinese_tokens}
                            onChange={v => updateDraft('filter_chinese_tokens', v)}
                        />
                    </Section>

                    {/* Save button */}
                    <div className="sticky bottom-0 bg-white/80 pb-2 pt-3 backdrop-blur-sm dark:bg-[#111318]/80">
                        <button
                            type="button"
                            onClick={handleSave}
                            disabled={saving}
                            className={`w-full rounded-xl px-4 py-3 text-sm font-bold shadow-sm transition-all ${saving ? 'cursor-not-allowed opacity-50' : ''} ${brand.solidButtonClass}`}
                        >
                            {saving ? '保存中...' : '保存配置'}
                        </button>
                    </div>
                </div>
            ) : null}
        </BottomSheet>
    );
}

function Section({ title, children }) {
    return (
        <div className="rounded-2xl border border-zinc-200/50 bg-zinc-50/50 p-4 dark:border-white/[0.06] dark:bg-white/[0.02]">
            <div className="mb-3 text-xs font-bold uppercase tracking-wider text-zinc-400 dark:text-white/30">{title}</div>
            <div className="space-y-3">{children}</div>
        </div>
    );
}

function FieldRow({ label, children }) {
    return (
        <div>
            <div className="mb-1.5 text-xs font-medium text-zinc-500 dark:text-white/50">{label}</div>
            {children}
        </div>
    );
}

