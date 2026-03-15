import React from 'react';
import NumberFlowValue from './NumberFlowValue.jsx';
import { copyToClipboard, hapticImpact, hapticNotification } from '../lib/telegram';

const USD_DISPLAY_LIMIT = 1e15;
const usdFormatter = new Intl.NumberFormat('en-US', {
    style: 'currency',
    currency: 'USD',
    maximumFractionDigits: 2,
});

const compactNumberFormatter = new Intl.NumberFormat('en-US', {
    notation: 'compact',
    maximumFractionDigits: 2,
});

function formatUsd(v) {
    const n = Number(v ?? 0);
    if (!Number.isFinite(n) || Math.abs(n) > USD_DISPLAY_LIMIT) return '$--';
    return usdFormatter.format(n);
}

function formatTokenAmount(v) {
    const n = Number(v ?? 0);
    if (!Number.isFinite(n) || Math.abs(n) > USD_DISPLAY_LIMIT) return '--';
    const abs = Math.abs(n);
    if (abs === 0) return '0';
    if (abs >= 1000) return compactNumberFormatter.format(n);
    if (abs >= 1) return n.toFixed(4).replace(/\.?0+$/, '');
    return n.toPrecision(4);
}

function formatPct(v, digits = 2) {
    const n = Number(v);
    if (!Number.isFinite(n)) return '--';
    return `${n.toFixed(digits)}%`;
}

function shortHex(value, head = 8, tail = 6) {
    const s = String(value || '').trim();
    if (!s) return '';
    if (s.length <= head + tail + 2) return s;
    return `${s.slice(0, head)}...${s.slice(-tail)}`;
}

function kpiTone(value) {
    const n = Number(value ?? 0);
    if (!Number.isFinite(n)) return 'text-zinc-500 dark:text-white/40';
    if (n > 0) return 'text-emerald-600 dark:text-emerald-300';
    if (n < 0) return 'text-red-600 dark:text-red-300';
    return 'text-zinc-700 dark:text-white/80';
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
        if (typeof onNotice === 'function') onNotice(`复制失败: ${String(e?.message || e)}`, 'error');
    }
}

