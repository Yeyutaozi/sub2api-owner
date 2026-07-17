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
from typing import Any, Awaitable, Callable

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
MAX_MODEL_PROXY_ASSET_BYTES = int(os.getenv("MAX_MODEL_PROXY_ASSET_BYTES", str(60 * 1024 * 1024)))
MAX_IMAGE_REFERENCE_COUNT = max(int(os.getenv("MAX_IMAGE_REFERENCE_COUNT", "16")), 1)
MAX_IMAGE_REFERENCE_BYTES = int(os.getenv("MAX_IMAGE_REFERENCE_BYTES", str(20 * 1024 * 1024)))
MAX_IMAGE_REFERENCE_TOTAL_BYTES = int(os.getenv("MAX_IMAGE_REFERENCE_TOTAL_BYTES", str(45 * 1024 * 1024)))
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
        except WorkerFailure as exc:
            await callback_failure(payload, exc.code, exc.message)
        except Exception as exc:  # noqa: BLE001 - keep Worker callbacks robust.
            LOGGER.exception("run failed: run_id=%s", payload.run_id)
            await callback_failure(payload, "WORKER_RUNTIME_ERROR", str(exc))
        finally:
            canceled_runs.discard(payload.run_id)


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
        except WorkerCanceled:
            await callback(payload, "canceled", status="canceled", message="Run canceled")
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
