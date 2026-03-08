import React, { useEffect, useMemo, useRef, useState } from 'react';

const POSTER_WIDTH = 1080;
const POSTER_HEIGHT = 1350;

function formatUsdSafe(value) {
  const number = Number(value ?? 0);
  if (!Number.isFinite(number)) return '$--';
  return new Intl.NumberFormat('en-US', {
    style: 'currency',
    currency: 'USD',
    maximumFractionDigits: 2,
  }).format(number);
}

function formatPctSafe(value) {
  const number = Number(value ?? 0);
  if (!Number.isFinite(number)) return '--';
  const prefix = number > 0 ? '+' : '';
  return `${prefix}${number.toFixed(2)}%`;
}

function formatPosterTime(value) {
  if (!value) return '--';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '--';
  const month = String(date.getMonth() + 1).padStart(2, '0');
  const day = String(date.getDate()).padStart(2, '0');
  const hour = String(date.getHours()).padStart(2, '0');
  const minute = String(date.getMinutes()).padStart(2, '0');
  return `${month}-${day} ${hour}:${minute}`;
}

function clamp(value, min, max) {
  return Math.max(min, Math.min(max, value));
}

function toDisplayName(loginUser) {
  if (!loginUser || typeof loginUser !== 'object') return 'Telegram User';
  const firstName = String(loginUser?.first_name || '').trim();
  const lastName = String(loginUser?.last_name || '').trim();
  const fullName = `${firstName} ${lastName}`.trim();
  if (fullName) return fullName;
  const username = String(loginUser?.username || '').trim();
  if (username) return `@${username}`;
  return 'Telegram User';
}

function toInitials(text) {
  const raw = String(text || '').trim();
  if (!raw) return '?';
  return raw.replace(/\s+/g, '').slice(0, 2).toUpperCase();
}

function loadImage(url) {
  return new Promise((resolve) => {
    const src = String(url || '').trim();
    if (!src) {
      resolve(null);
      return;
    }
    const img = new Image();
    img.crossOrigin = 'anonymous';
    img.referrerPolicy = 'no-referrer';
    img.onload = () => resolve(img);
    img.onerror = () => resolve(null);
    img.src = src;
  });
}

function roundRectPath(ctx, x, y, width, height, radius) {
  const r = Math.min(radius, width / 2, height / 2);
  ctx.beginPath();
  ctx.moveTo(x + r, y);
  ctx.arcTo(x + width, y, x + width, y + height, r);
  ctx.arcTo(x + width, y + height, x, y + height, r);
  ctx.arcTo(x, y + height, x, y, r);
  ctx.arcTo(x, y, x + width, y, r);
  ctx.closePath();
}

function fillRoundRect(ctx, x, y, width, height, radius, fillStyle, strokeStyle = '') {
  roundRectPath(ctx, x, y, width, height, radius);
  ctx.fillStyle = fillStyle;
  ctx.fill();
  if (strokeStyle) {
    ctx.strokeStyle = strokeStyle;
    ctx.lineWidth = 1;
    ctx.stroke();
  }
}

function drawAvatar(ctx, image, x, y, size, fallbackText, palette) {
  if (image) {
    ctx.save();
    roundRectPath(ctx, x, y, size, size, size / 2);
    ctx.clip();
    ctx.drawImage(image, x, y, size, size);
    ctx.restore();
    return;
  }
  const gradient = ctx.createLinearGradient(x, y, x + size, y + size);
  gradient.addColorStop(0, palette[0]);
  gradient.addColorStop(1, palette[1]);
  fillRoundRect(ctx, x, y, size, size, size / 2, gradient);
  ctx.fillStyle = '#ffffff';
  ctx.font = `700 ${Math.round(size * 0.34)}px Inter, system-ui, sans-serif`;
  ctx.textAlign = 'center';
  ctx.textBaseline = 'middle';
  ctx.fillText(toInitials(fallbackText), x + size / 2, y + size / 2 + 2);
}