export default function SmartMoneyPositionCard({
    position,
    onNotice,
    compact = false,
    showWalletAddress = false,
    walletAddress = '',
}) {
    const version = String(position?.pool_version || '').trim().toUpperCase() || '--';
    const poolId = String(position?.pool_id || '').trim();
    const positionId = String(position?.position_id || '').trim();
    const exchange = String(position?.exchange || '').trim();
    const pair = String(position?.pair || '').trim() || '--';
    const feePct = Number(position?.fee_pct);
    const inRange = Boolean(position?.in_range);
    const tickLower = Number(position?.tick_lower ?? 0);
    const tickUpper = Number(position?.tick_upper ?? 0);
    const currentTick = Number(position?.current_tick ?? 0);
    const sym0 = String(position?.token0_symbol || '').trim();
    const sym1 = String(position?.token1_symbol || '').trim();
    const amount0 = Number(position?.amount0 ?? 0);
    const amount1 = Number(position?.amount1 ?? 0);
    const usd0 = Number(position?.amount0_usd ?? 0);
    const usd1 = Number(position?.amount1_usd ?? 0);
    const posUsd = Number(position?.position_usd ?? 0);
    const feeUsd = Number(position?.claimable_fees_usd ?? 0);
    const feeStatus = String(position?.fee_status || '').trim();
    const feeError = String(position?.fee_error || '').trim();
    const feeTone = kpiTone(feeUsd);
    const cardClass = compact
        ? 'rounded-2xl border border-zinc-200/80 bg-zinc-50/70 p-3 dark:border-white/10 dark:bg-[#10141c]'
        : 'rounded-2xl border border-zinc-200 bg-white/40 backdrop-blur-md p-3 dark:border-white/10 dark:bg-white/5';

    return (
        <div className={cardClass}>
            <div className="flex items-start justify-between gap-3">
                <div className="min-w-0">
                    <div className="flex flex-wrap items-center gap-2">
                        <div className="truncate text-sm font-extrabold text-zinc-900 dark:text-white/90">{pair}</div>
                        <span className="shrink-0 rounded bg-zinc-100 px-1.5 py-0.5 text-[10px] font-semibold text-zinc-700 dark:bg-white/10 dark:text-white/70">
                            {version}
                        </span>
                        {exchange ? (
                            <span className="shrink-0 rounded bg-zinc-100 px-1.5 py-0.5 text-[10px] font-semibold text-zinc-700 dark:bg-white/10 dark:text-white/70">
                                {exchange}
                            </span>
                        ) : null}
                        {Number.isFinite(feePct) && feePct > 0 ? (
                            <span className="shrink-0 rounded bg-zinc-100 px-1.5 py-0.5 text-[10px] font-semibold text-zinc-700 dark:bg-white/10 dark:text-white/70">
                                <NumberFlowValue value={feePct} formatter={(v) => formatPct(v)} />
                            </span>
                        ) : null}
                        <span className={`shrink-0 rounded px-1.5 py-0.5 text-[10px] font-semibold ${inRange ? 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-200' : 'bg-zinc-500/10 text-zinc-700 dark:text-white/60'}`}>
                            {inRange ? 'In Range' : 'Out Range'}
                        </span>
                    </div>
                    <div className="mt-1 flex flex-wrap items-center gap-x-2 gap-y-1 text-[11px] text-zinc-500 dark:text-white/40">
                        <span className="font-mono">{shortHex(poolId, 10, 8) || '--'}</span>
                        <span>·</span>
                        <span className="font-mono">#<NumberFlowValue value={positionId || '--'} formatter={() => (positionId || '--')} /></span>
                        {showWalletAddress && walletAddress ? (
                            <>
                                <span>·</span>
                                <span className="font-mono">{shortHex(walletAddress, 10, 8)}</span>
                            </>
                        ) : null}
                    </div>
                </div>
                <div className="shrink-0 text-right">
                    <div className="text-[10px] text-zinc-500 dark:text-white/40">仓位估值</div>
                    <div className={`mt-0.5 text-sm font-extrabold tabular-nums ${kpiTone(posUsd)}`}>
                        <NumberFlowValue value={posUsd} formatter={(v) => formatUsd(v)} />
                    </div>
                </div>
            </div>

            <div className="mt-3 grid grid-cols-2 gap-2 text-[11px]">
                <div className="rounded-xl border border-zinc-200 bg-zinc-50 p-2 dark:border-white/10 dark:bg-[#0f1116]">
                    <div className="text-[10px] text-zinc-500 dark:text-white/40">区间</div>
                    <div className="mt-0.5 font-semibold tabular-nums text-zinc-900 dark:text-white/80">
                        <NumberFlowValue
                            value={Number.isFinite(tickLower) ? tickLower : '--'}
                            formatter={() => (Number.isFinite(tickLower) ? String(tickLower) : '--')}
                        />
                        {' '}→{' '}
                        <NumberFlowValue
                            value={Number.isFinite(tickUpper) ? tickUpper : '--'}
                            formatter={() => (Number.isFinite(tickUpper) ? String(tickUpper) : '--')}
                        />
                    </div>
                    <div className="text-[10px] text-zinc-500 dark:text-white/40">
                        当前{' '}
                        <NumberFlowValue
                            value={Number.isFinite(currentTick) ? currentTick : '--'}
                            formatter={() => (Number.isFinite(currentTick) ? String(currentTick) : '--')}
                        />
                    </div>
                </div>
                <div className="rounded-xl border border-zinc-200 bg-zinc-50 p-2 dark:border-white/10 dark:bg-[#0f1116]">
                    <div className="text-[10px] text-zinc-500 dark:text-white/40">可领取手续费</div>
                    <div className={`mt-0.5 font-semibold tabular-nums ${feeTone}`}>
                        {feeStatus === 'ok'
                            ? <NumberFlowValue value={feeUsd} formatter={(v) => formatUsd(v)} />
                            : '--'}
                    </div>
                    <div className="text-[10px] text-zinc-500 dark:text-white/40">
                        {feeStatus === 'error' && feeError ? feeError : (feeStatus ? `状态 ${feeStatus}` : '')}
                    </div>
                </div>
            </div>

            <div className="mt-2 grid grid-cols-2 gap-2 text-[11px]">
                <div className="rounded-xl border border-zinc-200 bg-zinc-50 p-2 dark:border-white/10 dark:bg-[#0f1116]">
                    <div className="text-[10px] text-zinc-500 dark:text-white/40">Token Amount</div>
                    <div className="mt-0.5 font-semibold tabular-nums text-zinc-900 dark:text-white/80">
                        <NumberFlowValue value={amount0} formatter={(v) => formatTokenAmount(v)} /> {sym0 || 'T0'}
                    </div>
                    <div className="text-[10px] text-zinc-500 dark:text-white/40">
                        <NumberFlowValue value={amount1} formatter={(v) => formatTokenAmount(v)} /> {sym1 || 'T1'}
                    </div>
                </div>
                <div className="rounded-xl border border-zinc-200 bg-zinc-50 p-2 dark:border-white/10 dark:bg-[#0f1116]">
                    <div className="text-[10px] text-zinc-500 dark:text-white/40">Token 估值</div>
                    <div className="mt-0.5 font-semibold tabular-nums text-zinc-900 dark:text-white/80">
                        <NumberFlowValue value={usd0} formatter={(v) => formatUsd(v)} /> {sym0 || 'T0'}
                    </div>
                    <div className="text-[10px] text-zinc-500 dark:text-white/40">
                        <NumberFlowValue value={usd1} formatter={(v) => formatUsd(v)} /> {sym1 || 'T1'}
                    </div>
                </div>
            </div>

            <div className="mt-3 flex items-center justify-end gap-2">
                <button
                    type="button"
                    onClick={() => {
                        hapticImpact('light');
                        safeCopy(poolId, onNotice);
                    }}
                    className="inline-flex items-center rounded-lg bg-zinc-100 px-2 py-1 text-[10px] font-semibold text-zinc-700 hover:bg-zinc-200 dark:bg-white/5 dark:text-white/70 dark:hover:bg-white/10"
                >
                    复制 PoolID
                </button>
                <button
                    type="button"
                    onClick={() => {
                        hapticImpact('light');
                        safeCopy(positionId, onNotice);
                    }}
                    className="inline-flex items-center rounded-lg bg-zinc-100 px-2 py-1 text-[10px] font-semibold text-zinc-700 hover:bg-zinc-200 dark:bg-white/5 dark:text-white/70 dark:hover:bg-white/10"
                >
                    复制 PositionID
                </button>
            </div>
        </div>
    );
}
