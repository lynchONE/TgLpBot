export const ACCENT_THEME_OPTIONS = [
    { key: 'lime', label: '新绿' },
    { key: 'emerald', label: '原绿' },
];

export function normalizeAccentTheme(value) {
    return String(value || '').trim().toLowerCase() === 'emerald' ? 'emerald' : 'lime';
}

const SHARED_TOKENS = {
    cardClass:
        'rounded-2xl border border-zinc-200/80 bg-white shadow-sm dark:border-white/10 dark:bg-[#14171c] dark:shadow-none',
    cardElevatedClass:
        'rounded-2xl border border-zinc-200/80 bg-white shadow-[0_8px_24px_-12px_rgba(15,23,42,0.18)] dark:border-white/10 dark:bg-[#14171c] dark:shadow-[0_8px_24px_-12px_rgba(0,0,0,0.6)]',
    cardCompactClass:
        'rounded-xl border border-zinc-200/70 bg-white/95 dark:border-white/10 dark:bg-[#14171c]/95',
    cardInsetClass:
        'rounded-xl border border-zinc-200/60 bg-zinc-50/80 dark:border-white/5 dark:bg-[#1c2026]/80',
    inputClass:
        'min-h-[40px] rounded-xl border border-zinc-200 bg-white px-3 text-sm text-zinc-900 outline-none transition placeholder:text-zinc-400 disabled:cursor-not-allowed disabled:opacity-60 dark:border-white/10 dark:bg-[#1c2026] dark:text-white/90 dark:placeholder:text-white/30',
    buttonSecondaryClass:
        'inline-flex items-center justify-center gap-1.5 rounded-xl border border-zinc-200 bg-white px-3 py-2 text-sm font-semibold text-zinc-700 shadow-sm transition hover:bg-zinc-50 active:scale-[0.98] disabled:cursor-not-allowed disabled:opacity-50 dark:border-white/10 dark:bg-white/5 dark:text-white/80 dark:hover:bg-white/10',
    buttonGhostClass:
        'inline-flex items-center justify-center gap-1.5 rounded-xl px-3 py-2 text-sm font-semibold text-zinc-600 transition hover:bg-zinc-100 active:scale-[0.98] disabled:cursor-not-allowed disabled:opacity-50 dark:text-white/70 dark:hover:bg-white/10',
    mutedTextClass: 'text-zinc-500 dark:text-white/55',
    subtleTextClass: 'text-zinc-400 dark:text-white/40',
    dividerClass: 'border-zinc-200/70 dark:border-white/10',
    skeletonClass: 'bg-zinc-100 dark:bg-white/[0.06]',
    pnlPosClass: 'text-emerald-600 dark:text-[#34d399]',
    pnlNegClass: 'text-rose-600 dark:text-[#f87171]',
    errorBoxClass:
        'rounded-xl border border-rose-200 bg-rose-50/80 px-3 py-2 text-sm text-rose-700 dark:border-rose-500/30 dark:bg-rose-500/10 dark:text-rose-200',
    emptyStateClass:
        'flex flex-col items-center justify-center gap-2 rounded-2xl border border-dashed border-zinc-200 bg-white/60 px-6 py-10 text-center text-sm text-zinc-500 dark:border-white/10 dark:bg-[#14171c]/60 dark:text-white/55',
};

