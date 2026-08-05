# Seedance 按次计费与 Ximei 专用接入实施方案

> 状态：已实施并完成本地自动化、构建与失败退款链路验收；`sd-2.5-mx` 因供应商当前故障，尚未完成成功成片验证
> 日期：2026-08-06
> 上游范围：仅 `apikey` 类型账号
> 接入方式：Ximei 官方 Developer API V3 专用适配，不建设通用协议配置引擎

## 1. 本期最终目的

本期只完成两个独立但需要一起验收的目标：

1. 将 Seedance 用户计费从按秒改为“不同公开模型 × 不同清晰度”的按次计费。
2. 在保持用户统一视频 API 不变的前提下，接入 Ximei 官方视频 API，并且本期只支持可乐、怀旧、芬达三个上游模型产品。

本期不建设管理员可配置的通用协议 DSL，不承诺未来任意供应商零代码接入。

## 2. 难度与工作量

整体难度：**中等，约 6.5/10**。

预计工作量：**7-12 个有效开发日**，真实视频任务和人工相关性验证另需自然等待时间。

工作量拆分：

| 模块 | 预计工作量 |
|---|---:|
| Seedance 按次计费与安全迁移 | 2-3 天 |
| Ximei 请求、查询、媒体与提示词适配 | 3-5 天 |
| 管理后台、测试、文档与灰度 | 2-4 天 |

主要风险不是 HTTP 请求本身，而是：

- 首帧、尾帧和时长需要参数与提示词共同约束。
- 三个 `provider_route` 实际代表不同上游模型/质量产品，必须由公开模型与清晰度组合确定。
- Ximei 文档缺少部分完整响应 Schema，需要真实请求确认。
- Ximei 任务失败后的退款、轮询和内容代理必须接入现有 Seedance 结算闭环。
- 计费单位切换不能把历史每秒单价静默解释成每次单价。

## 3. 明确不做的内容

本期不做：

- 通用 ProtocolProfile 数据库和映射 DSL。
- 任意供应商通过后台配置接入。
- 通用 N 次供应商 attempt 编排。
- 将 FFLink、慧取改写成配置模板。
- OAuth、Cookie、Session 或网页登录类型账号。
- Ximei 与不同质量产品之间的自动替换。

保留长期统一引擎设计文档，但不作为本期开发依赖。

## 4. 对外统一 API

用户仍然只调用 TKCreazy：

```text
POST   /v1/videos/generations
POST   /v1/videos/uploads
GET    /v1/videos/jobs
GET    /v1/videos/jobs/{job_id}
GET    /v1/videos/jobs/{job_id}/content
DELETE /v1/videos/jobs/{job_id}
```

用户请求示例保持统一：

```json
{
  "model": "sd-2.0-mx933",
  "prompt": "人物从室内走向窗边，动作自然连贯",
  "duration": 10,
  "resolution": "720p",
  "aspect_ratio": "9:16",
  "audio": true,
  "start_frame_url": "https://tkcreazy.top/v1/videos/uploads/file-a",
  "end_frame_url": "https://tkcreazy.top/v1/videos/uploads/file-b",
  "guidances": {
    "image_reference": [],
    "video_reference_base": [],
    "audio_reference": []
  }
}
```

用户不会传入或看到：

- `provider_route`
- Ximei Base URL 和 API Key
- `kele_pool`、`tc_pool`、`fenda_pool`
- Ximei 上游任务 ID
- Ximei 原始结果地址

## 5. Ximei 上游产品语义

Ximei 的 `provider_route` 不是普通网络线路，而是上游模型/质量产品选择字段。

| 内部产品 | 上游选择值 | 用户公开模型 | 清晰度 | 公开时长 |
|---|---|---|---|---|
| 可乐 | `kele_pool` | `sd-2.0-mx933` | 480p | 5/10/15 秒，省略为 5 秒 |
| 怀旧 | `tc_pool` | `sd-2.0-mx933` | 720p | 5/10/15 秒，省略为 5 秒 |
| 芬达 | `fenda_pool` | `sd-2.5-mx` | 720p | 5/10/15/30 秒，省略为 5 秒 |

