from __future__ import annotations

import asyncio
import base64
import hashlib
import hmac
import json
import logging
import os
import time
import uuid
from typing import Any

import httpx
from fastapi import BackgroundTasks, FastAPI, HTTPException, Request
from pydantic import BaseModel, ConfigDict, Field


LOGGER = logging.getLogger("sub2api_worker")
logging.basicConfig(level=os.getenv("LOG_LEVEL", "INFO"))

WORKER_VERSION = "0.1.0"
PROTOCOL = "sub2api-worker-v1"
MAX_CONCURRENCY = int(os.getenv("MAX_CONCURRENCY", "4"))
MODEL_PROXY_TIMEOUT_SECONDS = float(os.getenv("MODEL_PROXY_TIMEOUT_SECONDS", "300"))
CALLBACK_TIMEOUT_SECONDS = float(os.getenv("CALLBACK_TIMEOUT_SECONDS", "30"))
MAX_REMOTE_ARTIFACT_BYTES = int(os.getenv("MAX_REMOTE_ARTIFACT_BYTES", str(100 * 1024 * 1024)))
VERIFY_WORKER_SIGNATURE = os.getenv("VERIFY_WORKER_SIGNATURE", "true").lower() not in {"0", "false", "no"}
SIGNATURE_MAX_AGE_SECONDS = int(os.getenv("SIGNATURE_MAX_AGE_SECONDS", "300"))

app = FastAPI(title="Sub2API App Worker", version=WORKER_VERSION)
run_semaphore = asyncio.Semaphore(max(MAX_CONCURRENCY, 1))
canceled_runs: set[int] = set()


class LooseModel(BaseModel):
    model_config = ConfigDict(extra="allow")


class WorkerArtifactRef(LooseModel):
    artifact_id: int | None = None
    type: str = ""
    name: str = ""
    mime_type: str = ""
    url: str = ""
    object_key: str = ""
    size_bytes: int = 0
    sha256: str = ""
    metadata: dict[str, Any] = Field(default_factory=dict)


class ModelPolicy(LooseModel):
    node_id: str = ""
    role: str = ""
    model: str = ""
    model_group_id: int | None = None
    capability: str = ""
    required: bool = False


class WorkerRunUserContext(LooseModel):
    user_id: int
    api_key_id: int
    group_id: int | None = None


class WorkerRunRequest(LooseModel):
    run_id: int
    app_id: int
    app_version_id: int
    run_token: str
    callback_url: str
    model_proxy_url: str
    artifact_url: str = ""
    timeout_seconds: int = 600
    user: WorkerRunUserContext
    input: dict[str, Any] = Field(default_factory=dict)
    input_artifacts: list[WorkerArtifactRef] = Field(default_factory=list)
    input_assets: list[WorkerArtifactRef] = Field(default_factory=list)
    node_model_policy: dict[str, ModelPolicy] = Field(default_factory=dict)
    metadata: dict[str, Any] = Field(default_factory=dict)


class WorkerCancelRequest(LooseModel):
    run_id: int
    run_token: str
    reason: str = ""


class SelectedPolicy(BaseModel):
    policy_key: str
    node_id: str
    role: str
    model: str
    model_group_id: int | None = None
    capability: str = ""


class WorkerFailure(Exception):
    def __init__(self, code: str, message: str) -> None:
        self.code = code
        self.message = message
        super().__init__(message)


@app.get("/")
async def root() -> dict[str, Any]:
    return await health()


@app.get("/health")
async def health() -> dict[str, Any]:
    return {
        "status": "healthy",
        "protocol": PROTOCOL,
        "version": WORKER_VERSION,
        "capabilities": ["text", "vision", "image_generation", "workflow"],
        "routes": {
            "runs": ["/runs", "/text/runs", "/prompt/runs", "/workflow/runs"],
            "cancel": "/cancel",
        },
        "max_concurrency": MAX_CONCURRENCY,
        "metadata": {"model_proxy_required": True},
    }


