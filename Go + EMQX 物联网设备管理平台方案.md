# Go + EMQX 物联网设备管理平台：Vibe Coding 作战手册

面向目标：用最小可运行闭环，快速做出一个“能连、能看、能控、能演示”的 IoT 平台。

适用人群：想用 Go + EMQX 做项目、练工程能力、准备简历项目的人。

---

## 1. Vibe Coding 工作方式（先跑通，再变优雅）

这份文档不追求一次设计完美，而是追求持续可运行。

开发循环固定为：

1. 先定义一个最小目标（30-90 分钟可完成）。
2. 只改和目标直接相关的代码。
3. 立刻运行并观察日志。
4. 记录问题，先修阻塞，再做优化。
5. 完成一个闭环后再进入下一个目标。

一句话原则：每次提交都应该让项目“更能跑”。

---

## 2. 当前项目基线（按你现有目录落地）

当前目录已具备后端骨架：

- cmd/server/main.go：服务启动入口
- internal/config/config.go：配置管理
- internal/handler/device_handler.go：设备接口层
- internal/model/device.go：设备模型
- internal/repository/device_repository.go：设备数据访问
- cmd/simulator：模拟设备入口目录
- cmd/deploy/scripts.sh：部署脚本草稿

目标不是推翻重写，而是在这个骨架上逐步补齐能力。

---

## 3. 项目成功标准（Demo 视角）

你这个项目是否“成功”，用下面 5 条判断：

1. 可以新增设备，拿到 DeviceID 和 DeviceSecret。
2. 模拟设备能通过 EMQX 认证并连接。
3. 模拟设备上报数据后，后端能入库。
4. 可通过 API 下发控制指令，设备能收到并回执。
5. 能在 5 分钟内完整演示“注册 -> 上报 -> 控制 -> 回执”。

只要这 5 条达成，就已经是可展示的项目。

---

## 4. 技术栈（不贪多，够用就好）

| 分层 | 技术 | 本阶段定位 |
|---|---|---|
| MQTT Broker | EMQX 5.x | 设备接入、消息路由、认证联动 |
| 后端 | Go 1.21+ + Gin | API + 业务编排 |
| 数据层 | GORM + SQLite | 本地快速落地，后续可切 MySQL |
| MQTT 客户端 | paho.mqtt.golang | 订阅数据、发布指令 |
| 调试工具 | Postman + EMQX Dashboard | 接口联调、连接排错 |

备注：先别引入太多中间件。闭环稳定后再加 Redis、消息队列、监控系统。

---

## 5. 核心消息与接口约定（先统一，后开发）

### 5.1 Topic 规范

- 上报数据：device/{device_id}/data
- 控制下发：device/{device_id}/control
- 控制回执：device/{device_id}/feedback
- 心跳包：device/{device_id}/heartbeat

### 5.2 Payload 规范

数据上报示例：

```json
{
  "temp": 25.3,
  "humi": 48.1,
  "voltage": 3.7,
  "status": "running",
  "ts": 1710000000
}
```

控制指令示例：

```json
{
  "cmd": "open_fan",
  "param": 50,
  "trace_id": "ctrl-20260407-0001"
}
```

控制回执示例：

```json
{
  "trace_id": "ctrl-20260407-0001",
  "result": "ok",
  "message": "fan speed set to 50",
  "ts": 1710000005
}
```

### 5.3 API 最小集合

- GET /api/device
- POST /api/device
- PUT /api/device/:id
- DELETE /api/device/:id
- POST /api/device/:id/control
- GET /api/device/:id/command

接口返回建议统一格式：

```json
{
  "code": 0,
  "message": "ok",
  "data": {}
}
```

---

## 6. 四阶段 Vibe 开发路线（每阶段都可演示）

## 阶段 A：设备注册与认证打通（1-2 天）

目标：设备能合法接入。

任务卡：

