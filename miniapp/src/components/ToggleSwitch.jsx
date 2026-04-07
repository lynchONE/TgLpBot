import React from 'react';

/**
 * 自定义 Toggle 开关组件
 * Props:
 *   checked    - 是否选中
 *   onChange   - (newValue: boolean) => void
 *   disabled   - 是否禁用
 *   label      - 标签文本
 *   description - 副标签文本
 *   size       - 'sm' | 'md'
 */
export default function ToggleSwitch({
    checked = false,
    onChange,
    disabled = false,
    label = '',
    description = '',
    size = 'md',
}) {
    const w = size === 'sm' ? 'w-9' : 'w-11';
    const h = size === 'sm' ? 'h-5' : 'h-6';
    const dotSize = size === 'sm' ? 'h-3.5 w-3.5' : 'h-4.5 w-4.5';
    const dotTranslate = checked
        ? (size === 'sm' ? 'translate-x-[18px]' : 'translate-x-[22px]')
        : 'translate-x-[2px]';

    return (
        <label className={`flex items-center gap-3 ${disabled ? 'cursor-not-allowed opacity-50' : 'cursor-pointer'}`}>
            {(label || description) && (
                <div className="flex-1 min-w-0">
                    {label && <div className="text-sm font-medium text-zinc-900 dark:text-white/90">{label}</div>}
                    {description && <div className="mt-0.5 text-xs text-zinc-500 dark:text-white/40">{description}</div>}
                </div>
            )}
            <button
                type="button"
                role="switch"
                aria-checked={checked}
                disabled={disabled}
                onClick={() => !disabled && onChange?.(!checked)}
                className={`${w} ${h} relative inline-flex shrink-0 items-center rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none
                    ${checked
                        ? 'bg-emerald-500 dark:bg-emerald-600'
                        : 'bg-zinc-200 dark:bg-white/15'
                    }`}
            >
                <span
                    className={`${dotSize} inline-block transform rounded-full bg-white shadow-sm ring-0 transition-transform duration-200 ease-in-out ${dotTranslate}`}
                />
            </button>
        </label>
    );
}

