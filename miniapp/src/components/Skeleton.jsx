
// 骨架屏基础样式
const baseClass = 'animate-shimmer rounded';

// 骨架线条
export const SkeletonLine = ({ className = '', width = 'full', height = '4' }) => (
    <div
        className={`${baseClass} h-${height} ${width === 'full' ? 'w-full' : `w-${width}`} ${className}`}
        style={typeof width === 'number' ? { width: `${width}px` } : {}}
    />
);

// 骨架圆形
export const SkeletonCircle = ({ size = 10, className = '' }) => (
    <div
        className={`${baseClass} rounded-full ${className}`}
        style={{ width: `${size * 4}px`, height: `${size * 4}px` }}
    />
);

// 热门池子卡片骨架
export const SkeletonHotPoolCard = () => (
    <div className="rounded-2xl border border-zinc-200 bg-white/40 backdrop-blur-md p-4 shadow-sm dark:border-white/10 dark:bg-white/5">
        <div className="flex items-start justify-between gap-3">
            <div className="min-w-0 flex-1">
                <div className="flex items-center gap-2">
                    <div className={`${baseClass} h-4 w-20`} />
                    <div className={`${baseClass} h-5 w-12 rounded-lg`} />
                    <div className={`${baseClass} h-7 w-7 rounded-xl`} />
                    <div className={`${baseClass} h-7 w-7 rounded-xl`} />
                </div>
                <div className="mt-2 flex flex-wrap items-center gap-x-4 gap-y-1">
                    <div className={`${baseClass} h-3 w-24`} />
                    <div className={`${baseClass} h-3 w-20`} />
                    <div className={`${baseClass} h-3 w-16`} />
                </div>
            </div>
            <div className="text-right shrink-0 min-w-[110px]">
                <div className={`${baseClass} h-6 w-20 ml-auto`} />
                <div className={`${baseClass} h-3 w-16 mt-1 ml-auto`} />
            </div>
        </div>
        <div className="mt-3 flex items-center justify-between gap-2">
            <div className={`${baseClass} h-6 w-20 rounded-lg`} />
            <div className={`${baseClass} h-7 w-20 rounded-lg`} />
        </div>
    </div>
);

// 仓位卡片骨架
export const SkeletonPositionCard = () => (
    <div className="rounded-2xl border border-zinc-200 bg-white/40 backdrop-blur-md p-4 shadow-sm dark:border-white/10 dark:bg-white/5">
        <div className="flex items-start justify-between gap-3">
            <div>
                <div className={`${baseClass} h-6 w-32`} />
                <div className="mt-2 flex flex-wrap items-center gap-2">
                    <div className={`${baseClass} h-5 w-16 rounded-full`} />
                    <div className={`${baseClass} h-5 w-20 rounded-full`} />
                </div>
            </div>
            <div className="text-right">
                <div className={`${baseClass} h-3 w-24`} />
                <div className={`${baseClass} h-7 w-20 mt-1`} />
            </div>
        </div>
        <div className="mt-4 rounded-xl border border-zinc-200 bg-zinc-50 p-3 dark:border-white/10 dark:bg-[#0f1116]">
            <div className={`${baseClass} h-4 w-20`} />
            <div className="mt-3 grid grid-cols-4 gap-2">
                {[1, 2, 3, 4].map(i => (
                    <div key={i} className={`${baseClass} h-3 w-full`} />
                ))}
            </div>
            <div className="mt-3 grid grid-cols-4 gap-2">
                {[1, 2, 3, 4].map(i => (
                    <div key={i}>
                        <div className={`${baseClass} h-5 w-full`} />
                        <div className={`${baseClass} h-3 w-3/4 mt-1`} />
                    </div>
                ))}
            </div>
        </div>
        <div className="mt-3 grid grid-cols-4 gap-2">
            {[1, 2, 3, 4].map(i => (
                <div key={i} className={`${baseClass} h-9 rounded-xl`} />
            ))}
        </div>
        <div className="mt-3 rounded-xl border border-zinc-200 bg-zinc-50 p-3 dark:border-white/10 dark:bg-[#0f1116]">
            <div className={`${baseClass} h-6 w-full rounded-full`} />
        </div>
    </div>
);

// 通用骨架屏列表
export const SkeletonList = ({ count = 3, Card = SkeletonHotPoolCard }) => (
    <div className="space-y-3">
        {Array.from({ length: count }).map((_, i) => (
            <Card key={i} />
        ))}
    </div>
);

export default {
    SkeletonLine,
    SkeletonCircle,
    SkeletonHotPoolCard,
    SkeletonPositionCard,
    SkeletonList,
};
