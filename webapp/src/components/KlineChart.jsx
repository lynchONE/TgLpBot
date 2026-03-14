import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { createChart, CandlestickSeries, HistogramSeries } from 'lightweight-charts';
import { formatUtc8DateTime, formatUtc8TickMark, shortAddress, toUnixSeconds } from '../utils';

/* ── Wallet avatar images ── */
const avatarModules = import.meta.glob('../icon/avatar_*.png', { eager: true, import: 'default' });
const AVATAR_URLS = Object.values(avatarModules);

function walletAvatarIndex(address) {
  let h = 0;
  const s = String(address || '').toLowerCase();
  for (let i = 0; i < s.length; i++) h = ((h << 5) - h + s.charCodeAt(i)) | 0;
  return Math.abs(h) % (AVATAR_URLS.length || 1);
}

function walletAvatarUrl(address) {
  return AVATAR_URLS[walletAvatarIndex(address)] || AVATAR_URLS[0];
}

function getClusterAvatarUrl(cluster) {
  const items = Array.isArray(cluster?.items) ? cluster.items : [];
  const addr = items[0]?.wallet_address || '';
  return walletAvatarUrl(addr);
}

function normalizeWalletAddress(value) {
  const raw = String(value || '').trim();
  if (!/^0x[0-9a-fA-F]{40}$/.test(raw)) return '';
  return `0x${raw.slice(2).toLowerCase()}`;
}

function isClusterHighlighted(cluster, walletAddress) {
  const target = normalizeWalletAddress(walletAddress);
  if (!target) return false;
  return (Array.isArray(cluster?.items) ? cluster.items : []).some(
    (item) => normalizeWalletAddress(item?.wallet_address) === target
  );
}

function tooltipSafeLabel(value) {
  return String(value || '').trim() || '--';
}

function formatUSD(v) {
  if (!Number.isFinite(v) || v === 0) return '$0';
  if (v >= 1e6) return '$' + (v / 1e6).toFixed(1) + 'M';
  if (v >= 1e3) return '$' + (v / 1e3).toFixed(1) + 'K';
  return '$' + v.toFixed(v >= 100 ? 0 : 2);
}

function deOverlapMarkers(markers) {
  const step = 36;
  const xThreshold = 28;
  const result = [];
  for (const m of markers) {
    let { y } = m;
    let collision = true;
    let iter = 0;
    while (collision && iter < 10) {
      collision = false;
      for (const p of result) {
        if (Math.abs(p.x - m.x) < xThreshold && Math.abs(y - p.y) < step) {
          y = m.action === 'remove' ? p.y + step : p.y - step;
          collision = true;
          break;
        }
      }
      iter++;
    }
    result.push({ ...m, y });
  }
  return result;
}

function findNearestCandle(candleData, candleMap, targetTime) {
  const target = Number(targetTime || 0);
  if (!target || !candleData.length) return null;

  const direct = candleMap.get(target);
  if (direct) {
    return { candle: direct, time: target };
  }

  const firstTime = Number(candleData[0]?.time || 0);
  const lastTime = Number(candleData[candleData.length - 1]?.time || 0);
  if (!firstTime || !lastTime || target < firstTime || target > lastTime) {
    return null;
  }

  let low = 0;
  let high = candleData.length - 1;
  while (low <= high) {
    const mid = Math.floor((low + high) / 2);
    const midTime = Number(candleData[mid]?.time || 0);
    if (midTime === target) {
      return { candle: candleData[mid], time: midTime };
    }
    if (midTime < target) low = mid + 1;
    else high = mid - 1;
  }

  const prev = high >= 0 ? candleData[high] : null;
  const next = low < candleData.length ? candleData[low] : null;
  const prevTime = Number(prev?.time || 0);
  const nextTime = Number(next?.time || 0);

  if (!prev && !next) return null;
  if (!prev) return next ? { candle: next, time: nextTime } : null;
  if (!next) return prev ? { candle: prev, time: prevTime } : null;

  return Math.abs(target - prevTime) <= Math.abs(nextTime - target)
    ? { candle: prev, time: prevTime }
    : { candle: next, time: nextTime };
}

