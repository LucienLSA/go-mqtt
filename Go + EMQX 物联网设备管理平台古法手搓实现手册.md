# Go + EMQX 物联网设备管理平台：古法手搓实现手册

面向目标：不用花哨框架堆叠，不依赖一键脚手架，按最朴素工程流程手工把链路打通。

适用人群：想真正理解每一行代码为什么存在、准备做可演示 IoT 项目的人。

---

## 1. 古法编程原则（先能跑，再求好）

古法不是“慢”，而是“可控”：

1. 每次只做一个最小闭环。
2. 每次只改少量文件。
3. 改完立刻运行，先看日志再看感觉。
4. 出错先定位再优化，不做猜测性重构。
5. 保证随时可以回退到上一个可运行状态。

一句话：今天写的代码，今天就要能演示。

---

## 2. 你的现有目录如何手搓推进

当前目录已经很好，不需要推翻：

- cmd/server/main.go：服务启动与路由注册
- internal/config/config.go：配置读取
- internal/model/device.go：设备实体与数据结构
- internal/repository/device_repository.go：数据库访问
- internal/handler/device_handler.go：HTTP 接口
- cmd/simulator：模拟设备程序
- cmd/deploy/scripts.sh：部署脚本草稿

古法建议：保持目录不动，只做增量补齐。

---

## 3. 手搓前准备（一次到位）

### 3.1 运行环境

1. Go 1.21+
2. EMQX 5.x（本机或 Docker）
3. SQLite（本地文件即可）
4. Postman 或 curl
5. MQTTX（建议）或任意 MQTT 客户端

### 3.2 首次启动检查清单

1. 服务能启动，不 panic。
2. 健康接口能返回 200。
3. 数据库文件会自动创建。
4. EMQX Dashboard 可看到连接状态。

### 3.3 第 0 天启动清单（先拿到正反馈）

今天只做三件事，不做额外设计：

1. 启动服务并确认健康接口返回 200。
2. 手动创建设备，拿到 device_id 和 device_secret。
3. 用一台模拟设备完成一次连接和一次数据上报。

第 0 天完成定义（DoD）：

1. 你能在 10 分钟内重复完成上述 3 件事。
2. 全程有日志证据（启动、创建设备、连接、上报）。
3. 出现报错时你能定位到是 API、DB 还是 MQTT 链路问题。

---

## 4. 数据模型与消息契约（先写死，后扩展）

### 4.1 Topic 统一规范

1. 上报：device/{device_id}/data
2. 控制：device/{device_id}/control
3. 回执：device/{device_id}/feedback
4. 心跳：device/{device_id}/heartbeat

### 4.2 核心表（最小可用）

建议先做三张表：

1. device
- id
- device_id
- device_secret
- name
- status
- last_seen_at
- created_at
- updated_at

2. sensor_data
- id
- device_id
- temp
- humi
- voltage
- status
- ts
- created_at

3. command_log
- id
- device_id
- trace_id
- cmd
- param
- result
- message
- issued_at
- ack_at

---

## 5. 古法四阶段实现路线

## 阶段 A：设备注册与认证

目标：设备凭证真能决定是否允许连接。

实现步骤：

1. 在 internal/model/device.go 定义 Device 结构体和状态字段。
2. 在 internal/repository/device_repository.go 手写 CRUD 方法。
3. 在 internal/handler/device_handler.go 实现设备 CRUD API。
4. 在 cmd/server/main.go 注册路由。
5. 实现给 EMQX 的认证查询接口（用户名=DeviceID，密码=DeviceSecret）。
6. 对接 EMQX WebHook connected/disconnected，更新设备在线状态。

验收：

1. 错误密码连接失败。
2. 正确密码连接成功。
3. 设备断线后状态改为 offline。

完成定义（DoD）：

1. 错误密钥连接必失败，返回结果可复现。
2. 正确密钥连接必成功，且设备状态在 5 秒内更新为 online。
3. 设备断开后状态在 10 秒内更新为 offline。

失败排查：

1. 先查 EMQX 认证请求是否打到后端。
2. 再查数据库是否能查到设备凭证。
3. 最后查密码比对逻辑和 JSON 响应格式。

## 阶段 B：数据上报与落库

目标：数据能稳定写库，异常消息不拖垮服务。

实现步骤：

1. 在 MQTT 客户端初始化里订阅 device/+/data。
2. 回调里先取 device_id，再反序列化 payload。
3. 做字段校验，非法直接记录日志并 return。
4. 合法数据写入 sensor_data。
5. 更新设备 last_seen_at。

验收：

1. 模拟设备每 3 秒上报一次，数据库持续新增。
2. 随机发非法 JSON，服务仍持续运行。

完成定义（DoD）：

1. 连续运行 10 分钟，上报数据条数与预期误差不超过 5%。
2. 非法 JSON 连发 20 条，服务不退出且错误日志可检索。
3. 设备 last_seen_at 能跟随最新有效上报更新。

失败排查：

1. 先确认 topic 是否订阅成功。
2. 再确认 JSON 字段名与结构体标签一致。
3. 再看数据库写入错误日志。

## 阶段 C：控制指令闭环

目标：平台下发命令，设备能回执，后端可查历史。

实现步骤：

