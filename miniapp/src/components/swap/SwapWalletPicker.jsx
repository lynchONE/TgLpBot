import { Check, Wallet } from 'lucide-react';
import BottomSheet from '../BottomSheet.jsx';
import { formatTokenAmount, shortAddress } from '../../lib/swapMeta';

export default function SwapWalletPicker({
    open,
    onClose,
    wallets = [],
    selectedWalletId,
    nativeSymbol = 'BNB',
    onSelect,
}) {
    const handlePick = (wallet) => {
        if (!wallet) return;
        onSelect?.(wallet);
        onClose?.();
    };

    return (
        <BottomSheet open={open} onClose={onClose} title="选择钱包" maxHeightClass="max-h-[75vh]">
            <div className="space-y-1.5">
                {wallets.length === 0 ? (
                    <div className="rounded-2xl border border-dashed border-zinc-200 px-3 py-8 text-center text-xs text-zinc-400 dark:border-white/10 dark:text-white/35">
                        当前链下没有可用钱包
                    </div>
                ) : null}
                {wallets.map((w) => {
                    const isActive = String(w.id) === String(selectedWalletId);
                    return (
                        <button
                            key={w.id}
                            type="button"
                            onClick={() => handlePick(w)}
                            className={`flex w-full items-center gap-3 rounded-2xl border px-3 py-3 text-left transition ${
                                isActive
                                    ? 'border-zinc-900 bg-zinc-900 text-white dark:border-white dark:bg-white dark:text-zinc-900'
                                    : 'border-zinc-200 bg-white hover:bg-zinc-50 dark:border-white/10 dark:bg-white/5 dark:hover:bg-white/10'
                            }`}
                        >
                            <span
                                className={`flex h-9 w-9 shrink-0 items-center justify-center rounded-full ${
                                    isActive
                                        ? 'bg-white/15 text-white dark:bg-zinc-900/10 dark:text-zinc-900'
                                        : 'bg-zinc-100 text-zinc-500 dark:bg-white/10 dark:text-white/55'
                                }`}
                            >
                                <Wallet size={16} />
                            </span>
                            <div className="min-w-0 flex-1">
                                <div className="flex items-center gap-1.5">
                                    <span className="truncate text-[13px] font-bold">
                                        {w.name || `钱包 ${w.id}`}
                                    </span>
                                    {w.is_default ? (
                                        <span
                                            className={`rounded px-1 py-0.5 text-[8px] font-bold uppercase ${
                                                isActive
                                                    ? 'bg-white/20 dark:bg-zinc-900/10'
                                                    : 'bg-zinc-100 text-zinc-500 dark:bg-white/10 dark:text-white/55'
                                            }`}
                                        >
                                            默认
                                        </span>
                                    ) : null}
                                </div>
                                <div className={`mt-0.5 truncate font-mono text-[10px] ${isActive ? 'opacity-70' : 'text-zinc-400 dark:text-white/35'}`}>
                                    {shortAddress(w.address, 8, 6)}
                                </div>
                            </div>
                            <div className="shrink-0 text-right">
                                <div className="text-[13px] font-bold tabular-nums">
                                    {formatTokenAmount(w.native_balance)}
                                </div>
                                <div className={`text-[9px] uppercase tracking-wider ${isActive ? 'opacity-70' : 'text-zinc-400 dark:text-white/30'}`}>
                                    {nativeSymbol}
                                </div>
                            </div>
                            {isActive ? <Check size={14} className="shrink-0" /> : null}
                        </button>
                    );
                })}
            </div>
        </BottomSheet>
    );
}
