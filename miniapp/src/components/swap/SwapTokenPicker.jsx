import { useEffect, useMemo, useState } from 'react';
import { Search, Star } from 'lucide-react';
import BottomSheet from '../BottomSheet.jsx';
import {
    applyTokenMetadata,
    dedupeTokens,
    formatTokenAmount,
    formatUSDCompact,
    getPresetTokens,
    getRecentTokensFor,
    makeCustomToken,
    matchesToken,
    normalizeHex,
    shortAddress,
} from '../../lib/swapMeta';

function TokenLogo({ token, size = 28 }) {
    const symbol = String(token?.symbol || '?').trim();
    const color = String(token?.color || '#7c8aa6').trim();
    const logo = String(token?.logoUrl || '').trim();
    const [failed, setFailed] = useState(false);

    useEffect(() => { setFailed(false); }, [logo, token?.address]);

    if (logo && !failed) {
        return (
            <img
                src={logo}
                alt={symbol}
                onError={() => setFailed(true)}
                className="rounded-full bg-zinc-100 object-cover dark:bg-white/10"
                style={{ width: size, height: size }}
            />
        );
    }
    return (
        <span
            className="inline-flex items-center justify-center rounded-full text-[10px] font-bold text-white"
            style={{ width: size, height: size, background: color }}
        >
            {symbol.slice(0, 1).toUpperCase()}
        </span>
    );
}

function TokenRow({ token, balance, valueUSD, onClick, disabled, disabledReason }) {
    return (
        <button
            type="button"
            onClick={onClick}
            disabled={disabled}
            className={`flex w-full items-center gap-3 rounded-2xl px-3 py-2.5 text-left transition ${
                disabled
                    ? 'cursor-not-allowed opacity-40'
                    : 'hover:bg-zinc-100 active:scale-[0.99] dark:hover:bg-white/5'
            }`}
        >
            <TokenLogo token={token} size={36} />
            <div className="min-w-0 flex-1">
                <div className="flex items-center gap-1.5">
                    <span className="truncate text-[14px] font-bold text-zinc-900 dark:text-white/90">
                        {token?.symbol || '--'}
                    </span>
                    {token?.native ? (
                        <span className="rounded bg-zinc-900 px-1 py-0.5 text-[8px] font-bold text-white dark:bg-white dark:text-zinc-900">
                            NATIVE
                        </span>
                    ) : null}
                </div>
                <div className="truncate text-[10px] text-zinc-500 dark:text-white/40">
                    {token?.custom ? shortAddress(token.address, 8, 6) : (token?.name || '--')}
                </div>
                {disabled && disabledReason ? (
                    <div className="mt-0.5 text-[10px] text-amber-600 dark:text-amber-400">
                        {disabledReason}
                    </div>
                ) : null}
            </div>
            {balance ? (
                <div className="shrink-0 text-right">
                    <div className="text-[13px] font-semibold tabular-nums text-zinc-900 dark:text-white/85">
                        {formatTokenAmount(balance)}
                    </div>
                    {valueUSD ? (
                        <div className="text-[10px] tabular-nums text-zinc-400 dark:text-white/35">
                            {formatUSDCompact(valueUSD)}
                        </div>
                    ) : null}
                </div>
            ) : null}
        </button>
    );
}

function SectionLabel({ children }) {
    return (
        <div className="px-1 pt-3 pb-1.5 text-[10px] font-bold uppercase tracking-[0.18em] text-zinc-400 dark:text-white/35">
            {children}
        </div>
    );
}