@app.post("/runs")
@app.post("/text/runs")
@app.post("/prompt/runs")
@app.post("/workflow/runs")
async def submit_run(request: Request, background_tasks: BackgroundTasks) -> dict[str, Any]:
    raw_body = await request.body()
    try:
        payload = WorkerRunRequest.model_validate_json(raw_body)
    except Exception as exc:
        raise HTTPException(status_code=400, detail=f"invalid worker run payload: {exc}") from exc

    verify_signature_or_raise(payload.run_token, request, raw_body)

    worker_run_id = f"worker-{uuid.uuid4()}"
    background_tasks.add_task(process_run, payload, worker_run_id)
    return {
        "accepted": True,
        "worker_run_id": worker_run_id,
        "status": "running",
        "message": "Worker accepted",
        "estimated_time_seconds": min(max(payload.timeout_seconds, 10), 120),
        "metadata": {
            "worker": "sub2api-app-worker",
            "uses_model_proxy": True,
            "app_slug": payload.metadata.get("app_slug"),
        },
    }


@app.post("/cancel")
async def cancel_run(request: Request) -> dict[str, Any]:
    raw_body = await request.body()
    try:
        payload = WorkerCancelRequest.model_validate_json(raw_body)
    except Exception as exc:
        raise HTTPException(status_code=400, detail=f"invalid cancel payload: {exc}") from exc

    verify_signature_or_raise(payload.run_token, request, raw_body)
    canceled_runs.add(payload.run_id)
    return {
        "accepted": True,
        "status": "canceled",
        "message": payload.reason or "cancel requested",
        "metadata": {"run_id": payload.run_id},
    }


async def process_run(payload: WorkerRunRequest, worker_run_id: str) -> None:
    started = time.perf_counter()
    async with run_semaphore:
        if is_canceled(payload.run_id):
            await callback(payload, "canceled", status="canceled", message="Run canceled before start")
            return

        await callback(
            payload,
            "running",
            status="running",
            progress=0.05,
            message="Worker started",
            metadata={"worker_run_id": worker_run_id},
        )

        try:
            prompt = extract_prompt(payload.input)
            if not prompt:
                raise WorkerFailure("INPUT_PROMPT_REQUIRED", "Please provide a prompt or text input.")

            image_policy = find_policy(payload, capabilities={"image"})
            if image_policy is not None:
                await process_image_run(payload, worker_run_id, started, image_policy, prompt)
                return

            selected_policy = select_policy(payload)
            await callback(
                payload,
                "progress",
                status="running",
                node_id=selected_policy.node_id,
                role=selected_policy.role,
                progress=0.35,
                message="Calling model",
                metadata={
                    "policy_key": selected_policy.policy_key,
                    "model": selected_policy.model,
                    "uses_model_proxy": True,
                },
            )

            if is_canceled(payload.run_id):
                await callback(payload, "canceled", status="canceled", message="Run canceled before model call")
                return

            proxy_result = await call_model_proxy(payload, selected_policy, prompt)
            text = extract_model_text(proxy_result.get("response", {}))
            if not text:
                text = "The model call completed, but no displayable text was returned."

            usage = proxy_result.get("usage") if isinstance(proxy_result.get("usage"), dict) else {}
            duration_ms = int((time.perf_counter() - started) * 1000)
            await callback(
                payload,
                "succeeded",
                status="succeeded",
                node_id=selected_policy.node_id,
                role=selected_policy.role,
                progress=1.0,
                message="Run completed",
                output={
                    "result": text,
                    "model": selected_policy.model,
                    "node": selected_policy.node_id,
                },
                metadata={
                    "worker_run_id": worker_run_id,
                    "policy_key": selected_policy.policy_key,
                    "model": selected_policy.model,
                    "duration_ms": duration_ms,
                    "usage": usage,
                    "uses_model_proxy": True,
                },
            )
        except WorkerFailure as exc:
            await callback_failure(payload, exc.code, exc.message)
        except Exception as exc:  # noqa: BLE001 - keep Worker callbacks robust.
            LOGGER.exception("run failed: run_id=%s", payload.run_id)
            await callback_failure(payload, "WORKER_RUNTIME_ERROR", str(exc))
        finally:
            canceled_runs.discard(payload.run_id)


