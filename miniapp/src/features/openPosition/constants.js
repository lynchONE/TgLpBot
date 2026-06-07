export const STORAGE_OPEN_POSITION_WALLET_ID = 'tglp_open_position_wallet_id';
export const STORAGE_OPEN_POSITION_HIDE_WALLET_BALANCES = 'tglp_open_position_hide_wallet_balances';

export const OPEN_POSITION_RANGE_OPTIONS = [
    { key: 'percentage', label: '百分比区间' },
    { key: 'grid', label: 'Tick/价格' },
];

export const OPEN_POSITION_GRID_RADIUS = 8;
export const OPEN_POSITION_DEFAULT_GRID_OFFSET = 3;

export const OPEN_POSITION_MANUAL_OPTIONS = [
    { key: 'percentage', label: '百分比' },
    { key: 'grid', label: 'Tick网格' },
    { key: 'tick', label: '直接 Tick' },
    { key: 'price', label: '价格区间' },
];

export const POSITION_SM_RANGE_STALE_MS = 60_000;
export const POSITION_SM_RANGE_BATCH_SIZE = 8;
