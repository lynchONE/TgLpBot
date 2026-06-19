import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { AlertTriangle, Check, Eye, EyeOff, X } from 'lucide-react';
import { fetchGlobalConfig, fetchPoolLiquidityDistribution, prepareOpenPosition, previewOpenPosition } from '../api';
import LiquidityDistributionChart from './LiquidityDistributionChart.jsx';
import { TASK_MODE_OPTIONS, getOutOfRangeActionSummary as getTaskModeActionSummary } from '../taskModes';
import {
  normalizeTokenRisk,
  shortAddress as shortAddressShared,
  tokenRiskLabel,
  tokenRiskSummary,
  tokenRiskToneClass,
} from '../utils';

const PRESET_RANGES = [1, 2, 3, 5, 10, 20];
const OPEN_POSITION_AMOUNT_PRESETS = [200, 500, 1000, 1500, 2000];
const RANGE_INPUT_OPTIONS = [
  { key: 'percentage', label: '快捷%' },
  { key: 'grid', label: 'Tick格子' },
  { key: 'tick', label: '直接Tick' },
];
const GRID_RADIUS = 8;
const DEFAULT_GRID_OFFSET = 3;

function shortAddr(addr) {
  const value = String(addr || '').trim();
  if (value.length <= 10) return value || '--';
  return `${value.slice(0, 6)}..${value.slice(-4)}`;
}

function formatUsdCompact(value) {
  const num = Number(value);
  if (!Number.isFinite(num) || num <= 0) return '--';
  const abs = Math.abs(num);
  if (abs >= 1000000) return `$${(num / 1000000).toFixed(abs >= 10000000 ? 0 : 1).replace(/\.0$/, '')}M`;
  if (abs >= 1000) return `$${(num / 1000).toFixed(abs >= 10000 ? 0 : 1).replace(/\.0$/, '')}K`;
  if (abs >= 100) return `$${num.toFixed(0)}`;
  if (abs >= 10) return `$${num.toFixed(1).replace(/\.0$/, '')}`;
  return `$${num.toFixed(2).replace(/0+$/, '').replace(/\.$/, '')}`;
}

function parseOptionalPercent(raw) {
  const text = String(raw || '').trim();
  if (!text) return { valid: true, value: undefined };
  const num = Number(text);
  if (!Number.isFinite(num) || num < 0 || num > 100) {
    return { valid: false, value: undefined };
  }
  return { valid: true, value: num };
}

function formatPercent(value) {
  const num = Number(value);
  if (!Number.isFinite(num)) return '--';
  return `${num.toFixed(num >= 1 ? 2 : 3).replace(/0+$/, '').replace(/\.$/, '')}%`;
}

function formatUSDTValue(value) {
  const num = Number(value);
  if (!Number.isFinite(num) || num < 0) return '--';
  if (num === 0) return '0';
  if (num >= 1000) return num.toLocaleString(undefined, { maximumFractionDigits: 2 });
  if (num >= 1) return num.toFixed(2).replace(/0+$/, '').replace(/\.$/, '');
  return num.toFixed(4).replace(/0+$/, '').replace(/\.$/, '');
}

function formatAmountPresetValue(value) {
  const num = Number(value);
  if (!Number.isFinite(num) || num <= 0) return '';
  if (Number.isInteger(num)) return String(num);
  return num.toFixed(num >= 1 ? 2 : 4).replace(/0+$/, '').replace(/\.$/, '');
}

function resolveOpenPositionErrorPayload(error) {
  if (!error || typeof error !== 'object') return null;
  if (error.payload && typeof error.payload === 'object') return error.payload;
  return error;
}

function isOpenPositionSafetyError(error) {
  const payload = resolveOpenPositionErrorPayload(error);
  if (!payload) return false;
  const code = String(payload?.code || '').trim();
  return Boolean(
    code === 'zap_safety_check_failed' ||
    code.startsWith('pool_') ||
    typeof payload?.liquidity_usd === 'number' ||
    typeof payload?.max_open_amount === 'number' ||
    typeof payload?.price_deviation_percent === 'number' ||
    Boolean(payload?.token_risk) ||
    code === 'token_honeypot' ||
    Boolean(payload?.risk_ack_required)
  );
}

function extractOpenPositionErrorChecks(error, fallbackKey = 'preview_safety') {
  const payload = resolveOpenPositionErrorPayload(error);
  if (Array.isArray(payload?.checks) && payload.checks.length > 0) {
    return payload.checks;
  }
  if (!isOpenPositionSafetyError(payload)) {
    return [];
  }
  const detail = String(error?.message || payload?.message || '').trim() || '安全检查未通过';
  return [{
    key: fallbackKey,
    status: 'fail',
    label: '安全检查',
    detail,
  }];
}

function parseDCAPercentagesAny(raw) {
  if (Array.isArray(raw)) return raw.map((v) => Number(v) || 0);
  if (typeof raw === 'string' && raw.trim()) {
    const arr = JSON.parse(raw);
    if (Array.isArray(arr)) return arr.map((v) => Number(v) || 0);
  }
  return [50, 50];
}

function formatPriceValue(value) {
  const num = Number(value);
  if (!Number.isFinite(num) || num <= 0) return '--';
  if (num >= 1000) return num.toLocaleString(undefined, { maximumFractionDigits: 2 });
  if (num >= 1) return num.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 6 });
  return Number(num.toPrecision(6)).toString();
}

function formatPriceInputValue(value) {
  const text = formatPriceValue(value);
  return text === '--' ? '' : text;
}

function tickToPoolPrice(tick, token0Decimals, token1Decimals) {
  const tickValue = Number(tick);
  if (!Number.isFinite(tickValue)) return NaN;
  const decAdj = Math.pow(10, (Number(token0Decimals) || 18) - (Number(token1Decimals) || 18));
  return Math.pow(1.0001, tickValue) * decAdj;
}

function poolPriceToTick(price, token0Decimals, token1Decimals) {
  const priceValue = Number(price);
  if (!Number.isFinite(priceValue) || priceValue <= 0) return NaN;
  const decAdj = Math.pow(10, (Number(token0Decimals) || 18) - (Number(token1Decimals) || 18));
  const ratio = priceValue / decAdj;
  if (!Number.isFinite(ratio) || ratio <= 0) return NaN;
  return Math.log(ratio) / Math.log(1.0001);
}

function normalizeDisplayPriceTickRange(
  lowerRaw,
  upperRaw,
  invertPrice,
  token0Decimals,
  token1Decimals,
  tickSpacing,
  minTick,
  maxTick,
) {
  const lowerDisplay = Number(String(lowerRaw || '').trim());
  const upperDisplay = Number(String(upperRaw || '').trim());
  if (!Number.isFinite(lowerDisplay) || !Number.isFinite(upperDisplay) || lowerDisplay <= 0 || upperDisplay <= 0) {
    return null;
  }
  const firstPoolPrice = invertPrice ? 1 / lowerDisplay : lowerDisplay;
  const secondPoolPrice = invertPrice ? 1 / upperDisplay : upperDisplay;
  const firstTick = poolPriceToTick(firstPoolPrice, token0Decimals, token1Decimals);
  const secondTick = poolPriceToTick(secondPoolPrice, token0Decimals, token1Decimals);
  if (!Number.isFinite(firstTick) || !Number.isFinite(secondTick)) return null;
  const spacing = Number(tickSpacing);
  const resolvedMinTick = Number(minTick);
  const resolvedMaxTick = Number(maxTick);
  let lowerTick = Number.isFinite(spacing) && spacing > 0
    ? roundDownToTickSpacing(Math.min(firstTick, secondTick), spacing)
    : Math.floor(Math.min(firstTick, secondTick));
  let upperTick = Number.isFinite(spacing) && spacing > 0
    ? roundUpToTickSpacing(Math.max(firstTick, secondTick), spacing)
    : Math.ceil(Math.max(firstTick, secondTick));
  if (Number.isFinite(resolvedMinTick)) lowerTick = Math.max(lowerTick, resolvedMinTick);
  if (Number.isFinite(resolvedMaxTick)) upperTick = Math.min(upperTick, resolvedMaxTick);
  if (Number.isFinite(spacing) && spacing > 0 && upperTick <= lowerTick) {
    if (Number.isFinite(resolvedMaxTick) && lowerTick + spacing > resolvedMaxTick) {
      lowerTick = upperTick - spacing;
    } else {
      upperTick = lowerTick + spacing;
    }
  }
  if (!Number.isFinite(lowerTick) || !Number.isFinite(upperTick) || upperTick <= lowerTick) return null;
  return { lowerTick: Math.trunc(lowerTick), upperTick: Math.trunc(upperTick) };
}

function buildDisplayPriceRangeFromTicks(lowerTick, upperTick, invertPrice, token0Decimals, token1Decimals) {
  if (!Number.isInteger(lowerTick) || !Number.isInteger(upperTick) || upperTick <= lowerTick) return null;
  const firstPrice = tickToPoolPrice(lowerTick, token0Decimals, token1Decimals);
  const secondPrice = tickToPoolPrice(upperTick, token0Decimals, token1Decimals);
  if (!Number.isFinite(firstPrice) || !Number.isFinite(secondPrice) || firstPrice <= 0 || secondPrice <= 0) return null;
  const firstDisplay = invertPrice ? 1 / firstPrice : firstPrice;
  const secondDisplay = invertPrice ? 1 / secondPrice : secondPrice;
  if (!Number.isFinite(firstDisplay) || !Number.isFinite(secondDisplay) || firstDisplay <= 0 || secondDisplay <= 0) {
    return null;
  }
  return {
    lowerPrice: Math.min(firstDisplay, secondDisplay),
    upperPrice: Math.max(firstDisplay, secondDisplay),
  };
}

function estimateDisplayGridStepPercent(currentTick, tickSpacing, invertPrice, token0Decimals, token1Decimals) {
  const baseTick = Number(currentTick);
  const spacing = Number(tickSpacing);
  if (!Number.isFinite(baseTick) || !Number.isFinite(spacing) || spacing <= 0) return null;
  const currentPoolPrice = tickToPoolPrice(baseTick, token0Decimals, token1Decimals);
  const nextPoolPrice = tickToPoolPrice(baseTick + spacing, token0Decimals, token1Decimals);
  if (!Number.isFinite(currentPoolPrice) || currentPoolPrice <= 0 || !Number.isFinite(nextPoolPrice) || nextPoolPrice <= 0) {
    return null;
  }
  const currentDisplay = invertPrice ? 1 / currentPoolPrice : currentPoolPrice;
  const nextDisplay = invertPrice ? 1 / nextPoolPrice : nextPoolPrice;
  if (!Number.isFinite(currentDisplay) || currentDisplay <= 0 || !Number.isFinite(nextDisplay) || nextDisplay <= 0) {
    return null;
  }
  return Math.abs(((nextDisplay / currentDisplay) - 1) * 100);
}

function nudgeDisplayPriceBoundary(target, delta, invertPrice, tickSpacing, lowerTick, upperTick, minTick, maxTick) {
  const spacing = Number(tickSpacing);
  let nextLower = Number(lowerTick);
  let nextUpper = Number(upperTick);
  if (!Number.isFinite(spacing) || spacing <= 0) return null;
  if (!Number.isInteger(nextLower) || !Number.isInteger(nextUpper) || nextUpper <= nextLower) return null;

  const changedRawBoundary = target === 'lower'
    ? (invertPrice ? 'upper' : 'lower')
    : (invertPrice ? 'lower' : 'upper');

  if (target === 'lower') {
    if (invertPrice) nextUpper += delta * spacing;
    else nextLower -= delta * spacing;
  } else if (invertPrice) {
    nextLower -= delta * spacing;
  } else {
    nextUpper += delta * spacing;
  }

  const resolvedMinTick = Number(minTick);
  const resolvedMaxTick = Number(maxTick);
  if (Number.isFinite(resolvedMinTick)) nextLower = Math.max(nextLower, resolvedMinTick);
  if (Number.isFinite(resolvedMaxTick)) nextUpper = Math.min(nextUpper, resolvedMaxTick);

  if (changedRawBoundary === 'lower') {
    if (Number.isFinite(resolvedMaxTick) && nextLower > resolvedMaxTick - spacing) {
      nextLower = resolvedMaxTick - spacing;
    }
    if (nextLower >= nextUpper) nextUpper = nextLower + spacing;
    if (Number.isFinite(resolvedMaxTick) && nextUpper > resolvedMaxTick) {
      nextUpper = resolvedMaxTick;
      nextLower = nextUpper - spacing;
    }
  } else {
    if (Number.isFinite(resolvedMinTick) && nextUpper < resolvedMinTick + spacing) {
      nextUpper = resolvedMinTick + spacing;
    }
    if (nextUpper <= nextLower) nextLower = nextUpper - spacing;
    if (Number.isFinite(resolvedMinTick) && nextLower < resolvedMinTick) {
      nextLower = resolvedMinTick;
      nextUpper = nextLower + spacing;
    }
  }

  if (!Number.isInteger(nextLower) || !Number.isInteger(nextUpper) || nextUpper <= nextLower) return null;
  return { lowerTick: nextLower, upperTick: nextUpper };
}

function formatRangePercentCompact(value) {
  const num = Number(value);
  if (!Number.isFinite(num) || num <= 0) return '--';
  if (num >= 100) return `${Math.round(num)}%`;
  if (num >= 10) return `${num.toFixed(1).replace(/\.0$/, '')}%`;
  return `${num.toFixed(2).replace(/0+$/, '').replace(/\.$/, '')}%`;
}

function formatSignedPercentCompact(value) {
  const num = Number(value);
  if (!Number.isFinite(num)) return '--';
  if (Math.abs(num) < 0.0001) return '0%';
  return `${num > 0 ? '+' : '-'}${formatRangePercentCompact(Math.abs(num))}`;
}

function formatPercentInputValue(value) {
  const num = Number(value);
  if (!Number.isFinite(num) || num <= 0) return '';
  if (num >= 10) return num.toFixed(1).replace(/\.0$/, '');
  if (num >= 1) return num.toFixed(2).replace(/0+$/, '').replace(/\.$/, '');
  return num.toFixed(3).replace(/0+$/, '').replace(/\.$/, '');
}

function formatDCAIntervalHint(seconds) {
  const n = Number(seconds);
  if (!Number.isFinite(n) || n <= 0) return '立即';
  if (n < 1) return `${Math.round(n * 1000)}ms`;
  if (Number.isInteger(n)) return `${n}s`;
  return `${n.toFixed(1).replace(/\.0$/, '')}s`;
}

function estimateBinLiquidityUsd(bin, currentTick, token0Decimals, token1Decimals, quoteIsToken1) {
  if (!bin || typeof quoteIsToken1 !== 'boolean') return NaN;
  const liquidity = Number(bin?.liquidity);
  if (!Number.isFinite(liquidity) || liquidity === 0) return 0;
  const tickLower = Number(bin?.tick_lower);
  const tickUpper = Number(bin?.tick_upper);
  if (!Number.isFinite(tickLower) || !Number.isFinite(tickUpper) || tickUpper <= tickLower) return NaN;

  const tickMid = (tickLower + tickUpper) / 2;
  const sqrtLower = Math.pow(1.0001, tickLower / 2);
  const sqrtUpper = Math.pow(1.0001, tickUpper / 2);
  const sqrtCurrent = Number.isFinite(currentTick) ? Math.pow(1.0001, currentTick / 2) : sqrtLower;

  let amount0Raw = 0;
  let amount1Raw = 0;
  if (Number.isFinite(currentTick) && tickUpper <= currentTick) {
    amount1Raw = liquidity * (sqrtUpper - sqrtLower);
  } else if (Number.isFinite(currentTick) && tickLower >= currentTick) {
    amount0Raw = liquidity * (1 / sqrtLower - 1 / sqrtUpper);
  } else {
    amount0Raw = liquidity * (1 / sqrtCurrent - 1 / sqrtUpper);
    amount1Raw = liquidity * (sqrtCurrent - sqrtLower);
  }

  const amount0 = amount0Raw / Math.pow(10, token0Decimals);
  const amount1 = amount1Raw / Math.pow(10, token1Decimals);
  const priceToken0InToken1 = Math.pow(1.0001, tickMid) * Math.pow(10, token0Decimals - token1Decimals);
  return quoteIsToken1 ? amount0 * priceToken0InToken1 + amount1 : amount0 + amount1 / priceToken0InToken1;
}