async def process_image_run(payload: WorkerRunRequest, worker_run_id: str, started: float, image_policy: SelectedPolicy, prompt: str) -> None:
    rewrite_policy = find_policy(payload, capabilities={"text", "model"}, roles={"rewrite", "generate", "caption"})
    final_prompt = prompt
    if rewrite_policy is not None and rewrite_policy.policy_key != image_policy.policy_key:
        await callback(
            payload,
            "progress",
            status="running",
            node_id=rewrite_policy.node_id,
            role=rewrite_policy.role,
            progress=0.25,
            message="正在优化图片提示词",
            metadata={
                "policy_key": rewrite_policy.policy_key,
                "model": rewrite_policy.model,
                "uses_model_proxy": True,
            },
        )
        if is_canceled(payload.run_id):
            await callback(payload, "canceled", status="canceled", message="Run canceled before prompt preparation")
            return
        rewrite_result = await call_model_proxy(payload, rewrite_policy, prompt)
        rewritten = extract_model_text(rewrite_result.get("response", {}))
        if rewritten:
            final_prompt = rewritten

    await callback(
        payload,
        "progress",
        status="running",
        node_id=image_policy.node_id,
        role=image_policy.role,
        progress=0.6,
        message="正在生成图片",
        metadata={
            "policy_key": image_policy.policy_key,
            "model": image_policy.model,
            "uses_model_proxy": True,
        },
    )
    if is_canceled(payload.run_id):
        await callback(payload, "canceled", status="canceled", message="Run canceled before image generation")
        return

    proxy_result = await call_image_model_proxy(payload, image_policy, final_prompt)
    image = extract_image_result(proxy_result.get("response", {}))
    if not image:
        raise WorkerFailure("IMAGE_RESULT_EMPTY", "The image model returned no image URL or base64 data.")

    artifact_name = f"generated-{payload.run_id}.png"
    artifact: dict[str, Any]
    if image.get("url"):
        artifact = await archive_remote_artifact(
            payload,
            name=artifact_name,
            url=str(image["url"]),
            mime_type=str(image.get("mime_type") or "image/png"),
            metadata={
                "worker_run_id": worker_run_id,
                "policy_key": image_policy.policy_key,
                "model": image_policy.model,
                "prompt": final_prompt,
            },
        )
    else:
        artifact = await upload_base64_artifact(
            payload,
            name=artifact_name,
            b64_json=str(image["b64_json"]),
            mime_type=str(image.get("mime_type") or "image/png"),
            metadata={
                "worker_run_id": worker_run_id,
                "policy_key": image_policy.policy_key,
                "model": image_policy.model,
                "prompt": final_prompt,
            },
        )

    usage = proxy_result.get("usage") if isinstance(proxy_result.get("usage"), dict) else {}
    duration_ms = int((time.perf_counter() - started) * 1000)
    await callback(
        payload,
        "succeeded",
        status="succeeded",
        node_id=image_policy.node_id,
        role=image_policy.role,
        progress=1.0,
        message="图片已生成",
        output={
            "result": "图片已生成",
            "prompt": final_prompt,
            "artifact": artifact,
        },
        metadata={
            "worker_run_id": worker_run_id,
            "policy_key": image_policy.policy_key,
            "model": image_policy.model,
            "duration_ms": duration_ms,
            "usage": usage,
            "uses_model_proxy": True,
        },
    )


async def call_model_proxy(payload: WorkerRunRequest, policy: SelectedPolicy, prompt: str) -> dict[str, Any]:
    model_request: dict[str, Any] = {
        "model": policy.model,
        "messages": [
            {
                "role": "user",
                "content": build_message_content(prompt, input_artifacts(payload)),
            }
        ],
        "stream": False,
    }
    body: dict[str, Any] = {
        "run_id": payload.run_id,
        "node_id": policy.node_id,
        "role": policy.role,
        "model": policy.model,
        "endpoint": "/v1/chat/completions",
        "request": model_request,
        "metadata": {
            "worker": "sub2api-app-worker",
            "policy_key": policy.policy_key,
            "capability": policy.capability,
        },
    }
    if policy.model_group_id is not None:
        body["group_id"] = policy.model_group_id

    headers = {
        "Content-Type": "application/json",
        "Accept": "application/json",
        "X-Sub2API-Run-Token": payload.run_token,
        "X-Sub2API-Agent-Run-Token": payload.run_token,
    }
    async with httpx.AsyncClient(timeout=MODEL_PROXY_TIMEOUT_SECONDS) as client:
        response = await client.post(payload.model_proxy_url, json=body, headers=headers)
    if response.status_code >= 400:
        raise WorkerFailure("MODEL_PROXY_FAILED", truncate(response.text, 1000))

    data = response.json()
    return unwrap_sub2api_response(data)


