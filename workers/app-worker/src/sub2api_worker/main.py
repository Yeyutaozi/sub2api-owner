from __future__ import annotations

import asyncio
import base64
import hashlib
import hmac
import io
import json
import logging
import os
import re
import time
import uuid
from typing import Any, Awaitable, Callable

import httpx
from fastapi import BackgroundTasks, FastAPI, HTTPException, Request
from pydantic import BaseModel, ConfigDict, Field


LOGGER = logging.getLogger("sub2api_worker")
logging.basicConfig(level=os.getenv("LOG_LEVEL", "INFO"))

WORKER_VERSION = "0.2.0"
PROTOCOL = "sub2api-worker-v1"
MAX_CONCURRENCY = int(os.getenv("MAX_CONCURRENCY", "4"))
MODEL_PROXY_TIMEOUT_SECONDS = float(os.getenv("MODEL_PROXY_TIMEOUT_SECONDS", "300"))
CALLBACK_TIMEOUT_SECONDS = float(os.getenv("CALLBACK_TIMEOUT_SECONDS", "30"))
MAX_REMOTE_ARTIFACT_BYTES = int(os.getenv("MAX_REMOTE_ARTIFACT_BYTES", str(100 * 1024 * 1024)))
MAX_MODEL_PROXY_ASSET_BYTES = int(os.getenv("MAX_MODEL_PROXY_ASSET_BYTES", str(60 * 1024 * 1024)))
MAX_IMAGE_REFERENCE_COUNT = max(int(os.getenv("MAX_IMAGE_REFERENCE_COUNT", "16")), 1)
MAX_IMAGE_REFERENCE_BYTES = int(os.getenv("MAX_IMAGE_REFERENCE_BYTES", str(20 * 1024 * 1024)))
MAX_IMAGE_REFERENCE_TOTAL_BYTES = int(os.getenv("MAX_IMAGE_REFERENCE_TOTAL_BYTES", str(45 * 1024 * 1024)))
PAPER_REFERENCE_MAX_FILE_BYTES = int(os.getenv("PAPER_REFERENCE_MAX_FILE_BYTES", str(12 * 1024 * 1024)))
PAPER_REFERENCE_MAX_CHARS_PER_FILE = int(os.getenv("PAPER_REFERENCE_MAX_CHARS_PER_FILE", "24000"))
PAPER_REFERENCE_MAX_TOTAL_CHARS = int(os.getenv("PAPER_REFERENCE_MAX_TOTAL_CHARS", "60000"))
PAPER_OUTLINE_MAX_NODES = 100
PAPER_MODEL_PROXY_MAX_ATTEMPTS = max(int(os.getenv("PAPER_MODEL_PROXY_MAX_ATTEMPTS", "3")), 1)
PAPER_MODEL_PROXY_RETRY_BASE_SECONDS = max(float(os.getenv("PAPER_MODEL_PROXY_RETRY_BASE_SECONDS", "0.5")), 0.0)
PAPER_MODEL_PROXY_RETRY_MAX_SECONDS = max(float(os.getenv("PAPER_MODEL_PROXY_RETRY_MAX_SECONDS", "2")), 0.0)
PAPER_OUTLINE_MAX_CORRECTION_ATTEMPTS = max(int(os.getenv("PAPER_OUTLINE_MAX_CORRECTION_ATTEMPTS", "3")), 1)
PAPER_LOCKED_WORD_COUNT_MIN_PERCENT = 85
PAPER_LOCKED_WORD_COUNT_MAX_PERCENT = 115
_PAPER_WORD_TOKEN_RE = re.compile(
    r"[\u3400-\u4dbf\u4e00-\u9fff]|[A-Za-z0-9]+(?:['\u2019-][A-Za-z0-9]+)*"
)
_PAPER_SENTENCE_BOUNDARY_RE = re.compile(
    r"[。！？!?]|(?<!\d)\.(?=[\"'”’）】》」』]*(?:\s|$))"
)
_PAPER_CLAUSE_BOUNDARY_RE = re.compile(r"[,，;；:：]")
_PAPER_SENTENCE_CLOSERS = "\"'”’）】》」』"
_PAPER_INCOMPLETE_TERMINAL_RE = re.compile(r"[,，;；:：]([\"'”’）】》」』]*)$")
_PAPER_REFERENCE_SOURCE_HEADING_RE = re.compile(
    r"^\s*(?:来源|source|reference)\s*\[(\d+)\]\s*[:：]?\s*$",
    re.IGNORECASE,
)
_PAPER_REFERENCE_INLINE_RE = re.compile(r"^\s*\[(\d+)\]\s*(.*?)\s*$")
_PAPER_CITATION_MARKER_RE = re.compile(r"\[\[\s*CITE\s*:\s*(\d+)\s*\]\]|\[(\d+)\]", re.IGNORECASE)
GROK_VIDEO_REFERENCE_IMAGE_MAX_COUNT = max(int(os.getenv("GROK_VIDEO_REFERENCE_IMAGE_MAX_COUNT", "7")), 1)
GROK_VIDEO_DURATION_MAX_SECONDS = float(os.getenv("GROK_VIDEO_DURATION_MAX_SECONDS", "10"))
GROK_VIDEO_EXTENSION_DURATION_MIN_SECONDS = float(os.getenv("GROK_VIDEO_EXTENSION_DURATION_MIN_SECONDS", "2"))
GROK_VIDEO_EXTENSION_DURATION_MAX_SECONDS = float(os.getenv("GROK_VIDEO_EXTENSION_DURATION_MAX_SECONDS", "10"))
GROK_VIDEO_EDIT_INPUT_MAX_SECONDS = float(os.getenv("GROK_VIDEO_EDIT_INPUT_MAX_SECONDS", "8.7"))
GROK_VIDEO_EXTENSION_INPUT_MIN_SECONDS = float(os.getenv("GROK_VIDEO_EXTENSION_INPUT_MIN_SECONDS", "2"))
GROK_VIDEO_EXTENSION_INPUT_MAX_SECONDS = float(os.getenv("GROK_VIDEO_EXTENSION_INPUT_MAX_SECONDS", "15"))
VIDEO_POLL_INTERVAL_SECONDS = max(float(os.getenv("VIDEO_POLL_INTERVAL_SECONDS", "5")), 0.2)
STREAM_PROGRESS_INTERVAL_SECONDS = max(float(os.getenv("STREAM_PROGRESS_INTERVAL_SECONDS", "1")), 0.2)
VERIFY_WORKER_SIGNATURE = os.getenv("VERIFY_WORKER_SIGNATURE", "true").lower() not in {"0", "false", "no"}
SIGNATURE_MAX_AGE_SECONDS = int(os.getenv("SIGNATURE_MAX_AGE_SECONDS", "300"))
GROK_VIDEO_SUCCESS_STATUSES = {"completed", "succeeded", "success", "done"}
GROK_VIDEO_FAILURE_STATUSES = {"failed", "error", "canceled", "cancelled", "expired"}

app = FastAPI(title="Sub2API App Worker", version=WORKER_VERSION)
run_semaphore = asyncio.Semaphore(max(MAX_CONCURRENCY, 1))
canceled_runs: set[int] = set()
active_model_proxy_tasks: dict[int, asyncio.Task[Any]] = {}


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


class WorkerCanceled(Exception):
    pass


@app.get("/")
async def root() -> dict[str, Any]:
    return await health()


@app.get("/health")
async def health() -> dict[str, Any]:
    return {
        "status": "healthy",
        "protocol": PROTOCOL,
        "version": WORKER_VERSION,
        "capabilities": [
            "text",
            "vision",
            "image_generation",
            "image_edit",
            "audio_speech",
            "audio_transcription",
            "audio_translation",
            "video_generation",
            "grok_video_generation",
            "product_marketing",
            "academic_paper",
            "workflow",
        ],
        "routes": {
            "runs": [
                "/runs",
                "/text/runs",
                "/prompt/runs",
                "/image/runs",
                "/workflow/runs",
                "/audio/runs",
                "/video/runs",
                "/grok-video/runs",
                "/product-marketing/runs",
                "/academic-paper/runs",
            ],
            "cancel": "/cancel",
        },
        "max_concurrency": MAX_CONCURRENCY,
        "metadata": {"model_proxy_required": True},
    }


@app.post("/runs")
@app.post("/text/runs")
@app.post("/prompt/runs")
@app.post("/image/runs")
@app.post("/workflow/runs")
@app.post("/audio/runs")
@app.post("/video/runs")
@app.post("/grok-video/runs")
@app.post("/product-marketing/runs")
@app.post("/academic-paper/runs")
async def submit_run(request: Request, background_tasks: BackgroundTasks) -> dict[str, Any]:
    raw_body = await request.body()
    try:
        payload = WorkerRunRequest.model_validate_json(raw_body)
    except Exception as exc:
        raise HTTPException(status_code=400, detail=f"invalid worker run payload: {exc}") from exc

    payload.metadata["worker_route"] = payload.metadata.get("worker_route") or request.url.path
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
    cancel_active_model_proxy_task(payload.run_id)
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
            if is_academic_paper_run(payload):
                await process_academic_paper_run(payload, worker_run_id, started)
                return

            if is_product_marketing_run(payload):
                await process_product_marketing_run(payload, worker_run_id, started)
                return

            prompt = extract_prompt(payload.input)
            media_policy = select_media_policy(payload)
            if media_policy is not None:
                media_kind, selected_media_policy = media_policy
                if not prompt and media_kind not in {"audio_transcription", "audio_translation"}:
                    raise WorkerFailure("INPUT_PROMPT_REQUIRED", "Please provide a prompt or text input.")
                if media_kind == "image_generation":
                    await process_image_run(payload, worker_run_id, started, selected_media_policy, prompt)
                elif media_kind == "audio_speech":
                    await process_audio_speech_run(payload, worker_run_id, started, selected_media_policy, prompt)
                elif media_kind in {"audio_transcription", "audio_translation"}:
                    await process_audio_text_run(
                        payload,
                        worker_run_id,
                        started,
                        selected_media_policy,
                        media_kind,
                    )
                elif media_kind == "video_generation":
                    if is_grok_video_request(payload, selected_media_policy):
                        await process_grok_video_run(payload, worker_run_id, started, selected_media_policy, prompt)
                    else:
                        await process_video_run(payload, worker_run_id, started, selected_media_policy, prompt)
                return

            if not prompt:
                raise WorkerFailure("INPUT_PROMPT_REQUIRED", "Please provide a prompt or text input.")

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

            last_stream_callback_at = time.monotonic()

            async def report_stream_progress(partial_text: str) -> None:
                nonlocal last_stream_callback_at
                if is_canceled(payload.run_id):
                    raise WorkerCanceled()
                now = time.monotonic()
                if now - last_stream_callback_at < STREAM_PROGRESS_INTERVAL_SECONDS:
                    return
                last_stream_callback_at = now
                await callback(
                    payload,
                    "progress",
                    status="running",
                    node_id=selected_policy.node_id,
                    role=selected_policy.role,
                    progress=0.65,
                    message="Streaming model response",
                    output={
                        "result": partial_text,
                        "model": selected_policy.model,
                        "node": selected_policy.node_id,
                        "partial": True,
                    },
                    metadata={
                        "worker_run_id": worker_run_id,
                        "policy_key": selected_policy.policy_key,
                        "model": selected_policy.model,
                        "stream": True,
                        "uses_model_proxy": True,
                    },
                )

            proxy_result = await call_model_proxy(payload, selected_policy, prompt, on_text=report_stream_progress)
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
        except WorkerCanceled:
            await callback(payload, "canceled", status="canceled", message="Run canceled")
        except WorkerFailure as exc:
            await callback_failure(payload, exc.code, exc.message)
        except Exception as exc:  # noqa: BLE001 - keep Worker callbacks robust.
            LOGGER.exception("run failed: run_id=%s", payload.run_id)
            await callback_failure(payload, "WORKER_RUNTIME_ERROR", str(exc))
        finally:
            canceled_runs.discard(payload.run_id)


async def call_academic_paper_model_proxy(
    payload: WorkerRunRequest,
    policy: SelectedPolicy,
    prompt: str,
    *,
    stage: str,
    on_text: Callable[[str], Awaitable[None]] | None = None,
) -> dict[str, Any]:
    for attempt in range(1, PAPER_MODEL_PROXY_MAX_ATTEMPTS + 1):
        ensure_run_active(payload, f"before academic paper model call ({stage})")
        try:
            return await call_model_proxy(payload, policy, prompt, on_text=on_text)
        except asyncio.CancelledError as exc:
            if is_canceled(payload.run_id):
                raise WorkerCanceled() from exc
            raise
        except WorkerCanceled:
            raise
        except Exception as exc:
            if attempt >= PAPER_MODEL_PROXY_MAX_ATTEMPTS or not is_transient_academic_paper_model_error(exc):
                raise
            delay = min(
                PAPER_MODEL_PROXY_RETRY_BASE_SECONDS * (2 ** (attempt - 1)),
                PAPER_MODEL_PROXY_RETRY_MAX_SECONDS,
            )
            LOGGER.warning(
                "academic paper model proxy transient failure: run_id=%s stage=%s attempt=%s/%s delay=%.2fs error=%s",
                payload.run_id,
                stage,
                attempt,
                PAPER_MODEL_PROXY_MAX_ATTEMPTS,
                delay,
                truncate(str(exc), 500),
            )
            if delay > 0:
                try:
                    await asyncio.sleep(delay)
                except asyncio.CancelledError as canceled:
                    if is_canceled(payload.run_id):
                        raise WorkerCanceled() from canceled
                    raise
            ensure_run_active(payload, f"before retrying academic paper model call ({stage})")
    raise RuntimeError("academic paper model proxy retry loop ended unexpectedly")


def is_transient_academic_paper_model_error(exc: Exception) -> bool:
    if isinstance(
        exc,
        (httpx.TimeoutException, httpx.NetworkError, httpx.RemoteProtocolError, httpx.ProxyError),
    ):
        return True
    if not isinstance(exc, WorkerFailure):
        return False

    status, reason, detail = academic_paper_model_error_details(exc)
    if status is not None:
        return status == 429 or 500 <= status <= 599

    transient_codes = {
        "AGENT_MODEL_PROXY_UPSTREAM_ERROR",
        "AGENT_MODEL_PROXY_REQUEST_FAILED",
        "AGENT_MODEL_PROXY_RESPONSE_READ_FAILED",
    }
    if exc.code.upper() in transient_codes or reason.upper() in transient_codes:
        return True
    if exc.code not in {"MODEL_PROXY_FAILED", "MODEL_PROXY_STREAM_FAILED"}:
        return False

    normalized = detail.lower()
    transient_markers = (
        "temporarily unavailable",
        "temporary unavailable",
        "too many requests",
        "rate limit",
        "bad gateway",
        "service unavailable",
        "gateway timeout",
        "timed out",
        "timeout",
        "connection reset",
        "connection refused",
        "network error",
        "proxy error",
        "upstream request failed",
        "client connection lost",
        "stream usage incomplete",
        "unexpected eof",
        "try again",
    )
    return any(marker in normalized for marker in transient_markers) or bool(
        re.search(r"(?<!\d)(?:429|5\d\d)(?!\d)", normalized)
    )


def academic_paper_model_error_details(exc: WorkerFailure) -> tuple[int | None, str, str]:
    parsed = parse_json_object(exc.message)
    status: int | None = None
    for value in (parsed.get("status_code"), parsed.get("status"), parsed.get("code")):
        if isinstance(value, bool):
            continue
        try:
            candidate = int(value)
        except (TypeError, ValueError):
            continue
        if 100 <= candidate <= 599:
            status = candidate
            break
    reason = clean_string(parsed.get("reason"))
    message = clean_string(parsed.get("message"))
    error = parsed.get("error")
    if isinstance(error, dict):
        reason = reason or clean_string(error.get("code") or error.get("type"))
        message = message or clean_string(error.get("message"))
    elif isinstance(error, str):
        message = message or error.strip()
    detail = " ".join(part for part in (exc.code, reason, message, exc.message) if part)
    return status, reason, detail


async def process_academic_paper_run(payload: WorkerRunRequest, worker_run_id: str, started: float) -> None:
    topic = string_input(payload.input, "topic")
    if not topic:
        raise WorkerFailure("PAPER_TOPIC_REQUIRED", "请填写论文方向或题目。")
    target_word_count = required_int_input(payload.input, "word_count", minimum=1000, maximum=50000)
    locked_outline_nodes = (
        parse_academic_paper_outline_spec(
            payload.input.get("outline_spec"),
            target_word_count,
            abstract_enabled=boolean_input(payload.input, "abstract_enabled", default=True),
        )
        if "outline_spec" in payload.input
        else None
    )
    outline_locked = locked_outline_nodes is not None

    plan_policy, write_policy = select_academic_paper_policies(payload)
    references_enabled = boolean_input(payload.input, "references_enabled", default=True)
    await callback(
        payload,
        "progress",
        status="running",
        node_id=plan_policy.node_id,
        role=plan_policy.role,
        progress=0.1,
        message="正在读取参考资料并准备论文结构" if references_enabled else "正在准备论文结构",
        metadata=model_call_metadata(plan_policy),
    )
    ensure_run_active(payload, "before reading paper references")
    reference_context = await extract_paper_reference_context(payload) if references_enabled else ""
    reference_registry = parse_paper_reference_registry(reference_context) if references_enabled else None
    if references_enabled:
        expected_reference_count = exact_paper_reference_count(payload.input)
        if expected_reference_count is not None and (
            reference_registry is None
            or len(reference_registry.get("entries") or []) != expected_reference_count
        ):
            raise WorkerFailure(
                "PAPER_REFERENCE_COUNT_MISMATCH",
                f"参考资料未能解析出用户要求的 {expected_reference_count} 条连续编号文献。",
            )
    numeric_citation_contract = reference_registry is not None and academic_paper_uses_numeric_citations(payload.input)
    if numeric_citation_contract:
        reference_context = f"{reference_context}\n\n{paper_reference_contract_context(reference_registry)}"
    template_artifact = find_paper_template_artifact(payload)
    template_bytes: bytes | None = None
    if template_artifact is not None:
        if not is_docx_artifact_ref(template_artifact):
            raise WorkerFailure("PAPER_TEMPLATE_TYPE_INVALID", "论文模板必须是 .docx 格式的 Word 文件。")
        template_bytes = await download_input_artifact(template_artifact, max_bytes=PAPER_REFERENCE_MAX_FILE_BYTES)

    await callback(
        payload,
        "progress",
        status="running",
        node_id=plan_policy.node_id,
        role=plan_policy.role,
        progress=0.18,
        message="正在规划题目、摘要和附加部分" if outline_locked else "正在规划题目、摘要和章节大纲",
        metadata={
            **model_call_metadata(plan_policy),
            "reference_context_available": bool(reference_context),
            "template_available": template_bytes is not None,
            **({"outline_locked": True} if outline_locked else {}),
        },
    )
    ensure_run_active(payload, "before paper planning")
    plan_prompt = (
        build_locked_academic_paper_plan_prompt(
            payload.input,
            target_word_count,
            reference_context,
            locked_outline_nodes,
        )
        if locked_outline_nodes is not None
        else build_academic_paper_plan_prompt(payload.input, target_word_count, reference_context)
    )
    plan_result = await call_academic_paper_model_proxy(
        payload,
        plan_policy,
        plan_prompt,
        stage="plan",
    )
    plan_text = extract_model_text(plan_result.get("response", {}))
    plan = parse_academic_paper_plan(
        plan_text,
        payload.input,
        target_word_count,
        has_reference_material=bool(reference_context or reference_image_artifacts(payload)),
        locked_outline_nodes=locked_outline_nodes,
        reference_registry=reference_registry,
    )
    planned_abstract = clean_string(plan.get("abstract"))
    if locked_outline_nodes is not None:
        rebalance_locked_outline_words_for_abstract(
            locked_outline_nodes,
            target_word_count,
            planned_abstract if boolean_input(payload.input, "abstract_enabled", default=True) else "",
        )
        plan["sections"] = build_locked_academic_paper_tree(locked_outline_nodes, {})
    sections = plan["sections"]

    section_total = len(sections)
    if locked_outline_nodes is not None:
        written_sections, writer_usages, outline_deviations = await write_locked_academic_paper_sections(
            payload,
            write_policy,
            plan,
            locked_outline_nodes,
            reference_context,
        )
    else:
        written_sections = []
        writer_usages = []
        outline_deviations = []
        for index, section in enumerate(sections, start=1):
            ensure_run_active(payload, f"before paper section {index}")
            section_title = str(section.get("title") or f"第{index}章").strip()
            await callback(
                payload,
                "progress",
                status="running",
                node_id=write_policy.node_id,
                role=write_policy.role,
                progress=0.24 + ((index - 1) / max(section_total, 1)) * 0.58,
                message=f"正在撰写第 {index}/{section_total} 章：{section_title}",
                output={"completed_sections": len(written_sections), "total_sections": section_total},
                metadata={**model_call_metadata(write_policy), "section_index": index},
            )
            section_result = await call_academic_paper_model_proxy(
                payload,
                write_policy,
                build_academic_paper_section_prompt(
                    values=payload.input,
                    plan=plan,
                    section=section,
                    section_index=index,
                    section_total=section_total,
                    completed_sections=written_sections,
                    reference_context=reference_context,
                ),
                stage=f"section-{index}",
            )
            section_text = extract_model_text(section_result.get("response", {}))
            if not section_text:
                raise WorkerFailure("PAPER_SECTION_EMPTY", f"模型未返回“{section_title}”的正文内容。")
            written_section = parse_academic_paper_section(section_text, section)
            writer_usages.append(proxy_usage(section_result))
            section_target = positive_int(section.get("target_words"))
            section_words = count_text_words(flatten_paper_section_text(written_section))
            if section_target >= 600 and section_words < section_target * 0.65:
                await callback(
                    payload,
                    "progress",
                    status="running",
                    node_id=write_policy.node_id,
                    role=write_policy.role,
                    progress=0.24 + ((index - 0.5) / max(section_total, 1)) * 0.58,
                    message=f"第 {index}/{section_total} 章篇幅不足，正在补充论证内容",
                    output={
                        "section_title": section_title,
                        "current_word_count": section_words,
                        "target_word_count": section_target,
                    },
                    metadata={**model_call_metadata(write_policy), "section_index": index, "expansion": True},
                )
                expansion_result = await call_academic_paper_model_proxy(
                    payload,
                    write_policy,
                    build_academic_paper_expansion_prompt(
                        values=payload.input,
                        plan=plan,
                        section_plan=section,
                        written_section=written_section,
                        current_word_count=section_words,
                        reference_context=reference_context,
                    ),
                    stage=f"section-{index}-expansion",
                )
                expansion_text = extract_model_text(expansion_result.get("response", {}))
                writer_usages.append(proxy_usage(expansion_result))
                if expansion_text:
                    expanded_section = parse_academic_paper_section(expansion_text, section)
                    expanded_words = count_text_words(flatten_paper_section_text(expanded_section))
                    if abs(expanded_words - section_target) < abs(section_words - section_target):
                        written_section = expanded_section
            written_sections.append(written_section)

    ensure_run_active(payload, "before paper consistency review")
    await callback(
        payload,
        "progress",
        status="running",
        node_id=write_policy.node_id,
        role=write_policy.role,
        progress=0.84,
        message="正在检查摘要、术语、章节衔接和结论一致性",
        output={"completed_sections": section_total, "total_sections": section_total},
        metadata={**model_call_metadata(write_policy), "review_scope": "consistency"},
    )
    review_result = await call_academic_paper_model_proxy(
        payload,
        write_policy,
        (
            build_locked_academic_paper_review_prompt(payload.input, plan, written_sections)
            if outline_locked
            else build_academic_paper_review_prompt(payload.input, plan, written_sections)
        ),
        stage="review",
    )
    review = parse_json_object(extract_model_text(review_result.get("response", {})))
    if outline_locked:
        review.pop("conclusion_adjustments", None)
    consistency_notes = apply_academic_paper_review(plan, written_sections, review, payload.input)
    if locked_outline_nodes is not None and boolean_input(payload.input, "abstract_enabled", default=True):
        reviewed_total = count_academic_paper_words({"abstract": plan.get("abstract"), "sections": written_sections})
        planned_total = count_academic_paper_words({"abstract": planned_abstract, "sections": written_sections})
        if (
            not paper_word_count_within_tolerance(reviewed_total, target_word_count)
            and abs(planned_total - target_word_count) < abs(reviewed_total - target_word_count)
        ):
            plan["abstract"] = planned_abstract
            consistency_notes.append("终审摘要会使总字数超出容差，已保留规划阶段摘要。")

    if locked_outline_nodes is not None and not locked_academic_paper_outline_matches(
        written_sections,
        locked_outline_nodes,
    ):
        raise WorkerFailure("PAPER_OUTLINE_LOCK_BROKEN", "论文目录结构在生成过程中发生变化，已停止输出。")

    citation_contract: dict[str, Any] = {
        "reference_contract_valid": None,
        "reference_registry_locked": reference_registry is not None,
        "citation_contract_enforced": numeric_citation_contract,
        "citation_ids_used": [],
        "citation_ids_missing": [],
        "citation_ids_unknown": [],
        "citation_markers_malformed": False,
        "reference_contract_repaired": False,
    }
    if numeric_citation_contract and reference_registry is not None:
        citation_contract = enforce_academic_paper_citation_contract(written_sections, reference_registry)
        citation_contract.update(
            {
                "reference_registry_locked": True,
                "citation_contract_enforced": True,
            }
        )
        if (
            not citation_contract["reference_contract_valid"]
            and (citation_contract["citation_ids_missing"] or citation_contract["citation_ids_unknown"])
            and not citation_contract["citation_markers_malformed"]
        ):
            ensure_run_active(payload, "before paper citation repair")
            await callback(
                payload,
                "progress",
                status="running",
                node_id=write_policy.node_id,
                role=write_policy.role,
                progress=0.87,
                message="正在补齐正文引用位置并校验正文未被改写",
                output={"missing_citation_ids": citation_contract["citation_ids_missing"]},
                metadata={**model_call_metadata(write_policy), "review_scope": "citation_contract"},
            )
            citation_repair_result = await call_academic_paper_model_proxy(
                payload,
                write_policy,
                build_academic_paper_citation_repair_prompt(
                    written_sections,
                    reference_registry,
                    citation_contract["citation_ids_missing"],
                    citation_contract["citation_ids_unknown"],
                ),
                stage="citation-repair",
            )
            apply_academic_paper_citation_repair(
                extract_model_text(citation_repair_result.get("response", {})),
                written_sections,
            )
            writer_usages.append(proxy_usage(citation_repair_result))
            citation_contract = enforce_academic_paper_citation_contract(written_sections, reference_registry)
            citation_contract.update(
                {
                    "reference_contract_repaired": True,
                    "reference_registry_locked": True,
                    "citation_contract_enforced": True,
                }
            )
        if not citation_contract["reference_contract_valid"]:
            missing = ",".join(str(value) for value in citation_contract["citation_ids_missing"]) or "无"
            unknown = ",".join(str(value) for value in citation_contract["citation_ids_unknown"]) or "无"
            raise WorkerFailure(
                "PAPER_REFERENCE_CONTRACT_INVALID",
                f"正文引用契约校验失败：缺失编号 {missing}，未知编号 {unknown}；已停止生成 Word 文件。",
            )

    ensure_run_active(payload, "before Word document generation")
    paper = build_academic_paper_payload(plan, written_sections, payload.input)
    format_settings = academic_paper_format_settings(payload.input)
    actual_word_count = count_academic_paper_words(paper)
    await callback(
        payload,
        "progress",
        status="running",
        node_id=write_policy.node_id,
        role=write_policy.role,
        progress=0.9,
        message="正文已完成，正在排版并生成 Word 文件",
        output={"completed_sections": section_total, "total_sections": section_total},
        metadata={"workflow": "academic_paper", "actual_word_count": actual_word_count},
    )
    try:
        document_bytes = await asyncio.to_thread(
            build_academic_paper_docx,
            paper,
            format_settings,
            template_bytes=template_bytes,
        )
    except WorkerFailure:
        raise
    except Exception as exc:
        raise WorkerFailure("PAPER_DOCX_GENERATION_FAILED", f"Word 文档生成失败：{truncate(str(exc), 500)}") from exc

    document_name = "academic-paper.docx"
    document_mime = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
    artifact = await upload_artifact_bytes(
        payload,
        name=document_name,
        raw=document_bytes,
        mime_type=document_mime,
        metadata={
            "workflow": "academic_paper",
            "artifact_role": "paper_document",
            "title": paper["title"],
            "target_word_count": target_word_count,
            "actual_word_count": actual_word_count,
            "section_count": section_total,
        },
    )
    deviation_percent = round(((actual_word_count - target_word_count) / target_word_count) * 100, 1)
    quality_report = {
        "target_word_count": target_word_count,
        "actual_word_count": actual_word_count,
        "deviation_percent": deviation_percent,
        "section_count": section_total,
        "reference_count": count_paper_references(paper.get("references")),
        "consistency_notes": consistency_notes,
        **citation_contract,
    }
    if locked_outline_nodes is not None:
        quality_report.update(
            {
                "outline_locked": True,
                "outline_node_count": len(locked_outline_nodes),
                "outline_match": True,
                "outline_deviations": outline_deviations,
            }
        )
    result_text = (
        f"论文《{paper['title']}》已生成。\n"
        f"目标字数：{target_word_count}\n"
        f"实际字数：{actual_word_count}\n"
        f"章节数：{section_total}\n"
        "Word 文件已生成，可在结果文件中在线下载。"
    )
    await callback(
        payload,
        "succeeded",
        status="succeeded",
        node_id=write_policy.node_id,
        role=write_policy.role,
        progress=1.0,
        message="论文 Word 文件已生成",
        output={
            "result": result_text,
            "document": public_document_artifact(artifact, document_name, document_mime, len(document_bytes)),
            "word_count": actual_word_count,
            "quality_report": quality_report,
        },
        metadata={
            "workflow": "academic_paper",
            "worker_run_id": worker_run_id,
            "duration_ms": int((time.perf_counter() - started) * 1000),
            "plan_usage": proxy_usage(plan_result),
            "writer_usages": writer_usages,
            "review_usage": proxy_usage(review_result),
        },
    )


