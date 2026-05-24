import React, { useEffect, useState } from 'react';

// 通用底部抽屉组件 (Bottom Sheet)
export default function BottomSheet({
    open,
    onClose,
    title,
    children,
    maxHeightClass = 'max-h-[85vh]',
    className = '',
    contentClassName = 'px-5 pb-6 sm:pb-5',
    headerClassName = 'px-5 pt-3 sm:pt-5 pb-3',
    headerRight = null,
    footer = null,
    footerClassName = 'px-5 pt-3 pb-[calc(env(safe-area-inset-bottom)+0.75rem)]',
}) {
    const [isVisible, setIsVisible] = useState(false);
    const [isAnimating, setIsAnimating] = useState(false);

    useEffect(() => {
        if (open) {
            setIsVisible(true);
            requestAnimationFrame(() => {
                requestAnimationFrame(() => {
                    setIsAnimating(true);
                });
            });
            // 锁定 body 滚动
            document.body.style.overflow = 'hidden';
        } else {
            setIsAnimating(false);
            const timer = setTimeout(() => {
                setIsVisible(false);
                document.body.style.overflow = '';
            }, 300); // 对应 transition duration
            return () => clearTimeout(timer);
        }
        return () => {
            document.body.style.overflow = '';
        };
    }, [open]);

    if (!isVisible) return null;

    return (
        <div className="fixed inset-0 z-[60] flex items-end justify-center sm:items-center">
            {/* Backdrop */}
            <div
                className={`absolute inset-0 bg-black/60 backdrop-blur-sm transition-opacity duration-300 ease-in-out ${isAnimating ? 'opacity-100' : 'opacity-0'}`}
                onClick={onClose}
                aria-hidden="true"
            />

            {/* Sheet wrapper */}
            <div
                className={`relative w-full sm:max-w-md flex flex-col overflow-hidden bg-white/80 dark:bg-[#14171c]/90 backdrop-blur-xl border-t sm:border border-zinc-200/50 dark:border-white/10 rounded-t-3xl sm:rounded-3xl shadow-2xl transition-transform duration-300 ease-in-out ${maxHeightClass} ${className} ${isAnimating ? 'translate-y-0 sm:scale-100 opacity-100' : 'translate-y-full sm:translate-y-0 sm:scale-95 opacity-0'}`}
            >
                {/* 顶部抓手 (移动端暗示可下拉) */}
                <div
                    className="w-full flex justify-center pt-3 pb-2 sm:hidden touch-pan-y cursor-grab active:cursor-grabbing"
                    onClick={onClose}
                >
                    <div className="w-12 h-1.5 rounded-full bg-zinc-300 dark:bg-zinc-600" />
                </div>

                {/* Header */}
                <div className={`flex items-start justify-between shrink-0 ${headerClassName}`}>
                    <div className="text-lg font-bold text-zinc-900 dark:text-white/90 flex-1 min-w-0 pr-4">{title}</div>
                    <div className="flex items-center gap-2 shrink-0">
                        {headerRight}
                        <button
                            type="button"
                            onClick={onClose}
                            className="inline-flex h-8 w-8 items-center justify-center rounded-full bg-zinc-100 text-zinc-600 hover:bg-zinc-200 active:bg-zinc-300 dark:bg-white/10 dark:text-white/70 dark:hover:bg-white/20 transition-colors"
                            aria-label="关闭"
                        >
                            <svg className="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
                                <path d="M18 6L6 18M6 6l12 12" />
                            </svg>
                        </button>
                    </div>
                </div>

                {/* Content Area */}
                <div
                    className={`flex-1 min-h-0 overflow-y-auto overscroll-contain touch-pan-y ${contentClassName}`}
                    style={{ WebkitOverflowScrolling: 'touch' }}
                >
                    {children}
                </div>

                {footer ? (
                    <div className={`shrink-0 border-t border-zinc-200/70 bg-white/95 backdrop-blur-xl dark:border-white/10 dark:bg-[#14171c]/95 ${footerClassName}`}>
                        {footer}
                    </div>
                ) : null}
            </div>
        </div >
    );
}
