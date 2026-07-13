# 应用中心最终方案：智能体、工作流与对象存储

## 0. 一页总结

应用中心的最终定位：

```text
Sub2API 负责平台能力
  用户、API Key、分组、模型网关、计费、UsageLog、应用中心、运行记录、对象存储。

Worker 负责代码能力
  智能体、复杂工作流、生图、生视频、文档处理等真实业务逻辑。
```

核心闭环：

```text
开发者写 Worker 代码
  ↓
部署 Worker 服务
  ↓
管理员在 Sub2API 配置 Worker Host
  ↓
管理员创建应用版本，绑定 Worker Host、运行路径、输入输出、能力和默认模型
  ↓
Sub2API 健康检查和试运行
  ↓
管理员发布版本
  ↓
用户运行应用，选择自己的平台 API Key
  ↓
Sub2API 创建 run 并入队
  ↓
Sub2API Runner 调用 Worker
  ↓
Worker 通过 Sub2API Model Proxy 调模型
  ↓
Sub2API 完成鉴权、计费、UsageLog、对象存储和结果展示
```

最重要的设计结论：

- 管理员不是上传代码的人，管理员负责发布应用、配置 Worker Host、配置版本和试运行。
- 智能体和复杂工作流代码放在独立 Worker 项目里，不放进 Sub2API 主进程。
- Worker 推荐做成 `Runtime + App Packages` 架构，新增智能体/工作流时新增 handler，不重写 Worker 框架。
- Sub2API 应用中心必须支持 `Worker Host` 管理，包含 `Base URL`、鉴权、健康检查、并发、超时。
- 应用版本绑定 `worker_host_id + worker_route`，不要在各处散填完整 URL。
- Worker 不能拿用户 API Key 明文，模型调用必须回到 Sub2API Model Proxy。
- 用户运行应用时只能选择自己在平台内创建的 API Key。
- 运行结果、图片、视频、文件、大文本进入对象存储，DB 只保存引用和元数据。
- 不停机更新靠 Worker 版本化和 `published_version_id` 原子切换。
- 正在运行的任务绑定创建时的版本，新版本发布不影响旧任务。
- 第一版先做自研轻量应用中心和 Runner，Coze/Dify 可作为外部编排后端接入，不建议一开始深度二开。

第一版最小落地：

```text
Sub2API
  agent_apps
  agent_worker_hosts
  agent_app_versions
  agent_runs
  Runner 队列
  Model Proxy
  Artifact Service

Worker
  /health
  /runs
  /cancel 可选
  callback Sub2API

对象存储
  腾讯云 COS 香港私有 Bucket
```

第一版部署建议：

```text
主服务器
  Sub2API Web/API
  Sub2API Redis / 平台队列
  DB
  Runner

Worker 服务器
  可以先同机 Docker 容器
  也可以另一台裸服务器 Docker Compose
  不绑定域名也可以，用 IP:端口，但要限制防火墙和签名鉴权
  Worker 如需 Redis，应使用 Worker 自己的 Redis，不和 Sub2API 共用
```

推荐技术栈：

```text
Sub2API 后端
  Go + Gin + ent/migrations + Sub2API Redis
  继续沿用当前项目技术栈，不重写。

Sub2API 前端
  Vue 3 + TypeScript + Vite
  继续沿用当前项目风格。

Sub2API Runner
  Go
  可以先作为主服务内 goroutine/worker pool，后续拆成独立 cmd。

Worker 协议
  HTTP + JSON + HMAC 签名 + run_token
  跨语言，不绑定某一种框架。

官方 Worker 模板
  AI 能力 Worker 首选 Python + FastAPI + Pydantic
  平台型/高性能 Worker 可选 Go + Gin/Fiber
  TypeScript + Fastify + Zod 可作为接口编排型模板
```

开发节点：

```text
P0 协议定稿
  Worker Host、App Version、Run、Callback、Model Proxy、Artifact 协议。

P1 数据模型
  agent_worker_hosts、agent_apps、agent_app_versions、agent_runs、agent_input_assets、agent_artifacts。

P2 管理后台
  Worker Host 管理、应用版本创建、健康检查、试运行、发布、回滚。

P3 运行闭环
  用户运行应用、选择 API Key、创建 run、入队、Runner 调 Worker。

P4 Model Proxy
  Worker 通过 Sub2API 调模型，Sub2API 负责用户 Key、分组、限速、UsageLog。

P5 Artifact
  输入文件和结果产物进对象存储，DB 只保存引用。

P6 Worker Runtime 模板
  /health、/runs、/cancel、验签、并发、回调、SDK、示例 app。

P7 并发和稳定性
  队列、租约、重试、幂等、超时、取消、限流、日志。
```

进程控制：

```text
每个阶段都必须有可验收结果。
每个阶段都能单独测试。
先跑通一个最小文本 Worker，再扩展图片、文件、视频。
不先做拖拽式工作流编辑器。
不先深度二开 Coze/Dify。
不让 Worker 绕过 Sub2API 模型网关。
```

当前实现审计提示：

```text
截至当前开发状态，Sub2API 应用中心只能视为底座和示例闭环，不应视为完整生产版。

当前可优先验收：
  Worker Host 健康检查
  文本 Demo 应用发布
  用户选择平台内 API Key 发起运行
  用户按节点/角色选择平台内 API Key
  用户上传图片/文件输入并生成 input asset 引用
  Sub2API Redis Stream 运行队列
  Sub2API Runner consumer group 消费 run
  pending run 通过 XPENDING + XCLAIM 恢复消费，兼容现有旧 Redis
  Worker Host 派发槽位优先使用 Redis 分布式计数
  Worker 收到 input_assets / input_artifacts
  Worker 回调 succeeded
  Worker 上传或登记产物，用户获取短期下载链接
  用户可取消 queued/running run
  Sub2API 使用 Redis 暂存运行期 run token，并向 Worker /cancel 传播取消信号
  Sub2API 写入最小运行事件流，用户运行详情可轮询查看时间线
  应用中心运行事件日志作为审计和时间线长期保留，不接入自动清理

当前不能宣称完成：
  完整发布、试运行、回滚、下架
  完整重试策略、强制中断式取消、SSE 实时事件推送和 Runner 监控
  回调和产物写入的完整幂等保障
  UsageLog 关联 app/run/node/node_role
  用户主动删除/归档结果、空间额度、收藏结果保留策略
  历史产物一键转输入资产
  多 Worker 多实例调度和失败迁移
```

后续开发时，状态口径必须区分：

```text
基础接口存在      不等于 功能生产可用。
Demo 能跑通       不等于 多元智能体/工作流已完成。
对象存储接口存在  不等于 输入资产、产物归档、清理策略完整闭环。
```

## 1. 方案定位

应用中心不是单纯的智能体列表，也不是只保存 Prompt 的工具页，而是一个站内 AI 应用发布与运行平台：

```text
应用中心 = App Catalog + Workflow/Agent Runtime + 用户 API Key 授权 + 对象存储结果管理 + 现有网关计费
```

核心目标：

- 管理员可以发布应用，应用可以是提示词、工作流、智能体或外部托管服务。
- 用户必须使用自己在平台内创建并归属自己的 API Key 运行应用，运行表单只能选择 Key，不能填写或上传 API Key 明文。
- 所有模型调用继续复用现有 API Key、分组、余额、套餐、限速、风控、UsageLog。
- 应用发布、下架、删除不需要服务停机。
- 工作流节点可以声明不同模型和不同能力分组。
- 运行结果、图片、视频、文件、大文本统一进入对象存储，数据库只保存索引和对象引用。

本方案中的“应用”是统一抽象，智能体和工作流都是应用的一种运行类型。

## 2. 核心概念

```text
App 应用
  prompt       简单提示词应用
  workflow     固定流程编排
  agent        目标驱动、自主决策
  external     外部 Worker/服务托管的复杂应用
```

智能体和工作流的区别：

```text
智能体 = 目标驱动，运行时可以动态决策下一步
工作流 = 流程驱动，按预设节点、分支和依赖执行
```

工程上二者共用以下底座：

- 应用目录。
- 版本发布。
- 输入表单。
- API Key 授权。
- 节点执行。
- 用量计费。
- 事件流。
- 结果与产物管理。

## 3. 总体架构

```text
前端
  用户应用中心 / 应用详情 / 运行表单 / 运行记录 / 结果页 / 产物下载
  管理员应用管理 / 版本发布 / 节点配置 / 运行审计 / 用量统计

后端
  App Catalog          应用目录
  Version Manager      应用版本管理
  Runtime Executor     Prompt/Workflow/Agent 执行器
  Key Resolver         用户 API Key 与能力绑定解析
  Node Scheduler       节点调度与状态机
  Gateway Adapter      调用现有模型网关
  Artifact Service     结果和产物管理
  Event Stream         SSE/轮询进度
  Cleanup Job          过期清理

存储
  DB                   状态、索引、元数据、审计
  对象存储              完整结果、图片、视频、文件、日志归档
  Sub2API Redis         平台运行中状态、队列、短期事件缓存
```

所有模型调用必须走平台内部网关或网关适配层，不能绕过 Sub2API 的现有计费链路。

### 3.1 代码型智能体/工作流闭环技术方案

代码型智能体和工作流的完整闭环应该是：

```text
开发者开发 Worker
  ↓
部署 Worker 到服务器或容器
  ↓
管理员在 Sub2API 配置 Worker Host
  ↓
管理员创建应用版本并绑定 Worker Host
  ↓
Sub2API 健康检查和试运行
  ↓
管理员发布版本
  ↓
用户选择应用、填写输入、选择平台 API Key
  ↓
Sub2API 创建运行并入队
  ↓
Sub2API Runner 调用 Worker
  ↓
Worker 通过 Sub2API 模型代理调用模型
  ↓
Worker 回调进度和产物
  ↓
Sub2API 计费、写 UsageLog、归档对象存储、展示结果
```

这里的关键是：Sub2API 应用中心必须支持管理员配置 Worker Host。

Worker Host 是一个可复用的外部执行服务配置，不等同于某个具体应用版本。一个 Host 可以服务多个应用，也可以按版本拆成多个 Host。

```text
Worker Host 配置
  名称
  部署位置
  Base URL / Host
  协议版本
  健康检查路径
  运行路径
  取消路径
  鉴权方式
  签名密钥引用
  最大并发
  默认超时
  状态
```

示例：

```text
name = video-worker-hk-1
base_url = http://10.0.0.8:8080
protocol = sub2api-worker-v1
health_path = /health
run_path = /runs
cancel_path = /cancel
auth_type = hmac_run_token
max_concurrency = 4
timeout_seconds = 1800
```

如果没有内网，也可以临时使用：

```text
base_url = http://公网IP:8080
```

生产环境更推荐：

```text
base_url = https://worker.example.com
```

应用版本绑定 Worker Host，而不是让管理员在每个地方随便填完整 URL：

```text
agent_app_versions
  app_id
  version
  runtime_type = worker
  worker_host_id = video-worker-hk-1
  worker_route = /video/v2/runs
  worker_health_route = /video/v2/health
  image_ref = registry.example.com/video-worker:v2
  source_ref = git commit / release tag
  input_schema_json
  output_schema_json
  capabilities_json
```