const BRAND_THEMES = {
    lime: {
        key: 'lime',
        iconChipClass: 'bg-[#bcff2f]/10 text-[#6f9616] ring-1 ring-[#bcff2f]/20 dark:bg-[#bcff2f]/15 dark:text-[#e3ffa0] dark:ring-[#bcff2f]/25',
        softButtonClass: 'bg-[#bcff2f]/15 text-[#6f9616] ring-1 ring-[#bcff2f]/25 hover:bg-[#bcff2f]/20 dark:bg-[#bcff2f]/10 dark:text-[#e3ffa0] dark:ring-[#bcff2f]/25 dark:hover:bg-[#bcff2f]/15',
        solidButtonClass: 'bg-[#bcff2f] text-[#182108] hover:bg-[#c8ff55] active:bg-[#a9ec22]',
        solidRingClass: 'ring-1 ring-[#bcff2f]/30',
        gradientButtonClass: 'bg-gradient-to-r from-[#bcff2f] to-[#8fda21] text-[#182108] shadow-md shadow-[#bcff2f]/20 ring-1 ring-black/5 dark:from-[#bcff2f] dark:to-[#8fda21] dark:text-[#182108] dark:shadow-[#bcff2f]/20 dark:ring-white/10',
        textClass: 'text-[#6f9616] dark:text-[#bcff2f]',
        inputFocusClass: 'focus:border-[#bcff2f] dark:focus:border-[#bcff2f]',
        selectionClass: 'border-[#bcff2f]/40 bg-[#bcff2f]/10 ring-1 ring-[#bcff2f]/20 dark:border-[#bcff2f]/30 dark:bg-[#bcff2f]/10 dark:ring-[#bcff2f]/20',
        dotClass: 'bg-[#bcff2f]',
        successNoticeClass: 'bg-[#bcff2f] text-[#182108]',
        navActiveClass: 'bg-[#bcff2f]/12 text-[#6f9616] dark:bg-[#bcff2f]/10 dark:text-[#bcff2f]',
        actionPillButtonClass: 'inline-flex items-center gap-1.5 rounded-full border border-black/70 bg-[linear-gradient(180deg,#303811_0%,#252d0d_100%)] px-3 py-1 text-[10px] font-semibold leading-none text-[#bcff2f] shadow-[inset_0_1px_0_rgba(255,255,255,0.03)] transition hover:bg-[linear-gradient(180deg,#353f14_0%,#2a3210_100%)] disabled:cursor-not-allowed disabled:opacity-50',
        focusRingClass: 'focus-visible:ring-2 focus-visible:ring-[#bcff2f]/40 focus-visible:ring-offset-0',
        ...SHARED_TOKENS,
    },
    emerald: {
        key: 'emerald',
        iconChipClass: 'bg-emerald-500/10 text-emerald-700 ring-1 ring-emerald-500/20 dark:bg-emerald-500/15 dark:text-emerald-300 dark:ring-emerald-500/25',
        softButtonClass: 'bg-emerald-500/15 text-emerald-700 ring-1 ring-emerald-500/25 hover:bg-emerald-500/20 dark:bg-emerald-500/10 dark:text-emerald-200 dark:ring-emerald-500/25 dark:hover:bg-emerald-500/15',
        solidButtonClass: 'bg-emerald-500 text-white hover:bg-emerald-600 active:bg-emerald-700',
        solidRingClass: 'ring-1 ring-emerald-500/30',
        gradientButtonClass: 'bg-gradient-to-r from-emerald-400 to-teal-500 text-white shadow-md shadow-emerald-500/20 dark:from-emerald-500 dark:to-teal-600 dark:text-white dark:shadow-emerald-900/40 ring-1 ring-black/5 dark:ring-white/10',
        textClass: 'text-emerald-700 dark:text-emerald-300',
        inputFocusClass: 'focus:border-emerald-400 dark:focus:border-emerald-400',
        selectionClass: 'border-emerald-500/40 bg-emerald-500/10 ring-1 ring-emerald-500/20 dark:border-emerald-400/30 dark:bg-emerald-500/10 dark:ring-emerald-400/20',
        dotClass: 'bg-emerald-400',
        successNoticeClass: 'bg-emerald-600 text-white',
        navActiveClass: 'bg-emerald-50 text-emerald-600 dark:bg-emerald-500/10 dark:text-emerald-400',
        actionPillButtonClass: 'inline-flex items-center gap-1.5 rounded-full border border-black/70 bg-[linear-gradient(180deg,#143328_0%,#0d241c_100%)] px-3 py-1 text-[10px] font-semibold leading-none text-emerald-300 shadow-[inset_0_1px_0_rgba(255,255,255,0.03)] transition hover:bg-[linear-gradient(180deg,#184030_0%,#103127_100%)] disabled:cursor-not-allowed disabled:opacity-50',
        focusRingClass: 'focus-visible:ring-2 focus-visible:ring-emerald-400/40 focus-visible:ring-offset-0',
        ...SHARED_TOKENS,
    },
};

export function getBrandTheme(value) {
    return BRAND_THEMES[normalizeAccentTheme(value)];
}
