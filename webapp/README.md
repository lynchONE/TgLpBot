# Web Workbench

独立的 Web 端工作台项目（与 `miniapp/` 分离），用于桌面端组合展示：

- 热门池子
- K线
- 仓位
- 聪明钱

## 特性

- 4 模块可自由开关组合
- 支持模块拖拽重排（按拖拽后的顺序展示）
- 布局规则：
  - 4 模块：2x2
  - 3 模块：三列
  - 2 模块：上下两行
  - 1 模块：单模块全屏
- 接入现有后端 API：`hot_pools / pool_ohlcv / realtime_positions / smart_money`
- 右上角 Telegram 登录：后端鉴权权限并换取 `initData + 用户资料`
- 偏 OKX 风格暗色高对比设计

## 启动

```bash
cd webapp
npm install
npm run dev
```

## 配置

通过环境变量配置，不再在页面填写 `apiBaseUrl/initData`。

新建 `webapp/.env.local`：

```bash
VITE_API_BASE_URL=http://localhost:8080
VITE_TELEGRAM_BOT_ID=123456789
VITE_TELEGRAM_BOT_USERNAME=your_bot_username
VITE_DEFAULT_CHAIN=bsc
```

## 登录与鉴权说明

- 点击右上角 Telegram 图标（`webapp/src/img/telegram.svg`）唤起二维码登录。
- 前端拿到 Telegram 登录回调后调用：`/api/web_login?endpoint=telegram_login`。
- 后端会：
  - 校验 Telegram 登录签名
  - 校验该用户是否有 Bot/MiniApp 权限
  - 生成并返回可用于现有接口鉴权的 `initData`
  - 返回用户头像、昵称等资料
