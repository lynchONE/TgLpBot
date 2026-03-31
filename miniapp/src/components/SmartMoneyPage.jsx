import React, { useState, useEffect, useCallback, useMemo, useRef } from 'react';
import {
    Eye, Wallet, Settings, Search, Plus, ExternalLink, X, Check,
    ChevronRight, ChevronDown, ChevronLeft, Pause, Play, Trash2, Copy, Flame, Pencil,
} from 'lucide-react';
import {
    fetchSMPools, fetchSMPoolStats, fetchSMPositions, fetchSMWallets,
    fetchSMPositionDetail,
    fetchSMStats, addSMWallet, updateSMWallet, deleteSMWallet,
    fetchSMContracts, addSMContract, updateSMContract, deleteSMContract,
    uploadSMWalletAvatar, resolveSMAvatarAssetUrl,
    fetchSMGoldenDogConfig, saveSMGoldenDogConfig, testSMGoldenDogConfig,
} from '../lib/smartMoneyApi';
import { getBrandTheme } from '../lib/brand';
import { formatDurationFrom } from '../lib/time';
import FlashIcon from './FlashIcon.jsx';
import PositionCard from './PositionCard.jsx';
import uniswapIcon from '../image/uniswap.svg';
import pancakeIcon from '../image/pancake.svg';
import gmgnIcon from '../image/gmgn.svg';
import avatar01 from '../../../webapp/src/icon/avatar_01.png';
import avatar02 from '../../../webapp/src/icon/avatar_02.png';
import avatar03 from '../../../webapp/src/icon/avatar_03.png';
import avatar04 from '../../../webapp/src/icon/avatar_04.png';
import avatar05 from '../../../webapp/src/icon/avatar_05.png';
import avatar06 from '../../../webapp/src/icon/avatar_06.png';
import avatar07 from '../../../webapp/src/icon/avatar_07.png';
import avatar08 from '../../../webapp/src/icon/avatar_08.png';
import avatar09 from '../../../webapp/src/icon/avatar_09.png';
import avatar10 from '../../../webapp/src/icon/avatar_10.png';
import avatar11 from '../../../webapp/src/icon/avatar_11.png';
import avatar12 from '../../../webapp/src/icon/avatar_12.png';
import avatar13 from '../../../webapp/src/icon/avatar_13.png';
import avatar14 from '../../../webapp/src/icon/avatar_14.png';
import avatar15 from '../../../webapp/src/icon/avatar_15.png';
import avatar16 from '../../../webapp/src/icon/avatar_16.png';

const PROTOCOL_MAP = {
    pancake_v3: { version: 'V3', icon: pancakeIcon, color: '#d1884f' },
    uniswap_v3: { version: 'V3', icon: uniswapIcon, color: '#ff007a' },
    uniswap_v4: { version: 'V4', icon: uniswapIcon, color: '#ff007a' },
};
const WALLET_AVATAR_ICONS = [
    avatar01,
    avatar02,
    avatar03,
    avatar04,
    avatar05,
    avatar06,
    avatar07,
    avatar08,
    avatar09,
    avatar10,
    avatar11,
    avatar12,
    avatar13,
    avatar14,
    avatar15,
    avatar16,
];
const SMART_MONEY_AVATAR_ACCEPT = 'image/png,image/jpeg,image/webp';
const SMART_MONEY_AVATAR_MAX_BYTES = 5 * 1024 * 1024;
const GMGN_STABLE_SYMBOLS = new Set(['usdc', 'usdt', 'busd', 'dai', 'frax', 'usdd', 'fdusd', 'wbnb', 'weth', 'wsol', 'bnb', 'eth', 'sol']);

function getBrandLinkClass(brand) {
    return brand?.key === 'emerald'
        ? 'text-emerald-300 hover:text-emerald-200'
        : 'text-[#dfff8b] hover:text-[#efffb8]';
}

function GoldenDogPageContent({ apiBaseUrl, initData, brand }) {
    const hasInitData = Boolean(String(initData || '').trim());
    const [loading, setLoading] = useState(hasInitData);
    const [saving, setSaving] = useState(false);
    const [testingMode, setTestingMode] = useState('');
    const [error, setError] = useState('');
    const [notice, setNotice] = useState('');
    const [status, setStatus] = useState(null);
    const [draft, setDraft] = useState(() => createGoldenDogDraft());
    const [activeTab, setActiveTab] = useState('wallet');

    const barkStatusText = useMemo(() => goldenDogBarkStatusText(status), [status]);
    const intensityOptions = useMemo(
        () => (Array.isArray(status?.available_intensities) && status.available_intensities.length > 0
            ? status.available_intensities
            : GOLDEN_DOG_INTENSITY_OPTIONS),
        [status],
    );
    const activePoolThresholdCount = useMemo(
        () => countGoldenDogPoolThresholds(draft.pool_mode),
        [draft.pool_mode],
    );

    const applyResponse = useCallback((resp) => {
        setStatus(resp || null);
        setDraft(mapGoldenDogConfigToDraft(resp?.config));
    }, []);

    const loadConfig = useCallback(async () => {
        if (!hasInitData) {
            setLoading(false);
            setStatus(null);
            return;
        }
        setLoading(true);
        setError('');
        try {
            applyResponse(await fetchSMGoldenDogConfig({ apiBaseUrl, initData, chain: 'bsc' }));
        } catch (err) {
            setError(String(err?.message || err || '加载失败'));
        } finally {
            setLoading(false);
        }
    }, [apiBaseUrl, applyResponse, hasInitData, initData]);

    useEffect(() => {
        loadConfig();
    }, [loadConfig]);

    const updateWalletMode = useCallback((key, value) => {
        setDraft((prev) => ({
            ...prev,
            wallet_mode: { ...prev.wallet_mode, [key]: value },
        }));
    }, []);

    const updatePoolMode = useCallback((key, value) => {
        setDraft((prev) => ({
            ...prev,
            pool_mode: { ...prev.pool_mode, [key]: value },
        }));
    }, []);

    const buildSavePayload = useCallback(() => {
        const walletMinWallets = parseGoldenDogRequiredInt(draft.wallet_mode.min_wallets, '钱包数量');
        const walletWindowMinutes = parseGoldenDogRequiredInt(draft.wallet_mode.window_minutes, '统计窗口');
        const walletCooldownMinutes = parseGoldenDogRequiredInt(draft.wallet_mode.cooldown_minutes, '冷却时间', { min: 0 });
        const poolCooldownMinutes = parseGoldenDogRequiredInt(draft.pool_mode.cooldown_minutes, '池子模式冷却时间', { min: 0 });
        const poolMinTotalFees = parseGoldenDogOptionalNumber(draft.pool_mode.min_total_fees, '最小手续费');
        const poolMinTransactionCount = parseGoldenDogOptionalNumber(draft.pool_mode.min_transaction_count, '最小交易笔数');
        const poolMinTVL = parseGoldenDogOptionalNumber(draft.pool_mode.min_tvl, '最小 TVL');
        const poolMinVolume = parseGoldenDogOptionalNumber(draft.pool_mode.min_volume, '最小 VOL');
        const poolMinFeeRate = parseGoldenDogOptionalNumber(draft.pool_mode.min_fee_rate, '最小费率');
        const poolMinActiveLiquidityRatioPct = parseGoldenDogOptionalNumber(draft.pool_mode.min_active_liquidity_ratio, '最小活跃费率', { max: 100 });

        const thresholdCount = [
            poolMinTotalFees,
            poolMinTransactionCount,
            poolMinTVL,
            poolMinVolume,
            poolMinFeeRate,
            poolMinActiveLiquidityRatioPct,
        ].filter((value) => Number(value) > 0).length;
        if (draft.pool_mode.enabled && thresholdCount === 0) {
            throw new Error('池子参数模式至少需要填写一个筛选阈值。');
        }

        return {
            wallet_mode: {
                enabled: Boolean(draft.wallet_mode.enabled),
                min_wallets: walletMinWallets,
                window_minutes: walletWindowMinutes,
                cooldown_minutes: walletCooldownMinutes,
                intensity: draft.wallet_mode.intensity || 'ring',
            },
            pool_mode: {
                enabled: Boolean(draft.pool_mode.enabled),
                cooldown_minutes: poolCooldownMinutes,
                min_total_fees: poolMinTotalFees,
                min_transaction_count: poolMinTransactionCount,
                min_tvl: poolMinTVL,
                min_volume: poolMinVolume,
                min_fee_rate: poolMinFeeRate,
                min_active_liquidity_ratio: poolMinActiveLiquidityRatioPct,
                intensity: draft.pool_mode.intensity || 'ring',
            },
        };
    }, [draft]);

    const handleSave = useCallback(async () => {
        if (!hasInitData) {
            setError('请先登录 Telegram 后再保存监控通知。');
            return;
        }

        setSaving(true);
        setError('');
        setNotice('');
        try {
            const resp = await saveSMGoldenDogConfig({
                apiBaseUrl,
                initData,
                chain: 'bsc',
                config: buildSavePayload(),
            });
            applyResponse(resp);
            setNotice('配置已保存');
        } catch (err) {
            setError(String(err?.message || err || '保存失败'));
        } finally {
            setSaving(false);
        }
    }, [apiBaseUrl, applyResponse, buildSavePayload, hasInitData, initData]);

    const handleTest = useCallback(async (mode) => {
        if (!hasInitData) {
            setError('请先登录 Telegram 后再测试监控通知。');
            return;
        }

        setTestingMode(mode);
        setError('');
        setNotice('');
        try {
            const intensity = mode === 'pool' ? draft.pool_mode.intensity : draft.wallet_mode.intensity;
            const resp = await testSMGoldenDogConfig({
                apiBaseUrl,
                initData,
                chain: 'bsc',
                mode,
                intensity,
            });
            setNotice(resp?.message || '测试通知已发送');
        } catch (err) {
            setError(String(err?.message || err || '测试失败'));
        } finally {
            setTestingMode('');
        }
    }, [apiBaseUrl, draft.pool_mode.intensity, draft.wallet_mode.intensity, hasInitData, initData]);

    /* ── 自定义紧凑下拉 ── */
    const MiniSelect = useCallback(({ value, options: opts, onChange: onChg }) => {
        const [open, setOpen] = useState(false);
        const ref = useRef(null);
        const label = (opts.find(o => String(o.value) === String(value)) || opts[0])?.label || '';
        useEffect(() => {
            if (!open) return;
            const h = (e) => { if (ref.current && !ref.current.contains(e.target)) setOpen(false); };
            document.addEventListener('mousedown', h);
            document.addEventListener('touchstart', h);
            return () => { document.removeEventListener('mousedown', h); document.removeEventListener('touchstart', h); };
        }, [open]);
        return (
            <div ref={ref} className="relative">
                <button type="button" onClick={() => setOpen(v => !v)}
                    className="flex w-full items-center justify-between gap-1 rounded-lg border border-white/[0.07] bg-black/30 px-2.5 py-[7px] text-[11px] text-zinc-200 transition-colors"
                    style={{ borderColor: open ? 'rgba(251,191,36,0.3)' : undefined }}
                >
                    <span className="truncate">{label}</span>
                    <svg width="8" height="8" viewBox="0 0 10 10" fill="none" className="shrink-0 transition-transform" style={{ transform: open ? 'rotate(180deg)' : 'none' }}>
                        <path d="M2 3.5L5 6.5L8 3.5" stroke="#71717a" strokeWidth="1.4" strokeLinecap="round" strokeLinejoin="round" />
                    </svg>
                </button>
                {open && (
                    <div className="absolute left-0 right-0 bottom-[calc(100%+3px)] z-[60] rounded-lg border border-white/[0.08] bg-[rgba(12,12,15,0.97)] p-1 shadow-[0_-10px_36px_rgba(0,0,0,0.55)]"
                        style={{ backdropFilter: 'blur(14px)', WebkitBackdropFilter: 'blur(14px)' }}>
                        {opts.map(opt => {
                            const sel = String(opt.value) === String(value);
                            return (
                                <button key={opt.value} type="button"
                                    onClick={() => { onChg(opt.value); setOpen(false); }}
                                    className={`block w-full rounded-md px-2.5 py-[6px] text-left text-[11px] transition-colors
                                        ${sel ? 'bg-amber-400/12 text-amber-300' : 'text-zinc-400 active:bg-white/5'}`}
                                >{opt.label}</button>
                            );
                        })}
                    </div>
                )}
            </div>
        );
    }, []);

    const miniInputCls = "w-full rounded-lg border border-white/[0.07] bg-black/30 px-2.5 py-[7px] text-[11px] text-zinc-100 text-center placeholder-zinc-400 outline-none transition-colors focus:border-amber-400/40";
    const pillBtn = (active) => `rounded-lg px-2 py-[5px] text-[10px] font-semibold transition-all ${active ? 'bg-amber-400/15 text-amber-200' : 'bg-white/[0.04] text-zinc-500'}`;
    const pillBtnTeal = (active) => `rounded-lg px-2 py-[5px] text-[10px] font-semibold transition-all ${active ? 'bg-emerald-400/15 text-emerald-200' : 'bg-white/[0.04] text-zinc-500'}`;

    return (
        <div className="space-y-2.5 pb-24">
            {/* ── 顶部标题行 ── */}
            <div className="flex items-center justify-between gap-2">
                <div className="flex items-center gap-2 min-w-0">
                    <div className="flex h-7 w-7 shrink-0 items-center justify-center rounded-lg border border-amber-400/20 bg-amber-400/10 text-amber-300"><Flame size={13} /></div>
                    <div className="min-w-0">
                        <div className="text-[13px] font-semibold text-zinc-100 leading-tight">监控通知</div>
                        <div className="flex flex-wrap items-center gap-1 mt-0.5">
                            <span className={`inline-block rounded px-1 py-[1px] text-[9px] leading-none ${draft.wallet_mode.enabled ? 'bg-amber-400/12 text-amber-300' : 'bg-zinc-800 text-zinc-500'}`}>金狗{draft.wallet_mode.enabled ? '✓' : '✗'}</span>
                            <span className={`inline-block rounded px-1 py-[1px] text-[9px] leading-none ${draft.pool_mode.enabled ? 'bg-emerald-400/12 text-emerald-300' : 'bg-zinc-800 text-zinc-500'}`}>池子{draft.pool_mode.enabled ? '✓' : '✗'}</span>
                            <span className="inline-block rounded px-1 py-[1px] text-[9px] leading-none bg-zinc-800 text-zinc-400">Bark {barkStatusText}</span>
                        </div>
                    </div>
                </div>
                <button type="button" onClick={handleSave} disabled={saving || !hasInitData}
                    className={`shrink-0 rounded-xl px-3 py-1.5 text-[11px] font-semibold disabled:opacity-40 ${brand.solidButtonClass}`}
                >{saving ? '...' : '保存'}</button>
            </div>

            {!hasInitData ? <div className="rounded-lg border border-red-500/20 bg-red-500/10 px-2.5 py-1.5 text-[11px] text-red-200">请先登录 Telegram</div> : null}
            {error ? <div className="rounded-lg border border-red-500/20 bg-red-500/10 px-2.5 py-1.5 text-[11px] text-red-200">{error}</div> : null}
            {!error && notice ? <div className="rounded-lg border border-emerald-500/20 bg-emerald-500/10 px-2.5 py-1.5 text-[11px] text-emerald-200">{notice}</div> : null}

            {loading ? <div className="py-6 text-center text-[11px] text-zinc-500">加载中...</div> : (
                <div className="space-y-2.5">
                    {/* ── Tab 切换 ── */}
                    <div className="flex items-center gap-0.5 rounded-xl bg-zinc-900/60 p-0.5 border border-white/[0.03]">
                        <button type="button" onClick={() => setActiveTab('wallet')}
                            className={`flex-1 rounded-[10px] py-1.5 text-[11px] font-semibold transition-all ${activeTab === 'wallet' ? 'bg-amber-400/15 text-amber-300' : 'text-zinc-500'}`}
                        >金狗通知</button>
                        <button type="button" onClick={() => setActiveTab('pool')}
                            className={`flex-1 rounded-[10px] py-1.5 text-[11px] font-semibold transition-all ${activeTab === 'pool' ? 'bg-emerald-400/15 text-emerald-300' : 'text-zinc-500'}`}
                        >池子监控</button>
                    </div>

                    {/* ── 金狗通知 Tab ── */}
                    {activeTab === 'wallet' && (
                        <section className="rounded-2xl border border-amber-400/10 bg-gradient-to-b from-[rgba(20,20,24,0.96)] to-[rgba(9,9,11,0.98)] p-3">
                            <div className="flex items-center justify-between gap-2">
                                <span className="text-[12px] font-semibold text-zinc-100">聪明钱聚集</span>
                                <div className="flex items-center gap-1">
                                    <button type="button" className={pillBtn(draft.wallet_mode.enabled)} onClick={() => updateWalletMode('enabled', true)}>开启</button>
                                    <button type="button" className={pillBtn(!draft.wallet_mode.enabled)} onClick={() => updateWalletMode('enabled', false)}>关闭</button>
                                    <button type="button" className={pillBtn(false)} disabled={testingMode === 'wallet' || !hasInitData} onClick={() => handleTest('wallet')}>{testingMode === 'wallet' ? '...' : '测试'}</button>
                                </div>
                            </div>
                            {/* inline 指标条 */}
                            <div className="mt-2 flex flex-wrap items-center gap-x-3 gap-y-1 text-[10px] justify-center">
                                <span className="text-zinc-400">钱包 <span className="text-zinc-100">{draft.wallet_mode.min_wallets || '--'}</span></span>
                                <span className="text-zinc-400">窗口 <span className="text-zinc-100">{draft.wallet_mode.window_minutes || '--'}m</span></span>
                                <span className="text-zinc-400">冷却 <span className="text-zinc-100">{draft.wallet_mode.cooldown_minutes || '--'}m</span></span>
                                <span className="text-zinc-400">强度 <span className="text-zinc-100">{goldenDogIntensityLabel(draft.wallet_mode.intensity)}</span></span>
                            </div>
                            <div className="mt-3 grid grid-cols-2 gap-1.5">
                                <input className={miniInputCls} type="number" min="1" step="1" placeholder="钱包数量" value={draft.wallet_mode.min_wallets} onChange={(e) => updateWalletMode('min_wallets', e.target.value)} />
                                <input className={miniInputCls} type="number" min="1" step="1" placeholder="窗口(分钟)" value={draft.wallet_mode.window_minutes} onChange={(e) => updateWalletMode('window_minutes', e.target.value)} />
                                <input className={miniInputCls} type="number" min="0" step="1" placeholder="冷却(分钟)" value={draft.wallet_mode.cooldown_minutes} onChange={(e) => updateWalletMode('cooldown_minutes', e.target.value)} />
                                <MiniSelect value={draft.wallet_mode.intensity} options={intensityOptions} onChange={(v) => updateWalletMode('intensity', v)} />
                            </div>
                        </section>
                    )}

                    {/* ── 池子监控 Tab ── */}
                    {activeTab === 'pool' && (
                        <section className="rounded-2xl border border-emerald-400/10 bg-gradient-to-b from-[rgba(18,24,27,0.96)] to-[rgba(9,11,14,0.98)] p-3">
                            <div className="flex items-center justify-between gap-2">
                                <span className="text-[12px] font-semibold text-zinc-100">池子参数监控</span>
                                <div className="flex items-center gap-1">
                                    <button type="button" className={pillBtnTeal(draft.pool_mode.enabled)} onClick={() => updatePoolMode('enabled', true)}>开启</button>
                                    <button type="button" className={pillBtnTeal(!draft.pool_mode.enabled)} onClick={() => updatePoolMode('enabled', false)}>关闭</button>
                                    <button type="button" className={pillBtnTeal(false)} disabled={testingMode === 'pool' || !hasInitData} onClick={() => handleTest('pool')}>{testingMode === 'pool' ? '...' : '测试'}</button>
                                </div>
                            </div>
                            {/* inline 指标条 */}
                            <div className="mt-2 flex flex-wrap items-center gap-x-3 gap-y-1 text-[10px] justify-center">
                                <span className="text-zinc-400">TVL <span className="text-zinc-100">{goldenDogThresholdText(draft.pool_mode.min_tvl, '$')}</span></span>
                                <span className="text-zinc-400">VOL <span className="text-zinc-100">{goldenDogThresholdText(draft.pool_mode.min_volume, '$')}</span></span>
                                <span className="text-zinc-400">笔数 <span className="text-zinc-100">{goldenDogThresholdText(draft.pool_mode.min_transaction_count)}</span></span>
                                <span className="text-zinc-400">费率 <span className="text-zinc-100">{goldenDogThresholdText(draft.pool_mode.min_fee_rate, '', '%')}</span></span>
                                <span className="text-zinc-400">活跃 <span className="text-zinc-100">{goldenDogThresholdText(draft.pool_mode.min_active_liquidity_ratio, '', '%')}</span></span>
                            </div>
                            <div className="mt-2.5 grid grid-cols-2 gap-1.5">
                                <input className={miniInputCls} type="number" min="0" step="0.01" placeholder="最小 TVL($)" value={draft.pool_mode.min_tvl} onChange={(e) => updatePoolMode('min_tvl', e.target.value)} />
                                <input className={miniInputCls} type="number" min="0" step="0.01" placeholder="最小 VOL($)" value={draft.pool_mode.min_volume} onChange={(e) => updatePoolMode('min_volume', e.target.value)} />
                                <input className={miniInputCls} type="number" min="0" step="1" placeholder="最小交易笔数" value={draft.pool_mode.min_transaction_count} onChange={(e) => updatePoolMode('min_transaction_count', e.target.value)} />
                                <input className={miniInputCls} type="number" min="0" step="0.001" placeholder="费率(%)" value={draft.pool_mode.min_fee_rate} onChange={(e) => updatePoolMode('min_fee_rate', e.target.value)} />
                                <input className={miniInputCls} type="number" min="0" step="0.001" placeholder="活跃费率(%)" value={draft.pool_mode.min_active_liquidity_ratio} onChange={(e) => updatePoolMode('min_active_liquidity_ratio', e.target.value)} />
                                <input className={miniInputCls} type="number" min="0" step="1" placeholder="冷却(分钟)" value={draft.pool_mode.cooldown_minutes} onChange={(e) => updatePoolMode('cooldown_minutes', e.target.value)} />
                            </div>
                            <div className="mt-2">
                                <MiniSelect value={draft.pool_mode.intensity} options={intensityOptions} onChange={(v) => updatePoolMode('intensity', v)} />
                            </div>
                        </section>
                    )}
                </div>
            )}
        </div>
    );
}

