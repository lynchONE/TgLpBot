import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { CheckCircle2, Copy, KeyRound, Pencil, Plus, RefreshCw, ShieldCheck, Star, Trash2, WalletCards } from 'lucide-react';
import BottomSheet from './BottomSheet.jsx';
import CustomSelect from './CustomSelect.jsx';
import ConfirmDialog from './ConfirmDialog.jsx';
import { fetchWallets, walletCRUD } from '../lib/api';
import { getBrandTheme } from '../lib/brand';

const CHAIN_OPTIONS = [
    { value: 'bsc', label: 'BSC', icon: '🟡' },
    { value: 'base', label: 'Base', icon: '🔵' },
];

function shortAddress(addr) {
    if (!addr) return '-';
    return `${addr.slice(0, 8)}...${addr.slice(-6)}`;
}

function formatBalance(value, digits) {
    if (value === 'N/A') return '-';
    const num = Number(value);
    if (!Number.isFinite(num)) return '-';
    return num.toFixed(digits);
}

export default function WalletManagePage({ open = true, onClose, apiBaseUrl, initData, accentTheme = 'lime', multiChainEnabled = true, embedded = false }) {
    const brand = getBrandTheme(accentTheme);
    const [chain, setChain] = useState('bsc');
    const [wallets, setWallets] = useState([]);
    const [nativeSymbol, setNativeSymbol] = useState('BNB');
    const [stableSymbol, setStableSymbol] = useState('USDT');
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState('');
    const [copiedAddr, setCopiedAddr] = useState('');
    const [crudAction, setCrudAction] = useState(null); // 'import', 'create', 'rename'
    const [crudForm, setCrudForm] = useState({ name: '', privateKey: '', walletId: null });
    const [crudLoading, setCrudLoading] = useState(false);
    const [deleteTarget, setDeleteTarget] = useState(null);

    const load = useCallback(async () => {
        if (!initData) return;
        setLoading(true);
        setError('');
        try {
            const resp = await fetchWallets({ apiBaseUrl, initData, chain });
            setWallets(resp?.wallets || []);
            setNativeSymbol(resp?.native_symbol || 'BNB');
            setStableSymbol(resp?.stable_symbol || 'USDT');
        } catch (e) {
            setError(String(e?.message || e));
            setWallets([]); // clear on error
        } finally {
            setLoading(false);
        }
    }, [apiBaseUrl, initData, chain]);

    useEffect(() => {
        if (open) {
            load();
            setCrudAction(null);
            setError('');
        }
    }, [open, load]);

    const copyAddress = async (addr) => {
        try {
            await navigator.clipboard.writeText(addr);
            setCopiedAddr(addr);
            setTimeout(() => setCopiedAddr(''), 2000);
        } catch { /* ignore */ }
    };

    const handleCrudSubmit = async (e) => {
        e.preventDefault();
        setCrudLoading(true);
        setError('');
        try {
            await walletCRUD({
                apiBaseUrl,
                initData,
                action: crudAction,
                privateKey: crudForm.privateKey,
                name: crudForm.name,
                walletId: crudForm.walletId,
            });
            setCrudAction(null);
            setCrudForm({ name: '', privateKey: '', walletId: null });
            await load();
        } catch (err) {
            setError(String(err?.message || err));
        } finally {
            setCrudLoading(false);
        }
    };

    const handleAction = async (action, w) => {
        if (action === 'delete') {
            setDeleteTarget(w);
            return;
        }
        if (action === 'set_default') {
            setLoading(true);
            setError('');
            try {
                await walletCRUD({
                    apiBaseUrl,
                    initData,
                    action,
                    walletId: w.id,
            });
            await load();
        } catch (err) {
                setError(String(err?.message || err));
                setLoading(false);
            }
        } else if (action === 'rename') {
            setCrudAction('rename');
            setCrudForm({ name: w.name || '', privateKey: '', walletId: w.id });
        }
    };

    const confirmDeleteWallet = async () => {
        const w = deleteTarget;
        if (!w) return;
        setDeleteTarget(null);
        setLoading(true);
        setError('');
        try {
            await walletCRUD({
                apiBaseUrl,
                initData,
                action: 'delete',
                walletId: w.id,
            });
            await load();
        } catch (err) {
            setError(String(err?.message || err));
            setLoading(false);
        }
    };

    const totalNative = wallets.reduce((s, w) => {
        const v = parseFloat(w.native_balance);
        return s + (Number.isFinite(v) ? v : 0);
    }, 0);
    const totalStable = wallets.reduce((s, w) => {
        const v = parseFloat(w.stable_balance);
        return s + (Number.isFinite(v) ? v : 0);
    }, 0);
    const defaultWallet = useMemo(() => wallets.find((w) => w.is_default), [wallets]);
    const inputClass = `w-full rounded-2xl border border-zinc-200/80 bg-white/90 px-4 py-3 text-sm font-semibold text-zinc-900 shadow-sm outline-none transition placeholder:text-zinc-400 ${brand.inputFocusClass} dark:border-white/10 dark:bg-white/[0.04] dark:text-white/90 dark:placeholder:text-white/25`;

    const renderCrudForm = () => {
        if (!crudAction) return null;
        const title = crudAction === 'import' ? '导入钱包' : crudAction === 'create' ? '创建钱包' : '重命名钱包';
        const description = crudAction === 'import'
            ? '粘贴私钥后会在当前链创建可用钱包，请确认来源安全。'
            : crudAction === 'create'
                ? '系统会创建新钱包并保存到当前账户。'
                : '只修改展示名称，不影响链上地址。';
        const Icon = crudAction === 'import' ? KeyRound : crudAction === 'create' ? Plus : Pencil;
        return (
            <div className="mb-4 overflow-hidden rounded-[28px] border border-zinc-200/70 bg-white/90 p-4 shadow-[0_18px_44px_rgba(15,23,42,0.08)] dark:border-white/10 dark:bg-white/[0.045]">
                <div className="mb-4 flex items-start justify-between gap-3">
                    <div className="flex min-w-0 items-start gap-3">
                        <div className={`inline-flex h-11 w-11 shrink-0 items-center justify-center rounded-2xl ${brand.iconChipClass}`}>
                            <Icon className="h-5 w-5" />
                        </div>
                        <div className="min-w-0">
                            <div className="text-[10px] font-black uppercase tracking-[0.2em] text-zinc-400 dark:text-white/35">WALLET ACTION</div>
                            <h3 className="mt-1 text-base font-black text-zinc-900 dark:text-white">{title}</h3>
                            <p className="mt-1 text-xs leading-5 text-zinc-500 dark:text-white/45">{description}</p>
                        </div>
                    </div>
                    <button
                        type="button"
                        onClick={() => setCrudAction(null)}
                        className="shrink-0 rounded-full bg-zinc-100 px-3 py-1.5 text-[11px] font-bold text-zinc-500 transition hover:bg-zinc-200 dark:bg-white/10 dark:text-white/55 dark:hover:bg-white/15"
                    >
                        关闭
                    </button>
                </div>
                <form onSubmit={handleCrudSubmit} className="space-y-3">
                    {crudAction === 'import' && (
                        <div>
                            <label className="mb-1.5 block text-xs font-semibold text-zinc-500 dark:text-zinc-400">私钥 (Hex)</label>
                            <input
                                type="text"
                                value={crudForm.privateKey}
                                onChange={(e) => setCrudForm({ ...crudForm, privateKey: e.target.value })}
                                className={inputClass}
                                placeholder="输入私钥..."
                                required
                            />
                        </div>
                    )}
                    <div>
                        <label className="mb-1.5 block text-xs font-semibold text-zinc-500 dark:text-zinc-400">钱包名称</label>
                        <input
                            type="text"
                            value={crudForm.name}
                            onChange={(e) => setCrudForm({ ...crudForm, name: e.target.value })}
                            className={inputClass}
                            placeholder="如: 常用钱包1"
                            required
                        />
                    </div>
                    <div className="flex justify-end gap-2 pt-2">
                        <button
                            type="button"
                            onClick={() => setCrudAction(null)}
                            className="rounded-2xl px-4 py-3 text-xs font-bold text-zinc-500 hover:bg-zinc-100 dark:hover:bg-white/5"
                        >
                            取消
                        </button>
                        <button
                            type="submit"
                            disabled={crudLoading}
                            className={`rounded-2xl px-5 py-3 text-xs font-black shadow-sm transition-all ${crudLoading ? 'cursor-not-allowed opacity-50' : ''} ${brand.solidButtonClass}`}
                        >
                            {crudLoading ? '处理中...' : '确定'}
                        </button>
                    </div>
                </form>
            </div>
        );
    };

    const footer = (
        <button
            type="button"
            onClick={load}
            disabled={loading}
            className={`flex w-full items-center justify-center gap-2 rounded-2xl px-4 py-3.5 text-sm font-black shadow-sm transition-all ${loading ? 'cursor-not-allowed opacity-50' : ''} ${brand.solidButtonClass}`}
        >
            <RefreshCw className={`h-4 w-4 ${loading ? 'animate-spin' : ''}`} />
            {loading ? '刷新中...' : '刷新余额'}
        </button>
    );

    return (
        <WalletFrame
            embedded={embedded}
            open={open}
            onClose={onClose}
            title="钱包管理"
            maxHeightClass="max-h-[92vh]"
            contentClassName="px-5 pb-0 sm:pb-0"
            footer={footer}
        >
            <section className="relative mb-4 overflow-hidden rounded-[32px] border border-zinc-200/70 bg-zinc-950 p-5 text-white shadow-[0_24px_60px_rgba(15,23,42,0.24)] dark:border-white/[0.1]">
                <div className={`absolute -right-8 -top-10 h-36 w-36 rounded-full blur-2xl ${brand.dotClass} opacity-40`} />
                <div className="absolute -bottom-14 left-4 h-32 w-32 rounded-full bg-sky-400/25 blur-3xl" />
                <div className="relative">
                    <div className="flex items-start justify-between gap-3">
                        <div className="flex min-w-0 gap-3">
                            <div className="inline-flex h-12 w-12 shrink-0 items-center justify-center rounded-3xl bg-white/12 text-white ring-1 ring-white/15">
                                <WalletCards className="h-5 w-5" />
                            </div>
                            <div className="min-w-0">
                                <div className="text-[10px] font-black uppercase tracking-[0.28em] text-white/45">VAULTS</div>
                                <div className="mt-1 text-2xl font-black tracking-tight">{wallets.length} 个钱包</div>
                                <div className="mt-1 truncate text-xs text-white/55">
                                    默认：{defaultWallet?.name || shortAddress(defaultWallet?.address) || '未设置'}
                                </div>
                            </div>
                        </div>
                        {multiChainEnabled && (
                            <div className="w-28 shrink-0">
                                <CustomSelect value={chain} onChange={setChain} options={CHAIN_OPTIONS} placeholder="选择链" />
                            </div>
                        )}
                    </div>
                    <div className="mt-5 grid grid-cols-2 gap-2">
                        <WalletMetric label={`${nativeSymbol} 总计`} value={totalNative.toFixed(4)} />
                        <WalletMetric label={`${stableSymbol} 总计`} value={totalStable.toFixed(2)} />
                    </div>
                </div>
            </section>

            {error && (
                <div className="mb-4 rounded-xl border border-red-500/30 bg-red-500/10 p-3 text-xs text-red-700 dark:text-red-300">
                    {error}
                </div>
            )}

            {/* Actions Top */}
            {!crudAction && (
                <div className="mb-4 grid grid-cols-2 gap-2">
                    <button
                        onClick={() => { setCrudAction('create'); setCrudForm({ name: '', privateKey: '', walletId: null }); }}
                        className="group rounded-[24px] border border-zinc-200/80 bg-white/90 p-4 text-left shadow-sm transition hover:-translate-y-0.5 hover:shadow-md dark:border-white/10 dark:bg-white/[0.04]"
                    >
                        <span className={`mb-3 inline-flex h-10 w-10 items-center justify-center rounded-2xl ${brand.iconChipClass}`}>
                            <Plus className="h-[18px] w-[18px]" />
                        </span>
                        <span className="block text-sm font-black text-zinc-900 dark:text-white/90">创建新钱包</span>
                        <span className="mt-1 block text-[11px] leading-4 text-zinc-500 dark:text-white/42">自动生成并加入当前账户</span>
                    </button>
                    <button
                        onClick={() => { setCrudAction('import'); setCrudForm({ name: '', privateKey: '', walletId: null }); }}
                        className="group rounded-[24px] border border-zinc-200/80 bg-white/90 p-4 text-left shadow-sm transition hover:-translate-y-0.5 hover:shadow-md dark:border-white/10 dark:bg-white/[0.04]"
                    >
                        <span className="mb-3 inline-flex h-10 w-10 items-center justify-center rounded-2xl bg-amber-500/10 text-amber-700 ring-1 ring-amber-500/20 dark:bg-amber-500/15 dark:text-amber-200 dark:ring-amber-500/25">
                            <KeyRound className="h-[18px] w-[18px]" />
                        </span>
                        <span className="block text-sm font-black text-zinc-900 dark:text-white/90">导入钱包</span>
                        <span className="mt-1 block text-[11px] leading-4 text-zinc-500 dark:text-white/42">用私钥添加已有地址</span>
                    </button>
                </div>
            )}

            {renderCrudForm()}

            {loading && !wallets.length ? (
                <div className="flex items-center justify-center py-12 text-sm text-zinc-400 dark:text-white/40">
                    <svg className="mr-2 h-5 w-5 animate-spin" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                        <circle className="opacity-25" cx="12" cy="12" r="10" /><path className="opacity-75" d="M4 12a8 8 0 018-8" />
                    </svg>
                    加载中...
                </div>
            ) : wallets.length === 0 ? (
                <div className="rounded-[28px] border border-dashed border-zinc-200 bg-white/70 px-5 py-10 text-center text-sm text-zinc-400 dark:border-white/[0.08] dark:bg-white/[0.02] dark:text-white/40">
                    <WalletCards className="mx-auto mb-3 h-9 w-9 opacity-45" />
                    <div className="font-bold text-zinc-700 dark:text-white/65">暂无钱包</div>
                    <div className="mt-1 text-xs">先创建或导入一个钱包，就能在这里看余额和默认状态。</div>
                </div>
            ) : (
                <div className="space-y-3 pb-4">
                    {wallets.map((w) => (
                        <WalletCard
                            key={w.id || w.address}
                            wallet={w}
                            nativeSymbol={nativeSymbol}
                            stableSymbol={stableSymbol}
                            copied={copiedAddr === w.address}
                            onCopy={copyAddress}
                            onAction={handleAction}
                        />
                    ))}
                </div>
            )}

            <ConfirmDialog
                open={Boolean(deleteTarget)}
                title="删除钱包"
                message={`确定要删除钱包 ${deleteTarget?.name || shortAddress(deleteTarget?.address)} 吗？`}
                confirmText="删除"
                cancelText="取消"
                danger
                loading={loading}
                onConfirm={confirmDeleteWallet}
                onCancel={() => setDeleteTarget(null)}
            />
            {embedded ? (
                <div className="sticky bottom-[calc(76px+env(safe-area-inset-bottom,0px))] -mx-1 border-t border-zinc-200/70 bg-white/[0.88] px-1 pb-2 pt-3 backdrop-blur-xl dark:border-white/[0.08] dark:bg-[#111318]/90">
                    {footer}
                </div>
            ) : null}
        </WalletFrame>
    );
}

