# Change: 增加热门池子与聪明钱池子筛选阈值

## Why
- 热门池子列表当前支持最低费用、最低费率等包含式筛选，但无法排除费率过高的池子，用户仍需要手动跳过高风险或不符合偏好的池子。
- 聪明钱池子视图当前更偏展示列表，缺少池子级筛选入口，用户无法快速按聪明钱当前仓位金额和池子费率缩小观察范围。

## What Changes
- 热门池子筛选增加“排除费率高于 X% 的池子”选项。
- 聪明钱池子视图增加筛选按钮和弹层，支持：
  - 按池子当前聪明钱仓位金额设置最低值。
  - 按池子费率设置最高值。
- 筛选默认关闭或为空时保持现有列表行为不变；应用后仅影响当前视图展示，不改变开仓逻辑。

## Impact
- Affected specs:
  - `web-workbench`
  - `miniapp-smart-money`
- Affected code:
  - `webapp/src/App.jsx`
  - `webapp/src/styles.css`
  - 可能涉及 `miniapp/src/components/SmartMoneyPage.jsx`
- Risks / tradeoffs:
  - 前端本地筛选只作用于当前已加载数据；若后续需要跨分页/服务端过滤，再扩展 API 参数。
  - 费率字段在不同数据源可能存在缺失，缺失时应按“不满足筛选”处理，避免误展示。
