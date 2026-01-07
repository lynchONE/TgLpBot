## 1. Implementation
- [ ] 1.1 Register `/healthz` and `/readyz` routes in the web server
- [ ] 1.2 Implement dependency checks (config, MySQL ping, Redis ping; include optional ClickHouse/blockchain status)
- [ ] 1.3 Add Go unit tests for handler status codes and payload shape
- [ ] 1.4 (Optional) Add Docker Compose / deployment docs for using these endpoints as health checks
- [ ] 1.5 Run `cd backend; go test ./...`
