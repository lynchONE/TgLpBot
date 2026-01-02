import React, { useMemo, useState } from 'react';
import { Segmented, Table, Tag, Typography, Tooltip } from 'antd';
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
                    title: 'Pool Name',
                    dataIndex: 'name',
                    key: 'name',
                    width: 320,
                    render: (text, record) => (
                        <div className="group">
                            <div className="flex items-center gap-2 mb-0.5">
                                <span className="font-semibold text-sm text-foreground group-hover:text-primary transition-colors">{text}</span>
                                {!!record?.dex_id && (
                                    <span className="text-[10px] font-bold px-1.5 py-0.5 rounded bg-muted text-muted-foreground uppercase tracking-wider">
                                        {formatDexLabel(record.dex_id)}
                                    </span>
                                )}
                                {!!record?.pool_fee_percentage && (
                                    <span className="text-[10px] font-bold px-1.5 py-0.5 rounded bg-blue-500/10 text-blue-500">
                                        {Number(record.pool_fee_percentage).toFixed(2)}%
                                    </span>
                                )}
                            </div>
                            <div className="flex items-center gap-2">
                                <Tooltip title={record.address}>
                                    <Typography.Text className="text-[11px] text-muted-foreground font-mono cursor-pointer hover:text-foreground transition-colors" copyable={{ text: record.address }}>
                                        {record.address?.slice(0, 6)}...{record.address?.slice(-4)}
                                    </Typography.Text>
                                </Tooltip>
                            </div>
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
                        <div className="flex flex-col items-end">
                            <span className="text-foreground font-medium font-mono text-xs">
                                ${val ? Number(val).toFixed(6) : '0.00'}
                            </span>
                        </div>
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
                        <span className="text-foreground font-mono text-xs">
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
                        <span className="text-foreground font-mono text-xs">
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
                    render: (_, record) => {
                        const val = getNum(record, keyFor('fee_apr'));
                        return (
                            <span className={`font-mono text-xs font-medium ${val > 50 ? 'text-green-500' : 'text-foreground'}`}>
                                {val.toFixed(2)}%
                            </span>
                        );
                    },
                },
                {
                    title: 'TVL',
                    dataIndex: 'reserve_usd',
                    key: 'reserve_usd',
                    align: 'right',
                    width: 130,
                    sorter: (a, b) => getNum(a, 'reserve_usd') - getNum(b, 'reserve_usd'),
                    render: (val) => (
                        <span className="text-muted-foreground font-mono text-xs">
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
                        const isPos = val >= 0;
                        return (
                            <span className={`text-[11px] font-bold px-2 py-0.5 rounded-full ${isPos ? 'bg-green-500/10 text-green-500' : 'bg-red-500/10 text-red-500'}`}>
                                {isPos ? '+' : ''}{val.toFixed(2)}%
                            </span>
                        );
                    },
                },
            ];
        },
        [interval, intervalLabel, usdCompactFormatter]
    );

    if (error) return (
        <div className="p-6 rounded-2xl bg-destructive/10 border border-destructive/20 text-destructive text-center">
            <p className="font-bold">Error loading pools</p>
            <p className="text-xs mt-1 opacity-80">{error.message}</p>
        </div>
    );

    return (
        <div className="overflow-x-auto">
            <div className="mb-6 flex flex-wrap items-center justify-between gap-3">
                <div className="text-xs font-medium text-muted-foreground px-1">
                    Performance Interval: <span className="text-foreground">{intervalLabel}</span>
                </div>
                <Segmented
                    size="small"
                    value={interval}
                    onChange={setInterval}
                    className="bg-muted p-0.5"
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
                size="middle"
                className="custom-table"
                scroll={{ x: true }}
                pagination={{ pageSize: 10, position: ['bottomCenter'], className: "!mb-0" }}
            />
        </div>
    );
};

export default PoolTable;
