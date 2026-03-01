import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { copyToClipboard, hapticImpact, hapticNotification } from '../lib/telegram';
import {
    fetchSmartMoneyWatchedWallets,
    addSmartMoneyWatchedWallets,
    removeSmartMoneyWatchedWallets,
    updateSmartMoneyWatchedWalletLabel,
} from '../lib/api';

function normalizeAddress(value) {
    const text = String(value || '').trim();
    if (!/^0x[0-9a-fA-F]{40}$/.test(text)) return '';
    return `0x${text.slice(2).toLowerCase()}`;
}

function shortHex(value, head = 6, tail = 4) {
    const s = String(value || '').trim();
    if (!s) return '';
    if (s.length <= head + tail + 2) return s;
    return `${s.slice(0, head)}...${s.slice(-tail)}`;
}

function formatTime(value) {
    const ts = Date.parse(String(value || ''));
    if (!Number.isFinite(ts) || ts <= 0) return '--';
    const diffSec = Math.max(0, Math.floor((Date.now() - ts) / 1000));
    if (diffSec < 60) return `${diffSec}s`;
    if (diffSec < 3600) return `${Math.floor(diffSec / 60)}m`;
    if (diffSec < 86400) return `${Math.floor(diffSec / 3600)}h`;
    return `${Math.floor(diffSec / 86400)}d`;
}

function sourceLabel(source) {
    const s = String(source || '').trim().toLowerCase();
    if (!s || s === 'user_managed') return '手动';
    if (s === 'scan_add' || s === 'smart_lp') return '合约发现';
    return s;
}