Sub2API 调用 Worker 时拼出实际地址：

```text
run_url = worker_host.base_url + app_version.worker_route
health_url = worker_host.base_url + app_version.worker_health_route
```

这样做的好处：

- 管理员可以在后台维护 Worker Host。
- 某台 Worker 服务器换 IP 或域名时，只改 Host，不需要逐个改应用。
- 应用版本仍然可以绑定不同 Worker 路由，实现 v1/v2 并存。
- 可以对每个 Host 做健康检查、并发限制、超时和熔断。
- Sub2API 主服务不需要停机，也不需要加载智能体代码。

Sub2API 和 Worker 的连接协议：

```text
Sub2API -> Worker
  POST {worker_base_url}{worker_route}
  Header:
    X-Sub2API-Run-Token
    X-Sub2API-Signature
    X-Sub2API-Timestamp
  Body:
    run_id
    app_id
    app_version_id
    input
    input_assets
    input_artifacts      兼容字段，内容同 input_assets
    callback_url
    model_proxy_url
    artifact_url         Worker 可登记外部产物或追加 /upload 上传产物
    timeout_seconds

Worker -> Sub2API
  POST callback_url
  Header:
    X-Worker-Signature
  Body:
    run_id
    status
    progress
    node_id
    artifacts
    error
```

Worker 调模型不能直接拿用户 API Key，而是调用：

```text
POST {model_proxy_url}
```

Sub2API 在模型代理层根据 `run_token` 找到当前用户、用户选择的 `api_key_id`、节点/角色绑定、分组、余额和模型权限，然后复用现有模型网关。

输入资产下发原则：

```text
用户上传图片/文件
  POST /api/v1/agent-input-assets
  ↓
Sub2API 上传到对象存储，DB 只保存 agent_input_assets 引用
  ↓
用户创建运行时传 input_asset_ids
  POST /api/v1/agent-apps/{id}/runs
  ↓
Sub2API 校验资产归属并把引用快照写入 agent_runs.input_summary_json
  ↓
Sub2API 派发 Worker 时生成短期下载 URL
  input_assets / input_artifacts
```

最小闭环需要实现的模块：

```text
Sub2API 后端
  Worker Host 管理
  应用版本管理
  运行队列
  Runner
  Worker 调用客户端
  Model Proxy
  Callback 接收器
  Artifact Service
  UsageLog 关联

Worker 项目
  /health
  /runs
  /cancel 可选
  业务逻辑
  Sub2API Model Proxy Client
  Callback Client
  Dockerfile

Sub2API 前端
  Worker Host 配置页
  应用版本绑定 Worker Host
  健康检查
  试运行
  发布 / 回滚
```

## 4. 发布与热插拔设计

管理员在后台发布的是一份版本化的应用配置，而不是直接把任意 Go 代码加载进主进程。

产品界面上，这份配置应该表现为“基础信息 + 输入项 + 节点编排 + 模型绑定 + 发布设置”的表单和列表。后端可以把它保存为内部 Manifest，用于校验、版本化、热加载和导入导出；普通管理员不需要手写 JSON。

推荐支持三种运行形态：

```text
1. Declarative App
   由内部 manifest 描述 prompt / workflow，后端解释执行。

2. Built-in Node
   平台内置节点：模型调用、HTTP 请求、条件判断、JSON 转换、图片生成、视频生成。

3. External App
   复杂应用由外部 Worker 承载，平台通过 HTTP/Webhook 调用。
```

发布流程：

```text
管理员编辑草稿
  ↓
后端校验应用配置并生成内部 manifest
  ↓
保存 agent_app_versions
  ↓
点击发布
  ↓
原子切换 published_version_id
  ↓
运行时缓存失效并热加载
```

删除流程：

```text
下架应用
  ↓
软删除 agent_apps
  ↓
保留历史 agent_runs 与 version 快照
```

设计原则：

- 新运行使用最新已发布版本。
- 进行中的运行绑定创建时的版本，不受新发布影响。
- 下架只影响新运行，不中断已有运行。
- 删除使用软删除，审计和用量记录保留。

## 5. Key、模型与分组策略

管理员不能指定使用某个用户 API Key。管理员只能声明节点需要什么能力。

用户也不能在应用运行表单里手动填写 API Key。应用中心只能展示当前登录用户在平台内已创建、未删除、可用或可诊断的 API Key 列表，并让用户选择其中一个或多个 Key。后端创建运行时只接收 `api_key_id` 或能力绑定对象，不接收 `sk-...` 这类明文 Key 字符串。

能力标签示例：

```text
text
image
video
long_context
cheap
premium
web_search
```

### 5.1 节点与模型角色

节点不一定只对应一次模型调用。特别是生图、生视频、文档分析、报告生成这类能力，通常都是复合节点。

例如一个生图节点可能包含：

```text
商品信息理解          text
生图提示词改写        text
负面词/风格参数生成   text
图片生成              image
图片结果总结          text 或 vision
```

因此，应用内部结构里不能只支持“节点级模型”，还要支持“节点内角色级模型绑定”：

```text
node
  └─ model_bindings
       prompt_rewrite   -> text model
       image_generate   -> image model
       caption          -> text/vision model
```

用户侧也不应该被迫给每个内部小步骤逐一选模型。推荐交互是：

```text
普通用户：
  从平台内 API Key 列表中按能力选择 Key，使用管理员配置的默认模型。

高级用户：
  展开高级设置，覆盖某个节点或某个模型角色的模型。
```

也就是说，用户运行时主要完成能力授权：

```text
text  使用哪个 API Key / 分组 / 默认模型
image 使用哪个 API Key / 分组 / 默认模型
video 使用哪个 API Key / 分组 / 默认模型
```

运行时再由节点内部角色按能力取对应绑定：

```text
prompt_rewrite -> text binding
image_generate -> image binding
caption        -> text 或 vision binding
```

这样可以避免一个“生图节点”只绑定生图模型，却遗漏文字处理、提示词改写和结果总结的成本与权限。

### 5.2 MVP 策略

第一版建议使用单 API Key 运行：

```text
用户运行应用时选择一个 API Key
  ↓
所有节点和节点内部模型角色都使用该 Key 的身份和分组
  ↓
节点或模型角色可以使用不同模型
  ↓
所有模型必须在该 Key 对应分组允许范围内
```

这种方式实现简单，权限边界清晰，也最容易复用现有 API Key 鉴权、余额、限速和 UsageLog。

MVP 的运行请求示例：

```json
{
  "input": {
    "product_name": "轻薄冲锋衣",
    "selling_points": "防水、透气、适合通勤和徒步"
  },
  "api_key_id": 12345
}
```

注意：请求体只传平台内 API Key 的 ID。后端必须通过当前登录用户校验该 ID 的归属和状态，不能接受明文 API Key。

如果一个应用同时需要 `text` 和 `image`，而用户选择的 API Key 分组不支持其中任一能力，则运行前应直接提示不可运行。

### 5.3 增强版策略

支持按能力绑定多个 API Key：

```text
text  节点使用 Key A
image 节点使用 Key B
video 节点使用 Key C
```

运行前由用户授权：

```text
能力标签 -> 用户 API Key -> API Key 分组 -> 可用模型/渠道
```

增强版运行请求示例：

```json
{
  "input": {
    "product_name": "轻薄冲锋衣",
    "selling_points": "防水、透气、适合通勤和徒步"
  },
  "key_bindings": {
    "text": { "api_key_id": 12345 },
    "image": { "api_key_id": 67890 }
  }
}
```

这里的 `api_key_id` 仍然必须来自当前用户在平台内创建的 Key 列表。

复合节点会按模型角色读取对应能力绑定：

```text
image_node.prompt_rewrite -> text 绑定
image_node.image_generate -> image 绑定
video_node.script         -> text 绑定
video_node.render         -> video 绑定
```

### 5.4 高级版策略

节点或模型角色只声明能力，平台自动从用户可用 Key 和分组中选择：

```text
node.model_bindings.image_generate.capability = image
  ↓
查找用户可用 image API Key
  ↓
按成本、质量、可用性选择分组
  ↓
失败时按 fallback 策略切换
```

### 5.5 执行校验

每个节点和节点内部模型角色执行前必须校验：

- API Key 属于当前用户。
- API Key 状态可用。
- API Key 对应分组可用。
- 节点或模型角色的模型在分组允许范围内。
- 用户余额、套餐、限速、并发限制通过。
- 每一次模型调用成本能够写入 UsageLog 或建立对账关联。

每一次实际模型调用都应记录：

```text
run_id
node_id
node_role
capability
api_key_id
group_id
model
usage_log_id
cost
status
```

## 6. 应用配置与内部 Manifest 设计

“配置与节点”更准确地说应该叫“应用编排”或“流程配置”。它不是要求管理员写 JSON，而是让管理员通过后台界面定义一个应用怎么被用户运行。

推荐的后台配置方式：

```text
基础信息
  应用名称、简介、图标、分类、可见范围

发布形态
  提示词应用、工作流应用、智能体应用、外部托管应用

输入项
  文本框、下拉框、图片上传、文件上传、音频上传、视频上传、数量和大小限制

节点编排
  每个节点代表一个工作步骤，例如文档解析、检索、HTTP 工具调用、条件判断、生成图片、生成视频、归档结果

智能体设置
  目标描述、系统提示词、可用工具、最大步数、记忆权限、失败兜底策略

外部托管设置
  Worker 地址、鉴权方式、回调地址、超时时间、健康检查、输入输出协议

模型绑定
  为节点或节点内角色选择默认模型和所需能力，例如 text / image / video

发布设置
  版本说明、资源限制、保留周期、是否立即发布
```

内部 Manifest 是应用发布和运行的核心协议，主要服务于后端校验、版本快照、热加载、导入导出和审计。第一版后台应优先做表单和节点列表；JSON 编辑器只作为高级模式，不作为普通发布的必填入口。

### 6.0 代码型智能体/工作流定稿架构

前面几个概念容易混在一起，这里给出最终定稿：

```text
Sub2API 主系统
  负责用户、API Key、分组、模型网关、计费、UsageLog、应用中心、运行记录、对象存储引用。

Agent Worker 包
  负责某个智能体或工作流的真实业务代码。
  可以是 Go / Node.js / Python 项目。
  以独立服务或容器部署，不加载进 Sub2API 主进程。

应用版本
  Sub2API 里保存的发布记录。
  记录这个应用版本调用哪个 Worker、需要哪些能力、输入输出是什么、结果怎么保存。
```

也就是说，真正的代码发布不是“管理员在后台上传代码”，而是：

```text
开发者写 Agent Worker 代码
  ↓
构建 Docker 镜像或部署成独立服务
  ↓
在 Sub2API 管理后台注册这个 Worker 版本
  ↓
Sub2API 做健康检查和试运行
  ↓
管理员点击发布
  ↓
Sub2API 把 published_version_id 切到新版本
```

#### 6.0.1 智能体代码放在哪里

推荐两种方式：

