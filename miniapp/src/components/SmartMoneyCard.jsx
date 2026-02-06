import React, { useMemo } from 'react';
import { formatRelativeTime } from '../lib/time';
import { copyToClipboard, hapticNotification, hapticImpact } from '../lib/telegram';

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

function formatPct(v, digits = 4) {
    const n = Number(v);
    if (!Number.isFinite(n)) return '--';
    // Backend returns fee as a percent number (e.g. 500 -> 0.05%).
    return `${n.toFixed(digits)}%`;
}

function shortHex(value, head = 6, tail = 4) {
    const s = String(value || '').trim();
    if (!s) return '';
    if (s.length <= head + tail + 2) return s;
    return `${s.slice(0, head)}...${s.slice(-tail)}`;
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

export default function SmartMoneyCard({ overview, loading = false, tick, onNotice }) {
    const pools = Array.isArray(overview?.pools) ? overview.pools : [];
    const wallets = Array.isArray(overview?.wallets_24h) ? overview.wallets_24h : [];
    const warnings = Array.isArray(overview?.warnings) ? overview.warnings : [];

    const updatedAtText = useMemo(
        () => formatRelativeTime(overview?.updated_at, tick) || '--',
        [overview?.updated_at, tick],
    );

    return (
        <div className="rounded-2xl border border-zinc-200 bg-white p-4 shadow-sm dark:border-white/10 dark:bg-[#111318] dark:shadow-none">
            <div className="flex items-start justify-between gap-3">
                <div>
                    <div className="text-sm font-extrabold text-zinc-900 dark:text-white/90">
                        Smart Money
                        {loading ? (
                            <span className="ml-2 inline-flex items-center rounded-lg bg-zinc-100 px-2 py-0.5 text-[10px] font-semibold text-zinc-600 dark:bg-white/5 dark:text-white/60">
                                加载中...
                            </span>
                        ) : null}
                    </div>
                    <div className="mt-0.5 text-[11px] text-zinc-500 dark:text-white/40">
                        最近1小时池子 {pools.length} 个 · 最近24小时钱包 {wallets.length} 个 · 更新 {updatedAtText}
                    </div>
                </div>
                <button
                    type="button"
                    onClick={() => {
                        hapticImpact('light');
                        safeCopy(JSON.stringify(overview || {}), onNotice);
                    }}
                    disabled={!overview}
                    className={`inline-flex items-center rounded-xl px-3 py-2 text-xs font-semibold ring-1 ${overview
                        ? 'bg-white text-zinc-700 ring-zinc-200 hover:bg-zinc-50 dark:bg-white/5 dark:text-white/80 dark:ring-white/10 dark:hover:bg-white/10'
                        : 'cursor-not-allowed bg-zinc-100 text-zinc-400 ring-zinc-200 dark:bg-white/5 dark:text-white/30 dark:ring-white/10'
                        }`}
                >
                    复制JSON
                </button>
            </div>

            {warnings.length ? (
                <div className="mt-3 rounded-xl border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-xs text-amber-700 dark:text-amber-200">
                    <div className="font-semibold">提示</div>
                    <ul className="mt-1 list-disc space-y-1 pl-4">
                        {warnings.slice(0, 6).map((w, i) => (
                            <li key={String(i)}>{String(w)}</li>
                        ))}
                    </ul>
                </div>
            ) : null}

            <div className="mt-3 grid grid-cols-1 gap-3">
                <div className="rounded-xl border border-zinc-200 bg-zinc-50 p-3 dark:border-white/10 dark:bg-[#0f1116]">
                    <div className="flex items-center justify-between">
                        <div className="text-xs font-semibold text-zinc-700 dark:text-white/80">最近1小时参与池子</div>
                        <div className="text-[11px] text-zinc-500 dark:text-white/40">{pools.length}</div>
                    </div>

                    {!loading && pools.length === 0 ? (
                        <div className="mt-2 rounded-xl border border-zinc-200 bg-white/70 p-3 text-[11px] text-zinc-500 dark:border-white/10 dark:bg-white/5 dark:text-white/60">
                            暂无数据
                        </div>
                    ) : null}

                    {pools.length ? (
                        <div className="mt-2 space-y-2">
                            {pools.slice(0, 20).map((p) => {
                                const poolId = String(p?.pool_id || '').trim();
                                const pair = String(p?.pair || '').trim();
                                const pv = String(p?.pool_version || '').trim();
                                const feePct = Number(p?.fee_pct);
                                const walletCount = Number(p?.wallet_count ?? 0);

                                const title = pair || shortHex(poolId, 10, 6) || '-';
                                const subtitleParts = [];
                                if (pv) subtitleParts.push(pv.toUpperCase());
                                if (Number.isFinite(feePct) && feePct > 0) subtitleParts.push(`Fee ${formatPct(feePct, 2)}`);
                                const subtitle = subtitleParts.join(' · ');

                                return (
                                    <div
                                        key={`${pv}:${poolId}`}
                                        className="flex items-center justify-between gap-3 rounded-xl border border-zinc-200 bg-white px-3 py-2 shadow-sm dark:border-white/10 dark:bg-[#111318] dark:shadow-none"
                                    >
                                        <div className="min-w-0">
                                            <div className="truncate text-sm font-semibold text-zinc-900 dark:text-white/90">
                                                {title}
                                            </div>
                                            <div className="mt-0.5 truncate text-[10px] text-zinc-500 dark:text-white/40">
                                                {subtitle || shortHex(poolId, 16, 8) || '--'}
                                            </div>
                                        </div>
                                        <div className="flex shrink-0 items-center gap-2">
                                            <div className="text-right">
                                                <div className="text-sm font-extrabold tabular-nums text-zinc-900 dark:text-white/90">
                                                    {Number.isFinite(walletCount) ? walletCount : '--'}
                                                </div>
                                                <div className="text-[10px] text-zinc-500 dark:text-white/40">钱包</div>
                                            </div>
                                            <button
                                                type="button"
                                                onClick={() => {
                                                    hapticImpact('light');
                                                    safeCopy(poolId, onNotice);
                                                }}
                                                className="inline-flex items-center rounded-lg bg-zinc-100 px-2 py-1 text-[11px] font-semibold text-zinc-700 hover:bg-zinc-200 dark:bg-white/5 dark:text-white/70 dark:hover:bg-white/10"
                                            >
                                                复制
                                            </button>
                                        </div>
                                    </div>
                                );
                            })}
                        </div>
                    ) : null}
                </div>

                <div className="rounded-xl border border-zinc-200 bg-zinc-50 p-3 dark:border-white/10 dark:bg-[#0f1116]">
                    <div className="flex items-center justify-between">
                        <div className="text-xs font-semibold text-zinc-700 dark:text-white/80">最近24小时钱包盈亏</div>
                        <div className="text-[11px] text-zinc-500 dark:text-white/40">{wallets.length}</div>
                    </div>

                    {!loading && wallets.length === 0 ? (
                        <div className="mt-2 rounded-xl border border-zinc-200 bg-white/70 p-3 text-[11px] text-zinc-500 dark:border-white/10 dark:bg-white/5 dark:text-white/60">
                            暂无数据
                        </div>
                    ) : null}

                    {wallets.length ? (
                        <div className="mt-2 space-y-2">
                            {wallets.slice(0, 50).map((w) => {
                                const addr = String(w?.wallet_address || '').trim();
                                const pnl = Number(w?.pnl_usdt_24h ?? 0);
                                const inUsd = Number(w?.in_usdt_24h ?? 0);
                                const outUsd = Number(w?.out_usdt_24h ?? 0);
                                const cnt24 = Number(w?.event_count_24h ?? 0);
                                const cnt1h = Number(w?.event_count_1h ?? 0);

                                const pnlTone =
                                    !Number.isFinite(pnl) ? 'text-zinc-500 dark:text-white/40' : pnl > 0 ? 'text-emerald-600 dark:text-emerald-300' : pnl < 0 ? 'text-red-600 dark:text-red-300' : 'text-zinc-700 dark:text-white/80';

                                return (
                                    <div
                                        key={addr}
                                        className="flex items-center justify-between gap-3 rounded-xl border border-zinc-200 bg-white px-3 py-2 shadow-sm dark:border-white/10 dark:bg-[#111318] dark:shadow-none"
                                    >
                                        <div className="min-w-0">
                                            <div className="flex items-center gap-2">
                                                <div className="truncate text-sm font-semibold text-zinc-900 dark:text-white/90">
                                                    {shortHex(addr, 10, 8) || '--'}
                                                </div>
                                                <span className="inline-flex items-center rounded-lg bg-zinc-100 px-2 py-0.5 text-[10px] font-semibold text-zinc-600 dark:bg-white/5 dark:text-white/60">
                                                    1h {Number.isFinite(cnt1h) ? cnt1h : '--'}
                                                </span>
                                                <span className="inline-flex items-center rounded-lg bg-zinc-100 px-2 py-0.5 text-[10px] font-semibold text-zinc-600 dark:bg-white/5 dark:text-white/60">
                                                    24h {Number.isFinite(cnt24) ? cnt24 : '--'}
                                                </span>
                                            </div>
                                            <div className="mt-0.5 truncate text-[10px] text-zinc-500 dark:text-white/40">
                                                In {formatUsd(inUsd)} · Out {formatUsd(outUsd)}
                                            </div>
                                        </div>
                                        <div className="flex shrink-0 items-center gap-2">
                                            <div className={`text-right ${pnlTone}`}>
                                                <div className="text-sm font-extrabold tabular-nums">
                                                    {formatUsd(pnl)}
                                                </div>
                                                <div className="text-[10px] opacity-70">PnL</div>
                                            </div>
                                            <button
                                                type="button"
                                                onClick={() => {
                                                    hapticImpact('light');
                                                    safeCopy(addr, onNotice);
                                                }}
                                                className="inline-flex items-center rounded-lg bg-zinc-100 px-2 py-1 text-[11px] font-semibold text-zinc-700 hover:bg-zinc-200 dark:bg-white/5 dark:text-white/70 dark:hover:bg-white/10"
                                            >
                                                复制
                                            </button>
                                        </div>
                                    </div>
                                );
                            })}
                        </div>
                    ) : null}
                </div>
            </div>
        </div>
    );
}
