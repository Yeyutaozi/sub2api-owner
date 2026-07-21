# App Worker 独立服务器部署与启动

本文说明如何将 `workers/app-worker` 部署到独立服务器，并连接另一台服务器上的 Sub2API 主服务。

## 1. 部署关系

```text
Sub2API 主服务器  -> Worker 服务器:8091       派发任务、取消任务、健康检查
Worker 服务器     -> Sub2API API Base URL    模型代理、运行回调、Artifact 上传
```

Worker 不需要部署 PostgreSQL、Sub2API Redis 或对象存储密钥。对象存储仍由 Sub2API 主服务管理。

## 2. 前置条件

- Worker 服务器已安装 Git、Docker 和 Docker Compose。
- Worker 服务器可以访问 Sub2API 的 API 域名。
- Sub2API 主服务器可以访问 Worker 服务器的 `8091` 端口。
- 推荐两台服务器使用内网通信；如果只能使用公网，应通过防火墙限制来源 IP，或为 Worker 配置 HTTPS。

Word 论文工作流不依赖 Microsoft Word、Office COM 或 Windows 字体。Worker 使用
`python-docx` 直接生成标准 `.docx` 文件，因此可以在纯 Linux Docker 服务器运行。
字体名称和字号会写入 Word 文档；只有额外在服务器端转换 PDF/图片预览时，才需要
安装对应中文字体和 LibreOffice，这不是当前 Worker 生成 Word 成品的必需条件。

新版论文表单会额外提交 `outline_spec.version = 1` 的结构化目录。Worker 在 Linux
服务器上按节点 ID 锁定标题、层级和顺序，模型只生成各节点正文。没有该字段的旧版
应用仍走兼容流程，因此升级 Worker 后不要求立即重新发布所有历史应用版本。

严格引用证据核验同样不依赖 Office：PDF 按页、DOCX 按段落/表格、文本按行提取。
新版应用开启后，需要连续编号书目，并以 `[1]`、`[2]` 开头命名对应全文文件。
Worker 只声明“已与上传来源逐字核对”，不会把该结果描述为联网验证出版物真伪。

## 3. 拉取代码

在 Worker 服务器执行：

```bash
cd /opt
git clone --depth 1 --single-branch --branch main --no-tags \
  https://github.com/Yeyutaozi/sub2api-owner.git
cd sub2api-owner
git rev-parse --short HEAD
```

国内服务器可使用已审核的 GitHub 加速地址或公司内部 Git 镜像。拉取后必须核对提交号与主服务部署版本一致。

## 4. 创建运行配置

```bash
cd /opt/sub2api-owner/workers/app-worker
cp .env.example .env
nano .env
```

推荐配置：

```env
HOST=0.0.0.0
PORT=8091

MAX_CONCURRENCY=4
MODEL_PROXY_TIMEOUT_SECONDS=300
CALLBACK_TIMEOUT_SECONDS=30
STREAM_PROGRESS_INTERVAL_SECONDS=1

MAX_REMOTE_ARTIFACT_BYTES=104857600
MAX_MODEL_PROXY_ASSET_BYTES=62914560
MAX_IMAGE_REFERENCE_COUNT=16
MAX_IMAGE_REFERENCE_BYTES=20971520
MAX_IMAGE_REFERENCE_TOTAL_BYTES=47185920
PAPER_REFERENCE_MAX_FILE_BYTES=12582912
PAPER_REFERENCE_MAX_CHARS_PER_FILE=24000
PAPER_REFERENCE_MAX_TOTAL_CHARS=60000
PAPER_EVIDENCE_MIN_QUOTE_CHARS=12
PAPER_EVIDENCE_AUDIT_BATCH_SIZE=24
PAPER_EVIDENCE_CHUNKS_PER_OCCURRENCE=5
PAPER_EVIDENCE_MAX_PROMPT_CHARS=80000
PAPER_LITERATURE_TIMEOUT_SECONDS=20
PAPER_LITERATURE_MAX_RESULTS=12
PAPER_LITERATURE_MAX_PDF_BYTES=12582912
PAPER_LITERATURE_MAILTO=admin@example.com
PAPER_LITERATURE_USER_AGENT=Sub2API-App-Worker/0.4
PAPER_LITERATURE_ALLOW_PROXY_FAKE_IP=false
VIDEO_POLL_INTERVAL_SECONDS=5

VERIFY_WORKER_SIGNATURE=true
SIGNATURE_MAX_AGE_SECONDS=300
```

说明：

- `.env` 是 Worker 的运行配置文件。
- `worker.yaml` 是能力和路由描述文件，不负责加载运行参数。
- Dockerfile 当前固定监听容器内 `8091`，因此端口映射应保持 `8091:8091`。
- `MAX_CONCURRENCY` 应与后台 Worker Host 的最大并发保持一致。
- 生产环境必须保持 `VERIFY_WORKER_SIGNATURE=true`。

## 5. 构建镜像

```bash
cd /opt/sub2api-owner/workers/app-worker
docker build -t sub2api-app-worker:0.4.0 .
```

