# Sub2API App Worker

这是 Sub2API 应用中心的独立 Worker 模板。Worker 负责运行具体智能体/工作流代码，Sub2API 负责用户、Key、计费、对象存储和应用发布配置。

## 核心约定

- Worker 单独部署，不进入 Sub2API 主进程。
- Worker 可以和 Sub2API 不在同一台服务器，也不需要绑定域名；Sub2API 能访问 `host:port` 即可。
- Worker 不接收用户明文 API Key，只接收短期 `run_token`。
- Worker 调模型必须请求 Sub2API 下发的 `model_proxy_url`，由 Sub2API 用用户运行时选择的 Key 代理调用上游模型。
- Worker 结果和文件通过 Sub2API Artifact 接口进入对象存储，数据库只保存引用。
- Worker 自己需要 Redis 时，使用 Worker 自己的 Redis；不要复用 Sub2API 的队列 Redis。
- 文本模型调用默认使用 SSE 流式 Model Proxy。Worker 会聚合最终文本，并按 `STREAM_PROGRESS_INTERVAL_SECONDS` 限频回调部分结果；默认每秒最多一次，不会按 Token 写数据库。

## 对象存储配置

对象存储配置在 Sub2API 后端，不在 Worker 项目里。Worker 不需要、也不应该持有对象存储 AccessKey。

Sub2API 会按以下顺序读取 `config.yaml`：

1. `DATA_DIR` 环境变量指向的目录
2. Docker 内 `/app/data`
3. 当前运行目录
4. 当前运行目录下的 `config`
5. `/etc/sub2api`

腾讯云 COS 香港示例：

```yaml
agent_artifacts:
  enabled: true
  provider: cos
  region: ap-hongkong
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
  retention_days: 0
  cleanup_expired_artifacts_enabled: false
```

也可以直接用环境变量覆盖配置：

```powershell
$env:AGENT_ARTIFACTS_ENABLED='true'
$env:AGENT_ARTIFACTS_PROVIDER='cos'
$env:AGENT_ARTIFACTS_REGION='ap-hongkong'
$env:AGENT_ARTIFACTS_BUCKET='sub2api-agent-artifacts'
$env:AGENT_ARTIFACTS_ACCESS_KEY_ID='你的 SecretId'
$env:AGENT_ARTIFACTS_SECRET_ACCESS_KEY='你的 SecretKey'
$env:AGENT_ARTIFACTS_PREFIX='agent-artifacts'
$env:AGENT_ARTIFACTS_MAX_UPLOAD_BYTES='536870912'
$env:AGENT_ARTIFACTS_DOWNLOAD_URL_TTL_SECONDS='3600'
$env:AGENT_ARTIFACTS_RETENTION_DAYS='0'
$env:AGENT_ARTIFACTS_CLEANUP_EXPIRED_ARTIFACTS_ENABLED='false'
```

常用 provider：

- `cos`：腾讯云 COS，`region` 填 `ap-hongkong` 时会自动推导 `https://cos.ap-hongkong.myqcloud.com`
- `oss`：阿里云 OSS S3 兼容接口
- `r2`：Cloudflare R2，需要 `account_id`
- `minio`：MinIO 或自建 S3，需要填写 `endpoint`
- `s3`：AWS S3 或原生 S3 兼容服务
- `custom`：其他 S3 兼容服务，需要填写 `endpoint`

建议 Bucket 保持私有。用户下载结果时由 Sub2API 先鉴权，再生成短期签名 URL。

## 本地启动

```powershell
cd workers/app-worker
python -m venv .venv
.\.venv\Scripts\Activate.ps1
pip install -r requirements.txt
$env:PYTHONPATH='src'
uvicorn sub2api_worker.main:app --host 0.0.0.0 --port 8091 --reload
```

在 Sub2API 管理后台登记 Worker Host：

- 服务地址：`http://127.0.0.1:8091`
- 健康检查路径：`/health`
- 默认运行路径：`/runs`
- 取消路径：`/cancel`

应用版本里的运行路径可以填：

