import { AlertTriangle, Check, Eye, EyeOff } from 'lucide-react';

import { formatUsdCompact, tokenRiskLabel, tokenRiskSummary } from '../../../lib/format';
import { tokenRiskPanelClass } from '../tokenRiskClass';

const OPEN_POSITION_AMOUNT_PRESETS = [200, 500, 1000, 1500, 2000];

function formatAmountPresetValue(value) {
    const num = Number(value);
    if (!Number.isFinite(num) || num <= 0) return '';
    if (Number.isInteger(num)) return String(num);
    return num.toFixed(num >= 1 ? 2 : 4).replace(/0+$/, '').replace(/\.$/, '');
}

export function OpenPositionWalletSelector({
    multiWalletEnabled,
    walletsLoading,
    walletsData,
    walletsError,
    openPositionWalletOptions,
    openPositionWalletBalancesHidden,
    openPositionWalletId,
    brand,
    onToggleBalancesHidden,
    onSelectWallet,
}) {
    if (!multiWalletEnabled) return null;

    const selectedWalletId = String(openPositionWalletId || '').trim();

    return (
        <div className="op-funding-card op-wallet-panel rounded-2xl border border-zinc-200/60 bg-zinc-50/60 p-3 dark:border-white/10 dark:bg-white/5">
            <div className="flex items-center justify-between gap-2">
                <div className="text-xs font-semibold text-zinc-900 dark:text-white/80">钱包</div>
                <div className="text-[11px] text-zinc-500 dark:text-white/40">
                    {walletsLoading
                        ? '加载中...'
                        : [
                            String(walletsData?.chain || '').toUpperCase(),
                            walletsData?.native_symbol && walletsData?.stable_symbol
                                ? `${walletsData.native_symbol}/${walletsData.stable_symbol}`
                                : '',
                        ].filter(Boolean).join(' | ')}
                </div>
                <button
                    type="button"
                    onClick={onToggleBalancesHidden}
                    className="inline-flex h-8 w-8 shrink-0 items-center justify-center rounded-full border border-zinc-200 bg-white/80 text-zinc-600 transition hover:bg-zinc-100 dark:border-white/10 dark:bg-white/5 dark:text-white/65 dark:hover:bg-white/10"
                    title={openPositionWalletBalancesHidden ? '显示钱包余额' : '隐藏钱包余额'}
                    aria-label={openPositionWalletBalancesHidden ? '显示钱包余额' : '隐藏钱包余额'}
                >
                    {openPositionWalletBalancesHidden ? <Eye className="h-4 w-4" /> : <EyeOff className="h-4 w-4" />}
                </button>
            </div>

            {walletsError ? (
                <div className="mt-2 rounded-xl border border-red-500/30 bg-red-500/10 p-2 text-xs text-red-700 dark:text-red-200">
                    {walletsError}
                </div>
            ) : null}

            {!walletsLoading && !walletsError && openPositionWalletOptions.length === 0 ? (
                <div className="mt-2 text-xs text-zinc-500 dark:text-white/50">当前没有可用钱包。</div>
            ) : null}

            <div
                className="op-wallet-grid mt-2 grid gap-2"
                style={{ gridTemplateColumns: `repeat(${Math.min(Math.max(openPositionWalletOptions.length, 1), 3)}, minmax(0, 1fr))` }}
            >
                {openPositionWalletOptions.map((w) => {
                    const id = String(w?.id || '').trim();
                    const addr = String(w?.address || '').trim();
                    const name = String(w?.name || '').trim();
                    const shortAddr = addr.length > 12 ? `${addr.slice(0, 6)}..${addr.slice(-4)}` : addr;
                    const selected = id && (id === selectedWalletId || (!selectedWalletId && openPositionWalletOptions.length === 1));

                    return (
                        <button
                            key={id || addr}
                            type="button"
                            onClick={() => {
                                if (!id) return;
                                onSelectWallet(id);
                            }}
                            aria-pressed={selected}
                            className={`op-wallet-option flex min-h-[38px] w-full min-w-0 items-center rounded-xl border px-2.5 py-1.5 text-left transition ${selected
                                ? `${brand.selectionClass} shadow-sm`
                                : 'border-zinc-200 bg-white/80 text-zinc-700 hover:bg-white dark:border-white/10 dark:bg-white/5 dark:text-white/70 dark:hover:bg-white/10'
                                }`}
                        >
                            <div className="flex min-w-0 flex-1 items-center gap-1">
                                <span className="truncate text-[11px] font-semibold leading-tight">{name || shortAddr || `钱包 ${id}`}</span>
                                {w?.is_default ? (
                                    <span className="shrink-0 rounded bg-zinc-500/10 px-1 py-px text-[9px] font-bold text-zinc-500 dark:text-white/50">默认</span>
                                ) : null}
                            </div>
                            <span className="shrink-0 pl-1 text-[10px] font-semibold tabular-nums text-zinc-900/75 dark:text-white/70">
                                {openPositionWalletBalancesHidden ? '****' : `$${String(w?.stable_balance ?? '--')}`}
                            </span>
                            {selected ? (
                                <span className="op-wallet-selected-mark" aria-hidden="true">
                                    <Check className="h-2.5 w-2.5" strokeWidth={3} />
                                </span>
                            ) : null}
                        </button>
                    );
                })}
            </div>
        </div>
    );
}