const GOLDEN_DOG_INTENSITY_OPTIONS = [
    { value: 'ring', label: '\u54CD\u94C3', description: '\u666E\u901A\u63D0\u9192' },
    { value: 'persistent_ring', label: '\u6301\u7EED\u54CD\u94C3', description: '\u6301\u7EED\u63D0\u9192' },
    { value: 'critical_ring', label: '\u9759\u97F3\u5F3A\u63D0\u9192', description: '\u9759\u97F3\u4E5F\u54CD' },
];

const GOLDEN_DOG_FEE_RATE_OPTIONS = [
    { value: '', label: '不限' },
    { value: '100', label: '0.0100%' },
    { value: '500', label: '0.0500%' },
    { value: '2500', label: '0.2500%' },
    { value: '3000', label: '0.3000%' },
    { value: '10000', label: '1.0000%' },
];

function createGoldenDogDraft() {
    return {
        wallet_mode: {
            enabled: false,
            min_wallets: '3',
            window_minutes: '10',
            cooldown_minutes: '30',
            intensity: 'ring',
        },
        pool_mode: {
            enabled: false,
            cooldown_minutes: '30',
            min_total_fees: '',
            min_transaction_count: '',
            min_tvl: '',
            min_volume: '',
            min_fee_rate: '',
            min_active_liquidity_ratio: '',
            intensity: 'ring',
        },
    };
}

function formatGoldenDogDraftValue(value, { emptyWhenZero = false, multiplier = 1 } = {}) {
    const num = Number(value);
    if (!Number.isFinite(num)) return emptyWhenZero ? '' : '0';
    const scaled = Number((num * multiplier).toFixed(4));
    if (emptyWhenZero && Math.abs(scaled) < 0.000001) return '';
    return String(scaled);
}

function mapGoldenDogConfigToDraft(cfg) {
    const next = createGoldenDogDraft();
    const source = cfg || {};
    next.wallet_mode.enabled = Boolean(source.enabled);
    next.wallet_mode.min_wallets = String(source.min_wallets ?? 3);
    next.wallet_mode.window_minutes = String(source.window_minutes ?? 10);
    next.wallet_mode.cooldown_minutes = String(source.cooldown_minutes ?? 30);
    next.wallet_mode.intensity = String(source.wallet_intensity || 'ring');
    next.pool_mode.enabled = Boolean(source.pool_enabled);
    next.pool_mode.cooldown_minutes = String(source.pool_cooldown_minutes ?? 30);
    next.pool_mode.min_total_fees = formatGoldenDogDraftValue(source.pool_min_total_fees, { emptyWhenZero: true });
    next.pool_mode.min_transaction_count = formatGoldenDogDraftValue(source.pool_min_transaction_count, { emptyWhenZero: true });
    next.pool_mode.min_tvl = formatGoldenDogDraftValue(source.pool_min_tvl, { emptyWhenZero: true });
    next.pool_mode.min_volume = formatGoldenDogDraftValue(source.pool_min_volume, { emptyWhenZero: true });
    next.pool_mode.min_fee_rate = formatGoldenDogDraftValue(source.pool_min_fee_rate, { emptyWhenZero: true });
    next.pool_mode.min_active_liquidity_ratio = formatGoldenDogDraftValue(source.pool_min_active_liquidity_ratio, { emptyWhenZero: true });
    next.pool_mode.intensity = String(source.pool_intensity || 'ring');
    return next;
}

function parseGoldenDogRequiredInt(value, label, { min = 1 } = {}) {
    const num = Number.parseInt(String(value || '').trim(), 10);
    if (!Number.isFinite(num) || num < min) {
        throw new Error(`${label}必须大于等于 ${min}。`);
    }
    return num;
}

function parseGoldenDogOptionalNumber(value, label, { max = Number.MAX_SAFE_INTEGER } = {}) {
    const raw = String(value || '').trim();
    if (!raw) return 0;
    const num = Number(raw);
    if (!Number.isFinite(num) || num < 0) {
        throw new Error(`${label}必须是大于等于 0 的数字。`);
    }
    if (num > max) {
        throw new Error(`${label}不能大于 ${max}。`);
    }
    return num;
}

function goldenDogBarkStatusText(status) {
    if (status?.bark_ready) return '已就绪';
    if (status?.bark_configured) return status?.bark_enabled ? '已配置未就绪' : '已配置未开启';
    return '未配置';
}

function goldenDogIntensityLabel(value) {
    return GOLDEN_DOG_INTENSITY_OPTIONS.find((item) => item.value === value)?.label || '响铃';
}

function countGoldenDogPoolThresholds(poolMode) {
    return [
        'min_total_fees',
        'min_transaction_count',
        'min_tvl',
        'min_volume',
        'min_fee_rate',
        'min_active_liquidity_ratio',
    ].reduce((count, key) => count + (String(poolMode?.[key] || '').trim() ? 1 : 0), 0);
}

function goldenDogThresholdText(value, prefix = '', suffix = '') {
    const raw = String(value || '').trim();
    return raw ? `${prefix}${raw}${suffix}` : '--';
}

function getBrandFocusRingClass(brand) {
    return brand?.key === 'emerald'
        ? 'focus:ring-emerald-500'
        : 'focus:ring-[#bcff2f]';
}

function getInputClass(brand) {
    return `w-full rounded-2xl border border-white/[0.04] bg-zinc-950/55 px-3 py-2.5 text-sm text-zinc-100 outline-none placeholder:text-zinc-500 focus:ring-1 ${getBrandFocusRingClass(brand)}`;
}

function getFilterButtonClass(active, brand) {
    return active
        ? brand.softButtonClass
        : 'border border-white/[0.04] bg-zinc-900/55 text-zinc-400 hover:bg-zinc-800/70';
}

function getIconButtonClass(danger = false) {
    return [
        'inline-flex h-9 w-9 items-center justify-center rounded-xl border transition disabled:cursor-not-allowed disabled:opacity-50',
        danger
            ? 'border-red-500/20 bg-red-500/10 text-red-300 hover:bg-red-500/15'
            : 'border-white/[0.05] bg-zinc-900/65 text-zinc-300 hover:bg-zinc-800/80',
    ].join(' ');
}

function walletAvatarIdx(addr) {
    if (!addr || addr.length < 6) return 0;
    return parseInt(addr.slice(-4), 16) % WALLET_AVATAR_ICONS.length;
}

function resolveWalletAvatarSrc(address, avatarUrl) {
    const preferred = resolveSMAvatarAssetUrl(avatarUrl);
    if (preferred) return preferred;
    return WALLET_AVATAR_ICONS[walletAvatarIdx(address)] || WALLET_AVATAR_ICONS[0];
}

function shortAddr(addr) {
    if (!addr || addr.length < 10) return addr || '';
    return addr.slice(0, 6) + '...' + addr.slice(-4);
}

function tailAddr(value) {
    const raw = String(value || '').trim();
    if (!raw) return '--';
    return raw.slice(-4);
}

function isHexAddressValue(value) {
    return /^0x[a-fA-F0-9]{40}$/.test(String(value || '').trim());
}

function getPairLabel(value) {
    const pair = String(value?.trading_pair || '').trim();
    if (pair && pair !== '/') return pair;
    const left = String(value?.token0_symbol || '').trim();
    const right = String(value?.token1_symbol || '').trim();
    if (left && right) return `${left}/${right}`;
    if (left) return left;
    if (right) return right;
    return '未识别交易对';
}

function getPoolIdentifier(value) {
    return String(value?.pool_address || '').trim();
}

function resolvePoolChain(value) {
    if (String(value?.chain || '').trim()) return String(value.chain).trim().toLowerCase();
    return Number(value?.chain_id) === 8453 ? 'base' : 'bsc';
}

function pickGmgnTokenAddress(pool) {
    const pair = String(pool?.trading_pair || '').trim();
    const token0 = String(pool?.token0_address || '').trim();
    const token1 = String(pool?.token1_address || '').trim();
    if (!pair) return token0 || token1;

    const symbols = pair.split('/').map((part) => String(part || '').trim().toLowerCase());
    if (symbols.length !== 2) return token0 || token1;

    const [leftSymbol, rightSymbol] = symbols;
    const leftStable = GMGN_STABLE_SYMBOLS.has(leftSymbol);
    const rightStable = GMGN_STABLE_SYMBOLS.has(rightSymbol);
    if (leftStable && !rightStable) return token1 || token0;
    if (rightStable && !leftStable) return token0 || token1;
    return token0 || token1;
}

function buildGmgnUrl(pool, fallbackChain = 'bsc') {
    const tokenAddress = pickGmgnTokenAddress(pool);
    if (!tokenAddress) return '';
    const chain = String(pool?.chain || fallbackChain || 'bsc').trim().toLowerCase() === 'base' ? 'base' : 'bsc';
    return `https://gmgn.ai/${chain}/token/${tokenAddress}`;
}

function getPairInitials(value) {
    return getPairLabel(value)
        .split(/[/-]/)
        .map((part) => String(part || '').trim().charAt(0).toUpperCase())
        .join('')
        .slice(0, 2) || 'LP';
}

function formatFeeTier(fee) {
    if (!fee) return '';
    return `${(Number(fee) / 10000).toFixed(4)}%`;
}

function formatUSDCompact(value) {
    const num = Number(value);
    if (!Number.isFinite(num) || num <= 0) return '—';
    const abs = Math.abs(num);
    if (abs >= 1000000) return `$${(num / 1000000).toFixed(abs >= 10000000 ? 0 : 1).replace(/\.0$/, '')}M`;
    if (abs >= 1000) return `$${(num / 1000).toFixed(abs >= 10000 ? 0 : 1).replace(/\.0$/, '')}K`;
    if (abs >= 100) return `$${num.toFixed(0)}`;
    if (abs >= 10) return `$${num.toFixed(1).replace(/\.0$/, '')}`;
    return `$${num.toFixed(2).replace(/0+$/, '').replace(/\.$/, '')}`;
}

function formatWalletBalance(value) {
    const num = Number(value);
    if (!Number.isFinite(num)) return '--';
    if (num === 0) return '$0';
    return formatUSDCompact(num);
}

function formatRangePercent(value) {
    const num = Number(value);
    if (!Number.isFinite(num) || num <= 0) return '—';
    if (num >= 100) return `±${Math.round(num)}%`;
    if (num >= 10) return `±${num.toFixed(1).replace(/\.0$/, '')}%`;
    return `±${num.toFixed(2).replace(/0+$/, '').replace(/\.$/, '')}%`;
}

function formatRangePercentPlain(value) {
    const num = Number(value);
    if (!Number.isFinite(num) || num <= 0) return '--';
    if (num >= 100) return `${Math.round(num)}%`;
    if (num >= 10) return `${num.toFixed(1).replace(/\.0$/, '')}%`;
    return `${num.toFixed(2).replace(/0+$/, '').replace(/\.$/, '')}%`;
}

const POOL_CARD_RANGE_LIMIT = 5;
const POSITION_PREVIEW_STALE_MS = 30000;
const POSITION_PREVIEW_BATCH_SIZE = 4;
const POSITION_LIST_PAGE_SIZE = 6;
const WALLET_LIST_PAGE_SIZE = 10;
const USD_PREVIEW_FORMATTER = new Intl.NumberFormat('en-US', {
    style: 'currency',
    currency: 'USD',
    maximumFractionDigits: 2,
});

