import { useState, useEffect, useCallback, useMemo, useRef, Suspense, lazy } from 'react';
import {
    Eye, Wallet, Settings, Search, Plus, ExternalLink, X, Check,
    ChevronRight, ChevronDown, ChevronLeft, Pause, Play, Trash2, Copy, Flame, Pencil, SlidersHorizontal, Activity,
    Clock, DollarSign, Percent, Users, Zap, Radar,
} from 'lucide-react';

const LazySmartMoneyAssetsPage = lazy(() => import('./SmartMoneyAssetsPage.jsx'));
import {
    fetchSMPools, fetchSMPoolStats, fetchSMPoolFeeHeatmap, fetchSMPositions, fetchSMWallets,
    fetchSMPositionDetail,
    fetchSMStats, addSMWallet, updateSMWallet, deleteSMWallet,
    fetchSMZombieWallets, deleteSMZombieWallets,
    fetchSMContracts, addSMContract, updateSMContract, deleteSMContract,
    uploadSMWalletAvatar, resolveSMAvatarAssetUrl,
    fetchSMPoolLiquidityWalletCandidates, importSMPoolLiquidityWallets, streamSMPoolLiquidityWalletCandidates,
    fetchSMGoldenDogConfig, saveSMGoldenDogConfig, testSMGoldenDogConfig,
    fetchSMWatchWallets, fetchSMWatchActivity, saveSMWatchWallets,
    fetchSMWatchOpenAlertConfig, saveSMWatchOpenAlertConfig, testSMWatchOpenAlertConfig,
    fetchSMAutoFollow, saveSMAutoFollowConfig, deleteSMAutoFollowConfig,
    buildSMEventsWsUrl,
} from '../lib/smartMoneyApi';
import { getBrandTheme } from '../lib/brand';
import { formatDurationFrom } from '../lib/time';
import {
    formatFeeTier,
    formatUSDCompact,
    formatWalletBalance,
} from '../lib/format';
import FlashIcon from './FlashIcon.jsx';
import PositionCard from './PositionCard.jsx';
import uniswapIcon from '../image/uniswap.svg';
import pancakeIcon from '../image/pancake.svg';
import gmgnIcon from '../image/gmgn.svg';
import {
    getBrandActionChipClass,
    getBrandLinkClass,
    getFilterButtonClass,
    getIconButtonClass,
    getInputClass,
} from '../features/smartMoney/shared/brandClasses';
import {
    isHexAddressValue,
    normalizeWalletAddress,
    shortAddr,
    tailAddr,
} from '../features/smartMoney/shared/wallet';
import { buildGmgnUrl } from '../features/smartMoney/shared/pools';
import { formatOptionalNumber, parseOptionalNumber } from '../features/smartMoney/shared/poolFilterStorage';
import {
    formatHeatmapRate,
    formatHeatmapUSD,
    formatPreviewUsd,
    getPositionSelectionKey,
} from '../features/smartMoney/shared/format';