1. 实现 POST /api/device/:id/control。
2. 生成 trace_id，先写 command_log 为 pending。
3. 发布到 device/{device_id}/control。
4. 订阅 device/+/feedback。
5. 按 trace_id 回写 command_log 的 result/message/ack_at。
6. 增加命令历史查询接口。

验收：

1. 下发后 3 秒内能收到回执。
2. 历史接口可看到 pending 变 ok 或 fail。

完成定义（DoD）：

1. 连续下发 10 条指令，至少 9 条在 3 秒内拿到回执。
2. 每条指令都能按 trace_id 查到完整记录（下发+回执）。
3. 没回执的指令会保留 pending，不会被错误覆盖。

失败排查：

1. 先看 control topic 是否发出。
2. 再看设备是否订阅正确 topic。
3. 再查 feedback 中 trace_id 是否和下发一致。

## 阶段 D：最小演示页（可选）

目标：5 分钟能讲完一个完整业务故事。

页面最小功能：

1. 设备列表：在线状态、最后上报时间。
2. 设备详情：最近 N 条温湿度数据。
3. 指令按钮：下发后显示回执状态。

如果暂时不做前端，也可用 Postman + Dashboard 演示。

完成定义（DoD）：

1. 你能在 5 分钟内演示注册 -> 上报 -> 控制 -> 回执。
2. 演示过程不依赖临时改代码，只做正常操作。
3. 出问题时你能在 3 分钟内定位到具体链路环节。

---

## 6. 手搓代码顺序（按文件执行）

建议严格按顺序，不跳步：

1. internal/model/device.go
2. internal/repository/device_repository.go
3. internal/handler/device_handler.go
4. internal/mqtt（连接、订阅、发布）
5. cmd/server/main.go（组装依赖）
6. cmd/simulator（模拟设备上报与回执）

每完成一项都做一次手动回归：

1. 能启动
2. 能请求
3. 能入库
4. 能收发 MQTT

### 6.1 手写优先级规则（防止卡死在细节）

每个功能都按这个顺序推进：

1. 先写主流程（happy path），保证功能可跑。
2. 再补参数校验（缺字段、类型错误、空值）。
3. 再补日志字段（trace_id、device_id、action、error）。
4. 最后补异常分支和 recover，保证错误不拖垮服务。

禁止事项：

1. 主流程没跑通前，不做重构。
2. 验收没通过前，不做性能优化。
3. 单次改动超过 2 个模块时，先拆任务再继续。

---

## 7. 古法调试法（不用玄学）

问题定位顺序固定：

1. 先看输入：请求参数、topic、payload。
2. 再看处理：路由是否命中、回调是否触发。
3. 再看输出：数据库写入、MQTT 发布是否成功。
4. 最后看副作用：设备状态、命令状态有没有更新。

建议日志字段固定包含：

1. trace_id
2. device_id
3. topic
4. action
5. error

原则：日志先于断点，证据先于猜测。p

### 7.1 报错处理决策树（固定动作）

按顺序执行，不要跳步：

1. 看输入：请求参数、topic、payload 是否符合约定。
2. 看路由：接口是否命中，MQTT 回调是否被触发。
3. 看 DB：查询和写入是否报错，事务是否提交。
4. 看 MQTT：订阅是否成功，发布是否返回成功 token。
5. 看外部依赖：EMQX、网络、端口、防火墙是否异常。

定位输出模板：

1. 现象：发生了什么。
2. 证据：日志或返回值是什么。
3. 根因：哪一层出错。
4. 修复：改了什么。
5. 回归：如何复测通过。

---

## 8. 手动验收脚本清单（每次发版前）

1. 创建设备，记录 device_id 与 device_secret。
2. 用错误密钥连接，确认被拒绝。
3. 用正确密钥连接，确认在线。
4. 模拟设备上报 10 条数据，确认入库数量一致。
5. 下发 3 条控制命令，确认都收到回执。
6. 断开设备，确认状态离线。

---

## 9. 每日古法记录模板

可直接复制：

[Date]
Goal:
Done:
Issue:
Fix:
Next:

要求：每天至少有一个可演示结果。

---

## 10. 常见坑位与古法解法

1. 坑：MQTT 回调里做重查询导致阻塞。
解法：回调只做解析和轻写入，复杂逻辑异步化。

2. 坑：topic 拼错一个字符全链路静默失败。
解法：把 topic 常量集中定义，不手写魔法字符串。

3. 坑：没有 trace_id，控制链路无法排查。
解法：下发时必须生成 trace_id，日志和表都保存。

4. 坑：离线状态不同步，页面显示失真。
解法：WebHook + 心跳双保险更新在线状态。

5. 坑：异常消息直接 panic。
解法：所有消息处理先 recover，错误只打日志不退出。

---

## 11. 古法完成标准（能讲、能演示、能复盘）

满足以下条件即可认为项目阶段完成：

1. 注册 -> 连接认证 -> 上报 -> 控制 -> 回执全链路打通。
2. 任意单条坏消息不会导致服务退出。
3. 你能在 5 分钟内现场演示并解释每一步。
4. 你知道每个关键文件负责什么。

---

## 12. 一句话收尾

古法手搓的价值不是代码写得多，而是你对系统行为有完整掌控。先打穿闭环，再逐步精进架构。