- `/runs`：通用文本应用
- `/text/runs`：文本处理入口
- `/image/runs`：统一图片智能体入口；只传提示词时文生图，上传可选参考图时自动图生图
- `/workflow/runs`：图文工作流入口
- `/audio/runs`：语音合成、音频转写或翻译入口
- `/video/runs`：OpenAI 兼容视频生成入口
- `/grok-video/runs`：Grok 视频生成、编辑和续写入口
- `/product-marketing/runs`：AI 商品营销包工作流
- `/academic-paper/runs`：分章节生成并排版 Word 论文的工作流

## Word 论文工作流

`/academic-paper/runs` 会依次执行论文规划、分章节写作、全文一致性检查、DOCX 排版和 Artifact 上传。管理员端可直接应用“Word 论文模板”，配置两项 `gpt-5.5` 文本模型角色：`academic_paper.plan` 与 `academic_paper.write`。

论文工作流支持：

- 论文方向、类型、学科、学历、目标字数和可视化五级目录树
- 目录以 `outline_spec.version = 1` 的稳定节点 ID、标题和层级提交；模型只填写节点正文，不能改动目录
- `.docx`、`.pdf`、`.txt`、`.md`、`.csv`、`.json` 参考资料提取
- 可选严格引用证据核验：逐个正文引用匹配同编号上传全文中的原文摘录与页码/段落定位
- 可选联网文献检索：OpenAlex 与 Crossref 双源查询、DOI/题名去重、年份和开放获取筛选
- 严格模式可下载 OpenAlex/Crossref 返回的开放获取 PDF，完成全文解析和题名/DOI 身份核验后再纳入正文引用
- 可选学校或期刊 `.docx` 模板
- A3、A4、A5、B5、Letter、Legal 和自定义纸张
- 中文与西文字体、标题 1–5 级字号/行距/段距/对齐、正文缩进
- 自动目录字段、多级标题编号、封面、页眉页脚、页码、参考文献、致谢和附录
- 最终 `.docx` 通过 Sub2API Artifact 接口写入对象存储

参考资料提取上限可通过以下环境变量调整：

- `PAPER_REFERENCE_MAX_FILE_BYTES`：单个参考文件最大下载字节数
- `PAPER_REFERENCE_MAX_CHARS_PER_FILE`：单文件最多加入模型上下文的字符数
- `PAPER_REFERENCE_MAX_TOTAL_CHARS`：一次运行全部参考资料的最大上下文字符数
- `PAPER_EVIDENCE_MIN_QUOTE_CHARS`：逐字证据摘录的最小实质字符数
- `PAPER_EVIDENCE_AUDIT_BATCH_SIZE`：单次模型证据审计的最大引用 occurrence 数
- `PAPER_EVIDENCE_CHUNKS_PER_OCCURRENCE`：每个引用送审的候选全文块数量
- `PAPER_EVIDENCE_MAX_PROMPT_CHARS`：单批证据块允许进入审计提示词的最大字符数
- `PAPER_LITERATURE_TIMEOUT_SECONDS`：单次文献 API 或开放全文请求超时
- `PAPER_LITERATURE_MAX_RESULTS`：一次运行最多检索并处理的文献数量，最大 20
- `PAPER_LITERATURE_MAX_PDF_BYTES`：单个联网开放 PDF 的最大下载字节数
- `PAPER_LITERATURE_MAILTO`：OpenAlex/Crossref 礼貌池联系邮箱，生产环境建议填写
- `PAPER_LITERATURE_USER_AGENT`：文献服务请求标识；留空时使用带 Worker 版本号的默认标识
- `PAPER_LITERATURE_ALLOW_PROXY_FAKE_IP`：仅在 Clash 等代理使用 `198.18.0.0/15` Fake-IP 时开启

模型只允许使用上传资料中能够明确识别出处的引用。资料不足时不会承诺参考文献、实验数据、访谈或调查结果真实存在。

