## 1. Implementation
- [x] 1.1 新增 Alpha 数据读取方法，包含今日空投与稳定度数据。
- [x] 1.2 新增顶部 `AlphaTicker` 组件，完成字段归一化与紧凑展示。
- [x] 1.3 将组件挂载到 webapp 首页顶部居中区域，并处理登录态、访客态、工作模式的显示规则。
- [x] 1.4 补充样式，确保桌面和移动端不会挤压登录区或工作台配置。
- [x] 1.5 如浏览器直连接口受限，补充 webapp API 代理。
- [x] 1.6 补充后端 `/api/alpha` 代理，兼容 nginx 静态部署路径。

## 2. Validation
- [x] 2.1 运行 `npm run build`。
- [x] 2.2 做针对性 diff 检查，确认顶部栏、登录入口、访客页和工作台布局没有回归。
- [x] 2.3 运行 `go test ./service/web_server`。