const PROTOCOL_MAP = {
    pancake_v3: { version: 'V3', icon: pancakeIcon, color: '#d1884f' },
    uniswap_v3: { version: 'V3', icon: uniswapIcon, color: '#ff007a' },
    uniswap_v4: { version: 'V4', icon: uniswapIcon, color: '#ff007a' },
};
const WALLET_AVATAR_ICONS = Object.entries(
    import.meta.glob('../icon/avatar_*.png', { eager: true, import: 'default' })
).sort(([a], [b]) => a.localeCompare(b, undefined, { numeric: true })).map(([, src]) => src);
const SMART_MONEY_AVATAR_ACCEPT = 'image/png,image/jpeg,image/webp';
const SMART_MONEY_AVATAR_MAX_BYTES = 5 * 1024 * 1024;
function formatDateTimeLocalValue(date) {
    const pad = (value) => String(value).padStart(2, '0');
    return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`;
}

function createDefaultTokenLiquidityRange() {
    const end = new Date();
    end.setMilliseconds(0);
    const start = new Date(end.getTime() - 24 * 60 * 60 * 1000);
    return {
        start: formatDateTimeLocalValue(start),
        end: formatDateTimeLocalValue(end),
    };
}

function tokenLiquidityLocalToISO(value) {
    const text = String(value || '').trim();
    if (!text) return '';
    const date = new Date(text);
    if (Number.isNaN(date.getTime())) return '';
    return date.toISOString();
}

function formatTokenLiquidityDateTimeRange(startValue, endValue) {
    const start = String(startValue || '').replace('T', ' ');
    const end = String(endValue || '').replace('T', ' ');
    if (!start || !end) return '请选择开始和结束时间';
    return `${start} → ${end}`;
}

const SMART_MONEY_RADAR_SCAN_TIMEOUT_MS = 50 * 1000;

function formatRadarElapsed(ms) {
    const totalSeconds = Math.max(0, Math.floor(Number(ms || 0) / 1000));
    const minutes = Math.floor(totalSeconds / 60);
    const seconds = totalSeconds % 60;
    if (minutes <= 0) return `${seconds}s`;
    return `${minutes}m ${String(seconds).padStart(2, '0')}s`;
}

function createRadarLogEntry(text, tone = 'info') {
    const date = new Date();
    return {
        id: `${date.getTime()}-${Math.random().toString(16).slice(2)}`,
        time: date.toLocaleTimeString('zh-CN', { hour12: false, hour: '2-digit', minute: '2-digit', second: '2-digit' }),
        text,
        tone,
    };
}

function parsePoolLiquidityInput(value) {
    const text = String(value || '').trim();
    if (/^0x[a-fA-F0-9]{40}$/.test(text)) {
        return { poolAddress: text.toLowerCase(), poolId: '' };
    }
    if (/^0x[a-fA-F0-9]{64}$/.test(text)) {
        return { poolAddress: '', poolId: text.toLowerCase() };
    }
    return null;
}

function upsertPoolLiquidityCandidate(list, candidate) {
    const wallet = String(candidate?.wallet_address || '').toLowerCase();
    if (!wallet) return Array.isArray(list) ? list : [];
    const next = Array.isArray(list) ? [...list] : [];
    const index = next.findIndex((item) => String(item?.wallet_address || '').toLowerCase() === wallet);
    if (index >= 0) next[index] = { ...next[index], ...candidate, wallet_address: wallet };
    else next.push({ ...candidate, wallet_address: wallet });
    return next.sort((a, b) => {
        const amountDiff = Number(b?.max_amount_usd || 0) - Number(a?.max_amount_usd || 0);
        if (amountDiff !== 0) return amountDiff;
        return String(b?.tx_time || '').localeCompare(String(a?.tx_time || ''));
    });
}

function GoldenDogPageContent({
    apiBaseUrl,
    initData,
    brand,
    watchedWallets = [],
    watchedWalletSet = new Set(),
    watchToggleMap = {},
    onToggleWatchWallet,
}) {
    const hasInitData = Boolean(String(initData || '').trim());
    const [loading, setLoading] = useState(hasInitData);
    const [saving, setSaving] = useState(false);
    const [testingMode, setTestingMode] = useState('');
    const [error, setError] = useState('');
    const [notice, setNotice] = useState('');
    const [status, setStatus] = useState(null);
    const [watchAlertStatus, setWatchAlertStatus] = useState(null);
    const [draft, setDraft] = useState(() => createGoldenDogDraft());
    const [watchAlertDraft, setWatchAlertDraft] = useState(() => createWatchOpenAlertDraft());
    const [activeTab, setActiveTab] = useState('wallet');
    const [lastWatchEventText, setLastWatchEventText] = useState('');

    const barkStatusText = useMemo(() => goldenDogBarkStatusText(status), [status]);
    const watchBarkStatusText = useMemo(() => goldenDogBarkStatusText(watchAlertStatus), [watchAlertStatus]);
    const intensityOptions = useMemo(
        () => (Array.isArray(status?.available_intensities) && status.available_intensities.length > 0
            ? status.available_intensities
            : GOLDEN_DOG_INTENSITY_OPTIONS),
        [status],
    );
    const watchedWalletList = useMemo(
        () => Array.from(new Set((Array.isArray(watchedWallets) ? watchedWallets : [])
            .map((item) => normalizeWalletAddress(item))
            .filter(Boolean))).sort(),
        [watchedWallets],
    );

    const applyResponse = useCallback((resp) => {
        setStatus(resp || null);
        setDraft(mapGoldenDogConfigToDraft(resp?.config));
    }, []);
    const applyWatchAlertResponse = useCallback((resp) => {
        setWatchAlertStatus(resp || null);
        setWatchAlertDraft(mapWatchOpenAlertConfigToDraft(resp?.config));
    }, []);

    const loadConfig = useCallback(async () => {
        if (!hasInitData) {
            setLoading(false);
            setStatus(null);
            setWatchAlertStatus(null);
            return;
        }
        setLoading(true);
        setError('');
        try {
            const [goldenDogResp, watchAlertResp] = await Promise.all([
                fetchSMGoldenDogConfig({ apiBaseUrl, initData, chain: 'bsc' }),
                fetchSMWatchOpenAlertConfig({ apiBaseUrl, initData, chain: 'bsc' }),
            ]);
            applyResponse(goldenDogResp);
            applyWatchAlertResponse(watchAlertResp);
        } catch (err) {
            setError(String(err?.message || err || '加载失败'));
        } finally {
            setLoading(false);
        }
    }, [apiBaseUrl, applyResponse, applyWatchAlertResponse, hasInitData, initData]);

    useEffect(() => {
        loadConfig();
    }, [loadConfig]);

    useEffect(() => {
        if (!hasInitData || !watchAlertDraft.enabled || !watchAlertDraft.sound_enabled || !watchedWalletList.length) {
            return undefined;
        }

        const wsUrl = buildSMEventsWsUrl(apiBaseUrl);
        if (!wsUrl) return undefined;

        let cancelled = false;
        let socket;
        try {
            socket = new WebSocket(wsUrl);
        } catch {
            return undefined;
        }

        socket.onmessage = async (messageEvent) => {
            if (cancelled) return;
            if (typeof document !== 'undefined' && document.visibilityState !== 'visible') return;
            try {
                const payload = JSON.parse(messageEvent.data);
                if (payload?.type !== 'lp_event') return;
                const event = payload?.data || {};
                const eventType = String(event?.event_type || '').trim().toLowerCase();
                const walletAddress = normalizeWalletAddress(event?.wallet_address);
                if (eventType !== 'add' || !walletAddress || !watchedWalletSet.has(walletAddress)) return;
                const walletName = String(event?.wallet_label || '').trim() || shortAddr(walletAddress);
                const pairLabel = getPairLabel({
                    token0_symbol: event?.token0_symbol,
                    token1_symbol: event?.token1_symbol,
                }) || tailAddr(event?.pool_address);
                setLastWatchEventText(`${walletName} 开仓 ${pairLabel}`);
                console.log('[SmartMoney] 检测到开仓事件，准备播放音效:', walletName, pairLabel);
                const played = await playSmartMoneyBeep();
                console.log('[SmartMoney] 音效播放结果:', played ? '成功' : '失败');
            } catch (err) {
                console.error('[SmartMoney] WebSocket消息处理错误:', err);
            }
        };

        return () => {
            cancelled = true;
            try {
                socket?.close();
            } catch {
                // ignore close errors
            }
        };
    }, [apiBaseUrl, hasInitData, watchAlertDraft.enabled, watchAlertDraft.sound_enabled, watchedWalletList, watchedWalletSet]);

    const updateWalletMode = useCallback((key, value) => {
        setDraft((prev) => ({
            ...prev,
            wallet_mode: { ...prev.wallet_mode, [key]: value },
        }));
    }, []);

    const updateWalletAmountTier = useCallback((index, key, value) => {
        setDraft((prev) => {
            const tiers = Array.isArray(prev.wallet_mode.amount_intensity_tiers)
                ? prev.wallet_mode.amount_intensity_tiers.map((tier) => ({ ...tier }))
                : cloneGoldenDogDefaultAmountTiers();
            while (tiers.length < 3) {
                tiers.push({ ...GOLDEN_DOG_DEFAULT_AMOUNT_TIERS[tiers.length] });
            }
            tiers[index] = { ...tiers[index], [key]: value };
            return {
                ...prev,
                wallet_mode: { ...prev.wallet_mode, amount_intensity_tiers: tiers },
            };
        });
    }, []);

    const updatePoolMode = useCallback((key, value) => {
        setDraft((prev) => ({
            ...prev,
            pool_mode: { ...prev.pool_mode, [key]: value },
        }));
    }, []);
    const updateWatchAlertMode = useCallback((key, value) => {
        setWatchAlertDraft((prev) => ({ ...prev, [key]: value }));
    }, []);

    const buildSavePayload = useCallback(() => {
        const walletMinWallets = parseGoldenDogRequiredInt(draft.wallet_mode.min_wallets, '钱包数量');
        const walletWindowMinutes = parseGoldenDogRequiredInt(draft.wallet_mode.window_minutes, '统计窗口');
        const walletCooldownMinutes = parseGoldenDogRequiredInt(draft.wallet_mode.cooldown_minutes, '冷却时间', { min: 0 });
        const walletMinTotalAmountUSD = parseGoldenDogOptionalNumber(draft.wallet_mode.min_total_amount_usd, '最低合计金额');
        const walletIntensityMode = draft.wallet_mode.intensity_mode === 'amount_tiers' ? 'amount_tiers' : 'fixed';
        const walletAmountIntensityTiers = walletIntensityMode === 'amount_tiers'
            ? parseGoldenDogAmountIntensityTiers(draft.wallet_mode.amount_intensity_tiers, walletIntensityMode)
            : [];
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
                min_total_amount_usd: walletMinTotalAmountUSD,
                intensity: draft.wallet_mode.intensity || 'ring',
                intensity_mode: walletIntensityMode,
                amount_intensity_tiers: walletAmountIntensityTiers,
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
            if (activeTab === 'watch_open') {
                const resp = await saveSMWatchOpenAlertConfig({
                    apiBaseUrl,
                    initData,
                    chain: 'bsc',
                    config: {
                        enabled: Boolean(watchAlertDraft.enabled),
                        bark_enabled: Boolean(watchAlertDraft.bark_enabled),
                        sound_enabled: Boolean(watchAlertDraft.sound_enabled),
                    },
                });
                applyWatchAlertResponse(resp);
                setNotice('特别关注开仓配置已保存');
            } else {
                const resp = await saveSMGoldenDogConfig({
                    apiBaseUrl,
                    initData,
                    chain: 'bsc',
                    config: buildSavePayload(),
                });
                applyResponse(resp);
                setNotice('配置已保存');
            }
        } catch (err) {
            setError(String(err?.message || err || '保存失败'));
        } finally {
            setSaving(false);
        }
    }, [activeTab, apiBaseUrl, applyResponse, applyWatchAlertResponse, buildSavePayload, hasInitData, initData, watchAlertDraft]);

    const handleTest = useCallback(async (mode) => {
        if (mode !== 'watch_sound' && !hasInitData) {
            setError('请先登录 Telegram 后再测试监控通知。');
            return;
        }

        setTestingMode(mode);
        setError('');
        setNotice('');
        try {
            if (mode === 'watch_bark') {
                const resp = await testSMWatchOpenAlertConfig({ apiBaseUrl, initData, chain: 'bsc' });
                setNotice(resp?.message || '特别关注开仓 Bark 测试已发送');
            } else if (mode === 'watch_sound') {
                const ok = await playSmartMoneyBeep();
                if (!ok) {
                    throw new Error('当前环境不支持提示音播放。');
                }
                setNotice('提示音已播放');
            } else {
                const intensity = mode === 'pool' ? draft.pool_mode.intensity : goldenDogWalletTestIntensity(draft.wallet_mode);
                const resp = await testSMGoldenDogConfig({
                    apiBaseUrl,
                    initData,
                    chain: 'bsc',
                    mode,
                    intensity,
                });
                setNotice(resp?.message || '测试通知已发送');
            }
        } catch (err) {
            setError(String(err?.message || err || '测试失败'));
        } finally {
            setTestingMode('');
        }
    }, [apiBaseUrl, draft.pool_mode.intensity, draft.wallet_mode, hasInitData, initData]);

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
    const pillBtnBlue = (active) => `rounded-lg px-2 py-[5px] text-[10px] font-semibold transition-all ${active ? 'bg-sky-400/15 text-sky-200' : 'bg-white/[0.04] text-zinc-500'}`;

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
                            <span className={`inline-block rounded px-1 py-[1px] text-[9px] leading-none ${watchAlertDraft.enabled ? 'bg-sky-400/12 text-sky-300' : 'bg-zinc-800 text-zinc-500'}`}>特别关注{watchAlertDraft.enabled ? '✓' : '✗'}</span>
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
            {!error && lastWatchEventText ? <div className="rounded-lg border border-sky-500/20 bg-sky-500/10 px-2.5 py-1.5 text-[11px] text-sky-200">最近提醒：{lastWatchEventText}</div> : null}
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
                        <button type="button" onClick={() => setActiveTab('watch_open')}
                            className={`flex-1 rounded-[10px] py-1.5 text-[11px] font-semibold transition-all ${activeTab === 'watch_open' ? 'bg-sky-400/15 text-sky-300' : 'text-zinc-500'}`}
                        >特别关注开仓</button>
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
                                <span className="text-zinc-400">金额 <span className="text-zinc-100">{goldenDogThresholdText(draft.wallet_mode.min_total_amount_usd, '$')}</span></span>
                                <span className="text-zinc-400">窗口 <span className="text-zinc-100">{draft.wallet_mode.window_minutes || '--'}m</span></span>
                                <span className="text-zinc-400">冷却 <span className="text-zinc-100">{draft.wallet_mode.cooldown_minutes || '--'}m</span></span>
                                <span className="text-zinc-400">强度 <span className="text-zinc-100">{draft.wallet_mode.intensity_mode === 'amount_tiers' ? '金额阶梯' : goldenDogIntensityLabel(draft.wallet_mode.intensity)}</span></span>
                            </div>
                            <div className="mt-3 grid grid-cols-2 gap-1.5">
                                <input className={miniInputCls} type="number" min="1" step="1" placeholder="钱包数量" value={draft.wallet_mode.min_wallets} onChange={(e) => updateWalletMode('min_wallets', e.target.value)} />
                                <input className={miniInputCls} type="number" min="1" step="1" placeholder="窗口(分钟)" value={draft.wallet_mode.window_minutes} onChange={(e) => updateWalletMode('window_minutes', e.target.value)} />
                                <input className={miniInputCls} type="number" min="0" step="1" placeholder="冷却(分钟)" value={draft.wallet_mode.cooldown_minutes} onChange={(e) => updateWalletMode('cooldown_minutes', e.target.value)} />
                                <input className={miniInputCls} type="number" min="0" step="0.01" placeholder="最低金额($)" value={draft.wallet_mode.min_total_amount_usd} onChange={(e) => updateWalletMode('min_total_amount_usd', e.target.value)} />
                                <MiniSelect value={draft.wallet_mode.intensity_mode} options={GOLDEN_DOG_INTENSITY_MODE_OPTIONS} onChange={(v) => updateWalletMode('intensity_mode', v)} />
                                <MiniSelect value={draft.wallet_mode.intensity} options={intensityOptions} onChange={(v) => updateWalletMode('intensity', v)} />
                            </div>
                            {draft.wallet_mode.intensity_mode === 'amount_tiers' && (
                                <div className="mt-2 grid grid-cols-1 gap-1.5">
                                    {(draft.wallet_mode.amount_intensity_tiers || cloneGoldenDogDefaultAmountTiers()).map((tier, index) => (
                                        <div key={index} className="grid grid-cols-2 gap-1.5 rounded-xl border border-white/[0.05] bg-black/20 p-1.5">
                                            <input className={miniInputCls} type="number" min="0" step="0.01" placeholder={`第${index + 1}档金额($)`} value={tier.min_amount_usd} onChange={(e) => updateWalletAmountTier(index, 'min_amount_usd', e.target.value)} />
                                            <MiniSelect value={tier.intensity} options={intensityOptions} onChange={(v) => updateWalletAmountTier(index, 'intensity', v)} />
                                        </div>
                                    ))}
                                </div>
                            )}
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

                    {activeTab === 'watch_open' && (
                        <section className="rounded-2xl border border-sky-400/10 bg-gradient-to-b from-[rgba(16,23,32,0.96)] to-[rgba(8,12,18,0.98)] p-3">
                            <div className="flex items-start justify-between gap-2">
                                <div className="min-w-0">
                                    <div className="text-[12px] font-semibold text-zinc-100">特别关注开仓</div>
                                    <div className="text-[10px] text-zinc-500">特别关注的钱包一旦开仓，可发 Bark，页面前台可播放一声“滴”。</div>
                                </div>
                                <div className="flex items-center gap-1">
                                    <button type="button" className={pillBtnBlue(watchAlertDraft.enabled)} onClick={() => updateWatchAlertMode('enabled', true)}>开启</button>
                                    <button type="button" className={pillBtnBlue(!watchAlertDraft.enabled)} onClick={() => updateWatchAlertMode('enabled', false)}>关闭</button>
                                </div>
                            </div>

                            <div className="mt-3 grid grid-cols-2 gap-1.5">
                                <div className="rounded-lg border border-white/[0.07] bg-black/20 px-2.5 py-2">
                                    <div className="text-[10px] text-zinc-500">通知状态</div>
                                    <div className="mt-1 text-[11px] font-semibold text-zinc-100">{smartMoneyWatchOpenStatusText(watchAlertDraft)}</div>
                                </div>
                                <div className="rounded-lg border border-white/[0.07] bg-black/20 px-2.5 py-2">
                                    <div className="text-[10px] text-zinc-500">Bark 状态</div>
                                    <div className="mt-1 text-[11px] font-semibold text-zinc-100">{watchBarkStatusText}</div>
                                </div>
                                <div className="rounded-lg border border-white/[0.07] bg-black/20 px-2.5 py-2">
                                    <div className="text-[10px] text-zinc-500">特别关注钱包</div>
                                    <div className="mt-1 text-[11px] font-semibold text-zinc-100">{watchedWalletList.length} 个</div>
                                </div>
                                <div className="rounded-lg border border-white/[0.07] bg-black/20 px-2.5 py-2">
                                    <div className="text-[10px] text-zinc-500">前台音效</div>
                                    <div className="mt-1 text-[11px] font-semibold text-zinc-100">{watchAlertDraft.sound_enabled ? '已开启' : '已关闭'}</div>
                                </div>
                            </div>

                            <div className="mt-3 grid grid-cols-2 gap-1.5">
                                <button
                                    type="button"
                                    className={`rounded-xl border px-3 py-2 text-[11px] font-semibold transition ${watchAlertDraft.bark_enabled ? 'border-sky-400/25 bg-sky-400/12 text-sky-200' : 'border-white/[0.05] bg-zinc-900/60 text-zinc-400'}`}
                                    onClick={() => updateWatchAlertMode('bark_enabled', !watchAlertDraft.bark_enabled)}
                                >
                                    Bark 推送 {watchAlertDraft.bark_enabled ? '已开' : '已关'}
                                </button>
                                <button
                                    type="button"
                                    className={`rounded-xl border px-3 py-2 text-[11px] font-semibold transition ${watchAlertDraft.sound_enabled ? 'border-sky-400/25 bg-sky-400/12 text-sky-200' : 'border-white/[0.05] bg-zinc-900/60 text-zinc-400'}`}
                                    onClick={() => updateWatchAlertMode('sound_enabled', !watchAlertDraft.sound_enabled)}
                                >
                                    前台提示音 {watchAlertDraft.sound_enabled ? '已开' : '已关'}
                                </button>
                                <button
                                    type="button"
                                    className="rounded-xl border border-white/[0.05] bg-zinc-900/60 px-3 py-2 text-[11px] font-semibold text-zinc-200 transition disabled:opacity-50"
                                    disabled={testingMode === 'watch_bark' || !hasInitData}
                                    onClick={() => handleTest('watch_bark')}
                                >
                                    {testingMode === 'watch_bark' ? '测试中...' : '测试 Bark'}
                                </button>
                                <button
                                    type="button"
                                    className="rounded-xl border border-white/[0.05] bg-zinc-900/60 px-3 py-2 text-[11px] font-semibold text-zinc-200 transition disabled:opacity-50"
                                    disabled={testingMode === 'watch_sound'}
                                    onClick={() => handleTest('watch_sound')}
                                >
                                    {testingMode === 'watch_sound' ? '试听中...' : '试听提示音'}
                                </button>
                            </div>

                            <div className="mt-3 rounded-xl border border-white/[0.05] bg-zinc-900/45 px-3 py-2 text-[10px] text-zinc-500">
                                Bark Key / Server / Group 继续复用全局配置。
                            </div>

                            <div className="mt-3 space-y-2">
                                <div className="text-[11px] font-semibold text-sky-200">特别关注列表</div>
                                {watchedWalletList.length ? watchedWalletList.map((walletAddress) => (
                                    <div key={walletAddress} className="flex items-center justify-between gap-3 rounded-xl border border-white/[0.05] bg-black/20 px-3 py-2">
                                        <div className="flex min-w-0 items-center gap-2.5">
                                            <img
                                                src={resolveWalletAvatarSrc(walletAddress)}
                                                alt=""
                                                className="h-9 w-9 flex-shrink-0 rounded-full"
                                            />
                                            <div className="min-w-0">
                                                <div className="text-[12px] font-semibold text-zinc-100">尾号 {tailAddr(walletAddress)}</div>
                                                <div className="text-[10px] text-zinc-400">{shortAddr(walletAddress)}</div>
                                            </div>
                                        </div>
                                        <button
                                            type="button"
                                            className="rounded-lg border border-red-500/20 bg-red-500/10 px-2.5 py-1.5 text-[10px] font-semibold text-red-200 transition disabled:opacity-50"
                                            disabled={Boolean(watchToggleMap[walletAddress])}
                                            onClick={() => onToggleWatchWallet?.(walletAddress)}
                                        >
                                            {watchToggleMap[walletAddress] ? '处理中...' : '移除'}
                                        </button>
                                    </div>
                                )) : (
                                    <div className="rounded-xl border border-dashed border-white/[0.08] bg-white/[0.02] px-3 py-3 text-[10px] text-zinc-500">
                                        暂无特别关注钱包，可在钱包视图里点按钮加入。
                                    </div>
                                )}
                            </div>
                        </section>
                    )}
                </div>
            )}
        </div>
    );
}

const GOLDEN_DOG_INTENSITY_OPTIONS = [
    { value: 'ring', label: '响铃', description: '普通提醒' },
    { value: 'persistent_ring', label: '持续响铃', description: '持续提醒' },
    { value: 'critical_ring', label: '静音强提醒', description: '静音也响' },
];

const GOLDEN_DOG_INTENSITY_MODE_OPTIONS = [
    { value: 'fixed', label: '固定强度' },
    { value: 'amount_tiers', label: '按金额阶梯' },
];

const GOLDEN_DOG_DEFAULT_AMOUNT_TIERS = [
    { min_amount_usd: '1000', intensity: 'ring' },
    { min_amount_usd: '5000', intensity: 'persistent_ring' },
    { min_amount_usd: '20000', intensity: 'critical_ring' },
];

function createGoldenDogDraft() {
    return {
        wallet_mode: {
            enabled: false,
            min_wallets: '3',
            window_minutes: '10',
            cooldown_minutes: '30',
            min_total_amount_usd: '',
            intensity: 'ring',
            intensity_mode: 'fixed',
            amount_intensity_tiers: cloneGoldenDogDefaultAmountTiers(),
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

function mapGoldenDogAmountTiers(value) {
    let rows = value;
    if (typeof rows === 'string' && rows.trim()) {
        try {
            rows = JSON.parse(rows);
        } catch {
            rows = null;
        }
    }
    if (!Array.isArray(rows) || rows.length === 0) {
        return cloneGoldenDogDefaultAmountTiers();
    }
    const mapped = rows.slice(0, 3).map((tier, index) => ({
        min_amount_usd: formatGoldenDogDraftValue(tier?.min_amount_usd, { emptyWhenZero: false }),
        intensity: String(tier?.intensity || GOLDEN_DOG_DEFAULT_AMOUNT_TIERS[index]?.intensity || 'ring'),
    }));
    while (mapped.length < 3) {
        mapped.push({ ...GOLDEN_DOG_DEFAULT_AMOUNT_TIERS[mapped.length] });
    }
    return mapped;
}

function mapGoldenDogConfigToDraft(cfg) {
    const next = createGoldenDogDraft();
    const source = cfg || {};
    next.wallet_mode.enabled = Boolean(source.enabled);
    next.wallet_mode.min_wallets = String(source.min_wallets ?? 3);
    next.wallet_mode.window_minutes = String(source.window_minutes ?? 10);
    next.wallet_mode.cooldown_minutes = String(source.cooldown_minutes ?? 30);
    next.wallet_mode.min_total_amount_usd = formatGoldenDogDraftValue(source.wallet_min_total_amount_usd, { emptyWhenZero: true });
    next.wallet_mode.intensity = String(source.wallet_intensity || 'ring');
    next.wallet_mode.intensity_mode = String(source.wallet_intensity_mode || 'fixed');
    next.wallet_mode.amount_intensity_tiers = mapGoldenDogAmountTiers(source.wallet_amount_intensity_tiers);
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

function parseGoldenDogAmountIntensityTiers(rows, intensityMode) {
    const parsed = (Array.isArray(rows) ? rows : [])
        .map((tier, index) => ({
            min_amount_usd: parseGoldenDogOptionalNumber(tier?.min_amount_usd, `第 ${index + 1} 档金额`),
            intensity: tier?.intensity || 'ring',
        }))
        .filter((tier) => tier.min_amount_usd > 0)
        .sort((a, b) => a.min_amount_usd - b.min_amount_usd);
    if (intensityMode === 'amount_tiers' && parsed.length === 0) {
        throw new Error('按金额阶梯推送至少需要填写一档金额。');
    }
    return parsed;
}

function goldenDogBarkStatusText(status) {
    if (status?.bark_ready) return '已就绪';
    if (status?.bark_configured) return status?.bark_enabled ? '已配置未就绪' : '已配置未开启';
    return '未配置';
}

function goldenDogIntensityLabel(value) {
    return GOLDEN_DOG_INTENSITY_OPTIONS.find((item) => item.value === value)?.label || '响铃';
}

function goldenDogThresholdText(value, prefix = '', suffix = '') {
    const raw = String(value || '').trim();
    return raw ? `${prefix}${raw}${suffix}` : '--';
}

function goldenDogWalletTestIntensity(walletMode) {
    if (walletMode?.intensity_mode !== 'amount_tiers') {
        return walletMode?.intensity || 'ring';
    }
    const tiers = parseGoldenDogAmountIntensityTiers(walletMode?.amount_intensity_tiers, 'fixed');
    return tiers[tiers.length - 1]?.intensity || walletMode?.intensity || 'ring';
}

function createWatchOpenAlertDraft() {
    return {
        enabled: false,
        bark_enabled: false,
        sound_enabled: false,
    };
}

function mapWatchOpenAlertConfigToDraft(cfg) {
    const source = cfg || {};
    return {
        enabled: Boolean(source.enabled),
        bark_enabled: Boolean(source.bark_enabled),
        sound_enabled: Boolean(source.sound_enabled),
    };
}

function smartMoneyWatchOpenStatusText(draft) {
    if (draft?.enabled) return '运行中';
    return '已暂停';
}

let smartMoneyBeepAudioContext = null;

async function playSmartMoneyBeep() {
    if (typeof window === 'undefined') return false;
    const AudioCtx = window.AudioContext || window.webkitAudioContext;
    if (!AudioCtx) return false;
    if (!smartMoneyBeepAudioContext) {
        smartMoneyBeepAudioContext = new AudioCtx();
    }

    const ctx = smartMoneyBeepAudioContext;
    if (ctx.state === 'suspended') {
        try {
            await ctx.resume();
        } catch {
            return false;
        }
    }

    // 创建一个更悦耳的三音符上升旋律
    const notes = [
        { freq: 523.25, start: 0, duration: 0.12 },      // C5
        { freq: 659.25, start: 0.10, duration: 0.12 },   // E5
        { freq: 783.99, start: 0.20, duration: 0.18 }    // G5
    ];

    const masterGain = ctx.createGain();
    masterGain.gain.setValueAtTime(0.15, ctx.currentTime);
    masterGain.connect(ctx.destination);

    notes.forEach(note => {
        const osc = ctx.createOscillator();
        const gain = ctx.createGain();

        osc.type = 'sine';
        osc.frequency.setValueAtTime(note.freq, ctx.currentTime + note.start);

        // 平滑的音量包络
        gain.gain.setValueAtTime(0.0001, ctx.currentTime + note.start);
        gain.gain.exponentialRampToValueAtTime(1, ctx.currentTime + note.start + 0.02);
        gain.gain.exponentialRampToValueAtTime(0.0001, ctx.currentTime + note.start + note.duration);

        osc.connect(gain);
        gain.connect(masterGain);
        osc.start(ctx.currentTime + note.start);
        osc.stop(ctx.currentTime + note.start + note.duration);
    });

    return true;
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

function walletSourceLabel(source) {
    const value = String(source || '').trim();
    if (value === 'manual') return '手动添加';
    if (value === 'contract_interaction') return '合约发现';
    if (value === 'token_liquidity_indexer') return '雷达发现';
    if (value === 'pool_liquidity_radar') return '池子雷达';
    return value || '未标记来源';
}

function walletSourceBadgeClass(source) {
    const value = String(source || '').trim();
    if (value === 'manual') return 'border-emerald-500/20 bg-emerald-500/10 text-emerald-300';
    if (value === 'token_liquidity_indexer' || value === 'pool_liquidity_radar') return 'border-sky-400/20 bg-sky-400/10 text-sky-200';
    return 'border-white/10 bg-zinc-800/80 text-zinc-300';
}

function walletSourceContractLabel(value) {
    const address = normalizeWalletAddress(value);
    if (address) return `来源合约 ${shortAddr(address)}`;
    const poolId = String(value || '').trim();
    if (/^0x[a-fA-F0-9]{64}$/.test(poolId)) return `来源 poolId ${shortAddr(poolId)}`;
    return '';
}

function parsePoolMetricNumber(value) {
    if (value === null || value === undefined || value === '') return NaN;
    const raw = typeof value === 'string' ? value.replace(/,/g, '').trim() : value;
    const direct = Number(raw);
    if (Number.isFinite(direct)) return direct;
    const match = String(value).match(/-?\d+(\.\d+)?/);
    if (!match) return NaN;
    const parsed = Number(match[0]);
    return Number.isFinite(parsed) ? parsed : NaN;
}

function resolvePositionPreviewFeeUsd(detail, position) {
    const liveFee = parsePoolMetricNumber(detail?.totals?.fee_usd);
    if (Number.isFinite(liveFee)) return liveFee;
    if (String(position?.fee_status || '').trim() === 'unavailable') return NaN;
    return parsePoolMetricNumber(position?.fee_usd);
}

function resolveSmartMoneyPoolMarketCapDisplay(pool) {
    const candidates = [
        pool?.fdv_usd,
        pool?.current_token_fdv_usd,
    ];
    for (const candidate of candidates) {
        const value = parsePoolMetricNumber(candidate);
        if (Number.isFinite(value) && value > 0) return value;
    }
    return NaN;
}

function resolveSmartMoneyPoolMarketCapLabel() {
    return 'FDV';
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

function getPairInitials(value) {
    return getPairLabel(value)
        .split(/[/-]/)
        .map((part) => String(part || '').trim().charAt(0).toUpperCase())
        .join('')
        .slice(0, 2) || 'LP';
}

const SMART_MONEY_POOL_FILTER_STORAGE_KEY = 'tglp_smart_money_pool_filter_v1';
const EMPTY_SMART_MONEY_POOL_FILTER = { minSmartMoneyUsd: null, maxFeeRate: null, minMarketCapUsd: null };
const SMART_MONEY_POOL_SOURCE_TABS = [
    { key: 'all', label: '全部', source: '' },
    { key: 'manual', label: '手动添加', source: 'manual' },
    { key: 'contract', label: '合约发现', source: 'contract_interaction' },
];
const SMART_MONEY_POOL_SOURCE_BY_KEY = Object.fromEntries(
    SMART_MONEY_POOL_SOURCE_TABS.map((item) => [item.key, item.source]),
);

function normalizeStoredSmartMoneyPoolFilter(value) {
    if (!value || typeof value !== 'object') {
        return { ...EMPTY_SMART_MONEY_POOL_FILTER };
    }
    return {
        minSmartMoneyUsd: Number.isFinite(Number(value.minSmartMoneyUsd)) ? Number(value.minSmartMoneyUsd) : null,
        maxFeeRate: Number.isFinite(Number(value.maxFeeRate)) ? Number(value.maxFeeRate) : null,
        minMarketCapUsd: Number.isFinite(Number(value.minMarketCapUsd)) ? Number(value.minMarketCapUsd) : null,
    };
}

function cloneGoldenDogDefaultAmountTiers() {
    return GOLDEN_DOG_DEFAULT_AMOUNT_TIERS.map((tier) => ({ ...tier }));
}

function readStoredSmartMoneyPoolFilter() {
    if (typeof window === 'undefined' || !window.localStorage) {
        return { ...EMPTY_SMART_MONEY_POOL_FILTER };
    }
    try {
        const raw = window.localStorage.getItem(SMART_MONEY_POOL_FILTER_STORAGE_KEY);
        if (!raw) return { ...EMPTY_SMART_MONEY_POOL_FILTER };
        return normalizeStoredSmartMoneyPoolFilter(JSON.parse(raw));
    } catch {
        return { ...EMPTY_SMART_MONEY_POOL_FILTER };
    }
}

function writeStoredSmartMoneyPoolFilter(value) {
    if (typeof window === 'undefined' || !window.localStorage) {
        return;
    }
    try {
        const normalized = normalizeStoredSmartMoneyPoolFilter(value);
        const isEmpty = !Number.isFinite(normalized.minSmartMoneyUsd)
            && !Number.isFinite(normalized.maxFeeRate)
            && !Number.isFinite(normalized.minMarketCapUsd);
        if (isEmpty) {
            window.localStorage.removeItem(SMART_MONEY_POOL_FILTER_STORAGE_KEY);
            return;
        }
        window.localStorage.setItem(SMART_MONEY_POOL_FILTER_STORAGE_KEY, JSON.stringify(normalized));
    } catch {
        // ignore storage failures
    }
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
const POSITION_PREVIEW_BATCH_SIZE = 4;
const POOL_LIST_PAGE_SIZE = 10;
const POSITION_LIST_PAGE_SIZE = 6;
const WALLET_LIST_PAGE_SIZE = 10;
const POOL_HEATMAP_PAGE_SIZE = 10;
const WATCH_ACTIVITY_PAGE_SIZE = 12;
const POOL_HEATMAP_WINDOWS = [
    { key: '30s', label: '30s' },
    { key: '1m', label: '1min' },
    { key: '5m', label: '5min' },
    { key: '1h', label: '1h' },
];
const POOL_HEATMAP_SORTS = [
    { key: 'rate', label: '速率' },
    { key: 'fee', label: '手续费' },
];
function heatmapWindowLabel(value) {
    return POOL_HEATMAP_WINDOWS.find((item) => item.key === value)?.label || '1min';
}

function formatHeatmapAge(seconds) {
    const value = Number(seconds);
    if (!Number.isFinite(value) || value <= 0) return '--';
    if (value < 60) return `${Math.max(1, Math.round(value))}秒`;
    if (value < 3600) return `${Math.round(value / 60)}分钟`;
    if (value < 86400) return `${Math.round(value / 3600)}小时`;
    return `${Math.round(value / 86400)}天`;
}

function formatWatchActivityAction(value) {
    const eventType = String(value || '').trim();
    if (eventType === 'add') return '加 LP';
    if (eventType === 'remove') return '撤 LP';
    return eventType || 'LP 操作';
}

function getWatchActivityActionClass(value) {
    return String(value || '').trim() === 'remove'
        ? 'border-red-500/20 bg-red-500/10 text-red-300'
        : 'border-emerald-500/20 bg-emerald-500/10 text-emerald-300';
}

function normalizeWatchWalletItems(resp, fallbackWallets = []) {
    const items = Array.isArray(resp?.items) ? resp.items : [];
    const seen = new Set();
    const normalized = [];

    items.forEach((item) => {
        const address = normalizeWalletAddress(item?.wallet_address);
        if (!address || seen.has(address)) return;
        seen.add(address);
        normalized.push({ ...item, wallet_address: address });
    });

    fallbackWallets.forEach((value) => {
        const address = normalizeWalletAddress(value);
        if (!address || seen.has(address)) return;
        seen.add(address);
        normalized.push({ wallet_address: address, wallet_color: '#7F77DD' });
    });

    return normalized;
}

function heatmapSampleText(row) {
    const status = String(row?.sample_status || '').trim();
    if (status === 'ok') return '样本完整';
    if (status === 'partial') return '部分样本';
    return '样本不足';
}

function useSmartMoneyPositionPreviewMap(apiBaseUrl, positions) {
    const [previewMap, setPreviewMap] = useState({});

    useEffect(() => {
        const rows = Array.isArray(positions) ? positions : [];
        if (rows.length === 0) {
            setPreviewMap({});
            return undefined;
        }

        let cancelled = false;
        setPreviewMap({});

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
                        rangeText: data?.in_range === true ? '区间内' : data?.in_range === false ? '已离开区间' : '',
                        feeUsd: resolvePositionPreviewFeeUsd(data, position),
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
                        feeUsd: resolvePositionPreviewFeeUsd(null, position),
                        runningSince: String(prev[key]?.runningSince || position?.opened_at || '').trim(),
                    },
                }));
            }
        };

        (async () => {
            for (let index = 0; index < rows.length && !cancelled; index += POSITION_PREVIEW_BATCH_SIZE) {
                const batch = rows.slice(index, index + POSITION_PREVIEW_BATCH_SIZE);
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

function WalletIdentity({ address, color, label, avatarUrl, source, sourceContract, size = 40, onClick, showCopy = false, showSource = false }) {
    const sourceText = walletSourceLabel(source);
    const sourceContractText = walletSourceContractLabel(sourceContract);
    const inner = (
        <>
            <WalletAvatar address={address} color={color} avatarUrl={avatarUrl} size={size} />
            <span className="truncate text-left text-sm text-zinc-100">
                {label && label !== address ? label : shortAddr(address)}
            </span>
            {showSource ? (
                <Badge className={walletSourceBadgeClass(source)} title={sourceContractText || sourceText}>
                    {sourceText}
                </Badge>
            ) : null}
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

function PositionPreviewMetricsLite({ position, preview, compact = false }) {
    const rangeText = String(preview?.rangeText || '').trim() || '--';
    const rangeMetricClass = rangeText === '区间内'
        ? 'border-emerald-500/20 bg-emerald-500/10 text-emerald-300'
        : rangeText === '已离开区间'
            ? 'border-red-500/20 bg-red-500/10 text-red-300'
            : 'border-white/[0.05] bg-black/20 text-zinc-300';
    const rangeLabelClass = rangeText === '区间内'
        ? 'text-emerald-200'
        : rangeText === '已离开区间'
            ? 'text-red-200'
            : 'text-zinc-100';
    const previewFeeValue = Number(preview?.feeUsd);
    const feeValue = Number.isFinite(previewFeeValue) ? previewFeeValue : resolvePositionPreviewFeeUsd(null, position);
    const feeText = Number.isFinite(feeValue) ? formatPreviewUsd(feeValue) : '--';
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
    const runningText = formatDurationFrom(preview?.runningSince || position?.opened_at) || '--';
    const runtimeMetricClass = runningText !== '--'
        ? 'border-emerald-500/20 bg-emerald-500/10 text-emerald-300'
        : 'border-white/[0.05] bg-black/20 text-zinc-300';
    const runtimeLabelClass = runningText !== '--' ? 'text-emerald-200' : 'text-zinc-100';

    return (
        <div className={`mt-2 flex flex-wrap items-stretch gap-2 ${compact ? 'pt-2 border-t border-white/[0.05]' : ''}`}>
            <span className={`inline-flex min-w-[104px] items-center justify-between gap-2 whitespace-nowrap rounded-full border px-2.5 py-1 text-[10px] ${rangeMetricClass}`}>
                <strong className={`font-semibold ${rangeLabelClass}`}>区间状态</strong>
                <span className="text-right tabular-nums">{rangeText}</span>
            </span>
            <span className={`inline-flex min-w-[104px] items-center justify-between gap-2 whitespace-nowrap rounded-full border px-2.5 py-1 text-[10px] ${feeMetricClass}`}>
                <strong className={`font-semibold ${feeLabelClass}`}>手续费</strong>
                <span className="text-right tabular-nums">{feeText}</span>
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

function SmartMoneyPositionDetailPanel({ apiBaseUrl, position, onClose }) {
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
                    allowTaskActions={false}
                    showAbsolutePnl={false}
                    headerAccessory={(
                        <div className="flex items-center gap-1">
                            <Badge className={walletSourceBadgeClass(detail.wallet_source)}>
                                {walletSourceLabel(detail.wallet_source)}
                            </Badge>
                            {walletSourceContractLabel(detail.wallet_source_contract) ? (
                                <Badge className="hidden border-white/10 bg-zinc-800/80 text-zinc-400 sm:inline-flex">
                                    {walletSourceContractLabel(detail.wallet_source_contract)}
                                </Badge>
                            ) : null}
                            <button
                                type="button"
                                onClick={onClose}
                                aria-label="收起详情"
                                className="inline-flex h-8 w-8 shrink-0 items-center justify-center rounded-xl border border-white/[0.06] bg-black/20 text-zinc-400 transition hover:bg-black/30 hover:text-zinc-200"
                            >
                                <X size={15} />
                            </button>
                        </div>
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
                            acc[p.wallet_address] = {
                                color: p.wallet_color,
                                label: p.wallet_label,
                                source: p.wallet_source,
                                sourceContract: p.wallet_source_contract,
                            };
                        }
                        return acc;
                    }, {})
                ).map(([addr, { color, label, source, sourceContract }]) => (
                    <span key={addr} className="flex items-center gap-1 text-zinc-400">
                        <span className="inline-block w-2 h-2 rounded-full" style={{ backgroundColor: color }} />
                        {label || shortAddr(addr)}
                        <Badge className={walletSourceBadgeClass(source)} title={walletSourceContractLabel(sourceContract) || walletSourceLabel(source)}>
                            {walletSourceLabel(source)}
                        </Badge>
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

function formatZombieLastActive(value) {
    const raw = String(value || '').trim();
    if (!raw) return '从未活动';
    return relativeTime(raw) || raw.slice(0, 10);
}

function zombieHistoryCount(item) {
    return Number(item?.total_event_count || 0)
        + Number(item?.position_count || 0)
        + Number(item?.active_position_count || 0)
        + Number(item?.snapshot_count || 0)
        + Number(item?.transfer_event_count || 0)
        + Number(item?.daily_stat_count || 0)
        + Number(item?.live_state_count || 0);
}

function ZombieWalletSheet({ open, candidates, selectedMap, busy, onToggle, onToggleAll, onClose, onDelete }) {
    if (!open) return null;
    const list = Array.isArray(candidates) ? candidates : [];
    const selectedCount = list.filter((item) => selectedMap[`${item.address}:${item.chain_id}`]).length;
    const allSelected = list.length > 0 && selectedCount === list.length;

    return (
        <div className="fixed inset-0 z-50 flex items-end justify-center bg-black/70 p-3 sm:items-center" onClick={busy ? undefined : onClose}>
            <div
                className="flex max-h-[88vh] w-full max-w-xl flex-col rounded-[28px] border border-white/[0.06] bg-zinc-950/95 p-4 shadow-[0_24px_80px_-32px_rgba(0,0,0,0.95)]"
                onClick={(e) => e.stopPropagation()}
            >
                <div className="flex items-start justify-between gap-3">
                    <div>
                        <h3 className="text-base font-semibold text-zinc-100">僵尸钱包</h3>
                        <p className="mt-1 text-xs leading-5 text-zinc-400">
                            最近 30 天没有 LP 开仓或撤仓的钱包。确认删除后会同时删除该钱包的聪明钱历史数据。
                        </p>
                    </div>
                    <button
                        type="button"
                        onClick={onClose}
                        disabled={busy}
                        className="inline-flex h-9 w-9 shrink-0 items-center justify-center rounded-xl border border-white/[0.06] bg-zinc-900/70 text-zinc-400 transition hover:text-zinc-200 disabled:opacity-50"
                    >
                        <X size={16} />
                    </button>
                </div>

                {list.length > 0 ? (
                    <>
                        <div className="mt-4 flex items-center justify-between gap-3 text-xs text-zinc-400">
                            <button
                                type="button"
                                onClick={() => onToggleAll(!allSelected)}
                                disabled={busy}
                                className="rounded-xl border border-white/[0.06] bg-zinc-900/70 px-3 py-1.5 text-zinc-300 transition hover:bg-zinc-800/80 disabled:opacity-50"
                            >
                                {allSelected ? '取消全选' : '全选'}
                            </button>
                            <span>{selectedCount} / {list.length} 已选择</span>
                        </div>
                        <div className="mt-3 min-h-0 flex-1 space-y-2 overflow-y-auto pr-1">
                            {list.map((item) => {
                                const key = `${item.address}:${item.chain_id}`;
                                const checked = Boolean(selectedMap[key]);
                                return (
                                    <label
                                        key={key}
                                        className={`flex cursor-pointer items-center gap-3 rounded-2xl border p-3 transition ${
                                            checked
                                                ? 'border-amber-300/25 bg-amber-300/10'
                                                : 'border-white/[0.05] bg-zinc-900/60 hover:bg-zinc-900/80'
                                        }`}
                                    >
                                        <input
                                            type="checkbox"
                                            checked={checked}
                                            disabled={busy}
                                            onChange={() => onToggle(key)}
                                            className="h-4 w-4 accent-amber-300"
                                        />
                                        <WalletAvatar address={item.address} avatarUrl={item.avatar_url} size={34} />
                                        <div className="min-w-0 flex-1">
                                            <div className="truncate text-sm font-semibold text-zinc-100">{item.label || shortAddr(item.address)}</div>
                                            <div className="mt-0.5 truncate text-[11px] text-zinc-500">
                                                {shortAddr(item.address)} · {walletSourceLabel(item.source)} · 最后活动 {formatZombieLastActive(item.last_active_at)}
                                            </div>
                                        </div>
                                        <div className="shrink-0 text-right">
                                            <div className="text-sm font-semibold text-zinc-100">{zombieHistoryCount(item)}</div>
                                            <div className="text-[10px] text-zinc-500">历史项</div>
                                        </div>
                                    </label>
                                );
                            })}
                        </div>
                    </>
                ) : (
                    <div className="mt-4 rounded-2xl border border-dashed border-white/[0.06] bg-zinc-900/45 px-4 py-8 text-center text-sm text-zinc-500">
                        没有找到僵尸钱包
                    </div>
                )}

                <div className="mt-4 flex gap-2">
                    <button
                        type="button"
                        onClick={onClose}
                        disabled={busy}
                        className="flex-1 rounded-2xl border border-white/[0.06] bg-zinc-900/70 px-4 py-2.5 text-sm text-zinc-300 transition hover:bg-zinc-800/80 disabled:opacity-50"
                    >
                        关闭
                    </button>
                    <button
                        type="button"
                        onClick={onDelete}
                        disabled={busy || selectedCount === 0}
                        className="flex-1 rounded-2xl border border-red-500/20 bg-red-500/10 px-4 py-2.5 text-sm text-red-200 transition hover:bg-red-500/15 disabled:opacity-50"
                    >
                        {busy ? '删除中...' : `删除 ${selectedCount} 个`}
                    </button>
                </div>
            </div>
        </div>
    );
}

function TokenLiquidityImportSheet({ open, apiBaseUrl, brand, onClose, onImported }) {
    const [poolInput, setPoolInput] = useState('');
    const [minAmountUsd, setMinAmountUsd] = useState('500');
    const [timeRange, setTimeRange] = useState(() => createDefaultTokenLiquidityRange());
    const [limit, setLimit] = useState('30');
    const [loading, setLoading] = useState(false);
    const [saving, setSaving] = useState(false);
    const [error, setError] = useState('');
    const [data, setData] = useState(null);
    const [selected, setSelected] = useState({});
    const [importResult, setImportResult] = useState(null);
    const [scanStep, setScanStep] = useState('');
    const [scanStartedAt, setScanStartedAt] = useState(null);
    const [scanElapsedMs, setScanElapsedMs] = useState(0);
    const [scanTarget, setScanTarget] = useState(null);
    const [scanLogs, setScanLogs] = useState([]);
    const scanAbortRef = useRef(null);

    const appendScanLog = useCallback((text, tone = 'info') => {
        setScanLogs((prev) => [...prev.slice(-7), createRadarLogEntry(text, tone)]);
    }, []);

    useEffect(() => {
        if (!open) return;
        setError('');
        setImportResult(null);
        setScanStep('');
        setScanStartedAt(null);
        setScanElapsedMs(0);
        setScanTarget(null);
        setScanLogs([]);
        setTimeRange(createDefaultTokenLiquidityRange());
        scanAbortRef.current?.abort();
        scanAbortRef.current = null;
    }, [open]);

    useEffect(() => {
        if (!open || (!loading && !saving) || !scanStartedAt) return undefined;
        const tick = () => setScanElapsedMs(Date.now() - scanStartedAt);
        tick();
        const timer = window.setInterval(tick, 1000);
        return () => window.clearInterval(timer);
    }, [open, loading, saving, scanStartedAt]);

    const candidates = Array.isArray(data?.candidates) ? data.candidates : [];
    const selectedWallets = candidates
        .filter((item) => selected[item.wallet_address])
        .map((item) => item.wallet_address);
    const allSelected = candidates.length > 0 && selectedWallets.length === candidates.length;
    const currentRangeLabel = formatTokenLiquidityDateTimeRange(timeRange.start, timeRange.end);
    const scanProgressPct = (loading || saving)
        ? Math.min(96, Math.max(8, (scanElapsedMs / SMART_MONEY_RADAR_SCAN_TIMEOUT_MS) * 96))
        : (scanStep ? 100 : 0);
    const scanTargetText = scanTarget ? `${scanTarget.kind} ${shortAddr(scanTarget.value)}` : '';
    const scanStatusText = loading ? '运行中' : saving ? '导入中' : error ? '失败' : data ? '完成' : '就绪';

    const stopScan = useCallback(() => {
        scanAbortRef.current?.abort();
        scanAbortRef.current = null;
        setLoading(false);
        setScanStep('扫描已停止');
        appendScanLog('已停止当前扫描。', 'warn');
    }, [appendScanLog]);

    const closeSheet = useCallback(() => {
        if (loading) {
            stopScan();
        }
        onClose?.();
    }, [loading, onClose, stopScan]);

    if (!open) return null;

    const preview = async () => {
        const poolTarget = parsePoolLiquidityInput(poolInput);
        if (!poolTarget) {
            setError('请输入有效的 V3 池子合约地址或 V4 poolId');
            setScanStep('扫描失败');
            setScanStartedAt(null);
            setScanElapsedMs(0);
            setScanTarget(null);
            setScanLogs([createRadarLogEntry('参数校验失败：池子地址或 poolId 格式不正确。', 'error')]);
            return;
        }
        setScanStep('校验扫描参数');
        const startTime = tokenLiquidityLocalToISO(timeRange.start);
        const endTime = tokenLiquidityLocalToISO(timeRange.end);
        if (!startTime || !endTime) {
            setError('请选择有效的开始和结束时间。');
            setScanStep('扫描失败');
            setScanStartedAt(null);
            setScanElapsedMs(0);
            setScanTarget(null);
            setScanLogs([createRadarLogEntry('参数校验失败：开始时间和结束时间必须完整。', 'error')]);
            return;
        }
        if (new Date(endTime).getTime() <= new Date(startTime).getTime()) {
            setError('结束时间必须晚于开始时间。');
            setScanStep('扫描失败');
            setScanStartedAt(null);
            setScanElapsedMs(0);
            setScanTarget(null);
            setScanLogs([createRadarLogEntry('参数校验失败：结束时间必须晚于开始时间。', 'error')]);
            return;
        }
        const startedAt = Date.now();
        const rangeHours = Math.max(1, Math.ceil((new Date(endTime).getTime() - new Date(startTime).getTime()) / (60 * 60 * 1000)));
        const targetValue = poolTarget.poolAddress || poolTarget.poolId;
        const targetKind = poolTarget.poolAddress ? 'V3 Pool' : 'V4 PoolId';
        setLoading(true);
        setError('');
        setImportResult(null);
        setData({ candidates: [], excluded_count: 0, warnings: [] });
        setSelected({});
        setScanStartedAt(startedAt);
        setScanElapsedMs(0);
        setScanTarget({
            kind: targetKind,
            value: targetValue,
            range: currentRangeLabel,
            rangeHours,
            minAmountUsd: Number(minAmountUsd),
            limit: Number(limit),
        });
        setScanLogs([
            createRadarLogEntry(`参数已校验：${targetKind} ${shortAddr(targetValue)}，约 ${rangeHours} 小时时间窗。`),
            createRadarLogEntry(`提交后端流式 RPC 扫描，最低金额 ${formatUSDCompact(Number(minAmountUsd))}，最多返回 ${Number(limit)} 个候选。`),
        ]);
        scanAbortRef.current?.abort();
        const controller = new AbortController();
        scanAbortRef.current = controller;
        const applyBatchResponse = (resp) => {
            setScanStep('整理候选钱包');
            const list = Array.isArray(resp?.candidates) ? resp.candidates : [];
            const excludedCount = Number(resp?.excluded_count || 0);
            const isPartial = Boolean(resp?.partial);
            appendScanLog(
                `后端返回：候选 ${list.length} 个，排除 ${excludedCount} 条事件。`,
                isPartial ? 'warn' : (list.length > 0 ? 'success' : 'info'),
            );
            if (isPartial) {
                appendScanLog('本次只返回部分扫描结果，列表可先查看/勾选；完整结果请缩小时间范围后再扫。', 'warn');
            }
            if (Array.isArray(resp?.warnings) && resp.warnings.length > 0) {
                appendScanLog(`扫描提示：${resp.warnings.slice(0, 2).join('；')}`, 'warn');
            }
            const nextSelected = {};
            list.forEach((item) => {
                if (!item.already_monitored) nextSelected[item.wallet_address] = true;
            });
            setData(resp);
            setSelected(nextSelected);
            setScanStep(isPartial
                ? `已返回部分结果，找到 ${list.length} 个候选钱包`
                : (list.length > 0 ? `扫描完成，找到 ${list.length} 个候选钱包` : '扫描完成，未找到符合条件的钱包'));
        };
        try {
            setScanStep('建立流式扫描连接');
            let lastStage = '';
            await streamSMPoolLiquidityWalletCandidates({
                apiBaseUrl,
                chain: 'bsc',
                ...poolTarget,
                minAmountUsd: Number(minAmountUsd),
                startTime,
                endTime,
                limit: Number(limit),
                signal: controller.signal,
                onStage: (event) => {
                    const stageText = event?.message || event?.stage || '扫描中';
                    if (event?.current_block && event?.to_block) {
                        setScanStep(`${stageText}：${event.current_block}/${event.to_block}`);
                    } else {
                        setScanStep(stageText);
                    }
                    if (event?.stage && event.stage !== lastStage) {
                        lastStage = event.stage;
                        appendScanLog(stageText);
                    }
                },
                onCandidate: (event) => {
                    const candidate = event?.candidate;
                    const wallet = String(candidate?.wallet_address || '').toLowerCase();
                    if (!wallet) return;
                    setData((prev) => ({
                        ...(prev || {}),
                        candidates: upsertPoolLiquidityCandidate(prev?.candidates, candidate),
                        excluded_count: Number(event?.excluded_count || prev?.excluded_count || 0),
                    }));
                    setSelected((prev) => {
                        if (candidate.already_monitored || Object.prototype.hasOwnProperty.call(prev, wallet)) return prev;
                        return { ...prev, [wallet]: true };
                    });
                    setScanStep(`实时发现 ${Number(event?.candidate_count || 1)} 个候选钱包`);
                    appendScanLog(`发现候选：${shortAddr(wallet)}，最大加池 ${formatUSDCompact(Number(candidate.max_amount_usd || 0))}。`, 'success');
                },
                onWarning: (event) => {
                    appendScanLog(`扫描提示：${event?.message || '扫描过程中出现提示'}`, 'warn');
                },
                onSummary: (event) => {
                    const resp = event?.response;
                    if (resp) {
                        setData(resp);
                    } else {
                        setData((prev) => ({
                            ...(prev || {}),
                            excluded_count: Number(event?.excluded_count || prev?.excluded_count || 0),
                            partial: Boolean(event?.partial),
                            warnings: Array.isArray(event?.warnings) ? event.warnings : (prev?.warnings || []),
                        }));
                    }
                    const count = Number(event?.candidate_count || resp?.candidates?.length || 0);
                    setScanStep(event?.partial ? `已返回部分结果，找到 ${count} 个候选钱包` : `扫描完成，找到 ${count} 个候选钱包`);
                },
                onDone: (event) => {
                    appendScanLog(event?.partial ? '流式扫描结束：当前为部分结果。' : '流式扫描完成。', event?.partial ? 'warn' : 'success');
                },
                onError: (event) => {
                    appendScanLog(`流式扫描失败：${event?.message || '未知错误'}`, 'error');
                },
            });
        } catch (err) {
            if (controller.signal.aborted) {
                return;
            }
            const message = String(err?.message || err || '扫描失败');
            if (/不支持流式扫描/.test(message)) {
                appendScanLog('当前环境不支持流式扫描，降级为普通扫描。', 'warn');
                try {
                    const resp = await fetchSMPoolLiquidityWalletCandidates({
                        apiBaseUrl,
                        chain: 'bsc',
                        ...poolTarget,
                        minAmountUsd: Number(minAmountUsd),
                        startTime,
                        endTime,
                        limit: Number(limit),
                    });
                    applyBatchResponse(resp);
                } catch (fallbackErr) {
                    const fallbackMessage = String(fallbackErr?.message || fallbackErr || '扫描失败');
                    setError(fallbackMessage);
                    setScanStep('扫描失败');
                    appendScanLog(`普通扫描失败：${fallbackMessage}`, 'error');
                }
                return;
            }
            setError(message);
            setScanStep('扫描失败');
            appendScanLog(`扫描失败：${message}`, 'error');
        } finally {
            if (scanAbortRef.current === controller) scanAbortRef.current = null;
            setScanElapsedMs(Date.now() - startedAt);
            setLoading(false);
        }
    };

    const importSelected = async () => {
        if (selectedWallets.length === 0) {
            setError('请至少选择一个钱包。');
            return;
        }
        const poolTarget = parsePoolLiquidityInput(poolInput);
        if (!poolTarget) {
            setError('请输入有效的 V3 池子合约地址或 V4 poolId');
            return;
        }
        setSaving(true);
        setError('');
        setScanStep('导入选中的候选钱包');
        const startedAt = Date.now();
        setScanStartedAt(startedAt);
        setScanElapsedMs(0);
        appendScanLog(`开始导入 ${selectedWallets.length} 个候选钱包。`);
        try {
            const resp = await importSMPoolLiquidityWallets({
                apiBaseUrl,
                chain: 'bsc',
                ...poolTarget,
                wallets: selectedWallets,
                labelPrefix: '雷达',
            });
            setImportResult(resp);
            setScanStep('导入完成');
            appendScanLog(`导入完成：新增 ${resp.created || 0}，恢复 ${resp.reactivated || 0}，跳过 ${resp.skipped_existing || 0}。`, 'success');
            await onImported?.();
        } catch (err) {
            const message = String(err?.message || err || '导入失败');
            setError(message);
            setScanStep('导入失败');
            appendScanLog(`导入失败：${message}`, 'error');
        } finally {
            setScanElapsedMs(Date.now() - startedAt);
            setSaving(false);
        }
    };

    return (
        <div className="fixed inset-0 z-[70] flex items-end bg-black/60 px-2 pb-2">
            <div className="max-h-[92vh] w-full overflow-hidden rounded-[28px] border border-white/10 bg-zinc-950 shadow-2xl">
                <div className="flex items-center justify-between border-b border-white/10 px-4 py-3">
                    <div>
                        <div className="text-base font-semibold text-zinc-100">聪明钱雷达</div>
                        <div className="text-xs text-zinc-500">RPC 池子加池事件扫描 · 支持 V3/V4</div>
                    </div>
                    <button type="button" className={getIconButtonClass(false)} onClick={closeSheet} disabled={saving}>
                        <X size={16} />
                    </button>
                </div>
                <div className="max-h-[calc(92vh-60px)] overflow-y-auto p-4">
                    <div className="space-y-2">
                        <input
                            className={getInputClass(brand)}
                            placeholder="V3 池子地址 / V4 poolId"
                            value={poolInput}
                            onChange={(e) => setPoolInput(e.target.value)}
                        />
                        <div className="rounded-2xl border border-white/10 bg-zinc-900/55 p-2.5">
                            <div className="mb-2 flex items-center justify-between text-[10px] text-zinc-500">
                                <span className="font-semibold text-zinc-400">时间范围</span>
                                <span className="max-w-[220px] truncate text-right">{currentRangeLabel}</span>
                            </div>
                            <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
                                <label className="min-w-0">
                                    <span className="mb-1 block text-[10px] text-zinc-500">开始时间</span>
                                    <input
                                        className={getInputClass(brand)}
                                        type="datetime-local"
                                        step="1"
                                        value={timeRange.start}
                                        onChange={(e) => setTimeRange((prev) => ({ ...prev, start: e.target.value }))}
                                    />
                                </label>
                                <label className="min-w-0">
                                    <span className="mb-1 block text-[10px] text-zinc-500">结束时间</span>
                                    <input
                                        className={getInputClass(brand)}
                                        type="datetime-local"
                                        step="1"
                                        value={timeRange.end}
                                        onChange={(e) => setTimeRange((prev) => ({ ...prev, end: e.target.value }))}
                                    />
                                </label>
                            </div>
                        </div>
                        <div className="grid grid-cols-2 gap-2">
                            <label className="min-w-0">
                                <span className="mb-1 block text-[10px] text-zinc-500">最低金额(USD)</span>
                                <input className={getInputClass(brand)} type="number" min="1" value={minAmountUsd} onChange={(e) => setMinAmountUsd(e.target.value)} />
                            </label>
                            <label className="min-w-0">
                                <span className="mb-1 block text-[10px] text-zinc-500">数量上限</span>
                                <input className={getInputClass(brand)} type="number" min="1" max="100" value={limit} onChange={(e) => setLimit(e.target.value)} />
                            </label>
                        </div>
                        <button
                            type="button"
                            className={`${brand.solidButtonClass} ${brand.solidRingClass} w-full rounded-2xl px-3 py-2 text-sm font-semibold disabled:opacity-50`}
                            onClick={preview}
                            disabled={loading || saving}
                        >
                            {loading ? '扫描中...' : '扫描候选钱包'}
                        </button>
                        {loading ? (
                            <button
                                type="button"
                                className="w-full rounded-2xl border border-white/10 bg-white/[0.03] px-3 py-2 text-sm font-semibold text-zinc-200"
                                onClick={stopScan}
                                disabled={saving}
                            >
                                停止扫描
                            </button>
                        ) : null}
                    </div>

                    {(loading || saving || scanStep) ? (
                        <div className={`mt-3 rounded-2xl border px-3 py-2 ${error ? 'border-red-400/20 bg-red-500/10' : 'border-sky-400/20 bg-sky-400/10'}`}>
                            <div className="flex items-center justify-between gap-3 text-xs font-semibold text-zinc-100">
                                <span>{scanStep || (loading ? '准备扫描' : '等待操作')}</span>
                                <span className="shrink-0 text-[10px] text-zinc-500">{scanStatusText} · {formatRadarElapsed(scanElapsedMs)}</span>
                            </div>
                            <div className="mt-2 h-1.5 overflow-hidden rounded-full bg-white/10">
                                <div
                                    className={`relative h-full min-w-10 overflow-hidden rounded-full bg-gradient-to-r from-sky-400/20 via-lime-400 to-sky-400/20 transition-[width] duration-300 ${loading || saving ? 'after:absolute after:inset-0 after:rounded-full after:bg-gradient-to-r after:from-transparent after:via-white/35 after:to-transparent after:animate-[radarScan_1.15s_ease-in-out_infinite]' : ''}`}
                                    style={{ width: `${scanProgressPct}%` }}
                                />
                            </div>
                            <div className="mt-2 text-[10px] leading-relaxed text-zinc-500">
                                {loading ? '后端正在流式扫描链上加池事件，找到候选会立即插入列表。' : null}
                                {saving ? '正在写入监控钱包，请保持弹窗打开。' : null}
                                {!loading && !saving && data ? `候选 ${candidates.length} 个，已排除 ${Number(data?.excluded_count || 0)} 条事件。` : null}
                                {!loading && !saving && error ? '请求未完成，参数保留，可直接重试。' : null}
                            </div>
                            {scanTarget ? (
                                <div className="mt-2 grid grid-cols-2 gap-1.5">
                                    {[
                                        ['目标', scanTargetText],
                                        ['时间窗', `${scanTarget.rangeHours}h`],
                                        ['阈值', formatUSDCompact(scanTarget.minAmountUsd)],
                                        ['上限', scanTarget.limit],
                                    ].map(([label, value]) => (
                                        <div key={label} className="min-w-0 rounded-xl border border-white/10 bg-black/15 px-2 py-1.5">
                                            <div className="text-[9px] font-semibold text-zinc-500">{label}</div>
                                            <div className="mt-0.5 truncate text-[10px] font-semibold text-zinc-200">{value}</div>
                                        </div>
                                    ))}
                                </div>
                            ) : null}
                            {scanLogs.length > 0 ? (
                                <div className="mt-2 space-y-1.5">
                                    {scanLogs.map((item) => {
                                        const toneClass = item.tone === 'success'
                                            ? 'text-emerald-200'
                                            : item.tone === 'warn'
                                                ? 'text-amber-200'
                                                : item.tone === 'error'
                                                    ? 'text-red-200'
                                                    : 'text-zinc-400';
                                        return (
                                            <div key={item.id} className="grid grid-cols-[56px_minmax(0,1fr)] gap-2 text-[10px] leading-relaxed">
                                                <span className="font-mono text-zinc-600">{item.time}</span>
                                                <span className={`${toneClass} break-words`}>{item.text}</span>
                                            </div>
                                        );
                                    })}
                                </div>
                            ) : null}
                        </div>
                    ) : null}

                    {error ? <div className="mt-3 rounded-2xl border border-red-500/20 bg-red-500/10 px-3 py-2 text-sm text-red-200">{error}</div> : null}
                    {importResult ? (
                        <div className="mt-3 rounded-2xl border border-emerald-500/20 bg-emerald-500/10 px-3 py-2 text-xs text-emerald-200">
                            已新增 {importResult.created || 0}，已恢复 {importResult.reactivated || 0}，已跳过 {importResult.skipped_existing || 0}
                        </div>
                    ) : null}
                    {data ? (
                        <div className="mt-3 flex flex-wrap gap-1.5 text-[10px] text-zinc-500">
                            <span>已排除 {Number(data?.excluded_count || 0)} 条事件</span>
                            {Array.isArray(data?.warnings) && data.warnings.length > 0 ? (
                                <span title={data.warnings.join('\n')}>{data.warnings.length} 条提示</span>
                            ) : null}
                        </div>
                    ) : null}

                    {candidates.length > 0 ? (
                        <div className="mt-4 space-y-2">
                            <div className="flex items-center justify-between text-xs text-zinc-500">
                                <span>{candidates.length} 个候选钱包</span>
                                <button
                                    type="button"
                                    className="text-zinc-300"
                                    onClick={() => {
                                        if (allSelected) {
                                            setSelected({});
                                            return;
                                        }
                                        const next = {};
                                        candidates.forEach((item) => { next[item.wallet_address] = true; });
                                        setSelected(next);
                                    }}
                                >
                                    {allSelected ? '取消全选' : '全选'}
                                </button>
                            </div>
                            {candidates.map((item) => {
                                const checked = Boolean(selected[item.wallet_address]);
                                return (
                                    <label key={`${item.wallet_address}:${item.tx_hash}`} className="flex gap-3 rounded-2xl border border-white/10 bg-zinc-900/70 p-3">
                                        <input
                                            type="checkbox"
                                            className="mt-1 h-4 w-4"
                                            checked={checked}
                                            onChange={(e) => setSelected((prev) => ({ ...prev, [item.wallet_address]: e.target.checked }))}
                                        />
                                        <div className="min-w-0 flex-1">
                                            <div className="flex items-center justify-between gap-2">
                                                <span className="font-mono text-xs text-zinc-100">{shortAddr(item.wallet_address)}</span>
                                                <span className="text-xs font-semibold text-emerald-300">{formatUSDCompact(item.max_amount_usd)}</span>
                                            </div>
                                            <div className="mt-1 flex flex-wrap gap-1.5 text-[10px] text-zinc-500">
                                                <span>{item.pair || 'pool'}</span>
                                                <span>{item.protocol || 'protocol'}</span>
                                                <span>{item.amount_source || 'amount'}</span>
                                                <span>{shortAddr(item.pool_address)}</span>
                                                {item.already_monitored ? <span>已监控</span> : null}
                                            </div>
                                        </div>
                                    </label>
                                );
                            })}
                            <button
                                type="button"
                                className={`${brand.solidButtonClass} ${brand.solidRingClass} mt-2 w-full rounded-2xl px-3 py-2 text-sm font-semibold disabled:opacity-50`}
                                onClick={importSelected}
                                disabled={loading || saving || selectedWallets.length === 0}
                            >
                                {saving ? '导入中...' : `导入 ${selectedWallets.length} 个钱包`}
                            </button>
                        </div>
                    ) : data ? (
                        <div className="mt-4 rounded-2xl border border-white/10 bg-zinc-900/60 px-3 py-5 text-center text-sm text-zinc-500">没有找到符合条件的钱包</div>
                    ) : null}
                </div>
            </div>
        </div>
    );
}

// ============ PAGES ============

function buildPoolFromWatchActivity(event) {
    return {
        pool_address: event.pool_address,
        chain_id: event.chain_id,
        chain: resolvePoolChain(event),
        protocol: event.protocol,
        token0_address: event.token0_address,
        token1_address: event.token1_address,
        token0_symbol: event.token0_symbol,
        token1_symbol: event.token1_symbol,
        trading_pair: event.trading_pair,
        display_token_address: event.display_token_address,
        display_token_symbol: event.display_token_symbol,
        display_token_logo_url: event.display_token_logo_url,
        fee_tier: event.fee_tier,
    };
}

function WatchActivityCard({ event, brand, onSelectWallet, onSelectPool }) {
    const walletAddress = normalizeWalletAddress(event?.wallet_address) || String(event?.wallet_address || '').trim();
    const poolAddress = String(event?.pool_address || '').trim();
    const amountValue = Number(event?.total_usd);
    const amountText = Number.isFinite(amountValue) && amountValue > 0 ? formatUSDCompact(amountValue) : '--';
    const pairLabel = getPairLabel(event);
    const rangeText = event?.tick_lower !== null && event?.tick_lower !== undefined && event?.tick_upper !== null && event?.tick_upper !== undefined
        ? `${event.tick_lower} - ${event.tick_upper}`
        : '--';
    const nftText = event?.nft_token_id ? `NFT #${event.nft_token_id}` : '';
    const canOpenWallet = Boolean(walletAddress);
    const canOpenPool = Boolean(poolAddress);

    return (
        <div className="rounded-[24px] border border-white/[0.04] bg-zinc-900/60 p-3 shadow-[0_18px_50px_-32px_rgba(0,0,0,0.95)]">
            <div className="flex items-start gap-3">
                <PairAvatar item={event} size="sm" />
                <div className="min-w-0 flex-1">
                    <div className="flex flex-wrap items-center gap-1.5">
                        <Badge className={getWatchActivityActionClass(event?.event_type)}>
                            {formatWatchActivityAction(event?.event_type)}
                        </Badge>
                        <ProtocolBadge protocol={event?.protocol} />
                        <FeeBadge fee={event?.fee_tier} />
                    </div>
                    <div className="mt-1 truncate text-sm font-semibold text-zinc-100">{pairLabel}</div>
                    <div className="mt-2">
                        <WalletIdentity
                            address={walletAddress}
                            color={event?.wallet_color}
                            label={event?.wallet_label || walletAddress}
                            avatarUrl={event?.wallet_avatar_url}
                            source={event?.wallet_source}
                            sourceContract={event?.wallet_source_contract}
                            size={30}
                            showSource
                            onClick={canOpenWallet ? () => onSelectWallet?.(walletAddress) : undefined}
                        />
                    </div>
                </div>
                <div className="shrink-0 text-right">
                    <div className="text-sm font-semibold tabular-nums text-zinc-100">{amountText}</div>
                    <div className="mt-1 text-[10px] text-zinc-500">{relativeTime(event?.tx_timestamp)}</div>
                </div>
            </div>

            <div className="mt-3 flex flex-wrap items-center gap-1.5 text-[10px]">
                <span className="inline-flex min-w-[104px] items-center justify-between gap-2 rounded-full border border-white/[0.05] bg-black/20 px-2.5 py-1 text-zinc-300">
                    <strong className="font-semibold text-zinc-100">金额</strong>
                    <span className="tabular-nums">{amountText}</span>
                </span>
                <span className="inline-flex min-w-[132px] items-center justify-between gap-2 rounded-full border border-white/[0.05] bg-black/20 px-2.5 py-1 text-zinc-300">
                    <strong className="font-semibold text-zinc-100">Tick</strong>
                    <span className="max-w-[132px] truncate font-mono">{rangeText}</span>
                </span>
                {nftText ? (
                    <span className="inline-flex items-center rounded-full border border-white/[0.05] bg-black/20 px-2.5 py-1 text-zinc-300">
                        {nftText}
                    </span>
                ) : null}
            </div>

            <div className="mt-3 flex flex-wrap items-center justify-between gap-2 border-t border-white/[0.05] pt-3">
                <div className="flex min-w-0 flex-wrap items-center gap-1.5">
                    <CompactIdentifier value={poolAddress} label="池子" />
                    {event?.tx_hash ? <CompactIdentifier value={event.tx_hash} label="TX" /> : null}
                </div>
                <div className="ml-auto flex shrink-0 items-center gap-2">
                    {event?.explorer_url ? (
                        <a
                            href={event.explorer_url}
                            target="_blank"
                            rel="noreferrer"
                            className={`inline-flex items-center gap-1 text-[11px] ${getBrandLinkClass(brand)}`}
                        >
                            浏览器 <ExternalLink size={10} />
                        </a>
                    ) : null}
                    {canOpenPool ? (
                        <button
                            type="button"
                            className={`inline-flex items-center gap-1 text-[11px] ${getBrandActionChipClass(brand)}`}
                            onClick={() => onSelectPool?.(buildPoolFromWatchActivity(event))}
                        >
                            池子 <ExternalLink size={10} />
                        </button>
                    ) : null}
                </div>
            </div>
        </div>
    );
}