```text
方式 A：同仓库多目录，适合第一版
  sub2api-owner/
    backend/
    frontend/
    workers/
      research-agent/
      video-workflow/
      document-agent/

方式 B：独立仓库，适合后期
  sub2api-agent-workers/
    research-agent/
    video-workflow/
    document-agent/
```

每个代码型智能体/工作流至少包含：

```text
worker.yaml       声明名称、版本、能力、输入输出、模型角色
src/              真实业务代码
Dockerfile        构建镜像
README.md         开发和部署说明
```

示例：

```text
workers/video-workflow/
  worker.yaml
  Dockerfile
  src/
    main.ts
    handlers/
      run.ts
      callbacks.ts
```

#### 6.0.2 Worker 和 Sub2API 怎么联系

Sub2API 调 Worker，Worker 再回调 Sub2API。

```text
用户运行应用
  ↓
Sub2API 校验用户 api_key_id、余额、分组和限速
  ↓
Sub2API 创建 agent_run
  ↓
Sub2API 内部 Runner 领取任务
  ↓
Runner 调用对应 Agent Worker 的 /runs
  ↓
Worker 执行业务逻辑
  ↓
Worker 需要模型时调用 Sub2API 模型代理，不直接拿用户 API Key
  ↓
Worker 回调 Sub2API 上报进度、节点结果和产物
  ↓
Sub2API 写 UsageLog、对象存储引用和最终运行状态
```

Worker 接口最小协议：

```text
GET /health
  健康检查。

POST /runs
  接收运行任务。

POST /cancel
  可选，取消运行。
```

Sub2API 给 Worker 的运行请求只包含：

```text
run_id
app_id
app_version_id
input
input_assets
callback_url
model_proxy_url
artifact_upload_url 或 artifact_upload_policy
run_token
timeout_seconds
```

不能传：

```text
用户 API Key 明文
数据库账号
对象存储永久密钥
平台主服务内部密钥
```

#### 6.0.3 Worker 怎么调用模型

Worker 不直接调用上游模型，也不持有用户 API Key。它调用 Sub2API 暴露的运行期模型代理。

```text
Worker -> Sub2API Model Proxy -> 现有模型 Gateway -> 上游模型
```

Worker 请求示例语义：

```text
run_token
node_id
node_role
capability = text / image / video
model = 管理员默认模型或运行时允许的模型
messages / prompt / files
```

Sub2API 在模型代理层完成：

- 校验 run_token。
- 找到 run 对应的用户和 `api_key_id`。
- 校验 API Key、分组、模型权限、余额、限速。
- 调用现有 Gateway。
- 写 UsageLog，并关联 `app_id/run_id/node_id/node_role`。

这样 Worker 有代码能力，但没有绕过 Sub2API 的计费和权限。

#### 6.0.4 怎么做到不停机更新

不停机更新靠“Worker 版本化 + Sub2API 发布指针切换”，不是靠主进程热加载代码。

```text
旧版本
  app_version_id = v1
  worker_url = https://worker.example.com/video/v1
  正在运行的任务继续用 v1

新版本
  app_version_id = v2
  worker_url = https://worker.example.com/video/v2
  先部署、健康检查、试运行
  通过后切换 published_version_id 到 v2
```

发布规则：

- 新运行使用最新已发布版本。
- 已经开始的运行继续绑定创建时的版本。
- v1 Worker 保留一段时间，直到 v1 的运行全部结束或超时。
- v2 异常时，管理员可以把 published_version_id 回滚到 v1。
- 下架只阻止新运行，不杀掉已有运行。

部署方式可以是：

```text
同一台服务器 Docker Compose
  app-worker-video-v1
  app-worker-video-v2
  Nginx 按路径或端口转发

另一台服务器
  v1 和 v2 两个容器并存
  Sub2API 配置不同 worker_url

K8s / 云容器
  用镜像 tag 或 digest 发布
  通过 service/canary/blue-green 切换
```

第一版最简单实现：

```text
Worker 新版本部署成新容器
  app-worker-video-v2

Sub2API 新增 agent_app_versions 记录
  version = 2
  worker_url = http://app-worker-video-v2:8080

点击发布
  agent_apps.published_version_id = 2
```

这就能做到 Sub2API 主服务不停机发布新的智能体/工作流代码。

#### 6.0.5 Worker 推荐架构

Worker 不建议做成“一个应用一个临时脚本”。推荐做成可扩展的 Worker Runtime：

```text
Agent Worker Service
  HTTP Adapter
  Auth & Signature
  Run Dispatcher
  App Registry
  Execution Engine
  Sub2API SDK
  Concurrency Controller
  Temp File Manager
  Observability

App Packages
  video-workflow
  document-agent
  research-agent
  image-batch-workflow
```

分层职责：

```text
HTTP Adapter
  提供 /health、/runs、/cancel。
  只负责接收请求、验签、返回 accepted，不承载具体业务逻辑。

Run Dispatcher
  根据 app_slug / app_version_id / worker_route 找到对应应用包。
  负责幂等检查、排队、取消和状态流转。

App Registry
  注册不同智能体和工作流。
  新增应用时只新增 handler，不改 Worker Runtime 主流程。

Execution Engine
  执行具体 Agent 或 Workflow。
  支持普通函数、DAG 工作流、Agent 循环、外部平台适配。

Sub2API SDK
  封装 model_proxy、callback、artifact upload、input asset download、event emit。
  业务代码不直接拼 Sub2API HTTP 细节。

Concurrency Controller
  控制全局并发、应用并发、单用户并发、长任务并发。
  避免一个视频任务拖垮整台 Worker。

Temp File Manager
  管理临时文件目录、大小限制、过期清理。

Observability
  记录 run_id、app_version_id、节点耗时、错误、回调状态和资源使用。
```

推荐目录结构：

```text
workers/app-worker/
  worker.yaml
  Dockerfile
  src/
    main.ts
    runtime/
      server.ts
      auth.ts
      dispatcher.ts
      registry.ts
      concurrency.ts
      temp-files.ts
    sdk/
      sub2-client.ts
      model-proxy.ts
      artifacts.ts
      callbacks.ts
      events.ts
    apps/
      video-workflow/
        manifest.ts
        handler.ts
        nodes.ts
      document-agent/
        manifest.ts
        handler.ts
        tools.ts
      research-agent/
        manifest.ts
        handler.ts
        tools.ts
```

业务应用包统一导出：

```text
manifest
  app_slug
  supported_versions
  input_schema
  output_schema
  capabilities
  default_resource_limits

handler
  async run(ctx)
```

`ctx` 是 Worker SDK 提供给业务代码的运行上下文：

```text
ctx.runId
ctx.appVersionId
ctx.input
ctx.assets
ctx.model.call(...)
ctx.artifacts.upload(...)
ctx.events.emit(...)
ctx.callbacks.progress(...)
ctx.temp.createFile(...)
ctx.cancelSignal
ctx.logger
```

不同类型的应用都可以落在同一个抽象上：

```text
简单任务
  handler 里直接执行一段函数。

工作流
  handler 调用 DAG 执行器，按节点依赖运行。

智能体
  handler 调用 Agent Loop，模型根据工具结果决定下一步。

外部平台适配
  handler 调 Coze/Dify/自有引擎，再把结果按 Sub2API 协议回传。
```

推荐的扩展策略：

```text
第一版
  一个 app-worker-general 承载多个轻量应用包。
  用 App Registry 区分不同 app_slug。

任务变重后
  按领域拆 Worker：
    app-worker-text
    app-worker-document
    app-worker-image
    app-worker-video

超重任务
  单独部署 GPU Worker 或第三方渲染服务适配器。
```

推荐的运行模式：

```text
POST /runs
  Worker 验签和幂等检查。
  如果任务可接收，立即返回 202 accepted。
  实际执行放入 Worker 本地队列或执行池。
  进度和最终结果通过 callback_url 回传 Sub2API。
```

不要让 `/runs` 长时间阻塞等待视频、文档、批处理等任务完成。

并发控制建议：

```text
global_max_concurrency        整个 Worker 最大并发
app_max_concurrency           单个应用最大并发
heavy_task_max_concurrency    视频/转码/批处理等重任务并发
queue_limit                   本地等待队列上限
run_timeout_seconds           单次运行超时
heartbeat_interval_seconds    长任务心跳
```

版本和扩展边界：

- Worker Runtime 尽量稳定，少改。
- 新智能体/工作流优先新增 `apps/*/handler`。
- 通用能力沉淀到 `sdk` 或 `runtime`。
- 不在运行时动态执行管理员上传的任意代码。
- 新版本通过镜像、容器、Host 路由和 `published_version_id` 切换。

这样设计后，Worker 可以同时支持：

- 固定工作流。
- 目标驱动智能体。
- 文档解析和 RAG。
- 生图、生视频、音频处理。
- 批处理任务。
- Coze/Dify 等外部编排平台适配。

### 6.1 智能体和工作流的开发方式

这里要把“开发”和“发布”拆开：

```text
开发 = 产生一个可运行能力
发布 = 把这个能力包装成平台应用，配置输入、权限、版本和可见范围，然后上架给用户
```

管理员不一定写代码。平台应该同时支持三种开发方式：

```text
1. 配置式开发
   使用平台已有节点、模型、工具和表单配置出应用。
   适合提示词应用、固定工作流、轻量智能体。
   普通管理员主要使用这种方式，不需要写代码。

2. 外部 Worker 开发
   开发者用 Go/Node/Python 等语言写一个独立服务。
   这个服务实现平台约定的 HTTP/Webhook 协议，负责复杂逻辑、长任务或专用工作流。
   管理员在发布台填写 Worker 地址、鉴权、回调、超时、输入输出协议后发布。
   主服务不需要停机，也不需要把第三方代码加载到主进程。

3. 平台内置节点开发
   开发者在 Sub2API 后端代码里新增一种通用节点或工具。
   例如通用文档解析节点、检索节点、图片处理节点、视频转码节点。
   这种方式需要正常发版，但发版后管理员可以反复用它配置很多应用。
```

因此，管理员“开发智能体和工作流”在产品里通常是：

```text
选择已有能力
  ↓
配置输入表单
  ↓
配置节点、工具、目标、模型角色
  ↓
配置权限、对象存储、保留策略
  ↓
试运行
  ↓
发布版本
```

而真正需要代码的部分，应由开发者提前做成“可配置能力”：

```text
复杂业务逻辑 -> 外部 Worker
通用平台能力 -> 内置节点
简单流程编排 -> 管理员配置
```

智能体和工作流的区别也体现在开发方式上：

```text
工作流
  管理员配置固定节点、分支、依赖和失败重试。
  节点可以是内置节点，也可以调用外部 Worker。

智能体
  管理员配置目标、系统提示词、可用工具、最大步数、模型角色和权限边界。
  工具本身可以是内置工具，也可以是外部服务。
```

不要把“管理员发布应用”设计成“管理员上传任意代码”。这会带来安全、隔离、依赖、资源和审计问题。代码型扩展应优先通过外部 Worker 接入；平台主服务只保存配置、调度运行、鉴权计费和归档结果。

### 6.2 Worker 是什么，部署在哪里

