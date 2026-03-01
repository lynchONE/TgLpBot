import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { copyToClipboard, hapticImpact, hapticNotification } from '../lib/telegram';
import {
    fetchSmartMoneyWatchedWallets,
    addSmartMoneyWatchedWallets,
    removeSmartMoneyWatchedWallets,
    updateSmartMoneyWatchedWalletLabel,
} from '../lib/api';

function shortHex(value, head = 6, tail = 4) {
    const s = String(value || '').trim();
    if (!s) return '';
    if (s.length <= head + tail + 2) return s;
    return `${s.slice(0, head)}...${s.slice(-tail)}`;
}

export default function SmartMoneyWatchedWalletsTab({ apiBaseUrl, initData, chain, onNotice }) {
    const [wallets, setWallets] = useState([]);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState('');
    const [nonce, setNonce] = useState(0);

    // Add form
    const [showAddForm, setShowAddForm] = useState(false);
    const [addInput, setAddInput] = useState('');
    const [addLabel, setAddLabel] = useState('');
    const [adding, setAdding] = useState(false);

    // Select for batch delete
    const [selected, setSelected] = useState(new Set());
    const [deleting, setDeleting] = useState(false);

    // Inline label editing
    const [editingId, setEditingId] = useState(null);
    const [editLabel, setEditLabel] = useState('');
    const [savingLabel, setSavingLabel] = useState(false);

    // Fetch wallets
    useEffect(() => {
        if (!apiBaseUrl || !initData) return;
        let cancelled = false;
        const ac = new AbortController();

        setLoading(true);
        setError('');
        fetchSmartMoneyWatchedWallets({ apiBaseUrl, initData, chain, signal: ac.signal })
            .then((data) => {
                if (cancelled) return;
                setWallets(Array.isArray(data?.wallets) ? data.wallets : []);
            })
            .catch((err) => {
                if (cancelled) return;
                setError(String(err?.message || err || '加载失败'));
            })
            .finally(() => {
                if (!cancelled) setLoading(false);
            });

        return () => {
            cancelled = true;
            ac.abort();
        };
    }, [apiBaseUrl, initData, chain, nonce]);

    // Add wallets
    const handleAdd = useCallback(async () => {
        if (!addInput.trim()) return;
        setAdding(true);
        setError('');
        try {
            // Parse addresses: one per line or comma-separated
            const lines = addInput
                .split(/[\n,;]+/)
                .map((s) => s.trim())
                .filter((s) => /^0x[0-9a-fA-F]{40}$/.test(s));

            if (lines.length === 0) {
                setError('未找到有效的钱包地址 (格式: 0x...)');
                return;
            }

            const walletEntries = lines.map((addr) => ({
                address: addr,
                label: addLabel.trim() || '',
            }));

            const resp = await addSmartMoneyWatchedWallets({
                apiBaseUrl,
                initData,
                chain,
                wallets: walletEntries,
            });

            hapticNotification('success');
            const msg = `添加 ${resp?.added || 0} 个${resp?.duplicates ? `，重复 ${resp.duplicates} 个` : ''}`;
            if (onNotice) onNotice(msg);

            setAddInput('');
            setAddLabel('');
            setShowAddForm(false);
            setNonce((v) => v + 1);
        } catch (err) {
            setError(String(err?.message || err || '添加失败'));
            hapticNotification('error');
        } finally {
            setAdding(false);
        }
    }, [addInput, addLabel, apiBaseUrl, initData, chain, onNotice]);

    // Remove wallets
    const handleRemoveSelected = useCallback(async () => {
        if (selected.size === 0) return;
        setDeleting(true);
        setError('');
        try {
            const addrs = Array.from(selected);
            const resp = await removeSmartMoneyWatchedWallets({
                apiBaseUrl,
                initData,
                chain,
                walletAddresses: addrs,
            });

            hapticNotification('success');
            if (onNotice) onNotice(`已移除 ${resp?.deleted || 0} 个监控地址`);

            setSelected(new Set());
            setNonce((v) => v + 1);
        } catch (err) {
            setError(String(err?.message || err || '移除失败'));
            hapticNotification('error');
        } finally {
            setDeleting(false);
        }
    }, [selected, apiBaseUrl, initData, chain, onNotice]);

    // Update label
    const handleSaveLabel = useCallback(
        async (walletAddress) => {
            setSavingLabel(true);
            try {
                await updateSmartMoneyWatchedWalletLabel({
                    apiBaseUrl,
                    initData,
                    chain,
                    walletAddress,
                    label: editLabel.trim(),
                });
                hapticNotification('success');
                setEditingId(null);
                setNonce((v) => v + 1);
            } catch (err) {
                setError(String(err?.message || err || '更新失败'));
                hapticNotification('error');
            } finally {
                setSavingLabel(false);
            }
        },
        [editLabel, apiBaseUrl, initData, chain],
    );

    const toggleSelect = useCallback((addr) => {
        setSelected((prev) => {
            const next = new Set(prev);
            if (next.has(addr)) next.delete(addr);
            else next.add(addr);
            return next;
        });
    }, []);

    const selectAll = useCallback(() => {
        if (selected.size === wallets.length) {
            setSelected(new Set());
        } else {
            setSelected(new Set(wallets.map((w) => w.wallet_address)));
        }
    }, [wallets, selected]);

    // Loading state
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
            {/* Header */}
            <div className="flex items-center justify-between">
                <div className="text-xs font-semibold text-zinc-700 dark:text-white/80">
                    监控钱包
                    <span className="ml-1 text-zinc-400 dark:text-white/40">({wallets.length}/50)</span>
                </div>
                <div className="flex gap-1.5">
                    {wallets.length > 0 && (
                        <button
                            type="button"
                            onClick={() => {
                                hapticImpact('light');
                                selectAll();
                            }}
                            className="rounded-lg bg-zinc-100 px-2 py-1 text-[10px] font-semibold text-zinc-600 hover:bg-zinc-200 dark:bg-white/5 dark:text-white/60 dark:hover:bg-white/10"
                        >
                            {selected.size === wallets.length ? '取消全选' : '全选'}
                        </button>
                    )}
                    {selected.size > 0 && (
                        <button
                            type="button"
                            onClick={() => {
                                hapticImpact('medium');
                                handleRemoveSelected();
                            }}
                            disabled={deleting}
                            className="rounded-lg bg-red-500/10 px-2 py-1 text-[10px] font-semibold text-red-600 hover:bg-red-500/20 disabled:opacity-50 dark:text-red-400"
                        >
                            {deleting ? '删除中…' : `删除 (${selected.size})`}
                        </button>
                    )}
                    <button
                        type="button"
                        onClick={() => {
                            hapticImpact('light');
                            setShowAddForm(!showAddForm);
                        }}
                        className="rounded-lg bg-emerald-500/10 px-2.5 py-1 text-[10px] font-semibold text-emerald-600 hover:bg-emerald-500/20 dark:text-emerald-400"
                    >
                        {showAddForm ? '收起' : '+ 添加'}
                    </button>
                </div>
            </div>

            {/* Add Form */}
            {showAddForm && (
                <div className="rounded-xl border border-emerald-500/20 bg-emerald-500/5 p-3 dark:border-emerald-500/10 dark:bg-emerald-500/5">
                    <div className="text-[11px] font-semibold text-emerald-700 dark:text-emerald-300">
                        添加监控地址
                    </div>
                    <div className="mt-1 text-[10px] text-zinc-500 dark:text-white/40">
                        每行一个地址，或用逗号分隔，支持批量添加
                    </div>
                    <textarea
                        value={addInput}
                        onChange={(e) => setAddInput(e.target.value)}
                        placeholder={'0x1234...abcd\n0x5678...efgh'}
                        rows={3}
                        className="mt-2 w-full rounded-lg bg-white px-2.5 py-1.5 font-mono text-[11px] text-zinc-900 outline-none ring-0 placeholder:text-zinc-400 dark:bg-white/5 dark:text-white/80 dark:placeholder:text-white/25"
                    />
                    <input
                        type="text"
                        value={addLabel}
                        onChange={(e) => setAddLabel(e.target.value)}
                        placeholder="备注名 (可选，批量添加时统一设置)"
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
                            {adding ? '添加中…' : '批量添加'}
                        </button>
                    </div>
                </div>
            )}

            {/* Error */}
            {error && (
                <div className="rounded-xl border border-red-500/30 bg-red-500/10 p-2 text-[11px] text-red-700 dark:border-red-500/20 dark:bg-red-500/5 dark:text-red-200">
                    {error}
                </div>
            )}

            {/* Empty state */}
            {wallets.length === 0 && !showAddForm && (
                <div className="rounded-xl border border-dashed border-zinc-300 bg-zinc-50 p-6 text-center dark:border-white/10 dark:bg-white/[0.02]">
                    <div className="text-sm text-zinc-400 dark:text-white/30">暂无监控钱包</div>
                    <div className="mt-1 text-[11px] text-zinc-400 dark:text-white/25">
                        点击「+ 添加」按钮开始监控聪明钱地址
                    </div>
                </div>
            )}

            {/* Wallet List */}
            {wallets.map((w) => {
                const addr = String(w.wallet_address || '').trim();
                const isSelected = selected.has(addr);
                const isEditing = editingId === w.id;

                return (
                    <div
                        key={w.id || addr}
                        className={`flex items-center gap-2 rounded-xl border p-2.5 transition ${
                            isSelected
                                ? 'border-red-500/30 bg-red-500/5 dark:border-red-500/20 dark:bg-red-500/5'
                                : 'border-zinc-200 bg-white dark:border-white/10 dark:bg-white/[0.02]'
                        }`}
                    >
                        {/* Checkbox */}
                        <button
                            type="button"
                            onClick={() => {
                                hapticImpact('light');
                                toggleSelect(addr);
                            }}
                            className={`flex h-4 w-4 shrink-0 items-center justify-center rounded border transition ${
                                isSelected
                                    ? 'border-red-500 bg-red-500 text-white'
                                    : 'border-zinc-300 dark:border-white/20'
                            }`}
                        >
                            {isSelected && (
                                <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="3">
                                    <polyline points="20 6 9 17 4 12" />
                                </svg>
                            )}
                        </button>

                        {/* Content */}
                        <div className="min-w-0 flex-1">
                            <div className="flex items-center gap-1.5">
                                <button
                                    type="button"
                                    onClick={() => {
                                        copyToClipboard(addr);
                                        hapticNotification('success');
                                        if (onNotice) onNotice('已复制地址');
                                    }}
                                    className="font-mono text-[11px] font-semibold text-zinc-800 hover:text-emerald-600 dark:text-white/80 dark:hover:text-emerald-400"
                                    title={addr}
                                >
                                    {shortHex(addr)}
                                </button>
                            </div>

                            {/* Label */}
                            {isEditing ? (
                                <div className="mt-1 flex items-center gap-1">
                                    <input
                                        type="text"
                                        value={editLabel}
                                        onChange={(e) => setEditLabel(e.target.value)}
                                        maxLength={100}
                                        autoFocus
                                        onKeyDown={(e) => {
                                            if (e.key === 'Enter') handleSaveLabel(addr);
                                            if (e.key === 'Escape') setEditingId(null);
                                        }}
                                        className="w-full rounded bg-zinc-100 px-1.5 py-0.5 text-[10px] text-zinc-700 outline-none dark:bg-white/10 dark:text-white/70"
                                        placeholder="输入备注名"
                                    />
                                    <button
                                        type="button"
                                        onClick={() => handleSaveLabel(addr)}
                                        disabled={savingLabel}
                                        className="shrink-0 text-[10px] font-semibold text-emerald-600 dark:text-emerald-400"
                                    >
                                        {savingLabel ? '…' : '保存'}
                                    </button>
                                    <button
                                        type="button"
                                        onClick={() => setEditingId(null)}
                                        className="shrink-0 text-[10px] text-zinc-400"
                                    >
                                        取消
                                    </button>
                                </div>
                            ) : (
                                <button
                                    type="button"
                                    onClick={() => {
                                        hapticImpact('light');
                                        setEditingId(w.id);
                                        setEditLabel(w.label || '');
                                    }}
                                    className="mt-0.5 text-[10px] text-zinc-400 hover:text-zinc-600 dark:text-white/30 dark:hover:text-white/60"
                                >
                                    {w.label ? w.label : '点击添加备注'}
                                </button>
                            )}
                        </div>

                        {/* Single delete */}
                        <button
                            type="button"
                            onClick={async () => {
                                hapticImpact('medium');
                                try {
                                    await removeSmartMoneyWatchedWallets({
                                        apiBaseUrl,
                                        initData,
                                        chain,
                                        walletAddresses: [addr],
                                    });
                                    hapticNotification('success');
                                    if (onNotice) onNotice('已移除');
                                    setNonce((v) => v + 1);
                                } catch (err) {
                                    setError(String(err?.message || '移除失败'));
                                }
                            }}
                            className="shrink-0 rounded-lg p-1 text-zinc-400 hover:bg-red-500/10 hover:text-red-500 dark:text-white/30 dark:hover:text-red-400"
                            title="移除"
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
