import React, { useMemo } from 'react';
import NumberFlow from '@number-flow/react';

const NUM_SEGMENT_RE = /[+-]?(?:\d{1,3}(?:,\d{3})+|\d+)(?:\.\d+)?/g;

function isAsciiWordChar(ch) {
    if (!ch) return false;
    return /[A-Za-z0-9_]/.test(ch);
}

function parseNumberSegment(raw) {
    if (!raw) return null;
    const sign = raw[0] === '+' || raw[0] === '-' ? raw[0] : '';
    const unsigned = sign ? raw.slice(1) : raw;
    const parts = unsigned.split('.');
    const intPartRaw = parts[0] || '';
    const fracPartRaw = parts[1] || '';
    const clean = `${sign}${unsigned.replace(/,/g, '')}`;
    const value = Number(clean);
    if (!Number.isFinite(value)) return null;

    const format = {
        useGrouping: intPartRaw.includes(','),
    };

    if (fracPartRaw.length > 0) {
        format.minimumFractionDigits = fracPartRaw.length;
        format.maximumFractionDigits = fracPartRaw.length;
    }

    if (sign === '+') {
        format.signDisplay = 'always';
    }

    const intDigits = intPartRaw.replace(/,/g, '');
    if (intDigits.length > 1 && intDigits.startsWith('0')) {
        format.minimumIntegerDigits = intDigits.length;
    }

    return { value, format };
}

function splitTextSegments(text) {
    const input = String(text ?? '');
    if (!input) return [];

    const out = [];
    let last = 0;
    let match;
    while ((match = NUM_SEGMENT_RE.exec(input)) !== null) {
        const raw = match[0];
        const start = match.index;
        const end = start + raw.length;

        const prev = start > 0 ? input[start - 1] : '';
        const next = end < input.length ? input[end] : '';
        const looksLikeHexPrefix = raw === '0' && (next === 'x' || next === 'X');
        if (looksLikeHexPrefix) {
            continue;
        }
        if (isAsciiWordChar(prev) && isAsciiWordChar(next)) {
            continue;
        }

        if (start > last) {
            out.push({ type: 'text', key: `t-${last}-${start}`, text: input.slice(last, start) });
        }
        const parsed = parseNumberSegment(raw);
        if (!parsed) {
            out.push({ type: 'text', key: `t-${start}-${end}`, text: raw });
        } else {
            out.push({ type: 'number', key: `n-${start}-${end}-${raw}`, ...parsed });
        }
        last = end;
    }

    if (last < input.length) {
        out.push({ type: 'text', key: `t-${last}-${input.length}`, text: input.slice(last) });
    }

    return out;
}

export default function NumberFlowValue({
    value,
    className = '',
    fallback = '--',
    formatter,
    locale = 'en-US',
    formatOptions,
    durationMs = 420,
}) {
    const numberFormatter = useMemo(() => {
        if (typeof formatter === 'function') return null;
        return new Intl.NumberFormat(locale, formatOptions || {});
    }, [formatter, locale, formatOptions]);

    const text = useMemo(() => {
        if (typeof formatter === 'function') {
            const out = formatter(value);
            if (out === null || out === undefined) return String(fallback);
            return String(out);
        }
        const n = Number(value);
        if (!Number.isFinite(n)) return String(fallback);
        if (!numberFormatter) return String(n);
        return numberFormatter.format(n);
    }, [formatter, value, fallback, numberFormatter]);

    const timing = useMemo(() => ({
        duration: durationMs,
        easing: 'cubic-bezier(0.22, 1, 0.36, 1)',
        fill: 'both',
    }), [durationMs]);
    const rootClassName = `inline-flex items-center tabular-nums lining-nums ${className}`.trim();

    if (typeof formatter !== 'function') {
        const n = Number(value);
        if (!Number.isFinite(n)) {
            return <span className={rootClassName}>{text}</span>;
        }
        return (
            <span className={rootClassName} aria-label={text}>
                <NumberFlow
                    value={n}
                    locales={locale}
                    format={formatOptions}
                    transformTiming={timing}
                    spinTiming={timing}
                    opacityTiming={timing}
                />
            </span>
        );
    }

    const segments = splitTextSegments(text);
    const hasNumber = segments.some((s) => s.type === 'number');
    if (!hasNumber) {
        return <span className={rootClassName}>{text}</span>;
    }

    return (
        <span className={rootClassName} aria-label={text}>
            {segments.map((seg) => {
                if (seg.type === 'number') {
                    return (
                        <NumberFlow
                            key={seg.key}
                            value={seg.value}
                            locales={locale}
                            format={seg.format}
                            transformTiming={timing}
                            spinTiming={timing}
                            opacityTiming={timing}
                        />
                    );
                }
                return (
                    <span key={seg.key} className="inline-flex items-center">
                        {seg.text}
                    </span>
                );
            })}
        </span>
    );
}