async def write_locked_academic_paper_sections(
    payload: WorkerRunRequest,
    write_policy: SelectedPolicy,
    plan: dict[str, Any],
    outline_nodes: list[dict[str, Any]],
    reference_context: str,
) -> tuple[list[dict[str, Any]], list[dict[str, Any]], list[dict[str, Any]]]:
    node_groups = locked_outline_top_level_groups(outline_nodes)
    content_by_id: dict[str, str] = {}
    writer_usages: list[dict[str, Any]] = []
    section_total = len(node_groups)

    for index, group in enumerate(node_groups, start=1):
        ensure_run_active(payload, f"before locked paper section {index}")
        section_title = group[0]["title"]
        await callback(
            payload,
            "progress",
            status="running",
            node_id=write_policy.node_id,
            role=write_policy.role,
            progress=0.24 + ((index - 1) / max(section_total, 1)) * 0.52,
            message=f"正在按锁定目录撰写第 {index}/{section_total} 章：{section_title}",
            output={"completed_sections": index - 1, "total_sections": section_total},
            metadata={
                **model_call_metadata(write_policy),
                "section_index": index,
                "outline_locked": True,
                "outline_node_ids": [node["id"] for node in group],
            },
        )
        section_result = await call_academic_paper_model_proxy(
            payload,
            write_policy,
            build_locked_academic_paper_section_prompt(
                values=payload.input,
                plan=plan,
                outline_nodes=outline_nodes,
                section_nodes=group,
                section_index=index,
                section_total=section_total,
                completed_content=content_by_id,
                reference_context=reference_context,
            ),
            stage=f"locked-section-{index}",
        )
        writer_usages.append(proxy_usage(section_result))
        content_by_id.update(
            parse_locked_academic_paper_contents(
                extract_model_text(section_result.get("response", {})),
                {node["id"] for node in group},
            )
        )

    missing_nodes = [node for node in outline_nodes if node["id"] not in content_by_id]
    if missing_nodes:
        ensure_run_active(payload, "before locked outline missing-node retry")
        await callback(
            payload,
            "progress",
            status="running",
            node_id=write_policy.node_id,
            role=write_policy.role,
            progress=0.78,
            message=f"目录中有 {len(missing_nodes)} 个节点缺少正文，正在补写一次",
            output={"missing_node_ids": [node["id"] for node in missing_nodes]},
            metadata={**model_call_metadata(write_policy), "outline_locked": True, "missing_retry": True},
        )
        retry_result = await call_academic_paper_model_proxy(
            payload,
            write_policy,
            build_locked_academic_paper_missing_prompt(
                values=payload.input,
                plan=plan,
                missing_nodes=missing_nodes,
                completed_content=content_by_id,
                reference_context=reference_context,
            ),
            stage="locked-missing-nodes",
        )
        writer_usages.append(proxy_usage(retry_result))
        content_by_id.update(
            parse_locked_academic_paper_contents(
                extract_model_text(retry_result.get("response", {})),
                {node["id"] for node in missing_nodes},
            )
        )
        missing_nodes = [node for node in outline_nodes if node["id"] not in content_by_id]
        if missing_nodes:
            missing_ids = ", ".join(node["id"] for node in missing_nodes[:10])
            if len(missing_nodes) > 10:
                missing_ids += " 等"
            raise WorkerFailure(
                "PAPER_OUTLINE_CONTENT_MISSING",
                f"模型补写后仍有目录节点缺少正文：{missing_ids}。",
            )

    for index, group in enumerate(node_groups, start=1):
        target_words = sum(positive_int(node.get("target_words")) for node in group)
        actual_words = locked_outline_group_word_count(group, content_by_id)
        for correction_attempt in range(1, PAPER_OUTLINE_MAX_CORRECTION_ATTEMPTS + 1):
            if target_words <= 0 or paper_word_count_within_tolerance(actual_words, target_words):
                break
            ensure_run_active(payload, f"before locked paper section {index} length correction")
            await callback(
                payload,
                "progress",
                status="running",
                node_id=write_policy.node_id,
                role=write_policy.role,
                progress=0.79 + (index / max(section_total, 1)) * 0.04,
                message=(
                    f"第 {index}/{section_total} 章篇幅偏离目标，"
                    f"正在进行第 {correction_attempt}/{PAPER_OUTLINE_MAX_CORRECTION_ATTEMPTS} 次校正"
                ),
                output={
                    "section_node_id": group[0]["id"],
                    "current_word_count": actual_words,
                    "target_word_count": target_words,
                    "correction_attempt": correction_attempt,
                },
                metadata={
                    **model_call_metadata(write_policy),
                    "section_index": index,
                    "outline_locked": True,
                    "length_correction": True,
                    "correction_attempt": correction_attempt,
                },
            )
            correction_result = await call_academic_paper_model_proxy(
                payload,
                write_policy,
                build_locked_academic_paper_correction_prompt(
                    values=payload.input,
                    plan=plan,
                    section_nodes=group,
                    completed_content=content_by_id,
                    current_word_count=actual_words,
                    target_word_count=target_words,
                    reference_context=reference_context,
                ),
                stage=f"locked-section-{index}-correction-{correction_attempt}",
            )
            writer_usages.append(proxy_usage(correction_result))
            content_by_id.update(
                parse_locked_academic_paper_contents(
                    extract_model_text(correction_result.get("response", {})),
                    {node["id"] for node in group},
                )
            )
            actual_words = locked_outline_group_word_count(group, content_by_id)

        if target_words > 0:
            actual_words = hard_cap_locked_outline_group_words(group, content_by_id)

    sections = build_locked_academic_paper_tree(outline_nodes, content_by_id)
    deviations = locked_outline_word_count_deviations(node_groups, content_by_id)
    return sections, writer_usages, deviations


async def process_product_marketing_run(payload: WorkerRunRequest, worker_run_id: str, started: float) -> None:
    product_name = string_input(payload.input, "product_name")
    selling_points = string_input(payload.input, "selling_points") or string_input(payload.input, "selling")
    if not product_name:
        raise WorkerFailure("PRODUCT_NAME_REQUIRED", "Please provide a product name.")
    if not selling_points:
        raise WorkerFailure("SELLING_POINTS_REQUIRED", "Please provide the product selling points.")

    output_count = bounded_int_input(payload.input, "output_count", default=3, minimum=1, maximum=4)
    analysis_policy = find_policy(
        payload,
        capabilities={"vision", "text", "model"},
        roles={"analyze", "marketing", "generate"},
    )
    image_policy = find_policy(
        payload,
        capabilities={"image", "image_generation", "image_edit", "text_to_image", "image_to_image"},
        roles={"generate"},
    )
    if analysis_policy is None:
        raise WorkerFailure("MARKETING_POLICY_REQUIRED", "Product marketing requires a text or vision model policy.")
    if image_policy is None:
        raise WorkerFailure("IMAGE_POLICY_REQUIRED", "Product marketing requires an image generation model policy.")

    await callback(
        payload,
        "progress",
        status="running",
        node_id=analysis_policy.node_id,
        role=analysis_policy.role,
        progress=0.12,
        message="Analyzing product and planning the marketing package",
        metadata=model_call_metadata(analysis_policy),
    )
    ensure_run_active(payload, "before product analysis")
    analysis_result = await call_model_proxy(payload, analysis_policy, build_product_marketing_prompt(payload.input, output_count))
    analysis_text = extract_model_text(analysis_result.get("response", {}))
    if not analysis_text:
        raise WorkerFailure("MARKETING_PLAN_EMPTY", "The marketing model returned no usable plan.")
    plan = parse_product_marketing_plan(analysis_text, payload.input, output_count)

    ensure_run_active(payload, "before image generation")

    references = reference_image_artifacts(payload)
    reference_bodies = await download_reference_images(references) if references else []
    image_artifacts: list[dict[str, Any]] = []
    image_usages: list[dict[str, Any]] = []
    prompts = plan["image_prompts"]
    for index, image_prompt in enumerate(prompts, start=1):
        ensure_run_active(payload, f"before image {index}")
        await callback(
            payload,
            "progress",
            status="running",
            node_id=image_policy.node_id,
            role=image_policy.role,
            progress=0.25 + (index - 1) * (0.65 / output_count),
            message=f"Generating marketing image {index} of {output_count}",
            output={"completed_images": len(image_artifacts), "total_images": output_count},
            metadata={**model_call_metadata(image_policy), "image_index": index, "reference_count": len(references)},
        )
        proxy_result = await call_image_model_proxy(
            payload,
            image_policy,
            image_prompt,
            references=references,
            reference_bodies=reference_bodies,
        )
        image = extract_image_result(proxy_result.get("response", {}))
        if not image:
            raise WorkerFailure("IMAGE_RESULT_EMPTY", f"Image model returned no image for output {index}.")
        artifact_metadata = {
            "workflow": "product_marketing",
            "product_name": product_name,
            "artifact_role": "marketing_image",
            "image_index": index,
            "prompt": image_prompt,
            "reference_count": len(references),
        }
        if image.get("url"):
            artifact = await archive_remote_artifact(
                payload,
                name=f"product-marketing-{payload.run_id}-{index}.png",
                url=str(image["url"]),
                mime_type=str(image.get("mime_type") or "image/png"),
                metadata=artifact_metadata,
            )
        else:
            artifact = await upload_base64_artifact(
                payload,
                name=f"product-marketing-{payload.run_id}-{index}.png",
                b64_json=str(image["b64_json"]),
                mime_type=str(image.get("mime_type") or "image/png"),
                metadata=artifact_metadata,
            )
        image_artifacts.append(artifact)
        image_usages.append(proxy_usage(proxy_result))

    ensure_run_active(payload, "after image generation")
    await callback(
        payload,
        "succeeded",
        status="succeeded",
        node_id=image_policy.node_id,
        role=image_policy.role,
        progress=1.0,
        message="Product marketing package completed",
        output={
            "result": format_product_marketing_result(plan, product_name),
            "marketing_plan": plan,
            "image_count": len(image_artifacts),
        },
        metadata={
            "workflow": "product_marketing",
            "worker_run_id": worker_run_id,
            "duration_ms": int((time.perf_counter() - started) * 1000),
            "analysis_usage": proxy_usage(analysis_result),
            "image_usages": image_usages,
            "reference_count": len(references),
        },
    )


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

    references = reference_image_artifacts(payload)
    reference_bodies: list[bytes] = []
    generation_mode = "image_to_image" if references else "text_to_image"
    if references:
        await callback(
            payload,
            "progress",
            status="running",
            node_id=image_policy.node_id,
            role=image_policy.role,
            progress=0.45,
            message=f"Preparing {len(references)} reference image(s)",
            metadata={
                "policy_key": image_policy.policy_key,
                "model": image_policy.model,
                "generation_mode": generation_mode,
                "reference_count": len(references),
                "reference_artifact_ids": [reference.artifact_id for reference in references if reference.artifact_id is not None],
            },
        )
        reference_bodies = await download_reference_images(references)
        if is_canceled(payload.run_id):
            await callback(payload, "canceled", status="canceled", message="Run canceled before image editing")
            return

    await callback(
        payload,
        "progress",
        status="running",
        node_id=image_policy.node_id,
        role=image_policy.role,
        progress=0.6,
        message=f"正在基于 {len(references)} 张参考图生成图片" if references else "正在生成图片",
        metadata={
            "policy_key": image_policy.policy_key,
            "model": image_policy.model,
            "generation_mode": generation_mode,
            "reference_count": len(references),
            "uses_model_proxy": True,
        },
    )
    if is_canceled(payload.run_id):
        await callback(payload, "canceled", status="canceled", message="Run canceled before image generation")
        return

    proxy_result = await call_image_model_proxy(
        payload,
        image_policy,
        final_prompt,
        references=references,
        reference_bodies=reference_bodies,
    )
    if is_canceled(payload.run_id):
        await callback(payload, "canceled", status="canceled", message="Run canceled after image generation")
        return
    image = extract_image_result(proxy_result.get("response", {}))
    if not image:
        raise WorkerFailure("IMAGE_RESULT_EMPTY", "The image model returned no image URL or base64 data.")

    artifact_name = f"generated-{payload.run_id}.png"
    artifact_metadata = {
        "worker_run_id": worker_run_id,
        "policy_key": image_policy.policy_key,
        "model": image_policy.model,
        "prompt": final_prompt,
        "generation_mode": generation_mode,
        "reference_count": len(references),
    }
    if references:
        artifact_metadata["reference_artifact_ids"] = [reference.artifact_id for reference in references if reference.artifact_id is not None]
        artifact_metadata["reference_names"] = [reference.name for reference in references]
        if len(references) == 1:
            artifact_metadata["reference_artifact_id"] = references[0].artifact_id
            artifact_metadata["reference_name"] = references[0].name
    artifact: dict[str, Any]
    if image.get("url"):
        artifact = await archive_remote_artifact(
            payload,
            name=artifact_name,
            url=str(image["url"]),
            mime_type=str(image.get("mime_type") or "image/png"),
            metadata=artifact_metadata,
        )
    else:
        artifact = await upload_base64_artifact(
            payload,
            name=artifact_name,
            b64_json=str(image["b64_json"]),
            mime_type=str(image.get("mime_type") or "image/png"),
            metadata=artifact_metadata,
        )
    if is_canceled(payload.run_id):
        await callback(payload, "canceled", status="canceled", message="Run canceled after image archival")
        return

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
            "generation_mode": generation_mode,
            "reference_count": len(references),
            "artifact": artifact,
        },
        metadata={
            "worker_run_id": worker_run_id,
            "policy_key": image_policy.policy_key,
            "model": image_policy.model,
            "duration_ms": duration_ms,
            "usage": usage,
            "generation_mode": generation_mode,
            "reference_count": len(references),
            "uses_model_proxy": True,
        },
    )


async def process_audio_speech_run(
    payload: WorkerRunRequest,
    worker_run_id: str,
    started: float,
    policy: SelectedPolicy,
    prompt: str,
) -> None:
    await callback(
        payload,
        "progress",
        status="running",
        node_id=policy.node_id,
        role=policy.role,
        progress=0.45,
        message="Generating audio",
        metadata=model_call_metadata(policy),
    )
    if is_canceled(payload.run_id):
        await callback(payload, "canceled", status="canceled", message="Run canceled before audio generation")
        return

    request_body: dict[str, Any] = {
        "input": prompt,
        "voice": string_input(payload.input, "voice") or "alloy",
    }
    copy_input_fields(payload.input, request_body, "response_format", "speed", "instructions")
    proxy_result = await call_model_proxy_request(
        payload,
        policy,
        endpoint="/v1/audio/speech",
        request_body=request_body,
    )
    if is_canceled(payload.run_id):
        await callback(payload, "canceled", status="canceled", message="Run canceled after audio generation")
        return
    body_base64 = str(proxy_result.get("body_base64") or "").strip()
    if not body_base64:
        raise WorkerFailure("AUDIO_RESULT_EMPTY", "The audio model returned no audio data.")

    mime_type = normalized_content_type(proxy_result.get("content_type")) or audio_mime_from_format(
        string_input(payload.input, "response_format")
    )
    artifact_name = proxy_artifact_name(proxy_result, f"speech-{payload.run_id}", mime_type)
    artifact = await upload_base64_artifact(
        payload,
        name=artifact_name,
        b64_json=body_base64,
        mime_type=mime_type,
        metadata={
            **model_call_metadata(policy),
            "worker_run_id": worker_run_id,
            "media_type": "audio",
        },
    )
    if is_canceled(payload.run_id):
        await callback(payload, "canceled", status="canceled", message="Run canceled after audio archival")
        return
    usage = proxy_usage(proxy_result)
    await callback(
        payload,
        "succeeded",
        status="succeeded",
        node_id=policy.node_id,
        role=policy.role,
        progress=1.0,
        message="Audio generated",
        output={"result": "Audio generated", "artifact": artifact},
        metadata=run_completion_metadata(worker_run_id, policy, started, usage),
    )


async def process_audio_text_run(
    payload: WorkerRunRequest,
    worker_run_id: str,
    started: float,
    policy: SelectedPolicy,
    media_kind: str,
) -> None:
    source = next((artifact for artifact in input_artifacts(payload) if is_audio_artifact_ref(artifact)), None)
    if source is None:
        raise WorkerFailure("AUDIO_INPUT_REQUIRED", "Please upload an audio file for transcription or translation.")

    await callback(
        payload,
        "progress",
        status="running",
        node_id=policy.node_id,
        role=policy.role,
        progress=0.35,
        message="Preparing audio input",
        metadata=model_call_metadata(policy),
    )
    raw = await download_input_artifact(source)
    if is_canceled(payload.run_id):
        await callback(payload, "canceled", status="canceled", message="Run canceled before audio processing")
        return

    request_body: dict[str, Any] = {}
    copy_input_fields(payload.input, request_body, "language", "prompt", "response_format", "temperature")
    endpoint = "/v1/audio/translations" if media_kind == "audio_translation" else "/v1/audio/transcriptions"
    proxy_result = await call_model_proxy_request(
        payload,
        policy,
        endpoint=endpoint,
        request_body=request_body,
        content_type="multipart/form-data",
        multipart=[
            {
                "name": "file",
                "filename": source.name or f"audio-{payload.run_id}",
                "content_type": artifact_ref_mime(source) or "application/octet-stream",
                "body_base64": base64.b64encode(raw).decode("ascii"),
            }
        ],
    )
    if is_canceled(payload.run_id):
        await callback(payload, "canceled", status="canceled", message="Run canceled after audio processing")
        return
    text = extract_model_text(proxy_result.get("response", {}))
    if not text:
        raise WorkerFailure("AUDIO_TEXT_RESULT_EMPTY", "The audio model returned no transcript text.")

    usage = proxy_usage(proxy_result)
    await callback(
        payload,
        "succeeded",
        status="succeeded",
        node_id=policy.node_id,
        role=policy.role,
        progress=1.0,
        message="Audio processed",
        output={"result": text, "source": source.name},
        metadata=run_completion_metadata(worker_run_id, policy, started, usage),
    )


