import React, { useEffect, useMemo, useRef, useState } from 'react';
import { formatRelativeTime } from '../lib/time';
import {
    fetchSmartMoneyFollowConfigs,
    fetchSmartMoneyGoldenDogConfig,
    fetchSmartMoneyPoolAdds,
    saveSmartMoneyGoldenDogConfig,
} from '../lib/api';
import { copyToClipboard, hapticImpact, hapticNotification } from '../lib/telegram';
import ModuleHeader from './ModuleHeader.jsx';
import NumberFlowValue from './NumberFlowValue.jsx';
import SmartMoneyFollowModal from './SmartMoneyFollowModal.jsx';
import SmartMoneyPoolAddsModal from './SmartMoneyPoolAddsModal.jsx';
import SmartMoneyWalletPositionsModal from './SmartMoneyWalletPositionsModal.jsx';
import SmartMoneyWatchedWalletsTab from './SmartMoneyWatchedWalletsTab.jsx';
import SmartMoney24hPoolAddsCard from './SmartMoney24hPoolAddsCard.jsx';

const USD_DISPLAY_LIMIT = 1e15;
const usdFormatter = new Intl.NumberFormat('en-US', {
    style: 'currency',
    currency: 'USD',
    maximumFractionDigits: 2,
});
const usdPlainFormatter = new Intl.NumberFormat('en-US', {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
});

function formatUsd(v) {
    const n = Number(v ?? 0);
    if (!Number.isFinite(n) || Math.abs(n) > USD_DISPLAY_LIMIT) return '$--';
    return usdFormatter.format(n);
}

function formatUsdPlain(v) {
    const n = Number(v ?? 0);
    if (!Number.isFinite(n) || Math.abs(n) > USD_DISPLAY_LIMIT) return '--';
    return usdPlainFormatter.format(n);
}

function formatPct(v, digits = 2) {
    const n = Number(v);
    if (!Number.isFinite(n)) return '--';
    return `${n.toFixed(digits)}%`;
}

function formatPrice(v) {
    const n = Number(v ?? 0);
    if (!Number.isFinite(n) || n <= 0) return '--';
    if (n >= 1000) return n.toFixed(2);
    if (n >= 1) return n.toFixed(4).replace(/\.?0+$/, '');
    return n.toPrecision(4);
}

/** 紧凑价格格式 — 极小数值用下标零表示法 (e.g. 0.0₃6959) */
const SUBSCRIPT_DIGITS = ['₀', '₁', '₂', '₃', '₄', '₅', '₆', '₇', '₈', '₉'];
function compactPrice(v) {
    const n = Number(v ?? 0);
    if (!Number.isFinite(n) || n <= 0) return '--';
    if (n >= 1000) return n.toLocaleString('en-US', { maximumFractionDigits: 2 });
    if (n >= 1) return n.toFixed(4).replace(/\.?0+$/, '');
    if (n >= 0.01) return n.toPrecision(4);
    // count leading zeros after "0."
    const s = n.toFixed(20);
    const dotIdx = s.indexOf('.');
    if (dotIdx < 0) return n.toPrecision(4);
    let zeroCount = 0;
    for (let i = dotIdx + 1; i < s.length; i++) {
        if (s[i] === '0') zeroCount++;
        else break;
    }
    if (zeroCount < 2) return n.toPrecision(4);
    // get significant digits (4-5 digits after leading zeros)
    const sigStart = dotIdx + 1 + zeroCount;
    const sigDigits = s.slice(sigStart, sigStart + 4).replace(/0+$/, '') || '0';
    const sub = String(zeroCount).split('').map(d => SUBSCRIPT_DIGITS[Number(d)] || d).join('');
    return `0.0${sub}${sigDigits}`;
}

function formatWindowLabel(windowSec) {
    const sec = Number(windowSec ?? 0);
    if (!Number.isFinite(sec) || sec <= 0) return '';
    const hours = sec / 3600;
    if (hours >= 1 && Math.abs(hours - Math.round(hours)) < 1e-9) return `${Math.round(hours)}h`;
    if (hours >= 1) return `${hours.toFixed(1)}h`;
    const minutes = sec / 60;
    if (minutes >= 1 && Math.abs(minutes - Math.round(minutes)) < 1e-9) return `${Math.round(minutes)}m`;
    if (minutes >= 1) return `${minutes.toFixed(0)}m`;
    return `${Math.round(sec)}s`;
}

function shortHex(value, head = 6, tail = 4) {
    const s = String(value || '').trim();
    if (!s) return '';
    if (s.length <= head + tail + 2) return s;
    return `${s.slice(0, head)}...${s.slice(-tail)}`;
}

function normalizeWalletAddress(value) {
    const s = String(value || '').trim();
    if (!/^0x[0-9a-fA-F]{40}$/.test(s)) return '';
    return `0x${s.slice(2).toLowerCase()}`;
}

async function safeCopy(value, onNotice) {
    const text = String(value || '').trim();
    if (!text) return;
    try {
        await copyToClipboard(text);
        hapticNotification('success');
        if (typeof onNotice === 'function') onNotice('已复制', 'success');
    } catch (e) {
        hapticNotification('error');
        if (typeof onNotice === 'function') onNotice(`复制失败：${String(e?.message || e)}`, 'error');
    }
}

function toIntInRange(v, min, max, fallback) {
    const n = Math.round(Number(String(v ?? '').trim()));
    if (!Number.isFinite(n)) return fallback;
    if (n < min) return min;
    if (n > max) return max;
    return n;
}

