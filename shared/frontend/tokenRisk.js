export function normalizeHexAddress(value) {
    const raw = String(value || '').trim();
    if (!raw) return '';
    const body = raw.startsWith('0x') || raw.startsWith('0X') ? raw.slice(2) : raw;
    if (!/^[a-fA-F0-9]{40}$/.test(body)) return '';
    return `0x${body.toLowerCase()}`;
}

export function shortAddress(value, left = 6, right = 4) {
    const raw = String(value || '').trim();
    if (!raw) return '--';
    if (raw.length <= left + right + 3) return raw;
    return `${raw.slice(0, left)}...${raw.slice(-right)}`;
}

const TOKEN_RISK_LEVEL_LABELS = ['未定义', '低', '中', '中高', '高', '高(人工)'];

function tokenRiskLevelToChinese(value) {
    const raw = String(value || '').trim();
    switch (raw.toLowerCase()) {
        case 'undefined':
            return '未定义';
        case 'low':
            return '低';
        case 'medium':
            return '中';
        case 'medium-high':
            return '中高';
        case 'high':
            return '高';
        case 'high(manual)':
            return '高(人工)';
        default:
            return raw;
    }
}

function tokenRiskWarningToChinese(value) {
    const raw = String(value || '').trim();
    const lower = raw.toLowerCase();
    if (!raw) return '';
    if (lower.includes('okx marked honeypot')) return 'OKX 标记为貔貅盘';
    if (lower.includes('okx marked low liquidity')) return 'OKX 标记为低流动性';
    if (lower.startsWith('okx risk level:')) {
        return `OKX 风险等级: ${tokenRiskLevelToChinese(raw.split(':').slice(1).join(':'))}`;
    }
    if (lower.startsWith('okx risk lookup failed:')) {
        return `OKX 风控查询失败: ${raw.split(':').slice(1).join(':').trim()}`;
    }
    if (lower.includes('429') || lower.includes('too many')) return 'OKX 风控接口限流，已延后后台刷新';
    if (lower.includes('advanced-info returned empty data')) return 'OKX advanced-info 未返回风控数据';
    if (lower.includes('low liquidity')) return raw.replace(/low liquidity/ig, '低流动性');
    if (lower.includes('honeypot')) return raw.replace(/honeypot/ig, '貔貅盘');
    return raw;
}

export function normalizeTokenRisk(value) {
    if (!value || typeof value !== 'object' || Array.isArray(value)) return null;
    const level = Number(value.risk_control_level);
    const warnings = Array.isArray(value.warnings)
        ? value.warnings.map(tokenRiskWarningToChinese).filter(Boolean)
        : [];
    const tags = Array.isArray(value.token_tags)
        ? value.token_tags.map((item) => String(item || '').trim()).filter(Boolean)
        : [];
    return {
        ...value,
        risk_control_level: Number.isFinite(level) ? level : 0,
        risk_control_label: TOKEN_RISK_LEVEL_LABELS[Number.isFinite(level) ? level : 0] || tokenRiskLevelToChinese(value.risk_control_label) || '未知',
        risk_tone: String(value.risk_tone || '').trim() || 'unknown',
        token_symbol: String(value.token_symbol || '').trim(),
        token_address: normalizeHexAddress(value.token_address),
        has_honeypot: Boolean(value.has_honeypot),
        has_low_liquidity: Boolean(value.has_low_liquidity),
        warnings,
        token_tags: tags,
        error: tokenRiskWarningToChinese(value.error),
    };
}

export function tokenRiskToneClass(risk) {
    const normalized = normalizeTokenRisk(risk);
    if (!normalized) return 'unknown';
    if (normalized.has_honeypot) return 'critical';
    switch (normalized.risk_tone) {
        case 'critical':
        case 'high':
        case 'medium':
        case 'low':
        case 'neutral':
        case 'unknown':
            return normalized.risk_tone;
        default:
            return 'unknown';
    }
}

export function tokenRiskLabel(risk) {
    const normalized = normalizeTokenRisk(risk);
    if (!normalized) return '';
    if (normalized.has_honeypot) return '貔貅盘';
    if (normalized.has_low_liquidity) return '低流动性';
    if (normalized.error) return '风控未知';
    return `风险 ${normalized.risk_control_label}`;
}

export function tokenRiskSummary(risk) {
    const normalized = normalizeTokenRisk(risk);
    if (!normalized) return '';
    if (normalized.warnings.length > 0) return normalized.warnings.join('；');
    const symbol = normalized.token_symbol || shortAddress(normalized.token_address);
    return `${symbol ? `${symbol} ` : ''}OKX 风控等级: ${normalized.risk_control_label}`;
}
