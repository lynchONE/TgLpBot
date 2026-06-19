export function parseDCAPercentages(raw) {
    if (Array.isArray(raw)) return raw.map((value) => Number(value) || 0);
    if (typeof raw === 'string' && raw.trim()) {
        const parsed = JSON.parse(raw);
        if (Array.isArray(parsed)) return parsed.map((value) => Number(value) || 0);
    }
    return [50, 50];
}

export function buildGlobalConfigDraft(cfg = {}) {
    return {
        rebalance_timeout: cfg.rebalance_timeout ?? 10,
        slippage_tolerance: cfg.slippage_tolerance ?? 0.5,
        auto_reinvest: cfg.auto_reinvest ?? false,
        residual_tolerance: cfg.residual_tolerance ?? 1.0,
        zap_loss_tolerance: cfg.zap_loss_tolerance ?? 0.5,
        extra_notifications_enabled: cfg.extra_notifications_enabled ?? true,
        filter_chinese_tokens: cfg.filter_chinese_tokens ?? false,
        multi_chain_enabled: cfg.multi_chain_enabled ?? true,
        default_chain: cfg.default_chain || 'bsc',
        multi_wallet_enabled: cfg.multi_wallet_enabled ?? false,
        bark_enabled: cfg.bark_enabled ?? false,
        bark_server: cfg.bark_server || '',
        bark_group: cfg.bark_group || '',
        dca_enabled: cfg.dca_enabled ?? false,
        dca_percentages: parseDCAPercentages(cfg.dca_percentages_json ?? cfg.dca_percentages),
        dca_interval_seconds: cfg.dca_interval_seconds ?? 30,
        dca_min_split_amount_usdt: cfg.dca_min_split_amount_usdt ?? 0,
    };
}