function getPositionSelectionKey(position) {
    const positionRef = String(position?.position_ref || '').trim();
    if (positionRef) return positionRef;
    const id = String(position?.id || '').trim();
    if (id) return id;
    const wallet = String(position?.wallet_address || '').trim().toLowerCase();
    const pool = String(position?.pool_address || '').trim().toLowerCase();
    const nft = String(position?.nft_token_id || '').trim();
    return [wallet, pool, nft].filter(Boolean).join(':');
}

function formatPreviewUsd(value) {
    const num = Number(value);
    if (!Number.isFinite(num)) return '--';
    return USD_PREVIEW_FORMATTER.format(num);
}

function useSmartMoneyPositionPreviewMap(apiBaseUrl, positions) {
    const [previewMap, setPreviewMap] = useState({});
    const previewRef = useRef(previewMap);

    useEffect(() => {
        previewRef.current = previewMap;
    }, [previewMap]);

    useEffect(() => {
        const rows = Array.isArray(positions) ? positions : [];
        if (rows.length === 0) return undefined;

        const now = Date.now();
        const pending = rows.filter((position) => {
            const key = getPositionSelectionKey(position);
            if (!key) return false;
            const cached = previewRef.current[key];
            return !cached || now - Number(cached.fetchedAt || 0) >= POSITION_PREVIEW_STALE_MS;
        });
        if (pending.length === 0) return undefined;

        let cancelled = false;

        const loadPreview = async (position) => {
            const key = getPositionSelectionKey(position);
            if (!key) return;
            try {
                const data = await fetchSMPositionDetail({
                    apiBaseUrl,
                    positionRef: position.position_ref,
                    positionId: position.id,
                });
                if (cancelled) return;
                setPreviewMap((prev) => ({
                    ...prev,
                    [key]: {
                        fetchedAt: Date.now(),
                        feeUsd: Number(data?.totals?.fee_usd ?? 0),
                        runningSince: String(data?.running_since || position?.opened_at || '').trim(),
                    },
                }));
            } catch (error) {
                if (cancelled) return;
                setPreviewMap((prev) => ({
                    ...prev,
                    [key]: {
                        ...(prev[key] || {}),
                        fetchedAt: Date.now(),
                        runningSince: String(prev[key]?.runningSince || position?.opened_at || '').trim(),
                    },
                }));
            }
        };

        (async () => {
            for (let index = 0; index < pending.length && !cancelled; index += POSITION_PREVIEW_BATCH_SIZE) {
                const batch = pending.slice(index, index + POSITION_PREVIEW_BATCH_SIZE);
                await Promise.all(batch.map((position) => loadPreview(position)));
            }
        })();

        return () => {
            cancelled = true;
        };
    }, [apiBaseUrl, positions]);

    return previewMap;
}

function getPoolCardRangeGroups(pool) {
    const groups = Array.isArray(pool?.range_groups)
        ? pool.range_groups.filter((item) => Number(item?.range_percent) > 0)
        : [];
    return groups;
}

function PoolCardRangeSummary({ pool }) {
    const [expanded, setExpanded] = useState(false);
    const groups = getPoolCardRangeGroups(pool);
    const visibleGroups = expanded ? groups : groups.slice(0, POOL_CARD_RANGE_LIMIT);
    const hiddenCount = Math.max(0, groups.length - visibleGroups.length);
    if (!groups.length) {
        return <div className="text-[10px] text-zinc-500">暂无聪明钱区间</div>;
    }
    return (
        <div className="flex min-w-0 flex-1 flex-col gap-1.5">
            {visibleGroups.map((group, index) => (
                <div
                    key={`${pool?.pool_address || 'pool'}:${Number(group?.range_percent || 0)}:${index}`}
                    className="inline-flex min-w-0 max-w-full items-center gap-1.5 self-start rounded-full border border-white/[0.05] bg-black/20 px-2.5 py-1 text-[10px] text-zinc-300"
                >
                    <span className="shrink-0 font-semibold text-zinc-100">{formatRangePercentPlain(group.range_percent)}</span>
                    {Math.max(0, Number(group?.position_count) || 0) > 1 ? (
                        <span className="shrink-0 rounded-full bg-white/[0.05] px-1.5 py-0.5 text-[9px] font-semibold text-zinc-200">
                            {Number(group.position_count)}仓
                        </span>
                    ) : null}
                    <span className="truncate text-zinc-400">{formatUSDCompact(group.total_amount_usd)}</span>
                </div>
            ))}
            {groups.length > POOL_CARD_RANGE_LIMIT ? (
                <button
                    type="button"
                    className="w-fit pl-1 text-[10px] text-lime-300/80 transition hover:text-lime-200"
                    onClick={(event) => {
                        event.stopPropagation();
                        setExpanded((prev) => !prev);
                    }}
                >
                    {expanded ? '收起区间' : `展开全部区间${hiddenCount > 0 ? ` (+${hiddenCount})` : ''}`}
                </button>
            ) : null}
        </div>
    );
}

function relativeTime(dateStr) {
    if (!dateStr) return '';
    const d = new Date(dateStr);
    const diff = (Date.now() - d.getTime()) / 1000;
    if (diff < 60) return '刚刚';
    if (diff < 3600) return `${Math.floor(diff / 60)}分钟前`;
    if (diff < 86400) return `${Math.floor(diff / 3600)}小时前`;
    return `${Math.floor(diff / 86400)}天前`;
}

function CopyButton({ text, small = false, className = '' }) {
    const [copied, setCopied] = useState(false);
    if (!text) return null;
    return (
        <button
            type="button"
            className={[
                'inline-flex items-center justify-center rounded-full text-zinc-500 transition hover:text-zinc-200',
                small ? 'h-5 w-5' : 'h-6 w-6',
                className,
            ].join(' ')}
            onClick={(e) => {
                e.stopPropagation();
                navigator.clipboard.writeText(text);
                setCopied(true);
                setTimeout(() => setCopied(false), 1200);
            }}
            title="复制"
        >
            {copied ? <Check size={small ? 10 : 12} /> : <Copy size={small ? 10 : 12} />}
        </button>
    );
}

function Badge({ children, className = '', style }) {
    return (
        <span
            className={`inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-[10px] ${className}`}
            style={style}
        >
            {children}
        </span>
    );
}

function WalletAvatar({ address, color, size = 36, avatarUrl }) {
    const fallbackSrc = WALLET_AVATAR_ICONS[walletAvatarIdx(address)] || WALLET_AVATAR_ICONS[0];
    const preferredSrc = resolveWalletAvatarSrc(address, avatarUrl);
    const [iconSrc, setIconSrc] = useState(preferredSrc);
    const strokeColor = color || '#7F77DD';

    useEffect(() => {
        setIconSrc(preferredSrc);
    }, [preferredSrc]);

    return (
        <span
            className="inline-flex shrink-0 items-center justify-center overflow-hidden rounded-[20px] border bg-zinc-950/80 p-px"
            style={{ width: size, height: size, borderColor: `${strokeColor}66` }}
        >
            <img
                src={iconSrc}
                alt=""
                className="h-full w-full rounded-[18px] object-cover"
                onError={() => {
                    if (iconSrc !== fallbackSrc) {
                        setIconSrc(fallbackSrc);
                    }
                }}
            />
        </span>
    );
}

function CompactIdentifier({ value, label = 'ID' }) {
    if (!value) return null;
    return (
        <span className="inline-flex items-center gap-1 rounded-full border border-white/[0.05] bg-zinc-900/70 px-2 py-1 text-[10px] text-zinc-400">
            <span className="text-zinc-500">{label}</span>
            <span className="font-mono text-zinc-200">{tailAddr(value)}</span>
            <CopyButton text={value} small />
        </span>
    );
}

function WalletIdentity({ address, color, label, avatarUrl, size = 40, onClick, showCopy = false }) {
    const inner = (
        <>
            <WalletAvatar address={address} color={color} avatarUrl={avatarUrl} size={size} />
            <span className="truncate text-left text-sm text-zinc-100">
                {label && label !== address ? label : shortAddr(address)}
            </span>
            {showCopy ? <CopyButton text={address} small className="shrink-0" /> : null}
        </>
    );

    if (typeof onClick === 'function') {
        return (
            <button
                type="button"
                className="flex min-w-0 items-center gap-2 rounded-xl text-left transition hover:text-zinc-100"
                onClick={(event) => {
                    event.stopPropagation();
                    onClick(event);
                }}
            >
                {inner}
            </button>
        );
    }

    return <div className="flex min-w-0 items-center gap-2">{inner}</div>;
}

function ProtocolBadge({ protocol }) {
    const info = PROTOCOL_MAP[protocol];
    return (
        <Badge className="border-white/10 bg-zinc-800/80 text-zinc-300" style={info ? { borderColor: `${info.color}40` } : undefined}>
            {info && <img src={info.icon} alt="" className="h-3.5 w-3.5 rounded-full" />}
            {info?.version || protocol}
        </Badge>
    );
}

function FeeBadge({ fee }) {
    if (!fee) return null;
    return <Badge className="border-white/10 bg-zinc-800/80 text-zinc-300">{formatFeeTier(fee)}</Badge>;
}

function PairAvatar({ item, size = 'md' }) {
    const displayTokenLogoUrl = String(item?.display_token_logo_url || '').trim();
    const displayTokenSymbol = String(item?.display_token_symbol || '').trim();
    const fallback = (displayTokenSymbol || getPairInitials(item) || 'LP').slice(0, 4).toUpperCase();
    const sizeClass = {
        sm: 'h-10 w-10 text-[11px]',
        md: 'h-11 w-11 text-xs',
        lg: 'h-12 w-12 text-sm',
    }[size] || 'h-11 w-11 text-xs';

    return (
        <span className={`relative inline-flex shrink-0 items-center justify-center overflow-hidden rounded-2xl border border-white/[0.05] bg-zinc-900/70 ${sizeClass}`}>
            {displayTokenLogoUrl ? (
                <>
                    <img
                        src={displayTokenLogoUrl}
                        alt=""
                        className="h-full w-full object-cover"
                        onError={(e) => {
                            e.currentTarget.style.display = 'none';
                            const fallbackNode = e.currentTarget.parentElement?.querySelector('.pair-avatar-fallback');
                            if (fallbackNode) fallbackNode.style.display = 'flex';
                        }}
                    />
                    <span className="pair-avatar-fallback hidden h-full w-full items-center justify-center bg-zinc-900 text-zinc-100">
                        {fallback}
                    </span>
                </>
            ) : (
                <span className="flex h-full w-full items-center justify-center bg-zinc-900 text-zinc-100">
                    {fallback}
                </span>
            )}
        </span>
    );
}

function StatCard({ label, value, color, compact = false, valueClassName = '' }) {
    return (
        <div className={`rounded-2xl border border-white/[0.04] bg-zinc-900/55 shadow-[0_12px_30px_-28px_rgba(0,0,0,0.9)] ${compact ? 'p-2.5' : 'p-3'}`}>
            <div className={`${compact ? 'text-[10px]' : 'text-[11px]'} mb-1 text-zinc-500`}>{label}</div>
            <div className={`${compact ? 'text-base leading-tight' : 'text-lg'} font-semibold break-words ${color || 'text-zinc-100'} ${valueClassName}`}>{value ?? '—'}</div>
        </div>
    );
}

function MiniMetric({ label, value }) {
    return (
        <div className="rounded-xl border border-white/[0.04] bg-zinc-950/45 px-2.5 py-2 text-center">
            <div className="text-[10px] text-zinc-500">{label}</div>
            <div className="mt-1 text-sm font-semibold text-zinc-100">{value ?? '—'}</div>
        </div>
    );
}

function PositionPreviewMetrics({ position, preview, compact = false }) {
    const runningText = formatDurationFrom(preview?.runningSince || position?.opened_at) || '--';
    const feeValue = Number(preview?.feeUsd);
    const feeText = Number.isFinite(feeValue) ? formatPreviewUsd(preview.feeUsd) : '--';
    const feeMetricClass = Number.isFinite(feeValue)
        ? (feeValue > 0
            ? 'border-emerald-500/20 bg-emerald-500/10 text-emerald-300'
            : feeValue < 0
                ? 'border-red-500/20 bg-red-500/10 text-red-300'
                : 'border-white/[0.05] bg-black/20 text-zinc-300')
        : 'border-white/[0.05] bg-black/20 text-zinc-300';
    const feeLabelClass = Number.isFinite(feeValue)
        ? (feeValue > 0 ? 'text-emerald-200' : feeValue < 0 ? 'text-red-200' : 'text-zinc-100')
        : 'text-zinc-100';
    const pnlMetricClass = 'hidden';
    const pnlLabelClass = '';
    const pnlText = '';
    const runtimeMetricClass = runningText !== '--'
        ? 'border-emerald-500/20 bg-emerald-500/10 text-emerald-300'
        : 'border-white/[0.05] bg-black/20 text-zinc-300';
    const runtimeLabelClass = runningText !== '--' ? 'text-emerald-200' : 'text-zinc-100';

    return (
        <div className={`mt-2 flex flex-wrap items-stretch gap-2 ${compact ? 'pt-2 border-t border-white/[0.05]' : ''}`}>
            <span className={`inline-flex min-w-[104px] items-center justify-between gap-2 whitespace-nowrap rounded-full border px-2.5 py-1 text-[10px] ${feeMetricClass}`}>
                <strong className={`font-semibold ${feeLabelClass}`}>手续费</strong>
                <span className="text-right tabular-nums">{feeText}</span>
            </span>
            <span className={`inline-flex min-w-[104px] items-center justify-between gap-2 whitespace-nowrap rounded-full border px-2.5 py-1 text-[10px] ${pnlMetricClass}`}>
                <strong className={`font-semibold ${pnlLabelClass}`}>绝对收益</strong>
                <span className="text-right tabular-nums">{pnlText}</span>
            </span>
            <span className={`inline-flex min-w-[104px] items-center justify-between gap-2 whitespace-nowrap rounded-full border px-2.5 py-1 text-[10px] ${runtimeMetricClass}`}>
                <strong className={`font-semibold ${runtimeLabelClass}`}>运行时间</strong>
                <span className="text-right tabular-nums">{runningText}</span>
            </span>
        </div>
    );
}

function PositionPagination({ page, total, brand, pageSize = POSITION_LIST_PAGE_SIZE, onChange }) {
    const totalPages = Math.max(1, Math.ceil(Number(total || 0) / pageSize));
    if (totalPages <= 1) return null;
    return (
        <div className="mt-3 flex items-center justify-center gap-2">
            <button
                type="button"
                className={`rounded-full px-3 py-1.5 text-[11px] ${getFilterButtonClass(false, brand)}`}
                disabled={page <= 1}
                onClick={() => onChange(page - 1)}
            >
                上一页
            </button>
            <span className={`rounded-full px-3 py-1.5 text-[11px] ${getFilterButtonClass(true, brand)}`}>
                {page} / {totalPages}
            </span>
            <button
                type="button"
                className={`rounded-full px-3 py-1.5 text-[11px] ${getFilterButtonClass(false, brand)}`}
                disabled={page >= totalPages}
                onClick={() => onChange(page + 1)}
            >
                下一页
            </button>
        </div>
    );
}

function SmartMoneyPositionDetailPanel({ apiBaseUrl, position, brand, onClose }) {
    const [detail, setDetail] = useState(null);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState('');

    useEffect(() => {
        if (!position) return undefined;

        let timerId = 0;
        let cancelled = false;

        const load = async (silent = false) => {
            if (!silent) {
                setLoading(true);
                setError('');
            }
            try {
                const data = await fetchSMPositionDetail({
                    apiBaseUrl,
                    positionRef: position.position_ref,
                    positionId: position.id,
                });
                if (cancelled) return;
                setDetail(data || null);
                setError('');
                const pollSec = Math.max(Number(data?.poll_interval_sec || 1), 1);
                timerId = window.setTimeout(() => {
                    load(true);
                }, pollSec * 1000);
            } catch (err) {
                if (cancelled) return;
                setError(String(err?.message || err || '详情加载失败'));
                timerId = window.setTimeout(() => {
                    load(true);
                }, 3000);
            } finally {
                if (!cancelled) {
                    setLoading(false);
                }
            }
        };

        load(false);

        return () => {
            cancelled = true;
            window.clearTimeout(timerId);
        };
    }, [apiBaseUrl, position]);

    if (!position) return null;

    return (
        <div className="mt-3 rounded-[28px] border border-white/[0.05] bg-zinc-950/82 p-3 shadow-[0_24px_80px_-42px_rgba(0,0,0,0.95)]">
            {error ? (
                <div className="mb-3 rounded-2xl border border-red-500/20 bg-red-500/10 px-3 py-2 text-sm text-red-200">
                    {error}
                </div>
            ) : null}

            {Array.isArray(detail?.warnings) && detail.warnings.length > 0 ? (
                <div className="mb-3 rounded-2xl border border-amber-500/20 bg-amber-500/10 px-3 py-2 text-xs text-amber-200">
                    {detail.warnings.join(' / ')}
                </div>
            ) : null}

            {loading && !detail ? (
                <div className="rounded-[24px] border border-white/[0.04] bg-zinc-900/55 px-4 py-10 text-center text-sm text-zinc-500">
                    正在读取链上仓位...
                </div>
            ) : detail ? (
                <PositionCard
                    position={detail}
                    walletAddress={detail.wallet_address}
                    updatedAt={detail.updated_at}
                    pollIntervalSec={detail.poll_interval_sec}
                    allowTaskActions={false}
                    showAbsolutePnl={false}
                    headerAccessory={(
                        <button
                            type="button"
                            onClick={onClose}
                            aria-label="收起详情"
                            className="inline-flex h-8 w-8 shrink-0 items-center justify-center rounded-xl border border-white/[0.06] bg-black/20 text-zinc-400 transition hover:bg-black/30 hover:text-zinc-200"
                        >
                            <X size={15} />
                        </button>
                    )}
                />
            ) : null}
        </div>
    );
}