const SUBSCRIPT_DIGITS = ['₀', '₁', '₂', '₃', '₄', '₅', '₆', '₇', '₈', '₉'];
function toSubscript(n) {
  return String(n).split('').map((d) => SUBSCRIPT_DIGITS[Number(d)]).join('');
}

function smartPriceFormatter(price) {
  if (!Number.isFinite(price) || price === 0) return '0';
  const abs = Math.abs(price);
  const sign = price < 0 ? '-' : '';

  if (abs >= 1e4) return sign + abs.toFixed(2);
  if (abs >= 1) return sign + abs.toFixed(4);
  if (abs >= 0.001) return sign + abs.toFixed(6);

  // Very small prices: subscript-zero notation
  // e.g. 0.00000001234 → 0.0₇1234
  const exp = Math.floor(Math.log10(abs));
  let zeros = -exp - 1;
  let sig = Math.round(abs * Math.pow(10, -exp + 3));
  if (sig >= 10000) {
    zeros = Math.max(1, zeros - 1);
    sig = Math.round(sig / 10);
  }
  return sign + '0.0' + toSubscript(zeros) + sig;
}

function computeMinMove(candleData) {
  let minPrice = Number.POSITIVE_INFINITY;
  for (const row of candleData) {
    if (row.low > 0) minPrice = Math.min(minPrice, row.low);
    if (row.close > 0) minPrice = Math.min(minPrice, row.close);
  }
  if (!Number.isFinite(minPrice) || minPrice <= 0) return 0.01;
  const exp = Math.floor(Math.log10(minPrice)) - 4;
  return Math.pow(10, Math.min(-2, exp));
}

function clamp(value, min, max) {
  if (!Number.isFinite(value)) return min;
  return Math.min(Math.max(value, min), max);
}

function projectClusters(chart, candleSeries, candleData, candleMap, candleIndexMap, clusters, hostWidth, hostHeight, userAvatarUrl) {
  if (!chart || !candleSeries || !clusters.length) return [];
  const timeScale = chart.timeScale();
  const projected = [];
  const width = Math.max(0, Number(hostWidth || 0));
  const height = Math.max(0, Number(hostHeight || 0));
  const xPad = 18;
  const yPad = 18;
  const priceTop = 12;
  const priceBottom = height > 0 ? Math.max(priceTop + 32, Math.floor(height * 0.78)) : 0;
  const minPrice = candleData.reduce((acc, row) => Math.min(acc, Number(row?.low || 0)), Number.POSITIVE_INFINITY);
  const maxPrice = candleData.reduce((acc, row) => Math.max(acc, Number(row?.high || 0)), 0);
  const priceSpan = Number.isFinite(maxPrice - minPrice) && maxPrice > minPrice ? (maxPrice - minPrice) : 1;
  const visibleRange = timeScale.getVisibleRange?.() || null;
  const visibleFrom = visibleRange ? toUnixSeconds(visibleRange.from) : 0;
  const visibleTo = visibleRange ? toUnixSeconds(visibleRange.to) : 0;
  const hasVisibleRange = visibleFrom > 0 && visibleTo > 0;
  const edgeBuffer = 24;

  for (const cluster of clusters) {
    const located =
      findNearestCandle(candleData, candleMap, cluster.time) ||
      findNearestCandle(candleData, candleMap, cluster.items?.[0]?.bucket_t) ||
      findNearestCandle(candleData, candleMap, cluster.items?.[0]?.t);
    if (!located?.candle) continue;
    const candle = located.candle;

    const time = Number(located.time || cluster.time || 0);
    if (hasVisibleRange && (time < visibleFrom || time > visibleTo)) continue;

    let x = timeScale.timeToCoordinate(time);
    if (!Number.isFinite(x)) continue;
    if (width > 0 && (x < -edgeBuffer || x > width + edgeBuffer)) continue;

    const anchorPrice = cluster.action === 'remove'
      ? Number(candle?.l ?? candle?.low ?? 0)
      : Number(candle?.h ?? candle?.high ?? 0);
    let y = candleSeries.priceToCoordinate(anchorPrice);
    if (!Number.isFinite(y) && height > 0) {
      const normalized = (maxPrice - anchorPrice) / priceSpan;
      y = priceTop + normalized * Math.max(1, priceBottom - priceTop);
    }
    if (!Number.isFinite(y)) continue;

    const offset = cluster.action === 'remove' ? 18 : -18;
    const minY = yPad;
    const maxY = height > 0 ? Math.max(minY, priceBottom - yPad) : Number.POSITIVE_INFINITY;

    projected.push({
      ...cluster,
      x: clamp(x, xPad, Math.max(xPad, width - xPad)),
      y: clamp(y + offset, minY, maxY),
      label: cluster.isMyTrade && userAvatarUrl ? userAvatarUrl : getClusterAvatarUrl(cluster),
    });
  }
  return projected;
}

