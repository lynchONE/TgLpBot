import React, { useMemo } from 'react';
import { openLink } from '../lib/telegram';
import { formatDurationFrom, formatRelativeTime } from '../lib/time';

const Icon = ({ path, className = '' }) => (
    <svg viewBox="0 0 24 24" fill="currentColor" className={className} aria-hidden="true">
        <path d={path} />
    </svg>
);

const icons = {
    trend: 'M3 17l6-6 4 4 7-7v4h2V4h-8v2h4l-5 5-4-4-7 7z',
    wallet: 'M4 7a3 3 0 013-3h13v4H7a1 1 0 000 2h14v7a3 3 0 01-3 3H7a3 3 0 01-3-3V7zm16 6h-5v4h5v-4z',
    refresh: 'M17.65 6.35A7.95 7.95 0 0012 4V1L7 6l5 5V7a5 5 0 11-5 5H5a7 7 0 107.65-5.65z',
    link: 'M3.9 12a5 5 0 015-5h3v2h-3a3 3 0 000 6h3v2h-3a5 5 0 01-5-5zm7-1h2v2h-2v-2zm4.1-4h3a5 5 0 010 10h-3v-2h3a3 3 0 000-6h-3V7z',
};

const formatUsd = (v) => {
    const n = Number(v || 0);
    return `$${n.toFixed(2)}`;
};

const pillClassForStatus = (label) => {
    if (label?.includes('错误')) return 'bg-red-500/15 text-red-300 ring-red-500/30';
    if (label?.includes('停止')) return 'bg-amber-500/15 text-amber-300 ring-amber-500/30';
    if (label?.includes('等待')) return 'bg-sky-500/15 text-sky-300 ring-sky-500/30';
    return 'bg-emerald-500/15 text-emerald-300 ring-emerald-500/30';
};