启用 `citation_evidence_enabled=true` 时还必须填写 `reference_bibliography`，每行使用连续的 `[1]`、`[2]` 编号，并上传对应全文。单独文件应以 `[1]`、`[2]` 开头命名；合并文本也可以使用 `Source [n]` 或 `来源 [n]` 分段。Worker 在最终正文完成后为每个 `[[CITE:n]]` 建立 occurrence，要求审计模型返回同编号全文块中的逐字摘录，再由代码校验 chunk、编号、摘录长度和原文子串。缺失、错配、伪造或漏审在一次安全引用位置修复后仍不通过时，不生成 Word 文件。

该报告表示引用已与用户上传来源逐字核对，不代表 Worker 已联网确认出版物真伪，也不代表对论证含义作出绝对事实保证。未提供 `citation_evidence_enabled` 的旧应用版本默认关闭严格模式，行为保持兼容。

联网检索只使用公开文献元数据和合法开放获取全文，不绕过登录、付费墙或版权限制。普通模式可以使用检索元数据和摘要；严格模式只会纳入成功取得开放全文并通过 DOI/题名身份核验的在线文献。单个文献服务失败时会使用另一个服务降级，全部服务失败时本次运行明确失败，不静默伪造参考文献。

结构化目录输入使用以下协议。`nodes` 必须按文档顺序扁平排列，首节点必须为一级标题，层级不能一次跳过一级：

```json
{
  "version": 1,
  "nodes": [
    { "id": "chapter-1", "title": "绪论", "level": 1 },
    { "id": "section-1-1", "title": "研究背景", "level": 2 }
  ]
}
```

Worker 会按顶层章节分配正文预算，只接受 `ID -> content` 的模型结果；未知节点会被忽略，缺失节点只补写一次，最终目录签名不一致时停止输出。未提交 `outline_spec` 的旧应用版本继续使用原有目录规划流程。

## 开发一个新智能体/工作流

1. 在 Worker 中新增或复用一个运行路由，例如 `/product-image/runs`。
2. 在 Sub2API 管理后台发布应用版本，配置 Worker Host 和运行路径。
3. 配置用户输入表单，例如文本、下拉选项、图片上传、文件上传。
4. 配置模型能力绑定，例如“文本改写”“图片生成”“图片理解”，并按需限制允许的 Key 分组。
5. Worker 从请求里读取 `input`、`input_artifacts`（兼容 `input_assets`）和 `node_model_policy`。
6. Worker 调用 `model_proxy_url`，每次请求带上对应的 `policy_key`，不要自己找用户 Key。
7. Worker 有阶段进度时调用 callback，便于用户页自动刷新状态。
8. Worker 生成结果后调用 Artifact 上传/登记接口，把对象存储引用交回 Sub2API。
9. Worker 最后 callback `succeeded` 或 `failed`。

## 模型能力示例

文本应用：

```json
{
  "text.generate": {
    "node_id": "text",
    "role": "generate",
    "capability": "text",
    "model": "gpt-4.1-mini",
    "required": true
  }
}
```

统一文生图 / 图生图智能体：

```json
{
  "image_generation.generate": {
    "node_id": "image_generation",
    "role": "generate",
    "capability": "image",
    "model": "gpt-image-1"
  }
}
```

同一个应用版本使用 `/image/runs`，输入表单包含必填 `prompt` 和可选图片字段 `reference_image`（`x-asset-role: reference`）。未上传参考图时 Worker 调用 `/v1/images/generations`；上传一张或多张参考图后自动改用 multipart `/v1/images/edits`，每张图片使用一个 `image` 字段，输出仍固定为一张。无需再创建第二个应用或第二个 Worker。参考图通过 Sub2API 签名 URL 下载，生成结果仍由 Sub2API Artifact 接口写入当前对象存储。

音频和视频应用使用同一套 Model Proxy 与 Artifact 链路。Worker 会按 `capability` 自动选择处理方式：

