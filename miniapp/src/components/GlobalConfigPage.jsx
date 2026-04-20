import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { AlertTriangle, CheckCircle, Filter, Layers, Plus, Save, Settings2, Shield, Sparkles, Trash2, Wallet } from 'lucide-react';
import BottomSheet from './BottomSheet.jsx';
import ToggleSwitch from './ToggleSwitch.jsx';
import CustomSelect from './CustomSelect.jsx';
import { fetchGlobalConfig, saveGlobalConfig } from '../lib/api';
import { getBrandTheme } from '../lib/brand';

const CHAIN_OPTIONS = [
    { value: 'bsc', label: 'BSC' },
    { value: 'base', label: 'Base' },
];

function parseDCAPercentages(raw) {
    if (Array.isArray(raw)) return raw.map((v) => Number(v) || 0);
    if (typeof raw === 'string' && raw.trim()) {
        try {
            const arr = JSON.parse(raw);
            if (Array.isArray(arr)) return arr.map((v) => Number(v) || 0);
        } catch {
            // fall through
        }
    }
    return [50, 50];
}

function buildDraft(cfg = {}) {
    return {
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
    };
}

function formatRebalanceTimeout(value) {
    const seconds = Number(value);
    if (!Number.isFinite(seconds)) return '--';
    return seconds <= 0 ? 'Immediate' : `${seconds}s`;
}

function getChainLabel(value) {
    return CHAIN_OPTIONS.find((item) => item.value === value)?.label || '未设置';
}

function countEnabledFeatures(draft) {
    return [
        draft.stop_loss_enabled,
        draft.auto_reinvest,
        draft.extra_notifications_enabled,
        draft.filter_chinese_tokens,
        draft.multi_chain_enabled,
        draft.multi_wallet_enabled,
        draft.bark_enabled,
    ].filter(Boolean).length;
}