async def call_image_model_proxy(payload: WorkerRunRequest, policy: SelectedPolicy, prompt: str) -> dict[str, Any]:
    image_request: dict[str, Any] = {
        "model": policy.model,
        "prompt": prompt,
        "n": 1,
    }
    size = payload.input.get("size")
    if isinstance(size, str) and size.strip():
        image_request["size"] = size.strip()
    quality = payload.input.get("quality")
    if isinstance(quality, str) and quality.strip():
        image_request["quality"] = quality.strip()

    body: dict[str, Any] = {
        "run_id": payload.run_id,
        "node_id": policy.node_id,
        "role": policy.role,
        "model": policy.model,
        "endpoint": "/v1/images/generations",
        "request": image_request,
        "metadata": {
            "worker": "sub2api-app-worker",
            "policy_key": policy.policy_key,
            "capability": policy.capability,
        },
    }
    if policy.model_group_id is not None:
        body["group_id"] = policy.model_group_id

    headers = {
        "Content-Type": "application/json",
        "Accept": "application/json",
        "X-Sub2API-Run-Token": payload.run_token,
        "X-Sub2API-Agent-Run-Token": payload.run_token,
    }
    async with httpx.AsyncClient(timeout=MODEL_PROXY_TIMEOUT_SECONDS) as client:
        response = await client.post(payload.model_proxy_url, json=body, headers=headers)
    if response.status_code >= 400:
        raise WorkerFailure("MODEL_PROXY_FAILED", truncate(response.text, 1000))

    data = response.json()
    return unwrap_sub2api_response(data)


async def register_external_artifact(payload: WorkerRunRequest, *, name: str, url: str, mime_type: str, metadata: dict[str, Any]) -> dict[str, Any]:
    if not payload.artifact_url:
        raise WorkerFailure("ARTIFACT_URL_MISSING", "Sub2API artifact URL is missing.")
    body = {
        "run_id": payload.run_id,
        "type": "output",
        "name": name,
        "mime_type": mime_type,
        "storage_provider": "external",
        "object_url": url,
        "metadata": metadata,
    }
    headers = {
        "Content-Type": "application/json",
        "Accept": "application/json",
        "X-Sub2API-Run-Token": payload.run_token,
    }
    async with httpx.AsyncClient(timeout=CALLBACK_TIMEOUT_SECONDS) as client:
        response = await client.post(payload.artifact_url, json=body, headers=headers)
    if response.status_code >= 400:
        raise WorkerFailure("ARTIFACT_REGISTER_FAILED", truncate(response.text, 1000))
    result = unwrap_sub2api_response(response.json())
    return result if isinstance(result, dict) else {}


async def archive_remote_artifact(payload: WorkerRunRequest, *, name: str, url: str, mime_type: str, metadata: dict[str, Any]) -> dict[str, Any]:
    try:
        async with httpx.AsyncClient(timeout=MODEL_PROXY_TIMEOUT_SECONDS, follow_redirects=True) as client:
            response = await client.get(url)
        response.raise_for_status()
    except Exception as exc:
        raise WorkerFailure("ARTIFACT_DOWNLOAD_FAILED", f"无法下载模型生成结果：{truncate(str(exc), 500)}") from exc
    content_length = int(response.headers.get("content-length") or 0)
    if content_length > MAX_REMOTE_ARTIFACT_BYTES or len(response.content) > MAX_REMOTE_ARTIFACT_BYTES:
        raise WorkerFailure("ARTIFACT_TOO_LARGE", "模型生成结果超过 Worker 允许归档的大小")
    resolved_mime = response.headers.get("content-type", "").split(";", 1)[0].strip() or mime_type
    return await upload_artifact_bytes(payload, name=name, raw=response.content, mime_type=resolved_mime, metadata=metadata)