function WatchActivityPage({
    apiBaseUrl,
    initData,
    hasInitData,
    brand,
    watchedWallets = [],
    watchToggleMap = {},
    onToggleWatchWallet,
    onSelectWallet,
    onSelectPool,
    onOpenWallets,
    pollIntervalSec = 15,
}) {
    const canLoad = Boolean(String(initData || '').trim()) && hasInitData !== false;
    const [activities, setActivities] = useState([]);
    const [walletItems, setWalletItems] = useState(() => normalizeWatchWalletItems(null, watchedWallets));
    const [selectedWallet, setSelectedWallet] = useState('');
    const [total, setTotal] = useState(0);
    const [page, setPage] = useState(1);
    const [loading, setLoading] = useState(canLoad);
    const [error, setError] = useState('');
    const loadSeqRef = useRef(0);
    const activeWallet = normalizeWalletAddress(selectedWallet);
    const normalizedWatchedWallets = useMemo(
        () => Array.from(new Set((Array.isArray(watchedWallets) ? watchedWallets : [])
            .map((item) => normalizeWalletAddress(item))
            .filter(Boolean))).sort(),
        [watchedWallets],
    );

    useEffect(() => {
        loadSeqRef.current += 1;
        setWalletItems((prev) => {
            const allowed = new Set(normalizedWatchedWallets);
            if (allowed.size === 0) return [];
            return normalizeWatchWalletItems({
                items: prev.filter((item) => allowed.has(normalizeWalletAddress(item?.wallet_address))),
            }, normalizedWatchedWallets);
        });
    }, [normalizedWatchedWallets]);

    useEffect(() => {
        if (!activeWallet || normalizedWatchedWallets.includes(activeWallet)) return;
        setSelectedWallet('');
    }, [activeWallet, normalizedWatchedWallets]);

    const load = useCallback((silent = false) => {
        if (!canLoad) {
            setActivities([]);
            setTotal(0);
            setLoading(false);
            return;
        }

        const seq = ++loadSeqRef.current;
        if (!silent) {
            setLoading(true);
            setError('');
        }

        fetchSMWatchActivity({
            apiBaseUrl,
            initData,
            chain: 'bsc',
            wallet: activeWallet || undefined,
            page,
            size: WATCH_ACTIVITY_PAGE_SIZE,
        })
            .then((resp) => {
                if (seq !== loadSeqRef.current) return;
                if (!Array.isArray(resp?.list) || !Number.isFinite(Number(resp?.total))) {
                    throw new Error('特别关注动态格式错误');
                }
                const nextTotal = Number(resp.total);
                const totalPages = Math.max(1, Math.ceil(nextTotal / WATCH_ACTIVITY_PAGE_SIZE));
                setWalletItems(normalizeWatchWalletItems(resp, normalizedWatchedWallets));
                if (page > totalPages) {
                    setActivities([]);
                    setTotal(nextTotal);
                    setPage(totalPages);
                    return;
                }
                setActivities(resp.list);
                setTotal(nextTotal);
                setError('');
            })
            .catch((err) => {
                if (seq !== loadSeqRef.current) return;
                setActivities([]);
                setTotal(0);
                setError(String(err?.message || err || '加载特别关注动态失败'));
            })
            .finally(() => {
                if (!silent && seq === loadSeqRef.current) {
                    setLoading(false);
                }
            });
    }, [activeWallet, apiBaseUrl, canLoad, initData, normalizedWatchedWallets, page]);

    useEffect(() => {
        load();
    }, [load]);

    useEffect(() => {
        if (!canLoad) return undefined;
        const timer = setInterval(() => {
            load(true);
        }, Math.max(2, Number(pollIntervalSec)) * 1000);
        return () => clearInterval(timer);
    }, [canLoad, load, pollIntervalSec]);

    useEffect(() => {
        setPage(1);
    }, [activeWallet]);

    const filterWallets = walletItems;
    const emptyWallets = filterWallets.length === 0;

    if (!canLoad) {
        return (
            <div className="rounded-2xl border border-dashed border-white/[0.05] bg-zinc-900/45 px-4 py-8 text-center text-sm text-zinc-500">
                需要 Telegram initData 才能读取特别关注动态。
            </div>
        );
    }

    return (
        <div>
            <div className="mb-3 flex gap-2 overflow-x-auto pb-1 text-[11px]">
                <button
                    type="button"
                    className={`inline-flex shrink-0 items-center gap-1.5 rounded-2xl px-3 py-2 font-semibold ${getFilterButtonClass(!activeWallet, brand)}`}
                    onClick={() => setSelectedWallet('')}
                >
                    全部
                </button>
                {filterWallets.map((item) => {
                    const address = normalizeWalletAddress(item.wallet_address);
                    const active = activeWallet === address;
                    const busy = Boolean(watchToggleMap[address]);
                    return (
                        <div
                            key={address}
                            className={`inline-flex max-w-[220px] shrink-0 items-center gap-1 rounded-2xl pl-2.5 pr-1 py-1.5 font-semibold ${getFilterButtonClass(active, brand)}`}
                        >
                            <button
                                type="button"
                                className="inline-flex min-w-0 flex-1 items-center gap-2"
                                onClick={() => setSelectedWallet(address)}
                            >
                                <WalletAvatar address={address} color={item.wallet_color} avatarUrl={item.wallet_avatar_url} size={26} />
                                <span className="min-w-0 truncate">{item.wallet_label || shortAddr(address)}</span>
                            </button>
                            <button
                                type="button"
                                className="inline-flex h-6 w-6 shrink-0 items-center justify-center rounded-xl text-zinc-400 transition hover:bg-red-500/10 hover:text-red-200 disabled:opacity-50"
                                disabled={busy}
                                onClick={() => {
                                    onToggleWatchWallet?.(address, false);
                                    if (active) setSelectedWallet('');
                                }}
                                title="移除特别关注"
                                aria-label={`移除特别关注 ${shortAddr(address)}`}
                            >
                                {busy ? <span className="text-[10px]">...</span> : <X size={12} />}
                            </button>
                        </div>
                    );
                })}
            </div>

            {emptyWallets && !loading ? (
                <div className="rounded-2xl border border-dashed border-white/[0.05] bg-zinc-900/45 px-4 py-8 text-center text-sm text-zinc-500">
                    <div>还没有特别关注钱包。</div>
                    <button
                        type="button"
                        className={`mt-3 inline-flex items-center gap-1 rounded-full px-3 py-1.5 text-[11px] ${getFilterButtonClass(true, brand)}`}
                        onClick={onOpenWallets}
                    >
                        去钱包视图添加
                    </button>
                </div>
            ) : loading ? (
                <div className="py-8 text-center text-zinc-500">加载中...</div>
            ) : error ? (
                <div className="rounded-2xl border border-red-400/15 bg-red-500/5 px-4 py-8 text-center text-sm text-red-200">
                    {error}
                </div>
            ) : activities.length === 0 ? (
                <div className="rounded-2xl border border-dashed border-white/[0.05] bg-zinc-900/45 px-4 py-8 text-center text-sm text-zinc-500">
                    当前范围暂无加 LP / 撤 LP 记录
                </div>
            ) : (
                <div className="space-y-3">
                    <div className="px-1 text-[10px] text-zinc-500">
                        第 {page} 页 · 当前显示 {activities.length} 条 / 共 {total} 条记录
                    </div>
                    {activities.map((event) => (
                        <WatchActivityCard
                            key={`${event.tx_hash || event.id}:${event.log_index || 0}`}
                            event={event}
                            brand={brand}
                            onSelectWallet={onSelectWallet}
                            onSelectPool={onSelectPool}
                        />
                    ))}
                </div>
            )}

            <PositionPagination page={page} total={total} brand={brand} pageSize={WATCH_ACTIVITY_PAGE_SIZE} onChange={setPage} />
        </div>
    );
}