本方案将用户给出的“芬达 7209”解释为“芬达 720p”。公开模型 `sd-2.5-mx` 固定使用 `fenda_pool` 和 720p，但允许 5、10、15、30 秒四档时长，省略 `duration` 时按 5 秒处理；不提供其他清晰度或时长。三个产品由 Ximei 专用能力表维护，不放入通用 FFLink/Huiqu 模型能力表。

### 5.1 公开模型映射

Ximei API Key 账号使用“公开模型 + 清晰度”的结构化产品映射。不能沿用简单的“公开模型 -> 上游模型字符串”映射，因为 `sd-2.0-mx933` 的 480p 和 720p 必须选择不同的 `provider_route`：

```json
{
  "sd-2.0-mx933": {
    "480p": "kele_pool",
    "720p": "tc_pool"
  },
  "sd-2.5-mx": {
    "720p": "fenda_pool"
  }
}
```

以上映射是内置、确定性的适配规则。用户只提交公开模型和清晰度，管理员只启用合法产品组合，不手工输入 `provider_route`。

必须在转发上游前拒绝不支持的组合，不能静默降级、升档或改换模型：

- `sd-2.0-mx933` 只支持 480p 和 720p；480p 固定走可乐，720p 固定走怀旧；`duration` 只允许 5、10、15，省略为 5。
- `sd-2.5-mx` 只支持 720p，并固定走芬达；`duration` 只允许 5、10、15、30，省略为 5。
- `sd-2.0-mx933 + 1080p`、`sd-2.5-mx + 480p` 和 `sd-2.5-mx + 1080p` 必须在平台侧返回明确的参数错误。
- `sd-2.5-mx` 填写 5、10、15、30 之外的值时，必须在平台侧返回明确的参数错误，不能静默改写。
- `sd-2.0-mx933` 与现有慧取公开模型 `sd2-mx933` 是两个精确匹配、互不合并的模型 ID；现有慧取路由保持不变。
- 本期只允许表中的三个内部产品，其他 Ximei 产品一律不进入调度候选。

## 6. Ximei API Key 账号配置

管理员创建账号时：

```text
平台：Seedance
账号类型：API Key
视频供应商：Ximei Developer API V3
Base URL：https://liantongyidong.ximeiedu.org
API Key：sk_live_*
产品组合：启用 `sd-2.0-mx933@480p`、`sd-2.0-mx933@720p`、`sd-2.5-mx@720p` 中的一个或多个
代理：可选
并发：按该上游 Key 的真实限制配置
```

后端强制校验：

- 仅允许 `account_type=apikey`。
- API Key 非空；建议校验 `sk_live_` 前缀并允许管理员确认例外。
- Base URL 使用现有安全校验，防止 SSRF。
- Ximei 产品必须来自内置允许列表。
- 每个“公开模型 + 清晰度”组合只能映射到一个明确 Ximei 产品。
- 只允许启用内置的三个合法组合，不允许管理员手工填写或覆盖 `provider_route`。
- 密钥加密保存并在日志、响应和后台展示中脱敏。

一个 API Key 账号可以同时启用三个产品组合，账号并发额度由三个产品共享，不复制成三个独立账号。

## 7. 专用适配器边界

新增 Ximei 专用适配器，建议文件边界：

```text
backend/internal/service/seedance_ximei.go
backend/internal/service/seedance_ximei_models.go
backend/internal/service/seedance_ximei_prompt.go
backend/internal/service/seedance_ximei_test.go
backend/internal/service/seedance_ximei_prompt_test.go
```

职责分离：

- `seedance_ximei_models.go`：产品能力、秒数、清晰度、比例和素材限制。
- `seedance_ximei_prompt.go`：首帧、尾帧、时长和素材角色的确定性提示词编译。
- `seedance_ximei.go`：HTTP 请求、任务 ID、状态、结果和错误解析。
- Seedance handler：只做统一请求解析、账号调度、绑定、计费和统一响应。

即使本期做专用适配，也不能把 Ximei 请求拼装继续堆进现有 `forwardSeedance` 大函数。

## 8. 请求映射

