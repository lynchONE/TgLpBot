import { useCallback, useEffect, useMemo, useState } from 'react';
import { buildGlobalConfigDraft as buildDraft } from '../../../shared/frontend/globalConfig.js';
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

function formatRebalanceTimeout(value) {
    const seconds = Number(value);
    if (!Number.isFinite(seconds)) return '--';
    return seconds <= 0 ? '立即执行' : `${seconds}s`;
}

function getChainLabel(value) {
    return CHAIN_OPTIONS.find((item) => item.value === value)?.label || '未设置';
}

function countEnabledFeatures(draft) {
    return [
        draft.auto_reinvest,
        draft.extra_notifications_enabled,
        draft.filter_chinese_tokens,
        draft.multi_chain_enabled,
        draft.multi_wallet_enabled,
        draft.bark_enabled,
    ].filter(Boolean).length;
}

export default function GlobalConfigPage({ open = true, onClose, apiBaseUrl, initData, accentTheme = 'lime', onConfigChanged, embedded = false }) {
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
    const riskTone = Number(draft.slippage_tolerance || 0) > 1 || Number(draft.zap_loss_tolerance || 0) > 1
        ? '偏激进'
        : '稳健';

    const inputClass = `w-full rounded-xl border border-zinc-200 bg-zinc-50/80 px-3 py-2.5 text-sm font-semibold text-zinc-900 outline-none transition placeholder:text-zinc-400 ${brand.inputFocusClass} dark:border-white/[0.06] dark:bg-white/[0.03] dark:text-white/90 dark:placeholder:text-white/25`;

    return (
        <ConfigFrame
            embedded={embedded}
            open={open}
            onClose={onClose}
            title="全局配置"
            maxHeightClass="max-h-[92vh]"
            contentClassName="px-5 pb-0 sm:pb-0"
            className="bg-[linear-gradient(180deg,rgba(250,250,249,0.96),rgba(255,255,255,0.9))] dark:bg-[linear-gradient(180deg,rgba(17,19,24,0.96),rgba(10,12,16,0.94))]"
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
                <div className="space-y-3 pb-4">
                    <section className="rounded-2xl border border-zinc-200/80 bg-white p-3 dark:border-white/5 dark:bg-[#14171c]">
                        <div className="flex items-start gap-3">
                                <div className={`inline-flex h-10 w-10 shrink-0 items-center justify-center rounded-xl ${brand.iconChipClass}`}>
                                    <Settings2 className="h-5 w-5" />
                                </div>
                                <div className="min-w-0 flex-1">
                                    <div className="text-[14px] font-extrabold leading-tight text-zinc-900 dark:text-white/95">全局配置</div>
                                    <div className="mt-0.5 text-[10px] text-zinc-500 dark:text-white/40">交易保护 / 分批加仓 / 链路钱包 / 通知过滤</div>
                                </div>
                            </div>

                            <div className="mt-3 grid grid-cols-3 gap-1.5">
                                <QuickSwitch label="多钱包" active={draft.multi_wallet_enabled} onClick={() => updateDraft('multi_wallet_enabled', !draft.multi_wallet_enabled)} />
                                <QuickSwitch label="分批" active={draft.dca_enabled} onClick={() => updateDraft('dca_enabled', !draft.dca_enabled)} />
                                <QuickSwitch label="Bark" active={draft.bark_enabled} onClick={() => updateDraft('bark_enabled', !draft.bark_enabled)} />
                            </div>
                    </section>

                    <section className="grid grid-cols-2 gap-2">
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
                            <SummaryStat
                                label="风险偏好"
                                value={riskTone}
                            />
                    </section>

                    <div className="grid gap-3 lg:grid-cols-[1.15fr_0.85fr]">
                        <Section
                            icon={Shield}
                            iconClassName={brand.iconChipClass}
                            title="交易保护"
                            description="常用安全参数放在一起，先调这里就够用。"
                            accent={brand.dotClass}
                        >
                            <div className="grid gap-3 sm:grid-cols-2">
                                <FieldCard label="再平衡超时" hint={`-1 表示立即执行。当前：${formatRebalanceTimeout(draft.rebalance_timeout)}`}>
                                    <InputWithSuffix
                                        type="number"
                                        value={draft.rebalance_timeout}
                                        onChange={(e) => updateDraft('rebalance_timeout', Number(e.target.value) || 0)}
                                        className={inputClass}
                                        placeholder="-1 / 10"
                                        suffix="秒"
                                    />
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
                            icon={Sparkles}
                            iconClassName="bg-violet-500/10 text-violet-700 ring-1 ring-violet-500/20 dark:bg-violet-500/15 dark:text-violet-200 dark:ring-violet-500/25"
                            title="自动化"
                            description="把自动复投等开关独立出来，避免和链路配置混在一起。"
                            compact
                        >
                            <ToggleSwitch
                                label="自动复投"
                                description="把已实现利润自动投入下一次操作。"
                                checked={draft.auto_reinvest}
                                onChange={(value) => updateDraft('auto_reinvest', value)}
                            />
                            <MutedHint>
                                自动类开关建议小额验证后再长期开启。
                            </MutedHint>
                        </Section>
                    </div>

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
                        description="决定默认网络、是否显示链切换，以及是否启用多钱包。"
                    >
                        <FieldCard label="默认网络" hint="新建操作时默认使用的链">
                            <CustomSelect
                                value={draft.default_chain}
                                onChange={(value) => updateDraft('default_chain', value)}
                                options={CHAIN_OPTIONS}
                            />
                        </FieldCard>
                        <div className="grid gap-3 sm:grid-cols-2">
                            <ToggleSwitch
                                label="多链模式"
                                description="允许在 BSC 和 Base 之间切换。"
                                checked={draft.multi_chain_enabled}
                                onChange={(value) => updateDraft('multi_chain_enabled', value)}
                            />
                            <ToggleSwitch
                                label="多钱包模式"
                                description="允许同一账户管理多个钱包地址。"
                                checked={draft.multi_wallet_enabled}
                                onChange={(value) => updateDraft('multi_wallet_enabled', value)}
                            />
                        </div>
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

                    <div className={`${embedded ? 'sticky bottom-[calc(76px+env(safe-area-inset-bottom,0px))] -mx-1 rounded-[24px] border border-zinc-200/70 bg-white/[0.92] px-4 pb-4 pt-3 shadow-[0_18px_50px_rgba(15,23,42,0.12)] dark:border-white/[0.08] dark:bg-[rgba(17,19,24,0.96)] dark:shadow-[0_18px_50px_rgba(0,0,0,0.42)]' : 'sticky bottom-0 -mx-5 border-t border-zinc-200/70 bg-white/[0.88] px-5 pb-[calc(env(safe-area-inset-bottom,0px)+12px)] pt-3 dark:border-white/[0.08] dark:bg-[rgba(17,19,24,0.94)]'} backdrop-blur-xl`}>
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
        </ConfigFrame>
    );
}

