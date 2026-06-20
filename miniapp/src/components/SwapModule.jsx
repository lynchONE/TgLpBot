import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { ArrowDown, ChevronDown, History, RefreshCw, Settings2, Wallet } from 'lucide-react';
import ModuleHeader from './ModuleHeader.jsx';
import ConfirmDialog from './ConfirmDialog.jsx';
import NumberFlowValue from './NumberFlowValue.jsx';
import SwapTokenPicker from './swap/SwapTokenPicker.jsx';
import SwapWalletPicker from './swap/SwapWalletPicker.jsx';
import SwapQuoteDetails from './swap/SwapQuoteDetails.jsx';
import SwapHistoryDrawer from './swap/SwapHistoryDrawer.jsx';
import {
    createWalletSwapLimitOrder,
    fetchWallets,
    fetchWalletSwapTokenMetadata,
    walletSwapPreview,
    walletSwapSingleExecute,
    walletSwapSingleQuote,
} from '../lib/api';
import { AMOUNT_PRESETS, AUTO_QUOTE_REFRESH_MS, CHAIN_META, MIN_WALLET_TOKEN_VALUE_USD, SLIPPAGE_PRESETS, applyAmountPreset, applyTokenMetadata, formatQuoteGasCostSummary, formatQuoteRelativeTime, formatTokenAmount, formatUSDCompact, getChainConfig, getNativePresetToken, getPresetTokens, normalizeHex, pushRecentToken, shortAddress, shouldFetchTokenMetadata } from '../lib/swapMeta';
const CHAIN_OPTIONS = Object.values(CHAIN_META).map((c) => ({
    key: c.key,
    label: c.label,
    short: c.shortLabel,
    emoji: c.emoji,
}));

const SWAP_MODES = [
    { key: 'market', label: '市价兑换' },
    { key: 'limit', label: '限价单' },
];

function TokenChip({ token, onClick, placeholder = '选择代币' }) {
    const [imgFailed, setImgFailed] = useState(false);
    const logo = String(token?.logoUrl || '').trim();
    useEffect(() => { setImgFailed(false); }, [logo, token?.address]);

    if (!token) {
        return (
            <button
                type="button"
                onClick={onClick}
                className="inline-flex items-center gap-1.5 rounded-full bg-zinc-900 px-3 py-1.5 text-[12px] font-bold text-white dark:bg-white dark:text-zinc-900"
            >
                {placeholder}
                <ChevronDown size={14} />
            </button>
        );
    }
    const symbol = String(token.symbol || '?').slice(0, 1).toUpperCase();
    const color = token.color || '#7c8aa6';
    return (
        <button
            type="button"
            onClick={onClick}
            className="inline-flex items-center gap-1.5 rounded-full border border-zinc-200 bg-white px-2 py-1 text-[12px] font-bold text-zinc-900 transition active:scale-[0.98] dark:border-white/10 dark:bg-white/10 dark:text-white/90"
        >
            {logo && !imgFailed ? (
                <img
                    src={logo}
                    alt={token.symbol || ''}
                    onError={() => setImgFailed(true)}
                    className="h-6 w-6 rounded-full bg-zinc-100 object-cover dark:bg-white/10"
                />
            ) : (
                <span
                    className="flex h-6 w-6 items-center justify-center rounded-full text-[10px] font-bold text-white"
                    style={{ background: color }}
                >
                    {symbol}
                </span>
            )}
            <span className="truncate max-w-[80px]">{token.symbol}</span>
            <ChevronDown size={14} className="text-zinc-400 dark:text-white/45" />
        </button>
    );
}

function ChainPill({ option, active, onClick }) {
    return (
        <button
            type="button"
            onClick={onClick}
            className={`inline-flex items-center gap-1.5 rounded-full border px-3 py-1.5 text-[12px] font-semibold transition active:scale-[0.98] ${
                active
                    ? 'border-zinc-900 bg-zinc-900 text-white dark:border-white dark:bg-white dark:text-zinc-900'
                    : 'border-zinc-200 bg-white text-zinc-600 hover:bg-zinc-100 dark:border-white/10 dark:bg-white/5 dark:text-white/65 dark:hover:bg-white/10'
            }`}
        >
            <span className="text-[14px]">{option.emoji}</span>
            <span>{option.short}</span>
        </button>
    );
}

function SegmentButton({ active, children, onClick }) {
    return (
        <button
            type="button"
            onClick={onClick}
            className={`flex-1 rounded-xl py-2 text-[13px] font-bold transition ${
                active
                    ? 'bg-zinc-900 text-white shadow-sm dark:bg-white dark:text-zinc-900'
                    : 'text-zinc-500 hover:text-zinc-900 dark:text-white/55 dark:hover:text-white/90'
            }`}
        >
            {children}
        </button>
    );
}

