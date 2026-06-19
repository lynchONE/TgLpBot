import { useCallback, useEffect, useMemo, useState } from 'react';
import { ArrowRightLeft, CheckCircle2, CircleDot, Coins, Factory, Flame, Layers, RefreshCw, Rocket, Search, SlidersHorizontal, Wallet, Zap } from 'lucide-react';
import {
  executeCreatePool,
  fetchHotPools,
  fetchWallets,
  previewCreatePool,
} from '../api';
import pancakeLogo from '../img/pancake.svg';
import uniswapLogo from '../img/uniswap.svg';
import PanelShell, { EmptyState } from './PanelShell';
import { normalizeHexAddress, normalizePoolAddress, shortAddress } from '../utils';
import CustomSelect from './CustomSelect';

const PROTOCOL_OPTIONS = [
  { key: 'univ3', label: 'Uniswap V3', badge: 'V3', logoSrc: uniswapLogo, accent: '#ff007a' },
  { key: 'univ4', label: 'Uniswap V4', badge: 'V4', logoSrc: uniswapLogo, accent: '#ff007a' },
  { key: 'pcsv3', label: 'PancakeSwap V3', badge: 'V3', logoSrc: pancakeLogo, accent: '#d1884f' },
];

const MODE_OPTIONS = [
  { key: 'create_and_seed', label: '创建并首仓', desc: '建池后立即注入首笔流动性。' },
  { key: 'create_only', label: '只创建池子', desc: '仅初始化池子价格，不执行首仓。' },
];

const AMOUNT_MODE_OPTIONS = [
  { key: 'dual_exact', label: '双币精确输入', desc: '两边数量都由你手动指定。' },
  { key: 'single_auto_swap', label: '单币自动配比', desc: '只输入一边，系统按价格和区间补齐另一边。' },
];

const SOURCE_TABS = [
  { key: 'manual', label: '手动输入', icon: Search },
  { key: 'hot', label: '热门池子', icon: Flame },
];

const FIXED_FEE_TIERS = {
  univ3: [100, 500, 3000, 10000],
  pcsv3: [100, 500, 2500, 10000],
  univ4: [100, 500, 3000, 10000],
};

const COMMON_TICK_SPACING = {
  100: 1,
  500: 10,
  2500: 50,
  3000: 60,
  10000: 200,
};

const STABLE_QUOTES = new Set(['USD', 'USDT', 'USDC', 'BUSD', 'FDUSD']);
const PRICE_RE = /^1\s+([A-Za-z0-9._-]+)\s*=\s*([0-9.]+)\s*([A-Za-z0-9._-]+)$/i;

function formatFeeTier(feeTier) {
  const n = Number(feeTier || 0);
  if (!Number.isFinite(n) || n <= 0) return '--';
  return `${(n / 10000).toFixed(4)}%`;
}

function getProtocolOption(protocol) {
  return PROTOCOL_OPTIONS.find((item) => item.key === protocol) || null;
}

function defaultFeeTier(protocol) {
  const values = FIXED_FEE_TIERS[protocol] || FIXED_FEE_TIERS.univ3;
  return values[1] || values[0] || 500;
}

function defaultTickSpacing(feeTier) {
  return COMMON_TICK_SPACING[Number(feeTier || 0)] || 0;
}

function inferProtocol(exchangeName, versionText) {
  const text = `${String(exchangeName || '').toLowerCase()} ${String(versionText || '').toLowerCase()}`;
  if (text.includes('pancake') || text.includes('pcs')) return 'pcsv3';
  if (text.includes('uniswap') && text.includes('v4')) return 'univ4';
  if (text.includes('uniswap')) return 'univ3';
  return '';
}

function buildProtocolMeta(exchangeName, versionText) {
  const protocol = inferProtocol(exchangeName, versionText);
  const option = getProtocolOption(protocol);
  return {
    protocol,
    label: option?.label || String(exchangeName || '').trim() || '--',
    badge: option?.badge || String(versionText || '').trim().toUpperCase(),
    logoSrc: option?.logoSrc || '',
    accent: option?.accent || '',
  };
}

function parsePair(pair) {
  const parts = String(pair || '')
    .split('/')
    .map((item) => String(item || '').trim())
    .filter(Boolean);
  return [parts[0] || '', parts[1] || ''];
}

function buildPricePrefill(token0Symbol, token1Symbol, priceDisplay) {
  const match = String(priceDisplay || '').trim().match(PRICE_RE);
  if (!match) return '';
  const quotedSymbol = String(match[1] || '').trim().toUpperCase();
  const quotedPrice = Number(match[2]);
  const quoteCurrency = String(match[3] || '').trim().toUpperCase();
  if (!Number.isFinite(quotedPrice) || quotedPrice <= 0) return '';
  if (!STABLE_QUOTES.has(quoteCurrency)) return '';

  const sym0 = String(token0Symbol || '').trim().toUpperCase();
  const sym1 = String(token1Symbol || '').trim().toUpperCase();
  if (quotedSymbol === sym0 && STABLE_QUOTES.has(sym1)) return String(quotedPrice);
  if (quotedSymbol === sym1 && STABLE_QUOTES.has(sym0)) return String(1 / quotedPrice);
  return '';
}

function feeTierFromPct(value) {
  const n = Number(value ?? 0);
  if (!Number.isFinite(n) || n <= 0) return 0;
  return Math.round(n * 10000);
}

function parsePositiveFloat(raw) {
  const value = Number.parseFloat(String(raw ?? '').trim());
  return Number.isFinite(value) && value > 0 ? value : null;
}

