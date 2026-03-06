import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { createChart, CandlestickSeries, HistogramSeries } from 'lightweight-charts';
import { formatUtc8DateTime, formatUtc8TickMark, shortAddress, toUnixSeconds } from '../utils';

function getClusterText(cluster) {
  const items = Array.isArray(cluster?.items) ? cluster.items : [];
  if (items.length > 1) return String(items.length);
  const item = items[0];
  const label = String(item?.wallet_label || '').trim();
  if (label) {
    const letters = label
      .split(/\s+/)
      .map((part) => part.trim().charAt(0).toUpperCase())
      .join('')
      .slice(0, 2);
    if (letters) return letters;
  }
  return shortAddress(item?.wallet_address || '', 2, 2).replace(/\./g, '');
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

function clamp(value, min, max) {
  if (!Number.isFinite(value)) return min;
  return Math.min(Math.max(value, min), max);
}

function projectClusters(chart, candleSeries, candleData, candleMap, candleIndexMap, clusters, hostWidth, hostHeight) {
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

  for (const cluster of clusters) {
    const located =
      findNearestCandle(candleData, candleMap, cluster.time) ||
      findNearestCandle(candleData, candleMap, cluster.items?.[0]?.bucket_t) ||
      findNearestCandle(candleData, candleMap, cluster.items?.[0]?.t);
    if (!located?.candle) continue;
    const candle = located.candle;

    const time = Number(located.time || cluster.time || 0);
    let x = timeScale.timeToCoordinate(time);
    if (!Number.isFinite(x) && width > xPad * 2) {
      const idx = Number(candleIndexMap.get(time));
      if (Number.isFinite(idx) && candleData.length > 1) {
        x = xPad + ((width - xPad * 2) * idx) / (candleData.length - 1);
      } else {
        x = width / 2;
      }
    }
    if (!Number.isFinite(x)) continue;

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
      label: getClusterText(cluster),
    });
  }
  return projected;
}

export default function KlineChart({
  candles,
  markers,
  loading = false,
  error = '',
  onMarkerClick,
  activeMarkerId = '',
  viewportKey = '',
}) {
  const wrapRef = useRef(null);
  const chartHostRef = useRef(null);
  const chartRef = useRef(null);
  const candleSeriesRef = useRef(null);
  const volumeSeriesRef = useRef(null);
  const [projectedMarkers, setProjectedMarkers] = useState([]);

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
      const key = `${time}:${action}`;
      const prev = grouped.get(key) || {
        id: key,
        time,
        action,
        items: [],
        estimatedUSD: 0,
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
        hostHeight
      )
    );
  }, [candleData, candleMap, candleIndexMap, markerClusters]);

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
      window.requestAnimationFrame(updateProjection);
    };
    chart.timeScale().subscribeVisibleTimeRangeChange(onVisibleChange);

    const observer = new ResizeObserver((entries) => {
      const box = entries?.[0]?.contentRect;
      if (!box) return;
      chart.applyOptions({
        width: Math.max(260, Math.floor(box.width)),
        height: Math.max(360, Math.floor(box.height)),
      });
      window.requestAnimationFrame(updateProjection);
    });
    observer.observe(host);

    return () => {
      observer.disconnect();
      chart.timeScale().unsubscribeVisibleTimeRangeChange(onVisibleChange);
      chart.remove();
      chartRef.current = null;
      candleSeriesRef.current = null;
      volumeSeriesRef.current = null;
    };
  }, [updateProjection]);

  useEffect(() => {
    if (!candleSeriesRef.current || !volumeSeriesRef.current) return;
    const volumeData = candleData.map((row) => ({
      time: row.time,
      value: row.volume,
      color: row.close >= row.open ? 'rgba(22,199,132,0.45)' : 'rgba(234,57,67,0.45)',
    }));

    candleSeriesRef.current.setData(candleData);
    volumeSeriesRef.current.setData(volumeData);
    window.requestAnimationFrame(updateProjection);
  }, [candleData, updateProjection]);

  useEffect(() => {
    if (!chartRef.current || !candleData.length) return;
    chartRef.current.timeScale().fitContent();
    window.requestAnimationFrame(updateProjection);
  }, [viewportKey, candleData.length, updateProjection]);

  useEffect(() => {
    window.requestAnimationFrame(updateProjection);
  }, [updateProjection]);

  const empty = !loading && !error && candleData.length === 0;

  return (
    <div className="kline-native-wrap" ref={wrapRef}>
      <div className="kline-native-stage" ref={chartHostRef} />

      <div className="kline-marker-layer">
        {projectedMarkers.map((cluster) => (
          <button
            key={cluster.id}
            type="button"
            className={`kline-marker ${cluster.action} ${activeMarkerId === cluster.id ? 'active' : ''}`}
            style={{
              left: `${cluster.x}px`,
              top: `${cluster.y}px`,
            }}
            onClick={() => onMarkerClick?.(cluster)}
            title={`${cluster.action === 'remove' ? '减仓' : '加仓'} · ${cluster.items.length} 条活动`}
          >
            {cluster.label}
          </button>
        ))}
      </div>

      {loading ? <div className="kline-state-overlay">加载 K 线中...</div> : null}
      {!loading && error ? <div className="kline-state-overlay error">{error}</div> : null}
      {empty ? <div className="kline-state-overlay">暂无 K 线数据</div> : null}
    </div>
  );
}