function roundDownToTickSpacing(tick, tickSpacing) {
  const spacing = Number(tickSpacing);
  const value = Number(tick);
  if (!Number.isFinite(spacing) || spacing <= 0 || !Number.isFinite(value)) return 0;
  const remainder = value % spacing;
  if (remainder === 0) return value;
  return value < 0 ? value - remainder - spacing : value - remainder;
}

function roundUpToTickSpacing(tick, tickSpacing) {
  const spacing = Number(tickSpacing);
  const value = Number(tick);
  if (!Number.isFinite(spacing) || spacing <= 0 || !Number.isFinite(value)) return 0;
  const down = roundDownToTickSpacing(value, spacing);
  return down === value ? value : down + spacing;
}

function buildGridBins(editor, radius = GRID_RADIUS) {
  const currentTick = Number(editor?.current_tick);
  const tickSpacing = Number(editor?.tick_spacing);
  if (!Number.isFinite(currentTick) || !Number.isFinite(tickSpacing) || tickSpacing <= 0) return [];
  const anchorLower = Number.isFinite(Number(editor?.anchor_tick_lower))
    ? Number(editor.anchor_tick_lower)
    : roundDownToTickSpacing(currentTick, tickSpacing);
  const anchorUpper = Number.isFinite(Number(editor?.anchor_tick_upper))
    ? Number(editor.anchor_tick_upper)
    : anchorLower + tickSpacing;
  const bins = [];
  for (let idx = -radius; idx <= radius; idx += 1) {
    let lowerTick;
    let upperTick;
    if (idx === 0) {
      lowerTick = anchorLower;
      upperTick = anchorUpper;
    } else if (idx > 0) {
      lowerTick = anchorUpper + (idx - 1) * tickSpacing;
      upperTick = lowerTick + tickSpacing;
    } else {
      upperTick = anchorLower + (idx + 1) * tickSpacing;
      lowerTick = upperTick - tickSpacing;
    }
    bins.push({
      key: `grid-${idx}`,
      index: idx,
      lowerTick,
      upperTick,
      isCurrent: idx === 0,
    });
  }
  return bins;
}

function buildDefaultFocusedTickRange(editor, gridOffset = DEFAULT_GRID_OFFSET) {
  const currentTick = Number(editor?.current_tick);
  const tickSpacing = Number(editor?.tick_spacing);
  if (!Number.isFinite(currentTick) || !Number.isFinite(tickSpacing) || tickSpacing <= 0) return null;
  const offset = Math.max(1, Number(gridOffset) || DEFAULT_GRID_OFFSET);
  const anchorLower = Number.isFinite(Number(editor?.anchor_tick_lower))
    ? Number(editor.anchor_tick_lower)
    : roundDownToTickSpacing(currentTick, tickSpacing);
  const anchorUpper = Number.isFinite(Number(editor?.anchor_tick_upper))
    ? Number(editor.anchor_tick_upper)
    : anchorLower + tickSpacing;
  if (!Number.isInteger(anchorLower) || !Number.isInteger(anchorUpper) || anchorUpper <= anchorLower) return null;
  let lowerTick = anchorLower - offset * tickSpacing;
  let upperTick = anchorUpper + offset * tickSpacing;
  const minTick = Number(editor?.min_tick);
  const maxTick = Number(editor?.max_tick);
  if (Number.isFinite(minTick)) lowerTick = Math.max(lowerTick, minTick);
  if (Number.isFinite(maxTick)) upperTick = Math.min(upperTick, maxTick);
  if (upperTick <= lowerTick) {
    upperTick = lowerTick + tickSpacing;
    if (Number.isFinite(maxTick) && upperTick > maxTick) {
      upperTick = maxTick;
      lowerTick = upperTick - tickSpacing;
    }
  }
  if (!Number.isInteger(lowerTick) || !Number.isInteger(upperTick) || upperTick <= lowerTick) return null;
  return { lowerTick, upperTick };
}

function buildDefaultFocusedPercentageRange(editor, gridOffset = DEFAULT_GRID_OFFSET) {
  const focused = buildDefaultFocusedTickRange(editor, gridOffset);
  const currentTick = Number(editor?.current_tick);
  if (!focused || !Number.isFinite(currentTick)) return null;
  const lowerPct = (1 - Math.pow(1.0001, focused.lowerTick - currentTick)) * 100;
  const upperPct = (Math.pow(1.0001, focused.upperTick - currentTick) - 1) * 100;
  if (!(lowerPct > 0) || !(upperPct > 0)) return null;
  return {
    lowerValue: formatPercentInputValue(lowerPct),
    upperValue: formatPercentInputValue(upperPct),
  };
}