/** 池子概览卡片 — 重新设计的钱包明细展示 */
function PoolOverviewCard({
    rank, pair, poolId, version, feePct, walletCount,
    previewStatus, previewWallets, previewError, previewTotalUsd,
    hasNoPreview, defaultShow, needsExpand,
    onCopyPoolId, onViewDetails, onCopyWallet, onLoadPreview, onQuickOpen,
}) {
    const [expanded, setExpanded] = useState(false);
    const displayWallets = expanded ? previewWallets : previewWallets.slice(0, defaultShow);
    const hiddenCount = previewWallets.length - defaultShow;

    return (
        <div className="rounded-2xl border border-zinc-200 bg-white p-3 shadow-sm transition-shadow active:shadow-md dark:border-white/10 dark:bg-[#141821] dark:shadow-none">
            {/* ─── 池子头部 ─── */}
            <div className="flex items-center justify-between gap-2">
                <div className="flex min-w-0 items-center gap-1.5">
                    <span className="inline-flex h-5 min-w-[22px] shrink-0 items-center justify-center rounded-md bg-emerald-500/15 px-1 text-[10px] font-bold tabular-nums text-emerald-700 dark:bg-emerald-500/10 dark:text-emerald-300">
                        #{rank}
                    </span>
                    <span className="truncate text-[13px] font-bold text-zinc-900 dark:text-white/90">
                        {pair || shortHex(poolId, 10, 6) || '--'}
                    </span>
                    <span className="shrink-0 rounded bg-zinc-100 px-1 py-0.5 text-[9px] font-semibold text-zinc-600 dark:bg-white/10 dark:text-white/60">
                        {version || '--'}
                    </span>
                    {Number.isFinite(feePct) && feePct > 0 ? (
                        <span className="shrink-0 rounded bg-zinc-100 px-1 py-0.5 text-[9px] font-semibold text-zinc-600 dark:bg-white/10 dark:text-white/60">
                            {formatPct(feePct)}
                        </span>
                    ) : null}
                </div>
                <div className="shrink-0 flex items-center gap-1.5">
                    {previewTotalUsd > 0 ? (
                        <span className="rounded-lg bg-amber-500/10 px-1.5 py-0.5 text-[10px] font-semibold tabular-nums text-amber-700 dark:bg-amber-500/15 dark:text-amber-300">
                            ${formatUsdPlain(previewTotalUsd)}
                        </span>
                    ) : null}
                    <span className="rounded-lg bg-emerald-500/10 px-1.5 py-0.5 text-[10px] font-semibold tabular-nums text-emerald-700 dark:bg-emerald-500/15 dark:text-emerald-300">
                        {Number.isFinite(walletCount) ? walletCount : 0} 钱包
                    </span>
                </div>
            </div>

            {/* ─── 钱包明细 ─── */}
            <div className="mt-2 space-y-1.5">
                {/* Loading skeleton */}
                {previewStatus === 'loading' && previewWallets.length === 0 ? (
                    <div className="space-y-1.5">
                        <div className="h-12 animate-shimmer rounded-xl" />
                        <div className="h-12 animate-shimmer rounded-xl" />
                    </div>
                ) : null}

                {/* Error */}
                {previewStatus === 'error' ? (
                    <div className="rounded-xl border border-red-500/30 bg-red-500/10 px-3 py-2 text-[10px] text-red-700 dark:border-red-500/20 dark:bg-red-500/5 dark:text-red-200">
                        {previewError || '加载失败'}
                        <button
                            type="button"
                            onClick={onLoadPreview}
                            className="ml-2 underline hover:no-underline"
                        >
                            重试
                        </button>
                    </div>
                ) : null}

                {/* Not loaded yet (on-demand) */}
                {hasNoPreview && previewStatus !== 'error' ? (
                    <button
                        type="button"
                        onClick={onLoadPreview}
                        className="w-full rounded-xl border border-dashed border-zinc-300 bg-zinc-50/50 py-2.5 text-[11px] font-medium text-zinc-500 transition hover:border-emerald-400 hover:text-emerald-600 dark:border-white/15 dark:bg-white/5 dark:text-white/50 dark:hover:border-emerald-400 dark:hover:text-emerald-300"
                    >
                        点击加载钱包明细
                    </button>
                ) : null}

                {/* Wallet rows — new 2-row layout */}
                {displayWallets.map((row, rowIdx) => {
                    const walletAddr = String(row?.wallet_address || '').trim();
                    const amountUsd = Number(row?.total_usd ?? 0);
                    const sharePct = Number(row?._share_pct ?? 0);
                    const priceLower = Number(row?.price_lower ?? 0);
                    const priceUpper = Number(row?.price_upper ?? 0);
                    const quote = String(row?.price_quote || '').trim();
                    const hasRange = Number.isFinite(priceLower) && priceLower > 0 && Number.isFinite(priceUpper) && priceUpper > 0;

                    return (
                        <div
                            key={`${walletAddr || rowIdx}-${row?._rank || rowIdx}`}
                            className="rounded-xl border border-zinc-200/80 bg-zinc-50/60 p-2 dark:border-white/10 dark:bg-white/[0.03]"
                        >
                            {/* Row 1: wallet + amount + share */}
                            <div className="flex items-center justify-between gap-2">
                                <button
                                    type="button"
                                    onClick={() => onCopyWallet(walletAddr)}
                                    className="font-mono text-[11px] font-semibold text-zinc-800 hover:text-emerald-600 dark:text-white/85 dark:hover:text-emerald-300"
                                    title={walletAddr}
                                >
                                    {shortHex(walletAddr, 8, 6) || '--'}
                                </button>
                                <div className="flex items-center gap-2">
                                    <span className="text-[11px] font-bold tabular-nums text-zinc-900 dark:text-white/85">
                                        ${Number.isFinite(amountUsd) ? formatUsdPlain(amountUsd) : '--'}
                                    </span>
                                    {hasRange ? (
                                        <span className="min-w-[36px] text-right text-[10px] font-semibold tabular-nums text-emerald-600 dark:text-emerald-300">
                                            {formatPct(Math.abs(priceUpper - priceLower) / (priceUpper + priceLower) * 100, 1)}
                                        </span>
                                    ) : null}
                                </div>
                            </div>
                            {/* Row 2: range — full width, never truncated */}
                            {hasRange ? (
                                <div className="mt-1 flex items-center gap-1 text-[10px]">
                                    <span className="shrink-0 text-zinc-400 dark:text-white/30">区间</span>
                                    <span className="font-mono tabular-nums text-zinc-600 dark:text-white/55">
                                        {compactPrice(priceLower)}
                                    </span>
                                    <span className="text-zinc-400 dark:text-white/30">→</span>
                                    <span className="font-mono tabular-nums text-zinc-600 dark:text-white/55">
                                        {compactPrice(priceUpper)}
                                    </span>
                                    {quote ? (
                                        <span className="text-zinc-400 dark:text-white/25">{quote}</span>
                                    ) : null}
                                </div>
                            ) : null}
                        </div>
                    );
                })}

                {/* Empty state */}
                {previewWallets.length === 0 && previewStatus === 'success' ? (
                    <div className="rounded-xl border border-zinc-200/80 bg-zinc-50/50 px-3 py-2 text-[10px] text-zinc-500 dark:border-white/10 dark:bg-white/5 dark:text-white/55">
                        暂无参与钱包明细
                    </div>
                ) : null}

                {/* Expand/collapse toggle */}
                {needsExpand ? (
                    <button
                        type="button"
                        onClick={() => setExpanded((v) => !v)}
                        className="w-full rounded-lg py-1 text-[10px] font-semibold text-zinc-500 hover:text-emerald-600 dark:text-white/40 dark:hover:text-emerald-300"
                    >
                        {expanded ? '收起' : `展开全部 (还有 ${hiddenCount} 个)`}
                    </button>
                ) : null}
            </div>

            {/* ─── 底部操作 ─── */}
            <div className="mt-2 flex items-center justify-end gap-1.5">
                <button
                    type="button"
                    onClick={() => {
                        hapticImpact('light');
                        if (onQuickOpen) onQuickOpen(previewWallets);
                    }}
                    disabled={!poolId || !version}
                    className="inline-flex items-center rounded-lg bg-blue-500/15 px-2.5 py-1 text-[10px] font-bold text-blue-600 ring-1 ring-blue-500/25 hover:bg-blue-500/25 disabled:opacity-40 dark:bg-blue-500/20 dark:text-blue-300 dark:ring-blue-500/30"
                >
                    快速开单
                </button>
                <button
                    type="button"
                    onClick={onCopyPoolId}
                    disabled={!poolId}
                    className="inline-flex items-center rounded-lg bg-amber-300 px-2.5 py-1 text-[10px] font-semibold text-amber-950 hover:bg-amber-200 disabled:opacity-40 dark:bg-amber-300 dark:text-amber-950 dark:hover:bg-amber-200"
                >
                    复制池子ID
                </button>
                <button
                    type="button"
                    onClick={onViewDetails}
                    disabled={!poolId || !version}
                    className="inline-flex items-center rounded-lg bg-emerald-500 px-2.5 py-1 text-[10px] font-semibold text-white hover:bg-emerald-600 disabled:opacity-40"
                >
                    查看明细
                </button>
            </div>
        </div>
    );
}