function PoolListPage({ apiBaseUrl, onSelectPool, onOpenPosition, brand, pollIntervalSec = 15 }) {
    const [pools, setPools] = useState([]);
    const [poolsTotal, setPoolsTotal] = useState(0);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState('');
    const [search, setSearch] = useState('');
    const [protocolFilter, setProtocolFilter] = useState('all');
    const [sourceScope, setSourceScope] = useState('all');
    const [page, setPage] = useState(1);
    const [filterOpen, setFilterOpen] = useState(false);
    const [poolFilter, setPoolFilter] = useState(readStoredSmartMoneyPoolFilter);
    const [poolFilterDraft, setPoolFilterDraft] = useState({ minSmartMoneyUsd: '', maxFeeRate: '', minMarketCapUsd: '' });
    const loadSeqRef = useRef(0);
    const loadInFlightRef = useRef(false);
    const searchKeyword = useMemo(() => String(search || '').trim(), [search]);
    const sourceFilter = SMART_MONEY_POOL_SOURCE_BY_KEY[sourceScope];

    const load = useCallback((silent = false) => {
        if (silent && loadInFlightRef.current) return Promise.resolve();
        const seq = ++loadSeqRef.current;
        loadInFlightRef.current = true;
        if (!silent) {
            setLoading(true);
            setError('');
        }
        return fetchSMPools({
            apiBaseUrl,
            page,
            size: POOL_LIST_PAGE_SIZE,
            keyword: searchKeyword || undefined,
            protocol: protocolFilter !== 'all' ? protocolFilter : undefined,
            source: sourceFilter,
            minSmartMoneyUsd: poolFilter.minSmartMoneyUsd,
            maxFeeRate: poolFilter.maxFeeRate,
            minMarketCapUsd: poolFilter.minMarketCapUsd,
        })
            .then((d) => {
                if (seq !== loadSeqRef.current) return;
                if (!Array.isArray(d?.list) || !Number.isFinite(Number(d?.total))) {
                    throw new Error('池子数据格式错误');
                }
                const total = Number(d.total);
                const list = d.list;
                const totalPages = Math.max(1, Math.ceil(total / POOL_LIST_PAGE_SIZE));
                if (page > totalPages) {
                    setPools([]);
                    setPoolsTotal(total);
                    setPage(totalPages);
                    return;
                }
                setPools(list);
                setPoolsTotal(total);
                setError('');
            })
            .catch((err) => {
                if (seq !== loadSeqRef.current) return;
                if (!silent) {
                    setPools([]);
                    setPoolsTotal(0);
                    setError(String(err?.message || err || '加载池子失败'));
                }
            })
            .finally(() => {
                if (seq === loadSeqRef.current) {
                    loadInFlightRef.current = false;
                    if (!silent) {
                        setLoading(false);
                    }
                }
            });
    }, [apiBaseUrl, page, poolFilter.maxFeeRate, poolFilter.minMarketCapUsd, poolFilter.minSmartMoneyUsd, protocolFilter, searchKeyword, sourceFilter]);

    useEffect(() => { load(); }, [load]);
    useEffect(() => {
        const timer = setInterval(() => {
            load(true);
        }, Math.max(2, Number(pollIntervalSec)) * 1000);
        return () => clearInterval(timer);
    }, [load, pollIntervalSec]);
    useEffect(() => {
        setPage(1);
    }, [poolFilter.maxFeeRate, poolFilter.minMarketCapUsd, poolFilter.minSmartMoneyUsd, protocolFilter, searchKeyword, sourceScope]);

    const poolFilterActive = useMemo(
        () => Number.isFinite(poolFilter.minSmartMoneyUsd)
            || Number.isFinite(poolFilter.maxFeeRate)
            || Number.isFinite(poolFilter.minMarketCapUsd),
        [poolFilter.maxFeeRate, poolFilter.minMarketCapUsd, poolFilter.minSmartMoneyUsd],
    );
    const poolFilterSummaryItems = useMemo(() => {
        const items = [];
        if (Number.isFinite(poolFilter.minMarketCapUsd)) items.push(`FDV ≥ ${formatUSDCompact(poolFilter.minMarketCapUsd)}`);
        if (Number.isFinite(poolFilter.maxFeeRate)) items.push(`费率 ≤ ${poolFilter.maxFeeRate}%`);
        if (Number.isFinite(poolFilter.minSmartMoneyUsd)) items.push(`聪明钱 ≥ ${formatUSDCompact(poolFilter.minSmartMoneyUsd)}`);
        return items;
    }, [poolFilter.maxFeeRate, poolFilter.minMarketCapUsd, poolFilter.minSmartMoneyUsd]);
    const openPoolFilter = useCallback(() => {
        setPoolFilterDraft({
            minSmartMoneyUsd: formatOptionalNumber(poolFilter.minSmartMoneyUsd),
            maxFeeRate: formatOptionalNumber(poolFilter.maxFeeRate),
            minMarketCapUsd: formatOptionalNumber(poolFilter.minMarketCapUsd),
        });
        setFilterOpen((prev) => !prev);
    }, [poolFilter.maxFeeRate, poolFilter.minMarketCapUsd, poolFilter.minSmartMoneyUsd]);
    const applyPoolFilter = useCallback(() => {
        const next = {
            minSmartMoneyUsd: parseOptionalNumber(poolFilterDraft.minSmartMoneyUsd),
            maxFeeRate: parseOptionalNumber(poolFilterDraft.maxFeeRate),
            minMarketCapUsd: parseOptionalNumber(poolFilterDraft.minMarketCapUsd),
        };
        setPoolFilter(next);
        writeStoredSmartMoneyPoolFilter(next);
        setFilterOpen(false);
        setPage(1);
    }, [poolFilterDraft.maxFeeRate, poolFilterDraft.minMarketCapUsd, poolFilterDraft.minSmartMoneyUsd]);
    const clearPoolFilter = useCallback(() => {
        const next = { ...EMPTY_SMART_MONEY_POOL_FILTER };
        setPoolFilter(next);
        writeStoredSmartMoneyPoolFilter(next);
        setPoolFilterDraft({ minSmartMoneyUsd: '', maxFeeRate: '', minMarketCapUsd: '' });
        setFilterOpen(false);
        setPage(1);
    }, []);
    const hasFilter = Boolean(searchKeyword) || protocolFilter !== 'all' || sourceScope !== 'all' || poolFilterActive;

    return (
        <div>
            <div className="mb-3 grid grid-cols-3 gap-1 rounded-[20px] border border-white/[0.05] bg-zinc-950/50 p-1 text-[11px]">
                {SMART_MONEY_POOL_SOURCE_TABS.map((item) => (
                    <button
                        key={item.key}
                        type="button"
                        className={`inline-flex min-h-[34px] items-center justify-center rounded-[16px] px-2 font-semibold transition ${getFilterButtonClass(sourceScope === item.key, brand)}`}
                        onClick={() => {
                            setSourceScope(item.key);
                            setPage(1);
                        }}
                        aria-pressed={sourceScope === item.key}
                    >
                        {item.label}
                    </button>
                ))}
            </div>
            <div className="mb-3 flex gap-2">
                <div className="relative flex-1">
                    <Search size={14} className="absolute left-3 top-1/2 -translate-y-1/2 text-zinc-500" />
                    <input
                        className={getInputClass(brand).replace('px-3', 'pl-9 pr-3')}
                        placeholder="搜索池子..."
                        value={search}
                        onChange={(e) => {
                            setSearch(e.target.value);
                            setPage(1);
                        }}
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
                            onClick={() => {
                                setProtocolFilter(p);
                                setPage(1);
                            }}
                        >
                            {info && <img src={info.icon} alt="" className="h-3.5 w-3.5 rounded-full" />}
                            {p === 'all' ? '全部' : info?.version || p}
                        </button>
                    );
                })}
                <button
                    type="button"
                    className={`inline-flex shrink-0 items-center gap-1 rounded-full px-3 py-1.5 ${getFilterButtonClass(poolFilterActive, brand)}`}
                    onClick={openPoolFilter}
                >
                    <SlidersHorizontal size={13} />
                    筛选
                </button>
            </div>
            {poolFilterSummaryItems.length > 0 ? (
                <div className="mb-3 flex flex-wrap items-center gap-1.5 text-[10px]">
                    <span className={`inline-flex items-center rounded-full px-2 py-1 font-semibold ${getFilterButtonClass(true, brand)}`}>
                        已筛选
                    </span>
                    {poolFilterSummaryItems.map((item) => (
                        <span
                            key={item}
                            className="inline-flex max-w-full items-center rounded-full border border-white/[0.06] bg-zinc-900/60 px-2 py-1 text-zinc-400"
                        >
                            <span className="truncate">{item}</span>
                        </span>
                    ))}
                </div>
            ) : (
                <div className="mb-3 px-1 text-[10px] text-zinc-500">
                    筛选可排除低 FDV 和高费率池子
                </div>
            )}

            {filterOpen ? (
                <div className="mb-4 rounded-2xl border border-white/[0.06] bg-zinc-900/80 p-3 shadow-[0_18px_50px_-32px_rgba(0,0,0,0.95)]">
                    <div className="mb-3 flex items-start justify-between gap-3">
                        <div>
                            <div className="text-sm font-semibold text-zinc-100">池子筛选</div>
                            <div className="mt-0.5 text-[11px] text-zinc-500">按聪明钱仓位、排除高费率和排除低 FDV 过滤池子</div>
                        </div>
                        <button
                            type="button"
                            className="rounded-full p-1 text-zinc-500 transition hover:bg-white/5 hover:text-zinc-200"
                            onClick={() => setFilterOpen(false)}
                            aria-label="关闭筛选"
                        >
                            <X size={16} />
                        </button>
                    </div>
                    <div className="grid grid-cols-2 gap-2">
                        <label className="text-[11px] text-zinc-500">
                            <span className="mb-1 block">聪明钱仓位 ≥ (USD)</span>
                            <input
                                className={getInputClass(brand)}
                                value={poolFilterDraft.minSmartMoneyUsd}
                                onChange={(e) => setPoolFilterDraft((prev) => ({ ...prev, minSmartMoneyUsd: e.target.value }))}
                                inputMode="decimal"
                                placeholder="可选"
                            />
                        </label>
                        <label className="text-[11px] text-zinc-500">
                            <span className="mb-1 block">排除高费率：费率 ≤ (%)</span>
                            <input
                                className={getInputClass(brand)}
                                value={poolFilterDraft.maxFeeRate}
                                onChange={(e) => setPoolFilterDraft((prev) => ({ ...prev, maxFeeRate: e.target.value }))}
                                inputMode="decimal"
                                placeholder="可选"
                            />
                        </label>
                        <label className="text-[11px] text-zinc-500">
                            <span className="mb-1 block">排除低 FDV：FDV ≥ (USD)</span>
                            <input
                                className={getInputClass(brand)}
                                value={poolFilterDraft.minMarketCapUsd}
                                onChange={(e) => setPoolFilterDraft((prev) => ({ ...prev, minMarketCapUsd: e.target.value }))}
                                inputMode="decimal"
                                placeholder="可选"
                            />
                        </label>
                    </div>
                    <div className="mt-3 flex gap-2">
                        <button type="button" className={`rounded-full px-3 py-1.5 text-[11px] ${getFilterButtonClass(true, brand)}`} onClick={applyPoolFilter}>
                            应用
                        </button>
                        <button type="button" className={`rounded-full px-3 py-1.5 text-[11px] ${getFilterButtonClass(false, brand)}`} onClick={clearPoolFilter}>
                            清空
                        </button>
                    </div>
                </div>
            ) : null}

            {loading ? (
                <div className="py-8 text-center text-zinc-500">加载中...</div>
            ) : error ? (
                <div className="rounded-2xl border border-red-400/15 bg-red-500/5 px-4 py-8 text-center text-sm text-red-200">
                    {error}
                </div>
            ) : pools.length === 0 ? (
                <div className="rounded-2xl border border-dashed border-white/[0.05] bg-zinc-900/45 px-4 py-8 text-center text-sm text-zinc-500">
                    {hasFilter ? '暂无符合条件的池子' : '暂无活跃仓位的池子'}
                </div>
            ) : (
                <div className="space-y-3">
                    <div className="px-1 text-[10px] text-zinc-500">
                        第 {page} 页 · 当前显示 {pools.length} 个 / 共 {poolsTotal} 个池子
                    </div>
                    {pools.map(pool => {
                        const marketCap = resolveSmartMoneyPoolMarketCapDisplay(pool);
                        const marketCapLabel = resolveSmartMoneyPoolMarketCapLabel();
                        const marketCapAvailable = Number.isFinite(marketCap) && marketCap > 0;
                        return (
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
                                            <Badge className={marketCapAvailable
                                                ? 'border-cyan-400/20 bg-cyan-400/10 text-cyan-200'
                                                : 'border-white/10 bg-zinc-800/70 text-zinc-500'}
                                            >
                                                {marketCapLabel} {marketCapAvailable ? formatUSDCompact(marketCap) : '--'}
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
                                            className={`mini-follow-open-btn mt-1 inline-flex h-6 shrink-0 items-center gap-1 rounded-full px-2 text-[10px] font-semibold leading-none shadow-sm ${brand.solidButtonClass} ${brand.solidRingClass}`}
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
                        );
                    })}
                </div>
            )}
            <PositionPagination page={page} total={poolsTotal} brand={brand} pageSize={POOL_LIST_PAGE_SIZE} onChange={setPage} />
        </div>
    );
}