function WalletFrame({ embedded, children, footer, ...sheetProps }) {
    if (embedded) {
        return <div className="space-y-4 pb-1">{children}</div>;
    }
    return <BottomSheet {...sheetProps} footer={footer}>{children}</BottomSheet>;
}

function WalletMetric({ label, value }) {
    return (
        <div className="rounded-2xl bg-white/10 px-3 py-3 ring-1 ring-white/10">
            <div className="text-[10px] font-bold uppercase tracking-[0.14em] text-white/45">{label}</div>
            <div className="mt-1 truncate text-lg font-black tabular-nums text-white">{value}</div>
        </div>
    );
}

function WalletCard({ wallet, nativeSymbol, stableSymbol, copied, onCopy, onAction }) {
    const actionClass = 'inline-flex items-center justify-center gap-1.5 rounded-xl bg-zinc-100 px-3 py-2 text-[11px] font-bold text-zinc-600 transition-colors hover:bg-zinc-200 dark:bg-white/5 dark:text-white/60 dark:hover:bg-white/10 dark:hover:text-white/80';

    return (
        <div className="overflow-hidden rounded-[28px] border border-zinc-200/70 bg-white/[0.92] p-4 shadow-[0_14px_34px_rgba(15,23,42,0.06)] transition hover:-translate-y-0.5 hover:shadow-[0_20px_44px_rgba(15,23,42,0.1)] dark:border-white/[0.08] dark:bg-white/[0.035] dark:hover:shadow-none">
            <div className="flex items-start gap-3">
                <div className={`flex h-11 w-11 shrink-0 items-center justify-center rounded-2xl ${wallet.is_default ? 'bg-emerald-500/12 text-emerald-700 ring-1 ring-emerald-500/20 dark:text-emerald-300' : 'bg-zinc-100 text-zinc-500 ring-1 ring-zinc-200 dark:bg-white/[0.06] dark:text-white/50 dark:ring-white/[0.08]'}`}>
                    {wallet.is_default ? <ShieldCheck className="h-5 w-5" /> : <WalletCards className="h-5 w-5" />}
                </div>
                <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-2">
                        <span className="truncate text-sm font-black text-zinc-900 dark:text-white/90">
                            {wallet.name || `钱包 ${wallet.id}`}
                        </span>
                        {wallet.is_default ? (
                            <span className="inline-flex shrink-0 items-center gap-1 rounded-full bg-emerald-500/10 px-2 py-0.5 text-[9px] font-black text-emerald-600 ring-1 ring-emerald-500/20 dark:text-emerald-400">
                                <Star className="h-3 w-3 fill-current" />
                                默认
                            </span>
                        ) : null}
                    </div>
                    <button
                        type="button"
                        onClick={() => onCopy(wallet.address)}
                        className="mt-1 inline-flex max-w-full items-center gap-1.5 text-left font-mono text-[11px] text-zinc-400 transition-colors hover:text-zinc-600 dark:text-white/30 dark:hover:text-white/60"
                        title="点击复制"
                    >
                        <span className="truncate">{shortAddress(wallet.address)}</span>
                        {copied ? <CheckCircle2 className="h-3.5 w-3.5 shrink-0 text-emerald-500" /> : <Copy className="h-3.5 w-3.5 shrink-0" />}
                    </button>
                </div>
            </div>

            <div className="mt-4 grid grid-cols-2 gap-2">
                <BalanceTile label={nativeSymbol} value={formatBalance(wallet.native_balance, 4)} />
                <BalanceTile label={stableSymbol} value={formatBalance(wallet.stable_balance, 2)} emphasis />
            </div>

            <div className="mt-4 flex items-center gap-2 border-t border-zinc-100 pt-3 dark:border-white/5">
                {!wallet.is_default ? (
                    <button type="button" onClick={() => onAction('set_default', wallet)} className={actionClass}>
                        <Star className="h-3.5 w-3.5" />
                        默认
                    </button>
                ) : null}
                <button type="button" onClick={() => onAction('rename', wallet)} className={actionClass}>
                    <Pencil className="h-3.5 w-3.5" />
                    重命名
                </button>
                <button
                    type="button"
                    onClick={() => onAction('delete', wallet)}
                    className="ml-auto inline-flex items-center justify-center gap-1.5 rounded-xl px-3 py-2 text-[11px] font-bold text-red-500 transition-colors hover:bg-red-50 dark:text-red-400 dark:hover:bg-red-500/10"
                >
                    <Trash2 className="h-3.5 w-3.5" />
                    删除
                </button>
            </div>
        </div>
    );
}

function BalanceTile({ label, value, emphasis = false }) {
    return (
        <div className={`rounded-2xl px-3 py-3 ring-1 ${emphasis ? 'bg-emerald-500/[0.06] ring-emerald-500/15 dark:bg-emerald-500/[0.08]' : 'bg-zinc-50 ring-zinc-200 dark:bg-black/20 dark:ring-white/[0.06]'}`}>
            <div className={`text-[10px] font-black uppercase tracking-[0.14em] ${emphasis ? 'text-emerald-600 dark:text-emerald-300' : 'text-zinc-400 dark:text-white/30'}`}>{label}</div>
            <div className="mt-1 truncate text-sm font-black tabular-nums text-zinc-900 dark:text-white/85">{value}</div>
        </div>
    );
}
