import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
  fetchWallets,
  fetchWalletSwapTokenMetadata,
  walletSwapSingleExecute,
  walletSwapSingleQuote,
  walletSwapPreview,
} from '../api';
import PanelShell from './PanelShell';
import { normalizeHexAddress, shortAddress } from '../utils';
import { ArrowDown, ChevronDown, RefreshCw, Search, Settings, Wallet, X, TrendingUp, Check } from 'lucide-react';

const CHAIN_META = {
  bsc: {
    label: 'BNB Chain',
    nativeSymbol: 'BNB',
    stable: {
      symbol: 'USDT',
      name: 'Tether USD',
      address: '0x55d398326f99059fF775485246999027B3197955',
      color: '#26a17b',
    },
    presets: [
      { symbol: 'USDT', name: 'Tether USD', address: '0x55d398326f99059fF775485246999027B3197955', color: '#26a17b' },
      { symbol: 'WBNB', name: 'Wrapped BNB', address: '0xbb4CdB9CBd36B01bD1cBaEBF2De08d9173bc095c', color: '#f0b90b' },
      { symbol: 'USDC', name: 'USD Coin', address: '0x8AC76a51cc950d9822D68b83fE1Ad97B32Cd580d', color: '#2775ca' },
      { symbol: 'BTCB', name: 'Bitcoin BEP20', address: '0x7130d2A12B9BCbfae4f2634d864A1Ee1Ce3Ead9c', color: '#f7931a' },
      { symbol: 'ETH', name: 'Ethereum Token', address: '0x2170Ed0880ac9A755fd29B2688956BD959F933F8', color: '#627eea' },
    ],
  },
  base: {
    label: 'Base',
    nativeSymbol: 'ETH',
    stable: {
      symbol: 'USDC',
      name: 'USD Coin',
      address: '0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913',
      color: '#2775ca',
    },
    presets: [
      { symbol: 'USDC', name: 'USD Coin', address: '0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913', color: '#2775ca' },
      { symbol: 'WETH', name: 'Wrapped Ether', address: '0x4200000000000000000000000000000000000006', color: '#627eea' },
      { symbol: 'cbBTC', name: 'Coinbase Wrapped BTC', address: '0xcbb7c0000ab88b473b1f5afd9ef808440eed33bf', color: '#f7931a' },
    ],
  },
};

const RECENT_STORAGE_KEY = 'tg_lp_bot_swap_recent_tokens_v1';

// 绉婚櫎涓嶉渶瑕佺殑鏍囩椤碉紝鍙繚鐣欏厬鎹㈠姛鑳?// const TABS = [
//   { key: 'swap', label: '鍏戞崲', enabled: true },
// ];

const SLIPPAGE_PRESETS = ['0.5', '1.0', '2.0'];

function dedupeTokens(tokens) {
  const seen = new Set();
  const list = [];
  for (const token of tokens || []) {
    const address = normalizeHexAddress(token?.address);
    if (!address || seen.has(address)) continue;
    seen.add(address);
    list.push({
      address,
      symbol: String(token?.symbol || shortAddress(address, 4, 4)).trim() || shortAddress(address, 4, 4),
      name: String(token?.name || 'Custom Token').trim() || 'Custom Token',
      color: String(token?.color || '#7c8aa6').trim() || '#7c8aa6',
      custom: Boolean(token?.custom),
      logoUrl: String(token?.logoUrl || '').trim(),
    });
  }
  return list;
}

function loadRecentTokens() {
  try {
    const raw = window.localStorage.getItem(RECENT_STORAGE_KEY);
    if (!raw) return {};
    const parsed = JSON.parse(raw);
    if (!parsed || typeof parsed !== 'object') return {};
    return parsed;
  } catch {
    return {};
  }
}

function saveRecentTokens(next) {
  try {
    window.localStorage.setItem(RECENT_STORAGE_KEY, JSON.stringify(next));
  } catch {
    // ignore storage failures
  }
}

function buildCustomToken(address) {
  const normalized = normalizeHexAddress(address);
  if (!normalized) return null;
  return {
    address: normalized,
    symbol: shortAddress(normalized, 4, 4),
    name: '鑷畾涔夊悎绾﹀湴鍧€',
    color: '#7c8aa6',
    custom: true,
  };
}

function getChainConfig(chain) {
  return CHAIN_META[chain] || CHAIN_META.bsc;
}

function getPresetTokens(chain) {
  return dedupeTokens(getChainConfig(chain).presets);
}

function buildTokenLookup(tokens) {
  return dedupeTokens(tokens);
}

function resolveTokenMeta(address, tokens) {
  const normalized = normalizeHexAddress(address);
  if (!normalized) return null;
  const pool = buildTokenLookup(tokens);
  return pool.find((item) => item.address === normalized) || buildCustomToken(normalized);
}

function shouldFetchTokenMetadata(token) {
  const address = normalizeHexAddress(token?.address);
  if (!address) return false;
  const symbol = String(token?.symbol || '').trim();
  const name = String(token?.name || '').trim();
  const logoUrl = String(token?.logoUrl || '').trim();
  if (!logoUrl) return true;
  if (Boolean(token?.custom)) return true;
  return !symbol || !name || name === symbol;
}

function applyTokenMetadata(token, tokenMetaMap) {
  if (!token) return token;
  const address = normalizeHexAddress(token.address);
  if (!address) return token;
  const meta = tokenMetaMap?.[address];
  if (!meta) return token;

  const fallbackSymbol = shortAddress(address, 4, 4);
  const symbol = String(token.symbol || '').trim();
  const name = String(token.name || '').trim();
  const nextSymbol = symbol && symbol !== fallbackSymbol
    ? symbol
    : String(meta.symbol || symbol || fallbackSymbol).trim() || fallbackSymbol;
  const nextName = Boolean(token.custom) || !name || name === symbol
    ? String(meta.name || name || nextSymbol).trim() || nextSymbol
    : name;

  return {
    ...token,
    address,
    symbol: nextSymbol,
    name: nextName,
    logoUrl: String(token.logoUrl || meta.logoUrl || '').trim(),
  };
}