function SmartMoneyPoolViewPage({ apiBaseUrl, onSelectPool, onOpenPosition, brand, pollIntervalSec = 15 }) {
    const [tab, setTab] = useState('active');
    return (
        <div>
            <div className="mb-4 grid grid-cols-2 gap-2 rounded-[22px] border border-white/[0.05] bg-zinc-950/50 p-1">
                {[
                    { key: 'active', label: '活跃池子', icon: Activity },
                    { key: 'heatmap', label: '收益火焰图', icon: Flame },
                ].map(({ key, label, icon: Icon }) => (
                    <button
                        key={key}
                        type="button"
                        className={`inline-flex min-h-[42px] items-center justify-center gap-1.5 rounded-[18px] px-3 text-xs font-semibold transition ${getFilterButtonClass(tab === key, brand)}`}
                        onClick={() => setTab(key)}
                    >
                        <Icon size={14} />
                        {label}
                    </button>
                ))}
            </div>
            {tab === 'heatmap' ? (
                <PoolFeeHeatmapPage
                    apiBaseUrl={apiBaseUrl}
                    onSelectPool={onSelectPool}
                    onOpenPosition={onOpenPosition}
                    brand={brand}
                    pollIntervalSec={pollIntervalSec}
                />
            ) : (
                <PoolListPage
                    apiBaseUrl={apiBaseUrl}
                    onSelectPool={onSelectPool}
                    onOpenPosition={onOpenPosition}
                    brand={brand}
                    pollIntervalSec={pollIntervalSec}
                />
            )}
        </div>
    );
}