Worker 本质上就是一个独立运行的服务或进程，用来执行耗时、复杂、可独立扩展的任务。它不是用户 API Key，也不是模型本身。

本方案里要区分两类 Worker：

```text
平台内部运行 Worker
  属于 Sub2API 应用中心的一部分。
  负责消费运行队列、执行内置节点、调用平台 Gateway、写 UsageLog、归档对象存储。
  可以和 Sub2API 主服务部署在同一台服务器，也可以拆成单独进程或容器。

外部应用 Worker
  属于某个复杂应用或一组复杂应用。
  由开发者用 Go / Node.js / Python 等语言实现。
  通过 HTTP/Webhook 接收平台任务，回调进度和结果。
  不加载进 Sub2API 主进程，因此新增或删除外部 Worker 不需要主服务停机。
```

推荐部署方式：

```text
MVP / 轻量任务
  部署在当前香港服务器上。
  用 Docker Compose 拆成多个容器：
    sub2api-web
    sub2api-runner
    app-worker-video
    sub2api-redis
    worker-redis 可选
  如果 Worker 只给平台内部调用，可以走 Docker 内网地址。

中等任务
  部署在另一台香港或附近地区服务器。
  通过 HTTPS 暴露 Worker API。
  平台用签名请求或 run-scoped token 调用 Worker。

重任务 / 生视频 / GPU
  部署在专门的 GPU 服务器、云容器、K8s、云函数或第三方渲染服务旁边。
  平台只负责提交任务、鉴权计费、进度回调和对象存储归档。
```

对于你当前的香港服务器，建议第一版这样落地：

```text
同一台香港服务器
  Sub2API 主服务
  Sub2API Redis / 队列
  App Center 内部运行 Worker
  轻量外部 Worker
  Worker Redis 可选，且应作为 Worker 私有依赖

对象存储
  腾讯云 COS 香港 ap-hongkong

重型任务
  后续再拆到单独机器或云服务
```

如果使用另一台裸服务器部署外部 Worker，裸机本身可以，但建议容器化运行，避免依赖污染和升级困难。

另一台 Worker 服务器需要部署：

```text
基础环境
  Linux 系统、Docker / Docker Compose、时区、NTP、基础防火墙。

Worker 程序
  app-worker-general 或 app-worker-video 等应用 Worker。
  由开发者实现具体业务逻辑。

运行时依赖
  按 Worker 技术栈安装 Node.js / Python / Go 二进制 / ffmpeg / 浏览器内核 / 图片视频处理库等。
  如果用 Docker，依赖应尽量打进镜像。

Worker 私有中间件
  如果 Worker 需要异步队列、任务状态、限流或缓存，可以单独部署 Worker Redis。
  Worker Redis 属于 Worker 项目内部依赖，不和 Sub2API Redis 共用。
  Sub2API 不直接读写 Worker Redis，Worker 也不直接读写 Sub2API Redis。

网络入口
  Nginx / Caddy / Traefik 反向代理。
  HTTPS 证书。
  只暴露 /health 和 /runs 等必要接口。

安全配置
  平台调用 Worker 的签名密钥。
  Worker 回调平台的一次性运行令牌。
  IP 白名单或内网隧道。
  请求体大小、超时、并发限制。

日志与监控
  Worker 日志、错误日志、任务耗时、成功率、磁盘和内存监控。

临时目录
  用于视频、图片、文档等中间文件处理。
  必须有定时清理策略。
```

另一台 Worker 服务器通常不需要部署：

- Sub2API 主服务。
- 主数据库。
- 用户 API Key 数据。
- 对象存储本体。
- Sub2API Redis。

它只需要能访问：

- Sub2API 平台回调地址。
- 平台提供的模型代理或 Gateway Adapter。
- 对象存储的短期上传/下载地址，或平台文件代理。

推荐拓扑：

```text
主服务器
  Sub2API Web/API
  App Center 内部 Runner
  Sub2API Redis / 平台队列
  数据库

Worker 服务器
  Nginx/Caddy
  app-worker-video
  Worker Redis 可选
  临时文件目录

对象存储
  腾讯云 COS 香港
```

平台调用 Worker 时不要传数据库账号、用户 API Key 明文或对象存储永久密钥。只传运行 ID、输入文件引用、回调地址和短期令牌。

Redis 拆分原则：

```text
Sub2API Redis
  属于平台侧。
  用于 Sub2API 缓存、平台运行队列、Runner 租约、短期事件等。

Worker Redis
  属于 Worker 侧。
  只在 Worker 需要本地异步队列、执行状态、去重、限流时部署。
  可以和 Worker 放在同一台服务器或同一个 Docker Compose 内。

两者关系
  不共享。
  不互相直连。
  不作为 Sub2API 与 Worker 的通信协议。
  Sub2API 与 Worker 只通过 HTTP + JSON + 签名 + callback 连接。
```

不绑定域名也可以部署 Worker：

```text
开发 / 测试 / 临时环境
  可以直接使用 http://服务器公网IP:端口。
  例如 http://1.2.3.4:8080/runs。

两台服务器有内网互通
  优先使用内网 IP。
  例如 http://10.0.0.8:8080/runs。

生产环境
  更推荐绑定域名并启用 HTTPS。
  例如 https://worker.example.com/runs。
```

如果不绑定域名，必须额外注意：

- 防火墙只开放 Worker 必要端口，最好只允许 Sub2API 主服务器 IP 访问。
- 请求必须有签名或一次性运行令牌，不能只靠“隐藏端口”保护。
- HTTP 明文只适合内网或测试环境；公网生产建议 HTTPS。
- 使用公网 IP 时，服务器换 IP 会导致 Sub2API 后台配置失效。
- 如果需要浏览器直接访问 Worker，HTTPS/证书问题会更明显；但本方案里 Worker 主要由 Sub2API 后端调用，浏览器不直接访问。

是否需要新建项目：

```text
需要。

外部 Worker 本质上就是一个独立后端服务项目。
它不是重新部署一套 Sub2API，也不是在管理后台写一段代码。
```

推荐两种组织方式：

```text
方式 A：放在当前仓库里
  sub2api-owner/
    backend/
    frontend/
    workers/
      app-worker-video/
      app-worker-document/

适合第一版，代码集中，部署简单。

方式 B：单独一个仓库
  sub2api-app-workers/
    app-worker-video/
    app-worker-document/

适合后期团队拆分、独立发版、独立扩容。
```

一个最小 Worker 项目需要包含：

```text
HTTP 服务
  GET  /health
  POST /runs

业务代码
  根据 app_slug / app_version_id / run_id 执行具体任务。

回调代码
  调用 Sub2API 的 callback_url 上报 running / progress / succeeded / failed。

Dockerfile
  把 Worker 和依赖打成镜像。

环境变量
  WORKER_SECRET
  SUB2API_BASE_URL
  TEMP_DIR
  MAX_CONCURRENCY

日志
  输出任务 ID、耗时、错误、回调状态。
```

裸服务器落地步骤：

```text
1. 开发一个 Worker 项目
   例如 app-worker-video。

2. 写好 /health 和 /runs 接口
   /health 给平台检查存活。
   /runs 接收平台提交的任务。

3. 打 Docker 镜像
   把 Node/Python/Go 程序和 ffmpeg 等依赖打进去。

4. 在裸服务器安装 Docker / Docker Compose

5. 用 docker compose up -d 启动 Worker

6. 用 Nginx / Caddy 暴露 HTTPS 地址
   例如 https://worker.example.com/runs

7. 在 Sub2API 管理后台配置 Worker 地址、签名密钥、回调策略

8. 点击健康检查和试运行

9. 通过后发布应用版本
```

外部 Worker 的最小协议：

```text
POST /runs
  平台提交任务，携带 run_id、app_version_id、输入引用、回调地址和一次性运行令牌。

GET /health
  平台发布前和运行前检查 Worker 是否可用。

POST callback_url
  Worker 回调进度、节点状态、产物引用和最终状态。
```

外部 Worker 必须遵守：

- 不能拿到用户 API Key 明文。
- 需要模型调用时，通过平台提供的模型代理或 Gateway Adapter。
- 需要读取输入文件时，通过平台发放的短期对象引用或平台文件代理。
- 产物要回传给平台，由平台写入对象存储和数据库引用。
- 回调必须可重试、幂等，避免重复扣费或重复写产物。
- 每个请求都要有签名、运行令牌、超时和大小限制。

一个 Worker 不一定只服务一个应用。可以先做一个通用 `app-worker` 服务，内部按 `app_slug` 或 `app_version_id` 分发到不同处理逻辑；等任务变重后，再把视频、图片、文档等 Worker 拆开。

### 6.3 并发调用设计

应用中心必须按并发系统设计，不能把一次运行理解成一个同步 HTTP 请求。

并发分四层控制：

```text
运行级并发
  多个用户同时运行多个应用。
  后端创建 agent_run 后进入队列，由内部 Runner/Worker 池异步消费。

节点级并发
  同一个工作流内没有依赖关系的节点可以并行执行。
  通过 DAG 调度和 max_parallel_nodes 控制。

模型级并发
  每一次模型调用仍走 Sub2API Gateway。
  继续复用用户 API Key、分组、限速、余额、UsageLog。

外部 Worker 并发
  平台不能无限制打到某个 Worker。
  每个外部 Worker 要配置 max_concurrency、timeout、queue_limit、retry_policy。
```

关键机制：

```text
队列
  当前落地使用 Sub2API Redis Stream。
  每个 run 创建后 XADD 到 agent:runs:stream。
  Runner 使用 consumer group agent-runners 消费。
  Redis 不可用时，创建 run 会降级为进程内 goroutine 派发，保证开发环境可用。

Worker 池
  多个 sub2api-runner 实例并发消费 Sub2API 平台队列。
  支持水平扩容。

租约和心跳
  当前基础版依赖 Redis Stream pending entries。
  Runner 崩溃后，pending 超过最小空闲时间会通过 XPENDING + XCLAIM 捞回，兼容 Redis 5.0+。
  后续增强版再补显式 heartbeat、attempt、retry_policy 和可观测事件。

Worker Host 派发并发
  当前基础版优先用 Redis sorted set 做分布式派发槽位。
  Redis 异常时降级为本机内存 semaphore。
  Worker 实际执行并发仍由 Worker Runtime 自己控制。

取消传播
  用户在 Sub2API 取消 run 时，平台先把自己的 agent_runs 状态改为 canceled。
  如果 run 仍处于 running，Sub2API 再读取平台 Redis 中短期保存的 run_token，用 HMAC 签名请求调用 Worker Host 的 cancel_path，默认 /cancel。
  Sub2API 同时写入 Redis cancel flag。
  取消后新的 Model Proxy 请求会被拒绝，已经发出去且尚未完成的 Model Proxy 请求不主动打断，等待它自然返回。
  Worker 收到取消请求后记录 cancel_requested；具体 handler 应在每个节点、每次模型调用后检查取消状态，避免继续执行后续节点。
  当前基础版不把明文 run_token 写入 DB，只在 Sub2API Redis 中短期保存；Sub2API Redis 不和 Worker Redis 共用。
  即使 Redis 不可用，DB 状态变成 canceled 后，后续新的 Model Proxy 请求也会因为 run 已终态而被拒绝。
  如果第三方模型服务已经实际收到请求，本次请求可能自然完成并产生上游费用；平台侧保证的是取消点之后不再继续发起后续模型调用。

幂等
  run_id + node_id + attempt 作为幂等键。
  回调和产物写入必须可重试，不重复扣费，不重复写最终结果。

限流
  app/user/api_key/group/model/worker 多维度限流。

背压
  队列积压、Worker 满载、对象存储异常时，新运行应进入排队或直接提示稍后重试。
```