async def upload_base64_artifact(payload: WorkerRunRequest, *, name: str, b64_json: str, mime_type: str, metadata: dict[str, Any]) -> dict[str, Any]:
    if not payload.artifact_url:
        raise WorkerFailure("ARTIFACT_URL_MISSING", "Sub2API artifact URL is missing.")
    try:
        raw = base64.b64decode(b64_json)
    except Exception as exc:
        raise WorkerFailure("IMAGE_BASE64_INVALID", "The image model returned invalid base64 data.") from exc
    return await upload_artifact_bytes(payload, name=name, raw=raw, mime_type=mime_type, metadata=metadata)


async def upload_artifact_bytes(payload: WorkerRunRequest, *, name: str, raw: bytes, mime_type: str, metadata: dict[str, Any]) -> dict[str, Any]:
    headers = {
        "Accept": "application/json",
        "X-Sub2API-Run-Token": payload.run_token,
    }
    data = {
        "type": "output",
        "name": name,
        "mime_type": mime_type,
        "metadata": json.dumps(metadata, ensure_ascii=False),
    }
    files = {"file": (name, raw, mime_type)}
    async with httpx.AsyncClient(timeout=MODEL_PROXY_TIMEOUT_SECONDS) as client:
        response = await client.post(f"{payload.artifact_url.rstrip('/')}/upload", data=data, files=files, headers=headers)
    if response.status_code >= 400:
        raise WorkerFailure("ARTIFACT_UPLOAD_FAILED", truncate(response.text, 1000))
    result = unwrap_sub2api_response(response.json())
    return result if isinstance(result, dict) else {}


async def callback(
    payload: WorkerRunRequest,
    event_type: str,
    *,
    status: str = "",
    node_id: str = "",
    role: str = "",
    progress: float | None = None,
    message: str = "",
    output: dict[str, Any] | None = None,
    metadata: dict[str, Any] | None = None,
) -> None:
    body: dict[str, Any] = {
        "run_id": payload.run_id,
        "run_token": payload.run_token,
        "event_type": event_type,
        "status": status,
        "node_id": node_id,
        "role": role,
        "message": message,
        "metadata": metadata or {},
    }
    if progress is not None:
        body["progress"] = progress
    if output is not None:
        body["output"] = output

    headers = {
        "Content-Type": "application/json",
        "Accept": "application/json",
        "X-Sub2API-Run-Token": payload.run_token,
    }
    async with httpx.AsyncClient(timeout=CALLBACK_TIMEOUT_SECONDS) as client:
        response = await client.post(payload.callback_url, json=body, headers=headers)
    if response.status_code >= 400:
        LOGGER.warning(
            "callback rejected: run_id=%s status=%s body=%s",
            payload.run_id,
            response.status_code,
            truncate(response.text, 500),
        )


async def callback_failure(payload: WorkerRunRequest, code: str, message: str) -> None:
    await callback(
        payload,
        "failed",
        status="failed",
        message=message,
        metadata={"error_code": code},
    )


