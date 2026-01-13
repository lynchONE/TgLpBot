# K 线默认时间周期优化

## 更新时间
2026-01-13 22:59

## 需求背景
用户希望在 miniapp 中打开 DexScreener 和 DEXTools 查看 K 线图时，默认显示 1 分钟（1m）的 K 线，而不是之前的默认时间周期。

## 实现内容

### 修改文件
- `miniapp/src/components/KlineModal.jsx`

### 具体修改

#### 1. DexScreener（V2/V3 池）
- **修改前**：URL 中没有设置时间周期参数，使用平台默认值
- **修改后**：添加 `interval=1` 参数，默认显示 1 分钟 K 线
- **URL 格式**：`https://dexscreener.com/{chain}/{address}?embed=1&theme={theme}&items=0&info=0&interval=1`

#### 2. DEXTools（V4 池）
- **修改前**：`chartResolution=30`（30 分钟）
- **修改后**：`chartResolution=1`（1 分钟）
- **URL 格式**：`https://www.dextools.io/widget-chart/en/{chain}/pe-light/{address}?theme={theme}&chartType=1&chartResolution=1&drawingToolbars=false`

## 技术说明

### URL 参数说明
- **DexScreener**：使用 `interval` 参数控制时间周期，`interval=1` 表示 1 分钟
- **DEXTools**：使用 `chartResolution` 参数控制时间周期，`chartResolution=1` 表示 1 分钟

### 代码注释
在代码中添加了中文注释说明参数含义，方便后续维护。

## 测试建议
1. 打开 miniapp，选择一个 V2/V3 池，点击查看 K 线，确认 DexScreener 显示的是 1 分钟 K 线
2. 选择一个 V4 池，点击查看 K 线，确认 DEXTools 显示的是 1 分钟 K 线
3. 测试深色/浅色主题切换，确认 K 线图主题正确切换

## 影响范围
- 仅影响 K 线图的默认显示时间周期
- 不影响其他功能
- 用户仍可在 DexScreener 或 DEXTools 界面中手动切换到其他时间周期