推荐 MVP 并发参数：

```text
agent_run_worker_count = 2-4
agent_run_max_parallel_nodes = 2
app_default_max_concurrency = 10
user_default_max_concurrency = 2
external_worker_default_max_concurrency = 2
external_worker_default_timeout_seconds = 1800
```

以后压力上来后，再按应用类型拆 Worker：

```text
text-runner      文本、轻量工作流
image-runner     生图、图片处理
video-worker     生视频、转码、长任务
document-worker  文档解析、RAG 构建
```

### 6.4 Coze/Dify 这类平台是否二开

Coze、Dify 这类开源项目可以用，但不建议一开始把它们作为 Sub2API 应用中心的核心代码二开。

原因是你的核心系统不是单纯 Agent 平台，而是：

```text
用户 API Key 选择
平台网关计费
分组和限速
余额和 UsageLog
对象存储归档
应用中心上架
运行记录和权限
多租户隔离
并发队列和 Worker 调度
```

这些能力都和现有 Sub2API 强绑定。如果直接深度二开 Coze/Dify，后续要改用户体系、Key 体系、模型网关、计费、产物存储、权限和 UI 风格，成本会很高。

推荐策略：

```text
主线自建
  自己实现轻量应用中心、运行队列、Runner、对象存储、UsageLog 关联。
  保证和 Sub2API 的用户、Key、分组、计费完全一致。

外部编排平台可接入
  Coze/Dify 可以作为外部应用 Worker 或外部编排引擎。
  管理员在应用中心注册一个 Coze/Dify 应用地址。
  平台提交任务，Coze/Dify 执行，结果回调或由平台拉取。

不建议深度 fork 作为主工程
  除非你决定把产品重心迁移成完整低代码 Agent 平台，并接受长期维护它的复杂度。
```

也就是说，第一阶段不要“二开 Coze 替代应用中心”，而是：

```text
Sub2API 应用中心 = 主入口、权限、Key、计费、运行记录、结果管理
Coze/Dify = 可选的外部编排后端
自研 Worker = 复杂业务能力的默认接入方式
```

这样既保留未来接入 Coze/Dify 的空间，又不会让应用中心被大型第三方平台架构绑死。

### 6.5 管理员发布方式

管理员发布时应走“发布向导”或“发布台”，而不是直接面对一个固定的图文模板。

推荐流程：

```text
1. 选择发布形态
   提示词应用 / 工作流应用 / 智能体应用 / 外部托管应用

2. 填写基础信息
   名称、说明、分类、图标、可见范围、版本说明

3. 定义输入输出
   输入可以是文本、图片、文件、音频、视频；输出可以是文本、图片、视频、文件、结构化 JSON

4. 配置运行方式
   提示词应用配置 prompt 和默认模型
   工作流应用配置节点、分支、依赖、重试
   智能体应用配置目标、工具、最大步数、记忆和权限
   外部托管应用配置 Worker/Webhook、签名、回调和健康检查

5. 声明能力需求
   只声明 text/image/video/audio/file/tool 等能力和默认模型，不选择用户 API Key

6. 配置结果策略
   结果类型、对象存储路径、是否允许用户主动归档/删除、是否计入空间额度

7. 校验与试运行
   后端校验输入输出、节点、能力、模型、对象存储、外部服务可达性
   试运行使用管理员自己的平台 API Key 或平台测试 Key，不消耗任何用户 Key

8. 发布版本
   通过校验后生成版本快照，原子切换 published_version_id
```

发布后：

- 新用户运行使用最新已发布版本。
- 已经开始的运行继续使用创建时的版本快照。
- 管理员可以下架、回滚到旧版本、复制旧版本为新草稿。
- 删除应用只做软删除，历史运行、用量和对象引用默认长期保留；后续仅允许用户主动删除/归档，或在明确开启空间治理策略后处理。

### 6.6 多元化应用支持

应用中心不能只围绕“图文生成”设计。后台应把不同应用形态拆成统一的发布接口，但允许不同运行方式：

```text
提示词应用
  适合单轮或少量模型调用，例如标题生成、翻译、摘要、SQL 解释。

工作流应用
  适合固定流程，例如文件解析 -> 检索 -> 报告生成 -> PDF 导出。

智能体应用
  适合目标驱动，例如研究助手、客服排障、运营分析，运行时可根据工具结果决定下一步。

外部托管应用
  适合复杂或重资源任务，例如生视频、批量图片处理、长时间爬取、专用工作流引擎。
```

可插拔能力分三层：

```text
配置可插拔
  用已有节点、模型和工具创建新应用，不需要发版，不需要停机。

外部执行器可插拔
  新增一个 Worker/Webhook 注册到应用中心即可发布新能力，主服务不需要停机。

内置节点扩展
  如果要把新节点代码放进主服务，仍建议走正常服务发版；不要在主进程热加载任意代码。
```

因此，第一版后台至少要支持“发布形态选择 + 输入输出定义 + 运行方式配置 + 版本发布”。节点编排只是工作流应用的核心配置，不应该成为所有智能体和外部应用的唯一表达方式。

下面只是一个工作流应用的内部结构示例，用来说明版本快照和模型角色绑定。提示词、智能体、外部托管应用会有不同的运行配置字段，但发布流程保持一致。

```json
{
  "type": "workflow",
  "name": "商品海报生成器",
  "description": "根据商品信息生成卖点、海报文案和图片。",
  "input_schema": {
    "type": "object",
    "required": ["product_name", "selling_points"],
    "properties": {
      "product_name": { "type": "string", "minLength": 1, "maxLength": 100 },
      "selling_points": { "type": "string", "maxLength": 2000 },
      "style": {
        "type": "string",
        "enum": ["clean", "premium", "social"],
        "default": "clean"
      },
      "reference_images": {
        "type": "array",
        "items": {
          "type": "object",
          "required": ["file_id"],
          "properties": {
            "file_id": { "type": "string" },
            "role": {
              "type": "string",
              "enum": ["product", "style", "background"],
              "default": "product"
            }
          }
        },
        "maxItems": 4,
        "default": []
      }
    }
  },
  "key_binding": {
    "mode": "single",
    "capabilities": ["text", "image"]
  },
  "nodes": [
    {
      "id": "copy",
      "type": "model_call",
      "capability": "text",
      "model": "claude-sonnet-4",
      "prompt": "根据商品信息生成海报文案。",
      "group_policy": {
        "mode": "current_api_key"
      }
    },
    {
      "id": "image",
      "type": "image_generation",
      "capabilities": ["text", "image"],
      "depends_on": ["copy"],
      "inputs": {
        "reference_images": "{{ input.reference_images }}"
      },
      "model_bindings": {
        "prompt_rewrite": {
          "capability": "text",
          "default_model": "gpt-4.1-mini",
          "prompt": "把海报文案改写成适合生图模型的视觉提示词。"
        },
        "image_generate": {
          "capability": "image",
          "default_model": "gpt-image-1"
        }
      },
      "group_policy": {
        "mode": "current_api_key"
      }
    }
  ],
  "resource_limits": {
    "timeout_seconds": 600,
    "max_parallel_nodes": 2,
    "max_model_calls_per_run": 20,
    "max_artifact_bytes": 524288000
  }
}
```

内部 Manifest 必须支持版本化。运行记录只保存 `app_version_id`，并通过该版本读取当时的配置快照。

### 6.7 图片和文件输入

应用输入不应只支持文本。生图、生视频、文档分析、图片编辑等应用都需要图片或文件作为输入。

输入图片/文件的处理原则：

```text
用户在运行表单选择或上传图片/文件
  ↓
后端校验文件类型、大小、数量和用户权限
  ↓
上传到私有对象存储
  ↓
创建输入资产记录
  ↓
运行请求只携带 file_id / artifact_id / storage_key 引用
  ↓
节点执行时按引用读取输入资产
```

禁止把图片、视频、PDF、DOCX 等二进制内容写入数据库。数据库只保存输入资产的对象存储引用、MIME、大小、hash、所属用户和过期时间。

输入图片来源可以包括：

- 用户本次上传的图片。
- 用户历史运行产物。
- 管理员随应用发布的示例素材。
- 外部应用回调产生的中间产物。

输入资产必须和输出产物一样走权限校验，用户只能引用自己拥有或应用公开允许使用的资产。

当前落地接口：

```text
POST /api/v1/agent-input-assets
  multipart/form-data
  file
  app_id
  field_name
  asset_type = image / file / audio / video
  asset_role
  metadata

GET /api/v1/agent-input-assets
  查询当前用户已上传的输入资产。

GET /api/v1/agent-input-assets/{id}/download-url
  返回短期下载 URL。

POST /api/v1/agent-apps/{id}/runs
  input_asset_ids = [asset_id...]
```

运行创建时，Sub2API 必须校验：

- `input_asset_ids` 中的资产归属当前用户。
- 资产未删除、未过期。
- 如果资产绑定了 `app_id`，必须和当前运行应用一致。
- Worker 收到的是短期下载 URL 和对象引用，不是对象存储永久密钥。

## 7. 运行流程

```text
用户打开应用
  ↓
填写输入
  ↓
上传或选择输入图片/文件
  ↓
输入资产写入对象存储并生成引用
  ↓
选择 API Key 或能力 Key
  ↓
创建 agent_run
  ↓
异步执行
  ↓
每个节点及其内部模型角色解析模型、分组、Key
  ↓
调用现有 Gateway
  ↓
节点输出上传对象存储
  ↓
DB 写 artifact 元数据
  ↓
最终 result.json 上传对象存储
  ↓
agent_run 标记完成
```

长任务必须异步执行。生图、生视频、文档生成、外部应用调用都不应依赖一个长时间阻塞的 HTTP 请求。

运行中展示：

```text
SSE 推送实时事件
轮询作为兜底
节点完成一个展示一个
图片/视频产物生成后立即可见
```

## 8. 结果与对象存储

结果保存原则：

```text
DB = 控制面和索引
对象存储 = 完整结果和产物本体
Sub2API Redis = 平台运行中临时状态和实时推送
Worker Redis = Worker 私有队列或缓存，可选，不与 Sub2API 共用
```

数据库不保存大结果，也不保存永久公开 URL。数据库只保存对象引用和元数据。

### 8.1 存储内容

对象存储保存：

- 用户上传的输入图片、视频、文档和其他文件。
- 完整最终结果 `result.json`。
- 节点输出 `output.json`。
- 运行事件归档 `events.jsonl`。
- 图片、视频、音频、文件、压缩包。
- 大文本、Markdown、DOCX、PDF 等导出产物。