Ximei 官方创建接口：

```text
POST /api/v3/contents/generations/tasks
Authorization: Bearer sk_live_*
Content-Type: application/json
Idempotency-Key: <平台生成的稳定键>
```

统一请求转换为：

```json
{
  "model": "video",
  "provider_route": "tc_pool",
  "prompt": "<编译后的提示词>",
  "duration": 10,
  "image_urls": ["https://..."],
  "video_urls": [
    {"url": "https://...", "durationSeconds": 6}
  ],
  "audio_urls": [
    {"url": "https://...", "durationSeconds": 8}
  ],
  "aspect_ratio": "9:16",
  "generate_audio": true,
  "number_of_runs": 1
}
```

上例由用户请求中的 `model=sd-2.0-mx933` 和 `resolution=720p` 唯一解析为 `provider_route=tc_pool`。该内部字段不能由用户请求覆盖。

秒数字段的名称、类型和各产品范围必须来自已确认的产品契约并由适配器严格校验。代码和公开 Schema 必须将 `sd-2.5-mx` 固定为 `resolution=720p`，允许 `duration=5/10/15/30`，省略时补 5。可乐和怀旧继续使用公开 5/10/15 秒规则。

## 9. 首帧、尾帧和提示词编译

Ximei 没有与现有统一 API 完全等价的首尾帧结构，首尾帧角色需要通过素材顺序和提示词共同表达。

### 9.1 固定图片排序

建议顺序：

```text
@Image1 = start_frame_url（存在时）
@Image2 = end_frame_url（存在时）
后续 @ImageN = 普通 image_reference，保持用户输入顺序
```

如果只有尾帧没有首帧，统一 API 层应拒绝，避免 `@Image1` 角色歧义。

### 9.2 提示词编译规则

保留用户原始提示词，并在末尾追加平台生成的约束段：

```text
<用户原始提示词>

[Reference constraints]
- Use @Image1 as the exact opening-frame composition and subject identity.
- Use @Image2 as the ending-frame composition and final pose.
- Generate an exactly 10-second continuous video.
- Keep the transition continuous; do not swap identities or replace key objects.
```

实际中英文模板以真实效果测试为准，但必须满足：

- 用户原始提示词不被改写或删除。
- 注入内容由代码生成，不允许用户覆盖内部约束边界。
- 重试同一任务得到完全相同的编译结果。
- 编译后的提示词长度必须重新校验。
- 日志只记录脱敏摘要；完整提示词按现有审计规则处理。
- 用户已经写了 `@ImageN` 时需要检测冲突，不能产生双重角色定义。

### 9.3 时长处理

Ximei 使用正式秒数参数，必须：

1. 将统一 `duration` 传入正式秒数字段。
2. 在提示词中重复明确同一秒数。
3. 任务完成后使用 `ffprobe` 验证成品实际时长。

提示词只是增强约束，正式计费和能力校验仍使用统一请求中的秒数。

产品时长校验必须在请求调度前完成：

- `sd-2.0-mx933`：只允许 5、10、15 秒，省略时补 5。
- `sd-2.5-mx`：只允许 5、10、15、30 秒，省略时补 5；其他值直接返回参数错误，禁止静默改写。

### 9.4 能力声明

能力表应明确角色实现方式：

```text
start_frame_mode = prompt_reference
end_frame_mode = prompt_reference
duration_mode = parameter_and_prompt
```

接口文档应说明首尾帧是模型语义约束，不承诺像专用首尾帧接口一样逐像素锁定。

## 10. 音视频素材处理

Ximei 的视频和音频对象需要 `durationSeconds`。

处理规则：

- 平台上传和外部 URL：调用方都必须为每个参考视频、参考音频填写真实的 `duration_seconds`。
- `duration_seconds` 缺失、为零或负数时，在调用上游前返回明确参数错误；当前不由服务器自动探测或伪造数值。
- 素材时长、数量和总量按所选 Ximei 产品能力校验。
- 传给 Ximei 的素材 URL 必须在其任务生命周期内可访问。
- 平台对象存储签名过期时，settlement/fallback 不应复用失效 URL。

