# EMQX Neuron 接入指南（基于当前 go-mqtt 项目）

目标：让 Neuron 作为协议网关接入 EMQX，你的 Go 服务继续负责设备管理与业务接口。

---

## 1. 架构建议

推荐链路：

1. 工业设备（Modbus / OPC UA / S7 等）
2. Neuron（南向采集 + 北向 MQTT）
3. EMQX（连接管理 + 规则路由）
4. go-mqtt 服务（设备管理、状态、控制）

说明：

- 你的 Go 服务不直接对接工业协议。
- Neuron 负责协议适配与采集。
- EMQX 负责消息中转和认证。

---

## 2. 当前项目已支持的接入点

你项目里已提供 EMQX 鉴权与 WebHook：

1. POST /emqx/auth
2. POST /emqx/webhook
3. POST /api/v1/emqx/auth
4. POST /api/v1/emqx/webhook

新增支持：

- 你可以用环境变量配置 Neuron 网关专用账号：
  - NEURON_GATEWAY_USERNAME
  - NEURON_GATEWAY_PASSWORD

这样 Neuron 连接 EMQX 时可走网关账号，不会和设备账号冲突。

---

## 3. 先启动你的服务

Windows PowerShell 示例：

```powershell
$env:NEURON_GATEWAY_USERNAME="neuron-gw"
$env:NEURON_GATEWAY_PASSWORD="neuron-gw-secret"
go run .\cmd\server
```

验证：

```bash
curl -X GET "http://localhost:8080/ping"
```

---

## 4. 在 EMQX 中配置 HTTP 认证

在 EMQX Dashboard 中创建 HTTP 认证器（MQTT）：

1. Method: POST
2. URL: http://<你的服务IP>:8080/emqx/auth
3. Body(JSON):

```json
{
  "username": "${username}",
  "password": "${password}",
  "clientid": "${clientid}"
}
```

4. 期望返回：

```json
{"result":"allow","is_superuser":false}
```

---

## 5. 在 EMQX 中配置 WebHook

创建 WebHook（或 Rule + HTTP Action）发送连接事件到：

- http://<你的服务IP>:8080/emqx/webhook

建议发送字段：

```json
{
  "event": "${event}",
  "action": "${action}",
  "username": "${username}",
  "clientid": "${clientid}"
}
```

说明：

- connected/disconnected 事件会触发设备在线状态更新。
- 如果 username 不是设备 ID（例如 neuron-gw），状态更新会被忽略，不影响流程。

---

## 6. 在 Neuron 中配置北向 MQTT

在 Neuron Web 控制台中：

1. 创建北向 MQTT 应用节点。
2. Broker 地址填 EMQX 地址（例如 tcp://192.168.71.128:1883）。
3. Username: neuron-gw
4. Password: neuron-gw-secret
5. QoS: 0 或 1（建议先用 0）
6. 数据格式：JSON（先别上 Protobuf）

建议先用 Neuron 默认 topic 验证连通，再做业务 topic 适配。

---

## 7. Topic 对齐策略（推荐）

你的业务 topic 是：

1. device/{device_id}/data
2. device/{device_id}/control
3. device/{device_id}/feedback

如果 Neuron 当前上报为 neuron/{node}/upload，可选两种方式：

1. 在 Neuron 中自定义上报 topic，直接改为 device/{device_id}/data。
2. 在 EMQX Rule Engine 做 topic 重写后再转发。

建议优先方案 1，链路最短，排障最简单。

---

## 8. 最小联调步骤

1. 启动 go-mqtt（带 NEURON_GATEWAY_* 环境变量）。
2. 在 EMQX 配好 HTTP 认证与 WebHook。
3. 在 Neuron 配好北向 MQTT，确认连接状态为 connected。
4. 用 Neuron 上报一条数据。
5. 在 EMQX 订阅对应 topic，确认消息到达。
6. 再把 topic 对齐到 device/{device_id}/data，进入你的业务链路。

---

## 9. 常见问题

1. Neuron 连接 EMQX 被拒绝
- 检查 NEURON_GATEWAY_USERNAME / PASSWORD 是否和 Neuron 配置一致。
- 检查 EMQX HTTP 认证 URL 是否能访问到你的服务。

2. WebHook 调用失败
- 检查服务地址是否可被 EMQX 访问（注意容器网络与本机回环地址）。

3. 上报 topic 对不上
- 先在 EMQX 里订阅 #，确认 Neuron 实际发布的 topic，再做映射。

---

## 10. 下一步建议

当连通稳定后再做：

1. 在 Go 服务中新增 MQTT 订阅模块（消费 Neuron 上报并落库）。
2. 对 command/control 增加 trace_id 贯穿。
3. 增加 EMQX 规则引擎模板，固化 topic 映射。