function ConfigFrame({ embedded, children, ...sheetProps }) {
    if (embedded) {
        return <div className="space-y-4 pb-1">{children}</div>;
    }
    return <BottomSheet {...sheetProps}>{children}</BottomSheet>;
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
        <div className={`rounded-xl border border-zinc-200/70 bg-zinc-50 px-3 py-2.5 ring-1 ring-zinc-200 dark:border-white/[0.06] dark:bg-white/[0.03] dark:ring-white/[0.06] ${toneClass || ''}`}>
            <div className="text-[9px] font-medium uppercase tracking-wide text-zinc-400 dark:text-white/35">{label}</div>
            <div className="mt-1 truncate text-base font-extrabold leading-none text-zinc-900 dark:text-white/95">{value}</div>
        </div>
    );
}

function QuickSwitch({ label, active, onClick }) {
    return (
        <button
            type="button"
            onClick={onClick}
            className={`rounded-xl px-3 py-2.5 text-left ring-1 transition active:scale-[0.98] ${
                active
                    ? 'bg-emerald-500/[0.08] text-emerald-700 ring-emerald-500/20 dark:bg-emerald-500/[0.10] dark:text-emerald-300 dark:ring-emerald-400/20'
                    : 'bg-zinc-50 text-zinc-500 ring-zinc-200 hover:bg-zinc-100 dark:bg-white/[0.03] dark:text-white/45 dark:ring-white/[0.06] dark:hover:bg-white/[0.06]'
            }`}
        >
            <div className="text-[9px] font-medium uppercase tracking-wide">{active ? 'ON' : 'OFF'}</div>
            <div className="mt-1 text-xs font-bold">{label}</div>
        </button>
    );
}