export default function GlobalConfigPage({ open, onClose, apiBaseUrl, initData, accentTheme = 'lime', onConfigChanged }) {
    const brand = getBrandTheme(accentTheme);
    const [config, setConfig] = useState(null);
    const [loading, setLoading] = useState(false);
    const [saving, setSaving] = useState(false);
    const [error, setError] = useState('');
    const [success, setSuccess] = useState('');
    const [draft, setDraft] = useState(buildDraft());

    const loadConfig = useCallback(async () => {
        if (!initData) return;
        setLoading(true);
        setError('');
        try {
            const resp = await fetchGlobalConfig({ apiBaseUrl, initData });
            const cfg = resp?.config || resp || {};
            const nextDraft = buildDraft(cfg);
            setConfig(cfg);
            setDraft(nextDraft);
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
            const nextConfig = resp?.config || resp || {};
            setConfig(nextConfig);
            setDraft(buildDraft(nextConfig));
            setSuccess('配置已保存');
            onConfigChanged?.(nextConfig);
            setTimeout(() => setSuccess(''), 2000);
        } catch (e) {
            setError(String(e?.message || e));
        } finally {
            setSaving(false);
        }
    }, [apiBaseUrl, initData, draft, onConfigChanged]);

    const updateDraft = (key, value) => {
        setDraft((prev) => ({ ...prev, [key]: value }));
    };

    const pristineDraft = buildDraft(config || {});
    const hasChanges = JSON.stringify(pristineDraft) !== JSON.stringify(draft);
    const enabledFeatureCount = countEnabledFeatures(draft);
    const notificationMode = draft.bark_enabled
        ? '应用内 + Bark'
        : (draft.extra_notifications_enabled ? '应用内通知' : '静默');

    const inputClass = `w-full rounded-2xl border border-zinc-200/80 bg-white/80 px-4 py-3 text-sm text-zinc-900 shadow-sm outline-none transition placeholder:text-zinc-400 ${brand.inputFocusClass} dark:border-white/10 dark:bg-white/[0.04] dark:text-white/90 dark:placeholder:text-white/25`;

    return (
        <BottomSheet
            open={open}
            onClose={onClose}
            title="全局配置"
            maxHeightClass="max-h-[92vh]"
            contentClassName="px-5 pb-0 sm:pb-0"
        >
            {error ? (
                <NoticeBanner tone="error" icon={AlertTriangle}>
                    {error}
                </NoticeBanner>
            ) : null}
            {success ? (
                <NoticeBanner tone="success" icon={CheckCircle}>
                    {success}
                </NoticeBanner>
            ) : null}

            {loading && !config ? (
                <div className="flex items-center justify-center py-16 text-sm text-zinc-500 dark:text-white/45">
                    <svg className="mr-2 h-5 w-5 animate-spin" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                        <circle className="opacity-25" cx="12" cy="12" r="10" />
                        <path className="opacity-75" d="M4 12a8 8 0 018-8" />
                    </svg>
                    正在加载配置...
                </div>
            ) : config ? (
                <div className="space-y-4 pb-4">
                    <section className="overflow-hidden rounded-[30px] border border-zinc-200/70 bg-[radial-gradient(circle_at_top_left,rgba(255,255,255,0.96),rgba(247,248,250,0.88))] p-5 shadow-[0_18px_40px_rgba(15,23,42,0.08)] dark:border-white/[0.08] dark:bg-[radial-gradient(circle_at_top_left,rgba(255,255,255,0.06),rgba(255,255,255,0.02))] dark:shadow-none">
                        <div className="flex items-start gap-3">
                            <div className={`inline-flex h-11 w-11 shrink-0 items-center justify-center rounded-2xl ${brand.softButtonClass}`}>
                                <Settings2 className="h-5 w-5" />
                            </div>
                            <div className="min-w-0 flex-1">
                                <div className="text-[11px] font-semibold uppercase tracking-[0.24em] text-zinc-500 dark:text-white/35">
                                    全局策略
                                </div>
                                <div className="mt-1 text-xl font-semibold text-zinc-900 dark:text-white/92">
                                    把重要开关做得更清楚
                                </div>
                                <p className="mt-2 text-sm leading-6 text-zinc-600 dark:text-white/55">
                                    交易容忍度、止损、通知与链路切换都集中在这里。开关会直接显示状态，避免再出现“空按钮”。
                                </p>
                            </div>
                        </div>

                        <div className="mt-4 grid grid-cols-1 gap-2.5 sm:grid-cols-3">
                            <SummaryStat
                                label="已启用项"
                                value={`${enabledFeatureCount} 项`}
                                toneClass={brand.softButtonClass}
                            />
                            <SummaryStat
                                label="默认网络"
                                value={getChainLabel(draft.default_chain)}
                            />
                            <SummaryStat
                                label="通知模式"
                                value={notificationMode}
                            />
                        </div>
                    </section>

                    <Section
                        icon={Shield}
                        iconClassName={brand.iconChipClass}
                        title="交易与保护"
                        description="控制调仓时间、滑点与残余容忍范围。"
                    >
                        <div className="grid gap-3 sm:grid-cols-2">
                            <FieldCard label="再平衡超时" hint="任务执行超过阈值后停止">
                                <InputWithSuffix
                                    type="number"
                                    value={draft.rebalance_timeout}
                                    onChange={(e) => updateDraft('rebalance_timeout', Number(e.target.value) || 0)}
                                    className={inputClass}
                                    placeholder="-1 / 10"
                                    suffix="秒"
                                />
                                <div className="text-[11px] text-zinc-500 dark:text-white/45">{`-1 means immediate. Current: ${formatRebalanceTimeout(draft.rebalance_timeout)}`}</div>
                            </FieldCard>
                            <FieldCard label="滑点容忍" hint="开仓、补仓时允许的价格偏差">
                                <InputWithSuffix
                                    type="number"
                                    step="0.1"
                                    value={draft.slippage_tolerance}
                                    onChange={(e) => updateDraft('slippage_tolerance', Number(e.target.value) || 0)}
                                    className={inputClass}
                                    placeholder="0.5"
                                    suffix="%"
                                />
                            </FieldCard>
                            <FieldCard label="残余容忍" hint="允许未完全成交或残余资产的比例">
                                <InputWithSuffix
                                    type="number"
                                    step="0.1"
                                    value={draft.residual_tolerance}
                                    onChange={(e) => updateDraft('residual_tolerance', Number(e.target.value) || 0)}
                                    className={inputClass}
                                    placeholder="1.0"
                                    suffix="%"
                                />
                            </FieldCard>
                            <FieldCard label="Zap 损耗容忍" hint="使用 Zap 路径时接受的损耗上限">
                                <InputWithSuffix
                                    type="number"
                                    step="0.1"
                                    value={draft.zap_loss_tolerance}
                                    onChange={(e) => updateDraft('zap_loss_tolerance', Number(e.target.value) || 0)}
                                    className={inputClass}
                                    placeholder="0.5"
                                    suffix="%"
                                />
                            </FieldCard>
                        </div>
                    </Section>

                    <Section
                        icon={AlertTriangle}
                        iconClassName="bg-amber-500/10 text-amber-700 ring-1 ring-amber-500/20 dark:bg-amber-500/15 dark:text-amber-200 dark:ring-amber-500/25"
                        title="止损策略"
                        description="在价格偏离范围后自动退出，减少拖延执行。"
                    >
                        <ToggleSwitch
                            label="启用止损"
                            description="超出价格区间后，按规则执行止损。"
                            checked={draft.stop_loss_enabled}
                            onChange={(value) => updateDraft('stop_loss_enabled', value)}
                        />
                        {draft.stop_loss_enabled ? (
                            <div className="grid gap-3 sm:grid-cols-2">
                                <FieldCard label="止损阈值" hint="按区间宽度计算的触发比例">
                                    <InputWithSuffix
                                        type="number"
                                        step="0.1"
                                        value={draft.stop_loss_threshold}
                                        onChange={(e) => updateDraft('stop_loss_threshold', Number(e.target.value) || 0)}
                                        className={inputClass}
                                        placeholder="10"
                                        suffix="%"
                                    />
                                </FieldCard>
                                <FieldCard label="止损延迟" hint="触发后等待多久再执行">
                                    <InputWithSuffix
                                        type="number"
                                        value={draft.stop_loss_delay_seconds}
                                        onChange={(e) => updateDraft('stop_loss_delay_seconds', Number(e.target.value) || 0)}
                                        className={inputClass}
                                        placeholder="0"
                                        suffix="秒"
                                    />
                                </FieldCard>
                            </div>
                        ) : (
                            <MutedHint>
                                当前未启用止损，开启后会出现阈值和延迟设置。
                            </MutedHint>
                        )}
                    </Section>

                    <Section
                        icon={Sparkles}
                        iconClassName="bg-violet-500/10 text-violet-700 ring-1 ring-violet-500/20 dark:bg-violet-500/15 dark:text-violet-200 dark:ring-violet-500/25"
                        title="自动化"
                        description="用于控制收益再投入等自动行为。"
                    >
                        <ToggleSwitch
                            label="自动复投"
                            description="把已实现利润自动投入下一次操作。"
                            checked={draft.auto_reinvest}
                            onChange={(value) => updateDraft('auto_reinvest', value)}
                        />
                    </Section>

                    <DCASection
                        draft={draft}
                        updateDraft={updateDraft}
                        inputClass={inputClass}
                        brand={brand}
                    />

                    <Section
                        icon={Wallet}
                        iconClassName="bg-sky-500/10 text-sky-700 ring-1 ring-sky-500/20 dark:bg-sky-500/15 dark:text-sky-200 dark:ring-sky-500/25"
                        title="链路与钱包"
                        description="决定是否启用多链、多钱包，以及默认工作网络。"
                    >
                        <ToggleSwitch
                            label="多链模式"
                            description="允许在 BSC 和 Base 之间切换。"
                            checked={draft.multi_chain_enabled}
                            onChange={(value) => updateDraft('multi_chain_enabled', value)}
                        />
                        <FieldCard label="默认网络" hint="新建操作时默认使用的链">
                            <CustomSelect
                                value={draft.default_chain}
                                onChange={(value) => updateDraft('default_chain', value)}
                                options={CHAIN_OPTIONS}
                            />
                        </FieldCard>
                        <ToggleSwitch
                            label="多钱包模式"
                            description="允许同一账户管理多个钱包地址。"
                            checked={draft.multi_wallet_enabled}
                            onChange={(value) => updateDraft('multi_wallet_enabled', value)}
                        />
                    </Section>

                    <Section
                        icon={CheckCircle}
                        iconClassName="bg-emerald-500/10 text-emerald-700 ring-1 ring-emerald-500/20 dark:bg-emerald-500/15 dark:text-emerald-200 dark:ring-emerald-500/25"
                        title="通知设置"
                        description="应用内运行日志和 Bark 推送都在这里配置。"
                    >
                        <ToggleSwitch
                            label="日志通知"
                            description="接收额外的策略运行与执行日志通知。"
                            checked={draft.extra_notifications_enabled}
                            onChange={(value) => updateDraft('extra_notifications_enabled', value)}
                        />
                        <ToggleSwitch
                            label="Bark 推送"
                            description="通过 Bark 发送 iOS 推送。"
                            checked={draft.bark_enabled}
                            onChange={(value) => updateDraft('bark_enabled', value)}
                        />
                        {draft.bark_enabled ? (
                            <div className="grid gap-3 sm:grid-cols-2">
                                <FieldCard label="Bark 服务地址" hint="例如官方服务或你自己的 Bark 服务">
                                    <input
                                        type="text"
                                        value={draft.bark_server}
                                        onChange={(e) => updateDraft('bark_server', e.target.value)}
                                        className={inputClass}
                                        placeholder="https://api.day.app"
                                    />
                                </FieldCard>
                                <FieldCard label="Bark 分组" hint="用于在通知中心中归类消息">
                                    <input
                                        type="text"
                                        value={draft.bark_group}
                                        onChange={(e) => updateDraft('bark_group', e.target.value)}
                                        className={inputClass}
                                        placeholder="TgLpBot"
                                    />
                                </FieldCard>
                            </div>
                        ) : (
                            <MutedHint>
                                如果你只需要应用内通知，可以保持 Bark 关闭。
                            </MutedHint>
                        )}
                    </Section>

                    <Section
                        icon={Filter}
                        iconClassName="bg-zinc-900 text-white ring-1 ring-black/5 dark:bg-white/10 dark:text-white/85 dark:ring-white/10"
                        title="过滤规则"
                        description="控制一些默认过滤项，让列表更干净。"
                    >
                        <ToggleSwitch
                            label="过滤中文代币"
                            description="隐藏名称中包含中文的代币。"
                            checked={draft.filter_chinese_tokens}
                            onChange={(value) => updateDraft('filter_chinese_tokens', value)}
                        />
                    </Section>

                    <div className="sticky bottom-0 -mx-5 border-t border-zinc-200/70 bg-white/88 px-5 pb-[calc(env(safe-area-inset-bottom,0px)+12px)] pt-3 backdrop-blur-xl dark:border-white/[0.08] dark:bg-[#111318]/90">
                        <div className="mb-3 flex items-center justify-between gap-3">
                            <div className="min-w-0">
                                <div className="text-sm font-semibold text-zinc-900 dark:text-white/90">
                                    {hasChanges ? '有未保存修改' : '当前配置已同步'}
                                </div>
                                <div className="mt-0.5 text-[11px] text-zinc-500 dark:text-white/45">
                                    {hasChanges
                                        ? '保存后会立即写入当前账户配置。'
                                        : '继续调整参数后，底部按钮会自动进入可保存状态。'}
                                </div>
                            </div>
                            <div className={`shrink-0 rounded-full px-3 py-1 text-[11px] font-semibold ${hasChanges ? brand.softButtonClass : 'bg-zinc-100 text-zinc-500 dark:bg-white/[0.06] dark:text-white/40'}`}>
                                {hasChanges ? '待保存' : '最新'}
                            </div>
                        </div>

                        <button
                            type="button"
                            onClick={handleSave}
                            disabled={saving || !hasChanges}
                            className={`flex w-full items-center justify-center gap-2 rounded-2xl px-4 py-3.5 text-sm font-bold shadow-sm transition-all ${
                                saving || !hasChanges
                                    ? 'cursor-not-allowed bg-zinc-200 text-zinc-500 shadow-none dark:bg-white/[0.08] dark:text-white/35'
                                    : brand.gradientButtonClass
                            }`}
                        >
                            <Save className="h-4 w-4" />
                            {saving ? '保存中...' : hasChanges ? '保存配置' : '当前已是最新'}
                        </button>
                    </div>
                </div>
            ) : null}
        </BottomSheet>
    );
}