async def process_video_run(
    payload: WorkerRunRequest,
    worker_run_id: str,
    started: float,
    policy: SelectedPolicy,
    prompt: str,
) -> None:
    await callback(
        payload,
        "progress",
        status="running",
        node_id=policy.node_id,
        role=policy.role,
        progress=0.25,
        message="Starting video generation",
        metadata=model_call_metadata(policy),
    )
    if is_canceled(payload.run_id):
        await callback(payload, "canceled", status="canceled", message="Run canceled before video generation")
        return

    request_body: dict[str, Any] = {"prompt": prompt}
    copy_input_fields(payload.input, request_body, "quality")
    copy_alias_input_field(payload.input, request_body, "seconds", "duration")
    copy_alias_input_field(payload.input, request_body, "size", "resolution")
    reference = next((artifact for artifact in input_artifacts(payload) if is_image_artifact_ref(artifact)), None)
    multipart: list[dict[str, Any]] | None = None
    if reference is not None:
        raw_reference = await download_input_artifact(reference)
        if is_canceled(payload.run_id):
            await callback(payload, "canceled", status="canceled", message="Run canceled before video generation")
            return
        multipart = [
            {
                "name": "input_reference",
                "filename": reference.name or f"video-reference-{payload.run_id}",
                "content_type": artifact_ref_mime(reference) or "application/octet-stream",
                "body_base64": base64.b64encode(raw_reference).decode("ascii"),
            }
        ]
    proxy_result = await call_model_proxy_request(
        payload,
        policy,
        endpoint="/v1/videos",
        request_body=request_body,
        content_type="multipart/form-data" if multipart else "",
        multipart=multipart,
    )
    if is_canceled(payload.run_id):
        await callback(payload, "canceled", status="canceled", message="Run canceled after video creation")
        return
    usage = proxy_usage(proxy_result)
    response = proxy_result.get("response", {})
    if not isinstance(response, dict):
        response = {}
    video_id = first_string(response, "id", "video_id", "request_id")
    status = first_string(response, "status").lower()
    if not video_id:
        raise WorkerFailure("VIDEO_REQUEST_ID_MISSING", "The video model returned no request ID.")

    video_timeout_seconds = payload.timeout_seconds if payload.timeout_seconds > 0 else 600
    deadline = time.monotonic() + max(video_timeout_seconds, 10)
    while status not in {"completed", "succeeded", "success", "failed", "error", "canceled", "cancelled"}:
        if is_canceled(payload.run_id):
            await callback(payload, "canceled", status="canceled", message="Run canceled during video generation")
            return
        if time.monotonic() >= deadline:
            raise WorkerFailure("VIDEO_GENERATION_TIMEOUT", "Video generation did not finish before the Worker timeout.")
        await callback(
            payload,
            "progress",
            status="running",
            node_id=policy.node_id,
            role=policy.role,
            progress=0.65,
            message="Waiting for video generation",
            metadata={**model_call_metadata(policy), "video_id": video_id, "video_status": status or "queued"},
        )
        await asyncio.sleep(VIDEO_POLL_INTERVAL_SECONDS)
        if is_canceled(payload.run_id):
            await callback(payload, "canceled", status="canceled", message="Run canceled during video generation")
            return
        status_result = await call_model_proxy_request(
            payload,
            policy,
            endpoint=f"/v1/videos/{video_id}",
            method="GET",
        )
        current = status_result.get("response", {})
        if isinstance(current, dict):
            response = current
        status = first_string(response, "status").lower()
        proxy_result = status_result

    if status in {"failed", "error", "canceled", "cancelled"}:
        message = first_string(response, "error", "message") or f"Video generation ended with status {status}."
        raise WorkerFailure("VIDEO_GENERATION_FAILED", message)
    if is_canceled(payload.run_id):
        await callback(payload, "canceled", status="canceled", message="Run canceled before video download")
        return

    artifact_metadata = {
        **model_call_metadata(policy),
        "worker_run_id": worker_run_id,
        "media_type": "video",
        "video_id": video_id,
    }
    completed_media = extract_media_result(response, "video/mp4")
    if completed_media and completed_media.get("b64_json"):
        artifact = await upload_base64_artifact(
            payload,
            name=f"video-{payload.run_id}.mp4",
            b64_json=str(completed_media["b64_json"]),
            mime_type=str(completed_media.get("mime_type") or "video/mp4"),
            metadata=artifact_metadata,
        )
    else:
        try:
            content_result = await call_model_proxy_request(
                payload,
                policy,
                endpoint=f"/v1/videos/{video_id}/content",
                method="GET",
            )
        except WorkerCanceled:
            await callback(payload, "canceled", status="canceled", message="Run canceled during model stream")
            return
        except WorkerFailure as exc:
            if exc.code != "MODEL_PROXY_FAILED" or not completed_media or not completed_media.get("url"):
                raise
            artifact = await archive_remote_artifact(
                payload,
                name=f"video-{payload.run_id}.mp4",
                url=str(completed_media["url"]),
                mime_type=str(completed_media.get("mime_type") or "video/mp4"),
                metadata=artifact_metadata,
            )
        else:
            try:
                artifact = await archive_proxy_media_result(
                    payload,
                    content_result,
                    default_name=f"video-{payload.run_id}.mp4",
                    default_mime="video/mp4",
                    metadata=artifact_metadata,
                )
            except WorkerFailure as exc:
                if exc.code != "MEDIA_RESULT_EMPTY" or not completed_media or not completed_media.get("url"):
                    raise
                artifact = await archive_remote_artifact(
                    payload,
                    name=f"video-{payload.run_id}.mp4",
                    url=str(completed_media["url"]),
                    mime_type=str(completed_media.get("mime_type") or "video/mp4"),
                    metadata=artifact_metadata,
                )
    if is_canceled(payload.run_id):
        await callback(payload, "canceled", status="canceled", message="Run canceled after video archival")
        return
    await callback(
        payload,
        "succeeded",
        status="succeeded",
        node_id=policy.node_id,
        role=policy.role,
        progress=1.0,
        message="Video generated",
        output={"result": "Video generated", "video_id": video_id, "artifact": artifact},
        metadata=run_completion_metadata(worker_run_id, policy, started, usage),
    )


async def process_grok_video_run(
    payload: WorkerRunRequest,
    worker_run_id: str,
    started: float,
    policy: SelectedPolicy,
    prompt: str,
) -> None:
    mode = grok_video_mode(payload)
    effective_policy = select_grok_video_mode_policy(payload, mode, policy)
    await callback(
        payload,
        "progress",
        status="running",
        node_id=effective_policy.node_id,
        role=effective_policy.role,
        progress=0.2,
        message=f"Starting Grok video job ({mode})",
        metadata={**model_call_metadata(effective_policy), "generation_mode": mode},
    )
    if is_canceled(payload.run_id):
        await callback(payload, "canceled", status="canceled", message="Run canceled before Grok video generation")
        return

    endpoint, request_body, source_metadata = await build_grok_video_request(payload, mode, prompt)
    if is_canceled(payload.run_id):
        await callback(payload, "canceled", status="canceled", message="Run canceled before Grok video request")
        return

    await callback(
        payload,
        "progress",
        status="running",
        node_id=effective_policy.node_id,
        role=effective_policy.role,
        progress=0.45,
        message="Calling Grok video model",
        metadata={
            **model_call_metadata(effective_policy),
            "generation_mode": mode,
            "grok_video_endpoint": endpoint,
            **source_metadata,
        },
    )
    proxy_result = await call_model_proxy_request(
        payload,
        effective_policy,
        endpoint=endpoint,
        request_body=request_body,
    )
    if is_canceled(payload.run_id):
        await callback(payload, "canceled", status="canceled", message="Run canceled after Grok video request")
        return

    usage = proxy_usage(proxy_result)
    response = proxy_result.get("response", {})
    if not isinstance(response, dict):
        response = {}
    request_id = grok_video_request_id(response)
    if not request_id:
        raise WorkerFailure("GROK_VIDEO_REQUEST_ID_MISSING", "The Grok video model returned no request ID.")

    try:
        response, usage = await poll_grok_video_response(
            payload,
            effective_policy,
            request_id,
            response,
            usage,
            mode,
        )
    except WorkerCanceled:
        return
    if is_canceled(payload.run_id):
        await callback(payload, "canceled", status="canceled", message="Run canceled before Grok video download")
        return

    artifact_metadata = {
        **model_call_metadata(effective_policy),
        "worker_run_id": worker_run_id,
        "media_type": "video",
        "generation_mode": mode,
        "grok_video_request_id": request_id,
        "prompt": prompt,
        **source_metadata,
    }
    artifact = await archive_grok_video_result(
        payload,
        response,
        name=f"grok-video-{payload.run_id}.mp4",
        metadata=artifact_metadata,
    )
    if is_canceled(payload.run_id):
        await callback(payload, "canceled", status="canceled", message="Run canceled after Grok video archival")
        return

    await callback(
        payload,
        "succeeded",
        status="succeeded",
        node_id=effective_policy.node_id,
        role=effective_policy.role,
        progress=1.0,
        message="Grok video generated",
        output={
            "result": "Grok video generated",
            "prompt": prompt,
            "generation_mode": mode,
            "video_id": request_id,
            "artifact": artifact,
        },
        metadata={
            **run_completion_metadata(worker_run_id, effective_policy, started, usage),
            "generation_mode": mode,
            "grok_video_request_id": request_id,
            **source_metadata,
        },
    )


async def build_grok_video_request(
    payload: WorkerRunRequest,
    mode: str,
    prompt: str,
) -> tuple[str, dict[str, Any], dict[str, Any]]:
    images = [artifact for artifact in input_artifacts(payload) if is_image_artifact_ref(artifact)]
    videos = [artifact for artifact in input_artifacts(payload) if is_video_artifact_ref(artifact)]
    source_video_url = grok_source_video_url(payload)
    has_video_input = bool(videos or source_video_url)
    source_metadata: dict[str, Any] = {
        "input_image_count": len(images),
        "input_video_count": len(videos) + (1 if source_video_url and not videos else 0),
    }

    if mode == "text_to_video":
        if images or has_video_input:
            raise WorkerFailure(
                "GROK_VIDEO_INPUT_MISMATCH",
                "text_to_video mode does not accept image or video input.",
            )
        request_body = {"prompt": prompt}
        apply_grok_video_generation_options(payload.input, request_body, max_duration=GROK_VIDEO_DURATION_MAX_SECONDS)
        return "/v1/videos/generations", request_body, source_metadata

    if mode == "image_to_video":
        if has_video_input:
            raise WorkerFailure("GROK_VIDEO_INPUT_MISMATCH", "image_to_video mode does not accept video input.")
        source_image = select_grok_source_image(payload, images)
        if source_image is None:
            raise WorkerFailure("GROK_VIDEO_IMAGE_REQUIRED", "image_to_video mode requires one source image.")
        if len(images) > 1:
            raise WorkerFailure(
                "GROK_VIDEO_INPUT_MISMATCH",
                "image_to_video mode accepts one source image. Use reference_to_video for multiple reference images.",
            )
        request_body = {
            "prompt": prompt,
            "image": {"image_url": await artifact_to_data_url(source_image, max_bytes=MAX_IMAGE_REFERENCE_BYTES)},
        }
        source_metadata.update(
            {
                "source_image_artifact_id": source_image.artifact_id,
                "source_image_name": source_image.name,
            }
        )
        apply_grok_video_generation_options(payload.input, request_body, max_duration=GROK_VIDEO_DURATION_MAX_SECONDS)
        return "/v1/videos/generations", request_body, source_metadata

    if mode == "reference_to_video":
        if has_video_input:
            raise WorkerFailure("GROK_VIDEO_INPUT_MISMATCH", "reference_to_video mode does not accept video input.")
        references = select_grok_reference_images(payload, images)
        if not references:
            raise WorkerFailure("GROK_VIDEO_REFERENCES_REQUIRED", "reference_to_video mode requires reference images.")
        if len(references) > GROK_VIDEO_REFERENCE_IMAGE_MAX_COUNT:
            raise WorkerFailure(
                "GROK_VIDEO_REFERENCE_COUNT_EXCEEDED",
                f"At most {GROK_VIDEO_REFERENCE_IMAGE_MAX_COUNT} reference images are supported for Grok reference_to_video.",
            )
        request_body = {
            "prompt": prompt,
            "images": [
                {"image_url": await artifact_to_data_url(reference, max_bytes=MAX_IMAGE_REFERENCE_BYTES)}
                for reference in references
            ],
        }
        source_metadata.update(
            {
                "reference_count": len(references),
                "reference_artifact_ids": [item.artifact_id for item in references if item.artifact_id is not None],
                "reference_names": [item.name for item in references],
            }
        )
        apply_grok_video_generation_options(payload.input, request_body, max_duration=GROK_VIDEO_DURATION_MAX_SECONDS)
        return "/v1/videos/generations", request_body, source_metadata

    if mode == "edit_video":
        if images:
            raise WorkerFailure("GROK_VIDEO_INPUT_MISMATCH", "edit_video mode does not accept image input.")
        ensure_no_grok_edit_unsupported_options(payload.input)
        source_url, source_video = require_grok_source_video(payload, videos, source_video_url, "edit_video")
        validate_grok_source_video_duration(
            source_video,
            max_seconds=GROK_VIDEO_EDIT_INPUT_MAX_SECONDS,
            code="GROK_VIDEO_EDIT_INPUT_TOO_LONG",
            message=f"edit_video input video must be at most {GROK_VIDEO_EDIT_INPUT_MAX_SECONDS:g} seconds.",
        )
        request_body = {"prompt": prompt, "video": {"url": source_url}}
        source_metadata.update(grok_source_video_metadata(source_video, source_url))
        return "/v1/videos/edits", request_body, source_metadata

    if mode == "extend_video":
        if images:
            raise WorkerFailure("GROK_VIDEO_INPUT_MISMATCH", "extend_video mode does not accept image input.")
        ensure_no_grok_extension_unsupported_options(payload.input)
        source_url, source_video = require_grok_source_video(payload, videos, source_video_url, "extend_video")
        validate_grok_source_video_duration(
            source_video,
            min_seconds=GROK_VIDEO_EXTENSION_INPUT_MIN_SECONDS,
            max_seconds=GROK_VIDEO_EXTENSION_INPUT_MAX_SECONDS,
            code="GROK_VIDEO_EXTENSION_INPUT_DURATION_INVALID",
            message=(
                f"extend_video input video must be between {GROK_VIDEO_EXTENSION_INPUT_MIN_SECONDS:g} "
                f"and {GROK_VIDEO_EXTENSION_INPUT_MAX_SECONDS:g} seconds."
            ),
        )
        request_body = {"prompt": prompt, "video": {"url": source_url}}
        apply_grok_video_extension_options(payload.input, request_body)
        source_metadata.update(grok_source_video_metadata(source_video, source_url))
        return "/v1/videos/extensions", request_body, source_metadata

    raise WorkerFailure("GROK_VIDEO_MODE_UNSUPPORTED", f"Unsupported Grok video mode: {mode}.")


async def poll_grok_video_response(
    payload: WorkerRunRequest,
    policy: SelectedPolicy,
    request_id: str,
    response: dict[str, Any],
    usage: dict[str, Any],
    mode: str,
) -> tuple[dict[str, Any], dict[str, Any]]:
    status = grok_video_status(response)
    if status in GROK_VIDEO_SUCCESS_STATUSES and extract_media_result(response, "video/mp4"):
        return response, usage

    video_timeout_seconds = payload.timeout_seconds if payload.timeout_seconds > 0 else 600
    deadline = time.monotonic() + max(video_timeout_seconds, 10)
    terminal_statuses = GROK_VIDEO_SUCCESS_STATUSES | GROK_VIDEO_FAILURE_STATUSES
    while status not in terminal_statuses:
        if is_canceled(payload.run_id):
            await callback(payload, "canceled", status="canceled", message="Run canceled during Grok video generation")
            raise WorkerCanceled()
        if time.monotonic() >= deadline:
            raise WorkerFailure("GROK_VIDEO_GENERATION_TIMEOUT", "Grok video generation did not finish before the Worker timeout.")
        await callback(
            payload,
            "progress",
            status="running",
            node_id=policy.node_id,
            role=policy.role,
            progress=0.7,
            message="Waiting for Grok video generation",
            metadata={
                **model_call_metadata(policy),
                "generation_mode": mode,
                "grok_video_request_id": request_id,
                "grok_video_status": status or "queued",
            },
        )
        await asyncio.sleep(VIDEO_POLL_INTERVAL_SECONDS)
        if is_canceled(payload.run_id):
            await callback(payload, "canceled", status="canceled", message="Run canceled during Grok video generation")
            raise WorkerCanceled()
        status_result = await call_model_proxy_request(
            payload,
            policy,
            endpoint=f"/v1/videos/{request_id}",
            method="GET",
        )
        current = status_result.get("response", {})
        if isinstance(current, dict):
            response = current
        status = grok_video_status(response)
        LOGGER.info(
            "Grok video poll request_id=%s status=%s has_media=%s",
            request_id,
            status or "queued",
            bool(extract_media_result(response, "video/mp4")),
        )
        status_usage = proxy_usage(status_result)
        if status_usage:
            usage = status_usage

    if status in GROK_VIDEO_FAILURE_STATUSES:
        message = grok_video_error_message(response) or f"Grok video generation ended with status {status}."
        raise WorkerFailure("GROK_VIDEO_GENERATION_FAILED", message)
    if not extract_media_result(response, "video/mp4"):
        raise WorkerFailure("GROK_VIDEO_RESULT_EMPTY", "The Grok video model completed but returned no downloadable video.")
    return response, usage


async def archive_grok_video_result(
    payload: WorkerRunRequest,
    response: dict[str, Any],
    *,
    name: str,
    metadata: dict[str, Any],
) -> dict[str, Any]:
    media = extract_media_result(response, "video/mp4")
    if not media:
        raise WorkerFailure("GROK_VIDEO_RESULT_EMPTY", "The Grok video model returned no downloadable video.")
    if media.get("b64_json"):
        return await upload_base64_artifact(
            payload,
            name=name,
            b64_json=str(media["b64_json"]),
            mime_type=str(media.get("mime_type") or "video/mp4"),
            metadata=metadata,
        )
    if media.get("url"):
        return await archive_remote_artifact(
            payload,
            name=name,
            url=str(media["url"]),
            mime_type=str(media.get("mime_type") or "video/mp4"),
            metadata=metadata,
        )
    raise WorkerFailure("GROK_VIDEO_RESULT_EMPTY", "The Grok video model returned no downloadable video.")


async def call_model_proxy(
    payload: WorkerRunRequest,
    policy: SelectedPolicy,
    prompt: str,
    *,
    on_text: Callable[[str], Awaitable[None]] | None = None,
) -> dict[str, Any]:
    model_request: dict[str, Any] = {
        "model": policy.model,
        "messages": [
            {
                "role": "user",
                "content": build_message_content(prompt, input_artifacts(payload)),
            }
        ],
        "stream": True,
        "stream_options": {"include_usage": True},
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
        "Accept": "text/event-stream",
        "X-Sub2API-Run-Token": payload.run_token,
        "X-Sub2API-Agent-Run-Token": payload.run_token,
    }
    current_task = asyncio.current_task()
    if current_task is not None:
        active_model_proxy_tasks[payload.run_id] = current_task
    try:
        async with httpx.AsyncClient(timeout=MODEL_PROXY_TIMEOUT_SECONDS) as client:
            async with client.stream("POST", payload.model_proxy_url, json=body, headers=headers) as response:
                if response.status_code >= 400:
                    raw = await response.aread()
                    raise WorkerFailure("MODEL_PROXY_FAILED", truncate(raw.decode("utf-8", errors="replace"), 1000))

                content_type = response.headers.get("content-type", "").lower()
                if "text/event-stream" not in content_type:
                    raw = await response.aread()
                    try:
                        return unwrap_sub2api_response(json.loads(raw))
                    except (TypeError, ValueError) as exc:
                        raise WorkerFailure("MODEL_PROXY_STREAM_INVALID", "Model proxy returned an invalid streaming response.") from exc

                event_type = ""
                done_received = False
                text_parts: list[str] = []
                usage: dict[str, Any] = {}
                async for line in response.aiter_lines():
                    if is_canceled(payload.run_id):
                        raise WorkerCanceled()
                    if line.startswith("event:"):
                        event_type = line[6:].strip()
                        continue
                    if not line.startswith("data:"):
                        if not line:
                            event_type = ""
                        continue
                    data = line[5:].strip()
                    if not data:
                        continue
                    if data == "[DONE]":
                        done_received = True
                        continue
                    if done_received:
                        continue
                    try:
                        event_payload = json.loads(data)
                    except ValueError as exc:
                        raise WorkerFailure("MODEL_PROXY_STREAM_INVALID", "Model proxy returned invalid SSE JSON.") from exc
                    if not isinstance(event_payload, dict):
                        continue
                    error_message = model_stream_error_message(event_type, event_payload)
                    if error_message:
                        raise WorkerFailure("MODEL_PROXY_STREAM_FAILED", error_message)
                    stream_usage = model_stream_usage(event_payload)
                    if stream_usage:
                        usage = stream_usage
                    delta = model_stream_text_delta(event_type, event_payload)
                    if not delta:
                        continue
                    text_parts.append(delta)
                    if on_text is not None:
                        await on_text("".join(text_parts))
    except asyncio.CancelledError as exc:
        if is_canceled(payload.run_id):
            raise WorkerCanceled() from exc
        raise
    finally:
        if current_task is not None and active_model_proxy_tasks.get(payload.run_id) is current_task:
            active_model_proxy_tasks.pop(payload.run_id, None)

    return {
        "response": {"text": "".join(text_parts)},
        "usage": usage,
        "status": 200,
        "content_type": "text/event-stream",
        "metadata": {"stream": True},
    }


def model_stream_text_delta(event_type: str, payload: dict[str, Any]) -> str:
    if event_type in {"response.output_text.delta", "response.refusal.delta"}:
        delta = payload.get("delta")
        return delta if isinstance(delta, str) else ""
    choices = payload.get("choices")
    if isinstance(choices, list) and choices and isinstance(choices[0], dict):
        delta = choices[0].get("delta")
        if isinstance(delta, dict):
            return stream_content_text(delta.get("content"))
    delta = payload.get("delta")
    if isinstance(delta, str):
        return delta
    return ""


def stream_content_text(content: Any) -> str:
    if isinstance(content, str):
        return content
    if not isinstance(content, list):
        return ""
    parts: list[str] = []
    for item in content:
        if not isinstance(item, dict):
            continue
        text = item.get("text") or item.get("content")
        if isinstance(text, str):
            parts.append(text)
    return "".join(parts)


def model_stream_usage(payload: dict[str, Any]) -> dict[str, Any]:
    usage = payload.get("usage")
    if isinstance(usage, dict):
        return usage
    response = payload.get("response")
    if isinstance(response, dict) and isinstance(response.get("usage"), dict):
        return response["usage"]
    return {}


def model_stream_error_message(event_type: str, payload: dict[str, Any]) -> str:
    error = payload.get("error")
    if isinstance(error, dict):
        message = error.get("message")
        if isinstance(message, str) and message.strip():
            return message.strip()
    if event_type == "error" or payload.get("type") == "error":
        message = payload.get("message")
        if isinstance(message, str) and message.strip():
            return message.strip()
        return "Model proxy stream failed."
    return ""


async def call_image_model_proxy(
    payload: WorkerRunRequest,
    policy: SelectedPolicy,
    prompt: str,
    *,
    references: list[WorkerArtifactRef] | None = None,
    reference_bodies: list[bytes] | None = None,
) -> dict[str, Any]:
    image_request: dict[str, Any] = {
        "prompt": prompt,
        "n": 1,
    }
    copy_input_fields(
        payload.input,
        image_request,
        "size",
        "quality",
        "background",
        "output_format",
        "output_compression",
        "input_fidelity",
    )

    references = references or []
    if not references:
        return await call_model_proxy_request(
            payload,
            policy,
            endpoint="/v1/images/generations",
            request_body=image_request,
        )

    if reference_bodies is None:
        reference_bodies = await download_reference_images(references)
    if len(reference_bodies) != len(references):
        raise WorkerFailure("IMAGE_REFERENCE_BODY_MISMATCH", "Reference image metadata and downloaded bodies do not match.")
    return await call_model_proxy_request(
        payload,
        policy,
        endpoint="/v1/images/edits",
        request_body=image_request,
        content_type="multipart/form-data",
        multipart=[
            {
                "name": "image",
                "filename": reference.name or f"image-reference-{payload.run_id}-{index + 1}.png",
                "content_type": artifact_ref_mime(reference) or "application/octet-stream",
                "body_base64": base64.b64encode(reference_body).decode("ascii"),
            }
            for index, (reference, reference_body) in enumerate(zip(references, reference_bodies))
        ],
    )


async def call_model_proxy_request(
    payload: WorkerRunRequest,
    policy: SelectedPolicy,
    *,
    endpoint: str,
    method: str = "POST",
    request_body: dict[str, Any] | None = None,
    content_type: str = "",
    multipart: list[dict[str, Any]] | None = None,
) -> dict[str, Any]:
    body: dict[str, Any] = {
        "run_id": payload.run_id,
        "node_id": policy.node_id,
        "role": policy.role,
        "model": policy.model,
        "endpoint": endpoint,
        "method": method,
        "request": request_body or {},
        "metadata": {
            "worker": "sub2api-app-worker",
            "policy_key": policy.policy_key,
            "capability": policy.capability,
        },
    }
    if policy.model_group_id is not None:
        body["group_id"] = policy.model_group_id
    if content_type:
        body["content_type"] = content_type
    if multipart:
        body["multipart"] = multipart

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
    return unwrap_sub2api_response(response.json())