export function OpenPositionPrivateZapHint({ show }) {
    if (!show) return null;

    return (
        <div className="rounded-xl border border-emerald-500/25 bg-gradient-to-br from-emerald-500/12 to-transparent p-3 dark:border-emerald-400/20 dark:from-emerald-400/10">
            <div className="flex items-start gap-3">
                <div className="mt-0.5 flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-emerald-500/15 text-emerald-600 dark:text-emerald-300">
                    <Check className="h-3 w-3" strokeWidth={3} />
                </div>
                <div className="min-w-0">
                    <div className="text-xs font-semibold text-zinc-900 dark:text-white/85">智能建议金额</div>
                    <div className="mt-1 text-[11px] leading-5 text-zinc-600 dark:text-white/60">
                        系统会结合池子深度、钱包余额和当前模式给出建议金额，你也可以直接手动输入。</div>
                </div>
            </div>
        </div>
    );
}

export function OpenPositionTokenRiskPanel({ tokenRisk, tokenRiskTone, tokenRiskSymbol }) {
    if (!tokenRisk) return null;

    const riskLabel = tokenRiskLabel(tokenRisk);
    const riskTitle = riskLabel.startsWith('风险')
        ? `OKX 风险${riskLabel.replace(/^风险\s*/, '')}`
        : `OKX ${riskLabel}`;
    const riskSummary = tokenRiskSummary(tokenRisk);

    return (
        <div
            className={`op-risk-banner flex min-h-8 items-center gap-1.5 rounded-xl border px-2.5 py-1.5 text-[11px] leading-none ${tokenRiskPanelClass(tokenRiskTone)}`}
            title={riskSummary}
        >
            <AlertTriangle className="h-3.5 w-3.5 shrink-0" strokeWidth={2.5} />
            <span className="op-risk-main shrink-0 font-bold">{riskTitle}</span>
            <span className="op-risk-token min-w-0 flex-1 truncate opacity-70">{tokenRiskSymbol}</span>
        </div>
    );
}

