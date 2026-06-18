export const ACCENT_THEME_OPTIONS = [
    { key: 'lime', label: '新绿' },
    { key: 'emerald', label: '原绿' },
];

export function normalizeAccentTheme(value) {
    return String(value || '').trim().toLowerCase() === 'emerald' ? 'emerald' : 'lime';
}

const SHARED_TOKENS = {
    cardClass:
        'rounded-lg border border-white/[0.08] bg-[#080808] shadow-none',
    cardElevatedClass:
        'rounded-lg border border-white/[0.08] bg-[#080808] shadow-none',
    cardCompactClass:
        'rounded-lg border border-white/[0.08] bg-[#0b0b0b]',
    cardInsetClass:
        'rounded-lg border border-white/[0.06] bg-[#101010]',
    inputClass:
        'min-h-[40px] rounded-lg border border-white/[0.09] bg-[#080808] px-3 text-sm text-white outline-none transition placeholder:text-white/25 disabled:cursor-not-allowed disabled:opacity-60 focus:border-[#b7ff1a]/50 focus:ring-2 focus:ring-[#b7ff1a]/15',
    buttonSecondaryClass:
        'inline-flex items-center justify-center gap-1.5 rounded-md border border-white/[0.09] bg-[#111] px-3 py-2 text-sm font-semibold text-white/70 shadow-none transition hover:border-white/20 hover:bg-[#191919] hover:text-white disabled:cursor-not-allowed disabled:opacity-50',
    buttonGhostClass:
        'inline-flex items-center justify-center gap-1.5 rounded-md px-3 py-2 text-sm font-semibold text-white/60 transition hover:bg-white/[0.06] hover:text-white disabled:cursor-not-allowed disabled:opacity-50',
    mutedTextClass: 'text-white/50',
    subtleTextClass: 'text-white/35',
    dividerClass: 'border-white/[0.08]',
    skeletonClass: 'bg-white/[0.06]',
    pnlPosClass: 'text-[#9cff00]',
    pnlNegClass: 'text-[#ff4fb3]',
    errorBoxClass:
        'rounded-lg border border-[#ff4fb3]/30 bg-[#ff4fb3]/10 px-3 py-2 text-sm text-[#ffb8df]',
    emptyStateClass:
        'flex flex-col items-center justify-center gap-2 rounded-lg border border-dashed border-white/[0.09] bg-[#080808] px-6 py-10 text-center text-sm text-white/45',
};

const BRAND_THEMES = {
    lime: {
        key: 'lime',
        iconChipClass: 'bg-[#b7ff1a]/12 text-[#d9ff63] ring-1 ring-[#b7ff1a]/25',
        softButtonClass: 'border border-[#b7ff1a]/28 bg-[#b7ff1a]/10 text-[#d9ff63] ring-0 hover:bg-[#b7ff1a]/14',
        solidButtonClass: 'border border-[#b7ff1a]/44 bg-[#b7ff1a] text-[#050700] hover:bg-[#c8ff3d] active:bg-[#9cff00]',
        solidRingClass: 'ring-1 ring-[#b7ff1a]/28',
        gradientButtonClass: 'border border-[#b7ff1a]/44 bg-[#b7ff1a] text-[#050700] shadow-none ring-0 hover:bg-[#c8ff3d]',
        textClass: 'text-[#d9ff63]',
        inputFocusClass: 'focus:border-[#b7ff1a]/60 focus:ring-2 focus:ring-[#b7ff1a]/15',
        selectionClass: 'border-[#b7ff1a]/45 bg-[#b7ff1a]/11 ring-1 ring-[#b7ff1a]/18 text-[#d9ff63]',
        dotClass: 'bg-[#b7ff1a]',
        successNoticeClass: 'bg-[#b7ff1a] text-[#050700]',
        navActiveClass: 'bg-[#b7ff1a] text-[#050700]',
        actionPillButtonClass: 'inline-flex items-center gap-1 rounded-full border border-[#b7ff1a]/44 bg-[#b7ff1a] px-2.5 py-1 text-[10px] font-bold leading-none text-[#050700] shadow-none transition hover:bg-[#c8ff3d] active:bg-[#9cff00] disabled:cursor-not-allowed disabled:opacity-50',
        focusRingClass: 'focus-visible:ring-2 focus-visible:ring-[#b7ff1a]/40 focus-visible:ring-offset-0',
        ...SHARED_TOKENS,
    },
    emerald: {
        key: 'emerald',
        iconChipClass: 'bg-[#9cff00]/12 text-[#9cff00] ring-1 ring-[#9cff00]/24',
        softButtonClass: 'border border-[#9cff00]/28 bg-[#9cff00]/10 text-[#9cff00] ring-0 hover:bg-[#9cff00]/14',
        solidButtonClass: 'border border-[#9cff00]/44 bg-[#9cff00] text-[#050700] hover:bg-[#b7ff1a] active:bg-[#86d900]',
        solidRingClass: 'ring-1 ring-[#9cff00]/28',
        gradientButtonClass: 'border border-[#9cff00]/44 bg-[#9cff00] text-[#050700] shadow-none ring-0 hover:bg-[#b7ff1a]',
        textClass: 'text-[#9cff00]',
        inputFocusClass: 'focus:border-[#9cff00]/60 focus:ring-2 focus:ring-[#9cff00]/15',
        selectionClass: 'border-[#9cff00]/45 bg-[#9cff00]/11 ring-1 ring-[#9cff00]/18 text-[#9cff00]',
        dotClass: 'bg-[#9cff00]',
        successNoticeClass: 'bg-[#9cff00] text-[#050700]',
        navActiveClass: 'bg-[#9cff00] text-[#050700]',
        actionPillButtonClass: 'inline-flex items-center gap-1 rounded-full border border-[#9cff00]/44 bg-[#9cff00] px-2.5 py-1 text-[10px] font-bold leading-none text-[#050700] shadow-none transition hover:bg-[#b7ff1a] active:bg-[#86d900] disabled:cursor-not-allowed disabled:opacity-50',
        focusRingClass: 'focus-visible:ring-2 focus-visible:ring-emerald-400/40 focus-visible:ring-offset-0',
        ...SHARED_TOKENS,
    },
};

export function getBrandTheme(value) {
    return BRAND_THEMES[normalizeAccentTheme(value)];
}
