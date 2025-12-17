import React, { useMemo, useState } from 'react';
import { Segmented, Table, Tag, Typography } from 'antd';
import { useQuery } from '@tanstack/react-query';

const fetchPools = async () => {
    const response = await fetch('http://localhost:8080/api/pools');
    if (!response.ok) {
        throw new Error('Network response was not ok');
    }
    return response.json();
};

const formatDexLabel = (dexId) => {
    if (!dexId) return 'DEX';
    const cleaned = String(dexId).replace(/[_-]+/g, ' ').trim();
    return cleaned.replace(/\b\w/g, (c) => c.toUpperCase());
};

const PoolTable = ({ refetchIntervalMs = 30000 }) => {
    const [interval, setInterval] = useState('h24');

    const { data, isLoading, error } = useQuery({
        queryKey: ['pools'],
        queryFn: fetchPools,
        refetchInterval: Math.max(5000, Number(refetchIntervalMs) || 30000),
    });

    const pools = data?.data || [];

    const usdCompactFormatter = useMemo(
        () =>
            new Intl.NumberFormat('en-US', {
                style: 'currency',
                currency: 'USD',
                notation: 'compact',
                maximumFractionDigits: 2,
            }),
        []
    );

    const intervalLabel = useMemo(() => {
        switch (interval) {
            case 'm5':
                return '5m';
            case 'h1':
                return '1h';
            case 'h6':
                return '6h';
            default:
                return '24h';
        }
    }, [interval]);

    const columns = useMemo(
        () => {
            const keyFor = (prefix) => `${prefix}_${interval}`;
            const getNum = (record, key) => Number(record?.[key] || 0);
            const formatUsdCompact = (value) => usdCompactFormatter.format(Number(value || 0));

            return [
                {
                    title: 'Pool',
                    dataIndex: 'name',
                    key: 'name',
                    width: 320,
                    render: (text, record) => (
                        <div>
                            <div className="flex items-center gap-2">
                                <div className="font-semibold text-sm text-gray-900 dark:text-gray-100">{text}</div>
                                {!!record?.dex_id && (
                                    <Tag className="border-0 text-[11px] leading-[18px]">
                                        {formatDexLabel(record.dex_id)}
                                    </Tag>
                                )}
                                {!!record?.pool_fee_percentage && (
                                    <Tag className="border-0 text-[11px] leading-[18px]" color="blue">
                                        {Number(record.pool_fee_percentage).toFixed(2)}%
                                    </Tag>
                                )}
                            </div>
                            <Typography.Text
                                className="text-[11px] text-gray-600 dark:text-gray-400 font-mono"
                            copyable={{ text: record.address }}
                            >
                                {record.address?.slice(0, 6)}...{record.address?.slice(-4)}
                            </Typography.Text>
                        </div>
                    ),
                },
            {
                title: 'Price',
                dataIndex: 'price_usd',
                key: 'price_usd',
                align: 'right',
                width: 120,
                render: (val) => (
                    <span className="text-gray-900 dark:text-gray-100 font-medium font-mono text-xs">
                        ${val ? Number(val).toFixed(6) : '0.00'}
                    </span>
                ),
            },
            {
                title: `${intervalLabel} Vol`,
                key: keyFor('volume'),
                align: 'right',
                width: 130,
                sorter: (a, b) => getNum(a, keyFor('volume')) - getNum(b, keyFor('volume')),
                defaultSortOrder: interval === 'h24' ? 'descend' : undefined,
                render: (_, record) => (
                    <span className="text-gray-900 dark:text-gray-100 font-mono text-xs">
                        {formatUsdCompact(getNum(record, keyFor('volume')))}
                    </span>
                ),
            },
            {
                title: `${intervalLabel} Fees`,
                key: keyFor('fee_usd'),
                align: 'right',
                width: 130,
                sorter: (a, b) => getNum(a, keyFor('fee_usd')) - getNum(b, keyFor('fee_usd')),
                render: (_, record) => (
                    <span className="text-gray-900 dark:text-gray-100 font-mono text-xs">
                        {formatUsdCompact(getNum(record, keyFor('fee_usd')))}
                    </span>
                ),
            },
            {
                title: `${intervalLabel} APR`,
                key: keyFor('fee_apr'),
                align: 'right',
                width: 120,
                sorter: (a, b) => getNum(a, keyFor('fee_apr')) - getNum(b, keyFor('fee_apr')),
                render: (_, record) => (
                    <span className="text-gray-900 dark:text-gray-100 font-mono text-xs">
                        {getNum(record, keyFor('fee_apr')).toFixed(2)}%
                    </span>
                ),
            },
            {
                title: 'TVL',
                dataIndex: 'reserve_usd',
                key: 'reserve_usd',
                align: 'right',
                width: 130,
                sorter: (a, b) => getNum(a, 'reserve_usd') - getNum(b, 'reserve_usd'),
                render: (val) => (
                    <span className="text-gray-900 dark:text-gray-100 font-mono text-xs">
                        {formatUsdCompact(val)}
                    </span>
                ),
            },
            {
                title: `${intervalLabel} Chg`,
                key: keyFor('price_change'),
                align: 'right',
                width: 110,
                sorter: (a, b) => getNum(a, keyFor('price_change')) - getNum(b, keyFor('price_change')),
                render: (_, record) => {
                    const val = getNum(record, keyFor('price_change'));
                    return (
                        <Tag
                            color={val >= 0 ? 'green' : 'red'}
                            className="text-[11px] font-bold border-0 px-2 py-0.5"
                        >
                            {val.toFixed(2)}%
                        </Tag>
                    );
                },
            },
            ];
        },
        [interval, intervalLabel, usdCompactFormatter]
    );

    if (error) return <div className="p-4 text-red-500 bg-red-50 dark:bg-red-900/10 rounded">Error loading pools: {error.message}</div>;

    return (
        <div className="overflow-x-auto">
            <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
                <div className="text-xs text-gray-600 dark:text-gray-400">Interval: {intervalLabel}</div>
                <Segmented
                    size="small"
                    value={interval}
                    onChange={setInterval}
                    options={[
                        { label: '5m', value: 'm5' },
                        { label: '1h', value: 'h1' },
                        { label: '6h', value: 'h6' },
                        { label: '24h', value: 'h24' },
                    ]}
                />
            </div>
            <Table
                columns={columns}
                dataSource={pools}
                rowKey="id"
                loading={isLoading}
                size="small"
                className="text-xs"
                scroll={{ x: true }}
                pagination={{ pageSize: 10, position: ['bottomCenter'] }}
                rowClassName={() =>
                    'bg-transparent hover:bg-slate-50/80 dark:hover:bg-white/5 transition-colors'
                }
            />
        </div>
    );
};

export default PoolTable;
