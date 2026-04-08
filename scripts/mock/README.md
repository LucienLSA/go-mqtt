# Mock Scripts

## 脚本说明
- `seed_api_data.sh`: 登录、批量创建设备、可选模拟 webhook 上线事件。
- `batch_control.sh`: 登录、批量下发控制、查询命令历史。

## 运行前提
- 服务已启动：`go run ./cmd/server`
- `.env` 配置可用，数据库与 MQTT 可连接。
- Linux 依赖：`curl`、`jq`、`openssl`

先给脚本执行权限：
```bash
chmod +x ./scripts/mock/*.sh
```

## 1) 批量造测试数据
```bash
./scripts/mock/seed_api_data.sh --base-url http://localhost:8080 --device-count 10
```

如果开启了 webhook HMAC 校验，补充 secret：
```bash
./scripts/mock/seed_api_data.sh --base-url http://localhost:8080 --device-count 10 --webhook-secret "change-this-webhook-secret"
```

## 2) 批量下发控制
```bash
./scripts/mock/batch_control.sh --base-url http://localhost:8080 --limit 5 --command reboot --delay-seconds 1
```

## 常见问题
- 401: 登录失败或 token 无效。
- 403: 当前用户角色无权限。
- webhook 401: HMAC 签名头未配置或 secret 不一致。
- webhook 403: 请求来源 IP 不在白名单中。