async def archive_proxy_media_result(
    payload: WorkerRunRequest,
    proxy_result: dict[str, Any],
    *,
    default_name: str,
    default_mime: str,
    metadata: dict[str, Any],
) -> dict[str, Any]:
    mime_type = normalized_content_type(proxy_result.get("content_type")) or default_mime
    name = proxy_artifact_name(proxy_result, os.path.splitext(default_name)[0], mime_type)
    body_base64 = str(proxy_result.get("body_base64") or "").strip()
    if body_base64:
        return await upload_base64_artifact(
            payload,
            name=name,
            b64_json=body_base64,
            mime_type=mime_type,
            metadata=metadata,
        )

    media = extract_media_result(proxy_result.get("response", {}), default_mime)
    if media and media.get("url"):
        return await archive_remote_artifact(
            payload,
            name=name,
            url=str(media["url"]),
            mime_type=str(media.get("mime_type") or mime_type),
            metadata=metadata,
        )
    if media and media.get("b64_json"):
        return await upload_base64_artifact(
            payload,
            name=name,
            b64_json=str(media["b64_json"]),
            mime_type=str(media.get("mime_type") or mime_type),
            metadata=metadata,
        )
    raise WorkerFailure("MEDIA_RESULT_EMPTY", "The media model returned no downloadable content.")


async def download_limited(
    url: str,
    *,
    max_bytes: int,
    download_error_code: str,
    download_error_prefix: str,
    too_large_code: str,
    too_large_message: str,
) -> tuple[bytes, httpx.Headers]:
    try:
        async with httpx.AsyncClient(timeout=MODEL_PROXY_TIMEOUT_SECONDS, follow_redirects=True) as client:
            async with client.stream("GET", url) as response:
                response.raise_for_status()
                if max_bytes > 0 and response_content_length(response.headers) > max_bytes:
                    raise WorkerFailure(too_large_code, too_large_message)
                content = bytearray()
                async for chunk in response.aiter_bytes():
                    if max_bytes > 0 and len(content) + len(chunk) > max_bytes:
                        raise WorkerFailure(too_large_code, too_large_message)
                    content.extend(chunk)
                return bytes(content), response.headers
    except WorkerFailure:
        raise
    except Exception as exc:
        raise WorkerFailure(download_error_code, f"{download_error_prefix}: {truncate(str(exc), 500)}") from exc


async def download_input_artifact(artifact: WorkerArtifactRef, *, max_bytes: int | None = None) -> bytes:
    if not artifact.url:
        raise WorkerFailure("INPUT_ASSET_URL_MISSING", f"Input asset {artifact.name or artifact.artifact_id} has no download URL.")
    limits = [value for value in (MAX_MODEL_PROXY_ASSET_BYTES, max_bytes) if value is not None and value > 0]
    effective_max_bytes = min(limits) if limits else 0
    raw, _ = await download_limited(
        artifact.url,
        max_bytes=effective_max_bytes,
        download_error_code="INPUT_ASSET_DOWNLOAD_FAILED",
        download_error_prefix="Unable to download input asset",
        too_large_code="INPUT_ASSET_TOO_LARGE",
        too_large_message="Input asset exceeds the Model Proxy upload limit.",
    )
    return raw


async def download_reference_images(references: list[WorkerArtifactRef]) -> list[bytes]:
    if len(references) > MAX_IMAGE_REFERENCE_COUNT:
        raise WorkerFailure(
            "IMAGE_REFERENCE_COUNT_EXCEEDED",
            f"At most {MAX_IMAGE_REFERENCE_COUNT} reference images are supported per run.",
        )
    bodies: list[bytes] = []
    total_bytes = 0
    for reference in references:
        body = await download_input_artifact(reference, max_bytes=MAX_IMAGE_REFERENCE_BYTES)
        total_bytes += len(body)
        if MAX_IMAGE_REFERENCE_TOTAL_BYTES > 0 and total_bytes > MAX_IMAGE_REFERENCE_TOTAL_BYTES:
            raise WorkerFailure(
                "IMAGE_REFERENCE_TOTAL_TOO_LARGE",
                "Reference images exceed the combined Model Proxy upload limit.",
            )
        bodies.append(body)
    return bodies


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
    return require_artifact_response(result)


async def archive_remote_artifact(payload: WorkerRunRequest, *, name: str, url: str, mime_type: str, metadata: dict[str, Any]) -> dict[str, Any]:
    raw, headers = await download_limited(
        url,
        max_bytes=artifact_size_limit(payload),
        download_error_code="ARTIFACT_DOWNLOAD_FAILED",
        download_error_prefix="无法下载模型生成结果",
        too_large_code="ARTIFACT_TOO_LARGE",
        too_large_message="模型生成结果超过 Worker 允许归档的大小",
    )
    resolved_mime = headers.get("content-type", "").split(";", 1)[0].strip() or mime_type
    return await upload_artifact_bytes(payload, name=name, raw=raw, mime_type=resolved_mime, metadata=metadata)


async def upload_base64_artifact(payload: WorkerRunRequest, *, name: str, b64_json: str, mime_type: str, metadata: dict[str, Any]) -> dict[str, Any]:
    if not payload.artifact_url:
        raise WorkerFailure("ARTIFACT_URL_MISSING", "Sub2API artifact URL is missing.")
    encoded = b64_json.strip()
    if encoded.startswith("data:"):
        _, separator, encoded = encoded.partition(",")
        if not separator:
            raise WorkerFailure("ARTIFACT_BASE64_INVALID", "The media model returned invalid base64 data.")
    encoded = "".join(encoded.split())
    max_bytes = artifact_size_limit(payload)
    if max_bytes > 0 and (len(encoded) * 3 // 4) > max_bytes + 2:
        raise WorkerFailure("ARTIFACT_TOO_LARGE", "The generated artifact exceeds the Worker archive limit.")
    try:
        raw = base64.b64decode(encoded, validate=True)
    except Exception as exc:
        raise WorkerFailure("ARTIFACT_BASE64_INVALID", "The media model returned invalid base64 data.") from exc
    if max_bytes > 0 and len(raw) > max_bytes:
        raise WorkerFailure("ARTIFACT_TOO_LARGE", "The generated artifact exceeds the Worker archive limit.")
    if not raw:
        raise WorkerFailure("ARTIFACT_EMPTY", "The media model returned an empty artifact.")
    return await upload_artifact_bytes(payload, name=name, raw=raw, mime_type=mime_type, metadata=metadata)


async def upload_artifact_bytes(payload: WorkerRunRequest, *, name: str, raw: bytes, mime_type: str, metadata: dict[str, Any]) -> dict[str, Any]:
    if not payload.artifact_url:
        raise WorkerFailure("ARTIFACT_URL_MISSING", "Sub2API artifact URL is missing.")
    max_bytes = artifact_size_limit(payload)
    if max_bytes > 0 and len(raw) > max_bytes:
        raise WorkerFailure("ARTIFACT_TOO_LARGE", "The generated artifact exceeds the Worker archive limit.")
    if not raw:
        raise WorkerFailure("ARTIFACT_EMPTY", "The media model returned an empty artifact.")
    headers = {
        "Accept": "application/json",
        "X-Sub2API-Run-Token": payload.run_token,
    }
    data = {
        "type": "output",
        "name": name,
        "mime_type": mime_type,
        "sha256": hashlib.sha256(raw).hexdigest(),
        "metadata": json.dumps(metadata, ensure_ascii=False),
    }
    files = {"file": (name, raw, mime_type)}
    async with httpx.AsyncClient(timeout=MODEL_PROXY_TIMEOUT_SECONDS) as client:
        response = await client.post(f"{payload.artifact_url.rstrip('/')}/upload", data=data, files=files, headers=headers)
    if response.status_code >= 400:
        raise WorkerFailure("ARTIFACT_UPLOAD_FAILED", truncate(response.text, 1000))
    result = unwrap_sub2api_response(response.json())
    return require_artifact_response(result)


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
    error_details: dict[str, Any] | None = None,
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
    if error_details is not None:
        body["error"] = error_details

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
        error_details={"code": code, "message": message},
        metadata={"error_code": code},
    )


def select_media_policy(payload: WorkerRunRequest) -> tuple[str, SelectedPolicy] | None:
    preferred_route = str(payload.metadata.get("worker_route") or "").lower()
    candidates: list[tuple[int, str, str, ModelPolicy]] = []
    for key, policy in (payload.node_model_policy or {}).items():
        normalized = policy if isinstance(policy, ModelPolicy) else ModelPolicy.model_validate(policy)
        node_id, role = policy_key_parts(key)
        normalized.node_id = normalized.node_id or node_id
        normalized.role = normalized.role or role or "generate"
        media_kind = media_policy_kind(normalized, payload)
        if not media_kind:
            continue
        score = 50
        if media_kind.startswith("audio_") and "audio" in preferred_route:
            score -= 30
        if media_kind == "video_generation" and "video" in preferred_route:
            score -= 30
        if media_kind == "image_generation" and "image" in preferred_route:
            score -= 30
        if media_kind in {"audio_transcription", "audio_translation"} and any(
            is_audio_artifact_ref(item) for item in input_artifacts(payload)
        ):
            score -= 20
        candidates.append((score, key, media_kind, normalized))

    if not candidates:
        return None
    _, key, media_kind, policy = sorted(candidates, key=lambda item: (item[0], item[1]))[0]
    if not policy.model:
        raise WorkerFailure("MODEL_POLICY_MODEL_REQUIRED", f"Model policy {key} is missing model.")
    return media_kind, SelectedPolicy(
        policy_key=key,
        node_id=policy.node_id or "media",
        role=policy.role or "generate",
        model=policy.model,
        model_group_id=policy.model_group_id,
        capability=policy.capability or media_kind,
    )


def media_policy_kind(policy: ModelPolicy, payload: WorkerRunRequest) -> str:
    capability = normalize_policy_value(policy.capability)
    role = normalize_policy_value(policy.role)
    if capability in {
        "image",
        "images",
        "image_generation",
        "image_generate",
        "image_edit",
        "image_to_image",
        "text_to_image",
    }:
        return "image_generation"
    if capability in {
        "video",
        "videos",
        "video_generation",
        "video_generate",
        "text_to_video",
        "image_to_video",
        "reference_to_video",
        "edit_video",
        "extend_video",
        "video_edit",
        "video_extend",
        "video_extension",
        "grok_video",
        "grok_video_generation",
        "grok_video_generate",
        "grok_text_to_video",
        "grok_image_to_video",
        "grok_reference_to_video",
        "grok_video_edit",
        "grok_video_extend",
    }:
        return "video_generation"
    if capability in {"audio_transcription", "audio_transcriptions", "transcription", "speech_to_text", "stt"}:
        return "audio_transcription"
    if capability in {"audio_translation", "audio_translations", "audio_translate"}:
        return "audio_translation"
    if capability in {"audio_speech", "speech", "text_to_speech", "tts", "audio_generation"}:
        return "audio_speech"
    if capability == "audio":
        if role in {"translate", "translation"}:
            return "audio_translation"
        if role in {"transcribe", "transcription", "speech_to_text", "stt"} or any(
            is_audio_artifact_ref(item) for item in input_artifacts(payload)
        ):
            return "audio_transcription"
        return "audio_speech"
    return ""


def normalize_policy_value(value: str) -> str:
    return value.strip().lower().replace("-", "_").replace(" ", "_")


def is_product_marketing_run(payload: WorkerRunRequest) -> bool:
    route = normalize_policy_value(str(payload.metadata.get("worker_route") or ""))
    return "product_marketing" in route


def is_academic_paper_run(payload: WorkerRunRequest) -> bool:
    route = normalize_policy_value(str(payload.metadata.get("worker_route") or ""))
    return "academic_paper" in route


def ensure_run_active(payload: WorkerRunRequest, stage: str) -> None:
    if is_canceled(payload.run_id):
        raise WorkerCanceled(f"Run canceled {stage}")


def bounded_int_input(values: dict[str, Any], key: str, *, default: int, minimum: int, maximum: int) -> int:
    value = values.get(key, default)
    if isinstance(value, bool):
        raise WorkerFailure("INPUT_INVALID", f"{key} must be an integer between {minimum} and {maximum}.")
    try:
        parsed = int(value)
    except (TypeError, ValueError) as exc:
        raise WorkerFailure("INPUT_INVALID", f"{key} must be an integer between {minimum} and {maximum}.") from exc
    if parsed < minimum or parsed > maximum:
        raise WorkerFailure("INPUT_INVALID", f"{key} must be between {minimum} and {maximum}.")
    return parsed


def required_int_input(values: dict[str, Any], key: str, *, minimum: int, maximum: int) -> int:
    if key not in values or values.get(key) in (None, ""):
        raise WorkerFailure("PAPER_WORD_COUNT_REQUIRED", "请填写论文目标字数。")
    return bounded_int_input(values, key, default=minimum, minimum=minimum, maximum=maximum)


def select_academic_paper_policies(payload: WorkerRunRequest) -> tuple[SelectedPolicy, SelectedPolicy]:
    capabilities = {"text", "vision", "model"}
    plan_policy = find_policy(
        payload,
        capabilities=capabilities,
        roles={"plan", "outline", "analyze", "analyse"},
    )
    write_policy = find_policy(
        payload,
        capabilities=capabilities,
        roles={"write", "author", "generate"},
    )
    fallback = plan_policy or write_policy
    if fallback is None:
        fallback = select_policy(payload)
        if normalize_policy_value(fallback.capability) not in capabilities:
            raise WorkerFailure("PAPER_MODEL_POLICY_REQUIRED", "论文工作流需要 text、vision 或 model 类型的模型策略。")
    return plan_policy or fallback, write_policy or fallback


async def extract_paper_reference_context(payload: WorkerRunRequest) -> str:
    parts: list[str] = []
    total_chars = 0
    for artifact in input_artifacts(payload):
        if is_paper_template_artifact(artifact) or is_image_artifact_ref(artifact) or not is_supported_paper_reference(artifact):
            continue
        raw = await download_input_artifact(artifact, max_bytes=PAPER_REFERENCE_MAX_FILE_BYTES)
        text = await asyncio.to_thread(extract_paper_reference_text, artifact, raw)
        text = normalize_reference_text(text)
        if not text:
            continue
        remaining = PAPER_REFERENCE_MAX_TOTAL_CHARS - total_chars if PAPER_REFERENCE_MAX_TOTAL_CHARS > 0 else len(text)
        if remaining <= 0:
            break
        per_file_limit = PAPER_REFERENCE_MAX_CHARS_PER_FILE if PAPER_REFERENCE_MAX_CHARS_PER_FILE > 0 else len(text)
        excerpt = text[: min(per_file_limit, remaining)]
        total_chars += len(excerpt)
        name = artifact.name or f"reference-{len(parts) + 1}"
        parts.append(f"=== 参考资料：{name} ===\n{excerpt}")
    if not parts:
        return ""
    return (
        "以下内容来自用户上传的参考资料。只能引用其中能够明确识别作者、题名或出处的资料；"
        "无法确认来源时不要编造作者、年份、刊名、DOI、页码或研究数据。\n\n"
        + "\n\n".join(parts)
    )


def find_paper_template_artifact(payload: WorkerRunRequest) -> WorkerArtifactRef | None:
    return next((artifact for artifact in input_artifacts(payload) if is_paper_template_artifact(artifact)), None)


def is_paper_template_artifact(artifact: WorkerArtifactRef) -> bool:
    for key in ("asset_role", "field_name", "input_name"):
        value = artifact.metadata.get(key)
        if isinstance(value, str) and normalize_policy_value(value) in {"template", "template_file", "paper_template"}:
            return True
    return False


def is_docx_artifact_ref(artifact: WorkerArtifactRef) -> bool:
    return artifact_ref_mime(artifact) == "application/vnd.openxmlformats-officedocument.wordprocessingml.document" or artifact.name.lower().endswith(".docx")


def is_supported_paper_reference(artifact: WorkerArtifactRef) -> bool:
    mime_type = artifact_ref_mime(artifact)
    extension = os.path.splitext(artifact.name.lower())[1]
    if mime_type.startswith("text/"):
        return True
    if mime_type in {
        "application/json",
        "application/pdf",
        "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
    }:
        return True
    return extension in {".txt", ".md", ".markdown", ".csv", ".json", ".docx", ".pdf"}


def extract_paper_reference_text(artifact: WorkerArtifactRef, raw: bytes) -> str:
    mime_type = artifact_ref_mime(artifact)
    extension = os.path.splitext(artifact.name.lower())[1]
    try:
        if mime_type == "application/pdf" or extension == ".pdf":
            return extract_pdf_reference_text(raw)
        if mime_type == "application/vnd.openxmlformats-officedocument.wordprocessingml.document" or extension == ".docx":
            return extract_docx_reference_text(raw)
        return decode_text_reference(raw)
    except WorkerFailure:
        raise
    except Exception as exc:
        name = artifact.name or str(artifact.artifact_id or "reference")
        raise WorkerFailure("PAPER_REFERENCE_PARSE_FAILED", f"无法解析参考资料“{name}”：{truncate(str(exc), 300)}") from exc


def decode_text_reference(raw: bytes) -> str:
    for encoding in ("utf-8-sig", "gb18030", "utf-16"):
        try:
            return raw.decode(encoding)
        except UnicodeDecodeError:
            continue
    return raw.decode("utf-8", errors="replace")


def extract_docx_reference_text(raw: bytes) -> str:
    try:
        from docx import Document
        from docx.table import Table
        from docx.text.paragraph import Paragraph
    except ImportError as exc:
        raise WorkerFailure(
            "PAPER_REFERENCE_PARSER_MISSING",
            "解析 Word 参考资料需要安装 python-docx，请重新构建 Worker 镜像。",
        ) from exc
    document = Document(io.BytesIO(raw))
    parts: list[str] = []
    for block in document.iter_inner_content():
        if isinstance(block, Paragraph):
            if block.text.strip():
                parts.append(block.text)
            continue
        if isinstance(block, Table):
            for row in block.rows:
                cells = [cell.text.strip() for cell in row.cells]
                if any(cells):
                    parts.append("\t".join(cells))
    return "\n".join(parts)


def extract_pdf_reference_text(raw: bytes) -> str:
    try:
        from pypdf import PdfReader
    except ImportError as exc:
        raise WorkerFailure(
            "PAPER_REFERENCE_PARSER_MISSING",
            "解析 PDF 参考资料需要安装 pypdf，请重新构建 Worker 镜像。",
        ) from exc
    reader = PdfReader(io.BytesIO(raw))
    parts: list[str] = []
    for page in reader.pages:
        text = (page.extract_text() or "").strip()
        if text:
            parts.append(text)
    return "\n\n".join(parts)


def normalize_reference_text(value: str) -> str:
    value = value.replace("\x00", "")
    value = re.sub(r"[ \t]+", " ", value)
    value = re.sub(r"\n{4,}", "\n\n\n", value)
    return value.strip()


def parse_paper_reference_registry(reference_context: str) -> dict[str, Any] | None:
    """Build a deterministic bibliography from explicitly numbered source lines."""
    lines = (reference_context or "").splitlines()
    candidates: list[tuple[int, str]] = []
    consumed_lines: set[int] = set()

    def continuation(start_index: int, initial: str = "") -> tuple[str, set[int]]:
        parts = [initial.strip()] if initial.strip() else []
        consumed: set[int] = set()
        index = start_index
        while index < len(lines):
            candidate = lines[index].strip()
            if not candidate:
                if parts:
                    break
                index += 1
                continue
            if (
                _PAPER_REFERENCE_SOURCE_HEADING_RE.match(candidate)
                or _PAPER_REFERENCE_INLINE_RE.match(candidate)
            ):
                break
            if candidate.startswith("===") or re.match(
                r"^(?:可核验要点|要点|说明|摘要|evidence|takeaway|notes?)\s*[:：]",
                candidate,
                re.IGNORECASE,
            ):
                break
            parts.append(candidate)
            consumed.add(index)
            index += 1
        return " ".join(parts).strip(), consumed

    for index, line in enumerate(lines):
        heading = _PAPER_REFERENCE_SOURCE_HEADING_RE.match(line)
        if heading is None:
            continue
        reference_id = int(heading.group(1))
        citation, consumed = continuation(index + 1)
        if not citation:
            continue
        inline = _PAPER_REFERENCE_INLINE_RE.fullmatch(citation)
        if inline is not None and int(inline.group(1)) == reference_id:
            citation = inline.group(2).strip()
        if citation and _PAPER_REFERENCE_SOURCE_HEADING_RE.match(citation) is None:
            candidates.append((reference_id, citation))
            consumed_lines.update(consumed)

    for index, line in enumerate(lines):
        if index in consumed_lines:
            continue
        inline = _PAPER_REFERENCE_INLINE_RE.match(line)
        if inline is not None:
            citation, consumed = continuation(index + 1, inline.group(2))
            if citation and looks_like_bibliographic_reference(citation):
                candidates.append((int(inline.group(1)), citation))
                consumed_lines.update(consumed)

    if not candidates:
        return None
    reference_ids = [reference_id for reference_id, _citation in candidates]
    if len(reference_ids) != len(set(reference_ids)):
        return None
    ordered = sorted(candidates, key=lambda item: item[0])
    if [reference_id for reference_id, _citation in ordered] != list(range(1, len(ordered) + 1)):
        return None

    entries = [
        {"id": reference_id, "citation": citation, "formatted": f"[{reference_id}] {citation}"}
        for reference_id, citation in ordered
    ]
    return {
        "ids": [entry["id"] for entry in entries],
        "entries": entries,
        "bibliography": [entry["citation"] for entry in entries],
        "numbered_bibliography": [entry["formatted"] for entry in entries],
    }


def looks_like_bibliographic_reference(value: str) -> bool:
    return bool(
        re.search(
            r"(?:\[(?:J|M|C|D|R|N|P|S|EB/OL)\]|\bDOI\s*[:：]|\b10\.\d{4,9}/|"
            r"出版社|学报|期刊|journal|press|proceedings)",
            value or "",
            re.IGNORECASE,
        )
    )


def paper_reference_contract_context(reference_registry: dict[str, Any]) -> str:
    bibliography = reference_registry.get("numbered_bibliography") or []
    reference_ids = reference_registry.get("ids") or []
    return f"""确定性参考文献注册表（由代码从用户资料解析，优先级最高）：
{json.dumps(bibliography, ensure_ascii=False, indent=2)}

严格引用契约：
1. 文末参考文献由代码固定为上述注册表，忽略并覆盖模型返回的 references。
2. 正文只能引用编号 {reference_ids}，每个编号至少出现一次，禁止新增未知编号。
3. 生成正文时使用内部标记 [[CITE:n]]，例如 [[CITE:1]]；不要自行改写、合并或重排注册表。
4. [[CITE:n]] 只应放在相应来源能够支持的论断之后，不能放在标题中。"""


def exact_paper_reference_count(values: dict[str, Any]) -> int | None:
    requirement = string_input(values, "reference_requirements")
    if not requirement:
        return None
    patterns = (
        r"(?:恰好|正好)\s*(\d{1,3})\s*(?:条|篇)\s*(?:参考文献|文献)",
        r"(?:参考文献|文献)[^。；;\n]{0,30}?(?:恰好|正好)\s*(\d{1,3})\s*(?:条|篇)?",
        r"\bexactly\s+(\d{1,3})\s+references?\b",
    )
    for pattern in patterns:
        match = re.search(pattern, requirement, re.IGNORECASE)
        if match is not None:
            count = int(match.group(1))
            return count if count > 0 else None
    return None


def academic_paper_citation_style(values: dict[str, Any]) -> str:
    style = first_nonempty_input(values, "citation_style", "citation_format")
    if style:
        return style
    raw_settings = values.get("format_settings")
    if isinstance(raw_settings, str):
        raw_settings = parse_json_object(raw_settings)
    if isinstance(raw_settings, dict):
        style = first_nonempty_input(raw_settings, "citation_style", "citation_format")
        if style:
            return style
    return "gbt7714_numeric"


def academic_paper_uses_numeric_citations(values: dict[str, Any]) -> bool:
    style = academic_paper_citation_style(values)
    normalized = style.strip().lower().replace("-", "_").replace(" ", "")
    if normalized in {
        "gbt7714_numeric",
        "gb/t7714_numeric",
        "numeric",
        "numbered",
        "ieee",
        "vancouver",
    }:
        return True
    return "7714" in normalized and not any(
        marker in normalized for marker in ("author", "year", "著者", "年份")
    )


def validate_academic_paper_citation_contract(
    sections: list[dict[str, Any]],
    reference_registry: dict[str, Any],
) -> dict[str, Any]:
    expected_ids = {int(value) for value in reference_registry.get("ids") or []}
    used_ids: set[int] = set()
    malformed_markers = False
    for section in sections:
        if not isinstance(section, dict):
            continue
        section_text = flatten_paper_section_text(section)
        for match in _PAPER_CITATION_MARKER_RE.finditer(section_text):
            used_ids.add(int(match.group(1) or match.group(2)))
        if re.search(r"\[\[\s*CITE\b", _PAPER_CITATION_MARKER_RE.sub("", section_text), re.IGNORECASE):
            malformed_markers = True
    missing_ids = sorted(expected_ids - used_ids)
    unknown_ids = sorted(used_ids - expected_ids)
    return {
        "reference_contract_valid": not missing_ids and not unknown_ids and not malformed_markers,
        "citation_ids_used": sorted(used_ids),
        "citation_ids_missing": missing_ids,
        "citation_ids_unknown": unknown_ids,
        "citation_markers_malformed": malformed_markers,
    }


