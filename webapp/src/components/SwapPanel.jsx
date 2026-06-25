import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
  cancelWalletSwapLimitOrder,
  createWalletSwapLimitOrder,
  fetchWalletSwapHistory,
  fetchWalletSwapLimitOrders,
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
import pancakeLogo from '../img/pancake.svg';
import uniswapLogo from '../img/uniswap.svg';
import okxLogo from '../img/okx.svg';

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
  const raw = window.localStorage.getItem(RECENT_STORAGE_KEY);
  if (!raw) return {};
  const parsed = JSON.parse(raw);
  if (!parsed || typeof parsed !== 'object') return {};
  return parsed;
}

function saveRecentTokens(next) {
  window.localStorage.setItem(RECENT_STORAGE_KEY, JSON.stringify(next));
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

function buildHistoryTokenMeta(token, tokenMetaMap, chain) {
  if (!token?.address) return null;
  return applyTokenMetadata({
    address: token.address,
    symbol: String(token.symbol || '').trim() || shortAddress(token.address, 4, 4),
    name: String(token.name || token.symbol || '').trim() || shortAddress(token.address, 4, 4),
    logoUrl: String(token.logo_url || '').trim(),
    native: Boolean(token.is_native),
    color: '#7c8aa6',
  }, tokenMetaMap, chain);
}

function formatTokenAmount(value) {
  const num = Number(value);
  if (!Number.isFinite(num) || num <= 0) return '0.0';
  if (num >= 1000) return num.toLocaleString('en-US', { maximumFractionDigits: 2 });
  if (num >= 1) return num.toLocaleString('en-US', { maximumFractionDigits: 6 });
  return num.toLocaleString('en-US', { maximumFractionDigits: 8 });
}

function formatRawTokenAmount(rawValue, decimalsValue) {
  const raw = String(rawValue ?? '').trim();
  const decimals = Number(decimalsValue);
  if (!/^\d+$/.test(raw) || !Number.isInteger(decimals) || decimals < 0) return '';
  const trimmedRaw = raw.replace(/^0+/, '') || '0';
  if (trimmedRaw === '0') return '0';
  if (decimals === 0) return trimmedRaw;
  if (trimmedRaw.length <= decimals) {
    const frac = `${'0'.repeat(decimals - trimmedRaw.length)}${trimmedRaw}`.replace(/0+$/, '');
    return frac ? `0.${frac}` : '0';
  }
  const intPart = trimmedRaw.slice(0, trimmedRaw.length - decimals);
  const fracPart = trimmedRaw.slice(trimmedRaw.length - decimals).replace(/0+$/, '');
  return fracPart ? `${intPart}.${fracPart}` : intPart;
}

function formatGasUnits(value) {
  const raw = String(value ?? '').trim();
  const num = raw.startsWith('0x') ? Number.parseInt(raw, 16) : Number(raw);
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

function formatQuoteGasCostSummary(quote, nativeSymbol) {
  if (!quote) return '--';
  const gasNative = formatGasCost(quote?.estimated_gas_native, quote?.estimated_gas_symbol || nativeSymbol);
  const gasUSD = formatGasUSD(quote?.estimated_gas_usd);
  if (gasNative !== '--' && gasUSD !== '--') return `${gasNative} / ${gasUSD}`;
  if (gasUSD !== '--') return gasUSD;
  if (gasNative !== '--') return gasNative;
  return '--';
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

const DEX_ICON_MAP = [
  { match: ['pancake', 'pcs'], src: pancakeLogo, label: 'PancakeSwap', color: '#d1884f' },
  { match: ['uniswap', 'uni v'], src: uniswapLogo, label: 'Uniswap', color: '#ff007a' },
  { match: ['sushi'], src: null, label: 'SushiSwap', color: '#e05daa', fallbackLetter: 'S' },
  { match: ['curve'], src: null, label: 'Curve', color: '#ff2d55', fallbackLetter: 'C' },
  { match: ['balancer'], src: null, label: 'Balancer', color: '#1e1e1e', fallbackLetter: 'B' },
  { match: ['1inch'], src: null, label: '1inch', color: '#1b314f', fallbackLetter: '1' },
  { match: ['dodo'], src: null, label: 'DODO', color: '#ffe804', fallbackLetter: 'D' },
  { match: ['kyber'], src: null, label: 'KyberSwap', color: '#31cb9e', fallbackLetter: 'K' },
  { match: ['aerodrome'], src: null, label: 'Aerodrome', color: '#0052ff', fallbackLetter: 'A' },
  { match: ['velodrome'], src: null, label: 'Velodrome', color: '#0052ff', fallbackLetter: 'V' },
];

const PROVIDER_ICON_MAP = {
  okx: { src: okxLogo, color: '#000' },
  binance: { src: bnbLogo, color: '#f0b90b' },
};

function getDexIconInfo(name) {
  const lower = String(name || '').toLowerCase();
  for (const entry of DEX_ICON_MAP) {
    if (entry.match.some((keyword) => lower.includes(keyword))) {
      return entry;
    }
  }
  return null;
}

function getProviderIcon(provider) {
  const key = String(provider || '').toLowerCase().trim();
  return PROVIDER_ICON_MAP[key] || null;
}

function shouldShowSwapRoute(provider) {
  return Boolean(String(provider || '').trim());
}

function quoteSelectionKey(quote) {
  return String(quote?.quote_id || quote?.provider || '').trim();
}

function DexIconBadge({ name, size = 16 }) {
  const info = getDexIconInfo(name);
  const versionMatch = String(name || '').match(/[vV](\d+)/);
  const version = versionMatch ? `V${versionMatch[1]}` : '';
  if (info?.src) {
    return (
      <span className="swap-dex-icon-badge" title={name}>
        <img src={info.src} alt={info.label} style={{ width: size, height: size, borderRadius: 3 }} />
        {version ? <small className="swap-dex-version">{version}</small> : null}
      </span>
    );
  }
  if (info) {
    return (
      <span className="swap-dex-icon-badge" title={name}>
        <span className="swap-dex-icon-letter" style={{ '--dex-color': info.color, width: size, height: size }}>{info.fallbackLetter}</span>
        {version ? <small className="swap-dex-version">{version}</small> : null}
      </span>
    );
  }
  // Unknown DEX - show first letter
  const letter = String(name || '?').trim().charAt(0).toUpperCase();
  return (
    <span className="swap-dex-icon-badge" title={name}>
      <span className="swap-dex-icon-letter" style={{ '--dex-color': '#4a5568', width: size, height: size }}>{letter}</span>
      {version ? <small className="swap-dex-version">{version}</small> : null}
    </span>
  );
}

function RouteDexIcons({ routeSummary, route }) {
  // Try to extract DEX names from route array first, fallback to route_summary text
  const dexNames = useMemo(() => {
    if (Array.isArray(route) && route.length > 0) {
      const seen = new Set();
      return route
        .map((hop) => String(hop?.source || hop?.tool || '').trim())
        .filter((name) => {
          if (!name || seen.has(name)) return false;
          seen.add(name);
          return true;
        });
    }
    const summary = String(routeSummary || '').trim();
    if (!summary || summary === '--') return [];
    return summary.split(/\s*->\s*/).map((s) => s.trim()).filter(Boolean);
  }, [route, routeSummary]);

  if (!dexNames.length) return null;

  return (
    <span className="swap-dex-route-icons">
      {dexNames.map((name, i) => (
        <React.Fragment key={`${name}-${i}`}>
          {i > 0 ? <span className="swap-dex-arrow" /> : null}
          <DexIconBadge name={name} size={16} />
        </React.Fragment>
      ))}
    </span>
  );
}

function ProviderIcon({ provider, size = 18 }) {
  const icon = getProviderIcon(provider);
  if (icon?.src) {
    return <img src={icon.src} alt={provider} className="swap-prov-icon" style={{ width: size, height: size }} />;
  }
  return null;
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
  const [swapMode, setSwapMode] = useState('market');
  const [limitTargetMode, setLimitTargetMode] = useState('to_amount');
  const [limitTargetToAmount, setLimitTargetToAmount] = useState('');
  const [limitTargetPrice, setLimitTargetPrice] = useState('');

  const [quoteInfo, setQuoteInfo] = useState(null);
  const [selectedProvider, setSelectedProvider] = useState('');
  const [quoting, setQuoting] = useState(false);
  const [quoteError, setQuoteError] = useState('');
  const [refreshingQuote, setRefreshingQuote] = useState(false);
  const [quoteRefreshTick, setQuoteRefreshTick] = useState(0);
  const [lastQuoteAt, setLastQuoteAt] = useState(0);

  const [executing, setExecuting] = useState(false);
  const [execError, setExecError] = useState('');
  const [execSuccess, setExecSuccess] = useState('');
  const [execResult, setExecResult] = useState(null);
  const [showConfirm, setShowConfirm] = useState(false);

  const [pickerOpen, setPickerOpen] = useState(false);
  const [pickerSide, setPickerSide] = useState('from');
  const [tokenQuery, setTokenQuery] = useState('');
  const [recentTokens, setRecentTokens] = useState(() => loadRecentTokens());

  const [walletTokens, setWalletTokens] = useState([]);
  const [walletTokensKey, setWalletTokensKey] = useState('');
  const [loadingWalletTokens, setLoadingWalletTokens] = useState(false);
  const [walletTokensError, setWalletTokensError] = useState('');
  const [swapHistory, setSwapHistory] = useState([]);
  const [loadingSwapHistory, setLoadingSwapHistory] = useState(false);
  const [swapHistoryError, setSwapHistoryError] = useState('');
  const [limitOrders, setLimitOrders] = useState([]);
  const [loadingLimitOrders, setLoadingLimitOrders] = useState(false);
  const [limitOrdersError, setLimitOrdersError] = useState('');
  const [limitOrderBusyId, setLimitOrderBusyId] = useState('');
  const [tokenMetaMap, setTokenMetaMap] = useState({});

  const quoteTimeout = useRef(null);
  const quoteAbortRef = useRef(null);
  const quoteSeqRef = useRef(0);
  const lastRequestedQuoteKeyRef = useRef('');
  const walletSelectRef = useRef(null);
  const walletTokensAbortRef = useRef(null);
  const walletTokensSeqRef = useRef(0);
  const swapHistoryAbortRef = useRef(null);
  const swapHistorySeqRef = useRef(0);
  const limitOrdersAbortRef = useRef(null);
  const limitOrdersSeqRef = useRef(0);

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
  const providerQuotes = useMemo(
    () => Array.isArray(quoteInfo?.quotes) ? quoteInfo.quotes : [],
    [quoteInfo]
  );
  const selectedQuote = useMemo(() => {
    if (!providerQuotes.length) return null;
    if (selectedProvider) {
      const hit = providerQuotes.find((item) => quoteSelectionKey(item) === selectedProvider);
      if (hit) return hit;
    }
    if (quoteInfo?.best_quote_id || quoteInfo?.best_provider) {
      const best = providerQuotes.find((item) => {
        const bestQuoteID = String(quoteInfo?.best_quote_id || '').trim();
        if (bestQuoteID && String(item?.quote_id || '').trim() === bestQuoteID) return true;
        return !bestQuoteID && item?.provider === quoteInfo.best_provider;
      });
      if (best) return best;
    }
    return providerQuotes[0] || null;
  }, [providerQuotes, quoteInfo, selectedProvider]);
  const availableProviderCount = useMemo(
    () => providerQuotes.filter((item) => item?.status === 'available').length,
    [providerQuotes]
  );

  const selectedQuoteAmount = useMemo(
    () => formatTokenAmount(selectedQuote?.net_to_amount_float || quoteInfo?.to_amount_float),
    [quoteInfo, selectedQuote]
  );
  const quoteGasUnits = useMemo(
    () => formatGasUnits(selectedQuote?.estimated_gas),
    [selectedQuote]
  );
  const quoteGasNative = useMemo(
    () => formatGasCost(selectedQuote?.estimated_gas_native, selectedQuote?.estimated_gas_symbol || chainConfig.nativeSymbol),
    [chainConfig.nativeSymbol, selectedQuote]
  );
  const quoteGasUSD = useMemo(
    () => formatGasUSD(selectedQuote?.estimated_gas_usd),
    [selectedQuote]
  );
  const clearExecutionFeedback = useCallback(() => {
    setExecError('');
    setExecSuccess('');
    setExecResult(null);
  }, []);
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
  const selectedQuoteFeeDisplay = useMemo(() => {
    const rule = selectedQuote?.fee_rule || '';
    if (!rule) return '--';
    // Try to extract percentage from fee_rule like "交易额 0.15%" or "交易额 0.25%"
    const pctMatch = rule.match(/([\d.]+)\s*%/);
    if (pctMatch) {
      const pct = Number(pctMatch[1]);
      const netAmount = Number(selectedQuote?.net_to_amount_float);
      if (Number.isFinite(pct) && pct > 0 && Number.isFinite(netAmount) && netAmount > 0) {
        // fee ≈ netAmount * pct / (100 - pct)
        const feeUsd = netAmount * pct / (100 - pct);
        if (feeUsd > 0.001) {
          const feeStr = feeUsd.toLocaleString('en-US', { maximumFractionDigits: feeUsd >= 1 ? 2 : 4 });
          return `${rule} ≈ ${feeStr} U`;
        }
      }
    }
    return rule;
  }, [selectedQuote]);
  const selectedQuoteRouteText = useMemo(
    () => selectedQuote?.route_summary || '--',
    [selectedQuote]
  );
  const showSelectedQuoteRoute = useMemo(
    () => selectedQuote?.status === 'available' && shouldShowSwapRoute(selectedQuote?.provider),
    [selectedQuote]
  );
  const minReceived = useMemo(() => {
    const fromQuote = Number(selectedQuote?.min_to_amount_float);
    if (Number.isFinite(fromQuote) && fromQuote > 0) return formatTokenAmount(fromQuote);
    const out = Number(selectedQuote?.net_to_amount_float || quoteInfo?.to_amount_float);
    const slip = Number(slippage);
    if (!Number.isFinite(out) || out <= 0 || !Number.isFinite(slip) || slip < 0) return '--';
    return formatTokenAmount(out * (1 - slip / 100));
  }, [quoteInfo, selectedQuote, slippage]);
  const limitTargetToAmountNumber = useMemo(() => Number(limitTargetToAmount), [limitTargetToAmount]);
  const limitTargetPriceNumber = useMemo(() => Number(limitTargetPrice), [limitTargetPrice]);
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

    const recent = recentChainTokens
      .filter((token) => !withBalance.some((item) => item.address === token.address))
      .filter((token) => matchesToken(token, keyword))
      .map(enrichToken);

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
    setExecResult(null);
    setShowConfirm(false);
    setWalletTokens([]);
    setWalletTokensKey('');
    setSwapHistory([]);
    setSwapHistoryError('');
    setLimitOrders([]);
    setLimitOrdersError('');
    setLimitTargetToAmount('');
    setLimitTargetPrice('');
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
        throw error;
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
        throw error;
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
      throw error;
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
          balanceRaw: t.balance_raw || '',
          decimals: Number.isFinite(Number(t.decimals)) ? Number(t.decimals) : null,
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
      // 加载失败时清空当前余额，避免展示过期数据。
      setWalletTokens([]);
      setWalletTokensKey(requestKey);
      setWalletTokensError(String(error?.message || error || '\u52a0\u8f7d\u94b1\u5305\u4f59\u989d\u5931\u8d25'));
      throw error;
    } finally {
      if (walletTokensAbortRef.current === controller) {
        walletTokensAbortRef.current = null;
      }
      if (walletTokensSeqRef.current === seq) {
        setLoadingWalletTokens(false);
      }
    }
  }, [apiBaseUrl, initData, chain, selectedWalletId]);

  const loadSwapHistory = useCallback(async () => {
    if (!initData || !selectedWalletId) return;
    const seq = swapHistorySeqRef.current + 1;
    swapHistorySeqRef.current = seq;
    if (swapHistoryAbortRef.current) {
      swapHistoryAbortRef.current.abort();
    }
    const controller = new AbortController();
    swapHistoryAbortRef.current = controller;
    setLoadingSwapHistory(true);
    setSwapHistoryError('');
    try {
      const resp = await fetchWalletSwapHistory({
        apiBaseUrl,
        initData,
        chain,
        walletId: selectedWalletId,
        limit: 8,
        signal: controller.signal,
      });
      if (controller.signal.aborted || swapHistorySeqRef.current !== seq) return;
      setSwapHistory(Array.isArray(resp?.records) ? resp.records : []);
    } catch (error) {
      if (controller.signal.aborted || swapHistorySeqRef.current !== seq) return;
      console.error('loadSwapHistory failed', error);
      setSwapHistory([]);
      setSwapHistoryError(String(error?.message || error || '加载兑换历史失败'));
      throw error;
    } finally {
      if (swapHistoryAbortRef.current === controller) {
        swapHistoryAbortRef.current = null;
      }
      if (swapHistorySeqRef.current === seq) {
        setLoadingSwapHistory(false);
      }
    }
  }, [apiBaseUrl, initData, chain, selectedWalletId]);

  const loadLimitOrders = useCallback(async () => {
    if (!initData || !selectedWalletId) return;
    const seq = limitOrdersSeqRef.current + 1;
    limitOrdersSeqRef.current = seq;
    if (limitOrdersAbortRef.current) {
      limitOrdersAbortRef.current.abort();
    }
    const controller = new AbortController();
    limitOrdersAbortRef.current = controller;
    setLoadingLimitOrders(true);
    setLimitOrdersError('');
    try {
      const resp = await fetchWalletSwapLimitOrders({
        apiBaseUrl,
        initData,
        chain,
        walletId: selectedWalletId,
        limit: 20,
        signal: controller.signal,
      });
      if (controller.signal.aborted || limitOrdersSeqRef.current !== seq) return;
      setLimitOrders(Array.isArray(resp?.orders) ? resp.orders : []);
    } catch (error) {
      if (controller.signal.aborted || limitOrdersSeqRef.current !== seq) return;
      console.error('loadLimitOrders failed', error);
      setLimitOrders([]);
      setLimitOrdersError(String(error?.message || error || '加载限价单失败'));
      throw error;
    } finally {
      if (limitOrdersAbortRef.current === controller) {
        limitOrdersAbortRef.current = null;
      }
      if (limitOrdersSeqRef.current === seq) {
        setLoadingLimitOrders(false);
      }
    }
  }, [apiBaseUrl, initData, chain, selectedWalletId]);

  useEffect(() => {
    if (!hasInitData) return;
    loadWallets();
    clearExecutionFeedback();
  }, [clearExecutionFeedback, hasInitData, loadWallets]);

  useEffect(() => {
    if (walletTokensAbortRef.current) {
      walletTokensAbortRef.current.abort();
      walletTokensAbortRef.current = null;
    }
    setWalletDropdownOpen(false);
    setWalletTokens([]);
    setWalletTokensKey('');
    setWalletTokensError('');
    setSwapHistory([]);
    setSwapHistoryError('');
    setLimitOrders([]);
    setLimitOrdersError('');
    clearExecutionFeedback();
  }, [clearExecutionFeedback, currentWalletTokenKey]);

  useEffect(() => {
    if (!hasInitData || !currentWalletTokenKey || loadingWalletTokens) return;
    if (walletTokensKey === currentWalletTokenKey) return;
    loadWalletTokens();
  }, [currentWalletTokenKey, hasInitData, loadWalletTokens, loadingWalletTokens, walletTokensKey]);

  useEffect(() => {
    if (!hasInitData || !currentWalletTokenKey) return;
    loadSwapHistory();
    loadLimitOrders();
  }, [currentWalletTokenKey, hasInitData, loadLimitOrders, loadSwapHistory]);

  useEffect(() => () => {
    if (walletTokensAbortRef.current) {
      walletTokensAbortRef.current.abort();
    }
    if (swapHistoryAbortRef.current) {
      swapHistoryAbortRef.current.abort();
    }
    if (limitOrdersAbortRef.current) {
      limitOrdersAbortRef.current.abort();
    }
  }, []);

  useEffect(() => {
    if (!providerQuotes.length) {
      if (selectedProvider) setSelectedProvider('');
      return;
    }
    const currentExists = selectedProvider && providerQuotes.some((item) => quoteSelectionKey(item) === selectedProvider);
    if (currentExists) return;
    const preferred = quoteInfo?.best_quote_id
      ? providerQuotes.find((item) => String(item?.quote_id || '').trim() === String(quoteInfo.best_quote_id).trim())
      : null;
    const nextQuote = preferred || providerQuotes.find((item) => item?.status === 'available') || providerQuotes[0];
    const nextProvider = quoteSelectionKey(nextQuote);
    if (nextProvider && nextProvider !== selectedProvider) {
      setSelectedProvider(nextProvider);
    }
  }, [providerQuotes, quoteInfo, selectedProvider]);

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
      setQuoteError(resp?.available_count > 0 ? '' : String(resp?.message || '\u6682\u65e0\u53ef\u7528\u62a5\u4ef7'));
      setLastQuoteAt(Date.now());
    } catch (error) {
      if (signal?.aborted) return;
      if (quoteSeqRef.current !== seq) return;
      if (!preservePrevious) {
        setQuoteInfo(null);
      }
      setQuoteError(String(error?.message || error));
      throw error;
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
    clearExecutionFeedback();
  }, [clearExecutionFeedback, persistRecentToken, pickerSide]);

  const handleSwap = async () => {
    if (!initData || !normalizedFromToken || !normalizedToToken || !selectedQuote?.provider) return;
    if (quoteTimeout.current) clearTimeout(quoteTimeout.current);
    if (quoteAbortRef.current) quoteAbortRef.current.abort();
    setExecuting(true);
    setExecError('');
    setExecSuccess('');
    setExecResult(null);
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
        provider: selectedQuote.provider,
        quoteId: selectedQuote.quote_id,
      });
      setExecSuccess(resp?.message || '\\u5151\\u6362\\u5df2\\u5b8c\\u6210');
      setExecResult(resp || null);
      setShowConfirm(false);
      setAmount('');
      setQuoteInfo(null);
      setQuoting(false);
      setQuoteError('');
      setRefreshingQuote(false);
      setLastQuoteAt(0);
      lastRequestedQuoteKeyRef.current = '';
      // 交易完成后刷新钱包、余额和历史记录。
      await Promise.all([loadWallets(), loadWalletTokens(), loadSwapHistory()]);
    } catch (error) {
      setExecError(String(error?.message || error));
      setShowConfirm(false);
      throw error;
    } finally {
      setExecuting(false);
    }
  };

  const handleCreateLimitOrder = async () => {
    if (!initData || !normalizedFromToken || !normalizedToToken) return;
    setExecuting(true);
    setExecError('');
    setExecSuccess('');
    setExecResult(null);
    try {
      const targetToAmount = limitTargetMode === 'to_amount' ? limitTargetToAmount : '';
      const targetPrice = limitTargetMode === 'price' ? limitTargetPrice : '';
      const resp = await createWalletSwapLimitOrder({
        apiBaseUrl,
        initData,
        chain,
        walletId: selectedWalletId,
        fromToken: normalizedFromToken,
        toToken: normalizedToToken,
        amount,
        targetToAmount,
        targetPrice,
        slippagePercent: Number.parseFloat(slippage),
        provider: selectedQuote?.provider || quoteInfo?.best_provider || 'best',
      });
      setExecSuccess(resp?.message || '限价单已创建');
      setExecResult(resp?.order || null);
      setLimitTargetToAmount('');
      setLimitTargetPrice('');
      await loadLimitOrders();
    } catch (error) {
      setExecError(String(error?.message || error));
      throw error;
    } finally {
      setExecuting(false);
    }
  };

  const handleCancelLimitOrder = async (orderId) => {
    if (!initData || !orderId) return;
    setLimitOrderBusyId(String(orderId));
    setLimitOrdersError('');
    try {
      await cancelWalletSwapLimitOrder({
        apiBaseUrl,
        initData,
        chain,
        orderId,
      });
      await loadLimitOrders();
    } catch (error) {
      setLimitOrdersError(String(error?.message || error || '取消限价单失败'));
      throw error;
    } finally {
      setLimitOrderBusyId('');
    }
  };

  const handleMaxAmount = () => {
    if (!normalizedFromToken) return;
    const walletToken = walletTokens.find((t) => t.address === normalizedFromToken);
    if (walletToken && walletToken.balance) {
      clearExecutionFeedback();
      const preciseBalance = formatRawTokenAmount(walletToken.balanceRaw, walletToken.decimals);
      if (preciseBalance) {
        setAmount(preciseBalance);
      }
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
    clearExecutionFeedback();
  };

  const handleChainSelect = useCallback((nextChain) => {
    if (!nextChain || nextChain === chain) return;
    setWalletDropdownOpen(false);
    if (typeof onChainChange === 'function') {
      onChainChange(nextChain);
    }
  }, [chain, onChainChange]);

  const baseSwapReady = Boolean(
    selectedWalletId &&
    normalizedFromToken &&
    normalizedToToken &&
    fromTokenMeta?.canSwap !== false &&
    toTokenMeta?.canSwap !== false &&
    normalizedFromToken !== normalizedToToken &&
    Number(amount) > 0 &&
    selectedQuote &&
    selectedQuote?.status === 'available' &&
    selectedQuote?.can_execute !== false &&
    !quoting &&
    !executing
  );
  const isLimitTargetReady = limitTargetMode === 'price'
    ? Number.isFinite(limitTargetPriceNumber) && limitTargetPriceNumber > 0
    : Number.isFinite(limitTargetToAmountNumber) && limitTargetToAmountNumber > 0;
  const isReadyToSwap = swapMode === 'limit'
    ? Boolean(baseSwapReady && isLimitTargetReady)
    : baseSwapReady;

  let submitLabel = swapMode === 'limit' ? '创建限价单' : '\u9884\u89c8\u5151\u6362';
  if (!selectedWalletId) submitLabel = walletLoading ? '\u52a0\u8f7d\u94b1\u5305\u4e2d...' : '\u8bf7\u5148\u9009\u62e9\u94b1\u5305';
  else if (!normalizedFromToken) submitLabel = '\u9009\u62e9\u5356\u51fa\u4ee3\u5e01';
  else if (!amount || Number(amount) <= 0) submitLabel = '\u8f93\u5165\u5356\u51fa\u6570\u91cf';
  else if (!normalizedToToken) submitLabel = '\u9009\u62e9\u4e70\u5165\u4ee3\u5e01';
  else if (fromTokenMeta?.canSwap === false || toTokenMeta?.canSwap === false) submitLabel = '\u539f\u751f\u5e01\u6682\u4e0d\u652f\u6301\u76f4\u63a5\u5151\u6362';
  else if (normalizedFromToken === normalizedToToken) submitLabel = '\u4e0d\u80fd\u5151\u6362\u540c\u4e00\u4ee3\u5e01';
  else if (quoting) submitLabel = '\u83b7\u53d6\u6700\u4f18\u62a5\u4ef7\u4e2d...';
  else if (!quoteInfo) submitLabel = '\u7b49\u5f85\u62a5\u4ef7';
  else if (!availableProviderCount) submitLabel = '\u6682\u65e0\u53ef\u7528\u62a5\u4ef7';
  else if (!selectedQuote || selectedQuote?.status !== 'available') submitLabel = '\u9009\u62e9\u53ef\u7528\u62a5\u4ef7';
  else if (swapMode === 'limit' && !isLimitTargetReady) submitLabel = limitTargetMode === 'price' ? '输入目标价格' : '输入目标到账';

  return (
    <PanelShell
      title={'\u4e00\u952e\u5151\u6362'}
      subtitle={'\u5feb\u901f\u5151\u6362\u4efb\u610f\u652f\u6301\u7684\u4ee3\u5e01'}
      icon={RefreshCw}
    >
      <div className="swap-panel">
        <div className="swap-panel-shell">

          <div className="swap-mode-tabs" role="tablist" aria-label="兑换模式">
            <button type="button" className={swapMode === 'market' ? 'active' : ''} onClick={() => setSwapMode('market')}>市价兑换</button>
            <button type="button" className={swapMode === 'limit' ? 'active' : ''} onClick={() => setSwapMode('limit')}>限价单</button>
          </div>

          {/* ─── Top Strip: Chain pills + Slippage ─── */}
          <div className="swap-top-strip">
            <div className="swap-chain-pills">
              {chainOptions.map((option) => (
                <button
                  key={option.key}
                  type="button"
                  className={`swap-chain-pill${option.key === chain ? ' active' : ''}`}
                  onClick={() => handleChainSelect(option.key)}
                  title={option.label}
                >
                  <img src={option.icon} alt={option.iconAlt || option.label} />
                  <span>{option.label}</span>
                </button>
              ))}
            </div>
            <div className="swap-slip-strip">
              {SLIPPAGE_PRESETS.map((item) => (
                <button
                  key={item}
                  type="button"
                  className={`swap-slip-pill${Number(slippage) === Number(item) ? ' active' : ''}`}
                  onClick={() => { setSlippage(item); setSlippageDirty(true); }}
                >
                  {item}%
                </button>
              ))}
              <div className="swap-slip-custom">
                <input
                  type="number"
                  min="0"
                  step="0.1"
                  value={slippage}
                  onChange={(event) => { setSlippage(event.target.value); setSlippageDirty(true); }}
                  placeholder="1.0"
                />
                <span>%</span>
              </div>
            </div>
          </div>

          {swapMode === 'limit' ? (
            <div className="swap-limit-card">
              <div className="swap-limit-head">
                <div>
                  <strong>触发条件</strong>
                  <span>{selectedQuote?.status === 'available' ? `当前预估 ${selectedQuoteAmount} ${toTokenMeta?.symbol || ''}`.trim() : '先选择代币并获取当前报价'}</span>
                </div>
                <div className="swap-limit-toggle">
                  <button type="button" className={limitTargetMode === 'to_amount' ? 'active' : ''} onClick={() => setLimitTargetMode('to_amount')}>到账</button>
                  <button type="button" className={limitTargetMode === 'price' ? 'active' : ''} onClick={() => setLimitTargetMode('price')}>价格</button>
                </div>
              </div>
              <div className="swap-limit-input-row">
                <input
                  type="number"
                  min="0"
                  step="any"
                  value={limitTargetMode === 'price' ? limitTargetPrice : limitTargetToAmount}
                  onChange={(event) => {
                    clearExecutionFeedback();
                    if (limitTargetMode === 'price') setLimitTargetPrice(event.target.value);
                    else setLimitTargetToAmount(event.target.value);
                  }}
                  placeholder={limitTargetMode === 'price' ? '目标价格' : '目标到账数量'}
                />
                <span>{limitTargetMode === 'price' ? `${toTokenMeta?.symbol || 'To'} / ${fromTokenMeta?.symbol || 'From'}` : (toTokenMeta?.symbol || 'To')}</span>
              </div>
            </div>
          ) : null}

          {/* ─── Wallet Selector ─── */}
          <div className="swap-wallet-bar" ref={walletSelectRef}>
            <button
              type="button"
              className="swap-wallet-trigger"
              onClick={() => setWalletDropdownOpen((c) => !c)}
              disabled={walletLoading || !wallets.length}
            >
              <Wallet size={14} />
              <span className="swap-wallet-trigger-text">
                {selectedWallet
                  ? `${selectedWallet.name || '\u94b1\u5305'} \u00b7 ${shortAddress(selectedWallet.address, 6, 4)}`
                  : walletLoading ? '\u52a0\u8f7d\u94b1\u5305\u4e2d...' : '\u6682\u65e0\u53ef\u7528\u94b1\u5305'}
              </span>
              <ChevronDown size={14} style={{ marginLeft: 'auto', opacity: 0.5, transition: 'transform .2s' }} />
            </button>
            {walletDropdownOpen && wallets.length > 0 ? (
              <div className="swap-wallet-dropdown">
                {wallets.map((wallet) => (
                  <button
                    key={wallet.id}
                    type="button"
                    className={`swap-wallet-option${String(wallet.id) === String(selectedWalletId) ? ' active' : ''}`}
                    onClick={() => { clearExecutionFeedback(); setSelectedWalletId(String(wallet.id)); setWalletDropdownOpen(false); }}
                  >
                    <div className="swap-wallet-option-info">
                      <strong>{wallet.name || '\u94b1\u5305'}</strong>
                      <span>{shortAddress(wallet.address, 8, 6)}</span>
                    </div>
                    <div className="swap-wallet-option-bal">
                      <span>{chainConfig.nativeSymbol}</span>
                      <strong>{formatNativeBalance(wallet.native_balance)}</strong>
                    </div>
                    {String(wallet.id) === String(selectedWalletId) ? <Check size={14} className="swap-wallet-check" /> : null}
                  </button>
                ))}
              </div>
            ) : null}
          </div>

          {/* ─── Main Swap Cards ─── */}
          <div className="swap-cards-container">
            {/* From Card */}
            <div className="swap-card swap-card--from">
              <div className="swap-card-head">
                <span className="swap-card-label">From</span>
                <span className="swap-card-addr">{selectedWalletAddressLabel}</span>
              </div>
              <div className="swap-card-body">
                <input
                  type="text"
                  inputMode="decimal"
                  className="swap-amount-input"
                  value={amount}
                  onChange={(event) => { clearExecutionFeedback(); setAmount(event.target.value); }}
                  placeholder="0"
                />
                <TokenButton token={fromTokenMeta} placeholder={'\u9009\u62e9\u4ee3\u5e01'} onClick={() => { setPickerSide('from'); setPickerOpen(true); }} />
              </div>
              <div className="swap-card-foot">
                <span className="swap-card-name">{fromTokenMeta ? fromTokenMeta.name : '\u672a\u9009\u62e9\u5356\u51fa\u4ee3\u5e01'}</span>
                <div className="swap-card-balance-area">
                  {fromTokenBalance && Number(fromTokenBalance) > 0 ? (
                    <>
                      <span className="swap-balance-text">{formatTokenAmount(fromTokenBalance)}</span>
                      <button type="button" className="swap-max-btn" onClick={handleMaxAmount}>MAX</button>
                    </>
                  ) : selectedWallet ? (
                    <span className="swap-native-text">{chainConfig.nativeSymbol} {formatNativeBalance(selectedWallet.native_balance)}</span>
                  ) : <span className="swap-native-text">--</span>}
                </div>
              </div>
            </div>

            {/* Switch */}
            <button type="button" className="swap-switch-button" onClick={handleReverse} aria-label={'\u5207\u6362\u5151\u6362\u65b9\u5411'}>
              <ArrowDown size={16} strokeWidth={2.5} />
            </button>

            {/* To Card */}
            <div className="swap-card swap-card--to">
              <div className="swap-card-head">
                <span className="swap-card-label">To</span>
                <span className="swap-card-addr">{selectedWalletAddressLabel}</span>
              </div>
              <div className="swap-card-body">
                <div className={`swap-quote-output${quoting ? ' loading' : ''}${refreshingQuote ? ' refreshing' : ''}`}>
                  {quoting && !quoteInfo ? '...' : selectedQuoteAmount}
                </div>
                <TokenButton token={toTokenMeta} placeholder={'\u9009\u62e9\u4ee3\u5e01'} onClick={() => { setPickerSide('to'); setPickerOpen(true); }} />
              </div>
              <div className="swap-card-foot">
                <span className="swap-card-name">{toTokenMeta ? toTokenMeta.name : '\u672a\u9009\u62e9\u76ee\u6807\u4ee3\u5e01'}</span>
                <span className="swap-min-received">{`\u6700\u5c11\u5230\u8d26 ${minReceived}`}</span>
              </div>
            </div>
          </div>

          {/* ─── Quote / Provider Section ─── */}
          <div className={`swap-quote-section${refreshingQuote ? ' refreshing' : ''}`}>
            {quoteInfo ? (
              <>
                <div className="swap-quote-toolbar">
                  <div className={`swap-refresh-badge${refreshingQuote ? ' active' : ''}`}>
                    <RefreshCw size={11} />
                    <span>{refreshingQuote ? '\u62a5\u4ef7\u5237\u65b0\u4e2d' : quoteStampText ? `\u5df2\u66f4\u65b0 ${quoteStampText}` : '\u5b9e\u65f6\u62a5\u4ef7'}</span>
                  </div>
                </div>

                <div className="swap-providers">
                  {providerQuotes.map((quote, index) => {
                    const key = quoteSelectionKey(quote) || `provider-${index}`;
                    const active = quoteSelectionKey(selectedQuote) === quoteSelectionKey(quote);
                    const gasCostText = formatQuoteGasCostSummary(quote, chainConfig.nativeSymbol);
                    const quoteName = quote?.vendor_name
                      ? `${quote?.provider_label || quote?.provider || '--'} · ${quote.vendor_name}`
                      : (quote?.provider_label || quote?.provider || '--');
                    return (
                      <button
                        key={key}
                        type="button"
                        className={`swap-prov-card${active ? ' active' : ''}${quote?.status !== 'available' ? ' unavailable' : ''}`}
                        onClick={() => setSelectedProvider(quoteSelectionKey(quote))}
                      >
                        <div className="swap-prov-row-top">
                          <div className="swap-prov-identity">
                            <ProviderIcon provider={quote?.provider} size={18} />
                            <strong className="swap-prov-name">{quoteName}</strong>
                            <span className="swap-prov-tag">{quote?.recommended ? '\u63a8\u8350' : (quote?.status === 'available' ? '\u53ef\u7528' : '\u4e0d\u53ef\u7528')}</span>
                          </div>
                          {quote?.status === 'available' ? (
                            <span className="swap-prov-chip">{`${formatTokenAmount(quote?.net_to_amount_float)} ${toTokenMeta?.symbol || ''}`.trim()}</span>
                          ) : (
                            <span className="swap-prov-chip muted">{'\u65e0\u62a5\u4ef7'}</span>
                          )}
                        </div>
                        {quote?.status === 'available' ? (
                          <div className="swap-prov-row-bottom">
                            {shouldShowSwapRoute(quote?.provider) && (quote?.route?.length || quote?.route_summary) ? (
                              <RouteDexIcons routeSummary={quote?.route_summary} route={quote?.route} />
                            ) : <span />}
                            <span className="swap-prov-gas-text">{gasCostText}</span>
                          </div>
                        ) : (
                          <div className="swap-prov-row-bottom">
                            <span className="swap-prov-error-inline">{quote?.error || '\u8be5 provider \u6682\u65f6\u4e0d\u53ef\u7528'}</span>
                          </div>
                        )}
                      </button>
                    );
                  })}
                </div>

                {selectedQuote ? (
                  selectedQuote?.status === 'available' ? (
                    <div className="swap-detail-list">
                      <DetailRow label={'\u5f53\u524d Provider'} value={selectedQuote?.provider_label || '--'} />
                      {selectedQuote?.vendor_name ? <DetailRow label={'Binance Vendor'} value={selectedQuote.vendor_name} /> : null}
                      <DetailRow label={'\u9884\u4f30\u5230\u8d26'} value={`${selectedQuoteAmount} ${toTokenMeta?.symbol || ''}`.trim()} emphasis />
                      <DetailRow label={'\u6700\u5c11\u5230\u8d26'} value={`${minReceived} ${toTokenMeta?.symbol || ''}`.trim()} />
                      <DetailRow label={'\u624b\u7eed\u8d39'} value={selectedQuoteFeeDisplay} />
                      <DetailRow label={'\u9884\u4f30 Gas'} value={quoteGasUnits} />
                      <DetailRow label={'Gas \u8d39\u7528'} value={quoteGasCostText} />
                      {showSelectedQuoteRoute && (selectedQuote?.route?.length || selectedQuote?.route_summary) ? (
                        <DetailRow label={'\u8def\u5f84\u6458\u8981'} value={<RouteDexIcons routeSummary={selectedQuoteRouteText} route={selectedQuote?.route} />} />
                      ) : null}
                      <DetailRow label={'\u6ed1\u70b9\u8bbe\u7f6e'} value={`${slippage || '1.0'}%`} />
                    </div>
                  ) : (
                    <div className="swap-empty-hint">
                      <strong>{`${selectedQuote?.provider_label || '\u5f53\u524d'} \u6682\u65f6\u4e0d\u53ef\u7528`}</strong>
                      <span>{selectedQuote?.error || '\u8bf7\u5207\u6362\u5176\u4ed6 provider \u6216\u7a0d\u540e\u91cd\u8bd5\u3002'}</span>
                    </div>
                  )
                ) : null}
              </>
            ) : (
              <div className="swap-empty-hint">
                <strong>{'\u8f93\u5165\u6570\u91cf\u540e\u81ea\u52a8\u62a5\u4ef7'}</strong>
                <span>{'\u9009\u62e9\u4ee3\u5e01\uff0c\u6216\u76f4\u63a5\u7c98\u8d34 ERC-20 \u5408\u7ea6\u5730\u5740\u3002'}</span>
              </div>
            )}
          </div>

          {/* ─── Messages ─── */}
          {quoteError ? <div className="swap-msg swap-msg--error"><strong>{'\u62a5\u4ef7\u5931\u8d25:'}</strong> {quoteError}</div> : null}
          {execError ? <div className="swap-msg swap-msg--error"><strong>{'\u5151\u6362\u5931\u8d25:'}</strong> {execError}</div> : null}
          {execSuccess ? (
            <div className="swap-msg swap-msg--success">
              <strong>{execSuccess}</strong>
              {execResult?.provider_label ? <span>{`\u6267\u884c Provider ${execResult.provider_label}`}</span> : null}
              {execResult?.to_amount_float ? <span>{`\u5b9e\u9645\u5230\u8d26 ${execResult.to_amount_float} ${toTokenMeta?.symbol || ''}`.trim()}</span> : null}
              {execResult?.completed_at ? <span>{`\u5b8c\u6210\u65f6\u95f4 ${execResult.completed_at}`}</span> : null}
              {execResult?.tx_hash ? (
                <span className="swap-msg-tx">
                  <span>{shortAddress(execResult.tx_hash, 10, 8)}</span>
                  {execResult?.tx_url ? <a href={execResult.tx_url} target="_blank" rel="noreferrer">{'\u67e5\u770b Tx'}</a> : null}
                </span>
              ) : null}
            </div>
          ) : null}

          {/* ─── Submit ─── */}
          <button type="button" className="swap-submit-button" disabled={!isReadyToSwap} onClick={() => (swapMode === 'limit' ? handleCreateLimitOrder() : setShowConfirm(true))}>
            {executing ? '\u6267\u884c\u4e2d...' : submitLabel}
          </button>

          {swapMode === 'limit' ? (
            <div className="swap-history-card">
              <div className="swap-history-head">
                <div>
                  <strong>限价单</strong>
                  <span>{selectedWallet ? shortAddress(selectedWallet.address, 8, 6) : '当前钱包'}</span>
                </div>
                <button type="button" className="swap-history-refresh" onClick={() => loadLimitOrders()} disabled={loadingLimitOrders || !selectedWalletId}>
                  {loadingLimitOrders ? '刷新中...' : '刷新'}
                </button>
              </div>
              {limitOrdersError ? <div className="swap-history-empty">{limitOrdersError}</div> : null}
              {!limitOrdersError && loadingLimitOrders && limitOrders.length === 0 ? <div className="swap-history-empty">正在加载限价单...</div> : null}
              {!limitOrdersError && !loadingLimitOrders && limitOrders.length === 0 ? <div className="swap-history-empty">当前钱包暂无限价单</div> : null}
              {limitOrders.map((order) => {
                const fromOT = buildHistoryTokenMeta(order.from_token, tokenMetaMap, chain);
                const toOT = buildHistoryTokenMeta(order.to_token, tokenMetaMap, chain);
                const open = order.status === 'open';
                return (
                  <div key={order.id} className="swap-limit-order-row">
                    <div className="swap-limit-order-main">
                      <strong>{`${order.from_amount_float || '--'} ${fromOT?.symbol || order?.from_token?.symbol || ''} -> ${order.target_to_amount_float || '--'} ${toOT?.symbol || order?.to_token?.symbol || ''}`}</strong>
                      <span>{`Provider ${order.provider_label || '--'} · ${order.status || '--'}`}</span>
                      {order.last_quote_to_amount_float ? <small>{`最近报价 ${order.last_quote_to_amount_float} ${toOT?.symbol || ''} · ${order.last_checked_at || '--'}`}</small> : null}
                      {order.tx_url ? <a href={order.tx_url} target="_blank" rel="noreferrer">{shortAddress(order.tx_hash, 8, 6)}</a> : null}
                      {order.last_error ? <small className="error">{order.last_error}</small> : null}
                    </div>
                    <button
                      type="button"
                      className="swap-limit-cancel"
                      disabled={!open || limitOrderBusyId === String(order.id)}
                      onClick={() => handleCancelLimitOrder(order.id)}
                    >
                      {limitOrderBusyId === String(order.id) ? '处理中' : open ? '取消' : '已结束'}
                    </button>
                  </div>
                );
              })}
            </div>
          ) : null}

          {/* ─── History ─── */}
          <div className="swap-history-card">
            <div className="swap-history-head">
              <div>
                <strong>{'\u6700\u8fd1\u5151\u6362'}</strong>
                <span>{selectedWallet ? shortAddress(selectedWallet.address, 8, 6) : '\u5f53\u524d\u94b1\u5305'}</span>
              </div>
              <button type="button" className="swap-history-refresh" onClick={() => loadSwapHistory()} disabled={loadingSwapHistory || !selectedWalletId}>
                {loadingSwapHistory ? '\u5237\u65b0\u4e2d...' : '\u5237\u65b0'}
              </button>
            </div>
            {swapHistoryError ? <div className="swap-history-empty">{swapHistoryError}</div> : null}
            {!swapHistoryError && loadingSwapHistory && swapHistory.length === 0 ? <div className="swap-history-empty">{'\u6b63\u5728\u52a0\u8f7d\u5151\u6362\u5386\u53f2...'}</div> : null}
            {!swapHistoryError && !loadingSwapHistory && swapHistory.length === 0 ? <div className="swap-history-empty">{'\u5f53\u524d\u94b1\u5305\u6682\u65e0\u5151\u6362\u8bb0\u5f55'}</div> : null}
            {swapHistory.map((item) => {
              const fromHT = buildHistoryTokenMeta(item.from_token, tokenMetaMap, chain);
              const toHT = buildHistoryTokenMeta(item.to_token, tokenMetaMap, chain);
              return (
                <div key={item.id || item.tx_hash} className="swap-history-row">
                  <div className="swap-history-route">
                    <div className="swap-history-token">
                      <TokenGlyph token={fromHT || makeCustomToken(item?.from_token?.address)} size="sm" />
                      <div>
                        <strong>{`${item.amount_in_float || '--'} ${fromHT?.symbol || item?.from_token?.symbol || ''}`.trim()}</strong>
                        <span>{fromHT?.name || item?.from_token?.name || item?.from_token?.address}</span>
                      </div>
                    </div>
                    <ArrowDown size={14} />
                    <div className="swap-history-token">
                      <TokenGlyph token={toHT || makeCustomToken(item?.to_token?.address)} size="sm" />
                      <div>
                        <strong>{`${item.amount_out_float || '--'} ${toHT?.symbol || item?.to_token?.symbol || ''}`.trim()}</strong>
                        <span>{toHT?.name || item?.to_token?.name || item?.to_token?.address}</span>
                      </div>
                    </div>
                  </div>
                  <div className="swap-history-meta">
                    <span>{item.created_at || '--'}</span>
                    {item.provider_label ? <span className="swap-history-provider">{item.provider_label}</span> : null}
                    <span className="swap-history-status">{item.status || 'confirmed'}</span>
                    {item.tx_url ? <a href={item.tx_url} target="_blank" rel="noreferrer" className="swap-history-link">{shortAddress(item.tx_hash, 8, 6)}</a> : <span>{shortAddress(item.tx_hash, 8, 6)}</span>}
                  </div>
                </div>
              );
            })}
          </div>
        </div>

        {/* ─── Token Picker Backdrop ─── */}
        {pickerOpen ? <div className="swap-overlay-backdrop" onClick={() => setPickerOpen(false)} /> : null}

        {/* ─── Token Picker Modal ─── */}
        {pickerOpen ? (
          <div className="swap-modal-overlay" style={{ background: 'transparent' }}>
            <div className="swap-token-modal" onClick={(e) => e.stopPropagation()}>
              <div className="swap-modal-header">
                <div><h3>{pickerSide === 'from' ? '\u9009\u62e9\u5356\u51fa\u4ee3\u5e01' : '\u9009\u62e9\u4e70\u5165\u4ee3\u5e01'}</h3></div>
                <button type="button" className="swap-modal-close" onClick={() => setPickerOpen(false)}><X size={18} /></button>
              </div>
              <div className="swap-token-search">
                <Search size={16} />
                <input type="text" value={tokenQuery} onChange={(e) => setTokenQuery(e.target.value)} placeholder={'\u641c\u7d22\u4ee3\u5e01\u540d\u79f0\u3001\u7b26\u53f7\uff0c\u6216\u7c98\u8d34\u5408\u7ea6\u5730\u5740'} autoFocus />
                {loadingWalletTokens && walletTokens.length > 0 ? <div style={{ fontSize: '11px', color: 'var(--text-muted)', whiteSpace: 'nowrap' }}>{'\u5237\u65b0\u4e2d...'}</div> : null}
              </div>
              <div className="swap-quick-picks">
                {presetTokens.slice(0, 5).map((token) => (
                  <button key={token.address} type="button" className="swap-quick-pick" onClick={() => handleSelectToken(token)}>
                    <TokenGlyph token={token} size="sm" />
                    <span>{token.symbol}</span>
                  </button>
                ))}
              </div>
              <div className="swap-token-list">
                {loadingWalletTokens && walletTokens.length === 0 ? <div className="swap-token-list-status"><div>{'\u6b63\u5728\u52a0\u8f7d\u94b1\u5305\u4f59\u989d...'}</div><small>{'\u9996\u6b21\u52a0\u8f7d\u53ef\u80fd\u9700\u8981\u51e0\u79d2\u949f'}</small></div> : null}
                {!loadingWalletTokens && walletTokensError ? (
                  <div className="swap-token-list-error">
                    <strong>{'\u94b1\u5305\u4f59\u989d\u52a0\u8f7d\u5931\u8d25'}</strong>
                    <span>{walletTokensError}</span>
                    <button type="button" onClick={() => loadWalletTokens()} className="swap-token-list-retry">{'\u91cd\u8bd5\u67e5\u8be2'}</button>
                  </div>
                ) : null}
                {!loadingWalletTokens && !walletTokensError && hasLoadedWalletTokens && walletTokens.length === 0 ? (
                  <div className="swap-token-list-note">
                    <strong>{'\u94b1\u5305\u4f59\u989d'}</strong>
                    <span>{`\u672a\u8fd4\u56de\u5f53\u524d\u94b1\u5305\u4e2d\u4ef7\u503c >= $${MIN_WALLET_TOKEN_VALUE_USD.toFixed(1)} \u7684\u4ee3\u5e01\u4f59\u989d\u3002`}</span>
                  </div>
                ) : null}
                {pickerTokens.customCandidate ? (
                  <div className="swap-token-section">
                    <div className="swap-token-section-title">{'\u81ea\u5b9a\u4e49\u5730\u5740'}</div>
                    <button type="button" className="swap-token-row" onClick={() => handleSelectToken(pickerTokens.customCandidate)}>
                      <TokenGlyph token={pickerTokens.customCandidate} />
                      <div className="swap-token-row-copy"><strong>{pickerTokens.customCandidate.symbol}</strong><span>{pickerTokens.customCandidate.address}</span></div>
                      <span className="swap-token-tag">{'\u7c98\u8d34\u4f7f\u7528'}</span>
                    </button>
                  </div>
                ) : null}
                {pickerTokens.withBalance && pickerTokens.withBalance.length > 0 ? (
                  <div className="swap-token-section">
                    <div className="swap-token-section-title">{'\u94b1\u5305\u4f59\u989d'}</div>
                    {pickerTokens.withBalance.map((token) => (
                      <button key={token.address} type="button" className={`swap-token-row${token.canSwap === false ? ' disabled' : ''}`} onClick={() => handleSelectToken(token)} disabled={token.canSwap === false}>
                        <TokenGlyph token={token} />
                        <div className="swap-token-row-copy">
                          <strong>{token.symbol}</strong>
                          <span>{token.canSwap === false ? (token.disabledReason || token.name) : token.name}{token.valueUSDT > 0 ? <small className="swap-token-usd">{` \u2248 $${token.valueUSDT.toFixed(2)}`}</small> : null}</span>
                        </div>
                        <div className="swap-token-row-right">
                          <span className="swap-token-bal-tag">{formatTokenAmount(token.balance)}</span>
                          {token.canSwap === false ? <span className="swap-token-tag muted">{'\u539f\u751f\u5e01'}</span> : null}
                        </div>
                      </button>
                    ))}
                  </div>
                ) : null}
                {pickerTokens.recent.length ? (
                  <div className="swap-token-section">
                    <div className="swap-token-section-title">{'\u6700\u8fd1\u4f7f\u7528'}</div>
                    {pickerTokens.recent.map((token) => (
                      <button key={token.address} type="button" className={`swap-token-row${token.canSwap === false ? ' disabled' : ''}`} onClick={() => handleSelectToken(token)} disabled={token.canSwap === false}>
                        <TokenGlyph token={token} />
                        <div className="swap-token-row-copy"><strong>{token.symbol}</strong><span>{token.custom ? token.address : token.name}</span></div>
                        <div className="swap-token-row-right">
                          {token.balance && Number(token.balance) > 0 ? <span className="swap-token-bal-inline">{formatTokenAmount(token.balance)}</span> : null}
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
                      <button key={token.address} type="button" className={`swap-token-row${token.canSwap === false ? ' disabled' : ''}`} onClick={() => handleSelectToken(token)} disabled={token.canSwap === false}>
                        <TokenGlyph token={token} />
                        <div className="swap-token-row-copy"><strong>{token.symbol}</strong><span>{token.name}</span></div>
                        <div className="swap-token-row-right">
                          {token.balance && Number(token.balance) > 0 ? <span className="swap-token-bal-inline">{formatTokenAmount(token.balance)}</span> : null}
                          <span className="swap-token-tag subtle">{shortAddress(token.address, 5, 4)}</span>
                        </div>
                      </button>
                    ))}
                  </div>
                ) : null}
                {!loadingWalletTokens && !pickerTokens.customCandidate && !pickerTokens.withBalance?.length && !pickerTokens.recent.length && !pickerTokens.preset.length ? (
                  <div className="swap-token-empty"><strong>{'\u6ca1\u6709\u5339\u914d\u7ed3\u679c'}</strong><span>{'\u4e5f\u53ef\u4ee5\u76f4\u63a5\u7c98\u8d34 ERC-20 \u5408\u7ea6\u5730\u5740\u3002'}</span></div>
                ) : null}
              </div>
            </div>
          </div>
        ) : null}

        {/* ─── Confirm Modal ─── */}
        {showConfirm && selectedQuote ? (
          <div className="swap-modal-overlay" onClick={() => (!executing ? setShowConfirm(false) : null)}>
            <div className="swap-confirm-modal" onClick={(e) => e.stopPropagation()}>
              <div className="swap-modal-header">
                <div><h3>{'\u786e\u8ba4\u5151\u6362'}</h3></div>
                <button type="button" className="swap-modal-close" onClick={() => setShowConfirm(false)} disabled={executing}><X size={18} /></button>
              </div>
              <div className="swap-confirm-route">
                <div className="swap-confirm-flow">
                  <div className="swap-confirm-token">
                    <TokenGlyph token={fromTokenMeta || makeCustomToken(normalizedFromToken)} />
                    <div><span>{'\u652f\u4ed8'}</span><strong>{amount} {fromTokenMeta?.symbol || shortAddress(normalizedFromToken, 4, 4)}</strong></div>
                  </div>
                  <div className="swap-confirm-arrow"><ArrowDown size={16} /></div>
                  <div className="swap-confirm-token">
                    <TokenGlyph token={toTokenMeta || makeCustomToken(normalizedToToken)} />
                    <div><span>{'\u83b7\u5f97'}</span><strong>{selectedQuoteAmount} {toTokenMeta?.symbol || shortAddress(normalizedToToken, 4, 4)}</strong></div>
                  </div>
                </div>
              </div>
              <div className="swap-confirm-details">
                <DetailRow label={'Provider'} value={selectedQuote?.provider_label || '--'} />
                {selectedQuote?.vendor_name ? <DetailRow label={'Binance Vendor'} value={selectedQuote.vendor_name} /> : null}
                <DetailRow label={'\u6700\u5c11\u5230\u8d26'} value={`${minReceived} ${toTokenMeta?.symbol || ''}`.trim()} />
                <DetailRow label={'\u624b\u7eed\u8d39'} value={selectedQuoteFeeDisplay} />
                <DetailRow label={'\u6ed1\u70b9\u5bb9\u5fcd'} value={`${slippage || '1.0'}%`} />
                <DetailRow label={'\u9884\u4f30 Gas'} value={quoteGasUnits} />
                <DetailRow label={'Gas \u8d39\u7528'} value={quoteGasCostText} />
                {showSelectedQuoteRoute && (selectedQuote?.route?.length || selectedQuote?.route_summary) ? (
                  <DetailRow label={'\u8def\u5f84\u6458\u8981'} value={<RouteDexIcons routeSummary={selectedQuoteRouteText} route={selectedQuote?.route} />} />
                ) : null}
              </div>
              <div className="swap-confirm-actions">
                <button type="button" className="swap-confirm-cancel" onClick={() => setShowConfirm(false)} disabled={executing}>{'\u53d6\u6d88'}</button>
                <button type="button" className="swap-submit-button compact" onClick={handleSwap} disabled={executing || quoting || selectedQuote?.status !== 'available'}>
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