function projectRangeOverlays(candleSeries, overlays, hostWidth, hostHeight) {
  if (!candleSeries || !Array.isArray(overlays) || overlays.length === 0) return [];
  const width = Math.max(0, Number(hostWidth || 0));
  const height = Math.max(0, Number(hostHeight || 0));
  const out = [];

  overlays.forEach((overlay) => {
    const type = String(overlay?.type || '').trim().toLowerCase();
    if (type === 'range') {
      const lower = Number(overlay?.priceLower || 0);
      const upper = Number(overlay?.priceUpper || 0);
      if (!Number.isFinite(lower) || lower <= 0 || !Number.isFinite(upper) || upper <= 0) return;
      const lowerYRaw = candleSeries.priceToCoordinate(lower);
      const upperYRaw = candleSeries.priceToCoordinate(upper);
      if (!Number.isFinite(lowerYRaw) && !Number.isFinite(upperYRaw)) return;
      const lowerY = Number.isFinite(lowerYRaw) ? lowerYRaw : upperYRaw;
      const upperY = Number.isFinite(upperYRaw) ? upperYRaw : lowerYRaw;
      const topY = Math.min(lowerY, upperY);
      const bottomY = Math.max(lowerY, upperY);
      out.push({
        id: overlay.id,
        type: 'range',
        color: String(overlay?.color || 'red'),
        label: String(overlay?.label || ''),
        avatarUrl: String(overlay?.avatarUrl || ''),
        priceLower: Math.min(lower, upper),
        priceUpper: Math.max(lower, upper),
        topY,
        bottomY,
        midY: Number.isFinite(topY) && Number.isFinite(bottomY) ? (topY + bottomY) / 2 : 0,
        width,
        height,
      });
      return;
    }

    const price = Number(overlay?.price || 0);
    if (!Number.isFinite(price) || price <= 0) return;
    const y = candleSeries.priceToCoordinate(price);
    if (!Number.isFinite(y)) return;
    out.push({
      id: overlay.id,
      type: 'mid',
      color: String(overlay?.color || 'blue'),
      label: String(overlay?.label || ''),
      y,
      width,
      height,
    });
  });

  return out;
}