// ============ PriceRangeChart ============
function PriceRangeChart({ positions, currentPrice }) {
    if (!positions || positions.length === 0) return null;

    const validPositions = positions.filter(p => p.price_lower && p.price_upper);
    if (validPositions.length === 0) return null;

    const allPrices = validPositions.flatMap(p => [parseFloat(p.price_lower), parseFloat(p.price_upper)]);
    let minP = Math.min(...allPrices);
    let maxP = Math.max(...allPrices);
    const padding = (maxP - minP) * 0.1 || 1;
    minP -= padding;
    maxP += padding;
    if (minP < 0) minP = 0;

    const priceToPct = (price) => Math.max(0, Math.min(100, ((price - minP) / (maxP - minP)) * 100));

    const parsedCurrentPrice = Number.parseFloat(currentPrice);
    const currentPct = Number.isFinite(parsedCurrentPrice) ? priceToPct(parsedCurrentPrice) : null;

    const walletCounts = {};
    validPositions.forEach(p => {
        walletCounts[p.wallet_address] = (walletCounts[p.wallet_address] || 0) + 1;
    });
    const walletIndices = {};

    const currentLabelStyle = currentPct === null
        ? null
        : currentPct >= 92
            ? { right: 0 }
            : currentPct <= 8
                ? { left: 0 }
                : { left: `${currentPct}%`, transform: 'translateX(-50%)' };

    return (
        <div className="mb-4 overflow-hidden rounded-2xl bg-zinc-800/40 p-3">
            <div className="relative w-full overflow-hidden pt-5" style={{ minHeight: validPositions.length * 14 + 50 }}>
                {/* Current price line */}
                {currentPct !== null && (
                    <div
                        className="absolute top-5 bottom-6 w-px bg-yellow-400/60 z-10"
                        style={{ left: `${currentPct}%` }}
                    >
                        <div
                            className="absolute -top-4 whitespace-nowrap text-[9px] text-yellow-400"
                            style={currentLabelStyle || undefined}
                        >
                            {currentPrice}
                        </div>
                    </div>
                )}

                {/* Position bars */}
                {validPositions.map((p, i) => {
                    const left = priceToPct(parseFloat(p.price_lower));
                    const right = priceToPct(parseFloat(p.price_upper));
                    const width = Math.max(right - left, 0.5);
                    const color = p.wallet_color || '#7F77DD';

                    const walletKey = p.wallet_address;
                    walletIndices[walletKey] = (walletIndices[walletKey] || 0) + 1;
                    const idx = walletIndices[walletKey];
                    const opacity = idx === 1 ? 0.85 : idx === 2 ? 0.6 : 0.4;

                    const inRange = currentPrice &&
                        parseFloat(p.price_lower) <= parseFloat(currentPrice) &&
                        parseFloat(currentPrice) <= parseFloat(p.price_upper);

                    return (
                        <div
                            key={p.id || i}
                            className="absolute h-[10px] rounded-[3px]"
                            style={{
                                left: `${left}%`,
                                width: `${width}%`,
                                top: i * 14 + 20,
                                backgroundColor: color,
                                opacity: inRange ? opacity : 0.35,
                            }}
                            title={`${shortAddr(p.wallet_address)}: ${p.price_lower} - ${p.price_upper}`}
                        />
                    );
                })}

                {/* X-axis labels */}
                <div className="absolute bottom-0 left-0 right-0 flex justify-between text-[9px] text-zinc-600">
                    {Array.from({ length: 5 }, (_, i) => {
                        const price = minP + ((maxP - minP) / 4) * i;
                        return <span key={i}>{price.toPrecision(4)}</span>;
                    })}
                </div>
            </div>

            {/* Legend */}
            <div className="flex flex-wrap gap-2 mt-3 text-[10px]">
                {Object.entries(
                    validPositions.reduce((acc, p) => {
                        if (!acc[p.wallet_address]) {
                            acc[p.wallet_address] = { color: p.wallet_color, label: p.wallet_label };
                        }
                        return acc;
                    }, {})
                ).map(([addr, { color, label }]) => (
                    <span key={addr} className="flex items-center gap-1 text-zinc-400">
                        <span className="inline-block w-2 h-2 rounded-full" style={{ backgroundColor: color }} />
                        {label || shortAddr(addr)}
                    </span>
                ))}
            </div>
        </div>
    );
}

function ConfirmDialog({ open, title, description, confirmLabel = '确认', busy = false, onConfirm, onCancel }) {
    if (!open) return null;

    return (
        <div className="fixed inset-0 z-50 flex items-end justify-center bg-black/60 p-4 sm:items-center" onClick={busy ? undefined : onCancel}>
            <div
                className="w-full max-w-sm rounded-[28px] border border-white/[0.05] bg-zinc-950/95 p-5 shadow-[0_24px_80px_-32px_rgba(0,0,0,0.95)]"
                onClick={(e) => e.stopPropagation()}
            >
                <div className="flex items-start justify-between gap-3">
                    <div>
                        <h3 className="text-base font-semibold text-zinc-100">{title}</h3>
                        <p className="mt-2 text-sm text-zinc-400">{description}</p>
                    </div>
                    <button
                        type="button"
                        onClick={onCancel}
                        disabled={busy}
                        className="inline-flex h-9 w-9 items-center justify-center rounded-xl border border-white/[0.05] bg-zinc-900/65 text-zinc-400 transition hover:text-zinc-200"
                    >
                        <X size={16} />
                    </button>
                </div>
                <div className="mt-5 flex gap-2">
                    <button
                        type="button"
                        onClick={onCancel}
                        disabled={busy}
                        className="flex-1 rounded-2xl border border-white/[0.05] bg-zinc-900/65 px-4 py-2.5 text-sm text-zinc-300 transition hover:bg-zinc-800/80"
                    >
                        取消
                    </button>
                    <button
                        type="button"
                        onClick={onConfirm}
                        disabled={busy}
                        className="flex-1 rounded-2xl border border-red-500/20 bg-red-500/10 px-4 py-2.5 text-sm text-red-200 transition hover:bg-red-500/15 disabled:opacity-50"
                    >
                        {busy ? '处理中...' : confirmLabel}
                    </button>
                </div>
            </div>
        </div>
    );
}

// ============ PAGES ============

function PoolListPage({ apiBaseUrl, onSelectPool, onOpenPosition, brand }) {
    const [pools, setPools] = useState([]);
    const [loading, setLoading] = useState(true);
    const [search, setSearch] = useState('');
    const [protocolFilter, setProtocolFilter] = useState('all');

    const load = useCallback((silent = false) => {
        if (!silent) {
            setLoading(true);
        }
        fetchSMPools({ apiBaseUrl })
            .then(d => setPools(d?.list || []))
            .catch(() => { })
            .finally(() => {
                if (!silent) {
                    setLoading(false);
                }
            });
    }, [apiBaseUrl]);

    useEffect(() => { load(); }, [load]);
    useEffect(() => {
        const timer = setInterval(() => {
            load(true);
        }, 10000);
        return () => clearInterval(timer);
    }, [load]);

    const filtered = useMemo(() => {
        let list = pools;
        if (search) {
            const q = search.toLowerCase();
            list = list.filter(p => getPairLabel(p).toLowerCase().includes(q) || getPoolIdentifier(p).toLowerCase().includes(q));
        }
        if (protocolFilter !== 'all') {
            list = list.filter(p => p.protocol === protocolFilter);
        }
        return list;
    }, [pools, search, protocolFilter]);

    return (
        <div>
            <div className="mb-3 flex gap-2">
                <div className="relative flex-1">
                    <Search size={14} className="absolute left-3 top-1/2 -translate-y-1/2 text-zinc-500" />
                    <input
                        className={getInputClass(brand).replace('px-3', 'pl-9 pr-3')}
                        placeholder="搜索池子..."
                        value={search}
                        onChange={e => setSearch(e.target.value)}
                    />
                </div>
            </div>
            <div className="mb-4 flex gap-1.5 overflow-x-auto pb-1 text-[11px]">
                {['all', 'pancake_v3', 'uniswap_v3', 'uniswap_v4'].map(p => {
                    const info = PROTOCOL_MAP[p];
                    return (
                        <button
                            key={p}
                            type="button"
                            className={`inline-flex shrink-0 items-center gap-1 rounded-full px-3 py-1.5 ${getFilterButtonClass(protocolFilter === p, brand)}`}
                            onClick={() => setProtocolFilter(p)}
                        >
                            {info && <img src={info.icon} alt="" className="h-3.5 w-3.5 rounded-full" />}
                            {p === 'all' ? '全部' : info?.version || p}
                        </button>
                    );
                })}
            </div>

            {loading ? (
                <div className="py-8 text-center text-zinc-500">加载中...</div>
            ) : filtered.length === 0 ? (
                <div className="rounded-2xl border border-dashed border-white/[0.05] bg-zinc-900/45 px-4 py-8 text-center text-sm text-zinc-500">
                    暂无活跃仓位的池子
                </div>
            ) : (
                <div className="space-y-3">
                    {filtered.map(pool => (
                        <button
                            key={pool.pool_address}
                            type="button"
                            className="w-full rounded-[24px] border border-white/[0.04] bg-zinc-900/60 p-3 text-left shadow-[0_18px_50px_-32px_rgba(0,0,0,0.95)] transition active:scale-[0.995]"
                            onClick={() => onSelectPool(pool)}
                        >
                            <div className="flex items-start gap-3">
                                <PairAvatar item={pool} size="md" />
                                <div className="min-w-0 flex-1">
                                    <div className="flex flex-wrap items-center gap-1.5">
                                        <span className="truncate text-sm font-semibold text-zinc-100">{getPairLabel(pool)}</span>
                                        <ProtocolBadge protocol={pool.protocol} />
                                        <FeeBadge fee={pool.fee_tier} />
                                    </div>
                                    <div className="mt-2 flex flex-wrap items-center gap-1.5">
                                        <CompactIdentifier value={getPoolIdentifier(pool)} label="池子" />
                                        <Badge className="border-white/10 bg-zinc-800/80 text-zinc-200">
                                            总仓位 {Number(pool.total_position_amount_usd) > 0 ? formatUSDCompact(pool.total_position_amount_usd) : '--'}
                                        </Badge>
                                    </div>
                                    <div className="mt-3 flex flex-wrap items-center gap-2 text-[11px] text-zinc-500">
                                        <span>{pool.wallet_count} 钱包</span>
                                        <span className="text-zinc-700">·</span>
                                        <span>{pool.open_position_count} 仓位</span>
                                        <span className="text-zinc-700">·</span>
                                        <span className={`inline-flex items-center gap-1 ${pool.latest_event_at && (Date.now() - new Date(pool.latest_event_at).getTime()) < 120000 ? 'text-green-300' : ''}`}>
                                            <span className={`inline-block h-1.5 w-1.5 rounded-full ${pool.latest_event_at && (Date.now() - new Date(pool.latest_event_at).getTime()) < 120000 ? 'bg-green-400' : 'bg-zinc-600'}`} />
                                            {relativeTime(pool.latest_event_at)}
                                        </span>
                                    </div>
                                    <div className="mt-1.5">
                                        <PoolCardRangeSummary pool={pool} />
                                    </div>
                                </div>
                                {typeof onOpenPosition === 'function' ? (
                                    <button
                                        type="button"
                                        className={`mt-1 inline-flex h-6 shrink-0 items-center gap-1 rounded-full px-2 text-[10px] font-semibold leading-none shadow-sm ${brand.solidButtonClass} ${brand.solidRingClass}`}
                                        onClick={(event) => {
                                            event.stopPropagation();
                                            onOpenPosition(pool);
                                        }}
                                    >
                                        <FlashIcon className="h-2.5 w-2.5 shrink-0" />
                                        跟单
                                    </button>
                                ) : (
                                    <ChevronRight size={16} className="mt-1 shrink-0 text-zinc-600" />
                                )}
                            </div>
                        </button>
                    ))}
                </div>
            )}
        </div>
    );
}