function formatDisplayAmount(value) {
  const n = Number(value);
  if (!Number.isFinite(n) || n <= 0) return '';
  let digits = 8;
  if (n >= 1000) digits = 2;
  else if (n >= 1) digits = 6;
  else if (n >= 0.01) digits = 8;
  else digits = 10;
  return n.toLocaleString('en-US', {
    useGrouping: false,
    maximumFractionDigits: digits,
  });
}

function estimateMirrorAmounts({ singleInputSide, amountA, amountB, priceAB }) {
  const price = parsePositiveFloat(priceAB);
  if (!price || !singleInputSide) return { mirrorA: '', mirrorB: '', source: '' };
  if (singleInputSide === 'token_a') {
    const value = parsePositiveFloat(amountA);
    if (!value) return { mirrorA: '', mirrorB: '', source: '' };
    return {
      mirrorA: formatDisplayAmount(value),
      mirrorB: formatDisplayAmount(value * price),
      source: 'spot_price',
    };
  }
  const value = parsePositiveFloat(amountB);
  if (!value) return { mirrorA: '', mirrorB: '', source: '' };
  return {
    mirrorA: formatDisplayAmount(value / price),
    mirrorB: formatDisplayAmount(value),
    source: 'spot_price',
  };
}

function buildSources(rows, kind) {
  return (Array.isArray(rows) ? rows : [])
    .map((row, index) => {
      const token0Address = normalizeHexAddress(kind === 'hot' ? row?.token0_address : row?.token0);
      const token1Address = normalizeHexAddress(kind === 'hot' ? row?.token1_address : row?.token1);
      if (!token0Address || !token1Address) return null;

      const [pair0, pair1] = parsePair(kind === 'hot' ? row?.trading_pair : row?.pair);
      const protocolMeta = buildProtocolMeta(
        kind === 'hot' ? row?.factory_name || row?.dex || '热门池子' : row?.exchange || '',
        kind === 'hot' ? row?.protocol_version : row?.pool_version,
      );
      const poolKey = normalizePoolAddress(row?.pool_address || row?.pool_id) || `fallback-${index}`;
      const feeTier = feeTierFromPct(kind === 'hot' ? row?.fee_percentage : row?.fee_pct);

      return {
        key: `${kind}:${poolKey}`,
        label: String(kind === 'hot' ? row?.trading_pair : row?.pair || '--').trim(),
        token0Address,
        token1Address,
        token0Symbol: String(row?.token0_symbol || pair0 || '').trim(),
        token1Symbol: String(row?.token1_symbol || pair1 || '').trim(),
        feeTier,
        tickSpacing: Number(row?.tick_spacing || row?.tickSpacing || 0) || 0,
        priceHint:
          kind === 'hot'
            ? String(row?.price_display || '').trim()
            : '可直接带入协议、费率和币对，再根据你的策略调整。',
        pricePrefill:
          kind === 'hot'
            ? buildPricePrefill(row?.token0_symbol || pair0, row?.token1_symbol || pair1, row?.price_display)
            : '',
        protocol: protocolMeta.protocol,
        protocolMeta,
      };
    })
    .filter(Boolean)
    .slice(0, 12);
}

function ProtocolMark({ protocol, meta, compact = false }) {
  const resolved = meta || getProtocolOption(protocol);
  if (!resolved) return null;
  return (
    <span
      className={`cp-protocol-pill${compact ? ' compact' : ''}`}
      style={resolved.accent ? { '--cp-protocol-accent': resolved.accent } : undefined}
    >
      {resolved.logoSrc ? <img src={resolved.logoSrc} alt="" className="cp-protocol-logo" /> : null}
      {resolved.badge ? <span className="cp-protocol-version">{resolved.badge}</span> : null}
    </span>
  );
}

function StepSection({ step, title, subtitle, children, accent = false }) {
  return (
    <div className={`cp-section${accent ? ' cp-section--accent' : ''}`}>
      <div className="cp-section-header">
        <span className="cp-step-badge">{step}</span>
        <div className="cp-section-title-wrap">
          <span className="cp-section-title">{title}</span>
          {subtitle ? <span className="cp-section-subtitle">{subtitle}</span> : null}
        </div>
      </div>
      <div className="cp-section-body">{children}</div>
    </div>
  );
}