## 6. 使用配置文件启动

```bash
docker run -d \
  --name sub2api-app-worker \
  --restart unless-stopped \
  --env-file /opt/sub2api-owner/workers/app-worker/.env \
  -p 8091:8091 \
  sub2api-app-worker:0.4.0
```

检查容器和日志：

```bash
docker ps --filter name=sub2api-app-worker
docker logs --tail=100 sub2api-app-worker
curl http://127.0.0.1:8091/health
```

健康接口应返回 `status: healthy`。

## 7. 配置防火墙

只允许 Sub2API 主服务器访问 Worker 的 `8091`。例如主服务器内网 IP 是 `10.0.0.10`：

```bash
sudo ufw allow from 10.0.0.10 to any port 8091 proto tcp
```

云服务器还需要在安全组中添加同样的来源限制。不要将 `8091` 无限制开放给公网。

## 8. 验证双向连通

在 Worker 服务器测试主服务：

```bash
curl https://sub.example.com/health
```

在 Sub2API 主服务器测试 Worker：

```bash
curl http://WORKER_PRIVATE_IP:8091/health
```

两个方向都必须返回 HTTP 200。

## 9. 配置 Sub2API 主服务

在管理后台进入：

```text
系统设置 -> 站点设置 -> API 端点地址
```

填写 Worker 能访问的主服务地址，例如：

```text
https://sub.example.com
```

不要附加 `/api/v1`，末尾也不需要 `/`。

然后进入 Worker Host 管理页面并新建 Worker：

```text
名称：远程 App Worker
服务地址：http://WORKER_PRIVATE_IP:8091
健康路径：/health
默认运行路径：/runs
取消路径：/cancel
最大并发：4
状态：启用
```

保存后执行健康检查。

应用版本可按能力选择运行路径：

```text
通用/文本：/runs 或 /text/runs
文生图 / 图生图：/image/runs（参考图可选，同一个智能体自动切换）
工作流：/workflow/runs
音频：/audio/runs
视频：/video/runs
Grok 视频：/grok-video/runs
AI 商品营销包：/product-marketing/runs
Word 论文：/academic-paper/runs
```

Git 只发布 Worker 和前端代码，不会同步数据库中的应用、应用版本、Worker Host 或对象存储配置。
升级论文 Worker 后，管理员还必须在应用管理中基于“Word 论文模板”创建并发布新版本，确认版本绑定
`/academic-paper/runs`，且输入结构包含联网文献检索和严格引用证据字段。发布新版本不需要重启
Sub2API 或 Worker；旧版本会继续保留用于历史运行回溯。

## 10. 修改配置后重启

修改 `.env` 后必须重建容器，镜像不需要重新构建：

```bash
docker rm -f sub2api-app-worker

docker run -d \
  --name sub2api-app-worker \
  --restart unless-stopped \
  --env-file /opt/sub2api-owner/workers/app-worker/.env \
  -p 8091:8091 \
  sub2api-app-worker:0.4.0
```

## 11. 更新 Worker 代码

只有 `workers/app-worker` 代码发生变化时才需要重新构建 Worker 镜像：

```bash
cd /opt/sub2api-owner
git pull --ff-only origin main

cd workers/app-worker
docker build -t sub2api-app-worker:0.4.0 .
docker rm -f sub2api-app-worker

docker run -d \
  --name sub2api-app-worker \
  --restart unless-stopped \
  --env-file /opt/sub2api-owner/workers/app-worker/.env \
  -p 8091:8091 \
  sub2api-app-worker:0.4.0
```

最后重新执行健康检查和一次真实应用运行。

## 12. 常见问题

### 主服务健康检查 Worker 失败

- 从主服务器执行 `curl http://WORKER_PRIVATE_IP:8091/health`。
- 检查 Worker 服务器防火墙和云安全组。
- 检查 Docker 是否映射了 `8091:8091`。

### Worker 一直执行但无法回调

- 从 Worker 服务器执行 `curl https://sub.example.com/health`。
- 检查后台“API 端点地址”，不能填写 `127.0.0.1`，也不能附加 `/api/v1`。
- 检查主服务反向代理、HTTPS 证书和访问控制。

### 修改 `.env` 后没有生效

Docker 不会自动重新读取 `.env`。删除并重新创建 Worker 容器即可，不需要重新构建镜像。

## 13. Grok 生视频 Worker 应用配置

本 Worker 已内置 Grok 视频专用入口：

```text
/grok-video/runs
```

它和旧的 `/video/runs` 不同：旧入口继续走 OpenAI 兼容 `/v1/videos`；Grok 入口会按模式调用：

- 文生视频、图生视频、多参考图生视频：`/v1/videos/generations`
- 视频编辑：`/v1/videos/edits`
- 视频续写/扩展：`/v1/videos/extensions`
- 任务状态轮询：`/v1/videos/{request_id}`

### 13.1 Worker Host

管理后台创建或编辑 Worker Host：

```text
服务地址：http://WORKER_PRIVATE_IP:8091
健康路径：/health
默认运行路径：/grok-video/runs
取消路径：/cancel
最大并发：按服务器能力填写，例如 2 或 4
```