function PoolDetailPage({ apiBaseUrl, pool, onBack, onSelectWallet, brand }) {
    const [positions, setPositions] = useState([]);
    const [positionsTotal, setPositionsTotal] = useState(0);
    const [poolStats, setPoolStats] = useState(null);
    const [status, setStatus] = useState('open');
    const [page, setPage] = useState(1);
    const [loading, setLoading] = useState(true);
    const [selectedPosition, setSelectedPosition] = useState(null);
    const poolChain = resolvePoolChain(pool);
    const poolGmgnUrl = useMemo(() => buildGmgnUrl({ ...pool, chain: poolChain }, poolChain), [pool, poolChain]);
    const positionPreviews = useSmartMoneyPositionPreviewMap(apiBaseUrl, positions);

    useEffect(() => {
        fetchSMPoolStats({ apiBaseUrl, poolAddress: pool.pool_address }).then(setPoolStats).catch(() => { });
    }, [apiBaseUrl, pool.pool_address]);

    useEffect(() => {
        setLoading(true);
        fetchSMPositions({
            apiBaseUrl,
            pool: pool.pool_address,
            status,
            page,
            size: POSITION_LIST_PAGE_SIZE,
            orderBy: 'position_amount_desc',
        })
            .then(d => {
                setPositions(d?.list || []);
                setPositionsTotal(Number(d?.total || 0));
            })
            .catch(() => { })
            .finally(() => setLoading(false));
    }, [apiBaseUrl, page, pool.pool_address, status]);

    useEffect(() => {
        setPage(1);
    }, [pool.pool_address, status]);

    useEffect(() => {
        if (!selectedPosition) return;
        const selectedKey = getPositionSelectionKey(selectedPosition);
        if (positions.some((pos) => getPositionSelectionKey(pos) === selectedKey)) return;
        setSelectedPosition(null);
    }, [positions, selectedPosition]);
    const selectedPositionKey = selectedPosition ? getPositionSelectionKey(selectedPosition) : '';

    return (
        <div>
            <button
                type="button"
                onClick={onBack}
                className="mb-4 inline-flex items-center gap-1.5 rounded-full border border-white/[0.05] bg-zinc-900/65 px-3 py-1.5 text-sm text-zinc-300 transition hover:bg-zinc-800/80"
            >
                <ChevronLeft size={14} />
                返回池子列表
            </button>

            <div className="mb-4 rounded-[24px] border border-white/[0.04] bg-zinc-900/60 p-4">
                <div className="flex items-start gap-3">
                    <PairAvatar item={pool} size="lg" />
                    <div className="min-w-0 flex-1">
                        <div className="flex flex-wrap items-center gap-1.5">
                            <span className="truncate text-lg font-semibold text-zinc-100">{getPairLabel(pool)}</span>
                            <ProtocolBadge protocol={pool.protocol} />
                            <FeeBadge fee={pool.fee_tier} />
                        </div>
                        <div className="mt-2 flex flex-wrap items-center gap-1.5">
                            <CompactIdentifier value={getPoolIdentifier(pool)} label="池子" />
                            {poolGmgnUrl ? (
                                <a
                                    href={poolGmgnUrl}
                                    target="_blank"
                                    rel="noopener noreferrer"
                                    className={`inline-flex items-center gap-1 text-xs ${getBrandLinkClass(brand)}`}
                                    title="GMGN"
                                >
                                    <img src={gmgnIcon} alt="GMGN" className="h-3.5 w-3.5" />
                                    <span>GMGN</span>
                                </a>
                            ) : null}
                        </div>
                    </div>
                </div>
            </div>

            {poolStats && (
                <div className="grid grid-cols-2 gap-1.5 mb-4">
                    <StatCard label="当前价格" value={poolStats.current_price || '—'} compact valueClassName="text-[13px]" />
                    <StatCard label="钱包数" value={poolStats.wallet_count} compact />
                    <StatCard label="持仓笔数" value={poolStats.open_position_count} compact />
                    <StatCard label="今日关闭" value={poolStats.closed_today_count} color="text-red-400" compact />
                </div>
            )}

            <PriceRangeChart
                positions={positions}
                currentPrice={poolStats?.current_price}
            />

            <div className="flex items-center justify-between mb-3 gap-3">
                <div>
                    <div className="text-sm font-medium text-zinc-100">仓位列表</div>
                    <div className="text-[11px] text-zinc-500">按钱包查看该池子的聪明钱持仓</div>
                </div>
                <div className="flex gap-1 text-[11px]">
                    {['open', 'all'].map(s => (
                        <button
                            key={s}
                            type="button"
                            className={`rounded-full px-3 py-1.5 ${getFilterButtonClass(status === s, brand)}`}
                            onClick={() => setStatus(s)}
                        >
                            {s === 'open' ? '持仓中' : '全部'}
                        </button>
                    ))}
                </div>
            </div>

            {loading ? (
                <div className="py-6 text-center text-zinc-500">加载中...</div>
            ) : positions.length === 0 ? (
                <div className="rounded-2xl border border-dashed border-white/[0.05] bg-zinc-900/45 px-4 py-8 text-center text-sm text-zinc-500">
                    {status === 'open'
                        ? '全部已关闭，切换到“全部”查看历史记录'
                        : '暂无仓位数据'}
                </div>
            ) : (
                <div className="space-y-3">
                    {positions.map(pos => {
                        const positionKey = getPositionSelectionKey(pos) || String(pos.id || '');
                        const isSelected = Boolean(positionKey) && positionKey === selectedPositionKey;
                        return (
                            <div key={positionKey || pos.id}>
                                <div
                                    className={`rounded-[22px] border border-white/[0.04] bg-zinc-900/60 p-3 transition active:scale-[0.995] ${pos.status === 'closed' ? 'opacity-70' : ''}`}
                                    onClick={() => setSelectedPosition(pos)}
                                >
                                    <div className="flex items-start justify-between gap-3">
                                        <div className="min-w-0 flex-1">
                                            <WalletIdentity
                                                address={pos.wallet_address}
                                                color={pos.wallet_color}
                                                label={pos.wallet_label || pos.wallet_address}
                                                avatarUrl={pos.wallet_avatar_url}
                                                onClick={() => onSelectWallet(pos.wallet_address)}
                                            />
                                        </div>
                                        <div className="flex flex-col items-end gap-1">
                                            <div className="text-sm font-semibold text-zinc-100">{formatUSDCompact(pos.position_amount_usd)}</div>
                                            <Badge className={pos.status === 'open'
                                                ? 'border-emerald-500/20 bg-emerald-500/10 text-emerald-300'
                                                : 'border-white/10 bg-zinc-800/80 text-zinc-400'}>
                                                {pos.status === 'open' ? '持仓中' : '已关闭'}
                                            </Badge>
                                        </div>
                                    </div>

                                    <div className="mt-3 flex items-end justify-between gap-3">
                                        <div className="min-w-0">
                                            <div className={`truncate font-mono text-[11px] text-zinc-300 ${pos.status === 'closed' ? 'line-through opacity-70' : ''}`}>
                                                {pos.price_lower && pos.price_upper ? `${pos.price_lower} - ${pos.price_upper}` : '—'}
                                            </div>
                                            <div className="mt-1 flex flex-wrap items-center gap-2 text-[10px] text-zinc-500">
                                                <span>NFT #{pos.nft_token_id || '--'}</span>
                                                {Number(pos.range_percent) > 0 ? <span>{formatRangePercent(pos.range_percent)}</span> : null}
                                            </div>
                                        </div>
                                        {pos.bscscan_url ? (
                                            <a
                                                href={pos.bscscan_url}
                                                target="_blank"
                                                rel="noopener noreferrer"
                                                className={`inline-flex items-center gap-1 text-xs ${getBrandLinkClass(brand)}`}
                                                onClick={(event) => event.stopPropagation()}
                                            >
                                                查看交易
                                                <ExternalLink size={10} />
                                            </a>
                                        ) : null}
                                    </div>
                                    <PositionPreviewMetrics
                                        position={pos}
                                        preview={positionPreviews[getPositionSelectionKey(pos)]}
                                    />
                                </div>
                                {isSelected ? (
                                    <SmartMoneyPositionDetailPanel
                                        apiBaseUrl={apiBaseUrl}
                                        position={selectedPosition}
                                        brand={brand}
                                        onClose={() => setSelectedPosition(null)}
                                    />
                                ) : null}
                            </div>
                        );
                    })}
                </div>
            )}
            <PositionPagination page={page} total={positionsTotal} brand={brand} onChange={setPage} />
        </div>
    );
}

function WalletListPage({ apiBaseUrl, onSelectWallet, onAddWallet, brand, refreshKey }) {
    const [wallets, setWallets] = useState([]);
    const [walletsTotal, setWalletsTotal] = useState(0);
    const [loading, setLoading] = useState(true);
    const [search, setSearch] = useState('');
    const [page, setPage] = useState(1);
    const [busyKey, setBusyKey] = useState('');
    const [actionError, setActionError] = useState('');
    const [confirmState, setConfirmState] = useState(null);
    const [editingWallet, setEditingWallet] = useState(null);
    const loadSeqRef = useRef(0);
    const searchKeyword = useMemo(() => String(search || '').trim(), [search]);

    const load = useCallback((silent = false) => {
        const seq = ++loadSeqRef.current;
        if (!silent) {
            setLoading(true);
        }
        fetchSMWallets({
            apiBaseUrl,
            page,
            size: WALLET_LIST_PAGE_SIZE,
            keyword: searchKeyword || undefined,
        })
            .then(d => {
                if (seq !== loadSeqRef.current) return;
                const total = Number(d?.total || 0);
                const list = Array.isArray(d?.list) ? d.list : [];
                const totalPages = Math.max(1, Math.ceil(total / WALLET_LIST_PAGE_SIZE));
                if (page > totalPages) {
                    setPage(totalPages);
                    return;
                }
                setWallets(list);
                setWalletsTotal(total);
            })
            .catch(() => { })
            .finally(() => {
                if (!silent && seq === loadSeqRef.current) {
                    setLoading(false);
                }
            });
    }, [apiBaseUrl, page, searchKeyword]);

    useEffect(() => { load(); }, [load, refreshKey]);

    useEffect(() => {
        const timer = setInterval(() => {
            load(true);
        }, 10000);
        return () => clearInterval(timer);
    }, [load]);

    const handleToggle = async (wallet) => {
        setBusyKey(`wallet-toggle:${wallet.address}`);
        setActionError('');
        try {
            await updateSMWallet({ apiBaseUrl, address: wallet.address, updates: { is_active: !wallet.is_active } });
            await load();
        } catch (err) {
            setActionError(err?.message || '操作失败');
        } finally {
            setBusyKey('');
        }
    };

    const confirmDelete = async () => {
        if (!confirmState) return;
        setBusyKey(confirmState.key);
        setActionError('');
        try {
            await confirmState.action();
            await load();
            setConfirmState(null);
        } catch (err) {
            setConfirmState(null);
            setActionError(err?.message || '操作失败');
        } finally {
            setBusyKey('');
        }
    };

    return (
        <div>
            <div className="mb-3 flex gap-2">
                <div className="relative flex-1">
                    <Search size={14} className="absolute left-3 top-1/2 -translate-y-1/2 text-zinc-500" />
                    <input
                        className={getInputClass(brand).replace('px-3', 'pl-9 pr-3')}
                        placeholder="搜索钱包..."
                        value={search}
                        onChange={e => {
                            setSearch(e.target.value);
                            setPage(1);
                        }}
                    />
                </div>
                <button
                    onClick={onAddWallet}
                    className={`${brand.solidButtonClass} ${brand.solidRingClass} inline-flex shrink-0 items-center gap-1 rounded-2xl px-3 py-2 text-sm`}
                >
                    <Plus size={14} /> 添加
                </button>
            </div>

            {actionError ? (
                <div className="mb-3 rounded-2xl border border-red-500/20 bg-red-500/10 px-3 py-2 text-sm text-red-200">
                    {actionError}
                </div>
            ) : null}

            {loading ? (
                <div className="py-8 text-center text-zinc-500">加载中...</div>
            ) : wallets.length === 0 ? (
                <div className="rounded-2xl border border-dashed border-white/[0.05] bg-zinc-900/45 px-4 py-8 text-center text-sm text-zinc-500">
                    暂无监控钱包，点击“添加”开始
                </div>
            ) : (
                <div className="space-y-3">
                    {wallets.map(w => (
                        <button
                            key={w.address}
                            type="button"
                            className="w-full rounded-[24px] border border-white/[0.04] bg-zinc-900/60 p-3 text-left shadow-[0_18px_50px_-32px_rgba(0,0,0,0.95)] transition active:scale-[0.995]"
                            onClick={() => onSelectWallet(w.address)}
                        >
                            <div className="flex items-start justify-between gap-3">
                                <div className="min-w-0 flex-1">
                                    <WalletIdentity
                                        address={w.address}
                                        color={w.color}
                                        label={w.label || w.address}
                                        avatarUrl={w.avatar_url}
                                        size={44}
                                        showCopy
                                    />
                                    <div className="mt-2 flex flex-wrap items-center gap-1.5">
                                        <Badge className="border-white/10 bg-zinc-800/80 text-zinc-300">
                                            {w.source === 'manual' ? '手动' : '合约'}
                                        </Badge>
                                        <Badge className={w.is_active
                                            ? 'border-emerald-500/20 bg-emerald-500/10 text-emerald-300'
                                            : 'border-white/10 bg-zinc-800/80 text-zinc-400'}>
                                            {w.is_active ? '监控中' : '已暂停'}
                                        </Badge>
                                        {w.last_active_at ? (
                                            <Badge className="border-white/10 bg-zinc-800/80 text-zinc-400">
                                                {relativeTime(w.last_active_at)}
                                            </Badge>
                                        ) : null}
                                    </div>
                                </div>
                                <div className="flex shrink-0 gap-1">
                                    <button
                                        type="button"
                                        className={getIconButtonClass(false)}
                                        disabled={busyKey === `wallet-toggle:${w.address}` || busyKey === `wallet-delete:${w.address}`}
                                        onClick={e => {
                                            e.stopPropagation();
                                            setEditingWallet(w);
                                        }}
                                    >
                                        <Pencil size={14} />
                                    </button>
                                    <button
                                        type="button"
                                        className={getIconButtonClass(false)}
                                        disabled={busyKey === `wallet-toggle:${w.address}` || busyKey === `wallet-delete:${w.address}`}
                                        onClick={e => { e.stopPropagation(); handleToggle(w); }}
                                    >
                                        {w.is_active ? <Pause size={14} /> : <Play size={14} />}
                                    </button>
                                    <button
                                        type="button"
                                        className={getIconButtonClass(true)}
                                        disabled={busyKey === `wallet-toggle:${w.address}` || busyKey === `wallet-delete:${w.address}`}
                                        onClick={e => {
                                            e.stopPropagation();
                                            setConfirmState({
                                                key: `wallet-delete:${w.address}`,
                                                title: '删除钱包',
                                                description: `确认删除钱包 ${shortAddr(w.address)} 吗？`,
                                                action: () => deleteSMWallet({ apiBaseUrl, address: w.address }),
                                            });
                                        }}
                                    >
                                        <Trash2 size={14} />
                                    </button>
                                </div>
                            </div>

                            <div className="mt-3 grid grid-cols-4 gap-2">
                                <MiniMetric label="余额" value={formatWalletBalance(w.wallet_balance_usd)} />
                                <MiniMetric label="持仓" value={w.open_position_count} />
                                <MiniMetric label="池子" value={w.active_pool_count} />
                                <MiniMetric label="末尾" value={tailAddr(w.address)} />
                            </div>
                        </button>
                    ))}
                </div>
            )}

            <PositionPagination page={page} total={walletsTotal} brand={brand} pageSize={WALLET_LIST_PAGE_SIZE} onChange={setPage} />

            <ConfirmDialog
                open={Boolean(confirmState)}
                title={confirmState?.title || '确认操作'}
                description={confirmState?.description || ''}
                confirmLabel="删除"
                busy={busyKey.startsWith('wallet-delete:')}
                onCancel={() => {
                    if (!busyKey.startsWith('wallet-delete:')) setConfirmState(null);
                }}
                onConfirm={confirmDelete}
            />
            <EditWalletModal
                open={Boolean(editingWallet)}
                apiBaseUrl={apiBaseUrl}
                wallet={editingWallet}
                brand={brand}
                onClose={() => setEditingWallet(null)}
                onSaved={async () => {
                    await load();
                    setEditingWallet(null);
                }}
            />
        </div>
    );
}

function WalletDetailPage({ apiBaseUrl, walletAddress, onBack, onSelectPool, brand }) {
    const [positions, setPositions] = useState([]);
    const [positionsTotal, setPositionsTotal] = useState(0);
    const [walletInfo, setWalletInfo] = useState(null);
    const [status, setStatus] = useState('open');
    const [page, setPage] = useState(1);
    const [loading, setLoading] = useState(true);
    const [selectedPosition, setSelectedPosition] = useState(null);
    const positionPreviews = useSmartMoneyPositionPreviewMap(apiBaseUrl, positions);

    useEffect(() => {
        fetchSMStats({ apiBaseUrl, address: walletAddress }).then(setWalletInfo).catch(() => { });
    }, [apiBaseUrl, walletAddress]);

    useEffect(() => {
        setLoading(true);
        fetchSMPositions({
            apiBaseUrl,
            wallet: walletAddress,
            status,
            page,
            size: POSITION_LIST_PAGE_SIZE,
            orderBy: 'position_amount_desc',
        })
            .then(d => {
                setPositions(d?.list || []);
                setPositionsTotal(Number(d?.total || 0));
            })
            .catch(() => { })
            .finally(() => setLoading(false));
    }, [apiBaseUrl, page, walletAddress, status]);

    useEffect(() => {
        setPage(1);
    }, [walletAddress, status]);

    useEffect(() => {
        if (!selectedPosition) return;
        const selectedKey = getPositionSelectionKey(selectedPosition);
        if (positions.some((pos) => getPositionSelectionKey(pos) === selectedKey)) return;
        setSelectedPosition(null);
    }, [positions, selectedPosition]);
    const selectedPositionKey = selectedPosition ? getPositionSelectionKey(selectedPosition) : '';

    // Group positions by pool
    const poolGroups = useMemo(() => {
        const groups = {};
        (positions || []).forEach(p => {
            if (!groups[p.pool_address]) {
                groups[p.pool_address] = {
                    pool_address: p.pool_address,
                    token0_symbol: p.token0_symbol,
                    token1_symbol: p.token1_symbol,
                    trading_pair: p.trading_pair,
                    display_token_address: p.display_token_address,
                    display_token_symbol: p.display_token_symbol,
                    display_token_logo_url: p.display_token_logo_url,
                    fee_tier: p.fee_tier,
                    protocol: p.protocol,
                    positions: [],
                    hasOpen: false,
                };
            }
            groups[p.pool_address].positions.push(p);
            if (p.status === 'open') groups[p.pool_address].hasOpen = true;
        });
        return Object.values(groups).sort((a, b) => {
            if (a.hasOpen && !b.hasOpen) return -1;
            if (!a.hasOpen && b.hasOpen) return 1;
            return 0;
        });
    }, [positions]);

    return (
        <div>
            <button
                type="button"
                onClick={onBack}
                className="mb-4 inline-flex items-center gap-1.5 rounded-full border border-white/[0.05] bg-zinc-900/65 px-3 py-1.5 text-sm text-zinc-300 transition hover:bg-zinc-800/80"
            >
                <ChevronLeft size={14} />
                返回钱包列表
            </button>

            {walletInfo && (
                <div className="mb-4 rounded-[24px] border border-white/[0.04] bg-zinc-900/60 p-4">
                    <div className="flex items-start gap-3">
                        <WalletAvatar address={walletAddress} color={walletInfo.color || '#7F77DD'} avatarUrl={walletInfo.avatar_url} size={72} />
                        <div className="min-w-0 flex-1">
                            <div className="truncate text-lg font-semibold text-zinc-100">
                                {walletInfo.label || `钱包 ${tailAddr(walletAddress)}`}
                            </div>
                            <div className="mt-2 flex flex-wrap items-center gap-1.5">
                                <CompactIdentifier value={walletAddress} label="钱包" />
                                <Badge className="border-white/10 bg-zinc-800/80 text-zinc-300">
                                    {walletInfo.source === 'manual' ? '手动' : '合约'}
                                </Badge>
                                <Badge className={walletInfo.is_active
                                    ? 'border-emerald-500/20 bg-emerald-500/10 text-emerald-300'
                                    : 'border-white/10 bg-zinc-800/80 text-zinc-400'}>
                                    {walletInfo.is_active ? '监控中' : '已暂停'}
                                </Badge>
                            </div>
                        </div>
                    </div>

                    <div className="grid grid-cols-2 gap-1.5 mt-3">
                        <StatCard label="钱包余额" value={formatWalletBalance(walletInfo.wallet_balance_usd)} compact />
                        <StatCard label="持仓笔数" value={walletInfo.open_position_count} compact />
                        <StatCard label="活跃池子" value={walletInfo.active_pool_count} compact />
                        <StatCard label="总添加次数" value={walletInfo.total_add_count} compact />
                        <StatCard label="总移除次数" value={walletInfo.total_remove_count} compact />
                    </div>
                </div>
            )}

            <div className="flex items-center justify-between mb-3 gap-3">
                <div>
                    <div className="text-sm font-medium text-zinc-100">按池子分组</div>
                    <div className="text-[11px] text-zinc-500">集中查看该钱包在不同池子的 LP 行为</div>
                </div>
                <div className="flex gap-1 text-[11px]">
                    {['open', 'all'].map(s => (
                        <button
                            key={s}
                            type="button"
                            className={`rounded-full px-3 py-1.5 ${getFilterButtonClass(status === s, brand)}`}
                            onClick={() => setStatus(s)}
                        >
                            {s === 'open' ? '持仓中' : '全部'}
                        </button>
                    ))}
                </div>
            </div>

            {loading ? (
                <div className="py-6 text-center text-zinc-500">加载中...</div>
            ) : poolGroups.length === 0 ? (
                <div className="rounded-2xl border border-dashed border-white/[0.05] bg-zinc-900/45 px-4 py-8 text-center text-sm text-zinc-500">
                    暂未检测到 LP 活动
                </div>
            ) : (
                <div className="space-y-3">
                    {poolGroups.map(group => (
                        <PoolGroupCard
                            key={group.pool_address}
                            group={group}
                            brand={brand}
                            positionPreviews={positionPreviews}
                            selectedPositionKey={selectedPositionKey}
                            onOpenPositionDetail={setSelectedPosition}
                            detailPanel={selectedPosition ? (
                                <SmartMoneyPositionDetailPanel
                                    apiBaseUrl={apiBaseUrl}
                                    position={selectedPosition}
                                    brand={brand}
                                    onClose={() => setSelectedPosition(null)}
                                />
                            ) : null}
                            onSelectPool={() => onSelectPool({
                                pool_address: group.pool_address,
                                token0_symbol: group.token0_symbol,
                                token1_symbol: group.token1_symbol,
                                trading_pair: group.trading_pair,
                                display_token_address: group.display_token_address,
                                display_token_symbol: group.display_token_symbol,
                                display_token_logo_url: group.display_token_logo_url,
                                fee_tier: group.fee_tier,
                                protocol: group.protocol,
                            })}
                        />
                    ))}
                </div>
            )}
            <PositionPagination page={page} total={positionsTotal} brand={brand} onChange={setPage} />
        </div>
    );
}