DB 保存：

- 运行状态。
- 应用和版本。
- 用户和 API Key 绑定。
- 成本摘要。
- 对象存储 provider/bucket/key。
- 文件大小、MIME、hash、过期时间。
- 列表页需要的简短 preview。

### 8.2 对象路径规范

推荐路径：

```text
agent-artifacts/{user_id}/{run_id}/result.json
agent-artifacts/{user_id}/{run_id}/events.jsonl
agent-artifacts/{user_id}/{run_id}/inputs/{file_id}.{ext}
agent-artifacts/{user_id}/{run_id}/nodes/{node_id}/output.json
agent-artifacts/{user_id}/{run_id}/nodes/{node_id}/roles/{node_role}/output.json
agent-artifacts/{user_id}/{run_id}/artifacts/{artifact_id}.png
agent-artifacts/{user_id}/{run_id}/artifacts/{artifact_id}.mp4
agent-artifacts/{user_id}/{run_id}/artifacts/{artifact_id}.pdf
```

如果后续需要分环境：

```text
{env}/agent-artifacts/{user_id}/{run_id}/...
```

### 8.3 Artifact 元数据

`agent_artifacts` 建议字段：

```text
id
run_id
node_id
node_role
user_id
artifact_type        input_image / input_file / result_json / events_jsonl / text / image / video / file
storage_provider     cos / r2 / s3 / local
storage_bucket
storage_key
mime_type
file_size
sha256
preview_text
metadata_json
expires_at
deleted_at
created_at
```

不建议字段：

```text
public_url
permanent_download_url
```

下载时由后端鉴权后生成短期签名 URL。

### 8.4 腾讯云 COS 建议

如果使用腾讯云 COS：

```text
Bucket 地域：中国香港
Bucket 权限：私有
存储类型：标准存储
资源包：中国香港和境外通用
下载方式：后端鉴权后生成临时签名 URL
```

套餐建议：

```text
刚上线、用户少：
  100GB 标准存储容量包
  100GB 外网下行流量包

有生图和文件下载：
  500GB 标准存储容量包
  500GB 外网下行流量包

有生视频：
  1TB 标准存储容量包
  1TB 或更高外网下行流量包
```

第一版不建议使用低频存储。应用结果通常会在 7-30 天内频繁查看或下载，标准存储更合适。

下载链路：

```text
用户请求下载
  ↓
后端校验当前用户是否拥有 run/artifact
  ↓
后端生成 5-15 分钟有效的 COS signed URL
  ↓
用户直接从 COS 下载
```

不要让后端长期中转大文件下载，否则会额外消耗服务器带宽。

### 8.5 保存周期

业务数据默认不做定期删除。保存策略建议拆成两类：

```text
业务数据
  agent_runs：长期保留
  usage_logs / 计费记录：按现有平台审计策略长期保留
  result.json、图片、视频、文件、输入资产：长期保留，允许用户主动删除/归档，后续可计入用户空间额度
  用户收藏结果：长期保存，但占用用户空间额度

运行事件日志
  agent_run_events：作为智能体运行审计和用户时间线长期保留，不自动硬删
  后续如果事件量很大，可以追加对象存储归档 events.jsonl，但 DB 自动删除不是默认策略

原 Sub 运维日志
  ops_error_logs / ops_system_logs / ops_alert_events：继续按原 `ops.cleanup.schedule` 和原保留天数清理

显式空间治理
  只有管理员明确开启结果过期清理时，才给产物/输入资产写 expires_at 并按策略清理
```

当前应用中心默认不清理智能体运行日志和结果。对象存储结果的基础配置为：

```yaml
agent_artifacts:
  retention_days: 0              # 用户输入资产和运行产物默认不过期；>0 才会设置 expires_at
  cleanup_expired_artifacts_enabled: false  # 默认不清理用户结果/输入资产
```

原 Sub 运维清理流程保持不变：

```text
OpsCleanupService 按 ops.cleanup.schedule 触发
  ↓
只清理 ops_error_logs / ops_system_logs / ops_alert_events / 指标等原运维数据
  ↓
agent_runs、agent_run_events、usage_logs、计费记录、用户结果引用保持不变
```

产物和输入资产属于用户业务数据，默认不自动删除。只有在明确开启 `cleanup_expired_artifacts_enabled=true` 且设置 `retention_days > 0` 后，清理任务才会扫描 `expires_at`，删除平台对象存储中由 Sub2API 上传的过期对象，并把 `agent_artifacts / agent_input_assets` 标记 `deleted_at`。Worker 登记的 external URL 不会被 Sub2API 删除。

用户删除自己的运行结果时，也应先删除对象存储产物，再软删除或隐藏运行记录。

## 9. 数据模型

建议新增表：

```text
agent_apps
agent_worker_hosts
agent_app_versions
agent_runs
agent_run_nodes
agent_run_node_calls
agent_run_events
agent_input_assets
agent_artifacts
```

### 9.1 agent_apps

```text
id
slug
name
description
category
icon
app_type              prompt / workflow / agent / external
status                draft / published / archived
visibility            public / group / private
published_version_id
created_by
created_at
updated_at
deleted_at
```

### 9.2 agent_worker_hosts

```text
id
name
description
deploy_location       same_server / private_server / public_server / cloud_container / gpu
base_url              http://10.0.0.8:8080 或 https://worker.example.com
protocol              sub2api-worker-v1 / dify / coze / custom
auth_type             hmac_run_token / internal_network_hmac
secret_ref            签名密钥引用，不直接存明文
health_path           默认 /health
run_path              默认 /runs
cancel_path           默认 /cancel，可为空
max_concurrency
timeout_seconds
status                active / disabled / unhealthy
last_health_status
last_health_checked_at
created_by
created_at
updated_at
deleted_at
```

`agent_worker_hosts` 是管理员配置 Worker Host 的地方。它保存“Worker 服务在哪里、怎么鉴权、最大并发是多少”，不绑定某一个应用版本。

### 9.3 agent_app_versions

```text
id
app_id
version
manifest_json
runtime_type          builtin / worker / external_engine
worker_host_id        绑定 agent_worker_hosts，可为空
worker_route          应用版本运行路径，可覆盖 Host 默认 run_path
worker_health_route   应用版本健康检查路径，可覆盖 Host 默认 health_path
worker_protocol       sub2api-worker-v1 / dify / coze / custom
image_ref             Docker 镜像 tag 或 digest，可选
source_ref            git commit / release tag，可选
capabilities_json     text / image / video / audio / file / tool
input_schema_json
output_schema_json
resource_limits_json
status                draft / published / retired
published_by
published_at
created_at
```

`agent_app_versions` 是 Sub2API 和智能体代码版本的绑定点。

```text
配置式应用
  runtime_type = builtin
  manifest_json 描述 prompt / workflow / agent 配置
  worker_host_id 为空

代码型应用
  runtime_type = worker
  manifest_json 描述输入输出、能力和模型角色
  worker_host_id 指向管理员配置的 Worker Host
  worker_route 指向这个应用版本的运行入口
  image_ref/source_ref 用于审计这个版本对应哪份代码

外部编排平台
  runtime_type = external_engine
  worker_protocol = dify / coze / custom
  worker_host_id 指向外部平台 Host
  worker_route 指向外部平台应用运行入口
```

发布时只切换 `agent_apps.published_version_id`。运行开始后，`agent_runs.app_version_id` 固定指向当时版本，因此新版本发布不会影响正在运行的旧任务。

### 9.4 agent_runs

```text
id
app_id
app_version_id
user_id
status                pending / running / succeeded / failed / canceled
input_ref             对象存储引用，可选
input_preview_json
key_bindings_json
output_ref            result.json 对象引用
output_preview_json
error_message
total_cost
started_at
finished_at
created_at
deleted_at
```

### 9.5 agent_input_assets

```text
id
run_id                可空，预上传资产创建时为空
user_id
app_id                可空，绑定到某个应用时填写
field_name            reference_images / document_files 等输入字段
asset_type            image / file / audio / video
asset_role            product / style / background / document 等业务角色
storage_provider
bucket
object_key
object_url
mime_type
size_bytes
sha256
metadata_json
expires_at
deleted_at
created_at
```

`agent_input_assets` 记录用户上传或从历史产物中选择的输入图片/文件。节点执行时通过 `file_id` 或该表 ID 读取对象存储，不读取数据库中的二进制内容。预上传资产先不绑定 `run_id`；运行创建时把资产引用快照写入 `agent_runs.input_summary_json`，派发 Worker 时再生成短期下载 URL。

### 9.6 agent_run_nodes

```text
id
run_id
node_id
node_type
status
model_bindings_snapshot_json
input_ref
output_ref
cost
error_message
started_at
finished_at
created_at
```

`agent_run_nodes` 记录节点整体状态。对于复合节点，实际模型调用明细记录在 `agent_run_node_calls`。

### 9.7 agent_run_node_calls

```text
id
run_id
node_id
node_role             prompt_rewrite / image_generate / caption 等
capability            text / image / video / vision 等
status
api_key_id
group_id
model
input_ref
output_ref
usage_log_id
cost
error_message
started_at
finished_at
created_at
```

`agent_run_node_calls` 是模型调用、成本统计和 UsageLog 对账的最小粒度。简单节点通常只有一条 call，复合节点可以有多条 call。

### 9.8 agent_run_events

```text
id
run_id
user_id
status
node_id
node_role
event_type            queued / dispatching / running / worker_accepted / progress / log / model_proxy / artifact / failed / succeeded / canceled / timeout
message
progress              0-1
metadata_json         仅保存短事件、引用和统计信息，不保存大内容或 API Key 明文
created_at
```

当前最小落地已经新增 `agent_run_events`，用于记录排队、派发、Worker 接收、回调进度、模型代理、产物、取消和终态事件，并通过用户端运行详情轮询展示。

事件日志量大时，可以只在 DB 保留最近事件，完整事件流归档到对象存储 `events.jsonl`。SSE 实时推送仍属于后续增强项。

### 9.9 usage_logs 关联

推荐给现有 `usage_logs` 增加：

```text
app_id
run_id
node_id
node_role
```

如果希望减少对现有 usage 表的改动，也可以新增关联表：

```text
agent_run_usage_links
  run_id
  node_id
  node_role
  usage_log_id
```

更推荐直接扩展 `usage_logs`，因为后续按应用、节点和模型角色统计成本会更简单。

## 10. API 设计

用户端：

```text
GET    /api/v1/agent-apps
GET    /api/v1/agent-apps/:slug
GET    /api/v1/agent-apps/:slug/key-options
POST   /api/v1/agent-assets
POST   /api/v1/agent-apps/:slug/runs
GET    /api/v1/agent-runs
GET    /api/v1/agent-runs/:id
GET    /api/v1/agent-runs/:id/events
GET    /api/v1/agent-runs/:id/artifacts
GET    /api/v1/agent-artifacts/:id/download
POST   /api/v1/agent-runs/:id/cancel
POST   /api/v1/agent-runs/:id/preserve
DELETE /api/v1/agent-runs/:id
```

`POST /api/v1/agent-assets` 用于在创建运行前上传输入图片/文件，返回可放入运行请求的资产引用：

