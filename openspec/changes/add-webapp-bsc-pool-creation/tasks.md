## 1. 后端参数与校验
- [x] 1.1 扩展 `create_pool_preview` / `create_pool_execute` 请求与响应结构，增加 `tick_spacing`、`amount_mode`、`min_price`、`max_price` 以及镜像金额字段
- [x] 1.2 收敛协议费率规则：`Uniswap V3` 固定 `100/500/3000/10000`，`PancakeSwap V3` 固定 `100/500/2500/10000`，`Uniswap V4` 支持任意静态 fee 并校验 `tick_spacing`
- [x] 1.3 实现 `full_range` / `custom_range` 的价格区间归一化与 tick 对齐
- [x] 1.4 为 preview 增加单侧输入镜像金额估算、估算来源与风险提示

## 2. 后端执行链路
- [x] 2.1 保持 V3 / Pancake V3 双币建池链路可用，并补齐固定费率与自定义区间执行
- [x] 2.2 实现 V3 / Pancake V3 的单币自动换币建池，复用现有换币/入池链路
- [x] 2.3 实现 V4 任意静态 fee + `tick_spacing` 的建池与自定义区间执行
- [x] 2.4 实现 V4 的单币自动换币建池，复用现有换币/入池链路

## 3. WebApp 交互
- [x] 3.1 更新创建池子面板的协议费率控件：V3 / Pancake V3 使用固定档位，V4 使用任意 fee 输入并展示 `tick_spacing`
- [x] 3.2 增加 `full_range` / `custom_range` 切换与价格区间输入
- [x] 3.3 增加单侧金额输入后的镜像金额展示，以及 `dual_exact` / `single_auto_swap` 模式切换
- [x] 3.4 更新 preview 摘要、风险提示、执行结果与错误展示

## 4. 验证
- [ ] 4.1 为后端补齐费率校验、区间归一化、镜像金额估算、V3 / V4 单币执行的定向测试
- [x] 4.2 完成 `webapp` 构建验证
- [ ] 4.3 在 BSC 环境手工验证三类协议的双币 / 单币建池流程
