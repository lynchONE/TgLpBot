export function getBrandLinkClass(brand) {
    return brand?.key === 'emerald'
        ? 'text-emerald-300 hover:text-emerald-200'
        : 'text-[#dfff8b] hover:text-[#efffb8]';
}

export function getBrandActionChipClass(brand) {
    return brand?.key === 'emerald'
        ? 'rounded-full border border-emerald-400/25 bg-emerald-500/12 px-2.5 py-1 font-semibold text-emerald-200 hover:bg-emerald-500/18'
        : 'rounded-full border border-[#bcff2f]/25 bg-[#bcff2f]/14 px-2.5 py-1 font-semibold text-[#efffb8] hover:bg-[#bcff2f]/20';
}

export function getBrandFocusRingClass(brand) {
    return brand?.key === 'emerald'
        ? 'focus:ring-emerald-500'
        : 'focus:ring-[#bcff2f]';
}

export function getInputClass(brand) {
    return `w-full rounded-2xl border border-white/[0.04] bg-zinc-950/55 px-3 py-2.5 text-sm text-zinc-100 outline-none placeholder:text-zinc-500 focus:ring-1 ${getBrandFocusRingClass(brand)}`;
}

export function getFilterButtonClass(active, brand) {
    return active
        ? brand.softButtonClass
        : 'border border-white/[0.04] bg-zinc-900/55 text-zinc-400 hover:bg-zinc-800/70';
}

export function getIconButtonClass(danger = false) {
    return [
        'inline-flex h-9 w-9 items-center justify-center rounded-xl border transition disabled:cursor-not-allowed disabled:opacity-50',
        danger
            ? 'border-red-500/20 bg-red-500/10 text-red-300 hover:bg-red-500/15'
            : 'border-white/[0.05] bg-zinc-900/65 text-zinc-300 hover:bg-zinc-800/80',
    ].join(' ');
}