function formatTokenAmount(value) {
  const num = Number(value);
  if (!Number.isFinite(num) || num <= 0) return '0.0';
  if (num >= 1000) return num.toLocaleString('en-US', { maximumFractionDigits: 2 });
  if (num >= 1) return num.toLocaleString('en-US', { maximumFractionDigits: 6 });
  return num.toLocaleString('en-US', { maximumFractionDigits: 8 });
}

function formatGas(value) {
  const num = Number(value);
  if (!Number.isFinite(num) || num <= 0) return '--';
  return num.toLocaleString('en-US', { maximumFractionDigits: 0 });
}

function formatNativeBalance(value) {
  const num = Number(value);
  if (!Number.isFinite(num)) return '--';
  return num.toLocaleString('en-US', { minimumFractionDigits: 0, maximumFractionDigits: 4 });
}

function matchesToken(token, query) {
  const keyword = String(query || '').trim().toLowerCase();
  if (!keyword) return true;
  return [token.symbol, token.name, token.address].some((value) =>
    String(value || '').toLowerCase().includes(keyword)
  );
}

function TokenGlyph({ token, size = 'md' }) {
  const symbol = String(token?.symbol || '?').trim();
  const color = String(token?.color || '#7c8aa6').trim() || '#7c8aa6';
  const logoUrl = String(token?.logoUrl || '').trim();
  const [imgFailed, setImgFailed] = useState(false);

  useEffect(() => {
    setImgFailed(false);
  }, [logoUrl, symbol, token?.address]);

  if (logoUrl && !imgFailed) {
    return (
      <img
        src={logoUrl}
        alt={symbol}
        className={`swap-token-glyph size-${size}`}
        style={{ objectFit: 'cover' }}
        onError={() => setImgFailed(true)}
      />
    );
  }

  return (
    <span className={`swap-token-glyph size-${size}`} style={{ '--token-color': color }}>
      {symbol.slice(0, 1)}
    </span>
  );
}

function TokenButton({ token, placeholder, onClick }) {
  return (
    <button type="button" className="swap-token-button" onClick={onClick}>
      {token ? (
        <>
          <TokenGlyph token={token} />
          <span className="swap-token-button-copy">
            <strong>{token.symbol}</strong>
            <small>{token.custom ? shortAddress(token.address, 6, 4) : token.name}</small>
          </span>
        </>
      ) : (
        <span className="swap-token-placeholder">{placeholder}</span>
      )}
      <ChevronDown size={16} />
    </button>
  );
}

function DetailRow({ label, value, emphasis = false }) {
  return (
    <div className={`swap-detail-row${emphasis ? ' emphasis' : ''}`}>
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  );
}