def select_policy(payload: WorkerRunRequest) -> SelectedPolicy:
    policies = payload.node_model_policy or {}
    prefer_vision = any(item.mime_type.startswith("image/") for item in input_artifacts(payload))
    candidates: list[tuple[int, str, ModelPolicy]] = []
    for key, policy in policies.items():
        normalized = policy if isinstance(policy, ModelPolicy) else ModelPolicy.model_validate(policy)
        node_id, role = policy_key_parts(key)
        normalized.node_id = normalized.node_id or node_id
        normalized.role = normalized.role or role or "generate"
        capability = (normalized.capability or "model").lower()
        score = 50
        if normalized.model:
            score -= 10
        if prefer_vision and capability in {"vision", "image", "model"}:
            score -= 20
        if not prefer_vision and capability in {"text", "model"}:
            score -= 20
        if normalized.role in {"generate", "rewrite", "summarize", "caption", "vision"}:
            score -= 5
        candidates.append((score, key, normalized))

    if candidates:
        _, key, policy = sorted(candidates, key=lambda item: (item[0], item[1]))[0]
        if not policy.model:
            raise WorkerFailure("MODEL_POLICY_MODEL_REQUIRED", f"Model policy {key} is missing model.")
        return SelectedPolicy(
            policy_key=key,
            node_id=policy.node_id or "text",
            role=policy.role or "generate",
            model=policy.model,
            model_group_id=policy.model_group_id,
            capability=policy.capability or "model",
        )

    default_model = payload.metadata.get("default_model")
    model = ""
    if isinstance(default_model, dict):
        model = str(default_model.get("model") or default_model.get("name") or "").strip()
    model = model or os.getenv("DEFAULT_MODEL", "").strip()
    if not model:
        raise WorkerFailure(
            "MODEL_POLICY_REQUIRED",
            "This app version has no node model policy, so the Worker cannot call Sub2API Model Proxy.",
        )
    return SelectedPolicy(policy_key="default.generate", node_id="default", role="generate", model=model, capability="model")


def find_policy(
    payload: WorkerRunRequest,
    *,
    capabilities: set[str] | None = None,
    roles: set[str] | None = None,
) -> SelectedPolicy | None:
    candidates: list[tuple[int, str, ModelPolicy]] = []
    for key, policy in (payload.node_model_policy or {}).items():
        normalized = policy if isinstance(policy, ModelPolicy) else ModelPolicy.model_validate(policy)
        node_id, role = policy_key_parts(key)
        normalized.node_id = normalized.node_id or node_id
        normalized.role = normalized.role or role or "generate"
        capability = (normalized.capability or "model").lower()
        if capabilities is not None and capability not in capabilities:
            continue
        if roles is not None and normalized.role not in roles:
            continue
        score = 50
        if normalized.model:
            score -= 10
        if roles is not None and normalized.role in roles:
            score -= 10
        if capabilities is not None and capability in capabilities:
            score -= 10
        candidates.append((score, key, normalized))

    if not candidates:
        return None
    _, key, policy = sorted(candidates, key=lambda item: (item[0], item[1]))[0]
    if not policy.model:
        raise WorkerFailure("MODEL_POLICY_MODEL_REQUIRED", f"Model policy {key} is missing model.")
    return SelectedPolicy(
        policy_key=key,
        node_id=policy.node_id or "text",
        role=policy.role or "generate",
        model=policy.model,
        model_group_id=policy.model_group_id,
        capability=policy.capability or "model",
    )


def build_message_content(prompt: str, artifacts: list[WorkerArtifactRef]) -> str | list[dict[str, Any]]:
    image_parts = [
        {
            "type": "image_url",
            "image_url": {"url": artifact.url},
        }
        for artifact in artifacts
        if artifact.url and artifact.mime_type.startswith("image/")
    ]
    if not image_parts:
        return prompt
    return [{"type": "text", "text": prompt}, *image_parts]


def input_artifacts(payload: WorkerRunRequest) -> list[WorkerArtifactRef]:
    if payload.input_artifacts:
        return payload.input_artifacts
    return payload.input_assets


def extract_prompt(values: dict[str, Any]) -> str:
    for key in ("prompt", "text", "content", "question", "query", "instruction", "description"):
        value = values.get(key)
        if isinstance(value, str) and value.strip():
            return value.strip()
    primitive_lines = []
    for key, value in values.items():
        if key == "input_assets":
            continue
        if isinstance(value, str) and value.strip():
            primitive_lines.append(f"{key}: {value.strip()}")
        elif isinstance(value, (int, float, bool)):
            primitive_lines.append(f"{key}: {value}")
    return "\n".join(primitive_lines).strip()