export function OpenPositionAmountSlippagePanel({
    amount,
    maxAmount,
    slippage,
    slippagePlaceholder,
    globalSlippageHint,
    needsHighSlippageConfirm,
    taskSlippage,
    brand,
    onAmountChange,
    onSlippageChange,
}) {
    const maxAmountValue = Number(maxAmount);
    const hasMaxAmount = Number.isFinite(maxAmountValue) && maxAmountValue > 0;
    const amountValue = Number(amount);

    return (
        <div className="op-funding-card op-amount-panel rounded-2xl border border-zinc-200/60 bg-zinc-50/60 p-3 dark:border-white/10 dark:bg-white/5">
            {/* 金额：大字主输入 */}
            <div className="flex items-baseline justify-between gap-2">
                <span className="text-xs font-semibold text-zinc-500 dark:text-white/50">开仓金额</span>
                <span className="text-[11px] font-medium text-zinc-400 dark:text-white/35">USDT</span>
            </div>
            <input
                value={amount}
                onChange={(e) => onAmountChange(e.target.value)}
                inputMode="decimal"
                placeholder="0.00"
                className="mt-1 w-full border-0 bg-transparent p-0 text-[26px] font-semibold tracking-tight text-zinc-900 outline-none placeholder:text-zinc-300 dark:text-white dark:placeholder:text-white/20"
            />
            <div className="mt-2 grid grid-cols-3 gap-1.5" aria-label="快捷开仓金额">
                {OPEN_POSITION_AMOUNT_PRESETS.map((value) => {
                    const active = Number.isFinite(amountValue) && amountValue === value;
                    return (
                        <button
                            key={value}
                            type="button"
                            onClick={() => onAmountChange(String(value))}
                            className={`h-8 rounded-lg border px-2 text-[11px] font-extrabold transition active:scale-[0.99] ${active
                                ? `${brand.selectionClass} text-zinc-900 dark:text-white`
                                : 'border-zinc-200/70 bg-white/70 text-zinc-600 hover:bg-white dark:border-white/10 dark:bg-white/5 dark:text-white/60 dark:hover:bg-white/10'
                                }`}
                        >
                            {value}
                        </button>
                    );
                })}
                <button
                    type="button"
                    onClick={() => {
                        const next = formatAmountPresetValue(maxAmountValue);
                        if (next) onAmountChange(next);
                    }}
                    disabled={!hasMaxAmount}
                    className={`h-8 rounded-lg border px-2 text-[11px] font-extrabold transition active:scale-[0.99] ${hasMaxAmount && Number.isFinite(amountValue) && Math.abs(amountValue - maxAmountValue) < 0.000001
                        ? `${brand.selectionClass} text-zinc-900 dark:text-white`
                        : 'border-sky-200/70 bg-sky-50/70 text-sky-700 hover:bg-sky-50 disabled:cursor-not-allowed disabled:opacity-45 dark:border-sky-400/15 dark:bg-sky-400/10 dark:text-sky-200 dark:hover:bg-sky-400/15'
                        }`}
                    title={hasMaxAmount ? `使用当前钱包余额 ${formatAmountPresetValue(maxAmountValue)} USDT` : '当前钱包余额不可用'}
                >
                    MAX
                </button>
            </div>
            <div className="op-slippage-field mt-3 border-t border-zinc-200/60 pt-3 dark:border-white/10">
                <div className="flex items-baseline justify-between gap-2">
                    <span className="text-xs font-semibold text-zinc-500 dark:text-white/50">滑点</span>
                    <span className="op-slippage-hint text-[11px] leading-4 text-zinc-400 dark:text-white/40">{globalSlippageHint}</span>
                </div>
                <div className="op-slippage-input-wrap relative mt-1">
                    <input
                        value={slippage}
                        onChange={(e) => onSlippageChange(e.target.value)}
                        inputMode="decimal"
                        className={`w-full border-0 bg-transparent p-0 pr-8 text-[26px] font-semibold tracking-tight text-zinc-900 outline-none placeholder:text-zinc-300 ${brand.inputFocusClass} dark:text-white dark:placeholder:text-white/20`}
                        placeholder={slippagePlaceholder}
                    />
                    <span className="pointer-events-none absolute right-0 top-1/2 -translate-y-1/2 text-sm font-semibold text-zinc-400 dark:text-white/40">%</span>
                </div>
            </div>
            {needsHighSlippageConfirm ? (
                <div className="mt-2 rounded-xl border border-amber-500/25 bg-amber-500/10 px-2.5 py-1.5 text-[10px] leading-4 text-amber-700 dark:border-amber-400/25 dark:bg-amber-400/10 dark:text-amber-200">
                    滑点 {taskSlippage.value}% 较高，可能成交价较差。
                </div>
            ) : null}
        </div>
    );
}