本期不新增任意文件直传 Ximei；用户文件先通过 TKCreazy 统一上传入口进入对象存储，再由 Ximei 通过 URL 获取。

## 11. 任务创建、查询与结果

Ximei 查询接口：

```text
GET /api/v3/contents/generations/tasks/{cstask_id}
Authorization: Bearer sk_live_*
```

状态映射：

| Ximei | TKCreazy |
|---|---|
| `queued` | `queued` |
| `running` | `running` |
| `succeeded` | `completed` |
| `failed` | `failed` |

成功结果读取 `content.video_url`，失败读取 `error.code` 和 `error.message`。

### 11.1 任务 ID

Ximei 上游 `cstask_*` 只在内部保存。用户看到平台生成的 `vidjob_*`。

任务绑定至少保存：

- 平台 job ID
- Ximei 上游 task ID
- 账号 ID
- 公开模型
- Ximei 产品
- 请求快照和持久化素材引用
- 任务状态
- 结算和退款状态

查询已创建任务时，即使账号后来被暂停，也必须允许使用原绑定账号查询；暂停只禁止创建新任务。

### 11.2 创建响应字段确认

当前文档没有完整创建成功 JSON。开发时必须通过真实调用确认任务 ID 的准确路径，不能同时猜测多个字段后直接上线。

### 11.3 内容代理

最终向用户返回：

```text
/v1/videos/jobs/{vidjob_id}/content
```

平台从 Ximei 结果地址流式代理或转存对象存储。响应不暴露 Ximei 域名、产品名和 API Key。

## 12. Ximei fallback 范围

本期不建设通用跨产品 fallback。

规则：

- FFLink 431 -> 慧取 933 的现有 fallback 保持不变。
- 用户明确选择的 Ximei 产品失败后，直接进入最终失败和退款流程。
- 可乐、怀旧、芬达不互相自动替换，因为模型、质量和清晰度不同。
- Ximei 不作为 FFLink/Huiqu 的隐式 fallback，除非后续有独立、明确的产品等价性方案。

这样可以避免用户选择720p产品却被静默替换成480p，或 SD2 被替换为 SD2.5。

## 13. Seedance 按次计费

### 13.1 计费目标

```text
price = video_model_prices[requested_public_model][resolution]
total_cost = price * video_count
actual_cost = total_cost * effective_multiplier
```

`duration_seconds` 不参与 Seedance 用户费用计算。

### 13.2 复用现有结构

当前已经具备：

- 模型 × 清晰度价格矩阵。
- 分组价格配置。
- 用户专属模型清晰度价格覆盖。
- Seedance 请求模型计费身份。

需要修改的是：

- Seedance 不再调用按秒公式。
- 价格表文案从 `$ / 秒` 改为 `$ / 次`。
- 价格预览不再乘秒数。
- 测试从 `单价 × 10秒` 改为 `单价 × 1次`。

Grok、LTX、HappyHorse 本期保持原有计费逻辑。

### 13.3 显式计费单位

建议增加分组级：

```text
video_billing_unit = per_second | per_request
```

仅 Seedance 允许切换到 `per_request`。不要通过平台名称或价格是否存在来隐式猜测。

### 13.4 安全迁移

现有价格是每秒价，不能自动当成每次价：

1. 数据库增加计费单位，现有分组保持 `per_second`。
2. 后台管理员填写并预览新的单次价格。
3. 确认用户专属覆盖价格。
4. 原子切换该分组到 `per_request`。
5. 新任务使用新单位；已经创建的任务使用创建时计费快照或原 usage 记录。

账号无需重配，但价格配置必须审核。

### 13.5 失败与退款

- Ximei 接受任务后只记录一次 Seedance 用量。
- 轮询和内容下载不重复计费。
- 成功任务不退款。
- 最终失败按现有幂等退款机制退款一次。
- 重复轮询和 worker 重启不能重复退款。
- Ximei 上游是否对审核失败扣积分单独记录，不改变平台用户退款幂等逻辑。

审核类失败是否向用户收费是待确认的产品政策，不能由上游扣费规则自动决定。

