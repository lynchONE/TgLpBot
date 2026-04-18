# 开仓界面优化

## 修改时间
2026-04-18

## 修改内容

### 1. 移除"参考建议："标签文本
- **miniapp** (`miniapp/src/App.jsx`): 删除了第3683行的 `<span>参考建议:</span>`
- **webapp** (`webapp/src/components/OpenPositionModal.jsx`): 删除了第586行的 `<span>参考建议:</span>`
- **原因**: "参考建议："文字占据了水平空间，遮挡了后面的保守/中性/激进 pill 标签

### 2. 钱包列表优化为紧凑 Chip 布局
- **之前**: 每个钱包占满一整行，显示名称、完整地址、原生代币余额、稳定币余额，纵向堆叠
- **之后**: 钱包以小型 pill/chip 形式横向排列（flex-wrap），每个 chip 只显示：
  - 钱包名称（加粗）
  - 默认标签（如适用）
  - 稳定币余额（如 `$951.46`）

#### Miniapp 改动
- `miniapp/src/App.jsx`: 将钱包列表容器从 `max-h-56 overflow-y-auto space-y-2`（纵向滚动列表）改为 `flex flex-wrap gap-1.5`（横向 wrap 布局）
- 每个钱包按钮从全宽卡片 (`w-full rounded-xl px-3 py-2`) 改为紧凑 chip (`inline-flex rounded-full px-2.5 py-1`)

#### Webapp 改动
- `webapp/src/components/OpenPositionModal.jsx`: 移除了 `wallet-chip-addr` 地址行，余额简化为只显示稳定币
- `webapp/src/styles.css`: 
  - `.wallet-picker-list` 从 `flex-direction: column` 改为 `flex-wrap: wrap`
  - `.wallet-chip` 改为 `inline-flex`、圆角 pill 样式（`border-radius: 20px`）、更紧凑的 padding
  - 移除了 `.wallet-chip-addr` 样式（不再需要）