def normalize_academic_paper_citation_markers(sections: list[dict[str, Any]]) -> None:
    def normalize_section(section: dict[str, Any]) -> None:
        content = section.get("content")
        if isinstance(content, str):
            section["content"] = _PAPER_CITATION_MARKER_RE.sub(
                lambda match: f"[{int(match.group(1) or match.group(2))}]",
                content,
            )
        children = section.get("children")
        if isinstance(children, list):
            for child in children:
                if isinstance(child, dict):
                    normalize_section(child)

    for section in sections:
        if isinstance(section, dict):
            normalize_section(section)


def enforce_academic_paper_citation_contract(
    sections: list[dict[str, Any]],
    reference_registry: dict[str, Any],
) -> dict[str, Any]:
    """Normalize citation markers without changing any non-citation content."""
    normalize_academic_paper_citation_markers(sections)
    result = validate_academic_paper_citation_contract(sections, reference_registry)
    return {**result, "reference_contract_repaired": False}


def academic_paper_citation_content_nodes(sections: list[dict[str, Any]]) -> dict[str, dict[str, Any]]:
    nodes: dict[str, dict[str, Any]] = {}

    def visit(section: dict[str, Any], path: tuple[int, ...]) -> None:
        node_id = clean_string(section.get("id")) or "section-" + "-".join(str(value) for value in path)
        if node_id in nodes:
            node_id = f"{node_id}-{'-'.join(str(value) for value in path)}"
        if isinstance(section.get("content"), str):
            nodes[node_id] = section
        children = section.get("children")
        if isinstance(children, list):
            for index, child in enumerate(children, start=1):
                if isinstance(child, dict):
                    visit(child, (*path, index))

    for index, section in enumerate(sections, start=1):
        if isinstance(section, dict):
            visit(section, (index,))
    return nodes


def citation_neutral_paper_text(value: str) -> str:
    without_citations = _PAPER_CITATION_MARKER_RE.sub("", value or "")
    normalized_lines = [re.sub(r"[ \t]+", " ", line) for line in without_citations.split("\n")]
    return "\n".join(normalized_lines).strip()


def build_academic_paper_citation_repair_prompt(
    sections: list[dict[str, Any]],
    reference_registry: dict[str, Any],
    missing_ids: list[int],
    unknown_ids: list[int],
) -> str:
    content_nodes = academic_paper_citation_content_nodes(sections)
    contents = {node_id: clean_string(node.get("content")) for node_id, node in content_nodes.items()}
    total_chars = sum(len(value) for value in contents.values())
    if total_chars > PAPER_REFERENCE_MAX_TOTAL_CHARS:
        raise WorkerFailure(
            "PAPER_CITATION_REPAIR_TOO_LARGE",
            "正文过长，无法在单次安全修复中补齐引用；已停止生成 Word 文件。",
        )
    entries = reference_registry.get("entries") or []
    return f"""你是学术论文引用校对员。正文内容已经锁定，不能改写；你只负责插入或删除引用标记。

代码注册的参考文献：
{json.dumps(entries, ensure_ascii=False, indent=2)}

当前缺失的引用编号：{json.dumps(missing_ids, ensure_ascii=False)}
当前未知、必须删除或替换的引用编号：{json.dumps(unknown_ids, ensure_ascii=False)}

锁定正文（键是不可修改的节点 ID）：
{json.dumps(contents, ensure_ascii=False, indent=2)}

只返回 JSON 对象，不要使用 Markdown：{{"contents": {{"节点 ID": "修复后的完整正文"}}}}

严格规则：
1. 只能插入或删除形如 [[CITE:n]] 或 [n] 的引用标记，禁止修改、增删、重排任何其他文字和标点。
2. 每个缺失编号至少插入一次，并放在该来源能够支持的具体论断之后；不得把全部编号机械堆在段末。
3. 只能使用注册表中的编号，禁止新增未知编号。
4. contents 只需返回实际修改的节点；键必须使用上方原节点 ID。"""


def apply_academic_paper_citation_repair(
    raw: str,
    sections: list[dict[str, Any]],
) -> None:
    parsed = parse_json_object(raw)
    repaired_contents = parsed.get("contents")
    if not isinstance(repaired_contents, dict) or not repaired_contents:
        raise WorkerFailure(
            "PAPER_CITATION_REPAIR_INVALID",
            "引用修复未返回有效的 contents 对象；已停止生成 Word 文件。",
        )
    content_nodes = academic_paper_citation_content_nodes(sections)
    updates: list[tuple[dict[str, Any], str]] = []
    for node_id, repaired in repaired_contents.items():
        node = content_nodes.get(str(node_id))
        if node is None or not isinstance(repaired, str) or not repaired.strip():
            raise WorkerFailure(
                "PAPER_CITATION_REPAIR_INVALID",
                "引用修复包含未知节点或空正文；已停止生成 Word 文件。",
            )
        original = clean_string(node.get("content"))
        if citation_neutral_paper_text(original) != citation_neutral_paper_text(repaired):
            raise WorkerFailure(
                "PAPER_CITATION_REPAIR_CHANGED_CONTENT",
                "引用修复改动了引用标记之外的正文；已停止生成 Word 文件。",
            )
        updates.append((node, repaired.strip()))
    for node, repaired in updates:
        node["content"] = repaired


def build_academic_paper_plan_prompt(values: dict[str, Any], word_count: int, reference_context: str) -> str:
    section_count = recommended_paper_section_count(word_count)
    brief = academic_paper_brief(values, word_count)
    references = reference_context or "用户没有提供可抽取的文字参考资料。不要因此虚构参考文献或研究数据。"
    keyword_instruction = academic_paper_keyword_instruction(values)
    return f"""你是一名严谨的学术论文策划编辑。请依据用户要求规划一篇可直接写作和排版的完整论文。

论文要求：
{json.dumps(brief, ensure_ascii=False, indent=2)}

参考资料规则与内容：
{references}

只返回一个 JSON 对象，不要使用 Markdown 代码块，也不要添加解释。结构必须为：
{{
  "title": "论文题目",
  "abstract": "完整摘要",
  "keywords": ["关键词1", "关键词2"],
  "sections": [
    {{
      "title": "一级章节标题",
      "level": 1,
      "target_words": 1000,
      "purpose": "本章任务",
      "key_points": ["论点或内容要点"],
      "subsections": ["二级标题1", "二级标题2"]
    }}
  ],
  "references": ["仅列出能从用户资料中确认的真实参考文献"],
  "acknowledgements": "仅在用户启用致谢时填写，否则为空字符串",
  "appendices": [{{"title": "附录标题", "content": "仅在用户启用附录时生成的完整内容"}}]
}}

顶层章节建议控制为 {section_count} 个，最多 8 个；章节目标字数总和必须接近 {word_count}。目录层级最多五级，并优先服从用户提供的目录结构。摘要应概括研究目的、方法、主要内容和结论。{keyword_instruction}严格遵守摘要、关键词、参考文献、致谢和附录的启用开关；未启用的部分返回空值。引用只能来自用户资料中可识别的来源；没有可靠来源时 references 返回空数组，严禁虚构文献、数据、访谈、实验或调查结果。"""


def build_locked_academic_paper_plan_prompt(
    values: dict[str, Any],
    word_count: int,
    reference_context: str,
    outline_nodes: list[dict[str, Any]],
) -> str:
    brief = academic_paper_brief(values, word_count)
    outline = [locked_outline_prompt_node(node) for node in outline_nodes]
    references = reference_context or "用户没有提供可抽取的文字参考资料。不要因此虚构参考文献或研究数据。"
    keyword_instruction = academic_paper_keyword_instruction(values)
    return f"""你是一名严谨的学术论文策划编辑。论文目录已经由用户确认并锁定，你只负责规划题目、摘要、关键词和附加部分，不能规划或修改章节。

论文要求：
{json.dumps(brief, ensure_ascii=False, indent=2)}

代码锁定的论文目录（仅用于理解全文主题）：
{json.dumps(outline, ensure_ascii=False, indent=2)}

参考资料规则与内容：
{references}

只返回一个 JSON 对象，不要使用 Markdown 代码块，也不要添加解释。结构必须为：
{{
  "title": "论文题目",
  "abstract": "完整摘要",
  "keywords": ["关键词1", "关键词2"],
  "references": ["仅列出能从用户资料中确认的真实参考文献"],
  "acknowledgements": "仅在用户启用致谢时填写，否则为空字符串",
  "appendices": [{{"title": "附录标题", "content": "仅在用户启用附录时生成的完整内容"}}]
}}

不要返回 sections、outline、children、标题层级或任何目录替代方案。目录中的 ID、标题、层级、顺序和目标字数全部由代码控制。摘要应覆盖锁定目录所表达的研究目的、方法、主要内容和结论。{keyword_instruction}严格遵守摘要、关键词、参考文献、致谢和附录的启用开关；未启用的部分返回空值。引用只能来自用户资料中可识别的来源；没有可靠来源时 references 返回空数组，严禁虚构文献、数据、访谈、实验或调查结果。"""


def build_locked_academic_paper_section_prompt(
    *,
    values: dict[str, Any],
    plan: dict[str, Any],
    outline_nodes: list[dict[str, Any]],
    section_nodes: list[dict[str, Any]],
    section_index: int,
    section_total: int,
    completed_content: dict[str, str],
    reference_context: str,
) -> str:
    previous_excerpt = "\n".join(completed_content.values())[-1200:]
    requirements = locked_academic_paper_writing_requirements(values)
    references = reference_context or "没有可靠的用户参考资料；不得虚构引用、数据或研究过程。"
    return f"""你是一名学术论文作者。现在按用户锁定的目录撰写完整论文《{plan['title']}》的第 {section_index}/{section_total} 个一级章节。

全文摘要：
{plan.get('abstract', '')}

全文锁定目录：
{json.dumps([locked_outline_prompt_node(node) for node in outline_nodes], ensure_ascii=False, indent=2)}

本次必须填写的目录节点：
{json.dumps([locked_outline_prompt_node(node) for node in section_nodes], ensure_ascii=False, indent=2)}

写作要求：
{json.dumps(requirements, ensure_ascii=False, indent=2)}

已完成内容的结尾（用于保持衔接，可为空）：
{previous_excerpt}

参考资料规则与内容：
{references}

只返回 JSON，不要使用 Markdown 代码块。结构必须为：
{{"contents": {{"目录节点ID": "该节点标题下的完整正文"}}}}

contents 必须逐一包含本次列出的每个 ID，键只能使用原 ID，值必须是非空正文字符串。不要返回 title、level、children 或新的目录节点，不要在正文中重复输出标题。每个节点尽量接近其 target_words；具有下级节点的标题可写简短引导和衔接正文。内容必须连贯、可直接进入 Word，不能写提纲、写作建议、占位符或生成说明。引用只能使用用户资料中可明确识别的真实来源；不得伪造参考文献、统计数据、实验结果或调查结论。"""


def build_locked_academic_paper_missing_prompt(
    *,
    values: dict[str, Any],
    plan: dict[str, Any],
    missing_nodes: list[dict[str, Any]],
    completed_content: dict[str, str],
    reference_context: str,
) -> str:
    completed_excerpt = "\n".join(completed_content.values())[-1800:]
    references = reference_context or "没有可靠的用户参考资料；不得虚构引用、数据或研究过程。"
    return f"""你是一名学术论文作者。上一次输出遗漏了锁定目录中的部分节点，请仅补写这些缺失节点。

论文题目：{plan.get('title', '')}
论文摘要：{plan.get('abstract', '')}
必须补写的节点：
{json.dumps([locked_outline_prompt_node(node) for node in missing_nodes], ensure_ascii=False, indent=2)}

写作要求：
{json.dumps(locked_academic_paper_writing_requirements(values), ensure_ascii=False, indent=2)}

已有正文结尾（用于保持衔接）：
{completed_excerpt}

参考资料规则与内容：
{references}

只返回 {{"contents": {{"目录节点ID": "该节点的完整正文"}}}} 形式的 JSON。contents 必须包含上面每一个 ID，不能返回标题、层级、children、新节点或解释。不要在正文中重复标题，值必须为非空字符串，并尽量达到每个节点的 target_words。"""


def build_locked_academic_paper_correction_prompt(
    *,
    values: dict[str, Any],
    plan: dict[str, Any],
    section_nodes: list[dict[str, Any]],
    completed_content: dict[str, str],
    current_word_count: int,
    target_word_count: int,
    reference_context: str,
) -> str:
    current = {node["id"]: completed_content[node["id"]] for node in section_nodes}
    references = reference_context or "没有可靠的用户参考资料；不得虚构引用、数据或研究过程。"
    return f"""你是学术论文编辑。以下锁定目录子树的正文篇幅偏离目标，请在不改变任何 ID 的前提下校正篇幅，并返回各节点的完整替换正文。

论文题目：{plan.get('title', '')}
锁定节点与目标字数：
{json.dumps([locked_outline_prompt_node(node) for node in section_nodes], ensure_ascii=False, indent=2)}

当前正文（约 {current_word_count} 字，子树目标约 {target_word_count} 字）：
{json.dumps(current, ensure_ascii=False, indent=2)}

写作要求：
{json.dumps(locked_academic_paper_writing_requirements(values), ensure_ascii=False, indent=2)}

参考资料规则与内容：
{references}

只返回 {{"contents": {{"目录节点ID": "校正后的完整正文"}}}} 形式的 JSON。必须沿用且只使用列出的 ID，不要返回 title、level、children、新节点或解释，不要在正文中重复标题。每个值必须是非空字符串，总篇幅应接近 {target_word_count} 字，各节点尽量接近各自 target_words。保留正确论述，压缩重复内容或补足必要的概念界定、因果链条、对比分析和段落衔接；不得虚构引用、数据或研究过程。"""


def build_academic_paper_section_prompt(
    *,
    values: dict[str, Any],
    plan: dict[str, Any],
    section: dict[str, Any],
    section_index: int,
    section_total: int,
    completed_sections: list[dict[str, Any]],
    reference_context: str,
) -> str:
    outline = [
        {
            "title": item.get("title"),
            "target_words": item.get("target_words"),
            "subsections": item.get("subsections", []),
        }
        for item in plan["sections"]
    ]
    previous_excerpt = ""
    if completed_sections:
        previous_excerpt = flatten_paper_section_text(completed_sections[-1])[-1200:]
    requirements = {
        "paper_type": string_input(values, "paper_type"),
        "discipline": string_input(values, "discipline"),
        "education_level": string_input(values, "education_level"),
        "language": string_input(values, "language") or "zh-CN",
        "research_method": string_input(values, "research_method"),
        "writing_requirements": string_input(values, "writing_requirements"),
        "writing_style": string_input(values, "writing_style"),
        "citation_style": academic_paper_citation_style(values),
        "citation_requirements": string_input(values, "citation_requirements"),
        "reference_requirements": string_input(values, "reference_requirements"),
        "additional_requirements": string_input(values, "additional_requirements"),
    }
    references = reference_context or "没有可靠的用户参考资料；不得虚构引用、数据或研究过程。"
    return f"""你是一名学术论文作者。现在撰写完整论文《{plan['title']}》的第 {section_index}/{section_total} 个顶层章节。

全文摘要：
{plan.get('abstract', '')}

全文目录：
{json.dumps(outline, ensure_ascii=False, indent=2)}

当前章节计划：
{json.dumps(section, ensure_ascii=False, indent=2)}

写作要求：
{json.dumps(requirements, ensure_ascii=False, indent=2)}

上一章节结尾（用于保持衔接，可为空）：
{previous_excerpt}

参考资料规则与内容：
{references}

只返回 JSON，不要使用 Markdown 代码块。结构为：
{{
  "title": "保持当前一级章节标题",
  "level": 1,
  "content": "本章一级标题下的引导或总结正文",
  "children": [
    {{"title": "二级标题", "level": 2, "content": "完整正文", "children": []}}
  ]
}}

当前章节正文目标约 {section.get('target_words', 0)} 字。内容必须是连贯、可直接进入 Word 的正式论文正文，不能写提纲、写作建议、占位符或生成说明。论证应与其他章节分工清晰，避免重复。可使用最多五级标题。引用只能使用用户资料中可明确识别的真实来源；资料不足时采用不带虚构出处的学术论述，不得伪造参考文献、统计数据、实验结果或调查结论。"""


def build_academic_paper_expansion_prompt(
    *,
    values: dict[str, Any],
    plan: dict[str, Any],
    section_plan: dict[str, Any],
    written_section: dict[str, Any],
    current_word_count: int,
    reference_context: str,
) -> str:
    target_word_count = positive_int(section_plan.get("target_words"))
    gap = max(target_word_count - current_word_count, 0)
    references = reference_context or "没有可靠的用户参考资料；不得为扩充篇幅而虚构引用、数据或研究过程。"
    return f"""你是学术论文编辑。以下章节明显短于目标篇幅，请在保留原有正确内容和标题结构的基础上扩写论证，并返回完整替换版章节。

论文题目：{plan.get('title', '')}
章节计划：
{json.dumps(section_plan, ensure_ascii=False, indent=2)}

现有章节（约 {current_word_count} 字，目标约 {target_word_count} 字，缺口约 {gap} 字）：
{json.dumps(written_section, ensure_ascii=False, indent=2)}

补充写作要求：
{json.dumps({
    'writing_requirements': string_input(values, 'writing_requirements'),
    'writing_style': string_input(values, 'writing_style'),
    'research_method': string_input(values, 'research_method'),
    'citation_style': academic_paper_citation_style(values),
    'citation_requirements': string_input(values, 'citation_requirements'),
}, ensure_ascii=False, indent=2)}

参考资料规则与内容：
{references}

只返回与原章节相同结构的 JSON：
{{"title":"一级章节标题","level":1,"content":"完整正文","children":[{{"title":"二级标题","level":2,"content":"完整正文","children":[]}}]}}

返回的是完整替换版，不是增量补丁。优先补充概念界定、因果链条、对比分析、机制解释和段落衔接，避免同义反复。不得加入无法由用户资料支持的具体数据、实验、调查、案例细节或虚构引用。"""


def build_academic_paper_review_prompt(
    values: dict[str, Any],
    plan: dict[str, Any],
    sections: list[dict[str, Any]],
) -> str:
    chapter_overview = []
    for index, section in enumerate(sections, start=1):
        content = flatten_paper_section_text(section)
        chapter_overview.append(
            {
                "index": index,
                "title": section.get("title"),
                "word_count": count_text_words(content),
                "opening_excerpt": content[:500],
                "ending_excerpt": content[-700:],
            }
        )
    keyword_count = academic_paper_keyword_target_count(values)
    keyword_example = f'["恰好 {keyword_count} 个最终关键词"]' if keyword_count else "[]"
    return f"""你是论文终审编辑。请对下列论文进行一次轻量的一致性审校，不要重写全文。

用户指定题目：{first_nonempty_input(values, 'paper_title', 'title')}
当前题目：{plan.get('title', '')}
当前摘要：{plan.get('abstract', '')}
当前关键词：{json.dumps(plan.get('keywords', []), ensure_ascii=False)}
各章概要与首尾摘录：
{json.dumps(chapter_overview, ensure_ascii=False, indent=2)}

附加部分要求：
{json.dumps({
    'acknowledgements_enabled': boolean_input(values, 'acknowledgements_enabled', default=False),
    'acknowledgements_requirements': string_input(values, 'acknowledgements_requirements'),
    'appendix_enabled': boolean_input(values, 'appendix_enabled', default=False),
    'appendix_title': string_input(values, 'appendix_title'),
    'appendix_requirements': string_input(values, 'appendix_requirements'),
}, ensure_ascii=False, indent=2)}

只返回 JSON，不要使用 Markdown 代码块：
{{
  "final_title": "保持用户指定题目；未指定时可做小幅学术化调整",
  "abstract": "与全文研究目的、方法、论证和结论一致的最终摘要",
  "keywords": {keyword_example},
  "conclusion_adjustments": "仅在结论明显不一致时返回可替换结论正文，否则为空字符串",
  "acknowledgements": "启用致谢时返回符合要求的完整致谢，否则为空字符串",
  "appendices": [{{"title": "附录标题", "content": "启用附录时返回完整附录内容"}}],
  "consistency_notes": ["已检查或仍需用户核验的简短事项"]
}}

{academic_paper_keyword_instruction(values)}重点检查题目与正文一致性、摘要是否覆盖全文、核心术语是否统一、相邻章节是否衔接、结论是否回答研究问题。不得新增正文中不存在的实验、数据、调查或引用，不得编造参考文献。"""


def build_locked_academic_paper_review_prompt(
    values: dict[str, Any],
    plan: dict[str, Any],
    sections: list[dict[str, Any]],
) -> str:
    chapter_overview = []
    for index, section in enumerate(sections, start=1):
        content = flatten_paper_section_text(section)
        chapter_overview.append(
            {
                "index": index,
                "id": section.get("id"),
                "title": section.get("title"),
                "word_count": count_text_words(content),
                "opening_excerpt": content[:500],
                "ending_excerpt": content[-700:],
            }
        )
    keyword_count = academic_paper_keyword_target_count(values)
    keyword_example = f'["恰好 {keyword_count} 个最终关键词"]' if keyword_count else "[]"
    return f"""你是论文终审编辑。论文目录已经由用户锁定，请只审校题目、摘要、关键词和附加部分的一致性，不能重写正文或目录。

用户指定题目：{first_nonempty_input(values, 'paper_title', 'title')}
当前题目：{plan.get('title', '')}
当前摘要：{plan.get('abstract', '')}
当前关键词：{json.dumps(plan.get('keywords', []), ensure_ascii=False)}
各章概要与首尾摘录：
{json.dumps(chapter_overview, ensure_ascii=False, indent=2)}

附加部分要求：
{json.dumps({
    'acknowledgements_enabled': boolean_input(values, 'acknowledgements_enabled', default=False),
    'acknowledgements_requirements': string_input(values, 'acknowledgements_requirements'),
    'appendix_enabled': boolean_input(values, 'appendix_enabled', default=False),
    'appendix_title': string_input(values, 'appendix_title'),
    'appendix_requirements': string_input(values, 'appendix_requirements'),
}, ensure_ascii=False, indent=2)}

只返回 JSON，不要使用 Markdown 代码块：
{{
  "final_title": "保持用户指定题目；未指定时可做小幅学术化调整",
  "abstract": "与全文研究目的、方法、论证和结论一致的最终摘要",
  "keywords": {keyword_example},
  "acknowledgements": "启用致谢时返回符合要求的完整致谢，否则为空字符串",
  "appendices": [{{"title": "附录标题", "content": "启用附录时返回完整附录内容"}}],
  "consistency_notes": ["已检查或仍需用户核验的简短事项"]
}}

不得返回 conclusion_adjustments、sections、outline、contents、标题层级或正文替换内容。{academic_paper_keyword_instruction(values)}重点检查题目与正文一致性、摘要是否覆盖全文、核心术语是否统一、相邻章节是否衔接、结论是否回答研究问题。不得新增正文中不存在的实验、数据、调查或引用，不得编造参考文献。"""


def apply_academic_paper_review(
    plan: dict[str, Any],
    sections: list[dict[str, Any]],
    review: dict[str, Any],
    values: dict[str, Any],
) -> list[str]:
    explicit_title = first_nonempty_input(values, "paper_title", "title")
    reviewed_title = clean_string(review.get("final_title"))
    if not explicit_title and reviewed_title:
        plan["title"] = reviewed_title
    reviewed_abstract = clean_string(review.get("abstract"))
    if reviewed_abstract:
        plan["abstract"] = reviewed_abstract
    reviewed_keywords = normalize_string_list(review.get("keywords"))
    plan["keywords"] = normalize_academic_paper_keywords(
        reviewed_keywords or plan.get("keywords"),
        values,
        fallback_values=plan.get("keywords"),
    )
    conclusion = clean_string(review.get("conclusion_adjustments"))
    if conclusion and sections:
        sections[-1]["content"] = conclusion
    if boolean_input(values, "acknowledgements_enabled", default=False):
        acknowledgements = clean_string(review.get("acknowledgements"))
        if acknowledgements:
            plan["acknowledgements"] = acknowledgements
    if boolean_input(values, "appendix_enabled", default=False):
        appendices = normalize_paper_appendices(review.get("appendices") or review.get("appendix"))
        if appendices:
            plan["appendices"] = appendices
    notes = normalize_string_list(review.get("consistency_notes"))
    if not notes:
        notes = ["已完成题目、摘要、关键词、章节衔接和结论的一致性检查。"]
    return notes[:10]