function PoolGroupCard({ group, onSelectPool, onOpenPositionDetail, brand, positionPreviews, selectedPositionKey = '', detailPanel = null }) {
    const [collapsed, setCollapsed] = useState(!group.hasOpen);
    const openCount = group.positions.filter(p => p.status === 'open').length;
    const closedCount = group.positions.filter(p => p.status === 'closed').length;

    return (
        <div className={`rounded-[24px] border border-white/[0.04] bg-zinc-900/60 overflow-hidden ${!group.hasOpen ? 'opacity-70' : ''}`}>
            <div
                className="flex items-start justify-between gap-3 p-3 cursor-pointer hover:bg-zinc-800/40"
                onClick={() => setCollapsed(!collapsed)}
            >
                <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-2">
                        {collapsed ? <ChevronRight size={14} className="text-zinc-500" /> : <ChevronDown size={14} className="text-zinc-500" />}
                        <PairAvatar item={group} size="sm" />
                        <span className="truncate text-sm font-medium text-zinc-100">{getPairLabel(group)}</span>
                    </div>
                    <div className="mt-2 flex flex-wrap items-center gap-1.5 pl-6">
                        <CompactIdentifier value={group.pool_address} label="池子" />
                        <ProtocolBadge protocol={group.protocol} />
                        <FeeBadge fee={group.fee_tier} />
                    </div>
                </div>
                <div className="flex shrink-0 flex-col items-end gap-1 text-right">
                    <span className="text-[11px] text-zinc-500">
                        {group.hasOpen ? `${openCount} 个仓位` : `${closedCount} 已关闭`}
                    </span>
                    <button
                        type="button"
                        className={`inline-flex items-center gap-1 text-[11px] ${getBrandLinkClass(brand)}`}
                        onClick={e => { e.stopPropagation(); onSelectPool(); }}
                    >
                        池子详情 <ExternalLink size={10} />
                    </button>
                </div>
            </div>
            {!collapsed && (
                <div className="px-3 pb-3 space-y-2">
                    {group.positions.map(pos => {
                        const positionKey = getPositionSelectionKey(pos) || String(pos.id || '');
                        const isSelected = Boolean(positionKey) && positionKey === selectedPositionKey;
                        return (
                            <div key={positionKey || pos.id}>
                                <div
                                    className={`rounded-2xl border border-white/[0.04] bg-zinc-950/45 px-3 py-2.5 transition active:scale-[0.995] ${pos.status === 'closed' ? 'opacity-70' : ''}`}
                                    onClick={() => onOpenPositionDetail?.(pos)}
                                >
                                    <div className="flex items-center justify-between gap-3">
                                        <span className="text-sm font-semibold text-zinc-100">{formatUSDCompact(pos.position_amount_usd)}</span>
                                        <Badge className={pos.status === 'open'
                                            ? 'border-emerald-500/20 bg-emerald-500/10 text-emerald-300'
                                            : 'border-white/10 bg-zinc-800/80 text-zinc-400'}>
                                            {pos.status === 'open' ? '持仓中' : '已关闭'}
                                        </Badge>
                                    </div>
                                    <div className="mt-2 flex items-end justify-between gap-3">
                                        <div className="min-w-0">
                                            <div className={`truncate font-mono text-[11px] text-zinc-300 ${pos.status === 'closed' ? 'line-through opacity-70' : ''}`}>
                                                {pos.price_lower && pos.price_upper ? `${pos.price_lower} - ${pos.price_upper}` : '—'}
                                            </div>
                                            <div className="mt-1 flex flex-wrap items-center gap-2 text-[10px] text-zinc-500">
                                                <span>NFT #{pos.nft_token_id || '--'}</span>
                                                {Number(pos.range_percent) > 0 ? <span>{formatRangePercent(pos.range_percent)}</span> : null}
                                            </div>
                                        </div>
                                        {pos.bscscan_url ? (
                                            <a
                                                href={pos.bscscan_url}
                                                target="_blank"
                                                rel="noopener noreferrer"
                                                className={`inline-flex items-center gap-1 text-xs ${getBrandLinkClass(brand)}`}
                                                onClick={(event) => event.stopPropagation()}
                                            >
                                                查看交易
                                                <ExternalLink size={10} />
                                            </a>
                                        ) : null}
                                    </div>
                                    <PositionPreviewMetrics
                                        position={pos}
                                        preview={positionPreviews?.[getPositionSelectionKey(pos)]}
                                        compact
                                    />
                                </div>
                                {isSelected ? detailPanel : null}
                            </div>
                        );
                    })}
                </div>
            )}
        </div>
    );
}

function SettingsPage({ apiBaseUrl, brand }) {
    const [tab, setTab] = useState('wallets');

    return (
        <div>
            <div className="flex gap-1 mb-4 text-[12px]">
                {['wallets', 'contracts'].map(t => (
                    <button
                        key={t}
                        className={`px-3 py-1.5 rounded ${tab === t ? brand.softButtonClass : 'bg-zinc-800 text-zinc-400'}`}
                        onClick={() => setTab(t)}
                    >
                        {t === 'wallets' ? '钱包' : '合约'}
                    </button>
                ))}
            </div>
            {tab === 'wallets' ? (
                <WalletSettingsTab apiBaseUrl={apiBaseUrl} brand={brand} />
            ) : (
                <ContractSettingsTab apiBaseUrl={apiBaseUrl} brand={brand} />
            )}
        </div>
    );
}

function ContractSettingsPage({ apiBaseUrl, brand }) {
    return <ContractSettingsTab apiBaseUrl={apiBaseUrl} brand={brand} />;
}

function GoldenDogPage({ apiBaseUrl, initData, brand }) {
    return <GoldenDogPageContent apiBaseUrl={apiBaseUrl} initData={initData} brand={brand} />;

    const hasInitData = Boolean(String(initData || '').trim());
    const [loading, setLoading] = useState(hasInitData);
    const [saving, setSaving] = useState(false);
    const [error, setError] = useState('');
    const [savedAt, setSavedAt] = useState('');
    const [status, setStatus] = useState(null);
    const [draft, setDraft] = useState({
        enabled: false,
        min_wallets: '3',
        window_minutes: '10',
        cooldown_minutes: '30',
    });

    const applyResponse = useCallback((resp) => {
        setStatus(resp || null);
        const cfg = resp?.config || {};
        setDraft({
            enabled: Boolean(cfg.enabled),
            min_wallets: String(cfg.min_wallets ?? 3),
            window_minutes: String(cfg.window_minutes ?? 10),
            cooldown_minutes: String(cfg.cooldown_minutes ?? 30),
        });
    }, []);

    const loadConfig = useCallback(async () => {
        if (!hasInitData) {
            setLoading(false);
            setStatus(null);
            return;
        }
        setLoading(true);
        setError('');
        try {
            applyResponse(await fetchSMGoldenDogConfig({ apiBaseUrl, initData, chain: 'bsc' }));
        } catch (err) {
            setError(String(err?.message || err || '加载失败'));
        } finally {
            setLoading(false);
        }
    }, [apiBaseUrl, applyResponse, hasInitData, initData]);

    useEffect(() => {
        loadConfig();
    }, [loadConfig]);

    const barkStatusText = useMemo(() => {
        if (status?.bark_ready) return '已就绪';
        if (status?.bark_configured) return status?.bark_enabled ? '已配置未就绪' : '已配置未开启';
        return '未配置';
    }, [status]);

    const handleSave = useCallback(async () => {
        if (!hasInitData) {
            setError('缺少 Telegram initData，无法保存提醒配置。');
            return;
        }

        const minWallets = Number.parseInt(String(draft.min_wallets || '').trim(), 10);
        const windowMinutes = Number.parseInt(String(draft.window_minutes || '').trim(), 10);
        const cooldownMinutes = Number.parseInt(String(draft.cooldown_minutes || '').trim(), 10);
        if (!Number.isFinite(minWallets) || minWallets < 1) {
            setError('钱包数量必须大于等于 1。');
            return;
        }
        if (!Number.isFinite(windowMinutes) || windowMinutes < 1) {
            setError('统计窗口必须大于等于 1 分钟。');
            return;
        }
        if (!Number.isFinite(cooldownMinutes) || cooldownMinutes < 0) {
            setError('冷却时间不能小于 0。');
            return;
        }

        setSaving(true);
        setError('');
        setSavedAt('');
        try {
            const resp = await saveSMGoldenDogConfig({
                apiBaseUrl,
                initData,
                chain: 'bsc',
                config: {
                    enabled: Boolean(draft.enabled),
                    min_wallets: minWallets,
                    window_minutes: windowMinutes,
                    cooldown_minutes: cooldownMinutes,
                },
            });
            applyResponse(resp);
            setSavedAt('配置已保存');
        } catch (err) {
            setError(String(err?.message || err || '保存失败'));
        } finally {
            setSaving(false);
        }
    }, [apiBaseUrl, applyResponse, draft, hasInitData, initData]);

    return (
        <div className="space-y-4">
            <section className="rounded-[30px] border border-amber-400/15 bg-[radial-gradient(circle_at_top_left,rgba(251,191,36,0.16),transparent_34%),linear-gradient(180deg,rgba(24,24,27,0.94),rgba(9,9,11,0.98))] p-4 shadow-[0_28px_90px_-42px_rgba(0,0,0,0.95)]">
                <div className="flex items-start justify-between gap-3">
                    <div className="flex min-w-0 items-start gap-3">
                        <div className="inline-flex h-11 w-11 shrink-0 items-center justify-center rounded-2xl border border-amber-400/20 bg-amber-400/10 text-amber-300">
                            <Flame size={18} />
                        </div>
                        <div className="min-w-0">
                            <div className="text-base font-semibold text-zinc-100">金狗通知</div>
                            <div className="mt-2 flex flex-wrap items-center gap-1.5">
                                <Badge className={draft.enabled
                                    ? 'border-amber-400/20 bg-amber-400/12 text-amber-200'
                                    : 'border-white/10 bg-zinc-800/80 text-zinc-400'}>
                                    {draft.enabled ? '已开启' : '已关闭'}
                                </Badge>
                                <Badge className="border-white/10 bg-zinc-800/80 text-zinc-300">
                                    Bark {barkStatusText}
                                </Badge>
                            </div>
                        </div>
                    </div>
                    <div className="flex shrink-0 gap-2">
                        <button
                            type="button"
                            className={`rounded-2xl px-3 py-2 text-sm ${getFilterButtonClass(draft.enabled, brand)}`}
                            onClick={() => setDraft((prev) => ({ ...prev, enabled: true }))}
                        >
                            开启
                        </button>
                        <button
                            type="button"
                            className={`rounded-2xl px-3 py-2 text-sm ${getFilterButtonClass(!draft.enabled, brand)}`}
                            onClick={() => setDraft((prev) => ({ ...prev, enabled: false }))}
                        >
                            关闭
                        </button>
                    </div>
                </div>
                <div className="mt-4 grid grid-cols-2 gap-2 sm:grid-cols-4">
                    <StatCard label="通知状态" value={draft.enabled ? '运行中' : '暂停'} compact />
                    <StatCard label="Bark 状态" value={barkStatusText} compact />
                    <StatCard label="触发钱包" value={`${draft.min_wallets || '--'} 个`} compact />
                    <StatCard label="冷却时间" value={`${draft.cooldown_minutes || '--'} 分钟`} compact />
                </div>
            </section>
            <div className="hidden mb-4 flex items-center justify-between gap-3">
                <div>
                    <div className="text-sm font-medium text-zinc-100">金狗通知</div>
                    <div className="text-[11px] text-zinc-500">同交易对聚合聪明钱钱包数，满足阈值后 Bark 提醒</div>
                </div>
                <div className="flex gap-2">
                    <button
                        type="button"
                        className={`rounded-2xl px-3 py-2 text-sm ${getFilterButtonClass(draft.enabled, brand)}`}
                        onClick={() => setDraft((prev) => ({ ...prev, enabled: true }))}
                    >
                        开启
                    </button>
                    <button
                        type="button"
                        className={`rounded-2xl px-3 py-2 text-sm ${getFilterButtonClass(!draft.enabled, brand)}`}
                        onClick={() => setDraft((prev) => ({ ...prev, enabled: false }))}
                    >
                        关闭
                    </button>
                </div>
            </div>

            <div className="hidden mb-4 grid grid-cols-3 gap-2">
                <StatCard label="Bark 状态" value={barkStatusText} compact />
                <StatCard label="钱包阈值" value={`${draft.min_wallets || '--'} 个`} compact />
                <StatCard label="冷却时间" value={`${draft.cooldown_minutes || '--'} 分钟`} compact />
            </div>

            {!hasInitData ? (
                <div className="mb-3 rounded-2xl border border-red-500/20 bg-red-500/10 px-3 py-2 text-sm text-red-200">
                    缺少 Telegram initData，无法保存提醒配置。
                </div>
            ) : null}
            {error ? (
                <div className="mb-3 rounded-2xl border border-red-500/20 bg-red-500/10 px-3 py-2 text-sm text-red-200">
                    {error}
                </div>
            ) : null}
            {!error && savedAt ? (
                <div className="mb-3 rounded-2xl border border-emerald-500/20 bg-emerald-500/10 px-3 py-2 text-sm text-emerald-200">
                    {savedAt}
                </div>
            ) : null}

            <div className="hidden mb-4 rounded-[24px] border border-white/[0.04] bg-zinc-900/60 p-4 text-sm text-zinc-400">
                <div className="leading-6">
                    当同一个交易对在统计窗口内达到设定的钱包数量时，后端会复用全局 Bark 配置发送提醒。
                    聚合口径按交易对计算，不区分池子地址和 fee tier。
                </div>
                <div className="mt-2 leading-6">
                    Bark Key / Server / Group 继续来自全局配置，这里只维护开关、阈值、窗口和冷却时间。
                </div>
            </div>

            {loading ? (
                <div className="py-8 text-center text-zinc-500">加载中...</div>
            ) : (
                <div className="grid gap-3 sm:grid-cols-3">
                    <input
                        className={getInputClass(brand)}
                        type="number"
                        min="1"
                        step="1"
                        placeholder="钱包数量"
                        value={draft.min_wallets}
                        onChange={e => setDraft((prev) => ({ ...prev, min_wallets: e.target.value }))}
                    />
                    <input
                        className={getInputClass(brand)}
                        type="number"
                        min="1"
                        step="1"
                        placeholder="统计窗口(分钟)"
                        value={draft.window_minutes}
                        onChange={e => setDraft((prev) => ({ ...prev, window_minutes: e.target.value }))}
                    />
                    <input
                        className={getInputClass(brand)}
                        type="number"
                        min="0"
                        step="1"
                        placeholder="冷却时间(分钟)"
                        value={draft.cooldown_minutes}
                        onChange={e => setDraft((prev) => ({ ...prev, cooldown_minutes: e.target.value }))}
                    />
                    <button
                        type="button"
                        onClick={handleSave}
                        disabled={saving || !hasInitData}
                        className={`w-full rounded-[24px] px-4 py-3 text-sm font-semibold disabled:opacity-50 sm:col-span-3 ${brand.solidButtonClass}`}
                    >
                        {saving ? '保存中...' : '保存金狗通知配置'}
                    </button>
                </div>
            )}
        </div>
    );
}

