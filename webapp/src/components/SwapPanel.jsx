import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
  fetchGlobalConfig,
  fetchWallets,
  fetchWalletSwapTokenMetadata,
  walletSwapSingleExecute,
  walletSwapSingleQuote,
  walletSwapPreview,
} from '../api';
import PanelShell from './PanelShell';
import { normalizeHexAddress, shortAddress } from '../utils';
import { ArrowDown, ChevronDown, RefreshCw, Search, Wallet, X, Check } from 'lucide-react';
import bnbLogo from '../img/bnb.svg';
import baseLogo from '../img/base.svg';

const CHAIN_META = {
  bsc: {
    label: 'BNB Chain',
    nativeSymbol: 'BNB',
    icon: bnbLogo,
    iconAlt: 'BNB Chain',
    nativeLogoUrl: bnbLogo,
    wrappedNativeAddress: '0xbb4CdB9CBd36B01bD1cBaEBF2De08d9173bc095c',
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
    icon: baseLogo,
    iconAlt: 'Base',
    nativeLogoUrl: baseLogo,
    wrappedNativeAddress: '0x4200000000000000000000000000000000000006',
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
const AUTO_QUOTE_REFRESH_MS = 8000;
const NATIVE_PSEUDO_ADDRESS = '0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee';
const MIN_WALLET_TOKEN_VALUE_USD = 0.1;
const NETWORK_KEYS = ['bsc', 'base'];

// 缂備礁顦…宄扳枍鎼淬垻鈻旂€广儱顦版禒姗€鎮烽弴姘冲厡婵炲牊鍨垮浠嬪炊閳哄﹤濮版俊鐐€楅。顔炬濠靛鐭楁い蹇撳暟缁犱粙鏌ｉ敐鍡欐噧闁告﹩鍓熼獮鎴﹀閻樺樊娼梺?// const TABS = [
//   { key: 'swap', label: '闂佺绻戦崹璺虹暦?, enabled: true },
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
      name: String(token?.name || '\u81ea\u5b9a\u4e49\u4ee3\u5e01').trim() || '\u81ea\u5b9a\u4e49\u4ee3\u5e01',
      color: String(token?.color || '#7c8aa6').trim() || '#7c8aa6',
      custom: Boolean(token?.custom),
      logoUrl: String(token?.logoUrl || '').trim(),
      native: Boolean(token?.native),
      canSwap: token?.canSwap !== false,
      disabledReason: String(token?.disabledReason || '').trim(),
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
  return makeCustomToken(address);
}

function makeCustomToken(address) {
  const normalized = normalizeHexAddress(address);
  if (!normalized) return null;
  if (normalized === NATIVE_PSEUDO_ADDRESS) {
    return {
      address: normalized,
      symbol: 'NATIVE',
      name: '\u539f\u751f\u5e01',
      color: '#7c8aa6',
      custom: true,
      native: true,
      canSwap: true,
    };
  }
  return {
    address: normalized,
    symbol: shortAddress(normalized, 4, 4),
    name: '\u81ea\u5b9a\u4e49\u5408\u7ea6\u5730\u5740',
    color: '#7c8aa6',
    custom: true,
  };
}
function getChainConfig(chain) {
  return CHAIN_META[chain] || CHAIN_META.bsc;
}

function getNativePresetToken(chain) {
  const chainConfig = getChainConfig(chain);
  const nativeSymbol = String(chainConfig?.nativeSymbol || 'NATIVE').trim() || 'NATIVE';
  const wrappedToken = Array.isArray(chainConfig?.presets)
    ? chainConfig.presets.find((token) => String(token?.symbol || '').trim().startsWith('W'))
    : null;
  return {
    address: NATIVE_PSEUDO_ADDRESS,
    symbol: nativeSymbol,
    name: nativeSymbol,
    color: String(wrappedToken?.color || '#7c8aa6').trim() || '#7c8aa6',
    logoUrl: String(chainConfig?.nativeLogoUrl || '').trim(),
    native: true,
    canSwap: true,
  };
}

function getPresetTokens(chain) {
  return dedupeTokens([getNativePresetToken(chain), ...getChainConfig(chain).presets]);
}

function buildTokenLookup(tokens) {
  return dedupeTokens(tokens);
}

function resolveTokenMeta(address, tokens) {
  const normalized = normalizeHexAddress(address);
  if (!normalized) return null;
  const pool = buildTokenLookup(tokens);
  return pool.find((item) => item.address === normalized) || makeCustomToken(normalized);
}

function shouldFetchTokenMetadata(token) {
  const address = normalizeHexAddress(token?.address);
  if (!address) return false;
  if (Boolean(token?.native)) return false;
  const symbol = String(token?.symbol || '').trim();
  const name = String(token?.name || '').trim();
  const logoUrl = String(token?.logoUrl || '').trim();
  if (!logoUrl) return true;
  if (Boolean(token?.custom)) return true;
  return !symbol || !name || name === symbol;
}

function resolveNativeLogoUrl(token, tokenMetaMap, chain) {
  const chainConfig = getChainConfig(chain);
  const wrappedAddress = normalizeHexAddress(chainConfig?.wrappedNativeAddress);
  const wrappedLogoUrl = wrappedAddress ? String(tokenMetaMap?.[wrappedAddress]?.logoUrl || '').trim() : '';
  const currentLogoUrl = String(token?.logoUrl || '').trim();
  return wrappedLogoUrl || currentLogoUrl || String(chainConfig?.nativeLogoUrl || '').trim();
}

function applyTokenMetadata(token, tokenMetaMap, chain) {
  if (!token) return token;
  const address = normalizeHexAddress(token.address);
  if (!address) return token;
  if (Boolean(token.native)) {
    return {
      ...token,
      address,
      logoUrl: resolveNativeLogoUrl(token, tokenMetaMap, chain),
    };
  }
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

function formatGasUnits(value) {
  const num = Number(value);
  if (!Number.isFinite(num) || num <= 0) return '--';
  return `${num.toLocaleString('en-US', { maximumFractionDigits: 0 })} gas`;
}

function formatNativeBalance(value) {
  const num = Number(value);
  if (!Number.isFinite(num)) return '--';
  return num.toLocaleString('en-US', { minimumFractionDigits: 0, maximumFractionDigits: 4 });
}

function formatGasCost(value, symbol) {
  const num = Number(value);
  if (!Number.isFinite(num) || num <= 0) return '--';
  return `${formatTokenAmount(num)} ${symbol}`;
}

function formatGasUSD(value) {
  const num = Number(value);
  if (!Number.isFinite(num) || num <= 0) return '--';
  return `≈ ${num.toLocaleString('en-US', { maximumFractionDigits: num >= 1 ? 2 : 4 })} U`;
}

function formatQuoteClock(value) {
  const ts = Number(value);
  if (!Number.isFinite(ts) || ts <= 0) return '';
  return new Date(ts).toLocaleTimeString('zh-CN', {
    hour12: false,
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  });
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

export default function SwapPanel({ apiBaseUrl, initData, hasInitData, chain = 'bsc', onChainChange }) {
  const chainConfig = getChainConfig(chain);

  const [wallets, setWallets] = useState([]);
  const [selectedWalletId, setSelectedWalletId] = useState('');
  const [walletLoading, setWalletLoading] = useState(false);

  const [fromToken, setFromToken] = useState('');
  const [toToken, setToToken] = useState(() => chainConfig.stable.address);
  const [amount, setAmount] = useState('');
  const [slippage, setSlippage] = useState('1.0');
  const [slippageDirty, setSlippageDirty] = useState(false);
  const [walletDropdownOpen, setWalletDropdownOpen] = useState(false);

  const [quoteInfo, setQuoteInfo] = useState(null);
  const [quoting, setQuoting] = useState(false);
  const [quoteError, setQuoteError] = useState('');
  const [refreshingQuote, setRefreshingQuote] = useState(false);
  const [quoteRefreshTick, setQuoteRefreshTick] = useState(0);
  const [lastQuoteAt, setLastQuoteAt] = useState(0);

  const [executing, setExecuting] = useState(false);
  const [execError, setExecError] = useState('');
  const [execSuccess, setExecSuccess] = useState('');
  const [showConfirm, setShowConfirm] = useState(false);

  const [pickerOpen, setPickerOpen] = useState(false);
  const [pickerSide, setPickerSide] = useState('from');
  const [tokenQuery, setTokenQuery] = useState('');
  const [recentTokens, setRecentTokens] = useState(() => loadRecentTokens());

  const [walletTokens, setWalletTokens] = useState([]);
  const [walletTokensKey, setWalletTokensKey] = useState('');
  const [loadingWalletTokens, setLoadingWalletTokens] = useState(false);
  const [walletTokensError, setWalletTokensError] = useState('');
  const [tokenMetaMap, setTokenMetaMap] = useState({});

  const quoteTimeout = useRef(null);
  const quoteAbortRef = useRef(null);
  const quoteSeqRef = useRef(0);
  const lastRequestedQuoteKeyRef = useRef('');
  const walletSelectRef = useRef(null);
  const walletTokensAbortRef = useRef(null);
  const walletTokensSeqRef = useRef(0);

  const normalizedFromToken = normalizeHexAddress(fromToken);
  const normalizedToToken = normalizeHexAddress(toToken);
  const selectedWallet = useMemo(
    () => wallets.find((item) => String(item.id) === String(selectedWalletId)) || null,
    [wallets, selectedWalletId]
  );
  const currentWalletTokenKey = useMemo(
    () => (selectedWalletId ? `${chain}:${selectedWalletId}` : ''),
    [chain, selectedWalletId]
  );
  const chainOptions = useMemo(
    () => NETWORK_KEYS.map((key) => ({ key, ...CHAIN_META[key] })).filter((item) => item?.key),
    []
  );
  const hasLoadedWalletTokens = useMemo(
    () => walletTokensKey === currentWalletTokenKey,
    [currentWalletTokenKey, walletTokensKey]
  );
  const rawPresetTokens = useMemo(() => getPresetTokens(chain), [chain]);
  const rawRecentChainTokens = useMemo(
    () => dedupeTokens(recentTokens?.[chain] || []),
    [chain, recentTokens]
  );
  const presetTokens = useMemo(
    () => rawPresetTokens.map((token) => applyTokenMetadata(token, tokenMetaMap, chain)),
    [chain, rawPresetTokens, tokenMetaMap]
  );
  const recentChainTokens = useMemo(
    () => rawRecentChainTokens.map((token) => applyTokenMetadata(token, tokenMetaMap, chain)),
    [chain, rawRecentChainTokens, tokenMetaMap]
  );
  const enrichedWalletTokens = useMemo(
    () => walletTokens.map((token) => applyTokenMetadata(token, tokenMetaMap, chain)),
    [chain, walletTokens, tokenMetaMap]
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
  const quoteGasUnits = useMemo(
    () => formatGasUnits(quoteInfo?.estimated_gas),
    [quoteInfo]
  );
  const quoteGasNative = useMemo(
    () => formatGasCost(quoteInfo?.estimated_gas_native, quoteInfo?.estimated_gas_symbol || chainConfig.nativeSymbol),
    [chainConfig.nativeSymbol, quoteInfo]
  );
  const quoteGasUSD = useMemo(
    () => formatGasUSD(quoteInfo?.estimated_gas_usd),
    [quoteInfo]
  );
  const quoteStampText = useMemo(
    () => formatQuoteClock(lastQuoteAt),
    [lastQuoteAt]
  );
  const quoteGasCostText = useMemo(() => {
    if (quoteGasNative !== '--' && quoteGasUSD !== '--') return `${quoteGasNative} / ${quoteGasUSD}`;
    if (quoteGasUSD !== '--') return quoteGasUSD;
    if (quoteGasNative !== '--') return quoteGasNative;
    return '--';
  }, [quoteGasNative, quoteGasUSD]);
  const minReceived = useMemo(() => {
    const out = Number(quoteInfo?.to_amount_float);
    const slip = Number(slippage);
    if (!Number.isFinite(out) || out <= 0 || !Number.isFinite(slip) || slip < 0) return '--';
    return formatTokenAmount(out * (1 - slip / 100));
  }, [quoteInfo, slippage]);
  const quoteRequestKey = useMemo(
    () => [chain, selectedWalletId, normalizedFromToken, normalizedToToken, amount, slippage].join('|'),
    [amount, chain, normalizedFromToken, normalizedToToken, selectedWalletId, slippage]
  );
  const selectedWalletAddressLabel = useMemo(
    () => (selectedWallet ? shortAddress(selectedWallet.address, 8, 6) : '\u672a\u9009\u62e9\u94b1\u5305'),
    [selectedWallet]
  );

  const pickerTokens = useMemo(() => {
    const keyword = String(tokenQuery || '').trim().toLowerCase();

    // Merge wallet balance data into token rows.
    const enrichToken = (token) => {
      const walletToken = enrichedWalletTokens.find((wt) => wt.address === token.address);
      return {
        ...token,
        balance: walletToken?.balance || '0',
        valueUSDT: walletToken?.valueUSDT || 0,
        native: Boolean(walletToken?.native || token?.native),
        canSwap: walletToken?.canSwap ?? token?.canSwap ?? true,
        disabledReason: String(walletToken?.disabledReason || token?.disabledReason || '').trim(),
      };
    };

    // 闂佸搫鐗嗛ˇ顔剧礊閹寸儑绱ｆ繝闈涚墛閻ｅ崬霉閻欏懐鍒扮紒?
    const withBalance = enrichedWalletTokens
      .filter((wt) => matchesToken(wt, keyword))
      .map((wt) => {
        const existing = [...recentChainTokens, ...presetTokens].find((t) => t.address === wt.address);
        return existing ? enrichToken(existing) : {
          address: wt.address,
          symbol: wt.symbol,
          name: wt.name || wt.symbol,
          color: '#7c8aa6',
          balance: wt.balance,
          valueUSDT: wt.valueUSDT,
          logoUrl: wt.logoUrl || '',
          native: Boolean(wt.native),
          canSwap: wt.canSwap !== false,
          disabledReason: String(wt.disabledReason || '').trim(),
        };
      })
      .sort((a, b) => (b.valueUSDT || 0) - (a.valueUSDT || 0));

    // 闂佸搫鐗冮崑鎾诲级閳哄倸鐏ｅ┑鐐叉喘閹粙濡歌閻ｅ崬霉閻欏懐鍒扮紒?闂佸湱鍎ょ敮鈥斥枍鎼粹槅鍟呴柟缁樺笚闊剙霉閿濆棛鎳囨い銈呭€垮畷姘旈崟鈹惧亾閸愨晝鈻旀い鎾跺У閻?
    const recent = recentChainTokens
      .filter((token) => !withBalance.some((item) => item.address === token.address))
      .filter((token) => matchesToken(token, keyword))
      .map(enrichToken);

    // 闁汇埄鍨伴幗婊堝极閵堝棛顩烽柨婵嗘噽椤?闂佸湱鍎ょ敮鈥斥枍鎼粹槅鍟呴柟缁樺笚闊剙霉閿濆棛鎳囨い銈呭€垮畷顏嗕沪閻愵兛绮柡澶嗘櫆閸ㄧ敻宕归鍡樺仒闁靛鍊涢崢顒勬煟?
    const preset = presetTokens
      .filter((token) => !withBalance.some((item) => item.address === token.address))
      .filter((token) => !recent.some((item) => item.address === token.address))
      .filter((token) => matchesToken(token, keyword))
      .map(enrichToken);

    const customCandidate = applyTokenMetadata(makeCustomToken(tokenQuery), tokenMetaMap, chain);
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
    setRefreshingQuote(false);
    setQuoteRefreshTick(0);
    setLastQuoteAt(0);
    setExecError('');
    setExecSuccess('');
    setShowConfirm(false);
    setWalletTokens([]);
    setWalletTokensKey('');
    setTokenMetaMap({});
    lastRequestedQuoteKeyRef.current = '';
  }, [chainConfig.stable.address]);

  useEffect(() => {
    setSlippageDirty(false);
  }, [initData]);

  useEffect(() => {
    if (!hasInitData || !initData || slippageDirty) return undefined;
    const controller = new AbortController();
    fetchGlobalConfig({
      apiBaseUrl,
      initData,
      signal: controller.signal,
    })
      .then((resp) => {
        if (controller.signal.aborted) return;
        const cfg = resp?.config || resp || {};
        const nextSlippage = Number(cfg?.slippage_tolerance);
        if (Number.isFinite(nextSlippage) && nextSlippage > 0) {
          setSlippage(String(nextSlippage));
        }
      })
      .catch((error) => {
        if (controller.signal.aborted) return;
        console.error('fetchGlobalConfig failed', error);
      });
    return () => controller.abort();
  }, [apiBaseUrl, hasInitData, initData, slippageDirty]);

  const metadataCandidates = useMemo(() => {
    const customTokens = [
      makeCustomToken(normalizedFromToken),
      makeCustomToken(normalizedToToken),
      makeCustomToken(tokenQuery),
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
    if (!initData || !selectedWalletId) return;
    const requestKey = `${chain}:${selectedWalletId}`;
    const seq = walletTokensSeqRef.current + 1;
    walletTokensSeqRef.current = seq;
    if (walletTokensAbortRef.current) {
      walletTokensAbortRef.current.abort();
    }
    const controller = new AbortController();
    walletTokensAbortRef.current = controller;
    setLoadingWalletTokens(true);
    setWalletTokensError('');
    try {
      // 闂傚倸瀚粔宕囩礊閸℃稑瀚夐柍褜鍓涙禍姝岀疀鎼达絽绠氶梺绋匡工閵堢危閸ヮ剙纾圭痪顓㈩棑缁€澶愭煛閸曨偄鈷旈柕鍥ㄥ哺瀵挳寮堕幋顓熲柤婵炲濯寸徊鐣岀博?
      const resp = await walletSwapPreview({
        apiBaseUrl,
        initData,
        chain,
        walletId: selectedWalletId,
        minValueUsd: MIN_WALLET_TOKEN_VALUE_USD,
        signal: controller.signal,
      });
      if (controller.signal.aborted || walletTokensSeqRef.current !== seq) return;
      const tokens = (resp?.tokens || []).map((t) => {
        const address = normalizeHexAddress(t.address);
        if (!address) return null;
        return {
          address,
          symbol: t.symbol,
          name: t.name || t.symbol,
          balance: t.balance,
          valueUSDT: t.value_usdt || 0,
          logoUrl: t.logo_url || (t.is_native ? getChainConfig(chain).nativeLogoUrl || '' : ''),
          native: Boolean(t.is_native),
          canSwap: t.can_swap !== false,
          disabledReason: t.disabled_reason || '',
        };
      }).filter(Boolean);
      setWalletTokens(tokens);
      setWalletTokensKey(requestKey);
    } catch (error) {
      if (controller.signal.aborted || walletTokensSeqRef.current !== seq) return;
      console.error('loadWalletTokens failed', error);
      // 婵犮垺鍎肩划鍓ф喆閿曞倸绫嶉柡鍫㈡暩閻熸繃绻涢幘铏櫧闁宠鐗犻弫宥囦沪閼测晝顔旈梺浼欑稻閻熴倗绮婚敐澶婄鐎广儱娲﹂悾閬嶆煛娴ｅ搫顣肩€?
      setWalletTokens([]);
      setWalletTokensKey(requestKey);
      setWalletTokensError(String(error?.message || error || '\u52a0\u8f7d\u94b1\u5305\u4f59\u989d\u5931\u8d25'));
    } finally {
      if (walletTokensAbortRef.current === controller) {
        walletTokensAbortRef.current = null;
      }
      if (walletTokensSeqRef.current === seq) {
        setLoadingWalletTokens(false);
      }
    }
  }, [apiBaseUrl, initData, chain, selectedWalletId]);

  useEffect(() => {
    if (!hasInitData) return;
    loadWallets();
    setExecError('');
    setExecSuccess('');
  }, [hasInitData, loadWallets]);

  // 闂佸憡鐟禍婊冿耿椤忓牆绠ラ柟鎯х－绾捐霉閻欏懐鍒扮紒鏃傛暬閺屽懏寰勭€ｎ亶浠撮梺闈╃祷閸斿秴顪冮崒鐐茬闁绘鍎ょ粊鏉棵归敐鍡欐噰妞ゃ倕鍊块弫宥呯暆閸曨亞绱氶梺绋跨箰缁夊磭绮径搴ｇ杸闁告盯鍋婂ú锝夋煟?API 闁荤姴顑呴崯浼村极?
  useEffect(() => {
    if (walletTokensAbortRef.current) {
      walletTokensAbortRef.current.abort();
      walletTokensAbortRef.current = null;
    }
    setWalletDropdownOpen(false);
    setWalletTokens([]);
    setWalletTokensKey('');
    setWalletTokensError('');
  }, [currentWalletTokenKey]);

  useEffect(() => {
    if (!hasInitData || !currentWalletTokenKey || loadingWalletTokens) return;
    if (walletTokensKey === currentWalletTokenKey) return;
    loadWalletTokens();
  }, [currentWalletTokenKey, hasInitData, loadWalletTokens, loadingWalletTokens, walletTokensKey]);

  useEffect(() => () => {
    if (walletTokensAbortRef.current) {
      walletTokensAbortRef.current.abort();
    }
  }, []);

  useEffect(() => {
    if (!walletDropdownOpen) return undefined;
    const handlePointerDown = (event) => {
      if (walletSelectRef.current && !walletSelectRef.current.contains(event.target)) {
        setWalletDropdownOpen(false);
      }
    };
    window.addEventListener('mousedown', handlePointerDown);
    return () => window.removeEventListener('mousedown', handlePointerDown);
  }, [walletDropdownOpen]);

  const doQuote = useCallback(async ({
    amt,
    fromAddress,
    toAddress,
    walletId,
    chainId,
    slip,
    signal,
    seq,
    preservePrevious = false,
  }) => {
    const amountNumber = Number(amt);
    if (!walletId || !Number.isFinite(amountNumber) || amountNumber <= 0 || !fromAddress || !toAddress) {
      setQuoteInfo(null);
      setRefreshingQuote(false);
      setLastQuoteAt(0);
      lastRequestedQuoteKeyRef.current = '';
      setQuoteError('');
      setQuoting(false);
      return;
    }
    if (fromAddress === toAddress) {
      setQuoteInfo(null);
      setQuoteError('\u5356\u51fa\u4ee3\u5e01\u548c\u4e70\u5165\u4ee3\u5e01\u4e0d\u80fd\u76f8\u540c');
      setQuoting(false);
      setRefreshingQuote(false);
      return;
    }
    setQuoting(true);
    setRefreshingQuote(preservePrevious);
    setQuoteError('');
    if (!preservePrevious) {
      setQuoteInfo(null);
    }

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
      setLastQuoteAt(Date.now());
    } catch (error) {
      if (signal?.aborted) return;
      if (quoteSeqRef.current !== seq) return;
      if (!preservePrevious) {
        setQuoteInfo(null);
      }
      setQuoteError(String(error?.message || error));
    } finally {
      if (quoteSeqRef.current === seq) {
        setQuoting(false);
        setRefreshingQuote(false);
      }
    }
  }, [apiBaseUrl, initData]);

  useEffect(() => {
    if (quoteTimeout.current) clearTimeout(quoteTimeout.current);
    if (quoteAbortRef.current) quoteAbortRef.current.abort();

    const amountNumber = Number(amount);
    if (!selectedWalletId || !Number.isFinite(amountNumber) || amountNumber <= 0 || !normalizedFromToken || !normalizedToToken) {
      setQuoting(false);
      setRefreshingQuote(false);
      setQuoteInfo(null);
      setLastQuoteAt(0);
      lastRequestedQuoteKeyRef.current = '';
      if (normalizedFromToken && normalizedToToken && normalizedFromToken === normalizedToToken) {
        setQuoteError('\u5356\u51fa\u4ee3\u5e01\u548c\u4e70\u5165\u4ee3\u5e01\u4e0d\u80fd\u76f8\u540c');
      } else {
        setQuoteError('');
      }
      return undefined;
    }

    const preservePrevious = lastRequestedQuoteKeyRef.current === quoteRequestKey;
    const delay = preservePrevious ? 0 : 450;
    quoteTimeout.current = setTimeout(() => {
      const seq = quoteSeqRef.current + 1;
      quoteSeqRef.current = seq;
      const controller = new AbortController();
      quoteAbortRef.current = controller;
      lastRequestedQuoteKeyRef.current = quoteRequestKey;
      doQuote({
        amt: amount,
        fromAddress: normalizedFromToken,
        toAddress: normalizedToToken,
        walletId: selectedWalletId,
        chainId: chain,
        slip: slippage,
        signal: controller.signal,
        seq,
        preservePrevious,
      });
    }, delay);

    return () => {
      if (quoteTimeout.current) clearTimeout(quoteTimeout.current);
      if (quoteAbortRef.current) quoteAbortRef.current.abort();
    };
  }, [amount, chain, doQuote, normalizedFromToken, normalizedToToken, quoteRefreshTick, quoteRequestKey, selectedWalletId, slippage]);

  useEffect(() => {
    const amountNumber = Number(amount);
    if (
      !hasInitData ||
      !selectedWalletId ||
      !Number.isFinite(amountNumber) ||
      amountNumber <= 0 ||
      !normalizedFromToken ||
      !normalizedToToken ||
      normalizedFromToken === normalizedToToken ||
      executing
    ) {
      return undefined;
    }

    const timer = window.setInterval(() => {
      setQuoteRefreshTick((current) => current + 1);
    }, AUTO_QUOTE_REFRESH_MS);

    return () => window.clearInterval(timer);
  }, [amount, executing, hasInitData, normalizedFromToken, normalizedToToken, selectedWalletId]);

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
    if (token.canSwap === false) {
      setQuoteError(token.disabledReason || `${token.symbol} \u6682\u4e0d\u652f\u6301\u76f4\u63a5\u5151\u6362`);
      return;
    }
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
      setExecSuccess(resp?.tx_hash || '\\u4ea4\\u6613\\u5df2\\u63d0\\u4ea4');
      setShowConfirm(false);
      setAmount('');
      setQuoteInfo(null);
      setRefreshingQuote(false);
      setLastQuoteAt(0);
      lastRequestedQuoteKeyRef.current = '';
      // 濠电偞鎸搁幊鎰板煘閺嶃劍濯存繛鍡樻惄閺夎櫣绱撻崒娑欏碍闁宦板姂閺佸秶浠﹂懖鈺冩啴濠电偛妫岄崜婵囨櫠閿曗偓椤曪綁鍩€椤掑嫭鐒诲璺侯儏椤忋儵鏌涢敐鍐ㄥ婵＄偛鍊块弻灞筋吋閸℃鍘愰梺鍛婃⒒婵儳霉?
      setWalletTokens([]);
      setWalletTokensKey('');
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
    setRefreshingQuote(false);
    setLastQuoteAt(0);
    lastRequestedQuoteKeyRef.current = '';
    setExecError('');
    setExecSuccess('');
  };

  const handleChainSelect = useCallback((nextChain) => {
    if (!nextChain || nextChain === chain) return;
    setWalletDropdownOpen(false);
    if (typeof onChainChange === 'function') {
      onChainChange(nextChain);
    }
  }, [chain, onChainChange]);

  const renderNetworkButtons = useCallback(() => (
    <div className="swap-network-list">
      {chainOptions.map((option) => {
        const active = option.key === chain;
        return (
          <button
            key={option.key}
            type="button"
            className={`swap-network-button${active ? ' active' : ''}`}
            onClick={() => handleChainSelect(option.key)}
            aria-pressed={active}
            title={option.label}
          >
            <span className="swap-network-icon-wrap">
              <img src={option.icon} alt={option.iconAlt || option.label} className="swap-network-icon" />
            </span>
            <span className="swap-network-copy">
              <strong>{option.label}</strong>
            </span>
          </button>
        );
      })}
    </div>
  ), [chain, chainOptions, handleChainSelect]);

  const isReadyToSwap = Boolean(
    selectedWalletId &&
    normalizedFromToken &&
    normalizedToToken &&
    fromTokenMeta?.canSwap !== false &&
    toTokenMeta?.canSwap !== false &&
    normalizedFromToken !== normalizedToToken &&
    Number(amount) > 0 &&
    quoteInfo &&
    !quoting &&
    !executing
  );

  let submitLabel = '\u9884\u89c8\u5151\u6362';
  if (!selectedWalletId) submitLabel = walletLoading ? '\u52a0\u8f7d\u94b1\u5305\u4e2d...' : '\u8bf7\u5148\u9009\u62e9\u94b1\u5305';
  else if (!normalizedFromToken) submitLabel = '\u9009\u62e9\u5356\u51fa\u4ee3\u5e01';
  else if (!amount || Number(amount) <= 0) submitLabel = '\u8f93\u5165\u5356\u51fa\u6570\u91cf';
  else if (!normalizedToToken) submitLabel = '\u9009\u62e9\u4e70\u5165\u4ee3\u5e01';
  else if (fromTokenMeta?.canSwap === false || toTokenMeta?.canSwap === false) submitLabel = '\u539f\u751f\u5e01\u6682\u4e0d\u652f\u6301\u76f4\u63a5\u5151\u6362';
  else if (normalizedFromToken === normalizedToToken) submitLabel = '\u4e0d\u80fd\u5151\u6362\u540c\u4e00\u4ee3\u5e01';
  else if (quoting) submitLabel = '\u83b7\u53d6\u6700\u4f18\u62a5\u4ef7\u4e2d...';
  else if (!quoteInfo) submitLabel = '\u7b49\u5f85\u62a5\u4ef7';

  return (
    <PanelShell
      title={'\u4e00\u952e\u5151\u6362'}
      subtitle={'\u5feb\u901f\u5151\u6362\u4efb\u610f\u652f\u6301\u7684\u4ee3\u5e01'}
      icon={RefreshCw}
    >
      <div className="swap-panel">
        <div className="swap-panel-shell">
          <div className="swap-panel-topbar">
            <div>
              <div style={{ fontSize: '18px', fontWeight: '700', color: 'var(--text)' }}>
                {'\u5151\u6362'}
              </div>
            </div>
          </div>

          <div className="swap-controls-card">
              <div className="swap-control-block">
                <div className="swap-control-label-row">
                  <span className="swap-control-label">{'\u7f51\u7edc'}</span>
                  <strong>{chainConfig.label}</strong>
                </div>
              {renderNetworkButtons()}
            </div>

            <div className="swap-control-grid">
              <div className="swap-control-block">
                <span className="swap-control-label">{'\u6267\u884c\u94b1\u5305'}</span>
                <div className="swap-custom-select-wrap" ref={walletSelectRef}>
                  <button
                    type="button"
                    className="swap-custom-select-trigger"
                    onClick={() => setWalletDropdownOpen((current) => !current)}
                    disabled={walletLoading || !wallets.length}
                  >
                    <Wallet size={14} />
                    <span className="swap-custom-select-value">
                      {selectedWallet
                        ? `${selectedWallet.name || '\u94b1\u5305'} \u00b7 ${shortAddress(selectedWallet.address, 8, 6)}`
                        : walletLoading
                          ? '\u52a0\u8f7d\u94b1\u5305\u4e2d...'
                          : '\u6682\u65e0\u53ef\u7528\u94b1\u5305'}
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
                              {wallet.name || '\u94b1\u5305'}
                            </span>
                            <span className="swap-custom-select-option-address">
                              {shortAddress(wallet.address, 8, 6)}
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
              </div>

              <div className="swap-control-block">
                <div className="swap-control-label-row">
                  <span className="swap-control-label">{'\u6ed1\u70b9'}</span>
                  <strong>{`${slippage || '1.0'}%`}</strong>
                </div>
                <div className="swap-slippage-input-group">
                  <div className="swap-slippage-pills">
                    {SLIPPAGE_PRESETS.map((item) => (
                      <button
                        key={item}
                        type="button"
                        className={`swap-slippage-pill${Number(slippage) === Number(item) ? ' active' : ''}`}
                        onClick={() => {
                          setSlippage(item);
                          setSlippageDirty(true);
                        }}
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
                      onChange={(event) => {
                        setSlippage(event.target.value);
                        setSlippageDirty(true);
                      }}
                      className="swap-slippage-input"
                      placeholder="1.0"
                    />
                    <span>%</span>
                  </div>
                </div>
              </div>
            </div>
          </div>

          <div className="swap-surface">
            <div className="swap-card-group">
              <div className="swap-card">
                <div className="swap-card-head">
                  <span>{'From'}</span>
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
                        {'\u6700\u5927'}
                      </button>
                    ) : null}
                    <small>{selectedWalletAddressLabel}</small>
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
                    placeholder={'\u9009\u62e9\u4ee3\u5e01'}
                    onClick={() => {
                      setPickerSide('from');
                      setPickerOpen(true);
                    }}
                  />
                </div>
                <div className="swap-card-foot">
                  <span>{fromTokenMeta ? fromTokenMeta.name : '\u672a\u9009\u62e9\u5356\u51fa\u4ee3\u5e01'}</span>
                  <span style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
                    {fromTokenBalance && Number(fromTokenBalance) > 0 ? (
                      <>
                        <span style={{ color: 'var(--positive)' }}>{'\u4f59\u989d:'}</span>
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
                aria-label={'\u5207\u6362\u5151\u6362\u65b9\u5411'}
              >
                <ArrowDown size={18} strokeWidth={2.5} />
              </button>

              <div className="swap-card muted">
                <div className="swap-card-head">
                  <span>{'To'}</span>
                  <small>{selectedWalletAddressLabel}</small>
                </div>
                <div className="swap-card-body">
                  <div className={`swap-quote-output${quoting ? ' loading' : ''}${refreshingQuote ? ' refreshing' : ''}`}>
                    {quoting && !quoteInfo ? '...' : selectedQuoteAmount}
                  </div>
                  <TokenButton
                    token={toTokenMeta}
                    placeholder={'\u9009\u62e9\u4ee3\u5e01'}
                    onClick={() => {
                      setPickerSide('to');
                      setPickerOpen(true);
                    }}
                  />
                </div>
                <div className="swap-card-foot">
                  <span>{toTokenMeta ? toTokenMeta.name : '\u672a\u9009\u62e9\u76ee\u6807\u4ee3\u5e01'}</span>
                  <span>{`\u6700\u5c11\u5230\u8d26 ${minReceived}`}</span>
                </div>
              </div>
            </div>

            <div className={`swap-summary-card${refreshingQuote ? ' refreshing' : ''}`}>
              {quoteInfo ? (
                <>
                  <div className="swap-summary-toolbar">
                    <div className={`swap-live-badge${refreshingQuote ? ' active' : ''}`}>
                      <RefreshCw size={12} />
                      <span>{refreshingQuote ? '\u62a5\u4ef7\u5237\u65b0\u4e2d' : quoteStampText ? `\u5df2\u66f4\u65b0 ${quoteStampText}` : '\u5b9e\u65f6\u62a5\u4ef7'}</span>
                    </div>
                  </div>
                  <DetailRow
                    label={'\u9884\u4f30\u5230\u8d26'}
                    value={`${selectedQuoteAmount} ${toTokenMeta?.symbol || ''}`.trim()}
                    emphasis
                  />
                  <DetailRow label={'\u6700\u5c11\u5230\u8d26'} value={`${minReceived} ${toTokenMeta?.symbol || ''}`.trim()} />
                  <DetailRow label={'\u9884\u4f30 Gas'} value={quoteGasUnits} />
                  <DetailRow label={'Gas \u8d39\u7528'} value={quoteGasCostText} />
                  <DetailRow label={'\u6ed1\u70b9\u8bbe\u7f6e'} value={`${slippage || '1.0'}%`} />
                </>
              ) : (
                <div className="swap-summary-empty">
                  <strong>{'\u8f93\u5165\u6570\u91cf\u540e\u81ea\u52a8\u62a5\u4ef7'}</strong>
                  <span>{'\u9009\u62e9\u4ee3\u5e01\uff0c\u6216\u76f4\u63a5\u7c98\u8d34 ERC-20 \u5408\u7ea6\u5730\u5740\u3002'}</span>
                </div>
              )}
            </div>

            {quoteError ? (
              <div className="panel-error">
                <strong>{'\u62a5\u4ef7\u5931\u8d25:'}</strong> {quoteError}
              </div>
            ) : null}

            {execError ? (
              <div className="panel-error">
                <strong>{'\u5151\u6362\u5931\u8d25:'}</strong> {execError}
              </div>
            ) : null}

            {execSuccess ? (
              <div className="panel-success swap-success-card">
                <strong>{'\u5151\u6362\u8bf7\u6c42\u5df2\u63d0\u4ea4'}</strong>
                <span>{execSuccess}</span>
              </div>
            ) : null}

            <button
              type="button"
              className="swap-submit-button"
              disabled={!isReadyToSwap}
              onClick={() => setShowConfirm(true)}
            >
              {executing ? '\u6267\u884c\u4e2d...' : submitLabel}
            </button>
          </div>
        </div>

        {pickerOpen ? (
          <div
            className="swap-overlay-backdrop"
            onClick={() => {
              setPickerOpen(false);
            }}
          />
        ) : null}

        {pickerOpen ? (
          <div className="swap-modal-overlay" style={{ background: 'transparent' }}>
            <div className="swap-token-modal" onClick={(event) => event.stopPropagation()}>
              <div className="swap-modal-header">
                <div>
                  <div className="swap-modal-kicker">{'\u9009\u62e9\u4ee3\u5e01'}</div>
                  <h3>{pickerSide === 'from' ? '\u9009\u62e9\u5356\u51fa\u4ee3\u5e01' : '\u9009\u62e9\u4e70\u5165\u4ee3\u5e01'}</h3>
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
                  placeholder={'\u641c\u7d22\u4ee3\u5e01\u540d\u79f0\u3001\u7b26\u53f7\uff0c\u6216\u7c98\u8d34\u5408\u7ea6\u5730\u5740'}
                  autoFocus
                />
                {loadingWalletTokens && walletTokens.length > 0 ? (
                  <div style={{ fontSize: '11px', color: 'var(--text-muted)', whiteSpace: 'nowrap' }}>
                    {'\u5237\u65b0\u4e2d...'}
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
                    <div style={{ marginBottom: '8px' }}>{'\u6b63\u5728\u52a0\u8f7d\u94b1\u5305\u4f59\u989d...'}</div>
                    <div style={{ fontSize: '11px', opacity: '0.7' }}>{'\u9996\u6b21\u52a0\u8f7d\u53ef\u80fd\u9700\u8981\u51e0\u79d2\u949f'}</div>
                  </div>
                ) : null}

                {!loadingWalletTokens && walletTokensError ? (
                  <div style={{
                    margin: '0 0 12px',
                    padding: '12px 14px',
                    borderRadius: '16px',
                    border: '1px solid rgba(248, 113, 113, 0.22)',
                    background: 'rgba(127, 29, 29, 0.18)',
                    color: '#fecaca',
                  }}>
                    <div style={{ fontSize: '12px', fontWeight: 700, marginBottom: '4px' }}>
                      {'\u94b1\u5305\u4f59\u989d\u52a0\u8f7d\u5931\u8d25'}
                    </div>
                    <div style={{ fontSize: '12px', lineHeight: 1.6, color: '#fca5a5' }}>
                      {walletTokensError}
                    </div>
                    <button
                      type="button"
                      onClick={() => loadWalletTokens()}
                      style={{
                        marginTop: '10px',
                        padding: '6px 10px',
                        borderRadius: '10px',
                        border: '1px solid rgba(254, 202, 202, 0.24)',
                        background: 'rgba(254, 202, 202, 0.08)',
                        color: '#fee2e2',
                        fontSize: '11px',
                        fontWeight: 700,
                        cursor: 'pointer',
                      }}
                    >
                      {'\u91cd\u8bd5 OKX \u67e5\u8be2'}
                    </button>
                  </div>
                ) : null}

                {!loadingWalletTokens && !walletTokensError && hasLoadedWalletTokens && walletTokens.length === 0 ? (
                  <div style={{
                    margin: '0 0 12px',
                    padding: '12px 14px',
                    borderRadius: '16px',
                    border: '1px solid rgba(148, 163, 184, 0.16)',
                    background: 'rgba(15, 23, 42, 0.42)',
                    color: 'var(--text-muted)',
                  }}>
                    <div style={{ fontSize: '12px', fontWeight: 700, marginBottom: '4px', color: 'var(--text-primary)' }}>
                      {'\u94b1\u5305\u4f59\u989d'}
                    </div>
                    <div style={{ fontSize: '12px', lineHeight: 1.6 }}>
                      {`OKX \u672a\u8fd4\u56de\u5f53\u524d\u94b1\u5305\u4e2d\u4ef7\u503c >= $${MIN_WALLET_TOKEN_VALUE_USD.toFixed(1)} \u7684\u4ee3\u5e01\u4f59\u989d\u3002`}
                    </div>
                  </div>
                ) : null}

                {pickerTokens.customCandidate ? (
                  <div className="swap-token-section">
                    <div className="swap-token-section-title">{'\u81ea\u5b9a\u4e49\u5730\u5740'}</div>
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
                      <span className="swap-token-tag">{'\u7c98\u8d34\u4f7f\u7528'}</span>
                    </button>
                  </div>
                ) : null}

                {pickerTokens.withBalance && pickerTokens.withBalance.length > 0 ? (
                  <div className="swap-token-section">
                    <div className="swap-token-section-title">{'\u94b1\u5305\u4f59\u989d'}</div>
                    {pickerTokens.withBalance.map((token) => (
                      <button
                        key={token.address}
                        type="button"
                        className={`swap-token-row${token.canSwap === false ? ' disabled' : ''}`}
                        onClick={() => handleSelectToken(token)}
                        disabled={token.canSwap === false}
                      >
                        <TokenGlyph token={token} />
                        <div className="swap-token-row-copy">
                          <strong>{token.symbol}</strong>
                          <span style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
                            {token.canSwap === false ? (token.disabledReason || token.name) : token.name}
                            {token.valueUSDT > 0 ? (
                              <span style={{ color: '#a0a8ba', fontSize: '11px' }}>
                                {`\u2248 $${token.valueUSDT.toFixed(2)}`}
                              </span>
                            ) : null}
                          </span>
                        </div>
                        <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'flex-end', gap: '2px' }}>
                          <span className="swap-token-tag" style={{ background: 'rgba(34, 197, 94, 0.08)', borderColor: 'rgba(34, 197, 94, 0.22)', color: '#22c55e' }}>
                            {formatTokenAmount(token.balance)}
                          </span>
                          {token.canSwap === false ? (
                            <span className="swap-token-tag muted">{'\u539f\u751f\u5e01'}</span>
                          ) : null}
                        </div>
                      </button>
                    ))}
                  </div>
                ) : null}

                {pickerTokens.recent.length ? (
                  <div className="swap-token-section">
                    <div className="swap-token-section-title">{'\u6700\u8fd1\u4f7f\u7528'}</div>
                    {pickerTokens.recent.map((token) => (
                      <button
                        key={token.address}
                        type="button"
                        className={`swap-token-row${token.canSwap === false ? ' disabled' : ''}`}
                        onClick={() => handleSelectToken(token)}
                        disabled={token.canSwap === false}
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
                          <span className="swap-token-tag">{'\u6700\u8fd1'}</span>
                        </div>
                      </button>
                    ))}
                  </div>
                ) : null}

                {pickerTokens.preset.length ? (
                  <div className="swap-token-section">
                    <div className="swap-token-section-title">{'\u5e38\u7528\u4ee3\u5e01'}</div>
                    {pickerTokens.preset.map((token) => (
                      <button
                        key={token.address}
                        type="button"
                        className={`swap-token-row${token.canSwap === false ? ' disabled' : ''}`}
                        onClick={() => handleSelectToken(token)}
                        disabled={token.canSwap === false}
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
                    <strong>{'\u6ca1\u6709\u5339\u914d\u7ed3\u679c'}</strong>
                    <span>{'\u4e5f\u53ef\u4ee5\u76f4\u63a5\u7c98\u8d34 ERC-20 \u5408\u7ea6\u5730\u5740\u3002'}</span>
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
                  <div className="swap-modal-kicker">{'\u786e\u8ba4\u5151\u6362'}</div>
                  <h3>{'\u786e\u8ba4\u5151\u6362'}</h3>
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
                    <TokenGlyph token={fromTokenMeta || makeCustomToken(normalizedFromToken)} />
                    <div>
                      <span>{'\u652f\u4ed8'}</span>
                      <strong>{amount} {fromTokenMeta?.symbol || shortAddress(normalizedFromToken, 4, 4)}</strong>
                    </div>
                  </div>
                  <div className="swap-confirm-arrow">
                    <ArrowDown size={16} />
                  </div>
                  <div className="swap-confirm-token">
                    <TokenGlyph token={toTokenMeta || makeCustomToken(normalizedToToken)} />
                    <div>
                      <span>{'\u83b7\u5f97'}</span>
                      <strong>{selectedQuoteAmount} {toTokenMeta?.symbol || shortAddress(normalizedToToken, 4, 4)}</strong>
                    </div>
                  </div>
                </div>
              </div>

              <div className="swap-confirm-details">
                <DetailRow label={'\u6700\u5c11\u5230\u8d26'} value={`${minReceived} ${toTokenMeta?.symbol || ''}`.trim()} />
                <DetailRow label={'\u6ed1\u70b9\u5bb9\u5fcd'} value={`${slippage || '1.0'}%`} />
                <DetailRow label={'\u9884\u4f30 Gas'} value={quoteGasUnits} />
                <DetailRow label={'Gas \u8d39\u7528'} value={quoteGasCostText} />
              </div>

              <div className="swap-confirm-actions">
                <button
                  type="button"
                  className="swap-confirm-cancel"
                  onClick={() => setShowConfirm(false)}
                  disabled={executing}
                >
                  {'\u53d6\u6d88'}
                </button>
                <button
                  type="button"
                  className="swap-submit-button compact"
                  onClick={handleSwap}
                  disabled={executing || quoting}
                >
                  {executing ? '\u63d0\u4ea4\u4e2d...' : quoting ? '\u62a5\u4ef7\u5237\u65b0\u4e2d...' : '\u63d0\u4ea4\u4ea4\u6613'}
                </button>
              </div>
            </div>
          </div>
        ) : null}
      </div>
    </PanelShell>
  );
}