export default function PositionCard({ position, walletAddress, bnbBalance, pollIntervalSec, updatedAt }) {
    const token0 = position?.token_rows?.[0];
    const token1 = position?.token_rows?.[1];

    const titleRight = useMemo(() => formatUsd(position?.totals?.total_usd), [position?.totals?.total_usd]);

    const poolLink = useMemo(() => {
        const pool = position?.pool_id;
        if (!pool) return null;
        if (/^0x[a-fA-F0-9]{40}$/.test(pool)) return `https://bscscan.com/address/${pool}`;
        return null;
    }, [position?.pool_id]);

    const openWallet = () => openLink(`https://bscscan.com/address/${walletAddress}`);
    const openPool = () => poolLink && openLink(poolLink);
    const openToken = (addr) => addr && openLink(`https://bscscan.com/token/${addr}`);

    return (
        <div className="rounded-3xl border border-white/10 bg-[#14161a]/70 p-4 shadow-[0_0_0_1px_rgba(0,0,0,0.25)] backdrop-blur">
            <div className="flex items-start justify-between gap-3">
                <div>
                    <div className="text-sm font-semibold text-white/90">{position?.title}</div>
                    <div className="mt-2 flex flex-wrap items-center gap-2">
                        <span
                            className={`inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-semibold ring-1 ${pillClassForStatus(
                                position?.status_label
                            )}`}
                        >
                            <span className="h-1.5 w-1.5 rounded-full bg-current opacity-90" />
                            {position?.status_label || '运行中'}
                        </span>
                        <span className="inline-flex items-center gap-1 rounded-full bg-white/5 px-2 py-0.5 text-xs text-white/70 ring-1 ring-white/10">
                            <Icon path={icons.trend} className="h-3.5 w-3.5 text-white/60" />
                            {bnbBalance} BNB
                        </span>
                    </div>
                </div>
                <div className="flex items-center gap-2">
                    <div className="text-right">
                        <div className="text-xs text-white/50">总计</div>
                        <div className="text-lg font-extrabold text-fuchsia-300">{titleRight}</div>
                    </div>
                </div>
            </div>

            <div className="mt-4 rounded-2xl border border-white/10 bg-black/20 p-3">
                <div className="flex items-center justify-between">
                    <div className="text-xs font-semibold text-white/70">余额信息</div>
                    <div className="text-[11px] text-white/40">{position?.exchange}</div>
                </div>
                <div className="mt-3 grid grid-cols-4 gap-2 text-[11px] text-white/40">
                    <div>Token</div>
                    <div className="flex items-center gap-1 justify-end">
                        <Icon path={icons.wallet} className="h-3.5 w-3.5" />
                        钱包
                    </div>
                    <div className="justify-end flex items-center gap-1"># 仓位</div>
                    <div className="justify-end flex items-center gap-1 text-emerald-300/80">手续费</div>
                </div>

                {[token0, token1].filter(Boolean).map((row) => (
                    <div key={row.address} className="mt-3 grid grid-cols-4 gap-2 items-start">
                        <div>
                            <div className="text-sm font-bold text-white/90">{row.symbol}</div>
                            <div className="text-[11px] text-white/40">{row.price_usd_text || `$${Number(row.price_usd || 0).toFixed(4)}`}</div>
                        </div>
                        <div className="text-right">
                            <div className="text-sm font-semibold text-white/90 tabular-nums">{row.wallet_amount}</div>
                            <div className="text-[11px] text-white/40 tabular-nums">{formatUsd(row.wallet_usd)}</div>
                        </div>
                        <div className="text-right">
                            <div className="text-sm font-semibold text-white/90 tabular-nums">{row.position_amount}</div>
                            <div className="text-[11px] text-white/40 tabular-nums">{formatUsd(row.position_usd)}</div>
                        </div>
                        <div className="text-right">
                            <div className="text-sm font-semibold text-emerald-300 tabular-nums">{row.fee_amount}</div>
                            <div className="text-[11px] text-emerald-300/70 tabular-nums">{formatUsd(row.fee_usd)}</div>
                        </div>
                    </div>
                ))}

                <div className="mt-3 border-t border-white/10 pt-3 grid grid-cols-4 gap-2 text-sm font-semibold tabular-nums">
                    <div className="text-white/60">小计</div>
                    <div className="text-right text-sky-300">{formatUsd(position?.totals?.wallet_usd)}</div>
                    <div className="text-right text-sky-300">{formatUsd(position?.totals?.position_usd)}</div>
                    <div className="text-right text-emerald-300">{formatUsd(position?.totals?.fee_usd)}</div>
                </div>
            </div>

            <div className="mt-3 grid grid-cols-4 gap-2">
                <button
                    onClick={openWallet}
                    className="rounded-xl border border-white/10 bg-white/5 py-2 text-xs font-semibold text-white/70 hover:bg-white/10 active:bg-white/15"
                >
                    钱包
                </button>
                <button
                    onClick={openPool}
                    disabled={!poolLink}
                    className="rounded-xl border border-white/10 bg-white/5 py-2 text-xs font-semibold text-white/70 hover:bg-white/10 active:bg-white/15 disabled:opacity-40"
                >
                    池子
                </button>
                <button
                    onClick={() => openToken(token0?.address)}
                    disabled={!token0?.address}
                    className="rounded-xl border border-white/10 bg-white/5 py-2 text-xs font-semibold text-white/70 hover:bg-white/10 active:bg-white/15 disabled:opacity-40"
                >
                    {token0?.symbol || 'Token0'}
                </button>
                <button
                    onClick={() => openToken(token1?.address)}
                    disabled={!token1?.address}
                    className="rounded-xl border border-white/10 bg-white/5 py-2 text-xs font-semibold text-white/70 hover:bg-white/10 active:bg-white/15 disabled:opacity-40"
                >
                    {token1?.symbol || 'Token1'}
                </button>
            </div>

            <div className="mt-3 rounded-2xl border border-white/10 bg-black/10 p-3 text-[11px] text-white/60">
                <div className="grid grid-cols-3 gap-2">
                    <div>
                        <div className="text-white/40">Tick</div>
                        <div className="mt-0.5 font-semibold text-white/80 tabular-nums">{position?.current_tick ?? 0}</div>
                    </div>
                    <div>
                        <div className="text-white/40">±{Number(position?.range_percent || 0).toFixed(1)}%</div>
                        <div className="mt-0.5 font-semibold text-white/80 tabular-nums">
                            {position?.in_range ? '区间内' : '超范围'}
                        </div>
                    </div>
                    <div className="text-right">
                        <div className="text-white/40"># NFT</div>
                        <div className="mt-0.5 font-semibold text-white/80 tabular-nums">{position?.position_id}</div>
                    </div>
                </div>

                <div className="mt-2 grid grid-cols-4 gap-2">
                    <div>
                        <div className="text-white/40">间隔</div>
                        <div className="mt-0.5 font-semibold text-white/80 tabular-nums">{pollIntervalSec}s</div>
                    </div>
                    <div>
                        <div className="text-white/40">超范围</div>
                        <div className="mt-0.5 font-semibold text-white/80 tabular-nums">{position?.out_of_range}</div>
                    </div>
                    <div>
                        <div className="text-white/40">运行</div>
                        <div className="mt-0.5 font-semibold text-emerald-300 tabular-nums">
                            {formatDurationFrom(position?.running_since)}
                        </div>
                    </div>
                    <div className="text-right">
                        <div className="text-white/40">更新时间</div>
                        <div className="mt-0.5 font-semibold text-white/80 tabular-nums">
                            {formatRelativeTime(updatedAt)}
                        </div>
                    </div>
                </div>
            </div>
        </div>
    );
}