export default function SwapPanel({ apiBaseUrl, initData, hasInitData, chain = 'bsc' }) {
  const chainConfig = getChainConfig(chain);

  const [wallets, setWallets] = useState([]);
  const [selectedWalletId, setSelectedWalletId] = useState('');
  const [walletLoading, setWalletLoading] = useState(false);

  const [fromToken, setFromToken] = useState('');
  const [toToken, setToToken] = useState(() => chainConfig.stable.address);
  const [amount, setAmount] = useState('');
  const [slippage, setSlippage] = useState('1.0');
  const [showSettings, setShowSettings] = useState(false);
  const [walletDropdownOpen, setWalletDropdownOpen] = useState(false);

  const [quoteInfo, setQuoteInfo] = useState(null);
  const [quoting, setQuoting] = useState(false);
  const [quoteError, setQuoteError] = useState('');

  const [executing, setExecuting] = useState(false);
  const [execError, setExecError] = useState('');
  const [execSuccess, setExecSuccess] = useState('');
  const [showConfirm, setShowConfirm] = useState(false);

  const [pickerOpen, setPickerOpen] = useState(false);
  const [pickerSide, setPickerSide] = useState('from');
  const [tokenQuery, setTokenQuery] = useState('');
  const [recentTokens, setRecentTokens] = useState(() => loadRecentTokens());

  const [walletTokens, setWalletTokens] = useState([]);
  const [loadingWalletTokens, setLoadingWalletTokens] = useState(false);
  const [tokenMetaMap, setTokenMetaMap] = useState({});

  const quoteTimeout = useRef(null);
  const quoteAbortRef = useRef(null);
  const quoteSeqRef = useRef(0);

  const normalizedFromToken = normalizeHexAddress(fromToken);
  const normalizedToToken = normalizeHexAddress(toToken);
  const selectedWallet = useMemo(
    () => wallets.find((item) => String(item.id) === String(selectedWalletId)) || null,
    [wallets, selectedWalletId]
  );
  const rawPresetTokens = useMemo(() => getPresetTokens(chain), [chain]);
  const rawRecentChainTokens = useMemo(
    () => dedupeTokens(recentTokens?.[chain] || []),
    [chain, recentTokens]
  );
  const presetTokens = useMemo(
    () => rawPresetTokens.map((token) => applyTokenMetadata(token, tokenMetaMap)),
    [rawPresetTokens, tokenMetaMap]
  );
  const recentChainTokens = useMemo(
    () => rawRecentChainTokens.map((token) => applyTokenMetadata(token, tokenMetaMap)),
    [rawRecentChainTokens, tokenMetaMap]
  );
  const enrichedWalletTokens = useMemo(
    () => walletTokens.map((token) => applyTokenMetadata(token, tokenMetaMap)),
    [walletTokens, tokenMetaMap]
  );
  const tokenLookup = useMemo(
    () => buildTokenLookup([...enrichedWalletTokens, ...recentChainTokens, ...presetTokens]),
    [enrichedWalletTokens, recentChainTokens, presetTokens]
  );
  const fromTokenMeta = useMemo(
    () => resolveTokenMeta(fromToken, tokenLookup),
    [fromToken, tokenLookup]
  );
  const toTokenMeta = useMemo(
    () => resolveTokenMeta(toToken, tokenLookup),
    [toToken, tokenLookup]
  );

  const selectedQuoteAmount = useMemo(
    () => formatTokenAmount(quoteInfo?.to_amount_float),
    [quoteInfo]
  );
  const minReceived = useMemo(() => {
    const out = Number(quoteInfo?.to_amount_float);
    const slip = Number(slippage);
    if (!Number.isFinite(out) || out <= 0 || !Number.isFinite(slip) || slip < 0) return '--';
    return formatTokenAmount(out * (1 - slip / 100));
  }, [quoteInfo, slippage]);

  const pickerTokens = useMemo(() => {
    const keyword = String(tokenQuery || '').trim().toLowerCase();

    // Merge wallet balance data into token rows.
    const enrichToken = (token) => {
      const walletToken = enrichedWalletTokens.find((wt) => wt.address === token.address);
      return {
        ...token,
        balance: walletToken?.balance || '0',
        valueUSDT: walletToken?.valueUSDT || 0,
      };
    };

    // 鏈変綑棰濈殑浠ｅ竵
    const withBalance = enrichedWalletTokens
      .filter((wt) => matchesToken(wt, keyword))
      .map((wt) => {
        const existing = [...recentChainTokens, ...presetTokens].find((t) => t.address === wt.address);
        return existing ? enrichToken(existing) : {
          address: wt.address,
          symbol: wt.symbol,
          name: wt.symbol,
          color: '#7c8aa6',
          balance: wt.balance,
          valueUSDT: wt.valueUSDT,
          logoUrl: wt.logoUrl || '',
        };
      })
      .sort((a, b) => (b.valueUSDT || 0) - (a.valueUSDT || 0));

    // 鏈€杩戜娇鐢ㄧ殑浠ｅ竵(鎺掗櫎宸插湪浣欓鍒楄〃涓殑)
    const recent = recentChainTokens
      .filter((token) => !withBalance.some((item) => item.address === token.address))
      .filter((token) => matchesToken(token, keyword))
      .map(enrichToken);

    // 甯哥敤浠ｅ竵(鎺掗櫎宸插湪浣欓鍜屾渶杩戝垪琛ㄤ腑鐨?
    const preset = presetTokens
      .filter((token) => !withBalance.some((item) => item.address === token.address))
      .filter((token) => !recent.some((item) => item.address === token.address))
      .filter((token) => matchesToken(token, keyword))
      .map(enrichToken);

    const customCandidate = applyTokenMetadata(buildCustomToken(tokenQuery), tokenMetaMap);
    return {
      customCandidate:
        customCandidate &&
        !withBalance.some((item) => item.address === customCandidate.address) &&
        !recent.some((item) => item.address === customCandidate.address) &&
        !preset.some((item) => item.address === customCandidate.address)
          ? customCandidate
          : null,
      withBalance,
      recent,
      preset,
    };
  }, [enrichedWalletTokens, tokenMetaMap, tokenQuery, recentChainTokens, presetTokens]);

  const persistRecentToken = useCallback((token) => {
    const normalized = normalizeHexAddress(token?.address);
    if (!normalized) return;
    setRecentTokens((prev) => {
      const current = dedupeTokens(prev?.[chain] || []);
      const nextList = dedupeTokens([{ ...token, address: normalized }, ...current]).slice(0, 6);
      const next = { ...(prev || {}), [chain]: nextList };
      saveRecentTokens(next);
      return next;
    });
  }, [chain]);

  useEffect(() => {
    setToToken(chainConfig.stable.address);
    setFromToken('');
    setAmount('');
    setQuoteInfo(null);
    setQuoteError('');
    setExecError('');
    setExecSuccess('');
    setShowConfirm(false);
    setWalletTokens([]);
    setTokenMetaMap({});
  }, [chainConfig.stable.address]);

  const metadataCandidates = useMemo(() => {
    const customTokens = [
      buildCustomToken(normalizedFromToken),
      buildCustomToken(normalizedToToken),
      buildCustomToken(tokenQuery),
    ].filter(Boolean);
    return buildTokenLookup([
      ...rawPresetTokens,
      ...rawRecentChainTokens,
      ...walletTokens,
      ...customTokens,
    ]);
  }, [normalizedFromToken, normalizedToToken, rawPresetTokens, rawRecentChainTokens, tokenQuery, walletTokens]);

  useEffect(() => {
    if (!hasInitData || !initData) return undefined;
    const addresses = metadataCandidates
      .filter((token) => shouldFetchTokenMetadata(token) && !tokenMetaMap[token.address])
      .map((token) => token.address);
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
        const rows = Array.isArray(resp?.tokens) ? resp.tokens : [];
        if (!rows.length) return;
        setTokenMetaMap((prev) => {
          const next = { ...prev };
          for (const item of rows) {
            const address = normalizeHexAddress(item?.address);
            if (!address) continue;
            next[address] = {
              address,
              symbol: String(item?.symbol || '').trim(),
              name: String(item?.name || '').trim(),
              logoUrl: String(item?.logo_url || '').trim(),
            };
          }
          return next;
        });
      })
      .catch((error) => {
        if (controller.signal.aborted) return;
        console.error('fetchWalletSwapTokenMetadata failed', error);
      });

    return () => controller.abort();
  }, [apiBaseUrl, chain, hasInitData, initData, metadataCandidates, tokenMetaMap]);

  const loadWallets = useCallback(async () => {
    if (!initData) return;
    setWalletLoading(true);
    try {
      const resp = await fetchWallets({ apiBaseUrl, initData, chain });
      const list = resp?.wallets || [];
      setWallets(list);
      setSelectedWalletId((current) => {
        if (list.some((item) => String(item.id) === String(current))) return current;
        const fallback = list.find((item) => item.is_default) || list[0];
        return fallback ? String(fallback.id) : '';
      });
    } catch (error) {
      console.error('fetchWallets failed', error);
    } finally {
      setWalletLoading(false);
    }
  }, [apiBaseUrl, initData, chain]);

  const loadWalletTokens = useCallback(async () => {
    if (!initData || !selectedWalletId) {
      console.log('loadWalletTokens: missing initData or selectedWalletId', { initData: !!initData, selectedWalletId });
      return;
    }
    setLoadingWalletTokens(true);
    console.log('loadWalletTokens: starting', { chain, selectedWalletId });
    try {
      // 闄嶄綆鏈€灏忎环鍊奸槇鍊硷紝鏄剧ず鏇村浠ｅ竵
      const resp = await walletSwapPreview({ apiBaseUrl, initData, chain, minValueUsd: 0.001 });
      console.log('loadWalletTokens: response', resp);
      const tokens = (resp?.tokens || []).map((t) => ({
        address: normalizeHexAddress(t.address),
        symbol: t.symbol,
        name: t.symbol,
        balance: t.balance,
        valueUSDT: t.value_usdt || 0,
        logoUrl: t.logo_url || '',
      }));
      console.log('loadWalletTokens: processed tokens', tokens);
      setWalletTokens(tokens);
    } catch (error) {
      console.error('loadWalletTokens failed', error);
      // 澶辫触鏃朵笉娓呯┖锛屼繚鐣欎箣鍓嶇殑鏁版嵁
      if (walletTokens.length === 0) {
        setWalletTokens([]);
      }
    } finally {
      setLoadingWalletTokens(false);
    }
  }, [apiBaseUrl, initData, chain, selectedWalletId, walletTokens.length]);

  useEffect(() => {
    if (!hasInitData) return;
    loadWallets();
    setExecError('');
    setExecSuccess('');
  }, [hasInitData, loadWallets]);

  // 鍙湪鎵撳紑浠ｅ竵閫夋嫨鍣ㄦ椂鍔犺浇浣欓锛岄伩鍏嶄笉蹇呰鐨?API 璋冪敤
  useEffect(() => {
    if (!hasInitData || !selectedWalletId || !pickerOpen) return;
    // Avoid refetching while the current token cache is still valid.
    if (walletTokens.length > 0) return;
    loadWalletTokens();
  }, [hasInitData, selectedWalletId, pickerOpen, walletTokens.length, loadWalletTokens]);

  const doQuote = useCallback(async ({
    amt,
    fromAddress,
    toAddress,
    walletId,
    chainId,
    slip,
    signal,
    seq,
  }) => {
    const amountNumber = Number(amt);
    if (!walletId || !Number.isFinite(amountNumber) || amountNumber <= 0 || !fromAddress || !toAddress) {
      setQuoteInfo(null);
      setQuoteError('');
      setQuoting(false);
      return;
    }
    if (fromAddress === toAddress) {
      setQuoteInfo(null);
      setQuoteError('From and to tokens must be different.');
      setQuoting(false);
      return;
    }

    setQuoting(true);
    setQuoteError('');
    setQuoteInfo(null);

    try {
      const resp = await walletSwapSingleQuote({
        apiBaseUrl,
        initData,
        chain: chainId,
        walletId,
        fromToken: fromAddress,
        toToken: toAddress,
        amount: amt,
        slippagePercent: Number.parseFloat(slip),
        signal,
      });
      if (quoteSeqRef.current !== seq) return;
      setQuoteInfo(resp);
    } catch (error) {
      if (signal?.aborted) return;
      if (quoteSeqRef.current !== seq) return;
      setQuoteInfo(null);
      setQuoteError(String(error?.message || error));
    } finally {
      if (quoteSeqRef.current === seq) {
        setQuoting(false);
      }
    }
  }, [apiBaseUrl, initData]);

  useEffect(() => {
    if (quoteTimeout.current) clearTimeout(quoteTimeout.current);
    if (quoteAbortRef.current) quoteAbortRef.current.abort();

    const amountNumber = Number(amount);
    if (!selectedWalletId || !Number.isFinite(amountNumber) || amountNumber <= 0 || !normalizedFromToken || !normalizedToToken) {
      setQuoting(false);
      setQuoteInfo(null);
      if (normalizedFromToken && normalizedToToken && normalizedFromToken === normalizedToToken) {
        setQuoteError('From and to tokens must be different.');
      } else {
        setQuoteError('');
      }
      return undefined;
    }

    quoteTimeout.current = setTimeout(() => {
      const seq = quoteSeqRef.current + 1;
      quoteSeqRef.current = seq;
      const controller = new AbortController();
      quoteAbortRef.current = controller;
      doQuote({
        amt: amount,
        fromAddress: normalizedFromToken,
        toAddress: normalizedToToken,
        walletId: selectedWalletId,
        chainId: chain,
        slip: slippage,
        signal: controller.signal,
        seq,
      });
    }, 450);

    return () => {
      if (quoteTimeout.current) clearTimeout(quoteTimeout.current);
      if (quoteAbortRef.current) quoteAbortRef.current.abort();
    };
  }, [amount, chain, doQuote, normalizedFromToken, normalizedToToken, selectedWalletId, slippage]);

  useEffect(() => {
    if (!pickerOpen && !showConfirm && !walletDropdownOpen) return undefined;
    const onKeyDown = (event) => {
      if (event.key !== 'Escape') return;
      if (pickerOpen) {
        setPickerOpen(false);
        return;
      }
      if (walletDropdownOpen) {
        setWalletDropdownOpen(false);
        return;
      }
      if (!executing) setShowConfirm(false);
    };
    window.addEventListener('keydown', onKeyDown);
    return () => window.removeEventListener('keydown', onKeyDown);
  }, [executing, pickerOpen, showConfirm, walletDropdownOpen]);

  const handleSelectToken = useCallback((token) => {
    if (!token?.address) return;
    if (pickerSide === 'from') setFromToken(token.address);
    else setToToken(token.address);
    persistRecentToken(token);
    setPickerOpen(false);
    setTokenQuery('');
    setExecError('');
    setExecSuccess('');
  }, [persistRecentToken, pickerSide]);

  const handleSwap = async () => {
    if (!initData || !normalizedFromToken || !normalizedToToken) return;
    setExecuting(true);
    setExecError('');
    setExecSuccess('');
    try {
      const resp = await walletSwapSingleExecute({
        apiBaseUrl,
        initData,
        chain,
        walletId: selectedWalletId,
        fromToken: normalizedFromToken,
        toToken: normalizedToToken,
        amount,
        slippagePercent: Number.parseFloat(slippage),
      });
      setExecSuccess(resp?.tx_hash || 'Transaction submitted');
      setShowConfirm(false);
      setAmount('');
      setQuoteInfo(null);
      // 娓呯┖浣欓缂撳瓨锛屼笅娆℃墦寮€閫夋嫨鍣ㄦ椂閲嶆柊鍔犺浇
      setWalletTokens([]);
    } catch (error) {
      setExecError(String(error?.message || error));
      setShowConfirm(false);
    } finally {
      setExecuting(false);
    }
  };

  const handleMaxAmount = () => {
    if (!normalizedFromToken) return;
    const walletToken = walletTokens.find((t) => t.address === normalizedFromToken);
    if (walletToken && walletToken.balance) {
      setAmount(walletToken.balance);
    }
  };

  const fromTokenBalance = useMemo(() => {
    if (!normalizedFromToken) return null;
    const walletToken = walletTokens.find((t) => t.address === normalizedFromToken);
    return walletToken?.balance || '0';
  }, [normalizedFromToken, walletTokens]);

  const handleReverse = () => {
    if (!normalizedFromToken && !normalizedToToken) return;
    setFromToken(toToken);
    setToToken(fromToken);
    setAmount('');
    setQuoteInfo(null);
    setQuoteError('');
    setExecError('');
    setExecSuccess('');
  };

  const isReadyToSwap = Boolean(
    selectedWalletId &&
    normalizedFromToken &&
    normalizedToToken &&
    normalizedFromToken !== normalizedToToken &&
    Number(amount) > 0 &&
    quoteInfo &&
    !quoting &&
    !executing
  );

  let submitLabel = '棰勮鍏戞崲';
  if (!selectedWalletId) submitLabel = walletLoading ? '鍔犺浇閽卞寘涓?..' : '璇峰厛閫夋嫨閽卞寘';
  else if (!normalizedFromToken) submitLabel = '閫夋嫨鍗栧嚭浠ｅ竵';
  else if (!amount || Number(amount) <= 0) submitLabel = '杈撳叆鍗栧嚭鏁伴噺';
  else if (!normalizedToToken) submitLabel = '閫夋嫨涔板叆浠ｅ竵';
  else if (normalizedFromToken === normalizedToToken) submitLabel = '涓嶈兘鍏戞崲鍚屼竴浠ｅ竵';
  else if (quoting) submitLabel = '鑾峰彇鏈€浼樻姤浠蜂腑...';
  else if (!quoteInfo) submitLabel = '绛夊緟鎶ヤ环';

  return (
    <PanelShell
      title="Swap"
      subtitle="Swap any supported token"
      icon={RefreshCw}
    >
      <div className="swap-panel">
        <div className="swap-panel-shell">
          <div className="swap-panel-topbar">
            <div style={{ fontSize: '18px', fontWeight: '700', color: 'var(--text)' }}>
              鍏戞崲
            </div>
            <button
              type="button"
              className={`swap-settings-trigger${showSettings ? ' active' : ''}`}
              onClick={() => setShowSettings((current) => !current)}
              aria-label="閰嶇疆鍏戞崲鍙傛暟"
            >
              <Settings size={17} />
            </button>
          </div>

          {showSettings ? (
            <div className="swap-settings-card">
              <div className="swap-settings-grid">
                <label className="swap-settings-field">
                  <span>鎵ц閽卞寘</span>
                  <div className="swap-custom-select-wrap">
                    <button
                      type="button"
                      className="swap-custom-select-trigger"
                      onClick={() => setWalletDropdownOpen(!walletDropdownOpen)}
                      disabled={walletLoading || !wallets.length}
                    >
                      <Wallet size={14} />
                      <span className="swap-custom-select-value">
                        {selectedWallet
                          ? `${selectedWallet.name || '閽卞寘'} 路 ${shortAddress(selectedWallet.address)}`
                          : walletLoading
                            ? '鍔犺浇閽卞寘涓?..'
                            : '鏆傛棤鍙敤閽卞寘'}
                      </span>
                      <ChevronDown size={14} style={{ marginLeft: 'auto', opacity: 0.6 }} />
                    </button>
                    {walletDropdownOpen && wallets.length > 0 ? (
                      <div className="swap-custom-select-dropdown">
                        {wallets.map((wallet) => (
                          <button
                            key={wallet.id}
                            type="button"
                            className={`swap-custom-select-option${String(wallet.id) === String(selectedWalletId) ? ' active' : ''}`}
                            onClick={() => {
                              setSelectedWalletId(String(wallet.id));
                              setWalletDropdownOpen(false);
                            }}
                          >
                            <div className="swap-custom-select-option-main">
                              <span className="swap-custom-select-option-name">
                                {wallet.name || '閽卞寘'}
                              </span>
                              <span className="swap-custom-select-option-address">
                                {shortAddress(wallet.address)}
                              </span>
                            </div>
                            <div className="swap-custom-select-option-balance">
                              <span>{chainConfig.nativeSymbol}</span>
                              <span>{formatNativeBalance(wallet.native_balance)}</span>
                            </div>
                            {String(wallet.id) === String(selectedWalletId) ? (
                              <Check size={16} className="swap-custom-select-option-check" />
                            ) : null}
                          </button>
                        ))}
                      </div>
                    ) : null}
                  </div>
                </label>

                <label className="swap-settings-field">
                  <span>婊戠偣涓婇檺</span>
                  <div className="swap-slippage-input-group">
                    <div className="swap-slippage-pills">
                      {SLIPPAGE_PRESETS.map((item) => (
                        <button
                          key={item}
                          type="button"
                          className={`swap-slippage-pill${String(slippage) === item ? ' active' : ''}`}
                          onClick={() => setSlippage(item)}
                        >
                          {item}%
                        </button>
                      ))}
                    </div>
                    <div className="swap-slippage-input-wrap">
                      <input
                        type="number"
                        min="0"
                        step="0.1"
                        value={slippage}
                        onChange={(event) => setSlippage(event.target.value)}
                        className="swap-slippage-input"
                        placeholder="1.0"
                      />
                      <span>%</span>
                    </div>
                  </div>
                </label>
              </div>
            </div>
          ) : null}

          <div className="swap-surface">
            <div className="swap-context-row">
              <div className="swap-context-pill strong">{chainConfig.label}</div>
              <div className="swap-context-pill">
                {selectedWallet
                  ? `${selectedWallet.name || '閽卞寘'} 路 ${shortAddress(selectedWallet.address)}`
                  : walletLoading
                    ? '鍔犺浇閽卞寘涓?..'
                    : '鏈€夋嫨閽卞寘'}
              </div>
            </div>

            <div className="swap-card-group">
              <div className="swap-card">
                <div className="swap-card-head">
                  <span>鍑哄敭</span>
                  <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                    {fromTokenBalance && Number(fromTokenBalance) > 0 ? (
                      <button
                        type="button"
                        onClick={handleMaxAmount}
                        style={{
                          padding: '4px 10px',
                          borderRadius: '8px',
                          border: '1px solid rgba(var(--accent-rgb), 0.4)',
                          background: 'rgba(var(--accent-rgb), 0.12)',
                          color: 'var(--accent-text)',
                          fontSize: '11px',
                          fontWeight: '700',
                          cursor: 'pointer',
                          transition: 'all 0.18s ease',
                        }}
                        onMouseEnter={(e) => {
                          e.currentTarget.style.background = 'rgba(var(--accent-rgb), 0.2)';
                        }}
                        onMouseLeave={(e) => {
                          e.currentTarget.style.background = 'rgba(var(--accent-rgb), 0.12)';
                        }}
                      >
                        鏈€澶?                      </button>
                    ) : null}
                    <small>{fromTokenMeta ? shortAddress(fromTokenMeta.address, 8, 6) : '鏀寔绮樿创浠绘剰鍚堢害鍦板潃'}</small>
                  </div>
                </div>
                <div className="swap-card-body">
                  <input
                    type="number"
                    min="0"
                    step="any"
                    className="swap-amount-input"
                    value={amount}
                    onChange={(event) => setAmount(event.target.value)}
                    placeholder="0"
                  />
                  <TokenButton
                    token={fromTokenMeta}
                    placeholder="閫夋嫨浠ｅ竵"
                    onClick={() => {
                      setPickerSide('from');
                      setPickerOpen(true);
                    }}
                  />
                </div>
                <div className="swap-card-foot">
                  <span>{fromTokenMeta ? fromTokenMeta.name : '鏈€夋嫨鍗栧嚭浠ｅ竵'}</span>
                  <span style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
                    {fromTokenBalance && Number(fromTokenBalance) > 0 ? (
                      <>
                        <span style={{ color: 'var(--positive)' }}>浣欓:</span>
                        <span style={{ fontWeight: '700' }}>{formatTokenAmount(fromTokenBalance)}</span>
                      </>
                    ) : selectedWallet ? (
                      `${chainConfig.nativeSymbol} ${formatNativeBalance(selectedWallet.native_balance)}`
                    ) : (
                      '--'
                    )}
                  </span>
                </div>
              </div>

              <button
                type="button"
                className="swap-switch-button"
                onClick={handleReverse}
                aria-label="鍒囨崲鍏戞崲鏂瑰悜"
              >
                <ArrowDown size={18} strokeWidth={2.5} />
              </button>

              <div className="swap-card muted">
                <div className="swap-card-head">
                  <span>璐拱</span>
                  <small>{toTokenMeta ? shortAddress(toTokenMeta.address, 8, 6) : '閫夋嫨鐩爣浠ｅ竵'}</small>
                </div>
                <div className="swap-card-body">
                  <div className={`swap-quote-output${quoting ? ' loading' : ''}`}>
                    {quoting ? '...' : selectedQuoteAmount}
                  </div>
                  <TokenButton
                    token={toTokenMeta}
                    placeholder="閫夋嫨浠ｅ竵"
                    onClick={() => {
                      setPickerSide('to');
                      setPickerOpen(true);
                    }}
                  />
                </div>
                <div className="swap-card-foot">
                  <span>{toTokenMeta ? toTokenMeta.name : '鏈€夋嫨鐩爣浠ｅ竵'}</span>
                  <span>鏈€灏戝埌璐?{minReceived}</span>
                </div>
              </div>
            </div>

            <div className="swap-summary-card">
              {quoteInfo ? (
                <>
                  <DetailRow
                    label="棰勪及鍒拌处"
                    value={`${selectedQuoteAmount} ${toTokenMeta?.symbol || ''}`.trim()}
                    emphasis
                  />
                  <DetailRow label="Minimum received" value={`${minReceived} ${toTokenMeta?.symbol || ''}`.trim()} />
                  <DetailRow label="棰勪及 Gas" value={formatGas(quoteInfo?.estimated_gas)} />
                  <DetailRow label="婊戠偣璁剧疆" value={`${slippage || '1.0'}%`} />
                </>
              ) : (
                <div className="swap-summary-empty">
                  <strong>Enter an amount to get a quote</strong>
                  <span>Select a token or paste an ERC-20 contract address.</span>
                </div>
              )}
            </div>

            {quoteError ? (
              <div className="panel-error">
                <strong>鎶ヤ环澶辫触:</strong> {quoteError}
              </div>
            ) : null}

            {execError ? (
              <div className="panel-error">
                <strong>鍏戞崲澶辫触:</strong> {execError}
              </div>
            ) : null}

            {execSuccess ? (
              <div className="panel-success swap-success-card">
                <strong>Swap request submitted</strong>
                <span>{execSuccess}</span>
              </div>
            ) : null}

            <button
              type="button"
              className="swap-submit-button"
              disabled={!isReadyToSwap}
              onClick={() => setShowConfirm(true)}
            >
              {executing ? '鎵ц涓?..' : submitLabel}
            </button>
          </div>
        </div>

        {(pickerOpen || walletDropdownOpen) ? (
          <div
            className="swap-overlay-backdrop"
            onClick={() => {
              setPickerOpen(false);
              setWalletDropdownOpen(false);
            }}
          />
        ) : null}

        {pickerOpen ? (
          <div className="swap-modal-overlay" style={{ background: 'transparent' }}>
            <div className="swap-token-modal" onClick={(event) => event.stopPropagation()}>
              <div className="swap-modal-header">
                <div>
                  <div className="swap-modal-kicker">閫夋嫨浠ｅ竵</div>
                  <h3>{pickerSide === 'from' ? '閫夋嫨鍗栧嚭浠ｅ竵' : '閫夋嫨涔板叆浠ｅ竵'}</h3>
                </div>
                <button type="button" className="swap-modal-close" onClick={() => setPickerOpen(false)}>
                  <X size={18} />
                </button>
              </div>

              <div className="swap-token-search">
                <Search size={16} />
                <input
                  type="text"
                  value={tokenQuery}
                  onChange={(event) => setTokenQuery(event.target.value)}
                  placeholder="鎼滅储绗﹀彿锛屾垨绮樿创鍚堢害鍦板潃"
                  autoFocus
                />
                {loadingWalletTokens && walletTokens.length > 0 ? (
                  <div style={{ fontSize: '11px', color: 'var(--text-muted)', whiteSpace: 'nowrap' }}>
                    鍒锋柊涓?..
                  </div>
                ) : null}
              </div>

              <div className="swap-quick-picks">
                {presetTokens.slice(0, 5).map((token) => (
                  <button
                    key={token.address}
                    type="button"
                    className="swap-quick-pick"
                    onClick={() => handleSelectToken(token)}
                  >
                    <TokenGlyph token={token} size="sm" />
                    <span>{token.symbol}</span>
                  </button>
                ))}
              </div>

              <div className="swap-token-list">
                {loadingWalletTokens && walletTokens.length === 0 ? (
                  <div style={{ padding: '20px', textAlign: 'center', color: 'var(--text-muted)', fontSize: '13px' }}>
                    <div style={{ marginBottom: '8px' }}>馃攧 鍔犺浇閽卞寘浣欓涓?..</div>
                    <div style={{ fontSize: '11px', opacity: '0.7' }}>棣栨鍔犺浇鍙兘闇€瑕佸嚑绉掗挓</div>
                  </div>
                ) : null}

                {pickerTokens.customCandidate ? (
                  <div className="swap-token-section">
                    <div className="swap-token-section-title">鑷畾涔夊湴鍧€</div>
                    <button
                      type="button"
                      className="swap-token-row"
                      onClick={() => handleSelectToken(pickerTokens.customCandidate)}
                    >
                      <TokenGlyph token={pickerTokens.customCandidate} />
                      <div className="swap-token-row-copy">
                        <strong>{pickerTokens.customCandidate.symbol}</strong>
                        <span>{pickerTokens.customCandidate.address}</span>
                      </div>
                      <span className="swap-token-tag">绮樿创浣跨敤</span>
                    </button>
                  </div>
                ) : null}

                {pickerTokens.withBalance && pickerTokens.withBalance.length > 0 ? (
                  <div className="swap-token-section">
                    <div className="swap-token-section-title">閽卞寘浣欓</div>
                    {pickerTokens.withBalance.map((token) => (
                      <button
                        key={token.address}
                        type="button"
                        className="swap-token-row"
                        onClick={() => handleSelectToken(token)}
                      >
                        <TokenGlyph token={token} />
                        <div className="swap-token-row-copy">
                          <strong>{token.symbol}</strong>
                          <span style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
                            {token.name}
                            {token.valueUSDT > 0 ? (
                              <span style={{ color: '#a0a8ba', fontSize: '11px' }}>
                                鈮?${token.valueUSDT.toFixed(2)}
                              </span>
                            ) : null}
                          </span>
                        </div>
                        <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'flex-end', gap: '2px' }}>
                          <span className="swap-token-tag" style={{ background: 'rgba(34, 197, 94, 0.08)', borderColor: 'rgba(34, 197, 94, 0.22)', color: '#22c55e' }}>
                            {formatTokenAmount(token.balance)}
                          </span>
                        </div>
                      </button>
                    ))}
                  </div>
                ) : null}

                {pickerTokens.recent.length ? (
                  <div className="swap-token-section">
                    <div className="swap-token-section-title">Recent</div>
                    {pickerTokens.recent.map((token) => (
                      <button
                        key={token.address}
                        type="button"
                        className="swap-token-row"
                        onClick={() => handleSelectToken(token)}
                      >
                        <TokenGlyph token={token} />
                        <div className="swap-token-row-copy">
                          <strong>{token.symbol}</strong>
                          <span>{token.custom ? token.address : token.name}</span>
                        </div>
                        <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'flex-end', gap: '2px' }}>
                          {token.balance && Number(token.balance) > 0 ? (
                            <span style={{ fontSize: '11px', color: '#22c55e', fontWeight: '600' }}>
                              {formatTokenAmount(token.balance)}
                            </span>
                          ) : null}
                          <span className="swap-token-tag">Recent</span>
                        </div>
                      </button>
                    ))}
                  </div>
                ) : null}

                {pickerTokens.preset.length ? (
                  <div className="swap-token-section">
                    <div className="swap-token-section-title">甯哥敤浠ｅ竵</div>
                    {pickerTokens.preset.map((token) => (
                      <button
                        key={token.address}
                        type="button"
                        className="swap-token-row"
                        onClick={() => handleSelectToken(token)}
                      >
                        <TokenGlyph token={token} />
                        <div className="swap-token-row-copy">
                          <strong>{token.symbol}</strong>
                          <span>{token.name}</span>
                        </div>
                        <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'flex-end', gap: '2px' }}>
                          {token.balance && Number(token.balance) > 0 ? (
                            <span style={{ fontSize: '11px', color: '#22c55e', fontWeight: '600' }}>
                              {formatTokenAmount(token.balance)}
                            </span>
                          ) : null}
                          <span className="swap-token-tag">{shortAddress(token.address, 5, 4)}</span>
                        </div>
                      </button>
                    ))}
                  </div>
                ) : null}

                {!loadingWalletTokens && !pickerTokens.customCandidate && !pickerTokens.withBalance?.length && !pickerTokens.recent.length && !pickerTokens.preset.length ? (
                  <div className="swap-token-empty">
                    <strong>娌℃湁鍖归厤缁撴灉</strong>
                    <span>You can also paste an ERC-20 contract address.</span>
                  </div>
                ) : null}
              </div>
            </div>
          </div>
        ) : null}

        {showConfirm && quoteInfo ? (
          <div className="swap-modal-overlay" onClick={() => (!executing ? setShowConfirm(false) : null)}>
            <div className="swap-confirm-modal" onClick={(event) => event.stopPropagation()}>
              <div className="swap-modal-header">
                <div>
                  <div className="swap-modal-kicker">Review swap</div>
                  <h3>纭鍏戞崲</h3>
                </div>
                <button
                  type="button"
                  className="swap-modal-close"
                  onClick={() => setShowConfirm(false)}
                  disabled={executing}
                >
                  <X size={18} />
                </button>
              </div>

              <div className="swap-confirm-route">
                <div className="swap-confirm-flow">
                  <div className="swap-confirm-token">
                    <TokenGlyph token={fromTokenMeta || buildCustomToken(normalizedFromToken)} />
                    <div>
                      <span>鏀粯</span>
                      <strong>{amount} {fromTokenMeta?.symbol || shortAddress(normalizedFromToken, 4, 4)}</strong>
                    </div>
                  </div>
                  <div className="swap-confirm-arrow">
                    <ArrowDown size={16} />
                  </div>
                  <div className="swap-confirm-token">
                    <TokenGlyph token={toTokenMeta || buildCustomToken(normalizedToToken)} />
                    <div>
                      <span>鑾峰緱</span>
                      <strong>{selectedQuoteAmount} {toTokenMeta?.symbol || shortAddress(normalizedToToken, 4, 4)}</strong>
                    </div>
                  </div>
                </div>
              </div>

              <div className="swap-confirm-details">
                <DetailRow label="Minimum received" value={`${minReceived} ${toTokenMeta?.symbol || ''}`.trim()} />
                <DetailRow label="婊戠偣瀹瑰繊" value={`${slippage || '1.0'}%`} />
                <DetailRow label="棰勪及 Gas" value={formatGas(quoteInfo?.estimated_gas)} />
              </div>

              <div className="swap-confirm-actions">
                <button
                  type="button"
                  className="swap-confirm-cancel"
                  onClick={() => setShowConfirm(false)}
                  disabled={executing}
                >
                  鍙栨秷
                </button>
                <button
                  type="button"
                  className="swap-submit-button compact"
                  onClick={handleSwap}
                  disabled={executing}
                >
                  {executing ? '鎻愪氦涓?..' : '鎻愪氦浜ゆ槗'}
                </button>
              </div>
            </div>
          </div>
        ) : null}
      </div>
    </PanelShell>
  );
}
