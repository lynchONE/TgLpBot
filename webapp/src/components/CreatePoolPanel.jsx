import React, { useCallback, useEffect, useMemo, useState } from 'react';
import {
  ArrowRightLeft,
  BrainCircuit,
  CheckCircle2,
  ChevronDown,
  CircleDot,
  Coins,
  Factory,
  Flame,
  Layers,
  RefreshCw,
  Rocket,
  Search,
  Wallet,
  Zap,
} from 'lucide-react';
import {
  executeCreatePool,
  fetchHotPools,
  fetchSmartMoneyOverview,
  fetchWallets,
  previewCreatePool,
} from '../api';
import PanelShell, { EmptyState } from './PanelShell';
import { normalizeHexAddress, shortAddress } from '../utils';

const PROTOCOL_OPTIONS = [
  { key: 'univ3', label: 'Uniswap V3', icon: 'U3' },
  { key: 'univ4', label: 'Uniswap V4', icon: 'U4' },
  { key: 'pcsv3', label: 'Pancake V3', icon: 'P3' },
];

const MODE_OPTIONS = [
  { key: 'create_and_seed', label: '建池+首注', desc: '创建池子并立即注入初始流动性' },
  { key: 'create_only', label: '仅建池', desc: '只初始化池子价格，稍后手动首注' },
];

const SOURCE_TABS = [
  { key: 'manual', label: '手动输入', icon: Search },
  { key: 'hot', label: '热门池子', icon: Flame },
  { key: 'smart', label: '聪明钱', icon: BrainCircuit },
];

const STABLE_SYMBOLS = new Set(['USDT', 'USDC', 'BUSD', 'DAI', 'FDUSD', 'USD']);
const PRICE_RE = /^1\s+([A-Za-z0-9._-]+)\s*=\s*([0-9.]+)\s*(USD|USDT|USDC|BUSD)$/i;

function parsePairSymbols(pair) {
  const parts = String(pair || '')
    .split('/')
    .map((item) => String(item || '').trim())
    .filter(Boolean);
  return [parts[0] || '', parts[1] || ''];
}

function normalizeSymbol(value) {
  return String(value || '').trim().toUpperCase();
}

function isStableSymbol(symbol) {
  return STABLE_SYMBOLS.has(normalizeSymbol(symbol));
}

function feeTierFromPct(value) {
  const n = Number(value ?? 0);
  if (!Number.isFinite(n) || n <= 0) return 0;
  return Math.round(n * 10000);
}

function supportedFeeTiers(protocol) {
  switch (String(protocol || '').trim()) {
    case 'pcsv3':
      return [100, 500, 2500, 10000];
    case 'univ4':
    case 'univ3':
    default:
      return [100, 500, 3000, 10000];
  }
}

function supportsFeeTier(protocol, feeTier) {
  return supportedFeeTiers(protocol).includes(Number(feeTier || 0));
}

function defaultFeeTier(protocol) {
  return supportedFeeTiers(protocol)[1] || supportedFeeTiers(protocol)[0] || 500;
}

function formatFeeTier(feeTier) {
  const n = Number(feeTier || 0);
  if (!Number.isFinite(n) || n <= 0) return '--';
  return `${(n / 10000).toFixed(n >= 10000 ? 2 : 2).replace(/\.?0+$/, '')}%`;
}

function buildSourcePricePrefill(token0Symbol, token1Symbol, priceDisplay) {
  const match = String(priceDisplay || '').trim().match(PRICE_RE);
  if (!match) return '';
  const quotedSymbol = normalizeSymbol(match[1]);
  const priceValue = Number(match[2]);
  if (!Number.isFinite(priceValue) || priceValue <= 0) return '';

  const sym0 = normalizeSymbol(token0Symbol);
  const sym1 = normalizeSymbol(token1Symbol);
  const token0Stable = isStableSymbol(sym0);
  const token1Stable = isStableSymbol(sym1);

  if (quotedSymbol === sym0 && token1Stable) return String(priceValue);
  if (quotedSymbol === sym1 && token0Stable) return String(1 / priceValue);
  return '';
}

