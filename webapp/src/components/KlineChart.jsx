import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { createChart, CandlestickSeries, HistogramSeries } from 'lightweight-charts';
import { Check, Copy, Pencil, X } from 'lucide-react';
import { resolveSMAvatarAssetUrl } from '../smartMoneyApi';
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

function markerWalletAvatarUrl(marker) {
  const uploaded = resolveSMAvatarAssetUrl(marker?.wallet_avatar_url);
  if (uploaded) return uploaded;
  return walletAvatarUrl(marker?.wallet_address || '');
}

function getClusterAvatarUrl(cluster) {
  const items = Array.isArray(cluster?.items) ? cluster.items : [];
  return markerWalletAvatarUrl(items[0]);
}

function normalizeWalletAddress(value) {
  const raw = String(value || '').trim();
  if (!/^0x[0-9a-fA-F]{40}$/.test(raw)) return '';
  return `0x${raw.slice(2).toLowerCase()}`;
}

function walletTailLabel(value) {
  const address = normalizeWalletAddress(value);
  return address ? address.slice(-4) : '';
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

function formatSignedUSD(value) {
  const num = Number(value);
  if (!Number.isFinite(num) || num === 0) return '$0';
  return `${num > 0 ? '+' : '-'}${formatUSD(Math.abs(num))}`;
}

function formatSignedPercent(value) {
  const num = Number(value);
  if (!Number.isFinite(num)) return '';
  if (num === 0) return '0%';
  if (Math.abs(num) >= 100) return `${num > 0 ? '+' : '-'}${Math.abs(num).toFixed(0)}%`;
  if (Math.abs(num) >= 10) return `${num > 0 ? '+' : '-'}${Math.abs(num).toFixed(1).replace(/\.0$/, '')}%`;
  return `${num > 0 ? '+' : '-'}${Math.abs(num).toFixed(2).replace(/0+$/, '').replace(/\.$/, '')}%`;
}

function formatRangePercent(value) {
  const num = Number(value);
  if (!Number.isFinite(num) || num <= 0) return '';
  if (num >= 100) return `\u00B1${Math.round(num)}%`;
  if (num >= 10) return `\u00B1${num.toFixed(1).replace(/\.0$/, '')}%`;
  return `\u00B1${num.toFixed(2).replace(/0+$/, '').replace(/\.$/, '')}%`;
}

const CANDLE_SCALE_MARGINS = { top: 0.12, bottom: 0.22 };
const MARKER_SIZE = 24;
const MARKER_EDGE_PADDING = 10;
const MARKER_ANCHOR_GAP = 16;
const MARKER_RAIL_ROW_GAP = 20;
const MARKER_DENSE_THRESHOLD = 28;
const MARKER_MERGE_X_GAP = 18;
const MARKER_RECENT_UNMERGED_SECONDS = 10 * 60;

function mergeProjectedMarker(group, marker) {
  const groupItems = Array.isArray(group.items) ? group.items : [];
  const markerItems = Array.isArray(marker.items) ? marker.items : [];
  const nextItems = [...groupItems, ...markerItems];
  const groupWeight = Math.max(1, groupItems.length);
  const markerWeight = Math.max(1, markerItems.length);
  const totalWeight = groupWeight + markerWeight;
  const markerUsd = Number(marker.estimatedUSD);
  const groupUsd = Number(group.estimatedUSD);
  const nextUSD = (Number.isFinite(groupUsd) ? groupUsd : 0) + (Number.isFinite(markerUsd) ? markerUsd : 0);
  const primary = Math.abs(markerUsd) > Math.abs(groupUsd) ? marker : group;
  return {
    ...group,
    x: ((group.x * groupWeight) + (marker.x * markerWeight)) / totalWeight,
    anchorY: ((group.anchorY * groupWeight) + (marker.anchorY * markerWeight)) / totalWeight,
    items: nextItems,
    estimatedUSD: nextUSD,
    label: primary.label,
  };
}

function mergeDenseProjectedMarkers(markers) {
  if (markers.length <= MARKER_DENSE_THRESHOLD) return markers;
  const latestTime = markers.reduce((max, marker) => {
    const t = Number(marker.time || 0);
    return Number.isFinite(t) && t > max ? t : max;
  }, 0);
  const recentCutoff = latestTime > 0 ? latestTime - MARKER_RECENT_UNMERGED_SECONDS : Number.POSITIVE_INFINITY;
  const recent = [];
  const mergeCandidates = [];
  markers.forEach((marker) => {
    const markerTime = Number(marker.time || 0);
    if (Number.isFinite(markerTime) && markerTime >= recentCutoff) {
      recent.push(marker);
      return;
    }
    mergeCandidates.push(marker);
  });
  if (mergeCandidates.length <= MARKER_DENSE_THRESHOLD) {
    return [...mergeCandidates, ...recent];
  }
  const sorted = [...mergeCandidates].sort((a, b) => {
    const railOrder = String(a.rail).localeCompare(String(b.rail));
    if (railOrder !== 0) return railOrder;
    const actionOrder = String(a.action).localeCompare(String(b.action));
    if (actionOrder !== 0) return actionOrder;
    const typeOrder = Number(a.isMyTrade) - Number(b.isMyTrade);
    if (typeOrder !== 0) return typeOrder;
    return a.x - b.x;
  });
  const groups = [];
  sorted.forEach((marker) => {
    const key = `${marker.rail}:${marker.action}:${marker.isMyTrade ? 'my' : 'sm'}`;
    const last = groups[groups.length - 1];
    if (last && last.mergeKey === key && Math.abs(last.x - marker.x) <= MARKER_MERGE_X_GAP) {
      groups[groups.length - 1] = {
        ...mergeProjectedMarker(last, marker),
        mergeKey: key,
      };
      return;
    }
    groups.push({ ...marker, mergeKey: key });
  });
  return [...groups.map(({ mergeKey, ...marker }) => marker), ...recent];
}

function layoutMarkerRails(markers, hostHeight, priceTop, priceBottom) {
  const rows = hostHeight >= 440 ? 2 : 1;
  const hostMinY = MARKER_EDGE_PADDING + (MARKER_SIZE / 2);
  const hostMaxY = Math.max(hostMinY, hostHeight - MARKER_EDGE_PADDING - (MARKER_SIZE / 2));
  const topSafeY = Number.isFinite(priceTop)
    ? Math.max(hostMinY, priceTop + (MARKER_SIZE / 2) + 6)
    : hostMinY;
  const bottomSafeY = Number.isFinite(priceBottom)
    ? Math.min(hostMaxY, priceBottom - (MARKER_SIZE / 2) - 8)
    : hostMaxY;
  const minCenterY = Math.min(topSafeY, bottomSafeY);
  const maxCenterY = Math.max(topSafeY, bottomSafeY);
  const lanes = {
    top: new Array(rows).fill(Number.NEGATIVE_INFINITY),
    bottom: new Array(rows).fill(Number.NEGATIVE_INFINITY),
  };
  const merged = mergeDenseProjectedMarkers(markers);
  return [...merged]
    .sort((a, b) => a.x - b.x)
    .map((marker) => {
      const rail = marker.rail === 'bottom' ? 'bottom' : 'top';
      const footprint = marker.items.length > 1 ? MARKER_SIZE + 8 : MARKER_SIZE + 4;
      let lane = 0;
      let bestGap = Number.NEGATIVE_INFINITY;
      for (let index = 0; index < rows; index += 1) {
        const gap = marker.x - lanes[rail][index];
        if (gap >= footprint) {
          lane = index;
          break;
        }
        if (gap > bestGap) {
          bestGap = gap;
          lane = index;
        }
      }
      lanes[rail][lane] = marker.x + footprint;
      const anchorY = Number.isFinite(marker.anchorY) ? marker.anchorY : minCenterY;
      const laneOffset = lane * MARKER_RAIL_ROW_GAP;
      let y;
      if (rail === 'bottom') {
        const desiredY = anchorY + MARKER_ANCHOR_GAP + laneOffset;
        y = desiredY <= maxCenterY ? desiredY : maxCenterY - laneOffset;
      } else {
        const desiredY = anchorY - MARKER_ANCHOR_GAP - laneOffset;
        y = desiredY >= minCenterY ? desiredY : minCenterY + laneOffset;
      }
      y = clamp(y, minCenterY, maxCenterY);
      const pinHeight = Math.max(0, Math.abs(anchorY - y) - (MARKER_SIZE / 2) - 4);
      return {
        ...marker,
        y,
        lane,
        pinHeight,
        pinDirection: anchorY >= y ? 'down' : 'up',
      };
    });
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

function formatMeasurementPercent(percent) {
  if (!Number.isFinite(percent)) return '--';
  const sign = percent > 0 ? '+' : '';
  const digits = Math.abs(percent) >= 100 ? 1 : 2;
  return `${sign}${percent.toFixed(digits)}%`;
}

function projectDrawing(chart, candleSeries, drawing, hostWidth, hostHeight) {
  if (!chart || !candleSeries || !drawing?.start || !drawing?.end) return null;
  const timeScale = chart.timeScale();
  const rawX1 = timeScale.logicalToCoordinate?.(drawing.start.logical);
  const rawX2 = timeScale.logicalToCoordinate?.(drawing.end.logical);
  const rawY1 = candleSeries.priceToCoordinate(drawing.start.price);
  const rawY2 = candleSeries.priceToCoordinate(drawing.end.price);
  if (![rawX1, rawX2, rawY1, rawY2].every((value) => Number.isFinite(value))) return null;
  const x1 = Number(rawX1);
  const x2 = Number(rawX2);
  const y1 = Number(rawY1);
  const y2 = Number(rawY2);
  const startPrice = Number(drawing.start.price || 0);
  const endPrice = Number(drawing.end.price || 0);
  const percent = startPrice > 0 && Number.isFinite(endPrice)
    ? ((endPrice - startPrice) / startPrice) * 100
    : 0;
  const width = Math.max(0, Number(hostWidth || 0));
  const height = Math.max(0, Number(hostHeight || 0));
  const labelX = drawing.type === 'rect' ? (Math.min(x1, x2) + Math.max(x1, x2)) / 2 : (x1 + x2) / 2;
  const labelY = drawing.type === 'rect'
    ? Math.min(y1, y2) - 10
    : ((y1 + y2) / 2) - 10;
  return {
    type: drawing.type,
    x1,
    y1,
    x2,
    y2,
    left: Math.min(x1, x2),
    top: Math.min(y1, y2),
    width: Math.abs(x2 - x1),
    height: Math.abs(y2 - y1),
    label: formatMeasurementPercent(percent),
    labelX: clamp(labelX, 20, Math.max(20, width - 20)),
    labelY: clamp(labelY, 20, Math.max(20, height - 20)),
    positive: percent >= 0,
    draft: !drawing.complete,
  };
}

function projectClusters(chart, candleSeries, candleData, candleMap, clusters, hostWidth, hostHeight, userAvatarUrl) {
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

    projected.push({
      ...cluster,
      x: clamp(x, xPad, Math.max(xPad, width - xPad)),
      y: clamp(y, yPad, height > 0 ? Math.max(yPad, height - yPad) : Number.POSITIVE_INFINITY),
      anchorY: clamp(y, yPad, height > 0 ? Math.max(yPad, height - yPad) : Number.POSITIVE_INFINITY),
      rail: cluster.action === 'remove' ? 'bottom' : 'top',
      label: cluster.isMyTrade && userAvatarUrl ? userAvatarUrl : getClusterAvatarUrl(cluster),
    });
  }
  return layoutMarkerRails(projected, height, priceTop, priceBottom);
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
      let topY = Math.min(lowerY, upperY);
      let bottomY = Math.max(lowerY, upperY);
      const minPixelGap = Math.max(0, Number(overlay?.minPixelGap || 0));
      if (minPixelGap > 0 && Number.isFinite(topY) && Number.isFinite(bottomY) && (bottomY - topY) < minPixelGap) {
        const midY = (topY + bottomY) / 2;
        topY = midY - minPixelGap / 2;
        bottomY = midY + minPixelGap / 2;
        if (height > 0) {
          const minY = 12;
          const maxY = Math.max(minY + minPixelGap, height - 12);
          if (topY < minY) {
            const offset = minY - topY;
            topY += offset;
            bottomY += offset;
          }
          if (bottomY > maxY) {
            const offset = bottomY - maxY;
            topY -= offset;
            bottomY -= offset;
          }
        }
      }
      out.push({
        id: overlay.id,
        type: 'range',
        color: String(overlay?.color || 'red'),
        label: String(overlay?.label || ''),
        avatarUrl: String(overlay?.avatarUrl || ''),
        showAvatar: overlay?.showAvatar !== false,
        minPixelGap,
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
      price,
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
  onSaveWalletLabel,
  drawingTool = 'none',
  drawingResetNonce = 0,
  viewportKey = '',
  chartHeight = 520,
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
  const [projectedDrawing, setProjectedDrawing] = useState(null);
  const [hoveredCluster, setHoveredCluster] = useState(null);
  const [completedDrawing, setCompletedDrawing] = useState(null);
  const [draftDrawing, setDraftDrawing] = useState(null);
  const [labelEditing, setLabelEditing] = useState(false);
  const [labelDraft, setLabelDraft] = useState('');
  const [labelSaving, setLabelSaving] = useState(false);
  const [labelError, setLabelError] = useState('');
  const updateProjectionRef = useRef(null);
  const visibleRangeHandlerRef = useRef(onVisibleRangeChange);
  const tooltipHideTimerRef = useRef(null);
  const drawingStartRef = useRef(null);
  const rectDrawingRef = useRef(false);

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

  const markerClusters = useMemo(() => {
    const rows = Array.isArray(markers) ? markers : [];
    return rows
      .map((row, index) => {
        const action = String(row?.action || 'add').toLowerCase() === 'remove' ? 'remove' : 'add';
        const time = toUnixSeconds(row?.bucket_t || row?.t);
        if (!time) return null;
        const isMyTrade = Boolean(row?.is_my_trade);
        const walletAddress = normalizeWalletAddress(row?.wallet_address);
        const eventID = String(row?.event_id || '').trim();
        return {
          id: eventID || `${isMyTrade ? 'my' : 'sm'}:${time}:${action}:${walletAddress || index}:${index}`,
          time,
          action,
          items: [row],
          estimatedUSD: Number(row?.estimated_usd || 0),
          isMyTrade,
        };
      })
      .filter(Boolean)
      .sort((a, b) => a.time - b.time);
  }, [markers]);

  const displayMarkers = projectedMarkers;

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
    if (activeMarkerId) return;
    clearTooltipHideTimer();
    tooltipHideTimerRef.current = window.setTimeout(() => {
      setHoveredCluster(null);
      tooltipHideTimerRef.current = null;
    }, 180);
  }, [activeMarkerId, clearTooltipHideTimer]);

  const clearDrawingState = useCallback(() => {
    drawingStartRef.current = null;
    rectDrawingRef.current = false;
    setCompletedDrawing(null);
    setDraftDrawing(null);
    setProjectedDrawing(null);
  }, []);

  const copyWalletAddress = useCallback((walletAddress) => {
    const address = normalizeWalletAddress(walletAddress);
    if (!address) return;
    navigator.clipboard?.writeText(address).catch(() => {});
  }, []);

  const resolveDrawingAnchor = useCallback((event) => {
    const host = chartHostRef.current;
    const chart = chartRef.current;
    const candleSeries = candleSeriesRef.current;
    if (!host || !chart || !candleSeries) return null;
    const rect = host.getBoundingClientRect();
    if (!rect.width || !rect.height) return null;
    const x = clamp(event.clientX - rect.left, 0, rect.width);
    const y = clamp(event.clientY - rect.top, 0, rect.height);
    const timeScale = chart.timeScale();
    const rawLogical = timeScale.coordinateToLogical?.(x);
    let logical = Number.isFinite(rawLogical) ? Number(rawLogical) : Number.NaN;
    if (!Number.isFinite(logical)) {
      const visibleLogicalRange = timeScale.getVisibleLogicalRange?.();
      if (visibleLogicalRange) {
        logical = x <= rect.width / 2 ? Number(visibleLogicalRange.from) : Number(visibleLogicalRange.to);
      }
    }
    const rawPrice = candleSeries.coordinateToPrice(y);
    const price = Number(rawPrice);
    if (!Number.isFinite(logical) || !Number.isFinite(price) || price <= 0) return null;
    return { logical, price };
  }, []);

  const handleDrawingClick = useCallback((event) => {
    if (drawingTool !== 'line') return;
    const anchor = resolveDrawingAnchor(event);
    if (!anchor) return;
    event.preventDefault();
    event.stopPropagation();
    const start = drawingStartRef.current;
    if (!start) {
      drawingStartRef.current = anchor;
      setCompletedDrawing(null);
      setDraftDrawing(null);
      return;
    }
    setCompletedDrawing({ type: 'line', start, end: anchor, complete: true });
    setDraftDrawing(null);
    drawingStartRef.current = null;
  }, [drawingTool, resolveDrawingAnchor]);

  const handleDrawingPointerDown = useCallback((event) => {
    if (drawingTool !== 'rect') return;
    const anchor = resolveDrawingAnchor(event);
    if (!anchor) return;
    event.preventDefault();
    event.stopPropagation();
    drawingStartRef.current = anchor;
    rectDrawingRef.current = true;
    event.currentTarget.setPointerCapture?.(event.pointerId);
    setCompletedDrawing(null);
    setDraftDrawing({ type: 'rect', start: anchor, end: anchor, complete: false });
  }, [drawingTool, resolveDrawingAnchor]);

  const handleDrawingPointerMove = useCallback((event) => {
    const anchor = resolveDrawingAnchor(event);
    if (!anchor) return;
    if (drawingTool === 'line' && drawingStartRef.current) {
      setDraftDrawing({ type: 'line', start: drawingStartRef.current, end: anchor, complete: false });
      return;
    }
    if (drawingTool === 'rect' && rectDrawingRef.current && drawingStartRef.current) {
      setDraftDrawing({ type: 'rect', start: drawingStartRef.current, end: anchor, complete: false });
    }
  }, [drawingTool, resolveDrawingAnchor]);

  const handleDrawingPointerUp = useCallback((event) => {
    if (drawingTool !== 'rect' || !rectDrawingRef.current || !drawingStartRef.current) return;
    const anchor = resolveDrawingAnchor(event);
    if (!anchor) {
      clearDrawingState();
      return;
    }
    event.preventDefault();
    event.stopPropagation();
    event.currentTarget.releasePointerCapture?.(event.pointerId);
    setCompletedDrawing({ type: 'rect', start: drawingStartRef.current, end: anchor, complete: true });
    setDraftDrawing(null);
    drawingStartRef.current = null;
    rectDrawingRef.current = false;
  }, [clearDrawingState, drawingTool, resolveDrawingAnchor]);

  useEffect(() => {
    clearDrawingState();
  }, [clearDrawingState, drawingResetNonce, drawingTool, viewportKey]);

  useEffect(() => {
    if (drawingTool !== 'none') {
      clearTooltipHideTimer();
      setHoveredCluster(null);
    }
  }, [clearTooltipHideTimer, drawingTool]);

  useEffect(() => {
    if (!activeMarkerId) return;
    clearTooltipHideTimer();
    setHoveredCluster(displayMarkers.find((item) => item.id === activeMarkerId) || null);
  }, [activeMarkerId, clearTooltipHideTimer, displayMarkers]);

  useEffect(() => {
    if (activeMarkerId) return;
    clearTooltipHideTimer();
    setHoveredCluster(null);
  }, [activeMarkerId, clearTooltipHideTimer]);

  useEffect(() => {
    const handlePointerDownOutside = (event) => {
      if (!activeMarkerId && !hoveredCluster) return;
      const target = event.target;
      if (!(target instanceof Element)) return;
      if (target.closest('.kline-marker') || target.closest('.kline-marker-tooltip')) return;
      clearTooltipHideTimer();
      setHoveredCluster(null);
      onMarkerClick?.(null);
    };

    document.addEventListener('mousedown', handlePointerDownOutside);
    return () => {
      document.removeEventListener('mousedown', handlePointerDownOutside);
    };
  }, [activeMarkerId, hoveredCluster, clearTooltipHideTimer, onMarkerClick]);

  const updateProjection = useCallback(() => {
    const hostWidth = chartHostRef.current?.clientWidth || wrapRef.current?.clientWidth || 0;
    const hostHeight = chartHostRef.current?.clientHeight || wrapRef.current?.clientHeight || 0;
    setProjectedMarkers(
      projectClusters(
        chartRef.current,
        candleSeriesRef.current,
        candleData,
        candleMap,
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
    setProjectedDrawing(
      projectDrawing(
        chartRef.current,
        candleSeriesRef.current,
        draftDrawing || completedDrawing,
        hostWidth,
        hostHeight
      )
    );
  }, [candleData, candleMap, completedDrawing, draftDrawing, markerClusters, rangeOverlays, userAvatarUrl]);

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
    candleSeries.priceScale().applyOptions({
      scaleMargins: CANDLE_SCALE_MARGINS,
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
  const tooltipCluster = activeMarkerId
    ? (displayMarkers.find((item) => item.id === activeMarkerId) || null)
    : hoveredCluster;

  const tooltipData = useMemo(() => {
    if (!tooltipCluster) return null;
    const c = tooltipCluster;
    const primary = c.items?.[0];
    if (!primary) return null;
    const walletAddress = normalizeWalletAddress(primary.wallet_address || '');
    const walletLabelRaw = c.isMyTrade ? '' : String(primary.wallet_label || '').trim();
    const walletName = c.isMyTrade ? '我的交易' : (walletLabelRaw || walletTailLabel(walletAddress));
    const lower = Number(primary.price_lower || 0);
    const upper = Number(primary.price_upper || 0);
    const hasRange = lower > 0 && upper > 0;
    const estimatedCostUsd = Number(primary.estimated_cost_usd);
    const hasEstimatedCost = primary.estimated_cost_usd !== undefined && primary.estimated_cost_usd !== null && Number.isFinite(estimatedCostUsd);
    const estimatedPnlUsd = Number(primary.estimated_realized_pnl_usd);
    const hasEstimatedPnl = primary.estimated_realized_pnl_usd !== undefined && primary.estimated_realized_pnl_usd !== null && Number.isFinite(estimatedPnlUsd);
    const estimatedPnlPct = Number(primary.estimated_realized_pnl_pct);
    const hasEstimatedPnlPct = primary.estimated_realized_pnl_pct !== undefined && primary.estimated_realized_pnl_pct !== null && Number.isFinite(estimatedPnlPct);
    const matchedOpenT = Number(primary.matched_open_t || 0);
    const nearestCandle = findNearestCandle(candleData, candleMap, primary.t || primary.bucket_t);
    const nearestChartPrice = Number(nearestCandle?.candle?.close || 0);
    const nearestChartTime = Number(nearestCandle?.time || 0);
    const rangePct = hasRange ? `±${(((upper - lower) / (upper + lower)) * 100).toFixed(1)}%` : '';
    return {
      walletAddress,
      walletName,
      walletLabelRaw,
      walletAvatarUrl: c.isMyTrade ? String(tooltipCluster.label || '').trim() : markerWalletAvatarUrl(primary),
      lower,
      upper,
      hasRange,
      rangePct,
      normalizedRangePct: Number(primary.range_percent || 0) > 0
        ? formatRangePercent(primary.range_percent)
        : rangePct,
      totalUSD: c.estimatedUSD,
      hasEstimatedCost,
      estimatedCostUsd,
      hasEstimatedPnl,
      estimatedPnlUsd,
      estimatedPnlPctLabel: hasEstimatedPnlPct ? formatSignedPercent(estimatedPnlPct) : '',
      matchedOpenLabel: matchedOpenT > 0 ? formatUtc8DateTime(matchedOpenT) : '',
      hasNearestChartPrice: Number.isFinite(nearestChartPrice) && nearestChartPrice > 0,
      nearestChartPrice,
      nearestChartTimeLabel: nearestChartTime > 0 ? formatUtc8DateTime(nearestChartTime) : '',
      count: c.items.length,
      isMyTrade: c.isMyTrade,
      canEditLabel: !c.isMyTrade && Boolean(walletAddress),
      watched: walletAddress ? watchedWalletSet.has(walletAddress) : false,
      watchBusy: walletAddress ? Boolean(watchToggleMap[walletAddress]) : false,
    };
  }, [candleData, candleMap, tooltipCluster, watchToggleMap, watchedWalletSet]);

  useEffect(() => {
    setLabelEditing(false);
    setLabelDraft(tooltipData?.walletLabelRaw || '');
    setLabelSaving(false);
    setLabelError('');
  }, [tooltipCluster?.id, tooltipData?.walletAddress, tooltipData?.walletLabelRaw]);

  const handleSubmitWalletLabel = useCallback(async (event) => {
    event?.preventDefault?.();
    event?.stopPropagation?.();
    if (!tooltipData?.canEditLabel || !tooltipData.walletAddress) return;
    const nextLabel = String(labelDraft || '').trim();
    if (nextLabel === String(tooltipData.walletLabelRaw || '').trim()) {
      setLabelEditing(false);
      setLabelError('');
      return;
    }
    if (typeof onSaveWalletLabel !== 'function') {
      setLabelEditing(false);
      setLabelError('');
      return;
    }
    setLabelSaving(true);
    setLabelError('');
    try {
      await onSaveWalletLabel(tooltipData.walletAddress, nextLabel);
      setLabelEditing(false);
    } catch (err) {
      setLabelError(String(err?.message || err || '保存失败'));
    } finally {
      setLabelSaving(false);
    }
  }, [labelDraft, onSaveWalletLabel, tooltipData]);

  const chartStageStyle = {
    height: `${Math.max(320, Number(chartHeight || 0))}px`,
    minHeight: `${Math.max(320, Number(chartHeight || 0))}px`,
  };

  return (
    <div className="kline-native-wrap" ref={wrapRef} style={chartStageStyle}>
      <div className="kline-native-stage" ref={chartHostRef} style={chartStageStyle} />

      <div className="kline-marker-layer">
        {projectedRangeOverlays.map((overlay) => {
          if (overlay.type === 'range') {
            return (
              <React.Fragment key={overlay.id}>
                <div
                  className={`kline-range-line ${overlay.color} top`}
                  style={{ top: `${overlay.topY}px` }}
                >
                  {overlay.label ? (
                    <span className="kline-range-label">{tooltipSafeLabel(overlay.label)}</span>
                  ) : null}
                  <span className="kline-axis-price">{smartPriceFormatter(overlay.priceUpper)}</span>
                </div>
                <div
                  className={`kline-range-line ${overlay.color} bottom`}
                  style={{ top: `${overlay.bottomY}px` }}
                >
                  <span className="kline-axis-price">{smartPriceFormatter(overlay.priceLower)}</span>
                </div>
                {overlay.showAvatar && overlay.avatarUrl ? (
                  <div
                    className={`kline-range-avatar ${overlay.color}`}
                    style={{ top: `${overlay.midY}px` }}
                  >
                    <img src={overlay.avatarUrl} alt="" />
                  </div>
                ) : null}
              </React.Fragment>
            );
          }
          return (
            <div
              key={overlay.id}
              className={`kline-mid-line ${overlay.color}`}
              style={{ top: `${overlay.y}px` }}
            >
              {overlay.label ? (
                <span className="kline-mid-line-label">{tooltipSafeLabel(overlay.label)}</span>
              ) : null}
              <span className="kline-axis-price">{smartPriceFormatter(overlay.price)}</span>
            </div>
          );
        })}

        {displayMarkers.map((cluster) => (
          <button
            key={cluster.id}
            type="button"
            className={`kline-marker ${cluster.action} rail-${cluster.rail} pin-${cluster.pinDirection} ${cluster.items.length > 1 ? 'clustered' : ''} ${cluster.isMyTrade ? 'my-trade' : ''} ${activeMarkerId === cluster.id ? 'active' : ''} ${isClusterHighlighted(cluster, highlightWalletAddress) ? 'wallet-highlighted' : ''}`}
            style={{
              left: `${cluster.x}px`,
              top: `${cluster.y}px`,
              '--pin-height': `${cluster.pinHeight}px`,
            }}
            onClick={() => {
              clearTooltipHideTimer();
              setHoveredCluster(cluster);
              onMarkerClick?.(cluster);
            }}
            onMouseEnter={() => {
              if (activeMarkerId) return;
              clearTooltipHideTimer();
              setHoveredCluster(cluster);
            }}
            onMouseLeave={scheduleTooltipHide}
          >
            <img className="kline-marker-avatar" src={cluster.label} alt="" />
            {cluster.items.length > 1 ? (
              <span className="kline-marker-badge">{cluster.items.length}</span>
            ) : null}
          </button>
        ))}

        {tooltipCluster && tooltipData && (
          <div
            className={`kline-marker-tooltip ${tooltipCluster.action}`}
            style={{
              left: `${tooltipCluster.x}px`,
              top: `${tooltipCluster.y}px`,
            }}
            onMouseEnter={clearTooltipHideTimer}
            onMouseLeave={scheduleTooltipHide}
          >
            <div className="kmt-head">
              <span className="kmt-emoji"><img src={tooltipData.walletAvatarUrl || tooltipCluster.label} alt="" /></span>
              <span className="kmt-wallet-wrap">
                <span className="kmt-wallet">{tooltipData.walletName}</span>
                {tooltipData.walletAddress ? (
                  <button
                    type="button"
                    className="kmt-copy-btn"
                    onClick={(event) => {
                      event.preventDefault();
                      event.stopPropagation();
                      copyWalletAddress(tooltipData.walletAddress);
                    }}
                    aria-label="复制钱包地址"
                    title="复制钱包地址"
                  >
                    <Copy size={11} />
                  </button>
                ) : null}
              </span>
              {tooltipData.canEditLabel ? (
                <button
                  type="button"
                  className={`kmt-edit-btn ${labelEditing ? 'active' : ''}`}
                  disabled={labelSaving}
                  onClick={(event) => {
                    event.preventDefault();
                    event.stopPropagation();
                    if (labelEditing) return;
                    setLabelDraft(tooltipData.walletLabelRaw || '');
                    setLabelError('');
                    setLabelEditing(true);
                  }}
                  aria-label="编辑钱包标签"
                  title="编辑钱包标签"
                >
                  <Pencil size={11} />
                </button>
              ) : null}
              {!tooltipData.isMyTrade && tooltipData.walletAddress ? (
                <button
                  type="button"
                  className={`kmt-watch-btn ${tooltipData.watched ? 'active' : ''}`}
                  disabled={tooltipData.watchBusy}
                  onClick={(event) => {
                    event.preventDefault();
                    event.stopPropagation();
                    onToggleWatch?.(tooltipData.walletAddress, tooltipData.walletName, tooltipData.watched);
                  }}
                  aria-label={tooltipData.watched ? '取消特别关注' : '加入特别关注'}
                  title={tooltipData.watched ? '取消特别关注' : '加入特别关注'}
                >
                  {tooltipData.watched ? '\u2665' : '\u2661'}
                </button>
              ) : null}
              {tooltipData.count > 1 ? <span className="kmt-count">x{tooltipData.count}</span> : null}
            </div>
            {labelEditing ? (
              <form className="kmt-label-form" onSubmit={handleSubmitWalletLabel}>
                <input
                  className="kmt-label-input"
                  type="text"
                  maxLength={100}
                  autoFocus
                  placeholder="输入钱包标签，留空可清除"
                  value={labelDraft}
                  onChange={(event) => setLabelDraft(event.target.value)}
                  onKeyDown={(event) => {
                    if (event.key !== 'Escape') return;
                    event.preventDefault();
                    setLabelDraft(tooltipData.walletLabelRaw || '');
                    setLabelError('');
                    setLabelEditing(false);
                  }}
                />
                <div className="kmt-label-actions">
                  <button
                    type="submit"
                    className="kmt-text-btn primary"
                    disabled={labelSaving}
                  >
                    <Check size={11} />
                    {labelSaving ? '保存中' : '保存'}
                  </button>
                  <button
                    type="button"
                    className="kmt-text-btn"
                    disabled={labelSaving}
                    onClick={(event) => {
                      event.preventDefault();
                      event.stopPropagation();
                      setLabelDraft(tooltipData.walletLabelRaw || '');
                      setLabelError('');
                      setLabelEditing(false);
                    }}
                  >
                    <X size={11} />
                    取消
                  </button>
                </div>
                {labelError ? (
                  <div className="kmt-error">{labelError}</div>
                ) : (
                  <div className="kmt-hint">留空后保存可清除标签</div>
                )}
              </form>
            ) : null}
            <div className="kmt-row">
              <span className={`kmt-action ${tooltipCluster.action}`}>
                {tooltipCluster.action === 'remove' ? '减仓' : '加仓'}
              </span>
              <span className="kmt-usd">{formatUSD(tooltipData.totalUSD)}</span>
            </div>
            {tooltipData.hasRange && (
              <div className="kmt-row">
                <span className="kmt-range">
                  {smartPriceFormatter(tooltipData.lower)} → {smartPriceFormatter(tooltipData.upper)}
                </span>
                <span className="kmt-pct">{tooltipData.normalizedRangePct || tooltipData.rangePct}</span>
              </div>
            )}
            {tooltipCluster.action === 'remove' && tooltipData.hasEstimatedCost ? (
              <div className="kmt-stat-row">
                <span className="kmt-stat-label">估算成本</span>
                <span className="kmt-stat-value">{formatUSD(tooltipData.estimatedCostUsd)}</span>
              </div>
            ) : null}
            {tooltipCluster.action === 'remove' && tooltipData.hasEstimatedPnl ? (
              <div className="kmt-stat-row">
                <span className="kmt-stat-label">估算盈亏</span>
                <span className={`kmt-stat-value ${tooltipData.estimatedPnlUsd > 0 ? 'positive' : tooltipData.estimatedPnlUsd < 0 ? 'negative' : 'neutral'}`}>
                  {formatSignedUSD(tooltipData.estimatedPnlUsd)}
                  {tooltipData.estimatedPnlPctLabel ? (
                    <span className="kmt-stat-pct">{tooltipData.estimatedPnlPctLabel}</span>
                  ) : null}
                </span>
              </div>
            ) : null}
            {tooltipCluster.action === 'remove' && tooltipData.hasNearestChartPrice ? (
              <div className="kmt-stat-row">
                <span className="kmt-stat-label">图表最近价</span>
                <span className="kmt-stat-value subtle">{smartPriceFormatter(tooltipData.nearestChartPrice)}</span>
              </div>
            ) : null}
            {tooltipCluster.action === 'remove' && tooltipData.matchedOpenLabel ? (
              <div className="kmt-meta-row">对应开仓: {tooltipData.matchedOpenLabel}</div>
            ) : null}
            {tooltipCluster.action === 'remove' && tooltipData.nearestChartTimeLabel ? (
              <div className="kmt-meta-row">最近 candle: {tooltipData.nearestChartTimeLabel}</div>
            ) : null}
          </div>
        )}
      </div>

      <div
        className={`kline-drawing-layer ${drawingTool !== 'none' ? `interactive ${drawingTool}` : ''}`}
        onClick={handleDrawingClick}
        onPointerDown={handleDrawingPointerDown}
        onPointerMove={handleDrawingPointerMove}
        onPointerUp={handleDrawingPointerUp}
      >
        {projectedDrawing ? (
          <>
            <svg
              className="kline-drawing-svg"
              viewBox={`0 0 ${Math.max(1, chartHostRef.current?.clientWidth || 1)} ${Math.max(1, chartHostRef.current?.clientHeight || 1)}`}
              preserveAspectRatio="none"
            >
              {projectedDrawing.type === 'line' ? (
                <line
                  x1={projectedDrawing.x1}
                  y1={projectedDrawing.y1}
                  x2={projectedDrawing.x2}
                  y2={projectedDrawing.y2}
                  className={`kline-drawing-line ${projectedDrawing.positive ? 'positive' : 'negative'} ${projectedDrawing.draft ? 'draft' : ''}`}
                />
              ) : (
                <rect
                  x={projectedDrawing.left}
                  y={projectedDrawing.top}
                  width={Math.max(1, projectedDrawing.width)}
                  height={Math.max(1, projectedDrawing.height)}
                  className={`kline-drawing-rect ${projectedDrawing.positive ? 'positive' : 'negative'} ${projectedDrawing.draft ? 'draft' : ''}`}
                />
              )}
            </svg>
            <div
              className={`kline-drawing-label ${projectedDrawing.positive ? 'positive' : 'negative'} ${projectedDrawing.draft ? 'draft' : ''}`}
              style={{
                left: `${projectedDrawing.labelX}px`,
                top: `${projectedDrawing.labelY}px`,
              }}
            >
              {projectedDrawing.label}
            </div>
          </>
        ) : null}
      </div>

      {loading ? <div className="kline-state-overlay">加载 K 线中...</div> : null}
      {!loading && error ? <div className="kline-state-overlay error">{error}</div> : null}
      {empty ? <div className="kline-state-overlay">暂无 K 线数据</div> : null}
    </div>
  );
}