export default function CreatePoolPanel({ apiBaseUrl, initData, hasInitData }) {
  const [wallets, setWallets] = useState([]);
  const [walletsLoading, setWalletsLoading] = useState(false);
  const [walletsError, setWalletsError] = useState('');
  const [hotSources, setHotSources] = useState([]);
  const [sourcesLoading, setSourcesLoading] = useState(false);
  const [sourcesError, setSourcesError] = useState('');

  const [protocol, setProtocol] = useState('univ3');
  const [mode, setMode] = useState('create_and_seed');
  const [amountMode, setAmountMode] = useState('dual_exact');
  const [rangeMode, setRangeMode] = useState('full_range');
  const [walletId, setWalletId] = useState(0);
  const [sourceTab, setSourceTab] = useState('manual');
  const [selectedSourceKey, setSelectedSourceKey] = useState('');
  const [tokenAAddress, setTokenAAddress] = useState('');
  const [tokenBAddress, setTokenBAddress] = useState('');
  const [feeTier, setFeeTier] = useState(defaultFeeTier('univ3'));
  const [customFeeTierInput, setCustomFeeTierInput] = useState('');
  const [tickSpacing, setTickSpacing] = useState(defaultTickSpacing(defaultFeeTier('univ3')));
  const [initialPrice, setInitialPrice] = useState('');
  const [minPrice, setMinPrice] = useState('');
  const [maxPrice, setMaxPrice] = useState('');
  const [amountA, setAmountA] = useState('');
  const [amountB, setAmountB] = useState('');
  const [singleInputSide, setSingleInputSide] = useState('token_a');
  const [preview, setPreview] = useState(null);
  const [previewLoading, setPreviewLoading] = useState(false);
  const [executeLoading, setExecuteLoading] = useState(false);
  const [error, setError] = useState('');
  const [result, setResult] = useState(null);

  const activeSources = useMemo(() => (sourceTab === 'hot' ? hotSources : []), [hotSources, sourceTab]);
  const selectedSource = useMemo(
    () => activeSources.find((item) => item.key === selectedSourceKey) || null,
    [activeSources, selectedSourceKey],
  );
  const protocolFees = useMemo(() => FIXED_FEE_TIERS[protocol] || FIXED_FEE_TIERS.univ3, [protocol]);
  const protocolMeta = useMemo(() => getProtocolOption(protocol), [protocol]);
  const selectedWalletId = useMemo(() => {
    const numeric = Number(walletId || 0);
    if (Number.isFinite(numeric) && numeric > 0 && wallets.some((item) => Number(item?.id || 0) === numeric)) {
      return numeric;
    }
    const fallback = wallets.find((item) => item?.is_default) || wallets[0];
    return Number(fallback?.id || 0);
  }, [walletId, wallets]);
  const selectedWallet = useMemo(
    () => wallets.find((item) => Number(item?.id || 0) === selectedWalletId) || null,
    [selectedWalletId, wallets],
  );

  const effectiveAmountA = useMemo(() => {
    if (mode !== 'create_and_seed') return '';
    if (amountMode !== 'single_auto_swap') return amountA;
    return singleInputSide === 'token_a' ? amountA : '';
  }, [amountA, amountMode, mode, singleInputSide]);

  const effectiveAmountB = useMemo(() => {
    if (mode !== 'create_and_seed') return '';
    if (amountMode !== 'single_auto_swap') return amountB;
    return singleInputSide === 'token_b' ? amountB : '';
  }, [amountB, amountMode, mode, singleInputSide]);

  const priceForMirror =
    String(initialPrice || '').trim() ||
    String(selectedSource?.pricePrefill || '').trim() ||
    String(preview?.suggested_initial_price || '').trim();

  const localMirror = useMemo(
    () =>
      estimateMirrorAmounts({
        singleInputSide,
        amountA,
        amountB,
        priceAB: priceForMirror,
      }),
    [amountA, amountB, priceForMirror, singleInputSide],
  );

  const displayedAmountA =
    amountMode === 'single_auto_swap' && singleInputSide === 'token_b'
      ? preview?.mirror_amount_a || localMirror.mirrorA
      : amountA;
  const displayedAmountB =
    amountMode === 'single_auto_swap' && singleInputSide === 'token_a'
      ? preview?.mirror_amount_b || localMirror.mirrorB
      : amountB;
  const mirrorSource = preview?.mirror_amount_source || localMirror.source;

  const payload = useMemo(
    () => ({
      apiBaseUrl,
      initData,
      chain: 'bsc',
      protocol,
      walletId: selectedWalletId || undefined,
      tokenAAddress,
      tokenBAddress,
      feeTier: Number(feeTier || 0),
      tickSpacing: protocol === 'univ4' ? Number(tickSpacing || 0) || undefined : undefined,
      initialPrice,
      mode,
      rangeMode,
      amountMode: mode === 'create_and_seed' ? amountMode : 'dual_exact',
      minPrice: rangeMode === 'custom_range' ? minPrice : undefined,
      maxPrice: rangeMode === 'custom_range' ? maxPrice : undefined,
      amountA: effectiveAmountA,
      amountB: effectiveAmountB,
    }),
    [
      amountMode,
      apiBaseUrl,
      effectiveAmountA,
      effectiveAmountB,
      feeTier,
      initData,
      initialPrice,
      maxPrice,
      minPrice,
      mode,
      protocol,
      rangeMode,
      selectedWalletId,
      tickSpacing,
      tokenAAddress,
      tokenBAddress,
    ],
  );

  const invalidate = useCallback(() => {
    setPreview(null);
    setResult(null);
    setError('');
  }, []);

  useEffect(() => {
    if (protocol !== 'univ4') {
      const allowed = FIXED_FEE_TIERS[protocol] || FIXED_FEE_TIERS.univ3;
      const nextFee = allowed.includes(Number(feeTier || 0)) ? Number(feeTier || 0) : defaultFeeTier(protocol);
      const nextSpacing = defaultTickSpacing(nextFee);
      if (nextFee !== Number(feeTier || 0)) setFeeTier(nextFee);
      if (nextSpacing !== Number(tickSpacing || 0)) setTickSpacing(nextSpacing);
      if (customFeeTierInput) setCustomFeeTierInput('');
      return;
    }

    if (Number(feeTier || 0) <= 0) {
      setFeeTier(defaultFeeTier(protocol));
    }
    if (Number(tickSpacing || 0) <= 0) {
      const nextSpacing = defaultTickSpacing(feeTier || defaultFeeTier(protocol));
      if (nextSpacing > 0) setTickSpacing(nextSpacing);
    }
  }, [customFeeTierInput, feeTier, protocol, tickSpacing]);

  useEffect(() => {
    if (!hasInitData) {
      setWallets([]);
      setHotSources([]);
      return undefined;
    }

    const controller = new AbortController();
    setWalletsLoading(true);
    setWalletsError('');
    fetchWallets({ apiBaseUrl, initData, chain: 'bsc', signal: controller.signal })
      .then((resp) => {
        const nextWallets = Array.isArray(resp?.wallets) ? resp.wallets : [];
        setWallets(nextWallets);
        const fallback = nextWallets.find((item) => item?.is_default) || nextWallets[0];
        setWalletId((prev) => {
          const prevId = Number(prev || 0);
          if (Number.isFinite(prevId) && prevId > 0 && nextWallets.some((item) => Number(item?.id || 0) === prevId)) {
            return prevId;
          }
          return Number(fallback?.id || 0);
        });
      })
      .catch((err) => {
        if (err?.name !== 'AbortError') {
          setWallets([]);
          setWalletsError(String(err?.message || err));
        }
      })
      .finally(() => setWalletsLoading(false));
    return () => controller.abort();
  }, [apiBaseUrl, hasInitData, initData]);

  const reloadSources = useCallback(() => {
    if (!hasInitData) return undefined;
    const controller = new AbortController();
    setSourcesLoading(true);
    setSourcesError('');
    Promise.all([
      fetchHotPools({
        apiBaseUrl,
        initData,
        chain: 'bsc',
        sort: 'fees',
        timeframeMinutes: 5,
        limit: 24,
        signal: controller.signal,
      }),
    ])
      .then(([hotResp]) => {
        setHotSources(buildSources(hotResp?.data, 'hot'));
      })
      .catch((err) => {
        if (err?.name !== 'AbortError') {
          setSourcesError(String(err?.message || err));
        }
      })
      .finally(() => setSourcesLoading(false));
    return () => controller.abort();
  }, [apiBaseUrl, hasInitData, initData]);

  useEffect(() => {
    const cleanup = reloadSources();
    return () => {
      if (typeof cleanup === 'function') cleanup();
    };
  }, [reloadSources]);

  const applySource = useCallback(
    (source, tabKey) => {
      if (!source) return;
      const nextProtocol = source.protocol || protocol;
      const nextFeeTier = Number(source.feeTier || 0) || defaultFeeTier(nextProtocol);
      const nextSpacing = Number(source.tickSpacing || 0) || defaultTickSpacing(nextFeeTier);
      setSourceTab(tabKey);
      setSelectedSourceKey(source.key);
      setProtocol(nextProtocol);
      setTokenAAddress(source.token0Address || '');
      setTokenBAddress(source.token1Address || '');
      setFeeTier(nextFeeTier);
      setCustomFeeTierInput(
        nextProtocol === 'univ4' && !FIXED_FEE_TIERS.univ4.includes(nextFeeTier) ? String(nextFeeTier) : '',
      );
      setTickSpacing(nextSpacing);
      setInitialPrice(source.pricePrefill || '');
      invalidate();
    },
    [invalidate, protocol],
  );

  const handlePreview = useCallback(async () => {
    if (!hasInitData) {
      setError('请先完成 Telegram 登录。');
      return;
    }
    setPreviewLoading(true);
    setError('');
    setResult(null);
    try {
      const resp = await previewCreatePool(payload);
      setPreview(resp);
      if (!String(initialPrice || '').trim() && String(resp?.suggested_initial_price || '').trim()) {
        setInitialPrice(String(resp.suggested_initial_price).trim());
      }
      if (protocol === 'univ4' && Number(tickSpacing || 0) <= 0 && Number(resp?.tick_spacing || 0) > 0) {
        setTickSpacing(Number(resp.tick_spacing));
      }
      if (resp?.single_sided_input === 'token_a' || resp?.single_sided_input === 'token_b') {
        setSingleInputSide(resp.single_sided_input);
      }
    } catch (err) {
      setPreview(null);
      setError(String(err?.message || err));
    } finally {
      setPreviewLoading(false);
    }
  }, [hasInitData, initialPrice, payload, protocol, tickSpacing]);

  const handleExecute = useCallback(async () => {
    if (!hasInitData) {
      setError('请先完成 Telegram 登录。');
      return;
    }
    setExecuteLoading(true);
    setError('');
    setResult(null);
    try {
      const resp = await executeCreatePool(payload);
      setResult(resp);
      setPreview(null);
    } catch (err) {
      setError(String(err?.message || err));
    } finally {
      setExecuteLoading(false);
    }
  }, [hasInitData, payload]);

  const handleProtocolChange = useCallback(
    (nextProtocol) => {
      setProtocol(nextProtocol);
      const nextFee = defaultFeeTier(nextProtocol);
      setFeeTier(nextFee);
      setTickSpacing(defaultTickSpacing(nextFee));
      setCustomFeeTierInput('');
      invalidate();
    },
    [invalidate],
  );

  const handleAmountModeChange = useCallback(
    (nextMode) => {
      setAmountMode(nextMode);
      if (nextMode === 'single_auto_swap') {
        setSingleInputSide((prev) => prev || 'token_a');
        if (singleInputSide === 'token_a') {
          setAmountB('');
        } else {
          setAmountA('');
        }
      }
      invalidate();
    },
    [invalidate, singleInputSide],
  );

  const handleSingleInputSideChange = useCallback(
    (side) => {
      setSingleInputSide(side);
      if (side === 'token_a') {
        setAmountB('');
      } else {
        setAmountA('');
      }
      invalidate();
    },
    [invalidate],
  );

  const handleFeeTierChipClick = useCallback(
    (value) => {
      setFeeTier(value);
      if (protocol === 'univ4') {
        setCustomFeeTierInput('');
      }
      const nextSpacing = defaultTickSpacing(value);
      if (nextSpacing > 0) setTickSpacing(nextSpacing);
      invalidate();
    },
    [invalidate, protocol],
  );

  const handleCustomFeeInputChange = useCallback(
    (event) => {
      const nextValue = String(event.target.value || '').replace(/[^\d]/g, '').slice(0, 6);
      setCustomFeeTierInput(nextValue);
      const numeric = Number(nextValue || 0);
      setFeeTier(numeric);
      const nextSpacing = defaultTickSpacing(numeric);
      if (nextSpacing > 0) setTickSpacing(nextSpacing);
      invalidate();
    },
    [invalidate],
  );

  const handleAmountAChange = useCallback(
    (event) => {
      const value = event.target.value;
      if (amountMode === 'single_auto_swap') {
        setSingleInputSide('token_a');
        setAmountA(value);
        setAmountB('');
      } else {
        setAmountA(value);
      }
      invalidate();
    },
    [amountMode, invalidate],
  );

  const handleAmountBChange = useCallback(
    (event) => {
      const value = event.target.value;
      if (amountMode === 'single_auto_swap') {
        setSingleInputSide('token_b');
        setAmountB(value);
        setAmountA('');
      } else {
        setAmountB(value);
      }
      invalidate();
    },
    [amountMode, invalidate],
  );

  const actionDisabled = !hasInitData || walletsLoading || !selectedWalletId;

  return (
    <PanelShell
      title="创建池子"
      subtitle="BSC · Uniswap / Pancake"
      icon={Factory}
      actions={
        <button
          type="button"
          className="ghost-chip"
          onClick={reloadSources}
          disabled={!hasInitData || sourcesLoading}
        >
          <RefreshCw size={13} className={sourcesLoading ? 'spin' : ''} />
          刷新来源
        </button>
      }
    >
      {!hasInitData ? (
        <EmptyState text="请先完成 Telegram 登录，然后再创建池子。" />
      ) : (
        <div className="cp-root">
          <StepSection step="1" title="协议与模式" subtitle="先确定目标协议和首仓方式">
            <div className="cp-segmented-row">
              <div className="cp-segmented">
                {PROTOCOL_OPTIONS.map((item) => (
                  <button
                    key={item.key}
                    type="button"
                    className={`cp-seg-btn${protocol === item.key ? ' active' : ''}`}
                    onClick={() => handleProtocolChange(item.key)}
                  >
                    <span
                      className="cp-seg-icon"
                      style={item.accent ? { '--cp-protocol-accent': item.accent } : undefined}
                    >
                      <img src={item.logoSrc} alt="" className="cp-protocol-logo" />
                    </span>
                    <span className="cp-seg-label">{item.label}</span>
                  </button>
                ))}
              </div>
            </div>

            <div className="cp-mode-cards">
              {MODE_OPTIONS.map((item) => (
                <button
                  key={item.key}
                  type="button"
                  className={`cp-mode-card${mode === item.key ? ' active' : ''}`}
                  onClick={() => {
                    setMode(item.key);
                    invalidate();
                  }}
                >
                  <span className="cp-mode-radio">
                    {mode === item.key ? <CircleDot size={16} /> : <span className="cp-mode-radio-empty" />}
                  </span>
                  <span className="cp-mode-text">
                    <strong>{item.label}</strong>
                    <small>{item.desc}</small>
                  </span>
                </button>
              ))}
            </div>

            {mode === 'create_and_seed' ? (
              <div className="cp-mode-cards">
                {AMOUNT_MODE_OPTIONS.map((item) => (
                  <button
                    key={item.key}
                    type="button"
                    className={`cp-mode-card${amountMode === item.key ? ' active' : ''}`}
                    onClick={() => handleAmountModeChange(item.key)}
                  >
                    <span className="cp-mode-radio">
                      {amountMode === item.key ? (
                        <CircleDot size={16} />
                      ) : (
                        <span className="cp-mode-radio-empty" />
                      )}
                    </span>
                    <span className="cp-mode-text">
                      <strong>{item.label}</strong>
                      <small>{item.desc}</small>
                    </span>
                  </button>
                ))}
              </div>
            ) : null}
          </StepSection>

          <StepSection
            step="2"
            title="来源池预填"
            subtitle="可直接带入币对、协议、费率和一个价格参考"
          >
            <div className="cp-source-tabs">
              {SOURCE_TABS.map((tab) => {
                const Icon = tab.icon;
                return (
                  <button
                    key={tab.key}
                    type="button"
                    className={`cp-source-tab${sourceTab === tab.key ? ' active' : ''}`}
                    onClick={() => setSourceTab(tab.key)}
                  >
                    <Icon size={14} />
                    <span>{tab.label}</span>
                  </button>
                );
              })}
            </div>

            {sourceTab === 'manual' ? (
              <div className="cp-source-empty">
                <Search size={20} className="cp-source-empty-icon" />
                <span>手动模式下直接填写 Token、费率、价格和区间。</span>
              </div>
            ) : sourcesLoading && activeSources.length === 0 ? (
              <div className="cp-source-empty">
                <RefreshCw size={16} className="spin" />
                <span>正在加载来源池子...</span>
              </div>
            ) : activeSources.length === 0 ? (
              <div className="cp-source-empty">
                <span>{sourcesError || '当前没有可用的来源池子。'}</span>
              </div>
            ) : (
              <div className="cp-source-grid">
                {activeSources.map((source) => (
                  <button
                    key={source.key}
                    type="button"
                    className={`cp-source-card${selectedSourceKey === source.key ? ' active' : ''}`}
                    onClick={() => applySource(source, sourceTab)}
                  >
                    <div className="cp-source-top">
                      <span className="cp-source-pair">{source.label}</span>
                      <span className="cp-source-fee">{formatFeeTier(source.feeTier)}</span>
                    </div>
                    <div className="cp-source-bottom">
                      <ProtocolMark protocol={source.protocol} meta={source.protocolMeta} compact />
                      {source.tickSpacing ? <small>tick {source.tickSpacing}</small> : null}
                    </div>
                    {selectedSourceKey === source.key ? (
                      <span className="cp-source-check">
                        <CheckCircle2 size={14} />
                      </span>
                    ) : null}
                  </button>
                ))}
              </div>
            )}

            {selectedSource ? (
              <div className="cp-source-hint-bar">
                <Zap size={13} />
                <div className="cp-source-hint-content">
                  <span>
                    已选择 <strong>{selectedSource.label}</strong>
                  </span>
                  <div className="cp-source-hint-meta">
                    <ProtocolMark protocol={selectedSource.protocol} meta={selectedSource.protocolMeta} compact />
                    {selectedSource.priceHint ? <span>{selectedSource.priceHint}</span> : null}
                  </div>
                </div>
              </div>
            ) : null}
          </StepSection>

          <StepSection step="3" title="参数配置" subtitle="确认钱包、价格、区间和首仓金额" accent>
            <div className="cp-form-grid">
              <div className="cp-field">
                <label className="cp-field-label">
                  <Wallet size={13} />
                  <span>执行钱包</span>
                </label>
                <div className="cp-select-wrap">
                  <CustomSelect
                    value={selectedWalletId || ''}
                    onChange={(value) => {
                      setWalletId(Number(value || 0));
                      invalidate();
                    }}
                    options={wallets.length === 0
                      ? [{ value: '', label: '暂无可用钱包' }]
                      : wallets.map((item) => ({
                        value: item.id,
                        label: `${item.name || shortAddress(item.address, 6, 4)} · ${shortAddress(item.address, 6, 4)}`,
                      }))}
                    disabled={wallets.length === 0}
                  />
                </div>
                {walletsLoading ? <small className="cp-field-hint">正在读取钱包列表...</small> : null}
                {walletsError ? <small className="cp-field-error">{walletsError}</small> : null}
              </div>

              <div className="cp-field">
                <label className="cp-field-label">
                  <Layers size={13} />
                  <span>费率档位</span>
                </label>
                <div className="cp-fee-chips">
                  {protocolFees.map((item) => (
                    <button
                      key={item}
                      type="button"
                      className={`cp-fee-chip${Number(feeTier || 0) === item ? ' active' : ''}`}
                      onClick={() => handleFeeTierChipClick(item)}
                    >
                      {formatFeeTier(item)}
                    </button>
                  ))}
                </div>
                {protocol === 'univ4' ? (
                  <div className="cp-fee-custom">
                    <input
                      type="number"
                      min="1"
                      step="1"
                      className="cp-fee-custom-input"
                      value={customFeeTierInput}
                      onChange={handleCustomFeeInputChange}
                      placeholder="任意静态 fee，例如 750"
                    />
                    <span className="cp-fee-custom-note">
                      Uniswap V4 支持任意静态费率。常见档位仍可一键选择，自定义费率时请确认 tick
                      spacing。
                    </span>
                  </div>
                ) : (
                  <small className="cp-field-hint">
                    {protocol === 'pcsv3'
                      ? 'PancakeSwap V3 固定支持 0.0100% / 0.0500% / 0.2500% / 1.0000%。'
                      : 'Uniswap V3 固定支持 0.0100% / 0.0500% / 0.3000% / 1.0000%。'}
                  </small>
                )}
              </div>

              <div className="cp-field">
                <label className="cp-field-label">
                  <Coins size={13} />
                  <span>Token A 地址</span>
                </label>
                <input
                  type="text"
                  className="cp-input"
                  value={tokenAAddress}
                  onChange={(event) => {
                    setTokenAAddress(event.target.value);
                    invalidate();
                  }}
                  placeholder="0x..."
                />
              </div>

              <div className="cp-field">
                <label className="cp-field-label">
                  <ArrowRightLeft size={13} />
                  <span>Token B 地址</span>
                </label>
                <input
                  type="text"
                  className="cp-input"
                  value={tokenBAddress}
                  onChange={(event) => {
                    setTokenBAddress(event.target.value);
                    invalidate();
                  }}
                  placeholder="0x..."
                />
              </div>

              {protocol === 'univ4' ? (
                <div className="cp-field">
                  <label className="cp-field-label">
                    <SlidersHorizontal size={13} />
                    <span>Tick Spacing</span>
                  </label>
                  <input
                    type="number"
                    min="1"
                    step="1"
                    className="cp-input"
                    value={tickSpacing || ''}
                    onChange={(event) => {
                      setTickSpacing(Number(event.target.value || 0));
                      invalidate();
                    }}
                    placeholder="例如 1 / 10 / 60 / 200"
                  />
                  <small className="cp-field-hint">
                    V4 自定义费率不能靠 fee 反推 tick spacing，建池时这里必须正确。
                  </small>
                </div>
              ) : null}

              <div className={`cp-field${protocol === 'univ4' ? '' : ' cp-field--full'}`}>
                <label className="cp-field-label">
                  <span>初始价格</span>
                </label>
                <input
                  type="text"
                  className="cp-input"
                  value={initialPrice}
                  onChange={(event) => {
                    setInitialPrice(event.target.value);
                    invalidate();
                  }}
                  placeholder="1 TokenA = X TokenB，留空则用后端建议价"
                />
                <small className="cp-field-hint">
                  单币模式下，前端会先按这里的价格粗估另一边金额；预览后会再按真实区间修正。
                </small>
              </div>

              <div className="cp-field cp-field--full">
                <label className="cp-field-label">
                  <SlidersHorizontal size={13} />
                  <span>区间模式</span>
                </label>
                <div className="cp-fee-chips">
                  <button
                    type="button"
                    className={`cp-fee-chip${rangeMode === 'full_range' ? ' active' : ''}`}
                    onClick={() => {
                      setRangeMode('full_range');
                      invalidate();
                    }}
                  >
                    Full Range
                  </button>
                  <button
                    type="button"
                    className={`cp-fee-chip${rangeMode === 'custom_range' ? ' active' : ''}`}
                    onClick={() => {
                      setRangeMode('custom_range');
                      invalidate();
                    }}
                  >
                    自定义区间
                  </button>
                </div>
              </div>

              {rangeMode === 'custom_range' ? (
                <>
                  <div className="cp-field">
                    <label className="cp-field-label">
                      <span>下沿价格</span>
                    </label>
                    <input
                      type="text"
                      className="cp-input"
                      value={minPrice}
                      onChange={(event) => {
                        setMinPrice(event.target.value);
                        invalidate();
                      }}
                      placeholder="1 TokenA = X TokenB"
                    />
                  </div>
                  <div className="cp-field">
                    <label className="cp-field-label">
                      <span>上沿价格</span>
                    </label>
                    <input
                      type="text"
                      className="cp-input"
                      value={maxPrice}
                      onChange={(event) => {
                        setMaxPrice(event.target.value);
                        invalidate();
                      }}
                      placeholder="1 TokenA = X TokenB"
                    />
                  </div>
                </>
              ) : null}

              {mode === 'create_and_seed' && amountMode === 'single_auto_swap' ? (
                <div className="cp-field cp-field--full">
                  <label className="cp-field-label">
                    <ArrowRightLeft size={13} />
                    <span>单侧输入</span>
                  </label>
                  <div className="cp-fee-chips">
                    <button
                      type="button"
                      className={`cp-fee-chip${singleInputSide === 'token_a' ? ' active' : ''}`}
                      onClick={() => handleSingleInputSideChange('token_a')}
                    >
                      输入 Token A
                    </button>
                    <button
                      type="button"
                      className={`cp-fee-chip${singleInputSide === 'token_b' ? ' active' : ''}`}
                      onClick={() => handleSingleInputSideChange('token_b')}
                    >
                      输入 Token B
                    </button>
                  </div>
                  <small className="cp-field-hint">
                    先按当前价格给出镜像数量，点击预览后会按目标区间重新计算，并显示自动换币方向。
                  </small>
                </div>
              ) : null}

              {mode === 'create_and_seed' ? (
                <>
                  <div className="cp-field">
                    <label className="cp-field-label">
                      <span>Token A 数量</span>
                    </label>
                    <input
                      type="text"
                      className={`cp-input${
                        amountMode === 'single_auto_swap' && singleInputSide === 'token_b'
                          ? ' cp-input--readonly'
                          : ''
                      }`}
                      value={displayedAmountA}
                      readOnly={amountMode === 'single_auto_swap' && singleInputSide === 'token_b'}
                      onChange={handleAmountAChange}
                      placeholder="例如 1000"
                    />
                    {amountMode === 'single_auto_swap' ? (
                      <small className="cp-field-hint">
                        {singleInputSide === 'token_a'
                          ? '当前是主输入侧。'
                          : mirrorSource === 'range_ratio'
                            ? '这里显示的是按目标区间估算出来的镜像数量。'
                            : '这里显示的是按当前价格粗估出来的镜像数量。'}
                      </small>
                    ) : null}
                  </div>

                  <div className="cp-field">
                    <label className="cp-field-label">
                      <span>Token B 数量</span>
                    </label>
                    <input
                      type="text"
                      className={`cp-input${
                        amountMode === 'single_auto_swap' && singleInputSide === 'token_a'
                          ? ' cp-input--readonly'
                          : ''
                      }`}
                      value={displayedAmountB}
                      readOnly={amountMode === 'single_auto_swap' && singleInputSide === 'token_a'}
                      onChange={handleAmountBChange}
                      placeholder="例如 0.5"
                    />
                    {amountMode === 'single_auto_swap' ? (
                      <small className="cp-field-hint">
                        {singleInputSide === 'token_b'
                          ? '当前是主输入侧。'
                          : mirrorSource === 'range_ratio'
                            ? '这里显示的是按目标区间估算出来的镜像数量。'
                            : '这里显示的是按当前价格粗估出来的镜像数量。'}
                      </small>
                    ) : null}
                  </div>
                </>
              ) : (
                <div className="cp-field cp-field--full">
                  <div className="cp-mode-notice">
                    <Zap size={14} />
                    <span>当前模式只会初始化池子价格，不会立即执行首仓。</span>
                  </div>
                </div>
              )}
            </div>
          </StepSection>

          <div className="cp-summary-strip">
            <div className="cp-summary-item">
              <span>链</span>
              <strong>BSC</strong>
            </div>
            <div className="cp-summary-divider" />
            <div className="cp-summary-item">
              <span>协议</span>
              <strong className="cp-summary-protocol">
                <ProtocolMark protocol={protocol} meta={protocolMeta} compact />
              </strong>
            </div>
            <div className="cp-summary-divider" />
            <div className="cp-summary-item">
              <span>区间</span>
              <strong>{rangeMode === 'full_range' ? 'Full Range' : 'Custom Range'}</strong>
            </div>
            <div className="cp-summary-divider" />
            <div className="cp-summary-item">
              <span>钱包</span>
              <strong>{selectedWallet ? selectedWallet.name || shortAddress(selectedWallet.address, 4, 4) : '--'}</strong>
            </div>
          </div>

          {error ? (
            <div className="cp-error-bar">
              <span>{error}</span>
            </div>
          ) : null}

          <div className="cp-actions">
            <button
              type="button"
              className="cp-btn cp-btn--ghost"
              onClick={handlePreview}
              disabled={previewLoading || executeLoading || actionDisabled}
            >
              {previewLoading ? <RefreshCw size={14} className="spin" /> : <Search size={14} />}
              {previewLoading ? '预览中...' : '预览参数'}
            </button>
            <button
              type="button"
              className="cp-btn cp-btn--primary"
              onClick={handleExecute}
              disabled={executeLoading || previewLoading || actionDisabled}
            >
              {executeLoading ? <RefreshCw size={14} className="spin" /> : <Rocket size={14} />}
              {executeLoading ? '执行中...' : mode === 'create_only' ? '创建池子' : '创建并首仓'}
            </button>
          </div>

          {preview ? (
            <div className={`cp-result-card${preview.ready_to_execute ? ' cp-result-card--ready' : ''}`}>
              <div className="cp-result-header">
                <span className="cp-result-title">预览结果</span>
                <span className={`cp-result-status${preview.ready_to_execute ? ' ready' : ''}`}>
                  {preview.pool_exists
                    ? '目标池子已存在'
                    : preview.ready_to_execute
                      ? '参数已满足执行条件'
                      : '仍有参数需要补齐'}
                </span>
              </div>

              <div className="cp-result-grid">
                <div className="cp-kv">
                  <span>Token 顺序</span>
                  <strong>
                    {preview.token0?.symbol || '--'} / {preview.token1?.symbol || '--'}
                  </strong>
                </div>
                <div className="cp-kv">
                  <span>Fee / Tick</span>
                  <strong>
                    {formatFeeTier(preview.fee_tier)} · {preview.tick_spacing || '--'}
                  </strong>
                </div>
                <div className="cp-kv">
                  <span>初始价格</span>
                  <strong>{preview.initial_price || preview.suggested_initial_price || '--'}</strong>
                </div>
                <div className="cp-kv">
                  <span>区间</span>
                  <strong>
                    {preview.range_mode === 'full_range'
                      ? 'Full Range'
                      : `${preview.min_price || '--'} ~ ${preview.max_price || '--'}`}
                  </strong>
                </div>
                <div className="cp-kv">
                  <span>Ticks</span>
                  <strong>
                    {preview.tick_lower} / {preview.tick_upper}
                  </strong>
                </div>
                <div className="cp-kv">
                  <span>金额模式</span>
                  <strong>
                    {preview.amount_mode === 'single_auto_swap' ? '单币自动配比' : '双币精确输入'}
                  </strong>
                </div>
                <div className="cp-kv">
                  <span>镜像数量</span>
                  <strong>
                    {preview.mirror_amount_a || preview.mirror_amount_b
                      ? `${preview.mirror_amount_a || '--'} / ${preview.mirror_amount_b || '--'}`
                      : '--'}
                  </strong>
                </div>
                <div className="cp-kv">
                  <span>自动换币</span>
                  <strong>
                    {preview.estimated_swap_direction
                      ? `${preview.estimated_swap_direction} · ${preview.estimated_swap_amount || '--'}`
                      : '--'}
                  </strong>
                </div>
                <div className="cp-kv">
                  <span>预计流动性</span>
                  <strong>{preview.estimated_liquidity || '--'}</strong>
                </div>
                <div className="cp-kv">
                  <span>Pool 标识</span>
                  <strong>
                    {preview.existing_pool_address
                      ? shortAddress(preview.existing_pool_address, 10, 8)
                      : preview.predicted_pool_id
                        ? shortAddress(preview.predicted_pool_id, 10, 8)
                        : '--'}
                  </strong>
                </div>
              </div>

              {Array.isArray(preview?.warnings) && preview.warnings.length > 0 ? (
                <div className="cp-warnings">
                  {preview.warnings.map((item, index) => (
                    <div key={`${item}-${index}`} className="cp-warning-item">
                      {item}
                    </div>
                  ))}
                </div>
              ) : null}
            </div>
          ) : null}

          {result ? (
            <div className="cp-result-card cp-result-card--success">
              <div className="cp-result-header">
                <span className="cp-result-title">
                  <CheckCircle2 size={15} />
                  执行完成
                </span>
                <span className="cp-result-status ready">{result.status || 'ok'}</span>
              </div>

              <div className="cp-result-grid">
                <div className="cp-kv">
                  <span>Pool</span>
                  <strong>
                    {result.pool_address
                      ? shortAddress(result.pool_address, 10, 8)
                      : result.pool_id
                        ? shortAddress(result.pool_id, 10, 8)
                        : '--'}
                  </strong>
                </div>
                <div className="cp-kv">
                  <span>TokenId</span>
                  <strong>{result.token_id || '--'}</strong>
                </div>
                <div className="cp-kv">
                  <span>Liquidity</span>
                  <strong>{result.liquidity || '--'}</strong>
                </div>
                <div className="cp-kv">
                  <span>主交易</span>
                  <strong>{result.tx_hash ? shortAddress(result.tx_hash, 10, 8) : '--'}</strong>
                </div>
              </div>

              {Array.isArray(result?.warnings) && result.warnings.length > 0 ? (
                <div className="cp-warnings">
                  {result.warnings.map((item, index) => (
                    <div key={`${item}-${index}`} className="cp-warning-item">
                      {item}
                    </div>
                  ))}
                </div>
              ) : null}

              {Array.isArray(result?.explorer_urls) && result.explorer_urls.length > 0 ? (
                <div className="cp-explorer-links">
                  {result.explorer_urls.map((item, index) => (
                    <a
                      key={`${item}-${index}`}
                      href={item}
                      target="_blank"
                      rel="noreferrer"
                      className="cp-explorer-link"
                    >
                      查看交易 #{index + 1}
                    </a>
                  ))}
                </div>
              ) : null}
            </div>
          ) : null}
        </div>
      )}
    </PanelShell>
  );
}