export function OpenPositionRecommendedAmounts({
    positions,
    onApplyAmount,
}) {
    if (positions.length === 0) return null;

    return (
        <div className="mt-2 flex flex-wrap gap-1.5 text-zinc-900 dark:text-white/80">
            {positions.map((item, index) => {
                const tone = item?.mode === 'conservative'
                    ? { border: 'border-emerald-500/30', bg: 'bg-emerald-500/10', text: 'text-emerald-700 dark:text-emerald-400', icon: '稳' }
                    : item?.mode === 'neutral'
                        ? { border: 'border-amber-500/30', bg: 'bg-amber-500/10', text: 'text-amber-700 dark:text-amber-400', icon: '均' }
                        : { border: 'border-red-500/30', bg: 'bg-red-500/10', text: 'text-red-700 dark:text-red-400', icon: '进' };
                return (
                    <button
                        key={`${item?.mode || 'mode'}-${index}`}
                        type="button"
                        onClick={() => onApplyAmount(String(item?.liquidity_to_add || ''))}
                        className={`flex items-center gap-1 rounded-full border px-2 py-1 text-left text-[10px] font-bold ${tone.border} ${tone.bg} ${tone.text} transition-all duration-150 hover:brightness-110 active:scale-[0.99]`}
                    >
                        <span className="grayscale-[0.2] overflow-hidden">{tone.icon}</span>
                        <span className="shrink-0">{formatUsdCompact(item?.liquidity_to_add)}</span>
                    </button>
                );
            })}
        </div>
    );
}

export function OpenPositionFundingStep({
    active,
    multiWalletEnabled,
    walletsLoading,
    walletsData,
    walletsError,
    walletOptions,
    walletBalancesHidden,
    walletId,
    privateZapHintVisible,
    tokenRisk,
    tokenRiskTone,
    tokenRiskSymbol,
    amount,
    maxAmount,
    slippage,
    slippagePlaceholder,
    globalSlippageHint,
    needsHighSlippageConfirm,
    taskSlippage,
    recommendedPositions,
    brand,
    onToggleWalletBalancesHidden,
    onSelectWallet,
    onAmountChange,
    onSlippageChange,
    onApplyRecommendedAmount,
}) {
    return (
        <div className={`space-y-3 ${active ? '' : 'hidden'}`}>
            <OpenPositionWalletSelector
                multiWalletEnabled={multiWalletEnabled}
                walletsLoading={walletsLoading}
                walletsData={walletsData}
                walletsError={walletsError}
                openPositionWalletOptions={walletOptions}
                openPositionWalletBalancesHidden={walletBalancesHidden}
                openPositionWalletId={walletId}
                brand={brand}
                onToggleBalancesHidden={onToggleWalletBalancesHidden}
                onSelectWallet={onSelectWallet}
            />

            <OpenPositionPrivateZapHint show={privateZapHintVisible} />

            <OpenPositionTokenRiskPanel
                tokenRisk={tokenRisk}
                tokenRiskTone={tokenRiskTone}
                tokenRiskSymbol={tokenRiskSymbol}
            />

            <OpenPositionAmountSlippagePanel
                amount={amount}
                maxAmount={maxAmount}
                slippage={slippage}
                slippagePlaceholder={slippagePlaceholder}
                globalSlippageHint={globalSlippageHint}
                needsHighSlippageConfirm={needsHighSlippageConfirm}
                taskSlippage={taskSlippage}
                brand={brand}
                onAmountChange={onAmountChange}
                onSlippageChange={onSlippageChange}
            />

            <OpenPositionRecommendedAmounts
                positions={recommendedPositions}
                onApplyAmount={onApplyRecommendedAmount}
            />
        </div>
    );
}