function drawStatCard(ctx, { x, y, width, height, label, value, tone }) {
  const fills = {
    neutral: 'rgba(15, 23, 42, 0.84)',
    positive: 'rgba(16, 185, 129, 0.12)',
    negative: 'rgba(239, 68, 68, 0.12)',
    accent: 'rgba(96, 165, 250, 0.12)',
  };
  const strokes = {
    neutral: 'rgba(148, 163, 184, 0.18)',
    positive: 'rgba(16, 185, 129, 0.26)',
    negative: 'rgba(239, 68, 68, 0.26)',
    accent: 'rgba(96, 165, 250, 0.26)',
  };
  const colors = {
    neutral: '#E2E8F0',
    positive: '#6EE7B7',
    negative: '#FCA5A5',
    accent: '#93C5FD',
  };

  fillRoundRect(ctx, x, y, width, height, 30, fills[tone] || fills.neutral, strokes[tone] || strokes.neutral);
  ctx.fillStyle = 'rgba(226, 232, 240, 0.72)';
  ctx.font = '500 28px Inter, system-ui, sans-serif';
  ctx.textAlign = 'left';
  ctx.textBaseline = 'top';
  ctx.fillText(label, x + 28, y + 24);

  ctx.fillStyle = colors[tone] || colors.neutral;
  ctx.font = '700 42px Inter, system-ui, sans-serif';
  ctx.fillText(value, x + 28, y + 66);
}

function drawChart(ctx, series, x, y, width, height) {
  fillRoundRect(ctx, x, y, width, height, 34, 'rgba(15, 23, 42, 0.84)', 'rgba(148, 163, 184, 0.18)');

  ctx.fillStyle = '#F8FAFC';
  ctx.font = '700 34px Inter, system-ui, sans-serif';
  ctx.textAlign = 'left';
  ctx.textBaseline = 'top';
  ctx.fillText('开单以来走势', x + 32, y + 28);

  ctx.fillStyle = 'rgba(226, 232, 240, 0.65)';
  ctx.font = '500 22px Inter, system-ui, sans-serif';
  ctx.fillText('基于 OKX 行情的价格收益曲线', x + 32, y + 74);

  const plotX = x + 36;
  const plotY = y + 136;
  const plotWidth = width - 72;
  const plotHeight = height - 192;

  if (!Array.isArray(series) || series.length < 2) {
    fillRoundRect(ctx, plotX, plotY, plotWidth, plotHeight, 26, 'rgba(15, 23, 42, 0.52)', 'rgba(148, 163, 184, 0.12)');
    ctx.fillStyle = 'rgba(226, 232, 240, 0.68)';
    ctx.font = '600 28px Inter, system-ui, sans-serif';
    ctx.textAlign = 'center';
    ctx.textBaseline = 'middle';
    ctx.fillText('暂无可用走势数据', plotX + plotWidth / 2, plotY + plotHeight / 2);
    return;
  }

  const points = series
    .map((item) => ({
      t: Number(item?.t || 0),
      value: Number(item?.value || 0),
    }))
    .filter((item) => Number.isFinite(item.t) && item.t > 0 && Number.isFinite(item.value))
    .sort((a, b) => a.t - b.t);

  if (points.length < 2) {
    fillRoundRect(ctx, plotX, plotY, plotWidth, plotHeight, 26, 'rgba(15, 23, 42, 0.52)', 'rgba(148, 163, 184, 0.12)');
    ctx.fillStyle = 'rgba(226, 232, 240, 0.68)';
    ctx.font = '600 28px Inter, system-ui, sans-serif';
    ctx.textAlign = 'center';
    ctx.textBaseline = 'middle';
    ctx.fillText('暂无可用走势数据', plotX + plotWidth / 2, plotY + plotHeight / 2);
    return;
  }

  const minT = points[0].t;
  const maxT = points[points.length - 1].t;
  const values = points.map((item) => item.value);
  const minValue = Math.min(...values);
  const maxValue = Math.max(...values);
  const pad = Math.max(1.5, (maxValue - minValue) * 0.18);
  const yMin = minValue - pad;
  const yMax = maxValue + pad;
  const valueRange = yMax - yMin || 1;
  const timeRange = maxT - minT || 1;
  const zeroY = plotY + plotHeight - ((0 - yMin) / valueRange) * plotHeight;

  for (let index = 0; index < 5; index += 1) {
    const rowY = plotY + (plotHeight / 4) * index;
    ctx.strokeStyle = 'rgba(148, 163, 184, 0.12)';
    ctx.lineWidth = 1;
    ctx.beginPath();
    ctx.moveTo(plotX, rowY);
    ctx.lineTo(plotX + plotWidth, rowY);
    ctx.stroke();
  }

  if (zeroY >= plotY && zeroY <= plotY + plotHeight) {
    ctx.strokeStyle = 'rgba(226, 232, 240, 0.18)';
    ctx.setLineDash([10, 10]);
    ctx.beginPath();
    ctx.moveTo(plotX, zeroY);
    ctx.lineTo(plotX + plotWidth, zeroY);
    ctx.stroke();
    ctx.setLineDash([]);
  }

  const projected = points.map((item) => ({
    x: plotX + ((item.t - minT) / timeRange) * plotWidth,
    y: plotY + plotHeight - ((item.value - yMin) / valueRange) * plotHeight,
    value: item.value,
  }));

  const areaGradient = ctx.createLinearGradient(plotX, plotY, plotX, plotY + plotHeight);
  areaGradient.addColorStop(0, 'rgba(56, 189, 248, 0.35)');
  areaGradient.addColorStop(1, 'rgba(56, 189, 248, 0.02)');
  ctx.beginPath();
  ctx.moveTo(projected[0].x, plotY + plotHeight);
  projected.forEach((point) => ctx.lineTo(point.x, point.y));
  ctx.lineTo(projected[projected.length - 1].x, plotY + plotHeight);
  ctx.closePath();
  ctx.fillStyle = areaGradient;
  ctx.fill();

  const lineGradient = ctx.createLinearGradient(plotX, plotY, plotX + plotWidth, plotY + plotHeight);
  lineGradient.addColorStop(0, '#38BDF8');
  lineGradient.addColorStop(1, projected[projected.length - 1].value >= 0 ? '#22C55E' : '#FB7185');
  ctx.strokeStyle = lineGradient;
  ctx.lineWidth = 6;
  ctx.lineJoin = 'round';
  ctx.lineCap = 'round';
  ctx.beginPath();
  projected.forEach((point, index) => {
    if (index === 0) ctx.moveTo(point.x, point.y);
    else ctx.lineTo(point.x, point.y);
  });
  ctx.stroke();

  const lastPoint = projected[projected.length - 1];
  ctx.fillStyle = projected[projected.length - 1].value >= 0 ? '#22C55E' : '#FB7185';
  ctx.beginPath();
  ctx.arc(lastPoint.x, lastPoint.y, 10, 0, Math.PI * 2);
  ctx.fill();
  ctx.fillStyle = '#ffffff';
  ctx.beginPath();
  ctx.arc(lastPoint.x, lastPoint.y, 4, 0, Math.PI * 2);
  ctx.fill();

  const topLabel = formatPctSafe(yMax);
  const bottomLabel = formatPctSafe(yMin);
  ctx.textAlign = 'right';
  ctx.textBaseline = 'middle';
  ctx.font = '500 20px Inter, system-ui, sans-serif';
  ctx.fillStyle = 'rgba(226, 232, 240, 0.64)';
  ctx.fillText(topLabel, plotX + plotWidth, plotY - 18);
  ctx.fillText(bottomLabel, plotX + plotWidth, plotY + plotHeight + 22);

  ctx.textAlign = 'left';
  ctx.textBaseline = 'middle';
  ctx.fillText('开单', plotX, plotY + plotHeight + 22);
  ctx.textAlign = 'right';
  ctx.fillText('当前', plotX + plotWidth, plotY + plotHeight + 22);
}

