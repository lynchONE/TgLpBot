 # TgLpBot Mini App (实时仓位)

## 开发

```bash
npm install
npm run dev
```

默认会请求 `http://localhost:8080/api/realtime_positions`，可通过环境变量覆盖：

```bash
VITE_API_BASE_URL=http://localhost:8080
```

## 部署

部署到 Vercel 后，将部署地址填到后端的 `TELEGRAM_WEBAPP_URL`（`backend/.env`）。