function NoticeBanner({ tone = 'success', icon: Icon, children }) {
    const toneClass = tone === 'error'
        ? 'border-red-500/25 bg-red-500/10 text-red-700 dark:text-red-300'
        : 'border-emerald-500/25 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300';

    return (
        <div className={`mb-4 flex items-start gap-2 rounded-2xl border px-3 py-3 text-xs ${toneClass}`}>
            <Icon className="mt-0.5 h-4 w-4 shrink-0" />
            <div className="min-w-0 leading-5">{children}</div>
        </div>
    );
}

function SummaryStat({ label, value, toneClass = '' }) {
    return (
        <div className={`rounded-2xl border border-zinc-200/70 bg-white/75 px-3 py-3 dark:border-white/[0.08] dark:bg-white/[0.04] ${toneClass || ''}`}>
            <div className="text-[11px] font-medium text-zinc-500 dark:text-white/45">{label}</div>
            <div className="mt-1 truncate text-sm font-semibold text-zinc-900 dark:text-white/92">{value}</div>
        </div>
    );
}

function Section({ icon: Icon, iconClassName, title, description, children }) {
    return (
        <section className="rounded-[28px] border border-zinc-200/60 bg-white/80 p-4 shadow-[0_10px_30px_rgba(15,23,42,0.05)] dark:border-white/[0.08] dark:bg-white/[0.03] dark:shadow-none">
            <div className="mb-4 flex items-start gap-3">
                <div className={`inline-flex h-10 w-10 shrink-0 items-center justify-center rounded-2xl ${iconClassName}`}>
                    <Icon className="h-4.5 w-4.5" />
                </div>
                <div className="min-w-0 flex-1">
                    <div className="text-sm font-semibold text-zinc-900 dark:text-white/92">{title}</div>
                    <div className="mt-1 text-xs leading-5 text-zinc-500 dark:text-white/45">{description}</div>
                </div>
            </div>
            <div className="space-y-3">{children}</div>
        </section>
    );
}