def academic_paper_brief(values: dict[str, Any], word_count: int) -> dict[str, Any]:
    return {
        "topic": string_input(values, "topic"),
        "preferred_title": first_nonempty_input(values, "paper_title", "title"),
        "paper_type": string_input(values, "paper_type") or "学术论文",
        "discipline": string_input(values, "discipline"),
        "education_level": string_input(values, "education_level"),
        "language": string_input(values, "language") or "zh-CN",
        "target_word_count": word_count,
        "outline_requirements": first_nonempty_input(values, "outline_requirements", "directory_structure", "toc_structure"),
        "abstract_enabled": boolean_input(values, "abstract_enabled", default=True),
        "abstract_requirements": first_nonempty_input(values, "abstract_requirements", "abstract_requirement"),
        "keywords_enabled": boolean_input(values, "keywords_enabled", default=True),
        "keywords_count": optional_int_input(values, "keywords_count", default=5, minimum=2, maximum=10),
        "keywords_requirements": string_input(values, "keywords_requirements"),
        "citation_style": academic_paper_citation_style(values),
        "citation_requirements": string_input(values, "citation_requirements"),
        "references_enabled": boolean_input(values, "references_enabled", default=True),
        "reference_requirements": string_input(values, "reference_requirements"),
        "acknowledgements_enabled": boolean_input(values, "acknowledgements_enabled", default=False),
        "acknowledgements_requirements": string_input(values, "acknowledgements_requirements"),
        "appendix_enabled": boolean_input(values, "appendix_enabled", default=False),
        "appendix_requirements": string_input(values, "appendix_requirements"),
        "research_method": string_input(values, "research_method"),
        "writing_requirements": string_input(values, "writing_requirements"),
        "writing_style": string_input(values, "writing_style") or "严谨、清晰、符合学术规范",
        "additional_requirements": string_input(values, "additional_requirements"),
    }


def parse_academic_paper_outline_spec(
    value: Any,
    word_count: int,
    *,
    abstract_enabled: bool = True,
) -> list[dict[str, Any]]:
    if isinstance(value, str):
        try:
            value = json.loads(value)
        except ValueError as exc:
            raise WorkerFailure("PAPER_OUTLINE_SPEC_INVALID", "outline_spec 必须是有效的 JSON 对象。") from exc
    if not isinstance(value, dict):
        raise WorkerFailure("PAPER_OUTLINE_SPEC_INVALID", "outline_spec 必须是对象。")
    version = value.get("version")
    if isinstance(version, bool) or not isinstance(version, int) or version != 1:
        raise WorkerFailure("PAPER_OUTLINE_SPEC_INVALID", "outline_spec.version 必须为 1。")
    raw_nodes = value.get("nodes")
    if not isinstance(raw_nodes, list):
        raise WorkerFailure("PAPER_OUTLINE_SPEC_INVALID", "outline_spec.nodes 必须是扁平节点数组。")
    if not 1 <= len(raw_nodes) <= PAPER_OUTLINE_MAX_NODES:
        raise WorkerFailure(
            "PAPER_OUTLINE_SPEC_INVALID",
            f"outline_spec.nodes 数量必须在 1 到 {PAPER_OUTLINE_MAX_NODES} 之间。",
        )

    nodes: list[dict[str, Any]] = []
    seen_ids: set[str] = set()
    previous_level = 0
    for index, raw_node in enumerate(raw_nodes, start=1):
        if not isinstance(raw_node, dict):
            raise WorkerFailure("PAPER_OUTLINE_SPEC_INVALID", f"目录第 {index} 个节点必须是对象。")
        node_id = clean_string(raw_node.get("id"))
        title = clean_string(raw_node.get("title"))
        level = raw_node.get("level")
        if not node_id or len(node_id) > 128:
            raise WorkerFailure("PAPER_OUTLINE_SPEC_INVALID", f"目录第 {index} 个节点的 id 无效。")
        if node_id in seen_ids:
            raise WorkerFailure("PAPER_OUTLINE_SPEC_INVALID", f"目录节点 id 重复：{node_id}。")
        if not title or len(title) > 200:
            raise WorkerFailure("PAPER_OUTLINE_SPEC_INVALID", f"目录第 {index} 个节点的 title 无效。")
        if isinstance(level, bool) or not isinstance(level, int) or not 1 <= level <= 5:
            raise WorkerFailure("PAPER_OUTLINE_SPEC_INVALID", f"目录节点 {node_id} 的 level 必须是 1 到 5。")
        if index == 1 and level != 1:
            raise WorkerFailure("PAPER_OUTLINE_SPEC_INVALID", "目录首节点的 level 必须为 1。")
        if previous_level and level > previous_level + 1:
            raise WorkerFailure("PAPER_OUTLINE_SPEC_INVALID", f"目录节点 {node_id} 存在层级跳跃。")
        nodes.append({"id": node_id, "title": title, "level": level})
        seen_ids.add(node_id)
        previous_level = level

    abstract_budget = academic_paper_abstract_word_budget(word_count) if abstract_enabled else 0
    allocate_locked_outline_words(nodes, max(word_count - abstract_budget, len(nodes)))
    return nodes


def academic_paper_abstract_word_budget(word_count: int) -> int:
    return min(max(round(word_count * 0.05), 120), 300)


def rebalance_locked_outline_words_for_abstract(
    nodes: list[dict[str, Any]],
    target_word_count: int,
    abstract: str,
) -> None:
    abstract_words = count_text_words(abstract)
    section_budget = max(target_word_count - abstract_words, len(nodes))
    allocate_locked_outline_words(nodes, section_budget)


def locked_paper_word_count_limits(target_word_count: int) -> tuple[int, int]:
    if target_word_count <= 0:
        return 0, 0
    lower_limit = (
        target_word_count * PAPER_LOCKED_WORD_COUNT_MIN_PERCENT + 99
    ) // 100
    upper_limit = target_word_count * PAPER_LOCKED_WORD_COUNT_MAX_PERCENT // 100
    return lower_limit, max(lower_limit, upper_limit)


def paper_word_count_within_tolerance(actual_word_count: int, target_word_count: int) -> bool:
    lower_limit, upper_limit = locked_paper_word_count_limits(target_word_count)
    return target_word_count > 0 and lower_limit <= actual_word_count <= upper_limit


def normalize_locked_paper_terminal(value: str) -> str:
    text = value.strip()
    match = _PAPER_INCOMPLETE_TERMINAL_RE.search(text)
    if match is None:
        return text
    punctuation = text[match.start()]
    has_chinese = bool(re.search(r"[\u3400-\u4dbf\u4e00-\u9fff]", text[: match.start()]))
    terminator = "。" if punctuation in "，；：" or has_chinese else "."
    return f"{text[:match.start()].rstrip()}{terminator}{match.group(1)}"


def locked_paper_boundary_prefixes(
    text: str,
    exact_prefix: str,
    tokens: list[re.Match[str]],
    boundary_pattern: re.Pattern[str],
    min_word_count: int,
) -> tuple[str, str]:
    preferred = ""
    below_minimum = ""
    counted_tokens = 0
    for boundary in boundary_pattern.finditer(exact_prefix):
        boundary_end = boundary.end()
        while boundary_end < len(exact_prefix) and exact_prefix[boundary_end] in _PAPER_SENTENCE_CLOSERS:
            boundary_end += 1
        while counted_tokens < len(tokens) and tokens[counted_tokens].end() <= boundary_end:
            counted_tokens += 1
        if counted_tokens <= 0:
            continue
        candidate = normalize_locked_paper_terminal(text[:boundary_end])
        if counted_tokens >= min_word_count:
            preferred = candidate
        else:
            below_minimum = candidate
    return preferred, below_minimum


def select_locked_paper_fallback(
    candidates: list[str],
    max_word_count: int,
    min_word_count: int,
) -> tuple[str, str]:
    meeting_minimum = ""
    longest_below_minimum = ""
    longest_below_count = 0
    for candidate in candidates:
        candidate_words = count_text_words(candidate)
        if candidate_words <= 0 or candidate_words > max_word_count:
            continue
        if candidate_words >= min_word_count and not meeting_minimum:
            meeting_minimum = candidate
        if candidate_words > longest_below_count:
            longest_below_minimum = candidate
            longest_below_count = candidate_words
    return meeting_minimum, longest_below_minimum


def repeat_locked_paper_fallback_sentences(
    sentences: list[str],
    max_word_count: int,
    min_word_count: int,
) -> str:
    selected: list[str] = []
    selected_words = 0
    while selected_words < min_word_count:
        added_sentence = False
        for sentence in sentences:
            sentence_words = count_text_words(sentence)
            if sentence_words <= 0 or selected_words + sentence_words > max_word_count:
                continue
            selected.append(sentence)
            selected_words += sentence_words
            added_sentence = True
            if selected_words >= min_word_count:
                return " ".join(selected)
        if not added_sentence:
            break
    return ""


def build_locked_paper_fallback_sentence(
    title: str,
    source_text: str,
    max_word_count: int,
    min_word_count: int,
) -> str:
    normalized_title = " ".join(title.strip().split()).strip(
        " ,，.。!！?？;；:：\"'“”‘’（）()【】[]《》"
    )
    has_chinese = bool(
        re.search(r"[\u3400-\u4dbf\u4e00-\u9fff]", f"{normalized_title}{source_text}")
    )

    if has_chinese:
        clauses = [
            "说明核心概念",
            "界定研究范围",
            "梳理形成背景",
            "分析影响因素",
            "讨论作用机制",
            "比较现实表现",
            "识别主要差异",
            "评估理论价值",
            "说明实践意义",
            "归纳基本结论",
            "明确适用条件",
            "指出分析边界",
            "联系整体研究",
            "支持后续论证",
            "回应研究目标",
            "总结关键认识",
        ]

        def chinese_candidates(base: str) -> list[str]:
            candidates = [f"{base}。"]
            current = base
            for clause in clauses:
                current = f"{current}，{clause}"
                candidates.append(f"{current}。")
            return candidates

        primary = chinese_candidates(
            f"本节围绕{normalized_title}展开分析" if normalized_title else "本节围绕该主题展开分析"
        )
        generic = chinese_candidates("本节围绕该主题展开分析")
        primary_match, primary_longest = select_locked_paper_fallback(
            primary,
            max_word_count,
            min_word_count,
        )
        generic_match, generic_longest = select_locked_paper_fallback(
            generic,
            max_word_count,
            min_word_count,
        )
        if primary_match:
            return primary_match
        if generic_match:
            return generic_match
        topic = normalized_title or "该主题"
        expanded = repeat_locked_paper_fallback_sentences(
            [
                f"本节围绕{topic}展开分析，说明核心概念，界定研究范围，并梳理相关问题的形成背景。",
                "围绕该主题，分析重点包括主要影响因素、作用机制、现实表现及其差异。",
                "结合整体研究，本文评估相关问题的理论价值与实践意义，并明确结论适用的条件。",
                "在此基础上，本节归纳能够支持的基本判断，指出分析边界，并为后续论证提供依据。",
                "本节继续说明该主题。",
                "相关要点由此明确。",
            ],
            max_word_count,
            min_word_count,
        )
        if expanded:
            return expanded
        if primary_longest:
            return primary_longest
        if generic_longest:
            return generic_longest
        if max_word_count >= 4:
            return "本节概述。"
        if max_word_count >= 2:
            return "概述。"
        return "述。" if max_word_count >= 1 else ""

    if not normalized_title:
        return ""

    def english_candidates(base: str) -> list[str]:
        candidates = [f"{base}.", f"{base} systematically."]
        current = base
        for clause in (
            "defines its scope",
            "reviews its context",
            "examines the main relationships",
            "compares relevant conditions",
            "explains the practical implications",
            "states the supported conclusion",
        ):
            current = f"{current}, {clause}"
            candidates.append(f"{current}.")
        return candidates

    primary = english_candidates(f"This section examines {normalized_title}")
    generic = english_candidates("This section examines the topic")
    primary_match, primary_longest = select_locked_paper_fallback(primary, max_word_count, min_word_count)
    generic_match, generic_longest = select_locked_paper_fallback(generic, max_word_count, min_word_count)
    expanded = repeat_locked_paper_fallback_sentences(
        [
            f"This section examines {normalized_title}, defines its scope, and reviews the relevant context.",
            "The discussion addresses key relationships, operating conditions, and practical implications.",
            "It states the conclusions supported by the analysis and identifies the applicable limits.",
            "These points provide a clear basis for the remaining discussion.",
            "The topic remains the focus.",
        ],
        max_word_count,
        min_word_count,
    )
    return primary_match or generic_match or expanded or primary_longest or generic_longest or (
        "Summary." if max_word_count >= 1 else ""
    )


def trim_locked_paper_content_to_word_limit(
    value: str,
    max_word_count: int,
    *,
    min_word_count: int = 1,
    fallback_title: str = "",
) -> str:
    text = normalize_locked_paper_terminal(value)
    tokens = list(_PAPER_WORD_TOKEN_RE.finditer(text))
    if not tokens or len(tokens) <= max_word_count:
        return text

    max_word_count = max(1, min(max_word_count, len(tokens)))
    min_word_count = max(1, min(min_word_count, max_word_count))
    exact_end = tokens[max_word_count].start() if max_word_count < len(tokens) else len(text)
    exact_prefix = text[:exact_end].rstrip()

    sentence_preferred, sentence_below_minimum = locked_paper_boundary_prefixes(
        text,
        exact_prefix,
        tokens,
        _PAPER_SENTENCE_BOUNDARY_RE,
        min_word_count,
    )
    clause_preferred, clause_below_minimum = locked_paper_boundary_prefixes(
        text,
        exact_prefix,
        tokens,
        _PAPER_CLAUSE_BOUNDARY_RE,
        min_word_count,
    )
    for boundary_prefix in (
        sentence_preferred,
        sentence_below_minimum,
        clause_preferred,
        clause_below_minimum,
    ):
        if boundary_prefix:
            return boundary_prefix

    fallback = build_locked_paper_fallback_sentence(
        fallback_title,
        text,
        max_word_count,
        min_word_count,
    )
    if fallback:
        return fallback
    return exact_prefix


def hard_cap_locked_outline_group_words(
    group: list[dict[str, Any]],
    content_by_id: dict[str, str],
) -> int:
    for node in group:
        node_id = node["id"]
        content_by_id[node_id] = normalize_locked_paper_terminal(content_by_id.get(node_id, ""))

    target_words = sum(positive_int(node.get("target_words")) for node in group)
    lower_limit, upper_limit = locked_paper_word_count_limits(target_words)
    actual_words = locked_outline_group_word_count(group, content_by_id)
    if target_words <= 0 or actual_words <= upper_limit:
        return actual_words

    node_counts: list[tuple[int, dict[str, Any], int, int]] = []
    for node in group:
        node_actual = count_text_words(content_by_id.get(node["id"], ""))
        node_target = positive_int(node.get("target_words"))
        node_counts.append((node_actual - node_target, node, node_actual, node_target))
    node_counts.sort(key=lambda item: item[0], reverse=True)

    for node_excess, node, node_actual, node_target in node_counts:
        if actual_words <= upper_limit:
            break
        removable = min(actual_words - upper_limit, max(node_excess, 0), max(node_actual - 1, 0))
        if removable <= 0:
            continue
        other_words = actual_words - node_actual
        max_words = node_actual - removable
        min_words = max(1, node_target, lower_limit - other_words)
        trimmed = trim_locked_paper_content_to_word_limit(
            content_by_id[node["id"]],
            max_words,
            min_word_count=min_words,
            fallback_title=clean_string(node.get("title")),
        )
        trimmed_words = count_text_words(trimmed)
        content_by_id[node["id"]] = trimmed
        actual_words = other_words + trimmed_words

    if actual_words > upper_limit:
        fallback_nodes = sorted(
            group,
            key=lambda node: count_text_words(content_by_id.get(node["id"], "")),
            reverse=True,
        )
        for node in fallback_nodes:
            if actual_words <= upper_limit:
                break
            node_actual = count_text_words(content_by_id.get(node["id"], ""))
            removable = min(actual_words - upper_limit, max(node_actual - 1, 0))
            if removable <= 0:
                continue
            other_words = actual_words - node_actual
            trimmed = trim_locked_paper_content_to_word_limit(
                content_by_id[node["id"]],
                node_actual - removable,
                min_word_count=max(1, lower_limit - other_words),
                fallback_title=clean_string(node.get("title")),
            )
            trimmed_words = count_text_words(trimmed)
            content_by_id[node["id"]] = trimmed
            actual_words = other_words + trimmed_words

    refill_nodes = sorted(
        group,
        key=lambda node: positive_int(node.get("target_words")),
        reverse=True,
    )
    while actual_words < lower_limit:
        added_words = 0
        for node in refill_nodes:
            available_words = upper_limit - actual_words
            required_words = lower_limit - actual_words
            if available_words <= 0:
                break
            node_id = node["id"]
            current_content = content_by_id.get(node_id, "").strip()
            addition = build_locked_paper_fallback_sentence(
                clean_string(node.get("title")),
                current_content,
                available_words,
                required_words,
            )
            addition_words = count_text_words(addition)
            if addition_words <= 0:
                continue
            content_by_id[node_id] = f"{current_content}\n\n{addition}".strip()
            actual_words += addition_words
            added_words += addition_words
            if actual_words >= lower_limit:
                break
        if added_words <= 0:
            break
    return actual_words