def extract_model_text(response: dict[str, Any]) -> str:
    choices = response.get("choices")
    if isinstance(choices, list) and choices:
        message = choices[0].get("message") if isinstance(choices[0], dict) else None
        if isinstance(message, dict):
            content = message.get("content")
            text = text_from_content(content)
            if text:
                return text

    output_text = response.get("output_text")
    if isinstance(output_text, str) and output_text.strip():
        return output_text.strip()

    output = response.get("output")
    if isinstance(output, list):
        parts: list[str] = []
        for item in output:
            if not isinstance(item, dict):
                continue
            content = item.get("content")
            text = text_from_content(content)
            if text:
                parts.append(text)
        if parts:
            return "\n".join(parts).strip()

    return ""


def extract_image_result(response: dict[str, Any]) -> dict[str, Any] | None:
    if not isinstance(response, dict):
        return None
    data = response.get("data")
    if isinstance(data, list):
        for item in data:
            if not isinstance(item, dict):
                continue
            url = item.get("url")
            if isinstance(url, str) and url.strip():
                return {"url": url.strip(), "mime_type": item.get("mime_type") or "image/png"}
            b64_json = item.get("b64_json")
            if isinstance(b64_json, str) and b64_json.strip():
                return {"b64_json": b64_json.strip(), "mime_type": item.get("mime_type") or "image/png"}

    output = response.get("output")
    if isinstance(output, list):
        for item in output:
            if not isinstance(item, dict):
                continue
            if item.get("type") in {"image", "output_image"}:
                url = item.get("url")
                if isinstance(url, str) and url.strip():
                    return {"url": url.strip(), "mime_type": item.get("mime_type") or "image/png"}
                b64_json = item.get("b64_json") or item.get("image_base64")
                if isinstance(b64_json, str) and b64_json.strip():
                    return {"b64_json": b64_json.strip(), "mime_type": item.get("mime_type") or "image/png"}

    url = response.get("url")
    if isinstance(url, str) and url.strip():
        return {"url": url.strip(), "mime_type": response.get("mime_type") or "image/png"}
    b64_json = response.get("b64_json")
    if isinstance(b64_json, str) and b64_json.strip():
        return {"b64_json": b64_json.strip(), "mime_type": response.get("mime_type") or "image/png"}
    return None


def text_from_content(content: Any) -> str:
    if isinstance(content, str):
        return content.strip()
    if isinstance(content, list):
        parts = []
        for item in content:
            if isinstance(item, dict):
                value = item.get("text") or item.get("content")
                if isinstance(value, str) and value.strip():
                    parts.append(value.strip())
        return "\n".join(parts).strip()
    return ""


def unwrap_sub2api_response(payload: dict[str, Any]) -> dict[str, Any]:
    if "data" in payload and "code" in payload:
        data = payload.get("data")
        if isinstance(data, dict):
            return data
    return payload


def verify_signature_or_raise(run_token: str, request: Request, body: bytes) -> None:
    if not VERIFY_WORKER_SIGNATURE:
        return
    timestamp = request.headers.get("X-Sub2API-Timestamp", "")
    signature = request.headers.get("X-Sub2API-Signature", "")
    if not timestamp or not signature:
        raise HTTPException(status_code=401, detail="missing worker signature")
    try:
        ts = int(timestamp)
    except ValueError as exc:
        raise HTTPException(status_code=401, detail="invalid worker signature timestamp") from exc
    if SIGNATURE_MAX_AGE_SECONDS > 0 and abs(int(time.time()) - ts) > SIGNATURE_MAX_AGE_SECONDS:
        raise HTTPException(status_code=401, detail="expired worker signature")

    mac = hmac.new(run_token.encode("utf-8"), digestmod=hashlib.sha256)
    mac.update(timestamp.encode("utf-8"))
    mac.update(b".")
    mac.update(body)
    expected = "sha256=" + mac.hexdigest()
    if not hmac.compare_digest(expected, signature):
        raise HTTPException(status_code=401, detail="invalid worker signature")


def policy_key_parts(policy_key: str) -> tuple[str, str]:
    parts = policy_key.split(".", 1)
    if len(parts) == 2:
        return parts[0], parts[1]
    return policy_key, ""


def is_canceled(run_id: int) -> bool:
    return run_id in canceled_runs


def truncate(value: str, limit: int) -> str:
    value = value.strip()
    if len(value) <= limit:
        return value
    return value[:limit] + "..."