function PoolFeeHeatmapPage({ apiBaseUrl, onSelectPool, onOpenPosition, brand, pollIntervalSec = 15 }) {
    const [rows, setRows] = useState([]);
    const [total, setTotal] = useState(0);
    const [page, setPage] = useState(1);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState('');
    const [sort, setSort] = useState('rate');
    const [windowKey, setWindowKey] = useState('1m');
    const [protocolFilter, setProtocolFilter] = useState('all');
    const [search, setSearch] = useState('');
    const loadSeqRef = useRef(0);
    const loadInFlightRef = useRef(false);
    const searchKeyword = useMemo(() => String(search || '').trim(), [search]);

    const load = useCallback((silent = false) => {
        if (silent && loadInFlightRef.current) return Promise.resolve();
        const seq = ++loadSeqRef.current;
        loadInFlightRef.current = true;
        if (!silent) {
            setLoading(true);
            setError('');
        }
        return fetchSMPoolFeeHeatmap({
            apiBaseUrl,
            window: windowKey,
            sort,
            keyword: searchKeyword || undefined,
            protocol: protocolFilter !== 'all' ? protocolFilter : undefined,
            page,
            size: POOL_HEATMAP_PAGE_SIZE,
        })
            .then((data) => {
                if (seq !== loadSeqRef.current) return;
                if (!Array.isArray(data?.list)) throw new Error('收益火焰图数据格式错误');
                const nextTotal = Number(data.total || 0);
                const totalPages = Math.max(1, Math.ceil(nextTotal / POOL_HEATMAP_PAGE_SIZE));
                if (page > totalPages) {
                    setRows([]);
                    setTotal(nextTotal);
                    setPage(totalPages);
                    return;
                }
                setRows(data.list);
                setTotal(nextTotal);
                setError('');
            })
            .catch((err) => {
                if (seq !== loadSeqRef.current) return;
                if (!silent) {
                    setRows([]);
                    setTotal(0);
                    setError(String(err?.message || err || '加载收益火焰图失败'));
                }
            })
            .finally(() => {
                if (seq === loadSeqRef.current) {
                    loadInFlightRef.current = false;
                    if (!silent) setLoading(false);
                }
            });
    }, [apiBaseUrl, page, protocolFilter, searchKeyword, sort, windowKey]);

    useEffect(() => {
        setPage(1);
    }, [protocolFilter, searchKeyword, sort, windowKey]);

    useEffect(() => { load(); }, [load]);
    useEffect(() => {
        const timer = setInterval(() => load(true), Math.max(2, Number(pollIntervalSec)) * 1000);
        return () => clearInterval(timer);
    }, [load, pollIntervalSec]);

    const maxIntensity = useMemo(() => {
        return Math.max(...rows.map((row) => {
            if (sort === 'fee') {
                return Number(row?.fee_position_count || 0) > 0 ? Number(row?.fee_usd || 0) : 0;
            }
            return Number(row?.rate_position_count || 0) > 0 ? Number(row?.fee_rate_per_1k_usd_window || 0) : 0;
        }), 0);
    }, [rows, sort]);

    return (
        <div>
            <div className="mb-3 flex gap-2">
                <div className="relative flex-1">
                    <Search size={14} className="absolute left-3 top-1/2 -translate-y-1/2 text-zinc-500" />
                    <input
                        className={getInputClass(brand).replace('px-3', 'pl-9 pr-3')}
                        placeholder="搜索池子..."
                        value={search}
                        onChange={(e) => setSearch(e.target.value)}
                    />
                </div>
            </div>

            <div className="mb-3 flex gap-1.5 overflow-x-auto pb-1 text-[11px]">
                {POOL_HEATMAP_SORTS.map((item) => (
                    <button
                        key={item.key}
                        type="button"
                        className={`inline-flex shrink-0 items-center gap-1 rounded-full px-3 py-1.5 ${getFilterButtonClass(sort === item.key, brand)}`}
                        onClick={() => setSort(item.key)}
                    >
                        {item.key === 'rate' ? <Activity size={13} /> : <DollarSign size={13} />}
                        {item.label}
                    </button>
                ))}
                {POOL_HEATMAP_WINDOWS.map((item) => (
                    <button
                        key={item.key}
                        type="button"
                        className={`inline-flex shrink-0 items-center gap-1 rounded-full px-3 py-1.5 ${getFilterButtonClass(windowKey === item.key, brand)}`}
                        onClick={() => setWindowKey(item.key)}
                    >
                        <Clock size={12} />
                        {item.label}
                    </button>
                ))}
            </div>

            <div className="mb-4 flex gap-1.5 overflow-x-auto pb-1 text-[11px]">
                {['all', 'pancake_v3', 'uniswap_v3', 'uniswap_v4'].map((p) => {
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
            ) : error ? (
                <div className="rounded-2xl border border-red-400/15 bg-red-500/5 px-4 py-8 text-center text-sm text-red-200">{error}</div>
            ) : rows.length === 0 ? (
                <div className="rounded-2xl border border-dashed border-white/[0.05] bg-zinc-900/45 px-4 py-8 text-center text-sm text-zinc-500">
                    暂无可计算收益的池子
                </div>
            ) : (
                <div className="space-y-3">
                    <div className="px-1 text-[10px] text-zinc-500">
                        第 {page} 页 · 当前显示 {rows.length} 个 / 共 {total} 个池子 · {sort === 'rate' ? `按 ${heatmapWindowLabel(windowKey)} 速率` : '按手续费总额'}
                    </div>
                    {rows.map((row, index) => (
                        <PoolFeeHeatmapCard
                            key={row.pool_address}
                            row={row}
                            rank={(page - 1) * POOL_HEATMAP_PAGE_SIZE + index + 1}
                            sort={sort}
                            windowKey={windowKey}
                            maxIntensity={maxIntensity}
                            brand={brand}
                            onSelectPool={onSelectPool}
                            onOpenPosition={onOpenPosition}
                        />
                    ))}
                    <PositionPagination page={page} total={total} brand={brand} pageSize={POOL_HEATMAP_PAGE_SIZE} onChange={setPage} />
                </div>
            )}
        </div>
    );
}

function PoolFeeHeatmapCard({ row, rank, sort, windowKey, maxIntensity, brand, onSelectPool, onOpenPosition }) {
    const hasFeeSample = Number(row?.fee_position_count || 0) > 0;
    const hasRateSample = Number(row?.rate_position_count || 0) > 0;
    const metricValue = sort === 'fee' && hasFeeSample
        ? Number(row?.fee_usd || 0)
        : sort !== 'fee' && hasRateSample
            ? Number(row?.fee_rate_per_1k_usd_window || 0)
            : 0;
    const intensity = maxIntensity > 0 ? Math.max(0.08, Math.min(1, metricValue / maxIntensity)) : 0.08;
    const reliable = String(row?.sample_status || '') === 'ok';
    const partial = String(row?.sample_status || '') === 'partial';
    const marketCap = resolveSmartMoneyPoolMarketCapDisplay(row);
    const marketCapLabel = resolveSmartMoneyPoolMarketCapLabel();
    const marketCapAvailable = Number.isFinite(marketCap) && marketCap > 0;
    return (
        <button
            type="button"
            className="relative w-full overflow-hidden rounded-[24px] border border-white/[0.05] bg-zinc-900/65 p-3 text-left shadow-[0_18px_50px_-32px_rgba(0,0,0,0.95)] transition active:scale-[0.995]"
            onClick={() => onSelectPool(row)}
        >
            <div
                className="pointer-events-none absolute inset-x-0 top-0 h-1 bg-gradient-to-r from-amber-500 via-orange-400 to-emerald-300"
                style={{ opacity: 0.25 + intensity * 0.65 }}
            />
            <div
                className="pointer-events-none absolute -right-10 -top-16 h-32 w-32 rounded-full bg-orange-400/20 blur-3xl"
                style={{ opacity: intensity * 0.8 }}
            />
            <div className="relative flex items-start gap-3">
                <div className="flex shrink-0 flex-col items-center gap-2">
                    <span className="inline-flex h-7 min-w-7 items-center justify-center rounded-full border border-amber-300/20 bg-amber-300/10 px-2 text-[11px] font-bold text-amber-200">
                        #{rank}
                    </span>
                    <PairAvatar item={row} size="md" />
                </div>
                <div className="min-w-0 flex-1">
                    <div className="flex flex-wrap items-center gap-1.5">
                        <span className="truncate text-sm font-semibold text-zinc-100">{getPairLabel(row)}</span>
                        <ProtocolBadge protocol={row.protocol} />
                        <FeeBadge fee={row.fee_tier} />
                    </div>
                    <div className="mt-2 grid grid-cols-2 gap-2">
                        <div className="rounded-2xl border border-white/[0.05] bg-black/20 px-3 py-2">
                            <div className="text-[10px] text-zinc-500">总手续费</div>
                            <div className="mt-1 text-base font-semibold text-amber-100">{hasFeeSample ? formatHeatmapUSD(row.fee_usd) : '--'}</div>
                        </div>
                        <div className="rounded-2xl border border-white/[0.05] bg-black/20 px-3 py-2">
                            <div className="text-[10px] text-zinc-500">每1000U/{heatmapWindowLabel(windowKey)}</div>
                            <div className="mt-1 text-base font-semibold text-emerald-200">{hasRateSample ? formatHeatmapRate(row.fee_rate_per_1k_usd_window) : '--'}</div>
                        </div>
                    </div>
                    <div className="mt-2 h-2 overflow-hidden rounded-full bg-black/30">
                        <div
                            className="h-full rounded-full bg-gradient-to-r from-amber-400 via-orange-400 to-emerald-300"
                            style={{ width: `${Math.max(6, intensity * 100)}%` }}
                        />
                    </div>
                    <div className="mt-3 flex flex-wrap items-center gap-1.5 text-[10px] text-zinc-500">
                        <span>{row.wallet_count} 钱包</span>
                        <span>·</span>
                        <span>{row.open_position_count} 仓位</span>
                        <span>·</span>
                        <span>仓位 {formatHeatmapUSD(row.total_position_amount_usd)}</span>
                        <span>·</span>
                        <span className={marketCapAvailable ? 'text-zinc-400' : 'text-zinc-600'}>
                            {marketCapLabel} {marketCapAvailable ? formatUSDCompact(marketCap) : '--'}
                        </span>
                        <span>·</span>
                        <span>均龄 {formatHeatmapAge(row.average_position_age_seconds)}</span>
                    </div>
                    <div className="mt-2 flex flex-wrap items-center justify-between gap-2">
                        <span className={`rounded-full border px-2 py-1 text-[10px] ${reliable
                            ? 'border-emerald-400/20 bg-emerald-400/10 text-emerald-200'
                            : partial
                                ? 'border-amber-400/20 bg-amber-400/10 text-amber-200'
                                : 'border-zinc-500/20 bg-zinc-500/10 text-zinc-400'
                            }`}
                        >
                            {heatmapSampleText(row)} · {row.rate_position_count || 0}/{row.open_position_count || 0} 仓
                        </span>
                        {typeof onOpenPosition === 'function' ? (
                            <button
                                type="button"
                                className={`mini-follow-open-btn inline-flex h-7 shrink-0 items-center gap-1 rounded-full px-2.5 text-[10px] font-semibold leading-none shadow-sm ${brand.solidButtonClass} ${brand.solidRingClass}`}
                                onClick={(event) => {
                                    event.stopPropagation();
                                    onOpenPosition(row);
                                }}
                            >
                                <FlashIcon className="h-2.5 w-2.5 shrink-0" />
                                跟单
                            </button>
                        ) : null}
                    </div>
                </div>
            </div>
        </button>
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
    const poolStatsMarketCap = resolveSmartMoneyPoolMarketCapDisplay(poolStats || pool);
    const poolStatsMarketCapLabel = resolveSmartMoneyPoolMarketCapLabel();
    const poolStatsMarketCapAvailable = Number.isFinite(poolStatsMarketCap) && poolStatsMarketCap > 0;

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
                    <StatCard
                        label={poolStatsMarketCapLabel}
                        value={poolStatsMarketCapAvailable ? formatUSDCompact(poolStatsMarketCap) : '--'}
                        compact
                        valueClassName="text-[13px]"
                    />
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
                                                source={pos.wallet_source}
                                                sourceContract={pos.wallet_source_contract}
                                                showSource
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
                                    <PositionPreviewMetricsLite
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

function WalletListPage({ apiBaseUrl, onSelectWallet, onAddWallet, brand, refreshKey, watchedWalletSet = new Set(), watchToggleMap = {}, onToggleWatchWallet, pollIntervalSec = 15 }) {
    const [wallets, setWallets] = useState([]);
    const [walletsTotal, setWalletsTotal] = useState(0);
    const [loading, setLoading] = useState(true);
    const [search, setSearch] = useState('');
    const [page, setPage] = useState(1);
    const [busyKey, setBusyKey] = useState('');
    const [actionError, setActionError] = useState('');
    const [confirmState, setConfirmState] = useState(null);
    const [editingWallet, setEditingWallet] = useState(null);
    const [zombieOpen, setZombieOpen] = useState(false);
    const [tokenLiquidityOpen, setTokenLiquidityOpen] = useState(false);
    const [zombieCandidates, setZombieCandidates] = useState([]);
    const [zombieSelected, setZombieSelected] = useState({});
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
        }, Math.max(2, Number(pollIntervalSec)) * 1000);
        return () => clearInterval(timer);
    }, [load, pollIntervalSec]);

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

    const findZombieWallets = async () => {
        setBusyKey('wallet-zombies:find');
        setActionError('');
        try {
            const data = await fetchSMZombieWallets({ apiBaseUrl, days: 30 });
            const list = Array.isArray(data?.list) ? data.list : [];
            const selected = {};
            list.forEach((item) => {
                selected[`${item.address}:${item.chain_id}`] = true;
            });
            setZombieCandidates(list);
            setZombieSelected(selected);
            setZombieOpen(true);
        } catch (err) {
            setActionError(err?.message || '查找僵尸钱包失败');
        } finally {
            setBusyKey('');
        }
    };

    const toggleZombieWallet = (key) => {
        setZombieSelected((prev) => ({ ...prev, [key]: !prev[key] }));
    };

    const toggleAllZombieWallets = (checked) => {
        const next = {};
        zombieCandidates.forEach((item) => {
            next[`${item.address}:${item.chain_id}`] = checked;
        });
        setZombieSelected(next);
    };

    const deleteSelectedZombieWallets = async () => {
        const walletsToDelete = zombieCandidates
            .filter((item) => zombieSelected[`${item.address}:${item.chain_id}`])
            .map((item) => ({ address: item.address, chain_id: item.chain_id }));
        if (walletsToDelete.length === 0) return;
        setBusyKey('wallet-zombies:delete');
        setActionError('');
        try {
            await deleteSMZombieWallets({ apiBaseUrl, wallets: walletsToDelete });
            setZombieOpen(false);
            setZombieCandidates([]);
            setZombieSelected({});
            await load();
        } catch (err) {
            setActionError(err?.message || '删除僵尸钱包失败');
        } finally {
            setBusyKey('');
        }
    };

    return (
        <div>
            <div className="mb-3 flex flex-wrap gap-2">
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
                    type="button"
                    onClick={() => setTokenLiquidityOpen(true)}
                    className="inline-flex shrink-0 items-center gap-1 rounded-2xl border border-sky-300/20 bg-sky-300/10 px-3 py-2 text-sm font-semibold text-sky-100 transition hover:bg-sky-300/15"
                >
                    <Radar size={14} /> 聪明钱雷达
                </button>
                <button
                    type="button"
                    onClick={findZombieWallets}
                    disabled={busyKey === 'wallet-zombies:find' || busyKey === 'wallet-zombies:delete'}
                    className="inline-flex shrink-0 items-center gap-1 rounded-2xl border border-amber-300/20 bg-amber-300/10 px-3 py-2 text-sm font-semibold text-amber-100 transition hover:bg-amber-300/15 disabled:opacity-50"
                >
                    <Activity size={14} /> 僵尸
                </button>
                <button
                    type="button"
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
                                        source={w.source}
                                        sourceContract={w.source_contract}
                                        size={44}
                                        showCopy
                                    />
                                    <div className="mt-2 flex flex-wrap items-center gap-1.5">
                                        <Badge className={walletSourceBadgeClass(w.source)}>
                                            {walletSourceLabel(w.source)}
                                        </Badge>
                                        {walletSourceContractLabel(w.source_contract) ? (
                                            <Badge className="border-white/10 bg-zinc-800/80 text-zinc-400">
                                                {walletSourceContractLabel(w.source_contract)}
                                            </Badge>
                                        ) : null}
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
                                        className="inline-flex min-w-[56px] items-center justify-center rounded-xl border border-sky-400/20 bg-sky-400/10 px-2.5 text-[10px] font-semibold text-sky-100 transition disabled:opacity-50"
                                        disabled={Boolean(watchToggleMap[normalizeWalletAddress(w.address)]) || busyKey === `wallet-toggle:${w.address}` || busyKey === `wallet-delete:${w.address}`}
                                        onClick={e => {
                                            e.stopPropagation();
                                            onToggleWatchWallet?.(w.address);
                                        }}
                                    >
                                        {watchToggleMap[normalizeWalletAddress(w.address)] ? '处理中' : (watchedWalletSet.has(normalizeWalletAddress(w.address)) ? '已关注' : '特别关注')}
                                    </button>
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
                                                description: `确认删除钱包 ${shortAddr(w.address)} 吗？该钱包的聪明钱历史数据也会删除。`,
                                                action: () => deleteSMWallet({ apiBaseUrl, address: w.address }),
                                            });
                                        }}
                                    >
                                        <Trash2 size={14} />
                                    </button>
                                </div>
                            </div>

                            <div className="mt-3 grid grid-cols-3 gap-2">
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
            <ZombieWalletSheet
                open={zombieOpen}
                candidates={zombieCandidates}
                selectedMap={zombieSelected}
                busy={busyKey === 'wallet-zombies:delete'}
                onToggle={toggleZombieWallet}
                onToggleAll={toggleAllZombieWallets}
                onClose={() => {
                    if (busyKey !== 'wallet-zombies:delete') setZombieOpen(false);
                }}
                onDelete={deleteSelectedZombieWallets}
            />
            <TokenLiquidityImportSheet
                open={tokenLiquidityOpen}
                apiBaseUrl={apiBaseUrl}
                brand={brand}
                onClose={() => {
                    if (!busyKey) setTokenLiquidityOpen(false);
                }}
                onImported={async () => {
                    await load();
                }}
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

