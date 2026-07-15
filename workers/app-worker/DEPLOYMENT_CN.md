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
VIDEO_POLL_INTERVAL_SECONDS=2

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
docker build -t sub2api-app-worker:0.1.0 .
```

## 6. 使用配置文件启动

```bash
docker run -d \
  --name sub2api-app-worker \
  --restart unless-stopped \
  --env-file /opt/sub2api-owner/workers/app-worker/.env \
  -p 8091:8091 \
  sub2api-app-worker:0.1.0
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
```

## 10. 修改配置后重启

修改 `.env` 后必须重建容器，镜像不需要重新构建：

```bash
docker rm -f sub2api-app-worker

docker run -d \
  --name sub2api-app-worker \
  --restart unless-stopped \
  --env-file /opt/sub2api-owner/workers/app-worker/.env \
  -p 8091:8091 \
  sub2api-app-worker:0.1.0
```

## 11. 更新 Worker 代码

只有 `workers/app-worker` 代码发生变化时才需要重新构建 Worker 镜像：

```bash
cd /opt/sub2api-owner
git pull --ff-only origin main

cd workers/app-worker
docker build -t sub2api-app-worker:0.1.0 .
docker rm -f sub2api-app-worker

docker run -d \
  --name sub2api-app-worker \
  --restart unless-stopped \
  --env-file /opt/sub2api-owner/workers/app-worker/.env \
  -p 8091:8091 \
  sub2api-app-worker:0.1.0
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