export default function OpenPositionModal({
  apiBaseUrl,
  initData,
  pool,
  chain,
  wallets,
  walletsLoading,
  smartRanges,
  smartRangesLoading,
  selectedWalletId,
  submitError,
  submitRisk,
  onClearSubmitError,
  onWalletSelect,
  onSubmit,
  onClose,
  busy,
}) {
  const [amount, setAmount] = useState('100');
  const [rangeInputMode, setRangeInputMode] = useState('percentage');
  const [rangeLower, setRangeLower] = useState('');
  const [rangeUpper, setRangeUpper] = useState('');
  const [rangeUpperAuto, setRangeUpperAuto] = useState(true);
  const [tickLowerInput, setTickLowerInput] = useState('');
  const [tickUpperInput, setTickUpperInput] = useState('');
  const [priceLowerInput, setPriceLowerInput] = useState('');
  const [priceUpperInput, setPriceUpperInput] = useState('');
  const [gridBoundaryTarget, setGridBoundaryTarget] = useState('lower');
  const [slippage, setSlippage] = useState('');
  const [entrySwapSlippage, setEntrySwapSlippage] = useState('');
  const [entrySwapSlippageDirty, setEntrySwapSlippageDirty] = useState(false);
  const [entrySwapPreview, setEntrySwapPreview] = useState(null);
  const [entrySwapPreviewLoading, setEntrySwapPreviewLoading] = useState(false);
  const [previewPending, setPreviewPending] = useState(false);
  const [previewSuspended, setPreviewSuspended] = useState(false);
  const [entrySwapPreviewError, setEntrySwapPreviewError] = useState('');
  const defaultRangeSeededRef = useRef(false);
  const previewResumeTimerRef = useRef(null);
  const autoSingleSideRangeRef = useRef('');
  const [privateZapInfo, setPrivateZapInfo] = useState(null);
  const [preparePrivateZapInfo, setPreparePrivateZapInfo] = useState(null);
  const [previewChecks, setPreviewChecks] = useState([]);
  const [prepareTokenRisk, setPrepareTokenRisk] = useState(null);
  const [previewTokenRisk, setPreviewTokenRisk] = useState(null);
  const [error, setError] = useState('');
  const [riskAck, setRiskAck] = useState(false);
  const [dcaEnabled, setDcaEnabled] = useState(false);
  const [dcaPercentages, setDcaPercentages] = useState([50, 50]);
  const [dcaInterval, setDcaInterval] = useState(30);
  const [dcaDefaultsLoaded, setDcaDefaultsLoaded] = useState(false);
  const [globalDcaMinSplitAmount, setGlobalDcaMinSplitAmount] = useState(0);
  const [globalSlippageTolerance, setGlobalSlippageTolerance] = useState(NaN);
  const [dcaExpanded, setDcaExpanded] = useState(false);
  const [taskMode, setTaskMode] = useState('pause');
  const [walletBalancesHidden, setWalletBalancesHidden] = useState(false);
  const [prepareRangeEditor, setPrepareRangeEditor] = useState(null);
  const [previewRangeEditor, setPreviewRangeEditor] = useState(null);

  const [liqProfile, setLiqProfile] = useState(null);
  const [liqProfileLoading, setLiqProfileLoading] = useState(false);
  const [liqProfileError, setLiqProfileError] = useState('');
  useEffect(() => {
    const saved = window.localStorage.getItem('tglp_open_position_hide_wallet_balances');
    setWalletBalancesHidden(saved === '1');
  }, []);
  useEffect(() => {
    window.localStorage.setItem('tglp_open_position_hide_wallet_balances', walletBalancesHidden ? '1' : '0');
  }, [walletBalancesHidden]);

  const pair = pool?.trading_pair || '--';
  const addr = String(pool?.pool_address || '').trim();
  const version = String(pool?.protocol_version || pool?.factory_name || '').trim();
  const outOfRangeActions = useMemo(
    () => getTaskModeActionSummary(taskMode),
    [taskMode],
  );
  const activeChecks = useMemo(() => (
    Array.isArray(previewChecks) && previewChecks.length > 0
      ? previewChecks
      : []
  ), [previewChecks]);
  const checks = useMemo(() => (
    Array.isArray(activeChecks)
      ? activeChecks.filter((item) => String(item?.key || '').trim() !== 'entry_swap')
      : []
  ), [activeChecks]);
  const warnChecks = checks.filter(c => c.status === 'warn');
  const failChecks = checks.filter(c => c.status === 'fail');
  const blockingFailChecks = failChecks;
  const submitRiskMessage = String(submitRisk?.message || '').trim();
  const submitRiskCode = String(submitRisk?.code || '').trim();
  const submitRiskBlocksSubmit = Boolean(submitRiskMessage)
    && submitRiskCode !== 'pool_liquidity_warning'
    && !Boolean(submitRisk?.risk_ack_required);
  const hasBlockingSafetyFailure = blockingFailChecks.length > 0 || submitRiskBlocksSubmit;
  const blockingSafetyMessage = blockingFailChecks.length > 0
    ? blockingFailChecks.map(c => c.detail || c.label).filter(Boolean).join('; ')
    : '';
  const riskRequiresAck = warnChecks.some(c => c.extra?.risk_ack_required);
  const riskLiquidityUsd = warnChecks.reduce((m, c) => {
    const v = Number(c.extra?.liquidity_usd);
    return (Number.isFinite(v) && v >= 0 && m === null) ? v : m;
  }, null);
  const riskMessage = warnChecks.length > 0
    ? (warnChecks.map(c => c.detail || c.label).filter(Boolean).join('；') || null)
    : null;

  const tokenRisk = normalizeTokenRisk(
    previewTokenRisk || prepareTokenRisk || submitRisk?.token_risk || pool?.token_risk
  );
  const tokenRiskTone = tokenRiskToneClass(tokenRisk);
  const tokenRiskTitle = tokenRisk ? tokenRiskLabel(tokenRisk) : '';
  const tokenRiskDetail = tokenRisk ? tokenRiskSummary(tokenRisk) : '';
  const tokenRiskSymbol = tokenRisk?.token_symbol || shortAddressShared(tokenRisk?.token_address);

  const showWalletPicker = Array.isArray(wallets) && wallets.length > 1;
  const visibleSmartRanges = useMemo(() => (
    Array.isArray(smartRanges)
      ? smartRanges
        .filter((item) => Number(item?.range_percent) > 0)
        .slice(0, 6)
      : []
  ), [smartRanges]);
  const quickRangeOptions = useMemo(() => {
    const merged = [];
    const seen = new Set();
    for (const item of visibleSmartRanges) {
      const rangePercent = Number(item?.range_percent);
      if (!Number.isFinite(rangePercent) || rangePercent <= 0) continue;
      const key = `${rangePercent}-${rangePercent}`;
      if (seen.has(key)) continue;
      seen.add(key);
      const positionCount = Math.max(0, Number(item?.position_count) || 0);
      merged.push({
        key: `smart-${rangePercent}-${positionCount}-${merged.length}`,
        label: `${formatRangePercentCompact(rangePercent)}${positionCount > 1 ? ` +${positionCount - 1}` : ''}`,
        subLabel: `聪明钱 · ${formatUsdCompact(item?.total_amount_usd)}`,
        lowerValue: rangePercent,
        upperValue: rangePercent,
      });
    }
    for (const item of PRESET_RANGES) {
      const key = `${item}-${item}`;
      if (seen.has(key)) continue;
      seen.add(key);
      merged.push({
        key: `preset-${item}`,
        label: `${item}%`,
        subLabel: '快捷区间',
        lowerValue: item,
        upperValue: item,
      });
    }
    return merged;
  }, [visibleSmartRanges]);
  const resolvedWalletId = useMemo(() => {
    if (!Array.isArray(wallets) || wallets.length === 0) return 0;
    if (wallets.length === 1) return wallets[0].id;
    if (selectedWalletId && wallets.some((w) => w.id === selectedWalletId)) return selectedWalletId;
    const def = wallets.find((w) => w.is_default);
    return def ? def.id : wallets[0].id;
  }, [wallets, selectedWalletId]);
  const selectedWalletStableBalance = useMemo(() => {
    if (!Array.isArray(wallets) || wallets.length === 0) return NaN;
    const selected = wallets.find((wallet) => Number(wallet?.id) === Number(resolvedWalletId)) || wallets[0];
    if (!selected || selected.stable_balance === 'N/A') return NaN;
    const value = Number(selected.stable_balance);
    return Number.isFinite(value) && value > 0 ? value : NaN;
  }, [resolvedWalletId, wallets]);

  const taskSlippage = useMemo(() => parseOptionalPercent(slippage), [slippage]);
  const entrySwapSlippageValue = useMemo(() => parseOptionalPercent(entrySwapSlippage), [entrySwapSlippage]);
  const amountValue = Number(amount);
  const effectiveGlobalDcaMinSplitAmount = Number.isFinite(Number(globalDcaMinSplitAmount)) && Number(globalDcaMinSplitAmount) > 0
    ? Number(globalDcaMinSplitAmount)
    : 0;
  const dcaAmountBelowThreshold = effectiveGlobalDcaMinSplitAmount > 0
    && Number.isFinite(amountValue)
    && amountValue > 0
    && amountValue < effectiveGlobalDcaMinSplitAmount;
  const effectiveDcaEnabled = dcaEnabled && !dcaAmountBelowThreshold;
  const globalSlippageHint = Number.isFinite(Number(globalSlippageTolerance)) && Number(globalSlippageTolerance) >= 0
    ? `滑点: ${formatPercent(globalSlippageTolerance)}`
    : '留空则使用全局配置';
  const rangeLowerValue = Number(rangeLower);
  const rangeUpperValue = Number(rangeUpper);
  const tickLowerValue = Number(String(tickLowerInput || '').trim());
  const tickUpperValue = Number(String(tickUpperInput || '').trim());
  const visibleError = error || entrySwapPreviewError || blockingSafetyMessage || submitRiskMessage || String(submitError || '').trim();
  const submitDisabled = busy || previewPending || previewSuspended || hasBlockingSafetyFailure;
  const submitDisabledReason = busy
    ? '正在提交'
    : previewPending
      ? '开仓预览刷新中'
      : previewSuspended
        ? '区间刚调整，等待预览刷新'
        : hasBlockingSafetyFailure
          ? (blockingSafetyMessage || submitRiskMessage || '安全检查未通过')
          : '';
  const submitButtonLabel = busy
    ? '提交中...'
    : previewPending
      ? '预览刷新中...'
      : previewSuspended
        ? '等待预览刷新'
        : '确认开仓';
  const showPrivateZapProtectionHint = Boolean(privateZapInfo?.show_protection_hint || preparePrivateZapInfo?.show_protection_hint);
  const rangeEditor = previewRangeEditor || prepareRangeEditor;
  const gridBins = useMemo(() => buildGridBins(rangeEditor), [rangeEditor]);
  const defaultFocusedRange = useMemo(() => buildDefaultFocusedTickRange(rangeEditor), [rangeEditor]);
  const defaultFocusedPercentageRange = useMemo(() => buildDefaultFocusedPercentageRange(rangeEditor), [rangeEditor]);
  const rangeShapeLabel = useMemo(() => {
    switch (String(rangeEditor?.position_shape || '').trim()) {
      case 'single_token0':
      case 'single_token1':
        return `单边 ${rangeEditor?.dominant_token_symbol || '--'}`;
      case 'dual_sided':
        return '双边';
      default:
        return '';
    }
  }, [rangeEditor]);

  const token0Decimals = Number(pool?.token0_decimals ?? pool?.token0?.decimals ?? 18) || 18;
  const token1Decimals = Number(pool?.token1_decimals ?? pool?.token1?.decimals ?? 18) || 18;
  const token0Symbol = String(pool?.token0_symbol || pool?.token0?.symbol || '').toUpperCase();
  const token1Symbol = String(pool?.token1_symbol || pool?.token1?.symbol || '').toUpperCase();
  const STABLE_OR_QUOTE = new Set(['USDT', 'USDC', 'BUSD', 'DAI', 'FDUSD', 'TUSD', 'USDD', 'USDE']);
  const quoteIsToken1 = STABLE_OR_QUOTE.has(token1Symbol) ? true : (STABLE_OR_QUOTE.has(token0Symbol) ? false : undefined);
  const defaultInvert = STABLE_OR_QUOTE.has(token0Symbol);
  const [invertPrice, setInvertPrice] = useState(defaultInvert);
  useEffect(() => { setInvertPrice(defaultInvert); }, [defaultInvert]);
  const manualRangeInputOptions = useMemo(() => (
    [...RANGE_INPUT_OPTIONS, { key: 'price', label: '浠锋牸鍖洪棿' }]
  ), []);
  const priceTickRange = useMemo(() => (
    normalizeDisplayPriceTickRange(
      priceLowerInput,
      priceUpperInput,
      invertPrice,
      token0Decimals,
      token1Decimals,
      Number(rangeEditor?.tick_spacing),
      Number(rangeEditor?.min_tick),
      Number(rangeEditor?.max_tick),
    )
  ), [
    priceLowerInput,
    priceUpperInput,
    invertPrice,
    token0Decimals,
    token1Decimals,
    rangeEditor?.tick_spacing,
    rangeEditor?.min_tick,
    rangeEditor?.max_tick,
  ]);
  const selectedManualTickLower = useMemo(() => {
    if (rangeInputMode === 'price') return priceTickRange?.lowerTick ?? null;
    return Number.isInteger(tickLowerValue) ? tickLowerValue : null;
  }, [rangeInputMode, priceTickRange, tickLowerValue]);
  const selectedManualTickUpper = useMemo(() => {
    if (rangeInputMode === 'price') return priceTickRange?.upperTick ?? null;
    return Number.isInteger(tickUpperValue) ? tickUpperValue : null;
  }, [rangeInputMode, priceTickRange, tickUpperValue]);
  const syncPriceInputsFromTicks = useCallback((lowerTick, upperTick) => {
    const displayRange = buildDisplayPriceRangeFromTicks(lowerTick, upperTick, invertPrice, token0Decimals, token1Decimals);
    if (!displayRange) return false;
    setPriceLowerInput(formatPriceInputValue(displayRange.lowerPrice));
    setPriceUpperInput(formatPriceInputValue(displayRange.upperPrice));
    return true;
  }, [invertPrice, token0Decimals, token1Decimals]);
  const clearErrors = useCallback(() => {
    if (error) setError('');
    if (entrySwapPreviewError) setEntrySwapPreviewError('');
    if (typeof onClearSubmitError === 'function') onClearSubmitError();
  }, [error, entrySwapPreviewError, onClearSubmitError]);
  const applyAmountPreset = useCallback((value) => {
    const next = formatAmountPresetValue(value);
    if (!next) return;
    clearErrors();
    setAmount(next);
  }, [clearErrors]);
  const applyResolvedTickRange = useCallback((lowerTick, upperTick, options = {}) => {
    if (!Number.isInteger(lowerTick) || !Number.isInteger(upperTick) || upperTick <= lowerTick) return false;
    setTickLowerInput(String(lowerTick));
    setTickUpperInput(String(upperTick));
    syncPriceInputsFromTicks(lowerTick, upperTick);
    if (options.clear !== false) clearErrors();
    return true;
  }, [clearErrors, syncPriceInputsFromTicks]);
  const suppressPreview = useCallback((delay = 900) => {
    setPreviewSuspended(true);
    setEntrySwapPreviewLoading(false);
    setPreviewPending(false);
    if (previewResumeTimerRef.current) {
      window.clearTimeout(previewResumeTimerRef.current);
    }
    previewResumeTimerRef.current = window.setTimeout(() => {
      setPreviewSuspended(false);
      previewResumeTimerRef.current = null;
    }, delay);
  }, []);

  useEffect(() => () => {
    if (previewResumeTimerRef.current) {
      window.clearTimeout(previewResumeTimerRef.current);
    }
  }, []);

  const previewRequest = useMemo(() => {
    if (!apiBaseUrl || !initData || !addr || !version) return null;
    if (!Number.isFinite(amountValue) || amountValue <= 0) return null;
    if (!taskSlippage.valid || !entrySwapSlippageValue.valid) return null;
    if (walletsLoading) return null;
    if (showWalletPicker && !resolvedWalletId) return null;
    const requestRangeInputMode = rangeInputMode === 'price' ? 'tick' : rangeInputMode;
    const base = {
      apiBaseUrl,
      initData,
      chain,
      poolAddress: addr,
      poolVersion: version,
      amount: amountValue,
      rangeInputMode: requestRangeInputMode,
      slippageTolerance: taskSlippage.value,
      entrySwapSlippageTolerance: entrySwapSlippageValue.value,
      allowEntrySwap: true,
      walletId: resolvedWalletId || undefined,
      ackLiquidityRisk: riskAck,
      taskMode,
    };
    if (requestRangeInputMode === 'percentage') {
      if (!Number.isFinite(rangeLowerValue) || rangeLowerValue <= 0 || rangeLowerValue >= 100) return null;
      if (!Number.isFinite(rangeUpperValue) || rangeUpperValue <= 0 || rangeUpperValue >= 100) return null;
      return {
        ...base,
        rangeLowerPct: rangeLowerValue,
        rangeUpperPct: rangeUpperValue,
      };
    }
    if (!Number.isInteger(selectedManualTickLower) || !Number.isInteger(selectedManualTickUpper) || selectedManualTickLower >= selectedManualTickUpper) {
      return null;
    }
    return {
      ...base,
      tickLower: selectedManualTickLower,
      tickUpper: selectedManualTickUpper,
    };
  }, [
    apiBaseUrl,
    initData,
    addr,
    version,
    chain,
    amountValue,
    rangeInputMode,
    rangeLowerValue,
    rangeUpperValue,
    selectedManualTickLower,
    selectedManualTickUpper,
    taskSlippage,
    entrySwapSlippageValue,
    walletsLoading,
    showWalletPicker,
    resolvedWalletId,
    riskAck,
    taskMode,
  ]);

  useEffect(() => {
    defaultRangeSeededRef.current = false;
    setRiskAck(false);
    setPreviewChecks([]);
    setEntrySwapPreview(null);
    setEntrySwapPreviewError('');
    setEntrySwapPreviewLoading(false);
    setPreviewPending(false);
    setPreviewSuspended(false);
    setPrivateZapInfo(null);
    setPreparePrivateZapInfo(null);
    setPrepareTokenRisk(null);
    setPreviewTokenRisk(null);
    setEntrySwapSlippage('');
    setEntrySwapSlippageDirty(false);
    setPrepareRangeEditor(null);
    setPreviewRangeEditor(null);
    setRangeLower('');
    setRangeUpper('');
    setRangeUpperAuto(true);
    setRangeInputMode('percentage');
    setTickLowerInput('');
    setTickUpperInput('');
    setPriceLowerInput('');
    setPriceUpperInput('');
    setGridBoundaryTarget('lower');
    autoSingleSideRangeRef.current = '';
    if (previewResumeTimerRef.current) {
      window.clearTimeout(previewResumeTimerRef.current);
      previewResumeTimerRef.current = null;
    }
  }, [addr, version]);

  useEffect(() => {
    if (!apiBaseUrl || !initData || !addr || !version) {
      setPrepareRangeEditor(null);
      setPreparePrivateZapInfo(null);
      return undefined;
    }
    if (walletsLoading) return undefined;
    if (showWalletPicker && !resolvedWalletId) return undefined;

    let active = true;
    const controller = new AbortController();
    prepareOpenPosition({
      apiBaseUrl,
      initData,
      chain,
      poolAddress: addr,
      poolVersion: version,
      walletId: resolvedWalletId || undefined,
      signal: controller.signal,
    })
      .then((resp) => {
        if (!active) return;
        setPrepareRangeEditor(resp?.range_editor && typeof resp.range_editor === 'object' ? resp.range_editor : null);
        setPreparePrivateZapInfo(resp?.private_zap && typeof resp.private_zap === 'object' ? resp.private_zap : null);
        setPrepareTokenRisk(resp?.token_risk && typeof resp.token_risk === 'object' ? resp.token_risk : null);
      })
      .catch((err) => {
        if (!active || controller.signal.aborted) return;
        setPrepareRangeEditor(null);
        setPreparePrivateZapInfo(null);
        setPrepareTokenRisk(null);
        throw err;
      });
    return () => {
      active = false;
      controller.abort();
    };
  }, [apiBaseUrl, initData, addr, version, chain, walletsLoading, showWalletPicker, resolvedWalletId]);

  useEffect(() => {
    if (!riskRequiresAck) setRiskAck(false);
  }, [riskRequiresAck]);

  useEffect(() => {
    if (!entrySwapPreview?.required || entrySwapSlippageDirty) return;
    const recommended = Number(entrySwapPreview?.recommended_slippage_tolerance);
    const current = Number(entrySwapPreview?.current_slippage_tolerance);
    const next = Number.isFinite(recommended) ? recommended : current;
    if (!Number.isFinite(next)) return;
    setEntrySwapSlippage(String(next));
  }, [entrySwapPreview, entrySwapSlippageDirty]);

  useEffect(() => {
    if (previewSuspended) {
      setEntrySwapPreviewLoading(false);
      setPreviewPending(false);
      return undefined;
    }
    if (!previewRequest) {
      setEntrySwapPreview(null);
      setEntrySwapPreviewLoading(false);
      setPreviewPending(false);
      setEntrySwapPreviewError('');
      setPrivateZapInfo(null);
      setPreviewChecks([]);
      setPreviewRangeEditor(null);
      setPreviewTokenRisk(null);
      return undefined;
    }

    let active = true;
    const controller = new AbortController();
    setPreviewPending(true);
    setEntrySwapPreviewLoading(false);
    setEntrySwapPreviewError('');

    const timer = window.setTimeout(async () => {
      try {
        const resp = await previewOpenPosition({
          ...previewRequest,
          signal: controller.signal,
        });
        if (!active) return;
        setPreviewChecks(Array.isArray(resp?.checks) ? resp.checks : []);
        setEntrySwapPreview(resp?.entry_swap || { required: false });
        setPrivateZapInfo(resp?.private_zap && typeof resp.private_zap === 'object' ? resp.private_zap : null);
        setPreviewRangeEditor(resp?.range_editor && typeof resp.range_editor === 'object' ? resp.range_editor : null);
        setPreviewTokenRisk(resp?.token_risk && typeof resp.token_risk === 'object' ? resp.token_risk : null);
      } catch (e) {
        if (!active || controller.signal.aborted) return;
        const payload = resolveOpenPositionErrorPayload(e);
        const failChecks = extractOpenPositionErrorChecks(e);
        const entrySwapInfo = payload?.entry_swap && typeof payload.entry_swap === 'object'
          ? payload.entry_swap
          : null;
        setEntrySwapPreview(entrySwapInfo);
        setPrivateZapInfo(payload?.private_zap && typeof payload.private_zap === 'object' ? payload.private_zap : null);
        setPreviewChecks(failChecks);
        setPreviewRangeEditor(payload?.range_editor && typeof payload.range_editor === 'object' ? payload.range_editor : null);
        setPreviewTokenRisk(payload?.token_risk && typeof payload.token_risk === 'object' ? payload.token_risk : null);
        setEntrySwapPreviewError(failChecks.length > 0 ? '' : String(e?.message || e || '获取前置兑换预览失败'));
        throw e;
      } finally {
        if (active) {
          setEntrySwapPreviewLoading(false);
          setPreviewPending(false);
        }
      }
    }, 350);

    return () => {
      active = false;
      window.clearTimeout(timer);
      controller.abort();
    };
  }, [previewRequest, previewSuspended]);

  const applyRange = useCallback((lo, hi) => {
    clearErrors();
    suppressPreview();
    setRangeInputMode('percentage');
    setRangeLower(String(lo));
    setRangeUpper(String(hi));
    setRangeUpperAuto(true);
  }, [clearErrors, suppressPreview]);
  const displayedLowerPct = Number(rangeEditor?.range_lower_pct ?? rangeLower);
  const displayedUpperPct = Number(rangeEditor?.range_upper_pct ?? rangeUpper);

  const handleRangeLowerChange = useCallback((value) => {
    clearErrors();
    suppressPreview();
    setRangeInputMode('percentage');
    setRangeLower((prevLower) => {
      if (rangeUpperAuto || String(rangeUpper || '').trim() === '' || String(rangeUpper) === String(prevLower)) {
        setRangeUpper(value);
      }
      return value;
    });
  }, [clearErrors, rangeUpper, rangeUpperAuto, suppressPreview]);

  const handleRangeUpperChange = useCallback((value) => {
    clearErrors();
    suppressPreview();
    setRangeInputMode('percentage');
    setRangeUpperAuto(false);
    setRangeUpper(value);
  }, [clearErrors, suppressPreview]);

  const syncTicksFromEditor = useCallback((editor) => {
    const lower = Number(editor?.tick_lower);
    const upper = Number(editor?.tick_upper);
    return applyResolvedTickRange(lower, upper, { clear: false });
  }, [applyResolvedTickRange]);

  const applyDefaultTickRange = useCallback(() => {
    if (syncTicksFromEditor(previewRangeEditor)) return;
    if (defaultFocusedRange && applyResolvedTickRange(defaultFocusedRange.lowerTick, defaultFocusedRange.upperTick, { clear: false })) {
      return;
    }
    const lower = Number(rangeEditor?.anchor_tick_lower);
    const upper = Number(rangeEditor?.anchor_tick_upper);
    applyResolvedTickRange(lower, upper, { clear: false });
  }, [previewRangeEditor, defaultFocusedRange, rangeEditor, syncTicksFromEditor, applyResolvedTickRange]);

  const handleRangeInputModeChange = useCallback((mode) => {
    clearErrors();
    suppressPreview();
    setRangeInputMode(mode);
    if (mode !== 'percentage') {
      if (!syncTicksFromEditor(previewRangeEditor)) {
        applyDefaultTickRange();
      }
    }
  }, [clearErrors, previewRangeEditor, applyDefaultTickRange, syncTicksFromEditor, suppressPreview]);

  const nudgeTickBoundary = useCallback((target, delta) => {
    const spacing = Number(rangeEditor?.tick_spacing);
    if (!Number.isFinite(spacing) || spacing <= 0) return;
    const minTick = Number(rangeEditor?.min_tick);
    const maxTick = Number(rangeEditor?.max_tick);
    let nextLower = Number.isInteger(selectedManualTickLower) ? selectedManualTickLower : Number(rangeEditor?.tick_lower);
    let nextUpper = Number.isInteger(selectedManualTickUpper) ? selectedManualTickUpper : Number(rangeEditor?.tick_upper);
    if (!Number.isInteger(nextLower)) nextLower = Number(rangeEditor?.anchor_tick_lower);
    if (!Number.isInteger(nextUpper)) nextUpper = Number(rangeEditor?.anchor_tick_upper);
    if (!Number.isInteger(nextLower) || !Number.isInteger(nextUpper)) return;
    const nextRange = nudgeDisplayPriceBoundary(
      target,
      delta,
      invertPrice,
      spacing,
      nextLower,
      nextUpper,
      minTick,
      maxTick,
    );
    if (!nextRange) return;
    suppressPreview();
    setRangeInputMode('tick');
    applyResolvedTickRange(nextRange.lowerTick, nextRange.upperTick);
  }, [rangeEditor, selectedManualTickLower, selectedManualTickUpper, invertPrice, applyResolvedTickRange, suppressPreview]);

  const applyGridBin = useCallback((bin) => {
    if (!bin) return;
    const spacing = Number(rangeEditor?.tick_spacing);
    if (!Number.isFinite(spacing) || spacing <= 0) return;
    let nextLower = Number.isInteger(selectedManualTickLower) ? selectedManualTickLower : Number(rangeEditor?.anchor_tick_lower);
    let nextUpper = Number.isInteger(selectedManualTickUpper) ? selectedManualTickUpper : Number(rangeEditor?.anchor_tick_upper);
    if (gridBoundaryTarget === 'lower') {
      nextLower = bin.lowerTick;
      if (nextLower >= nextUpper) nextUpper = nextLower + spacing;
    } else {
      nextUpper = bin.upperTick;
      if (nextUpper <= nextLower) nextLower = nextUpper - spacing;
    }
    suppressPreview();
    applyResolvedTickRange(nextLower, nextUpper);
  }, [rangeEditor, selectedManualTickLower, selectedManualTickUpper, gridBoundaryTarget, applyResolvedTickRange, suppressPreview]);

  const shiftRangeToSingleSide = useCallback((side) => {
    const spacing = Number(rangeEditor?.tick_spacing);
    if (!Number.isFinite(spacing) || spacing <= 0) return;
    const anchorLower = Number(rangeEditor?.anchor_tick_lower);
    const anchorUpper = Number(rangeEditor?.anchor_tick_upper);
    if (!Number.isInteger(anchorLower) || !Number.isInteger(anchorUpper)) return;
    const minTick = Number(rangeEditor?.min_tick);
    const maxTick = Number(rangeEditor?.max_tick);
    const resolvedCurrentLower = Number(rangeEditor?.tick_lower);
    const resolvedCurrentUpper = Number(rangeEditor?.tick_upper);
    const currentLower = Number.isInteger(selectedManualTickLower)
      ? selectedManualTickLower
      : (Number.isInteger(resolvedCurrentLower) ? resolvedCurrentLower : anchorLower);
    const currentUpper = Number.isInteger(selectedManualTickUpper)
      ? selectedManualTickUpper
      : (Number.isInteger(resolvedCurrentUpper) ? resolvedCurrentUpper : anchorUpper);
    const width = Math.max(spacing, currentUpper - currentLower);
    let nextLower = currentLower;
    let nextUpper = currentUpper;
    if (side === 'lower') {
      nextUpper = anchorLower;
      nextLower = nextUpper - width;
      if (Number.isFinite(minTick) && nextLower < minTick) {
        nextLower = minTick;
        nextUpper = nextLower + width;
      }
    } else {
      nextLower = anchorUpper;
      nextUpper = nextLower + width;
      if (Number.isFinite(maxTick) && nextUpper > maxTick) {
        nextUpper = maxTick;
        nextLower = nextUpper - width;
      }
    }
    suppressPreview();
    setRangeInputMode('tick');
    applyResolvedTickRange(nextLower, nextUpper);
  }, [rangeEditor, selectedManualTickLower, selectedManualTickUpper, applyResolvedTickRange, suppressPreview]);

  useEffect(() => {
    if (rangeInputMode !== 'percentage') return;
    if (defaultRangeSeededRef.current) return;
    if (String(rangeLower || '').trim() || String(rangeUpper || '').trim()) return;
    if (!defaultFocusedPercentageRange) return;
    setRangeLower(defaultFocusedPercentageRange.lowerValue);
    setRangeUpper(defaultFocusedPercentageRange.upperValue);
    defaultRangeSeededRef.current = true;
  }, [rangeInputMode, rangeLower, rangeUpper, defaultFocusedPercentageRange]);

  useEffect(() => {
    if (rangeInputMode === 'percentage') return;
    if (String(tickLowerInput || '').trim() && String(tickUpperInput || '').trim()) return;
    applyDefaultTickRange();
  }, [rangeInputMode, tickLowerInput, tickUpperInput, applyDefaultTickRange]);

  const lastInvertRef = useRef(invertPrice);
  useEffect(() => {
    if (lastInvertRef.current === invertPrice) return;
    lastInvertRef.current = invertPrice;
    if (rangeInputMode !== 'price') return;
    if (!Number.isInteger(selectedManualTickLower) || !Number.isInteger(selectedManualTickUpper) || selectedManualTickUpper <= selectedManualTickLower) {
      return;
    }
    syncPriceInputsFromTicks(selectedManualTickLower, selectedManualTickUpper);
  }, [invertPrice, rangeInputMode, selectedManualTickLower, selectedManualTickUpper, syncPriceInputsFromTicks]);

  useEffect(() => {
    if (!apiBaseUrl || !initData || dcaDefaultsLoaded) return;
    let cancelled = false;
    const controller = new AbortController();
    fetchGlobalConfig({ apiBaseUrl, initData, signal: controller.signal })
      .then((resp) => {
        if (cancelled) return;
        const cfg = resp?.config || resp || {};
        setDcaEnabled(Boolean(cfg.dca_enabled));
        setDcaPercentages(parseDCAPercentagesAny(cfg.dca_percentages_json ?? cfg.dca_percentages));
        setGlobalDcaMinSplitAmount(Number(cfg.dca_min_split_amount_usdt) || 0);
        setGlobalSlippageTolerance(Number(cfg.slippage_tolerance));
        const interval = Number(cfg.dca_interval_seconds);
        if (Number.isFinite(interval) && interval >= 0) setDcaInterval(interval);
        setDcaDefaultsLoaded(true);
      })
      .catch((err) => {
        if (!cancelled) setDcaDefaultsLoaded(true);
        if (!cancelled) throw err;
      });
    return () => {
      cancelled = true;
      controller.abort();
    };
  }, [apiBaseUrl, initData, dcaDefaultsLoaded]);

  const dcaSum = useMemo(
    () => dcaPercentages.reduce((acc, v) => acc + (Number(v) || 0), 0),
    [dcaPercentages],
  );
  const dcaSumValid = Math.abs(dcaSum - 100) < 0.01;
  const dcaSummaryItems = useMemo(() => {
    if (!Array.isArray(dcaPercentages) || dcaPercentages.length === 0) return [];
    return dcaPercentages.map((pct, idx) => {
      const batchAmount = Number.isFinite(amountValue) && amountValue > 0
        ? formatUsdCompact(amountValue * (Number(pct) || 0) / 100)
        : '$--';
      return {
        key: `batch-${idx}`,
        label: idx === 0 ? '首批' : `第${idx + 1}批`,
        amount: batchAmount,
      };
    });
  }, [amountValue, dcaPercentages]);

  useEffect(() => {
    setDcaExpanded(false);
    setTaskMode('pause');
  }, [addr]);

  const protocolKind = useMemo(() => {
    const v = String(version || '').toLowerCase();
    if (v.includes('v4')) return 'v4';
    if (v.includes('v3') || v.includes('pancake') || v.includes('aerodrome') || v.includes('slipstream')) return 'v3';
    return '';
  }, [version]);

  const reloadLiqProfile = useCallback(() => {
    if (!addr || !protocolKind || !chain) {
      setLiqProfile(null);
      return;
    }
    setLiqProfileLoading(true);
    setLiqProfileError('');
    fetchPoolLiquidityDistribution({
      apiBaseUrl,
      initData,
      chain,
      protocol: protocolKind,
      address: addr,
      radius: 24,
    })
      .then((data) => {
        setLiqProfile(data);
      })
      .catch((err) => {
        const msg = String(err?.message || err || '');
        if (/page could not be found|<html|<!doctype/i.test(msg)) {
          // eslint-disable-next-line no-console
          console.warn('[liquidity_distribution] backend route missing or unreachable', msg.slice(0, 200));
          setLiqProfileError('接口未就绪');
        } else {
          setLiqProfileError(msg.slice(0, 80));
        }
        // 刷新失败时保留旧数据，避免画布闪烁清空
        throw err;
      })
      .finally(() => {
        setLiqProfileLoading(false);
      });
  }, [apiBaseUrl, initData, addr, protocolKind, chain]);

  useEffect(() => {
    if (!addr || !protocolKind || !chain) {
      setLiqProfile(null);
      return undefined;
    }
    const ctrl = new AbortController();
    // 池子切换（addr/protocol/chain 改变）时清空旧数据；同一池子的刷新不清空
    setLiqProfile(null);
    setLiqProfileLoading(true);
    setLiqProfileError('');
    fetchPoolLiquidityDistribution({
      apiBaseUrl,
      initData,
      chain,
      protocol: protocolKind,
      address: addr,
      radius: 24,
      signal: ctrl.signal,
    })
      .then((data) => {
        if (ctrl.signal.aborted) return;
        setLiqProfile(data);
      })
      .catch((err) => {
        if (ctrl.signal.aborted) return;
        const msg = String(err?.message || err || '');
        if (/page could not be found|<html|<!doctype/i.test(msg)) {
          // eslint-disable-next-line no-console
          console.warn('[liquidity_distribution] backend route missing or unreachable', msg.slice(0, 200));
          setLiqProfileError('接口未就绪');
        } else {
          setLiqProfileError(msg.slice(0, 80));
        }
        setLiqProfile(null);
        throw err;
      })
      .finally(() => {
        if (!ctrl.signal.aborted) setLiqProfileLoading(false);
      });
    return () => ctrl.abort();
  }, [apiBaseUrl, initData, addr, protocolKind, chain]);

  // 定时刷新流动性分布：3s 一次，in-flight 去重防止堆请求；后端已无缓存，近似实时。
  const liqInFlightRef = useRef(false);
  const reloadLiqProfileSafe = useCallback(() => {
    if (liqInFlightRef.current) return;
    if (!addr || !protocolKind || !chain) return;
    liqInFlightRef.current = true;
    setLiqProfileError('');
    fetchPoolLiquidityDistribution({
      apiBaseUrl, initData, chain, protocol: protocolKind, address: addr, radius: 24,
    })
      .then((data) => { setLiqProfile(data); })
      .catch((err) => {
        const msg = String(err?.message || err || '');
        if (/page could not be found|<html|<!doctype/i.test(msg)) {
          // eslint-disable-next-line no-console
          console.warn('[liquidity_distribution] backend route missing', msg.slice(0, 200));
          setLiqProfileError('接口未就绪');
        } else {
          setLiqProfileError(msg.slice(0, 80));
        }
        throw err;
      })
      .finally(() => { liqInFlightRef.current = false; });
  }, [apiBaseUrl, initData, addr, protocolKind, chain]);

  useEffect(() => {
    if (!addr || !protocolKind || !chain) return undefined;
    const timer = setInterval(() => {
      if (document.hidden) return;
      reloadLiqProfileSafe();
    }, 3000);
    return () => clearInterval(timer);
  }, [addr, protocolKind, chain, reloadLiqProfileSafe]);

  const chartLowerTick = useMemo(() => {
    if (rangeInputMode !== 'percentage') {
      return Number.isInteger(selectedManualTickLower)
        ? selectedManualTickLower
        : (defaultFocusedRange?.lowerTick ?? null);
    }
    const ed = rangeEditor;
    if (!ed || !Number.isFinite(Number(ed.current_tick))) return null;
    const lowerPct = Number(rangeLower);
    if (!Number.isFinite(lowerPct) || lowerPct <= 0) return defaultFocusedRange?.lowerTick ?? null;
    const ratio = 1 - lowerPct / 100;
    if (ratio <= 0) return defaultFocusedRange?.lowerTick ?? null;
    return Math.round(Number(ed.current_tick) + Math.log(ratio) / Math.log(1.0001));
  }, [rangeInputMode, selectedManualTickLower, rangeEditor, rangeLower, defaultFocusedRange]);

  const chartUpperTick = useMemo(() => {
    if (rangeInputMode !== 'percentage') {
      return Number.isInteger(selectedManualTickUpper)
        ? selectedManualTickUpper
        : (defaultFocusedRange?.upperTick ?? null);
    }
    const ed = rangeEditor;
    if (!ed || !Number.isFinite(Number(ed.current_tick))) return null;
    const upperPct = Number(rangeUpper);
    if (!Number.isFinite(upperPct) || upperPct <= 0) return defaultFocusedRange?.upperTick ?? null;
    const ratio = 1 + upperPct / 100;
    return Math.round(Number(ed.current_tick) + Math.log(ratio) / Math.log(1.0001));
  }, [rangeInputMode, selectedManualTickUpper, rangeEditor, rangeUpper, defaultFocusedRange]);

  const priceRange = useMemo(() => {
    const refTick = Number(liqProfile?.current_tick);
    const fallbackTick = Number(rangeEditor?.current_tick);
    const baseTick = Number.isFinite(refTick) ? refTick : (Number.isFinite(fallbackTick) ? fallbackTick : null);
    if (baseTick === null) return null;
    const apply = (p) => (invertPrice && p > 0 ? 1 / p : p);
    const fmt = (v) => {
      if (!Number.isFinite(v) || v <= 0) return '--';
      if (v >= 1_000_000) return v.toExponential(3);
      if (v >= 1) return v.toLocaleString(undefined, { maximumFractionDigits: 4 });
      if (v >= 0.0001) return v.toLocaleString(undefined, { maximumFractionDigits: 6 });
      return v.toExponential(3);
    };
    const lowerTick = Number.isFinite(chartLowerTick) ? chartLowerTick : null;
    const upperTick = Number.isFinite(chartUpperTick) ? chartUpperTick : null;
    const currentPrice = apply(tickToPoolPrice(baseTick, token0Decimals, token1Decimals));
    const lowerPrice = lowerTick !== null ? apply(tickToPoolPrice(lowerTick, token0Decimals, token1Decimals)) : null;
    const upperPrice = upperTick !== null ? apply(tickToPoolPrice(upperTick, token0Decimals, token1Decimals)) : null;
    const gridStepPct = estimateDisplayGridStepPercent(
      baseTick,
      Number(rangeEditor?.tick_spacing),
      invertPrice,
      token0Decimals,
      token1Decimals,
    );
    // invert 情况下 tickLower 对应较大价格（现实"上限"），所以文字显示要互换
    const lowerText = lowerPrice !== null && upperPrice !== null ? Math.min(lowerPrice, upperPrice) : null;
    const upperText = lowerPrice !== null && upperPrice !== null ? Math.max(lowerPrice, upperPrice) : null;
    // 计价代币单位：invert=true 时是 token0，否则是 token1
    const toPct = (value) => {
      if (!Number.isFinite(currentPrice) || currentPrice <= 0 || !Number.isFinite(value) || value <= 0) return null;
      return ((value / currentPrice) - 1) * 100;
    };
    const quoteSymbol = invertPrice ? token0Symbol : token1Symbol;
    const baseSymbol = invertPrice ? token1Symbol : token0Symbol;
    return {
      currentText: fmt(currentPrice),
      lowerText: lowerText !== null ? fmt(lowerText) : '--',
      upperText: upperText !== null ? fmt(upperText) : '--',
      lowerPctText: lowerText !== null ? formatSignedPercentCompact(toPct(lowerText)) : '--',
      upperPctText: upperText !== null ? formatSignedPercentCompact(toPct(upperText)) : '--',
      quoteSymbol,
      baseSymbol,
      gridStepPctText: Number.isFinite(gridStepPct) ? formatRangePercentCompact(gridStepPct) : '--',
      tickSpacing: Number(rangeEditor?.tick_spacing),
    };
  }, [liqProfile, rangeEditor, chartLowerTick, chartUpperTick, token0Decimals, token1Decimals, invertPrice, token0Symbol, token1Symbol]);
  const resolvedSelectionShape = useMemo(() => {
    const currentTick = Number(liqProfile?.current_tick ?? rangeEditor?.current_tick);
    const lowerTick = Number(chartLowerTick);
    const upperTick = Number(chartUpperTick);
    if (!Number.isFinite(currentTick) || !Number.isFinite(lowerTick) || !Number.isFinite(upperTick) || upperTick <= lowerTick) {
      return { shape: '', dominantTokenSymbol: '' };
    }
    if (currentTick < lowerTick) {
      return { shape: 'single_token0', dominantTokenSymbol: token0Symbol };
    }
    if (currentTick >= upperTick) {
      return { shape: 'single_token1', dominantTokenSymbol: token1Symbol };
    }
    return { shape: 'dual_sided', dominantTokenSymbol: '' };
  }, [liqProfile, rangeEditor, chartLowerTick, chartUpperTick, token0Symbol, token1Symbol]);
  const isSingleSidedSelection = String(resolvedSelectionShape.shape || '').startsWith('single_');

  const selectedShareRangeLowerTick = useMemo(() => {
    if (rangeInputMode !== 'percentage') {
      return Number.isInteger(selectedManualTickLower) ? selectedManualTickLower : null;
    }
    const resolvedLower = Number(rangeEditor?.tick_lower);
    return Number.isInteger(resolvedLower) ? resolvedLower : chartLowerTick;
  }, [rangeInputMode, selectedManualTickLower, rangeEditor, chartLowerTick]);

  const selectedShareRangeUpperTick = useMemo(() => {
    if (rangeInputMode !== 'percentage') {
      return Number.isInteger(selectedManualTickUpper) ? selectedManualTickUpper : null;
    }
    const resolvedUpper = Number(rangeEditor?.tick_upper);
    return Number.isInteger(resolvedUpper) ? resolvedUpper : chartUpperTick;
  }, [rangeInputMode, selectedManualTickUpper, rangeEditor, chartUpperTick]);

  const shareEstimateInfo = useMemo(() => {
    if (!liqProfile) return null;
    const amt = Number(amount);
    if (!Number.isFinite(amt) || amt <= 0) return null;
    const currentTick = Number(liqProfile?.current_tick);
    const lowerTick = Number(selectedShareRangeLowerTick);
    const upperTick = Number(selectedShareRangeUpperTick);
    if (!Number.isFinite(currentTick) || !Number.isFinite(lowerTick) || !Number.isFinite(upperTick) || upperTick <= lowerTick) {
      return null;
    }
    const bins = Array.isArray(liqProfile?.bins) ? liqProfile.bins : [];
    if (bins.length === 0) return null;

    const profileMinTick = Math.min(...bins.map((bin) => Number(bin?.tick_lower)).filter(Number.isFinite));
    const profileMaxTick = Math.max(...bins.map((bin) => Number(bin?.tick_upper)).filter(Number.isFinite));
    if (!Number.isFinite(profileMinTick) || !Number.isFinite(profileMaxTick)) return null;
    if (lowerTick < profileMinTick || upperTick > profileMaxTick) {
      return {
        share: null,
        existingLiquidityUsd: null,
        coverage: 'out_of_view',
      };
    }

    const overlappingBins = bins.filter((bin) => {
      const binLower = Number(bin?.tick_lower);
      const binUpper = Number(bin?.tick_upper);
      return Number.isFinite(binLower)
        && Number.isFinite(binUpper)
        && binUpper > lowerTick
        && binLower < upperTick;
    });

    if (overlappingBins.length === 0) {
      return {
        share: 100,
        existingLiquidityUsd: 0,
        coverage: 'ok',
      };
    }

    let existingLiquidityUsd = 0;
    let hasUsdSample = false;
    for (const bin of overlappingBins) {
      const usd = estimateBinLiquidityUsd(bin, currentTick, token0Decimals, token1Decimals, quoteIsToken1);
      const binLower = Number(bin?.tick_lower);
      const binUpper = Number(bin?.tick_upper);
      const overlapSpan = Math.max(0, Math.min(binUpper, upperTick) - Math.max(binLower, lowerTick));
      const binSpan = binUpper - binLower;
      const overlapRatio = binSpan > 0 ? Math.min(1, overlapSpan / binSpan) : 0;
      if (Number.isFinite(usd) && usd >= 0 && overlapRatio > 0) {
        existingLiquidityUsd += usd * overlapRatio;
        hasUsdSample = true;
      }
    }

    if (!hasUsdSample) {
      return {
        share: null,
        existingLiquidityUsd: null,
        coverage: 'unavailable',
      };
    }

    return {
      share: Math.min((amt / (existingLiquidityUsd + amt)) * 100, 100),
      existingLiquidityUsd,
      coverage: 'ok',
    };
  }, [
    liqProfile,
    amount,
    selectedShareRangeLowerTick,
    selectedShareRangeUpperTick,
    token0Decimals,
    token1Decimals,
    quoteIsToken1,
  ]);

  const shareEstimate = Number.isFinite(Number(shareEstimateInfo?.share)) ? Number(shareEstimateInfo.share) : null;

  useEffect(() => {
    if (!isSingleSidedSelection) {
      autoSingleSideRangeRef.current = '';
      return;
    }
    const signature = `${resolvedSelectionShape.shape}:${chartLowerTick}:${chartUpperTick}`;
    if (!signature || autoSingleSideRangeRef.current === signature) return;
    autoSingleSideRangeRef.current = signature;
    if (dcaEnabled) setDcaEnabled(false);
  }, [
    isSingleSidedSelection,
    resolvedSelectionShape.shape,
    chartLowerTick,
    chartUpperTick,
    dcaEnabled,
  ]);
  const submitDcaEnabled = effectiveDcaEnabled && !isSingleSidedSelection;

  const onChartRangeChange = useCallback(({ lower, upper }) => {
    if (!liqProfile) return;
    suppressPreview(1100);
    const nextLower = Number.isFinite(lower)
      ? lower
      : (Number.isInteger(selectedManualTickLower) ? selectedManualTickLower : chartLowerTick);
    const nextUpper = Number.isFinite(upper)
      ? upper
      : (Number.isInteger(selectedManualTickUpper) ? selectedManualTickUpper : chartUpperTick);
    if (!Number.isInteger(nextLower) || !Number.isInteger(nextUpper) || nextUpper <= nextLower) return;
    setRangeInputMode('tick');
    applyResolvedTickRange(nextLower, nextUpper);
  }, [liqProfile, selectedManualTickLower, selectedManualTickUpper, chartLowerTick, chartUpperTick, applyResolvedTickRange, suppressPreview]);

  const handleChartRangeDragStart = useCallback(() => {
    suppressPreview(1200);
    if (!Number.isInteger(chartLowerTick) || !Number.isInteger(chartUpperTick) || chartUpperTick <= chartLowerTick) return;
    setRangeInputMode('tick');
    applyResolvedTickRange(chartLowerTick, chartUpperTick, { clear: false });
  }, [chartLowerTick, chartUpperTick, applyResolvedTickRange, suppressPreview]);

  const handleChartRangeDragEnd = useCallback(() => {
    suppressPreview(850);
  }, [suppressPreview]);

  const handleChartBinSelect = useCallback((bin) => {
    if (!bin) return;
    const lower = Number(bin?.tick_lower);
    const upper = Number(bin?.tick_upper);
    if (!Number.isInteger(lower) || !Number.isInteger(upper) || upper <= lower) return;
    suppressPreview();
    setRangeInputMode('tick');
    applyResolvedTickRange(lower, upper);
  }, [applyResolvedTickRange, suppressPreview]);

  const handleSubmit = useCallback(() => {
    if (!Number.isFinite(amountValue) || amountValue <= 0) {
      setError('请输入有效的开仓金额。');
      return;
    }
    if (rangeInputMode === 'percentage') {
      if (!Number.isFinite(rangeLowerValue) || rangeLowerValue <= 0 || rangeLowerValue >= 100) {
        setError('请输入有效的下限范围。');
        return;
      }
      if (!Number.isFinite(rangeUpperValue) || rangeUpperValue <= 0 || rangeUpperValue >= 100) {
        setError('请输入有效的上限范围。');
        return;
      }
    } else if (rangeInputMode !== 'price' && (!Number.isInteger(tickLowerValue) || !Number.isInteger(tickUpperValue) || tickLowerValue >= tickUpperValue)) {
      setError('请输入有效的 Tick 区间。');
      return;
    }
    if (rangeInputMode !== 'percentage' && (!Number.isInteger(selectedManualTickLower) || !Number.isInteger(selectedManualTickUpper) || selectedManualTickLower >= selectedManualTickUpper)) {
      setError(rangeInputMode === 'price' ? '请输入有效的价格区间。' : '请输入有效的 Tick 区间。');
      return;
    }
    if (!taskSlippage.valid) {
      setError('任务滑点必须在 0 到 100 之间。');
      return;
    }
    if (!entrySwapSlippageValue.valid) {
      setError('前置兑换滑点必须在 0 到 100 之间。');
      return;
    }
    if (showWalletPicker && !resolvedWalletId) {
      setError('请选择钱包。');
      return;
    }
    if (failChecks.length > 0) {
      setError(failChecks.map(c => c.detail || c.label).join('; '));
      return;
    }
    if (previewRequest && (previewPending || previewSuspended)) {
      setError('前置兑换预览仍在加载，请稍后再试。');
      return;
    }
    if (previewRequest && !entrySwapPreview) {
      setError('前置兑换预览尚未就绪，请稍后再试。');
      return;
    }

    if (submitDcaEnabled) {
      if (dcaPercentages.length < 2 || dcaPercentages.length > 5) {
        setError('分批次数必须在 2–5 批之间。');
        return;
      }
      if (dcaPercentages.some((v) => !(Number(v) >= 5))) {
        setError('每批占比必须 ≥ 5%。');
        return;
      }
      if (!dcaSumValid) {
        setError(`分批百分比之和必须等于 100%（当前 ${dcaSum.toFixed(2)}%）。`);
        return;
      }
      const intervalValue = Number(dcaInterval);
      if (!(Number.isFinite(intervalValue) && intervalValue >= 0 && intervalValue <= 300)) {
        setError('批次间隔必须在 0–300 秒之间。');
        return;
      }
    }

    setError('');
    const rangePayload = rangeInputMode === 'percentage'
      ? {
        rangeLowerPct: rangeLowerValue,
        rangeUpperPct: rangeUpperValue,
      }
      : {
        tickLower: selectedManualTickLower,
        tickUpper: selectedManualTickUpper,
      };
    const requestRangeInputMode = rangeInputMode === 'price' ? 'tick' : rangeInputMode;
    onSubmit({
      poolAddress: addr,
      poolVersion: version,
      chain,
      amount: amountValue,
      rangeInputMode: requestRangeInputMode,
      ...rangePayload,
      slippageTolerance: taskSlippage.value,
      entrySwapSlippageTolerance: entrySwapPreview?.required ? entrySwapSlippageValue.value : undefined,
      allowEntrySwap: true,
      confirmEntrySwap: Boolean(entrySwapPreview?.required),
      walletId: resolvedWalletId || undefined,
      ackLiquidityRisk: riskAck,
      dcaEnabled: submitDcaEnabled,
      dcaPercentages: submitDcaEnabled ? dcaPercentages.map((v) => Number(v) || 0) : undefined,
      dcaIntervalSeconds: submitDcaEnabled ? Number(dcaInterval) : undefined,
      taskMode,
    });
  }, [
    amountValue,
    rangeInputMode,
    rangeLowerValue,
    rangeUpperValue,
    selectedManualTickLower,
    selectedManualTickUpper,
    taskSlippage,
    entrySwapSlippageValue,
    showWalletPicker,
    resolvedWalletId,
    riskRequiresAck,
    riskAck,
    failChecks,
    previewRequest,
    previewPending,
    previewSuspended,
    entrySwapPreview,
    onSubmit,
    addr,
    version,
    chain,
    dcaEnabled,
    effectiveDcaEnabled,
    submitDcaEnabled,
    dcaPercentages,
    dcaSum,
    dcaSumValid,
    dcaInterval,
    taskMode,
  ]);


  return (
    <div className="modal-overlay" onClick={onClose}>
      <div className="modal-box modal-box-wide" onClick={(e) => e.stopPropagation()}>
        <div className="modal-header">
          <h3>开仓</h3>
          <button type="button" className="modal-close" onClick={onClose} disabled={busy}>&times;</button>
        </div>

        <div className="modal-content opm-grid">
          {/* ───────── 左栏：池子信息 + 流动性分布 + 区间设置 ───────── */}
          <div className="opm-left">
            <div className="opm-pair-info">
              <div className="modal-pair">{pair}</div>
              <div className="modal-addr">{addr ? `${addr.slice(0, 10)}...${addr.slice(-8)}` : '--'}</div>
            </div>

            <LiquidityDistributionChart
              bins={liqProfile?.bins || []}
              currentTick={Number(liqProfile?.current_tick)}
              tickSpacing={Number(liqProfile?.tick_spacing)}
              rangeLowerTick={chartLowerTick}
              rangeUpperTick={chartUpperTick}
              onRangeChange={onChartRangeChange}
              onRangeDragStart={handleChartRangeDragStart}
              onRangeDragEnd={handleChartRangeDragEnd}
              onBinSelect={handleChartBinSelect}
              loading={liqProfileLoading}
              token0Decimals={token0Decimals}
              token1Decimals={token1Decimals}
              invertPrice={invertPrice}
              tokenLeftLabel={invertPrice ? token1Symbol : token0Symbol}
              tokenRightLabel={invertPrice ? token0Symbol : token1Symbol}
              quoteIsToken1={quoteIsToken1}
              titleText="流动性分布"
              titlePlacement="left"
              height={220}
            />

            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 10, marginTop: 4 }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 6, fontSize: 11, color: 'var(--text-muted)' }}>
                <span>计价：</span>
                <strong style={{ color: 'var(--text)' }}>
                  {priceRange?.baseSymbol || '--'}/<span style={{ color: 'var(--text-muted)' }}>{priceRange?.quoteSymbol || '--'}</span>
                </strong>
                <button
                  type="button"
                  className="ghost-chip"
                  style={{ padding: '2px 8px', fontSize: 11, minWidth: 0 }}
                  onClick={() => setInvertPrice((v) => !v)}
                  title="切换价格方向"
                >⇄</button>
              </div>
              {liqProfileError ? (
                <div style={{ display: 'flex', gap: 6, alignItems: 'center', fontSize: 11, color: 'var(--text-muted)' }}>
                  <span>{liqProfileError}</span>
                  <button
                    type="button"
                    className="ghost-chip"
                    style={{ padding: '2px 10px', fontSize: 11 }}
                    onClick={reloadLiqProfile}
                    disabled={liqProfileLoading}
                  >重试</button>
                </div>
              ) : null}
            </div>

            <div className="modal-range-section opm-section">
              <div style={{ display: 'grid', gap: 8 }}>
                <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12, alignItems: 'flex-start', flexWrap: 'wrap' }}>
                  <div style={{ display: 'grid', gap: 4 }}>
                    <span className="modal-range-label">区间设置</span>
                  </div>
                  {priceRange?.gridStepPctText && priceRange.gridStepPctText !== '--' ? (
                    <div style={{
                      padding: '5px 10px',
                      borderRadius: 999,
                      border: '1px solid rgba(148, 163, 184, 0.18)',
                      background: 'rgba(15, 23, 42, 0.22)',
                      fontSize: 11,
                      fontWeight: 600,
                      color: 'var(--text-muted)',
                    }}>
                      每格约 {priceRange.gridStepPctText}
                    </div>
                  ) : null}
                </div>

                <div
                  className="opm-range-strip"
                  style={{ gridTemplateColumns: `repeat(${Math.max(quickRangeOptions.length, 1)}, minmax(0, 1fr))` }}
                >
                  {quickRangeOptions.map((option) => {
                    const isActive =
                      Number.isFinite(displayedLowerPct) &&
                      Number.isFinite(displayedUpperPct) &&
                      Math.abs(displayedLowerPct - Number(option.lowerValue)) < 0.05 &&
                      Math.abs(displayedUpperPct - Number(option.upperValue)) < 0.05;
                    const isSmart = String(option.key || '').startsWith('smart-');
                    return (
                      <button
                        key={option.key}
                        type="button"
                        onClick={() => applyRange(option.lowerValue, option.upperValue)}
                        className={`opm-range-chip${isActive ? ' active' : ''}${isSmart ? ' is-smart' : ''}`}
                        title={option.subLabel}
                      >
                        {isSmart ? <span className="opm-range-chip-dot" aria-hidden="true" /> : null}
                        <span className="opm-range-chip-label">{option.label}</span>
                      </button>
                    );
                  })}
                </div>
                {smartRangesLoading ? (
                  <div style={{ fontSize: 11, color: 'var(--text-muted)' }}>聪明钱区间加载中...</div>
                ) : null}

                <div className="opm-custom-percent-card" style={{
                  padding: 8,
                  borderRadius: 10,
                  border: rangeInputMode === 'percentage'
                    ? '1px solid rgba(188, 255, 47, 0.28)'
                    : '1px solid rgba(148, 163, 184, 0.18)',
                  background: rangeInputMode === 'percentage'
                    ? 'rgba(188, 255, 47, 0.06)'
                    : 'rgba(15, 23, 42, 0.14)',
                  display: 'grid',
                  gap: 7,
                }}>
                  <div style={{ display: 'flex', justifyContent: 'space-between', gap: 8, alignItems: 'center', flexWrap: 'wrap' }}>
                    <strong style={{ fontSize: 12, color: 'var(--text)' }}>自定义百分比</strong>
                    <button
                      type="button"
                      className="ghost-chip"
                      style={{
                        minWidth: 0,
                        padding: '3px 8px',
                        fontSize: 11,
                        borderColor: rangeInputMode === 'percentage' ? 'rgba(188, 255, 47, 0.4)' : undefined,
                        color: rangeInputMode === 'percentage' ? 'var(--text)' : undefined,
                        background: rangeInputMode === 'percentage' ? 'rgba(188, 255, 47, 0.1)' : undefined,
                      }}
                      onClick={() => handleRangeInputModeChange('percentage')}
                    >
                      {rangeInputMode === 'percentage' ? '百分比编辑中' : '切换百分比'}
                    </button>
                  </div>
                  <div className="modal-row" style={{ marginTop: 0 }}>
                    <label className="modal-field">
                      <span>下限 %</span>
                      <input
                        type="number"
                        value={rangeLower}
                        onChange={(e) => handleRangeLowerChange(e.target.value)}
                        min="0.1"
                        step="0.1"
                      />
                    </label>
                    <label className="modal-field">
                      <span>上限 %</span>
                      <input
                        type="number"
                        value={rangeUpper}
                        onChange={(e) => handleRangeUpperChange(e.target.value)}
                        min="0.1"
                        step="0.1"
                      />
                    </label>
                  </div>
                </div>

                <div className="opm-price-editor-card" style={{
                  padding: 12,
                  borderRadius: 16,
                  border: '1px solid rgba(148, 163, 184, 0.18)',
                  background: 'rgba(15, 23, 42, 0.18)',
                  display: 'grid',
                  gap: 10,
                }}>
                  <div style={{ display: 'flex', justifyContent: 'space-between', gap: 10, alignItems: 'center', flexWrap: 'wrap' }}>
                    <div style={{ display: 'grid', gap: 4 }}>
                      <span style={{ fontSize: 10, letterSpacing: '0.16em', textTransform: 'uppercase', color: 'var(--text-muted)', fontWeight: 700 }}>Price Range</span>
                      <strong style={{ color: 'var(--text)', fontSize: 14 }}>{priceRange?.baseSymbol || '--'}/{priceRange?.quoteSymbol || '--'}</strong>
                    </div>
                    <button
                      type="button"
                      className="ghost-chip"
                      style={{ minWidth: 0, padding: '3px 9px', fontSize: 11 }}
                      onClick={() => setInvertPrice((v) => !v)}
                    >
                      切换计价
                    </button>
                  </div>

                  <div style={{ display: 'grid', gridTemplateColumns: 'repeat(2, minmax(0, 1fr))', gap: 8 }}>
                    <div style={{
                      padding: 10,
                      borderRadius: 14,
                      border: '1px solid rgba(148, 163, 184, 0.14)',
                      background: 'rgba(255, 255, 255, 0.04)',
                      display: 'grid',
                      gap: 8,
                    }}>
                      <div style={{ display: 'grid', gap: 4 }}>
                        <span style={{ fontSize: 11, color: 'var(--text-muted)' }}>下限价格</span>
                        <div style={{ display: 'flex', alignItems: 'baseline', gap: 6, flexWrap: 'wrap' }}>
                          <strong style={{ color: 'var(--text)', fontSize: 16 }}>{priceRange?.lowerText || '--'}</strong>
                          <span style={{ fontSize: 11, color: 'var(--text-muted)' }}>{priceRange?.lowerPctText || '--'}</span>
                        </div>
                      </div>
                      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(2, minmax(0, 1fr))', gap: 6 }}>
                        <button
                          type="button"
                          className="ghost-chip"
                          style={{ minWidth: 0, padding: '4px 0', fontSize: 11 }}
                          onClick={() => nudgeTickBoundary('lower', -1)}
                        >
                          -1格
                        </button>
                        <button
                          type="button"
                          className="ghost-chip"
                          style={{ minWidth: 0, padding: '4px 0', fontSize: 11 }}
                          onClick={() => nudgeTickBoundary('lower', 1)}
                        >
                          +1格
                        </button>
                      </div>
                    </div>

                    <div style={{
                      padding: 10,
                      borderRadius: 14,
                      border: '1px solid rgba(148, 163, 184, 0.14)',
                      background: 'rgba(255, 255, 255, 0.04)',
                      display: 'grid',
                      gap: 8,
                    }}>
                      <div style={{ display: 'grid', gap: 4 }}>
                        <span style={{ fontSize: 11, color: 'var(--text-muted)' }}>上限价格</span>
                        <div style={{ display: 'flex', alignItems: 'baseline', gap: 6, flexWrap: 'wrap' }}>
                          <strong style={{ color: 'var(--text)', fontSize: 16 }}>{priceRange?.upperText || '--'}</strong>
                          <span style={{ fontSize: 11, color: 'var(--text-muted)' }}>{priceRange?.upperPctText || '--'}</span>
                        </div>
                      </div>
                      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(2, minmax(0, 1fr))', gap: 6 }}>
                        <button
                          type="button"
                          className="ghost-chip"
                          style={{ minWidth: 0, padding: '4px 0', fontSize: 11 }}
                          onClick={() => nudgeTickBoundary('upper', -1)}
                        >
                          -1格
                        </button>
                        <button
                          type="button"
                          className="ghost-chip"
                          style={{ minWidth: 0, padding: '4px 0', fontSize: 11 }}
                          onClick={() => nudgeTickBoundary('upper', 1)}
                        >
                          +1格
                        </button>
                      </div>
                    </div>
                  </div>

                  <div style={{ display: 'grid', gap: 8, fontSize: 11, color: 'var(--text-muted)' }}>
                    <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12 }}>
                      <span>当前价格</span>
                      <strong style={{ color: 'var(--text)' }}>{priceRange?.currentText || '--'}</strong>
                    </div>
                    <div>点击柱子会直接选中该 Tick 区间，拖动两侧边界可继续微调，也支持越过当前活跃格。</div>
                  </div>
                </div>

                <div style={{ display: 'none',
                  padding: 14,
                  borderRadius: 18,
                  border: String(rangeEditor?.position_shape || '').startsWith('single_')
                    ? '1px solid rgba(16, 185, 129, 0.28)'
                    : '1px solid rgba(59, 130, 246, 0.24)',
                  background: String(rangeEditor?.position_shape || '').startsWith('single_')
                    ? 'linear-gradient(135deg, rgba(16, 185, 129, 0.12), rgba(16, 185, 129, 0.04))'
                    : 'linear-gradient(135deg, rgba(59, 130, 246, 0.12), rgba(59, 130, 246, 0.04))',
                  gap: 8,
                }}>
                  <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12, alignItems: 'center', flexWrap: 'wrap' }}>
                    <strong style={{ color: 'var(--text)' }}>
                      {String(rangeEditor?.position_shape || '').startsWith('single_') ? '当前将开单边池' : '当前将开双边池'}
                    </strong>
                    {rangeShapeLabel ? (
                      <span style={{
                        padding: '3px 8px',
                        borderRadius: 999,
                        fontSize: 10,
                        fontWeight: 700,
                        background: 'rgba(255, 255, 255, 0.08)',
                        color: 'var(--text)',
                      }}>
                        {rangeShapeLabel}
                      </span>
                    ) : null}
                  </div>
                  <div style={{ fontSize: 11, color: 'var(--text-muted)', lineHeight: 1.5 }}>
                    {String(rangeEditor?.position_shape || '').startsWith('single_')
                      ? `从 USDT 入场后，最终会更偏向 ${rangeEditor?.dominant_token_symbol || '--'} 单边资产。`
                      : '当前区间覆盖现价，执行时会自动分配到两侧资产。'}
                  </div>
                  <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap' }}>
                    <button
                      type="button"
                      className="ghost-chip"
                      style={{ minWidth: 0, padding: '4px 10px', fontSize: 11 }}
                      onClick={() => shiftRangeToSingleSide('lower')}
                    >
                      单边下限
                    </button>
                    <button
                      type="button"
                      className="ghost-chip"
                      style={{ minWidth: 0, padding: '4px 10px', fontSize: 11 }}
                      onClick={() => shiftRangeToSingleSide('upper')}
                    >
                      单边上限
                    </button>
                  </div>
                  <div style={{ fontSize: 10, color: 'var(--text-muted)' }}>
                    保留当前宽度，整体移动到现价下方或上方，也允许继续越过活跃格微调。
                  </div>
                </div>
              </div>
              <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12, alignItems: 'center', flexWrap: 'wrap' }}>
                <span style={{
                  display: 'inline-flex',
                  alignItems: 'center',
                  padding: '4px 10px',
                  borderRadius: 999,
                  fontSize: 10,
                  fontWeight: 700,
                  border: String(rangeEditor?.position_shape || '').startsWith('single_')
                    ? '1px solid rgba(16, 185, 129, 0.28)'
                    : '1px solid rgba(59, 130, 246, 0.24)',
                  background: String(rangeEditor?.position_shape || '').startsWith('single_')
                    ? 'rgba(16, 185, 129, 0.12)'
                    : 'rgba(59, 130, 246, 0.10)',
                  color: String(rangeEditor?.position_shape || '').startsWith('single_') ? '#34d399' : '#7dd3fc',
                }}>
                  {String(rangeEditor?.position_shape || '').startsWith('single_')
                    ? `当前：${rangeShapeLabel || `单边 ${rangeEditor?.dominant_token_symbol || '--'}`}`
                    : '当前：双边池'}
                </span>
                <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap' }}>
                  <button
                    type="button"
                    className="ghost-chip"
                    style={{ minWidth: 0, padding: '4px 10px', fontSize: 11 }}
                    onClick={() => shiftRangeToSingleSide('lower')}
                  >
                    单边下限
                  </button>
                  <button
                    type="button"
                    className="ghost-chip"
                    style={{ minWidth: 0, padding: '4px 10px', fontSize: 11 }}
                    onClick={() => shiftRangeToSingleSide('upper')}
                  >
                    单边上限
                  </button>
                </div>
              </div>
              <div style={{ display: 'none' }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12, alignItems: 'baseline', flexWrap: 'wrap' }}>
                <span className="modal-range-label">区间设置</span>
                <div style={{ display: 'flex', flexWrap: 'wrap', justifyContent: 'flex-end', gap: 6 }}>
                  {manualRangeInputOptions.map((option) => {
                    const active = rangeInputMode === option.key;
                    return (
                      <button
                        key={option.key}
                        type="button"
                        className="ghost-chip"
                        style={{
                          minWidth: 0,
                          padding: '4px 10px',
                          fontSize: 11,
                          borderColor: active ? 'rgba(188, 255, 47, 0.4)' : undefined,
                          color: active ? 'var(--text)' : undefined,
                          background: active ? 'rgba(188, 255, 47, 0.1)' : undefined,
                        }}
                        onClick={() => handleRangeInputModeChange(option.key)}
                      >
                        {option.label}
                      </button>
                    );
                  })}
                </div>
              </div>
              <div style={{ marginTop: 6, fontSize: 11, color: 'var(--text-muted)' }}>
                {rangeInputMode === 'percentage'
                  ? '拖动图表上的绿/红边界快速调整区间'
                  : (rangeInputMode === 'grid'
                    ? '点图上的柱子可直接选一格，也可以继续拖动或微调 Tick'
                    : '直接输入 Tick，支持当前价外的任意区间')}
              </div>

              {rangeInputMode === 'percentage' ? (
                <>
                  {smartRangesLoading ? (
                    <div className="modal-range-hint">聪明钱区间加载中...</div>
                  ) : null}
                  {visibleSmartRanges.length > 0 ? (
                    <div className="modal-range-picks">
                      {visibleSmartRanges.map((item, index) => {
                        const rangePct = Number(item?.range_percent);
                        const positionCount = Math.max(0, Number(item?.position_count) || 0);
                        const isActive =
                          Math.abs(Number(rangeLower) - rangePct) < 0.05 &&
                          Math.abs(Number(rangeUpper) - rangePct) < 0.05;
                        return (
                          <button
                            key={`${rangePct}-${positionCount}-${index}`}
                            type="button"
                            className={`range-chip smart ${isActive ? 'active' : ''}`}
                            onClick={() => applyRange(rangePct, rangePct)}
                          >
                            <span>{`${rangePct}%${positionCount > 1 ? ` +${positionCount - 1}` : ''}`}</span>
                            <span className="range-chip-sub">{formatUsdCompact(item?.total_amount_usd)}</span>
                          </button>
                        );
                      })}
                    </div>
                  ) : null}
                  <div className="modal-range-picks modal-range-picks-default">
                    {PRESET_RANGES.map((item) => {
                      const isActive =
                        Math.abs(Number(rangeLower) - item) < 0.05 &&
                        Math.abs(Number(rangeUpper) - item) < 0.05;
                      return (
                        <button
                          key={item}
                          type="button"
                          className={`range-chip ${isActive ? 'active' : ''}`}
                          onClick={() => applyRange(item, item)}
                        >
                          {item}%
                        </button>
                      );
                    })}
                  </div>

                  <div className="modal-row">
                    <label className="modal-field">
                      <span>下限 %</span>
                      <input
                        type="number"
                        value={rangeLower}
                        onChange={(e) => handleRangeLowerChange(e.target.value)}
                        min="0.1"
                        step="0.5"
                      />
                    </label>
                    <label className="modal-field">
                      <span>上限 %</span>
                      <input
                        type="number"
                        value={rangeUpper}
                        onChange={(e) => handleRangeUpperChange(e.target.value)}
                        min="0.1"
                        step="0.5"
                      />
                    </label>
                  </div>
                </>
              ) : (
                <>
                  <div style={{
                    marginTop: 12,
                    padding: 12,
                    borderRadius: 14,
                    border: '1px solid rgba(56, 189, 248, 0.2)',
                    background: 'linear-gradient(135deg, rgba(14, 165, 233, 0.10), rgba(14, 165, 233, 0.04))',
                    display: 'grid',
                    gap: 8,
                    fontSize: 11,
                    color: 'var(--text-muted)',
                  }}>
                    <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12 }}>
                      <span>当前 Tick</span>
                      <strong style={{ color: 'var(--text)', fontFamily: 'var(--font-mono, ui-monospace, monospace)' }}>
                        {Number.isFinite(Number(rangeEditor?.current_tick)) ? rangeEditor.current_tick : '--'}
                      </strong>
                    </div>
                    <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12 }}>
                      <span>Tick Spacing</span>
                      <strong style={{ color: 'var(--text)', fontFamily: 'var(--font-mono, ui-monospace, monospace)' }}>
                        {Number.isFinite(Number(rangeEditor?.tick_spacing)) ? rangeEditor.tick_spacing : '--'}
                      </strong>
                    </div>
                    <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12 }}>
                      <span>当前价格</span>
                      <strong style={{ color: 'var(--text)' }}>
                        {formatPriceValue(rangeEditor?.current_price)}
                      </strong>
                    </div>
                    {rangeShapeLabel ? (
                      <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12 }}>
                        <span>仓位形态</span>
                        <strong style={{ color: 'var(--text)' }}>{rangeShapeLabel}</strong>
                      </div>
                    ) : null}
                  </div>

                  <div style={{
                    marginTop: 12,
                    padding: 12,
                    borderRadius: 14,
                    border: String(rangeEditor?.position_shape || '').startsWith('single_')
                      ? '1px solid rgba(16, 185, 129, 0.28)'
                      : '1px solid rgba(59, 130, 246, 0.24)',
                    background: String(rangeEditor?.position_shape || '').startsWith('single_')
                      ? 'linear-gradient(135deg, rgba(16, 185, 129, 0.12), rgba(16, 185, 129, 0.04))'
                      : 'linear-gradient(135deg, rgba(59, 130, 246, 0.12), rgba(59, 130, 246, 0.04))',
                    display: 'grid',
                    gap: 8,
                  }}>
                    <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12, alignItems: 'center' }}>
                      <strong style={{ color: 'var(--text)' }}>
                        {String(rangeEditor?.position_shape || '').startsWith('single_') ? '当前将开单边仓' : '当前将开双边仓'}
                      </strong>
                      {rangeShapeLabel ? (
                        <span style={{
                          padding: '3px 8px',
                          borderRadius: 999,
                          fontSize: 10,
                          fontWeight: 700,
                          background: 'rgba(255, 255, 255, 0.08)',
                          color: 'var(--text)',
                        }}>
                          {rangeShapeLabel}
                        </span>
                      ) : null}
                    </div>
                    <div style={{ fontSize: 11, color: 'var(--text-muted)', lineHeight: 1.5 }}>
                      {String(rangeEditor?.position_shape || '').startsWith('single_')
                        ? `从 USDT 入场后，最终会更偏向 ${rangeEditor?.dominant_token_symbol || '--'} 单边资产。`
                        : '当前区间覆盖现价，执行时会自动分配到两侧资产。'}
                    </div>
                    <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6 }}>
                      <button
                        type="button"
                        className="ghost-chip"
                        style={{ minWidth: 0, padding: '4px 10px', fontSize: 11 }}
                        onClick={() => shiftRangeToSingleSide('lower')}
                      >
                        单边下限
                      </button>
                      <button
                        type="button"
                        className="ghost-chip"
                        style={{ minWidth: 0, padding: '4px 10px', fontSize: 11 }}
                        onClick={() => shiftRangeToSingleSide('upper')}
                      >
                        单边上限
                      </button>
                    </div>
                    <div style={{ fontSize: 10, color: 'var(--text-muted)' }}>
                      保留当前宽度，整体挪到当前价下方或上方，也允许继续越过活跃格调整。
                    </div>
                  </div>

                  {rangeInputMode === 'grid' ? (
                    <>
                      <div style={{ marginTop: 12, display: 'flex', flexWrap: 'wrap', gap: 6 }}>
                        <button
                          type="button"
                          className="ghost-chip"
                          style={{
                            minWidth: 0,
                            padding: '4px 10px',
                            fontSize: 11,
                            borderColor: gridBoundaryTarget === 'lower' ? 'rgba(188, 255, 47, 0.4)' : undefined,
                            color: gridBoundaryTarget === 'lower' ? 'var(--text)' : undefined,
                            background: gridBoundaryTarget === 'lower' ? 'rgba(188, 255, 47, 0.1)' : undefined,
                          }}
                          onClick={() => setGridBoundaryTarget('lower')}
                        >
                          调整下限
                        </button>
                        <button
                          type="button"
                          className="ghost-chip"
                          style={{
                            minWidth: 0,
                            padding: '4px 10px',
                            fontSize: 11,
                            borderColor: gridBoundaryTarget === 'upper' ? 'rgba(188, 255, 47, 0.4)' : undefined,
                            color: gridBoundaryTarget === 'upper' ? 'var(--text)' : undefined,
                            background: gridBoundaryTarget === 'upper' ? 'rgba(188, 255, 47, 0.1)' : undefined,
                          }}
                          onClick={() => setGridBoundaryTarget('upper')}
                        >
                          调整上限
                        </button>
                        <button
                          type="button"
                          className="ghost-chip"
                          style={{ minWidth: 0, padding: '4px 10px', fontSize: 11 }}
                          onClick={() => nudgeTickBoundary(gridBoundaryTarget, -1)}
                        >
                          -1 格
                        </button>
                        <button
                          type="button"
                          className="ghost-chip"
                          style={{ minWidth: 0, padding: '4px 10px', fontSize: 11 }}
                          onClick={() => nudgeTickBoundary(gridBoundaryTarget, 1)}
                        >
                          +1 格
                        </button>
                      </div>

                      <div style={{ marginTop: 10, display: 'flex', flexWrap: 'wrap', gap: 6 }}>
                        {gridBins.map((bin) => {
                          const isSelected =
                            Number.isInteger(tickLowerValue) &&
                            Number.isInteger(tickUpperValue) &&
                            bin.lowerTick >= tickLowerValue &&
                            bin.upperTick <= tickUpperValue;
                          return (
                            <button
                              key={bin.key}
                              type="button"
                              onClick={() => applyGridBin(bin)}
                              style={{
                                minWidth: 88,
                                padding: '8px 10px',
                                borderRadius: 12,
                                border: `1px solid ${isSelected ? 'rgba(188, 255, 47, 0.4)' : 'rgba(148, 163, 184, 0.18)'}`,
                                background: isSelected ? 'rgba(188, 255, 47, 0.10)' : 'rgba(15, 23, 42, 0.22)',
                                color: isSelected ? 'var(--text)' : 'var(--text-muted)',
                                textAlign: 'left',
                                display: 'flex',
                                flexDirection: 'column',
                                gap: 4,
                              }}
                            >
                              <span style={{ fontSize: 11, fontWeight: 600 }}>
                                {bin.isCurrent ? '当前格' : `${bin.lowerTick} ~ ${bin.upperTick}`}
                              </span>
                              <span style={{ fontSize: 10, opacity: 0.72 }}>
                                {bin.isCurrent ? '锚点' : `第 ${Math.abs(bin.index)} 格`}
                              </span>
                            </button>
                          );
                        })}
                      </div>
                    </>
                  ) : null}

                  {rangeInputMode === 'price' ? (
                    <>
                      <div className="modal-row" style={{ marginTop: 12 }}>
                        <label className="modal-field">
                          <span>下限价格</span>
                          <input
                            type="number"
                            value={priceLowerInput}
                            onChange={(e) => {
                              clearErrors();
                              suppressPreview();
                              setPriceLowerInput(e.target.value);
                            }}
                            min="0"
                            step="any"
                          />
                        </label>
                        <label className="modal-field">
                          <span>上限价格</span>
                          <input
                            type="number"
                            value={priceUpperInput}
                            onChange={(e) => {
                              clearErrors();
                              suppressPreview();
                              setPriceUpperInput(e.target.value);
                            }}
                            min="0"
                            step="any"
                          />
                        </label>
                      </div>
                      <div style={{
                        marginTop: 10,
                        padding: 12,
                        borderRadius: 14,
                        border: '1px solid rgba(148, 163, 184, 0.18)',
                        background: 'rgba(15, 23, 42, 0.22)',
                        display: 'grid',
                        gap: 8,
                        fontSize: 11,
                        color: 'var(--text-muted)',
                      }}>
                        <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12 }}>
                          <span>价格计价</span>
                          <strong style={{ color: 'var(--text)' }}>
                            {priceRange?.baseSymbol || '--'}/{priceRange?.quoteSymbol || '--'}
                          </strong>
                        </div>
                      </div>
                    </>
                  ) : (
                  <div className="modal-row" style={{ marginTop: 12 }}>
                    <label className="modal-field">
                      <span>下限 Tick</span>
                      <input
                        type="number"
                        value={tickLowerInput}
                        onChange={(e) => {
                          clearErrors();
                          suppressPreview();
                          setTickLowerInput(e.target.value);
                        }}
                        step="1"
                      />
                    </label>
                    <label className="modal-field">
                      <span>上限 Tick</span>
                      <input
                        type="number"
                        value={tickUpperInput}
                        onChange={(e) => {
                          clearErrors();
                          suppressPreview();
                          setTickUpperInput(e.target.value);
                        }}
                        step="1"
                      />
                    </label>
                  </div>
                  )}

                  {rangeEditor?.range_lower_pct || rangeEditor?.range_upper_pct ? (
                    <div style={{
                      marginTop: 10,
                      padding: 12,
                      borderRadius: 14,
                      border: '1px solid rgba(148, 163, 184, 0.18)',
                      background: 'rgba(15, 23, 42, 0.22)',
                      display: 'grid',
                      gap: 8,
                      fontSize: 11,
                      color: 'var(--text-muted)',
                    }}>
                      <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12 }}>
                        <span>价格区间映射</span>
                        <strong style={{ color: 'var(--text)' }}>
                          {formatPriceValue(rangeEditor?.range_lower_price)} - {formatPriceValue(rangeEditor?.range_upper_price)}
                        </strong>
                      </div>
                      <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12 }}>
                        <span>百分比映射</span>
                        <strong style={{ color: 'var(--text)' }}>
                          {formatRangePercentCompact(rangeEditor?.range_lower_pct)} / {formatRangePercentCompact(rangeEditor?.range_upper_pct)}
                        </strong>
                      </div>
                    </div>
                  ) : null}
                </>
              )}
              </div>

              {false ? (
                <div className="opm-price-range">
                  <div className="opm-price-cell opm-price-cell-lower">
                    <div className="opm-price-label">下限价</div>
                    <div className="opm-price-value">{priceRange.lowerText}</div>
                  </div>
                  <div className="opm-price-cell opm-price-cell-current">
                    <div className="opm-price-label">当前价</div>
                    <div className="opm-price-value">{priceRange.currentText}</div>
                  </div>
                  <div className="opm-price-cell opm-price-cell-upper">
                    <div className="opm-price-label">上限价</div>
                    <div className="opm-price-value">{priceRange.upperText}</div>
                  </div>
                </div>
              ) : null}

              <div className="opm-share-card">
                <div className="opm-share-card-copy" style={{ display: 'grid', gap: 2 }}>
                  <div className="opm-share-label">预计区间资金占比</div>
                  <div className="opm-share-detail" style={{ fontSize: 10, color: 'var(--text-muted)' }}>
                    {shareEstimateInfo?.coverage === 'ok' && Number.isFinite(Number(shareEstimateInfo?.existingLiquidityUsd))
                      ? `按已选区间现有资金 ${formatUsdCompact(shareEstimateInfo.existingLiquidityUsd)} 估算`
                      : (shareEstimateInfo?.coverage === 'out_of_view'
                        ? '已选区间超出当前分布窗口，暂不估算'
                        : (Number(amount) > 0 ? '按已选区间现有资金估算' : ''))}
                  </div>
                </div>
                <div className="opm-share-value">
                  {shareEstimate !== null
                    ? `${shareEstimate < 0.0001 ? shareEstimate.toFixed(6) : shareEstimate.toFixed(4)}%`
                    : (Number(amount) > 0 ? '--' : '--')}
                </div>
              </div>
            </div>
          </div>

          {/* ───────── 右栏：钱包 + 金额 + 滑点 + 入场兑换 + DCA + 风险/错误 ───────── */}
          <div className="opm-right">
            {showPrivateZapProtectionHint ? (
              <div className="modal-info-note opm-section" style={{ display: 'flex', gap: '10px', alignItems: 'flex-start', padding: '14px', borderRadius: '16px', border: '1px solid rgba(16, 185, 129, 0.3)', background: 'linear-gradient(135deg, rgba(16, 185, 129, 0.1), transparent)' }}>
                <div style={{ marginTop: '2px', display: 'flex', alignItems: 'center', justifyContent: 'center', width: '20px', height: '20px', borderRadius: '50%', background: 'rgba(16, 185, 129, 0.2)', color: '#10b981', flexShrink: 0 }}>
                  <Check size={12} strokeWidth={3} />
                </div>
                <div style={{ fontSize: '12px', lineHeight: 1.6, color: 'var(--text-hint, rgba(255, 255, 255, 0.8))' }}>
                  <strong style={{ display: 'block', marginBottom: '4px' }}>私有合约保驾护航</strong>
                  首次开仓时会自动部署与您钱包绑定的专属合约，确保交易更安全私密。如遇网络中断，再次重试即可直接复用，不会重复产生部署消耗。
                </div>
              </div>
            ) : null}

            {riskMessage ? (
              <div
                className={`opm-section opm-liquidity-risk ${riskRequiresAck ? 'is-ack' : 'is-soft'}`}
              >
                <AlertTriangle size={14} />
                <span>{riskMessage}</span>
                {Number.isFinite(riskLiquidityUsd) && riskLiquidityUsd >= 0 ? (
                  <strong>{formatUsdCompact(riskLiquidityUsd)}</strong>
                ) : null}
              </div>
            ) : null}

            {walletsLoading ? (
              <div className="wallet-picker-loading">钱包加载中...</div>
            ) : null}

            {tokenRisk ? (
              <div
                className={`opm-section token-risk-card is-${tokenRiskTone}`}
                title={tokenRiskDetail}
              >
                <AlertTriangle size={14} />
                <strong>{tokenRiskTitle}</strong>
                <span>{tokenRiskSymbol || 'Token'} · OKX 风控 · {tokenRiskDetail}</span>
                <b>等级 {tokenRisk.risk_control_label}</b>
              </div>
            ) : null}

            {showWalletPicker && !walletsLoading ? (
              <div className="wallet-picker opm-section">
                <div className="wallet-picker-header">
                  <span className="wallet-picker-label">钱包</span>
                  <button
                    type="button"
                    className="wallet-visibility-btn"
                    onClick={() => setWalletBalancesHidden((prev) => !prev)}
                    title={walletBalancesHidden ? '显示钱包金额' : '隐藏钱包金额'}
                    aria-label={walletBalancesHidden ? '显示钱包金额' : '隐藏钱包金额'}
                  >
                    {walletBalancesHidden ? <Eye className="wallet-visibility-icon" /> : <EyeOff className="wallet-visibility-icon" />}
                  </button>
                </div>
                <div
                  className="wallet-picker-list"
                  style={{ gridTemplateColumns: `repeat(${Math.min(wallets.length, 3) || 1}, minmax(0, 1fr))` }}
                >
                  {wallets.map((wallet) => {
                    const active = wallet.id === resolvedWalletId;
                    return (
                      <button
                        key={wallet.id}
                        type="button"
                        className={`wallet-chip ${active ? 'active' : ''}`}
                        onClick={() => {
                          clearErrors();
                          onWalletSelect(wallet.id);
                        }}
                      >
                        <span className="wallet-chip-name">
                          {wallet.name || shortAddr(wallet.address)}
                          {wallet.is_default ? <span className="wallet-chip-default">默认</span> : null}
                        </span>
                        <span className="wallet-chip-bal">
                          {wallet.stable_balance !== 'N/A'
                            ? (walletBalancesHidden ? '****' : `$${wallet.stable_balance}`)
                            : ''}
                        </span>
                      </button>
                    );
                  })}
                </div>
              </div>
            ) : null}

            <div className="modal-form opm-section">
              <div className="opm-compact-fields">
                <label className="modal-field">
                  <span>开仓金额 (USDT)</span>
                  <input
                    type="number"
                    value={amount}
                    onChange={(e) => {
                      clearErrors();
                      setAmount(e.target.value);
                    }}
                    min="1"
                    step="10"
                  />
                  <div className="opm-amount-presets" aria-label="快捷开仓金额">
                    {OPEN_POSITION_AMOUNT_PRESETS.map((value) => {
                      const presetValue = String(value);
                      const active = Number(amount) === value;
                      return (
                        <button
                          key={presetValue}
                          type="button"
                          className={`opm-amount-preset${active ? ' active' : ''}`}
                          onClick={() => applyAmountPreset(value)}
                        >
                          {presetValue}
                        </button>
                      );
                    })}
                    <button
                      type="button"
                      className={`opm-amount-preset is-max${Number.isFinite(selectedWalletStableBalance) && Math.abs(Number(amount) - selectedWalletStableBalance) < 0.000001 ? ' active' : ''}`}
                      onClick={() => applyAmountPreset(selectedWalletStableBalance)}
                      disabled={!Number.isFinite(selectedWalletStableBalance)}
                      title={Number.isFinite(selectedWalletStableBalance) ? `使用当前钱包余额 ${formatUSDTValue(selectedWalletStableBalance)} USDT` : '当前钱包余额不可用'}
                    >
                      MAX
                    </button>
                  </div>
                </label>

                <label className="modal-field">
                  <span>任务滑点 %</span>
                  <input
                    type="number"
                    value={slippage}
                    onChange={(e) => {
                      clearErrors();
                      setSlippage(e.target.value);
                    }}
                    min="0"
                    step="0.1"
                    placeholder={String(slippage || '').trim() ? '0.5' : globalSlippageHint}
                  />
                </label>
                {!String(slippage || '').trim() ? (
                  <div style={{ marginTop: 6, fontSize: 11, opacity: 0.58, lineHeight: 1.45 }}>
                    {globalSlippageHint}
                  </div>
                ) : null}
              </div>

            </div>

            {(entrySwapPreviewLoading || entrySwapPreview?.required) ? (
              <div className="modal-info-note opm-section">
                <div style={{ fontWeight: 600, marginBottom: 8 }}>前置兑换</div>
                {entrySwapPreviewLoading ? (
                  <div>正在获取推荐滑点和预计到账数量...</div>
                ) : null}
                {entrySwapPreview?.required ? (
                  <>
                    <div style={{ marginTop: 6 }}>
                      推荐滑点：{formatPercent(entrySwapPreview?.recommended_slippage_tolerance)}
                    </div>
                    <div style={{ marginTop: 4 }}>
                      当前滑点：{formatPercent(entrySwapPreview?.current_slippage_tolerance)}
                    </div>
                    <div style={{ marginTop: 4 }}>
                      预计到账：{entrySwapPreview?.expected_amount_out || '--'} {entrySwapPreview?.to_token_symbol || ''}
                    </div>
                    <div style={{ marginTop: 4 }}>
                      兑换路径：{entrySwapPreview?.amount_in || '--'} {entrySwapPreview?.from_token_symbol || ''} 到 {entrySwapPreview?.to_token_symbol || ''}
                    </div>

                    <label className="modal-field" style={{ marginTop: 12 }}>
                      <span>前置兑换滑点 %</span>
                      <input
                        type="number"
                        value={entrySwapSlippage}
                        onChange={(e) => {
                          clearErrors();
                          setEntrySwapSlippageDirty(true);
                          setEntrySwapSlippage(e.target.value);
                        }}
                        min="0"
                        step="0.1"
                        placeholder="仅作用于本次前置兑换"
                      />
                    </label>
                  </>
                ) : null}
              </div>
                ) : null}

            <div className="opm-section" style={{
              padding: 10,
              borderRadius: 10,
              border: '1px solid rgba(168, 85, 247, 0.18)',
              background: 'rgba(168, 85, 247, 0.06)',
              display: 'grid',
              gap: 7,
            }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', gap: 10, alignItems: 'center', flexWrap: 'wrap' }}>
                <span style={{ fontWeight: 600, fontSize: 12 }}>{'\u672c\u6b21\u5f00\u4ed3'}</span>
                <span style={{ fontSize: 11, color: 'var(--text-muted)' }}>
                  上破 {outOfRangeActions.above} / 下破 {outOfRangeActions.below}
                </span>
              </div>
              <div className="opm-toggle-grid">
                {TASK_MODE_OPTIONS.map((option) => (
                  <button
                    key={option.value}
                    type="button"
                    onClick={() => {
                      clearErrors();
                      setTaskMode(option.value);
                    }}
                    disabled={busy}
                    className={`opm-toggle-btn is-rebalance${taskMode === option.value ? ' active' : ''}`}
                  >
                    <span className="opm-toggle-copy">
                      <span className="opm-toggle-title">{option.label}</span>
                    </span>
                    <span className="opm-toggle-pill">
                      {taskMode === option.value ? '当前' : '选'}
                    </span>
                  </button>
                ))}
              </div>
            </div>

            <div className="opm-section" style={{
              padding: 10,
              borderRadius: 10,
              border: '1px solid rgba(6, 182, 212, 0.25)',
              background: 'rgba(6, 182, 212, 0.06)',
            }}>
              <label style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', cursor: 'pointer' }}>
                <span style={{ fontWeight: 600, fontSize: 13 }}>分批加仓（防插针）</span>
                <span style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
                  <input
                    type="checkbox"
                    checked={dcaEnabled}
                    onChange={(e) => {
                      clearErrors();
                      setDcaEnabled(e.target.checked);
                    }}
                    disabled={busy || isSingleSidedSelection}
                  />
                  <span style={{ fontSize: 12 }}>
                    {isSingleSidedSelection ? '单边池不支持' : (dcaEnabled ? '本次启用' : '本次不启用')}
                  </span>
                </span>
              </label>
              <button
                type="button"
                onClick={() => setDcaExpanded((v) => !v)}
                disabled={busy}
                style={{
                  width: '100%',
                  marginTop: 7,
                  padding: '7px 9px',
                  borderRadius: 9,
                  border: '1px solid rgba(6, 182, 212, 0.18)',
                  background: 'rgba(8, 47, 73, 0.22)',
                  color: 'inherit',
                  display: 'flex',
                  alignItems: 'center',
                  gap: 10,
                  cursor: busy ? 'not-allowed' : 'pointer',
                }}
              >
                <div
                  style={{
                    flex: 1,
                    minWidth: 0,
                    display: 'flex',
                    gap: 6,
                    alignItems: 'center',
                    overflowX: 'auto',
                    whiteSpace: 'nowrap',
                    scrollbarWidth: 'none',
                  }}
                >
                  {dcaEnabled ? (
                    <>
                      {dcaSummaryItems.map((item) => (
                        <span
                          key={item.key}
                          style={{
                            display: 'inline-flex',
                            alignItems: 'center',
                            gap: 4,
                            padding: '3px 8px',
                            borderRadius: 999,
                            border: '1px solid rgba(125, 211, 252, 0.18)',
                            background: 'rgba(255, 255, 255, 0.05)',
                            fontSize: 11,
                            fontWeight: 600,
                          }}
                        >
                          <span style={{ opacity: 0.72 }}>{item.label}</span>
                          <span style={{ color: '#cffafe' }}>{item.amount}</span>
                        </span>
                      ))}
                      <span
                        style={{
                          display: 'inline-flex',
                          alignItems: 'center',
                          padding: '3px 8px',
                          borderRadius: 999,
                          border: '1px solid rgba(103, 232, 249, 0.22)',
                          background: 'rgba(6, 182, 212, 0.12)',
                          fontSize: 11,
                          fontWeight: 700,
                          color: '#a5f3fc',
                        }}
                      >
                        间隔 {formatDCAIntervalHint(dcaInterval)}
                      </span>
                      {effectiveGlobalDcaMinSplitAmount > 0 ? (
                        <span
                          style={{
                            display: 'inline-flex',
                            alignItems: 'center',
                            padding: '3px 8px',
                            borderRadius: 999,
                            border: dcaAmountBelowThreshold ? '1px solid rgba(251, 191, 36, 0.32)' : '1px solid rgba(255, 255, 255, 0.1)',
                            background: dcaAmountBelowThreshold ? 'rgba(245, 158, 11, 0.12)' : 'rgba(255, 255, 255, 0.05)',
                            fontSize: 11,
                            fontWeight: 600,
                            color: dcaAmountBelowThreshold ? '#fde68a' : 'rgba(255, 255, 255, 0.72)',
                          }}
                        >
                          低于 {formatUSDTValue(effectiveGlobalDcaMinSplitAmount)} USDT 不拆分
                        </span>
                      ) : null}
                    </>
                  ) : (
                    <span style={{ fontSize: 11, opacity: 0.78 }}>
                      未启用，开仓将一次性成交。
                    </span>
                  )}
                </div>
                <span style={{ fontSize: 11, fontWeight: 700, color: '#a5f3fc', flexShrink: 0 }}>
                  {dcaExpanded ? '收起 ▲' : '修改 ▾'}
                </span>
              </button>
              {dcaExpanded && isSingleSidedSelection ? (
                <div style={{
                  marginTop: 6,
                  padding: '6px 8px',
                  borderRadius: 8,
                  border: '1px solid rgba(251, 191, 36, 0.28)',
                  background: 'rgba(245, 158, 11, 0.1)',
                  color: '#fde68a',
                  fontSize: 11,
                  lineHeight: 1.5,
                }}>
                  单边池会被策略判定为价格已出区间，所以本次开仓不支持分批加仓。
                </div>
              ) : null}
              {dcaExpanded && dcaEnabled && dcaAmountBelowThreshold ? (
                <div style={{
                  marginTop: 6,
                  padding: '6px 8px',
                  borderRadius: 8,
                  border: '1px solid rgba(251, 191, 36, 0.28)',
                  background: 'rgba(245, 158, 11, 0.1)',
                  color: '#fde68a',
                  fontSize: 11,
                  lineHeight: 1.5,
                }}>
                  当前金额 {formatUSDTValue(amountValue)} USDT 低于阈值，本次提交会按单笔开仓处理。
                </div>
              ) : null}
              {dcaExpanded && dcaEnabled ? (
                <div style={{ marginTop: 8 }}>
                  <div style={{ fontSize: 12, fontWeight: 600, marginBottom: 6 }}>每批占比（共 {dcaPercentages.length} 批）</div>
                  <div style={{ display: 'grid', gap: 5 }}>
                  {dcaPercentages.map((value, idx) => (
                    <div key={idx} style={{ display: 'flex', gap: 6, alignItems: 'center' }}>
                      <span style={{ minWidth: 56, fontSize: 11, opacity: 0.7 }}>
                        {idx === 0 ? '首批' : `第 ${idx + 1} 批`}
                      </span>
                      <input
                        type="number"
                        step="0.1"
                        min="5"
                        max="100"
                        value={value}
                        onChange={(e) => {
                          clearErrors();
                          const next = dcaPercentages.slice();
                          next[idx] = Number(e.target.value) || 0;
                          setDcaPercentages(next);
                        }}
                        disabled={busy}
                        style={{ flex: 1, padding: '3px 8px' }}
                      />
                      <span style={{ fontSize: 11, opacity: 0.6 }}>%</span>
                      {dcaPercentages.length > 2 ? (
                        <button
                          type="button"
                          className="ghost-chip"
                          onClick={() => {
                            clearErrors();
                            setDcaPercentages(dcaPercentages.filter((_, i) => i !== idx));
                          }}
                          disabled={busy}
                        >
                          ×
                        </button>
                      ) : null}
                    </div>
                  ))}
                  </div>
                  <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginTop: 8, fontSize: 11 }}>
                    <span style={{ color: dcaSumValid ? '#10b981' : '#f59e0b', fontWeight: 600 }}>
                      合计：{dcaSum.toFixed(2)}% {dcaSumValid ? '✓' : '（必须等于 100%）'}
                    </span>
                    <span style={{ display: 'flex', gap: 6 }}>
                      <button
                        type="button"
                        className="ghost-chip"
                        onClick={() => {
                          clearErrors();
                          const n = dcaPercentages.length || 2;
                          const base = Math.floor((100 / n) * 100) / 100;
                          const next = Array(n).fill(base);
                          next[n - 1] = Math.round((100 - base * (n - 1)) * 100) / 100;
                          setDcaPercentages(next);
                        }}
                        disabled={busy}
                      >
                        平均分配
                      </button>
                      <button
                        type="button"
                        className="ghost-chip"
                        onClick={() => {
                          if (dcaPercentages.length >= 5) return;
                          clearErrors();
                          const n = dcaPercentages.length + 1;
                          const base = Math.floor((100 / n) * 100) / 100;
                          const next = Array(n).fill(base);
                          next[n - 1] = Math.round((100 - base * (n - 1)) * 100) / 100;
                          setDcaPercentages(next);
                        }}
                        disabled={busy || dcaPercentages.length >= 5}
                      >
                        ＋ 追加批次
                      </button>
                    </span>
                  </div>
                  <div style={{ marginTop: 10, display: 'flex', gap: 8, alignItems: 'center' }}>
                    <span style={{ fontSize: 12, fontWeight: 600, minWidth: 80 }}>批次间隔</span>
                    <input
                      type="number"
                      step="0.001"
                      min="0"
                      max="300"
                      value={dcaInterval}
                      onChange={(e) => {
                        clearErrors();
                        setDcaInterval(Number(e.target.value) || 0);
                      }}
                      disabled={busy}
                      style={{ flex: 1, padding: '4px 8px' }}
                    />
                    <span style={{ fontSize: 11, opacity: 0.6 }}>秒 (0–300)</span>
                  </div>
                  <div style={{ marginTop: 4, fontSize: 11, opacity: 0.6 }}>
                    支持小数秒，0.3 = 300ms。
                  </div>
                </div>
              ) : null}
            </div>

            {visibleError ? (
              <div className="opm-section" style={{
                padding: 16,
                borderRadius: 16,
                border: '1px solid rgba(239, 68, 68, 0.4)',
                background: 'linear-gradient(135deg, rgba(239, 68, 68, 0.1), rgba(239, 68, 68, 0.05))',
                color: 'var(--text-error, #fca5a5)',
                display: 'flex',
                gap: 12,
                alignItems: 'flex-start',
                boxShadow: '0 1px 2px rgba(0,0,0,0.05)',
              }}>
                <div style={{ marginTop: 2, display: 'flex', alignItems: 'center', justifyContent: 'center', width: '20px', height: '20px', borderRadius: '50%', backgroundColor: 'rgba(239, 68, 68, 0.2)', color: '#ef4444', flexShrink: 0 }}>
                  <X size={12} strokeWidth={3} />
                </div>
                <div style={{ fontSize: 12, fontWeight: 500, lineHeight: 1.5 }}>{visibleError}</div>
              </div>
            ) : null}
          </div>
        </div>

        <div className="modal-actions">
          <button type="button" className="ghost-chip" onClick={onClose} disabled={busy}>取消</button>
          <button
            type="button"
            className={`accent-btn ${hasBlockingSafetyFailure ? 'is-blocked' : ''}`}
            onClick={handleSubmit}
            disabled={submitDisabled}
            title={submitDisabledReason || undefined}
          >
            {submitButtonLabel}
          </button>
        </div>
      </div>
    </div>
  );
}