## 14. 管理后台改动

### 14.1 创建/编辑账号

Seedance API Key 账号增加供应商选项：

```text
FFLink
慧取
Ximei Developer API V3
```

选择 Ximei 时显示：

- Base URL
- 视频 API Key
- 模型产品映射
- 并发
- 代理
- 账号测试

三个合法的“公开模型 + 清晰度”产品组合以选项形式展示，不让管理员手输 `provider_route`。

### 14.2 账号测试

账号测试分两层：

1. 连接和鉴权检查。
2. 指定产品、秒数和最小安全提示词的真实视频测试。

仅访问公开 `/health` 不能证明 API Key、创建任务和轮询可用。

### 14.3 价格矩阵

Seedance 分组价格页面：

- 显示计费单位。
- `per_request` 模式明确显示 `$ / 次`。
- 按公开模型和清晰度编辑价格。
- 支持用户专属单次价格覆盖。
- 显示最终倍率后的单次价格。
- 切换计费单位前要求二次确认。

## 15. 后端主要改动位置

预计涉及：

- `backend/internal/service/seedance.go`
- `backend/internal/service/seedance_huiqu.go`
- `backend/internal/service/fflink_video_models.go`
- `backend/internal/service/account.go`
- `backend/internal/service/openai_gateway_usage.go`
- `backend/internal/service/billing_service.go`
- `backend/internal/service/seedance_usage.go`
- `backend/internal/handler/seedance.go`
- `backend/internal/repository/seedance_task_binding_repo.go`
- 分组、账号和用户专属价格 DTO/Repository
- Seedance settlement worker

新增 Ximei 专用服务和测试文件，但保持 Ximei HTTP 细节不进入计费、handler 和 FFLink/Huiqu 代码。

## 16. 前端主要改动位置

预计涉及：

- Seedance 供应商类型和默认 Base URL。
- 创建/编辑账号 Ximei 表单。
- Ximei 产品映射选择器。
- Seedance 价格矩阵单位和预览。
- 用户专属视频价格表单。
- 中英文错误文案和说明。

不改变用户视频生成页面的请求结构。

## 17. 实施阶段

### 阶段 1：计费双模式（2-3 天）

- 增加显式计费单位。
- 实现 Seedance 按次公式。
- 保持其他视频平台不变。
- 修改管理员和用户专属价格界面。
- 补齐迁移、缓存和 DTO 测试。

验收：同一公开模型、同一清晰度在其所有合法时长下费用完全相同；`sd-2.5-mx@720p` 的 5/10/15/30 秒每次均只收一份配置价格，不乘时长计费。

### 阶段 2：Ximei 账号与产品能力（1-2 天）

- 增加 `video_provider=ximei`。
- 增加 Base URL、Key 和三个产品组合映射。
- 实现产品能力校验。
- 保证旧账号缺少 provider 时仍默认为 FFLink。

验收：Ximei 账号只能是 API Key；一个账号可映射多个产品并共享并发。

### 阶段 3：请求与提示词适配（2-3 天）

- 实现 JSON 请求。
- 实现素材时长探测。
- 实现首帧、尾帧和时长提示词编译。
- 实现稳定幂等键。
- 增加请求快照和脱敏日志。

验收：单元测试精确断言产品、秒数、素材顺序和编译提示词；`sd-2.5-mx` 接受 5/10/15/30 秒，省略时默认补齐 5 秒，其他秒数在调度前返回参数错误。

### 阶段 4：任务查询、结果和退款（1-2 天）

- 解析创建任务 ID。
- 查询并映射状态。
- 结果代理或转存。
- 接入 settlement 和最终失败退款。
- 保持现有 FFLink -> 慧取 fallback 不变。

验收：Ximei 成功只计费一次，失败只退款一次，用户只看到平台 job ID。

### 阶段 5：真实测试与文档（2-3 天）

- 可乐和怀旧逐个测试 5/10/15 秒；芬达逐个验证 5/10/15/30 秒，并验证省略时默认为 5 秒及拒绝其他秒数。
- 测试首帧、尾帧、单图、多图、视频、音频。
- 用 `ffprobe` 校验秒数、分辨率和音轨。
- 人工检查人物、声音、首尾构图和参考素材相关性。
- 更新 OpenAPI/Apifox 和管理员配置手册。

