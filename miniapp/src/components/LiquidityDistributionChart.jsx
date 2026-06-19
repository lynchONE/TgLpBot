import SharedLiquidityDistributionChart, { DEFAULT_LIQUIDITY_CHART_THEME } from '../../../shared/frontend/LiquidityDistributionChart.jsx';

const MINIAPP_THEME = {
    ...DEFAULT_LIQUIDITY_CHART_THEME,
    barInside: 'linear-gradient(to top, rgba(34, 211, 138, 0.85), rgba(34, 211, 138, 0.35))',
    rangeBg: 'rgba(34, 211, 138, 0.1)',
    handleLower: '#22d38a',
    handleUpper: '#ff5e76',
    priceTagText: '#ecf2ff',
    emptyText: '#9aa8c4',
    tooltipText: '#ecf2ff',
    tooltipMuted: '#9aa8c4',
    tooltipValue: '#bcff2f',
};

export default function LiquidityDistributionChart(props) {
    return (
        <SharedLiquidityDistributionChart
            height={200}
            handleHitPx={28}
            theme={MINIAPP_THEME}
            {...props}
        />
    );
}
