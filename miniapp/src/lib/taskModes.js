export const TASK_MODE_OPTIONS = [
    {
        value: 'rebalance_all',
        label: '双向再平衡',
        shortLabel: '双向再平衡',
        description: '涨破和跌破区间都自动再平衡',
    },
    {
        value: 'exit_all',
        label: '双向撤出',
        shortLabel: '双向撤出',
        description: '涨破和跌破区间都自动撤出并结束任务',
    },
    {
        value: 'rebalance_up_exit_down',
        label: '上破再平衡',
        shortLabel: '上破再平衡',
        description: '涨破区间自动再平衡，跌破区间自动撤出',
    },
    {
        value: 'pause',
        label: '暂停任务',
        shortLabel: '暂停',
        description: '完全暂停自动处理，只能手动操作',
    },
];

export function normalizeTaskMode(taskMode, paused = false) {
    if (paused || String(taskMode || '').trim() === 'pause') {
        return 'pause';
    }
    switch (String(taskMode || '').trim()) {
        case 'rebalance_all':
        case 'rebalance_up_exit_down':
        case 'exit_all':
            return String(taskMode).trim();
        default:
            return 'exit_all';
    }
}

export function getTaskModeMeta(taskMode, paused = false) {
    const normalized = normalizeTaskMode(taskMode, paused);
    return TASK_MODE_OPTIONS.find((item) => item.value === normalized) || TASK_MODE_OPTIONS[1];
}

export function getOutOfRangeActionSummary(taskMode, paused = false) {
    const normalized = normalizeTaskMode(taskMode, paused);
    switch (normalized) {
        case 'rebalance_all':
            return { above: '自动再平衡', below: '自动再平衡' };
        case 'rebalance_up_exit_down':
            return { above: '自动再平衡', below: '自动撤仓终止' };
        case 'pause':
            return { above: '仅手动处理', below: '仅手动处理' };
        default:
            return { above: '自动撤仓终止', below: '自动撤仓终止' };
    }
}