function buildHotSources(rows) {
  return (Array.isArray(rows) ? rows : [])
    .map((row, index) => {
      const token0Address = normalizeHexAddress(row?.token0_address);
      const token1Address = normalizeHexAddress(row?.token1_address);
      if (!token0Address || !token1Address) return null;
      const [pair0, pair1] = parsePairSymbols(row?.trading_pair);
      const token0Symbol = String(row?.token0_symbol || pair0 || '').trim();
      const token1Symbol = String(row?.token1_symbol || pair1 || '').trim();
      const feeTier = feeTierFromPct(row?.fee_percentage);
      return {
        key: `hot:${normalizeHexAddress(row?.pool_address) || index}`,
        label: String(row?.trading_pair || `${token0Symbol}/${token1Symbol}` || '--').trim(),
        subtitle: String(row?.factory_name || row?.dex || '热门池子').trim(),
        token0Address,
        token1Address,
        token0Symbol,
        token1Symbol,
        feeTier,
        priceHint: String(row?.price_display || '').trim(),
        pricePrefill: buildSourcePricePrefill(token0Symbol, token1Symbol, row?.price_display),
      };
    })
    .filter(Boolean)
    .slice(0, 10);
}

function buildSmartSources(rows) {
  return (Array.isArray(rows) ? rows : [])
    .map((row, index) => {
      const token0Address = normalizeHexAddress(row?.token0);
      const token1Address = normalizeHexAddress(row?.token1);
      if (!token0Address || !token1Address) return null;
      const [pair0, pair1] = parsePairSymbols(row?.pair);
      return {
        key: `smart:${normalizeHexAddress(row?.pool_id) || index}`,
        label: String(row?.pair || '--').trim(),
        subtitle: `${String(row?.exchange || 'Smart Money').trim()} ${String(row?.pool_version || '').trim().toUpperCase()}`.trim(),
        token0Address,
        token1Address,
        token0Symbol: String(row?.token0_symbol || pair0 || '').trim(),
        token1Symbol: String(row?.token1_symbol || pair1 || '').trim(),
        feeTier: feeTierFromPct(row?.fee_pct),
        priceHint: '可作为币对与费率参考来源',
        pricePrefill: '',
      };
    })
    .filter(Boolean)
    .slice(0, 10);
}

function fieldErrorText(resp) {
  if (!resp) return '';
  if (Array.isArray(resp?.warnings) && resp.warnings.length > 0) return String(resp.warnings[0]);
  return '';
}

