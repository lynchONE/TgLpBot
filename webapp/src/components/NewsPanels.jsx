import React, { useEffect, useRef, useState } from 'react';
import { Newspaper } from 'lucide-react';
import { formatUtc8DateTime, formatUtc8Time } from '../utils';

const NEWS_TICKER_DEFAULT_DURATION_SEC = 360;
const NEWS_TICKER_MIN_SPEED = 2;
const NEWS_TICKER_MAX_SPEED = 80;
const NEWS_TICKER_DEFAULT_SPEED = 8;

function normalizeTickerSpeed(value) {
  const n = Number(value);
  if (!Number.isFinite(n)) return NEWS_TICKER_DEFAULT_SPEED;
  return Math.min(NEWS_TICKER_MAX_SPEED, Math.max(NEWS_TICKER_MIN_SPEED, Math.round(n)));
}

function formatNewsDateTime(value) {
  if (!value) return '';
  const text = formatUtc8DateTime(value);
  return text === '--' ? '' : text;
}

function formatNewsTickerTime(value) {
  if (!value) return '';
  const text = formatUtc8Time(value);
  return text === '--' ? '' : text;
}

export function NewsShowcase({ items, loading, error, status, onOpen }) {
  const rows = Array.isArray(items) ? items.slice(0, 4) : [];
  if (rows.length === 0) return null;

  const showStatus = loading || status !== 'ok';
  return (
    <section className="news-showcase" aria-label="热点推荐新闻">
      <div className="news-showcase-head">
        <div className="news-showcase-title">
          <Newspaper size={15} />
          <span>热点推荐</span>
        </div>
        {showStatus ? (
          <span className={`news-showcase-status ${status === 'ok' ? 'ok' : ''}`}>
            {loading ? '同步中' : '待同步'}
          </span>
        ) : null}
      </div>
      {rows.length > 0 ? (
        <div className="news-showcase-list">
          {rows.map((item, index) => (
            <button
              type="button"
              key={`${item.external_id || item.id || index}`}
              className="news-showcase-item"
              onClick={() => onOpen(item.source_link)}
              disabled={!item.source_link}
              title={item.title}
            >
              <span className="news-showcase-rank">{index + 1}</span>
              <span className="news-showcase-main">
                <span className="news-showcase-item-title">{item.title}</span>
                <span className="news-showcase-meta">
                  {item.author ? <span>{item.author}</span> : null}
                  {item.release_time ? <span>{formatNewsDateTime(item.release_time)}</span> : null}
                </span>
              </span>
            </button>
          ))}
        </div>
      ) : (
        <div className="news-showcase-empty">
          {loading ? '正在读取新闻...' : error || '暂无新闻'}
        </div>
      )}
    </section>
  );
}

export function NewsTicker({ items, loading, error, speedPxPerSec, onOpen }) {
  const rows = Array.isArray(items) ? items.filter((item) => item?.title) : [];
  const tickerRows = rows.length > 0 ? [...rows, ...rows] : [];
  const tickerContentKey = rows.map((item) => `${item.external_id || item.id || ''}:${item.title}`).join('|');
  const marqueeRef = useRef(null);
  const [durationSec, setDurationSec] = useState(NEWS_TICKER_DEFAULT_DURATION_SEC);

  useEffect(() => {
    const marquee = marqueeRef.current;
    if (!marquee || tickerRows.length === 0) {
      setDurationSec(NEWS_TICKER_DEFAULT_DURATION_SEC);
      return undefined;
    }

    const updateDuration = () => {
      const distancePx = marquee.scrollWidth / 2;
      if (!Number.isFinite(distancePx) || distancePx <= 0) return;
      const nextDuration = Math.max(1, Math.round((distancePx / normalizeTickerSpeed(speedPxPerSec)) * 10) / 10);
      setDurationSec((prev) => (Math.abs(prev - nextDuration) < 0.1 ? prev : nextDuration));
    };

    const frameId = window.requestAnimationFrame(updateDuration);
    let observer = null;
    if (typeof ResizeObserver !== 'undefined') {
      observer = new ResizeObserver(updateDuration);
      observer.observe(marquee);
    }
    window.addEventListener('resize', updateDuration);

    return () => {
      window.cancelAnimationFrame(frameId);
      if (observer) observer.disconnect();
      window.removeEventListener('resize', updateDuration);
    };
  }, [tickerContentKey, tickerRows.length, speedPxPerSec]);

  if (rows.length === 0) return null;

  return (
    <div
      className="news-ticker"
      role="region"
      aria-label="热点新闻滚动条"
      style={{ '--news-ticker-duration': `${durationSec}s` }}
    >
      <div className="news-ticker-label">NEWS</div>
      <div className="news-ticker-track">
        {tickerRows.length > 0 ? (
          <div className="news-ticker-marquee" ref={marqueeRef}>
            {tickerRows.map((item, index) => (
              <button
                type="button"
                key={`${item.external_id || item.id || index}:${index}`}
                onClick={() => onOpen(item.source_link)}
                disabled={!item.source_link}
                title={item.title}
              >
                {item.release_time ? <time>{formatNewsTickerTime(item.release_time)}</time> : null}
                <span>{item.title}</span>
              </button>
            ))}
          </div>
        ) : (
          <span className="news-ticker-empty">
            {loading ? '新闻同步中...' : error || '暂无新闻滚动内容'}
          </span>
        )}
      </div>
    </div>
  );
}