验收：只有真实通过的字段、秒数和素材限制进入用户文档。

## 18. 测试矩阵

### 18.1 计费

- 不同模型、不同清晰度分别取单次价格。
- 同模型同清晰度下不乘秒数。
- 用户专属价格覆盖。
- Ximei 成功、失败和重复轮询。
- FFLink -> 慧取 fallback 不重复计费。
- Grok 按秒计费回归。

### 18.2 Ximei 请求

- 文生视频。
- 首帧、首尾帧。
- 单图、多图。
- 单视频、多视频。
- 单音频、多音频。
- 混合素材和产品上限。
- 可乐/怀旧的 5/10/15 秒、芬达的 5/10/15/30 秒和默认 5 秒，以及各自清晰度和比例。
- 生成声音开关。
- 幂等重试。

### 18.3 Ximei 响应

- `queued/running/succeeded/failed`。
- 任务不存在和鉴权失败。
- 上游资源不足、服务不可用和审核失败。
- 结果 URL 失效、Range 下载和内容代理。
- settlement worker 重启和重复 claim。

### 18.4 兼容回归

- 历史无 provider 的 FFLink 账号。
- 显式 FFLink 和慧取账号。
- 431 到 933 fallback。
- 933 直接失败退款。
- 原模型列表、价格矩阵和用户专属价格。
- 原 Apifox 请求示例。

## 19. 发布与回滚

建议分两次发布：

1. 发布按次计费双模式，但不立即切换现有分组。
2. 发布 Ximei 适配器并只在测试分组灰度。

灰度通过后：

- 配置 Ximei API Key 账号。
- 启用需要的公开模型与清晰度产品组合。
- 填写单次价格。
- 原子切换测试分组到按次。
- 真实运行并观察成功率、退款、延迟和上游成本。
- 再逐个切换生产分组。

回滚时：

- 禁用 Ximei 账号即可停止新任务。
- 在途 Ximei 任务仍由绑定账号完成查询和退款。
- 分组计费单位可切回旧模式，但已创建任务按原记录处理。
- FFLink、慧取路径不受 Ximei 开关影响。

## 20. 最终验收标准

1. 用户只调用统一 TKCreazy 视频 API。
2. Ximei 使用官方 `/api/v3/contents/generations/tasks` 接口。
3. Ximei 账号只支持 API Key 类型。
4. 三个合法的“公开模型 + 清晰度”组合被确定性映射到三个 `provider_route`，其他 Ximei 产品不可被选择。
5. 首帧、尾帧和时长通过确定性提示词编译及正式秒数参数映射。
6. Ximei 上游任务 ID、产品名和域名不暴露给用户。
7. Ximei 成功、失败、退款和内容代理进入现有 Seedance 闭环。
8. 现有 FFLink、慧取账号无需重新配置。
9. 原有 FFLink -> 慧取 fallback 行为不变。
10. Seedance 按“公开模型 × 清晰度”单次计费，不乘秒数。
11. 用户专属单次价格覆盖可用。
12. Grok 等其他视频平台计费无回归。
13. OpenAPI/Apifox 和管理员配置说明与真实测试一致。
14. `sd-2.5-mx` 仅允许 720p；`duration` 仅允许 5、10、15、30 秒，省略时按 5 秒处理，其他秒数在调度前被拒绝。

## 21. 开发前待确认

1. 可乐和怀旧的 5/10/15 秒需持续做生产回归；芬达固定 720p，并按已确认契约支持 5/10/15/30 秒、默认 5 秒，不再把固定 30 秒作为能力前提。
2. 首帧和尾帧的最终中英文提示词模板。
3. 用户显式使用 `@ImageN` 时是拒绝冲突还是重新编号。
4. Ximei 审核、肖像、版权类最终失败是否向用户收费。
5. Ximei 结果是直接代理还是统一转存对象存储。
6. 现有 Seedance 分组逐个切换按次，还是统一维护窗口切换。