function Section({ icon: Icon, iconClassName, title, description, children, accent = '', compact = false }) {
    return (
        <section className={`relative overflow-hidden rounded-2xl border border-zinc-200/80 bg-white p-3 dark:border-white/5 dark:bg-[#14171c] ${compact ? 'h-full' : ''}`}>
            {accent ? <div className={`absolute -right-8 -top-8 h-20 w-20 rounded-full blur-2xl opacity-20 ${accent}`} /> : null}
            <div className="mb-3 flex items-start gap-2.5">
                <div className={`inline-flex h-9 w-9 shrink-0 items-center justify-center rounded-xl ${iconClassName}`}>
                    <Icon className="h-[18px] w-[18px]" />
                </div>
                <div className="min-w-0 flex-1">
                    <div className="text-[12px] font-bold text-zinc-900 dark:text-white/90">{title}</div>
                    <div className="mt-1 text-[10px] leading-4 text-zinc-500 dark:text-white/40">{description}</div>
                </div>
            </div>
            <div className="space-y-3">{children}</div>
        </section>
    );
}

function FieldCard({ label, hint, children }) {
    return (
        <div className="rounded-xl bg-zinc-50 px-3 py-2.5 ring-1 ring-zinc-200 dark:bg-white/[0.03] dark:ring-white/[0.06]">
            <div className="mb-2">
                <div className="text-[11px] font-semibold text-zinc-800 dark:text-white/80">{label}</div>
                {hint ? <div className="mt-1 text-[10px] leading-4 text-zinc-500 dark:text-white/35">{hint}</div> : null}
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
        <div className="rounded-xl border border-dashed border-zinc-200 bg-zinc-50/80 px-3 py-3 text-[11px] leading-5 text-zinc-500 dark:border-white/[0.08] dark:bg-white/[0.02] dark:text-white/45">
            {children}
        </div>
    );
}

function DCASection({ draft, updateDraft, inputClass }) {
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

    const actionBtnClass = 'inline-flex items-center gap-1.5 rounded-2xl bg-white/70 px-3 py-2 text-xs font-semibold text-zinc-700 ring-1 ring-zinc-200 shadow-sm transition hover:bg-white dark:bg-white/5 dark:text-white/70 dark:ring-white/10 dark:hover:bg-white/10 disabled:cursor-not-allowed disabled:opacity-40';

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
                                    className={actionBtnClass}
                                >
                                    平均分配
                                </button>
                                <button
                                    type="button"
                                    onClick={addBatch}
                                    disabled={percentages.length >= 5}
                                    className={actionBtnClass}
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
                            step="0.001"
                            min="0"
                            max="300"
                            value={draft.dca_interval_seconds}
                            onChange={(e) => updateDraft('dca_interval_seconds', Number(e.target.value) || 0)}
                            className={inputClass}
                            suffix="秒"
                        />
                    </FieldCard>
                    <FieldCard label="最小拆分金额" hint="当本次开仓金额低于该阈值时，将直接按单笔成交，不再拆成多批执行。">
                        <InputWithSuffix
                            type="number"
                            step="0.1"
                            min="0"
                            value={draft.dca_min_split_amount_usdt}
                            onChange={(e) => updateDraft('dca_min_split_amount_usdt', Number(e.target.value) || 0)}
                            className={inputClass}
                            suffix="USDT"
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
