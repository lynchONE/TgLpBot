import React, { useEffect, useMemo, useState } from 'react';

const DIGITS = Array.from({ length: 50 }, (_, i) => i % 10);

function clampDigit(n) {
    const v = Number(n);
    if (!Number.isFinite(v)) return 0;
    if (v < 0) return 0;
    if (v > 9) return 9;
    return Math.round(v);
}

function buildTokens(text) {
    const input = String(text ?? '');
    if (!input) return [];

    const decimalIndex = input.lastIndexOf('.');
    const integerEnd = decimalIndex >= 0 ? decimalIndex : input.length;
    const integerPlaces = new Array(integerEnd).fill(-1);

    let place = 0;
    for (let i = integerEnd - 1; i >= 0; i -= 1) {
        const ch = input[i];
        if (ch >= '0' && ch <= '9') {
            integerPlaces[i] = place;
            place += 1;
        }
    }

    let fracPlace = 0;
    const out = [];
    for (let i = 0; i < input.length; i += 1) {
        const ch = input[i];
        const isDigit = ch >= '0' && ch <= '9';
        if (!isDigit) {
            out.push({ type: 'char', key: `c-${i}-${ch}`, char: ch });
            continue;
        }
        if (i < integerEnd) {
            out.push({ type: 'digit', key: `i-${integerPlaces[i]}`, digit: Number(ch) });
            continue;
        }
        fracPlace += 1;
        out.push({ type: 'digit', key: `f-${fracPlace}`, digit: Number(ch) });
    }
    return out;
}

function DigitWheel({ digit, durationMs = 420 }) {
    const targetDigit = clampDigit(digit);
    const [offset, setOffset] = useState(20 + targetDigit);

    useEffect(() => {
        setOffset((prev) => {
            const prevDigit = ((prev % 10) + 10) % 10;
            let delta = targetDigit - prevDigit;
            if (delta > 5) delta -= 10;
            if (delta < -5) delta += 10;
            let next = prev + delta;
            while (next < 10) next += 10;
            while (next > 39) next -= 10;
            return next;
        });
    }, [targetDigit]);

    return (
        <span className="relative inline-flex h-[1em] w-[0.62em] overflow-hidden">
            <span
                className="absolute left-0 top-0 flex flex-col leading-none transition-transform will-change-transform"
                style={{
                    transform: `translateY(-${offset}em)`,
                    transitionDuration: `${durationMs}ms`,
                    transitionTimingFunction: 'cubic-bezier(0.22, 1, 0.36, 1)',
                }}
                aria-hidden="true"
            >
                {DIGITS.map((n, idx) => (
                    <span key={idx} className="flex h-[1em] items-center justify-center leading-none">
                        {n}
                    </span>
                ))}
            </span>
        </span>
    );
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

    const tokens = useMemo(() => buildTokens(text), [text]);
    const hasDigit = tokens.some((t) => t.type === 'digit');
    const rootClassName = `inline-flex items-center align-[-0.08em] tabular-nums lining-nums leading-none ${className}`.trim();

    if (!hasDigit) {
        return <span className={rootClassName}>{text}</span>;
    }

    return (
        <span className={rootClassName} aria-label={text}>
            {tokens.map((token) => {
                if (token.type === 'digit') {
                    return <DigitWheel key={token.key} digit={token.digit} durationMs={durationMs} />;
                }
                return (
                    <span key={token.key} className="inline-block">
                        {token.char}
                    </span>
                );
            })}
        </span>
    );
}
