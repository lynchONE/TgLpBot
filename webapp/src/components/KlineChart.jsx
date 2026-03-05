import React, { useEffect, useRef } from 'react';
import { createChart } from 'lightweight-charts';
import { toUnixSeconds } from '../utils';

export default function KlineChart({ candles }) {
  const containerRef = useRef(null);
  const chartRef = useRef(null);
  const candleSeriesRef = useRef(null);
  const volumeSeriesRef = useRef(null);

  useEffect(() => {
    if (!containerRef.current) return;
    const host = containerRef.current;
    const chart = createChart(host, {
      width: Math.max(260, host.clientWidth),
      height: Math.max(260, host.clientHeight),
      layout: {
        background: { color: '#0b1018' },
        textColor: '#95a0b5',
      },
      grid: {
        vertLines: { color: 'rgba(122, 142, 173, 0.15)' },
        horzLines: { color: 'rgba(122, 142, 173, 0.15)' },
      },
      rightPriceScale: { borderColor: 'rgba(122, 142, 173, 0.25)' },
      timeScale: {
        borderColor: 'rgba(122, 142, 173, 0.25)',
        timeVisible: true,
        secondsVisible: false,
      },
      crosshair: {
        vertLine: { color: 'rgba(255, 183, 59, 0.35)' },
        horzLine: { color: 'rgba(255, 183, 59, 0.35)' },
      },
    });

    const candleSeries = chart.addCandlestickSeries({
      upColor: '#16c784',
      downColor: '#ea3943',
      borderUpColor: '#16c784',
      borderDownColor: '#ea3943',
      wickUpColor: '#16c784',
      wickDownColor: '#ea3943',
    });
    const volumeSeries = chart.addHistogramSeries({
      priceFormat: { type: 'volume' },
      priceScaleId: '',
    });
    volumeSeries.priceScale().applyOptions({
      scaleMargins: { top: 0.76, bottom: 0 },
    });

    chartRef.current = chart;
    candleSeriesRef.current = candleSeries;
    volumeSeriesRef.current = volumeSeries;

    const observer = new ResizeObserver((entries) => {
      const box = entries?.[0]?.contentRect;
      if (!box) return;
      chart.applyOptions({
        width: Math.max(260, Math.floor(box.width)),
        height: Math.max(260, Math.floor(box.height)),
      });
    });
    observer.observe(host);

    return () => {
      observer.disconnect();
      chart.remove();
      chartRef.current = null;
      candleSeriesRef.current = null;
      volumeSeriesRef.current = null;
    };
  }, []);

  useEffect(() => {
    if (!candleSeriesRef.current || !volumeSeriesRef.current) return;

    const rows = Array.isArray(candles) ? candles : [];
    const candleData = [];
    const volumeData = [];
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
      candleData.push({ time, open, high, low, close });
      volumeData.push({
        time,
        value: Number.isFinite(volume) ? volume : 0,
        color: close >= open ? 'rgba(22,199,132,0.5)' : 'rgba(234,57,67,0.5)',
      });
    }

    candleSeriesRef.current.setData(candleData);
    volumeSeriesRef.current.setData(volumeData);
    if (candleData.length) chartRef.current?.timeScale().fitContent();
  }, [candles]);

  return <div className="kline-chart" ref={containerRef} />;
}