def allocate_locked_outline_words(nodes: list[dict[str, Any]], word_count: int) -> None:
    weights = [
        60 if index + 1 < len(nodes) and nodes[index + 1]["level"] > node["level"] else 100
        for index, node in enumerate(nodes)
    ]
    allocations = [1] * len(nodes)
    remaining = max(word_count - len(nodes), 0)
    total_weight = sum(weights)
    if remaining and total_weight:
        extra = [(remaining * weight) // total_weight for weight in weights]
        for index, value in enumerate(extra):
            allocations[index] += value
        undistributed = word_count - sum(allocations)
        remainders = sorted(
            range(len(nodes)),
            key=lambda index: (remaining * weights[index]) % total_weight,
            reverse=True,
        )
        for index in remainders[:undistributed]:
            allocations[index] += 1
    for node, target_words in zip(nodes, allocations):
        node["target_words"] = target_words


def locked_outline_top_level_groups(nodes: list[dict[str, Any]]) -> list[list[dict[str, Any]]]:
    groups: list[list[dict[str, Any]]] = []
    for node in nodes:
        if node["level"] == 1:
            groups.append([])
        groups[-1].append(node)
    return groups


def build_locked_academic_paper_tree(
    nodes: list[dict[str, Any]],
    content_by_id: dict[str, str],
) -> list[dict[str, Any]]:
    roots: list[dict[str, Any]] = []
    stack: list[dict[str, Any]] = []
    for node in nodes:
        level = node["level"]
        written = {
            "id": node["id"],
            "title": node["title"],
            "level": level,
            "target_words": node["target_words"],
            "content": content_by_id.get(node["id"], ""),
            "children": [],
        }
        while stack and stack[-1]["level"] >= level:
            stack.pop()
        if level == 1:
            roots.append(written)
        else:
            stack[-1]["children"].append(written)
        stack.append(written)
    return roots


def locked_academic_paper_outline_matches(
    sections: list[dict[str, Any]],
    outline_nodes: list[dict[str, Any]],
) -> bool:
    actual: list[tuple[str, str, int]] = []

    def visit(node: dict[str, Any]) -> None:
        actual.append(
            (
                clean_string(node.get("id")),
                clean_string(node.get("title")),
                positive_int(node.get("level")),
            )
        )
        for child in node.get("children", []):
            if isinstance(child, dict):
                visit(child)

    for section in sections:
        visit(section)
    expected = [(node["id"], node["title"], node["level"]) for node in outline_nodes]
    return actual == expected


def locked_outline_prompt_node(node: dict[str, Any]) -> dict[str, Any]:
    return {key: node[key] for key in ("id", "title", "level", "target_words")}


def locked_academic_paper_writing_requirements(values: dict[str, Any]) -> dict[str, Any]:
    return {
        "paper_type": string_input(values, "paper_type"),
        "discipline": string_input(values, "discipline"),
        "education_level": string_input(values, "education_level"),
        "language": string_input(values, "language") or "zh-CN",
        "research_method": string_input(values, "research_method"),
        "writing_requirements": string_input(values, "writing_requirements"),
        "writing_style": string_input(values, "writing_style"),
        "citation_style": academic_paper_citation_style(values),
        "citation_requirements": string_input(values, "citation_requirements"),
        "reference_requirements": string_input(values, "reference_requirements"),
        "additional_requirements": string_input(values, "additional_requirements"),
    }


def parse_locked_academic_paper_contents(raw: str, allowed_ids: set[str]) -> dict[str, str]:
    parsed = parse_json_object(raw)
    contents = parsed.get("contents") if isinstance(parsed.get("contents"), dict) else parsed
    result: dict[str, str] = {}
    for node_id, value in contents.items():
        if node_id not in allowed_ids or not isinstance(value, str):
            continue
        content = value.strip()
        if content:
            result[node_id] = content
    return result


def locked_outline_group_word_count(group: list[dict[str, Any]], content_by_id: dict[str, str]) -> int:
    return sum(count_text_words(content_by_id.get(node["id"], "")) for node in group)


def locked_outline_word_count_deviations(
    groups: list[list[dict[str, Any]]],
    content_by_id: dict[str, str],
) -> list[dict[str, Any]]:
    deviations: list[dict[str, Any]] = []
    for group in groups:
        target_words = sum(positive_int(node.get("target_words")) for node in group)
        actual_words = locked_outline_group_word_count(group, content_by_id)
        if target_words <= 0 or paper_word_count_within_tolerance(actual_words, target_words):
            continue
        deviations.append(
            {
                "node_id": group[0]["id"],
                "target_word_count": target_words,
                "actual_word_count": actual_words,
                "deviation_percent": round(((actual_words - target_words) / target_words) * 100, 1),
            }
        )
    return deviations


def parse_academic_paper_plan(
    raw: str,
    values: dict[str, Any],
    word_count: int,
    *,
    has_reference_material: bool = False,
    locked_outline_nodes: list[dict[str, Any]] | None = None,
    reference_registry: dict[str, Any] | None = None,
) -> dict[str, Any]:
    parsed = parse_json_object(raw)
    explicit_title = first_nonempty_input(values, "paper_title", "title")
    topic = string_input(values, "topic")
    title = explicit_title or clean_string(parsed.get("title")) or topic
    abstract = clean_string(parsed.get("abstract")) or f"本文围绕{topic}展开分析，梳理相关理论与现实问题，并在系统论证的基础上形成研究结论。"
    keywords = normalize_academic_paper_keywords(parsed.get("keywords"), values)

    if locked_outline_nodes is not None:
        sections = build_locked_academic_paper_tree(locked_outline_nodes, {})
    else:
        raw_sections = parsed.get("sections") or parsed.get("outline")
        sections = []
        if isinstance(raw_sections, list):
            for index, item in enumerate(raw_sections[:8], start=1):
                normalized = normalize_paper_plan_section(item, index)
                if normalized is not None:
                    sections.append(normalized)
        if len(sections) < 2:
            sections = fallback_paper_sections(values, word_count)
        allocate_paper_section_words(sections, word_count)

    references_enabled = boolean_input(values, "references_enabled", default=True)
    if reference_registry is not None and references_enabled:
        references = list(reference_registry.get("bibliography") or [])
    else:
        references = normalize_string_list(parsed.get("references")) if has_reference_material and references_enabled else []
        supplied_references = normalize_string_list(values.get("references"))
        if supplied_references and references_enabled:
            references = supplied_references
    acknowledgements = ""
    if boolean_input(values, "acknowledgements_enabled", default=False):
        acknowledgements = string_input(values, "acknowledgements") or clean_string(parsed.get("acknowledgements"))
    appendices = []
    if boolean_input(values, "appendix_enabled", default=False):
        appendices = normalize_paper_appendices(parsed.get("appendices") or parsed.get("appendix"))
    return {
        "title": title,
        "abstract": abstract,
        "keywords": keywords,
        "sections": sections,
        "references": references,
        "acknowledgements": acknowledgements,
        "appendices": appendices,
    }


def parse_json_object(raw: str) -> dict[str, Any]:
    candidate = (raw or "").strip()
    if candidate.startswith("```"):
        candidate = candidate.split("\n", 1)[1] if "\n" in candidate else candidate[3:]
        candidate = candidate.rsplit("```", 1)[0].strip()
    try:
        parsed = json.loads(candidate)
        return parsed if isinstance(parsed, dict) else {}
    except (TypeError, ValueError):
        pass
    opening = candidate.find("{")
    if opening >= 0:
        try:
            parsed, _ = json.JSONDecoder().raw_decode(candidate[opening:])
            return parsed if isinstance(parsed, dict) else {}
        except (TypeError, ValueError):
            pass
    return {}


def normalize_paper_plan_section(value: Any, index: int) -> dict[str, Any] | None:
    if isinstance(value, str):
        title = value.strip()
        source: dict[str, Any] = {}
    elif isinstance(value, dict):
        source = value
        title = clean_string(source.get("title") or source.get("heading") or source.get("name"))
    else:
        return None
    if not title:
        return None
    target_words = positive_int(source.get("target_words") or source.get("word_count"))
    subsections = source.get("subsections") or source.get("children") or []
    return {
        "title": title,
        "level": 1,
        "target_words": target_words,
        "purpose": clean_string(source.get("purpose") or source.get("description")),
        "key_points": normalize_string_list(source.get("key_points") or source.get("points")),
        "subsections": normalize_outline_items(subsections),
        "index": index,
    }


def normalize_outline_items(value: Any) -> list[dict[str, Any]]:
    if not isinstance(value, list):
        return []
    items: list[dict[str, Any]] = []
    for item in value[:20]:
        if isinstance(item, str) and item.strip():
            items.append({"title": item.strip(), "level": 2})
        elif isinstance(item, dict):
            title = clean_string(item.get("title") or item.get("heading") or item.get("name"))
            if title:
                items.append({"title": title, "level": min(max(positive_int(item.get("level")) or 2, 2), 5)})
    return items


def normalize_paper_appendices(value: Any) -> list[dict[str, Any]]:
    if isinstance(value, str) and value.strip():
        return [{"title": "附录", "content": value.strip()}]
    if not isinstance(value, list):
        return []
    appendices: list[dict[str, Any]] = []
    for index, item in enumerate(value[:10], start=1):
        if isinstance(item, str) and item.strip():
            appendices.append({"title": f"附录 {index}", "content": item.strip()})
        elif isinstance(item, dict):
            content = normalized_section_content(item.get("content") or item.get("body") or item.get("text"))
            if content:
                appendices.append(
                    {
                        "title": clean_string(item.get("title")) or f"附录 {index}",
                        "content": content,
                    }
                )
    return appendices


def fallback_paper_sections(values: dict[str, Any], word_count: int) -> list[dict[str, Any]]:
    custom_titles = parse_outline_requirement_titles(
        first_nonempty_input(values, "outline_requirements", "directory_structure", "toc_structure")
    )
    count = recommended_paper_section_count(word_count)
    templates = {
        4: ["绪论", "理论基础与相关研究", "核心问题分析", "结论与展望"],
        5: ["绪论", "理论基础与相关研究", "研究设计与分析框架", "核心问题分析", "结论与展望"],
        6: ["绪论", "理论基础与相关研究", "研究设计与分析框架", "现状与核心问题分析", "讨论与对策建议", "结论与展望"],
        7: ["绪论", "理论基础与相关研究", "研究设计与分析框架", "现状分析", "核心问题与成因", "讨论与对策建议", "结论与展望"],
        8: ["绪论", "理论基础与相关研究", "研究设计与分析框架", "现状分析", "案例或实证分析", "核心问题与成因", "讨论与对策建议", "结论与展望"],
    }
    titles = custom_titles if len(custom_titles) >= 2 else templates[count]
    return [
        {
            "title": title,
            "level": 1,
            "target_words": 0,
            "purpose": "",
            "key_points": [],
            "subsections": [],
            "index": index,
        }
        for index, title in enumerate(titles[:8], start=1)
    ]


def parse_outline_requirement_titles(value: str) -> list[str]:
    titles: list[str] = []
    for line in value.splitlines():
        candidate = line.strip()
        if not candidate or len(candidate) > 80:
            continue
        if re.match(r"^\d+\.\d+", candidate):
            continue
        candidate = re.sub(
            r"^(?:第[一二三四五六七八九十百\d]+[章节篇部分]|[一二三四五六七八九十]+[、.．]|\d+[、.．\s]+|[-*#]+\s*)",
            "",
            candidate,
        ).strip()
        if candidate and candidate not in titles:
            titles.append(candidate)
    return titles[:10]


def recommended_paper_section_count(word_count: int) -> int:
    if word_count <= 3000:
        return 4
    if word_count <= 6000:
        return 5
    if word_count <= 12000:
        return 6
    if word_count <= 20000:
        return 7
    return 8


def allocate_paper_section_words(sections: list[dict[str, Any]], word_count: int) -> None:
    weights = [max(positive_int(section.get("target_words")), 0) for section in sections]
    if not any(weights):
        weights = [80 if index in {0, len(sections) - 1} else 100 for index in range(len(sections))]
    total_weight = sum(weights) or len(sections)
    assigned = 0
    for index, (section, weight) in enumerate(zip(sections, weights)):
        if index == len(sections) - 1:
            target = max(word_count - assigned, 1)
        else:
            target = max(round(word_count * weight / total_weight), 1)
            assigned += target
        section["target_words"] = target


def parse_academic_paper_section(raw: str, expected: dict[str, Any]) -> dict[str, Any]:
    parsed = parse_json_object(raw)
    title = clean_string(expected.get("title"))
    if not parsed:
        return {"title": title, "level": 1, "content": strip_markdown_fence(raw), "children": []}
    content = normalized_section_content(parsed.get("content") or parsed.get("body") or parsed.get("text"))
    children_value = parsed.get("children") or parsed.get("subsections") or []
    children = normalize_written_section_children(children_value, default_level=2)
    if not content and not children:
        content = strip_markdown_fence(raw)
    return {"title": title, "level": 1, "content": content, "children": children}


def normalize_written_section_children(value: Any, *, default_level: int) -> list[dict[str, Any]]:
    if not isinstance(value, list):
        return []
    children: list[dict[str, Any]] = []
    for item in value:
        if isinstance(item, str) and item.strip():
            children.append({"title": "", "level": default_level, "content": item.strip(), "children": []})
            continue
        if not isinstance(item, dict):
            continue
        title = clean_string(item.get("title") or item.get("heading") or item.get("name"))
        content = normalized_section_content(item.get("content") or item.get("body") or item.get("text"))
        level = min(max(positive_int(item.get("level")) or default_level, default_level), 5)
        nested = normalize_written_section_children(item.get("children") or [], default_level=min(level + 1, 5))
        if title or content or nested:
            children.append({"title": title, "level": level, "content": content, "children": nested})
    return children


def normalized_section_content(value: Any) -> str:
    if isinstance(value, str):
        return value.strip()
    if isinstance(value, list):
        return "\n\n".join(str(item).strip() for item in value if str(item).strip())
    return ""


def strip_markdown_fence(value: str) -> str:
    candidate = value.strip()
    if candidate.startswith("```"):
        candidate = candidate.split("\n", 1)[1] if "\n" in candidate else candidate[3:]
        candidate = candidate.rsplit("```", 1)[0]
    return candidate.strip()


def build_academic_paper_payload(
    plan: dict[str, Any],
    sections: list[dict[str, Any]],
    values: dict[str, Any],
) -> dict[str, Any]:
    paper: dict[str, Any] = {
        "title": plan["title"],
        "sections": sections,
    }
    if boolean_input(values, "abstract_enabled", default=True):
        paper["abstract"] = string_input(values, "abstract") or plan.get("abstract", "")
    if boolean_input(values, "keywords_enabled", default=True):
        paper["keywords"] = normalize_academic_paper_keywords(plan.get("keywords"), values)
    if boolean_input(values, "references_enabled", default=True):
        paper["references"] = plan.get("references", [])
    if boolean_input(values, "acknowledgements_enabled", default=False):
        paper["acknowledgements"] = string_input(values, "acknowledgements") or plan.get("acknowledgements", "")
    if boolean_input(values, "cover_enabled", default=True):
        cover_aliases = {
            "subtitle": ("subtitle", "cover_subtitle"),
            "author": ("author", "cover_author"),
            "institution": ("institution", "cover_school"),
            "department": ("department", "cover_department"),
            "major": ("major", "cover_major"),
            "student_id": ("student_id", "cover_student_id"),
            "advisor": ("advisor", "cover_supervisor"),
            "date": ("date", "cover_submission_date"),
        }
        for key, aliases in cover_aliases.items():
            value = first_nonempty_input(values, *aliases)
            if value:
                paper[key] = value
    if boolean_input(values, "appendix_enabled", default=False):
        appendices = values.get("appendices")
        paper["appendices"] = appendices if isinstance(appendices, list) else plan.get("appendices", [])
    return paper


def academic_paper_format_settings(values: dict[str, Any]) -> dict[str, Any]:
    settings: dict[str, Any] = {}
    raw_settings = values.get("format_settings")
    if isinstance(raw_settings, str):
        raw_settings = parse_json_object(raw_settings)
    if isinstance(raw_settings, dict):
        deep_merge_dict(settings, raw_settings)

    assign_setting(settings, ("citation_style",), academic_paper_citation_style(values))

    assign_setting(settings, ("pagination", "include_title_page"), values.get("cover_enabled"))
    assign_setting(settings, ("pagination", "title_page_break_after"), values.get("cover_page_break_after"))

    assign_setting(settings, ("format_preset",), first_present_value(values, "page_format_preset", "format_preset"))
    assign_setting(settings, ("page", "size"), values.get("page_size"))
    assign_setting(settings, ("page", "orientation"), first_present_value(values, "page_orientation", "orientation"))
    assign_setting(settings, ("page", "width_mm"), first_present_value(values, "page_width_mm", "custom_page_width_mm"))
    assign_setting(settings, ("page", "height_mm"), first_present_value(values, "page_height_mm", "custom_page_height_mm"))
    for side in ("top", "bottom", "left", "right", "gutter"):
        margin = first_present_value(values, f"page_margin_{side}_mm", f"margin_{side}_mm")
        if margin is None:
            margin = centimeters_to_millimeters(values.get(f"page_margin_{side}_cm"))
        if side == "gutter":
            gutter = first_present_value(
                values,
                "page_gutter_mm",
                "page_margin_gutter_mm",
                "margin_gutter_mm",
            )
            if gutter is not None:
                margin = gutter
        assign_setting(settings, ("page", "margins_mm", side), margin)
    for key in ("header_distance_mm", "footer_distance_mm"):
        area = key.removesuffix("_distance_mm")
        distance = first_present_value(values, f"page_{key}", key, f"{area}_distance_mm")
        if distance is None:
            distance = centimeters_to_millimeters(values.get(f"{area}_distance_cm"))
        assign_setting(settings, ("page", key), distance)

    assign_setting(
        settings,
        ("fonts", "east_asia"),
        first_present_value(values, "default_east_asia_font", "east_asia_font"),
    )
    assign_setting(settings, ("fonts", "latin"), first_present_value(values, "default_latin_font", "latin_font"))

    style_names = (
        "title",
        "subtitle",
        "author",
        "metadata",
        "body",
        "abstract",
        "keywords",
        "references",
        "acknowledgements",
        "appendix",
    )
    style_keys = (
        "east_asia_font",
        "latin_font",
        "size_pt",
        "bold",
        "italic",
        "underline",
        "alignment",
        "first_line_indent_chars",
        "first_line_indent_cm",
        "left_indent_cm",
        "right_indent_cm",
        "line_spacing_mode",
        "line_spacing_value",
        "space_before_pt",
        "space_after_pt",
        "hanging_indent_cm",
        "keep_with_next",
        "keep_together",
        "page_break_before",
        "widow_control",
    )
    for style_name in style_names:
        for key in style_keys:
            assign_setting(settings, (style_name, key), values.get(f"{style_name}_{key}"))
    for level in range(1, 6):
        for key in style_keys:
            assign_setting(settings, ("headings", str(level), key), values.get(f"heading{level}_{key}"))

    for label in ("abstract", "keywords", "references", "acknowledgements", "appendix"):
        assign_setting(settings, ("labels", label), values.get(f"{label}_title"))

    assign_setting(settings, ("heading_numbering", "enabled"), values.get("heading_numbering_enabled"))
    numbering_style = string_input(values, "heading_numbering_style")
    if numbering_style == "none":
        assign_setting(settings, ("heading_numbering", "enabled"), False)
    formats, number_formats = academic_heading_numbering_formats(values, numbering_style)
    if formats is not None:
        assign_setting(settings, ("heading_numbering", "formats"), formats)
    if number_formats is not None:
        assign_setting(settings, ("heading_numbering", "number_formats"), number_formats)
    suffix = academic_heading_numbering_suffix(
        first_present_value(values, "heading_numbering_suffix", "heading_numbering_separator")
    )
    assign_setting(settings, ("heading_numbering", "suffix"), suffix)
    for key in ("enabled", "title", "levels", "page_break_before", "page_break_after"):
        assign_setting(settings, ("toc", key), values.get(f"toc_{key}"))
    for area in ("header", "footer"):
        for key in (
            "enabled",
            "text",
            "alignment",
            "east_asia_font",
            "latin_font",
            "size_pt",
            "bold",
            "italic",
            "different_first_page",
            "distance_mm",
        ):
            value = values.get(f"{area}_{key}")
            if key == "distance_mm" and (value is None or value == ""):
                value = centimeters_to_millimeters(values.get(f"{area}_distance_cm"))
            assign_setting(settings, (area, key), value)
    for key in ("enabled", "position", "alignment", "start", "format", "show_on_first_page", "prefix", "suffix"):
        assign_setting(settings, ("page_number", key), values.get(f"page_number_{key}"))
    for key in (
        "title_page_break_after",
        "toc_page_break_after",
        "abstract_page_break_after",
        "chapter_page_break_before",
        "references_page_break_before",
        "acknowledgements_page_break_before",
        "appendix_page_break_before",
    ):
        assign_setting(settings, ("pagination", key), values.get(f"pagination_{key}"))
    return settings


def deep_merge_dict(target: dict[str, Any], source: dict[str, Any]) -> None:
    for key, value in source.items():
        if isinstance(value, dict):
            child = target.setdefault(str(key), {})
            if isinstance(child, dict):
                deep_merge_dict(child, value)
            else:
                target[str(key)] = value
        else:
            target[str(key)] = value


def assign_setting(target: dict[str, Any], path: tuple[str, ...], value: Any) -> None:
    if value is None or value == "":
        return
    cursor = target
    for key in path[:-1]:
        child = cursor.setdefault(key, {})
        if not isinstance(child, dict):
            child = {}
            cursor[key] = child
        cursor = child
    cursor[path[-1]] = value


def coerce_structured_setting(value: Any) -> Any:
    if not isinstance(value, str):
        return value
    candidate = value.strip()
    if not candidate or candidate[0] not in "[{":
        return value
    try:
        return json.loads(candidate)
    except ValueError:
        return value


def academic_heading_numbering_formats(
    values: dict[str, Any],
    style: str,
) -> tuple[list[str] | None, list[str] | None]:
    presets: dict[str, tuple[list[str], list[str]]] = {
        "chinese_chapter": (
            ["第%1章", "%1.%2", "%1.%2.%3", "%1.%2.%3.%4", "%1.%2.%3.%4.%5"],
            ["chineseCounting", "decimal", "decimal", "decimal", "decimal"],
        ),
        "decimal": (
            ["%1", "%1.%2", "%1.%2.%3", "%1.%2.%3.%4", "%1.%2.%3.%4.%5"],
            ["decimal"] * 5,
        ),
        "chinese_outline": (
            ["%1、", "（%2）", "%3.", "（%4）", "%5."],
            ["chineseCounting", "chineseCounting", "decimal", "decimal", "decimal"],
        ),
        "custom": (
            ["第%1章", "%1.%2", "%1.%2.%3", "%1.%2.%3.%4", "%1.%2.%3.%4.%5"],
            ["decimal"] * 5,
        ),
    }
    explicit = coerce_structured_setting(values.get("heading_numbering_formats"))
    has_level_override = any(values.get(f"heading{level}_number_format") not in (None, "") for level in range(1, 6))
    if style not in presets and not isinstance(explicit, (list, dict)) and not has_level_override:
        return None, None

    base_formats, number_formats = presets.get(style, presets["custom"])
    formats = list(base_formats)
    if isinstance(explicit, list):
        for index, value in enumerate(explicit[:5]):
            if isinstance(value, str) and value:
                formats[index] = value
    elif isinstance(explicit, dict):
        for level in range(1, 6):
            value = explicit.get(str(level), explicit.get(level))
            if isinstance(value, str) and value:
                formats[level - 1] = value
    for level in range(1, 6):
        value = values.get(f"heading{level}_number_format")
        if isinstance(value, str) and value:
            formats[level - 1] = value
    return formats, list(number_formats)


def academic_heading_numbering_suffix(value: Any) -> str | None:
    if value is None:
        return None
    normalized = str(value).strip().lower()
    if normalized in {"tab", "制表符", "\\t"} or value == "\t":
        return "tab"
    if normalized in {"nothing", "none", "无", "不间隔"}:
        return "nothing"
    return "space"


def build_academic_paper_docx(
    paper: dict[str, Any],
    settings: dict[str, Any],
    *,
    template_bytes: bytes | None = None,
) -> bytes:
    try:
        from .paper_document import build_academic_paper_docx as build_document
    except ImportError as exc:
        raise WorkerFailure(
            "PAPER_DOCX_DEPENDENCY_MISSING",
            "生成 Word 文件所需依赖不可用，请重新构建 Worker 镜像。",
        ) from exc
    return build_document(paper, settings, template_bytes=template_bytes)


def count_academic_paper_words(paper: dict[str, Any]) -> int:
    parts = [clean_string(paper.get("abstract"))]
    for section in paper.get("sections", []):
        if isinstance(section, dict):
            parts.append(flatten_paper_section_text(section))
    return count_text_words("\n".join(parts))


def flatten_paper_section_text(section: dict[str, Any]) -> str:
    parts = [clean_string(section.get("content"))]
    children = section.get("children")
    if isinstance(children, list):
        for child in children:
            if isinstance(child, dict):
                parts.append(flatten_paper_section_text(child))
    return "\n".join(part for part in parts if part)


def count_text_words(value: str) -> int:
    return len(_PAPER_WORD_TOKEN_RE.findall(value))


def count_paper_references(value: Any) -> int:
    if isinstance(value, list):
        return len([item for item in value if clean_string(item)])
    if isinstance(value, str):
        return len([line for line in value.splitlines() if line.strip()])
    return 0


def public_document_artifact(
    _artifact: dict[str, Any],
    name: str,
    mime_type: str,
    size_bytes: int,
) -> dict[str, Any]:
    return {"name": name, "mime_type": mime_type, "size_bytes": size_bytes}


def normalize_string_list(value: Any) -> list[str]:
    if isinstance(value, list):
        items = []
        for item in value:
            if isinstance(item, dict):
                text = clean_string(item.get("text") or item.get("citation") or item.get("title"))
            else:
                text = clean_string(item)
            if text:
                items.append(text)
        return items
    if isinstance(value, str):
        candidate = value.strip()
        if not candidate:
            return []
        structured = coerce_structured_setting(candidate)
        if isinstance(structured, list):
            return normalize_string_list(structured)
        return [item.strip(" -\t") for item in re.split(r"[\n,，;；]+", candidate) if item.strip(" -\t")]
    return []


def academic_paper_keyword_target_count(values: dict[str, Any]) -> int:
    if not boolean_input(values, "keywords_enabled", default=True):
        return 0
    return optional_int_input(values, "keywords_count", default=5, minimum=2, maximum=10)


def academic_paper_keyword_instruction(values: dict[str, Any]) -> str:
    target_count = academic_paper_keyword_target_count(values)
    if target_count == 0:
        return "关键词未启用，keywords 必须返回空数组。"
    return f"关键词已启用，keywords 必须恰好返回 {target_count} 个互不重复的关键词。"


def normalize_academic_paper_keywords(
    value: Any,
    values: dict[str, Any],
    *,
    fallback_values: Any = None,
) -> list[str]:
    target_count = academic_paper_keyword_target_count(values)
    if target_count == 0:
        return []

    candidates: list[str] = []
    candidates.extend(normalize_string_list(values.get("keywords")))
    candidates.extend(normalize_string_list(value))
    candidates.extend(normalize_string_list(fallback_values))

    requirement_candidates = normalize_string_list(values.get("keywords_requirements"))
    if len(requirement_candidates) > 1:
        candidates.extend(requirement_candidates)

    for source in (
        first_nonempty_input(values, "topic", "paper_title", "title"),
        string_input(values, "discipline"),
    ):
        if source:
            candidates.append(source[:32])
    candidates.extend(normalize_string_list(values.get("research_method")))

    language = string_input(values, "language").lower()
    if language.startswith("en"):
        candidates.extend(
            [
                "Research Topic",
                "Theoretical Framework",
                "Influence Mechanism",
                "Empirical Analysis",
                "Research Method",
                "Practical Path",
                "Policy Recommendation",
                "Applied Research",
                "Development Trend",
                "Optimization Strategy",
            ]
        )
    else:
        candidates.extend(
            [
                "研究主题",
                "理论基础",
                "影响机制",
                "实证分析",
                "研究方法",
                "实践路径",
                "对策建议",
                "应用研究",
                "发展趋势",
                "优化策略",
            ]
        )

    keywords: list[str] = []
    seen: set[str] = set()
    for candidate in candidates:
        keyword = clean_string(candidate).strip(" -\t,，;；。")
        identity = keyword.casefold()
        if not keyword or identity in seen:
            continue
        seen.add(identity)
        keywords.append(keyword)
        if len(keywords) == target_count:
            break
    return keywords


def clean_string(value: Any) -> str:
    return value.strip() if isinstance(value, str) else ""


def positive_int(value: Any) -> int:
    if isinstance(value, bool):
        return 0
    try:
        return max(int(value), 0)
    except (TypeError, ValueError):
        return 0


def optional_int_input(values: dict[str, Any], key: str, *, default: int, minimum: int, maximum: int) -> int:
    value = values.get(key)
    if value in (None, ""):
        return default
    try:
        return min(max(int(value), minimum), maximum)
    except (TypeError, ValueError):
        return default


def boolean_input(values: dict[str, Any], key: str, *, default: bool) -> bool:
    value = values.get(key)
    if value is None or value == "":
        return default
    if isinstance(value, bool):
        return value
    if isinstance(value, (int, float)):
        return value != 0
    if isinstance(value, str):
        normalized = normalize_policy_value(value)
        if normalized in {"true", "yes", "on", "1", "enabled"}:
            return True
        if normalized in {"false", "no", "off", "0", "disabled"}:
            return False
    return default


def first_nonempty_input(values: dict[str, Any], *keys: str) -> str:
    for key in keys:
        value = string_input(values, key)
        if value:
            return value
    return ""


def first_present_value(values: dict[str, Any], *keys: str) -> Any:
    for key in keys:
        value = values.get(key)
        if value is not None and value != "":
            return value
    return None


def centimeters_to_millimeters(value: Any) -> float | None:
    if value is None or value == "" or isinstance(value, bool):
        return None
    try:
        return float(value) * 10
    except (TypeError, ValueError):
        return None


def build_product_marketing_prompt(values: dict[str, Any], output_count: int) -> str:
    fields = {
        "product_name": string_input(values, "product_name"),
        "selling_points": string_input(values, "selling_points") or string_input(values, "selling"),
        "target_audience": string_input(values, "target_audience"),
        "platform": string_input(values, "platform") or "general ecommerce",
        "visual_style": string_input(values, "visual_style") or string_input(values, "style") or "clean commercial photography",
        "language": string_input(values, "language") or "zh-CN",
        "campaign_goal": string_input(values, "campaign_goal") or "conversion",
        "additional_requirements": string_input(values, "additional_requirements"),
    }
    return f"""You are a senior ecommerce creative director. Analyze the supplied product images when present and create a production-ready marketing package.
Product brief:
{json.dumps(fields, ensure_ascii=False, indent=2)}

Return JSON only, without Markdown fences. Use exactly this shape:
{{
  "summary": "short campaign direction",
  "headlines": ["3 to 5 platform-ready headlines"],
  "selling_points": ["3 to 6 concise benefit-led points"],
  "description": "finished product description in the requested language",
  "visual_direction": "consistent art direction",
  "image_prompts": ["exactly {output_count} detailed image-generation prompts"]
}}
Each image prompt must identify the product, intended composition, background, lighting, camera angle, platform, and visual style. When reference product images are supplied, explicitly require preserving product shape, materials, colors, logo, labels, and packaging details. Do not invent certifications, prices, discounts, or unsupported product claims."""


def format_product_marketing_result(plan: dict[str, Any], product_name: str) -> str:
    sections: list[str] = [f"{product_name} AI 商品营销包"]

    summary = str(plan.get("summary") or "").strip()
    if summary:
        sections.append(f"营销策略\n{summary}")

    for title, key in (("标题建议", "headlines"), ("核心卖点", "selling_points")):
        values = plan.get(key)
        items = [str(item).strip() for item in values if str(item).strip()] if isinstance(values, list) else []
        if items:
            sections.append(f"{title}\n" + "\n".join(f"- {item}" for item in items))

    description = str(plan.get("description") or "").strip()
    if description:
        sections.append(f"商品文案\n{description}")

    visual_direction = str(plan.get("visual_direction") or "").strip()
    if visual_direction:
        sections.append(f"视觉方向\n{visual_direction}")

    return "\n\n".join(sections)


def parse_product_marketing_plan(raw: str, values: dict[str, Any], output_count: int) -> dict[str, Any]:
    candidate = raw.strip()
    if candidate.startswith("```"):
        candidate = candidate.split("\n", 1)[1] if "\n" in candidate else candidate[3:]
        candidate = candidate.rsplit("```", 1)[0].strip()
    try:
        parsed = json.loads(candidate)
    except (TypeError, ValueError):
        parsed = {}
    if not isinstance(parsed, dict):
        parsed = {}

    product_name = string_input(values, "product_name")
    style = string_input(values, "visual_style") or string_input(values, "style") or "clean commercial photography"
    platform = string_input(values, "platform") or "ecommerce"
    prompts = parsed.get("image_prompts")
    valid_prompts = [item.strip() for item in prompts if isinstance(item, str) and item.strip()] if isinstance(prompts, list) else []
    while len(valid_prompts) < output_count:
        index = len(valid_prompts) + 1
        valid_prompts.append(
            f"Commercial marketing image {index} for {product_name}; {style}; optimized for {platform}; "
            "preserve the exact product design, colors, materials, logo, labels, and packaging from the reference images; "
            "clear product focus, professional lighting, realistic details, no unsupported text or claims."
        )
    parsed["image_prompts"] = valid_prompts[:output_count]
    parsed.setdefault("summary", raw.strip())
    for key in ("headlines", "selling_points"):
        if not isinstance(parsed.get(key), list):
            parsed[key] = []
    for key in ("description", "visual_direction"):
        if not isinstance(parsed.get(key), str):
            parsed[key] = ""
    return parsed


def select_policy(payload: WorkerRunRequest) -> SelectedPolicy:
    policies = payload.node_model_policy or {}
    prefer_vision = any(is_image_artifact_ref(item) for item in input_artifacts(payload))
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
        if artifact.url and is_image_artifact_ref(artifact)
    ]
    if not image_parts:
        return prompt
    return [{"type": "text", "text": prompt}, *image_parts]