/* ---------- Section wrapper with step indicator ---------- */
function StepSection({ step, title, subtitle, children, accent }) {
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
  const [smartSources, setSmartSources] = useState([]);
  const [sourcesLoading, setSourcesLoading] = useState(false);
  const [sourcesError, setSourcesError] = useState('');

  const [protocol, setProtocol] = useState('univ3');
  const [mode, setMode] = useState('create_and_seed');
  const [walletId, setWalletId] = useState(0);
  const [sourceTab, setSourceTab] = useState('manual');
  const [selectedSourceKey, setSelectedSourceKey] = useState('');
  const [tokenAAddress, setTokenAAddress] = useState('');
  const [tokenBAddress, setTokenBAddress] = useState('');
  const [feeTier, setFeeTier] = useState(defaultFeeTier('univ3'));
  const [initialPrice, setInitialPrice] = useState('');
  const [amountA, setAmountA] = useState('');
  const [amountB, setAmountB] = useState('');
  const [preview, setPreview] = useState(null);
  const [previewLoading, setPreviewLoading] = useState(false);
  const [executeLoading, setExecuteLoading] = useState(false);
  const [error, setError] = useState('');
  const [result, setResult] = useState(null);

  const sourceOptions = useMemo(() => ({
    hot: hotSources,
    smart: smartSources,
  }), [hotSources, smartSources]);

  const selectedSource = useMemo(() => {
    const list = sourceOptions[sourceTab] || [];
    return list.find((item) => item.key === selectedSourceKey) || null;
  }, [selectedSourceKey, sourceOptions, sourceTab]);

  const protocolFees = useMemo(() => supportedFeeTiers(protocol), [protocol]);
  const selectedWalletId = useMemo(() => {
    const numeric = Number(walletId || 0);
    if (Number.isFinite(numeric) && numeric > 0 && wallets.some((item) => Number(item?.id || 0) === numeric)) {
      return numeric;
    }
    const def = wallets.find((item) => item?.is_default);
    return Number(def?.id || wallets?.[0]?.id || 0);
  }, [walletId, wallets]);

  useEffect(() => {
    if (supportsFeeTier(protocol, feeTier)) return;
    setFeeTier(defaultFeeTier(protocol));
  }, [feeTier, protocol]);

  useEffect(() => {
    if (!hasInitData) {
      setWallets([]);
      setHotSources([]);
      setSmartSources([]);
      return undefined;
    }

    const controller = new AbortController();
    setWalletsLoading(true);
    setWalletsError('');
    fetchWallets({ apiBaseUrl, initData, chain: 'bsc', signal: controller.signal })
      .then((resp) => {
        const nextWallets = Array.isArray(resp?.wallets) ? resp.wallets : [];
        setWallets(nextWallets);
        const def = nextWallets.find((item) => item?.is_default);
        setWalletId((prev) => (Number(prev || 0) > 0 ? prev : Number(def?.id || nextWallets?.[0]?.id || 0)));
      })
      .catch((err) => {
        if (err?.name !== 'AbortError') {
          setWalletsError(String(err?.message || err));
          setWallets([]);
        }
      })
      .finally(() => setWalletsLoading(false));

    return () => controller.abort();
  }, [apiBaseUrl, hasInitData, initData]);

  const reloadSources = useCallback(() => {
    if (!hasInitData) return;
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
      fetchSmartMoneyOverview({
        apiBaseUrl,
        initData,
        chain: 'bsc',
        poolLimit: 16,
        walletLimit: 8,
        poolsWindowHours: 2,
        pnlWindowHours: 2,
        signal: controller.signal,
      }),
    ])
      .then(([hotResp, smartResp]) => {
        setHotSources(buildHotSources(hotResp?.data));
        setSmartSources(buildSmartSources(smartResp?.pools));
      })
      .catch((err) => {
        if (err?.name !== 'AbortError') setSourcesError(String(err?.message || err));
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

  const applySource = useCallback((source, tabKey) => {
    if (!source) return;
    setSourceTab(tabKey);
    setSelectedSourceKey(source.key);
    setTokenAAddress(source.token0Address || '');
    setTokenBAddress(source.token1Address || '');
    if (supportsFeeTier(protocol, source.feeTier)) {
      setFeeTier(source.feeTier);
    }
    setInitialPrice(source.pricePrefill || '');
    setPreview(null);
    setResult(null);
    setError('');
  }, [protocol]);

  const createPayload = useCallback(() => ({
    apiBaseUrl,
    initData,
    chain: 'bsc',
    protocol,
    walletId: selectedWalletId || undefined,
    tokenAAddress,
    tokenBAddress,
    feeTier: Number(feeTier || 0),
    initialPrice,
    mode,
    rangeMode: 'full_range',
    amountA,
    amountB,
  }), [amountA, amountB, apiBaseUrl, feeTier, initData, initialPrice, mode, protocol, selectedWalletId, tokenAAddress, tokenBAddress]);

  const handlePreview = useCallback(async () => {
    if (!hasInitData) {
      setError('请先完成 Telegram 登录。');
      return;
    }
    setPreviewLoading(true);
    setError('');
    setResult(null);
    try {
      const resp = await previewCreatePool(createPayload());
      setPreview(resp);
      if (!String(initialPrice || '').trim() && String(resp?.suggested_initial_price || '').trim()) {
        setInitialPrice(String(resp.suggested_initial_price).trim());
      }
    } catch (err) {
      setError(String(err?.message || err));
      setPreview(null);
    } finally {
      setPreviewLoading(false);
    }
  }, [createPayload, hasInitData, initialPrice]);

  const handleExecute = useCallback(async () => {
    if (!hasInitData) {
      setError('请先完成 Telegram 登录。');
      return;
    }
    setExecuteLoading(true);
    setError('');
    setResult(null);
    try {
      const resp = await executeCreatePool(createPayload());
      setResult(resp);
      setPreview(null);
    } catch (err) {
      setError(String(err?.message || err));
    } finally {
      setExecuteLoading(false);
    }
  }, [createPayload, hasInitData]);

  const activeSources = sourceOptions[sourceTab] || [];
  const selectedWallet = wallets.find((w) => Number(w?.id || 0) === selectedWalletId);

  return (
    <PanelShell
      title="创建池子"
      subtitle="BSC · Uniswap V3 / V4 / Pancake V3"
      icon={Factory}
      actions={(
        <button
          type="button"
          className="ghost-chip"
          onClick={reloadSources}
          disabled={!hasInitData || sourcesLoading}
        >
          <RefreshCw size={13} className={sourcesLoading ? 'spin' : ''} />
          刷新来源
        </button>
      )}
    >
      {!hasInitData ? (
        <EmptyState text="请先完成 Telegram 登录后再创建池子。" />
      ) : (
        <div className="cp-root">
          {/* ===== Step 1: Protocol & Mode ===== */}
          <StepSection step="1" title="选择协议与模式">
            <div className="cp-segmented-row">
              <div className="cp-segmented">
                {PROTOCOL_OPTIONS.map((item) => (
                  <button
                    key={item.key}
                    type="button"
                    className={`cp-seg-btn${protocol === item.key ? ' active' : ''}`}
                    onClick={() => setProtocol(item.key)}
                  >
                    <span className="cp-seg-icon">{item.icon}</span>
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
                  onClick={() => setMode(item.key)}
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
          </StepSection>

          {/* ===== Step 2: Source ===== */}
          <StepSection step="2" title="数据来源" subtitle="选择热门池子或聪明钱快速填充参数">
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
                <span>手动模式 - 直接在下方输入币对地址</span>
              </div>
            ) : sourcesLoading && activeSources.length === 0 ? (
              <div className="cp-source-empty">
                <RefreshCw size={16} className="spin" />
                <span>正在加载 BSC 来源池子...</span>
              </div>
            ) : activeSources.length === 0 ? (
              <div className="cp-source-empty">
                <span>{sourcesError || '暂无可用来源数据'}</span>
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
                      <span className="cp-source-dex">{source.subtitle}</span>
                    </div>
                    {selectedSourceKey === source.key && (
                      <span className="cp-source-check"><CheckCircle2 size={14} /></span>
                    )}
                  </button>
                ))}
              </div>
            )}

            {selectedSource ? (
              <div className="cp-source-hint-bar">
                <Zap size={13} />
                <span>
                  已选: <strong>{selectedSource.label}</strong> · {selectedSource.subtitle}
                  {selectedSource.priceHint ? ` · ${selectedSource.priceHint}` : ''}
                </span>
              </div>
            ) : null}
          </StepSection>

          {/* ===== Step 3: Parameters ===== */}
          <StepSection step="3" title="配置参数">
            <div className="cp-form-grid">
              {/* Wallet */}
              <div className="cp-field">
                <label className="cp-field-label">
                  <Wallet size={13} />
                  <span>执行钱包</span>
                </label>
                <div className="cp-select-wrap">
                  <select value={selectedWalletId || ''} onChange={(e) => setWalletId(Number(e.target.value || 0))}>
                    {wallets.map((item) => (
                      <option key={item.id} value={item.id}>
                        {(item.name || shortAddress(item.address, 6, 4))} · {shortAddress(item.address, 6, 4)}
                      </option>
                    ))}
                  </select>
                  <ChevronDown size={14} className="cp-select-arrow" />
                </div>
                {walletsLoading ? <small className="cp-field-hint">正在读取 BSC 钱包...</small> : null}
                {walletsError ? <small className="cp-field-error">{walletsError}</small> : null}
              </div>

              {/* Fee tier */}
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
                      className={`cp-fee-chip${feeTier === item ? ' active' : ''}`}
                      onClick={() => setFeeTier(item)}
                    >
                      {formatFeeTier(item)}
                    </button>
                  ))}
                </div>
              </div>

              {/* Token A */}
              <div className="cp-field">
                <label className="cp-field-label">
                  <Coins size={13} />
                  <span>Token A 地址</span>
                </label>
                <input
                  type="text"
                  className="cp-input"
                  value={tokenAAddress}
                  onChange={(e) => setTokenAAddress(e.target.value)}
                  placeholder="0x..."
                />
              </div>

              {/* Token B */}
              <div className="cp-field">
                <label className="cp-field-label">
                  <ArrowRightLeft size={13} />
                  <span>Token B 地址</span>
                </label>
                <input
                  type="text"
                  className="cp-input"
                  value={tokenBAddress}
                  onChange={(e) => setTokenBAddress(e.target.value)}
                  placeholder="0x..."
                />
              </div>

              {/* Initial Price */}
              <div className="cp-field cp-field--full">
                <label className="cp-field-label">
                  <span>初始价格</span>
                </label>
                <input
                  type="text"
                  className="cp-input"
                  value={initialPrice}
                  onChange={(e) => setInitialPrice(e.target.value)}
                  placeholder="1 TokenA = X TokenB (留空则自动推导)"
                />
                <small className="cp-field-hint">如果留空，后端会先尝试用两边 token 的 USD 价格自动推导</small>
              </div>

              {/* Amounts (only in seed mode) */}
              {mode === 'create_and_seed' ? (
                <>
                  <div className="cp-field">
                    <label className="cp-field-label"><span>Token A 数量</span></label>
                    <input
                      type="text"
                      className="cp-input"
                      value={amountA}
                      onChange={(e) => setAmountA(e.target.value)}
                      placeholder="例如 1000"
                    />
                  </div>
                  <div className="cp-field">
                    <label className="cp-field-label"><span>Token B 数量</span></label>
                    <input
                      type="text"
                      className="cp-input"
                      value={amountB}
                      onChange={(e) => setAmountB(e.target.value)}
                      placeholder="例如 0.5"
                    />
                  </div>
                </>
              ) : (
                <div className="cp-field cp-field--full">
                  <div className="cp-mode-notice">
                    <Zap size={14} />
                    <span>当前模式为仅建池，只会初始化池子价格，不会立刻打首仓</span>
                  </div>
                </div>
              )}
            </div>
          </StepSection>

          {/* ===== Summary strip ===== */}
          <div className="cp-summary-strip">
            <div className="cp-summary-item">
              <span>范围模式</span>
              <strong>Full Range</strong>
            </div>
            <div className="cp-summary-divider" />
            <div className="cp-summary-item">
              <span>目标链</span>
              <strong>BSC</strong>
            </div>
            <div className="cp-summary-divider" />
            <div className="cp-summary-item">
              <span>钱包</span>
              <strong>{selectedWallet ? (selectedWallet.name || shortAddress(selectedWallet.address, 4, 4)) : '--'}</strong>
            </div>
            <div className="cp-summary-divider" />
            <div className="cp-summary-item">
              <span>协议</span>
              <strong>{PROTOCOL_OPTIONS.find((p) => p.key === protocol)?.label || '--'}</strong>
            </div>
          </div>

          {/* ===== Error ===== */}
          {error ? (
            <div className="cp-error-bar">
              <span>{error}</span>
            </div>
          ) : null}

          {/* ===== Actions ===== */}
          <div className="cp-actions">
            <button
              type="button"
              className="cp-btn cp-btn--ghost"
              onClick={handlePreview}
              disabled={previewLoading || executeLoading}
            >
              {previewLoading ? <RefreshCw size={14} className="spin" /> : <Search size={14} />}
              {previewLoading ? '预览中...' : '预览参数'}
            </button>
            <button
              type="button"
              className="cp-btn cp-btn--primary"
              onClick={handleExecute}
              disabled={executeLoading || previewLoading}
            >
              {executeLoading ? <RefreshCw size={14} className="spin" /> : <Rocket size={14} />}
              {executeLoading ? '执行中...' : mode === 'create_only' ? '创建池子' : '一键创建并首注'}
            </button>
          </div>

          {/* ===== Preview ===== */}
          {preview ? (
            <div className={`cp-result-card${preview.ready_to_execute ? ' cp-result-card--ready' : ''}`}>
              <div className="cp-result-header">
                <span className="cp-result-title">预览结果</span>
                <span className={`cp-result-status${preview.ready_to_execute ? ' ready' : ''}`}>
                  {preview.pool_exists
                    ? '目标协议下已存在同币对同费率池子'
                    : preview.ready_to_execute
                      ? '参数已满足执行条件'
                      : '仍有参数需要补齐'}
                </span>
              </div>
              <div className="cp-result-grid">
                <div className="cp-kv"><span>Token 排序</span><strong>{preview.token0?.symbol || '--'} / {preview.token1?.symbol || '--'}</strong></div>
                <div className="cp-kv"><span>初始价格</span><strong>{preview.initial_price || '--'}</strong></div>
                <div className="cp-kv"><span>Tick 区间</span><strong>{preview.tick_lower} / {preview.tick_upper}</strong></div>
                <div className="cp-kv"><span>TickSpacing</span><strong>{preview.tick_spacing || '--'}</strong></div>
                <div className="cp-kv"><span>预测 PoolId</span><strong>{preview.predicted_pool_id ? shortAddress(preview.predicted_pool_id, 10, 8) : '--'}</strong></div>
                <div className="cp-kv"><span>估算流动性</span><strong>{preview.estimated_liquidity || '--'}</strong></div>
              </div>
              {Array.isArray(preview?.warnings) && preview.warnings.length > 0 ? (
                <div className="cp-warnings">
                  {preview.warnings.map((item, index) => (
                    <div key={`${item}-${index}`} className="cp-warning-item">{item}</div>
                  ))}
                </div>
              ) : null}
            </div>
          ) : null}

          {/* ===== Result ===== */}
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
                <div className="cp-kv"><span>Pool</span><strong>{result.pool_address ? shortAddress(result.pool_address, 10, 8) : shortAddress(result.pool_id, 10, 8)}</strong></div>
                <div className="cp-kv"><span>TokenId</span><strong>{result.token_id || '--'}</strong></div>
                <div className="cp-kv"><span>Liquidity</span><strong>{result.liquidity || '--'}</strong></div>
                <div className="cp-kv"><span>主交易</span><strong>{shortAddress(result.tx_hash, 10, 8)}</strong></div>
              </div>
              {Array.isArray(result?.explorer_urls) && result.explorer_urls.length > 0 ? (
                <div className="cp-explorer-links">
                  {result.explorer_urls.map((item, index) => (
                    <a key={`${item}-${index}`} href={item} target="_blank" rel="noreferrer" className="cp-explorer-link">
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
