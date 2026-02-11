import React, { useEffect, useMemo, useState } from 'react';
import { formatRelativeTime } from '../lib/time';
import { fetchSmartMoneyFollowConfigs } from '../lib/api';
import { copyToClipboard, hapticImpact, hapticNotification } from '../lib/telegram';
import ModuleHeader from './ModuleHeader.jsx';
import SmartMoneyFollowModal from './SmartMoneyFollowModal.jsx';
import SmartMoneyWalletPositionsModal from './SmartMoneyWalletPositionsModal.jsx';

const USD_DISPLAY_LIMIT = 1e15;
const usdFormatter = new Intl.NumberFormat('en-US', {
    style: 'currency',
    currency: 'USD',
    maximumFractionDigits: 2,
});

function formatUsd(v) {
    const n = Number(v ?? 0);
    if (!Number.isFinite(n) || Math.abs(n) > USD_DISPLAY_LIMIT) return '$--';
    return usdFormatter.format(n);
}

function formatPct(v, digits = 2) {
    const n = Number(v);
    if (!Number.isFinite(n)) return '--';
    return `${n.toFixed(digits)}%`;
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

function kpiTone(value) {
    const n = Number(value ?? 0);
    if (!Number.isFinite(n)) return 'text-zinc-500 dark:text-white/40';
    if (n > 0) return 'text-emerald-600 dark:text-emerald-300';
    if (n < 0) return 'text-red-600 dark:text-red-300';
    return 'text-zinc-700 dark:text-white/80';
}

export default function SmartMoneyCard({ overview, loading = false, tick, onNotice, apiBaseUrl, initData }) {
    const pools = Array.isArray(overview?.pools) ? overview.pools : [];
    const wallets = Array.isArray(overview?.wallets_24h) ? overview.wallets_24h : [];
    const warnings = Array.isArray(overview?.warnings) ? overview.warnings : [];
    const poolWindowLabel = formatWindowLabel(overview?.pools_window_sec) || '2h';
    const pnlWindowLabel = formatWindowLabel(overview?.pnl_window_sec) || '24h';
    const chain = String(overview?.chain || 'bsc').trim() || 'bsc';
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
    const [customWalletAddr, setCustomWalletAddr] = useState('');
    const [customWalletErr, setCustomWalletErr] = useState('');
    const [followConfigsLoading, setFollowConfigsLoading] = useState(false);
    const [followConfigsError, setFollowConfigsError] = useState('');
    const [followConfigs, setFollowConfigs] = useState([]);
    const [followConfigNonce, setFollowConfigNonce] = useState(0);

    const updatedAtText = useMemo(
        () => formatRelativeTime(overview?.updated_at, tick) || '--',
        [overview?.updated_at, tick],
    );
    const topWallets = useMemo(() => wallets.slice(0, 30), [wallets]);
    const topPools = useMemo(() => pools.slice(0, 20), [pools]);

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
        if (!apiBaseUrl || !initData) {
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
                setFollowConfigs(Array.isArray(resp?.configs) ? resp.configs : []);
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

    const subtitle = `最近${poolWindowLabel}池子 ${pools.length} 个 · 最近${pnlWindowLabel}钱包 ${wallets.length} 个 · 更新 ${updatedAtText}`;

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
                    className={`rounded-lg px-3 py-1.5 text-[11px] font-semibold transition ${
                        activeTab === 'overview'
                            ? 'bg-emerald-500 text-white'
                            : 'text-zinc-600 hover:bg-zinc-100 dark:text-white/70 dark:hover:bg-white/10'
                    }`}
                >
                    概览
                </button>
                <button
                    type="button"
                    onClick={() => setActiveTab('follow')}
                    className={`rounded-lg px-3 py-1.5 text-[11px] font-semibold transition ${
                        activeTab === 'follow'
                            ? 'bg-emerald-500 text-white'
                            : 'text-zinc-600 hover:bg-zinc-100 dark:text-white/70 dark:hover:bg-white/10'
                    }`}
                >
                    跟单
                </button>
            </div>

            {activeTab === 'overview' ? (
                <div className="mt-3 grid grid-cols-1 gap-3 xl:grid-cols-2">
                    <div className="rounded-xl border border-zinc-200 bg-zinc-50 p-3 dark:border-white/10 dark:bg-[#0f1116]">
                        <div className="mb-2 flex items-center justify-between">
                            <div className="text-xs font-semibold text-zinc-700 dark:text-white/80">最近{poolWindowLabel}参与池子</div>
                            <div className="text-[11px] text-zinc-500 dark:text-white/40">Top {topPools.length}</div>
                        </div>
                        {topPools.length ? (
                            <div className="overflow-x-auto">
                                <table className="min-w-full text-left text-[11px]">
                                    <thead className="text-zinc-500 dark:text-white/40">
                                        <tr>
                                            <th className="pb-1 pr-3 font-medium">池子</th>
                                            <th className="pb-1 pr-3 font-medium">版本/费率</th>
                                            <th className="pb-1 pr-3 text-right font-medium">钱包数</th>
                                            <th className="pb-1 pr-0 text-right font-medium">操作</th>
                                        </tr>
                                    </thead>
                                    <tbody className="text-zinc-800 dark:text-white/85">
                                        {topPools.map((pool) => {
                                            const poolId = String(pool?.pool_id || '').trim();
                                            const pair = String(pool?.pair || '').trim();
                                            const version = String(pool?.pool_version || '').trim().toUpperCase();
                                            const feePct = Number(pool?.fee_pct);
                                            const walletCount = Number(pool?.wallet_count ?? 0);
                                            return (
                                                <tr key={`${version}:${poolId}`} className="border-t border-zinc-200/70 dark:border-white/10">
                                                    <td className="py-1.5 pr-3 font-semibold">{pair || shortHex(poolId, 10, 6) || '--'}</td>
                                                    <td className="py-1.5 pr-3 text-zinc-500 dark:text-white/40">
                                                        {version || '--'}
                                                        {Number.isFinite(feePct) && feePct > 0 ? ` · ${formatPct(feePct)}` : ''}
                                                    </td>
                                                    <td className="py-1.5 pr-3 text-right tabular-nums">{Number.isFinite(walletCount) ? walletCount : '--'}</td>
                                                    <td className="py-1.5 pr-0 text-right">
                                                        <button
                                                            type="button"
                                                            onClick={() => {
                                                                hapticImpact('light');
                                                                safeCopy(poolId, onNotice);
                                                            }}
                                                            className="inline-flex items-center rounded-lg bg-zinc-100 px-2 py-1 text-[10px] font-semibold text-zinc-700 hover:bg-zinc-200 dark:bg-white/5 dark:text-white/70 dark:hover:bg-white/10"
                                                        >
                                                            复制
                                                        </button>
                                                    </td>
                                                </tr>
                                            );
                                        })}
                                    </tbody>
                                </table>
                            </div>
                        ) : (
                            <div className="rounded-xl border border-zinc-200 bg-white/70 p-3 text-[11px] text-zinc-500 dark:border-white/10 dark:bg-white/5 dark:text-white/60">
                                暂无池子数据
                            </div>
                        )}
                    </div>

                    <div className="rounded-xl border border-zinc-200 bg-zinc-50 p-3 dark:border-white/10 dark:bg-[#0f1116]">
                        <div className="mb-2 flex items-center justify-between">
                            <div className="text-xs font-semibold text-zinc-700 dark:text-white/80">最近{pnlWindowLabel}钱包盈亏</div>
                            <div className="text-[11px] text-zinc-500 dark:text-white/40">Top {topWallets.length}</div>
                        </div>
                        {topWallets.length ? (
                            <div className="space-y-2">
                                {topWallets.map((wallet, index) => {
                                    const addr = String(wallet?.wallet_address || '').trim();
                                    const balanceDeltaUsd = Number(wallet?.balance_delta_usdt_24h ?? wallet?.pnl_usdt_24h ?? 0);
                                    const startValueUsd = Number(wallet?.start_value_usdt_24h ?? 0);
                                    const endValueUsd = Number(wallet?.end_value_usdt_24h ?? 0);
                                    const cnt1h = Number(wallet?.event_count_1h ?? 0);
                                    const cnt24h = Number(wallet?.event_count_24h ?? 0);
                                    const rank = index + 1;
                                    const rankTone = rank <= 3
                                        ? 'bg-emerald-500/15 text-emerald-700 dark:bg-emerald-500/10 dark:text-emerald-200'
                                        : 'bg-zinc-100 text-zinc-700 dark:bg-white/5 dark:text-white/70';
                                    const pnlTone = kpiTone(balanceDeltaUsd);
                                    return (
                                        <div key={addr || String(index)} className="rounded-xl border border-zinc-200 bg-white p-2.5 shadow-sm dark:border-white/10 dark:bg-[#111318] dark:shadow-none">
                                            <div className="flex items-start justify-between gap-2">
                                                <div className="min-w-0">
                                                    <div className="flex items-center gap-2">
                                                        <span className={`inline-flex h-5 min-w-[20px] items-center justify-center rounded-md px-1 text-[10px] font-bold ${rankTone}`}>
                                                            #{rank}
                                                        </span>
                                                        <span className="truncate font-mono text-[11px] font-semibold text-zinc-900 dark:text-white/90">
                                                            {shortHex(addr, 10, 8) || '--'}
                                                        </span>
                                                    </div>
                                                    <div className="mt-1 flex flex-wrap items-center gap-x-2 gap-y-0.5 text-[10px] text-zinc-500 dark:text-white/40">
                                                        <span>24h净变化 {formatUsd(balanceDeltaUsd)}</span>
                                                        <span className="opacity-80">今日0点 {formatUsd(startValueUsd)} → 当前 {formatUsd(endValueUsd)}</span>
                                                        <span>1h/{pnlWindowLabel} {Number.isFinite(cnt1h) ? cnt1h : '--'}/{Number.isFinite(cnt24h) ? cnt24h : '--'}</span>
                                                    </div>
                                                </div>
                                                <div className="shrink-0 text-right">
                                                    <div className={`text-sm font-extrabold tabular-nums ${pnlTone}`}>{formatUsd(balanceDeltaUsd)}</div>
                                                    <div className="text-[10px] text-zinc-500 dark:text-white/40">24h净变化</div>
                                                </div>
                                            </div>

                                            <div className="mt-2 flex items-center justify-end gap-1.5">
                                                <button
                                                    type="button"
                                                    onClick={() => {
                                                        hapticImpact('light');
                                                        safeCopy(addr, onNotice);
                                                    }}
                                                    className="inline-flex items-center rounded-lg bg-zinc-100 px-2 py-1 text-[10px] font-semibold text-zinc-700 hover:bg-zinc-200 dark:bg-white/5 dark:text-white/70 dark:hover:bg-white/10"
                                                >
                                                    复制
                                                </button>
                                                <button
                                                    type="button"
                                                    onClick={() => {
                                                        hapticImpact('light');
                                                        setFollowModalAddr(addr);
                                                        setFollowModalOpen(true);
                                                    }}
                                                    className="inline-flex items-center rounded-lg bg-emerald-500/15 px-2 py-1 text-[10px] font-semibold text-emerald-700 hover:bg-emerald-500/20 dark:bg-emerald-500/10 dark:text-emerald-200 dark:hover:bg-emerald-500/15"
                                                >
                                                    跟单
                                                </button>
                                                <button
                                                    type="button"
                                                    onClick={() => {
                                                        hapticImpact('light');
                                                        setWalletModalAddr(addr);
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
                        ) : (
                            <div className="rounded-xl border border-zinc-200 bg-white/70 p-3 text-[11px] text-zinc-500 dark:border-white/10 dark:bg-white/5 dark:text-white/60">
                                暂无钱包数据
                            </div>
                        )}
                    </div>
                </div>
            ) : (
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
                                <span className="text-[11px] text-zinc-500 dark:text-white/40">{followConfigsLoading ? '加载中…' : `${enabledFollowWallets.length} 个`}</span>
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
                                        <div key={wallet} className="flex items-center justify-between gap-2 rounded-lg border border-zinc-200 bg-white p-2 dark:border-white/10 dark:bg-[#111318]">
                                            <div className="min-w-0">
                                                <div className="truncate text-[11px] font-semibold text-zinc-800 dark:text-white/85">{shortHex(wallet, 10, 8)}</div>
                                                <div className="text-[10px] text-zinc-500 dark:text-white/40">
                                                    单次 {formatUsd(perTrade)} · 总额 {formatUsd(maxTotal)} · 延迟 {Number.isFinite(dMin) ? dMin : 0}-{Number.isFinite(dMax) ? dMax : 60}s
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
