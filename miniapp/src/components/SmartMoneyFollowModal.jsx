import React, { useEffect, useMemo, useRef, useState } from 'react';
import { fetchSmartMoneyFollowConfig, saveSmartMoneyFollowConfig } from '../lib/api';
import { copyToClipboard, hapticImpact, hapticNotification } from '../lib/telegram';

const Icon = ({ path, className = '' }) => (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" className={className} aria-hidden="true">
        <path d={path} />
    </svg>
);

const icons = {
    close: 'M6 18L18 6M6 6l12 12',
    refresh: 'M21 12a9 9 0 1 1-2.64-6.36M21 3v6h-6',
};

function shortHex(value, head = 8, tail = 6) {
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

function parseNumberInput(v) {
    const text = String(v ?? '').trim();
    if (!text) return undefined;
    const n = Number(text);
    if (!Number.isFinite(n)) return undefined;
    return n;
}

function toIntInRange(v, min, max, fallback) {
    const n = Math.round(Number(v));
    if (!Number.isFinite(n)) return fallback;
    if (n < min) return min;
    if (n > max) return max;
    return n;
}

export default function SmartMoneyFollowModal({
    open,
    onClose,
    apiBaseUrl,
    initData,
    chain = 'bsc',
    walletAddress,
    onNotice,
    onSaved,
    onOpenPositions,
}) {
    const [loading, setLoading] = useState(false);
    const [saving, setSaving] = useState(false);
    const [error, setError] = useState('');
    const [nonce, setNonce] = useState(0);

    const [followEnabled, setFollowEnabled] = useState(false);
    const [followPerTrade, setFollowPerTrade] = useState('');
    const [followMaxTotal, setFollowMaxTotal] = useState('');
    const [followDelayMin, setFollowDelayMin] = useState('0');
    const [followDelayMax, setFollowDelayMax] = useState('60');

    const hasHydratedRef = useRef(false);

    const addr = String(walletAddress || '').trim();
    const chainLabel = String(chain || 'bsc').trim() || 'bsc';

    const minDelayNumber = useMemo(() => toIntInRange(followDelayMin, 0, 60, 0), [followDelayMin]);
    const maxDelayNumber = useMemo(() => toIntInRange(followDelayMax, 0, 60, 60), [followDelayMax]);
    const normalizedDelayRange = useMemo(() => {
        if (maxDelayNumber >= minDelayNumber) {
            return { min: minDelayNumber, max: maxDelayNumber };
        }
        return { min: maxDelayNumber, max: minDelayNumber };
    }, [minDelayNumber, maxDelayNumber]);

    useEffect(() => {
        if (!open) {
            setLoading(false);
            setSaving(false);
            setError('');
            setFollowEnabled(false);
            setFollowPerTrade('');
            setFollowMaxTotal('');
            setFollowDelayMin('0');
            setFollowDelayMax('60');
            hasHydratedRef.current = false;
            return;
        }
        if (!addr) {
            setError('walletAddress 为空');
            return;
        }

        let aborted = false;
        const controller = new AbortController();

        setLoading(true);
        setError('');

        fetchSmartMoneyFollowConfig({
            apiBaseUrl,
            initData,
            chain: chainLabel,
            walletAddress: addr,
            signal: controller.signal,
        })
            .then((resp) => {
                if (aborted) return;
                const cfg = resp?.config || null;
                setFollowEnabled(Boolean(cfg?.enabled));
                setFollowPerTrade(String(cfg?.per_trade_amount_usdt ?? ''));
                setFollowMaxTotal(String(cfg?.max_total_amount_usdt ?? ''));
                setFollowDelayMin(String(cfg?.delay_min_seconds ?? 0));
                setFollowDelayMax(String(cfg?.delay_max_seconds ?? 60));
                hasHydratedRef.current = true;
            })
            .catch((e) => {
                if (aborted) return;
                setError(String(e?.message || e));
            })
            .finally(() => {
                if (aborted) return;
                setLoading(false);
            });

        return () => {
            aborted = true;
            controller.abort();
        };
    }, [open, addr, apiBaseUrl, initData, chainLabel, nonce]);

    async function handleSave() {
        if (!addr) return;
        if (!hasHydratedRef.current) {
            setError('配置尚未加载完成，请稍后再试');
            return;
        }

        const per = parseNumberInput(followPerTrade);
        const max = parseNumberInput(followMaxTotal);

        if (typeof per === 'number' && per < 0) {
            setError('单次跟单金额必须 >= 0');
            return;
        }
        if (typeof max === 'number' && max < 0) {
            setError('最大跟单金额必须 >= 0');
            return;
        }
        if (typeof per === 'number' && typeof max === 'number' && max > 0 && per > max) {
            setError('单次跟单金额不能大于最大跟单金额');
            return;
        }

        setSaving(true);
        setError('');
        try {
            const resp = await saveSmartMoneyFollowConfig({
                apiBaseUrl,
                initData,
                chain: chainLabel,
                walletAddress: addr,
                enabled: followEnabled,
                perTradeAmountUSDT: per,
                maxTotalAmountUSDT: max,
                delayMinSeconds: normalizedDelayRange.min,
                delayMaxSeconds: normalizedDelayRange.max,
            });

            const cfg = resp?.config || null;
            if (cfg) {
                setFollowEnabled(Boolean(cfg?.enabled));
                setFollowPerTrade(String(cfg?.per_trade_amount_usdt ?? ''));
                setFollowMaxTotal(String(cfg?.max_total_amount_usdt ?? ''));
                setFollowDelayMin(String(cfg?.delay_min_seconds ?? 0));
                setFollowDelayMax(String(cfg?.delay_max_seconds ?? 60));
                hasHydratedRef.current = true;
            }

            hapticNotification('success');
            if (typeof onNotice === 'function') onNotice(followEnabled ? '跟单已开启' : '跟单已停用', 'success');
            if (typeof onSaved === 'function') onSaved();
        } catch (e) {
            hapticNotification('error');
            setError(String(e?.message || e));
            if (typeof onNotice === 'function') onNotice(`保存失败: ${String(e?.message || e)}`, 'error');
        } finally {
            setSaving(false);
        }
    }

    if (!open) return null;

    return (
        <div className="fixed inset-0 z-50 flex items-end sm:items-center justify-center sm:p-4">
            <button
                type="button"
                className="absolute inset-0 bg-black/60 backdrop-blur-sm transition-opacity"
                onClick={onClose}
                aria-label="关闭"
            />

            <div className="relative w-full max-w-lg overflow-hidden rounded-t-2xl sm:rounded-2xl border border-zinc-200 bg-white shadow-2xl dark:border-white/10 dark:bg-[#111318] flex flex-col h-[92vh] sm:h-[680px]">
                <div className="flex items-center justify-between gap-3 px-4 py-3 border-b border-zinc-100 dark:border-white/5 bg-white/50 dark:bg-white/5 shrink-0">
                    <div className="min-w-0">
                        <div className="truncate text-sm font-bold text-zinc-900 dark:text-white/90">跟单设置</div>
                        <div className="mt-0.5 flex items-center gap-2 text-[10px] text-zinc-500 dark:text-white/40 font-mono">
                            <span className="truncate">{shortHex(addr, 12, 10) || '--'}</span>
                            <span className="shrink-0">·</span>
                            <span className="shrink-0">{chainLabel}</span>
                        </div>
                    </div>
                    <div className="flex items-center gap-2">
                        <button
                            type="button"
                            onClick={() => {
                                hapticImpact('light');
                                safeCopy(addr, onNotice);
                            }}
                            className="inline-flex items-center rounded-lg bg-zinc-100 px-2.5 py-1 text-[11px] font-semibold text-zinc-700 hover:bg-zinc-200 dark:bg-white/5 dark:text-white/70 dark:hover:bg-white/10"
                        >
                            复制
                        </button>
                        <button
                            type="button"
                            onClick={() => {
                                hapticImpact('light');
                                setNonce((v) => v + 1);
                            }}
                            className="inline-flex h-8 w-8 items-center justify-center rounded-lg bg-zinc-100 text-zinc-600 transition hover:bg-zinc-200 active:bg-zinc-300 dark:bg-zinc-800 dark:text-white dark:hover:bg-zinc-700 dark:active:bg-zinc-600"
                            aria-label="刷新"
                            title="刷新"
                        >
                            <Icon path={icons.refresh} className="h-4 w-4" />
                        </button>
                        <button
                            type="button"
                            onClick={onClose}
                            className="inline-flex h-8 w-8 items-center justify-center rounded-lg bg-zinc-100 text-zinc-600 transition hover:bg-zinc-200 active:bg-zinc-300 dark:bg-zinc-800 dark:text-white dark:hover:bg-zinc-700 dark:active:bg-zinc-600"
                            aria-label="关闭"
                        >
                            <Icon path={icons.close} className="h-5 w-5" />
                        </button>
                    </div>
                </div>

                <div className="flex-1 overflow-auto p-4">
                    <div className="rounded-2xl border border-zinc-200 bg-white/70 p-3 dark:border-white/10 dark:bg-white/5">
                        <div className="flex items-center justify-between gap-3">
                            <div className="min-w-0">
                                <div className="text-sm font-semibold text-zinc-900 dark:text-white/90">开关</div>
                                <div className="mt-0.5 text-[10px] text-zinc-500 dark:text-white/40">
                                    {loading ? '加载配置中…' : '钱包开 LP 我也开 / 钱包撤 LP 我也撤'}
                                </div>
                            </div>
                            <button
                                type="button"
                                onClick={() => {
                                    hapticImpact('light');
                                    setFollowEnabled((v) => !v);
                                }}
                                disabled={saving || loading}
                                className={`shrink-0 inline-flex items-center rounded-full px-3 py-1 text-[11px] font-semibold transition ${
                                    followEnabled
                                        ? 'bg-emerald-500/15 text-emerald-700 hover:bg-emerald-500/20 dark:bg-emerald-500/10 dark:text-emerald-200 dark:hover:bg-emerald-500/15'
                                        : 'bg-zinc-100 text-zinc-700 hover:bg-zinc-200 dark:bg-white/5 dark:text-white/70 dark:hover:bg-white/10'
                                }`}
                            >
                                {followEnabled ? '已启用' : '已停用'}
                            </button>
                        </div>

                        <div className="mt-3 grid grid-cols-2 gap-2 text-xs">
                            <label className="rounded-xl border border-zinc-200 bg-zinc-50 p-2 dark:border-white/10 dark:bg-[#0f1116]">
                                <div className="text-[10px] text-zinc-500 dark:text-white/40">单次跟单（USDT）</div>
                                <input
                                    type="number"
                                    inputMode="decimal"
                                    min="0"
                                    step="0.01"
                                    value={followPerTrade}
                                    onChange={(e) => setFollowPerTrade(e.target.value)}
                                    className="mt-1 w-full rounded-lg bg-white px-2 py-1 text-[12px] font-semibold tabular-nums text-zinc-900 outline-none ring-0 dark:bg-white/5 dark:text-white/80"
                                    placeholder="例如 20"
                                    disabled={saving}
                                />
                            </label>
                            <label className="rounded-xl border border-zinc-200 bg-zinc-50 p-2 dark:border-white/10 dark:bg-[#0f1116]">
                                <div className="text-[10px] text-zinc-500 dark:text-white/40">最大跟单（USDT）</div>
                                <input
                                    type="number"
                                    inputMode="decimal"
                                    min="0"
                                    step="0.01"
                                    value={followMaxTotal}
                                    onChange={(e) => setFollowMaxTotal(e.target.value)}
                                    className="mt-1 w-full rounded-lg bg-white px-2 py-1 text-[12px] font-semibold tabular-nums text-zinc-900 outline-none ring-0 dark:bg-white/5 dark:text-white/80"
                                    placeholder="例如 200"
                                    disabled={saving}
                                />
                            </label>
                            <label className="rounded-xl border border-zinc-200 bg-zinc-50 p-2 dark:border-white/10 dark:bg-[#0f1116]">
                                <div className="text-[10px] text-zinc-500 dark:text-white/40">延迟最小（秒）</div>
                                <input
                                    type="number"
                                    inputMode="numeric"
                                    min="0"
                                    max="60"
                                    step="1"
                                    value={followDelayMin}
                                    onChange={(e) => setFollowDelayMin(e.target.value)}
                                    className="mt-1 w-full rounded-lg bg-white px-2 py-1 text-[12px] font-semibold tabular-nums text-zinc-900 outline-none ring-0 dark:bg-white/5 dark:text-white/80"
                                    placeholder="0"
                                    disabled={saving}
                                />
                            </label>
                            <label className="rounded-xl border border-zinc-200 bg-zinc-50 p-2 dark:border-white/10 dark:bg-[#0f1116]">
                                <div className="text-[10px] text-zinc-500 dark:text-white/40">延迟最大（秒）</div>
                                <input
                                    type="number"
                                    inputMode="numeric"
                                    min="0"
                                    max="60"
                                    step="1"
                                    value={followDelayMax}
                                    onChange={(e) => setFollowDelayMax(e.target.value)}
                                    className="mt-1 w-full rounded-lg bg-white px-2 py-1 text-[12px] font-semibold tabular-nums text-zinc-900 outline-none ring-0 dark:bg-white/5 dark:text-white/80"
                                    placeholder="60"
                                    disabled={saving}
                                />
                            </label>
                        </div>

                        {error ? (
                            <div className="mt-2 rounded-xl border border-red-500/30 bg-red-500/10 p-2 text-[11px] text-red-700 dark:border-red-500/20 dark:bg-red-500/5 dark:text-red-200">
                                {error}
                            </div>
                        ) : null}

                        <div className="mt-3 flex items-center justify-between gap-2">
                            <div className="text-[10px] text-zinc-500 dark:text-white/40">
                                停用只会停止后续跟单，不会自动撤出已有跟单仓位。
                            </div>
                            <button
                                type="button"
                                onClick={() => {
                                    hapticImpact('light');
                                    handleSave();
                                }}
                                disabled={saving || loading}
                                className="inline-flex items-center rounded-lg bg-emerald-500 px-3 py-1.5 text-[11px] font-semibold text-white hover:bg-emerald-600 disabled:opacity-60 disabled:hover:bg-emerald-500"
                            >
                                {saving ? '保存中…' : '保存'}
                            </button>
                        </div>
                    </div>

                    <div className="mt-3 rounded-2xl border border-zinc-200 bg-white/70 p-3 dark:border-white/10 dark:bg-white/5">
                        <div className="flex items-center justify-between gap-2">
                            <div className="text-sm font-semibold text-zinc-900 dark:text-white/90">钱包仓位</div>
                            <button
                                type="button"
                                onClick={() => {
                                    if (typeof onOpenPositions === 'function') {
                                        hapticImpact('light');
                                        onOpenPositions(addr);
                                    }
                                }}
                                className="inline-flex items-center rounded-lg bg-zinc-100 px-2 py-1 text-[10px] font-semibold text-zinc-700 hover:bg-zinc-200 dark:bg-white/5 dark:text-white/70 dark:hover:bg-white/10"
                            >
                                查看仓位
                            </button>
                        </div>
                        <div className="mt-1 text-[10px] text-zinc-500 dark:text-white/40">
                            跟单页与仓位页已拆分：仓位只展示持仓，跟单只负责开关和参数。
                        </div>
                    </div>
                </div>
            </div>
        </div>
    );
}
