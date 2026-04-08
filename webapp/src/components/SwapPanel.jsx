import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { fetchWallets, walletSwapSingleExecute, walletSwapSingleQuote, walletSwapPreview } from '../api';
import PanelShell from './PanelShell';
import { normalizeHexAddress, shortAddress } from '../utils';
import { ArrowDown, ChevronDown, RefreshCw, Search, Settings, Wallet, X, TrendingUp } from 'lucide-react';

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

const TABS = [
  { key: 'swap', label: '兑换', enabled: true },
  { key: 'limit', label: '限额', enabled: false },
  { key: 'buy', label: '购买', enabled: false },
  { key: 'sell', label: '出售', enabled: false },
];

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
      name: String(token?.name || '自定义代币').trim() || '自定义代币',
      color: String(token?.color || '#7c8aa6').trim() || '#7c8aa6',
      custom: Boolean(token?.custom),
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
    name: '自定义合约地址',
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

function resolveTokenMeta(chain, address, recentTokens) {
  const normalized = normalizeHexAddress(address);
  if (!normalized) return null;
  const pool = dedupeTokens([...(recentTokens?.[chain] || []), ...getPresetTokens(chain)]);
  return pool.find((item) => item.address === normalized) || buildCustomToken(normalized);
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

  const quoteTimeout = useRef(null);
  const quoteAbortRef = useRef(null);
  const quoteSeqRef = useRef(0);

  const normalizedFromToken = normalizeHexAddress(fromToken);
  const normalizedToToken = normalizeHexAddress(toToken);
  const fromTokenMeta = useMemo(
    () => resolveTokenMeta(chain, fromToken, recentTokens),
    [chain, fromToken, recentTokens]
  );
  const toTokenMeta = useMemo(
    () => resolveTokenMeta(chain, toToken, recentTokens),
    [chain, toToken, recentTokens]
  );
  const selectedWallet = useMemo(
    () => wallets.find((item) => String(item.id) === String(selectedWalletId)) || null,
    [wallets, selectedWalletId]
  );
  const presetTokens = useMemo(() => getPresetTokens(chain), [chain]);
  const recentChainTokens = useMemo(
    () => dedupeTokens(recentTokens?.[chain] || []),
    [chain, recentTokens]
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

    // 合并钱包余额信息到代币列表
    const enrichToken = (token) => {
      const walletToken = walletTokens.find((wt) => wt.address === token.address);
      return {
        ...token,
        balance: walletToken?.balance || '0',
        valueUSDT: walletToken?.valueUSDT || 0,
      };
    };

    // 有余额的代币
    const withBalance = walletTokens
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
        };
      })
      .sort((a, b) => (b.valueUSDT || 0) - (a.valueUSDT || 0));

    // 最近使用的代币(排除已在余额列表中的)
    const recent = recentChainTokens
      .filter((token) => !withBalance.some((item) => item.address === token.address))
      .filter((token) => matchesToken(token, keyword))
      .map(enrichToken);

    // 常用代币(排除已在余额和最近列表中的)
    const preset = presetTokens
      .filter((token) => !withBalance.some((item) => item.address === token.address))
      .filter((token) => !recent.some((item) => item.address === token.address))
      .filter((token) => matchesToken(token, keyword))
      .map(enrichToken);

    const customCandidate = buildCustomToken(tokenQuery);
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
  }, [tokenQuery, recentChainTokens, presetTokens, walletTokens]);

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
  }, [chainConfig.stable.address]);

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
    setLoadingWalletTokens(true);
    try {
      const resp = await walletSwapPreview({ apiBaseUrl, initData, chain, minValueUsd: 0.01 });
      const tokens = (resp?.tokens || []).map((t) => ({
        address: normalizeHexAddress(t.address),
        symbol: t.symbol,
        balance: t.balance,
        valueUSDT: t.value_usdt,
      }));
      setWalletTokens(tokens);
    } catch (error) {
      console.error('loadWalletTokens failed', error);
      setWalletTokens([]);
    } finally {
      setLoadingWalletTokens(false);
    }
  }, [apiBaseUrl, initData, chain, selectedWalletId]);

  useEffect(() => {
    if (!hasInitData) return;
    loadWallets();
    setExecError('');
    setExecSuccess('');
  }, [hasInitData, loadWallets]);

  useEffect(() => {
    if (!hasInitData || !selectedWalletId) return;
    loadWalletTokens();
  }, [hasInitData, selectedWalletId, loadWalletTokens]);

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
      setQuoteError('支付和接收代币不能相同');
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
        setQuoteError('支付和接收代币不能相同');
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
    if (!pickerOpen && !showConfirm) return undefined;
    const onKeyDown = (event) => {
      if (event.key !== 'Escape') return;
      if (pickerOpen) {
        setPickerOpen(false);
        return;
      }
      if (!executing) setShowConfirm(false);
    };
    window.addEventListener('keydown', onKeyDown);
    return () => window.removeEventListener('keydown', onKeyDown);
  }, [executing, pickerOpen, showConfirm]);

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
      setExecSuccess(resp?.tx_hash || '交易已提交');
      setShowConfirm(false);
      setAmount('');
      setQuoteInfo(null);
      // 刷新钱包余额
      loadWalletTokens();
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

  let submitLabel = '预览兑换';
  if (!selectedWalletId) submitLabel = walletLoading ? '加载钱包中...' : '请先选择钱包';
  else if (!normalizedFromToken) submitLabel = '选择卖出代币';
  else if (!amount || Number(amount) <= 0) submitLabel = '输入卖出数量';
  else if (!normalizedToToken) submitLabel = '选择买入代币';
  else if (normalizedFromToken === normalizedToToken) submitLabel = '不能兑换同一代币';
  else if (quoting) submitLabel = '获取最优报价中...';
  else if (!quoteInfo) submitLabel = '等待报价';

  return (
    <PanelShell
      title="一键兑换"
      subtitle="Uniswap 风格重构 · 单币闪兑由 OKX DEX 聚合路由"
      icon={RefreshCw}
    >
      <div className="swap-panel">
        <div className="swap-panel-shell">
          <div className="swap-panel-topbar">
            <div className="swap-tabs" role="tablist" aria-label="swap modes">
              {TABS.map((tab) => (
                <button
                  key={tab.key}
                  type="button"
                  className={`swap-tab${tab.enabled ? ' active' : ''}`}
                  disabled={!tab.enabled}
                >
                  {tab.label}
                </button>
              ))}
            </div>
            <button
              type="button"
              className={`swap-settings-trigger${showSettings ? ' active' : ''}`}
              onClick={() => setShowSettings((current) => !current)}
              aria-label="配置兑换参数"
            >
              <Settings size={17} />
            </button>
          </div>

          {showSettings ? (
            <div className="swap-settings-card">
              <div className="swap-settings-grid">
                <label className="swap-settings-field">
                  <span>执行钱包</span>
                  <div className="swap-select-wrap">
                    <Wallet size={14} />
                    <select
                      value={selectedWalletId}
                      onChange={(event) => setSelectedWalletId(event.target.value)}
                      disabled={walletLoading || !wallets.length}
                      className="swap-select"
                    >
                      {!wallets.length ? (
                        <option value="">{walletLoading ? '加载钱包中...' : '暂无可用钱包'}</option>
                      ) : null}
                      {wallets.map((wallet) => (
                        <option key={wallet.id} value={String(wallet.id)}>
                          {wallet.name || '钱包'} · {shortAddress(wallet.address)} · {chainConfig.nativeSymbol}{' '}
                          {formatNativeBalance(wallet.native_balance)}
                        </option>
                      ))}
                    </select>
                  </div>
                </label>

                <label className="swap-settings-field">
                  <span>滑点上限</span>
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

              <div className="swap-settings-footnote">
                钱包直接发起链上交易，报价由 OKX DEX 聚合返回。
              </div>
            </div>
          ) : null}

          <div className="swap-surface">
            <div className="swap-context-row">
              <div className="swap-context-pill strong">{chainConfig.label}</div>
              <div className="swap-context-pill">
                {selectedWallet
                  ? `${selectedWallet.name || '钱包'} · ${shortAddress(selectedWallet.address)}`
                  : walletLoading
                    ? '加载钱包中...'
                    : '未选择钱包'}
              </div>
            </div>

            <div className="swap-card-group">
              <div className="swap-card">
                <div className="swap-card-head">
                  <span>出售</span>
                  <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                    {fromTokenBalance && Number(fromTokenBalance) > 0 ? (
                      <button
                        type="button"
                        onClick={handleMaxAmount}
                        style={{
                          padding: '4px 10px',
                          borderRadius: '8px',
                          border: '1px solid rgba(255, 61, 156, 0.3)',
                          background: 'rgba(255, 61, 156, 0.08)',
                          color: '#d31f79',
                          fontSize: '11px',
                          fontWeight: '700',
                          cursor: 'pointer',
                          transition: 'all 0.18s ease',
                        }}
                        onMouseEnter={(e) => {
                          e.currentTarget.style.background = 'rgba(255, 61, 156, 0.15)';
                        }}
                        onMouseLeave={(e) => {
                          e.currentTarget.style.background = 'rgba(255, 61, 156, 0.08)';
                        }}
                      >
                        最大
                      </button>
                    ) : null}
                    <small>{fromTokenMeta ? shortAddress(fromTokenMeta.address, 8, 6) : '支持粘贴任意合约地址'}</small>
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
                    placeholder="选择代币"
                    onClick={() => {
                      setPickerSide('from');
                      setPickerOpen(true);
                    }}
                  />
                </div>
                <div className="swap-card-foot">
                  <span>{fromTokenMeta ? fromTokenMeta.name : '未选择卖出代币'}</span>
                  <span>
                    {fromTokenBalance && Number(fromTokenBalance) > 0
                      ? `余额: ${formatTokenAmount(fromTokenBalance)}`
                      : selectedWallet
                        ? `${chainConfig.nativeSymbol} ${formatNativeBalance(selectedWallet.native_balance)}`
                        : '--'}
                  </span>
                </div>
              </div>

              <button
                type="button"
                className="swap-switch-button"
                onClick={handleReverse}
                aria-label="切换兑换方向"
              >
                <ArrowDown size={18} strokeWidth={2.5} />
              </button>

              <div className="swap-card muted">
                <div className="swap-card-head">
                  <span>购买</span>
                  <small>{toTokenMeta ? shortAddress(toTokenMeta.address, 8, 6) : '选择目标代币'}</small>
                </div>
                <div className="swap-card-body">
                  <div className={`swap-quote-output${quoting ? ' loading' : ''}`}>
                    {quoting ? '...' : selectedQuoteAmount}
                  </div>
                  <TokenButton
                    token={toTokenMeta}
                    placeholder="选择代币"
                    onClick={() => {
                      setPickerSide('to');
                      setPickerOpen(true);
                    }}
                  />
                </div>
                <div className="swap-card-foot">
                  <span>{toTokenMeta ? toTokenMeta.name : '未选择目标代币'}</span>
                  <span>最少到账 {minReceived}</span>
                </div>
              </div>
            </div>

            <div className="swap-summary-card">
              {quoteInfo ? (
                <>
                  <DetailRow
                    label="预估到账"
                    value={`${selectedQuoteAmount} ${toTokenMeta?.symbol || ''}`.trim()}
                    emphasis
                  />
                  <DetailRow label="最少到账" value={`${minReceived} ${toTokenMeta?.symbol || ''}`.trim()} />
                  <DetailRow label="执行路径" value="OKX DEX Aggregator" />
                  <DetailRow label="预估 Gas" value={formatGas(quoteInfo?.estimated_gas)} />
                  <DetailRow label="滑点设置" value={`${slippage || '1.0'}%`} />
                </>
              ) : (
                <div className="swap-summary-empty">
                  <strong>输入数量后自动报价</strong>
                  <span>选择常用代币，或在选择器内直接粘贴 ERC-20 合约地址。</span>
                </div>
              )}
            </div>

            {quoteError ? (
              <div className="panel-error">
                <strong>报价失败:</strong> {quoteError}
              </div>
            ) : null}

            {execError ? (
              <div className="panel-error">
                <strong>兑换失败:</strong> {execError}
              </div>
            ) : null}

            {execSuccess ? (
              <div className="panel-success swap-success-card">
                <strong>兑换请求已提交</strong>
                <span>{execSuccess}</span>
              </div>
            ) : null}

            <button
              type="button"
              className="swap-submit-button"
              disabled={!isReadyToSwap}
              onClick={() => setShowConfirm(true)}
            >
              {executing ? '执行中...' : submitLabel}
            </button>

            <div className="swap-footnote">
              参考 Uniswap 的卡片式布局重构，保留你现有后端报价与执行链路。
            </div>
          </div>
        </div>

        {pickerOpen ? (
          <div className="swap-modal-overlay" onClick={() => setPickerOpen(false)}>
            <div className="swap-token-modal" onClick={(event) => event.stopPropagation()}>
              <div className="swap-modal-header">
                <div>
                  <div className="swap-modal-kicker">选择代币</div>
                  <h3>{pickerSide === 'from' ? '选择卖出代币' : '选择买入代币'}</h3>
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
                  placeholder="搜索符号，或粘贴合约地址"
                  autoFocus
                />
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
                {loadingWalletTokens ? (
                  <div style={{ padding: '20px', textAlign: 'center', color: '#8a92a6', fontSize: '13px' }}>
                    加载钱包余额中...
                  </div>
                ) : null}

                {pickerTokens.customCandidate ? (
                  <div className="swap-token-section">
                    <div className="swap-token-section-title">自定义地址</div>
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
                      <span className="swap-token-tag">粘贴使用</span>
                    </button>
                  </div>
                ) : null}

                {pickerTokens.withBalance && pickerTokens.withBalance.length > 0 ? (
                  <div className="swap-token-section">
                    <div className="swap-token-section-title">钱包余额</div>
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
                                ≈ ${token.valueUSDT.toFixed(2)}
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
                    <div className="swap-token-section-title">最近使用</div>
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
                          <span className="swap-token-tag">最近</span>
                        </div>
                      </button>
                    ))}
                  </div>
                ) : null}

                {pickerTokens.preset.length ? (
                  <div className="swap-token-section">
                    <div className="swap-token-section-title">常用代币</div>
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
                    <strong>没有匹配结果</strong>
                    <span>可以直接粘贴 ERC-20 合约地址。</span>
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
                  <h3>确认兑换</h3>
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
                      <span>支付</span>
                      <strong>{amount} {fromTokenMeta?.symbol || shortAddress(normalizedFromToken, 4, 4)}</strong>
                    </div>
                  </div>
                  <div className="swap-confirm-arrow">
                    <ArrowDown size={16} />
                  </div>
                  <div className="swap-confirm-token">
                    <TokenGlyph token={toTokenMeta || buildCustomToken(normalizedToToken)} />
                    <div>
                      <span>获得</span>
                      <strong>{selectedQuoteAmount} {toTokenMeta?.symbol || shortAddress(normalizedToToken, 4, 4)}</strong>
                    </div>
                  </div>
                </div>
              </div>

              <div className="swap-confirm-details">
                <DetailRow label="最少到账" value={`${minReceived} ${toTokenMeta?.symbol || ''}`.trim()} />
                <DetailRow label="滑点容忍" value={`${slippage || '1.0'}%`} />
                <DetailRow label="预估 Gas" value={formatGas(quoteInfo?.estimated_gas)} />
                <DetailRow label="聚合来源" value="OKX DEX Aggregator" />
              </div>

              <div className="swap-confirm-actions">
                <button
                  type="button"
                  className="swap-confirm-cancel"
                  onClick={() => setShowConfirm(false)}
                  disabled={executing}
                >
                  取消
                </button>
                <button
                  type="button"
                  className="swap-submit-button compact"
                  onClick={handleSwap}
                  disabled={executing}
                >
                  {executing ? '提交中...' : '提交交易'}
                </button>
              </div>
            </div>
          </div>
        ) : null}
      </div>
    </PanelShell>
  );
}
