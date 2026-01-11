## ADDED Requirements

### Requirement: MiniApp 代币链接跳转 GMGN
MiniApp 实时仓位任务面板中，点击代币地址时 MUST 跳转到 GMGN 的代币详情页（BSC）。

#### Scenario: 点击代币地址打开 GMGN
- **WHEN** 用户在仓位卡片中点击某代币地址 `0x...`
- **THEN** MiniApp 打开 `https://gmgn.ai/bsc/token/<tokenAddress>`

