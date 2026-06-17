export function getBrandLinkClass(brand) {
    return brand?.key === 'emerald'
        ? 'text-[#9cff00] hover:text-[#cfff57]'
        : 'text-[#dfff8b] hover:text-[#efffb8]';
}

export function getBrandActionChipClass(brand) {
    return brand?.key === 'emerald'
        ? 'rounded-md border border-[#9cff00]/28 bg-[#9cff00]/10 px-2.5 py-1 font-semibold text-[#9cff00] hover:bg-[#9cff00]/14'
        : 'rounded-md border border-[#b7ff1a]/28 bg-[#b7ff1a]/10 px-2.5 py-1 font-semibold text-[#d9ff63] hover:bg-[#b7ff1a]/14';
}

export function getBrandFocusRingClass(brand) {
    return brand?.key === 'emerald'
        ? 'focus:ring-[#9cff00]'
        : 'focus:ring-[#b7ff1a]';
}

export function getInputClass(brand) {
    return `w-full rounded-lg border border-white/[0.08] bg-[#080808] px-3 py-2.5 text-sm text-white outline-none placeholder:text-white/25 focus:ring-1 ${getBrandFocusRingClass(brand)}`;
}

export function getFilterButtonClass(active, brand) {
    return active
        ? brand.softButtonClass
        : 'border border-white/[0.08] bg-[#111] text-white/50 hover:border-white/[0.18] hover:bg-[#191919] hover:text-white';
}

export function getIconButtonClass(danger = false) {
    return [
        'inline-flex h-9 w-9 items-center justify-center rounded-lg border transition disabled:cursor-not-allowed disabled:opacity-50',
        danger
            ? 'border-[#ff4fb3]/28 bg-[#ff4fb3]/10 text-[#ffb8df] hover:bg-[#ff4fb3]/15'
            : 'border-white/[0.08] bg-[#111] text-white/65 hover:bg-[#191919] hover:text-white',
    ].join(' ');
}