- `audio_speech`：调用 `/v1/audio/speech`，把二进制音频归档为结果 Artifact。
- `audio_transcription` / `audio_translation`：下载用户上传的音频资产，通过 multipart 请求调用 `/v1/audio/transcriptions` 或 `/v1/audio/translations`。
- `video_generation`：调用 `/v1/videos`，可把上传图片作为 `input_reference` multipart 参考图，轮询任务状态后通过鉴权的 `/content` 路径下载并归档视频 Artifact。

```json
{
  "speech.generate": {
    "node_id": "speech",
    "role": "generate",
    "capability": "audio_speech",
    "model": "gpt-4o-mini-tts"
  },
  "video.generate": {
    "node_id": "video",
    "role": "generate",
    "capability": "video_generation",
    "model": "sora-2"
  }
}
```

媒体链路限制：

- Model Proxy 单次请求和响应上限是 64 MiB。Worker 默认把音频上传限制为 60 MiB（`MAX_MODEL_PROXY_ASSET_BYTES`），为 multipart 边界和字段预留空间。
- 多图编辑默认最多 16 张，每张最多 20 MiB、合计最多 45 MiB，分别由 `MAX_IMAGE_REFERENCE_COUNT`、`MAX_IMAGE_REFERENCE_BYTES` 和 `MAX_IMAGE_REFERENCE_TOTAL_BYTES` 控制。最终是否支持多图语义融合取决于所选上游模型。
- 视频完成后优先通过 `/v1/videos/:id/content` 获取，确保私有上游仍携带用户 Key；仅当 content 路径不可用且响应提供了可直接访问 URL 时才回退。content 路径受 64 MiB 响应上限约束。
- 输入音频、参考图和远程媒体按流式字节数执行上限检查，不会先把超限文件完整读入内存；Artifact 上限还会取应用版本 `artifact_policy.max_file_mb` 与 Worker 配置中的较小值。
- `/video/runs` 使用 OpenAI 兼容 `/v1/videos` 协议。Grok 的 `/v1/videos/generations`、编辑和扩展路由由 Sub2API 官方 Grok 网关处理，不在这个通用 Worker 中混用。
- 取消会在模型调用之间和视频轮询之间生效；已经发出的单次上游请求不能保证立即中断。

如果设置了 `model_group_id`，用户运行应用时只能选择该分组下自己的平台 Key。管理员不填写用户 Key。

## 并发控制

Sub2API 和 Worker 各管一层并发：

- Sub2API Worker Host 的最大并发用于限制派发到某个 Worker Host 的运行数。
- Worker 的 `MAX_CONCURRENCY` 用于限制当前 Worker 进程内真正执行的任务数。
- 多台 Worker 可以登记成多个 Worker Host，或在同一 Host 后面挂负载均衡。

推荐线上先用：

```powershell
$env:MAX_CONCURRENCY='4'
$env:STREAM_PROGRESS_INTERVAL_SECONDS='1'
uvicorn sub2api_worker.main:app --host 0.0.0.0 --port 8091
```

需要横向扩容时，优先让每个 Worker Host 地址对应一个独立 Worker 实例，再由 Sub2API 的 Host 并发限制统一调度。这样取消请求能直接命中持有运行状态的进程；后端的 Model Proxy 和 Artifact 接口仍会拒绝已取消运行，作为跨进程兜底。

## 取消运行

Sub2API 会调用 Worker 的取消路径。Worker 已经开始的单次上游请求可能无法立刻中断，但后续步骤必须检查取消状态，避免用户停止后继续发新的模型请求。

## 本地验收

先执行不访问外网的协议烟雾测试：

```powershell
python -m py_compile src/sub2api_worker/main.py tests/test_media_worker.py
python -m unittest discover -s tests -p 'test_*.py' -v
```

1. 启动 Sub2API、前端、Redis 和 Worker。
2. 管理后台创建 Worker Host，健康检查通过。
3. 发布应用版本；文本填 `/runs`，统一文生图 / 图生图智能体填 `/image/runs`。
4. 用户侧选择应用，选择平台内已有 Key，提交运行。
5. 运行完成后检查用户侧结果预览、对象存储引用、使用记录和扣费记录。
6. 测试停止运行，确认后续步骤不再继续发起新的模型请求。