```json
{
  "id": "file_1024",
  "type": "input_image",
  "mime_type": "image/png",
  "file_size": 245760,
  "storage_provider": "cos",
  "expires_at": "2026-08-05T00:00:00Z"
}
```

`GET /api/v1/agent-apps/:slug/key-options` 根据应用配置的能力需求，返回当前用户可选择的内部 API Key 列表：

```json
{
  "mode": "single",
  "capabilities": ["text", "image"],
  "keys": [
    {
      "id": 12345,
      "name": "默认 Key",
      "group_id": 8,
      "group_name": "文本与图片",
      "capabilities": ["text", "image"],
      "status": "active"
    }
  ]
}
```

该接口只返回脱敏信息，不返回 API Key 明文。

管理员端：

```text
GET    /api/v1/admin/agent-worker-hosts
POST   /api/v1/admin/agent-worker-hosts
GET    /api/v1/admin/agent-worker-hosts/:id
PUT    /api/v1/admin/agent-worker-hosts/:id
POST   /api/v1/admin/agent-worker-hosts/:id/health-check
DELETE /api/v1/admin/agent-worker-hosts/:id
GET    /api/v1/admin/agent-apps
POST   /api/v1/admin/agent-apps
GET    /api/v1/admin/agent-apps/:id
PUT    /api/v1/admin/agent-apps/:id
POST   /api/v1/admin/agent-apps/:id/versions
PUT    /api/v1/admin/agent-apps/:id/versions/:version_id
POST   /api/v1/admin/agent-apps/:id/versions/:version_id/health-check
POST   /api/v1/admin/agent-apps/:id/versions/:version_id/test-run
POST   /api/v1/admin/agent-apps/:id/publish
POST   /api/v1/admin/agent-apps/:id/rollback
POST   /api/v1/admin/agent-apps/:id/unpublish
DELETE /api/v1/admin/agent-apps/:id
GET    /api/v1/admin/agent-runs
GET    /api/v1/admin/agent-runs/:id
```

代码型智能体/工作流的发布接口语义：

```text
POST /agent-worker-hosts
  管理员创建 Worker Host，写入 base_url、协议、鉴权方式、默认路径、最大并发和超时。

POST /agent-worker-hosts/:id/health-check
  Sub2API 调用 Host 的 health_path，确认这台 Worker 服务可访问。

POST /versions
  创建一个新版本，写入 worker_host_id、worker_route、worker_protocol、image_ref、source_ref、schema 和能力声明。

POST /health-check
  Sub2API 使用 worker_host.base_url + app_version.worker_health_route，确认新代码版本可访问。

POST /test-run
  使用管理员自己的平台 API Key 或测试 Key 创建试运行。
  试运行必须完整走队列、模型代理、UsageLog、对象存储。

POST /publish
  健康检查和试运行通过后，原子切换 published_version_id。

POST /rollback
  把 published_version_id 切回某个旧版本。
```

管理员默认只看脱敏运行信息和聚合统计，不直接看用户完整输入、输出和私有文件内容。

## 11. 前端页面

用户侧：

```text
/apps
  应用中心列表

/apps/:slug
  应用详情、输入表单、费用提示、API Key 选择器

/agent-runs
  我的运行记录

/agent-runs/:id
  运行详情、节点进度、结果预览、产物下载
```

管理员侧：

```text
/admin/agent-apps
  应用列表

/admin/agent-worker-hosts
  Worker Host 列表、Base URL、鉴权、健康检查、并发和超时配置

/admin/agent-apps/new
  创建应用

/admin/agent-apps/:id
  编辑应用、输入项、节点编排、模型绑定、Worker Host 绑定、版本、发布状态
  高级模式可查看、导入或导出内部 Manifest

/admin/agent-runs
  运行审计和统计
```

第一版可以先使用“发布向导 + 表单 + 节点列表 + 模型绑定表”完成应用发布，不必立即做拖拽式工作流编辑器。JSON/Manifest 只放在高级模式，用于开发者排查、导入导出和版本对比。

运行表单的 API Key 控件必须是平台内部 Key 选择器：

```text
单 Key 模式：
  下拉选择当前用户已有 API Key。

能力绑定模式：
  text/image/video 等能力分别选择当前用户已有 API Key。
```

禁止在应用运行表单里提供“填写 API Key 明文”的输入框。

## 12. 安全与权限

必须遵守以下边界：

- 应用不能读取用户 API Key 明文。
- 应用运行表单不能接收用户填写的 API Key 明文，只能选择平台内已创建 Key 的 ID。
- 后端创建运行时必须校验 `api_key_id` 或 `key_bindings.*.api_key_id` 属于当前用户。
- 外部 Worker 不能拿到用户 API Key，只能拿到一次性运行令牌或调用平台内部模型代理。
- 对象存储 Bucket 必须私有。
- 下载必须经过后端鉴权。
- 签名 URL 有效期建议 5-15 分钟。
- 管理员默认不能查看用户完整结果和私有文件。
- 输入图片/文件上传和产物下载要校验 MIME、大小、扩展名、图片尺寸和数量。
- 输入图片建议清理 EXIF 等可能包含隐私的元数据，或在上传前明确提示用户。
- 用户只能在运行中引用自己拥有的输入资产、历史产物，或应用公开提供的示例资产。
- HTTP 节点要做 SSRF 防护，禁止访问内网地址和平台敏感地址。
- 每个应用、每个用户、每个节点都要有超时、并发、最大调用次数和最大产物大小限制。

外部应用调用建议：

```text
平台创建 run-scoped token
  ↓
外部 Worker 使用 run-scoped token 回调平台
  ↓
模型调用仍通过平台内部 Gateway Adapter
  ↓
平台写 UsageLog 和 artifact
```

不要让外部 Worker 绕过平台直接请求上游模型。

## 13. 运维与配置

新增系统配置建议：

```text
agent_apps_enabled
agent_run_worker_count
agent_run_max_parallel_nodes
agent_run_default_timeout_seconds
agent_artifact_storage_provider
agent_artifact_storage_bucket
agent_artifact_storage_endpoint
agent_artifact_storage_region
agent_artifact_storage_path_style
agent_result_retention_days_optional
agent_artifact_retention_days_optional
agent_video_retention_days_optional
agent_user_storage_quota_bytes
agent_presigned_url_ttl_seconds
```

对象存储配置应兼容 S3 协议，便于在腾讯云 COS、Cloudflare R2、MinIO、AWS S3、阿里云 OSS 之间切换。

Sub2API 应用中心对象存储统一走 S3 兼容层，第一版不为每家云写独立上传逻辑。配置优先使用 `agent_artifacts.provider + region/account_id` 自动推导 endpoint；如果厂商 endpoint 规则变化，或使用自建/小众 S3 兼容服务，则直接填写 `agent_artifacts.endpoint` 覆盖。

支持的主流 provider：

```text
provider=s3
  AWS S3 或任意 AWS SDK 原生兼容服务
  endpoint 可为空，region 建议填写实际区域，如 ap-east-1、us-east-1

provider=cos
  腾讯云 COS
  region 示例：ap-hongkong
  endpoint 自动推导：https://cos.{region}.myqcloud.com

provider=oss
  阿里云 OSS S3 兼容接口
  region 示例：cn-hongkong
  endpoint 自动推导：https://oss-{region}.aliyuncs.com

provider=obs
  华为云 OBS S3 兼容接口
  region 示例：ap-southeast-1
  endpoint 自动推导：https://obs.{region}.myhuaweicloud.com

provider=tos
  火山引擎 TOS S3 兼容接口
  region 示例：cn-hongkong
  endpoint 自动推导：https://tos-s3-{region}.volces.com

provider=r2
  Cloudflare R2
  需要 account_id
  endpoint 自动推导：https://{account_id}.r2.cloudflarestorage.com
  默认使用 path-style

provider=minio
  MinIO 或自建 S3
  必须填写 endpoint
  默认使用 path-style

provider=wasabi
  Wasabi
  endpoint 自动推导：https://s3.{region}.wasabisys.com

provider=b2
  Backblaze B2 S3 Compatible
  endpoint 自动推导：https://s3.{region}.backblazeb2.com

provider=spaces
  DigitalOcean Spaces
  endpoint 自动推导：https://{region}.digitaloceanspaces.com

provider=scaleway
  Scaleway Object Storage
  endpoint 自动推导：https://s3.{region}.scw.cloud

provider=vultr
  Vultr Object Storage
  endpoint 自动推导：https://{region}.vultrobjects.com

provider=gcs
  Google Cloud Storage S3 兼容 HMAC Key
  endpoint 自动推导：https://storage.googleapis.com
  默认使用 path-style

provider=bos
  百度智能云 BOS S3 兼容接口
  endpoint 自动推导：https://s3.{region}.bcebos.com

provider=custom
  其他 S3 兼容服务
  必须填写 endpoint
```

推荐配置字段：

```yaml
agent_artifacts:
  enabled: true
  provider: cos
  region: ap-hongkong
  account_id: ""
  endpoint: ""
  bucket: sub2api-agent-artifacts
  access_key_id: "${AGENT_ARTIFACTS_ACCESS_KEY_ID}"
  secret_access_key: "${AGENT_ARTIFACTS_SECRET_ACCESS_KEY}"
  prefix: agent-artifacts
  public_base_url: ""
  force_path_style: false
  virtual_host_style: false
  disable_checksum: true
  max_upload_bytes: 536870912
  download_url_ttl_seconds: 3600
  retention_days: 0              # 输入资产和运行产物默认不过期；>0 才写 expires_at
  cleanup_expired_artifacts_enabled: false  # 默认不清理用户结果/输入资产
```

安全要求：
- Bucket 必须私有。
- Worker 不得拿对象存储永久密钥。
- 用户下载必须先经过 Sub2API 鉴权，再生成短期签名 URL。
- DB 只保存 provider、bucket、object_key、object_url、mime、size、hash、expires_at 等引用和元数据。

## 14. 分阶段落地

### 14.0 技术选型与开发进程控制

#### 语言和框架

Sub2API 主系统继续沿用当前项目技术栈：

```text
后端
  Go
  Gin
  ent / SQL migrations
  Sub2API Redis
  PostgreSQL / SQLite 兼容现有部署

前端
  Vue 3
  TypeScript
  Vite
  现有后台表格、表单、侧边栏风格
```

不要为了应用中心重写主系统，也不要把 Coze/Dify 作为主系统底座。

Runner 推荐使用 Go：

```text
第一版
  可以先在 Sub2API 主服务进程内启动 runner goroutine。

增强版
  拆成独立二进制：
    sub2api-server
    sub2api-runner

扩容版
  多个 sub2api-runner 实例并发消费 Sub2API Redis 队列。
```

Worker 协议必须跨语言：

```text
HTTP + JSON
HMAC 签名
run_token
callback_url
model_proxy_url
artifact_upload_policy
```

#### Go 和 Python 的分工

Go 能支持这套 AI 应用中心，但更适合做平台层，而不是承担全部 AI 生态开发。

推荐分工：