function WalletSettingsTab({ apiBaseUrl, brand }) {
    const [wallets, setWallets] = useState([]);
    const [loading, setLoading] = useState(true);
    const [showAdd, setShowAdd] = useState(false);
    const [newAddr, setNewAddr] = useState('');
    const [newLabel, setNewLabel] = useState('');
    const [saving, setSaving] = useState(false);

    const load = useCallback(() => {
        setLoading(true);
        fetchSMWallets({ apiBaseUrl, size: 100 })
            .then(d => setWallets(d?.list || []))
            .catch(() => { })
            .finally(() => setLoading(false));
    }, [apiBaseUrl]);

    useEffect(() => { load(); }, [load]);

    const handleAdd = async () => {
        setSaving(true);
        try {
            await addSMWallet({ apiBaseUrl, address: newAddr, label: newLabel });
            setShowAdd(false);
            setNewAddr('');
            setNewLabel('');
            load();
        } catch (err) {
            alert(err.message);
        } finally {
            setSaving(false);
        }
    };

    const handleToggle = async (w) => {
        try {
            await updateSMWallet({ apiBaseUrl, address: w.address, updates: { is_active: !w.is_active } });
            load();
        } catch (err) {
            alert(err.message);
        }
    };

    const handleDelete = async (w) => {
        if (!confirm(`确认删除钱包 ${shortAddr(w.address)}？`)) return;
        try {
            await deleteSMWallet({ apiBaseUrl, address: w.address });
            load();
        } catch (err) {
            alert(err.message);
        }
    };

    return (
        <div>
            <div className="flex justify-between items-center mb-3">
                <span className="text-sm text-zinc-300">监控钱包</span>
                <button
                    onClick={() => setShowAdd(!showAdd)}
                    className={`${brand.solidButtonClass} ${brand.solidRingClass} rounded-lg px-3 py-1.5 text-xs flex items-center gap-1`}
                >
                    <Plus size={12} /> 添加钱包
                </button>
            </div>

            {showAdd && (
                <div className="bg-zinc-800 rounded-lg p-3 mb-3 space-y-2">
                    <input
                        className="w-full bg-zinc-900 rounded px-3 py-2 text-sm text-zinc-200 outline-none"
                        placeholder="钱包地址 (0x...)"
                        value={newAddr}
                        onChange={e => setNewAddr(e.target.value)}
                    />
                    <input
                        className="w-full bg-zinc-900 rounded px-3 py-2 text-sm text-zinc-200 outline-none"
                        placeholder="标签（可选）"
                        value={newLabel}
                        onChange={e => setNewLabel(e.target.value)}
                    />
                    <div className="flex gap-2 justify-end">
                        <button onClick={() => setShowAdd(false)} className="text-xs text-zinc-400 hover:text-zinc-300 px-3 py-1.5">取消</button>
                        <button onClick={handleAdd} disabled={saving} className={`text-xs ${brand.solidButtonClass} rounded px-3 py-1.5 disabled:opacity-50`}>
                            {saving ? '保存中...' : '添加'}
                        </button>
                    </div>
                </div>
            )}

            {loading ? (
                <div className="text-center text-zinc-500 py-4">加载中...</div>
            ) : wallets.length === 0 ? (
                <div className="text-center text-zinc-500 py-4">暂无钱包。</div>
            ) : (
                <div className="space-y-2">
                    {wallets.map(w => (
                        <div key={w.address} className="bg-zinc-800/60 rounded-lg p-3 flex items-center justify-between">
                            <div>
                                <div className="text-sm text-zinc-200">{w.label || shortAddr(w.address)}</div>
                                <div className="text-[10px] text-zinc-500 font-mono">{shortAddr(w.address)}</div>
                                <div className="flex gap-1 mt-1 text-[10px]">
                                    <span className={`px-1.5 py-0.5 rounded ${w.source === 'manual' ? brand.selectionClass : 'bg-zinc-700 text-zinc-400'}`}>
                                        {w.source === 'manual' ? '手动' : '合约'}
                                    </span>
                                    <span className={`px-1.5 py-0.5 rounded ${w.is_active ? 'bg-green-600/20 text-green-400' : 'bg-zinc-700 text-zinc-500'}`}>
                                        {w.is_active ? '监控中' : '已暂停'}
                                    </span>
                                </div>
                            </div>
                            <div className="flex gap-1">
                                <button onClick={() => handleToggle(w)} className="p-1.5 text-zinc-500 hover:text-zinc-300">
                                    {w.is_active ? <Pause size={14} /> : <Play size={14} />}
                                </button>
                                <button onClick={() => handleDelete(w)} className="p-1.5 text-zinc-500 hover:text-red-400">
                                    <Trash2 size={14} />
                                </button>
                            </div>
                        </div>
                    ))}
                </div>
            )}
        </div>
    );
}

function ContractSettingsTab({ apiBaseUrl, brand }) {
    const [contracts, setContracts] = useState([]);
    const [loading, setLoading] = useState(true);
    const [showAdd, setShowAdd] = useState(false);
    const [newAddr, setNewAddr] = useState('');
    const [newDesc, setNewDesc] = useState('');
    const [saving, setSaving] = useState(false);
    const [busyKey, setBusyKey] = useState('');
    const [actionError, setActionError] = useState('');
    const [confirmState, setConfirmState] = useState(null);
    const [editingContract, setEditingContract] = useState(null);

    const load = useCallback((silent = false) => {
        if (!silent) {
            setLoading(true);
        }
        fetchSMContracts({ apiBaseUrl })
            .then(d => setContracts(d?.list || []))
            .catch(() => { })
            .finally(() => {
                if (!silent) {
                    setLoading(false);
                }
            });
    }, [apiBaseUrl]);

    useEffect(() => { load(); }, [load]);
    useEffect(() => {
        const timer = setInterval(() => {
            load(true);
        }, 10000);
        return () => clearInterval(timer);
    }, [load]);

    const handleAdd = async () => {
        setSaving(true);
        setActionError('');
        try {
            const addr = String(newAddr || '').trim();
            if (!isHexAddressValue(addr)) {
                throw new Error('请输入合法的合约地址');
            }
            await addSMContract({ apiBaseUrl, contract_address: addr, description: newDesc });
            setShowAdd(false);
            setNewAddr('');
            setNewDesc('');
            await load();
        } catch (err) {
            setActionError(err?.message || '操作失败');
        } finally {
            setSaving(false);
        }
    };

    const handleToggle = async (c) => {
        setBusyKey(`contract-toggle:${c.contract_address}`);
        setActionError('');
        try {
            await updateSMContract({ apiBaseUrl, address: c.contract_address, updates: { is_active: !c.is_active } });
            await load();
        } catch (err) {
            setActionError(err?.message || '操作失败');
        } finally {
            setBusyKey('');
        }
    };

    const confirmDelete = async () => {
        if (!confirmState) return;
        setBusyKey(confirmState.key);
        setActionError('');
        try {
            await confirmState.action();
            await load();
            setConfirmState(null);
        } catch (err) {
            setConfirmState(null);
            setActionError(err?.message || '操作失败');
        } finally {
            setBusyKey('');
        }
    };

    return (
        <div>
            <div className="flex justify-between items-center mb-3 gap-3">
                <div>
                    <div className="text-sm font-medium text-zinc-100">合约管理</div>
                    <div className="text-[11px] text-zinc-500">同步 webapp 的监控配置布局</div>
                </div>
                <button
                    type="button"
                    onClick={() => setShowAdd(!showAdd)}
                    className={`${brand.solidButtonClass} ${brand.solidRingClass} inline-flex shrink-0 items-center gap-1 rounded-2xl px-3 py-2 text-sm`}
                >
                    <Plus size={14} /> 添加合约
                </button>
            </div>

            {actionError ? (
                <div className="mb-3 rounded-2xl border border-red-500/20 bg-red-500/10 px-3 py-2 text-sm text-red-200">
                    {actionError}
                </div>
            ) : null}

            {showAdd && (
                <div className="mb-3 rounded-[24px] border border-white/[0.04] bg-zinc-900/60 p-3 space-y-2">
                    <input
                        className={getInputClass(brand)}
                        placeholder="合约地址 (0x...)"
                        value={newAddr}
                        onChange={e => setNewAddr(e.target.value)}
                    />
                    <textarea
                        className={`${getInputClass(brand)} min-h-[88px] resize-none`}
                        placeholder="描述（可选）"
                        rows={2}
                        value={newDesc}
                        onChange={e => setNewDesc(e.target.value)}
                    />
                    <div className="text-[11px] text-zinc-500">
                        只需要填写监控合约地址，添加后会直接扫描发往该地址的交易。
                    </div>
                    <div className="flex gap-2">
                        <button type="button" onClick={() => setShowAdd(false)} className="flex-1 rounded-2xl border border-white/[0.05] bg-zinc-900/65 px-4 py-2.5 text-sm text-zinc-300 transition hover:bg-zinc-800/80">取消</button>
                        <button type="button" onClick={handleAdd} disabled={saving} className={`flex-1 rounded-2xl px-4 py-2.5 text-sm disabled:opacity-50 ${brand.solidButtonClass}`}>
                            {saving ? '保存中...' : '添加'}
                        </button>
                    </div>
                </div>
            )}

            {loading ? (
                <div className="py-8 text-center text-zinc-500">加载中...</div>
            ) : contracts.length === 0 ? (
                <div className="rounded-2xl border border-dashed border-white/[0.05] bg-zinc-900/45 px-4 py-8 text-center text-sm text-zinc-500">暂无配置合约</div>
            ) : (
                <div className="space-y-3">
                    {contracts.map(c => (
                        <div key={c.contract_address} className="rounded-[24px] border border-white/[0.04] bg-zinc-900/60 p-3 shadow-[0_18px_50px_-32px_rgba(0,0,0,0.95)]">
                            <div className="flex items-start justify-between gap-3">
                                <div className="min-w-0 flex-1">
                                    <div className="flex flex-wrap items-center gap-1.5">
                                        <span className="text-sm font-semibold text-zinc-100">监控合约</span>
                                        <Badge className={c.is_active
                                            ? 'border-emerald-500/20 bg-emerald-500/10 text-emerald-300'
                                            : 'border-white/10 bg-zinc-800/80 text-zinc-400'}>
                                            {c.is_active ? '活跃' : '已暂停'}
                                        </Badge>
                                    </div>
                                    <div className="mt-2 flex flex-wrap items-center gap-1.5">
                                        <CompactIdentifier value={c.contract_address} label="合约" />
                                    </div>
                                    {c.description && <div className="mt-2 text-sm text-zinc-400">{c.description}</div>}
                                    <div className="mt-2 text-[11px] text-zinc-500">已扫描至区块 {c.last_scanned_block || '未扫描'}</div>
                                </div>
                                <div className="flex shrink-0 gap-1">
                                    <button
                                        type="button"
                                        onClick={() => setEditingContract(c)}
                                        disabled={busyKey === `contract-toggle:${c.contract_address}` || busyKey === `contract-delete:${c.contract_address}`}
                                        className={getIconButtonClass(false)}
                                    >
                                        <Pencil size={14} />
                                    </button>
                                    <button
                                        type="button"
                                        onClick={() => handleToggle(c)}
                                        disabled={busyKey === `contract-toggle:${c.contract_address}` || busyKey === `contract-delete:${c.contract_address}`}
                                        className={getIconButtonClass(false)}
                                    >
                                        {c.is_active ? <Pause size={14} /> : <Play size={14} />}
                                    </button>
                                    <button
                                        type="button"
                                        disabled={busyKey === `contract-toggle:${c.contract_address}` || busyKey === `contract-delete:${c.contract_address}`}
                                        className={getIconButtonClass(true)}
                                        onClick={() => setConfirmState({
                                            key: `contract-delete:${c.contract_address}`,
                                            title: '删除合约',
                                            description: `确认删除合约 ${shortAddr(c.contract_address)} 吗？`,
                                            action: () => deleteSMContract({ apiBaseUrl, address: c.contract_address }),
                                        })}
                                    >
                                        <Trash2 size={14} />
                                    </button>
                                </div>
                            </div>
                        </div>
                    ))}
                </div>
            )}

            <ConfirmDialog
                open={Boolean(confirmState)}
                title={confirmState?.title || '确认操作'}
                description={confirmState?.description || ''}
                confirmLabel="删除"
                busy={busyKey.startsWith('contract-delete:')}
                onCancel={() => {
                    if (!busyKey.startsWith('contract-delete:')) setConfirmState(null);
                }}
                onConfirm={confirmDelete}
            />
            <EditContractModal
                open={Boolean(editingContract)}
                apiBaseUrl={apiBaseUrl}
                contract={editingContract}
                brand={brand}
                onClose={() => setEditingContract(null)}
                onSaved={async () => {
                    await load();
                    setEditingContract(null);
                }}
            />
        </div>
    );
}