export default function SmartMoneyWatchedWalletsTab({ apiBaseUrl, initData, chain, onNotice }) {
    const [wallets, setWallets] = useState([]);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState('');
    const [warningMsg, setWarningMsg] = useState('');
    const [refreshTick, setRefreshTick] = useState(0);
    const [stats, setStats] = useState({ manualTotal: 0, systemTotal: 0, maxManual: 50 });

    const [query, setQuery] = useState('');
    const [selected, setSelected] = useState(new Set());

    const [showAddForm, setShowAddForm] = useState(false);
    const [addInput, setAddInput] = useState('');
    const [addLabel, setAddLabel] = useState('');
    const [adding, setAdding] = useState(false);
    const [deleting, setDeleting] = useState(false);

    const [editingAddr, setEditingAddr] = useState('');
    const [editLabel, setEditLabel] = useState('');
    const [savingLabel, setSavingLabel] = useState(false);

    useEffect(() => {
        if (!initData) {
            setLoading(false);
            setError('缺少 initData，无法加载监控列表');
            return;
        }
        let cancelled = false;
        const ac = new AbortController();
        const timeout = setTimeout(() => {
            try {
                ac.abort();
            } catch {
                // ignore
            }
        }, 12000);

        setLoading(true);
        setError('');
        setWarningMsg('');
        fetchSmartMoneyWatchedWallets({ apiBaseUrl, initData, chain, signal: ac.signal })
            .then((resp) => {
                if (cancelled) return;
                const list = Array.isArray(resp?.wallets) ? resp.wallets : [];
                setWallets(list);
                const warns = Array.isArray(resp?.warnings) ? resp.warnings.map((w) => String(w || '').trim()).filter(Boolean) : [];
                setWarningMsg(warns[0] || '');
                const manualTotal = Number(resp?.manual_total ?? list.filter((w) => String(w?.source || 'user_managed').toLowerCase() === 'user_managed').length);
                const systemTotal = Number(resp?.system_total ?? Math.max(0, list.length - manualTotal));
                const maxManual = Number(resp?.max_manual ?? 50);
                setStats({
                    manualTotal: Number.isFinite(manualTotal) && manualTotal >= 0 ? manualTotal : 0,
                    systemTotal: Number.isFinite(systemTotal) && systemTotal >= 0 ? systemTotal : 0,
                    maxManual: Number.isFinite(maxManual) && maxManual > 0 ? maxManual : 50,
                });
                setSelected((prev) => {
                    if (!prev.size) return prev;
                    const keep = new Set();
                    const valid = new Set(list.map((w) => String(w?.wallet_address || '').trim().toLowerCase()));
                    prev.forEach((addr) => {
                        if (valid.has(String(addr).toLowerCase())) keep.add(addr);
                    });
                    return keep;
                });
            })
            .catch((err) => {
                if (cancelled) return;
                const message = String(err?.message || err || '');
                if (message.toLowerCase().includes('aborted') || message.toLowerCase().includes('abort')) {
                    setError('请求超时，请点“刷新”重试');
                    return;
                }
                setError(message || '加载失败');
                setWarningMsg('');
            })
            .finally(() => {
                if (!cancelled) setLoading(false);
            });

        return () => {
            cancelled = true;
            clearTimeout(timeout);
            ac.abort();
        };
    }, [apiBaseUrl, initData, chain, refreshTick]);

    const filteredWallets = useMemo(() => {
        const keyword = String(query || '').trim().toLowerCase();
        if (!keyword) return wallets;
        return wallets.filter((row) => {
            const addr = String(row?.wallet_address || '').toLowerCase();
            const label = String(row?.label || '').toLowerCase();
            return addr.includes(keyword) || label.includes(keyword);
        });
    }, [wallets, query]);

    const selectedCount = selected.size;
    const allFilteredSelectable = filteredWallets.filter((row) => row?.removable !== false);
    const allFilteredSelected = allFilteredSelectable.length > 0 && allFilteredSelectable.every((row) => selected.has(String(row?.wallet_address || '').trim().toLowerCase()));

    const toggleSelect = useCallback((walletAddress) => {
        const normalized = String(walletAddress || '').trim().toLowerCase();
        if (!normalized) return;
        setSelected((prev) => {
            const next = new Set(prev);
            if (next.has(normalized)) next.delete(normalized);
            else next.add(normalized);
            return next;
        });
    }, []);

    const toggleSelectAllFiltered = useCallback(() => {
        setSelected((prev) => {
            const next = new Set(prev);
            if (allFilteredSelected) {
                filteredWallets.forEach((row) => next.delete(String(row?.wallet_address || '').trim().toLowerCase()));
            } else {
                filteredWallets.forEach((row) => {
                    if (row?.removable === false) return;
                    const addr = String(row?.wallet_address || '').trim().toLowerCase();
                    if (addr) next.add(addr);
                });
            }
            return next;
        });
    }, [allFilteredSelected, filteredWallets]);

    const handleAdd = useCallback(async () => {
        if (!addInput.trim()) return;
        setAdding(true);
        setError('');
        try {
            const rawList = addInput
                .split(/[\n,;]+/)
                .map((s) => s.trim())
                .filter(Boolean);

            const normalized = rawList.map(normalizeAddress).filter(Boolean);
            const unique = Array.from(new Set(normalized));
            const invalidCount = rawList.length - normalized.length;

            if (!unique.length) {
                setError('未检测到有效钱包地址（格式: 0x...）');
                return;
            }

            const payload = unique.map((addr) => ({
                address: addr,
                label: String(addLabel || '').trim(),
            }));

            const resp = await addSmartMoneyWatchedWallets({
                apiBaseUrl,
                initData,
                chain,
                wallets: payload,
            });

            const added = Number(resp?.added ?? 0);
            const duplicates = Number(resp?.duplicates ?? 0);
            const msgParts = [`新增 ${added}`];
            if (duplicates > 0) msgParts.push(`重复 ${duplicates}`);
            if (invalidCount > 0) msgParts.push(`无效 ${invalidCount}`);

            hapticNotification('success');
            if (onNotice) onNotice(msgParts.join('，'));

            setAddInput('');
            setAddLabel('');
            setShowAddForm(false);
            setRefreshTick((v) => v + 1);
        } catch (err) {
            setError(String(err?.message || err || '添加失败'));
            hapticNotification('error');
        } finally {
            setAdding(false);
        }
    }, [addInput, addLabel, apiBaseUrl, initData, chain, onNotice]);

    const handleDeleteSelected = useCallback(async () => {
        const toDelete = Array.from(selected);
        if (!toDelete.length) return;
        setDeleting(true);
        setError('');
        try {
            const resp = await removeSmartMoneyWatchedWallets({
                apiBaseUrl,
                initData,
                chain,
                walletAddresses: toDelete,
            });
            hapticNotification('success');
            if (onNotice) onNotice(`已移除 ${Number(resp?.deleted ?? 0)} 个钱包`);
            setSelected(new Set());
            setRefreshTick((v) => v + 1);
        } catch (err) {
            setError(String(err?.message || err || '删除失败'));
            hapticNotification('error');
        } finally {
            setDeleting(false);
        }
    }, [selected, apiBaseUrl, initData, chain, onNotice]);

    const handleSaveLabel = useCallback(async (walletAddress) => {
        const addr = String(walletAddress || '').trim();
        if (!addr) return;
        setSavingLabel(true);
        setError('');
        try {
            await updateSmartMoneyWatchedWalletLabel({
                apiBaseUrl,
                initData,
                chain,
                walletAddress: addr,
                label: String(editLabel || '').trim(),
            });
            hapticNotification('success');
            if (onNotice) onNotice('备注已更新');
            setEditingAddr('');
            setRefreshTick((v) => v + 1);
        } catch (err) {
            setError(String(err?.message || err || '更新备注失败'));
            hapticNotification('error');
        } finally {
            setSavingLabel(false);
        }
    }, [apiBaseUrl, initData, chain, editLabel, onNotice]);

    const handleDeleteOne = useCallback(async (walletAddress) => {
        const addr = String(walletAddress || '').trim().toLowerCase();
        if (!addr) return;
        setDeleting(true);
        setError('');
        try {
            await removeSmartMoneyWatchedWallets({
                apiBaseUrl,
                initData,
                chain,
                walletAddresses: [addr],
            });
            hapticNotification('success');
            if (onNotice) onNotice('已移除 1 个钱包');
            setSelected((prev) => {
                const next = new Set(prev);
                next.delete(addr);
                return next;
            });
            setRefreshTick((v) => v + 1);
        } catch (err) {
            setError(String(err?.message || err || '删除失败'));
            hapticNotification('error');
        } finally {
            setDeleting(false);
        }
    }, [apiBaseUrl, initData, chain, onNotice]);

    if (loading) {
        return (
            <div className="mt-3 space-y-2">
                {[1, 2, 3].map((i) => (
                    <div key={i} className="h-12 animate-pulse rounded-xl bg-zinc-100 dark:bg-white/5" />
                ))}
            </div>
        );
    }

    return (
        <div className="mt-3 space-y-3">
            <div className="flex flex-wrap items-center justify-between gap-2">
                <div className="text-xs font-semibold text-zinc-700 dark:text-white/80">
                    监控钱包
                    <span className="ml-1 text-zinc-400 dark:text-white/40">
                        （总计 {wallets.length} / 手动 {stats.manualTotal}/{stats.maxManual} / 合约 {stats.systemTotal}）
                    </span>
                </div>
                <div className="flex items-center gap-1.5">
                    <button
                        type="button"
                        onClick={() => {
                            hapticImpact('light');
                            setRefreshTick((v) => v + 1);
                        }}
                        className="rounded-lg bg-zinc-100 px-2 py-1 text-[10px] font-semibold text-zinc-600 hover:bg-zinc-200 dark:bg-white/5 dark:text-white/60 dark:hover:bg-white/10"
                    >
                        刷新
                    </button>
                    {filteredWallets.length > 0 && (
                        <button
                            type="button"
                            onClick={() => {
                                hapticImpact('light');
                                toggleSelectAllFiltered();
                            }}
                            className="rounded-lg bg-zinc-100 px-2 py-1 text-[10px] font-semibold text-zinc-600 hover:bg-zinc-200 dark:bg-white/5 dark:text-white/60 dark:hover:bg-white/10"
                        >
                            {allFilteredSelected ? '取消全选' : '全选结果'}
                        </button>
                    )}
                    {selectedCount > 0 && (
                        <button
                            type="button"
                            onClick={() => {
                                hapticImpact('medium');
                                handleDeleteSelected();
                            }}
                            disabled={deleting}
                            className="rounded-lg bg-red-500/10 px-2 py-1 text-[10px] font-semibold text-red-600 hover:bg-red-500/20 disabled:opacity-50 dark:text-red-400"
                        >
                            {deleting ? '删除中...' : `删除(${selectedCount})`}
                        </button>
                    )}
                    <button
                        type="button"
                        onClick={() => {
                            hapticImpact('light');
                            setShowAddForm((v) => !v);
                        }}
                        className="rounded-lg bg-emerald-500/10 px-2.5 py-1 text-[10px] font-semibold text-emerald-600 hover:bg-emerald-500/20 dark:text-emerald-400"
                    >
                        {showAddForm ? '收起' : '+ 添加'}
                    </button>
                </div>
            </div>

            <div className="rounded-xl border border-zinc-200 bg-zinc-50 p-2 dark:border-white/10 dark:bg-white/[0.02]">
                <input
                    type="text"
                    value={query}
                    onChange={(e) => setQuery(e.target.value)}
                    placeholder="搜索地址或备注..."
                    className="w-full rounded-lg bg-white px-2.5 py-1.5 text-[11px] text-zinc-900 outline-none ring-0 placeholder:text-zinc-400 dark:bg-white/5 dark:text-white/80 dark:placeholder:text-white/25"
                />
            </div>

            {showAddForm && (
                <div className="rounded-xl border border-emerald-500/20 bg-emerald-500/5 p-3 dark:border-emerald-500/10 dark:bg-emerald-500/5">
                    <div className="text-[11px] font-semibold text-emerald-700 dark:text-emerald-300">批量添加监控地址</div>
                    <div className="mt-1 text-[10px] text-zinc-500 dark:text-white/40">
                        每行一个地址，或用逗号分隔；自动去重。
                    </div>
                    <textarea
                        value={addInput}
                        onChange={(e) => setAddInput(e.target.value)}
                        placeholder={'0x1234...abcd\n0x5678...ef01'}
                        rows={3}
                        className="mt-2 w-full rounded-lg bg-white px-2.5 py-1.5 font-mono text-[11px] text-zinc-900 outline-none ring-0 placeholder:text-zinc-400 dark:bg-white/5 dark:text-white/80 dark:placeholder:text-white/25"
                    />
                    <input
                        type="text"
                        value={addLabel}
                        onChange={(e) => setAddLabel(e.target.value)}
                        placeholder="备注（可选）"
                        maxLength={100}
                        className="mt-1.5 w-full rounded-lg bg-white px-2.5 py-1.5 text-[11px] text-zinc-900 outline-none ring-0 placeholder:text-zinc-400 dark:bg-white/5 dark:text-white/80 dark:placeholder:text-white/25"
                    />
                    <div className="mt-2 flex justify-end">
                        <button
                            type="button"
                            onClick={handleAdd}
                            disabled={adding || !addInput.trim()}
                            className="rounded-lg bg-emerald-500 px-3 py-1.5 text-[11px] font-semibold text-white hover:bg-emerald-600 disabled:opacity-50"
                        >
                            {adding ? '添加中...' : '批量添加'}
                        </button>
                    </div>
                </div>
            )}

            {error && (
                <div className="rounded-xl border border-red-500/30 bg-red-500/10 p-2 text-[11px] text-red-700 dark:border-red-500/20 dark:bg-red-500/5 dark:text-red-200">
                    {error}
                </div>
            )}
            {!error && warningMsg && (
                <div className="rounded-xl border border-amber-500/30 bg-amber-500/10 p-2 text-[11px] text-amber-700 dark:border-amber-500/20 dark:bg-amber-500/5 dark:text-amber-200">
                    {warningMsg}
                </div>
            )}

            {filteredWallets.length === 0 && (
                <div className="rounded-xl border border-dashed border-zinc-300 bg-zinc-50 p-6 text-center dark:border-white/10 dark:bg-white/[0.02]">
                    {wallets.length === 0 ? (
                        <>
                            <div className="text-sm text-zinc-500 dark:text-white/40">暂无监控钱包</div>
                            <div className="mt-1 text-[11px] text-zinc-400 dark:text-white/30">
                                点击 “+ 添加” 开始监控聪明钱钱包。
                            </div>
                        </>
                    ) : (
                        <div className="text-sm text-zinc-400 dark:text-white/30">暂无匹配的钱包</div>
                    )}
                </div>
            )}

            {filteredWallets.map((row) => {
                const addr = String(row?.wallet_address || '').trim().toLowerCase();
                const isSelected = selected.has(addr);
                const isEditing = editingAddr === addr;
                const editableLabel = row?.editable_label !== false;
                const removable = row?.removable !== false;
                const src = String(row?.source || 'user_managed').trim().toLowerCase();
                const srcText = sourceLabel(src);
                const srcClass = src === 'user_managed'
                    ? 'bg-emerald-500/10 text-emerald-700 dark:bg-emerald-500/15 dark:text-emerald-300'
                    : 'bg-amber-500/10 text-amber-700 dark:bg-amber-500/15 dark:text-amber-300';
                return (
                    <div
                        key={String(row?.id || addr)}
                        className={`flex items-center gap-2 rounded-xl border p-2.5 transition ${
                            isSelected
                                ? 'border-red-500/30 bg-red-500/5 dark:border-red-500/20 dark:bg-red-500/5'
                                : 'border-zinc-200 bg-white dark:border-white/10 dark:bg-white/[0.02]'
                        }`}
                    >
                        <button
                            type="button"
                            onClick={() => {
                                if (!removable) return;
                                toggleSelect(addr);
                            }}
                            disabled={!removable}
                            className={`flex h-4 w-4 shrink-0 items-center justify-center rounded border transition ${
                                isSelected
                                    ? 'border-red-500 bg-red-500 text-white'
                                    : 'border-zinc-300 dark:border-white/20'
                            } ${!removable ? 'opacity-40' : ''}`}
                        >
                            {isSelected && (
                                <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="3">
                                    <polyline points="20 6 9 17 4 12" />
                                </svg>
                            )}
                        </button>

                        <div className="min-w-0 flex-1">
                            <div className="flex items-center gap-2">
                                <button
                                    type="button"
                                    onClick={() => {
                                        copyToClipboard(addr);
                                        hapticNotification('success');
                                        if (onNotice) onNotice('已复制地址');
                                    }}
                                    className="font-mono text-[11px] font-semibold text-zinc-800 hover:text-emerald-600 dark:text-white/80 dark:hover:text-emerald-300"
                                    title={addr}
                                >
                                    {shortHex(addr, 8, 6)}
                                </button>
                                <span className="text-[10px] text-zinc-400 dark:text-white/30">
                                    {formatTime(row?.created_at)}
                                </span>
                                <span className={`rounded px-1 py-0.5 text-[9px] font-semibold ${srcClass}`}>
                                    {srcText}
                                </span>
                            </div>

                            {isEditing && editableLabel ? (
                                <div className="mt-1 flex items-center gap-1">
                                    <input
                                        type="text"
                                        value={editLabel}
                                        onChange={(e) => setEditLabel(e.target.value)}
                                        maxLength={100}
                                        autoFocus
                                        onKeyDown={(e) => {
                                            if (e.key === 'Enter') handleSaveLabel(addr);
                                            if (e.key === 'Escape') setEditingAddr('');
                                        }}
                                        className="w-full rounded bg-zinc-100 px-1.5 py-0.5 text-[10px] text-zinc-700 outline-none dark:bg-white/10 dark:text-white/70"
                                        placeholder="输入备注"
                                    />
                                    <button
                                        type="button"
                                        onClick={() => handleSaveLabel(addr)}
                                        disabled={savingLabel}
                                        className="shrink-0 text-[10px] font-semibold text-emerald-600 dark:text-emerald-400"
                                    >
                                        {savingLabel ? '...' : '保存'}
                                    </button>
                                    <button
                                        type="button"
                                        onClick={() => setEditingAddr('')}
                                        className="shrink-0 text-[10px] text-zinc-400"
                                    >
                                        取消
                                    </button>
                                </div>
                            ) : editableLabel ? (
                                <button
                                    type="button"
                                    onClick={() => {
                                        hapticImpact('light');
                                        setEditingAddr(addr);
                                        setEditLabel(String(row?.label || ''));
                                    }}
                                    className="mt-0.5 text-[10px] text-zinc-400 hover:text-zinc-600 dark:text-white/30 dark:hover:text-white/60"
                                >
                                    {String(row?.label || '').trim() || '点击添加备注'}
                                </button>
                            ) : (
                                <div className="mt-0.5 text-[10px] text-zinc-400 dark:text-white/35">
                                    {String(row?.label || '').trim() || '合约监控钱包'}
                                </div>
                            )}
                        </div>

                        <button
                            type="button"
                            onClick={() => {
                                if (!removable) return;
                                hapticImpact('medium');
                                handleDeleteOne(addr);
                            }}
                            disabled={deleting || !removable}
                            className="shrink-0 rounded-lg p-1 text-zinc-400 hover:bg-red-500/10 hover:text-red-500 disabled:opacity-50 dark:text-white/30 dark:hover:text-red-400"
                            title={removable ? '移除' : '不可移除'}
                        >
                            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                                <line x1="18" y1="6" x2="6" y2="18" />
                                <line x1="6" y1="6" x2="18" y2="18" />
                            </svg>
                        </button>
                    </div>
                );
            })}
        </div>
    );
}
