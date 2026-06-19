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
    return `w-full min-h-[44px] rounded-lg border border-white/[0.16] bg-[#131820] px-3 py-2.5 text-sm text-white outline-none placeholder:text-white/45 focus:ring-2 ${getBrandFocusRingClass(brand)}`;
}

export function getFilterButtonClass(active, brand) {
    return active
        ? brand.softButtonClass
        : 'border border-white/[0.18] bg-[#252b34] text-white hover:border-white/[0.28] hover:bg-[#303844] hover:text-white';
}

export function getIconButtonClass(danger = false) {
    return [
        'inline-flex min-h-[44px] min-w-[44px] items-center justify-center rounded-lg border transition disabled:cursor-not-allowed disabled:bg-[#232830] disabled:text-white/45',
        danger
            ? 'border-[#ff4fb3]/28 bg-[#ff4fb3]/10 text-[#ffb8df] hover:bg-[#ff4fb3]/15'
            : 'border-white/[0.18] bg-[#252b34] text-white hover:bg-[#303844] hover:text-white',
    ].join(' ');
}