```text
Go
  Sub2API 主服务
  API 网关
  权限和 API Key 校验
  模型代理
  计费和 UsageLog
  Runner 队列消费
  Worker Host 管理
  高并发、低依赖、稳定运行的内部服务

Python
  RAG
  文档解析
  图片/视频/音频处理
  AI Agent 工具链
  向量库和 Embedding 生态
  复杂模型 SDK 适配
  实验性智能体和工作流
```

当前项目的最佳选择不是二选一，而是：

```text
Sub2API 平台和 Runner：Go
AI Worker：Python 优先
少量高性能 Worker：Go 可选
接口编排型 Worker：TypeScript 可选
```

原因：

- 当前 Sub2API 后端已经是 Go/Gin，继续用 Go 做平台成本最低。
- Go 的并发、部署、内存占用和稳定性适合 Runner、队列、网关和计费。
- Python 的 AI 生态更完整，适合快速开发 RAG、图片视频、Agent 工具和模型实验。
- Worker 协议是 HTTP，所以语言可以混用，不会把平台绑死在某个语言上。

如果第一版只选一个 Worker 模板，更推荐 Python：

```text
Python + FastAPI + Pydantic
  /health
  /runs
  /cancel
  Sub2API SDK
  App Registry
  并发控制
```

如果第一版目标只是跑通最小闭环，且暂时没有 RAG、图片视频处理，也可以用 Go 写一个最小 Worker。但只要要做多元化 AI 能力，Python Worker 会更顺手。

官方 Worker 模板建议：

```text
第一模板：Python + FastAPI + Pydantic
  适合 RAG、文档处理、图片视频处理、AI 生态依赖和快速实验。

第二模板：Go + Gin/Fiber
  适合高性能、低依赖、部署简单的内部 Worker。

可选模板：TypeScript + Fastify + Zod
  适合接口编排、异步任务、类型约束、前后端同构 schema。
```

第一版推荐先做 Python Worker 模板，因为它更贴近应用中心未来的多元 AI 能力。Go Worker 模板可以随后补，用于高性能或低依赖场景。

#### 开发进程控制

不要一次性做完整平台，按闭环拆阶段，每一阶段都要可验收。

```text
P0 协议冻结
  目标：确定 Worker Host、Worker Run、Callback、Model Proxy、Artifact 的协议。
  交付：协议文档、请求响应样例、签名规则。
  验收：不写业务代码，也能用 curl/mock 跑通健康检查和签名校验。

P1 数据模型和迁移
  目标：Sub2API 能保存 Worker Host、应用版本、运行记录和对象引用。
  交付：agent_worker_hosts、agent_apps、agent_app_versions、agent_runs、agent_artifacts、agent_run_events 等表。
  验收：后台或 API 可以创建 Worker Host 和应用草稿版本。

P2 Worker Host 管理
  目标：管理员可以配置 Worker Base URL、鉴权、并发、超时和健康检查。
  交付：管理 API + 前端页面。
  验收：点击健康检查能请求 Worker /health 并保存状态。

P3 Worker Runtime 模板
  目标：提供一个可复用 Worker 项目模板。
  交付：/health、/runs、/cancel、验签、回调、并发控制、示例 text app。
  验收：Sub2API 可以调用 Worker，Worker 返回 accepted 并回调 succeeded。

P4 运行队列和 Runner
  目标：用户运行应用后不阻塞 HTTP，请求进入队列异步执行。
  已交付：Sub2API Redis Stream 队列、Runner consumer group、pending claim、Worker Host Redis 派发槽位、取消拦截和 Worker /cancel 传播。
  待增强：显式 heartbeat、attempt/retry_policy、Runner 指标和后台可视化。
  验收：并发创建多个 run，Runner 能稳定消费；Runner 异常退出后 pending run 可被重新 claim。

P5 Model Proxy 和 UsageLog
  目标：Worker 调模型必须回到 Sub2API。
  交付：运行期 model_proxy_url、run_token 校验、UsageLog 关联 app/run/node/node_role。
  验收：Worker 发起一次文本模型调用，计费记录归属到用户选择的 API Key。

P6 Artifact 和对象存储
  目标：输入文件、输出结果、事件日志和产物进入对象存储。
  已交付：输入资产上传、产物上传/登记、下载签名 URL、agent_artifacts 元数据、agent_run_events 最小时间线、产物过期清理显式开关。
  待增强：完整事件日志归档到对象存储 events.jsonl、用户主动删除/归档结果、空间额度、历史产物转输入资产。
  验收：用户可以查看运行结果、下载自己的产物，并看到本次运行的关键事件时间线。

P7 发布和回滚
  目标：新版本发布不影响旧运行。
  交付：health-check、test-run、publish、rollback。
  验收：v1 正在运行时发布 v2，新 run 走 v2，旧 run 继续走 v1。

P8 多元化应用扩展
  目标：从 text app 扩到 document/image/video/agent。
  交付：多个 App Package 示例。
  验收：新增一个应用只新增 handler/manifest，不改 Worker Runtime 主流程。
```

每个阶段的进程控制指标：

```text
功能指标
  是否能独立演示。
  是否有 API 测试或最小集成测试。
  是否能失败重试。

安全指标
  Worker 是否拿不到用户 API Key。
  请求是否有签名和 run_token。
  对象存储是否只用短期授权。

并发指标
  是否有 max_concurrency。
  是否有 queue_limit。
  是否有 timeout。
  是否有幂等键。

运维指标
  是否有健康检查。
  是否有错误日志。
  是否能回滚。
  是否能清理临时文件。
```

开发优先级：

```text
必须先做
  Worker Host
  App Version
  Run Queue
  Runner
  Model Proxy
  UsageLog
  Artifact

可以后做
  拖拽式工作流编辑器
  Coze/Dify 深度集成
  多 API Key 能力绑定
  高级 Agent 自主规划
  Marketplace 排行榜
```

### 第一阶段：MVP

目标是把闭环跑通。

范围：

- 应用中心列表。
- 管理员通过发布向导创建应用草稿、校验、试运行和发布版本。
- 第一版重点支持 prompt/workflow 应用，同时预留智能体和外部托管应用的发布形态字段。
- 单 API Key 运行。
- 节点和模型角色的默认模型配置。
- 图片/文件输入资产上传，并以对象存储引用传入运行。
- 异步运行。
- 运行记录。
- 结果和产物存对象存储。
- 运行事件展示。
- 基础 UsageLog 关联。

暂不做：

- 可视化工作流编辑器。
- 多 Key 能力绑定。
- 复杂 Agent 自主规划。
- 任意代码上传执行。
- 主服务内任意热加载第三方节点代码。

### 第二阶段：增强能力

范围：

- 能力级多 API Key 绑定。
- 生图、生视频节点。
- 外部托管应用注册、健康检查和回调签名。
- 结果收藏和用户空间额度。
- 应用分组可见。
- 事件日志归档到对象存储。
- 应用级成本统计。
- 节点失败重试。

### 第三阶段：高级平台

范围：

- Agent 自主决策。
- 外部 Worker 应用。
- 可视化工作流编辑器。
- 节点回滚和断点续跑。
- 热门应用排行。
- 应用成本排行。
- 应用级 Marketplace 能力。

## 15. 验收标准

MVP 验收标准：

- 管理员可以创建、编辑、发布、下架应用。
- 用户可以在应用中心看到已发布应用。
- 用户运行应用前必须从平台内已有 API Key 列表中选择自己的 Key。
- 运行接口只接受 `api_key_id` 或 `key_bindings.*.api_key_id`，不接受 API Key 明文。
- 前端运行表单不得出现 API Key 明文输入框。
- API Key 不属于当前用户时运行失败。
- 节点或模型角色的模型不在 API Key 分组允许范围内时运行失败。
- 至少支持一个 prompt 应用和一个多节点 workflow 应用。
- 至少支持一个包含 `prompt_rewrite` 和 `image_generate` 两个模型角色的生图复合节点。
- 生图应用支持参考图片输入，运行请求只传输入资产 ID，不传图片二进制。
- 运行过程可通过 SSE 或轮询查看。
- 最终 `result.json` 写入对象存储。
- 图片/文件产物写入对象存储。
- DB 只保存对象引用和元数据。
- 用户只能下载自己的产物。
- 下载链接为短期签名 URL。
- UsageLog 能关联到 app/run/node/node_role。
- 原 Sub 默认清理任务只处理错误日志、系统日志、告警事件和指标等运维数据。
- 智能体 `agent_run_events` 作为运行审计和用户时间线保留，不自动清理。
- 只有显式开启结果过期清理时，清理任务才按 `expires_at` 处理对象存储产物和输入资产。

## 16. 关键结论

最终方案应保持这条主线：

```text
管理员发布能力
  ↓
用户授权自己的平台 API Key
  ↓
运行时按节点和模型角色选择模型与能力分组
  ↓
模型调用继续进入现有网关和计费体系
  ↓
完整结果和大产物进入对象存储
  ↓
DB 保存状态、索引、对象引用和审计信息
```

这样可以同时满足：

- 不停机发布和下架应用。
- 支持复杂工作流和未来智能体。
- 不绕开现有 Sub2API 的核心计费和风控。
- 不让数据库承载图片、视频和大文本。
- 后续可以在 COS、R2、S3、OSS 之间切换存储实现。

## 17. 用户体验闭环补充（2026-07-10）

以下约束已经纳入实现，后续开发不得回退：

- 当前只开放 Worker 运行方式；未完成的 prompt/internal runtime 不得出现在发布选项中。
- 发布或切换为正式版本前，Sub2API 必须读取 Worker 健康清单并验证应用运行路径存在。
- 新版本默认复制当前发布版本的 Worker、输入、输出、模型和产物策略，不从空白工作流重新填写。
- 模型能力支持“必须绑定”和“按需调用”；条件节点未启用时不得强迫用户选择 Key。
- 用户结果由 `output_schema_json` 驱动，并对未知对象、数组和表格提供结构化兜底展示。
- 用户运行详情必须展示该 run 关联的模型请求、Token 和实际扣费。
- 文件上传前先完成全部本地校验，显示上传进度，失败重试复用已上传资产。
- 默认 Key 推荐优先选择满足厂商/分组要求且倍率较低的可用 Key，用户仍可手动切换。
- 停止运行的文案必须说明：后续步骤会被拦截，但已经发出的请求可能完成并计费。
- 对象存储“测试连接”必须真实完成上传、签名下载和删除，测试失败不得切换当前配置。
- 切换 provider、Endpoint、Account ID 或 Access Key ID 时必须重新输入 Secret Access Key。
- 保存对象存储凭证前必须固定 `totp.encryption_key`，解密失败必须向管理员显示明确错误。
- 对象存储厂商、凭证、上传上限、下载 TTL、保留时间和清理开关均从 DB 动态读取，无需重启。
- 私有 Bucket 中的应用图标必须通过短期签名 URL 展示，不能直接把 `cos://`、`oss://` 地址交给浏览器。
- 上游模型返回临时图片 URL 时，Worker 必须下载并重新上传对象存储，不能把临时外链作为长期结果。
- 管理员必须能够编辑、禁用和删除应用；删除应用不删除用户历史运行与使用记录。