也可以把默认运行路径仍设为 `/runs`，然后在具体应用版本里把运行路径填成 `/grok-video/runs`。

### 13.2 应用输入字段

建议同一个智能体支持五种模式，用一个下拉字段控制：

```text
mode：select，必填
  text_to_video
  image_to_video
  reference_to_video
  edit_video
  extend_video

prompt：textarea，必填
duration：number/select，可选
resolution：select，可选，例如 720p
aspect_ratio：select，可选，例如 16:9、9:16
source_image：image，可选，用于 image_to_video
reference_images：image，多文件，可选，用于 reference_to_video
source_video：video/file，可选，用于 edit_video / extend_video
```

字段名不是硬性唯一，但推荐使用上面的名字。Worker 也兼容 `video_mode`、`generation_mode`、`operation`、`source_video_url`、`video_url` 等别名。

### 13.3 模式边界

```text
text_to_video
  只允许 prompt，不允许上传图片或视频。

image_to_video
  需要 1 张 source_image。
  多张图片请改用 reference_to_video。

reference_to_video
  需要 reference_images。
  默认最多 7 张，可用 GROK_VIDEO_REFERENCE_IMAGE_MAX_COUNT 调整。

edit_video
  需要 1 个 source_video 或 source_video_url。
  不接受 duration / resolution / aspect_ratio。
  如果输入视频元数据里有 duration_seconds，默认限制最长 8.7 秒。

extend_video
  需要 1 个 source_video 或 source_video_url。
  只接受 duration，不接受 resolution / aspect_ratio。
  duration 默认限制 2-10 秒。
  如果输入视频元数据里有 duration_seconds，默认要求源视频 2-15 秒。
```

相关环境变量：

```env
GROK_VIDEO_REFERENCE_IMAGE_MAX_COUNT=7
GROK_VIDEO_DURATION_MAX_SECONDS=10
GROK_VIDEO_EXTENSION_DURATION_MIN_SECONDS=2
GROK_VIDEO_EXTENSION_DURATION_MAX_SECONDS=10
GROK_VIDEO_EDIT_INPUT_MAX_SECONDS=8.7
GROK_VIDEO_EXTENSION_INPUT_MIN_SECONDS=2
GROK_VIDEO_EXTENSION_INPUT_MAX_SECONDS=15
```

修改 `.env` 后需要重建容器：

```bash
docker rm -f sub2api-app-worker
docker run -d \
  --name sub2api-app-worker \
  --restart unless-stopped \
  --env-file /opt/sub2api-owner/workers/app-worker/.env \
  -p 8091:8091 \
  sub2api-app-worker:0.4.0
```

### 13.4 模型策略

应用版本里模型厂商选择 `Grok`。至少配置一个视频模型策略：

```json
{
  "video.generate": {
    "node_id": "video",
    "role": "generate",
    "capability": "video_generation",
    "model": "grok-imagine-video"
  }
}
```

如果你想让图生视频单独使用 `grok-imagine-video-1.5`，可以再加一个更具体的策略。Worker 会按 `mode` 优先匹配：

```json
{
  "video.generate": {
    "node_id": "video",
    "role": "generate",
    "capability": "video_generation",
    "model": "grok-imagine-video"
  },
  "video.image_to_video": {
    "node_id": "video",
    "role": "image_to_video",
    "capability": "image_to_video",
    "model": "grok-imagine-video-1.5"
  }
}
```

注意：Worker 不保存 Grok API Key。Grok Key 仍在 Sub2API 主服务的用户 Key / 模型代理链路里管理。

### 13.5 Artifact / COS 注意点

- 生视频结果仍通过 Sub2API Artifact 接口写入对象存储，Worker 不需要 COS SecretId/SecretKey。
- 图生视频和多参考图会把用户上传的图片转成 data URL 交给主服务 Model Proxy。
- 视频编辑/续写会把 `source_video` 的签名 URL 传给上游模型；这个 URL 必须能被 xAI/Grok 上游访问。若你的对象存储只允许内网访问，真实调用会失败。
- 视频结果通常较大，应用版本的 artifact policy 建议把 `max_file_mb` 设置到 512MB 左右，同时主服务 `agent_artifacts.max_upload_bytes` 也要足够大。

### 13.6 快速验收

1. 主服务确认 Grok 账号/Key 可用。
2. Worker 服务器：

```bash
curl http://127.0.0.1:8091/health
```

3. 主服务服务器：

```bash
curl http://WORKER_PRIVATE_IP:8091/health
```

4. 应用中心发布 Grok 视频应用，运行路径填 `/grok-video/runs`。
5. 依次测试五个 mode：

```text
text_to_video：只填 prompt
image_to_video：prompt + 1 张 source_image
reference_to_video：prompt + 多张 reference_images
edit_video：prompt + 1 个 source_video
extend_video：prompt + 1 个 source_video + duration
```

6. 检查运行记录状态、用户侧结果预览、对象存储文件、扣费记录。