1. 设计 device 表：device_id、device_secret、name、status、created_at、updated_at。
2. 实现设备 CRUD API。
3. 对接 EMQX HTTP 认证接口（用户名=DeviceID，密码=DeviceSecret）。
4. 接入 EMQX WebHook（connected/disconnected）更新设备在线状态。

验收标准：

- 错误凭据连接被拒绝。
- 正确凭据连接成功，状态变在线。
- 断开后状态能回写为离线。

## 阶段 B：数据采集与落库（1-2 天）

目标：设备数据可持续写入数据库。

任务卡：

1. 订阅 device/+/data。
2. 解析 JSON 为结构体，做字段校验。
3. 将有效数据落入 sensor_data 表。
4. 对异常消息只记录日志，不让服务崩溃。

验收标准：

- 模拟设备每 3 秒上报一次，数据库稳定新增记录。
- 非法 JSON 不会导致进程退出。

## 阶段 C：控制指令闭环（1-2 天）

目标：平台能下发命令，设备有反馈。

任务卡：

1. 实现 POST /api/device/:id/control。
2. 发布消息到 device/{device_id}/control。
3. 订阅 device/+/feedback，记录控制结果。
4. 提供命令历史查询接口。

验收标准：

- API 下发后 3 秒内收到回执。
- 历史记录能看到命令与执行结果。

## 阶段 D：可视化最小页（1-2 天）

目标：具备“可讲故事”的演示界面。

任务卡：

1. 设备列表页：在线状态、最后上报时间。
2. 设备详情页：最近 N 条温湿度数据。
3. 指令按钮：点击后可看到回执状态。

验收标准：

- 一台模拟设备可完成端到端演示。

---

## 7. 每日推进模板（照抄可用）

每日只做 3 件事：

1. 今天目标（一个闭环）：例如“打通设备认证”。
2. 今日产出（可运行）：例如“错误密钥被拒绝，正确密钥可连接”。
3. 阻塞与下一步：例如“WebHook 签名未校验，明天补上”。

建议记录格式：

```text
[Date]
Goal:
Done:
Blocked:
Next:
```

---

## 8. Vibe Coding 的 AI 协作提示词（可直接用）

用于让 AI 更稳地给你代码，不跑偏：

1. “只改一个文件，最小修改实现 X，保留现有结构。”
2. “先给出可运行版本，再列可选优化，不要一次性重构。”
3. “请附带 3 条手动验证步骤，确保我能立刻测试。”
4. “如果涉及 MQTT，请明确 topic、payload、QoS、异常处理。”
5. “输出内容按：改动点、代码、验证方式、回滚方式。”

---

## 9. 高频踩坑清单（提前规避）

1. 在 MQTT 回调里做重 DB 操作，导致消息堆积。
2. topic 命名不统一，后续调试成本暴涨。
3. 没有 trace_id，控制链路难以定位问题。
4. 设备断线后状态未更新，前端显示失真。
5. 异常没有降级策略，一条坏消息拖垮服务。

建议底线：任何单条消息错误都不应导致服务退出。

---

## 10. 第二阶段之后再做的优化（避免过早优化）

当 A/B/C 阶段稳定后，再考虑：

1. SQLite -> MySQL，补索引（device_id、ts）。
2. JWT 鉴权与 RBAC。
3. 告警阈值、告警历史与处理流。
4. Prometheus + Grafana 监控。
5. Docker Compose 一键部署。

---

## 11. 最终交付建议（用于学习和简历）

至少准备以下内容：

1. 一段 3-5 分钟演示视频（注册、上报、控制、回执）。
2. 一份架构图（设备 -> EMQX -> Go 服务 -> DB -> 前端）。
3. 一份接口文档（设备、数据、控制）。
4. 一份问题复盘（遇到什么坑，怎么解）。

这四项比“写了多少代码”更能体现工程能力。

---

## 12. 一句话收尾

把这个项目当作“持续可运行的工程实验”，不是“追求一次完美的系统设计”。

先把链路跑通，再把细节磨好，你的成长速度会快很多。