async function renderPoster(canvas, data, loginUser) {
  if (!canvas || !data) return;
  canvas.width = POSTER_WIDTH;
  canvas.height = POSTER_HEIGHT;

  const ctx = canvas.getContext('2d');
  if (!ctx) return;

  const [userImage, tokenImage] = await Promise.all([
    loadImage(loginUser?.photo_url || ''),
    loadImage(data?.theme_token?.logo_url || ''),
  ]);

  ctx.clearRect(0, 0, POSTER_WIDTH, POSTER_HEIGHT);
  const background = ctx.createLinearGradient(0, 0, POSTER_WIDTH, POSTER_HEIGHT);
  background.addColorStop(0, '#07111F');
  background.addColorStop(0.55, '#101B34');
  background.addColorStop(1, '#050914');
  ctx.fillStyle = background;
  ctx.fillRect(0, 0, POSTER_WIDTH, POSTER_HEIGHT);

  const flare = ctx.createRadialGradient(920, 120, 40, 920, 120, 300);
  flare.addColorStop(0, 'rgba(96, 165, 250, 0.38)');
  flare.addColorStop(1, 'rgba(96, 165, 250, 0)');
  ctx.fillStyle = flare;
  ctx.fillRect(0, 0, POSTER_WIDTH, POSTER_HEIGHT);

  const glow = ctx.createRadialGradient(160, 220, 20, 160, 220, 260);
  glow.addColorStop(0, 'rgba(168, 85, 247, 0.28)');
  glow.addColorStop(1, 'rgba(168, 85, 247, 0)');
  ctx.fillStyle = glow;
  ctx.fillRect(0, 0, POSTER_WIDTH, POSTER_HEIGHT);

  fillRoundRect(ctx, 42, 42, POSTER_WIDTH - 84, POSTER_HEIGHT - 84, 40, 'rgba(7, 15, 28, 0.55)', 'rgba(148, 163, 184, 0.12)');

  drawAvatar(ctx, userImage, 84, 84, 92, toDisplayName(loginUser), ['#2563EB', '#9333EA']);
  drawAvatar(ctx, tokenImage, POSTER_WIDTH - 204, 88, 120, data?.theme_token?.symbol || data?.pair || 'LP', ['#0EA5E9', '#22C55E']);

  ctx.fillStyle = 'rgba(148, 163, 184, 0.85)';
  ctx.font = '500 24px Inter, system-ui, sans-serif';
  ctx.textAlign = 'left';
  ctx.textBaseline = 'top';
  ctx.fillText('TgLPBot · Profit Poster', 198, 92);

  ctx.fillStyle = '#F8FAFC';
  ctx.font = '700 40px Inter, system-ui, sans-serif';
  ctx.fillText(toDisplayName(loginUser), 198, 126);

  ctx.fillStyle = 'rgba(226, 232, 240, 0.72)';
  ctx.font = '500 26px Inter, system-ui, sans-serif';
  ctx.fillText(`${String(data?.chain || '--').toUpperCase()} · ${String(data?.exchange || 'LP Position').trim()}`, 198, 176);

  ctx.fillStyle = '#F8FAFC';
  ctx.font = '800 68px Inter, system-ui, sans-serif';
  ctx.fillText(String(data?.pair || 'LP Position'), 84, 286);

  ctx.fillStyle = 'rgba(226, 232, 240, 0.72)';
  ctx.font = '500 28px Inter, system-ui, sans-serif';
  ctx.fillText(data?.theme_token?.name || data?.theme_token?.symbol || 'Token', 84, 360);

  const profit = Number(data?.profit_usd ?? 0);
  const profitColor = profit >= 0 ? '#4ADE80' : '#FB7185';
  ctx.fillStyle = 'rgba(148, 163, 184, 0.82)';
  ctx.font = '500 28px Inter, system-ui, sans-serif';
  ctx.fillText('未实现利润', 84, 458);
  ctx.fillStyle = profitColor;
  ctx.font = '800 98px Inter, system-ui, sans-serif';
  ctx.fillText(`${profit >= 0 ? '+' : ''}${formatUsdSafe(profit)}`, 84, 500);

  drawStatCard(ctx, {
    x: 84,
    y: 690,
    width: 286,
    height: 156,
    label: '本次投入',
    value: formatUsdSafe(data?.invest_usd),
    tone: 'accent',
  });
  drawStatCard(ctx, {
    x: 397,
    y: 690,
    width: 286,
    height: 156,
    label: '当前盈利',
    value: formatUsdSafe(data?.profit_usd),
    tone: profit >= 0 ? 'positive' : 'negative',
  });
  drawStatCard(ctx, {
    x: 710,
    y: 690,
    width: 286,
    height: 156,
    label: '收益率',
    value: formatPctSafe(data?.profit_pct),
    tone: profit >= 0 ? 'positive' : 'negative',
  });

  drawChart(ctx, data?.series, 84, 884, POSTER_WIDTH - 168, 320);

  ctx.fillStyle = 'rgba(226, 232, 240, 0.7)';
  ctx.font = '500 24px Inter, system-ui, sans-serif';
  ctx.textAlign = 'left';
  ctx.textBaseline = 'middle';
  ctx.fillText(`开单时间：${formatPosterTime(data?.opened_at)}`, 84, 1264);
  ctx.fillText(`生成时间：${formatPosterTime(data?.generated_at)}`, 84, 1300);

  ctx.textAlign = 'right';
  ctx.fillText(data?.window_label || '开单以来价格收益', POSTER_WIDTH - 84, 1264);
  ctx.fillText('数据源：OKX Market', POSTER_WIDTH - 84, 1300);
}

