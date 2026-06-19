import { Check, Power, X } from 'lucide-react';

export default function ToggleSwitch({
    checked = false,
    onChange,
    disabled = false,
    label = '',
    description = '',
    size = 'md',
}) {
    const trackWidth = size === 'sm' ? 'w-[68px]' : 'w-[78px]';
    const trackHeight = size === 'sm' ? 'h-8' : 'h-9';
    const thumbSize = size === 'sm' ? 'h-6 w-6' : 'h-7 w-7';
    const thumbTranslate = checked
        ? (size === 'sm' ? 'translate-x-[34px]' : 'translate-x-[39px]')
        : 'translate-x-[2px]';
    const stateText = checked ? '已开启' : '已关闭';

    return (
        <div
            className={`rounded-2xl border p-3.5 transition-all ${
                disabled ? 'opacity-55' : ''
            } ${
                checked
                    ? 'border-emerald-500/20 bg-emerald-500/[0.06] dark:border-emerald-400/20 dark:bg-emerald-500/[0.08]'
                    : 'border-zinc-200/80 bg-white/70 dark:border-white/[0.08] dark:bg-white/[0.03]'
            }`}
        >
            <div className="flex items-center gap-3">
                {(label || description) ? (
                    <div className="min-w-0 flex-1">
                        {label ? (
                            <div className="text-sm font-semibold text-zinc-900 dark:text-white/90">
                                {label}
                            </div>
                        ) : null}
                        {description ? (
                            <div className="mt-1 text-xs leading-5 text-zinc-500 dark:text-white/45">
                                {description}
                            </div>
                        ) : null}
                    </div>
                ) : (
                    <div className="flex-1" />
                )}

                <div className="flex shrink-0 items-center gap-2">
                    <span
                        className={`hidden rounded-full px-2.5 py-1 text-[11px] font-semibold sm:inline-flex ${
                            checked
                                ? 'bg-emerald-500/12 text-emerald-700 dark:bg-emerald-500/15 dark:text-emerald-200'
                                : 'bg-zinc-100 text-zinc-500 dark:bg-white/[0.06] dark:text-white/40'
                        }`}
                    >
                        {stateText}
                    </span>
                    <button
                        type="button"
                        role="switch"
                        aria-checked={checked}
                        aria-label={label || stateText}
                        disabled={disabled}
                        onClick={() => !disabled && onChange?.(!checked)}
                        className={`${trackWidth} ${trackHeight} relative inline-flex items-center rounded-full border transition-all duration-200 ease-out focus:outline-none ${
                            checked
                                ? 'border-emerald-400/30 bg-[linear-gradient(180deg,rgba(16,185,129,0.95),rgba(5,150,105,0.9))] shadow-[0_8px_18px_rgba(16,185,129,0.25)]'
                                : 'border-zinc-200 bg-zinc-100 dark:border-white/10 dark:bg-white/[0.08]'
                        }`}
                    >
                        <span
                            className={`absolute left-2 inline-flex items-center gap-1 text-[10px] font-semibold transition-opacity ${
                                checked
                                    ? 'opacity-0'
                                    : 'text-zinc-500 dark:text-white/35'
                            }`}
                        >
                            <X className="h-3 w-3" />
                            关闭
                        </span>
                        <span
                            className={`absolute right-2 inline-flex items-center gap-1 text-[10px] font-semibold transition-opacity ${
                                checked
                                    ? 'text-white/90 opacity-100'
                                    : 'opacity-0'
                            }`}
                        >
                            <Check className="h-3 w-3" />
                            开启
                        </span>
                        <span
                            className={`absolute top-1 ${thumbSize} ${thumbTranslate} inline-flex items-center justify-center rounded-full bg-white shadow-[0_6px_16px_rgba(0,0,0,0.16)] transition-transform duration-200 ease-out`}
                        >
                            <Power className={`h-3.5 w-3.5 ${checked ? 'text-emerald-600' : 'text-zinc-400'}`} />
                        </span>
                    </button>
                </div>
            </div>
        </div>
    );
}