def input_artifacts(payload: WorkerRunRequest) -> list[WorkerArtifactRef]:
    if payload.input_artifacts:
        return payload.input_artifacts
    return payload.input_assets


def artifact_ref_mime(artifact: WorkerArtifactRef) -> str:
    direct = artifact.mime_type.strip().lower()
    if direct:
        return direct
    for key in ("mime_type", "content_type", "media_type"):
        value = artifact.metadata.get(key)
        if isinstance(value, str) and value.strip():
            return value.strip().lower()
    return mime_from_filename(artifact.name)


def artifact_ref_asset_type(artifact: WorkerArtifactRef) -> str:
    value = artifact.metadata.get("asset_type")
    return normalize_policy_value(value) if isinstance(value, str) else ""


def is_audio_artifact_ref(artifact: WorkerArtifactRef) -> bool:
    return artifact_ref_mime(artifact).startswith("audio/") or artifact_ref_asset_type(artifact) == "audio"


def is_image_artifact_ref(artifact: WorkerArtifactRef) -> bool:
    return artifact_ref_mime(artifact).startswith("image/") or artifact_ref_asset_type(artifact) == "image"


def is_video_artifact_ref(artifact: WorkerArtifactRef) -> bool:
    return artifact_ref_mime(artifact).startswith("video/") or artifact_ref_asset_type(artifact) == "video"


def is_grok_video_request(payload: WorkerRunRequest, policy: SelectedPolicy) -> bool:
    route = normalize_policy_value(str(payload.metadata.get("worker_route") or ""))
    if "grok_video" in route:
        return True
    capability = normalize_policy_value(policy.capability)
    if capability in {
        "grok_video",
        "grok_video_generation",
        "grok_video_generate",
        "grok_text_to_video",
        "grok_image_to_video",
        "grok_reference_to_video",
        "grok_video_edit",
        "grok_video_extend",
    }:
        return True
    return policy.model.strip().lower().startswith("grok-imagine-video")


def grok_video_mode(payload: WorkerRunRequest) -> str:
    raw = ""
    for key in ("mode", "video_mode", "generation_mode", "operation", "task"):
        raw = string_input(payload.input, key)
        if raw:
            break
    normalized = normalize_policy_value(raw)
    aliases = {
        "": "",
        "text": "text_to_video",
        "text_to_video": "text_to_video",
        "txt2video": "text_to_video",
        "t2v": "text_to_video",
        "文生视频": "text_to_video",
        "image": "image_to_video",
        "image_to_video": "image_to_video",
        "img2video": "image_to_video",
        "i2v": "image_to_video",
        "first_frame": "image_to_video",
        "图生视频": "image_to_video",
        "reference": "reference_to_video",
        "reference_to_video": "reference_to_video",
        "references_to_video": "reference_to_video",
        "multi_image_to_video": "reference_to_video",
        "multi_images_to_video": "reference_to_video",
        "images_to_video": "reference_to_video",
        "多图生视频": "reference_to_video",
        "edit": "edit_video",
        "edit_video": "edit_video",
        "video_edit": "edit_video",
        "视频编辑": "edit_video",
        "extend": "extend_video",
        "extension": "extend_video",
        "extend_video": "extend_video",
        "video_extend": "extend_video",
        "video_extension": "extend_video",
        "视频续写": "extend_video",
    }
    if normalized and normalized not in aliases:
        raise WorkerFailure(
            "GROK_VIDEO_MODE_UNSUPPORTED",
            "Grok video mode must be one of text_to_video, image_to_video, reference_to_video, edit_video, extend_video.",
        )
    if normalized:
        return aliases[normalized]

    images = [artifact for artifact in input_artifacts(payload) if is_image_artifact_ref(artifact)]
    videos = [artifact for artifact in input_artifacts(payload) if is_video_artifact_ref(artifact)]
    if videos or grok_source_video_url(payload):
        return "edit_video"
    if len(images) > 1:
        return "reference_to_video"
    if images:
        return "image_to_video"
    return "text_to_video"


def select_grok_video_mode_policy(
    payload: WorkerRunRequest,
    mode: str,
    fallback: SelectedPolicy,
) -> SelectedPolicy:
    mode_aliases = {
        mode,
        mode.replace("_video", ""),
        f"grok_{mode}",
        f"video_{mode}",
    }
    if mode == "text_to_video":
        mode_aliases.update({"video", "videos", "video_generation", "video_generate", "generate"})
    elif mode == "image_to_video":
        mode_aliases.update({"first_frame_to_video", "image_video"})
    elif mode == "reference_to_video":
        mode_aliases.update({"references_to_video", "multi_image_to_video", "images_to_video"})
    elif mode == "edit_video":
        mode_aliases.update({"video_edit", "edit"})
    elif mode == "extend_video":
        mode_aliases.update({"video_extend", "video_extension", "extend", "extension"})

    candidates: list[tuple[int, str, ModelPolicy]] = []
    for key, policy in (payload.node_model_policy or {}).items():
        normalized = policy if isinstance(policy, ModelPolicy) else ModelPolicy.model_validate(policy)
        node_id, role = policy_key_parts(key)
        normalized.node_id = normalized.node_id or node_id
        normalized.role = normalized.role or role or "generate"
        capability = normalize_policy_value(normalized.capability)
        role_value = normalize_policy_value(normalized.role)
        key_value = normalize_policy_value(key)
        if not (
            capability in mode_aliases
            or role_value in mode_aliases
            or key_value in mode_aliases
            or any(alias and alias in key_value for alias in mode_aliases if alias not in {"video", "generate"})
        ):
            continue
        if not normalized.model.strip().lower().startswith("grok-imagine-video"):
            continue
        score = 50
        if role_value == mode or capability == mode:
            score -= 20
        if normalized.model:
            score -= 10
        candidates.append((score, key, normalized))

    if not candidates:
        return fallback
    _, key, policy = sorted(candidates, key=lambda item: (item[0], item[1]))[0]
    if not policy.model:
        raise WorkerFailure("MODEL_POLICY_MODEL_REQUIRED", f"Model policy {key} is missing model.")
    return SelectedPolicy(
        policy_key=key,
        node_id=policy.node_id or fallback.node_id,
        role=policy.role or fallback.role,
        model=policy.model,
        model_group_id=policy.model_group_id,
        capability=policy.capability or mode,
    )


def artifact_ref_field_name(artifact: WorkerArtifactRef) -> str:
    value = artifact.metadata.get("field_name")
    return normalize_policy_value(value) if isinstance(value, str) else ""


def artifact_ref_role(artifact: WorkerArtifactRef) -> str:
    value = artifact.metadata.get("asset_role")
    return normalize_policy_value(value) if isinstance(value, str) else ""


def select_grok_source_image(payload: WorkerRunRequest, images: list[WorkerArtifactRef]) -> WorkerArtifactRef | None:
    if not images:
        return None
    preferred_fields = {"source_image", "input_image", "first_frame", "image", "init_image"}
    preferred_roles = {"source", "input", "first_frame", "init"}
    for artifact in images:
        if artifact_ref_field_name(artifact) in preferred_fields or artifact_ref_role(artifact) in preferred_roles:
            return artifact
    return images[0]


def select_grok_reference_images(payload: WorkerRunRequest, images: list[WorkerArtifactRef]) -> list[WorkerArtifactRef]:
    if not images:
        return []
    preferred_fields = {"reference", "reference_image", "reference_images", "references", "images"}
    preferred_roles = {"reference"}
    selected = [
        artifact
        for artifact in images
        if artifact_ref_field_name(artifact) in preferred_fields or artifact_ref_role(artifact) in preferred_roles
    ]
    return selected or images


def grok_source_video_url(payload: WorkerRunRequest) -> str:
    for key in ("source_video_url", "video_url", "input_video_url", "reference_video_url"):
        value = string_input(payload.input, key)
        if value:
            return value
    return ""


def require_grok_source_video(
    payload: WorkerRunRequest,
    videos: list[WorkerArtifactRef],
    source_video_url: str,
    mode: str,
) -> tuple[str, WorkerArtifactRef | None]:
    if len(videos) > 1 or (videos and source_video_url):
        raise WorkerFailure("GROK_VIDEO_INPUT_MISMATCH", f"{mode} mode accepts exactly one source video.")
    source_video = select_grok_source_video(videos)
    if source_video is not None:
        if not source_video.url:
            raise WorkerFailure("GROK_VIDEO_SOURCE_URL_MISSING", f"{mode} input video has no download URL.")
        return source_video.url, source_video
    if source_video_url:
        return source_video_url, None
    raise WorkerFailure("GROK_VIDEO_SOURCE_REQUIRED", f"{mode} mode requires one source video.")


def select_grok_source_video(videos: list[WorkerArtifactRef]) -> WorkerArtifactRef | None:
    if not videos:
        return None
    preferred_fields = {"source_video", "input_video", "video", "reference_video"}
    preferred_roles = {"source", "input", "reference"}
    for artifact in videos:
        if artifact_ref_field_name(artifact) in preferred_fields or artifact_ref_role(artifact) in preferred_roles:
            return artifact
    return videos[0]


def grok_source_video_metadata(source_video: WorkerArtifactRef | None, source_url: str) -> dict[str, Any]:
    if source_video is None:
        return {"source_video_url_input": bool(source_url)}
    return {
        "source_video_artifact_id": source_video.artifact_id,
        "source_video_name": source_video.name,
        "source_video_duration_seconds": artifact_duration_seconds(source_video),
    }


async def artifact_to_data_url(artifact: WorkerArtifactRef, *, max_bytes: int | None = None) -> str:
    raw = await download_input_artifact(artifact, max_bytes=max_bytes)
    mime_type = artifact_ref_mime(artifact) or "application/octet-stream"
    return f"data:{mime_type};base64,{base64.b64encode(raw).decode('ascii')}"


def apply_grok_video_generation_options(source: dict[str, Any], target: dict[str, Any], *, max_duration: float) -> None:
    duration = numeric_input(source, "duration", "seconds")
    if duration is not None:
        if duration <= 0 or duration > max_duration:
            raise WorkerFailure(
                "GROK_VIDEO_DURATION_INVALID",
                f"Grok video duration must be greater than 0 and at most {max_duration:g} seconds.",
            )
        target["duration"] = normalized_number(duration)
    copy_alias_input_field(source, target, "resolution", "size")
    copy_alias_input_field(source, target, "aspect_ratio", "ratio")


def apply_grok_video_extension_options(source: dict[str, Any], target: dict[str, Any]) -> None:
    duration = numeric_input(source, "duration", "seconds")
    if duration is not None:
        if duration < GROK_VIDEO_EXTENSION_DURATION_MIN_SECONDS or duration > GROK_VIDEO_EXTENSION_DURATION_MAX_SECONDS:
            raise WorkerFailure(
                "GROK_VIDEO_EXTENSION_DURATION_INVALID",
                (
                    f"Grok video extension duration must be between "
                    f"{GROK_VIDEO_EXTENSION_DURATION_MIN_SECONDS:g} and "
                    f"{GROK_VIDEO_EXTENSION_DURATION_MAX_SECONDS:g} seconds."
                ),
            )
        target["duration"] = normalized_number(duration)


def ensure_no_grok_edit_unsupported_options(source: dict[str, Any]) -> None:
    unsupported = present_input_keys(source, "duration", "seconds", "resolution", "size", "aspect_ratio", "ratio")
    if unsupported:
        raise WorkerFailure(
            "GROK_VIDEO_EDIT_OPTIONS_UNSUPPORTED",
            f"edit_video does not support custom {', '.join(unsupported)}.",
        )


def ensure_no_grok_extension_unsupported_options(source: dict[str, Any]) -> None:
    unsupported = present_input_keys(source, "resolution", "size", "aspect_ratio", "ratio")
    if unsupported:
        raise WorkerFailure(
            "GROK_VIDEO_EXTENSION_OPTIONS_UNSUPPORTED",
            f"extend_video does not support custom {', '.join(unsupported)}.",
        )


def present_input_keys(source: dict[str, Any], *keys: str) -> list[str]:
    present: list[str] = []
    for key in keys:
        value = source.get(key)
        if value is not None and value != "":
            present.append(key)
    return present


def numeric_input(source: dict[str, Any], *keys: str) -> float | None:
    for key in keys:
        value = source.get(key)
        if value is None or value == "":
            continue
        if isinstance(value, bool):
            raise WorkerFailure("INPUT_NUMBER_INVALID", f"{key} must be a number.")
        if isinstance(value, (int, float)):
            return float(value)
        if isinstance(value, str):
            try:
                return float(value.strip())
            except ValueError as exc:
                raise WorkerFailure("INPUT_NUMBER_INVALID", f"{key} must be a number.") from exc
        raise WorkerFailure("INPUT_NUMBER_INVALID", f"{key} must be a number.")
    return None


def normalized_number(value: float) -> int | float:
    return int(value) if value.is_integer() else value


def artifact_duration_seconds(artifact: WorkerArtifactRef | None) -> float | None:
    if artifact is None:
        return None
    for key in ("duration_seconds", "duration", "media_duration_seconds", "video_duration_seconds"):
        value = artifact.metadata.get(key)
        if value is None or value == "":
            continue
        if isinstance(value, bool):
            continue
        if isinstance(value, (int, float)):
            return float(value)
        if isinstance(value, str):
            try:
                return float(value.strip())
            except ValueError:
                continue
    return None


def validate_grok_source_video_duration(
    artifact: WorkerArtifactRef | None,
    *,
    code: str,
    message: str,
    min_seconds: float | None = None,
    max_seconds: float | None = None,
) -> None:
    duration = artifact_duration_seconds(artifact)
    if duration is None:
        return
    if min_seconds is not None and duration < min_seconds:
        raise WorkerFailure(code, message)
    if max_seconds is not None and duration > max_seconds:
        raise WorkerFailure(code, message)


def grok_video_request_id(response: dict[str, Any]) -> str:
    for key in ("request_id", "id", "video_id"):
        value = first_string(response, key)
        if value:
            return value
    for key in ("data", "video", "result", "output"):
        nested = response.get(key)
        if isinstance(nested, dict):
            value = grok_video_request_id(nested)
            if value:
                return value
        if isinstance(nested, list):
            for item in nested:
                if isinstance(item, dict):
                    value = grok_video_request_id(item)
                    if value:
                        return value
    return ""


def grok_video_status(response: dict[str, Any]) -> str:
    for key in ("status", "state"):
        value = first_string(response, key)
        if value:
            return normalize_policy_value(value)
    for key in ("data", "video", "result", "output"):
        nested = response.get(key)
        if isinstance(nested, dict):
            value = grok_video_status(nested)
            if value:
                return value
        if isinstance(nested, list):
            for item in nested:
                if isinstance(item, dict):
                    value = grok_video_status(item)
                    if value:
                        return value
    return ""


def grok_video_error_message(response: dict[str, Any]) -> str:
    for key in ("error", "message", "failure_reason", "reason"):
        value = response.get(key)
        if isinstance(value, str) and value.strip():
            return value.strip()
        if isinstance(value, dict):
            nested = first_string(value, "message", "error", "reason")
            if nested:
                return nested
    for key in ("data", "video", "result", "output"):
        nested = response.get(key)
        if isinstance(nested, dict):
            value = grok_video_error_message(nested)
            if value:
                return value
    return ""


def reference_image_artifacts(payload: WorkerRunRequest) -> list[WorkerArtifactRef]:
    images = [artifact for artifact in input_artifacts(payload) if is_image_artifact_ref(artifact)]
    if not images:
        return []

    preferred_roles = {"reference", "source", "input", "init"}
    role_matches = []
    for artifact in images:
        role = artifact.metadata.get("asset_role")
        if isinstance(role, str) and normalize_policy_value(role) in preferred_roles:
            role_matches.append(artifact)
    if role_matches:
        return role_matches

    preferred_fields = {"reference", "reference_image", "source_image", "input_image", "images"}
    field_matches = []
    for artifact in images:
        field_name = artifact.metadata.get("field_name")
        if isinstance(field_name, str) and normalize_policy_value(field_name) in preferred_fields:
            field_matches.append(artifact)
    if field_matches:
        return field_matches

    return images


def reference_image_artifact(payload: WorkerRunRequest) -> WorkerArtifactRef | None:
    references = reference_image_artifacts(payload)
    return references[0] if references else None


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
    direct_text = response.get("text")
    if isinstance(direct_text, str) and direct_text.strip():
        return direct_text.strip()

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


def extract_media_result(response: Any, default_mime: str) -> dict[str, Any] | None:
    if not isinstance(response, dict):
        return None
    for key in ("url", "output_url", "download_url", "content_url"):
        value = response.get(key)
        if isinstance(value, str) and value.strip():
            return {"url": value.strip(), "mime_type": response.get("mime_type") or default_mime}
    for key in ("b64_json", "body_base64", "base64", "data"):
        value = response.get(key)
        if isinstance(value, str) and value.strip() and not value.lstrip().startswith(("http://", "https://")):
            return {"b64_json": value.strip(), "mime_type": response.get("mime_type") or default_mime}
    for key in ("data", "video", "videos", "output", "outputs", "result"):
        nested = response.get(key)
        if isinstance(nested, dict):
            result = extract_media_result(nested, default_mime)
            if result:
                return result
        if isinstance(nested, list):
            for item in nested:
                result = extract_media_result(item, default_mime)
                if result:
                    return result
    return None


def first_string(values: dict[str, Any], *keys: str) -> str:
    for key in keys:
        value = values.get(key)
        if isinstance(value, str) and value.strip():
            return value.strip()
    return ""


def string_input(values: dict[str, Any], key: str) -> str:
    value = values.get(key)
    return value.strip() if isinstance(value, str) else ""


def copy_input_fields(source: dict[str, Any], target: dict[str, Any], *keys: str) -> None:
    for key in keys:
        value = source.get(key)
        if value is not None and value != "":
            target[key] = value


def copy_alias_input_field(source: dict[str, Any], target: dict[str, Any], key: str, alias: str) -> None:
    value = source.get(key)
    if value is None or value == "":
        value = source.get(alias)
    if value is not None and value != "":
        target[key] = value


def artifact_size_limit(payload: WorkerRunRequest) -> int:
    limits = [MAX_REMOTE_ARTIFACT_BYTES] if MAX_REMOTE_ARTIFACT_BYTES > 0 else []
    policy = payload.metadata.get("artifact_policy")
    if isinstance(policy, dict):
        value = policy.get("max_file_mb")
        if not isinstance(value, bool):
            try:
                max_file_mb = int(value)
            except (TypeError, ValueError):
                max_file_mb = 0
            if max_file_mb > 0:
                limits.append(max_file_mb * 1024 * 1024)
    return min(limits) if limits else 0


def response_content_length(headers: httpx.Headers) -> int:
    try:
        return max(int(headers.get("content-length") or 0), 0)
    except ValueError:
        return 0


def require_artifact_response(value: Any) -> dict[str, Any]:
    if not isinstance(value, dict) or not isinstance(value.get("artifact_id"), int) or value["artifact_id"] <= 0:
        raise WorkerFailure("ARTIFACT_RESPONSE_INVALID", "Sub2API returned an invalid artifact response.")
    return value


def proxy_usage(proxy_result: dict[str, Any]) -> dict[str, Any]:
    usage = proxy_result.get("usage")
    return usage if isinstance(usage, dict) else {}


def model_call_metadata(policy: SelectedPolicy) -> dict[str, Any]:
    return {
        "policy_key": policy.policy_key,
        "model": policy.model,
        "capability": policy.capability,
        "uses_model_proxy": True,
    }


def run_completion_metadata(
    worker_run_id: str,
    policy: SelectedPolicy,
    started: float,
    usage: dict[str, Any],
) -> dict[str, Any]:
    return {
        **model_call_metadata(policy),
        "worker_run_id": worker_run_id,
        "duration_ms": int((time.perf_counter() - started) * 1000),
        "usage": usage,
    }


def normalized_content_type(value: Any) -> str:
    if not isinstance(value, str):
        return ""
    return value.split(";", 1)[0].strip().lower()


def audio_mime_from_format(value: str) -> str:
    return {
        "aac": "audio/aac",
        "flac": "audio/flac",
        "mp3": "audio/mpeg",
        "opus": "audio/ogg",
        "pcm": "audio/L16",
        "wav": "audio/wav",
    }.get(value.strip().lower(), "audio/mpeg")


def mime_from_filename(name: str) -> str:
    extension = os.path.splitext(name.lower())[1]
    return {
        ".aac": "audio/aac",
        ".flac": "audio/flac",
        ".m4a": "audio/mp4",
        ".mp3": "audio/mpeg",
        ".ogg": "audio/ogg",
        ".opus": "audio/ogg",
        ".wav": "audio/wav",
        ".avi": "video/x-msvideo",
        ".m4v": "video/mp4",
        ".mkv": "video/x-matroska",
        ".mov": "video/quicktime",
        ".mp4": "video/mp4",
        ".webm": "video/webm",
        ".gif": "image/gif",
        ".jpeg": "image/jpeg",
        ".jpg": "image/jpeg",
        ".png": "image/png",
        ".webp": "image/webp",
    }.get(extension, "")


def extension_for_mime(mime_type: str) -> str:
    return {
        "audio/aac": ".aac",
        "audio/flac": ".flac",
        "audio/l16": ".pcm",
        "audio/mp4": ".m4a",
        "audio/mpeg": ".mp3",
        "audio/ogg": ".ogg",
        "audio/wav": ".wav",
        "video/mp4": ".mp4",
        "video/quicktime": ".mov",
        "video/webm": ".webm",
        "image/jpeg": ".jpg",
        "image/png": ".png",
        "image/webp": ".webp",
    }.get(normalized_content_type(mime_type), ".bin")


def proxy_artifact_name(proxy_result: dict[str, Any], default_stem: str, mime_type: str) -> str:
    headers = proxy_result.get("headers")
    if isinstance(headers, dict):
        disposition = headers.get("Content-Disposition") or headers.get("Content-disposition")
        if isinstance(disposition, str):
            for part in disposition.split(";"):
                key, separator, value = part.strip().partition("=")
                if separator and key.lower() == "filename":
                    filename = os.path.basename(value.strip().strip('"'))
                    if filename:
                        return filename
    return default_stem + extension_for_mime(mime_type)


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


def cancel_active_model_proxy_task(run_id: int) -> None:
    task = active_model_proxy_tasks.get(run_id)
    if task is not None and not task.done():
        task.cancel()


def truncate(value: str, limit: int) -> str:
    value = value.strip()
    if len(value) <= limit:
        return value
    return value[:limit] + "..."