export default function SmartMoneyCard({ overview, loading = false, tick, onNotice, apiBaseUrl, initData, onQuickOpenPool }) {
    const pools = Array.isArray(overview?.pools) ? overview.pools : [];
    const warnings = Array.isArray(overview?.warnings) ? overview.warnings : [];
    const poolWindowLabel = formatWindowLabel(overview?.pools_window_sec) || '2h';
    const chain = String(overview?.chain || 'bsc').trim() || 'bsc';
    const poolsWindowHours = useMemo(() => {
        const sec = Number(overview?.pools_window_sec ?? 0);
        if (!Number.isFinite(sec) || sec <= 0) return 2;
        const h = sec / 3600;
        if (h <= 0) return 2;
        return Math.max(1, Math.min(168, Math.round(h)));
    }, [overview?.pools_window_sec]);
    const pnlWindowHours = useMemo(() => {
        const sec = Number(overview?.pnl_window_sec ?? 0);
        if (!Number.isFinite(sec) || sec <= 0) return 24;
        const h = sec / 3600;
        if (h <= 0) return 24;
        return Math.max(1, Math.min(168, Math.round(h)));
    }, [overview?.pnl_window_sec]);

    const [activeTab, setActiveTab] = useState('overview');
    const [walletModalOpen, setWalletModalOpen] = useState(false);
    const [walletModalAddr, setWalletModalAddr] = useState('');
    const [followModalOpen, setFollowModalOpen] = useState(false);
    const [followModalAddr, setFollowModalAddr] = useState('');
    const [poolAddsOpen, setPoolAddsOpen] = useState(false);
    const [poolAddsPoolVersion, setPoolAddsPoolVersion] = useState('');
    const [poolAddsPoolId, setPoolAddsPoolId] = useState('');
    const [customWalletAddr, setCustomWalletAddr] = useState('');
    const [customWalletErr, setCustomWalletErr] = useState('');
    const [followConfigsLoading, setFollowConfigsLoading] = useState(false);
    const [followConfigsError, setFollowConfigsError] = useState('');
    const [followConfigs, setFollowConfigs] = useState([]);
    const [followConfigNonce, setFollowConfigNonce] = useState(0);

    const [goldenLoading, setGoldenLoading] = useState(false);
    const [goldenSaving, setGoldenSaving] = useState(false);
    const [goldenError, setGoldenError] = useState('');
    const [goldenNonce, setGoldenNonce] = useState(0);

    const [goldenEnabled, setGoldenEnabled] = useState(false);
    const [goldenMinWallets, setGoldenMinWallets] = useState('3');
    const [goldenWindowMinutes, setGoldenWindowMinutes] = useState('10');
    const [goldenCooldownMinutes, setGoldenCooldownMinutes] = useState('30');
    const [poolAddsPreviewMap, setPoolAddsPreviewMap] = useState({});
    const poolAddsPreviewRef = useRef({});

    const updatedAtText = useMemo(
        () => formatRelativeTime(overview?.updated_at, tick) || '--',
        [overview?.updated_at, tick],
    );
    const topPools = useMemo(() => pools.slice(0, 20), [pools]);
    const preloadPools = useMemo(() => pools.slice(0, 3), [pools]);

    const enabledFollowWallets = useMemo(() => {
        if (!Array.isArray(followConfigs)) return [];
        return followConfigs
            .filter((cfg) => Boolean(cfg?.enabled))
            .map((cfg) => {
                const walletAddress = normalizeWalletAddress(cfg?.wallet_address);
                if (!walletAddress) return null;
                return {
                    wallet_address: walletAddress,
                    per_trade_amount_usdt: Number(cfg?.per_trade_amount_usdt ?? 0),
                    max_total_amount_usdt: Number(cfg?.max_total_amount_usdt ?? 0),
                    delay_min_seconds: Number(cfg?.delay_min_seconds ?? 0),
                    delay_max_seconds: Number(cfg?.delay_max_seconds ?? 60),
                    updated_at: cfg?.updated_at,
                };
            })
            .filter(Boolean)
            .sort((a, b) => {
                const ta = Date.parse(String(a?.updated_at || '')) || 0;
                const tb = Date.parse(String(b?.updated_at || '')) || 0;
                return tb - ta;
            });
    }, [followConfigs]);

    useEffect(() => {
        poolAddsPreviewRef.current = poolAddsPreviewMap || {};
    }, [poolAddsPreviewMap]);

    useEffect(() => {
        setPoolAddsPreviewMap({});
    }, [chain, initData]);

    useEffect(() => {
        if (activeTab !== 'overview') return;
        if (!initData) return;
        if (!Array.isArray(preloadPools) || preloadPools.length === 0) return;

        const now = Date.now();
        const previewCache = poolAddsPreviewRef.current || {};
        const targets = preloadPools
            .map((pool) => {
                const poolVersion = String(pool?.pool_version || '').trim().toLowerCase();
                const poolId = String(pool?.pool_id || '').trim();
                if (!poolVersion || !poolId) return null;
                if (poolVersion !== 'v3' && poolVersion !== 'v4') return null;
                const walletCount = Number(pool?.wallet_count ?? 0);
                const limit = Number.isFinite(walletCount) && walletCount > 0
                    ? Math.max(20, Math.min(200, Math.round(walletCount)))
                    : 60;
                return {
                    key: `${poolVersion}:${poolId}`,
                    poolVersion,
                    poolId,
                    limit,
                };
            })
            .filter(Boolean)
            .filter((item) => {
                const cached = previewCache[item.key];
                if (!cached) return true;
                const fetchedAt = Number(cached?.fetchedAt ?? 0);
                if (cached?.status === 'loading') return false;
                if (cached?.status === 'error' && now - fetchedAt < 20_000) return false;
                if (cached?.status === 'success' && now - fetchedAt < 45_000) return false;
                return true;
            });

        if (targets.length === 0) return;

        let aborted = false;
        const controller = new AbortController();

        setPoolAddsPreviewMap((prev) => {
            const next = { ...(prev || {}) };
            targets.forEach((item) => {
                const prevItem = next[item.key] || {};
                next[item.key] = {
                    ...prevItem,
                    status: 'loading',
                    error: '',
                    fetchedAt: Number(prevItem?.fetchedAt ?? 0),
                    wallets: Array.isArray(prevItem?.wallets) ? prevItem.wallets : [],
                    totalUsd: Number(prevItem?.totalUsd ?? 0),
                };
            });
            return next;
        });

        let cursor = 0;
        const worker = async () => {
            while (!aborted) {
                const task = targets[cursor];
                cursor += 1;
                if (!task) break;

                try {
                    const resp = await fetchSmartMoneyPoolAdds({
                        apiBaseUrl,
                        initData,
                        chain,
                        poolVersion: task.poolVersion,
                        poolId: task.poolId,
                        windowHours: poolsWindowHours,
                        limit: task.limit,
                        feesLimit: 0,
                        signal: controller.signal,
                    });
                    if (aborted) return;

                    const rawWallets = Array.isArray(resp?.wallets) ? resp.wallets : [];
                    const totalUsd = rawWallets.reduce((acc, row) => {
                        const n = Number(row?.total_usd ?? 0);
                        if (!Number.isFinite(n) || n <= 0) return acc;
                        return acc + n;
                    }, 0);

                    const wallets = rawWallets.map((row, rowIndex) => {
                        const amountUsd = Number(row?.total_usd ?? 0);
                        const sharePct = totalUsd > 0 && Number.isFinite(amountUsd) ? (amountUsd / totalUsd) * 100 : 0;
                        return {
                            ...row,
                            _rank: rowIndex + 1,
                            _share_pct: sharePct,
                        };
                    });

                    setPoolAddsPreviewMap((prev) => ({
                        ...(prev || {}),
                        [task.key]: {
                            status: 'success',
                            error: '',
                            fetchedAt: Date.now(),
                            wallets,
                            totalUsd,
                            warnings: Array.isArray(resp?.warnings) ? resp.warnings : [],
                        },
                    }));
                } catch (e) {
                    if (aborted) return;
                    setPoolAddsPreviewMap((prev) => {
                        const prevItem = (prev || {})[task.key] || {};
                        return {
                            ...(prev || {}),
                            [task.key]: {
                                ...prevItem,
                                status: 'error',
                                error: String(e?.message || e),
                                fetchedAt: Date.now(),
                                wallets: Array.isArray(prevItem?.wallets) ? prevItem.wallets : [],
                                totalUsd: Number(prevItem?.totalUsd ?? 0),
                            },
                        };
                    });
                }
            }
        };

        const workers = Array.from({ length: Math.min(3, targets.length) }, () => worker());
        Promise.all(workers).catch(() => { });

        return () => {
            aborted = true;
            controller.abort();
        };
    }, [activeTab, apiBaseUrl, initData, chain, poolsWindowHours, preloadPools]);

    useEffect(() => {
        if (!initData) {
            setFollowConfigs([]);
            setFollowConfigsError('');
            setFollowConfigsLoading(false);
            return;
        }

        let aborted = false;
        const controller = new AbortController();

        setFollowConfigsLoading(true);
        setFollowConfigsError('');

        fetchSmartMoneyFollowConfigs({
            apiBaseUrl,
            initData,
            chain,
            enabledOnly: false,
            limit: 200,
            signal: controller.signal,
        })
            .then((resp) => {
                if (aborted) return;
                const incoming = Array.isArray(resp?.configs) ? resp.configs : [];
                setFollowConfigs((prev) => {
                    const prevList = Array.isArray(prev) ? prev : [];
                    if (incoming.length > 0) return incoming;
                    const prevEnabled = prevList.filter((cfg) => Boolean(cfg?.enabled));
                    return prevEnabled.length > 0 ? prevList : [];
                });
            })
            .catch((e) => {
                if (aborted) return;
                setFollowConfigs([]);
                setFollowConfigsError(String(e?.message || e));
            })
            .finally(() => {
                if (aborted) return;
                setFollowConfigsLoading(false);
            });

        return () => {
            aborted = true;
            controller.abort();
        };
    }, [apiBaseUrl, initData, chain, followConfigNonce]);

    useEffect(() => {
        if (activeTab !== 'golden') return;
        if (!initData) {
            setGoldenError('');
            setGoldenLoading(false);
            return;
        }

        let aborted = false;
        const controller = new AbortController();

        setGoldenLoading(true);
        setGoldenError('');

        fetchSmartMoneyGoldenDogConfig({
            apiBaseUrl,
            initData,
            chain,
            signal: controller.signal,
        })
            .then((resp) => {
                if (aborted) return;
                const cfg = resp?.config || null;
                if (!cfg) return;
                setGoldenEnabled(Boolean(cfg?.enabled));
                setGoldenMinWallets(String(cfg?.min_wallets ?? 3));
                setGoldenWindowMinutes(String(cfg?.window_minutes ?? 10));
                setGoldenCooldownMinutes(String(cfg?.cooldown_minutes ?? 30));
            })
            .catch((e) => {
                if (aborted) return;
                setGoldenError(String(e?.message || e));
            })
            .finally(() => {
                if (aborted) return;
                setGoldenLoading(false);
            });

        return () => {
            aborted = true;
            controller.abort();
        };
    }, [activeTab, apiBaseUrl, initData, chain, goldenNonce]);

    const subtitle = `最近${poolWindowLabel}池子 ${pools.length} 个 · 更新 ${updatedAtText}`;

    async function handleSaveGoldenDog() {
        if (!initData) {
            hapticNotification('error');
            setGoldenError('未获取到 initData，无法保存金狗通知配置');
            if (typeof onNotice === 'function') onNotice('未获取到 initData，无法保存金狗通知配置', 'error');
            return;
        }
        const minWallets = toIntInRange(goldenMinWallets, 2, 100, 3);
        const windowMinutes = toIntInRange(goldenWindowMinutes, 1, 180, 10);
        const cooldownMinutes = toIntInRange(goldenCooldownMinutes, 0, 1440, 30);

        setGoldenSaving(true);
        setGoldenError('');
        try {
            const resp = await saveSmartMoneyGoldenDogConfig({
                apiBaseUrl,
                initData,
                chain,
                enabled: Boolean(goldenEnabled),
                minWallets,
                windowMinutes,
                cooldownMinutes,
            });
            const cfg = resp?.config || null;
            if (cfg) {
                setGoldenEnabled(Boolean(cfg?.enabled));
                setGoldenMinWallets(String(cfg?.min_wallets ?? minWallets));
                setGoldenWindowMinutes(String(cfg?.window_minutes ?? windowMinutes));
                setGoldenCooldownMinutes(String(cfg?.cooldown_minutes ?? cooldownMinutes));
            }
            hapticNotification('success');
            if (typeof onNotice === 'function') onNotice('已保存金狗通知配置', 'success');
            setGoldenNonce((v) => v + 1);
        } catch (e) {
            hapticNotification('error');
            setGoldenError(String(e?.message || e));
            if (typeof onNotice === 'function') onNotice(`保存失败: ${String(e?.message || e)}`, 'error');
        } finally {
            setGoldenSaving(false);
        }
    }

    return (
        <ModuleHeader
            title={(
                <>
                    Smart Money
                    {loading ? (
                        <span className="ml-2 inline-flex items-center rounded-lg bg-zinc-100 px-2 py-0.5 text-[10px] font-semibold text-zinc-600 dark:bg-white/5 dark:text-white/60">
                            加载中...
                        </span>
                    ) : null}
                </>
            )}
            subtitle={subtitle}
            className="mt-0"
        >
            {warnings.length ? (
                <div className="mt-3 rounded-xl border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-xs text-amber-700 dark:text-amber-200">
                    <div className="font-semibold">提示</div>
                    <ul className="mt-1 list-disc space-y-1 pl-4">
                        {warnings.slice(0, 4).map((w, i) => (
                            <li key={String(i)}>{String(w)}</li>
                        ))}
                    </ul>
                </div>
            ) : null}

            <div className="mt-3 inline-flex rounded-xl border border-zinc-200 bg-zinc-50 p-1 dark:border-white/10 dark:bg-[#0f1116]">
                <button
                    type="button"
                    onClick={() => setActiveTab('overview')}
                    className={`rounded-lg px-3 py-1.5 text-[11px] font-semibold transition ${activeTab === 'overview'
                        ? 'bg-emerald-500 text-white'
                        : 'text-zinc-600 hover:bg-zinc-100 dark:text-white/70 dark:hover:bg-white/10'
                        }`}
                >
                    概览
                </button>
                <button
                    type="button"
                    onClick={() => setActiveTab('follow')}
                    className={`rounded-lg px-3 py-1.5 text-[11px] font-semibold transition ${activeTab === 'follow'
                        ? 'bg-emerald-500 text-white'
                        : 'text-zinc-600 hover:bg-zinc-100 dark:text-white/70 dark:hover:bg-white/10'
                        }`}
                >
                    跟单
                </button>
                <button
                    type="button"
                    onClick={() => setActiveTab('golden')}
                    className={`rounded-lg px-3 py-1.5 text-[11px] font-semibold transition ${activeTab === 'golden'
                        ? 'bg-emerald-500 text-white'
                        : 'text-zinc-600 hover:bg-zinc-100 dark:text-white/70 dark:hover:bg-white/10'
                        }`}
                >
                    金狗通知
                </button>
                <button
                    type="button"
                    onClick={() => setActiveTab('monitor')}
                    className={`rounded-lg px-3 py-1.5 text-[11px] font-semibold transition ${activeTab === 'monitor'
                        ? 'bg-emerald-500 text-white'
                        : 'text-zinc-600 hover:bg-zinc-100 dark:text-white/70 dark:hover:bg-white/10'
                        }`}
                >
                    监控
                </button>
                <button
                    type="button"
                    onClick={() => setActiveTab('24h')}
                    className={`rounded-lg px-3 py-1.5 text-[11px] font-semibold transition ${activeTab === '24h'
                        ? 'bg-emerald-500 text-white'
                        : 'text-zinc-600 hover:bg-zinc-100 dark:text-white/70 dark:hover:bg-white/10'
                        }`}
                >
                    24h加池
                </button>
            </div>

            {activeTab === 'overview' ? (
                <div className="mt-3 space-y-3">
                    <div className="flex items-center justify-between">
                        <div className="text-xs font-semibold text-zinc-700 dark:text-white/80">最近{poolWindowLabel}参与池子</div>
                        <span className="text-[11px] text-zinc-500 dark:text-white/40">{topPools.length} 个</span>
                    </div>

                    {topPools.length ? (
                        <div className="space-y-3">
                            {topPools.map((pool, index) => {
                                const poolId = String(pool?.pool_id || '').trim();
                                const pair = String(pool?.pair || '').trim();
                                const version = String(pool?.pool_version || '').trim().toUpperCase();
                                const poolVersionLower = String(pool?.pool_version || '').trim().toLowerCase();
                                const feePct = Number(pool?.fee_pct);
                                const walletCount = Number(pool?.wallet_count ?? 0);
                                const rank = index + 1;
                                const key = `${version || 'POOL'}:${poolId || rank}`;
                                const previewKey = `${poolVersionLower}:${poolId}`;
                                const preview = poolAddsPreviewMap[previewKey] || null;
                                const previewStatus = String(preview?.status || '');
                                const previewWallets = Array.isArray(preview?.wallets) ? preview.wallets : [];
                                const previewError = String(preview?.error || '').trim();
                                const previewTotalUsd = Number(preview?.totalUsd ?? 0);
                                const hasNoPreview = !preview && previewStatus !== 'loading';
                                const DEFAULT_SHOW = 3;
                                const needsExpand = previewWallets.length > DEFAULT_SHOW;

                                return (
                                    <PoolOverviewCard
                                        key={key}
                                        rank={rank}
                                        pair={pair}
                                        poolId={poolId}
                                        version={version}
                                        feePct={feePct}
                                        walletCount={walletCount}
                                        previewStatus={previewStatus}
                                        previewWallets={previewWallets}
                                        previewError={previewError}
                                        previewTotalUsd={previewTotalUsd}
                                        hasNoPreview={hasNoPreview}
                                        defaultShow={DEFAULT_SHOW}
                                        needsExpand={needsExpand}
                                        onCopyPoolId={() => {
                                            hapticImpact('light');
                                            safeCopy(poolId, onNotice);
                                        }}
                                        onViewDetails={() => {
                                            const pv = poolVersionLower;
                                            if (!pv || !poolId) return;
                                            hapticImpact('light');
                                            setPoolAddsPoolVersion(pv);
                                            setPoolAddsPoolId(poolId);
                                            setPoolAddsOpen(true);
                                        }}
                                        onCopyWallet={(addr) => {
                                            hapticImpact('light');
                                            safeCopy(addr, onNotice);
                                        }}
                                        onQuickOpen={(wList) => {
                                            if (typeof onQuickOpenPool === 'function') {
                                                onQuickOpenPool({
                                                    pool_address: poolId,
                                                    pool_id: poolId,
                                                    protocol_version: version,
                                                    trading_pair: pair,
                                                    chain,
                                                    smartMoneyWallets: wList || previewWallets, // Pass down the smart money data
                                                });
                                            }
                                        }}
                                        onLoadPreview={() => {
                                            // On-demand load for pools that weren't preloaded
                                            if (previewStatus === 'loading') return;
                                            const controller = new AbortController();
                                            setPoolAddsPreviewMap((prev) => ({
                                                ...(prev || {}),
                                                [previewKey]: {
                                                    ...(prev?.[previewKey] || {}),
                                                    status: 'loading',
                                                    error: '',
                                                    fetchedAt: Date.now(),
                                                },
                                            }));
                                            const limit = Number.isFinite(walletCount) && walletCount > 0
                                                ? Math.max(20, Math.min(200, Math.round(walletCount)))
                                                : 60;
                                            fetchSmartMoneyPoolAdds({
                                                apiBaseUrl,
                                                initData,
                                                chain,
                                                poolVersion: poolVersionLower,
                                                poolId,
                                                windowHours: poolsWindowHours,
                                                limit,
                                                feesLimit: 0,
                                                signal: controller.signal,
                                            })
                                                .then((resp) => {
                                                    const rawWallets = Array.isArray(resp?.wallets) ? resp.wallets : [];
                                                    const totalUsd = rawWallets.reduce((acc, row) => {
                                                        const n = Number(row?.total_usd ?? 0);
                                                        return Number.isFinite(n) && n > 0 ? acc + n : acc;
                                                    }, 0);
                                                    const wallets = rawWallets.map((row, ri) => ({
                                                        ...row,
                                                        _rank: ri + 1,
                                                        _share_pct: totalUsd > 0 ? ((Number(row?.total_usd ?? 0) || 0) / totalUsd) * 100 : 0,
                                                    }));
                                                    setPoolAddsPreviewMap((prev) => ({
                                                        ...(prev || {}),
                                                        [previewKey]: { status: 'success', error: '', fetchedAt: Date.now(), wallets, totalUsd },
                                                    }));
                                                })
                                                .catch((e) => {
                                                    setPoolAddsPreviewMap((prev) => ({
                                                        ...(prev || {}),
                                                        [previewKey]: {
                                                            ...(prev?.[previewKey] || {}),
                                                            status: 'error',
                                                            error: String(e?.message || e),
                                                            fetchedAt: Date.now(),
                                                        },
                                                    }));
                                                });
                                        }}
                                    />
                                );
                            })}
                        </div>
                    ) : (
                        <div className="rounded-xl border border-zinc-200 bg-white p-3 text-[11px] text-zinc-500 dark:border-white/10 dark:bg-[#131821] dark:text-white/60">
                            暂无池子数据
                        </div>
                    )}
                </div>
            ) : activeTab === 'follow' ? (
                <div className="mt-3 space-y-3">
                    <div className="rounded-xl border border-zinc-200 bg-zinc-50 p-3 dark:border-white/10 dark:bg-[#0f1116]">
                        <div className="text-xs font-semibold text-zinc-700 dark:text-white/80">自定义跟单钱包</div>
                        <div className="mt-1 flex items-center gap-2">
                            <input
                                type="text"
                                inputMode="text"
                                autoComplete="off"
                                value={customWalletAddr}
                                onChange={(e) => {
                                    setCustomWalletAddr(e.target.value);
                                    if (customWalletErr) setCustomWalletErr('');
                                }}
                                onKeyDown={(e) => {
                                    if (e.key !== 'Enter') return;
                                    const normalized = normalizeWalletAddress(customWalletAddr);
                                    if (!normalized) {
                                        hapticNotification('error');
                                        setCustomWalletErr('Invalid wallet address (expected 0x...)');
                                        return;
                                    }
                                    hapticImpact('light');
                                    if (customWalletErr) setCustomWalletErr('');
                                    setCustomWalletAddr(normalized);
                                    setFollowModalAddr(normalized);
                                    setFollowModalOpen(true);
                                }}
                                className="min-w-0 flex-1 rounded-lg bg-white px-2 py-1 text-[12px] font-semibold text-zinc-900 outline-none ring-0 dark:bg-white/5 dark:text-white/80"
                                placeholder="0x..."
                            />
                            <button
                                type="button"
                                onClick={() => {
                                    const normalized = normalizeWalletAddress(customWalletAddr);
                                    if (!normalized) {
                                        hapticNotification('error');
                                        setCustomWalletErr('Invalid wallet address (expected 0x...)');
                                        return;
                                    }
                                    hapticImpact('light');
                                    if (customWalletErr) setCustomWalletErr('');
                                    setCustomWalletAddr(normalized);
                                    setFollowModalAddr(normalized);
                                    setFollowModalOpen(true);
                                }}
                                className="shrink-0 inline-flex items-center rounded-lg bg-emerald-500/15 px-2 py-1 text-[10px] font-semibold text-emerald-700 hover:bg-emerald-500/20 dark:bg-emerald-500/10 dark:text-emerald-200 dark:hover:bg-emerald-500/15"
                            >
                                跟单设置
                            </button>
                        </div>
                        {customWalletErr ? (
                            <div className="mt-1 text-[10px] text-red-600 dark:text-red-300">{customWalletErr}</div>
                        ) : null}
                    </div>

                    <div className="rounded-xl border border-zinc-200 bg-zinc-50 p-3 dark:border-white/10 dark:bg-[#0f1116]">
                        <div className="mb-2 flex items-center justify-between">
                            <div className="text-xs font-semibold text-zinc-700 dark:text-white/80">已开启跟单钱包</div>
                            <div className="inline-flex items-center gap-2">
                                <span className="text-[11px] text-zinc-500 dark:text-white/40">
                                    {followConfigsLoading ? '加载中…' : <><NumberFlowValue value={enabledFollowWallets.length} formatOptions={{ maximumFractionDigits: 0 }} /> 个</>}
                                </span>
                                <button
                                    type="button"
                                    onClick={() => {
                                        hapticImpact('light');
                                        setFollowConfigNonce((v) => v + 1);
                                    }}
                                    className="inline-flex items-center rounded-lg bg-zinc-100 px-2 py-1 text-[10px] font-semibold text-zinc-700 hover:bg-zinc-200 dark:bg-white/5 dark:text-white/70 dark:hover:bg-white/10"
                                >
                                    刷新
                                </button>
                            </div>
                        </div>

                        {followConfigsError ? (
                            <div className="mt-1 text-[10px] text-red-600 dark:text-red-300">{followConfigsError}</div>
                        ) : null}

                        {!followConfigsError && enabledFollowWallets.length === 0 ? (
                            <div className="mt-1 text-[11px] text-zinc-500 dark:text-white/50">当前没有开启跟单的钱包</div>
                        ) : null}

                        {enabledFollowWallets.length ? (
                            <div className="mt-1 space-y-1.5">
                                {enabledFollowWallets.map((cfg) => {
                                    const wallet = String(cfg?.wallet_address || '');
                                    const perTrade = Number(cfg?.per_trade_amount_usdt ?? 0);
                                    const maxTotal = Number(cfg?.max_total_amount_usdt ?? 0);
                                    const dMin = Number(cfg?.delay_min_seconds ?? 0);
                                    const dMax = Number(cfg?.delay_max_seconds ?? 60);
                                    return (
                                        <div key={wallet} className="flex items-center justify-between gap-2 rounded-lg border border-zinc-200 bg-white/40 backdrop-blur-md p-2 dark:border-white/10 dark:bg-white/5">
                                            <div className="min-w-0">
                                                <div className="truncate text-[11px] font-semibold text-zinc-800 dark:text-white/85">{shortHex(wallet, 10, 8)}</div>
                                                <div className="text-[10px] text-zinc-500 dark:text-white/40">
                                                    单次 <NumberFlowValue value={perTrade} formatter={(v) => formatUsd(v)} /> · 总额 <NumberFlowValue value={maxTotal} formatter={(v) => formatUsd(v)} /> · 延迟 <NumberFlowValue value={Number.isFinite(dMin) ? dMin : 0} formatOptions={{ maximumFractionDigits: 0 }} />-<NumberFlowValue value={Number.isFinite(dMax) ? dMax : 60} formatOptions={{ maximumFractionDigits: 0 }} />s
                                                </div>
                                            </div>
                                            <div className="shrink-0 inline-flex items-center gap-1.5">
                                                <button
                                                    type="button"
                                                    onClick={() => {
                                                        hapticImpact('light');
                                                        safeCopy(wallet, onNotice);
                                                    }}
                                                    className="inline-flex items-center rounded-lg bg-zinc-100 px-2 py-1 text-[10px] font-semibold text-zinc-700 hover:bg-zinc-200 dark:bg-white/5 dark:text-white/70 dark:hover:bg-white/10"
                                                >
                                                    复制
                                                </button>
                                                <button
                                                    type="button"
                                                    onClick={() => {
                                                        hapticImpact('light');
                                                        setFollowModalAddr(wallet);
                                                        setFollowModalOpen(true);
                                                    }}
                                                    className="inline-flex items-center rounded-lg bg-emerald-500/15 px-2 py-1 text-[10px] font-semibold text-emerald-700 hover:bg-emerald-500/20 dark:bg-emerald-500/10 dark:text-emerald-200 dark:hover:bg-emerald-500/15"
                                                >
                                                    设置
                                                </button>
                                                <button
                                                    type="button"
                                                    onClick={() => {
                                                        hapticImpact('light');
                                                        setWalletModalAddr(wallet);
                                                        setWalletModalOpen(true);
                                                    }}
                                                    className="inline-flex items-center rounded-lg bg-zinc-100 px-2 py-1 text-[10px] font-semibold text-zinc-700 hover:bg-zinc-200 dark:bg-white/5 dark:text-white/70 dark:hover:bg-white/10"
                                                >
                                                    仓位
                                                </button>
                                            </div>
                                        </div>
                                    );
                                })}
                            </div>
                        ) : null}
                    </div>
                </div>
            ) : (
                <div className="mt-3 space-y-3">
                    <div className="rounded-xl border border-zinc-200 bg-zinc-50 p-3 dark:border-white/10 dark:bg-[#0f1116]">
                        <div className="flex items-center justify-between gap-2">
                            <div className="text-xs font-semibold text-zinc-700 dark:text-white/80">金狗通知</div>
                            <div className="inline-flex items-center gap-2">
                                <button
                                    type="button"
                                    onClick={() => {
                                        hapticImpact('light');
                                        setGoldenEnabled((v) => !v);
                                    }}
                                    disabled={goldenSaving || goldenLoading}
                                    className={`inline-flex items-center rounded-lg px-2 py-1 text-[10px] font-semibold transition ${goldenEnabled
                                        ? 'bg-emerald-500/15 text-emerald-700 hover:bg-emerald-500/20 dark:bg-emerald-500/10 dark:text-emerald-200 dark:hover:bg-emerald-500/15'
                                        : 'bg-zinc-100 text-zinc-700 hover:bg-zinc-200 dark:bg-white/5 dark:text-white/70 dark:hover:bg-white/10'
                                        }`}
                                >
                                    {goldenEnabled ? '已启用' : '已停用'}
                                </button>
                                <button
                                    type="button"
                                    onClick={() => {
                                        hapticImpact('light');
                                        setGoldenNonce((v) => v + 1);
                                    }}
                                    disabled={goldenSaving || goldenLoading}
                                    className="inline-flex items-center rounded-lg bg-zinc-100 px-2 py-1 text-[10px] font-semibold text-zinc-700 hover:bg-zinc-200 disabled:opacity-60 dark:bg-white/5 dark:text-white/70 dark:hover:bg-white/10"
                                >
                                    刷新
                                </button>
                            </div>
                        </div>

                        <div className="mt-2 grid grid-cols-1 gap-2 sm:grid-cols-3">
                            <label className="rounded-xl border border-zinc-200 bg-white/40 backdrop-blur-md p-2 dark:border-white/10 dark:bg-white/5">
                                <div className="text-[10px] text-zinc-500 dark:text-white/40">触发钱包数</div>
                                <input
                                    type="number"
                                    inputMode="numeric"
                                    min="2"
                                    max="100"
                                    step="1"
                                    value={goldenMinWallets}
                                    onChange={(e) => setGoldenMinWallets(e.target.value)}
                                    disabled={goldenSaving || goldenLoading}
                                    className="mt-1 w-full rounded-lg bg-white px-2 py-1 text-[12px] font-semibold tabular-nums text-zinc-900 outline-none ring-0 dark:bg-white/5 dark:text-white/80"
                                    placeholder="3"
                                />
                            </label>
                            <label className="rounded-xl border border-zinc-200 bg-white/40 backdrop-blur-md p-2 dark:border-white/10 dark:bg-white/5">
                                <div className="text-[10px] text-zinc-500 dark:text-white/40">时间窗口(分钟)</div>
                                <input
                                    type="number"
                                    inputMode="numeric"
                                    min="1"
                                    max="180"
                                    step="1"
                                    value={goldenWindowMinutes}
                                    onChange={(e) => setGoldenWindowMinutes(e.target.value)}
                                    disabled={goldenSaving || goldenLoading}
                                    className="mt-1 w-full rounded-lg bg-white px-2 py-1 text-[12px] font-semibold tabular-nums text-zinc-900 outline-none ring-0 dark:bg-white/5 dark:text-white/80"
                                    placeholder="10"
                                />
                            </label>
                            <label className="rounded-xl border border-zinc-200 bg-white/40 backdrop-blur-md p-2 dark:border-white/10 dark:bg-white/5">
                                <div className="text-[10px] text-zinc-500 dark:text-white/40">冷却(分钟)</div>
                                <input
                                    type="number"
                                    inputMode="numeric"
                                    min="0"
                                    max="1440"
                                    step="1"
                                    value={goldenCooldownMinutes}
                                    onChange={(e) => setGoldenCooldownMinutes(e.target.value)}
                                    disabled={goldenSaving || goldenLoading}
                                    className="mt-1 w-full rounded-lg bg-white px-2 py-1 text-[12px] font-semibold tabular-nums text-zinc-900 outline-none ring-0 dark:bg-white/5 dark:text-white/80"
                                    placeholder="30"
                                />
                            </label>
                        </div>

                        <div className="mt-2 text-[10px] text-zinc-500 dark:text-white/40">
                            Bark 通知地址复用全局配置中的 Bark 设置（Key/Server/Group）。
                        </div>

                        {goldenError ? (
                            <div className="mt-2 rounded-xl border border-red-500/30 bg-red-500/10 p-2 text-[11px] text-red-700 dark:border-red-500/20 dark:bg-red-500/5 dark:text-red-200">
                                {goldenError}
                            </div>
                        ) : null}

                        <div className="mt-3 flex items-center justify-end gap-2">
                            <button
                                type="button"
                                onClick={() => {
                                    hapticImpact('light');
                                    handleSaveGoldenDog();
                                }}
                                disabled={goldenSaving || goldenLoading}
                                className="inline-flex items-center rounded-lg bg-emerald-500 px-3 py-1.5 text-[11px] font-semibold text-white hover:bg-emerald-600 disabled:opacity-60 disabled:hover:bg-emerald-500"
                            >
                                {goldenSaving ? '保存中…' : goldenLoading ? '加载中…' : '保存'}
                            </button>
                        </div>
                    </div>
                </div>
            )}

            {activeTab === 'monitor' && (
                <SmartMoneyWatchedWalletsTab
                    apiBaseUrl={apiBaseUrl}
                    initData={initData}
                    chain={chain}
                    onNotice={onNotice}
                />
            )}

            {activeTab === '24h' && (
                <SmartMoney24hPoolAddsCard
                    apiBaseUrl={apiBaseUrl}
                    initData={initData}
                    chain={chain}
                    onNotice={onNotice}
                    onQuickOpenPool={onQuickOpenPool}
                />
            )}

            <SmartMoneyWalletPositionsModal
                open={walletModalOpen}
                onClose={() => {
                    setWalletModalOpen(false);
                    setWalletModalAddr('');
                }}
                apiBaseUrl={apiBaseUrl}
                initData={initData}
                chain={chain}
                walletAddress={walletModalAddr}
                windowHours={pnlWindowHours}
                onNotice={onNotice}
            />

            <SmartMoneyPoolAddsModal
                open={poolAddsOpen}
                onClose={() => {
                    setPoolAddsOpen(false);
                    setPoolAddsPoolVersion('');
                    setPoolAddsPoolId('');
                }}
                apiBaseUrl={apiBaseUrl}
                initData={initData}
                chain={chain}
                poolVersion={poolAddsPoolVersion}
                poolId={poolAddsPoolId}
                windowHours={poolsWindowHours}
                onNotice={onNotice}
                onOpenFollow={(addr) => {
                    const normalized = normalizeWalletAddress(addr);
                    if (!normalized) return;
                    setPoolAddsOpen(false);
                    setFollowModalAddr(normalized);
                    setFollowModalOpen(true);
                }}
                onOpenPositions={(addr) => {
                    const normalized = normalizeWalletAddress(addr);
                    if (!normalized) return;
                    setPoolAddsOpen(false);
                    setWalletModalAddr(normalized);
                    setWalletModalOpen(true);
                }}
            />

            <SmartMoneyFollowModal
                open={followModalOpen}
                onClose={() => {
                    setFollowModalOpen(false);
                    setFollowModalAddr('');
                }}
                apiBaseUrl={apiBaseUrl}
                initData={initData}
                chain={chain}
                walletAddress={followModalAddr}
                onNotice={onNotice}
                onSaved={(savedCfg) => {
                    if (savedCfg && normalizeWalletAddress(savedCfg?.wallet_address)) {
                        const normalizedWallet = normalizeWalletAddress(savedCfg?.wallet_address);
                        const normalizedChain = String(savedCfg?.chain || chain || '').trim().toLowerCase();
                        setFollowConfigs((prev) => {
                            const list = Array.isArray(prev) ? [...prev] : [];
                            const nextCfg = {
                                ...savedCfg,
                                wallet_address: normalizedWallet,
                                chain: normalizedChain,
                                enabled: Boolean(savedCfg?.enabled),
                            };
                            const idx = list.findIndex((it) => {
                                const w = normalizeWalletAddress(it?.wallet_address);
                                const c = String(it?.chain || '').trim().toLowerCase();
                                return w === normalizedWallet && c === normalizedChain;
                            });
                            if (idx >= 0) {
                                list[idx] = { ...list[idx], ...nextCfg };
                            } else {
                                list.unshift(nextCfg);
                            }
                            return list;
                        });
                    }
                    setFollowConfigNonce((v) => v + 1);
                }}
                onOpenPositions={(addr) => {
                    const normalized = normalizeWalletAddress(addr);
                    if (!normalized) return;
                    setWalletModalAddr(normalized);
                    setWalletModalOpen(true);
                }}
            />
        </ModuleHeader>
    );
}