function PresetAmountButton({ ratio, label, onClick, disabled }) {
    return (
        <button
            type="button"
            onClick={() => onClick?.(ratio)}
            disabled={disabled}
            className={`rounded-full px-2.5 py-0.5 text-[10px] font-bold transition ${
                disabled
                    ? 'bg-zinc-100 text-zinc-300 dark:bg-white/5 dark:text-white/20'
                    : 'bg-zinc-100 text-zinc-600 hover:bg-zinc-200 active:bg-zinc-900 active:text-white dark:bg-white/5 dark:text-white/65 dark:hover:bg-white/10'
            }`}
        >
            {label}
        </button>
    );
}

export default function SwapModule({
    apiBaseUrl,
    initData,
    hasInitData,
    onNotice,
    pollIntervalSec = 8,
    multiChainEnabled = true,
}) {
    /* core state */
    const [mode, setMode] = useState('market');
    const [chain, setChain] = useState('bsc');
    const [slippage, setSlippage] = useState('1.0');

    const [wallets, setWallets] = useState([]);
    const [walletLoading, setWalletLoading] = useState(false);
    const [selectedWalletId, setSelectedWalletId] = useState('');

    const [walletTokens, setWalletTokens] = useState([]);
    const [, setWalletTokensLoading] = useState(false);
    const [walletTokensError, setWalletTokensError] = useState('');

    const [fromToken, setFromToken] = useState(() => getNativePresetToken('bsc'));
    const [toToken, setToToken] = useState(() => getPresetTokens('bsc')[1] || null);
    const [amount, setAmount] = useState('');

    /* limit-order state */
    const [limitMode, setLimitMode] = useState('to_amount'); // 'to_amount' | 'price'
    const [limitTargetAmount, setLimitTargetAmount] = useState('');
    const [limitTargetPrice, setLimitTargetPrice] = useState('');

    /* quote state */
    const [quote, setQuote] = useState(null);
    const [quoting, setQuoting] = useState(false);
    const [quoteError, setQuoteError] = useState('');
    const [lastQuoteAt, setLastQuoteAt] = useState(0);

    /* execution state */
    const [executing, setExecuting] = useState(false);
    const [execError, setExecError] = useState('');
    const [confirmOpen, setConfirmOpen] = useState(false);

    /* drawers */
    const [pickerSide, setPickerSide] = useState(null); // 'from' | 'to' | null
    const [walletPickerOpen, setWalletPickerOpen] = useState(false);
    const [quoteDetailsOpen, setQuoteDetailsOpen] = useState(false);
    const [historyOpen, setHistoryOpen] = useState(false);
    const [showSlippage, setShowSlippage] = useState(false);

    /* token metadata enrichment (logo / canonical symbol / name) */
    const [tokenMetaMap, setTokenMetaMap] = useState({});

    const debounceRef = useRef(null);
    const refreshTimerRef = useRef(null);
    const walletsAbortRef = useRef(null);
    const walletsSeqRef = useRef(0);
    const walletTokensAbortRef = useRef(null);
    const walletTokensSeqRef = useRef(0);
    const onNoticeRef = useRef(onNotice);
    const [walletTokensKey, setWalletTokensKey] = useState('');
    const [tick, setTick] = useState(0);

    useEffect(() => {
        onNoticeRef.current = onNotice;
    }, [onNotice]);

    /* chain change → reset tokens to defaults */
    useEffect(() => {
        const native = getNativePresetToken(chain);
        const stable = getPresetTokens(chain).find((t) => !t.native);
        setFromToken(native);
        setToToken(stable || null);
        setAmount('');
        setQuote(null);
        setQuoteError('');
        setExecError('');
        setSelectedWalletId('');
        setWalletTokens([]);
        setWalletTokensKey('');
        setTokenMetaMap({});
    }, [chain]);

    /* clock for "x秒前" hint */
    useEffect(() => {
        const id = setInterval(() => setTick(Date.now()), 1000);
        return () => clearInterval(id);
    }, []);

    const chainConfig = useMemo(() => getChainConfig(chain), [chain]);
    const nativeSymbol = chainConfig.nativeSymbol;
    const currentWalletTokensKey = useMemo(
        () => (selectedWalletId ? `${chain}:${selectedWalletId}` : ''),
        [chain, selectedWalletId],
    );
    const walletTokensReady = walletTokensKey === currentWalletTokensKey;

    const loadWallets = useCallback(async () => {
        if (!hasInitData) return;
        const seq = walletsSeqRef.current + 1;
        walletsSeqRef.current = seq;
        if (walletsAbortRef.current) {
            walletsAbortRef.current.abort();
        }
        const controller = new AbortController();
        walletsAbortRef.current = controller;
        setWalletLoading(true);
        try {
            const resp = await fetchWallets({
                apiBaseUrl,
                initData,
                chain,
                signal: controller.signal,
            });
            if (controller.signal.aborted || walletsSeqRef.current !== seq) return;
            if (!Array.isArray(resp.wallets)) {
                throw new Error('钱包列表响应格式错误');
            }
            const list = resp.wallets;
            setWallets(list);
            setSelectedWalletId((current) => {
                if (list.some((item) => String(item.id) === String(current))) return current;
                const def = list.find((item) => item.is_default) || list[0];
                return def ? String(def.id) : '';
            });
        } catch (e) {
            if (controller.signal.aborted || walletsSeqRef.current !== seq) return;
            setWallets([]);
            setSelectedWalletId('');
            onNoticeRef.current?.(String(e?.message || e));
        } finally {
            if (walletsAbortRef.current === controller) {
                walletsAbortRef.current = null;
            }
            if (walletsSeqRef.current === seq) {
                setWalletLoading(false);
            }
        }
    }, [apiBaseUrl, initData, chain, hasInitData]);

    const loadWalletTokens = useCallback(async () => {
        if (!hasInitData || !selectedWalletId) {
            if (walletTokensAbortRef.current) {
                walletTokensAbortRef.current.abort();
                walletTokensAbortRef.current = null;
            }
            setWalletTokens([]);
            setWalletTokensKey('');
            return;
        }
        const requestKey = `${chain}:${selectedWalletId}`;
        const seq = walletTokensSeqRef.current + 1;
        walletTokensSeqRef.current = seq;
        if (walletTokensAbortRef.current) {
            walletTokensAbortRef.current.abort();
        }
        const controller = new AbortController();
        walletTokensAbortRef.current = controller;
        setWalletTokensLoading(true);
        setWalletTokensError('');
        try {
            const resp = await walletSwapPreview({
                apiBaseUrl,
                initData,
                chain,
                walletId: selectedWalletId,
                minValueUsd: MIN_WALLET_TOKEN_VALUE_USD,
                signal: controller.signal,
            });
            if (controller.signal.aborted || walletTokensSeqRef.current !== seq) return;
            const list = (resp?.tokens || []).map((t) => ({
                address: normalizeHex(t.address) || String(t.address || '').toLowerCase(),
                symbol: t.symbol,
                name: t.name || t.symbol,
                balance: t.balance,
                balanceRaw: t.balance_raw || '',
                decimals: Number.isFinite(Number(t.decimals)) ? Number(t.decimals) : null,
                valueUSDT: t.value_usdt || 0,
                logoUrl: t.logo_url || '',
                native: Boolean(t.is_native),
                canSwap: t.can_swap !== false,
                disabledReason: t.disabled_reason || '',
            })).filter((t) => t.address);
            setWalletTokens(list);
            setWalletTokensKey(requestKey);
        } catch (e) {
            if (controller.signal.aborted || walletTokensSeqRef.current !== seq) return;
            setWalletTokensError(String(e?.message || e));
            setWalletTokens([]);
            setWalletTokensKey(requestKey);
        } finally {
            if (walletTokensAbortRef.current === controller) {
                walletTokensAbortRef.current = null;
            }
            if (walletTokensSeqRef.current === seq) {
                setWalletTokensLoading(false);
            }
        }
    }, [apiBaseUrl, initData, chain, hasInitData, selectedWalletId]);

    useEffect(() => { loadWallets(); }, [loadWallets]);
    useEffect(() => { loadWalletTokens(); }, [loadWalletTokens]);
    useEffect(() => () => {
        if (walletsAbortRef.current) {
            walletsAbortRef.current.abort();
        }
        if (walletTokensAbortRef.current) {
            walletTokensAbortRef.current.abort();
        }
    }, []);

    /* enriched token lists with logo / name from tokenMetaMap */
    const rawPresetTokens = useMemo(() => getPresetTokens(chain), [chain]);
    const enrichedWalletTokens = useMemo(
        () => (walletTokensReady ? walletTokens.map((t) => applyTokenMetadata(t, tokenMetaMap, chain)) : []),
        [walletTokens, tokenMetaMap, chain, walletTokensReady],
    );

    const fromTokenEnriched = useMemo(
        () => (fromToken ? applyTokenMetadata(fromToken, tokenMetaMap, chain) : null),
        [fromToken, tokenMetaMap, chain],
    );
    const toTokenEnriched = useMemo(
        () => (toToken ? applyTokenMetadata(toToken, tokenMetaMap, chain) : null),
        [toToken, tokenMetaMap, chain],
    );

    /* fetch metadata for any token missing logo / canonical name */
    useEffect(() => {
        if (!hasInitData) return undefined;
        const candidates = [
            ...rawPresetTokens,
            ...walletTokens,
            fromToken,
            toToken,
        ].filter(Boolean);
        const addresses = [];
        const seen = new Set();
        for (const token of candidates) {
            if (!shouldFetchTokenMetadata(token)) continue;
            const addr = normalizeHex(token.address);
            if (!addr || seen.has(addr) || tokenMetaMap[addr]) continue;
            seen.add(addr);
            addresses.push(addr);
        }
        if (!addresses.length) return undefined;

        const controller = new AbortController();
        fetchWalletSwapTokenMetadata({
            apiBaseUrl,
            initData,
            chain,
            addresses,
            signal: controller.signal,
        })
            .then((resp) => {
                if (controller.signal.aborted) return;
                const rows = Array.isArray(resp?.tokens) ? resp.tokens : [];
                if (!rows.length) return;
                setTokenMetaMap((prev) => {
                    const next = { ...prev };
                    for (const item of rows) {
                        const addr = normalizeHex(item?.address);
                        if (!addr) continue;
                        next[addr] = {
                            address: addr,
                            symbol: String(item?.symbol || '').trim(),
                            name: String(item?.name || '').trim(),
                            logoUrl: String(item?.logo_url || '').trim(),
                        };
                    }
                    return next;
                });
            })
            .catch((e) => {
                if (controller.signal.aborted) return;
                // 元数据是装饰性的，拉取失败不打扰用户，保留 fallback 字母圆
                console.warn('fetchWalletSwapTokenMetadata failed', e);
            });
        return () => controller.abort();
    }, [apiBaseUrl, initData, chain, hasInitData, rawPresetTokens, walletTokens, fromToken, toToken, tokenMetaMap]);

    const fromBalanceToken = useMemo(() => {
        if (!walletTokensReady) return null;
        if (!fromToken?.address) return null;
        const found = walletTokens.find((t) => t.address === normalizeHex(fromToken.address));
        return found || null;
    }, [walletTokens, fromToken, walletTokensReady]);

    const fromBalanceText = fromBalanceToken ? formatTokenAmount(fromBalanceToken.balance) : '--';
    const fromValueUsdText = fromBalanceToken && fromBalanceToken.valueUSDT
        ? formatUSDCompact(fromBalanceToken.valueUSDT)
        : '';

    /* auto quote */
    const doQuote = useCallback(async () => {
        const amt = String(amount || '').trim();
        const amtNum = Number(amt);
        if (!amt || !Number.isFinite(amtNum) || amtNum <= 0) {
            setQuote(null);
            setQuoteError('');
            return;
        }
        if (!fromToken?.address || !toToken?.address || !selectedWalletId) {
            return;
        }
        if (normalizeHex(fromToken.address) === normalizeHex(toToken.address)) {
            setQuoteError('支付与接收代币相同');
            setQuote(null);
            return;
        }
        setQuoting(true);
        setQuoteError('');
        try {
            const resp = await walletSwapSingleQuote({
                apiBaseUrl,
                initData,
                chain,
                walletId: selectedWalletId,
                fromToken: fromToken.address,
                toToken: toToken.address,
                amount: amt,
                slippagePercent: Number(slippage),
            });
            setQuote(resp);
            setLastQuoteAt(Date.now());
        } catch (e) {
            setQuoteError(String(e?.message || e));
            setQuote(null);
        } finally {
            setQuoting(false);
        }
    }, [amount, fromToken, toToken, selectedWalletId, apiBaseUrl, initData, chain, slippage]);

    /* debounce on inputs */
    useEffect(() => {
        if (debounceRef.current) clearTimeout(debounceRef.current);
        debounceRef.current = setTimeout(doQuote, 800);
        return () => clearTimeout(debounceRef.current);
    }, [doQuote]);

    /* periodic refresh */
    useEffect(() => {
        if (refreshTimerRef.current) clearInterval(refreshTimerRef.current);
        const cadence = Math.max(5, Number(pollIntervalSec) || 8) * 1000;
        refreshTimerRef.current = setInterval(() => {
            if (document.visibilityState === 'visible') doQuote();
        }, Math.max(cadence, AUTO_QUOTE_REFRESH_MS));
        return () => clearInterval(refreshTimerRef.current);
    }, [doQuote, pollIntervalSec]);

    const handleReverse = () => {
        const a = fromToken;
        setFromToken(toToken);
        setToToken(a);
        setAmount('');
        setQuote(null);
    };

    const handleSelectToken = (side, token) => {
        if (!token) return;
        if (side === 'from') {
            if (normalizeHex(token.address) === normalizeHex(toToken?.address)) {
                setToToken(fromToken);
            }
            setFromToken(token);
        } else {
            if (normalizeHex(token.address) === normalizeHex(fromToken?.address)) {
                setFromToken(toToken);
            }
            setToToken(token);
        }
        pushRecentToken(chain, token);
        setAmount('');
        setQuote(null);
    };

    const handleSelectWallet = (wallet) => {
        if (!wallet?.id) return;
        const nextId = String(wallet.id);
        setSelectedWalletId(nextId);
        setWalletTokens([]);
        setWalletTokensKey('');
        setAmount('');
        setQuote(null);
        setQuoteError('');
        setExecError('');
        setLastQuoteAt(0);
    };

    const handlePreset = (ratio) => {
        if (!fromBalanceToken?.balance) return;
        const next = applyAmountPreset(fromBalanceToken.balance, ratio);
        if (next) setAmount(next);
    };

    const selectedWallet = useMemo(
        () => wallets.find((w) => String(w.id) === String(selectedWalletId)),
        [wallets, selectedWalletId],
    );

    const sameToken =
        fromToken?.address &&
        toToken?.address &&
        normalizeHex(fromToken.address) === normalizeHex(toToken.address);

    const amountNumeric = Number(amount);
    const validAmount = Number.isFinite(amountNumeric) && amountNumeric > 0;

    const validLimit = mode === 'limit'
        ? (limitMode === 'price'
            ? String(limitTargetPrice || '').trim() !== ''
            : String(limitTargetAmount || '').trim() !== '')
        : true;

    const canSubmit =
        hasInitData &&
        !!selectedWalletId &&
        !!fromToken &&
        !!toToken &&
        !sameToken &&
        validAmount &&
        !!quote &&
        validLimit &&
        !executing &&
        !quoting;

    /* submit */
    const handleExecute = useCallback(async () => {
        setExecuting(true);
        setExecError('');
        try {
            if (mode === 'limit') {
                await createWalletSwapLimitOrder({
                    apiBaseUrl,
                    initData,
                    chain,
                    walletId: selectedWalletId,
                    fromToken: fromToken.address,
                    toToken: toToken.address,
                    amount: String(amount).trim(),
                    targetToAmount: limitMode === 'to_amount' ? limitTargetAmount : '',
                    targetPrice: limitMode === 'price' ? limitTargetPrice : '',
                    slippagePercent: Number(slippage),
                    provider: quote?.best_provider || quote?.provider || '',
                });
                onNotice?.('限价单已创建', 'success');
                setLimitTargetAmount('');
                setLimitTargetPrice('');
            } else {
                const resp = await walletSwapSingleExecute({
                    apiBaseUrl,
                    initData,
                    chain,
                    walletId: selectedWalletId,
                    fromToken: fromToken.address,
                    toToken: toToken.address,
                    amount: String(amount).trim(),
                    slippagePercent: Number(slippage),
                    provider: quote?.best_provider || quote?.provider || '',
                });
                const tx = resp?.tx_hash || '已提交';
                onNotice?.(`兑换已提交 ${tx.slice(0, 10)}…`, 'success');
            }
            setConfirmOpen(false);
            setAmount('');
            setQuote(null);
            // refresh balances after a moment
            setTimeout(loadWalletTokens, 1500);
        } catch (e) {
            const msg = String(e?.message || e);
            setExecError(msg);
            onNotice?.(msg, 'error');
        } finally {
            setExecuting(false);
        }
    }, [
        mode,
        apiBaseUrl,
        initData,
        chain,
        selectedWalletId,
        fromToken,
        toToken,
        amount,
        slippage,
        quote,
        limitMode,
        limitTargetAmount,
        limitTargetPrice,
        loadWalletTokens,
        onNotice,
    ]);

    const handleSlippageInput = (value) => {
        const sanitized = String(value || '').replace(/[^0-9.]/g, '');
        setSlippage(sanitized);
    };

    const submitText = executing
        ? '处理中…'
        : !hasInitData
            ? '未登录'
            : !selectedWalletId
                ? '选择钱包'
                : sameToken
                    ? '代币相同'
                    : !validAmount
                        ? '输入金额'
                        : quoting
                            ? '获取报价…'
                            : !quote
                                ? quoteError ? '无法兑换' : '等待报价'
                                : mode === 'limit'
                                    ? (validLimit ? '创建限价单' : '填写触发条件')
                                    : '确认兑换';

    const toAmountText = quoting && !quote
        ? '…'
        : quote?.to_amount_float || '0.0';

    return (
        <div className="mini-swap-module space-y-3">
            <ModuleHeader
                title="兑换"
                subtitle={selectedWallet
                    ? `${selectedWallet.name || '钱包'} · ${shortAddress(selectedWallet.address, 6, 4)}`
                    : (hasInitData ? '一键兑换 · 市价/限价' : '请先登录')}
                actions={(
                    <button
                        type="button"
                        onClick={() => setHistoryOpen(true)}
                        disabled={!hasInitData || !selectedWalletId}
                        className="inline-flex h-9 w-9 items-center justify-center rounded-2xl bg-white/70 text-zinc-700 ring-1 ring-zinc-200 transition hover:bg-white disabled:opacity-40 dark:bg-white/5 dark:text-white/70 dark:ring-white/10"
                        title="兑换记录"
                    >
                        <History size={16} />
                    </button>
                )}
            />

            {/* mode segment */}
            <div className="mini-swap-segment flex gap-1 rounded-2xl border border-zinc-200 bg-white/70 p-1 shadow-sm dark:border-white/10 dark:bg-[#0f1116]/80 dark:shadow-none">
                {SWAP_MODES.map((m) => (
                    <SegmentButton key={m.key} active={mode === m.key} onClick={() => setMode(m.key)}>
                        {m.label}
                    </SegmentButton>
                ))}
            </div>

            {/* chain + slippage strip */}
            <div className="mini-swap-strip flex items-center justify-between gap-2">
                {multiChainEnabled ? (
                    <div className="flex flex-wrap gap-1.5">
                        {CHAIN_OPTIONS.map((opt) => (
                            <ChainPill
                                key={opt.key}
                                option={opt}
                                active={chain === opt.key}
                                onClick={() => setChain(opt.key)}
                            />
                        ))}
                    </div>
                ) : <div />}
                <button
                    type="button"
                    onClick={() => setShowSlippage((v) => !v)}
                    className="inline-flex items-center gap-1.5 rounded-full bg-zinc-100 px-3 py-1.5 text-[11px] font-semibold text-zinc-700 transition hover:bg-zinc-200 dark:bg-white/10 dark:text-white/75 dark:hover:bg-white/15"
                >
                    <Settings2 size={12} />
                    滑点 {slippage}%
                </button>
            </div>

            {showSlippage ? (
                <div className="mini-swap-slippage rounded-2xl border border-zinc-200 bg-zinc-50 p-3 dark:border-white/10 dark:bg-white/5">
                    <div className="text-[10px] font-bold uppercase tracking-[0.18em] text-zinc-500 dark:text-white/45">
                        滑点容忍度
                    </div>
                    <div className="mt-2 flex items-center gap-2">
                        {SLIPPAGE_PRESETS.map((preset) => (
                            <button
                                key={preset}
                                type="button"
                                onClick={() => setSlippage(preset)}
                                className={`flex-1 rounded-xl py-1.5 text-[12px] font-bold transition ${
                                    Number(slippage) === Number(preset)
                                        ? 'bg-zinc-900 text-white dark:bg-white dark:text-zinc-900'
                                        : 'bg-white text-zinc-600 ring-1 ring-zinc-200 dark:bg-white/5 dark:text-white/65 dark:ring-white/10'
                                }`}
                            >
                                {preset}%
                            </button>
                        ))}
                        <div className="relative flex-1">
                            <input
                                value={slippage}
                                onChange={(e) => handleSlippageInput(e.target.value)}
                                inputMode="decimal"
                                className="w-full rounded-xl border border-zinc-200 bg-white px-2.5 py-1.5 text-right text-[12px] tabular-nums text-zinc-900 outline-none focus:border-zinc-400 dark:border-white/10 dark:bg-white/5 dark:text-white/90"
                                placeholder="自定义"
                            />
                            <span className="absolute right-2.5 top-1/2 -translate-y-1/2 text-[10px] font-semibold text-zinc-400">%</span>
                        </div>
                    </div>
                </div>
            ) : null}

            {/* wallet bar */}
            <button
                type="button"
                onClick={() => setWalletPickerOpen(true)}
                disabled={!hasInitData}
                className="mini-swap-wallet flex w-full items-center gap-3 rounded-2xl border border-zinc-200 bg-white px-3 py-2.5 text-left transition active:scale-[0.99] disabled:opacity-40 dark:border-white/10 dark:bg-[#0f1116]/80"
            >
                <span className="flex h-9 w-9 items-center justify-center rounded-full bg-zinc-100 text-zinc-500 dark:bg-white/10 dark:text-white/55">
                    <Wallet size={16} />
                </span>
                <div className="min-w-0 flex-1">
                    {selectedWallet ? (
                        <>
                            <div className="truncate text-[13px] font-bold text-zinc-900 dark:text-white/90">
                                {selectedWallet.name || `钱包 ${selectedWallet.id}`}
                            </div>
                            <div className="mt-0.5 truncate font-mono text-[10px] text-zinc-400 dark:text-white/35">
                                {shortAddress(selectedWallet.address, 8, 6)}
                            </div>
                        </>
                    ) : (
                        <div className="text-[12px] text-zinc-500 dark:text-white/55">
                            {walletLoading ? '加载钱包…' : '点击选择钱包'}
                        </div>
                    )}
                </div>
                {selectedWallet ? (
                    <div className="shrink-0 text-right">
                        <div className="text-[13px] font-bold tabular-nums text-zinc-900 dark:text-white/85">
                            {formatTokenAmount(selectedWallet.native_balance)}
                        </div>
                        <div className="text-[9px] uppercase tracking-wider text-zinc-400 dark:text-white/30">
                            {nativeSymbol}
                        </div>
                    </div>
                ) : null}
                <ChevronDown size={14} className="shrink-0 text-zinc-400 dark:text-white/40" />
            </button>

            {/* from-to card */}
            <div className="mini-swap-card relative rounded-3xl bg-zinc-50 p-1 dark:bg-[#0f1116]/80">
                {/* FROM */}
                <div className="mini-swap-panel mini-swap-panel--from rounded-[20px] bg-white p-4 dark:bg-[#14171c]">
                    <div className="flex items-center justify-between text-[11px] text-zinc-500 dark:text-white/45">
                        <span className="font-semibold uppercase tracking-wider">支付</span>
                        <span className="tabular-nums">
                            余额 {fromBalanceText}
                            {fromValueUsdText ? ` · ${fromValueUsdText}` : ''}
                        </span>
                    </div>
                    <div className="mt-1 flex items-center gap-2">
                        <input
                            value={amount}
                            onChange={(e) => setAmount(e.target.value)}
                            placeholder="0.0"
                            inputMode="decimal"
                            className="min-w-0 flex-1 bg-transparent text-[32px] font-black tabular-nums text-zinc-900 outline-none placeholder:text-zinc-300 dark:text-white dark:placeholder:text-zinc-700"
                        />
                        <TokenChip token={fromTokenEnriched} onClick={() => setPickerSide('from')} placeholder="选择支付" />
                    </div>
                    <div className="mt-2 flex flex-wrap items-center gap-1">
                        {AMOUNT_PRESETS.map((ratio) => (
                            <PresetAmountButton
                                key={ratio}
                                ratio={ratio}
                                label={ratio === 1 ? 'MAX' : `${Math.round(ratio * 100)}%`}
                                disabled={!fromBalanceToken || !Number(fromBalanceToken.balance)}
                                onClick={handlePreset}
                            />
                        ))}
                    </div>
                </div>

                {/* reverse */}
                <div className="mini-swap-reverse flex justify-center">
                    <button
                        type="button"
                        onClick={handleReverse}
                        className="flex h-10 w-10 items-center justify-center rounded-full bg-white ring-4 ring-zinc-50 transition hover:rotate-180 dark:bg-[#23262d] dark:ring-[#0f1116] dark:text-white/80"
                        title="反向兑换"
                    >
                        <ArrowDown size={18} strokeWidth={2.5} className="text-zinc-700 dark:text-white/85" />
                    </button>
                </div>

                {/* TO */}
                <div className="mini-swap-panel mini-swap-panel--to mt-1 rounded-[20px] bg-white p-4 dark:bg-[#14171c]">
                    <div className="flex items-center justify-between text-[11px] text-zinc-500 dark:text-white/45">
                        <span className="font-semibold uppercase tracking-wider">获得 (估算)</span>
                        {quote?.from_amount_float ? (
                            <span className="tabular-nums">
                                {quote.exchange_rate
                                    ? `1 ${fromToken?.symbol || ''} ≈ ${Number(quote.exchange_rate).toLocaleString('en-US', { maximumFractionDigits: 8 })} ${toToken?.symbol || ''}`
                                    : ''}
                            </span>
                        ) : null}
                    </div>
                    <div className="mt-1 flex items-center gap-2">
                        <div className="min-w-0 flex-1 truncate text-[32px] font-black tabular-nums text-zinc-900 dark:text-white">
                            {quoting && !quote ? (
                                <span className="animate-pulse text-zinc-300 dark:text-zinc-700">…</span>
                            ) : (
                                <NumberFlowValue value={toAmountText} formatter={() => toAmountText} />
                            )}
                        </div>
                        <TokenChip token={toTokenEnriched} onClick={() => setPickerSide('to')} placeholder="选择获得" />
                    </div>
                    {quote ? (
                        <div className="mt-2 flex items-center justify-between text-[11px] text-zinc-500 dark:text-white/45">
                            <span>
                                via <span className="font-semibold text-zinc-700 dark:text-white/75">
                                    {String(quote?.best_provider || quote?.provider || '--').toUpperCase()}
                                </span>
                                {quote?.estimated_gas_usd ? (
                                    <span className="ml-2 tabular-nums">
                                        gas {formatQuoteGasCostSummary(quote, nativeSymbol)}
                                    </span>
                                ) : null}
                            </span>
                            <button
                                type="button"
                                onClick={() => setQuoteDetailsOpen(true)}
                                className="text-[11px] font-semibold text-zinc-700 underline decoration-dotted underline-offset-2 dark:text-white/75"
                            >
                                详情
                            </button>
                        </div>
                    ) : null}
                </div>
            </div>

            {/* limit order extra card */}
            {mode === 'limit' ? (
                <div className="mini-swap-limit-card rounded-2xl border border-zinc-200 bg-white p-3 dark:border-white/10 dark:bg-[#0f1116]/80">
                    <div className="flex items-center justify-between">
                        <div className="text-[11px] font-bold uppercase tracking-[0.18em] text-zinc-500 dark:text-white/45">
                            触发条件
                        </div>
                        <div className="flex gap-1 rounded-full bg-zinc-100 p-0.5 dark:bg-white/10">
                            <button
                                type="button"
                                onClick={() => setLimitMode('to_amount')}
                                className={`rounded-full px-2.5 py-1 text-[10px] font-bold transition ${
                                    limitMode === 'to_amount'
                                        ? 'bg-zinc-900 text-white dark:bg-white dark:text-zinc-900'
                                        : 'text-zinc-600 dark:text-white/65'
                                }`}
                            >
                                到账数量
                            </button>
                            <button
                                type="button"
                                onClick={() => setLimitMode('price')}
                                className={`rounded-full px-2.5 py-1 text-[10px] font-bold transition ${
                                    limitMode === 'price'
                                        ? 'bg-zinc-900 text-white dark:bg-white dark:text-zinc-900'
                                        : 'text-zinc-600 dark:text-white/65'
                                }`}
                            >
                                价格
                            </button>
                        </div>
                    </div>
                    <div className="mt-2 flex items-center gap-2 rounded-xl border border-zinc-200 bg-zinc-50 px-3 py-2 dark:border-white/10 dark:bg-white/5">
                        <input
                            value={limitMode === 'price' ? limitTargetPrice : limitTargetAmount}
                            onChange={(e) => {
                                if (limitMode === 'price') setLimitTargetPrice(e.target.value);
                                else setLimitTargetAmount(e.target.value);
                            }}
                            inputMode="decimal"
                            placeholder={limitMode === 'price' ? '目标价格' : '目标到账数量'}
                            className="min-w-0 flex-1 bg-transparent text-[18px] font-bold tabular-nums text-zinc-900 outline-none placeholder:text-zinc-300 dark:text-white dark:placeholder:text-zinc-700"
                        />
                        <span className="text-[11px] font-semibold text-zinc-500 dark:text-white/45">
                            {limitMode === 'price'
                                ? `${toToken?.symbol || 'To'} / ${fromToken?.symbol || 'From'}`
                                : (toToken?.symbol || 'To')}
                        </span>
                    </div>
                </div>
            ) : null}

            {/* errors */}
            {quoteError ? (
                <div className="rounded-xl border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-[11px] text-amber-700 dark:text-amber-300">
                    报价失败：{quoteError}
                </div>
            ) : null}
            {execError ? (
                <div className="rounded-xl border border-red-500/30 bg-red-500/10 px-3 py-2 text-[11px] text-red-700 dark:text-red-300">
                    {execError}
                </div>
            ) : null}
            {walletTokensError ? (
                <div className="rounded-xl border border-zinc-200 bg-zinc-50 px-3 py-2 text-[11px] text-zinc-500 dark:border-white/10 dark:bg-white/5 dark:text-white/55">
                    钱包余额加载失败：{walletTokensError}
                </div>
            ) : null}

            {/* quote freshness */}
            {quote && lastQuoteAt ? (
                <div className="flex items-center justify-end gap-1 text-[10px] text-zinc-400 dark:text-white/30">
                    <RefreshCw size={9} className={quoting ? 'animate-spin' : undefined} />
                    {formatQuoteRelativeTime(lastQuoteAt, tick)} 刷新
                </div>
            ) : null}

            {/* primary button (sticky-ish at bottom of view) */}
            <div className="mini-swap-submit pt-2">
                <button
                    type="button"
                    onClick={() => (canSubmit ? setConfirmOpen(true) : null)}
                    disabled={!canSubmit}
                    className={`w-full rounded-2xl py-4 text-[15px] font-black tracking-wide transition active:scale-[0.99] ${
                        !canSubmit
                            ? 'bg-zinc-100 text-zinc-400 dark:bg-white/5 dark:text-white/30'
                            : 'bg-zinc-900 text-white shadow-lg shadow-zinc-900/15 hover:bg-zinc-800 dark:bg-white dark:text-zinc-900 dark:hover:bg-zinc-100'
                    }`}
                >
                    {submitText}
                </button>
            </div>

            {/* drawers */}
            <SwapTokenPicker
                open={pickerSide !== null}
                onClose={() => setPickerSide(null)}
                chain={chain}
                walletTokens={enrichedWalletTokens}
                tokenMetaMap={tokenMetaMap}
                excludeAddress={pickerSide === 'from' ? toToken?.address : fromToken?.address}
                onSelect={(t) => handleSelectToken(pickerSide, t)}
            />
            <SwapWalletPicker
                open={walletPickerOpen}
                onClose={() => setWalletPickerOpen(false)}
                wallets={wallets}
                selectedWalletId={selectedWalletId}
                nativeSymbol={nativeSymbol}
                onSelect={handleSelectWallet}
            />
            <SwapQuoteDetails
                open={quoteDetailsOpen}
                onClose={() => setQuoteDetailsOpen(false)}
                quote={quote}
                fromToken={fromToken}
                toToken={toToken}
                nativeSymbol={nativeSymbol}
            />
            <SwapHistoryDrawer
                open={historyOpen}
                onClose={() => setHistoryOpen(false)}
                apiBaseUrl={apiBaseUrl}
                initData={initData}
                chain={chain}
                walletId={selectedWalletId}
                onNotice={onNotice}
            />

            <ConfirmDialog
                open={confirmOpen}
                title={mode === 'limit' ? '创建限价单？' : '确认兑换？'}
                message={[
                    `支付：${amount} ${fromToken?.symbol || ''}`,
                    `${mode === 'limit' ? '目标' : '到账估算'}：${
                        mode === 'limit'
                            ? (limitMode === 'price'
                                ? `${limitTargetPrice} ${toToken?.symbol || ''}/${fromToken?.symbol || ''}`
                                : `${limitTargetAmount} ${toToken?.symbol || ''}`)
                            : `≈ ${quote?.to_amount_float || 0} ${toToken?.symbol || ''}`
                    }`,
                    `路由：${String(quote?.best_provider || quote?.provider || '--').toUpperCase()}`,
                    `滑点：${slippage}%`,
                    `钱包：${shortAddress(selectedWallet?.address || '', 6, 4)}`,
                    mode === 'limit'
                        ? '限价单将在条件满足时由系统自动执行。'
                        : '此操作链上不可逆，请确认。',
                ].join('\n')}
                confirmText={mode === 'limit' ? '创建限价单' : '提交兑换'}
                cancelText="取消"
                danger={false}
                loading={executing}
                onConfirm={handleExecute}
                onCancel={() => setConfirmOpen(false)}
            />
        </div>
    );
}