function WalletDetailPage({ apiBaseUrl, walletAddress, onBack, onSelectPool, brand, watchedWalletSet = new Set(), watchToggleMap = {}, onToggleWatchWallet }) {
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
                                <Badge className={walletSourceBadgeClass(walletInfo.source)}>
                                    {walletSourceLabel(walletInfo.source)}
                                </Badge>
                                {walletSourceContractLabel(walletInfo.source_contract) ? (
                                    <Badge className="border-white/10 bg-zinc-800/80 text-zinc-400">
                                        {walletSourceContractLabel(walletInfo.source_contract)}
                                    </Badge>
                                ) : null}
                                <Badge className={walletInfo.is_active
                                    ? 'border-emerald-500/20 bg-emerald-500/10 text-emerald-300'
                                    : 'border-white/10 bg-zinc-800/80 text-zinc-400'}>
                                    {walletInfo.is_active ? '监控中' : '已暂停'}
                                </Badge>
                            </div>
                            <div className="mt-3">
                                <button
                                    type="button"
                                    className="inline-flex items-center gap-2 rounded-xl border border-sky-400/20 bg-sky-400/10 px-3 py-2 text-[11px] font-semibold text-sky-100 transition disabled:opacity-50"
                                    disabled={Boolean(watchToggleMap[normalizeWalletAddress(walletAddress)])}
                                    onClick={() => onToggleWatchWallet?.(walletAddress)}
                                >
                                    {watchToggleMap[normalizeWalletAddress(walletAddress)]
                                        ? '处理中...'
                                        : (watchedWalletSet.has(normalizeWalletAddress(walletAddress)) ? '已特别关注' : '加入特别关注')}
                                </button>
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
                        className={`inline-flex items-center gap-1 text-[11px] ${getBrandActionChipClass(brand)}`}
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
                                    <PositionPreviewMetricsLite
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

function ContractSettingsPage({ apiBaseUrl, brand, pollIntervalSec = 15 }) {
    return <ContractSettingsTab apiBaseUrl={apiBaseUrl} brand={brand} pollIntervalSec={pollIntervalSec} />;
}

function GoldenDogPage({ apiBaseUrl, initData, brand, watchedWallets = [], watchedWalletSet = new Set(), watchToggleMap = {}, onToggleWatchWallet }) {
    return (
        <GoldenDogPageContent
            apiBaseUrl={apiBaseUrl}
            initData={initData}
            brand={brand}
            watchedWallets={watchedWallets}
            watchedWalletSet={watchedWalletSet}
            watchToggleMap={watchToggleMap}
            onToggleWatchWallet={onToggleWatchWallet}
        />
    );

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

const AUTO_FOLLOW_DEFAULT_DRAFT = {
    id: 0,
    target_wallet_address: '',
    target_wallet_addresses: [''],
    execution_wallet_id: '',
    execution_wallet_address: '',
    trigger_mode: 'any',
    trigger_min_wallets: '2',
    trigger_window_seconds: '300',
    enabled: true,
    amount_mode: 'fixed',
    fixed_amount_usdt: '50',
    ratio_percent: '100',
    delay_mode: 'immediate',
    delay_seconds: '0',
    follow_close: true,
};

function normalizeAutoFollowWalletList(config) {
    const source = Array.isArray(config?.target_wallet_addresses) && config.target_wallet_addresses.length > 0
        ? config.target_wallet_addresses
        : [config?.target_wallet_address || ''];
    const wallets = source.map((value) => String(value || '').trim()).filter(Boolean);
    return wallets.length ? wallets : [''];
}

function parseAutoFollowWalletInputs(values) {
    const seen = new Set();
    const wallets = [];
    (Array.isArray(values) ? values : []).forEach((value) => {
        const address = normalizeWalletAddress(value);
        if (!address) {
            if (String(value || '').trim()) throw new Error('请输入有效的钱包地址');
            return;
        }
        if (!seen.has(address)) {
            seen.add(address);
            wallets.push(address);
        }
    });
    if (!wallets.length) throw new Error('至少填写一个跟单钱包地址');
    return wallets;
}

function autoFollowTriggerText(config) {
    const wallets = normalizeAutoFollowWalletList(config).filter(Boolean);
    if (String(config?.trigger_mode || 'any') === 'threshold') {
        return `${Number(config?.trigger_min_wallets || 2)} / ${wallets.length} 钱包 · ${Number(config?.trigger_window_seconds || 300)}s`;
    }
    return wallets.length > 1 ? `任意 1 / ${wallets.length} 钱包` : '单钱包触发';
}

function findAutoFollowExecutionWallet(wallets, id, address) {
    const walletId = Number(id) || 0;
    const addr = normalizeWalletAddress(address);
    if (!Array.isArray(wallets)) return null;
    return wallets.find((wallet) => {
        if (walletId > 0 && Number(wallet?.id) === walletId) return true;
        return addr && normalizeWalletAddress(wallet?.address) === addr;
    }) || null;
}

function formatAutoFollowExecutionWallet(value, wallets) {
    const wallet = findAutoFollowExecutionWallet(wallets, value?.execution_wallet_id, value?.execution_wallet_address);
    if (wallet) {
        const name = String(wallet.name || '').trim();
        const addr = normalizeWalletAddress(wallet.address);
        return name ? `${name} · ${shortAddr(addr)}` : shortAddr(addr);
    }
    return shortAddr(normalizeWalletAddress(value?.execution_wallet_address)) || '未设置';
}

function ensureAutoFollowDraftExecutionWallet(draft, wallets) {
    if (Number(draft?.execution_wallet_id) > 0) return draft;
    const source = Array.isArray(wallets) ? wallets : [];
    const wallet = source.find((item) => item?.is_default) || source[0];
    if (!wallet) return draft;
    return {
        ...draft,
        execution_wallet_id: String(wallet.id),
        execution_wallet_address: normalizeWalletAddress(wallet.address),
    };
}

function createAutoFollowDraft(config) {
    if (!config) return { ...AUTO_FOLLOW_DEFAULT_DRAFT };
    const ratio = Number(config.ratio);
    const wallets = normalizeAutoFollowWalletList(config);
    return {
        id: Number(config.id) || 0,
        target_wallet_address: String(config.target_wallet_address || ''),
        target_wallet_addresses: wallets,
        execution_wallet_id: config.execution_wallet_id ? String(config.execution_wallet_id) : '',
        execution_wallet_address: normalizeWalletAddress(config.execution_wallet_address),
        trigger_mode: config.trigger_mode === 'threshold' ? 'threshold' : 'any',
        trigger_min_wallets: config.trigger_min_wallets != null ? String(config.trigger_min_wallets) : '2',
        trigger_window_seconds: config.trigger_window_seconds != null ? String(config.trigger_window_seconds) : '300',
        enabled: Boolean(config.enabled),
        amount_mode: String(config.amount_mode || 'fixed'),
        fixed_amount_usdt: String(config.fixed_amount_usdt || ''),
        ratio_percent: Number.isFinite(ratio) ? String(ratio * 100) : '100',
        delay_mode: String(config.delay_mode || 'immediate'),
        delay_seconds: String(config.delay_seconds || '0'),
        follow_close: Boolean(config.follow_close),
    };
}

function normalizeAutoFollowDraft(draft) {
    const wallets = parseAutoFollowWalletInputs(draft.target_wallet_addresses);
    const executionWalletID = Number(draft.execution_wallet_id);
    if (!Number.isFinite(executionWalletID) || executionWalletID <= 0) throw new Error('请选择执行钱包');
    const executionWalletAddress = normalizeWalletAddress(draft.execution_wallet_address);

    const amountMode = String(draft.amount_mode || '').trim();
    const fixedAmount = Number(String(draft.fixed_amount_usdt || '').trim());
    const ratioPercent = Number(String(draft.ratio_percent || '').trim());
    const delayMode = String(draft.delay_mode || '').trim();
    const delaySeconds = Number(String(draft.delay_seconds || '').trim());

    if (amountMode !== 'fixed' && amountMode !== 'ratio') throw new Error('请选择跟单金额模式');
    if (amountMode === 'fixed' && (!Number.isFinite(fixedAmount) || fixedAmount <= 0)) throw new Error('固定金额必须大于 0');
    if (amountMode === 'ratio' && (!Number.isFinite(ratioPercent) || ratioPercent <= 0)) throw new Error('比例必须大于 0');
    if (delayMode !== 'immediate' && delayMode !== 'fixed_delay') throw new Error('请选择跟单延时模式');
    if (delayMode === 'fixed_delay' && (!Number.isFinite(delaySeconds) || delaySeconds < 0 || delaySeconds > 86400)) {
        throw new Error('延时秒数必须在 0 到 86400 之间');
    }
    const triggerMode = draft.trigger_mode === 'threshold' ? 'threshold' : 'any';
    let triggerMinWallets = 1;
    let triggerWindowSeconds = 300;
    if (triggerMode === 'threshold') {
        triggerMinWallets = Math.round(Number(String(draft.trigger_min_wallets || '').trim()));
        triggerWindowSeconds = Math.round(Number(String(draft.trigger_window_seconds || '').trim()));
        if (!Number.isFinite(triggerMinWallets) || triggerMinWallets < 2) throw new Error('触发钱包数至少为 2');
        if (triggerMinWallets > wallets.length) throw new Error('触发钱包数不能超过目标钱包数量');
        if (!Number.isFinite(triggerWindowSeconds) || triggerWindowSeconds <= 0 || triggerWindowSeconds > 86400) {
            throw new Error('统计窗口必须在 1 到 86400 秒之间');
        }
    }

    return {
        id: Number(draft.id) || 0,
        target_wallet_address: wallets[0],
        target_wallet_addresses: wallets,
        execution_wallet_id: executionWalletID,
        execution_wallet_address: executionWalletAddress,
        trigger_mode: triggerMode,
        trigger_min_wallets: triggerMinWallets,
        trigger_window_seconds: triggerWindowSeconds,
        enabled: Boolean(draft.enabled),
        amount_mode: amountMode,
        fixed_amount_usdt: amountMode === 'fixed' ? fixedAmount : 0,
        ratio: amountMode === 'ratio' ? ratioPercent / 100 : 1,
        delay_mode: delayMode,
        delay_seconds: delayMode === 'fixed_delay' ? Math.round(delaySeconds) : 0,
        follow_close: Boolean(draft.follow_close),
    };
}

function formatAutoFollowJobTime(value) {
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) return '--';
    return date.toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' });
}

function autoFollowStatusClass(status) {
    switch (String(status || '').toLowerCase()) {
        case 'success':
        case 'created':
            return 'border-emerald-400/20 bg-emerald-500/10 text-emerald-200';
        case 'failed':
            return 'border-red-400/20 bg-red-500/10 text-red-200';
        case 'running':
        case 'matched':
            return 'border-amber-400/20 bg-amber-500/10 text-amber-200';
        case 'skipped':
            return 'border-white/10 bg-zinc-800/80 text-zinc-400';
        default:
            return 'border-sky-400/20 bg-sky-500/10 text-sky-200';
    }
}

function autoFollowEventActionLabel(eventType) {
    return String(eventType || '').toLowerCase() === 'remove' ? '撤 LP' : '加 LP';
}

function autoFollowEventActionClass(eventType) {
    return String(eventType || '').toLowerCase() === 'remove'
        ? 'border-red-400/20 bg-red-500/10 text-red-200'
        : 'border-emerald-400/20 bg-emerald-500/10 text-emerald-200';
}

function formatAutoFollowEventAmount(event) {
    const total = Number(event?.total_usd);
    if (Number.isFinite(total) && total > 0) return formatUSDCompact(total);
    return '--';
}

function formatAutoFollowEventRange(event) {
    if (event?.tick_lower === null || event?.tick_lower === undefined || event?.tick_upper === null || event?.tick_upper === undefined) {
        return '';
    }
    return `${event.tick_lower} - ${event.tick_upper}`;
}

function AutoFollowPage({ apiBaseUrl, initData, hasInitData, brand }) {
    const [loading, setLoading] = useState(Boolean(hasInitData));
    const [saving, setSaving] = useState(false);
    const [configs, setConfigs] = useState([]);
    const [jobs, setJobs] = useState([]);
    const [attempts, setAttempts] = useState([]);
    const [targetEvents, setTargetEvents] = useState([]);
    const [jobEvents, setJobEvents] = useState([]);
    const [executionWallets, setExecutionWallets] = useState([]);
    const [draft, setDraft] = useState(() => createAutoFollowDraft());
    const [activeTab, setActiveTab] = useState('configure');
    const [error, setError] = useState('');
    const [notice, setNotice] = useState('');

    const load = useCallback(async () => {
        if (!hasInitData) {
            setLoading(false);
            setConfigs([]);
            setJobs([]);
            setAttempts([]);
            setTargetEvents([]);
            setJobEvents([]);
            setExecutionWallets([]);
            return;
        }
        setLoading(true);
        setError('');
        try {
            const resp = await fetchSMAutoFollow({ apiBaseUrl, initData, chain: 'bsc' });
            const wallets = Array.isArray(resp?.wallets) ? resp.wallets : [];
            setExecutionWallets(wallets);
            setConfigs(Array.isArray(resp?.configs) ? resp.configs : []);
            setJobs(Array.isArray(resp?.jobs) ? resp.jobs : []);
            setAttempts(Array.isArray(resp?.attempts) ? resp.attempts : []);
            setTargetEvents(Array.isArray(resp?.target_events) ? resp.target_events : []);
            setJobEvents(Array.isArray(resp?.job_events) ? resp.job_events : []);
            setDraft((prev) => ensureAutoFollowDraftExecutionWallet(prev, wallets));
        } catch (err) {
            setError(String(err?.message || err || '加载失败'));
        } finally {
            setLoading(false);
        }
    }, [apiBaseUrl, hasInitData, initData]);

    useEffect(() => {
        load();
    }, [load]);

    const resetDraft = useCallback(() => {
        setDraft(ensureAutoFollowDraftExecutionWallet(createAutoFollowDraft(), executionWallets));
    }, [executionWallets]);

    const saveDraft = useCallback(async () => {
        if (!hasInitData) {
            setError('缺少 Telegram initData');
            return;
        }
        if (executionWallets.length === 0) {
            setError('没有可用执行钱包');
            return;
        }
        setSaving(true);
        setError('');
        setNotice('');
        try {
            const config = normalizeAutoFollowDraft(draft);
            await saveSMAutoFollowConfig({ apiBaseUrl, initData, chain: 'bsc', config });
            setNotice('自动跟单配置已保存');
            resetDraft();
            await load();
        } catch (err) {
            setError(String(err?.message || err || '保存失败'));
        } finally {
            setSaving(false);
        }
    }, [apiBaseUrl, draft, executionWallets.length, hasInitData, initData, load, resetDraft]);

    const deleteConfig = useCallback(async (id) => {
        if (!id || !hasInitData) return;
        setSaving(true);
        setError('');
        setNotice('');
        try {
            await deleteSMAutoFollowConfig({ apiBaseUrl, initData, chain: 'bsc', id });
            setNotice('配置已删除');
            await load();
        } catch (err) {
            setError(String(err?.message || err || '删除失败'));
        } finally {
            setSaving(false);
        }
    }, [apiBaseUrl, hasInitData, initData, load]);

    const toggleConfig = useCallback(async (config) => {
        if (!config || !hasInitData) return;
        setSaving(true);
        setError('');
        setNotice('');
        try {
            await saveSMAutoFollowConfig({
                apiBaseUrl,
                initData,
                chain: 'bsc',
                config: {
                    id: config.id,
                    chain: config.chain,
                    target_wallet_address: config.target_wallet_address,
                    target_wallet_addresses: normalizeAutoFollowWalletList(config).filter(Boolean),
                    execution_wallet_id: Number(config.execution_wallet_id || 0),
                    execution_wallet_address: config.execution_wallet_address || '',
                    trigger_mode: config.trigger_mode || 'any',
                    trigger_min_wallets: Number(config.trigger_min_wallets || 1),
                    trigger_window_seconds: Number(config.trigger_window_seconds || 300),
                    enabled: !config.enabled,
                    amount_mode: config.amount_mode,
                    fixed_amount_usdt: config.fixed_amount_usdt,
                    ratio: config.ratio,
                    delay_mode: config.delay_mode,
                    delay_seconds: config.delay_seconds,
                    follow_close: config.follow_close,
                },
            });
            await load();
        } catch (err) {
            setError(String(err?.message || err || '更新失败'));
        } finally {
            setSaving(false);
        }
    }, [apiBaseUrl, hasInitData, initData, load]);

    const activeCount = configs.filter((item) => item?.enabled).length;
    const autoFollowEventByID = useMemo(() => {
        const map = new Map();
        [...targetEvents, ...jobEvents].forEach((event) => {
            const id = Number(event?.id) || 0;
            if (id > 0) map.set(id, event);
        });
        return map;
    }, [jobEvents, targetEvents]);
    const autoFollowJobByEventID = useMemo(() => {
        const map = new Map();
        jobs.forEach((job) => {
            const put = (value) => {
                const id = Number(value) || 0;
                if (id > 0 && !map.has(id)) map.set(id, job);
            };
            put(job?.event_id);
            if (Array.isArray(job?.trigger_event_ids)) {
                job.trigger_event_ids.forEach(put);
            }
        });
        return map;
    }, [jobs]);
    const autoFollowAttemptByEventID = useMemo(() => {
        const map = new Map();
        attempts.forEach((attempt) => {
            const id = Number(attempt?.event_id) || 0;
            if (id > 0 && !map.has(id)) map.set(id, attempt);
        });
        return map;
    }, [attempts]);
    const autoFollowFlowItems = useMemo(() => {
        const items = [];
        const seenAttempts = new Set();
        jobs.forEach((job) => {
            const attempt = attempts.find((item) => Number(item?.job_id) === Number(job?.id));
            if (attempt?.id) seenAttempts.add(Number(attempt.id));
            items.push({ kind: 'job', job, attempt, time: job?.created_at || job?.scheduled_at });
        });
        attempts.forEach((attempt) => {
            const id = Number(attempt?.id) || 0;
            if (id > 0 && seenAttempts.has(id)) return;
            items.push({ kind: 'attempt', attempt, time: attempt?.created_at || attempt?.updated_at });
        });
        return items.sort((a, b) => new Date(b.time || 0).getTime() - new Date(a.time || 0).getTime());
    }, [attempts, jobs]);
    const draftWallets = Array.isArray(draft.target_wallet_addresses) && draft.target_wallet_addresses.length > 0
        ? draft.target_wallet_addresses
        : [''];
    const setDraftWallet = (index, value) => {
        setDraft((prev) => {
            const current = Array.isArray(prev.target_wallet_addresses) && prev.target_wallet_addresses.length > 0
                ? prev.target_wallet_addresses
                : [''];
            const next = current.map((wallet, i) => (i === index ? value : wallet));
            return { ...prev, target_wallet_addresses: next, target_wallet_address: next[0] || '' };
        });
    };
    const addDraftWallet = () => {
        setDraft((prev) => {
            const current = Array.isArray(prev.target_wallet_addresses) && prev.target_wallet_addresses.length > 0
                ? prev.target_wallet_addresses
                : [''];
            return { ...prev, target_wallet_addresses: [...current, ''] };
        });
    };
    const removeDraftWallet = (index) => {
        setDraft((prev) => {
            const current = Array.isArray(prev.target_wallet_addresses) && prev.target_wallet_addresses.length > 0
                ? prev.target_wallet_addresses
                : [''];
            const next = current.filter((_, i) => i !== index);
            const finalWallets = next.length ? next : [''];
            return { ...prev, target_wallet_addresses: finalWallets, target_wallet_address: finalWallets[0] || '' };
        });
    };
    const setExecutionWallet = (id) => {
        const wallet = findAutoFollowExecutionWallet(executionWallets, id, '');
        setDraft((prev) => ({
            ...prev,
            execution_wallet_id: wallet ? String(wallet.id) : '',
            execution_wallet_address: wallet ? normalizeWalletAddress(wallet.address) : '',
        }));
    };

    return (
        <div className="mini-sm-follow-page space-y-4">
            <section className="rounded-[30px] border border-white/[0.04] bg-[radial-gradient(circle_at_top_left,rgba(59,130,246,0.14),transparent_34%),linear-gradient(180deg,rgba(24,24,27,0.94),rgba(9,9,11,0.98))] p-4 shadow-[0_28px_90px_-42px_rgba(0,0,0,0.95)]">
                <div className="flex items-start justify-between gap-3">
                    <div className="flex min-w-0 items-start gap-3">
                        <div className="inline-flex h-11 w-11 shrink-0 items-center justify-center rounded-2xl border border-sky-400/20 bg-sky-400/10 text-sky-200">
                            <Copy size={18} />
                        </div>
                        <div className="min-w-0">
                            <div className="text-base font-semibold text-zinc-100">自动跟单</div>
                            <div className="mt-2 flex flex-wrap items-center gap-1.5">
                                <Badge className="border-sky-400/20 bg-sky-400/10 text-sky-200">BSC</Badge>
                                <Badge className="border-white/10 bg-zinc-800/80 text-zinc-300">{activeCount} 个运行中</Badge>
                            </div>
                        </div>
                    </div>
                    <button
                        type="button"
                        onClick={load}
                        disabled={loading}
                        className={`mini-sm-follow-secondary rounded-2xl px-3 py-2 text-sm disabled:opacity-50 ${getFilterButtonClass(false, brand)}`}
                    >
                        刷新
                    </button>
                </div>
                <div className="mt-4 grid grid-cols-3 gap-2">
                    <StatCard label="配置数" value={configs.length} compact />
                    <StatCard label="运行中" value={activeCount} compact />
                    <StatCard label="最近任务" value={autoFollowFlowItems.length} compact />
                </div>
            </section>

            {!hasInitData ? (
                <div className="rounded-2xl border border-red-500/20 bg-red-500/10 px-3 py-2 text-sm text-red-200">
                    缺少 Telegram initData，无法保存自动跟单配置。
                </div>
            ) : null}
            {error ? (
                <div className="rounded-2xl border border-red-500/20 bg-red-500/10 px-3 py-2 text-sm text-red-200">
                    {error}
                </div>
            ) : null}
            {!error && notice ? (
                <div className="rounded-2xl border border-emerald-500/20 bg-emerald-500/10 px-3 py-2 text-sm text-emerald-200">
                    {notice}
                </div>
            ) : null}

            <div className="mini-sm-follow-tabs grid grid-cols-3 gap-1.5 rounded-[22px] border border-white/[0.05] bg-zinc-950/50 p-1.5">
                {[
                    { key: 'configure', label: draft.id ? '编辑任务' : '配置任务', icon: Settings },
                    { key: 'configs', label: '我的跟单', icon: Users, count: configs.length },
                    { key: 'jobs', label: '最近任务', icon: Activity, count: autoFollowFlowItems.length },
                ].map(({ key, label, icon: Icon, count }) => (
                    <button
                        key={key}
                        type="button"
                        onClick={() => setActiveTab(key)}
                        data-active={activeTab === key}
                        className={`relative inline-flex min-h-[54px] flex-col items-center justify-center gap-1 rounded-[18px] px-1.5 py-2 text-[11px] font-semibold leading-tight transition active:scale-[0.98] ${
                            activeTab === key
                                ? `${brand.softButtonClass}`
                                : 'text-zinc-500 hover:bg-white/[0.04] hover:text-zinc-300'
                        }`}
                    >
                        <Icon size={14} />
                        <span className="text-center">{label}</span>
                        {typeof count === 'number' ? (
                            <span className="absolute right-1.5 top-1.5 min-w-4 rounded-full bg-white/10 px-1 text-[9px] font-bold tabular-nums text-zinc-300">
                                {count}
                            </span>
                        ) : null}
                    </button>
                ))}
            </div>

            {activeTab === 'configure' ? (
            <section className="rounded-[24px] border border-white/[0.04] bg-zinc-900/60 p-4">
                <div className="mb-3 flex items-center justify-between gap-3">
                    <div className="text-sm font-semibold text-zinc-100">{draft.id ? '编辑配置' : '新增配置'}</div>
                    {draft.id ? (
                        <button type="button" onClick={resetDraft} className="text-xs text-zinc-400 hover:text-zinc-200">
                            新建
                        </button>
                    ) : null}
                </div>

                <div className="space-y-3">
                    <div className="space-y-2">
                        <div className="text-[11px] font-medium text-zinc-500">执行钱包</div>
                        {loading && executionWallets.length === 0 ? (
                            <div className="rounded-2xl border border-white/[0.04] bg-zinc-950/45 px-3 py-3 text-xs text-zinc-500">
                                加载钱包中...
                            </div>
                        ) : (
                            <div className="grid gap-2 sm:grid-cols-2">
                                {executionWallets.map((wallet) => {
                                    const active = Number(draft.execution_wallet_id) === Number(wallet.id);
                                    const addr = normalizeWalletAddress(wallet.address);
                                    return (
                                        <button
                                            key={wallet.id}
                                            type="button"
                                            onClick={() => setExecutionWallet(wallet.id)}
                                            className={`flex min-w-0 items-center gap-2 rounded-2xl border px-3 py-2.5 text-left transition ${
                                                active
                                                    ? 'border-sky-400/30 bg-sky-400/10 text-sky-100'
                                                    : 'border-white/[0.05] bg-zinc-950/45 text-zinc-400 hover:bg-white/[0.04]'
                                            }`}
                                        >
                                            <Wallet size={14} className="shrink-0" />
                                            <span className="min-w-0">
                                                <span className="block truncate text-sm font-semibold">
                                                    {String(wallet.name || '').trim() || `钱包 #${wallet.id}`}
                                                </span>
                                                <span className="block truncate text-[10px] text-zinc-500">
                                                    {shortAddr(addr)}{wallet.is_default ? ' · 默认' : ''}
                                                </span>
                                            </span>
                                        </button>
                                    );
                                })}
                            </div>
                        )}
                    </div>

                    <div className="space-y-2">
                        <div className="flex items-center justify-between gap-2">
                            <div className="text-[11px] font-medium text-zinc-500">目标钱包</div>
                            <button
                                type="button"
                                onClick={addDraftWallet}
                                className={`mini-sm-follow-secondary rounded-xl px-2.5 py-1.5 text-xs ${getFilterButtonClass(false, brand)}`}
                            >
                                <Plus size={12} className="mr-1 inline" /> 添加
                            </button>
                        </div>
                        {draftWallets.map((wallet, index) => (
                            <div key={index} className="grid grid-cols-[minmax(0,1fr)_38px] gap-2">
                                <input
                                    className={getInputClass(brand)}
                                    placeholder="跟单钱包地址 (0x...)"
                                    value={wallet}
                                    onChange={(e) => setDraftWallet(index, e.target.value)}
                                />
                                <button
                                    type="button"
                                    onClick={() => removeDraftWallet(index)}
                                    disabled={draftWallets.length === 1}
                                    className={`${getIconButtonClass(true)} h-full min-h-[42px] disabled:opacity-40`}
                                    title="移除钱包"
                                >
                                    <X size={14} />
                                </button>
                            </div>
                        ))}
                    </div>

                    <div className="space-y-2">
                        <div className="grid grid-cols-2 gap-2">
                            <button
                                type="button"
                                data-active={draft.trigger_mode !== 'threshold'}
                                className={`rounded-2xl px-3 py-2.5 text-sm ${getFilterButtonClass(draft.trigger_mode !== 'threshold', brand)}`}
                                onClick={() => setDraft((prev) => ({ ...prev, trigger_mode: 'any' }))}
                            >
                                <Zap size={13} className="mr-1 inline" /> 任意钱包
                            </button>
                            <button
                                type="button"
                                data-active={draft.trigger_mode === 'threshold'}
                                className={`rounded-2xl px-3 py-2.5 text-sm ${getFilterButtonClass(draft.trigger_mode === 'threshold', brand)}`}
                                onClick={() => setDraft((prev) => ({ ...prev, trigger_mode: 'threshold' }))}
                            >
                                <Users size={13} className="mr-1 inline" /> 多钱包确认
                            </button>
                        </div>
                        {draft.trigger_mode === 'threshold' ? (
                            <div className="grid grid-cols-2 gap-2">
                                <input
                                    className={getInputClass(brand)}
                                    type="number"
                                    min="2"
                                    step="1"
                                    placeholder="触发数量"
                                    value={draft.trigger_min_wallets}
                                    onChange={(e) => setDraft((prev) => ({ ...prev, trigger_min_wallets: e.target.value }))}
                                />
                                <input
                                    className={getInputClass(brand)}
                                    type="number"
                                    min="1"
                                    step="1"
                                    placeholder="统计窗口(秒)"
                                    value={draft.trigger_window_seconds}
                                    onChange={(e) => setDraft((prev) => ({ ...prev, trigger_window_seconds: e.target.value }))}
                                />
                            </div>
                        ) : null}
                    </div>

                    <div className="grid grid-cols-2 gap-2">
                        <button
                            type="button"
                            data-active={draft.enabled}
                            className={`rounded-2xl px-3 py-2.5 text-sm ${getFilterButtonClass(draft.enabled, brand)}`}
                            onClick={() => setDraft((prev) => ({ ...prev, enabled: true }))}
                        >
                            <Play size={13} className="mr-1 inline" /> 开启
                        </button>
                        <button
                            type="button"
                            data-active={!draft.enabled}
                            className={`rounded-2xl px-3 py-2.5 text-sm ${getFilterButtonClass(!draft.enabled, brand)}`}
                            onClick={() => setDraft((prev) => ({ ...prev, enabled: false }))}
                        >
                            <Pause size={13} className="mr-1 inline" /> 暂停
                        </button>
                    </div>

                    <div className="grid grid-cols-2 gap-2">
                        <button
                            type="button"
                            data-active={draft.amount_mode === 'fixed'}
                            className={`rounded-2xl px-3 py-2.5 text-sm ${getFilterButtonClass(draft.amount_mode === 'fixed', brand)}`}
                            onClick={() => setDraft((prev) => ({ ...prev, amount_mode: 'fixed' }))}
                        >
                            <DollarSign size={13} className="mr-1 inline" /> 固定金额
                        </button>
                        <button
                            type="button"
                            data-active={draft.amount_mode === 'ratio'}
                            className={`rounded-2xl px-3 py-2.5 text-sm ${getFilterButtonClass(draft.amount_mode === 'ratio', brand)}`}
                            onClick={() => setDraft((prev) => ({ ...prev, amount_mode: 'ratio' }))}
                        >
                            <Percent size={13} className="mr-1 inline" /> 按比例
                        </button>
                    </div>

                    {draft.amount_mode === 'fixed' ? (
                        <input
                            className={getInputClass(brand)}
                            type="number"
                            min="0"
                            step="0.01"
                            placeholder="固定金额 USDT"
                            value={draft.fixed_amount_usdt}
                            onChange={(e) => setDraft((prev) => ({ ...prev, fixed_amount_usdt: e.target.value }))}
                        />
                    ) : (
                        <input
                            className={getInputClass(brand)}
                            type="number"
                            min="0"
                            step="1"
                            placeholder="开仓金额比例 (%)"
                            value={draft.ratio_percent}
                            onChange={(e) => setDraft((prev) => ({ ...prev, ratio_percent: e.target.value }))}
                        />
                    )}

                    <div className="grid grid-cols-2 gap-2">
                        <button
                            type="button"
                            data-active={draft.delay_mode === 'immediate'}
                            className={`rounded-2xl px-3 py-2.5 text-sm ${getFilterButtonClass(draft.delay_mode === 'immediate', brand)}`}
                            onClick={() => setDraft((prev) => ({ ...prev, delay_mode: 'immediate', delay_seconds: '0' }))}
                        >
                            <Clock size={13} className="mr-1 inline" /> 立即
                        </button>
                        <button
                            type="button"
                            data-active={draft.delay_mode === 'fixed_delay'}
                            className={`rounded-2xl px-3 py-2.5 text-sm ${getFilterButtonClass(draft.delay_mode === 'fixed_delay', brand)}`}
                            onClick={() => setDraft((prev) => ({ ...prev, delay_mode: 'fixed_delay' }))}
                        >
                            <Clock size={13} className="mr-1 inline" /> 延时
                        </button>
                    </div>

                    {draft.delay_mode === 'fixed_delay' ? (
                        <input
                            className={getInputClass(brand)}
                            type="number"
                            min="0"
                            step="1"
                            placeholder="延时秒数"
                            value={draft.delay_seconds}
                            onChange={(e) => setDraft((prev) => ({ ...prev, delay_seconds: e.target.value }))}
                        />
                    ) : null}

                    <label className="flex items-center justify-between gap-3 rounded-2xl border border-white/[0.04] bg-zinc-950/45 px-3 py-3">
                        <span className="text-sm text-zinc-200">撤仓跟单</span>
                        <input
                            type="checkbox"
                            className="h-5 w-5 accent-lime-400"
                            checked={draft.follow_close}
                            onChange={(e) => setDraft((prev) => ({ ...prev, follow_close: e.target.checked }))}
                        />
                    </label>

                    <button
                        type="button"
                        onClick={saveDraft}
                        disabled={saving || !hasInitData || executionWallets.length === 0}
                        className={`mini-sm-follow-submit w-full rounded-[24px] px-4 py-3 text-sm font-semibold disabled:opacity-50 ${brand.solidButtonClass}`}
                    >
                        {saving ? '保存中...' : '保存自动跟单配置'}
                    </button>
                </div>
            </section>
            ) : null}

            {activeTab === 'configs' ? (
            <section className="space-y-2">
                <div className="text-sm font-semibold text-zinc-100">跟单配置</div>
                {loading ? (
                    <div className="py-6 text-center text-zinc-500">加载中...</div>
                ) : configs.length === 0 ? (
                    <div className="rounded-2xl border border-white/[0.04] bg-zinc-900/55 px-3 py-4 text-center text-sm text-zinc-500">
                        暂无自动跟单配置，可到“配置任务”新增。
                    </div>
                ) : (
                    configs.map((config) => {
                        const wallets = normalizeAutoFollowWalletList(config).filter(Boolean);
                        return (
                            <div key={config.id} className="rounded-2xl border border-white/[0.04] bg-zinc-900/60 p-3">
                                <div className="flex items-start justify-between gap-3">
                                    <div className="min-w-0">
                                        <div className="flex items-center gap-2">
                                            <span className="truncate font-mono text-sm text-zinc-100">
                                                {wallets.length > 1 ? `${shortAddr(wallets[0])} +${wallets.length - 1}` : shortAddr(config.target_wallet_address)}
                                            </span>
                                            <Badge className={config.enabled
                                                ? 'border-emerald-400/20 bg-emerald-500/10 text-emerald-200'
                                                : 'border-white/10 bg-zinc-800/80 text-zinc-400'}>
                                                {config.enabled ? '开启' : '暂停'}
                                            </Badge>
                                        </div>
                                        <div className="mt-2 flex flex-wrap gap-1.5">
                                            <Badge className="border-white/10 bg-zinc-800/80 text-zinc-300">
                                                {autoFollowTriggerText(config)}
                                            </Badge>
                                            <Badge className="border-white/10 bg-zinc-800/80 text-zinc-300">
                                                执行 {formatAutoFollowExecutionWallet(config, executionWallets)}
                                            </Badge>
                                            <Badge className="border-white/10 bg-zinc-800/80 text-zinc-300">
                                                {config.amount_mode === 'ratio'
                                                    ? `${Number(config.ratio * 100).toFixed(0)}%`
                                                    : formatUSDCompact(config.fixed_amount_usdt)}
                                            </Badge>
                                            <Badge className="border-white/10 bg-zinc-800/80 text-zinc-300">
                                                {config.delay_mode === 'fixed_delay' ? `${config.delay_seconds}s` : '立即'}
                                            </Badge>
                                            <Badge className="border-white/10 bg-zinc-800/80 text-zinc-300">
                                                撤仓{config.follow_close ? '跟单' : '忽略'}
                                            </Badge>
                                        </div>
                                    </div>
                                    <div className="flex shrink-0 gap-1.5">
                                        <button type="button" className={getIconButtonClass(false)} onClick={() => { setDraft(ensureAutoFollowDraftExecutionWallet(createAutoFollowDraft(config), executionWallets)); setActiveTab('configure'); }} title="编辑">
                                            <Pencil size={14} />
                                        </button>
                                        <button type="button" className={getIconButtonClass(false)} onClick={() => toggleConfig(config)} title={config.enabled ? '暂停' : '开启'} disabled={saving}>
                                            {config.enabled ? <Pause size={14} /> : <Play size={14} />}
                                        </button>
                                        <button type="button" className={getIconButtonClass(true)} onClick={() => deleteConfig(config.id)} title="删除" disabled={saving}>
                                            <Trash2 size={14} />
                                        </button>
                                    </div>
                                </div>
                            </div>
                        );
                    })
                )}
            </section>
            ) : null}

            {activeTab === 'jobs' ? (
            <section className="space-y-2">
                <div className="text-sm font-semibold text-zinc-100">最近任务</div>
                {autoFollowFlowItems.length === 0 && targetEvents.length === 0 ? (
                    <div className="rounded-2xl border border-white/[0.04] bg-zinc-900/55 px-3 py-4 text-center text-sm text-zinc-500">
                        暂无目标事件和执行记录
                    </div>
                ) : (
                    <div className="grid gap-3 lg:grid-cols-2">
                        <div className="space-y-2">
                            <div className="flex items-center justify-between gap-2 text-[11px] font-medium text-zinc-500">
                                <span>被跟单钱包事件流</span>
                                <span>{targetEvents.length}</span>
                            </div>
                            {targetEvents.length === 0 ? (
                                <div className="rounded-2xl border border-white/[0.04] bg-zinc-900/55 px-3 py-4 text-center text-sm text-zinc-500">
                                    暂无目标钱包 LP 事件
                                </div>
                            ) : (
                                targetEvents.slice(0, 8).map((event) => {
                                    const linkedJob = autoFollowJobByEventID.get(Number(event.id) || 0);
                                    const linkedAttempt = autoFollowAttemptByEventID.get(Number(event.id) || 0);
                                    const followStatus = linkedJob?.status || linkedAttempt?.status || '';
                                    const rangeText = formatAutoFollowEventRange(event);
                                    return (
                                        <div key={event.id} className="rounded-2xl border border-white/[0.04] bg-zinc-900/55 p-3">
                                            <div className="flex items-start justify-between gap-3">
                                                <div className="min-w-0">
                                                    <div className="flex flex-wrap items-center gap-1.5">
                                                        <Badge className={autoFollowEventActionClass(event.event_type)}>
                                                            {autoFollowEventActionLabel(event.event_type)}
                                                        </Badge>
                                                        {followStatus ? (
                                                            <Badge className={autoFollowStatusClass(followStatus)}>
                                                                跟单 {followStatus}
                                                            </Badge>
                                                        ) : (
                                                            <Badge className="border-white/10 bg-zinc-800/80 text-zinc-400">
                                                                未生成跟单
                                                            </Badge>
                                                        )}
                                                    </div>
                                                    <div className="mt-1 truncate text-sm font-semibold text-zinc-100">{getPairLabel(event)}</div>
                                                    <div className="mt-1 truncate text-[11px] text-zinc-500">
                                                        {shortAddr(event.wallet_address)}
                                                        {' · '}
                                                        {formatAutoFollowJobTime(event.tx_timestamp)}
                                                        {rangeText ? ` · Tick ${rangeText}` : ''}
                                                    </div>
                                                    {!linkedJob && linkedAttempt?.message ? (
                                                        <div className="mt-1 text-[11px] text-red-200 line-clamp-2">{linkedAttempt.message}</div>
                                                    ) : null}
                                                </div>
                                                <div className="shrink-0 text-right">
                                                    <div className="text-sm font-semibold text-zinc-100">{formatAutoFollowEventAmount(event)}</div>
                                                    <div className="text-[10px] text-zinc-500">事件 #{event.id}</div>
                                                </div>
                                            </div>
                                        </div>
                                    );
                                })
                            )}
                        </div>

                        <div className="space-y-2">
                            <div className="flex items-center justify-between gap-2 text-[11px] font-medium text-zinc-500">
                                <span>我们的跟单事件流</span>
                                <span>{autoFollowFlowItems.length}</span>
                            </div>
                            {autoFollowFlowItems.length === 0 ? (
                                <div className="rounded-2xl border border-white/[0.04] bg-zinc-900/55 px-3 py-4 text-center text-sm text-zinc-500">
                                    暂无跟单任务
                                </div>
                            ) : (
                                autoFollowFlowItems.slice(0, 8).map((item) => {
                                    const job = item.job;
                                    const attempt = item.attempt;
                                    const row = job || attempt;
                                    const triggerWallets = Array.isArray(job?.trigger_wallet_addresses) ? job.trigger_wallet_addresses.filter(Boolean) : [];
                                    const event = autoFollowEventByID.get(Number(row?.event_id) || 0);
                                    const status = job?.status || attempt?.status || '';
                                    const action = job?.action || attempt?.action;
                                    const message = job?.error_message || attempt?.message || '';
                                    return (
                                        <div key={`${item.kind}:${row?.id || row?.event_id}`} className="rounded-2xl border border-white/[0.04] bg-zinc-900/55 p-3">
                                            <div className="flex items-start justify-between gap-3">
                                                <div className="min-w-0">
                                                    <div className="flex flex-wrap items-center gap-1.5">
                                                        <Badge className={autoFollowStatusClass(status)}>{status}</Badge>
                                                        <span className="text-sm text-zinc-100">{action === 'close' ? '撤仓' : '开仓'}</span>
                                                        {item.kind === 'attempt' ? (
                                                            <Badge className="border-red-400/20 bg-red-500/10 text-red-200">未建任务</Badge>
                                                        ) : null}
                                                    </div>
                                                    <div className="mt-1 truncate text-sm font-semibold text-zinc-100">
                                                        {event ? getPairLabel(event) : `事件 #${row?.event_id || '--'}`}
                                                    </div>
                                                    <div className="mt-1 truncate text-[11px] text-zinc-500">
                                                        {triggerWallets.length > 1 ? `${triggerWallets.length} 钱包触发` : shortAddr(triggerWallets[0] || row?.target_wallet_address)}
                                                        {' · '}
                                                        执行 {formatAutoFollowExecutionWallet(row, executionWallets)}
                                                        {' · '}
                                                        {formatAutoFollowJobTime(job?.scheduled_at || attempt?.created_at)}
                                                    </div>
                                                    {message ? (
                                                        <div className="mt-1 text-[11px] text-red-200 line-clamp-2">{message}</div>
                                                    ) : null}
                                                </div>
                                                <div className="shrink-0 text-right">
                                                    <div className="text-sm font-semibold text-zinc-100">{Number(job?.amount_usdt) > 0 ? formatUSDCompact(job.amount_usdt) : '--'}</div>
                                                    <div className="text-[10px] text-zinc-500">{job ? `任务 #${job.id}` : `尝试 #${attempt?.id}`}</div>
                                                </div>
                                            </div>
                                        </div>
                                    );
                                })
                            )}
                        </div>
                    </div>
                )}
            </section>
            ) : null}
        </div>
    );
}

function ContractSettingsTab({ apiBaseUrl, brand, pollIntervalSec = 15 }) {
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
        }, Math.max(2, Number(pollIntervalSec)) * 1000);
        return () => clearInterval(timer);
    }, [load, pollIntervalSec]);

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
            setError(String(err?.message || err));
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
export default function SmartMoneyPage({ apiBaseUrl, initData = '', hasInitData, isAdmin = false, accentTheme = 'lime', pollIntervalSec = 15, onOpenPosition }) {
    const brand = useMemo(() => getBrandTheme(accentTheme), [accentTheme]);
    const [view, setView] = useState('pools');
    const [stats, setStats] = useState(null);
    const [selectedPool, setSelectedPool] = useState(null);
    const [selectedWallet, setSelectedWallet] = useState(null);
    const [showAddModal, setShowAddModal] = useState(false);
    const [walletRefreshKey, setWalletRefreshKey] = useState(0);
    const [watchedWallets, setWatchedWallets] = useState([]);
    const [watchToggleMap, setWatchToggleMap] = useState({});
    const watchedWalletSet = useMemo(() => new Set(watchedWallets), [watchedWallets]);

    const applyWatchWalletResponse = useCallback((resp) => {
        const nextWallets = Array.isArray(resp?.wallets)
            ? Array.from(new Set(resp.wallets.map((item) => normalizeWalletAddress(item)).filter(Boolean))).sort()
            : [];
        setWatchedWallets(nextWallets);
    }, []);

    const refreshStats = useCallback(() => {
        fetchSMStats({ apiBaseUrl }).then(setStats).catch(() => { });
    }, [apiBaseUrl]);

    useEffect(() => {
        refreshStats();
        const interval = setInterval(() => {
            refreshStats();
        }, Math.max(2, Number(pollIntervalSec)) * 1000);
        return () => clearInterval(interval);
    }, [pollIntervalSec, refreshStats]);

    useEffect(() => {
        if (!hasInitData) return undefined;
        let cancelled = false;
        (async () => {
            try {
                const resp = await fetchSMWatchWallets({ apiBaseUrl, initData, chain: 'bsc' });
                if (cancelled) return;
                applyWatchWalletResponse(resp);
            } catch {
                // ignore watch wallet sync failures
            }
        })();
        return () => {
            cancelled = true;
        };
    }, [apiBaseUrl, applyWatchWalletResponse, hasInitData, initData]);

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

    const handleToggleWatchWallet = useCallback((walletAddress, nextWatched) => {
        const address = normalizeWalletAddress(walletAddress);
        if (!address) return;
        setWatchToggleMap((prev) => ({ ...prev, [address]: true }));

        const clearBusy = () => {
            setWatchToggleMap((prev) => {
                if (!prev[address]) return prev;
                const next = { ...prev };
                delete next[address];
                return next;
            });
        };

        if (!hasInitData) {
            setWatchedWallets((prev) => {
                const next = new Set(prev);
                const shouldWatch = typeof nextWatched === 'boolean' ? nextWatched : !next.has(address);
                if (shouldWatch) next.add(address);
                else next.delete(address);
                return Array.from(next).sort();
            });
            window.setTimeout(clearBusy, 0);
            return;
        }

        const watched = typeof nextWatched === 'boolean' ? nextWatched : !watchedWalletSet.has(address);
        saveSMWatchWallets({ apiBaseUrl, initData, chain: 'bsc', walletAddress: address, watched })
            .then((resp) => {
                applyWatchWalletResponse(resp);
            })
            .catch(() => {
                // ignore remote persistence failures
            })
            .finally(clearBusy);
    }, [apiBaseUrl, applyWatchWalletResponse, hasInitData, initData, watchedWalletSet]);

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
        <div className="mini-sm-page max-w-3xl mx-auto pb-24">
            <section className="mini-sm-shell rounded-[30px] border border-white/[0.04] bg-[radial-gradient(circle_at_top_left,rgba(255,255,255,0.04),transparent_35%),linear-gradient(180deg,rgba(24,24,27,0.92),rgba(9,9,11,0.96))] p-4 shadow-[0_28px_90px_-42px_rgba(0,0,0,0.95)]">
                {stats && !isDetailView && (
                    <div
                        className={`mini-sm-monitor mb-4 rounded-[24px] border px-4 py-3 ${monitorSummary.enabled
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
                    <div className="mini-sm-stats grid grid-cols-2 gap-2 mb-4">
                        <StatCard label="活跃池子" value={stats.active_pool_count} />
                        <StatCard label="监控钱包" value={stats.monitored_wallet_count} />
                        <StatCard label="持仓笔数" value={stats.open_position_count} />
                        <StatCard label="今日关闭" value={stats.closed_today_count} color="text-red-400" />
                    </div>
                )}

                {!isDetailView && (
                    <div className={`mini-sm-tabs grid ${isAdmin ? 'grid-cols-3 sm:grid-cols-7' : 'grid-cols-3 sm:grid-cols-6'} gap-2 mb-4`}>
                        {[
                            { key: 'pools', label: '池子视图', icon: Eye },
                            { key: 'wallets', label: '钱包视图', icon: Wallet },
                            { key: 'watch_activity', label: '特别关注', icon: Activity },
                            { key: 'settings', label: '合约视图', icon: Settings },
                            { key: 'auto_follow', label: '自动跟单', icon: Copy },
                        ].map(({ key, label, icon: Icon }) => (
                            <button
                                key={key}
                                type="button"
                                data-active={view === key}
                                className={`inline-flex min-h-[60px] flex-col items-center justify-center gap-1 rounded-2xl px-2 py-2 text-[11px] leading-tight sm:min-h-0 sm:flex-row sm:gap-1.5 sm:px-3 sm:py-2.5 sm:text-sm ${getFilterButtonClass(view === key, brand)}`}
                                onClick={() => setView(key)}
                            >
                                <Icon size={13} className="shrink-0 sm:h-[14px] sm:w-[14px]" />
                                <span className="text-center whitespace-normal break-words sm:truncate">{label}</span>
                            </button>
                        ))}
                        <button
                            type="button"
                            data-active={view === 'golden_dog'}
                            className={`inline-flex min-h-[60px] flex-col items-center justify-center gap-1 rounded-2xl px-2 py-2 text-[11px] leading-tight sm:min-h-0 sm:flex-row sm:gap-1.5 sm:px-3 sm:py-2.5 sm:text-sm ${getFilterButtonClass(view === 'golden_dog', brand)}`}
                            onClick={() => setView('golden_dog')}
                        >
                            <Flame size={13} className="shrink-0 sm:h-[14px] sm:w-[14px]" />
                            <span className="text-center whitespace-normal break-words sm:truncate">监控通知</span>
                        </button>
                        {isAdmin && (
                            <button
                                type="button"
                                data-active={view === 'assets'}
                                className={`inline-flex min-h-[60px] flex-col items-center justify-center gap-1 rounded-2xl px-2 py-2 text-[11px] leading-tight sm:min-h-0 sm:flex-row sm:gap-1.5 sm:px-3 sm:py-2.5 sm:text-sm ${getFilterButtonClass(view === 'assets', brand)}`}
                                onClick={() => setView('assets')}
                            >
                                <Wallet size={13} className="shrink-0 sm:h-[14px] sm:w-[14px]" />
                                <span className="text-center whitespace-normal break-words sm:truncate">聪明钱资产</span>
                            </button>
                        )}
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
                        watchedWalletSet={watchedWalletSet}
                        watchToggleMap={watchToggleMap}
                        onToggleWatchWallet={handleToggleWatchWallet}
                    />
                ) : view === 'pools' ? (
                    <SmartMoneyPoolViewPage
                        apiBaseUrl={apiBaseUrl}
                        onSelectPool={handleSelectPool}
                        onOpenPosition={onOpenPosition}
                        brand={brand}
                        pollIntervalSec={pollIntervalSec}
                    />
                ) : view === 'wallets' ? (
                    <WalletListPage
                        apiBaseUrl={apiBaseUrl}
                        onSelectWallet={(addr) => setSelectedWallet(addr)}
                        onAddWallet={() => setShowAddModal(true)}
                        brand={brand}
                        refreshKey={walletRefreshKey}
                        watchedWalletSet={watchedWalletSet}
                        watchToggleMap={watchToggleMap}
                        onToggleWatchWallet={handleToggleWatchWallet}
                        pollIntervalSec={pollIntervalSec}
                    />
                ) : view === 'watch_activity' ? (
                    <WatchActivityPage
                        apiBaseUrl={apiBaseUrl}
                        initData={initData}
                        hasInitData={hasInitData}
                        brand={brand}
                        watchedWallets={watchedWallets}
                        watchToggleMap={watchToggleMap}
                        onToggleWatchWallet={handleToggleWatchWallet}
                        onSelectWallet={handleSelectWallet}
                        onSelectPool={handleSelectPool}
                        onOpenWallets={() => setView('wallets')}
                        pollIntervalSec={pollIntervalSec}
                    />
                ) : view === 'golden_dog' ? (
                    <GoldenDogPage
                        apiBaseUrl={apiBaseUrl}
                        initData={initData}
                        brand={brand}
                        watchedWallets={watchedWallets}
                        watchedWalletSet={watchedWalletSet}
                        watchToggleMap={watchToggleMap}
                        onToggleWatchWallet={handleToggleWatchWallet}
                    />
                ) : view === 'auto_follow' ? (
                    <AutoFollowPage
                        apiBaseUrl={apiBaseUrl}
                        initData={initData}
                        hasInitData={hasInitData}
                        brand={brand}
                    />
                ) : view === 'assets' && isAdmin ? (
                    <Suspense fallback={<div className="py-6 text-center text-[11px] text-zinc-500">加载聪明钱资产模块...</div>}>
                        <LazySmartMoneyAssetsPage
                            apiBaseUrl={apiBaseUrl}
                            initData={initData}
                            hasInitData={Boolean(String(initData || '').trim())}
                            isAdmin={isAdmin}
                            pollIntervalSec={pollIntervalSec}
                            accentTheme={accentTheme}
                        />
                    </Suspense>
                ) : (
                    <ContractSettingsPage apiBaseUrl={apiBaseUrl} brand={brand} pollIntervalSec={pollIntervalSec} />
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
