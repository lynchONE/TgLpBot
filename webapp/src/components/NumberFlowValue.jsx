import { useMemo } from 'react';
import NumberFlow from '@number-flow/react';

const NUM_RE = /[+-]?(?:\d{1,3}(?:,\d{3})+|\d+)(?:\.\d+)?/g;

function parseNum(raw) {
  if (!raw) return null;
  const sign = raw[0] === '+' || raw[0] === '-' ? raw[0] : '';
  const unsigned = sign ? raw.slice(1) : raw;
  const parts = unsigned.split('.');
  const intPart = parts[0] || '';
  const fracPart = parts[1] || '';
  const value = Number(`${sign}${unsigned.replace(/,/g, '')}`);
  if (!Number.isFinite(value)) return null;

  const format = { useGrouping: intPart.includes(',') };
  if (fracPart.length > 0) {
    format.minimumFractionDigits = fracPart.length;
    format.maximumFractionDigits = fracPart.length;
  }
  if (sign === '+') format.signDisplay = 'always';
  return { value, format };
}

function splitSegments(text) {
  const input = String(text ?? '');
  if (!input) return [];
  const out = [];
  let last = 0;
  let m;
  while ((m = NUM_RE.exec(input)) !== null) {
    const raw = m[0];
    const start = m.index;
    const end = start + raw.length;
    const prev = start > 0 ? input[start - 1] : '';
    const next = end < input.length ? input[end] : '';
    if (raw === '0' && (next === 'x' || next === 'X')) continue;
    if (/\w/.test(prev) && /\w/.test(next)) continue;
    if (start > last) out.push({ t: 's', k: `s${last}`, v: input.slice(last, start) });
    const p = parseNum(raw);
    if (!p) { out.push({ t: 's', k: `s${start}`, v: raw }); }
    else { out.push({ t: 'n', k: `n${start}`, ...p }); }
    last = end;
  }
  if (last < input.length) out.push({ t: 's', k: `s${last}`, v: input.slice(last) });
  return out;
}

const TIMING = { duration: 400, easing: 'cubic-bezier(0.22,1,0.36,1)', fill: 'both' };

export default function NumberFlowValue({ value, formatter, fallback = '--', className = '' }) {
  const text = useMemo(() => {
    if (typeof formatter === 'function') {
      const out = formatter(value);
      return out == null ? String(fallback) : String(out);
    }
    const n = Number(value);
    return Number.isFinite(n) ? String(n) : String(fallback);
  }, [value, formatter, fallback]);

  const segments = splitSegments(text);
  const hasNum = segments.some((s) => s.t === 'n');

  if (!hasNum) return <span className={className}>{text}</span>;

  return (
    <span className={className} aria-label={text}>
      {segments.map((seg) =>
        seg.t === 'n' ? (
          <NumberFlow
            key={seg.k}
            value={seg.value}
            locales="en-US"
            format={seg.format}
            transformTiming={TIMING}
            spinTiming={TIMING}
            opacityTiming={TIMING}
            animated
            respectMotionPreference={false}
          />
        ) : (
          <span key={seg.k}>{seg.v}</span>
        )
      )}
    </span>
  );
}