function FieldCard({ label, hint, children }) {
    return (
        <div className="rounded-2xl border border-zinc-200/70 bg-white/75 p-3 shadow-sm dark:border-white/[0.06] dark:bg-black/20">
            <div className="mb-2">
                <div className="text-sm font-semibold text-zinc-800 dark:text-white/88">{label}</div>
                {hint ? (
                    <div className="mt-1 text-[11px] leading-5 text-zinc-500 dark:text-white/45">{hint}</div>
                ) : null}
            </div>
            {children}
        </div>
    );
}

function InputWithSuffix({ className, suffix, ...props }) {
    return (
        <div className="relative">
            <input {...props} className={`${className} ${suffix ? 'pr-11' : ''}`} />
            {suffix ? (
                <span className="pointer-events-none absolute inset-y-0 right-4 flex items-center text-xs font-semibold text-zinc-400 dark:text-white/35">
                    {suffix}
                </span>
            ) : null}
        </div>
    );
}

function MutedHint({ children }) {
    return (
        <div className="rounded-2xl border border-dashed border-zinc-200 bg-zinc-50/80 px-3 py-3 text-xs leading-5 text-zinc-500 dark:border-white/[0.08] dark:bg-white/[0.02] dark:text-white/45">
            {children}
        </div>
    );
}

