# 变更：AutoLP 跳过开仓时通知用户

## Why
用户看到 Top1 候选池推送但没有开仓，缺少“为什么没开仓”的提示。

## What Changes
- 当池子符合候选条件但因任意原因未开仓时，向用户推送通知。
- 增加按“用户+池子”的频控，5 分钟内只推送一次。
- 通知中包含简洁的原因说明。

## Impact
- 影响规格：auto-lp
- 影响代码：backend/service/auto_lp/auto_lp_service.go，backend/service/auto_lp（新增辅助逻辑）
