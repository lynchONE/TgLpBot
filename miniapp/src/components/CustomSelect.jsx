import { useEffect, useRef, useState } from 'react';

export default function CustomSelect({
    value,
    onChange,
    options = [],
    placeholder = '请选择',
    className = '',
    disabled = false,
}) {
    const [open, setOpen] = useState(false);
    const ref = useRef(null);

    useEffect(() => {
        if (!open) return;
        const handler = (e) => {
            if (ref.current && !ref.current.contains(e.target)) setOpen(false);
        };
        document.addEventListener('pointerdown', handler);
        return () => document.removeEventListener('pointerdown', handler);
    }, [open]);

    const selected = options.find((o) => o.value === value);

    return (
        <div ref={ref} className={`relative ${className}`}>
            <button
                type="button"
                disabled={disabled}
                onClick={() => !disabled && setOpen((v) => !v)}
                className={`flex w-full items-center justify-between gap-2 rounded-xl border px-3 py-2.5 text-sm transition-colors
                    ${disabled
                        ? 'cursor-not-allowed border-zinc-200/50 bg-zinc-100/50 text-zinc-400 dark:border-white/5 dark:bg-white/[0.02] dark:text-white/30'
                        : 'border-zinc-200 bg-white/70 text-zinc-900 hover:bg-white dark:border-white/10 dark:bg-white/5 dark:text-white/90 dark:hover:bg-white/10'
                    }`}
            >
                <span className="truncate">
                    {selected ? (
                        <span className="flex items-center gap-2">
                            {selected.icon && <span className="text-base">{selected.icon}</span>}
                            {selected.label}
                        </span>
                    ) : (
                        <span className="text-zinc-400 dark:text-white/30">{placeholder}</span>
                    )}
                </span>
                <svg
                    className={`h-4 w-4 shrink-0 text-zinc-400 transition-transform dark:text-white/40 ${open ? 'rotate-180' : ''}`}
                    fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth="2"
                >
                    <path strokeLinecap="round" strokeLinejoin="round" d="M19 9l-7 7-7-7" />
                </svg>
            </button>

            {open && (
                <div className="absolute left-0 right-0 top-full z-50 mt-1 max-h-60 overflow-y-auto rounded-xl border border-zinc-200 bg-white shadow-xl dark:border-white/10 dark:bg-[#1a1d24]">
                    {options.map((opt) => (
                        <button
                            key={opt.value}
                            type="button"
                            onClick={() => {
                                onChange(opt.value);
                                setOpen(false);
                            }}
                            className={`flex w-full items-center gap-2 px-3 py-2.5 text-left text-sm transition-colors
                                ${opt.value === value
                                    ? 'bg-emerald-50 font-semibold text-emerald-700 dark:bg-emerald-500/10 dark:text-emerald-400'
                                    : 'text-zinc-700 hover:bg-zinc-50 dark:text-white/80 dark:hover:bg-white/5'
                                }`}
                        >
                            {opt.icon && <span className="text-base">{opt.icon}</span>}
                            <span className="truncate">{opt.label}</span>
                            {opt.value === value && (
                                <svg className="ml-auto h-4 w-4 shrink-0 text-emerald-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth="2.5">
                                    <path strokeLinecap="round" strokeLinejoin="round" d="M5 13l4 4L19 7" />
                                </svg>
                            )}
                        </button>
                    ))}
                </div>
            )}
        </div>
    );
}