function DCASection({ draft, updateDraft, inputClass, brand }) {
    const percentages = Array.isArray(draft.dca_percentages) ? draft.dca_percentages : [];
    const sum = useMemo(
        () => percentages.reduce((acc, v) => acc + (Number(v) || 0), 0),
        [percentages],
    );
    const sumValid = Math.abs(sum - 100) < 0.01;

    const updatePct = (idx, value) => {
        const next = percentages.slice();
        next[idx] = Number(value) || 0;
        updateDraft('dca_percentages', next);
    };
    const addBatch = () => {
        if (percentages.length >= 5) return;
        const even = Math.round((100 / (percentages.length + 1)) * 100) / 100;
        const next = Array(percentages.length + 1).fill(even);
        const diff = 100 - even * next.length;
        next[next.length - 1] = Math.round((even + diff) * 100) / 100;
        updateDraft('dca_percentages', next);
    };
    const removeBatch = (idx) => {
        if (percentages.length <= 2) return;
        const next = percentages.filter((_, i) => i !== idx);
        updateDraft('dca_percentages', next);
    };
    const equalize = () => {
        const n = percentages.length || 2;
        const base = Math.floor((100 / n) * 100) / 100;
        const next = Array(n).fill(base);
        next[next.length - 1] = Math.round((100 - base * (n - 1)) * 100) / 100;
        updateDraft('dca_percentages', next);
    };

    const pillBtnClass = `inline-flex items-center gap-1 rounded-full px-3 py-1 text-[11px] font-semibold transition ${brand.softButtonClass}`;
    const pillBtnDisabledClass = 'disabled:cursor-not-allowed disabled:opacity-40';

    return (
        <Section
            icon={Layers}
            iconClassName="bg-cyan-500/10 text-cyan-700 ring-1 ring-cyan-500/20 dark:bg-cyan-500/15 dark:text-cyan-200 dark:ring-cyan-500/25"
            title="分批加仓（防插针）"
            description="将一次开仓拆为 2–5 批：首批开仓，后续批次按间隔向同一仓位增加流动性，避免一次性打到插针高点。"
        >
            <ToggleSwitch
                label="启用分批加仓"
                description="作为默认策略，单次开仓时可覆盖。"
                checked={draft.dca_enabled}
                onChange={(value) => updateDraft('dca_enabled', value)}
            />
            {draft.dca_enabled ? (
                <>
                    <FieldCard label="每批占比" hint={`共 ${percentages.length} 批，之和必须等于 100%。`}>
                        <div className="space-y-2">
                            {percentages.map((value, idx) => (
                                <div key={idx} className="flex items-center gap-2">
                                    <div className="w-8 shrink-0 text-[11px] font-semibold text-zinc-500 dark:text-white/45">
                                        {idx === 0 ? '首批' : `第 ${idx + 1} 批`}
                                    </div>
                                    <InputWithSuffix
                                        type="number"
                                        step="0.1"
                                        min="5"
                                        max="100"
                                        value={value}
                                        onChange={(e) => updatePct(idx, e.target.value)}
                                        className={inputClass}
                                        suffix="%"
                                    />
                                    {percentages.length > 2 ? (
                                        <button
                                            type="button"
                                            onClick={() => removeBatch(idx)}
                                            className="shrink-0 rounded-xl border border-zinc-200/70 bg-white/80 p-2 text-zinc-500 transition hover:text-red-600 dark:border-white/10 dark:bg-white/[0.04] dark:text-white/45 dark:hover:text-red-400"
                                            aria-label="删除此批"
                                        >
                                            <Trash2 className="h-4 w-4" />
                                        </button>
                                    ) : null}
                                </div>
                            ))}
                        </div>
                        <div className="mt-3 flex items-center justify-between gap-2">
                            <div className={`text-[11px] font-semibold ${sumValid ? 'text-emerald-600 dark:text-emerald-300' : 'text-amber-600 dark:text-amber-300'}`}>
                                合计：{sum.toFixed(2)}% {sumValid ? '✓' : '（必须等于 100%）'}
                            </div>
                            <div className="flex items-center gap-2">
                                <button
                                    type="button"
                                    onClick={equalize}
                                    className={pillBtnClass}
                                >
                                    平均分配
                                </button>
                                <button
                                    type="button"
                                    onClick={addBatch}
                                    disabled={percentages.length >= 5}
                                    className={`${pillBtnClass} ${pillBtnDisabledClass}`}
                                >
                                    <Plus className="h-3 w-3" />
                                    追加批次
                                </button>
                            </div>
                        </div>
                    </FieldCard>
                    <FieldCard label="批次间隔" hint="每批之间的等待时间（0–300 秒，支持小数，0.3 = 300ms）">
                        <InputWithSuffix
                            type="number"
                            step="0.1"
                            min="0"
                            max="300"
                            value={draft.dca_interval_seconds}
                            onChange={(e) => updateDraft('dca_interval_seconds', Number(e.target.value) || 0)}
                            className={inputClass}
                            suffix="秒"
                        />
                    </FieldCard>
                </>
            ) : (
                <MutedHint>
                    关闭时，开仓一次性成交；开启后将按上面设置分批。单次开仓时仍可临时覆盖。
                </MutedHint>
            )}
        </Section>
    );
}