function EditWalletModal({ open, apiBaseUrl, wallet, onClose, onSaved, brand }) {
    const [label, setLabel] = useState('');
    const [avatarFile, setAvatarFile] = useState(null);
    const [removeAvatar, setRemoveAvatar] = useState(false);
    const [saving, setSaving] = useState(false);
    const [error, setError] = useState('');
    const [avatarPreviewUrl, setAvatarPreviewUrl] = useState('');

    useEffect(() => {
        if (!open || !wallet) return;
        setLabel(String(wallet?.label || ''));
        setAvatarFile(null);
        setRemoveAvatar(false);
        setError('');
        setSaving(false);
    }, [open, wallet]);

    useEffect(() => {
        if (!open || !wallet) {
            setAvatarPreviewUrl('');
            return undefined;
        }
        if (avatarFile) {
            const objectUrl = URL.createObjectURL(avatarFile);
            setAvatarPreviewUrl(objectUrl);
            return () => URL.revokeObjectURL(objectUrl);
        }
        setAvatarPreviewUrl(removeAvatar ? '' : String(wallet?.avatar_url || ''));
        return undefined;
    }, [avatarFile, open, removeAvatar, wallet]);

    const handleAvatarFileChange = useCallback((event) => {
        const nextFile = event.target.files?.[0];
        event.target.value = '';
        if (!nextFile) return;
        if (!['image/png', 'image/jpeg', 'image/webp'].includes(String(nextFile.type || '').toLowerCase())) {
            setError('头像仅支持 PNG、JPG、WEBP。');
            return;
        }
        if (nextFile.size > SMART_MONEY_AVATAR_MAX_BYTES) {
            setError('头像大小不能超过 5MB。');
            return;
        }
        setAvatarFile(nextFile);
        setRemoveAvatar(false);
        setError('');
    }, []);

    const handleSubmit = async () => {
        if (!wallet?.address) return;
        setSaving(true);
        setError('');
        try {
            const nextLabel = String(label || '').trim();
            const currentLabel = String(wallet?.label || '').trim();
            const shouldClearAvatar = !avatarFile && removeAvatar && String(wallet?.avatar_url || '').trim();

            if (avatarFile) {
                await uploadSMWalletAvatar({
                    apiBaseUrl,
                    address: wallet.address,
                    file: avatarFile,
                });
            }

            const updates = {};
            if (nextLabel !== currentLabel) {
                updates.label = nextLabel || null;
            }
            if (shouldClearAvatar) {
                updates.avatar_url = null;
            }

            if (Object.keys(updates).length > 0) {
                await updateSMWallet({
                    apiBaseUrl,
                    address: wallet.address,
                    updates,
                });
            }
            await onSaved?.();
        } catch (err) {
            setError(err?.message || '保存失败');
        } finally {
            setSaving(false);
        }
    };

    if (!open || !wallet) return null;

    return (
        <div className="fixed inset-0 z-50 flex items-end justify-center bg-black/60 p-4 sm:items-center" onClick={saving ? undefined : onClose}>
            <div className="w-full max-w-md rounded-[28px] border border-white/[0.05] bg-zinc-950/95 p-5 shadow-[0_24px_80px_-32px_rgba(0,0,0,0.95)]" onClick={(e) => e.stopPropagation()}>
                <div className="flex items-start justify-between gap-3">
                    <div>
                        <h3 className="text-lg font-semibold text-zinc-100">编辑钱包</h3>
                        <div className="mt-2">
                            <CompactIdentifier value={wallet.address} label="钱包" />
                        </div>
                    </div>
                    <button type="button" onClick={onClose} disabled={saving} className="inline-flex h-9 w-9 items-center justify-center rounded-xl border border-white/[0.05] bg-zinc-900/65 text-zinc-400 transition hover:text-zinc-200">
                        <X size={18} />
                    </button>
                </div>

                {error ? (
                    <div className="mt-4 rounded-2xl border border-red-500/20 bg-red-500/10 px-3 py-2 text-sm text-red-200">
                        {error}
                    </div>
                ) : null}

                <div className="mt-4 space-y-3">
                    <div className="flex items-center gap-3 rounded-2xl border border-white/[0.05] bg-zinc-900/65 px-3 py-3">
                        <WalletAvatar
                            address={wallet.address}
                            color={wallet.color || '#7F77DD'}
                            avatarUrl={avatarPreviewUrl}
                            size={64}
                        />
                        <div className="min-w-0 flex-1">
                            <div className="flex flex-wrap gap-2">
                                <label className={`${brand.softButtonClass || 'border border-white/[0.06] bg-zinc-900/65 text-zinc-200'} inline-flex cursor-pointer items-center rounded-xl px-3 py-2 text-sm`}>
                                    <input
                                        type="file"
                                        accept={SMART_MONEY_AVATAR_ACCEPT}
                                        disabled={saving}
                                        className="hidden"
                                        onChange={handleAvatarFileChange}
                                    />
                                    上传头像
                                </label>
                                <button
                                    type="button"
                                    onClick={() => {
                                        setAvatarFile(null);
                                        setRemoveAvatar(true);
                                        setError('');
                                    }}
                                    disabled={saving || (!avatarFile && !String(wallet?.avatar_url || '').trim())}
                                    className="inline-flex items-center rounded-xl border border-white/[0.06] bg-zinc-900/65 px-3 py-2 text-sm text-zinc-300 transition hover:bg-zinc-800/80 disabled:opacity-50"
                                >
                                    恢复默认
                                </button>
                            </div>
                            <div className="mt-2 text-[11px] text-zinc-500">支持 PNG/JPG/WEBP，大小不超过 5MB。</div>
                            {avatarFile ? <div className="mt-1 truncate text-[11px] text-zinc-400">{avatarFile.name}</div> : null}
                        </div>
                    </div>
                    <input
                        className={getInputClass(brand)}
                        placeholder="钱包标签"
                        value={label}
                        onChange={(e) => setLabel(e.target.value)}
                    />
                </div>

                <div className="mt-5 flex gap-2">
                    <button type="button" onClick={onClose} disabled={saving} className="flex-1 rounded-2xl border border-white/[0.05] bg-zinc-900/65 px-4 py-2.5 text-sm text-zinc-300 transition hover:bg-zinc-800/80">取消</button>
                    <button
                        type="button"
                        onClick={handleSubmit}
                        disabled={saving}
                        className={`flex-1 rounded-2xl px-4 py-2.5 text-sm disabled:opacity-50 ${brand.solidButtonClass}`}
                    >
                        {saving ? '保存中...' : '保存'}
                    </button>
                </div>
            </div>
        </div>
    );
}

function EditContractModal({ open, apiBaseUrl, contract, onClose, onSaved, brand }) {
    const [description, setDescription] = useState('');
    const [saving, setSaving] = useState(false);
    const [error, setError] = useState('');

    useEffect(() => {
        if (!open || !contract) return;
        setDescription(String(contract?.description || ''));
        setError('');
        setSaving(false);
    }, [contract, open]);

    const handleSubmit = async () => {
        if (!contract?.contract_address) return;
        setSaving(true);
        setError('');
        try {
            await updateSMContract({
                apiBaseUrl,
                address: contract.contract_address,
                updates: { description: String(description || '').trim() || null },
            });
            await onSaved?.();
        } catch (err) {
            setError(err?.message || '保存失败');
        } finally {
            setSaving(false);
        }
    };

    if (!open || !contract) return null;

    return (
        <div className="fixed inset-0 z-50 flex items-end justify-center bg-black/60 p-4 sm:items-center" onClick={saving ? undefined : onClose}>
            <div className="w-full max-w-md rounded-[28px] border border-white/[0.05] bg-zinc-950/95 p-5 shadow-[0_24px_80px_-32px_rgba(0,0,0,0.95)]" onClick={(e) => e.stopPropagation()}>
                <div className="flex items-start justify-between gap-3">
                    <div>
                        <h3 className="text-lg font-semibold text-zinc-100">编辑合约</h3>
                        <div className="mt-2">
                            <CompactIdentifier value={contract.contract_address} label="合约" />
                        </div>
                    </div>
                    <button type="button" onClick={onClose} disabled={saving} className="inline-flex h-9 w-9 items-center justify-center rounded-xl border border-white/[0.05] bg-zinc-900/65 text-zinc-400 transition hover:text-zinc-200">
                        <X size={18} />
                    </button>
                </div>

                {error ? (
                    <div className="mt-4 rounded-2xl border border-red-500/20 bg-red-500/10 px-3 py-2 text-sm text-red-200">
                        {error}
                    </div>
                ) : null}

                <div className="mt-4 space-y-3">
                    <textarea
                        className={`${getInputClass(brand)} min-h-[110px] resize-none`}
                        placeholder="合约备注"
                        rows={3}
                        value={description}
                        onChange={(e) => setDescription(e.target.value)}
                    />
                </div>

                <div className="mt-5 flex gap-2">
                    <button type="button" onClick={onClose} disabled={saving} className="flex-1 rounded-2xl border border-white/[0.05] bg-zinc-900/65 px-4 py-2.5 text-sm text-zinc-300 transition hover:bg-zinc-800/80">取消</button>
                    <button
                        type="button"
                        onClick={handleSubmit}
                        disabled={saving}
                        className={`flex-1 rounded-2xl px-4 py-2.5 text-sm disabled:opacity-50 ${brand.solidButtonClass}`}
                    >
                        {saving ? '保存中...' : '保存'}
                    </button>
                </div>
            </div>
        </div>
    );
}

// ============ Add Wallet Modal ============
function AddWalletModal({ apiBaseUrl, onClose, onAdded, brand }) {
    const [address, setAddress] = useState('');
    const [label, setLabel] = useState('');
    const [saving, setSaving] = useState(false);
    const [error, setError] = useState('');

    const handleSubmit = async () => {
        setSaving(true);
        setError('');
        try {
            await addSMWallet({ apiBaseUrl, address, label });
            onAdded?.();
            onClose();
        } catch (err) {
            setError(err?.message || '添加失败');
        } finally {
            setSaving(false);
        }
    };

    return (
        <div className="fixed inset-0 z-[60] flex items-end justify-center bg-black/60 p-4 sm:items-center" onClick={saving ? undefined : onClose}>
            <div className="w-full max-w-md rounded-[28px] border border-white/[0.05] bg-zinc-950/95 p-5 shadow-[0_24px_80px_-32px_rgba(0,0,0,0.95)]" onClick={(e) => e.stopPropagation()}>
                <div className="flex items-start justify-between gap-3">
                    <div>
                        <h3 className="text-lg font-semibold text-zinc-100">添加钱包</h3>
                        <p className="mt-1 text-sm text-zinc-500">沿用 webapp 的弹窗式添加流程</p>
                    </div>
                    <button type="button" onClick={onClose} disabled={saving} className="inline-flex h-9 w-9 items-center justify-center rounded-xl border border-white/[0.05] bg-zinc-900/65 text-zinc-400 transition hover:text-zinc-200">
                        <X size={18} />
                    </button>
                </div>

                {error ? (
                    <div className="mt-4 rounded-2xl border border-red-500/20 bg-red-500/10 px-3 py-2 text-sm text-red-200">
                        {error}
                    </div>
                ) : null}

                <div className="mt-4 space-y-3">
                    <input
                        className={getInputClass(brand)}
                        placeholder="钱包地址 (0x...)"
                        value={address}
                        onChange={e => setAddress(e.target.value)}
                    />
                    <input
                        className={getInputClass(brand)}
                        placeholder="标签（可选）"
                        value={label}
                        onChange={e => setLabel(e.target.value)}
                    />
                </div>
                <div className="mt-5 flex gap-2">
                    <button type="button" onClick={onClose} disabled={saving} className="flex-1 rounded-2xl border border-white/[0.05] bg-zinc-900/65 px-4 py-2.5 text-sm text-zinc-300 transition hover:bg-zinc-800/80">取消</button>
                    <button
                        type="button"
                        onClick={handleSubmit}
                        disabled={!address || saving}
                        className={`flex-1 rounded-2xl px-4 py-2.5 text-sm disabled:opacity-50 ${brand.solidButtonClass}`}
                    >
                        {saving ? '添加中...' : '添加钱包'}
                    </button>
                </div>
            </div>
        </div>
    );
}

// ============ MAIN COMPONENT ============
export default function SmartMoneyPage({ apiBaseUrl, initData = '', accentTheme = 'lime', onOpenPosition }) {
    const brand = useMemo(() => getBrandTheme(accentTheme), [accentTheme]);
    const [view, setView] = useState('pools');
    const [stats, setStats] = useState(null);
    const [selectedPool, setSelectedPool] = useState(null);
    const [selectedWallet, setSelectedWallet] = useState(null);
    const [showAddModal, setShowAddModal] = useState(false);
    const [walletRefreshKey, setWalletRefreshKey] = useState(0);

    const refreshStats = useCallback(() => {
        fetchSMStats({ apiBaseUrl }).then(setStats).catch(() => { });
    }, [apiBaseUrl]);

    useEffect(() => {
        refreshStats();
        const interval = setInterval(() => {
            refreshStats();
        }, 30000);
        return () => clearInterval(interval);
    }, [refreshStats]);

    const handleSelectPool = useCallback((pool) => {
        setSelectedPool(pool);
        setSelectedWallet(null);
    }, []);

    const handleSelectWallet = useCallback((addr) => {
        setSelectedWallet(addr);
        setSelectedPool(null);
        setView('wallets');
    }, []);

    const handleBack = useCallback(() => {
        setSelectedPool(null);
        setSelectedWallet(null);
    }, []);

    const isDetailView = selectedPool || selectedWallet;
    const monitorSummary = useMemo(() => {
        const activeWallets = stats?.monitored_wallet_count ?? 0;
        const activeContracts = stats?.active_contract_count ?? 0;
        const watcherEnabled = Boolean(stats?.watcher_enabled);
        const contractMonitorEnabled = Boolean(stats?.crawler_enabled);
        const monitorEnabled = Boolean(stats?.monitor_enabled);

        if (!monitorEnabled) {
            return {
                enabled: false,
                label: '监控未开启',
                detail: '后端 Smart Money 服务未启动',
            };
        }

        const channels = [];
        if (watcherEnabled) channels.push(`LP 监控 ${activeWallets} 钱包`);
        if (contractMonitorEnabled) channels.push(activeContracts > 0 ? `合约监控 ${activeContracts} 个` : '合约监控待配置');

        return {
            enabled: true,
            label: '监控已开启',
            detail: channels.length ? channels.join(' / ') : 'Smart Money 服务运行中',
        };
    }, [stats]);

    return (
        <div className="max-w-3xl mx-auto pb-24">
            <section className="rounded-[30px] border border-white/[0.04] bg-[radial-gradient(circle_at_top_left,rgba(255,255,255,0.04),transparent_35%),linear-gradient(180deg,rgba(24,24,27,0.92),rgba(9,9,11,0.96))] p-4 shadow-[0_28px_90px_-42px_rgba(0,0,0,0.95)]">
                {stats && !isDetailView && (
                    <div
                        className={`mb-4 rounded-[24px] border px-4 py-3 ${monitorSummary.enabled
                            ? 'border-white/[0.05] bg-white/[0.02]'
                            : 'border-white/[0.04] bg-zinc-900/55'
                            }`}
                    >
                        <div className="inline-flex items-center gap-2 rounded-full bg-zinc-900/70 px-3 py-1 text-[11px] font-medium text-zinc-100">
                            <span className={`inline-block h-2 w-2 rounded-full ${monitorSummary.enabled ? brand.dotClass : 'bg-zinc-500'}`} />
                            {monitorSummary.label}
                        </div>
                        <div className="mt-2 text-[12px] text-zinc-500">{monitorSummary.detail}</div>
                    </div>
                )}

                {stats && !isDetailView && (
                    <div className="grid grid-cols-2 gap-2 mb-4">
                        <StatCard label="活跃池子" value={stats.active_pool_count} />
                        <StatCard label="监控钱包" value={stats.monitored_wallet_count} />
                        <StatCard label="持仓笔数" value={stats.open_position_count} />
                        <StatCard label="今日关闭" value={stats.closed_today_count} color="text-red-400" />
                    </div>
                )}

                {!isDetailView && (
                    <div className="grid grid-cols-4 gap-2 mb-4">
                        {[
                            { key: 'pools', label: '池子视图', icon: Eye },
                            { key: 'wallets', label: '钱包视图', icon: Wallet },
                            { key: 'settings', label: '合约视图', icon: Settings },
                        ].map(({ key, label, icon: Icon }) => (
                            <button
                                key={key}
                                type="button"
                                className={`inline-flex min-h-[60px] flex-col items-center justify-center gap-1 rounded-2xl px-2 py-2 text-[11px] leading-tight sm:min-h-0 sm:flex-row sm:gap-1.5 sm:px-3 sm:py-2.5 sm:text-sm ${getFilterButtonClass(view === key, brand)}`}
                                onClick={() => setView(key)}
                            >
                                <Icon size={13} className="shrink-0 sm:h-[14px] sm:w-[14px]" />
                                <span className="text-center whitespace-normal break-words sm:truncate">{label}</span>
                            </button>
                        ))}
                        <button
                            type="button"
                            className={`inline-flex min-h-[60px] flex-col items-center justify-center gap-1 rounded-2xl px-2 py-2 text-[11px] leading-tight sm:min-h-0 sm:flex-row sm:gap-1.5 sm:px-3 sm:py-2.5 sm:text-sm ${getFilterButtonClass(view === 'golden_dog', brand)}`}
                            onClick={() => setView('golden_dog')}
                        >
                            <Flame size={13} className="shrink-0 sm:h-[14px] sm:w-[14px]" />
                            <span className="text-center whitespace-normal break-words sm:truncate">监控通知</span>
                        </button>
                    </div>
                )}

                {selectedPool ? (
                    <PoolDetailPage
                        apiBaseUrl={apiBaseUrl}
                        pool={selectedPool}
                        onBack={handleBack}
                        onSelectWallet={handleSelectWallet}
                        brand={brand}
                    />
                ) : selectedWallet ? (
                    <WalletDetailPage
                        apiBaseUrl={apiBaseUrl}
                        walletAddress={selectedWallet}
                        onBack={handleBack}
                        onSelectPool={handleSelectPool}
                        brand={brand}
                    />
                ) : view === 'pools' ? (
                    <PoolListPage
                        apiBaseUrl={apiBaseUrl}
                        onSelectPool={handleSelectPool}
                        onOpenPosition={onOpenPosition}
                        brand={brand}
                    />
                ) : view === 'wallets' ? (
                    <WalletListPage
                        apiBaseUrl={apiBaseUrl}
                        onSelectWallet={(addr) => setSelectedWallet(addr)}
                        onAddWallet={() => setShowAddModal(true)}
                        brand={brand}
                        refreshKey={walletRefreshKey}
                    />
                ) : view === 'golden_dog' ? (
                    <GoldenDogPage
                        apiBaseUrl={apiBaseUrl}
                        initData={initData}
                        brand={brand}
                    />
                ) : (
                    <ContractSettingsPage apiBaseUrl={apiBaseUrl} brand={brand} />
                )}
            </section>

            {showAddModal && (
                <AddWalletModal
                    apiBaseUrl={apiBaseUrl}
                    onClose={() => setShowAddModal(false)}
                    brand={brand}
                    onAdded={() => {
                        setWalletRefreshKey((value) => value + 1);
                        refreshStats();
                    }}
                />
            )}
        </div>
    );
}
