import React from 'react';

const TONE_CLASS = {
    ok: 'bg-emerald-500',
    warn: 'bg-amber-500',
    danger: 'bg-red-500',
    idle: 'bg-zinc-400 dark:bg-zinc-500',
    accent: 'bg-sky-500',
};

const SIZE_CLASS = {
    xs: 'h-1.5 w-1.5',
    sm: 'h-2 w-2',
    md: 'h-2.5 w-2.5',
    lg: 'h-3 w-3',
};

export default function StatusDot({ tone = 'idle', pulse = false, size = 'sm', className = '' }) {
    const dot = TONE_CLASS[tone] || TONE_CLASS.idle;
    const dim = SIZE_CLASS[size] || SIZE_CLASS.sm;
    return (
        <span className={`relative inline-flex shrink-0 ${dim} ${className}`}>
            {pulse ? (
                <span className={`absolute inset-0 rounded-full ${dot} opacity-60 motion-safe:animate-ping`} />
            ) : null}
            <span className={`relative inline-block rounded-full ${dim} ${dot} ring-2 ring-white/70 dark:ring-[#0c0e12]`} />
        </span>
    );
}
