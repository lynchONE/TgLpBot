## ADDED Requirements

### Requirement: 搜索池子入口
MiniApp 的「热门池子」页面 MUST 提供“搜索池子”入口，允许用户输入池子ID或代币名称/符号进行搜索。

搜索结果 MUST 最多展示 10 条，并允许用户从结果中选择一个池子进入现有“一键开仓”流程。

#### Scenario: 按池子ID搜索并开仓
- **WHEN** 用户在搜索框输入一个池子ID（V3 pool address 或 V4 poolId）
- **THEN** 页面展示该池子的搜索结果，并可点击进入“一键开仓”弹窗完成开仓

#### Scenario: 按代币名称搜索多池子
- **WHEN** 用户输入代币名称/符号（例如 `USDT`）且命中多个池子
- **THEN** 搜索结果按 TVL 倒序排序，最多展示 10 条

#### Scenario: 无结果提示
- **WHEN** 用户输入关键字但未命中任何池子
- **THEN** 页面提示“未找到相关池子”