export default function SwapTokenPicker({
    open,
    onClose,
    chain,
    walletTokens = [],
    tokenMetaMap = {},
    onSelect,
    excludeAddress,
}) {
    const [query, setQuery] = useState('');

    useEffect(() => {
        if (!open) setQuery('');
    }, [open]);

    const presets = useMemo(
        () => getPresetTokens(chain).map((t) => applyTokenMetadata(t, tokenMetaMap, chain)),
        [chain, tokenMetaMap],
    );
    const recents = useMemo(
        () => (open
            ? getRecentTokensFor(chain).map((t) => applyTokenMetadata(t, tokenMetaMap, chain))
            : []),
        [open, chain, tokenMetaMap],
    );

    const excludeNorm = normalizeHex(excludeAddress);
    const filterExclude = (t) => !excludeNorm || normalizeHex(t?.address) !== excludeNorm;

    const filteredHolding = useMemo(
        () => dedupeTokens(walletTokens).filter((t) => matchesToken(t, query) && filterExclude(t)),
        [walletTokens, query, excludeNorm],
    );
    const filteredPresets = useMemo(
        () => presets.filter((t) => matchesToken(t, query) && filterExclude(t)),
        [presets, query, excludeNorm],
    );
    const filteredRecents = useMemo(
        () => recents.filter((t) => matchesToken(t, query) && filterExclude(t)),
        [recents, query, excludeNorm],
    );

    const queryNormalized = String(query || '').trim().toLowerCase();
    const looksLikeAddress = /^0x[0-9a-f]{40}$/.test(queryNormalized);
    const customCandidate = looksLikeAddress
        ? applyTokenMetadata(makeCustomToken(queryNormalized), tokenMetaMap, chain)
        : null;

    const handlePick = (token) => {
        if (!token) return;
        onSelect?.(token);
        onClose?.();
    };

    return (
        <BottomSheet
            open={open}
            onClose={onClose}
            title="选择代币"
            maxHeightClass="max-h-[90vh]"
        >
            <div className="space-y-2">
                <div className="flex items-center gap-2 rounded-2xl border border-zinc-200 bg-zinc-50 px-3 py-2.5 dark:border-white/10 dark:bg-white/5">
                    <Search size={16} className="text-zinc-400 dark:text-white/40" />
                    <input
                        autoFocus
                        value={query}
                        onChange={(e) => setQuery(e.target.value)}
                        placeholder="搜索 symbol / 名称 / 合约地址 (0x...)"
                        className="flex-1 bg-transparent text-[13px] text-zinc-900 placeholder:text-zinc-400 outline-none dark:text-white/90 dark:placeholder:text-white/35"
                    />
                </div>

                {customCandidate && filteredPresets.length === 0 && filteredHolding.length === 0 && filteredRecents.length === 0 ? (
                    <>
                        <SectionLabel>自定义合约地址</SectionLabel>
                        <TokenRow token={customCandidate} onClick={() => handlePick(customCandidate)} />
                    </>
                ) : null}

                {filteredHolding.length > 0 ? (
                    <>
                        <SectionLabel>钱包持有</SectionLabel>
                        {filteredHolding.map((t) => (
                            <TokenRow
                                key={`hold:${t.address}`}
                                token={t}
                                balance={t.balance}
                                valueUSD={t.valueUSDT}
                                disabled={t.canSwap === false}
                                disabledReason={t.disabledReason}
                                onClick={() => handlePick(t)}
                            />
                        ))}
                    </>
                ) : null}

                {filteredRecents.length > 0 ? (
                    <>
                        <SectionLabel>最近用过</SectionLabel>
                        {filteredRecents.map((t) => (
                            <TokenRow
                                key={`recent:${t.address}`}
                                token={t}
                                onClick={() => handlePick(t)}
                            />
                        ))}
                    </>
                ) : null}

                {filteredPresets.length > 0 ? (
                    <>
                        <SectionLabel>
                            <span className="inline-flex items-center gap-1.5">
                                <Star size={10} className="opacity-50" />
                                常用代币
                            </span>
                        </SectionLabel>
                        {filteredPresets.map((t) => (
                            <TokenRow
                                key={`preset:${t.address}`}
                                token={t}
                                onClick={() => handlePick(t)}
                            />
                        ))}
                    </>
                ) : null}

                {!customCandidate && filteredHolding.length === 0 && filteredRecents.length === 0 && filteredPresets.length === 0 ? (
                    <div className="rounded-2xl border border-dashed border-zinc-200 px-3 py-8 text-center text-xs text-zinc-400 dark:border-white/10 dark:text-white/35">
                        无匹配代币。粘贴 0x 开头的合约地址可使用自定义代币。
                    </div>
                ) : null}
            </div>
        </BottomSheet>
    );
}