export default function KlineChart({
  candles,
  markers,
  rangeOverlays = [],
  loading = false,
  error = '',
  onMarkerClick,
  onVisibleRangeChange,
  activeMarkerId = '',
  highlightWalletAddress = '',
  watchedWalletSet = new Set(),
  watchToggleMap = {},
  onToggleWatch,
  viewportKey = '',
  userAvatarUrl = '',
}) {
  const wrapRef = useRef(null);
  const chartHostRef = useRef(null);
  const chartRef = useRef(null);
  const candleSeriesRef = useRef(null);
  const volumeSeriesRef = useRef(null);
  const prevViewportKeyRef = useRef('');
  const lastVisibleRangeRef = useRef({ from: 0, to: 0 });
  const [projectedMarkers, setProjectedMarkers] = useState([]);
  const [projectedRangeOverlays, setProjectedRangeOverlays] = useState([]);
  const [hoveredCluster, setHoveredCluster] = useState(null);
  const updateProjectionRef = useRef(null);
  const visibleRangeHandlerRef = useRef(onVisibleRangeChange);
  const tooltipHideTimerRef = useRef(null);

  visibleRangeHandlerRef.current = onVisibleRangeChange;

  const candleData = useMemo(() => {
    const rows = Array.isArray(candles) ? candles : [];
    const out = [];
    for (const row of rows) {
      const time = toUnixSeconds(row?.t);
      const open = Number(row?.o);
      const high = Number(row?.h);
      const low = Number(row?.l);
      const close = Number(row?.c);
      const volume = Number(row?.v);
      if (
        !time ||
        !Number.isFinite(open) ||
        !Number.isFinite(high) ||
        !Number.isFinite(low) ||
        !Number.isFinite(close)
      ) {
        continue;
      }
      out.push({
        time,
        open,
        high,
        low,
        close,
        volume: Number.isFinite(volume) ? volume : 0,
      });
    }
    return out;
  }, [candles]);

  const candleMap = useMemo(() => {
    const map = new Map();
    candleData.forEach((row) => map.set(row.time, row));
    return map;
  }, [candleData]);

  const candleIndexMap = useMemo(() => {
    const map = new Map();
    candleData.forEach((row, index) => map.set(row.time, index));
    return map;
  }, [candleData]);

  const markerClusters = useMemo(() => {
    const rows = Array.isArray(markers) ? markers : [];
    const grouped = new Map();
    rows.forEach((row) => {
      const action = String(row?.action || 'add').toLowerCase() === 'remove' ? 'remove' : 'add';
      const time = toUnixSeconds(row?.bucket_t || row?.t);
      if (!time) return;
      const isMyTrade = Boolean(row?.is_my_trade);
      const key = isMyTrade ? `my:${time}:${action}` : `${time}:${action}`;
      const prev = grouped.get(key) || {
        id: key,
        time,
        action,
        items: [],
        estimatedUSD: 0,
        isMyTrade,
      };
      prev.items.push(row);
      prev.estimatedUSD += Number(row?.estimated_usd || 0);
      grouped.set(key, prev);
    });
    return Array.from(grouped.values())
      .map((cluster) => ({
        ...cluster,
        items: [...cluster.items].sort((a, b) => Number(b?.estimated_usd || 0) - Number(a?.estimated_usd || 0)),
      }))
      .sort((a, b) => a.time - b.time);
  }, [markers]);

  const displayMarkers = useMemo(
    () => deOverlapMarkers(projectedMarkers),
    [projectedMarkers]
  );

  useEffect(() => {
    setHoveredCluster((prev) => {
      if (!prev) return null;
      const next = displayMarkers.find((item) => item.id === prev.id);
      return next || null;
    });
  }, [displayMarkers]);

  const clearTooltipHideTimer = useCallback(() => {
    if (!tooltipHideTimerRef.current) return;
    window.clearTimeout(tooltipHideTimerRef.current);
    tooltipHideTimerRef.current = null;
  }, []);

  const scheduleTooltipHide = useCallback(() => {
    clearTooltipHideTimer();
    tooltipHideTimerRef.current = window.setTimeout(() => {
      setHoveredCluster(null);
      tooltipHideTimerRef.current = null;
    }, 180);
  }, [clearTooltipHideTimer]);

  const updateProjection = useCallback(() => {
    const hostWidth = chartHostRef.current?.clientWidth || wrapRef.current?.clientWidth || 0;
    const hostHeight = chartHostRef.current?.clientHeight || wrapRef.current?.clientHeight || 0;
    setProjectedMarkers(
      projectClusters(
        chartRef.current,
        candleSeriesRef.current,
        candleData,
        candleMap,
        candleIndexMap,
        markerClusters,
        hostWidth,
        hostHeight,
        userAvatarUrl
      )
    );
    setProjectedRangeOverlays(
      projectRangeOverlays(
        candleSeriesRef.current,
        rangeOverlays,
        hostWidth,
        hostHeight
      )
    );
  }, [candleData, candleMap, candleIndexMap, markerClusters, rangeOverlays, userAvatarUrl]);

  updateProjectionRef.current = updateProjection;

  const emitVisibleRange = useCallback(() => {
    const chart = chartRef.current;
    if (!chart) return;
    const raw = chart.timeScale().getVisibleRange?.() || null;
    let from = raw ? toUnixSeconds(raw.from) : 0;
    let to = raw ? toUnixSeconds(raw.to) : 0;
    if ((!from || !to) && candleData.length) {
      from = Number(candleData[0]?.time || 0);
      to = Number(candleData[candleData.length - 1]?.time || 0);
    }
    if (!from || !to) return;
    if (to < from) {
      const tmp = from;
      from = to;
      to = tmp;
    }
    const prev = lastVisibleRangeRef.current;
    if (prev.from === from && prev.to === to) return;
    lastVisibleRangeRef.current = { from, to };
    visibleRangeHandlerRef.current?.({ from, to });
  }, [candleData]);

  useEffect(() => {
    if (!chartHostRef.current) return;
    const host = chartHostRef.current;

    const chart = createChart(host, {
      width: Math.max(260, host.clientWidth),
      height: Math.max(360, host.clientHeight || 360),
      layout: {
        background: { color: '#0b1018' },
        textColor: '#95a0b5',
      },
      localization: {
        locale: 'zh-CN',
        timeFormatter: (time) => formatUtc8DateTime(time),
      },
      grid: {
        vertLines: { color: 'rgba(122, 142, 173, 0.12)' },
        horzLines: { color: 'rgba(122, 142, 173, 0.12)' },
      },
      rightPriceScale: { borderColor: 'rgba(122, 142, 173, 0.22)' },
      timeScale: {
        borderColor: 'rgba(122, 142, 173, 0.22)',
        timeVisible: true,
        secondsVisible: false,
        tickMarkFormatter: (time, tickMarkType) => formatUtc8TickMark(time, tickMarkType),
      },
      crosshair: {
        vertLine: { color: 'rgba(255, 183, 59, 0.35)' },
        horzLine: { color: 'rgba(255, 183, 59, 0.35)' },
      },
      handleScroll: true,
      handleScale: true,
    });

    const candleSeries = chart.addSeries(CandlestickSeries, {
      upColor: '#16c784',
      downColor: '#ea3943',
      borderUpColor: '#16c784',
      borderDownColor: '#ea3943',
      wickUpColor: '#16c784',
      wickDownColor: '#ea3943',
      priceLineVisible: true,
      lastValueVisible: true,
    });
    const volumeSeries = chart.addSeries(HistogramSeries, {
      priceFormat: { type: 'volume' },
      priceScaleId: '',
    });
    volumeSeries.priceScale().applyOptions({
      scaleMargins: { top: 0.76, bottom: 0 },
    });

    chartRef.current = chart;
    candleSeriesRef.current = candleSeries;
    volumeSeriesRef.current = volumeSeries;

    const onVisibleChange = () => {
      window.requestAnimationFrame(() => {
        updateProjectionRef.current?.();
        emitVisibleRange();
      });
    };
    chart.timeScale().subscribeVisibleTimeRangeChange(onVisibleChange);

    const observer = new ResizeObserver((entries) => {
      const box = entries?.[0]?.contentRect;
      if (!box) return;
      chart.applyOptions({
        width: Math.max(260, Math.floor(box.width)),
        height: Math.max(360, Math.floor(box.height)),
      });
      window.requestAnimationFrame(() => updateProjectionRef.current?.());
    });
    observer.observe(host);

    return () => {
      if (tooltipHideTimerRef.current) {
        window.clearTimeout(tooltipHideTimerRef.current);
        tooltipHideTimerRef.current = null;
      }
      observer.disconnect();
      chart.timeScale().unsubscribeVisibleTimeRangeChange(onVisibleChange);
      chart.remove();
      chartRef.current = null;
      candleSeriesRef.current = null;
      volumeSeriesRef.current = null;
    };
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    if (!candleSeriesRef.current || !volumeSeriesRef.current) return;
    const volumeData = candleData.map((row) => ({
      time: row.time,
      value: row.volume,
      color: row.close >= row.open ? 'rgba(22,199,132,0.45)' : 'rgba(234,57,67,0.45)',
    }));

    candleSeriesRef.current.applyOptions({
      priceFormat: {
        type: 'custom',
        formatter: smartPriceFormatter,
        minMove: computeMinMove(candleData),
      },
    });
    candleSeriesRef.current.setData(candleData);
    volumeSeriesRef.current.setData(volumeData);
    window.requestAnimationFrame(() => {
      updateProjectionRef.current?.();
      emitVisibleRange();
    });
  }, [candleData, emitVisibleRange]);

  useEffect(() => {
    if (prevViewportKeyRef.current === viewportKey) return;
    prevViewportKeyRef.current = viewportKey;
    chartRef.current.timeScale().fitContent();
    lastVisibleRangeRef.current = { from: 0, to: 0 };
    window.requestAnimationFrame(() => {
      updateProjectionRef.current?.();
      emitVisibleRange();
    });
  }, [viewportKey, candleData.length, emitVisibleRange]);

  useEffect(() => {
    window.requestAnimationFrame(updateProjection);
  }, [updateProjection]);

  const empty = !loading && !error && candleData.length === 0;

  const tooltipData = useMemo(() => {
    if (!hoveredCluster) return null;
    const c = hoveredCluster;
    const primary = c.items?.[0];
    if (!primary) return null;
    const walletAddress = normalizeWalletAddress(primary.wallet_address || '');
    const walletName = c.isMyTrade ? '我的交易' : (String(primary.wallet_label || '').trim() || shortAddress(primary.wallet_address || '', 6, 4));
    const lower = Number(primary.price_lower || 0);
    const upper = Number(primary.price_upper || 0);
    const hasRange = lower > 0 && upper > 0;
    const rangePct = hasRange ? `±${(((upper - lower) / (upper + lower)) * 100).toFixed(1)}%` : '';
    return {
      walletAddress,
      walletName,
      lower,
      upper,
      hasRange,
      rangePct,
      totalUSD: c.estimatedUSD,
      count: c.items.length,
      isMyTrade: c.isMyTrade,
      watched: walletAddress ? watchedWalletSet.has(walletAddress) : false,
      watchBusy: walletAddress ? Boolean(watchToggleMap[walletAddress]) : false,
    };
  }, [hoveredCluster, watchToggleMap, watchedWalletSet]);

  return (
    <div className="kline-native-wrap" ref={wrapRef}>
      <div className="kline-native-stage" ref={chartHostRef} />

      <div className="kline-marker-layer">
        {projectedRangeOverlays.map((overlay) => {
          if (overlay.type === 'range') {
            return (
              <React.Fragment key={overlay.id}>
                <div
                  className={`kline-range-line ${overlay.color} top`}
                  style={{ top: `${overlay.topY}px` }}
                >
                  <span className="kline-range-price">{smartPriceFormatter(overlay.priceUpper)}</span>
                </div>
                <div
                  className={`kline-range-line ${overlay.color} bottom`}
                  style={{ top: `${overlay.bottomY}px` }}
                >
                  <span className="kline-range-price">{smartPriceFormatter(overlay.priceLower)}</span>
                </div>
                <div
                  className={`kline-range-avatar ${overlay.color}`}
                  style={{ top: `${overlay.midY}px` }}
                >
                  {overlay.avatarUrl ? <img src={overlay.avatarUrl} alt="" /> : null}
                </div>
              </React.Fragment>
            );
          }
          return (
            <div
              key={overlay.id}
              className={`kline-mid-line ${overlay.color}`}
              style={{ top: `${overlay.y}px` }}
            >
              <span className="kline-mid-line-label">{tooltipSafeLabel(overlay.label)}</span>
            </div>
          );
        })}

        {displayMarkers.map((cluster) => (
          <button
            key={cluster.id}
            type="button"
            className={`kline-marker ${cluster.action} ${cluster.isMyTrade ? 'my-trade' : ''} ${activeMarkerId === cluster.id ? 'active' : ''} ${isClusterHighlighted(cluster, highlightWalletAddress) ? 'wallet-highlighted' : ''}`}
            style={{
              left: `${cluster.x}px`,
              top: `${cluster.y}px`,
            }}
            onClick={() => onMarkerClick?.(cluster)}
            onMouseEnter={() => {
              clearTooltipHideTimer();
              setHoveredCluster(cluster);
            }}
            onMouseLeave={scheduleTooltipHide}
          >
            <img className="kline-marker-avatar" src={cluster.label} alt="" />
          </button>
        ))}

        {hoveredCluster && tooltipData && (
          <div
            className={`kline-marker-tooltip ${hoveredCluster.action}`}
            style={{
              left: `${hoveredCluster.x}px`,
              top: `${hoveredCluster.y}px`,
            }}
            onMouseEnter={clearTooltipHideTimer}
            onMouseLeave={scheduleTooltipHide}
          >
            <div className="kmt-head">
              <span className="kmt-emoji"><img src={hoveredCluster.label} alt="" /></span>
              <span className="kmt-wallet">{tooltipData.walletName}</span>
              {!tooltipData.isMyTrade && tooltipData.walletAddress ? (
                <button
                  type="button"
                  className={`kmt-watch-btn ${tooltipData.watched ? 'active' : ''}`}
                  disabled={tooltipData.watchBusy}
                  onClick={() => onToggleWatch?.(tooltipData.walletAddress, tooltipData.walletName)}
                  aria-label={tooltipData.watched ? '取消特别关注' : '加入特别关注'}
                  title={tooltipData.watched ? '取消特别关注' : '加入特别关注'}
                >
                  {tooltipData.watched ? '\u2665' : '\u2661'}
                </button>
              ) : null}
              {tooltipData.count > 1 && <span className="kmt-count">等{tooltipData.count}笔</span>}
            </div>
            <div className="kmt-row">
              <span className={`kmt-action ${hoveredCluster.action}`}>
                {hoveredCluster.action === 'remove' ? '减仓' : '加仓'}
              </span>
              <span className="kmt-usd">{formatUSD(tooltipData.totalUSD)}</span>
            </div>
            {tooltipData.hasRange && (
              <div className="kmt-row">
                <span className="kmt-range">
                  {smartPriceFormatter(tooltipData.lower)} → {smartPriceFormatter(tooltipData.upper)}
                </span>
                <span className="kmt-pct">{tooltipData.rangePct}</span>
              </div>
            )}
          </div>
        )}
      </div>

      {loading ? <div className="kline-state-overlay">加载 K 线中...</div> : null}
      {!loading && error ? <div className="kline-state-overlay error">{error}</div> : null}
      {empty ? <div className="kline-state-overlay">暂无 K 线数据</div> : null}
    </div>
  );
}