export default function PositionProfitPosterModal({
  task,
  data,
  loading,
  error,
  loginUser,
  onClose,
  onRetry,
}) {
  const canvasRef = useRef(null);
  const [rendering, setRendering] = useState(false);
  const [renderError, setRenderError] = useState('');

  const filename = useMemo(() => {
    const pair = String(data?.pair || task?.title || 'position')
      .replace(/[^\w\u4e00-\u9fa5-]+/g, '_')
      .replace(/^_+|_+$/g, '');
    return `${pair || 'position'}-task-${task?.task_id || data?.task_id || 'poster'}.png`;
  }, [data?.pair, data?.task_id, task?.task_id, task?.title]);

  useEffect(() => {
    let alive = true;
    if (!data || loading) return undefined;

    setRendering(true);
    setRenderError('');
    renderPoster(canvasRef.current, data, loginUser)
      .catch((err) => {
        if (!alive) return;
        setRenderError(String(err?.message || err));
      })
      .finally(() => {
        if (alive) setRendering(false);
      });

    return () => {
      alive = false;
    };
  }, [data, loading, loginUser]);

  const handleDownload = () => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    if (canvas.toBlob) {
      canvas.toBlob((blob) => {
        if (!blob) return;
        const href = URL.createObjectURL(blob);
        const link = document.createElement('a');
        link.href = href;
        link.download = filename;
        link.click();
        setTimeout(() => URL.revokeObjectURL(href), 1000);
      }, 'image/png');
      return;
    }
    const href = canvas.toDataURL('image/png');
    const link = document.createElement('a');
    link.href = href;
    link.download = filename;
    link.click();
  };

  return (
    <div className="modal-overlay ppm-overlay" onClick={onClose}>
      <div className="ppm-shell" onClick={(event) => event.stopPropagation()}>
        <div className="ppm-header">
          <div>
            <h3>收益海报</h3>
            <p>{data?.pair || task?.title || '当前仓位'}</p>
          </div>
          <button type="button" className="modal-close" onClick={onClose}>&times;</button>
        </div>

        <div className="ppm-body">
          <div className="ppm-preview-wrap">
            {(loading || rendering) && (
              <div className="ppm-canvas-state">正在生成收益海报...</div>
            )}
            {!loading && !error && !renderError && (
              <canvas ref={canvasRef} className="ppm-canvas" />
            )}
            {!loading && (error || renderError) && (
              <div className="ppm-canvas-state ppm-canvas-state--error">
                {String(error || renderError)}
              </div>
            )}
          </div>

          <div className="ppm-side">
            <div className="ppm-side-card">
              <div className="ppm-side-title">核心数据</div>
              <div className="ppm-side-grid">
                <div>
                  <span>投入</span>
                  <strong>{formatUsdSafe(data?.invest_usd)}</strong>
                </div>
                <div>
                  <span>未实现利润</span>
                  <strong className={Number(data?.profit_usd ?? 0) >= 0 ? 'positive' : 'negative'}>
                    {formatUsdSafe(data?.profit_usd)}
                  </strong>
                </div>
                <div>
                  <span>盈利百分比</span>
                  <strong className={Number(data?.profit_pct ?? 0) >= 0 ? 'positive' : 'negative'}>
                    {formatPctSafe(data?.profit_pct)}
                  </strong>
                </div>
                <div>
                  <span>代币</span>
                  <strong>{data?.theme_token?.name || data?.theme_token?.symbol || '--'}</strong>
                </div>
              </div>
            </div>

            {Array.isArray(data?.warnings) && data.warnings.length > 0 ? (
              <div className="ppm-side-card ppm-side-card--warn">
                <div className="ppm-side-title">提示</div>
                <ul className="ppm-warning-list">
                  {data.warnings.map((item, index) => (
                    <li key={`${item}-${index}`}>{item}</li>
                  ))}
                </ul>
              </div>
            ) : null}

            <div className="ppm-side-actions">
              <button
                type="button"
                className="accent-btn"
                onClick={handleDownload}
                disabled={loading || rendering || !!error || !!renderError}
              >
                下载 PNG
              </button>
              <button type="button" className="ghost-chip ppm-retry-btn" onClick={onRetry} disabled={loading}>
                重新生成
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
